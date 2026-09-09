package services

// Cliente HTTP para a API da GoSMS (docs/Parceiros e integrações/GoSMS API
// - Documentação.md, seção 5.1 — POST /v1/messages), usado pelo módulo real
// de comunicação (internal/handlers/comunicacao_handlers.go). Isolado dos
// outros domínios (não importa internal/db, internal/domain/aggregates nem
// internal/finance) — recebe sempre o token de API já decifrado pelo
// chamador, nunca lê variáveis de ambiente diretamente.
//
// Não reutiliza nem depende de ziett_sms_test_client.go — aquele arquivo é
// exclusivo da rota isolada de teste POST /integracoes/ziett/mensagens/teste
// (Tarefa 20) e deve continuar isolado.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const goSMSBaseURL = "https://api.go-sms.co.ao"

type ComunicacaoGoSMSClient struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// NewComunicacaoGoSMSClient recebe o token de API já decifrado (nunca lido
// de variável de ambiente — vem de projection_remetentes_comunicacao,
// decifrado com services.DecryptComunicacaoSegredo).
func NewComunicacaoGoSMSClient(token string) *ComunicacaoGoSMSClient {
	return &ComunicacaoGoSMSClient{token: token, httpClient: &http.Client{Timeout: 15 * time.Second}, baseURL: goSMSBaseURL}
}

// ComunicacaoGoSMSAPIError modela os dois formatos de erro já observados na
// GoSMS (ver seção 4 da documentação): "errors" como objeto único
// {message, code} OU como array de objetos {message}. Os dois formatos são
// tentados na decodificação (ver parseGoSMSError).
type ComunicacaoGoSMSAPIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *ComunicacaoGoSMSAPIError) Error() string {
	if e == nil {
		return "erro da GoSMS"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return fmt.Sprintf("erro da GoSMS com status %d", e.StatusCode)
}

type ComunicacaoGoSMSNetworkError struct{ Err error }

func (e *ComunicacaoGoSMSNetworkError) Error() string { return "falha ao contactar a GoSMS" }
func (e *ComunicacaoGoSMSNetworkError) Unwrap() error { return e.Err }

func parseGoSMSError(statusCode int, body []byte) *ComunicacaoGoSMSAPIError {
	// Formato 1: {"errors": {"message": "...", "code": "..."}}
	var objErr struct {
		Errors struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &objErr); err == nil && objErr.Errors.Message != "" {
		return &ComunicacaoGoSMSAPIError{StatusCode: statusCode, Code: objErr.Errors.Code, Message: objErr.Errors.Message}
	}
	// Formato 2: {"errors": [{"message": "..."}]}
	var arrErr struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &arrErr); err == nil && len(arrErr.Errors) > 0 && arrErr.Errors[0].Message != "" {
		return &ComunicacaoGoSMSAPIError{StatusCode: statusCode, Message: arrErr.Errors[0].Message}
	}
	return &ComunicacaoGoSMSAPIError{StatusCode: statusCode, Message: "a GoSMS retornou erro sem corpo reconhecido"}
}

// EnviarSMS envia uma SMS via POST /v1/messages. destinatarioNacional deve
// já vir validado no formato nacional angolano de 9 dígitos (sem "0"
// inicial, sem "+244") — a GoSMS aceita esse formato diretamente em "to"
// (ver exemplo "921939411" na documentação, seção 5.1). Retorna o "id" do
// envio (o "lote", campo `id` da resposta 201) para gravar como
// mensagem_externa_id.
func (c *ComunicacaoGoSMSClient) EnviarSMS(ctx context.Context, remetente, destinatarioNacional, conteudo string) (string, error) {
	payload := map[string]string{
		"message": conteudo,
		"from":    remetente,
		"to":      destinatarioNacional,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Token "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &ComunicacaoGoSMSNetworkError{Err: err}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		var parsed struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return "", err
		}
		if parsed.ID == "" {
			return "", errors.New("resposta da GoSMS sem id de envio")
		}
		return parsed.ID, nil
	}
	return "", parseGoSMSError(resp.StatusCode, respBody)
}
