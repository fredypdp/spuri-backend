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
		Error:     userMessage,
		RequestID: requestID,
	})
}

func RespondWithValidationError(c *gin.Context, err error) {
	log.Printf("📋 [RespondWithValidationError] Erro de validação: %v", err)
	RespondWithError(c, http.StatusBadRequest, SafeErrorMessage(err), err)
}

func RespondWithInternalError(c *gin.Context, err error) {
	log.Printf("💥 [RespondWithInternalError] Erro interno: %v", err)
	RespondWithError(c, http.StatusInternalServerError, 
		"erro interno do servidor", err)
}

func RespondWithNotFoundError(c *gin.Context, resource string) {
	log.Printf("🔍 [RespondWithNotFoundError] Recurso não encontrado: %s", resource)
	RespondWithError(c, http.StatusNotFound,
		resource+" não encontrado", nil)
}

func RespondWithUnauthorizedError(c *gin.Context) {
	log.Printf("🔒 [RespondWithUnauthorizedError] Credenciais inválidas - IP: %s", c.ClientIP())
	RespondWithError(c, http.StatusUnauthorized,
		"credenciais inválidas", nil)
}

func RespondWithForbiddenError(c *gin.Context, message string) {
	if message == "" {
		message = "acesso negado"
	}
	log.Printf("⛔ [RespondWithForbiddenError] %s - IP: %s", message, c.ClientIP())
	RespondWithError(c, http.StatusForbidden, message, nil)
}

func getOrCreateRequestID(c *gin.Context) string {
	if reqID, exists := c.Get("request_id"); exists {
		if id, ok := reqID.(string); ok {
			log.Printf("🔖 [getOrCreateRequestID] RequestID existente: %s", id)
			return id
		}
	}
	
	reqID := uuid.New().String()
	c.Set("request_id", reqID)
	log.Printf("🆕 [getOrCreateRequestID] Novo RequestID criado: %s", reqID)
	return reqID
}

func getUserIDFromContext(c *gin.Context) string {
	if userID, exists := c.Get("user_id"); exists {
		// Aceita tanto string quanto uuid.UUID
		switch v := userID.(type) {
		case string:
			log.Printf("👤 [getUserIDFromContext] UserID (string): %s", v)
			return v
		case uuid.UUID:
			log.Printf("👤 [getUserIDFromContext] UserID (UUID): %s", v.String())
			return v.String()
		}
	}
	log.Printf("👻 [getUserIDFromContext] UserID não encontrado - retornando 'anonymous'")
	return "anonymous"
}

func logError(le LoggedError) {
	if le.Error != nil {
		log.Printf(`❌ [ERROR] RequestID=%s Method=%s Path=%s IP=%s User=%s Error=%v`,
			le.RequestID, le.Method, le.Path, le.IP, le.UserID, le.Error)
	} else {
		log.Printf(`⚠️ [WARNING] RequestID=%s Method=%s Path=%s IP=%s User=%s (no error object)`,
			le.RequestID, le.Method, le.Path, le.IP, le.UserID)
	}
}

func SafeErrorMessage(err error) string {
	if err == nil {
		log.Printf("🤷 [SafeErrorMessage] Erro nil recebido")
		return ""
	}
	
	errStr := strings.ToLower(err.Error())
	log.Printf("🔍 [SafeErrorMessage] Processando erro: %s", errStr)
	
	errorMessages := map[string]string{
		"no rows":                         "registro não encontrado",
		"duplicate key":                   "valor já existe",
		"foreign key constraint":          "operação inválida",
		"check constraint":                "dados inválidos",
		"invalid input syntax":            "formato de dados inválido",
		"not null":                        "campo obrigatório não preenchido",
		"unique constraint":               "valor já existe",
		"value too long":                  "valor excede tamanho máximo",
		"permission denied":               "acesso negado",
		"invalid uuid":                    "identificador inválido",
		"connection refused":              "serviço temporariamente indisponível",
		"timeout":                         "operação demorou muito tempo",
	}
	
	for key, msg := range errorMessages {
		if strings.Contains(errStr, key) {
			log.Printf("✅ [SafeErrorMessage] Mensagem mapeada: '%s' -> '%s'", key, msg)
			return msg
		}
	}
	
	log.Printf("⚠️ [SafeErrorMessage] Mensagem não mapeada, retornando original")
	return err.Error()
}