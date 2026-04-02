package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// =============================================================================
// Resposta padrão de batch
//
// Todas as rotas /batch retornam este envelope.
// HTTP 200   — todos os itens tiveram sucesso
// HTTP 207   — sucesso parcial (alguns falharam)
// HTTP 422   — todos os itens falharam
//
// Semântica de atomicidade: NÃO há rollback entre itens. Cada item é uma
// operação independente no ledger (Event Sourcing). Se o item 3 falhar,
// os itens 1 e 2 já foram gravados. Esse comportamento é idêntico ao de
// chamar a rota individual N vezes — o cliente deve tratar "falhas parciais"
// re-enviando apenas os itens com sucesso=false.
// =============================================================================

// BatchItemResult descreve o resultado de um único item do batch.
type BatchItemResult struct {
	Index   int         `json:"index"`
	Sucesso bool        `json:"sucesso"`
	Dados   interface{} `json:"dados,omitempty"`
	Erro    string      `json:"erro,omitempty"`
}

// BatchResponse é o envelope retornado por todos os endpoints /batch.
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
// POST /academia/estudante/register/batch
// Limite: 100 estudantes por chamada
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
// POST /academia/notas-aluno/batch
// Limite: 200 notas por chamada
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
// PUT /academia/atualizar-nota/batch
// Limite: 200 correções por chamada
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
// DELETE /academia/nota/batch
// Body: [{ "id": "uuid", "motivo": "string" }, ...]
// Limite: 200 por chamada
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
// POST /academia/faltas-aluno/batch
// Limite: 200 por chamada
// =============================================================================

func RegistrarFaltasBatch(c *gin.Context) {
	type ReqFalta struct {
		CodigoEstudante      string  `json:"codigo_estudante"`
		Data                 string  `json:"data"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
		Quantidade           int     `json:"quantidade"`
		Observacao           *string `json:"observacao"`
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
// PUT /academia/atualizar-falta/batch
// Limite: 200 por chamada
// =============================================================================

func AtualizarFaltaBatch(c *gin.Context) {
	type ReqAtualizar struct {
		ID                   string  `json:"id"`
		Data                 *string `json:"data"`
		MateriaDisciplinarID *string `json:"materia_disciplinar_id"`
		Quantidade           *int    `json:"quantidade"`
		Observacao           *string `json:"observacao"`
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
// DELETE /academia/falta/batch
// Body: [{ "id": "uuid", "motivo": "string" }, ...]
// Limite: 200 por chamada
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
// POST /academia/avaliacao-final/batch
// Limite: 100 por chamada (operação mais pesada — remove estudantes de turmas)
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
// PUT /academia/estudante/status-escolar/batch
//
// Consolida os três endpoints de status escolar num único batch.
// Cada item especifica o tipo ("fundamental" | "medio" | "superior").
//
// Body:
// [
//   { "codigo_estudante": "ABC1234", "tipo": "fundamental", "novo_status": "em_andamento" },
//   { "codigo_estudante": "DEF5678", "tipo": "superior",    "novo_status": "finalizado" }
// ]
// Limite: 100 por chamada
// =============================================================================

func AtualizarStatusEscolarBatch(c *gin.Context) {
	type ReqStatus struct {
		CodigoEstudante string `json:"codigo_estudante"`
		Tipo            string `json:"tipo"` // "fundamental" | "medio" | "superior"
		NovoStatus      string `json:"novo_status"`
	}
	var reqs []ReqStatus
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
		if req.CodigoEstudante == "" {
			results = append(results, batchErr(i, fmt.Errorf("codigo_estudante é obrigatório")))
			continue
		}

		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "codigo", Value: req.CodigoEstudante}}
		setJSONBody(rc, gin.H{"novo_status": req.NovoStatus})

		switch req.Tipo {
		case "fundamental":
			AtualizarStatusEscolarFundamentalHandler(rc)
		case "medio":
			AtualizarStatusEscolarMedioHandler(rc)
		case "superior":
			AtualizarStatusSuperiorHandler(rc)
		default:
			results = append(results, batchErr(i,
				fmt.Errorf("tipo inválido: %q — use fundamental, medio ou superior", req.Tipo)))
			continue
		}
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// POST /academia/curso/batch
// Limite: 50 cursos por chamada
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
// PUT /academia/curso/desativar/batch
// Body: [{ "id": "uuid" }, ...]
// Limite: 50 por chamada
// =============================================================================

func AtivarCursoBatch(c *gin.Context) {
	runIDParamBatch(c, "id", 50, func(rc *gin.Context) { AtivarCurso(rc) })
}

func DesativarCursoBatch(c *gin.Context) {
	runIDParamBatch(c, "id", 50, func(rc *gin.Context) { DesativarCurso(rc) })
}

// =============================================================================
// DELETE /academia/curso/batch
// Body: [{ "id": "uuid", "motivo": "string (opcional)" }, ...]
// Limite: 50 por chamada
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
// POST /academia/materia/batch
// Limite: 100 matérias por chamada
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
// Body: [{ "id": "uuid" }, ...]
// Limite: 100 por chamada
// =============================================================================

func AtivarMateriaBatch(c *gin.Context) {
	runIDParamBatch(c, "id", 100, func(rc *gin.Context) { AtivarMateria(rc) })
}

func DesativarMateriaBatch(c *gin.Context) {
	runIDParamBatch(c, "id", 100, func(rc *gin.Context) { DesativarMateria(rc) })
}

// =============================================================================
// DELETE /academia/materia/batch
// Body: [{ "id": "uuid" }, ...]
// Limite: 100 por chamada
// =============================================================================

func DeletarMateriaBatch(c *gin.Context) {
	runIDParamBatch(c, "id", 100, func(rc *gin.Context) { DeletarMateria(rc) })
}

// =============================================================================
// POST /academia/turma/batch
// Limite: 50 turmas por chamada
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
// PUT /academia/turma/desativar/batch
// Body: [{ "codigo": "string" }, ...]
// Limite: 50 por chamada
// =============================================================================

func AtivarTurmaBatch(c *gin.Context) {
	runCodigoParamBatch(c, 50, func(rc *gin.Context) { AtivarTurma(rc) })
}

func DesativarTurmaBatch(c *gin.Context) {
	runCodigoParamBatch(c, 50, func(rc *gin.Context) { DesativarTurma(rc) })
}

// =============================================================================
// DELETE /academia/turma/batch
// Body: [{ "codigo": "string", "motivo": "string (opcional)" }, ...]
// Limite: 50 por chamada
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
// POST /academia/turma/estudante/batch
//
// Adiciona vários pares (turma, estudante) de uma vez.
// Cada item especifica a turma, permitindo distribuir estudantes por
// turmas diferentes numa única chamada.
//
// Body:
// [
//   { "codigo_turma": "T1", "codigo_estudante": "ABC1234" },
//   { "codigo_turma": "T2", "codigo_estudante": "DEF5678" }
// ]
// Limite: 100 por chamada
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
// DELETE /academia/turma/estudante/batch
//
// Remove vários pares (turma, estudante) de uma vez.
// Body:
// [
//   { "codigo_turma": "T1", "codigo_estudante": "ABC1234" },
//   { "codigo_turma": "T2", "codigo_estudante": "DEF5678" }
// ]
// Limite: 100 por chamada
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
// POST /dominis/academia/register/batch  (admin)
// Limite: 50 academias por chamada
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
// PUT /dominis/academia/ativar/batch  (admin, requer RequireAdm)
// Body: [{ "codigo": "LDA20261" }, ...]
// Limite: 50 por chamada
// =============================================================================

func AtivarAcademiaBatch(c *gin.Context) {
	runCodigoParamBatch(c, 50, func(rc *gin.Context) { AtivarAcademia(rc) })
}

// =============================================================================
// PUT /dominis/academia/desativar/batch  (admin, requer RequireAdm)
// Body: [{ "codigo": "LDA20261", "motivo": "string" }, ...]
// Limite: 50 por chamada
// =============================================================================

func DesativarAcademiaBatch(c *gin.Context) {
	type ReqDesativar struct {
		Codigo string `json:"codigo"`
		Motivo string `json:"motivo"`
	}
	var reqs []ReqDesativar
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
		setJSONBody(rc, gin.H{"motivo": req.Motivo})
		DesativarAcademia(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}

// =============================================================================
// Helpers internos reutilizáveis
// =============================================================================

// validarTamanhoBatch garante que o array tem entre 1 e max itens.
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
		Codigo string `json:"codigo"`
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
		rc := newFakeContext(c)
		rc.Params = gin.Params{gin.Param{Key: "codigo", Value: req.Codigo}}
		fn(rc)
		results = append(results, extractResult(rc, i))
	}

	c.JSON(batchHTTPStatus(results), newBatchResponse(results))
}
