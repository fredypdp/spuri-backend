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
		utils.RespondWithValidationError(c, fmt.Errorf("payload inválido"))
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
		utils.RespondWithValidationError(c, fmt.Errorf("type deve ser fundamental, medio ou superior"))
		return
	}
}

func alterarAnosFundamental(c *gin.Context, academiaDTO *projections.AcademiaDTO, req anosAcademicosRequest, op string) error {
	if academiaDTO.Nivel != "escolar" || academiaDTO.NivelEscolar == nil || (*academiaDTO.NivelEscolar != "fundamental" && *academiaDTO.NivelEscolar != "misto") {
		return fmt.Errorf("academia não pode gerenciar anos do fundamental")
	}
	if len(req.AnosAcademicos) == 0 {
		return fmt.Errorf("anos_academicos é obrigatório")
	}
	if err := utils.ValidateAnosFundamental(req.AnosAcademicos); err != nil {
		return err
	}
	novos := combinarAnos(academiaDTO.AnosAcademicos, req.AnosAcademicos, op)
	if len(novos) == 0 {
		return fmt.Errorf("academias fundamental/misto devem manter ao menos um ano acadêmico ativo")
	}
	removidos := valoresRemovidos(academiaDTO.AnosAcademicos, novos)
	if len(removidos) > 0 {
		qtd, err := getEstudanteProjection(c).CountActiveByFundamentalAnos(academiaDTO.CodigoAcademia, removidos)
		if err != nil {
			return err
		}
		if qtd > 0 {
			return conflictError(fmt.Sprintf("não é possível desativar anos_academicos %v: existem %d estudante(s) ativo(s) vinculados", removidos, qtd))
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
		return fmt.Errorf("curso_id é obrigatório para type %s", req.Type)
	}
	cursoDTO, err := getCursosProjection(c).GetByID(*req.CursoID)
	if err != nil || cursoDTO == nil {
		return fmt.Errorf("curso não encontrado")
	}
	if cursoDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		return fmt.Errorf("curso nao pertence a esta academia")
	}
	if cursoDTO.Type != req.Type {
		return fmt.Errorf("type do payload não corresponde ao tipo do curso")
	}
	var novosAnos []string
	var novosPeriodos *[]string
	if req.Type == "medio" {
		if len(req.AnosAcademicos) == 0 {
			return fmt.Errorf("anos_academicos é obrigatório para curso médio")
		}
		novosAnos = combinarAnos(cursoDTO.AnosAcademicos, req.AnosAcademicos, op)
	} else {
		if req.Periodos == nil {
			return fmt.Errorf("periodos é obrigatório para curso superior")
		}
		anos, periodos, err := derivarCursoSuperior(*req.Periodos)
		if err != nil {
			return err
		}
		novosAnos = anos
		novosPeriodos = &periodos
	}
	if err := validarEdicaoCursoComEstudantesAtivos(c, cursoDTO, novosAnos, novosPeriodos); err != nil {
		return conflictError(err.Error())
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
			utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia é obrigatório para admin"))
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
func responderErroAnos(c *gin.Context, err error) {
	if _, ok := err.(anosConflict); ok {
		utils.RespondWithConflictError(c, err.Error())
	} else {
		utils.RespondWithValidationError(c, err)
	}
}
