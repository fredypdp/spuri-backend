package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"spuri/internal/utils"
	"strings"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// Adapters para jobs assíncronos (1 item por execução)
//
// Objetivo: todo processamento batch/async reaproveita os handlers normais
// (mesma validação, mesmas regras de domínio e mesma resposta por item).
// ============================================================================

// restoreBody repõe o body original no request sem re-serializar.
//
// BUG ORIGINAL: a versão anterior usava setJSONBody(c, rawBody) que internamente
// chama json.Marshal(rawBody). Quando rawBody é []byte, json.Marshal produz uma
// string base64 em vez do JSON original — double-encoding que corrompe o payload
// para qualquer handler que chame ShouldBindJSON depois.
func restoreBody(c *gin.Context, raw []byte) {
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	if c.Request.Header == nil {
		c.Request.Header = make(http.Header)
	}
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.ContentLength = int64(len(raw))
}

func bindJobItemWithoutLosingBody(c *gin.Context, target interface{}) error {
	if c.Request == nil || c.Request.Body == nil {
		return fmt.Errorf("body ausente")
	}

	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}

	if len(rawBody) == 0 {
		return fmt.Errorf("body ausente")
	}

	// Restaura ANTES de ShouldBindJSON — o reader precisa estar disponível.
	restoreBody(c, rawBody)

	if err := c.ShouldBindJSON(target); err != nil {
		return err
	}

	// ShouldBindJSON consome o stream; restaura novamente para o handler principal.
	restoreBody(c, rawBody)
	return nil
}

func AtivarAcademiaJobItem(c *gin.Context) {
	var req struct {
		Codigo         string `json:"codigo"`
		CodigoAcademia string `json:"codigo_academia"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo}", nil)
		return
	}
	codigo := firstNonEmptyTrimmed(req.Codigo, req.CodigoAcademia)
	if codigo == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "codigo", Value: codigo}}
	AtivarAcademia(c)
}

func DesativarAcademiaJobItem(c *gin.Context) {
	var req struct {
		Codigo         string `json:"codigo"`
		CodigoAcademia string `json:"codigo_academia"`
		Motivo         string `json:"motivo"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo, motivo}", nil)
		return
	}
	codigo := firstNonEmptyTrimmed(req.Codigo, req.CodigoAcademia)
	if codigo == "" || req.Motivo == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo, motivo}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "codigo", Value: codigo}}
	DesativarAcademia(c)
}

func AdicionarEstudanteATurmaJobItem(c *gin.Context) {
	var req struct {
		CodigoTurma     string `json:"codigo_turma"`
		CodigoEstudante string `json:"codigo_estudante"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.CodigoTurma == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma, codigo_estudante}", nil)
		return
	}
	if req.CodigoEstudante == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma, codigo_estudante}", nil)
		return
	}

	c.Params = gin.Params{gin.Param{Key: "codigo", Value: req.CodigoTurma}}
	AdicionarEstudanteATurma(c)
}

func AtualizarDadosAcademiaJobItem(c *gin.Context) {
	AtualizarDadosAcademia(c)
}

func CriarCategoriaNotaJobItem(c *gin.Context) {
	CriarCategoriaNota(c)
}

func DeletarCategoriaNotaJobItem(c *gin.Context) {
	var req struct {
		Codigo string `json:"codigo"`
		Nome   string `json:"nome"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo}", nil)
		return
	}

	codigo := strings.TrimSpace(req.Codigo)
	if codigo == "" {
		codigo = strings.TrimSpace(req.Nome)
	}
	if codigo == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo}", nil)
		return
	}

	c.Params = gin.Params{gin.Param{Key: "codigo", Value: codigo}}
	DeletarCategoriaNota(c)
}

func AtivarCursoJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	AtivarCurso(c)
}

func DesativarCursoJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	DesativarCurso(c)
}

func AtualizarDadosCursoJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	AtualizarDadosCurso(c)
}

func DeletarCursoJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	DeletarCurso(c)
}

func AtivarMateriaJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	AtivarMateria(c)
}

func DesativarMateriaJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	DesativarMateria(c)
}

func AtualizarDadosMateriaJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	AtualizarDadosMateria(c)
}

func DeletarMateriaJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	DeletarMateria(c)
}

func AtivarTurmaJobItem(c *gin.Context) {
	var req struct {
		Codigo      string `json:"codigo"`
		CodigoTurma string `json:"codigo_turma"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma}", nil)
		return
	}
	codigo := firstNonEmptyTrimmed(req.Codigo, req.CodigoTurma)
	if codigo == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "codigo", Value: codigo}}
	AtivarTurma(c)
}

func DesativarTurmaJobItem(c *gin.Context) {
	var req struct {
		Codigo      string `json:"codigo"`
		CodigoTurma string `json:"codigo_turma"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma}", nil)
		return
	}
	codigo := firstNonEmptyTrimmed(req.Codigo, req.CodigoTurma)
	if codigo == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "codigo", Value: codigo}}
	DesativarTurma(c)
}

func AtualizarDadosTurmaJobItem(c *gin.Context) {
	var req struct {
		Codigo      string `json:"codigo"`
		CodigoTurma string `json:"codigo_turma"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma}", nil)
		return
	}
	codigo := firstNonEmptyTrimmed(req.Codigo, req.CodigoTurma)
	if codigo == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "codigo", Value: codigo}}
	AtualizarDadosTurma(c)
}

func DeletarTurmaJobItem(c *gin.Context) {
	var req struct {
		Codigo      string `json:"codigo"`
		CodigoTurma string `json:"codigo_turma"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma}", nil)
		return
	}
	codigo := firstNonEmptyTrimmed(req.Codigo, req.CodigoTurma)
	if codigo == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "codigo", Value: codigo}}
	DeletarTurma(c)
}

func RemoverEstudanteTurmaJobItem(c *gin.Context) {
	var req struct {
		CodigoTurma     string `json:"codigo_turma"`
		CodigoEstudante string `json:"codigo_estudante"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.CodigoTurma == "" || req.CodigoEstudante == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {codigo_turma, codigo_estudante}", nil)
		return
	}
	c.Params = gin.Params{
		gin.Param{Key: "codigo", Value: req.CodigoTurma},
		gin.Param{Key: "codigoEstudante", Value: req.CodigoEstudante},
	}
	RemoverEstudanteDaTurma(c)
}

func AtivarAdminJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	AtivarAdmin(c)
}

func DesativarAdminJobItem(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := bindJobItemWithoutLosingBody(c, &req); err != nil || req.ID == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter ao menos {id}", nil)
		return
	}
	c.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
	DesativarAdmin(c)
}
