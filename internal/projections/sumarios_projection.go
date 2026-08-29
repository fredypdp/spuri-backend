package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"spuri/internal/db"

	"github.com/google/uuid"
)

type SumarioDTO struct {
	ID             string  `json:"id"`
	CodigoAcademia string  `json:"codigo_academia"`
	SumarioTitulo  string  `json:"sumario_titulo"`
	Descricao      *string `json:"descricao,omitempty"`
	Periodo        string  `json:"periodo"`
	AnoAcademico   string  `json:"ano_academico"`
	Nivel          string  `json:"nivel"`
	Type           string  `json:"type"`
	CursoID        *string `json:"curso_id,omitempty"`
	MateriaID      string  `json:"materia_id"`
	CriadoPor      *string `json:"criado_por,omitempty"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	Version        int     `json:"version"`
}

type SumariosProjection struct {
	client *db.Client
}

func NewSumariosProjection(client *db.Client) *SumariosProjection {
	return &SumariosProjection{client: client}
}

func (p *SumariosProjection) Name() string {
	return "sumarios"
}

func (p *SumariosProjection) Handle(event db.Event) error {
	handlers := map[string]func(db.Event) error{
		"SumarioCriado":           p.handleSumarioCriado,
		"SumarioDadosAtualizados": p.handleSumarioDadosAtualizados,
		"SumarioDeletado":         p.handleSumarioDeletado,
	}
	if h, ok := handlers[event.EventType]; ok {
		return h(event)
	}
	return nil
}

func (p *SumariosProjection) handleSumarioCriado(event db.Event) error {
	var payload struct {
		CodigoAcademia string     `json:"codigo_academia"`
		SumarioTitulo  string     `json:"sumario_titulo"`
		Descricao      *string    `json:"descricao,omitempty"`
		Periodo        string     `json:"periodo"`
		AnoAcademico   string     `json:"ano_academico"`
		Nivel          string     `json:"nivel"`
		Type           string     `json:"type"`
		CursoID        *uuid.UUID `json:"curso_id,omitempty"`
		MateriaID      uuid.UUID  `json:"materia_id"`
		CriadoPor      uuid.UUID  `json:"criado_por"`
		CriadoEm       string     `json:"criado_em"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleSumarioCriado: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		INSERT INTO projection_sumarios (
			id, codigo_academia, sumario_titulo, descricao, periodo, ano_academico,
			nivel, type, curso_id, materia_id, criado_por, status,
			created_at, updated_at, last_event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'ativo',$12,$12,$13,1)
		ON CONFLICT (id) DO NOTHING
	`, event.AggregateID, payload.CodigoAcademia, payload.SumarioTitulo, payload.Descricao,
		payload.Periodo, payload.AnoAcademico, payload.Nivel, payload.Type,
		payload.CursoID, payload.MateriaID, payload.CriadoPor, payload.CriadoEm, event.EventID)
	if err != nil {
		return fmt.Errorf("handleSumarioCriado: exec error: %w", err)
	}
	return nil
}

func (p *SumariosProjection) handleSumarioDadosAtualizados(event db.Event) error {
	var payload struct {
		SumarioTitulo *string `json:"sumario_titulo,omitempty"`
		Descricao     *string `json:"descricao,omitempty"`
		AtualizadoEm  string  `json:"atualizado_em"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleSumarioDadosAtualizados: parse error: %w", err)
	}

	sets := []string{"updated_at = $1", "last_event_id = $2", "version = version + 1"}
	args := []interface{}{payload.AtualizadoEm, event.EventID}
	if payload.SumarioTitulo != nil {
		args = append(args, *payload.SumarioTitulo)
		sets = append(sets, fmt.Sprintf("sumario_titulo = $%d", len(args)))
	}
	if payload.Descricao != nil {
		args = append(args, *payload.Descricao)
		sets = append(sets, fmt.Sprintf("descricao = $%d", len(args)))
	}
	args = append(args, event.AggregateID)
	query := fmt.Sprintf("UPDATE projection_sumarios SET %s WHERE id = $%d",
		joinComma(sets), len(args))

	if _, err := p.client.DB().Exec(query, args...); err != nil {
		return fmt.Errorf("handleSumarioDadosAtualizados: exec error: %w", err)
	}
	return nil
}

func (p *SumariosProjection) handleSumarioDeletado(event db.Event) error {
	var payload struct {
		DeletadoEm string `json:"deletado_em"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleSumarioDeletado: parse error: %w", err)
	}
	_, err := p.client.DB().Exec(`
		UPDATE projection_sumarios
		SET status = 'deletado', deleted_at = $1, updated_at = $1, last_event_id = $2, version = version + 1
		WHERE id = $3
	`, payload.DeletadoEm, event.EventID, event.AggregateID)
	if err != nil {
		return fmt.Errorf("handleSumarioDeletado: exec error: %w", err)
	}
	return nil
}

// joinComma existe só para não puxar "strings" por um único Join — se o
// pacote já importar "strings" em outro arquivo do mesmo pacote, pode trocar
// isto por strings.Join(sets, ", ") e remover esta função.
func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// ============================================================================
// Leitura
// ============================================================================

func (p *SumariosProjection) GetByID(id uuid.UUID) (*SumarioDTO, error) {
	if id == uuid.Nil {
		return nil, fmt.Errorf("UUID inválido")
	}
	row := p.client.DB().QueryRow(`
		SELECT id, codigo_academia, sumario_titulo, descricao, periodo, ano_academico,
			nivel, type, curso_id, materia_id, criado_por, status, created_at, updated_at, version
		FROM projection_sumarios WHERE id = $1
	`, id)
	return scanSumario(row)
}

func (p *SumariosProjection) GetByAcademia(codigoAcademia string, materiaID, periodo, anoAcademico *string) ([]SumarioDTO, error) {
	query := `
		SELECT id, codigo_academia, sumario_titulo, descricao, periodo, ano_academico,
			nivel, type, curso_id, materia_id, criado_por, status, created_at, updated_at, version
		FROM projection_sumarios
		WHERE codigo_academia = $1 AND deleted_at IS NULL
	`
	args := []interface{}{codigoAcademia}
	if materiaID != nil {
		args = append(args, *materiaID)
		query += fmt.Sprintf(" AND materia_id = $%d", len(args))
	}
	if periodo != nil {
		args = append(args, *periodo)
		query += fmt.Sprintf(" AND periodo = $%d", len(args))
	}
	if anoAcademico != nil {
		args = append(args, *anoAcademico)
		query += fmt.Sprintf(" AND ano_academico = $%d", len(args))
	}
	query += " ORDER BY created_at DESC"

	rows, err := p.client.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSumarios(rows)
}

func scanSumario(row *sql.Row) (*SumarioDTO, error) {
	var dto SumarioDTO
	var descricao, cursoID, criadoPor sql.NullString
	err := row.Scan(
		&dto.ID, &dto.CodigoAcademia, &dto.SumarioTitulo, &descricao, &dto.Periodo, &dto.AnoAcademico,
		&dto.Nivel, &dto.Type, &cursoID, &dto.MateriaID, &criadoPor, &dto.Status,
		&dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if descricao.Valid {
		dto.Descricao = &descricao.String
	}
	if cursoID.Valid {
		dto.CursoID = &cursoID.String
	}
	if criadoPor.Valid {
		dto.CriadoPor = &criadoPor.String
	}
	return &dto, nil
}

func scanSumarios(rows *sql.Rows) ([]SumarioDTO, error) {
	var result []SumarioDTO
	for rows.Next() {
		var dto SumarioDTO
		var descricao, cursoID, criadoPor sql.NullString
		if err := rows.Scan(
			&dto.ID, &dto.CodigoAcademia, &dto.SumarioTitulo, &descricao, &dto.Periodo, &dto.AnoAcademico,
			&dto.Nivel, &dto.Type, &cursoID, &dto.MateriaID, &criadoPor, &dto.Status,
			&dto.CreatedAt, &dto.UpdatedAt, &dto.Version,
		); err != nil {
			return nil, err
		}
		if descricao.Valid {
			dto.Descricao = &descricao.String
		}
		if cursoID.Valid {
			dto.CursoID = &cursoID.String
		}
		if criadoPor.Valid {
			dto.CriadoPor = &criadoPor.String
		}
		result = append(result, dto)
	}
	return result, rows.Err()
}

// Rebuild reprocessa todos os eventos "Sumario*" do zero. Segue o mesmo padrão
// de materias_projection.go: se quiser usar o ExistenceCache para validar que
// materia_id existe em projection_materias antes de inserir (mesma ideia que
// materias_projection.go usa para academia_id), copie esse padrão daqui.
// Deixei a assinatura mínima abaixo; ajuste para bater com a interface Projection
// real usada pelas outras projections (verifique manager.go / a interface
// Projection em internal/projections).
func (p *SumariosProjection) GetLastProcessedEventID() (int64, error) {
	return NewBaseProjection(p.client).GetLastProcessedEventIDByName(p.Name())
}
func (p *SumariosProjection) UpdateCheckpoint(eventID int64) error {
	return NewBaseProjection(p.client).UpdateCheckpointByName(p.Name(), eventID)
}
func (p *SumariosProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE TABLE projection_sumarios CASCADE`); err != nil {
		return fmt.Errorf("Rebuild: clear error: %w", err)
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='Sumario' ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e db.Event
		var prev sql.NullString
		if err := rows.Scan(&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.EventVersion, &e.Payload, &e.Metadata, &e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &prev); err != nil {
			return err
		}
		if prev.Valid {
			e.PreviousHash = &prev.String
		}
		if err := p.Handle(e); err != nil {
			return err
		}
	}
	return rows.Err()
}
