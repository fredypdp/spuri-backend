package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"

	"spuri/internal/middleware"
	"spuri/internal/utils"
)

type materiaAplicavelEscopo struct {
	CursoID      *string     `json:"curso_id,omitempty"`
	AnoAcademico string      `json:"ano_academico"`
	Materias     []uuid.UUID `json:"materias"`
}

type regraAvaliacaoFinalDTO struct {
	ID                       uuid.UUID                `json:"id"`
	CodigoAcademia           string                   `json:"codigo_academia"`
	Type                     string                   `json:"type" binding:"required"`
	Nome                     string                   `json:"nome" binding:"required"`
	Descricao                *string                  `json:"descricao,omitempty"`
	Nivel                    string                   `json:"nivel,omitempty"`
	AnosAcademicos           []string                 `json:"anos_academicos,omitempty"`
	NotaMinimaAprovacao      float64                  `json:"nota_minima_aprovacao" binding:"required"`
	CategoriasEnvolvidas     []string                 `json:"categorias_envolvidas,omitempty"`
	Formula                  string                   `json:"formula" binding:"required"`
	MateriasAplicaveis       []uuid.UUID              `json:"materias_aplicaveis,omitempty"`
	MateriasAplicaveisEscopo []materiaAplicavelEscopo `json:"-"`
	LimiteMateriasPendentes  *int                     `json:"limite_materias_pendentes,omitempty"`
	AplicaSeReprovadoEmType  *string                  `json:"aplica_se_reprovado_em_type,omitempty"`
	Status                   string                   `json:"status"`
	Version                  int                      `json:"version"`
	NotaDespertadora         *string                  `json:"nota_despertadora,omitempty"`
	Source                   string                   `json:"source,omitempty"`
	Fixed                    bool                     `json:"fixed,omitempty"`
	Readonly                 bool                     `json:"readonly,omitempty"`
}

// UnmarshalJSON preserves the legacy public contract for fundamental rules
// (anos_academicos as []string), while accepting the new medio contract
// (items grouped by curso_id). Internally the medio scope is flattened as
// "<curso_id>|<ano_academico>" so existing rule-chain and uniqueness helpers
// can compare scopes deterministically until the projection is fully
// decomposed into an auxiliary scope table.
func (r *regraAvaliacaoFinalDTO) UnmarshalJSON(b []byte) error {
	type alias regraAvaliacaoFinalDTO
	var raw struct {
		*alias
		AnosAcademicos     json.RawMessage `json:"anos_academicos"`
		MateriasAplicaveis json.RawMessage `json:"materias_aplicaveis"`
	}
	raw.alias = (*alias)(r)
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw.AnosAcademicos) > 0 && string(raw.AnosAcademicos) != "null" {
		var anos []string
		if err := json.Unmarshal(raw.AnosAcademicos, &anos); err == nil {
			r.AnosAcademicos = anos
		} else {
			var porCurso []struct {
				CursoID        string   `json:"curso_id"`
				AnosAcademicos []string `json:"anos_academicos"`
			}
			if err := json.Unmarshal(raw.AnosAcademicos, &porCurso); err != nil {
				return fmt.Errorf("anos_academicos deve ser array de strings para fundamental ou array de objetos por curso para medio")
			}
			r.AnosAcademicos = r.AnosAcademicos[:0]
			cursosVistos := map[string]bool{}
			for _, item := range porCurso {
				cursoID := strings.TrimSpace(item.CursoID)
				if cursoID == "" {
					return fmt.Errorf("anos_academicos de médio exige curso_id")
				}
				if cursosVistos[cursoID] {
					return fmt.Errorf("anos_academicos de médio não aceita dois itens para o mesmo curso_id: %s", cursoID)
				}
				cursosVistos[cursoID] = true
				anosVistos := map[string]bool{}
				for _, ano := range item.AnosAcademicos {
					ano = strings.TrimSpace(ano)
					if ano == "" {
						return fmt.Errorf("anos_academicos de médio não pode conter ano vazio")
					}
					if anosVistos[ano] {
						return fmt.Errorf("ano_academico duplicado em anos_academicos do curso_id %s: %s", cursoID, ano)
					}
					anosVistos[ano] = true
					r.AnosAcademicos = append(r.AnosAcademicos, cursoID+"|"+ano)
				}
			}
		}
	}
	if len(raw.MateriasAplicaveis) > 0 && string(raw.MateriasAplicaveis) != "null" {
		var porEscopo []materiaAplicavelEscopo
		if err := json.Unmarshal(raw.MateriasAplicaveis, &porEscopo); err != nil {
			return fmt.Errorf("materias_aplicaveis deve ser lista de objetos por escopo; lista simples global de IDs não é aceita")
		}
		r.MateriasAplicaveis = r.MateriasAplicaveis[:0]
		r.MateriasAplicaveisEscopo = porEscopo
		for _, item := range porEscopo {
			r.MateriasAplicaveis = append(r.MateriasAplicaveis, item.Materias...)
		}
	}
	return nil
}

func CriarRegraAvaliacaoFinal(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	body, _ := c.GetRawData()
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	if jsonCampoPresente(body, "tipo_ensino") {
		utils.RespondWithValidationError(c, fmt.Errorf("campo legado tipo_ensino não é aceito; use nivel"))
		return
	}
	if jsonCampoPresente(body, "materias_chave") {
		utils.RespondWithValidationError(c, fmt.Errorf("materias_chave não é aceito em regras de avaliação final; configure matérias-chave no curso médio, por ano_academico"))
		return
	}
	var req regraAvaliacaoFinalDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campos obrigatórios: type, nome, nivel, nota_minima_aprovacao, formula"))
		return
	}
	if err := preencherValidarNivelRegraAcademia(&req, academiaDTO.Nivel, academiaDTO.NivelEscolar); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if req.Nivel != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("regras de avaliação final escolares são fixas do sistema; configure regras apenas para ensino superior"))
		return
	}
	if strings.TrimSpace(req.Type) == "" || strings.TrimSpace(req.Nome) == "" || req.NotaMinimaAprovacao <= 0 || strings.TrimSpace(req.Formula) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("regra de avaliação final incompleta"))
		return
	}
	req.Type, err = normalizarTypeRegraAvaliacaoFinal(req.Type)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if req.AplicaSeReprovadoEmType != nil {
		if strings.TrimSpace(*req.AplicaSeReprovadoEmType) == "" {
			req.AplicaSeReprovadoEmType = nil
		} else {
			dependeDe, err := normalizarTypeRegraAvaliacaoFinal(*req.AplicaSeReprovadoEmType)
			if err != nil {
				utils.RespondWithValidationError(c, fmt.Errorf("aplica_se_reprovado_em_type inválido: %w", err))
				return
			}
			req.AplicaSeReprovadoEmType = &dependeDe
		}
	}
	if req.Nivel != "fundamental" && req.Nivel != "medio" && req.Nivel != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("nivel deve ser fundamental, medio ou superior"))
		return
	}
	if err := validarCamposPorNivelRegraAvaliacaoFinal(req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarEscopoRegraAvaliacaoFinal(req.Nivel, req.AnosAcademicos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarAnosOfertadosRegraAvaliacaoFinal(req.Nivel, req.AnosAcademicos, academiaDTO.AnosAcademicos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarCursosEscopoRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, req.Nivel, req.AnosAcademicos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarMateriasAplicaveisRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, req.Nivel, req.AnosAcademicos, req.MateriasAplicaveisEscopo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if req.AplicaSeReprovadoEmType != nil && *req.AplicaSeReprovadoEmType == req.Type {
		utils.RespondWithValidationError(c, fmt.Errorf("aplica_se_reprovado_em_type não pode apontar para o próprio type"))
		return
	}
	if err := validarUnicidadeRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, req.Nivel, req.Type, req.AnosAcademicos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarRaizUnicaRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, req.Nivel, req.Type, req.AnosAcademicos, req.AplicaSeReprovadoEmType); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarDependenciaRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, req.Nivel, req.Type, req.AnosAcademicos, req.AplicaSeReprovadoEmType); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	formulaNormalizada, categoriasFormula, err := validarFormulaAvaliacaoPorNivel(req.Nivel, req.Formula, req.CategoriasEnvolvidas)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	req.CategoriasEnvolvidas = categoriasFormula
	if err := validarCategoriasRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, req.AnosAcademicos, req.CategoriasEnvolvidas); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarNotaDespertadoraRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, req.AnosAcademicos, req.AplicaSeReprovadoEmType, req.NotaDespertadora); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	id := uuid.New()
	_, err = getDbClient(c).DB().Exec(`INSERT INTO projection_regras_avaliacao_final (id,codigo_academia,type,nome,descricao,nivel,anos_academicos,nota_minima_aprovacao,categorias_envolvidas,formula,aplica_se_reprovado_em_type,materias_aplicaveis,limite_materias_pendentes,nota_despertadora,status,version) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'ativo',1)`, id, academiaDTO.CodigoAcademia, req.Type, req.Nome, req.Descricao, req.Nivel, toJSON(req.AnosAcademicos), req.NotaMinimaAprovacao, toJSON(req.CategoriasEnvolvidas), formulaNormalizada, req.AplicaSeReprovadoEmType, toJSON(uuidStrings(req.MateriasAplicaveis)), req.LimiteMateriasPendentes, req.NotaDespertadora)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("erro ao criar regra de avaliação final: %w", err))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "regra de avaliação final criada", "id": id})
}

func jsonCampoPresente(body []byte, campo string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return false
	}
	_, ok := raw[campo]
	return ok
}

func preencherValidarNivelRegraAcademia(req *regraAvaliacaoFinalDTO, nivelAcademia string, nivelEscolar *string) error {
	nivelAcademia = strings.TrimSpace(strings.ToLower(nivelAcademia))
	nivelReq := strings.TrimSpace(strings.ToLower(req.Nivel))
	if nivelAcademia == "superior" {
		if nivelReq != "" && nivelReq != "superior" {
			return fmt.Errorf("nivel incompatível com academia superior")
		}
		req.Nivel = "superior"
		return nil
	}
	if nivelEscolar == nil || strings.TrimSpace(*nivelEscolar) == "" {
		return fmt.Errorf("academia escolar sem nivel_escolar configurado")
	}
	escolar := strings.TrimSpace(strings.ToLower(*nivelEscolar))
	if escolar == "misto" || escolar == "fundamental_medio" {
		if nivelReq != "fundamental" && nivelReq != "medio" {
			return fmt.Errorf("academia mista deve informar nivel fundamental ou medio")
		}
		req.Nivel = nivelReq
		return nil
	}
	if nivelReq != "" && nivelReq != escolar {
		return fmt.Errorf("nivel incompatível com nivel_escolar da academia")
	}
	req.Nivel = escolar
	return nil
}

func validarCamposPorNivelRegraAvaliacaoFinal(req regraAvaliacaoFinalDTO) error {
	if req.Nivel == "fundamental" {
		if len(req.AnosAcademicos) == 0 {
			return fmt.Errorf("anos_academicos é obrigatório para regras de nivel fundamental")
		}
		for _, ano := range req.AnosAcademicos {
			if strings.Contains(ano, "|") {
				return fmt.Errorf("anos_academicos de regra fundamental deve ser array simples de strings; não envie escopo por curso")
			}
		}
		if req.LimiteMateriasPendentes != nil {
			return fmt.Errorf("limite_materias_pendentes não é aceito para nivel fundamental")
		}
		return nil
	}
	if req.Nivel == "medio" {
		if len(req.AnosAcademicos) == 0 {
			return fmt.Errorf("anos_academicos é obrigatório para regras de nivel medio e deve ser agrupado por curso")
		}
		for _, escopo := range req.AnosAcademicos {
			if !strings.Contains(escopo, "|") {
				return fmt.Errorf("anos_academicos de regra medio deve usar objetos com curso_id e anos_academicos; array simples legado não é aceito")
			}
		}
	} else if len(req.AnosAcademicos) > 0 {
		return fmt.Errorf("anos_academicos não é aceito para nivel superior")
	}
	if req.LimiteMateriasPendentes == nil {
		return fmt.Errorf("limite_materias_pendentes é obrigatório para nivel medio ou superior")
	}
	if *req.LimiteMateriasPendentes < 0 {
		return fmt.Errorf("limite_materias_pendentes não pode ser negativo")
	}
	return nil
}

func normalizarTypeRegraAvaliacaoFinal(typ string) (string, error) {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		return "", fmt.Errorf("type é obrigatório")
	}

	var b strings.Builder
	ultimoUnderscore := false
	for _, r := range typ {
		switch {
		case unicode.IsSpace(r):
			if !ultimoUnderscore {
				b.WriteRune('_')
				ultimoUnderscore = true
			}
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			ultimoUnderscore = false
		case r == '_':
			b.WriteRune(r)
			ultimoUnderscore = true
		default:
			return "", fmt.Errorf("type deve conter apenas letras, números, espaços ou underscore; espaços são convertidos para '_'")
		}
	}

	normalizado := strings.Trim(b.String(), "_")
	if normalizado == "" {
		return "", fmt.Errorf("type deve conter pelo menos uma letra ou número")
	}
	return normalizado, nil
}

func ListarRegrasAvaliacaoFinal(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	rows, err := getDbClient(c).DB().Query(`SELECT id,codigo_academia,type,nome,descricao,nivel,anos_academicos,nota_minima_aprovacao,categorias_envolvidas,formula,aplica_se_reprovado_em_type,materias_aplicaveis,limite_materias_pendentes,status,version,nota_despertadora FROM projection_regras_avaliacao_final WHERE codigo_academia=$1 ORDER BY created_at DESC`, academiaDTO.CodigoAcademia)
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
	if academiaDTO.Nivel == "escola" {
		for _, ano := range academiaDTO.AnosAcademicos {
			tipo := "fundamental"
			if isAnoMedio(ano) {
				tipo = "medio"
			}
			out = append(out, regrasAvaliacaoFinalEscolaresFixas(c, academiaDTO.CodigoAcademia, tipo, ano, nil, nil)...)
		}
	}
	c.JSON(http.StatusOK, gin.H{"regras": out, "total": len(out)})
}

type editarRegraAvaliacaoFinalDTO struct {
	Nome                 string   `json:"nome" binding:"required"`
	Descricao            *string  `json:"descricao,omitempty"`
	NotaMinimaAprovacao  float64  `json:"nota_minima_aprovacao" binding:"required"`
	Formula              string   `json:"formula" binding:"required"`
	CategoriasEnvolvidas []string `json:"categorias_envolvidas,omitempty"`
	NotaDespertadora     *string  `json:"nota_despertadora,omitempty"`
}

func EditarRegraAvaliacaoFinal(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}
	body, _ := c.GetRawData()
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	if jsonCampoPresente(body, "tipo_ensino") {
		utils.RespondWithValidationError(c, fmt.Errorf("campo legado tipo_ensino não é aceito; use nivel"))
		return
	}
	if jsonCampoPresente(body, "materias_chave") {
		utils.RespondWithValidationError(c, fmt.Errorf("materias_chave não é aceito em regras de avaliação final; configure matérias-chave no curso médio, por ano_academico"))
		return
	}
	if jsonCampoPresente(body, "nivel") || jsonCampoPresente(body, "anos_academicos") {
		utils.RespondWithValidationError(c, fmt.Errorf("nivel e anos_academicos não podem ser alterados; crie uma nova regra para mudar o escopo"))
		return
	}
	var req editarRegraAvaliacaoFinalDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("campos obrigatórios: nome, nota_minima_aprovacao, formula"))
		return
	}
	if strings.TrimSpace(req.Nome) == "" || req.NotaMinimaAprovacao <= 0 || strings.TrimSpace(req.Formula) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("edição de regra de avaliação final incompleta"))
		return
	}
	regra, err := getRegraAvaliacaoFinalPorID(c, academiaDTO.CodigoAcademia, id)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if regra == nil {
		utils.RespondWithNotFoundError(c, "regra de avaliação final")
		return
	}
	if regra.Nivel != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("regras de avaliação final escolares são fixas do sistema e não podem ser editadas"))
		return
	}
	if regra.Status != "ativo" {
		utils.RespondWithValidationError(c, fmt.Errorf("somente regras ativas podem ser editadas"))
		return
	}
	formulaNormalizada, categoriasFormula, err := validarFormulaAvaliacaoPorNivel(regra.Nivel, req.Formula, req.CategoriasEnvolvidas)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarCategoriasRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, regra.AnosAcademicos, categoriasFormula); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := validarNotaDespertadoraRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, regra.AnosAcademicos, regra.AplicaSeReprovadoEmType, req.NotaDespertadora); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	notaDespertadora := regra.NotaDespertadora
	if req.NotaDespertadora != nil {
		notaDespertadora = req.NotaDespertadora
	}
	res, err := getDbClient(c).DB().Exec(`UPDATE projection_regras_avaliacao_final
		SET nome=$1, descricao=$2, nota_minima_aprovacao=$3, categorias_envolvidas=$4, formula=$5, nota_despertadora=$6, updated_at=CURRENT_TIMESTAMP, version=version+1
		WHERE id=$7 AND codigo_academia=$8 AND status='ativo'`,
		strings.TrimSpace(req.Nome), req.Descricao, req.NotaMinimaAprovacao, toJSON(categoriasFormula), formulaNormalizada, notaDespertadora, id, academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("erro ao editar regra de avaliação final: %w", err))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		utils.RespondWithNotFoundError(c, "regra de avaliação final")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "regra de avaliação final atualizada", "id": id})
}

func DeletarRegraAvaliacaoFinal(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}
	regra, err := getRegraAvaliacaoFinalPorID(c, academiaDTO.CodigoAcademia, id)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if regra == nil {
		utils.RespondWithNotFoundError(c, "regra de avaliação final")
		return
	}
	if regra.Nivel != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("regras de avaliação final escolares são fixas do sistema e não podem ser removidas"))
		return
	}
	if regra.Status != "ativo" {
		utils.RespondWithValidationError(c, fmt.Errorf("regra de avaliação final já está inativa"))
		return
	}
	ids, err := idsCadeiaDependenteRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, regra.Nivel, regra.Type, regra.AnosAcademicos)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if len(ids) == 0 {
		ids = []uuid.UUID{id}
	}
	_, err = getDbClient(c).DB().Exec(`UPDATE projection_regras_avaliacao_final
		SET status='inativo', updated_at=CURRENT_TIMESTAMP, version=version+1
		WHERE codigo_academia=$1 AND id = ANY($2::uuid[]) AND status='ativo'`, academiaDTO.CodigoAcademia, pq.Array(uuidStrings(ids)))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("erro ao deletar regra de avaliação final: %w", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "regra de avaliação final inativada com dependentes", "id": id, "total_inativadas": len(ids)})
}

func validarEscopoRegraAvaliacaoFinal(tipoEnsino string, niveis []string) error {
	vistos := map[string]bool{}
	for _, nivel := range niveis {
		nivel = strings.TrimSpace(nivel)
		if nivel == "" {
			return fmt.Errorf("anos_academicos não pode conter valores vazios")
		}
		if vistos[nivel] {
			return fmt.Errorf("ano_academico duplicado no escopo da regra: %s", nivel)
		}
		vistos[nivel] = true
		switch tipoEnsino {
		case "fundamental":
			if err := utils.ValidateAnoFundamental(nivel); err != nil {
				return fmt.Errorf("anos_academicos inválido para fundamental: %w", err)
			}
		case "medio":
			partes := strings.Split(nivel, "|")
			if len(partes) != 2 || strings.TrimSpace(partes[0]) == "" {
				return fmt.Errorf("anos_academicos de médio deve informar curso_id e ano_academico")
			}
			if _, err := uuid.Parse(strings.TrimSpace(partes[0])); err != nil {
				return fmt.Errorf("curso_id inválido em anos_academicos de médio")
			}
			if err := utils.ValidateAnoMedio(strings.TrimSpace(partes[1])); err != nil {
				return fmt.Errorf("anos_academicos inválido para médio: %w", err)
			}
		case "superior":
			return fmt.Errorf("anos_academicos não é aceito para nivel superior")
		}
	}
	return nil
}

func validarAnosOfertadosRegraAvaliacaoFinal(nivel string, escopos, anosAcademia []string) error {
	if nivel != "fundamental" {
		return nil
	}
	ofertados := map[string]bool{}
	for _, ano := range anosAcademia {
		ofertados[strings.TrimSpace(ano)] = true
	}
	for _, ano := range escopos {
		if len(ofertados) > 0 && !ofertados[strings.TrimSpace(ano)] {
			return fmt.Errorf("ano_academico fundamental não ofertado pela academia: %s", ano)
		}
	}
	return nil
}

func validarMateriasAplicaveisRegraAvaliacaoFinal(c *gin.Context, codigoAcademia, nivel string, escopos []string, itens []materiaAplicavelEscopo) error {
	if len(itens) == 0 {
		return nil
	}
	escopoOK := map[string]bool{}
	for _, e := range escopos {
		escopoOK[strings.TrimSpace(e)] = true
	}
	vistosItens := map[string]bool{}
	for _, item := range itens {
		ano := strings.TrimSpace(item.AnoAcademico)
		if ano == "" || len(item.Materias) == 0 {
			return fmt.Errorf("materias_aplicaveis deve informar ano_academico e materias não vazio")
		}
		chave := ano
		curso := ""
		if item.CursoID != nil {
			curso = strings.TrimSpace(*item.CursoID)
		}
		if nivel == "fundamental" {
			if curso != "" {
				return fmt.Errorf("materias_aplicaveis fundamental não aceita curso_id")
			}
			if !escopoOK[ano] {
				return fmt.Errorf("materias_aplicaveis ano_academico fora dos anos_academicos da regra: %s", ano)
			}
		} else {
			if curso == "" {
				return fmt.Errorf("materias_aplicaveis de nivel %s exige curso_id", nivel)
			}
			if _, err := uuid.Parse(curso); err != nil {
				return fmt.Errorf("curso_id inválido em materias_aplicaveis")
			}
			chave = curso + "|" + ano
			if nivel == "medio" && !escopoOK[chave] {
				return fmt.Errorf("materias_aplicaveis curso_id/ano_academico fora dos anos_academicos da regra: %s/%s", curso, ano)
			}
		}
		if vistosItens[chave] {
			return fmt.Errorf("item duplicado em materias_aplicaveis para escopo %s", chave)
		}
		vistosItens[chave] = true
		vistosMaterias := map[uuid.UUID]bool{}
		for _, materiaID := range item.Materias {
			if vistosMaterias[materiaID] {
				return fmt.Errorf("materia_id duplicado em materias_aplicaveis: %s", materiaID)
			}
			vistosMaterias[materiaID] = true
			if err := validarMateriaAplicavelEscopo(c, codigoAcademia, nivel, curso, ano, materiaID); err != nil {
				return err
			}
		}
	}
	return nil
}

func validarMateriaAplicavelEscopo(c *gin.Context, codigoAcademia, nivel, curso, ano string, materiaID uuid.UUID) error {
	var existe bool
	if nivel == "fundamental" {
		if err := getDbClient(c).DB().QueryRow(`SELECT EXISTS (SELECT 1 FROM projection_materias WHERE id=$1 AND codigo_academia=$2 AND type='fundamental' AND status='ativo' AND deleted_at IS NULL AND anos_academicos ? $3)`, materiaID, codigoAcademia, ano).Scan(&existe); err != nil {
			return err
		}
	} else if nivel == "medio" {
		if err := getDbClient(c).DB().QueryRow(`SELECT EXISTS (SELECT 1 FROM projection_materias WHERE id=$1 AND codigo_academia=$2 AND type='medio' AND status='ativo' AND deleted_at IS NULL AND curso_id=$3 AND anos_academicos ? $4)`, materiaID, codigoAcademia, curso, ano).Scan(&existe); err != nil {
			return err
		}
	} else {
		if err := getDbClient(c).DB().QueryRow(`SELECT EXISTS (SELECT 1 FROM projection_materias WHERE id=$1 AND codigo_academia=$2 AND type='superior' AND status='ativo' AND deleted_at IS NULL AND curso_id=$3 AND periodo=$4)`, materiaID, codigoAcademia, curso, ano).Scan(&existe); err != nil {
			return err
		}
	}
	if !existe {
		return fmt.Errorf("materia_id %s inválida ou fora do escopo %s/%s/%s", materiaID, nivel, curso, ano)
	}
	return nil
}

func validarCursosEscopoRegraAvaliacaoFinal(c *gin.Context, codigoAcademia, nivel string, escopos []string) error {
	if nivel != "medio" {
		return nil
	}
	for _, escopo := range escopos {
		partes := strings.Split(escopo, "|")
		if len(partes) != 2 {
			return fmt.Errorf("anos_academicos de médio deve informar curso_id e ano_academico")
		}
		cursoID := strings.TrimSpace(partes[0])
		ano := strings.TrimSpace(partes[1])
		var existe bool
		if err := getDbClient(c).DB().QueryRow(`SELECT EXISTS (
			SELECT 1
			FROM projection_cursos
			WHERE id=$1
			  AND codigo_academia=$2
			  AND type='medio'
			  AND status='ativo'
			  AND deleted_at IS NULL
			  AND anos_academicos ? $3
		)`, cursoID, codigoAcademia, ano).Scan(&existe); err != nil {
			return err
		}
		if !existe {
			return fmt.Errorf("curso_id/ano_academico inválido ou fora da academia para regra média: %s/%s", cursoID, ano)
		}
	}
	return nil
}

func escoposRegraParaConsulta(nivel string, anos []string) []string {
	if nivel != "fundamental" && len(anos) == 0 {
		return []string{""}
	}
	return anos
}

func validarUnicidadeRegraAvaliacaoFinal(c *gin.Context, codigoAcademia, tipoEnsino, typ string, anos []string) error {
	for _, ano := range escoposRegraParaConsulta(tipoEnsino, anos) {
		ano = strings.TrimSpace(ano)
		if ano == "" && tipoEnsino == "fundamental" {
			return fmt.Errorf("anos_academicos não pode conter valores vazios")
		}
		var exists bool
		if err := getDbClient(c).DB().QueryRow(`SELECT EXISTS (
			SELECT 1 FROM projection_regras_avaliacao_final
			WHERE codigo_academia=$1 AND nivel=$2 AND type=$3 AND status='ativo' AND (($2 <> 'fundamental' AND $2 <> 'medio') OR ($2 = 'fundamental' AND anos_academicos ? $4) OR ($2 = 'medio' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(anos_academicos) e(v) WHERE e.v = $4)))
		)`, codigoAcademia, tipoEnsino, typ, ano).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("já existe regra ativa de avaliação final para type=%s nivel=%s ano=%s", typ, tipoEnsino, ano)
		}
	}
	return nil
}

func validarRaizUnicaRegraAvaliacaoFinal(c *gin.Context, codigoAcademia, tipoEnsino, typ string, anos []string, dependeDe *string) error {
	if dependeDe != nil && strings.TrimSpace(*dependeDe) != "" {
		return nil
	}
	for _, ano := range escoposRegraParaConsulta(tipoEnsino, anos) {
		ano = strings.TrimSpace(ano)
		var rootType string
		err := getDbClient(c).DB().QueryRow(`SELECT type
			FROM projection_regras_avaliacao_final
			WHERE codigo_academia=$1
			  AND nivel=$2
			  AND status='ativo'
			  AND aplica_se_reprovado_em_type IS NULL
			  AND (($2 <> 'fundamental' AND $2 <> 'medio') OR ($2 = 'fundamental' AND anos_academicos ? $3) OR ($2 = 'medio' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(anos_academicos) e(v) WHERE e.v = $3)))
			LIMIT 1`, codigoAcademia, tipoEnsino, ano).Scan(&rootType)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		if rootType != typ {
			return fmt.Errorf("já existe avaliação final raiz ativa para nivel=%s ano=%s: %s", tipoEnsino, ano, rootType)
		}
	}
	return nil
}

func validarCadeiaAvaliacaoFinalAplicavel(regras []regraAvaliacaoFinalDTO, codigoAcademia, tipoEnsino, anoAcademico string) error {
	raizes := 0
	tipos := map[string]bool{}
	for _, regra := range regras {
		tipos[regra.Type] = true
		if regra.AplicaSeReprovadoEmType == nil || strings.TrimSpace(*regra.AplicaSeReprovadoEmType) == "" {
			raizes++
		}
	}
	if raizes == 0 {
		return fmt.Errorf("nenhuma regra raiz de avaliação final encontrada para academia=%s nivel=%s ano=%s", codigoAcademia, tipoEnsino, anoAcademico)
	}
	if raizes > 1 {
		return fmt.Errorf("mais de uma regra raiz de avaliação final encontrada para academia=%s nivel=%s ano=%s", codigoAcademia, tipoEnsino, anoAcademico)
	}
	for _, regra := range regras {
		if regra.AplicaSeReprovadoEmType == nil || strings.TrimSpace(*regra.AplicaSeReprovadoEmType) == "" {
			continue
		}
		if !tipos[strings.TrimSpace(*regra.AplicaSeReprovadoEmType)] {
			return fmt.Errorf("regra type=%s depende de type fora da cadeia aplicável: %s", regra.Type, *regra.AplicaSeReprovadoEmType)
		}
	}
	return nil
}

func validarDependenciaRegraAvaliacaoFinal(c *gin.Context, codigoAcademia, tipoEnsino, typ string, anos []string, dependeDe *string) error {
	if dependeDe == nil || strings.TrimSpace(*dependeDe) == "" {
		return nil
	}
	visitado := map[string]bool{typ: true}
	atual := strings.TrimSpace(*dependeDe)
	var anosRaiz []byte
	for atual != "" {
		if visitado[atual] {
			return fmt.Errorf("aplica_se_reprovado_em_type cria ciclo em %s", atual)
		}
		visitado[atual] = true
		var prox sql.NullString
		rows, err := getDbClient(c).DB().Query(`SELECT aplica_se_reprovado_em_type, anos_academicos
			FROM projection_regras_avaliacao_final
			WHERE codigo_academia=$1 AND nivel=$2 AND type=$3 AND status='ativo'
			ORDER BY created_at DESC`, codigoAcademia, tipoEnsino, atual)
		if err != nil {
			return err
		}
		var anosAtual []byte
		encontrouEscopo := false
		for rows.Next() {
			var proxCandidato sql.NullString
			var anosCandidato []byte
			if err := rows.Scan(&proxCandidato, &anosCandidato); err != nil {
				rows.Close()
				return err
			}
			var anosRegra []string
			if err := json.Unmarshal(anosCandidato, &anosRegra); err != nil {
				rows.Close()
				return err
			}
			if mesmosAnosAcademicos(anos, anosRegra) {
				prox = proxCandidato
				anosAtual = anosCandidato
				encontrouEscopo = true
				break
			}
		}
		if closeErr := rows.Close(); closeErr != nil {
			return closeErr
		}
		if !encontrouEscopo {
			return fmt.Errorf("aplica_se_reprovado_em_type referencia type inexistente/inativo ou fora do escopo de anos_academicos da cadeia: %s", atual)
		}
		if !prox.Valid {
			anosRaiz = anosAtual
			break
		}
		atual = strings.TrimSpace(prox.String)
	}
	if len(anosRaiz) == 0 {
		return nil
	}
	var raiz []string
	if err := json.Unmarshal(anosRaiz, &raiz); err != nil {
		return err
	}
	if !mesmosAnosAcademicos(anos, raiz) {
		return fmt.Errorf("regra dependente deve usar exatamente os mesmos anos_academicos da regra raiz")
	}
	return nil
}

func mesmosAnosAcademicos(a, b []string) bool {
	ma := map[string]bool{}
	for _, v := range a {
		v = strings.TrimSpace(v)
		if v != "" {
			ma[v] = true
		}
	}
	mb := map[string]bool{}
	for _, v := range b {
		v = strings.TrimSpace(v)
		if v != "" {
			mb[v] = true
		}
	}
	if len(ma) != len(mb) {
		return false
	}
	for v := range ma {
		if !mb[v] {
			return false
		}
	}
	return true
}

func getRegraAvaliacaoFinal(c *gin.Context, codigoAcademia, tipoEnsino, anoAcademico, typ string) (*regraAvaliacaoFinalDTO, error) {
	if typ == "" {
		typ = "normal"
	}
	if regra := regraAvaliacaoFinalEscolarFixa(codigoAcademia, tipoEnsino, anoAcademico, typ, nil, ""); regra != nil {
		return regra, nil
	}
	rows, err := getDbClient(c).DB().Query(`SELECT id,codigo_academia,type,nome,descricao,nivel,anos_academicos,nota_minima_aprovacao,categorias_envolvidas,formula,aplica_se_reprovado_em_type,materias_aplicaveis,limite_materias_pendentes,status,version,nota_despertadora FROM projection_regras_avaliacao_final WHERE codigo_academia=$1 AND nivel=$2 AND type=$3 AND status='ativo' AND (($2 <> 'fundamental' AND $2 <> 'medio') OR ($2 = 'fundamental' AND anos_academicos ? $4) OR ($2 = 'medio' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(anos_academicos) e(v) WHERE split_part(e.v, '|', 2) = $4)))`, codigoAcademia, tipoEnsino, typ, anoAcademico)
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
		return nil, fmt.Errorf("nenhuma regra ativa de avaliação final encontrada para type=%s nivel=%s ano=%s", typ, tipoEnsino, anoAcademico)
	}
	return found, nil
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func getRegraAvaliacaoFinalPorID(c *gin.Context, codigoAcademia string, id uuid.UUID) (*regraAvaliacaoFinalDTO, error) {
	row := getDbClient(c).DB().QueryRow(`SELECT id,codigo_academia,type,nome,descricao,nivel,anos_academicos,nota_minima_aprovacao,categorias_envolvidas,formula,aplica_se_reprovado_em_type,materias_aplicaveis,limite_materias_pendentes,status,version,nota_despertadora
		FROM projection_regras_avaliacao_final
		WHERE id=$1 AND codigo_academia=$2`, id, codigoAcademia)
	r, err := scanRegra(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func idsCadeiaDependenteRegraAvaliacaoFinal(c *gin.Context, codigoAcademia, tipoEnsino, rootType string, rootAnos []string) ([]uuid.UUID, error) {
	rows, err := getDbClient(c).DB().Query(`SELECT id,codigo_academia,type,nome,descricao,nivel,anos_academicos,nota_minima_aprovacao,categorias_envolvidas,formula,aplica_se_reprovado_em_type,materias_aplicaveis,limite_materias_pendentes,status,version,nota_despertadora
		FROM projection_regras_avaliacao_final
		WHERE codigo_academia=$1 AND nivel=$2 AND status='ativo'`, codigoAcademia, tipoEnsino)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var regras []regraAvaliacaoFinalDTO
	for rows.Next() {
		r, err := scanRegra(rows)
		if err != nil {
			return nil, err
		}
		if mesmosAnosAcademicos(r.AnosAcademicos, rootAnos) {
			regras = append(regras, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	selecionados := map[string]bool{rootType: true}
	for mudou := true; mudou; {
		mudou = false
		for _, r := range regras {
			if r.AplicaSeReprovadoEmType == nil {
				continue
			}
			if selecionados[strings.TrimSpace(*r.AplicaSeReprovadoEmType)] && !selecionados[r.Type] {
				selecionados[r.Type] = true
				mudou = true
			}
		}
	}
	var ids []uuid.UUID
	for _, r := range regras {
		if selecionados[r.Type] {
			ids = append(ids, r.ID)
		}
	}
	return ids, nil
}

func listarRegrasAvaliacaoFinalAplicaveis(c *gin.Context, codigoAcademia, tipoEnsino, anoAcademico string, categoria *string, cursoID *string) ([]regraAvaliacaoFinalDTO, error) {
	if regras := regrasAvaliacaoFinalEscolaresFixas(c, codigoAcademia, tipoEnsino, anoAcademico, categoria, cursoID); len(regras) > 0 {
		return regras, nil
	}
	query := `SELECT id,codigo_academia,type,nome,descricao,nivel,anos_academicos,nota_minima_aprovacao,categorias_envolvidas,formula,aplica_se_reprovado_em_type,materias_aplicaveis,limite_materias_pendentes,status,version,nota_despertadora
		FROM projection_regras_avaliacao_final
		WHERE codigo_academia=$1
		  AND nivel=$2
		  AND status='ativo'
		  AND (($2 <> 'fundamental' AND $2 <> 'medio') OR ($2 = 'fundamental' AND anos_academicos ? $3) OR ($2 = 'medio' AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(anos_academicos) e(v) WHERE split_part(e.v, '|', 2) = $3)))`
	args := []interface{}{codigoAcademia, tipoEnsino, anoAcademico}
	if tipoEnsino == "medio" && cursoID != nil && strings.TrimSpace(*cursoID) != "" {
		args = append(args, strings.TrimSpace(*cursoID)+"|"+strings.TrimSpace(anoAcademico))
		query += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(anos_academicos) e(v) WHERE e.v = $%d)", len(args))
	}
	if categoria != nil && strings.TrimSpace(*categoria) != "" {
		args = append(args, strings.TrimSpace(*categoria))
		query += fmt.Sprintf(" AND ((aplica_se_reprovado_em_type IS NULL AND nota_despertadora = $%d) OR aplica_se_reprovado_em_type IS NOT NULL)", len(args))
	}
	query += ` ORDER BY CASE WHEN aplica_se_reprovado_em_type IS NULL THEN 0 ELSE 1 END, type`

	rows, err := getDbClient(c).DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []regraAvaliacaoFinalDTO{}
	for rows.Next() {
		r, err := scanRegra(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanRegra(rows rowScanner) (regraAvaliacaoFinalDTO, error) {
	var r regraAvaliacaoFinalDTO
	var anos, cats, materiasAplicaveis []byte
	var limitePendentes sql.NullInt64
	var formula string
	err := rows.Scan(&r.ID, &r.CodigoAcademia, &r.Type, &r.Nome, &r.Descricao, &r.Nivel, &anos, &r.NotaMinimaAprovacao, &cats, &formula, &r.AplicaSeReprovadoEmType, &materiasAplicaveis, &limitePendentes, &r.Status, &r.Version, &r.NotaDespertadora)
	_ = json.Unmarshal(anos, &r.AnosAcademicos)
	_ = json.Unmarshal(cats, &r.CategoriasEnvolvidas)
	r.MateriasAplicaveis = uuidListFromJSON(materiasAplicaveis)
	if limitePendentes.Valid {
		v := int(limitePendentes.Int64)
		r.LimiteMateriasPendentes = &v
	}
	r.Formula = formula
	return r, err
}
func toJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func uuidListFromJSON(b []byte) []uuid.UUID {
	var ss []string
	_ = json.Unmarshal(b, &ss)
	out := make([]uuid.UUID, 0, len(ss))
	for _, s := range ss {
		if id, err := uuid.Parse(s); err == nil {
			out = append(out, id)
		}
	}
	return out
}

type formulaASTKind int

const (
	formulaASTNumber formulaASTKind = iota
	formulaASTReference
	formulaASTBinary
)

type formulaAST struct {
	Kind      formulaASTKind
	Value     float64
	Categoria string
	Periodo   string
	Op        rune
	Left      *formulaAST
	Right     *formulaAST
}

type formulaParser struct {
	input string
	pos   int
}

const maxFormulaAvaliacaoLen = 1000

func validarCategoriasRegraAvaliacaoFinal(c *gin.Context, codigoAcademia string, anosAcademicos, categorias []string) error {
	vistos := map[string]bool{}
	for _, cat := range categorias {
		cat = strings.TrimSpace(cat)
		if cat == "" {
			return fmt.Errorf("categorias_envolvidas não pode conter valores vazios")
		}
		if vistos[cat] {
			return fmt.Errorf("categoria duplicada em categorias_envolvidas: %s", cat)
		}
		vistos[cat] = true
	}

	categoriasProj := getCategoriasNotaProjection(c)
	categoriasAcademia, err := categoriasProj.ListarPorAcademia(codigoAcademia)
	if err != nil {
		return err
	}
	disponiveis := map[string]bool{}
	anos := map[string]bool{}
	for _, ano := range anosAcademicos {
		anos[strings.TrimSpace(ano)] = true
	}
	for _, cat := range categoriasAcademia {
		if len(anos) == 0 {
			disponiveis[cat.Codigo] = true
			continue
		}
		for _, ano := range cat.AnosAcademicos {
			if anos[ano] {
				disponiveis[cat.Codigo] = true
				break
			}
		}
	}
	for cat := range vistos {
		if !disponiveis[cat] {
			return fmt.Errorf("categoria %s não está ativa/configurada para a academia nos anos_academicos da regra", cat)
		}
	}
	return nil
}

func validarNotaDespertadoraRegraAvaliacaoFinal(c *gin.Context, codigoAcademia string, anosAcademicos []string, aplicaSeReprovadoEmType *string, notaDespertadora *string) error {
	if aplicaSeReprovadoEmType != nil && strings.TrimSpace(*aplicaSeReprovadoEmType) != "" {
		if notaDespertadora != nil && strings.TrimSpace(*notaDespertadora) != "" {
			return fmt.Errorf("nota_despertadora é permitida apenas em regras raízes; regras descendentes são acionadas por reprovação na ancestral")
		}
		return nil
	}
	if notaDespertadora == nil || strings.TrimSpace(*notaDespertadora) == "" {
		return nil
	}
	codigo := strings.TrimSpace(*notaDespertadora)
	if strings.ContainsAny(codigo, " \t\r\n") {
		return fmt.Errorf("nota_despertadora deve ser o codigo exato de uma categoria de nota")
	}
	return validarCategoriasRegraAvaliacaoFinal(c, codigoAcademia, anosAcademicos, []string{codigo})
}

func validarFormulaAvaliacaoPorNivel(nivel, formula string, categorias []string) (string, []string, error) {
	ast, normalized, err := parseFormulaAvaliacao(formula)
	if err != nil {
		return "", nil, err
	}
	if err := validarPeriodosFormulaPorNivel(nivel, ast); err != nil {
		return "", nil, err
	}
	extraidas := categoriasFormula(ast)
	if len(categorias) == 0 {
		allowed := map[string]bool{}
		for _, c := range extraidas {
			allowed[c] = true
		}
		if err := validarASTFormula(ast, allowed); err != nil {
			return "", nil, err
		}
		return normalized, extraidas, nil
	}
	allowed := map[string]bool{}
	for _, c := range categorias {
		allowed[strings.TrimSpace(c)] = true
	}
	if err := validarASTFormula(ast, allowed); err != nil {
		return "", nil, err
	}
	if !mesmasCategorias(categorias, extraidas) {
		return "", nil, fmt.Errorf("categorias_envolvidas deve corresponder exatamente às categorias referenciadas na formula")
	}
	return normalized, extraidas, nil
}

func validarPeriodosFormulaPorNivel(nivel string, n *formulaAST) error {
	if n == nil {
		return nil
	}
	if n.Kind == formulaASTReference {
		if nivel == "superior" && n.Periodo != "" {
			return fmt.Errorf("formula de nivel superior deve referenciar apenas [categoria]; o periodo é inferido pela matéria avaliada")
		}
		if nivel != "superior" && n.Periodo == "" {
			return fmt.Errorf("formula de nivel %s deve informar periodo em cada referência, no formato [categoria,periodo]", nivel)
		}
	}
	if err := validarPeriodosFormulaPorNivel(nivel, n.Left); err != nil {
		return err
	}
	return validarPeriodosFormulaPorNivel(nivel, n.Right)
}

func validarFormulaAvaliacao(formula string, categorias []string) (string, []string, error) {
	ast, normalized, err := parseFormulaAvaliacao(formula)
	if err != nil {
		return "", nil, err
	}
	if err := validarPeriodosFormulaPorNivel("fundamental", ast); err != nil {
		return "", nil, err
	}
	extraidas := categoriasFormula(ast)
	if len(categorias) == 0 {
		allowed := map[string]bool{}
		for _, c := range extraidas {
			allowed[c] = true
		}
		if err := validarASTFormula(ast, allowed); err != nil {
			return "", nil, err
		}
		return normalized, extraidas, nil
	}
	allowed := map[string]bool{}
	for _, c := range categorias {
		allowed[strings.TrimSpace(c)] = true
	}
	if err := validarASTFormula(ast, allowed); err != nil {
		return "", nil, err
	}
	if !mesmasCategorias(categorias, extraidas) {
		return "", nil, fmt.Errorf("categorias_envolvidas deve corresponder exatamente às categorias referenciadas na formula")
	}
	return normalized, extraidas, nil
}

func categoriasFormula(ast *formulaAST) []string {
	vistos := map[string]bool{}
	var out []string
	var walk func(*formulaAST)
	walk = func(n *formulaAST) {
		if n == nil {
			return
		}
		if n.Kind == formulaASTReference {
			if !vistos[n.Categoria] {
				vistos[n.Categoria] = true
				out = append(out, n.Categoria)
			}
			return
		}
		walk(n.Left)
		walk(n.Right)
	}
	walk(ast)
	return out
}

func mesmasCategorias(a, b []string) bool { return mesmosAnosAcademicos(a, b) }

func parseFormulaAvaliacao(formula string) (*formulaAST, string, error) {
	formula = strings.TrimSpace(formula)
	if formula == "" {
		return nil, "", fmt.Errorf("formula não pode ser vazia")
	}
	if len(formula) > maxFormulaAvaliacaoLen {
		return nil, "", fmt.Errorf("formula excede o limite de %d caracteres", maxFormulaAvaliacaoLen)
	}
	p := &formulaParser{input: formula}
	ast, err := p.parseExpression()
	if err != nil {
		return nil, "", err
	}
	p.skipSpaces()
	if p.pos != len(p.input) {
		return nil, "", fmt.Errorf("token inválido na formula na posição %d", p.pos+1)
	}
	return ast, ast.String(), nil
}

func (p *formulaParser) parseExpression() (*formulaAST, error) {
	left, err := p.parseTerm()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpaces()
		if !p.consume('+') && !p.consume('-') {
			return left, nil
		}
		op := rune(p.input[p.pos-1])
		right, err := p.parseTerm()
		if err != nil {
			return nil, err
		}
		left = &formulaAST{Kind: formulaASTBinary, Op: op, Left: left, Right: right}
	}
}

func (p *formulaParser) parseTerm() (*formulaAST, error) {
	left, err := p.parseFactor()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpaces()
		if !p.consume('*') && !p.consume('/') {
			return left, nil
		}
		op := rune(p.input[p.pos-1])
		right, err := p.parseFactor()
		if err != nil {
			return nil, err
		}
		if op == '/' && right.Kind == formulaASTNumber && right.Value == 0 {
			return nil, fmt.Errorf("divisão por zero não permitida")
		}
		left = &formulaAST{Kind: formulaASTBinary, Op: op, Left: left, Right: right}
	}
}

func (p *formulaParser) parseFactor() (*formulaAST, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("formula incompleta")
	}
	ch := rune(p.input[p.pos])
	if ch == '(' {
		p.pos++
		n, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		p.skipSpaces()
		if !p.consume(')') {
			return nil, fmt.Errorf("parêntese ')' esperado")
		}
		return n, nil
	}
	if ch == '[' {
		return p.parseReference()
	}
	if unicode.IsDigit(ch) {
		return p.parseNumber()
	}
	return nil, fmt.Errorf("token inválido na formula na posição %d", p.pos+1)
}

func (p *formulaParser) parseReference() (*formulaAST, error) {
	p.pos++
	catStart := p.pos
	for p.pos < len(p.input) && isFormulaIdentRune(rune(p.input[p.pos])) {
		p.pos++
	}
	categoria := p.input[catStart:p.pos]
	if categoria == "" {
		return nil, fmt.Errorf("referência de nota exige categoria")
	}
	p.skipSpaces()
	periodo := ""
	if p.consume(',') {
		p.skipSpaces()
		perStart := p.pos
		for p.pos < len(p.input) && isFormulaIdentRune(rune(p.input[p.pos])) {
			p.pos++
		}
		periodo = p.input[perStart:p.pos]
		if periodo == "" {
			return nil, fmt.Errorf("referência de nota exige periodo")
		}
		p.skipSpaces()
	}
	if !p.consume(']') {
		return nil, fmt.Errorf("referência de nota deve terminar com ']'")
	}
	return &formulaAST{Kind: formulaASTReference, Categoria: categoria, Periodo: periodo}, nil
}

func (p *formulaParser) parseNumber() (*formulaAST, error) {
	start := p.pos
	for p.pos < len(p.input) && unicode.IsDigit(rune(p.input[p.pos])) {
		p.pos++
	}
	if p.pos < len(p.input) && p.input[p.pos] == '.' {
		p.pos++
		if p.pos >= len(p.input) || !unicode.IsDigit(rune(p.input[p.pos])) {
			return nil, fmt.Errorf("número decimal inválido")
		}
		for p.pos < len(p.input) && unicode.IsDigit(rune(p.input[p.pos])) {
			p.pos++
		}
	}
	v, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
		return nil, fmt.Errorf("número inválido")
	}
	return &formulaAST{Kind: formulaASTNumber, Value: v}, nil
}

func (p *formulaParser) skipSpaces() {
	for p.pos < len(p.input) && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}
func (p *formulaParser) consume(ch byte) bool {
	if p.pos < len(p.input) && p.input[p.pos] == ch {
		p.pos++
		return true
	}
	return false
}
func isFormulaIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}

func (n *formulaAST) String() string {
	switch n.Kind {
	case formulaASTNumber:
		return strconv.FormatFloat(n.Value, 'f', -1, 64)
	case formulaASTReference:
		if n.Periodo == "" {
			return fmt.Sprintf("[%s]", n.Categoria)
		}
		return fmt.Sprintf("[%s,%s]", n.Categoria, n.Periodo)
	case formulaASTBinary:
		return fmt.Sprintf("(%s%c%s)", n.Left.String(), n.Op, n.Right.String())
	}
	return ""
}

func validarASTFormula(n *formulaAST, cats map[string]bool) error {
	switch n.Kind {
	case formulaASTNumber:
		if n.Value < 0 {
			return fmt.Errorf("constantes negativas não são permitidas")
		}
	case formulaASTReference:
		if !cats[n.Categoria] {
			return fmt.Errorf("categoria %s não está em categorias_envolvidas", n.Categoria)
		}
		if n.Periodo != "" {
			if err := utils.ValidatePeriodo(n.Periodo); err != nil {
				return fmt.Errorf("periodo inválido na formula: %s", n.Periodo)
			}
		}
	case formulaASTBinary:
		if n.Op == '/' && n.Right.Kind == formulaASTNumber && n.Right.Value == 0 {
			return fmt.Errorf("divisão por zero não permitida")
		}
		if err := validarASTFormula(n.Left, cats); err != nil {
			return err
		}
		if err := validarASTFormula(n.Right, cats); err != nil {
			return err
		}
	}
	return nil
}

func calcularFormulaAvaliacao(formula string, notas map[string]map[string][]float64) (float64, error) {
	ast, _, err := parseFormulaAvaliacao(formula)
	if err != nil {
		return 0, err
	}
	return evalASTFormula(ast, notas)
}

func evalASTFormula(n *formulaAST, notas map[string]map[string][]float64) (float64, error) {
	switch n.Kind {
	case formulaASTNumber:
		return n.Value, nil
	case formulaASTReference:
		vals := notas[n.Categoria][n.Periodo]
		if len(vals) == 0 {
			return 0, fmt.Errorf("nota ausente: categoria=%s periodo=%s", n.Categoria, n.Periodo)
		}
		var s float64
		for _, v := range vals {
			s += v
		}
		return s, nil
	case formulaASTBinary:
		l, err := evalASTFormula(n.Left, notas)
		if err != nil {
			return 0, err
		}
		r, err := evalASTFormula(n.Right, notas)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case '+':
			return l + r, nil
		case '-':
			return l - r, nil
		case '*':
			return l * r, nil
		case '/':
			if r == 0 {
				return 0, fmt.Errorf("divisão por zero")
			}
			return l / r, nil
		}
	}
	return 0, fmt.Errorf("formula inválida")
}

func formulaContemPeriodo(n *formulaAST, periodoAtual string) bool {
	if n == nil {
		return false
	}
	if n.Kind == formulaASTReference {
		return n.Periodo == periodoAtual
	}
	return formulaContemPeriodo(n.Left, periodoAtual) || formulaContemPeriodo(n.Right, periodoAtual)
}

func preencherPeriodoFormulaSuperior(formula, periodo string) string {
	ast, _, err := parseFormulaAvaliacao(formula)
	if err != nil {
		return formula
	}
	var walk func(*formulaAST)
	walk = func(n *formulaAST) {
		if n == nil {
			return
		}
		if n.Kind == formulaASTReference && n.Periodo == "" {
			n.Periodo = periodo
		}
		walk(n.Left)
		walk(n.Right)
	}
	walk(ast)
	return ast.String()
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

func carregarNotasFormulaMateria(c *gin.Context, codigoEstudante, codigoAcademia, anoLectivo string, materiaID uuid.UUID, categorias []string) (map[string]map[string][]float64, error) {
	rows, err := getDbClient(c).DB().Query(`SELECT categoria,periodo,nota FROM projection_notas WHERE codigo_estudante=$1 AND codigo_academia=$2 AND ano_lectivo=$3 AND materia_disciplinar_id=$4 AND categoria=ANY($5) AND deleted_at IS NULL`, codigoEstudante, codigoAcademia, anoLectivo, materiaID, pq.Array(categorias))
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

func validarFormulaSuperiorContemPeriodo(formula string, periodoAtual string) error {
	ast, _, err := parseFormulaAvaliacao(formula)
	if err != nil {
		return err
	}
	if !formulaContemPeriodo(ast, periodoAtual) {
		return fmt.Errorf("formula superior deve incluir o periodo atual %s", periodoAtual)
	}
	return nil
}
