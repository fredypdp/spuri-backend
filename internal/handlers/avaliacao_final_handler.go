package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegistrarAvaliacaoFinal — POST /academia/avaliacao-final
//
// Substitui RegistrarAprovacaoAno. Além de registrar o evento no aggregate:
//   - Aprovado: avança o ano (ou finaliza o ciclo).
//   - Aprovado ou Reprovado: remove o estudante da turma atual da academia.
//
// Validação de notas (apenas ao aprovar sem observacao preenchida):
//   - fundamental: nota_escola nos 3 trimestres de cada matéria do ano atual.
//   - medio:       nota_escola em cada período do curso para cada matéria do ano atual.
//   - superior:    nota_exame em cada período do curso para cada matéria do ano atual.
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

	// ── Validação de níveis (reutiliza funções do aprovacao_handlers.go) ──────
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

	if err := estudante.RegistrarAvaliacaoFinal(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		req.TipoEnsino,
		req.AnoAcademicoAtual,
		req.ProximoAnoAcademico,
		req.Aprovado,
		req.Observacao,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// ── Remove estudante da turma atual (independente de aprovação) ───────────
	turmaRemovidaDe := removerEstudanteDeTurmaAtual(c, req.CodigoEstudante, academiaDTO.CodigoAcademia, userID)

	resultado := "reprovado"
	if req.Aprovado {
		if req.ProximoAnoAcademico != nil {
			resultado = fmt.Sprintf("aprovado → %s", *req.ProximoAnoAcademico)
		} else {
			resultado = "aprovado (ciclo finalizado)"
		}
	}

	log.Printf("[avaliacao-final] estudante=%s resultado=%s turma_removida=%s",
		req.CodigoEstudante, resultado, turmaRemovidaDe)

	c.JSON(http.StatusOK, gin.H{
		"message":          "avaliação final registrada com sucesso",
		"codigo_estudante": req.CodigoEstudante,
		"resultado":        resultado,
		"turma_removida":   turmaRemovidaDe,
	})
}

// GetAvaliacoesFinaisEstudante — GET /avaliacoes-estudante/:codigo
// Substitui o handler antigo, agora usa a projection avaliacao_final.
func GetAvaliacoesFinaisEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		utils.RespondWithForbiddenError(c, "Você só pode visualizar suas próprias avaliações")
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

// GetMinhasAvaliacoes — GET /estudante/minhas-avaliacoes
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

// removerEstudanteDeTurmaAtual percorre todas as turmas da academia e remove o
// estudante da primeira em que for encontrado. Retorna o codigo_turma ou "".
func removerEstudanteDeTurmaAtual(
	c *gin.Context,
	codigoEstudante string,
	codigoAcademia string,
	removidoPorID uuid.UUID,
) string {
	turmasProj := getTurmasProjection(c)
	turmas, err := turmasProj.ListByAcademia(codigoAcademia)
	if err != nil {
		log.Printf("[avaliacao-final] erro ao listar turmas: %v", err)
		return ""
	}

	repository := getRepository(c)

	for _, turmaDTO := range turmas {
		for _, cod := range turmaDTO.Estudantes {
			if cod != codigoEstudante {
				continue
			}
			agg, err := repository.Load(turmaDTO.ID, "Turma")
			if err != nil {
				log.Printf("[avaliacao-final] erro ao carregar turma %s: %v", turmaDTO.CodigoTurma, err)
				return ""
			}
			turmaAgg, ok := agg.(*aggregates.Turma)
			if !ok {
				return ""
			}
			if err := turmaAgg.RemoverEstudante(codigoEstudante, removidoPorID); err != nil {
				log.Printf("[avaliacao-final] erro ao remover estudante da turma: %v", err)
				return ""
			}
			if err := repository.Save(turmaAgg); err != nil {
				log.Printf("[avaliacao-final] erro ao salvar turma: %v", err)
				return ""
			}
			return turmaDTO.CodigoTurma
		}
	}
	return ""
}

// validarNotasParaAprovacao verifica se todas as notas obrigatórias estão
// presentes para o ano letivo e tipo de ensino indicados.
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
		periodosEsperados = cursoDTO.Periodos
		for _, m := range todasMaterias {
			if m.Type != "superior" || m.CursoID == nil || *m.CursoID != *cursoSuperiorID {
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

	// Sem matérias cadastradas — não bloqueia
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
	for _, materia := range materiasFiltradas {
		for _, periodo := range periodosEsperados {
			key := notaKey{materia.ID.String(), periodo, categoriaEsperada}
			if !notasExistentes[key] {
				faltando = append(faltando, fmt.Sprintf("matéria '%s' — %s", materia.Nome, periodo))
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

// ============================================================================
// Helpers de validação de níveis
// Reutilizados por avaliacao_final_handler.go
// ============================================================================

// validarNiveisFundamental valida nivel_atual (e proximo_nivel) contra a lista
// dinâmica de anos que a academia declarou em AnosAcademicos.
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

// validarNiveisCurso valida nivel_atual e proximo_nivel contra AnosAcademicos[]
// do curso vinculado ao estudante (médio ou superior).
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