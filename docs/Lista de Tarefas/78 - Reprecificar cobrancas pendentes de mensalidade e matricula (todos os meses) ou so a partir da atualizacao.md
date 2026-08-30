---
criado: 2026-08-30 00:00
origem: Orquestração Claude (decisão de produto do usuário, esclarecida em 2ª rodada; investigação, validação de mecanismo e redação por Claude)
status: pendente
substitui: "09 - Exigir escolha de vigência (cobranças pendentes vs mês específico) ao alterar preço da mensalidade.md" (OBSOLETO — apague esse arquivo se ainda existir; o desenho lá descrito só alcançava o mês corrente em diante e não cobria matrícula, o que não atende ao pedido real do usuário)
---

# Permitir que uma atualização de preço (mensalidade ou matrícula) escolha entre reprecificar tudo que está pendente ou só valer a partir de agora (pendente)

## Prompt recomendado para executar a atualização

Implemente a Seção 1 (migração), Seção 2 (mensalidade) e Seção 3 (matrícula) exatamente como especificado, nesta ordem — a Seção 2 depende da migração da Seção 1, e a Seção 3 é independente das duas primeiras e pode ser feita em paralelo ou depois. A investigação, o desenho e a validação do mecanismo com PostgreSQL real já foram feitos (ver "Validação já realizada"); não é necessário redesenhar nada, só seguir a especificação. Ao final, atualize `Documentação da API.md` (Seção 5) e garanta que todos os testes da Seção 4 passem. Não é necessário nenhuma UI — o frontend é uma tarefa separada em `spuripainel` (`src/docs/Tarefa - Escolha obrigatória de vigência na configuração de mensalidade e matrícula (Frontend).md`).

## Objetivo (correção sobre a primeira versão desta tarefa)

Ao configurar um novo preço de **mensalidade** ou de **taxa de matrícula**, a chamada passa a exigir a escolha explícita entre:

- **`cobrancas_pendentes`**: o novo preço vale para **toda cobrança/obrigação que ainda não foi paga e que não está aguardando pagamento**, **de todos os meses** — inclusive meses/solicitações já vencidos há vários meses, não só o mês corrente em diante. Uma cobrança real já gerada (mesmo que ainda `aguardando_pagamento`, não resolvida) nunca é alterada — seu valor já está congelado desde que foi criada.
- **`a_partir_da_atualizacao`**: o novo preço só passa a valer para o que for cobrado **depois** deste momento — é exatamente o comportamento que o sistema já tem hoje, sem nenhuma mudança (mantido só para a escolha ser sempre explícita e obrigatória, nunca implícita).

Esta é uma reescrita completa da primeira versão desta tarefa. O pedido original tinha sido entendido como "proteger o mês corrente e permitir agendar um mês futuro específico" — o esclarecimento do usuário deixou claro que o alcance precisa ser **todos os meses em atraso**, não só o mês corrente em diante, e que **matrícula também precisa da mesma escolha**, não só mensalidade. Isso muda o mecanismo internamente: só ajustar `vigente_em` (a abordagem da v1) é insuficiente porque `resolveConfiguracao` (mensalidade.go) resolve o preço **por mês de referência**, e não existe um único valor de `vigente_em` que faça uma versão nova "vencer" simultaneamente para vários meses de referência diferentes já passados sem quebrar a resolução histórica de meses anteriores a esses. A Seção 2 explica e valida a solução correta.

## Validação já realizada (não repita esta investigação)

Com PostgreSQL 16 real (schema extraído das migrations atuais) foi confirmado:

1. **A abordagem de só ajustar `vigente_em` não alcança atrasados**: com uma config antiga (`vigente_em`=janeiro, 10000) e uma mais nova (`vigente_em`=início de agosto, 12000), resolver o preço para **junho** (2 meses atrasado, nunca cobrado) devolve **10000** — a versão de agosto nunca é considerada para um mês de referência anterior a ela, não importa como seu `vigente_em` seja escolhido, porque `vigente_em` só pode "vencer" para referências **iguais ou posteriores** a ele mesmo.
2. **A solução correta** (Seção 2): quando a versão mais recente de um escopo (por ordem real de criação, nunca por `vigente_em`) foi criada com `modo_vigencia=cobrancas_pendentes`, ela passa a valer para **qualquer** mês cuja obrigação ainda esteja pendente, **ignorando completamente a comparação de data** — confirmado que a mesma consulta `ORDER BY sequencia DESC LIMIT 1` (Seção 2.1) encontra essa versão corretamente independente de qual mês está sendo resolvido.
3. **`event_id` não serve como "ordem de criação"**: é um `UUID` gerado com `uuid.New()` (`github.com/google/uuid`), ou seja, aleatório (v4) — **não** é cronologicamente ordenável. O código já existente usa `event_id DESC` como desempate de `vigente_em` empatado (em `resolveConfiguracao` e nas views `financeiro_*_configuracoes_atual`) — isso é uma falha latente pré-existente (na prática quase nunca importa, porque empates exatos de timestamp eram improváveis), mas o novo mecanismo de override **depende genuinamente** de saber qual foi a versão mais recente, então precisa de uma coluna verdadeiramente monotônica — daí a coluna nova `sequencia` (Seção 2.1), preenchida a partir de `db.Event.ID` (o `BIGSERIAL` de `spuri_ledger`, que é a única fonte de ordem cronológica real neste sistema).
4. **Remoção interage corretamente**: se a versão mais recente (`cobrancas_pendentes`) de um escopo for removida (`RemoveMensalidadeConfiguracao`), a mesma checagem de remoção já existente em `resolveConfiguracao` (comparando `removido_em` contra o intervalo relevante) também funciona para o override, bastando usar "agora" como limite (já que o override não depende de mês de referência) — confirmado com uma consulta manual.
5. **Matrícula precisa de um mecanismo diferente, não o mesmo truque**: `ResolveMatriculaConfiguracao` já sempre usa a versão mais recente (sem noção de mês de referência) — o preço de uma solicitação de matrícula **não é recalculado dinamicamente**; ele é **congelado** em `projection_solicitacoes_matricula.valor_matricula` no momento em que a academia aprova a solicitação (`AprovarSolicitacaoMatricula` → `agg.MarcarPendentePagamentoMatricula(cfg.Valor, cfg.MetodosPagamento)`, em `internal/handlers/solicitacao_matricula_handlers.go:337-341`). Isso significa que, ao contrário da mensalidade, **não existe nada para "reler dinamicamente depois"** — para `cobrancas_pendentes` alcançar solicitações já aprovadas e ainda não pagas, é preciso **escrever** um novo evento em cada solicitação afetada (Seção 3), não só mudar como o preço é lido.

---

# 1. Migração: `financeiro_mensalidade_configuracoes` ganha `sequencia` e `modo_vigencia`

Crie `migrations/116_financeiro_mensalidade_modo_vigencia.sql` (ajuste o número se, no momento de implementar, já existir um `116_*.sql` — use o próximo número livre; hoje o maior é `115_corrigir_periodo_semestre_superior.sql`):

```sql
BEGIN;

ALTER TABLE financeiro_mensalidade_configuracoes
    ADD COLUMN sequencia BIGINT,
    ADD COLUMN modo_vigencia TEXT NOT NULL DEFAULT 'a_partir_da_atualizacao'
        CHECK (modo_vigencia IN ('a_partir_da_atualizacao', 'cobrancas_pendentes'));

-- Backfill: para toda versão já existente (criada antes desta tarefa), a
-- ordem real de criação é a mesma ordem em que o evento entrou no ledger —
-- e modo_vigencia='a_partir_da_atualizacao' (o default acima) preserva
-- exatamente o comportamento que essas versões sempre tiveram, já que o
-- mecanismo de override (Seção 2.2) só ativa para modo_vigencia=
-- 'cobrancas_pendentes'.
UPDATE financeiro_mensalidade_configuracoes c
SET sequencia = l.id
FROM spuri_ledger l
WHERE l.event_id = c.event_id;

ALTER TABLE financeiro_mensalidade_configuracoes
    ALTER COLUMN sequencia SET NOT NULL;

CREATE INDEX idx_fin_mensalidade_config_sequencia
    ON financeiro_mensalidade_configuracoes (codigo_academia, nivel, ano_academico, curso_id, sequencia DESC);

-- A view financeiro_mensalidade_configuracoes_atual passa a expor
-- modo_vigencia também, para que ListMensalidadeConfiguracoes possa
-- devolvê-lo (só informativo — não muda a semântica da view, que continua
-- resolvendo por vigente_em/event_id DESC como já fazia).
DROP VIEW financeiro_mensalidade_configuracoes_atual;
CREATE VIEW financeiro_mensalidade_configuracoes_atual AS
SELECT c.codigo_academia, c.nivel, c.ano_academico, c.curso_id, c.valor,
       c.mes_fim_cobranca, c.metodos_pagamento, c.vigente_em, c.modo_vigencia
FROM (
    SELECT DISTINCT ON (codigo_academia, nivel, ano_academico, curso_id)
        codigo_academia, nivel, ano_academico, curso_id, valor,
        mes_fim_cobranca, metodos_pagamento, vigente_em, modo_vigencia, sequencia
    FROM financeiro_mensalidade_configuracoes
    ORDER BY codigo_academia, nivel, ano_academico, curso_id, sequencia DESC
) c
LEFT JOIN LATERAL (
    SELECT removido_em FROM financeiro_mensalidade_configuracoes_remocoes r
    WHERE r.codigo_academia = c.codigo_academia AND r.nivel = c.nivel
      AND r.ano_academico = c.ano_academico
      AND r.curso_id IS NOT DISTINCT FROM c.curso_id
      AND r.removido_em >= c.vigente_em
    ORDER BY r.removido_em DESC LIMIT 1
) rm ON true
WHERE rm.removido_em IS NULL;

COMMIT;

COMMENT ON COLUMN financeiro_mensalidade_configuracoes.sequencia IS
    'Ordem real de criação (copiada de spuri_ledger.id no momento da projeção) — event_id é um UUID aleatório e não serve para isso. Usada por Service.ultimaConfiguracaoMensalidade (mensalidade_vigencia.go) para decidir qual foi a versão mais recente, independente de vigente_em.';
COMMENT ON COLUMN financeiro_mensalidade_configuracoes.modo_vigencia IS
    'Escolha feita pelo chamador em ConfigureMensalidade: a_partir_da_atualizacao (default, comportamento histórico) ou cobrancas_pendentes (a versão mais recente com este modo passa a valer para qualquer obrigação ainda pendente, de qualquer mês — ver resolveConfiguracaoEfetiva).';
```

**Não altere** `financeiro_matricula_configuracoes` — matrícula não precisa de `sequencia`/`modo_vigencia` como coluna (ver Seção 3, o mecanismo lá é outro).

---

# 2. Mensalidade

## 2.1 Novo arquivo `internal/finance/mensalidade_vigencia.go`

```go
package finance

// Este arquivo implementa a escolha obrigatória de modo_vigencia ao
// configurar um novo preço de mensalidade: "cobrancas_pendentes" (o novo
// preço vale para toda obrigação ainda pendente — sem cobrança real
// gerada —, de qualquer mês, inclusive atrasados) ou
// "a_partir_da_atualizacao" (comportamento histórico: resolveConfiguracao
// por data, sem nenhuma mudança).
//
// resolveConfiguracao (mensalidade.go) permanece INALTERADA: ela resolve
// por mês de referência e continua sendo a fonte de verdade para
// qualquer mês cuja obrigação já não esteja mais pendente (paga/anulada) —
// nunca queremos reescrever o preço histórico de um mês já resolvido.
// resolveConfiguracaoEfetiva (abaixo) é a nova porta de entrada: só ela
// decide, com base em "esta obrigação está pendente?", se deve ignorar a
// data e usar a última versão cobrancas_pendentes, ou cair para
// resolveConfiguracao normalmente.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	ModoVigenciaAPartirDaAtualizacao = "a_partir_da_atualizacao"
	ModoVigenciaCobrancasPendentes   = "cobrancas_pendentes"
)

func modoVigenciaValido(v string) bool {
	return v == ModoVigenciaAPartirDaAtualizacao || v == ModoVigenciaCobrancasPendentes
}

// ultimaConfiguracaoMensalidade devolve a versão mais RECENTEMENTE CRIADA
// (por sequencia, nunca por vigente_em/event_id) de um escopo, e se ela
// está removida agora. Ao contrário de resolveConfiguracao, não recebe
// nenhuma data de referência — só existe para responder "qual foi a
// última decisão tomada para este escopo, e ela ainda vale?".
func (s *Service) ultimaConfiguracaoMensalidade(ctx context.Context, academia, nivel, ano string, curso *uuid.UUID, agora time.Time) (view MensalidadeConfiguracaoView, removida bool, err error) {
	var cursoText sql.NullString
	err = s.client.DB().QueryRowContext(ctx, `SELECT curso_id,valor::float8,mes_fim_cobranca,metodos_pagamento,vigente_em,modo_vigencia
		FROM financeiro_mensalidade_configuracoes
		WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4
		ORDER BY sequencia DESC LIMIT 1`,
		academia, nivel, ano, nullableUUID(curso)).Scan(&cursoText, &view.Valor, &view.MesFimCobranca, pq.Array(&view.MetodosPagamento), &view.VigenteEm, &view.ModoVigencia)
	if err == sql.ErrNoRows {
		return MensalidadeConfiguracaoView{}, false, fmt.Errorf("%w: configuração de mensalidade", ErrNotFound)
	}
	if err != nil {
		return MensalidadeConfiguracaoView{}, false, err
	}
	view.CodigoAcademia, view.Nivel, view.AnoAcademico = academia, nivel, ano
	if cursoText.Valid {
		id, e := uuid.Parse(cursoText.String)
		if e != nil {
			return MensalidadeConfiguracaoView{}, false, e
		}
		view.CursoID = &id
	}
	var removidoEm time.Time
	errRem := s.client.DB().QueryRowContext(ctx, `SELECT removido_em FROM financeiro_mensalidade_configuracoes_remocoes
		WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4
		AND removido_em >= $5 AND removido_em <= $6 ORDER BY removido_em DESC LIMIT 1`,
		academia, nivel, ano, nullableUUID(curso), view.VigenteEm, agora.UTC()).Scan(&removidoEm)
	if errRem == nil {
		return view, true, nil
	}
	if errRem != sql.ErrNoRows {
		return view, false, errRem
	}
	return view, false, nil
}

// resolveConfiguracaoEfetiva é a nova porta de entrada usada por
// ListMensalidades e PendenciasSemCobranca no lugar de chamar
// resolveConfiguracao diretamente. pendente deve ser exatamente
// (state == EstadoPendente) do mês/obrigação sendo resolvido — o chamador
// SEMPRE precisa saber isso ANTES de chamar esta função (ver Seções 2.3 e
// 2.4, que reordenam os respectivos laços para calcular o estado antes de
// resolver o preço).
func (s *Service) resolveConfiguracaoEfetiva(ctx context.Context, academia, nivel, ano string, curso *uuid.UUID, referencia time.Time, pendente bool) (MensalidadeConfiguracaoView, error) {
	if pendente {
		ultima, removida, err := s.ultimaConfiguracaoMensalidade(ctx, academia, nivel, ano, curso, time.Now())
		if err != nil && !errors.Is(err, ErrNotFound) {
			return MensalidadeConfiguracaoView{}, err
		}
		if err == nil {
			if removida {
				return MensalidadeConfiguracaoView{}, fmt.Errorf("%w: configuração de mensalidade removida", ErrNotFound)
			}
			if ultima.ModoVigencia == ModoVigenciaCobrancasPendentes {
				return ultima, nil
			}
		}
		// err == ErrNotFound (nunca configurado) ou modo == a_partir_da_atualizacao:
		// cai para a resolução normal por data abaixo, exatamente como sempre.
	}
	return s.resolveConfiguracao(ctx, academia, nivel, ano, curso, referencia)
}
```

Adicione `"fmt"` ao import se ainda não usar (é usado em `fmt.Errorf`). Confirme os imports finais compilando — o esqueleto acima pode precisar de ajuste de imports conforme o que já está em uso no arquivo.

## 2.2 `MensalidadeConfiguracaoInput`/`View` ganham `modo_vigencia` (mensalidade.go)

```go
type MensalidadeConfiguracaoInput struct {
	CodigoAcademia   string   `json:"codigo_academia"`
	Nivel            string   `json:"nivel"`
	AnoAcademico     string   `json:"ano_academico"`
	CursoID          *string  `json:"curso_id,omitempty"`
	Valor            float64  `json:"valor"`
	MesFimCobranca   int      `json:"mes_fim_cobranca"`
	MetodosPagamento []string `json:"metodos_pagamento"`
	ModoVigencia     string   `json:"modo_vigencia"`
}

type MensalidadeConfiguracaoView struct {
	CodigoAcademia   string     `json:"codigo_academia"`
	Nivel            string     `json:"nivel"`
	AnoAcademico     string     `json:"ano_academico"`
	CursoID          *uuid.UUID `json:"curso_id,omitempty"`
	Valor            float64    `json:"valor"`
	MesFimCobranca   int        `json:"mes_fim_cobranca"`
	MetodosPagamento []string   `json:"metodos_pagamento"`
	VigenteEm        time.Time  `json:"vigente_em"`
	ModoVigencia     string     `json:"modo_vigencia,omitempty"`
}
```

Em `validateConfiguracaoMensalidade`, adicione (junto das outras validações de `in`, em qualquer ponto do corpo):

```go
if !modoVigenciaValido(in.ModoVigencia) {
	return errors.New(`modo_vigencia é obrigatório: informe "cobrancas_pendentes" ou "a_partir_da_atualizacao"`)
}
```

Em `ConfigureMensalidade`, inclua `modo_vigencia` no payload do evento (para auditoria e para o projetor gravar a coluna nova):

```go
payload := map[string]any{"codigo_academia": in.CodigoAcademia, "nivel": in.Nivel, "ano_academico": in.AnoAcademico, "curso_id": optionalString(in.CursoID), "valor": in.Valor, "mes_fim_cobranca": in.MesFimCobranca, "metodos_pagamento": in.MetodosPagamento, "modo_vigencia": in.ModoVigencia}
```

O resto de `ConfigureMensalidade` **não muda** — continua terminando com `return s.resolveConfiguracao(ctx, in.CodigoAcademia, in.Nivel, in.AnoAcademico, cursoID, time.Now().UTC())`, que devolve corretamente a versão recém-criada (já que `vigente_em` continua sendo sempre "agora" em ambos os modos — só o `modo_vigencia` muda o que acontece depois, na leitura).

`ListMensalidadeConfiguracoes` ganha `modo_vigencia` na projeção de colunas (a view já expõe a coluna, Seção 1):

```go
rows, err := s.client.DB().QueryContext(ctx, `SELECT nivel,ano_academico,curso_id,valor::float8,mes_fim_cobranca,metodos_pagamento,vigente_em,modo_vigencia FROM financeiro_mensalidade_configuracoes_atual WHERE codigo_academia=$1 ORDER BY nivel,ano_academico,curso_id`, codigoAcademia)
...
if err := rows.Scan(&v.Nivel, &v.AnoAcademico, &curso, &v.Valor, &v.MesFimCobranca, pq.Array(&v.MetodosPagamento), &v.VigenteEm, &v.ModoVigencia); err != nil {
```

## 2.3 `internal/projections/financeiro_projection.go`

No `case "MensalidadeConfigurada":`, adicione `ModoVigencia` à struct de deserialização, valide, e inclua `e.ID` (sequencia) + `modoVigencia` no INSERT:

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
		ModoVigencia     string   `json:"modo_vigencia"`
	}
	if err := json.Unmarshal(e.Payload, &in); err != nil {
		return err
	}
	if in.CodigoAcademia == "" || in.Nivel == "" || in.AnoAcademico == "" || in.Valor <= 0 {
		return fmt.Errorf("evento MensalidadeConfigurada inválido")
	}
	modoVigencia := in.ModoVigencia
	if modoVigencia == "" {
		// Eventos gravados ANTES desta tarefa nunca tiveram este campo no
		// payload — tratar como a_partir_da_atualizacao preserva
		// exatamente o comportamento que sempre tiveram (replay
		// determinístico do ledger antigo).
		modoVigencia = "a_partir_da_atualizacao"
	}
	_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes (event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,metodos_pagamento,vigente_em,sequencia,modo_vigencia) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'' )::uuid,$7,$8,$9,$10,$11,$12) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.Nivel, in.AnoAcademico, stringValue(in.CursoID), in.Valor, in.MesFimCobranca, pq.Array(in.MetodosPagamento), e.OccurredAt, e.ID, modoVigencia)
	return err
```

**Não altere** o `case "MensalidadeConfiguracaoRemovida":` nem nada de matrícula neste arquivo.

## 2.4 `ListMensalidades` (mensalidade.go) usa `resolveConfiguracaoEfetiva`

O laço precisa ser **reordenado**: hoje resolve o preço (`resolveConfiguracao`) antes de calcular o estado (`estadoObrigacao`); precisa ser o contrário, porque `resolveConfiguracaoEfetiva` exige saber `pendente` antes de resolver.

Substitua o corpo do laço interno de `ListMensalidades` (a partir de `for _, ref := range mesesAnoLetivo(...)`) por:

```go
for _, ref := range mesesAnoLetivo(v.AnoLetivo, v.Nivel) {
	if posicaoNoAnoLetivo(ref.Month, natural) < inicioPos {
		continue
	}
	state, audit, err := s.estadoObrigacao(ctx, codigoEstudante, v.CodigoAcademia, v.AnoLetivo, ref.Month)
	if err != nil {
		return nil, err
	}
	cfg, err := s.resolveConfiguracaoEfetiva(ctx, v.CodigoAcademia, v.Nivel, v.AnoAcademico, v.CursoID, ref.Data, state == EstadoPendente)
	if errors.Is(err, ErrNotFound) {
		continue
	}
	if err != nil {
		return nil, err
	}
	if posicaoNoAnoLetivo(ref.Month, natural) > posicaoNoAnoLetivo(cfg.MesFimCobranca, natural) {
		continue
	}
	result = append(result, MensalidadeMesView{CodigoEstudante: codigoEstudante, CodigoAcademia: v.CodigoAcademia, AnoLetivo: v.AnoLetivo, Mes: ref.Month, DataReferencia: ref.Data, Nivel: v.Nivel, AnoAcademico: v.AnoAcademico, CursoID: v.CursoID, Valor: cfg.Valor, MesFimCobranca: cfg.MesFimCobranca, Estado: state, EventosAuditoria: audit})
}
```

Note que isto move `estadoObrigacao` para ANTES do corte por `mes_fim_cobranca` — um mês fora do período cobrável ainda tem `estadoObrigacao` calculado agora, mas isso é só uma consulta extra e descartada para esses meses (o mesmo padrão de custo que `resolveConfiguracao` já tinha antes de ser cortado); não muda nenhum resultado observável, só a ordem das chamadas.

## 2.5 `PendenciasSemCobranca` (mensalidade_pendencias.go) usa `resolveConfiguracaoEfetiva`

Aqui a reordenação é ainda mais simples porque `estados` já foi buscado inteiro em memória (`estadosObrigacaoBatch`, linha 275) antes do laço — não é uma chamada de I/O nova, só uma leitura de mapa que precisa acontecer mais cedo no laço. Substitua o trecho do laço a partir de `chaveCfg := ...` até o fechamento do `if estado != EstadoPendente { continue }` por:

```go
chaveMes := v.CodigoEstudante + "|" + v.AnoLetivo + "|" + strconv.Itoa(ref.Month)
estado := EstadoPendente
var audit []uuid.UUID
if info, ok := estados[chaveMes]; ok {
	estado = info.Estado
	audit = info.Audit
}
if estado != EstadoPendente {
	continue
}
chaveCfg := v.CodigoAcademia + "|" + v.Nivel + "|" + v.AnoAcademico + "|" + optionalUUID(v.CursoID) + "|" + ref.Data.Format("2006-01")
cfg, temCfg := cfgCache[chaveCfg]
if !temCfg {
	if cfgNaoEncontrada[chaveCfg] {
		continue
	}
	cfg, err = s.resolveConfiguracaoEfetiva(ctx, v.CodigoAcademia, v.Nivel, v.AnoAcademico, v.CursoID, ref.Data, true)
	if errors.Is(err, ErrNotFound) {
		cfgNaoEncontrada[chaveCfg] = true
		continue
	}
	if err != nil {
		return nil, err
	}
	cfgCache[chaveCfg] = cfg
}
if posicaoNoAnoLetivo(ref.Month, natural) > posicaoNoAnoLetivo(cfg.MesFimCobranca, natural) {
	continue
}
```

O quinto argumento é sempre `true` aqui porque, neste ponto do laço reordenado, só se chega até a resolução de preço quando `estado == EstadoPendente` (o `continue` acima já descartou todo o resto) — mantenha `cfgCache`/`cfgNaoEncontrada` com a MESMA chave de hoje (sem acrescentar `pendente` nela): como esta função **só** produz linhas pendentes, o cache nunca precisa distinguir os dois casos.

Ajuste o restante do corpo do laço (a montagem de `MensalidadeMesView` em `out = append(...)`) para não recalcular `estado`/`audit` de novo — já estão calculados acima, exatamente como já estavam no código original, só que mais cedo.

## 2.6 (opcional, mas recomendado) `resolveConfiguracao` usa `sequencia DESC` em vez de `event_id DESC`

Já que a coluna `sequencia` passa a existir, aproveite para corrigir o mesmo desempate aleatório em `resolveConfiguracao` (mensalidade.go, a query em `resolveConfiguracao`): troque `event_id DESC` por `sequencia DESC` no `ORDER BY`. É uma correção de correção pré-existente (Seção "Validação já realizada", item 3) que nunca foi reportada como bug porque colisões de `vigente_em` eram improváveis até hoje — não é estritamente necessária para esta tarefa funcionar (o `resolveConfiguracaoEfetiva` novo não depende deste desempate), mas corrigir agora evita deixar uma armadilha para trás. Se preferir não mexer nisso agora por prudência (é uma função crítica e amplamente testada), pule este item — não bloqueia a Seção 2.

---

# 3. Matrícula

## 3.1 `MatriculaConfiguracaoInput`/`View` ganham `modo_vigencia` (matricula.go)

```go
type MatriculaConfiguracaoInput struct {
	CodigoAcademia   string   `json:"codigo_academia"`
	Nivel            string   `json:"nivel"`
	AnoAcademico     string   `json:"ano_academico"`
	CursoID          *string  `json:"curso_id,omitempty"`
	Valor            float64  `json:"valor"`
	MetodosPagamento []string `json:"metodos_pagamento"`
	ModoVigencia     string   `json:"modo_vigencia"`
}
type MatriculaConfiguracaoView struct {
	CodigoAcademia    string                    `json:"codigo_academia"`
	Nivel             string                    `json:"nivel"`
	AnoAcademico      string                    `json:"ano_academico"`
	CursoID           *uuid.UUID                `json:"curso_id,omitempty"`
	Valor             float64                   `json:"valor"`
	MetodosPagamento  []string                  `json:"metodos_pagamento"`
	VigenteEm         time.Time                 `json:"vigente_em"`
	ModoVigencia      string                    `json:"modo_vigencia,omitempty"`
	Repricing         *MatriculaRepricingResumo `json:"repricing_pendentes,omitempty"`
}

// MatriculaRepricingResumo resume o efeito de modo_vigencia=cobrancas_pendentes
// sobre solicitações já aprovadas e ainda não pagas (Seção 3.3). Só é
// preenchido no response de ConfigureMatricula quando esse modo foi usado;
// nunca aparece em ListMatriculaConfiguracoes.
type MatriculaRepricingResumo struct {
	Atualizadas int `json:"atualizadas"`
	Ignoradas   int `json:"ignoradas"`
	Falhas      int `json:"falhas"`
}
```

Em `validateConfiguracaoMatricula`, adicione a mesma checagem da Seção 2.2:

```go
if !modoVigenciaValido(in.ModoVigencia) {
	return errors.New(`modo_vigencia é obrigatório: informe "cobrancas_pendentes" ou "a_partir_da_atualizacao"`)
}
```

(`modoVigenciaValido` é a mesma função da Seção 2.1 — `mensalidade_vigencia.go` fica no mesmo pacote `finance`, então é reaproveitada diretamente, sem duplicar.)

## 3.2 Novo evento no aggregate `SolicitacaoMatricula`

Em `internal/domain/aggregates/solicitacao_matricula.go`, siga exatamente o padrão de `MarcarPendentePagamentoMatricula`/`SolicitacaoMatriculaAprovadaPendentePagamentoEvent`/`applyAprovadaPendentePagamento` (linhas 84-97 e 486-497 e 605-613 do arquivo atual):

```go
// AtualizarValorPendentePagamentoMatricula reprecifica uma solicitação que
// já está aprovada e aguardando pagamento de matrícula (ver
// ConfigureMatricula com modo_vigencia=cobrancas_pendentes, matricula.go).
// Só pode ser chamado nesse estado — nunca muda o Status.
func (s *SolicitacaoMatricula) AtualizarValorPendentePagamentoMatricula(valor float64, metodos []string) error {
	if s.Status != StatusSolicitacaoAprovadaPendentePagamentoMatricula || valor <= 0 || len(metodos) == 0 {
		return fmt.Errorf("solicitação deve estar pendente de pagamento de matrícula, com valor e métodos de pagamento válidos")
	}
	ev := &SolicitacaoMatriculaValorPendenteAtualizadoEvent{BaseEvent: BaseEvent{EventType: "SolicitacaoMatriculaValorPendenteAtualizado", AggregateID: s.ID}, CodigoSolicitacao: s.CodigoSolicitacao, CodigoAcademia: s.CodigoAcademia, Valor: valor, MetodosPagamento: metodos, OccurredAt: time.Now().UTC()}
	s.RaiseEvent(ev)
	return nil
}
```

Struct do evento (ao lado de `SolicitacaoMatriculaAprovadaPendentePagamentoEvent`):

```go
type SolicitacaoMatriculaValorPendenteAtualizadoEvent struct {
	BaseEvent
	CodigoSolicitacao string
	CodigoAcademia    string
	Valor             float64
	MetodosPagamento  []string
	OccurredAt        time.Time
}

func (e *SolicitacaoMatriculaValorPendenteAtualizadoEvent) GetPayload() interface{} { return e }
func (e *SolicitacaoMatriculaValorPendenteAtualizadoEvent) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}
```

Em `Apply` (o `switch event.GetEventType()`), adicione:

```go
case "SolicitacaoMatriculaValorPendenteAtualizado":
	return s.applyValorPendenteAtualizado(event)
```

E o handler (ao lado de `applyAprovadaPendentePagamento`):

```go
func (s *SolicitacaoMatricula) applyValorPendenteAtualizado(event DomainEvent) error {
	data, _ := json.Marshal(event.GetPayload())
	var ev SolicitacaoMatriculaValorPendenteAtualizadoEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	s.ValorMatricula = &ev.Valor
	s.MetodosPagamentoMatricula = append([]string(nil), ev.MetodosPagamento...)
	s.UpdatedAt = ev.OccurredAt
	return nil
}
```

## 3.3 Projeção: `internal/projections/solicitacao_matricula_projection.go`

Adicione o case (em `Handle`, ao lado de `"SolicitacaoMatriculaAprovadaPendentePagamento"`):

```go
case "SolicitacaoMatriculaValorPendenteAtualizado":
	return p.handleValorPendenteAtualizado(event)
```

E o handler (ao lado de `handleAprovadaPendentePagamento` — note que este **não** mexe em `status`, só nos dois campos de preço):

```go
func (p *SolicitacaoMatriculaProjection) handleValorPendenteAtualizado(event db.Event) error {
	var payload struct {
		Valor            float64   `json:"Valor"`
		MetodosPagamento []string  `json:"MetodosPagamento"`
		OccurredAt       time.Time `json:"OccurredAt"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	_, err := p.client.DB().Exec(`UPDATE projection_solicitacoes_matricula SET valor_matricula=$1, metodos_pagamento_matricula=$2, updated_at=$3, version=$4, last_event_id=$5 WHERE id=$6`, payload.Valor, pq.Array(payload.MetodosPagamento), payload.OccurredAt, event.EventVersion, event.EventID, event.AggregateID)
	return err
}
```

Confirme se `Rebuild()` (mesmo arquivo) processa eventos por `event_type` de forma genérica (via `Handle`) — se sim, nenhuma alteração adicional é necessária ali; o novo `case` já é coberto automaticamente pelo replay.

## 3.4 `ConfigureMatricula` (matricula.go): reprecificar solicitações pendentes

Adicione ao mesmo arquivo:

```go
// reprecificarSolicitacoesMatriculaPendentes é chamada por ConfigureMatricula
// quando modo_vigencia=cobrancas_pendentes. Busca toda solicitação já
// aprovada e aguardando pagamento de matrícula (status=
// aprovada_pendente_pagamento_matricula) cujo escopo (nivel + ano_academico
// + curso, derivado dos mesmos campos que escopoMatriculaSolicitacao usa
// em internal/handlers/solicitacao_matricula_handlers.go) bate com o da
// configuração recém-salva, e reprecifica cada uma — exceto as que já têm
// uma cobrança real aberta (matriculaTemCobrancaAberta), que nunca são
// tocadas: o valor de uma cobrança já gerada é imutável por construção.
//
// Cada solicitação é uma reprecificação independente (um evento próprio no
// stream daquela SolicitacaoMatricula) — uma falha isolada (ex.: a
// solicitação mudou de estado entre a consulta e o Load, uma corrida
// legítima e esperada com o próprio aplicante pagando nesse instante) é
// contada em Falhas/Ignoradas e NÃO aborta as demais nem falha
// ConfigureMatricula como um todo: a configuração do novo preço já foi
// salva com sucesso antes desta função ser chamada.
func (s *Service) reprecificarSolicitacoesMatriculaPendentes(ctx context.Context, academia, nivel, ano string, curso *string, novoValor float64, novosMetodos []string, actorID, actorType, ip string) MatriculaRepricingResumo {
	resumo := MatriculaRepricingResumo{}
	var rows *sql.Rows
	var err error
	switch nivel {
	case NivelFundamental:
		rows, err = s.client.DB().QueryContext(ctx, `SELECT id, codigo_solicitacao FROM projection_solicitacoes_matricula WHERE codigo_academia=$1 AND status='aprovada_pendente_pagamento_matricula' AND ano_escolar_fundamental=$2`, academia, ano)
	case NivelMedio:
		rows, err = s.client.DB().QueryContext(ctx, `SELECT id, codigo_solicitacao FROM projection_solicitacoes_matricula WHERE codigo_academia=$1 AND status='aprovada_pendente_pagamento_matricula' AND ano_escolar_medio=$2 AND curso_medio_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid`, academia, ano, optionalString(curso))
	case NivelSuperior:
		rows, err = s.client.DB().QueryContext(ctx, `SELECT id, codigo_solicitacao FROM projection_solicitacoes_matricula WHERE codigo_academia=$1 AND status='aprovada_pendente_pagamento_matricula' AND ano_superior=$2 AND curso_superior_id IS NOT DISTINCT FROM NULLIF($3,'')::uuid`, academia, ano, optionalString(curso))
	default:
		return resumo
	}
	if err != nil {
		resumo.Falhas++
		return resumo
	}
	type alvo struct {
		id     uuid.UUID
		codigo string
	}
	var alvos []alvo
	for rows.Next() {
		var a alvo
		if rows.Scan(&a.id, &a.codigo) == nil {
			alvos = append(alvos, a)
		}
	}
	rows.Close()
	for _, a := range alvos {
		aberto, err := s.matriculaTemCobrancaAberta(ctx, a.codigo)
		if err != nil || aberto {
			resumo.Ignoradas++
			continue
		}
		loaded, err := s.repository.WithContext(ctx).Load(a.id, "SolicitacaoMatricula")
		if err != nil {
			resumo.Falhas++
			continue
		}
		agg, ok := loaded.(*aggregates.SolicitacaoMatricula)
		if !ok {
			resumo.Falhas++
			continue
		}
		if err := agg.AtualizarValorPendentePagamentoMatricula(novoValor, novosMetodos); err != nil {
			resumo.Ignoradas++
			continue
		}
		if err := s.repository.WithContext(ctx).SaveWithAudit(agg, db.AuditContext{UserID: actorID, UserType: actorType, IP: ip}); err != nil {
			resumo.Falhas++
			continue
		}
		resumo.Atualizadas++
	}
	return resumo
}
```

Adicione `"spuri/internal/db"` ao import de `matricula.go` se ainda não estiver presente (é necessário para `db.AuditContext`).

Em `ConfigureMatricula`, depois de gravar o evento `MatriculaConfigurada` com sucesso e antes do `return`, chame a função acima quando aplicável:

```go
func (s *Service) ConfigureMatricula(ctx context.Context, in MatriculaConfiguracaoInput, actorID, actorType, ip string) (MatriculaConfiguracaoView, error) {
	if err := s.validateConfiguracaoMatricula(ctx, &in); err != nil {
		return MatriculaConfiguracaoView{}, err
	}
	in.Valor = roundAmount(in.Valor)
	payload := map[string]any{"codigo_academia": in.CodigoAcademia, "nivel": in.Nivel, "ano_academico": in.AnoAcademico, "curso_id": optionalString(in.CursoID), "valor": in.Valor, "metodos_pagamento": in.MetodosPagamento, "modo_vigencia": in.ModoVigencia}
	if err := s.recordMensalidade(ctx, in.CodigoAcademia, aggregates.MatriculaConfigurada, payload, actorID, actorType, ip); err != nil {
		return MatriculaConfiguracaoView{}, err
	}
	out, err := s.ResolveMatriculaConfiguracao(ctx, in.CodigoAcademia, in.Nivel, in.AnoAcademico, in.CursoID)
	if err != nil {
		return MatriculaConfiguracaoView{}, err
	}
	out.ModoVigencia = in.ModoVigencia
	if in.ModoVigencia == ModoVigenciaCobrancasPendentes {
		resumo := s.reprecificarSolicitacoesMatriculaPendentes(ctx, in.CodigoAcademia, in.Nivel, in.AnoAcademico, in.CursoID, out.Valor, out.MetodosPagamento, actorID, actorType, ip)
		out.Repricing = &resumo
	}
	return out, nil
}
```

## 3.5 `RemoveMatriculaConfiguracao` — nenhuma mudança

Não precisa de nenhum ajuste: remover a configuração vigente nunca reprecifica nada (isso só acontece explicitamente via `modo_vigencia=cobrancas_pendentes` em `ConfigureMatricula`).

---

# 4. Testes obrigatórios

Localize os testes de mensalidade em `internal/finance/mensalidade_integration_test.go`/`mensalidade_pendencias_integration_test.go`/`mensalidade_test.go`, e crie um novo `internal/finance/matricula_configuracao_vigencia_integration_test.go` para os de matrícula (o arquivo `matricula_remocao_integration_test.go` já existente mostra o padrão de setup a seguir).

**Mensalidade:**
1. `modo_vigencia` ausente/inválido em `ConfigureMensalidade` → erro de validação (com e sem configuração prévia no escopo).
2. `cobrancas_pendentes` **alcança um mês 2+ meses atrasado, nunca cobrado**: configure preço A; avance (sem cobrar) um ou mais meses; configure preço B com `cobrancas_pendentes`; confirme via `ListMensalidades`/`resolveConfiguracaoEfetiva` que o mês atrasado agora resolve em B, não em A. Este é o teste central desta tarefa — sem ele, a regressão para a v1 (só mês corrente em diante) passaria despercebida.
3. `a_partir_da_atualizacao`: mesmo cenário do item 2, mas confirme que o mês atrasado **continua** resolvendo em A (comportamento preservado).
4. Money-safety: gere uma cobrança real `aguardando_pagamento` para um mês com preço A; configure `cobrancas_pendentes` com preço B; confirme que `financeiro_cobrancas.payload->>'amount'` da cobrança já criada **não muda**.
5. Um mês já **pago** ou **anulado** nunca é afetado por `cobrancas_pendentes` — confirme que `ListMensalidades` continua devolvendo o preço histórico correto (via `resolveConfiguracao` normal) para esses meses mesmo depois de uma reconfiguração `cobrancas_pendentes`.
6. Remoção: configure `cobrancas_pendentes`, depois `RemoveMensalidadeConfiguracao`; confirme que meses pendentes voltam a `ErrNotFound` (sem preço ativo).
7. `PendenciasSemCobranca` com múltiplos estudantes: confirme que `cobrancas_pendentes` reprecifica corretamente vários estudantes com meses atrasados diferentes, numa única chamada, sem regressão de performance perceptível (não precisa medir tempo, só confirmar corretude com um cenário de vários vínculos, exercitando o `cfgCache`).
8. Compatibilidade de replay: grave manualmente um evento `MensalidadeConfigurada` sem `modo_vigencia`/`sequencia` no payload simulando um evento pré-existente à tarefa; rode o rebuild e confirme que a linha projetada recebe `modo_vigencia='a_partir_da_atualizacao'`.

**Matrícula:**
9. `modo_vigencia` ausente/inválido em `ConfigureMatricula` → erro de validação.
10. `cobrancas_pendentes` reprecifica uma solicitação já aprovada e pendente de pagamento (sem cobrança aberta): confirme `valor_matricula`/`metodos_pagamento_matricula` atualizados em `projection_solicitacoes_matricula`, e que `IniciarPagamentoMatricula` chamado em seguida usa o **novo** valor.
11. `cobrancas_pendentes` **não** reprecifica uma solicitação que já tem cobrança `aguardando_pagamento` — confirme que `valor_matricula` permanece o antigo, e que a cobrança já criada mantém seu `amount` original.
12. `a_partir_da_atualizacao`: mesmo cenário do item 10, mas confirme que `valor_matricula` da solicitação já aprovada **não muda** (comportamento preservado — só novas aprovações usam o novo preço).
13. `ConfigureMatricula` com `cobrancas_pendentes` devolve `repricing_pendentes` com a contagem correta (`atualizadas`/`ignoradas`) para um cenário com 2 solicitações pendentes (uma sem cobrança aberta, outra com).
14. O reprecificamento respeita escopo: uma solicitação de um `ano_academico`/`curso_id` diferente do configurado **não** é tocada.

---

# 5. `Documentação da API.md`

Nas seções de `POST`/`PUT /financeiro/mensalidades/configuracoes` e `POST`/`PUT /financeiro/matriculas/configuracoes` (ou nome equivalente — confirme o path exato buscando por `ConfigureMatricula` em `cmd/server/main.go`), adicione `modo_vigencia` (obrigatório, `"cobrancas_pendentes"` | `"a_partir_da_atualizacao"`) à tabela de campos do request, com a mesma redação do Objetivo desta tarefa, e no response de matrícula documente o campo adicional opcional `repricing_pendentes` (só presente quando `modo_vigencia=cobrancas_pendentes`).

# Fora de escopo

- Qualquer outro tipo de cobrança fora de mensalidade e taxa de matrícula (não existe nenhum outro no sistema hoje).
- Mudar `RemoveMensalidadeConfiguracao`/`RemoveMatriculaConfiguracao` — nenhuma delas precisa de ajuste.
- Notificar o aplicante/estudante quando uma solicitação de matrícula é reprecificada (ex.: e-mail/SMS avisando do novo valor) — se o negócio quiser isso, é uma tarefa própria de comunicação.
- Job assíncrono/fila para o reprecificamento de matrícula em lote — roda de forma síncrona dentro da própria chamada de `ConfigureMatricula`, como descrito na Seção 3.4. Se no futuro o volume de solicitações pendentes por escopo crescer muito, mover para um job assíncrono é uma otimização própria, separada.

# Critérios de aceite

1. `modo_vigencia` obrigatório e validado em `ConfigureMensalidade` e `ConfigureMatricula`, sem exceção.
2. `cobrancas_pendentes` de mensalidade alcança qualquer mês pendente, **inclusive atrasados de vários meses** — provado pelo teste 2 da Seção 4.
3. `cobrancas_pendentes` de matrícula reprecifica toda solicitação aprovada e pendente de pagamento sem cobrança aberta, e nunca toca uma que já tem cobrança aberta — provado pelos testes 10 e 11.
4. `a_partir_da_atualizacao` preserva exatamente o comportamento anterior a esta tarefa, para os dois recursos.
5. Nenhuma cobrança real já gerada (mensalidade ou matrícula) tem seu valor alterado por uma reconfiguração posterior, em nenhum dos dois modos — provado pelos testes 4 e 11.
6. Migração da Seção 1 aplicada, com `sequencia` corretamente preenchida por backfill para toda versão pré-existente.
7. Replay/rebuild do ledger permanece determinístico para eventos gravados antes desta tarefa — provado pelo teste 8.
8. Todos os 14 testes da Seção 4 existem e passam.
9. `Documentação da API.md` atualizada.
10. `go build ./...` e `go test ./internal/finance/... ./internal/projections/... ./internal/domain/aggregates/...` (com `RUN_POSTGRES_INTEGRATION=1`) passam sem regressão.

## Procedimento de conclusão

Ao finalizar: atualize o título e o front matter deste documento para `status: feito`, e mova-o para `docs/Tarefas feitas/`. Apague `docs/Lista de Tarefas/09 - Exigir escolha de vigência (cobranças pendentes vs mês específico) ao alterar preço da mensalidade.md` se ele ainda existir no repositório — ele foi substituído integralmente por este documento.
