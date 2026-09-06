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
	"spuri/internal/projections"
	"spuri/internal/utils"
	"strings"
)

type servicoExtraPayload struct {
	// Tags json explícitas são obrigatórias aqui: sem elas, encoding/json só
	// casa um campo do JSON de entrada com um campo do struct por
	// correspondência exata OU por strings.EqualFold — e EqualFold NÃO ignora
	// "_", só maiúsculas/minúsculas. "tipo_cobranca" (13 chars) nunca é igual,
	// nem por fold, a "TipoCobranca" (12 chars, sem underscore). Sem as tags
	// abaixo, todo campo com mais de uma palavra (tipo_cobranca,
	// metodos_pagamento, tem_taxa_inscricao, valor_taxa_inscricao,
	// metodos_pagamento_taxa_inscricao, anos_academicos_disponiveis,
	// documento_obrigatorio, documento_instrucoes) ficava sempre no zero-value
	// depois do json.Unmarshal(b, r) abaixo, mesmo com bindServicoExtraPayload
	// validando corretamente que o cliente enviou essas chaves — confirmado
	// isolando exatamente esta lógica num programa Go mínimo. Só "nome",
	// "pago", "preco", "categoria" e "descricao" (uma palavra) funcionavam por
	// coincidência. Na prática isto tornava impossível criar qualquer serviço
	// pago, com taxa de inscrição, com documento obrigatório ou com restrição
	// de anos acadêmicos — bug encontrado na auditoria pós-implementação.
	Nome                          string                 `json:"nome"`
	Descricao                     string                 `json:"descricao"`
	Categoria                     string                 `json:"categoria"`
	Pago                          bool                   `json:"pago"`
	Preco                         float64                `json:"preco"`
	TipoCobranca                  string                 `json:"tipo_cobranca"`
	MetodosPagamento              []string               `json:"metodos_pagamento"`
	TemTaxaInscricao              bool                   `json:"tem_taxa_inscricao"`
	ValorTaxaInscricao            float64                `json:"valor_taxa_inscricao"`
	MetodosPagamentoTaxaInscricao []string               `json:"metodos_pagamento_taxa_inscricao"`
	AnosAcademicosDisponiveis     []string               `json:"anos_academicos_disponiveis"`
	CursosDisponiveis             []string               `json:"cursos_disponiveis"`
	DocumentoObrigatorio          bool                   `json:"documento_obrigatorio"`
	DocumentoInstrucoes           string                 `json:"documento_instrucoes"`
	DetalhesPersonalizados        map[string]interface{} `json:"detalhes_personalizados"`
	informado                     map[string]bool
}

func bindServicoExtraPayload(c *gin.Context, r *servicoExtraPayload) error {
	var raw map[string]json.RawMessage
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(&raw); e != nil {
		return fmt.Errorf("dados invalidos")
	}
	allowed := map[string]bool{"nome": true, "descricao": true, "categoria": true, "pago": true, "preco": true, "tipo_cobranca": true, "metodos_pagamento": true, "tem_taxa_inscricao": true, "valor_taxa_inscricao": true, "metodos_pagamento_taxa_inscricao": true, "anos_academicos_disponiveis": true, "cursos_disponiveis": true, "documento_obrigatorio": true, "documento_instrucoes": true, "detalhes_personalizados": true}
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

// validarPosseCursosDisponiveis confirma que cada curso pertence à academia,
// não foi deletado, possui o tipo esperado e oferece o ano informado.
func validarPosseCursosDisponiveis(c *gin.Context, codigoAcademia string, cursosDisponiveis []string) error {
	cursosProj := getCursosProjection(c)
	cache := map[uuid.UUID]*projections.CursoDTO{}
	for _, item := range cursosDisponiveis {
		partes := strings.SplitN(item, "|", 2)
		if len(partes) != 2 {
			continue
		}
		cursoID, err := uuid.Parse(partes[0])
		if err != nil {
			continue
		}
		curso, ok := cache[cursoID]
		if !ok {
			curso, err = cursosProj.GetByID(cursoID)
			if err != nil {
				return fmt.Errorf("erro ao verificar curso %s: %v", cursoID, err)
			}
			cache[cursoID] = curso
		}
		if curso == nil {
			return fmt.Errorf("curso %s não encontrado", cursoID)
		}
		if curso.CodigoAcademia != codigoAcademia {
			return fmt.Errorf("curso %s não pertence a esta academia", cursoID)
		}
		if curso.Status == "deletado" {
			return fmt.Errorf("curso %s foi removido e não pode ser usado em serviços extras", cursoID)
		}
		ano := partes[1]
		tipoEsperado := "medio"
		if strings.HasSuffix(ano, "_ano_superior") {
			tipoEsperado = "superior"
		}
		if curso.Type != tipoEsperado {
			return fmt.Errorf("ano %q não corresponde ao tipo do curso %s (%s)", ano, cursoID, curso.Type)
		}
		anoValido := false
		for _, a := range curso.AnosAcademicos {
			if a == ano {
				anoValido = true
				break
			}
		}
		if !anoValido {
			return fmt.Errorf("ano %q não faz parte dos anos acadêmicos do curso %s", ano, cursoID)
		}
	}
	return nil
}

// estudanteElegivelServicoExtra cruza as restrições do serviço com o ano e
// curso atuais do estudante. anos_academicos_disponiveis só cobre
// fundamental (sem curso); médio e superior são verificados sempre via
// cursos_disponiveis — não há mais fallback de ano solto para esses dois.
func estudanteElegivelServicoExtra(serv *projections.ServicoExtraDTO, est *projections.EstudanteDTO) bool {
	contains := func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	if est.AnoEscolar != nil && contains(serv.AnosAcademicosDisponiveis, *est.AnoEscolar) {
		return true
	}
	if est.CursoMedioID != nil && est.AnoEscolarMedio != nil && contains(serv.CursosDisponiveis, *est.CursoMedioID+"|"+*est.AnoEscolarMedio) {
		return true
	}
	if est.CursoSuperiorID != nil && est.AnoSuperior != nil && contains(serv.CursosDisponiveis, *est.CursoSuperiorID+"|"+*est.AnoSuperior) {
		return true
	}
	return false
}

// servicoExtraToJSON serializa o aggregate em memória com as MESMAS chaves
// snake_case de ServicoExtraDTO (internal/projections/servico_extra_projection.go),
// para que criar/atualizar/ativar/desativar devolvam exatamente o mesmo
// formato que listar/buscar (que leem a projeção). Sem isto, as respostas de
// mutação serializavam os nomes de campo do Go direto (PascalCase: "Nome",
// "CodigoAcademia", ...) por falta de tags json no aggregate, incoerente com
// o resto da API — bug encontrado na auditoria pós-implementação.
func servicoExtraToJSON(s *aggregates.ServicoExtra) gin.H {
	var preco, valorTaxa *float64
	var tipoCobranca *string
	if s.Pago {
		preco, tipoCobranca = ptr(s.Preco), ptr(s.TipoCobranca)
	}
	if s.TemTaxaInscricao {
		valorTaxa = ptr(s.ValorTaxaInscricao)
	}
	return gin.H{
		"id":                               s.GetID(),
		"codigo_academia":                  s.CodigoAcademia,
		"nome":                             s.Nome,
		"descricao":                        s.Descricao,
		"categoria":                        s.Categoria,
		"pago":                             s.Pago,
		"preco":                            preco,
		"tipo_cobranca":                    tipoCobranca,
		"metodos_pagamento":                s.MetodosPagamento,
		"tem_taxa_inscricao":               s.TemTaxaInscricao,
		"valor_taxa_inscricao":             valorTaxa,
		"metodos_pagamento_taxa_inscricao": s.MetodosPagamentoTaxaInscricao,
		"anos_academicos_disponiveis":      s.AnosAcademicosDisponiveis,
		"cursos_disponiveis":               s.CursosDisponiveis,
		"documento_obrigatorio":            s.DocumentoObrigatorio,
		"documento_instrucoes":             s.DocumentoInstrucoes,
		"detalhes_personalizados":          s.DetalhesPersonalizados,
		"ativo":                            s.Ativo,
		"created_at":                       s.CreatedAt,
		"updated_at":                       s.UpdatedAt,
	}
}
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
	if e := validarPosseCursosDisponiveis(c, codigo, r.CursosDisponiveis); e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	s := aggregates.NewServicoExtra()
	if e := s.Criar(codigo, r.Nome, r.Descricao, r.Categoria, r.Pago, r.Preco, r.TipoCobranca, r.MetodosPagamento, r.TemTaxaInscricao, r.ValorTaxaInscricao, r.MetodosPagamentoTaxaInscricao, r.AnosAcademicosDisponiveis, r.CursosDisponiveis, r.DocumentoObrigatorio, r.DocumentoInstrucoes, r.DetalhesPersonalizados, id); e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	if e := getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: id.String(), UserType: "academia", IP: c.ClientIP()}); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "serviço extra criado com sucesso", "data": servicoExtraToJSON(s)})
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
	if r.informado["cursos_disponiveis"] {
		if e := validarPosseCursosDisponiveis(c, s.CodigoAcademia, r.CursosDisponiveis); e != nil {
			utils.RespondWithValidationError(c, e)
			return
		}
	}
	var detalhes map[string]interface{}
	if r.informado["detalhes_personalizados"] {
		detalhes = r.DetalhesPersonalizados
	}
	e := s.Atualizar(cond(r, "nome", r.Nome), cond(r, "descricao", r.Descricao), cond(r, "categoria", r.Categoria), cond(r, "pago", r.Pago), cond(r, "preco", r.Preco), cond(r, "tipo_cobranca", r.TipoCobranca), cond(r, "metodos_pagamento", r.MetodosPagamento), cond(r, "tem_taxa_inscricao", r.TemTaxaInscricao), cond(r, "valor_taxa_inscricao", r.ValorTaxaInscricao), cond(r, "metodos_pagamento_taxa_inscricao", r.MetodosPagamentoTaxaInscricao), cond(r, "anos_academicos_disponiveis", r.AnosAcademicosDisponiveis), cond(r, "cursos_disponiveis", r.CursosDisponiveis), cond(r, "documento_obrigatorio", r.DocumentoObrigatorio), cond(r, "documento_instrucoes", r.DocumentoInstrucoes), detalhes, id)
	if e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	if e = getRepository(c).SaveWithAudit(s, db.AuditContext{UserID: id.String(), UserType: "academia", IP: c.ClientIP()}); e != nil {
		utils.RespondWithInternalError(c, e)
		return
	}
	c.JSON(200, gin.H{"message": "serviço extra atualizado com sucesso", "data": servicoExtraToJSON(s)})
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
	c.JSON(200, gin.H{"data": servicoExtraToJSON(s)})
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
	if role, _ := middleware.GetUserType(c); role != "admin" {
		codigo, _, ok := academy(c)
		if !ok {
			return
		}
		if x.CodigoAcademia != codigo {
			utils.RespondWithForbiddenError(c, "serviço extra não pertence a esta academia")
			return
		}
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
