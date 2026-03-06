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

// BootstrapAdminFPP cria o primeiro admin FPP do sistema.
//
// 🔒 SEGURANÇA:
//   - Só funciona se não existir NENHUM admin no sistema.
//   - Após o primeiro uso a rota fica bloqueada (retorna 403).
//   - Advisory lock PostgreSQL serializa requisições concorrentes.
//   - A senha NÃO é retornada na resposta.
//
// FIX E4-LN-02: o advisory lock era mantido retido DURANTE o polling síncrono
// (até 10 segundos), o que bloqueava concorrentes e mantinha uma conexão de
// banco ocupada enquanto a goroutine HTTP ficava suspensa.
//
// Correção: o advisory lock é adquirido, a verificação + gravação de eventos
// é feita com o lock ativo, e então o lock é LIBERADO imediatamente após o
// SaveWithAudit — antes de qualquer polling. O polling ocorre já sem o lock,
// tornando o período de espera pela projeção um problema puramente de
// responsividade da resposta HTTP, sem impacto em outras requisições.
func BootstrapAdminFPP(c *gin.Context) {
	log.Println("🔵 [BOOTSTRAP] Iniciando criação de Admin FPP...")

	// Body obrigatório — sem fallback hardcoded.
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

	client := getDbClient(c)
	dbConn := client.DB()

	// ── FASE 1: Adquirir advisory lock e executar operações críticas ───────
	//
	// FIX E4-LN-02: o lock protege APENAS a fase de verificação + escrita no
	// ledger. O polling não está mais dentro da região crítica.
	// Garante que apenas uma goroutine/instância executa o bootstrap por vez.
	if _, err := dbConn.Exec(`SELECT pg_advisory_lock(hashtext('bootstrap_admin_fpp'))`); err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao adquirir advisory lock: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro interno ao iniciar bootstrap"})
		return
	}

	// FIX E4-LN-02: liberação do lock ocorre logo após SaveWithAudit,
	// não ao final da função. O defer abaixo é safety-net para casos de
	// retorno antecipado por erro antes da liberação explícita.
	lockReleased := false
	releaseAdvisoryLock := func() {
		if !lockReleased {
			lockReleased = true
			if _, err := dbConn.Exec(`SELECT pg_advisory_unlock(hashtext('bootstrap_admin_fpp'))`); err != nil {
				log.Printf("[WARN] [BOOTSTRAP] Erro ao liberar advisory lock: %v", err)
			}
		}
	}
	defer releaseAdvisoryLock()

	// Verificação pós-lock — garante que não houve bootstrap paralelo
	adminProj := getAdminProjection(c)
	admins, err := adminProj.GetAll()
	if err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao verificar admins: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar admins existentes"})
		return
	}

	if len(admins) > 0 {
		log.Printf("⚠️ [BOOTSTRAP] Sistema já possui %d admin(s)", len(admins))
		c.JSON(http.StatusForbidden, gin.H{
			"error":        "sistema já possui administradores",
			"message":      "use o endpoint /admin/register para criar novos admins",
			"total_admins": len(admins),
		})
		return
	}

	// Verificar se email já existe (redundante mas defensivo)
	existing, _ := adminProj.GetByEmail(req.Email)
	if existing != nil {
		log.Printf("❌ [BOOTSTRAP] Email %s já cadastrado", req.Email)
		c.JSON(http.StatusConflict, gin.H{"error": "email já cadastrado"})
		return
	}

	log.Println("🔐 [BOOTSTRAP] Gerando hash bcrypt...")
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao gerar hash: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	repository := getRepository(c)
	newAdmin := aggregates.NewAdmin()

	log.Println("🗂️ [BOOTSTRAP] Criando agregado Admin...")
	if err := newAdmin.Criar(req.Nome, req.Email, string(hashedPassword), "fpp", nil); err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao criar agregado: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("✅ [BOOTSTRAP] Agregado criado — ID: %s", newAdmin.ID)

	audit := db.AuditContext{
		UserID:   "bootstrap",
		UserType: "sistema",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(newAdmin, audit); err != nil {
		log.Printf("❌ [BOOTSTRAP] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao criar admin FPP"})
		return
	}

	// ── FASE 2: Liberar o lock ANTES do polling ────────────────────────────
	//
	// FIX E4-LN-02: libera o advisory lock imediatamente após a escrita no
	// ledger. A partir daqui não há mais região crítica — outras requisições
	// (que receberão 403 porque já existe admin) podem avançar sem esperar
	// pelos até 10s do polling abaixo.
	releaseAdvisoryLock()

	// ── FASE 3: Polling fora do lock ───────────────────────────────────────
	//
	// Aguarda até 10s pela projeção processar o evento.
	// O lock já foi liberado — apenas a goroutine HTTP está suspensa aqui.
	log.Println("✅ [BOOTSTRAP] Eventos gravados no ledger. Aguardando projeção (lock já liberado)...")

	adminID := newAdmin.ID
	const maxAttempts = 50
	const interval = 200 * time.Millisecond

	var adminProcessado bool
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		time.Sleep(interval)
		dto, err := adminProj.GetByID(adminID)
		if err == nil && dto != nil {
			adminProcessado = true
			log.Printf("✅ [BOOTSTRAP] Projeção processada na tentativa %d", attempt)
			break
		}
		log.Printf("⏳ [BOOTSTRAP] Aguardando projeção... tentativa %d/%d", attempt, maxAttempts)
	}

	if !adminProcessado {
		log.Printf("⚠️ [BOOTSTRAP] Projeção ainda não processou após %d tentativas", maxAttempts)
		c.JSON(http.StatusCreated, gin.H{
			"success": true,
			"message": "✅ Admin FPP criado com sucesso! A projeção ainda está sendo processada.",
			"data": gin.H{
				"id":    newAdmin.ID,
				"nome":  newAdmin.Nome,
				"email": req.Email,
				"role":  "fpp",
			},
			"aviso":      "aguarde alguns segundos antes de fazer login",
			"next_steps": []string{"1. Aguarde 5-10 segundos", "2. Faça login em POST /admin/login", "3. Altere a senha após o primeiro acesso"},
		})
		return
	}

	log.Printf("🎉 [BOOTSTRAP] Processo completo! Admin ID: %s, Email: %s", newAdmin.ID, req.Email)

	// A senha NÃO é incluída na resposta.
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
			"1. Faça login em POST /admin/login",
			"2. Altere a senha após o primeiro acesso",
			"3. Crie outros admins conforme necessário",
		},
	})
}