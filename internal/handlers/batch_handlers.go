package handlers

import (
	"fmt"
	"net/http"
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
// POST /academia/estudante/register/batch — limite 100
// =============================================================================

func RegisterEstudanteBatch(c *gin.Context) {
	var reqs []CadastroEstudanteAcademiaRequest
	if err := c.ShouldBindJSON(&reqs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de estudantes"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 100); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		RegisterEstudantePorAcademia(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// POST /academia/notas-aluno/batch — limite 200
// =============================================================================

func RegistrarNotaBatch(c *gin.Context) {
	type ReqNota struct {
		CodigoEstudante      string  `json:"codigo_estudante"`
		Periodo              string  `json:"periodo"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
		Tipo                 string  `json:"tipo"`
		Categoria            string  `json:"categoria"`
		Nota                 float64 `json:"nota"`
		Observacao           *string `json:"observacao"`
	}
	var reqs []ReqNota
	if err := c.ShouldBindJSON(&reqs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de notas"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 200); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		RegistrarNota(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// PUT /academia/atualizar-nota/batch — limite 200
// =============================================================================

func AtualizarNotaBatch(c *gin.Context) {
	type ReqAtualizar struct {
		ID         string   `json:"id"`
		NotaNova   *float64 `json:"nota_nova"`
		Observacao string   `json:"observacao"`
	}
	var reqs []ReqAtualizar
	if err := c.ShouldBindJSON(&reqs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 200); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		AtualizarNota(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// DELETE /academia/nota/batch — limite 200
// =============================================================================

func DeletarNotaBatch(c *gin.Context) {
	type ReqDeletar struct {
		ID     string `json:"id"`
		Motivo string `json:"motivo"`
	}
	var reqs []ReqDeletar
	if err := c.ShouldBindJSON(&reqs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 200); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
		setJSONBody(rc, gin.H{"motivo": req.Motivo})
		DeletarNota(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// POST /academia/faltas-aluno/batch — limite 200
// =============================================================================

func RegistrarFaltasBatch(c *gin.Context) {
	type ReqFalta struct {
		CodigoEstudante      string     `json:"codigo_estudante"`
		Data                 utils.Date `json:"data"`
		MateriaDisciplinarID string     `json:"materia_disciplinar_id"`
		Quantidade           int        `json:"quantidade"`
		Observacao           *string    `json:"observacao"`
	}
	var reqs []ReqFalta
	if err := c.ShouldBindJSON(&reqs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de faltas"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 200); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		RegistrarFaltas(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// PUT /academia/atualizar-falta/batch — limite 200
// =============================================================================

func AtualizarFaltaBatch(c *gin.Context) {
	type ReqAtualizar struct {
		ID                   string      `json:"id"`
		Data                 *utils.Date `json:"data"`
		MateriaDisciplinarID *string     `json:"materia_disciplinar_id"`
		Quantidade           *int        `json:"quantidade"`
		Observacao           *string     `json:"observacao"`
	}
	var reqs []ReqAtualizar
	if err := c.ShouldBindJSON(&reqs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 200); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		setJSONBody(rc, req)
		AtualizarFalta(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// DELETE /academia/falta/batch — limite 200
// =============================================================================

func DeletarFaltaBatch(c *gin.Context) {
	type ReqDeletar struct {
		ID     string `json:"id"`
		Motivo string `json:"motivo"`
	}
	var reqs []ReqDeletar
	if err := c.ShouldBindJSON(&reqs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 200); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	results := make([]BatchItemResult, 0, len(reqs))
	for i, req := range reqs {
		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "id", Value: req.ID}}
		setJSONBody(rc, gin.H{"motivo": req.Motivo})
		DeletarFalta(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de avaliações"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 100); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de cursos"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de matérias"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 100); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de turmas"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 100); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 100); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de academias"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de {codigo, motivo}"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), 50); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de {id}"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), max); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "body deve ser um array de {codigo}"})
		return
	}
	if err := validarTamanhoBatch(len(reqs), max); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
