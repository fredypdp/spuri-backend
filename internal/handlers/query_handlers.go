// ============================================================================
// ARQUIVO: internal/handlers/query_handlers.go
// 🔥 ATUALIZADO: Debug e tratamento de erros melhorado
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// ROTAS UNIFICADAS DE INSCRIÇÕES
// ============================================================================

// ListarInscricoes - Rota unificada GET /inscricoes
func ListarInscricoes(c *gin.Context) {
	// 🔍 LOG: Início da requisição
	log.Printf("🔵 [INSCRICOES] Iniciando ListarInscricoes")
	
	// Extrair dados do usuário do contexto
	userID, exists := middleware.GetUserID(c)
	if !exists {
		log.Printf("❌ [INSCRICOES] user_id não encontrado no contexto")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "usuário não autenticado"})
		return
	}
	
	userType, exists := middleware.GetUserType(c)
	if !exists {
		log.Printf("❌ [INSCRICOES] user_type não encontrado no contexto")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "tipo de usuário não identificado"})
		return
	}
	
	log.Printf("📋 [INSCRICOES] UserID: %v, UserType: %s", userID, userType)
	
	// Parâmetros de paginação
	limit := 50
	offset := 0
	
	if limitParam := c.Query("limit"); limitParam != "" {
		fmt.Sscanf(limitParam, "%d", &limit)
	}
	if offsetParam := c.Query("offset"); offsetParam != "" {
		fmt.Sscanf(offsetParam, "%d", &offset)
	}
	
	log.Printf("📊 [INSCRICOES] Paginação - Limit: %d, Offset: %d", limit, offset)
	
	// Filtro por status (opcional)
	statusFilter := c.Query("status")
	if statusFilter != "" {
		log.Printf("🔍 [INSCRICOES] Filtro de status: %s", statusFilter)
	}

	inscProj := getInscricoesProjection(c)
	client := getGenesisClient(c)
	
	// Estrutura para armazenar inscrições
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
	
	switch userType {
	case "admin":
		log.Printf("👑 [INSCRICOES] Processando como ADMIN - retorna todas")
		
		if statusFilter != "" {
			query := `
				SELECT 
					id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, 
					TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as created_at,
					TO_CHAR(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as updated_at,
					event_id::text as event_id, version
				FROM projection_inscricoes
				WHERE status = $1
				ORDER BY created_at DESC
				LIMIT $2 OFFSET $3
			`
			
			log.Printf("📝 [INSCRICOES] Executando query com filtro de status")
			err = client.DB().Select(&inscricoes, query, statusFilter, limit, offset)
			if err != nil {
				log.Printf("❌ [INSCRICOES] Erro na query com filtro: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições", "details": err.Error()})
				return
			}
			
			countQuery := `SELECT COUNT(*) FROM projection_inscricoes WHERE status = $1`
			err = client.DB().Get(&total, countQuery, statusFilter)
			if err != nil {
				log.Printf("⚠️ [INSCRICOES] Erro ao contar total: %v", err)
				total = len(inscricoes)
			}
		} else {
			log.Printf("📝 [INSCRICOES] Executando query sem filtro")
			
			inscricoesDTO, errDTO := inscProj.GetAll(limit, offset)
			if errDTO != nil {
				log.Printf("❌ [INSCRICOES] Erro ao buscar todas: %v", errDTO)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições", "details": errDTO.Error()})
				return
			}
			
			// Converter DTOs para formato detalhado
			for _, i := range inscricoesDTO {
				curso := i.Curso
				eventID := ""
				if i.EventID != uuid.Nil {
					eventID = i.EventID.String()
				}
				
				inscricoes = append(inscricoes, InscricaoDetalhada{
					ID:              i.ID.String(),
					EstudanteID:     i.EstudanteID.String(),
					CodigoEstudante: i.CodigoEstudante,
					AcademiaID:      i.AcademiaID.String(),
					CodigoAcademia:  i.CodigoAcademia,
					Tipo:            i.Tipo,
					AnoInscricao:    i.AnoInscricao,
					Curso:           curso,
					Status:          i.Status,
					CreatedAt:       i.CreatedAt.Format("2006-01-02T15:04:05Z"),
					UpdatedAt:       i.UpdatedAt.Format("2006-01-02T15:04:05Z"),
					EventID:         &eventID,
					Version:         &i.Version,
				})
			}
			
			totalCount, _ := inscProj.CountAll()
			total = totalCount
		}
		
	case "academia":
		log.Printf("🏫 [INSCRICOES] Processando como ACADEMIA - apenas da própria academia")
		
		// Converter userID para uuid.UUID
		academiaUUID, ok := userID.(uuid.UUID)
		if !ok {
			log.Printf("❌ [INSCRICOES] Erro ao converter userID para UUID: %v", userID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar ID da academia"})
			return
		}
		
		if statusFilter != "" {
			inscricoesDTO, errDTO := inscProj.GetByAcademia(academiaUUID, statusFilter)
			if errDTO != nil {
				log.Printf("❌ [INSCRICOES] Erro ao buscar por academia com filtro: %v", errDTO)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições", "details": errDTO.Error()})
				return
			}
			
			// Converter DTOs
			for _, i := range inscricoesDTO {
				curso := i.Curso
				eventID := ""
				if i.EventID != uuid.Nil {
					eventID = i.EventID.String()
				}
				
				inscricoes = append(inscricoes, InscricaoDetalhada{
					ID:              i.ID.String(),
					EstudanteID:     i.EstudanteID.String(),
					CodigoEstudante: i.CodigoEstudante,
					AcademiaID:      i.AcademiaID.String(),
					CodigoAcademia:  i.CodigoAcademia,
					Tipo:            i.Tipo,
					AnoInscricao:    i.AnoInscricao,
					Curso:           curso,
					Status:          i.Status,
					CreatedAt:       i.CreatedAt.Format("2006-01-02T15:04:05Z"),
					UpdatedAt:       i.UpdatedAt.Format("2006-01-02T15:04:05Z"),
					EventID:         &eventID,
					Version:         &i.Version,
				})
			}
			total = len(inscricoes)
		} else {
			query := `
				SELECT 
					id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status,
					TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as created_at,
					TO_CHAR(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as updated_at,
					event_id::text as event_id, version
				FROM projection_inscricoes
				WHERE academia_id = $1
				ORDER BY created_at DESC
				LIMIT $2 OFFSET $3
			`
			
			log.Printf("📝 [INSCRICOES] Executando query para academia_id: %s", academiaUUID.String())
			err = client.DB().Select(&inscricoes, query, academiaUUID, limit, offset)
			if err != nil {
				log.Printf("❌ [INSCRICOES] Erro na query academia: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições", "details": err.Error()})
				return
			}
			
			countQuery := `SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = $1`
			err = client.DB().Get(&total, countQuery, academiaUUID)
			if err != nil {
				log.Printf("⚠️ [INSCRICOES] Erro ao contar: %v", err)
				total = len(inscricoes)
			}
		}
		
	case "estudante":
		log.Printf("👨‍🎓 [INSCRICOES] Processando como ESTUDANTE - apenas próprias inscrições")
		
		// Converter userID para uuid.UUID
		estudanteUUID, ok := userID.(uuid.UUID)
		if !ok {
			log.Printf("❌ [INSCRICOES] Erro ao converter userID para UUID: %v", userID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar ID do estudante"})
			return
		}
		
		inscricoesDTO, errDTO := inscProj.GetByEstudante(estudanteUUID)
		if errDTO != nil {
			log.Printf("❌ [INSCRICOES] Erro ao buscar por estudante: %v", errDTO)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições", "details": errDTO.Error()})
			return
		}
		
		// Aplicar filtro de status se fornecido e converter DTOs
		for _, i := range inscricoesDTO {
			if statusFilter == "" || i.Status == statusFilter {
				curso := i.Curso
				eventID := ""
				if i.EventID != uuid.Nil {
					eventID = i.EventID.String()
				}
				
				inscricoes = append(inscricoes, InscricaoDetalhada{
					ID:              i.ID.String(),
					EstudanteID:     i.EstudanteID.String(),
					CodigoEstudante: i.CodigoEstudante,
					AcademiaID:      i.AcademiaID.String(),
					CodigoAcademia:  i.CodigoAcademia,
					Tipo:            i.Tipo,
					AnoInscricao:    i.AnoInscricao,
					Curso:           curso,
					Status:          i.Status,
					CreatedAt:       i.CreatedAt.Format("2006-01-02T15:04:05Z"),
					UpdatedAt:       i.UpdatedAt.Format("2006-01-02T15:04:05Z"),
					EventID:         &eventID,
					Version:         &i.Version,
				})
			}
		}
		total = len(inscricoes)
		
	default:
		log.Printf("❌ [INSCRICOES] Tipo de usuário inválido: %s", userType)
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}
	
	log.Printf("✅ [INSCRICOES] Sucesso! Total: %d, Retornando: %d inscrições", total, len(inscricoes))

	c.JSON(http.StatusOK, gin.H{
		"inscricoes":    inscricoes,
		"total":         len(inscricoes),
		"total_geral":   total,
		"limit":         limit,
		"offset":        offset,
		"status_filter": statusFilter,
		"user_type":     userType,
	})
}

// ListarInscricoesPendentes - Rota unificada GET /inscricoes-pendentes
func ListarInscricoesPendentes(c *gin.Context) {
	log.Printf("🔵 [INSCRICOES-PENDENTES] Iniciando")
	
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	
	log.Printf("📋 [INSCRICOES-PENDENTES] UserType: %s", userType)
	
	inscProj := getInscricoesProjection(c)
	client := getGenesisClient(c)
	
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
	var err error
	
	switch userType {
	case "admin":
		query := `
			SELECT 
				id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status,
				TO_CHAR(created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as created_at,
				TO_CHAR(updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as updated_at
			FROM projection_inscricoes
			WHERE status = 'espera'
			ORDER BY created_at DESC
		`
		err = client.DB().Select(&inscricoes, query)
		
	case "academia":
		academiaUUID, _ := userID.(uuid.UUID)
		inscricoesDTO, errDTO := inscProj.GetByAcademia(academiaUUID, "espera")
		if errDTO != nil {
			err = errDTO
		} else {
			for _, i := range inscricoesDTO {
				curso := i.Curso
				inscricoes = append(inscricoes, InscricaoDetalhada{
					ID:              i.ID.String(),
					EstudanteID:     i.EstudanteID.String(),
					CodigoEstudante: i.CodigoEstudante,
					AcademiaID:      i.AcademiaID.String(),
					CodigoAcademia:  i.CodigoAcademia,
					Tipo:            i.Tipo,
					AnoInscricao:    i.AnoInscricao,
					Curso:           curso,
					Status:          i.Status,
					CreatedAt:       i.CreatedAt.Format("2006-01-02T15:04:05Z"),
					UpdatedAt:       i.UpdatedAt.Format("2006-01-02T15:04:05Z"),
				})
			}
		}
		
	case "estudante":
		estudanteUUID, _ := userID.(uuid.UUID)
		inscricoesDTO, errDTO := inscProj.GetByEstudante(estudanteUUID)
		if errDTO != nil {
			err = errDTO
		} else {
			for _, i := range inscricoesDTO {
				if i.Status == "espera" {
					curso := i.Curso
					inscricoes = append(inscricoes, InscricaoDetalhada{
						ID:              i.ID.String(),
						EstudanteID:     i.EstudanteID.String(),
						CodigoEstudante: i.CodigoEstudante,
						AcademiaID:      i.AcademiaID.String(),
						CodigoAcademia:  i.CodigoAcademia,
						Tipo:            i.Tipo,
						AnoInscricao:    i.AnoInscricao,
						Curso:           curso,
						Status:          i.Status,
						CreatedAt:       i.CreatedAt.Format("2006-01-02T15:04:05Z"),
						UpdatedAt:       i.UpdatedAt.Format("2006-01-02T15:04:05Z"),
					})
				}
			}
		}
		
	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}
	
	if err != nil {
		log.Printf("❌ [INSCRICOES-PENDENTES] Erro: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições pendentes", "details": err.Error()})
		return
	}

	log.Printf("✅ [INSCRICOES-PENDENTES] Retornando %d inscrições", len(inscricoes))
	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
		"status":     "espera",
		"user_type":  userType,
	})
}

// Resto das funções (GetNotasEstudante, GetFaltasEstudante, etc.) permanecem iguais...
// [código anterior mantido]

// GetNotasEstudante busca notas por código
func GetNotasEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	// Buscar estudante por código
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar permissões
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

	// Buscar notas
	notasProj := getNotasProjection(c)
	notas, err := notasProj.GetByEstudante(estudante.ID)
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

// GetFaltasEstudante busca faltas por código
func GetFaltasEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	// Buscar estudante por código
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar permissões
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

	// Buscar faltas
	faltasProj := getFaltasProjection(c)
	faltas, err := faltasProj.GetByEstudante(estudante.ID)
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

// GetHistoricoCompleto busca histórico por código
func GetHistoricoCompleto(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar permissões
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

	// Buscar notas
	notasProj := getNotasProjection(c)
	notas, _ := notasProj.GetByEstudante(estudante.ID)

	// Buscar faltas
	faltasProj := getFaltasProjection(c)
	faltas, _ := faltasProj.GetByEstudante(estudante.ID)

	// Buscar inscrições
	inscProj := getInscricoesProjection(c)
	inscricoes, _ := inscProj.GetByEstudante(estudante.ID)

	c.JSON(http.StatusOK, gin.H{
		"estudante":  estudante,
		"notas":      notas,
		"faltas":     faltas,
		"inscricoes": inscricoes,
	})
}

// GetEventosEstudante busca eventos por código
func GetEventosEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar permissões
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	if userType == "estudante" && userID != estudante.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}

	// Buscar eventos do ledger
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

// VerificarIntegridade busca por código
func VerificarIntegridade(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar integridade
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

// GetMinhasInscricoes retorna inscrições do estudante logado
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

// GetMeuHistorico retorna histórico completo do estudante logado
func GetMeuHistorico(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	// Buscar dados
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar dados"})
		return
	}

	notasProj := getNotasProjection(c)
	notas, _ := notasProj.GetByEstudante(userID)

	faltasProj := getFaltasProjection(c)
	faltas, _ := faltasProj.GetByEstudante(userID)

	inscProj := getInscricoesProjection(c)
	inscricoes, _ := inscProj.GetByEstudante(userID)

	c.JSON(http.StatusOK, gin.H{
		"estudante":  estudante,
		"notas":      notas,
		"faltas":     faltas,
		"inscricoes": inscricoes,
	})
}

// ListarTodasAcademias lista todas as academias
func ListarTodasAcademias(c *gin.Context) {
	query := `
		SELECT 
			id, nome, codigo_academia, type, provincia,
			status, nivel_escolar, created_at,
			total_estudantes, total_inscricoes_pendentes
		FROM projection_academias
		ORDER BY created_at DESC
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
	client := getGenesisClient(c)
	if err := client.DB().Select(&academias, query); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academias"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"academias": academias,
		"total":     len(academias),
	})
}