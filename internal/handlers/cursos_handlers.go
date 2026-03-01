package handlers

import (
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// ============================================================================
// POST /academia/cursos
// ============================================================================

func CriarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome           string   `json:"nome"            binding:"required"`
		Type           string   `json:"type"            binding:"required"`
		AnosAcademicos []string `json:"anos_academicos" binding:"required"`
		Periodos       []string `json:"periodos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campos obrigatórios: nome, type, anos_academicos"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	if academiaDTO.Status != "ativo" {
		utils.RespondWithForbiddenError(c, "Academia inativa não pode criar cursos")
		return
	}

	if err := validarTipoCursoVsAcademia(req.Type, academiaDTO.Type); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	curso := aggregates.NewCurso()

	if err := curso.Criar(req.Nome, req.Type, req.AnosAcademicos, req.Periodos, academiaDTO.CodigoAcademia); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso criado: %s - %s (periodos=%v)", req.Nome, curso.ID, curso.Periodos)

	c.JSON(http.StatusCreated, gin.H{
		"message": "curso criado com sucesso",
		"data": gin.H{
			"id":       curso.ID,
			"nome":     curso.Nome,
			"type":     curso.Type,
			"periodos": curso.Periodos,
		},
	})
}

// ============================================================================
// GET /academia/cursos
// ============================================================================

func ListarCursos(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	cursosProj := getCursosProjection(c)
	cursos, err := cursosProj.GetByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"cursos": cursos,
		"total":  len(cursos),
	})
}

// ============================================================================
// PUT /academia/cursos/:id
// ============================================================================

func AtualizarDadosCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
		return
	}

	// Type não é aceito: o tipo do curso é imutável após a criação.
	var req struct {
		Nome           *string   `json:"nome"`
		AnosAcademicos []string  `json:"anos_academicos"`
		Periodos       *[]string `json:"periodos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados invalidos"))
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "curso nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.AtualizarDados(req.Nome, req.AnosAcademicos, req.Periodos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso atualizado: %s (periodos=%v)", curso.Nome, curso.Periodos)
	c.JSON(http.StatusOK, gin.H{
		"message":         "curso atualizado com sucesso",
		"nome":            curso.Nome,
		"type":            curso.Type,
		"anos_academicos": curso.AnosAcademicos,
		"periodos":        curso.Periodos,
	})
}

// ============================================================================
// PUT /academia/cursos/:id/ativar
// ============================================================================

func AtivarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso invalido"))
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Curso nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.Ativar(); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso ativado: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "curso ativado com sucesso", "nome": curso.Nome})
}

// ============================================================================
// PUT /academia/cursos/:id/desativar
// ============================================================================

func DesativarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso invalido"))
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "Curso nao pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	if err := curso.Desativar(); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso desativado: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "curso desativado com sucesso", "nome": curso.Nome})
}

// DeletarCurso remove logicamente um curso da academia.
//
// Regras de negócio (checadas ANTES de delegar ao aggregate):
//   1. Curso deve pertencer à academia autenticada
//   2. Curso deve estar inativo
//   3. Não pode haver estudantes matriculados neste curso
//   4. Não pode haver matérias ativas vinculadas ao curso
//
// Para cursos superiores, matérias inativas vinculadas são deletadas
// em cascata (cada uma emite seu próprio MateriaDeletada no ledger).
//
// Evento CursoDeletado gravado no ledger (auditável).
func DeletarCurso(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
		return
	}

	var req struct {
		Motivo string `json:"motivo"` // opcional, recomendado para auditoria
	}
	_ = c.ShouldBindJSON(&req)

	// ── 1. Verificar propriedade ──────────────────────────────────────────
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "curso não pertence a esta academia")
		return
	}

	// ── 2. Curso deve estar inativo ───────────────────────────────────────
	if cursoDTO.Status == "ativo" {
		utils.RespondWithValidationError(c, fmt.Errorf("desative o curso antes de deletá-lo"))
		return
	}

	// ── 3. Checar estudantes matriculados neste curso ─────────────────────
	estudanteProj := getEstudanteProjection(c)
	estudantesNoCurso, err := estudanteProj.CountByCurso(cursoID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if estudantesNoCurso > 0 {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"não é possível deletar: %d estudante(s) matriculado(s) neste curso",
			estudantesNoCurso,
		))
		return
	}

	// ── 4. Checar matérias vinculadas ─────────────────────────────────────
	materiasProj := getMateriasProjection(c)
	materiasDoCurso, err := materiasProj.GetByCurso(cursoID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Bloquear se houver matérias ATIVAS
	for _, m := range materiasDoCurso {
		if m.Status == "ativo" {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"desative todas as matérias antes de deletar o curso (matéria ativa: %s)",
				m.Nome,
			))
			return
		}
	}

	repository := getRepository(c)

	// ── 5. Deletar matérias inativas em cascata (cada uma emite evento) ───
	// Apenas cursos superiores tendem a ter matérias; médio normalmente não.
	var materiasDeletedNomes []string
	for _, m := range materiasDoCurso {
		if m.Status == "deletado" {
			continue // já deletada anteriormente
		}
		materiaAgg, err := repository.Load(m.ID, "MateriaDisciplinar")
		if err != nil {
			log.Printf("⚠️  [DeletarCurso] Erro ao carregar matéria %s: %v", m.ID, err)
			continue
		}
		materia := materiaAgg.(*aggregates.MateriaDisciplinar)
		if err := materia.Deletar(); err != nil {
			log.Printf("⚠️  [DeletarCurso] Erro ao deletar matéria %s: %v", m.Nome, err)
			continue
		}
		if err := repository.Save(materia); err != nil {
			utils.RespondWithInternalError(c, fmt.Errorf("erro ao deletar matéria %s: %w", m.Nome, err))
			return
		}
		materiasDeletedNomes = append(materiasDeletedNomes, m.Nome)
	}

	// ── 6. Deletar turmas vinculadas ao curso (inativas, sem estudantes) ──
	turmasProj := getTurmasProjection(c)
	turmasDoCurso, err := turmasProj.ListByCurso(cursoID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	var turmasDeletedCodigos []string
	for _, t := range turmasDoCurso {
		if t.Status == "deletado" {
			continue
		}
		if t.Status == "ativo" {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"desative todas as turmas vinculadas antes de deletar o curso (turma ativa: %s)",
				t.CodigoTurma,
			))
			return
		}
		if len(t.Estudantes) > 0 {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"turma %s ainda possui estudantes vinculados", t.CodigoTurma,
			))
			return
		}
		turmaAgg, err := repository.Load(t.ID, "Turma")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		turma := turmaAgg.(*aggregates.Turma)
		if err := turma.Deletar(academiaID, fmt.Sprintf("cascata: curso %s deletado", cursoDTO.Nome)); err != nil {
			log.Printf("⚠️  [DeletarCurso] Erro ao deletar turma %s: %v", t.CodigoTurma, err)
			continue
		}
		if err := repository.Save(turma); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		turmasDeletedCodigos = append(turmasDeletedCodigos, t.CodigoTurma)
	}

	// ── 7. Deletar o curso em si ──────────────────────────────────────────
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	curso := cursoAgg.(*aggregates.Curso)
	if err := curso.Deletar(academiaID, req.Motivo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		log.Printf("❌ [DeletarCurso] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [DeletarCurso] curso=%s academia=%s materias_deletadas=%v turmas_deletadas=%v",
		cursoDTO.Nome, academiaDTO.CodigoAcademia, materiasDeletedNomes, turmasDeletedCodigos,
	)

	c.JSON(http.StatusOK, gin.H{
		"message":           "curso deletado com sucesso",
		"curso_id":          cursoID,
		"nome":              cursoDTO.Nome,
		"materias_deletadas": materiasDeletedNomes,
		"turmas_deletadas":  turmasDeletedCodigos,
		"auditavel":         true,
	})
}

// ============================================================================
// Helpers internos
// ============================================================================

func validarTipoCursoVsAcademia(tipoCurso, tipoAcademia string) error {
	switch tipoAcademia {
	case "escola":
		if tipoCurso != "medio" {
			return fmt.Errorf("academias do tipo 'escola' so podem criar cursos do tipo 'medio'")
		}
	case "superior":
		if tipoCurso != "superior" {
			return fmt.Errorf("academias do tipo 'superior' so podem criar cursos do tipo 'superior'")
		}
	}
	return nil
}