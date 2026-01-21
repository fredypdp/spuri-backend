package projections

import (
	"context"
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

// GetLastProcessedEventID implementação padrão usando QueryRowContext
func (bp *BaseProjection) GetLastProcessedEventIDByName(name string) (int64, error) {
	ctx := context.Background()
	var lastID int64
	
	err := bp.client.DB().QueryRowContext(ctx, `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`, name).Scan(&lastID)
	
	if err != nil {
		// sql.ErrNoRows retorna 0
		return 0, nil
	}
	return lastID, nil
}

// UpdateCheckpoint implementação padrão usando ExecContext
func (bp *BaseProjection) UpdateCheckpointByName(name string, eventID int64) error {
	ctx := context.Background()
	
	_, err := bp.client.DB().ExecContext(ctx, `
		INSERT INTO projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed
		) VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, name, eventID)
	
	return err
}