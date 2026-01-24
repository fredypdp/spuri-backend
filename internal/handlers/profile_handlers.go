// ============================================================================
// ARQUIVO: internal/handlers/profile_handlers.go
// Handlers para consulta de dados de perfil e usuários
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
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

	log.Printf("👤 [MEU-PERFIL] Buscando perfil - UserID: %s, Tipo: %s", userID, userType)

	switch userType {
	case "estudante":
		log.Printf("🔍 [MEU-PERFIL-DEBUG] Redirecionando para perfil de estudante")
		getPerfilEstudante(c, userID)
	case "academia":
		log.Printf("🔍 [MEU-PERFIL-DEBUG] Redirecionando para perfil de academia")
		getPerfilAcademia(c, userID)
	case "admin":
		log.Printf("🔍 [MEU-PERFIL-DEBUG] Redirecionando para perfil de admin")
		getPerfilAdmin(c, userID)
	default:
		log.Printf("❌ [MEU-PERFIL-DEBUG] Tipo de usuário inválido: %s", userType)
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo de usuário inválido"})
	}
}

// getPerfilEstudante busca perfil de estudante logado
func getPerfilEstudante(c *gin.Context, userID interface{}) {
	log.Printf("🎓 [PERFIL-ESTUDANTE] Buscando perfil do estudante")
	
	estudanteProj := getEstudanteProjection(c)

	// Converter para uuid.UUID
	id, ok := userID.(uuid.UUID)
	if !ok {
		log.Printf("❌ [PERFIL-ESTUDANTE-DEBUG] Erro ao converter ID para UUID")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar ID"})
		return
	}

	log.Printf("🔍 [PERFIL-ESTUDANTE-DEBUG] Buscando estudante ID: %s", id)
	estudante, err := estudanteProj.GetByID(id)
	if err != nil || estudante == nil {
		log.Printf("❌ [PERFIL-ESTUDANTE-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	log.Printf("✅ [PERFIL-ESTUDANTE-DEBUG] Estudante encontrado: %s (código: %s)", 
		estudante.Nome, estudante.CodigoEstudante)

	// Buscar nome da academia se vinculado
	var academiaInfo *gin.H
	if estudante.CodigoAcademia != nil {
		log.Printf("🔍 [PERFIL-ESTUDANTE-DEBUG] Buscando academia vinculada: %s", *estudante.CodigoAcademia)
		academiaProj := getAcademiaProjection(c)
		academia, _ := academiaProj.GetByCodigo(*estudante.CodigoAcademia)
		if academia != nil {
			log.Printf("✅ [PERFIL-ESTUDANTE-DEBUG] Academia vinculada: %s", academia.Nome)
			academiaInfo = &gin.H{
				"codigo": academia.CodigoAcademia,
				"nome":   academia.Nome,
				"tipo":   academia.Type,
			}
		}
	}

	log.Printf("✅ [PERFIL-ESTUDANTE] Perfil retornado com sucesso")

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
	log.Printf("🏫 [PERFIL-ACADEMIA] Buscando perfil da academia")
	
	academiaProj := getAcademiaProjection(c)

	// Converter para uuid.UUID
	id, ok := userID.(uuid.UUID)
	if !ok {
		log.Printf("❌ [PERFIL-ACADEMIA-DEBUG] Erro ao converter ID para UUID")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar ID"})
		return
	}

	log.Printf("🔍 [PERFIL-ACADEMIA-DEBUG] Buscando academia ID: %s", id)
	academia, err := academiaProj.GetByID(id)
	if err != nil || academia == nil {
		log.Printf("❌ [PERFIL-ACADEMIA-DEBUG] Academia não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	log.Printf("✅ [PERFIL-ACADEMIA-DEBUG] Academia encontrada: %s (código: %s)", 
		academia.Nome, academia.CodigoAcademia)
	log.Printf("✅ [PERFIL-ACADEMIA] Perfil retornado com sucesso")

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
	log.Printf("👨‍💼 [PERFIL-ADMIN] Buscando perfil do admin")
	
	adminProj := getAdminProjection(c)

	// Converter para uuid.UUID
	id, ok := userID.(uuid.UUID)
	if !ok {
		log.Printf("❌ [PERFIL-ADMIN-DEBUG] Erro ao converter ID para UUID")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar ID"})
		return
	}

	log.Printf("🔍 [PERFIL-ADMIN-DEBUG] Buscando admin ID: %s", id)
	admin, err := adminProj.GetByID(id)
	if err != nil || admin == nil {
		log.Printf("❌ [PERFIL-ADMIN-DEBUG] Admin não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	log.Printf("✅ [PERFIL-ADMIN-DEBUG] Admin encontrado: %s (role: %s)", admin.Nome, admin.Role)
	log.Printf("✅ [PERFIL-ADMIN] Perfil retornado com sucesso")

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
	codigoEstudante := c.Param("codigo")

	log.Printf("🔍 [GET-ESTUDANTE-CODIGO] Buscando estudante - Código: %s, Consultado por: %s", 
		codigoEstudante, userType)

	// Verificar se é academia ou admin
	if userType != "academia" && userType != "admin" {
		log.Printf("❌ [GET-ESTUDANTE-CODIGO-DEBUG] Acesso negado - Tipo: %s", userType)
		c.JSON(http.StatusForbidden, gin.H{
			"error": "acesso negado: apenas academias e administradores",
		})
		return
	}

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	log.Printf("🔍 [GET-ESTUDANTE-CODIGO-DEBUG] Buscando estudante por código: %s", codigoEstudante)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		log.Printf("❌ [GET-ESTUDANTE-CODIGO-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	log.Printf("✅ [GET-ESTUDANTE-CODIGO-DEBUG] Estudante encontrado: %s", estudante.Nome)

	// Buscar informações da academia se vinculado
	var academiaInfo *gin.H
	if estudante.CodigoAcademia != nil {
		log.Printf("🔍 [GET-ESTUDANTE-CODIGO-DEBUG] Buscando academia vinculada: %s", *estudante.CodigoAcademia)
		academiaProj := getAcademiaProjection(c)
		academia, _ := academiaProj.GetByCodigo(*estudante.CodigoAcademia)
		if academia != nil {
			log.Printf("✅ [GET-ESTUDANTE-CODIGO-DEBUG] Academia vinculada: %s", academia.Nome)
			academiaInfo = &gin.H{
				"codigo":        academia.CodigoAcademia,
				"nome":          academia.Nome,
				"tipo":          academia.Type,
				"provincia":     academia.Provincia,
				"nivel_escolar": academia.NivelEscolar,
			}
		}
	}

	log.Printf("✅ [GET-ESTUDANTE-CODIGO] Dados retornados com sucesso")

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
func GetAcademiaPorCodigo(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	codigoAcademia := c.Param("codigo")

	log.Printf("🔍 [GET-ACADEMIA-CODIGO] Buscando academia - Código: %s, Consultado por: %s", 
		codigoAcademia, userType)

	// Verificar se é academia ou admin
	if userType != "academia" && userType != "admin" {
		log.Printf("❌ [GET-ACADEMIA-CODIGO-DEBUG] Acesso negado - Tipo: %s", userType)
		c.JSON(http.StatusForbidden, gin.H{
			"error": "acesso negado: apenas academias e administradores",
		})
		return
	}

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [GET-ACADEMIA-CODIGO-DEBUG] Buscando academia por código: %s", codigoAcademia)
	academia, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		log.Printf("❌ [GET-ACADEMIA-CODIGO-DEBUG] Academia não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	log.Printf("✅ [GET-ACADEMIA-CODIGO-DEBUG] Academia encontrada: %s", academia.Nome)

	// Buscar estatísticas adicionais apenas para admin
	var estatisticas *gin.H
	if userType == "admin" {
		log.Printf("🔍 [GET-ACADEMIA-CODIGO-DEBUG] Buscando estatísticas completas (admin)...")
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

		log.Printf("🔍 [GET-ACADEMIA-CODIGO-DEBUG] Query estatísticas: %s", query)

		err := client.DB().QueryRow(query).Scan(
			&stats.TotalInscricoesTotal,
			&stats.TotalNotasRegistradas,
			&stats.TotalFaltasRegistradas,
		)
		if err == nil {
			log.Printf("✅ [GET-ACADEMIA-CODIGO-DEBUG] Estatísticas coletadas - Inscrições: %d, Notas: %d, Faltas: %d",
				stats.TotalInscricoesTotal, stats.TotalNotasRegistradas, stats.TotalFaltasRegistradas)
			estatisticas = &gin.H{
				"total_inscricoes_historico": stats.TotalInscricoesTotal,
				"total_notas_registradas":    stats.TotalNotasRegistradas,
				"total_faltas_registradas":   stats.TotalFaltasRegistradas,
			}
		} else {
			log.Printf("⚠️ [GET-ACADEMIA-CODIGO-DEBUG] Erro ao buscar estatísticas: %v", err)
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

	log.Printf("✅ [GET-ACADEMIA-CODIGO] Dados retornados com sucesso")

	c.JSON(http.StatusOK, response)
}

// ============================================================================
// ROTAS EXCLUSIVAS ADMIN
// ============================================================================

// GetAdminPorEmail busca dados de admin por email (apenas admin)
func GetAdminPorEmail(c *gin.Context) {
	email := c.Param("email")

	log.Printf("🔍 [GET-ADMIN-EMAIL] Buscando admin por email: %s", email)

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(email)
	if err != nil || admin == nil {
		log.Printf("❌ [GET-ADMIN-EMAIL-DEBUG] Admin não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "administrador não encontrado"})
		return
	}

	log.Printf("✅ [GET-ADMIN-EMAIL] Admin encontrado: %s (role: %s)", admin.Nome, admin.Role)

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