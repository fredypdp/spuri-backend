// ============================================================================
// ARQUIVO: internal/handlers/academia_handlers.go
// ============================================================================

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
	if err := repository.SaveWithAudit(academia, audit); err != nil {
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
		if nivel == "fundamental" || nivel == "misto" {
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

	repository := getRepository(c)
	academiaAgg, err := repository.Load(userID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	academia := academiaAgg.(*aggregates.Academia)

	if err := academia.AtualizarDados(
		req.Nome, req.Provincia, req.Endereco,
		req.NumeroTelefone, req.Email, req.Website,
		req.NivelEscolar, req.AnosAcademicos, req.Cursos,
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

	log.Printf("Dados da academia atualizados: %s", academia.CodigoAcademia)
	c.JSON(http.StatusOK, gin.H{"message": "dados da academia atualizados com sucesso"})
}

// ============================================================================
// GET /academia/consultar-academia/:codigo  e  GET /consultar-academia/:codigo
// ============================================================================

func GetAcademiaPorCodigo(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	codigoAcademia := c.Param("codigo")

	if userType != "academia" && userType != "admin" {
		utils.RespondWithForbiddenError(c, "Acesso negado. Apenas academias e administradores podem consultar academias.")
		return
	}

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	var estatisticas *gin.H
	if userType == "admin" {
		client := getDbClient(c)

		var stats struct {
			TotalNotasRegistradas  int
			TotalFaltasRegistradas int
		}

		err := client.DB().QueryRow(`
			SELECT
				(SELECT COUNT(*) FROM projection_notas WHERE codigo_academia = $1) as total_notas,
				(SELECT COUNT(*) FROM projection_faltas WHERE codigo_academia = $1) as total_faltas
		`, codigoAcademia).Scan(
			&stats.TotalNotasRegistradas,
			&stats.TotalFaltasRegistradas,
		)
		if err == nil {
			estatisticas = &gin.H{
				"total_notas_registradas":  stats.TotalNotasRegistradas,
				"total_faltas_registradas": stats.TotalFaltasRegistradas,
			}
		}
	}

	response := gin.H{
		"academia": gin.H{
			"id":                  academia.ID,
			"type":                academia.Type,
			"nome":                academia.Nome,
			"codigo_academia":     academia.CodigoAcademia,
			"email":               academia.Email,
			"email_verificado":    academia.EmailVerificado,
			"provincia":           academia.Provincia,
			"endereco":            academia.Endereco,
			"numero_telefone":     academia.NumeroTelefone,
			"website":             academia.Website,
			"nivel_escolar":       academia.NivelEscolar,
			"anos_academicos":     academia.AnosAcademicos,
			"status":              academia.Status,
			"motivo_desativacao":  academia.MotivoDesativacao,
			"cursos":              academia.Cursos,
			"created_at":          academia.CreatedAt,
			"updated_at":          academia.UpdatedAt,
			"total_estudantes":    academia.TotalEstudantes,
			"version":             academia.Version,
		},
	}

	if estatisticas != nil {
		response["estatisticas"] = estatisticas
	}

	c.JSON(http.StatusOK, response)
}

// ============================================================================
// GET /academias
// ============================================================================

func ListarTodasAcademias(c *gin.Context) {
	_, _ = middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	client := getDbClient(c)

	// FIX E-07: removido `WHERE deleted_at IS NULL` — coluna não existe no schema.
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
// PUT /admin/academia/:codigo/ativar
// ============================================================================

// AtivarAcademia ativa uma academia.
// Rota protegida por AuthMiddleware + RequireAdmin + RequireGerente.
//
// FIX E-09: removida chamada redundante a verificarPermissaoAdmin — o middleware
// RequireGerente já garante a permissão. Manter ambos era ineficiente e confuso.
func AtivarAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	codigoAcademia := c.Param("codigo")

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

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
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

// ============================================================================
// PUT /admin/academia/:codigo/desativar
// ============================================================================

// DesativarAcademia desativa uma academia com motivo obrigatório.
// Rota protegida por AuthMiddleware + RequireAdmin + RequireGerente.
//
// FIX E-09: removida chamada redundante a verificarPermissaoAdmin.
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

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "admin",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
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

// ============================================================================
// Helpers
// ============================================================================

func validarProvincia(provincia string) (string, error) {
	provinciaInput := strings.ToUpper(strings.TrimSpace(provincia))

	provinciaMap := map[string]string{
		"BENGO": "BGO", "BGO": "BGO",
		"BENGUELA": "BGU", "BGU": "BGU",
		"BIE": "BIE", "BIÉ": "BIE",
		"CABINDA": "CAB", "CAB": "CAB",
		"CUANDO": "CND", "CND": "CND", "CUANDO CUBANGO": "CND",
		"CUANZA NORTE": "CNO", "CNO": "CNO", "KWANZA NORTE": "CNO",
		"CUANZA SUL": "CUS", "CUS": "CUS", "KWANZA SUL": "CUS",
		"CUBANGO": "CBG", "CBG": "CBG",
		"CUNENE": "CNN", "CNN": "CNN",
		"HUAMBO": "HUA", "HUA": "HUA",
		"HUILA": "HUI", "HUÍLA": "HUI", "HUI": "HUI",
		"ICOLO E BENGO": "IBG", "IBG": "IBG",
		"LUANDA": "LUA", "LUA": "LUA",
		"LUNDA NORTE": "LNO", "LNO": "LNO",
		"LUNDA SUL": "LSU", "LSU": "LSU",
		"MALANJE": "MAL", "MAL": "MAL",
		"MOXICO": "MOX", "MOX": "MOX",
		"MOXICO LESTE": "MXL", "MXL": "MXL",
		"NAMIBE": "NAM", "NAM": "NAM",
		"UIGE": "UIG", "UÍGE": "UIG", "UIG": "UIG",
		"ZAIRE": "ZAI", "ZAI": "ZAI",
	}

	if code, ok := provinciaMap[provinciaInput]; ok {
		return code, nil
	}
	return "", fmt.Errorf("província inválida: '%s'", provincia)
}

// generateCodigoAcademia gera um código único no formato {SIGLA}{ANO}{SEQ}.
//
// Exemplo: BGU20261, BGU20262, BGU20263 ...
//
// Estratégia de concorrência:
//   - pg_advisory_xact_lock serializa gerações com o mesmo prefix (sigla+ano).
//   - A transação segura o lock até o commit, evitando dois processos lerem
//     o mesmo MAX e gerarem o mesmo sequencial.
//   - O UNIQUE constraint em codigo_academia é a última linha de defesa.
func generateCodigoAcademia(sigla string, db *sqlx.DB) (string, error) {
	year := time.Now().Year()
	prefix := fmt.Sprintf("%s%d", sigla, year)

	// Hash do prefix para o advisory lock (int64 requerido pelo PostgreSQL)
	h := fnv.New64a()
	h.Write([]byte(prefix))
	lockKey := int64(h.Sum64() & 0x7FFFFFFFFFFFFFFF) // garantir positivo

	tx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		return "", fmt.Errorf("erro ao obter advisory lock: %w", err)
	}

	var maxSeq int
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(
			CAST(SUBSTRING(codigo_academia FROM $1) AS INTEGER)
		), 0)
		FROM projection_academias
		WHERE codigo_academia LIKE $2
	`, fmt.Sprintf("^%s(\\d+)$", prefix), prefix+"%").Scan(&maxSeq)
	if err != nil {
		return "", fmt.Errorf("erro ao buscar sequencial: %w", err)
	}

	newSeq := maxSeq + 1
	codigo := fmt.Sprintf("%s%d", prefix, newSeq)

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("erro ao commitar transação: %w", err)
	}

	return codigo, nil
}