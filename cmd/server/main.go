package main

import (
	"log"
	"os"
	"fmt"
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
	projManager.RegisterProjection("inscricoes", projections.NewInscricoesProjection(dbClient))
	projManager.RegisterProjection("cursos", projections.NewCursosProjection(dbClient))
	projManager.RegisterProjection("materias", projections.NewMateriasProjection(dbClient))
	projManager.RegisterProjection("aprovacao_ano", projections.NewAprovacaoAnoProjection(dbClient))
	projManager.RegisterProjection("sistema_config", projections.NewSistemaConfigProjection(dbClient))
	projManager.RegisterProjection("turmas", projections.NewTurmasProjection(dbClient))

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

	router.GET("/docs", handlers.GetSwaggerUI)
	router.GET("/docs/openapi.json", handlers.GetOpenAPISpec)	
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
	// emailGroup.Use(middleware.EmailRateLimit())
	{
		// Rotas originais (enviam email via backend)
		emailGroup.POST("/verificar-email/:token", handlers.VerificarEmail)
		emailGroup.POST("/verificar-email/solicitar", handlers.SolicitarVerificacaoEmail)
		emailGroup.POST("/recuperar-senha/solicitar", handlers.SolicitarRecuperacaoSenha)
		emailGroup.POST("/recuperar-senha/:token", handlers.ResetarSenha)
		
		// Novas rotas (retornam token para frontend enviar email)
		emailGroup.POST("/gerar-token/verificacao", handlers.GerarTokenVerificacao)
		emailGroup.POST("/gerar-token/recuperacao", handlers.GerarTokenRecuperacao)
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
		protected.GET("/inscricoes/estudante/:codigo", handlers.GetInscricoesPorCodigoEstudante)
		protected.GET("/estudantes", handlers.ListarEstudantes)
		protected.GET("/aprovacoes-estudante/:codigo", handlers.GetAprovacoesEstudante)
		protected.GET("/ano-letivo-atual", handlers.GetAnoLetivoAtual)
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
		estudante.GET("/minhas-aprovacoes", handlers.GetMinhasAprovacoes)
		estudante.GET("/inscricoes/:codigo", handlers.GetInscricoesPorCodigoEstudante)
	}

	academia := router.Group("/academia")
	academia.Use(middleware.AuthMiddleware())
	academia.Use(middleware.RequireAcademia())
	academia.Use(middleware.ValidarStatusAcademia())
	{
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
		academia.POST("/aprovacao-ano", handlers.RegistrarAprovacaoAno)
		academia.PUT("/materias/:id/ativar", handlers.AtivarMateria)
		academia.PUT("/materias/:id/desativar", handlers.DesativarMateria)
		academia.GET("/inscricoes/estudante/:codigo", handlers.GetInscricoesPorCodigoEstudante)
		academia.PUT("/estudante/:codigo/curso", handlers.AlterarCursoEstudante)
		academia.POST("/estudante/register", handlers.RegisterEstudantePorAcademia)
		academia.POST("/registrar-nota", handlers.RegistrarNota)
		academia.PUT("/atualizar-nota", handlers.AtualizarNota)
		academia.POST("/categorias-nota", handlers.CriarCategoriaNotaSuperior)
		academia.GET("/categorias-nota", handlers.ListarCategoriasNota)
		academia.POST("/turmas", handlers.CriarTurma)
		academia.GET("/turmas", handlers.ListarTurmasAcademia)
		academia.GET("/turmas/:codigo", handlers.GetTurma)
		academia.PUT("/turmas/:codigo", handlers.AtualizarTurma)
		academia.POST("/turmas/:codigo/estudantes", handlers.AdicionarEstudanteATurma)
		academia.DELETE("/turmas/:codigo/estudantes/:codigoEstudante", handlers.RemoverEstudanteDaTurma)
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
			adminFPP.POST("/metrics/reset", handlers.ResetMetrics)
			adminFPP.POST("/definir-ano-letivo", handlers.DefinirAnoLetivo)
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
	env := os.Getenv("ENV")
	allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")
	var allowedOrigins []string
	
	if env == "production" {
		if allowedOriginsEnv != "" {
			allowedOrigins = strings.Split(allowedOriginsEnv, ",")
			for i := range allowedOrigins {
				allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
			}
			log.Printf("[INFO] CORS PRODUCAO: %v", allowedOrigins)
		} else {
			log.Printf("[WARN] ATENCAO: ALLOWED_ORIGINS não configurado em PRODUCAO")
			log.Printf("[ERROR] CORS BLOQUEADO - Configure ALLOWED_ORIGINS")
			allowedOrigins = []string{}
		}
	} else {
		if allowedOriginsEnv != "" {
			allowedOrigins = strings.Split(allowedOriginsEnv, ",")
			for i := range allowedOrigins {
				allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
			}
		} else {
			allowedOrigins = []string{
				"http://localhost:3000",
				"http://localhost:5173",
				"http://localhost:8080",
			}
		}
		log.Printf("[INFO] CORS DESENVOLVIMENTO: %v", allowedOrigins)
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		
		originAllowed := false
		for _, allowedOrigin := range allowedOrigins {
			if origin == allowedOrigin {
				originAllowed = true
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				break
			}
		}
		
		if !originAllowed && origin != "" {
			if env == "production" {
				log.Printf("[WARN] CORS BLOQUEADO: %s (não está na whitelist)", origin)
			}
		}
		
		if originAllowed {
			c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Max-Age", "86400")
		}
		
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		
		if c.Request.Method == "OPTIONS" {
			if originAllowed {
				c.AbortWithStatus(204)
			} else {
				c.AbortWithStatus(403)
			}
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