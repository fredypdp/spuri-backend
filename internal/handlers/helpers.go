package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/services"
)

// ============================================================================
// Repositório e cliente de banco
// ============================================================================

func getRepository(c *gin.Context) *db.AggregateRepository {
	repo, _ := c.Get("repository")
	base := repo.(*db.AggregateRepository)
	if c != nil && c.Request != nil {
		return base.WithContext(c.Request.Context())
	}
	return base
}

// getDbClient retorna o *db.Client injetado pelo middleware de setup.
//
// H4-14: se o client não estiver no contexto (bug de configuração do router),
// a requisição é abortada com 500 e nil NÃO é retornado — prevenindo panic.
// Criar um novo pool de conexões por requisição seria vazamento de recursos;
// o client deve sempre vir do contexto injetado em setupRouter.
func getDbClient(c *gin.Context) *db.Client {
	client, exists := c.Get("dbClient")
	if !exists {
		log.Printf("❌ [getDbClient] dbClient ausente no contexto Gin — abortando requisição. Path: %s", c.Request.URL.Path)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "erro interno: cliente de banco de dados não disponível",
		})
		return nil
	}
	dbCli, ok := client.(*db.Client)
	if !ok || dbCli == nil {
		log.Printf("❌ [getDbClient] dbClient no contexto não é *db.Client válido. Path: %s", c.Request.URL.Path)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error": "erro interno: cliente de banco de dados inválido",
		})
		return nil
	}
	return dbCli
}

// getDbClientFromContext é alias de getDbClient — mantido por compatibilidade.
func getDbClientFromContext(c *gin.Context) *db.Client {
	return getDbClient(c)
}

func getProjManager(c *gin.Context) *projections.Manager {
	raw, _ := c.Get("projManager")
	return raw.(*projections.Manager)
}

// ============================================================================
// Helpers de projeção
//
// Cada função instancia a projeção directamente via New...Projection(dbClient).
// O Manager não expõe GetProjection — as projeções não são recuperadas pelo
// Manager nos handlers; apenas RegisterProjection e StartProcessing são usados.
// ============================================================================

func getAdminProjection(c *gin.Context) *projections.AdminProjection {
	return projections.NewAdminProjection(getDbClient(c))
}

func getAcademiaProjection(c *gin.Context) *projections.AcademiaProjection {
	return projections.NewAcademiaProjection(getDbClient(c))
}

func getEstudanteProjection(c *gin.Context) *projections.EstudanteProjection {
	return projections.NewEstudanteProjection(getDbClient(c))
}

func getCursosProjection(c *gin.Context) *projections.CursosProjection {
	return projections.NewCursosProjection(getDbClient(c))
}

func getMateriasProjection(c *gin.Context) *projections.MateriasProjection {
	return projections.NewMateriasProjection(getDbClient(c))
}

func getNotasProjection(c *gin.Context) *projections.NotasProjection {
	return projections.NewNotasProjection(getDbClient(c))
}

func getFaltasProjection(c *gin.Context) *projections.FaltasProjection {
	return projections.NewFaltasProjection(getDbClient(c))
}

func getAvaliacaoFinalProjection(c *gin.Context) *projections.AvaliacaoFinalProjection {
	return projections.NewAvaliacaoFinalProjection(getDbClient(c))
}

func getTurmasProjection(c *gin.Context) *projections.TurmasProjection {
	return projections.NewTurmasProjection(getDbClient(c))
}

func getCategoriasNotaProjection(c *gin.Context) *projections.CategoriasNotaProjection {
	return projections.NewCategoriasNotaProjection(getDbClient(c))
}

func getEmailService(c *gin.Context) *services.EmailService {
	return services.NewEmailService(getDbClient(c).DB())
}

// ============================================================================
// Helpers de permissão admin
// ============================================================================

// verificarPermissaoAdmin verifica se o admin autenticado tem role >= minRole.
// Usado por handlers que precisam de autorização adicional além do middleware.
func verificarPermissaoAdmin(c *gin.Context, minRole string) error {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		return fmt.Errorf("usuário não autenticado")
	}
	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByID(userID)
	if err != nil || admin == nil {
		return fmt.Errorf("administrador não encontrado")
	}
	if admin.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}
	hierarchy := map[string]int{"fpp": 3, "adm": 2, "gerente": 1}
	if hierarchy[admin.Role] < hierarchy[minRole] {
		return fmt.Errorf("permissão negada: requer role '%s' ou superior", minRole)
	}
	return nil
}

// ============================================================================
// Helper de auditoria de ações admin
// ============================================================================

// registrarAcaoAdmin persiste uma ação administrativa no aggregate Admin
// via event sourcing. Falhas são apenas logadas — não abortam o handler
// principal, pois a operação primária já foi concluída com sucesso.
//
// Assinatura compatível com os handlers que chamam:
//
//	registrarAcaoAdmin(c, adminUserID, "acao", map[string]interface{}{...})
func registrarAcaoAdmin(c *gin.Context, adminID uuid.UUID, acao string, detalhes map[string]interface{}) {
	repository := getRepository(c)

	agg, err := repository.Load(adminID, "Admin")
	if err != nil {
		log.Printf("[WARN] registrarAcaoAdmin: falha ao carregar admin %s: %v", adminID, err)
		return
	}
	admin, ok := agg.(*aggregates.Admin)
	if !ok {
		log.Printf("[WARN] registrarAcaoAdmin: tipo de aggregate inesperado para admin %s", adminID)
		return
	}

	if err := admin.RegistrarAcao(acao, detalhes); err != nil {
		log.Printf("[WARN] registrarAcaoAdmin: falha ao registar ação '%s' para admin %s: %v", acao, adminID, err)
		return
	}

	audit := db.AuditContext{
		UserID:   adminID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(admin, audit); err != nil {
		log.Printf("[WARN] registrarAcaoAdmin: falha ao persistir ação '%s' para admin %s: %v", acao, adminID, err)
	}
}

// ============================================================================
// Helpers de SQL
// ============================================================================

// getNullString converte sql.NullString para interface{} (nil se inválido).
// Usado em scans de queries que retornam colunas nullable.
func getNullString(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

// getNullUUID converte sql.NullString contendo UUID para interface{}.
func getNullUUID(ns sql.NullString) interface{} {
	if ns.Valid && ns.String != "" {
		if id, err := uuid.Parse(ns.String); err == nil {
			return id.String()
		}
	}
	return nil
}

// jsonToMap faz unmarshal de uma coluna JSON nullable para map.
// Retorna nil se o valor for NULL ou vazio — sem panic.
func jsonToMap(raw *string) map[string]interface{} {
	if raw == nil || *raw == "" {
		return nil
	}
	var m map[string]interface{}
	_ = json.Unmarshal([]byte(*raw), &m)
	return m
}

// ============================================================================
// Helpers de string
// ============================================================================

// maskEmail ofusca um endereço de email para exibição segura.
// Ex: "joao.silva@gmail.com" → "j*******a@gmail.com"
func maskEmail(email string) string {
	for i, ch := range email {
		if ch == '@' {
			local := email[:i]
			domain := email[i:]
			if len(local) <= 2 {
				return "**" + domain
			}
			return string(local[0]) + repeat("*", len(local)-2) + string(local[len(local)-1]) + domain
		}
	}
	return "***@***"
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
