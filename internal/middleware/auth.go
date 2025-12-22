package middleware

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims representa as claims do JWT
type Claims struct {
	UserID   uuid.UUID `json:"user_id"`
	UserType string    `json:"user_type"` // "estudante" ou "academia"
	jwt.RegisteredClaims
}

var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "seu_segredo_muito_secreto_aqui_mude_em_producao"
	}
	jwtSecret = []byte(secret)
}

// GenerateToken gera um token JWT
func GenerateToken(userID uuid.UUID, userType string) (string, error) {
	expiryHours := 24
	if hours := os.Getenv("JWT_EXPIRY_HOURS"); hours != "" {
		// Ignorar erro, usar padrão
	}

	claims := Claims{
		UserID:   userID,
		UserType: userType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// AuthMiddleware verifica o token JWT
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token não fornecido"})
			c.Abort()
			return
		}

		// Remover "Bearer " do início
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "formato de token inválido"})
			c.Abort()
			return
		}

		// Parse e validar o token
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token inválido ou expirado"})
			c.Abort()
			return
		}

		// Adicionar claims ao contexto
		c.Set("user_id", claims.UserID)
		c.Set("user_type", claims.UserType)

		c.Next()
	}
}

// RequireAcademia verifica se o usuário é uma academia
func RequireAcademia() gin.HandlerFunc {
	return func(c *gin.Context) {
		userType, exists := c.Get("user_type")
		if !exists || userType != "academia" {
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas academias"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireEstudante verifica se o usuário é um estudante
func RequireEstudante() gin.HandlerFunc {
	return func(c *gin.Context) {
		userType, exists := c.Get("user_type")
		if !exists || userType != "estudante" {
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas estudantes"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetUserID obtém o ID do usuário do contexto
func GetUserID(c *gin.Context) (uuid.UUID, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, false
	}
	
	id, ok := userID.(uuid.UUID)
	return id, ok
}

// GetUserType obtém o tipo do usuário do contexto
func GetUserType(c *gin.Context) (string, bool) {
	userType, exists := c.Get("user_type")
	if !exists {
		return "", false
	}
	
	t, ok := userType.(string)
	return t, ok
}