package finance

// Este arquivo contém APENAS a unificação de "cobranças reais"
// (financeiro_cobrancas, via ListCobrancas/ListCobrancasEstudante) e
// "pendências sem cobrança" (MensalidadeMesView, via
// PendenciasSemCobranca/PendenciasSemCobrancaEstudante) numa única lista
// paginada — ver ListarPagamentosUnificado. Nenhuma das quatro funções
// citadas acima muda: esta é uma camada de composição por cima delas, não
// uma reimplementação.
//
// Motivação e desenho completos em docs/Debbugs/ e docs/Lista de Tarefas/
// da tarefa "Unificar cobrancas e pendencias_sem_cobranca numa unica lista
// paginada".

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// mesesComCobrancaRealVinculada devolve o conjunto de (codigo_estudante,
// ano_letivo, mes) que já têm PELO MENOS uma cobrança real vinculada em
// financeiro_mensalidade_cobrancas — mesmo que ela tenha falhado. Mesma
// consulta que a função cobrancasExistentesMensalidade fazia antes da
// tarefa 63 (removida ali) — mas usada aqui para um propósito DIFERENTE:
// deduplicação na composição da lista unificada (ver
// filtrarPendenciasComCobrancaRealVinculada), não para decidir se um mês
// está pago. Essa decisão continua sendo feita exclusivamente por
// Estado != EstadoPendente, dentro de PendenciasSemCobranca/
// PendenciasSemCobrancaEstudante — nenhuma das duas muda, e a tarefa 63
// continua válida: um mês com cobrança falhada continua contando como
// pendente. O que esta função resolve é só evitar que ele apareça DUAS
// vezes na lista final — uma vez como a cobrança real (com seu status
// verdadeiro, ex. "falhada") e outra vez como uma pendência sintética
// redundante para o mesmo mês.
func (s *Service) mesesComCobrancaRealVinculada(ctx context.Context, academia string, estudantes []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(estudantes) == 0 {
		return out, nil
	}
	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT codigo_estudante, ano_letivo, mes FROM financeiro_mensalidade_cobrancas WHERE codigo_academia=$1 AND codigo_estudante = ANY($2)`, academia, pq.Array(estudantes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var estudante, ano string
		var mes int
		if err := rows.Scan(&estudante, &ano, &mes); err != nil {
			return nil, err
		}
		out[estudante+"|"+ano+"|"+strconv.Itoa(mes)] = true
	}
	return out, rows.Err()
}

// FiltrarPendenciasComCobrancaRealVinculada remove de pendencias qualquer
// mês que já tenha uma cobrança real vinculada (ver
// mesesComCobrancaRealVinculada) — para uso exclusivo antes de montar a
// lista unificada em ListarPagamentosUnificado. Agrupa internamente por
// CodigoAcademia (cada MensalidadeMesView já carrega a sua) em vez de
// exigir uma única academia como parâmetro: GET /financeiro/cobrancas
// sempre opera numa única academia, mas GET /financeiro/cobrancas/estudante/:codigo
// pode devolver pendências de mais de uma academia (histórico do
// estudante) — mesesComCobrancaRealVinculada exige uma única academia por
// chamada (mesma exigência de escopoMensalidadeEstudantes), então esta
// função chama uma vez por academia distinta presente em pendencias.
func (s *Service) FiltrarPendenciasComCobrancaRealVinculada(ctx context.Context, pendencias []MensalidadeMesView) ([]MensalidadeMesView, error) {
	if len(pendencias) == 0 {
		return pendencias, nil
	}
	estudantesPorAcademia := map[string][]string{}
	for _, p := range pendencias {
		estudantesPorAcademia[p.CodigoAcademia] = append(estudantesPorAcademia[p.CodigoAcademia], p.CodigoEstudante)
	}
	vinculadas := map[string]bool{}
	for academia, estudantes := range estudantesPorAcademia {
		v, err := s.mesesComCobrancaRealVinculada(ctx, academia, estudantes)
		if err != nil {
			return nil, err
		}
		for chave := range v {
			vinculadas[chave] = true
		}
	}
	out := make([]MensalidadeMesView, 0, len(pendencias))
	for _, p := range pendencias {
		chave := p.CodigoEstudante + "|" + p.AnoLetivo + "|" + strconv.Itoa(p.Mes)
		if !vinculadas[chave] {
			out = append(out, p)
		}
	}
	return out, nil
}

// pendenciaNamespace é um UUID fixo e arbitrário, só para servir de
// namespace de uuid.NewSHA1 — não precisa estar registrado em lugar
// nenhum, só precisa ser constante entre chamadas para que o mesmo
// (academia, estudante, ano_letivo, mes) sempre produza o mesmo id
// sintético em pendenciaParaPagamentoResumo. Gerado uma vez com uuid.New()
// e fixado aqui; não tem nenhum significado além de ser constante.
var pendenciaNamespace = uuid.MustParse("c8ede658-7791-4abf-a329-164fba114d8f")

// PagamentoResumo é CobrancaResumo mais um único campo adicional,
// PendenciaSemCobranca — ver ListarPagamentosUnificado para o porquê da
// unificação. Quando PendenciaSemCobranca é true, o item foi sintetizado a
// partir de uma pendência de mensalidade sem NENHUMA cobrança criada (nem
// tentada) — não existe uma linha real em financeiro_cobrancas por trás
// dele. Quando é false, é uma cobrança real, com todos os campos vindos de
// financeiro_cobrancas exatamente como sempre foi.
//
// Status == "pendente" pode vir de QUALQUER um dos dois casos: uma
// cobrança real cujo status ainda não foi resolvido pelo provedor
// (PendenciaSemCobranca=false — o pagamento foi tentado e a AppyPay
// retornou um estado não-terminal), ou uma pendência sintética
// (PendenciaSemCobranca=true — não tem cobrança gerada). O campo
// PendenciaSemCobranca é o que desambigua os dois — sem ele, os dois casos
// seriam indistinguíveis só pelo status.
type PagamentoResumo struct {
	CobrancaResumo
	PendenciaSemCobranca bool `json:"pendencia_sem_cobranca"`
}

// PagamentoListResult é o resultado paginado de ListarPagamentosUnificado —
// mesmo papel de CobrancaListResult, para a lista já unificada. Total é o
// total geral (pendências + cobranças reais) que casa com os filtros
// aplicados — o mesmo significado que CobrancaListResult.Total sempre
// teve, só que agora somando as duas fontes.
type PagamentoListResult struct {
	Pagamentos []PagamentoResumo `json:"pagamentos"`
	Total      int               `json:"total"`
}

// pendenciaParaPagamentoResumo sintetiza um PagamentoResumo
// (PendenciaSemCobranca=true) a partir de uma MensalidadeMesView. Não
// inventa nenhum dado que não exista de verdade na pendência: campos que
// só fazem sentido para uma cobrança real e que a pendência não tem
// (provider_charge_id, merchant_transaction_id, método de pagamento,
// atualizado_em) ficam vazios/nil — a ausência é honesta, não um valor
// inventado.
//
// ID é determinístico (uuid v5, hash de
// "codigo_academia|codigo_estudante|ano_letivo|mes") em vez de aleatório:
// a mesma pendência sempre produz o mesmo ID entre chamadas sucessivas
// (o frontend usa como key de lista sem precisar reordenar a cada
// requisição), e nunca colide com um ID real de financeiro_cobrancas
// (sempre uuid v4 aleatórios gerados por uuid.New() — versão diferente de
// UUID, RFC 4122, sem sobreposição possível com uuid v5).
func pendenciaParaPagamentoResumo(m MensalidadeMesView) PagamentoResumo {
	chave := m.CodigoAcademia + "|" + m.CodigoEstudante + "|" + m.AnoLetivo + "|" + strconv.Itoa(m.Mes)
	return PagamentoResumo{
		CobrancaResumo: CobrancaResumo{
			ID:              uuid.NewSHA1(pendenciaNamespace, []byte(chave)),
			ContextoTipo:    ContextoAcademia,
			CodigoAcademia:  m.CodigoAcademia,
			Origem:          "mensalidade",
			Status:          EstadoPendente,
			Valor:           m.Valor,
			Moeda:           "AOA",
			Descricao:       fmt.Sprintf("Propinas %s: 1 mensalidade(s) — pendência sem cobrança gerada", m.CodigoAcademia),
			CodigoEstudante: m.CodigoEstudante,
			Mensalidades:    []MensalidadeSelecaoMes{{AnoLetivo: m.AnoLetivo, Mes: m.Mes}},
		},
		PendenciaSemCobranca: true,
	}
}

// ListarPagamentosUnificado combina, numa única lista paginada, as
// cobranças reais (buscarCobrancas) com as pendências de mensalidade sem
// nenhuma cobrança (pendencias, já totalmente resolvidas pelo chamador via
// PendenciasSemCobranca ou PendenciasSemCobrancaEstudante — nenhuma das
// duas muda). pendencias pode ser nil/vazio (ex.: nenhum filtro de escopo
// informado em GET /financeiro/cobrancas) — nesse caso o resultado é
// puramente as cobranças reais, com a paginação original intacta.
//
// IMPORTANTE: o chamador deve passar pendencias já filtrada por
// FiltrarPendenciasComCobrancaRealVinculada antes de chamar esta função —
// ListarPagamentosUnificado não faz essa deduplicação sozinha (não tem
// acesso a banco). Sem esse filtro prévio, um mês com uma cobrança real
// falhada apareceria duas vezes na lista final: uma vez como a cobrança
// real (na fonte buscarCobrancas) e outra como pendência sintética
// redundante (na fonte pendencias) — porque, desde a tarefa 63,
// PendenciasSemCobranca inclui corretamente qualquer mês ainda não pago,
// tentativa falhada ou não.
//
// Pendências vêm sempre primeiro (representam ação pendente — "isto ainda
// precisa de uma cobrança"), cobranças reais depois (histórico do que já
// foi tentado, sucesso ou não). Dentro de cada bloco a ordem já vem
// correta da fonte (pendências: por CodigoEstudante depois
// DataReferencia, ordenado por PendenciasSemCobranca; cobranças: por
// updated_at DESC, ordenado por ListCobrancas/ListCobrancasEstudante) —
// esta função nunca reordena, só concatena.
//
// A paginação NÃO busca as duas listas inteiras para depois cortar em
// memória — isso reintroduziria, para cobranças reais, o mesmo tipo de
// problema de escala já corrigido no N+1 de PendenciasSemCobranca (ver
// docs/Tarefas feitas/62): pendências já são computadas por inteiro de
// qualquer forma (é assim que PendenciasSemCobranca sempre funcionou,
// nada muda nisso aqui — o próprio chamador já as tinha completas antes
// de chamar esta função), mas cobranças reais continuam paginadas dentro
// do banco (LIMIT/OFFSET): buscarCobrancas só é chamada com o tanto de
// linhas que ainda cabe na página atual, com o offset ajustado para
// descontar quantos itens da página já vieram de pendências — nunca busca
// mais linhas de financeiro_cobrancas do que cabem na página.
//
// Quando a página inteira já foi preenchida por pendências
// (limiteCobrancas chega a 0), buscarCobrancas ainda é chamada uma vez com
// limit=0 só para obter o total real de cobranças (a contagem roda antes
// do LIMIT dentro de ListCobrancas/ListCobrancasEstudante, então limit=0
// não busca nenhuma linha extra, só a contagem que já seria calculada de
// qualquer forma).
func ListarPagamentosUnificado(pendencias []MensalidadeMesView, buscarCobrancas func(limit, offset int) (*CobrancaListResult, error), limit, offset int) (*PagamentoListResult, error) {
	totalPendencias := len(pendencias)

	var pendenciasNaPagina []MensalidadeMesView
	var limiteCobrancas, offsetCobrancas int

	if offset < totalPendencias {
		fim := offset + limit
		if fim > totalPendencias {
			fim = totalPendencias
		}
		pendenciasNaPagina = pendencias[offset:fim]
		limiteCobrancas = limit - len(pendenciasNaPagina)
		offsetCobrancas = 0
	} else {
		limiteCobrancas = limit
		offsetCobrancas = offset - totalPendencias
	}

	itens := make([]PagamentoResumo, 0, limit)
	for _, p := range pendenciasNaPagina {
		itens = append(itens, pendenciaParaPagamentoResumo(p))
	}

	buscaLimit := limiteCobrancas
	if buscaLimit <= 0 {
		buscaLimit = 0
	}
	res, err := buscarCobrancas(buscaLimit, offsetCobrancas)
	if err != nil {
		return nil, err
	}
	if limiteCobrancas > 0 {
		for _, c := range res.Cobrancas {
			itens = append(itens, PagamentoResumo{CobrancaResumo: c, PendenciaSemCobranca: false})
		}
	}

	return &PagamentoListResult{
		Pagamentos: itens,
		Total:      totalPendencias + res.Total,
	}, nil
}
