// ============================================================================
// ARQUIVO: internal/projections/faltas_projection.go
// 🔥 CORRIGIDO: GetLastProcessedEventID com tratamento de erro
// ============================================================================

package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/genesisdb"
	"time"

	"github.com/google/uuid"
)

type FaltasProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

func NewFaltasProjection(client *genesisdb.Client) *FaltasProjection {
	return &FaltasProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *FaltasProjection) Name() string {
	return "faltas"
}

func (p *FaltasProjection) Handle(event genesisdb.Event) error {
	if event.EventType != "FaltasRegistradas" {
		return nil
	}

	return p.handleFaltasRegistradas(event)
}

func (p *FaltasProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE event_type = 'FaltasRegistradas'
		ORDER BY id ASC
	`

	rows, err := p.client.DB().QueryContext(p.ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var event genesisdb.Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			return err
		}

		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}

	return rows.Err()
}

// 🔥 CORRIGIDO: Tratamento de sql.ErrNoRows
func (p *FaltasProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().QueryRowContext(p.ctx, query, p.Name()).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

func (p *FaltasProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		INSERT INTO projection_checkpoints (
			projection_name, 
			last_processed_event_id, 
			last_processed_at,
			events_processed
		) VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`

	_, err := p.client.DB().ExecContext(p.ctx, query, p.Name(), eventID)
	return err
}

func (p *FaltasProjection) clear() error {
	_, err := p.client.DB().ExecContext(p.ctx, `TRUNCATE TABLE projection_faltas CASCADE`)
	return err
}

func (p *FaltasProjection) handleFaltasRegistradas(event genesisdb.Event) error {
	var payload struct {
		CodigoAcademia string `json:"CodigoAcademia"`
		AnoLectivo     string `json:"AnoLectivo"`
		Periodo        string `json:"Periodo"`
		Materias       []struct {
			Nome   string `json:"Nome"`
			Faltas int    `json:"Faltas"`
		} `json:"Materias"`
		RegisteredAt time.Time `json:"RegisteredAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	materiasJSON, err := json.Marshal(payload.Materias)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO projection_faltas (
			estudante_id, codigo_academia, ano_lectivo, periodo,
			materias, registered_at, event_id, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = p.client.DB().ExecContext(
		p.ctx, query,
		event.AggregateID,
		payload.CodigoAcademia,
		payload.AnoLectivo,
		payload.Periodo,
		materiasJSON,
		payload.RegisteredAt,
		event.EventID,
		event.EventVersion,
	)

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

func (p *FaltasProjection) GetByEstudante(estudanteID uuid.UUID) ([]FaltasDTO, error) {
	query := `
		SELECT 
			id, estudante_id, codigo_academia, ano_lectivo, periodo,
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

		err := rows.Scan(
			&dto.ID, &dto.EstudanteID, &dto.CodigoAcademia,
			&dto.AnoLectivo, &dto.Periodo, &materiasJSON,
			&dto.RegisteredAt, &dto.EventID, &dto.Version,
		)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(materiasJSON, &dto.Materias); err != nil {
			return nil, err
		}

		result = append(result, dto)
	}

	return result, rows.Err()
}

type FaltasDTO struct {
	ID             uuid.UUID `json:"id"`
	EstudanteID    uuid.UUID `json:"estudante_id"`
	CodigoAcademia string    `json:"codigo_academia"`
	AnoLectivo     string    `json:"ano_lectivo"`
	Periodo        string    `json:"periodo"`
	Materias       []struct {
		Nome   string `json:"nome"`
		Faltas int    `json:"faltas"`
	} `json:"materias"`
	RegisteredAt time.Time `json:"registered_at"`
	EventID      uuid.UUID `json:"event_id"`
	Version      int       `json:"version"`
}