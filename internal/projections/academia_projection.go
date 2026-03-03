// ============================================================================
// ARQUIVO: internal/projections/academia_projection.go
// ============================================================================

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

// Handle processa eventos do ledger.
func (p *AcademiaProjection) Handle(event db.Event) error {
	if event.AggregateType == "Academia" {
		academiaHandlers := map[string]func(db.Event) error{
			"AcademiaCriada":           p.handleAcademiaCriada,
			"AcademiaAtivada":          p.handleStatusChange("ativo"),
			"AcademiaDesativada":       p.handleStatusChange("inativo"),
			"CursosAtualizados":        p.handleCursosAtualizados,
			"AcademiaDadosAtualizados": p.handleAcademiaDadosAtualizados,
			"EmailVerificado":          p.handleEmailVerificado,
			// CategoriaNotaAdicionada é tratado pela CategoriasNotaProjection dedicada.
		}

		if handler, ok := academiaHandlers[event.EventType]; ok {
			log.Printf("[DEBUG] [academias] Processando %s para %s", event.EventType, event.AggregateID)
			return handler(event)
		}
		return nil
	}

	// Evento EstudanteCriadoComVinculo: incrementa total_estudantes
	if event.AggregateType == "Estudante" && event.EventType == "EstudanteCriadoComVinculo" {
		return p.handleEstudanteCriadoComVinculo(event)
	}

	return nil
}

// ============================================================================
// Rebuild
// ============================================================================

func (p *AcademiaProjection) Rebuild() error {
	log.Printf("[DEBUG] [academias] Rebuild iniciado")

	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar projection_academias: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Academia'
		   OR (aggregate_type = 'Estudante' AND event_type = 'EstudanteCriadoComVinculo')
		ORDER BY id ASC
	`)
	if err != nil {
		return fmt.Errorf("erro ao buscar eventos para rebuild: %w", err)
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
			return fmt.Errorf("erro ao processar evento %d (type=%s): %w", event.ID, event.EventType, err)
		}
		count++
	}

	log.Printf("[DEBUG] [academias] Rebuild concluído: %d eventos processados", count)
	return rows.Err()
}

// ============================================================================
// Checkpoint
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

// ============================================================================
// Handlers de evento
// ============================================================================

func (p *AcademiaProjection) handleAcademiaCriada(event db.Event) error {
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
		AnosAcademicos []string  `json:"AnosAcademicos"`
		Cursos         []string  `json:"Cursos"`
		CreatedAt      time.Time `json:"CreatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAcademiaCriada: parse error: %w", err)
	}

	cursosJSON, _ := json.Marshal(payload.Cursos)
	anosJSON, _ := json.Marshal(payload.AnosAcademicos)

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_academias (
			id, type, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, cursos, status, email_verificado,
			total_estudantes,
			created_at, updated_at, version, last_event_id
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, 'inativo', FALSE,
			0,
			$14, CURRENT_TIMESTAMP, $15, $16
		)
		ON CONFLICT (id) DO NOTHING
	`,
		event.AggregateID, payload.Type, payload.Nome, payload.CodigoAcademia, payload.SenhaHash,
		payload.Provincia, payload.Endereco, payload.NumeroTelefone, payload.Email, payload.Website,
		payload.NivelEscolar, anosJSON, cursosJSON,
		payload.CreatedAt, event.EventVersion, event.EventID,
	)
	return err
}

func (p *AcademiaProjection) handleStatusChange(novoStatus string) func(db.Event) error {
	return func(event db.Event) error {
		_, err := p.client.DB().Exec(`
			UPDATE projection_academias
			SET status = $1,
			    updated_at = CURRENT_TIMESTAMP,
			    version = $2,
			    last_event_id = $3
			WHERE id = $4
		`, novoStatus, event.EventVersion, event.EventID, event.AggregateID)
		return err
	}
}

func (p *AcademiaProjection) handleCursosAtualizados(event db.Event) error {
	var payload struct {
		NovoCursos []string  `json:"NovoCursos"`
		UpdatedAt  time.Time `json:"UpdatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleCursosAtualizados: parse error: %w", err)
	}

	cursosJSON, _ := json.Marshal(payload.NovoCursos)

	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET cursos = $1,
		    updated_at = CURRENT_TIMESTAMP,
		    version = $2,
		    last_event_id = $3
		WHERE id = $4
	`, cursosJSON, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleEstudanteCriadoComVinculo incrementa total_estudantes quando
// uma academia cria um estudante diretamente (vínculo direto na criação).
func (p *AcademiaProjection) handleEstudanteCriadoComVinculo(event db.Event) error {
	var payload struct {
		CodigoAcademia string `json:"CodigoAcademia"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteCriadoComVinculo: parse error: %w", err)
	}
	if payload.CodigoAcademia == "" {
		return fmt.Errorf("handleEstudanteCriadoComVinculo: CodigoAcademia ausente no payload")
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET total_estudantes = total_estudantes + 1,
		    updated_at       = CURRENT_TIMESTAMP
		WHERE codigo_academia = $1
	`, payload.CodigoAcademia)
	return err
}

// handleAcademiaDadosAtualizados atualiza campos opcionais da academia.
func (p *AcademiaProjection) handleAcademiaDadosAtualizados(event db.Event) error {
	var payload struct {
		Nome           *string  `json:"Nome"`
		Provincia      *string  `json:"Provincia"`
		Endereco       *string  `json:"Endereco"`
		NumeroTelefone *string  `json:"NumeroTelefone"`
		Email          *string  `json:"Email"`
		Website        *string  `json:"Website"`
		NivelEscolar   *string  `json:"NivelEscolar"`
		AnosAcademicos []string `json:"AnosAcademicos"`
		Cursos         []string `json:"Cursos"`
		EmailAlterado  bool     `json:"EmailAlterado"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAcademiaDadosAtualizados: parse error: %w", err)
	}

	args := []interface{}{}
	setClauses := []string{}
	paramIdx := 1

	addParam := func(clause string, val interface{}) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", clause, paramIdx))
		args = append(args, val)
		paramIdx++
	}

	if payload.Nome != nil {
		addParam("nome", *payload.Nome)
	}
	if payload.Provincia != nil {
		addParam("provincia", *payload.Provincia)
	}
	if payload.Endereco != nil {
		addParam("endereco", *payload.Endereco)
	}
	if payload.NumeroTelefone != nil {
		addParam("numero_telefone", *payload.NumeroTelefone)
	}
	if payload.Email != nil {
		addParam("email", *payload.Email)
		if payload.EmailAlterado {
			addParam("email_verificado", false)
		}
	}
	if payload.Website != nil {
		addParam("website", *payload.Website)
	}
	if payload.NivelEscolar != nil {
		addParam("nivel_escolar", *payload.NivelEscolar)
	}
	if payload.AnosAcademicos != nil {
		anosJSON, _ := json.Marshal(payload.AnosAcademicos)
		addParam("anos_academicos", anosJSON)
	}
	if payload.Cursos != nil {
		cursosJSON, _ := json.Marshal(payload.Cursos)
		addParam("cursos", cursosJSON)
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, fmt.Sprintf("updated_at = CURRENT_TIMESTAMP"))
	setClauses = append(setClauses, fmt.Sprintf("version = $%d", paramIdx))
	args = append(args, event.EventVersion)
	paramIdx++
	setClauses = append(setClauses, fmt.Sprintf("last_event_id = $%d", paramIdx))
	args = append(args, event.EventID)
	paramIdx++

	args = append(args, event.AggregateID)
	query := fmt.Sprintf(
		"UPDATE projection_academias SET %s WHERE id = $%d",
		joinStrings(setClauses, ", "),
		paramIdx,
	)

	_, err := p.client.DB().Exec(query, args...)
	return err
}

func (p *AcademiaProjection) handleEmailVerificado(event db.Event) error {
	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET email_verificado = TRUE,
		    updated_at = CURRENT_TIMESTAMP,
		    version = $1,
		    last_event_id = $2
		WHERE id = $3
	`, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

// AcademiaDTO representa a visão de leitura de uma academia.
type AcademiaDTO struct {
	ID              uuid.UUID `json:"id"`
	Type            string    `json:"type"`
	Nome            string    `json:"nome"`
	CodigoAcademia  string    `json:"codigo_academia"`
	SenhaHash       string    `json:"senha_hash,omitempty"`
	Provincia       string    `json:"provincia"`
	Endereco        string    `json:"endereco"`
	NumeroTelefone  *string   `json:"numero_telefone,omitempty"`
	Email           *string   `json:"email,omitempty"`
	Website         *string   `json:"website,omitempty"`
	NivelEscolar    *string   `json:"nivel_escolar,omitempty"`
	AnosAcademicos  []string  `json:"anos_academicos,omitempty"`
	Status          string    `json:"status"`
	Cursos          []string  `json:"cursos"`
	EmailVerificado bool      `json:"email_verificado"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	TotalEstudantes int       `json:"total_estudantes"`
	Version         int       `json:"version"`
}

func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, type, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, status, cursos, email_verificado,
			created_at, updated_at, total_estudantes, version
		FROM projection_academias
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	return scanAcademia(row)
}

func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, type, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, status, cursos, email_verificado,
			created_at, updated_at, total_estudantes, version
		FROM projection_academias
		WHERE codigo_academia = $1 AND deleted_at IS NULL
	`, codigo)
	return scanAcademia(row)
}

func (p *AcademiaProjection) GetByEmail(email string) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, type, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, status, cursos, email_verificado,
			created_at, updated_at, total_estudantes, version
		FROM projection_academias
		WHERE email = $1 AND deleted_at IS NULL
	`, email)
	return scanAcademia(row)
}

// GetByCodigoOrEmail tenta encontrar a academia por código primeiro,
// depois por email — usado no login onde o usuário pode informar qualquer um.
func (p *AcademiaProjection) GetByCodigoOrEmail(codigoOrEmail string) (*AcademiaDTO, error) {
	academia, err := p.GetByCodigo(codigoOrEmail)
	if err != nil {
		return nil, err
	}
	if academia != nil {
		return academia, nil
	}
	return p.GetByEmail(codigoOrEmail)
}

func scanAcademia(row interface{ Scan(...interface{}) error }) (*AcademiaDTO, error) {
	var a AcademiaDTO
	var cursosJSON, anosJSON []byte
	err := row.Scan(
		&a.ID, &a.Type, &a.Nome, &a.CodigoAcademia, &a.SenhaHash,
		&a.Provincia, &a.Endereco, &a.NumeroTelefone, &a.Email, &a.Website,
		&a.NivelEscolar, &anosJSON, &a.Status, &cursosJSON, &a.EmailVerificado,
		&a.CreatedAt, &a.UpdatedAt, &a.TotalEstudantes, &a.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if cursosJSON != nil {
		json.Unmarshal(cursosJSON, &a.Cursos)
	}
	if anosJSON != nil {
		json.Unmarshal(anosJSON, &a.AnosAcademicos)
	}
	return &a, nil
}

// ============================================================================
// Helpers
// ============================================================================

func (p *AcademiaProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_academias CASCADE`)
	return err
}

func joinStrings(ss []string, sep string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}