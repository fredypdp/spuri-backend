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

	// Injeção de dependências — disponível em todos os handlers via c.Get(...)
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", dbClient)
		c.Set("repository", repository)
		c.Set("projManager", projManager)
		c.Next()
	})

	// ── Rotas públicas ─────────────────────────────────────────────────────
	//
	// POST /login é o único endpoint de autenticação do sistema.
	// Aceita type "admin" | "academia" | "estudante" no body.
	// Não existe /admin/login — o handler Login em auth_handlers.go cobre os 3 tipos.
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

	// ── Rotas exclusivas do estudante ─────────────────────────────────────
	estudante := router.Group("/estudante")
	estudante.Use(middleware.AuthMiddleware())
	estudante.Use(middleware.RequireEstudante())
	{
		estudante.PUT("/dados-pessoais", handlers.AtualizarDadosPessoais)
		estudante.GET("/minhas-avaliacoes", handlers.GetMinhasAvaliacoes)
		estudante.GET("/minhas-notas", handlers.GetMinhasNotas)
		estudante.GET("/minhas-faltas", handlers.GetMinhasFaltas)
	}

	// ── Rotas de academia ─────────────────────────────────────────────────
	academia := router.Group("/academia")
	academia.Use(middleware.AuthMiddleware())
	academia.Use(middleware.RequireAcademia())
	academia.Use(middleware.ValidarStatusAcademia())
	{
		academia.PUT("/dados", handlers.AtualizarDadosAcademia)
		
		academia.POST("/estudante/register", handlers.RegisterEstudantePorAcademia)
		academia.POST("/notas-aluno", handlers.RegistrarNota)
		academia.PUT("/atualizar-nota", handlers.AtualizarNota)
		academia.DELETE("/nota/:id",  handlers.DeletarNota)
		academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
		academia.PUT("/atualizar-falta", handlers.AtualizarFalta)
		academia.DELETE("/falta/:id", handlers.DeletarFalta)
		academia.POST("/aprovacao-ano", handlers.RegistrarAprovacaoAno)
		academia.POST("/avaliacao-final", handlers.RegistrarAvaliacaoFinal)
		academia.POST("/categorias-nota", handlers.CriarCategoriaNota)
		academia.GET("/categorias-nota", handlers.ListarCategoriasNota)
		academia.PUT("/estudante/:codigo/status-escolar-fundamental", handlers.AtualizarStatusEscolarFundamentalHandler)
		academia.PUT("/estudante/:codigo/status-escolar-medio", handlers.AtualizarStatusEscolarMedioHandler)
		academia.PUT("/estudante/:codigo/status-superior", handlers.AtualizarStatusSuperiorHandler)

		// ── Cursos ────────────────────────────────────────────────────────
		academia.POST("/curso", handlers.CriarCurso)
		academia.GET("/cursos", handlers.ListarCursos)
		academia.GET("/curso/:id", handlers.GetCurso)
		academia.PUT("/curso/:id/ativar", handlers.AtivarCurso)
		academia.PUT("/curso/:id/desativar", handlers.DesativarCurso)
		academia.PUT("/curso/:id/dados", handlers.AtualizarDadosCurso)
		academia.DELETE("/curso/:id", handlers.DeletarCurso)

		// ── Matérias ──────────────────────────────────────────────────────
		academia.POST("/materia", handlers.CriarMateria)
		academia.GET("/materias", handlers.ListarMaterias)
		academia.GET("/materia/:id", handlers.GetMateria)
		academia.PUT("/materia/:id/ativar", handlers.AtivarMateria)
		academia.PUT("/materia/:id/desativar", handlers.DesativarMateria)
		academia.PUT("/materia/:id/periodo", handlers.DefinirPeriodoMateria)
		academia.PUT("/materia/:id/dados", handlers.AtualizarDadosMateria)
		academia.DELETE("/materia/:id", handlers.DeletarMateria)

		// ── Turmas ────────────────────────────────────────────────────────
		academia.POST("/turma", handlers.CriarTurma)
		academia.GET("/turmas", handlers.ListarTurmasAcademia)
		academia.GET("/turma/:codigo", handlers.GetTurma)
		academia.PUT("/turma/:codigo/ativar", handlers.AtivarTurma)
		academia.PUT("/turma/:codigo/desativar", handlers.DesativarTurma)
		academia.PUT("/turma/:codigo/dados", handlers.AtualizarDadosTurma)
		academia.DELETE("/turma/:codigo", handlers.DeletarTurma)
		academia.POST("/turma/:codigo/estudante", handlers.AdicionarEstudanteATurma)
		academia.DELETE("/turma/:codigo/estudantes/:codigo_estudante", handlers.RemoverEstudanteDaTurma)

	}

	// ── Rotas de admin ────────────────────────────────────────────────────
	// Este grupo exige autenticação válida (AuthMiddleware) e role admin (RequireAdmin).
	admin := router.Group("/dominis")
	admin.Use(middleware.AuthMiddleware())
	admin.Use(middleware.RequireAdmin())
	{
		admin.POST("/register", handlers.RegisterAdmin)
		admin.POST("/academia/register", handlers.RegisterAcademia)
		admin.PUT("/academia/:codigo/ativar", middleware.RequireAdm(), handlers.AtivarAcademia)
		admin.PUT("/academia/:codigo/desativar", middleware.RequireAdm(), handlers.DesativarAcademia)
		admin.PUT("/admin/:id/ativar", middleware.RequireAdm(), handlers.AtivarAdmin)
		admin.PUT("/admin/:id/desativar", middleware.RequireAdm(), handlers.DesativarAdmin)
		admin.GET("/admin-lista", handlers.ListarTodosAdmins)
		admin.POST("/ano-letivo", middleware.RequireFPP(), handlers.DefinirAnoLetivo)
		admin.GET("/metrics", handlers.GetSystemMetrics)
		admin.POST("/projections/rebuild/:name", middleware.RequireFPP(), handlers.RebuildProjection)
		admin.GET("/consultar-admin/:email", handlers.GetAdminPorEmail)
		admin.GET("/registros", handlers.ListarTodosRegistros)
		admin.GET("/registros/:codigo", handlers.ListarRegistrosPorEstudante)
		admin.PUT("/admin/:id/role", middleware.RequireFPP(), handlers.AtualizarRoleAdmin)
		admin.PUT("/admin/:id/dados", handlers.AtualizarDadosAdmin)
	}

	return router
}

// corsMiddleware — whitelist configurável via ALLOWED_ORIGINS.
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")
		var allowedOrigins []string
		if allowedOriginsEnv != "" {
			for _, o := range strings.Split(allowedOriginsEnv, ",") {
				trimmed := strings.TrimSpace(o)
				if trimmed != "" {
					allowedOrigins = append(allowedOrigins, trimmed)
				}
			}
		}

		origin := c.Request.Header.Get("Origin")
		allowed := false

		if len(allowedOrigins) == 0 {
			allowed = true // modo dev: aceita qualquer origem
		} else {
			for _, o := range allowedOrigins {
				if o == origin {
					allowed = true
					break
				}
			}
		}

		if allowed && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Request-ID")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// requestIDMiddleware propaga ou gera um X-Request-ID para rastreamento.
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-ID")

		if requestID != "" && !requestIDPattern.MatchString(requestID) {
			requestID = ""
		}

		if requestID == "" {
			requestID = fmt.Sprintf("%d", time.Now().UnixNano())
		}

		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)
		c.Next()
	}
}