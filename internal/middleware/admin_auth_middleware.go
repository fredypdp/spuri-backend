package middleware

import (
	"log"
	"net/http"
	"spuri/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireAdmin verifica que o usuário autenticado é um admin com qualquer role válido
// (gerente, adm ou fpp) e que está ativo.
//
// CORRIGIDO P11: antes, este middleware verificava apenas user_type == "admin",
// sem consultar o banco para checar role e status. Isso significava que um token
// de admin inativo ainda passava por rotas protegidas apenas por RequireAdmin.
// Agora: delega para RequireAdminRole("gerente"), que consulta projection_admins
// e verifica status == "ativo" E role >= gerente.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("👤 [RequireAdmin] Verificando admin — Path: %s", c.Request.URL.Path)

		// Verificação rápida de tipo antes de bater no banco.
		userType, exists := c.Get("user_type")
		if !exists || userType != "admin" {
			log.Printf("❌ [RequireAdmin] user_type ausente ou incorreto: %v", userType)
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas administradores"})
			c.Abort()
			return
		}

		// Delega para RequireAdminRole que valida role e status no banco.
		RequireAdminRole("gerente")(c)
	}
}

// RequireAdminRole verifica que o admin autenticado possui ao menos o role mínimo exigido
// e que está ativo. Consulta projection_admins com prepared statement.
func RequireAdminRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("🔐 [RequireAdminRole] Role mínima: %s — Path: %s", minRole, c.Request.URL.Path)

		userID, exists := c.Get("user_id")
		if !exists {
			log.Printf("❌ [RequireAdminRole] user_id não encontrado")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "não autenticado"})
			c.Abort()
			return
		}

		clientRaw, exists := c.Get("dbClient")
		if !exists {
			log.Printf("❌ [RequireAdminRole] dbClient não encontrado no contexto")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar permissões"})
			c.Abort()
			return
		}

		client := clientRaw.(*db.Client)

		uid, ok := userID.(uuid.UUID)
		if !ok {
			log.Printf("❌ [RequireAdminRole] UserID não é UUID válido")
			c.JSON(http.StatusForbidden, gin.H{"error": "administrador não encontrado"})
			c.Abort()
			return
		}

		// Prepared statement — sem interpolação de string.
		var role, status string
		err := client.DB().QueryRow(
			`SELECT role, status FROM projection_admins WHERE id = $1`,
			uid,
		).Scan(&role, &status)
		if err != nil {
			log.Printf("❌ [RequireAdminRole] Erro ao buscar admin: %v", err)
			c.JSON(http.StatusForbidden, gin.H{"error": "administrador não encontrado"})
			c.Abort()
			return
		}

		log.Printf("✅ [RequireAdminRole] Admin encontrado — Role: %s, Status: %s", role, status)

		if status != "ativo" {
			log.Printf("❌ [RequireAdminRole] Admin inativo")
			c.JSON(http.StatusForbidden, gin.H{"error": "administrador inativo"})
			c.Abort()
			return
		}

		hierarchy := map[string]int{
			"fpp":     3,
			"adm":     2,
			"gerente": 1,
		}

		currentLevel := hierarchy[role]
		requiredLevel := hierarchy[minRole]

		log.Printf("🔍 [RequireAdminRole] Hierarquia — current: %d (%s), required: %d (%s)",
			currentLevel, role, requiredLevel, minRole)

		if currentLevel < requiredLevel {
			log.Printf("❌ [RequireAdminRole] Permissão insuficiente")
			c.JSON(http.StatusForbidden, gin.H{
				"error":         "permissão negada",
				"required_role": minRole,
				"your_role":     role,
			})
			c.Abort()
			return
		}

		c.Set("admin_role", role)
		log.Printf("✅ [RequireAdminRole] Permissão OK")
		c.Next()
	}
}

func RequireGerente() gin.HandlerFunc {
	return RequireAdminRole("gerente")
}

func RequireAdm() gin.HandlerFunc {
	return RequireAdminRole("adm")
}

func RequireFPP() gin.HandlerFunc {
	return RequireAdminRole("fpp")
}