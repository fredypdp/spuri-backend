package handlers

import (
	"fmt"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/middleware"
	"spuri/internal/utils"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func ListarInscricoes(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	userType, exists := middleware.GetUserType(c)
	if !exists {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	limit, offset := getPaginationParams(c)
	statusFilter := c.Query("status")
	client := getDbClient(c)

	type InscricaoDetalhada struct {
		ID              uuid.UUID  `json:"id"`
		EstudanteID     uuid.UUID  `json:"estudante_id"`
		CodigoEstudante string     `json:"codigo_estudante"`
		AcademiaID      uuid.UUID  `json:"academia_id"`
		CodigoAcademia  string     `json:"codigo_academia"`
		Tipo            string     `json:"tipo"`
		AnoInscricao    string     `json:"ano_inscricao"`
		Curso           *string    `json:"curso,omitempty"`
		Status          string     `json:"status"`
		StatusUsado     bool       `json:"status_usado"`
		CreatedAt       time.Time  `json:"created_at"`
		UpdatedAt       time.Time  `json:"updated_at"`
		EventID         *uuid.UUID `json:"event_id,omitempty"`
		Version         *int       `json:"version,omitempty"`
	}

	var inscricoes []InscricaoDetalhada
	var total int
	var query string
	var countQuery string

	switch userType {
	case "admin":
		if statusFilter != "" {
			safeStatus := db.SafeString(statusFilter)
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				WHERE status = '%s'
				ORDER BY created_at DESC LIMIT %d OFFSET %d
			`, safeStatus, limit, offset)
			countQuery = fmt.Sprintf(`SELECT COUNT(*) FROM projection_inscricoes WHERE status = '%s'`, safeStatus)
		} else {
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				ORDER BY created_at DESC LIMIT %d OFFSET %d
			`, limit, offset)
			countQuery = `SELECT COUNT(*) FROM projection_inscricoes`
		}

	case "academia":
		if statusFilter != "" {
			safeStatus := db.SafeString(statusFilter)
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				WHERE academia_id = '%s' AND status = '%s'
				ORDER BY created_at DESC LIMIT %d OFFSET %d
			`, userID, safeStatus, limit, offset)
			countQuery = fmt.Sprintf(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = '%s' AND status = '%s'`, userID, safeStatus)
		} else {
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				WHERE academia_id = '%s'
				ORDER BY created_at DESC LIMIT %d OFFSET %d
			`, userID, limit, offset)
			countQuery = fmt.Sprintf(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = '%s'`, userID)
		}

	case "estudante":
		if statusFilter != "" {
			safeStatus := db.SafeString(statusFilter)
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				WHERE estudante_id = '%s' AND status = '%s'
				ORDER BY created_at DESC
			`, userID, safeStatus)
		} else {
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				WHERE estudante_id = '%s'
				ORDER BY created_at DESC
			`, userID)
		}

	default:
		utils.RespondWithForbiddenError(c, "Acesso negado")
		return
	}

	rows, err := client.DB().Query(query)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var insc InscricaoDetalhada
		if err := rows.Scan(&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &insc.Curso, &insc.Status, &insc.StatusUsado,
			&insc.CreatedAt, &insc.UpdatedAt, &insc.EventID, &insc.Version); err == nil {
			inscricoes = append(inscricoes, insc)
		}
	}

	if userType == "estudante" {
		total = len(inscricoes)
	} else {
		client.DB().QueryRow(countQuery).Scan(&total)
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes":    inscricoes,
		"total":         len(inscricoes),
		"total_geral":   total,
		"limit":         limit,
		"offset":        offset,
		"has_next":      offset+len(inscricoes) < total,
		"status_filter": statusFilter,
		"user_type":     userType,
	})
}

func ListarInscricoesPendentes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	limit, offset := getPaginationParams(c)
	client := getDbClient(c)

	type InscricaoDetalhada struct {
		ID              uuid.UUID `json:"id"`
		EstudanteID     uuid.UUID `json:"estudante_id"`
		CodigoEstudante string    `json:"codigo_estudante"`
		AcademiaID      uuid.UUID `json:"academia_id"`
		CodigoAcademia  string    `json:"codigo_academia"`
		Tipo            string    `json:"tipo"`
		AnoInscricao    string    `json:"ano_inscricao"`
		Curso           *string   `json:"curso,omitempty"`
		Status          string    `json:"status"`
		StatusUsado     bool      `json:"status_usado"`
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
		EventID         uuid.UUID `json:"event_id"`
		Version         int       `json:"version"`
	}

	var inscricoes []InscricaoDetalhada
	var total int
	var query string
	var countQuery string

	switch userType {
	case "admin":
		query = fmt.Sprintf(`
			SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
			FROM projection_inscricoes
			WHERE status = 'espera'
			ORDER BY created_at DESC LIMIT %d OFFSET %d
		`, limit, offset)
		countQuery = `SELECT COUNT(*) FROM projection_inscricoes WHERE status = 'espera'`

	case "academia":
		query = fmt.Sprintf(`
			SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
			FROM projection_inscricoes
			WHERE status = 'espera' AND academia_id = '%s'
			ORDER BY created_at DESC LIMIT %d OFFSET %d
		`, userID, limit, offset)
		countQuery = fmt.Sprintf(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = '%s' AND status = 'espera'`, userID)

	case "estudante":
		query = fmt.Sprintf(`
			SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, status_usado, created_at, updated_at, event_id, version
			FROM projection_inscricoes
			WHERE status = 'espera' AND estudante_id = '%s'
			ORDER BY created_at DESC
		`, userID)

	default:
		utils.RespondWithForbiddenError(c, "Acesso negado")
		return
	}

	rows, err := client.DB().Query(query)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var insc InscricaoDetalhada
		if err := rows.Scan(&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &insc.Curso, &insc.Status, &insc.StatusUsado,
			&insc.CreatedAt, &insc.UpdatedAt, &insc.EventID, &insc.Version); err == nil {
			inscricoes = append(inscricoes, insc)
		}
	}

	if userType == "estudante" {
		total = len(inscricoes)
	} else {
		client.DB().QueryRow(countQuery).Scan(&total)
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes":  inscricoes,
		"total":       len(inscricoes),
		"total_geral": total,
		"limit":       limit,
		"offset":      offset,
		"has_next":    offset+len(inscricoes) < total,
		"status":      "espera",
		"user_type":   userType,
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

	inscProj := getInscricoesProjection(c)
	inscricoes, _ := inscProj.GetByEstudante(estudante.ID)

	c.JSON(http.StatusOK, gin.H{
		"estudante":  estudante,
		"notas":      notas,
		"faltas":     faltas,
		"inscricoes": inscricoes,
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

func GetMinhasInscricoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscProj := getInscricoesProjection(c)
	inscricoes, err := inscProj.GetByEstudante(userID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
	})
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

	inscProj := getInscricoesProjection(c)
	inscricoes, _ := inscProj.GetByEstudante(userID)

	c.JSON(http.StatusOK, gin.H{
		"estudante":  estudante,
		"notas":      notas,
		"faltas":     faltas,
		"inscricoes": inscricoes,
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
			email_verificado, created_at, updated_at, total_estudantes,
			total_inscricoes_pendentes, version
		FROM projection_academias
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
			ID                       uuid.UUID
			Type                     string
			Nome                     string
			CodigoAcademia           string
			Provincia                string
			Endereco                 string
			NumeroTelefone           *string
			Email                    *string
			Website                  *string
			NivelEscolar             *string
			Status                   string
			Cursos                   []byte
			EmailVerificado          bool
			CreatedAt                time.Time
			UpdatedAt                time.Time
			TotalEstudantes          int
			TotalInscricoesPendentes int
			Version                  int
		}

		if err := rows.Scan(&aca.ID, &aca.Type, &aca.Nome, &aca.CodigoAcademia,
			&aca.Provincia, &aca.Endereco, &aca.NumeroTelefone, &aca.Email,
			&aca.Website, &aca.NivelEscolar, &aca.Status, &aca.Cursos,
			&aca.EmailVerificado, &aca.CreatedAt, &aca.UpdatedAt,
			&aca.TotalEstudantes, &aca.TotalInscricoesPendentes, &aca.Version); err == nil {
			
			academiaMap := map[string]interface{}{
				"id":                         aca.ID,
				"type":                       aca.Type,
				"nome":                       aca.Nome,
				"codigo_academia":            aca.CodigoAcademia,
				"provincia":                  aca.Provincia,
				"endereco":                   aca.Endereco,
				"numero_telefone":            aca.NumeroTelefone,
				"email":                      aca.Email,
				"website":                    aca.Website,
				"nivel_escolar":              aca.NivelEscolar,
				"status":                     aca.Status,
				"email_verificado":           aca.EmailVerificado,
				"created_at":                 aca.CreatedAt,
				"updated_at":                 aca.UpdatedAt,
				"total_estudantes":           aca.TotalEstudantes,
				"total_inscricoes_pendentes": aca.TotalInscricoesPendentes,
				"version":                    aca.Version,
			}

			var cursos []string
			if len(aca.Cursos) > 0 {
				academiaMap["cursos"] = cursos
			}

			academias = append(academias, academiaMap)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"academias":    academias,
		"total":        len(academias),
		"tipo_usuario": userType,
	})
}