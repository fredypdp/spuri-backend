package utils

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ErrorResponse struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
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

func SafeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	
	errStr := strings.ToLower(err.Error())
	
	errorMessages := map[string]string{
		"no rows":                         "Registro não encontrado no sistema",
		"duplicate key":                   "Este registro já existe",
		"foreign key constraint":          "Operação inválida: referência inexistente",
		"check constraint":                "Dados fornecidos são inválidos",
		"invalid input syntax":            "Formato de dados inválido",
		"not null":                        "Campo obrigatório não foi preenchido",
		"unique constraint":               "Este valor já está cadastrado",
		"value too long":                  "Valor excede tamanho máximo permitido",
		"permission denied":               "Acesso negado",
		"invalid uuid":                    "Identificador inválido",
		"connection refused":              "Serviço temporariamente indisponível",
		"timeout":                         "Operação demorou muito tempo",
		"bilhete":                         "Bilhete de identidade inválido (deve conter 12 números e 2 letras)",
		"email":                           "Formato de email inválido",
		"senha":                           "Senha deve ter no mínimo 6 caracteres",
		"provincia":                       "Província inválida",
		"periodo":                         "Período inválido",
		"role":                            "Perfil de acesso inválido",
		"type":                            "Tipo inválido",
		"status":                          "Status inválido",
	}
	
	for key, msg := range errorMessages {
		if strings.Contains(errStr, key) {
			return msg
		}
	}
	
	return err.Error()
}