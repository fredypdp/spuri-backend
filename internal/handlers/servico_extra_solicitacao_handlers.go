package handlers

import (
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/utils"
	"strings"
)

func loadSolicitacaoServicoExtra(c *gin.Context, id uuid.UUID) (*aggregates.SolicitacaoServicoExtra, bool) {
	a, e := getRepository(c).WithContext(c.Request.Context()).Load(id, "SolicitacaoServicoExtra")
	if e != nil {
		utils.RespondWithNotFoundError(c, "solicitação de serviço extra")
		return nil, false
	}
	s, ok := a.(*aggregates.SolicitacaoServicoExtra)
	if !ok {
		utils.RespondWithInternalError(c, errors.New("tipo de aggregate inesperado"))
		return nil, false
	}
	return s, true
}
func SolicitarServicoExtra(c *gin.Context) {
	sid, e := uuid.Parse(c.Param("id"))
	if e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	uid, _ := middleware.GetUserID(c)
	est, e := getEstudanteProjection(c).GetByID(uid)
	if e != nil || est == nil || est.CodigoAcademia == nil {
		utils.RespondWithForbiddenError(c, "estudante não está vinculado a uma academia")
		return
	}
	serv, e := getServicosExtrasProjection(c).GetByID(sid)
	if e != nil || serv == nil || !serv.Ativo || serv.CodigoAcademia != *est.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "serviço extra indisponível")
		return
	}
	active, e := getSolicitacoesServicoExtraProjection(c).ExisteAtiva(sid, est.CodigoEstudante)
	if e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	if active {
		utils.RespondWithConflictError(c, "já existe solicitação ativa para este serviço")
		return
	}
	s := aggregates.NewSolicitacaoServicoExtra()
	if e = s.Criar(sid, serv.CodigoAcademia, est.CodigoEstudante, "", ""); e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	if e = getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: uid.String(), UserType: "estudante", IP: c.ClientIP()}); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": s})
}
func solicFromParam(c *gin.Context) (*aggregates.SolicitacaoServicoExtra, bool) {
	id, e := uuid.Parse(c.Param("id"))
	if e != nil {
		utils.RespondWithValidationError(c, e)
		return nil, false
	}
	return loadSolicitacaoServicoExtra(c, id)
}
func AprovarSolicitacaoServicoExtra(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	codigo, user, ok := academy(c)
	if !ok {
		return
	}
	if s.CodigoAcademia != codigo {
		utils.RespondWithForbiddenError(c, "solicitação não pertence à academia")
		return
	}
	serv, e := getServicosExtrasProjection(c).GetByID(s.ServicoExtraID)
	if e != nil || serv == nil {
		utils.RespondWithNotFoundError(c, "serviço extra")
		return
	}
	var valor float64
	if serv.ValorTaxaInscricao != nil {
		valor = *serv.ValorTaxaInscricao
	}
	if e = s.Aprovar(serv.TemTaxaInscricao, valor, serv.MetodosPagamentoTaxaInscricao, user); e != nil {
		utils.RespondWithConflictError(c, e.Error())
		return
	}
	if e = getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: user.String(), UserType: "academia", IP: c.ClientIP()}); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(200, gin.H{"status": s.Status})
}
func ReprovarSolicitacaoServicoExtra(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	codigo, user, ok := academy(c)
	if !ok {
		return
	}
	if s.CodigoAcademia != codigo {
		utils.RespondWithForbiddenError(c, "solicitação não pertence à academia")
		return
	}
	var in struct {
		Motivo string `json:"motivo_reprovacao"`
	}
	if c.ShouldBindJSON(&in) != nil || strings.TrimSpace(in.Motivo) == "" {
		utils.RespondWithValidationError(c, errors.New("motivo_reprovacao é obrigatório"))
		return
	}
	if e := s.Reprovar(in.Motivo, user); e != nil {
		utils.RespondWithConflictError(c, e.Error())
		return
	}
	if e := getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: user.String(), UserType: "academia", IP: c.ClientIP()}); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(200, gin.H{"status": s.Status})
}
func efetivarVinculoServicoExtraPago(c *gin.Context, id string) error {
	x, e := uuid.Parse(id)
	if e != nil {
		return e
	}
	s, ok := loadSolicitacaoServicoExtra(c, x)
	if !ok {
		return errors.New("solicitação não encontrada")
	}
	if s.Status != aggregates.StatusInscricaoAprovadaPendentePagamentoTaxa {
		return nil
	}
	if e = s.VincularAposPagamento(); e != nil {
		return e
	}
	return getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: "appypay:servico_extra", UserType: "sistema", IP: c.ClientIP()})
}
func IniciarPagamentoTaxaInscricaoServicoExtra(c *gin.Context) {
	var in finance.TaxaInscricaoServicoExtraPagamentoInput
	if c.ShouldBindJSON(&in) != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok || typ != "estudante" {
		utils.RespondWithForbiddenError(c, "somente o estudante pode iniciar pagamento")
		return
	}
	sid := c.Query("solicitacao_id")
	suid, e := uuid.Parse(sid)
	if e != nil {
		utils.RespondWithValidationError(c, errors.New("solicitacao_id inválido"))
		return
	}
	s, ok := loadSolicitacaoServicoExtra(c, suid)
	if !ok {
		return
	}
	var code string
	if e = getDBClient(c).DB().QueryRowContext(c.Request.Context(), `SELECT codigo_estudante FROM projection_estudantes WHERE id=$1`, id).Scan(&code); e != nil || s.CodigoEstudante != code {
		utils.RespondWithForbiddenError(c, "esta solicitação não pertence ao estudante autenticado")
		return
	}
	in.SolicitacaoID = sid
	out, e := FinanceiroService.IniciarPagamentoTaxaInscricaoServicoExtra(c.Request.Context(), in, c.ClientIP())
	if e != nil {
		financeError(c, e)
		return
	}
	if strings.EqualFold(out.Charge.Status, "success") {
		_ = efetivarVinculoServicoExtraPago(c, sid)
	}
	c.JSON(http.StatusCreated, out)
}
func cancelarSolicitacaoServicoExtra(c *gin.Context, s *aggregates.SolicitacaoServicoExtra, motivo, por, actor string) {
	var e error
	if s.Status == aggregates.StatusInscricaoAprovadaPendentePagamentoTaxa {
		e = FinanceiroService.CancelarCobrancaTaxaInscricaoServicoAberta(c.Request.Context(), s.GetID().String(), motivo, actor, por, c.ClientIP())
		if e == nil {
			e = s.CancelarAntesDaVinculacao(motivo, por)
		}
	} else if s.Status == aggregates.StatusInscricaoVinculada {
		e = s.Cancelar(motivo, por)
	} else {
		utils.RespondWithConflictError(c, fmt.Sprintf("solicitação em estado '%s' não pode ser cancelada", s.Status))
		return
	}
	if e != nil {
		utils.RespondWithConflictError(c, e.Error())
		return
	}
	if e = getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: actor, UserType: por, IP: c.ClientIP()}); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(200, gin.H{"status": s.Status})
}
func CancelarSolicitacaoServicoExtraAcademia(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	code, id, ok := academy(c)
	if !ok {
		return
	}
	if s.CodigoAcademia != code {
		utils.RespondWithForbiddenError(c, "solicitação não pertence à academia")
		return
	}
	var x struct {
		Motivo string `json:"motivo"`
	}
	_ = c.ShouldBindJSON(&x)
	cancelarSolicitacaoServicoExtra(c, s, x.Motivo, "academia", id.String())
}
func CancelarSolicitacaoServicoExtraEstudante(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	id, _ := middleware.GetUserID(c)
	est, e := getEstudanteProjection(c).GetByID(id)
	if e != nil || est == nil || s.CodigoEstudante != est.CodigoEstudante {
		utils.RespondWithForbiddenError(c, "solicitação não pertence ao estudante")
		return
	}
	var x struct {
		Motivo string `json:"motivo"`
	}
	_ = c.ShouldBindJSON(&x)
	cancelarSolicitacaoServicoExtra(c, s, x.Motivo, "estudante", id.String())
}

// ListarSolicitacoesServicoExtraAcademia retorna as solicitações da academia autenticada.
func ListarSolicitacoesServicoExtraAcademia(c *gin.Context) {
	codigo, _, ok := academy(c)
	if !ok {
		return
	}
	rows, err := getDBClient(c).DB().QueryContext(c.Request.Context(), `SELECT id,servico_extra_id,codigo_estudante,status,motivo_reprovacao,motivo_cancelamento,created_at,updated_at FROM projection_solicitacoes_servico_extra WHERE codigo_academia=$1 AND (NULLIF($2,'') IS NULL OR status=$2) AND (NULLIF($3,'') IS NULL OR servico_extra_id=$3::uuid) ORDER BY created_at DESC`, codigo, c.Query("status"), c.Query("servico_extra_id"))
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, service uuid.UUID
		var student, status string
		var repro, cancel *string
		var created, updated interface{}
		if err := rows.Scan(&id, &service, &student, &status, &repro, &cancel, &created, &updated); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "servico_extra_id": service, "codigo_estudante": student, "status": status, "motivo_reprovacao": repro, "motivo_cancelamento": cancel, "created_at": created, "updated_at": updated})
	}
	c.JSON(200, gin.H{"solicitacoes": out, "total": len(out)})
}
func GetSolicitacaoServicoExtraAcademia(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	code, _, ok := academy(c)
	if !ok {
		return
	}
	if s.CodigoAcademia != code {
		utils.RespondWithForbiddenError(c, "solicitação não pertence à academia")
		return
	}
	c.JSON(200, gin.H{"data": s})
}
func ListarMinhasInscricoesServicoExtra(c *gin.Context) {
	id, _ := middleware.GetUserID(c)
	est, e := getEstudanteProjection(c).GetByID(id)
	if e != nil || est == nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	rows, e := getDBClient(c).DB().QueryContext(c.Request.Context(), `SELECT id FROM projection_solicitacoes_servico_extra WHERE codigo_estudante=$1 AND (NULLIF($2,'') IS NULL OR status=$2) ORDER BY created_at DESC`, est.CodigoEstudante, c.Query("status"))
	if e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	defer rows.Close()
	out := []*aggregates.SolicitacaoServicoExtra{}
	for rows.Next() {
		var sid uuid.UUID
		if rows.Scan(&sid) != nil {
			continue
		}
		if s, ok := loadSolicitacaoServicoExtra(c, sid); ok {
			out = append(out, s)
		}
	}
	c.JSON(200, gin.H{"inscricoes": out, "total": len(out)})
}
