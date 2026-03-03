package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// ============================================================================
// POST /academia/notas-aluno
// ============================================================================

func RegistrarNota(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante      string  `json:"codigo_estudante"       binding:"required"`
		AnoLectivo           string  `json:"ano_lectivo"            binding:"required"`
		Periodo              string  `json:"periodo"                binding:"required"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
		Tipo                 string  `json:"tipo"                   binding:"required"`
		Categoria            string  `json:"categoria"              binding:"required"`
		Nota                 float64 `json:"nota"`
		Observacao           *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"campos obrigatorios: codigo_estudante, ano_lectivo, periodo, "+
				"materia_disciplinar_id, tipo, categoria, nota",
		))
		return
	}

	// Academia autenticada
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// Validar tipo de nota vs tipo de academia
	tipoEsperado := map[string]string{
		"escola":   "escolar",
		"superior": "superior",
	}[academiaDTO.Type]

	if req.Tipo != tipoEsperado {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"academia do tipo '%s' so pode registrar notas do tipo '%s'",
			academiaDTO.Type, tipoEsperado,
		))
		return
	}

	// Estudante
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante nao pertence a esta academia")
		return
	}

	// Materia
	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("materia_disciplinar_id invalido"))
		return
	}
	materiasProj := getMateriasProjection(c)
	materiaDTO, _ := materiasProj.GetByID(materiaID)
	if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "materia nao pertence a esta academia")
		return
	}

	if req.Tipo == aggregates.TipoSuperior {
		if materiaDTO.Status != "ativo" {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"materia '%s' esta inativa. Ative-a antes de registrar notas",
				materiaDTO.Nome,
			))
			return
		}
		if materiaDTO.Periodo == nil || *materiaDTO.Periodo == "" {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"materia '%s' nao possui periodo definido",
				materiaDTO.Nome,
			))
			return
		}
		if req.Periodo != *materiaDTO.Periodo {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"periodo '%s' invalido para a materia '%s'. Periodo definido: '%s'",
				req.Periodo, materiaDTO.Nome, *materiaDTO.Periodo,
			))
			return
		}
	}

	// Resolver periodos validos para este contexto
	periodosValidos, err := resolverPeriodosValidos(c, req.Tipo, materiaDTO.CursoID)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Inferir AnoAcademico
	anoAcademico, err := inferirAnoAcademicoParaNota(estudanteDTO.AnoEscolar, materiaDTO.AnosAcademicos, materiaDTO.Nome)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Categorias adicionais (apenas para superior)
	var categoriasAdicionais []string
	if req.Tipo == aggregates.TipoSuperior {
		categoriasAdicionais = carregarCategoriasAdicionais(c, academiaDTO.CodigoAcademia)
	}

	// Aggregate e comando
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
		anoAcademico,
		req.Periodo,
		materiaID,
		req.Tipo,
		req.Categoria,
		req.Nota,
		req.Observacao,
		categoriasAdicionais,
		periodosValidos,
	)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Nota registrada: %s - %.2f [%s/%s] em %s (ano_academico=%s, periodo=%s)",
		req.CodigoEstudante, req.Nota, req.Tipo, req.Categoria, materiaDTO.Nome, anoAcademico, req.Periodo)

	c.JSON(http.StatusCreated, gin.H{
		"message":          "nota registrada com sucesso",
		"estudante":        req.CodigoEstudante,
		"materia":          materiaDTO.Nome,
		"tipo":             req.Tipo,
		"categoria":        req.Categoria,
		"nota":             req.Nota,
		"ano_academico":    anoAcademico,
		"periodo":          req.Periodo,
		"periodos_validos": periodosValidos,
	})
}

// ============================================================================
// PUT /academia/atualizar-nota
// ============================================================================

func AtualizarNota(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		ID         string  `json:"id"         binding:"required"`
		NotaNova   float64 `json:"nota_nova"`
		Observacao string  `json:"observacao" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"campos obrigatorios: id, nota_nova, observacao",
		))
		return
	}

	if req.NotaNova < 0 || req.NotaNova > 20 {
		utils.RespondWithValidationError(c, fmt.Errorf("nota_nova deve estar entre 0 e 20"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	notasProj := getNotasProjection(c)
	notaAtual, err := notasProj.GetNotaByID(req.ID)
	if err != nil || notaAtual == nil {
		utils.RespondWithNotFoundError(c, "nota")
		return
	}

	if notaAtual.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "nota nao pertence a esta academia")
		return
	}

	materiaID, _ := uuid.Parse(notaAtual.MateriaDisciplinarID)

	// Resolver periodos validos com base na materia da nota
	materiasProj := getMateriasProjection(c)
	materiaDTO, _ := materiasProj.GetByID(materiaID)

	if notaAtual.Tipo == aggregates.TipoSuperior && materiaDTO != nil {
		if materiaDTO.Periodo == nil || *materiaDTO.Periodo == "" {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"materia '%s' nao possui periodo definido",
				materiaDTO.Nome,
			))
			return
		}
		if notaAtual.Periodo != *materiaDTO.Periodo {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"periodo da nota ('%s') nao corresponde ao periodo da materia ('%s')",
				notaAtual.Periodo, *materiaDTO.Periodo,
			))
			return
		}
	}

	var cursoIDPtr *uuid.UUID
	if materiaDTO != nil && materiaDTO.CursoID != nil {
		cursoIDPtr = materiaDTO.CursoID
	}

	periodosValidos, err := resolverPeriodosValidos(c, notaAtual.Tipo, cursoIDPtr)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	var categoriasAdicionais []string
	if notaAtual.Tipo == aggregates.TipoSuperior {
		categoriasAdicionais = carregarCategoriasAdicionais(c, academiaDTO.CodigoAcademia)
	}

	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(notaAtual.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
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
		notaAtual.AnoLectivo,
		notaAtual.Periodo,
		materiaID,
		notaAtual.Tipo,
		notaAtual.Categoria,
		notaAtual.Nota,
		req.NotaNova,
		req.Observacao,
		categoriasAdicionais,
		periodosValidos,
	)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Nota atualizada: %s [%s/%s] %.2f -> %.2f",
		notaAtual.CodigoEstudante, notaAtual.Tipo, notaAtual.Categoria, notaAtual.Nota, req.NotaNova)

	c.JSON(http.StatusOK, gin.H{
		"message":       "nota atualizada com sucesso",
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
		utils.RespondWithValidationError(c, fmt.Errorf("campo obrigatorio: nome"))
		return
	}

	nome := strings.ToLower(strings.TrimSpace(req.Nome))
	if !strings.HasPrefix(nome, "nota_") {
		nome = "nota_" + nome
	}
	req.Nome = nome

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

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

	log.Printf("Categoria de nota criada: %s para academia %s", req.Nome, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusCreated, gin.H{
		"message":   "categoria criada com sucesso",
		"categoria": req.Nome,
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

func GetNotasEstudante(c *gin.Context) {
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
		utils.RespondWithForbiddenError(c, "Você só pode visualizar suas próprias notas")
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
			return
		}
	}

	notasProj := getNotasProjection(c)
	notas, err := notasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"notas":            notas,
		"total":            len(notas),
	})
}

// ============================================================================
// Helpers internos
// ============================================================================

// resolverPeriodosValidos retorna a lista de periodos aceitos para o registro
// de uma nota:
//   - tipo="escolar"  -> 3 trimestres fixos
//   - tipo="superior" -> periodos do curso ao qual a materia pertence
func resolverPeriodosValidos(c *gin.Context, tipo string, cursoID *uuid.UUID) ([]string, error) {
	switch tipo {
	case aggregates.TipoEscolar:
		return aggregates.PeriodosEscolar, nil

	case aggregates.TipoSuperior:
		if cursoID == nil {
			return nil, fmt.Errorf("materia do tipo 'superior' deve estar associada a um curso")
		}
		cursosProj := getCursosProjection(c)
		cursoDTO, err := cursosProj.GetByID(*cursoID)
		if err != nil {
			return nil, fmt.Errorf("erro ao buscar curso da materia: %w", err)
		}
		if cursoDTO == nil {
			return nil, fmt.Errorf("curso associado a materia nao encontrado")
		}
		if len(cursoDTO.Periodos) == 0 {
			return nil, fmt.Errorf(
				"o curso '%s' nao possui periodos configurados. "+
					"Atualize o curso via PUT /academia/cursos/:id com o campo 'periodos'",
				cursoDTO.Nome,
			)
		}
		return cursoDTO.Periodos, nil

	default:
		return nil, fmt.Errorf("tipo de nota invalido: '%s'. Use 'escolar' ou 'superior'", tipo)
	}
}

// inferirAnoAcademicoParaNota determina o ano_academico correto para o registro de nota.
//   - anoEscolarEstudante != nil e nao vazio -> fundamental -> retorna anoEscolarEstudante
//   - caso contrario (medio ou superior) -> retorna nivelMateria[0]
func inferirAnoAcademicoParaNota(
	anoEscolarEstudante *string,
	nivelMateria []string,
	nomeMateria string,
) (string, error) {
	if anoEscolarEstudante != nil && strings.TrimSpace(*anoEscolarEstudante) != "" {
		return *anoEscolarEstudante, nil
	}

	if len(nivelMateria) == 0 {
		return "", fmt.Errorf(
			"a materia '%s' nao possui anos_academicos definidos; impossivel inferir ano_academico",
			nomeMateria,
		)
	}
	if len(nivelMateria) != 1 {
		return "", fmt.Errorf(
			"materias de medio/superior devem ter exatamente 1 ano academico, "+
				"mas a materia '%s' possui %d",
			nomeMateria, len(nivelMateria),
		)
	}

	return nivelMateria[0], nil
}

// carregarCategoriasAdicionais retorna os nomes das categorias adicionais
// cadastradas pela academia. Retorna slice vazio em caso de erro (nao fatal).
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

// inferirAnoAcademicoFaltas é idêntico a inferirAnoAcademicoParaNota —
// reutilizado no handler de faltas para manter a mesma lógica.
func inferirAnoAcademicoFaltas(
	anoEscolarEstudante *string,
	nivelMateria []string,
	nomeMateria string,
) (string, error) {
	return inferirAnoAcademicoParaNota(anoEscolarEstudante, nivelMateria, nomeMateria)
}

// validarNota verifica se a nota está no intervalo 0–20.
// Usado tanto em RegistrarNota quanto como pré-validação no handler.
func validarNota(nota float64) error {
	if nota < 0 || nota > 20 {
		return fmt.Errorf("nota deve estar entre 0 e 20, recebido: %.2f", nota)
	}
	return nil
}

// ============================================================================
// Utilitário: GetNotasEstudante (alias do handler de query)
// ============================================================================

// getNotasProjection é um helper local que obtém a NotasProjection do contexto.
// Definido em query_handlers.go; declarado aqui apenas para documentação.
// (Não redeclarar — já existe em outro arquivo do pacote handlers)

// NOTE: utils é importado via utils "spuri/internal/utils"
var _ = utils.RespondWithValidationError