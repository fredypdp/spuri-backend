package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/middleware"
	"spuri/internal/utils"
)

func getPaginationParams(c *gin.Context) (limit, offset int) {
	limitStr := c.Query("limit")
	offsetStr := c.Query("offset")

	limit = 50
	offset = 0

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 200 {
			limit = l
		}
	}

	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	return limit, offset
}

func GetMeuHistorico(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	notasProj := getNotasProjection(c)
	notas, _ := notasProj.GetByEstudante(estudante.CodigoEstudante)

	faltasProj := getFaltasProjection(c)
	faltas, _ := faltasProj.GetByEstudante(estudante.CodigoEstudante)

	c.JSON(http.StatusOK, gin.H{
		"estudante": estudante,
		"notas":     notas,
		"faltas":    faltas,
	})
}

func ListarTodasAcademias(c *gin.Context) {
	_, _ = middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	_ = getAcademiaProjection(c)
	client := getDbClient(c)

	query := `
		SELECT id, type, nome, codigo_academia, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, status, cursos,
			email_verificado, created_at, updated_at, total_estudantes, version
		FROM projection_academias
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := client.DB().Query(query)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()

	var academias []map[string]interface{}
	for rows.Next() {
		var aca struct {
			ID              uuid.UUID
			Type            string
			Nome            string
			CodigoAcademia  string
			Provincia       string
			Endereco        string
			NumeroTelefone  *string
			Email           *string
			Website         *string
			NivelEscolar    *string
			Status          string
			Cursos          []byte
			EmailVerificado bool
			CreatedAt       interface{}
			UpdatedAt       interface{}
			TotalEstudantes int
			Version         int
		}

		if err := rows.Scan(&aca.ID, &aca.Type, &aca.Nome, &aca.CodigoAcademia,
			&aca.Provincia, &aca.Endereco, &aca.NumeroTelefone, &aca.Email,
			&aca.Website, &aca.NivelEscolar, &aca.Status, &aca.Cursos,
			&aca.EmailVerificado, &aca.CreatedAt, &aca.UpdatedAt,
			&aca.TotalEstudantes, &aca.Version,
		); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		var cursos []string
		if len(aca.Cursos) > 0 {
			_ = json.Unmarshal(aca.Cursos, &cursos)
		}
		if cursos == nil {
			cursos = []string{}
		}

		acadMap := map[string]interface{}{
			"id":               aca.ID,
			"type":             aca.Type,
			"nome":             aca.Nome,
			"codigo_academia":  aca.CodigoAcademia,
			"provincia":        aca.Provincia,
			"endereco":         aca.Endereco,
			"numero_telefone":  aca.NumeroTelefone,
			"email":            aca.Email,
			"website":          aca.Website,
			"nivel_escolar":    aca.NivelEscolar,
			"status":           aca.Status,
			"cursos":           cursos,
			"email_verificado": aca.EmailVerificado,
			"created_at":       aca.CreatedAt,
			"updated_at":       aca.UpdatedAt,
			"total_estudantes": aca.TotalEstudantes,
			"version":          aca.Version,
		}

		// Ocultar email de não-admins
		if userType != "admin" {
			delete(acadMap, "email")
		}

		academias = append(academias, acadMap)
	}

	if academias == nil {
		academias = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"academias": academias,
		"total":     len(academias),
	})
}

func GetNotasEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		utils.RespondWithForbiddenError(c, "Você só pode visualizar suas próprias notas")
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
			return
		}
	}

	notasProj := getNotasProjection(c)
	notas, err := notasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"notas":            notas,
		"total":            len(notas),
	})
}

func GetFaltasEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		utils.RespondWithForbiddenError(c, "Você só pode visualizar suas próprias faltas")
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
			return
		}
	}

	faltasProj := getFaltasProjection(c)
	faltas, err := faltasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"faltas":           faltas,
		"total":            len(faltas),
	})
}

func GetHistoricoCompleto(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		utils.RespondWithForbiddenError(c, "Você só pode visualizar seu próprio histórico")
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
			return
		}
	}

	notasProj := getNotasProjection(c)
	notas, _ := notasProj.GetByEstudante(codigoEstudante)

	faltasProj := getFaltasProjection(c)
	faltas, _ := faltasProj.GetByEstudante(codigoEstudante)

	c.JSON(http.StatusOK, gin.H{
		"estudante": estudante,
		"notas":     notas,
		"faltas":    faltas,
	})
}

func GetEventosEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		utils.RespondWithForbiddenError(c, "Você só pode visualizar seus próprios eventos")
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
		"nome":             estudante.Nome,
		"eventos":          eventos,
		"total":            len(eventos),
		"message":          "Histórico completo de eventos (Event Sourcing)",
	})
}

func VerificarIntegridade(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	repository := getRepository(c)
	isValid, err := repository.VerifyIntegrity(estudante.ID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"integro":          isValid,
		"message": func() string {
			if isValid {
				return "✅ Cadeia de hashes íntegra. Eventos não foram alterados."
			}
			return "⚠️ ATENÇÃO: Cadeia de hashes comprometida!"
		}(),
	})
}

func GetEstudantePorCodigoQuery(c *gin.Context) {
	// alias para compatibilidade — chama GetEstudantePorCodigo de profile_handlers
	GetEstudantePorCodigo(c)
}

// ListarEstudantes — delegado para admin_handlers
func ListarEstudantesQuery(c *gin.Context) {
	ListarEstudantes(c)
}

// getPaginationParamsWithDefaults é alias público para outros handlers
func getPaginationParamsWithDefaults(c *gin.Context, defaultLimit int) (limit, offset int) {
	limit, offset = getPaginationParams(c)
	if limit == 50 && defaultLimit > 0 {
		limit = defaultLimit
	}
	return
}

// RespondForbiddenIfNotOwner verifica se o userID autenticado corresponde ao estudante.
// Retorna true se o acesso foi bloqueado (handler deve retornar).
func respondForbiddenIfNotOwner(c *gin.Context, estudanteID uuid.UUID, msg string) bool {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	if userType == "estudante" && userID != estudanteID {
		utils.RespondWithForbiddenError(c, msg)
		return true
	}
	return false
}

// fmtOptional retorna o valor ou nil se vazio — helper para respostas JSON.
func fmtOptional(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

// fmtRequiredStr converte string para interface{} para uso em maps.
func fmtRequiredStr(s string) interface{} {
	return s
}

// suppress unused import warning
var _ = fmt.Sprintf