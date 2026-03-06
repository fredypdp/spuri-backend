package handlers

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
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
	// Subconjunto de: primeiro_fundamental … nono_fundamental.
	AnosAcademicos []string `json:"anos_academicos"`
}

// RegisterAcademia cria uma nova academia.
// Rota protegida por AuthMiddleware + RequireAdmin + RequireAdm.
//
// FIX E-01: audit agora usa o userID do admin autenticado, não "anonimo".
// FIX E-12: ValidateNome, ValidateEndereco e validação de província já aplicados.
// FIX C12: criadoPor passado ao aggregate para rastreabilidade forense.
func RegisterAcademia(c *gin.Context) {
	// FIX E-01: extrair userID do admin antes de qualquer saída antecipada
	adminUserID, hasAdminID := middleware.GetUserID(c)

	var req RegisterAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: type, nome, provincia e endereco"))
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

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Website != nil {
		if err := utils.ValidateURL(*req.Website); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Type != "escola" && req.Type != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo deve ser 'escola' ou 'superior'"))
		return
	}

	// Validar anos_academicos conforme nivel_escolar
	if req.Type == "escola" && req.NivelEscolar != nil {
		nivel := *req.NivelEscolar
		if nivel == "fundamental" || nivel == "misto" {
			if len(req.AnosAcademicos) == 0 {
				utils.RespondWithValidationError(c, fmt.Errorf(
					"anos_academicos é obrigatório para escolas de nivel_escolar '%s'", nivel,
				))
				return
			}
			if err := utils.ValidateAnosFundamental(req.AnosAcademicos); err != nil {
				utils.RespondWithValidationError(c, err)
				return
			}
		} else if nivel == "medio" && len(req.AnosAcademicos) > 0 {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"escolas de nivel_escolar 'medio' não devem definir anos_academicos",
			))
			return
		}
	}

	codigoProvincia, err := validarProvincia(req.Provincia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	dbClient := getDbClient(c)
	codigoAcademia, err := generateCodigoAcademia(codigoProvincia, dbClient.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(codigoAcademia), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	repository := getRepository(c)
	academia := aggregates.NewAcademia()

	// FIX C12: passar criadoPor para rastreabilidade forense no payload do evento
	var criadoPor *uuid.UUID
	if hasAdminID {
		criadoPor = &adminUserID
	}

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
		criadoPor,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// FIX E-01: usar userID do admin autenticado para auditoria correta.
	// A rota está protegida por RequireAdm — sempre há um admin autenticado.
	auditUserID := "sistema"
	if hasAdminID {
		auditUserID = adminUserID.String()
	}

	audit := db.AuditContext{
		UserID:   auditUserID,
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(c.Request.Context(), academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Academia criada: %s - %s (por admin: %s)", codigoAcademia, req.Nome, auditUserID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "academia criada com sucesso",
		"data": gin.H{
			"id":              academia.ID,
			"codigo_academia": academia.CodigoAcademia,
		},
	})
}

// ============================================================================
// PUT /academia/dados
// ============================================================================

// AtualizarDadosAcademia atualiza dados da academia autenticada.
//
// FIX E-11: revalida coerência de anos_academicos com nivel_escolar no handler.
// FIX E-12: ValidateNome, ValidateEndereco e ValidateURL chamados quando presentes.
// FIX C5/C10: proteção de status também no aggregate (defesa em profundidade).
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
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}

	// FIX E-12: validações de campos quando presentes
	if req.Nome != nil {
		if err := utils.ValidateNome(*req.Nome); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Endereco != nil {
		if err := utils.ValidateEndereco(*req.Endereco); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Website != nil {
		if err := utils.ValidateURL(*req.Website); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	// FIX E-11: revalidar anos_academicos com nivel_escolar efetivo
	// A validação completa também acontece no aggregate, mas validar no handler
	// antes de carregar o aggregate reduz carga desnecessária.
	if req.NivelEscolar != nil && len(req.AnosAcademicos) > 0 {
		nivel := *req.NivelEscolar
		if nivel == "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf(
				"escolas de nivel_escolar 'medio' não devem definir anos_academicos",
			))
			return
		}
		if nivel == "fundamental" || nivel == "misto" {
			if err := utils.ValidateAnosFundamental(req.AnosAcademicos); err != nil {
				utils.RespondWithValidationError(c, err)
				return
			}
		}
	}

	repository := getRepository(c)
	agg, err := repository.Load(c.Request.Context(), userID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := agg.(*aggregates.Academia)

	if err := academia.AtualizarDados(
		req.Nome,
		req.Provincia,
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
	if err := repository.SaveWithAudit(c.Request.Context(), academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "dados atualizados com sucesso"})
}

// ============================================================================
// PUT /admin/academia/:codigo/ativar
// ============================================================================

// AtivarAcademia ativa uma academia.
// Rota protegida por AuthMiddleware + RequireAdmin + RequireGerente.
//
// FIX E-09: removida chamada redundante a verificarPermissaoAdmin — o middleware
// RequireGerente já garante a permissão. Manter ambos era ineficiente e confuso.
func AtivarAcademia(c *gin.Context) {
	codigo := c.Param("codigo")

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigo)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(c.Request.Context(), academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := agg.(*aggregates.Academia)

	if err := academia.Ativar(); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	adminUserID, _ := middleware.GetUserID(c)
	audit := db.AuditContext{
		UserID:   adminUserID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(c.Request.Context(), academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, adminUserID, "ativar_academia", map[string]interface{}{
		"codigo_academia": codigo,
	})

	c.JSON(http.StatusOK, gin.H{"message": "academia ativada com sucesso"})
}

// ============================================================================
// PUT /admin/academia/:codigo/desativar
// ============================================================================

// DesativarAcademia desativa uma academia.
// Rota protegida por AuthMiddleware + RequireAdmin + RequireGerente.
//
// FIX C9: desativadoPor agora é passado ao aggregate para inclusão no payload
// do evento AcademiaDesativada — rastreabilidade forense completa.
func DesativarAcademia(c *gin.Context) {
	codigo := c.Param("codigo")

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigo)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(c.Request.Context(), academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := agg.(*aggregates.Academia)

	// FIX C9: passar desativadoPor ao aggregate para inclusão no payload do evento
	adminUserID, _ := middleware.GetUserID(c)
	if err := academia.Desativar(req.Motivo, adminUserID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   adminUserID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(c.Request.Context(), academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, adminUserID, "desativar_academia", map[string]interface{}{
		"codigo_academia": codigo,
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

	// Academias são controladas por status (ativo/inativo), não por soft-delete.
	query := `
		SELECT id, type, nome, codigo_academia, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, status, cursos,
			email_verificado, created_at, updated_at, total_estudantes, version
		FROM projection_academias
		ORDER BY created_at DESC
	`

	rows, err := client.DB().Query(query)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()

	var academias []map[string]interface{}
	for rows.Next() {
		var aca struct {
			ID              uuid.UUID
			Type            string
			Nome            string
			CodigoAcademia  string
			Provincia       string
			Endereco        string
			NumeroTelefone  *string
			Email           *string
			Website         *string
			NivelEscolar    *string
			Status          string
			Cursos          []byte
			EmailVerificado bool
			CreatedAt       interface{}
			UpdatedAt       interface{}
			TotalEstudantes int
			Version         int
		}

		if err := rows.Scan(&aca.ID, &aca.Type, &aca.Nome, &aca.CodigoAcademia,
			&aca.Provincia, &aca.Endereco, &aca.NumeroTelefone, &aca.Email,
			&aca.Website, &aca.NivelEscolar, &aca.Status, &aca.Cursos,
			&aca.EmailVerificado, &aca.CreatedAt, &aca.UpdatedAt,
			&aca.TotalEstudantes, &aca.Version,
		); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}

		var cursos []string
		if len(aca.Cursos) > 0 {
			_ = json.Unmarshal(aca.Cursos, &cursos)
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
			"email":            aca.Email,
			"website":          aca.Website,
			"nivel_escolar":    aca.NivelEscolar,
			"status":           aca.Status,
			"cursos":           cursos,
			"email_verificado": aca.EmailVerificado,
			"created_at":       aca.CreatedAt,
			"updated_at":       aca.UpdatedAt,
			"total_estudantes": aca.TotalEstudantes,
			"version":          aca.Version,
		}

		// Ocultar email de não-admins
		if userType != "admin" {
			delete(acadMap, "email")
		}

		academias = append(academias, acadMap)
	}

	if academias == nil {
		academias = []map[string]interface{}{}
	}

	c.JSON(http.StatusOK, gin.H{
		"academias": academias,
		"total":     len(academias),
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

	// Ocultar email de não-admins
	if userType == "admin" {
		resp["email"] = academia.Email
		resp["motivo_desativacao"] = academia.MotivoDesativacao
	}

	c.JSON(http.StatusOK, resp)
}

// ============================================================================
// Helpers internos
// ============================================================================

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

func generateCodigoAcademia(codigoProvincia string, db *sqlx.DB) (string, error) {
	var codigo string
	err := db.QueryRow(`
		SELECT spuri_generate_codigo_academia($1)
	`, codigoProvincia).Scan(&codigo)
	if err != nil {
		// Fallback com hash se função SQL não estiver disponível
		h := fnv.New32a()
		h.Write([]byte(codigoProvincia + time.Now().String()))
		codigo = fmt.Sprintf("%s%08d", codigoProvincia, h.Sum32()%100000000)
	}
	return codigo, nil
}