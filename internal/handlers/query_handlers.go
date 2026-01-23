package handlers

import (
	"database/sql"
	"fmt"
	"log"
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
		log.Println("[ListarInscricoes] Usuário não autenticado")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "usuário não autenticado"})
		return
	}

	userType, exists := middleware.GetUserType(c)
	if !exists {
		log.Println("[ListarInscricoes] Tipo de usuário não identificado")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tipo de usuário não identificado"})
		return
	}

	limit, offset := getPaginationParams(c)
	statusFilter := c.Query("status")
	client := getDbClient(c)
	
	log.Printf("[ListarInscricoes] userID=%s, userType=%s, limit=%d, offset=%d, status=%s", 
		userID, userType, limit, offset, statusFilter)

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
	var query string
	var countQuery string

	switch userType {
	case "admin":
		if statusFilter != "" {
			safeStatus := db.SafeString(statusFilter)
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				WHERE status = '%s'
				ORDER BY created_at DESC LIMIT %d OFFSET %d
			`, safeStatus, limit, offset)
			countQuery = fmt.Sprintf(`SELECT COUNT(*) FROM projection_inscricoes WHERE status = '%s'`, safeStatus)
		} else {
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, created_at, updated_at, event_id, version
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
					tipo, ano_inscricao, curso, status, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				WHERE academia_id = '%s' AND status = '%s'
				ORDER BY created_at DESC LIMIT %d OFFSET %d
			`, userID, safeStatus, limit, offset)
			countQuery = fmt.Sprintf(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = '%s' AND status = '%s'`, userID, safeStatus)
		} else {
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, created_at, updated_at, event_id, version
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
					tipo, ano_inscricao, curso, status, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				WHERE estudante_id = '%s' AND status = '%s'
				ORDER BY created_at DESC
			`, userID, safeStatus)
		} else {
			query = fmt.Sprintf(`
				SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, created_at, updated_at, event_id, version
				FROM projection_inscricoes
				WHERE estudante_id = '%s'
				ORDER BY created_at DESC
			`, userID)
		}

	default:
		log.Printf("[ListarInscricoes] Tipo de usuário inválido: %s", userType)
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	log.Printf("[ListarInscricoes] Query: %s", query)

	rows, err := client.DB().Query(query)
	if err != nil {
		log.Printf("[ListarInscricoes] Erro na query: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var insc InscricaoDetalhada
		err := rows.Scan(&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &insc.Curso, &insc.Status,
			&insc.CreatedAt, &insc.UpdatedAt, &insc.EventID, &insc.Version)
		if err != nil {
			log.Printf("[ListarInscricoes] Erro ao fazer scan: %v", err)
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

	if userType == "estudante" {
		total = len(inscricoes)
	} else {
		client.DB().QueryRow(countQuery).Scan(&total)
	}

	log.Printf("[ListarInscricoes] Retornando %d inscrições (total_geral=%d)", len(inscricoes), total)

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
	
	log.Printf("[ListarInscricoesPendentes] userID=%s, userType=%s, limit=%d, offset=%d", 
		userID, userType, limit, offset)

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
	var query string
	var countQuery string

	switch userType {
	case "admin":
		query = fmt.Sprintf(`
			SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, created_at, updated_at
			FROM projection_inscricoes
			WHERE status = 'espera'
			ORDER BY created_at DESC LIMIT %d OFFSET %d
		`, limit, offset)
		countQuery = `SELECT COUNT(*) FROM projection_inscricoes WHERE status = 'espera'`

	case "academia":
		query = fmt.Sprintf(`
			SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, created_at, updated_at
			FROM projection_inscricoes
			WHERE status = 'espera' AND academia_id = '%s'
			ORDER BY created_at DESC LIMIT %d OFFSET %d
		`, userID, limit, offset)
		countQuery = fmt.Sprintf(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = '%s' AND status = 'espera'`, userID)

	case "estudante":
		query = fmt.Sprintf(`
			SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, created_at, updated_at
			FROM projection_inscricoes
			WHERE status = 'espera' AND estudante_id = '%s'
			ORDER BY created_at DESC
		`, userID)

	default:
		log.Printf("[ListarInscricoesPendentes] Tipo de usuário inválido: %s", userType)
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	log.Printf("[ListarInscricoesPendentes] Query: %s", query)

	rows, err := client.DB().Query(query)
	if err != nil {
		log.Printf("[ListarInscricoesPendentes] Erro na query: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var insc InscricaoDetalhada
		err := rows.Scan(&insc.ID, &insc.EstudanteID, &insc.CodigoEstudante, &insc.AcademiaID,
			&insc.CodigoAcademia, &insc.Tipo, &insc.AnoInscricao, &insc.Curso, &insc.Status,
			&insc.CreatedAt, &insc.UpdatedAt)
		if err != nil {
			log.Printf("[ListarInscricoesPendentes] Erro ao fazer scan: %v", err)
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

	if userType == "estudante" {
		total = len(inscricoes)
	} else {
		client.DB().QueryRow(countQuery).Scan(&total)
	}

	log.Printf("[ListarInscricoesPendentes] Retornando %d inscrições pendentes (total_geral=%d)", len(inscricoes), total)

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