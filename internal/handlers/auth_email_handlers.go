// ============================================================================
// ARQUIVO: internal/handlers/auth_email_handlers.go
// 🔥 CORRIGIDO: Conversão de tipos UUID + Prepared Statements
// ============================================================================

package handlers

import (
	"net/http"
	"spuri/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// VerificarEmail verifica email usando token
// GET /verificar-email/:token
func VerificarEmail(c *gin.Context) {
	token := c.Param("token")
	
	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "verificacao_email")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Atualizar email_verificado
	client := getDbClient(c)
	
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

	query := "UPDATE " + table + " SET email_verificado = TRUE WHERE id = $1"
	_, err = client.DB().Exec(query, tokenInfo.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Email verificado com sucesso!",
		"email":   tokenInfo.Email,
	})
}

// SolicitarRecuperacaoSenha envia email de recuperação
// POST /recuperar-senha/solicitar
func SolicitarRecuperacaoSenha(c *gin.Context) {
	var req struct {
		Identificador string `json:"identificador" binding:"required"` // email, codigo
		Tipo          string `json:"tipo" binding:"required"`          // estudante, academia, admin
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	client := getDbClient(c)
	emailSvc := getEmailService(c)

	// 🔥 CORRIGIDO: Usar uuid.UUID em vez de string
	var userID uuid.UUID
	var email, nome string
	var query string

	switch req.Tipo {
	case "estudante":
		query = `SELECT id, email, nome FROM projection_estudantes 
		         WHERE codigo_estudante = $1 OR email = $1`
	case "academia":
		query = `SELECT id, email, nome FROM projection_academias 
		         WHERE codigo_academia = $1 OR email = $1`
	case "admin":
		query = `SELECT id, email, nome FROM projection_admins WHERE email = $1`
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo inválido"})
		return
	}

	// ✅ USAR Get ao invés de QueryRow
	type result struct {
		ID    uuid.UUID `db:"id"`
		Email string    `db:"email"`
		Nome  string    `db:"nome"`
	}
	
	var res result
	err := client.DB().Get(&res, query, req.Identificador)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}
	
	userID = res.ID
	email = res.Email
	nome = res.Nome

	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "usuário não possui email cadastrado"})
		return
	}

	// Enviar email - agora userID é uuid.UUID ✅
	if err := emailSvc.SendPasswordResetEmail(userID, req.Tipo, email, nome); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao enviar email"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Email de recuperação enviado! Verifique sua caixa de entrada.",
		"email":   maskEmail(email),
	})
}

// ResetarSenha redefine senha usando token
// POST /recuperar-senha/:token
func ResetarSenha(c *gin.Context) {
	token := c.Param("token")

	emailSvc := getEmailService(c)
	tokenInfo, err := emailSvc.VerifyToken(token, "recuperacao_senha")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	client := getDbClient(c)

	// 🔥 CORRIGIDO: Buscar código/role baseado no tipo de usuário
	var codigo string
	var table string
	var query string

	switch tokenInfo.UserType {
	case "estudante":
		query = `SELECT codigo_estudante FROM projection_estudantes WHERE id = $1`
		table = "projection_estudantes"
	case "academia":
		query = `SELECT codigo_academia FROM projection_academias WHERE id = $1`
		table = "projection_academias"
	case "admin":
		query = `SELECT role FROM projection_admins WHERE id = $1`
		table = "projection_admins"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo de usuário inválido"})
		return
	}

	// ✅ USAR Get ao invés de QueryRow
	err = client.DB().Get(&codigo, query, tokenInfo.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}

	// Gerar senha padrão
	defaultPassword := services.GetDefaultPassword(tokenInfo.UserType, codigo)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	// Atualizar senha
	updateQuery := "UPDATE " + table + " SET senha_hash = $1 WHERE id = $2"
	_, err = client.DB().Exec(updateQuery, string(hashedPassword), tokenInfo.UserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao resetar senha"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Senha resetada com sucesso!",
		"senha_padrao":    defaultPassword,
		"email":           tokenInfo.Email,
		"proximos_passos": "Faça login com a senha padrão e altere para uma senha segura.",
	})
}

// AlterarSenha altera senha do usuário logado
// PUT /alterar-senha
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

	// Verificar senha atual
	var senhaHash string
	query := "SELECT senha_hash FROM " + table + " WHERE id = $1"
	
	// ✅ USAR Get ao invés de QueryRow
	err := client.DB().Get(&senhaHash, query, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "usuário não encontrado"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.SenhaAtual)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "senha atual incorreta"})
		return
	}

	// Hash nova senha
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	// Atualizar
	updateQuery := "UPDATE " + table + " SET senha_hash = $1 WHERE id = $2"
	_, err = client.DB().Exec(updateQuery, string(hashedPassword), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao alterar senha"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Senha alterada com sucesso!",
	})
}

// Helper: mascara email
func maskEmail(email string) string {
	if len(email) < 5 {
		return "***@***"
	}
	
	// Encontrar posição do @
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
	
	// Mostrar primeiro e último char antes do @
	if atIndex <= 2 {
		return "***@" + email[atIndex+1:]
	}
	
	masked := string(email[0]) + "***" + string(email[atIndex-1]) + email[atIndex:]
	return masked
}

// Helper: obtém serviço de email
func getEmailService(c *gin.Context) *services.EmailService {
	client := getDbClient(c)
	return services.NewEmailService(client.DB())
}