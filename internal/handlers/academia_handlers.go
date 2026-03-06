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
	userID, _ := middleware.GetUserID(c)

	var req RegisterAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados obrigatórios: type, nome, provincia e endereco"))
		return
	}

	if req.Type != "escola" && req.Type != "universidade" && req.Type != "instituto" {
		utils.RespondWithValidationError(c, fmt.Errorf("type deve ser 'escola', 'universidade' ou 'instituto'"))
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

	codigoProvincia, err := validarProvincia(req.Provincia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

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

	client := getDbClient(c)
	codigoAcademia, err := generateCodigoAcademia(codigoProvincia, client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Senha padrão = código da academia (operador que registou conhece o código).
	senhaHash, err := bcrypt.GenerateFromPassword([]byte(codigoAcademia), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academia := aggregates.NewAcademia()
	if err := academia.Criar(
		req.Type,
		req.Nome,
		codigoProvincia,
		req.Provincia,
		req.Endereco,
		req.NumeroTelefone,
		req.Email,
		req.Website,
		req.NivelEscolar,
		req.AnosAcademicos,
		req.Cursos,
		codigoAcademia,
		string(senhaHash),
		userID,
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
		"senha_inicial":   codigoAcademia,
		"data": gin.H{
			"id":              academia.ID,
			"nome":            req.Nome,
			"provincia":       req.Provincia,
			"codigo_academia": codigoAcademia,
		},
	})
}

// ============================================================================
// PUT /admin/academia/:id/ativar
// ============================================================================

// AtivarAcademia ativa uma academia por UUID.
// Rota: PUT /admin/academia/:id/ativar
// Protegida por AuthMiddleware + RequireAdmin + RequireAdm.
//
// FIX E4-AA-01: a rota registada em main.go usa o parâmetro `:id` (UUID da
// academia), mas o handler original lia `c.Param("codigo")` — valor sempre
// string vazia, academia nunca encontrada, resultado sempre 404.
// Corrigido: lemos `:id` como UUID e carregamos o aggregate diretamente por ID,
// sem lookup intermediário por código. Mais eficiente e consistente com os
// demais handlers de admin que operam por UUID (AtivarAdmin, DesativarAdmin).
func AtivarAcademia(c *gin.Context) {
	// FIX E4-AA-01: ler `:id` (UUID) — consistente com a definição da rota.
	academiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de academia inválido"))
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(academiaID, "Academia")
	if err != nil {
		utils.RespondWithNotFoundError(c, "academia")
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
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, adminUserID, "ativar_academia", map[string]interface{}{
		"academia_id":     academiaID.String(),
		"codigo_academia": academia.CodigoAcademia,
	})

	c.JSON(http.StatusOK, gin.H{"message": "academia ativada com sucesso"})
}

// ============================================================================
// PUT /admin/academia/:id/desativar
// ============================================================================

// DesativarAcademia desativa uma academia por UUID.
// Rota: PUT /admin/academia/:id/desativar
// Protegida por AuthMiddleware + RequireAdmin + RequireAdm.
//
// FIX E4-AA-01: mesma correção de AtivarAcademia — lê `:id` (UUID) em vez
// de `:codigo` que nunca era preenchido pela rota.
// FIX C9: desativadoPor passado ao aggregate para rastreabilidade forense.
func DesativarAcademia(c *gin.Context) {
	// FIX E4-AA-01: ler `:id` (UUID) — consistente com a definição da rota.
	academiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de academia inválido"))
		return
	}

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(academiaID, "Academia")
	if err != nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	academia := agg.(*aggregates.Academia)

	// FIX C9: passar desativadoPor ao aggregate para inclusão no payload do evento.
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
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	registrarAcaoAdmin(c, adminUserID, "desativar_academia", map[string]interface{}{
		"academia_id":     academiaID.String(),
		"codigo_academia": academia.CodigoAcademia,
		"motivo":          req.Motivo,
	})

	c.JSON(http.StatusOK, gin.H{"message": "academia desativada com sucesso"})
}

// ============================================================================
// GET /academias
// ============================================================================

// ListarTodasAcademias lista academias acessíveis ao usuário autenticado.
//
// E4-ED-03 (aceito por design): estudantes autenticados podem listar academias
// com campos não-sensíveis (nome, endereço, província, nível escolar).
// Email é ocultado para não-admins. Campos operacionais internos (total_estudantes,
// version) são omitidos para estudantes. Este comportamento é intencional —
// estudantes precisam localizar academias para matrícula.
func ListarTodasAcademias(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	client := getDbClient(c)

	// Academias são controladas por status (ativo/inativo), não por soft-delete.
	rows, err := client.DB().Query(`
		SELECT id, type, nome, codigo_academia, provincia, endereco,
		       numero_telefone, email, website, nivel_escolar, status,
		       cursos, email_verificado, created_at, updated_at, total_estudantes, version
		FROM projection_academias
		WHERE status = 'ativo'
		ORDER BY nome ASC
	`)
	if err != nil {
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
			_ = json.Unmarshal([]byte(*aca.CursosJSON), &cursos)
		}
		if cursos == nil {
			cursos = []string{}
		}

		// Campos públicos — visíveis a todos os tipos autenticados.
		acadMap := map[string]interface{}{
			"id":              aca.ID,
			"type":            aca.Type,
			"nome":            aca.Nome,
			"codigo_academia": aca.CodigoAcademia,
			"provincia":       aca.Provincia,
			"endereco":        aca.Endereco,
			"numero_telefone": aca.NumeroTelefone,
			"website":         aca.Website,
			"nivel_escolar":   aca.NivelEscolar,
			"status":          aca.Status,
			"cursos":          cursos,
			"email_verificado": aca.EmailVerificado,
			"created_at":      aca.CreatedAt,
			"updated_at":      aca.UpdatedAt,
		}

		// Campos operacionais internos — apenas para admin.
		if userType == "admin" {
			acadMap["email"] = aca.Email
			acadMap["total_estudantes"] = aca.TotalEstudantes
			acadMap["version"] = aca.Version
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

	// Ocultar email e dados sensíveis de não-admins.
	if userType == "admin" {
		resp["email"] = academia.Email
		resp["motivo_desativacao"] = academia.MotivoDesativacao
	}

	c.JSON(http.StatusOK, resp)
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
	agg, err := repository.Load(userID, "Academia")
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
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "dados atualizados com sucesso"})
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
		// Fallback com hash se função SQL não estiver disponível.
		h := fnv.New32a()
		h.Write([]byte(codigoProvincia + time.Now().String()))
		codigo = fmt.Sprintf("%s%08d", codigoProvincia, h.Sum32()%100000000)
	}
	return codigo, nil
}