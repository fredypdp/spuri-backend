package jobs

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Status representa o estado de um job assíncrono.
type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusDone       Status = "done"
	StatusFailed     Status = "failed"
)

// JobType identifica qual operação o job executa.
type JobType string

const (
	JobTypeRegisterAcademiaBatch        JobType = "register_academia_batch"
	JobTypeAtivarAcademiaBatch          JobType = "ativar_academia_batch"
	JobTypeDesativarAcademiaBatch       JobType = "desativar_academia_batch"
	JobTypeRegisterEstudanteBatch       JobType = "register_estudante_batch"
	JobTypeRegistrarNotaBatch           JobType = "registrar_nota_batch"
	JobTypeAtualizarNotaBatch           JobType = "atualizar_nota_batch"
	JobTypeDeletarNotaBatch             JobType = "deletar_nota_batch"
	JobTypeRegistrarFaltasBatch         JobType = "registrar_faltas_batch"
	JobTypeAtualizarFaltaBatch          JobType = "atualizar_falta_batch"
	JobTypeDeletarFaltaBatch            JobType = "deletar_falta_batch"
	JobTypeRegistrarAvaliacaoFinalBatch JobType = "registrar_avaliacao_final_batch"
	JobTypeAtualizarStatusEscolarBatch  JobType = "atualizar_status_escolar_batch"
	JobTypeCriarCursoBatch              JobType = "criar_curso_batch"
	JobTypeCriarMateriaBatch            JobType = "criar_materia_batch"
	JobTypeCriarTurmaBatch              JobType = "criar_turma_batch"
	JobTypeAdicionarEstudanteBatch      JobType = "adicionar_estudante_batch"
	JobTypeAtualizarDadosAcademiaBatch  JobType = "atualizar_dados_academia_batch"
	JobTypeCriarCategoriaNotaBatch      JobType = "criar_categoria_nota_batch"
	JobTypeAtivarCursoBatch             JobType = "ativar_curso_batch"
	JobTypeDesativarCursoBatch          JobType = "desativar_curso_batch"
	JobTypeAtualizarDadosCursoBatch     JobType = "atualizar_dados_curso_batch"
	JobTypeDeletarCursoBatch            JobType = "deletar_curso_batch"
	JobTypeAtivarMateriaBatch           JobType = "ativar_materia_batch"
	JobTypeDesativarMateriaBatch        JobType = "desativar_materia_batch"
	JobTypeDefinirPeriodoMateriaBatch   JobType = "definir_periodo_materia_batch"
	JobTypeAtualizarDadosMateriaBatch   JobType = "atualizar_dados_materia_batch"
	JobTypeDeletarMateriaBatch          JobType = "deletar_materia_batch"
	JobTypeAtivarTurmaBatch             JobType = "ativar_turma_batch"
	JobTypeDesativarTurmaBatch          JobType = "desativar_turma_batch"
	JobTypeAtualizarDadosTurmaBatch     JobType = "atualizar_dados_turma_batch"
	JobTypeDeletarTurmaBatch            JobType = "deletar_turma_batch"
	JobTypeRemoverEstudanteTurmaBatch   JobType = "remover_estudante_turma_batch"
	JobTypeAtivarAdminBatch             JobType = "ativar_admin_batch"
	JobTypeDesativarAdminBatch          JobType = "desativar_admin_batch"
	JobTypeRebuildProjection            JobType = "rebuild_projection"
)

// ItemResult representa o resultado de um item individual dentro do job.
type ItemResult struct {
	Index   int             `json:"index"`
	Sucesso bool            `json:"sucesso"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Dados   json.RawMessage `json:"dados,omitempty"`
	Erro    string          `json:"erro,omitempty"`
}

// Job representa um trabalho assíncrono no sistema.
type Job struct {
	ID          uuid.UUID       `json:"id"`
	Type        JobType         `json:"type"`
	Status      Status          `json:"status"`
	UserID      uuid.UUID       `json:"user_id"`
	UserType    string          `json:"user_type"`
	Payload     json.RawMessage `json:"payload"` // input serializado
	Results     []ItemResult    `json:"results"` // resultados parciais/finais
	TotalItems  int             `json:"total_items"`
	DoneItems   int             `json:"done_items"`
	FailItems   int             `json:"fail_items"`
	Error       string          `json:"error,omitempty"` // erro fatal (não por item)
	CreatedAt   time.Time       `json:"created_at"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// Progress retorna progresso de 0..100.
func (j *Job) Progress() int {
	if j.TotalItems == 0 {
		return 0
	}
	processed := j.DoneItems + j.FailItems
	return (processed * 100) / j.TotalItems
}

// IsDone retorna true quando o job não pode avançar mais.
func (j *Job) IsDone() bool {
	return j.Status == StatusDone || j.Status == StatusFailed
}

// Summary retorna um resumo leve para polling.
type Summary struct {
	ID          uuid.UUID  `json:"id"`
	Type        JobType    `json:"type"`
	Status      Status     `json:"status"`
	Progress    int        `json:"progress"`
	TotalItems  int        `json:"total_items"`
	DoneItems   int        `json:"done_items"`
	FailItems   int        `json:"fail_items"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (j *Job) ToSummary() Summary {
	return Summary{
		ID:          j.ID,
		Type:        j.Type,
		Status:      j.Status,
		Progress:    j.Progress(),
		TotalItems:  j.TotalItems,
		DoneItems:   j.DoneItems,
		FailItems:   j.FailItems,
		Error:       j.Error,
		CreatedAt:   j.CreatedAt,
		StartedAt:   j.StartedAt,
		CompletedAt: j.CompletedAt,
	}
}
