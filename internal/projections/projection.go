package projections

import (
	"context"
	"database/sql"
	"log"
	"spuri/internal/db"

	"github.com/google/uuid"
)

type Projection interface {
	Name() string
	Handle(event db.Event) error
	Rebuild() error
	GetLastProcessedEventID() (int64, error)
	UpdateCheckpoint(eventID int64) error
}

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

func (bp *BaseProjection) GetLastProcessedEventIDByName(name string) (int64, error) {
	var lastID int64
	err := bp.client.DB().QueryRow(
		`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = $1`,
		name,
	).Scan(&lastID)
	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] Nenhum checkpoint encontrado para: %s", name)
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	log.Printf("[DEBUG] LastID: %d para projection: %s", lastID, name)
	return lastID, nil
}

func (bp *BaseProjection) UpdateCheckpointByName(name string, eventID int64) error {
	if eventID < 0 {
		eventID = 0
	}
	_, err := bp.client.DB().Exec(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, name, eventID)
	if err != nil {
		log.Printf("[ERROR] Erro ao atualizar checkpoint para %s: %v", name, err)
	}
	return err
}

// ============================================================================
// Helpers internos
// ============================================================================

func nullOrUUID(u *uuid.UUID) interface{} {
	if u == nil {
		return nil
	}
	return u.String()
}

func nullOrString(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}