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

func RegisterAcademia(c *gin.Context) {
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

	// Rota pública — sem JWT, usar contexto de sistema
	audit := db.AuditContext{
		UserID:   "anonimo",
		UserType: "sistema",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(academia, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Academia criada: %s - %s", codigoAcademia, req.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message": "academia criada com sucesso",
		"data": gin.H{
			"id":              academia.ID,
			"codigo_academia": academia.CodigoAcademia,
		},
	})
}

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
			"id":               academia.ID,
			"type":             academia.Type,
			"nome":             academia.Nome,
			"codigo_academia":  academia.CodigoAcademia,
			"email":            academia.Email,
			"email_verificado": academia.EmailVerificado,
			"provincia":        academia.Provincia,
			"endereco":         academia.Endereco,
			"numero_telefone":  academia.NumeroTelefone,
			"website":          academia.Website,
			"nivel_escolar":    academia.NivelEscolar,
			"anos_academicos":  academia.AnosAcademicos,
			"status":           academia.Status,
			"cursos":           academia.Cursos,
			"created_at":       academia.CreatedAt,
			"updated_at":       academia.UpdatedAt,
			"total_estudantes": academia.TotalEstudantes,
			"version":          academia.Version,
		},
	}

	if estatisticas != nil {
		response["estatisticas"] = estatisticas
	}

	c.JSON(http.StatusOK, response)
}

func ListarTodasAcademias(c *gin.Context) {
	_, _ = middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	_ = getAcademiaProjection(c)
	client := getDbClient(c)

	query := `
		SELECT id, type, nome, codigo_academia, provincia, endereco,
			numero_telefone, email, website, nivel_escolar, status, cursos,
			email_verificado, created_at, updated_at, total_estudantes, version
		FROM projection_academias
		WHERE deleted_at IS NULL
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
func generateCodigoAcademia(codigoProvincia string, db *sqlx.DB) (string, error) {
	ano := time.Now().Year()
	prefix := fmt.Sprintf("%s%d", codigoProvincia, ano) // ex: "BGU2026"

	// Chave determinística para o advisory lock (int64 via FNV hash do prefix)
	lockKey := prefixLockKey(prefix)

	log.Printf("🔒 [generateCodigoAcademia] Adquirindo lock para prefix=%s (lockKey=%d)", prefix, lockKey)

	tx, err := db.Begin()
	if err != nil {
		return "", fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback() // no-op após Commit

	// Serializa todas as gerações com o mesmo prefix dentro desta instância e
	// em quaisquer outras instâncias conectadas ao mesmo PostgreSQL.
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, lockKey); err != nil {
		return "", fmt.Errorf("erro ao adquirir advisory lock: %w", err)
	}

	// Busca o maior sequencial já existente para este prefix.
	query := fmt.Sprintf(`
		SELECT COALESCE(
			MAX(CAST(SUBSTRING(codigo_academia, %d) AS INTEGER)),
			0
		)
		FROM projection_academias
		WHERE codigo_academia ~ '^%s[0-9]+$'
	`, len(prefix)+1, prefix)

	var maxSeq int
	if err := tx.QueryRow(query).Scan(&maxSeq); err != nil {
		return "", fmt.Errorf("erro ao buscar sequencial: %w", err)
	}

	nextSeq := maxSeq + 1
	codigo := fmt.Sprintf("%s%d", prefix, nextSeq)

	log.Printf("✅ [generateCodigoAcademia] Código gerado: %s (maxSeq=%d → next=%d)", codigo, maxSeq, nextSeq)

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("erro ao confirmar transação: %w", err)
	}

	return codigo, nil
}

// prefixLockKey converte uma string (ex: "BGU2026") em um int64 estável
// para uso como chave no pg_advisory_xact_lock.
func prefixLockKey(prefix string) int64 {
	h := fnv.New64a()
	h.Write([]byte(prefix))
	return int64(h.Sum64())
}