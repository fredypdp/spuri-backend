package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
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

// requestIDPattern define os caracteres permitidos num X-Request-ID externo.
// Apenas alfanuméricos e hífen — sem newlines, null bytes ou caracteres de controlo.
var requestIDPattern = regexp.MustCompile(`^[a-zA-Z0-9\-]{1,64}$`)

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

	// ── Tier 1 — sem dependências externas ───────────────────────────────
	projManager.RegisterProjection("admins", projections.NewAdminProjection(dbClient))
	projManager.RegisterProjection("academias", projections.NewAcademiaProjection(dbClient))
	projManager.RegisterProjection("cursos", projections.NewCursosProjection(dbClient))
	projManager.RegisterProjection("materias", projections.NewMateriasProjection(dbClient))
	projManager.RegisterProjection("sistema_config", projections.NewSistemaConfigProjection(dbClient))
	projManager.RegisterProjection("categorias_nota", projections.NewCategoriasNotaProjection(dbClient))

	// ── Tier 2 — dependem de academias/cursos ────────────────────────────
	projManager.RegisterProjection("estudantes", projections.NewEstudanteProjection(dbClient))
	projManager.RegisterProjection("turmas", projections.NewTurmasProjection(dbClient))

	// ── Tier 3 — dependem de estudantes e materias ───────────────────────
	projManager.RegisterProjection("notas", projections.NewNotasProjection(dbClient))
	projManager.RegisterProjection("faltas", projections.NewFaltasProjection(dbClient))

	// ── Tier 4 — dependem de estudantes e aprovações ─────────────────────
	projManager.RegisterProjection("aprovacoes", projections.NewAprovacaoAnoProjection(dbClient))
	projManager.RegisterProjection("reprovacoes", projections.NewReprovacoesProjection(dbClient))
	projManager.RegisterProjection("avaliacao_final", projections.NewAvaliacaoFinalProjection(dbClient))

	go projManager.StartProcessing()
	return nil
}

func setupRouter() *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())
	router.Use(requestIDMiddleware())
	router.Use(middleware.MonitoringMiddleware())
	router.Use(middleware.GlobalRateLimit())

	// Injeção de dependências
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", dbClient)
		c.Set("repository", repository)
		c.Set("projManager", projManager)
		c.Next()
	})

	// ── Rotas públicas ────────────────────────────────────────────────────
	router.POST("/login", middleware.LoginRateLimit(), handlers.Login)
	router.POST("/bootstrap", handlers.BootstrapAdminFPP)

	// ── Rotas de email (públicas com rate limit próprio) ──────────────────
	emailGroup := router.Group("/email")
	emailGroup.Use(middleware.EmailRateLimit())
	{
		emailGroup.POST("/verificar-email/:token", handlers.VerificarEmail)
		emailGroup.POST("/verificar-email/solicitar", handlers.SolicitarVerificacaoEmail)
		emailGroup.POST("/recuperar-senha/solicitar", handlers.SolicitarRecuperacaoSenha)
		emailGroup.POST("/recuperar-senha/:token", handlers.ResetarSenha)
		emailGroup.POST("/gerar-token/verificacao", handlers.GerarTokenVerificacao)
		emailGroup.POST("/gerar-token/recuperacao", handlers.GerarTokenRecuperacao)
	}

	// ── Rotas autenticadas (qualquer tipo) ────────────────────────────────
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.PUT("/alterar-senha", handlers.AlterarSenha)
		protected.GET("/meu-perfil", handlers.GetMeuPerfil)
		protected.GET("/academias", handlers.ListarTodasAcademias)
		protected.GET("/eventos-estudante/:codigo", handlers.GetEventosEstudante)
		protected.GET("/verificar-integridade/:codigo", handlers.VerificarIntegridade)
		protected.GET("/consultar-estudante/:codigo", handlers.GetEstudantePorCodigo)
		protected.GET("/consultar-academia/:codigo", handlers.GetAcademiaPorCodigo)
		protected.GET("/estudantes", handlers.ListarEstudantes)
		protected.GET("/ano-letivo-atual", handlers.GetAnoLetivoAtual)
		protected.GET("/avaliacoes", handlers.ListarAvaliacoes)
		protected.GET("/aprovacoes", handlers.ListarAprovacoes)
		protected.GET("/reprovacoes", handlers.ListarReprovacoes)
		
		protected.GET("/notas-estudante/:codigo", middleware.RequireAcademiaOuAdmin(), handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:codigo", middleware.RequireAcademiaOuAdmin(), handlers.GetFaltasEstudante)
		protected.GET("/avaliacoes-estudante/:codigo", middleware.RequireAcademiaOuAdmin(), handlers.GetAvaliacoesFinaisEstudante)
	}
	
	estudante := router.Group("/estudante")
	estudante.Use(middleware.AuthMiddleware())
	estudante.Use(middleware.RequireEstudante())
	{
		estudante.PUT("/dados-pessoais", handlers.AtualizarDadosPessoais)
		estudante.GET("/minhas-avaliacoes", handlers.GetMinhasAvaliacoes)
		// H4-06: rotas de leitura exclusivas do estudante autenticado.
		// O handler usa o userID do JWT — sem parâmetro de código na URL.
		estudante.GET("/minhas-notas", handlers.GetMinhasNotas)
		estudante.GET("/minhas-faltas", handlers.GetMinhasFaltas)
	}

	// ── Rotas de academia ─────────────────────────────────────────────────
	academia := router.Group("/academia")
	academia.Use(middleware.AuthMiddleware())
	academia.Use(middleware.RequireAcademia())
	academia.Use(middleware.ValidarStatusAcademia())
	{
		academia.POST("/estudante/register", handlers.RegisterEstudantePorAcademia)
		academia.POST("/notas-aluno", handlers.RegistrarNota)
		academia.PUT("/atualizar-nota", handlers.AtualizarNota)
		academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
		academia.POST("/aprovacao-ano", handlers.RegistrarAprovacaoAno)
		academia.POST("/avaliacao-final", handlers.RegistrarAvaliacaoFinal)
		academia.POST("/categorias-nota", handlers.CriarCategoriaNotaSuperior)
		academia.GET("/categorias-nota", handlers.ListarCategoriasNota)

		// FIX-C4: novas rotas de status escolar — protegidas por RequireAcademia().
		// Academia informa o codigo do estudante na URL e o novo status no body.
		academia.PUT("/estudante/:codigo/status-escolar-fundamental", handlers.AtualizarStatusEscolarFundamentalHandler)
		academia.PUT("/estudante/:codigo/status-escolar-medio", handlers.AtualizarStatusEscolarMedioHandler)
		academia.PUT("/estudante/:codigo/status-superior", handlers.AtualizarStatusSuperiorHandler)

		// ── Turmas ────────────────────────────────────────────────────────
		academia.POST("/turmas", handlers.CriarTurma)
		academia.GET("/turmas", handlers.ListarTurmasAcademia)
		academia.GET("/turmas/:codigo", handlers.GetTurma)
		academia.PUT("/turmas/:codigo/ativar", handlers.AtivarTurma)
		academia.PUT("/turmas/:codigo/desativar", handlers.DesativarTurma)
		academia.PUT("/turmas/:codigo/dados", handlers.AtualizarDadosTurma)
		academia.DELETE("/turmas/:codigo", handlers.DeletarTurma)
		academia.POST("/turmas/:codigo/estudantes", handlers.AdicionarEstudanteATurma)
		academia.DELETE("/turmas/:codigo/estudantes/:codigo_estudante", handlers.RemoverEstudanteDaTurma)

		// ── Cursos ────────────────────────────────────────────────────────
		academia.POST("/cursos", handlers.CriarCurso)
		academia.GET("/cursos", handlers.ListarCursos)
		academia.GET("/cursos/:id", handlers.GetCurso)
		academia.PUT("/cursos/:id/ativar", handlers.AtivarCurso)
		academia.PUT("/cursos/:id/desativar", handlers.DesativarCurso)
		academia.PUT("/cursos/:id/dados", handlers.AtualizarDadosCurso)
		academia.DELETE("/cursos/:id", handlers.DeletarCurso)

		// ── Matérias ──────────────────────────────────────────────────────
		academia.POST("/materias", handlers.CriarMateria)
		academia.GET("/materias", handlers.ListarMaterias)
		academia.GET("/materias/:id", handlers.GetMateria)
		academia.PUT("/materias/:id/ativar", handlers.AtivarMateria)
		academia.PUT("/materias/:id/desativar", handlers.DesativarMateria)
		academia.PUT("/materias/:id/periodo", handlers.DefinirPeriodoMateria)
		academia.PUT("/materias/:id/dados", handlers.AtualizarDadosMateria)
		academia.DELETE("/materias/:id", handlers.DeletarMateria)

		// ── Atualização geral ─────────────────────────────────────────────
		academia.PUT("/dados", handlers.AtualizarDadosAcademia)
	}

	// ── Rotas de admin ────────────────────────────────────────────────────
	admin := router.Group("/admin")
	admin.Use(middleware.AuthMiddleware())
	admin.Use(middleware.RequireAdmin())
	{
		admin.POST("/register", handlers.RegisterAdmin)
		admin.POST("/academia/register", handlers.RegisterAcademia)
		
		admin.PUT("/academia/:codigo/ativar", middleware.RequireAdm(), handlers.AtivarAcademia)
		admin.PUT("/academia/:codigo/desativar", middleware.RequireAdm(), handlers.DesativarAcademia)

		admin.PUT("/admin/:id/ativar", handlers.AtivarAdmin)
		admin.PUT("/admin/:id/desativar", handlers.DesativarAdmin)
		admin.GET("/admins", handlers.ListarTodosAdmins)
		admin.GET("/ano-letivo", handlers.GetAnoLetivoAtual)
		admin.POST("/ano-letivo", handlers.DefinirAnoLetivo)
		admin.GET("/metrics", handlers.GetSystemMetrics)
		admin.POST("/projections/rebuild/:name", handlers.RebuildProjection)

		// FIX E4-ED-02: /consultar-admin restrito a adm+ (verificado no handler).
		admin.GET("/consultar-admin/:email", handlers.GetAdminPorEmail)

		// Rotas de registros/relatórios
		admin.GET("/registros", handlers.ListarTodosRegistros)
		admin.GET("/registros/:codigo", handlers.ListarRegistrosPorEstudante)

		// Rotas de gestão de admin
		admin.PUT("/admin/:id/role", handlers.AtualizarRoleAdmin)
		admin.PUT("/admin/:id/dados", handlers.AtualizarDadosAdmin)
	}

	// ── Rotas de health / docs ─────────────────────────────────────────────
	router.GET("/health", handlers.HealthCheck)
	router.GET("/docs", handlers.GetDocs)

	return router
}

// corsMiddleware — whitelist configurável via ALLOWED_ORIGINS.
func corsMiddleware() gin.HandlerFunc {
	env := os.Getenv("ENV")
	rawOrigins := os.Getenv("ALLOWED_ORIGINS")

	allowedOrigins := map[string]bool{}

	if rawOrigins != "" {
		for _, o := range strings.Split(rawOrigins, ",") {
			origin := strings.TrimSpace(o)
			if origin != "" {
				allowedOrigins[origin] = true
				log.Printf("[CORS] Origin permitida: %s", origin)
			}
		}
	} else if env != "production" {
		defaults := []string{
			"http://localhost:3000",
			"http://localhost:5173",
			"http://localhost:8080",
		}
		for _, o := range defaults {
			allowedOrigins[o] = true
		}
		log.Printf("[CORS] Modo desenvolvimento: permitindo origens localhost")
	} else {
		log.Printf("[WARN] [CORS] ALLOWED_ORIGINS não configurado em produção — CORS desativado para origens externas")
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if origin != "" && allowedOrigins[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}

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

		if requestID != "" {
			// FIX E4-ED-01: validar antes de usar — rejeitar se contiver
			// caracteres fora da whitelist ou exceder 64 caracteres.
			if !requestIDPattern.MatchString(requestID) {
				log.Printf("[WARN] [requestID] X-Request-ID inválido descartado (len=%d) — IP: %s",
					len(requestID), c.ClientIP())
				requestID = ""
			}
		}

		if requestID == "" {
			// Gerar ID interno: não usa entrada do cliente — sem risco de injection.
			requestID = fmt.Sprintf("%d-%s",
				time.Now().UnixNano(),
				strings.ReplaceAll(c.ClientIP(), ".", "-"))
		}

		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}