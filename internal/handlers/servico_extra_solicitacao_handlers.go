package handlers

import (
	"bytes"
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
	"time"
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
	guard, e := db.NewUniqueOperationGuard(getDbClient(c)).WithContext(c.Request.Context()).Reserve(
		"solicitacao_servico_extra:ativa",
		db.CanonicalGuardKey(sid.String(), est.CodigoEstudante),
		db.UniqueGuardOptions{UserID: uid.String(), UserType: "estudante"},
	)
	if e != nil {
		utils.RespondWithConflictError(c, "já existe solicitação ativa para este serviço")
		return
	}
	guardConsumed := false
	defer func() {
		if !guardConsumed {
			_ = guard.Release()
		}
	}()

	var documentoPath, documentoURL string
	// A solicitação pode não ter documento; nesse caso, uma requisição sem
	// multipart/form-data continua válida quando o serviço não o exige.
	_ = c.Request.ParseMultipartForm(MaxPDFUploadBytes + 1024)
	if fh, err := c.FormFile("documento"); err != nil {
		if serv.DocumentoObrigatorio {
			utils.RespondWithValidationError(c, fmt.Errorf("documento é obrigatório para este serviço"))
			return
		}
	} else {
		pdf, err := readAndValidatePDF("documento", fh)
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		provider := getStorageProvider(c)
		if provider == nil {
			utils.RespondWithInternalError(c, errors.New("storage não configurado"))
			return
		}
		path := fmt.Sprintf("%s/estudantes/%s/servicos_extras/%s.pdf", serv.CodigoAcademia, est.CodigoEstudante, sid)
		stored, err := provider.Upload(path, bytes.NewReader(pdf.data), pdf.size)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		documentoPath, documentoURL = stored.Path, stored.FileURL
	}
	s := aggregates.NewSolicitacaoServicoExtra()
	if e = s.Criar(sid, serv.CodigoAcademia, est.CodigoEstudante, documentoPath, documentoURL); e != nil {
		if documentoPath != "" {
			_ = getStorageProvider(c).Delete(documentoPath)
		}
		utils.RespondWithValidationError(c, e)
		return
	}
	if e = getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: uid.String(), UserType: "estudante", IP: c.ClientIP()}); e != nil {
		if documentoPath != "" {
			_ = getStorageProvider(c).Delete(documentoPath)
		}
		utils.RespondWithInternalError(c, e)
		return
	}
	if e = guard.Consume(s.GetID()); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	guardConsumed = true
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
		codigo, tipo, mes, ano, err := FinanceiroService.DadosServicoExtraDaCobranca(c.Request.Context(), out.Charge.ID.String())
		if err == nil && codigo != "" {
			switch tipo {
			case "taxa_inscricao":
				_ = efetivarVinculoServicoExtraPago(c, codigo)
			case "mensalidade", "preco_unico":
				_ = FinanceiroService.ConfirmarLancamentoServicoExtraPago(c.Request.Context(), codigo, tipo, ano, mes, id.String(), typ, c.ClientIP())
			}
		}
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

func DownloadDocumentoSolicitacaoServicoExtraAcademia(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	codigo, _, ok := academy(c)
	if !ok {
		return
	}
	if s.CodigoAcademia != codigo {
		utils.RespondWithForbiddenError(c, "solicitação não pertence à academia")
		return
	}
	downloadDocumentoServicoExtra(c, s)
}

func DownloadDocumentoSolicitacaoServicoExtraEstudante(c *gin.Context) {
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
	downloadDocumentoServicoExtra(c, s)
}

func downloadDocumentoServicoExtra(c *gin.Context, s *aggregates.SolicitacaoServicoExtra) {
	if s.DocumentoPath == "" {
		utils.RespondWithNotFoundError(c, "documento")
		return
	}
	streamDocumento(c, "documento_servico_extra", aggregates.DocumentoMatricula{Path: s.DocumentoPath, FileURL: s.DocumentoURL})
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

func pendenciasServicoExtra(c *gin.Context, s *aggregates.SolicitacaoServicoExtra) {
	if s.Status != aggregates.StatusInscricaoVinculada && s.Status != aggregates.StatusInscricaoCancelada {
		c.JSON(http.StatusOK, gin.H{"pendencias": []finance.ServicoExtraPendenciaView{}})
		return
	}
	serv, err := getServicosExtrasProjection(c).GetByID(s.ServicoExtraID)
	if err != nil || serv == nil {
		utils.RespondWithNotFoundError(c, "serviço extra")
		return
	}
	// ServicoExtraDTO.Preco e .TipoCobranca são *float64/*string (nulos quando
	// pago=false — ver CHECK chk_servico_extra_pago_campos na migration 118).
	// Este código só é alcançado quando a inscrição está vinculada a um serviço
	// com tipo_cobranca mensal/unico, o que implica pago=true e portanto ambos
	// não-nulos na prática; ainda assim, checamos explicitamente em vez de
	// desreferenciar direto, para nunca dar panic (500 cru) se essa invariante
	// for quebrada por qualquer caminho futuro. Sem isto, o build falhava:
	// "cannot use serv.TipoCobranca (variable of type *string) as string value".
	if serv.Preco == nil || serv.TipoCobranca == nil {
		utils.RespondWithInternalError(c, errors.New("serviço extra sem preço configurado"))
		return
	}
	fim := time.Now()
	if s.Status == aggregates.StatusInscricaoCancelada {
		fim = s.UpdatedAt
	}
	out, err := FinanceiroService.PendenciasServicoExtra(c.Request.Context(), s.GetID().String(), *serv.TipoCobranca, *serv.Preco, s.VinculadaEm, fim)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pendencias": out})
}
func MinhasPendenciasServicoExtra(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	id, _ := middleware.GetUserID(c)
	est, err := getEstudanteProjection(c).GetByID(id)
	if err != nil || est == nil || s.CodigoEstudante != est.CodigoEstudante {
		utils.RespondWithForbiddenError(c, "solicitação não pertence ao estudante")
		return
	}
	pendenciasServicoExtra(c, s)
}
func PendenciasServicoExtraAcademia(c *gin.Context) {
	s, ok := solicFromParam(c)
	if !ok {
		return
	}
	codigo, _, ok := academy(c)
	if !ok {
		return
	}
	if s.CodigoAcademia != codigo {
		utils.RespondWithForbiddenError(c, "solicitação não pertence à academia")
		return
	}
	pendenciasServicoExtra(c, s)
}
func IniciarPagamentoServicoExtraObrigacao(c *gin.Context) {
	var in finance.ServicoExtraObrigacaoPagamentoInput
	if c.ShouldBindJSON(&in) != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	id, typ, _, ok := financeActor(c)
	if !ok || typ != "estudante" {
		utils.RespondWithForbiddenError(c, "somente o estudante pode iniciar pagamento")
		return
	}
	sid, err := uuid.Parse(in.SolicitacaoID)
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("solicitacao_id inválido"))
		return
	}
	s, ok := loadSolicitacaoServicoExtra(c, sid)
	if !ok {
		return
	}
	var estudante string
	if err = getDBClient(c).DB().QueryRowContext(c.Request.Context(), `SELECT codigo_estudante FROM projection_estudantes WHERE id=$1`, id).Scan(&estudante); err != nil || s.CodigoEstudante != estudante {
		utils.RespondWithForbiddenError(c, "esta solicitação não pertence ao estudante autenticado")
		return
	}
	if s.Status != aggregates.StatusInscricaoVinculada {
		utils.RespondWithConflictError(c, "solicitação não possui obrigações pagáveis")
		return
	}
	serv, err := getServicosExtrasProjection(c).GetByID(s.ServicoExtraID)
	if err != nil || serv == nil {
		utils.RespondWithNotFoundError(c, "serviço extra")
		return
	}
	// Mesmo motivo do comentário em pendenciasServicoExtra acima:
	// ServicoExtraDTO.Preco é *float64, precisa ser desreferenciado.
	if serv.Preco == nil {
		utils.RespondWithInternalError(c, errors.New("serviço extra sem preço configurado"))
		return
	}
	out, err := FinanceiroService.IniciarPagamentoServicoExtraObrigacao(c.Request.Context(), in, serv.CodigoAcademia, *serv.Preco, serv.MetodosPagamento, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
func alterarObrigacaoServicoExtra(c *gin.Context, reativar bool) {
	var in struct {
		SolicitacaoID  string `json:"solicitacao_id"`
		TipoLancamento string `json:"tipo_lancamento"`
		Ano            int    `json:"ano"`
		Mes            int    `json:"mes"`
		Motivo         string `json:"motivo"`
	}
	if c.ShouldBindJSON(&in) != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	sid, err := uuid.Parse(in.SolicitacaoID)
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("solicitacao_id inválido"))
		return
	}
	s, ok := loadSolicitacaoServicoExtra(c, sid)
	if !ok {
		return
	}
	codigo, actor, ok := academy(c)
	if !ok {
		return
	}
	if s.CodigoAcademia != codigo {
		utils.RespondWithForbiddenError(c, "solicitação não pertence à academia")
		return
	}
	if reativar {
		err = FinanceiroService.ReativarObrigacaoServicoExtra(c.Request.Context(), in.SolicitacaoID, in.TipoLancamento, in.Ano, in.Mes, in.Motivo, actor.String(), "academia", c.ClientIP())
	} else {
		err = FinanceiroService.AnularObrigacaoServicoExtra(c.Request.Context(), in.SolicitacaoID, in.TipoLancamento, in.Ano, in.Mes, in.Motivo, actor.String(), "academia", c.ClientIP())
	}
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
func AnularObrigacaoServicoExtra(c *gin.Context)   { alterarObrigacaoServicoExtra(c, false) }
func ReativarObrigacaoServicoExtra(c *gin.Context) { alterarObrigacaoServicoExtra(c, true) }
