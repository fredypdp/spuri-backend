package handlers

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// Adapters para jobs assíncronos (1 item por execução)
//
// Objetivo: todo processamento batch/async reaproveita os handlers normais
// (mesma validação, mesmas regras de domínio e mesma resposta por item).
// ============================================================================

func AtivarAcademiaJobItem(c *gin.Context) {
	var req struct {
		Codigo string `json:"codigo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Codigo == "" {
		c.JSON(400, gin.H{"error": "body deve conter {codigo}"})
		return
	}
	c.Params = gin.Params{gin.Param{Key: "codigo", Value: req.Codigo}}
	AtivarAcademia(c)
}

func DesativarAcademiaJobItem(c *gin.Context) {
	var req struct {
		Codigo string `json:"codigo"`
		Motivo string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Codigo == "" || req.Motivo == "" {
		c.JSON(400, gin.H{"error": "body deve conter {codigo, motivo}"})
		return
	}
	c.Params = gin.Params{gin.Param{Key: "codigo", Value: req.Codigo}}
	setJSONBody(c, gin.H{"motivo": req.Motivo})
	DesativarAcademia(c)
}

func AtualizarNotaJobItem(c *gin.Context) {
	var req struct {
		ID         string   `json:"id"`
		NotaNova   *float64 `json:"nota_nova"`
		Observacao string   `json:"observacao"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		c.JSON(400, gin.H{"error": "body deve conter ao menos {id}"})
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	setJSONBody(c, gin.H{
		"nota_nova":  req.NotaNova,
		"observacao": req.Observacao,
	})
	AtualizarNota(c)
}

func DeletarNotaJobItem(c *gin.Context) {
	var req struct {
		ID     string `json:"id"`
		Motivo string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		c.JSON(400, gin.H{"error": "body deve conter ao menos {id}"})
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	setJSONBody(c, gin.H{"motivo": req.Motivo})
	DeletarNota(c)
}

func AtualizarFaltaJobItem(c *gin.Context) {
	var req struct {
		ID           string  `json:"id"`
		Quantidade   *int    `json:"quantidade"`
		Data         *string `json:"data"`
		Observacao   *string `json:"observacao"`
		Justificada  *bool   `json:"justificada"`
		TipoAusencia *string `json:"tipo_ausencia"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		c.JSON(400, gin.H{"error": "body deve conter ao menos {id}"})
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	setJSONBody(c, gin.H{
		"quantidade":    req.Quantidade,
		"data":          req.Data,
		"observacao":    req.Observacao,
		"justificada":   req.Justificada,
		"tipo_ausencia": req.TipoAusencia,
	})
	AtualizarFalta(c)
}

func DeletarFaltaJobItem(c *gin.Context) {
	var req struct {
		ID     string `json:"id"`
		Motivo string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
		c.JSON(400, gin.H{"error": "body deve conter ao menos {id}"})
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	setJSONBody(c, gin.H{"motivo": req.Motivo})
	DeletarFalta(c)
}

func AtualizarStatusEscolarJobItem(c *gin.Context) {
	var req struct {
		CodigoEstudante string `json:"codigo_estudante"`
		Tipo            string `json:"tipo"`
		NovoStatus      string `json:"novo_status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.CodigoEstudante == "" {
		c.JSON(400, gin.H{"error": "body deve conter {codigo_estudante, tipo, novo_status}"})
		return
	}

	c.Params = gin.Params{gin.Param{Key: "codigo", Value: req.CodigoEstudante}}
	setJSONBody(c, gin.H{"novo_status": req.NovoStatus})

	switch req.Tipo {
	case "fundamental":
		AtualizarStatusEscolarFundamentalHandler(c)
	case "medio":
		AtualizarStatusEscolarMedioHandler(c)
	case "superior":
		AtualizarStatusSuperiorHandler(c)
	default:
		c.JSON(400, gin.H{"error": fmt.Sprintf("tipo inválido: %q — use fundamental, medio ou superior", req.Tipo)})
	}
}

func AdicionarEstudanteATurmaJobItem(c *gin.Context) {
	var req struct {
		CodigoTurma     string `json:"codigo_turma"`
		CodigoEstudante string `json:"codigo_estudante"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.CodigoTurma == "" {
		c.JSON(400, gin.H{"error": "body deve conter ao menos {codigo_turma}"})
		return
	}

	c.Params = gin.Params{gin.Param{Key: "codigo", Value: req.CodigoTurma}}
	setJSONBody(c, gin.H{"codigo_estudante": req.CodigoEstudante})
	AdicionarEstudanteATurma(c)
}

