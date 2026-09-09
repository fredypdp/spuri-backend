package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)

func setupNivelEscolarTestRouter(t *testing.T, client *db.Client, userID uuid.UUID) *gin.Engine {
	t.Helper()
	repository := db.NewAggregateRepository(client)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", client)
		c.Set("repository", repository)
		c.Set("user_id", userID)
		c.Set("user_type", "academia")
	})
	router.PUT("/academia/nivel-escolar", AtualizarNivelEscolarAcademia)
	return router
}

func criarAcademiaEscolarParaTeste(t *testing.T, client *db.Client, codigo, nivelEscolar string, anos []string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	agg := &aggregates.Academia{}
	agg.SetID(id)
	nif := fmt.Sprintf("9%09d", time.Now().UnixNano()%1000000000)
	if err := agg.Criar("escola", "private", "Academia Teste "+codigo, nif, codigo, "hash", "LUA", "Rua Teste", nil, nil, nil, &nivelEscolar, nil, anos, nil); err != nil {
		t.Fatalf("erro ao criar academia de teste: %v", err)
	}
	repository := db.NewAggregateRepository(client)
	if err := repository.SaveWithAudit(agg, db.AuditContext{UserID: "integration-test", UserType: "sistema", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("erro ao salvar academia de teste: %v", err)
	}
	if err := projections.NewAcademiaProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção de academias: %v", err)
	}
	return id
}

func putNivelEscolar(t *testing.T, router *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/academia/nivel-escolar", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func nivelEscolarTestClient(t *testing.T) *db.Client {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL não definido — pulei teste de integração de nivel_escolar")
	}
	client, err := db.NewClient(db.DefaultConfig())
	if err != nil {
		t.Fatalf("erro ao conectar no banco de teste: %v", err)
	}
	return client
}

// TestAtualizarNivelEscolarBloqueiaComDependenciasAtivas cobre o cenário
// central da Tarefa 07 revisada: uma academia fundamental com um estudante
// realmente ativo (status_escolar_fundamental='em_andamento') não pode virar
// 'medio' — a API deve responder 409 e a mudança não deve ser persistida.
func TestAtualizarNivelEscolarBloqueiaComDependenciasAtivas(t *testing.T) {
	client := nivelEscolarTestClient(t)
	codigo := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigo, "fundamental", []string{"1_ano_fundamental"})

	_, err := client.DB().Exec(`
		INSERT INTO projection_estudantes (id, nome, senha_hash, telefone, codigo_academia, status, status_escolar_fundamental, ano_escolar_fundamental, status_escolar_medio, created_at, updated_at)
		VALUES ($1, 'Estudante IT Ativo', 'hash', '900000099', $2, 'ativo', 'em_andamento', '1_ano_fundamental', 'inativo', now(), now())
	`, uuid.New(), codigo)
	if err != nil {
		t.Fatalf("erro ao inserir estudante de teste: %v", err)
	}

	router := setupNivelEscolarTestRouter(t, client, academiaID)
	rec := putNivelEscolar(t, router, map[string]any{"nivel_escolar": "medio"})

	if rec.Code != http.StatusConflict {
		t.Fatalf("esperava 409, recebeu %d: %s", rec.Code, rec.Body.String())
	}

	var nivelAtual string
	if err := client.DB().QueryRow(`SELECT nivel_escolar FROM projection_academias WHERE codigo_academia = $1`, codigo).Scan(&nivelAtual); err != nil {
		t.Fatalf("erro ao verificar nivel_escolar pós-tentativa: %v", err)
	}
	if nivelAtual != "fundamental" {
		t.Fatalf("nivel_escolar não deveria ter mudado; esperava 'fundamental', obteve %q", nivelAtual)
	}
}

// TestAtualizarNivelEscolarPermiteQuandoSemDependencias cobre o caminho
// feliz: academia fundamental sem nenhuma dependência ativa consegue virar
// 'medio', e — ponto crítico corrigido nesta tarefa — anos_academicos fica
// SQL NULL no banco (não '[]'), como a constraint check_anos_academicos_nivel
// exige.
func TestAtualizarNivelEscolarPermiteQuandoSemDependencias(t *testing.T) {
	client := nivelEscolarTestClient(t)
	codigo := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigo, "fundamental", []string{"1_ano_fundamental"})

	router := setupNivelEscolarTestRouter(t, client, academiaID)
	rec := putNivelEscolar(t, router, map[string]any{"nivel_escolar": "medio"})

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	if err := projections.NewAcademiaProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção após o PUT: %v", err)
	}

	var nivelAtual string
	var anosJSON []byte
	if err := client.DB().QueryRow(`SELECT nivel_escolar, anos_academicos FROM projection_academias WHERE codigo_academia = $1`, codigo).Scan(&nivelAtual, &anosJSON); err != nil {
		t.Fatalf("erro ao verificar estado pós-atualização: %v", err)
	}
	if nivelAtual != "medio" {
		t.Fatalf("esperava nivel_escolar='medio', obteve %q", nivelAtual)
	}
	if anosJSON != nil {
		t.Fatalf("esperava anos_academicos SQL NULL após virar 'medio', obteve %q — regressão do bug '[]' vs NULL", string(anosJSON))
	}
}

// TestAtualizarNivelEscolarDeMedioParaFundamentalExigeAnos cobre a transição
// inversa: medio -> fundamental exige anos_academicos no payload (medio
// nunca tem anos_academicos hoje) e os persiste corretamente.
func TestAtualizarNivelEscolarDeMedioParaFundamentalExigeAnos(t *testing.T) {
	client := nivelEscolarTestClient(t)
	codigo := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigo, "medio", nil)

	router := setupNivelEscolarTestRouter(t, client, academiaID)

	// Sem anos_academicos -> 400
	recSemAnos := putNivelEscolar(t, router, map[string]any{"nivel_escolar": "fundamental"})
	if recSemAnos.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 sem anos_academicos, recebeu %d: %s", recSemAnos.Code, recSemAnos.Body.String())
	}

	// Com anos_academicos -> 200
	rec := putNivelEscolar(t, router, map[string]any{
		"nivel_escolar":   "fundamental",
		"anos_academicos": []string{"1_ano_fundamental", "2_ano_fundamental"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d: %s", rec.Code, rec.Body.String())
	}
	if err := projections.NewAcademiaProjection(client).Rebuild(); err != nil {
		t.Fatalf("erro ao reconstruir projeção após o PUT: %v", err)
	}

	var nivelAtual string
	var anosJSON []byte
	if err := client.DB().QueryRow(`SELECT nivel_escolar, anos_academicos FROM projection_academias WHERE codigo_academia = $1`, codigo).Scan(&nivelAtual, &anosJSON); err != nil {
		t.Fatalf("erro ao verificar estado pós-atualização: %v", err)
	}
	if nivelAtual != "fundamental" {
		t.Fatalf("esperava nivel_escolar='fundamental', obteve %q", nivelAtual)
	}
	if anosJSON == nil {
		t.Fatalf("esperava anos_academicos preenchido, obteve NULL")
	}
}

// TestAtualizarNivelEscolarParaMistoNaoExigeValidacao cobre a transição
// aditiva (fundamental -> misto): não perde nenhum domínio, então não deve
// exigir ausência de dependências nem novo anos_academicos.
func TestAtualizarNivelEscolarParaMistoNaoExigeValidacao(t *testing.T) {
	client := nivelEscolarTestClient(t)
	codigo := "IT" + uuid.New().String()[:8]
	academiaID := criarAcademiaEscolarParaTeste(t, client, codigo, "fundamental", []string{"1_ano_fundamental"})

	_, err := client.DB().Exec(`
		INSERT INTO projection_estudantes (id, nome, senha_hash, telefone, codigo_academia, status, status_escolar_fundamental, ano_escolar_fundamental, status_escolar_medio, created_at, updated_at)
		VALUES ($1, 'Estudante IT Ativo Misto', 'hash', '900000098', $2, 'ativo', 'em_andamento', '1_ano_fundamental', 'inativo', now(), now())
	`, uuid.New(), codigo)
	if err != nil {
		t.Fatalf("erro ao inserir estudante de teste: %v", err)
	}

	router := setupNivelEscolarTestRouter(t, client, academiaID)
	rec := putNivelEscolar(t, router, map[string]any{"nivel_escolar": "misto"})

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200 (transição aditiva não deve ser bloqueada por dependências), recebeu %d: %s", rec.Code, rec.Body.String())
	}
}
