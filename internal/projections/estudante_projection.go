// ============================================================================
// ARQUIVO: internal/projections/estudante_projection.go
// CORRIGIDO: Remover Context de todas as queries
// ============================================================================

package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/genesisdb"
	"time"

	"github.com/google/uuid"
)

// EstudanteProjection projeção de estudantes
type EstudanteProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

// NewEstudanteProjection cria nova projeção de estudante
func NewEstudanteProjection(client *genesisdb.Client) *EstudanteProjection {
	return &EstudanteProjection{
		client: client,
		ctx:    context.Background(),
	}
}

// Name implementa Projection
func (p *EstudanteProjection) Name() string {
	return "estudantes"
}

// Handle processa um evento
func (p *EstudanteProjection) Handle(event genesisdb.Event) error {
	// Processar apenas eventos de Estudante
	if event.AggregateType != "Estudante" {
		return nil
	}

	switch event.EventType {
	case "EstudanteCriado":
		return p.handleEstudanteCriado(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "VinculoAtualizado":
		return p.handleVinculoAtualizado(event)
	default:
		return nil
	}
}

// Rebuild reconstrói a projeção do zero
func (p *EstudanteProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE aggregate_type = 'Estudante'
		ORDER BY id ASC
	`

	var events []genesisdb.Event
	if err := p.client.DB().Select(&events, query); err != nil {
		return err
	}

	for _, event := range events {
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}

	return nil
}

// GetLastProcessedEventID implementa Projection
func (p *EstudanteProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().Get(&lastID, query, p.Name())
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

// UpdateCheckpoint implementa Projection
func (p *EstudanteProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		UPDATE projection_checkpoints
		SET 
			last_processed_event_id = $1,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = events_processed + 1
		WHERE projection_name = $2
	`

	_, err := p.client.DB().Exec(query, eventID, p.Name())
	return err
}

// clear limpa a projeção
func (p *EstudanteProjection) clear() error {
	query := `TRUNCATE TABLE projection_estudantes CASCADE`
	_, err := p.client.DB().Exec(query)
	return err
}

// Event Handlers

func (p *EstudanteProjection) handleEstudanteCriado(event genesisdb.Event) error {
	log.Printf("🔵 [PROJEÇÃO ESTUDANTE] Processando EstudanteCriado")
	
	var payload struct {
		Nome                  string     `json:"Nome"`
		CodigoEstudante       string     `json:"CodigoEstudante"`
		SenhaHash             string     `json:"SenhaHash"`
		BilheteIdentidade     *string    `json:"BilheteIdentidade"`
		BilheteIdentidadeResp *string    `json:"BilheteIdentidadeResp"`
		AnoEscolar            *string    `json:"AnoEscolar"`
		AnoSuperior           *string    `json:"AnoSuperior"`
		CursoMedio            *string    `json:"CursoMedio"`
		CursoSuperior         *string    `json:"CursoSuperior"`
		StatusEscolar         *string    `json:"StatusEscolar"`
		StatusSuperior        *string    `json:"StatusSuperior"`
		CreatedAt             time.Time  `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.SenhaHash == "" {
		return fmt.Errorf("SenhaHash vazio no evento")
	}
	
	if payload.CodigoEstudante == "" {
		return fmt.Errorf("CodigoEstudante vazio no evento")
	}

	query := `
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, bilhete_identidade, 
			bilhete_identidade_responsavel, codigo_academia, ano_escolar, ano_superior, 
			curso_medio, curso_superior, status_escolar, status_superior, 
			version, created_at, updated_at, last_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (id) DO UPDATE SET
			nome = $2,
			codigo_estudante = $3,
			senha_hash = $4,
			bilhete_identidade = $5,
			bilhete_identidade_responsavel = $6,
			codigo_academia = $7,
			ano_escolar = $8,
			ano_superior = $9,
			curso_medio = $10,
			curso_superior = $11,
			status_escolar = $12,
			status_superior = $13,
			version = $14,
			updated_at = $16,
			last_event_id = $17
	`

	result, err := p.client.DB().Exec(
		query,
		event.AggregateID,
		payload.Nome,
		payload.CodigoEstudante,
		payload.SenhaHash,
		payload.BilheteIdentidade,
		payload.BilheteIdentidadeResp,
		nil, // codigo_academia inicia NULL
		payload.AnoEscolar,
		payload.AnoSuperior,
		payload.CursoMedio,
		payload.CursoSuperior,
		payload.StatusEscolar,
		payload.StatusSuperior,
		event.EventVersion,
		payload.CreatedAt,
		time.Now(),
		event.EventID,
	)

	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("✅ [PROJEÇÃO ESTUDANTE] Salvo com sucesso! (rows: %d)", rowsAffected)

	return nil
}

func (p *EstudanteProjection) handleInscricaoAprovada(event genesisdb.Event) error {
	var payload struct {
		CodigoAcademia string `json:"CodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		UPDATE projection_estudantes
		SET 
			codigo_academia = $1,
			version = $2,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $3,
			total_inscricoes = total_inscricoes + 1
		WHERE id = $4
	`

	_, err := p.client.DB().Exec(
		query,
		payload.CodigoAcademia,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

func (p *EstudanteProjection) handleVinculoAtualizado(event genesisdb.Event) error {
	var payload struct {
		NovoCodigoAcademia string `json:"NovoCodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		UPDATE projection_estudantes
		SET 
			codigo_academia = $1,
			version = $2,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $3
		WHERE id = $4
	`

	_, err := p.client.DB().Exec(
		query,
		payload.NovoCodigoAcademia,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

// Query methods

// GetByID busca estudante por ID na projeção
func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	query := `
		SELECT 
			id, nome, codigo_estudante, senha_hash, bilhete_identidade, 
			bilhete_identidade_responsavel, codigo_academia, ano_escolar, ano_superior, 
			curso_medio, curso_superior, status_escolar, status_superior, 
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE id = $1
	`

	var dto EstudanteDTO
	err := p.client.DB().Get(&dto, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

// GetByCodigo busca estudante por código
func (p *EstudanteProjection) GetByCodigo(codigo string) (*EstudanteDTO, error) {
	log.Printf("🔍 [PROJEÇÃO ESTUDANTE] GetByCodigo: %s", codigo)
	
	query := `
		SELECT 
			id, nome, codigo_estudante, senha_hash, bilhete_identidade, 
			bilhete_identidade_responsavel, codigo_academia, ano_escolar, ano_superior, 
			curso_medio, curso_superior, status_escolar, status_superior, 
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE codigo_estudante = $1
	`

	var dto EstudanteDTO
	err := p.client.DB().Get(&dto, query, codigo)
	if err == sql.ErrNoRows {
		log.Printf("❌ [PROJEÇÃO ESTUDANTE] Não encontrado: %s", codigo)
		return nil, nil
	}
	if err != nil {
		log.Printf("❌ [PROJEÇÃO ESTUDANTE] Erro: %v", err)
		return nil, err
	}

	log.Printf("✅ [PROJEÇÃO ESTUDANTE] Encontrado: %s", dto.Nome)
	return &dto, nil
}

// GetByBilhete busca por bilhete (compatibilidade)
func (p *EstudanteProjection) GetByBilhete(bilhete string) (*EstudanteDTO, error) {
	query := `
		SELECT 
			id, nome, codigo_estudante, senha_hash, bilhete_identidade, 
			bilhete_identidade_responsavel, codigo_academia, ano_escolar, ano_superior, 
			curso_medio, curso_superior, status_escolar, status_superior, 
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE bilhete_identidade = $1 OR bilhete_identidade_responsavel = $1
		LIMIT 1
	`

	var dto EstudanteDTO
	err := p.client.DB().Get(&dto, query, bilhete)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

// EstudanteDTO com CodigoAcademia
type EstudanteDTO struct {
	ID                    uuid.UUID `db:"id" json:"id"`
	Nome                  string    `db:"nome" json:"nome"`
	CodigoEstudante       string    `db:"codigo_estudante" json:"codigo_estudante"`
	SenhaHash             string    `db:"senha_hash" json:"-"`
	BilheteIdentidade     *string   `db:"bilhete_identidade" json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp *string   `db:"bilhete_identidade_responsavel" json:"bilhete_identidade_responsavel,omitempty"`
	CodigoAcademia        *string   `db:"codigo_academia" json:"codigo_academia,omitempty"`
	AnoEscolar            *string   `db:"ano_escolar" json:"ano_escolar,omitempty"`
	AnoSuperior           *string   `db:"ano_superior" json:"ano_superior,omitempty"`
	CursoMedio            *string   `db:"curso_medio" json:"curso_medio,omitempty"`
	CursoSuperior         *string   `db:"curso_superior" json:"curso_superior,omitempty"`
	StatusEscolar         *string   `db:"status_escolar" json:"status_escolar,omitempty"`
	StatusSuperior        *string   `db:"status_superior" json:"status_superior,omitempty"`
	CreatedAt             time.Time `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time `db:"updated_at" json:"updated_at"`
	TotalNotas            int       `db:"total_notas" json:"total_notas"`
	TotalFaltas           int       `db:"total_faltas" json:"total_faltas"`
	TotalInscricoes       int       `db:"total_inscricoes" json:"total_inscricoes"`
	Version               int       `db:"version" json:"version"`
}