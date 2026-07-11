package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"
)

type anosAcademicosRequest struct {
	Type           string     `json:"type"`
	CursoID        *uuid.UUID `json:"curso_id"`
	AnosAcademicos []string   `json:"anos_academicos"`
	Periodos       *int       `json:"periodos"`
}

func bindAnosAcademicosRequest(c *gin.Context, req *anosAcademicosRequest) error {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return newAnosValidationError("payload", "json_invalido", "O corpo da requisição deve ser um JSON válido. Verifique vírgulas, aspas, chaves e tipos dos campos antes de reenviar.")
	}
	for campo := range raw {
		switch campo {
		case "type", "curso_id", "anos_academicos", "periodos":
		case "codigo_academia":
			return newAnosValidationError(campo, "campo_nao_permitido", "Não envie 'codigo_academia' em escritas de anos acadêmicos. A academia alterada é sempre a academia autenticada.")
		case "substituir", "replace", "patch", "set", "update":
			return newAnosValidationError(campo, "campo_nao_permitido", "Payloads de substituição em massa não são suportados. Use POST para adicionar ou DELETE para remover anos acadêmicos.")
		default:
			return newAnosValidationError(campo, "campo_nao_permitido", fmt.Sprintf("Campo não suportado em anos acadêmicos: %s", campo))
		}
	}
	data, _ := json.Marshal(raw)
	if err := json.Unmarshal(data, req); err != nil {
		return newAnosValidationError("payload", "json_invalido", "O corpo da requisição contém campos com tipos inválidos para anos acadêmicos.")
	}
	return nil
}

func ListarAnosAcademicos(c *gin.Context) {
	academiaDTO, ok := academiaAutenticada(c)
	if !ok {
		return
	}
	cursos, err := getCursosProjection(c).GetByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"academia": gin.H{"nivel": academiaDTO.Nivel, "nivel_escolar": academiaDTO.NivelEscolar, "anos_academicos": academiaDTO.AnosAcademicos},
		"cursos":   cursos,
	})
}

func AdicionarAnosAcademicos(c *gin.Context) { alterarAnosAcademicos(c, "add") }
func RemoverAnosAcademicos(c *gin.Context)   { alterarAnosAcademicos(c, "remove") }

func alterarAnosAcademicos(c *gin.Context, op string) {
	academiaDTO, ok := academiaAutenticada(c)
	if !ok {
		return
	}
	var req anosAcademicosRequest
	if err := bindAnosAcademicosRequest(c, &req); err != nil {
		responderErroAnos(c, err)
		return
	}
	switch req.Type {
	case "fundamental":
		if err := alterarAnosFundamental(c, academiaDTO, req, op); err != nil {
			responderErroAnos(c, err)
			return
		}
	case "medio":
		responderErroAnosValidacao(c, "type", "operacao_nao_suportada", "Cursos médios não aceitam gestão manual de anos acadêmicos; os anos são fixos e derivados do modelo do curso.")
		return
	case "superior":
		if err := alterarEscopoCurso(c, academiaDTO, req, op); err != nil {
			responderErroAnos(c, err)
			return
		}
	default:
		responderErroAnosValidacao(c, "type", "valor_invalido", fmt.Sprintf("O campo 'type' recebeu '%s', mas só aceita: 'fundamental', 'medio' ou 'superior'. Use 'fundamental' para anos do ensino fundamental, 'medio' para cursos médios e 'superior' para cursos superiores.", req.Type))
		return
	}
}

func alterarAnosFundamental(c *gin.Context, academiaDTO *projections.AcademiaDTO, req anosAcademicosRequest, op string) error {
	if academiaDTO.Nivel != "escola" || academiaDTO.NivelEscolar == nil || (*academiaDTO.NivelEscolar != "fundamental" && *academiaDTO.NivelEscolar != "misto") {
		return newAnosValidationError("type", "nivel_incompativel", fmt.Sprintf("Esta academia não pode gerenciar anos do ensino fundamental porque o nível cadastrado é nivel='%s' e nivel_escolar='%s'. Somente academias escolares com nivel_escolar 'fundamental' ou 'misto' podem alterar anos fundamentais.", academiaDTO.Nivel, stringPtrValue(academiaDTO.NivelEscolar)))
	}
	if len(req.AnosAcademicos) == 0 {
		return newAnosValidationError("anos_academicos", "campo_obrigatorio", "Informe pelo menos um ano no campo 'anos_academicos'. Exemplo válido para fundamental: ['1_ano_fundamental', '2_ano_fundamental'].")
	}
	if err := utils.ValidateAnosFundamental(req.AnosAcademicos); err != nil {
		return newAnosValidationError("anos_academicos", "formato_invalido", err.Error())
	}
	novos := combinarAnos(academiaDTO.AnosAcademicos, req.AnosAcademicos, op)
	if len(novos) == 0 {
		return newAnosValidationError("anos_academicos", "remocao_invalida", "A operação removeria todos os anos acadêmicos. Academias fundamental/misto precisam manter pelo menos um ano ativo.")
	}
	removidos := valoresRemovidos(academiaDTO.AnosAcademicos, novos)
	if len(removidos) > 0 {
		if err := validarRemocaoAnosFundamentalSemDependenciasAtivas(c, academiaDTO.CodigoAcademia, removidos); err != nil {
			return err
		}
	}
	repository := getRepository(c)
	agg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		return err
	}
	academia := agg.(*aggregates.Academia)
	if err := academia.AtualizarDados(nil, nil, nil, nil, nil, nil, nil, nil, nil, novos, nil); err != nil {
		return err
	}
	userID, _ := middleware.GetUserID(c)
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		return err
	}
	c.JSON(http.StatusOK, gin.H{"message": "anos acadêmicos atualizados com sucesso", "type": "fundamental", "anos_academicos": novos})
	return nil
}

type dependenciasAnoFundamental struct {
	EstudantesAtivos               int
	SolicitacoesMatriculaPendentes int
	MateriasAtivas                 int
	CategoriasNotaAtivas           int
	RegrasAvaliacaoFinalAtivas     int
}

func (d dependenciasAnoFundamental) total() int {
	return d.EstudantesAtivos + d.SolicitacoesMatriculaPendentes + d.MateriasAtivas + d.CategoriasNotaAtivas + d.RegrasAvaliacaoFinalAtivas
}

func (d dependenciasAnoFundamental) resumo() string {
	partes := []string{}
	if d.EstudantesAtivos > 0 {
		partes = append(partes, fmt.Sprintf("%d estudante(s) ativo(s)", d.EstudantesAtivos))
	}
	if d.SolicitacoesMatriculaPendentes > 0 {
		partes = append(partes, fmt.Sprintf("%d solicitação(ões) de matrícula pendente(s)", d.SolicitacoesMatriculaPendentes))
	}
	if d.MateriasAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d matéria(s) ativa(s)", d.MateriasAtivas))
	}
	if d.CategoriasNotaAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d categoria(s) de nota ativa(s)", d.CategoriasNotaAtivas))
	}
	if d.RegrasAvaliacaoFinalAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d regra(s) de avaliação final ativa(s)", d.RegrasAvaliacaoFinalAtivas))
	}
	return strings.Join(partes, ", ")
}

func validarRemocaoAnosFundamentalSemDependenciasAtivas(c *gin.Context, codigoAcademia string, anos []string) error {
	deps, err := contarDependenciasAtivasAnosFundamental(c, codigoAcademia, anos)
	if err != nil {
		return err
	}
	if deps.total() == 0 {
		return nil
	}

	return conflictErrorWithDetail(
		"anos_academicos",
		"dependencias_ativas_vinculadas",
		fmt.Sprintf("Não é possível desativar os anos %v porque existem dependências ativas vinculadas a eles: %s. Resolva essas dependências antes de remover os anos acadêmicos.", anos, deps.resumo()),
	)
}

func contarDependenciasAtivasAnosFundamental(c *gin.Context, codigoAcademia string, anos []string) (dependenciasAnoFundamental, error) {
	deps := dependenciasAnoFundamental{}
	var err error
	deps.EstudantesAtivos, err = getEstudanteProjection(c).CountActiveByFundamentalAnos(codigoAcademia, anos)
	if err != nil {
		return deps, err
	}
	deps.SolicitacoesMatriculaPendentes, err = getSolicitacaoMatriculaProjection(c).CountPendingByFundamentalAnos(codigoAcademia, anos)
	if err != nil {
		return deps, err
	}

	db := getDbClient(c).DB()
	if err := db.QueryRow(`
		SELECT COUNT(*)
		  FROM projection_materias
		 WHERE codigo_academia = $1
		   AND type = 'fundamental'
		   AND status = 'ativo'
		   AND deleted_at IS NULL
		   AND EXISTS (SELECT 1 FROM unnest($2::text[]) AS ano WHERE anos_academicos ? ano)
	`, codigoAcademia, pq.Array(anos)).Scan(&deps.MateriasAtivas); err != nil {
		return deps, err
	}
	if err := db.QueryRow(`
		SELECT COUNT(*)
		  FROM projection_categorias_nota
		 WHERE codigo_academia = $1
		   AND status = 'ativo'
		   AND EXISTS (SELECT 1 FROM unnest($2::text[]) AS ano WHERE anos_academicos ? ano)
	`, codigoAcademia, pq.Array(anos)).Scan(&deps.CategoriasNotaAtivas); err != nil {
		return deps, err
	}
	if err := db.QueryRow(`
		SELECT COUNT(*)
		  FROM projection_regras_avaliacao_final
		 WHERE codigo_academia = $1
		   AND nivel = 'fundamental'
		   AND status = 'ativo'
		   AND EXISTS (SELECT 1 FROM unnest($2::text[]) AS ano WHERE anos_academicos ? ano)
	`, codigoAcademia, pq.Array(anos)).Scan(&deps.RegrasAvaliacaoFinalAtivas); err != nil {
		return deps, err
	}
	return deps, nil
}

func alterarEscopoCurso(c *gin.Context, academiaDTO *projections.AcademiaDTO, req anosAcademicosRequest, op string) error {
	if req.Type == "superior" && academiaDTO.Nivel != "superior" {
		return newAnosValidationError("type", "nivel_incompativel", fmt.Sprintf("Esta academia não pode gerenciar períodos do ensino superior porque o nível cadastrado é nivel='%s' e nivel_escolar='%s'. Somente academias superiores podem gerenciar escopos superiores.", academiaDTO.Nivel, stringPtrValue(academiaDTO.NivelEscolar)))
	}
	return newAnosValidationError("type", "operacao_nao_suportada", "Cursos superiores não aceitam adição ou remoção direta de anos acadêmicos/períodos por /academia/anos-academicos. Use o fluxo específico de períodos, quando disponível.")
}

func academiaAutenticada(c *gin.Context) (*projections.AcademiaDTO, bool) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	var (
		dto *projections.AcademiaDTO
		err error
	)
	if userType == "admin" {
		codigoAcademia := c.Query("codigo_academia")
		if codigoAcademia == "" {
			responderErroAnosValidacao(c, "codigo_academia", "campo_obrigatorio", "Administradores precisam informar o parâmetro de consulta 'codigo_academia' para o sistema saber de qual academia deve listar/alterar os anos acadêmicos. Exemplo: ?codigo_academia=ACA001")
			return nil, false
		}
		dto, err = getAcademiaProjection(c).GetByCodigo(codigoAcademia)
	} else {
		dto, err = getAcademiaProjection(c).GetByID(userID)
	}
	if err != nil || dto == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return nil, false
	}
	return dto, true
}

func combinarAnos(atuais, entrada []string, op string) []string {
	m := map[string]struct{}{}
	if op != "set" {
		for _, v := range atuais {
			m[v] = struct{}{}
		}
	}
	if op == "remove" {
		for _, v := range entrada {
			delete(m, v)
		}
	} else {
		for _, v := range entrada {
			m[v] = struct{}{}
		}
	}
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func validarSequenciaAnosMedio(anos []string) error {
	if err := utils.ValidateAnosCurso("medio", anos); err != nil {
		return err
	}
	for i, ano := range anos {
		numero, sufixo, ok := strings.Cut(ano, "_")
		if !ok || sufixo != "ano_medio" {
			return fmt.Errorf("ano '%s' inválido para curso médio", ano)
		}
		n, err := strconv.Atoi(numero)
		if err != nil || n <= 0 {
			return fmt.Errorf("ano '%s' inválido para curso médio", ano)
		}
		esperado := i + 1
		if n != esperado {
			return fmt.Errorf("anos do ensino médio devem estar em ordem sequencial crescente começando em 1_ano_medio; esperado %d_ano_medio na posição %d", esperado, i+1)
		}
	}
	return nil
}

type anosConflict string

func (e anosConflict) Error() string { return string(e) }
func conflictError(msg string) error { return anosConflict(msg) }

type anosValidationError struct{ detail utils.ValidationDetail }

func (e anosValidationError) Error() string { return e.detail.Message }

type anosConflictDetail struct{ detail utils.ValidationDetail }

func (e anosConflictDetail) Error() string { return e.detail.Message }
func newAnosValidationError(field, code, message string) error {
	return anosValidationError{detail: utils.ValidationDetail{Field: field, Code: code, Message: message}}
}
func responderErroAnosValidacao(c *gin.Context, field, code, message string) {
	responderErroAnos(c, newAnosValidationError(field, code, message))
}
func conflictErrorWithDetail(field, code, message string) error {
	return anosConflictDetail{detail: utils.ValidationDetail{Field: field, Code: code, Message: message}}
}
func responderErroAnos(c *gin.Context, err error) {
	if validationErr, ok := err.(anosValidationError); ok {
		utils.RespondWithDetailedError(c, http.StatusBadRequest, validationErr.Error(), err, []utils.ValidationDetail{validationErr.detail})
		return
	}
	if conflictDetail, ok := err.(anosConflictDetail); ok {
		utils.RespondWithDetailedError(c, http.StatusConflict, conflictDetail.Error(), err, []utils.ValidationDetail{conflictDetail.detail})
		return
	}
	if conflictErr, ok := err.(anosConflict); ok {
		utils.RespondWithConflictError(c, conflictErr.Error())
	} else {
		utils.RespondWithValidationError(c, err)
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return "não informado"
	}
	return *value
}
