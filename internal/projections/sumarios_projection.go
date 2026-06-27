package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/db"
	"time"

	"github.com/google/uuid"
)

type SumariosProjection struct{ client *db.Client }

func NewSumariosProjection(client *db.Client) *SumariosProjection {
	return &SumariosProjection{client: client}
}
func (p *SumariosProjection) Name() string { return "sumarios" }
func (p *SumariosProjection) GetLastProcessedEventID() (int64, error) {
	var id int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}
func (p *SumariosProjection) UpdateCheckpoint(eventID int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES ($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT (projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), eventID)
	return err
}
func (p *SumariosProjection) Handle(event db.Event) error {
	if event.AggregateType != "SumarioAula" {
		return nil
	}
	switch event.EventType {
	case "SumarioAulaCriado":
		return p.handleCriado(event)
	case "SumarioAulaAtualizado":
		return p.handleAtualizado(event)
	case "SumarioAulaDesativado":
		return p.handleDesativado(event)
	}
	return nil
}
func (p *SumariosProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE TABLE projection_sumarios_aulas CASCADE`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='SumarioAula' ORDER BY id ASC`)
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
			return fmt.Errorf("erro no evento %d: %w", e.ID, err)
		}
	}
	return rows.Err()
}
func (p *SumariosProjection) handleCriado(event db.Event) error {
	var x struct {
		AcademiaID     uuid.UUID
		CodigoAcademia string
		SumarioTitulo  string
		Descricao      *string
		Periodo        string
		AnoAcademico   int
		Nivel          string
		Type           string
		CursoID        *uuid.UUID
		MateriaID      uuid.UUID
		CriadoPor      uuid.UUID
		CriadoEm       time.Time
		AtualizadoEm   time.Time
	}
	if err := json.Unmarshal(event.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`INSERT INTO projection_sumarios_aulas (id,academia_id,codigo_academia,sumario_titulo,descricao,periodo,ano_academico,nivel,type,curso_id,materia_id,criado_por,criado_em,atualizado_em,event_id,version) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT (id) DO NOTHING`, event.AggregateID, x.AcademiaID, x.CodigoAcademia, x.SumarioTitulo, x.Descricao, x.Periodo, x.AnoAcademico, x.Nivel, x.Type, x.CursoID, x.MateriaID, x.CriadoPor, x.CriadoEm, x.AtualizadoEm, event.EventID, event.EventVersion)
	return err
}
func (p *SumariosProjection) handleAtualizado(event db.Event) error {
	var x struct {
		SumarioTitulo *string
		Descricao     *string
		Periodo       *string
		AnoAcademico  *int
		CursoID       *uuid.UUID
		MateriaID     *uuid.UUID
		AtualizadoEm  time.Time
	}
	if err := json.Unmarshal(event.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`UPDATE projection_sumarios_aulas SET sumario_titulo=COALESCE($1,sumario_titulo), descricao=COALESCE($2,descricao), periodo=COALESCE($3,periodo), ano_academico=COALESCE($4,ano_academico), curso_id=COALESCE($5,curso_id), materia_id=COALESCE($6,materia_id), atualizado_em=$7, event_id=$8, version=$9 WHERE id=$10 AND deleted_at IS NULL`, x.SumarioTitulo, x.Descricao, x.Periodo, x.AnoAcademico, x.CursoID, x.MateriaID, x.AtualizadoEm, event.EventID, event.EventVersion, event.AggregateID)
	return err
}
func (p *SumariosProjection) handleDesativado(event db.Event) error {
	var x struct{ DesativadoEm time.Time }
	_ = json.Unmarshal(event.Payload, &x)
	if x.DesativadoEm.IsZero() {
		x.DesativadoEm = event.OccurredAt
	}
	_, err := p.client.DB().Exec(`UPDATE projection_sumarios_aulas SET deleted_at=$1,event_id=$2,version=$3 WHERE id=$4 AND deleted_at IS NULL`, x.DesativadoEm, event.EventID, event.EventVersion, event.AggregateID)
	return err
}

type SumarioDTO struct {
	ID             uuid.UUID  `json:"id"`
	AcademiaID     uuid.UUID  `json:"academia_id"`
	CodigoAcademia string     `json:"codigo_academia"`
	SumarioTitulo  string     `json:"sumario_titulo"`
	Descricao      *string    `json:"descricao,omitempty"`
	Periodo        string     `json:"periodo"`
	AnoAcademico   int        `json:"ano_academico"`
	Nivel          string     `json:"nivel"`
	Type           string     `json:"type"`
	CursoID        *uuid.UUID `json:"curso_id,omitempty"`
	MateriaID      uuid.UUID  `json:"materia_id"`
	CriadoPor      uuid.UUID  `json:"criado_por"`
	CriadoEm       time.Time  `json:"criado_em"`
	AtualizadoEm   time.Time  `json:"atualizado_em"`
}

func (p *SumariosProjection) GetByID(id uuid.UUID) (*SumarioDTO, error) {
	rows, err := p.client.DB().Query(`SELECT id,academia_id,codigo_academia,sumario_titulo,descricao,periodo,ano_academico,nivel,type,curso_id,materia_id,criado_por,criado_em,atualizado_em FROM projection_sumarios_aulas WHERE id=$1 AND deleted_at IS NULL`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	xs, err := scanSumarios(rows)
	if err != nil || len(xs) == 0 {
		return nil, err
	}
	return &xs[0], nil
}
func (p *SumariosProjection) List(codigoAcademia, periodo, ano, cursoID, materiaID string) ([]SumarioDTO, error) {
	q := `SELECT id,academia_id,codigo_academia,sumario_titulo,descricao,periodo,ano_academico,nivel,type,curso_id,materia_id,criado_por,criado_em,atualizado_em FROM projection_sumarios_aulas WHERE codigo_academia=$1 AND deleted_at IS NULL AND ($2='' OR periodo=$2) AND ($3='' OR ano_academico::text=$3) AND ($4='' OR curso_id::text=$4) AND ($5='' OR materia_id::text=$5) ORDER BY criado_em DESC`
	rows, err := p.client.DB().Query(q, codigoAcademia, periodo, ano, cursoID, materiaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSumarios(rows)
}
func scanSumarios(rows *sql.Rows) ([]SumarioDTO, error) {
	var out []SumarioDTO
	for rows.Next() {
		var x SumarioDTO
		var desc sql.NullString
		var curso sql.NullString
		if err := rows.Scan(&x.ID, &x.AcademiaID, &x.CodigoAcademia, &x.SumarioTitulo, &desc, &x.Periodo, &x.AnoAcademico, &x.Nivel, &x.Type, &curso, &x.MateriaID, &x.CriadoPor, &x.CriadoEm, &x.AtualizadoEm); err != nil {
			return out, err
		}
		if desc.Valid {
			x.Descricao = &desc.String
		}
		if curso.Valid {
			u, _ := uuid.Parse(curso.String)
			x.CursoID = &u
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
