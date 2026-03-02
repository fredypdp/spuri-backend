package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AcademiaProjection struct {
	client *db.Client
}

func NewAcademiaProjection(client *db.Client) *AcademiaProjection {
	return &AcademiaProjection{client: client}
}

func (p *AcademiaProjection) Name() string { return "academias" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *AcademiaProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *AcademiaProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *AcademiaProjection) Handle(event db.Event) error {
	if event.AggregateType != "Academia" {
		return nil
	}
	handlers := map[string]func(db.Event) error{
		"AcademiaCriada": p.handleAcademiaCriada,
		// BUG #A FIX — era "AcademiaAtualizada" (nome errado):
		"AcademiaDadosAtualizados": p.handleAcademiaDadosAtualizados,
		// BUG #A FIX — estavam completamente ausentes:
		"AcademiaAtivada":    p.handleAcademiaAtivada,
		"AcademiaDesativada": p.handleAcademiaDesativada,
		"CursosAtualizados":  p.handleCursosAtualizados,
		// Mantidos da versão anterior:
		"EmailVerificado":                         p.handleEmailVerificado,
		"ContadorEstudantesAtualizado":            p.handleContadorEstudantes,
		"ContadorInscricoesPendentesAtualizado":   p.handleContadorInscricoes,
	}
	if handler, ok := handlers[event.EventType]; ok {
		return handler(event)
	}
	return nil
}

func (p *AcademiaProjection) Rebuild() error {
	log.Printf("[DEBUG] [academias] Rebuild iniciado")
	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Academia'
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var event db.Event
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		); err != nil {
			return err
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro no evento %d: %w", event.ID, err)
		}
		count++
	}
	log.Printf("[DEBUG] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

func (p *AcademiaProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_academias CASCADE`)
	return err
}

// ============================================================================
// Event handlers
// ============================================================================

func (p *AcademiaProjection) handleAcademiaCriada(event db.Event) error {
	var payload struct {
		Type, Nome, CodigoAcademia, SenhaHash, Provincia, Endereco string
		NumeroTelefone, Email, Website, NivelEscolar                *string
		AnosAcademicos                                              []string
		Cursos                                                      []string
		CreatedAt                                                   time.Time
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error AcademiaCriada: %w", err)
	}

	cursosJSON, _ := json.Marshal(payload.Cursos)
	var anosJSON interface{}
	if len(payload.AnosAcademicos) > 0 {
		b, _ := json.Marshal(payload.AnosAcademicos)
		anosJSON = string(b)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_academias (
			id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, created_at, updated_at, version, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12,
			'inativo', $13, FALSE, $14, CURRENT_TIMESTAMP, $15, $16
		)
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID, payload.Type, payload.Nome, payload.CodigoAcademia,
		payload.SenhaHash, payload.Provincia, payload.Endereco,
		payload.NumeroTelefone, payload.Email, payload.Website, payload.NivelEscolar, anosJSON,
		string(cursosJSON), payload.CreatedAt, event.EventVersion, event.EventID,
	)
	return err
}

func (p *AcademiaProjection) handleAcademiaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome           *string
		Provincia      *string
		Endereco       *string
		NumeroTelefone *string
		Website        *string
		NivelEscolar   *string
		Email          *string
		EmailAlterado  bool
		AnosAcademicos []string
		Cursos         []string
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	type fieldUpdate struct {
		col string
		val interface{}
	}
	var updates []fieldUpdate

	if payload.Nome != nil {
		updates = append(updates, fieldUpdate{"nome", *payload.Nome})
	}
	if payload.Provincia != nil {
		updates = append(updates, fieldUpdate{"provincia", *payload.Provincia})
	}
	if payload.Endereco != nil {
		updates = append(updates, fieldUpdate{"endereco", *payload.Endereco})
	}
	if payload.NumeroTelefone != nil {
		updates = append(updates, fieldUpdate{"numero_telefone", *payload.NumeroTelefone})
	}
	if payload.Website != nil {
		updates = append(updates, fieldUpdate{"website", *payload.Website})
	}
	if payload.NivelEscolar != nil {
		updates = append(updates, fieldUpdate{"nivel_escolar", *payload.NivelEscolar})
	}
	if payload.Email != nil {
		updates = append(updates, fieldUpdate{"email", *payload.Email})
		if payload.EmailAlterado {
			p.client.DB().Exec(
				`UPDATE projection_academias SET email_verificado = FALSE WHERE id = $1`,
				event.AggregateID,
			)
		}
	}
	if payload.AnosAcademicos != nil {
		if len(payload.AnosAcademicos) > 0 {
			b, _ := json.Marshal(payload.AnosAcademicos)
			updates = append(updates, fieldUpdate{"anos_academicos", string(b)})
		} else {
			p.client.DB().Exec(
				`UPDATE projection_academias SET anos_academicos = NULL WHERE id = $1`,
				event.AggregateID,
			)
		}
	}
	if payload.Cursos != nil {
		b, _ := json.Marshal(payload.Cursos)
		updates = append(updates, fieldUpdate{"cursos", string(b)})
	}

	for _, u := range updates {
		query := fmt.Sprintf(`UPDATE projection_academias SET %s = $1 WHERE id = $2`, u.col)
		if _, err := p.client.DB().Exec(query, u.val, event.AggregateID); err != nil {
			return err
		}
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AcademiaProjection) handleEmailVerificado(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET email_verificado = TRUE, version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

func (p *AcademiaProjection) handleContadorEstudantes(event db.Event) error {
	var payload struct{ Total int }
	json.Unmarshal(event.Payload, &payload)
	_, err := p.client.DB().Exec(
		`UPDATE projection_academias SET total_estudantes = $1 WHERE id = $2`,
		payload.Total, event.AggregateID,
	)
	return err
}

func (p *AcademiaProjection) handleContadorInscricoes(event db.Event) error {
	var payload struct{ Total int }
	json.Unmarshal(event.Payload, &payload)
	_, err := p.client.DB().Exec(
		`UPDATE projection_academias SET total_inscricoes_pendentes = $1 WHERE id = $2`,
		payload.Total, event.AggregateID,
	)
	return err
}

// handleAcademiaAtivada — BUG #A FIX (NOVO).
// Atualiza status para 'ativo' quando admin executa Ativar().
func (p *AcademiaProjection) handleAcademiaAtivada(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET status = 'ativo',
			version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// Atualiza status para 'inativo' quando admin executa Desativar().
func (p *AcademiaProjection) handleAcademiaDesativada(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET status = 'inativo',
			version = $1, updated_at = CURRENT_TIMESTAMP, last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// Atualiza o campo cursos[] da academia quando AtualizarCursos() é executado.
func (p *AcademiaProjection) handleCursosAtualizados(event db.Event) error {
	var payload struct {
		NovoCursos []string `json:"NovoCursos"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error CursosAtualizados: %w", err)
	}
	cursosJSON, _ := json.Marshal(payload.NovoCursos)
	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET cursos = $1,
			version = $2, updated_at = CURRENT_TIMESTAMP, last_event_id = $3
		WHERE id = $4
	`, string(cursosJSON), event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Query / DTO
// ============================================================================

type AcademiaDTO struct {
	ID                       uuid.UUID `json:"id"`
	Type                     string    `json:"type"`
	Nome                     string    `json:"nome"`
	CodigoAcademia           string    `json:"codigo_academia"`
	SenhaHash                string    `json:"-"`
	Provincia                string    `json:"provincia"`
	Endereco                 string    `json:"endereco"`
	NumeroTelefone           *string   `json:"numero_telefone,omitempty"`
	Email                    *string   `json:"email,omitempty"`
	Website                  *string   `json:"website,omitempty"`
	NivelEscolar             *string   `json:"nivel_escolar,omitempty"`
	AnosAcademicos           []string  `json:"anos_academicos,omitempty"`
	Status                   string    `json:"status"`
	Cursos                   []string  `json:"cursos"`
	EmailVerificado          bool      `json:"email_verificado"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
	TotalEstudantes          int       `json:"total_estudantes"`
	TotalInscricoesPendentes int       `json:"total_inscricoes_pendentes"`
	Version                  int       `json:"version"`
}

func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	return p.queryAcademiaByField("id", id.String())
}

func (p *AcademiaProjection) GetByCodigoOrEmail(identifier string) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = $1 OR email = $1
		LIMIT 1
	`, identifier)
	return scanAcademia(row)
}

func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	return p.queryAcademiaByField("codigo_academia", codigo)
}

func (p *AcademiaProjection) GetByEmail(email string) (*AcademiaDTO, error) {
	return p.queryAcademiaByField("email", email)
}

func (p *AcademiaProjection) GetAll() ([]AcademiaDTO, error) {
	rows, err := p.client.DB().Query(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE deleted_at IS NULL
		ORDER BY nome ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AcademiaDTO
	for rows.Next() {
		var dto AcademiaDTO
		var cursosJSON, anosJSON []byte
		if err := rows.Scan(
			&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia, &dto.SenhaHash,
			&dto.Provincia, &dto.Endereco, &dto.NumeroTelefone, &dto.Email,
			&dto.Website, &dto.NivelEscolar, &anosJSON,
			&dto.Status, &cursosJSON,
			&dto.EmailVerificado, &dto.CreatedAt, &dto.UpdatedAt,
			&dto.TotalEstudantes, &dto.TotalInscricoesPendentes, &dto.Version,
		); err != nil {
			return nil, fmt.Errorf("erro ao escanear academia: %w", err)
		}
		json.Unmarshal(cursosJSON, &dto.Cursos)
		if len(anosJSON) > 0 {
			json.Unmarshal(anosJSON, &dto.AnosAcademicos)
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

func (p *AcademiaProjection) queryAcademiaByField(field, value string) (*AcademiaDTO, error) {
	// field is an internal constant (never user-provided), safe to interpolate
	query := fmt.Sprintf(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias WHERE %s = $1 LIMIT 1`,
		strings.ReplaceAll(field, "'", ""), // col name is internal, extra safety
	)
	row := p.client.DB().QueryRow(query, value)
	return scanAcademia(row)
}

func scanAcademia(row *sql.Row) (*AcademiaDTO, error) {
	var dto AcademiaDTO
	var cursosJSON, anosJSON []byte
	err := row.Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia, &dto.SenhaHash,
		&dto.Provincia, &dto.Endereco, &dto.NumeroTelefone, &dto.Email,
		&dto.Website, &dto.NivelEscolar, &anosJSON,
		&dto.Status, &cursosJSON,
		&dto.EmailVerificado, &dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(cursosJSON, &dto.Cursos)
	if len(anosJSON) > 0 {
		json.Unmarshal(anosJSON, &dto.AnosAcademicos)
	}
	return &dto, nil
}

// joinClauses une cláusulas SET com vírgula.
func joinClauses(clauses []string) string {
	result := ""
	for i, c := range clauses {
		if i > 0 {
			result += ", "
		}
		result += c
	}
	return result
}