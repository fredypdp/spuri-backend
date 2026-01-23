package handlers

import (
	"database/sql"
	"net/http"
	"spuri/internal/middleware"
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
		c.JSON(http.StatusUnauthorized, gin.H{"error": "usuário não autenticado"})
		return
	}

	userType, exists := middleware.GetUserType(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tipo de usuário não identificado"})
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
		CreatedAt       time.Time  `json:"created_at"`
		UpdatedAt       time.Time  `json:"updated_at"`
		EventID         *uuid.UUID `json:"event_id,omitempty"`
		Version         *int       `json:"version,omitempty"`
	}

	var inscricoes []InscricaoDetalhada
	var total int

	baseQuery := `
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status,
			created_at, updated_at,
			event_id, version
		FROM projection_inscricoes
	`

	var rows *sql.Rows
	var err error

	switch userType {
	case "admin":
		if statusFilter != "" {
			query := baseQuery + ` WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
			rows, err = client.DB().Query(query, statusFilter, limit, offset)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
				return
			}
			client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE status = $1`, statusFilter).Scan(&total)
		} else {
			query := baseQuery + ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
			rows, err = client.DB().Query(query, limit, offset)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
				return
			}
			client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes`).Scan(&total)
		}

	case "academia":
		if statusFilter != "" {
			query := baseQuery + ` WHERE academia_id = $1 AND status = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`
			rows, err = client.DB().Query(query, userID, statusFilter, limit, offset)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
				return
			}
			client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = $1 AND status = $2`, userID, statusFilter).Scan(&total)
		} else {
			query := baseQuery + ` WHERE academia_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
			rows, err = client.DB().Query(query, userID, limit, offset)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
				return
			}
			client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = $1`, userID).Scan(&total)
		}

	case "estudante":
		if statusFilter != "" {
			query := baseQuery + ` WHERE estudante_id = $1 AND status = $2 ORDER BY created_at DESC`
			rows, err = client.DB().Query(query, userID, statusFilter)
		} else {
			query := baseQuery + ` WHERE estudante_id = $1 ORDER BY created_at DESC`
			rows, err = client.DB().Query(query, userID)
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
			return
		}

	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	defer rows.Close()

	for rows.Next() {
		var insc InscricaoDetalhada
		err := rows.Scan(&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &insc.Curso, &insc.Status,
			&insc.CreatedAt, &insc.UpdatedAt, &insc.EventID, &insc.Version)
		if err != nil {
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

	if userType == "estudante" {
		total = len(inscricoes)
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
		CreatedAt       time.Time `json:"created_at"`
		UpdatedAt       time.Time `json:"updated_at"`
	}

	var inscricoes []InscricaoDetalhada
	var total int

	baseQuery := `
		SELECT 
			id, estudante_id, codigo_estudante, academia_id, codigo_academia,
			tipo, ano_inscricao, curso, status,
			created_at, updated_at
		FROM projection_inscricoes WHERE status = 'espera'
	`

	var rows *sql.Rows
	var err error

	switch userType {
	case "admin":
		query := baseQuery + ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		rows, err = client.DB().Query(query, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
			return
		}
		client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE status = 'espera'`).Scan(&total)

	case "academia":
		query := baseQuery + ` AND academia_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		rows, err = client.DB().Query(query, userID, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
			return
		}
		client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = $1 AND status = 'espera'`, userID).Scan(&total)

	case "estudante":
		query := baseQuery + ` AND estudante_id = $1 ORDER BY created_at DESC`
		rows, err = client.DB().Query(query, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
			return
		}

	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	defer rows.Close()

	for rows.Next() {
		var insc InscricaoDetalhada
		err := rows.Scan(&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &insc.Curso, &insc.Status,
			&insc.CreatedAt, &insc.UpdatedAt)
		if err != nil {
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

	if userType == "estudante" {
		total = len(inscricoes)
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
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
	}

	notasProj := getNotasProjection(c)
	notas, err := notasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar notas"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
	}

	faltasProj := getFaltasProjection(c)
	faltas, err := faltasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar faltas"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	repository := getRepository(c)
	eventos, err := repository.GetEventHistory(estudante.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar eventos"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	repository := getRepository(c)
	isValid, err := repository.VerifyIntegrity(estudante.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar integridade"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"integro":          isValid,
		"message": func() string {
			if isValid {
				return "Cadeia de hashes íntegra. Eventos não foram alterados."
			}
			return "ATENÇÃO: Cadeia de hashes comprometida!"
		}(),
	})
}

func GetMinhasInscricoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	inscProj := getInscricoesProjection(c)
	inscricoes, err := inscProj.GetByEstudante(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar dados"})
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

// ListarTodasAcademias lista todas academias (acesso para todos usuários logados)
func ListarTodasAcademias(c *gin.Context) {
	_, _ = middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	type AcademiaSimples struct {
		ID                       uuid.UUID `json:"id"`
		Nome                     string    `json:"nome"`
		CodigoAcademia           string    `json:"codigo_academia"`
		Type                     string    `json:"type"`
		Provincia                string    `json:"provincia"`
		Status                   string    `json:"status"`
		NivelEscolar             *string   `json:"nivel_escolar"`
		CreatedAt                string    `json:"created_at"`
		TotalEstudantes          int       `json:"total_estudantes"`
		TotalInscricoesPendentes int       `json:"total_inscricoes_pendentes"`
	}

	var academias []AcademiaSimples
	client := getDbClient(c)

	query := `
		SELECT 
			id, nome, codigo_academia, type, provincia,
			status, nivel_escolar, created_at,
			total_estudantes, total_inscricoes_pendentes
		FROM projection_academias
		ORDER BY created_at DESC
	`

	rows, err := client.DB().Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academias"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var aca AcademiaSimples
		err := rows.Scan(
			&aca.ID, &aca.Nome, &aca.CodigoAcademia, &aca.Type, &aca.Provincia,
			&aca.Status, &aca.NivelEscolar, &aca.CreatedAt,
			&aca.TotalEstudantes, &aca.TotalInscricoesPendentes,
		)
		if err != nil {
			continue
		}
		academias = append(academias, aca)
	}

	c.JSON(http.StatusOK, gin.H{
		"academias":    academias,
		"total":        len(academias),
		"tipo_usuario": userType,
	})
}