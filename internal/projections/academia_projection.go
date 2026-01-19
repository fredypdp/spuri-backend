// ============================================================================
// ARQUIVO: internal/projections/academia_projection.go
// 🔥 CORRIGIDO: TODAS as queries usando formato direto sem prepared statements
// ============================================================================

package projections

import (
	"context"
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
	ctx    context.Context
}

func NewAcademiaProjection(client *db.Client) *AcademiaProjection {
	return &AcademiaProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *AcademiaProjection) Name() string {
	return "academias"
}

func (p *AcademiaProjection) Handle(event db.Event) error {
	if event.AggregateType != "Academia" {
		return nil
	}

	switch event.EventType {
	case "AcademiaCriada":
		return p.handleAcademiaCriada(event)
	case "AcademiaAtivada":
		return p.handleAcademiaAtivada(event)
	case "AcademiaDesativada":
		return p.handleAcademiaDesativada(event)
	case "CursosAtualizados":
		return p.handleCursosAtualizados(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "InscricaoReprovada":
		return p.handleInscricaoReprovada(event)
	case "AcademiaDadosAtualizados":
		return p.handleAcademiaDadosAtualizados(event)
	default:
		return nil
	}
}

func (p *AcademiaProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Academia'
		ORDER BY id ASC
	`

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var event db.Event
		err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &event.PreviousHash,
		)
		if err != nil {
			return err
		}

		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}

	return rows.Err()
}

func (p *AcademiaProjection) GetLastProcessedEventID() (int64, error) {
	query := fmt.Sprintf(`
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, p.Name())

	var lastID int64
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

func (p *AcademiaProjection) UpdateCheckpoint(eventID int64) error {
	query := fmt.Sprintf(`
		INSERT INTO projection_checkpoints (
			projection_name, 
			last_processed_event_id, 
			last_processed_at,
			events_processed
		) VALUES ('%s', %d, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) 
		DO UPDATE SET
			last_processed_event_id = %d,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, p.Name(), eventID, eventID)

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_academias CASCADE`)
	return err
}

func (p *AcademiaProjection) handleAcademiaCriada(event db.Event) error {
	log.Printf("🔵 [PROJEÇÃO ACADEMIA] Processando AcademiaCriada")

	var payload struct {
		Type           string    `json:"Type"`
		Nome           string    `json:"Nome"`
		CodigoAcademia string    `json:"CodigoAcademia"`
		SenhaHash      string    `json:"SenhaHash"`
		Provincia      string    `json:"Provincia"`
		Endereco       string    `json:"Endereco"`
		NumeroTelefone *string   `json:"NumeroTelefone"`
		Email          *string   `json:"Email"`
		Website        *string   `json:"Website"`
		NivelEscolar   *string   `json:"NivelEscolar"`
		Cursos         []string  `json:"Cursos"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.SenhaHash == "" {
		return fmt.Errorf("SenhaHash vazio no evento")
	}

	cursosJSON, err := json.Marshal(payload.Cursos)
	if err != nil {
		return err
	}

	// ✅ STATUS SEMPRE 'inativo' AO CRIAR
	query := fmt.Sprintf(`
		INSERT INTO projection_academias (
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, version, created_at, updated_at, last_event_id
		) VALUES (
			'%s', '%s', '%s', '%s', '%s', '%s',
			'%s', %s, %s, %s, %s,
			'inativo', '%s', %d, '%s', '%s', '%s'
		)
		ON CONFLICT (id) DO UPDATE SET
			type = EXCLUDED.type,
			nome = EXCLUDED.nome,
			codigo_academia = EXCLUDED.codigo_academia,
			senha_hash = EXCLUDED.senha_hash,
			provincia = EXCLUDED.provincia,
			endereco = EXCLUDED.endereco,
			numero_telefone = EXCLUDED.numero_telefone,
			email = EXCLUDED.email,
			website = EXCLUDED.website,
			nivel_escolar = EXCLUDED.nivel_escolar,
			cursos = EXCLUDED.cursos,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			last_event_id = EXCLUDED.last_event_id
	`,
		event.AggregateID.String(),
		payload.Type,
		escapeString(payload.Nome),
		payload.CodigoAcademia,
		payload.SenhaHash,
		payload.Provincia,
		escapeString(payload.Endereco),
		formatNullableString(payload.NumeroTelefone),
		formatNullableString(payload.Email),
		formatNullableString(payload.Website),
		formatNullableString(payload.NivelEscolar),
		escapeString(string(cursosJSON)),
		event.EventVersion,
		payload.CreatedAt.Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
		event.EventID.String(),
	)

	_, err = p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleAcademiaAtivada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET 
			status = 'ativo',
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID.String(), event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleAcademiaDesativada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET 
			status = 'inativo',
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s'
		WHERE id = '%s'
	`, event.EventVersion, event.EventID.String(), event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleCursosAtualizados(event db.Event) error {
	var payload struct {
		NovoCursos []string `json:"NovoCursos"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	cursosJSON, err := json.Marshal(payload.NovoCursos)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET 
			cursos = '%s',
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s'
		WHERE id = '%s'
	`,
		escapeString(string(cursosJSON)),
		event.EventVersion,
		event.EventID.String(),
		event.AggregateID.String(),
	)

	_, err = p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleInscricaoAprovada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET 
			total_estudantes = total_estudantes + 1,
			total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

func (p *AcademiaProjection) handleInscricaoReprovada(event db.Event) error {
	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET 
			total_inscricoes_pendentes = GREATEST(total_inscricoes_pendentes - 1, 0),
			updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'
	`, event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}

// Query methods - 🔥 TODAS CORRIGIDAS

func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE id = '%s'
	`, id.String())

	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia,
		&dto.SenhaHash, &dto.Provincia, &dto.Endereco,
		&dto.NumeroTelefone, &dto.Email, &dto.Website,
		&dto.NivelEscolar, &dto.Status, &cursosJSON,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes,
		&dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(cursosJSON, &dto.Cursos); err != nil {
		dto.Cursos = []string{}
	}

	return &dto, nil
}

func (p *AcademiaProjection) GetByCodigoOrEmail(identifier string) (*AcademiaDTO, error) {
	// 🔥 CORRIGIDO: Query direta
	query := fmt.Sprintf(`
		SELECT 
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = '%s' OR email = '%s'
		LIMIT 1
	`, identifier, identifier)

	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia,
		&dto.SenhaHash, &dto.Provincia, &dto.Endereco,
		&dto.NumeroTelefone, &dto.Email, &dto.Website,
		&dto.NivelEscolar, &dto.Status, &cursosJSON,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes,
		&dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(cursosJSON, &dto.Cursos); err != nil {
		dto.Cursos = []string{}
	}

	return &dto, nil
}

func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	// 🔥 CORRIGIDO: Query direta
	query := fmt.Sprintf(`
		SELECT 
			id, type, nome, codigo_academia, senha_hash, provincia,
			endereco, numero_telefone, email, website, nivel_escolar,
			status, cursos, created_at, updated_at,
			total_estudantes, total_inscricoes_pendentes, version
		FROM projection_academias
		WHERE codigo_academia = '%s'
		LIMIT 1
	`, codigo)

	var dto AcademiaDTO
	var cursosJSON []byte

	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Type, &dto.Nome, &dto.CodigoAcademia,
		&dto.SenhaHash, &dto.Provincia, &dto.Endereco,
		&dto.NumeroTelefone, &dto.Email, &dto.Website,
		&dto.NivelEscolar, &dto.Status, &cursosJSON,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalEstudantes, &dto.TotalInscricoesPendentes,
		&dto.Version,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(cursosJSON, &dto.Cursos); err != nil {
		dto.Cursos = []string{}
	}

	return &dto, nil
}

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
	Status                   string    `json:"status"`
	Cursos                   []string  `json:"cursos"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
	TotalEstudantes          int       `json:"total_estudantes"`
	TotalInscricoesPendentes int       `json:"total_inscricoes_pendentes"`
	Version                  int       `json:"version"`
}

// Helpers

func escapeString(s string) string {
	result := ""
	for _, char := range s {
		if char == '\'' {
			result += "''"
		} else if char == '\\' {
			result += "\\\\"
		} else {
			result += string(char)
		}
	}
	return result
}

func formatNullableString(s *string) string {
	if s == nil {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", escapeString(*s))
}

func (p *AcademiaProjection) handleAcademiaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome           *string  `json:"Nome"`
		Provincia      *string  `json:"Provincia"`
		Endereco       *string  `json:"Endereco"`
		NumeroTelefone *string  `json:"NumeroTelefone"`
		Email          *string  `json:"Email"`
		Website        *string  `json:"Website"`
		NivelEscolar   *string  `json:"NivelEscolar"`
		Cursos         []string `json:"Cursos"`
		EmailAlterado  bool     `json:"EmailAlterado"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	updates := []string{}
	if payload.Nome != nil {
		updates = append(updates, fmt.Sprintf("nome = '%s'", escapeString(*payload.Nome)))
	}
	if payload.Provincia != nil {
		updates = append(updates, fmt.Sprintf("provincia = '%s'", *payload.Provincia))
	}
	if payload.Endereco != nil {
		updates = append(updates, fmt.Sprintf("endereco = '%s'", escapeString(*payload.Endereco)))
	}
	if payload.NumeroTelefone != nil {
		updates = append(updates, fmt.Sprintf("numero_telefone = '%s'", *payload.NumeroTelefone))
	}
	if payload.Email != nil {
		updates = append(updates, fmt.Sprintf("email = '%s'", *payload.Email))
		if payload.EmailAlterado {
			updates = append(updates, "email_verificado = FALSE")
		}
	}
	if payload.Website != nil {
		updates = append(updates, fmt.Sprintf("website = '%s'", escapeString(*payload.Website)))
	}
	if payload.NivelEscolar != nil {
		updates = append(updates, fmt.Sprintf("nivel_escolar = '%s'", *payload.NivelEscolar))
	}
	if payload.Cursos != nil {
		cursosJSON, _ := json.Marshal(payload.Cursos)
		updates = append(updates, fmt.Sprintf("cursos = '%s'", escapeString(string(cursosJSON))))
	}

	if len(updates) == 0 {
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_academias
		SET 
			%s,
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s'
		WHERE id = '%s'
	`, strings.Join(updates, ", "), event.EventVersion, event.EventID.String(), event.AggregateID.String())

	_, err := p.client.DB().Exec(query)
	return err
}