package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
)

const (
	ZiettSMSChannelType = "SMS"
	ziettDefaultBaseURL = "https://api.ziett.co/c/v1"
)

var ziettBaseURL = ziettDefaultBaseURL

type ZiettClient struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

type ZiettSendMessageRequest struct {
	RemitterID string
	TargetE164 string
	Content    string
}

type ZiettSendMessageResult struct {
	MessageID  string `json:"message_id"`
	TargetE164 string `json:"target_e164"`
}

type ZiettAPIError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Status    int                    `json:"status"`
	TraceID   string                 `json:"trace_id"`
	Timestamp string                 `json:"timestamp"`
	Service   string                 `json:"service"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

func (e *ZiettAPIError) Error() string {
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

type ZiettNetworkError struct{ Err error }

func (e *ZiettNetworkError) Error() string { return "falha ao contactar a Ziett" }
func (e *ZiettNetworkError) Unwrap() error { return e.Err }

func NewZiettClientFromEnv() (*ZiettClient, error) {
	apiKey := strings.TrimSpace(os.Getenv("ZIETT_API_KEY"))
	if apiKey == "" {
		return nil, errors.New("ZIETT_API_KEY não configurada")
	}
	return &ZiettClient{apiKey: apiKey, httpClient: &http.Client{Timeout: 15 * time.Second}, baseURL: ziettBaseURL}, nil
}

func NewZiettClientForTest(apiKey, baseURL string, httpClient *http.Client) *ZiettClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &ZiettClient{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

func FormatAngolanMobileE164(raw string) (string, error) {
	clean := strings.TrimSpace(raw)
	clean = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(clean)
	if clean == "" {
		return "", errors.New("target_e164 é obrigatório e deve usar o número nacional angolano de 9 dígitos, exemplo: 923456789")
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
			return "", errors.New("target_e164 deve conter apenas dígitos após normalização, no formato nacional angolano de 9 dígitos, exemplo: 923456789")
		}
	}
	if len(clean) != 9 || !strings.HasPrefix(clean, "9") {
		return "", errors.New("target_e164 deve ser um número móvel angolano de 9 dígitos iniciado por 9, sem 0 inicial e sem +244, exemplo: 923456789")
	}
	return "+244" + clean, nil
}

func (c *ZiettClient) SendSMS(ctx context.Context, req ZiettSendMessageRequest) (*ZiettSendMessageResult, error) {
	target, err := FormatAngolanMobileE164(req.TargetE164)
	if err != nil {
		return nil, err
	}
	payload := map[string]string{"remitter_id": req.RemitterID, "channel_type": ZiettSMSChannelType, "target_e164": target, "content": req.Content}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-KEY", c.apiKey)
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ZiettNetworkError{Err: err}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusAccepted {
		var parsed struct {
			MessageID string `json:"message_id"`
		}
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return nil, err
		}
		return &ZiettSendMessageResult{MessageID: parsed.MessageID, TargetE164: target}, nil
	}
	var apiErr ZiettAPIError
	if err := json.Unmarshal(respBody, &apiErr); err != nil || (apiErr.Code == "" && apiErr.Message == "") {
		apiErr = ZiettAPIError{Code: "ZIETT_ERROR", Message: "A Ziett retornou erro sem corpo padronizado", Status: resp.StatusCode}
	}
	if apiErr.Status == 0 {
		apiErr.Status = resp.StatusCode
	}
	return nil, &apiErr
}
