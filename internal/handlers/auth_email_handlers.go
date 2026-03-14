package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
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
// FIX-C3: estudante usa event sourcing via aggregate.VerificarEmail(),
// idêntico ao fluxo do Admin e Academia.
// FIX H4-RST-04: estrutura refatorada de if/if/if para switch — mais segura
// ao adicionar novos tipos de usuário (qualquer tipo não coberto cai no default).
func VerificarEmail(c *gin.Context) {
	token := c.Param("token")

	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "verificacao_email")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	switch tokenInfo.UserType {

	// ── Admin: event sourcing ──────────────────────────────────────────────
	case "admin":
		repository := getRepository(c)
		adminAgg, err := repository.Load(tokenInfo.UserID, "Admin")
		if err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}

		admin, ok := adminAgg.(*aggregates.Admin)
		if !ok {
			utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
			return
		}

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
		c.JSON(http.StatusOK, gin.H{"message": msg, "email": tokenInfo.Email})

	// ── Academia: event sourcing ───────────────────────────────────────────
	case "academia":
		repository := getRepository(c)
		academiaAgg, err := repository.Load(tokenInfo.UserID, "Academia")
		if err != nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}

		academia, ok := academiaAgg.(*aggregates.Academia)
		if !ok {
			utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
			return
		}

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
		c.JSON(http.StatusOK, gin.H{"message": msg, "email": tokenInfo.Email})

	// ── Estudante: event sourcing ──────────────────────────────────────────
	case "estudante":
		repository := getRepository(c)
		estudanteAgg, err := repository.Load(tokenInfo.UserID, "Estudante")
		if err != nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}

		estudante, ok := estudanteAgg.(*aggregates.Estudante)
		if !ok {
			utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
			return
		}

		alreadyVerified := false
		if err := estudante.VerificarEmail(); err != nil {
			if err.Error() == "email já verificado" {
				alreadyVerified = true
				log.Printf("[INFO] Email já estava verificado para estudante: %s", tokenInfo.Email)
			} else {
				utils.RespondWithInternalError(c, err)
				return
			}
		}

		if !alreadyVerified {
			audit := db.AuditContext{
				UserID:   tokenInfo.UserID.String(),
				UserType: "estudante",
				IP:       c.ClientIP(),
			}
			if err := repository.SaveWithAudit(estudante, audit); err != nil {
				utils.RespondWithInternalError(c, err)
				return
			}
			log.Printf("Email verificado (event sourcing) para estudante: %s", tokenInfo.Email)
		}

		msg := "Email verificado com sucesso!"
		if alreadyVerified {
			msg = "Email já estava verificado."
		}
		c.JSON(http.StatusOK, gin.H{"message": msg, "email": tokenInfo.Email})

	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido no token: %s", tokenInfo.UserType))
	}
}

// ============================================================================
// Geração de tokens (retornam o token ao frontend — frontend envia o email)
// ============================================================================

// GerarTokenVerificacao gera o token de verificação e o RETORNA ao frontend.
// O frontend (Next.js) é responsável por enviar o email com o token.
// Rota: POST /email/gerar-token/verificacao  (requer AuthMiddleware)
func GerarTokenVerificacao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	var email, nome string
	client := getDbClient(c)

	switch userType {
	case "estudante":
		if err := client.DB().QueryRow(
			`SELECT COALESCE(email,''), nome FROM projection_estudantes WHERE id = $1`,
			userID,
		).Scan(&email, &nome); err != nil || email == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("estudante não possui email cadastrado"))
			return
		}
	case "academia":
		if err := client.DB().QueryRow(
			`SELECT COALESCE(email,''), nome FROM projection_academias WHERE id = $1`,
			userID,
		).Scan(&email, &nome); err != nil || email == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("academia não possui email cadastrado"))
			return
		}
	case "admin":
		if err := client.DB().QueryRow(
			`SELECT email, nome FROM projection_admins WHERE id = $1`,
			userID,
		).Scan(&email, &nome); err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
		return
	}

	emailSvc := getEmailService(c)

	// Apenas gera e persiste o token — NÃO envia email (frontend faz isso)
	token, err := emailSvc.SaveToken(userID, userType, "verificacao_email", email, 24*time.Hour)
	if err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao gerar token: %w", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"token":     token,
		"email":     email,
		"nome":      nome,
		"tipo":      userType,
		"expira_em": "24 horas",
	})
}

// GerarTokenRecuperacao gera o token de recuperação e o RETORNA ao frontend.
// O frontend (Next.js) é responsável por enviar o email com o token.
// Rota: POST /email/gerar-token/recuperacao  (pública — usa identificador no body)
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

	var userID uuid.UUID
	var email, nome string
	var emailVerificado bool
	var idStr string
	var err error

	switch req.Tipo {
	case "estudante":
		err = client.DB().QueryRow(
			`SELECT id, COALESCE(email,''), nome, COALESCE(email_verificado, FALSE)
			 FROM projection_estudantes
			 WHERE codigo_estudante = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome, &emailVerificado)
	case "academia":
		err = client.DB().QueryRow(
			`SELECT id, COALESCE(email,''), nome, COALESCE(email_verificado, FALSE)
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

	emailSvc := getEmailService(c)

	// Apenas gera e persiste o token — NÃO envia email (frontend faz isso)
	token, err := emailSvc.SaveToken(userID, req.Tipo, "recuperacao_senha", email, 1*time.Hour)
	if err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao gerar token: %w", err))
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

// ============================================================================
// Solicitar (backend gera token E envia email — sem passar token ao frontend)
// ============================================================================

// SolicitarVerificacaoEmail gera o token e envia o email de verificação diretamente.
// Usado por fluxos onde o backend controla o envio completo.
// Rota: POST /email/verificar-email/solicitar  (requer AuthMiddleware)
func SolicitarVerificacaoEmail(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	var email, nome string
	client := getDbClient(c)

	switch userType {
	case "estudante":
		if err := client.DB().QueryRow(
			`SELECT COALESCE(email,''), nome FROM projection_estudantes WHERE id = $1`,
			userID,
		).Scan(&email, &nome); err != nil || email == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("estudante não possui email cadastrado"))
			return
		}
	case "academia":
		if err := client.DB().QueryRow(
			`SELECT COALESCE(email,''), nome FROM projection_academias WHERE id = $1`,
			userID,
		).Scan(&email, &nome); err != nil || email == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("academia não possui email cadastrado"))
			return
		}
	case "admin":
		if err := client.DB().QueryRow(
			`SELECT email, nome FROM projection_admins WHERE id = $1`,
			userID,
		).Scan(&email, &nome); err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
		return
	}

	emailSvc := getEmailService(c)

	// Gera token e envia o email diretamente — token NÃO retornado ao frontend
	if err := emailSvc.SendVerificationEmail(userID, userType, email, nome); err != nil {
		log.Printf("Erro ao enviar email de verificação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Email de verificação enviado para: %s", email)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Email de verificação enviado com sucesso!",
		"email":   email,
	})
}

// SolicitarRecuperacaoSenha gera o token e envia o email de recuperação diretamente.
// Usado por fluxos onde o backend controla o envio completo.
// Rota: POST /email/recuperar-senha/solicitar  (pública — usa identificador no body)
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

	var userID uuid.UUID
	var email, nome string
	var emailVerificado bool
	var idStr string
	var err error

	switch req.Tipo {
	case "estudante":
		err = client.DB().QueryRow(
			`SELECT id, COALESCE(email,''), nome, COALESCE(email_verificado, FALSE)
			 FROM projection_estudantes
			 WHERE codigo_estudante = $1 OR email = $1`,
			req.Identificador,
		).Scan(&idStr, &email, &nome, &emailVerificado)
	case "academia":
		err = client.DB().QueryRow(
			`SELECT id, COALESCE(email,''), nome, COALESCE(email_verificado, FALSE)
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

	emailSvc := getEmailService(c)

	// Gera token e envia o email diretamente — token NÃO retornado ao frontend
	if err := emailSvc.SendPasswordResetEmail(userID, req.Tipo, email, nome); err != nil {
		log.Printf("Erro ao enviar email de recuperação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Email de recuperação enviado para: %s", email)

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "Email de recuperação enviado com sucesso. Verifique sua caixa de entrada.",
		"expira_em": "1 hora",
	})
}

// ============================================================================
// Reset de senha via token
// ============================================================================

// ResetarSenha redefine a senha usando token de recuperação.
func ResetarSenha(c *gin.Context) {
	token := c.Param("token")

	var req struct {
		NovaSenha string `json:"nova_senha" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("nova_senha é obrigatória"))
		return
	}

	// VerifyToken valida e consome o token — prevenindo replay attacks.
	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "recuperacao_senha")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	client := getDbClient(c)

	switch tokenInfo.UserType {

	// ── Admin ──────────────────────────────────────────────────────────────
	case "admin":
		var emailVerificado bool
		if err = client.DB().QueryRow(
			`SELECT COALESCE(email_verificado, FALSE) FROM projection_admins WHERE id = $1`,
			tokenInfo.UserID,
		).Scan(&emailVerificado); err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
		if !emailVerificado {
			utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de resetar a senha")
			return
		}

		repository := getRepository(c)
		adminAgg, err := repository.Load(tokenInfo.UserID, "Admin")
		if err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
		admin, ok := adminAgg.(*aggregates.Admin)
		if !ok {
			utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
			return
		}
		if err := admin.AlterarSenha(string(hashedPassword), uuid.Nil, "reset_senha"); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		audit := db.AuditContext{UserID: "sistema", UserType: "sistema", IP: c.ClientIP()}
		if err := repository.SaveWithAudit(admin, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		log.Printf("Senha resetada (event sourcing) para admin: %s", tokenInfo.Email)
		c.JSON(http.StatusOK, gin.H{
			"message": "Senha redefinida com sucesso!",
			"email":   tokenInfo.Email,
		})

	// ── Academia ───────────────────────────────────────────────────────────
	case "academia":
		var emailVerificado bool
		if err = client.DB().QueryRow(
			`SELECT COALESCE(email_verificado, FALSE) FROM projection_academias WHERE id = $1`,
			tokenInfo.UserID,
		).Scan(&emailVerificado); err != nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		if !emailVerificado {
			utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de resetar a senha")
			return
		}

		repository := getRepository(c)
		academiaAgg, err := repository.Load(tokenInfo.UserID, "Academia")
		if err != nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		academia, ok := academiaAgg.(*aggregates.Academia)
		if !ok {
			utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
			return
		}
		if err := academia.AlterarSenha(string(hashedPassword), uuid.Nil, "reset_senha"); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		audit := db.AuditContext{UserID: "sistema", UserType: "sistema", IP: c.ClientIP()}
		if err := repository.SaveWithAudit(academia, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		log.Printf("Senha resetada (event sourcing) para academia: %s", tokenInfo.Email)
		c.JSON(http.StatusOK, gin.H{
			"message":         "Senha resetada com sucesso!",
			"proximos_passos": "Faça login e altere para uma senha segura.",
		})

	// ── Estudante ──────────────────────────────────────────────────────────
	case "estudante":
		var emailVerificado bool
		if err = client.DB().QueryRow(
			`SELECT COALESCE(email_verificado, FALSE) FROM projection_estudantes WHERE id = $1`,
			tokenInfo.UserID,
		).Scan(&emailVerificado); err != nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		if !emailVerificado {
			utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de resetar a senha")
			return
		}

		repository := getRepository(c)
		estudanteAgg, err := repository.Load(tokenInfo.UserID, "Estudante")
		if err != nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		estudante, ok := estudanteAgg.(*aggregates.Estudante)
		if !ok {
			utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
			return
		}
		if err := estudante.AlterarSenha(string(hashedPassword)); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		audit := db.AuditContext{UserID: "sistema", UserType: "sistema", IP: c.ClientIP()}
		if err := repository.SaveWithAudit(estudante, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		log.Printf("Senha resetada (event sourcing) para estudante: %s", tokenInfo.Email)
		c.JSON(http.StatusOK, gin.H{
			"message":         "Senha resetada com sucesso!",
			"proximos_passos": "Faça login com sua nova senha.",
		})

	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido no token: %s", tokenInfo.UserType))
	}
}

// ============================================================================
// Helpers internos
// ============================================================================

func gerarEEnviarTokenVerificacao(emailSvc interface {
	SendVerificationEmail(uuid.UUID, string, string, string) error
}, userID uuid.UUID, userType, email, nome string) error {
	return emailSvc.SendVerificationEmail(userID, userType, email, nome)
}

func gerarEEnviarTokenRecuperacao(emailSvc interface {
	SendPasswordResetEmail(uuid.UUID, string, string, string) error
}, userID uuid.UUID, userType, email, nome string) error {
	return emailSvc.SendPasswordResetEmail(userID, userType, email, nome)
}

var _ = time.Hour