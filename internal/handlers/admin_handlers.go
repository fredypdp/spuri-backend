// ============================================================================
// ARQUIVO: internal/handlers/admin_handlers.go
// CORREÇÃO BUG #4: Removida ConsultarAdmin (dead code — duplicava GetAdminPorEmail
//                  e não estava registrada em nenhuma rota).
// CORREÇÃO BUG #5: ListarTodosAdmins agora verifica erros de json.Marshal/Unmarshal.
//                  Antes: se marshal falhasse, adminMap seria nil e delete(nil,...)
//                  causaria panic em runtime.
// CORREÇÃO BUG #6: Removida linha `_ = http.StatusOK` de AtualizarDadosAdmin
//                  (inútil — http.StatusOK já é usado via c.JSON acima dela).
// ============================================================================

package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/services"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func LoginAdmin(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
		Senha string `json:"senha" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("email e senha são obrigatórios"))
		return
	}

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmailForLogin(req.Email)
	if err != nil || admin == nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	if admin.Status != "ativo" {
		utils.RespondWithForbiddenError(c, "Administrador inativo. Entre em contato com o suporte.")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.SenhaHash), []byte(req.Senha)); err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	token, err := middleware.GenerateToken(admin.ID, "admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Login admin bem-sucedido: %s (%s)", admin.Nome, admin.Role)
	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"nome":  admin.Nome,
		"role":  admin.Role,
		"type":  "admin",
	})
}

func RegisterAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome  string `json:"nome" binding:"required"`
		Email string `json:"email" binding:"required"`
		Role  string `json:"role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: nome, email e role"))
		return
	}

	if req.Role != "fpp" && req.Role != "adm" && req.Role != "gerente" {
		utils.RespondWithValidationError(c, fmt.Errorf("role deve ser 'fpp', 'adm' ou 'gerente'"))
		return
	}

	adminProj := getAdminProjection(c)
	creatorAdmin, err := adminProj.GetByID(userID)
	if err != nil || creatorAdmin == nil {
		utils.RespondWithForbiddenError(c, "Administrador não encontrado")
		return
	}

	repository := getRepository(c)
	creatorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	creator := creatorAgg.(*aggregates.Admin)

	if err := creator.ValidatePermission(req.Role); err != nil {
		utils.RespondWithForbiddenError(c, err.Error())
		return
	}

	existing, _ := adminProj.GetByEmail(req.Email)
	if existing != nil {
		utils.RespondWithConflictError(c, "Email já cadastrado no sistema")
		return
	}

	defaultPassword := services.GetDefaultPassword("admin", req.Role)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	newAdmin := aggregates.NewAdmin()
	if err := newAdmin.Criar(req.Nome, req.Email, string(hashedPassword), req.Role, &userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(newAdmin, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if err := creator.RegistrarAcao("admin_criado", map[string]interface{}{
		"novo_admin_id": newAdmin.ID.String(),
		"role":          req.Role,
		"email":         req.Email,
	}); err != nil {
		log.Printf("[WARN] Falha ao preparar ação do criador: %v", err)
	} else if err := repository.SaveWithAudit(creator, audit); err != nil {
		log.Printf("[WARN] Falha ao registrar ação do criador (admin_criado): %v", err)
	}

	log.Printf("Admin criado: %s (%s) por %s", req.Email, req.Role, creatorAdmin.Nome)
	c.JSON(http.StatusCreated, gin.H{
		"message": "administrador criado com sucesso",
		"data": gin.H{
			"id":           newAdmin.ID,
			"nome":         newAdmin.Nome,
			"email":        req.Email,
			"role":         newAdmin.Role,
			"senha_padrao": defaultPassword,
		},
	})
}

// GetAdminPorEmail consulta um administrador pelo e-mail.
// Rota: GET /admin/consultar-admin/:email
// CORRIGIDO BUG #4: ConsultarAdmin (dead code) removida — esta é a única função
// de busca por email, registrada na rota acima.
func GetAdminPorEmail(c *gin.Context) {
	email := c.Param("email")

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(email)
	if err != nil || admin == nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"admin": gin.H{
			"id":                     admin.ID,
			"nome":                   admin.Nome,
			"email":                  admin.Email,
			"email_verificado":       admin.EmailVerificado,
			"role":                   admin.Role,
			"status":                 admin.Status,
			"created_by":             admin.CreatedBy,
			"created_at":             admin.CreatedAt,
			"updated_at":             admin.UpdatedAt,
			"total_acoes_realizadas": admin.TotalAcoesRealizadas,
			"version":                admin.Version,
		},
	})
}

// ListarTodosAdmins retorna todos os administradores sem expor senha_hash.
// CORRIGIDO BUG #5: erros de json.Marshal/Unmarshal agora verificados.
// Antes: se marshal falhasse, adminMap era nil e delete(nil,...) causava panic.
func ListarTodosAdmins(c *gin.Context) {
	adminProj := getAdminProjection(c)
	admins, err := adminProj.GetAll()
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	var adminsResponse []map[string]interface{}
	for _, admin := range admins {
		adminBytes, err := json.Marshal(admin)
		if err != nil {
			log.Printf("[WARN] ListarTodosAdmins: falha ao serializar admin %s: %v", admin.ID, err)
			continue
		}
		var adminMap map[string]interface{}
		if err := json.Unmarshal(adminBytes, &adminMap); err != nil {
			log.Printf("[WARN] ListarTodosAdmins: falha ao desserializar admin %s: %v", admin.ID, err)
			continue
		}
		// SenhaHash tem tag json:"-" então já não aparece no marshal,
		// mas o delete é mantido como proteção defensiva explícita.
		delete(adminMap, "senha_hash")
		adminsResponse = append(adminsResponse, adminMap)
	}

	c.JSON(http.StatusOK, gin.H{
		"admins": adminsResponse,
		"total":  len(adminsResponse),
	})
}

func AtivarAdmin(c *gin.Context) {
	userID, _ := c.Get("user_id")
	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de administrador inválido"))
		return
	}

	repository := getRepository(c)
	targetAdminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	targetAdmin := targetAdminAgg.(*aggregates.Admin)
	if err := targetAdmin.Ativar(userID.(uuid.UUID)); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.(uuid.UUID).String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(targetAdmin, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, userID.(uuid.UUID), "admin_ativado", map[string]interface{}{
		"target_admin_id": targetID.String(),
		"target_email":    targetAdmin.Email,
	})

	log.Printf("Admin ativado: %s (por: %s)", targetAdmin.Email, userID)
	c.JSON(http.StatusOK, gin.H{
		"message": "administrador ativado com sucesso",
		"email":   targetAdmin.Email,
	})
}

func DesativarAdmin(c *gin.Context) {
	userID, _ := c.Get("user_id")
	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de administrador inválido"))
		return
	}

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}

	// Proteção contra auto-desativação
	if targetID == userID.(uuid.UUID) {
		utils.RespondWithValidationError(c, fmt.Errorf("você não pode desativar sua própria conta"))
		return
	}

	repository := getRepository(c)
	targetAdminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	targetAdmin := targetAdminAgg.(*aggregates.Admin)
	if err := targetAdmin.Desativar(userID.(uuid.UUID), req.Motivo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.(uuid.UUID).String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(targetAdmin, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, userID.(uuid.UUID), "admin_desativado", map[string]interface{}{
		"target_admin_id": targetID.String(),
		"target_email":    targetAdmin.Email,
		"motivo":          req.Motivo,
	})

	log.Printf("Admin desativado: %s - Motivo: %s (por: %s)", targetAdmin.Email, req.Motivo, userID)
	c.JSON(http.StatusOK, gin.H{
		"message": "administrador desativado com sucesso",
		"email":   targetAdmin.Email,
	})
}

func AtualizarRoleAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	adminID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de admin inválido"))
		return
	}

	var req struct {
		NovoRole string `json:"novo_role" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campo obrigatório: novo_role"))
		return
	}

	adminProj := getAdminProjection(c)
	currentAdmin, err := adminProj.GetByID(userID)
	if err != nil || currentAdmin == nil {
		utils.RespondWithNotFoundError(c, "admin executor")
		return
	}

	repository := getRepository(c)
	adminAgg, err := repository.Load(adminID, "Admin")
	if err != nil {
		utils.RespondWithNotFoundError(c, "admin")
		return
	}
	admin := adminAgg.(*aggregates.Admin)
	roleAnterior := admin.Role

	if err := admin.AtualizarRole(req.NovoRole, userID, currentAdmin.Role); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(admin, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Role atualizado: %s -> %s (Admin: %s)", roleAnterior, req.NovoRole, admin.Email)
	c.JSON(http.StatusOK, gin.H{
		"message":       "role atualizado com sucesso",
		"role_anterior": roleAnterior,
		"novo_role":     req.NovoRole,
	})
}

func AtualizarDadosAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de administrador inválido"))
		return
	}

	var req struct {
		Nome  *string `json:"nome"`
		Email *string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	repository := getRepository(c)
	adminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}
	admin := adminAgg.(*aggregates.Admin)

	// Admin pode editar os próprios dados sem restrição.
	// Para editar outro admin, precisa de role estritamente superior ao alvo.
	if userID != targetID {
		executorAgg, err := repository.Load(userID, "Admin")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		executor := executorAgg.(*aggregates.Admin)
		if err := executor.ValidatePermission(admin.Role); err != nil {
			utils.RespondWithForbiddenError(c, fmt.Sprintf("permissão negada: %s", err.Error()))
			return
		}
	}

	if err := admin.AtualizarDados(req.Nome, req.Email, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(admin, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Dados do admin atualizados: %s (por: %s)", admin.Email, userID)
	// CORRIGIDO BUG #6: removida linha `_ = http.StatusOK` que era inútil
	c.JSON(http.StatusOK, gin.H{"message": "dados do administrador atualizados com sucesso"})
}

// RebuildProjection reconstrói uma projeção a partir do ledger de eventos.
func RebuildProjection(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByID(userID)
	if err != nil || admin == nil {
		utils.RespondWithForbiddenError(c, "Apenas administradores podem reconstruir projeções")
		return
	}

	if admin.Role != "fpp" && admin.Role != "adm" {
		utils.RespondWithForbiddenError(c, "Apenas FPP ou ADM podem reconstruir projeções")
		return
	}

	projectionName := c.Param("name")
	if projectionName == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nome da projeção não fornecido"))
		return
	}

	client := getDbClient(c)
	manager := projections.NewManager(client)

	manager.RegisterProjection("estudantes", projections.NewEstudanteProjection(client))
	manager.RegisterProjection("academias", projections.NewAcademiaProjection(client))
	manager.RegisterProjection("admins", projections.NewAdminProjection(client))
	manager.RegisterProjection("notas", projections.NewNotasProjection(client))
	manager.RegisterProjection("faltas", projections.NewFaltasProjection(client))
	manager.RegisterProjection("cursos", projections.NewCursosProjection(client))
	manager.RegisterProjection("materias", projections.NewMateriasProjection(client))
	manager.RegisterProjection("sistema_config", projections.NewSistemaConfigProjection(client))
	manager.RegisterProjection("turmas", projections.NewTurmasProjection(client))
	manager.RegisterProjection("avaliacao_final", projections.NewAvaliacaoFinalProjection(client))
	manager.RegisterProjection("categorias_nota", projections.NewCategoriasNotaProjection(client))
	manager.RegisterProjection("aprovacao_ano", projections.NewAprovacaoAnoProjection(client))
	manager.RegisterProjection("reprovacoes", projections.NewReprovacoesProjection(client))

	var rebuildErr error
	if projectionName == "all" {
		rebuildErr = manager.RebuildAllProjections()
	} else {
		rebuildErr = manager.RebuildProjection(projectionName)
	}

	if rebuildErr != nil {
		utils.RespondWithInternalError(c, rebuildErr)
		return
	}

	registrarAcaoAdmin(c, userID, "projection_rebuilt", map[string]interface{}{
		"projection": projectionName,
		"admin_role": admin.Role,
	})

	log.Printf("Projeção %s reconstruída por %s (%s)", projectionName, admin.Nome, admin.Role)
	c.JSON(http.StatusOK, gin.H{
		"message":    "projeção reconstruída com sucesso",
		"projection": projectionName,
		"auditavel":  true,
	})
}

func GetProjectionStatus(c *gin.Context) {
	projectionName := c.Param("name")
	if projectionName == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nome da projeção não fornecido"))
		return
	}

	client := getDbClient(c)

	var proj projections.Projection
	switch projectionName {
	case "estudantes":
		proj = projections.NewEstudanteProjection(client)
	case "academias":
		proj = projections.NewAcademiaProjection(client)
	case "admins":
		proj = projections.NewAdminProjection(client)
	case "notas":
		proj = projections.NewNotasProjection(client)
	case "faltas":
		proj = projections.NewFaltasProjection(client)
	case "cursos":
		proj = projections.NewCursosProjection(client)
	case "materias":
		proj = projections.NewMateriasProjection(client)
	case "sistema_config":
		proj = projections.NewSistemaConfigProjection(client)
	case "turmas":
		proj = projections.NewTurmasProjection(client)
	case "avaliacao_final":
		proj = projections.NewAvaliacaoFinalProjection(client)
	case "categorias_nota":
		proj = projections.NewCategoriasNotaProjection(client)
	case "aprovacao_ano":
		proj = projections.NewAprovacaoAnoProjection(client)
	case "reprovacoes":
		proj = projections.NewReprovacoesProjection(client)
	default:
		utils.RespondWithNotFoundError(c, "projeção")
		return
	}

	lastEventID, err := proj.GetLastProcessedEventID()
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	var lastProcessedAt *string
	var eventsProcessed int
	client.DB().QueryRow( //nolint:errcheck
		`SELECT last_processed_at, events_processed
		 FROM projection_checkpoints
		 WHERE projection_name = $1`,
		projectionName,
	).Scan(&lastProcessedAt, &eventsProcessed)

	status := gin.H{
		"projection_name":        projectionName,
		"last_processed_event":   lastEventID,
		"events_processed_total": eventsProcessed,
	}
	if lastProcessedAt != nil {
		status["last_processed_at"] = *lastProcessedAt
	}
	c.JSON(http.StatusOK, status)
}

func GetAllProjectionsStatus(c *gin.Context) {
	client := getDbClient(c)

	rows, err := client.DB().Query(`
		SELECT projection_name, last_processed_event_id, last_processed_at, events_processed
		FROM projection_checkpoints
		ORDER BY projection_name
	`)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()

	var statuses []gin.H
	for rows.Next() {
		var name string
		var lastEventID int64
		var lastProcessedAt string
		var eventsProcessed int
		if err := rows.Scan(&name, &lastEventID, &lastProcessedAt, &eventsProcessed); err == nil {
			statuses = append(statuses, gin.H{
				"projection_name":      name,
				"last_processed_event": lastEventID,
				"last_processed_at":    lastProcessedAt,
				"events_processed":     eventsProcessed,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"projections": statuses,
		"total":       len(statuses),
	})
}

func GetLedgerStats(c *gin.Context) {
	client := getDbClient(c)

	var totalEvents int64
	var firstEvent, lastEvent string
	client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger`).Scan(&totalEvents)          //nolint:errcheck
	client.DB().QueryRow(`SELECT occurred_at FROM spuri_ledger ORDER BY id ASC LIMIT 1`).Scan(&firstEvent)  //nolint:errcheck
	client.DB().QueryRow(`SELECT occurred_at FROM spuri_ledger ORDER BY id DESC LIMIT 1`).Scan(&lastEvent) //nolint:errcheck

	rows, _ := client.DB().Query(`
		SELECT aggregate_type, COUNT(*) as count
		FROM spuri_ledger
		GROUP BY aggregate_type
		ORDER BY count DESC
	`)
	defer rows.Close()
	aggregateStats := make(map[string]int64)
	for rows.Next() {
		var aggType string
		var count int64
		if err := rows.Scan(&aggType, &count); err == nil {
			aggregateStats[aggType] = count
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_events":        totalEvents,
		"first_event_at":      firstEvent,
		"last_event_at":       lastEvent,
		"events_by_aggregate": aggregateStats,
	})
}

func VerifyAllIntegrity(c *gin.Context) {
	client := getDbClient(c)

	var total, withHash, withPrevious int64
	err := client.DB().QueryRow(`
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE ledger_hash IS NOT NULL) as with_hash,
			COUNT(*) FILTER (WHERE previous_hash IS NOT NULL) as with_previous
		FROM spuri_ledger
	`).Scan(&total, &withHash, &withPrevious)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	integro := (total == withHash) && (total-1 == withPrevious)
	c.JSON(http.StatusOK, gin.H{
		"integro":              integro,
		"total_events":         total,
		"events_with_hash":     withHash,
		"events_with_previous": withPrevious,
		"message": func() string {
			if integro {
				return "✅ Ledger íntegro - todos os eventos possuem hash"
			}
			return "⚠️ Problemas de integridade detectados"
		}(),
	})
}

func getNullString(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func getAdminProjection(c *gin.Context) *projections.AdminProjection {
	return projections.NewAdminProjection(getDbClient(c))
}

func verificarPermissaoAdmin(c *gin.Context, minRole string) error {
	userID, _ := middleware.GetUserID(c)
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

func registrarAcaoAdmin(c *gin.Context, adminID uuid.UUID, acao string, detalhes map[string]interface{}) {
	repository := getRepository(c)
	adminAgg, err := repository.Load(adminID, "Admin")
	if err != nil {
		log.Printf("Erro ao registrar ação '%s': %v", acao, err)
		return
	}
	admin := adminAgg.(*aggregates.Admin)
	admin.RegistrarAcao(acao, detalhes)
	repository.Save(admin)
}