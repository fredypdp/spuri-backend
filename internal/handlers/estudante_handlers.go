package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type RegisterEstudanteRequest struct {
	Senha                    string     `json:"senha" binding:"required"`
	Nome                     string     `json:"nome" binding:"required"`
	Email                    *string    `json:"email"`
	Telefone                 *string    `json:"telefone"`
	BilheteIdentidade        *string    `json:"bilhete_identidade"`
	BilheteIdentidadeResp    *string    `json:"bilhete_identidade_responsavel"`
	AnoEscolar               *string    `json:"ano_escolar"`
	AnoEscolarMedio          *string    `json:"ano_escolar_medio"`
	AnoSuperior              *string    `json:"ano_superior"`
	CursoMedioID             *uuid.UUID `json:"curso_medio_id"`
	CursoSuperiorID          *uuid.UUID `json:"curso_superior_id"`
	StatusEscolarFundamental *string    `json:"status_escolar_fundamental"`
	StatusEscolarMedio       *string    `json:"status_escolar_medio"`
	StatusSuperior           *string    `json:"status_superior"`
	Genero                   string     `json:"genero" binding:"required"`
}

func RegisterEstudante(c *gin.Context) {
	var req RegisterEstudanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateSenha(req.Senha); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Telefone != nil {
		if err := utils.ValidatePhone(*req.Telefone); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if err := utils.ValidateBilhete(utils.SafeDeref(req.BilheteIdentidade)); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateBilhete(utils.SafeDeref(req.BilheteIdentidadeResp)); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.BilheteIdentidade == nil && req.BilheteIdentidadeResp == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("pelo menos um bilhete de identidade é obrigatório"))
		return
	}

	if req.BilheteIdentidade != nil && *req.BilheteIdentidade != "" {
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(*req.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado"))
			return
		}
	}

	if req.CursoMedioID != nil && *req.CursoMedioID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id não encontrado"))
			return
		}
		if curso.Type != "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id deve ser do tipo 'medio'"))
			return
		}
	}

	if req.CursoSuperiorID != nil && *req.CursoSuperiorID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoSuperiorID)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id não encontrado"))
			return
		}
		if curso.Type != "superior" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id deve ser do tipo 'superior'"))
			return
		}
	}

	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	if err := estudante.Criar(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		req.Email,
		req.Telefone,
		req.BilheteIdentidade,
		req.BilheteIdentidadeResp,
		req.AnoEscolar,
		req.AnoEscolarMedio,
		req.AnoSuperior,
		req.CursoMedioID,
		req.CursoSuperiorID,
		req.StatusEscolarFundamental,
		req.StatusEscolarMedio,
		req.StatusSuperior,
		req.Genero,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Rota pública — sem JWT, usar contexto de sistema
	audit := db.AuditContext{
		UserID:   "anonimo",
		UserType: "sistema",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Estudante criado: %s - %s", codigoEstudante, req.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante criado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
		},
	})
}

type CadastroEstudanteAcademiaRequest struct {
	Nome                     string `json:"nome" binding:"required"`
	Email                    string `json:"email"`
	Telefone                 string `json:"telefone"`
	BilheteIdentidade        string `json:"bilhete_identidade"`
	BilheteResponsavel       string `json:"bilhete_identidade_responsavel"`
	AnoEscolar               string `json:"ano_escolar"`
	AnoEscolarMedio          string `json:"ano_escolar_medio"`
	AnoSuperior              string `json:"ano_superior"`
	CursoMedioID             string `json:"curso_medio_id"`
	CursoSuperiorID          string `json:"curso_superior_id"`
	StatusEscolarFundamental string `json:"status_escolar_fundamental"`
	StatusEscolarMedio       string `json:"status_escolar_medio"`
	StatusSuperior           string `json:"status_superior"`
	Genero                   string `json:"genero" binding:"required"`
}

func RegisterEstudantePorAcademia(c *gin.Context) {
	var req CadastroEstudanteAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("nome é obrigatório"))
		return
	}

	log.Printf("📥 [RegisterEstudantePorAcademia] Nome: %s, AnoEscolar: '%s', CursoMedioID: '%s'",
		req.Nome, req.AnoEscolar, req.CursoMedioID)

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != "" {
		if err := utils.ValidateEmail(req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Telefone != "" {
		if err := utils.ValidatePhone(req.Telefone); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.BilheteIdentidade != "" {
		if err := utils.ValidateBilhete(req.BilheteIdentidade); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.BilheteResponsavel != "" {
		if err := utils.ValidateBilhete(req.BilheteResponsavel); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.BilheteIdentidade != "" && req.BilheteResponsavel != "" && req.BilheteIdentidade == req.BilheteResponsavel {
		utils.RespondWithValidationError(c, fmt.Errorf("bilhete do estudante e do responsável não podem ser iguais"))
		return
	}

	if req.BilheteIdentidade == "" && req.BilheteResponsavel == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("pelo menos um bilhete de identidade é obrigatório"))
		return
	}

	if req.BilheteIdentidade != "" {
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(req.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado no sistema"))
			return
		}
	}

	// Converter strings para ponteiros
	var anoEscolarPtr, anoSuperiorPtr *string
	var cursoMedioUUID, cursoSuperiorUUID *uuid.UUID

	if req.AnoEscolar != "" {
		anoEscolarPtr = &req.AnoEscolar
	}
	var anoEscolarMedioPtr *string
	if req.AnoEscolarMedio != "" {
		anoEscolarMedioPtr = &req.AnoEscolarMedio
	}
	if req.AnoSuperior != "" {
		anoSuperiorPtr = &req.AnoSuperior
	}

	if req.CursoMedioID != "" {
		parsed, err := uuid.Parse(req.CursoMedioID)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id inválido"))
			return
		}
		cursoMedioUUID = &parsed

		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id não encontrado"))
			return
		}
		if curso.Type != "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id deve ser do tipo 'medio'"))
			return
		}
	}

	if req.CursoSuperiorID != "" {
		parsed, err := uuid.Parse(req.CursoSuperiorID)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id inválido"))
			return
		}
		cursoSuperiorUUID = &parsed

		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id não encontrado"))
			return
		}
		if curso.Type != "superior" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id deve ser do tipo 'superior'"))
			return
		}
	}

	academiaID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByID(academiaID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academia == nil {
		utils.RespondWithError(c, http.StatusNotFound, "academia não encontrada", nil)
		return
	}

	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("spuri123"), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Converter campos opcionais de string para *string
	var emailPtr, telefonePtr, bilhetePtr, bilheteRespPtr *string

	if req.Email != "" {
		emailPtr = &req.Email
	}
	if req.Telefone != "" {
		telefonePtr = &req.Telefone
	}
	if req.BilheteIdentidade != "" {
		bilhetePtr = &req.BilheteIdentidade
	}
	if req.BilheteResponsavel != "" {
		bilheteRespPtr = &req.BilheteResponsavel
	}

	// Status escolar fundamental — default "em_andamento" se academia não especificar
	var statusFundamentalPtr, statusMedioPtr, statusSuperiorPtr *string

	if req.StatusEscolarFundamental != "" {
		statusFundamentalPtr = &req.StatusEscolarFundamental
	} else {
		defaultStatus := "em_andamento"
		statusFundamentalPtr = &defaultStatus
	}
	if req.StatusEscolarMedio != "" {
		statusMedioPtr = &req.StatusEscolarMedio
	}
	if req.StatusSuperior != "" {
		statusSuperiorPtr = &req.StatusSuperior
	}

	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	log.Printf("🔧 [RegisterEstudantePorAcademia] AnoEscolar=%v, CursoMedioID=%v", anoEscolarPtr, cursoMedioUUID)

	if err := estudante.CriarComVinculo(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		emailPtr,
		telefonePtr,
		bilhetePtr,
		bilheteRespPtr,
		anoEscolarPtr,
		anoEscolarMedioPtr,
		anoSuperiorPtr,
		cursoMedioUUID,
		cursoSuperiorUUID,
		statusFundamentalPtr,
		statusMedioPtr,
		statusSuperiorPtr,
		academia.CodigoAcademia,
		req.Genero,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Rota autenticada — academia cadastrando estudante
	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [RegisterEstudantePorAcademia] Estudante criado: %s - AnoEscolar: %v, CursoMedioID: %v",
		codigoEstudante, anoEscolarPtr, cursoMedioUUID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante cadastrado e vinculado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
			"codigo_academia":  academia.CodigoAcademia,
			"status":           "ativo",
			"ano_escolar":      anoEscolarPtr,
			"curso_medio_id":   cursoMedioUUID,
		},
	})
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
// GET /estudante/minhas-avaliacoes
// ============================================================================

func GetMinhasAvaliacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	avaliacaoProj := getAvaliacaoFinalProjection(c)
	avaliacoes, err := avaliacaoProj.GetByEstudante(estudante.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"avaliacoes": avaliacoes,
		"total":      len(avaliacoes),
	})
}

// AtualizarStatusEscolarFundamentalHandler — PUT /estudante/status-escolar-fundamental
func AtualizarStatusEscolarFundamentalHandler(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusEscolarFundamental(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ Status escolar fundamental atualizado: %s → %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status_escolar_fundamental atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusEscolarMedioHandler — PUT /estudante/status-escolar-medio
func AtualizarStatusEscolarMedioHandler(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusEscolarMedio(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ Status escolar médio atualizado: %s → %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status_escolar_medio atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

// AtualizarStatusSuperior — PUT /estudante/status-superior
func AtualizarStatusSuperior(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		NovoStatus string `json:"novo_status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_status é obrigatório"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarStatusSuperior(req.NovoStatus); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ Status superior atualizado: %s → %s", estudante.CodigoEstudante, req.NovoStatus)
	c.JSON(http.StatusOK, gin.H{
		"message":     "status superior atualizado com sucesso",
		"novo_status": req.NovoStatus,
	})
}

func GetEventosEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		utils.RespondWithForbiddenError(c, "Você só pode visualizar seus próprios eventos")
		return
	}

	repository := getRepository(c)
	eventos, err := repository.GetEventHistory(estudante.ID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"eventos":          eventos,
		"total":            len(eventos),
		"message":          "Histórico completo de eventos (Event Sourcing)",
	})
}

func GetEstudantePorCodigo(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	codigoEstudante := c.Param("codigo")

	if userType != "academia" && userType != "admin" {
		utils.RespondWithForbiddenError(c, "Acesso negado. Apenas academias e administradores podem consultar estudantes.")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	var academiaInfo *gin.H
	if estudante.CodigoAcademia != nil {
		academiaProj := getAcademiaProjection(c)
		academia, _ := academiaProj.GetByCodigo(*estudante.CodigoAcademia)
		if academia != nil {
			academiaInfo = &gin.H{
				"codigo":        academia.CodigoAcademia,
				"nome":          academia.Nome,
				"tipo":          academia.Type,
				"provincia":     academia.Provincia,
				"nivel_escolar": academia.NivelEscolar,
			}
		}
	}

	var cursoMedioInfo *gin.H
	var cursoSuperiorInfo *gin.H

	cursosProj := getCursosProjection(c)

	if estudante.CursoMedioID != nil {
		cursoMedio, _ := cursosProj.GetByID(*estudante.CursoMedioID)
		if cursoMedio != nil {
			cursoMedioInfo = &gin.H{
				"id":     cursoMedio.ID,
				"nome":   cursoMedio.Nome,
				"type":   cursoMedio.Type,
				"status": cursoMedio.Status,
			}
		}
	}

	if estudante.CursoSuperiorID != nil {
		cursoSuperior, _ := cursosProj.GetByID(*estudante.CursoSuperiorID)
		if cursoSuperior != nil {
			cursoSuperiorInfo = &gin.H{
				"id":     cursoSuperior.ID,
				"nome":   cursoSuperior.Nome,
				"type":   cursoSuperior.Type,
				"status": cursoSuperior.Status,
			}
		}
	}

	userID, _ := middleware.GetUserID(c)
	c.JSON(http.StatusOK, gin.H{
		"estudante": gin.H{
			"id":                             estudante.ID,
			"nome":                           estudante.Nome,
			"codigo_estudante":               estudante.CodigoEstudante,
			"email":                          estudante.Email,
			"telefone":                       estudante.Telefone,
			"email_verificado":               estudante.EmailVerificado,
			"bilhete_identidade":             estudante.BilheteIdentidade,
			"bilhete_identidade_responsavel": estudante.BilheteIdentidadeResp,
			"codigo_academia":                estudante.CodigoAcademia,
			"academia_info":                  academiaInfo,
			"status":                         estudante.Status,
			"status_escolar_fundamental":     estudante.StatusEscolarFundamental,
			"status_escolar_medio":           estudante.StatusEscolarMedio,
			"status_superior":                estudante.StatusSuperior,
			"ano_escolar":                    estudante.AnoEscolar,
			"ano_escolar_medio":              estudante.AnoEscolarMedio,
			"ano_superior":                   estudante.AnoSuperior,
			"curso_medio_id":                 estudante.CursoMedioID,
			"curso_medio_info":               cursoMedioInfo,
			"curso_superior_id":              estudante.CursoSuperiorID,
			"curso_superior_info":            cursoSuperiorInfo,
			"created_at":                     estudante.CreatedAt,
			"updated_at":                     estudante.UpdatedAt,
			"total_notas":                    estudante.TotalNotas,
			"total_faltas":                   estudante.TotalFaltas,
			"total_inscricoes":               estudante.TotalInscricoes,
			"version":                        estudante.Version,
		},
		"consultado_por": userType,
		"consultado_por_id": userID,
	})
}

func ListarEstudantes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	client := getDbClient(c)

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		rows, err := client.DB().Query(`
			SELECT id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
				bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
				status, status_escolar_fundamental, status_escolar_medio, status_superior,
				ano_escolar, ano_escolar_medio, ano_superior,
				curso_medio_id, curso_superior_id, created_at, updated_at, total_notas,
				total_faltas, total_inscricoes, version
			FROM projection_estudantes
			WHERE codigo_academia = $1
			ORDER BY created_at DESC
		`, academiaDTO.CodigoAcademia)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		var estudantes []map[string]interface{}
		for rows.Next() {
			var id, cursoMedioID, cursoSuperiorID sql.NullString
			var nome, codigoEstudante, senhaHash, status string
			var statusEscolarFundamental, statusEscolarMedio, statusSuperior string
			var email, telefone, bilhete, bilheteResp, codigoAcad sql.NullString
			var anoEscolar, anoEscolarMedio, anoSuperior sql.NullString
			var emailVerif bool
			var createdAt, updatedAt string
			var totalNotas, totalFaltas, totalInsc, version int

			if err := rows.Scan(
				&id, &nome, &codigoEstudante, &senhaHash,
				&email, &telefone, &emailVerif, &bilhete, &bilheteResp, &codigoAcad,
				&status, &statusEscolarFundamental, &statusEscolarMedio, &statusSuperior,
				&anoEscolar, &anoEscolarMedio, &anoSuperior,
				&cursoMedioID, &cursoSuperiorID,
				&createdAt, &updatedAt, &totalNotas, &totalFaltas, &totalInsc, &version,
			); err != nil {
				log.Printf("[ERROR] ListarEstudantes (academia) scan: %v", err)
				continue
			}

			estudantes = append(estudantes, map[string]interface{}{
				"id":                             id.String,
				"nome":                           nome,
				"codigo_estudante":               codigoEstudante,
				"email":                          getNullString(email),
				"telefone":                       getNullString(telefone),
				"email_verificado":               emailVerif,
				"bilhete_identidade":             getNullString(bilhete),
				"bilhete_identidade_responsavel": getNullString(bilheteResp),
				"codigo_academia":                getNullString(codigoAcad),
				"status":                         status,
				"status_escolar_fundamental":     statusEscolarFundamental,
				"status_escolar_medio":           statusEscolarMedio,
				"status_superior":                statusSuperior,
				"ano_escolar":                    getNullString(anoEscolar),
				"ano_escolar_medio":              getNullString(anoEscolarMedio),
				"ano_superior":                   getNullString(anoSuperior),
				"curso_medio_id":                 getNullString(cursoMedioID),
				"curso_superior_id":              getNullString(cursoSuperiorID),
				"created_at":                     createdAt,
				"updated_at":                     updatedAt,
				"total_notas":                    totalNotas,
				"total_faltas":                   totalFaltas,
				"total_inscricoes":               totalInsc,
				"version":                        version,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"estudantes":      estudantes,
			"total":           len(estudantes),
			"tipo_usuario":    "academia",
			"codigo_academia": academiaDTO.CodigoAcademia,
			"nome_academia":   academiaDTO.Nome,
		})

	} else if userType == "admin" {
		rows, err := client.DB().Query(`
			SELECT id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
				bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
				status, status_escolar_fundamental, status_escolar_medio, status_superior,
				ano_escolar, ano_escolar_medio, ano_superior,
				curso_medio_id, curso_superior_id, created_at, updated_at, total_notas,
				total_faltas, total_inscricoes, version
			FROM projection_estudantes
			ORDER BY created_at DESC
		`)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		var estudantes []map[string]interface{}
		for rows.Next() {
			var id, cursoMedioID, cursoSuperiorID sql.NullString
			var nome, codigoEstudante, senhaHash, status string
			var statusEscolarFundamental, statusEscolarMedio, statusSuperior string
			var email, telefone, bilhete, bilheteResp, codigoAcad sql.NullString
			var anoEscolar, anoEscolarMedio, anoSuperior sql.NullString
			var emailVerif bool
			var createdAt, updatedAt string
			var totalNotas, totalFaltas, totalInsc, version int

			if err := rows.Scan(
				&id, &nome, &codigoEstudante, &senhaHash,
				&email, &telefone, &emailVerif, &bilhete, &bilheteResp, &codigoAcad,
				&status, &statusEscolarFundamental, &statusEscolarMedio, &statusSuperior,
				&anoEscolar, &anoEscolarMedio, &anoSuperior,
				&cursoMedioID, &cursoSuperiorID,
				&createdAt, &updatedAt, &totalNotas, &totalFaltas, &totalInsc, &version,
			); err != nil {
				log.Printf("[ERROR] ListarEstudantes (admin) scan: %v", err)
				continue
			}

			estudantes = append(estudantes, map[string]interface{}{
				"id":                             id.String,
				"nome":                           nome,
				"codigo_estudante":               codigoEstudante,
				"email":                          getNullString(email),
				"telefone":                       getNullString(telefone),
				"email_verificado":               emailVerif,
				"bilhete_identidade":             getNullString(bilhete),
				"bilhete_identidade_responsavel": getNullString(bilheteResp),
				"codigo_academia":                getNullString(codigoAcad),
				"status":                         status,
				"status_escolar_fundamental":     statusEscolarFundamental,
				"status_escolar_medio":           statusEscolarMedio,
				"status_superior":                statusSuperior,
				"ano_escolar":                    getNullString(anoEscolar),
				"ano_escolar_medio":              getNullString(anoEscolarMedio),
				"ano_superior":                   getNullString(anoSuperior),
				"curso_medio_id":                 getNullString(cursoMedioID),
				"curso_superior_id":              getNullString(cursoSuperiorID),
				"created_at":                     createdAt,
				"updated_at":                     updatedAt,
				"total_notas":                    totalNotas,
				"total_faltas":                   totalFaltas,
				"total_inscricoes":               totalInsc,
				"version":                        version,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"estudantes":   estudantes,
			"total":        len(estudantes),
			"tipo_usuario": "admin",
		})

	} else {
		utils.RespondWithForbiddenError(c, "Acesso negado. Apenas academias e administradores podem listar estudantes.")
	}
}