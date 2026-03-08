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

type NotasProjection struct {
	client *db.Client
}

func NewNotasProjection(client *db.Client) *NotasProjection {
	return &NotasProjection{client: client}
}

func (p *NotasProjection) Name() string { return "notas" }

// ============================================================================
// Interface Projection
// ============================================================================

// GetLastProcessedEventID — corrigido para prepared statement.
func (p *NotasProjection) GetLastProcessedEventID() (int64, error) {
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

// UpdateCheckpoint — corrigido para prepared statement.
func (p *NotasProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID)
	return err
}

// ============================================================================
// Handle
// ============================================================================

func (p *NotasProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"NotasRegistradas": p.handleNotasRegistradas,
		"NotaAtualizada":   p.handleNotaAtualizada,
		"NotaDeletada":     p.handleNotaDeletada,
	}
	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] [notas] Processando %s: %s", event.EventType, event.EventID)
		return handler(event)
	}
	return nil
}

// ============================================================================
// Rebuild — sql.NullString para previous_hash
// ============================================================================

func (p *NotasProjection) Rebuild() error {
	log.Printf("[DEBUG] [notas] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('NotasRegistradas', 'NotaAtualizada', 'NotaDeletada')
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
			return fmt.Errorf("erro ao escanear evento %d: %w", count, err)
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}

	log.Printf("[DEBUG] [notas] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *NotasProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_notas CASCADE`)
	return err
}

// ============================================================================
// Handlers de evento
// ============================================================================

// handleNotasRegistradas — P3-18: ON CONFLICT DO NOTHING substituído por UPSERT.
// Com "DO NOTHING", um replay de evento (idempotência ou rebuild parcial)
// descartava a nota silenciosamente sem nenhum log. O UPSERT garante que
// o dado mais recente prevalece e que a operação é rastreável.
func (p *NotasProjection) handleNotasRegistradas(event db.Event) error {
	var payload struct {
		CodigoEstudante      string    `json:"CodigoEstudante"`
		CodigoAcademia       string    `json:"CodigoAcademia"`
		AnoLectivo           string    `json:"AnoLectivo"`
		AnoAcademico         string    `json:"AnoAcademico"`
		Periodo              string    `json:"Periodo"`
		MateriaDisciplinarID string    `json:"MateriaDisciplinarID"`
		Tipo                 string    `json:"Tipo"`
		Categoria            string    `json:"Categoria"`
		Nota                 float64   `json:"Nota"`
		Observacao           *string   `json:"Observacao"`
		RegisteredAt         time.Time `json:"RegisteredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleNotasRegistradas: parse error: %w", err)
	}

	result, err := p.client.DB().Exec(`
		INSERT INTO projection_notas (
			codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			periodo, materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
		DO UPDATE SET
			nota          = EXCLUDED.nota,
			observacao    = EXCLUDED.observacao,
			event_id      = EXCLUDED.event_id,
			version       = EXCLUDED.version,
			registered_at = EXCLUDED.registered_at
	`,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Periodo, payload.MateriaDisciplinarID, payload.Tipo, payload.Categoria,
		payload.Nota, payload.Observacao,
		payload.RegisteredAt, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleNotasRegistradas: exec error: %w", err)
	}

	// Log informativo quando a nota já existia (replay/idempotência).
	if rows, _ := result.RowsAffected(); rows == 0 {
		log.Printf("[WARN] [notas] NotasRegistradas %s: nota já existia para estudante=%s periodo=%s materia=%s — atualizada via UPSERT",
			event.EventID, payload.CodigoEstudante, payload.Periodo, payload.MateriaDisciplinarID)
	}

	return nil
}

// handleNotaAtualizada processa o evento "NotaAtualizada".
func (p *NotasProjection) handleNotaAtualizada(event db.Event) error {
	var payload struct {
		CodigoEstudante      string    `json:"CodigoEstudante"`
		CodigoAcademia       string    `json:"CodigoAcademia"`
		AnoLectivo           string    `json:"AnoLectivo"`
		Periodo              string    `json:"Periodo"`
		MateriaDisciplinarID string    `json:"MateriaDisciplinarID"`
		Tipo                 string    `json:"Tipo"`
		Categoria            string    `json:"Categoria"`
		NotaAnterior         float64   `json:"NotaAnterior"`
		NotaNova             float64   `json:"NotaNova"`
		Observacao           string    `json:"Observacao"`
		UpdatedAt            time.Time `json:"UpdatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleNotaAtualizada: parse error: %w", err)
	}

	result, err := p.client.DB().Exec(`
		UPDATE projection_notas
		SET nota      = $1,
		    observacao = $2,
		    version    = $3,
		    event_id   = $4
		WHERE codigo_estudante      = $5
		  AND ano_lectivo           = $6
		  AND periodo               = $7
		  AND materia_disciplinar_id = $8
		  AND tipo                  = $9
		  AND categoria             = $10
	`,
		payload.NotaNova, payload.Observacao, event.EventVersion, event.EventID,
		payload.CodigoEstudante, payload.AnoLectivo, payload.Periodo,
		payload.MateriaDisciplinarID, payload.Tipo, payload.Categoria,
	)
	if err != nil {
		return fmt.Errorf("handleNotaAtualizada: exec error: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Nota não encontrada — pode acontecer em rebuild parcial ou ordem de eventos.
		// Não falha: o evento está no ledger; o rebuild completo resolverá.
		log.Printf("[WARN] [notas] NotaAtualizada %s: nota não encontrada para estudante=%s periodo=%s — ignorado",
			event.EventID, payload.CodigoEstudante, payload.Periodo)
	}
	return nil
}

// handleNotaDeletada processa o evento "NotaDeletada" — soft delete na projeção.
// Idempotente: se a nota já estiver deletada (deleted_at IS NOT NULL), não falha.
func (p *NotasProjection) handleNotaDeletada(event db.Event) error {
	var payload struct {
		NotaID string `json:"NotaID"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleNotaDeletada: parse error: %w", err)
	}

	result, err := p.client.DB().Exec(`
		UPDATE projection_notas
		SET deleted_at = NOW(),
		    version    = $1,
		    event_id   = $2
		WHERE id = $3
		  AND deleted_at IS NULL
	`, event.EventVersion, event.EventID, payload.NotaID)
	if err != nil {
		return fmt.Errorf("handleNotaDeletada: exec error: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("[WARN] [notas] NotaDeletada %s: nota id=%s não encontrada ou já deletada — ignorado",
			event.EventID, payload.NotaID)
	}
	return nil
}

// ============================================================================
// Queries de leitura
// ============================================================================

type NotaDTO struct {
	ID                   uuid.UUID  `json:"id"`
	CodigoEstudante      string     `json:"codigo_estudante"`
	CodigoAcademia       string     `json:"codigo_academia"`
	AnoLectivo           string     `json:"ano_lectivo"`
	AnoAcademico         string     `json:"ano_academico"`
	Periodo              string     `json:"periodo"`
	MateriaDisciplinarID string     `json:"materia_disciplinar_id"`
	MateriaNome          *string    `json:"materia_nome,omitempty"`
	Tipo                 string     `json:"tipo"`
	Categoria            string     `json:"categoria"`
	Nota                 float64    `json:"nota"`
	Observacao           *string    `json:"observacao,omitempty"`
	RegisteredAt         time.Time  `json:"registered_at"`
	EventID              uuid.UUID  `json:"event_id"`
	Version              int        `json:"version"`
}

func (p *NotasProjection) GetByEstudante(codigoEstudante string) ([]NotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.ano_academico,
			n.periodo, n.materia_disciplinar_id, m.nome,
			n.tipo, n.categoria, n.nota, n.observacao,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON m.id = n.materia_disciplinar_id::uuid
		WHERE n.codigo_estudante = $1
		  AND n.deleted_at IS NULL
		ORDER BY n.ano_lectivo DESC, n.periodo ASC
	`, codigoEstudante)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotas(rows)
}

func (p *NotasProjection) GetByAcademia(codigoAcademia string) ([]NotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.ano_academico,
			n.periodo, n.materia_disciplinar_id, m.nome,
			n.tipo, n.categoria, n.nota, n.observacao,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON m.id = n.materia_disciplinar_id::uuid
		WHERE n.codigo_academia = $1
		  AND n.deleted_at IS NULL
		ORDER BY n.registered_at DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotas(rows)
}

func scanNotas(rows *sql.Rows) ([]NotaDTO, error) {
	var notas []NotaDTO
	for rows.Next() {
		var n NotaDTO
		if err := rows.Scan(
			&n.ID, &n.CodigoEstudante, &n.CodigoAcademia, &n.AnoLectivo, &n.AnoAcademico,
			&n.Periodo, &n.MateriaDisciplinarID, &n.MateriaNome,
			&n.Tipo, &n.Categoria, &n.Nota, &n.Observacao,
			&n.RegisteredAt, &n.EventID, &n.Version,
		); err != nil {
			return nil, err
		}
		notas = append(notas, n)
	}
	return notas, rows.Err()
}

// GetNotaByID busca uma nota específica pelo UUID.
// Retorna nil sem erro quando a nota não existe ou foi soft-deleted.
// Usado por AtualizarNota para verificar ownership antes de aceitar correção.
func (p *NotasProjection) GetNotaByID(id uuid.UUID) (*NotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.ano_academico,
			n.periodo, n.materia_disciplinar_id, m.nome,
			n.tipo, n.categoria, n.nota, n.observacao,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON m.id = n.materia_disciplinar_id::uuid
		WHERE n.id = $1
		  AND n.deleted_at IS NULL
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notas, err := scanNotas(rows)
	if err != nil || len(notas) == 0 {
		return nil, err
	}
	return &notas[0], nil
}