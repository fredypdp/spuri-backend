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
		AnoLectivo          string  `json:"ano_lectivo"               binding:"required"`
		TipoEnsino          string  `json:"tipo_ensino"               binding:"required"`
		AnoAcademicoAtual   string  `json:"nivel_ano_academico_atual" binding:"required"`
		ProximoAnoAcademico *string `json:"proximo_ano_academico"`
		Aprovado            bool    `json:"aprovado"`
		Observacao          *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"campos obrigatórios: codigo_estudante, ano_lectivo, tipo_ensino, nivel_ano_academico_atual",
		))
		return
	}

	tiposValidos := map[string]bool{"fundamental": true, "medio": true, "superior": true}
	if !tiposValidos[req.TipoEnsino] {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo_ensino deve ser: fundamental, medio ou superior"))
		return
	}

	if !req.Aprovado && req.ProximoAnoAcademico != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("estudante reprovado não deve ter proximo_ano_academico definido"))
		return
	}

	// ── Academia ──────────────────────────────────────────────────────────────
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
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

	// ── Validação de notas (bloqueia aprovação sem observação de override) ────
	if req.Aprovado && (req.Observacao == nil || strings.TrimSpace(*req.Observacao) == "") {
		if errNota := validarNotasParaAprovacao(
			c,
			req.CodigoEstudante,
			req.AnoLectivo,
			req.TipoEnsino,
			req.AnoAcademicoAtual,
			academiaDTO.CodigoAcademia,
			estudanteDTO.CursoMedioID,
			estudanteDTO.CursoSuperiorID,
		); errNota != nil {
			utils.RespondWithValidationError(c, errNota)
			return
		}
	}

	// ── Validação de níveis ───────────────────────────────────────────────────
	switch req.TipoEnsino {
	case "fundamental":
		if err := validarNiveisFundamental(req.AnoAcademicoAtual, req.ProximoAnoAcademico, req.Aprovado, academiaDTO.AnosAcademicos); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	case "medio":
		if err := validarNiveisCurso(c, estudanteDTO.CursoMedioID, req.AnoAcademicoAtual, req.ProximoAnoAcademico, req.Aprovado); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	case "superior":
		if err := validarNiveisCurso(c, estudanteDTO.CursoSuperiorID, req.AnoAcademicoAtual, req.ProximoAnoAcademico, req.Aprovado); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
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

	// Captura a turma atual ANTES de remover o estudante
	turmaAtual := buscarTurmaAtual(c, req.CodigoEstudante, academiaDTO.CodigoAcademia)

	if err := estudante.RegistrarAvaliacaoFinal(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		req.TipoEnsino,
		req.AnoAcademicoAtual,
		req.ProximoAnoAcademico,
		turmaAtual,
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

	// Remove estudante de TODAS as turmas da academia
	turmasRemovidas := removerEstudanteDeTurmasAtual(c, req.CodigoEstudante, academiaDTO.CodigoAcademia, userID)

	resultado := "reprovado"
	if req.Aprovado {
		if req.ProximoAnoAcademico != nil {
			resultado = fmt.Sprintf("aprovado → %s", *req.ProximoAnoAcademico)
		} else {
			resultado = "aprovado (ciclo finalizado)"
		}
	}

	log.Printf("[avaliacao-final] estudante=%s resultado=%s turma=%v turmas_removidas=%v",
		req.CodigoEstudante, resultado, turmaAtual, turmasRemovidas)

	c.JSON(http.StatusOK, gin.H{
		"message":          "avaliação final registrada com sucesso",
		"codigo_estudante": req.CodigoEstudante,
		"resultado":        resultado,
		"codigo_turma":     turmaAtual,
		"turmas_removidas": turmasRemovidas,
	})
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
	tipoEnsino := c.Query("tipo_ensino")
	avaliacaoProj := getAvaliacaoFinalProjection(c)

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
		c.JSON(http.StatusOK, gin.H{"avaliacoes": avaliacoes, "total": len(avaliacoes)})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		var tp *string
		if tipoEnsino != "" {
			tp = &tipoEnsino
		}
		avaliacoes, err := avaliacaoProj.GetByAcademia(academiaDTO.CodigoAcademia, tp, nil)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"avaliacoes": avaliacoes, "total": len(avaliacoes)})

	default: // admin
		var tp *string
		if tipoEnsino != "" {
			tp = &tipoEnsino
		}
		avaliacoes, err := avaliacaoProj.GetAll(tp, nil)
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
		c.JSON(http.StatusOK, gin.H{"aprovacoes": aprovacoes, "total": len(aprovacoes)})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		aprovacoes, err := avaliacaoProj.GetAprovacoes(academiaDTO.CodigoAcademia)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"aprovacoes": aprovacoes, "total": len(aprovacoes)})

	default: // admin
		aprovacoes, err := avaliacaoProj.GetAprovacoes("")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"aprovacoes": aprovacoes, "total": len(aprovacoes)})
	}
}

// ============================================================================
// GET /reprovacoes
// Estudante → suas próprias reprovações
// Academia  → reprovações dos estudantes da academia
// Admin     → todas as reprovações do sistema
// ============================================================================

func ListarReprovacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	avaliacaoProj := getAvaliacaoFinalProjection(c)

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
		c.JSON(http.StatusOK, gin.H{"reprovacoes": reprovacoes, "total": len(reprovacoes)})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		reprovacoes, err := avaliacaoProj.GetReprovacoes(academiaDTO.CodigoAcademia)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"reprovacoes": reprovacoes, "total": len(reprovacoes)})

	default: // admin
		reprovacoes, err := avaliacaoProj.GetReprovacoes("")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"reprovacoes": reprovacoes, "total": len(reprovacoes)})
	}
}

// ============================================================================
// GET /estudante/minhas-avaliacoes
// ============================================================================

func GetMinhasAvaliacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	avaliacaoProj := getAvaliacaoFinalProjection(c)
	avaliacoes, err := avaliacaoProj.GetByEstudante(estudante.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"avaliacoes": avaliacoes,
		"total":      len(avaliacoes),
	})
}

// ============================================================================
// Helpers internos
// ============================================================================

// buscarTurmaAtual encontra a turma atual do estudante antes de removê-lo.
func buscarTurmaAtual(c *gin.Context, codigoEstudante, codigoAcademia string) *string {
	turmasProj := getTurmasProjection(c)
	turmas, err := turmasProj.ListByAcademia(codigoAcademia)
	if err != nil {
		log.Printf("[avaliacao-final] erro ao buscar turma atual do estudante %s: %v", codigoEstudante, err)
		return nil
	}
	for _, turma := range turmas {
		for _, cod := range turma.Estudantes {
			if cod == codigoEstudante {
				return &turma.CodigoTurma
			}
		}
	}
	return nil
}

// removerEstudanteDeTurmasAtual percorre TODAS as turmas da academia e remove o
// estudante de cada uma em que for encontrado.
func removerEstudanteDeTurmasAtual(
	c *gin.Context,
	codigoEstudante string,
	codigoAcademia string,
	removidoPorID uuid.UUID,
) []string {
	turmasProj := getTurmasProjection(c)
	turmas, err := turmasProj.ListByAcademia(codigoAcademia)
	if err != nil {
		log.Printf("[avaliacao-final] erro ao listar turmas: %v", err)
		return nil
	}

	repository := getRepository(c)
	var removidas []string

	for _, turmaDTO := range turmas {
		encontrado := false
		for _, cod := range turmaDTO.Estudantes {
			if cod == codigoEstudante {
				encontrado = true
				break
			}
		}
		if !encontrado {
			continue
		}

		agg, err := repository.Load(turmaDTO.ID, "Turma")
		if err != nil {
			log.Printf("[avaliacao-final] erro ao carregar turma %s: %v", turmaDTO.CodigoTurma, err)
			continue
		}
		turmaAgg, ok := agg.(*aggregates.Turma)
		if !ok {
			log.Printf("[avaliacao-final] erro ao converter aggregate da turma %s", turmaDTO.CodigoTurma)
			continue
		}
		if err := turmaAgg.RemoverEstudante(codigoEstudante, removidoPorID); err != nil {
			log.Printf("[avaliacao-final] erro ao remover estudante da turma %s: %v", turmaDTO.CodigoTurma, err)
			continue
		}
		auditTurma := db.AuditContext{
			UserID:   removidoPorID.String(),
			UserType: "academia",
			IP:       "",
		}
		if err := repository.SaveWithAudit(turmaAgg, auditTurma); err != nil {
			log.Printf("[avaliacao-final] erro ao salvar turma %s: %v", turmaDTO.CodigoTurma, err)
			continue
		}
		removidas = append(removidas, turmaDTO.CodigoTurma)
	}

	return removidas
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
	var periodosEsperados []string // usado apenas para fundamental e medio
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

		// Filtrar matérias do ano académico atual que tenham periodo definido
		for _, m := range todasMaterias {
			if m.Type != "superior" || m.CursoID == nil || *m.CursoID != *cursoSuperiorID {
				continue
			}
			// Matéria sem período definido: não pode ter nota — pular silenciosamente
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
		// Matéria superior tem período único fixo — usa materia.Periodo
		for _, materia := range materiasFiltradas {
			periodo := *materia.Periodo
			key := notaKey{materia.ID.String(), periodo, categoriaEsperada}
			if !notasExistentes[key] {
				faltando = append(faltando, fmt.Sprintf("matéria '%s' — %s", materia.Nome, periodo))
			}
		}
	} else {
		// Fundamental e médio: valida todos os períodos esperados por matéria
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

// validarNiveisFundamental valida nivel_atual e proximo_nivel contra os anos configurados na academia.
func validarNiveisFundamental(
	nivelAtual string,
	proximoNivel *string,
	aprovado bool,
	anosAcademia []string,
) error {
	if len(anosAcademia) == 0 {
		return fmt.Errorf("academia não possui anos_academicos configurados para o ensino fundamental")
	}

	posicao := make(map[string]int, len(anosAcademia))
	for i, a := range anosAcademia {
		posicao[a] = i
	}

	posAtual, ok := posicao[nivelAtual]
	if !ok {
		return fmt.Errorf("nivel_atual '%s' não pertence aos anos configurados nesta academia", nivelAtual)
	}

	if !aprovado {
		return nil
	}

	ultimoIdx := len(anosAcademia) - 1

	if proximoNivel == nil {
		if posAtual != ultimoIdx {
			return fmt.Errorf(
				"proximo_nivel é obrigatório: '%s' não é o último ano do ciclo fundamental nesta academia",
				nivelAtual,
			)
		}
		return nil
	}

	posProximo, ok := posicao[*proximoNivel]
	if !ok {
		return fmt.Errorf("proximo_nivel '%s' não pertence aos anos configurados nesta academia", *proximoNivel)
	}
	if posProximo <= posAtual {
		return fmt.Errorf(
			"proximo_nivel '%s' deve vir depois de nivel_atual '%s' na lista da academia",
			*proximoNivel, nivelAtual,
		)
	}
	return nil
}

// validarNiveisCurso valida nivel_atual e proximo_nivel contra os AnosAcademicos do curso (médio ou superior).
func validarNiveisCurso(
	c *gin.Context,
	cursoID *uuid.UUID,
	nivelAtual string,
	proximoNivel *string,
	aprovado bool,
) error {
	if cursoID == nil {
		return fmt.Errorf("estudante não possui curso vinculado para este tipo de ensino")
	}

	cursosProj := getCursosProjection(c)
	curso, err := cursosProj.GetByID(*cursoID)
	if err != nil || curso == nil {
		return fmt.Errorf("curso do estudante não encontrado")
	}

	if curso.Status != "ativo" {
		return fmt.Errorf("curso do estudante está inativo")
	}

	if len(curso.AnosAcademicos) == 0 {
		return fmt.Errorf("curso '%s' não possui anos_academicos definidos", curso.Nome)
	}

	posicao := make(map[string]int, len(curso.AnosAcademicos))
	for i, n := range curso.AnosAcademicos {
		posicao[n] = i
	}

	posAtual, ok := posicao[nivelAtual]
	if !ok {
		return fmt.Errorf("nivel_atual '%s' não pertence ao curso '%s'", nivelAtual, curso.Nome)
	}

	if !aprovado {
		return nil
	}

	ultimoIdx := len(curso.AnosAcademicos) - 1

	if proximoNivel == nil {
		if posAtual != ultimoIdx {
			return fmt.Errorf(
				"proximo_nivel é obrigatório: '%s' não é o último ano do curso '%s'",
				nivelAtual, curso.Nome,
			)
		}
		return nil
	}

	posProximo, ok := posicao[*proximoNivel]
	if !ok {
		return fmt.Errorf("proximo_nivel '%s' não pertence ao curso '%s'", *proximoNivel, curso.Nome)
	}
	if posProximo <= posAtual {
		return fmt.Errorf(
			"proximo_nivel '%s' deve vir depois de nivel_atual '%s' no curso '%s'",
			*proximoNivel, nivelAtual, curso.Nome,
		)
	}
	return nil
}