package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/services"
	"spuri/internal/utils"
)

// ============================================================================
// POST /academia/estudante/register
// ============================================================================

// CadastroEstudanteAcademiaRequest — genero e data_nascimento são obrigatórios.
type CadastroEstudanteAcademiaRequest struct {
	Nome                     string    `json:"nome"            binding:"required"`
	Genero                   string    `json:"genero"          binding:"required"`
	DataNascimento           time.Time `json:"data_nascimento" binding:"required"`
	Email                    string    `json:"email"`
	Telefone                 string    `json:"telefone"`
	BilheteIdentidade        string    `json:"bilhete_identidade"`
	BilheteResponsavel       string    `json:"bilhete_identidade_responsavel"`
	AnoEscolar               string    `json:"ano_escolar_fundamental"`
	AnoEscolarMedio          string    `json:"ano_escolar_medio"`
	AnoSuperior              string    `json:"ano_superior"`
	CursoMedioID             string    `json:"curso_medio_id"`
	CursoSuperiorID          string    `json:"curso_superior_id"`
	StatusEscolarFundamental string    `json:"status_escolar_fundamental"`
	StatusEscolarMedio       string    `json:"status_escolar_medio"`
	StatusSuperior           string    `json:"status_superior"`
}

func RegisterEstudantePorAcademia(c *gin.Context) {
	var req CadastroEstudanteAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campos obrigatórios: nome, genero, data_nascimento"))
		return
	}

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Genero != "masculino" && req.Genero != "feminino" {
		utils.RespondWithValidationError(c, fmt.Errorf("genero deve ser 'masculino' ou 'feminino'"))
		return
	}

	// data_nascimento obrigatório e deve ser estritamente no passado
	hoje := time.Now().UTC().Truncate(24 * time.Hour)
	dataNasc := req.DataNascimento.UTC().Truncate(24 * time.Hour)
	if !dataNasc.Before(hoje) {
		utils.RespondWithValidationError(c, fmt.Errorf("data_nascimento deve ser anterior à data atual"))
		return
	}

	if req.Email != "" {
		if err := utils.ValidateEmail(req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	// Resolver curso médio
	var cursoMedioUUID *uuid.UUID
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

	// Resolver curso superior
	var cursoSuperiorUUID *uuid.UUID
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

	defaultPassword := services.GetDefaultPassword("estudante", codigoEstudante)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Campos opcionais string → *string
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

	var anoEscolarPtr, anoEscolarMedioPtr, anoSuperiorPtr *string
	if req.AnoEscolar != "" {
		if err := utils.ValidateAnoFundamental(req.AnoEscolar); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("ano_escolar_fundamental inválido: %w", err))
			return
		}
		anoEscolarPtr = &req.AnoEscolar
	}
	if req.AnoEscolarMedio != "" {
		if err := utils.ValidateAnoMedio(req.AnoEscolarMedio); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("ano_escolar_medio inválido: %w", err))
			return
		}
		anoEscolarMedioPtr = &req.AnoEscolarMedio
	}
	if req.AnoSuperior != "" {
		if err := utils.ValidateAnoSuperior(req.AnoSuperior); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("ano_superior inválido: %w", err))
			return
		}
		anoSuperiorPtr = &req.AnoSuperior
	}

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

	if err := estudante.CriarComVinculo(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		emailPtr,
		telefonePtr,
		bilhetePtr,
		bilheteRespPtr,
		req.Genero,
		req.DataNascimento,
		anoEscolarPtr,
		anoEscolarMedioPtr,
		anoSuperiorPtr,
		cursoMedioUUID,
		cursoSuperiorUUID,
		statusFundamentalPtr,
		statusMedioPtr,
		statusSuperiorPtr,
		&academiaID,
		academia.CodigoAcademia,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Estudante criado por academia %s: %s - %s", academia.CodigoAcademia, codigoEstudante, req.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante registrado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
			"codigo_academia":  academia.CodigoAcademia,
		},
	})
}

// ============================================================================
// GET /estudantes
// ============================================================================

func ListarEstudantes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	client := getDbClient(c)

	// data_nascimento é NOT NULL após a migration — scan direto como time.Time.
	const selectCols = `
		SELECT id, nome, codigo_estudante, email, telefone, email_verificado,
			bilhete_identidade, bilhete_identidade_responsavel, codigo_academia,
			status, status_escolar_fundamental, status_escolar_medio, status_superior,
			ano_escolar_fundamental, ano_escolar_medio, ano_superior,
			curso_medio_id, curso_superior_id,
			genero, data_nascimento, created_at, updated_at,
			COALESCE(total_notas, 0), COALESCE(total_faltas, 0), version
		FROM projection_estudantes`

	var (
		rows       *sql.Rows
		err        error
		conditions []string
		args       []interface{}
	)

	if generos := parseMultiValueQueryParam(c, "genero"); len(generos) > 0 {
		args = append(args, pq.Array(generos))
		conditions = append(conditions, fmt.Sprintf("e.genero = ANY($%d)", len(args)))
	}
	if anosFund := parseMultiValueQueryParam(c, "ano_escolar_fundamental"); len(anosFund) > 0 {
		args = append(args, pq.Array(anosFund))
		conditions = append(conditions, fmt.Sprintf("e.ano_escolar_fundamental = ANY($%d)", len(args)))
	}
	if anosMedio := parseMultiValueQueryParam(c, "ano_escolar_medio"); len(anosMedio) > 0 {
		args = append(args, pq.Array(anosMedio))
		conditions = append(conditions, fmt.Sprintf("e.ano_escolar_medio = ANY($%d)", len(args)))
	}
	if anosSup := parseMultiValueQueryParam(c, "ano_superior"); len(anosSup) > 0 {
		args = append(args, pq.Array(anosSup))
		conditions = append(conditions, fmt.Sprintf("e.ano_superior = ANY($%d)", len(args)))
	}
	if turno := parseMultiValueQueryParam(c, "turno"); len(turno) > 0 {
		args = append(args, pq.Array(turno))
		conditions = append(conditions, fmt.Sprintf("t.turno = ANY($%d)", len(args)))
	}
	if codigoTurma := parseMultiValueQueryParam(c, "codigo_turma"); len(codigoTurma) > 0 {
		args = append(args, pq.Array(codigoTurma))
		conditions = append(conditions, fmt.Sprintf("t.codigo_turma = ANY($%d)", len(args)))
	}

	if withClass := strings.TrimSpace(c.Query("com_turma")); withClass != "" {
		v, parseErr := strconv.ParseBool(withClass)
		if parseErr != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("com_turma deve ser booleano (true/false)"))
			return
		}
		if v {
			conditions = append(conditions, "t.codigo_turma IS NOT NULL")
		} else {
			conditions = append(conditions, "t.codigo_turma IS NULL")
		}
	}

	if idadeMinStr := strings.TrimSpace(c.Query("idade_min")); idadeMinStr != "" {
		idadeMin, parseErr := strconv.Atoi(idadeMinStr)
		if parseErr != nil || idadeMin < 0 {
			utils.RespondWithValidationError(c, fmt.Errorf("idade_min inválida"))
			return
		}
		args = append(args, time.Now().AddDate(-idadeMin, 0, 0))
		conditions = append(conditions, fmt.Sprintf("e.data_nascimento <= $%d", len(args)))
	}
	if idadeMaxStr := strings.TrimSpace(c.Query("idade_max")); idadeMaxStr != "" {
		idadeMax, parseErr := strconv.Atoi(idadeMaxStr)
		if parseErr != nil || idadeMax < 0 {
			utils.RespondWithValidationError(c, fmt.Errorf("idade_max inválida"))
			return
		}
		args = append(args, time.Now().AddDate(-(idadeMax+1), 0, 1))
		conditions = append(conditions, fmt.Sprintf("e.data_nascimento >= $%d", len(args)))
	}

	for _, item := range []struct{ key, col string }{
		{"status_escolar_fundamental", "e.status_escolar_fundamental"},
		{"status_escolar_medio", "e.status_escolar_medio"},
		{"status_superior", "e.status_superior"},
	} {
		if values := parseMultiValueQueryParam(c, item.key); len(values) > 0 {
			args = append(args, pq.Array(values))
			conditions = append(conditions, fmt.Sprintf("%s = ANY($%d)", item.col, len(args)))
		}
	}

	baseQuery := selectCols + ` e
		LEFT JOIN projection_turmas t
		  ON t.codigo_academia = e.codigo_academia
		 AND EXISTS (
		    SELECT 1 FROM jsonb_array_elements_text(COALESCE(t.estudantes, '[]'::jsonb)) AS cod(codigo)
		    WHERE cod.codigo = e.codigo_estudante
		 )`

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithForbiddenError(c, "academia não encontrada")
			return
		}
		argsAcademia := append([]interface{}{}, args...)
		argsAcademia = append(argsAcademia, academiaDTO.CodigoAcademia)
		where := append([]string{}, conditions...)
		where = append(where, fmt.Sprintf("e.codigo_academia = $%d", len(argsAcademia)))
		rows, err = client.DB().Query(baseQuery+` WHERE `+strings.Join(where, " AND ")+` ORDER BY e.created_at DESC`, argsAcademia...)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()
		estudantes := scanEstudantesRows(rows)
		c.JSON(http.StatusOK, gin.H{
			"estudantes":      estudantes,
			"total":           len(estudantes),
			"tipo_usuario":    "academia",
			"codigo_academia": academiaDTO.CodigoAcademia,
			"nome_academia":   academiaDTO.Nome,
		})
		return
	}

	if userType == "admin" {
		query := baseQuery
		if len(conditions) > 0 {
			query += ` WHERE ` + strings.Join(conditions, " AND ")
		}
		query += ` ORDER BY e.created_at DESC`
		rows, err = client.DB().Query(query, args...)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()
		estudantes := scanEstudantesRows(rows)
		c.JSON(http.StatusOK, gin.H{
			"estudantes":   estudantes,
			"total":        len(estudantes),
			"tipo_usuario": "admin",
		})
		return
	}

	utils.RespondWithForbiddenError(c, "Acesso negado. Apenas academias e administradores podem listar estudantes.")
}

// scanEstudantesRows faz scan das linhas retornadas por ListarEstudantes.
// data_nascimento é NOT NULL no banco após a migration 043.
func scanEstudantesRows(rows *sql.Rows) []map[string]interface{} {
	var estudantes []map[string]interface{}
	for rows.Next() {
		var id, cursoMedioID, cursoSuperiorID sql.NullString
		var nome, codigoEstudante string
		var status, statusFund, statusMedio, statusSuperior sql.NullString
		var email, telefone, bilhete, bilheteResp, codigoAcad sql.NullString
		var anoEscolar, anoEscolarMedio, anoSuperior sql.NullString
		var emailVerif bool
		var genero sql.NullString
		var dataNascimento, createdAt, updatedAt sql.NullTime
		var totalNotas, totalFaltas, version int

		if err := rows.Scan(
			&id, &nome, &codigoEstudante,
			&email, &telefone, &emailVerif, &bilhete, &bilheteResp, &codigoAcad,
			&status, &statusFund, &statusMedio, &statusSuperior,
			&anoEscolar, &anoEscolarMedio, &anoSuperior,
			&cursoMedioID, &cursoSuperiorID,
			&genero, &dataNascimento, &createdAt, &updatedAt,
			&totalNotas, &totalFaltas, &version,
		); err != nil {
			log.Printf("[ERROR] ListarEstudantes scan: %v", err)
			continue
		}

		estudantes = append(estudantes, map[string]interface{}{
			"nome":                           nome,
			"codigo_estudante":               codigoEstudante,
			"email":                          getNullString(email),
			"telefone":                       getNullString(telefone),
			"email_verificado":               emailVerif,
			"bilhete_identidade":             getNullString(bilhete),
			"bilhete_identidade_responsavel": getNullString(bilheteResp),
			"codigo_academia":                getNullString(codigoAcad),
			"status":                         getNullString(status),
			"status_escolar_fundamental":     getNullString(statusFund),
			"status_escolar_medio":           getNullString(statusMedio),
			"status_superior":                getNullString(statusSuperior),
			"ano_escolar_fundamental":        getNullString(anoEscolar),
			"ano_escolar_medio":              getNullString(anoEscolarMedio),
			"ano_superior":                   getNullString(anoSuperior),
			"curso_medio_id":                 getNullString(cursoMedioID),
			"curso_superior_id":              getNullString(cursoSuperiorID),
			"genero":                         getNullString(genero),
			"data_nascimento":                formatNullDate(dataNascimento),
			"created_at":                     formatNullRFC3339(createdAt),
			"updated_at":                     formatNullRFC3339(updatedAt),
			"total_notas":                    totalNotas,
			"total_faltas":                   totalFaltas,
			"version":                        version,
		})
	}
	return estudantes
}

func formatNullDate(nt sql.NullTime) interface{} {
	if nt.Valid {
		return nt.Time.Format("2006-01-02")
	}
	return nil
}

func formatNullRFC3339(nt sql.NullTime) interface{} {
	if nt.Valid {
		return nt.Time.Format(time.RFC3339)
	}
	return nil
}

// ============================================================================
// GET /eventos-estudante/:codigo
// ============================================================================

func GetEventosEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	userType, _ := middleware.GetUserType(c)
	if userType != "admin" {
		utils.RespondWithForbiddenError(c, "Apenas administradores podem consultar eventos.")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	repository := getRepository(c)
	eventos, err := repository.GetEventHistory(estudante.ID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"eventos":          eventos,
		"total":            len(eventos),
	})
}

// ============================================================================
// PUT /estudante/dados-pessoais
// ============================================================================

// AtualizarDadosPessoaisRequest — DataNascimento é ponteiro (nil = não alterar).
// Genero não pode ser alterado após o cadastro inicial.
type AtualizarDadosPessoaisRequest struct {
	Nome                  *string    `json:"nome"`
	Email                 *string    `json:"email"`
	Telefone              *string    `json:"telefone"`
	BilheteIdentidade     *string    `json:"bilhete_identidade"`
	BilheteIdentidadeResp *string    `json:"bilhete_identidade_responsavel"`
	DataNascimento        *time.Time `json:"data_nascimento"`
}

func AtualizarDadosPessoais(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req AtualizarDadosPessoaisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// Validar data_nascimento apenas se fornecida
	if req.DataNascimento != nil {
		hoje := time.Now().UTC().Truncate(24 * time.Hour)
		dataNasc := req.DataNascimento.UTC().Truncate(24 * time.Hour)
		if !dataNasc.Before(hoje) {
			utils.RespondWithValidationError(c, fmt.Errorf("data_nascimento deve ser anterior à data atual"))
			return
		}
	}

	if req.BilheteIdentidade != nil && *req.BilheteIdentidade != "" {
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(*req.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil && existente.ID != userID {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado"))
			return
		}
	}

	repository := getRepository(c)
	estudanteAgg, err := repository.Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return
	}

	if err := estudante.AtualizarDadosPessoais(
		req.Nome, req.Email, req.Telefone,
		req.BilheteIdentidade, req.BilheteIdentidadeResp,
		req.DataNascimento,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "estudante",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Dados pessoais atualizados: %s", estudante.CodigoEstudante)
	c.JSON(http.StatusOK, gin.H{"message": "dados pessoais atualizados com sucesso"})
}

// ============================================================================
// GET /estudante/minhas-avaliacoes
// ============================================================================

func GetMinhasAvaliacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByID(userID)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	avaliacaoProj := getAvaliacaoFinalProjection(c)
	avaliacoes, err := avaliacaoProj.GetByEstudante(estudante.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"avaliacoes": avaliacoes,
		"total":      len(avaliacoes),
	})
}

// ============================================================================
// GET /consultar-estudante/:codigo
// ============================================================================

func GetEstudantePorCodigo(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	userType, _ := middleware.GetUserType(c)
	if userType != "academia" && userType != "admin" {
		utils.RespondWithForbiddenError(c, "Apenas academias e administradores podem consultar estudantes.")
		return
	}

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	if userType == "academia" {
		userID, _ := middleware.GetUserID(c)
		academiaProj := getAcademiaProjection(c)
		academia, _ := academiaProj.GetByID(userID)
		if academia == nil || estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academia.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia.")
			return
		}
	}

	var academiaInfo *gin.H
	if estudante.CodigoAcademia != nil {
		academiaProj := getAcademiaProjection(c)
		academia, _ := academiaProj.GetByCodigo(*estudante.CodigoAcademia)
		if academia != nil {
			academiaInfo = &gin.H{
				"codigo": academia.CodigoAcademia,
				"nome":   academia.Nome,
				"nivel":  academia.Nivel,
				"type":   academia.Type,
			}
		}
	}

	var cursoMedioInfo, cursoSuperiorInfo *gin.H
	cursosProj := getCursosProjection(c)

	if estudante.CursoMedioID != nil {
		cursoMedioUUID, err := uuid.Parse(*estudante.CursoMedioID)
		if err == nil {
			cursoMedio, _ := cursosProj.GetByID(cursoMedioUUID)
			if cursoMedio != nil {
				cursoMedioInfo = &gin.H{
					"id":     cursoMedio.ID,
					"nome":   cursoMedio.Nome,
					"type":   cursoMedio.Type,
					"status": cursoMedio.Status,
				}
			}
		}
	}

	if estudante.CursoSuperiorID != nil {
		cursoSuperiorUUID, err := uuid.Parse(*estudante.CursoSuperiorID)
		if err == nil {
			cursoSuperior, _ := cursosProj.GetByID(cursoSuperiorUUID)
			if cursoSuperior != nil {
				cursoSuperiorInfo = &gin.H{
					"id":     cursoSuperior.ID,
					"nome":   cursoSuperior.Nome,
					"type":   cursoSuperior.Type,
					"status": cursoSuperior.Status,
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante": gin.H{
			"id":                             estudante.ID,
			"nome":                           estudante.Nome,
			"codigo_estudante":               estudante.CodigoEstudante,
			"email":                          estudante.Email,
			"telefone":                       estudante.Telefone,
			"email_verificado":               estudante.EmailVerificado,
			"bilhete_identidade":             estudante.BilheteIdentidade,
			"bilhete_identidade_responsavel": estudante.BilheteIdentidadeResp,
			"genero":                         estudante.Genero,
			"data_nascimento":                estudante.DataNascimento.Format("2006-01-02"),
			"codigo_academia":                estudante.CodigoAcademia,
			"academia":                       academiaInfo,
			"status":                         estudante.Status,
			"status_escolar_fundamental":     estudante.StatusEscolarFundamental,
			"status_escolar_medio":           estudante.StatusEscolarMedio,
			"status_superior":                estudante.StatusSuperior,
			"ano_escolar_fundamental":        estudante.AnoEscolar,
			"ano_escolar_medio":              estudante.AnoEscolarMedio,
			"ano_superior":                   estudante.AnoSuperior,
			"curso_medio":                    cursoMedioInfo,
			"curso_superior":                 cursoSuperiorInfo,
			"created_at":                     estudante.CreatedAt,
			"updated_at":                     estudante.UpdatedAt,
			"version":                        estudante.Version,
		},
	})
}
