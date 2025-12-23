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
	log.Printf("🌐 Ambiente: %s", os.Getenv("ENV"))
	log.Printf("🗃️  GenesisDB: Event Sourcing ativado")
	
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
			"version": "2.0.0",
			"env":     os.Getenv("ENV"),
			"architecture": "Event Sourcing com GenesisDB",
			"db_stats": gin.H{
				"open_connections": stats.OpenConnections,
				"in_use": stats.InUse,
				"idle": stats.Idle,
			},
		})
	})

	// Autenticação
	router.POST("/login", handlers.Login)
	router.POST("/academia/register", handlers.RegisterAcademia)
	router.POST("/estudante/register", handlers.RegisterEstudante)

	// ============================================
	// ROTAS PROTEGIDAS
	// ============================================
	
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		// ROTAS DE ESTUDANTES
		estudante := protected.Group("/estudante")
		estudante.Use(middleware.RequireEstudante())
		{
			estudante.POST("/inscricao-escola", handlers.InscricaoEscola)
			estudante.POST("/inscricao-universidade", handlers.InscricaoUniversidade)
			estudante.GET("/minhas-inscricoes", handlers.GetMinhasInscricoes)
			estudante.GET("/meu-historico", handlers.GetMeuHistorico)
		}

		// ROTAS DE ACADEMIAS
		academia := protected.Group("/academia")
		academia.Use(middleware.RequireAcademia())
		{
			// Commands (CQRS - Write)
			academia.POST("/notas-aluno", handlers.RegistrarNotas)
			academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
			
			// Inscrições
			academia.GET("/inscricoes-pendentes", handlers.ListarInscricoesPendentes)
			academia.PUT("/inscricao/:id/aprovar", handlers.AprovarInscricao)
			academia.PUT("/inscricao/:id/reprovar", handlers.ReprovarInscricao)
		}

		// QUERIES (CQRS - Read)
		protected.GET("/notas-estudante/:estudanteId", handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:estudanteId", handlers.GetFaltasEstudante)
		protected.GET("/historico-estudante/:estudanteId", handlers.GetHistoricoCompleto)
		
		// Event Sourcing - Auditoria
		protected.GET("/eventos-estudante/:estudanteId", handlers.GetEventosEstudante)
		protected.GET("/verificar-integridade/:estudanteId", handlers.VerificarIntegridade)
		
		// Reconstrução de projeções (admin)
		protected.POST("/admin/rebuild-projection/:name", handlers.RebuildProjection)
	}

	// ============================================
	// DOCUMENTAÇÃO
	// ============================================
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Bem-vindo ao Spuri API v2.0 - Event Sourcing",
			"version": "2.0.0",
			"arquitetura": gin.H{
				"event_sourcing": "Estado reconstruído a partir de eventos imutáveis",
				"genesisdb": "Ledger imutável com hash chain",
				"cqrs": "Separação entre comandos (escrita) e queries (leitura)",
				"projections": "Read models otimizados reconstruídos automaticamente",
			},
			"rotas": gin.H{
				"publicas": []string{
					"POST /login",
					"POST /academia/register",
					"POST /estudante/register",
					"GET /health",
				},
				"estudante": []string{
					"POST /estudante/inscricao-escola",
					"POST /estudante/inscricao-universidade",
					"GET /estudante/minhas-inscricoes",
					"GET /estudante/meu-historico",
				},
				"academia": []string{
					"POST /academia/notas-aluno (Command)",
					"POST /academia/faltas-aluno (Command)",
					"GET /academia/inscricoes-pendentes (Query)",
					"PUT /academia/inscricao/:id/aprovar (Command)",
					"PUT /academia/inscricao/:id/reprovar (Command)",
				},
				"consultas": []string{
					"GET /notas-estudante/:estudanteId",
					"GET /faltas-estudante/:estudanteId",
					"GET /historico-estudante/:estudanteId",
				},
				"event_sourcing": []string{
					"GET /eventos-estudante/:estudanteId (histórico completo)",
					"GET /verificar-integridade/:estudanteId (verificar hash chain)",
					"POST /admin/rebuild-projection/:name (reconstruir projeção)",
				},
			},
			"features": []string{
				"✅ Event Sourcing completo com GenesisDB",
				"✅ Estado reconstruído a partir de eventos",
				"✅ Imutabilidade garantida por hash chain",
				"✅ CQRS com projeções otimizadas",
				"✅ Auditoria completa de todas as operações",
				"✅ Reconstrução de projeções a qualquer momento",
				"✅ Integridade verificável do ledger",
			},
		})
	})

	return router
}