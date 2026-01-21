// ============================================================================
// ARQUIVO: internal/middleware/auth_test.go
// Testes para middleware de autenticação
// ============================================================================

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.Default()
}

func TestGenerateToken(t *testing.T) {
	userID := uuid.New()

	t.Run("should generate token for estudante", func(t *testing.T) {
		token, err := GenerateToken(userID, "estudante")

		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("should generate token for academia", func(t *testing.T) {
		token, err := GenerateToken(userID, "academia")

		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("should generate token for admin", func(t *testing.T) {
		token, err := GenerateToken(userID, "admin")

		assert.NoError(t, err)
		assert.NotEmpty(t, token)
	})

	t.Run("tokens should be different", func(t *testing.T) {
		token1, _ := GenerateToken(userID, "estudante")
		time.Sleep(2 * time.Second) // Garantir timestamps diferentes
		token2, _ := GenerateToken(userID, "estudante")

		assert.NotEqual(t, token1, token2)
	})
}

func TestAuthMiddleware(t *testing.T) {
	router := setupTestRouter()
	
	protected := router.Group("/protected")
	protected.Use(AuthMiddleware())
	protected.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	t.Run("should reject request without token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/protected/test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("should reject request with invalid token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/protected/test", nil)
		req.Header.Set("Authorization", "Bearer invalid_token")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("should accept request with valid token", func(t *testing.T) {
		userID := uuid.New()
		token, _ := GenerateToken(userID, "estudante")

		req := httptest.NewRequest("GET", "/protected/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("should reject token without Bearer prefix", func(t *testing.T) {
		userID := uuid.New()
		token, _ := GenerateToken(userID, "estudante")

		req := httptest.NewRequest("GET", "/protected/test", nil)
		req.Header.Set("Authorization", token)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestRequireEstudante(t *testing.T) {
	router := setupTestRouter()
	
	estudanteRoute := router.Group("/estudante")
	estudanteRoute.Use(AuthMiddleware())
	estudanteRoute.Use(RequireEstudante())
	estudanteRoute.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	t.Run("should allow estudante", func(t *testing.T) {
		userID := uuid.New()
		token, _ := GenerateToken(userID, "estudante")

		req := httptest.NewRequest("GET", "/estudante/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("should reject academia", func(t *testing.T) {
		userID := uuid.New()
		token, _ := GenerateToken(userID, "academia")

		req := httptest.NewRequest("GET", "/estudante/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("should reject admin", func(t *testing.T) {
		userID := uuid.New()
		token, _ := GenerateToken(userID, "admin")

		req := httptest.NewRequest("GET", "/estudante/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestRequireAcademia(t *testing.T) {
	router := setupTestRouter()
	
	academiaRoute := router.Group("/academia")
	academiaRoute.Use(AuthMiddleware())
	academiaRoute.Use(RequireAcademia())
	academiaRoute.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	t.Run("should allow academia", func(t *testing.T) {
		userID := uuid.New()
		token, _ := GenerateToken(userID, "academia")

		req := httptest.NewRequest("GET", "/academia/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("should reject estudante", func(t *testing.T) {
		userID := uuid.New()
		token, _ := GenerateToken(userID, "estudante")

		req := httptest.NewRequest("GET", "/academia/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestRequireAdmin(t *testing.T) {
	router := setupTestRouter()
	
	adminRoute := router.Group("/admin")
	adminRoute.Use(AuthMiddleware())
	adminRoute.Use(RequireAdmin())
	adminRoute.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	t.Run("should allow admin", func(t *testing.T) {
		userID := uuid.New()
		token, _ := GenerateToken(userID, "admin")

		req := httptest.NewRequest("GET", "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("should reject non-admin", func(t *testing.T) {
		userID := uuid.New()
		token, _ := GenerateToken(userID, "estudante")

		req := httptest.NewRequest("GET", "/admin/test", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestGetUserID(t *testing.T) {
	t.Run("should return user ID from context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		expectedID := uuid.New()
		c.Set("user_id", expectedID)

		userID, ok := GetUserID(c)

		assert.True(t, ok)
		assert.Equal(t, expectedID, userID)
	})

	t.Run("should return false when user ID not in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, ok := GetUserID(c)

		assert.False(t, ok)
	})
}

func TestGetUserType(t *testing.T) {
	t.Run("should return user type from context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("user_type", "estudante")

		userType, ok := GetUserType(c)

		assert.True(t, ok)
		assert.Equal(t, "estudante", userType)
	})

	t.Run("should return false when user type not in context", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())

		_, ok := GetUserType(c)

		assert.False(t, ok)
	})
}