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
	RespondWithError(c, http.StatusBadRequest, SafeErrorMessage(err), err)
}

func RespondWithInternalError(c *gin.Context, err error) {
	RespondWithError(c, http.StatusInternalServerError, 
		"erro interno do servidor", err)
}

func RespondWithNotFoundError(c *gin.Context, resource string) {
	RespondWithError(c, http.StatusNotFound,
		resource+" não encontrado", nil)
}

func RespondWithUnauthorizedError(c *gin.Context) {
	RespondWithError(c, http.StatusUnauthorized,
		"credenciais inválidas", nil)
}

func RespondWithForbiddenError(c *gin.Context, message string) {
	if message == "" {
		message = "acesso negado"
	}
	RespondWithError(c, http.StatusForbidden, message, nil)
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
		if id, ok := userID.(uuid.UUID); ok {
			return id.String()
		}
	}
	return "anonymous"
}

func logError(le LoggedError) {
	log.Printf(`[ERROR] RequestID=%s Method=%s Path=%s IP=%s User=%s Error=%v`,
		le.RequestID, le.Method, le.Path, le.IP, le.UserID, le.Error)
}

func SafeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	
	errStr := strings.ToLower(err.Error())
	
	errorMessages := map[string]string{
		"no rows":                         "registro não encontrado",
		"duplicate key":                   "registro já existe",
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
			return msg
		}
	}
	
	return err.Error()
}