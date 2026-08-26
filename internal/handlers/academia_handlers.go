package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime/multipart"
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
	"spuri/internal/projections"
	"spuri/internal/services"
	"spuri/internal/storage"
	"spuri/internal/utils"
)

// ============================================================================
// POST /admin/academia/register
// ============================================================================

type RegisterAcademiaRequest struct {
	Nivel        string   `json:"nivel"           binding:"required"`
	Type         string   `json:"type"            binding:"required"`
	Nome         string   `json:"nome"            binding:"required"`
	NIF          string   `json:"nif"             binding:"required"`
	Provincia    string   `json:"provincia"       binding:"required"`
	Endereco     string   `json:"endereco"        binding:"required"`
	Telefone     *string  `json:"telefone"`
	Email        *string  `json:"email"`
	Website      *string  `json:"website"`
	NivelEscolar *string  `json:"nivel_escolar"`
	Cursos       []string `json:"cursos"`
	// AnosAcademicos — obrigatório para tipo="escola" com nivel_escolar "fundamental" ou "misto".
	// Subconjunto de: 1_fundamental … nono_fundamental.
	AnosAcademicos []string `json:"anos_academicos"`
}

func RegisterAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	adminProj := getAdminProjection(c)
	executorAdmin, err := adminProj.GetByID(userID)
	if err != nil || executorAdmin == nil {
		utils.RespondWithForbiddenError(c, "administrador executor não encontrado")
		return
	}
	if executorAdmin.Role != "fpp" {
		utils.RespondWithForbiddenError(c, "apenas admin com role 'fpp' pode cadastrar academias")
		return
	}

	req, alvara, ok := bindRegisterAcademiaRequest(c)
	if !ok {
		return
	}

	if req.Nivel != "escola" && req.Nivel != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("nivel deve ser 'escola' ou 'superior'"))
		return
	}
	req.Type = strings.TrimSpace(strings.ToLower(req.Type))
	if req.Type != "public" && req.Type != "private" {
		utils.RespondWithValidationError(c, fmt.Errorf("type deve ser 'public' ou 'private'"))
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
	if err := utils.ValidateNIF(req.NIF); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if alvara == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("alvara é obrigatório"))
		return
	}
	alvaraPDF, err := readAndValidatePDF("alvara", alvara)
	if err != nil {
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
	existing, err := getAcademiaProjection(c).GetByNIF(req.NIF)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if existing != nil {
		utils.RespondWithConflictError(c, "nif já cadastrado em outra academia")
		return
	}
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
		req.Nivel,
		req.Type,
		req.Nome,
		req.NIF,
		codigoAcademia,
		string(hashedPassword),
		codigoProvincia,
		req.Endereco,
		req.Telefone,
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
	provider := getStorageProvider(c)
	if provider == nil {
		p, err := storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
		provider = p
	}
	dir := fmt.Sprintf("%s/Documentação formal", codigoAcademia)
	if provider == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("storage indisponível"))
		return
	}
	if err := provider.EnsureDir(dir); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if _, err := provider.Upload(fmt.Sprintf("%s/alvara_%s.pdf", dir, codigoAcademia), bytes.NewReader(alvaraPDF.data), alvaraPDF.size); err != nil {
		_ = provider.Delete(dir)
		utils.RespondWithInternalError(c, fmt.Errorf("falha no upload do alvara: %w", err))
		return
	}

	if err := repository.SaveWithAudit(academia, audit); err != nil {
		_ = provider.Delete(dir)
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
			"nif":             req.NIF,
			"type":            req.Type,
			"provincia":       codigoProvincia,
			"codigo_academia": codigoAcademia,
		},
	})
}

// ============================================================================
// POST /academia/registo-publico
// ============================================================================
//
// RegisterAcademiaPublica permite que uma academia se autocadastre na
// plataforma sem autenticação prévia. É uma variação pública do fluxo
// administrativo (RegisterAcademia) — usa exatamente as mesmas validações
// e o mesmo agregado, mas SEM exigir um admin executor autenticado.
//
// Diferença deliberada em relação a RegisterAcademia: como não há um admin
// para comunicar a senha padrão à academia fora de banda, o cadastro
// público exige o campo "senha" (multipart/form-data) definido pela própria
// academia. RegisterAcademia (fluxo admin) permanece inalterado — continua
// sempre usando a senha padrão baseada no código, sem aceitar senha customizada.
//
// Segurança:
//   - academia.Criar() sempre inicia o status como "inativo" (ver
//     applyAcademiaCriada em internal/domain/aggregates/academia.go) —
//     este comportamento já é automático e não pode ser sobrescrito pelo
//     cliente, pois RegisterAcademiaRequest não possui campo de status.
//   - Apenas um admin com role "adm" ou "fpp" pode ativar a conta, através
//     das rotas já existentes PUT /dominis/academia/:codigo/ativar
//     (middleware.RequireAdm()) — nenhuma mudança necessária nessas rotas.
//   - Login com conta "inativo" já é bloqueado pelo handler Login
//     (internal/handlers/auth_handlers.go) — nenhuma mudança necessária.
func RegisterAcademiaPublica(c *gin.Context) {
	req, alvara, ok := bindRegisterAcademiaRequest(c)
	if !ok {
		return
	}

	if req.Nivel != "escola" && req.Nivel != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("nivel deve ser 'escola' ou 'superior'"))
		return
	}
	req.Type = strings.TrimSpace(strings.ToLower(req.Type))
	if req.Type != "public" && req.Type != "private" {
		utils.RespondWithValidationError(c, fmt.Errorf("type deve ser 'public' ou 'private'"))
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
	if err := utils.ValidateNIF(req.NIF); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if alvara == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("alvara é obrigatório"))
		return
	}
	alvaraPDF, err := readAndValidatePDF("alvara", alvara)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Senha obrigatória — exclusiva do cadastro público. Lida diretamente do
	// multipart/form-data já parseado por bindRegisterAcademiaRequest (c.PostForm
	// não reprocessa o body).
	senha := strings.TrimSpace(c.PostForm("senha"))
	if senha == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("senha é obrigatória"))
		return
	}
	if err := utils.ValidateSenha(senha); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

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
	existing, err := getAcademiaProjection(c).GetByNIF(req.NIF)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if existing != nil {
		utils.RespondWithConflictError(c, "nif já cadastrado em outra academia")
		return
	}
	codigoAcademia, err := generateCodigoAcademia(codigoProvincia, client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(senha), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := aggregates.NewAcademia()
	if err := academia.Criar(
		req.Nivel,
		req.Type,
		req.Nome,
		req.NIF,
		codigoAcademia,
		string(hashedPassword),
		codigoProvincia,
		req.Endereco,
		req.Telefone,
		req.Email,
		req.Website,
		req.NivelEscolar,
		req.Cursos,
		req.AnosAcademicos,
		nil,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	audit := db.AuditContext{
		UserID:   "publico",
		UserType: "publico",
		IP:       c.ClientIP(),
	}
	provider := getStorageProvider(c)
	if provider == nil {
		p, err := storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
		provider = p
	}
	dir := fmt.Sprintf("%s/Documentação formal", codigoAcademia)
	if provider == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("storage indisponível"))
		return
	}
	if err := provider.EnsureDir(dir); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if _, err := provider.Upload(fmt.Sprintf("%s/alvara_%s.pdf", dir, codigoAcademia), bytes.NewReader(alvaraPDF.data), alvaraPDF.size); err != nil {
		_ = provider.Delete(dir)
		utils.RespondWithInternalError(c, fmt.Errorf("falha no upload do alvara: %w", err))
		return
	}

	if err := repository.SaveWithAudit(academia, audit); err != nil {
		_ = provider.Delete(dir)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Academia auto-registada (cadastro público, pendente de ativação): %s (%s)", req.Nome, codigoAcademia)

	aviso := "guarde o código da academia: ele é o seu identificador de login. você definiu sua própria senha no cadastro."

	c.JSON(http.StatusCreated, gin.H{
		"message":         "cadastro recebido com sucesso. a conta fica inativa até que um administrador (role adm ou fpp) a ative.",
		"codigo_academia": codigoAcademia,
		"data": gin.H{
			"id":              academia.ID,
			"nome":            req.Nome,
			"nif":             req.NIF,
			"type":            req.Type,
			"provincia":       codigoProvincia,
			"codigo_academia": codigoAcademia,
			"status":          "inativo",
		},
		"aviso": aviso,
	})
}

// ============================================================================
// PUT /academia/dados
// ============================================================================

func AtualizarDadosAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	if rejectAcademiaDadosRestrictedFields(c) {
		return
	}

	var req struct {
		Nome      *string `json:"nome"`
		Provincia *string `json:"provincia"`
		Endereco  *string `json:"endereco"`
		Website   *string `json:"website"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("body inválido"))
		return
	}

	if req.Nome == nil && req.Provincia == nil && req.Endereco == nil && req.Website == nil {
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
		nil,
		nil,
		provCode,
		req.Endereco,
		nil,
		nil,
		req.Website,
		nil,
		nil,
		nil,
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

func DeletarAcademia(c *gin.Context) {
	codigoAcademia := c.Param("codigo")
	adminUserID, _ := middleware.GetUserID(c)

	adminProj := getAdminProjection(c)
	executorAdmin, err := adminProj.GetByID(adminUserID)
	if err != nil || executorAdmin == nil {
		utils.RespondWithForbiddenError(c, "administrador executor não encontrado")
		return
	}
	if executorAdmin.Role != "fpp" {
		utils.RespondWithForbiddenError(c, "apenas admin com role 'fpp' pode deletar academias")
		return
	}

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Motivo) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academiaDTO == nil || academiaDTO.Status == "deletado" {
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

	if err := academia.Deletar(req.Motivo, adminUserID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{UserID: adminUserID.String(), UserType: "admin", IP: c.ClientIP()}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	provider := getStorageProvider(c)
	if provider == nil {
		p, err := storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
		provider = p
	}
	if provider == nil {
		utils.RespondWithInternalError(c, fmt.Errorf("storage indisponível para deletar documentos da academia"))
		return
	}
	documentosDir := fmt.Sprintf("%s/Documentação formal", codigoAcademia)
	if err := provider.Delete(documentosDir); err != nil && !errors.Is(err, storage.ErrNotFound) {
		utils.RespondWithInternalError(c, fmt.Errorf("falha ao deletar documentos da academia: %w", err))
		return
	}

	registrarAcaoAdmin(c, adminUserID, "deletar_academia", map[string]interface{}{
		"academia_id":     academiaDTO.ID.String(),
		"codigo_academia": codigoAcademia,
		"motivo":          strings.TrimSpace(req.Motivo),
	})

	c.JSON(http.StatusOK, gin.H{"message": "academia deletada com sucesso"})
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
		SELECT pa.id, pa.nivel, pa.type, pa.nome, pa.codigo_academia, pa.provincia, pa.endereco,
		       pa.telefone, pa.telefone_verificado, pa.email, pa.website, pa.nivel_escolar, pa.anos_academicos, pa.status,
		       pa.cursos, pa.email_verificado, pa.created_at, pa.updated_at,
		       COALESCE(est_count.total_estudantes, 0) AS total_estudantes,
		       pa.version
		FROM projection_academias pa
		LEFT JOIN (
			SELECT codigo_academia, COUNT(*)::INT AS total_estudantes
			FROM projection_estudantes
			GROUP BY codigo_academia
		) est_count ON est_count.codigo_academia = pa.codigo_academia`

	var (
		rows       *sql.Rows
		err        error
		totalGeral int
	)

	statusFilter := c.Query("status")
	switch statusFilter {
	case "ativo", "inativo":
		if err = client.DB().QueryRow(`SELECT COUNT(*) FROM projection_academias WHERE status = $1`, statusFilter).Scan(&totalGeral); err != nil {
			log.Printf("[ERROR] ListarTodasAcademias: erro ao contar academias: %v", err)
			utils.RespondWithInternalError(c, err)
			return
		}
		rows, err = client.DB().Query(
			baseSelect+` WHERE pa.status = $1 ORDER BY pa.nome ASC LIMIT $2 OFFSET $3`,
			statusFilter, limit, offset,
		)
	default:
		if err = client.DB().QueryRow(`SELECT COUNT(*) FROM projection_academias`).Scan(&totalGeral); err != nil {
			log.Printf("[ERROR] ListarTodasAcademias: erro ao contar academias: %v", err)
			utils.RespondWithInternalError(c, err)
			return
		}
		rows, err = client.DB().Query(
			baseSelect+` ORDER BY pa.nome ASC LIMIT $1 OFFSET $2`,
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
			ID                 uuid.UUID  `db:"id"`
			Nivel              string     `db:"nivel"`
			Type               string     `db:"type"`
			Nome               string     `db:"nome"`
			CodigoAcademia     string     `db:"codigo_academia"`
			Provincia          string     `db:"provincia"`
			Endereco           string     `db:"endereco"`
			Telefone           *string    `db:"telefone"`
			TelefoneVerificado bool       `db:"telefone_verificado"`
			Email              *string    `db:"email"`
			Website            *string    `db:"website"`
			NivelEscolar       *string    `db:"nivel_escolar"`
			AnosJSON           []byte     `db:"anos_academicos"`
			Status             string     `db:"status"`
			CursosJSON         []byte     `db:"cursos"`
			EmailVerificado    bool       `db:"email_verificado"`
			CreatedAt          time.Time  `db:"created_at"`
			UpdatedAt          *time.Time `db:"updated_at"`
			TotalEstudantes    int        `db:"total_estudantes"`
			Version            int        `db:"version"`
		}
		if err := rows.Scan(
			&aca.ID, &aca.Nivel, &aca.Type, &aca.Nome, &aca.CodigoAcademia,
			&aca.Provincia, &aca.Endereco, &aca.Telefone, &aca.TelefoneVerificado, &aca.Email,
			&aca.Website, &aca.NivelEscolar, &aca.AnosJSON, &aca.Status,
			&aca.CursosJSON, &aca.EmailVerificado,
			&aca.CreatedAt, &aca.UpdatedAt,
			&aca.TotalEstudantes, &aca.Version,
		); err != nil {
			log.Printf("[WARN] ListarTodasAcademias: erro ao ler linha: %v", err)
			continue
		}

		var anosAcademicos []string
		if len(aca.AnosJSON) > 0 {
			if unmarshalErr := json.Unmarshal(aca.AnosJSON, &anosAcademicos); unmarshalErr != nil {
				log.Printf("[WARN] ListarTodasAcademias: falha ao desserializar anos_academicos da academia %s: %v",
					aca.CodigoAcademia, unmarshalErr)
			}
		}
		if anosAcademicos == nil {
			anosAcademicos = []string{}
		}

		var cursos []string
		if len(aca.CursosJSON) > 0 {
			if unmarshalErr := json.Unmarshal(aca.CursosJSON, &cursos); unmarshalErr != nil {
				log.Printf("[WARN] ListarTodasAcademias: falha ao desserializar cursos da academia %s: %v",
					aca.CodigoAcademia, unmarshalErr)
			}
		}
		if cursos == nil {
			cursos = []string{}
		}

		acadMap := map[string]interface{}{
			"nivel":           aca.Nivel,
			"type":            aca.Type,
			"nome":            aca.Nome,
			"codigo_academia": aca.CodigoAcademia,
			"provincia":       aca.Provincia,
			"endereco":        aca.Endereco,
			"nivel_escolar":   aca.NivelEscolar,
			"anos_academicos": anosAcademicos,
		}

		if userType != "" {
			acadMap["id"] = aca.ID
			acadMap["telefone"] = aca.Telefone
			acadMap["telefone_verificado"] = aca.TelefoneVerificado
			acadMap["website"] = aca.Website
			acadMap["status"] = aca.Status
			acadMap["cursos"] = cursos
			acadMap["email_verificado"] = aca.EmailVerificado
			acadMap["created_at"] = aca.CreatedAt
			acadMap["updated_at"] = aca.UpdatedAt
			acadMap["documentos"] = documentosComDownloadAcademia(aca.CodigoAcademia)
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
		"academias":   academias,
		"total":       len(academias),
		"total_geral": totalGeral,
		"limit":       limit,
		"offset":      offset,
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
		"nivel":           academia.Nivel,
		"type":            academia.Type,
		"nome":            academia.Nome,
		"codigo_academia": academia.CodigoAcademia,
		"provincia":       academia.Provincia,
		"endereco":        academia.Endereco,
		"nivel_escolar":   academia.NivelEscolar,
		"anos_academicos": academia.AnosAcademicos,
	}

	if userType != "" {
		resp["id"] = academia.ID
		resp["telefone"] = academia.Telefone
		resp["telefone_verificado"] = academia.TelefoneVerificado
		resp["website"] = academia.Website
		resp["status"] = academia.Status
		resp["cursos"] = academia.Cursos
		resp["email_verificado"] = academia.EmailVerificado
		resp["created_at"] = academia.CreatedAt
		resp["total_estudantes"] = academia.TotalEstudantes
		resp["ano_letivo"] = academia.AnoLetivo
		resp["tipo_ano_letivo"] = academia.TipoAnoLetivo
		resp["anos_letivos_lista"] = academia.AnosLetivosLista
		resp["documentos"] = documentosComDownloadAcademia(academia.CodigoAcademia)
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
		AnoLetivo string `json:"ano_letivo"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	tipo := tipoAnoLetivoDaAcademia(academiaDTO)
	anoLetivoGlobal, err := getAnoLetivoGlobalSistema(c, tipo)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if strings.TrimSpace(anoLetivoGlobal) == "" {
		utils.RespondWithConflictError(c, "ano letivo global do sistema ainda não foi definido pelo admin fpp")
		return
	}
	anoLetivoSolicitado := strings.TrimSpace(req.AnoLetivo)
	if anoLetivoSolicitado == "" {
		anoLetivoSolicitado = strings.TrimSpace(anoLetivoGlobal)
	}
	if anoLetivoSolicitado != strings.TrimSpace(anoLetivoGlobal) {
		utils.RespondWithValidationError(c, fmt.Errorf("o ano letivo da academia deve ser igual ao ano letivo global do sistema: %s", anoLetivoGlobal))
		return
	}
	if academiaDTO.AnoLetivo != nil && strings.TrimSpace(*academiaDTO.AnoLetivo) != "" {
		utils.RespondWithConflictError(c, "ano letivo da academia já foi definido; finalize o ano letivo atual para avançar automaticamente para o seguinte")
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

	if err := academia.DefinirAnoLetivo(anoLetivoSolicitado, tipo, userID); err != nil {
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
		anoLetivoSolicitado, tipo, academiaDTO.CodigoAcademia)

	periodo, err := periodoFixoPorTipoAnoLetivo(tipo)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":    "ano letivo definido com sucesso",
		"ano_letivo": anoLetivoSolicitado,
		"tipo":       tipo,
		"periodo":    periodo,
		"imutavel":   true,
	})
}

func getAnoLetivoGlobalSistema(c *gin.Context, tipo string) (string, error) {
	client := getDbClient(c)
	if client == nil {
		return "", fmt.Errorf("cliente de banco indisponível")
	}

	var anoLetivo sql.NullString
	err := client.DB().QueryRow(`
		SELECT ano_letivo_atual
		FROM projection_sistema_config
		WHERE chave = $1
	`, chaveAnoLetivoGlobal(tipo)).Scan(&anoLetivo)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	if !anoLetivo.Valid {
		return "", nil
	}
	return strings.TrimSpace(anoLetivo.String), nil
}

// ============================================================================
// GET /academia/ano-letivo
// ============================================================================

func GetAnoLetivoAcademia(c *gin.Context) {
	academiaDTO, err := getAcademiaAnoLetivoTarget(c)
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
// GET /academia/anos-letivos-lista
// ============================================================================

func GetAnosLetivosListaAcademia(c *gin.Context) {
	academiaDTO, err := getAcademiaAnoLetivoTarget(c)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"anos_letivos_lista": academiaDTO.AnosLetivosLista,
	})
}

// ============================================================================
// Helpers internos
// ============================================================================

func getAcademiaAnoLetivoTarget(c *gin.Context) (*projections.AcademiaDTO, error) {
	userType, _ := middleware.GetUserType(c)
	academiaProj := getAcademiaProjection(c)

	if userType == "admin" || userType == "estudante" {
		codigoAcademia := strings.TrimSpace(c.Query("codigo_academia"))
		if codigoAcademia == "" {
			return nil, fmt.Errorf("%s deve informar ?codigo_academia=CODIGO", userType)
		}
		return academiaProj.GetByCodigo(codigoAcademia)
	}

	userID, _ := middleware.GetUserID(c)
	return academiaProj.GetByID(userID)
}

func tipoAnoLetivoDaAcademia(academia *projections.AcademiaDTO) string {
	if academia != nil && academia.Nivel == "superior" {
		return "superior"
	}
	return "escolar"
}

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
		// Códigos (aceita tanto os atuais quanto o legado "LDA" para Luanda).
		"bgo": "BGO", "bgu": "BGU", "bie": "BIE", "cab": "CAB",
		"cnd": "CND", "cno": "CNO", "cus": "CUS", "cbg": "CBG", "ccu": "CBG",
		"cnn": "CNN", "cun": "CNN", "hua": "HUA", "hui": "HUI", "ibg": "IBG",
		"lua": "LDA", "lda": "LDA", "lno": "LNO", "lsu": "LSU",
		"mal": "MAL", "mox": "MOX", "mxl": "MXL", "nam": "NAM",
		"uig": "UIG", "zai": "ZAI",

		// Nomes (aceita grafias comuns/legadas).
		"bengo": "BGO", "benguela": "BGU", "bié": "BIE",
		"cabinda": "CAB", "cuanza norte": "CNO", "kwanza norte": "CNO",
		"cuanza sul": "CUS", "kwanza sul": "CUS",
		"cuando cubango": "CBG", "cubango": "CBG", "cuando": "CND",
		"cunene": "CNN", "huambo": "HUA",
		"huila": "HUI", "huíla": "HUI", "icolo e bengo": "IBG",
		"luanda": "LDA", "lunda norte": "LNO", "lunda sul": "LSU",
		"malanje": "MAL", "moxico": "MOX", "moxico leste": "MXL",
		"namibe": "NAM", "uige": "UIG", "uíge": "UIG", "zaire": "ZAI",
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
// CORREÇÕES (migrations 045 e 093): o caminho principal delega para a função
// SQL que consulta o spuri_ledger e reserva o código antes de retorná-lo.
//
// O fallback em Go também consulta o ledger via payload->>'CodigoAcademia' e
// grava em codigo_academia_reservas antes de devolver o código, fechando a
// janela de concorrência até a persistência do evento.
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
		return "", fmt.Errorf("erro ao contar códigos de academia no ledger: %w", countErr)
	}

	seq := count + 1
	codigo = fmt.Sprintf("%s%d", prefix, seq)

	// Loop de verificação e reserva (até 100 tentativas).
	for i := 0; i < 100; i++ {
		var reserved bool
		if checkErr := sqlDB.QueryRow(
			`WITH codigo_livre AS (
				SELECT NOT EXISTS (
					SELECT 1 FROM spuri_ledger
					WHERE event_type = 'AcademiaCriada'
					  AND payload->>'CodigoAcademia' = $1
					UNION ALL
					SELECT 1 FROM codigo_academia_reservas WHERE codigo_academia = $1
				) AS livre
			), reserva AS (
				INSERT INTO codigo_academia_reservas (codigo_academia)
				SELECT $1 FROM codigo_livre WHERE livre
				ON CONFLICT DO NOTHING
				RETURNING codigo_academia
			)
			SELECT EXISTS (SELECT 1 FROM reserva)`,
			codigo,
		).Scan(&reserved); checkErr != nil {
			return "", fmt.Errorf("erro ao reservar código de academia: %w", checkErr)
		} else if reserved {
			log.Printf("[WARN] generateCodigoAcademia: código gerado e reservado pelo fallback Go (ledger): %s", codigo)
			return codigo, nil
		}
		seq++
		codigo = fmt.Sprintf("%s%d", prefix, seq)
	}

	return "", fmt.Errorf("não foi possível reservar código único de academia após 100 tentativas")
}

func FinalizarAnoLetivoAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	var req struct {
		Type       string `json:"type" binding:"required"`
		AnoLetivo  string `json:"ano_letivo"`
		Observacao string `json:"observacao"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campo obrigatório: type"))
		return
	}
	tipo, err := normalizarTipoAnoLetivo(req.Type)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	if academiaDTO.AnoLetivo == nil || strings.TrimSpace(*academiaDTO.AnoLetivo) == "" {
		utils.RespondWithConflictError(c, "ano letivo da academia ainda não foi definido; defina de acordo com o ano letivo global antes de finalizar")
		return
	}
	ano := strings.TrimSpace(*academiaDTO.AnoLetivo)
	if reqAno := strings.TrimSpace(req.AnoLetivo); reqAno != "" && reqAno != ano {
		utils.RespondWithValidationError(c, fmt.Errorf("ano_letivo informado deve ser o ano letivo ativo da academia: %s", ano))
		return
	}
	if _, err := parseAnoLetivo(ano); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	seguinte, err := proximoAnoLetivo(ano)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if academiaDTO.TipoAnoLetivo != nil && strings.TrimSpace(*academiaDTO.TipoAnoLetivo) != "" {
		tipoAtual, err := normalizarTipoAnoLetivo(*academiaDTO.TipoAnoLetivo)
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		if tipoAtual != tipo {
			utils.RespondWithValidationError(c, fmt.Errorf("type deve corresponder ao tipo de ano letivo ativo da academia: %s", tipoAtual))
			return
		}
	}
	if err := validarDataAtualPermiteFinalizacaoAnoLetivo(getDbClient(c), tipo, ano, time.Now()); err != nil {
		utils.RespondWithValidationError(c, err)
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
	if err := academia.FinalizarAnoLetivo(ano, tipo, userID, req.Observacao); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := academia.DefinirAnoLetivo(seguinte, tipo, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	globalAtualizado, err := sincronizarAnoLetivoGlobalSeAcademiasAlinhadas(c, tipo, seguinte, userID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ano letivo finalizado com sucesso; academia avançada para o ano letivo seguinte", "academia_id": academiaDTO.ID, "type": tipo, "ano_letivo_finalizado": ano, "ano_letivo": seguinte, "finalizado": true, "global_atualizado": globalAtualizado})
}

func sincronizarAnoLetivoGlobalSeAcademiasAlinhadas(c *gin.Context, tipo string, anoLetivo string, userID uuid.UUID) (bool, error) {
	client := getDbClient(c)
	if client == nil {
		return false, fmt.Errorf("cliente de banco indisponível")
	}
	var total, alinhadas int
	if err := client.DB().QueryRow(`SELECT COUNT(*), COUNT(*) FILTER (WHERE ano_letivo=$1) FROM projection_academias WHERE status='ativo' AND nivel=$2`, anoLetivo, nivelAcademiaPorTipoAnoLetivo(tipo)).Scan(&total, &alinhadas); err != nil {
		return false, err
	}
	if total == 0 || total != alinhadas {
		return false, nil
	}
	atual, err := buscarAnoLetivoGlobalAtual(client, tipo)
	if err != nil {
		return false, err
	}
	if atual == anoLetivo {
		return false, nil
	}
	if err := salvarAnoLetivoGlobal(c, tipo, anoLetivo, userID); err != nil {
		return false, err
	}
	return true, nil
}

func ListarFinalizacoesAnoLetivoAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	client := getDbClient(c)
	if client == nil {
		return
	}
	rows, err := client.DB().Query(`SELECT type, ano_letivo, finalizado, finalizado_em, observacao FROM projection_anos_letivos_academia_finalizacoes WHERE academia_id=$1 ORDER BY ano_letivo DESC, type`, academiaDTO.ID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var tipo, ano string
		var fin bool
		var em time.Time
		var obs sql.NullString
		if err := rows.Scan(&tipo, &ano, &fin, &em, &obs); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		items = append(items, gin.H{"type": tipo, "ano_letivo": ano, "finalizado": fin, "finalizado_em": em, "observacao": obs.String})
	}
	c.JSON(http.StatusOK, gin.H{"finalizacoes": items})
}

func ListarFinalizacoesAnoLetivoAdmin(c *gin.Context) {
	client := getDbClient(c)
	if client == nil {
		return
	}
	tipo := strings.TrimSpace(c.Query("type"))
	ano := strings.TrimSpace(c.Query("ano_letivo"))
	args := []interface{}{}
	where := "WHERE 1=1"
	if tipo != "" {
		nt, err := normalizarTipoAnoLetivo(tipo)
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		args = append(args, nt)
		where += fmt.Sprintf(" AND f.type=$%d", len(args))
	}
	if ano != "" {
		if _, err := parseAnoLetivo(ano); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		args = append(args, ano)
		where += fmt.Sprintf(" AND f.ano_letivo=$%d", len(args))
	}
	rows, err := client.DB().Query(`SELECT f.academia_id, f.codigo_academia, f.type, f.ano_letivo, f.finalizado, f.finalizado_em, f.observacao FROM projection_anos_letivos_academia_finalizacoes f `+where+` ORDER BY f.ano_letivo DESC, f.codigo_academia`, args...)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var aid uuid.UUID
		var cod, t, a string
		var fin bool
		var em time.Time
		var obs sql.NullString
		if err := rows.Scan(&aid, &cod, &t, &a, &fin, &em, &obs); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		items = append(items, gin.H{"academia_id": aid, "codigo_academia": cod, "type": t, "ano_letivo": a, "finalizado": fin, "finalizado_em": em, "observacao": obs.String})
	}
	c.JSON(http.StatusOK, gin.H{"finalizacoes": items})
}

func calcularLimiteFinalizacao(client *db.Client, tipo, anoFiltro string) (string, string, int, int, error) {
	var total int
	nivel := "escola"
	if tipo == "superior" {
		nivel = "superior"
	}
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_academias WHERE status='ativo' AND nivel=$1`, nivel).Scan(&total); err != nil {
		return "", "", 0, 0, err
	}
	if total == 0 {
		return "", "", 0, 0, nil
	}
	rows, err := client.DB().Query(`SELECT ano_letivo, COUNT(DISTINCT academia_id) FROM projection_anos_letivos_academia_finalizacoes WHERE type=$1 AND finalizado=TRUE GROUP BY ano_letivo`, tipo)
	if err != nil {
		return "", "", 0, 0, err
	}
	defer rows.Close()
	marco := ""
	fin := 0
	for rows.Next() {
		var ano string
		var c int
		if err := rows.Scan(&ano, &c); err != nil {
			return "", "", 0, 0, err
		}
		if c >= total {
			if marco == "" {
				marco = ano
				fin = c
			} else if cmp, _ := compareAnoLetivo(ano, marco); cmp > 0 {
				marco = ano
				fin = c
			}
		}
	}
	minimo := ""
	if marco != "" {
		minimo, _ = proximoAnoLetivoValidado(marco)
	}
	return marco, minimo, total, fin, nil
}

func bindRegisterAcademiaRequest(c *gin.Context) (RegisterAcademiaRequest, *multipart.FileHeader, bool) {
	var req RegisterAcademiaRequest
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("multipart/form-data inválido"))
			return req, nil, false
		}
		get := func(k string) string { return strings.TrimSpace(c.PostForm(k)) }
		getRaw := func(k string) string { return c.PostForm(k) }
		req = RegisterAcademiaRequest{
			Nivel: get("nivel"), Type: get("type"), Nome: get("nome"), NIF: getRaw("nif"), Provincia: get("provincia"), Endereco: get("endereco"),
			Cursos: c.PostFormArray("cursos"), AnosAcademicos: c.PostFormArray("anos_academicos"),
		}
		if v := get("telefone"); v != "" {
			req.Telefone = &v
		}
		if v := get("email"); v != "" {
			req.Email = &v
		}
		if v := get("website"); v != "" {
			req.Website = &v
		}
		if v := get("nivel_escolar"); v != "" {
			req.NivelEscolar = &v
		}
		fh, err := c.FormFile("alvara")
		if err != nil {
			return req, nil, true
		}
		return req, fh, true
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: nivel, type, nome, nif, provincia, endereco e alvara"))
		return req, nil, false
	}
	return req, nil, true
}
