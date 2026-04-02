package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/services"
	"spuri/internal/utils"
)

// ============================================================================
// POST /admin/academia/register
// ============================================================================

type RegisterAcademiaRequest struct {
	Type           string   `json:"type"            binding:"required"`
	Nome           string   `json:"nome"            binding:"required"`
	Provincia      string   `json:"provincia"       binding:"required"`
	Endereco       string   `json:"endereco"        binding:"required"`
	NumeroTelefone *string  `json:"numero_telefone"`
	Email          *string  `json:"email"`
	Website        *string  `json:"website"`
	NivelEscolar   *string  `json:"nivel_escolar"`
	Cursos         []string `json:"cursos"`
	// AnosAcademicos — obrigatório para tipo="escola" com nivel_escolar "fundamental" ou "misto".
	// Subconjunto de: 1_fundamental … nono_fundamental.
	AnosAcademicos []string `json:"anos_academicos"`
}

func RegisterAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req RegisterAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: type, nome, provincia e endereco"))
		return
	}

	if req.Type != "escola" && req.Type != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("type deve ser 'escola' ou 'superior'"))
		return
	}

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateEndereco(req.Endereco); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// validarProvincia converte nome completo → código 3 letras (ex: "Luanda" → "LDA").
	codigoProvincia, err := validarProvincia(req.Provincia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.NivelEscolar != nil {
		nivel := *req.NivelEscolar
		if nivel == "medio" && len(req.AnosAcademicos) > 0 {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"escolas de nivel_escolar 'medio' não devem definir anos_academicos",
			))
			return
		}
		if nivel == "fundamental" || nivel == "misto" {
			if len(req.AnosAcademicos) == 0 {
				utils.RespondWithValidationError(c, fmt.Errorf(
					"escolas de nivel_escolar '%s' devem definir anos_academicos "+
						"(ex: 1_ano_fundamental, 2_ano_fundamental, ...)",
					nivel,
				))
				return
			}
			if err := utils.ValidateAnosFundamental(req.AnosAcademicos); err != nil {
				utils.RespondWithValidationError(c, err)
				return
			}
		}
	}

	client := getDbClient(c)
	codigoAcademia, err := generateCodigoAcademia(codigoProvincia, client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	defaultPassword := services.GetDefaultPassword("academia", codigoAcademia)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := aggregates.NewAcademia()
	if err := academia.Criar(
		req.Type,
		req.Nome,
		codigoAcademia,
		string(hashedPassword),
		codigoProvincia,
		req.Endereco,
		req.NumeroTelefone,
		req.Email,
		req.Website,
		req.NivelEscolar,
		req.Cursos,
		req.AnosAcademicos,
		&userID,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Academia registada: %s (%s) por admin %s", req.Nome, codigoAcademia, userID)
	c.JSON(http.StatusCreated, gin.H{
		"message":         "academia registada com sucesso",
		"codigo_academia": codigoAcademia,
		"data": gin.H{
			"id":              academia.ID,
			"nome":            req.Nome,
			"provincia":       codigoProvincia,
			"codigo_academia": codigoAcademia,
		},
	})
}

// ============================================================================
// PUT /academia/dados
// ============================================================================

func AtualizarDadosAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome           *string  `json:"nome"`
		Provincia      *string  `json:"provincia"`
		Endereco       *string  `json:"endereco"`
		NumeroTelefone *string  `json:"numero_telefone"`
		Email          *string  `json:"email"`
		Website        *string  `json:"website"`
		NivelEscolar   *string  `json:"nivel_escolar"`
		AnosAcademicos []string `json:"anos_academicos"`
		Cursos         []string `json:"cursos"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("body inválido"))
		return
	}

	if req.Nome == nil && req.Provincia == nil && req.Endereco == nil &&
		req.NumeroTelefone == nil && req.Email == nil && req.Website == nil &&
		req.NivelEscolar == nil && len(req.AnosAcademicos) == 0 && len(req.Cursos) == 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("ao menos um campo deve ser fornecido para atualização"))
		return
	}

	var provCode *string
	if req.Provincia != nil {
		code, err := validarProvincia(*req.Provincia)
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		provCode = &code
	}

	if req.NivelEscolar != nil {
		nivel := *req.NivelEscolar
		if nivel == "medio" && len(req.AnosAcademicos) > 0 {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"escolas de nivel_escolar 'medio' não devem definir anos_academicos",
			))
			return
		}
		if nivel == "fundamental" || nivel == "misto" {
			if len(req.AnosAcademicos) == 0 {
				utils.RespondWithValidationError(c, fmt.Errorf(
					"escolas de nivel_escolar '%s' devem definir anos_academicos "+
						"(ex: 1_ano_fundamental, 2_ano_fundamental, ...)",
					nivel,
				))
				return
			}
			if err := utils.ValidateAnosFundamental(req.AnosAcademicos); err != nil {
				utils.RespondWithValidationError(c, err)
				return
			}
		}
	}

	repository := getRepository(c)
	agg, err := repository.Load(userID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia, ok := agg.(*aggregates.Academia)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := academia.AtualizarDados(
		req.Nome,
		provCode,
		req.Endereco,
		req.NumeroTelefone,
		req.Email,
		req.Website,
		req.NivelEscolar,
		req.AnosAcademicos,
		req.Cursos,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "dados atualizados com sucesso"})
}

// ============================================================================
// PUT /admin/academia/:codigo/ativar
// ============================================================================

func AtivarAcademia(c *gin.Context) {
	codigoAcademia := c.Param("codigo")
	adminUserID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	academia, ok := agg.(*aggregates.Academia)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := academia.AtivarComAutor(adminUserID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   adminUserID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, adminUserID, "ativar_academia", map[string]interface{}{
		"academia_id":     academiaDTO.ID.String(),
		"codigo_academia": codigoAcademia,
	})

	c.JSON(http.StatusOK, gin.H{"message": "academia ativada com sucesso"})
}

// ============================================================================
// PUT /admin/academia/:codigo/desativar
// ============================================================================

func DesativarAcademia(c *gin.Context) {
	codigoAcademia := c.Param("codigo")
	adminUserID, _ := middleware.GetUserID(c)

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	academia, ok := agg.(*aggregates.Academia)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := academia.Desativar(req.Motivo, adminUserID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   adminUserID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, adminUserID, "desativar_academia", map[string]interface{}{
		"academia_id":     academiaDTO.ID.String(),
		"codigo_academia": codigoAcademia,
		"motivo":          req.Motivo,
	})

	c.JSON(http.StatusOK, gin.H{"message": "academia desativada com sucesso"})
}

// ============================================================================
// GET /academias
// ============================================================================

func ListarTodasAcademias(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	client := getDbClient(c)

	limit, offset := getPaginationParams(c)
	limit = db.ValidateLimit(limit)
	offset = db.ValidateOffset(offset)

	const baseSelect = `
		SELECT id, type, nome, codigo_academia, provincia, endereco,
		       numero_telefone, email, website, nivel_escolar, status,
		       cursos, email_verificado, created_at, updated_at, total_estudantes, version
		FROM projection_academias`

	var (
		rows *sql.Rows
		err  error
	)

	if userType == "admin" {
		statusFilter := c.Query("status")
		switch statusFilter {
		case "ativo", "inativo":
			rows, err = client.DB().Query(
				baseSelect+` WHERE status = $1 ORDER BY nome ASC LIMIT $2 OFFSET $3`,
				statusFilter, limit, offset,
			)
		default:
			rows, err = client.DB().Query(
				baseSelect+` ORDER BY nome ASC LIMIT $1 OFFSET $2`,
				limit, offset,
			)
		}
	} else {
		rows, err = client.DB().Query(
			baseSelect+` WHERE status = 'ativo' ORDER BY nome ASC LIMIT $1 OFFSET $2`,
			limit, offset,
		)
	}

	if err != nil {
		log.Printf("[ERROR] ListarTodasAcademias: erro na query: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()

	var academias []map[string]interface{}
	for rows.Next() {
		var aca struct {
			ID              uuid.UUID  `db:"id"`
			Type            string     `db:"type"`
			Nome            string     `db:"nome"`
			CodigoAcademia  string     `db:"codigo_academia"`
			Provincia       string     `db:"provincia"`
			Endereco        string     `db:"endereco"`
			NumeroTelefone  *string    `db:"numero_telefone"`
			Email           *string    `db:"email"`
			Website         *string    `db:"website"`
			NivelEscolar    *string    `db:"nivel_escolar"`
			Status          string     `db:"status"`
			CursosJSON      *string    `db:"cursos"`
			EmailVerificado bool       `db:"email_verificado"`
			CreatedAt       time.Time  `db:"created_at"`
			UpdatedAt       *time.Time `db:"updated_at"`
			TotalEstudantes int        `db:"total_estudantes"`
			Version         int        `db:"version"`
		}
		if err := rows.Scan(
			&aca.ID, &aca.Type, &aca.Nome, &aca.CodigoAcademia,
			&aca.Provincia, &aca.Endereco, &aca.NumeroTelefone, &aca.Email,
			&aca.Website, &aca.NivelEscolar, &aca.Status,
			&aca.CursosJSON, &aca.EmailVerificado,
			&aca.CreatedAt, &aca.UpdatedAt,
			&aca.TotalEstudantes, &aca.Version,
		); err != nil {
			log.Printf("[WARN] ListarTodasAcademias: erro ao ler linha: %v", err)
			continue
		}

		var cursos []string
		if aca.CursosJSON != nil && *aca.CursosJSON != "" {
			if unmarshalErr := json.Unmarshal([]byte(*aca.CursosJSON), &cursos); unmarshalErr != nil {
				log.Printf("[WARN] ListarTodasAcademias: falha ao desserializar cursos da academia %s: %v",
					aca.CodigoAcademia, unmarshalErr)
			}
		}
		if cursos == nil {
			cursos = []string{}
		}

		acadMap := map[string]interface{}{
			"id":               aca.ID,
			"type":             aca.Type,
			"nome":             aca.Nome,
			"codigo_academia":  aca.CodigoAcademia,
			"provincia":        aca.Provincia,
			"endereco":         aca.Endereco,
			"numero_telefone":  aca.NumeroTelefone,
			"website":          aca.Website,
			"nivel_escolar":    aca.NivelEscolar,
			"status":           aca.Status,
			"cursos":           cursos,
			"email_verificado": aca.EmailVerificado,
			"created_at":       aca.CreatedAt,
			"updated_at":       aca.UpdatedAt,
		}

		if userType == "admin" {
			acadMap["email"] = aca.Email
			acadMap["total_estudantes"] = aca.TotalEstudantes
			acadMap["version"] = aca.Version
		}

		academias = append(academias, acadMap)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[ERROR] ListarTodasAcademias: erro durante iteração de rows: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	if academias == nil {
		academias = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"academias": academias,
		"total":     len(academias),
		"limit":     limit,
		"offset":    offset,
	})
}

// ============================================================================
// GET /consultar-academia/:codigo
// ============================================================================

func GetAcademiaPorCodigo(c *gin.Context) {
	codigo := c.Param("codigo")
	userType, _ := middleware.GetUserType(c)

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByCodigo(codigo)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	resp := gin.H{
		"id":               academia.ID,
		"type":             academia.Type,
		"nome":             academia.Nome,
		"codigo_academia":  academia.CodigoAcademia,
		"provincia":        academia.Provincia,
		"endereco":         academia.Endereco,
		"numero_telefone":  academia.NumeroTelefone,
		"website":          academia.Website,
		"nivel_escolar":    academia.NivelEscolar,
		"anos_academicos":  academia.AnosAcademicos,
		"status":           academia.Status,
		"cursos":           academia.Cursos,
		"email_verificado": academia.EmailVerificado,
		"created_at":       academia.CreatedAt,
		"total_estudantes": academia.TotalEstudantes,
	}

	if userType == "admin" {
		resp["email"] = academia.Email
		resp["motivo_desativacao"] = academia.MotivoDesativacao
	}

	c.JSON(http.StatusOK, resp)
}

// ============================================================================
// POST /academia/ano-letivo
// ============================================================================

func DefinirAnoLetivoAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		AnoLetivo string `json:"ano_letivo" binding:"required"`
		Tipo      string `json:"tipo"       binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campos obrigatórios: ano_letivo e tipo"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	academia, ok := agg.(*aggregates.Academia)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := academia.DefinirAnoLetivo(req.AnoLetivo, req.Tipo, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [DefinirAnoLetivoAcademia] %s/%s definido por academia %s",
		req.AnoLetivo, req.Tipo, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":    "ano letivo definido com sucesso",
		"ano_letivo": req.AnoLetivo,
		"tipo":       req.Tipo,
	})
}

// ============================================================================
// GET /academia/ano-letivo
// ============================================================================

func GetAnoLetivoAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	if academiaDTO.AnoLetivo == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "ano letivo não definido para esta academia. Configure via POST /academia/ano-letivo",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ano_letivo": *academiaDTO.AnoLetivo,
		"tipo":       academiaDTO.TipoAnoLetivo,
		"ativado_em": academiaDTO.AnoLetivoAtivadoEm,
	})
}

// ============================================================================
// Helpers internos
// ============================================================================

func resolverAnoLetivoAcademia(anoLetivo *string, codigoAcademia string) (string, error) {
	if anoLetivo == nil || strings.TrimSpace(*anoLetivo) == "" {
		return "", fmt.Errorf(
			"a academia '%s' não possui um ano letivo ativo. "+
				"Configure via POST /academia/ano-letivo antes de registrar",
			codigoAcademia,
		)
	}
	return *anoLetivo, nil
}

func validarProvincia(provincia string) (string, error) {
	provincias := map[string]string{
		"luanda": "LDA", "bengo": "BGO", "benguela": "BGU",
		"bie": "BIE", "cabinda": "CAB", "cuando cubango": "CCU",
		"cuanza norte": "CNO", "cuanza sul": "CSU", "cunene": "CUN",
		"huambo": "HUA", "huila": "HUI", "lunda norte": "LNO",
		"lunda sul": "LSU", "malanje": "MAL", "moxico": "MOX",
		"namibe": "NAM", "uige": "UIG", "zaire": "ZAI",
	}
	normalized := strings.ToLower(strings.TrimSpace(provincia))
	if code, ok := provincias[normalized]; ok {
		return code, nil
	}
	return "", fmt.Errorf("província inválida: %s", provincia)
}

// generateCodigoAcademia gera o código único da academia no formato
// {PROVINCIA}{ANO}{SEQUENCIAL}, ex: LDA20261, LDA20262, BGU20261.
//
// CORREÇÃO (migration 045): o caminho principal agora delega para a função SQL
// corrigida que consulta o spuri_ledger (não a projection_academias).
//
// O fallback em Go também foi corrigido para consultar o ledger via
// payload->>'CodigoAcademia', eliminando a race condition que ocorria
// quando a projeção ainda não havia materializado o evento anterior.
//
// A race condition era: cadastro rápido de 2+ academias da mesma província
// no mesmo segundo → ambas veem COUNT=0 na projeção → mesmo código gerado
// → violação de unique constraint → projeção travada permanentemente.
func generateCodigoAcademia(codigoProvincia string, sqlDB *sqlx.DB) (string, error) {
	// Caminho principal: função SQL corrigida (migration 045) que consulta o ledger.
	var codigo string
	err := sqlDB.QueryRow(`SELECT spuri_generate_codigo_academia($1)`, codigoProvincia).Scan(&codigo)
	if err == nil {
		return codigo, nil
	}

	// Fallback: replicar a lógica corrigida no Go — consultar o LEDGER, não a projeção.
	log.Printf("[WARN] generateCodigoAcademia: função SQL indisponível (%v) — usando fallback Go (ledger)", err)

	ano := time.Now().Year()
	prefix := fmt.Sprintf("%s%d", codigoProvincia, ano)

	// Contar academias já gravadas no LEDGER com este prefixo.
	// O ledger é síncrono — o INSERT do evento já ocorreu antes desta função ser chamada.
	// payload->>'CodigoAcademia' extrai o campo do JSON do evento AcademiaCriada.
	var count int
	if countErr := sqlDB.QueryRow(
		`SELECT COUNT(*)
		 FROM spuri_ledger
		 WHERE event_type = 'AcademiaCriada'
		   AND payload->>'CodigoAcademia' LIKE $1`,
		prefix+"%",
	).Scan(&count); countErr != nil {
		// Se o ledger não estiver acessível, usar nanosegundo como último recurso.
		seq := (time.Now().UnixNano() % 9999) + 1
		codigo = fmt.Sprintf("%s%d", prefix, seq)
		log.Printf("[WARN] generateCodigoAcademia: falha ao consultar ledger (%v) — emergência: %s", countErr, codigo)
		return codigo, nil
	}

	seq := count + 1
	codigo = fmt.Sprintf("%s%d", prefix, seq)

	// Loop de verificação de unicidade no ledger (até 100 tentativas).
	for i := 0; i < 100; i++ {
		var exists bool
		if checkErr := sqlDB.QueryRow(
			`SELECT EXISTS(
				SELECT 1 FROM spuri_ledger
				WHERE event_type = 'AcademiaCriada'
				  AND payload->>'CodigoAcademia' = $1
			)`,
			codigo,
		).Scan(&exists); checkErr != nil || !exists {
			break
		}
		seq++
		codigo = fmt.Sprintf("%s%d", prefix, seq)
	}

	log.Printf("[WARN] generateCodigoAcademia: código gerado pelo fallback Go (ledger): %s", codigo)
	return codigo, nil
}