---
criado: 2026-08-13 00:00
origem: depuração pós-implementação das tarefas 26 e 27, feita pelo Spuri (Claude como orquestrador/auditor)
status: feito
depende_de: nenhuma (as tarefas 26 e 27 já foram implementadas — commits `1f20a3d` e `bd30f40`; esta tarefa corrige e reforça essa implementação, não a substitui)
---

# Correção pós-implementação das Fases 1 e 2 do módulo de pagamentos (feito)

## Prompt recomendado para executar a atualização

As tarefas 26 (`docs/Tarefas feitas/26 - ...md`) e 27 (`docs/Tarefas feitas/27 - ...md`) já foram implementadas. Uma auditoria de código confirmou que a Fase 1 (cancelamento de cobrança, precisão monetária) está correta e bem testada — **não mexer nela**. A Fase 2 (mensalidade) tem a lógica de negócio correta, mas com dois problemas concretos que devem ser corrigidos, e um terceiro ponto que precisa de investigação e, se confirmado, correção. Trabalhe exclusivamente em `internal/finance/mensalidade.go`, `internal/handlers/mensalidade_handlers.go`, os respetivos testes, e o aggregate `Turma` (`internal/domain/aggregates/turma.go`) apenas se a seção 3 for confirmada. Não reescreva nem redesenhe a lógica de negócio já existente — as três secções abaixo são correções e reforços pontuais, não uma nova implementação.

## Contexto (achados da auditoria)

Foi feita uma auditoria completa do código implementado, incluindo compilação (`go build ./...`) e execução dos testes unitários existentes — ambos passam sem erros. A auditoria confirmou:

**Fase 1 — correta e bem testada.** `roundAmount`/`amountsEqual` implementados e documentados; `CancelCharge` aplica exatamente a autorização restrita pedida (admin `fpp` só cancela `contexto="spuri"`, nunca cobrança de academia); o handler `CancelarCobrancaAppyPay` explicitamente não reaproveita `authorizeFinanceScope`; o evento de conflito pós-cancelamento está correto; há testes unitários e de integração cobrindo exatamente estes cenários (`TestIntegrationFinanceFPPAdminCannotCancelAcademyCharge`, `TestIntegrationCancelChargeAndLateSuccessConflict`, entre outros). Nenhuma ação necessária aqui.

**Fase 2 — lógica correta, mas com dois problemas confirmados:**

1. **Codificação UTF-8 corrompida ("mojibake") em todas as mensagens de erro com acentos**, em `internal/finance/mensalidade.go` (20 ocorrências) e `internal/handlers/mensalidade_handlers.go` (9 ocorrências). Exemplo real do ficheiro: a mensagem que devia ser `"amount deve ter no máximo duas casas decimais"` aparece como `"invÃ¡lido"`, `"obrigatÃ³rios"`, `"nÃ£o"`, etc. Confirmado por inspeção de bytes: o texto sofreu uma dupla codificação UTF-8 (os bytes UTF-8 corretos foram reinterpretados como Latin-1 e recodificados em UTF-8 por cima). Isto **não** afeta `internal/finance/appypay.go` (Fase 1, 0 ocorrências) nem `"Documentação da API.md"` (0 ocorrências) — está isolado exatamente aos dois ficheiros novos da Fase 2. Todas as mensagens de validação mostradas a academias, estudantes e admins através deste módulo saem com texto ilegível.
2. **Praticamente nenhuma cobertura de teste automatizado para a Fase 2.** Existe apenas `internal/finance/mensalidade_test.go` (3 testes unitários, sem tocar na base de dados): validação de `mesesAnoLetivo`/`anoLetivoValido`, determinismo do ID do aggregate, e uma cópia manual (não uma chamada à função real) da lógica de precedência de estado. **Não existe nenhum teste de integração para mensalidade** — ao contrário da Fase 1, que tem três. Isto significa que nenhum dos cenários exigidos pela tarefa 27 está de facto verificado automaticamente: resolução histórica de valor (Seção 6 — o requisito mais crítico do documento), granularidade fundamental vs. curso+ano, transferência de estudante entre academias, fronteiras de `mes_inicio`/`mes_fim_cobranca`, ou o bloqueio de `anular`/`reativar` a admins `fpp`. Dado que este módulo determina exatamente quanto uma família deve pagar, a ausência de testes aqui é o risco mais alto desta auditoria.

**Terceiro ponto — precisa de investigação, pode ser um bug real:** a resolução histórica de vínculo (`vinculosMensalidade`, em `mensalidade.go`) lê `t.nivel` e `t.curso_id` da linha **atual** da turma (`projection_turmas`) para preencher também as entradas do histórico (`historico_estudantes_ano_letivo`). O aggregate `Turma` tem um método `AtualizarDados(nivel *string, cursoID *uuid.UUID, ...)` que permite alterar `nivel`/`curso_id` de uma turma já existente, e `historico_estudantes_ano_letivo` guarda apenas a lista de códigos de estudante por ano letivo — **não** guarda qual era o `nivel`/`curso_id` da turma naquele ano letivo especificamente. Ou seja: se uma academia reutilizar/atualizar a mesma turma para o ano letivo seguinte com um `nivel` ou `curso_id` diferente (ex.: turma que era "6_ano_fundamental" passa a "7_ano_fundamental"), a consulta de mensalidade passaria a resolver o valor de um mês pendente antigo com o `nivel`/`curso_id` **atual** da turma, não o que estava em vigor naquele ano letivo — exatamente o erro que a Seção 6 da tarefa 27 exige impedir. Isto não foi confirmado com dados reais (por não haver ambiente de banco de dados disponível nesta auditoria), mas o desenho atual não tem nenhuma proteção contra este cenário, e é exatamente o tipo de caso que a cobertura de teste em falta (ponto 2) deveria ter apanhado.

---

# 1. Corrigir a codificação UTF-8 corrompida

## Objetivo

Eliminar a dupla codificação UTF-8 em todas as strings literais de `internal/finance/mensalidade.go` e `internal/handlers/mensalidade_handlers.go`, sem alterar nenhum comportamento, apenas o texto exibido.

## Escopo obrigatório

### 1.1 Corrigir os literais afetados

Cada string afetada deve ser corrigida para o texto correto em português (europeu/angolano), removendo a dupla codificação. Reproduza a transformação exata (confirmada nesta auditoria) para automatizar a correção com segurança, em vez de reescrever cada string manualmente (risco de erro humano em ~29 ocorrências):

```python
# Para cada string literal afetada, a correção é:
texto_corrigido = texto_corrompido.encode('latin-1').decode('utf-8')
# Ex.: "invÃ¡lido".encode('latin-1').decode('utf-8') == "inválido"
```

Aplique isto apenas às strings literais dentro de `errors.New(...)`, `fmt.Errorf(...)` e comentários afetados nesses dois ficheiros — não toque em nenhum identificador, nome de campo JSON, nem em qualquer outro ficheiro do repositório (a auditoria confirmou que o problema está isolado a estes dois ficheiros).

### 1.2 Confirmar que não sobra nenhuma ocorrência

Depois da correção, `grep -c "Ã§\|Ã£\|Ã©\|Ã­\|Ã³\|Ã¡\|Ã‰\|Ã‡" internal/finance/mensalidade.go internal/handlers/mensalidade_handlers.go` deve devolver `0` para ambos os ficheiros.

### 1.3 Testes obrigatórios

1. `go build ./...` continua a compilar sem erros após a correção;
2. os 3 testes unitários existentes em `mensalidade_test.go` continuam a passar inalterados (a correção é só de texto, não de lógica);
3. inspeção manual (ou script) confirmando que pelo menos 3 mensagens de erro específicas (ex.: a de `amount` inválido, a de autorização, a de `mes_inicio`) mostram texto correto em português.

---

# 2. Cobertura de testes automatizados para a Fase 2

## Objetivo

Verificar automaticamente, por teste, todos os cenários que a tarefa 27 já exigia e que hoje não têm nenhuma cobertura — em particular a resolução histórica de valor (Seção 6 da tarefa 27), que é o requisito de negócio mais sensível deste módulo.

## Escopo obrigatório

### 2.1 Testes de integração (novo ficheiro `internal/finance/mensalidade_integration_test.go`, seguindo o padrão já usado em `appypay_integration_test.go`)

No mínimo, cobrir:

1. **Resolução histórica de valor (Seção 6 da tarefa 27) — prioridade máxima:** configurar um valor para `6_ano_fundamental`, gerar/():simular um mês pendente nesse ano letivo, depois reconfigurar o valor para um novo preço; confirmar que o mês antigo continua a resolver para o preço antigo e que um mês com data de referência posterior à mudança resolve para o preço novo — reproduzindo exatamente o teste 6.3.4 da tarefa 27 (estudante paga, já depois da mudança de preço, um mês antigo ainda pendente: o valor cobrado é o preço antigo);
2. progressão de ano académico entre dois anos letivos (`6_ano_fundamental` → `7_ano_fundamental`): mês pendente do ano letivo antigo resolve com a configuração do ano académico antigo, não do atual (teste 6.3.1 da tarefa 27);
3. mudança de curso entre anos letivos (médio/superior): mesmo princípio, aplicado a `curso_id` (teste 6.3.2 da tarefa 27) — ver também a Seção 3 desta tarefa antes de escrever este teste;
4. transferência de estudante entre duas academias: pendência da academia anterior resolve com a configuração dessa academia, nunca com a da academia atual (teste 6.3.3 da tarefa 27);
5. granularidade: configuração de mensalidade rejeitada para academia pública; configuração aceite para `curso_id`+ano no médio/superior e rejeitada sem `curso_id`; configuração rejeitada para ano/curso que a academia não oferece (testes 1.4 da tarefa 27);
6. `mes_fim_cobranca` aceita apenas 6 ou 7, e nunca excede o período letivo fixo (teste 2.2.3 da tarefa 27);
7. `mes_inicio_cobranca`: só se aplica ao ano letivo em que foi definido; próximo ano letivo volta ao mês natural; rejeitado se anterior ao mês natural ou posterior ao `mes_fim_cobranca` (testes 4.3 da tarefa 27);
8. anular e reativar: fluxo completo, incluindo rejeitar reativação de mês não anulado ou já pago (testes 5.3.5/5.3.6 da tarefa 27);
9. **admin `fpp` tentando anular ou reativar uma obrigação → rejeitado**, mesmo tendo acesso de consulta ao mesmo estudante (teste 5.3.7 da tarefa 27 — este é um teste de regressão de segurança, não pode faltar);
10. consulta de mensalidades: academia só vê as suas próprias pendências; admin `fpp` e o próprio estudante veem pendências de todas as academias com que o estudante já teve vínculo.

### 2.2 Corrigir o teste unitário existente que testa uma cópia da lógica, não a função real

`TestMensalidadeStatePrecedence`, em `mensalidade_test.go`, reimplementa manualmente o switch de precedência de estado em vez de chamar `estadoObrigacao` (que exige banco de dados) ou uma função equivalente extraída e testável sem banco. Extraia a lógica de precedência de `estadoObrigacao` para uma função pura testável sem banco de dados (ex.: `precedenciaEstado(eventos []string) string`), reutilizada tanto pela função real quanto pelo teste — para que uma futura alteração na lógica real seja apanhada pelo teste, o que hoje não acontece.

### 2.3 Testes obrigatórios (resumo desta secção)

Todos os itens de 2.1 (10 cenários) devem existir como testes de integração passando, e o item 2.2 deve estar corrigido com teste unitário sobre a função real extraída.

---

# 3. Investigar e corrigir a estabilidade de `nivel`/`curso_id` de turma ao longo do tempo

## Objetivo

Confirmar se `Turma.AtualizarDados` pode alterar `nivel`/`curso_id` de uma turma que já tem histórico (`historico_estudantes_ano_letivo` não vazio) e, se puder, impedir isso — porque a resolução histórica da Fase 2 (tarefa 27, Seção 6) depende de `nivel`/`curso_id` da turma serem estáveis para qualquer ano letivo em que ela já teve estudantes.

## Regra de negócio

- Uma turma que já tem pelo menos uma entrada em `historico_estudantes_ano_letivo` (ou seja, já foi usada num ano letivo, mesmo que não o corrente) não pode mais ter `nivel` nem `curso_id` alterados por `AtualizarDados`. Isto preserva a garantia de que, para qualquer `(codigo_academia, ano_letivo)` em que uma turma teve estudantes, o `nivel`/`curso_id` lido hoje é exatamente o que estava em vigor naquele ano letivo — pré-condição não documentada de que a Fase 2 (mensalidade) depende.
- Turmas **sem** histórico (recém-criadas, ainda no primeiro ano letivo de uso) continuam a poder ter `nivel`/`curso_id` corrigidos livremente, como hoje.

## Escopo obrigatório

### 3.1 Confirmar o comportamento atual

Escrever um teste (`internal/domain/aggregates/turma_test.go` ou equivalente já existente) que comprove o estado atual: chamar `AtualizarDados` alterando `nivel` numa turma com `historico_estudantes_ano_letivo` não vazio e confirmar se isso é hoje aceite (documentando o resultado antes de corrigir, para que o commit de correção mostre claramente o comportamento anterior vs. o novo).

### 3.2 Adicionar a validação

Se o teste do item 3.1 confirmar que a alteração é aceite hoje, adicionar a validação em `Turma.AtualizarDados`: rejeitar alteração de `nivel`/`curso_id` quando `historico_estudantes_ano_letivo` não estiver vazio, com mensagem de erro clara. `turno` e outros campos não relacionados à identidade académica da turma continuam livres para alteração.

### 3.3 Testes obrigatórios

1. `AtualizarDados` alterando `nivel` numa turma **sem** histórico → aceite (comportamento inalterado);
2. `AtualizarDados` alterando `nivel` ou `curso_id` numa turma **com** histórico não vazio → rejeitado, com mensagem clara;
3. `AtualizarDados` alterando apenas `turno` numa turma com histórico não vazio → continua aceite;
4. teste de integração da Seção 2.1, item 3 (mudança de curso entre anos letivos), reescrito para confirmar que, com esta validação em vigor, a resolução histórica da Fase 2 está genuinamente correta — não apenas por não ter sido exercitado o caso de mudança indevida.

---

# Fora de escopo

- Qualquer alteração à Fase 1 (já auditada e confirmada correta).
- Qualquer alteração de lógica de negócio da Fase 2 além do que está descrito nas três secções acima — a lógica já implementada está correta e não deve ser redesenhada.
- Início da Fase 3 ou Fase 4 (tarefas 28 e 29) — permanecem tarefas separadas, não fazem parte desta correção.
- Migração de dados existentes que possam já ter sido afetados pelo cenário da Seção 3 antes desta correção (não há dados em produção ainda, dado o estado atual do projeto).

# Riscos e mitigações

| Risco | Mitigação |
| --- | --- |
| Corrigir a codificação (Seção 1) acidentalmente alterar alguma string além do necessário | Aplicar a transformação apenas às strings identificadas como corrompidas (grep confirma antes/depois), nunca uma reescrita livre |
| Testes novos (Seção 2) exigirem infraestrutura de banco de dados não disponível no ambiente de execução do Codex | Seguir exatamente o padrão de setup já usado em `appypay_integration_test.go`/`financeiro_handlers_integration_test.go`, que já resolve isso no projeto |
| Impedir alteração de `nivel`/`curso_id` (Seção 3) quebrar algum fluxo legítimo já existente de correção de dados de turma | Restringir a validação apenas a turmas com histórico não vazio (turmas novas continuam livres), e correr toda a suite de testes de `turma_test.go` já existente após a alteração |

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `grep` confirmar zero ocorrências de codificação corrompida nos dois ficheiros da Fase 2;
2. `go build ./...` e todos os testes unitários existentes continuarem a passar;
3. os 10 cenários de teste de integração da Seção 2.1 existirem e passarem, incluindo o teste de segurança (admin não pode anular/reativar);
4. `TestMensalidadeStatePrecedence` testar a função real extraída, não uma cópia da lógica;
5. a investigação da Seção 3 estiver documentada (resultado do teste 3.1) e, se confirmado o problema, a validação estiver implementada e testada (3.2/3.3);
6. nenhuma mudança de comportamento for introduzida na Fase 1 nem na lógica de negócio já correta da Fase 2.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Correção pós-implementação das Fases 1 e 2 do módulo de pagamentos (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
