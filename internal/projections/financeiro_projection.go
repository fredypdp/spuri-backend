package projections

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"spuri/internal/db"
	"spuri/internal/finance"
)

// FinanceiroProjection mantém os read models financeiro_* a partir do ledger.
type FinanceiroProjection struct{ client *db.Client }

func NewFinanceiroProjection(client *db.Client) *FinanceiroProjection {
	return &FinanceiroProjection{client: client}
}
func (p *FinanceiroProjection) Name() string { return "financeiro" }
func (p *FinanceiroProjection) GetLastProcessedEventID() (int64, error) {
	return NewBaseProjection(p.client).GetLastProcessedEventIDByName(p.Name())
}
func (p *FinanceiroProjection) UpdateCheckpoint(eventID int64) error {
	return NewBaseProjection(p.client).UpdateCheckpointByName(p.Name(), eventID)
}

func (p *FinanceiroProjection) Handle(event db.Event) error {
	if event.AggregateType != "Financeiro" {
		return nil
	}
	switch event.EventType {
	case "CredenciaisAppyPayCadastradas", "CredenciaisAppyPayAtualizadas", "CredenciaisAppyPayValidadas", "CredenciaisAppyPayAtivadas", "CredenciaisAppyPayDesativadas":
		return p.projectCredencial(event)
	case "ModalidadePagamentoGlobalAlterada", "ModalidadePagamentoSpuriAlterada", "ModalidadePagamentoAcademiaAlterada":
		return p.projectModalidade(event)
	case "CobrancaFinanceiraCriada", "CobrancaFinanceiraEnviadaAoProvider", "CobrancaFinanceiraStatusAtualizado", "CobrancaFinanceiraCancelada":
		return p.projectCobranca(event)
	case "WebhookFinanceiroRecebido", "WebhookFinanceiroIgnoradoComoDuplicado":
		return p.projectWebhook(event)
	case "ReembolsoFinanceiroSolicitado", "ReembolsoFinanceiroStatusAtualizado", "ReversaoFinanceiraSolicitada", "ReversaoFinanceiraStatusAtualizado", "DivergenciaFinanceiraDetectada", "DivergenciaFinanceiraReconciliada", "ReconciliacaoFinanceiraExecutada":
		return nil
	default:
		return fmt.Errorf("evento financeiro desconhecido: %s", event.EventType)
	}
}

func (p *FinanceiroProjection) Rebuild() error {
	log.Printf("[DEBUG] [financeiro] Rebuild iniciado — limpa apenas projeções financeiro_*, nunca o ledger")
	if _, err := p.client.DB().Exec(`
		DELETE FROM financeiro_webhooks_recebidos;
		DELETE FROM financeiro_cobrancas;
		DELETE FROM financeiro_credenciais_appypay;
		DELETE FROM financeiro_modalidade_pagamento;
	`); err != nil {
		return fmt.Errorf("falha ao limpar projeções financeiras: %w", err)
	}
	rows, err := p.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE aggregate_type = 'Financeiro'
		ORDER BY id ASC
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var event db.Event
		var prevHash sql.NullString
		if err := rows.Scan(&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType, &event.EventType, &event.EventVersion, &event.Payload, &event.Metadata, &event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash); err != nil {
			return err
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		if err := p.Handle(event); err != nil {
			return fmt.Errorf("erro no evento financeiro %d: %w", event.ID, err)
		}
		count++
	}
	log.Printf("[DEBUG] [financeiro] Rebuild concluído: %d eventos", count)
	return rows.Err()
}

func (p *FinanceiroProjection) projectCredencial(event db.Event) error {
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	c := finance.CredencialAppyPay{}
	var existing []byte
	if err := p.client.DB().QueryRow(`SELECT payload FROM financeiro_credenciais_appypay WHERE id=$1`, event.AggregateID).Scan(&existing); err == nil {
		_ = json.Unmarshal(existing, &c)
	}
	c.ID = event.AggregateID
	if c.CreatedAt.IsZero() {
		c.CreatedAt = event.OccurredAt
	}
	c.UpdatedAt = event.OccurredAt
	c.Version = event.EventVersion
	c.ContextoTipo = finance.ContextoTipo(str(payload, "contexto_tipo"))
	c.CodigoAcademia = str(payload, "codigo_academia")
	c.Ambiente = finance.Ambiente(str(payload, "ambiente"))
	c.AuthBaseURL = str(payload, "auth_base_url")
	c.APIBaseURL = str(payload, "api_base_url")
	c.WebAPIBaseURL = str(payload, "webapi_base_url")
	c.ClientID = str(payload, "client_id")
	c.Resource = str(payload, "resource")
	c.ClientSecretMask = str(payload, "client_secret_mask")
	c.WebhookSecretMask = str(payload, "webhook_secret_mask")
	c.Status = finance.StatusCredencial(str(payload, "status"))
	if c.Status == "" {
		c.Status = finance.StatusPendenteValidacao
	}
	c.Applications = projectionApps(payload["applications"])
	c.Historico = append(c.Historico, eventoFinanceiro(event, payload, "", c.CodigoAcademia))
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = p.client.DB().Exec(`INSERT INTO financeiro_credenciais_appypay (id, payload, updated_at) VALUES ($1, $2, CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload, updated_at=CURRENT_TIMESTAMP`, c.ID, raw)
	return err
}

func (p *FinanceiroProjection) projectCobranca(event db.Event) error {
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	c := finance.CobrancaFinanceira{}
	var existing []byte
	if err := p.client.DB().QueryRow(`SELECT payload FROM financeiro_cobrancas WHERE id=$1`, event.AggregateID).Scan(&existing); err == nil {
		_ = json.Unmarshal(existing, &c)
	}
	c.ID = event.AggregateID
	if c.CreatedAt.IsZero() {
		c.CreatedAt = event.OccurredAt
	}
	c.UpdatedAt = event.OccurredAt
	c.Version = event.EventVersion
	c.ContextoTipo = finance.ContextoTipo(str(payload, "contexto_tipo"))
	c.CodigoAcademia = str(payload, "codigo_academia")
	c.PagadorTipo = str(payload, "pagador_tipo")
	c.PagadorID = str(payload, "pagador_id")
	c.Moeda = str(payload, "moeda")
	c.MetodoPagamento = str(payload, "metodo_pagamento")
	c.Descricao = str(payload, "descricao")
	c.ReferenciaExterna = str(payload, "referencia_externa")
	c.MerchantTransactionID = str(payload, "merchant_transaction_id")
	c.ProviderChargeID = str(payload, "provider_charge_id")
	c.StatusProviderBruto = str(payload, "provider_status")
	c.Status = finance.StatusCobranca(str(payload, "status"))
	if v, ok := payload["valor"].(float64); ok {
		c.Valor = int64(v)
	}
	if c.Moeda == "" {
		c.Moeda = "AOA"
	}
	c.Historico = append(c.Historico, eventoFinanceiro(event, payload, "", c.CodigoAcademia))
	key := string(c.ContextoTipo) + ":" + c.CodigoAcademia + ":" + c.ReferenciaExterna
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = p.client.DB().Exec(`INSERT INTO financeiro_cobrancas (id, idempotency_key, payload, updated_at) VALUES ($1, $2, $3, CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key, payload=EXCLUDED.payload, updated_at=CURRENT_TIMESTAMP`, c.ID, key, raw)
	return err
}

func (p *FinanceiroProjection) projectModalidade(event db.Event) error {
	var current finance.ModalidadePagamento
	var raw []byte
	if err := p.client.DB().QueryRow(`SELECT payload FROM financeiro_modalidade_pagamento WHERE id='default'`).Scan(&raw); err == nil {
		_ = json.Unmarshal(raw, &current)
	}
	if current.Academias == nil {
		current = finance.ModalidadePagamento{GlobalAcademiasAtiva: true, SpuriAtiva: true, Academias: map[string]bool{}}
	}
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	ativa, _ := payload["ativa"].(bool)
	escopo := str(payload, "escopo")
	codigo := str(payload, "codigo_academia")
	if escopo == "global_academias" {
		current.GlobalAcademiasAtiva = ativa
	} else if escopo == "spuri" {
		current.SpuriAtiva = ativa
	} else if escopo == "academia" {
		current.Academias[codigo] = ativa
	}
	current.Historico = append(current.Historico, eventoFinanceiro(event, payload, escopo, codigo))
	out, err := json.Marshal(current)
	if err != nil {
		return err
	}
	_, err = p.client.DB().Exec(`INSERT INTO financeiro_modalidade_pagamento (id, payload, updated_at) VALUES ('default', $1, CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload, updated_at=CURRENT_TIMESTAMP`, out)
	return err
}

func (p *FinanceiroProjection) projectWebhook(event db.Event) error {
	var payload map[string]any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	eventID := str(payload, "event_id")
	if eventID == "" {
		eventID = event.EventID.String()
	}
	_, err := p.client.DB().Exec(`INSERT INTO financeiro_webhooks_recebidos (event_id, received_at) VALUES ($1, $2) ON CONFLICT (event_id) DO NOTHING`, eventID, event.OccurredAt)
	return err
}

func str(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}
func projectionApps(v any) []finance.Application {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]finance.Application, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, finance.Application{PaymentMethod: str(m, "paymentMethod"), ApplicationID: str(m, "applicationId"), APIKeyMask: str(m, "apiKey_mask"), WebhookURL: str(m, "webhook"), Metadata: stringMap(m["metadata"])})
	}
	return out
}
func stringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for k, val := range m {
		out[k] = fmt.Sprint(val)
	}
	return out
}

func eventoFinanceiro(event db.Event, payload map[string]any, escopo, codigo string) finance.EventoFinanceiro {
	metadata := map[string]any{}
	_ = json.Unmarshal(event.Metadata, &metadata)
	e := finance.EventoFinanceiro{Tipo: event.EventType, At: event.OccurredAt, Escopo: escopo, CodigoAcademia: codigo, Metadata: payload}
	e.AutorID = str(metadata, "user_id")
	e.AutorTipo = str(metadata, "user_type")
	e.Motivo = str(payload, "motivo")
	if e.Motivo == "" {
		e.Motivo = str(metadata, "motivo")
	}
	e.ContextoTipo = str(payload, "contexto_tipo")
	if e.ContextoTipo == "" && codigo != "" {
		e.ContextoTipo = string(finance.ContextoAcademia)
	}
	return e
}
