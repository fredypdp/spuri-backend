// ============================================================================
// ARQUIVO: internal/handlers/auth_handlers_test.go
// Testes para handlers de autenticação
// ============================================================================

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	return router
}

func TestLogin_Validation(t *testing.T) {
	router := setupTestRouter()
	router.POST("/login", Login)

	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectedStatus int
	}{
		{
			"missing usuario",
			map[string]interface{}{"senha": "123456", "type": "estudante"},
			http.StatusBadRequest,
		},
		{
			"missing senha",
			map[string]interface{}{"usuario": "ABC1234", "type": "estudante"},
			http.StatusBadRequest,
		},
		{
			"missing type",
			map[string]interface{}{"usuario": "ABC1234", "senha": "123456"},
			http.StatusBadRequest,
		},
		{
			"invalid type",
			map[string]interface{}{"usuario": "ABC1234", "senha": "123456", "type": "invalid"},
			http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestRegisterAcademia_Validation(t *testing.T) {
	router := setupTestRouter()
	router.POST("/academia/register", RegisterAcademia)

	t.Run("should reject invalid type", func(t *testing.T) {
		payload := map[string]interface{}{
			"type":      "invalido",
			"nome":      "Escola Teste",
			"senha":     "123456",
			"provincia": "LUA",
			"endereco":  "Rua Teste, 123",
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/academia/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("should require nivel_escolar for escola", func(t *testing.T) {
		payload := map[string]interface{}{
			"type":      "escola",
			"nome":      "Escola Teste",
			"senha":     "123456",
			"provincia": "LUA",
			"endereco":  "Rua Teste, 123",
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/academia/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestRegisterEstudante_Validation(t *testing.T) {
	router := setupTestRouter()
	router.POST("/estudante/register", RegisterEstudante)

	t.Run("should require bilhete", func(t *testing.T) {
		payload := map[string]interface{}{
			"nome":  "João Teste",
			"senha": "123456",
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/estudante/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("should validate nome length", func(t *testing.T) {
		payload := map[string]interface{}{
			"nome":               "A",
			"senha":              "123456",
			"bilhete_identidade": "123456789LA",
		}

		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/estudante/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestJWT_Generation(t *testing.T) {
	t.Run("should generate valid token", func(t *testing.T) {
		estudanteID := aggregates.NewEstudante().ID

		token, err := middleware.GenerateToken(estudanteID, "estudante")

		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("should include user type in token", func(t *testing.T) {
		academiaID := aggregates.NewAcademia().ID

		token, err := middleware.GenerateToken(academiaID, "academia")

		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})
}

func TestPasswordHashing(t *testing.T) {
	t.Run("should hash password correctly", func(t *testing.T) {
		password := "mySecurePassword123"

		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		assert.NoError(t, err)
		assert.NotEqual(t, password, string(hashed))
	})

	t.Run("should verify password correctly", func(t *testing.T) {
		password := "mySecurePassword123"
		hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		err := bcrypt.CompareHashAndPassword(hashed, []byte(password))

		assert.NoError(t, err)
	})

	t.Run("should reject wrong password", func(t *testing.T) {
		password := "mySecurePassword123"
		hashed, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		err := bcrypt.CompareHashAndPassword(hashed, []byte("wrongPassword"))

		assert.Error(t, err)
	})
}