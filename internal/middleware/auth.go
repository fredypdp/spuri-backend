// ============================================================================
// ARQUIVO: internal/middleware/auth.go
//
// CORREÇÕES APLICADAS:
//   FIX-A1 — JWT_SECRET: servidor falha fatalmente em produção se não configurado.
//             Antes: apenas logava aviso e continuava com secret público.
//             Agora: em ENV=production, ausência de JWT_SECRET causa log.Fatalf.
// ============================================================================

package middleware

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID   uuid.UUID `json:"user_id"`
	UserType string    `json:"user_type"`
	jwt.RegisteredClaims
}

var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	env := os.Getenv("ENV")

	if secret == "" {
		if env == "production" {
			// FIX-A1: em produção, JWT_SECRET ausente é erro fatal.
			// Sem isso, o servidor sobe com secret público conhecido por qualquer
			// pessoa que leia o código — comprometendo TODOS os tokens.
			log.Fatalf("[FATAL] JWT_SECRET não configurado em produção. Configure a variável de ambiente JWT_SECRET.")
		}
		secret = "seu_segredo_muito_secreto_aqui_mude_em_producao"
		log.Printf("⚠️  [JWT] Usando secret padrão — NÃO USE EM PRODUÇÃO. Configure JWT_SECRET.")
	}
	jwtSecret = []byte(secret)
	log.Printf("✅ [JWT] Secret configurado (%d bytes)", len(jwtSecret))
}

// getJWTExpiryHours retorna o número de horas de validade do token.
func getJWTExpiryHours() int {
	const defaultHours = 24
	hoursStr := os.Getenv("JWT_EXPIRY_HOURS")
	if hoursStr == "" {
		return defaultHours
	}
	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 {
		log.Printf("⚠️  [JWT] JWT_EXPIRY_HOURS inválido ('%s'), usando padrão %dh", hoursStr, defaultHours)
		return defaultHours
	}
	return hours
}

func GenerateToken(userID uuid.UUID, userType string) (string, error) {
	expiryHours := getJWTExpiryHours()

	claims := Claims{
		UserID:   userID,
		UserType: userType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		log.Printf("❌ [GenerateToken] Erro ao assinar token: %v", err)
		return "", err
	}

	log.Printf("✅ [GenerateToken] Token gerado - UserID: %s, Type: %s, Expiry: %dh", userID, userType, expiryHours)
	return tokenString, nil
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		if authHeader == "" {
			log.Printf("❌ [AuthMiddleware] Token não fornecido - IP: %s", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token não fornecido"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			log.Printf("❌ [AuthMiddleware] Formato de token inválido (sem 'Bearer ')")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "formato de token inválido"})
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			log.Printf("❌ [AuthMiddleware] Token inválido ou expirado: %v - IP: %s", err, c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token inválido ou expirado"})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("user_type", claims.UserType)

		log.Printf("✅ [AuthMiddleware] Autenticado - UserID: %s, UserType: %s", claims.UserID, claims.UserType)
		c.Next()
	}
}

func RequireAcademia() gin.HandlerFunc {
	return func(c *gin.Context) {
		userType, exists := c.Get("user_type")
		if !exists || userType != "academia" {
			log.Printf("❌ [RequireAcademia] Acesso negado - UserType: %v", userType)
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas academias"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireEstudante() gin.HandlerFunc {
	return func(c *gin.Context) {
		userType, exists := c.Get("user_type")
		if !exists || userType != "estudante" {
			log.Printf("❌ [RequireEstudante] Acesso negado - UserType: %v", userType)
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas estudantes"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userType, exists := c.Get("user_type")
		if !exists || userType != "admin" {
			log.Printf("❌ [RequireAdmin] Acesso negado - UserType: %v", userType)
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas administradores"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAcademiaOuAdmin bloqueia estudantes — apenas academias e admins passam.
func RequireAcademiaOuAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userType, exists := c.Get("user_type")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
			c.Abort()
			return
		}
		ut, _ := userType.(string)
		if ut != "academia" && ut != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas academias e administradores"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// GetUserID extrai o UUID do usuário autenticado do contexto Gin.
func GetUserID(c *gin.Context) (uuid.UUID, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return uuid.Nil, false
	}
	uid, ok := userID.(uuid.UUID)
	return uid, ok
}

// GetUserType extrai o tipo do usuário autenticado do contexto Gin.
func GetUserType(c *gin.Context) (string, bool) {
	userType, exists := c.Get("user_type")
	if !exists {
		return "", false
	}
	ut, ok := userType.(string)
	return ut, ok
}