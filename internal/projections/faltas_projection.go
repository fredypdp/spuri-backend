package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"
)

type FaltasProjection struct {
	client *db.Client
}

func NewFaltasProjection(client *db.Client) *FaltasProjection {
	return &FaltasProjection{client: client}
}

func (p *FaltasProjection) Name() string { return "faltas" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *FaltasProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	err := p.client.DB().QueryRow(
		`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = $1`,
		p.Name(),
	).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *FaltasProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

func (p *FaltasProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"FaltasRegistradas": p.handleFaltasRegistradas,
		"FaltaAtualizada":   p.handleFaltaAtualizada,
		"FaltaDeletada":     p.handleFaltaDeletada,
	}
	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] [faltas] Processando %s: %s", event.EventType, event.EventID)
		return handler(event)
	}
	return nil
}

func (p *FaltasProjection) HandleTx(tx *sql.Tx, event db.Event) error {
	switch event.EventType {
	case "FaltasRegistradas":
		return p.handleFaltasRegistradasTx(tx, event)
	case "FaltaAtualizada":
		return p.handleFaltaAtualizadaTx(tx, event)
	case "FaltaDeletada":
		return p.handleFaltaDeletadaTx(tx, event)
	default:
		return nil
	}
}

// ============================================================================
// Handlers de evento
// ============================================================================

func (p *FaltasProjection) handleFaltasRegistradas(event db.Event) error {
	var payload struct {
		CodigoEstudante      string    `json:"CodigoEstudante"`
		CodigoAcademia       string    `json:"CodigoAcademia"`
		AnoLectivo           string    `json:"AnoLectivo"`
		AnoAcademico         string    `json:"AnoAcademico"`
		Data                 time.Time `json:"Data"`
		MateriaDisciplinarID string    `json:"MateriaDisciplinarID"`
		Quantidade           int       `json:"Quantidade"`
		Observacao           *string   `json:"Observacao"`
		RegisteredAt         time.Time `json:"RegisteredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error FaltasRegistradas: %w", err)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_faltas (
			codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			data, materia_disciplinar_id, quantidade, observacao,
			registered_at, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Data.Format("2006-01-02"), payload.MateriaDisciplinarID, payload.Quantidade, payload.Observacao,
		payload.RegisteredAt, event.EventID, event.EventVersion,
	)
	return err
}

func (p *FaltasProjection) handleFaltaAtualizada(event db.Event) error {
	var payload struct {
		FaltaID              string     `json:"FaltaID"`
		Data                 *time.Time `json:"Data"`
		MateriaDisciplinarID *string    `json:"MateriaDisciplinarID"`
		Quantidade           *int       `json:"Quantidade"`
		Observacao           *string    `json:"Observacao"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error FaltaAtualizada: %w", err)
	}
	if payload.FaltaID == "" {
		return fmt.Errorf("handleFaltaAtualizada: FaltaID vazio no payload")
	}

	// Atualiza apenas os campos fornecidos (não-nil)
	if payload.Data != nil {
		if _, err := p.client.DB().Exec(`
			UPDATE projection_faltas SET data = $1, version = $2, event_id = $3 WHERE id = $4
		`, payload.Data.Format("2006-01-02"), event.EventVersion, event.EventID, payload.FaltaID); err != nil {
			return fmt.Errorf("handleFaltaAtualizada: update data: %w", err)
		}
	}
	if payload.MateriaDisciplinarID != nil {
		if _, err := p.client.DB().Exec(`
			UPDATE projection_faltas SET materia_disciplinar_id = $1, version = $2, event_id = $3 WHERE id = $4
		`, *payload.MateriaDisciplinarID, event.EventVersion, event.EventID, payload.FaltaID); err != nil {
			return fmt.Errorf("handleFaltaAtualizada: update materia: %w", err)
		}
	}
	if payload.Quantidade != nil {
		if _, err := p.client.DB().Exec(`
			UPDATE projection_faltas SET quantidade = $1, version = $2, event_id = $3 WHERE id = $4
		`, *payload.Quantidade, event.EventVersion, event.EventID, payload.FaltaID); err != nil {
			return fmt.Errorf("handleFaltaAtualizada: update quantidade: %w", err)
		}
	}
	if payload.Observacao != nil {
		if _, err := p.client.DB().Exec(`
			UPDATE projection_faltas SET observacao = $1, version = $2, event_id = $3 WHERE id = $4
		`, *payload.Observacao, event.EventVersion, event.EventID, payload.FaltaID); err != nil {
			return fmt.Errorf("handleFaltaAtualizada: update observacao: %w", err)
		}
	}
	return nil
}

// handleFaltaDeletada processa o evento "FaltaDeletada" — soft delete na projeção.
// Idempotente: se a falta já estiver deletada (deleted_at IS NOT NULL), não falha.
func (p *FaltasProjection) handleFaltaDeletada(event db.Event) error {
	var payload struct {
		FaltaID string `json:"FaltaID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleFaltaDeletada: parse error: %w", err)
	}

	result, err := p.client.DB().Exec(`
		UPDATE projection_faltas
		SET deleted_at = NOW(),
		    version    = $1,
		    event_id   = $2
		WHERE id = $3
		  AND deleted_at IS NULL
	`, event.EventVersion, event.EventID, payload.FaltaID)
	if err != nil {
		return fmt.Errorf("handleFaltaDeletada: exec error: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("[WARN] [faltas] FaltaDeletada %s: falta id=%s não encontrada ou já deletada — ignorado",
			event.EventID, payload.FaltaID)
	}
	return nil
}

func (p *FaltasProjection) handleFaltasRegistradasTx(tx *sql.Tx, event db.Event) error {
	var payload struct {
		CodigoEstudante      string    `json:"CodigoEstudante"`
		CodigoAcademia       string    `json:"CodigoAcademia"`
		AnoLectivo           string    `json:"AnoLectivo"`
		AnoAcademico         string    `json:"AnoAcademico"`
		Data                 time.Time `json:"Data"`
		MateriaDisciplinarID string    `json:"MateriaDisciplinarID"`
		Quantidade           int       `json:"Quantidade"`
		Observacao           *string   `json:"Observacao"`
		RegisteredAt         time.Time `json:"RegisteredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error FaltasRegistradas: %w", err)
	}

	_, err := tx.Exec(`
		INSERT INTO projection_faltas (
			codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			data, materia_disciplinar_id, quantidade, observacao,
			registered_at, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (event_id) DO NOTHING
	`,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Data.UTC(), payload.MateriaDisciplinarID, payload.Quantidade, payload.Observacao,
		payload.RegisteredAt.UTC(), event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleFaltasRegistradasTx: exec error: %w", err)
	}
	return nil
}

func (p *FaltasProjection) handleFaltaAtualizadaTx(tx *sql.Tx, event db.Event) error {
	var payload struct {
		FaltaID              string     `json:"FaltaID"`
		Data                 *time.Time `json:"Data"`
		MateriaDisciplinarID *string    `json:"MateriaDisciplinarID"`
		Quantidade           *int       `json:"Quantidade"`
		Observacao           *string    `json:"Observacao"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error FaltaAtualizada: %w", err)
	}
	if payload.FaltaID == "" {
		return fmt.Errorf("handleFaltaAtualizadaTx: FaltaID vazio no payload")
	}
	if payload.Data != nil {
		if _, err := tx.Exec(`UPDATE projection_faltas SET data = $1, version = $2, event_id = $3 WHERE id = $4`,
			payload.Data.Format("2006-01-02"), event.EventVersion, event.EventID, payload.FaltaID); err != nil {
			return err
		}
	}
	if payload.MateriaDisciplinarID != nil {
		if _, err := tx.Exec(`UPDATE projection_faltas SET materia_disciplinar_id = $1, version = $2, event_id = $3 WHERE id = $4`,
			*payload.MateriaDisciplinarID, event.EventVersion, event.EventID, payload.FaltaID); err != nil {
			return err
		}
	}
	if payload.Quantidade != nil {
		if _, err := tx.Exec(`UPDATE projection_faltas SET quantidade = $1, version = $2, event_id = $3 WHERE id = $4`,
			*payload.Quantidade, event.EventVersion, event.EventID, payload.FaltaID); err != nil {
			return err
		}
	}
	if payload.Observacao != nil {
		if _, err := tx.Exec(`UPDATE projection_faltas SET observacao = $1, version = $2, event_id = $3 WHERE id = $4`,
			*payload.Observacao, event.EventVersion, event.EventID, payload.FaltaID); err != nil {
			return err
		}
	}
	return nil
}

// handleFaltaDeletadaTx processa FaltaDeletada dentro de uma transação.
func (p *FaltasProjection) handleFaltaDeletadaTx(tx *sql.Tx, event db.Event) error {
	var payload struct {
		FaltaID string `json:"FaltaID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleFaltaDeletadaTx: parse error: %w", err)
	}

	result, err := tx.Exec(`
		UPDATE projection_faltas
		SET deleted_at = NOW(),
		    version    = $1,
		    event_id   = $2
		WHERE id = $3
		  AND deleted_at IS NULL
	`, event.EventVersion, event.EventID, payload.FaltaID)
	if err != nil {
		return fmt.Errorf("handleFaltaDeletadaTx: exec error: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("[WARN] [faltas] FaltaDeletadaTx %s: falta id=%s não encontrada ou já deletada — ignorado",
			event.EventID, payload.FaltaID)
	}
	return nil
}

// ============================================================================
// Rebuild
// ============================================================================

func (p *FaltasProjection) Rebuild() error {
	log.Printf("[DEBUG] [faltas] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('FaltasRegistradas', 'FaltaAtualizada', 'FaltaDeletada')
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var event db.Event
		var prevHash sql.NullString
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		); err != nil {
			return err
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}
	log.Printf("[DEBUG] [faltas] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *FaltasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_faltas CASCADE`)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

type FaltaDTO struct {
	ID                   string  `json:"id"`
	CodigoEstudante      string  `json:"codigo_estudante"`
	CodigoAcademia       string  `json:"codigo_academia"`
	AnoLectivo           string  `json:"ano_lectivo"`
	AnoAcademico         string  `json:"ano_academico"`
	Data                 string  `json:"data"`
	MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
	MateriaNome          *string `json:"materia_nome,omitempty"`
	Quantidade           int     `json:"quantidade"`
	Observacao           *string `json:"observacao,omitempty"`
	RegisteredAt         string  `json:"registered_at"`
	EventID              string  `json:"event_id"`
	Version              int     `json:"version"`
}

func (p *FaltasProjection) GetByID(id string) (*FaltaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, m.nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON m.id::text = f.materia_disciplinar_id
		WHERE f.id = $1
		  AND f.deleted_at IS NULL
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	faltas, err := scanFaltas(rows)
	if err != nil || len(faltas) == 0 {
		return nil, err
	}
	return &faltas[0], nil
}

func (p *FaltasProjection) GetByEstudante(codigoEstudante string) ([]FaltaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, m.nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON m.id::text = f.materia_disciplinar_id
		WHERE f.codigo_estudante = $1
		  AND f.deleted_at IS NULL
		ORDER BY f.data DESC
	`, codigoEstudante)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFaltas(rows)
}

func (p *FaltasProjection) GetByAcademia(codigoAcademia string) ([]FaltaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, m.nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON m.id::text = f.materia_disciplinar_id
		WHERE f.codigo_academia = $1
		  AND f.deleted_at IS NULL
		ORDER BY f.data DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFaltas(rows)
}

func (p *FaltasProjection) GetByPeriodo(codigoEstudante, anoLectivo string, dataInicio, dataFim time.Time) ([]FaltaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, m.nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON m.id::text = f.materia_disciplinar_id
		WHERE f.codigo_estudante = $1
			AND f.ano_lectivo = $2
			AND f.data BETWEEN $3 AND $4
			AND f.deleted_at IS NULL
		ORDER BY f.data DESC
	`, codigoEstudante, anoLectivo, dataInicio.Format(time.RFC3339), dataFim.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFaltas(rows)
}

func (p *FaltasProjection) GetAll() ([]FaltaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT f.id, f.codigo_estudante, f.codigo_academia, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, m.nome, f.quantidade, f.observacao,
			f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON m.id::text = f.materia_disciplinar_id
		WHERE f.deleted_at IS NULL
		ORDER BY f.data DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFaltas(rows)
}

func scanFaltas(rows *sql.Rows) ([]FaltaDTO, error) {
	var faltas []FaltaDTO
	for rows.Next() {
		var f FaltaDTO
		if err := rows.Scan(
			&f.ID, &f.CodigoEstudante, &f.CodigoAcademia, &f.AnoLectivo, &f.AnoAcademico,
			&f.Data, &f.MateriaDisciplinarID, &f.MateriaNome, &f.Quantidade, &f.Observacao,
			&f.RegisteredAt, &f.EventID, &f.Version,
		); err != nil {
			continue
		}
		faltas = append(faltas, f)
	}
	return faltas, rows.Err()
}