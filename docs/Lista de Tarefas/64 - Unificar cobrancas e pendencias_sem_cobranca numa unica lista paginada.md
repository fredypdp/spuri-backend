---
criado: 2026-08-24
origem: docs/Debbugs/Desenhar unificacao de cobrancas e pendencias_sem_cobranca numa unica lista paginada.md
status: pendente
tipo: nova_funcionalidade_refactor_de_contrato
depende_de: docs/Tarefas feitas/63 - Listar meses com cobranca falhada em pendencias_sem_cobranca.md
gera_dependencia_para: tarefa 65 (repositório spuripainel — frontend, aplicar DEPOIS desta)
---

# Unificar `cobrancas` + `pendencias_sem_cobranca` numa única lista paginada em `GET /financeiro/cobrancas` e `GET /financeiro/cobrancas/estudante/:codigo`

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Mesma situação das tarefas 62/63: você não tem `apt`, Docker nem `psql`. Não precisa disso aqui. Claude já validou esta correção inteira com PostgreSQL 16, Go 1.24 (e, para a tarefa 65, dependente desta, Node 22 e Next.js) reais, incluindo aplicar todos os arquivos sobre um clone **novo e limpo** de `main` e rodar a suíte de testes inteira do zero — 100% verde em todos os pacotes do repositório. Sua validação usa só `go build`, `go vet`, `gofmt` e `go test ./...` (os testes de integração pulam automaticamente sem `RUN_POSTGRES_INTEGRATION`, isso é esperado).

**Uma variável de ambiente importante para você NÃO precisar mexer:** os testes de integração de credenciais AppyPay (`FINANCE_ENCRYPTION_KEY`) não são afetados por esta tarefa — se você não tiver essa variável configurada, esses testes específicos (não relacionados a esta tarefa) vão continuar pulando ou falhando exatamente como já fariam antes desta tarefa. Não é algo para investigar ou corrigir aqui.

**Esta tarefa pressupõe que as tarefas 62 e 63 já foram aplicadas** (ambas já estão em `docs/Tarefas feitas/` no momento em que este documento foi escrito). Todos os arquivos abaixo são conteúdo **completo e final** — se por algum motivo 62/63 ainda não tiverem sido aplicadas, aplicar esta tarefa por si só entrega tudo junto (não precisa aplicá-las antes).

---

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento, nesta ordem. Todas as decisões já foram tomadas e validadas (desenho completo, implementação testada com PostgreSQL 16 e Go 1.24 reais, incluindo um clone novo e independente do repositório rodando a suíte de testes inteira do zero). Sua tarefa é mecânica: (1) aplicar a substituição cirúrgica em `internal/finance/appypay.go` descrita na seção 3; (2) criar os 3 arquivos novos em `internal/finance/` descritos nas seções 4, 5 e 6; (3) substituir o conteúdo inteiro de `internal/handlers/financeiro_handlers.go` pelo conteúdo da seção 7; (4) substituir o conteúdo inteiro dos 2 arquivos de teste de handler descritos nas seções 8 e 9; (5) atualizar a `Documentação da API.md` conforme a seção 10; (6) rodar cada item da seção "Checklist de validação" e reportar o resultado; (7) seguir o "Procedimento de conclusão". Não toque em nenhum arquivo ou lógica fora do escopo listado na seção "Fora de escopo". Não é necessário PostgreSQL, Docker nem `psql`.

---

## 2. Contexto

`GET /financeiro/cobrancas` devolvia duas listas separadas: `cobrancas` (paginada no banco) e `pendencias_sem_cobranca` (sempre completa, sem paginação própria) — forçando o cliente a lidar com duas fontes de paginação incompatíveis para montar uma única visão. Fredy pediu a unificação: uma lista só, `pagamentos`, em que cada item usa a mesma estrutura de uma cobrança, acrescida de um campo booleano `pendencia_sem_cobranca` que desambigua, quando `status` é `"pendente"`, se é porque a cobrança foi de fato tentada (e a AppyPay devolveu um estado ainda não resolvido) ou porque não existe nenhuma cobrança gerada para aquele mês.

**Problema de design resolvido antes de implementar:** a tarefa 63 fez `PendenciasSemCobranca` incluir corretamente qualquer mês ainda não pago, mesmo com uma tentativa FALHADA. Unificar as listas sem tratar isso faria o mesmo mês aparecer duas vezes (a cobrança real falhada + uma pendência sintética redundante). Resolvido com `FiltrarPendenciasComCobrancaRealVinculada`, que remove da lista de pendências qualquer mês que já tenha uma cobrança real vinculada — ver a análise completa em `docs/Debbugs/Desenhar unificacao de cobrancas e pendencias_sem_cobranca numa unica lista paginada.md`.

**Resumo da implementação:**
- `PagamentoResumo` (`internal/finance/pagamentos_unificado.go`, novo arquivo) = `CobrancaResumo` (existente, inalterada em sua estrutura de campos, exceto `AtualizadoEm`) + `PendenciaSemCobranca bool`.
- `ListarPagamentosUnificado` combina pendências (já resolvidas por inteiro pelo chamador) com cobranças reais (paginadas no banco), aplicando a paginação como uma lista única sem nunca buscar mais linhas de `financeiro_cobrancas` do que cabem na página atual.
- `FiltrarPendenciasComCobrancaRealVinculada` remove a redundância descrita acima.
- `CobrancaResumo.AtualizadoEm` muda de `time.Time` para `*time.Time`: um item sintético não tem nenhuma atividade real para reportar, e `nil` (omitido do JSON) é a representação honesta disso — mudança pontual, único campo do contrato existente que muda de tipo.
- Os dois handlers (`ListarCobrancasAppyPay`, `ConsultarCobrancasEstudante`) passam a devolver `{"pagamentos": [...], "total": N, "total_geral": M, "limit": L, "offset": O}` em vez de `{"cobrancas": [...], "pendencias_sem_cobranca": [...], ...}`.

Nenhuma das funções já existentes muda de comportamento: `PendenciasSemCobranca`, `PendenciasSemCobrancaEstudante`, `ListCobrancas`, `ListCobrancasEstudante`, `escopoMensalidadeEstudantes`, `estadosObrigacaoBatch` — todas continuam exatamente como estavam.

---

## 3. `internal/finance/appypay.go` — substituição cirúrgica (não é o arquivo inteiro)

Este arquivo tem 1531 linhas; só a struct `CobrancaResumo` muda (um campo de tipo, com um comentário novo explicando por quê). Localize o bloco EXATO abaixo (é único no arquivo) e substitua apenas ele — não altere mais nada no arquivo:

**Bloco a localizar (exato, incluindo indentação com tabs):**

```go
	Mensalidades      []MensalidadeSelecaoMes `json:"mensalidades,omitempty"`
	AtualizadoEm      time.Time               `json:"atualizado_em"`
}
```

**Substituir por (texto literal, exato):**

```go
	Mensalidades      []MensalidadeSelecaoMes `json:"mensalidades,omitempty"`
	// AtualizadoEm é ponteiro (não time.Time) desde a unificação de
	// pendências_sem_cobranca em ListarPagamentosUnificado
	// (pagamentos_unificado.go): um item sintetizado a partir de uma
	// pendência sem cobrança (PendenciaSemCobranca=true) nunca teve
	// nenhuma atividade real, então não há nenhum "atualizado em" honesto
	// para devolver — nil (omitido do JSON) em vez de inventar uma data.
	// Para uma cobrança real, continua sempre presente (a coluna é
	// NOT NULL em financeiro_cobrancas).
	AtualizadoEm *time.Time `json:"atualizado_em,omitempty"`
}
```

**Confirme antes de prosseguir:** `grep -n "AtualizadoEm" internal/finance/appypay.go` deve mostrar exatamente 2 ocorrências: a declaração do campo (`AtualizadoEm *time.Time`) e o uso já existente em `scanCobrancaResumo` (`&dto.AtualizadoEm`, que NÃO muda — o scan já funciona corretamente com o novo tipo ponteiro, confirmado empiricamente por Claude: `database/sql`/`lib/pq` escaneiam `*time.Time` a partir de uma coluna `NOT NULL` sem nenhuma mudança de código adicional).

---

## 4. `internal/finance/pagamentos_unificado.go` — criar arquivo novo

```go
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
```

---

## 5. `internal/finance/pagamentos_unificado_test.go` — criar arquivo novo

Testes unitários puros (não usam Postgres, não são `_integration_test.go`) da matemática de paginação — rodam com `go test ./internal/finance/...` normalmente, sem nenhuma variável de ambiente:

```go
package finance

import (
	"testing"
)

// cobrancaListStub simula buscarCobrancas: um "banco" fixo de N cobranças
// (identificadas só pelo índice, via MerchantTransactionID), com
// LIMIT/OFFSET aplicados exatamente como uma consulta SQL real aplicaria —
// para testar a matemática de paginação de ListarPagamentosUnificado sem
// precisar de PostgreSQL. Também conta quantas vezes foi chamada, para
// confirmar que ListarPagamentosUnificado nunca busca mais do que o
// necessário.
func cobrancaListStub(totalReal int, chamadas *int) func(limit, offset int) (*CobrancaListResult, error) {
	return func(limit, offset int) (*CobrancaListResult, error) {
		*chamadas++
		out := []CobrancaResumo{}
		for i := offset; i < offset+limit && i < totalReal; i++ {
			out = append(out, CobrancaResumo{MerchantTransactionID: idxToMTID(i)})
		}
		return &CobrancaListResult{Cobrancas: out, Total: totalReal}, nil
	}
}

func idxToMTID(i int) string {
	digits := "0123456789"
	if i < 10 {
		return "COB-" + string(digits[i])
	}
	return "COB-" + string(digits[i/10]) + string(digits[i%10])
}

func pendenciasFake(n int) []MensalidadeMesView {
	out := make([]MensalidadeMesView, n)
	for i := range out {
		out[i] = MensalidadeMesView{CodigoEstudante: idxToMTID(i), CodigoAcademia: "ACA1", AnoLetivo: "2026_2027", Mes: (i % 12) + 1}
	}
	return out
}

// TestListarPagamentosUnificadoSemPendencias cobre o caso mais comum hoje
// (nenhum filtro de escopo em GET /financeiro/cobrancas): sem pendências,
// o resultado deve ser um passthrough exato das cobranças reais, com o
// limit/offset originais intactos — nenhuma mudança de comportamento em
// relação a antes desta unificação.
func TestListarPagamentosUnificadoSemPendencias(t *testing.T) {
	var chamadas int
	res, err := ListarPagamentosUnificado(nil, cobrancaListStub(120, &chamadas), 30, 60)
	if err != nil {
		t.Fatal(err)
	}
	if chamadas != 1 {
		t.Fatalf("esperava exatamente 1 chamada a buscarCobrancas, obteve %d", chamadas)
	}
	if len(res.Pagamentos) != 30 {
		t.Fatalf("esperava 30 itens, obteve %d", len(res.Pagamentos))
	}
	if res.Pagamentos[0].MerchantTransactionID != idxToMTID(60) {
		t.Fatalf("esperava a página começar em COB idx 60, começou em %s", res.Pagamentos[0].MerchantTransactionID)
	}
	if res.Total != 120 {
		t.Fatalf("esperava total=120, obteve %d", res.Total)
	}
	for _, p := range res.Pagamentos {
		if p.PendenciaSemCobranca {
			t.Fatalf("nenhum item deveria ter PendenciaSemCobranca=true: %#v", p)
		}
	}
}

// TestListarPagamentosUnificadoPaginaSoComPendencias cobre a página em que
// as pendências sozinhas já preenchem o limit inteiro — buscarCobrancas
// ainda precisa ser chamada (com limit=0) só para obter o total real de
// cobranças, mas sem trazer nenhuma linha extra.
func TestListarPagamentosUnificadoPaginaSoComPendencias(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(50)
	res, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(10, &chamadas), 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if chamadas != 1 {
		t.Fatalf("esperava 1 chamada a buscarCobrancas (só para o total), obteve %d", chamadas)
	}
	if len(res.Pagamentos) != 30 {
		t.Fatalf("esperava 30 itens (todos de pendências), obteve %d", len(res.Pagamentos))
	}
	for i, p := range res.Pagamentos {
		if !p.PendenciaSemCobranca {
			t.Fatalf("item %d deveria ser uma pendência sintética: %#v", i, p)
		}
	}
	if res.Total != 60 {
		t.Fatalf("esperava total=60 (50 pendências + 10 cobranças), obteve %d", res.Total)
	}
}

// TestListarPagamentosUnificadoPaginaMista cobre a página de transição:
// parte do limit preenchida por pendências, o resto por cobranças reais —
// o caso central que esta tarefa existe para resolver.
func TestListarPagamentosUnificadoPaginaMista(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(25)
	res, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(100, &chamadas), 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if chamadas != 1 {
		t.Fatalf("esperava 1 chamada a buscarCobrancas, obteve %d", chamadas)
	}
	if len(res.Pagamentos) != 30 {
		t.Fatalf("esperava 30 itens (25 pendências + 5 cobranças), obteve %d", len(res.Pagamentos))
	}
	for i := 0; i < 25; i++ {
		if !res.Pagamentos[i].PendenciaSemCobranca {
			t.Fatalf("item %d deveria ser pendência", i)
		}
	}
	for i := 25; i < 30; i++ {
		if res.Pagamentos[i].PendenciaSemCobranca {
			t.Fatalf("item %d deveria ser cobrança real", i)
		}
	}
	// As 5 cobranças reais na página mista devem ser exatamente as 5
	// primeiras (offset=0 do lado das cobranças, porque as pendências
	// não consomem nenhum offset delas).
	if res.Pagamentos[25].MerchantTransactionID != idxToMTID(0) {
		t.Fatalf("esperava a 1a cobranca real (idx 0), obteve %s", res.Pagamentos[25].MerchantTransactionID)
	}
	if res.Pagamentos[29].MerchantTransactionID != idxToMTID(4) {
		t.Fatalf("esperava a 5a cobranca real (idx 4), obteve %s", res.Pagamentos[29].MerchantTransactionID)
	}
	if res.Total != 125 {
		t.Fatalf("esperava total=125 (25+100), obteve %d", res.Total)
	}
}

// TestListarPagamentosUnificadoPaginaSoComCobrancasAposPendencias cobre a
// página seguinte à página mista: offset já passou de totalPendencias, e o
// offset repassado a buscarCobrancas precisa estar corretamente ajustado
// para continuar exatamente de onde a página anterior parou do lado das
// cobranças — sem pular nem repetir nenhuma cobrança real.
func TestListarPagamentosUnificadoPaginaSoComCobrancasAposPendencias(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(25)
	// Página 2 (offset=30): a página 1 já consumiu as 25 pendências +
	// cobranças[0:5]; a página 2 deve continuar em cobranças[5:35].
	res, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(100, &chamadas), 30, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pagamentos) != 30 {
		t.Fatalf("esperava 30 itens, obteve %d", len(res.Pagamentos))
	}
	for _, p := range res.Pagamentos {
		if p.PendenciaSemCobranca {
			t.Fatalf("nenhum item desta página deveria ser pendência: %#v", p)
		}
	}
	if res.Pagamentos[0].MerchantTransactionID != idxToMTID(5) {
		t.Fatalf("esperava continuar em cobranca idx 5 (sem pular nem repetir), obteve %s", res.Pagamentos[0].MerchantTransactionID)
	}
	if res.Pagamentos[29].MerchantTransactionID != idxToMTID(34) {
		t.Fatalf("esperava terminar em cobranca idx 34, obteve %s", res.Pagamentos[29].MerchantTransactionID)
	}
}

// TestListarPagamentosUnificadoLimiteExatoNoFinalDasPendencias cobre o
// caso de borda em que totalPendencias é um múltiplo exato de limit: a
// página do limite exato deve vir 100% de pendências (limiteCobrancas=0,
// sem buscar nenhuma cobrança extra), e a página seguinte deve começar do
// offset 0 das cobranças reais, sem pular nenhuma.
func TestListarPagamentosUnificadoLimiteExatoNoFinalDasPendencias(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(30)

	pagina1, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(50, &chamadas), 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina1.Pagamentos) != 30 {
		t.Fatalf("pagina1: esperava 30 itens, obteve %d", len(pagina1.Pagamentos))
	}
	for _, p := range pagina1.Pagamentos {
		if !p.PendenciaSemCobranca {
			t.Fatalf("pagina1: todos os itens deveriam ser pendências: %#v", p)
		}
	}

	pagina2, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(50, &chamadas), 30, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina2.Pagamentos) != 30 {
		t.Fatalf("pagina2: esperava 30 itens, obteve %d", len(pagina2.Pagamentos))
	}
	for _, p := range pagina2.Pagamentos {
		if p.PendenciaSemCobranca {
			t.Fatalf("pagina2: nenhum item deveria ser pendência: %#v", p)
		}
	}
	if pagina2.Pagamentos[0].MerchantTransactionID != idxToMTID(0) {
		t.Fatalf("pagina2: esperava comecar na cobranca idx 0, obteve %s", pagina2.Pagamentos[0].MerchantTransactionID)
	}
}

// TestListarPagamentosUnificadoUltimaPaginaParcial cobre a última página,
// menor que limit dos dois lados (pendências e cobranças esgotam antes de
// preencher a página inteira) — o resultado deve conter só os itens que
// realmente existem, sem preencher com itens vazios/zerados.
func TestListarPagamentosUnificadoUltimaPaginaParcial(t *testing.T) {
	var chamadas int
	pendencias := pendenciasFake(5)
	res, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(3, &chamadas), 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Pagamentos) != 8 {
		t.Fatalf("esperava 8 itens (5 pendências + 3 cobranças, nada de preenchimento), obteve %d", len(res.Pagamentos))
	}
	if res.Total != 8 {
		t.Fatalf("esperava total=8, obteve %d", res.Total)
	}
}

// TestPendenciaParaPagamentoResumoIDDeterministico confirma que a mesma
// pendência sempre produz o mesmo id entre chamadas (importante para o
// frontend usar como key de lista sem piscar a cada nova requisição), e
// que pendências diferentes produzem ids diferentes.
func TestPendenciaParaPagamentoResumoIDDeterministico(t *testing.T) {
	m := MensalidadeMesView{CodigoEstudante: "EST001", CodigoAcademia: "ACA1", AnoLetivo: "2026_2027", Mes: 9, Valor: 15000}
	a := pendenciaParaPagamentoResumo(m)
	b := pendenciaParaPagamentoResumo(m)
	if a.ID != b.ID {
		t.Fatalf("a mesma pendência produziu ids diferentes entre chamadas: %s vs %s", a.ID, b.ID)
	}
	if !a.PendenciaSemCobranca {
		t.Fatal("esperava PendenciaSemCobranca=true")
	}
	if a.Status != EstadoPendente {
		t.Fatalf("esperava status=%q, obteve %q", EstadoPendente, a.Status)
	}
	if a.AtualizadoEm != nil {
		t.Fatalf("esperava AtualizadoEm nil para uma pendência sintética, obteve %v", a.AtualizadoEm)
	}
	if len(a.Mensalidades) != 1 || a.Mensalidades[0].AnoLetivo != "2026_2027" || a.Mensalidades[0].Mes != 9 {
		t.Fatalf("mensalidades não preservou ano_letivo/mes corretamente: %#v", a.Mensalidades)
	}

	outra := m
	outra.Mes = 10
	c := pendenciaParaPagamentoResumo(outra)
	if c.ID == a.ID {
		t.Fatal("pendências diferentes (mês diferente) produziram o mesmo id")
	}
}
```

---

## 6. `internal/finance/pagamentos_unificado_integration_test.go` — criar arquivo novo

Teste de integração real (precisa de `RUN_POSTGRES_INTEGRATION=1` e Postgres — no seu ambiente, vai aparecer como `SKIP`, o que é esperado):

```go
package finance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVinculados
// cobre a razão de existir FiltrarPendenciasComCobrancaRealVinculada: desde
// a tarefa 63, PendenciasSemCobranca inclui corretamente qualquer mês
// ainda não pago, mesmo com uma tentativa de cobrança falhada — mas isso
// significa que, ao montar a lista unificada, esse mês apareceria duas
// vezes (uma como a cobrança real falhada, outra como pendência sintética
// redundante) se não for filtrado antes.
func TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVinculados(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	// ESTVINC1 tem uma cobrança falhada para setembro — continua contando
	// como pendente (tarefa 63), mas não deve gerar pendência sintética
	// duplicada para o mesmo mês.
	seedMensalidadeTurma(t, client, academia, "T-VINC-A", "2026_2027", "ESTVINC1", nil)
	// ESTVINC2 nunca teve nenhuma tentativa — continua aparecendo como
	// pendência sintética normalmente.
	seedMensalidadeTurma(t, client, academia, "T-VINC-B", "2026_2027", "ESTVINC2", nil)

	chargeID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"status": "falhada", "amount": 15000, "currency": "AOA", "description": "Propinas: 1 mensalidade(s)",
		"payment_method": "GPO_QR", "codigo_estudante": "ESTVINC1",
		"mensalidades": []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 9}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
		chargeID, integrationMerchant("VINC"), academia, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_cobrancas (charge_id,codigo_estudante,codigo_academia,ano_letivo,mes) VALUES ($1,'ESTVINC1',$2,'2026_2027',9)`,
		chargeID, academia); err != nil {
		t.Fatal(err)
	}

	mesSetembro := 9
	pendencias, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", &mesSetembro)
	if err != nil {
		t.Fatal(err)
	}
	achouEst1AntesDoFiltro := false
	for _, p := range pendencias {
		if p.CodigoEstudante == "ESTVINC1" {
			achouEst1AntesDoFiltro = true
		}
	}
	if !achouEst1AntesDoFiltro {
		t.Fatal("pré-condição do teste: ESTVINC1/setembro deveria aparecer em PendenciasSemCobranca (tarefa 63), sem isso o teste não prova nada")
	}

	filtradas, err := service.FiltrarPendenciasComCobrancaRealVinculada(ctx, pendencias)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range filtradas {
		if p.CodigoEstudante == "ESTVINC1" && p.Mes == 9 {
			t.Fatalf("ESTVINC1/setembro já tem cobrança real vinculada (falhada); não deveria sobrar em pendências após o filtro: %#v", p)
		}
	}
	achouEst2 := false
	for _, p := range filtradas {
		if p.CodigoEstudante == "ESTVINC2" && p.Mes == 9 {
			achouEst2 = true
		}
	}
	if !achouEst2 {
		t.Fatal("ESTVINC2/setembro nunca teve nenhuma cobrança; deveria continuar em pendências após o filtro")
	}
}

// TestIntegrationListarCobrancasHandlerFluxoUnificado reproduz, com
// PostgreSQL real, exatamente o que ListarCobrancasAppyPay faz: resolve
// pendências, filtra as vinculadas, e unifica com as cobranças reais numa
// única lista paginada — confirmando que as peças (PendenciasSemCobranca,
// FiltrarPendenciasComCobrancaRealVinculada, ListCobrancas,
// ListarPagamentosUnificado) se encaixam corretamente contra dados reais,
// não só contra os stubs de pagamentos_unificado_test.go.
func TestIntegrationListarCobrancasHandlerFluxoUnificado(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// 3 estudantes sem NENHUMA tentativa (viram pendência sintética).
	for i, cod := range []string{"ESTFLX01", "ESTFLX02", "ESTFLX03"} {
		seedMensalidadeTurma(t, client, academia, "T-FLX-"+string(rune('A'+i)), "2026_2027", cod, nil)
	}
	// 1 estudante com cobrança PAGA para setembro — não deve aparecer em
	// nenhuma das duas fontes (nem pendência, nem em "falhada").
	seedMensalidadeTurma(t, client, academia, "T-FLX-PG", "2026_2027", "ESTFLXPG", nil)
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,ocorrido_em) VALUES ($1,$2,'ESTFLXPG',$3,'2026_2027',9,'paga',CURRENT_TIMESTAMP)`,
		uuid.New(), uuid.New(), academia); err != nil {
		t.Fatal(err)
	}
	// 1 estudante com cobrança FALHADA para setembro — deve aparecer como
	// cobrança real (status falhada), NÃO como pendência sintética.
	seedMensalidadeTurma(t, client, academia, "T-FLX-FL", "2026_2027", "ESTFLXFL", nil)
	chargeID := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"status": "falhada", "amount": 15000, "currency": "AOA", "description": "Propinas: 1 mensalidade(s)",
		"payment_method": "GPO_QR", "codigo_estudante": "ESTFLXFL",
		"mensalidades": []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 9}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
		chargeID, integrationMerchant("FLX"), academia, payload); err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_cobrancas (charge_id,codigo_estudante,codigo_academia,ano_letivo,mes) VALUES ($1,'ESTFLXFL',$2,'2026_2027',9)`,
		chargeID, academia); err != nil {
		t.Fatal(err)
	}

	mesSetembro := 9
	pendencias, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", &mesSetembro)
	if err != nil {
		t.Fatal(err)
	}
	pendencias, err = service.FiltrarPendenciasComCobrancaRealVinculada(ctx, pendencias)
	if err != nil {
		t.Fatal(err)
	}

	buscarCobrancas := func(limit, offset int) (*CobrancaListResult, error) {
		return service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2026_2027", &mesSetembro, limit, offset)
	}

	res, err := ListarPagamentosUnificado(pendencias, buscarCobrancas, 30, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Total esperado para setembro: 3 pendências sintéticas (FLX01-03) +
	// 1 cobrança real falhada (FLXFL) = 4. ESTFLXPG (pago) não entra em
	// nenhuma das duas fontes.
	if res.Total != 4 {
		t.Fatalf("esperava total=4 (3 pendências + 1 cobrança falhada), obteve %d: %#v", res.Total, res.Pagamentos)
	}
	if len(res.Pagamentos) != 4 {
		t.Fatalf("esperava 4 itens na página, obteve %d", len(res.Pagamentos))
	}

	var pendentesSinteticas, cobrancasReais int
	estudantesVistos := map[string]bool{}
	for _, p := range res.Pagamentos {
		estudantesVistos[p.CodigoEstudante] = true
		if p.PendenciaSemCobranca {
			pendentesSinteticas++
			if p.Status != EstadoPendente {
				t.Fatalf("pendência sintética deveria ter status=%q, obteve %q", EstadoPendente, p.Status)
			}
			if p.AtualizadoEm != nil {
				t.Fatalf("pendência sintética deveria ter AtualizadoEm nil, obteve %v", p.AtualizadoEm)
			}
		} else {
			cobrancasReais++
			if p.CodigoEstudante != "ESTFLXFL" {
				t.Fatalf("única cobrança real esperada era de ESTFLXFL, veio de %s", p.CodigoEstudante)
			}
			if p.Status != "falhada" {
				t.Fatalf("esperava status=falhada na cobrança real, obteve %q", p.Status)
			}
			if p.AtualizadoEm == nil {
				t.Fatal("cobrança real deveria ter AtualizadoEm preenchido")
			}
		}
	}
	if pendentesSinteticas != 3 {
		t.Fatalf("esperava 3 pendências sintéticas, obteve %d", pendentesSinteticas)
	}
	if cobrancasReais != 1 {
		t.Fatalf("esperava 1 cobrança real, obteve %d", cobrancasReais)
	}
	if estudantesVistos["ESTFLXPG"] {
		t.Fatal("ESTFLXPG (pago) não deveria aparecer em nenhuma das duas fontes")
	}
	if estudantesVistos["ESTVINC1"] {
		t.Fatal("estudante inesperado na lista")
	}

	// Ordem: pendências primeiro, cobranças reais depois.
	for i := 0; i < 3; i++ {
		if !res.Pagamentos[i].PendenciaSemCobranca {
			t.Fatalf("item %d deveria ser pendência (pendências vêm primeiro)", i)
		}
	}
	if res.Pagamentos[3].PendenciaSemCobranca {
		t.Fatal("item 3 deveria ser a cobrança real (por último)")
	}
}
```

---

## 7. `internal/handlers/financeiro_handlers.go` — substituir conteúdo inteiro

```go
package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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

// authorizeFinanceScope autoriza o ator autenticado a agir sobre um contexto
// financeiro (contexto_tipo + codigo_academia). Uma academia só pode agir
// sobre o próprio contexto (forçado por referência). Um admin com permissão
// "fpp" pode sempre CONSULTAR (write=false) qualquer contexto, inclusive o de
// qualquer academia — mas nunca pode CRIAR, ATUALIZAR, REMOVER ou ROTACIONAR
// (write=true) as configurações financeiras de uma academia especificamente;
// essa capacidade de escrita continua disponível apenas para o contexto
// "spuri" (as configurações do próprio Spuri, que não pertencem a nenhuma
// academia) e para a própria academia sobre si mesma.
func authorizeFinanceScope(c *gin.Context, context *string, academy *string, write bool) bool {
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
	if t != "admin" || !financeAdminAllowed(c) {
		return false
	}
	if write && *context == finance.ContextoAcademia {
		return false
	}
	return true
}

// credentialScopeAuthorized resolve o contexto/academia dono de uma
// credencial AppyPay pelo seu id e reaplica authorizeFinanceScope — o mesmo
// mecanismo que já garante que uma academia só mexe nas próprias credenciais
// e que um admin precisa da permissão "fpp" para consultar (e nunca pode
// escrever no contexto de uma academia). Usado pelas rotas de consulta e
// rotação do segredo de webhook: write=false para consulta, write=true para
// rotação (que gera um novo segredo, invalidando o anterior). Já escreve a
// resposta de erro (404 ou 403) no contexto quando retorna false.
func credentialScopeAuthorized(c *gin.Context, id uuid.UUID, write bool) bool {
	contexto, academia, err := FinanceiroService.CredentialScope(c.Request.Context(), id)
	if err != nil {
		financeError(c, err)
		return false
	}
	if !authorizeFinanceScope(c, &contexto, &academia, write) {
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
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia, true) {
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
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia, true) {
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
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia, true) {
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
	if !authorizeFinanceScope(c, &contexto, &academia, false) {
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
	if !credentialScopeAuthorized(c, id, false) {
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
	if !credentialScopeAuthorized(c, id, true) {
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
	// Fora do escopo de "configurações financeiras": criar uma cobrança não
	// é uma configuração de academia, é uma operação transacional pontual —
	// o admin FPP continua podendo emiti-la em qualquer contexto, como já
	// era o comportamento antes desta correção (write=false).
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia, false) {
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
	// Fora do escopo de "configurações financeiras": gerar QR Code é uma
	// operação transacional de uma cobrança já existente, não uma
	// configuração de academia (write=false, preserva comportamento anterior).
	if !authorizeFinanceScope(c, &in.ContextoTipo, &in.CodigoAcademia, false) {
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
	if !authorizeFinanceScope(c, &contexto, &academia, false) {
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

// parseOptionalMesQuery lê um parâmetro de query opcional como mês de
// calendário (1-12). Devolve nil quando o parâmetro não foi informado, e
// erro quando foi informado mas não é um inteiro entre 1 e 12.
func parseOptionalMesQuery(c *gin.Context, param string) (*int, error) {
	raw := strings.TrimSpace(c.Query(param))
	if raw == "" {
		return nil, nil
	}
	mes, err := strconv.Atoi(raw)
	if err != nil || mes < 1 || mes > 12 {
		return nil, fmt.Errorf("%s deve ser um mês entre 1 e 12", param)
	}
	return &mes, nil
}

func ListarCobrancasAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia, false) {
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
	mes, err := parseOptionalMesQuery(c, "mes")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	estados := c.QueryArray("estado")
	origens := c.QueryArray("tipo")
	// pendências sem cobrança só são computadas quando pelo menos um dos
	// quatro filtros de escopo (turma_id, curso_id, ano_academico,
	// ano_letivo) é informado junto de codigo_academia — sem isso, a
	// varredura seria sobre a academia inteira sem limite. mes (tarefa 60)
	// só refina esse escopo, nunca o substitui. Ver finance.PendenciasSemCobranca.
	var pendencias []finance.MensalidadeMesView
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		pendencias, err = FinanceiroService.PendenciasSemCobranca(c.Request.Context(), academia, turmaID, cursoID, anoAcademico, anoLetivo, mes)
		if err != nil {
			financeError(c, err)
			return
		}
		pendencias, err = FinanceiroService.FiltrarPendenciasComCobrancaRealVinculada(c.Request.Context(), pendencias)
		if err != nil {
			financeError(c, err)
			return
		}
	}
	res, err := finance.ListarPagamentosUnificado(pendencias, func(limitCobrancas, offsetCobrancas int) (*finance.CobrancaListResult, error) {
		return FinanceiroService.ListCobrancas(c.Request.Context(), contexto, academia, estados, origens, turmaID, cursoID, anoAcademico, anoLetivo, mes, limitCobrancas, offsetCobrancas)
	}, limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pagamentos": res.Pagamentos, "total": len(res.Pagamentos), "total_geral": res.Total, "limit": limit, "offset": offset})
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
	estados := c.QueryArray("estado")
	origens := c.QueryArray("tipo")
	// pendências sem cobrança são sempre calculadas aqui (sem exigir nenhum
	// filtro extra): esta consulta já está inerentemente delimitada a UM
	// estudante, então não há o mesmo risco de varredura sem limite que
	// existe em ListarCobrancasAppyPay. Ver
	// finance.PendenciasSemCobrancaEstudante.
	pendencias, err := FinanceiroService.PendenciasSemCobrancaEstudante(c.Request.Context(), codigo, somenteAcademia)
	if err != nil {
		financeError(c, err)
		return
	}
	pendencias, err = FinanceiroService.FiltrarPendenciasComCobrancaRealVinculada(c.Request.Context(), pendencias)
	if err != nil {
		financeError(c, err)
		return
	}
	// mes não é exposto como parâmetro de query nesta rota ainda (só em
	// GET /financeiro/cobrancas, tarefa 60) — passamos nil para manter o
	// comportamento anterior inalterado aqui.
	res, err := finance.ListarPagamentosUnificado(pendencias, func(limitCobrancas, offsetCobrancas int) (*finance.CobrancaListResult, error) {
		return FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, estados, origens, turmaID, cursoID, anoAcademico, anoLetivo, nil, limitCobrancas, offsetCobrancas)
	}, limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"pagamentos": res.Pagamentos, "total": len(res.Pagamentos), "total_geral": res.Total, "limit": limit, "offset": offset})
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
```

---

## 8. `internal/handlers/financeiro_cobrancas_handlers_test.go` — substituir conteúdo inteiro

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
)

// TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado cobre o
// Problema 1 no nível HTTP: uma academia só vê as próprias cobranças, e os
// filtros de estado/tipo funcionam através da rota real.
func TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academiaA := "LSTA" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
	academiaB := "LSTB" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]

	insert := func(academia, status, origemCampo, origemValor string) {
		payload := map[string]any{"status": status, "amount": 250.0, "currency": "AOA", "description": "teste", "payment_method": "REF"}
		if origemCampo != "" {
			payload[origemCampo] = origemValor
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		merchant := "LST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), merchant, academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert(academiaA, "criada", "", "")
	insert(academiaA, "Success", "codigo_estudante", "EST-LST-1")
	insert(academiaA, "cancelada", "codigo_solicitacao", "SOL-LST-1")
	insert(academiaB, "criada", "", "")

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	call := func(academia, query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?"+query, nil)
		ctx.Set("dbClient", client)
		ctx.Set("user_id", uuid.New())
		ctx.Set("user_type", "academia")
		ctx.Set("codigo_academia", academia)
		ListarCobrancasAppyPay(ctx)
		return recorder
	}

	var body struct {
		Pagamentos []struct {
			Origem string `json:"origem"`
			Status string `json:"status"`
		} `json:"pagamentos"`
		TotalGeral int `json:"total_geral"`
	}

	all := call(academiaA, "")
	if all.Code != http.StatusOK {
		t.Fatalf("listagem sem filtro = %d: %s", all.Code, all.Body.String())
	}
	if err := json.Unmarshal(all.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 3 {
		t.Fatalf("academia A deveria ver 3 cobranças próprias, viu %d: %s", body.TotalGeral, all.Body.String())
	}

	filtrada := call(academiaA, "estado=Success")
	if err := json.Unmarshal(filtrada.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 1 || len(body.Pagamentos) != 1 || body.Pagamentos[0].Origem != "mensalidade" {
		t.Fatalf("filtro por estado=Success deveria devolver só a cobrança de mensalidade paga: %s", filtrada.Body.String())
	}

	outraAcademia := call(academiaB, "")
	if err := json.Unmarshal(outraAcademia.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 1 {
		t.Fatalf("academia B deveria ver só a própria cobrança, viu %d", body.TotalGeral)
	}
}

// TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP garante
// que a nova rota usa a mesma autorização das demais rotas de /financeiro:
// um admin sem a permissão "fpp" não pode listar cobranças.
func TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	adminID := uuid.New()
	if _, err := client.DB().Exec(`INSERT INTO projection_admins (id,nome,email,senha_hash,role,status,created_by) VALUES ($1,'gerente-lst',$2,'hash','gerente','ativo',$1)`, adminID, "gerente-lst-"+uuid.NewString()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas", nil)
	ctx.Set("dbClient", client)
	ctx.Set("user_id", adminID)
	ctx.Set("user_type", "admin")

	ListarCobrancasAppyPay(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("admin sem permissão fpp recebeu %d, queria 403: %s", recorder.Code, recorder.Body.String())
	}
}
```

---

## 9. `internal/handlers/financeiro_pendencias_handlers_test.go` — substituir conteúdo inteiro

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
)

// seedAcademiaEscolarPrivadaComTurma cria a academia + turma mínimas
// necessárias para exercitar o escopo de mensalidade (turma_id, curso_id,
// ano_academico, ano_letivo) usado por PendenciasSemCobranca e pelos novos
// filtros de ListCobrancas/ListCobrancasEstudante — ver tarefa 58.
func seedAcademiaEscolarPrivadaComTurma(t *testing.T, client *db.Client, academia, codigoTurma, anoLetivo, anoAcademico, estudante string) {
	t.Helper()
	nif := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, uuid.NewString())
	if len(nif) < 10 {
		nif = nif + strings.Repeat("0", 10-len(nif))
	}
	nif = nif[:10]
	if _, err := client.DB().Exec(`INSERT INTO projection_academias
		(id,nivel,nome,nif,codigo_academia,senha_hash,provincia,endereco,nivel_escolar,status,cursos,anos_academicos,type,ano_letivo,created_at)
		VALUES ($1,'escola','Academia HTTP teste',$2,$3,'hash','LUA','endereco','fundamental','ativo','[]'::jsonb,$4::jsonb,'private',$5,CURRENT_TIMESTAMP)`,
		uuid.New(), nif, academia, `["`+anoAcademico+`"]`, anoLetivo); err != nil {
		t.Fatal(err)
	}
	historico := `{"` + anoLetivo + `":["` + estudante + `"]}`
	if _, err := client.DB().Exec(`INSERT INTO projection_turmas
		(id,codigo_turma,codigo_academia,nivel,curso_id,turno,estudantes,historico_estudantes_ano_letivo,status,created_at)
		VALUES ($1,$2,$3,$4,NULL,'manha','[]'::jsonb,$5::jsonb,'ativo',CURRENT_TIMESTAMP)`,
		uuid.New(), codigoTurma, academia, anoAcademico, historico); err != nil {
		t.Fatal(err)
	}
}

func seedMensalidadeConfigParaHTTP(t *testing.T, client *db.Client, academia, anoAcademico string, valor float64) {
	t.Helper()
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes
		(event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,vigente_em)
		VALUES ($1,$2,$3,'fundamental',$4,NULL,$5,7,'2026-01-01')`,
		uuid.New(), uuid.New(), academia, anoAcademico, valor); err != nil {
		t.Fatal(err)
	}
}

// TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciaSemCobranca
// cobre, no nível HTTP, o problema original da tarefa 58 (um estudante que
// nunca tentou nenhuma cobrança de mensalidade é invisível para a academia
// em GET /financeiro/cobrancas a menos que ela informe um filtro de
// escopo), já na forma unificada desta tarefa: quando ano_letivo é
// informado, a pendência aparece dentro de "pagamentos", com
// pendencia_sem_cobranca=true — não mais num array separado.
func TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciaSemCobranca(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "PND" + strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
	estudante := "ESTPND1"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-PND", "2026_2027", "7_ano_fundamental", estudante)
	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	call := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?"+query, nil)
		ctx.Set("dbClient", client)
		ctx.Set("user_id", uuid.New())
		ctx.Set("user_type", "academia")
		ctx.Set("codigo_academia", academia)
		ListarCobrancasAppyPay(ctx)
		return recorder
	}

	// Sem filtro de escopo: nenhuma cobrança foi criada ainda, e
	// pendências não são computadas (evita varredura sem limite) — a
	// lista unificada fica vazia.
	semEscopo := call("")
	if semEscopo.Code != http.StatusOK {
		t.Fatalf("sem escopo = %d: %s", semEscopo.Code, semEscopo.Body.String())
	}
	var bodySemEscopo struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
		TotalGeral int                       `json:"total_geral"`
	}
	if err := json.Unmarshal(semEscopo.Body.Bytes(), &bodySemEscopo); err != nil {
		t.Fatal(err)
	}
	if len(bodySemEscopo.Pagamentos) != 0 || bodySemEscopo.TotalGeral != 0 {
		t.Fatalf("sem filtro de escopo, esperava lista vazia (nenhuma cobrança real, pendências não computadas): %s", semEscopo.Body.String())
	}

	// Com ano_letivo: o estudante nunca tentou nenhuma cobrança, então
	// TODOS os meses pendentes dele devem vir em "pagamentos", com
	// pendencia_sem_cobranca=true.
	comEscopo := call("ano_letivo=2026_2027")
	if comEscopo.Code != http.StatusOK {
		t.Fatalf("com escopo = %d: %s", comEscopo.Code, comEscopo.Body.String())
	}
	var body struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
	}
	if err := json.Unmarshal(comEscopo.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Pagamentos) == 0 {
		t.Fatalf("esperava pagamentos não vazio: %s", comEscopo.Body.String())
	}
	for _, p := range body.Pagamentos {
		if !p.PendenciaSemCobranca {
			t.Fatalf("esperava só pendências sintéticas nesta academia (nenhuma cobrança real criada): %#v", p)
		}
		if p.CodigoEstudante != estudante {
			t.Fatalf("pendência de outro estudante inesperada: %#v", p)
		}
		if p.Status != finance.EstadoPendente {
			t.Fatalf("esperava status pendente, obteve %q", p.Status)
		}
		if p.AtualizadoEm != nil {
			t.Fatalf("pendência sintética não deveria ter atualizado_em: %#v", p)
		}
	}
}

// TestIntegrationConsultarCobrancasEstudanteIncluiPendenciaSemCobranca
// cobre, no nível HTTP, a versão por estudante (sempre calculada, sem
// exigir filtro de escopo): a própria academia, consultando o histórico de
// UM estudante específico, já enxerga dentro de "pagamentos" os meses que
// ele deve e nunca tentou pagar, marcados com pendencia_sem_cobranca=true.
func TestIntegrationConsultarCobrancasEstudanteIncluiPendenciaSemCobranca(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "PNDE" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
	estudante := "ESTPND2"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-PNDE", "2026_2027", "7_ano_fundamental", estudante)
	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)
	seedEstudanteParaCobrancas(t, client, estudante, academia)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+estudante, nil)
	ctx.Params = gin.Params{{Key: "codigo", Value: estudante}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", uuid.New())
	ctx.Set("user_type", "academia")
	ctx.Set("codigo_academia", academia)

	ConsultarCobrancasEstudante(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("academia consultando estudante vinculado = %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	achouPendencia := false
	for _, p := range body.Pagamentos {
		if p.PendenciaSemCobranca {
			achouPendencia = true
		}
	}
	if !achouPendencia {
		t.Fatalf("esperava ao menos 1 pendência sintética em pagamentos: %s", recorder.Body.String())
	}
}

// TestIntegrationListarCobrancasAppyPayFiltraPorMes cobre, no nível HTTP, o
// filtro mes (tarefa 60): combinado com ano_letivo, restringe a lista
// unificada "pagamentos" a um único mês de calendário — é este par de
// parâmetros que o passo final do drill-down do frontend usa.
func TestIntegrationListarCobrancasAppyPayFiltraPorMes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "MES" + strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
	estudante := "ESTHMS1"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-MES", "2026_2027", "7_ano_fundamental", estudante)
	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	call := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?"+query, nil)
		ctx.Set("dbClient", client)
		ctx.Set("user_id", uuid.New())
		ctx.Set("user_type", "academia")
		ctx.Set("codigo_academia", academia)
		ListarCobrancasAppyPay(ctx)
		return recorder
	}

	comMesInvalido := call("ano_letivo=2026_2027&mes=13")
	if comMesInvalido.Code != http.StatusBadRequest {
		t.Fatalf("mes=13 deveria ser rejeitado com 400, obteve %d: %s", comMesInvalido.Code, comMesInvalido.Body.String())
	}

	comMesSetembro := call("ano_letivo=2026_2027&mes=9")
	if comMesSetembro.Code != http.StatusOK {
		t.Fatalf("mes=9 = %d: %s", comMesSetembro.Code, comMesSetembro.Body.String())
	}
	var body struct {
		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
	}
	if err := json.Unmarshal(comMesSetembro.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Pagamentos) != 1 {
		t.Fatalf("esperava exatamente 1 pagamento filtrando por mes=9, obteve %d: %s", len(body.Pagamentos), comMesSetembro.Body.String())
	}
	if len(body.Pagamentos[0].Mensalidades) != 1 || body.Pagamentos[0].Mensalidades[0].Mes != 9 {
		t.Fatalf("esperava mes=9 em mensalidades[0], obteve %#v", body.Pagamentos[0].Mensalidades)
	}
}
```

---

## 10. `Documentação da API.md` — atualizar seções 19.7 e 19.8

Localize a seção `#### 19.7 GET /financeiro/cobrancas` (começa com essa linha exata) e a seção seguinte `#### 19.8 GET /financeiro/cobrancas/estudante/:codigo`, e substitua TUDO desde `#### 19.7` até (mas não incluindo) a linha `#### 19.9 POST /financeiro/appypay/cobrancas/:id/cancelar` pelo texto abaixo:

```markdown
#### 19.7 GET /financeiro/cobrancas

**Escopo da rota:** lista, numa única lista paginada, os pagamentos (mensalidade, matrícula ou avulsa) do contexto autorizado — visão de academia/admin sobre pagamentos recebidos e pendentes. Quando filtrada por turma, curso, ano acadêmico ou ano letivo, a lista também inclui as pendências de mensalidade que ainda não foram pagas e não têm nenhuma cobrança real vinculada — ver `pendencia_sem_cobranca` abaixo. Para o estudante consultar o próprio histórico de pagamentos, use 19.8.

**Proteção:** autenticado + academia do próprio contexto ou admin FPP. Estudantes recebem `403` nesta rota.

**Query params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---|---|---|
| `contexto_tipo` | string | Não | Contexto financeiro consultado. Para academia autenticada é forçado para `academia`. |
| `codigo_academia` | string | Não | Academia dona das cobranças. Para academia autenticada é forçado para o código do token. |
| `estado` | string, repetível | Não | Filtra pelo texto exato (case-sensitive) persistido em `status` de uma cobrança real — mistura estados internos (`solicitada`, `criada`, `cancelada`, `falhada`) e estados crus da AppyPay (`Success`, `Pending`, `Failed`, etc). Repita o parâmetro para casar mais de um valor (`?estado=Success&estado=Pending`). **Não filtra os itens sintéticos** (`pendencia_sem_cobranca: true`) — esses sempre têm `status: "pendente"` e sempre aparecem, independente deste filtro (ver regras de negócio). |
| `tipo` | string, repetível | Não | Filtra por origem: `matricula`, `mensalidade` ou `avulsa`. Mesma ressalva de `estado`: não filtra os itens sintéticos, que são sempre `origem: "mensalidade"`. |
| `turma_id` | UUID | Não | Restringe a pagamentos de mensalidade vinculados a esta turma. Só afeta cobranças reais de origem `mensalidade` — ver regras de negócio. |
| `curso_id` | UUID | Não | Restringe a pagamentos de mensalidade vinculados a este curso. Mesma ressalva de `turma_id`. |
| `ano_academico` | string | Não | Restringe a pagamentos de mensalidade deste ano/classe (ex.: `7_ano_fundamental`). Mesma ressalva de `turma_id`. |
| `ano_letivo` | string | Não | Restringe a pagamentos de mensalidade deste ano letivo (ex.: `2026_2027`). Mesma ressalva de `turma_id`. |
| `mes` | inteiro (1-12) | Não | Restringe a um mês de calendário específico. Só tem efeito quando combinado com pelo menos um dos quatro filtros acima; sozinho, é ignorado silenciosamente (não delimita o suficiente — um mês de calendário pode abranger estudantes de vários anos letivos diferentes). `400` se fora do intervalo 1-12. |
| `limit` | inteiro | Não | Itens por página. Padrão 50, mínimo 1, máximo 1000. |
| `offset` | inteiro | Não | Deslocamento de paginação. Padrão 0. |

**Response 200:**

```json
{
  "pagamentos": [
    {
      "codigo_academia": "ACA001",
      "origem": "mensalidade",
      "status": "pendente",
      "valor": 15000.00,
      "moeda": "AOA",
      "descricao": "Propinas ACA001: 1 mensalidade(s) — pendência sem cobrança gerada",
      "codigo_estudante": "EST0002",
      "mensalidades": [{"ano_letivo": "2025_2026", "mes": 9}],
      "pendencia_sem_cobranca": true,
      "id": "b6f2e6b1-3f1a-5e9c-8f2a-1a2b3c4d5e6f",
      "contexto_tipo": "academia"
    },
    {
      "id": "4d2bbf53-c8c0-4c9a-a3f4-5a0f0cf988d1",
      "provider_charge_id": "APPYPAY-987654",
      "merchant_transaction_id": "P2608LDA000001",
      "contexto_tipo": "academia",
      "codigo_academia": "ACA001",
      "origem": "mensalidade",
      "status": "Success",
      "valor": 1000.00,
      "moeda": "AOA",
      "descricao": "Mensalidade outubro/2025",
      "metodo_pagamento": "GPO",
      "codigo_estudante": "EST0001",
      "mensalidades": [{"ano_letivo": "2025_2026", "mes": 10}],
      "atualizado_em": "2026-08-08T12:30:00Z",
      "pendencia_sem_cobranca": false
    }
  ],
  "total": 2,
  "total_geral": 2,
  "limit": 50,
  "offset": 0
}
```

**Regras de negócio:**

- Cada item de `pagamentos` tem o mesmo formato — o mesmo objeto que antes era só uma "cobrança" ganhou o campo `pendencia_sem_cobranca`, e todo item o traz, dos dois tipos:
  - `pendencia_sem_cobranca: false` — um pagamento real, com todos os campos vindos de uma cobrança de fato criada (`id` real, `atualizado_em` presente, e opcionalmente `provider_charge_id`/`merchant_transaction_id`/`metodo_pagamento` quando fizer sentido). Não devolve `payment_info`, `response` (resposta crua da AppyPay) nem `qrCodeArr`; para o detalhe completo de uma cobrança específica, use 19.6.
  - `pendencia_sem_cobranca: true` — um mês de mensalidade que ainda não foi pago (nem anulado) e não tem **nenhuma** cobrança real vinculada (nem sequer uma tentativa falhada) — sintetizado a partir da mesma computação de 19.17 (`MensalidadeMesView`). `id` é determinístico (hash estável de academia+estudante+ano_letivo+mês — a mesma pendência sempre tem o mesmo `id` entre chamadas, útil como key de lista no cliente), `status` é sempre `"pendente"`, `atualizado_em` é sempre ausente (não existe nenhuma atividade real para reportar), e `metodo_pagamento`/`provider_charge_id`/`merchant_transaction_id` também ficam ausentes.
- **`status: "pendente"` pode vir de dois casos diferentes**, e é `pendencia_sem_cobranca` que desambigua qual: (a) uma cobrança real cujo status ainda não foi resolvido pelo provedor (`pendencia_sem_cobranca: false` — a cobrança foi de fato tentada e a AppyPay devolveu um estado não-terminal); ou (b) uma pendência sintética (`pendencia_sem_cobranca: true` — não existe nenhuma cobrança para este mês).
- Um mês com uma cobrança real **falhada** aparece como item real (`status: "falhada"`, `pendencia_sem_cobranca: false`) — **não** gera também um item sintético duplicado para o mesmo mês, mesmo continuando a valer como "ainda não pago" internamente (ver 19.17 e a tarefa que corrigiu esse critério). A cobrança real, com seu histórico verdadeiro, já é a representação desse mês na lista.
- `origem` é derivada do payload persistido para itens reais, nunca gravada separadamente: `matricula` quando a cobrança tem `codigo_solicitacao`, `mensalidade` quando tem `codigo_estudante` (e não tem `codigo_solicitacao`), `avulsa` nos demais casos. Itens sintéticos são sempre `origem: "mensalidade"`.
- **Ordenação:** itens sintéticos (`pendencia_sem_cobranca: true`) sempre vêm primeiro — representam ação pendente ("isto ainda precisa de uma cobrança"). Depois vêm os itens reais, por `updated_at DESC` (atividade mais recente primeiro). A paginação (`limit`/`offset`) percorre essa ordem combinada como uma lista única.
- `total` é o número de itens nesta página; `total_geral` é o total real (pendências sintéticas + cobranças reais) que casa com os filtros aplicados.
- `turma_id`, `curso_id`, `ano_academico` e `ano_letivo` só têm efeito sobre cobranças reais de origem `mensalidade`: usar qualquer um deles exclui automaticamente cobranças de `matricula` e `avulsa` do resultado, porque essas duas origens não têm um vínculo de turma/ano letivo resolvível (a cobrança de matrícula antecede a atribuição de turma do estudante). Pendências sintéticas só existem para `mensalidade`, então também dependem de pelo menos um desses quatro filtros — sem nenhum, nenhum item sintético é computado nem aparece na lista (evita varrer a academia inteira sem limite a cada chamada).

#### 19.8 GET /financeiro/cobrancas/estudante/:codigo

**Escopo da rota:** lista, numa única lista paginada, TODOS os pagamentos que um estudante já teve — cobranças reais, em qualquer estado, academia ou origem (incluindo a cobrança da matrícula original, mesmo que ela tenha sido paga antes de o estudante existir como tal), e pendências de mensalidade que ainda não têm nenhuma cobrança real vinculada. Mesmo formato de resposta de 19.7 — ver `pendencia_sem_cobranca` ali. É a visão do próprio estudante sobre o seu histórico de pagamentos. Para a visão de academia/admin sobre cobranças recebidas, use 19.7.

**Proteção:** o próprio estudante (`:codigo` deve ser o código do token), academia à qual o estudante pertence ou pertenceu (mesmo vínculo histórico de `GET /financeiro/mensalidades/estudante/:codigo`), ou admin FPP.

**Query params:**

| Campo | Tipo | Obrigatório | Descrição |
|---|---|---|---|
| `estado` | string, repetível | Não | Mesmo filtro de 19.7. Sem filtro, devolve todos os estados. |
| `tipo` | string, repetível | Não | Mesmo filtro de 19.7: `matricula`, `mensalidade` ou `avulsa`. |
| `turma_id` | UUID | Não | Mesmo filtro de 19.7. Só tem efeito quando quem consulta é a academia (isto é, quando o contexto de uma única academia já está resolvido) — ver regras de negócio. |
| `curso_id` | UUID | Não | Mesma ressalva de `turma_id`. |
| `ano_academico` | string | Não | Mesma ressalva de `turma_id`. |
| `ano_letivo` | string | Não | Mesma ressalva de `turma_id`. |
| `limit` | inteiro | Não | Itens por página. Padrão 50, mínimo 1, máximo 1000. |
| `offset` | inteiro | Não | Deslocamento de paginação. Padrão 0. |

**Response 200:** mesma estrutura de 19.7 — com uma diferença importante, ver regras de negócio.

**Regras de negócio:**

- Diferente de 19.7, esta consulta não aceita `contexto_tipo` nem `codigo_academia`: um estudante pode ter mensalidades e matrícula em mais de uma academia (histórico), e o histórico mostra tudo — exceto quando quem consulta é uma academia, caso em que o resultado é restrito às cobranças feitas a essa academia especificamente (uma academia nunca vê pagamentos que o estudante fez a outra academia, mesmo com vínculo histórico com as duas).
- A cobrança de matrícula é resolvida pelo vínculo `codigo_estudante_gerado`, já gravado em `projection_solicitacoes_matricula` quando a solicitação é aprovada — o payload da cobrança de matrícula em si nunca grava `codigo_estudante`, porque a cobrança é anterior ao registo do estudante.
- Sem filtro de `estado`, a listagem inclui cobranças reais pendentes, falhadas e canceladas, não só as pagas — intencional: o objetivo é o estudante conseguir ver tudo que já teve, não só os pagamentos concluídos.
- `turma_id`, `curso_id`, `ano_academico` e `ano_letivo` seguem a mesma restrição de 19.7 (só afetam cobranças reais de origem `mensalidade`), mas só têm efeito quando quem consulta é uma academia (o resultado já está então restrito a uma única academia); quando quem consulta é o próprio estudante ou admin FPP sem restringir a academia, esses quatro filtros são ignorados.
- **Diferença chave em relação a 19.7:** os itens sintéticos (`pendencia_sem_cobranca: true`) aqui são **sempre** calculados, sem exigir nenhum dos quatro filtros de escopo — porque esta consulta já está inerentemente limitada a um único estudante, então não há o mesmo risco de varredura sem limite que existe em 19.7 (que precisa de pelo menos um filtro para computar pendências).
- **Não aceita** o parâmetro `mes` — só disponível em 19.7 por enquanto.

**Erros comuns:** `404` estudante inexistente, `403` estudante tentando ver outro código, academia sem vínculo ou admin sem `fpp`.

```

**Confirme antes de prosseguir:** `grep -n "pendencias_sem_cobranca" "Documentação da API.md"` deve retornar vazio depois desta alteração.

---

## 11. Fora de escopo (não altere)

- Qualquer função de `internal/finance/mensalidade_pendencias.go` e `internal/finance/mensalidade_pendencias_batch.go` (tarefas 62/63) — `PendenciasSemCobranca`, `PendenciasSemCobrancaEstudante`, `escopoMensalidadeEstudantes`, `estadosObrigacaoBatch`, `chargeIDsEscopoMensalidade` — nenhuma muda.
- `internal/finance/mensalidade.go` inteiro — `ListMensalidades`, `estadoObrigacao`, `precedenciaEstado`, etc. — não muda.
- `internal/finance/appypay.go` além da substituição cirúrgica da seção 3 — `ListCobrancas`, `ListCobrancasEstudante`, `scanCobrancaResumo`, `CobrancaListResult`, e qualquer outra função — não mudam.
- Qualquer handler além de `ListarCobrancasAppyPay` e `ConsultarCobrancasEstudante`.
- Não crie nenhum mecanismo de cache, fila, worker assíncrono, nem materialize pendências numa tabela real — a unificação é só na camada de composição da resposta (`ListarPagamentosUnificado`), exatamente como especificado.
- **Não aplique a tarefa 65** (frontend, repositório `spuripainel`) como parte desta tarefa — são repositórios e PRs separados. A tarefa 65 depende desta (64) já estar mesclada, mas a ordem de execução mecânica das duas é independente (ver observação na tarefa 65 sobre isso).

---

## 12. Checklist de validação (Codex deve executar e reportar o resultado de cada item)

Nenhum destes comandos requer PostgreSQL, Docker ou `psql`:

1. `grep -n "AtualizadoEm" internal/finance/appypay.go` — deve mostrar exatamente 2 ocorrências (ver seção 3).
2. `grep -rn "pendencia_sem_cobranca\|PendenciaSemCobranca" --include="*.go" internal/` — deve aparecer só em `pagamentos_unificado.go`, `pagamentos_unificado_test.go`, `pagamentos_unificado_integration_test.go` e nos 2 handlers modificados.
3. `grep -n "pendencias_sem_cobranca" "Documentação da API.md"` — deve retornar vazio.
4. `go build ./...` — sem erros.
5. `go vet ./...` — sem erros.
6. `gofmt -l internal/finance/appypay.go internal/finance/pagamentos_unificado.go internal/finance/pagamentos_unificado_test.go internal/finance/pagamentos_unificado_integration_test.go internal/handlers/financeiro_handlers.go internal/handlers/financeiro_cobrancas_handlers_test.go internal/handlers/financeiro_pendencias_handlers_test.go` — vazio.
7. `go test ./...` — sem falhas (testes de integração aparecem como `SKIP` sem `RUN_POSTGRES_INTEGRATION`, esperado).
8. `git diff --stat` — alterações apenas nos arquivos listados nas seções 3 a 10, mais os documentos de conclusão.

Se qualquer item falhar, não prossiga — reporte o erro exato.

---

## 13. Critérios de aceite

- [ ] `internal/finance/appypay.go` com a substituição cirúrgica exata da seção 3, e mais nada alterado no arquivo.
- [ ] `internal/finance/pagamentos_unificado.go`, `pagamentos_unificado_test.go`, `pagamentos_unificado_integration_test.go` criados exatamente com o conteúdo das seções 4, 5 e 6.
- [ ] `internal/handlers/financeiro_handlers.go`, `financeiro_cobrancas_handlers_test.go`, `financeiro_pendencias_handlers_test.go` substituídos exatamente pelo conteúdo das seções 7, 8 e 9.
- [ ] `Documentação da API.md` atualizada conforme a seção 10.
- [ ] Todos os 8 itens do checklist de validação executados e reportados com sucesso.
- [ ] Nenhum arquivo fora do escopo desta tarefa foi alterado (seção 11).

---

## 14. Procedimento de conclusão

1. Mover este arquivo para `docs/Tarefas feitas/`, com `status: concluido` e `concluido: <data de hoje>` no frontmatter (numeração 64, a próxima disponível no momento em que este documento foi escrito).
2. Atualizar `docs/Debbugs/Desenhar unificacao de cobrancas e pendencias_sem_cobranca numa unica lista paginada.md`, campo `status`, para `corrigido_via_64_...` (nome real do arquivo desta tarefa após movido).
3. Um commit único, mensagem: `Unificar cobrancas e pendencias_sem_cobranca numa unica lista paginada (pagamentos)`.
4. Reportar a Fredy: resultado de cada item do checklist e `git diff --stat` do commit. Nenhuma validação adicional com PostgreSQL real é necessária — já foi feita, incluindo num clone independente do zero.
5. Avisar explicitamente a Fredy que a tarefa 65 (frontend, repositório `spuripainel`) depende deste merge para o contrato da API ficar coerente entre os dois lados — mas os dois códigos podem ser aplicados/revisados em paralelo, só o deploy do frontend deve acontecer depois (ou junto) do deploy deste backend.

**Nenhuma etapa deste procedimento remove ou altera qualquer código relacionado à inscrição de estudantes em academias** — todas as alterações estão contidas ao módulo financeiro de cobranças/pendências (`internal/finance/appypay.go` cirurgicamente, `internal/finance/pagamentos_unificado*.go`, `internal/handlers/financeiro_*handlers*.go`) e à documentação da API, sem tocar em matrícula, cadastro, turmas ou vínculo de estudante à academia.
