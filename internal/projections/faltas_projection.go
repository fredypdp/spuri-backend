package projections

import (
	"context"
	"encoding/json"
	"fmt"
	"spuri/internal/genesisdb"
	"time"

	"github.com/google/uuid"
)

// FaltasProjection projeção de faltas
type FaltasProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

// NewFaltasProjection cria nova projeção de faltas
func NewFaltasProjection(client *genesisdb.Client) *FaltasProjection {
	return &FaltasProjection{
		client: client,
		ctx:    context.Background(),
	}
}

// Name implementa Projection
func (p *FaltasProjection) Name() string {
	return "faltas"
}

// Handle processa um evento
func (p *FaltasProjection) Handle(event genesisdb.Event) error {
	if event.EventType != "FaltasRegistradas" {
		return nil
	}

	return p.handleFaltasRegistradas(event)
}

// Rebuild reconstrói a projeção do zero
func (p *FaltasProjection) Rebuild() error {
	// Limpar projeção
	if err := p.clear(); err != nil {
		return err
	}

	// Buscar todos os eventos FaltasRegistradas
	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE event_type = 'FaltasRegistradas'
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
func (p *FaltasProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().GetContext(p.ctx, &lastID, query, p.Name())
	return lastID, err
}

// UpdateCheckpoint implementa Projection
func (p *FaltasProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		UPDATE projection_checkpoints
		SET 
			last_processed_event_id = $1,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = events_processed + 1
		WHERE projection_name = $2
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, eventID, p.Name())
	return err
}

// clear limpa a projeção
func (p *FaltasProjection) clear() error {
	_, err := p.client.DB().ExecContext(p.ctx, `TRUNCATE TABLE projection_faltas CASCADE`)
	return err
}

// handleFaltasRegistradas processa evento FaltasRegistradas
func (p *FaltasProjection) handleFaltasRegistradas(event genesisdb.Event) error {
	var payload struct {
		IDAcademia   uuid.UUID `json:"IDAcademia"`
		AnoLectivo   string    `json:"AnoLectivo"`
		Periodo      string    `json:"Periodo"`
		Materias     []struct {
			Nome   string `json:"Nome"`
			Faltas int    `json:"Faltas"`
		} `json:"Materias"`
		RegisteredAt time.Time `json:"RegisteredAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	// Converter materias para JSONB
	materiasJSON, err := json.Marshal(payload.Materias)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO projection_faltas (
			estudante_id, id_academia, ano_lectivo, periodo,
			materias, registered_at, event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = p.client.DB().ExecContext(
		p.ctx, query,
		event.AggregateID,
		payload.IDAcademia,
		payload.AnoLectivo,
		payload.Periodo,
		materiasJSON,
		payload.RegisteredAt,
		event.EventID,
		event.EventVersion,
	)

	// Atualizar contador no estudante
	if err == nil {
		updateQuery := `
			UPDATE projection_estudantes
			SET total_faltas = total_faltas + 1
			WHERE id = $1
		`
		p.client.DB().ExecContext(p.ctx, updateQuery, event.AggregateID)
	}

	return err
}

// Query methods

// GetByEstudante busca faltas de um estudante
func (p *FaltasProjection) GetByEstudante(estudanteID uuid.UUID) ([]FaltasDTO, error) {
	query := `
		SELECT 
			id, estudante_id, id_academia, ano_lectivo, periodo,
			materias, registered_at, event_id, version
		FROM projection_faltas
		WHERE estudante_id = $1
		ORDER BY registered_at DESC
	`

	rows, err := p.client.DB().QueryContext(p.ctx, query, estudanteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []FaltasDTO
	for rows.Next() {
		var dto FaltasDTO
		var materiasJSON []byte

		if err := rows.Scan(
			&dto.ID,
			&dto.EstudanteID,
			&dto.IDAcademia,
			&dto.AnoLectivo,
			&dto.Periodo,
			&materiasJSON,
			&dto.RegisteredAt,
			&dto.EventID,
			&dto.Version,
		); err != nil {
			return nil, err
		}

		// Deserializar materias
		if err := json.Unmarshal(materiasJSON, &dto.Materias); err != nil {
			return nil, err
		}

		result = append(result, dto)
	}

	return result, nil
}

// FaltasDTO DTO da projeção
type FaltasDTO struct {
	ID           uuid.UUID `json:"id"`
	EstudanteID  uuid.UUID `json:"estudante_id"`
	IDAcademia   uuid.UUID `json:"id_academia"`
	AnoLectivo   string    `json:"ano_lectivo"`
	Periodo      string    `json:"periodo"`
	Materias     []struct {
		Nome   string `json:"nome"`
		Faltas int    `json:"faltas"`
	} `json:"materias"`
	RegisteredAt time.Time `json:"registered_at"`
	EventID      uuid.UUID `json:"event_id"`
	Version      int       `json:"version"`
}