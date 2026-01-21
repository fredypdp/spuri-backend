// ============================================================================
// ARQUIVO: internal/utils/errors_test.go
// Testes para tratamento de erros
// ============================================================================

package utils

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func setupTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestSafeErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected string
	}{
		{
			"no rows",
			errors.New("sql: no rows in result set"),
			"registro não encontrado",
		},
		{
			"duplicate key",
			errors.New("pq: duplicate key value violates unique constraint"),
			"registro já existe",
		},
		{
			"foreign key",
			errors.New("pq: foreign key constraint violated"),
			"operação inválida",
		},
		{
			"not null",
			errors.New("pq: not null constraint violated"),
			"campo obrigatório não preenchido",
		},
		{
			"unique constraint",
			errors.New("pq: unique constraint violation"),
			"valor já existe",
		},
		{
			"invalid uuid",
			errors.New("invalid UUID format"),
			"identificador inválido",
		},
		{
			"connection refused",
			errors.New("dial tcp: connection refused"),
			"serviço temporariamente indisponível",
		},
		{
			"timeout",
			errors.New("context deadline exceeded timeout"),
			"operação demorou muito tempo",
		},
		{
			"unknown error",
			errors.New("some random error"),
			"some random error",
		},
		{
			"nil error",
			nil,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SafeErrorMessage(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRespondWithError(t *testing.T) {
	t.Run("should set status code and error message", func(t *testing.T) {
		c, w := setupTestContext()
		
		RespondWithError(c, 400, "test error", errors.New("internal"))
		
		assert.Equal(t, 400, w.Code)
		assert.Contains(t, w.Body.String(), "test error")
	})

	t.Run("should include request ID if available", func(t *testing.T) {
		c, w := setupTestContext()
		c.Set("request_id", "test-123")
		
		RespondWithError(c, 400, "test error", nil)
		
		assert.Contains(t, w.Body.String(), "request_id")
		assert.Contains(t, w.Body.String(), "test-123")
	})
}

func TestRespondWithValidationError(t *testing.T) {
	t.Run("should respond with 400", func(t *testing.T) {
		c, w := setupTestContext()
		
		RespondWithValidationError(c, errors.New("validation failed"))
		
		assert.Equal(t, 400, w.Code)
		assert.Contains(t, w.Body.String(), "validation failed")
	})
}

func TestRespondWithInternalError(t *testing.T) {
	t.Run("should respond with 500", func(t *testing.T) {
		c, w := setupTestContext()
		
		RespondWithInternalError(c, errors.New("internal"))
		
		assert.Equal(t, 500, w.Code)
		assert.Contains(t, w.Body.String(), "erro interno")
	})

	t.Run("should hide internal error details", func(t *testing.T) {
		c, w := setupTestContext()
		
		RespondWithInternalError(c, errors.New("database password is 12345"))
		
		assert.NotContains(t, w.Body.String(), "password")
		assert.NotContains(t, w.Body.String(), "12345")
	})
}

func TestRespondWithNotFoundError(t *testing.T) {
	t.Run("should respond with 404", func(t *testing.T) {
		c, w := setupTestContext()
		
		RespondWithNotFoundError(c, "estudante")
		
		assert.Equal(t, 404, w.Code)
		assert.Contains(t, w.Body.String(), "estudante não encontrado")
	})
}

func TestRespondWithUnauthorizedError(t *testing.T) {
	t.Run("should respond with 401", func(t *testing.T) {
		c, w := setupTestContext()
		
		RespondWithUnauthorizedError(c)
		
		assert.Equal(t, 401, w.Code)
		assert.Contains(t, w.Body.String(), "credenciais inválidas")
	})
}

func TestRespondWithForbiddenError(t *testing.T) {
	t.Run("should respond with 403", func(t *testing.T) {
		c, w := setupTestContext()
		
		RespondWithForbiddenError(c, "custom message")
		
		assert.Equal(t, 403, w.Code)
		assert.Contains(t, w.Body.String(), "custom message")
	})

	t.Run("should use default message when empty", func(t *testing.T) {
		c, w := setupTestContext()
		
		RespondWithForbiddenError(c, "")
		
		assert.Contains(t, w.Body.String(), "acesso negado")
	})
}

func TestGetOrCreateRequestID(t *testing.T) {
	t.Run("should return existing request ID", func(t *testing.T) {
		c, _ := setupTestContext()
		c.Set("request_id", "existing-123")
		
		reqID := getOrCreateRequestID(c)
		
		assert.Equal(t, "existing-123", reqID)
	})

	t.Run("should create new request ID if not exists", func(t *testing.T) {
		c, _ := setupTestContext()
		
		reqID := getOrCreateRequestID(c)
		
		assert.NotEmpty(t, reqID)
		assert.Equal(t, 36, len(reqID)) // UUID format
	})
}

func TestGetUserIDFromContext(t *testing.T) {
	t.Run("should return user ID when present", func(t *testing.T) {
		c, _ := setupTestContext()
		c.Set("user_id", "user-123")
		
		userID := getUserIDFromContext(c)
		
		assert.Contains(t, userID, "user")
	})

	t.Run("should return anonymous when not present", func(t *testing.T) {
		c, _ := setupTestContext()
		
		userID := getUserIDFromContext(c)
		
		assert.Equal(t, "anonymous", userID)
	})
}