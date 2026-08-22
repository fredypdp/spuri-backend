package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
	"spuri/internal/utils"
)

// configure/list are academy-owned resources; FPP may only supervise
// (consultar/listar) them — an admin can never create, update, or remove a
// specific academy's mensalidade/matrícula/mês-de-início configuration
// (write=true is always denied for the "admin" actor type). Exemption
// decisions (anular/reativar obrigações) remain academy-exclusive and don't
// use this function at all.
func authorizeMensalidadeAcademia(c *gin.Context, codigo *string, write bool) bool {
	_, typ, own, ok := financeActor(c)
	if !ok {
		return false
	}
	if typ == "academia" {
		if strings.TrimSpace(*codigo) != "" && *codigo != own {
			return false
		}
		*codigo = own
		return true
	}
	if typ != "admin" || !financeAdminAllowed(c) || strings.TrimSpace(*codigo) == "" {
		return false
	}
	return !write
}

func ConfigurarMensalidade(c *gin.Context) {
	var in finance.MensalidadeConfiguracaoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeMensalidadeAcademia(c, &in.CodigoAcademia, true) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar mensalidade desta academia")
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.ConfigureMensalidade(c.Request.Context(), in, id.String(), typ, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func ListarConfiguracoesMensalidade(c *gin.Context) {
	codigo := c.Query("codigo_academia")
	if !authorizeMensalidadeAcademia(c, &codigo, false) {
		utils.RespondWithForbiddenError(c, "sem permissão para consultar mensalidades desta academia")
		return
	}
	out, err := FinanceiroService.ListMensalidadeConfiguracoes(c.Request.Context(), codigo)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"codigo_academia": codigo, "configuracoes": out})
}

// RemoverConfiguracaoMensalidadeInput identifica o escopo (mesma tripla
// nível+ano acadêmico+curso usada para configurar) cuja configuração de
// mensalidade deve deixar de estar ativa.
type RemoverConfiguracaoMensalidadeInput struct {
	CodigoAcademia string  `json:"codigo_academia"`
	Nivel          string  `json:"nivel"`
	AnoAcademico   string  `json:"ano_academico"`
	CursoID        *string `json:"curso_id,omitempty"`
}

func RemoverConfiguracaoMensalidade(c *gin.Context) {
	var in RemoverConfiguracaoMensalidadeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeMensalidadeAcademia(c, &in.CodigoAcademia, true) {
		utils.RespondWithForbiddenError(c, "sem permissão para remover configuração de mensalidade desta academia")
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	var cursoID *uuid.UUID
	if in.CursoID != nil && strings.TrimSpace(*in.CursoID) != "" {
		parsed, err := uuid.Parse(*in.CursoID)
		if err != nil {
			utils.RespondWithValidationError(c, errors.New("curso_id inválido"))
			return
		}
		cursoID = &parsed
	}
	if err := FinanceiroService.RemoveMensalidadeConfiguracao(c.Request.Context(), in.CodigoAcademia, in.Nivel, in.AnoAcademico, cursoID, id.String(), typ, c.ClientIP()); err != nil {
		financeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func ConfigurarMatricula(c *gin.Context) {
	var in finance.MatriculaConfiguracaoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeMensalidadeAcademia(c, &in.CodigoAcademia, true) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar matrícula desta academia")
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.ConfigureMatricula(c.Request.Context(), in, id.String(), typ, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func ListarConfiguracoesMatricula(c *gin.Context) {
	codigo := c.Query("codigo_academia")
	if !authorizeMensalidadeAcademia(c, &codigo, false) {
		utils.RespondWithForbiddenError(c, "sem permissão para consultar matrículas desta academia")
		return
	}
	out, err := FinanceiroService.ListMatriculaConfiguracoes(c.Request.Context(), codigo)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"codigo_academia": codigo, "configuracoes": out})
}

func RemoverConfiguracaoMatricula(c *gin.Context) {
	var in RemoverConfiguracaoMensalidadeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeMensalidadeAcademia(c, &in.CodigoAcademia, true) {
		utils.RespondWithForbiddenError(c, "sem permissão para remover configuração de matrícula desta academia")
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	var cursoID *uuid.UUID
	if in.CursoID != nil && strings.TrimSpace(*in.CursoID) != "" {
		parsed, err := uuid.Parse(*in.CursoID)
		if err != nil {
			utils.RespondWithValidationError(c, errors.New("curso_id inválido"))
			return
		}
		cursoID = &parsed
	}
	if err := FinanceiroService.RemoveMatriculaConfiguracao(c.Request.Context(), in.CodigoAcademia, in.Nivel, in.AnoAcademico, cursoID, id.String(), typ, c.ClientIP()); err != nil {
		financeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func DefinirMesInicioCobranca(c *gin.Context) {
	var in finance.MesInicioCobrancaInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeMensalidadeAcademia(c, &in.CodigoAcademia, true) {
		utils.RespondWithForbiddenError(c, "sem permissão para definir o início de cobrança desta academia")
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	if err := FinanceiroService.DefinirMesInicioCobranca(c.Request.Context(), in, id.String(), typ, c.ClientIP()); err != nil {
		financeError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// RemoverMesInicioCobrancaInput identifica a academia e o ano letivo cuja
// redefinição de mês de início de cobrança deve deixar de valer, voltando
// o sistema a usar o mês natural padrão do ano letivo.
type RemoverMesInicioCobrancaInput struct {
	CodigoAcademia string `json:"codigo_academia"`
	AnoLetivo      string `json:"ano_letivo"`
}

func RemoverMesInicioCobranca(c *gin.Context) {
	var in RemoverMesInicioCobrancaInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeMensalidadeAcademia(c, &in.CodigoAcademia, true) {
		utils.RespondWithForbiddenError(c, "sem permissão para remover o início de cobrança desta academia")
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	if err := FinanceiroService.RemoveMesInicioCobranca(c.Request.Context(), in.CodigoAcademia, in.AnoLetivo, id.String(), typ, c.ClientIP()); err != nil {
		financeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func ConsultarMensalidadesEstudante(c *gin.Context) {
	codigo := strings.TrimSpace(c.Param("codigo"))
	var estudanteID string
	err := getDBClient(c).DB().QueryRowContext(c.Request.Context(), `SELECT id::text FROM projection_estudantes WHERE codigo_estudante=$1`, codigo).Scan(&estudanteID)
	if err == sql.ErrNoRows {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	actorID, typ, own, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	var somenteAcademia *string
	switch typ {
	case "estudante":
		if actorID.String() != estudanteID {
			utils.RespondWithForbiddenError(c, "você só pode consultar as suas mensalidades")
			return
		}
	case "academia":
		if !academiaPossuiVinculoMensalidade(c, codigo, own) {
			utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
			return
		}
		somenteAcademia = &own
	case "admin":
		if !financeAdminAllowed(c) {
			utils.RespondWithForbiddenError(c, "sem permissão financeira FPP")
			return
		}
	default:
		utils.RespondWithForbiddenError(c, "sem permissão para consultar mensalidades")
		return
	}
	out, err := FinanceiroService.ListMensalidades(c.Request.Context(), codigo, somenteAcademia)
	if err != nil {
		financeError(c, err)
		return
	}
	metodos := map[string][]string{}
	academias := map[string]bool{}
	for _, mensalidade := range out {
		academias[mensalidade.CodigoAcademia] = true
	}
	for academia := range academias {
		if itens, err := FinanceiroService.MetodosPagamentoMensalidade(c.Request.Context(), academia); err == nil {
			metodos[academia] = itens
		}
	}
	c.JSON(http.StatusOK, gin.H{"codigo_estudante": codigo, "mensalidades": out, "metodos_pagamento_por_academia": metodos})
}

// IniciarPagamentoMensalidades is intentionally outside the academy/admin
// financial group: only the authenticated student may express this intent.
func IniciarPagamentoMensalidades(c *gin.Context) {
	var in finance.MensalidadePagamentoInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok || typ != "estudante" {
		utils.RespondWithForbiddenError(c, "somente o estudante pode iniciar pagamento de mensalidades")
		return
	}
	var codigo string
	if err := getDBClient(c).DB().QueryRowContext(c.Request.Context(), `SELECT codigo_estudante FROM projection_estudantes WHERE id=$1`, id).Scan(&codigo); err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	in.CodigoEstudante = codigo
	out, err := FinanceiroService.IniciarPagamentoMensalidades(c.Request.Context(), in, id.String(), typ, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}

func AnularObrigacoesMensalidade(c *gin.Context)   { alterarObrigacoesMensalidadeHandler(c, false) }
func ReativarObrigacoesMensalidade(c *gin.Context) { alterarObrigacoesMensalidadeHandler(c, true) }

func alterarObrigacoesMensalidadeHandler(c *gin.Context, reativar bool) {
	var in finance.ObrigacaoMensalidadeInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	id, typ, own, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	// FPP admins intentionally cannot make either of these business decisions.
	if typ != "academia" {
		utils.RespondWithForbiddenError(c, "somente a academia dona pode anular ou reativar mensalidades")
		return
	}
	if strings.TrimSpace(in.CodigoAcademia) != "" && in.CodigoAcademia != own {
		utils.RespondWithForbiddenError(c, "a academia só pode alterar as suas próprias mensalidades")
		return
	}
	in.CodigoAcademia = own
	if !academiaPossuiVinculoMensalidade(c, in.CodigoEstudante, own) {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}
	var err error
	if reativar {
		err = FinanceiroService.ReativarObrigacoesMensalidade(c.Request.Context(), in, id.String(), typ, c.ClientIP())
	} else {
		err = FinanceiroService.AnularObrigacoesMensalidade(c.Request.Context(), in, id.String(), typ, c.ClientIP())
	}
	if err != nil {
		financeError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// The owner of an old obligation remains its academy after a transfer. The
// historical turma membership is consequently part of authorization, not just
// price resolution.
func academiaPossuiVinculoMensalidade(c *gin.Context, codigoEstudante, codigoAcademia string) bool {
	var ok bool
	err := getDBClient(c).DB().QueryRowContext(c.Request.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM projection_estudantes WHERE codigo_estudante=$1 AND codigo_academia=$2
			UNION ALL
			SELECT 1 FROM projection_turmas t
			WHERE t.codigo_academia=$2
			  AND (t.estudantes ? $1 OR EXISTS (SELECT 1 FROM jsonb_each(t.historico_estudantes_ano_letivo) h WHERE h.value ? $1))
		)`, codigoEstudante, codigoAcademia).Scan(&ok)
	return err == nil && ok
}

func getDBClient(c *gin.Context) *db.Client {
	v, _ := c.Get("dbClient")
	return v.(*db.Client)
}
