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
		c.Set("genesisClient", genesisClient) // ← ADICIONADO: Cliente GenesisDB compartilhado
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
			// Commands (CQRS - Write) - 🔥 USAM codigo_estudante no body
			academia.POST("/notas-aluno", handlers.RegistrarNotas)
			academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
			
			// Inscrições
			academia.GET("/inscricoes-pendentes", handlers.ListarInscricoesPendentes)
			academia.PUT("/inscricao/:id/aprovar", handlers.AprovarInscricao)
			academia.PUT("/inscricao/:id/reprovar", handlers.ReprovarInscricao)
		}

		// QUERIES (CQRS - Read) - 🔥 USAM codigo_estudante na rota
		protected.GET("/notas-estudante/:codigo", handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:codigo", handlers.GetFaltasEstudante)
		protected.GET("/historico-estudante/:codigo", handlers.GetHistoricoCompleto)
		
		// 🔥 NOVO: Listar todas as inscrições (com paginação e filtro)
		protected.GET("/inscricoes", handlers.ListarTodasInscricoes)
		
		// Event Sourcing - Auditoria - 🔥 USAM codigo_estudante na rota
		protected.GET("/eventos-estudante/:codigo", handlers.GetEventosEstudante)
		protected.GET("/verificar-integridade/:codigo", handlers.VerificarIntegridade)
		
		// Reconstrução de projeções (admin)
		protected.POST("/admin/rebuild-projection/:name", handlers.RebuildProjection)
	}

	// ============================================
	// DOCUMENTAÇÃO - ATUALIZADA
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
			"mudancas_v2": gin.H{
				"codigo_estudante": "Estudantes agora possuem código único (AAA1234)",
				"codigo_academia": "Academias identificadas por código (BGO20251234)",
				"referencias": "Todas as referências usam códigos em vez de UUIDs",
				"login": "Login usa codigo_estudante para estudantes, codigo_academia para academias",
			},
			"rotas": gin.H{
				"publicas": []string{
					"POST /login (usa 'codigo_estudante' ou 'codigo_academia' em 'usuario')",
					"POST /academia/register (retorna 'codigo_academia')",
					"POST /estudante/register (retorna 'codigo_estudante')",
					"GET /health",
				},
				"estudante": []string{
					"POST /estudante/inscricao-escola (body: codigo_academia)",
					"POST /estudante/inscricao-universidade (body: codigo_academia)",
					"GET /estudante/minhas-inscricoes",
					"GET /estudante/meu-historico",
				},
				"academia": []string{
					"POST /academia/notas-aluno (body: codigo_estudante)",
					"POST /academia/faltas-aluno (body: codigo_estudante)",
					"GET /academia/inscricoes-pendentes (Query)",
					"PUT /academia/inscricao/:id/aprovar (Command)",
					"PUT /academia/inscricao/:id/reprovar (Command)",
				},
				"consultas": []string{
					"GET /notas-estudante/:codigo (usa codigo_estudante)",
					"GET /faltas-estudante/:codigo (usa codigo_estudante)",
					"GET /historico-estudante/:codigo (usa codigo_estudante)",
				},
				"event_sourcing": []string{
					"GET /eventos-estudante/:codigo (histórico completo)",
					"GET /verificar-integridade/:codigo (verificar hash chain)",
					"POST /admin/rebuild-projection/:name (reconstruir projeção)",
				},
			},
			"exemplos": gin.H{
				"registro_estudante": gin.H{
					"request": map[string]interface{}{
						"nome": "João Silva",
						"senha": "senha123",
						"bilhete_identidade_responsavel": "BI123456",
						"ano_escolar": "quinto_fundamental",
					},
					"response": map[string]interface{}{
						"message": "estudante criado com sucesso",
						"data": map[string]string{
							"id": "uuid-interno",
							"codigo_estudante": "KAF7392",
						},
					},
				},
				"login_estudante": gin.H{
					"request": map[string]string{
						"usuario": "KAF7392",
						"senha": "senha123",
						"type": "estudante",
					},
				},
				"registrar_notas": gin.H{
					"request": map[string]interface{}{
						"codigo_estudante": "KAF7392",
						"ano_lectivo": "2025/2026",
						"periodo": "trimestre_1",
						"materias": []map[string]interface{}{
							{"nome": "Matemática", "nota": 17},
							{"nome": "Português", "nota": 16},
						},
					},
				},
				"consultar_notas": gin.H{
					"url": "/notas-estudante/KAF7392",
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
				"✅ Códigos fáceis de memorizar (estudantes e academias)",
				"✅ Referências usando códigos em vez de UUIDs",
			},
		})
	})

	return router
}