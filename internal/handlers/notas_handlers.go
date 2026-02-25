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
// POST /academia/registrar-nota
// ============================================================================

func RegistrarNota(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante      string  `json:"codigo_estudante"       binding:"required"`
		AnoLectivo           string  `json:"ano_lectivo"            binding:"required"`
		Periodo              string  `json:"periodo"                binding:"required"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
		Tipo                 string  `json:"tipo"                   binding:"required"` // escolar | superior
		Categoria            string  `json:"categoria"              binding:"required"` // ver docs
		Nota                 float64 `json:"nota"                   binding:"required"`
		Observacao           *string `json:"observacao"`            // opcional
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"campos obrigatórios: codigo_estudante, ano_lectivo, periodo, "+
				"materia_disciplinar_id, tipo, categoria, nota",
		))
		return
	}

	// Carregar academia autenticada
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// Verificar coerência entre tipo da nota e tipo da academia
	if req.Tipo != academiaDTO.Type {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"tipo de nota '%s' não corresponde ao tipo da academia '%s'",
			req.Tipo, academiaDTO.Type,
		))
		return
	}

	// Carregar estudante
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

	// Validar matéria
	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("materia_disciplinar_id inválido"))
		return
	}
	materiasProj := getMateriasProjection(c)
	materiaDTO, _ := materiasProj.GetByID(materiaID)
	if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "matéria não pertence a esta academia")
		return
	}

	// Buscar categorias adicionais da academia (apenas para superior)
	var categoriasAdicionais []string
	if req.Tipo == "superior" {
		categoriasAdicionais = carregarCategoriasAdicionais(c, academiaDTO.CodigoAcademia)
	}

	// Carregar aggregate e executar comando
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante := estudanteAgg.(*aggregates.Estudante)

	err = estudante.RegistrarNota(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		req.Periodo,
		materiaID,
		req.Tipo,
		req.Categoria,
		req.Nota,
		req.Observacao,
		categoriasAdicionais,
	)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Nota registrada: %s — %.2f [%s/%s] em %s",
		req.CodigoEstudante, req.Nota, req.Tipo, req.Categoria, materiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":   "nota registrada com sucesso",
		"estudante": req.CodigoEstudante,
		"materia":   materiaDTO.Nome,
		"tipo":      req.Tipo,
		"categoria": req.Categoria,
		"nota":      req.Nota,
	})
}

// ============================================================================
// PUT /academia/atualizar-nota
// ============================================================================

func AtualizarNota(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante      string  `json:"codigo_estudante"       binding:"required"`
		AnoLectivo           string  `json:"ano_lectivo"            binding:"required"`
		Periodo              string  `json:"periodo"                binding:"required"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
		Tipo                 string  `json:"tipo"                   binding:"required"`
		Categoria            string  `json:"categoria"              binding:"required"`
		NotaNova             float64 `json:"nota_nova"              binding:"required"`
		Observacao           string  `json:"observacao"             binding:"required"` // obrigatória
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"campos obrigatórios: codigo_estudante, ano_lectivo, periodo, "+
				"materia_disciplinar_id, tipo, categoria, nota_nova, observacao",
		))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

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

	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("materia_disciplinar_id inválido"))
		return
	}
	materiasProj := getMateriasProjection(c)
	materiaDTO, _ := materiasProj.GetByID(materiaID)
	if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "matéria não pertence a esta academia")
		return
	}

	// Buscar nota atual na projeção (para registrar nota anterior no evento)
	notasProj := getNotasProjection(c)
	notaAtual, err := notasProj.GetNota(
		req.CodigoEstudante,
		req.AnoLectivo,
		req.Periodo,
		materiaID,
		req.Tipo,
		req.Categoria,
	)
	if err != nil || notaAtual == nil {
		utils.RespondWithNotFoundError(c, "nota a atualizar")
		return
	}

	var categoriasAdicionais []string
	if req.Tipo == "superior" {
		categoriasAdicionais = carregarCategoriasAdicionais(c, academiaDTO.CodigoAcademia)
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante := estudanteAgg.(*aggregates.Estudante)

	err = estudante.AtualizarNota(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		req.Periodo,
		materiaID,
		req.Tipo,
		req.Categoria,
		notaAtual.Nota,
		req.NotaNova,
		req.Observacao,
		categoriasAdicionais,
	)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Nota atualizada: %s — %.2f→%.2f [%s/%s]",
		req.CodigoEstudante, notaAtual.Nota, req.NotaNova, req.Tipo, req.Categoria)

	c.JSON(http.StatusOK, gin.H{
		"message":       "nota atualizada com sucesso",
		"estudante":     req.CodigoEstudante,
		"tipo":          req.Tipo,
		"categoria":     req.Categoria,
		"nota_anterior": notaAtual.Nota,
		"nota_nova":     req.NotaNova,
	})
}

// ============================================================================
// POST /academia/categorias-nota
// ============================================================================

func CriarCategoriaNotaSuperior(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome      string  `json:"nome"      binding:"required"`
		Descricao *string `json:"descricao"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campo obrigatório: nome"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// 🔒 Apenas academias do tipo "superior" podem criar categorias
	if academiaDTO.Type != "superior" {
		utils.RespondWithForbiddenError(c, "apenas universidades (tipo 'superior') podem criar categorias de nota")
		return
	}

	categoriasExistentes := carregarCategoriasAdicionais(c, academiaDTO.CodigoAcademia)

	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	academia := academiaAgg.(*aggregates.Academia)

	if err := academia.AdicionarCategoriaNotaSuperior(req.Nome, req.Descricao, categoriasExistentes); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(academia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Categoria de nota criada: %s — academia %s", req.Nome, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusCreated, gin.H{
		"message":   "categoria criada com sucesso",
		"nome":      req.Nome,
		"descricao": req.Descricao,
	})
}

// ============================================================================
// GET /academia/categorias-nota
// ============================================================================

func ListarCategoriasNota(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// 🔒 Apenas academias do tipo "superior" têm categorias adicionais
	if academiaDTO.Type != "superior" {
		utils.RespondWithForbiddenError(c, "apenas universidades (tipo 'superior') possuem categorias de nota")
		return
	}

	categoriasProj := getCategoriasNotaProjection(c)
	categorias, err := categoriasProj.ListarPorAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"categorias": categorias,
		"total":      len(categorias),
	})
}

// ============================================================================
// Helper — carrega nomes das categorias adicionais da academia
// ============================================================================

func carregarCategoriasAdicionais(c *gin.Context, codigoAcademia string) []string {
	categoriasProj := getCategoriasNotaProjection(c)
	categorias, err := categoriasProj.ListarPorAcademia(codigoAcademia)
	if err != nil {
		return []string{}
	}
	nomes := make([]string, 0, len(categorias))
	for _, cat := range categorias {
		nomes = append(nomes, cat.Nome)
	}
	return nomes
}