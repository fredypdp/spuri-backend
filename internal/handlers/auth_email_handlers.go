package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/services"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// SolicitarVerificacaoEmail envia email de verificação
func SolicitarVerificacaoEmail(c *gin.Context) {
	var req struct {
		Identificador string `json:"identificador" binding:"required"`
		Tipo          string `json:"tipo" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	client := getDbClient(c)
	emailSvc := getEmailService(c)
	
	if emailSvc == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "serviço de email não disponível"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo inválido"})
		return
	}

	var idStr string
	err := client.DB().QueryRow(query).Scan(&idStr, &email, &nome)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}
	
	userID, _ = uuid.Parse(idStr)

	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usuário não possui email cadastrado"})
		return
	}

	if err := emailSvc.SendVerificationEmail(userID, req.Tipo, email, nome); err != nil {
		log.Printf("Erro ao enviar email de verificação: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao enviar email"})
		return
	}

	log.Printf("Email de verificação enviado para: %s", email)

	c.JSON(http.StatusOK, gin.H{
		"message": "Email de verificação enviado! Verifique sua caixa de entrada.",
		"email":   maskEmail(email),
	})
}

// VerificarEmail verifica email usando token
func VerificarEmail(c *gin.Context) {
	token := c.Param("token")
	
	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "verificacao_email")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var table string
	switch tokenInfo.UserType {
	case "estudante":
		table = "projection_estudantes"
	case "academia":
		table = "projection_academias"
	case "admin":
		table = "projection_admins"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo de usuário inválido"})
		return
	}

	client := getDbClient(c)
	query := fmt.Sprintf("UPDATE %s SET email_verificado = TRUE WHERE id = '%s'", table, tokenInfo.UserID)
	
	_, err = client.DB().Exec(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar email"})
		return
	}

	log.Printf("Email verificado: %s", tokenInfo.Email)

	c.JSON(http.StatusOK, gin.H{
		"message": "Email verificado com sucesso!",
		"email":   tokenInfo.Email,
	})
}

// SolicitarRecuperacaoSenha envia email de recuperação
func SolicitarRecuperacaoSenha(c *gin.Context) {
	var req struct {
		Identificador string `json:"identificador" binding:"required"`
		Tipo          string `json:"tipo" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo inválido"})
		return
	}

	var idStr string
	err := client.DB().QueryRow(query).Scan(&idStr, &email, &nome)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}
	
	userID, _ = uuid.Parse(idStr)

	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usuário não possui email cadastrado"})
		return
	}

	if err := emailSvc.SendPasswordResetEmail(userID, req.Tipo, email, nome); err != nil {
		log.Printf("Erro ao enviar email de recuperação: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao enviar email"})
		return
	}

	log.Printf("Email de recuperação enviado para: %s", email)

	c.JSON(http.StatusOK, gin.H{
		"message": "Email de recuperação enviado! Verifique sua caixa de entrada.",
		"email":   maskEmail(email),
	})
}

// ResetarSenha redefine senha usando token
func ResetarSenha(c *gin.Context) {
	token := c.Param("token")

	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "recuperacao_senha")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo de usuário inválido"})
		return
	}

	err = client.DB().QueryRow(query).Scan(&codigo)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}

	defaultPassword := services.GetDefaultPassword(tokenInfo.UserType, codigo)
	
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	safeHash := db.SafeString(string(hashedPassword))
	updateQuery := fmt.Sprintf("UPDATE %s SET senha_hash = '%s' WHERE id = '%s'", table, safeHash, tokenInfo.UserID)
	
	_, err = client.DB().Exec(updateQuery)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao resetar senha"})
		return
	}

	log.Printf("Senha resetada para: %s", tokenInfo.Email)

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

	var req struct {
		SenhaAtual string `json:"senha_atual" binding:"required"`
		NovaSenha  string `json:"nova_senha" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	if len(req.NovaSenha) < 6 {
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo de usuário inválido"})
		return
	}

	query := fmt.Sprintf("SELECT senha_hash FROM %s WHERE id = '%s'", table, userID)
	
	var senhaHash string
	err := client.DB().QueryRow(query).Scan(&senhaHash)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.SenhaAtual)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "senha atual incorreta"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	safeHash := db.SafeString(string(hashedPassword))
	updateQuery := fmt.Sprintf("UPDATE %s SET senha_hash = '%s' WHERE id = '%s'", table, safeHash, userID)
	
	_, err = client.DB().Exec(updateQuery)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao alterar senha"})
		return
	}

	log.Printf("Senha alterada para usuário: %v", userID)

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
	
	return string(email[0]) + "***" + string(email[atIndex-1]) + email[atIndex:]
}

func getEmailService(c *gin.Context) *services.EmailService {
	client := getDbClient(c)
	return services.NewEmailService(client.DB())
}

// GerarTokenVerificacao gera token de verificação sem enviar email
func GerarTokenVerificacao(c *gin.Context) {
	var req struct {
		Identificador string `json:"identificador" binding:"required"`
		Tipo          string `json:"tipo" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo inválido"})
		return
	}

	var idStr string
	err := client.DB().QueryRow(query).Scan(&idStr, &email, &nome)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}

	userID, _ = uuid.Parse(idStr)

	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usuário não possui email cadastrado"})
		return
	}

	// Gerar token sem enviar email
	token, err := emailSvc.SaveToken(userID, req.Tipo, "verificacao_email", email, 24*time.Hour)
	if err != nil {
		log.Printf("Erro ao gerar token de verificação: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao gerar token"})
		return
	}

	log.Printf("Token de verificação gerado para: %s", email)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token":   token,
		"email":   email,
		"nome":    nome,
		"tipo":    req.Tipo,
		"expira_em": "24 horas",
	})
}

// GerarTokenRecuperacao gera token de recuperação sem enviar email
func GerarTokenRecuperacao(c *gin.Context) {
	var req struct {
		Identificador string `json:"identificador" binding:"required"`
		Tipo          string `json:"tipo" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo inválido"})
		return
	}

	var idStr string
	err := client.DB().QueryRow(query).Scan(&idStr, &email, &nome)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}

	userID, _ = uuid.Parse(idStr)

	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usuário não possui email cadastrado"})
		return
	}

	// Gerar token sem enviar email
	token, err := emailSvc.SaveToken(userID, req.Tipo, "recuperacao_senha", email, 1*time.Hour)
	if err != nil {
		log.Printf("Erro ao gerar token de recuperação: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao gerar token"})
		return
	}

	log.Printf("Token de recuperação gerado para: %s", email)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token":   token,
		"email":   email,
		"nome":    nome,
		"tipo":    req.Tipo,
		"expira_em": "1 hora",
	})
}