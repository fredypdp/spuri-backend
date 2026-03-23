package handlers

import (
	"fmt"
	"log"
	"net/http"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RegistrarAprovacaoAno(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string  `json:"codigo_estudante"  binding:"required"`
		TipoEnsino      string  `json:"tipo_ensino"       binding:"required"`
		NivelAtual      string  `json:"nivel_atual"       binding:"required"`
		ProximoNivel    *string `json:"proximo_nivel"`
		Aprovado        bool    `json:"aprovado"`
		Observacao      *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"campos obrigatórios: codigo_estudante, tipo_ensino, nivel_atual",
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

	// Validar formato do nivel_atual segundo o tipo de ensino.
	//
	// Fundamental: formato fixo [1-9]_ano_fundamental (ex.: 1_ano_fundamental).
	// Médio/Superior: formato dinâmico [n]_ano_medio ou [n]_ano_superior,
	//   validado contra os anos_academicos do curso vinculado ao estudante
	//   (verificação feita mais abaixo, após carregar estudante + curso).
	switch req.TipoEnsino {
	case "fundamental":
		if err := utils.ValidateAnoFundamental(req.NivelAtual); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_atual inválido: %w", err))
			return
		}
		if req.ProximoNivel != nil {
			if err := utils.ValidateAnoFundamental(*req.ProximoNivel); err != nil {
				utils.RespondWithValidationError(c, fmt.Errorf("proximo_nivel inválido: %w", err))
				return
			}
		}
	case "medio":
		if err := utils.ValidateAnoMedio(req.NivelAtual); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_atual inválido: %w", err))
			return
		}
		if req.ProximoNivel != nil {
			if err := utils.ValidateAnoMedio(*req.ProximoNivel); err != nil {
				utils.RespondWithValidationError(c, fmt.Errorf("proximo_nivel inválido: %w", err))
				return
			}
		}
	case "superior":
		if err := utils.ValidateAnoSuperior(req.NivelAtual); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_atual inválido: %w", err))
			return
		}
		if req.ProximoNivel != nil {
			if err := utils.ValidateAnoSuperior(*req.ProximoNivel); err != nil {
				utils.RespondWithValidationError(c, fmt.Errorf("proximo_nivel inválido: %w", err))
				return
			}
		}
	}

	if req.Aprovado && req.ProximoNivel == nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"proximo_nivel é obrigatório quando aprovado=true",
		))
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

	// ── Validação dos níveis contra os anos do curso (médio e superior) ───────
	//
	// Para fundamental, os anos são definidos na academia (AnosAcademicos).
	// Para médio e superior, os anos são definidos no curso vinculado ao estudante.
	// Em ambos os casos, nivel_atual e proximo_nivel devem pertencer à lista
	// configurada — não basta ter o formato correto.
	switch req.TipoEnsino {
	case "fundamental":
		if err := validarNivelContraAcademia(
			req.NivelAtual, req.ProximoNivel, req.Aprovado,
			academiaDTO.AnosAcademicos,
		); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}

	case "medio":
		if estudanteDTO.CursoMedioID == nil {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"estudante não possui curso médio vinculado",
			))
			return
		}
		cursoMedioUUID, err := uuid.Parse(*estudanteDTO.CursoMedioID)
		if err != nil {
			utils.RespondWithInternalError(c, fmt.Errorf("curso_medio_id do estudante é inválido"))
			return
		}
		cursosProj := getCursosProjection(c)
		cursoDTO, err := cursosProj.GetByID(cursoMedioUUID)
		if err != nil || cursoDTO == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso médio do estudante não encontrado"))
			return
		}
		if err := validarNivelContraCurso(
			req.NivelAtual, req.ProximoNivel, req.Aprovado,
			cursoDTO.AnosAcademicos, cursoDTO.Nome,
		); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}

	case "superior":
		if estudanteDTO.CursoSuperiorID == nil {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"estudante não possui curso superior vinculado",
			))
			return
		}
		cursoSuperiorUUID, err := uuid.Parse(*estudanteDTO.CursoSuperiorID)
		if err != nil {
			utils.RespondWithInternalError(c, fmt.Errorf("curso_superior_id do estudante é inválido"))
			return
		}
		cursosProj := getCursosProjection(c)
		cursoDTO, err := cursosProj.GetByID(cursoSuperiorUUID)
		if err != nil || cursoDTO == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso superior do estudante não encontrado"))
			return
		}
		if err := validarNivelContraCurso(
			req.NivelAtual, req.ProximoNivel, req.Aprovado,
			cursoDTO.AnosAcademicos, cursoDTO.Nome,
		); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	// ── Carregar aggregate e executar comando ─────────────────────────────────
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// FIX H4-TRX-03: type assertion protegida.
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := estudante.RegistrarAprovacaoAno(
		academiaDTO.CodigoAcademia,
		anoLectivo,
		req.TipoEnsino,
		req.NivelAtual,
		req.ProximoNivel,
		req.Aprovado,
		req.Observacao,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Persistir ─────────────────────────────────────────────────────────────
	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		log.Printf("❌ [RegistrarAprovacaoAno] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [RegistrarAprovacaoAno] estudante=%s tipo=%s nivel=%s aprovado=%v",
		req.CodigoEstudante, req.TipoEnsino, req.NivelAtual, req.Aprovado)

	c.JSON(http.StatusOK, gin.H{
		"message":          "aprovação registrada com sucesso",
		"codigo_estudante": req.CodigoEstudante,
		"tipo_ensino":      req.TipoEnsino,
		"nivel_atual":      req.NivelAtual,
		"proximo_nivel":    req.ProximoNivel,
		"aprovado":         req.Aprovado,
	})
}

// ============================================================================
// Helpers internos
// ============================================================================

// validarNivelContraAcademia valida nivel_atual e proximo_nivel contra
// os anos_academicos configurados na academia (ensino fundamental).
// A lógica é idêntica a validarNiveisFundamental em avaliacao_final_handler.go.
func validarNivelContraAcademia(
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
		return fmt.Errorf(
			"nivel_atual '%s' não pertence aos anos configurados nesta academia. "+
				"Anos disponíveis: %v",
			nivelAtual, anosAcademia,
		)
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
		return fmt.Errorf(
			"proximo_nivel '%s' não pertence aos anos configurados nesta academia. "+
				"Anos disponíveis: %v",
			*proximoNivel, anosAcademia,
		)
	}
	if posProximo <= posAtual {
		return fmt.Errorf(
			"proximo_nivel '%s' deve vir depois de nivel_atual '%s' na lista da academia",
			*proximoNivel, nivelAtual,
		)
	}
	return nil
}

// validarNivelContraCurso valida nivel_atual e proximo_nivel contra
// os AnosAcademicos do curso (médio ou superior).
// A lógica é idêntica a validarNiveisCurso em avaliacao_final_handler.go.
func validarNivelContraCurso(
	nivelAtual string,
	proximoNivel *string,
	aprovado bool,
	anosAcademicos []string,
	nomeC string,
) error {
	if len(anosAcademicos) == 0 {
		return fmt.Errorf("curso '%s' não possui anos_academicos definidos", nomeC)
	}

	posicao := make(map[string]int, len(anosAcademicos))
	for i, n := range anosAcademicos {
		posicao[n] = i
	}

	posAtual, ok := posicao[nivelAtual]
	if !ok {
		return fmt.Errorf(
			"nivel_atual '%s' não pertence ao curso '%s'. Anos disponíveis: %v",
			nivelAtual, nomeC, anosAcademicos,
		)
	}

	if !aprovado {
		return nil
	}

	ultimoIdx := len(anosAcademicos) - 1

	if proximoNivel == nil {
		if posAtual != ultimoIdx {
			return fmt.Errorf(
				"proximo_nivel é obrigatório: '%s' não é o último ano do curso '%s'",
				nivelAtual, nomeC,
			)
		}
		return nil
	}

	posProximo, ok := posicao[*proximoNivel]
	if !ok {
		return fmt.Errorf(
			"proximo_nivel '%s' não pertence ao curso '%s'. Anos disponíveis: %v",
			*proximoNivel, nomeC, anosAcademicos,
		)
	}
	if posProximo <= posAtual {
		return fmt.Errorf(
			"proximo_nivel '%s' deve vir depois de nivel_atual '%s' no curso '%s'",
			*proximoNivel, nivelAtual, nomeC,
		)
	}
	return nil
}