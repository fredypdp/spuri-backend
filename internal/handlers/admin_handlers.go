package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// LoginAdmin autentica administrador
func LoginAdmin(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
		Senha string `json:"senha" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(req.Email)
	if err != nil || admin == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
		return
	}

	if admin.Status != "ativo" {
		c.JSON(http.StatusForbidden, gin.H{"error": "administrador inativo"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.SenhaHash), []byte(req.Senha)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
		return
	}

	token, err := middleware.GenerateToken(admin.ID, "admin")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao gerar token"})
		return
	}

	log.Printf("Login admin bem-sucedido: %s (%s)", admin.Nome, admin.Role)

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"nome":  admin.Nome,
		"role":  admin.Role,
		"type":  "admin",
	})
}

// RegisterAdmin cria novo administrador
func RegisterAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome  string `json:"nome" binding:"required"`
		Email string `json:"email" binding:"required"`
		Senha string `json:"senha" binding:"required"`
		Role  string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	if req.Role != "fpp" && req.Role != "adm" && req.Role != "gerente" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role deve ser 'fpp', 'adm' ou 'gerente'"})
		return
	}

	adminProj := getAdminProjection(c)
	creatorAdmin, err := adminProj.GetByID(userID)
	if err != nil || creatorAdmin == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "administrador não encontrado"})
		return
	}

	repository := getRepository(c)
	creatorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar administrador"})
		return
	}

	creator := creatorAgg.(*aggregates.Admin)
	if err := creator.ValidatePermission(req.Role); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	existing, _ := adminProj.GetByEmail(req.Email)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email já cadastrado"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	newAdmin := aggregates.NewAdmin()
	if err := newAdmin.Criar(req.Nome, req.Email, string(hashedPassword), req.Role, &userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(newAdmin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao criar administrador"})
		return
	}

	creator.RegistrarAcao("admin_criado", map[string]interface{}{
		"novo_admin_id": newAdmin.ID.String(),
		"role":          req.Role,
		"email":         req.Email,
	})
	repository.Save(creator)

	log.Printf("Admin criado: %s (%s) por %s", req.Email, req.Role, creatorAdmin.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message": "administrador criado com sucesso",
		"data": gin.H{
			"id":    newAdmin.ID,
			"nome":  newAdmin.Nome,
			"email": req.Email,
			"role":  newAdmin.Role,
		},
	})
}

// ConsultarAdmin busca admin por email
func ConsultarAdmin(c *gin.Context) {
	email := c.Param("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email não fornecido"})
		return
	}

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar admin"})
		return
	}

	if admin == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "admin não encontrado"})
		return
	}

	c.JSON(http.StatusOK, admin)
}

// ListarEstudantes lista estudantes baseado no tipo de usuário
func ListarEstudantes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	client := getDbClient(c)
	_ = getEstudanteProjection(c)

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar dados da academia"})
			return
		}

		safeCodigoAcademia := db.SafeString(academiaDTO.CodigoAcademia)
		query := fmt.Sprintf(`
			SELECT id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
				bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
				status, status_escolar, status_superior, ano_escolar, ano_superior,
				curso_medio, curso_superior, created_at, updated_at, total_notas,
				total_faltas, total_inscricoes, version
			FROM projection_estudantes
			WHERE codigo_academia = '%s'
			ORDER BY created_at DESC
		`, safeCodigoAcademia)

		rows, err := client.DB().Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar estudantes"})
			return
		}
		defer rows.Close()

		var estudantes []map[string]interface{}
		for rows.Next() {
			var est projections.EstudanteDTO
			if err := rows.Scan(&est.ID, &est.Nome, &est.CodigoEstudante, &est.SenhaHash,
				&est.Email, &est.Telefone, &est.EmailVerificado,
				&est.BilheteIdentidade, &est.BilheteIdentidadeResp, &est.CodigoAcademia,
				&est.Status, &est.StatusEscolar, &est.StatusSuperior,
				&est.AnoEscolar, &est.AnoSuperior, &est.CursoMedio, &est.CursoSuperior,
				&est.CreatedAt, &est.UpdatedAt, &est.TotalNotas, &est.TotalFaltas,
				&est.TotalInscricoes, &est.Version); err == nil {
				
				estudanteMap := map[string]interface{}{
					"id":                             est.ID,
					"nome":                           est.Nome,
					"codigo_estudante":               est.CodigoEstudante,
					"email":                          est.Email,
					"telefone":                       est.Telefone,
					"email_verificado":               est.EmailVerificado,
					"bilhete_identidade":             est.BilheteIdentidade,
					"bilhete_identidade_responsavel": est.BilheteIdentidadeResp,
					"codigo_academia":                est.CodigoAcademia,
					"status":                         est.Status,
					"status_escolar":                 est.StatusEscolar,
					"status_superior":                est.StatusSuperior,
					"ano_escolar":                    est.AnoEscolar,
					"ano_superior":                   est.AnoSuperior,
					"curso_medio":                    est.CursoMedio,
					"curso_superior":                 est.CursoSuperior,
					"created_at":                     est.CreatedAt,
					"updated_at":                     est.UpdatedAt,
					"total_notas":                    est.TotalNotas,
					"total_faltas":                   est.TotalFaltas,
					"total_inscricoes":               est.TotalInscricoes,
					"version":                        est.Version,
				}
				estudantes = append(estudantes, estudanteMap)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"estudantes":      estudantes,
			"total":           len(estudantes),
			"tipo_usuario":    "academia",
			"codigo_academia": academiaDTO.CodigoAcademia,
			"nome_academia":   academiaDTO.Nome,
		})

	} else if userType == "admin" {
		query := `
			SELECT id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
				bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
				status, status_escolar, status_superior, ano_escolar, ano_superior,
				curso_medio, curso_superior, created_at, updated_at, total_notas,
				total_faltas, total_inscricoes, version
			FROM projection_estudantes
			ORDER BY created_at DESC
		`

		rows, err := client.DB().Query(query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar estudantes"})
			return
		}
		defer rows.Close()

		var estudantes []map[string]interface{}
		for rows.Next() {
			var est projections.EstudanteDTO
			if err := rows.Scan(&est.ID, &est.Nome, &est.CodigoEstudante, &est.SenhaHash,
				&est.Email, &est.Telefone, &est.EmailVerificado,
				&est.BilheteIdentidade, &est.BilheteIdentidadeResp, &est.CodigoAcademia,
				&est.Status, &est.StatusEscolar, &est.StatusSuperior,
				&est.AnoEscolar, &est.AnoSuperior, &est.CursoMedio, &est.CursoSuperior,
				&est.CreatedAt, &est.UpdatedAt, &est.TotalNotas, &est.TotalFaltas,
				&est.TotalInscricoes, &est.Version); err == nil {
				
				estudanteMap := map[string]interface{}{
					"id":                             est.ID,
					"nome":                           est.Nome,
					"codigo_estudante":               est.CodigoEstudante,
					"email":                          est.Email,
					"telefone":                       est.Telefone,
					"email_verificado":               est.EmailVerificado,
					"bilhete_identidade":             est.BilheteIdentidade,
					"bilhete_identidade_responsavel": est.BilheteIdentidadeResp,
					"codigo_academia":                est.CodigoAcademia,
					"status":                         est.Status,
					"status_escolar":                 est.StatusEscolar,
					"status_superior":                est.StatusSuperior,
					"ano_escolar":                    est.AnoEscolar,
					"ano_superior":                   est.AnoSuperior,
					"curso_medio":                    est.CursoMedio,
					"curso_superior":                 est.CursoSuperior,
					"created_at":                     est.CreatedAt,
					"updated_at":                     est.UpdatedAt,
					"total_notas":                    est.TotalNotas,
					"total_faltas":                   est.TotalFaltas,
					"total_inscricoes":               est.TotalInscricoes,
					"version":                        est.Version,
				}
				estudantes = append(estudantes, estudanteMap)
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"estudantes":   estudantes,
			"total":        len(estudantes),
			"tipo_usuario": "admin",
		})

	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "acesso negado: apenas academias e administradores"})
	}
}

// AtivarAcademia ativa academia (gerente, adm ou fpp)
func AtivarAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	codigoAcademia := c.Param("codigo")

	if err := verificarPermissaoAdmin(c, "gerente"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.Ativar(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao ativar academia"})
		return
	}

	registrarAcaoAdmin(c, userID, "academia_ativada", map[string]interface{}{
		"codigo_academia": codigoAcademia,
		"academia_id":     academiaDTO.ID.String(),
	})

	log.Printf("Academia ativada: %s", codigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":         "academia ativada com sucesso",
		"codigo_academia": academia.CodigoAcademia,
		"nome":            academia.Nome,
	})
}

// DesativarAcademia desativa academia (gerente, adm ou fpp)
func DesativarAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	codigoAcademia := c.Param("codigo")

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "motivo é obrigatório"})
		return
	}

	if err := verificarPermissaoAdmin(c, "gerente"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academiaDTO == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.Desativar(req.Motivo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao desativar academia"})
		return
	}

	registrarAcaoAdmin(c, userID, "academia_desativada", map[string]interface{}{
		"codigo_academia": codigoAcademia,
		"academia_id":     academiaDTO.ID.String(),
		"motivo":          req.Motivo,
	})

	log.Printf("Academia desativada: %s - Motivo: %s", codigoAcademia, req.Motivo)

	c.JSON(http.StatusOK, gin.H{
		"message":         "academia desativada com sucesso",
		"codigo_academia": academia.CodigoAcademia,
		"nome":            academia.Nome,
		"motivo":          req.Motivo,
	})
}

// RebuildProjection reconstrói projeção específica
func RebuildProjection(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	
	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByID(userID)
	if err != nil || admin == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "apenas administradores"})
		return
	}

	if admin.Role != "fpp" && admin.Role != "adm" {
		c.JSON(http.StatusForbidden, gin.H{"error": "apenas FPP ou ADM podem reconstruir projeções"})
		return
	}

	projectionName := c.Param("name")
	if projectionName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nome da projeção não fornecido"})
		return
	}

	client := getDbClient(c)
	manager := projections.NewManager(client)

	manager.RegisterProjection("estudantes", projections.NewEstudanteProjection(client))
	manager.RegisterProjection("academias", projections.NewAcademiaProjection(client))
	manager.RegisterProjection("admins", projections.NewAdminProjection(client))
	manager.RegisterProjection("notas", projections.NewNotasProjection(client))
	manager.RegisterProjection("faltas", projections.NewFaltasProjection(client))
	manager.RegisterProjection("inscricoes", projections.NewInscricoesProjection(client))
	manager.RegisterProjection("cursos", projections.NewCursosProjection(client))
	manager.RegisterProjection("materias", projections.NewMateriasProjection(client))

	var err2 error
	if projectionName == "all" {
		err2 = manager.RebuildAllProjections()
	} else {
		err2 = manager.RebuildProjection(projectionName)
	}

	if err2 != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "erro ao reconstruir projeção",
			"details": err2.Error(),
		})
		return
	}

	log.Printf("Projeção %s reconstruída por %s", projectionName, admin.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message":    "projeção reconstruída com sucesso",
		"projection": projectionName,
	})
}

// GetProjectionStatus retorna status de projeção
func GetProjectionStatus(c *gin.Context) {
	projectionName := c.Param("name")
	if projectionName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nome da projeção não fornecido"})
		return
	}

	client := getDbClient(c)
	
	var proj projections.Projection
	switch projectionName {
	case "estudantes":
		proj = projections.NewEstudanteProjection(client)
	case "academias":
		proj = projections.NewAcademiaProjection(client)
	case "admins":
		proj = projections.NewAdminProjection(client)
	case "notas":
		proj = projections.NewNotasProjection(client)
	case "faltas":
		proj = projections.NewFaltasProjection(client)
	case "inscricoes":
		proj = projections.NewInscricoesProjection(client)
	case "cursos":
		proj = projections.NewCursosProjection(client)
	case "materias":
		proj = projections.NewMateriasProjection(client)
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "projeção não encontrada"})
		return
	}

	lastEventID, err := proj.GetLastProcessedEventID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao obter status"})
		return
	}

	safeName := db.SafeString(projectionName)
	query := fmt.Sprintf(`
		SELECT last_processed_at, events_processed 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)

	var lastProcessedAt *string
	var eventsProcessed int
	client.DB().QueryRow(query).Scan(&lastProcessedAt, &eventsProcessed)

	status := gin.H{
		"projection_name":        projectionName,
		"last_processed_event":   lastEventID,
		"events_processed_total": eventsProcessed,
	}

	if lastProcessedAt != nil {
		status["last_processed_at"] = *lastProcessedAt
	}

	c.JSON(http.StatusOK, status)
}

// GetAllProjectionStatuses retorna status de todas projeções
func GetAllProjectionsStatus(c *gin.Context) {
	client := getDbClient(c)

	query := `
		SELECT projection_name, last_processed_event_id, last_processed_at, events_processed
		FROM projection_checkpoints
		ORDER BY projection_name
	`

	rows, err := client.DB().Query(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar status"})
		return
	}
	defer rows.Close()

	var statuses []gin.H
	for rows.Next() {
		var name string
		var lastEventID int64
		var lastProcessedAt string
		var eventsProcessed int

		if err := rows.Scan(&name, &lastEventID, &lastProcessedAt, &eventsProcessed); err == nil {
			statuses = append(statuses, gin.H{
				"projection_name":      name,
				"last_processed_event": lastEventID,
				"last_processed_at":    lastProcessedAt,
				"events_processed":     eventsProcessed,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"projections": statuses,
		"total":       len(statuses),
	})
}

// GetLedgerStats retorna estatísticas do ledger
func GetLedgerStats(c *gin.Context) {
	client := getDbClient(c)

	var totalEvents int64
	var firstEvent, lastEvent string

	client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger`).Scan(&totalEvents)
	client.DB().QueryRow(`SELECT occurred_at FROM spuri_ledger ORDER BY id ASC LIMIT 1`).Scan(&firstEvent)
	client.DB().QueryRow(`SELECT occurred_at FROM spuri_ledger ORDER BY id DESC LIMIT 1`).Scan(&lastEvent)

	aggregateQuery := `
		SELECT aggregate_type, COUNT(*) as count 
		FROM spuri_ledger 
		GROUP BY aggregate_type 
		ORDER BY count DESC
	`
	rows, _ := client.DB().Query(aggregateQuery)
	defer rows.Close()

	aggregateStats := make(map[string]int64)
	for rows.Next() {
		var aggType string
		var count int64
		if err := rows.Scan(&aggType, &count); err == nil {
			aggregateStats[aggType] = count
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_events":        totalEvents,
		"first_event_at":      firstEvent,
		"last_event_at":       lastEvent,
		"events_by_aggregate": aggregateStats,
	})
}

// VerifyAllIntegrity verifica integridade do ledger
func VerifyAllIntegrity(c *gin.Context) {
	client := getDbClient(c)

	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE ledger_hash IS NOT NULL) as with_hash,
			COUNT(*) FILTER (WHERE previous_hash IS NOT NULL) as with_previous
		FROM spuri_ledger
	`

	var total, withHash, withPrevious int64
	err := client.DB().QueryRow(query).Scan(&total, &withHash, &withPrevious)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar integridade"})
		return
	}

	integro := (total == withHash) && (total-1 == withPrevious)

	c.JSON(http.StatusOK, gin.H{
		"integro":              integro,
		"total_events":         total,
		"events_with_hash":     withHash,
		"events_with_previous": withPrevious,
		"message": func() string {
			if integro {
				return "✅ Ledger íntegro - todos os eventos possuem hash"
			}
			return "⚠️ Problemas de integridade detectados"
		}(),
	})
}

// HELPERS

func getAdminProjection(c *gin.Context) *projections.AdminProjection {
	return projections.NewAdminProjection(getDbClient(c))
}

func verificarPermissaoAdmin(c *gin.Context, minRole string) error {
	userID, _ := middleware.GetUserID(c)

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByID(userID)
	if err != nil || admin == nil {
		return fmt.Errorf("administrador não encontrado")
	}

	if admin.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	hierarchy := map[string]int{
		"fpp":     3,
		"adm":     2,
		"gerente": 1,
	}

	if hierarchy[admin.Role] < hierarchy[minRole] {
		return fmt.Errorf("permissão negada: requer role '%s' ou superior", minRole)
	}

	return nil
}

func registrarAcaoAdmin(c *gin.Context, adminID uuid.UUID, acao string, detalhes map[string]interface{}) {
	repository := getRepository(c)

	adminAgg, err := repository.Load(adminID, "Admin")
	if err != nil {
		log.Printf("Erro ao registrar ação: %v", err)
		return
	}

	admin := adminAgg.(*aggregates.Admin)
	admin.RegistrarAcao(acao, detalhes)
	repository.Save(admin)
}