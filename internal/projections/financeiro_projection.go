package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"spuri/internal/db"
)

// FinanceiroProjection is the sole writer of financial read models. Its event
// payloads are deliberately secret-free, allowing deterministic replay.
type FinanceiroProjection struct{ client *db.Client }

func NewFinanceiroProjection(client *db.Client) *FinanceiroProjection {
	return &FinanceiroProjection{client: client}
}
func (p *FinanceiroProjection) Name() string { return "financeiro" }
func (p *FinanceiroProjection) GetLastProcessedEventID() (int64, error) {
	return NewBaseProjection(p.client).GetLastProcessedEventIDByName(p.Name())
}
func (p *FinanceiroProjection) UpdateCheckpoint(id int64) error {
	return NewBaseProjection(p.client).UpdateCheckpointByName(p.Name(), id)
}

func (p *FinanceiroProjection) Handle(e db.Event) error {
	if e.AggregateType != "Financeiro" {
		return nil
	}
	var v map[string]any
	if err := json.Unmarshal(e.Payload, &v); err != nil {
		return err
	}
	switch e.EventType {
	case "MensalidadeConfigurada":
		var in struct {
			CodigoAcademia string  `json:"codigo_academia"`
			Nivel          string  `json:"nivel"`
			AnoAcademico   string  `json:"ano_academico"`
			CursoID        *string `json:"curso_id"`
			Valor          float64 `json:"valor"`
			MesFimCobranca int     `json:"mes_fim_cobranca"`
		}
		if err := json.Unmarshal(e.Payload, &in); err != nil {
			return err
		}
		if in.CodigoAcademia == "" || in.Nivel == "" || in.AnoAcademico == "" || in.Valor <= 0 {
			return fmt.Errorf("evento MensalidadeConfigurada invÃ¡lido")
		}
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes (event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,vigente_em) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'' )::uuid,$7,$8,$9) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.Nivel, in.AnoAcademico, stringValue(in.CursoID), in.Valor, in.MesFimCobranca, e.OccurredAt)
		return err
	case "MesInicioCobrancaDefinido":
		var in struct {
			CodigoAcademia string `json:"codigo_academia"`
			AnoLetivo      string `json:"ano_letivo"`
			MesInicio      int    `json:"mes_inicio"`
		}
		if err := json.Unmarshal(e.Payload, &in); err != nil {
			return err
		}
		if in.CodigoAcademia == "" || in.AnoLetivo == "" || in.MesInicio < 1 || in.MesInicio > 12 {
			return fmt.Errorf("evento MesInicioCobrancaDefinido invÃ¡lido")
		}
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_inicio_cobranca (event_id,aggregate_id,codigo_academia,ano_letivo,mes_inicio,definido_em) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.AnoLetivo, in.MesInicio, e.OccurredAt)
		return err
	case "ObrigacaoMensalidadeAnulada", "ObrigacaoMensalidadeReativada", "MensalidadePaga":
		var in struct {
			CodigoEstudante string `json:"codigo_estudante"`
			CodigoAcademia  string `json:"codigo_academia"`
			AnoLetivo       string `json:"ano_letivo"`
			Mes             int    `json:"mes"`
			Motivo          string `json:"motivo"`
		}
		if err := json.Unmarshal(e.Payload, &in); err != nil {
			return err
		}
		if in.CodigoEstudante == "" || in.CodigoAcademia == "" || in.AnoLetivo == "" || in.Mes < 1 || in.Mes > 12 {
			return fmt.Errorf("evento de obrigaÃ§Ã£o mensal invÃ¡lido")
		}
		tipo := map[string]string{"ObrigacaoMensalidadeAnulada": "anulada", "ObrigacaoMensalidadeReativada": "reativada", "MensalidadePaga": "paga"}[e.EventType]
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,motivo,ocorrido_em) VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoEstudante, in.CodigoAcademia, in.AnoLetivo, in.Mes, tipo, in.Motivo, e.OccurredAt)
		return err
	case "CredenciaisAppyPayConfiguradas":
		contexto, _ := v["contexto_tipo"].(string)
		academia, _ := v["codigo_academia"].(string)
		ambiente, _ := v["ambiente"].(string)
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_credenciais_appypay (id,contexto_tipo,codigo_academia,ambiente,payload,updated_at) VALUES ($1,$2,NULLIF($3,''),$4,$5,CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET contexto_tipo=EXCLUDED.contexto_tipo,codigo_academia=EXCLUDED.codigo_academia,ambiente=EXCLUDED.ambiente,payload=EXCLUDED.payload,updated_at=CURRENT_TIMESTAMP`, e.AggregateID, contexto, academia, ambiente, e.Payload)
		return err
	case "CobrancaAppyPaySolicitada", "CobrancaAppyPayCriada", "CobrancaAppyPayFalhou", "CobrancaAppyPayConsultada", "CobrancaAppyPayCancelada", "CobrancaAppyPayConflitoPosCancelamento", "QRCodeAppyPaySolicitado", "QRCodeAppyPayGerado", "QRCodeAppyPayFalhou":
		merchant, _ := v["merchant_transaction_id"].(string)
		provider, _ := v["provider_charge_id"].(string)
		contexto, _ := v["contexto_tipo"].(string)
		academia, _ := v["codigo_academia"].(string)
		if merchant == "" {
			return fmt.Errorf("evento financeiro sem merchant_transaction_id")
		}
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,provider_charge_id,merchant_transaction_id,contexto_tipo,codigo_academia,payload,updated_at) VALUES ($1,NULLIF($2,''),$3,$4,NULLIF($5,''),$6,CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET provider_charge_id=COALESCE(EXCLUDED.provider_charge_id,financeiro_cobrancas.provider_charge_id),payload=EXCLUDED.payload,updated_at=CURRENT_TIMESTAMP`, e.AggregateID, provider, merchant, contexto, academia, e.Payload)
		return err
	case "WebhookAppyPayRecebido":
		id, _ := v["event_id"].(string)
		metodo, _ := v["metodo"].(string)
		if id == "" || (metodo != "GPO" && metodo != "REF") {
			return fmt.Errorf("webhook financeiro inválido")
		}
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_webhooks_recebidos (event_id,metodo,received_at) VALUES ($1,$2,$3) ON CONFLICT (event_id) DO NOTHING`, id, metodo, e.OccurredAt)
		return err
	default:
		return fmt.Errorf("evento financeiro desconhecido: %s", e.EventType)
	}
}

func (p *FinanceiroProjection) Rebuild() error {
	// Secrets are operational material and intentionally survive a ledger replay.
	if _, err := p.client.DB().Exec(`DELETE FROM financeiro_mensalidade_obrigacoes_eventos; DELETE FROM financeiro_mensalidade_inicio_cobranca; DELETE FROM financeiro_mensalidade_configuracoes; DELETE FROM financeiro_webhooks_recebidos; DELETE FROM financeiro_cobrancas; DELETE FROM financeiro_credenciais_appypay;`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='Financeiro' ORDER BY id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e db.Event
		var previous sql.NullString
		if err := rows.Scan(&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.EventVersion, &e.Payload, &e.Metadata, &e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &previous); err != nil {
			return err
		}
		if previous.Valid {
			e.PreviousHash = &previous.String
		}
		if err := p.Handle(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// ApplyNow is used immediately after appending a ledger event. It invokes the
// same projection implementation as the asynchronous manager, never a second
// write path, while preserving a responsive API for newly configured accounts.
func (p *FinanceiroProjection) ApplyNow(id uuid.UUID) error {
	var e db.Event
	var previous sql.NullString
	err := p.client.DB().QueryRow(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE event_id=$1`, id).Scan(&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.EventVersion, &e.Payload, &e.Metadata, &e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &previous)
	if err != nil {
		return err
	}
	if previous.Valid {
		e.PreviousHash = &previous.String
	}
	return p.Handle(e)
}

func (p *FinanceiroProjection) ApplyLatestForAggregate(aggregateID uuid.UUID) error {
	var eventID uuid.UUID
	if err := p.client.DB().QueryRow(`SELECT event_id FROM spuri_ledger WHERE aggregate_id=$1 AND aggregate_type='Financeiro' ORDER BY id DESC LIMIT 1`, aggregateID).Scan(&eventID); err != nil {
		return err
	}
	return p.ApplyNow(eventID)
}
