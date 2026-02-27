package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// ESTUDANTE
// ============================================================================

func AtualizarDadosPessoaisEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome                  *string `json:"nome"`
		Email                 *string `json:"email"`
		Telefone              *string `json:"telefone"`
		BilheteIdentidade     *string `json:"bilhete_identidade"`
		BilheteIdentidadeResp *string `json:"bilhete_identidade_responsavel"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarDadosPessoais(req.Nome, req.Email, req.Telefone, req.BilheteIdentidade, req.BilheteIdentidadeResp); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	response := gin.H{"message": "dados pessoais atualizados com sucesso"}
	if req.Email != nil {
		response["aviso"] = "Email alterado. Verificação necessária."
		response["email_verificado"] = false
	}

	log.Printf("Dados pessoais atualizados: %s", estudante.CodigoEstudante)
	c.JSON(http.StatusOK, response)
}

func AtualizarDadosAcademicosEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		AnoEscolar      *string    `json:"ano_escolar"`
		AnoEscolarMedio *string    `json:"ano_escolar_medio"`
		AnoSuperior     *string    `json:"ano_superior"`
		CursoMedioID    *uuid.UUID `json:"curso_medio_id"`
		CursoSuperiorID *uuid.UUID `json:"curso_superior_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	if req.CursoMedioID != nil && *req.CursoMedioID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso == nil {
			utils.RespondWithNotFoundError(c, "curso médio")
			return
		}
		if curso.Type != "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id deve ser do tipo 'medio'"))
			return
		}
	}

	if req.CursoSuperiorID != nil && *req.CursoSuperiorID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoSuperiorID)
		if curso == nil {
			utils.RespondWithNotFoundError(c, "curso superior")
			return
		}
		if curso.Type != "superior" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id deve ser do tipo 'superior'"))
			return
		}
	}

	if req.CursoMedioID != nil && req.AnoEscolar != nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso != nil {
			anoValido := false
			for _, nivelCurso := range curso.Nivel {
				if nivelCurso == *req.AnoEscolar {
					anoValido = true
					break
				}
			}
			if !anoValido {
				utils.RespondWithValidationError(c, fmt.Errorf("ano escolar '%s' não existe no curso selecionado", *req.AnoEscolar))
				return
			}
		}
	}

	if req.CursoSuperiorID != nil && req.AnoSuperior != nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoSuperiorID)
		if curso != nil {
			anoValido := false
			for _, nivelCurso := range curso.Nivel {
				if nivelCurso == *req.AnoSuperior {
					anoValido = true
					break
				}
			}
			if !anoValido {
				utils.RespondWithValidationError(c, fmt.Errorf("ano superior '%s' não existe no curso selecionado", *req.AnoSuperior))
				return
			}
		}
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	
	if err := estudante.AtualizarDadosAcademicos(req.AnoEscolar, req.AnoEscolarMedio, req.AnoSuperior, req.CursoMedioID, req.CursoSuperiorID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Dados acadêmicos atualizados: %s", estudante.CodigoEstudante)
	c.JSON(http.StatusOK, gin.H{"message": "dados acadêmicos atualizados com sucesso"})
}

// ============================================================================
// ACADEMIA
// ============================================================================

func AtualizarDadosAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome           *string  `json:"nome"`
		Provincia      *string  `json:"provincia"`
		Endereco       *string  `json:"endereco"`
		NumeroTelefone *string  `json:"numero_telefone"`
		Email          *string  `json:"email"`
		Website        *string  `json:"website"`
		NivelEscolar   *string  `json:"nivel_escolar"`
		Cursos         []string `json:"cursos"`
		// AnosAcademicos — enviar apenas quando quiser alterar.
		// Para escolas fundamental/misto: obrigatório se nivelEscolar também for alterado para esses valores.
		// Omitir o campo (nil) para não alterar os anos cadastrados.
		AnosAcademicos []string `json:"anos_academicos"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	repository := getRepository(c)
	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	academia := academiaAgg.(*aggregates.Academia)

	// Validação cruzada: se anos_academicos veio no body, verificar consistência.
	// O próprio aggregate faz a validação completa; aqui só garantimos que o body
	// seja semanticamente coerente antes de chamar o domínio.
	if req.AnosAcademicos != nil {
		// Determinar nivel_escolar efetivo (novo ou atual)
		nivelEfetivo := academia.NivelEscolar
		if req.NivelEscolar != nil {
			nivelEfetivo = req.NivelEscolar
		}
		if nivelEfetivo != nil && (*nivelEfetivo == "fundamental" || *nivelEfetivo == "misto") {
			if err := utils.ValidateAnosFundamental(req.AnosAcademicos); err != nil {
				utils.RespondWithValidationError(c, err)
				return
			}
		}
	}

	if err := academia.AtualizarDados(
		req.Nome,
		req.Provincia,
		req.Endereco,
		req.NumeroTelefone,
		req.Email,
		req.Website,
		req.NivelEscolar,
		req.Cursos,
		req.AnosAcademicos, // <<< NOVO — nil = não alterar
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(academia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	response := gin.H{"message": "dados da academia atualizados com sucesso"}
	if req.Email != nil {
		response["aviso"] = "Email alterado. Verificação necessária."
	}
	c.JSON(http.StatusOK, response)
}

// ============================================================================
// ADMIN
// ============================================================================

func AtualizarDadosAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de administrador inválido"))
		return
	}

	var req struct {
		Nome  *string `json:"nome"`
		Email *string `json:"email"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	repository := getRepository(c)
	adminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	admin := adminAgg.(*aggregates.Admin)
	if err := admin.AtualizarDados(req.Nome, req.Email, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(admin); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Dados do admin atualizados: %s", admin.Email)
	c.JSON(http.StatusOK, gin.H{"message": "dados do administrador atualizados com sucesso"})
}

func AtualizarRoleAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	targetID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de administrador inválido"))
		return
	}

	var req struct {
		NovoRole string `json:"novo_role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_role é obrigatório"))
		return
	}

	adminProj := getAdminProjection(c)
	currentAdmin, err := adminProj.GetByID(userID)
	if err != nil || currentAdmin == nil || currentAdmin.Role != "fpp" {
		utils.RespondWithForbiddenError(c, "Apenas FPP pode alterar roles de administradores")
		return
	}

	repository := getRepository(c)
	adminAgg, err := repository.Load(targetID, "Admin")
	if err != nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	admin := adminAgg.(*aggregates.Admin)
	roleAnterior := admin.Role

	if err := admin.AtualizarRole(req.NovoRole, userID, currentAdmin.Role); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(admin); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Role atualizado: %s -> %s (Admin: %s)", roleAnterior, req.NovoRole, admin.Email)
	c.JSON(http.StatusOK, gin.H{
		"message":       "role atualizado com sucesso",
		"role_anterior": roleAnterior,
		"novo_role":     req.NovoRole,
	})
}

// ============================================================================
// CURSO
// ============================================================================

func AtualizarDadosCurso(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	cursoID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de curso inválido"))
		return
	}

	var req struct {
		Nome  *string  `json:"nome"`
		Type  *string  `json:"type"`
		Nivel []string `json:"nivel"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
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
		utils.RespondWithForbiddenError(c, "Curso não pertence a esta academia")
		return
	}

	repository := getRepository(c)
	cursoAgg, err := repository.Load(cursoID, "Curso")
	if err != nil {
		utils.RespondWithNotFoundError(c, "curso")
		return
	}

	curso := cursoAgg.(*aggregates.Curso)
	if err := curso.AtualizarDados(req.Nome, req.Type, req.Nivel); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(curso); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Curso atualizado: %s", curso.Nome)
	c.JSON(http.StatusOK, gin.H{
		"message": "curso atualizado com sucesso",
		"nome":    curso.Nome,
	})
}

// ============================================================================
// MATÉRIA
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
		Type *string `json:"type"`
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
	if err := materia.AtualizarDados(req.Nome, req.Type); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(materia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Matéria atualizada: %s", materia.Nome)
	c.JSON(http.StatusOK, gin.H{
		"message": "matéria atualizada com sucesso",
		"nome":    materia.Nome,
	})
}