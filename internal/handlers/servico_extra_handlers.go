package handlers

import (
	"encoding/json"
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

type servicoExtraPayload struct {
	Nome                          string
	Descricao                     string
	Categoria                     string
	Pago                          bool
	Preco                         float64
	TipoCobranca                  string
	MetodosPagamento              []string
	TemTaxaInscricao              bool
	ValorTaxaInscricao            float64
	MetodosPagamentoTaxaInscricao []string
	AnosAcademicosDisponiveis     []string
	DocumentoObrigatorio          bool
	DocumentoInstrucoes           string
	DetalhesPersonalizados        map[string]interface{}
	informado                     map[string]bool
}

func bindServicoExtraPayload(c *gin.Context, r *servicoExtraPayload) error {
	var raw map[string]json.RawMessage
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(&raw); e != nil {
		return fmt.Errorf("dados invalidos")
	}
	allowed := map[string]bool{"nome": true, "descricao": true, "categoria": true, "pago": true, "preco": true, "tipo_cobranca": true, "metodos_pagamento": true, "tem_taxa_inscricao": true, "valor_taxa_inscricao": true, "metodos_pagamento_taxa_inscricao": true, "anos_academicos_disponiveis": true, "documento_obrigatorio": true, "documento_instrucoes": true, "detalhes_personalizados": true}
	for k := range raw {
		if !allowed[k] {
			return fmt.Errorf("campo não suportado em serviço extra: %s", k)
		}
	}
	b, _ := json.Marshal(raw)
	if e := json.Unmarshal(b, r); e != nil {
		return fmt.Errorf("dados invalidos")
	}
	r.informado = map[string]bool{}
	for k := range raw {
		r.informado[k] = true
	}
	return nil
}
func academy(c *gin.Context) (string, uuid.UUID, bool) {
	id, _ := middleware.GetUserID(c)
	a, e := getAcademiaProjection(c).GetByID(id)
	if e != nil || a == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return "", id, false
	}
	return a.CodigoAcademia, id, true
}
func ptr[T any](v T) *T { return &v }
func CriarServicoExtra(c *gin.Context) {
	var r servicoExtraPayload
	if e := bindServicoExtraPayload(c, &r); e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	codigo, id, ok := academy(c)
	if !ok {
		return
	}
	if strings.TrimSpace(r.Nome) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("nome é obrigatório"))
		return
	}
	if r.Pago || r.TemTaxaInscricao {
		has, e := FinanceiroService.HasCredential(c.Request.Context(), finance.ContextoAcademia, codigo)
		if e != nil {
			utils.RespondWithInternalError(c, e)
			return
		}
		if !has {
			utils.RespondWithValidationError(c, fmt.Errorf("não é possível criar um serviço pago ou com taxa de inscrição sem credenciais AppyPay configuradas para a academia"))
			return
		}
	}
	s := aggregates.NewServicoExtra()
	if e := s.Criar(codigo, r.Nome, r.Descricao, r.Categoria, r.Pago, r.Preco, r.TipoCobranca, r.MetodosPagamento, r.TemTaxaInscricao, r.ValorTaxaInscricao, r.MetodosPagamentoTaxaInscricao, r.AnosAcademicosDisponiveis, r.DocumentoObrigatorio, r.DocumentoInstrucoes, r.DetalhesPersonalizados, id); e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	if e := getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: id.String(), UserType: "academia", IP: c.ClientIP()}); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "serviço extra criado com sucesso", "data": s})
}
func loadServico(c *gin.Context) (*aggregates.ServicoExtra, uuid.UUID, bool) {
	id, e := uuid.Parse(c.Param("id"))
	if e != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de serviço extra inválido"))
		return nil, uuid.Nil, false
	}
	codigo, user, ok := academy(c)
	if !ok {
		return nil, user, false
	}
	x, e := getRepository(c).Load(id, "ServicoExtra")
	if e != nil {
		utils.RespondWithNotFoundError(c, "serviço extra")
		return nil, user, false
	}
	s, ok := x.(*aggregates.ServicoExtra)
	if !ok || s.CodigoAcademia != codigo {
		utils.RespondWithForbiddenError(c, "serviço extra não pertence a esta academia")
		return nil, user, false
	}
	return s, user, true
}
func AtualizarServicoExtra(c *gin.Context) {
	var r servicoExtraPayload
	if e := bindServicoExtraPayload(c, &r); e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	s, id, ok := loadServico(c)
	if !ok {
		return
	}
	pago := s.Pago
	if r.informado["pago"] {
		pago = r.Pago
	}
	taxa := s.TemTaxaInscricao
	if r.informado["tem_taxa_inscricao"] {
		taxa = r.TemTaxaInscricao
	}
	if pago || taxa {
		has, e := FinanceiroService.HasCredential(c.Request.Context(), finance.ContextoAcademia, s.CodigoAcademia)
		if e != nil {
			utils.RespondWithInternalError(c, e)
			return
		}
		if !has {
			utils.RespondWithValidationError(c, fmt.Errorf("não é possível configurar cobrança sem credenciais AppyPay"))
			return
		}
	}
	var detalhes map[string]interface{}
	if r.informado["detalhes_personalizados"] {
		detalhes = r.DetalhesPersonalizados
	}
	e := s.Atualizar(cond(r, "nome", r.Nome), cond(r, "descricao", r.Descricao), cond(r, "categoria", r.Categoria), cond(r, "pago", r.Pago), cond(r, "preco", r.Preco), cond(r, "tipo_cobranca", r.TipoCobranca), cond(r, "metodos_pagamento", r.MetodosPagamento), cond(r, "tem_taxa_inscricao", r.TemTaxaInscricao), cond(r, "valor_taxa_inscricao", r.ValorTaxaInscricao), cond(r, "metodos_pagamento_taxa_inscricao", r.MetodosPagamentoTaxaInscricao), cond(r, "anos_academicos_disponiveis", r.AnosAcademicosDisponiveis), cond(r, "documento_obrigatorio", r.DocumentoObrigatorio), cond(r, "documento_instrucoes", r.DocumentoInstrucoes), detalhes, id)
	if e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	if e = getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: id.String(), UserType: "academia", IP: c.ClientIP()}); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(200, gin.H{"message": "serviço extra atualizado com sucesso", "data": s})
}
func cond[T any](r servicoExtraPayload, k string, v T) *T {
	if r.informado[k] {
		return &v
	}
	return nil
}
func toggle(c *gin.Context, on bool) {
	s, id, ok := loadServico(c)
	if !ok {
		return
	}
	var e error
	if on {
		e = s.Reativar(id)
	} else {
		e = s.Desativar(id)
	}
	if e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	if e = getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: id.String(), UserType: "academia", IP: c.ClientIP()}); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(200, gin.H{"data": s})
}
func DesativarServicoExtra(c *gin.Context) { toggle(c, false) }
func ReativarServicoExtra(c *gin.Context)  { toggle(c, true) }
func ListarServicosExtrasAcademia(c *gin.Context) {
	codigo, _, ok := academy(c)
	if !ok {
		return
	}
	x, e := getServicosExtrasProjection(c).GetByAcademia(codigo, false)
	if e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(200, gin.H{"servicos_extras": x, "total": len(x)})
}
func GetServicoExtra(c *gin.Context) {
	id, e := uuid.Parse(c.Param("id"))
	if e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	x, e := getServicosExtrasProjection(c).GetByID(id)
	if e != nil || x == nil {
		utils.RespondWithNotFoundError(c, "serviço extra")
		return
	}
	c.JSON(200, gin.H{"data": x})
}
func ListarServicosExtrasPublico(c *gin.Context) {
	x, e := getServicosExtrasProjection(c).GetByAcademia(c.Param("codigo_academia"), true)
	if e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(200, gin.H{"servicos_extras": x, "total": len(x)})
}
