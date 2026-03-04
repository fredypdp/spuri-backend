// ============================================================================
// ARQUIVO: cmd/server/main.go
//
// CORREÇÕES APLICADAS:
//   FIX-C4  — Rotas /estudante/status-escolar-* REMOVIDAS das rotas de estudante.
//              Novas rotas /academia/estudante/:codigo/status-* adicionadas,
//              protegidas por RequireAcademia() + ValidarStatusAcademia().
//   FIX-A1  — JWT_SECRET fatal em produção (tratado no middleware/auth.go).
//   [A42]   — CORS: wildcard substituído por whitelist configurável via ALLOWED_ORIGINS.
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
	router.POST("/estudante/register", handlers.RegisterEstudante)

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
		protected.GET("/notas-estudante/:codigo", handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:codigo", handlers.GetFaltasEstudante)
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

	// ── Rotas de estudante ─────────────────────────────────────────────────
	// FIX-C4: status-escolar-* REMOVIDOS daqui — estudante não pode mais alterar
	// seu próprio status escolar. Essa responsabilidade é exclusiva da academia.
	estudante := router.Group("/estudante")
	estudante.Use(middleware.AuthMiddleware())
	estudante.Use(middleware.RequireEstudante())
	{
		estudante.PUT("/dados-pessoais", handlers.AtualizarDadosPessoais)
		estudante.GET("/minhas-avaliacoes", handlers.GetMinhasAvaliacoes)
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
	}

	// ── Rotas de admin ────────────────────────────────────────────────────
	admin := router.Group("/admin")
	admin.Use(middleware.AuthMiddleware())
	admin.Use(middleware.RequireAdmin())
	{
		admin.POST("/register", handlers.RegisterAdmin)
		admin.POST("/academia/register", handlers.RegisterAcademia)
		admin.PUT("/academia/:id/ativar", handlers.AtivarAcademia)
		admin.PUT("/academia/:id/desativar", handlers.DesativarAcademia)
		admin.PUT("/admin/:id/ativar", handlers.AtivarAdmin)
		admin.PUT("/admin/:id/desativar", handlers.DesativarAdmin)
		admin.GET("/admins", handlers.ListarTodosAdmins)
		admin.GET("/ano-letivo", handlers.GetAnoLetivoAtual)
		admin.POST("/ano-letivo", handlers.DefinirAnoLetivo)
		admin.GET("/metrics", handlers.GetSystemMetrics)
		admin.POST("/projections/rebuild/:name", handlers.RebuildProjection)
	}

	return router
}

// corsMiddleware — [A42]: whitelist configurável via ALLOWED_ORIGINS.
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