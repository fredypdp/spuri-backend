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

type TelefonesExtraProjection struct {
	client *db.Client
}

func NewTelefonesExtraProjection(client *db.Client) *TelefonesExtraProjection {
	return &TelefonesExtraProjection{client: client}
}

func (p *TelefonesExtraProjection) Name() string { return "telefones_extra" }

// ============================================================================
// Interface Projection
// ============================================================================

func (p *TelefonesExtraProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *TelefonesExtraProjection) UpdateCheckpoint(eventID int64) error {
	eventID = int64(db.ValidateOffset(int(eventID)))
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, events_processed)
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

func (p *TelefonesExtraProjection) Handle(event db.Event) error {
	if event.AggregateType != "TelefoneExtra" {
		return nil
	}
	switch event.EventType {
	case "TelefoneExtraAdicionado":
		return p.handleTelefoneExtraAdicionado(event)
	case "TelefoneExtraVerificado":
		return p.handleTelefoneExtraVerificado(event)
	}
	return nil
}

// ============================================================================
// Rebuild
// ============================================================================

func (p *TelefonesExtraProjection) Rebuild() error {
	log.Printf("[telefones_extra] Rebuild iniciado")
	if _, err := p.client.DB().Exec(`DELETE FROM projection_telefones_extra`); err != nil {
		return fmt.Errorf("falha ao limpar projection_telefones_extra: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'TelefoneExtra'
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

	log.Printf("[telefones_extra] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

// ============================================================================
// Handlers de evento
// ============================================================================

func (p *TelefonesExtraProjection) handleTelefoneExtraAdicionado(event db.Event) error {
	var payload struct {
		IDUser         uuid.UUID `json:"IDUser"`
		TipoUser       string    `json:"TipoUser"`
		NumeroTelefone string    `json:"NumeroTelefone"`
		RegisteredAt   time.Time `json:"RegisteredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleTelefoneExtraAdicionado: unmarshal: %w", err)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_telefones_extra
			(id, id_user, tipo_user, numero_telefone, verificado,
			 registered_at, updated_at, event_id, version)
		VALUES ($1, $2, $3, $4, FALSE, $5, $5, $6, $7)
		ON CONFLICT (id_user, tipo_user, numero_telefone) DO NOTHING
	`,
		event.AggregateID,
		payload.IDUser, payload.TipoUser, payload.NumeroTelefone,
		payload.RegisteredAt, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleTelefoneExtraAdicionado: exec: %w", err)
	}
	return nil
}

func (p *TelefonesExtraProjection) handleTelefoneExtraVerificado(event db.Event) error {
	var payload struct {
		IDUser         uuid.UUID `json:"IDUser"`
		TipoUser       string    `json:"TipoUser"`
		NumeroTelefone string    `json:"NumeroTelefone"`
		VerificadoEm   time.Time `json:"VerificadoEm"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleTelefoneExtraVerificado: unmarshal: %w", err)
	}

	result, err := p.client.DB().Exec(`
		UPDATE projection_telefones_extra
		SET verificado  = TRUE,
		    updated_at  = $1,
		    event_id    = $2,
		    version     = $3
		WHERE id = $4
		  AND verificado = FALSE
	`, payload.VerificadoEm, event.EventID, event.EventVersion, event.AggregateID)
	if err != nil {
		return fmt.Errorf("handleTelefoneExtraVerificado: exec: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		log.Printf("[WARN] [telefones_extra] TelefoneExtraVerificado %s: registro id=%s não encontrado ou já verificado — ignorado",
			event.EventID, event.AggregateID)
	}
	return nil
}

// ============================================================================
// DTO e queries de leitura
// ============================================================================

type TelefoneExtraDTO struct {
	ID             uuid.UUID `json:"id"`
	IDUser         uuid.UUID `json:"id_user"`
	TipoUser       string    `json:"tipo_user"`
	NumeroTelefone string    `json:"numero_telefone"`
	Verificado     bool      `json:"verificado"`
	RegisteredAt   time.Time `json:"registered_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

const telefoneCols = `
	id, id_user, tipo_user, numero_telefone, verificado, registered_at, updated_at
`

// GetByUser retorna todos os telefones extras de um usuário.
func (p *TelefonesExtraProjection) GetByUser(idUser uuid.UUID, tipoUser string) ([]TelefoneExtraDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+telefoneCols+`
		FROM projection_telefones_extra
		WHERE id_user = $1 AND tipo_user = $2
		ORDER BY registered_at DESC`,
		idUser, tipoUser,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTelefones(rows)
}

// NumeroJaVerificado retorna true se o número já está verificado por qualquer usuário.
func (p *TelefonesExtraProjection) NumeroJaVerificado(numero string) (bool, error) {
	var exists bool
	err := p.client.DB().QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM projection_telefones_extra
			WHERE numero_telefone = $1 AND verificado = TRUE
		)
	`, numero).Scan(&exists)
	return exists, err
}

// UsuarioJaCadastrou retorna true se o usuário já cadastrou este número.
func (p *TelefonesExtraProjection) UsuarioJaCadastrou(idUser uuid.UUID, tipoUser, numero string) (bool, error) {
	var exists bool
	err := p.client.DB().QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM projection_telefones_extra
			WHERE id_user = $1 AND tipo_user = $2 AND numero_telefone = $3
		)
	`, idUser, tipoUser, numero).Scan(&exists)
	return exists, err
}

// GetByID retorna um telefone extra pelo UUID do aggregate.
func (p *TelefonesExtraProjection) GetByID(id uuid.UUID) (*TelefoneExtraDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+telefoneCols+`
		FROM projection_telefones_extra
		WHERE id = $1`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dtos, err := scanTelefones(rows)
	if err != nil || len(dtos) == 0 {
		return nil, err
	}
	return &dtos[0], nil
}

func scanTelefones(rows *sql.Rows) ([]TelefoneExtraDTO, error) {
	var result []TelefoneExtraDTO
	for rows.Next() {
		var dto TelefoneExtraDTO
		if err := rows.Scan(
			&dto.ID, &dto.IDUser, &dto.TipoUser, &dto.NumeroTelefone,
			&dto.Verificado, &dto.RegisteredAt, &dto.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}
