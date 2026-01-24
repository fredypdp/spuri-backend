// ============================================================================
// ARQUIVO: internal/handlers/cursos_handlers.go
// Handlers para gerenciamento de Cursos e Matérias (apenas Academia)
// ============================================================================

package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// CURSOS - Criar/Listar/Ativar/Desativar
// ============================================================================

// CriarCurso cria um novo curso (apenas academia ativa)
func CriarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📚 [CRIAR-CURSO] Iniciando criação - UserID: %s", userID)

	var req struct {
		Nome  string   `json:"nome" binding:"required"`
		Type  string   `json:"type" binding:"required"`
		Nivel []string `json:"nivel" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [CRIAR-CURSO-DEBUG] Erro validação JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("🔍 [CRIAR-CURSO-DEBUG] Dados recebidos - Nome: %s, Type: %s, Níveis: %v", 
		req.Nome, req.Type, req.Nivel)

	// Buscar código da academia logada
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [CRIAR-CURSO-DEBUG] Buscando academia ID: %s", userID)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [CRIAR-CURSO-DEBUG] Erro ao buscar academia: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	log.Printf("✅ [CRIAR-CURSO-DEBUG] Academia encontrada: %s (código: %s, status: %s)", 
		academiaDTO.Nome, academiaDTO.CodigoAcademia, academiaDTO.Status)

	// Verificar se academia está ativa
	if academiaDTO.Status != "ativo" {
		log.Printf("❌ [CRIAR-CURSO-DEBUG] Academia inativa - Status: %s", academiaDTO.Status)
		c.JSON(http.StatusForbidden, gin.H{"error": "academia inativa não pode criar cursos"})
		return
	}

	// Criar agregado Curso
	repository := getRepository(c)
	curso := aggregates.NewCurso()

	log.Printf("📦 [CRIAR-CURSO-DEBUG] Criando agregado Curso...")
	if err := curso.Criar(
		req.Nome,
		req.Type,
		req.Nivel,
		academiaDTO.CodigoAcademia,
	); err != nil {
		log.Printf("❌ [CRIAR-CURSO-DEBUG] Erro ao criar agregado: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("✅ [CRIAR-CURSO-DEBUG] Agregado criado - ID: %s", curso.ID)

	// Salvar eventos
	log.Printf("💾 [CRIAR-CURSO-DEBUG] Salvando eventos - Total: %d", len(curso.UncommittedEvents))
	if err := repository.Save(curso); err != nil {
		log.Printf("❌ [CRIAR-CURSO-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao criar curso"})
		return
	}

	log.Printf("✅ [CRIAR-CURSO] Curso criado com sucesso - Nome: %s, ID: %s", req.Nome, curso.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "curso criado com sucesso",
		"data": gin.H{
			"id":   curso.ID,
			"nome": curso.Nome,
			"type": curso.Type,
		},
	})
}

// ListarCursos lista cursos da academia logada
func ListarCursos(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📚 [LISTAR-CURSOS] Iniciando listagem - UserID: %s", userID)

	// Buscar código da academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [LISTAR-CURSOS-DEBUG] Buscando academia ID: %s", userID)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [LISTAR-CURSOS-DEBUG] Erro ao buscar academia: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	log.Printf("✅ [LISTAR-CURSOS-DEBUG] Academia encontrada: %s (código: %s)", 
		academiaDTO.Nome, academiaDTO.CodigoAcademia)

	// Buscar cursos
	cursosProj := getCursosProjection(c)
	log.Printf("🔍 [LISTAR-CURSOS-DEBUG] Buscando cursos da academia: %s", academiaDTO.CodigoAcademia)
	cursos, err := cursosProj.GetByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		log.Printf("❌ [LISTAR-CURSOS-DEBUG] Erro ao buscar cursos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar cursos"})
		return
	}

	log.Printf("✅ [LISTAR-CURSOS] Cursos listados - Total: %d", len(cursos))

	c.JSON(http.StatusOK, gin.H{
		"cursos": cursos,
		"total":  len(cursos),
	})
}

// AtivarCurso ativa um curso (academia)
func AtivarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("🔓 [ATIVAR-CURSO] Iniciando ativação - UserID: %s", userID)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("❌ [ATIVAR-CURSO-DEBUG] UUID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	log.Printf("🔍 [ATIVAR-CURSO-DEBUG] Curso ID: %s", cursoID)

	// Verificar se curso pertence à academia
	cursosProj := getCursosProjection(c)
	log.Printf("🔍 [ATIVAR-CURSO-DEBUG] Buscando curso na projeção...")
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		log.Printf("❌ [ATIVAR-CURSO-DEBUG] Curso não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	log.Printf("✅ [ATIVAR-CURSO-DEBUG] Curso encontrado: %s (código academia: %s, status: %s)", 
		cursoDTO.Nome, cursoDTO.CodigoAcademia, cursoDTO.Status)

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [ATIVAR-CURSO-DEBUG] Verificando propriedade da academia...")
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		log.Printf("❌ [ATIVAR-CURSO-DEBUG] Curso não pertence à academia - Academia: %v, Curso: %s", 
			academiaDTO, cursoDTO.CodigoAcademia)
		c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
		return
	}

	log.Printf("✅ [ATIVAR-CURSO-DEBUG] Academia verificada: %s", academiaDTO.Nome)

	// Carregar agregado
	repository := getRepository(c)
	log.Printf("📦 [ATIVAR-CURSO-DEBUG] Carregando agregado curso...")
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		log.Printf("❌ [ATIVAR-CURSO-DEBUG] Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	curso := cursoAgg.(*aggregates.Curso)
	log.Printf("✅ [ATIVAR-CURSO-DEBUG] Agregado carregado - Nome: %s", curso.Nome)

	log.Printf("💾 [ATIVAR-CURSO-DEBUG] Ativando curso no agregado...")
	if err := curso.Ativar(); err != nil {
		log.Printf("❌ [ATIVAR-CURSO-DEBUG] Erro ao ativar: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("💾 [ATIVAR-CURSO-DEBUG] Salvando eventos - Total: %d", len(curso.UncommittedEvents))
	if err := repository.Save(curso); err != nil {
		log.Printf("❌ [ATIVAR-CURSO-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao ativar curso"})
		return
	}

	log.Printf("✅ [ATIVAR-CURSO] Curso ativado com sucesso - Nome: %s", curso.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message": "curso ativado com sucesso",
		"nome":    curso.Nome,
	})
}

// DesativarCurso desativa um curso (academia)
func DesativarCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("🔒 [DESATIVAR-CURSO] Iniciando desativação - UserID: %s", userID)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("❌ [DESATIVAR-CURSO-DEBUG] UUID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	log.Printf("🔍 [DESATIVAR-CURSO-DEBUG] Curso ID: %s", cursoID)

	// Verificar propriedade
	cursosProj := getCursosProjection(c)
	log.Printf("🔍 [DESATIVAR-CURSO-DEBUG] Buscando curso na projeção...")
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		log.Printf("❌ [DESATIVAR-CURSO-DEBUG] Curso não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	log.Printf("✅ [DESATIVAR-CURSO-DEBUG] Curso encontrado: %s (status: %s)", 
		cursoDTO.Nome, cursoDTO.Status)

	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [DESATIVAR-CURSO-DEBUG] Verificando propriedade da academia...")
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		log.Printf("❌ [DESATIVAR-CURSO-DEBUG] Curso não pertence à academia")
		c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
		return
	}

	log.Printf("✅ [DESATIVAR-CURSO-DEBUG] Academia verificada: %s", academiaDTO.Nome)

	// Carregar e desativar
	repository := getRepository(c)
	log.Printf("📦 [DESATIVAR-CURSO-DEBUG] Carregando agregado curso...")
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		log.Printf("❌ [DESATIVAR-CURSO-DEBUG] Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	curso := cursoAgg.(*aggregates.Curso)
	log.Printf("✅ [DESATIVAR-CURSO-DEBUG] Agregado carregado - Nome: %s", curso.Nome)

	log.Printf("💾 [DESATIVAR-CURSO-DEBUG] Desativando curso no agregado...")
	if err := curso.Desativar(); err != nil {
		log.Printf("❌ [DESATIVAR-CURSO-DEBUG] Erro ao desativar: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("💾 [DESATIVAR-CURSO-DEBUG] Salvando eventos - Total: %d", len(curso.UncommittedEvents))
	if err := repository.Save(curso); err != nil {
		log.Printf("❌ [DESATIVAR-CURSO-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao desativar curso"})
		return
	}

	log.Printf("✅ [DESATIVAR-CURSO] Curso desativado com sucesso - Nome: %s", curso.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message": "curso desativado com sucesso",
		"nome":    curso.Nome,
	})
}

// ============================================================================
// MATÉRIAS - Criar/Listar/Ativar/Desativar
// ============================================================================

// CriarMateria cria uma nova matéria (apenas academia ativa)
func CriarMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📖 [CRIAR-MATERIA] Iniciando criação - UserID: %s", userID)

	var req struct {
		Nome    string     `json:"nome" binding:"required"`
		Type    string     `json:"type" binding:"required"`
		Nivel   []string   `json:"nivel"`
		CursoID *uuid.UUID `json:"curso_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [CRIAR-MATERIA-DEBUG] Erro validação JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("🔍 [CRIAR-MATERIA-DEBUG] Dados recebidos - Nome: %s, Type: %s, CursoID: %v", 
		req.Nome, req.Type, req.CursoID)

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [CRIAR-MATERIA-DEBUG] Buscando academia ID: %s", userID)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [CRIAR-MATERIA-DEBUG] Erro ao buscar academia: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	log.Printf("✅ [CRIAR-MATERIA-DEBUG] Academia encontrada: %s (status: %s)", 
		academiaDTO.Nome, academiaDTO.Status)

	if academiaDTO.Status != "ativo" {
		log.Printf("❌ [CRIAR-MATERIA-DEBUG] Academia inativa - Status: %s", academiaDTO.Status)
		c.JSON(http.StatusForbidden, gin.H{"error": "academia inativa não pode criar matérias"})
		return
	}

	// Se medio/superior, verificar se curso existe e está ativo
	if (req.Type == "medio" || req.Type == "superior") && req.CursoID != nil {
		log.Printf("🔍 [CRIAR-MATERIA-DEBUG] Verificando curso ID: %s", *req.CursoID)
		cursosProj := getCursosProjection(c)
		cursoDTO, _ := cursosProj.GetByID(*req.CursoID)
		
		if cursoDTO == nil {
			log.Printf("❌ [CRIAR-MATERIA-DEBUG] Curso não encontrado: %s", *req.CursoID)
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso não encontrado"})
			return
		}
		
		log.Printf("✅ [CRIAR-MATERIA-DEBUG] Curso encontrado: %s (status: %s)", 
			cursoDTO.Nome, cursoDTO.Status)
		
		if cursoDTO.Status != "ativo" {
			log.Printf("❌ [CRIAR-MATERIA-DEBUG] Curso inativo - Status: %s", cursoDTO.Status)
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso inativo não pode ter matérias"})
			return
		}
		
		if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
			log.Printf("❌ [CRIAR-MATERIA-DEBUG] Curso não pertence à academia - Curso: %s, Academia: %s", 
				cursoDTO.CodigoAcademia, academiaDTO.CodigoAcademia)
			c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
			return
		}
	}

	// Criar agregado MateriaDisciplinar
	repository := getRepository(c)
	materia := aggregates.NewMateriaDisciplinar()

	log.Printf("📦 [CRIAR-MATERIA-DEBUG] Criando agregado MateriaDisciplinar...")
	if err := materia.Criar(
		req.Nome,
		req.Type,
		req.Nivel,
		academiaDTO.CodigoAcademia,
		req.CursoID,
	); err != nil {
		log.Printf("❌ [CRIAR-MATERIA-DEBUG] Erro ao criar agregado: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("✅ [CRIAR-MATERIA-DEBUG] Agregado criado - ID: %s", materia.ID)

	// Salvar eventos
	log.Printf("💾 [CRIAR-MATERIA-DEBUG] Salvando eventos - Total: %d", len(materia.UncommittedEvents))
	if err := repository.Save(materia); err != nil {
		log.Printf("❌ [CRIAR-MATERIA-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao criar matéria"})
		return
	}

	log.Printf("✅ [CRIAR-MATERIA] Matéria criada com sucesso - Nome: %s, ID: %s", req.Nome, materia.ID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "matéria criada com sucesso",
		"data": gin.H{
			"id":   materia.ID,
			"nome": materia.Nome,
			"type": materia.Type,
		},
	})
}

// ListarMaterias lista matérias da academia
func ListarMaterias(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📖 [LISTAR-MATERIAS] Iniciando listagem - UserID: %s", userID)

	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [LISTAR-MATERIAS-DEBUG] Buscando academia ID: %s", userID)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [LISTAR-MATERIAS-DEBUG] Erro ao buscar academia: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}

	log.Printf("✅ [LISTAR-MATERIAS-DEBUG] Academia encontrada: %s (código: %s)", 
		academiaDTO.Nome, academiaDTO.CodigoAcademia)

	materiasProj := getMateriasProjection(c)
	log.Printf("🔍 [LISTAR-MATERIAS-DEBUG] Buscando matérias da academia: %s", academiaDTO.CodigoAcademia)
	materias, err := materiasProj.GetByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		log.Printf("❌ [LISTAR-MATERIAS-DEBUG] Erro ao buscar matérias: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar matérias"})
		return
	}

	log.Printf("✅ [LISTAR-MATERIAS] Matérias listadas - Total: %d", len(materias))

	c.JSON(http.StatusOK, gin.H{
		"materias": materias,
		"total":    len(materias),
	})
}