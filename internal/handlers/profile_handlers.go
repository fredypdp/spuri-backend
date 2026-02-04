package handlers

import (
	"fmt"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetMeuPerfil retorna dados do usuário logado
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo de usuário inválido"})
	}
}

// 🔥 ATUALIZADO
func getPerfilEstudante(c *gin.Context, userID interface{}) {
	estudanteProj := getEstudanteProjection(c)

	id, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar ID"})
		return
	}

	estudante, err := estudanteProj.GetByID(id)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
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

	// 🔥 BUSCAR INFORMAÇÕES DOS CURSOS
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
			"status_escolar":                 estudante.StatusEscolar,
			"status_superior":                estudante.StatusSuperior,
			"ano_escolar":                    estudante.AnoEscolar,
			"ano_superior":                   estudante.AnoSuperior,
			"curso_medio_id":                 estudante.CursoMedioID,    // 🔥 MUDOU
			"curso_medio_info":               cursoMedioInfo,            // 🔥 NOVO
			"curso_superior_id":              estudante.CursoSuperiorID, // 🔥 MUDOU
			"curso_superior_info":            cursoSuperiorInfo,         // 🔥 NOVO
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar ID"})
		return
	}

	academia, err := academiaProj.GetByID(id)
	if err != nil || academia == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tipo": "academia",
		"academia": gin.H{
			"id":                         academia.ID,
			"type":                       academia.Type,
			"nome":                       academia.Nome,
			"codigo_academia":            academia.CodigoAcademia,
			"email":                      academia.Email,
			"email_verificado":           academia.EmailVerificado,
			"provincia":                  academia.Provincia,
			"endereco":                   academia.Endereco,
			"numero_telefone":            academia.NumeroTelefone,
			"website":                    academia.Website,
			"nivel_escolar":              academia.NivelEscolar,
			"status":                     academia.Status,
			"cursos":                     academia.Cursos,
			"created_at":                 academia.CreatedAt,
			"updated_at":                 academia.UpdatedAt,
			"total_estudantes":           academia.TotalEstudantes,
			"total_inscricoes_pendentes": academia.TotalInscricoesPendentes,
			"version":                    academia.Version,
		},
	})
}

func getPerfilAdmin(c *gin.Context, userID interface{}) {
	adminProj := getAdminProjection(c)

	id, ok := userID.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar ID"})
		return
	}

	admin, err := adminProj.GetByID(id)
	if err != nil || admin == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
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

// 🔥 ATUALIZADO
func GetEstudantePorCodigo(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	codigoEstudante := c.Param("codigo")

	if userType != "academia" && userType != "admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "acesso negado: apenas academias e administradores",
		})
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
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

	// 🔥 BUSCAR INFORMAÇÕES DOS CURSOS
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
			"status_escolar":                 estudante.StatusEscolar,
			"status_superior":                estudante.StatusSuperior,
			"ano_escolar":                    estudante.AnoEscolar,
			"ano_superior":                   estudante.AnoSuperior,
			"curso_medio_id":                 estudante.CursoMedioID,    // 🔥 MUDOU
			"curso_medio_info":               cursoMedioInfo,            // 🔥 NOVO
			"curso_superior_id":              estudante.CursoSuperiorID, // 🔥 MUDOU
			"curso_superior_info":            cursoSuperiorInfo,         // 🔥 NOVO
			"created_at":                     estudante.CreatedAt,
			"updated_at":                     estudante.UpdatedAt,
			"total_notas":                    estudante.TotalNotas,
			"total_faltas":                   estudante.TotalFaltas,
			"total_inscricoes":               estudante.TotalInscricoes,
			"version":                        estudante.Version,
		},
		"consultado_por": userType,
	})
}

// GetAcademiaPorCodigo busca dados de academia por código
func GetAcademiaPorCodigo(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	codigoAcademia := c.Param("codigo")

	if userType != "academia" && userType != "admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "acesso negado: apenas academias e administradores",
		})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	var estatisticas *gin.H
	if userType == "admin" {
		client := getDbClient(c)

		var stats struct {
			TotalInscricoesTotal   int
			TotalNotasRegistradas  int
			TotalFaltasRegistradas int
		}

		safeCodAcad := db.SafeString(codigoAcademia)
		query := fmt.Sprintf(`
			SELECT 
				(SELECT COUNT(*) FROM projection_inscricoes WHERE codigo_academia = '%s') as total_inscricoes,
				(SELECT COUNT(*) FROM projection_notas WHERE codigo_academia = '%s') as total_notas,
				(SELECT COUNT(*) FROM projection_faltas WHERE codigo_academia = '%s') as total_faltas
		`, safeCodAcad, safeCodAcad, safeCodAcad)

		err := client.DB().QueryRow(query).Scan(
			&stats.TotalInscricoesTotal,
			&stats.TotalNotasRegistradas,
			&stats.TotalFaltasRegistradas,
		)
		if err == nil {
			estatisticas = &gin.H{
				"total_inscricoes_historico": stats.TotalInscricoesTotal,
				"total_notas_registradas":    stats.TotalNotasRegistradas,
				"total_faltas_registradas":   stats.TotalFaltasRegistradas,
			}
		}
	}

	response := gin.H{
		"academia": gin.H{
			"id":                         academia.ID,
			"type":                       academia.Type,
			"nome":                       academia.Nome,
			"codigo_academia":            academia.CodigoAcademia,
			"email":                      academia.Email,
			"email_verificado":           academia.EmailVerificado,
			"provincia":                  academia.Provincia,
			"endereco":                   academia.Endereco,
			"numero_telefone":            academia.NumeroTelefone,
			"website":                    academia.Website,
			"nivel_escolar":              academia.NivelEscolar,
			"status":                     academia.Status,
			"cursos":                     academia.Cursos,
			"created_at":                 academia.CreatedAt,
			"updated_at":                 academia.UpdatedAt,
			"total_estudantes":           academia.TotalEstudantes,
			"total_inscricoes_pendentes": academia.TotalInscricoesPendentes,
			"version":                    academia.Version,
		},
		"consultado_por": userType,
	}

	if estatisticas != nil {
		response["estatisticas_completas"] = estatisticas
	}

	c.JSON(http.StatusOK, response)
}

// GetAdminPorEmail busca dados de admin por email
func GetAdminPorEmail(c *gin.Context) {
	email := c.Param("email")

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(email)
	if err != nil || admin == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
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