// ============================================================================
// ARQUIVO: internal/handlers/estudante_handlers.go
//
// CORREÇÕES APLICADAS:
//   FIX-C4  — AtualizarStatusEscolar*Handler REMOVIDOS deste arquivo.
//              Esses handlers agora estão em academia_handlers.go e exigem
//              RequireAcademia(). Estudante não pode mais alterar próprio status
//              escolar — é responsabilidade exclusiva da academia.
//   FIX-S1  — Senha padrão de estudante criado por academia = código do estudante
//              (via services.GetDefaultPassword). Antes era "spuri123" hardcoded.
//   FIX-S2  — Audit context adicionado no RegisterEstudantePorAcademia.
// ============================================================================

package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/services"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================================
// POST /estudante/register  (público)
// ============================================================================

type CadastroEstudanteRequest struct {
	Nome                     string     `json:"nome" binding:"required"`
	Email                    *string    `json:"email"`
	Telefone                 *string    `json:"telefone"`
	BilheteIdentidade        *string    `json:"bilhete_identidade"`
	BilheteIdentidadeResp    *string    `json:"bilhete_identidade_responsavel"`
	AnoEscolar               *string    `json:"ano_escolar"`
	AnoEscolarMedio          *string    `json:"ano_escolar_medio"`
	AnoSuperior              *string    `json:"ano_superior"`
	CursoMedioID             *uuid.UUID `json:"curso_medio_id"`
	CursoSuperiorID          *uuid.UUID `json:"curso_superior_id"`
	StatusEscolarFundamental *string    `json:"status_escolar_fundamental"`
	StatusEscolarMedio       *string    `json:"status_escolar_medio"`
	StatusSuperior           *string    `json:"status_superior"`
	Senha                    string     `json:"senha" binding:"required"`
	Genero                   string     `json:"genero" binding:"required"`
}

func RegisterEstudante(c *gin.Context) {
	var req CadastroEstudanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if err := utils.ValidateSenha(req.Senha); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.BilheteIdentidade == nil && req.BilheteIdentidadeResp == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("pelo menos um bilhete de identidade é obrigatório"))
		return
	}

	if req.BilheteIdentidade != nil && *req.BilheteIdentidade != "" {
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(*req.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado"))
			return
		}
	}

	if req.CursoMedioID != nil && *req.CursoMedioID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id não encontrado"))
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
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id não encontrado"))
			return
		}
		if curso.Type != "superior" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id deve ser do tipo 'superior'"))
			return
		}
	}

	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	if err := estudante.Criar(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		req.Email,
		req.Telefone,
		req.BilheteIdentidade,
		req.BilheteIdentidadeResp,
		req.AnoEscolar,
		req.AnoEscolarMedio,
		req.AnoSuperior,
		req.CursoMedioID,
		req.CursoSuperiorID,
		req.StatusEscolarFundamental,
		req.StatusEscolarMedio,
		req.StatusSuperior,
		req.Genero,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   "anonimo",
		UserType: "sistema",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Estudante criado (auto-cadastro): %s - %s", codigoEstudante, req.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante criado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
		},
	})
}

// ============================================================================
// POST /academia/estudante/register  (academia)
// ============================================================================

type CadastroEstudanteAcademiaRequest struct {
	Nome                     string `json:"nome" binding:"required"`
	Email                    string `json:"email"`
	Telefone                 string `json:"telefone"`
	BilheteIdentidade        string `json:"bilhete_identidade"`
	BilheteResponsavel       string `json:"bilhete_identidade_responsavel"`
	AnoEscolar               string `json:"ano_escolar"`
	AnoEscolarMedio          string `json:"ano_escolar_medio"`
	AnoSuperior              string `json:"ano_superior"`
	CursoMedioID             string `json:"curso_medio_id"`
	CursoSuperiorID          string `json:"curso_superior_id"`
	StatusEscolarFundamental string `json:"status_escolar_fundamental"`
	StatusEscolarMedio       string `json:"status_escolar_medio"`
	StatusSuperior           string `json:"status_superior"`
	Genero                   string `json:"genero" binding:"required"`
}

func RegisterEstudantePorAcademia(c *gin.Context) {
	var req CadastroEstudanteAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("nome e genero são obrigatórios"))
		return
	}

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != "" {
		if err := utils.ValidateEmail(req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Genero != "masculino" && req.Genero != "feminino" {
		utils.RespondWithValidationError(c, fmt.Errorf("genero deve ser 'masculino' ou 'feminino'"))
		return
	}

	// Resolver curso médio
	var cursoMedioUUID *uuid.UUID
	if req.CursoMedioID != "" {
		parsed, err := uuid.Parse(req.CursoMedioID)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id inválido"))
			return
		}
		cursoMedioUUID = &parsed
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id não encontrado"))
			return
		}
		if curso.Type != "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id deve ser do tipo 'medio'"))
			return
		}
	}

	// Resolver curso superior
	var cursoSuperiorUUID *uuid.UUID
	if req.CursoSuperiorID != "" {
		parsed, err := uuid.Parse(req.CursoSuperiorID)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id inválido"))
			return
		}
		cursoSuperiorUUID = &parsed
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id não encontrado"))
			return
		}
		if curso.Type != "superior" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id deve ser do tipo 'superior'"))
			return
		}
	}

	academiaID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByID(academiaID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academia == nil {
		utils.RespondWithError(c, http.StatusNotFound, "academia não encontrada", nil)
		return
	}

	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// FIX-S1: senha padrão = código do estudante (não mais "spuri123" hardcoded).
	// GetDefaultPassword("estudante", codigoEstudante) retorna o próprio código.
	defaultPassword := services.GetDefaultPassword("estudante", codigoEstudante)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Converter campos opcionais de string para *string
	var emailPtr, telefonePtr, bilhetePtr, bilheteRespPtr *string
	if req.Email != "" {
		emailPtr = &req.Email
	}
	if req.Telefone != "" {
		telefonePtr = &req.Telefone
	}
	if req.BilheteIdentidade != "" {
		bilhetePtr = &req.BilheteIdentidade
	}
	if req.BilheteResponsavel != "" {
		bilheteRespPtr = &req.BilheteResponsavel
	}

	var anoEscolarPtr, anoEscolarMedioPtr, anoSuperiorPtr *string
	if req.AnoEscolar != "" {
		anoEscolarPtr = &req.AnoEscolar
	}
	if req.AnoEscolarMedio != "" {
		anoEscolarMedioPtr = &req.AnoEscolarMedio
	}
	if req.AnoSuperior != "" {
		anoSuperiorPtr = &req.AnoSuperior
	}

	var statusFundamentalPtr, statusMedioPtr, statusSuperiorPtr *string
	if req.StatusEscolarFundamental != "" {
		statusFundamentalPtr = &req.StatusEscolarFundamental
	} else {
		defaultStatus := "em_andamento"
		statusFundamentalPtr = &defaultStatus
	}
	if req.StatusEscolarMedio != "" {
		statusMedioPtr = &req.StatusEscolarMedio
	}
	if req.StatusSuperior != "" {
		statusSuperiorPtr = &req.StatusSuperior
	}

	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	if err := estudante.CriarComVinculo(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		emailPtr,
		telefonePtr,
		bilhetePtr,
		bilheteRespPtr,
		anoEscolarPtr,
		anoEscolarMedioPtr,
		anoSuperiorPtr,
		cursoMedioUUID,
		cursoSuperiorUUID,
		statusFundamentalPtr,
		statusMedioPtr,
		statusSuperiorPtr,
		academia.CodigoAcademia,
		req.Genero,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// FIX-S2: audit context com ID e tipo da academia.
	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Estudante criado por academia %s: %s - %s", academia.CodigoAcademia, codigoEstudante, req.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante registrado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
			"codigo_academia":  academia.CodigoAcademia,
		},
	})
}

// ============================================================================
// PUT /estudante/dados-pessoais
// ============================================================================

type AtualizarDadosPessoaisRequest struct {
	Nome                  *string `json:"nome"`
	Email                 *string `json:"email"`
	Telefone              *string `json:"telefone"`
	BilheteIdentidade     *string `json:"bilhete_identidade"`
	BilheteIdentidadeResp *string `json:"bilhete_identidade_responsavel"`
}

func AtualizarDadosPessoais(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req AtualizarDadosPessoaisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.BilheteIdentidade != nil && *req.BilheteIdentidade != "" {
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(*req.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		// Verifica se o bilhete pertence a OUTRO estudante
		if existente != nil && existente.ID != userID {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado"))
			return
		}
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	if err := estudante.AtualizarDadosPessoais(
		req.Nome, req.Email, req.Telefone,
		req.BilheteIdentidade, req.BilheteIdentidadeResp,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "estudante",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Dados pessoais atualizados: %s", estudante.CodigoEstudante)
	c.JSON(http.StatusOK, gin.H{"message": "dados pessoais atualizados com sucesso"})
}

// ============================================================================
// GET /estudante/minhas-avaliacoes
// ============================================================================

func GetMinhasAvaliacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	avaliacaoProj := getAvaliacaoFinalProjection(c)
	avaliacoes, err := avaliacaoProj.GetByEstudante(estudante.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"avaliacoes": avaliacoes,
		"total":      len(avaliacoes),
	})
}

// ============================================================================
// GET /consultar-estudante/:codigo
// ============================================================================

func GetEstudantePorCodigo(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	userType, _ := middleware.GetUserType(c)
	if userType != "academia" && userType != "admin" {
		utils.RespondWithForbiddenError(c, "Apenas academias e administradores podem consultar estudantes.")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	// Academia só pode consultar seus próprios estudantes
	if userType == "academia" {
		userID, _ := middleware.GetUserID(c)
		academiaProj := getAcademiaProjection(c)
		academia, _ := academiaProj.GetByID(userID)
		if academia == nil || estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academia.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia.")
			return
		}
	}

	var academiaInfo *gin.H
	if estudante.CodigoAcademia != nil {
		academiaProj := getAcademiaProjection(c)
		acad, _ := academiaProj.GetByCodigo(*estudante.CodigoAcademia)
		if acad != nil {
			academiaInfo = &gin.H{
				"codigo":        acad.CodigoAcademia,
				"nome":          acad.Nome,
				"tipo":          acad.Type,
				"provincia":     acad.Provincia,
				"nivel_escolar": acad.NivelEscolar,
			}
		}
	}

	var cursoMedioInfo, cursoSuperiorInfo *gin.H
	cursosProj := getCursosProjection(c)

	if estudante.CursoMedioID != nil {
		curso, _ := cursosProj.GetByID(*estudante.CursoMedioID)
		if curso != nil {
			cursoMedioInfo = &gin.H{
				"id":     curso.ID,
				"nome":   curso.Nome,
				"type":   curso.Type,
				"status": curso.Status,
			}
		}
	}

	if estudante.CursoSuperiorID != nil {
		curso, _ := cursosProj.GetByID(*estudante.CursoSuperiorID)
		if curso != nil {
			cursoSuperiorInfo = &gin.H{
				"id":     curso.ID,
				"nome":   curso.Nome,
				"type":   curso.Type,
				"status": curso.Status,
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante": gin.H{
			"nome":                           estudante.Nome,
			"codigo_estudante":               estudante.CodigoEstudante,
			"email":                          estudante.Email,
			"telefone":                       estudante.Telefone,
			"email_verificado":               estudante.EmailVerificado,
			"bilhete_identidade":             estudante.BilheteIdentidade,
			"bilhete_identidade_responsavel": estudante.BilheteIdentidadeResp,
			"codigo_academia":                estudante.CodigoAcademia,
			"status":                         estudante.Status,
			"status_escolar_fundamental":     estudante.StatusEscolarFundamental,
			"status_escolar_medio":           estudante.StatusEscolarMedio,
			"status_superior":                estudante.StatusSuperior,
			"ano_escolar":                    estudante.AnoEscolar,
			"ano_escolar_medio":              estudante.AnoEscolarMedio,
			"ano_superior":                   estudante.AnoSuperior,
			"genero":                         estudante.Genero,
			"academia":                       academiaInfo,
			"curso_medio":                    cursoMedioInfo,
			"curso_superior":                 cursoSuperiorInfo,
			"created_at":                     estudante.CreatedAt,
			"updated_at":                     estudante.UpdatedAt,
			"version":                        estudante.Version,
		},
	})
}

// ============================================================================
// GET /estudantes
// ============================================================================

func ListarEstudantes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	client := getDbClient(c)

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithForbiddenError(c, "academia não encontrada")
			return
		}

		rows, err := client.DB().Query(`
			SELECT id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
				bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
				status, status_escolar_fundamental, status_escolar_medio, status_superior,
				ano_escolar, ano_escolar_medio, ano_superior,
				curso_medio_id, curso_superior_id, created_at, updated_at,
				COALESCE(total_notas, 0), COALESCE(total_faltas, 0), version
			FROM projection_estudantes
			WHERE codigo_academia = $1
			ORDER BY created_at DESC
		`, academiaDTO.CodigoAcademia)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		estudantes := scanEstudantesRows(rows)
		c.JSON(http.StatusOK, gin.H{
			"estudantes":      estudantes,
			"total":           len(estudantes),
			"tipo_usuario":    "academia",
			"codigo_academia": academiaDTO.CodigoAcademia,
			"nome_academia":   academiaDTO.Nome,
		})

	} else if userType == "admin" {
		rows, err := client.DB().Query(`
			SELECT id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
				bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
				status, status_escolar_fundamental, status_escolar_medio, status_superior,
				ano_escolar, ano_escolar_medio, ano_superior,
				curso_medio_id, curso_superior_id, created_at, updated_at,
				COALESCE(total_notas, 0), COALESCE(total_faltas, 0), version
			FROM projection_estudantes
			ORDER BY created_at DESC
		`)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		estudantes := scanEstudantesRows(rows)
		c.JSON(http.StatusOK, gin.H{
			"estudantes":   estudantes,
			"total":        len(estudantes),
			"tipo_usuario": "admin",
		})

	} else {
		utils.RespondWithForbiddenError(c, "Acesso negado. Apenas academias e administradores podem listar estudantes.")
	}
}

func scanEstudantesRows(rows *sql.Rows) []map[string]interface{} {
	var estudantes []map[string]interface{}
	for rows.Next() {
		var id, cursoMedioID, cursoSuperiorID sql.NullString
		var nome, codigoEstudante, senhaHash, status string
		var statusFund, statusMedio, statusSuperior string
		var email, telefone, bilhete, bilheteResp, codigoAcad sql.NullString
		var anoEscolar, anoEscolarMedio, anoSuperior sql.NullString
		var emailVerif bool
		var createdAt, updatedAt string
		var totalNotas, totalFaltas, version int

		if err := rows.Scan(
			&id, &nome, &codigoEstudante, &senhaHash,
			&email, &telefone, &emailVerif, &bilhete, &bilheteResp, &codigoAcad,
			&status, &statusFund, &statusMedio, &statusSuperior,
			&anoEscolar, &anoEscolarMedio, &anoSuperior,
			&cursoMedioID, &cursoSuperiorID,
			&createdAt, &updatedAt, &totalNotas, &totalFaltas, &version,
		); err != nil {
			log.Printf("[ERROR] ListarEstudantes scan: %v", err)
			continue
		}

		estudantes = append(estudantes, map[string]interface{}{
			"nome":                           nome,
			"codigo_estudante":               codigoEstudante,
			"email":                          getNullString(email),
			"telefone":                       getNullString(telefone),
			"email_verificado":               emailVerif,
			"bilhete_identidade":             getNullString(bilhete),
			"bilhete_identidade_responsavel": getNullString(bilheteResp),
			"codigo_academia":                getNullString(codigoAcad),
			"status":                         status,
			"status_escolar_fundamental":     statusFund,
			"status_escolar_medio":           statusMedio,
			"status_superior":                statusSuperior,
			"ano_escolar":                    getNullString(anoEscolar),
			"ano_escolar_medio":              getNullString(anoEscolarMedio),
			"ano_superior":                   getNullString(anoSuperior),
			"curso_medio_id":                 getNullString(cursoMedioID),
			"curso_superior_id":              getNullString(cursoSuperiorID),
			"created_at":                     createdAt,
			"updated_at":                     updatedAt,
			"total_notas":                    totalNotas,
			"total_faltas":                   totalFaltas,
			"version":                        version,
		})
	}
	return estudantes
}

func getNullString(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

// ============================================================================
// GET /verificar-integridade/:codigo
// ============================================================================

func VerificarIntegridade(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	repository := getRepository(c)
	valid, err := repository.VerifyIntegrity(estudante.ID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"integro":          valid,
		"message": func() string {
			if valid {
				return "Ledger íntegro — cadeia de hashes válida"
			}
			return "ALERTA: ledger corrompido — cadeia de hashes inválida"
		}(),
	})
}

// ============================================================================
// GET /eventos-estudante/:codigo
// ============================================================================

func GetEventosEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	userType, _ := middleware.GetUserType(c)
	if userType != "admin" {
		utils.RespondWithForbiddenError(c, "Apenas administradores podem consultar eventos.")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	repository := getRepository(c)
	eventos, err := repository.GetEventHistory(estudante.ID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"eventos":          eventos,
		"total":            len(eventos),
	})
}