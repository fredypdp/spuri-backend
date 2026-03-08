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
// POST /academia/materias
// ============================================================================

func CriarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome           string     `json:"nome"            binding:"required"`
		Type           string     `json:"type"            binding:"required"`
		AnosAcademicos []string   `json:"anos_academicos"`
		CursoID        *uuid.UUID `json:"curso_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatorios: nome e tipo"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithForbiddenError(c, "Academia inativa nao pode criar materias")
		return
	}

	if (req.Type == "medio" || req.Type == "superior") && req.CursoID != nil {
		cursosProj := getCursosProjection(c)
		cursoDTO, _ := cursosProj.GetByID(*req.CursoID)

		if cursoDTO == nil {
			utils.RespondWithNotFoundError(c, "curso")
			return
		}

		if cursoDTO.Status != "ativo" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso inativo nao pode ter materias"))
			return
		}

		if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Curso nao pertence a esta academia")
			return
		}

		// Para superior: garantir que o curso tem periodos definidos
		if req.Type == "superior" && len(cursoDTO.Periodos) == 0 {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"o curso '%s' nao possui periodos definidos. "+
					"Atualize o curso antes de criar materias superiores",
				cursoDTO.Nome,
			))
			return
		}
	}

	repository := getRepository(c)
	materia := aggregates.NewMateriaDisciplinar()

	if err := materia.Criar(req.Nome, req.Type, req.AnosAcademicos, academiaDTO.CodigoAcademia, req.CursoID, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(materia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Materia criada: %s - %s", req.Nome, materia.ID)
	c.JSON(http.StatusCreated, gin.H{
		"message": "materia criada com sucesso",
		"data": gin.H{
			"id":     materia.ID,
			"nome":   materia.Nome,
			"type":   materia.Type,
			"status": materia.Status,
			"proximo_passo": func() *string {
				if materia.Type == "superior" {
					s := "defina o periodo via PUT /academia/materias/" + materia.ID.String() + "/periodo antes de ativar"
					return &s
				}
				return nil
			}(),
		},
	})
}

// ============================================================================
// GET /academia/materias
// ============================================================================

func ListarMaterias(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	materiasProj := getMateriasProjection(c)
	materias, err := materiasProj.GetByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"materias": materias,
		"total":    len(materias),
	})
}

// ============================================================================
// PUT /academia/materias/:id/ativar
// ============================================================================

func AtivarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de materia invalido"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Materia nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)
	if err := materia.Ativar(userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(materia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Materia ativada: %s", materia.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "materia ativada com sucesso", "nome": materia.Nome})
}

// ============================================================================
// PUT /academia/materias/:id/desativar
// ============================================================================

func DesativarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de materia invalido"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Materia nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)

	if err := materia.Desativar(userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(materia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Materia desativada: %s", materia.Nome)
	c.JSON(http.StatusOK, gin.H{
		"message": "materia desativada com sucesso",
		"nome":    materia.Nome,
	})
}

// ============================================================================
// PUT /academia/materias/:id
// ============================================================================

func AtualizarDadosMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de matéria inválido"))
		return
	}

	var req struct {
		Nome *string `json:"nome"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "matéria")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Matéria não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		utils.RespondWithNotFoundError(c, "matéria")
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)
	// FIX: assinatura corrigida — AtualizarDados(nome, anosAcademicos, cursoID).
	// Handler atualiza apenas o nome; os demais campos permanecem inalterados.
	if err := materia.AtualizarDados(req.Nome, nil, nil, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(materia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Matéria atualizada: %s", materia.Nome)
	c.JSON(http.StatusOK, gin.H{
		"message": "matéria atualizada com sucesso",
		"nome":    materia.Nome,
	})
}

// ============================================================================
// PUT /academia/materias/:id/periodo
// ============================================================================

// DefinirPeriodoMateria define o período de uma matéria do tipo 'superior'.
// Após definir o período, a matéria pode ser ativada.
func DefinirPeriodoMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de materia invalido"))
		return
	}

	var req struct {
		Periodo string `json:"periodo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campo obrigatorio: periodo"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Materia nao pertence a esta academia")
		return
	}

	// Validar que o período pertence ao curso vinculado
	if materiaDTO.CursoID != nil {
		cursosProj := getCursosProjection(c)
		cursoDTO, err := cursosProj.GetByID(*materiaDTO.CursoID)
		if err == nil && cursoDTO != nil && len(cursoDTO.Periodos) > 0 {
			periodoValido := false
			for _, p := range cursoDTO.Periodos {
				if p == req.Periodo {
					periodoValido = true
					break
				}
			}
			if !periodoValido {
				utils.RespondWithValidationError(c, fmt.Errorf(
					"periodo '%s' nao pertence ao curso '%s'. Periodos disponiveis: %v",
					req.Periodo, cursoDTO.Nome, cursoDTO.Periodos,
				))
				return
			}
		}
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)

	if err := materia.DefinirPeriodo(req.Periodo, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(materia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Periodo definido: %s → %s", materia.Nome, materia.Periodo)
	c.JSON(http.StatusOK, gin.H{
		"message": "periodo definido com sucesso",
		"nome":    materia.Nome,
		"periodo": materia.Periodo,
	})
}

// ============================================================================
// DELETE /academia/materias/:id
// ============================================================================

// DeletarMateria remove a matéria da projeção via evento MateriaDeletada.
// A matéria deve estar inativa antes de ser deletada.
// O histórico permanece intacto no ledger (event sourcing).
func DeletarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de materia invalido"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Materia nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		utils.RespondWithNotFoundError(c, "materia")
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)

	// FIX: Deletar agora requer (deletadoPor uuid.UUID, motivo string).
	if err := materia.Deletar(userID, ""); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(materia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Materia deletada: %s (%s)", materia.Nome, materia.ID)
	c.JSON(http.StatusOK, gin.H{
		"message": "materia deletada com sucesso",
		"nome":    materia.Nome,
	})
}

// ============================================================================
// GET /academia/materias/:id
// ============================================================================

// GetMateria retorna uma matéria pelo ID.
// Protegido por RequireAcademia — academia só pode ver suas próprias matérias.
func GetMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de matéria inválido"))
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		utils.RespondWithNotFoundError(c, "matéria")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "matéria não pertence a esta academia")
		return
	}

	c.JSON(http.StatusOK, materiaDTO)
}