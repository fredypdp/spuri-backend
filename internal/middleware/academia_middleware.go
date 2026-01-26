// ============================================================================
// ARQUIVO: internal/middleware/academia_middleware.go
// Middleware para validar status da academia
// ============================================================================

package middleware

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"

	"github.com/gin-gonic/gin"
)

// ValidarStatusAcademia verifica se a academia está ativa
func ValidarStatusAcademia() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("🏫 [ValidarStatusAcademia] Iniciando validação - Path: %s", c.Request.URL.Path)
		
		userType, _ := c.Get("user_type")

		// Aplicar apenas para academias
		if userType != "academia" {
			log.Printf("⏭️ [ValidarStatusAcademia] Não é academia, pulando - UserType: %v", userType)
			c.Next()
			return
		}

		userID, _ := GetUserID(c)
		log.Printf("🔍 [ValidarStatusAcademia] Buscando status da academia - ID: %s", userID)

		// Obter cliente do banco
		clientRaw, exists := c.Get("dbClient")
		if !exists {
			log.Printf("❌ [ValidarStatusAcademia] dbClient não encontrado no contexto")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "erro ao verificar status da academia",
			})
			c.Abort()
			return
		}

		client := clientRaw.(*db.Client)

		safeUserID := db.SafeString(userID.String())
		query := fmt.Sprintf(`SELECT status FROM projection_academias WHERE id = '%s'`, safeUserID)
		
		log.Printf("📝 [ValidarStatusAcademia] Query: %s", query)

		var status string
		err := client.DB().QueryRow(query).Scan(&status)
		if err != nil {
			log.Printf("❌ [ValidarStatusAcademia] Erro ao buscar status: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "erro ao verificar status da academia",
			})
			c.Abort()
			return
		}

		log.Printf("📊 [ValidarStatusAcademia] Status encontrado: %s", status)

		if status != "ativo" {
			log.Printf("⛔ [ValidarStatusAcademia] Academia inativa - bloqueando acesso")
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "academia inativa - não pode realizar operações",
				"status":  status,
				"message": "Entre em contato com o suporte para reativar sua conta",
			})
			c.Abort()
			return
		}

		log.Printf("✅ [ValidarStatusAcademia] Academia ativa - permitindo acesso")
		c.Next()
	}
}