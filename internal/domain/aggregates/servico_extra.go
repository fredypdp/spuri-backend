package aggregates

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"spuri/internal/utils"
)

// ServicoExtra representa um serviço adicional oferecido por uma academia,
// fora do currículo regular: transporte, atividades extracurriculares
// (dança, artes marciais, natação, etc.) ou qualquer outro serviço que a
// própria academia queira configurar.
//
// Duas dimensões financeiras independentes e ortogonais — NÃO simplificar:
//   - Pago/Preco/TipoCobranca descrevem o preço do próprio serviço
//     (recorrente mensal ou pagamento único), cobrado enquanto o estudante
//     estiver vinculado.
//   - TemTaxaInscricao/ValorTaxaInscricao descrevem uma taxa de admissão
//     cobrada uma única vez, no momento em que o estudante é vinculado ao
//     serviço — em analogia direta ao par mensalidade/matrícula já
//     existente no módulo financeiro. Um serviço gratuito (Pago=false)
//     pode ainda assim ter taxa de inscrição.
type ServicoExtra struct {
	BaseAggregate

	CodigoAcademia string
	Nome           string
	Descricao      string
	Categoria      string

	Pago             bool
	Preco            float64
	TipoCobranca     string // "unico" | "mensal" — vazio quando Pago=false
	MetodosPagamento []string

	TemTaxaInscricao              bool
	ValorTaxaInscricao            float64
	MetodosPagamentoTaxaInscricao []string

	AnosAcademicosDisponiveis []string
	CursosDisponiveis         []string

	DocumentoObrigatorio bool
	DocumentoInstrucoes  string

	DetalhesPersonalizados map[string]interface{}

	Ativo     bool
	CriadoPor uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

const (
	TipoCobrancaServicoUnico  = "unico"
	TipoCobrancaServicoMensal = "mensal"
)

var metodosPagamentoServicoExtraValidos = map[string]bool{
	"GPO":    true,
	"REF":    true,
	"GPO_QR": true,
}

func NewServicoExtra() *ServicoExtra {
	return &ServicoExtra{
		BaseAggregate: BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		},
		MetodosPagamento:              []string{},
		MetodosPagamentoTaxaInscricao: []string{},
		AnosAcademicosDisponiveis:     []string{},
		CursosDisponiveis:             []string{},
		DetalhesPersonalizados:        map[string]interface{}{},
		Ativo:                         true,
	}
}

func (s *ServicoExtra) GetType() string { return "ServicoExtra" }

// ============================================================================
// Eventos
// ============================================================================

type ServicoExtraCriadoEvent struct {
	BaseEvent
	CodigoAcademia                string
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
	CursosDisponiveis             []string
	DocumentoObrigatorio          bool
	DocumentoInstrucoes           string
	DetalhesPersonalizados        map[string]interface{}
	CriadoPor                     uuid.UUID
	CreatedAt                     time.Time
}

func (e *ServicoExtraCriadoEvent) GetPayload() interface{} { return e }
func (e *ServicoExtraCriadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ServicoExtraAtualizadoEvent usa ponteiros: nil = campo não alterado nesta
// atualização. Isto permite ao handler enviar só os campos que o cliente
// realmente informou no PATCH/PUT, sem sobrescrever os demais com zero-value.
type ServicoExtraAtualizadoEvent struct {
	BaseEvent
	Nome                          *string
	Descricao                     *string
	Categoria                     *string
	Pago                          *bool
	Preco                         *float64
	TipoCobranca                  *string
	MetodosPagamento              *[]string
	TemTaxaInscricao              *bool
	ValorTaxaInscricao            *float64
	MetodosPagamentoTaxaInscricao *[]string
	AnosAcademicosDisponiveis     *[]string
	CursosDisponiveis             *[]string
	DocumentoObrigatorio          *bool
	DocumentoInstrucoes           *string
	DetalhesPersonalizados        map[string]interface{} // nil = não alterar; não-nil substitui o mapa inteiro
	AtualizadoPor                 uuid.UUID
	UpdatedAt                     time.Time
}

func (e *ServicoExtraAtualizadoEvent) GetPayload() interface{} { return e }
func (e *ServicoExtraAtualizadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type ServicoExtraDesativadoEvent struct {
	BaseEvent
	DesativadoPor uuid.UUID
	UpdatedAt     time.Time
}

func (e *ServicoExtraDesativadoEvent) GetPayload() interface{} { return e }
func (e *ServicoExtraDesativadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

type ServicoExtraReativadoEvent struct {
	BaseEvent
	ReativadoPor uuid.UUID
	UpdatedAt    time.Time
}

func (e *ServicoExtraReativadoEvent) GetPayload() interface{} { return e }
func (e *ServicoExtraReativadoEvent) ToJSON() ([]byte, error) { return json.Marshal(e) }

// ============================================================================
// Apply dispatcher
// ============================================================================

func (s *ServicoExtra) Apply(event DomainEvent) error {
	switch event.GetEventType() {
	case "ServicoExtraCriado":
		return s.applyCriado(event)
	case "ServicoExtraAtualizado":
		return s.applyAtualizado(event)
	case "ServicoExtraDesativado":
		s.Ativo = false
		return nil
	case "ServicoExtraReativado":
		s.Ativo = true
		return nil
	default:
		return fmt.Errorf("tipo de evento desconhecido para ServicoExtra: %s", event.GetEventType())
	}
}

// ============================================================================
// Commands
// ============================================================================

// Criar valida e registra a criação de um serviço extra. A checagem de
// credenciais AppyPay (necessária quando pago=true OU temTaxaInscricao=true)
// é feita pelo HANDLER antes de chamar este método — o aggregate não importa
// internal/finance (evitaria ciclo de import) e só valida consistência
// interna dos campos.
func (s *ServicoExtra) Criar(
	codigoAcademia, nome, descricao, categoria string,
	pago bool, preco float64, tipoCobranca string, metodosPagamento []string,
	temTaxaInscricao bool, valorTaxaInscricao float64, metodosPagamentoTaxaInscricao []string,
	anosAcademicosDisponiveis []string,
	cursosDisponiveis []string,
	documentoObrigatorio bool, documentoInstrucoes string,
	detalhesPersonalizados map[string]interface{},
	criadoPor uuid.UUID,
) error {
	if strings.TrimSpace(codigoAcademia) == "" {
		return fmt.Errorf("codigo_academia é obrigatório")
	}
	if strings.TrimSpace(nome) == "" {
		return fmt.Errorf("nome é obrigatório")
	}
	metodosPagamento, err := normalizarMetodosPagamentoServicoExtra(metodosPagamento)
	if err != nil {
		return err
	}
	metodosPagamentoTaxaInscricao, err = normalizarMetodosPagamentoServicoExtra(metodosPagamentoTaxaInscricao)
	if err != nil {
		return err
	}
	if err := validarCamposPagamentoServico(pago, preco, tipoCobranca, metodosPagamento); err != nil {
		return err
	}
	if err := validarCamposTaxaInscricao(temTaxaInscricao, valorTaxaInscricao, metodosPagamentoTaxaInscricao); err != nil {
		return err
	}
	if err := validarAnosAcademicosServicoExtra(anosAcademicosDisponiveis); err != nil {
		return err
	}
	if err := validarCursosDisponiveisServicoExtra(cursosDisponiveis); err != nil {
		return err
	}
	if !pago {
		preco = 0
		tipoCobranca = ""
	}
	if !temTaxaInscricao {
		valorTaxaInscricao = 0
	}
	if detalhesPersonalizados == nil {
		detalhesPersonalizados = map[string]interface{}{}
	}

	event := &ServicoExtraCriadoEvent{
		BaseEvent:                     BaseEvent{EventType: "ServicoExtraCriado", AggregateID: s.ID},
		CodigoAcademia:                codigoAcademia,
		Nome:                          strings.TrimSpace(nome),
		Descricao:                     strings.TrimSpace(descricao),
		Categoria:                     strings.TrimSpace(categoria),
		Pago:                          pago,
		Preco:                         preco,
		TipoCobranca:                  tipoCobranca,
		MetodosPagamento:              metodosPagamento,
		TemTaxaInscricao:              temTaxaInscricao,
		ValorTaxaInscricao:            valorTaxaInscricao,
		MetodosPagamentoTaxaInscricao: metodosPagamentoTaxaInscricao,
		AnosAcademicosDisponiveis:     anosAcademicosDisponiveis,
		CursosDisponiveis:             cursosDisponiveis,
		DocumentoObrigatorio:          documentoObrigatorio,
		DocumentoInstrucoes:           strings.TrimSpace(documentoInstrucoes),
		DetalhesPersonalizados:        detalhesPersonalizados,
		CriadoPor:                     criadoPor,
		CreatedAt:                     time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// Atualizar aplica alterações parciais. Ponteiros nil = campo não enviado
// nesta chamada = mantém valor atual. Sempre que qualquer um dos campos
// financeiros (pago, preco, tipo_cobranca, metodos_pagamento,
// tem_taxa_inscricao, valor_taxa_inscricao, metodos_pagamento_taxa_inscricao)
// é alterado, o RESULTADO FINAL (aplicando a alteração sobre o estado atual)
// deve continuar consistente — por isso a validação abaixo sempre calcula os
// valores efetivos (atual + alteração) antes de validar, nunca valida só o
// campo isolado que veio no payload.
func (s *ServicoExtra) Atualizar(
	nome, descricao, categoria *string,
	pago *bool, preco *float64, tipoCobranca *string, metodosPagamento *[]string,
	temTaxaInscricao *bool, valorTaxaInscricao *float64, metodosPagamentoTaxaInscricao *[]string,
	anosAcademicosDisponiveis *[]string,
	cursosDisponiveis *[]string,
	documentoObrigatorio *bool, documentoInstrucoes *string,
	detalhesPersonalizados map[string]interface{},
	atualizadoPor uuid.UUID,
) error {
	if nome != nil && strings.TrimSpace(*nome) == "" {
		return fmt.Errorf("nome não pode ser vazio")
	}

	efetivoPago := s.Pago
	if pago != nil {
		efetivoPago = *pago
	}
	efetivoPreco := s.Preco
	if preco != nil {
		efetivoPreco = *preco
	}
	efetivoTipoCobranca := s.TipoCobranca
	if tipoCobranca != nil {
		efetivoTipoCobranca = *tipoCobranca
	}
	efetivoMetodos := s.MetodosPagamento
	if metodosPagamento != nil {
		normalizados, err := normalizarMetodosPagamentoServicoExtra(*metodosPagamento)
		if err != nil {
			return err
		}
		efetivoMetodos = normalizados
		metodosPagamento = &normalizados
	}
	if err := validarCamposPagamentoServico(efetivoPago, efetivoPreco, efetivoTipoCobranca, efetivoMetodos); err != nil {
		return err
	}

	efetivoTemTaxa := s.TemTaxaInscricao
	if temTaxaInscricao != nil {
		efetivoTemTaxa = *temTaxaInscricao
	}
	efetivoValorTaxa := s.ValorTaxaInscricao
	if valorTaxaInscricao != nil {
		efetivoValorTaxa = *valorTaxaInscricao
	}
	efetivoMetodosTaxa := s.MetodosPagamentoTaxaInscricao
	if metodosPagamentoTaxaInscricao != nil {
		normalizados, err := normalizarMetodosPagamentoServicoExtra(*metodosPagamentoTaxaInscricao)
		if err != nil {
			return err
		}
		efetivoMetodosTaxa = normalizados
		metodosPagamentoTaxaInscricao = &normalizados
	}
	if err := validarCamposTaxaInscricao(efetivoTemTaxa, efetivoValorTaxa, efetivoMetodosTaxa); err != nil {
		return err
	}

	if anosAcademicosDisponiveis != nil {
		if err := validarAnosAcademicosServicoExtra(*anosAcademicosDisponiveis); err != nil {
			return err
		}
	}
	if cursosDisponiveis != nil {
		if err := validarCursosDisponiveisServicoExtra(*cursosDisponiveis); err != nil {
			return err
		}
	}

	// Zera campos que deixaram de se aplicar, para o resultado final nunca
	// violar o CHECK constraint da tabela (ex.: desligar `pago` sem zerar
	// preco/tipo_cobranca explicitamente).
	if pago != nil && !*pago {
		zero := 0.0
		empty := ""
		preco = &zero
		tipoCobranca = &empty
		vazios := []string{}
		metodosPagamento = &vazios
	}
	if temTaxaInscricao != nil && !*temTaxaInscricao {
		zero := 0.0
		valorTaxaInscricao = &zero
		vazios := []string{}
		metodosPagamentoTaxaInscricao = &vazios
	}

	event := &ServicoExtraAtualizadoEvent{
		BaseEvent:                     BaseEvent{EventType: "ServicoExtraAtualizado", AggregateID: s.ID},
		Nome:                          nome,
		Descricao:                     descricao,
		Categoria:                     categoria,
		Pago:                          pago,
		Preco:                         preco,
		TipoCobranca:                  tipoCobranca,
		MetodosPagamento:              metodosPagamento,
		TemTaxaInscricao:              temTaxaInscricao,
		ValorTaxaInscricao:            valorTaxaInscricao,
		MetodosPagamentoTaxaInscricao: metodosPagamentoTaxaInscricao,
		AnosAcademicosDisponiveis:     anosAcademicosDisponiveis,
		CursosDisponiveis:             cursosDisponiveis,
		DocumentoObrigatorio:          documentoObrigatorio,
		DocumentoInstrucoes:           documentoInstrucoes,
		DetalhesPersonalizados:        detalhesPersonalizados,
		AtualizadoPor:                 atualizadoPor,
		UpdatedAt:                     time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

func (s *ServicoExtra) Desativar(desativadoPor uuid.UUID) error {
	if !s.Ativo {
		return fmt.Errorf("serviço já está inativo")
	}
	event := &ServicoExtraDesativadoEvent{
		BaseEvent:     BaseEvent{EventType: "ServicoExtraDesativado", AggregateID: s.ID},
		DesativadoPor: desativadoPor,
		UpdatedAt:     time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

func (s *ServicoExtra) Reativar(reativadoPor uuid.UUID) error {
	if s.Ativo {
		return fmt.Errorf("serviço já está ativo")
	}
	event := &ServicoExtraReativadoEvent{
		BaseEvent:    BaseEvent{EventType: "ServicoExtraReativado", AggregateID: s.ID},
		ReativadoPor: reativadoPor,
		UpdatedAt:    time.Now(),
	}
	s.RaiseEvent(event)
	return s.Apply(event)
}

// ============================================================================
// Apply handlers
// ============================================================================

func (s *ServicoExtra) applyCriado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p ServicoExtraCriadoEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	s.CodigoAcademia = p.CodigoAcademia
	s.Nome = p.Nome
	s.Descricao = p.Descricao
	s.Categoria = p.Categoria
	s.Pago = p.Pago
	s.Preco = p.Preco
	s.TipoCobranca = p.TipoCobranca
	s.MetodosPagamento = p.MetodosPagamento
	s.TemTaxaInscricao = p.TemTaxaInscricao
	s.ValorTaxaInscricao = p.ValorTaxaInscricao
	s.MetodosPagamentoTaxaInscricao = p.MetodosPagamentoTaxaInscricao
	s.AnosAcademicosDisponiveis = p.AnosAcademicosDisponiveis
	s.CursosDisponiveis = p.CursosDisponiveis
	s.DocumentoObrigatorio = p.DocumentoObrigatorio
	s.DocumentoInstrucoes = p.DocumentoInstrucoes
	s.DetalhesPersonalizados = p.DetalhesPersonalizados
	s.Ativo = true
	s.CriadoPor = p.CriadoPor
	s.CreatedAt = p.CreatedAt
	s.UpdatedAt = p.CreatedAt
	return nil
}

func (s *ServicoExtra) applyAtualizado(event DomainEvent) error {
	data, err := json.Marshal(event.GetPayload())
	if err != nil {
		return err
	}
	var p ServicoExtraAtualizadoEvent
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	if p.Nome != nil {
		s.Nome = *p.Nome
	}
	if p.Descricao != nil {
		s.Descricao = *p.Descricao
	}
	if p.Categoria != nil {
		s.Categoria = *p.Categoria
	}
	if p.Pago != nil {
		s.Pago = *p.Pago
	}
	if p.Preco != nil {
		s.Preco = *p.Preco
	}
	if p.TipoCobranca != nil {
		s.TipoCobranca = *p.TipoCobranca
	}
	if p.MetodosPagamento != nil {
		s.MetodosPagamento = *p.MetodosPagamento
	}
	if p.TemTaxaInscricao != nil {
		s.TemTaxaInscricao = *p.TemTaxaInscricao
	}
	if p.ValorTaxaInscricao != nil {
		s.ValorTaxaInscricao = *p.ValorTaxaInscricao
	}
	if p.MetodosPagamentoTaxaInscricao != nil {
		s.MetodosPagamentoTaxaInscricao = *p.MetodosPagamentoTaxaInscricao
	}
	if p.AnosAcademicosDisponiveis != nil {
		s.AnosAcademicosDisponiveis = *p.AnosAcademicosDisponiveis
	}
	if p.CursosDisponiveis != nil {
		s.CursosDisponiveis = *p.CursosDisponiveis
	}
	if p.DocumentoObrigatorio != nil {
		s.DocumentoObrigatorio = *p.DocumentoObrigatorio
	}
	if p.DocumentoInstrucoes != nil {
		s.DocumentoInstrucoes = *p.DocumentoInstrucoes
	}
	if p.DetalhesPersonalizados != nil {
		s.DetalhesPersonalizados = p.DetalhesPersonalizados
	}
	s.UpdatedAt = p.UpdatedAt
	return nil
}

// ============================================================================
// Validação — espelha exatamente os CHECK constraints da migration 118
// ============================================================================

func normalizarMetodosPagamentoServicoExtra(metodos []string) ([]string, error) {
	if len(metodos) == 0 {
		return []string{}, nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(metodos))
	for _, m := range metodos {
		m = strings.ToUpper(strings.TrimSpace(m))
		if !metodosPagamentoServicoExtraValidos[m] {
			return nil, fmt.Errorf("metodos_pagamento aceita apenas GPO, REF ou GPO_QR")
		}
		if seen[m] {
			return nil, fmt.Errorf("metodos_pagamento não pode conter duplicados")
		}
		seen[m] = true
		out = append(out, m)
	}
	return out, nil
}

func validarCamposPagamentoServico(pago bool, preco float64, tipoCobranca string, metodos []string) error {
	if !pago {
		return nil
	}
	if preco <= 0 {
		return fmt.Errorf("preco deve ser maior que zero quando o serviço é pago")
	}
	if tipoCobranca != TipoCobrancaServicoUnico && tipoCobranca != TipoCobrancaServicoMensal {
		return fmt.Errorf("tipo_cobranca deve ser 'unico' ou 'mensal' quando o serviço é pago")
	}
	if len(metodos) == 0 {
		return fmt.Errorf("metodos_pagamento é obrigatório quando o serviço é pago")
	}
	return nil
}

func validarCamposTaxaInscricao(temTaxaInscricao bool, valor float64, metodos []string) error {
	if !temTaxaInscricao {
		return nil
	}
	if valor <= 0 {
		return fmt.Errorf("valor_taxa_inscricao deve ser maior que zero quando tem_taxa_inscricao é verdadeiro")
	}
	if len(metodos) == 0 {
		return fmt.Errorf("metodos_pagamento_taxa_inscricao é obrigatório quando tem_taxa_inscricao é verdadeiro")
	}
	return nil
}

// validarAnosAcademicosServicoExtra valida apenas o FORMATO de cada ano
// informado, despachando para o validador correto pelo sufixo. Lista vazia
// é válida e significa "disponível para todos os anos" — não validar como
// erro. Deliberadamente NÃO cruza com cursos/turmas reais da academia (ver
// decisão de design 7 no documento da tarefa).
func validarAnosAcademicosServicoExtra(anos []string) error {
	for _, ano := range anos {
		switch {
		case strings.HasSuffix(ano, "_ano_fundamental"):
			if err := utils.ValidateAnoFundamental(ano); err != nil {
				return err
			}
		case strings.HasSuffix(ano, "_ano_medio"):
			if err := utils.ValidateAnoMedio(ano); err != nil {
				return err
			}
		case strings.HasSuffix(ano, "_ano_superior"):
			if err := utils.ValidateAnoSuperior(ano); err != nil {
				return err
			}
		default:
			return fmt.Errorf("formato de ano acadêmico inválido: %q", ano)
		}
	}
	return nil
}

// validarCursosDisponiveisServicoExtra valida somente o formato
// "<curso_id>|<ano_academico>". Cursos fundamentais não existem no sistema.
func validarCursosDisponiveisServicoExtra(cursos []string) error {
	for _, item := range cursos {
		partes := strings.SplitN(item, "|", 2)
		if len(partes) != 2 || partes[0] == "" || partes[1] == "" {
			return fmt.Errorf("formato de curso_disponivel inválido: %q — esperado \"<curso_id>|<ano_academico>\"", item)
		}
		if _, err := uuid.Parse(partes[0]); err != nil {
			return fmt.Errorf("curso_id inválido em %q: %v", item, err)
		}
		ano := partes[1]
		switch {
		case strings.HasSuffix(ano, "_ano_medio"):
			if err := utils.ValidateAnoMedio(ano); err != nil {
				return err
			}
		case strings.HasSuffix(ano, "_ano_superior"):
			if err := utils.ValidateAnoSuperior(ano); err != nil {
				return err
			}
		default:
			return fmt.Errorf("ano_academico inválido em %q: deve terminar em _ano_medio ou _ano_superior (fundamental não usa curso)", item)
		}
	}
	return nil
}
