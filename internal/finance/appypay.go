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
	"database/sql"
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
	"strconv"
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
	ID                    uuid.UUID `json:"id"`
	ProviderChargeID      string    `json:"provider_charge_id,omitempty"`
	MerchantTransactionID string    `json:"merchant_transaction_id"`
	Status                string    `json:"status"`
	// CodigoProvedor/MensagemProvedor/FonteProvedor/CategoriaMotivo expõem o
	// motivo real devolvido pela AppyPay (responseStatus.code/message/source
	// de POST/webhook, ou o do último transactionEvent de GET /charges) toda
	// vez que a cobrança não está simplesmente aguardando pagamento — ver
	// extractProviderOutcome/applyProviderOutcome nesta mesma package.
	CodigoProvedor   *int           `json:"codigo_provedor,omitempty"`
	MensagemProvedor string         `json:"mensagem_provedor,omitempty"`
	FonteProvedor    string         `json:"fonte_provedor,omitempty"`
	CategoriaMotivo  string         `json:"categoria_motivo,omitempty"`
	Response         map[string]any `json:"response,omitempty"`
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
	Origem string `json:"origem"`
	Status string `json:"status"`
	// CodigoProvedor/MensagemProvedor/FonteProvedor/CategoriaMotivo: ver o
	// comentário equivalente em ChargeResult, acima de CreateCharge. Mesma
	// origem de dado (payload["codigo_provedor"] etc.), lida aqui para a
	// listagem em vez de para uma única cobrança.
	CodigoProvedor   *int    `json:"codigo_provedor,omitempty"`
	MensagemProvedor string  `json:"mensagem_provedor,omitempty"`
	FonteProvedor    string  `json:"fonte_provedor,omitempty"`
	CategoriaMotivo  string  `json:"categoria_motivo,omitempty"`
	Valor            float64 `json:"valor"`
	Moeda            string  `json:"moeda,omitempty"`
	Descricao        string  `json:"descricao,omitempty"`
	// MensalidadesEmAberto é o subconjunto de Mensalidades cujo estado da
	// obrigação (financeiro_mensalidade_obrigacoes_eventos, mesma regra de
	// precedenciaEstado usada por estadoObrigacao/PendenciasSemCobranca)
	// ainda é EstadoPendente no momento da consulta. Só populado por
	// PreencherMensalidadesEmAberto (pagamentos_unificado.go), chamada
	// automaticamente ao final de ListCobrancas/ListCobrancasEstudante,
	// para Origem == "mensalidade" com Status terminal de falha
	// (Failed/Cancelled/Expired/falhada/cancelada — nunca para Success nem
	// para aguardando_pagamento, que já é auto-explicativo). Desde a
	// tarefa 73, a pendência sintética do mesmo mês também volta a
	// aparecer como item separado na listagem unificada (só cobranças "em
	// aberto" — aguardando_pagamento — continuam escondendo-a; ver
	// mesesComCobrancaRealVinculada), então este campo hoje é redundante
	// com aquele item na maioria dos casos — mantido por não quebrar
	// contrato de API e por continuar útil quando o chamador não pediu a
	// pendência sem cobrança (DeveIncluirPendenciasSemCobranca == false).
	MensalidadesEmAberto []MensalidadeSelecaoMes `json:"mensalidades_em_aberto,omitempty"`
	// MetodoPagamento reflete "GPO_QR" (não apenas "GPO") quando a cobrança
	// tem qr_code_type no payload — CreateGPOQRCode grava payment_method
	// como "GPO" internamente, então sem este ajuste a origem QR ficaria
	// indistinguível de um GPO comum nesta listagem.
	MetodoPagamento   string                  `json:"metodo_pagamento,omitempty"`
	CodigoEstudante   string                  `json:"codigo_estudante,omitempty"`
	CodigoSolicitacao string                  `json:"codigo_solicitacao,omitempty"`
	Mensalidades      []MensalidadeSelecaoMes `json:"mensalidades,omitempty"`
	// AtualizadoEm é ponteiro (não time.Time) desde a unificação de
	// pendências sintéticas em ListarPagamentosUnificado
	// (pagamentos_unificado.go): um item sintetizado a partir de uma
	// pendência sintética nunca teve
	// nenhuma atividade real, então não há nenhum "atualizado em" honesto
	// para devolver — nil (omitido do JSON) em vez de inventar uma data.
	// Para uma cobrança real, continua sempre presente (a coluna é
	// NOT NULL em financeiro_cobrancas).
	AtualizadoEm *time.Time `json:"atualizado_em,omitempty"`
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

// RemoveCredential remove as credenciais AppyPay configuradas para um
// contexto (academia ou spuri) no ambiente ativo. A remoção é registrada
// como um novo evento imutável no ledger (CredenciaisAppyPayRemovidas) — o
// evento CredenciaisAppyPayConfiguradas original nunca é apagado ou
// reescrito, apenas deixa de ser o fato mais recente. A partir deste
// comando, loadCredential volta a falhar com ErrNotFound para este
// contexto, bloqueando qualquer nova cobrança (mensalidade, matrícula ou
// cobrança avulsa) exatamente como bloquearia se a credencial nunca
// tivesse existido. O cofre de segredos (financeiro_segredos_appypay) é
// limpo neste mesmo comando, já que essa tabela vive fora do replay do
// ledger (não é reconstruída a partir de eventos).
func (s *Service) RemoveCredential(ctx context.Context, contextoTipo, codigoAcademia, userID, userType, ip string) error {
	if s.client == nil {
		return errors.New("serviço financeiro não inicializado")
	}
	contextoTipo = strings.ToLower(strings.TrimSpace(contextoTipo))
	if err := validContext(contextoTipo, codigoAcademia); err != nil {
		return err
	}
	ambiente := AmbienteAtual()
	credentialID, err := s.findCredentialID(ctx, contextoTipo, codigoAcademia, ambiente)
	if err != nil {
		return fmt.Errorf("%w: credenciais AppyPay não configuradas para este contexto", ErrNotFound)
	}
	payload := map[string]any{
		"credential_id":   credentialID.String(),
		"contexto_tipo":   contextoTipo,
		"codigo_academia": codigoAcademia,
		"ambiente":        ambiente,
	}
	if err := s.record(ctx, credentialID, "CredenciaisAppyPayRemovidas", payload, userID, userType, ip); err != nil {
		return err
	}
	if _, err := s.client.DB().ExecContext(ctx, `DELETE FROM financeiro_segredos_appypay WHERE credential_id=$1`, credentialID); err != nil {
		return err
	}
	return nil
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
	payload := chargePayload(id, in, "", EstadoCobrancaAguardandoPagamento, nil)
	if err = s.record(ctx, id, "CobrancaAppyPaySolicitada", payload, actorID, actorType, ip); err != nil {
		_ = s.releaseChargeReservation(ctx, in.MerchantTransactionID, id)
		return ChargeResult{}, err
	}
	providerBody := map[string]any{"amount": in.Amount, "currency": in.Currency, "description": in.Description, "merchantTransactionId": in.MerchantTransactionID, "paymentMethod": method}
	// paymentInfo/options/notify só entram no corpo quando têm conteúdo real.
	// providerBody é um map[string]any bruto (não uma struct com `omitempty`),
	// então json.Marshal sempre serializa toda chave presente no map — mesmo
	// que o valor seja um map[string]any{} vazio (vira "{}", não é omitido).
	// Isto importa sobretudo para REF com referência gerada pelo gateway, onde
	// a documentação da AppyPay mostra o corpo sem a chave "paymentInfo": um
	// objeto vazio ali pode ser lido pela AppyPay como "o merchant tentou
	// enviar uma referência própria, mas sem os campos exigidos", em vez de
	// "nenhuma referência própria, gerar automaticamente". Ver
	// docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md.
	if len(in.PaymentInfo) > 0 {
		providerBody["paymentInfo"] = in.PaymentInfo
	}
	if len(in.Options) > 0 {
		providerBody["options"] = in.Options
	}
	if len(in.Notify) > 0 {
		providerBody["notify"] = in.Notify
	}
	response, err := s.callJSON(ctx, credential, http.MethodPost, "/charges", providerBody, in.Async)
	if err != nil {
		_ = s.record(ctx, id, "CobrancaAppyPayFalhou", chargePayload(id, in, "", "Failed", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
		return ChargeResult{ID: id, MerchantTransactionID: in.MerchantTransactionID, Status: "Failed"}, err
	}
	providerID := responseID(response)
	outcome := extractProviderOutcome(response)
	status := normalizeChargeStatus(outcome.Status)
	if status == "" {
		// A AppyPay respondeu 2xx sem nenhum campo de status reconhecível no
		// corpo — a cobrança foi aceita mas ainda não temos nenhuma
		// informação sobre sua resolução, exatamente o significado de
		// aguardando pagamento.
		status = EstadoCobrancaAguardandoPagamento
	}
	if strings.HasPrefix(method, "REF") && isSuccessfulChargeStatus(status) {
		// docs/Parceiros e integrações/AppyPay Documentação.md ("Supported
		// Payment Methods" > "Validations and Limitations") documenta REF
		// como o único método de pagamento com "Webhook: Always required" —
		// ao contrário de GPO/UMM/eTPA ("Required for async request"), REF
		// NUNCA se confirma de forma síncrona: o cliente paga a referência
		// depois, num multibanco/balcão, fora desta chamada. A confirmação
		// só pode legitimamente chegar depois, por webhook ou consulta
		// (AcceptWebhook/consultCharge — nenhum dos dois é tocado por este
		// bloco).
		//
		// O código 100 ("Thank you! The payment has been successfully
		// registered") aparece na tabela de códigos da AppyPay com "Applies
		// To" incluindo REF, ao lado de UMM/GPO/FTBAI — métodos onde 100
		// pode sim significar pagamento realmente concluído na hora. Não há
		// nada na documentação garantindo que a AppyPay nunca devolve um
		// código dessa família na resposta síncrona de CRIAÇÃO de uma
		// referência (só que ela não deveria, dado que o pagamento em si só
		// acontece depois, fora da API). Por isso, e pela mesma razão que
		// motivou a correção do código 103 em CreateGPOQRCode: nunca confiar
		// numa classificação "Success" vinda da chamada de criação para REF,
		// custe o que custar — o pior cenário de aceitar por engano é
		// idêntico ao do QR code (matrícula/mensalidade efetivada sem
		// pagamento real), e o custo de bloquear é zero, porque REF genuíno
		// sempre confirma depois mesmo assim.
		status = EstadoCobrancaAguardandoPagamento
	}
	created := chargePayload(id, in, providerID, status, response)
	applyProviderOutcome(created, outcome)
	if err = s.record(ctx, id, "CobrancaAppyPayCriada", created, actorID, actorType, ip); err != nil {
		return ChargeResult{}, err
	}
	result := ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}
	if outcome.HasCode {
		code := outcome.Code
		result.CodigoProvedor = &code
	}
	result.MensagemProvedor, result.FonteProvedor, result.CategoriaMotivo = outcome.Message, outcome.Source, outcome.Categoria
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
	if err = s.record(ctx, id, "QRCodeAppyPaySolicitado", qrCodePayload(id, in, typ, "", EstadoCobrancaAguardandoPagamento, nil), actorID, actorType, ip); err != nil {
		_ = s.releaseChargeReservation(ctx, in.MerchantTransactionID, id)
		return QRCodeResult{}, err
	}
	response, err := s.callJSON(ctx, cred, http.MethodPost, "/qr-codes", body, false)
	if err != nil {
		_ = s.record(ctx, id, "QRCodeAppyPayFalhou", qrCodePayload(id, in, typ, "", "Failed", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
		return QRCodeResult{}, err
	}
	providerID := responseID(response)
	outcome := extractProviderOutcome(response)
	status := normalizeChargeStatus(outcome.Status)
	if status == "" {
		// Mesmo raciocínio de CreateCharge: 2xx sem status reconhecível =
		// aceito, ainda sem resolução conhecida.
		status = EstadoCobrancaAguardandoPagamento
	}
	if outcome.HasCode && outcome.Code == 103 {
		// O código 103 na documentação da AppyPay ("Post a GPO QR Code",
		// resposta de exemplo) significa exclusivamente "QR code criado com
		// sucesso" — o próprio campo literal "status" da resposta real é
		// "Active" (QR pronto para ser escaneado), nunca "pago". Só existe
		// nesta chamada de CRIAÇÃO; uma vez o cliente realmente pagar
		// escaneando o QR, a confirmação chega depois por webhook ou
		// consulta (consultCharge/AcceptWebhook), com um código diferente
		// (100/1100) — aí sim tratado como sucesso real de pagamento pela
		// mesma appyPayCodeOutcomes.
		//
		// appyPayCodeOutcomes mapeia 103 para "Success" porque é esse o
		// bracket que a própria documentação da AppyPay usa para o código —
		// mas ali "Success" quer dizer "a operação de API pedida (criar o
		// QR) teve sucesso", não "o pagamento foi recebido". Antes desta
		// correção, normalizeChargeStatus(outcome.Status) devolvia
		// "Success" tal e qual, isSuccessfulChargeStatus via true, e
		// CreateGPOQRCode chamava confirmMensalidadeCharge e o chamador
		// (IniciarPagamentoMatricula/IniciarPagamentoMensalidades) efetivava
		// a matrícula ou dava a mensalidade como paga — tudo isso só por
		// gerar o QR code, antes de qualquer pagamento real acontecer.
		//
		// Código/mensagem/fonte do provedor continuam preservados abaixo
		// (via applyProviderOutcome) para auditoria; só o status usado nas
		// decisões de pagamento é que nunca pode ser "Success" aqui.
		status = EstadoCobrancaAguardandoPagamento
	}
	payload := qrCodePayload(id, in, typ, providerID, status, response)
	applyProviderOutcome(payload, outcome)
	if err = s.record(ctx, id, "QRCodeAppyPayGerado", payload, actorID, actorType, ip); err != nil {
		return QRCodeResult{}, err
	}
	qr, _ := response["qrCodeArr"].(string)
	chargeResult := ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}
	if outcome.HasCode {
		code := outcome.Code
		chargeResult.CodigoProvedor = &code
	}
	chargeResult.MensagemProvedor, chargeResult.FonteProvedor, chargeResult.CategoriaMotivo = outcome.Message, outcome.Source, outcome.Categoria
	result := QRCodeResult{ChargeResult: chargeResult, QRCodeArr: qr}
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

// ListCobrancas lista cobranças de um contexto/academia, com filtros
// opcionais de estado e origem. turmaID, cursoID, anoAcademico e anoLetivo
// (introduzidos na tarefa 58) restringem adicionalmente o resultado às
// cobranças de mensalidade vinculadas a esse escopo (turma, curso, ano
// acadêmico ou ano letivo) — ver chargeIDsEscopoMensalidade. Como esse
// escopo só existe para cobranças de ORIGEM mensalidade, usar qualquer um
// desses quatro filtros exclui automaticamente cobranças de matrícula e
// avulsas do resultado; isso é intencional. Quando nenhum dos quatro é
// informado, o comportamento é idêntico ao anterior à tarefa 58.
// mes (tarefa 60) restringe adicionalmente a um mês de calendário (1-12)
// dentro do escopo — ver chargeIDsEscopoMensalidade. Só tem efeito quando
// combinado com pelo menos um dos quatro filtros de escopo acima; sozinho,
// não delimita o suficiente (haveria estudantes de vários anos letivos
// diferentes com cobranças naquele mesmo mês de calendário).
func (s *Service) ListCobrancas(ctx context.Context, contexto, academia string, estados, origens []string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, mes *int, limit, offset int) (*CobrancaListResult, error) {
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
		args = append(args, pq.Array(estadosCobrancaEquivalentes(estados)))
		i++
	}
	if len(origens) > 0 {
		clause, err := origensClause(origens)
		if err != nil {
			return nil, err
		}
		where += clause
	}
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		chargeIDs, err := s.chargeIDsEscopoMensalidade(ctx, academia, turmaID, cursoID, anoAcademico, anoLetivo, mes)
		if err != nil {
			return nil, err
		}
		where += fmt.Sprintf(" AND id = ANY($%d::uuid[])", i)
		args = append(args, pq.Array(chargeIDs))
		i++
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
		dto, err := scanCobrancaResumo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.PreencherMensalidadesEmAberto(ctx, out); err != nil {
		return nil, err
	}
	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
}

// estadosCobrancaEquivalentes expande os valores de filtro "estado"
// informados pelo chamador para o conjunto completo de valores brutos que
// scanCobrancaResumo/normalizeChargeStatus tratam como equivalentes, antes
// de montar a cláusula SQL "payload->>'status' = ANY($n)" em ListCobrancas/
// ListCobrancasEstudante. Necessário porque esse filtro SQL compara o valor
// BRUTO gravado no ledger — e cobranças criadas antes desta tarefa ainda
// têm, no payload de eventos já gravados (o ledger é append-only, ver
// spuri_ledger), os valores antigos "solicitada"/"criada"/"Requested"/
// "Pending" em vez do estado canônico EstadoCobrancaAguardandoPagamento.
// Sem esta expansão, filtrar por estado=aguardando_pagamento encontraria só
// as cobranças criadas DEPOIS do deploy desta tarefa, escondendo qualquer
// cobrança antiga que ainda esteja nesse estado — inconsistente com o que
// scanCobrancaResumo mostra ao ler a mesma linha (que já normaliza na
// leitura).
//
// A mesma lacuna existia para "Failed" (bug relatado por Fredy: GET
// .../estudante/:codigo?estado=Failed devolvia vazio mesmo havendo
// cobranças falhadas do estudante, reproduzido também para academia/admin
// — ver tarefa 69). "Failed" é o valor cru que a própria AppyPay devolve
// quando o PROCESSADOR recusa a cobrança (docs/Parceiros e integrações/
// AppyPay Documentação.md). Antes da tarefa 69, CreateCharge/
// CreateGPOQRCode gravavam um valor local diferente, "falhada", quando a
// própria chamada HTTP à AppyPay falhava — nunca chegando a existir uma
// cobrança do lado do provedor, então a AppyPay nunca teve chance de
// devolver "Failed". Por decisão de Fredy, esta tarefa unifica os dois:
// daqui pra frente CreateCharge/CreateGPOQRCode gravam "Failed"
// diretamente nesse caso — "falhada" só continua a existir como o valor
// BRUTO de cobranças criadas antes do deploy desta tarefa (o ledger é
// append-only, imutável), exatamente a mesma situação de
// aguardando_pagamento/Pending/Requested/solicitada/criada acima, só que
// para o estado terminal Failed em vez do estado aguardando_pagamento.
//
// Qualquer outro valor de filtro (Success, Cancelled, Expired, ou qualquer
// string não reconhecida, incluindo EstadoPendente) passa inalterado — só
// "aguardando_pagamento" e "Failed" têm equivalência com valores brutos
// históricos diferentes de si mesmos.
func estadosCobrancaEquivalentes(estados []string) []string {
	out := make([]string, 0, len(estados))
	for _, estado := range estados {
		switch {
		case strings.EqualFold(strings.TrimSpace(estado), EstadoCobrancaAguardandoPagamento):
			out = append(out, EstadoCobrancaAguardandoPagamento, "Pending", "Requested", "solicitada", "criada")
		case strings.EqualFold(strings.TrimSpace(estado), "Failed"):
			out = append(out, "Failed", "falhada")
		default:
			out = append(out, estado)
		}
	}
	return out
}

// origensClause monta a cláusula SQL "AND (...)" que filtra
// financeiro_cobrancas pelo tipo de cobrança derivado do payload
// (mensalidade, matrícula ou avulsa) — a mesma derivação usada por
// scanCobrancaResumo. Devolve "" (sem filtro) quando origens está vazio.
// Extraída durante a tarefa 49 para ser compartilhada por ListCobrancas e
// ListCobrancasEstudante e nunca divergir entre as duas.
func origensClause(origens []string) (string, error) {
	if len(origens) == 0 {
		return "", nil
	}
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
			return "", fmt.Errorf("tipo de cobrança inválido: %s", origem)
		}
	}
	return " AND (" + strings.Join(clauses, " OR ") + ")", nil
}

// scanCobrancaResumo lê uma linha de financeiro_cobrancas (id,
// provider_charge_id, merchant_transaction_id, contexto_tipo,
// codigo_academia, payload, updated_at, nesta ordem exata) e deriva os
// campos resumidos a partir do payload persistido. Compartilhado por
// ListCobrancas e ListCobrancasEstudante para não duplicar a lógica de
// derivação de origem/método/mensalidades — extraído durante a tarefa 48 sem
// nenhuma mudança de comportamento em relação ao que ListCobrancas já fazia
// inline desde a tarefa 47.
func scanCobrancaResumo(rows *sql.Rows) (CobrancaResumo, error) {
	var dto CobrancaResumo
	var rawPayload []byte
	if err := rows.Scan(&dto.ID, &dto.ProviderChargeID, &dto.MerchantTransactionID, &dto.ContextoTipo, &dto.CodigoAcademia, &rawPayload, &dto.AtualizadoEm); err != nil {
		return CobrancaResumo{}, err
	}
	var payload map[string]any
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return CobrancaResumo{}, err
	}
	rawStatus, _ := payload["status"].(string)
	dto.Status = normalizeChargeStatus(rawStatus)
	if codigo, ok := payload["codigo_provedor"].(float64); ok {
		c := int(codigo)
		dto.CodigoProvedor = &c
	}
	dto.MensagemProvedor, _ = payload["mensagem_provedor"].(string)
	dto.FonteProvedor, _ = payload["fonte_provedor"].(string)
	dto.CategoriaMotivo, _ = payload["categoria_motivo"].(string)
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
	return dto, nil
}

// turmaID, cursoID, anoAcademico e anoLetivo (tarefa 58) filtram
// adicionalmente por escopo de mensalidade, igual a ListCobrancas. Como o
// escopo exige codigo_academia (ver escopoMensalidadeEstudantes), esses
// quatro filtros só têm efeito quando somenteAcademia não é nil (chamada de
// uma academia) — quando o estudante ou um admin FPP consultam sem
// restringir a academia, informar qualquer um desses quatro filtros
// devolve erro de validação, porque não há uma única academia para resolver
// o escopo de turma/curso contra o histórico do estudante.
// mes (tarefa 60) tem o mesmo efeito de ListCobrancas: só delimita a mais
// junto de um dos quatro filtros acima. Nenhum endpoint HTTP expõe este
// parâmetro para ListCobrancasEstudante ainda — a assinatura só ganhou o
// parâmetro para poder compartilhar chargeIDsEscopoMensalidade com
// ListCobrancas sem duplicar código; ver tarefa 60 para o contexto completo.
func (s *Service) ListCobrancasEstudante(ctx context.Context, codigoEstudante string, somenteAcademia *string, estados, origens []string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, mes *int, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	if codigoEstudante == "" {
		return nil, errors.New("código do estudante é obrigatório")
	}
	where := `WHERE (payload->>'codigo_estudante' = $1 OR payload->>'codigo_solicitacao' IN (SELECT codigo_solicitacao FROM projection_solicitacoes_matricula WHERE codigo_estudante_gerado = $1))`
	args := []any{codigoEstudante}
	i := 2
	if somenteAcademia != nil {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, *somenteAcademia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estadosCobrancaEquivalentes(estados)))
		i++
	}
	if len(origens) > 0 {
		clause, err := origensClause(origens)
		if err != nil {
			return nil, err
		}
		where += clause
	}
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		academiaEscopo := ""
		if somenteAcademia != nil {
			academiaEscopo = *somenteAcademia
		}
		chargeIDs, err := s.chargeIDsEscopoMensalidade(ctx, academiaEscopo, turmaID, cursoID, anoAcademico, anoLetivo, mes)
		if err != nil {
			return nil, err
		}
		where += fmt.Sprintf(" AND id = ANY($%d::uuid[])", i)
		args = append(args, pq.Array(chargeIDs))
		i++
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
		dto, err := scanCobrancaResumo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.PreencherMensalidadesEmAberto(ctx, out); err != nil {
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

// isTerminalChargeStatus reporta se uma cobrança real chegou a um estado do
// qual nunca mais sai sozinha (não há mais nada que a AppyPay ou o Spuri
// vão fazer para resolvê-la). Cobre os dois estados locais ("cancelada" —
// cancelamento feito pelo Spuri; "falhada" — a própria chamada HTTP para a
// AppyPay falhou, sem chegar a existir uma cobrança do lado do provedor) e
// os quatro estados terminais documentados pela própria AppyPay e devolvidos
// verbatim (ver docs/Parceiros e integrações/AppyPay Documentação.md):
// Success (paga), Failed (recusada pelo processador), Cancelled (cancelada
// do lado da AppyPay) e Expired (referência REF expirou sem pagamento).
//
// Antes desta correção só "cancelada"/"falhada"/Success eram reconhecidos:
// uma cobrança devolvida pela AppyPay como Failed/Cancelled/Expired não era
// terminal aos olhos desta função, o que tinha dois efeitos colaterais
// reais — (1) CancelCharge não bloqueava uma segunda tentativa de
// cancelamento sobre uma cobrança já resolvida, podendo sobrescrever um
// status Failed genuíno com "cancelada", perdendo a razão real da falha; e
// (2) o SQL equivalente (chargeAbertaStatusExcluidos, mensalidade.go)
// tratava essa cobrança como "em aberto" para sempre. Precisa ficar
// manualmente sincronizada com chargeAbertaStatusExcluidos.
func isTerminalChargeStatus(status string) bool {
	trimmed := strings.TrimSpace(status)
	switch {
	case strings.EqualFold(trimmed, "cancelada"),
		strings.EqualFold(trimmed, "falhada"),
		strings.EqualFold(trimmed, "Failed"),
		strings.EqualFold(trimmed, "Cancelled"),
		strings.EqualFold(trimmed, "Expired"):
		return true
	default:
		return isSuccessfulChargeStatus(trimmed)
	}
}

// normalizeChargeStatus traduz o vocabulário histórico/bruto de status de
// uma cobrança real para os estados canônicos únicos que a API expõe,
// sempre que o valor de entrada tiver um equivalente canônico:
//
//   - EstadoCobrancaAguardandoPagamento (mensalidade.go): os estados locais
//     intermediários que o Spuri gravava antes da tarefa 66 ("solicitada",
//     gravado antes de qualquer chamada ao provedor; "criada", o fallback
//     usado quando o provedor responde 2xx sem nenhum campo de status) e os
//     estados brutos que a própria AppyPay documenta para esta mesma fase
//     ("Requested" e "Pending" — ver docs/Parceiros e integrações/AppyPay
//     Documentação.md).
//   - "Failed": desde a tarefa 69, também "falhada" — o valor local que
//     CreateCharge/CreateGPOQRCode gravavam antes desta tarefa quando a
//     própria chamada HTTP à AppyPay falhava (nunca chegando a existir uma
//     cobrança do lado do provedor, então a AppyPay nunca teve chance de
//     devolver "Failed" — ver estadosCobrancaEquivalentes, que resolve o
//     mesmo problema do lado do filtro SQL). Daqui pra frente
//     CreateCharge/CreateGPOQRCode já gravam "Failed" diretamente nesse caso;
//     "falhada" só volta a aparecer como o valor BRUTO de uma cobrança
//     criada antes do deploy desta tarefa — e mesmo assim nunca chega ao
//     chamador, porque normalizeChargeStatus a traduz aqui na leitura.
//
// Qualquer outro valor (Success, Cancelled, Expired, os dois canônicos
// acima, ou uma string não reconhecida) é devolvido inalterado — a função é
// idempotente e pode ser chamada tanto sobre um valor bruto recém-recebido
// da AppyPay quanto sobre um valor já gravado (histórico ou canônico).
//
// Entrada vazia é devolvida vazia: normalizeChargeStatus nunca decide um
// fallback por conta própria, porque o fallback correto depende do
// contexto de quem chama (ex.: CreateCharge trata "" como uma cobrança
// nova ainda sem informação = aguardando pagamento; já consultCharge trata
// "" como "o provedor não devolveu nada desta vez, mantém o status
// anterior").
func normalizeChargeStatus(raw string) string {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "":
		return ""
	case strings.EqualFold(trimmed, "Pending"),
		strings.EqualFold(trimmed, "Requested"),
		strings.EqualFold(trimmed, "solicitada"),
		strings.EqualFold(trimmed, "criada"):
		return EstadoCobrancaAguardandoPagamento
	case strings.EqualFold(trimmed, "falhada"):
		return "Failed"
	default:
		return trimmed
	}
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
	outcome := extractProviderOutcome(response)
	status := normalizeChargeStatus(outcome.Status)
	if status == "" {
		// AppyPay não devolveu nenhum campo de status reconhecível desta vez
		// — mantém o status anterior (row.Status já vem normalizado por
		// loadCharge) em vez de assumir um novo estado.
		status = row.Status
	}
	previousResponse := row.Payload["response"]
	payload := make(map[string]any, len(row.Payload)+7)
	for key, value := range row.Payload {
		payload[key] = value
	}
	payload["provider_charge_id"] = first(responseID(response), row.ProviderID)
	payload["status"] = status
	payload["response"] = sanitize(response)
	applyProviderOutcome(payload, outcome)
	providerID := first(responseID(response), row.ProviderID)
	result := ChargeResult{ID: row.ID, ProviderChargeID: providerID, MerchantTransactionID: row.Merchant, Status: status, Response: response}
	if outcome.HasCode {
		code := outcome.Code
		result.CodigoProvedor = &code
	}
	result.MensagemProvedor, result.FonteProvedor, result.CategoriaMotivo = outcome.Message, outcome.Source, outcome.Categoria
	if strings.EqualFold(row.Status, "cancelada") && isSuccessfulChargeStatus(status) {
		// Keep the cancellation definitive in the read model. The provider
		// result is recorded for manual FPP reconciliation instead of silently
		// accepting a payment that raced with local cancellation.
		payload["status"] = "cancelada"
		payload["provider_status"] = status
		if err = s.record(ctx, row.ID, "CobrancaAppyPayConflitoPosCancelamento", payload, actorID, actorType, ip); err != nil {
			return ChargeResult{}, err
		}
		result.Status = "cancelada"
		return result, nil
	}
	if status != row.Status || providerID != row.ProviderID || !sameJSON(payload["response"], previousResponse) {
		if err = s.record(ctx, row.ID, "CobrancaAppyPayConsultada", payload, actorID, actorType, ip); err != nil {
			return ChargeResult{}, err
		}
	}
	return result, nil
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
		AccessToken string          `json:"access_token"`
		ExpiresIn   flexibleSeconds `json:"expires_in"`
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

// flexibleSeconds decodifica um campo de segundos (usado aqui para
// expires_in) que a AppyPay pode enviar como número JSON puro OU como
// string JSON contendo um número. O endpoint real de token da AppyPay
// (login.microsoftonline.com/{tenant}/oauth2/token, o endpoint "v1" do
// Azure AD, não o "v2.0") devolve expires_in como STRING
// (ex.: "expires_in": "3599") — documentado no exemplo de resposta da
// secção "Get a token" e comportamento conhecido do próprio Azure AD v1
// (distinto do endpoint v2.0, que usa número). Antes desta correção o
// campo era declarado como int puro: json.Unmarshal de uma string JSON
// para um campo int retorna erro, e token() tratava qualquer erro de
// unmarshal como falha de autenticação total — mesmo com access_token
// presente e válido no mesmo payload. Ver
// docs/Debbugs/Auditoria de conformidade AppyPay — autenticação e geração
// de cobrança (produção).md.
type flexibleSeconds int

func (f *flexibleSeconds) UnmarshalJSON(b []byte) error {
	var asInt int
	if err := json.Unmarshal(b, &asInt); err == nil {
		*f = flexibleSeconds(asInt)
		return nil
	}
	var asString string
	if err := json.Unmarshal(b, &asString); err != nil {
		return err
	}
	n, err := strconv.Atoi(strings.TrimSpace(asString))
	if err != nil {
		return err
	}
	*f = flexibleSeconds(n)
	return nil
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

// liveChargeStatus faz uma consulta ao vivo (GET /charges/{id}) e devolve o
// status autoritativo reportado pela AppyPay nesse exato momento. É a
// medida de segurança que a própria documentação da AppyPay recomenda antes
// de tratar um webhook de sucesso como definitivo — "Important: As a
// security measure, double check the transaction by calling the GET
// /charges endpoint" (secção "Merchant Webhooks"), reforçada na secção
// interna "Escopo do Módulo Financeiro Base": "Ao receber, é recomendado
// confirmar o estado com um GET /charges/{id} antes de aplicar efeitos de
// negócio irreversíveis".
//
// Não persiste nada por si própria — quem chama decide o que gravar a
// partir do status devolvido. O segundo valor devolvido é false quando a
// consulta em si falhou (upstream indisponível, timeout, credencial
// temporariamente inacessível) ou não devolveu nenhum status reconhecível
// — nesses casos quem chama deve cair para confiar no que o webhook
// reportou, em vez de bloquear a confirmação: um GET indisponível não pode
// deixar uma cobrança presa sem nunca confirmar um pagamento que a própria
// AppyPay já reportou como bem-sucedido no webhook (o mesmo raciocínio do
// Bug 1 já corrigido, em que o webhook nunca efetivava a matrícula).
func (s *Service) liveChargeStatus(ctx context.Context, charge chargeRow) (status string, ok bool) {
	cred, err := s.loadCredential(ctx, charge.Contexto, charge.Academia)
	if err != nil {
		return "", false
	}
	path := "/charges/" + url.PathEscape(charge.ProviderID)
	if charge.ProviderID == "" {
		path = "/charges?merchantTransactionId=" + url.QueryEscape(charge.Merchant)
	}
	response, err := s.callJSON(ctx, cred, http.MethodGet, path, nil, false)
	if err != nil {
		return "", false
	}
	status = normalizeChargeStatus(extractProviderOutcome(response).Status)
	return status, status != ""
}

// AcceptWebhook reserves its event id first in the dedicated idempotency index.
// If ledger persistence fails the reservation is removed, so a delivery retry is
// still processed. No charge side-effect is executed here.
func (s *Service) AcceptWebhook(ctx context.Context, metodo, eventID string, owner WebhookOwner, payload map[string]any) (accepted bool, confirmedSuccess bool, err error) {
	metodo = strings.ToUpper(metodo)
	if (metodo != "GPO" && metodo != "REF") || strings.TrimSpace(eventID) == "" {
		return false, false, errors.New("webhook inválido")
	}
	res, err := s.client.DB().ExecContext(ctx, `INSERT INTO financeiro_webhooks_recebidos(event_id,metodo) VALUES($1,$2) ON CONFLICT(event_id) DO NOTHING`, eventID, metodo)
	if err != nil {
		return false, false, err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return false, false, nil
	}
	data := map[string]any{"event_id": eventID, "metodo": metodo, "credential_id": owner.CredentialID.String(), "contexto_tipo": owner.ContextoTipo, "codigo_academia": owner.CodigoAcademia, "payload": sanitize(payload)}
	if err = s.record(ctx, uuid.New(), "WebhookAppyPayRecebido", data, "appypay:webhook", "sistema", "webhook"); err != nil {
		_, _ = s.client.DB().ExecContext(ctx, `DELETE FROM financeiro_webhooks_recebidos WHERE event_id=$1`, eventID)
		return false, false, err
	}
	// Reflete no read model qualquer estado que o webhook reporte — sucesso
	// (Success) ou qualquer um dos outros três estados terminais que a
	// própria AppyPay documenta (Failed, Cancelled, Expired). Antes desta
	// correção só um webhook de sucesso atualizava a cobrança: um webhook
	// avisando que uma referência REF expirou ou que um GPO foi recusado
	// era gravado em WebhookAppyPayRecebido (acima) mas nunca refletia em
	// financeiro_cobrancas, deixando a cobrança "presa" em
	// aguardando_pagamento até alguém consultá-la manualmente.
	outcome := extractProviderOutcome(payload)
	if outcome.Status != "" || outcome.HasCode {
		normalized := normalizeChargeStatus(outcome.Status)
		success := isSuccessfulChargeStatus(normalized)
		if charge, loadErr := s.loadCharge(ctx, eventID); loadErr == nil && charge.Contexto == owner.ContextoTipo && charge.Academia == owner.CodigoAcademia &&
			// Um webhook atrasado e não-bem-sucedido nunca sobrescreve uma
			// cobrança que já chegou a um estado terminal (ex.: já paga, já
			// cancelada) — só um sucesso tem tratamento de conflito próprio
			// (abaixo) que pode correr por cima de um estado terminal local.
			(success || !isTerminalChargeStatus(charge.Status)) {
			// Double-check: um webhook de sucesso "normal" (a cobrança não
			// está cancelada localmente) só é tratado como definitivo depois
			// de uma consulta ao vivo concordar. O caso de sucesso chegando
			// depois de um cancelamento local (abaixo, eventType
			// CobrancaAppyPayConflitoPosCancelamento) já não dispara nenhum
			// efeito irreversível por si só — é só um registo de conflito
			// para reconciliação manual — então não precisa do double-check.
			if success && !strings.EqualFold(charge.Status, "cancelada") {
				if live, ok := s.liveChargeStatus(ctx, charge); ok {
					normalized = live
					success = isSuccessfulChargeStatus(live)
				}
			}
			updated := make(map[string]any, len(charge.Payload)+7)
			for k, v := range charge.Payload {
				updated[k] = v
			}
			updated["status"] = normalized
			if success {
				updated["status"] = "Success"
			}
			updated["provider_charge_id"] = first(responseID(payload), charge.ProviderID)
			updated["response"] = sanitize(payload)
			applyProviderOutcome(updated, outcome)
			eventType := "CobrancaAppyPayConsultada"
			if success && strings.EqualFold(charge.Status, "cancelada") {
				// A provider may still settle a REF/GPO/QR after Spuri's local
				// cancellation. Preserve cancellation and leave an explicit audit
				// conflict for FPP reconciliation.
				updated["status"] = "cancelada"
				updated["provider_status"] = "Success"
				eventType = "CobrancaAppyPayConflitoPosCancelamento"
			}
			if s.record(ctx, charge.ID, eventType, updated, "appypay:webhook", "sistema", "webhook") == nil && success && eventType == "CobrancaAppyPayConsultada" {
				confirmedSuccess = true
				_ = s.confirmMensalidadeCharge(ctx, charge.ID, "appypay:webhook", "sistema", "webhook")
			}
		}
	}
	return true, confirmedSuccess, nil
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
	rawStatus, _ := r.Payload["status"].(string)
	r.Status = normalizeChargeStatus(rawStatus)
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

// applyPersistedProviderFields preenche os campos de motivo de um
// ChargeResult a partir de um payload já persistido (financeiro_cobrancas),
// para que uma resposta idempotente (mesmo merchantTransactionId reenviado)
// devolva exatamente a mesma informação que a criação original devolveu —
// ver applyProviderOutcome, gravado neste mesmo payload em CreateCharge/
// CreateGPOQRCode/consultCharge/AcceptWebhook.
func applyPersistedProviderFields(result *ChargeResult, payload map[string]any) {
	if codigo, ok := payload["codigo_provedor"].(float64); ok {
		c := int(codigo)
		result.CodigoProvedor = &c
	}
	result.MensagemProvedor, _ = payload["mensagem_provedor"].(string)
	result.FonteProvedor, _ = payload["fonte_provedor"].(string)
	result.CategoriaMotivo, _ = payload["categoria_motivo"].(string)
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
	result := ChargeResult{ID: row.ID, ProviderChargeID: row.ProviderID, MerchantTransactionID: row.Merchant, Status: row.Status, Response: response}
	applyPersistedProviderFields(&result, row.Payload)
	return result, nil
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
	chargeResult := ChargeResult{ID: row.ID, ProviderChargeID: row.ProviderID, MerchantTransactionID: row.Merchant, Status: row.Status, Response: response}
	applyPersistedProviderFields(&chargeResult, row.Payload)
	return QRCodeResult{ChargeResult: chargeResult, QRCodeArr: qr}, nil
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
		// Duas formas válidas de paymentInfo para REF:
		//  1. Só dueDate (o caso introduzido nesta tarefa): a referência
		//     continua gerada pelo gateway (AppyPay escolhe
		//     referenceNumber), só o prazo de expiração é customizado —
		//     ver gerarCobranca em cobranca_geracao.go e o comentário sobre
		//     a hipótese ainda não confirmada contra o ambiente real da
		//     AppyPay.
		//  2. Os três campos completos (referenceNumber+dueDate+nib): a
		//     forma "referência gerada pelo comerciante" documentada pela
		//     AppyPay. Nenhum chamador atual usa esta forma — mantida por
		//     integridade caso um chamador futuro precise dela.
		_, hasDueDateOnly := in.PaymentInfo["dueDate"].(string)
		if hasDueDateOnly && len(in.PaymentInfo) == 1 {
			// válido: só dueDate.
		} else {
			for _, k := range []string{"referenceNumber", "dueDate", "nib"} {
				value, ok := in.PaymentInfo[k].(string)
				if !ok || strings.TrimSpace(value) == "" {
					return fmt.Errorf("REF com paymentInfo exige %s (ou apenas dueDate sozinho)", k)
				}
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
	// GET /charges/{id} (e GET /charges?merchantTransactionId=...) devolvem
	// tudo dentro de um envelope "payment" — ver
	// extractProviderOutcome, logo abaixo, para o mesmo problema aplicado ao
	// status.
	if payment, ok := v["payment"].(map[string]any); ok {
		for _, k := range []string{"id", "chargeId", "charge_id"} {
			if x, ok := payment[k].(string); ok {
				return x
			}
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

// providerOutcome carrega tudo o que a Spuri consegue aprender de uma única
// resposta/webhook da AppyPay sobre o resultado real de uma cobrança: o
// status usado para acionar a máquina de estados interna (Status, já
// resolvido — ver resolveOutcomeStatus), mais tudo o que é preciso para
// explicar "porquê" a um humano (Code/Message/Source, crus, exatamente como
// a AppyPay os enviou) e uma categoria de melhor esforço para filtragem
// programática (Categoria).
type providerOutcome struct {
	Status    string
	Code      int
	HasCode   bool
	Message   string
	Source    string
	Categoria string
}

func extractProviderOutcome(v map[string]any) providerOutcome {
	if payment, ok := v["payment"].(map[string]any); ok {
		out := providerOutcome{}
		if s, ok := payment["status"].(string); ok {
			out.Status = s
		}
		if events, ok := payment["transactionEvents"].([]any); ok && len(events) > 0 {
			if last, ok := events[len(events)-1].(map[string]any); ok {
				if rs, ok := last["responseStatus"].(map[string]any); ok {
					applyResponseStatus(&out, rs)
				}
			}
		}
		if out.Status != "" || out.HasCode {
			resolveOutcomeStatus(&out)
			return out
		}
	}
	if rs, ok := v["responseStatus"].(map[string]any); ok {
		out := providerOutcome{}
		applyResponseStatus(&out, rs)
		if out.Status != "" || out.HasCode {
			resolveOutcomeStatus(&out)
			return out
		}
	}
	out := providerOutcome{Status: responseStatus(v)}
	resolveOutcomeStatus(&out)
	return out
}

// IsSuccessfulProviderPayload informa se um payload cru vindo da AppyPay —
// seja a resposta síncrona de POST /charges ou /qr-codes, seja o corpo de um
// webhook — representa um pagamento concluído com sucesso.
//
// É a MESMA extração/normalização usada por CreateCharge, CreateGPOQRCode e
// ConsultCharge (extractProviderOutcome + normalizeChargeStatus), incluindo
// a leitura correta de "responseStatus.status"/"responseStatus.successful",
// que é onde a AppyPay realmente coloca o status (nunca em campos soltos
// "status"/"state" na raiz do payload — ver seção "Merchant Webhooks" de
// docs/Parceiros e integrações/AppyPay Documentação.md). Qualquer código
// fora deste pacote que precise reagir a um sucesso da AppyPay (por exemplo,
// um handler HTTP de webhook decidindo se efetiva uma matrícula) deve
// chamar esta função em vez de reimplementar sua própria leitura do
// payload — ver docs/Debbugs/Auditoria de conformidade AppyPay (autenticação
// e geração de cobrança).md para o bug que isto substitui.
func IsSuccessfulProviderPayload(payload map[string]any) bool {
	return isSuccessfulChargeStatus(normalizeChargeStatus(extractProviderOutcome(payload).Status))
}

func applyResponseStatus(out *providerOutcome, rs map[string]any) {
	if s, ok := rs["status"].(string); ok {
		out.Status = s
	}
	if m, ok := rs["message"].(string); ok {
		out.Message = m
	}
	if src, ok := rs["source"].(string); ok {
		out.Source = src
	}
	switch c := rs["code"].(type) {
	case float64:
		out.Code = int(c)
		out.HasCode = true
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(c)); err == nil {
			out.Code = n
			out.HasCode = true
		}
	}
}

func resolveOutcomeStatus(out *providerOutcome) {
	if out.HasCode {
		if info, ok := appyPayCodeOutcomes[out.Code]; ok {
			out.Status = info.Estado
			out.Categoria = info.Categoria
			return
		}
		out.Categoria = "desconhecido"
		if out.Status == "" {
			out.Status = "Failed"
		}
	}
}

func applyProviderOutcome(payload map[string]any, outcome providerOutcome) {
	if outcome.HasCode {
		payload["codigo_provedor"] = outcome.Code
	}
	if outcome.Message != "" {
		payload["mensagem_provedor"] = outcome.Message
	}
	if outcome.Source != "" {
		payload["fonte_provedor"] = outcome.Source
	}
	if outcome.Categoria != "" {
		payload["categoria_motivo"] = outcome.Categoria
	}
}

type appyPayCodeInfo struct {
	Estado    string
	Categoria string
}

var appyPayCodeOutcomes = map[int]appyPayCodeInfo{
	100: {"Success", ""}, 101: {"Pending", ""}, 102: {"Pending", ""}, 103: {"Success", ""}, 319: {"Pending", ""}, 1100: {"Success", ""},
	231: {"Cancelled", "recusado_pelo_cliente"}, 219: {"Cancelled", "recusado_pelo_cliente"},
	209: {"Cancelled", "saldo_insuficiente"}, 203: {"Cancelled", "saldo_insuficiente"}, 204: {"Cancelled", "saldo_insuficiente"},
	210: {"Cancelled", "tempo_esgotado"}, 211: {"Cancelled", "tempo_esgotado"},
	200: {"Cancelled", "recusado_pelo_processador"}, 206: {"Cancelled", "recusado_pelo_processador"}, 208: {"Cancelled", "recusado_pelo_processador"}, 217: {"Cancelled", "recusado_pelo_processador"}, 227: {"Cancelled", "recusado_pelo_processador"}, 230: {"Cancelled", "recusado_pelo_processador"}, 205: {"Cancelled", "recusado_pelo_processador"}, 207: {"Cancelled", "recusado_pelo_processador"}, 218: {"Cancelled", "recusado_pelo_processador"}, 226: {"Cancelled", "recusado_pelo_processador"},
	201: {"Cancelled", "recusado_pelo_emissor"}, 212: {"Cancelled", "recusado_pelo_emissor"}, 213: {"Cancelled", "recusado_pelo_emissor"}, 214: {"Cancelled", "recusado_pelo_emissor"}, 215: {"Cancelled", "recusado_pelo_emissor"}, 216: {"Cancelled", "recusado_pelo_emissor"}, 220: {"Cancelled", "recusado_pelo_emissor"}, 221: {"Cancelled", "recusado_pelo_emissor"}, 222: {"Cancelled", "recusado_pelo_emissor"}, 223: {"Cancelled", "recusado_pelo_emissor"}, 224: {"Cancelled", "recusado_pelo_emissor"}, 225: {"Cancelled", "recusado_pelo_emissor"}, 228: {"Cancelled", "recusado_pelo_emissor"}, 229: {"Cancelled", "recusado_pelo_emissor"}, 202: {"Cancelled", "recusado_pelo_emissor"},
	245: {"Expired", "referencia_expirada"}, 762: {"Failed", "referencia_invalida"}, 763: {"Failed", "referencia_duplicada"},
	233: {"Cancelled", "conta_invalida"}, 238: {"Cancelled", "recusado_pelo_processador"}, 239: {"Cancelled", "recusado_pelo_cliente"}, 240: {"Cancelled", "recusado_pelo_processador"}, 242: {"Cancelled", "conta_invalida"}, 243: {"Cancelled", "pin_invalido"}, 244: {"Cancelled", "erro_interno_provedor"}, 246: {"Failed", "erro_interno_provedor"}, 247: {"Failed", "erro_interno_provedor"}, 248: {"Failed", "erro_interno_provedor"}, 309: {"Failed", "erro_comunicacao"}, 310: {"Failed", "erro_interno_provedor"}, 311: {"Failed", "erro_interno_provedor"}, 312: {"Failed", "erro_interno_provedor"}, 313: {"Failed", "erro_interno_provedor"}, 314: {"Failed", "erro_comunicacao"}, 315: {"Failed", "erro_interno_provedor"}, 316: {"Failed", "erro_interno_provedor"}, 317: {"Failed", "erro_interno_provedor"}, 413: {"Failed", "erro_interno_provedor"}, 414: {"Failed", "erro_interno_provedor"}, 415: {"Failed", "erro_interno_provedor"}, 416: {"Failed", "erro_interno_provedor"}, 417: {"Failed", "erro_interno_provedor"}, 418: {"Failed", "erro_interno_provedor"}, 759: {"Failed", "valor_minimo"},
	249: {"Cancelled", "erro_interno_provedor"}, 318: {"Failed", "erro_interno_provedor"}, 301: {"Failed", "erro_interno_provedor"}, 302: {"Failed", "erro_interno_provedor"}, 306: {"Failed", "erro_comunicacao"}, 308: {"Failed", "erro_interno_provedor"}, 402: {"Failed", "erro_interno_provedor"}, 403: {"Failed", "erro_interno_provedor"}, 404: {"Failed", "erro_interno_provedor"}, 405: {"Failed", "erro_interno_provedor"}, 406: {"Failed", "erro_comunicacao"}, 407: {"Failed", "erro_comunicacao"}, 408: {"Failed", "erro_interno_provedor"}, 410: {"Failed", "erro_interno_provedor"}, 411: {"Failed", "erro_interno_provedor"}, 412: {"Failed", "erro_interno_provedor"}, 440: {"Failed", "erro_interno_provedor"}, 900: {"Failed", "erro_desconhecido"}, 901: {"Failed", "erro_desconhecido"}, 1101: {"Failed", "conta_inativa"}, 1102: {"Failed", "conta_inativa"}, 1103: {"Failed", "conta_inativa"}, 1104: {"Failed", "conta_inativa"}, 1105: {"Failed", "conta_inativa"}, -1: {"Failed", "erro_desconhecido"},
	717: {"Failed", "dados_invalidos"}, 718: {"Failed", "dados_invalidos"}, 719: {"Failed", "dados_invalidos"}, 720: {"Failed", "transacao_nao_encontrada"}, 726: {"Failed", "referencia_duplicada"}, 760: {"Failed", "dados_invalidos"}, 761: {"Failed", "dados_invalidos"}, 800: {"Failed", "dados_invalidos"}, 803: {"Failed", "dados_invalidos"}, 500: {"Failed", "erro_interno_provedor"}, 501: {"Failed", "erro_interno_provedor"}, 502: {"Failed", "erro_interno_provedor"}, 503: {"Failed", "erro_interno_provedor"}, 504: {"Failed", "erro_interno_provedor"}, 505: {"Failed", "erro_interno_provedor"}, 507: {"Failed", "erro_interno_provedor"}, 508: {"Failed", "erro_interno_provedor"}, 801: {"Failed", "dados_invalidos"}, 802: {"Failed", "dados_invalidos"},
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
