package handlers

import (
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegistrarNotasRequest representa requisição de registro de notas
type RegistrarNotasRequest struct {
	EstudanteID uuid.UUID `json:"estudante_id" binding:"required"`
	AnoLectivo  string    `json:"ano_lectivo" binding:"required"`
	Periodo     string    `json:"periodo" binding:"required"`
	Materias    []struct {
		Nome string  `json:"nome" binding:"required"`
		Nota float64 `json:"nota" binding:"required"`
	} `json:"materias" binding:"required"`
}

// RegistrarNotas comando para registrar notas (CQRS - Write)
func RegistrarNotas(c *gin.Context) {
	// Apenas academias podem registrar notas
	userID, _ := middleware.GetUserID(c)

	var req RegistrarNotasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Validações
	if len(req.Materias) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "materias não pode estar vazio"})
		return
	}

	// Carregar agregado Estudante (Event Sourcing)
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(req.EstudanteID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// Verificar se estudante pertence a esta academia
	if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	// Converter materias
	materias := make([]aggregates.Materia, len(req.Materias))
	for i, m := range req.Materias {
		materias[i] = aggregates.Materia{
			Nome: m.Nome,
			Nota: m.Nota,
		}
	}

	// Executar comando (gera eventos)
	err = estudante.RegistrarNotas(
		userID,
		req.AnoLectivo,
		req.Periodo,
		materias,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos (Event Sourcing)
	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao registrar notas"})
		return
	}

	// CQRS: Não retornamos o estado, apenas confirmação
	c.JSON(http.StatusOK, gin.H{
		"message": "notas registradas com sucesso",
	})
}

// RegistrarFaltasRequest representa requisição de registro de faltas
type RegistrarFaltasRequest struct {
	EstudanteID uuid.UUID `json:"estudante_id" binding:"required"`
	AnoLectivo  string    `json:"ano_lectivo" binding:"required"`
	Periodo     string    `json:"periodo" binding:"required"`
	Materias    []struct {
		Nome   string `json:"nome" binding:"required"`
		Faltas int    `json:"faltas" binding:"required"`
	} `json:"materias" binding:"required"`
}

// RegistrarFaltas comando para registrar faltas (CQRS - Write)
func RegistrarFaltas(c *gin.Context) {
	// Apenas academias podem registrar faltas
	userID, _ := middleware.GetUserID(c)

	var req RegistrarFaltasRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Validações
	if len(req.Materias) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "materias não pode estar vazio"})
		return
	}

	// Carregar agregado Estudante
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(req.EstudanteID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// Verificar se estudante pertence a esta academia
	if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	// Converter materias
	materias := make([]aggregates.MateriaFaltas, len(req.Materias))
	for i, m := range req.Materias {
		materias[i] = aggregates.MateriaFaltas{
			Nome:   m.Nome,
			Faltas: m.Faltas,
		}
	}

	// Executar comando
	err = estudante.RegistrarFaltas(
		userID,
		req.AnoLectivo,
		req.Periodo,
		materias,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao registrar faltas"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "faltas registradas com sucesso",
	})
}

// InscricaoEscolaRequest representa solicitação de inscrição em escola
type InscricaoEscolaRequest struct {
	IDAcademia          uuid.UUID `json:"id_academia" binding:"required"`
	AnoEscolarInscricao string    `json:"ano_escolar_inscricao" binding:"required"`
	CursoMedio          *string   `json:"curso_medio"`
}

// InscricaoEscola comando para solicitar inscrição em escola
func InscricaoEscola(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req InscricaoEscolaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Carregar agregado Estudante
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// Executar comando
	err = estudante.SolicitarInscricao(
		req.IDAcademia,
		"escola",
		req.AnoEscolarInscricao,
		req.CursoMedio,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao solicitar inscrição"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "inscrição solicitada com sucesso",
	})
}

// InscricaoUniversidadeRequest representa solicitação de inscrição em universidade
type InscricaoUniversidadeRequest struct {
	IDAcademia           uuid.UUID `json:"id_academia" binding:"required"`
	AnoSuperiorInscricao string    `json:"ano_superior_inscricao" binding:"required"`
	CursoSuperior        string    `json:"curso_superior" binding:"required"`
}

// InscricaoUniversidade comando para solicitar inscrição em universidade
func InscricaoUniversidade(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req InscricaoUniversidadeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Carregar agregado
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	// Executar comando
	err = estudante.SolicitarInscricao(
		req.IDAcademia,
		"universidade",
		req.AnoSuperiorInscricao,
		&req.CursoSuperior,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao solicitar inscrição"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "inscrição solicitada com sucesso",
	})
}

// AprovarInscricao comando para aprovar inscrição
func AprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscricaoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de inscrição inválido"})
		return
	}

	// Buscar inscrição na projeção
	inscProj := getInscricoesProjection(c)
	inscricao, err := inscProj.GetByID(inscricaoID)
	if err != nil || inscricao == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada"})
		return
	}

	// Verificar se pertence a esta academia
	if inscricao.AcademiaID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "inscrição não pertence a esta academia"})
		return
	}

	// Carregar agregados
	repository := getRepository(c)

	// Carregar estudante e aprovar
	estudanteAgg, err := repository.Load(inscricao.EstudanteID, "Estudante")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	err = estudante.AprovarInscricao(userID, inscricaoID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos do estudante
	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao aprovar inscrição"})
		return
	}

	// Carregar academia e registrar aprovação
	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar academia"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	err = academia.AprovarInscricao(
		inscricao.EstudanteID,
		inscricaoID,
		inscricao.Tipo,
		inscricao.AnoInscricao,
		inscricao.Curso,
	)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos da academia
	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao registrar aprovação"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "inscrição aprovada com sucesso",
	})
}

// ReprovarInscricao comando para reprovar inscrição
func ReprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscricaoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID de inscrição inválido"})
		return
	}

	// Buscar inscrição
	inscProj := getInscricoesProjection(c)
	inscricao, err := inscProj.GetByID(inscricaoID)
	if err != nil || inscricao == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada"})
		return
	}

	// Verificar permissão
	if inscricao.AcademiaID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "inscrição não pertence a esta academia"})
		return
	}

	// Carregar e executar comando na academia
	repository := getRepository(c)
	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar academia"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	err = academia.ReprovarInscricao(inscricao.EstudanteID, inscricaoID, "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao reprovar inscrição"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "inscrição reprovada",
	})
}