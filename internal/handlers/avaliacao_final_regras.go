package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"

	"spuri/internal/middleware"
	"spuri/internal/utils"
)

type regraAvaliacaoFinalDTO struct {
	ID                      uuid.UUID       `json:"id"`
	CodigoAcademia          string          `json:"codigo_academia"`
	Type                    string          `json:"type"`
	Nome                    string          `json:"nome"`
	Descricao               *string         `json:"descricao,omitempty"`
	TipoEnsino              string          `json:"tipo_ensino"`
	AnosAcademicos          []string        `json:"anos_academicos"`
	NotaMinimaAprovacao     float64         `json:"nota_minima_aprovacao"`
	CategoriasEnvolvidas    []string        `json:"categorias_envolvidas"`
	Formula                 json.RawMessage `json:"formula"`
	AplicaSeReprovadoEmType *string         `json:"aplica_se_reprovado_em_type,omitempty"`
	Status                  string          `json:"status"`
	Version                 int             `json:"version"`
}

type formulaNode struct {
	Op         string          `json:"op"`
	Left       *formulaNode    `json:"left"`
	Right      json.RawMessage `json:"right"`
	Items      []formulaNode   `json:"items"`
	Categories []string        `json:"categories"`
	Category   string          `json:"category"`
	Periods    []string        `json:"periods"`
}

func CriarRegraAvaliacaoFinal(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	var req regraAvaliacaoFinalDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campos obrigatórios: nome, tipo_ensino, anos_academicos, nota_minima_aprovacao, categorias_envolvidas, formula"))
		return
	}
	if strings.TrimSpace(req.Type) == "" {
		req.Type = "normal"
	}
	if strings.TrimSpace(req.Nome) == "" || req.NotaMinimaAprovacao <= 0 || len(req.AnosAcademicos) == 0 || len(req.CategoriasEnvolvidas) == 0 || len(req.Formula) == 0 {
		utils.RespondWithValidationError(c, fmt.Errorf("regra de avaliação final incompleta"))
		return
	}
	if req.TipoEnsino != "fundamental" && req.TipoEnsino != "medio" && req.TipoEnsino != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo_ensino deve ser fundamental, medio ou superior"))
		return
	}
	if req.AplicaSeReprovadoEmType != nil && *req.AplicaSeReprovadoEmType == req.Type {
		utils.RespondWithValidationError(c, fmt.Errorf("aplica_se_reprovado_em_type não pode apontar para o próprio type"))
		return
	}
	if err := validarFormulaAvaliacao(req.Formula, req.CategoriasEnvolvidas); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	id := uuid.New()
	_, err = getDbClient(c).DB().Exec(`INSERT INTO projection_regras_avaliacao_final (id,codigo_academia,type,nome,descricao,tipo_ensino,anos_academicos,nota_minima_aprovacao,categorias_envolvidas,formula,aplica_se_reprovado_em_type,status,version) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'ativo',1)`, id, academiaDTO.CodigoAcademia, req.Type, req.Nome, req.Descricao, req.TipoEnsino, toJSON(req.AnosAcademicos), req.NotaMinimaAprovacao, toJSON(req.CategoriasEnvolvidas), req.Formula, req.AplicaSeReprovadoEmType)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("erro ao criar regra (verifique duplicidade de type/tipo_ensino ativo): %w", err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "regra de avaliação final criada", "id": id})
}

func ListarRegrasAvaliacaoFinal(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	rows, err := getDbClient(c).DB().Query(`SELECT id,codigo_academia,type,nome,descricao,tipo_ensino,anos_academicos,nota_minima_aprovacao,categorias_envolvidas,formula,aplica_se_reprovado_em_type,status,version FROM projection_regras_avaliacao_final WHERE codigo_academia=$1 ORDER BY created_at DESC`, academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()
	out := []regraAvaliacaoFinalDTO{}
	for rows.Next() {
		r, err := scanRegra(rows)
		if err == nil {
			out = append(out, r)
		}
	}
	c.JSON(http.StatusOK, gin.H{"regras": out, "total": len(out)})
}

func getRegraAvaliacaoFinal(c *gin.Context, codigoAcademia, tipoEnsino, anoAcademico, typ string) (*regraAvaliacaoFinalDTO, error) {
	if typ == "" {
		typ = "normal"
	}
	rows, err := getDbClient(c).DB().Query(`SELECT id,codigo_academia,type,nome,descricao,tipo_ensino,anos_academicos,nota_minima_aprovacao,categorias_envolvidas,formula,aplica_se_reprovado_em_type,status,version FROM projection_regras_avaliacao_final WHERE codigo_academia=$1 AND tipo_ensino=$2 AND type=$3 AND status='ativo' AND anos_academicos ? $4`, codigoAcademia, tipoEnsino, typ, anoAcademico)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found *regraAvaliacaoFinalDTO
	for rows.Next() {
		r, err := scanRegra(rows)
		if err != nil {
			return nil, err
		}
		if found != nil {
			return nil, fmt.Errorf("mais de uma regra ativa aplicável")
		}
		found = &r
	}
	if found == nil {
		return nil, fmt.Errorf("nenhuma regra ativa de avaliação final encontrada para type=%s tipo_ensino=%s ano=%s", typ, tipoEnsino, anoAcademico)
	}
	return found, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanRegra(rows rowScanner) (regraAvaliacaoFinalDTO, error) {
	var r regraAvaliacaoFinalDTO
	var anos, cats []byte
	var formula []byte
	err := rows.Scan(&r.ID, &r.CodigoAcademia, &r.Type, &r.Nome, &r.Descricao, &r.TipoEnsino, &anos, &r.NotaMinimaAprovacao, &cats, &formula, &r.AplicaSeReprovadoEmType, &r.Status, &r.Version)
	_ = json.Unmarshal(anos, &r.AnosAcademicos)
	_ = json.Unmarshal(cats, &r.CategoriasEnvolvidas)
	r.Formula = json.RawMessage(formula)
	return r, err
}
func toJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func validarFormulaAvaliacao(raw json.RawMessage, categorias []string) error {
	var n formulaNode
	if err := json.Unmarshal(raw, &n); err != nil {
		return fmt.Errorf("formula inválida: %w", err)
	}
	allowed := map[string]bool{}
	for _, c := range categorias {
		allowed[c] = true
	}
	return validarNode(n, allowed)
}
func validarNode(n formulaNode, cats map[string]bool) error {
	switch n.Op {
	case "add":
		if len(n.Items) == 0 {
			return fmt.Errorf("add exige items")
		}
		for _, it := range n.Items {
			if err := validarNode(it, cats); err != nil {
				return err
			}
		}
	case "div":
		if n.Left == nil || len(n.Right) == 0 {
			return fmt.Errorf("div exige left/right")
		}
		var f float64
		if err := json.Unmarshal(n.Right, &f); err != nil || f == 0 {
			return fmt.Errorf("div right deve ser constante numérica diferente de zero")
		}
		return validarNode(*n.Left, cats)
	case "sum_periods":
		if len(n.Categories) == 0 || len(n.Periods) == 0 {
			return fmt.Errorf("sum_periods exige categories e periods")
		}
		for _, c := range n.Categories {
			if !cats[c] {
				return fmt.Errorf("categoria %s não está em categorias_envolvidas", c)
			}
		}
	case "category_total":
		if n.Category == "" || !cats[n.Category] {
			return fmt.Errorf("category_total usa categoria inválida")
		}
	default:
		return fmt.Errorf("operação de fórmula não suportada: %s", n.Op)
	}
	return nil
}

func calcularFormulaAvaliacao(raw json.RawMessage, notas map[string]map[string][]float64) (float64, error) {
	var n formulaNode
	if err := json.Unmarshal(raw, &n); err != nil {
		return 0, err
	}
	return evalNode(n, notas)
}
func evalNode(n formulaNode, notas map[string]map[string][]float64) (float64, error) {
	switch n.Op {
	case "add":
		var s float64
		for _, it := range n.Items {
			v, e := evalNode(it, notas)
			if e != nil {
				return 0, e
			}
			s += v
		}
		return s, nil
	case "div":
		v, e := evalNode(*n.Left, notas)
		if e != nil {
			return 0, e
		}
		var d float64
		_ = json.Unmarshal(n.Right, &d)
		if d == 0 {
			return 0, fmt.Errorf("divisão por zero")
		}
		return v / d, nil
	case "sum_periods":
		var s float64
		for _, p := range n.Periods {
			for _, c := range n.Categories {
				vals := notas[c][p]
				if len(vals) == 0 {
					return 0, fmt.Errorf("nota ausente: categoria=%s periodo=%s", c, p)
				}
				for _, v := range vals {
					s += v
				}
			}
		}
		return s, nil
	case "category_total":
		var s float64
		if len(notas[n.Category]) == 0 {
			return 0, fmt.Errorf("nota ausente: categoria=%s", n.Category)
		}
		for _, vals := range notas[n.Category] {
			for _, v := range vals {
				s += v
			}
		}
		return s, nil
	}
	return 0, fmt.Errorf("operação não suportada")
}

func carregarNotasFormula(c *gin.Context, codigoEstudante, codigoAcademia, anoLectivo string, categorias []string) (map[string]map[string][]float64, error) {
	rows, err := getDbClient(c).DB().Query(`SELECT categoria,periodo,nota FROM projection_notas WHERE codigo_estudante=$1 AND codigo_academia=$2 AND ano_lectivo=$3 AND categoria=ANY($4) AND deleted_at IS NULL`, codigoEstudante, codigoAcademia, anoLectivo, pq.Array(categorias))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string][]float64{}
	for rows.Next() {
		var cat, per string
		var nota float64
		if err := rows.Scan(&cat, &per, &nota); err != nil {
			return nil, err
		}
		if out[cat] == nil {
			out[cat] = map[string][]float64{}
		}
		out[cat][per] = append(out[cat][per], nota)
	}
	return out, rows.Err()
}
func _sqlNoRows(err error) bool { return err == sql.ErrNoRows }
