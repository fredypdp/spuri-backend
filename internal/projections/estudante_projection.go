// ============================================================================
// ARQUIVO: internal/projections/estudante_projection.go
// 🔥 COMPLETO COM NOVOS EVENTOS
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

type EstudanteProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewEstudanteProjection(client *db.Client) *EstudanteProjection {
	return &EstudanteProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (p *EstudanteProjection) Name() string {
	return "estudantes"
}

func (p *EstudanteProjection) Handle(event db.Event) error {
	if event.AggregateType != "Estudante" {
		return nil
	}

	switch event.EventType {
	case "EstudanteCriado":
		return p.handleEstudanteCriado(event)
	case "InscricaoAprovada":
		return p.handleInscricaoAprovada(event)
	case "EstudanteVinculado": // 🔥 NOVO
		return p.handleEstudanteVinculado(event)
	case "StatusEscolarAtualizado": // 🔥 NOVO
		return p.handleStatusEscolarAtualizado(event)
	case "StatusSuperiorAtualizado": // 🔥 NOVO
		return p.handleStatusSuperiorAtualizado(event)
	default:
		return nil
	}
}

func (p *EstudanteProjection) Rebuild() error {
	if err := p.clear(); err != nil {
		return err
	}

	query := `
		SELECT 
			id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Estudante'
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

func (p *EstudanteProjection) GetLastProcessedEventID() (int64, error) {
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

func (p *EstudanteProjection) UpdateCheckpoint(eventID int64) error {
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

func (p *EstudanteProjection) clear() error {
	_, err := p.client.DB().Exec(`TRUNCATE TABLE projection_estudantes CASCADE`)
	return err
}

// 🔥 ATUALIZADO: Incluir status, status_escolar, status_superior
func (p *EstudanteProjection) handleEstudanteCriado(event db.Event) error {
	log.Printf("🔵 [PROJEÇÃO ESTUDANTE] Processando EstudanteCriado")

	var payload struct {
		Nome                  string    `json:"Nome"`
		CodigoEstudante       string    `json:"CodigoEstudante"`
		SenhaHash             string    `json:"SenhaHash"`
		BilheteIdentidade     *string   `json:"BilheteIdentidade"`
		BilheteIdentidadeResp *string   `json:"BilheteIdentidadeResp"`
		AnoEscolar            *string   `json:"AnoEscolar"`
		AnoSuperior           *string   `json:"AnoSuperior"`
		CursoMedio            *string   `json:"CursoMedio"`
		CursoSuperior         *string   `json:"CursoSuperior"`
		StatusEscolar         string    `json:"StatusEscolar"`  // 🔥 ADICIONADO
		StatusSuperior        string    `json:"StatusSuperior"` // 🔥 ADICIONADO
		CreatedAt             time.Time `json:"CreatedAt"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	if payload.SenhaHash == "" {
		return fmt.Errorf("SenhaHash vazio no evento")
	}

	if payload.CodigoEstudante == "" {
		return fmt.Errorf("CodigoEstudante vazio no evento")
	}

	log.Printf("📋 [ESTUDANTE] Nome: %s, Código: %s", payload.Nome, payload.CodigoEstudante)
	log.Printf("🔐 [ESTUDANTE] SenhaHash (primeiros 30): %s...", payload.SenhaHash[:30])

	query := fmt.Sprintf(`
		INSERT INTO projection_estudantes (
			id, nome, codigo_estudante, senha_hash, bilhete_identidade, 
			bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, 
			curso_medio, curso_superior,
			version, created_at, updated_at, last_event_id
		) VALUES (
			'%s', '%s', '%s', '%s', %s, 
			%s, NULL,
			'inativo', '%s', '%s',
			%s, %s, 
			%s, %s,
			%d, '%s', '%s', '%s'
		)
		ON CONFLICT (id) DO UPDATE SET
			nome = EXCLUDED.nome,
			codigo_estudante = EXCLUDED.codigo_estudante,
			senha_hash = EXCLUDED.senha_hash,
			bilhete_identidade = EXCLUDED.bilhete_identidade,
			bilhete_identidade_responsavel = EXCLUDED.bilhete_identidade_responsavel,
			codigo_academia = EXCLUDED.codigo_academia,
			status = EXCLUDED.status,
			status_escolar = EXCLUDED.status_escolar,
			status_superior = EXCLUDED.status_superior,
			ano_escolar = EXCLUDED.ano_escolar,
			ano_superior = EXCLUDED.ano_superior,
			curso_medio = EXCLUDED.curso_medio,
			curso_superior = EXCLUDED.curso_superior,
			version = EXCLUDED.version,
			updated_at = EXCLUDED.updated_at,
			last_event_id = EXCLUDED.last_event_id
	`,
		event.AggregateID.String(),
		escapeStringEstudante(payload.Nome),
		payload.CodigoEstudante,
		payload.SenhaHash,
		formatNullableStringEstudante(payload.BilheteIdentidade),
		formatNullableStringEstudante(payload.BilheteIdentidadeResp),
		payload.StatusEscolar,
		payload.StatusSuperior,
		formatNullableStringEstudante(payload.AnoEscolar),
		formatNullableStringEstudante(payload.AnoSuperior),
		formatNullableStringEstudante(payload.CursoMedio),
		formatNullableStringEstudante(payload.CursoSuperior),
		event.EventVersion,
		payload.CreatedAt.Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
		event.EventID.String(),
	)

	result, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("❌ [ESTUDANTE] Erro ao salvar: %v", err)
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("✅ [PROJEÇÃO ESTUDANTE] Salvo com sucesso! (rows: %d)", rowsAffected)

	return nil
}

func (p *EstudanteProjection) handleInscricaoAprovada(event db.Event) error {
	var payload struct {
		CodigoAcademia string `json:"CodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	// NÃO vincular ainda, só incrementar contador
	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET 
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s',
			total_inscricoes = total_inscricoes + 1
		WHERE id = '%s'
	`,
		event.EventVersion,
		event.EventID.String(),
		event.AggregateID.String(),
	)

	_, err := p.client.DB().Exec(query)
	return err
}

// 🔥 NOVO: Vincular estudante à academia e tornar ativo
func (p *EstudanteProjection) handleEstudanteVinculado(event db.Event) error {
	log.Printf("🔗 [VINCULAÇÃO] Processando EstudanteVinculado")

	var payload struct {
		CodigoAcademia string `json:"CodigoAcademia"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET 
			codigo_academia = '%s',
			status = 'ativo',
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s'
		WHERE id = '%s'
	`,
		payload.CodigoAcademia,
		event.EventVersion,
		event.EventID.String(),
		event.AggregateID.String(),
	)

	result, err := p.client.DB().Exec(query)
	if err != nil {
		log.Printf("❌ [VINCULAÇÃO] Erro: %v", err)
		return err
	}

	rows, _ := result.RowsAffected()
	log.Printf("✅ [VINCULAÇÃO] Estudante vinculado e ativado! (rows: %d)", rows)

	return nil
}

// 🔥 NOVO: Atualizar status escolar
func (p *EstudanteProjection) handleStatusEscolarAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	// Se escolar vira inativo, superior também
	statusSuperior := ""
	if payload.NovoStatus == "inativo" {
		statusSuperior = ", status_superior = 'inativo'"
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET 
			status_escolar = '%s'%s,
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s'
		WHERE id = '%s'
	`,
		payload.NovoStatus,
		statusSuperior,
		event.EventVersion,
		event.EventID.String(),
		event.AggregateID.String(),
	)

	_, err := p.client.DB().Exec(query)
	return err
}

// 🔥 NOVO: Atualizar status superior
func (p *EstudanteProjection) handleStatusSuperiorAtualizado(event db.Event) error {
	var payload struct {
		NovoStatus string `json:"NovoStatus"`
	}

	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao parsear payload: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE projection_estudantes
		SET 
			status_superior = '%s',
			version = %d,
			updated_at = CURRENT_TIMESTAMP,
			last_event_id = '%s'
		WHERE id = '%s'
	`,
		payload.NovoStatus,
		event.EventVersion,
		event.EventID.String(),
		event.AggregateID.String(),
	)

	_, err := p.client.DB().Exec(query)
	return err
}

// Query methods

func (p *EstudanteProjection) GetByID(id uuid.UUID) (*EstudanteDTO, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, nome, codigo_estudante, senha_hash, bilhete_identidade, 
			bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, 
			curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE id = '%s'
	`, id.String())

	var dto EstudanteDTO
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalNotas, &dto.TotalFaltas, &dto.TotalInscricoes, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

func (p *EstudanteProjection) GetByCodigo(codigo string) (*EstudanteDTO, error) {
	log.Printf("🔎 [PROJEÇÃO ESTUDANTE] GetByCodigo: %s", codigo)

	query := fmt.Sprintf(`
		SELECT 
			id, nome, codigo_estudante, senha_hash, bilhete_identidade, 
			bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, 
			curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE codigo_estudante = '%s'
	`, codigo)

	var dto EstudanteDTO
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalNotas, &dto.TotalFaltas, &dto.TotalInscricoes, &dto.Version,
	)
	if err == sql.ErrNoRows {
		log.Printf("❌ [PROJEÇÃO ESTUDANTE] Não encontrado: %s", codigo)
		return nil, nil
	}
	if err != nil {
		log.Printf("❌ [PROJEÇÃO ESTUDANTE] Erro: %v", err)
		return nil, err
	}

	log.Printf("✅ [PROJEÇÃO ESTUDANTE] Encontrado: %s", dto.Nome)
	return &dto, nil
}

func (p *EstudanteProjection) GetByBilhete(bilhete string) (*EstudanteDTO, error) {
	query := fmt.Sprintf(`
		SELECT 
			id, nome, codigo_estudante, senha_hash, bilhete_identidade, 
			bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar, status_superior,
			ano_escolar, ano_superior, 
			curso_medio, curso_superior,
			created_at, updated_at, total_notas, total_faltas, total_inscricoes, version
		FROM projection_estudantes
		WHERE bilhete_identidade = '%s' OR bilhete_identidade_responsavel = '%s'
		LIMIT 1
	`, bilhete, bilhete)

	var dto EstudanteDTO
	err := p.client.DB().QueryRow(query).Scan(
		&dto.ID, &dto.Nome, &dto.CodigoEstudante, &dto.SenhaHash,
		&dto.BilheteIdentidade, &dto.BilheteIdentidadeResp, &dto.CodigoAcademia,
		&dto.Status, &dto.StatusEscolar, &dto.StatusSuperior,
		&dto.AnoEscolar, &dto.AnoSuperior, &dto.CursoMedio, &dto.CursoSuperior,
		&dto.CreatedAt, &dto.UpdatedAt,
		&dto.TotalNotas, &dto.TotalFaltas, &dto.TotalInscricoes, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &dto, nil
}

type EstudanteDTO struct {
	ID                    uuid.UUID `db:"id" json:"id"`
	Nome                  string    `db:"nome" json:"nome"`
	CodigoEstudante       string    `db:"codigo_estudante" json:"codigo_estudante"`
	SenhaHash             string    `db:"senha_hash" json:"-"`
	BilheteIdentidade     *string   `db:"bilhete_identidade" json:"bilhete_identidade,omitempty"`
	BilheteIdentidadeResp *string   `db:"bilhete_identidade_responsavel" json:"bilhete_identidade_responsavel,omitempty"`
	CodigoAcademia        *string   `db:"codigo_academia" json:"codigo_academia,omitempty"`
	Status                string    `db:"status" json:"status"`                   // 🔥 NOVO
	StatusEscolar         string    `db:"status_escolar" json:"status_escolar"`   // 🔥 ATUALIZADO
	StatusSuperior        string    `db:"status_superior" json:"status_superior"` // 🔥 ATUALIZADO
	AnoEscolar            *string   `db:"ano_escolar" json:"ano_escolar,omitempty"`
	AnoSuperior           *string   `db:"ano_superior" json:"ano_superior,omitempty"`
	CursoMedio            *string   `db:"curso_medio" json:"curso_medio,omitempty"`
	CursoSuperior         *string   `db:"curso_superior" json:"curso_superior,omitempty"`
	CreatedAt             time.Time `db:"created_at" json:"created_at"`
	UpdatedAt             time.Time `db:"updated_at" json:"updated_at"`
	TotalNotas            int       `db:"total_notas" json:"total_notas"`
	TotalFaltas           int       `db:"total_faltas" json:"total_faltas"`
	TotalInscricoes       int       `db:"total_inscricoes" json:"total_inscricoes"`
	Version               int       `db:"version" json:"version"`
}

// Helpers

func escapeStringEstudante(s string) string {
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

func formatNullableStringEstudante(s *string) string {
	if s == nil {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", escapeStringEstudante(*s))
}
