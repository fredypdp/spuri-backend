// ============================================================================
// ARQUIVO: cmd/server/main.go
// ============================================================================

package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"spuri/internal/db"
	"spuri/internal/handlers"
	"spuri/internal/middleware"
	"spuri/internal/projections"
)

var (
	dbClient    *db.Client
	repository  *db.AggregateRepository
	projManager *projections.Manager
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if os.Getenv("ENV") != "production" {
		if err := godotenv.Load(); err != nil {
			log.Println("[WARN] Arquivo .env não encontrado")
		}
	}

	if err := initDB(); err != nil {
		log.Fatalf("[ERROR] Erro ao conectar ao banco de dados: %v", err)
	}
	defer dbClient.Close()

	if err := initProjections(); err != nil {
		log.Fatalf("[ERROR] Erro ao inicializar projeções: %v", err)
	}

	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := setupRouter()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	if err := router.Run("0.0.0.0:" + port); err != nil {
		log.Fatalf("[ERROR] Erro ao iniciar servidor: %v", err)
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

	if err := dbClient.RunMigrations(); err != nil {
		return fmt.Errorf("erro ao rodar migrations: %w", err)
	}

	log.Println("[INFO] Banco de dados inicializado com Event Sourcing")
	return nil
}

func initProjections() error {
	projManager = projections.NewManager(dbClient)

	projManager.RegisterProjection("estudantes", projections.NewEstudanteProjection(dbClient))
	projManager.RegisterProjection("academias", projections.NewAcademiaProjection(dbClient))
	projManager.RegisterProjection("admins", projections.NewAdminProjection(dbClient))
	projManager.RegisterProjection("notas", projections.NewNotasProjection(dbClient))
	projManager.RegisterProjection("faltas", projections.NewFaltasProjection(dbClient))
	projManager.RegisterProjection("cursos", projections.NewCursosProjection(dbClient))
	projManager.RegisterProjection("materias", projections.NewMateriasProjection(dbClient))
	projManager.RegisterProjection("sistema_config", projections.NewSistemaConfigProjection(dbClient))
	projManager.RegisterProjection("turmas", projections.NewTurmasProjection(dbClient))
	projManager.RegisterProjection("avaliacao_final", projections.NewAvaliacaoFinalProjection(dbClient))
	projManager.RegisterProjection("categorias_nota", projections.NewCategoriasNotaProjection(dbClient))
	projManager.RegisterProjection("aprovacao_ano", projections.NewAprovacaoAnoProjection(dbClient))
	projManager.RegisterProjection("reprovacoes", projections.NewReprovacoesProjection(dbClient))

	go projManager.StartProcessing()

	log.Println("[INFO] Sistema de projeções inicializado")
	return nil
}

func setupRouter() *gin.Engine {
	router := gin.Default()

	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Next()
	})

	router.Use(middleware.MonitoringMiddleware())
	router.Use(corsMiddleware())
	router.Use(middleware.GlobalRateLimit())
	router.Use(requestIDMiddleware())

	router.Use(func(c *gin.Context) {
		c.Set("repository", repository)
		c.Set("projManager", projManager)
		c.Set("dbClient", dbClient)
		c.Next()
	})

	router.GET("/health", handlers.HealthCheckBasic)
	router.GET("/health/detailed", middleware.AuthMiddleware(), middleware.RequireAdmin(), handlers.HealthCheckDetailed)
	router.POST("/bootstrap/admin-fpp", handlers.BootstrapAdminFPP)

	loginGroup := router.Group("/")
	loginGroup.Use(middleware.LoginRateLimit())
	{
		loginGroup.POST("/login", handlers.Login)
		loginGroup.POST("/admin/login", handlers.LoginAdmin)
	}

	emailGroup := router.Group("/")
	{
		emailGroup.POST("/verificar-email/:token", handlers.VerificarEmail)
		emailGroup.POST("/verificar-email/solicitar", handlers.SolicitarVerificacaoEmail)
		emailGroup.POST("/recuperar-senha/solicitar", handlers.SolicitarRecuperacaoSenha)
		emailGroup.POST("/recuperar-senha/:token", handlers.ResetarSenha)
		emailGroup.POST("/gerar-token/verificacao", handlers.GerarTokenVerificacao)
		emailGroup.POST("/gerar-token/recuperacao", handlers.GerarTokenRecuperacao)
	}
	
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.PUT("/alterar-senha", handlers.AlterarSenha)
		protected.GET("/meu-perfil", handlers.GetMeuPerfil)
		protected.GET("/academias", handlers.ListarTodasAcademias)
		protected.GET("/notas-estudante/:codigo", handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:codigo", handlers.GetFaltasEstudante)
		protected.GET("/historico-estudante/:codigo", handlers.GetHistoricoCompleto)
		protected.GET("/eventos-estudante/:codigo", handlers.GetEventosEstudante)
		protected.GET("/verificar-integridade/:codigo", handlers.VerificarIntegridade)
		protected.GET("/consultar-estudante/:codigo", handlers.GetEstudantePorCodigo)
		protected.GET("/consultar-academia/:codigo", handlers.GetAcademiaPorCodigo)
		protected.GET("/estudantes", handlers.ListarEstudantes)
		protected.GET("/ano-letivo-atual", handlers.GetAnoLetivoAtual)
		protected.GET("/avaliacoes", handlers.ListarAvaliacoes)
		protected.GET("/aprovacoes", handlers.ListarAprovacoes)
		protected.GET("/reprovacoes", handlers.ListarReprovacoes)
		protected.GET("/avaliacoes-estudante/:codigo", middleware.RequireAcademiaOuAdmin(), handlers.GetAvaliacoesFinaisEstudante)
	}
	
	estudante := router.Group("/estudante")
	estudante.Use(middleware.AuthMiddleware())
	estudante.Use(middleware.RequireEstudante())
	{
		estudante.GET("/meu-historico", handlers.GetMeuHistorico)
		estudante.PUT("/status-escolar-fundamental", handlers.AtualizarStatusEscolarFundamentalHandler)
		estudante.PUT("/status-escolar-medio", handlers.AtualizarStatusEscolarMedioHandler)
		estudante.PUT("/status-superior", handlers.AtualizarStatusSuperior)
		estudante.PUT("/dados-pessoais", handlers.AtualizarDadosPessoaisEstudante)
		estudante.PUT("/dados-academicos", handlers.AtualizarDadosAcademicosEstudante)
		estudante.GET("/minhas-avaliacoes", handlers.GetMinhasAvaliacoes)
	}
	
	academia := router.Group("/academia")
	academia.Use(middleware.AuthMiddleware())
	academia.Use(middleware.RequireAcademia())
	academia.Use(middleware.ValidarStatusAcademia())
	{
		academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
		academia.GET("/consultar-estudante/:codigo", handlers.GetEstudantePorCodigo)
		academia.GET("/consultar-academia/:codigo", handlers.GetAcademiaPorCodigo)
		academia.POST("/cursos", handlers.CriarCurso)
		academia.GET("/cursos", handlers.ListarCursos)
		academia.PUT("/cursos/:id/ativar", handlers.AtivarCurso)
		academia.PUT("/cursos/:id/desativar", handlers.DesativarCurso)
		academia.POST("/materias", handlers.CriarMateria)
		academia.GET("/materias", handlers.ListarMaterias)
		academia.PUT("/materias/:id", handlers.AtualizarDadosMateria)
		academia.PUT("/materias/:id/ativar", handlers.AtivarMateria)
		academia.PUT("/materias/:id/desativar", handlers.DesativarMateria)
		academia.PUT("/materias/:id/periodo", handlers.DefinirPeriodoMateria)
		academia.DELETE("/materias/:id", handlers.DeletarMateria)
		academia.PUT("/dados", handlers.AtualizarDadosAcademia)
		academia.PUT("/cursos/:id", handlers.AtualizarDadosCurso)
		academia.PUT("/estudante/:codigo/curso", handlers.AlterarCursoEstudante)
		academia.POST("/estudante/register", handlers.RegisterEstudantePorAcademia)
		academia.POST("/registrar-nota", handlers.RegistrarNota)
		academia.PUT("/atualizar-nota", handlers.AtualizarNota)
		academia.POST("/aprovacao-ano", handlers.RegistrarAprovacaoAno)
		academia.POST("/categorias-nota", handlers.CriarCategoriaNotaSuperior)
		academia.GET("/categorias-nota", handlers.ListarCategoriasNota)
		academia.POST("/turmas", handlers.CriarTurma)
		academia.GET("/turmas", handlers.ListarTurmasAcademia)
		academia.GET("/turmas/:codigo", handlers.GetTurma)
		academia.PUT("/turmas/:codigo", handlers.AtualizarTurma)
		academia.POST("/turmas/:codigo/estudantes", handlers.AdicionarEstudanteATurma)
		academia.DELETE("/turmas/:codigo/estudantes/:codigoEstudante", handlers.RemoverEstudanteDaTurma)
		academia.POST("/avaliacao-final", handlers.RegistrarAvaliacaoFinal)
	}

	// -----------------------------------------------------------------------
	// Rotas exclusivas para administradores
	// -----------------------------------------------------------------------
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
		admin.GET("/metrics", handlers.GetMetrics)
		admin.GET("/system-stats", handlers.GetSystemStats)

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
			adminAdm.POST("/academia/register", handlers.RegisterAcademia)
		}

		adminFPP := admin.Group("/")
		adminFPP.Use(middleware.RequireFPP())
		{
			adminFPP.PUT("/admin/:id/ativar", handlers.AtivarAdmin)
			adminFPP.PUT("/admin/:id/desativar", handlers.DesativarAdmin)
			adminFPP.PUT("/role/:id", handlers.AtualizarRoleAdmin)
		}
	}

	return router
}

// ============================================================================
// Middlewares auxiliares do servidor
// ============================================================================

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")

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
			requestID = fmt.Sprintf("%d-%s",
				time.Now().UnixNano(),
				strings.ReplaceAll(c.ClientIP(), ".", "-"),
			)
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}