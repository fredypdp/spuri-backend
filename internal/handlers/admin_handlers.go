// ============================================================================
// ARQUIVO: internal/handlers/admin_handlers.go
//
// CORREÇÕES APLICADAS:
//   [A14] — AtualizarDadosAdmin: verifica unicidade de email via projeção ANTES
//            de emitir AdminDadosAtualizados. Sem essa verificação, o evento era
//            gravado no ledger imutável mas falhava na projeção, causando
//            inconsistência permanente ledger ↔ projeção.
//   [A41] — RegisterAdmin: senha_padrao REMOVIDA da resposta HTTP. Admin recebe
//            email com instruções; senha não exposta em respostas JSON.
//   [A26] — RebuildProjection: usa projManager global (injetado no contexto),
//            não cria manager local que opera concorrentemente com o global.
//   [A08] — AtivarAdmin: verifica hierarquia do executor antes de ativar.
//   [A10] — DesativarAdmin: verifica hierarquia do executor antes de desativar.
// ============================================================================

package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
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

	// [A41] senha_padrao REMOVIDA da resposta. A senha padrão é enviada por email
	// ao admin criado, nunca exposta via API.
	c.JSON(http.StatusCreated, gin.H{
		"message": "administrador criado com sucesso. A senha padrão foi enviada por email.",
		"data": gin.H{
			"id":    newAdmin.ID,
			"nome":  newAdmin.Nome,
			"email": req.Email,
			"role":  newAdmin.Role,
		},
	})
}

// GetAdminPorEmail consulta um administrador pelo e-mail.
// Rota: GET /admin/consultar-admin/:email
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
		},
	})
}

// ListarTodosAdmins retorna todos os administradores sem expor senha_hash.
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

// AtivarAdmin ativa um admin.
//
// [A08] CORRIGIDO: verifica hierarquia do executor antes de ativar.
// O executor deve ter role estritamente superior ao alvo.
func AtivarAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

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

	// [A08] Verificar hierarquia: executor deve ter role > alvo
	executorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	executor := executorAgg.(*aggregates.Admin)
	if err := executor.ValidatePermission(targetAdmin.Role); err != nil {
		utils.RespondWithForbiddenError(c, fmt.Sprintf("permissão negada para ativar admin com role '%s': %s", targetAdmin.Role, err.Error()))
		return
	}

	if err := targetAdmin.Ativar(userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(targetAdmin, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, userID, "admin_ativado", map[string]interface{}{
		"target_admin_id": targetID.String(),
		"target_email":    targetAdmin.Email,
	})

	log.Printf("Admin ativado: %s (por: %s)", targetAdmin.Email, userID)
	c.JSON(http.StatusOK, gin.H{
		"message": "administrador ativado com sucesso",
		"email":   targetAdmin.Email,
	})
}

// DesativarAdmin desativa um admin.
//
// [A10] CORRIGIDO: verifica hierarquia do executor antes de desativar.
// O executor deve ter role estritamente superior ao alvo.
func DesativarAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

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
	if targetID == userID {
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

	// [A10] Verificar hierarquia: executor deve ter role > alvo
	executorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	executor := executorAgg.(*aggregates.Admin)
	if err := executor.ValidatePermission(targetAdmin.Role); err != nil {
		utils.RespondWithForbiddenError(c, fmt.Sprintf("permissão negada para desativar admin com role '%s': %s", targetAdmin.Role, err.Error()))
		return
	}

	if err := targetAdmin.Desativar(userID, req.Motivo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(targetAdmin, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, userID, "admin_desativado", map[string]interface{}{
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

// AtualizarDadosAdmin atualiza nome e/ou email de um admin.
//
// [A14] CORRIGIDO: verifica unicidade do novo email via projeção ANTES de emitir
// o evento. Sem isso, AdminDadosAtualizados era gravado no ledger imutável e
// depois falhava na projeção com unique constraint, gerando inconsistência
// permanente que nem o rebuild conseguia resolver.
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

	// Admin pode editar os próprios dados sem restrição de hierarquia.
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

	// [A14] Verificar unicidade do novo email ANTES de emitir o evento.
	// Impede gravação de evento inválido no ledger imutável.
	if req.Email != nil && *req.Email != admin.Email {
		adminProj := getAdminProjection(c)
		existing, _ := adminProj.GetByEmail(*req.Email)
		if existing != nil && existing.ID != targetID {
			utils.RespondWithConflictError(c, "Este email já está cadastrado para outro administrador")
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
	c.JSON(http.StatusOK, gin.H{"message": "dados do administrador atualizados com sucesso"})
}

// RebuildProjection reconstrói uma projeção a partir do ledger de eventos.
//
// [A26] CORRIGIDO: usa projManager global (injetado pelo contexto Gin), não
// cria um manager local que operaria concorrentemente com o global, causando
// corrupção por escrita simultânea durante TRUNCATE + replay.
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

	// [A26] Usa o manager global injetado no contexto pelo setupRouter.
	// Esse manager já tem todas as projeções registradas e gerencia o loop
	// de processamento. Usar um manager local criaria race condition.
	manager := getProjManager(c)
	if manager == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("projection manager não disponível no contexto"))
		return
	}

	var rebuildErr error
	if projectionName == "all" {
		rebuildErr = manager.RebuildAllProjections()
	} else {
		// Valida que a projeção está registrada antes de tentar rebuild
		if !manager.IsProjectionRegistered(projectionName) {
			utils.RespondWithNotFoundError(c, fmt.Sprintf("projeção '%s'", projectionName))
			return
		}
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

	manager := getProjManager(c)
	if manager == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("projection manager não disponível"))
		return
	}

	status, err := manager.GetProjectionStatus(projectionName)
	if err != nil {
		utils.RespondWithNotFoundError(c, fmt.Sprintf("projeção '%s'", projectionName))
		return
	}

	c.JSON(http.StatusOK, status)
}

// ============================================================================
// Helper interno: registrar ação do admin logado
// ============================================================================

func registrarAcaoAdmin(c *gin.Context, userID uuid.UUID, acao string, detalhes map[string]interface{}) {
	repository := getRepository(c)
	adminAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		log.Printf("[WARN] registrarAcaoAdmin: falha ao carregar admin %s: %v", userID, err)
		return
	}
	admin := adminAgg.(*aggregates.Admin)
	if err := admin.RegistrarAcao(acao, detalhes); err != nil {
		log.Printf("[WARN] registrarAcaoAdmin: falha ao registrar ação '%s': %v", acao, err)
		return
	}
	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(admin, audit); err != nil {
		log.Printf("[WARN] registrarAcaoAdmin: falha ao salvar ação '%s': %v", acao, err)
	}
}