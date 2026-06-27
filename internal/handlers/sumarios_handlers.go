package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"
)

type sumarioReq struct {
	SumarioTitulo string  `json:"sumario_titulo"`
	Descricao     *string `json:"descricao"`
	Periodo       string  `json:"periodo"`
	AnoAcademico  int     `json:"ano_academico"`
	CursoID       *string `json:"curso_id"`
	MateriaID     string  `json:"materia_id"`
}

func getSumariosProjection(c *gin.Context) *projections.SumariosProjection {
	return projections.NewSumariosProjection(getDbClient(c))
}

func validarContextoSumario(c *gin.Context, academiaCodigo string, academiaNivel string, r sumarioReq) (nivel, typ string, cursoID *uuid.UUID, materiaID uuid.UUID, err error) {
	periodo := strings.TrimSpace(r.Periodo)
	if periodo == "" {
		err = fmt.Errorf("periodo é obrigatório")
		return
	}
	if !(strings.HasSuffix(periodo, "_trimestre") || strings.HasSuffix(periodo, "_semestre")) {
		err = fmt.Errorf("periodo deve usar formato N_trimestre ou N_semestre")
		return
	}
	materiaID, err = uuid.Parse(r.MateriaID)
	if err != nil {
		err = fmt.Errorf("materia_id inválido")
		return
	}
	mat, _ := getMateriasProjection(c).GetByID(materiaID)
	if mat == nil || mat.CodigoAcademia != academiaCodigo {
		err = fmt.Errorf("materia não pertence a esta academia")
		return
	}
	typ = mat.Type
	nivel = "escolar"
	if typ == "superior" {
		nivel = "superior"
	}
	if academiaNivel != "" && academiaNivel != "misto" && academiaNivel != nivel {
		err = fmt.Errorf("nível do sumário incompatível com a academia")
		return
	}
	if typ == "superior" && !strings.HasSuffix(periodo, "_semestre") {
		err = fmt.Errorf("sumários do superior devem usar semestre")
		return
	}
	if typ != "superior" && !strings.HasSuffix(periodo, "_trimestre") {
		err = fmt.Errorf("sumários escolares/médio devem usar trimestre")
		return
	}
	if mat.Periodo != nil && typ == "superior" && *mat.Periodo != periodo {
		err = fmt.Errorf("periodo incompatível com a matéria")
		return
	}
	okAno := false
	for _, a := range mat.AnosAcademicos {
		if a == strconv.Itoa(r.AnoAcademico) {
			okAno = true
		}
	}
	if !okAno {
		err = fmt.Errorf("ano_academico incompatível com a matéria")
		return
	}
	if mat.CursoID != nil {
		cursoID = mat.CursoID
	} else if r.CursoID != nil && strings.TrimSpace(*r.CursoID) != "" {
		u, e := uuid.Parse(*r.CursoID)
		if e != nil {
			err = fmt.Errorf("curso_id inválido")
			return
		}
		cursoID = &u
	}
	if typ == "medio" || typ == "superior" {
		if cursoID == nil {
			err = fmt.Errorf("curso_id é obrigatório para médio e superior")
			return
		}
		curso, _ := getCursosProjection(c).GetByID(*cursoID)
		if curso == nil || curso.CodigoAcademia != academiaCodigo || curso.Type != typ {
			err = fmt.Errorf("curso incompatível com a academia ou tipo")
			return
		}
	}
	return
}

func CriarSumario(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	var req sumarioReq
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("dados inválidos"))
		return
	}
	acad, _ := getAcademiaProjection(c).GetByID(userID)
	if acad == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	nivel, typ, cursoID, materiaID, err := validarContextoSumario(c, acad.CodigoAcademia, acad.Nivel, req)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	s := aggregates.NewSumarioAula()
	if err := s.Criar(acad.ID, acad.CodigoAcademia, req.SumarioTitulo, req.Descricao, req.Periodo, req.AnoAcademico, nivel, typ, cursoID, materiaID, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "sumário criado com sucesso", "id": s.ID})
}
func ListarSumarios(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	acad, _ := getAcademiaProjection(c).GetByID(userID)
	codigoAcademia := ""
	if acad != nil {
		codigoAcademia = acad.CodigoAcademia
	} else if userType == "admin" {
		codigoAcademia = strings.TrimSpace(c.Query("codigo_academia"))
	}
	if codigoAcademia == "" {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	xs, err := getSumariosProjection(c).List(codigoAcademia, c.Query("periodo"), c.Query("ano_academico"), c.Query("curso_id"), c.Query("materia_id"))
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": xs, "total": len(xs)})
}
func GetSumario(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	acad, _ := getAcademiaProjection(c).GetByID(userID)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}
	x, err := getSumariosProjection(c).GetByID(id)
	if err != nil || x == nil {
		utils.RespondWithNotFoundError(c, "sumário")
		return
	}
	if userType != "admin" && (acad == nil || x.CodigoAcademia != acad.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
		return
	}
	c.JSON(http.StatusOK, x)
}
func AtualizarSumario(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	acad, _ := getAcademiaProjection(c).GetByID(userID)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}
	atual, _ := getSumariosProjection(c).GetByID(id)
	if atual == nil {
		utils.RespondWithNotFoundError(c, "sumário")
		return
	}
	if acad == nil || atual.CodigoAcademia != acad.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
		return
	}
	var req sumarioReq
	_ = c.ShouldBindJSON(&req)
	if req.Periodo == "" {
		req.Periodo = atual.Periodo
	}
	if req.AnoAcademico == 0 {
		req.AnoAcademico = atual.AnoAcademico
	}
	if req.MateriaID == "" {
		req.MateriaID = atual.MateriaID.String()
	}
	if req.CursoID == nil && atual.CursoID != nil {
		s := atual.CursoID.String()
		req.CursoID = &s
	}
	_, _, cursoID, materiaID, err := validarContextoSumario(c, acad.CodigoAcademia, acad.Nivel, req)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	aggRaw, err := getRepository(c).Load(id, "SumarioAula")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	agg := aggRaw.(*aggregates.SumarioAula)
	var title *string
	if req.SumarioTitulo != "" {
		title = &req.SumarioTitulo
	}
	if err := agg.Atualizar(title, req.Descricao, &req.Periodo, &req.AnoAcademico, cursoID, &materiaID, userID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).SaveWithAudit(agg, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sumário atualizado com sucesso", "id": id})
}
func DeletarSumario(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	acad, _ := getAcademiaProjection(c).GetByID(userID)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("id inválido"))
		return
	}
	atual, _ := getSumariosProjection(c).GetByID(id)
	if atual == nil {
		utils.RespondWithNotFoundError(c, "sumário")
		return
	}
	if acad == nil || atual.CodigoAcademia != acad.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "sumário não pertence a esta academia")
		return
	}
	raw, err := getRepository(c).Load(id, "SumarioAula")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	agg := raw.(*aggregates.SumarioAula)
	_ = agg.Desativar(userID)
	if err := getRepository(c).SaveWithAudit(agg, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "sumário removido com sucesso"})
}
