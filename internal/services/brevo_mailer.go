// Envio de email via a API HTTP do Brevo (https://api.brevo.com/v3/smtp/email).
package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

var brevoHTTPClient = &http.Client{Timeout: 15 * time.Second}

// BrevoConfig agrupa a configuração de envio via API do Brevo.
type BrevoConfig struct {
	APIKey      string
	SenderEmail string // precisa estar verificado em app.brevo.com/settings/senders
	SenderName  string
}

// loadBrevoConfig lê a configuração do ambiente. Reaproveita EMAIL_FROM
// (com fallback para EMAIL_USER) como remetente.
func loadBrevoConfig() (BrevoConfig, bool) {
	sender := getEnvOrDefault("EMAIL_FROM", os.Getenv("EMAIL_USER"))
	cfg := BrevoConfig{
		APIKey:      os.Getenv("BREVO_API_KEY"),
		SenderEmail: sender,
		SenderName:  "Spuri",
	}
	if cfg.APIKey == "" || cfg.SenderEmail == "" {
		return cfg, false
	}
	return cfg, true
}

type brevoEmailAddress struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

type brevoSendRequest struct {
	Sender      brevoEmailAddress   `json:"sender"`
	To          []brevoEmailAddress `json:"to"`
	Subject     string              `json:"subject"`
	HTMLContent string              `json:"htmlContent"`
	TextContent string              `json:"textContent,omitempty"`
}

type brevoSendResponse struct {
	MessageID string `json:"messageId"`
}

type brevoErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// sendBrevoEmail envia um email via a API REST do Brevo.
func sendBrevoEmail(cfg BrevoConfig, toEmail, toName, subject, textBody, htmlBody string) error {
	return sendBrevoEmailToURL(cfg, "https://api.brevo.com/v3/smtp/email", toEmail, toName, subject, textBody, htmlBody)
}

// sendBrevoEmailToURL permite apontar para um servidor mock nos testes.
func sendBrevoEmailToURL(cfg BrevoConfig, url, toEmail, toName, subject, textBody, htmlBody string) error {
	reqBody := brevoSendRequest{
		Sender:      brevoEmailAddress{Email: cfg.SenderEmail, Name: cfg.SenderName},
		To:          []brevoEmailAddress{{Email: toEmail, Name: toName}},
		Subject:     subject,
		HTMLContent: htmlBody,
		TextContent: textBody,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("erro ao serializar email: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("erro ao montar requisição: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("api-key", cfg.APIKey)

	resp, err := brevoHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("erro na requisição ao Brevo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var ok brevoSendResponse
		_ = json.NewDecoder(resp.Body).Decode(&ok)
		return nil
	}

	var apiErr brevoErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&apiErr)
	if apiErr.Message != "" {
		return fmt.Errorf("brevo respondeu %d: %s (%s)", resp.StatusCode, apiErr.Message, apiErr.Code)
	}
	return fmt.Errorf("brevo respondeu status %d", resp.StatusCode)
}
