package services

// Cliente HTTP para a API da Ziett (docs/Parceiros e integrações/Ziett API
// - Documentação.md, seção 5 — POST /messages), usado pelo módulo real de
// comunicação (internal/handlers/comunicacao_handlers.go). Isolado dos
// outros domínios (não importa internal/db, internal/domain/aggregates nem
// internal/finance) — recebe sempre o token de API já decifrado pelo
// chamador, nunca lê variáveis de ambiente diretamente.
//
// Não reutiliza nem depende de ziett_sms_test_client.go — aquele arquivo é
// exclusivo da rota isolada de teste POST /integracoes/ziett/mensagens/teste
// (Tarefa 20) e deve continuar isolado. A lógica de normalização de
// telefone abaixo (formatarDestinatarioZiettE164) é uma cópia adaptada e
// independente da mesma regra já validada naquele arquivo — não a mesma
// função, para não criar acoplamento entre os dois módulos.

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
	"unicode"
)

const (
	comunicacaoZiettBaseURL    = "https://api.ziett.co/c/v1"
	comunicacaoZiettChannelSMS = "SMS"
)

type ComunicacaoZiettClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewComunicacaoZiettClient recebe o token de API já decifrado (nunca lido
// de variável de ambiente — vem de projection_remetentes_comunicacao,
// decifrado com services.DecryptComunicacaoSegredo).
func NewComunicacaoZiettClient(apiKey string) *ComunicacaoZiettClient {
	return &ComunicacaoZiettClient{apiKey: apiKey, httpClient: &http.Client{Timeout: 15 * time.Second}, baseURL: comunicacaoZiettBaseURL}
}

type ComunicacaoZiettAPIError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Status    int                    `json:"status"`
	TraceID   string                 `json:"trace_id"`
	Timestamp string                 `json:"timestamp"`
	Service   string                 `json:"service"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func (e *ComunicacaoZiettAPIError) Error() string {
	if e == nil {
		return "erro da Ziett"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return fmt.Sprintf("erro da Ziett com status %d", e.Status)
}

type ComunicacaoZiettNetworkError struct{ Err error }

func (e *ComunicacaoZiettNetworkError) Error() string { return "falha ao contactar a Ziett" }
func (e *ComunicacaoZiettNetworkError) Unwrap() error { return e.Err }

// formatarDestinatarioZiettE164 converte o formato nacional angolano de 9
// dígitos (sem "0" inicial, sem "+244") — o mesmo formato aceito
// diretamente pela GoSMS — para o E.164 completo que a Ziett exige em
// target_e164. Aceita defensivamente um "0" inicial ou um prefixo
// "+244"/"244" recebido por engano, exatamente como a normalização já
// validada em ziett_sms_test_client.go, mas reimplementada aqui de forma
// independente (ver nota de isolamento no topo do arquivo).
func formatarDestinatarioZiettE164(destinatarioNacional string) (string, error) {
	clean := strings.TrimSpace(destinatarioNacional)
	clean = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(clean)
	if clean == "" {
		return "", errors.New("destinatário é obrigatório e deve usar o número nacional angolano de 9 dígitos, exemplo: 923456789")
	}
	if strings.HasPrefix(clean, "+244") {
		clean = strings.TrimPrefix(clean, "+244")
	} else if strings.HasPrefix(clean, "244") {
		clean = strings.TrimPrefix(clean, "244")
	}
	if strings.HasPrefix(clean, "0") {
		clean = strings.TrimPrefix(clean, "0")
	}
	for _, r := range clean {
		if !unicode.IsDigit(r) {
			return "", errors.New("destinatário deve conter apenas dígitos após normalização, no formato nacional angolano de 9 dígitos, exemplo: 923456789")
		}
	}
	if len(clean) != 9 || !strings.HasPrefix(clean, "9") {
		return "", errors.New("destinatário deve ser um número móvel angolano de 9 dígitos iniciado por 9, sem 0 inicial e sem +244, exemplo: 923456789")
	}
	return "+244" + clean, nil
}

// EnviarSMS envia uma SMS via POST /messages, com channel_type sempre fixo
// em "SMS". remitterID é o identificador (UUID) do remetente configurado
// no painel da Ziett. destinatarioNacional deve vir no formato nacional
// angolano de 9 dígitos — esta função monta o E.164 completo antes de
// enviar. Retorna o message_id (202 Accepted) para gravar como
// mensagem_externa_id.
func (c *ComunicacaoZiettClient) EnviarSMS(ctx context.Context, remitterID, destinatarioNacional, conteudo string) (string, error) {
	target, err := formatarDestinatarioZiettE164(destinatarioNacional)
	if err != nil {
		return "", err
	}
	payload := map[string]string{
		"remitter_id":  remitterID,
		"channel_type": comunicacaoZiettChannelSMS,
		"target_e164":  target,
		"content":      conteudo,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &ComunicacaoZiettNetworkError{Err: err}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusAccepted {
		var parsed struct {
			MessageID string `json:"message_id"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return "", err
		}
		return parsed.MessageID, nil
	}
	var apiErr ComunicacaoZiettAPIError
	if err := json.Unmarshal(respBody, &apiErr); err != nil || (apiErr.Code == "" && apiErr.Message == "") {
		apiErr = ComunicacaoZiettAPIError{Code: "ZIETT_ERROR", Message: "a Ziett retornou erro sem corpo padronizado", Status: resp.StatusCode}
	}
	if apiErr.Status == 0 {
		apiErr.Status = resp.StatusCode
	}
	return "", &apiErr
}
