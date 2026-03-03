// ============================================================================
// ARQUIVO: internal/handlers/profile_handlers.go
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)


func AlterarSenha(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userType, _ := c.Get("user_type")

	var req struct {
		SenhaAtual string `json:"senha_atual" binding:"required"`
		NovaSenha  string `json:"nova_senha" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("senha_atual e nova_senha são obrigatórios"))
		return
	}

	if err := utils.ValidateSenha(req.NovaSenha); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Admin: event sourcing ──────────────────────────────────────────────
	if userType == "admin" {
		uid := userID.(uuid.UUID)

		adminProj := getAdminProjection(c)
		adminDTO, err := adminProj.GetByID(uid)
		if err != nil || adminDTO == nil {
			utils.RespondWithNotFoundError(c, "administrador")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(adminDTO.SenhaHash), []byte(req.SenhaAtual)); err != nil {
			utils.RespondWithUnauthorizedError(c)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		// CORRIGIDO #2: carregar aggregate e emitir evento AdminSenhaAlterada
		repository := getRepository(c)
		adminAgg, err := repository.Load(uid, "Admin")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		admin := adminAgg.(*aggregates.Admin)

		if err := admin.AlterarSenha(string(hashedPassword), uid, "alteracao_usuario"); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		audit := db.AuditContext{
			UserID:   uid.String(),
			UserType: "admin",
			IP:       c.ClientIP(),
		}
		if err := repository.SaveWithAudit(admin, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		log.Printf("Senha alterada (event sourcing) para admin: %v", uid)
		c.JSON(http.StatusOK, gin.H{"message": "Senha alterada com sucesso!"})
		return
	}

	// ── Estudante e Academia: UPDATE direto (sem evento próprio de senha ainda) ──
	client := getDbClient(c)

	var table string
	switch userType {
	case "estudante":
		table = "projection_estudantes"
	case "academia":
		table = "projection_academias"
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
		return
	}

	var senhaHash string
	err := client.DB().QueryRow(
		fmt.Sprintf("SELECT senha_hash FROM %s WHERE id = $1", table),
		userID,
	).Scan(&senhaHash)
	if err != nil {
		utils.RespondWithNotFoundError(c, "usuário")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.SenhaAtual)); err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	_, err = client.DB().Exec(
		fmt.Sprintf("UPDATE %s SET senha_hash = $1 WHERE id = $2", table),
		string(hashedPassword),
		userID,
	)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Senha alterada para %s: %v", userType, userID)
	c.JSON(http.StatusOK, gin.H{"message": "Senha alterada com sucesso!"})
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