package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"
)

type cursoPayload struct {
	Nome               string
	NomeInformado      bool
	Type               string
	TypeInformado      bool
	AnosAcademicos     []string
	AnosInformado      bool
	PeriodosQuantidade int
	PeriodosInformado  bool
}

func bindCursoPayload(c *gin.Context, req *cursoPayload) error {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf("dados invalidos")
	}
	for campo := range raw {
		switch campo {
		case "nome", "type", "anos_academicos", "periodos":
		default:
			return fmt.Errorf("campo não suportado em curso: %s", campo)
		}
	}
	if v, ok := raw["nome"]; ok {
		req.NomeInformado = true
		if err := json.Unmarshal(v, &req.Nome); err != nil {
			return fmt.Errorf("nome deve ser string")
		}
	}
	if v, ok := raw["type"]; ok {
		req.TypeInformado = true
		if err := json.Unmarshal(v, &req.Type); err != nil {
			return fmt.Errorf("type deve ser string")
		}
	}
	if v, ok := raw["anos_academicos"]; ok {
		req.AnosInformado = true
		if err := json.Unmarshal(v, &req.AnosAcademicos); err != nil {
			return fmt.Errorf("anos_academicos deve ser uma lista de strings")
		}
	}
	if v, ok := raw["periodos"]; ok {
		req.PeriodosInformado = true
		dec := json.NewDecoder(bytes.NewReader(v))
		dec.DisallowUnknownFields()
		var n int
		if err := dec.Decode(&n); err != nil {
			return fmt.Errorf("periodos deve ser um número inteiro positivo para curso superior")
		}
		req.PeriodosQuantidade = n
	}
	return nil
}

func prepararDadosCursoPorTipo(tipoCurso string, req cursoPayload, criacao bool) ([]string, []string, error) {
	if req.TypeInformado && req.Type != tipoCurso {
		return nil, nil, fmt.Errorf("type do payload não corresponde ao tipo de curso permitido para a academia")
	}
	if tipoCurso == "superior" {
		if req.AnosInformado {
			return nil, nil, fmt.Errorf("anos_academicos não deve ser enviado para curso superior; é calculado automaticamente a partir de periodos")
		}
		if !req.PeriodosInformado {
			return nil, nil, fmt.Errorf("periodos é obrigatório para curso superior")
		}
		return derivarCursoSuperior(req.PeriodosQuantidade)
	}
	if req.PeriodosInformado {
		return nil, nil, fmt.Errorf("periodos numérico é aceito apenas para curso superior")
	}
	if criacao && !req.AnosInformado {
		return nil, nil, fmt.Errorf("anos_academicos é obrigatório")
	}
	return req.AnosAcademicos, nil, nil
}

func prepararAtualizacaoCursoPorTipo(tipoCurso string, req cursoPayload) ([]string, *[]string, error) {
	if req.TypeInformado {
		return nil, nil, fmt.Errorf("type é imutável e não deve ser enviado na edição")
	}
	if !req.NomeInformado && !req.AnosInformado && !req.PeriodosInformado {
		return nil, nil, fmt.Errorf("nenhum campo para atualizar")
	}
	if tipoCurso == "superior" {
		if req.AnosInformado {
			return nil, nil, fmt.Errorf("anos_academicos não deve ser enviado para curso superior; é calculado automaticamente a partir de periodos")
		}
		if req.PeriodosInformado {
			anos, periodos, err := derivarCursoSuperior(req.PeriodosQuantidade)
			if err != nil {
				return nil, nil, err
			}
			return anos, &periodos, nil
		}
		return nil, nil, nil
	}
	if req.AnosInformado {
		if _, _, err := prepararDadosCursoPorTipo(tipoCurso, req, false); err != nil {
			return nil, nil, err
		}
		return req.AnosAcademicos, nil, nil
	}
	if req.PeriodosInformado {
		return nil, nil, fmt.Errorf("periodos numérico é aceito apenas para curso superior")
	}
	return nil, nil, nil
}

func nomeAtualizacao(req cursoPayload) *string {
	if !req.NomeInformado {
		return nil
	}
	nome := req.Nome
	return &nome
}

func derivarCursoSuperior(totalPeriodos int) ([]string, []string, error) {
	if totalPeriodos <= 0 {
		return nil, nil, fmt.Errorf("periodos deve ser um número inteiro positivo")
	}
	periodos := make([]string, totalPeriodos)
	for i := 1; i <= totalPeriodos; i++ {
		periodos[i-1] = fmt.Sprintf("%d_semestre", i)
	}
	totalAnos := (totalPeriodos + 1) / 2
	anos := make([]string, totalAnos)
	for i := 1; i <= totalAnos; i++ {
		anos[i-1] = fmt.Sprintf("%d_ano_superior", i)
	}
	return anos, periodos, nil
}

// ============================================================================
// POST /academia/cursos
// ============================================================================

func CriarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req cursoPayload
	if err := bindCursoPayload(c, &req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if strings.TrimSpace(req.Nome) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nome é obrigatório"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithForbiddenError(c, "Academia inativa não pode criar cursos")
		return
	}

	tipoCurso, err := resolverTipoCurso(academiaDTO.Nivel, academiaDTO.NivelEscolar)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	anosAcademicos, periodos, err := prepararDadosCursoPorTipo(tipoCurso, req, true)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	curso := aggregates.NewCurso()

	if err := curso.Criar(req.Nome, tipoCurso, anosAcademicos, periodos, academiaDTO.CodigoAcademia); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(curso, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso criado: %s - %s (periodos=%v)", req.Nome, curso.ID, curso.Periodos)

	c.JSON(http.StatusCreated, gin.H{
		"message": "curso criado com sucesso",
		"data": gin.H{
			"id":       curso.ID,
			"nome":     curso.Nome,
			"type":     curso.Type,
			"periodos": curso.Periodos,
		},
	})
}

// ============================================================================
// GET /academia/cursos
// ============================================================================

func ListarCursos(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	codigoAcademia := ""

	switch userType {
	case "admin":
		codigoAcademia = c.Query("codigo_academia")
		if codigoAcademia == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia é obrigatório para admin"))
			return
		}
	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		codigoAcademia = academiaDTO.CodigoAcademia
	default:
		codigoAcademia = c.Query("codigo_academia")
		if codigoAcademia == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia é obrigatório"))
			return
		}
	}

	if userType != "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if academiaDTO == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
	}

	cursosProj := getCursosProjection(c)
	cursos, err := cursosProj.GetByAcademia(codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"cursos": cursos,
		"total":  len(cursos),
	})
}

// ============================================================================
// PUT /academia/cursos/:id
// ============================================================================

func AtualizarDadosCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
		return
	}

	// Type não é aceito: o tipo do curso é imutável após a criação.
	var req cursoPayload
	if err := bindCursoPayload(c, &req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "curso nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso, ok := cursoAgg.(*aggregates.Curso)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	novosAnos, novosPeriodos, err := prepararAtualizacaoCursoPorTipo(cursoDTO.Type, req)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := validarEdicaoCursoComEstudantesAtivos(c, cursoDTO, novosAnos, novosPeriodos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := curso.AtualizarDados(nomeAtualizacao(req), novosAnos, novosPeriodos, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(curso, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso atualizado: %s (periodos=%v)", curso.Nome, curso.Periodos)
	c.JSON(http.StatusOK, gin.H{
		"message":         "curso atualizado com sucesso",
		"nome":            curso.Nome,
		"type":            curso.Type,
		"anos_academicos": curso.AnosAcademicos,
		"periodos":        curso.Periodos,
	})
}

func validarEdicaoCursoComEstudantesAtivos(c *gin.Context, curso *projections.CursoDTO, novosAnos []string, novosPeriodos *[]string) error {
	if novosAnos != nil {
		anosRemovidos := valoresRemovidos(curso.AnosAcademicos, novosAnos)
		qtd, err := getEstudanteProjection(c).CountActiveByCursoAndAnos(curso.ID, curso.Type, anosRemovidos)
		if err != nil {
			return err
		}
		if qtd > 0 {
			return fmt.Errorf("não é possível remover anos_academicos %v porque existem %d estudante(s) ativo(s) matriculado(s) nesses anos", anosRemovidos, qtd)
		}
	}

	if curso.Type == "superior" && novosPeriodos != nil {
		periodosRemovidos := valoresRemovidos(curso.Periodos, *novosPeriodos)
		semestresRemovidos := semestresDosPeriodos(periodosRemovidos)
		qtd, err := getEstudanteProjection(c).CountActiveByCursoSuperiorAndSemestres(curso.ID, semestresRemovidos)
		if err != nil {
			return err
		}
		if qtd > 0 {
			return fmt.Errorf("não é possível remover periodos %v porque existem %d estudante(s) ativo(s) matriculado(s) nesses semestres", periodosRemovidos, qtd)
		}
	}

	return nil
}

func valoresRemovidos(atuais, novos []string) []string {
	permitidos := make(map[string]struct{}, len(novos))
	for _, v := range novos {
		permitidos[v] = struct{}{}
	}

	removidos := make([]string, 0)
	for _, v := range atuais {
		if _, ok := permitidos[v]; !ok {
			removidos = append(removidos, v)
		}
	}
	return removidos
}

func semestresDosPeriodos(periodos []string) []int {
	semestres := make([]int, 0, len(periodos))
	for _, periodo := range periodos {
		numero, _, ok := strings.Cut(periodo, "_")
		if !ok {
			continue
		}
		semestre, err := strconv.Atoi(numero)
		if err == nil {
			semestres = append(semestres, semestre)
		}
	}
	return semestres
}

// ============================================================================
// PUT /academia/cursos/:id/ativar
// ============================================================================

func AtivarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso invalido"))
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Curso nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.Ativar(userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(curso, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso ativado: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "curso ativado com sucesso", "nome": curso.Nome})
}

// ============================================================================
// PUT /academia/cursos/:id/desativar
// ============================================================================

func DesativarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso invalido"))
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Curso nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso, ok := cursoAgg.(*aggregates.Curso)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := curso.Desativar(userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(curso, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso desativado: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "curso desativado com sucesso", "nome": curso.Nome})
}

// DeletarCurso remove logicamente um curso da academia.
//
// Regras de negócio (checadas ANTES de delegar ao aggregate):
//  1. Curso deve pertencer à academia autenticada
//  2. Curso deve estar inativo
//  3. Não pode haver estudantes matriculados neste curso
//  4. Não pode haver matérias ativas vinculadas ao curso
//
// Para cursos superiores, matérias inativas vinculadas são deletadas
// em cascata (cada uma emite seu próprio MateriaDeletada no ledger).
//
// Evento CursoDeletado gravado no ledger (auditável).
func DeletarCurso(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
		return
	}

	var req struct {
		Motivo string `json:"motivo"` // opcional, recomendado para auditoria
	}
	_ = c.ShouldBindJSON(&req)

	// ── 1. Verificar propriedade ──────────────────────────────────────────
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "curso não pertence a esta academia")
		return
	}

	// ── 2. Curso deve estar inativo ───────────────────────────────────────
	if cursoDTO.Status == "ativo" {
		utils.RespondWithValidationError(c, fmt.Errorf("desative o curso antes de deletá-lo"))
		return
	}

	// ── 3. Checar estudantes matriculados neste curso ─────────────────────
	estudanteProj := getEstudanteProjection(c)
	estudantesNoCurso, err := estudanteProj.CountByCurso(cursoID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if estudantesNoCurso > 0 {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"não é possível deletar: %d estudante(s) matriculado(s) neste curso",
			estudantesNoCurso,
		))
		return
	}

	// ── 4. Checar matérias vinculadas ─────────────────────────────────────
	materiasProj := getMateriasProjection(c)
	materiasDoCurso, err := materiasProj.GetByCurso(cursoID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Bloquear se houver matérias ATIVAS
	for _, m := range materiasDoCurso {
		if m.Status == "ativo" {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"desative todas as matérias antes de deletar o curso (matéria ativa: %s)",
				m.Nome,
			))
			return
		}
	}

	repository := getRepository(c)

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}

	// ── 5. Deletar matérias inativas em cascata (cada uma emite evento) ───
	// Apenas cursos superiores tendem a ter matérias; médio normalmente não.
	var materiasDeletedNomes []string
	for _, m := range materiasDoCurso {
		if m.Status == "deletado" {
			continue // já deletada anteriormente — skip seguro
		}

		materiaAgg, err := repository.Load(m.ID, "MateriaDisciplinar")
		if err != nil {
			// FIX BUG #3: era `log + continue` — agora retorna erro ao cliente.
			// O continue permitia que o curso fosse deletado com matérias ainda
			// "vivas" no ledger, gerando estado parcialmente inconsistente.
			utils.RespondWithInternalError(c, fmt.Errorf("erro ao carregar matéria '%s': %w", m.Nome, err))
			return
		}

		materia := materiaAgg.(*aggregates.MateriaDisciplinar)
		// FIX: Deletar agora requer (deletadoPor uuid.UUID, motivo string).
		if err := materia.Deletar(academiaID, fmt.Sprintf("cascata: curso %s deletado", cursoDTO.Nome)); err != nil {
			utils.RespondWithInternalError(c, fmt.Errorf("erro ao deletar matéria '%s': %w", m.Nome, err))
			return
		}

		if err := repository.SaveWithAudit(materia, audit); err != nil {
			utils.RespondWithInternalError(c, fmt.Errorf("erro ao salvar deleção da matéria '%s': %w", m.Nome, err))
			return
		}
		materiasDeletedNomes = append(materiasDeletedNomes, m.Nome)
	}

	// ── 6. Deletar turmas vinculadas ao curso (inativas, sem estudantes) ──
	turmasProj := getTurmasProjection(c)
	turmasDoCurso, err := turmasProj.ListByCurso(cursoID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	var turmasDeletedCodigos []string
	for _, t := range turmasDoCurso {
		if t.Status == "deletado" {
			continue // já deletada — skip seguro
		}
		if t.Status == "ativo" {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"desative todas as turmas vinculadas antes de deletar o curso (turma ativa: %s)",
				t.CodigoTurma,
			))
			return
		}
		if len(t.Estudantes) > 0 {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"turma %s ainda possui estudantes vinculados", t.CodigoTurma,
			))
			return
		}
		turmaAgg, err := repository.Load(t.ID, "Turma")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		turma := turmaAgg.(*aggregates.Turma)
		if err := turma.Deletar(academiaID, fmt.Sprintf("cascata: curso %s deletado", cursoDTO.Nome)); err != nil {
			utils.RespondWithInternalError(c, fmt.Errorf("erro ao deletar turma '%s': %w", t.CodigoTurma, err))
			return
		}
		if err := repository.SaveWithAudit(turma, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		turmasDeletedCodigos = append(turmasDeletedCodigos, t.CodigoTurma)
	}

	// ── 7. Deletar o curso em si ──────────────────────────────────────────
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	curso := cursoAgg.(*aggregates.Curso)
	if err := curso.Deletar(academiaID, req.Motivo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.SaveWithAudit(curso, audit); err != nil {
		log.Printf("❌ [DeletarCurso] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [DeletarCurso] curso=%s academia=%s materias_deletadas=%v turmas_deletadas=%v",
		cursoDTO.Nome, academiaDTO.CodigoAcademia, materiasDeletedNomes, turmasDeletedCodigos,
	)

	c.JSON(http.StatusOK, gin.H{
		"message":            "curso deletado com sucesso",
		"curso_id":           cursoID,
		"nome":               cursoDTO.Nome,
		"materias_deletadas": materiasDeletedNomes,
		"turmas_deletadas":   turmasDeletedCodigos,
		"auditavel":          true,
	})
}

// ============================================================================
// PUT /estudante/:codigo/alterar-curso  (rota dentro do grupo /academia)
// ============================================================================

func AlterarCursoEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	var req struct {
		TipoEnsino string    `json:"tipo_ensino" binding:"required"`
		CursoID    uuid.UUID `json:"curso_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo_ensino e curso_id são obrigatórios"))
		return
	}

	if req.TipoEnsino != "medio" && req.TipoEnsino != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo_ensino deve ser 'medio' ou 'superior'"))
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(req.CursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Curso não pertence a esta academia")
		return
	}

	if req.TipoEnsino == "medio" && cursoDTO.Type != "medio" {
		utils.RespondWithValidationError(c, fmt.Errorf("curso não é do tipo médio"))
		return
	}
	if req.TipoEnsino == "superior" && cursoDTO.Type != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("curso não é do tipo superior"))
		return
	}

	repository := getRepository(c)
	aggregate, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante, ok := aggregate.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	if err := estudante.AlterarCurso(req.CursoID, req.TipoEnsino); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "curso alterado com sucesso",
		"codigo_estudante": codigoEstudante,
		"tipo_ensino":      req.TipoEnsino,
		"curso_id":         req.CursoID,
		"curso_nome":       cursoDTO.Nome,
	})
}

// ============================================================================
// Helpers internos
// ============================================================================

func resolverTipoCurso(nivelAcademia string, nivelEscolar *string) (string, error) {
	if nivelAcademia == "superior" {
		return "superior", nil
	}
	if nivelAcademia == "escola" {
		if nivelEscolar == nil || *nivelEscolar != "medio" {
			return "", fmt.Errorf("apenas academias escolares de nível médio podem criar cursos")
		}
		return "medio", nil
	}
	return "", fmt.Errorf("nível de academia inválido para criação de curso")
}

// ============================================================================
// GET /academia/cursos/:id
// ============================================================================

// GetCurso retorna um curso pelo ID.
// Protegido por RequireAcademia — academia só pode ver seus próprios cursos.
func GetCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	if userType == "academia" {
		academiaDTO, _ := academiaProj.GetByID(userID)
		if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "curso não pertence a esta academia")
			return
		}
	}

	c.JSON(http.StatusOK, cursoDTO)
}
