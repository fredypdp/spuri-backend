---
data: 2026-08-23
status: corrigido_via_62_corrigir_n1_pendencias_sem_cobranca
auditor: Claude (orquestrador) — depuração profunda com PostgreSQL 16 e Go 1.24 reais em sandbox
tarefa_correcao: docs/Tarefas feitas/62 - Corrigir N+1 de PendenciasSemCobranca em GET financeiro-cobrancas com ano_letivo.md
---

# Depuração profunda — `GET /financeiro/cobrancas` trava por vários minutos quando `ano_letivo` é informado

## Resumo executivo

| Item | Conclusão |
|---|---|
| Sintoma relatado | `GET /financeiro/cobrancas?...&ano_letivo=2020_2021&mes=9...` demora vários minutos e não retorna nada, em **qualquer** valor de `estado` (pago, falhado, pendente, cancelado, todos) |
| Causa raiz | `PendenciasSemCobranca` (chamada sempre que `ano_letivo` — ou `turma_id`/`curso_id`/`ano_academico` — é informado, **independente do `estado`**) chama `ListMensalidades` **uma vez por estudante do escopo**, e cada chamada dispara **~37 consultas SQL sequenciais** |
| Por que só aparece com `ano_letivo` | `ano_letivo` sozinho casa com **todos os estudantes da academia inteira** naquele ano (todas as turmas), não com uma turma — ao contrário do que o comentário original do código presumia |
| Evidência quantitativa | Medido com PostgreSQL 16 real: **37 queries confirmadas por estudante** (log real do Postgres). Tempo cresce **linearmente**: 65ms (10 estudantes) → 325ms (50) → 1,05s (150) → 1,96s (300) — **em localhost**, sem latência de rede real de produção |
| Por que trava "vários minutos" em produção | Em produção o banco não é local: cada uma das ~37×N queries paga uma latência de rede real. Com centenas de estudantes no `ano_letivo`, isso multiplica milhares de idas ao banco em série numa única requisição HTTP |
| Correção aplicada e validada | Eliminado o N+1: os vínculos já resolvidos por `escopoMensalidadeEstudantes` passam a ser usados diretamente (sem re-consultar por estudante), `mesInicioEfetivo`/`resolveConfiguracao` são memoizados por combinação distinta (não mais por estudante), e o estado da obrigação (`estadoObrigacao`) passa a ser buscado em **lote**, numa única consulta para todos os estudantes |
| Resultado medido após a correção | **37 queries/estudante → 6 queries totais, independente de N** (quando `mes` é informado, como o frontend sempre faz). Tempo com 300 estudantes: 1,96s → **11,2ms** (~175× mais rápido), curva deixou de ser linear |
| Frontend (`spuripainel`) | **Nenhuma alteração necessária.** O padrão de chamada (`ano_letivo` + `mes` sempre juntos) já está correto por design — é exatamente o que o comentário do próprio backend já documentava como premissa. O bug é 100% de implementação no backend |
| Validação | `go build`, `go vet`, `gofmt`, e suíte completa de testes de integração (`go test ./internal/finance/...`) rodados com PostgreSQL 16 real neste sandbox — 100% verde nos testes relacionados a este módulo (as únicas falhas da suíte são 9 testes pré-existentes, não relacionados, que já falham no código original por falta de `FINANCE_ENCRYPTION_KEY` no ambiente — confirmado comparando a mesma bateria de testes antes e depois da correção) |
| Ação para o Codex | Aplicar mecanicamente os diffs da tarefa de correção vinculada acima. Codex **não precisa de PostgreSQL/Docker/psql** — tudo já foi validado com banco real por Claude; a validação do Codex é só `go build`/`go vet`/`gofmt`/`go test` (que já skipam os testes de integração sem banco disponível) |

---

## 1. Como o problema foi encontrado

1. Clonagem real de `fredypdp/spuri-backend` (branch `main`) e `fredypdp/spuripainel` (branch `main`).
2. Localização da rota a partir do log fornecido: `GET /financeiro/cobrancas` → `internal/handlers/financeiro_handlers.go`, função `ListarCobrancasAppyPay`.
3. Leitura completa do handler: identificado que, sempre que `turma_id`, `curso_id`, `ano_academico` **ou** `ano_letivo` é informado, o handler chama adicionalmente `FinanceiroService.PendenciasSemCobranca` — **isso acontece independentemente do valor de `estado`**, o que já explica por que o sintoma relatado ("acontece em todos os estados do select") não tem relação com o filtro de estado em si.
4. Leitura de `PendenciasSemCobranca` (`internal/finance/mensalidade_pendencias.go`): o próprio comentário do código já admitia o risco, mas presumia que o escopo seria sempre pequeno:

   > "o escopo obrigatório garante que o número de estudantes é sempre delimitado (uma turma, um ano acadêmico, um curso ou um ano letivo, nunca a academia inteira sem filtro), então o custo de N chamadas sequenciais é aceitável nesse volume"

   Essa premissa **não se sustenta para `ano_letivo` sozinho**: `escopoMensalidadeEstudantes` (a consulta que resolve o escopo) casa `ano_letivo` contra **todas as turmas da academia** que tiveram esse ano em `historico_estudantes_ano_letivo` ou `projection_academias.ano_letivo` — ou seja, o "escopo" de um `ano_letivo` é, na prática, a academia inteira naquele ano.
5. Leitura de `ListMensalidades` (`internal/finance/mensalidade.go`): confirmado que, por vínculo (turma/ano), a função chama `vinculosMensalidade` (1 query) + `mesInicioEfetivo` (1 query) + para cada um dos ~11 meses do ano letivo: `resolveConfiguracao` (1-2 queries) + `estadoObrigacao` (1 query).
6. Confirmação de que `mesInicioEfetivo` e `resolveConfiguracao` dependem **apenas** de `(academia, ano_letivo, nivel)` e `(academia, nivel, ano_academico, curso_id, mês)` respectivamente — **nunca do estudante** — tornando-os candidatos óbvios para memoização em vez de recomputação por estudante.
7. Instalação real de **PostgreSQL 16.15** e **Go 1.24.4** neste sandbox (via `apt-get`, que funciona aqui — diferente do ambiente do Codex, que bloqueia `apt`). Aplicadas as 116 migrations do repositório sem erro.
8. Reprodução empírica com dados sintéticos reais (múltiplas turmas, cada uma com um estudante distinto, todas no mesmo `ano_letivo=2020_2021`, replicando o cenário exato do request relatado: `ano_letivo=2020_2021&mes=9`).

---

## 2. Evidência quantitativa (medida, não estimada)

### 2.1 Contagem exata de queries por estudante (log real do PostgreSQL, `log_statement=all`)

Isolando uma única chamada de `PendenciasSemCobranca` para **1 estudante**, com marcadores SQL delimitando o início/fim no log do Postgres:

**Antes da correção: 37 comandos SQL** para processar 1 único estudante em 1 ano letivo (11 meses, fundamental):
- 1× `escopoMensalidadeEstudantes` (resolve o escopo — só roda 1 vez por requisição)
- 1× `cobrancasExistentesMensalidade`
- 1× `vinculosMensalidade` (por estudante)
- 1× `mesInicioEfetivo` (por estudante)
- 11× `resolveConfiguracao` (1 query de config + 1 query de `removido_em` = 2 por mês) = 22
- 11× `estadoObrigacao` (1 por mês)

Total: 1+1+1+1+22+11 = **37**, exatamente como contado no log real (`grep -cE "LOG: (statement|execute)"` na janela delimitada pelos marcadores).

### 2.2 Escala medida (PostgreSQL local, sem latência de rede) — antes da correção

| Estudantes no escopo (`ano_letivo` sozinho) | `escopoMensalidadeEstudantes` (query de escopo) | `PendenciasSemCobranca` completo | Média por estudante |
|---|---|---|---|
| 10 | 1,6ms | **64,6ms** | 6,46ms |
| 50 | 1,5ms | **325ms** | 6,50ms |
| 150 | 2,9ms | **1,045s** | 6,97ms |
| 300 | 3,9ms | **1,96s** | 6,53ms |

A consulta de escopo (`escopoMensalidadeEstudantes`) é rápida e **não** é o gargalo — ela roda em poucos milissegundos mesmo com 300 estudantes. O gargalo é inteiramente o laço `for _, estudante := range estudantes { ListMensalidades(...) }`, que cresce **linearmente** (~6,5ms/estudante) mesmo em localhost.

### 2.3 Por que isso vira "vários minutos" em produção

Os números acima são de um Postgres **na mesma máquina** (latência de rede desprezível, <0,3ms por round-trip). Em produção, o banco não está na mesma máquina do backend — cada uma das ~37×N idas ao banco paga a latência de rede real (mesmo que pequena, ex. 15-50ms, típica de um Postgres gerenciado remoto). Com N na casa de algumas centenas de estudantes (realista para `ano_letivo` sozinho, que cobre a academia inteira naquele ano):

- 300 estudantes × 37 queries = 11.100 idas ao banco em série, numa única requisição HTTP.
- Mesmo a 20ms de latência média por query: 11.100 × 20ms ≈ **222 segundos** (~3,7 minutos).
- A 50ms: 11.100 × 50ms ≈ **555 segundos** (~9,25 minutos).

Isso está em linha direta com o sintoma relatado ("demora muito... não retorna nada depois de muitos minutos") e explica também por que os logs mostram uma **segunda** requisição idêntica sendo iniciada 51 segundos depois da primeira, sem que a primeira jamais apareça como finalizada nos logs — a primeira requisição continuava presa no laço N+1 quando a segunda começou (o mais provável é o usuário ou a página tentando de novo após a demora, não um retry automático do frontend — não foi encontrado nenhum mecanismo de timeout/retry automático em `spuripainel/src/lib/api/services.ts`).

### 2.4 Escala medida — depois da correção

| Estudantes no escopo | `PendenciasSemCobranca` completo (depois) | Antes | Ganho |
|---|---|---|---|
| 10 | 4,3ms | 64,6ms | 15× |
| 50 | 5,7ms | 325ms | 57× |
| 150 | 6,8ms | 1,045s | 154× |
| 300 | 11,3ms | 1,96s | **173×** |

Contagem exata de queries para 1 estudante (mesmo teste de log real): **6 comandos SQL** (contra 37 antes) — e esses 6 **não crescem com o número de estudantes** quando todos compartilham a mesma combinação de turma/nível/curso (o caso comum), porque `mesInicioEfetivo` e `resolveConfiguracao` passam a ser memoizados por combinação, não por estudante, e `estadoObrigacao` passa a ser uma única consulta em lote.

---

## 3. Causa raiz em detalhe

### 3.1 O que `PendenciasSemCobranca` fazia (antes)

```go
for _, estudante := range estudantes {
    meses, err := s.ListMensalidades(ctx, estudante, &academia) // ~37 queries POR estudante
    ...
}
```

`ListMensalidades` foi desenhada para o caso de **um único estudante** (usada por `GET /financeiro/mensalidades/estudante/:codigo`), e é reaproveitada aqui, uma vez por estudante do escopo. Isso é correto quando o escopo é de fato pequeno (uma turma — 20 a 40 estudantes), mas `ano_letivo` sozinho não é um escopo pequeno: ele casa com todas as turmas da academia que tiveram aquele ano.

### 3.2 O que estava sendo desperdiçado

Dentro do laço de `ListMensalidades`, por vínculo (turma/ano) do estudante:

- `mesInicioEfetivo(academia, anoLetivo, nivel)` — **não depende do estudante**, só de `(academia, ano_letivo, nivel)`. Estudantes da mesma turma/nível recebem exatamente a mesma resposta, mas a consulta era refeita para cada um.
- `resolveConfiguracao(academia, nivel, anoAcademico, cursoID, mês)` — **não depende do estudante**, só de `(academia, nivel, ano_academico, curso_id, mês)`. Mesmo raciocínio: estudantes da mesma turma recebem a mesma configuração de preço/mês-fim, mas a consulta (na verdade 2 consultas: preço + verificação de remoção) era refeita para cada um, para cada mês.
- `vinculosMensalidade(estudante, academia)` — re-consultava exatamente o que `escopoMensalidadeEstudantes` **já tinha acabado de resolver** para aquele mesmo estudante, na mesma requisição.
- Só `estadoObrigacao(estudante, academia, anoLetivo, mes)` genuinamente depende do estudante (são os eventos de pagamento/anulação daquele estudante específico) — mas mesmo esse podia ser resolvido com **uma única consulta** para todos os estudantes de uma vez, em vez de uma consulta por (estudante, mês).

### 3.3 Achado secundário (não corrigido nesta tarefa — ver seção 5)

`ListCobrancas` (chamada pelo mesmo handler, antes de `PendenciasSemCobranca`) também chama `escopoMensalidadeEstudantes` internamente (via `chargeIDsEscopoMensalidade`), sempre que os mesmos quatro filtros são informados. Isso significa que, na mesma requisição HTTP, `escopoMensalidadeEstudantes` roda **duas vezes** (uma vez dentro de `ListCobrancas`, outra dentro de `PendenciasSemCobranca`). Medido: essa consulta custa poucos milissegundos mesmo em escala (seção 2.2), então **não é a causa da lentidão relatada** — é uma redundância pequena, documentada aqui para transparência, mas propositalmente fora do escopo da correção desta tarefa para não aumentar a superfície de mudança além do necessário. Ver seção 5 para detalhes de por que foi deixada de fora.

---

## 4. Correção aplicada (já implementada e validada com PostgreSQL real)

**Arquivos alterados:**

1. `internal/finance/mensalidade_pendencias.go` — reescrita de `PendenciasSemCobranca`. Nenhuma outra função deste arquivo foi alterada.
2. `internal/finance/mensalidade_pendencias_batch.go` — **novo arquivo**, contendo apenas `estadosObrigacaoBatch` (a versão em lote de `estadoObrigacao`), usada exclusivamente pela função acima.
3. `internal/finance/mensalidade_pendencias_integration_test.go` — adição de um teste novo (nenhum teste existente foi alterado ou removido).

**Funções que permanecem 100% inalteradas** (usadas por outros caminhos, como `GET /financeiro/mensalidades/estudante/:codigo`, e continuam com o comportamento de sempre): `ListMensalidades`, `vinculosMensalidade`, `mesInicioEfetivo`, `resolveConfiguracao`, `estadoObrigacao`, `precedenciaEstado`, `escopoMensalidadeEstudantes`, `chargeIDsEscopoMensalidade`, `cobrancasExistentesMensalidade`, `PendenciasSemCobrancaEstudante`, `ListCobrancas`.

### 4.1 Estratégia (por que esta abordagem, e não outra)

Em vez de reescrever a lógica de negócio (o que arriscaria divergir sutilmente do comportamento já testado), a correção:

1. **Reaproveita os vínculos que `escopoMensalidadeEstudantes` já resolveu** — em vez de `PendenciasSemCobranca` jogar essa informação fora e pedir de novo, por estudante, via `ListMensalidades`/`vinculosMensalidade`.
2. **Memoiza, dentro de uma única chamada**, os resultados de `mesInicioEfetivo` e `resolveConfiguracao` — chamando as **mesmas funções, sem nenhuma alteração nelas**, mas guardando o resultado por combinação (`academia|ano_letivo|nivel` e `academia|nivel|ano_academico|curso|mês`) para não repetir a mesma consulta para estudantes que compartilham a mesma turma/configuração.
3. **Substitui o laço `estadoObrigacao` por `estadosObrigacaoBatch`** — uma única consulta SQL que busca os eventos de obrigação de **todos** os estudantes do escopo de uma vez (`WHERE codigo_estudante = ANY($lista)`), agrupando em memória e aplicando `precedenciaEstado` (a mesma função, inalterada) por grupo.

Isso elimina o N+1 sem duplicar nenhuma regra de negócio: a única lógica nova é I/O em lote/memoização; a decisão de "o que é pendente", "qual o valor", "qual o mês final de cobrança" continua vindo exatamente das mesmas funções testadas de sempre.

### 4.2 Caso de borda identificado e corrigido: estudante em 2 turmas no mesmo ano

`escopoMensalidadeEstudantes` deduplica por `(turma_id, codigo_academia, ano_letivo, nivel, ano_academico, curso_id, codigo_estudante)` — **incluindo `turma_id`** — enquanto `vinculosMensalidade` (usada indiretamente pela versão antiga, via `ListMensalidades`) deduplica **sem** `turma_id`. Isso significa que um estudante que apareça em duas turmas diferentes para a mesma combinação de ano/nível/curso (ex.: transferência de turma no meio do ano letivo histórico) gera **duas linhas distintas** em `escopoMensalidadeEstudantes`.

Se a correção simplesmente iterasse essas linhas cruas, o mesmo mês pendente apareceria **duplicado** para esse estudante — uma regressão que o código original não tinha (porque operava por identidade do estudante, não por vínculo). Isso foi identificado por leitura cuidadosa do código (não só pelos testes existentes, que não cobriam este cenário) e corrigido deduplicando os vínculos, antes de processá-los, pela mesma chave que `vinculosMensalidade` já usa (sem `turma_id`). Um teste de regressão permanente foi adicionado (`TestIntegrationPendenciasSemCobrancaNaoDuplicaEstudanteEmDuasTurmasMesmoAno`) e confirmado, com PostgreSQL real, que:
- o código **original** já não duplicava neste cenário (correto, por design diferente);
- a **nova** implementação também não duplica (correto, graças à deduplicação adicionada).

### 4.3 Validação executada (com PostgreSQL 16 e Go 1.24 reais)

- `go build ./...` — sem erros.
- `go vet ./...` — sem erros.
- `gofmt -l internal/finance/mensalidade_pendencias.go internal/finance/mensalidade_pendencias_batch.go internal/finance/mensalidade_pendencias_integration_test.go` — vazio (sem divergência de formatação).
- `go test ./internal/finance/...` (com `RUN_POSTGRES_INTEGRATION=1` contra PostgreSQL 16 real, todas as 116 migrations aplicadas): **todos os testes relacionados a mensalidade/pendências/cobranças passam**, incluindo os 6 testes já existentes de `PendenciasSemCobranca`/`ListCobrancas` (nenhum foi alterado) e o teste novo de deduplicação.
- As **9 falhas** que aparecem na mesma rodada (`TestIntegrationRemoveCredentialRespeitaEventSourcing`, `TestIntegrationConfigureMensalidadeGravaNoLedgerEProjectaCorretamente`, `TestIntegrationConfigureMatriculaGravaNoLedgerEProjectaCorretamente`, `TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago`, `TestIntegrationRebuildFinanceiroReconstroiConfiguracoesEcobrancasMensalidade`, `TestIntegrationRemoveMatriculaConfiguracaoFluxoDeComando`, `TestIntegrationRemoveMensalidadeConfiguracaoFluxoDeComando`, `TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr`, `TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr`) são **pré-existentes e não relacionadas** a esta correção — confirmado rodando exatamente a mesma bateria contra o código **original** (sem a correção aplicada): as mesmas 9 falham da mesma forma, todas pela mesma causa (`FINANCE_ENCRYPTION_KEY é obrigatória`, uma variável de ambiente de criptografia não configurada neste sandbox de validação, sem relação com mensalidade/pendências).
- Reproduzido o cenário de escala (seção 2.4) com dados sintéticos reais em PostgreSQL, confirmando o ganho de performance medido.

### 4.4 `go.mod`/`go.sum`

**Não foram alterados.** Para compilar e testar neste sandbox (que, como o ambiente do Codex, não acessa `proxy.golang.org`), foram usados `replace` locais temporários em `go.mod` (apontando `golang.org/x/*`, `google.golang.org/protobuf`, `gopkg.in/yaml.v3`, `gopkg.in/gomail.v2` e `gopkg.in/alexcesaro/quotedprintable.v3` para os mirrors correspondentes no GitHub, com as versões/pseudo-versões corretas) — mas essas alterações foram **revertidas ao final** e nunca fazem parte do código entregue. `git diff go.mod go.sum` contra o `main` real está vazio.

---

## 5. Por que a redundância de `escopoMensalidadeEstudantes` (achado secundário, seção 3.3) não foi corrigida nesta tarefa

Corrigi-la exigiria mudar a forma como `ListCobrancas` e `PendenciasSemCobranca` são chamadas a partir do handler (ex.: resolver o escopo uma única vez no handler e passá-lo para as duas funções, o que muda a assinatura de `chargeIDsEscopoMensalidade` e/ou de `PendenciasSemCobranca`, hoje usadas também por outros pontos). O ganho medido (poucos milissegundos por requisição, seção 2.2) é desprezível perto do ganho da correção principal (segundos → milissegundos), e o risco de regressão em superfícies compartilhadas não se justifica para um ganho tão pequeno. Fica registrado aqui como observação para uma tarefa futura e isolada, caso a Spuri queira eliminar também essa redundância — não é necessário para resolver o sintoma relatado.

---

## 6. O que o Codex precisa fazer

Tudo já está implementado, testado e validado com PostgreSQL real por Claude. A tarefa do Codex é puramente mecânica: aplicar os diffs exatos documentados em `docs/Lista de Tarefas/Corrigir N+1 de PendenciasSemCobranca em GET financeiro-cobrancas com ano_letivo.md` e rodar as validações que não dependem de banco de dados (`go build`, `go vet`, `gofmt`, `go test ./...` — que já skipam automaticamente os testes de integração quando `RUN_POSTGRES_INTEGRATION` não está definida, sem falhar por isso). Nenhuma decisão de design fica em aberto para o Codex.
