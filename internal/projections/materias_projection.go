package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

type MateriasProjection struct {
	client *db.Client
}

func NewMateriasProjection(client *db.Client) *MateriasProjection {
	return &MateriasProjection{client: client}
}

func (mp *MateriasProjection) Name() string { return "materias" }

func (mp *MateriasProjection) Handle(event db.Event) error {
	log.Printf("[DEBUG] MateriasProjection.Handle - AggregateType: %s, EventType: %s", 
		event.AggregateType, event.EventType)
	
	if event.AggregateType != "MateriaDisciplinar" {
		log.Printf("[DEBUG] MateriasProjection.Handle - Evento ignorado, tipo: %s", event.AggregateType)
		return nil
	}

	switch event.EventType {
	case "MateriaCriada":
		return mp.handleMateriaCriada(event)
	case "MateriaAtivada":
		return mp.handleMateriaAtivada(event)
	case "MateriaDesativada":
		return mp.handleMateriaDesativada(event)
	case "MateriaDadosAtualizados":
		return mp.handleMateriaDadosAtualizados(event)
	}
	
	log.Printf("[DEBUG] MateriasProjection.Handle - EventType não tratado: %s", event.EventType)
	return nil
}

func (mp *MateriasProjection) Rebuild() error {
	log.Printf("[DEBUG] MateriasProjection.Rebuild - Iniciando rebuild")
	
	if err := mp.clear(); err != nil {
		log.Printf("[ERROR] MateriasProjection.Rebuild - Erro ao limpar tabela: %v", err)
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE aggregate_type = 'MateriaDisciplinar' ORDER BY id ASC
	`
	
	log.Printf("[DEBUG] MateriasProjection.Rebuild - Query: %s", query)
	
	rows, err := mp.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.Rebuild - Erro ao buscar eventos: %v", err)
		return err
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var event db.Event
		err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash)
		if err != nil {
			log.Printf("[ERROR] MateriasProjection.Rebuild - Erro ao fazer scan do evento: %v", err)
			return err
		}
		if err := mp.Handle(event); err != nil {
			log.Printf("[ERROR] MateriasProjection.Rebuild - Erro ao processar evento %d: %v", event.ID, err)
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
		count++
	}
	
	log.Printf("[DEBUG] MateriasProjection.Rebuild - Rebuild concluído, %d eventos processados", count)
	return rows.Err()
}

func (mp *MateriasProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(mp.Name())
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	log.Printf("[DEBUG] MateriasProjection.GetLastProcessedEventID - Query: %s", query)
	
	var lastID int64
	err := mp.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] MateriasProjection.GetLastProcessedEventID - Nenhum checkpoint encontrado")
		return 0, nil
	}
	
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.GetLastProcessedEventID - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] MateriasProjection.GetLastProcessedEventID - LastID: %d", lastID)
	}
	
	return lastID, err
}

func (mp *MateriasProjection) UpdateCheckpoint(eventID int64) error {
	safeName := db.SafeString(mp.Name())
	eventID = int64(db.ValidateOffset(int(eventID)))
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, safeName, eventID, eventID)
	
	log.Printf("[DEBUG] MateriasProjection.UpdateCheckpoint - Query: %s", query)
	
	_, err := mp.client.DB().Exec(query)
	
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.UpdateCheckpoint - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] MateriasProjection.UpdateCheckpoint - Checkpoint atualizado para eventID: %d", eventID)
	}
	
	return err
}

func (mp *MateriasProjection) clear() error {
	log.Printf("[DEBUG] MateriasProjection.clear - Limpando tabela projection_materias")
	_, err := mp.client.DB().Exec(`TRUNCATE TABLE projection_materias CASCADE`)
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.clear - Erro: %v", err)
	}
	return err
}

func (mp *MateriasProjection) handleMateriaCriada(event db.Event) error {
	log.Printf("[DEBUG] MateriasProjection.handleMateriaCriada - EventID: %s", event.EventID)
	
	var payload struct {
		Nome           string     `json:"Nome"`
		Type           string     `json:"Type"`
		Nivel          []string   `json:"Nivel"`
		CodigoAcademia string     `json:"CodigoAcademia"`
		CursoID        *uuid.UUID `json:"CursoID"`
		CreatedAt      time.Time  `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaCriada - Erro ao parsear payload: %v", err)
		return err
	}

	log.Printf("[DEBUG] MateriasProjection.handleMateriaCriada - Payload: Nome=%s, Type=%s", 
		payload.Nome, payload.Type)

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaCriada - UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeNome := db.SafeString(payload.Nome)
	safeType := db.SafeString(payload.Type)
	safeCodigo := db.SafeString(payload.CodigoAcademia)

	var nivelStr, cursoStr string
	if len(payload.Nivel) > 0 {
		nivelJSON, _ := json.Marshal(payload.Nivel)
		nivelStr = fmt.Sprintf("'%s'", db.SafeString(string(nivelJSON)))
	} else {
		nivelStr = "NULL"
	}

	if payload.CursoID != nil {
		cursoStr = fmt.Sprintf("'%s'", *payload.CursoID)
	} else {
		cursoStr = "NULL"
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_materias (id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version, last_event_id)
		VALUES ('%s', '%s', '%s', %s, '%s', %s, 'ativo', '%s', CURRENT_TIMESTAMP, %d, '%s')
	`, aggID, safeNome, safeType, nivelStr, safeCodigo, cursoStr,
		payload.CreatedAt.Format(time.RFC3339), event.EventVersion, event.EventID)

	log.Printf("[DEBUG] MateriasProjection.handleMateriaCriada - Insert query: %s", query)

	_, err := mp.client.DB().Exec(query)
	
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaCriada - Erro ao inserir: %v", err)
	} else {
		log.Printf("[DEBUG] MateriasProjection.handleMateriaCriada - Matéria criada com sucesso")
	}
	
	return err
}

func (mp *MateriasProjection) handleMateriaAtivada(event db.Event) error {
	log.Printf("[DEBUG] MateriasProjection.handleMateriaAtivada - EventID: %s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaAtivada - UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_materias SET status = 'ativo', version = %d, updated_at = CURRENT_TIMESTAMP WHERE id = '%s'
	`, event.EventVersion, aggID)
	
	log.Printf("[DEBUG] MateriasProjection.handleMateriaAtivada - Query: %s", query)
	
	_, err := mp.client.DB().Exec(query)
	
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaAtivada - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] MateriasProjection.handleMateriaAtivada - Matéria ativada com sucesso")
	}
	
	return err
}

func (mp *MateriasProjection) handleMateriaDesativada(event db.Event) error {
	log.Printf("[DEBUG] MateriasProjection.handleMateriaDesativada - EventID: %s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaDesativada - UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_materias SET status = 'inativo', version = %d, updated_at = CURRENT_TIMESTAMP WHERE id = '%s'
	`, event.EventVersion, aggID)
	
	log.Printf("[DEBUG] MateriasProjection.handleMateriaDesativada - Query: %s", query)
	
	_, err := mp.client.DB().Exec(query)
	
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaDesativada - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] MateriasProjection.handleMateriaDesativada - Matéria desativada com sucesso")
	}
	
	return err
}

func (mp *MateriasProjection) handleMateriaDadosAtualizados(event db.Event) error {
	log.Printf("[DEBUG] MateriasProjection.handleMateriaDadosAtualizados - EventID: %s", event.EventID)
	
	var payload struct {
		Nome *string `json:"Nome"`
		Type *string `json:"Type"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaDadosAtualizados - Erro ao parsear payload: %v", err)
		return err
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaDadosAtualizados - UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	if payload.Nome != nil {
		safe := db.SafeString(*payload.Nome)
		query := fmt.Sprintf(`UPDATE projection_materias SET nome = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[DEBUG] MateriasProjection.handleMateriaDadosAtualizados - Update nome: %s", query)
		mp.client.DB().Exec(query)
	}
	
	if payload.Type != nil {
		safe := db.SafeString(*payload.Type)
		query := fmt.Sprintf(`UPDATE projection_materias SET type = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[DEBUG] MateriasProjection.handleMateriaDadosAtualizados - Update type: %s", query)
		mp.client.DB().Exec(query)
	}

	updateQuery := fmt.Sprintf(`
		UPDATE projection_materias SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s' WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[DEBUG] MateriasProjection.handleMateriaDadosAtualizados - Final update: %s", updateQuery)
	
	_, err := mp.client.DB().Exec(updateQuery)
	
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.handleMateriaDadosAtualizados - Erro: %v", err)
	} else {
		log.Printf("[DEBUG] MateriasProjection.handleMateriaDadosAtualizados - Dados atualizados com sucesso")
	}
	
	return err
}

func (mp *MateriasProjection) GetByID(id uuid.UUID) (*MateriaDTO, error) {
	if id == uuid.Nil {
		log.Printf("[ERROR] MateriasProjection.GetByID - UUID inválido")
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE id = '%s'
	`, id)
	
	log.Printf("[DEBUG] MateriasProjection.GetByID - Query: %s", query)
	
	var dto MateriaDTO
	var nivelJSON sql.NullString
	var cursoID sql.NullString

	err := mp.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia, 
		&cursoID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
	
	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] MateriasProjection.GetByID - Matéria não encontrada")
		return nil, nil
	}
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.GetByID - Erro: %v", err)
		return nil, err
	}

	if nivelJSON.Valid && nivelJSON.String != "" {
		json.Unmarshal([]byte(nivelJSON.String), &dto.Nivel)
	}

	if cursoID.Valid {
		cid, _ := uuid.Parse(cursoID.String)
		dto.CursoID = &cid
	}

	log.Printf("[DEBUG] MateriasProjection.GetByID - Matéria encontrada: %s", dto.Nome)
	return &dto, nil
}

func (mp *MateriasProjection) GetByAcademia(codigoAcademia string) ([]MateriaDTO, error) {
	safeCodigo := db.SafeString(codigoAcademia)

	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, curso_id, status, created_at, updated_at, version
		FROM projection_materias WHERE codigo_academia = '%s' ORDER BY created_at DESC
	`, safeCodigo)
	
	log.Printf("[DEBUG] MateriasProjection.GetByAcademia - Query: %s", query)
	
	rows, err := mp.client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] MateriasProjection.GetByAcademia - Erro: %v", err)
		return nil, err
	}
	defer rows.Close()

	var materias []MateriaDTO
	for rows.Next() {
		var dto MateriaDTO
		var nivelJSON sql.NullString
		var cursoID sql.NullString

		err := rows.Scan(&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
			&cursoID, &dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
		if err != nil {
			log.Printf("[ERROR] MateriasProjection.GetByAcademia - Erro ao fazer scan: %v", err)
			continue
		}

		if nivelJSON.Valid && nivelJSON.String != "" {
			json.Unmarshal([]byte(nivelJSON.String), &dto.Nivel)
		}

		if cursoID.Valid {
			cid, _ := uuid.Parse(cursoID.String)
			dto.CursoID = &cid
		}

		materias = append(materias, dto)
	}

	log.Printf("[DEBUG] MateriasProjection.GetByAcademia - %d matérias encontradas", len(materias))
	return materias, rows.Err()
}

type MateriaDTO struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	Nome           string     `json:"nome" db:"nome"`
	Type           string     `json:"type" db:"type"`
	Nivel          []string   `json:"nivel,omitempty" db:"nivel"`
	CodigoAcademia string     `json:"codigo_academia" db:"codigo_academia"`
	CursoID        *uuid.UUID `json:"curso_id,omitempty" db:"curso_id"`
	Status         string     `json:"status" db:"status"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
	Version        int        `json:"version" db:"version"`
}