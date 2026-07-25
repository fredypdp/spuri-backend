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

type SolicitacaoEdicaoDadoEstudanteProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewSolicitacaoEdicaoDadoEstudanteProjection(client *db.Client) *SolicitacaoEdicaoDadoEstudanteProjection {
	return &SolicitacaoEdicaoDadoEstudanteProjection{client: client, ctx: context.Background()}
}
func (p *SolicitacaoEdicaoDadoEstudanteProjection) Name() string {
	return "solicitacoes_edicao_dados_estudante"
}
func (p *SolicitacaoEdicaoDadoEstudanteProjection) GetLastProcessedEventID() (int64, error) {
	var id int64
	err := p.client.DB().QueryRow(`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name=$1`, p.Name()).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return id, err
}
func (p *SolicitacaoEdicaoDadoEstudanteProjection) UpdateCheckpoint(id int64) error {
	_, err := p.client.DB().Exec(`INSERT INTO projection_checkpoints (projection_name,last_processed_event_id,last_processed_at,events_processed) VALUES ($1,$2,CURRENT_TIMESTAMP,1) ON CONFLICT (projection_name) DO UPDATE SET last_processed_event_id=$2,last_processed_at=CURRENT_TIMESTAMP,events_processed=projection_checkpoints.events_processed+1`, p.Name(), id)
	return err
}
func (p *SolicitacaoEdicaoDadoEstudanteProjection) Rebuild() error {
	if _, err := p.client.DB().Exec(`TRUNCATE projection_solicitacoes_edicao_dados_estudante`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at FROM spuri_ledger WHERE aggregate_type='SolicitacaoEdicaoDadoEstudante' ORDER BY id`)
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
func (p *SolicitacaoEdicaoDadoEstudanteProjection) Handle(event db.Event) error {
	if event.AggregateType != "SolicitacaoEdicaoDadoEstudante" {
		return nil
	}
	switch event.EventType {
	case "SolicitacaoEdicaoDadoEstudanteCriada":
		return p.handleCriada(event)
	case "SolicitacaoEdicaoDadoEstudanteAprovada":
		return p.handleAprovada(event)
	case "SolicitacaoEdicaoDadoEstudanteReprovada":
		return p.handleReprovada(event)
	}
	return nil
}

type SolicitacaoEdicaoDadoEstudanteDTO struct {
	ID                      uuid.UUID                     `json:"id"`
	CodigoSolicitacao       string                        `json:"codigo_solicitacao"`
	CodigoEstudante         string                        `json:"codigo_estudante"`
	CodigoAcademia          string                        `json:"codigo_academia"`
	Campo                   string                        `json:"campo"`
	ValorAtual              string                        `json:"valor_atual"`
	ValorSolicitado         string                        `json:"valor_solicitado"`
	DocumentoTemporarioPath string                        `json:"documento_temporario_path"`
	DocumentoTemporarioURL  string                        `json:"documento_temporario_url,omitempty"`
	Documento               aggregates.DocumentoMatricula `json:"documento,omitempty"`
	Status                  string                        `json:"status"`
	MotivoReprovacao        *string                       `json:"motivo_reprovacao,omitempty"`
	SolicitadoPor           string                        `json:"solicitado_por"`
	DecididoPor             *string                       `json:"decidido_por,omitempty"`
	CreatedAt               time.Time                     `json:"created_at"`
	UpdatedAt               time.Time                     `json:"updated_at"`
	Version                 int                           `json:"version"`
}

func (d SolicitacaoEdicaoDadoEstudanteDTO) GetCodigoSolicitacao() string {
	return d.CodigoSolicitacao
}

func (d SolicitacaoEdicaoDadoEstudanteDTO) GetDocumentoTemporarioPath() string {
	return d.DocumentoTemporarioPath
}

func (d SolicitacaoEdicaoDadoEstudanteDTO) GetDocumentoTemporarioURL() string {
	return d.DocumentoTemporarioURL
}

func (p *SolicitacaoEdicaoDadoEstudanteProjection) handleCriada(event db.Event) error {
	var x aggregates.SolicitacaoEdicaoDadoEstudanteCriadaEvent
	if err := json.Unmarshal(event.Payload, &x); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`INSERT INTO projection_solicitacoes_edicao_dados_estudante (id,codigo_solicitacao,codigo_estudante,codigo_academia,campo,valor_atual,valor_solicitado,documento_temporario_path,documento_temporario_url,status,solicitado_por,created_at,updated_at,version,last_event_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, updated_at=EXCLUDED.updated_at, version=EXCLUDED.version,last_event_id=EXCLUDED.last_event_id`, event.AggregateID, x.CodigoSolicitacao, x.CodigoEstudante, x.CodigoAcademia, x.Campo, x.ValorAtual, x.ValorSolicitado, x.DocumentoTemporarioPath, x.DocumentoTemporarioURL, x.Status, x.SolicitadoPor, x.CreatedAt, x.UpdatedAt, event.EventVersion, event.ID)
	return err
}
func (p *SolicitacaoEdicaoDadoEstudanteProjection) handleAprovada(event db.Event) error {
	var x aggregates.SolicitacaoEdicaoDadoEstudanteAprovadaEvent
	_ = json.Unmarshal(event.Payload, &x)
	_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_edicao_dados_estudante SET status=$1, decidido_por=$2, updated_at=$3, version=$4,last_event_id=$5 WHERE id=$6`, aggregates.StatusSolicitacaoAprovada, x.DecididoPor, x.UpdatedAt, event.EventVersion, event.ID, event.AggregateID)
	return err
}
func (p *SolicitacaoEdicaoDadoEstudanteProjection) handleReprovada(event db.Event) error {
	var x aggregates.SolicitacaoEdicaoDadoEstudanteReprovadaEvent
	_ = json.Unmarshal(event.Payload, &x)
	_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_edicao_dados_estudante SET status=$1, motivo_reprovacao=$2, decidido_por=$3, updated_at=$4, version=$5,last_event_id=$6 WHERE id=$7`, aggregates.StatusSolicitacaoReprovada, x.MotivoReprovacao, x.DecididoPor, x.UpdatedAt, event.EventVersion, event.ID, event.AggregateID)
	return err
}
func (p *SolicitacaoEdicaoDadoEstudanteProjection) ExistePendente(codigoEstudante, campo string) (bool, error) {
	var b bool
	err := p.client.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM projection_solicitacoes_edicao_dados_estudante WHERE codigo_estudante=$1 AND campo=$2 AND status='pendente')`, codigoEstudante, campo).Scan(&b)
	return b, err
}
func (p *SolicitacaoEdicaoDadoEstudanteProjection) GetByCodigo(codigo string) (*SolicitacaoEdicaoDadoEstudanteDTO, error) {
	rows, err := p.client.DB().Query(`SELECT id,codigo_solicitacao,codigo_estudante,codigo_academia,campo,valor_atual,valor_solicitado,documento_temporario_path,documento_temporario_url,status,motivo_reprovacao,solicitado_por,decidido_por,created_at,updated_at,version FROM projection_solicitacoes_edicao_dados_estudante WHERE codigo_solicitacao=$1`, codigo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if rows.Next() {
		return scanSolEdicao(rows)
	}
	return nil, sql.ErrNoRows
}
func (p *SolicitacaoEdicaoDadoEstudanteProjection) List(status, campo, codigoEstudante, codigoAcademia string, limit, offset int) ([]SolicitacaoEdicaoDadoEstudanteDTO, error) {
	wh := []string{"1=1"}
	args := []interface{}{}
	add := func(cond, v string) {
		if strings.TrimSpace(v) != "" {
			args = append(args, v)
			wh = append(wh, fmt.Sprintf(cond, len(args)))
		}
	}
	add("status=$%d", status)
	add("campo=$%d", campo)
	add("codigo_estudante=$%d", codigoEstudante)
	add("codigo_academia=$%d", codigoAcademia)
	args = append(args, limit, offset)
	q := fmt.Sprintf(`SELECT id,codigo_solicitacao,codigo_estudante,codigo_academia,campo,valor_atual,valor_solicitado,documento_temporario_path,documento_temporario_url,status,motivo_reprovacao,solicitado_por,decidido_por,created_at,updated_at,version FROM projection_solicitacoes_edicao_dados_estudante WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, strings.Join(wh, " AND "), len(args)-1, len(args))
	rows, err := p.client.DB().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SolicitacaoEdicaoDadoEstudanteDTO
	for rows.Next() {
		d, err := scanSolEdicao(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}
func scanSolEdicao(r interface{ Scan(...interface{}) error }) (*SolicitacaoEdicaoDadoEstudanteDTO, error) {
	var d SolicitacaoEdicaoDadoEstudanteDTO
	var url, mot, dec sql.NullString
	err := r.Scan(&d.ID, &d.CodigoSolicitacao, &d.CodigoEstudante, &d.CodigoAcademia, &d.Campo, &d.ValorAtual, &d.ValorSolicitado, &d.DocumentoTemporarioPath, &url, &d.Status, &mot, &d.SolicitadoPor, &dec, &d.CreatedAt, &d.UpdatedAt, &d.Version)
	if url.Valid {
		d.DocumentoTemporarioURL = url.String
	}
	if mot.Valid {
		d.MotivoReprovacao = &mot.String
	}
	if dec.Valid {
		d.DecididoPor = &dec.String
	}
	return &d, err
}
