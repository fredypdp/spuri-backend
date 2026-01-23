// ============================================================================
// ARQUIVO: internal/middleware/admin_auth_middleware.go
// Middleware de autenticação e autorização para administradores
// ============================================================================

package middleware

import (
	"net/http"
	"spuri/internal/db"
	"github.com/gin-gonic/gin"
)

// RequireAdmin verifica se o usuário é um admin (qualquer role)
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userType, exists := c.Get("user_type")
		if !exists || userType != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "acesso negado: apenas administradores",
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAdminRole verifica se o admin tem a role mínima necessária
func RequireAdminRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "não autenticado",
			})
			c.Abort()
			return
		}

		// Buscar role do admin na projeção
		clientRaw, exists := c.Get("dbClient")
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "erro ao verificar permissões",
			})
			c.Abort()
			return
		}

		client := clientRaw.(*db.Client)

		type AdminInfo struct {
			Role   string `db:"role"`
			Status string `db:"status"`
		}

		var info AdminInfo
		query := `
			SELECT role, status 
			FROM projection_admins 
			WHERE id = $1
		`

		err := client.DB().QueryRow(query, userID).Scan(&info.Role, &info.Status)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "administrador não encontrado",
			})
			c.Abort()
			return
		}

		// Verificar se está ativo
		if info.Status != "ativo" {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "administrador inativo",
			})
			c.Abort()
			return
		}

		// Hierarquia de roles
		hierarchy := map[string]int{
			"fpp":     3,
			"adm":     2,
			"gerente": 1,
		}

		currentLevel := hierarchy[info.Role]
		requiredLevel := hierarchy[minRole]

		if currentLevel < requiredLevel {
			c.JSON(http.StatusForbidden, gin.H{
				"error":        "permissão negada",
				"required_role": minRole,
				"your_role":    info.Role,
			})
			c.Abort()
			return
		}

		// Adicionar role ao contexto
		c.Set("admin_role", info.Role)
		c.Next()
	}
}

// RequireGerente - apenas gerente ou superior
func RequireGerente() gin.HandlerFunc {
	return RequireAdminRole("gerente")
}

// RequireAdm - apenas adm ou superior (fpp)
func RequireAdm() gin.HandlerFunc {
	return RequireAdminRole("adm")
}

// RequireFPP - apenas fpp
func RequireFPP() gin.HandlerFunc {
	return RequireAdminRole("fpp")
}