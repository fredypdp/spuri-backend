package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

type DocumentoExtraProjection struct{ client *db.Client }

func NewDocumentoExtraProjection(c *db.Client) *DocumentoExtraProjection {
	return &DocumentoExtraProjection{c}
}
func (p *DocumentoExtraProjection) Name() string { return "documentos_extra" }
func (p *DocumentoExtraProjection) GetLastProcessedEventID() (int64, error) {
	var v int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
func (p *DocumentoExtraProjection) UpdateCheckpoint(id int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT(projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return err
}
func (p *DocumentoExtraProjection) Handle(e db.Event) error {
	if e.AggregateType != "DocumentoExtra" {
		return nil
	}
	switch e.EventType {
	case "DocumentoExtraCriado":
		return p.created(e)
	case "DocumentoExtraAtualizado":
		return p.updated(e)
	case "DocumentoExtraDesativado":
		return p.active(e, false)
	case "DocumentoExtraReativado":
		return p.active(e, true)
	}
	return nil
}
func (p *DocumentoExtraProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE projection_documentos_extra CASCADE`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='DocumentoExtra' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e db.Event
		var prev sql.NullString
		if err = rows.Scan(&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.EventVersion, &e.Payload, &e.Metadata, &e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &prev); err != nil {
			return err
		}
		if prev.Valid {
			e.PreviousHash = &prev.String
		}
		if err = p.Handle(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

type DocumentoExtraDTO struct {
	ID             uuid.UUID `json:"id"`
	CodigoAcademia string    `json:"codigo_academia"`
	Rotulo         string    `json:"rotulo"`
	Tipo           string    `json:"tipo"`
	Obrigatorio    bool      `json:"obrigatorio"`
	Nivel          string    `json:"nivel"`
	AnoAcademico   string    `json:"ano_academico"`
	Ativo          bool      `json:"ativo"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (p *DocumentoExtraProjection) created(e db.Event) error {
	var x struct {
		CodigoAcademia string
		Rotulo         string
		Tipo           string
		Obrigatorio    bool
		Nivel          string
		AnoAcademico   string
		CriadoPor      uuid.UUID
		CreatedAt      time.Time
	}
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(
		`INSERT INTO projection_documentos_extra(id,codigo_academia,rotulo,tipo,obrigatorio,nivel,ano_academico,ativo,criado_por,created_at,updated_at,version,last_event_id)
		 VALUES($1,$2,$3,$4,$5,$6,$7,true,$8,$9,$9,$10,$11) ON CONFLICT(id) DO NOTHING`,
		e.AggregateID, x.CodigoAcademia, x.Rotulo, x.Tipo, x.Obrigatorio, x.Nivel, x.AnoAcademico, x.CriadoPor, x.CreatedAt, e.EventVersion, e.EventID)
	return err
}
func (p *DocumentoExtraProjection) updated(e db.Event) error {
	var x struct {
		Rotulo       string
		Tipo         string
		Obrigatorio  bool
		Nivel        string
		AnoAcademico string
	}
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(
		`UPDATE projection_documentos_extra SET rotulo=$1,tipo=$2,obrigatorio=$3,nivel=$4,ano_academico=$5,version=$6,last_event_id=$7,updated_at=CURRENT_TIMESTAMP WHERE id=$8`,
		x.Rotulo, x.Tipo, x.Obrigatorio, x.Nivel, x.AnoAcademico, e.EventVersion, e.EventID, e.AggregateID)
	return err
}
func (p *DocumentoExtraProjection) active(e db.Event, ativo bool) error {
	_, err := p.client.DB().Exec(`UPDATE projection_documentos_extra SET ativo=$1,version=$2,last_event_id=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$4`, ativo, e.EventVersion, e.EventID, e.AggregateID)
	return err
}
func (p *DocumentoExtraProjection) scan(row interface{ Scan(...interface{}) error }) (*DocumentoExtraDTO, error) {
	var d DocumentoExtraDTO
	err := row.Scan(&d.ID, &d.CodigoAcademia, &d.Rotulo, &d.Tipo, &d.Obrigatorio, &d.Nivel, &d.AnoAcademico, &d.Ativo, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

const documentoExtraCols = `id,codigo_academia,rotulo,tipo,obrigatorio,nivel,ano_academico,ativo,created_at,updated_at`

func (p *DocumentoExtraProjection) GetByID(id uuid.UUID) (*DocumentoExtraDTO, error) {
	d, err := p.scan(p.client.DB().QueryRow(`SELECT `+documentoExtraCols+` FROM projection_documentos_extra WHERE id=$1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}
func (p *DocumentoExtraProjection) GetByAcademia(codigo string, ativosOnly bool) ([]DocumentoExtraDTO, error) {
	q := `SELECT ` + documentoExtraCols + ` FROM projection_documentos_extra WHERE codigo_academia=$1`
	if ativosOnly {
		q += ` AND ativo=true`
	}
	q += ` ORDER BY ano_academico, rotulo`
	rows, err := p.client.DB().Query(q, codigo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DocumentoExtraDTO{}
	for rows.Next() {
		d, err := p.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar documentos extra: %w", err)
	}
	return out, nil
}

// GetAtivosPorAnoAcademico retorna as definições ATIVAS desta academia que se
// aplicam a um ano_academico específico — usado no cadastro direto e na
// solicitação de matrícula para saber quais documentos extra validar/exigir.
func (p *DocumentoExtraProjection) GetAtivosPorAnoAcademico(codigo, anoAcademico string) ([]DocumentoExtraDTO, error) {
	rows, err := p.client.DB().Query(
		`SELECT `+documentoExtraCols+` FROM projection_documentos_extra WHERE codigo_academia=$1 AND ano_academico=$2 AND ativo=true ORDER BY rotulo`,
		codigo, anoAcademico)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DocumentoExtraDTO{}
	for rows.Next() {
		d, err := p.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar documentos extra por ano acadêmico: %w", err)
	}
	return out, nil
}
