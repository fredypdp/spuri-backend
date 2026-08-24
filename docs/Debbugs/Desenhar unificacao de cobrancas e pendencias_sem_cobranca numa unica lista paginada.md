---
data: 2026-08-24
status: corrigido_via_64_unificar_cobrancas_e_pendencias_sem_cobranca_numa_unica_lista_paginada
auditor: Claude (orquestrador) — desenho e implementação com PostgreSQL 16, Go 1.24, Node 22 e Next.js reais em sandbox
tarefa_correcao_backend: docs/Lista de Tarefas/64 - Unificar cobrancas e pendencias_sem_cobranca numa unica lista paginada.md
tarefa_correcao_frontend: entregue separadamente para o repositório spuripainel (sem convenção docs/Lista de Tarefas própria) — ver "65 - Consumir a lista unificada de pagamentos (dependente da tarefa 64 do backend).md"
---

# Unificar `cobrancas` + `pendencias_sem_cobranca` numa única lista paginada, com `pendencia_sem_cobranca` como discriminador

## 1. Pedido original

Fredy pediu que `GET /financeiro/cobrancas` (e, por consistência/homogeneidade, `GET /financeiro/cobrancas/estudante/:codigo`) parasse de devolver duas listas separadas (`cobrancas` + `pendencias_sem_cobranca`, esta última sem paginação própria) e passasse a devolver **uma única lista paginada**, em que cada item usa a mesma estrutura de sempre (a de uma cobrança), acrescida de um campo booleano `pendencia_sem_cobranca` — permitindo ao cliente saber, quando `status` vier `"pendente"`, se é porque a cobrança foi de fato tentada e a AppyPay devolveu um estado ainda não resolvido, ou porque não existe nenhuma cobrança gerada para aquele mês.

## 2. Decisão de design que este pedido escancarou (resolvida antes de implementar)

A tarefa 63 (`docs/Tarefas feitas/63 - ...`) mudou `PendenciasSemCobranca` para incluir **qualquer** mês ainda não pago — mesmo que já tenha havido uma tentativa de cobrança FALHADA. Isso foi uma correção deliberada e correta (documentada e testada). Mas, ao unificar as duas listas numa só, essa mudança cria um problema novo: **o mesmo mês passaria a aparecer duas vezes** na lista unificada — uma vez como a cobrança real (`status: "falhada"`, `pendencia_sem_cobranca: false`) e outra vez como uma pendência sintética redundante (`status: "pendente"`, `pendencia_sem_cobranca: true`) para o mesmo `(estudante, ano_letivo, mes)`.

Isso foi confirmado empiricamente (não só deduzido): reproduzi o cenário com PostgreSQL real (1 cobrança falhada + `PendenciasSemCobranca` chamada em seguida) e vi os dois itens aparecerem, antes de eu introduzir a deduplicação abaixo.

**Resolução:** introduzida `FiltrarPendenciasComCobrancaRealVinculada` (`internal/finance/pagamentos_unificado.go`), que remove de `pendencias` qualquer mês que já tenha pelo menos uma linha em `financeiro_mensalidade_cobrancas` — a MESMA consulta que a antiga `cobrancasExistentesMensalidade` fazia (removida na tarefa 63), mas usada aqui para um propósito diferente: deduplicação na composição da lista final, não para decidir se um mês está pago (isso continua sendo, exclusivamente, `Estado != EstadoPendente`, vindo dos eventos de obrigação — tarefa 63 continua válida e intacta). O comentário no código é explícito sobre essa distinção, para que um futuro leitor (ou o Codex, numa tarefa futura) não confunda isso com uma reversão da tarefa 63.

## 3. Desenho da paginação (o motivo de existir esta tarefa)

Antes: `cobrancas` era paginada no banco (`LIMIT`/`OFFSET`); `pendencias_sem_cobranca` sempre vinha por inteiro, sem paginação — o cliente tinha que lidar com duas fontes de dados com paginação incompatível.

Agora: `ListarPagamentosUnificado` (`internal/finance/pagamentos_unificado.go`) trata a lista unificada como duas fontes concatenadas — pendências primeiro (já resolvidas por inteiro pelo chamador, é assim que `PendenciasSemCobranca` sempre funcionou, nada muda nisso), cobranças reais depois — e faz a matemática de paginação sem nunca buscar mais linhas de `financeiro_cobrancas` do que cabem na página atual:

- Se a página cai inteiramente dentro do intervalo de pendências, `buscarCobrancas` só é chamada com `limit=0` (só para obter o `total` real, sem trazer nenhuma linha extra — o `COUNT(*)` já roda antes do `LIMIT` dentro de `ListCobrancas`/`ListCobrancasEstudante`, então isso não é uma consulta desperdiçada).
- Se a página cai parcialmente nas duas fontes, o `offset` repassado a `buscarCobrancas` é ajustado para descontar quantos itens da página já vieram de pendências.
- Se a página cai inteiramente em cobranças reais, o comportamento é um passthrough exato do que já existia (mesmo `limit`/`offset`).

Esta matemática foi testada exaustivamente com testes unitários puros (sem Postgres — `pagamentos_unificado_test.go`, 8 casos incluindo limites exatos, páginas mistas e páginas de transição) e depois confirmada com um teste de integração real (`pagamentos_unificado_integration_test.go`) que reproduz, com PostgreSQL real, exatamente o que os handlers fazem: `PendenciasSemCobranca` → `FiltrarPendenciasComCobrancaRealVinculada` → `ListarPagamentosUnificado`.

## 4. Outras decisões de design

- **`id` determinístico para itens sintéticos:** `uuid.NewSHA1` com um namespace fixo, a partir de `academia|estudante|ano_letivo|mes` — a mesma pendência sempre produz o mesmo `id` entre chamadas (útil como `key` de lista no React), e nunca colide com um `id` real (sempre `uuid.New()`, versão 4; o sintético é versão 5 — versões diferentes de UUID nunca colidem, RFC 4122).
- **`AtualizadoEm` virou `*time.Time`** (era `time.Time`): um item sintético não tem nenhuma atividade real para reportar — `nil`, omitido do JSON, é honesto; inventar uma data (`time.Now()` ou zero-value) não seria. Validado empiricamente que `database/sql`/`lib/pq` escaneiam `*time.Time` corretamente a partir de uma coluna `NOT NULL` (só uso real no repositório, `scanCobrancaResumo`, não precisou de nenhuma outra mudança). Único campo do contrato existente que muda de tipo — documentado como mudança deliberada e coordenada com o frontend (tarefa 65).
- **`moeda: "AOA"` fixo** para itens sintéticos — é a única moeda usada em todo o fluxo de mensalidade no backend hoje (confirmado por grep, nenhuma outra moeda aparece em nenhum ponto do fluxo de mensalidade).
- **Descrição sintética** segue o mesmo formato `"Propinas {academia}: N mensalidade(s)"` já usado em cobranças reais de mensalidade (`internal/finance/mensalidade.go`), com um sufixo indicando que é uma pendência sem cobrança gerada.
- **Ordenação:** itens sintéticos sempre primeiro (ação pendente — "isto ainda precisa de uma cobrança"), cobranças reais depois, por `updated_at DESC` (inalterado). Decisão de produto razoável e documentada, não uma consequência técnica inevitável — outra ordenação poderia ter sido escolhida.
- **Endpoint do estudante (19.8) recebeu o mesmo tratamento**, por consistência: `PendenciasSemCobrancaEstudante` também precisa do mesmo filtro de deduplicação (`FiltrarPendenciasComCobrancaRealVinculada`), e como o estudante pode ter pendências em mais de uma academia (histórico), essa função agrupa internamente por `CodigoAcademia` (cada `MensalidadeMesView` já carrega o seu) em vez de exigir uma única academia como parâmetro.

## 5. Validação executada (com Postgres 16, Go 1.24, Node 22 e Next.js reais)

**Backend:**
- `go build`, `go vet`, `gofmt` limpos.
- 8 testes unitários puros da matemática de paginação (sem Postgres).
- 2 novos testes de integração (`FiltrarPendenciasComCobrancaRealVinculada` isolada; fluxo completo do handler reproduzido com dados reais) com PostgreSQL 16 real.
- 3 testes de handler HTTP reescritos para o novo contrato (`pagamentos` em vez de `cobrancas`/`pendencias_sem_cobranca`), rodados com PostgreSQL real via `httptest`.
- Suíte completa do repositório (`go test ./...`, todos os pacotes) rodada com `RUN_POSTGRES_INTEGRATION=1` e `FINANCE_ENCRYPTION_KEY` configurada (variável necessária para os testes de credenciais AppyPay, ausente por padrão neste sandbox — configurá-la eliminou as 9 falhas pré-existentes que apareciam nas tarefas anteriores, confirmando que eram mesmo só isso e nada relacionado ao código): **100% verde, todos os pacotes.**
- Confirmação final: apliquei os 7 arquivos (4 alterados + 3 novos) num clone **novo e independente** do repositório e rodei tudo de novo do zero — mesmo resultado, 100% verde.
- Documentação da API (`Documentação da API.md`, seções 19.7 e 19.8) reescrita para o novo contrato.

**Frontend:**
- `npx tsc --noEmit`: limpo, nos 4 arquivos alterados (`src/types/api.ts`, `financeiroShared.tsx`, `FinanceiroPagamentosPainel.tsx`, `EstudantePagamentosPainel.tsx`) e no repositório inteiro.
- `npx eslint`: limpo, nos 4 arquivos alterados e no repositório inteiro (os únicos erros/warnings que aparecem rodando o eslint completo estão em 4 arquivos que esta tarefa não toca — confirmados pré-existentes).
- Confirmação final: apliquei os 4 arquivos num clone **novo e independente** do repositório, rodei `npm install` + `tsc` + `eslint` de novo do zero — mesmo resultado, limpo.
- Descoberta incidental (não um bug introduzido por esta tarefa, uma lacuna pré-existente que esta unificação corrige de graça): a tela "Histórico de pagamentos" do estudante (`EstudantePagamentosPainel.tsx`) já ignorava silenciosamente `pendencias_sem_cobranca` — só renderizava `cobrancas`. Com a unificação, essa tela passa a mostrar pendências também, sem nenhuma mudança de código além da renomeação do campo consumido.

## 6. O que falta

Nada em aberto do lado da correção em si. As duas tarefas (backend 64, frontend 65) contêm o código completo e testado, prontas para execução mecânica pelo Codex.
