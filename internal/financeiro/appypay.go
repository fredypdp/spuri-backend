// Package financeiro contém a integração base (e agnóstica de negócio) com AppyPay.
package financeiro

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
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
	"github.com/jmoiron/sqlx"

	"spuri/internal/db"
)

const (
	contextSpuri    = "spuri"
	contextAcademia = "academia"
	methodGPO       = "GPO"
	methodREF       = "REF"
)

type Environment struct{ TokenURL, APIBaseURL, Name string }

// CurrentEnvironment centraliza a escolha TEST/PROD a partir de ENV.
func CurrentEnvironment() Environment {
	production := strings.EqualFold(strings.TrimSpace(os.Getenv("ENV")), "production")
	if production {
		return Environment{"https://login.microsoftonline.com/auth.appypay.co.ao/oauth2/token", apiBase("https://gwy-api.appypay.co.ao"), "PROD"}
	}
	return Environment{"https://login.microsoftonline.com/appypaydev.onmicrosoft.com/oauth2/token", apiBase("https://gwy-api-tst.appypay.co.ao"), "TEST"}
}

func apiBase(host string) string {
	version := strings.TrimSpace(os.Getenv("APPYPAY_API_VERSION"))
	if version == "" {
		version = "v2.0"
	}
	return host + "/" + version
}

type Scope struct {
	Type         string `json:"type"`
	AcademiaCode string `json:"academia_code,omitempty"`
}

func SpuriScope() Scope { return Scope{Type: contextSpuri} }
func AcademiaScope(code string) Scope {
	return Scope{Type: contextAcademia, AcademiaCode: strings.TrimSpace(code)}
}
func (s Scope) validate() error {
	if s.Type == contextSpuri && s.AcademiaCode == "" {
		return nil
	}
	if s.Type == contextAcademia && s.AcademiaCode != "" {
		return nil
	}
	return errors.New("escopo financeiro inválido")
}

type CredentialInput struct {
	ClientID        string `json:"client_id,omitempty"`
	ClientSecret    string `json:"client_secret,omitempty"`
	Resource        string `json:"resource,omitempty"`
	GPOMethod       string `json:"gpo_payment_method,omitempty"`
	REFMethod       string `json:"ref_payment_method,omitempty"`
	WebhookAuthType string `json:"webhook_auth_type,omitempty"` // basic ou api_key
	WebhookUsername string `json:"webhook_username,omitempty"`
	WebhookSecret   string `json:"webhook_secret,omitempty"`
}

type CredentialView struct {
	ID              uuid.UUID `json:"id"`
	Scope           Scope     `json:"scope"`
	Environment     string    `json:"environment"`
	ClientID        string    `json:"client_id,omitempty"`
	Resource        string    `json:"resource,omitempty"`
	GPOMethod       string    `json:"gpo_payment_method,omitempty"`
	REFMethod       string    `json:"ref_payment_method,omitempty"`
	WebhookAuthType string    `json:"webhook_auth_type,omitempty"`
	WebhookUsername string    `json:"webhook_username,omitempty"`
	WebhookSecret   string    `json:"webhook_secret,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type credential struct {
	CredentialView
	clientID, clientSecret, resource, gpo, ref, webhookUser, webhookSecret string
}
type tokenCache struct {
	value     string
	expiresAt time.Time
}

type Service struct {
	client     *db.Client
	httpClient *http.Client
	now        func() time.Time
	mu         sync.Mutex
	tokens     map[uuid.UUID]tokenCache
}

func NewService(client *db.Client) *Service {
	return &Service{client: client, httpClient: &http.Client{Timeout: 20 * time.Second}, now: time.Now, tokens: make(map[uuid.UUID]tokenCache)}
}

func (s *Service) ConfigureCredentials(ctx context.Context, scope Scope, in CredentialInput, actor db.AuditContext) (*CredentialView, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.ClientID) == "" || strings.TrimSpace(in.ClientSecret) == "" || strings.TrimSpace(in.Resource) == "" || strings.TrimSpace(in.GPOMethod) == "" || strings.TrimSpace(in.REFMethod) == "" {
		return nil, errors.New("client_id, client_secret, resource, gpo_payment_method e ref_payment_method são obrigatórios")
	}
	if !strings.HasPrefix(in.GPOMethod, "GPO_") || !strings.HasPrefix(in.REFMethod, "REF_") {
		return nil, errors.New("os métodos devem ter os prefixos GPO_ e REF_")
	}
	in.WebhookAuthType = strings.ToLower(strings.TrimSpace(in.WebhookAuthType))
	if in.WebhookAuthType != "" && in.WebhookAuthType != "basic" && in.WebhookAuthType != "api_key" {
		return nil, errors.New("webhook_auth_type deve ser basic ou api_key")
	}
	if in.WebhookAuthType == "basic" && (in.WebhookUsername == "" || in.WebhookSecret == "") {
		return nil, errors.New("webhook basic exige username e secret")
	}
	if in.WebhookAuthType == "api_key" && in.WebhookSecret == "" {
		return nil, errors.New("webhook api_key exige secret")
	}

	values := map[string]string{"client_id": in.ClientID, "client_secret": in.ClientSecret, "resource": in.Resource, "gpo_method": in.GPOMethod, "ref_method": in.REFMethod, "webhook_username": in.WebhookUsername, "webhook_secret": in.WebhookSecret}
	ciphertexts := make(map[string]string, len(values))
	for k, v := range values {
		if v != "" {
			c, err := encrypt(v)
			if err != nil {
				return nil, err
			}
			ciphertexts[k] = c
		}
	}
	env := CurrentEnvironment()
	tx, err := s.client.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var existingID uuid.UUID
	err = tx.QueryRowContext(ctx, `SELECT id FROM financeiro_credenciais_appypay WHERE contexto_tipo=$1 AND codigo_academia=$2 AND ambiente=$3`, scope.Type, scope.AcademiaCode, env.Name).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		existingID = uuid.New()
	}
	payload := map[string]any{"credential_id": existingID, "contexto_tipo": scope.Type, "codigo_academia": scope.AcademiaCode, "ambiente": env.Name, "client_id_mask": mask(in.ClientID), "resource_mask": mask(in.Resource), "gpo_method_mask": mask(in.GPOMethod), "ref_method_mask": mask(in.REFMethod), "webhook_auth_type": in.WebhookAuthType, "webhook_secret_mask": mask(in.WebhookSecret)}
	eventID, err := s.writeEventTx(ctx, tx, existingID, "CredenciaisAppyPayConfiguradas", payload, actor)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO financeiro_credenciais_appypay (id,contexto_tipo,codigo_academia,ambiente,client_id_mascarado,resource_mascarado,gpo_method_mascarado,ref_method_mascarado,webhook_auth_type,webhook_username_mascarado,webhook_secret_mascarado,version,last_event_id)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),1,$12)
	ON CONFLICT (contexto_tipo,codigo_academia,ambiente) DO UPDATE SET client_id_mascarado=EXCLUDED.client_id_mascarado,resource_mascarado=EXCLUDED.resource_mascarado,gpo_method_mascarado=EXCLUDED.gpo_method_mascarado,ref_method_mascarado=EXCLUDED.ref_method_mascarado,webhook_auth_type=EXCLUDED.webhook_auth_type,webhook_username_mascarado=EXCLUDED.webhook_username_mascarado,webhook_secret_mascarado=EXCLUDED.webhook_secret_mascarado,version=financeiro_credenciais_appypay.version+1,last_event_id=EXCLUDED.last_event_id,updated_at=CURRENT_TIMESTAMP`, existingID, scope.Type, scope.AcademiaCode, env.Name, mask(in.ClientID), mask(in.Resource), mask(in.GPOMethod), mask(in.REFMethod), in.WebhookAuthType, mask(in.WebhookUsername), mask(in.WebhookSecret), eventID)
	if err != nil {
		return nil, err
	}
	for typ, ciphertext := range ciphertexts {
		if _, err = tx.ExecContext(ctx, `UPDATE financeiro_segredos_appypay SET revoked_at=CURRENT_TIMESTAMP WHERE credential_id=$1 AND secret_type=$2 AND revoked_at IS NULL`, existingID, typ); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO financeiro_segredos_appypay (credential_id,secret_version,secret_type,ciphertext) VALUES ($1,COALESCE((SELECT MAX(secret_version)+1 FROM financeiro_segredos_appypay WHERE credential_id=$1 AND secret_type=$2),1),$2,$3)`, existingID, typ, ciphertext); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	delete(s.tokens, existingID)
	s.mu.Unlock()
	return s.GetCredential(ctx, scope)
}

func (s *Service) GetCredential(ctx context.Context, scope Scope) (*CredentialView, error) {
	c, err := s.loadCredential(ctx, scope, false)
	if err != nil {
		return nil, err
	}
	return &c.CredentialView, nil
}
func (s *Service) loadCredential(ctx context.Context, scope Scope, secrets bool) (*credential, error) {
	if err := scope.validate(); err != nil {
		return nil, err
	}
	env := CurrentEnvironment()
	var c credential
	err := s.client.DB().QueryRowContext(ctx, `SELECT id,contexto_tipo,codigo_academia,ambiente,client_id_mascarado,resource_mascarado,gpo_method_mascarado,ref_method_mascarado,COALESCE(webhook_auth_type,''),COALESCE(webhook_username_mascarado,''),COALESCE(webhook_secret_mascarado,''),updated_at FROM financeiro_credenciais_appypay WHERE contexto_tipo=$1 AND codigo_academia=$2 AND ambiente=$3`, scope.Type, scope.AcademiaCode, env.Name).Scan(&c.ID, &c.Scope.Type, &c.Scope.AcademiaCode, &c.Environment, &c.ClientID, &c.Resource, &c.GPOMethod, &c.REFMethod, &c.WebhookAuthType, &c.WebhookUsername, &c.WebhookSecret, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("credenciais AppyPay não configuradas para este escopo e ambiente")
	}
	if err != nil {
		return nil, err
	}
	if !secrets {
		return &c, nil
	}
	secretPtrs := map[string]*string{"client_id": &c.clientID, "client_secret": &c.clientSecret, "resource": &c.resource, "gpo_method": &c.gpo, "ref_method": &c.ref, "webhook_username": &c.webhookUser, "webhook_secret": &c.webhookSecret}
	for typ, dst := range secretPtrs {
		var ct string
		err = s.client.DB().QueryRowContext(ctx, `SELECT ciphertext FROM financeiro_segredos_appypay WHERE credential_id=$1 AND secret_type=$2 AND revoked_at IS NULL ORDER BY secret_version DESC LIMIT 1`, c.ID, typ).Scan(&ct)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		plain, e := decrypt(ct)
		if e != nil {
			return nil, e
		}
		*dst = plain
	}
	if c.clientID == "" || c.clientSecret == "" || c.resource == "" || c.gpo == "" || c.ref == "" {
		return nil, errors.New("credenciais AppyPay incompletas")
	}
	return &c, nil
}

type ChargeRequest struct {
	Amount                float64           `json:"amount"`
	Currency              string            `json:"currency,omitempty"`
	Description           string            `json:"description,omitempty"`
	MerchantTransactionID string            `json:"merchantTransactionId,omitempty"`
	PaymentMethod         string            `json:"paymentMethod,omitempty"`
	PaymentInfo           map[string]any    `json:"paymentInfo,omitempty"`
	Options               map[string]string `json:"options,omitempty"`
	Notify                map[string]any    `json:"notify,omitempty"`
	Async                 bool              `json:"async"`
}
type QRCodeRequest struct {
	Amount                float64    `json:"amount"`
	Currency              string     `json:"currency,omitempty"`
	Description           string     `json:"description,omitempty"`
	MerchantTransactionID string     `json:"merchantTransactionId,omitempty"`
	QRCodeType            string     `json:"qrCodeType,omitempty"`
	MinAmount             *float64   `json:"minAmount,omitempty"`
	MaxTransactions       *int       `json:"maxTransactions,omitempty"`
	StartDate             *time.Time `json:"startDate,omitempty"`
	EndDate               *time.Time `json:"endDate,omitempty"`
}
type ChargeResult struct {
	ID                    string         `json:"id,omitempty"`
	MerchantTransactionID string         `json:"merchantTransactionId,omitempty"`
	Status                string         `json:"status,omitempty"`
	HTTPStatus            int            `json:"-"`
	Data                  map[string]any `json:"data"`
}

func (s *Service) CreateCharge(ctx context.Context, scope Scope, req ChargeRequest, actor db.AuditContext) (*ChargeResult, error) {
	c, err := s.loadCredential(ctx, scope, true)
	if err != nil {
		return nil, err
	}
	method := methodFromPayment(req.PaymentMethod)
	if method == "" {
		return nil, errors.New("paymentMethod deve começar com GPO_ ou REF_")
	}
	if req.Amount <= 0 {
		return nil, errors.New("amount deve ser maior que zero")
	}
	if req.Currency == "" {
		req.Currency = "AOA"
	}
	if len(req.Options) > 2 {
		return nil, errors.New("options aceita no máximo 2 chaves")
	}
	if req.MerchantTransactionID == "" {
		req.MerchantTransactionID = newMerchantID()
	}
	if len(req.MerchantTransactionID) > 15 || !alnum(req.MerchantTransactionID) {
		return nil, errors.New("merchantTransactionId deve ser alfanumérico com até 15 caracteres")
	}
	if method == methodGPO {
		if req.PaymentInfo == nil || strings.TrimSpace(fmt.Sprint(req.PaymentInfo["phoneNumber"])) == "" {
			return nil, errors.New("GPO exige paymentInfo.phoneNumber")
		}
		req.PaymentMethod = c.gpo
	} else {
		req.PaymentMethod = c.ref
	}
	chargeID := uuid.New()
	body := chargeBody(req)
	payload := map[string]any{"charge_id": chargeID, "credential_id": c.ID, "contexto_tipo": scope.Type, "codigo_academia": scope.AcademiaCode, "merchant_transaction_id": req.MerchantTransactionID, "metodo": method, "request": body}
	if err = s.persistCharge(ctx, chargeID, c.ID, scope, req.MerchantTransactionID, method, "solicitada", body, map[string]any{}, "CobrancaFinanceiraSolicitada", payload, actor); err != nil {
		return nil, err
	}
	data, status, err := s.appypayJSON(ctx, c, "POST", "/charges", body, req.Async)
	if err != nil {
		_ = s.persistCharge(ctx, chargeID, c.ID, scope, req.MerchantTransactionID, method, "erro", body, map[string]any{"error": err.Error()}, "CobrancaFinanceiraStatusAtualizado", map[string]any{"charge_id": chargeID, "status": "erro"}, actor)
		return nil, err
	}
	result := resultFrom(data, status, req.MerchantTransactionID)
	if err = s.persistCharge(ctx, chargeID, c.ID, scope, req.MerchantTransactionID, method, result.Status, body, data, "CobrancaFinanceiraCriada", map[string]any{"charge_id": chargeID, "appypay_charge_id": result.ID, "status": result.Status, "response": data}, actor); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) CreateGPOQRCode(ctx context.Context, scope Scope, req QRCodeRequest, actor db.AuditContext) (*ChargeResult, error) {
	c, err := s.loadCredential(ctx, scope, true)
	if err != nil {
		return nil, err
	}
	if req.Amount <= 0 {
		return nil, errors.New("amount deve ser maior que zero")
	}
	if req.Currency == "" {
		req.Currency = "AOA"
	}
	if req.MerchantTransactionID == "" {
		req.MerchantTransactionID = newMerchantID()
	}
	if len(req.MerchantTransactionID) > 15 || !alnum(req.MerchantTransactionID) {
		return nil, errors.New("merchantTransactionId inválido")
	}
	if req.QRCodeType == "" {
		req.QRCodeType = "SINGLE"
	}
	req.QRCodeType = strings.ToUpper(req.QRCodeType)
	if req.QRCodeType != "SINGLE" && req.QRCodeType != "MULTIPLE" {
		return nil, errors.New("qrCodeType deve ser SINGLE ou MULTIPLE")
	}
	if req.QRCodeType == "MULTIPLE" && (req.MinAmount == nil || req.MaxTransactions == nil || req.StartDate == nil || req.EndDate == nil) {
		return nil, errors.New("MULTIPLE exige minAmount, maxTransactions, startDate e endDate")
	}
	body := map[string]any{"amount": req.Amount, "currency": req.Currency, "description": req.Description, "merchantTransactionId": req.MerchantTransactionID, "paymentMethod": c.gpo, "qrCodeType": req.QRCodeType}
	if req.MinAmount != nil {
		body["minAmount"] = *req.MinAmount
	}
	if req.MaxTransactions != nil {
		body["maxTransactions"] = *req.MaxTransactions
	}
	if req.StartDate != nil {
		body["startDate"] = req.StartDate.UTC().Format(time.RFC3339)
	}
	if req.EndDate != nil {
		body["endDate"] = req.EndDate.UTC().Format(time.RFC3339)
	}
	data, status, err := s.appypayJSON(ctx, c, "POST", "/qr-codes", body, false)
	if err != nil {
		return nil, err
	}
	result := resultFrom(data, status, req.MerchantTransactionID)
	eventPayload := map[string]any{"credential_id": c.ID, "merchant_transaction_id": req.MerchantTransactionID, "qr_code_id": result.ID, "status": result.Status}
	if err = s.writeEvent(ctx, uuid.New(), "QRCodeGPOGerado", eventPayload, actor); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) GetCharge(ctx context.Context, scope Scope, idOrMerchantID string, actor db.AuditContext) (*ChargeResult, error) {
	c, err := s.loadCredential(ctx, scope, true)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(idOrMerchantID) == "" {
		return nil, errors.New("id da cobrança é obrigatório")
	}
	path := "/charges/" + url.PathEscape(idOrMerchantID)
	// Quem só possui merchantTransactionId também pode consultar a cobrança.
	// Preferimos o id do provider já conhecido; sem ele, usamos o filtro oficial.
	var providerID string
	lookupErr := s.client.DB().QueryRowContext(ctx, `SELECT COALESCE(appypay_charge_id,'') FROM financeiro_cobrancas WHERE credential_id=$1 AND merchant_transaction_id=$2`, c.ID, idOrMerchantID).Scan(&providerID)
	if lookupErr == nil && providerID != "" {
		path = "/charges/" + url.PathEscape(providerID)
	} else if lookupErr == nil {
		path = "/charges?merchantTransactionId=" + url.QueryEscape(idOrMerchantID)
	}
	data, status, err := s.appypayJSON(ctx, c, "GET", path, nil, false)
	if err != nil {
		return nil, err
	}
	result := resultFrom(data, status, idOrMerchantID)
	var chargeID uuid.UUID
	_ = s.client.DB().QueryRowContext(ctx, `SELECT id FROM financeiro_cobrancas WHERE appypay_charge_id=$1 OR merchant_transaction_id=$1`, idOrMerchantID).Scan(&chargeID)
	if chargeID != uuid.Nil {
		_ = s.persistCharge(ctx, chargeID, c.ID, scope, result.MerchantTransactionID, "", result.Status, nil, data, "CobrancaFinanceiraStatusAtualizado", map[string]any{"charge_id": chargeID, "status": result.Status, "origem": "consulta", "response": data}, actor)
	}
	return result, nil
}

func (s *Service) ReceiveWebhook(ctx context.Context, method string, header http.Header, body map[string]any) (bool, error) {
	method = strings.ToUpper(method)
	if method != methodGPO && method != methodREF {
		return false, errors.New("método de webhook inválido")
	}
	c, err := s.webhookCredential(ctx, header)
	if err != nil {
		return false, err
	}
	key := stringValue(body, "id")
	if key == "" {
		key = stringValue(body, "merchantTransactionId")
	}
	if key == "" {
		return false, errors.New("webhook sem id ou merchantTransactionId")
	}
	eventKey := method + ":" + key
	clean := sanitize(body)
	tx, err := s.client.BeginTx(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var chargeID *uuid.UUID
	var found uuid.UUID
	_ = tx.QueryRowContext(ctx, `SELECT id FROM financeiro_cobrancas WHERE appypay_charge_id=$1 OR merchant_transaction_id=$1`, key).Scan(&found)
	if found != uuid.Nil {
		chargeID = &found
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO financeiro_webhooks_recebidos(event_key,metodo,credential_id,cobranca_id,payload) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, eventKey, method, c.ID, chargeID, toJSON(clean))
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if err = s.writeEventTxNoID(ctx, tx, uuid.New(), "WebhookFinanceiroIgnoradoComoDuplicado", map[string]any{"event_key": eventKey, "metodo": method}, db.AuditContext{UserType: "appypay_webhook"}); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if chargeID != nil {
		status := stringValue(body, "status")
		if status != "" {
			_, err = tx.ExecContext(ctx, `UPDATE financeiro_cobrancas SET status=$1, response=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$3`, status, toJSON(clean), *chargeID)
			if err != nil {
				return false, err
			}
		}
	}
	if err = s.writeEventTxNoID(ctx, tx, uuid.New(), "WebhookFinanceiroRecebido", map[string]any{"event_key": eventKey, "metodo": method, "credential_id": c.ID, "charge_id": chargeID, "payload": clean}, db.AuditContext{UserType: "appypay_webhook"}); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (s *Service) webhookCredential(ctx context.Context, h http.Header) (*credential, error) {
	env := CurrentEnvironment()
	rows, err := s.client.DB().QueryContext(ctx, `SELECT contexto_tipo,COALESCE(codigo_academia,'') FROM financeiro_credenciais_appypay WHERE ambiente=$1 AND webhook_auth_type IS NOT NULL`, env.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var scope Scope
		if err = rows.Scan(&scope.Type, &scope.AcademiaCode); err != nil {
			return nil, err
		}
		c, e := s.loadCredential(ctx, scope, true)
		if e != nil {
			continue
		}
		if c.WebhookAuthType == "basic" {
			u, p, ok := basicAuth(h)
			if ok && secureEqual(u, c.webhookUser) && secureEqual(p, c.webhookSecret) {
				return c, nil
			}
		} else if c.WebhookAuthType == "api_key" {
			v := h.Get("X-API-Key")
			if v == "" {
				v = h.Get("Api-Key")
			}
			if secureEqual(v, c.webhookSecret) {
				return c, nil
			}
		}
	}
	return nil, errors.New("webhook AppyPay não autorizado")
}

func (s *Service) appypayJSON(ctx context.Context, c *credential, method, path string, body map[string]any, async bool) (map[string]any, int, error) {
	token, err := s.token(ctx, c)
	if err != nil {
		return nil, 0, err
	}
	var reader io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return nil, 0, e
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, CurrentEnvironment().APIBaseURL+path, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if async {
		req.Header.Set("Accept", "application/vnd.appypay.asyncapi+json")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, res.StatusCode, err
	}
	data := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &data)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return data, res.StatusCode, fmt.Errorf("AppyPay respondeu HTTP %d", res.StatusCode)
	}
	return data, res.StatusCode, nil
}
func (s *Service) token(ctx context.Context, c *credential) (string, error) {
	s.mu.Lock()
	cached, ok := s.tokens[c.ID]
	s.mu.Unlock()
	if ok && s.now().Before(cached.expiresAt) {
		return cached.value, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {c.clientID}, "client_secret": {c.clientSecret}, "resource": {c.resource}}
	req, err := http.NewRequestWithContext(ctx, "POST", CurrentEnvironment().TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var response struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if e != nil {
		return "", e
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("falha ao obter token AppyPay: HTTP %d", res.StatusCode)
	}
	if e = json.Unmarshal(raw, &response); e != nil {
		return "", e
	}
	if response.AccessToken == "" {
		return "", errors.New("AppyPay não devolveu access_token")
	}
	expiry := time.Duration(response.ExpiresIn) * time.Second
	if expiry == 0 {
		expiry = time.Hour
	}
	s.mu.Lock()
	s.tokens[c.ID] = tokenCache{response.AccessToken, s.now().Add(expiry - time.Minute)}
	s.mu.Unlock()
	return response.AccessToken, nil
}

func (s *Service) persistCharge(ctx context.Context, id, credentialID uuid.UUID, scope Scope, merchant, method, status string, request, response map[string]any, eventType string, eventPayload map[string]any, actor db.AuditContext) error {
	tx, err := s.client.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	eventID, err := s.writeEventTx(ctx, tx, id, eventType, eventPayload, actor)
	if err != nil {
		return err
	}
	appypayID := stringValue(response, "id")
	_, err = tx.ExecContext(ctx, `INSERT INTO financeiro_cobrancas(id,credential_id,contexto_tipo,codigo_academia,merchant_transaction_id,appypay_charge_id,metodo,status,payload,response,version,last_event_id) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10,1,$11) ON CONFLICT(id) DO UPDATE SET appypay_charge_id=COALESCE(NULLIF(EXCLUDED.appypay_charge_id,''),financeiro_cobrancas.appypay_charge_id),metodo=COALESCE(NULLIF(EXCLUDED.metodo,''),financeiro_cobrancas.metodo),status=EXCLUDED.status,payload=CASE WHEN EXCLUDED.payload='null'::jsonb THEN financeiro_cobrancas.payload ELSE EXCLUDED.payload END,response=EXCLUDED.response,version=financeiro_cobrancas.version+1,last_event_id=EXCLUDED.last_event_id,updated_at=CURRENT_TIMESTAMP`, id, credentialID, scope.Type, nullable(scope.AcademiaCode), merchant, appypayID, method, status, toJSON(request), toJSON(response), eventID)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) writeEvent(ctx context.Context, id uuid.UUID, typ string, payload map[string]any, actor db.AuditContext) error {
	tx, err := s.client.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = s.writeEventTx(ctx, tx, id, typ, payload, actor); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) writeEventTxNoID(ctx context.Context, tx *sqlx.Tx, id uuid.UUID, typ string, payload map[string]any, actor db.AuditContext) error {
	_, err := s.writeEventTx(ctx, tx, id, typ, payload, actor)
	return err
}
func (s *Service) writeEventTx(ctx context.Context, tx *sqlx.Tx, id uuid.UUID, typ string, payload map[string]any, actor db.AuditContext) (uuid.UUID, error) {
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(event_version),0) FROM spuri_ledger WHERE aggregate_id=$1`, id).Scan(&version); err != nil {
		return uuid.Nil, err
	}
	eventID := uuid.New()
	meta, _ := json.Marshal(map[string]any{"user_id": actor.UserID, "user_type": actor.UserType, "ip": actor.IP})
	data, err := json.Marshal(payload)
	if err != nil {
		return uuid.Nil, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO spuri_ledger(event_id,aggregate_id,aggregate_type,event_type,event_version,payload,metadata,occurred_at) VALUES($1,$2,'Financeiro',$3,$4,$5,$6,CURRENT_TIMESTAMP)`, eventID, id, typ, version+1, data, meta)
	return eventID, err
}

func encrypt(plain string) (string, error) {
	key := keyMaterial()
	if len(key) == 0 {
		return "", errors.New("FINANCE_ENCRYPTION_KEY não configurada")
	}
	block, err := aes.NewCipher(key)
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
	return base64.RawStdEncoding.EncodeToString(nonce) + ":" + base64.RawStdEncoding.EncodeToString(gcm.Seal(nil, nonce, []byte(plain), nil)), nil
}
func decrypt(value string) (string, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return "", errors.New("ciphertext financeiro inválido")
	}
	nonce, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	data, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(keyMaterial())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	plain, err := gcm.Open(nil, nonce, data, nil)
	return string(plain), err
}
func keyMaterial() []byte {
	raw := strings.TrimSpace(os.Getenv("FINANCE_ENCRYPTION_KEY"))
	if raw == "" {
		return nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded
	}
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
func mask(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(v)-4) + v[len(v)-4:]
}
func nullable(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
func secureEqual(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func basicAuth(h http.Header) (string, string, bool) {
	authorization := h.Get("Authorization")
	if !strings.HasPrefix(authorization, "Basic ") {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(authorization, "Basic ")))
	if err != nil {
		return "", "", false
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
func toJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func stringValue(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}
func methodFromPayment(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	if strings.HasPrefix(v, "GPO_") {
		return methodGPO
	}
	if strings.HasPrefix(v, "REF_") {
		return methodREF
	}
	return ""
}
func alnum(v string) bool {
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return v != ""
}
func newMerchantID() string {
	return "TR" + strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", ""))[:13]
}
func chargeBody(r ChargeRequest) map[string]any {
	b := map[string]any{"amount": r.Amount, "currency": r.Currency, "description": r.Description, "merchantTransactionId": r.MerchantTransactionID, "paymentMethod": r.PaymentMethod}
	if r.PaymentInfo != nil {
		b["paymentInfo"] = r.PaymentInfo
	}
	if r.Options != nil {
		b["options"] = r.Options
	}
	if r.Notify != nil {
		b["notify"] = r.Notify
	}
	return b
}
func resultFrom(data map[string]any, status int, merchant string) *ChargeResult {
	r := &ChargeResult{ID: stringValue(data, "id"), MerchantTransactionID: stringValue(data, "merchantTransactionId"), Status: stringValue(data, "status"), HTTPStatus: status, Data: data}
	if r.MerchantTransactionID == "" {
		r.MerchantTransactionID = merchant
	}
	if r.Status == "" {
		r.Status = "aceita"
	}
	return r
}
func sanitize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		o := map[string]any{}
		for k, val := range x {
			lk := strings.ToLower(k)
			if strings.Contains(lk, "secret") || strings.Contains(lk, "token") || strings.Contains(lk, "apikey") || strings.Contains(lk, "authorization") {
				continue
			}
			o[k] = sanitize(val)
		}
		return o
	case []any:
		o := make([]any, len(x))
		for i := range x {
			o[i] = sanitize(x[i])
		}
		return o
	default:
		return v
	}
}
