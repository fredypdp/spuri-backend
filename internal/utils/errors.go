package utils

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type ErrorResponse struct {
	Error     string             `json:"error"`
	Message   string             `json:"message"`
	RequestID string             `json:"request_id,omitempty"`
	Details   []ValidationDetail `json:"details,omitempty"`
}

type ValidationDetail struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type LoggedError struct {
	RequestID string
	Path      string
	Method    string
	IP        string
	UserID    string
	Error     error
}

func RespondWithError(c *gin.Context, statusCode int, userMessage string, err error) {
	requestID := getOrCreateRequestID(c)
	details := extractValidationDetails(err)

	log.Printf("⚠️ [RespondWithError] Status: %d - Message: %s - RequestID: %s",
		statusCode, userMessage, requestID)

	logError(LoggedError{
		RequestID: requestID,
		Path:      c.Request.URL.Path,
		Method:    c.Request.Method,
		IP:        c.ClientIP(),
		UserID:    getUserIDFromContext(c),
		Error:     err,
	})

	c.JSON(statusCode, ErrorResponse{
		Error:     getErrorType(statusCode),
		Message:   userMessage,
		RequestID: requestID,
		Details:   details,
	})
}

func RespondWithValidationError(c *gin.Context, err error) {
	message := SafeErrorMessage(err)
	log.Printf("📋 [RespondWithValidationError] %s", message)
	RespondWithError(c, http.StatusBadRequest, message, err)
}

func RespondWithInternalError(c *gin.Context, err error) {
	log.Printf("💥 [RespondWithInternalError] Erro interno: %v", err)
	RespondWithError(c, http.StatusInternalServerError,
		"Erro interno do servidor. Tente novamente mais tarde.", err)
}

func RespondWithNotFoundError(c *gin.Context, resource string) {
	message := resource + " não encontrado"
	log.Printf("🔍 [RespondWithNotFoundError] %s", message)
	RespondWithError(c, http.StatusNotFound, message, nil)
}

func RespondWithUnauthorizedError(c *gin.Context) {
	log.Printf("🔒 [RespondWithUnauthorizedError] IP: %s", c.ClientIP())
	RespondWithError(c, http.StatusUnauthorized,
		"Credenciais inválidas. Verifique usuário e senha.", nil)
}

func RespondWithForbiddenError(c *gin.Context, message string) {
	if message == "" {
		message = "Acesso negado. Você não tem permissão para esta ação."
	}
	log.Printf("⛔ [RespondWithForbiddenError] %s - IP: %s", message, c.ClientIP())
	RespondWithError(c, http.StatusForbidden, message, nil)
}

func RespondWithConflictError(c *gin.Context, message string) {
	log.Printf("⚠️ [RespondWithConflictError] %s", message)
	RespondWithError(c, http.StatusConflict, message, nil)
}

func getErrorType(statusCode int) string {
	switch statusCode {
	case http.StatusBadRequest:
		return "VALIDATION_ERROR"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusConflict:
		return "CONFLICT"
	case http.StatusTooManyRequests:
		return "RATE_LIMIT"
	case http.StatusInternalServerError:
		return "INTERNAL_ERROR"
	default:
		return "ERROR"
	}
}

func getOrCreateRequestID(c *gin.Context) string {
	if reqID, exists := c.Get("request_id"); exists {
		if id, ok := reqID.(string); ok {
			return id
		}
	}

	reqID := uuid.New().String()
	c.Set("request_id", reqID)
	return reqID
}

func getUserIDFromContext(c *gin.Context) string {
	if userID, exists := c.Get("user_id"); exists {
		switch v := userID.(type) {
		case string:
			return v
		case uuid.UUID:
			return v.String()
		}
	}
	return "anonymous"
}

func logError(le LoggedError) {
	if le.Error != nil {
		log.Printf(`❌ [ERROR] RequestID=%s Method=%s Path=%s IP=%s User=%s Error=%v`,
			le.RequestID, le.Method, le.Path, le.IP, le.UserID, le.Error)
	} else {
		log.Printf(`⚠️ [WARNING] RequestID=%s Method=%s Path=%s IP=%s User=%s`,
			le.RequestID, le.Method, le.Path, le.IP, le.UserID)
	}
}

func extractValidationDetails(err error) []ValidationDetail {
	if err == nil {
		return nil
	}

	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		return nil
	}

	details := make([]ValidationDetail, 0, len(validationErrors))
	for _, ve := range validationErrors {
		field := toJSONFieldName(ve)
		details = append(details, ValidationDetail{
			Field:   field,
			Code:    ve.Tag(),
			Message: humanizeValidationMessage(field, ve),
		})
	}
	return details
}

func toJSONFieldName(fe validator.FieldError) string {
	if structField := fe.StructField(); structField != "" {
		return camelToSnake(structField)
	}
	if field := fe.Field(); field != "" {
		return camelToSnake(field)
	}
	return "campo"
}

func humanizeValidationMessage(field string, ve validator.FieldError) string {
	switch ve.Tag() {
	case "required":
		return fmt.Sprintf("o campo '%s' é obrigatório", field)
	case "email":
		return fmt.Sprintf("o campo '%s' deve ter formato de email válido", field)
	case "oneof":
		if param := strings.TrimSpace(ve.Param()); param != "" {
			return fmt.Sprintf("o campo '%s' deve ser um dos valores: %s", field, param)
		}
		return fmt.Sprintf("o campo '%s' possui valor inválido", field)
	case "min":
		return fmt.Sprintf("o campo '%s' deve ter no mínimo %s caracteres", field, ve.Param())
	case "max":
		return fmt.Sprintf("o campo '%s' deve ter no máximo %s caracteres", field, ve.Param())
	default:
		return fmt.Sprintf("o campo '%s' é inválido", field)
	}
}

func camelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func SafeErrorMessage(err error) string {
	if err == nil {
		return ""
	}

	errStr := strings.ToLower(err.Error())

	// Regras específicas primeiro para evitar colisões com chaves genéricas
	// (ex.: "periodo" em erros de duplicata de nota).
	if strings.Contains(errStr, "nota já registrada") || strings.Contains(errStr, "nota ja registrada") {
		return "Nota já registrada para o mesmo ano/período/matéria/tipo/categoria"
	}

	// Mensagens realmente relacionadas à validade do período.
	if (strings.Contains(errStr, "periodo '") || strings.Contains(errStr, "período '")) &&
		(strings.Contains(errStr, "inválido") || strings.Contains(errStr, "invalido")) {
		return "Período inválido"
	}

	errorRules := []struct {
		key string
		msg string
	}{
		{"no rows", "Registro não encontrado no sistema"},
		{"duplicate key", "Este registro já existe"},
		{"foreign key constraint", "Operação inválida: referência inexistente"},
		{"check constraint", "Dados fornecidos são inválidos"},
		{"invalid input syntax", "Formato de dados inválido"},
		{"not null", "Campo obrigatório não foi preenchido"},
		{"unique constraint", "Este valor já está cadastrado"},
		{"value too long", "Valor excede tamanho máximo permitido"},
		{"permission denied", "Acesso negado"},
		{"invalid uuid", "Identificador inválido"},
		{"connection refused", "Serviço temporariamente indisponível"},
		{"timeout", "Operação demorou muito tempo"},
		{"bilhete", "Bilhete de identidade inválido (deve conter 12 números e 2 letras)"},
		{"email", "Formato de email inválido"},
		{"senha", "Senha deve ter no mínimo 6 caracteres"},
		{"provincia", "Província inválida"},
		{"role", "Perfil de acesso inválido"},
		{"type", "Tipo inválido"},
		{"status", "Status inválido"},
	}

	for _, rule := range errorRules {
		if strings.Contains(errStr, rule.key) {
			return rule.msg
		}
	}

	return err.Error()
}
