package middleware

import (
	"log"
	"net/http"
	"os"
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
	if secret == "" {
		secret = "seu_segredo_muito_secreto_aqui_mude_em_producao"
		log.Printf("⚠️ [JWT] Usando secret padrão - configure JWT_SECRET em produção!")
	}
	jwtSecret = []byte(secret)
	log.Printf("✅ [JWT] Secret configurado")
}

func GenerateToken(userID uuid.UUID, userType string) (string, error) {
	log.Printf("🔑 [GenerateToken] Gerando token - UserID: %s, UserType: %s", userID, userType)
	
	expiryHours := 24
	if hours := os.Getenv("JWT_EXPIRY_HOURS"); hours != "" {
		log.Printf("📝 [GenerateToken] JWT_EXPIRY_HOURS configurado: %s (ignorado, usando padrão)", hours)
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
	tokenString, err := token.SignedString(jwtSecret)
	
	if err != nil {
		log.Printf("❌ [GenerateToken] Erro ao assinar token: %v", err)
		return "", err
	}
	
	log.Printf("✅ [GenerateToken] Token gerado com sucesso - Expira em %dh", expiryHours)
	return tokenString, nil
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("🔍 [AuthMiddleware] Verificando autenticação - Path: %s, Method: %s", 
			c.Request.URL.Path, c.Request.Method)
		
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			log.Printf("❌ [AuthMiddleware] Token não fornecido - IP: %s", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token não fornecido"})
			c.Abort()
			return
		}

		log.Printf("📝 [AuthMiddleware] Header Authorization encontrado")

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			log.Printf("❌ [AuthMiddleware] Formato de token inválido (sem 'Bearer ')")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "formato de token inválido"})
			c.Abort()
			return
		}

		log.Printf("🔓 [AuthMiddleware] Validando token...")

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
		
		log.Printf("✅ [AuthMiddleware] Autenticado - UserID: %s, UserType: %s, IP: %s", 
			claims.UserID, claims.UserType, c.ClientIP())
		c.Next()
	}
}

func RequireAcademia() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("🏫 [RequireAcademia] Verificando tipo de usuário - Path: %s", c.Request.URL.Path)
		
		userType, exists := c.Get("user_type")
		if !exists {
			log.Printf("❌ [RequireAcademia] user_type não encontrado no contexto")
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas academias"})
			c.Abort()
			return
		}
		
		log.Printf("🔍 [RequireAcademia] UserType: %v", userType)
		
		if userType != "academia" {
			log.Printf("❌ [RequireAcademia] Tipo incorreto: %v (esperado: academia)", userType)
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas academias"})
			c.Abort()
			return
		}
		
		log.Printf("✅ [RequireAcademia] OK")
		c.Next()
	}
}

func RequireEstudante() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("🎓 [RequireEstudante] Verificando tipo de usuário - Path: %s", c.Request.URL.Path)
		
		userType, exists := c.Get("user_type")
		if !exists {
			log.Printf("❌ [RequireEstudante] user_type não encontrado no contexto")
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas estudantes"})
			c.Abort()
			return
		}
		
		log.Printf("🔍 [RequireEstudante] UserType: %v", userType)
		
		if userType != "estudante" {
			log.Printf("❌ [RequireEstudante] Tipo incorreto: %v (esperado: estudante)", userType)
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas estudantes"})
			c.Abort()
			return
		}
		
		log.Printf("✅ [RequireEstudante] OK")
		c.Next()
	}
}

func GetUserID(c *gin.Context) (uuid.UUID, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		log.Printf("⚠️ [GetUserID] user_id não encontrado no contexto")
		return uuid.Nil, false
	}
	
	id, ok := userID.(uuid.UUID)
	if !ok {
		log.Printf("❌ [GetUserID] user_id não é UUID válido: %v", userID)
		return uuid.Nil, false
	}
	
	return id, ok
}

func GetUserType(c *gin.Context) (string, bool) {
	userType, exists := c.Get("user_type")
	if !exists {
		log.Printf("⚠️ [GetUserType] user_type não encontrado no contexto")
		return "", false
	}
	
	t, ok := userType.(string)
	if !ok {
		log.Printf("❌ [GetUserType] user_type não é string válida: %v", userType)
		return "", false
	}
	
	return t, ok
}