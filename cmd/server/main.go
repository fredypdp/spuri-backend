package main

import (
	"log"
	"os"
	"spuri/internal/handlers"
	"spuri/internal/middleware"
	"spuri/internal/store"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Carregar variáveis de ambiente
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  Arquivo .env não encontrado, usando variáveis de ambiente do sistema")
	}

	// Inicializar conexão com o banco de dados
	if err := store.InitDB(); err != nil {
		log.Fatalf("❌ Erro ao conectar ao banco de dados: %v", err)
	}
	defer store.CloseDB()

	// Configurar modo do Gin
	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Criar router
	router := gin.Default()

	// Middleware CORS (para permitir requisições do frontend)
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

	// ============================================
	// ROTAS PÚBLICAS (sem autenticação)
	// ============================================
	
	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"message": "Spuri está rodando!",
			"version": "1.0.0",
		})
	})

	// Autenticação
	router.POST("/login", handlers.Login)
	router.POST("/academia/register", handlers.RegisterAcademia)
	router.POST("/estudante/register", handlers.RegisterEstudante)

	// ============================================
	// ROTAS PROTEGIDAS (requerem autenticação)
	// ============================================
	
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		// ============================================
		// ROTAS DE ESTUDANTES
		// ============================================
		estudante := protected.Group("/estudante")
		estudante.Use(middleware.RequireEstudante())
		{
			// Inscrições
			estudante.POST("/inscricao-escola", handlers.InscricaoEscola)
			estudante.POST("/inscricao-universidade", handlers.InscricaoUniversidade)
			estudante.GET("/minhas-inscricoes", handlers.GetMinhasInscricoes)
			
			// Histórico próprio
			estudante.GET("/meu-historico", handlers.GetMeuHistorico)
		}

		// ============================================
		// ROTAS DE ACADEMIAS (Escolas/Universidades)
		// ============================================
		academia := protected.Group("/academia")
		academia.Use(middleware.RequireAcademia())
		{
			// Registros (Commands - CQRS)
			academia.POST("/notas-aluno", handlers.RegistrarNotas)
			academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
			
			// Inscrições
			academia.GET("/inscricoes-pendentes", handlers.ListarInscricoesPendentes)
			academia.PUT("/inscricao/:id/aprovar", handlers.AprovarInscricao)
			academia.PUT("/inscricao/:id/reprovar", handlers.ReprovarInscricao)
		}

		// ============================================
		// ROTAS DE CONSULTA (Queries - CQRS)
		// Acessíveis por estudantes E academias
		// ============================================
		
		// Consultas específicas
		protected.GET("/notas-estudante/:estudanteId", handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:estudanteId", handlers.GetFaltasEstudante)
		protected.GET("/historico-estudante/:estudanteId", handlers.GetHistoricoCompleto)
		
		// Auditoria completa (Event Sourcing)
		protected.GET("/eventos-estudante/:estudanteId", handlers.GetEventosEstudante)
	}

	// ============================================
	// DOCUMENTAÇÃO DAS ROTAS
	// ============================================
	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "Bem-vindo ao Spuri API",
			"version": "1.0.0",
			"arquitetura": gin.H{
				"event_sourcing": "Todos os eventos são registrados de forma imutável",
				"cqrs": "Separação entre comandos (escrita) e queries (leitura)",
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
					"POST /academia/notas-aluno",
					"POST /academia/faltas-aluno",
					"GET /academia/inscricoes-pendentes",
					"PUT /academia/inscricao/:id/aprovar",
					"PUT /academia/inscricao/:id/reprovar",
				},
				"consultas": []string{
					"GET /notas-estudante/:estudanteId",
					"GET /faltas-estudante/:estudanteId",
					"GET /historico-estudante/:estudanteId",
					"GET /eventos-estudante/:estudanteId (auditoria completa)",
				},
			},
		})
	})

	// Iniciar servidor
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Servidor rodando em http://localhost:%s", port)
	log.Printf("📚 Documentação: http://localhost:%s/", port)
	log.Printf("❤️  Health check: http://localhost:%s/health", port)
	
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("❌ Erro ao iniciar servidor: %v", err)
	}
}