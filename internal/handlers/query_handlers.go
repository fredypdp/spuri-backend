// ============================================================================
// ARQUIVO: internal/handlers/query_handlers.go
// ATUALIZADO: Usar codigo_estudante nas rotas
// ============================================================================

package handlers

import (
	"fmt"
	"net/http"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
)

// 🔥 ATUALIZADO: GetNotasEstudante busca por CÓDIGO
func GetNotasEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo") // 🔥 MUDOU de estudanteId para codigo

	// 🔥 BUSCAR estudante por CÓDIGO
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
		if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
	}

	// Buscar notas da projeção (CQRS - Read Model)
	notasProj := getNotasProjection(c)
	notas, err := notasProj.GetByEstudante(estudante.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar notas"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome": estudante.Nome,
		"notas": notas,
		"total": len(notas),
	})
}

// 🔥 ATUALIZADO: GetFaltasEstudante busca por CÓDIGO
func GetFaltasEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo") // 🔥 MUDOU

	// 🔥 BUSCAR estudante por CÓDIGO
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
		if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
			return
		}
	}

	// Buscar faltas da projeção
	faltasProj := getFaltasProjection(c)
	faltas, err := faltasProj.GetByEstudante(estudante.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar faltas"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome": estudante.Nome,
		"faltas": faltas,
		"total": len(faltas),
	})
}

// 🔥 ATUALIZADO: GetHistoricoCompleto busca por CÓDIGO
func GetHistoricoCompleto(c *gin.Context) {
	codigoEstudante := c.Param("codigo") // 🔥 MUDOU

	// 🔥 BUSCAR estudante por CÓDIGO
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

	// Verificar permissão para academia
	if userType == "academia" {
		if estudante.IDAcademia == nil || *estudante.IDAcademia != userID {
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

// 🔥 ATUALIZADO: GetEventosEstudante busca por CÓDIGO
func GetEventosEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo") // 🔥 MUDOU

	// 🔥 BUSCAR estudante por CÓDIGO
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
		"nome": estudante.Nome,
		"eventos": eventos,
		"total": len(eventos),
		"message": "Histórico completo de eventos (Event Sourcing)",
	})
}

// 🔥 ATUALIZADO: VerificarIntegridade busca por CÓDIGO
func VerificarIntegridade(c *gin.Context) {
	codigoEstudante := c.Param("codigo") // 🔥 MUDOU

	// 🔥 BUSCAR estudante por CÓDIGO
	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Verificar integridade da hash chain
	repository := getRepository(c)
	isValid, err := repository.VerifyIntegrity(estudante.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar integridade"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome": estudante.Nome,
		"integro": isValid,
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

// ListarInscricoesPendentes lista inscrições pendentes de uma academia
func ListarInscricoesPendentes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	inscProj := getInscricoesProjection(c)
	inscricoes, err := inscProj.GetByAcademia(userID, "espera")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
	})
}

// 🔥 NOVO: ListarTodasInscricoes lista todas as inscrições (admin/consulta)
func ListarTodasInscricoes(c *gin.Context) {
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
	status := c.Query("status") // "espera", "aprovado", "reprovado", ou vazio para todos

	inscProj := getInscricoesProjection(c)
	
	var inscricoes []interface{}
	var err error
	var total int
	
	if status != "" {
		// Buscar por status específico
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
		client := getGenesisClient(c)
		err = client.DB().Select(&inscricoes, query, status, limit, offset)
		
		// Contar total com filtro
		countQuery := `SELECT COUNT(*) FROM projection_inscricoes WHERE status = $1`
		client.DB().Get(&total, countQuery, status)
	} else {
		// Buscar todas
		inscricoesDTO, errDTO := inscProj.GetAll(limit, offset)
		if errDTO != nil {
			err = errDTO
		} else {
			for _, i := range inscricoesDTO {
				inscricoes = append(inscricoes, i)
			}
		}
		
		// Contar total
		total, _ = inscProj.CountAll()
	}
	
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar inscrições"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inscricoes": inscricoes,
		"total":      len(inscricoes),
		"total_geral": total,
		"limit":      limit,
		"offset":     offset,
	})
}