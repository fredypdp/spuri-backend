package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"spuri/internal/db"
)

type NotasProjection struct {
	client *db.Client
}

func NewNotasProjection(client *db.Client) *NotasProjection {
	return &NotasProjection{client: client}
}

func (p *NotasProjection) Name() string { return "notas" }

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
// Rebuild
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

// handleNotasRegistradas insere ou atualiza uma nota na projeção.
//
// FIX PROJ-NOTA-03: o ON CONFLICT anterior usava colunas
// (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id)
// que NÃO correspondem à constraint uq_nota_unica do banco, definida em migration 006 como:
//
//	UNIQUE (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
//
// Com as colunas erradas o ON CONFLICT nunca disparava, resultando em violação
// 23505 (unique_violation) em rebuild ou double-submit. Corrigido para usar
// exatamente as colunas da constraint, incluindo tipo e categoria.
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
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
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

	if rows, _ := result.RowsAffected(); rows == 0 {
		log.Printf("[WARN] [notas] NotasRegistradas %s: nota já existia para estudante=%s periodo=%s tipo=%s categoria=%s — atualizada via UPSERT",
			event.EventID, payload.CodigoEstudante, payload.Periodo, payload.Tipo, payload.Categoria)
	}

	return nil
}

// handleNotaAtualizada processa o evento "NotaAtualizada".
// Idempotente: se a nota não for encontrada (rebuild parcial ou ordem de eventos),
// loga WARN mas não falha — o rebuild completo resolverá.
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
		SET nota       = $1,
		    observacao = $2,
		    version    = $3,
		    event_id   = $4
		WHERE codigo_estudante       = $5
		  AND codigo_academia        = $6
		  AND ano_lectivo            = $7
		  AND periodo                = $8
		  AND materia_disciplinar_id = $9
		  AND tipo                   = $10
		  AND categoria              = $11
		  AND deleted_at IS NULL
	`,
		payload.NotaNova, payload.Observacao, event.EventVersion, event.EventID,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.Periodo,
		payload.MateriaDisciplinarID, payload.Tipo, payload.Categoria,
	)
	if err != nil {
		return fmt.Errorf("handleNotaAtualizada: exec error: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("[WARN] [notas] NotaAtualizada %s: nota não encontrada para estudante=%s periodo=%s tipo=%s categoria=%s — ignorado",
			event.EventID, payload.CodigoEstudante, payload.Periodo, payload.Tipo, payload.Categoria)
	}
	return nil
}

// handleNotaDeletada processa o evento "NotaDeletada" — soft delete na projeção.
// Idempotente: se a nota já estiver deletada (deleted_at IS NOT NULL), não falha.
//
// FIX PROJ-NOTA-01: DeletadoPor e Motivo lidos do payload e gravados em
// deletado_por e motivo_exclusao — permite consulta direta de auditoria sem
// inspecionar o spuri_ledger.
//
// FIX PROJ-NOTA-02: deleted_at usa payload.DeletedAt em vez de NOW(),
// preservando o timestamp real da deleção em rebuilds.
func (p *NotasProjection) handleNotaDeletada(event db.Event) error {
	var payload struct {
		NotaID      string    `json:"NotaID"`
		DeletadoPor uuid.UUID `json:"DeletadoPor"`
		Motivo      string    `json:"Motivo"`
		DeletedAt   time.Time `json:"DeletedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleNotaDeletada: parse error: %w", err)
	}

	// Fallback para OccurredAt em eventos antigos que não tenham DeletedAt preenchido.
	deletedAt := payload.DeletedAt
	if deletedAt.IsZero() {
		deletedAt = event.OccurredAt
	}

	result, err := p.client.DB().Exec(`
		UPDATE projection_notas
		SET deleted_at      = $1,
		    deletado_por    = $2,
		    motivo_exclusao = $3,
		    version         = $4,
		    event_id        = $5
		WHERE id = $6
		  AND deleted_at IS NULL
	`, deletedAt.UTC(), payload.DeletadoPor, payload.Motivo, event.EventVersion, event.EventID, payload.NotaID)
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
	ID                   uuid.UUID `json:"id"`
	CodigoEstudante      string    `json:"codigo_estudante"`
	CodigoAcademia       string    `json:"codigo_academia"`
	AnoLectivo           string    `json:"ano_lectivo"`
	AnoAcademico         string    `json:"ano_academico"`
	Periodo              string    `json:"periodo"`
	MateriaDisciplinarID string    `json:"materia_disciplinar_id"`
	MateriaNome          *string   `json:"materia_nome,omitempty"`
	Tipo                 string    `json:"tipo"`
	Categoria            string    `json:"categoria"`
	Nota                 float64   `json:"nota"`
	Observacao           *string   `json:"observacao,omitempty"`
	RegisteredAt         time.Time `json:"registered_at"`
	EventID              uuid.UUID `json:"event_id"`
	Version              int       `json:"version"`
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

// GetNotaByID busca uma nota específica pelo UUID.
// Retorna nil sem erro quando a nota não existe ou foi soft-deleted.
// Usado por AtualizarNota e DeletarNota para verificar ownership antes do comando.
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
