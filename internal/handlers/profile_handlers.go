// ============================================================================
// ARQUIVO: internal/handlers/profile_handlers.go
// NOVO: Handlers para consulta de dados de perfil e usuários
// ============================================================================

package handlers

import (
	"net/http"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// ROTAS DE PERFIL - USUÁRIO LOGADO
// ============================================================================

// GetMeuPerfil retorna dados do usuário logado (estudante, academia ou admin)
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

// getPerfilEstudante busca perfil de estudante logado
func getPerfilEstudante(c *gin.Context, userID interface{}) {
	estudanteProj := getEstudanteProjection(c)

	// Converter para uuid.UUID
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

	// Buscar nome da academia se vinculado
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

	c.JSON(http.StatusOK, gin.H{
		"tipo": "estudante",
		"estudante": gin.H{
			"id":                             estudante.ID,
			"nome":                           estudante.Nome,
			"codigo_estudante":               estudante.CodigoEstudante,
			"bilhete_identidade":             estudante.BilheteIdentidade,
			"bilhete_identidade_responsavel": estudante.BilheteIdentidadeResp,
			"codigo_academia":                estudante.CodigoAcademia,
			"academia_info":                  academiaInfo,
			"ano_escolar":                    estudante.AnoEscolar,
			"ano_superior":                   estudante.AnoSuperior,
			"curso_medio":                    estudante.CursoMedio,
			"curso_superior":                 estudante.CursoSuperior,
			"status_escolar":                 estudante.StatusEscolar,
			"status_superior":                estudante.StatusSuperior,
			"created_at":                     estudante.CreatedAt,
			"total_notas":                    estudante.TotalNotas,
			"total_faltas":                   estudante.TotalFaltas,
			"total_inscricoes":               estudante.TotalInscricoes,
		},
	})
}

// getPerfilAcademia busca perfil de academia logada
func getPerfilAcademia(c *gin.Context, userID interface{}) {
	academiaProj := getAcademiaProjection(c)

	// Converter para uuid.UUID
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
			"provincia":                  academia.Provincia,
			"endereco":                   academia.Endereco,
			"numero_telefone":            academia.NumeroTelefone,
			"email":                      academia.Email,
			"website":                    academia.Website,
			"nivel_escolar":              academia.NivelEscolar,
			"status":                     academia.Status,
			"cursos":                     academia.Cursos,
			"created_at":                 academia.CreatedAt,
			"total_estudantes":           academia.TotalEstudantes,
			"total_inscricoes_pendentes": academia.TotalInscricoesPendentes,
		},
	})
}

// getPerfilAdmin busca perfil de admin logado
func getPerfilAdmin(c *gin.Context, userID interface{}) {
	adminProj := getAdminProjection(c)

	// Converter para uuid.UUID
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
			"role":                   admin.Role,
			"status":                 admin.Status,
			"created_at":             admin.CreatedAt,
			"total_acoes_realizadas": admin.TotalAcoesRealizadas,
		},
	})
}

// ============================================================================
// ROTAS DE CONSULTA PÚBLICA (ACADEMIA E ADMIN)
// ============================================================================

// GetEstudantePorCodigo busca dados de estudante por código
// Permitido para: Academia (qualquer) e Admin
func GetEstudantePorCodigo(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)

	// Verificar se é academia ou admin
	if userType != "academia" && userType != "admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "acesso negado: apenas academias e administradores",
		})
		return
	}

	codigoEstudante := c.Param("codigo")

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Buscar informações da academia se vinculado
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

	c.JSON(http.StatusOK, gin.H{
		"estudante": gin.H{
			"id":                 estudante.ID,
			"nome":               estudante.Nome,
			"codigo_estudante":   estudante.CodigoEstudante,
			"bilhete_identidade": estudante.BilheteIdentidade,
			"codigo_academia":    estudante.CodigoAcademia,
			"academia_info":      academiaInfo,
			"ano_escolar":        estudante.AnoEscolar,
			"ano_superior":       estudante.AnoSuperior,
			"curso_medio":        estudante.CursoMedio,
			"curso_superior":     estudante.CursoSuperior,
			"status_escolar":     estudante.StatusEscolar,
			"status_superior":    estudante.StatusSuperior,
			"created_at":         estudante.CreatedAt,
			"total_notas":        estudante.TotalNotas,
			"total_faltas":       estudante.TotalFaltas,
			"total_inscricoes":   estudante.TotalInscricoes,
		},
		"consultado_por": userType,
	})
}

// GetAcademiaPorCodigo busca dados de academia por código
// Permitido para: Academia (qualquer) e Admin
func GetAcademiaPorCodigo(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)

	// Verificar se é academia ou admin
	if userType != "academia" && userType != "admin" {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "acesso negado: apenas academias e administradores",
		})
		return
	}

	codigoAcademia := c.Param("codigo")

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	// Buscar estatísticas adicionais apenas para admin
	var estatisticas *gin.H
	if userType == "admin" {
		client := getDbClient(c)

		var stats struct {
			TotalInscricoesTotal   int `db:"total_inscricoes"`
			TotalNotasRegistradas  int `db:"total_notas"`
			TotalFaltasRegistradas int `db:"total_faltas"`
		}

		query := `
			SELECT 
				(SELECT COUNT(*) FROM projection_inscricoes WHERE codigo_academia = $1) as total_inscricoes,
				(SELECT COUNT(*) FROM projection_notas WHERE codigo_academia = $1) as total_notas,
				(SELECT COUNT(*) FROM projection_faltas WHERE codigo_academia = $1) as total_faltas
		`

		if err := client.DB().Get(&stats, query, codigoAcademia); err == nil {
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
			"provincia":                  academia.Provincia,
			"endereco":                   academia.Endereco,
			"numero_telefone":            academia.NumeroTelefone,
			"email":                      academia.Email,
			"website":                    academia.Website,
			"nivel_escolar":              academia.NivelEscolar,
			"status":                     academia.Status,
			"cursos":                     academia.Cursos,
			"created_at":                 academia.CreatedAt,
			"total_estudantes":           academia.TotalEstudantes,
			"total_inscricoes_pendentes": academia.TotalInscricoesPendentes,
		},
		"consultado_por": userType,
	}

	if estatisticas != nil {
		response["estatisticas_completas"] = estatisticas
	}

	c.JSON(http.StatusOK, response)
}

// ============================================================================
// ROTAS EXCLUSIVAS ADMIN
// ============================================================================

// GetAdminPorEmail busca dados de admin por email (apenas admin)
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
			"role":                   admin.Role,
			"status":                 admin.Status,
			"created_by":             admin.CreatedBy,
			"created_at":             admin.CreatedAt,
			"total_acoes_realizadas": admin.TotalAcoesRealizadas,
		},
	})
}

// BuscarUsuario busca qualquer tipo de usuário (estudante, academia ou admin)
// Endpoint unificado apenas para admin
func BuscarUsuario(c *gin.Context) {
	tipo := c.Query("tipo")        // "estudante", "academia", "admin"
	identificador := c.Query("id") // código ou email

	if tipo == "" || identificador == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "parâmetros 'tipo' e 'id' são obrigatórios",
			"exemplo": "/admin/buscar-usuario?tipo=estudante&id=KAF7392",
		})
		return
	}

	switch tipo {
	case "estudante":
		buscarEstudanteAdmin(c, identificador)
	case "academia":
		buscarAcademiaAdmin(c, identificador)
	case "admin":
		buscarAdminPorEmail(c, identificador)
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error":         "tipo inválido",
			"tipos_validos": []string{"estudante", "academia", "admin"},
		})
	}
}

func buscarEstudanteAdmin(c *gin.Context, codigo string) {
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigo)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Buscar info academia
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

	c.JSON(http.StatusOK, gin.H{
		"tipo": "estudante",
		"dados": gin.H{
			"id":                 estudante.ID,
			"nome":               estudante.Nome,
			"codigo_estudante":   estudante.CodigoEstudante,
			"bilhete_identidade": estudante.BilheteIdentidade,
			"codigo_academia":    estudante.CodigoAcademia,
			"academia_info":      academiaInfo,
			"ano_escolar":        estudante.AnoEscolar,
			"ano_superior":       estudante.AnoSuperior,
			"curso_medio":        estudante.CursoMedio,
			"curso_superior":     estudante.CursoSuperior,
			"status_escolar":     estudante.StatusEscolar,
			"status_superior":    estudante.StatusSuperior,
			"created_at":         estudante.CreatedAt,
			"total_notas":        estudante.TotalNotas,
			"total_faltas":       estudante.TotalFaltas,
			"total_inscricoes":   estudante.TotalInscricoes,
		},
	})
}

func buscarAcademiaAdmin(c *gin.Context, codigo string) {
	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByCodigo(codigo)
	if err != nil || academia == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tipo": "academia",
		"dados": gin.H{
			"id":                         academia.ID,
			"type":                       academia.Type,
			"nome":                       academia.Nome,
			"codigo_academia":            academia.CodigoAcademia,
			"provincia":                  academia.Provincia,
			"endereco":                   academia.Endereco,
			"numero_telefone":            academia.NumeroTelefone,
			"email":                      academia.Email,
			"website":                    academia.Website,
			"nivel_escolar":              academia.NivelEscolar,
			"status":                     academia.Status,
			"cursos":                     academia.Cursos,
			"created_at":                 academia.CreatedAt,
			"total_estudantes":           academia.TotalEstudantes,
			"total_inscricoes_pendentes": academia.TotalInscricoesPendentes,
		},
	})
}

func buscarAdminPorEmail(c *gin.Context, email string) {
	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(email)
	if err != nil || admin == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tipo": "admin",
		"dados": gin.H{
			"id":                     admin.ID,
			"nome":                   admin.Nome,
			"email":                  admin.Email,
			"role":                   admin.Role,
			"status":                 admin.Status,
			"created_by":             admin.CreatedBy,
			"created_at":             admin.CreatedAt,
			"total_acoes_realizadas": admin.TotalAcoesRealizadas,
		},
	})
}
