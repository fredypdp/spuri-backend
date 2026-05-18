package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// POST /academia/avaliacao-final
// ============================================================================

func RegistrarAvaliacaoFinal(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante     string  `json:"codigo_estudante"          binding:"required"`
		AnoAcademicoAtual   string  `json:"nivel_ano_academico_atual" binding:"required"`
		ProximoAnoAcademico *string `json:"proximo_ano_academico,omitempty"`
		Aprovado            bool    `json:"aprovado"`
		Observacao          *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"campos obrigatórios: codigo_estudante, nivel_ano_academico_atual",
		))
		return
	}

	if req.ProximoAnoAcademico != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("proximo_ano_academico é calculado automaticamente pelo backend e não deve ser enviado"))
		return
	}

	// ── Academia ──────────────────────────────────────────────────────────────
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// Ano letivo obrigatório — bloqueia registro se a academia não tiver configurado
	anoLectivo, err := resolverAnoLetivoAcademia(academiaDTO.AnoLetivo, academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Estudante ─────────────────────────────────────────────────────────────
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}
	tipoEnsino := inferirTipoEnsinoDoEstudante(estudanteDTO)
	switch tipoEnsino {
	case "fundamental":
		if err := utils.ValidateAnoFundamental(req.AnoAcademicoAtual); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_ano_academico_atual inválido: %w", err))
			return
		}
	case "medio":
		if err := utils.ValidateAnoMedio(req.AnoAcademicoAtual); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_ano_academico_atual inválido: %w", err))
			return
		}
	case "superior":
		if err := utils.ValidateAnoSuperior(req.AnoAcademicoAtual); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_ano_academico_atual inválido: %w", err))
			return
		}
	}

	avaliacaoProj := getAvaliacaoFinalProjection(c)
	jaAvaliado, err := avaliacaoProj.ExistsByEstudanteAnoLetivo(
		req.CodigoEstudante,
		academiaDTO.CodigoAcademia,
		anoLectivo,
	)
	if err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao verificar avaliação final existente: %w", err))
		return
	}
	if jaAvaliado {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"avaliação final já registrada para este estudante no ano letivo %s",
			anoLectivo,
		))
		return
	}

	// FIX-COMPILE-02: EstudanteDTO armazena CursoMedioID e CursoSuperiorID como
	// *string (banco persiste UUID como texto). Converter para *uuid.UUID para
	// passar para validarNotasParaAprovacao e calcularProximoAnoCurso.
	var cursoMedioUUID, cursoSuperiorUUID *uuid.UUID
	if estudanteDTO.CursoMedioID != nil {
		if parsed, err := uuid.Parse(*estudanteDTO.CursoMedioID); err == nil {
			cursoMedioUUID = &parsed
		}
	}
	if estudanteDTO.CursoSuperiorID != nil {
		if parsed, err := uuid.Parse(*estudanteDTO.CursoSuperiorID); err == nil {
			cursoSuperiorUUID = &parsed
		}
	}

	// O nível informado deve corresponder ao nível atual do estudante para evitar
	// finalizações indevidas quando um nível incorreto é enviado no payload.
	if err := validarNivelAtualDoEstudante(estudanteDTO, tipoEnsino, req.AnoAcademicoAtual); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Validação de notas (bloqueia aprovação sem observação de override) ────
	if req.Aprovado && (req.Observacao == nil || strings.TrimSpace(*req.Observacao) == "") {
		if errNota := validarNotasParaAprovacao(
			c,
			req.CodigoEstudante,
			anoLectivo,
			tipoEnsino,
			req.AnoAcademicoAtual,
			academiaDTO.CodigoAcademia,
			cursoMedioUUID,
			cursoSuperiorUUID,
		); errNota != nil {
			utils.RespondWithValidationError(c, errNota)
			return
		}
	}

	// ── Cálculo do próximo nível (backend) ────────────────────────────────────
	var proximoAnoAcademico *string
	switch tipoEnsino {
	case "fundamental":
		proximoAnoAcademico, err = calcularProximoAnoFundamental(req.AnoAcademicoAtual, req.Aprovado)
	case "medio":
		proximoAnoAcademico, err = calcularProximoAnoCurso(c, cursoMedioUUID, req.AnoAcademicoAtual, req.Aprovado)
	case "superior":
		proximoAnoAcademico, err = calcularProximoAnoCurso(c, cursoSuperiorUUID, req.AnoAcademicoAtual, req.Aprovado)
	}
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Aggregate ─────────────────────────────────────────────────────────────
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado estudante"))
		return
	}

	// Captura turmas atuais ANTES de registrar avaliação final.
	// A remoção será aplicada de forma determinística na projeção de turmas ao
	// processar este mesmo evento AvaliacaoFinalAnoAcademico.
	turmasAtuais := buscarTurmasDoEstudante(c, req.CodigoEstudante, academiaDTO.CodigoAcademia)
	var turmaAtual *string
	if len(turmasAtuais) > 0 {
		turmaAtual = &turmasAtuais[0]
	}

	if err := estudante.RegistrarAvaliacaoFinal(
		academiaDTO.CodigoAcademia,
		anoLectivo,
		tipoEnsino,
		req.AnoAcademicoAtual,
		proximoAnoAcademico,
		turmaAtual,
		turmasAtuais,
		req.Aprovado,
		req.Observacao,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	resultado := "reprovado"
	if req.Aprovado {
		if proximoAnoAcademico != nil {
			resultado = fmt.Sprintf("aprovado → %s", *proximoAnoAcademico)
		} else {
			resultado = "aprovado (ciclo finalizado)"
		}
	}

	response := gin.H{
		"message":          "avaliação final registrada com sucesso",
		"tipo_ensino":      tipoEnsino,
		"resultado":        resultado,
		"turmas_removidas": turmasAtuais,
	}
	c.JSON(http.StatusCreated, response)
}

func inferirTipoEnsinoDoEstudante(estudante *projections.EstudanteDTO) string {
	if estudante == nil {
		return "fundamental"
	}
	if estudante.CursoSuperiorID != nil || estudante.AnoSuperior != nil || estudante.StatusSuperior == "em_andamento" {
		return "superior"
	}
	if estudante.CursoMedioID != nil || estudante.AnoEscolarMedio != nil || estudante.StatusEscolarMedio == "em_andamento" {
		return "medio"
	}
	return "fundamental"
}

// ============================================================================
// GET /avaliacoes
// Estudante → suas avaliações
// Academia  → todas da academia (?tipo_ensino=fundamental|medio|superior)
// Admin     → todas do sistema  (?tipo_ensino=...)
// ============================================================================

func ListarAvaliacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	avaliacaoProj := getAvaliacaoFinalProjection(c)
	filtros, err := parseFiltrosAvaliacaoFinal(c)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	switch userType {
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		avaliacoes, err := avaliacaoProj.GetByEstudante(estudante.CodigoEstudante)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		avaliacoes = filtrarAvaliacoesMemoria(avaliacoes, filtros)
		c.JSON(http.StatusOK, gin.H{"avaliacoes": avaliacoes, "total": len(avaliacoes)})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if filtros.CodigoAcademia != nil && *filtros.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "academia só pode consultar os próprios dados")
			return
		}
		filtros.CodigoAcademia = &academiaDTO.CodigoAcademia
		avaliacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, nil, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"avaliacoes": avaliacoes, "total": len(avaliacoes)})

	default: // admin
		if filtros.CodigoTurma != nil && filtros.CodigoAcademia == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("filtro codigo_turma exige codigo_academia para consultas admin"))
			return
		}
		avaliacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, nil, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"avaliacoes": avaliacoes, "total": len(avaliacoes)})
	}
}

// ============================================================================
// GET /avaliacoes-estudante/:codigo
// Apenas academia e admin (middleware.RequireAcademiaOuAdmin aplicado na rota).
// Academia → verifica se o estudante pertence à academia antes de retornar.
// Admin    → acesso irrestrito.
// ============================================================================

func GetAvaliacoesFinaisEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil ||
			*estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
			return
		}
	}

	avaliacaoProj := getAvaliacaoFinalProjection(c)
	avaliacoes, err := avaliacaoProj.GetByEstudante(codigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"avaliacoes":       avaliacoes,
		"total":            len(avaliacoes),
	})
}

// ============================================================================
// GET /aprovacoes
// Estudante → suas próprias aprovações
// Academia  → aprovações dos estudantes da academia
// Admin     → todas as aprovações do sistema
// ============================================================================

func ListarAprovacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	avaliacaoProj := getAvaliacaoFinalProjection(c)
	filtros, err := parseFiltrosAvaliacaoFinal(c)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	aprovado := true

	switch userType {
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		aprovacoes, err := avaliacaoProj.GetAprovacoesByEstudante(estudante.CodigoEstudante)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		aprovacoes = filtrarAvaliacoesMemoria(aprovacoes, filtros)
		c.JSON(http.StatusOK, gin.H{"aprovacoes": aprovacoes, "total": len(aprovacoes)})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if filtros.CodigoAcademia != nil && *filtros.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "academia só pode consultar os próprios dados")
			return
		}
		filtros.CodigoAcademia = &academiaDTO.CodigoAcademia
		aprovacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, &aprovado, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"aprovacoes": aprovacoes, "total": len(aprovacoes)})

	default: // admin
		if filtros.CodigoTurma != nil && filtros.CodigoAcademia == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("filtro codigo_turma exige codigo_academia para consultas admin"))
			return
		}
		aprovacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, &aprovado, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"aprovacoes": aprovacoes, "total": len(aprovacoes)})
	}
}

// ============================================================================
// GET /reprovacoes
// ============================================================================

func ListarReprovacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	avaliacaoProj := getAvaliacaoFinalProjection(c)
	filtros, err := parseFiltrosAvaliacaoFinal(c)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	aprovado := false

	switch userType {
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		reprovacoes, err := avaliacaoProj.GetReprovacoesByEstudante(estudante.CodigoEstudante)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		reprovacoes = filtrarAvaliacoesMemoria(reprovacoes, filtros)
		c.JSON(http.StatusOK, gin.H{"reprovacoes": reprovacoes, "total": len(reprovacoes)})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if filtros.CodigoAcademia != nil && *filtros.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "academia só pode consultar os próprios dados")
			return
		}
		filtros.CodigoAcademia = &academiaDTO.CodigoAcademia
		reprovacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, &aprovado, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"reprovacoes": reprovacoes, "total": len(reprovacoes)})

	default: // admin
		if filtros.CodigoTurma != nil && filtros.CodigoAcademia == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("filtro codigo_turma exige codigo_academia para consultas admin"))
			return
		}
		reprovacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, &aprovado, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"reprovacoes": reprovacoes, "total": len(reprovacoes)})
	}
}

type filtrosAvaliacaoFinal struct {
	TipoEnsino        *string
	AnoLectivo        *string
	AnoAcademicoAtual *string
	CodigoTurma       *string
	CodigoAcademia    *string
}

func parseFiltrosAvaliacaoFinal(c *gin.Context) (filtrosAvaliacaoFinal, error) {
	parse := func(name string) *string {
		value := strings.TrimSpace(c.Query(name))
		if value == "" {
			return nil
		}
		return &value
	}
	f := filtrosAvaliacaoFinal{
		TipoEnsino:        parse("tipo_ensino"),
		AnoLectivo:        parse("ano_letivo"),
		AnoAcademicoAtual: parse("ano_academico_atual"),
		CodigoTurma:       parse("codigo_turma"),
		CodigoAcademia:    parse("codigo_academia"),
	}
	if f.TipoEnsino != nil {
		switch *f.TipoEnsino {
		case "fundamental", "medio", "superior":
		default:
			return f, fmt.Errorf("tipo_ensino deve ser: fundamental, medio ou superior")
		}
	}
	return f, nil
}

func (f filtrosAvaliacaoFinal) toProjectionFilters() projections.AvaliacaoFinalFilters {
	return projections.AvaliacaoFinalFilters{
		TipoEnsino:        f.TipoEnsino,
		AnoLectivo:        f.AnoLectivo,
		AnoAcademicoAtual: f.AnoAcademicoAtual,
		CodigoTurma:       f.CodigoTurma,
	}
}

func filtrarAvaliacoesMemoria(in []projections.AvaliacaoFinalDTO, f filtrosAvaliacaoFinal) []projections.AvaliacaoFinalDTO {
	out := make([]projections.AvaliacaoFinalDTO, 0, len(in))
	for _, a := range in {
		if f.TipoEnsino != nil && a.TipoEnsino != *f.TipoEnsino {
			continue
		}
		if f.AnoLectivo != nil && a.AnoLectivo != *f.AnoLectivo {
			continue
		}
		if f.AnoAcademicoAtual != nil && a.AnoAcademicoAtual != *f.AnoAcademicoAtual {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ============================================================================
// Helpers internos
// ============================================================================

// buscarTurmasDoEstudante retorna todas as turmas atuais do estudante.
func buscarTurmasDoEstudante(c *gin.Context, codigoEstudante, codigoAcademia string) []string {
	turmasProj := getTurmasProjection(c)
	turmas, err := turmasProj.ListByAcademia(codigoAcademia)
	if err != nil {
		log.Printf("[avaliacao-final] erro ao buscar turma atual do estudante %s: %v", codigoEstudante, err)
		return nil
	}
	result := make([]string, 0, 2)
	for _, turma := range turmas {
		for _, cod := range turma.Estudantes {
			if cod == codigoEstudante {
				result = append(result, turma.CodigoTurma)
				break
			}
		}
	}
	return result
}

// validarNotasParaAprovacao verifica se todas as notas obrigatórias estão presentes.
func validarNotasParaAprovacao(
	c *gin.Context,
	codigoEstudante string,
	anoLectivo string,
	tipoEnsino string,
	anoAcademicoAtual string,
	codigoAcademia string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
) error {
	materiasProj := getMateriasProjection(c)
	notasProj := getNotasProjection(c)

	todasMaterias, err := materiasProj.GetByAcademia(codigoAcademia)
	if err != nil {
		return fmt.Errorf("erro ao carregar matérias: %w", err)
	}

	var materiasFiltradas []projections.MateriaDTO
	var periodosEsperados []string
	var categoriaEsperada string

	switch tipoEnsino {
	case "fundamental":
		for _, m := range todasMaterias {
			if m.Type != "fundamental" {
				continue
			}
			for _, a := range m.AnosAcademicos {
				if a == anoAcademicoAtual {
					materiasFiltradas = append(materiasFiltradas, m)
					break
				}
			}
		}
		periodosEsperados = []string{"1_trimestre", "2_trimestre", "3_trimestre"}
		categoriaEsperada = "nota_escola"

	case "medio":
		if cursoMedioID == nil {
			return fmt.Errorf("estudante não possui curso médio vinculado")
		}
		cursosProj := getCursosProjection(c)
		cursoDTO, err := cursosProj.GetByID(*cursoMedioID)
		if err != nil || cursoDTO == nil {
			return fmt.Errorf("curso médio não encontrado")
		}
		if len(cursoDTO.Periodos) > 0 {
			periodosEsperados = cursoDTO.Periodos
		} else {
			periodosEsperados = []string{"1_trimestre", "2_trimestre", "3_trimestre"}
		}
		for _, m := range todasMaterias {
			if m.Type != "medio" || m.CursoID == nil || *m.CursoID != *cursoMedioID {
				continue
			}
			for _, a := range m.AnosAcademicos {
				if a == anoAcademicoAtual {
					materiasFiltradas = append(materiasFiltradas, m)
					break
				}
			}
		}
		categoriaEsperada = "nota_escola"

	case "superior":
		if cursoSuperiorID == nil {
			return fmt.Errorf("estudante não possui curso superior vinculado")
		}
		cursosProj := getCursosProjection(c)
		cursoDTO, err := cursosProj.GetByID(*cursoSuperiorID)
		if err != nil || cursoDTO == nil {
			return fmt.Errorf("curso superior não encontrado")
		}
		if len(cursoDTO.Periodos) == 0 {
			return fmt.Errorf("curso superior não possui períodos configurados")
		}
		for _, m := range todasMaterias {
			if m.Type != "superior" || m.CursoID == nil || *m.CursoID != *cursoSuperiorID {
				continue
			}
			if m.Periodo == nil || *m.Periodo == "" {
				log.Printf("[avaliacao-final] matéria '%s' sem periodo definido — ignorada na validação", m.Nome)
				continue
			}
			for _, a := range m.AnosAcademicos {
				if a == anoAcademicoAtual {
					materiasFiltradas = append(materiasFiltradas, m)
					break
				}
			}
		}
		categoriaEsperada = "nota_exame"
	}

	if len(materiasFiltradas) == 0 {
		return nil
	}

	todasNotas, err := notasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		return fmt.Errorf("erro ao carregar notas: %w", err)
	}

	type notaKey struct{ materiaID, periodo, categoria string }
	notasExistentes := make(map[notaKey]bool)
	for _, n := range todasNotas {
		if n.AnoLectivo == anoLectivo && n.Categoria == categoriaEsperada {
			notasExistentes[notaKey{n.MateriaDisciplinarID, n.Periodo, n.Categoria}] = true
		}
	}

	var faltando []string
	if tipoEnsino == "superior" {
		for _, materia := range materiasFiltradas {
			periodo := *materia.Periodo
			key := notaKey{materia.ID.String(), periodo, categoriaEsperada}
			if !notasExistentes[key] {
				faltando = append(faltando, fmt.Sprintf("matéria '%s' — %s", materia.Nome, periodo))
			}
		}
	} else {
		for _, materia := range materiasFiltradas {
			for _, periodo := range periodosEsperados {
				key := notaKey{materia.ID.String(), periodo, categoriaEsperada}
				if !notasExistentes[key] {
					faltando = append(faltando, fmt.Sprintf("matéria '%s' — %s", materia.Nome, periodo))
				}
			}
		}
	}

	if len(faltando) > 0 {
		return fmt.Errorf(
			"notas de '%s' ausentes: %s. Preencha 'observacao' para forçar aprovação",
			categoriaEsperada,
			strings.Join(faltando, "; "),
		)
	}

	return nil
}

// calcularProximoAnoFundamental calcula o próximo ano na sequência fixa
// 1_ano_fundamental..9_ano_fundamental.
func calcularProximoAnoFundamental(
	nivelAtual string,
	aprovado bool,
) (*string, error) {
	sequenciaFundamental := []string{
		"1_ano_fundamental",
		"2_ano_fundamental",
		"3_ano_fundamental",
		"4_ano_fundamental",
		"5_ano_fundamental",
		"6_ano_fundamental",
		"7_ano_fundamental",
		"8_ano_fundamental",
		"9_ano_fundamental",
	}

	posAtual := -1
	for i, ano := range sequenciaFundamental {
		if ano == nivelAtual {
			posAtual = i
			break
		}
	}
	if posAtual == -1 {
		return nil, fmt.Errorf("nivel_atual '%s' não pertence à sequência fundamental (1_ano_fundamental..9_ano_fundamental)", nivelAtual)
	}

	if !aprovado {
		return nil, nil
	}

	if posAtual == len(sequenciaFundamental)-1 {
		return nil, nil
	}

	proximo := sequenciaFundamental[posAtual+1]
	return &proximo, nil
}

// calcularProximoAnoCurso calcula o próximo ano com base na sequência do curso (médio/superior).

func calcularProximoAnoCurso(
	c *gin.Context,
	cursoID *uuid.UUID,
	nivelAtual string,
	aprovado bool,
) (*string, error) {
	if cursoID == nil {
		return nil, fmt.Errorf("estudante não possui curso vinculado para este tipo de ensino")
	}

	cursosProj := getCursosProjection(c)
	curso, err := cursosProj.GetByID(*cursoID)
	if err != nil || curso == nil {
		return nil, fmt.Errorf("curso do estudante não encontrado")
	}

	if curso.Status != "ativo" {
		return nil, fmt.Errorf("curso do estudante está inativo")
	}

	if len(curso.AnosAcademicos) == 0 {
		return nil, fmt.Errorf("curso '%s' não possui anos_academicos definidos", curso.Nome)
	}

	posAtual := -1
	for i, n := range curso.AnosAcademicos {
		if n == nivelAtual {
			posAtual = i
			break
		}
	}
	if posAtual == -1 {
		return nil, fmt.Errorf("nivel_atual '%s' não pertence ao curso '%s'", nivelAtual, curso.Nome)
	}

	if !aprovado {
		return nil, nil
	}

	if posAtual == len(curso.AnosAcademicos)-1 {
		return nil, nil
	}

	proximo := curso.AnosAcademicos[posAtual+1]
	return &proximo, nil
}

func validarNivelAtualDoEstudante(estudante *projections.EstudanteDTO, tipoEnsino, nivelInformado string) error {
	if estudante == nil {
		return fmt.Errorf("estudante inválido")
	}

	var nivelAtual *string
	switch tipoEnsino {
	case "fundamental":
		nivelAtual = estudante.AnoEscolar
	case "medio":
		nivelAtual = estudante.AnoEscolarMedio
	case "superior":
		nivelAtual = estudante.AnoSuperior
	}

	if nivelAtual == nil || strings.TrimSpace(*nivelAtual) == "" {
		return fmt.Errorf("estudante não possui nível acadêmico atual definido para '%s'", tipoEnsino)
	}
	if *nivelAtual != nivelInformado {
		return fmt.Errorf("nivel_ano_academico_atual incompatível: esperado '%s', recebido '%s'", *nivelAtual, nivelInformado)
	}
	return nil
}
