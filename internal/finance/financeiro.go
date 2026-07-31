package finance

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
)

const AppyPayAPIBaseURLEnv = "APPYPAY_API_BASE_URL"

type ContextoTipo string

const (
	ContextoSpuri    ContextoTipo = "spuri"
	ContextoAcademia ContextoTipo = "academia"
)

type Ambiente string

const (
	AmbienteTeste Ambiente = "test"
	AmbienteProd  Ambiente = "prod"
)

type StatusCredencial string

const (
	StatusAtivo             StatusCredencial = "ativo"
	StatusInativo           StatusCredencial = "inativo"
	StatusPendenteValidacao StatusCredencial = "pendente_validacao"
	StatusErroValidacao     StatusCredencial = "erro_validacao"
)

type StatusCobranca string

const (
	CobrancaPendente  StatusCobranca = "pendente"
	CobrancaEnviada   StatusCobranca = "enviada_provider"
	CobrancaLiquidada StatusCobranca = "liquidada"
	CobrancaCancelada StatusCobranca = "cancelada"
	CobrancaFalhada   StatusCobranca = "falhada"
)

type Application struct {
	PaymentMethod   string
	ApplicationID   string
	APIKey          string `json:"-"`
	APIKeyEncrypted string `json:"-"`
	APIKeyMask      string
	WebhookURL      string
	Metadata        map[string]string
}
type CredencialAppyPay struct {
	ID                                               uuid.UUID
	ContextoTipo                                     ContextoTipo
	CodigoAcademia                                   string
	Ambiente                                         Ambiente
	AuthBaseURL, APIBaseURL, WebAPIBaseURL, ClientID string
	ClientSecretEncrypted                            string `json:"-"`
	ClientSecretMask, Resource                       string
	WebhookSecretEncrypted                           string `json:"-"`
	WebhookSecretMask                                string
	Applications                                     []Application
	Status                                           StatusCredencial
	Historico                                        []EventoFinanceiro
	CreatedAt, UpdatedAt                             time.Time
	Version                                          int
}
type CredencialInput struct {
	ContextoTipo   ContextoTipo       `json:"contexto_tipo"`
	CodigoAcademia string             `json:"codigo_academia"`
	Ambiente       Ambiente           `json:"ambiente"`
	AuthBaseURL    string             `json:"auth_base_url"`
	APIBaseURL     string             `json:"api_base_url"`
	WebAPIBaseURL  string             `json:"webapi_base_url"`
	ClientID       string             `json:"client_id"`
	ClientSecret   string             `json:"client_secret"`
	Resource       string             `json:"resource"`
	Applications   []ApplicationInput `json:"applications"`
	WebhookSecret  string             `json:"webhook_secret"`
}
type ApplicationInput struct {
	PaymentMethod string            `json:"paymentMethod"`
	ApplicationID string            `json:"applicationId"`
	APIKey        string            `json:"apiKey"`
	WebhookURL    string            `json:"webhook"`
	Metadata      map[string]string `json:"metadata"`
}
type ModalidadePagamento struct {
	GlobalAcademiasAtiva bool
	SpuriAtiva           bool
	Academias            map[string]bool
	Historico            []EventoFinanceiro
}
type EventoFinanceiro struct {
	Tipo, AutorID, AutorTipo, Motivo, Escopo, ContextoTipo, CodigoAcademia string
	At                                                                     time.Time
	Metadata                                                               map[string]any
}
type CobrancaFinanceira struct {
	ID                                                                                            uuid.UUID
	ContextoTipo                                                                                  ContextoTipo
	CodigoAcademia, PagadorTipo, PagadorID                                                        string
	Valor                                                                                         int64
	Moeda, MetodoPagamento, Descricao, ReferenciaExterna, MerchantTransactionID, ProviderChargeID string
	Metadata                                                                                      map[string]string
	Status                                                                                        StatusCobranca
	StatusProviderBruto                                                                           string
	Historico                                                                                     []EventoFinanceiro
	CreatedAt, UpdatedAt                                                                          time.Time
	Version                                                                                       int
}
type GerarCobrancaInput struct {
	ContextoTipo                                         ContextoTipo
	CodigoAcademia, PagadorTipo, PagadorID               string
	Valor                                                int64
	Moeda, MetodoPagamento, Descricao, ReferenciaExterna string
	Metadata                                             map[string]string
}
type Provider interface {
	TestarCredencial(context.Context, CredencialAppyPay) error
	CriarCobranca(context.Context, CredencialAppyPay, CobrancaFinanceira, Application) (string, string, error)
	ConsultarCobranca(context.Context, CredencialAppyPay, CobrancaFinanceira) (string, error)
}
type FakeProvider struct{}

func (FakeProvider) TestarCredencial(context.Context, CredencialAppyPay) error { return nil }
func (FakeProvider) CriarCobranca(context.Context, CredencialAppyPay, CobrancaFinanceira, Application) (string, string, error) {
	return "appy_" + uuid.NewString(), "PENDING", nil
}
func (FakeProvider) ConsultarCobranca(context.Context, CredencialAppyPay, CobrancaFinanceira) (string, error) {
	return "PAID", nil
}

type LedgerEvent struct {
	AggregateID uuid.UUID
	EventType   string
	Payload     map[string]any
	Metadata    map[string]any
	OccurredAt  time.Time
}

type LedgerWriter interface {
	AppendFinanceEvent(ctx context.Context, event LedgerEvent, autorID, autorTipo, origem string) (int, error)
	LoadFinanceEvents(ctx context.Context) ([]LedgerEvent, error)
}

type Service struct {
	mu         sync.Mutex
	creds      map[uuid.UUID]CredencialAppyPay
	charges    map[uuid.UUID]CobrancaFinanceira
	idem       map[string]uuid.UUID
	webhooks   map[string]bool
	modalidade ModalidadePagamento
	provider   Provider
	db         *sqlx.DB
	ledger     LedgerWriter
}

func NewService(p Provider) *Service {
	return NewServiceWithDB(nil, p)
}

func NewServiceWithDB(db *sqlx.DB, p Provider) *Service {
	return NewServiceWithDBAndLedger(db, p, nil)
}

func NewServiceWithClient(client *db.Client, p Provider) *Service {
	if client == nil {
		return NewService(p)
	}
	return NewServiceWithDBAndLedger(client.DB(), p, NewRepositoryLedger(client))
}

func NewServiceWithDBAndLedger(db *sqlx.DB, p Provider, ledger LedgerWriter) *Service {
	if p == nil {
		p = FakeProvider{}
	}
	s := &Service{creds: map[uuid.UUID]CredencialAppyPay{}, charges: map[uuid.UUID]CobrancaFinanceira{}, idem: map[string]uuid.UUID{}, webhooks: map[string]bool{}, modalidade: ModalidadePagamento{GlobalAcademiasAtiva: true, SpuriAtiva: true, Academias: map[string]bool{}}, provider: p, db: db}
	if ledger == nil && db != nil {
		ledger = SQLLedger{db: db}
	}
	s.ledger = ledger
	if db != nil {
		_ = s.loadPersisted(context.Background())
	}
	if ledger != nil && db == nil {
		_ = s.RebuildProjections(context.Background())
	}
	return s
}

func (SQLLedger) aggregateType() string { return "Financeiro" }

type RepositoryLedger struct{ repo *db.AggregateRepository }

func NewRepositoryLedger(client *db.Client) RepositoryLedger {
	return RepositoryLedger{repo: db.NewAggregateRepository(client)}
}

func (l RepositoryLedger) AppendFinanceEvent(ctx context.Context, event LedgerEvent, autorID, autorTipo, origem string) (int, error) {
	if l.repo == nil {
		return 0, nil
	}
	agg := aggregates.NewFinanceiroWithID(event.AggregateID)
	agg.RegistrarEvento(event.EventType, sanitizeMap(event.Payload))
	err := l.repo.WithContext(ctx).SaveWithAudit(agg, db.AuditContext{UserID: autorID, UserType: autorTipo, IP: origem})
	return agg.GetVersion(), err
}

func (l RepositoryLedger) LoadFinanceEvents(ctx context.Context) ([]LedgerEvent, error) {
	return nil, errors.New("RepositoryLedger não expõe replay; use FinanceiroProjection para rebuild canônico")
}

type SQLLedger struct{ db *sqlx.DB }

func (l SQLLedger) AppendFinanceEvent(ctx context.Context, event LedgerEvent, autorID, autorTipo, origem string) (int, error) {
	if l.db == nil {
		return 0, nil
	}
	if event.AggregateID == uuid.Nil {
		return 0, errors.New("aggregate_id financeiro inválido")
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}
	payload, err := json.Marshal(sanitizeMap(event.Payload))
	if err != nil {
		return 0, err
	}
	md := map[string]any{"user_id": autorID, "user_type": autorTipo, "origem": origem}
	for k, v := range event.Metadata {
		md[k] = v
	}
	metadata, err := json.Marshal(sanitizeMap(md))
	if err != nil {
		return 0, err
	}
	tx, err := l.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(event_version), 0)+1 FROM spuri_ledger WHERE aggregate_id=$1`, event.AggregateID).Scan(&version); err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO spuri_ledger (event_id, aggregate_id, aggregate_type, event_type, event_version, payload, metadata, occurred_at) VALUES ($1,$2,'Financeiro',$3,$4,$5,$6,$7)`, uuid.New(), event.AggregateID, event.EventType, version, payload, metadata, event.OccurredAt)
	if err != nil {
		return 0, err
	}
	return version, tx.Commit()
}

func (l SQLLedger) LoadFinanceEvents(ctx context.Context) ([]LedgerEvent, error) {
	if l.db == nil {
		return nil, nil
	}
	rows, err := l.db.QueryxContext(ctx, `SELECT aggregate_id, event_type, payload, metadata, occurred_at FROM spuri_ledger WHERE aggregate_type='Financeiro' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LedgerEvent
	for rows.Next() {
		var e LedgerEvent
		var p, m []byte
		if err := rows.Scan(&e.AggregateID, &e.EventType, &p, &m, &e.OccurredAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(p, &e.Payload)
		_ = json.Unmarshal(m, &e.Metadata)
		out = append(out, e)
	}
	return out, rows.Err()
}

func sanitizeMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range in {
		if ContainsSensitive(k) {
			out[k+"_redacted"] = "***"
			continue
		}
		out[k] = v
	}
	return out
}

func (s *Service) record(ctx context.Context, aggregateID uuid.UUID, eventType string, payload map[string]any, autorID, autorTipo, origem string) (EventoFinanceiro, error) {
	e := EventoFinanceiro{Tipo: eventType, AutorID: autorID, AutorTipo: autorTipo, At: time.Now().UTC()}
	if motivo, ok := payload["motivo"].(string); ok {
		e.Motivo = motivo
	}
	if s.ledger != nil {
		if _, err := s.ledger.AppendFinanceEvent(ctx, LedgerEvent{AggregateID: aggregateID, EventType: eventType, Payload: payload, Metadata: map[string]any{"motivo": e.Motivo}, OccurredAt: e.At}, autorID, autorTipo, origem); err != nil {
			return e, err
		}
	}
	return e, nil
}

func modalidadeAggregateID() uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("spuri:financeiro:modalidade"))
}

func credentialPayload(c CredencialAppyPay) map[string]any {
	return map[string]any{"credential_id": c.ID.String(), "contexto_tipo": string(c.ContextoTipo), "codigo_academia": c.CodigoAcademia, "ambiente": string(c.Ambiente), "auth_base_url": c.AuthBaseURL, "api_base_url": c.APIBaseURL, "webapi_base_url": c.WebAPIBaseURL, "client_id": c.ClientID, "resource": c.Resource, "client_secret_mask": c.ClientSecretMask, "webhook_secret_mask": c.WebhookSecretMask, "applications": safeApps(c.Applications), "status": string(c.Status)}
}
func safeApps(apps []Application) []map[string]any {
	out := []map[string]any{}
	for _, a := range apps {
		out = append(out, map[string]any{"paymentMethod": a.PaymentMethod, "applicationId": a.ApplicationID, "apiKey_mask": a.APIKeyMask, "webhook": a.WebhookURL, "metadata": a.Metadata})
	}
	return out
}
func payloadWithMotivo(payload map[string]any, motivo string) map[string]any {
	if motivo != "" {
		payload["motivo"] = motivo
	}
	return payload
}

func chargePayload(c CobrancaFinanceira) map[string]any {
	return map[string]any{"cobranca_id": c.ID.String(), "contexto_tipo": string(c.ContextoTipo), "codigo_academia": c.CodigoAcademia, "pagador_tipo": c.PagadorTipo, "pagador_id": c.PagadorID, "valor": c.Valor, "moeda": c.Moeda, "metodo_pagamento": c.MetodoPagamento, "descricao": c.Descricao, "metadata": c.Metadata, "referencia_externa": c.ReferenciaExterna, "merchant_transaction_id": c.MerchantTransactionID, "provider_charge_id": c.ProviderChargeID, "status": string(c.Status), "provider_status": c.StatusProviderBruto}
}

func (s *Service) loadPersisted(ctx context.Context) error {
	rows, err := s.db.QueryxContext(ctx, `SELECT payload FROM financeiro_credenciais_appypay`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var c CredencialAppyPay
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		if err := s.loadCredencialSegredos(ctx, &c); err != nil {
			return err
		}
		s.creds[c.ID] = c
	}
	rows, err = s.db.QueryxContext(ctx, `SELECT idempotency_key, payload FROM financeiro_cobrancas`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return err
		}
		var c CobrancaFinanceira
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		s.charges[c.ID] = c
		s.idem[key] = c.ID
	}
	rows, err = s.db.QueryxContext(ctx, `SELECT event_id FROM financeiro_webhooks_recebidos`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var eventID string
		if err := rows.Scan(&eventID); err != nil {
			return err
		}
		s.webhooks[eventID] = true
	}
	var raw []byte
	if err := s.db.QueryRowxContext(ctx, `SELECT payload FROM financeiro_modalidade_pagamento WHERE id='default'`).Scan(&raw); err == nil {
		_ = json.Unmarshal(raw, &s.modalidade)
	}
	return nil
}

func (s *Service) loadCredencialSegredos(ctx context.Context, c *CredencialAppyPay) error {
	if s.db == nil || c == nil {
		return nil
	}
	rows, err := s.db.QueryxContext(ctx, `
		SELECT DISTINCT ON (secret_type, COALESCE(application_id, '')) secret_type, COALESCE(application_id, ''), ciphertext
		FROM financeiro_segredos_appypay
		WHERE credential_id=$1 AND revoked_at IS NULL
		ORDER BY secret_type, COALESCE(application_id, ''), secret_version DESC
	`, c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var typ, appID, ciphertext string
		if err := rows.Scan(&typ, &appID, &ciphertext); err != nil {
			return err
		}
		switch typ {
		case "client_secret":
			c.ClientSecretEncrypted = ciphertext
		case "webhook_secret":
			c.WebhookSecretEncrypted = ciphertext
		case "api_key":
			for i := range c.Applications {
				if c.Applications[i].ApplicationID == appID {
					c.Applications[i].APIKeyEncrypted = ciphertext
				}
			}
		}
	}
	return rows.Err()
}

func (s *Service) projectCredencial(ctx context.Context, c CredencialAppyPay) error {
	if s.db == nil {
		return nil
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO financeiro_credenciais_appypay (id, payload, updated_at) VALUES ($1, $2, CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload, updated_at=CURRENT_TIMESTAMP`, c.ID, raw)
	if err != nil {
		return err
	}
	return s.projectCredencialSegredos(ctx, c)
}

func (s *Service) projectCredencialSegredos(ctx context.Context, c CredencialAppyPay) error {
	if s.db == nil {
		return nil
	}
	secrets := []struct {
		typ, appID, value string
	}{
		{typ: "client_secret", value: c.ClientSecretEncrypted},
		{typ: "webhook_secret", value: c.WebhookSecretEncrypted},
	}
	for _, app := range c.Applications {
		secrets = append(secrets, struct {
			typ, appID, value string
		}{typ: "api_key", appID: app.ApplicationID, value: app.APIKeyEncrypted})
	}
	for _, secret := range secrets {
		if secret.value == "" {
			continue
		}
		_, err := s.db.ExecContext(ctx, `INSERT INTO financeiro_segredos_appypay (credential_id, secret_version, secret_type, application_id, ciphertext) VALUES ($1, $2, $3, NULLIF($4, ''), $5) ON CONFLICT (credential_id, secret_type, COALESCE(application_id, ''), secret_version) DO UPDATE SET ciphertext=EXCLUDED.ciphertext, rotated_at=CURRENT_TIMESTAMP`, c.ID, c.Version, secret.typ, secret.appID, secret.value)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) projectCobranca(ctx context.Context, key string, c CobrancaFinanceira) error {
	if s.db == nil {
		return nil
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO financeiro_cobrancas (id, idempotency_key, payload, updated_at) VALUES ($1, $2, $3, CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key, payload=EXCLUDED.payload, updated_at=CURRENT_TIMESTAMP`, c.ID, key, raw)
	return err
}

func (s *Service) projectModalidade(ctx context.Context) error {
	if s.db == nil {
		return nil
	}
	raw, err := json.Marshal(s.modalidade)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO financeiro_modalidade_pagamento (id, payload, updated_at) VALUES ('default', $1, CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload, updated_at=CURRENT_TIMESTAMP`, raw)
	return err
}

func (s *Service) projectWebhook(ctx context.Context, eventID string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO financeiro_webhooks_recebidos (event_id) VALUES ($1) ON CONFLICT (event_id) DO NOTHING`, eventID)
	return err
}

func validarAutorID(autorID string) error {
	if strings.TrimSpace(autorID) == "" {
		return errors.New("autor_id obrigatório para auditoria financeira")
	}
	return nil
}

func (s *Service) CriarCredencial(ctx context.Context, in CredencialInput, autorID, autorTipo string) (CredencialAppyPay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validarAutorID(autorID); err != nil {
		return CredencialAppyPay{}, err
	}
	if autorTipo != "fpp" && autorTipo != "admin" {
		return CredencialAppyPay{}, errors.New("apenas FPP/ADMIN podem criar credenciais")
	}
	c, err := buildCredential(in)
	if err != nil {
		return c, err
	}
	ev, err := s.record(ctx, c.ID, "CredenciaisAppyPayCadastradas", credentialPayload(c), autorID, autorTipo, "http")
	if err != nil {
		return CredencialAppyPay{}, err
	}
	ev.ContextoTipo, ev.CodigoAcademia = string(c.ContextoTipo), c.CodigoAcademia
	c.Historico = append(c.Historico, ev)
	if err := s.projectCredencialSegredos(ctx, c); err != nil {
		return CredencialAppyPay{}, err
	}
	s.creds[c.ID] = c
	return maskCred(c), nil
}
func (s *Service) AtualizarCredencial(ctx context.Context, id uuid.UUID, in CredencialInput, autorID, autorTipo, codAcad string) (CredencialAppyPay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validarAutorID(autorID); err != nil {
		return CredencialAppyPay{}, err
	}
	old, ok := s.creds[id]
	if !ok {
		return old, errors.New("credencial não encontrada")
	}
	if autorTipo != "fpp" && autorTipo != "admin" && !(autorTipo == "academia" && old.ContextoTipo == ContextoAcademia && old.CodigoAcademia == codAcad) {
		return old, errors.New("sem permissão")
	}
	c, err := buildCredential(in)
	if err != nil {
		return c, err
	}
	if autorTipo == "academia" {
		c.ContextoTipo = old.ContextoTipo
		c.CodigoAcademia = old.CodigoAcademia
	}
	c.ID = id
	c.CreatedAt = old.CreatedAt
	c.Version = old.Version + 1
	ev, err := s.record(ctx, c.ID, "CredenciaisAppyPayAtualizadas", credentialPayload(c), autorID, autorTipo, "http")
	if err != nil {
		return CredencialAppyPay{}, err
	}
	ev.ContextoTipo, ev.CodigoAcademia = string(c.ContextoTipo), c.CodigoAcademia
	c.Historico = append(old.Historico, ev)
	if err := s.projectCredencialSegredos(ctx, c); err != nil {
		return CredencialAppyPay{}, err
	}
	s.creds[id] = c
	return maskCred(c), nil
}
func (s *Service) ListarCredenciais(autorTipo, codAcad string) []CredencialAppyPay {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []CredencialAppyPay{}
	for _, c := range s.creds {
		if !podeAcessarCredencial(autorTipo, codAcad, c) {
			continue
		}
		out = append(out, maskCred(c))
	}
	return out
}
func (s *Service) ObterCredencial(id uuid.UUID, autorTipo, codAcad string) (CredencialAppyPay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.creds[id]
	if !ok {
		return c, errors.New("credencial não encontrada")
	}
	if !podeAcessarCredencial(autorTipo, codAcad, c) {
		return c, errors.New("sem permissão")
	}
	return maskCred(c), nil
}
func (s *Service) TestarCredencial(ctx context.Context, id uuid.UUID, autorID, autorTipo, codAcad string) error {
	if err := validarAutorID(autorID); err != nil {
		return err
	}
	s.mu.Lock()
	c, ok := s.creds[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("credencial não encontrada")
	}
	if !podeAcessarCredencial(autorTipo, codAcad, c) {
		return errors.New("sem permissão")
	}
	if err := s.provider.TestarCredencial(ctx, c); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c = s.creds[id]
	ev, err := s.record(ctx, c.ID, "CredenciaisAppyPayValidadas", credentialPayload(c), autorID, autorTipo, "http")
	if err != nil {
		return err
	}
	ev.ContextoTipo, ev.CodigoAcademia = string(c.ContextoTipo), c.CodigoAcademia
	c.Historico = append(c.Historico, ev)
	if err := s.projectCredencialSegredos(ctx, c); err != nil {
		return err
	}
	s.creds[id] = c
	return nil
}
func podeAcessarCredencial(autorTipo, codAcad string, c CredencialAppyPay) bool {
	return autorTipo == "fpp" || autorTipo == "admin" || (autorTipo == "academia" && c.ContextoTipo == ContextoAcademia && c.CodigoAcademia == codAcad)
}

func (s *Service) AlterarStatusCredencial(id uuid.UUID, status StatusCredencial, autorID, autorTipo, codAcad, motivo string) (CredencialAppyPay, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validarAutorID(autorID); err != nil {
		return CredencialAppyPay{}, err
	}
	c, ok := s.creds[id]
	if !ok {
		return c, errors.New("credencial não encontrada")
	}
	if autorTipo != "fpp" && autorTipo != "admin" {
		return c, errors.New("apenas FPP/ADMIN")
	}
	c.Status = status
	c.UpdatedAt = time.Now().UTC()
	c.Version++
	eventoTipo := "CredenciaisAppyPayDesativadas"
	if status == StatusAtivo {
		eventoTipo = "CredenciaisAppyPayAtivadas"
	}
	ev, err := s.record(context.Background(), c.ID, eventoTipo, payloadWithMotivo(credentialPayload(c), motivo), autorID, autorTipo, "http")
	if err != nil {
		return CredencialAppyPay{}, err
	}
	ev.Motivo, ev.ContextoTipo, ev.CodigoAcademia = motivo, string(c.ContextoTipo), c.CodigoAcademia
	c.Historico = append(c.Historico, ev)
	if err := s.projectCredencialSegredos(context.Background(), c); err != nil {
		return CredencialAppyPay{}, err
	}
	s.creds[id] = c
	return maskCred(c), nil
}
func (s *Service) AlterarModalidade(escopo, codigo string, ativa bool, autorID, autorTipo, motivo string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validarAutorID(autorID); err != nil {
		return err
	}
	if autorTipo != "fpp" && autorTipo != "admin" {
		return errors.New("apenas FPP/ADMIN")
	}
	if escopo == "global_academias" {
		s.modalidade.GlobalAcademiasAtiva = ativa
	} else if escopo == "spuri" {
		s.modalidade.SpuriAtiva = ativa
	} else if escopo == "academia" {
		s.modalidade.Academias[codigo] = ativa
	} else {
		return errors.New("escopo inválido")
	}
	eventType := "ModalidadePagamentoGlobalAlterada"
	if escopo == "spuri" {
		eventType = "ModalidadePagamentoSpuriAlterada"
	} else if escopo == "academia" {
		eventType = "ModalidadePagamentoAcademiaAlterada"
	}
	ev, err := s.record(context.Background(), modalidadeAggregateID(), eventType, map[string]any{"escopo": escopo, "codigo_academia": codigo, "ativa": ativa, "motivo": motivo}, autorID, autorTipo, "http")
	if err != nil {
		return err
	}
	ev.Motivo, ev.Escopo, ev.CodigoAcademia = motivo, escopo, codigo
	s.modalidade.Historico = append(s.modalidade.Historico, ev)
	return nil
}
func (s *Service) GerarCobrancaFinanceiraBase(ctx context.Context, in GerarCobrancaInput, autorID string) (CobrancaFinanceira, error) {
	if err := validarAutorID(autorID); err != nil {
		return CobrancaFinanceira{}, err
	}
	in.Moeda = "AOA"
	if in.Valor <= 0 || in.ReferenciaExterna == "" {
		return CobrancaFinanceira{}, errors.New("valor e referencia_externa são obrigatórios")
	}

	s.mu.Lock()
	if in.ContextoTipo == ContextoAcademia {
		if !s.modalidade.GlobalAcademiasAtiva || !s.modalidade.Academias[in.CodigoAcademia] {
			s.mu.Unlock()
			return CobrancaFinanceira{}, errors.New("modalidade da academia inativa")
		}
		if in.PagadorTipo == "estudante" && in.Metadata["codigo_academia_estudante"] != "" && in.Metadata["codigo_academia_estudante"] != in.CodigoAcademia {
			s.mu.Unlock()
			return CobrancaFinanceira{}, errors.New("estudante não pertence à academia")
		}
	} else if !s.modalidade.SpuriAtiva {
		s.mu.Unlock()
		return CobrancaFinanceira{}, errors.New("modalidade spuri inativa")
	}
	key := string(in.ContextoTipo) + ":" + in.CodigoAcademia + ":" + in.ReferenciaExterna
	if id, ok := s.idem[key]; ok {
		ch := s.charges[id]
		s.mu.Unlock()
		return ch, nil
	}
	cred, app, err := s.activeCred(in.ContextoTipo, in.CodigoAcademia, in.MetodoPagamento)
	if err != nil {
		s.mu.Unlock()
		return CobrancaFinanceira{}, err
	}
	now := time.Now().UTC()
	ch := CobrancaFinanceira{ID: uuid.New(), ContextoTipo: in.ContextoTipo, CodigoAcademia: in.CodigoAcademia, PagadorTipo: in.PagadorTipo, PagadorID: in.PagadorID, Valor: in.Valor, Moeda: in.Moeda, MetodoPagamento: in.MetodoPagamento, Descricao: in.Descricao, ReferenciaExterna: in.ReferenciaExterna, MerchantTransactionID: merchantID(in), Metadata: in.Metadata, Status: CobrancaPendente, CreatedAt: now, UpdatedAt: now, Version: 1}
	ev, err := s.record(ctx, ch.ID, "CobrancaFinanceiraCriada", chargePayload(ch), autorID, "sistema", "http")
	if err != nil {
		s.mu.Unlock()
		return CobrancaFinanceira{}, err
	}
	ch.Historico = append(ch.Historico, ev)
	s.charges[ch.ID] = ch
	s.idem[key] = ch.ID
	s.mu.Unlock()

	pid, pstatus, providerErr := s.provider.CriarCobranca(ctx, cred, ch, app)

	s.mu.Lock()
	defer s.mu.Unlock()
	if providerErr != nil {
		ch.Status = CobrancaFalhada
	} else {
		ch.ProviderChargeID = pid
		ch.Status = CobrancaEnviada
		ch.StatusProviderBruto = pstatus
	}
	ev, recErr := s.record(ctx, ch.ID, "CobrancaFinanceiraEnviadaAoProvider", chargePayload(ch), autorID, "sistema", "provider")
	if recErr != nil {
		return CobrancaFinanceira{}, recErr
	}
	ev.Metadata = map[string]any{"provider_status": pstatus}
	ch.Historico = append(ch.Historico, ev)
	s.charges[ch.ID] = ch
	s.idem[key] = ch.ID
	return ch, providerErr
}
func (s *Service) ConsultarCobrancaFinanceiraBase(id uuid.UUID) (CobrancaFinanceira, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.charges[id]
	if !ok {
		return c, errors.New("cobrança não encontrada")
	}
	return c, nil
}
func (s *Service) SincronizarStatusCobrancaFinanceiraBase(ctx context.Context, id uuid.UUID, autorID string) (CobrancaFinanceira, error) {
	if err := validarAutorID(autorID); err != nil {
		return CobrancaFinanceira{}, err
	}
	s.mu.Lock()
	c, ok := s.charges[id]
	if !ok {
		s.mu.Unlock()
		return c, errors.New("cobrança não encontrada")
	}
	cred, _, err := s.activeCred(c.ContextoTipo, c.CodigoAcademia, c.MetodoPagamento)
	s.mu.Unlock()
	if err != nil {
		return c, err
	}
	st, err := s.provider.ConsultarCobranca(ctx, cred, c)
	if err != nil {
		return c, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c = s.charges[id]
	c.StatusProviderBruto = st
	if st == "PAID" {
		c.Status = CobrancaLiquidada
	}
	c.Version++
	c.UpdatedAt = time.Now().UTC()
	ev, err := s.record(ctx, c.ID, "CobrancaFinanceiraStatusAtualizado", chargePayload(c), autorID, "sistema", "reconciliation")
	if err != nil {
		return CobrancaFinanceira{}, err
	}
	ev.Metadata = map[string]any{"provider_status": st}
	c.Historico = append(c.Historico, ev)
	s.charges[id] = c
	return c, nil
}
func (s *Service) CancelarCobrancaFinanceiraBase(id uuid.UUID, autor, motivo string) (CobrancaFinanceira, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validarAutorID(autor); err != nil {
		return CobrancaFinanceira{}, err
	}
	c := s.charges[id]
	if c.Status == CobrancaLiquidada {
		return c, errors.New("cobrança liquidada não pode ser cancelada")
	}
	c.Status = CobrancaCancelada
	ev, err := s.record(context.Background(), c.ID, "CobrancaFinanceiraCancelada", payloadWithMotivo(chargePayload(c), motivo), autor, "sistema", "http")
	if err != nil {
		return CobrancaFinanceira{}, err
	}
	ev.Motivo = motivo
	c.Historico = append(c.Historico, ev)
	s.charges[id] = c
	return c, nil
}

func (s *Service) ReembolsarCobrancaFinanceiraBase(id uuid.UUID, valor int64, autor, motivo string) (EventoFinanceiro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validarAutorID(autor); err != nil {
		return EventoFinanceiro{}, err
	}
	c, ok := s.charges[id]
	if !ok {
		return EventoFinanceiro{}, errors.New("cobrança não encontrada")
	}
	if c.Status != CobrancaLiquidada {
		return EventoFinanceiro{}, errors.New("apenas cobranças liquidadas podem ser reembolsadas")
	}
	if valor <= 0 || valor > c.Valor {
		return EventoFinanceiro{}, errors.New("valor de reembolso inválido")
	}
	e, err := s.record(context.Background(), id, "ReembolsoFinanceiroSolicitado", map[string]any{"cobranca_id": id.String(), "valor": valor, "motivo": motivo}, autor, "sistema", "http")
	if err != nil {
		return EventoFinanceiro{}, err
	}
	e.Motivo = motivo
	e.Metadata = map[string]any{"cobranca_id": id.String(), "valor": valor}
	c.Historico = append(c.Historico, e)
	c.Version++
	s.charges[id] = c
	return e, nil
}

func (s *Service) ReverterCobrancaFinanceiraBase(id uuid.UUID, autor, motivo string) (EventoFinanceiro, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validarAutorID(autor); err != nil {
		return EventoFinanceiro{}, err
	}
	c, ok := s.charges[id]
	if !ok {
		return EventoFinanceiro{}, errors.New("cobrança não encontrada")
	}
	if c.MetodoPagamento != "UMM" {
		return EventoFinanceiro{}, errors.New("reversão disponível apenas para método suportado")
	}
	e, err := s.record(context.Background(), id, "ReversaoFinanceiraSolicitada", map[string]any{"cobranca_id": id.String(), "motivo": motivo}, autor, "sistema", "http")
	if err != nil {
		return EventoFinanceiro{}, err
	}
	e.Motivo = motivo
	e.Metadata = map[string]any{"cobranca_id": id.String()}
	c.Historico = append(c.Historico, e)
	c.Version++
	s.charges[id] = c
	return e, nil
}

func (s *Service) ProcessarWebhookFinanceiroBase(ctx context.Context, eventID string, chargeID uuid.UUID, autorID string) (bool, error) {
	if err := validarAutorID(autorID); err != nil {
		return false, err
	}
	s.mu.Lock()
	if s.webhooks[eventID] {
		_, _ = s.record(ctx, chargeID, "WebhookFinanceiroIgnoradoComoDuplicado", map[string]any{"event_id": eventID, "cobranca_id": chargeID.String()}, autorID, "sistema", "webhook")
		s.mu.Unlock()
		return false, nil
	}
	if _, err := s.record(ctx, chargeID, "WebhookFinanceiroRecebido", map[string]any{"event_id": eventID, "cobranca_id": chargeID.String()}, autorID, "sistema", "webhook"); err != nil {
		s.mu.Unlock()
		return false, err
	}
	if err := s.projectWebhook(ctx, eventID); err != nil {
		s.mu.Unlock()
		return false, err
	}
	s.webhooks[eventID] = true
	s.mu.Unlock()
	_, err := s.SincronizarStatusCobrancaFinanceiraBase(ctx, chargeID, autorID)
	return true, err
}
func (s *Service) ReconciliarFinanceiroBase(ctx context.Context, autorID string) ([]EventoFinanceiro, error) {
	if err := validarAutorID(autorID); err != nil {
		return nil, err
	}
	ev, err := s.record(ctx, modalidadeAggregateID(), "ReconciliacaoFinanceiraExecutada", map[string]any{"resultado": "executada"}, autorID, "sistema", "reconciliation")
	if err != nil {
		return nil, err
	}
	return []EventoFinanceiro{ev}, nil
}
func (s *Service) activeCred(ct ContextoTipo, cod, method string) (CredencialAppyPay, Application, error) {
	for _, c := range s.creds {
		if c.ContextoTipo == ct && c.Status == StatusAtivo && (ct == ContextoSpuri || c.CodigoAcademia == cod) {
			for _, a := range c.Applications {
				if a.PaymentMethod == method {
					return c, a, nil
				}
			}
		}
	}
	return CredencialAppyPay{}, Application{}, errors.New("credencial/application ativa não encontrada")
}
func buildCredential(in CredencialInput) (CredencialAppyPay, error) {
	if in.ContextoTipo != ContextoSpuri && in.ContextoTipo != ContextoAcademia {
		return CredencialAppyPay{}, errors.New("contexto_tipo inválido")
	}
	if in.ContextoTipo == ContextoAcademia && in.CodigoAcademia == "" {
		return CredencialAppyPay{}, errors.New("codigo_academia obrigatório")
	}
	apiBaseURL := strings.TrimSpace(os.Getenv(AppyPayAPIBaseURLEnv))
	if apiBaseURL == "" {
		return CredencialAppyPay{}, fmt.Errorf("%s obrigatório", AppyPayAPIBaseURLEnv)
	}
	if in.AuthBaseURL == "" || in.ClientID == "" || in.ClientSecret == "" || in.Resource == "" || len(in.Applications) == 0 {
		return CredencialAppyPay{}, errors.New("campos obrigatórios ausentes")
	}
	now := time.Now().UTC()
	cs, err := encrypt(in.ClientSecret)
	if err != nil {
		return CredencialAppyPay{}, err
	}
	wh, err := encrypt(in.WebhookSecret)
	if err != nil {
		return CredencialAppyPay{}, err
	}
	apps := []Application{}
	for _, a := range in.Applications {
		ek, err := encrypt(a.APIKey)
		if err != nil {
			return CredencialAppyPay{}, err
		}
		apps = append(apps, Application{PaymentMethod: a.PaymentMethod, ApplicationID: a.ApplicationID, APIKeyEncrypted: ek, APIKeyMask: MaskSecret(a.APIKey), WebhookURL: a.WebhookURL, Metadata: a.Metadata})
	}
	return CredencialAppyPay{ID: uuid.New(), ContextoTipo: in.ContextoTipo, CodigoAcademia: in.CodigoAcademia, Ambiente: in.Ambiente, AuthBaseURL: in.AuthBaseURL, APIBaseURL: apiBaseURL, WebAPIBaseURL: in.WebAPIBaseURL, ClientID: in.ClientID, ClientSecretEncrypted: cs, ClientSecretMask: MaskSecret(in.ClientSecret), Resource: in.Resource, WebhookSecretEncrypted: wh, WebhookSecretMask: MaskSecret(in.WebhookSecret), Applications: apps, Status: StatusPendenteValidacao, CreatedAt: now, UpdatedAt: now, Version: 1}, nil
}
func maskCred(c CredencialAppyPay) CredencialAppyPay {
	c.ClientSecretEncrypted = ""
	c.WebhookSecretEncrypted = ""
	for i := range c.Applications {
		c.Applications[i].APIKeyEncrypted = ""
		c.Applications[i].APIKey = ""
	}
	return c
}
func MaskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return "****" + s[len(s)-4:]
}
func financeEncryptionKeyMaterial() (string, error) {
	keyMaterial := strings.TrimSpace(os.Getenv("FINANCE_ENCRYPTION_KEY"))
	if keyMaterial == "" {
		return "", errors.New("FINANCE_ENCRYPTION_KEY obrigatório")
	}
	return keyMaterial, nil
}

func ValidateEncryptionConfig() error {
	_, err := financeEncryptionKeyMaterial()
	return err
}

func decrypt(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	keyMaterial, err := financeEncryptionKeyMaterial()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(keyMaterial + "spuri-finance-default-key"))
	block, err := aes.NewCipher(h[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext inválido")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func encrypt(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	keyMaterial, err := financeEncryptionKeyMaterial()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(keyMaterial + "spuri-finance-default-key"))
	block, err := aes.NewCipher(h[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(v), nil)), nil
}
func merchantID(in GerarCobrancaInput) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s", in.ContextoTipo, in.CodigoAcademia, in.ReferenciaExterna)))
	return "spuri_" + hex.EncodeToString(sum[:])[:32]
}
func ContainsSensitive(s string) bool {
	l := strings.ToLower(s)
	return strings.Contains(l, "client_secret") || strings.Contains(l, "apikey") || strings.Contains(l, "api_key") || strings.Contains(l, "token") || strings.Contains(l, "webhook_secret")
}

func (s *Service) RebuildProjections(ctx context.Context) error {
	if s.ledger == nil {
		return nil
	}
	events, err := s.ledger.LoadFinanceEvents(ctx)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.creds = map[uuid.UUID]CredencialAppyPay{}
	s.charges = map[uuid.UUID]CobrancaFinanceira{}
	s.idem = map[string]uuid.UUID{}
	s.webhooks = map[string]bool{}
	s.modalidade = ModalidadePagamento{GlobalAcademiasAtiva: true, SpuriAtiva: true, Academias: map[string]bool{}}
	for _, e := range events {
		if err := s.applyLedgerProjection(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func eventoFromLedger(e LedgerEvent, escopo, codigo string) EventoFinanceiro {
	ev := EventoFinanceiro{Tipo: e.EventType, At: e.OccurredAt, Escopo: escopo, CodigoAcademia: codigo, Metadata: e.Payload}
	if v, ok := e.Metadata["user_id"].(string); ok {
		ev.AutorID = v
	}
	if v, ok := e.Metadata["user_type"].(string); ok {
		ev.AutorTipo = v
	}
	if v, ok := e.Payload["motivo"].(string); ok {
		ev.Motivo = v
	} else if v, ok := e.Metadata["motivo"].(string); ok {
		ev.Motivo = v
	}
	if v, ok := e.Payload["contexto_tipo"].(string); ok {
		ev.ContextoTipo = v
	}
	return ev
}

func (s *Service) applyLedgerProjection(ctx context.Context, e LedgerEvent) error {
	switch e.EventType {
	case "CredenciaisAppyPayCadastradas", "CredenciaisAppyPayAtualizadas", "CredenciaisAppyPayValidadas", "CredenciaisAppyPayAtivadas", "CredenciaisAppyPayDesativadas":
		// Segredos cifrados são armazenamento operacional; replay público mantém metadados/máscaras.
		c := s.creds[e.AggregateID]
		c.ID = e.AggregateID
		if v, ok := e.Payload["contexto_tipo"].(string); ok {
			c.ContextoTipo = ContextoTipo(v)
		}
		if v, ok := e.Payload["codigo_academia"].(string); ok {
			c.CodigoAcademia = v
		}
		if v, ok := e.Payload["ambiente"].(string); ok {
			c.Ambiente = Ambiente(v)
		}
		if v, ok := e.Payload["auth_base_url"].(string); ok {
			c.AuthBaseURL = v
		}
		if v, ok := e.Payload["api_base_url"].(string); ok {
			c.APIBaseURL = v
		}
		if v, ok := e.Payload["webapi_base_url"].(string); ok {
			c.WebAPIBaseURL = v
		}
		if v, ok := e.Payload["client_id"].(string); ok {
			c.ClientID = v
		}
		if v, ok := e.Payload["resource"].(string); ok {
			c.Resource = v
		}
		if v, ok := e.Payload["client_secret_mask"].(string); ok {
			c.ClientSecretMask = v
		}
		if v, ok := e.Payload["webhook_secret_mask"].(string); ok {
			c.WebhookSecretMask = v
		}
		if v, ok := e.Payload["status"].(string); ok {
			c.Status = StatusCredencial(v)
		}
		c.UpdatedAt = e.OccurredAt
		if c.CreatedAt.IsZero() {
			c.CreatedAt = e.OccurredAt
		}
		c.Version++
		c.Historico = append(c.Historico, eventoFromLedger(e, "", c.CodigoAcademia))
		s.creds[e.AggregateID] = c
		return s.projectCredencial(ctx, c)
	case "ModalidadePagamentoGlobalAlterada", "ModalidadePagamentoSpuriAlterada", "ModalidadePagamentoAcademiaAlterada":
		ativa, _ := e.Payload["ativa"].(bool)
		escopo, _ := e.Payload["escopo"].(string)
		codigo, _ := e.Payload["codigo_academia"].(string)
		if escopo == "global_academias" {
			s.modalidade.GlobalAcademiasAtiva = ativa
		} else if escopo == "spuri" {
			s.modalidade.SpuriAtiva = ativa
		} else if escopo == "academia" {
			s.modalidade.Academias[codigo] = ativa
		}
		s.modalidade.Historico = append(s.modalidade.Historico, eventoFromLedger(e, escopo, codigo))
		return s.projectModalidade(ctx)
	case "CobrancaFinanceiraCriada", "CobrancaFinanceiraEnviadaAoProvider", "CobrancaFinanceiraStatusAtualizado", "CobrancaFinanceiraCancelada":
		c := s.charges[e.AggregateID]
		c.ID = e.AggregateID
		if v, ok := e.Payload["contexto_tipo"].(string); ok {
			c.ContextoTipo = ContextoTipo(v)
		}
		if v, ok := e.Payload["codigo_academia"].(string); ok {
			c.CodigoAcademia = v
		}
		if v, ok := e.Payload["pagador_tipo"].(string); ok {
			c.PagadorTipo = v
		}
		if v, ok := e.Payload["pagador_id"].(string); ok {
			c.PagadorID = v
		}
		if v, ok := e.Payload["valor"].(float64); ok {
			c.Valor = int64(v)
		}
		if v, ok := e.Payload["moeda"].(string); ok {
			c.Moeda = v
		}
		if c.Moeda == "" {
			c.Moeda = "AOA"
		}
		if v, ok := e.Payload["metodo_pagamento"].(string); ok {
			c.MetodoPagamento = v
		}
		if v, ok := e.Payload["descricao"].(string); ok {
			c.Descricao = v
		}
		if v, ok := e.Payload["referencia_externa"].(string); ok {
			c.ReferenciaExterna = v
		}
		if v, ok := e.Payload["merchant_transaction_id"].(string); ok {
			c.MerchantTransactionID = v
		}
		if v, ok := e.Payload["provider_charge_id"].(string); ok {
			c.ProviderChargeID = v
		}
		if v, ok := e.Payload["provider_status"].(string); ok {
			c.StatusProviderBruto = v
		}
		if v, ok := e.Payload["status"].(string); ok {
			c.Status = StatusCobranca(v)
		}
		c.UpdatedAt = e.OccurredAt
		if c.CreatedAt.IsZero() {
			c.CreatedAt = e.OccurredAt
		}
		c.Version++
		c.Historico = append(c.Historico, eventoFromLedger(e, "", c.CodigoAcademia))
		key := string(c.ContextoTipo) + ":" + c.CodigoAcademia + ":" + c.ReferenciaExterna
		s.charges[e.AggregateID] = c
		s.idem[key] = e.AggregateID
		return s.projectCobranca(ctx, key, c)
	case "WebhookFinanceiroRecebido", "WebhookFinanceiroIgnoradoComoDuplicado":
		if id, ok := e.Payload["event_id"].(string); ok {
			s.webhooks[id] = true
			return s.projectWebhook(ctx, id)
		}
		return nil
	case "ReembolsoFinanceiroSolicitado", "ReembolsoFinanceiroStatusAtualizado", "ReversaoFinanceiraSolicitada", "ReversaoFinanceiraStatusAtualizado", "DivergenciaFinanceiraDetectada", "DivergenciaFinanceiraReconciliada", "ReconciliacaoFinanceiraExecutada":
		return nil
	default:
		return fmt.Errorf("evento financeiro desconhecido no replay: %s", e.EventType)
	}
}
