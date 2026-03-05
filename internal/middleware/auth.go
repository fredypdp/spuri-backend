package middleware

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"spuri/internal/db"
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

// AuthMiddleware valida o JWT e verifica que o usuário ainda está ativo no banco.
//
// H4-17: além de validar assinatura e expiração, este middleware consulta a
// projeção de leitura correspondente ao userType e rejeita tokens de usuários
// com status diferente de "ativo". Isso garante que a desativação de um usuário
// tem efeito imediato — sem esperar a expiração natural do token.
//
// Fluxo:
//  1. Extrair e validar JWT (assinatura + expiração)
//  2. Obter dbClient do contexto (injetado pelo setupRouter)
//  3. Consultar status do usuário na projeção correspondente
//  4. Rejeitar com 401 se status != "ativo"
//  5. Injetar user_id e user_type no contexto Gin
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

		// H4-17: verificar status do usuário no banco.
		// Admins são verificados por RequireAdmin/RequireAdminRole que já
		// consultam projection_admins com verificação de status. Para não
		// duplicar a query de admin, verificamos apenas estudante e academia aqui.
		// A verificação de admin é delegada ao middleware RequireAdmin.
		if claims.UserType == "estudante" || claims.UserType == "academia" {
			if err := verificarStatusUsuario(c, claims.UserID, claims.UserType); err != nil {
				log.Printf("❌ [AuthMiddleware] Usuário inativo ou não encontrado - UserID: %s, Type: %s: %v",
					claims.UserID, claims.UserType, err)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "conta inativa ou não encontrada. Entre em contato com o suporte.",
				})
				c.Abort()
				return
			}
		}

		c.Set("user_id", claims.UserID)
		c.Set("user_type", claims.UserType)

		log.Printf("✅ [AuthMiddleware] Autenticado - UserID: %s, UserType: %s", claims.UserID, claims.UserType)
		c.Next()
	}
}

// verificarStatusUsuario consulta a projeção correspondente ao userType e
// retorna erro se o usuário não existir ou não estiver ativo.
// Usa prepared statement — sem interpolação de string.
func verificarStatusUsuario(c *gin.Context, userID uuid.UUID, userType string) error {
	clientRaw, exists := c.Get("dbClient")
	if !exists {
		// Se o dbClient não estiver no contexto (erro de configuração de router),
		// permitimos a passagem para não bloquear o arranque — o handler
		// seguinte falhará com 500 via getDbClient(). Logamos o problema.
		log.Printf("⚠️ [AuthMiddleware] dbClient ausente no contexto ao verificar status — path: %s", c.Request.URL.Path)
		return nil
	}
	client := clientRaw.(*db.Client)

	var table string
	switch userType {
	case "estudante":
		table = "projection_estudantes"
	case "academia":
		table = "projection_academias"
	default:
		// Tipo desconhecido — não bloqueamos; o middleware específico de role tratará.
		return nil
	}

	// A query usa $1 como prepared statement; table é uma constante interna
	// (nunca vem do usuário), portanto a interpolação de nome de tabela é segura.
	query := `SELECT status FROM ` + table + ` WHERE id = $1`

	var status string
	err := client.DB().QueryRow(query, userID).Scan(&status)
	if err == sql.ErrNoRows {
		return sql.ErrNoRows
	}
	if err != nil {
		log.Printf("⚠️ [AuthMiddleware] Erro ao verificar status do usuário %s: %v", userID, err)
		// Em caso de erro de banco (ex: timeout), permitimos a passagem com aviso
		// para não derrubar o serviço inteiro por instabilidade pontual do BD.
		return nil
	}

	if status != "ativo" {
		return &statusInativoError{userType: userType, status: status}
	}
	return nil
}

// statusInativoError representa o erro de usuário inativo.
type statusInativoError struct {
	userType string
	status   string
}

func (e *statusInativoError) Error() string {
	return e.userType + " está com status: " + e.status
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

// RequireAdmin está definido em admin_auth_middleware.go.
// Mantido aqui apenas como referência — não redeclarar.

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
	if !ok {
		return uuid.Nil, false
	}
	return uid, true
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