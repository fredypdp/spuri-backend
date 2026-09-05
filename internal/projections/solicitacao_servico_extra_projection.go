package projections

import (
	"database/sql"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/db"
	"time"
)

type SolicitacaoServicoExtraProjection struct{ client *db.Client }

func NewSolicitacaoServicoExtraProjection(c *db.Client) *SolicitacaoServicoExtraProjection {
	return &SolicitacaoServicoExtraProjection{c}
}
func (p *SolicitacaoServicoExtraProjection) Name() string { return "solicitacoes_servico_extra" }
func (p *SolicitacaoServicoExtraProjection) GetLastProcessedEventID() (int64, error) {
	return NewBaseProjection(p.client).GetLastProcessedEventIDByName(p.Name())
}
func (p *SolicitacaoServicoExtraProjection) UpdateCheckpoint(id int64) error {
	return NewBaseProjection(p.client).UpdateCheckpointByName(p.Name(), id)
}
func (p *SolicitacaoServicoExtraProjection) Rebuild() error { return nil }
func (p *SolicitacaoServicoExtraProjection) Handle(e db.Event) error {
	if e.AggregateType != "SolicitacaoServicoExtra" {
		return nil
	}
	var x map[string]json.RawMessage
	if err := json.Unmarshal(e.Payload, &x); err != nil {
		return err
	}
	switch e.EventType {
	case "SolicitacaoServicoExtraCriada":
		var v struct {
			ServicoExtraID                                               uuid.UUID
			CodigoAcademia, CodigoEstudante, DocumentoPath, DocumentoURL string
			CreatedAt                                                    time.Time
		}
		json.Unmarshal(e.Payload, &v)
		_, err := p.client.DB().Exec(`INSERT INTO projection_solicitacoes_servico_extra(id,servico_extra_id,codigo_academia,codigo_estudante,status,documento_path,documento_url,created_at,updated_at,version,last_event_id) VALUES($1,$2,$3,$4,'pendente',NULLIF($5,''),NULLIF($6,''),$7,$7,$8,$9) ON CONFLICT(id) DO NOTHING`, e.AggregateID, v.ServicoExtraID, v.CodigoAcademia, v.CodigoEstudante, v.DocumentoPath, v.DocumentoURL, v.CreatedAt, e.EventVersion, e.EventID)
		return err
	case "SolicitacaoServicoExtraAprovadaPendentePagamento":
		var v struct {
			ValorTaxaInscricao            float64
			MetodosPagamentoTaxaInscricao []string
			AprovadaPor                   uuid.UUID
			UpdatedAt                     time.Time
		}
		json.Unmarshal(e.Payload, &v)
		_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_servico_extra SET status='aprovada_pendente_pagamento_taxa_inscricao',valor_taxa_inscricao=$1,metodos_pagamento_taxa_inscricao=$2,aprovada_por=$3,updated_at=$4,version=$5,last_event_id=$6 WHERE id=$7`, v.ValorTaxaInscricao, pq.Array(v.MetodosPagamentoTaxaInscricao), v.AprovadaPor, v.UpdatedAt, e.EventVersion, e.EventID, e.AggregateID)
		return err
	case "SolicitacaoServicoExtraVinculada":
		var v struct {
			AprovadaPor uuid.UUID
			UpdatedAt   time.Time
		}
		json.Unmarshal(e.Payload, &v)
		_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_servico_extra SET status='vinculada',aprovada_por=COALESCE(NULLIF($1::uuid,'00000000-0000-0000-0000-000000000000'),aprovada_por),vinculada_em=COALESCE(vinculada_em,$2),updated_at=$2,version=$3,last_event_id=$4 WHERE id=$5`, v.AprovadaPor, v.UpdatedAt, e.EventVersion, e.EventID, e.AggregateID)
		return err
	case "SolicitacaoServicoExtraReprovada":
		return p.end(e, "reprovada", "motivo_reprovacao", "reprovada_por")
	case "SolicitacaoServicoExtraCanceladaAntesDaVinculacao":
		return p.end(e, "cancelada_antes_da_vinculacao", "motivo_cancelamento", "cancelada_por")
	case "SolicitacaoServicoExtraCancelada":
		return p.end(e, "cancelada", "motivo_cancelamento", "cancelada_por")
	}
	return nil
}
func (p *SolicitacaoServicoExtraProjection) end(e db.Event, status, col, actor string) error {
	var v struct {
		MotivoReprovacao, MotivoCancelamento, CanceladaPor string
		ReprovadaPor                                       uuid.UUID
		UpdatedAt                                          time.Time
	}
	json.Unmarshal(e.Payload, &v)
	value := v.MotivoReprovacao
	if value == "" {
		value = v.MotivoCancelamento
	}
	var err error
	if actor == "reprovada_por" {
		_, err = p.client.DB().Exec(`UPDATE projection_solicitacoes_servico_extra SET status=$1,motivo_reprovacao=$2,reprovada_por=$3,updated_at=$4,version=$5,last_event_id=$6 WHERE id=$7`, status, value, v.ReprovadaPor, v.UpdatedAt, e.EventVersion, e.EventID, e.AggregateID)
	} else {
		_, err = p.client.DB().Exec(`UPDATE projection_solicitacoes_servico_extra SET status=$1,motivo_cancelamento=$2,cancelada_por=$3,updated_at=$4,version=$5,last_event_id=$6 WHERE id=$7`, status, value, v.CanceladaPor, v.UpdatedAt, e.EventVersion, e.EventID, e.AggregateID)
	}
	return err
}
func (p *SolicitacaoServicoExtraProjection) ExisteAtiva(id uuid.UUID, codigo string) (bool, error) {
	var ok bool
	err := p.client.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM projection_solicitacoes_servico_extra WHERE servico_extra_id=$1 AND codigo_estudante=$2 AND status IN ('pendente','aprovada_pendente_pagamento_taxa_inscricao','vinculada'))`, id, codigo).Scan(&ok)
	return ok, err
}
func (p *SolicitacaoServicoExtraProjection) unused() sql.NullString { return sql.NullString{} }
