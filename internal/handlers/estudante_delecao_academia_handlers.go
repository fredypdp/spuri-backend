package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/utils"
)

// DeletarContaEstudantePorAcademia — Tarefa 98. Permite que a academia
// atualmente vinculada ao estudante delete a conta dele diretamente, sem
// exigir desvinculação prévia (esse é o caminho de autodeleção do próprio
// estudante, ver DeletarContaEstudante) — mas só quando essa mesma academia
// foi a que ORIGINALMENTE cadastrou o estudante no Spuri.
//
// "Cadastrou originalmente" é determinado consultando o primeiro evento do
// ledger do estudante (EstudanteCriadoComVinculo), e não o campo
// CodigoAcademia do estado atual: CodigoAcademia é sobrescrito sempre que o
// estudante é revinculado (ver Estudante.applyEstudanteReintegrado), inclusive
// para uma academia diferente da original (POST
// /estudante/solicitacoes-status/revinculacao/:codigo_academia aceita
// qualquer codigo_academia). Ou seja, uma academia que apenas recebeu o
// estudante por revinculação/transferência NÃO pode deletar a conta dele —
// só a que fez o cadastro (CriarComVinculo) na origem.
func DeletarContaEstudantePorAcademia(c *gin.Context) {
	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Motivo) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}

	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}

	if ctx.Estudante.Status != "ativo" && ctx.Estudante.Status != "pendente_documentos" {
		utils.RespondWithValidationError(c, fmt.Errorf("estudante não está vinculado a esta academia no momento"))
		return
	}

	repository := getRepository(c).WithContext(c.Request.Context())
	historico, err := repository.GetEventHistory(ctx.Estudante.GetID())
	if err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("falha ao consultar histórico do estudante: %w", err))
		return
	}
	if len(historico) == 0 || historico[0].EventType != "EstudanteCriadoComVinculo" {
		utils.RespondWithInternalError(c, fmt.Errorf("histórico do estudante inconsistente: primeiro evento inesperado"))
		return
	}
	var payloadCriacao struct {
		CodigoAcademia string
	}
	if err := json.Unmarshal(historico[0].Payload, &payloadCriacao); err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("falha ao interpretar histórico do estudante: %w", err))
		return
	}
	if payloadCriacao.CodigoAcademia != ctx.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "apenas a academia que cadastrou o estudante no Spuri pode deletar a conta dele")
		return
	}

	if err := ctx.Estudante.DeletarPorAcademia(req.Motivo, ctx.CodigoAcademia, ctx.AcademiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if !salvarEventoEstudante(c, ctx) {
		return
	}

	log.Printf("Estudante deletado pela academia que o cadastrou: %s (academia=%s) - Motivo: %s", ctx.Estudante.CodigoEstudante, ctx.CodigoAcademia, req.Motivo)
	c.JSON(http.StatusOK, gin.H{
		"message":          "conta do estudante deletada com sucesso",
		"codigo_estudante": ctx.Estudante.CodigoEstudante,
	})
}
