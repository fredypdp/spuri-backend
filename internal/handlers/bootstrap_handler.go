// ============================================================================
// ARQUIVO: internal/handlers/bootstrap_handler.go
// Endpoint especial para criar o primeiro admin FPP
// IMPORTANTE: SÓ funciona se não existir nenhum admin no sistema
// ============================================================================

package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// BootstrapAdminFPP cria o primeiro admin FPP
// 🔒 SEGURANÇA: Só funciona se não existir NENHUM admin no sistema
func BootstrapAdminFPP(c *gin.Context) {
	log.Println("🔵 [BOOTSTRAP] Iniciando criação de Admin FPP...")
	
	// Verificar se já existe algum admin
	adminProj := getAdminProjection(c)
	log.Println("🔍 [BOOTSTRAP-DEBUG] Buscando admins existentes...")
	admins, err := adminProj.GetAll()
	
	if err != nil {
		log.Printf("❌ [BOOTSTRAP-DEBUG] Erro ao verificar admins: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao verificar admins existentes",
		})
		return
	}

	log.Printf("📊 [BOOTSTRAP-DEBUG] Total de admins encontrados: %d", len(admins))

	// 🔒 BLOQUEIO: Se já existe admin, abortar
	if len(admins) > 0 {
		log.Printf("⚠️ [BOOTSTRAP] Sistema já possui %d admin(s)", len(admins))
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
		req.Email = "fredrodrigues795@gmail.com"
		req.Senha = "gloriasaobrasil"
		
		log.Println("ℹ️ [BOOTSTRAP-DEBUG] Usando valores padrão")
	}

	log.Printf("📋 [BOOTSTRAP-DEBUG] Dados recebidos - Nome: %s, Email: %s", req.Nome, req.Email)

	// Validações
	if req.Nome == "" || req.Email == "" || req.Senha == "" {
		log.Println("❌ [BOOTSTRAP-DEBUG] Validação falhou: campos obrigatórios vazios")
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "nome, email e senha são obrigatórios",
		})
		return
	}

	log.Printf("📋 [BOOTSTRAP] Criando admin: %s (%s)", req.Nome, req.Email)

	// Verificar se email já existe
	log.Printf("🔍 [BOOTSTRAP-DEBUG] Verificando se email %s já existe...", req.Email)
	existing, _ := adminProj.GetByEmail(req.Email)
	if existing != nil {
		log.Printf("❌ [BOOTSTRAP-DEBUG] Email %s já cadastrado", req.Email)
		c.JSON(http.StatusConflict, gin.H{"error": "email já cadastrado"})
		return
	}

	// Hash da senha - MESMO PADRÃO DE RegisterAdmin
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
	
	log.Printf("✅ [BOOTSTRAP-DEBUG] Hash gerado com sucesso (primeiros 30 chars): %s...", string(hashedPassword[:30]))

	// Criar agregado Admin - MESMO PADRÃO DE RegisterAdmin
	repository := getRepository(c)
	newAdmin := aggregates.NewAdmin()

	log.Println("🗂️ [BOOTSTRAP] Criando agregado Admin...")
	log.Printf("🔍 [BOOTSTRAP-DEBUG] Parâmetros agregado - Nome: %s, Email: %s, Role: fpp", req.Nome, req.Email)
	
	if err := newAdmin.Criar(
		req.Nome,
		req.Email,
		string(hashedPassword),
		"fpp", // Role FPP (máxima permissão)
		nil,   // Criado por ninguém (bootstrap)
	); err != nil {
		log.Printf("❌ [BOOTSTRAP-DEBUG] Erro ao criar agregado: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	log.Printf("✅ [BOOTSTRAP-DEBUG] Agregado criado - ID: %s", newAdmin.ID)

	// Salvar eventos - MESMO PADRÃO DE RegisterAdmin
	log.Println("💾 [BOOTSTRAP] Salvando eventos no Banco de dados...")
	log.Printf("🔍 [BOOTSTRAP-DEBUG] Total de eventos não confirmados: %d", len(newAdmin.UncommittedEvents))
	
	if err := repository.Save(newAdmin); err != nil {
		log.Printf("❌ [BOOTSTRAP-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "erro ao criar admin FPP",
		})
		return
	}

	log.Println("✅ [BOOTSTRAP] Admin FPP criado com sucesso!")

	// Verificar hash imediatamente
	log.Println("🔐 [BOOTSTRAP-DEBUG] Validando hash gerado...")
	if bcrypt.CompareHashAndPassword(hashedPassword, []byte(req.Senha)) == nil {
		log.Println("✅ [BOOTSTRAP-DEBUG] Hash validado: Login funcionará!")
	} else {
		log.Println("⚠️ [BOOTSTRAP-DEBUG] Aviso: Hash pode não validar corretamente")
	}

	// Aguardar um pouco para garantir processamento da projeção
	log.Println("⏳ [BOOTSTRAP] Aguardando processamento da projeção...")
	time.Sleep(2 * time.Second)

	log.Printf("🎉 [BOOTSTRAP-DEBUG] Processo completo! Admin ID: %s, Email: %s", newAdmin.ID, req.Email)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "✅ Admin FPP criado com sucesso!",
		"data": gin.H{
			"id":    newAdmin.ID,
			"nome":  newAdmin.Nome,
			"email": req.Email,
			"role":  "fpp",
		},
		"credentials": gin.H{
			"email": req.Email,
			"senha": req.Senha,
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