package projections

import (
	"context"
	"database/sql"
	"fmt"
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
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(name))
	
	err := bp.client.DB().QueryRow(query).Scan(&lastID)
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
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, db.SafeString(name), eventID, eventID)
	
	_, err := bp.client.DB().Exec(query)
	if err != nil {
		log.Printf("[ERROR] Erro ao atualizar checkpoint para %s: %v", name, err)
	}
	
	return err
}

func nullOrUUID(u *uuid.UUID) string {
	if u == nil {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", *u)
}

func nullOrString(s *string) string {
	if s == nil {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", db.SafeString(*s))
}