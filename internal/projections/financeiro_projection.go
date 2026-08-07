package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"spuri/internal/db"
)

// FinanceiroProjection materializa apenas dados não sensíveis. O cofre
// financeiro_segredos_appypay não participa do rebuild: por design, segredos
// nunca são incluídos no ledger.
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

func (p *FinanceiroProjection) Handle(event db.Event) error {
	if event.AggregateType != "Financeiro" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("payload financeiro inválido: %w", err)
	}
	switch event.EventType {
	case "CredenciaisAppyPayConfiguradas":
		return p.credentials(event, payload)
	case "CobrancaFinanceiraSolicitada", "CobrancaFinanceiraCriada", "CobrancaFinanceiraStatusAtualizado":
		return p.charge(event, payload)
	case "WebhookFinanceiroRecebido":
		return p.webhook(event, payload)
	}
	return nil
}

func (p *FinanceiroProjection) Rebuild() error {
	// Não apagar credenciais: o cofre operacional de segredos possui FK para elas
	// e não pode ser reconstruído do ledger. Cobranças e eventos recebidos, sim.
	if _, err := p.client.DB().Exec(`DELETE FROM financeiro_webhooks_recebidos; DELETE FROM financeiro_cobrancas`); err != nil {
		return err
	}
	rows, err := p.client.DB().Query(`SELECT id,event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at,recorded_at,ledger_hash,previous_hash FROM spuri_ledger WHERE aggregate_type='Financeiro' ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var e db.Event
		var previous sql.NullString
		if err = rows.Scan(&e.ID, &e.EventID, &e.AggregateID, &e.AggregateType, &e.EventType, &e.EventVersion, &e.Payload, &e.Metadata, &e.OccurredAt, &e.RecordedAt, &e.LedgerHash, &previous); err != nil {
			return err
		}
		if previous.Valid {
			e.PreviousHash = &previous.String
		}
		if err = p.Handle(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (p *FinanceiroProjection) credentials(e db.Event, v map[string]any) error {
	id, err := uuid.Parse(stringOf(v, "credential_id"))
	if err != nil {
		return err
	}
	_, err = p.client.DB().Exec(`UPDATE financeiro_credenciais_appypay SET client_id_mascarado=$1,resource_mascarado=$2,gpo_method_mascarado=$3,ref_method_mascarado=$4,webhook_auth_type=NULLIF($5,''),webhook_secret_mascarado=NULLIF($6,''),last_event_id=$7,updated_at=CURRENT_TIMESTAMP WHERE id=$8`, stringOf(v, "client_id_mask"), stringOf(v, "resource_mask"), stringOf(v, "gpo_method_mask"), stringOf(v, "ref_method_mask"), stringOf(v, "webhook_auth_type"), stringOf(v, "webhook_secret_mask"), e.EventID, id)
	return err
}
func (p *FinanceiroProjection) charge(e db.Event, v map[string]any) error {
	id, err := uuid.Parse(stringOf(v, "charge_id"))
	if err != nil {
		return nil
	}
	status := stringOf(v, "status")
	if status == "" {
		status = "solicitada"
	}
	response, _ := json.Marshal(v["response"])
	_, err = p.client.DB().Exec(`UPDATE financeiro_cobrancas SET appypay_charge_id=COALESCE(NULLIF($1,''),appypay_charge_id),status=$2,response=CASE WHEN $3::jsonb='null'::jsonb THEN response ELSE $3::jsonb END,last_event_id=$4,updated_at=CURRENT_TIMESTAMP WHERE id=$5`, stringOf(v, "appypay_charge_id"), status, string(response), e.EventID, id)
	return err
}
func (p *FinanceiroProjection) webhook(e db.Event, v map[string]any) error {
	key := stringOf(v, "event_key")
	if key == "" {
		return nil
	}
	payload, _ := json.Marshal(v["payload"])
	_, err := p.client.DB().Exec(`INSERT INTO financeiro_webhooks_recebidos(event_key,metodo,credential_id,cobranca_id,payload) VALUES($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5) ON CONFLICT DO NOTHING`, key, stringOf(v, "metodo"), stringOf(v, "credential_id"), stringOf(v, "charge_id"), string(payload))
	return err
}
func stringOf(v map[string]any, k string) string {
	if value, ok := v[k]; ok && value != nil {
		return fmt.Sprint(value)
	}
	return ""
}
