package handlers

import (
	"database/sql"
	"net/http"
	"spuri/internal/middleware"
	"strconv"

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

// ✅ CORRIGIDO: Query() + loop manual para todas as queries
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
		ID              string  `db:"id" json:"id"`
		EstudanteID     string  `db:"estudante_id" json:"estudante_id"`
		CodigoEstudante string  `db:"codigo_estudante" json:"codigo_estudante"`
		AcademiaID      string  `db:"academia_id" json:"academia_id"`
		CodigoAcademia  string  `db:"codigo_academia" json:"codigo_academia"`
		Tipo            string  `db:"tipo" json:"tipo"`
		AnoInscricao    string  `db:"ano_inscricao" json:"ano_inscricao"`
		Curso           *string `db:"curso" json:"curso,omitempty"`
		Status          string  `db:"status" json:"status"`
		CreatedAt       string  `db:"created_at" json:"created_at"`
		UpdatedAt       string  `db:"updated_at" json:"updated_at"`
		EventID         *string `db:"event_id" json:"event_id,omitempty"`
		Version         *int    `db:"version" json:"version,omitempty"`
	}

	var inscricoes []InscricaoDetalhada
	var err error
	var total int

	baseQuery := `
		SELECT 
			id::text, estudante_id::text, codigo_estudante, academia_id::text, codigo_academia,
			tipo, ano_inscricao, curso, status,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as created_at,
			TO_CHAR(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as updated_at,
			COALESCE(event_id::text, '') as event_id, version
		FROM projection_inscricoes
	`

	switch userType {
	case "admin":
		if statusFilter != "" {
			query := baseQuery + ` WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
			rows, err := client.DB().Query(query, statusFilter, limit, offset)
			if err != nil {
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
					continue
				}
				inscricoes = append(inscricoes, insc)
			}
			
			client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE status = $1`, statusFilter).Scan(&total)
		} else {
			query := baseQuery + ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
			rows, err := client.DB().Query(query, limit, offset)
			if err != nil {
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
					continue
				}
				inscricoes = append(inscricoes, insc)
			}
			
			client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes`).Scan(&total)
		}

	case "academia":
		if statusFilter != "" {
			query := baseQuery + ` WHERE academia_id = $1 AND status = $2 ORDER BY created_at DESC LIMIT $3 OFFSET $4`
			rows, err := client.DB().Query(query, userID, statusFilter, limit, offset)
			if err != nil {
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
					continue
				}
				inscricoes = append(inscricoes, insc)
			}
			
			client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = $1 AND status = $2`, userID, statusFilter).Scan(&total)
		} else {
			query := baseQuery + ` WHERE academia_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
			rows, err := client.DB().Query(query, userID, limit, offset)
			if err != nil {
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
					continue
				}
				inscricoes = append(inscricoes, insc)
			}
			
			client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = $1`, userID).Scan(&total)
		}

	case "estudante":
		if statusFilter != "" {
			query := baseQuery + ` WHERE estudante_id = $1 AND status = $2 ORDER BY created_at DESC`
			rows, err := client.DB().Query(query, userID, statusFilter)
			if err != nil {
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
					continue
				}
				inscricoes = append(inscricoes, insc)
			}
		} else {
			query := baseQuery + ` WHERE estudante_id = $1 ORDER BY created_at DESC`
			rows, err := client.DB().Query(query, userID)
			if err != nil {
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
					continue
				}
				inscricoes = append(inscricoes, insc)
			}
		}
		total = len(inscricoes)

	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes":    inscricoes,
		"total":         len(inscricoes),
		"total_geral":   total,
		"limit":         limit,
		"offset":        offset,
		"has_next":      offset + len(inscricoes) < total,
		"status_filter": statusFilter,
		"user_type":     userType,
	})
}

// ✅ CORRIGIDO: Query() + loop manual
func ListarInscricoesPendentes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	
	limit, offset := getPaginationParams(c)
	client := getDbClient(c)

	type InscricaoDetalhada struct {
		ID              string  `db:"id" json:"id"`
		EstudanteID     string  `db:"estudante_id" json:"estudante_id"`
		CodigoEstudante string  `db:"codigo_estudante" json:"codigo_estudante"`
		AcademiaID      string  `db:"academia_id" json:"academia_id"`
		CodigoAcademia  string  `db:"codigo_academia" json:"codigo_academia"`
		Tipo            string  `db:"tipo" json:"tipo"`
		AnoInscricao    string  `db:"ano_inscricao" json:"ano_inscricao"`
		Curso           *string `db:"curso" json:"curso,omitempty"`
		Status          string  `db:"status" json:"status"`
		CreatedAt       string  `db:"created_at" json:"created_at"`
		UpdatedAt       string  `db:"updated_at" json:"updated_at"`
	}

	var inscricoes []InscricaoDetalhada
	var total int

	baseQuery := `
		SELECT 
			id::text, estudante_id::text, codigo_estudante, academia_id::text, codigo_academia,
			tipo, ano_inscricao, curso, status,
			TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as created_at,
			TO_CHAR(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as updated_at
		FROM projection_inscricoes WHERE status = 'espera'
	`

	switch userType {
	case "admin":
		query := baseQuery + ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		rows, err := client.DB().Query(query, limit, offset)
		if err != nil {
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
				continue
			}
			inscricoes = append(inscricoes, insc)
		}
		
		client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE status = 'espera'`).Scan(&total)

	case "academia":
		query := baseQuery + ` AND academia_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
		rows, err := client.DB().Query(query, userID, limit, offset)
		if err != nil {
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
				continue
			}
			inscricoes = append(inscricoes, insc)
		}
		
		client.DB().QueryRow(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = $1 AND status = 'espera'`, userID).Scan(&total)

	case "estudante":
		query := baseQuery + ` AND estudante_id = $1 ORDER BY created_at DESC`
		rows, err := client.DB().Query(query, userID)
		if err != nil {
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
				continue
			}
			inscricoes = append(inscricoes, insc)
		}
		total = len(inscricoes)

	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes":  inscricoes,
		"total":       len(inscricoes),
		"total_geral": total,
		"limit":       limit,
		"offset":      offset,
		"has_next":    offset + len(inscricoes) < total,
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

// ✅ CORRIGIDO: Query() + loop manual
func ListarTodasAcademias(c *gin.Context) {
	limit, offset := getPaginationParams(c)
	
	query := `
		SELECT 
			id, nome, codigo_academia, type, provincia,
			status, nivel_escolar, created_at,
			total_estudantes, total_inscricoes_pendentes
		FROM projection_academias
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	type AcademiaSimples struct {
		ID                       uuid.UUID `db:"id" json:"id"`
		Nome                     string    `db:"nome" json:"nome"`
		CodigoAcademia           string    `db:"codigo_academia" json:"codigo_academia"`
		Type                     string    `db:"type" json:"type"`
		Provincia                string    `db:"provincia" json:"provincia"`
		Status                   string    `db:"status" json:"status"`
		NivelEscolar             *string   `db:"nivel_escolar" json:"nivel_escolar"`
		CreatedAt                string    `db:"created_at" json:"created_at"`
		TotalEstudantes          int       `db:"total_estudantes" json:"total_estudantes"`
		TotalInscricoesPendentes int       `db:"total_inscricoes_pendentes" json:"total_inscricoes_pendentes"`
	}

	var academias []AcademiaSimples
	var total int
	
	client := getDbClient(c)
	rows, err := client.DB().Query(query, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academias"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var aca AcademiaSimples
		err := rows.Scan(&aca.ID, &aca.Nome, &aca.CodigoAcademia, &aca.Type, &aca.Provincia,
			&aca.Status, &aca.NivelEscolar, &aca.CreatedAt, &aca.TotalEstudantes, &aca.TotalInscricoesPendentes)
		if err != nil {
			continue
		}
		academias = append(academias, aca)
	}
	
	client.DB().QueryRow(`SELECT COUNT(*) FROM projection_academias`).Scan(&total)

	c.JSON(http.StatusOK, gin.H{
		"academias":   academias,
		"total":       len(academias),
		"total_geral": total,
		"limit":       limit,
		"offset":      offset,
		"has_next":    offset + len(academias) < total,
	})
}