// ============================================================================
// ARQUIVO: internal/handlers/auth_email_handlers.go
//
// CORREÇÕES APLICADAS (auditoria Março 2026):
//   #9  — VerificarEmail: retorna mensagem diferenciada quando já verificado
//   #10 — GerarTokenRecuperacao: corrigido nome da coluna para `email`
//          (era `e_mail` — typo que causava "usuário não encontrado" para admins)
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/services"
	"spuri/internal/utils"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================================
// Verificação de email
// ============================================================================

// VerificarEmail verifica email usando token.
// FIX #9: resposta diferenciada quando o email já estava verificado.
func VerificarEmail(c *gin.Context) {
	token := c.Param("token")

	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "verificacao_email")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Admin: event sourcing ──────────────────────────────────────────────
	if tokenInfo.UserType == "admin" {
		repository := getRepository(c)
		adminAgg, err := repository.Load(tokenInfo.UserID, "Admin")
		if err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
		admin := adminAgg.(*aggregates.Admin)

		alreadyVerified := false
		if err := admin.VerificarEmail(); err != nil {
			if err.Error() == "email já verificado" {
				// FIX #9: idempotente — não retorna erro, mas responde com msg distinta
				alreadyVerified = true
				log.Printf("[INFO] Email já estava verificado para admin: %s", tokenInfo.Email)
			} else {
				utils.RespondWithInternalError(c, err)
				return
			}
		}

		// Só grava no ledger se houve mudança de estado
		if !alreadyVerified {
			audit := db.AuditContext{
				UserID:   tokenInfo.UserID.String(),
				UserType: "admin",
				IP:       c.ClientIP(),
			}
			if err := repository.SaveWithAudit(admin, audit); err != nil {
				utils.RespondWithInternalError(c, err)
				return
			}
			log.Printf("Email verificado (event sourcing) para admin: %s", tokenInfo.Email)
		}

		msg := "Email verificado com sucesso!"
		if alreadyVerified {
			msg = "Email já estava verificado."
		}
		c.JSON(http.StatusOK, gin.H{
			"message": msg,
			"email":   tokenInfo.Email,
		})
		return
	}

	// ── Estudante e Academia: UPDATE direto na projeção ────────────────────
	var table string
	switch tokenInfo.UserType {
	case "estudante":
		table = "projection_estudantes"
	case "academia":
		table = "projection_academias"
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
		return
	}

	client := getDbClient(c)
	// Tabela controlada por switch interno — sem interpolação de input externo
	if _, err = client.DB().Exec(
		fmt.Sprintf("UPDATE %s SET email_verificado = TRUE WHERE id = $1", table),
		tokenInfo.UserID,
	); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Email verificado: %s", tokenInfo.Email)
	c.JSON(http.StatusOK, gin.H{
		"message": "Email verificado com sucesso!",
		"email":   tokenInfo.Email,
	})
}

// ============================================================================
// Reset de senha
// ============================================================================

// ResetarSenha redefine senha usando token de recuperação.
func ResetarSenha(c *gin.Context) {
	token := c.Param("token")

	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "recuperacao_senha")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	client := getDbClient(c)

	// ── Admin: event sourcing ──────────────────────────────────────────────
	if tokenInfo.UserType == "admin" {
		var role string
		var emailVerificado bool
		err = client.DB().QueryRow(
			`SELECT role, COALESCE(email_verificado, FALSE) FROM projection_admins WHERE id = $1`,
			tokenInfo.UserID,
		).Scan(&role, &emailVerificado)
		if err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}

		if !emailVerificado {
			utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de resetar a senha")
			return
		}

		defaultPassword := services.GetDefaultPassword("admin", role)
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		repository := getRepository(c)
		adminAgg, err := repository.Load(tokenInfo.UserID, "Admin")
		if err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
		admin := adminAgg.(*aggregates.Admin)

		if err := admin.AlterarSenha(string(hashedPassword), uuid.Nil, "reset_senha"); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		audit := db.AuditContext{
			UserID:   "sistema",
			UserType: "sistema",
			IP:       c.ClientIP(),
		}
		if err := repository.SaveWithAudit(admin, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		log.Printf("Senha resetada (event sourcing) para admin: %s", tokenInfo.Email)
		c.JSON(http.StatusOK, gin.H{
			"message":         "Senha resetada com sucesso!",
			"senha_padrao":    defaultPassword,
			"email":           tokenInfo.Email,
			"proximos_passos": "Faça login com a senha padrão e altere para uma senha segura.",
		})
		return
	}

	// ── Estudante e Academia: UPDATE direto na projeção ────────────────────
	var table string
	switch tokenInfo.UserType {
	case "estudante":
		table = "projection_estudantes"
	case "academia":
		table = "projection_academias"
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
		return
	}

	defaultPassword := services.GetDefaultPassword(tokenInfo.UserType, "")
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Tabela controlada por switch interno — sem interpolação de input externo
	if _, err = client.DB().Exec(
		fmt.Sprintf("UPDATE %s SET senha_hash = $1 WHERE id = $2", table),
		string(hashedPassword), tokenInfo.UserID,
	); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Senha resetada para %s: %s", tokenInfo.UserType, tokenInfo.Email)
	c.JSON(http.StatusOK, gin.H{
		"message":         "Senha resetada com sucesso!",
		"senha_padrao":    defaultPassword,
		"proximos_passos": "Faça login com a senha padrão e altere para uma senha segura.",
	})
}

// ============================================================================
// Envio de email
// ============================================================================

// SolicitarVerificacaoEmail envia email de verificação para o usuário logado.
func SolicitarVerificacaoEmail(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := c.Get("user_type")

	var email, nome string
	client := getDbClient(c)

	switch userType {
	case "admin":
		err := client.DB().QueryRow(
			`SELECT email, nome FROM projection_admins WHERE id = $1`, userID,
		).Scan(&email, &nome)
		if err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
	case "estudante":
		err := client.DB().QueryRow(
			`SELECT email, nome FROM projection_estudantes WHERE id = $1`, userID,
		).Scan(&email, &nome)
		if err != nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
	case "academia":
		err := client.DB().QueryRow(
			`SELECT email, nome FROM projection_academias WHERE id = $1`, userID,
		).Scan(&email, &nome)
		if err != nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
		return
	}

	if email == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nenhum email cadastrado para este usuário"))
		return
	}

	emailSvc := getEmailService(c)
	if err := emailSvc.SendVerificationEmail(userID, userType.(string), email, nome); err != nil {
		log.Printf("[WARN] Erro ao enviar email de verificação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Email de verificação enviado! Verifique sua caixa de entrada.",
		"email":   maskEmail(email),
	})
}

// SolicitarRecuperacaoSenha envia email de recuperação de senha.
func SolicitarRecuperacaoSenha(c *gin.Context) {
	var req struct {
		Identificador string `json:"identificador" binding:"required"`
		Tipo          string `json:"tipo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("identificador e tipo são obrigatórios"))
		return
	}

	client := getDbClient(c)
	emailSvc := getEmailService(c)
	if emailSvc == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("serviço de email não disponível"))
		return
	}

	var userID uuid.UUID
	var email, nome string
	var emailVerificado bool
	var idStr string
	var err error

	switch req.Tipo {
	case "estudante":
		err = client.DB().QueryRow(
			`SELECT id, email, nome, COALESCE(email_verificado, FALSE)
			 FROM projection_estudantes
			 WHERE codigo_estudante = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome, &emailVerificado)
	case "academia":
		err = client.DB().QueryRow(
			`SELECT id, email, nome, COALESCE(email_verificado, FALSE)
			 FROM projection_academias
			 WHERE codigo_academia = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome, &emailVerificado)
	case "admin":
		// FIX #10: coluna correta é `email` (não `e_mail`)
		err = client.DB().QueryRow(
			`SELECT id, email, nome, COALESCE(email_verificado, FALSE)
			 FROM projection_admins WHERE email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome, &emailVerificado)
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo deve ser 'estudante', 'academia' ou 'admin'"))
		return
	}

	if err != nil {
		utils.RespondWithNotFoundError(c, "usuário")
		return
	}

	userID, _ = uuid.Parse(idStr)

	if email == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("usuário não possui email cadastrado"))
		return
	}

	if !emailVerificado {
		utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de solicitar recuperação de senha")
		return
	}

	if err := emailSvc.SendPasswordResetEmail(userID, req.Tipo, email, nome); err != nil {
		log.Printf("Erro ao enviar email de recuperação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Email de recuperação enviado para: %s", email)
	c.JSON(http.StatusOK, gin.H{
		"message": "Email de recuperação enviado! Verifique sua caixa de entrada.",
		"email":   maskEmail(email),
	})
}

// ============================================================================
// Geração de tokens (uso interno/admin — sem envio de email)
// ============================================================================

// GerarTokenVerificacao gera token de verificação sem enviar email.
func GerarTokenVerificacao(c *gin.Context) {
	var req struct {
		Identificador string `json:"identificador" binding:"required"`
		Tipo          string `json:"tipo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("identificador e tipo são obrigatórios"))
		return
	}

	client := getDbClient(c)
	emailSvc := getEmailService(c)

	var userID uuid.UUID
	var email, nome string
	var idStr string
	var err error

	switch req.Tipo {
	case "estudante":
		err = client.DB().QueryRow(
			`SELECT id, email, nome FROM projection_estudantes
			 WHERE codigo_estudante = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome)
	case "academia":
		err = client.DB().QueryRow(
			`SELECT id, email, nome FROM projection_academias
			 WHERE codigo_academia = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome)
	case "admin":
		err = client.DB().QueryRow(
			`SELECT id, email, nome FROM projection_admins WHERE email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome)
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo deve ser 'estudante', 'academia' ou 'admin'"))
		return
	}

	if err != nil {
		utils.RespondWithNotFoundError(c, "usuário")
		return
	}

	userID, _ = uuid.Parse(idStr)

	if email == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("usuário não possui email cadastrado"))
		return
	}

	token, err := emailSvc.SaveToken(userID, req.Tipo, "verificacao_email", email, 24*time.Hour)
	if err != nil {
		log.Printf("Erro ao gerar token de verificação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Token de verificação gerado para: %s", email)
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"token":     token,
		"email":     email,
		"nome":      nome,
		"tipo":      req.Tipo,
		"expira_em": "24 horas",
	})
}

// GerarTokenRecuperacao gera token de recuperação sem enviar email.
// FIX #10: corrigido nome da coluna para `email` (era `e_mail`).
func GerarTokenRecuperacao(c *gin.Context) {
	var req struct {
		Identificador string `json:"identificador" binding:"required"`
		Tipo          string `json:"tipo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("identificador e tipo são obrigatórios"))
		return
	}

	client := getDbClient(c)
	emailSvc := getEmailService(c)

	var userID uuid.UUID
	var email, nome string
	var emailVerificado bool
	var idStr string
	var err error

	switch req.Tipo {
	case "estudante":
		err = client.DB().QueryRow(
			`SELECT id, email, nome, COALESCE(email_verificado, FALSE)
			 FROM projection_estudantes
			 WHERE codigo_estudante = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome, &emailVerificado)
	case "academia":
		err = client.DB().QueryRow(
			`SELECT id, email, nome, COALESCE(email_verificado, FALSE)
			 FROM projection_academias
			 WHERE codigo_academia = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome, &emailVerificado)
	case "admin":
		// FIX #10: coluna correta é `email` (não `e_mail`)
		err = client.DB().QueryRow(
			`SELECT id, email, nome, COALESCE(email_verificado, FALSE)
			 FROM projection_admins WHERE email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome, &emailVerificado)
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo deve ser 'estudante', 'academia' ou 'admin'"))
		return
	}

	if err != nil {
		utils.RespondWithNotFoundError(c, "usuário")
		return
	}

	userID, _ = uuid.Parse(idStr)

	if email == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("usuário não possui email cadastrado"))
		return
	}

	if !emailVerificado {
		utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de solicitar recuperação de senha")
		return
	}

	token, err := emailSvc.SaveToken(userID, req.Tipo, "recuperacao_senha", email, 1*time.Hour)
	if err != nil {
		log.Printf("Erro ao gerar token de recuperação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Token de recuperação gerado para: %s", email)
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"token":     token,
		"email":     email,
		"nome":      nome,
		"tipo":      req.Tipo,
		"expira_em": "1 hora",
	})
}