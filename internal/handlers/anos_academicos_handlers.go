package handlers

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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
func AtualizarAnosAcademicos(c *gin.Context) { alterarAnosAcademicos(c, "set") }
func RemoverAnosAcademicos(c *gin.Context)   { alterarAnosAcademicos(c, "remove") }

func alterarAnosAcademicos(c *gin.Context, op string) {
	academiaDTO, ok := academiaAutenticada(c)
	if !ok {
		return
	}
	var req anosAcademicosRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		responderErroAnosValidacao(c, "payload", "json_invalido", "O corpo da requisição deve ser um JSON válido. Verifique vírgulas, aspas, chaves e tipos dos campos antes de reenviar.")
		return
	}
	switch req.Type {
	case "fundamental":
		if err := alterarAnosFundamental(c, academiaDTO, req, op); err != nil {
			responderErroAnos(c, err)
			return
		}
	case "medio", "superior":
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
	if academiaDTO.Nivel != "escolar" || academiaDTO.NivelEscolar == nil || (*academiaDTO.NivelEscolar != "fundamental" && *academiaDTO.NivelEscolar != "misto") {
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
		qtd, err := getEstudanteProjection(c).CountActiveByFundamentalAnos(academiaDTO.CodigoAcademia, removidos)
		if err != nil {
			return err
		}
		if qtd > 0 {
			return conflictErrorWithDetail("anos_academicos", "estudantes_ativos_vinculados", fmt.Sprintf("Não é possível desativar os anos %v porque existem %d estudante(s) ativo(s) vinculados a eles. Transfira, conclua ou inative esses estudantes antes de remover os anos.", removidos, qtd))
		}
	}
	repository := getRepository(c)
	agg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		return err
	}
	academia := agg.(*aggregates.Academia)
	if err := academia.AtualizarDados(nil, nil, nil, nil, nil, nil, nil, nil, novos, nil); err != nil {
		return err
	}
	userID, _ := middleware.GetUserID(c)
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		return err
	}
	c.JSON(http.StatusOK, gin.H{"message": "anos acadêmicos atualizados com sucesso", "type": "fundamental", "anos_academicos": novos})
	return nil
}

func alterarEscopoCurso(c *gin.Context, academiaDTO *projections.AcademiaDTO, req anosAcademicosRequest, op string) error {
	if req.CursoID == nil {
		return newAnosValidationError("curso_id", "campo_obrigatorio", fmt.Sprintf("O campo 'curso_id' é obrigatório quando type='%s', porque anos de médio/superior pertencem a um curso específico.", req.Type))
	}
	cursoDTO, err := getCursosProjection(c).GetByID(*req.CursoID)
	if err != nil || cursoDTO == nil {
		return newAnosValidationError("curso_id", "nao_encontrado", fmt.Sprintf("Nenhum curso foi encontrado com curso_id='%s'. Confira se o ID foi copiado corretamente e se o curso existe.", req.CursoID.String()))
	}
	if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		return newAnosValidationError("curso_id", "curso_de_outra_academia", fmt.Sprintf("O curso_id='%s' pertence à academia '%s', mas a requisição está autenticada para a academia '%s'. Use um curso da própria academia.", cursoDTO.ID, cursoDTO.CodigoAcademia, academiaDTO.CodigoAcademia))
	}
	if cursoDTO.Status != "ativo" {
		return newAnosValidationError("curso_id", "curso_inativo", fmt.Sprintf("O curso '%s' está com status '%s'. Só é possível gerenciar anos acadêmicos de cursos ativos.", cursoDTO.Nome, cursoDTO.Status))
	}
	if cursoDTO.Type != req.Type {
		return newAnosValidationError("type", "tipo_diferente_do_curso", fmt.Sprintf("O payload informou type='%s', mas o curso '%s' é do tipo '%s'. Envie o mesmo tipo do curso.", req.Type, cursoDTO.Nome, cursoDTO.Type))
	}
	var novosAnos []string
	var novosPeriodos *[]string
	if req.Type == "medio" {
		if len(req.AnosAcademicos) == 0 {
			return newAnosValidationError("anos_academicos", "campo_obrigatorio", "Informe pelo menos um ano em 'anos_academicos' para curso médio. Exemplo: ['1_ano_medio', '2_ano_medio'].")
		}
		if err := utils.ValidateAnosCurso("medio", req.AnosAcademicos); err != nil {
			return newAnosValidationError("anos_academicos", "formato_invalido", err.Error())
		}
		novosAnos = combinarAnos(cursoDTO.AnosAcademicos, req.AnosAcademicos, op)
	} else {
		if len(req.AnosAcademicos) > 0 {
			return newAnosValidationError("anos_academicos", "campo_nao_permitido", "Não envie 'anos_academicos' para curso superior. Para superior, envie apenas 'periodos'; o sistema calcula os anos automaticamente. Exemplo: periodos=8 gera anos como ['1_ano_superior', '2_ano_superior', ...].")
		}
		if req.Periodos == nil {
			return newAnosValidationError("periodos", "campo_obrigatorio", "O campo 'periodos' é obrigatório para curso superior, pois os anos acadêmicos são calculados pela quantidade de períodos do curso.")
		}
		anos, periodos, err := derivarCursoSuperior(*req.Periodos)
		if err != nil {
			return newAnosValidationError("periodos", "valor_invalido", fmt.Sprintf("%s. Informe um número inteiro positivo de períodos para o curso superior.", err.Error()))
		}
		novosAnos = anos
		novosPeriodos = &periodos
	}
	if err := validarEdicaoCursoComEstudantesAtivos(c, cursoDTO, novosAnos, novosPeriodos); err != nil {
		return conflictErrorWithDetail("anos_academicos", "estudantes_ativos_vinculados", fmt.Sprintf("%s. Ajuste primeiro os estudantes ativos vinculados ao curso antes de alterar/remover anos ou períodos.", err.Error()))
	}
	repository := getRepository(c)
	agg, err := repository.Load(cursoDTO.ID, "Curso")
	if err != nil {
		return err
	}
	curso := agg.(*aggregates.Curso)
	if err := curso.AtualizarDados(nil, novosAnos, novosPeriodos, academiaDTO.ID); err != nil {
		return err
	}
	if err := repository.SaveWithAudit(curso, db.AuditContext{UserID: academiaDTO.ID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		return err
	}
	resp := gin.H{"message": "anos acadêmicos atualizados com sucesso", "type": req.Type, "curso_id": cursoDTO.ID, "anos_academicos": curso.AnosAcademicos}
	if req.Type == "superior" {
		resp["periodos"] = curso.Periodos
	}
	c.JSON(http.StatusOK, resp)
	return nil
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
