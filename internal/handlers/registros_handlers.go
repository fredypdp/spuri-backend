package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"spuri/internal/db"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type NotaRegistroResponse struct {
	ID                   string     `json:"id"`
	CodigoEstudante      string     `json:"codigo_estudante"`
	EstudanteNome        string     `json:"estudante_nome"`
	CodigoAcademia       string     `json:"codigo_academia"`
	AcademiaNome         string     `json:"academia_nome"`
	AnoLectivo           string     `json:"ano_lectivo"`
	AnoAcademico         string     `json:"ano_academico"`
	Periodo              string     `json:"periodo"`
	MateriaDisciplinarID string     `json:"materia_disciplinar_id"`
	MateriaNome          string     `json:"materia_nome"`
	Tipo                 string     `json:"tipo"`
	Categoria            string     `json:"categoria"`
	Nota                 float64    `json:"nota"`
	Observacao           *string    `json:"observacao,omitempty"`
	RegistradoPor        *uuid.UUID `json:"registrado_por,omitempty"`
	ValorAnterior        *float64   `json:"valor_anterior,omitempty"`
	MotivoCorrecao       *string    `json:"motivo_correcao,omitempty"`
	CorrigidoPor         *uuid.UUID `json:"corrigido_por,omitempty"`
	CorrigidoEm          *time.Time `json:"corrigido_em,omitempty"`
	RegisteredAt         string     `json:"registered_at"`
	EventID              string     `json:"event_id"`
	Version              int        `json:"version"`
}

type FaltaRegistroResponse struct {
	ID                   string     `json:"id"`
	CodigoEstudante      string     `json:"codigo_estudante"`
	EstudanteNome        string     `json:"estudante_nome"`
	CodigoAcademia       string     `json:"codigo_academia"`
	AcademiaNome         string     `json:"academia_nome"`
	AnoLectivo           string     `json:"ano_lectivo"`
	AnoAcademico         string     `json:"ano_academico"`
	Periodo              string     `json:"periodo"`
	Data                 utils.Date `json:"data"`
	MateriaDisciplinarID string     `json:"materia_disciplinar_id"`
	MateriaNome          string     `json:"materia_nome"`
	Quantidade           int        `json:"quantidade"`
	Observacao           *string    `json:"observacao,omitempty"`
	RegistradoPor        *uuid.UUID `json:"registrado_por,omitempty"`
	ValorAnterior        *int       `json:"valor_anterior,omitempty"`
	MotivoCorrecao       *string    `json:"motivo_correcao,omitempty"`
	CorrigidoPor         *uuid.UUID `json:"corrigido_por,omitempty"`
	CorrigidoEm          *time.Time `json:"corrigido_em,omitempty"`
	RegisteredAt         string     `json:"registered_at"`
	EventID              string     `json:"event_id"`
	Version              int        `json:"version"`
}

func resolverEscopoRegistros(c *gin.Context) (userType, codigoAcademia string, ok bool) {
	userID, _ := middleware.GetUserID(c)
	userType, _ = middleware.GetUserType(c)

	switch userType {
	case "admin":
		return userType, "", true
	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return "", "", false
		}
		return userType, academiaDTO.CodigoAcademia, true
	default:
		utils.RespondWithForbiddenError(c, "acesso permitido apenas para admin e academia")
		return "", "", false
	}
}

// ListarNotas retorna notas do sistema:
// - admin: todas as notas
// - academia: apenas notas vinculadas ao seu codigo_academia
func ListarNotas(c *gin.Context) {
	userType, codigoAcademia, ok := resolverEscopoRegistros(c)
	if !ok {
		return
	}

	client := getDbClient(c)
	limit, offset := getPaginationParams(c)
	limit = db.ValidateLimit(limit)
	offset = db.ValidateOffset(offset)

	filtros, err := parseFiltrosRegistros(c, userType, codigoAcademia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if filtros.forbidden {
		utils.RespondWithForbiddenError(c, "academia só pode consultar os próprios dados")
		return
	}

	baseQuery := `
		SELECT
			n.id, n.codigo_estudante, e.nome as estudante_nome,
			n.codigo_academia, a.nome as academia_nome, n.ano_lectivo, n.ano_academico, n.periodo,
			n.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			n.tipo, n.categoria, n.nota, n.observacao, n.registrado_por, n.valor_anterior,
			n.motivo_correcao, n.corrigido_por, n.corrigido_em, n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_estudantes e ON n.codigo_estudante = e.codigo_estudante
		LEFT JOIN projection_academias a ON n.codigo_academia = a.codigo_academia
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id
	`
	baseCountQuery := `
		SELECT COUNT(*)
		FROM projection_notas n
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id
	`
	whereSQL, args := filtros.buildWhereSQL("n", true)
	orderPagination := fmt.Sprintf(" ORDER BY n.registered_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	argsRows := append(append([]interface{}{}, args...), limit, offset)

	var rowsErr error
	var rows interface {
		Close() error
		Next() bool
		Scan(...interface{}) error
		Err() error
	}

	rowsTyped, err := client.DB().Query(baseQuery+whereSQL+orderPagination, argsRows...)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	rows = rowsTyped
	defer rows.Close()

	var notas []NotaRegistroResponse
	for rows.Next() {
		var nota NotaRegistroResponse
		if err := rows.Scan(
			&nota.ID, &nota.CodigoEstudante, &nota.EstudanteNome,
			&nota.CodigoAcademia, &nota.AcademiaNome, &nota.AnoLectivo, &nota.AnoAcademico, &nota.Periodo,
			&nota.MateriaDisciplinarID, &nota.MateriaNome,
			&nota.Tipo, &nota.Categoria, &nota.Nota, &nota.Observacao, &nota.RegistradoPor, &nota.ValorAnterior,
			&nota.MotivoCorrecao, &nota.CorrigidoPor, &nota.CorrigidoEm, &nota.RegisteredAt, &nota.EventID, &nota.Version,
		); err != nil {
			log.Printf("[WARN] ListarNotas: erro ao ler linha: %v", err)
			continue
		}
		notas = append(notas, nota)
	}

	if rowsErr = rows.Err(); rowsErr != nil {
		log.Printf("[ERROR] ListarNotas: erro durante iteração: %v", rowsErr)
		utils.RespondWithInternalError(c, rowsErr)
		return
	}

	var total int
	if err := client.DB().QueryRow(baseCountQuery+whereSQL, args...).Scan(&total); err != nil {
		if userType == "academia" {
			log.Printf("[WARN] ListarNotas: erro ao contar notas da academia %s: %v", codigoAcademia, err)
		} else {
			log.Printf("[WARN] ListarNotas: erro ao contar notas: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"notas":       notas,
		"total":       len(notas),
		"total_geral": total,
		"limit":       limit,
		"offset":      offset,
	})
}

// ListarFaltas retorna faltas do sistema:
// - admin: todas as faltas
// - academia: apenas faltas vinculadas ao seu codigo_academia
func ListarFaltas(c *gin.Context) {
	userType, codigoAcademia, ok := resolverEscopoRegistros(c)
	if !ok {
		return
	}

	client := getDbClient(c)
	limit, offset := getPaginationParams(c)
	limit = db.ValidateLimit(limit)
	offset = db.ValidateOffset(offset)

	filtros, err := parseFiltrosRegistros(c, userType, codigoAcademia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if filtros.forbidden {
		utils.RespondWithForbiddenError(c, "academia só pode consultar os próprios dados")
		return
	}

	baseQuery := `
		SELECT
			f.id, f.codigo_estudante, e.nome as estudante_nome,
			f.codigo_academia, a.nome as academia_nome, f.ano_lectivo, f.ano_academico,
			COALESCE(f.periodo, ''), f.data, f.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			f.quantidade, f.observacao, f.registrado_por, f.valor_anterior,
			f.motivo_correcao, f.corrigido_por, f.corrigido_em, f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_estudantes e ON f.codigo_estudante = e.codigo_estudante
		LEFT JOIN projection_academias a ON f.codigo_academia = a.codigo_academia
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id = m.id
	`
	baseCountQuery := `
		SELECT COUNT(*)
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id = m.id
	`
	whereSQL, args := filtros.buildWhereSQL("f", true)
	orderPagination := fmt.Sprintf(" ORDER BY f.registered_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	argsRows := append(append([]interface{}{}, args...), limit, offset)

	var rows interface {
		Close() error
		Next() bool
		Scan(...interface{}) error
		Err() error
	}
	rowsTyped, err := client.DB().Query(baseQuery+whereSQL+orderPagination, argsRows...)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	rows = rowsTyped
	defer rows.Close()

	var faltas []FaltaRegistroResponse
	for rows.Next() {
		var falta FaltaRegistroResponse
		if err := rows.Scan(
			&falta.ID, &falta.CodigoEstudante, &falta.EstudanteNome,
			&falta.CodigoAcademia, &falta.AcademiaNome, &falta.AnoLectivo, &falta.AnoAcademico,
			&falta.Periodo, &falta.Data, &falta.MateriaDisciplinarID, &falta.MateriaNome,
			&falta.Quantidade, &falta.Observacao, &falta.RegistradoPor, &falta.ValorAnterior,
			&falta.MotivoCorrecao, &falta.CorrigidoPor, &falta.CorrigidoEm, &falta.RegisteredAt, &falta.EventID, &falta.Version,
		); err != nil {
			log.Printf("[WARN] ListarFaltas: erro ao ler linha: %v", err)
			continue
		}
		faltas = append(faltas, falta)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[ERROR] ListarFaltas: erro durante iteração: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	var total int
	if err := client.DB().QueryRow(baseCountQuery+whereSQL, args...).Scan(&total); err != nil {
		if userType == "academia" {
			log.Printf("[WARN] ListarFaltas: erro ao contar faltas da academia %s: %v", codigoAcademia, err)
		} else {
			log.Printf("[WARN] ListarFaltas: erro ao contar faltas: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"faltas":      faltas,
		"total":       len(faltas),
		"total_geral": total,
		"limit":       limit,
		"offset":      offset,
	})
}

type filtrosRegistros struct {
	anoLectivos           []string
	anoAcademicos         []string
	cursoIDs              []string
	codigosTurma          []string
	periodos              []string
	materiasDisciplinares []string
	codigosAcademia       []string
	categorias            []string
	corrigido             *bool
	forbidden             bool
}

func parseFiltrosRegistros(c *gin.Context, userType, codigoAcademiaEscopo string) (filtrosRegistros, error) {
	f := filtrosRegistros{
		anoLectivos:           parseMultiValueQueryParam(c, "ano_letivo"),
		anoAcademicos:         parseMultiValueQueryParam(c, "ano_academico"),
		cursoIDs:              parseMultiValueQueryParam(c, "curso_id"),
		codigosTurma:          parseMultiValueQueryParam(c, "codigo_turma"),
		periodos:              parseMultiValueQueryParam(c, "periodo"),
		materiasDisciplinares: parseMultiValueQueryParam(c, "materia_disciplinar_id"),
		codigosAcademia:       parseMultiValueQueryParam(c, "codigo_academia"),
		categorias:            parseMultiValueQueryParam(c, "categoria"),
	}
	if raw := strings.TrimSpace(c.Query("corrigido")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return f, fmt.Errorf("corrigido deve ser true ou false")
		}
		f.corrigido = &value
	}

	if len(f.codigosTurma) > 0 && userType == "admin" && len(f.codigosAcademia) == 0 {
		return f, fmt.Errorf("filtro codigo_turma exige codigo_academia para consultas admin")
	}

	if userType == "academia" {
		if len(f.codigosAcademia) > 0 {
			for _, codigoAcademia := range f.codigosAcademia {
				if codigoAcademia != codigoAcademiaEscopo {
					f.forbidden = true
					return f, nil
				}
			}
		}
		f.codigosAcademia = []string{codigoAcademiaEscopo}
	}

	for _, cursoID := range f.cursoIDs {
		if _, err := uuid.Parse(cursoID); err != nil {
			return f, fmt.Errorf("curso_id inválido")
		}
	}
	for _, materiaID := range f.materiasDisciplinares {
		if _, err := uuid.Parse(materiaID); err != nil {
			return f, fmt.Errorf("materia_disciplinar_id inválido")
		}
	}

	return f, nil
}

func parseMultiValueQueryParam(c *gin.Context, key string) []string {
	rawValues := c.QueryArray(key)
	if len(rawValues) == 0 {
		return nil
	}

	parsedValues := make([]string, 0, len(rawValues))
	seen := make(map[string]struct{}, len(rawValues))
	for _, raw := range rawValues {
		for _, part := range strings.Split(raw, ",") {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			parsedValues = append(parsedValues, value)
		}
	}
	return parsedValues
}

func (f filtrosRegistros) buildWhereSQL(alias string, includePeriodoRegistro bool) (string, []interface{}) {
	conditions := make([]string, 0, 10)
	args := make([]interface{}, 0, 10)
	add := func(sql string, value interface{}) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(sql, len(args)))
	}

	if len(f.codigosAcademia) > 0 {
		add(alias+".codigo_academia = ANY($%d)", pq.Array(f.codigosAcademia))
	}
	if len(f.anoLectivos) > 0 {
		add(alias+".ano_lectivo = ANY($%d)", pq.Array(f.anoLectivos))
	}
	if len(f.anoAcademicos) > 0 {
		add(alias+".ano_academico = ANY($%d)", pq.Array(f.anoAcademicos))
	}
	if len(f.cursoIDs) > 0 {
		add("m.curso_id = ANY($%d)", pq.Array(f.cursoIDs))
	}
	if len(f.periodos) > 0 {
		if includePeriodoRegistro {
			add(alias+".periodo = ANY($%d)", pq.Array(f.periodos))
		} else {
			add("m.periodo = ANY($%d)", pq.Array(f.periodos))
		}
	}
	if len(f.materiasDisciplinares) > 0 {
		add(alias+".materia_disciplinar_id = ANY($%d)", pq.Array(f.materiasDisciplinares))
	}
	if includePeriodoRegistro && len(f.categorias) > 0 {
		add(alias+".categoria = ANY($%d)", pq.Array(f.categorias))
	}
	if f.corrigido != nil {
		if *f.corrigido {
			conditions = append(conditions, alias+".corrigido_em IS NOT NULL")
		} else {
			conditions = append(conditions, alias+".corrigido_em IS NULL")
		}
	}
	if len(f.codigosTurma) > 0 {
		args = append(args, pq.Array(f.codigosTurma))
		conditions = append(conditions, fmt.Sprintf(`
EXISTS (
	SELECT 1
	FROM projection_turmas t
	WHERE t.deleted_at IS NULL
	  AND t.codigo_academia = %s.codigo_academia
	  AND t.codigo_turma = ANY($%d)
	  AND EXISTS (
		  SELECT 1
		  FROM jsonb_array_elements_text(
			  COALESCE(t.historico_estudantes_ano_letivo -> %s.ano_lectivo, '[]'::jsonb)
		  ) AS est(codigo)
		  WHERE est.codigo = %s.codigo_estudante
	  )
)`, alias, len(args), alias, alias))
	}

	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}
