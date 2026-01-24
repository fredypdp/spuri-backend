package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/middleware"
	"strconv"
	"time"

	"spuri/internal/db"
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
	
	log.Printf("🔍 [PAGINATION-DEBUG] Limit: %d, Offset: %d", limit, offset)
	return limit, offset
}

func ListarInscricoes(c *gin.Context) {
	userID, exists := middleware.GetUserID(c)
	if !exists {
		log.Println("❌ [LISTAR-INSCRICOES-DEBUG] Usuário não autenticado")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "usuário não autenticado"})
		return
	}

	userType, exists := middleware.GetUserType(c)
	if !exists {
		log.Println("❌ [LISTAR-INSCRICOES-DEBUG] Tipo de usuário não identificado")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tipo de usuário não identificado"})
		return
	}

	limit, offset := getPaginationParams(c)
	statusFilter := c.Query("status")
	client := getDbClient(c)
	
	log.Printf("📋 [LISTAR-INSCRICOES] Iniciando listagem - UserID: %s, Type: %s, Status: %s", 
		userID, userType, statusFilter)

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
		log.Printf("🔍 [LISTAR-INSCRICOES-DEBUG] Modo ADMIN - listando todas inscrições")
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
		log.Printf("🔍 [LISTAR-INSCRICOES-DEBUG] Modo ACADEMIA - filtrando por academia_id: %s", userID)
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
		log.Printf("🔍 [LISTAR-INSCRICOES-DEBUG] Modo ESTUDANTE - filtrando por estudante_id: %s", userID)
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
		log.Printf("❌ [LISTAR-INSCRICOES-DEBUG] Tipo de usuário inválido: %s", userType)
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	log.Printf("🔍 [LISTAR-INSCRICOES-DEBUG] Query: %s", query)

	rows, err := client.DB().Query(query)
	if err != nil {
		log.Printf("❌ [LISTAR-INSCRICOES-DEBUG] Erro na query: %v", err)
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
			log.Printf("⚠️ [LISTAR-INSCRICOES-DEBUG] Erro ao fazer scan: %v", err)
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

	if userType == "estudante" {
		total = len(inscricoes)
	} else {
		client.DB().QueryRow(countQuery).Scan(&total)
	}

	log.Printf("✅ [LISTAR-INSCRICOES] Retornando %d inscrições (total_geral=%d)", len(inscricoes), total)

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
	
	log.Printf("⏳ [LISTAR-INSCRICOES-PENDENTES] Iniciando - UserID: %s, Type: %s", userID, userType)

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
		log.Printf("🔍 [LISTAR-INSCRICOES-PENDENTES-DEBUG] Modo ADMIN")
		query = fmt.Sprintf(`
			SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, created_at, updated_at
			FROM projection_inscricoes
			WHERE status = 'espera'
			ORDER BY created_at DESC LIMIT %d OFFSET %d
		`, limit, offset)
		countQuery = `SELECT COUNT(*) FROM projection_inscricoes WHERE status = 'espera'`

	case "academia":
		log.Printf("🔍 [LISTAR-INSCRICOES-PENDENTES-DEBUG] Modo ACADEMIA - ID: %s", userID)
		query = fmt.Sprintf(`
			SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, created_at, updated_at
			FROM projection_inscricoes
			WHERE status = 'espera' AND academia_id = '%s'
			ORDER BY created_at DESC LIMIT %d OFFSET %d
		`, userID, limit, offset)
		countQuery = fmt.Sprintf(`SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = '%s' AND status = 'espera'`, userID)

	case "estudante":
		log.Printf("🔍 [LISTAR-INSCRICOES-PENDENTES-DEBUG] Modo ESTUDANTE - ID: %s", userID)
		query = fmt.Sprintf(`
			SELECT id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, created_at, updated_at
			FROM projection_inscricoes
			WHERE status = 'espera' AND estudante_id = '%s'
			ORDER BY created_at DESC
		`, userID)

	default:
		log.Printf("❌ [LISTAR-INSCRICOES-PENDENTES-DEBUG] Tipo inválido: %s", userType)
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	log.Printf("🔍 [LISTAR-INSCRICOES-PENDENTES-DEBUG] Query: %s", query)

	rows, err := client.DB().Query(query)
	if err != nil {
		log.Printf("❌ [LISTAR-INSCRICOES-PENDENTES-DEBUG] Erro na query: %v", err)
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
			log.Printf("⚠️ [LISTAR-INSCRICOES-PENDENTES-DEBUG] Erro ao fazer scan: %v", err)
			continue
		}
		inscricoes = append(inscricoes, insc)
	}

	if userType == "estudante" {
		total = len(inscricoes)
	} else {
		client.DB().QueryRow(countQuery).Scan(&total)
	}

	log.Printf("✅ [LISTAR-INSCRICOES-PENDENTES] Retornando %d pendentes (total=%d)", len(inscricoes), total)

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
	log.Printf("📝 [GET-NOTAS-ESTUDANTE] Buscando notas - Código: %s", codigoEstudante)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		log.Printf("❌ [GET-NOTAS-ESTUDANTE-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	log.Printf("✅ [GET-NOTAS-ESTUDANTE-DEBUG] Estudante encontrado: %s", estudante.Nome)

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	log.Printf("🔍 [GET-NOTAS-ESTUDANTE-DEBUG] Verificando permissões - Type: %s, UserID: %s", userType, userID)

	if userType == "estudante" && userID != estudante.ID {
		log.Printf("❌ [GET-NOTAS-ESTUDANTE-DEBUG] Acesso negado - estudante tentando ver notas de outro")
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			log.Printf("❌ [GET-NOTAS-ESTUDANTE-DEBUG] Estudante não pertence à academia")
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
		log.Printf("✅ [GET-NOTAS-ESTUDANTE-DEBUG] Academia verificada: %s", academiaDTO.Nome)
	}

	notasProj := getNotasProjection(c)
	log.Printf("🔍 [GET-NOTAS-ESTUDANTE-DEBUG] Buscando notas do estudante...")
	notas, err := notasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		log.Printf("❌ [GET-NOTAS-ESTUDANTE-DEBUG] Erro ao buscar notas: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar notas"})
		return
	}

	log.Printf("✅ [GET-NOTAS-ESTUDANTE] Retornando %d notas", len(notas))

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"notas":            notas,
		"total":            len(notas),
	})
}

func GetFaltasEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")
	log.Printf("📅 [GET-FALTAS-ESTUDANTE] Buscando faltas - Código: %s", codigoEstudante)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		log.Printf("❌ [GET-FALTAS-ESTUDANTE-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	log.Printf("✅ [GET-FALTAS-ESTUDANTE-DEBUG] Estudante encontrado: %s", estudante.Nome)

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	log.Printf("🔍 [GET-FALTAS-ESTUDANTE-DEBUG] Verificando permissões - Type: %s", userType)

	if userType == "estudante" && userID != estudante.ID {
		log.Printf("❌ [GET-FALTAS-ESTUDANTE-DEBUG] Acesso negado")
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			log.Printf("❌ [GET-FALTAS-ESTUDANTE-DEBUG] Estudante não pertence à academia")
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
		log.Printf("✅ [GET-FALTAS-ESTUDANTE-DEBUG] Academia verificada: %s", academiaDTO.Nome)
	}

	faltasProj := getFaltasProjection(c)
	log.Printf("🔍 [GET-FALTAS-ESTUDANTE-DEBUG] Buscando faltas...")
	faltas, err := faltasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		log.Printf("❌ [GET-FALTAS-ESTUDANTE-DEBUG] Erro ao buscar faltas: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar faltas"})
		return
	}

	log.Printf("✅ [GET-FALTAS-ESTUDANTE] Retornando %d registros de faltas", len(faltas))

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"faltas":           faltas,
		"total":            len(faltas),
	})
}

func GetHistoricoCompleto(c *gin.Context) {
	codigoEstudante := c.Param("codigo")
	log.Printf("📚 [GET-HISTORICO-COMPLETO] Buscando histórico completo - Código: %s", codigoEstudante)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		log.Printf("❌ [GET-HISTORICO-COMPLETO-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	log.Printf("✅ [GET-HISTORICO-COMPLETO-DEBUG] Estudante encontrado: %s", estudante.Nome)

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	log.Printf("🔍 [GET-HISTORICO-COMPLETO-DEBUG] Verificando permissões - Type: %s", userType)

	if userType == "estudante" && userID != estudante.ID {
		log.Printf("❌ [GET-HISTORICO-COMPLETO-DEBUG] Acesso negado")
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			log.Printf("❌ [GET-HISTORICO-COMPLETO-DEBUG] Estudante não pertence à academia")
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
	}

	log.Printf("🔍 [GET-HISTORICO-COMPLETO-DEBUG] Buscando notas...")
	notasProj := getNotasProjection(c)
	notas, _ := notasProj.GetByEstudante(codigoEstudante)
	log.Printf("✅ [GET-HISTORICO-COMPLETO-DEBUG] Notas: %d", len(notas))

	log.Printf("🔍 [GET-HISTORICO-COMPLETO-DEBUG] Buscando faltas...")
	faltasProj := getFaltasProjection(c)
	faltas, _ := faltasProj.GetByEstudante(codigoEstudante)
	log.Printf("✅ [GET-HISTORICO-COMPLETO-DEBUG] Faltas: %d", len(faltas))

	log.Printf("🔍 [GET-HISTORICO-COMPLETO-DEBUG] Buscando inscrições...")
	inscProj := getInscricoesProjection(c)
	inscricoes, _ := inscProj.GetByEstudante(estudante.ID)
	log.Printf("✅ [GET-HISTORICO-COMPLETO-DEBUG] Inscrições: %d", len(inscricoes))

	log.Printf("✅ [GET-HISTORICO-COMPLETO] Histórico completo retornado")

	c.JSON(http.StatusOK, gin.H{
		"estudante":  estudante,
		"notas":      notas,
		"faltas":     faltas,
		"inscricoes": inscricoes,
	})
}

func GetEventosEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")
	log.Printf("📜 [GET-EVENTOS-ESTUDANTE] Buscando eventos - Código: %s", codigoEstudante)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		log.Printf("❌ [GET-EVENTOS-ESTUDANTE-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	log.Printf("✅ [GET-EVENTOS-ESTUDANTE-DEBUG] Estudante encontrado: %s (ID: %s)", estudante.Nome, estudante.ID)

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		log.Printf("❌ [GET-EVENTOS-ESTUDANTE-DEBUG] Acesso negado")
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	repository := getRepository(c)
	log.Printf("🔍 [GET-EVENTOS-ESTUDANTE-DEBUG] Buscando histórico de eventos no ledger...")
	eventos, err := repository.GetEventHistory(estudante.ID)
	if err != nil {
		log.Printf("❌ [GET-EVENTOS-ESTUDANTE-DEBUG] Erro ao buscar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar eventos"})
		return
	}

	log.Printf("✅ [GET-EVENTOS-ESTUDANTE] Retornando %d eventos", len(eventos))

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
	log.Printf("🔐 [VERIFICAR-INTEGRIDADE] Verificando integridade - Código: %s", codigoEstudante)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		log.Printf("❌ [VERIFICAR-INTEGRIDADE-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	log.Printf("✅ [VERIFICAR-INTEGRIDADE-DEBUG] Estudante encontrado: %s (ID: %s)", estudante.Nome, estudante.ID)

	repository := getRepository(c)
	log.Printf("🔍 [VERIFICAR-INTEGRIDADE-DEBUG] Verificando cadeia de hashes...")
	isValid, err := repository.VerifyIntegrity(estudante.ID)
	if err != nil {
		log.Printf("❌ [VERIFICAR-INTEGRIDADE-DEBUG] Erro ao verificar integridade: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar integridade"})
		return
	}

	if isValid {
		log.Printf("✅ [VERIFICAR-INTEGRIDADE] Cadeia íntegra!")
	} else {
		log.Printf("🚨 [VERIFICAR-INTEGRIDADE] ATENÇÃO: Cadeia comprometida!")
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
	log.Printf("📋 [GET-MINHAS-INSCRICOES] Buscando inscrições do usuário: %s", userID)

	inscProj := getInscricoesProjection(c)
	inscricoes, err := inscProj.GetByEstudante(userID)
	if err != nil {
		log.Printf("❌ [GET-MINHAS-INSCRICOES-DEBUG] Erro ao buscar inscrições: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}

	log.Printf("✅ [GET-MINHAS-INSCRICOES] Retornando %d inscrições", len(inscricoes))

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
	})
}

func GetMeuHistorico(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📚 [GET-MEU-HISTORICO] Buscando histórico do usuário: %s", userID)

	estudanteProj := getEstudanteProjection(c)
	log.Printf("🔍 [GET-MEU-HISTORICO-DEBUG] Buscando dados do estudante...")
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil {
		log.Printf("❌ [GET-MEU-HISTORICO-DEBUG] Erro ao buscar estudante: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar dados"})
		return
	}

	log.Printf("✅ [GET-MEU-HISTORICO-DEBUG] Estudante: %s", estudante.Nome)

	log.Printf("🔍 [GET-MEU-HISTORICO-DEBUG] Buscando notas...")
	notasProj := getNotasProjection(c)
	notas, _ := notasProj.GetByEstudante(estudante.CodigoEstudante)
	log.Printf("✅ [GET-MEU-HISTORICO-DEBUG] Notas: %d", len(notas))

	log.Printf("🔍 [GET-MEU-HISTORICO-DEBUG] Buscando faltas...")
	faltasProj := getFaltasProjection(c)
	faltas, _ := faltasProj.GetByEstudante(estudante.CodigoEstudante)
	log.Printf("✅ [GET-MEU-HISTORICO-DEBUG] Faltas: %d", len(faltas))

	log.Printf("🔍 [GET-MEU-HISTORICO-DEBUG] Buscando inscrições...")
	inscProj := getInscricoesProjection(c)
	inscricoes, _ := inscProj.GetByEstudante(userID)
	log.Printf("✅ [GET-MEU-HISTORICO-DEBUG] Inscrições: %d", len(inscricoes))

	log.Printf("✅ [GET-MEU-HISTORICO] Histórico completo retornado")

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

	log.Printf("🏫 [LISTAR-TODAS-ACADEMIAS] Listando academias - Tipo usuário: %s", userType)

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

	log.Printf("🔍 [LISTAR-TODAS-ACADEMIAS-DEBUG] Executando query...")
	rows, err := client.DB().Query(query)
	if err != nil {
		log.Printf("❌ [LISTAR-TODAS-ACADEMIAS-DEBUG] Erro na query: %v", err)
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
			log.Printf("⚠️ [LISTAR-TODAS-ACADEMIAS-DEBUG] Erro ao fazer scan: %v", err)
			continue
		}
		academias = append(academias, aca)
	}

	log.Printf("✅ [LISTAR-TODAS-ACADEMIAS] Retornando %d academias", len(academias))

	c.JSON(http.StatusOK, gin.H{
		"academias":    academias,
		"total":        len(academias),
		"tipo_usuario": userType,
	})
}