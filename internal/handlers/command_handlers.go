package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RegistrarNotas registra notas para um estudante
func RegistrarNotas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📝 [REGISTRAR-NOTAS] Iniciando registro de notas - UserID: %s", userID)

	var req struct {
		CodigoEstudante      string  `json:"codigo_estudante" binding:"required"`
		AnoLectivo           string  `json:"ano_lectivo" binding:"required"`
		Periodo              string  `json:"periodo" binding:"required"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
		Nota                 float64 `json:"nota" binding:"required,min=0,max=20"`
		Observacao           *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [REGISTRAR-NOTAS-DEBUG] Erro validação JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("🔍 [REGISTRAR-NOTAS-DEBUG] Dados recebidos - Estudante: %s, Matéria: %s, Nota: %.2f", 
		req.CodigoEstudante, req.MateriaDisciplinarID, req.Nota)

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [REGISTRAR-NOTAS-DEBUG] Buscando academia ID: %s", userID)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [REGISTRAR-NOTAS-DEBUG] Academia não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}
	log.Printf("✅ [REGISTRAR-NOTAS-DEBUG] Academia encontrada: %s (código: %s)", 
		academiaDTO.Nome, academiaDTO.CodigoAcademia)

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	log.Printf("🔍 [REGISTRAR-NOTAS-DEBUG] Buscando estudante código: %s", req.CodigoEstudante)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		log.Printf("❌ [REGISTRAR-NOTAS-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}
	log.Printf("✅ [REGISTRAR-NOTAS-DEBUG] Estudante encontrado: %s (ID: %s)", 
		estudanteDTO.Nome, estudanteDTO.ID)

	// Verificar se estudante pertence à academia
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		log.Printf("❌ [REGISTRAR-NOTAS-DEBUG] Estudante não pertence à academia - Est: %v, Acad: %s", 
			estudanteDTO.CodigoAcademia, academiaDTO.CodigoAcademia)
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	// Validar matéria
	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		log.Printf("❌ [REGISTRAR-NOTAS-DEBUG] UUID matéria inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "materia_disciplinar_id inválido"})
		return
	}

	materiasProj := getMateriasProjection(c)
	log.Printf("🔍 [REGISTRAR-NOTAS-DEBUG] Buscando matéria ID: %s", materiaID)
	materiaDTO, _ := materiasProj.GetByID(materiaID)
	if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		log.Printf("❌ [REGISTRAR-NOTAS-DEBUG] Matéria não encontrada ou não pertence à academia")
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}
	log.Printf("✅ [REGISTRAR-NOTAS-DEBUG] Matéria encontrada: %s", materiaDTO.Nome)

	// Carregar agregado estudante
	repository := getRepository(c)
	log.Printf("📦 [REGISTRAR-NOTAS-DEBUG] Carregando agregado estudante ID: %s", estudanteDTO.ID)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		log.Printf("❌ [REGISTRAR-NOTAS-DEBUG] Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	log.Printf("✅ [REGISTRAR-NOTAS-DEBUG] Agregado carregado com sucesso")
	
	// Registrar nota
	log.Printf("💾 [REGISTRAR-NOTAS-DEBUG] Registrando nota no agregado...")
	err = estudante.RegistrarNota(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		req.Periodo,
		materiaID,
		req.Nota,
		req.Observacao,
	)

	if err != nil {
		log.Printf("❌ [REGISTRAR-NOTAS-DEBUG] Erro ao registrar nota: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	log.Printf("💾 [REGISTRAR-NOTAS-DEBUG] Salvando eventos - Total: %d", len(estudante.UncommittedEvents))
	if err := repository.Save(estudante); err != nil {
		log.Printf("❌ [REGISTRAR-NOTAS-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar notas"})
		return
	}

	log.Printf("✅ [REGISTRAR-NOTAS] Nota registrada com sucesso - Estudante: %s, Nota: %.2f", 
		req.CodigoEstudante, req.Nota)

	c.JSON(http.StatusCreated, gin.H{
		"message":   "nota registrada com sucesso",
		"estudante": req.CodigoEstudante,
		"materia":   materiaDTO.Nome,
		"nota":      req.Nota,
	})
}

// RegistrarFaltas registra faltas para um estudante
func RegistrarFaltas(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📝 [REGISTRAR-FALTAS] Iniciando registro de faltas - UserID: %s", userID)

	var req struct {
		CodigoEstudante      string  `json:"codigo_estudante" binding:"required"`
		AnoLectivo           string  `json:"ano_lectivo" binding:"required"`
		Data                 string  `json:"data" binding:"required"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id" binding:"required"`
		Quantidade           int     `json:"quantidade" binding:"required,min=1"`
		Observacao           *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] Erro validação JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("🔍 [REGISTRAR-FALTAS-DEBUG] Dados recebidos - Estudante: %s, Quantidade: %d", 
		req.CodigoEstudante, req.Quantidade)

	// Parse data
	data, err := time.Parse("2006-01-02", req.Data)
	if err != nil {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] Erro parse data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "formato de data inválido (use YYYY-MM-DD)"})
		return
	}
	log.Printf("✅ [REGISTRAR-FALTAS-DEBUG] Data parseada: %s", data.Format("2006-01-02"))

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [REGISTRAR-FALTAS-DEBUG] Buscando academia ID: %s", userID)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] Academia não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}
	log.Printf("✅ [REGISTRAR-FALTAS-DEBUG] Academia encontrada: %s", academiaDTO.Nome)

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	log.Printf("🔍 [REGISTRAR-FALTAS-DEBUG] Buscando estudante código: %s", req.CodigoEstudante)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}
	log.Printf("✅ [REGISTRAR-FALTAS-DEBUG] Estudante encontrado: %s", estudanteDTO.Nome)

	// Verificar se estudante pertence à academia
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] Estudante não pertence à academia")
		c.JSON(http.StatusForbidden, gin.H{"error": "estudante não pertence a esta academia"})
		return
	}

	// Validar matéria
	materiaID, err := uuid.Parse(req.MateriaDisciplinarID)
	if err != nil {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] UUID matéria inválido: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "materia_disciplinar_id inválido"})
		return
	}

	materiasProj := getMateriasProjection(c)
	log.Printf("🔍 [REGISTRAR-FALTAS-DEBUG] Buscando matéria ID: %s", materiaID)
	materiaDTO, _ := materiasProj.GetByID(materiaID)
	if materiaDTO == nil || materiaDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] Matéria não encontrada ou não pertence à academia")
		c.JSON(http.StatusForbidden, gin.H{"error": "matéria não pertence a esta academia"})
		return
	}
	log.Printf("✅ [REGISTRAR-FALTAS-DEBUG] Matéria encontrada: %s", materiaDTO.Nome)

	// Carregar agregado estudante
	repository := getRepository(c)
	log.Printf("📦 [REGISTRAR-FALTAS-DEBUG] Carregando agregado estudante ID: %s", estudanteDTO.ID)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	log.Printf("✅ [REGISTRAR-FALTAS-DEBUG] Agregado carregado com sucesso")
	
	// Registrar falta
	log.Printf("💾 [REGISTRAR-FALTAS-DEBUG] Registrando falta no agregado...")
	err = estudante.RegistrarFalta(
		academiaDTO.CodigoAcademia,
		req.AnoLectivo,
		data,
		materiaID,
		req.Quantidade,
		req.Observacao,
	)

	if err != nil {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] Erro ao registrar falta: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	log.Printf("💾 [REGISTRAR-FALTAS-DEBUG] Salvando eventos - Total: %d", len(estudante.UncommittedEvents))
	if err := repository.Save(estudante); err != nil {
		log.Printf("❌ [REGISTRAR-FALTAS-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar faltas"})
		return
	}

	log.Printf("✅ [REGISTRAR-FALTAS] Faltas registradas com sucesso - Estudante: %s, Qtd: %d", 
		req.CodigoEstudante, req.Quantidade)

	c.JSON(http.StatusCreated, gin.H{
		"message":    "faltas registradas com sucesso",
		"estudante":  req.CodigoEstudante,
		"materia":    materiaDTO.Nome,
		"quantidade": req.Quantidade,
	})
}

// AprovarInscricao aprova uma inscrição de estudante
func AprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📝 [APROVAR-INSCRICAO] Iniciando aprovação - UserID: %s", userID)

	var req struct {
		CodigoEstudante string  `json:"codigo_estudante" binding:"required"`
		Tipo            string  `json:"tipo" binding:"required"`
		AnoInscricao    string  `json:"ano_inscricao" binding:"required"`
		Curso           *string `json:"curso"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [APROVAR-INSCRICAO-DEBUG] Erro validação JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("🔍 [APROVAR-INSCRICAO-DEBUG] Dados recebidos - Estudante: %s, Tipo: %s", 
		req.CodigoEstudante, req.Tipo)

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [APROVAR-INSCRICAO-DEBUG] Buscando academia ID: %s", userID)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [APROVAR-INSCRICAO-DEBUG] Academia não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}
	log.Printf("✅ [APROVAR-INSCRICAO-DEBUG] Academia encontrada: %s (código: %s)", 
		academiaDTO.Nome, academiaDTO.CodigoAcademia)

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	log.Printf("🔍 [APROVAR-INSCRICAO-DEBUG] Buscando estudante código: %s", req.CodigoEstudante)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		log.Printf("❌ [APROVAR-INSCRICAO-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}
	log.Printf("✅ [APROVAR-INSCRICAO-DEBUG] Estudante encontrado: %s (ID: %s)", 
		estudanteDTO.Nome, estudanteDTO.ID)

	// Buscar inscrição pendente
	client := getDbClient(c)
	safeCodEst := db.SafeString(req.CodigoEstudante)
	safeCodAcad := db.SafeString(academiaDTO.CodigoAcademia)
	safeTipo := db.SafeString(req.Tipo)

	queryInsc := fmt.Sprintf(`
		SELECT id FROM projection_inscricoes 
		WHERE codigo_estudante = '%s' 
		AND codigo_academia = '%s' 
		AND tipo = '%s' 
		AND status = 'espera'
		LIMIT 1
	`, safeCodEst, safeCodAcad, safeTipo)

	log.Printf("🔍 [APROVAR-INSCRICAO-DEBUG] Query inscrição: %s", queryInsc)

	var inscricaoID uuid.UUID
	err = client.DB().QueryRow(queryInsc).Scan(&inscricaoID)
	if err != nil {
		log.Printf("❌ [APROVAR-INSCRICAO-DEBUG] Inscrição não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada ou já processada"})
		return
	}
	log.Printf("✅ [APROVAR-INSCRICAO-DEBUG] Inscrição encontrada: %s", inscricaoID)

	// Carregar agregado academia
	repository := getRepository(c)
	log.Printf("📦 [APROVAR-INSCRICAO-DEBUG] Carregando agregado academia ID: %s", academiaDTO.ID)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		log.Printf("❌ [APROVAR-INSCRICAO-DEBUG] Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar academia"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	log.Printf("✅ [APROVAR-INSCRICAO-DEBUG] Agregado carregado com sucesso")
	
	// Aprovar inscrição
	log.Printf("💾 [APROVAR-INSCRICAO-DEBUG] Aprovando inscrição no agregado...")
	err = academia.AprovarInscricao(
		estudanteDTO.ID,
		inscricaoID,
		req.Tipo,
		req.AnoInscricao,
		req.Curso,
	)
	
	if err != nil {
		log.Printf("❌ [APROVAR-INSCRICAO-DEBUG] Erro ao aprovar: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	log.Printf("💾 [APROVAR-INSCRICAO-DEBUG] Salvando eventos - Total: %d", len(academia.UncommittedEvents))
	if err := repository.Save(academia); err != nil {
		log.Printf("❌ [APROVAR-INSCRICAO-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao aprovar inscrição"})
		return
	}

	log.Printf("✅ [APROVAR-INSCRICAO] Inscrição aprovada - Estudante: %s, Tipo: %s", 
		req.CodigoEstudante, req.Tipo)

	c.JSON(http.StatusOK, gin.H{
		"message":   "inscrição aprovada com sucesso",
		"estudante": req.CodigoEstudante,
		"tipo":      req.Tipo,
	})
}

// ReprovarInscricao reprova uma inscrição de estudante
func ReprovarInscricao(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📝 [REPROVAR-INSCRICAO] Iniciando reprovação - UserID: %s", userID)

	var req struct {
		CodigoEstudante string `json:"codigo_estudante" binding:"required"`
		Motivo          string `json:"motivo" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [REPROVAR-INSCRICAO-DEBUG] Erro validação JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("🔍 [REPROVAR-INSCRICAO-DEBUG] Dados recebidos - Estudante: %s, Motivo: %s", 
		req.CodigoEstudante, req.Motivo)

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [REPROVAR-INSCRICAO-DEBUG] Buscando academia ID: %s", userID)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [REPROVAR-INSCRICAO-DEBUG] Academia não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}
	log.Printf("✅ [REPROVAR-INSCRICAO-DEBUG] Academia encontrada: %s", academiaDTO.Nome)

	// Buscar estudante
	estudanteProj := getEstudanteProjection(c)
	log.Printf("🔍 [REPROVAR-INSCRICAO-DEBUG] Buscando estudante código: %s", req.CodigoEstudante)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		log.Printf("❌ [REPROVAR-INSCRICAO-DEBUG] Estudante não encontrado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}
	log.Printf("✅ [REPROVAR-INSCRICAO-DEBUG] Estudante encontrado: %s", estudanteDTO.Nome)

	// Buscar inscrição pendente
	client := getDbClient(c)
	safeCodEst := db.SafeString(req.CodigoEstudante)
	safeCodAcad := db.SafeString(academiaDTO.CodigoAcademia)

	queryInsc := fmt.Sprintf(`
		SELECT id FROM projection_inscricoes 
		WHERE codigo_estudante = '%s' 
		AND codigo_academia = '%s' 
		AND status = 'espera'
		LIMIT 1
	`, safeCodEst, safeCodAcad)

	log.Printf("🔍 [REPROVAR-INSCRICAO-DEBUG] Query inscrição: %s", queryInsc)

	var inscricaoID uuid.UUID
	err = client.DB().QueryRow(queryInsc).Scan(&inscricaoID)
	if err != nil {
		log.Printf("❌ [REPROVAR-INSCRICAO-DEBUG] Inscrição não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "inscrição não encontrada ou já processada"})
		return
	}
	log.Printf("✅ [REPROVAR-INSCRICAO-DEBUG] Inscrição encontrada: %s", inscricaoID)

	// Carregar agregado academia
	repository := getRepository(c)
	log.Printf("📦 [REPROVAR-INSCRICAO-DEBUG] Carregando agregado academia ID: %s", academiaDTO.ID)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		log.Printf("❌ [REPROVAR-INSCRICAO-DEBUG] Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar academia"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	log.Printf("✅ [REPROVAR-INSCRICAO-DEBUG] Agregado carregado com sucesso")
	
	// Reprovar inscrição
	log.Printf("💾 [REPROVAR-INSCRICAO-DEBUG] Reprovando inscrição no agregado...")
	err = academia.ReprovarInscricao(
		estudanteDTO.ID,
		inscricaoID,
		req.Motivo,
	)
	
	if err != nil {
		log.Printf("❌ [REPROVAR-INSCRICAO-DEBUG] Erro ao reprovar: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	log.Printf("💾 [REPROVAR-INSCRICAO-DEBUG] Salvando eventos - Total: %d", len(academia.UncommittedEvents))
	if err := repository.Save(academia); err != nil {
		log.Printf("❌ [REPROVAR-INSCRICAO-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao reprovar inscrição"})
		return
	}

	log.Printf("✅ [REPROVAR-INSCRICAO] Inscrição reprovada - Estudante: %s", req.CodigoEstudante)

	c.JSON(http.StatusOK, gin.H{
		"message":   "inscrição reprovada",
		"estudante": req.CodigoEstudante,
		"motivo":    req.Motivo,
	})
}

// InscreverEstudante cria uma nova inscrição
func InscreverEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("📝 [INSCREVER-ESTUDANTE] Iniciando inscrição - UserID: %s", userID)

	var req struct {
		CodigoAcademia string  `json:"codigo_academia" binding:"required"`
		Tipo           string  `json:"tipo" binding:"required"`
		AnoInscricao   string  `json:"ano_inscricao" binding:"required"`
		Curso          *string `json:"curso"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("❌ [INSCREVER-ESTUDANTE-DEBUG] Erro validação JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("🔍 [INSCREVER-ESTUDANTE-DEBUG] Dados recebidos - Academia: %s, Tipo: %s", 
		req.CodigoAcademia, req.Tipo)

	// Buscar academia
	academiaProj := getAcademiaProjection(c)
	log.Printf("🔍 [INSCREVER-ESTUDANTE-DEBUG] Buscando academia código: %s", req.CodigoAcademia)
	academiaDTO, err := academiaProj.GetByCodigo(req.CodigoAcademia)
	if err != nil || academiaDTO == nil {
		log.Printf("❌ [INSCREVER-ESTUDANTE-DEBUG] Academia não encontrada: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}
	log.Printf("✅ [INSCREVER-ESTUDANTE-DEBUG] Academia encontrada: %s (ID: %s, Status: %s)", 
		academiaDTO.Nome, academiaDTO.ID, academiaDTO.Status)

	if academiaDTO.Status != "ativo" {
		log.Printf("❌ [INSCREVER-ESTUDANTE-DEBUG] Academia não ativa - Status: %s", academiaDTO.Status)
		c.JSON(http.StatusBadRequest, gin.H{"error": "academia não está ativa"})
		return
	}

	// Carregar agregado estudante
	repository := getRepository(c)
	log.Printf("📦 [INSCREVER-ESTUDANTE-DEBUG] Carregando agregado estudante ID: %s", userID)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		log.Printf("❌ [INSCREVER-ESTUDANTE-DEBUG] Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar estudante"})
		return
	}

	estudante := estudanteAgg.(*aggregates.Estudante)
	log.Printf("✅ [INSCREVER-ESTUDANTE-DEBUG] Agregado carregado com sucesso")
	
	// Solicitar inscrição
	log.Printf("💾 [INSCREVER-ESTUDANTE-DEBUG] Solicitando inscrição no agregado...")
	err = estudante.SolicitarInscricao(
		req.CodigoAcademia,
		req.Tipo,
		req.AnoInscricao,
		req.Curso,
	)

	if err != nil {
		log.Printf("❌ [INSCREVER-ESTUDANTE-DEBUG] Erro ao solicitar inscrição: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	log.Printf("💾 [INSCREVER-ESTUDANTE-DEBUG] Salvando eventos - Total: %d", len(estudante.UncommittedEvents))
	if err := repository.Save(estudante); err != nil {
		log.Printf("❌ [INSCREVER-ESTUDANTE-DEBUG] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao salvar inscrição"})
		return
	}

	log.Printf("✅ [INSCREVER-ESTUDANTE] Inscrição criada com sucesso - Academia: %s", academiaDTO.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message":  "inscrição criada com sucesso",
		"status":   "espera",
		"academia": academiaDTO.Nome,
	})
}