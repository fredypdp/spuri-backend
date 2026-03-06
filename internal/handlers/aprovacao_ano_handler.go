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
)

// RegistrarAprovacaoAno registra a aprovação ou reprovação de um estudante
// em um determinado ano escolar, avançando ou mantendo o nível.
//
// POST /academia/aprovacao-ano
func RegistrarAprovacaoAno(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante string  `json:"codigo_estudante"  binding:"required"`
		AnoLectivo      string  `json:"ano_lectivo"       binding:"required"`
		TipoEnsino      string  `json:"tipo_ensino"       binding:"required"`
		NivelAtual      string  `json:"nivel_atual"       binding:"required"`
		ProximoNivel    *string `json:"proximo_nivel"`
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

	// Aprovado=true exige proximo_nivel (exceto quando é o último ano do ciclo)
	// A validação de negócio é delegada ao aggregate.

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

	// ── Carregar aggregate e executar comando ─────────────────────────────────
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(c.Request.Context(), estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	if err := estudante.RegistrarAprovacaoAno(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
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
	if err := repository.SaveWithAudit(c.Request.Context(), estudante, audit); err != nil {
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