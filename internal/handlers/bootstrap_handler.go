// ============================================================================
// ARQUIVO: internal/handlers/bootstrap_handler.go
// Endpoint especial para criar o primeiro admin FPP
// IMPORTANTE: Só funciona se não existir nenhum admin no sistema
// ============================================================================

package handlers

import (
	"net/http"
	"spuri/internal/domain/aggregates"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// BootstrapAdminFPP cria o primeiro admin FPP
// 🔒 SEGURANÇA: Só funciona se não existir NENHUM admin no sistema
func BootstrapAdminFPP(c *gin.Context) {
	// Verificar se já existe algum admin
	adminProj := getAdminProjection(c)
	admins, err := adminProj.GetAll()
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao verificar admins existentes",
		})
		return
	}

	// 🔒 BLOQUEIO: Se já existe admin, abortar
	if len(admins) > 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "sistema já possui administradores",
			"message": "use o endpoint /admin/register para criar novos admins",
			"total_admins": len(admins),
		})
		return
	}

	// Dados fixos do primeiro admin FPP
	var req struct {
		Nome  string `json:"nome"`
		Email string `json:"email"`
		Senha string `json:"senha"`
	}

	// Permitir personalização ou usar valores padrão
	if err := c.ShouldBindJSON(&req); err != nil {
		// Usar valores padrão se não for fornecido
		req.Nome = "Admin FPP"
		req.Email = "admin@spuri.ao"
		req.Senha = "fpp@2025"
	}

	// Validações
	if req.Nome == "" || req.Email == "" || req.Senha == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "nome, email e senha são obrigatórios",
		})
		return
	}

	// Hash da senha usando bcrypt do Go
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(req.Senha), 
		bcrypt.DefaultCost, // Cost 10 - igual ao SQL
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao processar senha",
		})
		return
	}

	// Criar agregado Admin
	repository := getRepository(c)
	admin := aggregates.NewAdmin()

	if err := admin.Criar(
		req.Nome,
		req.Email,
		string(hashedPassword),
		"fpp", // Role FPP (máxima permissão)
		nil,   // Criado por ninguém (bootstrap)
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Salvar eventos
	if err := repository.Save(admin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao criar admin FPP",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "✅ Admin FPP criado com sucesso!",
		"data": gin.H{
			"id":    admin.ID,
			"nome":  admin.Nome,
			"email": req.Email,
			"role":  "fpp",
		},
		"credentials": gin.H{
			"email": req.Email,
			"senha": req.Senha,
		},
		"next_steps": []string{
			"1. Faça login em POST /admin/login",
			"2. Altere a senha após o primeiro acesso",
			"3. Crie outros admins conforme necessário",
		},
	})
}