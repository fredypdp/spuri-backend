package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegistrarAprovacaoAno — POST /academia/aprovacao-ano
//
// A academia define explicitamente nivel_atual e, se aprovado, proximo_nivel.
// Regras:
//   - fundamental: validado contra AnosAcademicos da academia (dinâmico)
//   - medio/superior: validado contra Nivel[] do curso vinculado ao estudante
//   - Reprovado + proximo_nivel ≠ nil → erro
//   - Aprovado + proximo_nivel == nil → último ano → status do ciclo = "finalizado"
//   - Reprovação gera evento no ledger (auditável) sem alterar estado do estudante
func RegistrarAprovacaoAno(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string  `json:"codigo_estudante" binding:"required"`
		AnoLectivo      string  `json:"ano_lectivo"      binding:"required"`
		TipoEnsino      string  `json:"tipo_ensino"      binding:"required"` // fundamental | medio | superior
		NivelAtual      string  `json:"nivel_atual"      binding:"required"`
		ProximoNivel    *string `json:"proximo_nivel"`                       // nil = último ano OU reprovado
		Aprovado        bool    `json:"aprovado"`
		Observacao      *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"campos obrigatórios: codigo_estudante, ano_lectivo, tipo_ensino, nivel_atual",
		))
		return
	}

	tiposValidos := map[string]bool{"fundamental": true, "medio": true, "superior": true}
	if !tiposValidos[req.TipoEnsino] {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"tipo_ensino deve ser: fundamental, medio ou superior",
		))
		return
	}

	if !req.Aprovado && req.ProximoNivel != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"estudante reprovado não deve ter proximo_nivel definido",
		))
		return
	}

	// ── Carregar academia ──────────────────────────────────────────────────
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// ── Carregar estudante ─────────────────────────────────────────────────
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
		return
	}

	// ── Validar níveis ─────────────────────────────────────────────────────
	var validationErr error
	switch req.TipoEnsino {
	case "fundamental":
		// Usa a lista dinâmica de anos declarada pela academia
		validationErr = validarNiveisFundamental(
			req.NivelAtual,
			req.ProximoNivel,
			req.Aprovado,
			academiaDTO.AnosAcademicos,
		)
	case "medio":
		validationErr = validarNiveisCurso(c, estudanteDTO.CursoMedioID, req.NivelAtual, req.ProximoNivel, req.Aprovado)
	case "superior":
		validationErr = validarNiveisCurso(c, estudanteDTO.CursoSuperiorID, req.NivelAtual, req.ProximoNivel, req.Aprovado)
	}
	if validationErr != nil {
		utils.RespondWithValidationError(c, validationErr)
		return
	}

	// ── Carregar aggregate e executar comando ──────────────────────────────
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	err = estudante.RegistrarAprovacaoAno(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		req.TipoEnsino,
		req.NivelAtual,
		req.ProximoNivel,
		req.Aprovado,
		req.Observacao,
	)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// ── Montar resposta ────────────────────────────────────────────────────
	resultado := "REPROVADO"
	if req.Aprovado {
		if req.ProximoNivel != nil {
			resultado = fmt.Sprintf("APROVADO → %s", *req.ProximoNivel)
		} else {
			resultado = "APROVADO (ciclo finalizado)"
		}
	}

	log.Printf("[aprovacao] %s | %s | %s → %s",
		req.CodigoEstudante, req.TipoEnsino, req.NivelAtual, resultado)

	c.JSON(http.StatusCreated, gin.H{
		"message":       "decisão registrada com sucesso",
		"estudante":     req.CodigoEstudante,
		"tipo_ensino":   req.TipoEnsino,
		"nivel_atual":   req.NivelAtual,
		"proximo_nivel": req.ProximoNivel,
		"resultado":     resultado,
	})
}

// GetAprovacoesEstudante — GET /admin/estudantes/:codigo/aprovacoes
func GetAprovacoesEstudante(c *gin.Context) {
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
		utils.RespondWithForbiddenError(c, "Você só pode visualizar suas próprias aprovações")
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

	aprovacaoProj := getAprovacaoAnoProjection(c)
	aprovacoes, err := aprovacaoProj.GetByEstudante(codigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"aprovacoes":       aprovacoes,
		"total":            len(aprovacoes),
	})
}

// GetMinhasAprovacoes — GET /estudante/minhas-aprovacoes
func GetMinhasAprovacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	aprovacaoProj := getAprovacaoAnoProjection(c)
	aprovacoes, err := aprovacaoProj.GetByEstudante(estudante.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"aprovacoes": aprovacoes,
		"total":      len(aprovacoes),
	})
}

// ============================================================================
// Helpers de validação de níveis
// ============================================================================

// validarNiveisFundamental valida nivel_atual (e proximo_nivel) contra a lista
// dinâmica de anos que a academia declarou em AnosAcademicos.
//
// Regras:
//   - nivel_atual deve existir na lista da academia.
//   - Reprovado: apenas valida existência — nenhuma mudança de estado esperada.
//   - Aprovado + proximo_nivel != nil: proximo_nivel deve existir na lista E
//     vir depois de nivel_atual na ordem declarada pela academia.
//   - Aprovado + proximo_nivel == nil: nivel_atual deve ser o ÚLTIMO da lista
//     (estudante conclui o ciclo fundamental nesta academia).
func validarNiveisFundamental(
	nivelAtual string,
	proximoNivel *string,
	aprovado bool,
	anosAcademia []string,
) error {
	if len(anosAcademia) == 0 {
		return fmt.Errorf("academia não possui anos_academicos configurados para o ensino fundamental")
	}

	// Indexar posição de cada ano (ordem importa)
	posicao := make(map[string]int, len(anosAcademia))
	for i, a := range anosAcademia {
		posicao[a] = i
	}

	posAtual, ok := posicao[nivelAtual]
	if !ok {
		return fmt.Errorf("nivel_atual '%s' não pertence aos anos configurados nesta academia", nivelAtual)
	}

	if !aprovado {
		// Reprovação: sem avanço — validação de existência já feita acima.
		return nil
	}

	ultimoIdx := len(anosAcademia) - 1

	if proximoNivel == nil {
		// Aprovação no último ano
		if posAtual != ultimoIdx {
			return fmt.Errorf(
				"proximo_nivel é obrigatório: '%s' não é o último ano do ciclo fundamental nesta academia",
				nivelAtual,
			)
		}
		return nil
	}

	// Aprovação com avanço para proximo_nivel
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

// validarNiveisCurso valida nivel_atual e proximo_nivel contra o array Nivel[]
// do curso vinculado ao estudante (médio ou superior).
// Os anos são definidos pela academia ao criar o curso — validação é posicional.
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

	if len(curso.Nivel) == 0 {
		return fmt.Errorf("curso '%s' não possui anos definidos", curso.Nome)
	}

	// Indexar posição de cada ano
	posicao := make(map[string]int, len(curso.Nivel))
	for i, n := range curso.Nivel {
		posicao[n] = i
	}

	posAtual, ok := posicao[nivelAtual]
	if !ok {
		return fmt.Errorf("nivel_atual '%s' não pertence ao curso '%s'", nivelAtual, curso.Nome)
	}

	if !aprovado {
		return nil
	}

	ultimoIdx := len(curso.Nivel) - 1

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