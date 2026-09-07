package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

type CategoriaServicoProjection struct{ client *db.Client }

func NewCategoriaServicoProjection(c *db.Client) *CategoriaServicoProjection {
	return &CategoriaServicoProjection{c}
}
func (p *CategoriaServicoProjection) Name() string { return "categorias_servico" }
func (p *CategoriaServicoProjection) GetLastProcessedEventID() (int64, error) {
	var v int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&v)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return v, err
}
func (p *CategoriaServicoProjection) UpdateCheckpoint(id int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT(projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return err
}
func (p *CategoriaServicoProjection) Handle(e db.Event) error {
	if e.AggregateType != "CategoriaServico" {
		return nil
	}
	switch e.EventType {
	case "CategoriaServicoCriada":
		return p.created(e)
	case "CategoriaServicoRenomeada":
		return p.renamed(e)
	case "CategoriaServicoDesativada":
		return p.active(e, false)
	case "CategoriaServicoReativada":
		return p.active(e, true)
	}
	return nil
}
func (p *CategoriaServicoProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE projection_categorias_servico CASCADE`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='CategoriaServico' ORDER BY id`)
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

type CategoriaServicoDTO struct {
	ID             uuid.UUID `json:"id"`
	CodigoAcademia string    `json:"codigo_academia"`
	Nome           string    `json:"nome"`
	Ativo          bool      `json:"ativo"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (p *CategoriaServicoProjection) created(e db.Event) error {
	var x struct {
		CodigoAcademia string
		Nome           string
		CriadoPor      uuid.UUID
		CreatedAt      time.Time
	}
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`INSERT INTO projection_categorias_servico(id,codigo_academia,nome,ativo,created_at,updated_at,version,last_event_id) VALUES($1,$2,$3,true,$4,$4,$5,$6) ON CONFLICT(id) DO NOTHING`, e.AggregateID, x.CodigoAcademia, x.Nome, x.CreatedAt, e.EventVersion, e.EventID)
	return err
}
func (p *CategoriaServicoProjection) renamed(e db.Event) error {
	var x struct{ Nome string }
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`UPDATE projection_categorias_servico SET nome=$1,version=$2,last_event_id=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$4`, x.Nome, e.EventVersion, e.EventID, e.AggregateID)
	return err
}
func (p *CategoriaServicoProjection) active(e db.Event, ativo bool) error {
	_, err := p.client.DB().Exec(`UPDATE projection_categorias_servico SET ativo=$1,version=$2,last_event_id=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$4`, ativo, e.EventVersion, e.EventID, e.AggregateID)
	return err
}
func (p *CategoriaServicoProjection) scan(row interface{ Scan(...interface{}) error }) (*CategoriaServicoDTO, error) {
	var d CategoriaServicoDTO
	err := row.Scan(&d.ID, &d.CodigoAcademia, &d.Nome, &d.Ativo, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

const categoriaServicoCols = `id,codigo_academia,nome,ativo,created_at,updated_at`

func (p *CategoriaServicoProjection) GetByID(id uuid.UUID) (*CategoriaServicoDTO, error) {
	d, err := p.scan(p.client.DB().QueryRow(`SELECT `+categoriaServicoCols+` FROM projection_categorias_servico WHERE id=$1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}
func (p *CategoriaServicoProjection) GetByAcademia(codigo string, ativosOnly bool) ([]CategoriaServicoDTO, error) {
	q := `SELECT ` + categoriaServicoCols + ` FROM projection_categorias_servico WHERE codigo_academia=$1`
	if ativosOnly {
		q += ` AND ativo=true`
	}
	q += ` ORDER BY nome`
	rows, err := p.client.DB().Query(q, codigo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CategoriaServicoDTO{}
	for rows.Next() {
		d, err := p.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar categorias de serviço: %w", err)
	}
	return out, nil
}
