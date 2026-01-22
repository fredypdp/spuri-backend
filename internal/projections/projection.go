package projections

import (
	"context"
	"database/sql"
	"spuri/internal/db"
)

// Projection interface para todas as projeções
type Projection interface {
	Name() string
	Handle(event db.Event) error
	Rebuild() error
	GetLastProcessedEventID() (int64, error)
	UpdateCheckpoint(eventID int64) error
}

// BaseProjection fornece métodos comuns para todas projeções
type BaseProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewBaseProjection(client *db.Client) *BaseProjection {
	return &BaseProjection{
		client: client,
		ctx:    context.Background(),
	}
}

// ✅ FIX: Usar Get do sqlx (NÃO cacheia prepared statements)
func (bp *BaseProjection) GetLastProcessedEventIDByName(name string) (int64, error) {
	var lastID int64
	
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`
	
	err := bp.client.DB().Get(&lastID, query, name)
	
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, nil
	}
	
	return lastID, nil
}

// ✅ FIX: Usar Exec do sqlx (NÃO cacheia prepared statements)
func (bp *BaseProjection) UpdateCheckpointByName(name string, eventID int64) error {
	query := `
		INSERT INTO projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed
		) VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`
	
	_, err := bp.client.DB().Exec(query, name, eventID)
	
	return err
}