// ============================================================================
// ARQUIVO: internal/handlers/profile_handlers.go
// ============================================================================

package handlers

import (
	"fmt"
	"net/http"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetAdminPorEmail consulta um administrador pelo e-mail.
// Rota: GET /admin/consultar-admin/:email
func GetAdminPorEmail(c *gin.Context) {
	email := c.Param("email")

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(email)
	if err != nil || admin == nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"admin": gin.H{
			"id":                     admin.ID,
			"nome":                   admin.Nome,
			"email":                  admin.Email,
			"email_verificado":       admin.EmailVerificado,
			"role":                   admin.Role,
			"status":                 admin.Status,
			"created_by":             admin.CreatedBy,
			"created_at":             admin.CreatedAt,
			"updated_at":             admin.UpdatedAt,
			"total_acoes_realizadas": admin.TotalAcoesRealizadas,
			"version":                admin.Version,
		},
	})
}

// BuscarUsuario localiza qualquer entidade (estudante, academia ou admin) por UUID.
// Rota: GET /admin/buscar-usuario?tipo=estudante&id=<uuid>
func BuscarUsuario(c *gin.Context) {
	tipo := c.Query("tipo")
	id := c.Query("id")

	if tipo == "" || id == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("parâmetros 'tipo' e 'id' são obrigatórios"))
		return
	}

	userID, err := uuid.Parse(id)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID inválido"))
		return
	}

	switch tipo {
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		c.JSON(http.StatusOK, gin.H{"tipo": "estudante", "dados": estudante})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academia, err := academiaProj.GetByID(userID)
		if err != nil || academia == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		c.JSON(http.StatusOK, gin.H{"tipo": "academia", "dados": academia})

	case "admin":
		adminProj := getAdminProjection(c)
		admin, err := adminProj.GetByID(userID)
		if err != nil || admin == nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}
		c.JSON(http.StatusOK, gin.H{"tipo": "admin", "dados": admin})

	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo inválido. Use: estudante, academia ou admin"))
	}
}

func GetMeuPerfil(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	switch userType {
	case "estudante":
		getPerfilEstudante(c, userID)
	case "academia":
		getPerfilAcademia(c, userID)
	case "admin":
		getPerfilAdmin(c, userID)
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
	}
}

func getPerfilEstudante(c *gin.Context, userID interface{}) {
	estudanteProj := getEstudanteProjection(c)

	id, ok := userID.(uuid.UUID)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao processar ID do usuário"))
		return
	}

	estudante, err := estudanteProj.GetByID(id)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	var academiaInfo *gin.H
	if estudante.CodigoAcademia != nil {
		academiaProj := getAcademiaProjection(c)
		academia, _ := academiaProj.GetByCodigo(*estudante.CodigoAcademia)
		if academia != nil {
			academiaInfo = &gin.H{
				"codigo": academia.CodigoAcademia,
				"nome":   academia.Nome,
				"tipo":   academia.Type,
			}
		}
	}

	var cursoMedioInfo *gin.H
	var cursoSuperiorInfo *gin.H

	cursosProj := getCursosProjection(c)

	if estudante.CursoMedioID != nil {
		cursoMedio, _ := cursosProj.GetByID(*estudante.CursoMedioID)
		if cursoMedio != nil {
			cursoMedioInfo = &gin.H{
				"id":     cursoMedio.ID,
				"nome":   cursoMedio.Nome,
				"type":   cursoMedio.Type,
				"status": cursoMedio.Status,
			}
		}
	}

	if estudante.CursoSuperiorID != nil {
		cursoSuperior, _ := cursosProj.GetByID(*estudante.CursoSuperiorID)
		if cursoSuperior != nil {
			cursoSuperiorInfo = &gin.H{
				"id":     cursoSuperior.ID,
				"nome":   cursoSuperior.Nome,
				"type":   cursoSuperior.Type,
				"status": cursoSuperior.Status,
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"tipo": "estudante",
		"estudante": gin.H{
			"id":                             estudante.ID,
			"nome":                           estudante.Nome,
			"codigo_estudante":               estudante.CodigoEstudante,
			"email":                          estudante.Email,
			"telefone":                       estudante.Telefone,
			"email_verificado":               estudante.EmailVerificado,
			"bilhete_identidade":             estudante.BilheteIdentidade,
			"bilhete_identidade_responsavel": estudante.BilheteIdentidadeResp,
			"codigo_academia":                estudante.CodigoAcademia,
			"academia_info":                  academiaInfo,
			"status":                         estudante.Status,
			"status_escolar_fundamental":     estudante.StatusEscolarFundamental,
			"status_escolar_medio":           estudante.StatusEscolarMedio,
			"status_superior":                estudante.StatusSuperior,
			"ano_escolar":                    estudante.AnoEscolar,
			"ano_escolar_medio":              estudante.AnoEscolarMedio,
			"ano_superior":                   estudante.AnoSuperior,
			"curso_medio_id":                 estudante.CursoMedioID,
			"curso_medio_info":               cursoMedioInfo,
			"curso_superior_id":              estudante.CursoSuperiorID,
			"curso_superior_info":            cursoSuperiorInfo,
			"created_at":                     estudante.CreatedAt,
			"updated_at":                     estudante.UpdatedAt,
			"total_notas":                    estudante.TotalNotas,
			"total_faltas":                   estudante.TotalFaltas,
			"total_inscricoes":               estudante.TotalInscricoes,
			"version":                        estudante.Version,
		},
	})
}

func getPerfilAcademia(c *gin.Context, userID interface{}) {
	academiaProj := getAcademiaProjection(c)

	id, ok := userID.(uuid.UUID)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao processar ID do usuário"))
		return
	}

	academia, err := academiaProj.GetByID(id)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tipo": "academia",
		"academia": gin.H{
			"id":              academia.ID,
			"type":            academia.Type,
			"nome":            academia.Nome,
			"codigo_academia": academia.CodigoAcademia,
			"email":           academia.Email,
			"email_verificado": academia.EmailVerificado,
			"provincia":       academia.Provincia,
			"endereco":        academia.Endereco,
			"numero_telefone": academia.NumeroTelefone,
			"website":         academia.Website,
			"nivel_escolar":   academia.NivelEscolar,
			"anos_academicos": academia.AnosAcademicos,
			"status":          academia.Status,
			"cursos":          academia.Cursos,
			"created_at":      academia.CreatedAt,
			"updated_at":      academia.UpdatedAt,
			"total_estudantes": academia.TotalEstudantes,
			"version":         academia.Version,
		},
	})
}

func getPerfilAdmin(c *gin.Context, userID interface{}) {
	adminProj := getAdminProjection(c)

	id, ok := userID.(uuid.UUID)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao processar ID do usuário"))
		return
	}

	admin, err := adminProj.GetByID(id)
	if err != nil || admin == nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tipo": "admin",
		"admin": gin.H{
			"id":                     admin.ID,
			"nome":                   admin.Nome,
			"email":                  admin.Email,
			"email_verificado":       admin.EmailVerificado,
			"role":                   admin.Role,
			"status":                 admin.Status,
			"created_by":             admin.CreatedBy,
			"created_at":             admin.CreatedAt,
			"updated_at":             admin.UpdatedAt,
			"total_acoes_realizadas": admin.TotalAcoesRealizadas,
			"version":                admin.Version,
		},
	})
}

func GetEstudantePorCodigo(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	codigoEstudante := c.Param("codigo")

	if userType != "academia" && userType != "admin" {
		utils.RespondWithForbiddenError(c, "Acesso negado. Apenas academias e administradores podem consultar estudantes.")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	var academiaInfo *gin.H
	if estudante.CodigoAcademia != nil {
		academiaProj := getAcademiaProjection(c)
		academia, _ := academiaProj.GetByCodigo(*estudante.CodigoAcademia)
		if academia != nil {
			academiaInfo = &gin.H{
				"codigo":        academia.CodigoAcademia,
				"nome":          academia.Nome,
				"tipo":          academia.Type,
				"provincia":     academia.Provincia,
				"nivel_escolar": academia.NivelEscolar,
			}
		}
	}

	var cursoMedioInfo *gin.H
	var cursoSuperiorInfo *gin.H

	cursosProj := getCursosProjection(c)

	if estudante.CursoMedioID != nil {
		cursoMedio, _ := cursosProj.GetByID(*estudante.CursoMedioID)
		if cursoMedio != nil {
			cursoMedioInfo = &gin.H{
				"id":     cursoMedio.ID,
				"nome":   cursoMedio.Nome,
				"type":   cursoMedio.Type,
				"status": cursoMedio.Status,
			}
		}
	}

	if estudante.CursoSuperiorID != nil {
		cursoSuperior, _ := cursosProj.GetByID(*estudante.CursoSuperiorID)
		if cursoSuperior != nil {
			cursoSuperiorInfo = &gin.H{
				"id":     cursoSuperior.ID,
				"nome":   cursoSuperior.Nome,
				"type":   cursoSuperior.Type,
				"status": cursoSuperior.Status,
			}
		}
	}

	userID, _ := middleware.GetUserID(c)
	c.JSON(http.StatusOK, gin.H{
		"estudante": gin.H{
			"id":                             estudante.ID,
			"nome":                           estudante.Nome,
			"codigo_estudante":               estudante.CodigoEstudante,
			"email":                          estudante.Email,
			"telefone":                       estudante.Telefone,
			"email_verificado":               estudante.EmailVerificado,
			"bilhete_identidade":             estudante.BilheteIdentidade,
			"bilhete_identidade_responsavel": estudante.BilheteIdentidadeResp,
			"codigo_academia":                estudante.CodigoAcademia,
			"academia_info":                  academiaInfo,
			"status":                         estudante.Status,
			"status_escolar_fundamental":     estudante.StatusEscolarFundamental,
			"status_escolar_medio":           estudante.StatusEscolarMedio,
			"status_superior":                estudante.StatusSuperior,
			"ano_escolar":                    estudante.AnoEscolar,
			"ano_escolar_medio":              estudante.AnoEscolarMedio,
			"ano_superior":                   estudante.AnoSuperior,
			"curso_medio_id":                 estudante.CursoMedioID,
			"curso_medio_info":               cursoMedioInfo,
			"curso_superior_id":              estudante.CursoSuperiorID,
			"curso_superior_info":            cursoSuperiorInfo,
			"created_at":                     estudante.CreatedAt,
			"updated_at":                     estudante.UpdatedAt,
			"total_notas":                    estudante.TotalNotas,
			"total_faltas":                   estudante.TotalFaltas,
			"total_inscricoes":               estudante.TotalInscricoes,
			"version":                        estudante.Version,
		},
		"consultado_por": userType,
		"consultado_por_id": userID,
	})
}

func GetAcademiaPorCodigo(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	codigoAcademia := c.Param("codigo")

	if userType != "academia" && userType != "admin" {
		utils.RespondWithForbiddenError(c, "Acesso negado. Apenas academias e administradores podem consultar academias.")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	var estatisticas *gin.H
	if userType == "admin" {
		client := getDbClient(c)

		var stats struct {
			TotalNotasRegistradas  int
			TotalFaltasRegistradas int
		}

		err := client.DB().QueryRow(`
			SELECT
				(SELECT COUNT(*) FROM projection_notas WHERE codigo_academia = $1) as total_notas,
				(SELECT COUNT(*) FROM projection_faltas WHERE codigo_academia = $1) as total_faltas
		`, codigoAcademia).Scan(
			&stats.TotalNotasRegistradas,
			&stats.TotalFaltasRegistradas,
		)
		if err == nil {
			estatisticas = &gin.H{
				"total_notas_registradas":  stats.TotalNotasRegistradas,
				"total_faltas_registradas": stats.TotalFaltasRegistradas,
			}
		}
	}

	response := gin.H{
		"academia": gin.H{
			"id":               academia.ID,
			"type":             academia.Type,
			"nome":             academia.Nome,
			"codigo_academia":  academia.CodigoAcademia,
			"email":            academia.Email,
			"email_verificado": academia.EmailVerificado,
			"provincia":        academia.Provincia,
			"endereco":         academia.Endereco,
			"numero_telefone":  academia.NumeroTelefone,
			"website":          academia.Website,
			"nivel_escolar":    academia.NivelEscolar,
			"anos_academicos":  academia.AnosAcademicos,
			"status":           academia.Status,
			"cursos":           academia.Cursos,
			"created_at":       academia.CreatedAt,
			"updated_at":       academia.UpdatedAt,
			"total_estudantes": academia.TotalEstudantes,
			"version":          academia.Version,
		},
	}

	if estatisticas != nil {
		response["estatisticas"] = estatisticas
	}

	c.JSON(http.StatusOK, response)
}