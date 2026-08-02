package middleware

import (
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("👤 [RequireAdmin] Verificando admin — Path: %s", c.Request.URL.Path)

		// Verificação rápida de tipo antes de bater no banco.
		userType, exists := c.Get("user_type")
		if !exists || userType != "admin" {
			log.Printf("❌ [RequireAdmin] user_type ausente ou incorreto: %v", userType)
			utils.RespondWithError(c, http.StatusForbidden, "acesso negado: apenas administradores", nil)
			c.Abort()
			return
		}

		// Delega para RequireAdminRole que valida role, status e email_verificado no banco.
		RequireAdminRole("gerente")(c)
	}
}

// RequireAdminRole verifica que o admin autenticado:
//   - existe na projeção
//   - está ativo (status = "ativo")
//   - possui e-mail verificado (email_verificado = true)
//   - possui ao menos o role mínimo exigido
//
// Admins sem e-mail verificado só podem fazer login — todas as rotas
// do grupo /dominis exigem email_verificado = true.
func RequireAdminRole(minRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Printf("🔐 [RequireAdminRole] Role mínima: %s — Path: %s", minRole, c.Request.URL.Path)

		userID, exists := c.Get("user_id")
		if !exists {
			log.Printf("❌ [RequireAdminRole] user_id não encontrado")
			utils.RespondWithError(c, http.StatusUnauthorized, "não autenticado", nil)
			c.Abort()
			return
		}

		clientRaw, exists := c.Get("dbClient")
		if !exists {
			log.Printf("❌ [RequireAdminRole] dbClient não encontrado no contexto")
			utils.RespondWithError(c, http.StatusInternalServerError, "erro ao verificar permissões", nil)
			c.Abort()
			return
		}

		client := clientRaw.(*db.Client)

		uid, ok := userID.(uuid.UUID)
		if !ok {
			log.Printf("❌ [RequireAdminRole] UserID não é UUID válido")
			utils.RespondWithError(c, http.StatusForbidden, "administrador não encontrado", nil)
			c.Abort()
			return
		}

		// Prepared statement — sem interpolação de string.
		// email_verificado incluído para bloquear admins que ainda não confirmaram o e-mail.
		var role, status string
		var emailVerificado bool
		err := client.DB().QueryRow(
			`SELECT role, status, email_verificado FROM projection_admins WHERE id = $1`,
			uid,
		).Scan(&role, &status, &emailVerificado)
		if err != nil {
			log.Printf("❌ [RequireAdminRole] Erro ao buscar admin: %v", err)
			utils.RespondWithError(c, http.StatusForbidden, "administrador não encontrado", nil)
			c.Abort()
			return
		}

		log.Printf("✅ [RequireAdminRole] Admin encontrado — Role: %s, Status: %s, EmailVerificado: %v",
			role, status, emailVerificado)

		if status != "ativo" {
			log.Printf("❌ [RequireAdminRole] Admin inativo")
			utils.RespondWithError(c, http.StatusForbidden, "administrador inativo", nil)
			c.Abort()
			return
		}

		// Bloquear acesso a todas as rotas /dominis se o e-mail não foi verificado.
		// O admin ainda pode fazer login e solicitar o reenvio do e-mail de verificação,
		// mas não pode executar nenhuma ação administrativa.
		if !emailVerificado {
			log.Printf("❌ [RequireAdminRole] Admin sem e-mail verificado: %s", uid)
			utils.RespondWithError(c, http.StatusForbidden,
				"e-mail não verificado. Verifique sua caixa de entrada e confirme seu e-mail para acessar o painel administrativo.", nil)
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
			utils.RespondWithErrorData(c, http.StatusForbidden, "permissão negada", nil, gin.H{
				"required_role": minRole,
				"your_role":     role,
			})
			c.Abort()
			return
		}

		c.Set("admin_role", role)
		log.Printf("✅ [RequireAdminRole] Permissão OK — email verificado, role suficiente")
		c.Next()
	}
}

// PopulateAdminRole preenche "admin_role" no contexto quando o usuário autenticado
// é um admin ativo e com e-mail verificado, SEM bloquear a requisição para outros
// tipos de usuário (academia, estudante) nem para admins que falhem alguma dessas
// condições.
//
// Diferente de RequireAdminRole/RequireFPP/RequireAdm/RequireGerente, esta função
// NUNCA aborta a requisição: qualquer situação em que o role não possa ser
// determinado (usuário não é admin, user_id ausente, dbClient ausente/nil, admin
// não encontrado, inativo, e-mail não verificado, erro de banco) apenas deixa
// "admin_role" sem preencher e segue para o próximo middleware/handler.
//
// Uso: grupos de rota que aceitam mais de um tipo de usuário (ex.: academia E
// admin, via RequireAcademiaOuAdmin) mas cujo handler precisa do role granular
// do admin (fpp/adm/gerente) para autorização fina — como o módulo financeiro.
// Sem esta função, RequireAcademiaOuAdmin() sozinho deixava passar o admin mas
// nunca preenchia "admin_role", fazendo o handler enxergar sempre "admin"
// genérico em vez do role real — o que bloqueava inclusive admins FPP de
// operações que a regra de negócio já permitia.
//
// FIX FIN-RBAC-01: adicionado após auditoria identificar que GET/POST de leitura
// e teste de credenciais AppyPay, e o PUT de atualização, ficaram inacessíveis
// para qualquer admin (incluindo FPP) depois que a checagem de role passou a ser
// estrita no serviço (internal/finance.Service), pois nenhuma rota do grupo
// /financeiro além das que já usavam RequireFPP() diretamente preenchia
// "admin_role".
func PopulateAdminRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		userType, exists := c.Get("user_type")
		if !exists || userType != "admin" {
			c.Next()
			return
		}

		userID, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}
		uid, ok := userID.(uuid.UUID)
		if !ok {
			c.Next()
			return
		}

		clientRaw, exists := c.Get("dbClient")
		if !exists {
			c.Next()
			return
		}
		client, ok := clientRaw.(*db.Client)
		if !ok || client == nil {
			c.Next()
			return
		}

		var role, status string
		var emailVerificado bool
		err := client.DB().QueryRow(
			`SELECT role, status, email_verificado FROM projection_admins WHERE id = $1`,
			uid,
		).Scan(&role, &status, &emailVerificado)
		if err != nil {
			log.Printf("⚠️ [PopulateAdminRole] não foi possível resolver role do admin %s: %v", uid, err)
			c.Next()
			return
		}
		if status != "ativo" || !emailVerificado {
			log.Printf("⚠️ [PopulateAdminRole] admin %s inativo ou sem e-mail verificado — admin_role não preenchido", uid)
			c.Next()
			return
		}

		c.Set("admin_role", role)
		log.Printf("✅ [PopulateAdminRole] admin_role=%s definido para %s", role, uid)
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
