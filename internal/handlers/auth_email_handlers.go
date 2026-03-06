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
		return
	}

	// ── Academia: event sourcing (FIX-C2) ─────────────────────────────────
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
		return
	}

	// ── Estudante: event sourcing (FIX-C3) ────────────────────────────────
	if tokenInfo.UserType == "estudante" {
		repository := getRepository(c)
		estudanteAgg, err := repository.Load(tokenInfo.UserID, "Estudante")
		if err != nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		estudante := estudanteAgg.(*aggregates.Estudante)

		alreadyVerified := false
		if err := estudante.VerificarEmail(); err != nil {
			if err.Error() == "email já verificado" {
				alreadyVerified = true
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
// FIX-C1: academia usa event sourcing via aggregate.AlterarSenha().
// FIX-C3b: estudante exige email_verificado=TRUE e usa event sourcing.
// FIX E4-LN-01: admin recebe a nova senha no body do request — não mais
//   uma constante hardcoded extraída de GetDefaultPassword(). A senha
//   hardcoded era pública no repositório, tornando qualquer conta que
//   passasse por reset trivialmente comprometível por qualquer pessoa
//   com acesso ao código-fonte.
func ResetarSenha(c *gin.Context) {
	token := c.Param("token")

	// FIX E4-LN-01: nova_senha obrigatória no body para TODOS os tipos.
	// Para admins: substitui o GetDefaultPassword hardcoded.
	// Para estudantes: mantém compatibilidade com o fluxo existente que já
	//   usava o código como senha; mas aceitar nova_senha do body é mais seguro.
	// O campo é opcional para estudantes (preserva backward compat) mas
	// obrigatório para admins.
	var req struct {
		NovaSenha string `json:"nova_senha"`
	}
	_ = c.ShouldBindJSON(&req) // bind opcional — verificamos por tipo abaixo

	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "recuperacao_senha")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	client := getDbClient(c)

	// ── Admin: event sourcing ──────────────────────────────────────────────
	if tokenInfo.UserType == "admin" {
		// FIX E4-LN-01: nova_senha obrigatória no body para admins.
		if req.NovaSenha == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("nova_senha é obrigatória no body"))
			return
		}

		if err := utils.ValidateSenha(req.NovaSenha); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}

		var emailVerificado bool
		err = client.DB().QueryRow(
			`SELECT COALESCE(email_verificado, FALSE) FROM projection_admins WHERE id = $1`,
			tokenInfo.UserID,
		).Scan(&emailVerificado)
		if err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}

		if !emailVerificado {
			utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de resetar a senha")
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
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
			"message": "Senha redefinida com sucesso!",
			"email":   tokenInfo.Email,
		})
		return
	}

	// ── Academia: event sourcing (FIX-C1) ─────────────────────────────────
	if tokenInfo.UserType == "academia" {
		// Aceitar nova_senha do body se fornecida; caso contrário usar código como antes.
		var senhaParaDefinir string
		if req.NovaSenha != "" {
			if err := utils.ValidateSenha(req.NovaSenha); err != nil {
				utils.RespondWithValidationError(c, err)
				return
			}
			senhaParaDefinir = req.NovaSenha
		} else {
			var codigoAcademia string
			if err := client.DB().QueryRow(
				`SELECT codigo_academia FROM projection_academias WHERE id = $1`,
				tokenInfo.UserID,
			).Scan(&codigoAcademia); err != nil {
				utils.RespondWithNotFoundError(c, "academia")
				return
			}
			senhaParaDefinir = codigoAcademia
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(senhaParaDefinir), bcrypt.DefaultCost)
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

		if err := academia.AlterarSenha(string(hashedPassword)); err != nil {
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
		c.JSON(http.StatusOK, gin.H{
			"message":         "Senha resetada com sucesso!",
			"proximos_passos": "Faça login e altere para uma senha segura.",
		})
		return
	}

	// ── Estudante: event sourcing (FIX-C3b) ───────────────────────────────
	if tokenInfo.UserType == "estudante" {
		var emailVerificado bool
		err = client.DB().QueryRow(
			`SELECT COALESCE(email_verificado, FALSE) FROM projection_estudantes WHERE id = $1`,
			tokenInfo.UserID,
		).Scan(&emailVerificado)
		if err != nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}

		if !emailVerificado {
			utils.RespondWithForbiddenError(c, "Por favor, verifique seu email antes de resetar a senha")
			return
		}

		// Aceitar nova_senha do body se fornecida; caso contrário usar código.
		var senhaParaDefinir string
		if req.NovaSenha != "" {
			if err := utils.ValidateSenha(req.NovaSenha); err != nil {
				utils.RespondWithValidationError(c, err)
				return
			}
			senhaParaDefinir = req.NovaSenha
		} else {
			var codigoEstudante string
			if err := client.DB().QueryRow(
				`SELECT codigo_estudante FROM projection_estudantes WHERE id = $1`,
				tokenInfo.UserID,
			).Scan(&codigoEstudante); err != nil {
				utils.RespondWithNotFoundError(c, "estudante")
				return
			}
			senhaParaDefinir = codigoEstudante
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(senhaParaDefinir), bcrypt.DefaultCost)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		repository := getRepository(c)
		estudanteAgg, err := repository.Load(tokenInfo.UserID, "Estudante")
		if err != nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		estudante := estudanteAgg.(*aggregates.Estudante)

		if err := estudante.AlterarSenha(string(hashedPassword)); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		audit := db.AuditContext{
			UserID:   "sistema",
			UserType: "sistema",
			IP:       c.ClientIP(),
		}
		if err := repository.SaveWithAudit(estudante, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		log.Printf("Senha resetada (event sourcing) para estudante: %s", tokenInfo.Email)
		c.JSON(http.StatusOK, gin.H{
			"message":         "Senha resetada com sucesso!",
			"proximos_passos": "Faça login com sua nova senha.",
		})
		return
	}

	utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
}

// ============================================================================
// Geração de tokens
// ============================================================================

// GerarTokenVerificacao envia email de verificação ao usuário autenticado.
//
// FIX HDL-04: token de verificação REMOVIDO da resposta HTTP.
func GerarTokenVerificacao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	var email, nome string
	client := getDbClient(c)

	switch userType {
	case "estudante":
		err := client.DB().QueryRow(
			`SELECT COALESCE(email,''), nome FROM projection_estudantes WHERE id = $1`,
			userID,
		).Scan(&email, &nome)
		if err != nil || email == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("estudante não possui email cadastrado"))
			return
		}
	case "academia":
		err := client.DB().QueryRow(
			`SELECT COALESCE(email,''), nome FROM projection_academias WHERE id = $1`,
			userID,
		).Scan(&email, &nome)
		if err != nil || email == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("academia não possui email cadastrado"))
			return
		}
	case "admin":
		err := client.DB().QueryRow(
			`SELECT email, nome FROM projection_admins WHERE id = $1`,
			userID,
		).Scan(&email, &nome)
		if err != nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
		return
	}

	emailSvc := getEmailService(c)
	if err := emailSvc.SendVerificationEmail(userID, userType, email, nome); err != nil {
		log.Printf("Erro ao enviar email de verificação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	// FIX HDL-04: token NÃO incluído na resposta.
	c.JSON(http.StatusOK, gin.H{
		"message": "Email de verificação enviado com sucesso!",
		"email":   email,
	})
}

func SolicitarVerificacaoEmail(c *gin.Context) {
	GerarTokenVerificacao(c)
}

func SolicitarRecuperacaoSenha(c *gin.Context) {
	GerarTokenRecuperacao(c)
}

// GerarTokenRecuperacao solicita recuperação de senha para um usuário.
//
// FIX HDL-03: token de recuperação REMOVIDO da resposta HTTP.
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

	if err := emailSvc.SendPasswordResetEmail(userID, req.Tipo, email, nome); err != nil {
		log.Printf("Erro ao enviar email de recuperação: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Email de recuperação enviado para: %s", email)

	// FIX HDL-03: token NÃO incluído na resposta.
	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"message":   "Email de recuperação enviado com sucesso. Verifique sua caixa de entrada.",
		"expira_em": "1 hora",
	})
}

// ============================================================================
// Helpers internos de email (reutilizáveis por outros handlers)
// ============================================================================

// gerarEEnviarTokenVerificacao é um helper interno reutilizável.
// Não expõe o token — envia diretamente para o email.
func gerarEEnviarTokenVerificacao(emailSvc interface {
	SendVerificationEmail(uuid.UUID, string, string, string) error
}, userID uuid.UUID, userType, email, nome string) error {
	return emailSvc.SendVerificationEmail(userID, userType, email, nome)
}

// gerarEEnviarTokenRecuperacao é um helper interno reutilizável.
func gerarEEnviarTokenRecuperacao(emailSvc interface {
	SendPasswordResetEmail(uuid.UUID, string, string, string) error
}, userID uuid.UUID, userType, email, nome string) error {
	return emailSvc.SendPasswordResetEmail(userID, userType, email, nome)
}

// Ensure time import is used (used in token expiry calculations at service layer).
var _ = time.Hour