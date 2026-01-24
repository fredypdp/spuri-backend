// ============================================================================
// ARQUIVO: internal/handlers/update_handlers.go (NOVO)
// Handlers para atualização de dados
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// ESTUDANTE - ATUALIZAÇÃO
// ============================================================================

// AtualizarDadosPessoaisEstudante atualiza dados pessoais do estudante logado
// PUT /estudante/dados-pessoais
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
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[DEBUG] Dados recebidos: nome=%v, email=%v, telefone=%v", req.Nome, req.Email, req.Telefone)

	// Carregar agregado
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar estudante: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	log.Printf("[DEBUG] Estudante carregado: %s", estudante.ID)

	// Executar comando
	err = estudante.AtualizarDadosPessoais(
		req.Nome,
		req.Email,
		req.Telefone,
		req.BilheteIdentidade,
		req.BilheteIdentidadeResp,
	)
	if err != nil {
		log.Printf("[DEBUG] Erro ao atualizar dados pessoais: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar dados"})
		return
	}

	response := gin.H{
		"message": "dados pessoais atualizados com sucesso",
	}

	// Se email foi alterado, avisar sobre verificação
	if req.Email != nil {
		log.Printf("[DEBUG] Email alterado para: %s", *req.Email)
		response["aviso"] = "email alterado - verificação necessária"
		response["email_verificado"] = false
	}

	log.Printf("[DEBUG] Dados pessoais atualizados com sucesso")
	c.JSON(http.StatusOK, response)
}

// AtualizarDadosAcademicosEstudante atualiza dados acadêmicos do estudante logado
// PUT /estudante/dados-academicos
func AtualizarDadosAcademicosEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosAcademicosEstudante - userID: %s", userID)

	var req struct {
		AnoEscolar    *string `json:"ano_escolar"`
		AnoSuperior   *string `json:"ano_superior"`
		CursoMedio    *string `json:"curso_medio"`
		CursoSuperior *string `json:"curso_superior"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[DEBUG] Dados recebidos: ano_escolar=%v, ano_superior=%v", req.AnoEscolar, req.AnoSuperior)

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar estudante: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)

	err = estudante.AtualizarDadosAcademicos(
		req.AnoEscolar,
		req.AnoSuperior,
		req.CursoMedio,
		req.CursoSuperior,
	)
	if err != nil {
		log.Printf("[DEBUG] Erro ao atualizar dados acadêmicos: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(estudante); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar dados"})
		return
	}

	log.Printf("[DEBUG] Dados acadêmicos atualizados com sucesso")
	c.JSON(http.StatusOK, gin.H{
		"message": "dados acadêmicos atualizados com sucesso",
	})
}

// ============================================================================
// ACADEMIA - ATUALIZAÇÃO
// ============================================================================

// AtualizarDadosAcademia atualiza dados da academia logada
// PUT /academia/dados
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
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[DEBUG] Dados recebidos: nome=%v, email=%v, provincia=%v", req.Nome, req.Email, req.Provincia)

	repository := getRepository(c)
	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar academia: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	log.Printf("[DEBUG] Academia carregada: %s", academia.ID)

	err = academia.AtualizarDados(
		req.Nome,
		req.Provincia,
		req.Endereco,
		req.NumeroTelefone,
		req.Email,
		req.Website,
		req.NivelEscolar,
		req.Cursos,
	)
	if err != nil {
		log.Printf("[DEBUG] Erro ao atualizar dados da academia: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar dados"})
		return
	}

	response := gin.H{
		"message": "dados da academia atualizados com sucesso",
	}

	if req.Email != nil {
		log.Printf("[DEBUG] Email alterado para: %s", *req.Email)
		response["aviso"] = "email alterado - verificação necessária"
		response["email_verificado"] = false
	}

	log.Printf("[DEBUG] Dados da academia atualizados com sucesso")
	c.JSON(http.StatusOK, response)
}

// ============================================================================
// ADMIN - ATUALIZAÇÃO
// ============================================================================

// AtualizarDadosAdmin atualiza dados do admin (próprio ou outro)
// PUT /admin/dados/:id
func AtualizarDadosAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosAdmin - userID: %s", userID)

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("[DEBUG] ID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	log.Printf("[DEBUG] TargetID: %s", targetID)

	var req struct {
		Nome  *string `json:"nome"`
		Email *string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[DEBUG] Dados recebidos: nome=%v, email=%v", req.Nome, req.Email)

	repository := getRepository(c)
	
	// Carregar admin alvo
	adminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar admin: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	admin := adminAgg.(*aggregates.Admin)
	log.Printf("[DEBUG] Admin carregado: %s", admin.ID)

	// Executar comando
	err = admin.AtualizarDados(req.Nome, req.Email, userID)
	if err != nil {
		log.Printf("[DEBUG] Erro ao atualizar dados: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(admin); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar dados"})
		return
	}

	log.Printf("[DEBUG] Dados do admin atualizados com sucesso")
	c.JSON(http.StatusOK, gin.H{
		"message": "dados do administrador atualizados com sucesso",
	})
}

// AtualizarRoleAdmin atualiza role de um admin (APENAS FPP)
// PUT /admin/role/:id
func AtualizarRoleAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarRoleAdmin - userID: %s", userID)

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("[DEBUG] ID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	log.Printf("[DEBUG] TargetID: %s", targetID)

	var req struct {
		NovoRole string `json:"novo_role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "novo_role é obrigatório"})
		return
	}

	log.Printf("[DEBUG] Novo role: %s", req.NovoRole)

	// Verificar se usuário é FPP
	adminProj := getAdminProjection(c)
	currentAdmin, err := adminProj.GetByID(userID)
	if err != nil || currentAdmin == nil {
		log.Printf("[DEBUG] Erro ao buscar admin atual: %v", err)
		c.JSON(http.StatusForbidden, gin.H{"error": "administrador não encontrado"})
		return
	}

	log.Printf("[DEBUG] Admin atual - Role: %s", currentAdmin.Role)

	if currentAdmin.Role != "fpp" {
		log.Printf("[DEBUG] Usuário não é FPP")
		c.JSON(http.StatusForbidden, gin.H{"error": "apenas FPP pode alterar roles"})
		return
	}

	repository := getRepository(c)
	
	// Carregar admin alvo
	adminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar admin alvo: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	admin := adminAgg.(*aggregates.Admin)
	roleAnterior := admin.Role
	log.Printf("[DEBUG] Role anterior: %s", roleAnterior)

	// Executar comando
	err = admin.AtualizarRole(req.NovoRole, userID, currentAdmin.Role)
	if err != nil {
		log.Printf("[DEBUG] Erro ao atualizar role: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(admin); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar role"})
		return
	}

	log.Printf("[DEBUG] Role atualizado com sucesso - Anterior: %s, Novo: %s", roleAnterior, req.NovoRole)
	c.JSON(http.StatusOK, gin.H{
		"message":       "role atualizado com sucesso",
		"role_anterior": roleAnterior,
		"novo_role":     req.NovoRole,
	})
}

// ============================================================================
// CURSO - ATUALIZAÇÃO
// ============================================================================

// AtualizarDadosCurso atualiza dados de um curso (apenas academia)
// PUT /academia/cursos/:id
func AtualizarDadosCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosCurso - userID: %s", userID)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("[DEBUG] ID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	log.Printf("[DEBUG] CursoID: %s", cursoID)

	var req struct {
		Nome  *string  `json:"nome"`
		Type  *string  `json:"type"`
		Nivel []string `json:"nivel"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[DEBUG] Dados recebidos: nome=%v, type=%v, nivel=%v", req.Nome, req.Type, req.Nivel)

	// Verificar propriedade
	cursosProj := getCursosProjection(c)
	cursoDTO, err := cursosProj.GetByID(cursoID)
	if err != nil || cursoDTO == nil {
		log.Printf("[DEBUG] Curso não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	log.Printf("[DEBUG] Curso encontrado - CodigoAcademia: %s", cursoDTO.CodigoAcademia)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != cursoDTO.CodigoAcademia {
		log.Printf("[DEBUG] Curso não pertence à academia do usuário")
		c.JSON(http.StatusForbidden, gin.H{"error": "curso não pertence a esta academia"})
		return
	}

	// Carregar e atualizar
	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar curso: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "curso não encontrado"})
		return
	}

	curso := cursoAgg.(*aggregates.Curso)

	err = curso.AtualizarDados(req.Nome, req.Type, req.Nivel)
	if err != nil {
		log.Printf("[DEBUG] Erro ao atualizar curso: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(curso); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar curso"})
		return
	}

	log.Printf("[DEBUG] Curso atualizado com sucesso: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{
		"message": "curso atualizado com sucesso",
		"nome":    curso.Nome,
	})
}

// ============================================================================
// MATÉRIA - ATUALIZAÇÃO
// ============================================================================

// AtualizarDadosMateria atualiza dados de uma matéria (apenas academia)
// PUT /academia/materias/:id
func AtualizarDadosMateria(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] AtualizarDadosMateria - userID: %s", userID)

	materiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		log.Printf("[DEBUG] ID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	log.Printf("[DEBUG] MateriaID: %s", materiaID)

	var req struct {
		Nome *string `json:"nome"`
		Type *string `json:"type"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[DEBUG] Erro ao fazer bind do JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[DEBUG] Dados recebidos: nome=%v, type=%v", req.Nome, req.Type)

	// Verificar propriedade
	materiasProj := getMateriasProjection(c)
	materiaDTO, err := materiasProj.GetByID(materiaID)
	if err != nil || materiaDTO == nil {
		log.Printf("[DEBUG] Matéria não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	log.Printf("[DEBUG] Matéria encontrada - CodigoAcademia: %s", materiaDTO.CodigoAcademia)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, _ := academiaProj.GetByID(userID)
	if academiaDTO == nil || academiaDTO.CodigoAcademia != materiaDTO.CodigoAcademia {
		log.Printf("[DEBUG] Matéria não pertence à academia do usuário")
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}

	// Carregar e atualizar
	repository := getRepository(c)
	materiaAgg, err := repository.Load(materiaID, "MateriaDisciplinar")
	if err != nil {
		log.Printf("[DEBUG] Erro ao carregar matéria: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "matéria não encontrada"})
		return
	}

	materia := materiaAgg.(*aggregates.MateriaDisciplinar)

	err = materia.AtualizarDados(req.Nome, req.Type)
	if err != nil {
		log.Printf("[DEBUG] Erro ao atualizar matéria: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(materia); err != nil {
		log.Printf("[DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao atualizar matéria"})
		return
	}

	log.Printf("[DEBUG] Matéria atualizada com sucesso: %s", materia.Nome)
	c.JSON(http.StatusOK, gin.H{
		"message": "matéria atualizada com sucesso",
		"nome":    materia.Nome,
	})
}