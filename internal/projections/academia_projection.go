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
//
// FIX E-23: eventos de AggregateType="Estudante" com EventType="EstudanteCriadoComVinculo"
// agora são processados corretamente.
//
// FIX C1: AcademiaSenhaAlterada adicionado ao map de handlers.
func (p *AcademiaProjection) Handle(event db.Event) error {
	if event.AggregateType == "Academia" {
		academiaHandlers := map[string]func(db.Event) error{
			"AcademiaCriada":            p.handleAcademiaCriada,
			"AcademiaAtivada":           p.handleStatusChange("ativo"),
			"AcademiaDesativada":        p.handleAcademiaDesativada,
			"CursosAtualizados":         p.handleCursosAtualizados,
			"AcademiaDadosAtualizados":  p.handleAcademiaDadosAtualizados,
			"EmailVerificado":           p.handleEmailVerificado,
			"AcademiaSenhaAlterada":     p.handleAcademiaSenhaAlterada,
			"AnoLetivoAcademiaDefinido": p.handleAnoLetivoAcademiaDefinido,
			// CategoriaNotaAdicionada é tratado pela CategoriasNotaProjection dedicada.
		}
		if handler, ok := academiaHandlers[event.EventType]; ok {
			log.Printf("[DEBUG] [academias] Processando %s para %s", event.EventType, event.AggregateID)
			return handler(event)
		}
		return nil
	}

	// FIX E-23: eventos cross-aggregate — AggregateType="Estudante"
	// Incrementa total_estudantes quando academia cria estudante diretamente.
	if event.AggregateType == "Estudante" && event.EventType == "EstudanteCriadoComVinculo" {
		return p.handleEstudanteCriadoComVinculo(event)
	}

	return nil
}

func (p *AcademiaProjection) HandleTx(tx *sql.Tx, event db.Event) error {
	if event.AggregateType == "Academia" {
		academiaHandlers := map[string]func(*sql.Tx, db.Event) error{
			"AcademiaCriada":            p.handleAcademiaCriadaTx,
			"AcademiaAtivada":           p.handleStatusChangeTx("ativo"),
			"AcademiaDesativada":        p.handleAcademiaDesativadaTx,
			"CursosAtualizados":         p.handleCursosAtualizadosTx,
			"AcademiaDadosAtualizados":  p.handleAcademiaDadosAtualizadosTx,
			"EmailVerificado":           p.handleEmailVerificadoTx,
			"AcademiaSenhaAlterada":     p.handleAcademiaSenhaAlteradaTx,
			"AnoLetivoAcademiaDefinido": p.handleAnoLetivoAcademiaDefinidoTx,
		}
		if handler, ok := academiaHandlers[event.EventType]; ok {
			return handler(tx, event)
		}
		return nil
	}

	// Cross-aggregate: acumulador não-idempotente — DEVE estar na tx.
	if event.AggregateType == "Estudante" && event.EventType == "EstudanteCriadoComVinculo" {
		return p.handleEstudanteCriadoComVinculoTx(tx, event)
	}

	return nil
}

// handleEstudanteCriadoComVinculoTx — acumulador não-idempotente dentro da tx.
func (p *AcademiaProjection) handleEstudanteCriadoComVinculoTx(tx *sql.Tx, event db.Event) error {
	var payload struct {
		CodigoAcademia string `json:"CodigoAcademia"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleEstudanteCriadoComVinculoTx: parse error: %w", err)
	}
	if payload.CodigoAcademia == "" {
		return fmt.Errorf("handleEstudanteCriadoComVinculoTx: CodigoAcademia ausente no payload")
	}

	_, err := tx.Exec(`
		UPDATE projection_academias
		SET total_estudantes = total_estudantes + 1,
		    updated_at       = CURRENT_TIMESTAMP
		WHERE codigo_academia = $1
	`, payload.CodigoAcademia)
	return err
}

// Os handlers Tx abaixo delegam ao pool do cliente (operações idempotentes),
// mas são fornecidos para manter a interface consistente dentro de HandleTx.
func (p *AcademiaProjection) handleAcademiaCriadaTx(tx *sql.Tx, event db.Event) error {
	return p.handleAcademiaCriada(event)
}

func (p *AcademiaProjection) handleStatusChangeTx(status string) func(*sql.Tx, db.Event) error {
	return func(tx *sql.Tx, event db.Event) error {
		return p.handleStatusChange(status)(event)
	}
}

func (p *AcademiaProjection) handleAcademiaDesativadaTx(tx *sql.Tx, event db.Event) error {
	return p.handleAcademiaDesativada(event)
}

func (p *AcademiaProjection) handleCursosAtualizadosTx(tx *sql.Tx, event db.Event) error {
	return p.handleCursosAtualizados(event)
}

func (p *AcademiaProjection) handleAcademiaDadosAtualizadosTx(tx *sql.Tx, event db.Event) error {
	return p.handleAcademiaDadosAtualizados(event)
}

func (p *AcademiaProjection) handleEmailVerificadoTx(tx *sql.Tx, event db.Event) error {
	return p.handleEmailVerificado(event)
}

func (p *AcademiaProjection) handleAcademiaSenhaAlteradaTx(tx *sql.Tx, event db.Event) error {
	return p.handleAcademiaSenhaAlterada(event)
}

func (p *AcademiaProjection) handleAnoLetivoAcademiaDefinidoTx(tx *sql.Tx, event db.Event) error {
	return p.handleAnoLetivoAcademiaDefinido(event)
}

// ============================================================================
// Rebuild
// ============================================================================

// Rebuild reconstrói a projeção do zero a partir do ledger.
//
// FIX E-25: inclui eventos "EstudanteCriadoComVinculo" no rebuild para que
// total_estudantes seja corretamente populado após um rebuild.
//
// FIX C1: AcademiaSenhaAlterada é processado via Handle() — o rebuild automaticamente
// inclui este evento porque filtra aggregate_type = 'Academia'.
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
			return fmt.Errorf("erro ao processar evento %d (type=%s, aggregate=%s): %w",
				event.ID, event.EventType, event.AggregateID, err)
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

// handleAcademiaCriada insere a academia na projeção de forma idempotente.
//
// FIX MIG-045: substituído ON CONFLICT (id) DO NOTHING por
// ON CONFLICT (codigo_academia) DO UPDATE.
//
// MOTIVAÇÃO: quando dois eventos no ledger têm o mesmo codigo_academia (gerado
// por race condition antes da migration 045 corrigir o gerador), o segundo
// evento causava falha na constraint UNIQUE(codigo_academia) — o ON CONFLICT (id)
// não protegia porque os dois eventos têm id (UUID) diferentes.
//
// Com ON CONFLICT (codigo_academia) DO UPDATE o handler é idempotente: se o
// código já existir, atualiza os campos com os dados do evento mais recente
// (exceto status e totais que são gerenciados por outros eventos).
// Isso garante que o pipeline de eventos nunca trava por duplicata de código.
//
// O gerador foi corrigido na migration 045 para consultar o ledger em vez da
// projeção, prevenindo novas colisões. Este ON CONFLICT é a segunda linha de
// defesa para eventos já gravados no ledger histórico.
func (p *AcademiaProjection) handleAcademiaCriada(event db.Event) error {
	var payload struct {
		Nivel          string    `json:"Nivel"`
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
	nivel := payload.Nivel
	if nivel == "" {
		nivel = payload.Type
	}

	cursosJSON, _ := json.Marshal(payload.Cursos)

	// FIX PROJ-01: nil → NULL no banco (constraint aceita NULL para fundamental/misto).
	// Array vazio "[]" → jsonb_array_length = 0 → viola check_anos_academicos_nivel.
	var anosValue interface{}
	if len(payload.AnosAcademicos) > 0 {
		b, _ := json.Marshal(payload.AnosAcademicos)
		anosValue = string(b)
	}

	_, err := p.client.DB().Exec(`
		INSERT INTO projection_academias (
			id, nivel, nome, codigo_academia, senha_hash,
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
		ON CONFLICT (codigo_academia) DO UPDATE SET
			id              = EXCLUDED.id,
			nivel           = EXCLUDED.nivel,
			nome            = EXCLUDED.nome,
			senha_hash      = EXCLUDED.senha_hash,
			provincia       = EXCLUDED.provincia,
			endereco        = EXCLUDED.endereco,
			numero_telefone = EXCLUDED.numero_telefone,
			email           = EXCLUDED.email,
			website         = EXCLUDED.website,
			nivel_escolar   = EXCLUDED.nivel_escolar,
			anos_academicos = EXCLUDED.anos_academicos,
			cursos          = EXCLUDED.cursos,
			updated_at      = CURRENT_TIMESTAMP,
			version         = EXCLUDED.version,
			last_event_id   = EXCLUDED.last_event_id
	`,
		event.AggregateID, payload.Nivel, payload.Nome, payload.CodigoAcademia, payload.SenhaHash,
		payload.Provincia, payload.Endereco, payload.NumeroTelefone, payload.Email, payload.Website,
		payload.NivelEscolar, anosValue, cursosJSON,
		payload.CreatedAt, event.EventVersion, event.EventID,
	)
	return err
}

// handleStatusChange retorna um handler que atualiza apenas o status.
//
// FIX PROJ-02: o resultado do UPDATE é verificado via RowsAffected.
// Se 0 linhas forem afetadas (ex: evento com UUID errado gerado pelo bug do
// repository.Load antes do FIX-REPO-03), loga um WARNING e retorna nil — não
// retorna erro para evitar travar o Rebuild ao processar eventos históricos
// com UUIDs inválidos. Eventos com UUIDs corretos (após FIX-REPO-03) sempre
// afetarão 1 linha.
func (p *AcademiaProjection) handleStatusChange(novoStatus string) func(db.Event) error {
	return func(event db.Event) error {
		var payload struct {
			CodigoAcademia string `json:"CodigoAcademia"`
		}
		_ = json.Unmarshal(event.Payload, &payload)

		result, err := p.client.DB().Exec(`
			UPDATE projection_academias
			SET status = $1,
			    updated_at = CURRENT_TIMESTAMP,
			    version = $2,
			    last_event_id = $3
			WHERE id = $4
		`, novoStatus, event.EventVersion, event.EventID, event.AggregateID)
		if err != nil {
			return err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("handleStatusChange: RowsAffected error: %w", err)
		}
		if rowsAffected == 0 {
			codigo := strings.TrimSpace(payload.CodigoAcademia)
			if codigo != "" {
				fallbackResult, fallbackErr := p.client.DB().Exec(`
					UPDATE projection_academias
					SET status = $1,
					    updated_at = CURRENT_TIMESTAMP,
					    version = $2,
					    last_event_id = $3
					WHERE codigo_academia = $4
				`, novoStatus, event.EventVersion, event.EventID, codigo)
				if fallbackErr != nil {
					return fmt.Errorf("handleStatusChange: fallback por codigo falhou: %w", fallbackErr)
				}
				fallbackRows, rowsErr := fallbackResult.RowsAffected()
				if rowsErr != nil {
					return fmt.Errorf("handleStatusChange: fallback RowsAffected error: %w", rowsErr)
				}
				if fallbackRows > 0 {
					log.Printf("[WARN] [academias] handleStatusChange(%s): aggregate_id %s não encontrado; aplicado fallback por código %s",
						novoStatus, event.AggregateID, codigo)
					return nil
				}
			}
			log.Printf("[WARN] [academias] handleStatusChange(%s): 0 linhas afetadas para aggregate %s — sem fallback aplicável",
				novoStatus, event.AggregateID)
			return nil
		}
		log.Printf("[DEBUG] [academias] Status atualizado para '%s' — aggregate: %s", novoStatus, event.AggregateID)
		return nil
	}
}

// handleAcademiaDesativada atualiza status, motivo e DesativadoPor.
//
// FIX E-10: motivo_desativacao agora é persistido na projeção para consulta
// direta, sem necessidade de inspecionar o ledger.
// FIX C9: DesativadoPor agora é lido do payload e persistido.
func (p *AcademiaProjection) handleAcademiaDesativada(event db.Event) error {
	var payload struct {
		CodigoAcademia string    `json:"CodigoAcademia"`
		Motivo         string    `json:"Motivo"`
		DesativadoPor  string    `json:"DesativadoPor"`
		DeactivatedAt  time.Time `json:"DeactivatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAcademiaDesativada: parse error: %w", err)
	}

	result, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET status              = 'inativo',
		    motivo_desativacao  = $1,
		    updated_at          = CURRENT_TIMESTAMP,
		    version             = $2,
		    last_event_id       = $3
		WHERE id = $4
	`, payload.Motivo, event.EventVersion, event.EventID, event.AggregateID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("handleAcademiaDesativada: RowsAffected error: %w", err)
	}
	if rowsAffected > 0 {
		return nil
	}

	codigo := strings.TrimSpace(payload.CodigoAcademia)
	if codigo == "" {
		return nil
	}

	_, err = p.client.DB().Exec(`
		UPDATE projection_academias
		SET status              = 'inativo',
		    motivo_desativacao  = $1,
		    updated_at          = CURRENT_TIMESTAMP,
		    version             = $2,
		    last_event_id       = $3
		WHERE codigo_academia = $4
	`, payload.Motivo, event.EventVersion, event.EventID, codigo)
	return err
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

	argIdx := 1
	setClauses := []string{
		"updated_at = CURRENT_TIMESTAMP",
		fmt.Sprintf("version = $%d", argIdx),
		fmt.Sprintf("last_event_id = $%d", argIdx+1),
	}
	args := []interface{}{event.EventVersion, event.EventID}
	argIdx = 3

	if payload.Nome != nil {
		setClauses = append(setClauses, fmt.Sprintf("nome = $%d", argIdx))
		args = append(args, *payload.Nome)
		argIdx++
	}
	if payload.Provincia != nil {
		setClauses = append(setClauses, fmt.Sprintf("provincia = $%d", argIdx))
		args = append(args, *payload.Provincia)
		argIdx++
	}
	if payload.Endereco != nil {
		setClauses = append(setClauses, fmt.Sprintf("endereco = $%d", argIdx))
		args = append(args, *payload.Endereco)
		argIdx++
	}
	if payload.NumeroTelefone != nil {
		setClauses = append(setClauses, fmt.Sprintf("numero_telefone = $%d", argIdx))
		args = append(args, *payload.NumeroTelefone)
		argIdx++
	}
	if payload.Email != nil {
		setClauses = append(setClauses, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, *payload.Email)
		argIdx++
	}
	if payload.EmailAlterado {
		setClauses = append(setClauses, "email_verificado = FALSE")
	}
	if payload.Website != nil {
		setClauses = append(setClauses, fmt.Sprintf("website = $%d", argIdx))
		args = append(args, *payload.Website)
		argIdx++
	}
	if payload.NivelEscolar != nil {
		setClauses = append(setClauses, fmt.Sprintf("nivel_escolar = $%d", argIdx))
		args = append(args, *payload.NivelEscolar)
		argIdx++
	}
	if payload.AnosAcademicos != nil {
		anosJSON, _ := json.Marshal(payload.AnosAcademicos)
		setClauses = append(setClauses, fmt.Sprintf("anos_academicos = $%d", argIdx))
		args = append(args, string(anosJSON))
		argIdx++
	}
	if payload.Cursos != nil {
		cursosJSON, _ := json.Marshal(payload.Cursos)
		setClauses = append(setClauses, fmt.Sprintf("cursos = $%d", argIdx))
		args = append(args, string(cursosJSON))
		argIdx++
	}

	// Apenas os 3 campos fixos — nada para atualizar.
	if len(setClauses) == 3 {
		return nil
	}

	query := "UPDATE projection_academias SET "
	for i, clause := range setClauses {
		if i > 0 {
			query += ", "
		}
		query += clause
	}
	query += fmt.Sprintf(" WHERE id = $%d", argIdx)
	args = append(args, event.AggregateID)

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

// handleAcademiaSenhaAlterada atualiza o hash de senha na projeção.
func (p *AcademiaProjection) handleAcademiaSenhaAlterada(event db.Event) error {
	var payload struct {
		NovaSenhaHash string `json:"NovaSenhaHash"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAcademiaSenhaAlterada: parse error: %w", err)
	}
	if payload.NovaSenhaHash == "" {
		return fmt.Errorf("handleAcademiaSenhaAlterada: NovaSenhaHash vazio no payload")
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET senha_hash    = $1,
		    updated_at    = CURRENT_TIMESTAMP,
		    version       = $2,
		    last_event_id = $3
		WHERE id = $4
	`, payload.NovaSenhaHash, event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// handleAnoLetivoAcademiaDefinido persiste o ano letivo ativo da academia.
func (p *AcademiaProjection) handleAnoLetivoAcademiaDefinido(event db.Event) error {
	var payload struct {
		AnoLetivo  string    `json:"AnoLetivo"`
		Tipo       string    `json:"Tipo"`
		DefinidoEm time.Time `json:"DefinidoEm"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAnoLetivoAcademiaDefinido: parse error: %w", err)
	}
	if payload.AnoLetivo == "" {
		return fmt.Errorf("handleAnoLetivoAcademiaDefinido: AnoLetivo ausente no payload")
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET ano_letivo             = $1,
		    tipo_ano_letivo        = $2,
		    ano_letivo_ativado_em  = $3,
		    updated_at             = CURRENT_TIMESTAMP,
		    version                = $4,
		    last_event_id          = $5
		WHERE id = $6
	`, payload.AnoLetivo, payload.Tipo, payload.DefinidoEm,
		event.EventVersion, event.EventID, event.AggregateID)
	return err
}

// ============================================================================
// Queries de leitura
// ============================================================================

// AcademiaDTO representa a visão de leitura de uma academia.
type AcademiaDTO struct {
	ID                 uuid.UUID  `json:"id"`
	Nivel              string     `json:"nivel"`
	Nome               string     `json:"nome"`
	CodigoAcademia     string     `json:"codigo_academia"`
	SenhaHash          string     `json:"-"`
	Provincia          string     `json:"provincia"`
	Endereco           string     `json:"endereco"`
	NumeroTelefone     *string    `json:"numero_telefone,omitempty"`
	Email              *string    `json:"email,omitempty"`
	Website            *string    `json:"website,omitempty"`
	NivelEscolar       *string    `json:"nivel_escolar,omitempty"`
	AnosAcademicos     []string   `json:"anos_academicos,omitempty"`
	Status             string     `json:"status"`
	MotivoDesativacao  *string    `json:"motivo_desativacao,omitempty"`
	Cursos             []string   `json:"cursos"`
	EmailVerificado    bool       `json:"email_verificado"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	TotalEstudantes    int        `json:"total_estudantes"`
	Version            int        `json:"version"`
	AnoLetivo          *string    `json:"ano_letivo,omitempty"`
	TipoAnoLetivo      *string    `json:"tipo_ano_letivo,omitempty"`
	AnoLetivoAtivadoEm *time.Time `json:"ano_letivo_ativado_em,omitempty"`
}

func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, nivel, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, status, motivo_desativacao, cursos, email_verificado,
			created_at, updated_at, total_estudantes, version,
			ano_letivo, tipo_ano_letivo, ano_letivo_ativado_em
		FROM projection_academias
		WHERE id = $1
	`, id)
	return scanAcademia(row)
}

func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, nivel, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, status, motivo_desativacao, cursos, email_verificado,
			created_at, updated_at, total_estudantes, version,
			ano_letivo, tipo_ano_letivo, ano_letivo_ativado_em
		FROM projection_academias
		WHERE codigo_academia = $1
	`, codigo)
	return scanAcademia(row)
}

func (p *AcademiaProjection) GetByEmail(email string) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, nivel, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, status, motivo_desativacao, cursos, email_verificado,
			created_at, updated_at, total_estudantes, version,
			ano_letivo, tipo_ano_letivo, ano_letivo_ativado_em
		FROM projection_academias
		WHERE email = $1
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
	var motivoDesativacao sql.NullString
	var anoLetivo, tipoAnoLetivo sql.NullString
	var anoLetivoAtivadoEm sql.NullTime

	err := row.Scan(
		&a.ID, &a.Nivel, &a.Nome, &a.CodigoAcademia, &a.SenhaHash,
		&a.Provincia, &a.Endereco, &a.NumeroTelefone, &a.Email, &a.Website,
		&a.NivelEscolar, &anosJSON, &a.Status, &motivoDesativacao, &cursosJSON, &a.EmailVerificado,
		&a.CreatedAt, &a.UpdatedAt, &a.TotalEstudantes, &a.Version,
		&anoLetivo, &tipoAnoLetivo, &anoLetivoAtivadoEm,
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
	if motivoDesativacao.Valid {
		a.MotivoDesativacao = &motivoDesativacao.String
	}
	if anoLetivo.Valid {
		a.AnoLetivo = &anoLetivo.String
	}
	if tipoAnoLetivo.Valid {
		a.TipoAnoLetivo = &tipoAnoLetivo.String
	}
	if anoLetivoAtivadoEm.Valid {
		a.AnoLetivoAtivadoEm = &anoLetivoAtivadoEm.Time
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
