// ============================================================================
// ARQUIVO: internal/handlers/helpers.go
//
// Funções auxiliares compartilhadas por todos os handlers do pacote.
// ============================================================================

package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"spuri/internal/db"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// Repositório e cliente de banco
// ============================================================================

func getRepository(c *gin.Context) *db.AggregateRepository {
	repo, _ := c.Get("repository")
	return repo.(*db.AggregateRepository)
}

func getDbClient(c *gin.Context) *db.Client {
	client, exists := c.Get("dbClient")
	if !exists {
		log.Printf("⚠️ Cliente BD não encontrado no contexto, criando novo")
		config := db.DefaultConfig()
		newClient, _ := db.NewClient(config)
		return newClient
	}
	return client.(*db.Client)
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
// Projeções
// ============================================================================

func getAdminProjection(c *gin.Context) *projections.AdminProjection {
	return projections.NewAdminProjection(getDbClient(c))
}

func getEstudanteProjection(c *gin.Context) *projections.EstudanteProjection {
	return projections.NewEstudanteProjection(getDbClient(c))
}

func getAcademiaProjection(c *gin.Context) *projections.AcademiaProjection {
	return projections.NewAcademiaProjection(getDbClient(c))
}

func getNotasProjection(c *gin.Context) *projections.NotasProjection {
	return projections.NewNotasProjection(getDbClient(c))
}

func getFaltasProjection(c *gin.Context) *projections.FaltasProjection {
	return projections.NewFaltasProjection(getDbClient(c))
}

func getCursosProjection(c *gin.Context) *projections.CursosProjection {
	return projections.NewCursosProjection(getDbClient(c))
}

func getMateriasProjection(c *gin.Context) *projections.MateriasProjection {
	return projections.NewMateriasProjection(getDbClient(c))
}

func getAprovacaoAnoProjection(c *gin.Context) *projections.AprovacaoAnoProjection {
	return projections.NewAprovacaoAnoProjection(getDbClient(c))
}

func getCategoriasNotaProjection(c *gin.Context) *projections.CategoriasNotaProjection {
	return projections.NewCategoriasNotaProjection(getDbClient(c))
}

func getAvaliacaoFinalProjection(c *gin.Context) *projections.AvaliacaoFinalProjection {
	return projections.NewAvaliacaoFinalProjection(getDbClient(c))
}

// ============================================================================
// Serviço de email
// ============================================================================

func getEmailService(c *gin.Context) *services.EmailService {
	// O EmailService é criado on-demand a partir do dbClient — não vive no contexto Gin.
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