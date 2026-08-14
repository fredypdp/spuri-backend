package handlers

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
	"strings"
	"time"

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

func parseCadastroEstudanteAsyncDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("data_nascimento deve ser YYYY-MM-DD anterior à data atual")
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("data_nascimento deve ser YYYY-MM-DD anterior à data atual")
	}
	return parsed, nil
}

type asyncUploadedPDF struct {
	Field string `json:"field"`
	Data  []byte `json:"data"`
	Size  int64  `json:"size"`
}

func newAsyncUploadedPDF(pdf uploadedPDF) asyncUploadedPDF {
	return asyncUploadedPDF{Field: pdf.field, Data: pdf.data, Size: pdf.size}
}

func (pdf asyncUploadedPDF) toUploadedPDF(fallbackField string) uploadedPDF {
	field := strings.TrimSpace(pdf.Field)
	if field == "" {
		field = fallbackField
	}
	return uploadedPDF{field: field, data: pdf.Data, size: pdf.Size}
}

type cadastroEstudanteJSONItem struct {
	CodigoTemporario       string                                   `json:"codigo_temporario"`
	Nome                   string                                   `json:"nome"`
	Genero                 string                                   `json:"genero"`
	DataNascimento         string                                   `json:"data_nascimento"`
	Email                  string                                   `json:"email"`
	Telefone               string                                   `json:"telefone"`
	TelefoneEncarregado    string                                   `json:"telefone_encarregado"`
	BilheteIdentidade      string                                   `json:"bilhete_identidade"`
	BilheteEncarregado     string                                   `json:"bilhete_identidade_encarregado"`
	AnoEscolar             string                                   `json:"ano_escolar_fundamental"`
	AnoEscolarMedio        string                                   `json:"ano_escolar_medio"`
	AnoSuperior            string                                   `json:"ano_superior"`
	CursoMedioID           string                                   `json:"curso_medio_id"`
	CursoSuperiorID        string                                   `json:"curso_superior_id"`
	CodigoTurma            string                                   `json:"codigo_turma"`
	DeclaracaoAnoAcademico string                                   `json:"declaracao_ano_academico"`
	Documentos             map[string]aggregates.DocumentoMatricula `json:"documentos"`
	Arquivos               map[string]asyncUploadedPDF              `json:"arquivos,omitempty"`
}

func (item cadastroEstudanteJSONItem) toCadastroRequest() (CadastroEstudanteAcademiaRequest, string, error) {
	dataNascimento, err := parseCadastroEstudanteAsyncDate(item.DataNascimento)
	if err != nil {
		return CadastroEstudanteAcademiaRequest{}, "", err
	}
	return CadastroEstudanteAcademiaRequest{
		Nome:                strings.TrimSpace(item.Nome),
		Genero:              strings.TrimSpace(item.Genero),
		DataNascimento:      dataNascimento,
		Email:               strings.TrimSpace(item.Email),
		Telefone:            strings.TrimSpace(item.Telefone),
		TelefoneEncarregado: strings.TrimSpace(item.TelefoneEncarregado),
		BilheteIdentidade:   strings.TrimSpace(item.BilheteIdentidade),
		BilheteEncarregado:  strings.TrimSpace(item.BilheteEncarregado),
		AnoEscolar:          strings.TrimSpace(item.AnoEscolar),
		AnoEscolarMedio:     strings.TrimSpace(item.AnoEscolarMedio),
		AnoSuperior:         strings.TrimSpace(item.AnoSuperior),
		CursoMedioID:        strings.TrimSpace(item.CursoMedioID),
		CursoSuperiorID:     strings.TrimSpace(item.CursoSuperiorID),
		CodigoTurma:         strings.TrimSpace(item.CodigoTurma),
		Documentos:          item.Documentos,
	}, strings.TrimSpace(item.DeclaracaoAnoAcademico), nil
}

func RegisterEstudantePorAcademiaJobItem(c *gin.Context) {
	var item cadastroEstudanteJSONItem
	if err := bindJobItemWithoutLosingBody(c, &item); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter os mesmos campos textuais do cadastro de estudante", nil)
		return
	}

	req, declaracaoAnoAcademico, err := item.toCadastroRequest()
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	files := map[string]uploadedPDF{}
	for field, pdf := range item.Arquivos {
		files[field] = pdf.toUploadedPDF(field)
	}
	if len(files) > 0 {
		c.Set("permitir_pendencia_documentos_em_falha_storage", true)
	}

	registerEstudantePorAcademiaComRequestModo(c, req, files, declaracaoAnoAcademico, len(files) == 0)
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
