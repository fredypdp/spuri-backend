package handlers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// ============================================================================
// PUT /estudante/dados-pessoais
// ============================================================================

func AtualizarDadosPessoaisEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome                  *string `json:"nome"`
		Email                 *string `json:"email"`
		Telefone              *string `json:"telefone"`
		BilheteIdentidade     *string `json:"bilhete_identidade"`
		BilheteIdentidadeResp *string `json:"bilhete_identidade_responsavel"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	if req.Nome == nil && req.Email == nil && req.Telefone == nil &&
		req.BilheteIdentidade == nil && req.BilheteIdentidadeResp == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("nenhum campo para atualizar"))
		return
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante := estudanteAgg.(*aggregates.Estudante)

	if err := estudante.AtualizarDadosPessoais(
		req.Nome, req.Email, req.Telefone,
		req.BilheteIdentidade, req.BilheteIdentidadeResp,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "estudante",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Dados pessoais atualizados: %s", estudante.CodigoEstudante)
	c.JSON(http.StatusOK, gin.H{"message": "dados pessoais atualizados com sucesso"})
}

// ============================================================================
// PUT /estudante/dados-academicos
// ============================================================================

func AtualizarDadosAcademicosEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		AnoEscolar      *string    `json:"ano_escolar"`
		AnoEscolarMedio *string    `json:"ano_escolar_medio"`
		AnoSuperior     *string    `json:"ano_superior"`
		CursoMedioID    *uuid.UUID `json:"curso_medio_id"`
		CursoSuperiorID *uuid.UUID `json:"curso_superior_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante := estudanteAgg.(*aggregates.Estudante)

	if err := estudante.AtualizarDadosAcademicos(
		req.AnoEscolar, req.AnoEscolarMedio, req.AnoSuperior,
		req.CursoMedioID, req.CursoSuperiorID,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "estudante",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Dados académicos atualizados: %s", estudante.CodigoEstudante)
	c.JSON(http.StatusOK, gin.H{"message": "dados académicos atualizados com sucesso"})
}

// ============================================================================
// PUT /academia/dados
// ============================================================================

func AtualizarDadosAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome           *string  `json:"nome"`
		Provincia      *string  `json:"provincia"`
		Endereco       *string  `json:"endereco"`
		NumeroTelefone *string  `json:"numero_telefone"`
		Email          *string  `json:"email"`
		Website        *string  `json:"website"`
		NivelEscolar   *string  `json:"nivel_escolar"`
		AnosAcademicos []string `json:"anos_academicos"`
		Cursos         []string `json:"cursos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Website != nil {
		if err := utils.ValidateURL(*req.Website); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	repository := getRepository(c)
	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	academia := academiaAgg.(*aggregates.Academia)

	if err := academia.AtualizarDados(
		req.Nome, req.Provincia, req.Endereco,
		req.NumeroTelefone, req.Email, req.Website,
		req.NivelEscolar, req.AnosAcademicos, req.Cursos,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Dados da academia atualizados: %s", academia.CodigoAcademia)
	c.JSON(http.StatusOK, gin.H{"message": "dados da academia atualizados com sucesso"})
}

// ============================================================================
// PUT /admin/role/:id
// ============================================================================

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

// ============================================================================
// PUT /admin/dados/:id
// CORRIGIDO #1: autorização horizontal.
// Um admin só pode editar admins de nível inferior ao seu.
// Admin pode sempre editar seus próprios dados.
// ============================================================================

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

	// CORRIGIDO #1: verificação de autorização horizontal.
	// Admin pode editar os próprios dados sem restrição.
	// Para editar outro admin, precisa de role superior ao alvo.
	if userID != targetID {
		executorAgg, err := repository.Load(userID, "Admin")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		executor := executorAgg.(*aggregates.Admin)

		// ValidatePermission retorna erro se executor.Role <= admin.Role (alvo)
		if err := executor.ValidatePermission(admin.Role); err != nil {
			utils.RespondWithForbiddenError(c, fmt.Sprintf(
				"permissão negada: %s", err.Error(),
			))
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

	// Suprimir aviso de import não utilizado (net/http é usado via utils)
	_ = http.StatusOK
}