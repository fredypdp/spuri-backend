package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
)

// These tests use the production router, authentication middleware, repository,
// ledger and projections. They deliberately require an isolated database because
// the projection rebuilds replay the complete ledger.
type registrosCorrecaoFixture struct {
	client        *db.Client
	repository    *db.AggregateRepository
	router        *gin.Engine
	academia      *aggregates.Academia
	outraAcademia *aggregates.Academia
	estudante     *aggregates.Estudante
	codigoAluno   string
	materiaID     uuid.UUID
	notaID        uuid.UUID
	faltaID       string
	token         string
	tokenOutra    string
}

func setupRegistrosCorrecaoIntegration(t *testing.T) *registrosCorrecaoFixture {
	t.Helper()
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDir) })

	client, err := db.NewClient(db.DefaultConfig())
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.RunMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	oldDBClient, oldRepository, oldProjManager := dbClient, repository, projManager
	dbClient = client
	repository = db.NewAggregateRepository(client)
	projManager = projections.NewManager(client)
	projManager.RegisterProjection("notas", projections.NewNotasProjection(client))
	projManager.RegisterProjection("faltas", projections.NewFaltasProjection(client))
	t.Cleanup(func() {
		dbClient, repository, projManager = oldDBClient, oldRepository, oldProjManager
	})

	sequence := time.Now().UnixNano()
	academia := criarAcademiaCorrecao(t, repository, sequence, "A")
	outraAcademia := criarAcademiaCorrecao(t, repository, sequence, "B")
	academiaProjection := projections.NewAcademiaProjection(client)
	if err := academiaProjection.Rebuild(); err != nil {
		t.Fatalf("rebuild academias: %v", err)
	}

	ano := "1_ano_fundamental"
	codigoAluno := fmt.Sprintf("%07d", sequence%10_000_000)
	estudante := aggregates.NewEstudante()
	if err := estudante.CriarComVinculo("Aluno de integração", codigoAluno, "hash", nil, nil, nil, nil, nil, "M", time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC), &ano, nil, nil, nil, nil, &academia.ID, academia.CodigoAcademia); err != nil {
		t.Fatalf("criar estudante: %v", err)
	}
	if err := repository.SaveWithAudit(estudante, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar estudante: %v", err)
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild estudantes: %v", err)
	}

	materiaID := uuid.New()
	if _, err := client.DB().Exec(`
		INSERT INTO projection_materias (id, nome, type, codigo_academia, anos_academicos, status, created_at)
		VALUES ($1, 'Matemática integração', 'fundamental', $2, '["1_ano_fundamental"]'::jsonb, 'ativo', CURRENT_TIMESTAMP)
	`, materiaID, academia.CodigoAcademia); err != nil {
		t.Fatalf("inserir matéria: %v", err)
	}

	if err := estudante.RegistrarNota(academia.CodigoAcademia, "2026", ano, "1_trimestre", materiaID, aggregates.TipoEscolar, "nota_professor", 8, nil, []string{"nota_professor", "prova_trimestral"}, aggregates.PeriodosEscolar, academia.ID, 10); err != nil {
		t.Fatalf("registrar nota inicial: %v", err)
	}
	if err := estudante.RegistrarFalta(academia.CodigoAcademia, "2026", ano, time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), materiaID, 2, nil, academia.ID, aggregates.MaxQuantidadeFaltasPadrao); err != nil {
		t.Fatalf("registrar falta inicial: %v", err)
	}
	if err := repository.SaveWithAudit(estudante, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar registros iniciais: %v", err)
	}
	if err := projections.NewNotasProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild notas iniciais: %v", err)
	}
	if err := projections.NewFaltasProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild faltas iniciais: %v", err)
	}

	var notaID uuid.UUID
	if err := client.DB().QueryRow(`SELECT id FROM projection_notas WHERE codigo_estudante = $1 AND categoria = 'nota_professor'`, codigoAluno).Scan(&notaID); err != nil {
		t.Fatalf("buscar nota inicial: %v", err)
	}
	var faltaID string
	if err := client.DB().QueryRow(`SELECT id FROM projection_faltas WHERE codigo_estudante = $1`, codigoAluno).Scan(&faltaID); err != nil {
		t.Fatalf("buscar falta inicial: %v", err)
	}
	token, err := middleware.GenerateToken(academia.ID, "academia")
	if err != nil {
		t.Fatalf("gerar token da academia: %v", err)
	}
	tokenOutra, err := middleware.GenerateToken(outraAcademia.ID, "academia")
	if err != nil {
		t.Fatalf("gerar token da outra academia: %v", err)
	}

	return &registrosCorrecaoFixture{
		client: client, repository: repository, router: setupRouter(), academia: academia, outraAcademia: outraAcademia,
		estudante: estudante, codigoAluno: codigoAluno, materiaID: materiaID, notaID: notaID, faltaID: faltaID,
		token: token, tokenOutra: tokenOutra,
	}
}

func criarAcademiaCorrecao(t *testing.T, repository *db.AggregateRepository, sequence int64, suffix string) *aggregates.Academia {
	t.Helper()
	academia := aggregates.NewAcademia()
	nivel := "fundamental"
	nif := fmt.Sprintf("%010d", (sequence+int64(suffix[0]))%10_000_000_000)
	codigo := fmt.Sprintf("T%06d", (sequence+int64(suffix[0]))%1_000_000)
	if err := academia.Criar("escolar", "private", "Academia "+suffix, nif, codigo, "hash", "LUA", "Endereço de integração", nil, nil, nil, &nivel, nil, []string{"1_ano_fundamental"}, nil); err != nil {
		t.Fatalf("criar academia %s: %v", suffix, err)
	}
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: uuid.NewString(), UserType: "admin", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar academia %s: %v", suffix, err)
	}
	if err := academia.AtivarComAutor(uuid.New()); err != nil {
		t.Fatalf("ativar academia %s: %v", suffix, err)
	}
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: uuid.NewString(), UserType: "admin", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar ativação da academia %s: %v", suffix, err)
	}
	return academia
}

func requestCorrecao(router *gin.Engine, token, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHTTPIntegrationCorrigirNotaEFalta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)

	for _, tc := range []struct {
		name, token, path, body string
		status                  int
	}{
		{"nota de outra academia", fx.tokenOutra, "/academia/notas-aluno/" + fx.notaID.String(), `{"nota":7,"motivo":"ajuste"}`, http.StatusForbidden},
		{"falta de outra academia", fx.tokenOutra, "/academia/faltas-aluno/" + fx.faltaID, `{"quantidade":1,"motivo":"ajuste"}`, http.StatusForbidden},
		{"nota sem motivo", fx.token, "/academia/notas-aluno/" + fx.notaID.String(), `{"nota":7}`, http.StatusBadRequest},
		{"falta sem motivo", fx.token, "/academia/faltas-aluno/" + fx.faltaID, `{"quantidade":1}`, http.StatusBadRequest},
		{"nota inexistente", fx.token, "/academia/notas-aluno/" + uuid.NewString(), `{"nota":7,"motivo":"ajuste"}`, http.StatusNotFound},
		{"falta inexistente", fx.token, "/academia/faltas-aluno/" + uuid.NewString(), `{"quantidade":1,"motivo":"ajuste"}`, http.StatusNotFound},
		{"nota acima do teto", fx.token, "/academia/notas-aluno/" + fx.notaID.String(), `{"nota":11,"motivo":"ajuste"}`, http.StatusBadRequest},
		{"falta acima do teto", fx.token, "/academia/faltas-aluno/" + fx.faltaID, `{"quantidade":101,"motivo":"ajuste"}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if w := requestCorrecao(fx.router, tc.token, http.MethodPatch, tc.path, tc.body); w.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tc.status, w.Body.String())
			}
		})
	}

	if w := requestCorrecao(fx.router, fx.token, http.MethodPatch, "/academia/notas-aluno/"+fx.notaID.String(), `{"nota":7.5,"motivo":"correção de lançamento"}`); w.Code != http.StatusOK {
		t.Fatalf("corrigir nota status = %d: %s", w.Code, w.Body.String())
	}
	if w := requestCorrecao(fx.router, fx.token, http.MethodPatch, "/academia/faltas-aluno/"+fx.faltaID, `{"quantidade":1,"motivo":"correção de chamada"}`); w.Code != http.StatusOK {
		t.Fatalf("corrigir falta status = %d: %s", w.Code, w.Body.String())
	}
	if err := projections.NewNotasProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild notas após correção: %v", err)
	}
	if err := projections.NewFaltasProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild faltas após correção: %v", err)
	}

	for _, tc := range []struct {
		path, campo string
	}{
		{"/notas", "notas"},
		{"/faltas", "faltas"},
	} {
		w := requestCorrecao(fx.router, fx.token, http.MethodGet, tc.path, "")
		if w.Code != http.StatusOK {
			t.Fatalf("listar %s status = %d: %s", tc.campo, w.Code, w.Body.String())
		}
		assertRespostaContemAuditoriaCorrecao(t, w.Body.Bytes(), tc.campo, fx.academia.ID.String())
	}
}

func TestHTTPIntegrationCorrigirNotaRecalculaAvaliacaoFinal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	for _, item := range []struct {
		periodo, categoria string
	}{
		{"1_trimestre", "prova_trimestral"},
		{"2_trimestre", "nota_professor"},
		{"2_trimestre", "prova_trimestral"},
		{"3_trimestre", "nota_professor"},
		{"3_trimestre", "prova_trimestral"},
	} {
		if err := fx.estudante.RegistrarNota(fx.academia.CodigoAcademia, "2026", "1_ano_fundamental", item.periodo, fx.materiaID, aggregates.TipoEscolar, item.categoria, 8, nil, []string{"nota_professor", "prova_trimestral"}, aggregates.PeriodosEscolar, fx.academia.ID, 10); err != nil {
			t.Fatalf("registrar nota %s/%s: %v", item.periodo, item.categoria, err)
		}
	}
	if err := fx.repository.SaveWithAudit(fx.estudante, db.AuditContext{UserID: fx.academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar notas da fórmula: %v", err)
	}
	if err := projections.NewNotasProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild notas da fórmula: %v", err)
	}
	var notaDespertadora uuid.UUID
	if err := fx.client.DB().QueryRow(`SELECT id FROM projection_notas WHERE codigo_estudante=$1 AND periodo='3_trimestre' AND categoria='prova_trimestral'`, fx.codigoAluno).Scan(&notaDespertadora); err != nil {
		t.Fatalf("buscar nota despertadora: %v", err)
	}

	if w := requestCorrecao(fx.router, fx.token, http.MethodPatch, "/academia/notas-aluno/"+notaDespertadora.String(), `{"nota":4,"motivo":"revisão da prova trimestral"}`); w.Code != http.StatusOK {
		t.Fatalf("corrigir nota despertadora status = %d: %s", w.Code, w.Body.String())
	}

	var payload []byte
	if err := fx.client.DB().QueryRow(`SELECT payload FROM spuri_ledger WHERE aggregate_id=$1 AND event_type='AvaliacaoFinalEscolar' ORDER BY id DESC LIMIT 1`, fx.estudante.ID).Scan(&payload); err != nil {
		t.Fatalf("buscar avaliação final recalculada: %v", err)
	}
	var evento struct {
		NotaFinal float64 `json:"NotaFinal"`
	}
	if err := json.Unmarshal(payload, &evento); err != nil {
		t.Fatalf("decodificar avaliação final: %v", err)
	}
	if math.Abs(evento.NotaFinal-(22.0/3.0)) > 0.001 {
		t.Fatalf("nota final após correção = %.4f, want %.4f", evento.NotaFinal, 22.0/3.0)
	}
}

func TestIntegrationRebuildNotasEFaltasMantemRegistrosCorrigidos(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)

	if w := requestCorrecao(fx.router, fx.token, http.MethodPatch, "/academia/notas-aluno/"+fx.notaID.String(), `{"nota":7.5,"motivo":"ajuste para validar rebuild"}`); w.Code != http.StatusOK {
		t.Fatalf("corrigir nota status = %d: %s", w.Code, w.Body.String())
	}
	if w := requestCorrecao(fx.router, fx.token, http.MethodPatch, "/academia/faltas-aluno/"+fx.faltaID, `{"quantidade":1,"motivo":"ajuste para validar rebuild"}`); w.Code != http.StatusOK {
		t.Fatalf("corrigir falta status = %d: %s", w.Code, w.Body.String())
	}

	for _, name := range []string{"notas", "faltas"} {
		if err := projManager.RebuildProjection(name); err != nil {
			t.Fatalf("primeiro rebuild de %s: %v", name, err)
		}
	}
	primeiro := snapshotRegistrosCorrigidos(t, fx.client, fx.notaID, fx.faltaID)

	for _, name := range []string{"notas", "faltas"} {
		if err := projManager.RebuildProjection(name); err != nil {
			t.Fatalf("segundo rebuild de %s: %v", name, err)
		}
	}
	segundo := snapshotRegistrosCorrigidos(t, fx.client, fx.notaID, fx.faltaID)
	if primeiro != segundo {
		t.Fatalf("rebuild não foi determinístico: primeiro=%+v segundo=%+v", primeiro, segundo)
	}
	if primeiro.Nota != 7.5 || primeiro.NotaAnterior == nil || *primeiro.NotaAnterior != 8 || primeiro.Falta != 1 || primeiro.FaltaAnterior == nil || *primeiro.FaltaAnterior != 2 {
		t.Fatalf("estado reconstruído não preservou as correções: %+v", primeiro)
	}
}

type snapshotRegistrosCorrecao struct {
	Nota          float64
	NotaAnterior  *float64
	Falta         int
	FaltaAnterior *int
}

func snapshotRegistrosCorrigidos(t *testing.T, client *db.Client, notaID uuid.UUID, faltaID string) snapshotRegistrosCorrecao {
	t.Helper()
	var state snapshotRegistrosCorrecao
	if err := client.DB().QueryRow(`SELECT nota, valor_anterior FROM projection_notas WHERE id=$1`, notaID).Scan(&state.Nota, &state.NotaAnterior); err != nil {
		t.Fatalf("snapshot da nota reconstruída: %v", err)
	}
	if err := client.DB().QueryRow(`SELECT quantidade, valor_anterior FROM projection_faltas WHERE id=$1`, faltaID).Scan(&state.Falta, &state.FaltaAnterior); err != nil {
		t.Fatalf("snapshot da falta reconstruída: %v", err)
	}
	return state
}

func assertRespostaContemAuditoriaCorrecao(t *testing.T, body []byte, campo, academiaID string) {
	t.Helper()
	var response map[string][]map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decodificar resposta de %s: %v; body=%s", campo, err, body)
	}
	for _, registro := range response[campo] {
		if registro["corrigido_por"] == academiaID && registro["registrado_por"] == academiaID && registro["motivo_correcao"] != nil {
			return
		}
	}
	t.Fatalf("resposta de %s não expôs campos de auditoria da correção: %s", campo, body)
}
