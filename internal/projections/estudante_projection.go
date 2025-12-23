// ============================================================================
// ARQUIVO: internal/projections/estudante_projection.go
// CORREÇÃO: Salvar senha_hash na projeção de estudantes
// ============================================================================

package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log" // 🔥 ADICIONAR
	"spuri/internal/genesisdb"
	"time"

	"github.com/google/uuid"
)

// EstudanteProjection projeção de estudantes
type EstudanteProjection struct {
	client *genesisdb.Client
	ctx    context.Context
}

// NewEstudanteProjection cria nova projeção de estudante
func NewEstudanteProjection(client *genesisdb.Client) *EstudanteProjection {
	return &EstudanteProjection{
		client: client,
		ctx:    context.Background(),
	}
}

// Name implementa Projection
func (p *EstudanteProjection) Name() string {
	return "estudantes"
}

// Handle processa um evento
func (p *EstudanteProjection) Handle(event genesisdb.Event) error {
	// Processar apenas eventos de Estudante
	if event.AggregateType != "Estudante" {
		return nil
	}

	switch event.EventType {
	case "EstudanteCriado":
		return p.handleEstudanteCriado(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "VinculoAtualizado":
		return p.handleVinculoAtualizado(event)
	default:
		// Eventos que não afetam esta projeção
		return nil
	}
}

// Rebuild reconstrói a projeção do zero
func (p *EstudanteProjection) Rebuild() error {
	// 1. Limpar projeção existente
	if err := p.clear(); err != nil {
		return err
	}

	// 2. Buscar todos os eventos de Estudante
	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM genesis_ledger
		WHERE aggregate_type = 'Estudante'
		ORDER BY id ASC
	`

	var events []genesisdb.Event
	if err := p.client.DB().Select(&events, query); err != nil {
		return err
	}

	// 3. Processar todos os eventos
	for _, event := range events {
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro ao processar evento %d: %w", event.ID, err)
		}
	}

	return nil
}

// GetLastProcessedEventID implementa Projection
func (p *EstudanteProjection) GetLastProcessedEventID() (int64, error) {
	query := `
		SELECT last_processed_event_id 
		FROM projection_checkpoints 
		WHERE projection_name = $1
	`

	var lastID int64
	err := p.client.DB().Get(&lastID, query, p.Name())
	if err != nil {
		return 0, err
	}

	return lastID, nil
}

// UpdateCheckpoint implementa Projection
func (p *EstudanteProjection) UpdateCheckpoint(eventID int64) error {
	query := `
		UPDATE projection_checkpoints
		SET 
			last_processed_event_id = $1,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = events_processed + 1
		WHERE projection_name = $2
	`

	_, err := p.client.DB().Exec(query, eventID, p.Name())
	return err
}

// clear limpa a projeção
func (p *EstudanteProjection) clear() error {
	query := `TRUNCATE TABLE projection_estudantes CASCADE`
	_, err := p.client.DB().Exec(query)
	return err
}

// Event Handlers

// 🔥 FIX: Adicionar SenhaHash ao payload e salvar na projeção
func (p *EstudanteProjection) handleEstudanteCriado(event genesisdb.Event) error {
	log.Printf("🔵 [PROJEÇÃO ESTUDANTE] Iniciando processamento de EstudanteCriado")
	log.Printf("   Event ID: %s", event.EventID)
	log.Printf("   Aggregate ID: %s", event.AggregateID)
	
	var payload struct {
		Nome                  string     `json:"Nome"`
		SenhaHash             string     `json:"SenhaHash"` // 🔥 ADICIONAR
		BilheteIdentidade     *string    `json:"BilheteIdentidade"`
		BilheteIdentidadeResp *string    `json:"BilheteIdentidadeResp"`
		AnoEscolar            *string    `json:"AnoEscolar"`
		AnoSuperior           *string    `json:"AnoSuperior"`
		CursoMedio            *string    `json:"CursoMedio"`
		CursoSuperior         *string    `json:"CursoSuperior"`
		StatusEscolar         *string    `json:"StatusEscolar"`
		StatusSuperior        *string    `json:"StatusSuperior"`
		CreatedAt             time.Time  `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		log.Printf("❌ [PROJEÇÃO ESTUDANTE] Erro ao parsear payload: %v", err)
		log.Printf("   Payload raw: %s", string(event.Payload))
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	log.Printf("📊 [PROJEÇÃO ESTUDANTE] Dados parseados:")
	log.Printf("   Nome: %s", payload.Nome)
	log.Printf("   SenhaHash existe: %v (length: %d)", payload.SenhaHash != "", len(payload.SenhaHash))
	
	// Verificar se senha está no payload
	if payload.SenhaHash == "" {
		log.Printf("❌ [PROJEÇÃO ESTUDANTE] SenhaHash vazio no evento!")
		return fmt.Errorf("SenhaHash vazio no evento")
	}

	// 🔥 FIX: Incluir senha_hash na query INSERT
	query := `
		INSERT INTO projection_estudantes (
			id, nome, senha_hash, bilhete_identidade, bilhete_identidade_responsavel,
			ano_escolar, ano_superior, curso_medio, curso_superior,
			status_escolar, status_superior, version, created_at,
			updated_at, last_event_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO UPDATE SET
			nome = $2,
			senha_hash = $3,
			bilhete_identidade = $4,
			bilhete_identidade_responsavel = $5,
			ano_escolar = $6,
			ano_superior = $7,
			curso_medio = $8,
			curso_superior = $9,
			status_escolar = $10,
			status_superior = $11,
			version = $12,
			updated_at = $14,
			last_event_id = $15
	`

	log.Printf("🔄 [PROJEÇÃO ESTUDANTE] Executando INSERT na tabela...")
	result, err := p.client.DB().Exec(
		query,
		event.AggregateID,
		payload.Nome,
		payload.SenhaHash, // 🔥 ADICIONAR
		payload.BilheteIdentidade,
		payload.BilheteIdentidadeResp,
		payload.AnoEscolar,
		payload.AnoSuperior,
		payload.CursoMedio,
		payload.CursoSuperior,
		payload.StatusEscolar,
		payload.StatusSuperior,
		event.EventVersion,
		payload.CreatedAt,
		time.Now(),
		event.EventID,
	)

	if err != nil {
		log.Printf("❌ [PROJEÇÃO ESTUDANTE] Erro ao executar INSERT: %v", err)
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("✅ [PROJEÇÃO ESTUDANTE] Estudante salvo com sucesso! (rows affected: %d)", rowsAffected)
	
	// Verificar se realmente salvou
	var count int
	checkQuery := `SELECT COUNT(*) FROM projection_estudantes WHERE id = $1`
	p.client.DB().Get(&count, checkQuery, event.AggregateID)
	log.Printf("🔍 [PROJEÇÃO ESTUDANTE] Verificação: %d registro(s) encontrado(s) com ID %s", count, event.AggregateID)

	return nil
}

func (p *EstudanteProjection) handleInscricaoAprovada(event genesisdb.Event) error {
	var payload struct {
		AcademiaID uuid.UUID `json:"AcademiaID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		UPDATE projection_estudantes
		SET 
			id_academia = $1,
			version = $2,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $3,
			total_inscricoes = total_inscricoes + 1
		WHERE id = $4
	`

	_, err := p.client.DB().Exec(
		query,
		payload.AcademiaID,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

func (p *EstudanteProjection) handleVinculoAtualizado(event genesisdb.Event) error {
	var payload struct {
		NovaAcademiaID uuid.UUID `json:"NovaAcademiaID"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := `
		UPDATE projection_estudantes
		SET 
			id_academia = $1,
			version = $2,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = $3
		WHERE id = $4
	`

	_, err := p.client.DB().Exec(
		query,
		payload.NovaAcademiaID,
		event.EventVersion,
		event.EventID,
		event.AggregateID,
	)

	return err
}

// Query methods - para uso nos handlers

// GetByID busca estudante por ID na projeção
func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	query := `
		SELECT 
			id, nome, senha_hash, bilhete_identidade, bilhete_identidade_responsavel,
			id_academia, ano_escolar, ano_superior, curso_medio, curso_superior,
			status_escolar, status_superior, created_at, updated_at,
			total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE id = $1
	`

	var dto EstudanteDTO
	err := p.client.DB().Get(&dto, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

// GetByBilhete busca estudante por bilhete
func (p *EstudanteProjection) GetByBilhete(bilhete string) (*EstudanteDTO, error) {
	query := `
		SELECT 
			id, nome, senha_hash, bilhete_identidade, bilhete_identidade_responsavel,
			id_academia, ano_escolar, ano_superior, curso_medio, curso_superior,
			status_escolar, status_superior, created_at, updated_at,
			total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE bilhete_identidade = $1 OR bilhete_identidade_responsavel = $1
		LIMIT 1
	`

	var dto EstudanteDTO
	err := p.client.DB().Get(&dto, query, bilhete)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

// EstudanteDTO DTO da projeção
type EstudanteDTO struct {
	ID                    uuid.UUID  `db:"id" json:"id"`
	Nome                  string     `db:"nome" json:"nome"`
	SenhaHash             string     `db:"senha_hash" json:"-"` // 🔥 ADICIONAR (nunca expor no JSON)
	BilheteIdentidade     *string    `db:"bilhete_identidade" json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp *string    `db:"bilhete_identidade_responsavel" json:"bilhete_identidade_responsavel,omitempty"`
	IDAcademia            *uuid.UUID `db:"id_academia" json:"id_academia,omitempty"`
	AnoEscolar            *string    `db:"ano_escolar" json:"ano_escolar,omitempty"`
	AnoSuperior           *string    `db:"ano_superior" json:"ano_superior,omitempty"`
	CursoMedio            *string    `db:"curso_medio" json:"curso_medio,omitempty"`
	CursoSuperior         *string    `db:"curso_superior" json:"curso_superior,omitempty"`
	StatusEscolar         *string    `db:"status_escolar" json:"status_escolar,omitempty"`
	StatusSuperior        *string    `db:"status_superior" json:"status_superior,omitempty"`
	CreatedAt             time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time  `db:"updated_at" json:"updated_at"`
	TotalNotas            int        `db:"total_notas" json:"total_notas"`
	TotalFaltas           int        `db:"total_faltas" json:"total_faltas"`
	TotalInscricoes       int        `db:"total_inscricoes" json:"total_inscricoes"`
	Version               int        `db:"version" json:"version"`
}