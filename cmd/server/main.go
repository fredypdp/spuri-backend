package main

import (
	"log"
	"os"
	"spuri/internal/db"
	"spuri/internal/handlers"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

var (
	dbClient    *db.Client
	repository  *db.AggregateRepository
	projManager *projections.Manager
)

func main() {
	// 🔒 UTF-8: Configurar output de log como UTF-8
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	
	if os.Getenv("ENV") != "production" {
		if err := godotenv.Load(); err != nil {
			log.Println("⚠️  Arquivo .env não encontrado")
		}
	}

	if err := initDB(); err != nil {
		log.Fatalf("❌ Erro ao conectar ao banco de dados: %v", err)
	}
	defer dbClient.Close()

	if err := initProjections(); err != nil {
		log.Fatalf("❌ Erro ao inicializar projeções: %v", err)
	}

	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := setupRouter()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("🚀 Spuri Event Sourcing rodando em http://localhost:%s", port)
	log.Printf("📚 Documentação: http://localhost:%s/", port)
	log.Printf("❤️  Health check: http://localhost:%s/health", port)
	log.Printf("🌍 Ambiente: %s", os.Getenv("ENV"))
	log.Printf("🔒 Segurança: Rate Limiting + Input Validation + Prepared Statements")
	log.Printf("🔤 Encoding: UTF-8")
	
	if err := router.Run("0.0.0.0:" + port); err != nil {
		log.Fatalf("❌ Erro ao iniciar servidor: %v", err)
	}
}

func initDB() error {
	config := db.DefaultConfig()
	
	var err error
	dbClient, err = db.NewClient(config)
	if err != nil {
		return err
	}

	repository = db.NewAggregateRepository(dbClient)

	if err := dbClient.Health(); err != nil {
		return err
	}

	log.Println("✅ Banco de dados inicializado com Event Sourcing")
	return nil
}

func initProjections() error {
	projManager = projections.NewManager(dbClient)
	
	projManager.RegisterProjection("estudantes", projections.NewEstudanteProjection(dbClient))
	projManager.RegisterProjection("academias", projections.NewAcademiaProjection(dbClient))
	projManager.RegisterProjection("admins", projections.NewAdminProjection(dbClient))
	projManager.RegisterProjection("notas", projections.NewNotasProjection(dbClient))
	projManager.RegisterProjection("faltas", projections.NewFaltasProjection(dbClient))
	projManager.RegisterProjection("inscricoes", projections.NewInscricoesProjection(dbClient))
	projManager.RegisterProjection("cursos", projections.NewCursosProjection(dbClient))
	projManager.RegisterProjection("materias", projections.NewMateriasProjection(dbClient))

	go projManager.StartProcessing()

	log.Println("✅ Sistema de projeções inicializado")
	return nil
}

func setupRouter() *gin.Engine {
	router := gin.Default()

	// 🔒 UTF-8: Middleware para garantir UTF-8 em todas respostas
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Next()
	})

	router.Use(corsMiddleware())
	router.Use(middleware.GlobalRateLimit())
	router.Use(requestIDMiddleware())

	router.Use(func(c *gin.Context) {
		c.Set("repository", repository)
		c.Set("projManager", projManager)
		c.Set("dbClient", dbClient)
		c.Next()
	})

	router.GET("/health", handlers.HealthCheck)
	router.POST("/bootstrap/admin-fpp", handlers.BootstrapAdminFPP)

	loginGroup := router.Group("/")
	loginGroup.Use(middleware.LoginRateLimit())
	{
		loginGroup.POST("/login", handlers.Login)
		loginGroup.POST("/admin/login", handlers.LoginAdmin)
	}

	router.POST("/academia/register", handlers.RegisterAcademia)
	router.POST("/estudante/register", handlers.RegisterEstudante)

	emailGroup := router.Group("/")
	emailGroup.Use(middleware.EmailRateLimit())
	{
		emailGroup.GET("/verificar-email/:token", handlers.VerificarEmail)
		emailGroup.POST("/recuperar-senha/solicitar", handlers.SolicitarRecuperacaoSenha)
		emailGroup.POST("/recuperar-senha/:token", handlers.ResetarSenha)
	}

	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.PUT("/alterar-senha", handlers.AlterarSenha)
		protected.GET("/meu-perfil", handlers.GetMeuPerfil)
		protected.GET("/academias", handlers.ListarTodasAcademias)
		protected.GET("/inscricoes", handlers.ListarInscricoes)
		protected.GET("/inscricoes-pendentes", handlers.ListarInscricoesPendentes)
		protected.GET("/notas-estudante/:codigo", handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:codigo", handlers.GetFaltasEstudante)
		protected.GET("/historico-estudante/:codigo", handlers.GetHistoricoCompleto)
		protected.GET("/eventos-estudante/:codigo", handlers.GetEventosEstudante)
		protected.GET("/verificar-integridade/:codigo", handlers.VerificarIntegridade)
		protected.GET("/consultar-estudante/:codigo", handlers.GetEstudantePorCodigo)
		protected.GET("/consultar-academia/:codigo", handlers.GetAcademiaPorCodigo)
		protected.GET("/estudantes", handlers.ListarEstudantes)
	}

	estudante := router.Group("/estudante")
	estudante.Use(middleware.AuthMiddleware())
	estudante.Use(middleware.RequireEstudante())
	{
		estudante.GET("/minhas-inscricoes", handlers.GetMinhasInscricoes)
		estudante.GET("/meu-historico", handlers.GetMeuHistorico)
		estudante.PUT("/status-escolar", handlers.AtualizarStatusEscolar)
		estudante.PUT("/status-superior", handlers.AtualizarStatusSuperior)
		estudante.POST("/inscricao-escola", handlers.InscricaoEscola)
		estudante.POST("/inscricao-universidade", handlers.InscricaoUniversidade)
		estudante.GET("/inscricoes-aprovadas", handlers.ListarInscricoesAprovadas)
		estudante.POST("/vincular-academia", handlers.VincularAcademia)
		estudante.PUT("/dados-pessoais", handlers.AtualizarDadosPessoaisEstudante)
		estudante.PUT("/dados-academicos", handlers.AtualizarDadosAcademicosEstudante)
	}

	academia := router.Group("/academia")
	academia.Use(middleware.AuthMiddleware())
	academia.Use(middleware.RequireAcademia())
	academia.Use(middleware.ValidarStatusAcademia())
	{
		academia.POST("/notas-aluno", handlers.RegistrarNotas)
		academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
		academia.PUT("/inscricao/:id/aprovar", handlers.AprovarInscricao)
		academia.PUT("/inscricao/:id/reprovar", handlers.ReprovarInscricao)
		academia.GET("/consultar-estudante/:codigo", handlers.GetEstudantePorCodigo)
		academia.GET("/consultar-academia/:codigo", handlers.GetAcademiaPorCodigo)
		academia.POST("/cursos", handlers.CriarCurso)
		academia.GET("/cursos", handlers.ListarCursos)
		academia.PUT("/cursos/:id/ativar", handlers.AtivarCurso)
		academia.PUT("/cursos/:id/desativar", handlers.DesativarCurso)
		academia.POST("/materias", handlers.CriarMateria)
		academia.GET("/materias", handlers.ListarMaterias)
		academia.PUT("/dados", handlers.AtualizarDadosAcademia)
		academia.PUT("/cursos/:id", handlers.AtualizarDadosCurso)
		academia.PUT("/materias/:id", handlers.AtualizarDadosMateria)
	}

	admin := router.Group("/admin")
	admin.Use(middleware.AuthMiddleware())
	admin.Use(middleware.RequireAdmin())
	{
		admin.GET("/todos-registros", handlers.ListarTodosRegistros)
		admin.GET("/registros/estudante/:codigo", handlers.ListarRegistrosPorEstudante)
		admin.GET("/registros/academia/:codigo", handlers.ListarRegistrosPorAcademia)
		admin.GET("/consultar-admin/:email", handlers.GetAdminPorEmail)
		admin.GET("/buscar-usuario", handlers.BuscarUsuario)
		admin.PUT("/dados/:id", handlers.AtualizarDadosAdmin)
		
		adminGerente := admin.Group("/")
		adminGerente.Use(middleware.RequireGerente())
		{
			adminGerente.PUT("/academia/:codigo/ativar", handlers.AtivarAcademia)
			adminGerente.PUT("/academia/:codigo/desativar", handlers.DesativarAcademia)
		}
		
		adminAdm := admin.Group("/")
		adminAdm.Use(middleware.RequireAdm())
		{
			adminAdm.POST("/register", handlers.RegisterAdmin)
			adminAdm.GET("/admins", handlers.ListarTodosAdmins)
		}
		
		adminFPP := admin.Group("/")
		adminFPP.Use(middleware.RequireFPP())
		{
			adminFPP.PUT("/admin/:id/ativar", handlers.AtivarAdmin)
			adminFPP.PUT("/admin/:id/desativar", handlers.DesativarAdmin)
			adminFPP.PUT("/role/:id", handlers.AtualizarRoleAdmin)
		}
		
		admin.POST("/rebuild-projection/:name", handlers.RebuildProjection)
		admin.GET("/projection-status/:name", handlers.GetProjectionStatus)
		admin.GET("/projections-status", handlers.GetAllProjectionStatuses)
		admin.GET("/ledger-stats", handlers.GetLedgerStats)
		admin.GET("/verify-all-integrity", handlers.VerifyAllIntegrity)
	}

	router.GET("/", handlers.APIDocumentation)

	return router
}

func corsMiddleware() gin.HandlerFunc {
	allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")
	var allowedOrigins []string
	
	if allowedOriginsEnv != "" {
		allowedOrigins = strings.Split(allowedOriginsEnv, ",")
		for i := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
		}
	} else {
		allowedOrigins = []string{
			"http://localhost:3000",
			"https://spuripainel.vercel.app",
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		
		for _, allowedOrigin := range allowedOrigins {
			if origin == allowedOrigin {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
		
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		
		// 🔒 UTF-8: Garantir charset UTF-8 no CORS
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	}
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")
		if requestID == "" {
			requestID = time.Now().Format("20060102150405")
		}
		c.Set("request_id", requestID)
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Next()
	}
}