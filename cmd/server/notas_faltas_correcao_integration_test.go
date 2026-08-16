package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	client              *db.Client
	repository          *db.AggregateRepository
	router              *gin.Engine
	academia            *aggregates.Academia
	outraAcademia       *aggregates.Academia
	estudante           *aggregates.Estudante
	codigoAluno         string
	materiaID           uuid.UUID
	codigoAlunoSuperior string
	materiaSuperiorID   uuid.UUID
	notaID              uuid.UUID
	faltaID             string
	token               string
	tokenOutra          string
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
	anoSuperior := "1_ano_superior"
	codigoAlunoSuperior := fmt.Sprintf("%07d", (sequence+1)%10_000_000)
	estudanteSuperior := aggregates.NewEstudante()
	if err := estudanteSuperior.CriarComVinculo("Aluno superior de integração", codigoAlunoSuperior, "hash", nil, nil, nil, nil, nil, "F", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), nil, nil, &anoSuperior, nil, nil, &academia.ID, academia.CodigoAcademia); err != nil {
		t.Fatalf("criar estudante superior: %v", err)
	}
	if err := repository.SaveWithAudit(estudanteSuperior, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar estudante superior: %v", err)
	}

	cursoSuperiorID := uuid.New()
	if _, err := client.DB().Exec(`
		INSERT INTO projection_cursos (id, nome, type, nivel, periodos, codigo_academia, status, created_at)
		VALUES ($1, 'Curso Superior integração', 'superior', '[]'::jsonb, '["1_semestre","2_semestre"]'::jsonb, $2, 'ativo', CURRENT_TIMESTAMP)
	`, cursoSuperiorID, academia.CodigoAcademia); err != nil {
		t.Fatalf("inserir curso superior: %v", err)
	}
	materiaSuperiorID := uuid.New()
	if _, err := client.DB().Exec(`
		INSERT INTO projection_materias (id, nome, type, codigo_academia, curso_id, periodo, anos_academicos, status, created_at)
		VALUES ($1, 'Cálculo I integração', 'superior', $2, $3, '1_semestre', '["1_ano_superior"]'::jsonb, 'ativo', CURRENT_TIMESTAMP)
	`, materiaSuperiorID, academia.CodigoAcademia, cursoSuperiorID); err != nil {
		t.Fatalf("inserir materia superior: %v", err)
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
	if err := estudante.RegistrarFalta(academia.CodigoAcademia, "2026", ano, "1_trimestre", time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), materiaID, 2, nil, academia.ID, aggregates.PeriodosEscolar, aggregates.MaxQuantidadeFaltasPadrao); err != nil {
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
		estudante: estudante, codigoAluno: codigoAluno, materiaID: materiaID, codigoAlunoSuperior: codigoAlunoSuperior, materiaSuperiorID: materiaSuperiorID, notaID: notaID, faltaID: faltaID,
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

func registrarFaltaCorrecao(t *testing.T, fx *registrosCorrecaoFixture, codigo, periodo string, materia uuid.UUID, data string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"codigo_estudante":"%s","data":"%s","periodo":"%s","materia_disciplinar_id":"%s","quantidade":1}`, codigo, data, periodo, materia)
	return requestCorrecao(fx.router, fx.token, http.MethodPost, "/academia/faltas-aluno", body)
}

func TestFaltasPeriodo02EscolarComPeriodoInvalidoRetorna400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	w := registrarFaltaCorrecao(t, fx, fx.codigoAluno, "4_trimestre", fx.materiaID, "2026-02-11")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}
func TestFaltasPeriodo03EscolarSemPeriodoRetorna400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	body := fmt.Sprintf(`{"codigo_estudante":"%s","data":"2026-02-12","materia_disciplinar_id":"%s","quantidade":1}`, fx.codigoAluno, fx.materiaID)
	w := requestCorrecao(fx.router, fx.token, http.MethodPost, "/academia/faltas-aluno", body)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "periodo") {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}
func TestFaltasPeriodo04SuperiorComPeriodoDaMateriaTemSucesso(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	w := registrarFaltaCorrecao(t, fx, fx.codigoAlunoSuperior, "1_semestre", fx.materiaSuperiorID, "2026-02-13")
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}
func TestFaltasPeriodo05SuperiorComPeriodoDiferenteDaMateriaRetorna400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	w := registrarFaltaCorrecao(t, fx, fx.codigoAlunoSuperior, "2_semestre", fx.materiaSuperiorID, "2026-02-14")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "periodo '2_semestre' invalido") {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}
func TestFaltasPeriodo06PatchComPeriodoNoCorpoRejeitaCampoLegado(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	w := requestCorrecao(fx.router, fx.token, http.MethodPatch, "/academia/faltas-aluno/"+fx.faltaID, `{"quantidade":1,"motivo":"ajuste","periodo":"2_trimestre"}`)
	if w.Code < 400 || !strings.Contains(w.Body.String(), "periodo") {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}
func TestFaltasPeriodo07PatchSemPeriodoPreservaPeriodoOriginal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	w := requestCorrecao(fx.router, fx.token, http.MethodPatch, "/academia/faltas-aluno/"+fx.faltaID, `{"quantidade":1,"motivo":"ajuste sem periodo"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	dto, err := projections.NewFaltasProjection(fx.client).GetByID(fx.faltaID)
	if err != nil || dto.Periodo != "1_trimestre" {
		t.Fatalf("periodo=%q err=%v", dto.Periodo, err)
	}
}
func TestFaltasPeriodo08ListarFaltasFiltraPorPeriodo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	_ = registrarFaltaCorrecao(t, fx, fx.codigoAluno, "2_trimestre", fx.materiaID, "2026-02-15")
	_ = projections.NewFaltasProjection(fx.client).Rebuild()
	w := requestCorrecao(fx.router, fx.token, http.MethodGet, "/faltas?periodo=1_trimestre", "")
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "2_trimestre") {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}
func TestFaltasPeriodo09FaltasEstudanteFiltraPorPeriodo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	_ = registrarFaltaCorrecao(t, fx, fx.codigoAluno, "2_trimestre", fx.materiaID, "2026-02-16")
	_ = projections.NewFaltasProjection(fx.client).Rebuild()
	w := requestCorrecao(fx.router, fx.token, http.MethodGet, "/faltas-estudante/"+fx.codigoAluno+"?periodo=2_trimestre", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "2_trimestre") {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}
func TestFaltasPeriodo10RebuildPreservaPeriodo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	if err := projections.NewFaltasProjection(fx.client).Rebuild(); err != nil {
		t.Fatal(err)
	}
	dto, err := projections.NewFaltasProjection(fx.client).GetByID(fx.faltaID)
	if err != nil || dto.Periodo != "1_trimestre" {
		t.Fatalf("periodo=%q err=%v", dto.Periodo, err)
	}
}
func TestFaltasPeriodo11MesmaChaveComPeriodoDiferenteAceitaAmbas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	d := "2026-02-17"
	if w := registrarFaltaCorrecao(t, fx, fx.codigoAluno, "1_trimestre", fx.materiaID, d); w.Code != http.StatusCreated {
		t.Fatalf("1 status=%d %s", w.Code, w.Body.String())
	}
	if w := registrarFaltaCorrecao(t, fx, fx.codigoAluno, "2_trimestre", fx.materiaID, d); w.Code != http.StatusCreated {
		t.Fatalf("2 status=%d %s", w.Code, w.Body.String())
	}
}
func TestFaltasPeriodo12BackfillMigracaoPreservaHistoricaSemPeriodo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	id1, id2 := uuid.NewString(), uuid.NewString()
	_, err := fx.client.DB().Exec(`INSERT INTO projection_faltas (id,codigo_estudante,codigo_academia,ano_lectivo,ano_academico,periodo,data,materia_disciplinar_id,quantidade,registered_at,event_id,version) VALUES ($1,$2,$3,'2026','1_ano_superior',NULL,'2026-02-18',$4,1,CURRENT_TIMESTAMP,'x',1),($5,$2,$3,'2026','1_ano_fundamental',NULL,'2026-02-19',$6,1,CURRENT_TIMESTAMP,'y',1)`, id1, fx.codigoAlunoSuperior, fx.academia.CodigoAcademia, fx.materiaSuperiorID.String(), id2, fx.materiaID.String())
	if err != nil {
		t.Fatal(err)
	}
	_, err = fx.client.DB().Exec(`UPDATE projection_faltas f SET periodo = m.periodo FROM projection_materias m WHERE f.materia_disciplinar_id::uuid = m.id AND m.type = 'superior' AND m.periodo IS NOT NULL AND f.periodo IS NULL`)
	if err != nil {
		t.Fatal(err)
	}
	dtos, err := projections.NewFaltasProjection(fx.client).GetByEstudante(fx.codigoAlunoSuperior)
	if err != nil || len(dtos) == 0 {
		t.Fatalf("dtos=%v err=%v", len(dtos), err)
	}
}
func TestFaltasPeriodo13GetByPeriodoMantemIntervaloEPeriodo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	dtos, err := projections.NewFaltasProjection(fx.client).GetByPeriodo(fx.codigoAluno, "2026", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC))
	if err != nil || len(dtos) == 0 || dtos[0].Periodo == "" {
		t.Fatalf("dtos=%v err=%v", len(dtos), err)
	}
}
func TestFaltasPeriodo15HistoricaSemPeriodoListavelECorrigivel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	if _, err := fx.client.DB().Exec(`UPDATE projection_faltas SET periodo=NULL WHERE id=$1`, fx.faltaID); err != nil {
		t.Fatal(err)
	}
	w := requestCorrecao(fx.router, fx.token, http.MethodGet, "/faltas-estudante/"+fx.codigoAluno, "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"periodo":""`) {
		t.Fatalf("list status=%d %s", w.Code, w.Body.String())
	}
	w = requestCorrecao(fx.router, fx.token, http.MethodPatch, "/academia/faltas-aluno/"+fx.faltaID, `{"quantidade":1,"motivo":"ajuste historico"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("patch status=%d %s", w.Code, w.Body.String())
	}
}
