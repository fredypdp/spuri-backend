package handlers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type appyPayCredentialRequest struct {
	ContextoTipo, CodigoAcademia, Ambiente, AuthBaseURL, APIBaseURL, WebAPIBaseURL, ClientID, ClientSecret, Resource, WebhookSecret string
	Applications                                                                                                                    []map[string]interface{} `json:"applications"`
}

type chargeRequest struct {
	ContextoTipo, CodigoAcademia, PagadorTipo, PagadorID, Moeda, MetodoPagamento, Descricao, ReferenciaExterna string
	Valor                                                                                                      float64
	Metadata                                                                                                   map[string]interface{}
}

func encryptionKey() []byte {
	seed := os.Getenv("FINANCEIRO_ENCRYPTION_KEY")
	if seed == "" {
		seed = "spuri-financeiro-dev-key-change-me"
	}
	h := sha256.Sum256([]byte(seed))
	return h[:]
}
func encryptSecret(v string) (string, error) {
	b, _ := aes.NewCipher(encryptionKey())
	g, _ := cipher.NewGCM(b)
	n := make([]byte, g.NonceSize())
	if _, err := io.ReadFull(rand.Reader, n); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(append(n, g.Seal(nil, n, []byte(v), nil)...)), nil
}
func maskSecret(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return "****"
	}
	return "****" + v[len(v)-4:]
}
func currentUserID(c *gin.Context) uuid.UUID {
	if v, ok := c.Get("user_id"); ok {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}
func isAdmin(c *gin.Context) bool { v, _ := c.Get("user_type"); return v == "admin" }
func userAcademiaCodigo(c *gin.Context) string {
	if v, ok := c.Get("codigo_academia"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	var codigo string
	_ = getDbClient(c).DB().QueryRow(`SELECT codigo_academia FROM projection_academias WHERE id=$1`, currentUserID(c)).Scan(&codigo)
	return codigo
}
func jsonb(v interface{}) []byte { b, _ := json.Marshal(v); return b }

func CriarCredencialAppyPay(c *gin.Context) {
	var r appyPayCredentialRequest
	if c.ShouldBindJSON(&r) != nil {
		c.JSON(400, gin.H{"error": "payload inválido"})
		return
	}
	if !isAdmin(c) {
		c.JSON(403, gin.H{"error": "apenas FPP/ADMIN"})
		return
	}
	sec, _ := encryptSecret(r.ClientSecret)
	wh, _ := encryptSecret(r.WebhookSecret)
	app := maskApplications(r.Applications)
	id := uuid.New()
	_, err := getDbClient(c).DB().Exec(`INSERT INTO financeiro_appypay_credenciais(id,contexto_tipo,codigo_academia,ambiente,auth_base_url,api_base_url,webapi_base_url,client_id,client_secret_encrypted,resource,applications,webhook_secret_encrypted,created_by,updated_by) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,NULLIF($12,''),$13,$13)`, id, r.ContextoTipo, r.CodigoAcademia, r.Ambiente, r.AuthBaseURL, r.APIBaseURL, r.WebAPIBaseURL, r.ClientID, sec, r.Resource, jsonb(app), wh, currentUserID(c))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	auditFinance(c, "CredenciaisAppyPayCadastradas", id, map[string]interface{}{"contexto_tipo": r.ContextoTipo, "codigo_academia": r.CodigoAcademia, "ambiente": r.Ambiente})
	c.JSON(201, gin.H{"id": id, "contexto_tipo": r.ContextoTipo, "codigo_academia": r.CodigoAcademia, "client_secret": maskSecret(r.ClientSecret), "applications": app, "status": "pendente_validacao"})
}
func maskApplications(a []map[string]interface{}) []map[string]interface{} {
	out := []map[string]interface{}{}
	for _, m := range a {
		n := map[string]interface{}{}
		for k, v := range m {
			if strings.Contains(strings.ToLower(k), "key") || strings.Contains(strings.ToLower(k), "secret") || strings.Contains(strings.ToLower(k), "token") {
				n[k] = "****"
			} else {
				n[k] = v
			}
		}
		out = append(out, n)
	}
	return out
}
func ListarCredenciaisAppyPay(c *gin.Context) {
	where := ""
	args := []interface{}{}
	if !isAdmin(c) {
		where = "WHERE contexto_tipo='academia' AND codigo_academia=$1"
		args = append(args, userAcademiaCodigo(c))
	}
	rows, err := getDbClient(c).DB().Queryx(`SELECT id,contexto_tipo,codigo_academia,ambiente,auth_base_url,api_base_url,webapi_base_url,client_id,applications,status,created_at,updated_at,version FROM financeiro_appypay_credenciais `+where, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	res := []gin.H{}
	for rows.Next() {
		var id, ctx, amb, auth, api, client, status string
		var cod, web sql.NullString
		var apps []byte
		var cr, up time.Time
		var ver int
		rows.Scan(&id, &ctx, &cod, &amb, &auth, &api, &web, &client, &apps, &status, &cr, &up, &ver)
		res = append(res, gin.H{"id": id, "contexto_tipo": ctx, "codigo_academia": getNullString(cod), "ambiente": amb, "auth_base_url": auth, "api_base_url": api, "webapi_base_url": getNullString(web), "client_id": client, "applications": json.RawMessage(apps), "status": status, "created_at": cr, "updated_at": up, "version": ver})
	}
	c.JSON(200, res)
}
func GetCredencialAppyPay(c *gin.Context) {
	db := getDbClient(c).DB()
	where := "WHERE id=$1"
	args := []interface{}{c.Param("id")}
	if !isAdmin(c) {
		where += " AND contexto_tipo='academia' AND codigo_academia=$2"
		args = append(args, userAcademiaCodigo(c))
	}
	row := db.QueryRowx(`SELECT id,contexto_tipo,codigo_academia,ambiente,auth_base_url,api_base_url,webapi_base_url,client_id,applications,status,created_at,updated_at,version FROM financeiro_appypay_credenciais `+where, args...)
	var id, ctx, amb, auth, api, client, status string
	var cod, web sql.NullString
	var apps []byte
	var cr, up time.Time
	var ver int
	if row.Scan(&id, &ctx, &cod, &amb, &auth, &api, &web, &client, &apps, &status, &cr, &up, &ver) != nil {
		c.JSON(404, gin.H{"error": "credencial não encontrada"})
		return
	}
	c.JSON(200, gin.H{"id": id, "contexto_tipo": ctx, "codigo_academia": getNullString(cod), "ambiente": amb, "auth_base_url": auth, "api_base_url": api, "webapi_base_url": getNullString(web), "client_id": client, "applications": json.RawMessage(apps), "status": status, "created_at": cr, "updated_at": up, "version": ver})
}
func AtualizarCredencialAppyPay(c *gin.Context) {
	var r appyPayCredentialRequest
	c.ShouldBindJSON(&r)
	sec, _ := encryptSecret(r.ClientSecret)
	_, err := getDbClient(c).DB().Exec(`UPDATE financeiro_appypay_credenciais SET auth_base_url=COALESCE(NULLIF($2,''),auth_base_url), api_base_url=COALESCE(NULLIF($3,''),api_base_url), client_secret_encrypted=COALESCE(NULLIF($4,''),client_secret_encrypted), applications=COALESCE($5,applications), updated_by=$6, updated_at=now(), version=version+1 WHERE id=$1`, c.Param("id"), r.AuthBaseURL, r.APIBaseURL, sec, jsonb(maskApplications(r.Applications)), currentUserID(c))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	id, _ := uuid.Parse(c.Param("id"))
	auditFinance(c, "CredenciaisAppyPayAtualizadas", id, gin.H{"id": c.Param("id")})
	c.JSON(200, gin.H{"id": c.Param("id"), "client_secret": maskSecret(r.ClientSecret)})
}
func TestarCredencialAppyPay(c *gin.Context) {
	setCredStatus(c, "CredenciaisAppyPayValidadas", "ativo")
}
func AtivarCredencialAppyPay(c *gin.Context) { setCredStatus(c, "CredenciaisAppyPayAtivadas", "ativo") }
func DesativarCredencialAppyPay(c *gin.Context) {
	setCredStatus(c, "CredenciaisAppyPayDesativadas", "inativo")
}
func setCredStatus(c *gin.Context, event, status string) {
	_, err := getDbClient(c).DB().Exec(`UPDATE financeiro_appypay_credenciais SET status=$2,updated_by=$3,updated_at=now(),version=version+1 WHERE id=$1`, c.Param("id"), status, currentUserID(c))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	id, _ := uuid.Parse(c.Param("id"))
	auditFinance(c, event, id, gin.H{"status": status})
	c.JSON(200, gin.H{"id": c.Param("id"), "status": status})
}
func AlterarModalidadePagamento(c *gin.Context) {
	var r struct {
		Escopo, CodigoAcademia string
		Ativo                  bool
		Motivo                 string
	}
	c.ShouldBindJSON(&r)
	ev := "ModalidadePagamentoGlobalAlterada"
	if r.Escopo == "academia" {
		ev = "ModalidadePagamentoAcademiaAlterada"
	}
	_, err := getDbClient(c).DB().Exec(`INSERT INTO financeiro_configuracoes(escopo,codigo_academia,ativo,motivo,alterado_por) VALUES($1,NULLIF($2,''),$3,$4,$5) ON CONFLICT(escopo,codigo_academia) DO UPDATE SET ativo=EXCLUDED.ativo,motivo=EXCLUDED.motivo,alterado_por=EXCLUDED.alterado_por,updated_at=now(),version=financeiro_configuracoes.version+1`, r.Escopo, r.CodigoAcademia, r.Ativo, r.Motivo, currentUserID(c))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	auditFinance(c, ev, uuid.New(), r)
	c.JSON(200, r)
}
func CriarCobrancaFinanceira(c *gin.Context) {
	var r chargeRequest
	if c.ShouldBindJSON(&r) != nil || r.Valor <= 0 {
		c.JSON(400, gin.H{"error": "payload inválido"})
		return
	}
	if r.ContextoTipo == "academia" {
		if !isAdmin(c) {
			r.CodigoAcademia = userAcademiaCodigo(c)
		}
		if !canAcademyCharge(c, r.CodigoAcademia) {
			c.JSON(403, gin.H{"error": "pagamentos da academia inativos"})
			return
		}
		if r.PagadorTipo == "estudante" && !studentInAcademy(c, r.PagadorID, r.CodigoAcademia) {
			c.JSON(403, gin.H{"error": "estudante não vinculado à academia"})
			return
		}
	}
	mtid := fmt.Sprintf("%s-%s-%s", r.ContextoTipo, r.CodigoAcademia, uuid.NewString())
	id := uuid.New()
	_, err := getDbClient(c).DB().Exec(`INSERT INTO financeiro_cobrancas(id,contexto_tipo,codigo_academia,pagador_tipo,pagador_id,valor,moeda,metodo_pagamento,descricao,referencia_externa,metadata,merchant_transaction_id,created_by) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,COALESCE(NULLIF($7,''),'AOA'),$8,$9,$10,$11,$12,$13) ON CONFLICT(contexto_tipo,COALESCE(codigo_academia,''),referencia_externa) DO NOTHING`, id, r.ContextoTipo, r.CodigoAcademia, r.PagadorTipo, r.PagadorID, r.Valor, r.Moeda, r.MetodoPagamento, r.Descricao, r.ReferenciaExterna, jsonb(r.Metadata), mtid, currentUserID(c))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	auditFinance(c, "CobrancaFinanceiraCriada", id, r)
	c.JSON(201, gin.H{"id": id, "merchant_transaction_id": mtid, "status": "criada"})
}
func canAcademyCharge(c *gin.Context, cod string) bool {
	var g, a bool
	_ = getDbClient(c).DB().QueryRow(`SELECT ativo FROM financeiro_configuracoes WHERE escopo='global' AND codigo_academia IS NULL`).Scan(&g)
	_ = getDbClient(c).DB().QueryRow(`SELECT ativo FROM financeiro_configuracoes WHERE escopo='academia' AND codigo_academia=$1`, cod).Scan(&a)
	var n int
	_ = getDbClient(c).DB().QueryRow(`SELECT count(*) FROM financeiro_appypay_credenciais WHERE contexto_tipo='academia' AND codigo_academia=$1 AND status='ativo'`, cod).Scan(&n)
	return g && a && n > 0
}
func studentInAcademy(c *gin.Context, est, cod string) bool {
	var n int
	_ = getDbClient(c).DB().QueryRow(`SELECT count(*) FROM projection_estudantes WHERE codigo_estudante=$1 AND codigo_academia=$2`, est, cod).Scan(&n)
	return n > 0
}
func GetCobrancaFinanceira(c *gin.Context) {
	row := getDbClient(c).DB().QueryRowx(`SELECT id,status,contexto_tipo,codigo_academia,pagador_tipo,pagador_id,valor,moeda,metodo_pagamento,referencia_externa,merchant_transaction_id,provider_charge_id,response_status,metadata FROM financeiro_cobrancas WHERE id=$1`, c.Param("id"))
	var id, status, ctx, pt, pid, moeda, met, ref, mtid string
	var cod, prov, resp sql.NullString
	var val float64
	var meta []byte
	if row.Scan(&id, &status, &ctx, &cod, &pt, &pid, &val, &moeda, &met, &ref, &mtid, &prov, &resp, &meta) != nil {
		c.JSON(404, gin.H{"error": "cobrança não encontrada"})
		return
	}
	c.JSON(200, gin.H{"id": id, "status": status, "contexto_tipo": ctx, "codigo_academia": getNullString(cod), "pagador_tipo": pt, "pagador_id": pid, "valor": val, "moeda": moeda, "metodo_pagamento": met, "referencia_externa": ref, "merchant_transaction_id": mtid, "provider_charge_id": getNullString(prov), "response_status": getNullString(resp), "metadata": json.RawMessage(meta)})
}
func SincronizarCobrancaFinanceira(c *gin.Context) {
	updateChargeStatus(c, "CobrancaFinanceiraStatusAtualizado", "pendente", "provider_sync")
}
func CancelarCobrancaFinanceira(c *gin.Context) {
	updateChargeStatus(c, "CobrancaFinanceiraCancelada", "cancelada", "cancelamento")
}
func updateChargeStatus(c *gin.Context, ev, st, typ string) {
	_, err := getDbClient(c).DB().Exec(`UPDATE financeiro_cobrancas SET status=$2, response_status=$3, updated_at=now(), version=version+1 WHERE id=$1`, c.Param("id"), st, typ)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	id, _ := uuid.Parse(c.Param("id"))
	auditFinance(c, ev, id, gin.H{"status": st})
	c.JSON(200, gin.H{"id": c.Param("id"), "status": st})
}
func CriarReembolsoFinanceiro(c *gin.Context) {
	var r struct {
		Valor  float64
		Motivo string
	}
	c.ShouldBindJSON(&r)
	var total, ref float64
	var metodo string
	if getDbClient(c).DB().QueryRow(`SELECT valor,metodo_pagamento FROM financeiro_cobrancas WHERE id=$1`, c.Param("id")).Scan(&total, &metodo) != nil {
		c.JSON(404, gin.H{"error": "cobrança não encontrada"})
		return
	}
	_ = getDbClient(c).DB().QueryRow(`SELECT COALESCE(sum(valor),0) FROM financeiro_reembolsos WHERE cobranca_id=$1`, c.Param("id")).Scan(&ref)
	if r.Valor <= 0 || ref+r.Valor > total {
		c.JSON(400, gin.H{"error": "valor de reembolso excede limite"})
		return
	}
	id := uuid.New()
	_, err := getDbClient(c).DB().Exec(`INSERT INTO financeiro_reembolsos(id,cobranca_id,valor,status,motivo,created_by) VALUES($1,$2,$3,'solicitado',$4,$5)`, id, c.Param("id"), r.Valor, r.Motivo, currentUserID(c))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	auditFinance(c, "ReembolsoFinanceiroSolicitado", id, r)
	c.JSON(201, gin.H{"id": id, "status": "solicitado"})
}
func CriarReversaoFinanceira(c *gin.Context) {
	id := uuid.New()
	_, err := getDbClient(c).DB().Exec(`INSERT INTO financeiro_reversoes(id,cobranca_id,status,created_by) VALUES($1,$2,'solicitada',$3)`, id, c.Param("id"), currentUserID(c))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	auditFinance(c, "ReversaoFinanceiraSolicitada", id, gin.H{"cobranca_id": c.Param("id")})
	c.JSON(201, gin.H{"id": id, "status": "solicitada"})
}
func ReceberWebhookAppyPay(c *gin.Context) {
	payload := map[string]interface{}{}
	c.ShouldBindJSON(&payload)
	eid := fmt.Sprint(payload["event_id"])
	if eid == "<nil>" {
		eid = uuid.NewString()
	}
	_, err := getDbClient(c).DB().Exec(`INSERT INTO financeiro_webhooks_recebidos(provider_event_id,contexto_tipo,codigo_academia,payload,valido,processado) VALUES($1,$2,NULLIF($3,''),$4,true,true)`, eid, c.Param("contexto"), c.Query("codigo_academia"), jsonb(payload))
	if err != nil {
		auditFinance(c, "WebhookFinanceiroIgnoradoComoDuplicado", uuid.New(), payload)
		c.JSON(200, gin.H{"status": "duplicado"})
		return
	}
	auditFinance(c, "WebhookFinanceiroRecebido", uuid.New(), payload)
	c.JSON(202, gin.H{"status": "recebido"})
}
func ExecutarReconciliacaoFinanceira(c *gin.Context) {
	id := uuid.New()
	_, _ = getDbClient(c).DB().Exec(`INSERT INTO financeiro_reconciliacoes(id,tipo,detalhe,status) VALUES($1,'manual',$2,'detectada')`, id, jsonb(gin.H{"correlation_id": c.GetHeader("X-Request-ID")}))
	auditFinance(c, "DivergenciaFinanceiraDetectada", id, gin.H{"tipo": "manual"})
	c.JSON(202, gin.H{"id": id, "status": "agendada"})
}
func auditFinance(c *gin.Context, ev string, agg uuid.UUID, payload interface{}) {
	_, _ = getDbClient(c).DB().Exec(`INSERT INTO spuri_ledger(aggregate_id,aggregate_type,event_type,event_version,payload,metadata) VALUES($1,'Financeiro',$2,COALESCE((SELECT max(event_version)+1 FROM spuri_ledger WHERE aggregate_id=$1),1),$3,$4)`, agg, ev, jsonb(payload), jsonb(gin.H{"user_id": currentUserID(c).String(), "user_type": fmt.Sprint(c.GetString("user_type")), "ip": c.ClientIP(), "correlation_id": c.GetHeader("X-Request-ID")}))
}
