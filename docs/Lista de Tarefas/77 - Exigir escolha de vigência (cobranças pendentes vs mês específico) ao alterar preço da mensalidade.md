---
criado: 2026-08-30 00:00
origem: Orquestração Claude (decisão de produto do usuário; investigação, validação de mecanismo e redação por Claude)
status: pendente
---

# Exigir escolha explícita de vigência ao alterar o valor da mensalidade: cobranças pendentes vs. mês específico (pendente)

## Prompt recomendado para executar a atualização

Implemente, em `internal/finance/mensalidade.go` (mais um novo arquivo `internal/finance/mensalidade_vigencia.go`) e em `internal/projections/financeiro_projection.go`, a exigência de que toda chamada a `POST`/`PUT /financeiro/mensalidades/configuracoes` informe obrigatoriamente um campo `modo_vigencia` com um entre dois valores — `"cobrancas_pendentes"` (o novo preço passa a valer imediatamente para toda obrigação de mensalidade que ainda não tenha nenhuma cobrança real gerada, do mês corrente em diante) ou `"mes_especifico"` (o novo preço só passa a valer a partir de um `vigencia_ano_letivo`/`vigencia_mes` futuro escolhido explicitamente) — sem nenhum valor padrão implícito. Siga exatamente as Seções 1 a 7 abaixo, que já definem os campos, a validação, o cálculo de `vigente_em`, a compatibilidade com eventos antigos do ledger, as duas correções colaterais necessárias em `ListMensalidadeConfiguracoes`/`metodosPagamentoMensalidade` e os testes obrigatórios — a investigação, a decisão de design e a validação do mecanismo já foram feitas e confirmadas com PostgreSQL real (ver Seção "Validação já realizada"); não é necessário (nem esperado) redesenhar a abordagem. `ConfigureMatricula`/`MatriculaConfiguracaoInput` e qualquer arquivo de migração **não devem ser alterados** — esta tarefa não precisa de nenhuma migração nova. Ao final, atualize `Documentação da API.md` (Seção 6) e garanta que todos os testes da Seção 7 passem.

## Contexto

`internal/finance/mensalidade.go` já versiona o preço da mensalidade por evento imutável (`MensalidadeConfigurada`): cada chamada a `ConfigureMensalidade` grava uma nova versão com `vigente_em` igual ao instante do próprio evento (`e.OccurredAt`, sempre "agora"), nunca reescrevendo uma versão anterior. `resolveConfiguracao` decide qual versão vale para uma data de referência (o 1º dia de um mês do ano letivo, ver `mesesAnoLetivo`) escolhendo a versão mais recente cujo `vigente_em` não seja posterior a essa referência. Como `vigente_em` é sempre "agora", isso já produz o mecanismo que o autor da tarefa chama de "proteção": um mês cuja referência já passou (o mês corrente, se hoje não é dia 1, e qualquer mês anterior) nunca é afetado por uma reconfiguração feita hoje — o novo preço só alcança automaticamente o **próximo** mês em diante.

Faltava dar ao chamador (a academia, ou o Spuri/FPP quando aplicável) o controle explícito sobre **duas coisas distintas** que hoje ficam implícitas na mecânica acima:

1. Fazer o novo preço alcançar imediatamente as obrigações do **mês corrente** que ainda estejam em `EstadoPendente` (ver `internal/finance/mensalidade.go`, comentário de `EstadoPendente`/`EstadoCobrancaAguardandoPagamento`: `EstadoPendente` é exclusivamente uma obrigação que **nunca** teve nenhuma cobrança real gerada; assim que existe uma cobrança real — mesmo que ainda `aguardando_pagamento`, não resolvida — o mês deixa de estar "sem cobrança" e o valor daquela cobrança já está congelado no seu próprio `payload`, imutável, e nunca é tocado por esta tarefa). Hoje isso só acontece automaticamente a partir do mês seguinte; é isso que o modo `cobrancas_pendentes` passa a permitir sob demanda.
2. Agendar o novo preço para um mês futuro **específico**, escolhido pelo chamador — não apenas "o mês seguinte a hoje", que é tudo que o mecanismo atual produz implicitamente. É isso que o modo `mes_especifico` formaliza.

`ConfigureMatricula` (taxa de matrícula) foi deliberadamente excluída desta tarefa: ao contrário da mensalidade, sua resolução (`ResolveMatriculaConfiguracao`) **sempre usa a versão mais recente**, sem nenhuma noção de data de referência por mês, e o valor de uma solicitação de matrícula pendente de pagamento já fica congelado em `projection_solicitacoes_matricula.valor_matricula` no momento da aprovação — nunca é recalculado a partir da configuração depois disso. Não existe, hoje, nenhum "mecanismo de vigor por mês" em matrícula para esta tarefa formalizar; ver "Fora de escopo".

## Decisões de escopo já tomadas (não é necessário planejar nada disto)

| Decisão | Resultado |
| --- | --- |
| `modo_vigencia=cobrancas_pendentes` alcança quais meses? | Do **mês corrente em diante**, apenas obrigações em `EstadoPendente` (sem cobrança real gerada). **Não** alcança meses já vencidos antes do mês corrente (dívida em atraso continua resolvendo pelo preço histórico do próprio mês) nem cobranças reais já geradas (pagas, anuladas ou `aguardando_pagamento`) — o valor de uma cobrança real já criada é imutável por construção (ela nunca é reconsultada em `resolveConfiguracao`), então isso já é garantido pela arquitetura; a Seção 7.7 exige um teste de regressão que trava essa garantia. |
| `modo_vigencia=mes_especifico` exige o quê? | `vigencia_ano_letivo` (`YYYY_YYYY`) + `vigencia_mes` (1–12) apontando para um mês **estritamente futuro** e que exista no calendário cobrável daquele `nivel` (ver `mesesAnoLetivo`; note que o mês 8 nunca existe para nenhum nível, e o mês 9 não existe para `superior`). |
| Matrícula (`ConfigureMatricula`) | Fora de escopo — motivo arquitetural explicado no Contexto. Não alterar. |
| Precisa de migração de banco? | Não. Nenhuma coluna nova é necessária — ver Seções 2 a 4. |
| `modo_vigencia`/`vigencia_ano_letivo`/`vigencia_mes` ficam auditáveis? | Sim, no payload do evento `MensalidadeConfigurada` no ledger (auditoria completa), mas **não** viram coluna consultável nem aparecem em `GET /financeiro/mensalidades/configuracoes` (só no response do próprio `POST`/`PUT` que os recebeu). |
| Efeito colateral que esta tarefa também precisa corrigir | Uma vez que `vigente_em` deixa de ser sempre "agora" (pode ser futuro, via `mes_especifico`), duas leituras existentes passam a poder mostrar um preço "atual" que na verdade ainda não está em vigor: `ListMensalidadeConfiguracoes` (`GET /financeiro/mensalidades/configuracoes`) e `metodosPagamentoMensalidade` (usada por `IniciarPagamentoMensalidades` para validar `metodo_pagamento`). Ambas são corrigidas nas Seções 3.4 e 3.5 (que por sua vez dependem do pré-requisito da Seção 3.3) — **confirmado por teste manual com PostgreSQL real**, ver "Validação já realizada". |
| Interação com `RemoveMensalidadeConfiguracao` | Documentada e testada (Seção 5), **não redesenhada** — remover a configuração hoje não cancela uma versão `mes_especifico` já agendada para o futuro; ela volta a valer sozinha a partir do próprio mês agendado, deixando uma lacuna sem preço vigente entre a remoção e essa data. Comportamento intencionalmente preservado nesta tarefa; ver "Fora de escopo" para uma eventual tarefa futura caso o negócio queira mudar isso. |

## Validação já realizada (não repita esta investigação)

Antes de escrever este documento, o mecanismo abaixo foi criado e testado com PostgreSQL 16 real (schema extraído literalmente das migrations 104/109) e com um pequeno programa Go isolado replicando `mesesAnoLetivo`/`posicaoNoAnoLetivo` (extraídos literalmente de `mensalidade.go`). Resultados confirmados empiricamente, sem precisar mudar uma linha da query SQL de `resolveConfiguracao`:

1. **Três versões coexistindo** (antiga `vigente_em=2026-01-01`, uma `mes_especifico` agendada para `2026-12-01`, e uma `cobrancas_pendentes` criada depois com `vigente_em=2026-08-01` = início do mês corrente): resolver para julho devolve a versão antiga (protegida); para agosto/setembro/novembro devolve a versão `cobrancas_pendentes`; para dezembro/janeiro devolve a versão `mes_especifico` agendada — ou seja, uma versão `mes_especifico` já agendada **continua valendo no seu mês**, mesmo depois de uma reconfiguração `cobrancas_pendentes` mais recente cobrir o intervalo antes dela. Documente esse comportamento em comentário de código exatamente como está descrito aqui.
2. **Duas correções `cobrancas_pendentes` no mesmo mês** (mesmo `vigente_em` exato): o desempate por `event_id DESC` já existente na query garante que a correção mais recente vence — importante porque, diferente do comportamento antigo (que praticamente nunca tinha empate de `vigente_em`, pois usava o relógio), `cobrancas_pendentes` vai gerar empates de `vigente_em` com frequência (todo o mês tem o mesmo valor truncado).
3. **Remoção "hoje" com uma `mes_especifico` já agendada para o futuro**: confirmado que a remoção invalida o preço apenas entre o instante da remoção e o mês agendado (nesse intervalo, `resolveConfiguracao` devolve `ErrNotFound`), e o mês agendado retoma sozinho, exatamente como descrito na tabela de decisões acima.
4. **Mapeamento `(ano_letivo, nivel, mes) → data`**: confirmado que `mesesAnoLetivo` nunca gera o mês 8 para nenhum nível, nem o mês 9 para `superior` — `calcularVigenciaMensalidade` (Seção 2) deve rejeitar esses casos com erro claro, e não apenas "deixar resolver para uma data errada".
5. **A view `financeiro_mensalidade_configuracoes_atual`** (`DISTINCT ON ... ORDER BY vigente_em DESC`) mostra, sem essa tarefa, a versão futura como se fosse "a atual" assim que ela é criada — confirmado com uma consulta manual reproduzindo a view. É exatamente o bug que a Seção 3.4 corrige.

---

# 1. Novos campos em `POST`/`PUT /financeiro/mensalidades/configuracoes`

## Objetivo

Tornar obrigatório, em toda chamada a `ConfigureMensalidade`, escolher entre os dois modos de vigência descritos no Contexto.

## Regra de negócio

Em `internal/finance/mensalidade.go`, adicione a `MensalidadeConfiguracaoInput`:

```go
type MensalidadeConfiguracaoInput struct {
	CodigoAcademia   string   `json:"codigo_academia"`
	Nivel            string   `json:"nivel"`
	AnoAcademico     string   `json:"ano_academico"`
	CursoID          *string  `json:"curso_id,omitempty"`
	Valor            float64  `json:"valor"`
	MesFimCobranca   int      `json:"mes_fim_cobranca"`
	MetodosPagamento []string `json:"metodos_pagamento"`
	ModoVigencia       string  `json:"modo_vigencia"`
	VigenciaAnoLetivo  *string `json:"vigencia_ano_letivo,omitempty"`
	VigenciaMes        *int    `json:"vigencia_mes,omitempty"`
}
```

E a `MensalidadeConfiguracaoView`:

```go
type MensalidadeConfiguracaoView struct {
	CodigoAcademia   string     `json:"codigo_academia"`
	Nivel            string     `json:"nivel"`
	AnoAcademico     string     `json:"ano_academico"`
	CursoID          *uuid.UUID `json:"curso_id,omitempty"`
	Valor            float64    `json:"valor"`
	MesFimCobranca   int        `json:"mes_fim_cobranca"`
	MetodosPagamento []string   `json:"metodos_pagamento"`
	VigenteEm        time.Time  `json:"vigente_em"`
	ModoVigencia      string                       `json:"modo_vigencia,omitempty"`
	ProximaAlteracao  *MensalidadeConfiguracaoView `json:"proxima_alteracao,omitempty"`
}
```

`ModoVigencia` só é preenchido no response de `POST`/`PUT` (é o modo que o próprio chamador acabou de escolher); em `GET /financeiro/mensalidades/configuracoes` este campo vem sempre vazio/omitido, porque não é persistido como coluna (ver decisão de escopo). `ProximaAlteracao` nunca é preenchido no response de `POST`/`PUT` (o próprio response já É a versão recém-criada, imediata ou futura); só é preenchido em `GET` (Seção 3.4).

## Escopo obrigatório

### 1.1 `modo_vigencia` é sempre obrigatório

Não existe valor padrão. Se `modo_vigencia` vier vazio ou diferente de `"cobrancas_pendentes"`/`"mes_especifico"`, a chamada deve falhar com `400`, **inclusive na primeira configuração de um escopo que nunca teve preço antes** — não crie um caminho especial "se é a primeira vez, não precisa escolher": o contrato do campo é sempre o mesmo, sem exceção, para o frontend nunca precisar descobrir se está criando ou atualizando.

### 1.2 Campos condicionais mutuamente exclusivos

- `modo_vigencia="cobrancas_pendentes"`: `vigencia_ano_letivo` e `vigencia_mes` **não podem** ser enviados (nem vazios/zero — rejeite se qualquer um dos dois vier presente no JSON, ou seja, ponteiro não-nil).
- `modo_vigencia="mes_especifico"`: `vigencia_ano_letivo` (formato `YYYY_YYYY`, mesma validação de `anoLetivoValido` já existente) e `vigencia_mes` (1–12) são **ambos obrigatórios**.

### 1.3 `mes_especifico` deve apontar para um mês futuro e cobrável

O par `(vigencia_ano_letivo, vigencia_mes)` deve corresponder a uma entrada real de `mesesAnoLetivo(vigencia_ano_letivo, nivel)` (isto é: para `fundamental`/`medio`, `vigencia_mes` ∈ {9,10,11,12,1,2,3,4,5,6,7}; para `superior`, `vigencia_mes` ∈ {10,11,12,1,2,3,4,5,6,7} — o mês 8 nunca é válido, e o mês 9 não é válido para `superior`). Além disso, a data resultante deve ser **estritamente posterior** ao instante da chamada (`time.Now()`); se não for, rejeitar com uma mensagem que oriente o chamador a usar `cobrancas_pendentes` em vez disso.

---

# 2. Novo arquivo `internal/finance/mensalidade_vigencia.go`

## Objetivo

Concentrar toda a lógica nova (constantes dos dois modos, cálculo de `vigente_em`, resolução "atual + próxima" reutilizada pelas Seções 3.4/3.5) num arquivo dedicado, sem misturar com o restante de `mensalidade.go`.

## Escopo obrigatório

Crie `internal/finance/mensalidade_vigencia.go` com exatamente este conteúdo (adapte apenas o comentário de topo do arquivo se necessário, mas preserve a lógica):

```go
package finance

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	// ModoVigenciaCobrancasPendentes aplica o novo preço a toda obrigação
	// de mensalidade ainda em EstadoPendente (nenhuma cobrança real
	// gerada) do mês corrente em diante, sem esperar o próximo mês.
	// Nunca alcança um mês já vencido antes do corrente, nem uma
	// cobrança real já criada (paga, anulada ou aguardando_pagamento) —
	// essa é imutável por construção (ver gerarCobranca/CreateCharge).
	ModoVigenciaCobrancasPendentes = "cobrancas_pendentes"
	// ModoVigenciaMesEspecifico agenda o novo preço para valer só a
	// partir de um (vigencia_ano_letivo, vigencia_mes) futuro escolhido
	// explicitamente pelo chamador.
	ModoVigenciaMesEspecifico = "mes_especifico"
)

// calcularVigenciaMensalidade traduz modo_vigencia (e vigencia_ano_letivo/
// vigencia_mes quando aplicável) num vigente_em concreto para o próximo
// evento MensalidadeConfigurada. É pura e não lê o banco: chame sempre
// DEPOIS de validateConfiguracaoMensalidade (que normaliza in.Nivel para
// minúsculas) e sempre passando explicitamente o "agora" do chamador —
// nunca chama time.Now() internamente — para permanecer testável sem
// banco e para que ConfigureMensalidade use o mesmo instante tanto para
// calcular vigente_em quanto para montar o response.
func calcularVigenciaMensalidade(in MensalidadeConfiguracaoInput, agora time.Time) (time.Time, error) {
	agora = agora.UTC()
	switch in.ModoVigencia {
	case ModoVigenciaCobrancasPendentes:
		if in.VigenciaAnoLetivo != nil || in.VigenciaMes != nil {
			return time.Time{}, errors.New("vigencia_ano_letivo e vigencia_mes não devem ser enviados quando modo_vigencia=cobrancas_pendentes")
		}
		return time.Date(agora.Year(), agora.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	case ModoVigenciaMesEspecifico:
		if in.VigenciaAnoLetivo == nil || strings.TrimSpace(*in.VigenciaAnoLetivo) == "" || in.VigenciaMes == nil {
			return time.Time{}, errors.New("vigencia_ano_letivo e vigencia_mes são obrigatórios quando modo_vigencia=mes_especifico")
		}
		anoLetivo := strings.TrimSpace(*in.VigenciaAnoLetivo)
		if !anoLetivoValido(anoLetivo) {
			return time.Time{}, errors.New("vigencia_ano_letivo inválido")
		}
		if !mesValido(*in.VigenciaMes) {
			return time.Time{}, errors.New("vigencia_mes deve estar entre 1 e 12")
		}
		var alvo time.Time
		encontrado := false
		for _, ref := range mesesAnoLetivo(anoLetivo, in.Nivel) {
			if ref.Month == *in.VigenciaMes {
				alvo, encontrado = ref.Data, true
				break
			}
		}
		if !encontrado {
			return time.Time{}, errors.New("vigencia_mes fora do período letivo cobrável deste nível para o vigencia_ano_letivo informado")
		}
		if !alvo.After(agora) {
			return time.Time{}, errors.New("vigencia_ano_letivo/vigencia_mes deve ser um mês futuro; use modo_vigencia=cobrancas_pendentes para aplicar a partir do mês atual")
		}
		return alvo, nil
	case "":
		return time.Time{}, errors.New("modo_vigencia é obrigatório: informe \"cobrancas_pendentes\" ou \"mes_especifico\"")
	default:
		return time.Time{}, errors.New("modo_vigencia deve ser \"cobrancas_pendentes\" ou \"mes_especifico\"")
	}
}

// mensalidadeEscopoConfig identifica um escopo (nivel, ano_academico,
// curso_id) que já teve pelo menos uma versão de configuração de
// mensalidade — independentemente de já estar removido ou ainda não
// vigente.
type mensalidadeEscopoConfig struct {
	Nivel        string
	AnoAcademico string
	CursoID      *uuid.UUID
}

// escoposConfiguradosMensalidade enumera todo escopo já configurado
// (mesmo que hoje removido ou só com versão futura) para a academia.
// Compartilhada por ListMensalidadeConfiguracoes e
// metodosPagamentoMensalidade (Seções 3.4 e 3.5) para não duplicar a
// mesma consulta.
func (s *Service) escoposConfiguradosMensalidade(ctx context.Context, academia string) ([]mensalidadeEscopoConfig, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT nivel,ano_academico,curso_id FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1 ORDER BY nivel,ano_academico,curso_id`, academia)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []mensalidadeEscopoConfig
	for rows.Next() {
		var e mensalidadeEscopoConfig
		var cursoText sql.NullString
		if err := rows.Scan(&e.Nivel, &e.AnoAcademico, &cursoText); err != nil {
			return nil, err
		}
		if cursoText.Valid {
			id, err := uuid.Parse(cursoText.String)
			if err != nil {
				return nil, err
			}
			e.CursoID = &id
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// configuracaoAtualEProxima resolve, para um escopo de mensalidade, a
// versão REALMENTE vigente agora (idêntico ao que resolveConfiguracao
// devolveria para referencia=agora) e, se existir, a próxima versão já
// agendada (vigente_em > agora) para o mesmo escopo. atual == nil
// significa que não há preço vigente agora para este escopo (removido,
// ou só existe versão futura ainda não iniciada).
func (s *Service) configuracaoAtualEProxima(ctx context.Context, academia, nivel, ano string, curso *uuid.UUID, agora time.Time) (atual *MensalidadeConfiguracaoView, proxima *MensalidadeConfiguracaoView, err error) {
	cfg, errAtual := s.resolveConfiguracao(ctx, academia, nivel, ano, curso, agora)
	if errAtual == nil {
		atual = &cfg
	} else if !errors.Is(errAtual, ErrNotFound) {
		return nil, nil, errAtual
	}
	var v MensalidadeConfiguracaoView
	var cursoText sql.NullString
	errProx := s.client.DB().QueryRowContext(ctx, `SELECT curso_id,valor::float8,mes_fim_cobranca,metodos_pagamento,vigente_em
		FROM financeiro_mensalidade_configuracoes
		WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4 AND vigente_em > $5
		ORDER BY vigente_em ASC, event_id ASC LIMIT 1`,
		academia, nivel, ano, nullableUUID(curso), agora.UTC()).Scan(&cursoText, &v.Valor, &v.MesFimCobranca, pq.Array(&v.MetodosPagamento), &v.VigenteEm)
	if errProx == sql.ErrNoRows {
		return atual, nil, nil
	}
	if errProx != nil {
		return atual, nil, errProx
	}
	v.CodigoAcademia, v.Nivel, v.AnoAcademico = academia, nivel, ano
	if cursoText.Valid {
		id, e := uuid.Parse(cursoText.String)
		if e != nil {
			return atual, nil, e
		}
		v.CursoID = &id
	}
	return atual, &v, nil
}
```

Note que `event_id` na tabela `financeiro_mensalidade_configuracoes` é `UUID PRIMARY KEY` (não serial) — a ordenação `ORDER BY vigente_em ASC, event_id ASC` só precisa ser um desempate estável para o caso (raro) de duas versões futuras com o mesmo `vigente_em`; qualquer ordem de `uuid` serve como desempate determinístico, então não é necessário nenhum tratamento especial aqui.

---

# 3. Alterações em `internal/finance/mensalidade.go`

## 3.1 `ConfigureMensalidade` usa o novo cálculo de vigência e nunca mais chama `resolveConfiguracao` para montar o response

**Objetivo:** a resposta do `POST`/`PUT` deve sempre refletir exatamente a versão que acabou de ser criada — imediata ou agendada para o futuro —, nunca "o que está vigente agora" (que, em `modo_vigencia=mes_especifico`, seria a versão *antiga*, confundindo quem acabou de agendar uma mudança futura).

Substitua o corpo de `ConfigureMensalidade` por:

```go
func (s *Service) ConfigureMensalidade(ctx context.Context, in MensalidadeConfiguracaoInput, actorID, actorType, ip string) (MensalidadeConfiguracaoView, error) {
	if s.client == nil {
		return MensalidadeConfiguracaoView{}, errors.New("serviço financeiro não inicializado")
	}
	if err := s.validateConfiguracaoMensalidade(ctx, &in); err != nil {
		return MensalidadeConfiguracaoView{}, err
	}
	in.Valor = roundAmount(in.Valor)
	agora := time.Now()
	vigenteEm, err := calcularVigenciaMensalidade(in, agora)
	if err != nil {
		return MensalidadeConfiguracaoView{}, err
	}
	payload := map[string]any{
		"codigo_academia": in.CodigoAcademia, "nivel": in.Nivel, "ano_academico": in.AnoAcademico,
		"curso_id": optionalString(in.CursoID), "valor": in.Valor, "mes_fim_cobranca": in.MesFimCobranca,
		"metodos_pagamento": in.MetodosPagamento,
		"vigente_em": vigenteEm.UTC().Format(time.RFC3339Nano),
		"modo_vigencia": in.ModoVigencia, "vigencia_ano_letivo": optionalString(in.VigenciaAnoLetivo), "vigencia_mes": in.VigenciaMes,
	}
	if err := s.recordMensalidade(ctx, in.CodigoAcademia, aggregates.MensalidadeConfigurada, payload, actorID, actorType, ip); err != nil {
		return MensalidadeConfiguracaoView{}, err
	}
	var cursoID *uuid.UUID
	if in.CursoID != nil && strings.TrimSpace(*in.CursoID) != "" {
		id, err := uuid.Parse(*in.CursoID)
		if err != nil {
			return MensalidadeConfiguracaoView{}, err
		}
		cursoID = &id
	}
	return MensalidadeConfiguracaoView{
		CodigoAcademia: in.CodigoAcademia, Nivel: in.Nivel, AnoAcademico: in.AnoAcademico, CursoID: cursoID,
		Valor: in.Valor, MesFimCobranca: in.MesFimCobranca, MetodosPagamento: in.MetodosPagamento,
		VigenteEm: vigenteEm, ModoVigencia: in.ModoVigencia,
	}, nil
}
```

`optionalString(in.VigenciaAnoLetivo)` reaproveita o helper já existente no arquivo (aceita `*string` e devolve `""` para `nil`). `in.VigenciaMes` (`*int`) pode ir direto no `map[string]any`: quando `nil`, é serializado como JSON `null` no payload do evento, o que é aceitável para fins de auditoria.

## 3.2 `validateConfiguracaoMensalidade` ganha as checagens da Seção 1.2

Adicione, em `validateConfiguracaoMensalidade` (depois das validações já existentes de `metodos_pagamento`, antes do `return nil` de cada ramo, ou em qualquer ponto do corpo — a ordem relativa às validações já existentes não importa, desde que rode antes de `ConfigureMensalidade` chamar `calcularVigenciaMensalidade`), a checagem de que `modo_vigencia`/`vigencia_ano_letivo`/`vigencia_mes` são mutuamente exclusivos conforme a Seção 1.2. Pode ser feito tanto dentro de `validateConfiguracaoMensalidade` quanto deixado inteiramente a cargo de `calcularVigenciaMensalidade` (que já cobre exatamente essas regras) — **não duplique a mesma checagem nos dois lugares**; escolha um só. Recomendado: deixar toda a validação de `modo_vigencia`/`vigencia_*` dentro de `calcularVigenciaMensalidade` (Seção 2), e não tocar em `validateConfiguracaoMensalidade` — ela já cobre `codigo_academia`/`nivel`/`ano_academico`/`curso_id`/`valor`/`mes_fim_cobranca`/`metodos_pagamento`, que são ortogonais.

## 3.3 Pré-requisito: `resolveConfiguracao` passa a também devolver `metodos_pagamento`

**Por que isto é necessário (achado da investigação, confirmado por leitura de código):** hoje, `resolveConfiguracao` (linha ~566) faz `SELECT curso_id,valor::float8,mes_fim_cobranca,vigente_em FROM financeiro_mensalidade_configuracoes WHERE ...` e escaneia só esses 4 campos — **nunca** lê `metodos_pagamento`, então `out.MetodosPagamento` sai sempre vazio de `resolveConfiguracao`. Isso já é um bug preexistente e independente desta tarefa (o response atual de `ConfigureMensalidade`, que hoje termina com `return s.resolveConfiguracao(...)`, na prática nunca devolve `metodos_pagamento` preenchido, apesar do que a documentação da API promete — a Seção 3.1 já corrige isso para o response de criação, ao parar de depender de `resolveConfiguracao` ali). Mas as Seções 3.4/3.5 abaixo **dependem** de `configuracaoAtualEProxima` (Seção 2), que por sua vez chama `resolveConfiguracao` para montar `atual` — sem este pré-requisito, `atual.MetodosPagamento` sairia sempre vazio e a correção da Seção 3.5 ficaria quebrada (sempre devolveria zero métodos de pagamento, pior do que o comportamento atual). Confirmado por leitura de código que nenhum dos outros três chamadores de `resolveConfiguracao` (`ListMensalidades`, o caminho em lote de `mensalidade_pendencias.go`, e `RemoveMensalidadeConfiguracao`) lê o campo `MetodosPagamento` do valor retornado — adicionar esta coluna é uma mudança aditiva e segura, sem nenhum efeito colateral nesses três locais.

Em `resolveConfiguracao`, troque a linha do `QueryRowContext`/`Scan` por:

```go
err := s.client.DB().QueryRowContext(ctx, `SELECT curso_id,valor::float8,mes_fim_cobranca,metodos_pagamento,vigente_em FROM financeiro_mensalidade_configuracoes WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4 ORDER BY CASE WHEN vigente_em <= $5 THEN 0 ELSE 1 END, CASE WHEN vigente_em <= $5 THEN vigente_em END DESC, CASE WHEN vigente_em > $5 THEN vigente_em END ASC, event_id DESC LIMIT 1`, academia, nivel, ano, nullableUUID(curso), referencia.UTC()).Scan(&cursoText, &out.Valor, &out.MesFimCobranca, pq.Array(&out.MetodosPagamento), &out.VigenteEm)
```

(`github.com/lib/pq` já está importado em `mensalidade.go`, usado em `ListMensalidadeConfiguracoes`; nenhum import novo é necessário para esta mudança específica.) Não altere mais nada no corpo de `resolveConfiguracao` — a checagem de remoção logo abaixo continua igual.

## 3.4 `ListMensalidadeConfiguracoes` deixa de confiar cegamente na view `_atual`

**Por que:** confirmado com PostgreSQL real (ver "Validação já realizada", item 5) que a view `financeiro_mensalidade_configuracoes_atual` (`DISTINCT ON ... ORDER BY vigente_em DESC`) devolve a versão de **maior** `vigente_em`, mesmo que ainda não tenha começado a valer — o que agora é possível com `modo_vigencia=mes_especifico`. Substitua o corpo de `ListMensalidadeConfiguracoes` por:

```go
func (s *Service) ListMensalidadeConfiguracoes(ctx context.Context, codigoAcademia string) ([]MensalidadeConfiguracaoView, error) {
	escopos, err := s.escoposConfiguradosMensalidade(ctx, codigoAcademia)
	if err != nil {
		return nil, err
	}
	agora := time.Now().UTC()
	result := []MensalidadeConfiguracaoView{}
	for _, e := range escopos {
		atual, proxima, err := s.configuracaoAtualEProxima(ctx, codigoAcademia, e.Nivel, e.AnoAcademico, e.CursoID, agora)
		if err != nil {
			return nil, err
		}
		if atual == nil {
			continue
		}
		v := *atual
		v.CodigoAcademia = codigoAcademia
		v.ProximaAlteracao = proxima
		result = append(result, v)
	}
	return result, nil
}
```

Se o escopo não tem nenhuma versão vigente agora (removido, ou só existe uma versão `mes_especifico` futura ainda não iniciada), ele simplesmente não aparece na lista — igual ao comportamento documentado hoje para um escopo removido; ver "Fora de escopo" quanto a expor esse caso de outra forma.

A query SQL direta que antes existia neste método (`SELECT ... FROM financeiro_mensalidade_configuracoes_atual WHERE ...`) deixa de ser usada aqui, mas **não apague a view nem a migration 109** — ela continua correta e é usada em outros lugares (ex.: `validateMesInicioCobranca`, que só precisa do menor `mes_fim_cobranca` já configurado, sem nenhuma relação com "está vigente agora").

## 3.5 `metodosPagamentoMensalidade` também para de confiar cegamente na view `_atual`

**Por que:** pelo mesmo motivo da Seção 3.4 (e depende do pré-requisito da Seção 3.3 já estar aplicado, senão `atual.MetodosPagamento` sairá sempre vazio) — hoje, `IniciarPagamentoMensalidades` valida `metodo_pagamento` contra `metodosPagamentoMensalidade`, que lê a mesma view. Sem esta correção, uma academia que agenda hoje (com `mes_especifico`) uma mudança de métodos de pagamento para dezembro passaria a ter, **já hoje**, os métodos antigos rejeitados e só os de dezembro aceitos — quebrando pagamentos legítimos do mês corrente até dezembro chegar. Substitua o corpo de `metodosPagamentoMensalidade` por:

```go
func (s *Service) metodosPagamentoMensalidade(ctx context.Context, academia string) ([]string, error) {
	escopos, err := s.escoposConfiguradosMensalidade(ctx, academia)
	if err != nil {
		return nil, err
	}
	agora := time.Now().UTC()
	set := map[string]bool{}
	for _, e := range escopos {
		atual, _, err := s.configuracaoAtualEProxima(ctx, academia, e.Nivel, e.AnoAcademico, e.CursoID, agora)
		if err != nil {
			return nil, err
		}
		if atual == nil {
			continue
		}
		for _, m := range atual.MetodosPagamento {
			set[strings.ToUpper(m)] = true
		}
	}
	out := make([]string, 0, len(set))
	for m := range set {
		out = append(out, m)
	}
	return out, nil
}
```

O número de escopos por academia é tipicamente pequeno (um punhado de combinações `nivel`/`ano_academico`/`curso_id`, não um-por-estudante), então chamar `configuracaoAtualEProxima`/`resolveConfiguracao` uma vez por escopo aqui **não** reintroduz o padrão N+1 que motivou o cache em `PendenciasSemCobranca` (esse é por estudante; este é por escopo de configuração).

---

# 4. `internal/projections/financeiro_projection.go` — compatibilidade com eventos antigos

## Objetivo

Eventos `MensalidadeConfigurada` gravados **antes** desta tarefa não têm `vigente_em` no payload — o projetor precisa continuar produzindo, para eles, exatamente o mesmo `vigente_em` que produz hoje (`e.OccurredAt`), para não corromper o rebuild/replay de dados já em produção.

## Escopo obrigatório

No `case "MensalidadeConfigurada":` de `Handle`, adicione o campo `VigenteEm` à struct anônima de deserialização e calcule o valor final com fallback:

```go
case "MensalidadeConfigurada":
	var in struct {
		CodigoAcademia   string   `json:"codigo_academia"`
		Nivel            string   `json:"nivel"`
		AnoAcademico     string   `json:"ano_academico"`
		CursoID          *string  `json:"curso_id"`
		Valor            float64  `json:"valor"`
		MesFimCobranca   int      `json:"mes_fim_cobranca"`
		MetodosPagamento []string `json:"metodos_pagamento"`
		VigenteEm        string   `json:"vigente_em"`
	}
	if err := json.Unmarshal(e.Payload, &in); err != nil {
		return err
	}
	if in.CodigoAcademia == "" || in.Nivel == "" || in.AnoAcademico == "" || in.Valor <= 0 {
		return fmt.Errorf("evento MensalidadeConfigurada inválido")
	}
	vigenteEm := e.OccurredAt
	if in.VigenteEm != "" {
		parsed, err := time.Parse(time.RFC3339Nano, in.VigenteEm)
		if err != nil {
			return fmt.Errorf("evento MensalidadeConfigurada com vigente_em inválido: %w", err)
		}
		vigenteEm = parsed
	}
	_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes (event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,metodos_pagamento,vigente_em) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'' )::uuid,$7,$8,$9,$10) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.Nivel, in.AnoAcademico, stringValue(in.CursoID), in.Valor, in.MesFimCobranca, pq.Array(in.MetodosPagamento), vigenteEm)
	return err
```

Adicione `"time"` ao bloco de `import` do arquivo (hoje só tem `context`, `database/sql`, `encoding/json`, `fmt`, `github.com/google/uuid`, `github.com/lib/pq`, `spuri/internal/db`).

**Não altere** o `case "MatriculaConfigurada":` nem `"MatriculaConfiguracaoRemovida":` — matrícula está fora de escopo (ver Contexto).

---

# 5. Interação com `RemoveMensalidadeConfiguracao` — documentar e testar, não redesenhar

`RemoveMensalidadeConfiguracao` grava um evento com `removido_em = time.Now()` e não muda nesta tarefa. Confirmado com PostgreSQL real (ver "Validação já realizada", item 3): se já existir uma versão `mes_especifico` agendada para o futuro no mesmo escopo, remover "agora" invalida o preço apenas entre o instante da remoção e o início do mês agendado — a partir do mês agendado, a configuração volta a resolver normalmente por conta própria, **sem** precisar ser recriada. Ou seja, remover "o preço atual" não cancela uma mudança futura já agendada separadamente; para isso, seria necessário um comando dedicado para cancelar a versão futura específica, o que está fora do escopo desta tarefa.

Adicione um comentário acima de `resolveConfiguracao` (ou do bloco que já comenta a checagem de remoção, por volta da linha 573-581 hoje) explicando este comportamento nestes termos, e implemente o teste da Seção 7.10 que trava exatamente esse cenário.

---

# 6. Atualização obrigatória de `Documentação da API.md`

Na Seção 6 (Financeiro), atualize:

### 6.1 Seção "19.14 POST e PUT /financeiro/mensalidades/configuracoes"

- Adicione `modo_vigencia` (string, obrigatório, `"cobrancas_pendentes"` ou `"mes_especifico"`), `vigencia_ano_letivo` (string, obrigatório apenas com `mes_especifico`) e `vigencia_mes` (inteiro 1–12, obrigatório apenas com `mes_especifico`) à tabela de campos do request, com a mesma redação das regras da Seção 1 deste documento.
- Atualize o JSON de exemplo do request e do response 201 para incluir os novos campos (no exemplo, use `modo_vigencia: "cobrancas_pendentes"` num, e adicione um segundo exemplo curto com `modo_vigencia: "mes_especifico"` + `vigencia_ano_letivo`/`vigencia_mes` no response, mostrando que `vigente_em` no response é a data futura agendada, não a data de hoje).
- Reescreva a frase atual *"cada chamada válida cria nova versão com `vigente_em` do servidor"* — isso deixa de ser sempre verdade; `vigente_em` agora é derivado de `modo_vigencia` (imediato = início do mês corrente; agendado = a data escolhida pelo chamador), nunca mais um timestamp arbitrário do relógio do servidor no momento da chamada.
- Documente explicitamente que uma versão `mes_especifico` já agendada continua valendo no seu próprio mês mesmo que uma reconfiguração `cobrancas_pendentes` mais recente cubra o intervalo antes dela (comportamento da Seção 2/"Validação já realizada", item 1).

### 6.2 Seção "19.15 GET /financeiro/mensalidades/configuracoes"

- Substitua a frase *"devolve apenas a versão mais recente (`vigente_em` mais alto) de cada combinação..."* pela descrição correta: devolve a versão **realmente vigente agora** de cada combinação (a mesma que seria usada para cobrar hoje), e, quando existir uma próxima alteração já agendada para o futuro (`modo_vigencia=mes_especifico`), ela aparece no campo adicional `proxima_alteracao` (mesmo formato de objeto, sem `proxima_alteracao` aninhado).
- Atualize o JSON de exemplo do response 200 mostrando um item com `proxima_alteracao` preenchido.

---

# 7. Testes obrigatórios

Localize os testes de banco em `internal/finance/mensalidade_integration_test.go` (reaproveite `seedMensalidadeAcademia`, `seedMensalidadeCurso`, `seedMensalidadeTurma`, `integrationClient`, `mensalidadeCodigo` já existentes) e os testes puros (sem banco) em `internal/finance/mensalidade_test.go` (reaproveite o padrão já usado ali para `mesesAnoLetivo`/`anoLetivoValido`). Não invente um mecanismo de mock de relógio: onde o teste precisar de "o mês corrente", calcule-o da mesma forma que o código de produção (`time.Now()` truncado), como já é costume nos testes de integração deste pacote.

1. **(Sem banco, `mensalidade_test.go`)** `calcularVigenciaMensalidade` com `modo_vigencia` vazio ou inválido → erro.
2. **(Sem banco)** `cobrancas_pendentes` com `vigencia_ano_letivo` e/ou `vigencia_mes` presentes → erro.
3. **(Sem banco)** `mes_especifico` sem `vigencia_ano_letivo` ou sem `vigencia_mes` → erro.
4. **(Sem banco)** `mes_especifico` com `vigencia_mes=8` (fundamental/médio) → erro; `vigencia_mes=9` com `nivel=superior` → erro.
5. **(Sem banco)** `mes_especifico` apontando para um mês igual ou anterior a "agora" → erro mencionando `cobrancas_pendentes`.
6. **(Sem banco)** `cobrancas_pendentes` bem-sucedido devolve `time.Date(ano, mes, 1, 0,0,0,0, UTC)` do mês corrente; `mes_especifico` bem-sucedido devolve exatamente a data de `mesesAnoLetivo` para o mês pedido.
7. **(Integração)** Money-safety: configure um preço A (`cobrancas_pendentes`), gere uma cobrança real `aguardando_pagamento` para o mês corrente (reaproveite o transporte mock de `appypay_integration_test.go`), depois reconfigure com `cobrancas_pendentes` e preço B; consulte `financeiro_cobrancas.payload->>'amount'` da cobrança já criada e confirme que **não mudou**. Este é o teste mais importante da tarefa — trava a garantia de que uma cobrança `aguardando_pagamento` nunca é afetada.
8. **(Integração)** `cobrancas_pendentes`: com preço A configurado, avance a leitura para o mês corrente e um mês anterior (chamando `resolveConfiguracao`/`ListMensalidades` diretamente com uma `referencia` passada manualmente) — confirme que o mês corrente resolve no preço B após a reconfiguração, e que um mês **anterior** ao corrente continua resolvendo no preço A.
9. **(Integração)** `mes_especifico`: configure preço A, agende preço B com `mes_especifico` para um `ano_letivo`/mês futuro; confirme via `resolveConfiguracao` que meses antes do alvo resolvem em A e o mês alvo em diante resolve em B.
10. **(Integração)** Cenário de 3 versões da "Validação já realizada" item 1: A (antiga) + B (`mes_especifico` agendado mais à frente) + C (`cobrancas_pendentes`, criado depois de B); confirme que o intervalo entre agora e o mês de B resolve em C, e que a partir do mês de B volta a resolver em B.
11. **(Integração)** Interação com remoção (Seção 5 / "Validação já realizada" item 3): A (atual) + B (`mes_especifico` futuro) + `RemoveMensalidadeConfiguracao` chamado "agora"; confirme `ErrNotFound` para referências entre a remoção e o mês de B, e que a partir do mês de B volta a resolver em B.
12. **(Integração)** `ListMensalidadeConfiguracoes`: com A (atual) + B (`mes_especifico` futuro) no mesmo escopo, confirme que o item da lista mostra os valores de A nos campos de topo e B em `proxima_alteracao`.
13. **(Integração)** `metodosPagamentoMensalidade`/`IniciarPagamentoMensalidades`: A com `metodos_pagamento=["REF"]` (atual) + B com `metodos_pagamento=["GPO_QR"]` agendado para o futuro (`mes_especifico`); confirme que, antes da data de B, iniciar pagamento de mensalidade com `REF` **funciona** e com `GPO_QR` é rejeitado (o oposto do que aconteceria sem a correção da Seção 3.5).
14. **(Integração)** Compatibilidade de replay: grave manualmente (via `s.repository`/`s.projection`, ou inserindo direto no ledger como os demais testes de integridade deste módulo já fazem — ver `financeiro_ledger_integrity_test.go` para o padrão) um evento `MensalidadeConfigurada` **sem** `vigente_em` no payload (simulando um evento anterior a esta tarefa); rode o rebuild/replay da projeção e confirme que `financeiro_mensalidade_configuracoes.vigente_em` sai igual a `ocorrido_em`/`e.OccurredAt` do evento — a mesma coisa que aconteceria sem esta tarefa.

Todos os testes de `modo_vigencia` inválido devem também ser exercidos a partir do handler HTTP (`ConfigurarMensalidade`), não só do `Service`, confirmando `400` — siga o padrão de teste HTTP já usado para os outros handlers financeiros deste pacote, se existir; caso os testes deste pacote sejam exclusivamente no nível do `Service` (parece ser o padrão observado), basta testar `Service.ConfigureMensalidade` diretamente e não é necessário criar um novo padrão de teste HTTP só para esta tarefa.

---

# Fora de escopo

- **Matrícula (`ConfigureMatricula`/`MatriculaConfiguracaoInput`)**: não tem noção de data de referência por mês (`ResolveMatriculaConfiguracao` sempre usa a versão mais recente) e o valor de uma solicitação pendente já fica congelado em `projection_solicitacoes_matricula.valor_matricula` na aprovação. Se o negócio quiser controle equivalente para matrícula, será uma tarefa própria, com desenho diferente (provavelmente uma atualização em lote de `valor_matricula` para solicitações ainda não pagas, não uma mudança em `ResolveMatriculaConfiguracao`).
- **`cobrancas_pendentes` alcançar meses já vencidos antes do mês corrente** (dívida em atraso): deliberadamente fora de escopo nesta versão — meses vencidos continuam resolvendo pelo preço histórico do próprio mês. Se o negócio quiser que uma reconfiguração também retroaja sobre atrasados, avaliar como tarefa própria (implica decidir, por exemplo, se isso deveria valer só para atraso dentro do mesmo ano letivo, e como isso interage com juros/multas caso existam no futuro).
- **Redesenhar `RemoveMensalidadeConfiguracao`** para cancelar automaticamente uma versão `mes_especifico` futura já agendada: comportamento atual preservado e testado (Seção 5/7.11); mudar isso é decisão de produto separada.
- **Expor um escopo que só tem versão futura (nunca teve preço vigente) em `GET /financeiro/mensalidades/configuracoes`**: por ora, esse escopo simplesmente não aparece na lista até sua primeira versão realmente começar a valer; o response do próprio `POST`/`PUT` que a criou já serve como confirmação imediata de que foi agendada corretamente.
- **Migração de banco**: nenhuma é necessária ou deve ser criada nesta tarefa.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `modo_vigencia` for obrigatório em toda chamada a `POST`/`PUT /financeiro/mensalidades/configuracoes`, sem exceção, com as duas opções e validações da Seção 1 implementadas;
2. `calcularVigenciaMensalidade` (Seção 2) existir em `internal/finance/mensalidade_vigencia.go` e cobrir exatamente os casos de erro e sucesso da Seção 7 (itens 1–6);
3. `ConfigureMensalidade` (Seção 3.1) sempre devolver no response a versão que acabou de criar (imediata ou agendada), nunca uma versão diferente resolvida por `resolveConfiguracao`;
4. `financeiro_projection.go` (Seção 4) ler `vigente_em` do payload quando presente e cair para `e.OccurredAt` quando ausente, sem quebrar nenhum teste de integridade de ledger/rebuild já existente;
5. `resolveConfiguracao` devolver `metodos_pagamento` (Seção 3.3), e `ListMensalidadeConfiguracoes`/`metodosPagamentoMensalidade` (Seções 3.4/3.5) nunca tratarem uma versão `mes_especifico` futura como se já estivesse vigente;
6. o teste de money-safety (Seção 7.7) passar, provando que uma cobrança `aguardando_pagamento` nunca tem seu valor alterado por uma reconfiguração posterior;
7. todos os 14 testes da Seção 7 existirem e passarem, incluindo o de compatibilidade de replay (7.14);
8. `Documentação da API.md` (Seção 6) estiver atualizada nos exatos pontos listados;
9. `ConfigureMatricula`, `MatriculaConfiguracaoInput` e todo arquivo em `migrations/` permanecerem inalterados;
10. `go build ./...` e a suíte completa de testes do módulo financeiro (`go test ./internal/finance/... ./internal/projections/...`, com `RUN_POSTGRES_INTEGRATION=1` e um PostgreSQL disponível) passarem sem nenhuma regressão nos testes já existentes antes desta tarefa.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno deste documento para `# Exigir escolha explícita de vigência ao alterar o valor da mensalidade: cobranças pendentes vs. mês específico (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
