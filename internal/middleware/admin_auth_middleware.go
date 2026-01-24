// ============================================================================
// ARQUIVO: internal/middleware/admin_auth_middleware.go
// Middleware de autenticação e autorização para administradores
// ============================================================================

package middleware

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("👤 [RequireAdmin] Verificando se é admin - Path: %s", c.Request.URL.Path)
		
		userType, exists := c.Get("user_type")
		if !exists {
			log.Printf("❌ [RequireAdmin] user_type não existe no contexto")
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas administradores"})
			c.Abort()
			return
		}
		
		log.Printf("🔍 [RequireAdmin] UserType encontrado: %v", userType)
		
		if userType != "admin" {
			log.Printf("❌ [RequireAdmin] UserType incorreto: %v (esperado: admin)", userType)
			c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas administradores"})
			c.Abort()
			return
		}
		
		log.Printf("✅ [RequireAdmin] OK - É admin")
		c.Next()
	}
}

func RequireAdminRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("🔑 [RequireAdminRole] Verificando role mínima: %s", minRole)
		
		userID, exists := c.Get("user_id")
		if !exists {
			log.Printf("❌ [RequireAdminRole] user_id não encontrado")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "não autenticado"})
			c.Abort()
			return
		}

		log.Printf("🔍 [RequireAdminRole] UserID: %v", userID)

		// Buscar role do admin na projeção
		clientRaw, exists := c.Get("dbClient")
		if !exists {
			log.Printf("❌ [RequireAdminRole] dbClient não encontrado no contexto")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar permissões"})
			c.Abort()
			return
		}

		client := clientRaw.(*db.Client)

		type AdminInfo struct {
			Role   string `db:"role"`
			Status string `db:"status"`
		}

		var info AdminInfo
		
		// Converter UUID para string
		uid, ok := userID.(uuid.UUID)
		if !ok {
			log.Printf("❌ [RequireAdminRole] UserID não é UUID válido")
			c.JSON(http.StatusForbidden, gin.H{"error": "administrador não encontrado"})
			c.Abort()
			return
		}
		
		safeUserID := db.SafeString(uid.String())
		query := fmt.Sprintf(`SELECT role, status FROM projection_admins WHERE id = '%s'`, safeUserID)

		log.Printf("🔍 [RequireAdminRole] Buscando admin na projeção...")
		err := client.DB().QueryRow(query).Scan(&info.Role, &info.Status)
		if err != nil {
			log.Printf("❌ [RequireAdminRole] Erro ao buscar admin: %v", err)
			c.JSON(http.StatusForbidden, gin.H{"error": "administrador não encontrado"})
			c.Abort()
			return
		}

		log.Printf("✅ [RequireAdminRole] Admin encontrado - Role: %s, Status: %s", info.Role, info.Status)

		if info.Status != "ativo" {
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

		currentLevel := hierarchy[info.Role]
		requiredLevel := hierarchy[minRole]

		log.Printf("🔍 [RequireAdminRole] Hierarquia - Current: %d (%s), Required: %d (%s)", 
			currentLevel, info.Role, requiredLevel, minRole)

		if currentLevel < requiredLevel {
			log.Printf("❌ [RequireAdminRole] Permissão insuficiente")
			c.JSON(http.StatusForbidden, gin.H{
				"error":         "permissão negada",
				"required_role": minRole,
				"your_role":     info.Role,
			})
			c.Abort()
			return
		}

		c.Set("admin_role", info.Role)
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