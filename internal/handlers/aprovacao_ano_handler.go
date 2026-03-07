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

var niveisValidosPorTipo = map[string]map[string]bool{
	"fundamental": {
		"1_ano": true, "2_ano": true, "3_ano": true, "4_ano": true,
		"5_ano": true, "6_ano": true, "7_ano": true, "8_ano": true, "9_ano": true,
	},
	"medio": {
		"10_ano": true, "11_ano": true, "12_ano": true, "13_ano": true,
	},
	"superior": {
		"1_ano": true, "2_ano": true, "3_ano": true, "4_ano": true,
		"5_ano": true, "6_ano": true,
	},
}

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

	// FIX H4-APR-02: validar nivel_atual contra o conjunto permitido para o tipo.
	niveisPermitidos := niveisValidosPorTipo[req.TipoEnsino]
	if !niveisPermitidos[req.NivelAtual] {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"nivel_atual %q inválido para tipo_ensino %q",
			req.NivelAtual, req.TipoEnsino,
		))
		return
	}

	// FIX H4-APR-02: validar proximo_nivel quando fornecido.
	if req.ProximoNivel != nil && !niveisPermitidos[*req.ProximoNivel] {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"proximo_nivel %q inválido para tipo_ensino %q",
			*req.ProximoNivel, req.TipoEnsino,
		))
		return
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