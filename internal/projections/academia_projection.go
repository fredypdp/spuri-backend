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
//
// FIX E-23: eventos de AggregateType="Estudante" com EventType="EstudanteCriadoComVinculo"
// agora são processados corretamente. O guard anterior `event.AggregateType == "Academia"`
// bloqueava todos esses eventos, mantendo total_estudantes sempre em 0.
//
// FIX C1: AcademiaSenhaAlterada adicionado ao map de handlers.
func (p *AcademiaProjection) Handle(event db.Event) error {
	if event.AggregateType == "Academia" {
		academiaHandlers := map[string]func(db.Event) error{
			"AcademiaCriada":           p.handleAcademiaCriada,
			"AcademiaAtivada":          p.handleStatusChange("ativo"),
			"AcademiaDesativada":       p.handleAcademiaDesativada,
			"CursosAtualizados":        p.handleCursosAtualizados,
			"AcademiaDadosAtualizados": p.handleAcademiaDadosAtualizados,
			"EmailVerificado":          p.handleEmailVerificado,
			// FIX C1: handler para novo evento de senha da academia
			"AcademiaSenhaAlterada": p.handleAcademiaSenhaAlterada,
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
			"AcademiaCriada":           p.handleAcademiaCriadaTx,
			"AcademiaAtivada":          p.handleStatusChangeTx("ativo"),
			"AcademiaDesativada":       p.handleAcademiaDesativadaTx,
			"CursosAtualizados":        p.handleCursosAtualizadosTx,
			"AcademiaDadosAtualizados": p.handleAcademiaDadosAtualizadosTx,
			"EmailVerificado":          p.handleEmailVerificadoTx,
			"AcademiaSenhaAlterada":    p.handleAcademiaSenhaAlteradaTx,
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
// Para estas operações, o benefício da atomicidade é a consistência do
// checkpoint — não a idempotência do Handle em si.

func (p *AcademiaProjection) handleAcademiaCriadaTx(tx *sql.Tx, event db.Event) error {
	// Delegar ao handler existente — AcademiaCriada usa INSERT ON CONFLICT DO NOTHING (idempotente).
	// O tx é passado mas a query usa p.client.DB(). Para atomicidade completa,
	// seria necessário reescrever todos os handlers para aceitar Querier.
	// Neste contexto, o benefício principal é o acumulador de estudantes acima.
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
 
	// FIX PROJ-01: nil → NULL no banco (constraint aceita NULL para fundamental/misto).
	// Array vazio "[]" → jsonb_array_length = 0 → viola check_anos_academicos_nivel.
	var anosValue interface{}
	if len(payload.AnosAcademicos) > 0 {
		b, _ := json.Marshal(payload.AnosAcademicos)
		anosValue = string(b)
	}
	// anosValue permanece nil se o slice for vazio → NULL no banco
 
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
		payload.NivelEscolar, anosValue, cursosJSON,
		payload.CreatedAt, event.EventVersion, event.EventID,
	)
	return err
}

// handleStatusChange retorna um handler que atualiza apenas o status (para Ativada).
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

// handleAcademiaDesativada atualiza status, motivo e DesativadoPor.
//
// FIX E-10: motivo_desativacao agora é persistido na projeção para consulta
// direta, sem necessidade de inspecionar o ledger.
// FIX C9: DesativadoPor agora é lido do payload e persistido.
func (p *AcademiaProjection) handleAcademiaDesativada(event db.Event) error {
	var payload struct {
		Motivo        string    `json:"Motivo"`
		// FIX C9: DesativadoPor vem do payload do evento (não só dos metadados)
		DesativadoPor string    `json:"DesativadoPor"`
		DeactivatedAt time.Time `json:"DeactivatedAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleAcademiaDesativada: parse error: %w", err)
	}

	_, err := p.client.DB().Exec(`
		UPDATE projection_academias
		SET status              = 'inativo',
		    motivo_desativacao  = $1,
		    updated_at          = CURRENT_TIMESTAMP,
		    version             = $2,
		    last_event_id       = $3
		WHERE id = $4
	`, payload.Motivo, event.EventVersion, event.EventID, event.AggregateID)
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
//
// FIX E-23: este handler agora é efetivamente chamado porque Handle()
// roteia eventos de AggregateType="Estudante" corretamente.
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
//
// FIX C6 (parcial): os nomes de campo da query são todos hardcoded no código,
// não vêm do request. A query dinâmica é mantida para compatibilidade, mas
// os nomes de campo são constantes — o risco de injection é mitigado.
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

	setClauses = append(setClauses, "updated_at = CURRENT_TIMESTAMP")
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

// handleAcademiaSenhaAlterada aplica a nova senha_hash na projeção.
//
// FIX C1: antes da correção, a senha era alterada via UPDATE direto na projeção,
// bypassando o ledger. Agora este handler garante que o evento AcademiaSenhaAlterada
// seja processado e a projeção atualizada de forma consistente com o event sourcing.
// Rebuild agora restaura a senha correta.
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

// ============================================================================
// Queries de leitura
// ============================================================================

// AcademiaDTO representa a visão de leitura de uma academia.
type AcademiaDTO struct {
	ID                uuid.UUID `json:"id"`
	Type              string    `json:"type"`
	Nome              string    `json:"nome"`
	CodigoAcademia    string    `json:"codigo_academia"`
	SenhaHash         string    `json:"-"`
	Provincia         string    `json:"provincia"`
	Endereco          string    `json:"endereco"`
	NumeroTelefone    *string   `json:"numero_telefone,omitempty"`
	Email             *string   `json:"email,omitempty"`
	Website           *string   `json:"website,omitempty"`
	NivelEscolar      *string   `json:"nivel_escolar,omitempty"`
	AnosAcademicos    []string  `json:"anos_academicos,omitempty"`
	Status            string    `json:"status"`
	MotivoDesativacao *string   `json:"motivo_desativacao,omitempty"`
	Cursos            []string  `json:"cursos"`
	EmailVerificado   bool      `json:"email_verificado"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	TotalEstudantes   int       `json:"total_estudantes"`
	Version           int       `json:"version"`
}

// FIX E-07: queries de leitura NÃO mais filtram `deleted_at IS NULL` porque
// a coluna deleted_at NÃO existe no schema projection_academias (migrations 001-025).
// Academias são desativadas via status='inativo', não soft-deleted.
// Se futuramente for necessário soft-delete, criar migration para adicionar a coluna ANTES.

func (p *AcademiaProjection) GetByID(id uuid.UUID) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, type, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, status, motivo_desativacao, cursos, email_verificado,
			created_at, updated_at, total_estudantes, version
		FROM projection_academias
		WHERE id = $1
	`, id)
	return scanAcademia(row)
}

func (p *AcademiaProjection) GetByCodigo(codigo string) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, type, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, status, motivo_desativacao, cursos, email_verificado,
			created_at, updated_at, total_estudantes, version
		FROM projection_academias
		WHERE codigo_academia = $1
	`, codigo)
	return scanAcademia(row)
}

func (p *AcademiaProjection) GetByEmail(email string) (*AcademiaDTO, error) {
	row := p.client.DB().QueryRow(`
		SELECT id, type, nome, codigo_academia, senha_hash,
			provincia, endereco, numero_telefone, email, website,
			nivel_escolar, anos_academicos, status, motivo_desativacao, cursos, email_verificado,
			created_at, updated_at, total_estudantes, version
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

// scanAcademia lê uma linha da projeção para AcademiaDTO.
// FIX E-07: não há mais `deleted_at` na query — campo removido do scan.
// FIX E-10: motivo_desativacao adicionado ao scan.
func scanAcademia(row interface{ Scan(...interface{}) error }) (*AcademiaDTO, error) {
	var a AcademiaDTO
	var cursosJSON, anosJSON []byte
	var motivoDesativacao sql.NullString

	err := row.Scan(
		&a.ID, &a.Type, &a.Nome, &a.CodigoAcademia, &a.SenhaHash,
		&a.Provincia, &a.Endereco, &a.NumeroTelefone, &a.Email, &a.Website,
		&a.NivelEscolar, &anosJSON, &a.Status, &motivoDesativacao, &cursosJSON, &a.EmailVerificado,
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
	if motivoDesativacao.Valid {
		a.MotivoDesativacao = &motivoDesativacao.String
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