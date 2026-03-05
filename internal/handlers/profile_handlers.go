// ============================================================================
// ARQUIVO: internal/handlers/profile_handlers.go
//
// CORREÇÕES APLICADAS:
//   FIX-C1  — AlterarSenha: academia agora usa event sourcing via aggregate,
//              idêntico ao admin. Antes: UPDATE direto em projection_academias —
//              bypassava o ledger.
//   H4-18   — AlterarSenha: estudante inativo bloqueado antes de processar.
//              Um estudante com token JWT válido mas status != "ativo" não pode
//              alterar senha. O bloqueio de login verifica status, mas a alteração
//              de senha não verificava — gap corrigido aqui.
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

// AlterarSenha permite que o usuário autenticado altere sua própria senha.
//
// FIX-C1: academia agora usa event sourcing via aggregate, idêntico ao admin.
// H4-18:  estudante inativo bloqueado — token válido não é suficiente para
//         alterar senha se o estudante estiver com status != "ativo".
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

	// ── Academia: event sourcing (FIX-C1) ─────────────────────────────────
	if userType == "academia" {
		uid := userID.(uuid.UUID)

		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(uid)
		if err != nil || academiaDTO == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(academiaDTO.SenhaHash), []byte(req.SenhaAtual)); err != nil {
			utils.RespondWithUnauthorizedError(c)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		repository := getRepository(c)
		academiaAgg, err := repository.Load(uid, "Academia")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		academia := academiaAgg.(*aggregates.Academia)

		if err := academia.AlterarSenha(string(hashedPassword), uid, "alteracao_usuario"); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		audit := db.AuditContext{
			UserID:   uid.String(),
			UserType: "academia",
			IP:       c.ClientIP(),
		}
		if err := repository.SaveWithAudit(academia, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		log.Printf("Senha alterada (event sourcing) para academia: %v", uid)
		c.JSON(http.StatusOK, gin.H{"message": "Senha alterada com sucesso!"})
		return
	}

	// ── Estudante: event sourcing via SenhaAlterada ────────────────────────
	if userType == "estudante" {
		uid := userID.(uuid.UUID)

		estudanteProj := getEstudanteProjection(c)
		// FIX-COMPILE-01: GetAuthByID retorna EstudanteAuthDTO com Hash.
		// GetByID retorna EstudanteDTO que não expõe SenhaHash (fix H4-05).
		estudanteAuth, err := estudanteProj.GetAuthByID(uid)
		if err != nil || estudanteAuth == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}

		// H4-18: estudante inativo não pode alterar senha mesmo com token JWT válido.
		// O AuthMiddleware não verifica status após emitir o token — esta é a barreira
		// no nível do handler para operações de escrita sensíveis.
		if estudanteAuth.Status != "ativo" {
			utils.RespondWithForbiddenError(c, "conta inativa. Não é possível alterar a senha.")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(estudanteAuth.Hash), []byte(req.SenhaAtual)); err != nil {
			utils.RespondWithUnauthorizedError(c)
			return
		}

		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		repository := getRepository(c)
		estudanteAgg, err := repository.Load(uid, "Estudante")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		estudante := estudanteAgg.(*aggregates.Estudante)

		if err := estudante.AlterarSenha(string(hashedPassword)); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		audit := db.AuditContext{
			UserID:   uid.String(),
			UserType: "estudante",
			IP:       c.ClientIP(),
		}
		if err := repository.SaveWithAudit(estudante, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		log.Printf("Senha alterada (event sourcing) para estudante: %v", uid)
		c.JSON(http.StatusOK, gin.H{"message": "Senha alterada com sucesso!"})
		return
	}

	utils.RespondWithValidationError(c, fmt.Errorf("tipo de usuário inválido"))
}

// ============================================================================
// GET /meu-perfil
// ============================================================================

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
		cursoMedioUUID, err := uuid.Parse(*estudante.CursoMedioID)
		if err == nil {
			cursoMedio, _ := cursosProj.GetByID(cursoMedioUUID)
			if cursoMedio != nil {
				cursoMedioInfo = &gin.H{
					"id":     cursoMedio.ID,
					"nome":   cursoMedio.Nome,
					"type":   cursoMedio.Type,
					"status": cursoMedio.Status,
				}
			}
		}
	}

	if estudante.CursoSuperiorID != nil {
		cursoSuperiorUUID, err := uuid.Parse(*estudante.CursoSuperiorID)
		if err == nil {
			cursoSuperior, _ := cursosProj.GetByID(cursoSuperiorUUID)
			if cursoSuperior != nil {
				cursoSuperiorInfo = &gin.H{
					"id":     cursoSuperior.ID,
					"nome":   cursoSuperior.Nome,
					"type":   cursoSuperior.Type,
					"status": cursoSuperior.Status,
				}
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
			"curso_medio":                    cursoMedioInfo,
			"curso_superior":                 cursoSuperiorInfo,
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
			"id":               academia.ID,
			"type":             academia.Type,
			"nome":             academia.Nome,
			"codigo_academia":  academia.CodigoAcademia,
			"provincia":        academia.Provincia,
			"endereco":         academia.Endereco,
			"numero_telefone":  academia.NumeroTelefone,
			"email":            academia.Email,
			"website":          academia.Website,
			"nivel_escolar":    academia.NivelEscolar,
			"anos_academicos":  academia.AnosAcademicos,
			"status":           academia.Status,
			"cursos":           academia.Cursos,
			"email_verificado": academia.EmailVerificado,
			"created_at":       academia.CreatedAt,
			"total_estudantes": academia.TotalEstudantes,
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
			"id":               admin.ID,
			"nome":             admin.Nome,
			"email":            admin.Email,
			"role":             admin.Role,
			"status":           admin.Status,
			"email_verificado": admin.EmailVerificado,
			"created_at":       admin.CreatedAt,
		},
	})
}

// ============================================================================
// GET /buscar-usuario (admin only)
// ============================================================================

// BuscarUsuario localiza qualquer entidade (estudante, academia ou admin) por UUID.
func BuscarUsuario(c *gin.Context) {
	idStr := c.Query("id")
	if idStr == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("parâmetro 'id' é obrigatório"))
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("UUID inválido"))
		return
	}

	// Tentar em cada projeção
	estudanteProj := getEstudanteProjection(c)
	if est, _ := estudanteProj.GetByID(id); est != nil {
		c.JSON(http.StatusOK, gin.H{"tipo": "estudante", "usuario": est})
		return
	}

	academiaProj := getAcademiaProjection(c)
	if aca, _ := academiaProj.GetByID(id); aca != nil {
		c.JSON(http.StatusOK, gin.H{"tipo": "academia", "usuario": aca})
		return
	}

	adminProj := getAdminProjection(c)
	if adm, _ := adminProj.GetByID(id); adm != nil {
		c.JSON(http.StatusOK, gin.H{"tipo": "admin", "usuario": adm})
		return
	}

	utils.RespondWithNotFoundError(c, "usuário")
}