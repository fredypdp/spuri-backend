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

type CursosProjection struct {
	client *db.Client
}

func NewCursosProjection(client *db.Client) *CursosProjection {
	return &CursosProjection{client: client}
}

func (p *CursosProjection) Name() string { return "cursos" }

func (p *CursosProjection) Handle(event db.Event) error {
	log.Printf("[CURSOS_PROJECTION] Recebendo evento: type=%s, aggregate_id=%s, event_id=%s", 
		event.EventType, event.AggregateID, event.EventID)
	
	if event.AggregateType != "Curso" {
		log.Printf("[CURSOS_PROJECTION] Ignorando evento de tipo %s", event.AggregateType)
		return nil
	}

	switch event.EventType {
	case "CursoCriado":
		return p.handleCursoCriado(event)
	case "CursoAtivado":
		return p.handleCursoAtivado(event)
	case "CursoDesativado":
		return p.handleCursoDesativado(event)
	case "CursoDadosAtualizados":
		return p.handleCursoDadosAtualizados(event)
	default:
		log.Printf("[CURSOS_PROJECTION] Tipo de evento desconhecido: %s", event.EventType)
	}
	return nil
}

func (p *CursosProjection) Rebuild() error {
	log.Printf("[CURSOS_PROJECTION] Iniciando rebuild da projeção")
	
	if err := p.clear(); err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao limpar projeção: %v", err)
		return err
	}

	query := `
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger WHERE aggregate_type = 'Curso' ORDER BY id ASC
	`
	
	log.Printf("[CURSOS_PROJECTION] Executando query de rebuild")
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao executar query de rebuild: %v", err)
		return err
	}
	defer rows.Close()

	eventCount := 0
	for rows.Next() {
		var event db.Event
		err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash)
		if err != nil {
			log.Printf("[CURSOS_PROJECTION] Erro ao fazer scan do evento: %v", err)
			return err
		}
		if err := p.Handle(event); err != nil {
			log.Printf("[CURSOS_PROJECTION] Erro ao processar evento %d: %v", event.ID, err)
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
		eventCount++
	}
	
	log.Printf("[CURSOS_PROJECTION] Rebuild concluído. %d eventos processados", eventCount)
	return rows.Err()
}

func (p *CursosProjection) GetLastProcessedEventID() (int64, error) {
	safeName := db.SafeString(p.Name())
	
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)
	
	log.Printf("[CURSOS_PROJECTION] Buscando último evento processado: %s", query)
	
	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		log.Printf("[CURSOS_PROJECTION] Nenhum checkpoint encontrado, retornando 0")
		return 0, nil
	}
	
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao buscar checkpoint: %v", err)
	} else {
		log.Printf("[CURSOS_PROJECTION] Último evento processado: %d", lastID)
	}
	
	return lastID, err
}

func (p *CursosProjection) UpdateCheckpoint(eventID int64) error {
	safeName := db.SafeString(p.Name())
	eventID = int64(db.ValidateOffset(int(eventID)))
	
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d, last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, safeName, eventID, eventID)
	
	log.Printf("[CURSOS_PROJECTION] Atualizando checkpoint para event_id=%d", eventID)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao atualizar checkpoint: %v", err)
	}
	return err
}

func (p *CursosProjection) clear() error {
	log.Printf("[CURSOS_PROJECTION] Limpando tabela projection_cursos")
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_cursos CASCADE`)
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao limpar tabela: %v", err)
	}
	return err
}

func (p *CursosProjection) handleCursoCriado(event db.Event) error {
	log.Printf("[CURSOS_PROJECTION] Processando CursoCriado: event_id=%s", event.EventID)
	
	var payload struct {
		Nome           string    `json:"Nome"`
		Type           string    `json:"Type"`
		Nivel          []string  `json:"Nivel"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao parsear payload: %v", err)
		return err
	}

	log.Printf("[CURSOS_PROJECTION] Dados do curso: nome=%s, type=%s, academia=%s, nivel=%v", 
		payload.Nome, payload.Type, payload.CodigoAcademia, payload.Nivel)

	nivelJSON, _ := json.Marshal(payload.Nivel)
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[CURSOS_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	safeNome := db.SafeString(payload.Nome)
	safeType := db.SafeString(payload.Type)
	safeNivel := db.SafeString(string(nivelJSON))
	safeCodigo := db.SafeString(payload.CodigoAcademia)

	query := fmt.Sprintf(`
		INSERT INTO projection_cursos (id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version, last_event_id)
		VALUES ('%s', '%s', '%s', '%s', '%s', 'ativo', '%s', CURRENT_TIMESTAMP, %d, '%s')
	`, aggID, safeNome, safeType, safeNivel, safeCodigo,
		payload.CreatedAt.Format(time.RFC3339), event.EventVersion, event.EventID)

	log.Printf("[CURSOS_PROJECTION] Executando insert: %s", query)

	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao processar CursoCriado (event_id: %s): %v", event.EventID, err)
	} else {
		log.Printf("[CURSOS_PROJECTION] CursoCriado processado com sucesso: id=%s", aggID)
	}
	return err
}

func (p *CursosProjection) handleCursoAtivado(event db.Event) error {
	log.Printf("[CURSOS_PROJECTION] Processando CursoAtivado: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[CURSOS_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_cursos SET status = 'ativo', version = %d, updated_at = CURRENT_TIMESTAMP WHERE id = '%s'
	`, event.EventVersion, aggID)
	
	log.Printf("[CURSOS_PROJECTION] Executando update: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao ativar curso %s: %v", aggID, err)
	} else {
		log.Printf("[CURSOS_PROJECTION] Curso %s ativado com sucesso", aggID)
	}
	return err
}

func (p *CursosProjection) handleCursoDesativado(event db.Event) error {
	log.Printf("[CURSOS_PROJECTION] Processando CursoDesativado: event_id=%s", event.EventID)
	
	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[CURSOS_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		UPDATE projection_cursos SET status = 'inativo', version = %d, updated_at = CURRENT_TIMESTAMP WHERE id = '%s'
	`, event.EventVersion, aggID)
	
	log.Printf("[CURSOS_PROJECTION] Executando update: %s", query)
	
	_, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao desativar curso %s: %v", aggID, err)
	} else {
		log.Printf("[CURSOS_PROJECTION] Curso %s desativado com sucesso", aggID)
	}
	return err
}

func (p *CursosProjection) handleCursoDadosAtualizados(event db.Event) error {
	log.Printf("[CURSOS_PROJECTION] Processando CursoDadosAtualizados: event_id=%s", event.EventID)
	
	var payload struct {
		Nome  *string  `json:"Nome"`
		Type  *string  `json:"Type"`
		Nivel []string `json:"Nivel"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao parsear payload: %v", err)
		return err
	}

	aggID := event.AggregateID
	if aggID == uuid.Nil {
		log.Printf("[CURSOS_PROJECTION] UUID inválido")
		return fmt.Errorf("UUID inválido")
	}

	if payload.Nome != nil {
		safe := db.SafeString(*payload.Nome)
		query := fmt.Sprintf(`UPDATE projection_cursos SET nome = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[CURSOS_PROJECTION] Atualizando nome: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.Type != nil {
		safe := db.SafeString(*payload.Type)
		query := fmt.Sprintf(`UPDATE projection_cursos SET type = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[CURSOS_PROJECTION] Atualizando type: %s", query)
		p.client.DB().Exec(query)
	}
	if payload.Nivel != nil {
		nivelJSON, _ := json.Marshal(payload.Nivel)
		safe := db.SafeString(string(nivelJSON))
		query := fmt.Sprintf(`UPDATE projection_cursos SET nivel = '%s' WHERE id = '%s'`, safe, aggID)
		log.Printf("[CURSOS_PROJECTION] Atualizando nivel: %s", query)
		p.client.DB().Exec(query)
	}

	updateQuery := fmt.Sprintf(`
		UPDATE projection_cursos SET version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s' WHERE id = '%s'
	`, event.EventVersion, event.EventID, aggID)
	
	log.Printf("[CURSOS_PROJECTION] Atualizando version e timestamp: %s", updateQuery)
	
	_, err := p.client.DB().Exec(updateQuery)
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao atualizar dados: %v", err)
	} else {
		log.Printf("[CURSOS_PROJECTION] Dados do curso %s atualizados com sucesso", aggID)
	}
	return err
}

func (p *CursosProjection) GetByID(id uuid.UUID) (*CursoDTO, error) {
	log.Printf("[CURSOS_PROJECTION] Buscando curso por ID: %s", id)
	
	if id == uuid.Nil {
		log.Printf("[CURSOS_PROJECTION] UUID inválido fornecido")
		return nil, fmt.Errorf("UUID inválido")
	}

	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos WHERE id = '%s'
	`, id)
	
	log.Printf("[CURSOS_PROJECTION] Executando query: %s", query)
	
	var dto CursoDTO
	var nivelJSON []byte
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
		&dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
	
	if err == sql.ErrNoRows {
		log.Printf("[CURSOS_PROJECTION] Curso não encontrado: %s", id)
		return nil, nil
	}
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao buscar curso: %v", err)
		return nil, err
	}

	json.Unmarshal(nivelJSON, &dto.Nivel)
	
	log.Printf("[CURSOS_PROJECTION] Curso encontrado: %s (%s)", dto.Nome, dto.CodigoAcademia)
	return &dto, nil
}

func (p *CursosProjection) GetByAcademia(codigoAcademia string) ([]CursoDTO, error) {
	log.Printf("[CURSOS_PROJECTION] Buscando cursos por academia: %s", codigoAcademia)
	
	safeCodigo := db.SafeString(codigoAcademia)

	query := fmt.Sprintf(`
		SELECT id, nome, type, nivel, codigo_academia, status, created_at, updated_at, version
		FROM projection_cursos WHERE codigo_academia = '%s' ORDER BY created_at DESC
	`, safeCodigo)
	
	log.Printf("[CURSOS_PROJECTION] Executando query: %s", query)
	
	rows, err := p.client.DB().Query(query)
	if err != nil {
		log.Printf("[CURSOS_PROJECTION] Erro ao buscar cursos: %v", err)
		return nil, err
	}
	defer rows.Close()

	var cursos []CursoDTO
	count := 0
	for rows.Next() {
		var dto CursoDTO
		var nivelJSON []byte
		err := rows.Scan(&dto.ID, &dto.Nome, &dto.Type, &nivelJSON, &dto.CodigoAcademia,
			&dto.Status, &dto.CreatedAt, &dto.UpdatedAt, &dto.Version)
		if err != nil {
			log.Printf("[CURSOS_PROJECTION] Erro ao fazer scan do curso: %v", err)
			continue
		}
		json.Unmarshal(nivelJSON, &dto.Nivel)
		cursos = append(cursos, dto)
		count++
	}

	log.Printf("[CURSOS_PROJECTION] %d cursos encontrados para academia %s", count, codigoAcademia)
	return cursos, rows.Err()
}

type CursoDTO struct {
	ID             uuid.UUID `json:"id" db:"id"`
	Nome           string    `json:"nome" db:"nome"`
	Type           string    `json:"type" db:"type"`
	Nivel          []string  `json:"nivel" db:"nivel"`
	CodigoAcademia string    `json:"codigo_academia" db:"codigo_academia"`
	Status         string    `json:"status" db:"status"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
	Version        int       `json:"version" db:"version"`
}