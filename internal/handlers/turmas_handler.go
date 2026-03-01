package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CriarTurma cria uma nova turma para a academia autenticada.
// Rota: POST /academia/turmas
func CriarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoTurma string     `json:"codigo_turma" binding:"required"`
		Nivel       string     `json:"nivel"        binding:"required"`
		CursoID     *uuid.UUID `json:"curso_id"`
		Turno       string     `json:"turno"        binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	existing, _ := turmasProj.GetByCodigoTurma(req.CodigoTurma, academiaDTO.CodigoAcademia)
	if existing != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("já existe uma turma com este código nesta academia"))
		return
	}

	turma := aggregates.NewTurma()
	if err := turma.Criar(
		req.CodigoTurma,
		academiaDTO.CodigoAcademia,
		req.Nivel,
		req.CursoID,
		req.Turno,
		academiaID,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	if err := repository.Save(turma); err != nil {
		log.Printf("❌ [CriarTurma] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [CriarTurma] %s criada na academia %s", req.CodigoTurma, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusCreated, gin.H{
		"message":      "turma criada com sucesso",
		"id":           turma.ID,
		"codigo_turma": req.CodigoTurma,
	})
}

// ListarTurmasAcademia lista todas as turmas da academia autenticada.
// Rota: GET /academia/turmas
func ListarTurmasAcademia(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmas, err := turmasProj.ListByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"turmas": turmas})
}

// GetTurma retorna uma turma pelo código.
// Rota: GET /academia/turmas/:codigo
func GetTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turma, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turma == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	c.JSON(http.StatusOK, turma)
}

// AdicionarEstudanteATurma adiciona um estudante à turma.
// Rota: POST /academia/turmas/:codigo/estudantes
func AdicionarEstudanteATurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	var req struct {
		CodigoEstudante string `json:"codigo_estudante" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Verifica se o estudante pertence à academia
	estudanteProj := getEstudanteProjection(c) // singular — função existente em helpers.go
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	if err := turma.AdicionarEstudante(req.CodigoEstudante, academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(turma); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "estudante adicionado à turma com sucesso",
		"codigo_turma":     codigoTurma,
		"codigo_estudante": req.CodigoEstudante,
	})
}

// RemoverEstudanteDaTurma remove um estudante da turma.
// Rota: DELETE /academia/turmas/:codigo/estudantes/:codigoEstudante
func RemoverEstudanteDaTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma     := c.Param("codigo")
	codigoEstudante := c.Param("codigoEstudante")

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	if err := turma.RemoverEstudante(codigoEstudante, academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(turma); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "estudante removido da turma com sucesso",
		"codigo_turma":     codigoTurma,
		"codigo_estudante": codigoEstudante,
	})
}

// AtualizarTurma atualiza dados da turma.
// Rota: PUT /academia/turmas/:codigo
func AtualizarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	var req struct {
		Nivel   *string    `json:"nivel"`
		CursoID *uuid.UUID `json:"curso_id"`
		Turno   *string    `json:"turno"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	if err := turma.AtualizarDados(req.Nivel, req.CursoID, req.Turno, academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(turma); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "turma atualizada com sucesso"})
}

// DeletarTurma remove logicamente uma turma da academia.
//
// Regras:
//   - Turma deve estar inativa (desative antes)
//   - Turma não pode ter estudantes vinculados
//   - Apenas a academia dona pode deletar
//   - Evento TurmaDeletada gravado no ledger (auditável)
func DeletarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma   := c.Param("codigo")

	var req struct {
		Motivo string `json:"motivo"` // opcional, mas recomendado para auditoria
	}
	// Não é obrigatório — ignorar erro de bind
	_ = c.ShouldBindJSON(&req)

	// ── Verificar propriedade ─────────────────────────────────────────────
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	// ── Carregar aggregate ────────────────────────────────────────────────
	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	// ── Executar comando (validações de negócio ficam no aggregate) ───────
	if err := turma.Deletar(academiaID, req.Motivo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Persistir (grava no ledger + atualiza projeção via manager) ───────
	if err := repository.Save(turma); err != nil {
		log.Printf("❌ [DeletarTurma] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [DeletarTurma] %s deletada pela academia %s", codigoTurma, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":      "turma deletada com sucesso",
		"codigo_turma": codigoTurma,
		"auditavel":    true,
	})
}

// getTurmasProjection instancia a projeção directamente (mesmo padrão dos outros helpers).
func getTurmasProjection(c *gin.Context) *projections.TurmasProjection {
	return projections.NewTurmasProjection(getDbClient(c))
}