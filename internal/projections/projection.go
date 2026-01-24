package projections

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"spuri/internal/db"
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
	safeName := db.SafeString(name)
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	log.Printf("[DEBUG] GetLastProcessedEventIDByName - Query: %s", query)
	
	var lastID int64
	err := bp.client.DB().QueryRow(query).Scan(&lastID)
	
	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] GetLastProcessedEventIDByName - Nenhum checkpoint encontrado para: %s", name)
		return 0, nil
	}
	if err != nil {
		log.Printf("[ERROR] GetLastProcessedEventIDByName - Erro ao buscar checkpoint: %v", err)
		return 0, nil
	}
	
	log.Printf("[DEBUG] GetLastProcessedEventIDByName - LastID encontrado: %d para projection: %s", lastID, name)
	return lastID, nil
}

func (bp *BaseProjection) UpdateCheckpointByName(name string, eventID int64) error {
	safeName := db.SafeString(name)
	
	if eventID < 0 {
		eventID = 0
	}
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (
			projection_name, last_processed_event_id, last_processed_at, events_processed
		) VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, safeName, eventID, eventID)
	
	log.Printf("[DEBUG] UpdateCheckpointByName - Query: %s", query)
	
	_, err := bp.client.DB().Exec(query)
	
	if err != nil {
		log.Printf("[ERROR] UpdateCheckpointByName - Erro ao atualizar checkpoint: %v", err)
	} else {
		log.Printf("[DEBUG] UpdateCheckpointByName - Checkpoint atualizado com sucesso para: %s, eventID: %d", name, eventID)
	}
	
	return err
}