package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/utils"
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
		"NotaCorrigida":    p.handleNotaCorrigida,
	}
	if handler, ok := handlers[event.EventType]; ok {
		utils.Debugf("[DEBUG] [notas] Processando %s: %s", event.EventType, event.EventID)
		return handler(event)
	}
	return nil
}

// ============================================================================
// Rebuild
// ============================================================================

func (p *NotasProjection) Rebuild() error {
	utils.Debugf("[DEBUG] [notas] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('NotasRegistradas', 'NotaCorrigida')
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

	utils.Debugf("[DEBUG] [notas] Rebuild concluído: %d eventos", count)
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
		RegistradoPor        uuid.UUID `json:"RegistradoPor"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleNotasRegistradas: parse error: %w", err)
	}

	result, err := p.client.DB().Exec(`
		INSERT INTO projection_notas (
			codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			periodo, materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, registrado_por, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
		DO NOTHING
	`,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Periodo, payload.MateriaDisciplinarID, payload.Tipo, payload.Categoria,
		payload.Nota, payload.Observacao,
		payload.RegisteredAt, payload.RegistradoPor, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleNotasRegistradas: exec error: %w", err)
	}

	if rows, _ := result.RowsAffected(); rows == 0 {
		utils.Debugf("[notas] evento duplicado ignorado: %s", event.EventID)
	}

	return nil
}

func (p *NotasProjection) handleNotaCorrigida(event db.Event) error {
	var payload struct {
		NotaAnteriorID uuid.UUID `json:"NotaAnteriorID"`
		NovaNota       float64   `json:"NovaNota"`
		NovaObservacao *string   `json:"NovaObservacao"`
		Motivo         string    `json:"Motivo"`
		CorrigidoPor   uuid.UUID `json:"CorrigidoPor"`
		CorrigidoEm    time.Time `json:"CorrigidoEm"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleNotaCorrigida: parse error: %w", err)
	}
	result, err := p.client.DB().Exec(`UPDATE projection_notas SET nota=$2, observacao=$3, valor_anterior=nota, motivo_correcao=$4, corrigido_por=$5, corrigido_em=$6, event_id=$7, version=$8 WHERE id=$1`, payload.NotaAnteriorID, payload.NovaNota, payload.NovaObservacao, payload.Motivo, payload.CorrigidoPor, payload.CorrigidoEm, event.EventID, event.EventVersion)
	if err != nil {
		return fmt.Errorf("handleNotaCorrigida: exec error: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return fmt.Errorf("handleNotaCorrigida: nota original %s não encontrada", payload.NotaAnteriorID)
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
	RegistradoPor        uuid.UUID  `json:"registrado_por"`
	ValorAnterior        *float64   `json:"valor_anterior,omitempty"`
	MotivoCorrecao       *string    `json:"motivo_correcao,omitempty"`
	CorrigidoPor         *uuid.UUID `json:"corrigido_por,omitempty"`
	CorrigidoEm          *time.Time `json:"corrigido_em,omitempty"`
	RegisteredAt         time.Time  `json:"registered_at"`
	EventID              uuid.UUID  `json:"event_id"`
	Version              int        `json:"version"`
}

func (p *NotasProjection) GetByEstudante(codigoEstudante string) ([]NotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.ano_academico,
			n.periodo, n.materia_disciplinar_id, m.nome,
			n.tipo, n.categoria, n.nota, n.observacao, n.registrado_por, n.valor_anterior, n.motivo_correcao, n.corrigido_por, n.corrigido_em,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON m.id = n.materia_disciplinar_id::uuid
		WHERE n.codigo_estudante = $1
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
			n.tipo, n.categoria, n.nota, n.observacao, n.registrado_por, n.valor_anterior, n.motivo_correcao, n.corrigido_por, n.corrigido_em,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON m.id = n.materia_disciplinar_id::uuid
		WHERE n.codigo_academia = $1
		ORDER BY n.registered_at DESC
	`, codigoAcademia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotas(rows)
}

// GetNotaByID busca uma nota específica pelo UUID.
// Retorna nil sem erro quando a nota não existe.
func (p *NotasProjection) GetNotaByID(id uuid.UUID) (*NotaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT n.id, n.codigo_estudante, n.codigo_academia, n.ano_lectivo, n.ano_academico,
			n.periodo, n.materia_disciplinar_id, m.nome,
			n.tipo, n.categoria, n.nota, n.observacao, n.registrado_por, n.valor_anterior, n.motivo_correcao, n.corrigido_por, n.corrigido_em,
			n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_materias m ON m.id = n.materia_disciplinar_id::uuid
		WHERE n.id = $1
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
			&n.Tipo, &n.Categoria, &n.Nota, &n.Observacao, &n.RegistradoPor, &n.ValorAnterior, &n.MotivoCorrecao, &n.CorrigidoPor, &n.CorrigidoEm,
			&n.RegisteredAt, &n.EventID, &n.Version,
		); err != nil {
			return nil, err
		}
		notas = append(notas, n)
	}
	return notas, rows.Err()
}
