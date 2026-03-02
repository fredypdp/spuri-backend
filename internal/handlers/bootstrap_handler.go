package handlers

import (
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// BootstrapAdminFPP cria o primeiro admin FPP.
// 🔒 SEGURANÇA: Só funciona se não existir NENHUM admin no sistema.
// CORRIGIDO: sem fallback de credenciais hardcoded — retorna HTTP 400 se body inválido.
func BootstrapAdminFPP(c *gin.Context) {
	log.Println("🔵 [BOOTSTRAP] Iniciando criação de Admin FPP...")

	// Verificar se já existe algum admin
	adminProj := getAdminProjection(c)
	log.Println("🔍 [BOOTSTRAP] Buscando admins existentes...")
	admins, err := adminProj.GetAll()

	if err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao verificar admins: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao verificar admins existentes",
		})
		return
	}

	log.Printf("📊 [BOOTSTRAP] Total de admins encontrados: %d", len(admins))

	// 🔒 BLOQUEIO: Se já existe admin, abortar
	if len(admins) > 0 {
		log.Printf("⚠️ [BOOTSTRAP] Sistema já possui %d admin(s)", len(admins))
		c.JSON(http.StatusForbidden, gin.H{
			"error":        "sistema já possui administradores",
			"message":      "use o endpoint /admin/register para criar novos admins",
			"total_admins": len(admins),
		})
		return
	}

	// Dados do admin — CORRIGIDO: sem fallback hardcoded.
	// Body é obrigatório. Se inválido, retorna 400.
	var req struct {
		Nome  string `json:"nome"  binding:"required"`
		Email string `json:"email" binding:"required"`
		Senha string `json:"senha" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [BOOTSTRAP] Body inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "nome, email e senha são obrigatórios",
			"example": gin.H{"nome": "Admin FPP", "email": "admin@dominio.com", "senha": "SenhaForte@123"},
		})
		return
	}

	log.Printf("📋 [BOOTSTRAP] Criando admin FPP: %s (%s)", req.Nome, req.Email)

	// Verificar se email já existe
	existing, _ := adminProj.GetByEmail(req.Email)
	if existing != nil {
		log.Printf("❌ [BOOTSTRAP] Email %s já cadastrado", req.Email)
		c.JSON(http.StatusConflict, gin.H{"error": "email já cadastrado"})
		return
	}

	// Hash da senha
	log.Println("🔐 [BOOTSTRAP] Gerando hash bcrypt...")
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(req.Senha),
		bcrypt.DefaultCost,
	)
	if err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao gerar hash: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao processar senha",
		})
		return
	}

	// Criar agregado Admin
	repository := getRepository(c)
	newAdmin := aggregates.NewAdmin()

	log.Println("🗂️ [BOOTSTRAP] Criando agregado Admin...")

	if err := newAdmin.Criar(
		req.Nome,
		req.Email,
		string(hashedPassword),
		"fpp", // Role FPP (máxima permissão)
		nil,   // Criado pelo sistema (bootstrap)
	); err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao criar agregado: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	log.Printf("✅ [BOOTSTRAP] Agregado criado - ID: %s", newAdmin.ID)

	// Salvar eventos — bootstrap público, sem JWT
	log.Println("💾 [BOOTSTRAP] Salvando eventos no banco de dados...")

	audit := db.AuditContext{
		UserID:   "bootstrap",
		UserType: "sistema",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(newAdmin, audit); err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao criar admin FPP",
		})
		return
	}

	log.Println("✅ [BOOTSTRAP] Admin FPP criado com sucesso!")

	// Aguardar processamento da projeção
	log.Println("⏳ [BOOTSTRAP] Aguardando processamento da projeção...")
	time.Sleep(2 * time.Second)

	log.Printf("🎉 [BOOTSTRAP] Processo completo! Admin ID: %s, Email: %s", newAdmin.ID, req.Email)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "✅ Admin FPP criado com sucesso!",
		"data": gin.H{
			"id":    newAdmin.ID,
			"nome":  newAdmin.Nome,
			"email": req.Email,
			"role":  "fpp",
		},
		"next_steps": []string{
			"1. Projeção processada - pode fazer login agora",
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