// Package finance is the only package allowed to call AppyPay's HTTP API.
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
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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

type CredentialInput struct {
	ContextoTipo     string `json:"contexto_tipo"`
	CodigoAcademia   string `json:"codigo_academia,omitempty"`
	Ambiente         string `json:"ambiente"`
	ClientID         string `json:"client_id"`
	ClientSecret     string `json:"client_secret"`
	Resource         string `json:"resource"`
	GPOPaymentMethod string `json:"gpo_payment_method"`
	REFPaymentMethod string `json:"ref_payment_method"`
	WebhookAuthType  string `json:"webhook_auth_type,omitempty"` // basic or api_key
	WebhookUsername  string `json:"webhook_username,omitempty"`
	WebhookSecret    string `json:"webhook_secret,omitempty"`
}
type CredentialView struct {
	ID                   uuid.UUID `json:"id"`
	ContextoTipo         string    `json:"contexto_tipo"`
	CodigoAcademia       string    `json:"codigo_academia,omitempty"`
	Ambiente             string    `json:"ambiente"`
	ClientIDMask         string    `json:"client_id_mask"`
	ResourceMask         string    `json:"resource_mask"`
	GPOPaymentMethodMask string    `json:"gpo_payment_method_mask"`
	REFPaymentMethodMask string    `json:"ref_payment_method_mask"`
	WebhookAuthType      string    `json:"webhook_auth_type,omitempty"`
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

func mask(v string) string {
	if len(v) <= 4 {
		return "****"
	}
	return "****" + v[len(v)-4:]
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
func (s *Service) ConfigureCredential(ctx context.Context, id *uuid.UUID, in CredentialInput, userID, userType, ip string) (CredentialView, error) {
	if s.client == nil {
		return CredentialView{}, errors.New("serviço financeiro não inicializado")
	}
	in.ContextoTipo = strings.ToLower(strings.TrimSpace(in.ContextoTipo))
	in.Ambiente = strings.ToLower(strings.TrimSpace(in.Ambiente))
	if err := validContext(in.ContextoTipo, in.CodigoAcademia); err != nil {
		return CredentialView{}, err
	}
	if in.Ambiente == "" {
		in.Ambiente = AmbienteAtual()
	}
	if in.Ambiente != AmbienteAtual() {
		return CredentialView{}, errors.New("credenciais devem pertencer ao ambiente ativo")
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(in.ClientID) == "" || strings.TrimSpace(in.ClientSecret) == "" || strings.TrimSpace(in.Resource) == "" || !strings.HasPrefix(in.GPOPaymentMethod, "GPO_") || !strings.HasPrefix(in.REFPaymentMethod, "REF_") {
		return CredentialView{}, errors.New("credenciais AppyPay incompletas ou inválidas")
	}
	if in.WebhookAuthType != "" && in.WebhookAuthType != "basic" && in.WebhookAuthType != "api_key" {
		return CredentialView{}, errors.New("webhook_auth_type inválido")
	}
	if in.WebhookAuthType == "basic" && (in.WebhookUsername == "" || in.WebhookSecret == "") {
		return CredentialView{}, errors.New("Basic Auth de webhook exige utilizador e segredo")
	}
	if in.WebhookAuthType == "api_key" && in.WebhookSecret == "" {
		return CredentialView{}, errors.New("API Key de webhook exige segredo")
	}
	credentialID := uuid.New()
	if id != nil {
		credentialID = *id
		if err := s.credentialBelongsToScope(ctx, credentialID, in.ContextoTipo, in.CodigoAcademia, in.Ambiente); err != nil {
			return CredentialView{}, err
		}
	} else if found, err := s.findCredentialID(ctx, in.ContextoTipo, in.CodigoAcademia, in.Ambiente); err == nil {
		credentialID = found
	}
	view := CredentialView{ID: credentialID, ContextoTipo: in.ContextoTipo, CodigoAcademia: in.CodigoAcademia, Ambiente: in.Ambiente, ClientIDMask: mask(in.ClientID), ResourceMask: mask(in.Resource), GPOPaymentMethodMask: mask(in.GPOPaymentMethod), REFPaymentMethodMask: mask(in.REFPaymentMethod), WebhookAuthType: in.WebhookAuthType, UpdatedAt: time.Now().UTC()}
	payload := map[string]any{"credential_id": credentialID.String(), "contexto_tipo": view.ContextoTipo, "codigo_academia": view.CodigoAcademia, "ambiente": view.Ambiente, "client_id_mask": view.ClientIDMask, "resource_mask": view.ResourceMask, "gpo_payment_method_mask": view.GPOPaymentMethodMask, "ref_payment_method_mask": view.REFPaymentMethodMask, "webhook_auth_type": view.WebhookAuthType, "updated_at": view.UpdatedAt}
	if err := s.record(ctx, credentialID, "CredenciaisAppyPayConfiguradas", payload, userID, userType, ip); err != nil {
		return CredentialView{}, err
	}
	if err := s.saveSecrets(ctx, credentialID, map[string]string{"client_id": in.ClientID, "client_secret": in.ClientSecret, "resource": in.Resource, "gpo_method": in.GPOPaymentMethod, "ref_method": in.REFPaymentMethod, "webhook_username": in.WebhookUsername, "webhook_secret": in.WebhookSecret}); err != nil {
		return CredentialView{}, err
	}
	return view, nil
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
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) CreateCharge(ctx context.Context, in ChargeRequest, actorID, actorType, ip string) (ChargeResult, error) {
	if err := validateCharge(&in); err != nil {
		return ChargeResult{}, err
	}
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
	return ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}, nil
}
func (s *Service) CreateGPOQRCode(ctx context.Context, in QRCodeRequest, actorID, actorType, ip string) (QRCodeResult, error) {
	if in.Currency == "" {
		in.Currency = "AOA"
	}
	if in.Amount <= 0 || strings.TrimSpace(in.Description) == "" {
		return QRCodeResult{}, errors.New("amount e description são obrigatórios")
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
	body := map[string]any{"amount": in.Amount, "currency": in.Currency, "description": in.Description, "merchantTransactionId": in.MerchantTransactionID, "paymentMethod": cred.GPO, "qrCodeType": typ}
	if typ == "MULTIPLE" {
		body["minAmount"] = *in.MinAmount
		body["maxTransactions"] = *in.MaxTransactions
		body["startDate"] = in.StartDate
		body["endDate"] = in.EndDate
	}
	response, err := s.callJSON(ctx, cred, http.MethodPost, "/qr-codes", body, false)
	if err != nil {
		return QRCodeResult{}, err
	}
	providerID := responseID(response)
	status := responseStatus(response)
	if status == "" {
		status = "criada"
	}
	payload := map[string]any{"charge_id": id.String(), "contexto_tipo": in.ContextoTipo, "codigo_academia": in.CodigoAcademia, "merchant_transaction_id": in.MerchantTransactionID, "provider_charge_id": providerID, "status": status, "payment_method": "GPO", "qr_code_type": typ, "response": sanitize(response)}
	if err = s.record(ctx, id, "QRCodeAppyPayGerado", payload, actorID, actorType, ip); err != nil {
		return QRCodeResult{}, err
	}
	qr, _ := response["qrCodeArr"].(string)
	return QRCodeResult{ChargeResult: ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}, QRCodeArr: qr}, nil
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
	cred, err := s.loadCredential(ctx, contexto, academia)
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
	if status != row.Status || providerID != row.ProviderID || !sameJSON(payload["response"], previousResponse) {
		if err = s.record(ctx, row.ID, "CobrancaAppyPayConsultada", payload, actorID, actorType, ip); err != nil {
			return ChargeResult{}, err
		}
	}
	return ChargeResult{ID: row.ID, ProviderChargeID: providerID, MerchantTransactionID: row.Merchant, Status: status, Response: response}, nil
}

type credentialSecrets struct {
	ID                                         uuid.UUID
	ClientID, ClientSecret, Resource, GPO, REF string
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
	return credentialSecrets{ID: id, ClientID: values["client_id"], ClientSecret: values["client_secret"], Resource: values["resource"], GPO: values["gpo_method"], REF: values["ref_method"]}, nil
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
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {cred.ClientID}, "client_secret": {cred.ClientSecret}, "resource": {cred.Resource}}
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
	for _, required := range []string{"client_id", "client_secret", "resource", "gpo_method", "ref_method"} {
		if out[required] == "" {
			return nil, errors.New("cofre de credenciais AppyPay incompleto")
		}
	}
	return out, rows.Err()
}

// AuthenticateWebhook accepts either the configured HTTP Basic credentials or
// an X-API-Key. It never reveals which configured account matched.
type WebhookOwner struct {
	CredentialID                 uuid.UUID
	ContextoTipo, CodigoAcademia string
}

func (s *Service) AuthenticateWebhook(ctx context.Context, basicUser, basicPassword, apiKey string) (WebhookOwner, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id,payload FROM financeiro_credenciais_appypay WHERE payload->>'webhook_auth_type' IN ('basic','api_key')`)
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
			WebhookAuthType string `json:"webhook_auth_type"`
			ContextoTipo    string `json:"contexto_tipo"`
			CodigoAcademia  string `json:"codigo_academia"`
		}
		if json.Unmarshal(raw, &meta) != nil {
			continue
		}
		secrets, err := s.loadSecrets(ctx, id)
		if err != nil {
			continue
		}
		if meta.WebhookAuthType == "basic" && basicUser != "" && constantTimeEqual(basicUser, secrets["webhook_username"]) && constantTimeEqual(basicPassword, secrets["webhook_secret"]) {
			return WebhookOwner{CredentialID: id, ContextoTipo: meta.ContextoTipo, CodigoAcademia: meta.CodigoAcademia}, nil
		}
		if meta.WebhookAuthType == "api_key" && apiKey != "" && constantTimeEqual(apiKey, secrets["webhook_secret"]) {
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
	err := s.client.DB().QueryRowContext(ctx, `SELECT id,COALESCE(provider_charge_id,''),merchant_transaction_id,contexto_tipo,COALESCE(codigo_academia,''),payload FROM financeiro_cobrancas WHERE provider_charge_id=$1 OR merchant_transaction_id=$1`, identifier).Scan(&r.ID, &r.ProviderID, &r.Merchant, &r.Contexto, &r.Academia, &raw)
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

func sameJSON(a, b any) bool {
	left, leftErr := json.Marshal(a)
	right, rightErr := json.Marshal(b)
	return leftErr == nil && rightErr == nil && bytes.Equal(left, right)
}
func validateCharge(in *ChargeRequest) error {
	if err := validContext(in.ContextoTipo, in.CodigoAcademia); err != nil {
		return err
	}
	if in.Amount <= 0 || strings.TrimSpace(in.Description) == "" {
		return errors.New("amount e description são obrigatórios")
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
	return map[string]any{"charge_id": id.String(), "contexto_tipo": in.ContextoTipo, "codigo_academia": in.CodigoAcademia, "amount": in.Amount, "currency": in.Currency, "description": in.Description, "merchant_transaction_id": in.MerchantTransactionID, "payment_method": in.PaymentMethod, "payment_info": in.PaymentInfo, "options": in.Options, "notify": in.Notify, "async": in.Async, "provider_charge_id": providerID, "status": status, "response": sanitize(response)}
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
