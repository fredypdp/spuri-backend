package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"spuri/internal/jobs"
	"spuri/internal/utils"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// =============================================================================
// Resposta padrão de batch
// =============================================================================

type BatchItemResult struct {
	Index   int         `json:"index"`
	Sucesso bool        `json:"sucesso"`
	Dados   interface{} `json:"dados,omitempty"`
	Erro    string      `json:"erro,omitempty"`
}

type BatchResponse struct {
	Total   int               `json:"total"`
	Sucesso int               `json:"sucesso"`
	Falhas  int               `json:"falhas"`
	Items   []BatchItemResult `json:"items"`
}

func newBatchResponse(items []BatchItemResult) BatchResponse {
	s := 0
	for _, it := range items {
		if it.Sucesso {
			s++
		}
	}
	return BatchResponse{
		Total:   len(items),
		Sucesso: s,
		Falhas:  len(items) - s,
		Items:   items,
	}
}

func batchHTTPStatus(results []BatchItemResult) int {
	falhas := 0
	for _, r := range results {
		if !r.Sucesso {
			falhas++
		}
	}
	switch {
	case falhas == 0:
		return http.StatusOK
	case falhas == len(results):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusMultiStatus
	}
}

func batchErr(index int, err error) BatchItemResult {
	return BatchItemResult{Index: index, Sucesso: false, Erro: err.Error()}
}

// =============================================================================
// POST /academia/estudante/register/async — limite 100
// =============================================================================

func RegisterEstudanteBatch(c *gin.Context) {
	if rejectRemovedJSONFields(c) {
		return
	}
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		registerEstudanteBatchMultipart(c)
		return
	}
	var payload struct {
		ComArquivo *bool                       `json:"com_arquivo"`
		Estudantes []cadastroEstudanteJSONItem `json:"estudantes"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve conter {com_arquivo:boolean, estudantes:[...]}", nil)
		return
	}
	if payload.ComArquivo == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("com_arquivo é obrigatório"))
		return
	}
	if *payload.ComArquivo {
		utils.RespondWithValidationError(c, fmt.Errorf("com_arquivo true exige multipart/form-data"))
		return
	}
	enqueueCadastroEstudanteJSONSemArquivos(c, payload.Estudantes)
}

func enqueueCadastroEstudanteJSONSemArquivos(c *gin.Context, items []cadastroEstudanteJSONItem) {
	if err := validarTamanhoBatch(len(items), 100); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if len(items) == 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("array não pode ser vazio"))
		return
	}
	enqueueAsyncBatchPayload(c, jobs.JobTypeRegisterEstudanteBatch, items, len(items))
}

func registerEstudanteBatchMultipart(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(64 << 20); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("multipart/form-data inválido"))
		return
	}
	if rejectRemovedMultipartFields(c) {
		return
	}
	if strings.TrimSpace(c.PostForm("com_arquivo")) != "true" {
		utils.RespondWithValidationError(c, fmt.Errorf("com_arquivo true é obrigatório para multipart/form-data"))
		return
	}
	estudantesRaw := c.PostForm("estudantes")
	if old, newf, ok := findRemovedJSONFieldString(estudantesRaw); ok {
		respondRemovedField(c, old, newf)
		return
	}
	var items []cadastroEstudanteJSONItem
	if err := json.Unmarshal([]byte(estudantesRaw), &items); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campo estudantes deve ser JSON válido"))
		return
	}
	filesByCodigo := map[string]map[string]uploadedPDF{}
	knownFields := map[string]bool{}
	for _, f := range solicitacaoDocFields {
		knownFields[f] = true
	}
	for field, fhs := range c.Request.MultipartForm.File {
		parts := strings.SplitN(field, ".", 2)
		if len(parts) != 2 || !knownFields[parts[1]] {
			utils.RespondWithValidationError(c, fmt.Errorf("campo de arquivo de lote inválido: %s", field))
			return
		}
		if len(fhs) != 1 {
			utils.RespondWithValidationError(c, fmt.Errorf("arquivo duplicado: %s", field))
			return
		}
		pdf, err := readAndValidatePDF(parts[1], fhs[0])
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		if filesByCodigo[parts[0]] == nil {
			filesByCodigo[parts[0]] = map[string]uploadedPDF{}
		}
		if _, exists := filesByCodigo[parts[0]][parts[1]]; exists {
			utils.RespondWithValidationError(c, fmt.Errorf("arquivo duplicado: %s", field))
			return
		}
		filesByCodigo[parts[0]][parts[1]] = pdf
	}
	if validarArquivosCadastroEstudanteBatch(c, items, filesByCodigo) {
		return
	}
	for i := range items {
		if filesByCodigo[items[i].CodigoTemporario] == nil {
			continue
		}
		items[i].Arquivos = map[string]asyncUploadedPDF{}
		for field, pdf := range filesByCodigo[items[i].CodigoTemporario] {
			items[i].Arquivos[field] = newAsyncUploadedPDF(pdf)
		}
	}
	enqueueCadastroEstudanteComArquivos(c, items)
}

func validarArquivosCadastroEstudanteBatch(c *gin.Context, items []cadastroEstudanteJSONItem, filesByCodigo map[string]map[string]uploadedPDF) bool {
	seen := map[string]bool{}
	for _, item := range items {
		if item.CodigoTemporario == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("codigo_temporario é obrigatório no lote com arquivos"))
			return true
		}
		if seen[item.CodigoTemporario] {
			utils.RespondWithValidationError(c, fmt.Errorf("codigo_temporario duplicado: %s", item.CodigoTemporario))
			return true
		}
		seen[item.CodigoTemporario] = true
	}
	for codigo := range filesByCodigo {
		if !seen[codigo] {
			utils.RespondWithValidationError(c, fmt.Errorf("arquivo órfão para codigo_temporario: %s", codigo))
			return true
		}
	}
	return false
}

func enqueueCadastroEstudanteComArquivos(c *gin.Context, items []cadastroEstudanteJSONItem) {
	if err := validarTamanhoBatch(len(items), 100); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	if len(items) == 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("array não pode ser vazio"))
		return
	}
	enqueueAsyncBatchPayload(c, jobs.JobTypeRegisterEstudanteBatch, items, len(items))
}

func processarCadastroEstudanteBatch(c *gin.Context, items []cadastroEstudanteJSONItem, filesByCodigo map[string]map[string]uploadedPDF, pendenteDocumentos bool) {
	if err := validarTamanhoBatch(len(items), 100); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	seen := map[string]bool{}
	if filesByCodigo != nil {
		for _, item := range items {
			if item.CodigoTemporario == "" {
				utils.RespondWithValidationError(c, fmt.Errorf("codigo_temporario é obrigatório no lote com arquivos"))
				return
			}
			if seen[item.CodigoTemporario] {
				utils.RespondWithValidationError(c, fmt.Errorf("codigo_temporario duplicado: %s", item.CodigoTemporario))
				return
			}
			seen[item.CodigoTemporario] = true
		}
		for codigo := range filesByCodigo {
			if !seen[codigo] {
				utils.RespondWithValidationError(c, fmt.Errorf("arquivo órfão para codigo_temporario: %s", codigo))
				return
			}
		}
	}
	results := make([]BatchItemResult, 0, len(items))
	for i, item := range items {
		req, declaracaoAnoAcademico, err := item.toCadastroRequest()
		if err != nil {
			results = append(results, batchErr(i, err))
			continue
		}
		files := map[string]uploadedPDF{}
		if filesByCodigo != nil {
			files = filesByCodigo[item.CodigoTemporario]
		}
		rc := newFakeContext(c)
		if filesByCodigo != nil {
			rc.Set("permitir_pendencia_documentos_em_falha_storage", true)
		}
		registerEstudantePorAcademiaComRequestModo(rc, req, files, declaracaoAnoAcademico, pendenteDocumentos)
		results = append(results, extractResult(rc, i))
	}
	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// POST /academia/notas-aluno/batch — limite 200
// =============================================================================

// =============================================================================
// POST /academia/faltas-aluno/batch — limite 200
// =============================================================================

// =============================================================================
// POST /academia/avaliacao-final/batch — limite 100
// =============================================================================

func RegistrarAvaliacaoFinalBatch(c *gin.Context) {
	type ReqAvaliacao struct {
		CodigoEstudante     string  `json:"codigo_estudante"`
		TipoEnsino          string  `json:"tipo_ensino"`
		AnoAcademicoAtual   string  `json:"nivel_ano_academico_atual"`
		ProximoAnoAcademico *string `json:"proximo_ano_academico"`
		Aprovado            bool    `json:"aprovado"`
		Observacao          *string `json:"observacao"`
	}
	var reqs []ReqAvaliacao
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array de avaliações", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 100); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		RegistrarAvaliacaoFinal(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// POST /academia/curso/batch — limite 50
// =============================================================================

func CriarCursoBatch(c *gin.Context) {
	type ReqCurso struct {
		Nome           string   `json:"nome"`
		Type           string   `json:"type"`
		AnosAcademicos []string `json:"anos_academicos"`
		Periodos       []string `json:"periodos"`
	}
	var reqs []ReqCurso
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array de cursos", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		CriarCurso(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// PUT /academia/curso/ativar/batch
// PUT /academia/curso/desativar/batch — limite 50
// =============================================================================

func AtivarCursoBatch(c *gin.Context) {
	runIDParamBatch(c, 50, func(rc *gin.Context) { AtivarCurso(rc) })
}

func DesativarCursoBatch(c *gin.Context) {
	runIDParamBatch(c, 50, func(rc *gin.Context) { DesativarCurso(rc) })
}

// =============================================================================
// DELETE /academia/curso/batch — limite 50
// =============================================================================

func DeletarCursoBatch(c *gin.Context) {
	type ReqDeletar struct {
		ID     string `json:"id"`
		Motivo string `json:"motivo"`
	}
	var reqs []ReqDeletar
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
		if req.Motivo != "" {
			setJSONBody(rc, gin.H{"motivo": req.Motivo})
		}
		DeletarCurso(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// POST /academia/materia/batch — limite 100
// =============================================================================

func CriarMateriaBatch(c *gin.Context) {
	type ReqMateria struct {
		Nome           string     `json:"nome"`
		Type           string     `json:"type"`
		AnosAcademicos []string   `json:"anos_academicos"`
		CursoID        *uuid.UUID `json:"curso_id"`
	}
	var reqs []ReqMateria
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array de matérias", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 100); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		CriarMateria(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// PUT /academia/materia/ativar/batch
// PUT /academia/materia/desativar/batch
// DELETE /academia/materia/batch — limite 100
// =============================================================================

func AtivarMateriaBatch(c *gin.Context) {
	runIDParamBatch(c, 100, func(rc *gin.Context) { AtivarMateria(rc) })
}

func DesativarMateriaBatch(c *gin.Context) {
	runIDParamBatch(c, 100, func(rc *gin.Context) { DesativarMateria(rc) })
}

func DeletarMateriaBatch(c *gin.Context) {
	runIDParamBatch(c, 100, func(rc *gin.Context) { DeletarMateria(rc) })
}

// =============================================================================
// POST /academia/turma/batch — limite 50
// =============================================================================

func CriarTurmaBatch(c *gin.Context) {
	type ReqTurma struct {
		CodigoTurma string     `json:"codigo_turma"`
		Nivel       string     `json:"nivel"`
		CursoID     *uuid.UUID `json:"curso_id"`
		Turno       string     `json:"turno"`
	}
	var reqs []ReqTurma
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array de turmas", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		CriarTurma(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// PUT /academia/turma/ativar/batch
// PUT /academia/turma/desativar/batch — limite 50
// =============================================================================

func AtivarTurmaBatch(c *gin.Context) {
	runCodigoParamBatch(c, 50, func(rc *gin.Context) { AtivarTurma(rc) })
}

func DesativarTurmaBatch(c *gin.Context) {
	runCodigoParamBatch(c, 50, func(rc *gin.Context) { DesativarTurma(rc) })
}

// =============================================================================
// DELETE /academia/turma/batch — limite 50
// =============================================================================

func DeletarTurmaBatch(c *gin.Context) {
	type ReqDeletar struct {
		Codigo string `json:"codigo"`
		Motivo string `json:"motivo"`
	}
	var reqs []ReqDeletar
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "codigo", Value: req.Codigo}}
		if req.Motivo != "" {
			setJSONBody(rc, gin.H{"motivo": req.Motivo})
		}
		DeletarTurma(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// POST /academia/turma/estudante/batch — limite 100
// =============================================================================

func AdicionarEstudanteBatch(c *gin.Context) {
	type ReqAdicionar struct {
		CodigoTurma     string `json:"codigo_turma"`
		CodigoEstudante string `json:"codigo_estudante"`
	}
	var reqs []ReqAdicionar
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 100); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		if req.CodigoTurma == "" || req.CodigoEstudante == "" {
			results = append(results, batchErr(i,
				fmt.Errorf("codigo_turma e codigo_estudante são obrigatórios")))
			continue
		}
		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "codigo", Value: req.CodigoTurma}}
		setJSONBody(rc, gin.H{"codigo_estudante": req.CodigoEstudante})
		AdicionarEstudanteATurma(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// DELETE /academia/turma/estudante/batch — limite 100
// =============================================================================

func RemoverEstudanteBatch(c *gin.Context) {
	type ReqRemover struct {
		CodigoTurma     string `json:"codigo_turma"`
		CodigoEstudante string `json:"codigo_estudante"`
	}
	var reqs []ReqRemover
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 100); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		if req.CodigoTurma == "" || req.CodigoEstudante == "" {
			results = append(results, batchErr(i,
				fmt.Errorf("codigo_turma e codigo_estudante são obrigatórios")))
			continue
		}
		rc := newFakeContext(c)
		rc.Params = gin.Params{
			gin.Param{Key: "codigo", Value: req.CodigoTurma},
			gin.Param{Key: "codigoEstudante", Value: req.CodigoEstudante},
		}
		RemoverEstudanteDaTurma(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// POST /dominis/academia/register/batch — limite 50
//
// Permite que um admin registe até 50 academias numa única chamada HTTP.
// Cada item do array segue a mesma estrutura de POST /dominis/academia/register.
// A resposta indica sucesso/falha por item, com o codigo_academia gerado para
// cada academia criada com sucesso.
//
// Requer: Auth admin + role >= gerente
// =============================================================================

func RegisterAcademiaBatch(c *gin.Context) {
	var reqs []RegisterAcademiaRequest
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array de academias", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		RegisterAcademia(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// PUT /dominis/academia/ativar/batch — limite 50
//
// Activa até 50 academias de uma vez.
// Body: [{ "codigo": "LDA20261" }, { "codigo": "BGU20261" }, ...]
//
// Requer: Auth admin + role >= adm
// =============================================================================

func AtivarAcademiaBatch(c *gin.Context) {
	runCodigoParamBatch(c, 50, func(rc *gin.Context) { AtivarAcademia(rc) })
}

// =============================================================================
// PUT /dominis/academia/desativar/batch — limite 50
//
// Desactiva até 50 academias de uma vez.
// O campo motivo é OBRIGATÓRIO por item — sem motivo, o item falha com 400.
//
// Body: [{ "codigo": "LDA20261", "motivo": "encerramento voluntário" }, ...]
//
// Requer: Auth admin + role >= adm
// =============================================================================

func DesativarAcademiaBatch(c *gin.Context) {
	type ReqDesativar struct {
		Codigo         string `json:"codigo"`
		CodigoAcademia string `json:"codigo_academia"`
		Motivo         string `json:"motivo"`
	}
	var reqs []ReqDesativar
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array de {codigo, motivo}", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		req.Codigo = firstNonEmptyTrimmed(req.Codigo, req.CodigoAcademia)
		req.Motivo = strings.TrimSpace(req.Motivo)
		if req.Codigo == "" {
			results = append(results, batchErr(i, fmt.Errorf("codigo é obrigatório")))
			continue
		}
		if req.Motivo == "" {
			results = append(results, batchErr(i, fmt.Errorf("motivo é obrigatório para desativar a academia %q", req.Codigo)))
			continue
		}
		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "codigo", Value: req.Codigo}}
		setJSONBody(rc, gin.H{"motivo": req.Motivo})
		DesativarAcademia(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// Helpers internos
// =============================================================================

func validarTamanhoBatch(n, max int) error {
	if n == 0 {
		return fmt.Errorf("array não pode ser vazio")
	}
	if n > max {
		return fmt.Errorf("máximo de %d itens por batch", max)
	}
	return nil
}

// runIDParamBatch executa fn para cada item com param "id" extraído do body.
// Body esperado: [{ "id": "uuid" }, ...]
func runIDParamBatch(c *gin.Context, max int, fn func(*gin.Context)) {
	type ReqID struct {
		ID string `json:"id"`
	}
	var reqs []ReqID
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array de {id}", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), max); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
		fn(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// runCodigoParamBatch executa fn para cada item com param "codigo" extraído do body.
// Body esperado: [{ "codigo": "string" }, ...]
func runCodigoParamBatch(c *gin.Context, max int, fn func(*gin.Context)) {
	type ReqCodigo struct {
		Codigo         string `json:"codigo"`
		CodigoAcademia string `json:"codigo_academia"`
	}
	var reqs []ReqCodigo
	if err := c.ShouldBindJSON(&reqs); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "body deve ser um array de {codigo}", nil)
		return
	}
	if err := validarTamanhoBatch(len(reqs), max); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		codigo := firstNonEmptyTrimmed(req.Codigo, req.CodigoAcademia)
		if codigo == "" {
			results = append(results, batchErr(i, fmt.Errorf("codigo é obrigatório")))
			continue
		}
		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "codigo", Value: codigo}}
		fn(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
