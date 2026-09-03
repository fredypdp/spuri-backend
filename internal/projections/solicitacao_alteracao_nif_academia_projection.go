package projections

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
)

type SolicitacaoAlteracaoNIFAcademiaProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewSolicitacaoAlteracaoNIFAcademiaProjection(client *db.Client) *SolicitacaoAlteracaoNIFAcademiaProjection {
	return &SolicitacaoAlteracaoNIFAcademiaProjection{client: client, ctx: context.Background()}
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) Name() string {
	return "solicitacoes_alteracao_nif_academia"
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) GetLastProcessedEventID() (int64, error) {
	var id int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) UpdateCheckpoint(id int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES ($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT (projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE projection_solicitacoes_alteracao_nif_academia`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at FROM spuri_ledger WHERE aggregate_type='SolicitacaoAlteracaoNIFAcademia' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ev db.Event
		if err := rows.Scan(&ev.ID, &ev.EventID, &ev.AggregateID, &ev.AggregateType, &ev.EventType, &ev.EventVersion, &ev.Payload, &ev.Metadata, &ev.OccurredAt, &ev.RecordedAt); err != nil {
			return err
		}
		if err := p.Handle(ev); err != nil {
			return err
		}
		_ = p.UpdateCheckpoint(ev.ID)
	}
	return rows.Err()
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) Handle(event db.Event) error {
	if event.AggregateType != "SolicitacaoAlteracaoNIFAcademia" {
		return nil
	}
	switch event.EventType {
	case "SolicitacaoAlteracaoNIFAcademiaCriada":
		return p.handleCriada(event)
	case "SolicitacaoAlteracaoNIFAcademiaAprovada":
		return p.handleAprovada(event)
	case "SolicitacaoAlteracaoNIFAcademiaReprovada":
		return p.handleReprovada(event)
	}
	return nil
}

type SolicitacaoAlteracaoNIFAcademiaDTO struct {
	ID                uuid.UUID `json:"id"`
	CodigoSolicitacao string    `json:"codigo_solicitacao"`
	CodigoAcademia    string    `json:"codigo_academia"`
	NIFAtual          string    `json:"nif_atual"`
	NIFSolicitado     string    `json:"nif_solicitado"`
	Status            string    `json:"status"`
	MotivoReprovacao  *string   `json:"motivo_reprovacao,omitempty"`
	SolicitadoPor     string    `json:"solicitado_por"`
	DecididoPor       *string   `json:"decidido_por,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Version           int       `json:"version"`
}

func (p *SolicitacaoAlteracaoNIFAcademiaProjection) handleCriada(event db.Event) error {
	var x aggregates.SolicitacaoAlteracaoNIFAcademiaCriadaEvent
	if err := json.Unmarshal(event.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`INSERT INTO projection_solicitacoes_alteracao_nif_academia (id,codigo_solicitacao,codigo_academia,nif_atual,nif_solicitado,status,solicitado_por,created_at,updated_at,version,last_event_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, updated_at=EXCLUDED.updated_at, version=EXCLUDED.version,last_event_id=EXCLUDED.last_event_id`, event.AggregateID, x.CodigoSolicitacao, x.CodigoAcademia, x.NIFAtual, x.NIFSolicitado, x.Status, x.SolicitadoPor, x.CreatedAt, x.UpdatedAt, event.EventVersion, event.ID)
	return err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) handleAprovada(event db.Event) error {
	var x aggregates.SolicitacaoAlteracaoNIFAcademiaAprovadaEvent
	_ = json.Unmarshal(event.Payload, &x)
	_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_alteracao_nif_academia SET status=$1, decidido_por=$2, updated_at=$3, version=$4,last_event_id=$5 WHERE id=$6`, aggregates.StatusSolicitacaoAprovada, x.DecididoPor, x.UpdatedAt, event.EventVersion, event.ID, event.AggregateID)
	return err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) handleReprovada(event db.Event) error {
	var x aggregates.SolicitacaoAlteracaoNIFAcademiaReprovadaEvent
	_ = json.Unmarshal(event.Payload, &x)
	_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_alteracao_nif_academia SET status=$1, motivo_reprovacao=$2, decidido_por=$3, updated_at=$4, version=$5,last_event_id=$6 WHERE id=$7`, aggregates.StatusSolicitacaoReprovada, x.MotivoReprovacao, x.DecididoPor, x.UpdatedAt, event.EventVersion, event.ID, event.AggregateID)
	return err
}

// ExistePendente reporta se a academia já tem uma solicitação de alteração
// de NIF pendente. O handler de criação consulta isto para devolver um erro
// amigável antes de tentar gravar — o índice único parcial
// idx_solicitacoes_alteracao_nif_academia_pendente (migration 117) é quem
// garante a regra de fato sob concorrência.
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) ExistePendente(codigoAcademia string) (bool, error) {
	var b bool
	err := p.client.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM projection_solicitacoes_alteracao_nif_academia WHERE codigo_academia=$1 AND status='pendente')`, codigoAcademia).Scan(&b)
	return b, err
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) GetByCodigo(codigo string) (*SolicitacaoAlteracaoNIFAcademiaDTO, error) {
	rows, err := p.client.DB().Query(`SELECT id,codigo_solicitacao,codigo_academia,nif_atual,nif_solicitado,status,motivo_reprovacao,solicitado_por,decidido_por,created_at,updated_at,version FROM projection_solicitacoes_alteracao_nif_academia WHERE codigo_solicitacao=$1`, codigo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		return scanSolAlteracaoNIF(rows)
	}
	return nil, sql.ErrNoRows
}
func (p *SolicitacaoAlteracaoNIFAcademiaProjection) List(status, codigoAcademia string, limit, offset int) ([]SolicitacaoAlteracaoNIFAcademiaDTO, error) {
	wh := []string{"1=1"}
	args := []interface{}{}
	add := func(cond, v string) {
		if strings.TrimSpace(v) != "" {
			args = append(args, v)
			wh = append(wh, fmt.Sprintf(cond, len(args)))
		}
	}
	add("status=$%d", status)
	add("codigo_academia=$%d", codigoAcademia)
	args = append(args, limit, offset)
	q := fmt.Sprintf(`SELECT id,codigo_solicitacao,codigo_academia,nif_atual,nif_solicitado,status,motivo_reprovacao,solicitado_por,decidido_por,created_at,updated_at,version FROM projection_solicitacoes_alteracao_nif_academia WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, strings.Join(wh, " AND "), len(args)-1, len(args))
	rows, err := p.client.DB().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SolicitacaoAlteracaoNIFAcademiaDTO
	for rows.Next() {
		d, err := scanSolAlteracaoNIF(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}
func scanSolAlteracaoNIF(r interface{ Scan(...interface{}) error }) (*SolicitacaoAlteracaoNIFAcademiaDTO, error) {
	var d SolicitacaoAlteracaoNIFAcademiaDTO
	var mot, dec sql.NullString
	err := r.Scan(&d.ID, &d.CodigoSolicitacao, &d.CodigoAcademia, &d.NIFAtual, &d.NIFSolicitado, &d.Status, &mot, &d.SolicitadoPor, &dec, &d.CreatedAt, &d.UpdatedAt, &d.Version)
	if mot.Valid {
		d.MotivoReprovacao = &mot.String
	}
	if dec.Valid {
		d.DecididoPor = &dec.String
	}
	return &d, err
}
