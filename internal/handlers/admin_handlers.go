package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/services"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func LoginAdmin(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
		Senha string `json:"senha" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("email e senha são obrigatórios"))
		return
	}

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(req.Email)
	if err != nil || admin == nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	if admin.Status != "ativo" {
		utils.RespondWithForbiddenError(c, "Administrador inativo. Entre em contato com o suporte.")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(admin.SenhaHash), []byte(req.Senha)); err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	token, err := middleware.GenerateToken(admin.ID, "admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
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

func RegisterAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome  string `json:"nome" binding:"required"`
		Email string `json:"email" binding:"required"`
		Role  string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: nome, email e role"))
		return
	}

	if req.Role != "fpp" && req.Role != "adm" && req.Role != "gerente" {
		utils.RespondWithValidationError(c, fmt.Errorf("role deve ser 'fpp', 'adm' ou 'gerente'"))
		return
	}

	adminProj := getAdminProjection(c)
	creatorAdmin, err := adminProj.GetByID(userID)
	if err != nil || creatorAdmin == nil {
		utils.RespondWithForbiddenError(c, "Administrador não encontrado")
		return
	}

	repository := getRepository(c)
	creatorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	creator := creatorAgg.(*aggregates.Admin)
	if err := creator.ValidatePermission(req.Role); err != nil {
		utils.RespondWithForbiddenError(c, err.Error())
		return
	}

	existing, _ := adminProj.GetByEmail(req.Email)
	if existing != nil {
		utils.RespondWithConflictError(c, "Email já cadastrado no sistema")
		return
	}

	defaultPassword := services.GetDefaultPassword("admin", req.Role)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	newAdmin := aggregates.NewAdmin()
	if err := newAdmin.Criar(req.Nome, req.Email, string(hashedPassword), req.Role, &userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(newAdmin); err != nil {
		utils.RespondWithInternalError(c, err)
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
			"senha_padrao": defaultPassword,
		},
	})
}

func ConsultarAdmin(c *gin.Context) {
	email := c.Param("email")
	if email == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("email não fornecido"))
		return
	}

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(email)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if admin == nil {
		utils.RespondWithNotFoundError(c, "administrador")
		return
	}

	c.JSON(http.StatusOK, admin)
}

func ListarEstudantes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	client := getDbClient(c)

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		safeCodigoAcademia := db.SafeString(academiaDTO.CodigoAcademia)
		query := fmt.Sprintf(`
			SELECT id, nome, codigo_estudante, senha_hash, email, telefone, email_verificado,
				bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
				status, status_escolar, status_superior, ano_escolar, ano_superior,
				curso_medio_id, curso_superior_id, created_at, updated_at, total_notas,
				total_faltas, total_inscricoes, version
			FROM projection_estudantes
			WHERE codigo_academia = '%s'
			ORDER BY created_at DESC
		`, safeCodigoAcademia)

		rows, err := client.DB().Query(query)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		var estudantes []map[string]interface{}
		for rows.Next() {
			var id, cursoMedioID, cursoSuperiorID sql.NullString
			var nome, codigoEstudante, senhaHash, status, statusEscolar, statusSuperior string
			var email, telefone, bilhete, bilheteResp, codigoAcad, anoEscolar, anoSuperior sql.NullString
			var emailVerif bool
			var createdAt, updatedAt string
			var totalNotas, totalFaltas, totalInsc, version int

			if err := rows.Scan(&id, &nome, &codigoEstudante, &senhaHash,
				&email, &telefone, &emailVerif, &bilhete, &bilheteResp, &codigoAcad,
				&status, &statusEscolar, &statusSuperior, &anoEscolar, &anoSuperior,
				&cursoMedioID, &cursoSuperiorID,
				&createdAt, &updatedAt, &totalNotas, &totalFaltas, &totalInsc, &version); err == nil {

				estudanteMap := map[string]interface{}{
					"id":                             id.String,
					"nome":                           nome,
					"codigo_estudante":               codigoEstudante,
					"email":                          getNullString(email),
					"telefone":                       getNullString(telefone),
					"email_verificado":               emailVerif,
					"bilhete_identidade":             getNullString(bilhete),
					"bilhete_identidade_responsavel": getNullString(bilheteResp),
					"codigo_academia":                getNullString(codigoAcad),
					"status":                         status,
					"status_escolar":                 statusEscolar,
					"status_superior":                statusSuperior,
					"ano_escolar":                    getNullString(anoEscolar),
					"ano_superior":                   getNullString(anoSuperior),
					"curso_medio_id":                 getNullString(cursoMedioID),
					"curso_superior_id":              getNullString(cursoSuperiorID),
					"created_at":                     createdAt,
					"updated_at":                     updatedAt,
					"total_notas":                    totalNotas,
					"total_faltas":                   totalFaltas,
					"total_inscricoes":               totalInsc,
					"version":                        version,
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
				curso_medio_id, curso_superior_id, created_at, updated_at, total_notas,
				total_faltas, total_inscricoes, version
			FROM projection_estudantes
			ORDER BY created_at DESC
		`

		rows, err := client.DB().Query(query)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		var estudantes []map[string]interface{}
		for rows.Next() {
			var id, cursoMedioID, cursoSuperiorID sql.NullString
			var nome, codigoEstudante, senhaHash, status, statusEscolar, statusSuperior string
			var email, telefone, bilhete, bilheteResp, codigoAcad, anoEscolar, anoSuperior sql.NullString
			var emailVerif bool
			var createdAt, updatedAt string
			var totalNotas, totalFaltas, totalInsc, version int

			if err := rows.Scan(&id, &nome, &codigoEstudante, &senhaHash,
				&email, &telefone, &emailVerif, &bilhete, &bilheteResp, &codigoAcad,
				&status, &statusEscolar, &statusSuperior, &anoEscolar, &anoSuperior,
				&cursoMedioID, &cursoSuperiorID,
				&createdAt, &updatedAt, &totalNotas, &totalFaltas, &totalInsc, &version); err == nil {

				estudanteMap := map[string]interface{}{
					"id":                             id.String,
					"nome":                           nome,
					"codigo_estudante":               codigoEstudante,
					"email":                          getNullString(email),
					"telefone":                       getNullString(telefone),
					"email_verificado":               emailVerif,
					"bilhete_identidade":             getNullString(bilhete),
					"bilhete_identidade_responsavel": getNullString(bilheteResp),
					"codigo_academia":                getNullString(codigoAcad),
					"status":                         status,
					"status_escolar":                 statusEscolar,
					"status_superior":                statusSuperior,
					"ano_escolar":                    getNullString(anoEscolar),
					"ano_superior":                   getNullString(anoSuperior),
					"curso_medio_id":                 getNullString(cursoMedioID),
					"curso_superior_id":              getNullString(cursoSuperiorID),
					"created_at":                     createdAt,
					"updated_at":                     updatedAt,
					"total_notas":                    totalNotas,
					"total_faltas":                   totalFaltas,
					"total_inscricoes":               totalInsc,
					"version":                        version,
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
		utils.RespondWithForbiddenError(c, "Acesso negado. Apenas academias e administradores podem listar estudantes.")
	}
}

func getNullString(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func AtivarAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	codigoAcademia := c.Param("codigo")

	if err := verificarPermissaoAdmin(c, "gerente"); err != nil {
		utils.RespondWithForbiddenError(c, err.Error())
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.Ativar(); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(academia); err != nil {
		utils.RespondWithInternalError(c, err)
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

func DesativarAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	codigoAcademia := c.Param("codigo")

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}

	if err := verificarPermissaoAdmin(c, "gerente"); err != nil {
		utils.RespondWithForbiddenError(c, err.Error())
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.Desativar(req.Motivo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(academia); err != nil {
		utils.RespondWithInternalError(c, err)
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

func RebuildProjection(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByID(userID)
	if err != nil || admin == nil {
		utils.RespondWithForbiddenError(c, "Apenas administradores podem reconstruir projeções")
		return
	}

	if admin.Role != "fpp" && admin.Role != "adm" {
		utils.RespondWithForbiddenError(c, "Apenas FPP ou ADM podem reconstruir projeções")
		return
	}

	projectionName := c.Param("name")
	if projectionName == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nome da projeção não fornecido"))
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
		utils.RespondWithInternalError(c, err2)
		return
	}

	log.Printf("Projeção %s reconstruída por %s", projectionName, admin.Nome)

	c.JSON(http.StatusOK, gin.H{
		"message":    "projeção reconstruída com sucesso",
		"projection": projectionName,
	})
}

func GetProjectionStatus(c *gin.Context) {
	projectionName := c.Param("name")
	if projectionName == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nome da projeção não fornecido"))
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
		utils.RespondWithNotFoundError(c, "projeção")
		return
	}

	lastEventID, err := proj.GetLastProcessedEventID()
	if err != nil {
		utils.RespondWithInternalError(c, err)
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

func GetAllProjectionsStatus(c *gin.Context) {
	client := getDbClient(c)

	query := `
		SELECT projection_name, last_processed_event_id, last_processed_at, events_processed
		FROM projection_checkpoints
		ORDER BY projection_name
	`

	rows, err := client.DB().Query(query)
	if err != nil {
		utils.RespondWithInternalError(c, err)
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
		utils.RespondWithInternalError(c, err)
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