// ============================================================================
// ARQUIVO: internal/handlers/bootstrap_handler.go
// Endpoint especial para criar o primeiro admin FPP
// IMPORTANTE: Só funciona se não existir nenhum admin no sistema
// ============================================================================

package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// BootstrapAdminFPP cria o primeiro admin FPP
// 🔒 SEGURANÇA: Só funciona se não existir NENHUM admin no sistema
func BootstrapAdminFPP(c *gin.Context) {
	log.Println("🔵 [BOOTSTRAP] Iniciando criação de Admin FPP...")
	
	// Verificar se já existe algum admin
	adminProj := getAdminProjection(c)
	admins, err := adminProj.GetAll()
	
	if err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao verificar admins: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao verificar admins existentes",
		})
		return
	}

	// 🔒 BLOQUEIO: Se já existe admin, abortar
	if len(admins) > 0 {
		log.Printf("⚠️  [BOOTSTRAP] Sistema já possui %d admin(s)", len(admins))
		c.JSON(http.StatusForbidden, gin.H{
			"error":        "sistema já possui administradores",
			"message":      "use o endpoint /admin/register para criar novos admins",
			"total_admins": len(admins),
			"admins_existentes": func() []gin.H {
				lista := []gin.H{}
				for _, adm := range admins {
					lista = append(lista, gin.H{
						"nome":   adm.Nome,
						"email":  adm.Email,
						"role":   adm.Role,
						"status": adm.Status,
					})
				}
				return lista
			}(),
		})
		return
	}

	// Dados do admin
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
		
		log.Println("ℹ️  [BOOTSTRAP] Usando valores padrão")
	}

	// Validações
	if req.Nome == "" || req.Email == "" || req.Senha == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "nome, email e senha são obrigatórios",
		})
		return
	}

	log.Printf("📋 [BOOTSTRAP] Criando admin: %s (%s)", req.Nome, req.Email)

	// Hash da senha usando bcrypt do Go (CORRETO!)
	log.Println("🔐 [BOOTSTRAP] Gerando hash bcrypt...")
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(req.Senha), 
		bcrypt.DefaultCost, // Cost 10
	)
	if err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao gerar hash: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao processar senha",
		})
		return
	}
	
	log.Printf("✅ [BOOTSTRAP] Hash gerado: %s...", string(hashedPassword[:30]))

	// Criar agregado Admin
	repository := getRepository(c)
	admin := aggregates.NewAdmin()

	log.Println("🏗️  [BOOTSTRAP] Criando agregado Admin...")
	if err := admin.Criar(
		req.Nome,
		req.Email,
		string(hashedPassword),
		"fpp", // Role FPP (máxima permissão)
		nil,   // Criado por ninguém (bootstrap)
	); err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao criar agregado: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Salvar eventos
	log.Println("💾 [BOOTSTRAP] Salvando eventos no GenesisDB...")
	if err := repository.Save(admin); err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao criar admin FPP",
		})
		return
	}

	log.Println("✅ [BOOTSTRAP] Admin FPP criado com sucesso!")

	// Verificar hash imediatamente
	if bcrypt.CompareHashAndPassword(hashedPassword, []byte(req.Senha)) == nil {
		log.Println("✅ [BOOTSTRAP] Hash validado: Login funcionará!")
	} else {
		log.Println("⚠️  [BOOTSTRAP] Aviso: Hash pode não validar corretamente")
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
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
			"1. Aguarde 2-3 segundos para a projeção processar",
			"2. Faça login em POST /admin/login",
			"3. Altere a senha após o primeiro acesso",
			"4. Crie outros admins conforme necessário",
		},
		"test_login": gin.H{
			"url":    "/admin/login",
			"method": "POST",
			"body": gin.H{
				"email": req.Email,
				"senha": req.Senha,
			},
		},
	})
}