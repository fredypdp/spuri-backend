package main

import (
	"log"
	"os"
	"spuri/internal/genesisdb"
	"spuri/internal/handlers"
	"spuri/internal/middleware"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var (
	genesisClient *genesisdb.Client
	repository    *genesisdb.AggregateRepository
	projManager   *projections.Manager
)

func main() {
	// Carregar variáveis de ambiente
	if os.Getenv("ENV") != "production" {
		if err := godotenv.Load(); err != nil {
			log.Println("⚠️  Arquivo .env não encontrado")
		}
	}

	// Inicializar GenesisDB
	if err := initGenesisDB(); err != nil {
		log.Fatalf("❌ Erro ao conectar ao GenesisDB: %v", err)
	}
	defer genesisClient.Close()

	// Inicializar sistema de projeções
	if err := initProjections(); err != nil {
		log.Fatalf("❌ Erro ao inicializar projeções: %v", err)
	}

	// Configurar modo do Gin
	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Criar router
	router := setupRouter()

	// Iniciar servidor
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Spuri Event Sourcing rodando em http://localhost:%s", port)
	log.Printf("📚 Documentação: http://localhost:%s/", port)
	log.Printf("❤️  Health check: http://localhost:%s/health", port)
	log.Printf("🌍 Ambiente: %s", os.Getenv("ENV"))
	log.Printf("🗃️  GenesisDB: Event Sourcing ativado")
	log.Printf("👑 Admin FPP: admin@spuri.ao / fpp@2025")
	
	if err := router.Run("0.0.0.0:" + port); err != nil {
		log.Fatalf("❌ Erro ao iniciar servidor: %v", err)
	}
}

// initGenesisDB inicializa conexão com GenesisDB
func initGenesisDB() error {
	config := genesisdb.DefaultConfig()
	
	var err error
	genesisClient, err = genesisdb.NewClient(config)
	if err != nil {
		return err
	}

	// Criar repositório de agregados
	repository = genesisdb.NewAggregateRepository(genesisClient)

	// Verificar health
	if err := genesisClient.Health(); err != nil {
		return err
	}

	log.Println("✅ GenesisDB inicializado com Event Sourcing")
	return nil
}

// initProjections inicializa sistema de projeções
func initProjections() error {
	projManager = projections.NewManager(genesisClient)
	
	// Registrar projeções
	projManager.RegisterProjection("estudantes", projections.NewEstudanteProjection(genesisClient))
	projManager.RegisterProjection("academias", projections.NewAcademiaProjection(genesisClient))
	projManager.RegisterProjection("admins", projections.NewAdminProjection(genesisClient))
	projManager.RegisterProjection("notas", projections.NewNotasProjection(genesisClient))
	projManager.RegisterProjection("faltas", projections.NewFaltasProjection(genesisClient))
	projManager.RegisterProjection("inscricoes", projections.NewInscricoesProjection(genesisClient))

	// Iniciar processamento em background
	go projManager.StartProcessing()

	log.Println("✅ Sistema de projeções inicializado")
	return nil
}

// setupRouter configura todas as rotas
func setupRouter() *gin.Engine {
	router := gin.Default()

	// Middleware CORS
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	})

	// Injetar dependências no contexto
	router.Use(func(c *gin.Context) {
		c.Set("repository", repository)
		c.Set("projManager", projManager)
		c.Set("genesisClient", genesisClient)
		c.Next()
	})

	// ============================================
	// ROTAS PÚBLICAS
	// ============================================
	
	// Health check
	router.GET("/health", func(c *gin.Context) {
		stats := genesisClient.Stats()
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "Spuri Event Sourcing rodando!",
			"version": "2.1.0",
			"env":     os.Getenv("ENV"),
			"architecture": "Event Sourcing com GenesisDB + Sistema Admin",
			"db_stats": gin.H{
				"open_connections": stats.OpenConnections,
				"in_use": stats.InUse,
				"idle": stats.Idle,
			},
		})
	})

	// Autenticação pública
	router.POST("/login", handlers.Login)
	router.POST("/admin/login", handlers.LoginAdmin)
	router.POST("/academia/register", handlers.RegisterAcademia)
	router.POST("/estudante/register", handlers.RegisterEstudante)

	// ============================================
	// ROTAS DE ESTUDANTES (PROTEGIDAS)
	// ============================================
	
	estudante := router.Group("/estudante")
	estudante.Use(middleware.AuthMiddleware())
	estudante.Use(middleware.RequireEstudante())
	{
		estudante.POST("/inscricao-escola", handlers.InscricaoEscola)
		estudante.POST("/inscricao-universidade", handlers.InscricaoUniversidade)
		estudante.GET("/minhas-inscricoes", handlers.GetMinhasInscricoes)
		estudante.GET("/meu-historico", handlers.GetMeuHistorico)
	}

	// ============================================
	// ROTAS DE ACADEMIAS (PROTEGIDAS + VALIDAÇÕES)
	// ============================================
	
	academia := router.Group("/academia")
	academia.Use(middleware.AuthMiddleware())
	academia.Use(middleware.RequireAcademia())
	academia.Use(middleware.ValidarStatusAcademia()) // 🔥 VALIDAR SE ESTÁ ATIVA
	{
		// Commands (CQRS - Write)
		academia.POST("/notas-aluno", handlers.RegistrarNotas)
		academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
		
		// Inscrições
		academia.GET("/inscricoes-pendentes", handlers.ListarInscricoesPendentes)
		academia.PUT("/inscricao/:id/aprovar", handlers.AprovarInscricao)
		academia.PUT("/inscricao/:id/reprovar", handlers.ReprovarInscricao)
	}

	// ============================================
	// ROTAS PROTEGIDAS (ESTUDANTES, ACADEMIAS, ADMINS)
	// ============================================
	
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		// Queries (CQRS - Read)
		protected.GET("/notas-estudante/:codigo", handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:codigo", handlers.GetFaltasEstudante)
		protected.GET("/historico-estudante/:codigo", handlers.GetHistoricoCompleto)
		
		// Event Sourcing - Auditoria
		protected.GET("/eventos-estudante/:codigo", handlers.GetEventosEstudante)
		protected.GET("/verificar-integridade/:codigo", handlers.VerificarIntegridade)
		
		// Inscrições
		protected.GET("/inscricoes", handlers.ListarTodasInscricoes)
	}

	// ============================================
	// ROTAS ADMINISTRATIVAS
	// ============================================
	
	admin := router.Group("/admin")
	admin.Use(middleware.AuthMiddleware())
	admin.Use(middleware.RequireAdmin())
	{
		// Consultas (todos os admins)
		admin.GET("/estudantes", handlers.ListarTodosEstudantes)
		admin.GET("/academias", handlers.ListarTodasAcademias)
		admin.GET("/inscricoes", handlers.ListarTodasInscricoes)
		
		// Gerenciamento de Academias (gerente+)
		adminGerente := admin.Group("/")
		adminGerente.Use(middleware.RequireGerente())
		{
			adminGerente.PUT("/academia/:id/ativar", handlers.AtivarAcademia)
			adminGerente.PUT("/academia/:id/desativar", handlers.DesativarAcademia)
		}
		
		// Gerenciamento de Admins (adm+)
		adminAdm := admin.Group("/")
		adminAdm.Use(middleware.RequireAdm())
		{
			adminAdm.POST("/register", handlers.RegisterAdmin)
			adminAdm.GET("/admins", handlers.ListarTodosAdmins)
		}
		
		// Ações exclusivas FPP
		adminFPP := admin.Group("/")
		adminFPP.Use(middleware.RequireFPP())
		{
			adminFPP.PUT("/admin/:id/ativar", handlers.AtivarAdmin)
			adminFPP.PUT("/admin/:id/desativar", handlers.DesativarAdmin)
		}
		
		// Projeções (qualquer admin)
		admin.POST("/rebuild-projection/:name", handlers.RebuildProjection)
		admin.GET("/projection-status/:name", handlers.GetProjectionStatus)
		admin.GET("/projections-status", handlers.GetAllProjectionStatuses)
		admin.GET("/ledger-stats", handlers.GetLedgerStats)
		admin.GET("/verify-all-integrity", handlers.VerifyAllIntegrity)
	}

	// ============================================
	// DOCUMENTAÇÃO
	// ============================================
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Bem-vindo ao Spuri API v2.1 - Event Sourcing + Admin",
			"version": "2.1.0",
			"admin_inicial": gin.H{
				"email": "admin@spuri.ao",
				"senha": "fpp@2025",
				"role":  "fpp",
				"aviso": "⚠️  ALTERE A SENHA APÓS O PRIMEIRO LOGIN!",
			},
		})
	})

	return router
}