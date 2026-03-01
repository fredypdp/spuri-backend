// ============================================================================
// ARQUIVO: internal/handlers/auth_handlers.go
// ============================================================================

package handlers

import (
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================================
// Login
// ============================================================================

type LoginRequest struct {
	Usuario string `json:"usuario" binding:"required"`
	Senha   string `json:"senha" binding:"required"`
	Type    string `json:"type" binding:"required"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Type != "estudante" && req.Type != "academia" {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo deve ser 'estudante' ou 'academia'"))
		return
	}

	estudanteProj := getEstudanteProjection(c)
	academiaProj := getAcademiaProjection(c)

	var userID uuid.UUID
	var userName string
	var senhaHash string
	var codigo string

	if req.Type == "academia" {
		academia, err := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if academia == nil {
			utils.RespondWithUnauthorizedError(c)
			return
		}
		userID = academia.ID
		userName = academia.Nome
		senhaHash = academia.SenhaHash
		codigo = academia.CodigoAcademia
	} else {
		estudante, err := estudanteProj.GetByCodigo(req.Usuario)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if estudante == nil {
			utils.RespondWithUnauthorizedError(c)
			return
		}
		userID = estudante.ID
		userName = estudante.Nome
		senhaHash = estudante.SenhaHash
		codigo = estudante.CodigoEstudante
	}

	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.Senha)); err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	token, err := middleware.GenerateToken(userID, req.Type)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Login bem-sucedido: %s (%s)", userName, req.Type)

	c.JSON(http.StatusOK, gin.H{
		"token":  token,
		"codigo": codigo,
		"nome":   userName,
		"type":   req.Type,
	})
}

// ============================================================================
// RegisterAcademia
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

// ============================================================================
// RegisterEstudante (auto-cadastro pelo próprio estudante)
// ============================================================================

type RegisterEstudanteRequest struct {
	Senha                    string     `json:"senha" binding:"required"`
	Nome                     string     `json:"nome" binding:"required"`
	Email                    *string    `json:"email"`
	Telefone                 *string    `json:"telefone"`
	BilheteIdentidade        *string    `json:"bilhete_identidade"`
	BilheteIdentidadeResp    *string    `json:"bilhete_identidade_responsavel"`
	AnoEscolar               *string    `json:"ano_escolar"`
	AnoEscolarMedio          *string    `json:"ano_escolar_medio"`
	AnoSuperior              *string    `json:"ano_superior"`
	CursoMedioID             *uuid.UUID `json:"curso_medio_id"`
	CursoSuperiorID          *uuid.UUID `json:"curso_superior_id"`
	StatusEscolarFundamental *string    `json:"status_escolar_fundamental"`
	StatusEscolarMedio       *string    `json:"status_escolar_medio"`
	StatusSuperior           *string    `json:"status_superior"`
	Genero                   string     `json:"genero" binding:"required"`
}

func RegisterEstudante(c *gin.Context) {
	var req RegisterEstudanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateSenha(req.Senha); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Telefone != nil {
		if err := utils.ValidatePhone(*req.Telefone); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if err := utils.ValidateBilhete(utils.SafeDeref(req.BilheteIdentidade)); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateBilhete(utils.SafeDeref(req.BilheteIdentidadeResp)); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.BilheteIdentidade == nil && req.BilheteIdentidadeResp == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("pelo menos um bilhete de identidade é obrigatório"))
		return
	}

	if req.BilheteIdentidade != nil && *req.BilheteIdentidade != "" {
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(*req.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado"))
			return
		}
	}

	if req.CursoMedioID != nil && *req.CursoMedioID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id não encontrado"))
			return
		}
		if curso.Type != "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id deve ser do tipo 'medio'"))
			return
		}
	}

	if req.CursoSuperiorID != nil && *req.CursoSuperiorID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoSuperiorID)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id não encontrado"))
			return
		}
		if curso.Type != "superior" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id deve ser do tipo 'superior'"))
			return
		}
	}

	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	if err := estudante.Criar(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		req.Email,
		req.Telefone,
		req.BilheteIdentidade,
		req.BilheteIdentidadeResp,
		req.AnoEscolar,
		req.AnoEscolarMedio,
		req.AnoSuperior,
		req.CursoMedioID,
		req.CursoSuperiorID,
		req.StatusEscolarFundamental,
		req.StatusEscolarMedio,
		req.StatusSuperior,
		req.Genero,
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
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Estudante criado: %s - %s", codigoEstudante, req.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante criado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
		},
	})
}

// ============================================================================
// RegisterEstudantePorAcademia (cadastro pela academia)
// ============================================================================

type CadastroEstudanteAcademiaRequest struct {
	Nome                     string `json:"nome" binding:"required"`
	Email                    string `json:"email"`
	Telefone                 string `json:"telefone"`
	BilheteIdentidade        string `json:"bilhete_identidade"`
	BilheteResponsavel       string `json:"bilhete_identidade_responsavel"`
	AnoEscolar               string `json:"ano_escolar"`
	AnoEscolarMedio          string `json:"ano_escolar_medio"`
	AnoSuperior              string `json:"ano_superior"`
	CursoMedioID             string `json:"curso_medio_id"`
	CursoSuperiorID          string `json:"curso_superior_id"`
	StatusEscolarFundamental string `json:"status_escolar_fundamental"`
	StatusEscolarMedio       string `json:"status_escolar_medio"`
	StatusSuperior           string `json:"status_superior"`
	Genero                   string `json:"genero" binding:"required"`
}

func RegisterEstudantePorAcademia(c *gin.Context) {
	var req CadastroEstudanteAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("nome é obrigatório"))
		return
	}

	log.Printf("📥 [RegisterEstudantePorAcademia] Nome: %s, AnoEscolar: '%s', CursoMedioID: '%s'",
		req.Nome, req.AnoEscolar, req.CursoMedioID)

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != "" {
		if err := utils.ValidateEmail(req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Telefone != "" {
		if err := utils.ValidatePhone(req.Telefone); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.BilheteIdentidade != "" {
		if err := utils.ValidateBilhete(req.BilheteIdentidade); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.BilheteResponsavel != "" {
		if err := utils.ValidateBilhete(req.BilheteResponsavel); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.BilheteIdentidade != "" && req.BilheteResponsavel != "" && req.BilheteIdentidade == req.BilheteResponsavel {
		utils.RespondWithValidationError(c, fmt.Errorf("bilhete do estudante e do responsável não podem ser iguais"))
		return
	}

	if req.BilheteIdentidade == "" && req.BilheteResponsavel == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("pelo menos um bilhete de identidade é obrigatório"))
		return
	}

	if req.BilheteIdentidade != "" {
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(req.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado no sistema"))
			return
		}
	}

	// Converter strings para ponteiros
	var anoEscolarPtr, anoSuperiorPtr *string
	var cursoMedioUUID, cursoSuperiorUUID *uuid.UUID

	if req.AnoEscolar != "" {
		anoEscolarPtr = &req.AnoEscolar
	}
	var anoEscolarMedioPtr *string
	if req.AnoEscolarMedio != "" {
		anoEscolarMedioPtr = &req.AnoEscolarMedio
	}
	if req.AnoSuperior != "" {
		anoSuperiorPtr = &req.AnoSuperior
	}

	if req.CursoMedioID != "" {
		parsed, err := uuid.Parse(req.CursoMedioID)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id inválido"))
			return
		}
		cursoMedioUUID = &parsed

		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id não encontrado"))
			return
		}
		if curso.Type != "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id deve ser do tipo 'medio'"))
			return
		}
	}

	if req.CursoSuperiorID != "" {
		parsed, err := uuid.Parse(req.CursoSuperiorID)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id inválido"))
			return
		}
		cursoSuperiorUUID = &parsed

		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id não encontrado"))
			return
		}
		if curso.Type != "superior" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id deve ser do tipo 'superior'"))
			return
		}
	}

	academiaID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByID(academiaID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academia == nil {
		utils.RespondWithError(c, http.StatusNotFound, "academia não encontrada", nil)
		return
	}

	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("spuri123"), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Converter campos opcionais de string para *string
	var emailPtr, telefonePtr, bilhetePtr, bilheteRespPtr *string

	if req.Email != "" {
		emailPtr = &req.Email
	}
	if req.Telefone != "" {
		telefonePtr = &req.Telefone
	}
	if req.BilheteIdentidade != "" {
		bilhetePtr = &req.BilheteIdentidade
	}
	if req.BilheteResponsavel != "" {
		bilheteRespPtr = &req.BilheteResponsavel
	}

	// Status escolar fundamental — default "em_andamento" se academia não especificar
	var statusFundamentalPtr, statusMedioPtr, statusSuperiorPtr *string

	if req.StatusEscolarFundamental != "" {
		statusFundamentalPtr = &req.StatusEscolarFundamental
	} else {
		defaultStatus := "em_andamento"
		statusFundamentalPtr = &defaultStatus
	}
	if req.StatusEscolarMedio != "" {
		statusMedioPtr = &req.StatusEscolarMedio
	}
	if req.StatusSuperior != "" {
		statusSuperiorPtr = &req.StatusSuperior
	}

	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	log.Printf("🔧 [RegisterEstudantePorAcademia] AnoEscolar=%v, CursoMedioID=%v", anoEscolarPtr, cursoMedioUUID)

	if err := estudante.CriarComVinculo(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		emailPtr,
		telefonePtr,
		bilhetePtr,
		bilheteRespPtr,
		anoEscolarPtr,
		anoEscolarMedioPtr,
		anoSuperiorPtr,
		cursoMedioUUID,
		cursoSuperiorUUID,
		statusFundamentalPtr,
		statusMedioPtr,
		statusSuperiorPtr,
		academia.CodigoAcademia,
		req.Genero,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Rota autenticada — academia cadastrando estudante
	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [RegisterEstudantePorAcademia] Estudante criado: %s - AnoEscolar: %v, CursoMedioID: %v",
		codigoEstudante, anoEscolarPtr, cursoMedioUUID)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante cadastrado e vinculado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
			"codigo_academia":  academia.CodigoAcademia,
			"status":           "ativo",
			"ano_escolar":      anoEscolarPtr,
			"curso_medio_id":   cursoMedioUUID,
		},
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