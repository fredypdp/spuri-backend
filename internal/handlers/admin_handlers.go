package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/services"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func DefinirAnoLetivoGlobalSistema(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		AnoLetivo string `json:"ano_letivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campo obrigatório: ano_letivo"))
		return
	}

	anoLetivo := strings.TrimSpace(req.AnoLetivo)
	if !isAnoLetivoValido(anoLetivo) {
		utils.RespondWithValidationError(c, fmt.Errorf("ano_letivo inválido: use o formato YYYY_YYYY com segundo ano = primeiro + 1"))
		return
	}

	client := getDbClient(c)
	if client == nil {
		return
	}

	_, err := client.DB().Exec(`
		INSERT INTO projection_sistema_config (
			chave, valor, ano_letivo_atual, definido_por, updated_at, version
		) VALUES (
			'ano_letivo_atual', $1::text, $1::varchar(20), $2::uuid, NOW(), 1
		)
		ON CONFLICT (chave) DO UPDATE SET
			valor = EXCLUDED.valor,
			ano_letivo_atual = EXCLUDED.ano_letivo_atual,
			definido_por = EXCLUDED.definido_por,
			updated_at = NOW(),
			version = COALESCE(projection_sistema_config.version, 0) + 1
	`, anoLetivo, userID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [DefinirAnoLetivoGlobalSistema] ano_letivo=%s definido por admin=%s", anoLetivo, userID.String())
	c.JSON(http.StatusOK, gin.H{
		"message":    "ano letivo global definido com sucesso",
		"ano_letivo": anoLetivo,
	})
}

func isAnoLetivoValido(v string) bool {
	if len(v) != 9 || v[4] != '_' {
		return false
	}
	var inicio, fim int
	_, err := fmt.Sscanf(v, "%4d_%4d", &inicio, &fim)
	return err == nil && fim == inicio+1
}

func RegisterAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome  string `json:"nome"  binding:"required"`
		Email string `json:"email" binding:"required"`
		Role  string `json:"role"  binding:"required"`
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
	if creatorAdmin.Role != "fpp" {
		utils.RespondWithForbiddenError(c, "apenas admin com role 'fpp' pode criar administradores")
		return
	}

	repository := getRepository(c)
	creatorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	creator, ok := creatorAgg.(*aggregates.Admin)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado para criador"))
		return
	}

	if err := creator.ValidatePermission(req.Role); err != nil {
		utils.RespondWithForbiddenError(c, err.Error())
		return
	}

	existing, _ := adminProj.GetByEmail(req.Email)
	if existing != nil {
		utils.RespondWithConflictError(c, "Email já cadastrado no sistema")
		return
	}

	// gera senha aleatória segura via crypto/rand.
	// A senha é enviada ao admin APENAS via email; a resposta HTTP nunca a expõe.
	plainPassword, err := services.GenerateSecurePassword()
	if err != nil {
		log.Printf("[ERROR] RegisterAdmin: falha ao gerar senha segura para %s: %v", req.Email, err)
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao gerar senha temporária"))
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(plainPassword), bcrypt.DefaultCost)
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

	// registro da ação do criador é auditoria secundária.
	// A criação já está persistida — falha aqui não reverte.
	if err := creator.RegistrarAcao("admin_criado", map[string]interface{}{
		"novo_admin_id": newAdmin.ID.String(),
		"role":          req.Role,
		"email":         req.Email,
	}); err != nil {
		log.Printf("[WARN] RegisterAdmin: falha ao preparar ação do criador: %v", err)
	} else if err := repository.SaveWithAudit(creator, audit); err != nil {
		log.Printf("[WARN] RegisterAdmin: falha ao registrar ação do criador no ledger: %v", err)
	}

	// envia email de boas-vindas com a senha temporária.
	// Falha de email não bloqueia a criação.
	emailSvc := getEmailService(c)
	if emailErr := emailSvc.SendAdminWelcomeEmail(req.Email, req.Nome, plainPassword, req.Role); emailErr != nil {
		log.Printf("[WARN] RegisterAdmin: falha ao enviar email para %s: %v", req.Email, emailErr)
		c.JSON(http.StatusCreated, gin.H{
			"message": "administrador criado com sucesso. ATENÇÃO: falha ao enviar email — solicite reset de senha via /recuperar-senha/solicitar.",
			"data": gin.H{
				"id":    newAdmin.ID,
				"nome":  newAdmin.Nome,
				"email": req.Email,
				"role":  newAdmin.Role,
			},
			"aviso": "email_nao_enviado",
		})
		return
	}

	log.Printf("Admin criado: %s (%s) por %s", req.Email, req.Role, creatorAdmin.Nome)
	c.JSON(http.StatusCreated, gin.H{
		"message": "administrador criado com sucesso. A senha temporária foi enviada por email.",
		"data": gin.H{
			"id":    newAdmin.ID,
			"nome":  newAdmin.Nome,
			"email": req.Email,
			"role":  newAdmin.Role,
		},
	})
}

func GetAdminPorEmail(c *gin.Context) {
	if err := verificarPermissaoAdmin(c, "adm"); err != nil {
		utils.RespondWithForbiddenError(c, "acesso restrito a administradores com role 'adm' ou superior")
		return
	}

	email := c.Param("email")

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(email)
	if err != nil || admin == nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	executorID, _ := middleware.GetUserID(c)
	executorAdmin, _ := adminProj.GetByID(executorID)

	resp := gin.H{
		"id":               admin.ID,
		"nome":             admin.Nome,
		"email":            admin.Email,
		"email_verificado": admin.EmailVerificado,
		"role":             admin.Role,
		"status":           admin.Status,
		"created_at":       admin.CreatedAt,
		"updated_at":       admin.UpdatedAt,
	}

	if executorAdmin != nil && executorAdmin.Role == "fpp" {
		resp["created_by"] = admin.CreatedBy
		resp["total_acoes_realizadas"] = admin.TotalAcoesRealizadas
	}

	c.JSON(http.StatusOK, gin.H{"admin": resp})
}

func ListarTodosAdmins(c *gin.Context) {
	if err := verificarPermissaoAdmin(c, "adm"); err != nil {
		utils.RespondWithForbiddenError(c, "acesso restrito a administradores com role 'adm' ou superior")
		return
	}

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
		// SenhaHash tem tag json:"-" mas o delete é proteção defensiva explícita.
		delete(adminMap, "senha_hash")
		adminsResponse = append(adminsResponse, adminMap)
	}

	c.JSON(http.StatusOK, gin.H{
		"admins": adminsResponse,
		"total":  len(adminsResponse),
	})
}

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

	targetAdmin, ok := targetAdminAgg.(*aggregates.Admin)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado para target admin"))
		return
	}

	executorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	executor, ok := executorAgg.(*aggregates.Admin)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado para executor"))
		return
	}

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

	targetAdmin, ok := targetAdminAgg.(*aggregates.Admin)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado para target admin"))
		return
	}

	executorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	executor, ok := executorAgg.(*aggregates.Admin)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado para executor"))
		return
	}

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

	if adminID == userID {
		utils.RespondWithValidationError(c, fmt.Errorf("você não pode alterar seu próprio role"))
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

	admin, ok := adminAgg.(*aggregates.Admin)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	roleAnterior := admin.Role

	executorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	executor, ok := executorAgg.(*aggregates.Admin)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado para executor"))
		return
	}

	if err := executor.ValidatePermission(roleAnterior); err != nil {
		utils.RespondWithForbiddenError(c, fmt.Sprintf("permissão negada para alterar admin com role '%s': %s", roleAnterior, err.Error()))
		return
	}

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

	registrarAcaoAdmin(c, userID, "admin_role_atualizado", map[string]interface{}{
		"target_admin_id": adminID.String(),
		"target_email":    admin.Email,
		"role_anterior":   roleAnterior,
		"novo_role":       req.NovoRole,
	})

	log.Printf("Role atualizado: %s -> %s (Admin: %s)", roleAnterior, req.NovoRole, admin.Email)
	c.JSON(http.StatusOK, gin.H{
		"message":       "role atualizado com sucesso",
		"role_anterior": roleAnterior,
		"novo_role":     req.NovoRole,
	})
}

func AtualizarDadosAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	adminID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de admin inválido"))
		return
	}

	var req struct {
		Nome  *string `json:"nome"`
		Email *string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("body inválido"))
		return
	}

	if req.Nome == nil && req.Email == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ao menos um campo deve ser fornecido: nome ou email"))
		return
	}

	adminProj := getAdminProjection(c)

	if req.Email != nil {
		existing, _ := adminProj.GetByEmail(*req.Email)
		if existing != nil && existing.ID != adminID {
			utils.RespondWithConflictError(c, "email já cadastrado no sistema")
			return
		}
	}

	repository := getRepository(c)
	adminAgg, err := repository.Load(adminID, "Admin")
	if err != nil {
		utils.RespondWithNotFoundError(c, "admin")
		return
	}

	admin, ok := adminAgg.(*aggregates.Admin)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
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

	c.JSON(http.StatusOK, gin.H{"message": "dados atualizados com sucesso"})
}
