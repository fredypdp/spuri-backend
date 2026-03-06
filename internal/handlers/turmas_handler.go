package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
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
	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
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

// AtivarTurma ativa uma turma inativa da academia.
// Rota: PUT /academia/turmas/:codigo/ativar
//
// NOVO (BUG #3 FIX): handler estava ausente — rota não existia em main.go.
// O aggregate Turma.Ativar() já estava implementado; faltava apenas este handler
// e o registro da rota.
func AtivarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	// ── Verificar propriedade ──────────────────────────────────────────────
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

	// ── Carregar aggregate e executar comando ─────────────────────────────
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

	if err := turma.Ativar(academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Persistir ─────────────────────────────────────────────────────────
	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		log.Printf("❌ [AtivarTurma] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [AtivarTurma] %s ativada pela academia %s", codigoTurma, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":      "turma ativada com sucesso",
		"codigo_turma": codigoTurma,
	})
}

// DesativarTurma desativa uma turma ativa da academia.
// Rota: PUT /academia/turmas/:codigo/desativar
//
// NOVO (BUG #3 FIX): handler estava ausente — rota não existia em main.go.
// O aggregate Turma.Desativar() já estava implementado; faltava apenas este handler
// e o registro da rota. Desativar é pré-requisito para DeletarTurma.
func DesativarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	// ── Verificar propriedade ──────────────────────────────────────────────
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

	// ── Carregar aggregate e executar comando ─────────────────────────────
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

	if err := turma.Desativar(academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Persistir ─────────────────────────────────────────────────────────
	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		log.Printf("❌ [DesativarTurma] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [DesativarTurma] %s desativada pela academia %s", codigoTurma, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":      "turma desativada com sucesso",
		"codigo_turma": codigoTurma,
	})
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
	estudanteProj := getEstudanteProjection(c)
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

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
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

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
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

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "turma atualizada com sucesso"})
}

// DeletarTurma remove logicamente uma turma da academia.
//
// Regras:
//   - Turma deve estar inativa (use PUT /turmas/:codigo/desativar antes)
//   - Turma não pode ter estudantes vinculados
//   - Apenas a academia dona pode deletar
//   - Evento TurmaDeletada gravado no ledger (auditável)
//
// Rota: DELETE /academia/turmas/:codigo
func DeletarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma   := c.Param("codigo")

	var req struct {
		Motivo string `json:"motivo"` // opcional, recomendado para auditoria
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
	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
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