package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

// FinanceiroService is initialised with the application database in initDB.
var FinanceiroService = finance.NewService(nil)

func financeActor(c *gin.Context) (uuid.UUID, string, string, bool) {
	id, ok := middleware.GetUserID(c)
	if !ok {
		return uuid.Nil, "", "", false
	}
	typeName, ok := middleware.GetUserType(c)
	if !ok {
		return uuid.Nil, "", "", false
	}
	return id, typeName, c.GetString("codigo_academia"), true
}
func financeAdminAllowed(c *gin.Context) bool {
	_, t, _, ok := financeActor(c)
	if !ok || t != "admin" {
		return false
	}
	return verificarPermissaoAdmin(c, "fpp") == nil
}
func authorizeFinanceScope(c *gin.Context, context *string, academy *string) bool {
	_, t, own, ok := financeActor(c)
	if !ok {
		return false
	}
	if t == "academia" {
		if *context != "" && *context != finance.ContextoAcademia {
			return false
		}
		if *academy != "" && *academy != own {
			return false
		}
		*context = finance.ContextoAcademia
		*academy = own
		return true
	}
	return t == "admin" && financeAdminAllowed(c)
}

// credentialScopeAuthorized resolve o contexto/academia dono de uma
// credencial AppyPay pelo seu id e reaplica authorizeFinanceScope — o mesmo
// mecanismo que já garante que uma academia só mexe nas próprias credenciais
// e que um admin precisa da permissão "fpp". Usado pelas rotas de consulta e
// rotação do segredo de webhook. Já escreve a resposta de erro (404 ou 403)
// no contexto quando retorna false.
func credentialScopeAuthorized(c *gin.Context, id uuid.UUID) bool {
	contexto, academia, err := FinanceiroService.CredentialScope(c.Request.Context(), id)
	if err != nil {
		financeError(c, err)
		return false
	}
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para esta credencial AppyPay")
		return false
	}
	return true
}

func financeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, finance.ErrNotFound):
		utils.RespondWithNotFoundError(c, "recurso financeiro")
	case errors.Is(err, finance.ErrConflict):
		utils.RespondWithConflictError(c, "operação financeira equivalente em processamento")
	case errors.Is(err, finance.ErrUpstream):
		utils.RespondWithServiceUnavailable(c, err)
	default:
		utils.RespondWithValidationError(c, err)
	}
}

// CredencialAppyPayCriada é a resposta exclusiva de POST .../credenciais: é a
// única vez que o segredo de webhook aparece "de graça" numa resposta, fora
// do GET .../webhook-secret dedicado — porque é a única oportunidade em que o
// usuário ainda não tem como consultá-lo de outra forma.
type CredencialAppyPayCriada struct {
	finance.CredentialView
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

func ConfigurarCredencialAppyPay(c *gin.Context) {
	var in finance.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar estas credenciais")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, webhookSecret, err := FinanceiroService.ConfigureCredential(c.Request.Context(), nil, in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, CredencialAppyPayCriada{CredentialView: out, WebhookSecret: webhookSecret})
}

// RemoverCredencialAppyPay remove as credenciais AppyPay configuradas para
// o contexto do ator autenticado (academia própria, ou "spuri" para um
// admin com permissão "fpp"). A partir deste comando, qualquer tentativa
// de gerar cobrança nesse contexto volta a ser bloqueada por ausência de
// credenciais, exatamente como antes de nunca terem sido configuradas.
func RemoverCredencialAppyPay(c *gin.Context) {
	var in struct {
		ContextoTipo   string `json:"contexto_tipo"`
		CodigoAcademia string `json:"codigo_academia"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para remover estas credenciais")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	if err := FinanceiroService.RemoveCredential(c.Request.Context(), in.ContextoTipo, in.CodigoAcademia, id.String(), t, c.ClientIP()); err != nil {
		financeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func AtualizarCredencialAppyPay(c *gin.Context) {
	idParam, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("id inválido"))
		return
	}
	var in finance.CredentialInput
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para configurar estas credenciais")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	// O segundo retorno (segredo em texto plano) só vem preenchido quando a
	// credencial ainda não tinha nenhum segredo de webhook — não deveria
	// acontecer numa atualização de credencial já existente; se acontecer, o
	// usuário ainda pode recuperá-lo em seguida via GET .../webhook-secret.
	out, _, err := FinanceiroService.ConfigureCredential(c.Request.Context(), &idParam, in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func ListarCredenciaisAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para consultar credenciais financeiras")
		return
	}
	out, err := FinanceiroService.ListCredentials(c.Request.Context(), contexto, academia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// ConsultarSegredoWebhookAppyPay devolve o segredo de webhook atual em texto
// plano. Só o dono do contexto (a própria academia, ou admin com permissão
// "fpp") pode consultar.
func ConsultarSegredoWebhookAppyPay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("id inválido"))
		return
	}
	if !credentialScopeAuthorized(c, id) {
		return
	}
	secret, err := FinanceiroService.WebhookSecret(c.Request.Context(), id)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"webhook_secret": secret, "webhook_header_name": finance.WebhookHeaderName})
}

// RotacionarSegredoWebhookAppyPay gera um novo segredo de webhook,
// invalidando o anterior imediatamente. Mesma autorização de
// ConsultarSegredoWebhookAppyPay.
func RotacionarSegredoWebhookAppyPay(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, errors.New("id inválido"))
		return
	}
	if !credentialScopeAuthorized(c, id) {
		return
	}
	actorID, actorType, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	secret, err := FinanceiroService.RotateWebhookSecret(c.Request.Context(), id, actorID.String(), actorType, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"webhook_secret": secret, "webhook_header_name": finance.WebhookHeaderName})
}

func CriarCobrancaAppyPay(c *gin.Context) {
	var in finance.ChargeRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.CreateCharge(c.Request.Context(), in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
func GerarQRCodeAppyPay(c *gin.Context) {
	var in finance.QRCodeRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.CreateGPOQRCode(c.Request.Context(), in, id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, out)
}
func ConsultarCobrancaAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	id, t, _, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	out, err := FinanceiroService.ConsultCharge(c.Request.Context(), contexto, academia, c.Param("id"), id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	// A cobrança pode ser de matrícula: diferente de uma cobrança de
	// mensalidade (confirmada via confirmMensalidadeCharge dentro do
	// próprio ConsultCharge), a efetivação do vínculo de matrícula
	// (criação do estudante e transição da solicitação) não faz parte do
	// pacote financeiro e precisa ser acionada aqui, exatamente como já é
	// feito em ReceberWebhookAppyPay e na criação síncrona da cobrança em
	// IniciarPagamentoMatricula. Sem isto, uma cobrança de matrícula que só
	// é confirmada pela AppyPay quando alguém consulta o status (fluxo
	// normal para GPO/REF, que nunca retornam "success" na criação) nunca
	// efetiva a matrícula.
	if strings.EqualFold(strings.TrimSpace(out.Status), "success") {
		if codigo, err := FinanceiroService.CodigoSolicitacaoDaCobranca(c.Request.Context(), c.Param("id")); err == nil && codigo != "" {
			if err := efetivarVinculoMatriculaPaga(c, codigo); err != nil {
				utils.RespondWithInternalError(c, err)
				return
			}
		}
	}
	c.JSON(http.StatusOK, out)
}

// ListarCobrancasAppyPay lista cobranças (mensalidade, matrícula ou avulsa)
// do contexto autorizado, com filtros opcionais por estado e origem e
// paginação — resolve o Problema 1 documentado em
// docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md.
// Mesma autorização de ConsultarCobrancaAppyPay/ListarCredenciaisAppyPay:
// uma academia só vê as próprias cobranças; um admin precisa da permissão
// "fpp" e pode consultar qualquer contexto/academia via query string.

// parseOptionalUUIDQuery lê um parâmetro de query opcional como UUID. Devolve
// nil quando o parâmetro não foi informado, e erro quando foi informado mas
// não é um UUID válido.
func parseOptionalUUIDQuery(c *gin.Context, param string) (*uuid.UUID, error) {
	raw := strings.TrimSpace(c.Query(param))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s inválido", param)
	}
	return &id, nil
}

func ListarCobrancasAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	turmaID, err := parseOptionalUUIDQuery(c, "turma_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	cursoID, err := parseOptionalUUIDQuery(c, "curso_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	anoAcademico := c.Query("ano_academico")
	anoLetivo := c.Query("ano_letivo")
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := FinanceiroService.ListCobrancas(c.Request.Context(), contexto, academia, c.QueryArray("estado"), c.QueryArray("tipo"), turmaID, cursoID, anoAcademico, anoLetivo, limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	body := gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset}
	// pendencias_sem_cobranca só é computado quando pelo menos um dos
	// quatro filtros de escopo (turma_id, curso_id, ano_academico,
	// ano_letivo) é informado junto de codigo_academia — sem isso, a
	// varredura seria sobre a academia inteira sem limite. Ver
	// finance.PendenciasSemCobranca.
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		pendencias, err := FinanceiroService.PendenciasSemCobranca(c.Request.Context(), academia, turmaID, cursoID, anoAcademico, anoLetivo)
		if err != nil {
			financeError(c, err)
			return
		}
		body["pendencias_sem_cobranca"] = pendencias
	}
	c.JSON(http.StatusOK, body)
}

// ConsultarCobrancasEstudante lista TODAS as cobranças (mensalidade,
// matrícula ou avulsa) já associadas a um estudante, em qualquer estado —
// diferente de ListarCobrancasAppyPay (academia/admin, dentro do próprio
// contexto), esta rota é acessível ao próprio estudante para consultar o seu
// histórico completo de pagamentos, exatamente como ConsultarMensalidadesEstudante
// já faz para as obrigações de mensalidade (mesmo desenho de autorização em
// três vias: estudante só o próprio código, academia só com vínculo e
// restrita à própria academia, admin com permissão "fpp").
func ConsultarCobrancasEstudante(c *gin.Context) {
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
			utils.RespondWithForbiddenError(c, "você só pode consultar os seus próprios pagamentos")
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
		utils.RespondWithForbiddenError(c, "sem permissão para consultar pagamentos")
		return
	}
	turmaID, err := parseOptionalUUIDQuery(c, "turma_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	cursoID, err := parseOptionalUUIDQuery(c, "curso_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	anoAcademico := c.Query("ano_academico")
	anoLetivo := c.Query("ano_letivo")
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, c.QueryArray("estado"), c.QueryArray("tipo"), turmaID, cursoID, anoAcademico, anoLetivo, limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	body := gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset}
	// pendencias_sem_cobranca é sempre calculado aqui (sem exigir nenhum
	// filtro extra): esta consulta já está inerentemente delimitada a UM
	// estudante, então não há o mesmo risco de varredura sem limite que
	// existe em ListarCobrancasAppyPay. Ver
	// finance.PendenciasSemCobrancaEstudante.
	pendencias, err := FinanceiroService.PendenciasSemCobrancaEstudante(c.Request.Context(), codigo, somenteAcademia)
	if err != nil {
		financeError(c, err)
		return
	}
	body["pendencias_sem_cobranca"] = pendencias
	c.JSON(http.StatusOK, body)
}

// CancelarCobrancaAppyPay intentionally does not use authorizeFinanceScope:
// FPP admins may cancel only Spuri's own charges, never a charge belonging to
// an academy. The service repeats this ownership check before recording.
func CancelarCobrancaAppyPay(c *gin.Context) {
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.RespondWithValidationError(c, errors.New("payload inválido"))
		return
	}
	id, actorType, ownAcademy, ok := financeActor(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}
	contexto, academia := "", ""
	switch actorType {
	case "academia":
		contexto, academia = finance.ContextoAcademia, ownAcademy
	case "admin":
		if !financeAdminAllowed(c) {
			utils.RespondWithForbiddenError(c, "sem permissão para cancelar esta cobrança")
			return
		}
		contexto = finance.ContextoSpuri
	default:
		utils.RespondWithForbiddenError(c, "sem permissão para cancelar esta cobrança")
		return
	}
	out, err := FinanceiroService.CancelCharge(c.Request.Context(), contexto, academia, c.Param("id"), in.Motivo, id.String(), actorType, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
func ReceberWebhookAppyPay(metodo string) gin.HandlerFunc {
	return func(c *gin.Context) {
		owner, err := FinanceiroService.AuthenticateWebhook(c.Request.Context(), c.Request.Header)
		if err != nil {
			c.Status(http.StatusUnauthorized)
			return
		}
		var payload map[string]any
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		eventID := webhookID(payload)
		if eventID == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if _, err := FinanceiroService.AcceptWebhook(c.Request.Context(), metodo, eventID, owner, payload); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		if isSuccessfulWebhook(payload) {
			if codigo, err := FinanceiroService.CodigoSolicitacaoDaCobranca(c.Request.Context(), eventID); err == nil && codigo != "" {
				if err := efetivarVinculoMatriculaPaga(c, codigo); err != nil {
					c.Status(http.StatusInternalServerError)
					return
				}
			}
		}
		c.Status(http.StatusOK)
	}
}
func isSuccessfulWebhook(payload map[string]any) bool {
	return strings.EqualFold(strings.TrimSpace(webhookStatus(payload)), "success")
}
func webhookStatus(payload map[string]any) string {
	for _, k := range []string{"status", "state"} {
		if v, ok := payload[k].(string); ok {
			return v
		}
	}
	return ""
}
func webhookID(payload map[string]any) string {
	for _, k := range []string{"id", "merchantTransactionId", "merchant_transaction_id"} {
		if v, ok := payload[k].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
