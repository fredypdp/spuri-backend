// ============================================================================
// ARQUIVO: internal/projections/inscricoes_projection.go
// CORREÇÕES:
//  1. handleInscricaoAprovada: usa InscricaoID como chave (não mais combinação de campos)
//  2. handleInscricaoReprovada: retorna erro do Exec em vez de ignorar
//  3. Contadores: erros logados (não fatais, mas visíveis)
//  4. handleEstudanteVinculado: retorna erro do Exec
// ============================================================================

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

type InscricoesProjection struct {
	client *db.Client
}

func NewInscricoesProjection(client *db.Client) *InscricoesProjection {
	return &InscricoesProjection{client: client}
}

func (p *InscricoesProjection) Name() string { return "inscricoes" }

func (p *InscricoesProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"EstudanteInscrito":  p.handleEstudanteInscrito,
		"InscricaoAprovada":  p.handleInscricaoAprovada,
		"InscricaoReprovada": p.handleInscricaoReprovada,
		"EstudanteVinculado": p.handleEstudanteVinculado,
	}

	if handler, ok := handlers[event.EventType]; ok {
		log.Printf("[DEBUG] [inscricoes] Processando %s: %s", event.EventType, event.EventID)
		return handler(event)
	}
	return nil
}

func (p *InscricoesProjection) Rebuild() error {
	log.Printf("[DEBUG] [inscricoes] Rebuild iniciado")

	if err := p.clear(); err != nil {
		return fmt.Errorf("falha ao limpar: %w", err)
	}

	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE event_type IN ('EstudanteInscrito', 'InscricaoAprovada', 'InscricaoReprovada', 'EstudanteVinculado')
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

	log.Printf("[DEBUG] [inscricoes] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *InscricoesProjection) GetLastProcessedEventID() (int64, error) {
	var lastID int64
	query := fmt.Sprintf(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = '%s'`,
		db.SafeString(p.Name()))
	err := p.client.DB().QueryRow(query).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return lastID, err
}

func (p *InscricoesProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *InscricoesProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_inscricoes CASCADE`)
	return err
}

// ============================================================================
// Handlers de eventos
// ============================================================================

func (p *InscricoesProjection) handleEstudanteInscrito(event db.Event) error {
	var payload struct {
		InscricaoID    uuid.UUID
		CodigoAcademia string
		Tipo           string
		AnoInscricao   string
		CursoID        *uuid.UUID
		CreatedAt      time.Time
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteInscrito: parse error: %w", err)
	}

	academiaID, err := p.getAcademiaID(payload.CodigoAcademia)
	if err != nil {
		return fmt.Errorf("handleEstudanteInscrito: academia '%s' não encontrada: %w", payload.CodigoAcademia, err)
	}

	codigoEstudante, err := p.getCodigoEstudante(event.AggregateID)
	if err != nil {
		return fmt.Errorf("handleEstudanteInscrito: estudante '%s' não encontrado: %w", event.AggregateID, err)
	}

	query := fmt.Sprintf(`
		INSERT INTO projection_inscricoes
			(id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			 tipo, ano_inscricao, curso_id, status, status_usado,
			 created_at, updated_at, event_id, version)
		VALUES
			('%s', '%s', '%s', '%s', '%s', '%s', '%s', %s, 'espera', false,
			 '%s', '%s', '%s', %d)
		ON CONFLICT (id) DO NOTHING`,
		payload.InscricaoID,
		event.AggregateID,
		db.SafeString(codigoEstudante),
		academiaID,
		db.SafeString(payload.CodigoAcademia),
		db.SafeString(payload.Tipo),
		db.SafeString(payload.AnoInscricao),
		nullOrUUID(payload.CursoID),
		payload.CreatedAt.Format(time.RFC3339),
		payload.CreatedAt.Format(time.RFC3339),
		event.EventID,
		event.EventVersion,
	)

	if _, err := p.client.DB().Exec(query); err != nil {
		return fmt.Errorf("handleEstudanteInscrito: insert falhou: %w", err)
	}

	// Contadores — não fatais, mas logados se falharem
	if _, err := p.client.DB().Exec(fmt.Sprintf(
		`UPDATE projection_academias SET total_inscricoes_pendentes = total_inscricoes_pendentes + 1 WHERE id = '%s'`,
		academiaID,
	)); err != nil {
		log.Printf("[WARN] [inscricoes] falha ao incrementar total_inscricoes_pendentes para academia %s: %v", academiaID, err)
	}

	if _, err := p.client.DB().Exec(fmt.Sprintf(
		`UPDATE projection_estudantes SET total_inscricoes = total_inscricoes + 1 WHERE id = '%s'`,
		event.AggregateID,
	)); err != nil {
		log.Printf("[WARN] [inscricoes] falha ao incrementar total_inscricoes para estudante %s: %v", event.AggregateID, err)
	}

	return nil
}

func (p *InscricoesProjection) handleInscricaoAprovada(event db.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID
		InscricaoID    uuid.UUID // chave primária da inscrição — usar para o UPDATE
		CodigoAcademia string
		Tipo           string
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleInscricaoAprovada: parse error: %w", err)
	}

	// CORREÇÃO: usar InscricaoID como chave (é UUID único da inscrição).
	// A versão anterior usava (estudante_id, academia_id, status, tipo) — ambíguo
	// quando o estudante tem mais de uma inscrição pendente do mesmo tipo.
	if payload.InscricaoID != uuid.Nil {
		query := fmt.Sprintf(`
			UPDATE projection_inscricoes
			SET status = 'aprovado', updated_at = CURRENT_TIMESTAMP
			WHERE id = '%s' AND status = 'espera'`,
			payload.InscricaoID,
		)
		if _, err := p.client.DB().Exec(query); err != nil {
			return fmt.Errorf("handleInscricaoAprovada: update por InscricaoID falhou: %w", err)
		}
		return nil
	}

	// Fallback para eventos antigos sem InscricaoID no payload
	estudanteID := payload.EstudanteID
	if estudanteID == uuid.Nil {
		estudanteID = event.AggregateID
	}

	academiaID, err := p.getAcademiaID(payload.CodigoAcademia)
	if err != nil {
		log.Printf("[WARN] [inscricoes] handleInscricaoAprovada: academia '%s' não encontrada (evento antigo): %v", payload.CodigoAcademia, err)
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET status = 'aprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' AND academia_id = '%s' AND status = 'espera' AND tipo = '%s'`,
		estudanteID, academiaID, db.SafeString(payload.Tipo),
	)
	if _, err := p.client.DB().Exec(query); err != nil {
		return fmt.Errorf("handleInscricaoAprovada: fallback update falhou: %w", err)
	}

	return nil
}

func (p *InscricoesProjection) handleInscricaoReprovada(event db.Event) error {
	var payload struct {
		EstudanteID    uuid.UUID
		InscricaoID    uuid.UUID
		CodigoAcademia string
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleInscricaoReprovada: parse error: %w", err)
	}

	// CORREÇÃO: usar InscricaoID quando disponível
	if payload.InscricaoID != uuid.Nil {
		query := fmt.Sprintf(`
			UPDATE projection_inscricoes
			SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP
			WHERE id = '%s' AND status = 'espera'`,
			payload.InscricaoID,
		)
		if _, err := p.client.DB().Exec(query); err != nil {
			return fmt.Errorf("handleInscricaoReprovada: update por InscricaoID falhou: %w", err)
		}
		return nil
	}

	// Fallback para eventos antigos sem InscricaoID
	academiaID, err := p.getAcademiaID(payload.CodigoAcademia)
	if err != nil {
		log.Printf("[WARN] [inscricoes] handleInscricaoReprovada: academia '%s' não encontrada: %v", payload.CodigoAcademia, err)
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET status = 'reprovado', updated_at = CURRENT_TIMESTAMP
		WHERE estudante_id = '%s' AND academia_id = '%s' AND status = 'espera'`,
		payload.EstudanteID, academiaID,
	)
	if _, err := p.client.DB().Exec(query); err != nil {
		return fmt.Errorf("handleInscricaoReprovada: fallback update falhou: %w", err)
	}

	return nil
}

func (p *InscricoesProjection) handleEstudanteVinculado(event db.Event) error {
	var payload struct {
		InscricaoID    uuid.UUID
		CodigoAcademia string
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteVinculado: parse error: %w", err)
	}

	if payload.InscricaoID == uuid.Nil {
		log.Printf("[WARN] [inscricoes] handleEstudanteVinculado: InscricaoID vazio para evento %s", event.EventID)
		return nil
	}

	query := fmt.Sprintf(`
		UPDATE projection_inscricoes
		SET status_usado = true, updated_at = CURRENT_TIMESTAMP
		WHERE id = '%s'`,
		payload.InscricaoID,
	)
	if _, err := p.client.DB().Exec(query); err != nil {
		return fmt.Errorf("handleEstudanteVinculado: update falhou: %w", err)
	}

	return nil
}

// ============================================================================
// Helpers
// ============================================================================

func (p *InscricoesProjection) getAcademiaID(codigoAcademia string) (uuid.UUID, error) {
	var id uuid.UUID
	query := fmt.Sprintf(`SELECT id FROM projection_academias WHERE codigo_academia = '%s' LIMIT 1`,
		db.SafeString(codigoAcademia))
	err := p.client.DB().QueryRow(query).Scan(&id)
	if err == sql.ErrNoRows {
		return uuid.Nil, fmt.Errorf("academia não encontrada: %s", codigoAcademia)
	}
	return id, err
}

func (p *InscricoesProjection) getCodigoEstudante(estudanteID uuid.UUID) (string, error) {
	var codigo string
	query := fmt.Sprintf(`SELECT codigo_estudante FROM projection_estudantes WHERE id = '%s' LIMIT 1`, estudanteID)
	err := p.client.DB().QueryRow(query).Scan(&codigo)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("estudante não encontrado: %s", estudanteID)
	}
	return codigo, err
}

// GetByEstudante retorna inscrições de um estudante pelo ID.
func (p *InscricoesProjection) GetByEstudante(estudanteID uuid.UUID) ([]map[string]interface{}, error) {
	query := fmt.Sprintf(`
		SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso_id, status, status_usado, created_at, updated_at
		FROM projection_inscricoes
		WHERE estudante_id = '%s'
		ORDER BY created_at DESC`, estudanteID)

	rows, err := p.client.DB().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var (
			id, estudanteIDScan, academiaID          uuid.UUID
			codigoEstudante, codigoAcademia          string
			tipo, anoInscricao, status               string
			statusUsado                              bool
			cursoID                                  sql.NullString
			createdAt, updatedAt                     time.Time
		)
		if err := rows.Scan(
			&id, &estudanteIDScan, &codigoEstudante, &academiaID, &codigoAcademia,
			&tipo, &anoInscricao, &cursoID, &status, &statusUsado, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		row := map[string]interface{}{
			"id":               id,
			"estudante_id":     estudanteIDScan,
			"codigo_estudante": codigoEstudante,
			"academia_id":      academiaID,
			"codigo_academia":  codigoAcademia,
			"tipo":             tipo,
			"ano_inscricao":    anoInscricao,
			"status":           status,
			"status_usado":     statusUsado,
			"created_at":       createdAt,
			"updated_at":       updatedAt,
		}
		if cursoID.Valid {
			row["curso_id"] = cursoID.String
		}
		results = append(results, row)
	}
	return results, rows.Err()
}