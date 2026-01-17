// ============================================================================
// ARQUIVO: internal/handlers/query_handlers.go
// 🔥 ATUALIZADO: Rotas unificadas de inscrições com lógica por tipo de usuário
// ============================================================================

package handlers

import (
	"fmt"
	"net/http"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// ROTAS UNIFICADAS DE INSCRIÇÕES
// ============================================================================

// ListarInscricoes - Rota unificada GET /inscricoes
// - Admin: retorna TODAS as inscrições
// - Academia: retorna apenas inscrições da própria academia
// - Estudante: retorna apenas inscrições do próprio estudante
func ListarInscricoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	
	// Parâmetros de paginação
	limit := 50
	offset := 0
	
	if limitParam := c.Query("limit"); limitParam != "" {
		fmt.Sscanf(limitParam, "%d", &limit)
	}
	if offsetParam := c.Query("offset"); offsetParam != "" {
		fmt.Sscanf(offsetParam, "%d", &offset)
	}
	
	// Filtro por status (opcional)
	statusFilter := c.Query("status") // "espera", "aprovado", "reprovado", ou vazio

	inscProj := getInscricoesProjection(c)
	client := getGenesisClient(c)
	
	var inscricoes []interface{}
	var err error
	var total int
	
	switch userType {
	case "admin":
		// ADMIN: Lista TODAS as inscrições
		if statusFilter != "" {
			query := `
				SELECT 
					id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, created_at, updated_at, 
					event_id, version
				FROM projection_inscricoes
				WHERE status = $1
				ORDER BY created_at DESC
				LIMIT $2 OFFSET $3
			`
			err = client.DB().Select(&inscricoes, query, statusFilter, limit, offset)
			
			countQuery := `SELECT COUNT(*) FROM projection_inscricoes WHERE status = $1`
			client.DB().Get(&total, countQuery, statusFilter)
		} else {
			inscricoesDTO, errDTO := inscProj.GetAll(limit, offset)
			if errDTO != nil {
				err = errDTO
			} else {
				for _, i := range inscricoesDTO {
					inscricoes = append(inscricoes, i)
				}
			}
			total, _ = inscProj.CountAll()
		}
		
	case "academia":
		// ACADEMIA: Lista apenas inscrições da própria academia
		if statusFilter != "" {
			inscricoesDTO, errDTO := inscProj.GetByAcademia(userID, statusFilter)
			if errDTO != nil {
				err = errDTO
			} else {
				for _, i := range inscricoesDTO {
					inscricoes = append(inscricoes, i)
				}
				total = len(inscricoesDTO)
			}
		} else {
			query := `
				SELECT 
					id, estudante_id, codigo_estudante, academia_id, codigo_academia,
					tipo, ano_inscricao, curso, status, created_at, updated_at, 
					event_id, version
				FROM projection_inscricoes
				WHERE academia_id = $1
				ORDER BY created_at DESC
				LIMIT $2 OFFSET $3
			`
			err = client.DB().Select(&inscricoes, query, userID, limit, offset)
			
			countQuery := `SELECT COUNT(*) FROM projection_inscricoes WHERE academia_id = $1`
			client.DB().Get(&total, countQuery, userID)
		}
		
	case "estudante":
		// ESTUDANTE: Lista apenas suas próprias inscrições
		inscricoesDTO, errDTO := inscProj.GetByEstudante(userID)
		if errDTO != nil {
			err = errDTO
		} else {
			// Aplicar filtro de status se fornecido
			for _, i := range inscricoesDTO {
				if statusFilter == "" || i.Status == statusFilter {
					inscricoes = append(inscricoes, i)
				}
			}
			total = len(inscricoes)
		}
		
	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes":   inscricoes,
		"total":        len(inscricoes),
		"total_geral":  total,
		"limit":        limit,
		"offset":       offset,
		"status_filter": statusFilter,
		"user_type":    userType,
	})
}

// ListarInscricoesPendentes - Rota unificada GET /inscricoes-pendentes
// - Admin: retorna TODAS as inscrições pendentes
// - Academia: retorna apenas inscrições pendentes da própria academia
// - Estudante: retorna apenas inscrições pendentes do próprio estudante
func ListarInscricoesPendentes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	
	inscProj := getInscricoesProjection(c)
	client := getGenesisClient(c)
	
	var inscricoes []interface{}
	var err error
	
	switch userType {
	case "admin":
		// ADMIN: Todas as inscrições pendentes
		query := `
			SELECT 
				id, estudante_id, codigo_estudante, academia_id, codigo_academia,
				tipo, ano_inscricao, curso, status, created_at, updated_at, 
				event_id, version
			FROM projection_inscricoes
			WHERE status = 'espera'
			ORDER BY created_at DESC
		`
		err = client.DB().Select(&inscricoes, query)
		
	case "academia":
		// ACADEMIA: Apenas inscrições pendentes da própria academia
		inscricoesDTO, errDTO := inscProj.GetByAcademia(userID, "espera")
		if errDTO != nil {
			err = errDTO
		} else {
			for _, i := range inscricoesDTO {
				inscricoes = append(inscricoes, i)
			}
		}
		
	case "estudante":
		// ESTUDANTE: Apenas suas inscrições pendentes
		inscricoesDTO, errDTO := inscProj.GetByEstudante(userID)
		if errDTO != nil {
			err = errDTO
		} else {
			for _, i := range inscricoesDTO {
				if i.Status == "espera" {
					inscricoes = append(inscricoes, i)
				}
			}
		}
		
	default:
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado"})
		return
	}
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições pendentes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
		"status":     "espera",
		"user_type":  userType,
	})
}

// ============================================================================
// OUTRAS ROTAS DE QUERY (mantidas para compatibilidade)
// ============================================================================

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