---
criado: 2026-08-03 00:00
origem: docs/Debbugs/Depuração — Módulo base de gestão financeira com AppyPay 1.md; docs/Debbugs/Depuração — Verificação das correções do módulo financeiro AppyPay 1.md; docs/Debbugs/Depuração — Verificação das correções 'AppyPay 1'.md
status: feito
---

# Remover por completo o módulo financeiro/pagamento AppyPay (rollback total)

## Prompt recomendado para executar a atualização

Execute este documento **passo a passo, na ordem em que está escrito, sem saltar secções**. Não tente reimplementar, corrigir ou "salvar" nenhuma parte do módulo financeiro — o objetivo é remoção total, não correção. Sempre que uma secção indicar um comando de terminal, execute-o literalmente. Sempre que indicar um bloco de código a remover, procure esse bloco exato no ficheiro indicado e remova-o (não remova mais nem menos do que o indicado, exceto quando a própria secção disser explicitamente para generalizar). Sempre que aparecer um erro de compilação (`go build ./...`) numa linha não prevista neste documento, remova apenas a(s) linha(s) que causam o erro relacionadas com o módulo financeiro — nunca apague um ficheiro inteiro que não esteja explicitamente listado na Secção 5. No fim, execute a checklist de verificação da Secção 10 e só marque a tarefa como concluída se todos os itens passarem.

## Contexto

O módulo financeiro/pagamento (integração AppyPay, `internal/finance`, aggregate `Financeiro`, handlers `/financeiro/*`) foi especificado na tarefa `15 - Modulo base de gestao financeira com AppyPay.md` e depois refatorado para Event Sourcing/CQRS na tarefa `16 - Refatorar modulo financeiro para Event Sourcing CQRS completo.md`. Três auditorias de segurança sucessivas (`docs/Debbugs/Depuração — Módulo base de gestão financeira com AppyPay 1.md` e os dois relatórios de verificação que se seguiram) encontraram, em cada ronda, **novas falhas críticas ou de alta gravidade introduzidas pelas próprias correções da ronda anterior**:

- Chave de cifra (`FINANCE_ENCRYPTION_KEY`) com fallback previsível derivado de uma string fixa no código-fonte quando a variável de ambiente correta não está definida.
- Condição de corrida entre a libertação do mutex e a chamada ao provedor externo que permite duplicar uma cobrança com a mesma `referencia_externa` (quebra de idempotência financeira).
- Uma academia autenticada conseguia reatribuir `contexto_tipo`/`codigo_academia` da própria credencial para outro tenant através de `PUT /financeiro/appypay/credenciais/:id`, quebrando o isolamento por instituição.
- Escritor duplo (`Service` síncrono + `FinanceiroProjection` assíncrona) duplicava o histórico auditável (`Historico`) das credenciais, cobranças e modalidade.
- Um estudante autenticado conseguia listar, consultar e testar (com escrita de evento no ledger) credenciais financeiras de qualquer academia, antes de uma correção parcial.

Este padrão — três rondas de auditoria, cada uma a corrigir a ronda anterior e a introduzir um novo problema — indica que o desenho atual do módulo é frágil o suficiente para justificar um **rollback completo** em vez de mais uma ronda de patches. Adicionalmente, **nenhum provedor HTTP real da AppyPay chegou a ser implementado** (o único `Provider` em uso é `FakeProvider`, que nunca faz uma chamada de rede real). Isto significa que esta remoção **não tem nenhuma cobrança, credencial ou webhook real por reconciliar externamente** — é um rollback interno e de baixo risco operacional, mesmo sendo um domínio financeiro.

Esta tarefa não é sobre "corrigir" o módulo. É sobre removê-lo por completo — código, migrations (neutralizadas, não apagadas — ver Secção 6), documentação e rastos na whitelist de eventos — e devolver a tarefa `15` ao estado pendente para uma reimplementação futura mais robusta, que deverá ler os relatórios de auditoria antes de começar.

**Nota para quem vai executar esta tarefa:** este documento foi escrito para ser executado por um modelo de IA de capacidade limitada (o nível gratuito do Codex web da OpenAI), não pelo Claude. Por isso, o método de execução (Secção 4) evita qualquer passo que exija inferência ou julgamento aberto: usa o compilador Go como guia mecânico para encontrar todas as referências de código, e `grep` como guia mecânico para encontrar todas as referências fora do código compilado (SQL, Markdown). Siga os comandos literalmente.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Código Go do módulo financeiro | Remoção total (ficheiros, aggregate, projeção, handlers, rotas, entradas na whitelist de eventos) | `go build ./...` e `go test ./...` passam sem nenhuma referência a `finance`/`Financeiro`/`AppyPay` |
| Migrations `097`/`098` | **Mantidas** (nunca apagar migrations antigas — ver precedente na migration `046`) | Uma nova migration neutraliza (`DROP TABLE IF EXISTS`) as tabelas `financeiro_*`, preservando o histórico de schema |
| `Documentação.md` | Remover as secções "Módulo financeiro base com AppyPay" e "Módulo financeiro — Event Sourcing/CQRS" | Documentação volta a descrever apenas funcionalidades que existem no código |
| Tarefa 15 | Move-se de `docs/Tarefas feitas/` para `docs/Lista de Tarefas/`, `status: pendente`, sem sufixo `(feito)` no título | Fica disponível para reimplementação futura, com lições aprendidas anexadas |
| Tarefa 16 | Apagada (não volta a pendente) | Já não faz sentido documentar um refactor de Event Sourcing de um módulo que deixou de existir; a próxima implementação da tarefa 15 deve nascer já correta em Event Sourcing |
| Relatórios de auditoria em `docs/Debbugs/` | **Mantidos**, com nota de contexto adicionada no topo | Preservam o conhecimento de vulnerabilidades para a reimplementação futura |
| Índice de tarefas | Atualizado: remove a linha da tarefa 16, atualiza a linha da tarefa 15, adiciona linha desta tarefa (17) no nível Crítico | Reflete o estado real do repositório |
| Método de verificação | Compilador Go (`go build`) para código; `grep` recursivo para tudo o resto | Garantia mecânica de que não sobra nenhuma referência morta |

---

# 1. Objetivo

Remover por completo, do repositório, todo o suporte ao módulo financeiro/pagamento AppyPay — incluindo mas não limitado a: pacote `internal/finance`, aggregate `Financeiro`, projeção financeira, handlers HTTP, rotas registadas, entradas na whitelist de eventos/aggregates, testes específicos, e todas as referências em `Documentação.md` — sem deixar nenhum código morto, rota fantasma, entrada de whitelist inútil ou referência em documentação. Ao mesmo tempo, devolver a tarefa 15 ao ficheiro de tarefas pendentes e remover a tarefa 16, para que uma futura reimplementação parta de zero com as lições dos três relatórios de auditoria já incorporadas desde o início.

# 2. Diagnóstico — porque este rollback é necessário (não repetir na execução, apenas para contexto)

Não é preciso reavaliar esta decisão durante a execução — ela já foi tomada. Esta secção existe apenas para quem for reimplementar a tarefa 15 no futuro entender o porquê. As três rondas de auditoria (ver `origem` no cabeçalho deste ficheiro) mostraram um padrão recorrente: cada correção pontual resolvia o problema relatado mas abria um problema novo, porque a suite de testes atual nunca exercita simultaneamente concorrência real, o par `Service`+`FinanceiroProjection` contra Postgres real, nem o papel `estudante` contra todas as rotas financeiras ao mesmo tempo. Um rollback total elimina a superfície de risco imediatamente; uma reimplementação futura deve nascer com testes de concorrência (`-race`), testes de replay contra Postgres real, e testes de isolamento por papel (`estudante`, `academia` de outro tenant) desde o primeiro commit — não adicionados depois, ronda a ronda.

# 3. Princípio orientador: dois tipos de remoção diferentes

Este repositório trata **código Go** e **migrations SQL** de forma diferente, e esta tarefa respeita essa distinção:

- **Código Go (aggregates, handlers, projeções, whitelist, rotas, testes):** é removido por completo. Não há histórico a preservar em código morto — o padrão do repositório (ver `internal/db/safe_queries_test.go`, função `TestValidateEventTypeRejectsRemovedStudentProgressionEvents`) é remover entradas de whitelist de eventos descontinuados, não mantê-las "por precaução".
- **Migrations SQL:** nunca são apagadas depois de terem existido — são um registo histórico append-only do schema, exatamente como o `spuri_ledger` é um registo append-only de eventos. O precedente exato está em `migrations/046_remove_aprovacao_reprovacao.sql`, que remove as tabelas `projection_aprovacao_ano`/`projection_reprovacoes` **sem apagar** as migrations `003` e `009` que as criaram. Esta tarefa segue o mesmo padrão: as migrations `097` e `098` ficam no repositório tal como estão; uma nova migration remove as tabelas.

Se em algum ponto desta tarefa surgir dúvida entre apagar um ficheiro de migration ou apenas neutralizá-lo com uma nova migration, a resposta é sempre: **nunca apagar uma migration já existente; criar uma nova.**

# 4. Método de execução recomendado (para executores com capacidade limitada)

Siga esta ordem exata. Não avance para o passo seguinte sem confirmar que o passo atual terminou sem erros inesperados.

1. **Remover os ficheiros Go listados na Secção 5.1** (apagar os ficheiros inteiros — são exclusivamente do módulo financeiro).
2. **Executar `go build ./... 2>&1`** e ler a lista de erros. Cada erro vai apontar exatamente um ficheiro e uma linha que ainda referencia algo que foi apagado. **Nunca apague um ficheiro inteiro nesta fase** a não ser que esteja listado na Secção 5.1 — em vez disso, abra o ficheiro apontado pelo erro, encontre a(s) linha(s) relacionadas com `finance`/`Financeiro`/`AppyPay`, e remova apenas essas linhas (a Secção 5.2 já lista os locais exatos esperados: `cmd/server/main.go` e `internal/domain/aggregates/aggregate.go`).
3. **Repetir `go build ./...`** até não haver nenhum erro. Isto garante mecanicamente que já não existe nenhuma referência de código a símbolos do módulo financeiro que tenham sido apagados.
4. **Remover as entradas mortas na whitelist** (Secção 5.3) — estas **não** causam erro de compilação (são apenas entradas de mapa), por isso têm de ser removidas manualmente seguindo a lista exata fornecida.
5. **Atualizar/remover os testes** indicados na Secção 9.
6. **Executar `go test ./... 2>&1`** e confirmar que passa. Se algum teste não listado nesta tarefa falhar por causa da remoção, corrija apenas a parte relacionada com finance/AppyPay desse teste; não altere lógica não relacionada.
7. **Migrations** — Secção 6.
8. **Documentação** — Secção 7.
9. **Tarefas 15/16 e índice** — Secção 8.
10. **Verificação final por `grep`** — Secção 10. Só depois de todos os itens da checklist passarem é que a tarefa está concluída.

# 5. Escopo obrigatório — código Go

## 5.1 Ficheiros a apagar por completo

Apague estes ficheiros inteiros (e o diretório `internal/finance/` fica vazio e também deve ser removido):

```
internal/finance/financeiro.go
internal/finance/financeiro_test.go
internal/domain/aggregates/financeiro.go
internal/projections/financeiro_projection.go
internal/handlers/financeiro_handlers.go
```

Comando sugerido:

```bash
rm -rf internal/finance
rm -f internal/domain/aggregates/financeiro.go
rm -f internal/projections/financeiro_projection.go
rm -f internal/handlers/financeiro_handlers.go
```

## 5.2 Ficheiros a editar (remover apenas os blocos indicados)

### `cmd/server/main.go`

Remover a linha de import:

```go
	"spuri/internal/finance"
```

Remover, dentro de `initDB()`, o bloco:

```go
	if err := finance.ValidateEncryptionConfig(); err != nil {
		return err
	}
```

e a linha:

```go
	handlers.FinanceiroService = finance.NewServiceWithClient(dbClient, nil)
```

Remover, dentro de `initProjections()`, a linha:

```go
	projManager.RegisterProjection("financeiro", projections.NewFinanceiroProjection(dbClient))
```

Remover, dentro de `setupRouter()`, todo o bloco (desde a declaração do grupo até à última rota):

```go
		financeiro := protected.Group("/financeiro")
		financeiro.Use(middleware.RequireAcademiaOuAdmin())
		// FIX FIN-RBAC-01: PopulateAdminRole resolve o role granular (fpp/adm/gerente)
		// do admin autenticado sem bloquear academias — necessário porque este grupo
		// aceita tanto academia quanto admin (RequireAcademiaOuAdmin não preenche
		// "admin_role"). Sem isso, GET/POST de leitura e teste de credenciais, e o PUT
		// de atualização, enxergavam sempre "admin" genérico em vez do role real,
		// bloqueando inclusive administradores FPP legítimos.
		financeiro.Use(middleware.PopulateAdminRole())
		financeiro.POST("/appypay/credenciais", middleware.RequireFPP(), handlers.CriarCredencialAppyPay)
		financeiro.PUT("/appypay/credenciais/:id", handlers.AtualizarCredencialAppyPay)
		financeiro.GET("/appypay/credenciais", handlers.ListarCredenciaisAppyPay)
		financeiro.GET("/appypay/credenciais/:id", handlers.ObterCredencialAppyPay)
		financeiro.POST("/appypay/credenciais/:id/testar", handlers.TestarCredencialAppyPay)
		financeiro.POST("/appypay/credenciais/:id/ativar", middleware.RequireFPP(), handlers.AtivarCredencialAppyPay)
		financeiro.POST("/appypay/credenciais/:id/desativar", middleware.RequireFPP(), handlers.DesativarCredencialAppyPay)
		financeiro.POST("/modalidade-pagamento", middleware.RequireFPP(), handlers.AlterarModalidadePagamento)
```

### `internal/domain/aggregates/aggregate.go`

Dentro de `DefaultAggregateFactory.Create`, remover:

```go
	case "Financeiro":
		return NewFinanceiro(), nil
```

### `internal/middleware/admin_auth_middleware.go` — decisão condicional sobre `PopulateAdminRole`

Depois de remover o bloco do grupo `/financeiro` em `main.go` (passo anterior), execute:

```bash
grep -rn "PopulateAdminRole" --include="*.go" .
```

- Se a **única** ocorrência restante for a definição da própria função (`func PopulateAdminRole() gin.HandlerFunc { ... }`) e o seu comentário, **remova a função inteira** de `internal/middleware/admin_auth_middleware.go` (do comentário `// PopulateAdminRole preenche...` até ao fecho do `}` da função).
- Se aparecer qualquer outra chamada `middleware.PopulateAdminRole()` fora do bloco já removido, **não apague a função** — deixe-a como está, pois está a ser usada por outra funcionalidade não relacionada com esta tarefa.

## 5.3 Whitelist de eventos e aggregates — `internal/db/safe_queries.go`

Remover de `validEventTypes` o bloco completo (comentário incluído):

```go
	// ── Financeiro / AppyPay ───────────────────────────────────────────────
	"CredenciaisAppyPayCadastradas":          true,
	"CredenciaisAppyPayAtualizadas":          true,
	"CredenciaisAppyPayValidadas":            true,
	"CredenciaisAppyPayAtivadas":             true,
	"CredenciaisAppyPayDesativadas":          true,
	"ModalidadePagamentoGlobalAlterada":      true,
	"ModalidadePagamentoSpuriAlterada":       true,
	"ModalidadePagamentoAcademiaAlterada":    true,
	"CobrancaFinanceiraCriada":               true,
	"CobrancaFinanceiraEnviadaAoProvider":    true,
	"CobrancaFinanceiraStatusAtualizado":     true,
	"CobrancaFinanceiraCancelada":            true,
	"ReembolsoFinanceiroSolicitado":          true,
	"ReembolsoFinanceiroStatusAtualizado":    true,
	"ReversaoFinanceiraSolicitada":           true,
	"ReversaoFinanceiraStatusAtualizado":     true,
	"WebhookFinanceiroRecebido":              true,
	"WebhookFinanceiroIgnoradoComoDuplicado": true,
	"DivergenciaFinanceiraDetectada":         true,
	"DivergenciaFinanceiraReconciliada":      true,
	"ReconciliacaoFinanceiraExecutada":       true,
```

Remover de `validAggregateTypes` a linha:

```go
	"Financeiro":                     true,
```

**Por que isto é seguro mesmo havendo eventos `Financeiro` antigos no `spuri_ledger` de algum ambiente real:** a whitelist só é consultada em escrita (`appendDirect`/`AppendTx`), nunca em leitura/replay. Eventos antigos do tipo `Financeiro` continuam no ledger para sempre (é imutável), e a verificação de integridade da cadeia de hashes (`verify_hash_chain`/`VerifyLedgerIntegrity`) não depende da whitelist nem de existir uma projeção ativa para esse `aggregate_type`. Remover as entradas apenas impede novas escritas — que é exatamente o objetivo.

# 6. Escopo obrigatório — migrations SQL

**Não apague `migrations/097_financeiro_base_persistencia.sql` nem `migrations/098_financeiro_event_sourcing.sql`.** Siga o precedente de `migrations/046_remove_aprovacao_reprovacao.sql` (leia esse ficheiro como modelo antes de continuar).

1. Descubra o próximo número de migration livre:

```bash
ls migrations | sort | tail -5
```

Use o número seguinte ao maior encontrado (à data de escrita desta tarefa, o maior número existente é `098`, portanto o próximo é `099` — mas confirme com o comando acima, porque pode já existir uma migration mais recente quando esta tarefa for executada).

2. Crie o ficheiro `migrations/099_remove_modulo_financeiro_appypay.sql` (ajuste o número conforme o passo anterior) com este conteúdo exato:

```sql
-- ============================================================================
-- MIGRATION 099 — Remover projeções do módulo financeiro/AppyPay (rollback)
-- ============================================================================
--
-- CONTEXTO:
--   O módulo financeiro/pagamento (AppyPay), introduzido pelas migrations 097
--   e 098, foi removido por completo do código da aplicação após três rondas
--   de auditoria de segurança identificarem falhas críticas e de alta
--   gravidade ainda não resolvidas de forma consistente (ver
--   docs/Debbugs/Depuração — Módulo base de gestão financeira com AppyPay 1.md
--   e os dois relatórios de verificação subsequentes). A tarefa original
--   (docs/Lista de Tarefas/15 - Modulo base de gestao financeira com AppyPay.md)
--   volta ao estado pendente para uma reimplementação futura mais robusta.
--
--   Nenhum provider HTTP real da AppyPay chegou a ser implementado (apenas
--   FakeProvider); não existem cobranças, credenciais ou webhooks reais a
--   reconciliar externamente antes desta remoção.
--
--   As migrations 097 e 098 NÃO são apagadas — seguem o mesmo padrão já usado
--   na migration 046 (que remove projection_aprovacao_ano/projection_reprovacoes
--   sem apagar as migrations 003/009 que as criaram): o histórico de schema é
--   append-only, apenas esta migration neutraliza as tabelas.
--
-- O QUE ESTA MIGRATION FAZ:
--   1. Remove as tabelas de projeção/armazenamento operacional do módulo
--      financeiro (dados, se existirem em algum ambiente, são perdidos — não
--      há reconciliação pendente conhecida).
--   2. Remove o checkpoint da projeção "financeiro".
-- ============================================================================

BEGIN;

DROP TABLE IF EXISTS financeiro_segredos_appypay;
DROP TABLE IF EXISTS financeiro_webhooks_recebidos;
DROP TABLE IF EXISTS financeiro_cobrancas;
DROP TABLE IF EXISTS financeiro_credenciais_appypay;
DROP TABLE IF EXISTS financeiro_modalidade_pagamento;

DELETE FROM projection_checkpoints WHERE projection_name = 'financeiro';

COMMIT;

DO $$ BEGIN
    RAISE NOTICE '✅ MIGRATION 099 — módulo financeiro/AppyPay removido (rollback completo)';
    RAISE NOTICE '   Tabelas financeiro_* removidas. Migrations 097/098 mantidas como histórico.';
END $$;
```

Não é necessário limpar linhas antigas da tabela `schema_migrations` referentes a `097_financeiro_base_persistencia.sql`/`098_financeiro_event_sourcing.sql` em ambientes onde já tenham corrido — isso é inofensivo e consistente com o resto do repositório (nenhuma migration existente limpa `schema_migrations` manualmente).

# 7. Escopo obrigatório — documentação

## 7.1 `Documentação.md`

Remova, no final do ficheiro, as duas secções completas: a que começa em `## Módulo financeiro base com AppyPay` e a que começa em `## Módulo financeiro — Event Sourcing/CQRS`, incluindo todo o conteúdo até ao fim do ficheiro (confirme com `tail -c 200 Documentação.md` que não sobra nada depois delas antes de apagar; se sobrar algo não relacionado com finance depois dessas secções, mantenha esse conteúdo).

Depois, procure por referências soltas fora dessas duas secções:

```bash
grep -n "financeir\|AppyPay\|appypay" Documentação.md
```

Se aparecer alguma linha fora das duas secções já removidas, avalie o contexto: só remova se for de facto uma referência ao módulo financeiro (rota `/financeiro`, `CredencialAppyPay`, etc.); não remova ocorrências de palavras semelhantes usadas com outro significado (não há nenhuma conhecida neste ficheiro, mas confirme).

## 7.2 Relatórios de auditoria em `docs/Debbugs/`

**Não apague** os três relatórios:

```
docs/Debbugs/Depuração — Módulo base de gestão financeira com AppyPay 1.md
docs/Debbugs/Depuração — Verificação das correções do módulo financeiro AppyPay 1.md
docs/Debbugs/Depuração — Verificação das correções 'AppyPay 1'.md
```

Eles documentam vulnerabilidades reais e são a referência obrigatória para uma reimplementação futura. Em vez de apagar, adicione no topo de cada um destes três ficheiros, imediatamente a seguir ao frontmatter, o seguinte bloco (idêntico nos três):

```markdown
> **Nota de arquivo (rollback):** o módulo financeiro/AppyPay descrito e auditado
> neste relatório foi removido por completo do código em `2026-08-03`
> (ver `docs/Lista de Tarefas/17 - Remover completamente o modulo financeiro
> AppyPay (rollback total).md`). Este documento é mantido apenas como
> referência histórica das vulnerabilidades encontradas, para orientar uma
> futura reimplementação mais robusta a partir da tarefa 15.
```

## 7.3 Documento de análise de origem (se existir)

Se existir o ficheiro `docs/Parceiros e integrações/AppyPay - Análise de Integração para o Serviço de Gestão Financeira do Spuri.md`, adicione-lhe a mesma nota de arquivo indicada em 7.2 (não apague).

# 8. Escopo obrigatório — tarefas 15 e 16 e índice

## 8.1 Tarefa 15 — volta a pendente

1. Mova o ficheiro:

```bash
git mv "docs/Tarefas feitas/15 - Modulo base de gestao financeira com AppyPay.md" \
       "docs/Lista de Tarefas/15 - Modulo base de gestao financeira com AppyPay.md"
```

2. No frontmatter do ficheiro movido, altere:

```yaml
status: feito
```

para:

```yaml
status: pendente
```

3. No título (primeira linha `# ...`), remova o sufixo ` (feito)` se estiver presente, de modo a que o título fique exatamente:

```
# Implementar módulo base de gestão financeira com AppyPay para Spuri e academias
```

4. Logo a seguir ao frontmatter e antes do título (ou logo a seguir ao título, à escolha, desde que fique bem visível no topo), adicione o seguinte bloco:

```markdown
> **Nota de reabertura:** esta tarefa foi implementada, auditada em três
> rondas e depois revertida por completo (rollback total — ver
> `docs/Lista de Tarefas/17 - Remover completamente o modulo financeiro
> AppyPay (rollback total).md` e os três relatórios em `docs/Debbugs/`
> referenciados nesse ficheiro). Antes de reimplementar, leia os três
> relatórios de auditoria: eles documentam falhas críticas específicas
> (chave de cifra previsível, condição de corrida na idempotência de
> cobranças, reatribuição de tenant numa credencial alheia, duplicação do
> histórico auditável) que a reimplementação deve evitar desde o primeiro
> commit — com testes de concorrência (`-race`), testes de replay contra
> Postgres real e testes de isolamento por papel (`estudante` incluído)
> desde o início, em vez de corrigidos ronda a ronda.
```

## 8.2 Tarefa 16 — remoção definitiva

A tarefa 16 documentava um refactor de Event Sourcing/CQRS de um módulo que já não existe. Apague-a por completo (não a mova para pendente):

```bash
git rm "docs/Tarefas feitas/16 - Refatorar modulo financeiro para Event Sourcing CQRS completo.md"
```

## 8.3 Índice — `docs/Lista de Tarefas/00 - Índice e ordem de implementação.md`

Remova, da tabela da secção "Nível 4 — Baixo", a linha da tarefa 16:

```markdown
| 16 | Refatorar módulo financeiro para Event Sourcing/CQRS completo | `16 - Refatorar modulo financeiro para Event Sourcing CQRS completo.md` | Corrige a lacuna arquitetural do módulo financeiro para que credenciais, modalidade, cobranças, webhooks e reconciliações usem o `spuri_ledger` como fonte de verdade, mantendo segredos protegidos e tabelas `financeiro_*` como projeções/read models. |
```

Substitua a linha da tarefa 15 nessa mesma tabela:

```markdown
| 15 | Implementar módulo base de gestão financeira com AppyPay para Spuri e academias | `15 - Modulo base de gestao financeira com AppyPay.md` | Cria a base genérica de cobranças AppyPay para o Spuri cobrar academias e para academias cobrarem estudantes com credenciais próprias, incluindo ativação por FPP/ADMIN, segurança, idempotência, webhooks e reconciliação. |
```

por:

```markdown
| 15 | Implementar módulo base de gestão financeira com AppyPay para Spuri e academias | `15 - Modulo base de gestao financeira com AppyPay.md` | Reimplementação a partir do zero, depois de um rollback total motivado por três rondas de auditoria de segurança (ver tarefa 17 e `docs/Debbugs/`); deve incorporar as lições aprendidas desde o primeiro commit. |
```

Na tabela da secção "Nível 1 — Crítico", adicione uma nova linha ao fundo:

```markdown
| 17 | Remover completamente o módulo financeiro AppyPay (rollback total) | `17 - Remover completamente o modulo financeiro AppyPay (rollback total).md` | Remove por completo código, rotas, whitelist e documentação do módulo financeiro/AppyPay após três rondas de auditoria encontrarem falhas críticas recorrentes; devolve a tarefa 15 ao estado pendente e remove a tarefa 16. Numeração fora de sequência propositadamente — deve ser executada com prioridade máxima, antes de qualquer outra tarefa pendente, por remover código de produção com vulnerabilidades de segurança confirmadas. |
```

# 9. Escopo obrigatório — testes automatizados

## 9.1 `cmd/server/main_test.go`

Remova a função de teste inteira `TestFinanceiroAppyPayRoutesRequireAuthentication` (desde `func TestFinanceiroAppyPayRoutesRequireAuthentication(t *testing.T) {` até ao `}` de fecho da função).

Adicione, no mesmo ficheiro, esta nova função de teste (segue o padrão já usado por `TestLegacyDominisSistemaAnoLetivoRouteIsRemoved` e `TestLegacyStudentProgressionAndInterruptionRoutesAreRemoved` no mesmo ficheiro):

```go
func TestFinanceiroAppyPayRoutesAreRemoved(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := setupRouter()

	const idPlaceholder = "00000000-0000-0000-0000-000000000000"

	removed := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/financeiro/appypay/credenciais"},
		{http.MethodPut, "/financeiro/appypay/credenciais/" + idPlaceholder},
		{http.MethodGet, "/financeiro/appypay/credenciais"},
		{http.MethodGet, "/financeiro/appypay/credenciais/" + idPlaceholder},
		{http.MethodPost, "/financeiro/appypay/credenciais/" + idPlaceholder + "/testar"},
		{http.MethodPost, "/financeiro/appypay/credenciais/" + idPlaceholder + "/ativar"},
		{http.MethodPost, "/financeiro/appypay/credenciais/" + idPlaceholder + "/desativar"},
		{http.MethodPost, "/financeiro/modalidade-pagamento"},
	}
	for _, tc := range removed {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("expected %s %s to be removed with 404, got %d", tc.method, tc.path, w.Code)
		}
	}
}
```

## 9.2 `internal/db/safe_queries_test.go`

Adicione estas duas novas funções de teste (seguem exatamente o padrão de `TestValidateEventTypeRejectsRemovedStudentProgressionEvents` já existente no mesmo ficheiro):

```go
func TestValidateEventTypeRejectsRemovedFinanceiroEvents(t *testing.T) {
	t.Parallel()

	removed := []string{
		"CredenciaisAppyPayCadastradas",
		"CredenciaisAppyPayAtualizadas",
		"CredenciaisAppyPayValidadas",
		"CredenciaisAppyPayAtivadas",
		"CredenciaisAppyPayDesativadas",
		"ModalidadePagamentoGlobalAlterada",
		"ModalidadePagamentoSpuriAlterada",
		"ModalidadePagamentoAcademiaAlterada",
		"CobrancaFinanceiraCriada",
		"CobrancaFinanceiraEnviadaAoProvider",
		"CobrancaFinanceiraStatusAtualizado",
		"CobrancaFinanceiraCancelada",
		"ReembolsoFinanceiroSolicitado",
		"ReembolsoFinanceiroStatusAtualizado",
		"ReversaoFinanceiraSolicitada",
		"ReversaoFinanceiraStatusAtualizado",
		"WebhookFinanceiroRecebido",
		"WebhookFinanceiroIgnoradoComoDuplicado",
		"DivergenciaFinanceiraDetectada",
		"DivergenciaFinanceiraReconciliada",
		"ReconciliacaoFinanceiraExecutada",
	}

	for _, eventType := range removed {
		eventType := eventType
		t.Run(eventType, func(t *testing.T) {
			t.Parallel()
			if err := ValidateEventType(eventType); err == nil {
				t.Fatalf("ValidateEventType(%q) retornou nil, want erro", eventType)
			}
		})
	}
}

func TestValidateAggregateTypeRejectsRemovedFinanceiroAggregate(t *testing.T) {
	t.Parallel()

	if err := ValidateAggregateType("Financeiro"); err == nil {
		t.Fatal("ValidateAggregateType(\"Financeiro\") retornou nil, want erro")
	}
}
```

## 9.3 Confirmar

```bash
go build ./... && go vet ./... && go test ./...
```

Todos os comandos devem terminar sem erro.

# 10. Verificação final obrigatória (checklist de grep)

Execute cada comando abaixo a partir da raiz do repositório e confirme o resultado esperado indicado. Só considere a tarefa concluída depois de todos passarem.

```bash
# 1. Nenhum ficheiro Go deve mencionar o módulo financeiro.
grep -rn "internal/finance" --include="*.go" .
# Esperado: nenhum resultado.

grep -rln "AppyPay\|appypay" --include="*.go" .
# Esperado: nenhum resultado.

grep -rln "Financeiro\b" --include="*.go" .
# Esperado: nenhum resultado (exceto, se aplicável, comentários incidentais
# não relacionados — confirme manualmente qualquer resultado antes de ignorar).

# 2. Nenhuma rota /financeiro registada.
grep -rn '"/financeiro' --include="*.go" .
# Esperado: nenhum resultado.

# 3. Migrations 097/098 continuam a existir (não foram apagadas).
ls migrations/097_financeiro_base_persistencia.sql migrations/098_financeiro_event_sourcing.sql
# Esperado: ambos os ficheiros listados sem erro.

# 4. Nova migration de limpeza existe.
ls migrations/*remove_modulo_financeiro_appypay.sql
# Esperado: um ficheiro listado.

# 5. Documentação.md já não descreve o módulo.
grep -n "Módulo financeiro" Documentação.md
# Esperado: nenhum resultado.

# 6. Tarefa 15 está pendente e tarefa 16 foi removida.
test -f "docs/Lista de Tarefas/15 - Modulo base de gestao financeira com AppyPay.md" && echo OK_15
test ! -f "docs/Tarefas feitas/15 - Modulo base de gestao financeira com AppyPay.md" && echo OK_15_NAO_ESTA_EM_FEITAS
test ! -f "docs/Tarefas feitas/16 - Refatorar modulo financeiro para Event Sourcing CQRS completo.md" && echo OK_16_APAGADA
# Esperado: as três linhas "OK_..." impressas.

# 7. Build e testes.
go build ./...
go vet ./...
go test ./...
# Esperado: todos sem erro.
```

Se qualquer um destes comandos devolver um resultado inesperado, volte à secção correspondente deste documento e corrija antes de continuar.

# Fora de escopo

- Reimplementar o módulo financeiro — isso pertence à tarefa 15 (que volta a `docs/Lista de Tarefas/` como pendente por causa desta tarefa, mas a reimplementação em si **não** faz parte desta tarefa 17).
- Apagar as migrations `097` e `098` — nunca apagar migrations já existentes (ver Secção 3 e 6).
- Alterar qualquer módulo não relacionado com finanças/AppyPay.
- Renumerar migrations existentes ou reorganizar a numeração histórica.
- Reescrever histórico do `spuri_ledger` — eventos `Financeiro` antigos, se existirem nalgum ambiente, permanecem no ledger (é imutável); esta tarefa só impede novas escritas.
- Apagar os relatórios de auditoria em `docs/Debbugs/` ou o documento de análise de origem — apenas anotar, nunca apagar (Secção 7.2 e 7.3).
- Reescrever ou reorganizar a numeração de outras tarefas do índice além do ajuste mínimo descrito na Secção 8.3.

# Critérios de aceite

1. O diretório `internal/finance/` não existe mais no repositório.
2. `internal/domain/aggregates/financeiro.go`, `internal/projections/financeiro_projection.go` e `internal/handlers/financeiro_handlers.go` não existem mais.
3. `cmd/server/main.go` não importa `spuri/internal/finance`, não chama `finance.ValidateEncryptionConfig`, não define `handlers.FinanceiroService`, não regista a projeção `"financeiro"` e não regista nenhuma rota `/financeiro/*`.
4. `internal/domain/aggregates/aggregate.go` não tem `case "Financeiro"`.
5. `internal/db/safe_queries.go` não tem nenhuma entrada `Financeiro`/`AppyPay`/`Cobranca`/`Reembolso`/`Reversao`/`Divergencia`/`Reconciliacao`/`WebhookFinanceiro`/`ModalidadePagamento*`/`CredenciaisAppyPay*` na whitelist de eventos nem `"Financeiro"` na whitelist de aggregates.
6. `go build ./...`, `go vet ./...` e `go test ./...` terminam sem erro.
7. `migrations/097_financeiro_base_persistencia.sql` e `migrations/098_financeiro_event_sourcing.sql` continuam a existir, inalteradas.
8. Existe uma nova migration (número seguinte ao mais alto existente no momento da execução) que remove as tabelas `financeiro_*` e o checkpoint `financeiro`.
9. `Documentação.md` já não contém as secções "Módulo financeiro base com AppyPay" nem "Módulo financeiro — Event Sourcing/CQRS".
10. `docs/Tarefas feitas/15 - Modulo base de gestao financeira com AppyPay.md` não existe; `docs/Lista de Tarefas/15 - Modulo base de gestao financeira com AppyPay.md` existe, com `status: pendente`, sem sufixo `(feito)` no título, e com a nota de reabertura descrita na Secção 8.1.
11. `docs/Tarefas feitas/16 - Refatorar modulo financeiro para Event Sourcing CQRS completo.md` não existe em nenhum diretório do repositório.
12. Os três relatórios em `docs/Debbugs/` referentes ao módulo financeiro continuam a existir, cada um com a nota de arquivo descrita na Secção 7.2.
13. `docs/Lista de Tarefas/00 - Índice e ordem de implementação.md` reflete as alterações descritas na Secção 8.3.
14. Todos os comandos da checklist da Secção 10 produzem o resultado esperado.

## Procedimento de conclusão

Ao finalizar esta tarefa:

1. Confirmar que todos os itens da checklist da Secção 10 passaram.
2. Atualizar o título interno para `# Remover por completo o módulo financeiro/pagamento AppyPay (rollback total) (feito)`;
3. Alterar o front matter para `status: feito`;
4. Mover este ficheiro para `docs/Tarefas feitas/`;
5. Remover a linha desta tarefa (17) da tabela "Nível 1 — Crítico" em `docs/Lista de Tarefas/00 - Índice e ordem de implementação.md`, já que deixa de estar pendente.
