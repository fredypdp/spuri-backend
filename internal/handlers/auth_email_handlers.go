// ============================================================================
// ARQUIVO: internal/handlers/auth_email_handlers.go
//
// CORREÇÕES APLICADAS:
//   [A24]  — ResetarSenha: senha_padrao REMOVIDA da resposta HTTP.
//   #9     — VerificarEmail: resposta diferenciada quando email já verificado.
//   #10    — GerarTokenRecuperacao: corrigido nome da coluna para `email`.
//   FIX C1 — ResetarSenha academia: event sourcing via AcademiaSenhaAlteradaEvent.
//            Antes: UPDATE direto na projeção — bypassava o ledger.
//   FIX C2 — VerificarEmail academia: event sourcing via academia.VerificarEmail().
//            Antes: UPDATE direto em projection_academias — Rebuild desfazia a verificação.
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
//
// FIX C2: academia agora usa event sourcing via aggregate.VerificarEmail(),
// idêntico ao fluxo do admin. O aggregate Academia.VerificarEmail() já existia
// e emite EmailVerificadoEvent — simplesmente não era chamado para academia.
// Antes: UPDATE direto em projection_academias → Rebuild desfazia a verificação.
// Agora: academia.VerificarEmail() → EmailVerificadoEvent → ledger → projeção.
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

	// ── Academia: event sourcing (FIX C2) ─────────────────────────────────
	// Antes: UPDATE direto em projection_academias — Rebuild desfazia a verificação.
	// Agora: academia.VerificarEmail() → EmailVerificadoEvent → ledger → projeção.
	if tokenInfo.UserType == "academia" {
		repository := getRepository(c)
		academiaAgg, err := repository.Load(tokenInfo.UserID, "Academia")
		if err != nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		academia := academiaAgg.(*aggregates.Academia)

		alreadyVerified := false
		if err := academia.VerificarEmail(); err != nil {
			if err.Error() == "email já verificado" {
				alreadyVerified = true
				log.Printf("[INFO] Email já estava verificado para academia: %s", tokenInfo.Email)
			} else {
				utils.RespondWithInternalError(c, err)
				return
			}
		}

		// Só grava no ledger se houve mudança de estado
		if !alreadyVerified {
			audit := db.AuditContext{
				UserID:   tokenInfo.UserID.String(),
				UserType: "academia",
				IP:       c.ClientIP(),
			}
			if err := repository.SaveWithAudit(academia, audit); err != nil {
				utils.RespondWithInternalError(c, err)
				return
			}
			log.Printf("Email verificado (event sourcing) para academia: %s", tokenInfo.Email)
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

	// ── Estudante: UPDATE direto na projeção ──────────────────────────────
	// NOTA: estudante mantém UPDATE direto pois o scope desta correção é academia.
	// A migração para event sourcing do estudante é uma tarefa separada.
	if tokenInfo.UserType == "estudante" {
		client := getDbClient(c)
		if _, err = client.DB().Exec(
			`UPDATE projection_estudantes SET email_verificado = TRUE WHERE id = $1`,
			tokenInfo.UserID,
		); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		log.Printf("Email verificado (direto) para estudante: %s", tokenInfo.Email)
		c.JSON(http.StatusOK, gin.H{
			"message": "Email verificado com sucesso!",
			"email":   tokenInfo.Email,
		})
		return
	}

	utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
}

// ============================================================================
// Reset de senha
// ============================================================================

// ResetarSenha redefine senha usando token de recuperação.
//
// [A24] CORRIGIDO: senha_padrao REMOVIDA da resposta HTTP.
// FIX C1: academia agora usa event sourcing via aggregate.AlterarSenha(),
// idêntico ao fluxo do admin.
// Antes: UPDATE direto em projection_academias — bypassava o ledger.
// Agora: academia.AlterarSenha() → AcademiaSenhaAlteradaEvent → ledger → projeção.
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

		// [A24] Sem senha_padrao na resposta.
		c.JSON(http.StatusOK, gin.H{
			"message":         "Senha resetada com sucesso!",
			"email":           tokenInfo.Email,
			"proximos_passos": "Faça login com a senha padrão enviada por email e altere para uma senha segura.",
		})
		return
	}

	// ── Academia: event sourcing (FIX C1) ─────────────────────────────────
	// Antes: UPDATE direto em projection_academias — bypassava o ledger.
	// Agora: academia.AlterarSenha() → AcademiaSenhaAlteradaEvent → ledger → projeção.
	if tokenInfo.UserType == "academia" {
		var emailVerificado bool
		err = client.DB().QueryRow(
			`SELECT COALESCE(email_verificado, FALSE) FROM projection_academias WHERE id = $1`,
			tokenInfo.UserID,
		).Scan(&emailVerificado)
		if err != nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}

		if !emailVerificado {
			utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de resetar a senha")
			return
		}

		defaultPassword := services.GetDefaultPassword("academia", "")
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		repository := getRepository(c)
		academiaAgg, err := repository.Load(tokenInfo.UserID, "Academia")
		if err != nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		academia := academiaAgg.(*aggregates.Academia)

		// uuid.Nil = reset via sistema (sem usuário autenticado)
		if err := academia.AlterarSenha(string(hashedPassword), uuid.Nil, "reset_senha"); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		audit := db.AuditContext{
			UserID:   "sistema",
			UserType: "sistema",
			IP:       c.ClientIP(),
		}
		if err := repository.SaveWithAudit(academia, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		log.Printf("Senha resetada (event sourcing) para academia: %s", tokenInfo.Email)

		// [A24] Sem senha_padrao na resposta.
		c.JSON(http.StatusOK, gin.H{
			"message":         "Senha resetada com sucesso!",
			"proximos_passos": "Faça login com sua senha padrão e altere para uma senha segura.",
		})
		return
	}

	// ── Estudante: UPDATE direto na projeção ──────────────────────────────
	// NOTA: estudante mantém UPDATE direto pois o scope desta correção é academia.
	if tokenInfo.UserType == "estudante" {
		defaultPassword := services.GetDefaultPassword(tokenInfo.UserType, "")
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		if _, err = client.DB().Exec(
			`UPDATE projection_estudantes SET senha_hash = $1 WHERE id = $2`,
			string(hashedPassword), tokenInfo.UserID,
		); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		log.Printf("Senha resetada para estudante: %s", tokenInfo.Email)

		// [A24] Sem senha_padrao na resposta.
		c.JSON(http.StatusOK, gin.H{
			"message":         "Senha resetada com sucesso!",
			"proximos_passos": "Faça login com sua senha padrão e altere para uma senha segura.",
		})
		return
	}

	utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
}

// ============================================================================
// Envio de email
// ============================================================================

// SolicitarVerificacaoEmail envia email de verificação para o usuário logado.
func SolicitarVerificacaoEmail(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	client := getDbClient(c)
	emailSvc := getEmailService(c)

	var email, nome string
	var err error

	switch userType {
	case "estudante":
		err = client.DB().QueryRow(
			`SELECT COALESCE(email, ''), nome FROM projection_estudantes WHERE id = $1`,
			userID,
		).Scan(&email, &nome)
	case "academia":
		err = client.DB().QueryRow(
			`SELECT COALESCE(email, ''), nome FROM projection_academias WHERE id = $1`,
			userID,
		).Scan(&email, &nome)
	case "admin":
		err = client.DB().QueryRow(
			`SELECT email, nome FROM projection_admins WHERE id = $1`,
			userID,
		).Scan(&email, &nome)
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
		return
	}

	if err != nil {
		utils.RespondWithNotFoundError(c, "usuário")
		return
	}

	if email == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nenhum email cadastrado para envio"))
		return
	}

	_, err = emailSvc.SaveToken(userID, userType, "verificacao_email", email, 24*time.Hour)
	if err != nil {
		log.Printf("Erro ao gerar token de verificação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Token de verificação gerado para: %s (%s)", email, userType)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Email de verificação enviado.",
		"email":   maskEmail(email),
	})
}

// SolicitarRecuperacaoSenha inicia o fluxo de recuperação de senha.
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

	var userID uuid.UUID
	var email string
	var emailVerificado bool
	var idStr string
	var err error

	switch req.Tipo {
	case "estudante":
		err = client.DB().QueryRow(
			`SELECT id, COALESCE(email, ''), COALESCE(email_verificado, FALSE)
			 FROM projection_estudantes
			 WHERE codigo_estudante = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &emailVerificado)
	case "academia":
		err = client.DB().QueryRow(
			`SELECT id, COALESCE(email, ''), COALESCE(email_verificado, FALSE)
			 FROM projection_academias
			 WHERE codigo_academia = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &emailVerificado)
	case "admin":
		err = client.DB().QueryRow(
			`SELECT id, email, COALESCE(email_verificado, FALSE)
			 FROM projection_admins WHERE email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &emailVerificado)
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo deve ser 'estudante', 'academia' ou 'admin'"))
		return
	}

	if err != nil {
		utils.RespondWithNotFoundError(c, "usuário")
		return
	}

	if email == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("usuário não possui email cadastrado"))
		return
	}

	if !emailVerificado {
		utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de solicitar recuperação de senha")
		return
	}

	userID, _ = uuid.Parse(idStr)

	_, err = emailSvc.SaveToken(userID, req.Tipo, "recuperacao_senha", email, 1*time.Hour)
	if err != nil {
		log.Printf("Erro ao gerar token de recuperação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Token de recuperação gerado para: %s", email)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Email de recuperação de senha enviado. Verifique sua caixa de entrada.",
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