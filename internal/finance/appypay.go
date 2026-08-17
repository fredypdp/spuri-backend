// Package finance is the only package allowed to call AppyPay's HTTP API.
//
// Monetary contract: every monetary value in this package and in payment
// phases 2, 3 and 4 is float64. This deliberately mirrors AppyPay's
// number<double> contract. New internal fields (such as ValorMensalidade and
// ValorMatricula) must use float64 plus roundAmount and amountsEqual below;
// cents, strings, decimal libraries, and local rounding rules are forbidden.
package finance

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)

const (
	ContextoSpuri    = "spuri"
	ContextoAcademia = "academia"
	AmbienteTeste    = "test"
	AmbienteProducao = "prod"
	asyncAccept      = "application/vnd.appypay.asyncapi+json"
)

var (
	// ErrNotFound is returned when an AppyPay resource does not exist in the
	// authorised financial context.
	ErrNotFound = errors.New("recurso financeiro não encontrado")
	// ErrUpstream means AppyPay could not be reached or returned an invalid
	// response. It is deliberately distinct from request validation errors.
	ErrUpstream = errors.New("serviço AppyPay indisponível")
	// ErrConflict denotes an idempotency key that is currently being processed.
	ErrConflict = errors.New("operação financeira equivalente em processamento")
)

const amountTolerance = 0.005

// roundAmount rounds a monetary amount to two decimal places using half away
// from zero (math.Round's documented rule). It is the sole monetary rounding
// rule for this module and the payment phases built on it.
func roundAmount(v float64) float64 { return math.Round(v*100) / 100 }

// amountsEqual compares monetary float64 values using half a cent of
// tolerance, avoiding direct floating-point equality for money.
func amountsEqual(a, b float64) bool { return math.Abs(a-b) <= amountTolerance }

type Endpoints struct{ TokenURL, APIBaseURL string }

func AmbienteAtual() string {
	if strings.EqualFold(os.Getenv("ENV"), "production") {
		return AmbienteProducao
	}
	return AmbienteTeste
}
func EndpointsAtuais() Endpoints {
	if AmbienteAtual() == AmbienteProducao {
		return Endpoints{"https://login.microsoftonline.com/auth.appypay.co.ao/oauth2/token", "https://gwy-api.appypay.co.ao/v2.0"}
	}
	return Endpoints{"https://login.microsoftonline.com/appypaydev.onmicrosoft.com/oauth2/token", "https://gwy-api-tst.appypay.co.ao/v2.0"}
}

// appyPayResource retorna o "resource" (UUID) exigido pela AppyPay no pedido
// de token OAuth2 client_credentials. Ao contrário de client_id/client_secret,
// este valor identifica a própria API da AppyPay (audience do token), não o
// merchant que está a autenticar — por isso é o mesmo para todas as
// academias e para o Spuri no mesmo ambiente, e vem de variável de ambiente
// em vez de ser configurado por credencial.
func appyPayResource() (string, error) {
	v := strings.TrimSpace(os.Getenv("APPYPAY_RESOURCE"))
	if v == "" {
		return "", errors.New("APPYPAY_RESOURCE não configurada")
	}
	return v, nil
}

// ValidateAppyPayResourceConfig valida no arranque do servidor que
// APPYPAY_RESOURCE está definida, antes de qualquer tentativa de gerar um
// token AppyPay.
func ValidateAppyPayResourceConfig() error {
	_, err := appyPayResource()
	return err
}

type CredentialInput struct {
	ContextoTipo     string `json:"contexto_tipo"`
	CodigoAcademia   string `json:"codigo_academia,omitempty"`
	Ambiente         string `json:"ambiente"`
	ClientID         string `json:"client_id"`
	ClientSecret     string `json:"client_secret"`
	GPOPaymentMethod string `json:"gpo_payment_method"`
	REFPaymentMethod string `json:"ref_payment_method"`
}
type CredentialView struct {
	ID                   uuid.UUID `json:"id"`
	ContextoTipo         string    `json:"contexto_tipo"`
	CodigoAcademia       string    `json:"codigo_academia,omitempty"`
	Ambiente             string    `json:"ambiente"`
	ClientIDMask         string    `json:"client_id_mask"`
	GPOPaymentMethodMask string    `json:"gpo_payment_method_mask"`
	REFPaymentMethodMask string    `json:"ref_payment_method_mask"`
	WebhookHeaderName    string    `json:"webhook_header_name,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
}
type ChargeRequest struct {
	ContextoTipo          string         `json:"contexto_tipo"`
	CodigoAcademia        string         `json:"codigo_academia,omitempty"`
	Amount                float64        `json:"amount"`
	Currency              string         `json:"currency"`
	Description           string         `json:"description"`
	MerchantTransactionID string         `json:"merchantTransactionId,omitempty"`
	PaymentMethod         string         `json:"paymentMethod"`
	PaymentInfo           map[string]any `json:"paymentInfo,omitempty"`
	Options               map[string]any `json:"options,omitempty"`
	Notify                map[string]any `json:"notify,omitempty"`
	Async                 bool           `json:"async"`
	// These fields are auditable Spuri metadata and are not forwarded to AppyPay.
	Mensalidades      []MensalidadeSelecaoMes `json:"mensalidades,omitempty"`
	CodigoEstudante   string                  `json:"codigo_estudante,omitempty"`
	CodigoSolicitacao string                  `json:"codigo_solicitacao,omitempty"`
}
type QRCodeRequest struct {
	ContextoTipo          string   `json:"contexto_tipo"`
	CodigoAcademia        string   `json:"codigo_academia,omitempty"`
	Amount                float64  `json:"amount"`
	Currency              string   `json:"currency"`
	Description           string   `json:"description"`
	MerchantTransactionID string   `json:"merchantTransactionId,omitempty"`
	QRCodeType            string   `json:"qrCodeType,omitempty"`
	MinAmount             *float64 `json:"minAmount,omitempty"`
	MaxTransactions       *int     `json:"maxTransactions,omitempty"`
	StartDate             string   `json:"startDate,omitempty"`
	EndDate               string   `json:"endDate,omitempty"`
	// Mensalidades is ledger metadata only; it is never sent to AppyPay.
	Mensalidades      []MensalidadeSelecaoMes `json:"mensalidades,omitempty"`
	CodigoEstudante   string                  `json:"codigo_estudante,omitempty"`
	CodigoSolicitacao string                  `json:"codigo_solicitacao,omitempty"`
}
type ChargeResult struct {
	ID                    uuid.UUID      `json:"id"`
	ProviderChargeID      string         `json:"provider_charge_id,omitempty"`
	MerchantTransactionID string         `json:"merchant_transaction_id"`
	Status                string         `json:"status"`
	Response              map[string]any `json:"response,omitempty"`
}
type QRCodeResult struct {
	ChargeResult
	QRCodeArr string `json:"qrCodeArr,omitempty"`
}

// CobrancaResumo é o resumo de uma cobrança devolvido pela listagem
// GET /financeiro/cobrancas. Deliberadamente não inclui payment_info,
// response nem qrCodeArr — esses detalhes completos continuam disponíveis
// apenas em GET /financeiro/appypay/cobrancas/:id, para quem já sabe o
// identificador da cobrança. Ver Problema 1 em
// docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md.
type CobrancaResumo struct {
	ID                    uuid.UUID `json:"id"`
	ProviderChargeID      string    `json:"provider_charge_id,omitempty"`
	MerchantTransactionID string    `json:"merchant_transaction_id"`
	ContextoTipo          string    `json:"contexto_tipo"`
	CodigoAcademia        string    `json:"codigo_academia,omitempty"`
	// Origem é derivada do payload da cobrança, nunca persistida
	// separadamente: "matricula" quando há codigo_solicitacao,
	// "mensalidade" quando há codigo_estudante (e não há
	// codigo_solicitacao), "avulsa" nos demais casos (cobrança criada
	// diretamente via POST /financeiro/appypay/cobrancas ou /appypay/qr-codes
	// sem vínculo a mensalidade nem matrícula).
	Origem    string  `json:"origem"`
	Status    string  `json:"status"`
	Valor     float64 `json:"valor"`
	Moeda     string  `json:"moeda,omitempty"`
	Descricao string  `json:"descricao,omitempty"`
	// MetodoPagamento reflete "GPO_QR" (não apenas "GPO") quando a cobrança
	// tem qr_code_type no payload — CreateGPOQRCode grava payment_method
	// como "GPO" internamente, então sem este ajuste a origem QR ficaria
	// indistinguível de um GPO comum nesta listagem.
	MetodoPagamento   string                  `json:"metodo_pagamento,omitempty"`
	CodigoEstudante   string                  `json:"codigo_estudante,omitempty"`
	CodigoSolicitacao string                  `json:"codigo_solicitacao,omitempty"`
	Mensalidades      []MensalidadeSelecaoMes `json:"mensalidades,omitempty"`
	AtualizadoEm      time.Time               `json:"atualizado_em"`
}

// CobrancaListResult é o resultado paginado de ListCobrancas.
type CobrancaListResult struct {
	Cobrancas []CobrancaResumo `json:"cobrancas"`
	Total     int              `json:"total"`
}

type tokenEntry struct {
	value     string
	expiresAt time.Time
}
type Service struct {
	client     *db.Client
	repository *db.AggregateRepository
	projection *projections.FinanceiroProjection
	httpClient *http.Client
	mu         sync.Mutex
	tokens     map[uuid.UUID]tokenEntry
}

func NewService(client *db.Client) *Service {
	var repo *db.AggregateRepository
	var projection *projections.FinanceiroProjection
	if client != nil {
		repo = db.NewAggregateRepository(client)
		projection = projections.NewFinanceiroProjection(client)
	}
	return &Service{client: client, repository: repo, projection: projection, httpClient: &http.Client{Timeout: 20 * time.Second}, tokens: map[uuid.UUID]tokenEntry{}}
}

// SetHTTPClient overrides the AppyPay HTTP client. It is intended for tests and
// controlled integrations that need a custom RoundTripper.
func (s *Service) SetHTTPClient(client *http.Client) {
	if client != nil {
		s.httpClient = client
	}
}

func mask(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return "****" + v[len(v)-4:]
}

// WebhookHeaderName é o único cabeçalho HTTP em que a AppyPay pode enviar o
// segredo de webhook. É fixo para toda a plataforma (todas as academias e o
// Spuri, em qualquer ambiente) — a AppyPay só oferece um único par nome/valor
// de cabeçalho no painel deles, então não há necessidade de variar por
// credencial. Prefixado com o nome do produto, seguindo a convenção comum de
// cabeçalhos customizados (ex.: X-GitHub-*, X-Stripe-*). Se o nome precisar
// mudar no futuro, basta trocar esta constante — nenhum outro código depende
// do valor literal.
const WebhookHeaderName = "X-Spuri-Webhook-Secret"

const (
	webhookSecretLength   = 15
	webhookSecretAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// generateWebhookSecret gera um segredo alfanumérico de 15 caracteres usando
// crypto/rand (seguro criptograficamente) com math/big para evitar víes de
// módulo sobre o alfabeto.
func generateWebhookSecret() (string, error) {
	out := make([]byte, webhookSecretLength)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(webhookSecretAlphabet))))
		if err != nil {
			return "", fmt.Errorf("erro ao gerar segredo de webhook: %w", err)
		}
		out[i] = webhookSecretAlphabet[n.Int64()]
	}
	return string(out), nil
}

// hasWebhookSecret verifica, sem decifrar nada, se uma credencial já possui
// um segredo de webhook gravado no cofre — usado por ConfigureCredential
// para decidir se deve gerar um segredo novo (apenas na primeira vez que uma
// credencial recebe um).
func (s *Service) hasWebhookSecret(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_segredos_appypay WHERE credential_id=$1 AND secret_type='webhook_secret')`, id).Scan(&exists)
	return exists, err
}

func validContext(c, academy string) error {
	if c == ContextoSpuri && academy == "" {
		return nil
	}
	if c == ContextoAcademia && strings.TrimSpace(academy) != "" {
		return nil
	}
	return errors.New("contexto financeiro inválido")
}
func (s *Service) ConfigureCredential(ctx context.Context, id *uuid.UUID, in CredentialInput, userID, userType, ip string) (CredentialView, string, error) {
	if s.client == nil {
		return CredentialView{}, "", errors.New("serviço financeiro não inicializado")
	}
	in.ContextoTipo = strings.ToLower(strings.TrimSpace(in.ContextoTipo))
	in.Ambiente = strings.ToLower(strings.TrimSpace(in.Ambiente))
	if err := validContext(in.ContextoTipo, in.CodigoAcademia); err != nil {
		return CredentialView{}, "", err
	}
	if in.Ambiente == "" {
		in.Ambiente = AmbienteAtual()
	}
	if in.Ambiente != AmbienteAtual() {
		return CredentialView{}, "", errors.New("credenciais devem pertencer ao ambiente ativo")
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(in.ClientID) == "" || strings.TrimSpace(in.ClientSecret) == "" || !strings.HasPrefix(in.GPOPaymentMethod, "GPO_") || !strings.HasPrefix(in.REFPaymentMethod, "REF_") {
		return CredentialView{}, "", errors.New("credenciais AppyPay incompletas ou inválidas")
	}
	credentialID := uuid.New()
	if id != nil {
		credentialID = *id
		if err := s.credentialBelongsToScope(ctx, credentialID, in.ContextoTipo, in.CodigoAcademia, in.Ambiente); err != nil {
			return CredentialView{}, "", err
		}
	} else if found, err := s.findCredentialID(ctx, in.ContextoTipo, in.CodigoAcademia, in.Ambiente); err == nil {
		credentialID = found
	}
	// O segredo de webhook só é gerado na primeira vez que esta credencial
	// recebe um — atualizações subsequentes nunca o tocam.
	hadSecret, err := s.hasWebhookSecret(ctx, credentialID)
	if err != nil {
		return CredentialView{}, "", err
	}
	secretsToSave := map[string]string{"client_id": in.ClientID, "client_secret": in.ClientSecret, "gpo_method": in.GPOPaymentMethod, "ref_method": in.REFPaymentMethod}
	var newWebhookSecret string
	if !hadSecret {
		newWebhookSecret, err = generateWebhookSecret()
		if err != nil {
			return CredentialView{}, "", err
		}
		secretsToSave["webhook_secret"] = newWebhookSecret
	}
	view := CredentialView{ID: credentialID, ContextoTipo: in.ContextoTipo, CodigoAcademia: in.CodigoAcademia, Ambiente: in.Ambiente, ClientIDMask: mask(in.ClientID), GPOPaymentMethodMask: mask(in.GPOPaymentMethod), REFPaymentMethodMask: mask(in.REFPaymentMethod), WebhookHeaderName: WebhookHeaderName, UpdatedAt: time.Now().UTC()}
	payload := map[string]any{"credential_id": credentialID.String(), "contexto_tipo": view.ContextoTipo, "codigo_academia": view.CodigoAcademia, "ambiente": view.Ambiente, "client_id_mask": view.ClientIDMask, "gpo_payment_method_mask": view.GPOPaymentMethodMask, "ref_payment_method_mask": view.REFPaymentMethodMask, "updated_at": view.UpdatedAt}
	if err := s.record(ctx, credentialID, "CredenciaisAppyPayConfiguradas", payload, userID, userType, ip); err != nil {
		return CredentialView{}, "", err
	}
	if err := s.saveSecrets(ctx, credentialID, secretsToSave); err != nil {
		return CredentialView{}, "", err
	}
	return view, newWebhookSecret, nil
}

func (s *Service) ListCredentials(ctx context.Context, contexto, academia string) ([]CredentialView, error) {
	q := `SELECT id,contexto_tipo,COALESCE(codigo_academia,''),ambiente,payload,updated_at FROM financeiro_credenciais_appypay`
	args := []any{}
	where := []string{}
	if contexto != "" {
		where = append(where, "contexto_tipo=$1")
		args = append(args, contexto)
	}
	if academia != "" {
		where = append(where, fmt.Sprintf("codigo_academia=$%d", len(args)+1))
		args = append(args, academia)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY updated_at DESC"
	rows, err := s.client.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CredentialView{}
	for rows.Next() {
		var v CredentialView
		var raw []byte
		if err := rows.Scan(&v.ID, &v.ContextoTipo, &v.CodigoAcademia, &v.Ambiente, &raw, &v.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &v)
		v.WebhookHeaderName = WebhookHeaderName
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) CreateCharge(ctx context.Context, in ChargeRequest, actorID, actorType, ip string) (ChargeResult, error) {
	if err := validateCharge(&in); err != nil {
		return ChargeResult{}, err
	}
	in.Amount = roundAmount(in.Amount)
	credential, err := s.loadCredential(ctx, in.ContextoTipo, in.CodigoAcademia)
	if err != nil {
		return ChargeResult{}, err
	}
	method, err := credential.method(in.PaymentMethod)
	if err != nil {
		return ChargeResult{}, err
	}
	if in.MerchantTransactionID == "" {
		in.MerchantTransactionID = merchantID()
	}
	if !validMerchantID(in.MerchantTransactionID) {
		return ChargeResult{}, errors.New("merchantTransactionId deve ser alfanumérico e ter no máximo 15 caracteres")
	}
	id := uuid.New()
	reserved, err := s.reserveCharge(ctx, in.MerchantTransactionID, id)
	if err != nil {
		return ChargeResult{}, err
	}
	if !reserved {
		result, err := s.existingChargeResult(ctx, in.MerchantTransactionID, in.ContextoTipo, in.CodigoAcademia)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrNotFound) {
			return ChargeResult{}, ErrConflict
		}
		return ChargeResult{}, err
	}
	payload := chargePayload(id, in, "", "solicitada", nil)
	if err = s.record(ctx, id, "CobrancaAppyPaySolicitada", payload, actorID, actorType, ip); err != nil {
		_ = s.releaseChargeReservation(ctx, in.MerchantTransactionID, id)
		return ChargeResult{}, err
	}
	providerBody := map[string]any{"amount": in.Amount, "currency": in.Currency, "description": in.Description, "merchantTransactionId": in.MerchantTransactionID, "paymentMethod": method, "paymentInfo": in.PaymentInfo, "options": in.Options, "notify": in.Notify}
	response, err := s.callJSON(ctx, credential, http.MethodPost, "/charges", providerBody, in.Async)
	if err != nil {
		_ = s.record(ctx, id, "CobrancaAppyPayFalhou", chargePayload(id, in, "", "falhada", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
		return ChargeResult{ID: id, MerchantTransactionID: in.MerchantTransactionID, Status: "falhada"}, err
	}
	providerID := responseID(response)
	status := responseStatus(response)
	if status == "" {
		status = "criada"
	}
	if err = s.record(ctx, id, "CobrancaAppyPayCriada", chargePayload(id, in, providerID, status, response), actorID, actorType, ip); err != nil {
		return ChargeResult{}, err
	}
	result := ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}
	if isSuccessfulChargeStatus(status) {
		_ = s.confirmMensalidadeCharge(ctx, id, actorID, actorType, ip)
	}
	return result, nil
}
func (s *Service) CreateGPOQRCode(ctx context.Context, in QRCodeRequest, actorID, actorType, ip string) (QRCodeResult, error) {
	if err := validateQRCode(&in); err != nil {
		return QRCodeResult{}, err
	}
	in.Amount = roundAmount(in.Amount)
	if in.MinAmount != nil {
		v := roundAmount(*in.MinAmount)
		in.MinAmount = &v
	}
	if in.MerchantTransactionID == "" {
		in.MerchantTransactionID = merchantID()
	}
	if !validMerchantID(in.MerchantTransactionID) {
		return QRCodeResult{}, errors.New("merchantTransactionId deve ser alfanumérico e ter no máximo 15 caracteres")
	}
	typ := strings.ToUpper(in.QRCodeType)
	if typ == "" {
		typ = "SINGLE"
	}
	if typ != "SINGLE" && typ != "MULTIPLE" {
		return QRCodeResult{}, errors.New("qrCodeType inválido")
	}
	if typ == "MULTIPLE" && (in.MinAmount == nil || in.MaxTransactions == nil || in.StartDate == "" || in.EndDate == "") {
		return QRCodeResult{}, errors.New("QR MULTIPLE exige minAmount, maxTransactions, startDate e endDate")
	}
	cred, err := s.loadCredential(ctx, in.ContextoTipo, in.CodigoAcademia)
	if err != nil {
		return QRCodeResult{}, err
	}
	id := uuid.New()
	reserved, err := s.reserveCharge(ctx, in.MerchantTransactionID, id)
	if err != nil {
		return QRCodeResult{}, err
	}
	if !reserved {
		result, err := s.existingQRCodeResult(ctx, in.MerchantTransactionID, in.ContextoTipo, in.CodigoAcademia)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, ErrNotFound) {
			return QRCodeResult{}, ErrConflict
		}
		return QRCodeResult{}, err
	}
	body := map[string]any{"amount": in.Amount, "currency": in.Currency, "description": in.Description, "merchantTransactionId": in.MerchantTransactionID, "paymentMethod": cred.GPO, "qrCodeType": typ}
	if typ == "MULTIPLE" {
		body["minAmount"] = *in.MinAmount
		body["maxTransactions"] = *in.MaxTransactions
		body["startDate"] = in.StartDate
		body["endDate"] = in.EndDate
	}
	if err = s.record(ctx, id, "QRCodeAppyPaySolicitado", qrCodePayload(id, in, typ, "", "solicitada", nil), actorID, actorType, ip); err != nil {
		_ = s.releaseChargeReservation(ctx, in.MerchantTransactionID, id)
		return QRCodeResult{}, err
	}
	response, err := s.callJSON(ctx, cred, http.MethodPost, "/qr-codes", body, false)
	if err != nil {
		_ = s.record(ctx, id, "QRCodeAppyPayFalhou", qrCodePayload(id, in, typ, "", "falhada", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
		return QRCodeResult{}, err
	}
	providerID := responseID(response)
	status := responseStatus(response)
	if status == "" {
		status = "criada"
	}
	payload := qrCodePayload(id, in, typ, providerID, status, response)
	if err = s.record(ctx, id, "QRCodeAppyPayGerado", payload, actorID, actorType, ip); err != nil {
		return QRCodeResult{}, err
	}
	qr, _ := response["qrCodeArr"].(string)
	result := QRCodeResult{ChargeResult: ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}, QRCodeArr: qr}
	if isSuccessfulChargeStatus(status) {
		_ = s.confirmMensalidadeCharge(ctx, id, actorID, actorType, ip)
	}
	return result, nil
}
func (s *Service) ConsultCharge(ctx context.Context, contexto, academia, identifier, actorID, actorType, ip string) (ChargeResult, error) {
	if identifier == "" {
		return ChargeResult{}, errors.New("id da cobrança é obrigatório")
	}
	row, err := s.loadCharge(ctx, identifier)
	if err != nil {
		return ChargeResult{}, err
	}
	if row.Contexto != contexto || row.Academia != academia {
		return ChargeResult{}, fmt.Errorf("%w: cobrança não encontrada no contexto", ErrNotFound)
	}
	result, err := s.consultCharge(ctx, row, actorID, actorType, ip)
	if err == nil && isSuccessfulChargeStatus(result.Status) {
		_ = s.confirmMensalidadeCharge(ctx, row.ID, actorID, actorType, ip)
	}
	return result, err
}

// CancelCharge only cancels the local Spuri charge record. AppyPay has no
// equivalent cancellation endpoint for REF, GPO, or GPO QR Codes, so a fresh
// provider consultation is mandatory before the cancellation is recorded.
func (s *Service) CancelCharge(ctx context.Context, contexto, academia, identifier, motivo, actorID, actorType, ip string) (ChargeResult, error) {
	if strings.TrimSpace(identifier) == "" {
		return ChargeResult{}, errors.New("id da cobrança é obrigatório")
	}
	row, err := s.loadCharge(ctx, identifier)
	if err != nil {
		return ChargeResult{}, err
	}
	if row.Contexto != contexto || row.Academia != academia || !canCancelCharge(row, academia, actorType) {
		return ChargeResult{}, fmt.Errorf("%w: cobrança não encontrada no contexto", ErrNotFound)
	}
	if isTerminalChargeStatus(row.Status) {
		return ChargeResult{ID: row.ID, ProviderChargeID: row.ProviderID, MerchantTransactionID: row.Merchant, Status: row.Status}, errors.New("cobrança já está em estado terminal e não pode ser cancelada")
	}
	current, err := s.consultCharge(ctx, row, actorID, actorType, ip)
	if err != nil {
		return current, err
	}
	if isSuccessfulChargeStatus(current.Status) {
		return current, errors.New("cobrança já foi paga e não pode ser cancelada")
	}
	payload := make(map[string]any, len(row.Payload)+4)
	for key, value := range row.Payload {
		payload[key] = value
	}
	payload["charge_id"] = row.ID.String()
	payload["contexto_tipo"] = row.Contexto
	payload["codigo_academia"] = row.Academia
	payload["provider_charge_id"] = current.ProviderChargeID
	payload["status"] = "cancelada"
	payload["response"] = sanitize(current.Response)
	if motivo = strings.TrimSpace(motivo); motivo != "" {
		payload["motivo"] = motivo
	}
	if err = s.record(ctx, row.ID, "CobrancaAppyPayCancelada", payload, actorID, actorType, ip); err != nil {
		return ChargeResult{}, err
	}
	return ChargeResult{ID: row.ID, ProviderChargeID: current.ProviderChargeID, MerchantTransactionID: row.Merchant, Status: "cancelada", Response: current.Response}, nil
}

// ListCobrancas lista cobranças AppyPay (mensalidade, matrícula ou avulsa)
// filtrando por contexto/academia, estado (status) e origem, com paginação.
// contexto e academia vazios não restringem a consulta — o mesmo padrão de
// filtro opcional já usado em ListCredentials. estados e origens vazios
// também não restringem. limit/offset devem já vir validados (bounded) por
// quem chama (ver handlers.ListarCobrancasAppyPay).
//
// O filtro de estado é um match exato (case-sensitive) sobre o texto cru
// persistido em payload->>'status' — o mesmo texto devolvido no campo
// "status" de GET /financeiro/appypay/cobrancas/:id. Esse campo mistura
// estados internos do Spuri ("solicitada", "criada", "cancelada", "falhada")
// com estados crus devolvidos pela AppyPay ("Success", "Pending", "Failed",
// etc.) — este método deliberadamente não normaliza nada, pelo mesmo motivo
// que isSuccessfulChargeStatus/isTerminalChargeStatus já fazem sua própria
// comparação case-insensitive em vez de normalizar o dado persistido.
func (s *Service) ListCobrancas(ctx context.Context, contexto, academia string, estados, origens []string, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	where := "WHERE 1=1"
	args := []any{}
	i := 1
	if contexto != "" {
		where += fmt.Sprintf(" AND contexto_tipo=$%d", i)
		args = append(args, contexto)
		i++
	}
	if academia != "" {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, academia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estados))
		i++
	}
	if len(origens) > 0 {
		clauses := make([]string, 0, len(origens))
		for _, origem := range origens {
			switch origem {
			case "matricula":
				clauses = append(clauses, "COALESCE(payload->>'codigo_solicitacao','') <> ''")
			case "mensalidade":
				clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_estudante','') <> '')")
			case "avulsa":
				clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_estudante','') = '')")
			default:
				return nil, fmt.Errorf("tipo de cobrança inválido: %s", origem)
			}
		}
		where += " AND (" + strings.Join(clauses, " OR ") + ")"
	}
	var total int
	if err := s.client.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM financeiro_cobrancas "+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`SELECT id, COALESCE(provider_charge_id,''), merchant_transaction_id, contexto_tipo, COALESCE(codigo_academia,''), payload, updated_at FROM financeiro_cobrancas %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, where, i, i+1)
	args = append(args, limit, offset)
	rows, err := s.client.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CobrancaResumo{}
	for rows.Next() {
		var dto CobrancaResumo
		var rawPayload []byte
		if err := rows.Scan(&dto.ID, &dto.ProviderChargeID, &dto.MerchantTransactionID, &dto.ContextoTipo, &dto.CodigoAcademia, &rawPayload, &dto.AtualizadoEm); err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(rawPayload, &payload); err != nil {
			return nil, err
		}
		dto.Status, _ = payload["status"].(string)
		dto.Valor, _ = payload["amount"].(float64)
		dto.Moeda, _ = payload["currency"].(string)
		dto.Descricao, _ = payload["description"].(string)
		dto.MetodoPagamento, _ = payload["payment_method"].(string)
		if qrType, ok := payload["qr_code_type"].(string); ok && qrType != "" {
			dto.MetodoPagamento = "GPO_QR"
		}
		dto.CodigoEstudante, _ = payload["codigo_estudante"].(string)
		dto.CodigoSolicitacao, _ = payload["codigo_solicitacao"].(string)
		switch {
		case dto.CodigoSolicitacao != "":
			dto.Origem = "matricula"
		case dto.CodigoEstudante != "":
			dto.Origem = "mensalidade"
		default:
			dto.Origem = "avulsa"
		}
		if mesesRaw, ok := payload["mensalidades"]; ok && mesesRaw != nil {
			if b, err := json.Marshal(mesesRaw); err == nil {
				var meses []MensalidadeSelecaoMes
				if json.Unmarshal(b, &meses) == nil {
					dto.Mensalidades = meses
				}
			}
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
}

func canCancelCharge(row chargeRow, academia, actorType string) bool {
	switch actorType {
	case "admin":
		return row.Contexto == ContextoSpuri && row.Academia == ""
	case "academia":
		return row.Contexto == ContextoAcademia && row.Academia == academia
	default:
		return false
	}
}

func isSuccessfulChargeStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "Success")
}

func isTerminalChargeStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "cancelada") ||
		strings.EqualFold(strings.TrimSpace(status), "falhada") ||
		isSuccessfulChargeStatus(status)
}

// consultCharge is shared by normal consultation and cancellation's mandatory
// pre-check. A late Success after local cancellation is preserved as a
// reconciliation conflict and never changes the local cancelled status.
func (s *Service) consultCharge(ctx context.Context, row chargeRow, actorID, actorType, ip string) (ChargeResult, error) {
	cred, err := s.loadCredential(ctx, row.Contexto, row.Academia)
	if err != nil {
		return ChargeResult{}, err
	}
	path := "/charges/" + url.PathEscape(row.ProviderID)
	if row.ProviderID == "" {
		path = "/charges?merchantTransactionId=" + url.QueryEscape(row.Merchant)
	}
	response, err := s.callJSON(ctx, cred, http.MethodGet, path, nil, false)
	if err != nil {
		return ChargeResult{}, err
	}
	status := responseStatus(response)
	if status == "" {
		status = row.Status
	}
	previousResponse := row.Payload["response"]
	payload := make(map[string]any, len(row.Payload)+3)
	for key, value := range row.Payload {
		payload[key] = value
	}
	payload["provider_charge_id"] = first(responseID(response), row.ProviderID)
	payload["status"] = status
	payload["response"] = sanitize(response)
	providerID := first(responseID(response), row.ProviderID)
	if strings.EqualFold(row.Status, "cancelada") && isSuccessfulChargeStatus(status) {
		// Keep the cancellation definitive in the read model. The provider
		// result is recorded for manual FPP reconciliation instead of silently
		// accepting a payment that raced with local cancellation.
		payload["status"] = "cancelada"
		payload["provider_status"] = status
		if err = s.record(ctx, row.ID, "CobrancaAppyPayConflitoPosCancelamento", payload, actorID, actorType, ip); err != nil {
			return ChargeResult{}, err
		}
		return ChargeResult{ID: row.ID, ProviderChargeID: providerID, MerchantTransactionID: row.Merchant, Status: "cancelada", Response: response}, nil
	}
	if status != row.Status || providerID != row.ProviderID || !sameJSON(payload["response"], previousResponse) {
		if err = s.record(ctx, row.ID, "CobrancaAppyPayConsultada", payload, actorID, actorType, ip); err != nil {
			return ChargeResult{}, err
		}
	}
	return ChargeResult{ID: row.ID, ProviderChargeID: providerID, MerchantTransactionID: row.Merchant, Status: status, Response: response}, nil
}

type credentialSecrets struct {
	ID                               uuid.UUID
	ClientID, ClientSecret, GPO, REF string
}

func (c credentialSecrets) method(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if strings.EqualFold(requested, "GPO") || strings.EqualFold(requested, c.GPO) {
		return c.GPO, nil
	}
	if strings.EqualFold(requested, "REF") || strings.EqualFold(requested, c.REF) {
		return c.REF, nil
	}
	return "", errors.New("paymentMethod não contratado para esta conta")
}
func (s *Service) loadCredential(ctx context.Context, contexto, academia string) (credentialSecrets, error) {
	if err := validContext(contexto, academia); err != nil {
		return credentialSecrets{}, err
	}
	var id uuid.UUID
	err := s.client.DB().QueryRowContext(ctx, `SELECT id FROM financeiro_credenciais_appypay WHERE contexto_tipo=$1 AND codigo_academia IS NOT DISTINCT FROM NULLIF($2,'') AND ambiente=$3`, contexto, academia, AmbienteAtual()).Scan(&id)
	if err != nil {
		return credentialSecrets{}, fmt.Errorf("%w: credenciais AppyPay não configuradas para este contexto", ErrNotFound)
	}
	values, err := s.loadSecrets(ctx, id)
	if err != nil {
		return credentialSecrets{}, err
	}
	return credentialSecrets{ID: id, ClientID: values["client_id"], ClientSecret: values["client_secret"], GPO: values["gpo_method"], REF: values["ref_method"]}, nil
}
func (s *Service) findCredentialID(ctx context.Context, contexto, academia, ambiente string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.client.DB().QueryRowContext(ctx, `SELECT id FROM financeiro_credenciais_appypay WHERE contexto_tipo=$1 AND codigo_academia IS NOT DISTINCT FROM NULLIF($2,'') AND ambiente=$3`, contexto, academia, ambiente).Scan(&id)
	return id, err
}

func (s *Service) credentialBelongsToScope(ctx context.Context, id uuid.UUID, contexto, academia, ambiente string) error {
	var found uuid.UUID
	err := s.client.DB().QueryRowContext(ctx, `SELECT id FROM financeiro_credenciais_appypay WHERE id=$1 AND contexto_tipo=$2 AND codigo_academia IS NOT DISTINCT FROM NULLIF($3,'') AND ambiente=$4`, id, contexto, academia, ambiente).Scan(&found)
	if err != nil {
		return fmt.Errorf("%w: credencial AppyPay não encontrada no contexto", ErrNotFound)
	}
	return nil
}

// CredentialScope resolve o contexto (spuri/academia) e a academia dona de
// uma credencial pelo seu id — usado pelos handlers de segredo de webhook
// para reaplicar a mesma autorização (authorizeFinanceScope) das demais
// rotas de credenciais.
func (s *Service) CredentialScope(ctx context.Context, id uuid.UUID) (string, string, error) {
	var contexto, academia string
	if err := s.client.DB().QueryRowContext(ctx, `SELECT contexto_tipo, COALESCE(codigo_academia,'') FROM financeiro_credenciais_appypay WHERE id=$1`, id).Scan(&contexto, &academia); err != nil {
		return "", "", fmt.Errorf("%w: credencial AppyPay não encontrada", ErrNotFound)
	}
	return contexto, academia, nil
}

// WebhookSecret devolve o segredo de webhook atual em texto plano.
func (s *Service) WebhookSecret(ctx context.Context, id uuid.UUID) (string, error) {
	secrets, err := s.loadSecrets(ctx, id)
	if err != nil {
		return "", err
	}
	secret := secrets["webhook_secret"]
	if secret == "" {
		return "", fmt.Errorf("%w: credencial sem segredo de webhook", ErrNotFound)
	}
	return secret, nil
}

// RotateWebhookSecret gera um novo segredo de webhook, substitui o anterior
// no cofre e grava um evento de auditoria no ledger. A rotação é imediata:
// o segredo anterior deixa de autenticar assim que esta função retorna sem
// erro.
func (s *Service) RotateWebhookSecret(ctx context.Context, id uuid.UUID, userID, userType, ip string) (string, error) {
	secret, err := generateWebhookSecret()
	if err != nil {
		return "", err
	}
	if err := s.record(ctx, id, "SegredoWebhookAppyPayRotacionado", map[string]any{"credential_id": id.String()}, userID, userType, ip); err != nil {
		return "", err
	}
	if err := s.saveSecrets(ctx, id, map[string]string{"webhook_secret": secret}); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Service) callJSON(ctx context.Context, cred credentialSecrets, method, path string, body any, async bool) (map[string]any, error) {
	token, err := s.token(ctx, cred)
	if err != nil {
		return nil, err
	}
	var data io.Reader
	if body != nil {
		raw, e := json.Marshal(body)
		if e != nil {
			return nil, e
		}
		data = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, EndpointsAtuais().APIBaseURL+path, data)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if async {
		req.Header.Set("Accept", asyncAccept)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer res.Body.Close()
	limited := io.LimitReader(res.Body, 2<<20)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: não foi possível ler a resposta AppyPay: %v", ErrUpstream, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: AppyPay respondeu HTTP %d", ErrUpstream, res.StatusCode)
	}
	out := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("%w: resposta AppyPay inválida: %v", ErrUpstream, err)
		}
	}
	return out, nil
}
func (s *Service) token(ctx context.Context, cred credentialSecrets) (string, error) {
	s.mu.Lock()
	cached, ok := s.tokens[cred.ID]
	s.mu.Unlock()
	if ok && time.Until(cached.expiresAt) > time.Minute {
		return cached.value, nil
	}
	resource, err := appyPayResource()
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {cred.ClientID}, "client_secret": {cred.ClientSecret}, "resource": {resource}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, EndpointsAtuais().TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("%w: não foi possível ler a resposta de token: %v", ErrUpstream, err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("%w: token AppyPay recusado: HTTP %d", ErrUpstream, res.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err = json.Unmarshal(raw, &out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("%w: resposta de token AppyPay inválida", ErrUpstream)
	}
	if out.ExpiresIn <= 0 {
		out.ExpiresIn = 3600
	}
	s.mu.Lock()
	s.tokens[cred.ID] = tokenEntry{out.AccessToken, time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)}
	s.mu.Unlock()
	return out.AccessToken, nil
}

func (s *Service) record(ctx context.Context, id uuid.UUID, event string, payload map[string]any, userID, userType, ip string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("autor do evento financeiro é obrigatório")
	}
	agg := aggregates.NewFinanceiroWithID(id)
	clean, _ := sanitize(payload).(map[string]any)
	agg.Registrar(event, clean)
	if err := s.repository.WithContext(ctx).SaveWithAudit(agg, db.AuditContext{UserID: userID, UserType: userType, IP: ip}); err != nil {
		return err
	}
	return s.projection.ApplyLatestForAggregate(id)
}
func (s *Service) saveSecrets(ctx context.Context, id uuid.UUID, plain map[string]string) error {
	for typ, value := range plain {
		if value == "" {
			continue
		}
		ciphertext, err := encrypt(value)
		if err != nil {
			return err
		}
		if _, err = s.client.DB().ExecContext(ctx, `INSERT INTO financeiro_segredos_appypay (credential_id,secret_type,ciphertext) VALUES ($1,$2,$3) ON CONFLICT (credential_id,secret_type) DO UPDATE SET ciphertext=EXCLUDED.ciphertext,created_at=CURRENT_TIMESTAMP`, id, typ, ciphertext); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) loadSecrets(ctx context.Context, id uuid.UUID) (map[string]string, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT secret_type,ciphertext FROM financeiro_segredos_appypay WHERE credential_id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var typ, ciphertext string
		if err := rows.Scan(&typ, &ciphertext); err != nil {
			return nil, err
		}
		v, e := decrypt(ciphertext)
		if e != nil {
			return nil, e
		}
		out[typ] = v
	}
	for _, required := range []string{"client_id", "client_secret", "gpo_method", "ref_method"} {
		if out[required] == "" {
			return nil, errors.New("cofre de credenciais AppyPay incompleto")
		}
	}
	return out, rows.Err()
}

// AuthenticateWebhook aceita apenas o método suportado pela AppyPay: um único
// cabeçalho HTTP fixo para toda a plataforma (WebhookHeaderName) cujo valor é
// comparado, em tempo constante, ao webhook_secret gerado pelo servidor para
// cada credencial. A AppyPay confirmou por e-mail que o painel de
// configuração de webhooks só oferece um par nome/valor de cabeçalho HTTP —
// nunca query parameter, nunca corpo do POST, e nunca campos separados de
// utilizador/senha para Basic Auth. It never reveals which configured
// account matched.
type WebhookOwner struct {
	CredentialID                 uuid.UUID
	ContextoTipo, CodigoAcademia string
}

func (s *Service) AuthenticateWebhook(ctx context.Context, headers http.Header) (WebhookOwner, error) {
	candidate := headers.Get(WebhookHeaderName)
	if candidate == "" {
		return WebhookOwner{}, errors.New("webhook não autenticado")
	}
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id,payload FROM financeiro_credenciais_appypay`)
	if err != nil {
		return WebhookOwner{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return WebhookOwner{}, err
		}
		var meta struct {
			ContextoTipo   string `json:"contexto_tipo"`
			CodigoAcademia string `json:"codigo_academia"`
		}
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		secrets, err := s.loadSecrets(ctx, id)
		if err != nil {
			continue
		}
		if secrets["webhook_secret"] != "" && constantTimeEqual(candidate, secrets["webhook_secret"]) {
			return WebhookOwner{CredentialID: id, ContextoTipo: meta.ContextoTipo, CodigoAcademia: meta.CodigoAcademia}, nil
		}
	}
	return WebhookOwner{}, errors.New("webhook não autenticado")
}

// AcceptWebhook reserves its event id first in the dedicated idempotency index.
// If ledger persistence fails the reservation is removed, so a delivery retry is
// still processed. No charge side-effect is executed here.
func (s *Service) AcceptWebhook(ctx context.Context, metodo, eventID string, owner WebhookOwner, payload map[string]any) (bool, error) {
	metodo = strings.ToUpper(metodo)
	if (metodo != "GPO" && metodo != "REF") || strings.TrimSpace(eventID) == "" {
		return false, errors.New("webhook inválido")
	}
	res, err := s.client.DB().ExecContext(ctx, `INSERT INTO financeiro_webhooks_recebidos(event_id,metodo) VALUES($1,$2) ON CONFLICT(event_id) DO NOTHING`, eventID, metodo)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return false, nil
	}
	data := map[string]any{"event_id": eventID, "metodo": metodo, "credential_id": owner.CredentialID.String(), "contexto_tipo": owner.ContextoTipo, "codigo_academia": owner.CodigoAcademia, "payload": sanitize(payload)}
	if err = s.record(ctx, uuid.New(), "WebhookAppyPayRecebido", data, "appypay:webhook", "sistema", "webhook"); err != nil {
		_, _ = s.client.DB().ExecContext(ctx, `DELETE FROM financeiro_webhooks_recebidos WHERE event_id=$1`, eventID)
		return false, err
	}
	if isSuccessfulChargeStatus(responseStatus(payload)) {
		if charge, loadErr := s.loadCharge(ctx, eventID); loadErr == nil && charge.Contexto == owner.ContextoTipo && charge.Academia == owner.CodigoAcademia {
			updated := make(map[string]any, len(charge.Payload)+3)
			for k, v := range charge.Payload {
				updated[k] = v
			}
			updated["status"] = "Success"
			updated["provider_charge_id"] = first(responseID(payload), charge.ProviderID)
			updated["response"] = sanitize(payload)
			eventType := "CobrancaAppyPayConsultada"
			if strings.EqualFold(charge.Status, "cancelada") {
				// A provider may still settle a REF/GPO/QR after Spuri's local
				// cancellation. Preserve cancellation and leave an explicit audit
				// conflict for FPP reconciliation.
				updated["status"] = "cancelada"
				updated["provider_status"] = "Success"
				eventType = "CobrancaAppyPayConflitoPosCancelamento"
			}
			if s.record(ctx, charge.ID, eventType, updated, "appypay:webhook", "sistema", "webhook") == nil && eventType == "CobrancaAppyPayConsultada" {
				_ = s.confirmMensalidadeCharge(ctx, charge.ID, "appypay:webhook", "sistema", "webhook")
			}
		}
	}
	return true, nil
}

type chargeRow struct {
	ID                                               uuid.UUID
	ProviderID, Merchant, Contexto, Academia, Status string
	Payload                                          map[string]any
}

func (s *Service) loadCharge(ctx context.Context, identifier string) (chargeRow, error) {
	var r chargeRow
	var raw []byte
	err := s.client.DB().QueryRowContext(ctx, `SELECT id,COALESCE(provider_charge_id,''),merchant_transaction_id,contexto_tipo,COALESCE(codigo_academia,''),payload FROM financeiro_cobrancas WHERE id::text=$1 OR provider_charge_id=$1 OR merchant_transaction_id=$1`, identifier).Scan(&r.ID, &r.ProviderID, &r.Merchant, &r.Contexto, &r.Academia, &raw)
	if err != nil {
		return r, fmt.Errorf("%w: cobrança não encontrada", ErrNotFound)
	}
	if err = json.Unmarshal(raw, &r.Payload); err != nil {
		return r, err
	}
	r.Status, _ = r.Payload["status"].(string)
	return r, nil
}

func (s *Service) reserveCharge(ctx context.Context, merchant string, chargeID uuid.UUID) (bool, error) {
	res, err := s.client.DB().ExecContext(ctx, `INSERT INTO financeiro_cobrancas_reservas (merchant_transaction_id,charge_id) VALUES ($1,$2) ON CONFLICT (merchant_transaction_id) DO NOTHING`, merchant, chargeID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	return affected > 0, err
}

func (s *Service) releaseChargeReservation(ctx context.Context, merchant string, chargeID uuid.UUID) error {
	_, err := s.client.DB().ExecContext(ctx, `DELETE FROM financeiro_cobrancas_reservas WHERE merchant_transaction_id=$1 AND charge_id=$2`, merchant, chargeID)
	return err
}

func (s *Service) existingChargeResult(ctx context.Context, merchant, contexto, academia string) (ChargeResult, error) {
	row, err := s.loadCharge(ctx, merchant)
	if err != nil {
		return ChargeResult{}, err
	}
	if row.Contexto != contexto || row.Academia != academia {
		// merchantTransactionId is globally reserved, but its original result
		// must never be disclosed outside its financial context.
		return ChargeResult{}, ErrConflict
	}
	response, _ := row.Payload["response"].(map[string]any)
	return ChargeResult{ID: row.ID, ProviderChargeID: row.ProviderID, MerchantTransactionID: row.Merchant, Status: row.Status, Response: response}, nil
}

func (s *Service) existingQRCodeResult(ctx context.Context, merchant, contexto, academia string) (QRCodeResult, error) {
	row, err := s.loadCharge(ctx, merchant)
	if err != nil {
		return QRCodeResult{}, err
	}
	return qrCodeResultFromRow(row, contexto, academia)
}

func qrCodeResultFromRow(row chargeRow, contexto, academia string) (QRCodeResult, error) {
	if row.Contexto != contexto || row.Academia != academia {
		return QRCodeResult{}, ErrConflict
	}
	if _, ok := row.Payload["qr_code_type"].(string); !ok {
		return QRCodeResult{}, ErrConflict
	}
	response, _ := row.Payload["response"].(map[string]any)
	qr, _ := response["qrCodeArr"].(string)
	return QRCodeResult{ChargeResult: ChargeResult{ID: row.ID, ProviderChargeID: row.ProviderID, MerchantTransactionID: row.Merchant, Status: row.Status, Response: response}, QRCodeArr: qr}, nil
}

func sameJSON(a, b any) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}
func validateCharge(in *ChargeRequest) error {
	if err := validContext(in.ContextoTipo, in.CodigoAcademia); err != nil {
		return err
	}
	if !validAmount(in.Amount) || strings.TrimSpace(in.Description) == "" {
		return errors.New("amount e description são obrigatórios")
	}
	if roundAmount(in.Amount) != in.Amount {
		return errors.New("amount deve ter no máximo duas casas decimais")
	}
	if in.Currency == "" {
		in.Currency = "AOA"
	}
	if in.MerchantTransactionID != "" && !validMerchantID(in.MerchantTransactionID) {
		return errors.New("merchantTransactionId deve ser alfanumérico e ter no máximo 15 caracteres")
	}
	if strings.ToUpper(in.Currency) != "AOA" {
		return errors.New("currency deve ser AOA")
	}
	if len(in.Options) > 2 {
		return errors.New("options aceita no máximo duas chaves")
	}
	m := strings.ToUpper(in.PaymentMethod)
	if m != "GPO" && m != "REF" && !strings.HasPrefix(m, "GPO_") && !strings.HasPrefix(m, "REF_") {
		return errors.New("método de pagamento fora do escopo")
	}
	phone, phoneOK := in.PaymentInfo["phoneNumber"].(string)
	if strings.HasPrefix(m, "GPO") && (!phoneOK || strings.TrimSpace(phone) == "") {
		return errors.New("GPO exige paymentInfo.phoneNumber")
	}
	if strings.HasPrefix(m, "REF") && len(in.PaymentInfo) > 0 {
		for _, k := range []string{"referenceNumber", "dueDate", "nib"} {
			value, ok := in.PaymentInfo[k].(string)
			if !ok || strings.TrimSpace(value) == "" {
				return fmt.Errorf("REF com paymentInfo exige %s", k)
			}
		}
	}
	return nil
}

func validateQRCode(in *QRCodeRequest) error {
	if err := validContext(in.ContextoTipo, in.CodigoAcademia); err != nil {
		return err
	}
	if !validAmount(in.Amount) || strings.TrimSpace(in.Description) == "" {
		return errors.New("amount e description são obrigatórios")
	}
	if roundAmount(in.Amount) != in.Amount {
		return errors.New("amount deve ter no máximo duas casas decimais")
	}
	if in.Currency == "" {
		in.Currency = "AOA"
	}
	if !strings.EqualFold(in.Currency, "AOA") {
		return errors.New("currency deve ser AOA")
	}
	if in.MinAmount != nil {
		if !validAmount(*in.MinAmount) {
			return errors.New("minAmount deve ser maior que zero")
		}
		if roundAmount(*in.MinAmount) != *in.MinAmount {
			return errors.New("minAmount deve ter no máximo duas casas decimais")
		}
	}
	return nil
}

func validAmount(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 }
func merchantID() string {
	b := make([]byte, 7)
	_, _ = rand.Read(b)
	return "T" + strings.ToUpper(hex.EncodeToString(b))
}
func validMerchantID(value string) bool {
	if len(value) == 0 || len(value) > 15 {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func chargePayload(id uuid.UUID, in ChargeRequest, providerID, status string, response map[string]any) map[string]any {
	return map[string]any{"charge_id": id.String(), "contexto_tipo": in.ContextoTipo, "codigo_academia": in.CodigoAcademia, "codigo_estudante": in.CodigoEstudante, "codigo_solicitacao": in.CodigoSolicitacao, "mensalidades": in.Mensalidades, "amount": roundAmount(in.Amount), "currency": in.Currency, "description": in.Description, "merchant_transaction_id": in.MerchantTransactionID, "payment_method": in.PaymentMethod, "payment_info": in.PaymentInfo, "options": in.Options, "notify": in.Notify, "async": in.Async, "provider_charge_id": providerID, "status": status, "response": sanitize(response)}
}
func qrCodePayload(id uuid.UUID, in QRCodeRequest, typ, providerID, status string, response map[string]any) map[string]any {
	var minAmount any
	if in.MinAmount != nil {
		minAmount = roundAmount(*in.MinAmount)
	}
	return map[string]any{"charge_id": id.String(), "contexto_tipo": in.ContextoTipo, "codigo_academia": in.CodigoAcademia, "codigo_estudante": in.CodigoEstudante, "codigo_solicitacao": in.CodigoSolicitacao, "amount": roundAmount(in.Amount), "currency": in.Currency, "description": in.Description, "merchant_transaction_id": in.MerchantTransactionID, "provider_charge_id": providerID, "status": status, "payment_method": "GPO", "qr_code_type": typ, "mensalidades": in.Mensalidades, "min_amount": minAmount, "max_transactions": in.MaxTransactions, "start_date": in.StartDate, "end_date": in.EndDate, "response": sanitize(response)}
}
func responseID(v map[string]any) string {
	for _, k := range []string{"id", "chargeId", "charge_id"} {
		if x, ok := v[k].(string); ok {
			return x
		}
	}
	return ""
}
func responseStatus(v map[string]any) string {
	for _, k := range []string{"status", "state"} {
		if x, ok := v[k].(string); ok {
			return x
		}
	}
	return ""
}
func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func sanitize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			l := strings.ToLower(k)
			if strings.Contains(l, "secret") || strings.Contains(l, "token") || strings.Contains(l, "apikey") || strings.Contains(l, "api_key") || strings.Contains(l, "authorization") {
				continue
			}
			out[k] = sanitize(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i := range x {
			out[i] = sanitize(x[i])
		}
		return out
	default:
		return v
	}
}
func key() ([]byte, error) {
	v := strings.TrimSpace(os.Getenv("FINANCE_ENCRYPTION_KEY"))
	if v == "" {
		return nil, errors.New("FINANCE_ENCRYPTION_KEY é obrigatória")
	}
	if decoded, err := base64.StdEncoding.DecodeString(v); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(v) < 32 {
		return nil, errors.New("FINANCE_ENCRYPTION_KEY deve ter pelo menos 32 caracteres ou ser Base64 de 32 bytes")
	}
	sum := sha256.Sum256([]byte(v))
	return sum[:], nil
}

// ValidateEncryptionConfig validates the mandatory financial-secret key at
// startup, before a request can attempt to persist any credentials.
func ValidateEncryptionConfig() error {
	_, err := key()
	return err
}
func encrypt(v string) (string, error) {
	k, err := key()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(append(nonce, gcm.Seal(nil, nonce, []byte(v), nil)...)), nil
}
func decrypt(v string) (string, error) {
	k, err := key()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(v)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext inválido")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), err
}
func constantTimeEqual(a, b string) bool { return hmac.Equal([]byte(a), []byte(b)) }
