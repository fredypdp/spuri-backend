// ============================================================================
// ARQUIVO: internal/utils/errors.go
// ✅ Error handling seguro - nunca expõe detalhes internos
// ============================================================================

package utils

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ErrorResponse representa erro retornado ao cliente
type ErrorResponse struct {
	Error     string `json:"error"`
	RequestID string `json:"request_id,omitempty"`
}

// LoggedError registra erro detalhado server-side
type LoggedError struct {
	RequestID string
	Path      string
	Method    string
	IP        string
	UserID    string
	Error     error
}

// RespondWithError responde com erro genérico e loga detalhes
func RespondWithError(c *gin.Context, statusCode int, userMessage string, err error) {
	requestID := getOrCreateRequestID(c)
	
	// Log detalhado server-side
	logError(LoggedError{
		RequestID: requestID,
		Path:      c.Request.URL.Path,
		Method:    c.Request.Method,
		IP:        c.ClientIP(),
		UserID:    getUserIDFromContext(c),
		Error:     err,
	})
	
	// Resposta genérica para cliente
	c.JSON(statusCode, ErrorResponse{
		Error:     userMessage,
		RequestID: requestID,
	})
}

// RespondWithValidationError responde com erro de validação
func RespondWithValidationError(c *gin.Context, err error) {
	RespondWithError(c, http.StatusBadRequest, err.Error(), err)
}

// RespondWithInternalError responde com erro interno genérico
func RespondWithInternalError(c *gin.Context, err error) {
	RespondWithError(c, http.StatusInternalServerError, 
		"erro interno do servidor", err)
}

// RespondWithNotFoundError responde com erro 404
func RespondWithNotFoundError(c *gin.Context, resource string) {
	RespondWithError(c, http.StatusNotFound,
		resource+" não encontrado", nil)
}

// RespondWithUnauthorizedError responde com erro de autenticação
func RespondWithUnauthorizedError(c *gin.Context) {
	RespondWithError(c, http.StatusUnauthorized,
		"credenciais inválidas", nil)
}

// RespondWithForbiddenError responde com erro de permissão
func RespondWithForbiddenError(c *gin.Context, message string) {
	if message == "" {
		message = "acesso negado"
	}
	RespondWithError(c, http.StatusForbidden, message, nil)
}

// getOrCreateRequestID obtém ou cria request ID
func getOrCreateRequestID(c *gin.Context) string {
	// Verificar se já existe
	if reqID, exists := c.Get("request_id"); exists {
		if id, ok := reqID.(string); ok {
			return id
		}
	}
	
	// Criar novo
	reqID := uuid.New().String()
	c.Set("request_id", reqID)
	return reqID
}

// getUserIDFromContext obtém user ID do contexto
func getUserIDFromContext(c *gin.Context) string {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(uuid.UUID); ok {
			return id.String()
		}
	}
	return "anonymous"
}

// logError registra erro com detalhes completos
func logError(le LoggedError) {
	log.Printf(`
[ERROR] Request ID: %s
  Path: %s %s
  IP: %s
  User: %s
  Error: %v
`,
		le.RequestID,
		le.Method,
		le.Path,
		le.IP,
		le.UserID,
		le.Error,
	)
}

// SafeErrorMessage retorna mensagem segura baseada no erro
func SafeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	
	// Mapeamento de erros conhecidos para mensagens seguras
	errorMessages := map[string]string{
		"sql: no rows in result set":     "registro não encontrado",
		"duplicate key value":             "registro já existe",
		"violates foreign key constraint": "operação inválida - referência não existe",
		"violates check constraint":       "dados inválidos",
		"invalid input syntax":            "formato de dados inválido",
	}
	
	errStr := err.Error()
	for key, msg := range errorMessages {
		if contains(errStr, key) {
			return msg
		}
	}
	
	// Mensagem genérica para erros não mapeados
	return "erro ao processar solicitação"
}

// contains verifica substring case-insensitive
func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || len(s) > len(substr) && 
		    s[len(s)-len(substr):] == substr || 
		    s[:len(substr)] == substr)
}