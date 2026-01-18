// ============================================================================
// ARQUIVO: internal/middleware/academia_middleware.go
// Middleware para validar status da academia
// ============================================================================

package middleware

import (
	"net/http"
	"spuri/internal/db"

	"github.com/gin-gonic/gin"
)

// ValidarStatusAcademia verifica se a academia está ativa
func ValidarStatusAcademia() gin.HandlerFunc {
	return func(c *gin.Context) {
		userType, _ := c.Get("user_type")

		// Aplicar apenas para academias
		if userType != "academia" {
			c.Next()
			return
		}

		userID, _ := GetUserID(c)

		// Buscar status da academia na projeção
		query := `
			SELECT status FROM projection_academias WHERE id = $1
		`

		// Obter cliente do banco
		clientRaw, exists := c.Get("dbClient")
		if !exists {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "erro ao verificar status da academia",
			})
			c.Abort()
			return
		}

		client := clientRaw.(*db.Client)

		var status string
		err := client.DB().Get(&status, query, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "erro ao verificar status da academia",
			})
			c.Abort()
			return
		}

		if status != "ativo" {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "academia inativa - não pode realizar operações",
				"status":  status,
				"message": "Entre em contato com o suporte para reativar sua conta",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
