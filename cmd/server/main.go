package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"spuri/internal/db"
	"spuri/internal/handlers"
	"spuri/internal/jobs"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/storage"
)

var (
	dbClient        *db.Client
	repository      *db.AggregateRepository
	projManager     *projections.Manager
	jobStore        *jobs.Store
	jobWorker       *jobs.Worker
	jobNotifier     *jobs.Notifier
	storageProvider storage.StorageProvider
)

// requestIDPattern define os caracteres permitidos num X-Request-ID externo.
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

	if err := initStorage(); err != nil {
		log.Fatalf("[ERROR] Erro ao inicializar armazenamento: %v", err)
	}

	if err := initProjections(); err != nil {
		log.Fatalf("[ERROR] Erro ao inicializar projeções: %v", err)
	}

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	initJobs(ctx)

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

func initStorage() error {
	provider, err := storage.NewDriveProvider()
	if err != nil {
		if os.Getenv("ENV") == "production" {
			return err
		}
		log.Printf("[WARN] armazenamento Google Drive não configurado, usando provider local: %v", err)
		os.Setenv("GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE", "true")
		provider, err = storage.NewDriveProvider()
		if err != nil {
			return err
		}
	}
	storageProvider = provider
	return nil
}

func initProjections() error {
	projManager = projections.NewManager(dbClient)

	// ── Tier 1 — sem dependências externas ───────────────────────────────
	projManager.RegisterProjection("admins", projections.NewAdminProjection(dbClient))
	projManager.RegisterProjection("academias", projections.NewAcademiaProjection(dbClient))
	projManager.RegisterProjection("cursos", projections.NewCursosProjection(dbClient))
	projManager.RegisterProjection("materias", projections.NewMateriasProjection(dbClient))
	projManager.RegisterProjection("categorias_nota", projections.NewCategoriasNotaProjection(dbClient))
	projManager.RegisterProjection("telefones_extra", projections.NewTelefonesExtraProjection(dbClient))

	// ── Tier 2 — dependem de academias/cursos ────────────────────────────
	projManager.RegisterProjection("estudantes", projections.NewEstudanteProjection(dbClient))
	projManager.RegisterProjection("turmas", projections.NewTurmasProjection(dbClient))

	// ── Tier 3 — dependem de estudantes e materias ───────────────────────
	projManager.RegisterProjection("notas", projections.NewNotasProjection(dbClient))
	projManager.RegisterProjection("faltas", projections.NewFaltasProjection(dbClient))

	// ── Tier 4 — avaliação final ──────────────────────────────────────────
	projManager.RegisterProjection("avaliacao_final", projections.NewAvaliacaoFinalProjection(dbClient))
	projManager.RegisterProjection("solicitacoes_matricula", projections.NewSolicitacaoMatriculaProjection(dbClient))

	go projManager.StartProcessing()
	return nil
}

// initJobs inicializa o sistema de jobs assíncronos.
// setupCtx injeta todas as dependências que os handlers individuais precisam.
func initJobs(ctx context.Context) {
	jobStore = jobs.NewStore(dbClient.DB())
	jobNotifier = jobs.NewNotifier()

	setupCtx := func(c *gin.Context, userID uuid.UUID, userType string) {
		c.Set("user_id", userID)
		c.Set("user_type", userType)
		c.Set("dbClient", dbClient)
		c.Set("repository", repository)
		c.Set("projManager", projManager)
		c.Set("jobStore", jobStore)
		c.Set("jobNotifier", jobNotifier)
		c.Set("storageProvider", storageProvider)
	}

	// 4 goroutines paralelas — ajustar conforme recursos do servidor
	jobWorker = jobs.NewWorker(jobStore, jobNotifier, setupCtx, 4)

	// ── Handlers de job por tipo ──────────────────────────────────────────
	jobWorker.RegisterHandler(jobs.JobTypeRegisterAcademiaBatch, handlers.RegisterAcademia)
	jobWorker.RegisterHandler(jobs.JobTypeAtivarAcademiaBatch, handlers.AtivarAcademiaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDesativarAcademiaBatch, handlers.DesativarAcademiaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeRegisterEstudanteBatch, handlers.RegisterEstudantePorAcademia)
	jobWorker.RegisterHandler(jobs.JobTypeRegistrarNotaBatch, handlers.RegistrarNota)
	jobWorker.RegisterHandler(jobs.JobTypeAtualizarNotaBatch, handlers.AtualizarNotaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDeletarNotaBatch, handlers.DeletarNotaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeRegistrarFaltasBatch, handlers.RegistrarFaltas)
	jobWorker.RegisterHandler(jobs.JobTypeAtualizarFaltaBatch, handlers.AtualizarFaltaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDeletarFaltaBatch, handlers.DeletarFaltaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeCriarCursoBatch, handlers.CriarCurso)
	jobWorker.RegisterHandler(jobs.JobTypeCriarMateriaBatch, handlers.CriarMateria)
	jobWorker.RegisterHandler(jobs.JobTypeCriarTurmaBatch, handlers.CriarTurma)
	jobWorker.RegisterHandler(jobs.JobTypeAdicionarEstudanteBatch, handlers.AdicionarEstudanteATurmaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeAtualizarDadosAcademiaBatch, handlers.AtualizarDadosAcademiaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeCriarCategoriaNotaBatch, handlers.CriarCategoriaNotaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDeletarCategoriaNotaBatch, handlers.DeletarCategoriaNotaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeAtivarCursoBatch, handlers.AtivarCursoJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDesativarCursoBatch, handlers.DesativarCursoJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeAtualizarDadosCursoBatch, handlers.AtualizarDadosCursoJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDeletarCursoBatch, handlers.DeletarCursoJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeAtivarMateriaBatch, handlers.AtivarMateriaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDesativarMateriaBatch, handlers.DesativarMateriaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDefinirPeriodoMateriaBatch, handlers.DefinirPeriodoMateriaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeAtualizarDadosMateriaBatch, handlers.AtualizarDadosMateriaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDeletarMateriaBatch, handlers.DeletarMateriaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeAtivarTurmaBatch, handlers.AtivarTurmaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDesativarTurmaBatch, handlers.DesativarTurmaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeAtualizarDadosTurmaBatch, handlers.AtualizarDadosTurmaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDeletarTurmaBatch, handlers.DeletarTurmaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeRemoverEstudanteTurmaBatch, handlers.RemoverEstudanteTurmaJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeAtivarAdminBatch, handlers.AtivarAdminJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeDesativarAdminBatch, handlers.DesativarAdminJobItem)
	jobWorker.RegisterHandler(jobs.JobTypeRebuildProjection, handlers.RebuildProjectionJobItem)

	jobWorker.Start(ctx)
	log.Println("[INFO] Job worker iniciado com 4 goroutines")
}

func setupRouter() *gin.Engine {
	router := gin.New()

	router.Use(gin.RecoveryWithWriter(gin.DefaultErrorWriter, func(c *gin.Context, recovered interface{}) {
		log.Printf("[PANIC] Recuperado: %v — IP: %s — Path: %s", recovered, c.ClientIP(), c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro interno do servidor"})
	}))

	router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'none'")
		if os.Getenv("ENV") == "production" {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	})

	router.Use(corsMiddleware())
	router.Use(requestIDMiddleware())
	router.Use(middleware.MonitoringMiddleware())
	router.Use(middleware.GlobalRateLimit())

	// Injetar dependências no contexto de cada requisição
	router.Use(func(c *gin.Context) {
		c.Set("dbClient", dbClient)
		c.Set("repository", repository)
		c.Set("projManager", projManager)
		c.Set("jobStore", jobStore)
		c.Set("jobWorker", jobWorker)
		c.Set("jobNotifier", jobNotifier)
		c.Set("storageProvider", storageProvider)
		c.Next()
	})

	// ── Rotas públicas ─────────────────────────────────────────────────────
	router.POST("/login", middleware.LoginRateLimit(), handlers.Login)
	router.POST("/bootstrap", middleware.LoginRateLimit(), handlers.BootstrapAdminFPP)
	router.POST("/solicitacao-matricula", handlers.CriarSolicitacaoMatricula)

	emailGroup := router.Group("/email")
	emailGroup.Use(middleware.EmailRateLimit())
	{
		emailGroup.POST("/verificar-email/:token", handlers.VerificarEmail)
		emailGroup.POST("/verificar-email/solicitar", handlers.SolicitarVerificacaoEmail)
		emailGroup.POST("/recuperar-senha/solicitar", handlers.SolicitarRecuperacaoSenha)
		emailGroup.POST("/recuperar-senha/:token", handlers.ResetarSenha)
		emailGroup.POST("/gerar-token/recuperacao", handlers.GerarTokenRecuperacao)

		emailAuthGroup := emailGroup.Group("/")
		emailAuthGroup.Use(middleware.AuthMiddleware())
		{
			emailAuthGroup.POST("/gerar-token/verificacao", handlers.GerarTokenVerificacao)
		}
	}

	// ── Rotas públicas com autenticação opcional ─────────────────────────
	router.GET("/academias", middleware.OptionalAuthMiddleware(), handlers.ListarTodasAcademias)
	router.GET("/academia/cursos", middleware.OptionalAuthMiddleware(), handlers.ListarCursos)
	router.GET("/academia/curso/:id", middleware.OptionalAuthMiddleware(), handlers.GetCurso)
	router.GET("/consultar-academia/:codigo", middleware.OptionalAuthMiddleware(), handlers.GetAcademiaPorCodigo)

	// ── Rotas de jobs assíncronos (qualquer usuário autenticado) ──────────
	jobRoutes := router.Group("/jobs")
	jobRoutes.Use(middleware.AuthMiddleware())
	{
		jobRoutes.GET("", handlers.ListJobs)
		jobRoutes.GET("/stream", handlers.StreamJobs)
		jobRoutes.DELETE("/:id/sse", handlers.HideJobFromSSE)
		jobRoutes.POST("/:id/retry-failed", handlers.RetryFailedJob)
		jobRoutes.GET("/:id", handlers.GetJob)
	}

	// ── Rotas autenticadas (qualquer tipo) ────────────────────────────────
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware())
	{
		protected.POST("/logout", handlers.Logout)
		protected.PUT("/alterar-senha", handlers.AlterarSenha)
		protected.GET("/meu-perfil", handlers.GetMeuPerfil)
		protected.GET("/eventos-estudante/:codigo", handlers.GetEventosEstudante)
		protected.GET("/verificar-integridade/:codigo", handlers.VerificarIntegridade)
		protected.GET("/consultar-estudante/:codigo", handlers.GetEstudantePorCodigo)
		protected.GET("/estudantes", middleware.RequireAcademiaOuAdmin(), handlers.ListarEstudantes)
		protected.GET("/avaliacoes", handlers.ListarAvaliacoes)
		protected.GET("/aprovacoes", handlers.ListarAprovacoes)
		protected.GET("/reprovacoes", handlers.ListarReprovacoes)
		protected.GET("/notas", handlers.ListarNotas)
		protected.GET("/faltas", handlers.ListarFaltas)
		protected.GET("/notas-estudante/:codigo", handlers.GetNotasEstudante)
		protected.GET("/faltas-estudante/:codigo", handlers.GetFaltasEstudante)
		protected.GET("/ano-letivo", handlers.GetAnoLetivoGlobalSistemaAtual)
		protected.GET("/anos-letivos-lista", handlers.GetAnosLetivosGlobaisLista)
		protected.GET("/solicitacoes-matricula", middleware.RequireAdmin(), handlers.ListarSolicitacoesMatriculaAdmin)
		protected.GET("/avaliacoes-estudante/:codigo", middleware.RequireAcademiaOuAdmin(), handlers.GetAvaliacoesFinaisEstudante)
		protected.GET("/turmas-estudante/:codigo", handlers.GetTurmasEstudante)
		protected.POST("/adicionar-telefone-extra", handlers.AdicionarTelefoneExtra)
		protected.POST("/definir-ano-letivo-seguinte", middleware.RequireAcademiaOuAdmin(), handlers.DefinirAnoLetivoSeguinte)
	}

	// ── Rotas exclusivas do estudante ─────────────────────────────────────
	estudante := router.Group("/estudante")
	estudante.Use(middleware.AuthMiddleware())
	estudante.Use(middleware.RequireEstudante())
	{
		estudante.PUT("/dados-pessoais", handlers.AtualizarDadosPessoais)
		estudante.GET("/minhas-avaliacoes", handlers.GetMinhasAvaliacoes)
		estudante.GET("/categorias-nota", handlers.ListarCategoriasNota)
	}

	// ── Rotas de academia ─────────────────────────────────────────────────
	academiaRead := router.Group("/academia")
	academiaRead.Use(middleware.AuthMiddleware())
	academiaRead.Use(middleware.RequireAcademiaOuAdmin())
	academiaRead.Use(middleware.ValidarStatusAcademia())
	{
		academiaRead.GET("/materias", handlers.ListarMaterias)
		academiaRead.GET("/materia/:id", handlers.GetMateria)
		academiaRead.GET("/turmas", handlers.ListarTurmasAcademia)
		academiaRead.GET("/turma/:codigo", handlers.GetTurma)
		academiaRead.GET("/ano-letivo", handlers.GetAnoLetivoAcademia)
		academiaRead.GET("/anos-letivos-lista", handlers.GetAnosLetivosListaAcademia)
		academiaRead.GET("/categorias-nota", handlers.ListarCategoriasNota)
		academiaRead.GET("/avaliacao-final/regras", handlers.ListarRegrasAvaliacaoFinal)
		academiaRead.GET("/solicitacoes-matricula", handlers.ListarSolicitacoesMatriculaAcademia)
		academiaRead.GET("/solicitacao-matricula/:codigo", handlers.GetSolicitacaoMatriculaAcademia)
	}

	academia := router.Group("/academia")
	academia.Use(middleware.AuthMiddleware())
	academia.Use(middleware.RequireAcademia())
	academia.Use(middleware.ValidarStatusAcademia())
	{
		academia.PUT("/dados", handlers.AtualizarDadosAcademia)
		academia.POST("/ano-letivo", handlers.DefinirAnoLetivoAcademia)
		academia.POST("/definir-ano-letivo", handlers.DefinirAnoLetivoAcademia)
		academia.PUT("/solicitacao-matricula/:codigo/aprovar", handlers.AprovarSolicitacaoMatricula)
		academia.PUT("/solicitacao-matricula/:codigo/reprovar", handlers.ReprovarSolicitacaoMatricula)

		academia.POST("/estudante/register", handlers.RegisterEstudantePorAcademia)
		academia.POST("/notas-aluno", handlers.RegistrarNota)
		academia.PUT("/atualizar-nota", handlers.AtualizarNota)
		academia.DELETE("/nota/:id", handlers.DeletarNota)
		academia.POST("/faltas-aluno", handlers.RegistrarFaltas)
		academia.PUT("/atualizar-falta", handlers.AtualizarFalta)
		academia.DELETE("/falta/:id", handlers.DeletarFalta)
		// Avaliação final é acionada automaticamente pelo registro de notas
		academia.POST("/avaliacao-final/regras", handlers.CriarRegraAvaliacaoFinal)
		academia.PUT("/avaliacao-final/regras/:id", handlers.EditarRegraAvaliacaoFinal)
		academia.DELETE("/avaliacao-final/regras/:id", handlers.DeletarRegraAvaliacaoFinal)
		academia.POST("/categorias-nota", handlers.CriarCategoriaNota)
		academia.DELETE("/categorias-nota/:codigo", handlers.DeletarCategoriaNota)
		academia.POST("/estudante/:codigo/matricula/fundamental", handlers.MatricularFundamentalHandler)
		academia.POST("/estudante/:codigo/matricula/medio", handlers.MatricularMedioHandler)
		academia.POST("/estudante/:codigo/matricula/superior", handlers.MatricularSuperiorHandler)
		academia.POST("/estudante/:codigo/interrupcao/fundamental", handlers.InterromperFundamentalHandler)
		academia.POST("/estudante/:codigo/interrupcao/medio", handlers.InterromperMedioHandler)
		academia.POST("/estudante/:codigo/trancamento/superior", handlers.TrancarSuperiorHandler)
		academia.POST("/estudante/:codigo/desvincular", handlers.DesvincularEstudanteHandler)
		academia.POST("/estudante/:codigo/revincular", handlers.ReintegrarEstudanteHandler)

		// ── Cursos ────────────────────────────────────────────────────────
		academia.POST("/curso", handlers.CriarCurso)
		academia.PUT("/curso/:id/ativar", handlers.AtivarCurso)
		academia.PUT("/curso/:id/desativar", handlers.DesativarCurso)
		academia.PUT("/curso/:id/dados", handlers.AtualizarDadosCurso)
		academia.DELETE("/curso/:id", handlers.DeletarCurso)

		// ── Matérias ──────────────────────────────────────────────────────
		academia.POST("/materia", handlers.CriarMateria)
		academia.PUT("/materia/:id/ativar", handlers.AtivarMateria)
		academia.PUT("/materia/:id/desativar", handlers.DesativarMateria)
		academia.PUT("/materia/:id/periodo", handlers.DefinirPeriodoMateria)
		academia.PUT("/materia/:id/dados", handlers.AtualizarDadosMateria)
		academia.DELETE("/materia/:id", handlers.DeletarMateria)

		// ── Turmas ────────────────────────────────────────────────────────
		academia.POST("/turma", handlers.CriarTurma)
		academia.PUT("/turma/:codigo/ativar", handlers.AtivarTurma)
		academia.PUT("/turma/:codigo/desativar", handlers.DesativarTurma)
		academia.PUT("/turma/:codigo/dados", handlers.AtualizarDadosTurma)
		academia.DELETE("/turma/:codigo", handlers.DeletarTurma)
		academia.POST("/turma/:codigo/estudante", handlers.AdicionarEstudanteATurma)
		academia.DELETE("/turma/:codigo/estudantes/:codigo_estudante", handlers.RemoverEstudanteDaTurma)

		// ── Batch assíncronos (submissão de jobs de longa duração) ────────
		academia.POST("/estudante/register/async", handlers.RegisterEstudanteBatchAsync)
		academia.POST("/notas-aluno/async", handlers.RegistrarNotaBatchAsync)
		academia.PUT("/atualizar-nota/async", handlers.AtualizarNotaBatchAsync)
		academia.DELETE("/nota/async", handlers.DeletarNotaBatchAsync)
		academia.POST("/faltas-aluno/async", handlers.RegistrarFaltasBatchAsync)
		academia.PUT("/atualizar-falta/async", handlers.AtualizarFaltaBatchAsync)
		academia.DELETE("/falta/async", handlers.DeletarFaltaBatchAsync)
		academia.POST("/curso/async", handlers.CriarCursoBatchAsync)
		academia.POST("/materia/async", handlers.CriarMateriaBatchAsync)
		academia.POST("/turma/async", handlers.CriarTurmaBatchAsync)
		academia.POST("/turma/estudante/async", handlers.AdicionarEstudanteBatchAsync)
		academia.PUT("/dados/async", handlers.AtualizarDadosAcademiaBatchAsync)
		academia.POST("/categorias-nota/async", handlers.CriarCategoriaNotaBatchAsync)
		academia.DELETE("/categorias-nota/async", handlers.DeletarCategoriaNotaBatchAsync)
		academia.PUT("/curso/ativar/async", handlers.AtivarCursoBatchAsync)
		academia.PUT("/curso/desativar/async", handlers.DesativarCursoBatchAsync)
		academia.PUT("/curso/dados/async", handlers.AtualizarDadosCursoBatchAsync)
		academia.DELETE("/curso/async", handlers.DeletarCursoBatchAsync)
		academia.PUT("/materia/ativar/async", handlers.AtivarMateriaBatchAsync)
		academia.PUT("/materia/desativar/async", handlers.DesativarMateriaBatchAsync)
		academia.PUT("/materia/periodo/async", handlers.DefinirPeriodoMateriaBatchAsync)
		academia.PUT("/materia/dados/async", handlers.AtualizarDadosMateriaBatchAsync)
		academia.DELETE("/materia/async", handlers.DeletarMateriaBatchAsync)
		academia.PUT("/turma/ativar/async", handlers.AtivarTurmaBatchAsync)
		academia.PUT("/turma/desativar/async", handlers.DesativarTurmaBatchAsync)
		academia.PUT("/turma/dados/async", handlers.AtualizarDadosTurmaBatchAsync)
		academia.DELETE("/turma/async", handlers.DeletarTurmaBatchAsync)
		academia.DELETE("/turma/estudante/async", handlers.RemoverEstudanteTurmaBatchAsync)
	}

	// ── Rotas de admin ────────────────────────────────────────────────────
	admin := router.Group("/dominis")
	admin.Use(middleware.AuthMiddleware())
	admin.Use(middleware.RequireAdmin())
	{
		admin.POST("/register", middleware.RequireFPP(), handlers.RegisterAdmin)
		admin.POST("/academia/register", middleware.RequireFPP(), handlers.RegisterAcademia)
		admin.PUT("/academia/:codigo/ativar", middleware.RequireAdm(), handlers.AtivarAcademia)
		admin.PUT("/academia/:codigo/desativar", middleware.RequireAdm(), handlers.DesativarAcademia)
		admin.PUT("/admin/:id/ativar", middleware.RequireAdm(), handlers.AtivarAdmin)
		admin.PUT("/admin/:id/desativar", middleware.RequireAdm(), handlers.DesativarAdmin)
		admin.GET("/admin-lista", handlers.ListarTodosAdmins)
		admin.GET("/metrics", handlers.GetSystemMetrics)
		admin.GET("/storage/quota", handlers.GetStorageQuota)
		admin.GET("/solicitacoes-matricula", handlers.ListarSolicitacoesMatriculaAdmin)
		admin.POST("/projections/rebuild/:name", middleware.RequireFPP(), handlers.RebuildProjection)
		admin.POST("/projections/rebuild/:name/async", middleware.RequireFPP(), handlers.RebuildProjectionAsync)
		admin.GET("/consultar-admin/:email", handlers.GetAdminPorEmail)
		admin.PUT("/admin/:id/role", middleware.RequireFPP(), handlers.AtualizarRoleAdmin)
		admin.PUT("/admin/:id/dados", handlers.AtualizarDadosAdmin)

		// ── Batch assíncronos (admin) ─────────────────────────────────────
		admin.POST("/academia/register/async", middleware.RequireFPP(), handlers.RegisterAcademiaBatchAsync)
		admin.PUT("/academia/ativar/async", middleware.RequireAdm(), handlers.AtivarAcademiaBatchAsync)
		admin.PUT("/academia/desativar/async", middleware.RequireAdm(), handlers.DesativarAcademiaBatchAsync)
		admin.PUT("/admin/ativar/async", middleware.RequireAdm(), handlers.AtivarAdminBatchAsync)
		admin.PUT("/admin/desativar/async", middleware.RequireAdm(), handlers.DesativarAdminBatchAsync)
	}

	// Configurações globais do painel administrativo.
	adminSistema := router.Group("/admin")
	adminSistema.Use(middleware.AuthMiddleware())
	adminSistema.Use(middleware.RequireAdmin())
	{
		adminSistema.POST("/sistema/ano-letivo", middleware.RequireFPP(), handlers.DefinirAnoLetivoGlobalSistema)
		adminSistema.POST("/definir-ano-letivo-geral", middleware.RequireFPP(), handlers.DefinirAnoLetivoGlobalSistema)
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
			allowed = true
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
