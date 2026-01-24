// ============================================================================
// ARQUIVO: internal/handlers/missing_handlers.go
// Handlers faltantes
// ============================================================================

package handlers

import (
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// InscricaoEscola - estudante solicita inscrição em escola
func InscricaoEscola(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("🎓 [INSCRICAO-ESCOLA] Iniciando inscrição - UserID: %s", userID)

	var req struct {
		CodigoAcademia      string  `json:"codigo_academia" binding:"required"`
		AnoEscolarInscricao string  `json:"ano_escolar_inscricao" binding:"required"`
		CursoMedio          *string `json:"curso_medio"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [INSCRICAO-ESCOLA-DEBUG] Erro validação JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("🔍 [INSCRICAO-ESCOLA-DEBUG] Dados recebidos - Academia: %s, Ano: %s", 
		req.CodigoAcademia, req.AnoEscolarInscricao)

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [INSCRICAO-ESCOLA-DEBUG] Buscando academia código: %s", req.CodigoAcademia)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [INSCRICAO-ESCOLA-DEBUG] Academia não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	log.Printf("✅ [INSCRICAO-ESCOLA-DEBUG] Academia encontrada: %s (status: %s)", 
		academiaDTO.Nome, academiaDTO.Status)

	if academiaDTO.Status != "ativo" {
		log.Printf("❌ [INSCRICAO-ESCOLA-DEBUG] Academia inativa - Status: %s", academiaDTO.Status)
		c.JSON(http.StatusBadRequest, gin.H{"error": "academia não está ativa"})
		return
	}

	// Carregar agregado estudante
	repository := getRepository(c)
	log.Printf("📦 [INSCRICAO-ESCOLA-DEBUG] Carregando agregado estudante ID: %s", userID)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("❌ [INSCRICAO-ESCOLA-DEBUG] Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	log.Printf("✅ [INSCRICAO-ESCOLA-DEBUG] Agregado carregado com sucesso")

	// Solicitar inscrição
	log.Printf("💾 [INSCRICAO-ESCOLA-DEBUG] Solicitando inscrição tipo: escola")
	err = estudante.SolicitarInscricao(
		req.CodigoAcademia,
		"escola",
		req.AnoEscolarInscricao,
		req.CursoMedio,
	)

	if err != nil {
		log.Printf("❌ [INSCRICAO-ESCOLA-DEBUG] Erro ao solicitar inscrição: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("💾 [INSCRICAO-ESCOLA-DEBUG] Salvando eventos - Total: %d", len(estudante.UncommittedEvents))
	if err := repository.Save(estudante); err != nil {
		log.Printf("❌ [INSCRICAO-ESCOLA-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar inscrição"})
		return
	}

	log.Printf("✅ [INSCRICAO-ESCOLA] Inscrição criada - Academia: %s", academiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "inscrição criada com sucesso",
		"status":   "espera",
		"academia": academiaDTO.Nome,
	})
}

// InscricaoUniversidade - estudante solicita inscrição em universidade
func InscricaoUniversidade(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("🎓 [INSCRICAO-UNIVERSIDADE] Iniciando inscrição - UserID: %s", userID)

	var req struct {
		CodigoAcademia string  `json:"codigo_academia" binding:"required"`
		AnoInscricao   string  `json:"ano_inscricao" binding:"required"`
		Curso          *string `json:"curso" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [INSCRICAO-UNIVERSIDADE-DEBUG] Erro validação JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("🔍 [INSCRICAO-UNIVERSIDADE-DEBUG] Dados recebidos - Academia: %s, Curso: %v", 
		req.CodigoAcademia, req.Curso)

	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [INSCRICAO-UNIVERSIDADE-DEBUG] Buscando academia código: %s", req.CodigoAcademia)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [INSCRICAO-UNIVERSIDADE-DEBUG] Academia não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	log.Printf("✅ [INSCRICAO-UNIVERSIDADE-DEBUG] Academia encontrada: %s (status: %s)", 
		academiaDTO.Nome, academiaDTO.Status)

	if academiaDTO.Status != "ativo" {
		log.Printf("❌ [INSCRICAO-UNIVERSIDADE-DEBUG] Academia inativa - Status: %s", academiaDTO.Status)
		c.JSON(http.StatusBadRequest, gin.H{"error": "academia não está ativa"})
		return
	}

	repository := getRepository(c)
	log.Printf("📦 [INSCRICAO-UNIVERSIDADE-DEBUG] Carregando agregado estudante ID: %s", userID)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("❌ [INSCRICAO-UNIVERSIDADE-DEBUG] Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	log.Printf("✅ [INSCRICAO-UNIVERSIDADE-DEBUG] Agregado carregado com sucesso")

	log.Printf("💾 [INSCRICAO-UNIVERSIDADE-DEBUG] Solicitando inscrição tipo: superior")
	err = estudante.SolicitarInscricao(
		req.CodigoAcademia,
		"superior",
		req.AnoInscricao,
		req.Curso,
	)

	if err != nil {
		log.Printf("❌ [INSCRICAO-UNIVERSIDADE-DEBUG] Erro ao solicitar inscrição: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("💾 [INSCRICAO-UNIVERSIDADE-DEBUG] Salvando eventos - Total: %d", len(estudante.UncommittedEvents))
	if err := repository.Save(estudante); err != nil {
		log.Printf("❌ [INSCRICAO-UNIVERSIDADE-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar inscrição"})
		return
	}

	log.Printf("✅ [INSCRICAO-UNIVERSIDADE] Inscrição criada - Academia: %s", academiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "inscrição criada com sucesso",
		"status":   "espera",
		"academia": academiaDTO.Nome,
	})
}

// BuscarUsuario - admin busca usuário por tipo e ID
func BuscarUsuario(c *gin.Context) {
	tipo := c.Query("tipo")
	id := c.Query("id")

	log.Printf("🔍 [BUSCAR-USUARIO] Buscando usuário - Tipo: %s, ID: %s", tipo, id)

	if tipo == "" || id == "" {
		log.Printf("❌ [BUSCAR-USUARIO-DEBUG] Parâmetros faltando - tipo: %s, id: %s", tipo, id)
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo e id são obrigatórios"})
		return
	}

	userID, err := uuid.Parse(id)
	if err != nil {
		log.Printf("❌ [BUSCAR-USUARIO-DEBUG] UUID inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	log.Printf("🔍 [BUSCAR-USUARIO-DEBUG] UUID parseado: %s", userID)

	switch tipo {
	case "estudante":
		log.Printf("🔍 [BUSCAR-USUARIO-DEBUG] Buscando estudante...")
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			log.Printf("❌ [BUSCAR-USUARIO-DEBUG] Estudante não encontrado: %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
			return
		}
		log.Printf("✅ [BUSCAR-USUARIO] Estudante encontrado: %s", estudante.Nome)
		c.JSON(http.StatusOK, gin.H{"tipo": "estudante", "dados": estudante})

	case "academia":
		log.Printf("🔍 [BUSCAR-USUARIO-DEBUG] Buscando academia...")
		academiaProj := getAcademiaProjection(c)
		academia, err := academiaProj.GetByID(userID)
		if err != nil || academia == nil {
			log.Printf("❌ [BUSCAR-USUARIO-DEBUG] Academia não encontrada: %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
			return
		}
		log.Printf("✅ [BUSCAR-USUARIO] Academia encontrada: %s", academia.Nome)
		c.JSON(http.StatusOK, gin.H{"tipo": "academia", "dados": academia})

	case "admin":
		log.Printf("🔍 [BUSCAR-USUARIO-DEBUG] Buscando admin...")
		adminProj := getAdminProjection(c)
		admin, err := adminProj.GetByID(userID)
		if err != nil || admin == nil {
			log.Printf("❌ [BUSCAR-USUARIO-DEBUG] Admin não encontrado: %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "admin não encontrado"})
			return
		}
		log.Printf("✅ [BUSCAR-USUARIO] Admin encontrado: %s", admin.Nome)
		c.JSON(http.StatusOK, gin.H{"tipo": "admin", "dados": admin})

	default:
		log.Printf("❌ [BUSCAR-USUARIO-DEBUG] Tipo inválido: %s", tipo)
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo inválido (estudante, academia, admin)"})
	}
}

// GetAllProjectionStatuses - alias para GetAllProjectionsStatus
func GetAllProjectionStatuses(c *gin.Context) {
	log.Printf("🔍 [GET-PROJECTION-STATUSES] Redirecionando para GetAllProjectionsStatus")
	GetAllProjectionsStatus(c)
}