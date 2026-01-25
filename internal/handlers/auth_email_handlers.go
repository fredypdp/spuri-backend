// ============================================================================
// auth_email_handlers.go - Handlers de verificação e recuperação + logs debug
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// SolicitarVerificacaoEmail envia email de verificação
func SolicitarVerificacaoEmail(c *gin.Context) {
	log.Printf("[HANDLER][INICIO] SolicitarVerificacaoEmail")
	
	var req struct {
		Identificador string `json:"identificador" binding:"required"`
		Tipo          string `json:"tipo" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[HANDLER][ERRO] Bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[HANDLER] Identificador: %s, Tipo: %s", req.Identificador, req.Tipo)

	client := getDbClient(c)
	log.Printf("[HANDLER] DbClient obtido: %v", client != nil)
	
	emailSvc := getEmailService(c)
	log.Printf("[HANDLER] EmailService obtido: %v", emailSvc != nil)
	
	if emailSvc == nil {
		log.Printf("[HANDLER][ERRO CRÍTICO] EmailService é NIL!")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "serviço de email não disponível"})
		return
	}
	
	log.Printf("[HANDLER] EmailService.IsEnabled(): %v", emailSvc.IsEnabled())

	var userID uuid.UUID
	var email, nome string
	var query string

	safeId := db.SafeString(req.Identificador)

	switch req.Tipo {
	case "estudante":
		query = fmt.Sprintf(`SELECT id, email, nome FROM projection_estudantes 
		         WHERE codigo_estudante = '%s' OR email = '%s'`, safeId, safeId)
	case "academia":
		query = fmt.Sprintf(`SELECT id, email, nome FROM projection_academias 
		         WHERE codigo_academia = '%s' OR email = '%s'`, safeId, safeId)
	case "admin":
		query = fmt.Sprintf(`SELECT id, email, nome FROM projection_admins WHERE email = '%s'`, safeId)
	default:
		log.Printf("[HANDLER][ERRO] Tipo inválido: %s", req.Tipo)
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo inválido"})
		return
	}

	log.Printf("[HANDLER] Executando query: %s", query)

	var idStr string
	err := client.DB().QueryRow(query).Scan(&idStr, &email, &nome)
	if err != nil {
		log.Printf("[HANDLER][ERRO] Usuário não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}
	
	userID, _ = uuid.Parse(idStr)
	log.Printf("[HANDLER] Usuário encontrado - ID: %s, Nome: %s, Email: %s", userID, nome, email)

	if email == "" {
		log.Printf("[HANDLER][ERRO] Email não cadastrado")
		c.JSON(http.StatusBadRequest, gin.H{"error": "usuário não possui email cadastrado"})
		return
	}

	log.Printf("[HANDLER] CHAMANDO SendVerificationEmail...")
	
	if err := emailSvc.SendVerificationEmail(userID, req.Tipo, email, nome); err != nil {
		log.Printf("[HANDLER][ERRO CRÍTICO] SendVerificationEmail FALHOU: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao enviar email",
			"details": err.Error(), // TEMPORÁRIO para debug
		})
		return
	}

	log.Printf("[HANDLER][SUCESSO] Email enviado com sucesso")

	c.JSON(http.StatusOK, gin.H{
		"message": "Email de verificação enviado! Verifique sua caixa de entrada.",
		"email":   maskEmail(email),
	})
}

// VerificarEmail verifica email usando token (POST)
func VerificarEmail(c *gin.Context) {
	token := c.Param("token")
	log.Printf("[DEBUG] VerificarEmail: Token recebido")
	
	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "verificacao_email")
	if err != nil {
		log.Printf("[ERROR] VerificarEmail: Token inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] VerificarEmail: Token válido - UserID: %s, Tipo: %s", tokenInfo.UserID, tokenInfo.UserType)

	var table string
	switch tokenInfo.UserType {
	case "estudante":
		table = "projection_estudantes"
	case "academia":
		table = "projection_academias"
	case "admin":
		table = "projection_admins"
	default:
		log.Printf("[ERROR] VerificarEmail: Tipo de usuário inválido: %s", tokenInfo.UserType)
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo de usuário inválido"})
		return
	}

	client := getDbClient(c)
	query := fmt.Sprintf("UPDATE %s SET email_verificado = TRUE WHERE id = '%s'", table, tokenInfo.UserID)
	
	log.Printf("[DEBUG] VerificarEmail: Atualizando email_verificado na tabela %s", table)
	
	_, err = client.DB().Exec(query)
	if err != nil {
		log.Printf("[ERROR] VerificarEmail: Erro ao atualizar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar email"})
		return
	}

	log.Printf("[DEBUG] VerificarEmail: Email verificado com sucesso para %s", tokenInfo.Email)

	c.JSON(http.StatusOK, gin.H{
		"message": "Email verificado com sucesso!",
		"email":   tokenInfo.Email,
	})
}

// SolicitarRecuperacaoSenha envia email de recuperação
func SolicitarRecuperacaoSenha(c *gin.Context) {
	log.Printf("[DEBUG] SolicitarRecuperacaoSenha: Início")
	
	var req struct {
		Identificador string `json:"identificador" binding:"required"`
		Tipo          string `json:"tipo" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] SolicitarRecuperacaoSenha: Erro no bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[DEBUG] SolicitarRecuperacaoSenha: Identificador: %s, Tipo: %s", req.Identificador, req.Tipo)

	client := getDbClient(c)
	emailSvc := getEmailService(c)

	var userID uuid.UUID
	var email, nome string
	var query string

	safeId := db.SafeString(req.Identificador)

	switch req.Tipo {
	case "estudante":
		query = fmt.Sprintf(`SELECT id, email, nome FROM projection_estudantes 
		         WHERE codigo_estudante = '%s' OR email = '%s'`, safeId, safeId)
	case "academia":
		query = fmt.Sprintf(`SELECT id, email, nome FROM projection_academias 
		         WHERE codigo_academia = '%s' OR email = '%s'`, safeId, safeId)
	case "admin":
		query = fmt.Sprintf(`SELECT id, email, nome FROM projection_admins WHERE email = '%s'`, safeId)
	default:
		log.Printf("[ERROR] SolicitarRecuperacaoSenha: Tipo inválido: %s", req.Tipo)
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo inválido"})
		return
	}

	log.Printf("[DEBUG] SolicitarRecuperacaoSenha: Executando query")

	var idStr string
	err := client.DB().QueryRow(query).Scan(&idStr, &email, &nome)
	if err != nil {
		log.Printf("[ERROR] SolicitarRecuperacaoSenha: Usuário não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}
	
	userID, _ = uuid.Parse(idStr)
	log.Printf("[DEBUG] SolicitarRecuperacaoSenha: Usuário encontrado - ID: %s, Nome: %s", userID, nome)

	if email == "" {
		log.Printf("[ERROR] SolicitarRecuperacaoSenha: Email não cadastrado")
		c.JSON(http.StatusBadRequest, gin.H{"error": "usuário não possui email cadastrado"})
		return
	}

	log.Printf("[DEBUG] SolicitarRecuperacaoSenha: Enviando email para: %s", email)

	if err := emailSvc.SendPasswordResetEmail(userID, req.Tipo, email, nome); err != nil {
		log.Printf("[ERROR] SolicitarRecuperacaoSenha: Erro ao enviar email: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao enviar email"})
		return
	}

	log.Printf("[DEBUG] SolicitarRecuperacaoSenha: Email enviado com sucesso")

	c.JSON(http.StatusOK, gin.H{
		"message": "Email de recuperação enviado! Verifique sua caixa de entrada.",
		"email":   maskEmail(email),
	})
}

// ResetarSenha redefine senha usando token (POST)
func ResetarSenha(c *gin.Context) {
	token := c.Param("token")
	log.Printf("[DEBUG] ResetarSenha: Token recebido")

	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "recuperacao_senha")
	if err != nil {
		log.Printf("[ERROR] ResetarSenha: Token inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] ResetarSenha: Token válido - UserID: %s, Tipo: %s", tokenInfo.UserID, tokenInfo.UserType)

	client := getDbClient(c)

	var codigo string
	var table string
	var query string

	switch tokenInfo.UserType {
	case "estudante":
		query = fmt.Sprintf(`SELECT codigo_estudante FROM projection_estudantes WHERE id = '%s'`, tokenInfo.UserID)
		table = "projection_estudantes"
	case "academia":
		query = fmt.Sprintf(`SELECT codigo_academia FROM projection_academias WHERE id = '%s'`, tokenInfo.UserID)
		table = "projection_academias"
	case "admin":
		query = fmt.Sprintf(`SELECT role FROM projection_admins WHERE id = '%s'`, tokenInfo.UserID)
		table = "projection_admins"
	default:
		log.Printf("[ERROR] ResetarSenha: Tipo de usuário inválido: %s", tokenInfo.UserType)
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo de usuário inválido"})
		return
	}

	log.Printf("[DEBUG] ResetarSenha: Buscando código do usuário")

	err = client.DB().QueryRow(query).Scan(&codigo)
	if err != nil {
		log.Printf("[ERROR] ResetarSenha: Usuário não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}

	log.Printf("[DEBUG] ResetarSenha: Código encontrado: %s", codigo)

	defaultPassword := services.GetDefaultPassword(tokenInfo.UserType, codigo)
	log.Printf("[DEBUG] ResetarSenha: Gerando hash da senha padrão")
	
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[ERROR] ResetarSenha: Erro ao gerar hash: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	safeHash := db.SafeString(string(hashedPassword))
	updateQuery := fmt.Sprintf("UPDATE %s SET senha_hash = '%s' WHERE id = '%s'", table, safeHash, tokenInfo.UserID)
	
	log.Printf("[DEBUG] ResetarSenha: Atualizando senha na tabela %s", table)
	
	_, err = client.DB().Exec(updateQuery)
	if err != nil {
		log.Printf("[ERROR] ResetarSenha: Erro ao atualizar senha: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao resetar senha"})
		return
	}

	log.Printf("[DEBUG] ResetarSenha: Senha resetada com sucesso")

	c.JSON(http.StatusOK, gin.H{
		"message":         "Senha resetada com sucesso!",
		"senha_padrao":    defaultPassword,
		"email":           tokenInfo.Email,
		"proximos_passos": "Faça login com a senha padrão e altere para uma senha segura.",
	})
}

// AlterarSenha altera senha do usuário logado
func AlterarSenha(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userType, _ := c.Get("user_type")

	log.Printf("[DEBUG] AlterarSenha: UserID: %s, UserType: %s", userID, userType)

	var req struct {
		SenhaAtual string `json:"senha_atual" binding:"required"`
		NovaSenha  string `json:"nova_senha" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] AlterarSenha: Erro no bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	if len(req.NovaSenha) < 6 {
		log.Printf("[ERROR] AlterarSenha: Senha muito curta")
		c.JSON(http.StatusBadRequest, gin.H{"error": "nova senha deve ter no mínimo 6 caracteres"})
		return
	}

	client := getDbClient(c)

	var table string
	switch userType {
	case "estudante":
		table = "projection_estudantes"
	case "academia":
		table = "projection_academias"
	case "admin":
		table = "projection_admins"
	default:
		log.Printf("[ERROR] AlterarSenha: Tipo de usuário inválido: %v", userType)
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo de usuário inválido"})
		return
	}

	query := fmt.Sprintf("SELECT senha_hash FROM %s WHERE id = '%s'", table, userID)
	
	log.Printf("[DEBUG] AlterarSenha: Buscando senha atual")
	
	var senhaHash string
	err := client.DB().QueryRow(query).Scan(&senhaHash)
	if err != nil {
		log.Printf("[ERROR] AlterarSenha: Usuário não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}

	log.Printf("[DEBUG] AlterarSenha: Verificando senha atual")

	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.SenhaAtual)); err != nil {
		log.Printf("[ERROR] AlterarSenha: Senha atual incorreta")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "senha atual incorreta"})
		return
	}

	log.Printf("[DEBUG] AlterarSenha: Gerando hash da nova senha")

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[ERROR] AlterarSenha: Erro ao gerar hash: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	safeHash := db.SafeString(string(hashedPassword))
	updateQuery := fmt.Sprintf("UPDATE %s SET senha_hash = '%s' WHERE id = '%s'", table, safeHash, userID)
	
	log.Printf("[DEBUG] AlterarSenha: Atualizando senha")
	
	_, err = client.DB().Exec(updateQuery)
	if err != nil {
		log.Printf("[ERROR] AlterarSenha: Erro ao atualizar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao alterar senha"})
		return
	}

	log.Printf("[DEBUG] AlterarSenha: Senha alterada com sucesso")

	c.JSON(http.StatusOK, gin.H{
		"message": "Senha alterada com sucesso!",
	})
}

func maskEmail(email string) string {
	if len(email) < 5 {
		return "***@***"
	}
	
	atIndex := -1
	for i, char := range email {
		if char == '@' {
			atIndex = i
			break
		}
	}
	
	if atIndex == -1 {
		return "***@***"
	}
	
	if atIndex <= 2 {
		return "***@" + email[atIndex+1:]
	}
	
	masked := string(email[0]) + "***" + string(email[atIndex-1]) + email[atIndex:]
	return masked
}

func getEmailService(c *gin.Context) *services.EmailService {
	client := getDbClient(c)
	return services.NewEmailService(client.DB())
}