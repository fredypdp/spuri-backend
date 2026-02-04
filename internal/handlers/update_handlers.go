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
// ESTUDANTE
// ============================================================================

func AtualizarDadosPessoaisEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosPessoaisEstudante - userID: %s", userID)

	var req struct {
		Nome                  *string `json:"nome"`
		Email                 *string `json:"email"`
		Telefone              *string `json:"telefone"`
		BilheteIdentidade     *string `json:"bilhete_identidade"`
		BilheteIdentidadeResp *string `json:"bilhete_identidade_responsavel"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarDadosPessoais(req.Nome, req.Email, req.Telefone, req.BilheteIdentidade, req.BilheteIdentidadeResp); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar"})
		return
	}

	response := gin.H{"message": "dados atualizados"}
	if req.Email != nil {
		response["aviso"] = "email alterado - verificação necessária"
		response["email_verificado"] = false
	}

	log.Printf("[DEBUG] Sucesso")
	c.JSON(http.StatusOK, response)
}

// 🔥 ATUALIZADO
func AtualizarDadosAcademicosEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosAcademicosEstudante - userID: %s", userID)

	var req struct {
		AnoEscolar      *string    `json:"ano_escolar"`
		AnoSuperior     *string    `json:"ano_superior"`
		CursoMedioID    *uuid.UUID `json:"curso_medio_id"`    // 🔥 MUDOU
		CursoSuperiorID *uuid.UUID `json:"curso_superior_id"` // 🔥 MUDOU
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// 🔥 VALIDAR CURSOS SE FORNECIDOS
	if req.CursoMedioID != nil && *req.CursoMedioID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso_medio_id não encontrado"})
			return
		}
		if curso.Type != "medio" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso_medio_id deve ser do tipo 'medio'"})
			return
		}
	}

	if req.CursoSuperiorID != nil && *req.CursoSuperiorID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoSuperiorID)
		if curso == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso_superior_id não encontrado"})
			return
		}
		if curso.Type != "superior" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "curso_superior_id deve ser do tipo 'superior'"})
			return
		}
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
	// 🔥 MUDOU - passar UUIDs
	if err := estudante.AtualizarDadosAcademicos(req.AnoEscolar, req.AnoSuperior, req.CursoMedioID, req.CursoSuperiorID); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar"})
		return
	}

	log.Printf("[DEBUG] Sucesso")
	c.JSON(http.StatusOK, gin.H{"message": "dados acadêmicos atualizados"})
}

// ============================================================================
// ACADEMIA
// ============================================================================

func AtualizarDadosAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosAcademia - userID: %s", userID)

	var req struct {
		Nome           *string  `json:"nome"`
		Provincia      *string  `json:"provincia"`
		Endereco       *string  `json:"endereco"`
		NumeroTelefone *string  `json:"numero_telefone"`
		Email          *string  `json:"email"`
		Website        *string  `json:"website"`
		NivelEscolar   *string  `json:"nivel_escolar"`
		Cursos         []string `json:"cursos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	repository := getRepository(c)
	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.AtualizarDados(req.Nome, req.Provincia, req.Endereco, req.NumeroTelefone, req.Email, req.Website, req.NivelEscolar, req.Cursos); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar"})
		return
	}

	response := gin.H{"message": "dados atualizados"}
	if req.Email != nil {
		response["aviso"] = "email alterado - verificação necessária"
		response["email_verificado"] = false
	}

	log.Printf("[DEBUG] Sucesso")
	c.JSON(http.StatusOK, response)
}

// ============================================================================
// ADMIN
// ============================================================================

func AtualizarDadosAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosAdmin - userID: %s", userID)

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	var req struct {
		Nome  *string `json:"nome"`
		Email *string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	repository := getRepository(c)
	adminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	admin := adminAgg.(*aggregates.Admin)
	if err := admin.AtualizarDados(req.Nome, req.Email, userID); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(admin); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar"})
		return
	}

	log.Printf("[DEBUG] Sucesso")
	c.JSON(http.StatusOK, gin.H{"message": "dados atualizados"})
}

func AtualizarRoleAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarRoleAdmin - userID: %s", userID)

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	var req struct {
		NovoRole string `json:"novo_role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "novo_role é obrigatório"})
		return
	}

	adminProj := getAdminProjection(c)
	currentAdmin, err := adminProj.GetByID(userID)
	if err != nil || currentAdmin == nil || currentAdmin.Role != "fpp" {
		log.Printf("[DEBUG] Não autorizado")
		c.JSON(http.StatusForbidden, gin.H{"error": "apenas FPP pode alterar roles"})
		return
	}

	repository := getRepository(c)
	adminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	admin := adminAgg.(*aggregates.Admin)
	roleAnterior := admin.Role

	if err := admin.AtualizarRole(req.NovoRole, userID, currentAdmin.Role); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(admin); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar"})
		return
	}

	log.Printf("[DEBUG] Sucesso - %s -> %s", roleAnterior, req.NovoRole)
	c.JSON(http.StatusOK, gin.H{"message": "role atualizado", "role_anterior": roleAnterior, "novo_role": req.NovoRole})
}

// ============================================================================
// CURSO
// ============================================================================

func AtualizarDadosCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosCurso - userID: %s", userID)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	var req struct {
		Nome  *string  `json:"nome"`
		Type  *string  `json:"type"`
		Nivel []string `json:"nivel"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		log.Printf("[DEBUG] Curso não encontrado")
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		log.Printf("[DEBUG] Curso não pertence à academia")
		c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	curso := cursoAgg.(*aggregates.Curso)
	if err := curso.AtualizarDados(req.Nome, req.Type, req.Nivel); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(curso); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar"})
		return
	}

	log.Printf("[DEBUG] Sucesso: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "curso atualizado", "nome": curso.Nome})
}

// ============================================================================
// MATÉRIA
// ============================================================================

func AtualizarDadosMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosMateria - userID: %s", userID)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	var req struct {
		Nome *string `json:"nome"`
		Type *string `json:"type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		log.Printf("[DEBUG] Matéria não encontrada")
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		log.Printf("[DEBUG] Matéria não pertence à academia")
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}

	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)
	if err := materia.AtualizarDados(req.Nome, req.Type); err != nil {
		log.Printf("[DEBUG] Erro: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(materia); err != nil {
		log.Printf("[DEBUG] Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar"})
		return
	}

	log.Printf("[DEBUG] Sucesso: %s", materia.Nome)
	c.JSON(http.StatusOK, gin.H{"message": "matéria atualizada", "nome": materia.Nome})
}