package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// Projection
// ============================================================================

type AcademiaProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewAcademiaProjection(client *db.Client) *AcademiaProjection {
	return &AcademiaProjection{client: client, ctx: context.Background()}
}

func (p *AcademiaProjection) Name() string { return "academias" }

func (p *AcademiaProjection) Handle(event db.Event) error {
	if event.AggregateType != "Academia" {
		return nil
	}

	handlers := map[string]func(db.Event) error{
		"AcademiaCriada":           p.handleAcademiaCriada,
		"AcademiaAtivada":          p.handleStatusChange("ativo"),
		"AcademiaDesativada":       p.handleStatusChange("inativo"),
		"CursosAtualizados":        p.handleCursosAtualizados,
		"InscricaoAprovada":        p.handleInscricaoAprovada,
		"InscricaoReprovada":       p.handleInscricaoReprovada,
		"AcademiaDadosAtualizados": p.handleAcademiaDadosAtualizados,
		"EmailVerificado":          p.handleEmailVerificado,
	}

	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] Processando %s para %s", event.EventType, event.AggregateID)
		return handler(event)
	}
	return nil
}

func (p *AcademiaProjection) Rebuild() error {
	log.Printf("[DEBUG] Rebuild iniciado")

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
		if err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash); err != nil {
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

func (p *AcademiaProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))

	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *AcademiaProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, db.SafeString(p.Name()), eventID, eventID)

	_, err := p.client.DB().Exec(query)
	return err
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
		return fmt.Errorf("parse error: %w", err)
	}

	if event.AggregateID == uuid.Nil || payload.SenhaHash == "" {
		return fmt.Errorf("dados inválidos")
	}

	cursosJSON, _ := json.Marshal(payload.Cursos)

	// anos_academicos pode ser NULL (para superior / escolas de médio)
	var anosJSON string
	if len(payload.AnosAcademicos) > 0 {
		b, _ := json.Marshal(payload.AnosAcademicos)
		anosJSON = fmt.Sprintf("'%s'", db.SafeString(string(b)))
	} else {
		anosJSON = "NULL"
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_academias (
			id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, version, created_at, updated_at, last_event_id
		) VALUES (
			'%s', '%s', '%s', '%s', '%s', '%s', '%s',
			%s, %s, %s, %s, %s,
			'inativo', '%s', FALSE, %d, '%s', CURRENT_TIMESTAMP, '%s'
		)
		ON CONFLICT (id) DO UPDATE SET
			type            = EXCLUDED.type,
			nome            = EXCLUDED.nome,
			codigo_academia = EXCLUDED.codigo_academia,
			senha_hash      = EXCLUDED.senha_hash,
			provincia       = EXCLUDED.provincia,
			endereco        = EXCLUDED.endereco,
			numero_telefone = EXCLUDED.numero_telefone,
			email           = EXCLUDED.email,
			website         = EXCLUDED.website,
			nivel_escolar   = EXCLUDED.nivel_escolar,
			anos_academicos = EXCLUDED.anos_academicos,
			cursos          = EXCLUDED.cursos,
			version         = EXCLUDED.version,
			updated_at      = EXCLUDED.updated_at,
			last_event_id   = EXCLUDED.last_event_id
	`,
		event.AggregateID,
		db.SafeString(payload.Type), db.SafeString(payload.Nome),
		db.SafeString(payload.CodigoAcademia), db.SafeString(payload.SenhaHash),
		db.SafeString(payload.Provincia), db.SafeString(payload.Endereco),
		nullOrString(payload.NumeroTelefone), nullOrString(payload.Email),
		nullOrString(payload.Website), nullOrString(payload.NivelEscolar),
		anosJSON,
		db.SafeString(string(cursosJSON)),
		event.EventVersion,
		payload.CreatedAt.Format(time.RFC3339),
		event.EventID,
	)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleStatusChange(status string) func(db.Event) error {
	return func(event db.Event) error {
		if event.AggregateID == uuid.Nil {
			return fmt.Errorf("UUID inválido")
		}
		query := fmt.Sprintf(`
			UPDATE projection_academias
			SET status = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
			WHERE id = '%s'
		`, status, event.EventVersion, event.EventID, event.AggregateID)
		_, err := p.client.DB().Exec(query)
		return err
	}
}

func (p *AcademiaProjection) handleCursosAtualizados(event db.Event) error {
	var payload struct{ NovoCursos []string }
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	cursosJSON, _ := json.Marshal(payload.NovoCursos)

	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET cursos = '%s', version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, db.SafeString(string(cursosJSON)), event.EventVersion, event.EventID, event.AggregateID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleInscricaoAprovada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET total_estudantes = total_estudantes + 1,
			total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleInscricaoReprovada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleAcademiaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome           *string
		Provincia      *string
		Endereco       *string
		NumeroTelefone *string
		Email          *string
		Website        *string
		NivelEscolar   *string
		AnosAcademicos []string
		Cursos         []string
		EmailAlterado  bool
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}

	// Colunas de string simples
	stringFields := map[string]*string{
		"nome":            payload.Nome,
		"provincia":       payload.Provincia,
		"endereco":        payload.Endereco,
		"numero_telefone": payload.NumeroTelefone,
		"website":         payload.Website,
		"nivel_escolar":   payload.NivelEscolar,
	}

	var setClauses []string
	for col, val := range stringFields {
		if val != nil {
			setClauses = append(setClauses, fmt.Sprintf("%s = '%s'", col, db.SafeString(*val)))
		}
	}

	if payload.Email != nil {
		setClauses = append(setClauses, fmt.Sprintf("email = '%s'", db.SafeString(*payload.Email)))
		if payload.EmailAlterado {
			setClauses = append(setClauses, "email_verificado = FALSE")
		}
	}

	// anos_academicos — só atualiza se o evento trouxer valor não-nil
	// ([]string{} com len 0 é distinto de nil: significa "limpar o campo")
	if payload.AnosAcademicos != nil {
		if len(payload.AnosAcademicos) > 0 {
			b, _ := json.Marshal(payload.AnosAcademicos)
			setClauses = append(setClauses, fmt.Sprintf("anos_academicos = '%s'", db.SafeString(string(b))))
		} else {
			setClauses = append(setClauses, "anos_academicos = NULL")
		}
	}

	if payload.Cursos != nil {
		b, _ := json.Marshal(payload.Cursos)
		setClauses = append(setClauses, fmt.Sprintf("cursos = '%s'", db.SafeString(string(b))))
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses,
		fmt.Sprintf("version = %d", event.EventVersion),
		"updated_at = CURRENT_TIMESTAMP",
		fmt.Sprintf("last_event_id = '%s'", event.EventID),
	)

	query := fmt.Sprintf(`UPDATE projection_academias SET %s WHERE id = '%s'`,
		joinClauses(setClauses), event.AggregateID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleEmailVerificado(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET email_verificado = TRUE, version = %d, updated_at = CURRENT_TIMESTAMP, last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID, event.AggregateID)
	_, err := p.client.DB().Exec(query)
	return err
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

// ============================================================================
// Query / DTO
// ============================================================================

func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	return p.queryAcademia(fmt.Sprintf("id = '%s'", id))
}

// GetByCodigoOrEmail busca academia por codigo_academia OU email.
// Usado no login onde o utilizador pode inserir qualquer um dos dois.
func (p *AcademiaProjection) GetByCodigoOrEmail(identifier string) (*AcademiaDTO, error) {
	safe := db.SafeString(identifier)
	return p.queryAcademia(fmt.Sprintf("codigo_academia = '%s' OR email = '%s'", safe, safe))
}

func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	return p.queryAcademia(fmt.Sprintf("codigo_academia = '%s'", db.SafeString(codigo)))
}

func (p *AcademiaProjection) GetByEmail(email string) (*AcademiaDTO, error) {
	return p.queryAcademia(fmt.Sprintf("email = '%s'", db.SafeString(email)))
}

func (p *AcademiaProjection) queryAcademia(whereClause string) (*AcademiaDTO, error) {
	query := fmt.Sprintf(`
		SELECT id, type, nome, codigo_academia, senha_hash, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, anos_academicos,
			status, cursos, email_verificado, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias WHERE %s LIMIT 1`, whereClause)

	var dto AcademiaDTO
	var cursosJSON []byte
	var anosJSON []byte

	err := p.client.DB().QueryRow(query).Scan(
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

// AcademiaDTO representa a projeção lida de projection_academias.
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
	// AnosAcademicos — anos do ensino fundamental oferecidos (nil para superior/médio)
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