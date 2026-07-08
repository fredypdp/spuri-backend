package middleware

import (
	"database/sql"
	"log"
	"net/http"
	"spuri/internal/db"

	"github.com/gin-gonic/gin"
)

// ValidarStatusAcademia verifica se a academia autenticada está ativa.
// Deve ser aplicado após AuthMiddleware() e RequireAcademia().
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

		userID, ok := GetUserID(c)
		if !ok {
			log.Printf("❌ [ValidarStatusAcademia] Não foi possível obter userID do contexto")
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "erro ao verificar status da academia",
			})
			c.Abort()
			return
		}

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

		// CORREÇÃO: prepared statement ($1) em vez de fmt.Sprintf + SafeString.
		// userID é um uuid.UUID proveniente do JWT, mas o padrão de prepared
		// statements deve ser uniforme em todo o projeto.
		var status string
		err := client.DB().QueryRow(
			`SELECT status FROM projection_academias WHERE id = $1`,
			userID,
		).Scan(&status)

		if err == sql.ErrNoRows {
			log.Printf("❌ [ValidarStatusAcademia] Academia não encontrada - ID: %s", userID)
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "academia não encontrada",
				"message": "Entre em contato com o suporte",
			})
			c.Abort()
			return
		}
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
