package main

import (
	"log"
	"os"
	"spuri/internal/db"
	"spuri/internal/handlers"
	"spuri/internal/middleware"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var (
	dbClient *db.Client
	repository    *db.AggregateRepository
	projManager   *projections.Manager
)

func main() {
	// Carregar variáveis de ambiente
	if os.Getenv("ENV") != "production" {
		if err := godotenv.Load(); err != nil {
			log.Println("⚠️  Arquivo .env não encontrado")
		}
	}

	// Inicializar db
	if err := initDB(); err != nil {
		log.Fatalf("❌ Erro ao conectar ao banco de dados: %v", err)
	}
	defer dbClient.Close()

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
	log.Printf("🗃️  Banco de dados: Event Sourcing ativado")
	log.Printf("🔑 Admin FPP: admin@spuri.ao / fpp@2025")
	
	if err := router.Run("0.0.0.0:" + port); err != nil {
		log.Fatalf("❌ Erro ao iniciar servidor: %v", err)
	}
}

// initDB inicializa conexão com banco de dados
func initDB() error {
	config := db.DefaultConfig()
	
	var err error
	dbClient, err = db.NewClient(config)
	if err != nil {
		return err
	}

	// Criar repositório de agregados
	repository = db.NewAggregateRepository(dbClient)

	// Verificar health
	if err := dbClient.Health(); err != nil {
		return err
	}

	log.Println("✅ Banco de dados inicializado com Event Sourcing")
	return nil
}

// initProjections inicializa sistema de projeções
func initProjections() error {
	projManager = projections.NewManager(dbClient)
	
	// Registrar projeções
	projManager.RegisterProjection("estudantes", projections.NewEstudanteProjection(dbClient))
	projManager.RegisterProjection("academias", projections.NewAcademiaProjection(dbClient))
	projManager.RegisterProjection("admins", projections.NewAdminProjection(dbClient))
	projManager.RegisterProjection("notas", projections.NewNotasProjection(dbClient))
	projManager.RegisterProjection("faltas", projections.NewFaltasProjection(dbClient))
	projManager.RegisterProjection("inscricoes", projections.NewInscricoesProjection(dbClient))

	// Iniciar processamento em background
	go projManager.StartProcessing()

	log.Println("✅ Sistema de projeções inicializado")
	return nil
}

// setupRouter configura todas as rotas
func setupRouter() *gin.Engine {
	router := gin.Default()

	// Middleware CORS - Origins Específicos
	router.Use(func(c *gin.Context) {
		// Origins permitidos
		allowedOrigins := []string{
			"https://spuripainel.vercel.app",
			"http://localhost:3000",
		}
		
		origin := c.Request.Header.Get("Origin")
		
		// Verificar se o origin está permitido
		for _, allowedOrigin := range allowedOrigins {
			if origin == allowedOrigin {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
		
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400") // 24 horas
		
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
		c.Set("dbClient", dbClient)
		c.Next()
	})

	// ============================================
	// ROTAS PÚBLICAS
	// ============================================
	
	// Health check
	router.GET("/health", func(c *gin.Context) {
		stats := dbClient.Stats()
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "Spuri Event Sourcing rodando!",
			"version": "2.4.0",
			"env":     os.Getenv("ENV"),
			"db_stats": gin.H{
				"open_connections": stats.OpenConnections,
				"in_use": stats.InUse,
				"idle": stats.Idle,
			},
		})
	})

	// 🔥 BOOTSTRAP: Criar primeiro admin FPP
	router.POST("/bootstrap/admin-fpp", handlers.BootstrapAdminFPP)

	// Autenticação pública
	router.POST("/login", handlers.Login)
	router.POST("/admin/login", handlers.LoginAdmin)
	router.POST("/academia/register", handlers.RegisterAcademia)
	router.POST("/estudante/register", handlers.RegisterEstudante)

	// ============================================
	// ROTAS PROTEGIDAS - QUALQUER USUÁRIO LOGADO
	// ============================================
	
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		// Perfil do usuário logado
		protected.GET("/meu-perfil", handlers.GetMeuPerfil)
		
		// Lista academias (todos podem ver)
		protected.GET("/academias", handlers.ListarTodasAcademias)
		
		// 🔥 ROTAS UNIFICADAS DE INSCRIÇÕES
		// Admin: todas | Academia: só suas | Estudante: só suas
		protected.GET("/inscricoes", handlers.ListarInscricoes)
		protected.GET("/inscricoes-pendentes", handlers.ListarInscricoesPendentes)
		
		// Consultas de notas/faltas/histórico
		protected.GET("/notas-estudante/:codigo", handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:codigo", handlers.GetFaltasEstudante)
		protected.GET("/historico-estudante/:codigo", handlers.GetHistoricoCompleto)
		
		// Event Sourcing - Auditoria
		protected.GET("/eventos-estudante/:codigo", handlers.GetEventosEstudante)
		protected.GET("/verificar-integridade/:codigo", handlers.VerificarIntegridade)
		
		// Consultas públicas (academia e admin)
		protected.GET("/consultar-estudante/:codigo", handlers.GetEstudantePorCodigo)
		protected.GET("/consultar-academia/:codigo", handlers.GetAcademiaPorCodigo)
		
		// Lista estudantes (academia: seus | admin: todos)
		protected.GET("/estudantes", handlers.ListarEstudantes)
	}

	// ============================================
	// ROTAS DE ESTUDANTES
	// ============================================
	
	estudante := router.Group("/estudante")
	estudante.Use(middleware.AuthMiddleware())
	estudante.Use(middleware.RequireEstudante())
	{
		// Inscrições
		estudante.GET("/minhas-inscricoes", handlers.GetMinhasInscricoes)
		estudante.GET("/meu-historico", handlers.GetMeuHistorico)
		estudante.PUT("/status-escolar", handlers.AtualizarStatusEscolar)
		estudante.PUT("/status-superior", handlers.AtualizarStatusSuperior)
		
		estudante.POST("/inscricao-escola", handlers.InscricaoEscola)
		estudante.POST("/inscricao-universidade", handlers.InscricaoUniversidade)
		estudante.GET("/inscricoes-aprovadas", handlers.ListarInscricoesAprovadas)
		estudante.POST("/vincular-academia", handlers.VincularAcademia)
	}

	// ============================================
	// ROTAS DE ACADEMIAS
	// ============================================
	
	academia := router.Group("/academia")
	academia.Use(middleware.AuthMiddleware())
	academia.Use(middleware.RequireAcademia())
	academia.Use(middleware.ValidarStatusAcademia())
	{
		// Commands (CQRS - Write)
		academia.POST("/notas-aluno", handlers.RegistrarNotas)
		academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
		
		// Inscrições - Aprovação/Reprovação
		academia.PUT("/inscricao/:id/aprovar", handlers.AprovarInscricao)
		academia.PUT("/inscricao/:id/reprovar", handlers.ReprovarInscricao)
		
		// Consultas específicas academia
		academia.GET("/consultar-estudante/:codigo", handlers.GetEstudantePorCodigo)
		academia.GET("/consultar-academia/:codigo", handlers.GetAcademiaPorCodigo)
	}

	// ============================================
	// ROTAS ADMINISTRATIVAS
	// ============================================
	
	admin := router.Group("/admin")
	admin.Use(middleware.AuthMiddleware())
	admin.Use(middleware.RequireAdmin())
	{
		// Todos os registros
		admin.GET("/todos-registros", handlers.ListarTodosRegistros)
		admin.GET("/registros/estudante/:codigo", handlers.ListarRegistrosPorEstudante)
		admin.GET("/registros/academia/:codigo", handlers.ListarRegistrosPorAcademia)
		
		// Consultas específicas admin
		admin.GET("/consultar-admin/:email", handlers.GetAdminPorEmail)
		admin.GET("/buscar-usuario", handlers.BuscarUsuario)
		
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
			"message": "Bem-vindo ao Spuri API v2.4",
			"version": "2.4.0",
			"features": []string{
				"Event Sourcing com NeonDB",
				"Sistema de Projeções CQRS",
				"Autenticação JWT",
				"Rotas Unificadas de Inscrições",
				"Controle de Acesso por Tipo de Usuário",
				"Auditoria Completa via Ledger",
			},
			"endpoints": gin.H{
				"inscricoes": gin.H{
					"admin":     "Retorna TODAS as inscrições",
					"academia":  "Retorna apenas inscrições da própria academia",
					"estudante": "Retorna apenas inscrições do próprio estudante",
				},
				"inscricoes-pendentes": gin.H{
					"admin":     "Retorna TODAS as inscrições pendentes",
					"academia":  "Retorna apenas inscrições pendentes da própria academia",
					"estudante": "Retorna apenas inscrições pendentes do próprio estudante",
				},
			},
		})
	})

	return router
}