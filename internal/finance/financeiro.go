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
	PaymentMethod, ApplicationID, APIKey, APIKeyEncrypted, APIKeyMask, WebhookURL string
	Metadata                                                                      map[string]string
}
type CredencialAppyPay struct {
	ID                                                                                                                                             uuid.UUID
	ContextoTipo                                                                                                                                   ContextoTipo
	CodigoAcademia                                                                                                                                 string
	Ambiente                                                                                                                                       Ambiente
	AuthBaseURL, APIBaseURL, WebAPIBaseURL, ClientID, ClientSecretEncrypted, ClientSecretMask, Resource, WebhookSecretEncrypted, WebhookSecretMask string
	Applications                                                                                                                                   []Application
	Status                                                                                                                                         StatusCredencial
	Historico                                                                                                                                      []EventoFinanceiro
	CreatedAt, UpdatedAt                                                                                                                           time.Time
	Version                                                                                                                                        int
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

type Service struct {
	mu         sync.Mutex
	creds      map[uuid.UUID]CredencialAppyPay
	charges    map[uuid.UUID]CobrancaFinanceira
	idem       map[string]uuid.UUID
	webhooks   map[string]bool
	modalidade ModalidadePagamento
	provider   Provider
	db         *sqlx.DB
}

func NewService(p Provider) *Service {
	return NewServiceWithDB(nil, p)
}

func NewServiceWithDB(db *sqlx.DB, p Provider) *Service {
	if p == nil {
		p = FakeProvider{}
	}
	s := &Service{creds: map[uuid.UUID]CredencialAppyPay{}, charges: map[uuid.UUID]CobrancaFinanceira{}, idem: map[string]uuid.UUID{}, webhooks: map[string]bool{}, modalidade: ModalidadePagamento{GlobalAcademiasAtiva: true, SpuriAtiva: true, Academias: map[string]bool{}}, provider: p, db: db}
	if db != nil {
		_ = s.loadPersisted(context.Background())
	}
	return s
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

func (s *Service) persistCredencial(ctx context.Context, c CredencialAppyPay) error {
	if s.db == nil {
		return nil
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO financeiro_credenciais_appypay (id, payload, updated_at) VALUES ($1, $2, CURRENT_TIMESTAMP) ON CONFLICT (id) DO UPDATE SET payload=EXCLUDED.payload, updated_at=CURRENT_TIMESTAMP`, c.ID, raw)
	return err
}

func (s *Service) persistCobranca(ctx context.Context, key string, c CobrancaFinanceira) error {
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

func (s *Service) persistModalidade(ctx context.Context) error {
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

func (s *Service) persistWebhook(ctx context.Context, eventID string) error {
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
	c.Historico = append(c.Historico, EventoFinanceiro{Tipo: "CredenciaisAppyPayCadastradas", AutorID: autorID, AutorTipo: autorTipo, ContextoTipo: string(c.ContextoTipo), CodigoAcademia: c.CodigoAcademia, At: c.CreatedAt})
	if err := s.persistCredencial(ctx, c); err != nil {
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
	c.ID = id
	c.CreatedAt = old.CreatedAt
	c.Version = old.Version + 1
	c.Historico = append(old.Historico, EventoFinanceiro{Tipo: "CredenciaisAppyPayAtualizadas", AutorID: autorID, AutorTipo: autorTipo, ContextoTipo: string(c.ContextoTipo), CodigoAcademia: c.CodigoAcademia, At: c.UpdatedAt})
	if err := s.persistCredencial(ctx, c); err != nil {
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
		if autorTipo == "academia" && (c.ContextoTipo != ContextoAcademia || c.CodigoAcademia != codAcad) {
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
	if autorTipo == "academia" && (c.ContextoTipo != ContextoAcademia || c.CodigoAcademia != codAcad) {
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
	if autorTipo == "academia" && (c.ContextoTipo != ContextoAcademia || c.CodigoAcademia != codAcad) {
		return errors.New("sem permissão")
	}
	if err := s.provider.TestarCredencial(ctx, c); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c = s.creds[id]
	c.Historico = append(c.Historico, EventoFinanceiro{Tipo: "CredenciaisAppyPayValidadas", AutorID: autorID, AutorTipo: autorTipo, ContextoTipo: string(c.ContextoTipo), CodigoAcademia: c.CodigoAcademia, At: time.Now().UTC()})
	if err := s.persistCredencial(ctx, c); err != nil {
		return err
	}
	s.creds[id] = c
	return nil
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
	c.Historico = append(c.Historico, EventoFinanceiro{Tipo: eventoTipo, AutorID: autorID, AutorTipo: autorTipo, Motivo: motivo, ContextoTipo: string(c.ContextoTipo), CodigoAcademia: c.CodigoAcademia, At: c.UpdatedAt})
	if err := s.persistCredencial(context.Background(), c); err != nil {
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
	s.modalidade.Historico = append(s.modalidade.Historico, EventoFinanceiro{Tipo: "ModalidadePagamentoAlterada", AutorID: autorID, AutorTipo: autorTipo, Motivo: motivo, Escopo: escopo, CodigoAcademia: codigo, At: time.Now().UTC()})
	return s.persistModalidade(context.Background())
}
func (s *Service) GerarCobrancaFinanceiraBase(ctx context.Context, in GerarCobrancaInput, autorID string) (CobrancaFinanceira, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := validarAutorID(autorID); err != nil {
		return CobrancaFinanceira{}, err
	}
	in.Moeda = "AOA"
	if in.Valor <= 0 || in.ReferenciaExterna == "" {
		return CobrancaFinanceira{}, errors.New("valor e referencia_externa são obrigatórios")
	}
	if in.ContextoTipo == ContextoAcademia {
		if !s.modalidade.GlobalAcademiasAtiva || !s.modalidade.Academias[in.CodigoAcademia] {
			return CobrancaFinanceira{}, errors.New("modalidade da academia inativa")
		}
		if in.PagadorTipo == "estudante" && in.Metadata["codigo_academia_estudante"] != "" && in.Metadata["codigo_academia_estudante"] != in.CodigoAcademia {
			return CobrancaFinanceira{}, errors.New("estudante não pertence à academia")
		}
	} else if !s.modalidade.SpuriAtiva {
		return CobrancaFinanceira{}, errors.New("modalidade spuri inativa")
	}
	key := string(in.ContextoTipo) + ":" + in.CodigoAcademia + ":" + in.ReferenciaExterna
	if id, ok := s.idem[key]; ok {
		return s.charges[id], nil
	}
	cred, app, err := s.activeCred(in.ContextoTipo, in.CodigoAcademia, in.MetodoPagamento)
	if err != nil {
		return CobrancaFinanceira{}, err
	}
	now := time.Now().UTC()
	ch := CobrancaFinanceira{ID: uuid.New(), ContextoTipo: in.ContextoTipo, CodigoAcademia: in.CodigoAcademia, PagadorTipo: in.PagadorTipo, PagadorID: in.PagadorID, Valor: in.Valor, Moeda: in.Moeda, MetodoPagamento: in.MetodoPagamento, Descricao: in.Descricao, ReferenciaExterna: in.ReferenciaExterna, MerchantTransactionID: merchantID(in), Metadata: in.Metadata, Status: CobrancaPendente, CreatedAt: now, UpdatedAt: now, Version: 1}
	ch.Historico = append(ch.Historico, EventoFinanceiro{Tipo: "CobrancaFinanceiraCriada", AutorID: autorID, At: now})
	pid, pstatus, err := s.provider.CriarCobranca(ctx, cred, ch, app)
	if err != nil {
		ch.Status = CobrancaFalhada
	} else {
		ch.ProviderChargeID = pid
		ch.Status = CobrancaEnviada
		ch.StatusProviderBruto = pstatus
	}
	ch.Historico = append(ch.Historico, EventoFinanceiro{Tipo: "CobrancaFinanceiraEnviadaAoProvider", AutorID: autorID, At: time.Now().UTC(), Metadata: map[string]any{"provider_status": pstatus}})
	if persistErr := s.persistCobranca(ctx, key, ch); persistErr != nil {
		return CobrancaFinanceira{}, persistErr
	}
	s.charges[ch.ID] = ch
	s.idem[key] = ch.ID
	return ch, err
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
	c.Historico = append(c.Historico, EventoFinanceiro{Tipo: "CobrancaFinanceiraStatusAtualizado", AutorID: autorID, At: c.UpdatedAt, Metadata: map[string]any{"provider_status": st}})
	key := string(c.ContextoTipo) + ":" + c.CodigoAcademia + ":" + c.ReferenciaExterna
	if err := s.persistCobranca(ctx, key, c); err != nil {
		return CobrancaFinanceira{}, err
	}
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
	c.Historico = append(c.Historico, EventoFinanceiro{Tipo: "CobrancaFinanceiraCancelada", AutorID: autor, Motivo: motivo, At: time.Now().UTC()})
	key := string(c.ContextoTipo) + ":" + c.CodigoAcademia + ":" + c.ReferenciaExterna
	if err := s.persistCobranca(context.Background(), key, c); err != nil {
		return CobrancaFinanceira{}, err
	}
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
	e := EventoFinanceiro{Tipo: "ReembolsoFinanceiroSolicitado", AutorID: autor, Motivo: motivo, At: time.Now().UTC(), Metadata: map[string]any{"cobranca_id": id.String(), "valor": valor}}
	c.Historico = append(c.Historico, e)
	c.Version++
	key := string(c.ContextoTipo) + ":" + c.CodigoAcademia + ":" + c.ReferenciaExterna
	if err := s.persistCobranca(context.Background(), key, c); err != nil {
		return EventoFinanceiro{}, err
	}
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
	e := EventoFinanceiro{Tipo: "ReversaoFinanceiraSolicitada", AutorID: autor, Motivo: motivo, At: time.Now().UTC(), Metadata: map[string]any{"cobranca_id": id.String()}}
	c.Historico = append(c.Historico, e)
	c.Version++
	key := string(c.ContextoTipo) + ":" + c.CodigoAcademia + ":" + c.ReferenciaExterna
	if err := s.persistCobranca(context.Background(), key, c); err != nil {
		return EventoFinanceiro{}, err
	}
	s.charges[id] = c
	return e, nil
}

func (s *Service) ProcessarWebhookFinanceiroBase(ctx context.Context, eventID string, chargeID uuid.UUID, autorID string) (bool, error) {
	if err := validarAutorID(autorID); err != nil {
		return false, err
	}
	s.mu.Lock()
	if s.webhooks[eventID] {
		s.mu.Unlock()
		return false, nil
	}
	if err := s.persistWebhook(ctx, eventID); err != nil {
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
	return []EventoFinanceiro{{Tipo: "ReconciliacaoFinanceiraExecutada", AutorID: autorID, At: time.Now().UTC()}}, nil
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
	cs, _ := encrypt(in.ClientSecret)
	wh, _ := encrypt(in.WebhookSecret)
	apps := []Application{}
	for _, a := range in.Applications {
		ek, _ := encrypt(a.APIKey)
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
func encrypt(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	h := sha256.Sum256([]byte(os.Getenv("FINANCE_ENCRYPTION_KEY") + "spuri-finance-default-key"))
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
