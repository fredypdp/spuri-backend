---
criado: 2026-08-14 00:00
origem: depuração pós-implementação das Fases 3, 4 e da correção 31, feita pelo Spuri (Claude como orquestrador/auditor, verificação com Go 1.24 + PostgreSQL real)
status: feito
depende_de: nenhuma (as tarefas 26-29 e 31 já foram implementadas; esta tarefa corrige e reforça essa implementação, não a substitui)
---

# Correção final do módulo de pagamentos — Fases 1 a 4 (feito)

## Prompt recomendado para executar a atualização

As tarefas 26, 27, 28, 29 e 31 já foram implementadas. Uma auditoria com ambiente real (Go 1.24 + PostgreSQL local, não apenas leitura de código) confirmou que a arquitetura e a maior parte da lógica de negócio das quatro fases estão corretas — em particular, a Fase 3 (pagamento entre academias, acesso restrito financeiro) e a Fase 4 (busca pública, valor fixado na aprovação, cancelamento em cascata reaproveitando o conflito da Fase 1) estão bem implementadas. Mas foram confirmados problemas concretos, com prioridade decrescente: (1) a correção de codificação da tarefa 31 nunca foi de facto aplicada; (2) um bug real de resolução de preço histórico que impede cobrar meses já decorridos sempre que uma academia configura/atualiza a mensalidade a meio do ano letivo — isto é o cenário mais comum de uso real, não um caso raro; (3) bugs nos próprios testes de integração que impedem a suite de correr e, por isso, mascaram outros problemas; (4) ausência de testes de integração para a Fase 4. Corrija pela ordem das secções abaixo — a Secção 3 (testes) deve ser corrigida **antes** de tentar validar a Secção 2, porque sem ela não é possível confirmar a correção com confiança.

**Importante:** desta vez, confirme cada correção correndo de facto `go test ./... -run TestIntegration` contra um PostgreSQL real (`RUN_POSTGRES_INTEGRATION=1`, `DATABASE_URL` local), não apenas por leitura de código. A ausência dessa verificação real foi a causa de pelo menos dois dos problemas listados abaixo terem passado despercebidos nas implementações anteriores.

## Contexto (metodologia e achados da auditoria)

Foi montado um ambiente de verificação real: `go build ./...` e `go vet ./...` completos (sem erros), e os testes de integração foram corridos contra um PostgreSQL local recém-criado (não apenas lidos). Isto revelou problemas que uma auditoria só por leitura de código não teria apanhado.

### O que está confirmado correto (não mexer)

- **Fase 1** (`internal/finance/appypay.go`): `CancelCharge`, a autorização restrita (admin só cancela `contexto="spuri"`), e o evento de conflito pós-cancelamento (`CobrancaAppyPayConflitoPosCancelamento`) estão corretos. Confirmado também que este mesmo mecanismo de conflito é reaproveitado corretamente por `AcceptWebhook` para **qualquer** cobrança, incluindo as de matrícula (Fase 4) — quando um webhook tardio confirma sucesso numa cobrança já cancelada, o status `cancelada` é preservado e o conflito é registado com o `codigo_solicitacao` no payload, disponível para reconciliação manual. Não é necessário nenhum mecanismo de conflito adicional específico para matrícula.
- **Fase 3**: a exceção de login para estudante sem vínculo ativo (`acesso_restrito_financeiro`) está corretamente amarrada, por claim de token dedicada, exclusivamente às duas rotas financeiras (`rotaFinanceiraRestrita`); nenhuma outra rota protegida foi reaberta. O cancelamento em cascata ao anular uma obrigação (`AnularObrigacoesMensalidade` → `CancelCharge`) está corretamente ligado.
- **Fase 4**: o endpoint de busca pública exige corretamente pelo menos 2 campos coincidentes, nunca devolve valor/métodos de pagamento na lista, e tem rate limiting. O valor da matrícula é lido do valor gravado no momento da aprovação (`ValorMatricula` persistido no aggregate), nunca recalculado no momento do pagamento. O cancelamento de uma solicitação pendente de pagamento cancela corretamente qualquer cobrança em aberto associada (`CancelarCobrancaMatriculaAberta`), e a efetivação do vínculo (`efetivarVinculoMatriculaPaga`) é idempotente por verificação de estado antes de criar o `Estudante`.

### Problemas confirmados

**1. A correção de codificação da tarefa 31 nunca foi aplicada.** `internal/finance/mensalidade.go` e `internal/handlers/mensalidade_handlers.go` continuam com exatamente a mesma dupla codificação UTF-8 já reportada — na verdade **mais ocorrências que antes** (23 e 12, respetivamente, eram 20 e 9), porque a Fase 3 acrescentou mais texto a estes mesmos ficheiros sem corrigir o problema já existente. Confirmado de novo por inspeção de bytes (`invÃ¡lido`, `obrigatÃ³rios`, `nÃ£o`, etc. continuam corrompidos).

**2. Bug real de negócio: `resolveConfiguracao` não tem fallback para a primeira configuração de um ano letivo já em curso.** A função (em `internal/finance/mensalidade.go`) resolve o preço de um mês com `WHERE vigente_em <= referencia ORDER BY vigente_em DESC LIMIT 1`. Quando uma academia configura (ou reconfigura) o valor da mensalidade **depois** de o ano letivo já ter começado — que é o cenário normal, não uma exceção — qualquer mês já decorrido daquele ano letivo tem uma data de referência **anterior** à data em que a configuração foi criada (`vigente_em`). A consulta não encontra nenhuma linha, e esse mês simplesmente não resolve nenhum valor — a pendência correspondente não aparece em lado nenhum do sistema. Isto foi confirmado experimentalmente: ao correr os testes de integração da Fase 2 contra um ano letivo já decorrido, praticamente todos os meses "desaparecem" com o erro "mensalidade ... não encontrada".
Isto contraria diretamente a Seção 6 da tarefa 27, que exige que o sistema saiba sempre exatamente o valor a cobrar — a primeira configuração feita por uma academia deve valer retroativamente para qualquer mês do ano letivo corrente que ainda não tinha nenhuma configuração anterior (não há "preço antigo" a proteger quando não existe preço nenhum antes).

**3. Bugs nos testes de integração que impedem a suite de correr (mascaram outros problemas).**
- `seedMensalidadeAcademia` (em `internal/finance/mensalidade_integration_test.go`) não preenche a coluna `nif`, que é `NOT NULL` desde a migration `080_academia_nif_alvara_pdf_errors.sql` — todo teste que usa este helper falha na inserção da academia, antes mesmo de testar qualquer lógica de mensalidade.
- O mesmo helper insere `'null'::jsonb` (um valor JSON nulo, não um `NULL` SQL) na coluna `anos_academicos` para academias de nível médio/superior — isto viola o constraint `check_anos_academicos_nivel` (que exige `anos_academicos IS NULL`, em SQL, para `nivel_escolar='medio'`), porque um valor `'null'::jsonb` **não** satisfaz `IS NULL` em SQL.
- `appyPayMockTransport` (em `internal/finance/appypay_integration_test.go`, partilhado por testes de todas as fases) devolve sempre o mesmo `id` de cobrança fixo (`"provider-charge"`) para qualquer pedido `POST`. Qualquer teste que crie mais de uma cobrança real na mesma execução colide com a restrição de unicidade `ux_financeiro_cobrancas_provider_id`. Isto quebra `TestIntegrationCancelChargeAndLateSuccessConflict` — um teste da própria Fase 1, nunca antes confirmado a passar de facto com banco de dados real.

**4. Ausência de testes de integração para a Fase 4.** Existem apenas testes unitários pequenos (`solicitacao_matricula_handlers_test.go`, `solicitacao_matricula_pagamento_test.go`) que não tocam base de dados. Não há nenhum teste de integração cobrindo o fluxo completo: busca pública, geração de cobrança de matrícula, cancelamento em cascata, e efetivação de vínculo via webhook — exatamente o mesmo tipo de lacuna que já tinha sido identificado (e corrigido) para a Fase 2 na tarefa 31, mas que não foi replicado para a Fase 4.

---

# 1. Corrigir definitivamente a codificação UTF-8

## Objetivo

Concluir o que a tarefa 31, Seção 1, já pedia e que não foi aplicado: eliminar a dupla codificação UTF-8 em `internal/finance/mensalidade.go` e `internal/handlers/mensalidade_handlers.go`, incluindo o texto acrescentado pela Fase 3 a estes mesmos ficheiros.

## Escopo obrigatório

### 1.1 Corrigir todos os literais afetados

Reaplicar exatamente a transformação já documentada na tarefa 31: `texto_corrompido.encode('latin-1').decode('utf-8')` recupera o texto original em português para cada string afetada. Reveja **todo** o ficheiro, não apenas as strings já identificadas anteriormente — a Fase 3 pode ter introduzido novas ocorrências do mesmo problema.

### 1.2 Confirmar que não sobra nenhuma ocorrência

`grep -c "Ã§\|Ã£\|Ã©\|Ã­\|Ã³\|Ã¡\|Ã‰\|Ã‡" internal/finance/mensalidade.go internal/handlers/mensalidade_handlers.go` deve devolver `0` para ambos.

### 1.3 Testes obrigatórios

1. `go build ./...` continua a compilar sem erros;
2. inspeção confirmando que pelo menos 5 mensagens de erro distintas (incluindo as introduzidas pela Fase 3, ex.: relacionadas a `mes_inicio`, autorização, formato de mês/ano) mostram texto correto em português.

---

# 2. Corrigir a resolução histórica de preço para a primeira configuração de um ano letivo em curso

## Objetivo

Garantir que a primeira configuração de mensalidade feita por uma academia para um `(nível/ano[/curso])`, mesmo quando o ano letivo já está em curso, se aplica retroativamente a qualquer mês desse ano letivo que ainda não tinha nenhuma configuração anterior — preservando ao mesmo tempo a regra já correta de que mudanças **posteriores** só afetam meses futuros.

## Regra de negócio

- Ao resolver o preço para a data de referência de um mês: se existir uma versão de configuração com `vigente_em <= referencia`, usar a mais recente dessas (comportamento já correto, não alterar). Se **não** existir nenhuma versão com `vigente_em <= referencia` (ou seja, a referência é anterior a qualquer configuração já feita), usar a versão **mais antiga** disponível para aquele `(academia, nível/ano[/curso])`, em vez de não devolver nenhum valor. Isto reflete que a configuração mais antiga é, por definição, "o preço desde sempre, tanto quanto o sistema sabe" para esse ano/curso — não há nenhum preço anterior a proteger.
- Isto não deve mudar em nada o comportamento já correto para quando **existe** uma versão anterior à referência — a regra de "mudança só afeta meses futuros" continua exatamente como está.

## Escopo obrigatório

### 2.1 Corrigir `resolveConfiguracao`

Ajustar a consulta (ou a função) para implementar o fallback descrito acima. Uma forma direta: preferir a versão mais recente com `vigente_em <= referencia`; na ausência de qualquer uma, cair para a versão de menor `vigente_em` disponível. Verifique se o mesmo padrão de resolução (`vigente_em <=` sem fallback) existe em algum outro ponto de `internal/finance/mensalidade.go` além de `resolveConfiguracao` e corrija também, se for o caso.

### 2.2 Testes obrigatórios

1. academia configura mensalidade pela primeira vez, num ano letivo já com 2 meses decorridos: os 2 meses já decorridos resolvem o valor recém-configurado (não ficam sem valor);
2. a mesma academia reconfigura o valor mais tarde, ainda dentro do mesmo ano letivo: os meses já decorridos **antes** da reconfiguração continuam a resolver o primeiro valor (comportamento já correto, confirmar que não regride); meses com data de referência **depois** da reconfiguração resolvem o novo valor;
3. reexecutar o teste de integração da tarefa 27 (Seção 6.3.4 — "estudante paga, já depois da mudança de preço, um mês antigo ainda pendente") e confirmar que continua a passar sem regressão.

---

# 3. Corrigir os testes de integração para que a suite corra de verdade

## Objetivo

Eliminar os três problemas de infraestrutura de teste identificados, para que a suite de integração do módulo financeiro (Fases 1 a 4) possa de facto ser executada e dar sinal fiável.

## Escopo obrigatório

### 3.1 Corrigir `seedMensalidadeAcademia`

Preencher a coluna `nif` com um valor válido (10 dígitos, único por chamada — ex.: derivado de um UUID gerado no momento) em todo `INSERT INTO projection_academias` feito por este helper.

### 3.2 Corrigir o valor de `anos_academicos` para médio/superior

Quando `nivel_escolar` for `medio` (ou quando a academia for de nível `superior`), inserir um `NULL` SQL genuíno na coluna `anos_academicos` — não a string `'null'` convertida para `jsonb`. Confirme com `SELECT anos_academicos IS NULL FROM projection_academias WHERE codigo_academia=...` que o valor inserido satisfaz `IS NULL`.

### 3.3 Corrigir `appyPayMockTransport`

Fazer o mock devolver um `id` de cobrança **único por pedido** (ex.: derivado do `merchantTransactionId` recebido no corpo do pedido, ou de um contador), em vez do valor fixo `"provider-charge"`, para que testes que criam mais de uma cobrança real na mesma execução não colidam com `ux_financeiro_cobrancas_provider_id`.

### 3.4 Correr a suite completa contra PostgreSQL real e corrigir o que aparecer

Depois de 3.1–3.3, corra `RUN_POSTGRES_INTEGRATION=1 DATABASE_URL=<postgres local> go test ./... -run TestIntegration -v` contra uma base de dados limpa. Isto deve destravar os testes que hoje falham só por causa dos problemas de seed acima — incluindo, no mínimo, `TestIntegrationMensalidadeMesInicioEValidadePorAno` e `TestIntegrationMensalidadeAnularEReativar`. **Não presuma que ficam corretos automaticamente**: se, depois de corrigir 3.1–3.3 e a Seção 2, algum destes (ou qualquer outro) continuar a falhar, diagnostique a causa raiz e corrija — pode ser um bug de teste adicional ou um bug de lógica ainda não identificado nesta auditoria. O critério de aceite desta tarefa exige a suite completa a passar de facto, não apenas os testes já nomeados nesta auditoria.

### 3.5 Testes obrigatórios

1. `RUN_POSTGRES_INTEGRATION=1 go test ./internal/finance/... ./internal/handlers/... -run TestIntegration` termina com `ok` em todos os pacotes, sem nenhum `FAIL`;
2. rodar a suite duas vezes seguidas contra a mesma base de dados recriada do zero entre as execuções, para confirmar que não há dependência de estado deixado por execuções anteriores.

---

# 4. Cobertura de teste de integração para a Fase 4 (matrícula)

## Objetivo

Cobrir por teste de integração o fluxo completo de cobrança de matrícula introduzido pela Fase 4, hoje verificado apenas por testes unitários que não tocam base de dados.

## Escopo obrigatório

### 4.1 Novo ficheiro de teste de integração (ex.: `internal/handlers/solicitacao_matricula_pagamento_integration_test.go` ou equivalente em `internal/finance`)

Cobrir, no mínimo:

1. aprovação de solicitação em academia com matrícula configurada: solicitação fica em `aprovada_pendente_pagamento_matricula`, sem nenhum `Estudante` criado;
2. busca pública com 2 campos coincidentes devolve a solicitação com `codigo_solicitacao`, sem valor nem métodos de pagamento;
3. busca pública com apenas 1 campo, ou com campos que não coincidem em conjunto, não devolve resultado;
4. `IniciarPagamentoMatricula` com método habilitado gera a cobrança com o valor fixado na aprovação;
5. tentar gerar uma segunda cobrança enquanto já existe uma em aberto → rejeitado;
6. webhook de sucesso para a cobrança de matrícula efetiva o vínculo (`CriarComVinculo`) exatamente uma vez, mesmo com reentrega do mesmo webhook (idempotência);
7. cancelamento da solicitação pendente de pagamento cancela a cobrança em aberto associada;
8. webhook de sucesso chegando **depois** de a solicitação já ter sido cancelada não cria vínculo, e o evento `CobrancaAppyPayConflitoPosCancelamento` fica registado na cobrança correspondente, com `codigo_solicitacao` no payload.

### 4.2 Testes obrigatórios (resumo)

Os 8 cenários de 4.1 devem existir como testes de integração e passar, corridos contra PostgreSQL real, seguindo o mesmo padrão de setup já usado pelos demais testes de integração do módulo financeiro.

---

# Fora de escopo

- Qualquer redesenho da lógica de negócio das Fases 1 a 4 além das correções pontuais descritas acima — a arquitetura já está correta.
- Migração de dados já afetados pelo bug da Seção 2 (não há dados em produção ainda).
- Início de qualquer fase ou funcionalidade nova além da correção deste módulo.

# Riscos e mitigações

| Risco | Mitigação |
| --- | --- |
| Corrigir `resolveConfiguracao` (Seção 2) introduzir uma regressão na regra já correta de "mudança só afeta meses futuros" | Teste de regressão explícito (2.2.2, 2.2.3) reexecutando cenários já cobertos antes da correção |
| Testes novos ou corrigidos (Seções 3 e 4) dependerem de infraestrutura de banco de dados não disponível no ambiente do Codex | Seguir exatamente o padrão de setup já usado em `appypay_integration_test.go`, que já resolve isso no projeto; se PostgreSQL não estiver disponível no ambiente de execução, documentar isso explicitamente em vez de presumir que os testes passam |
| Corrigir a codificação (Seção 1) acidentalmente alterar alguma string além do necessário | Aplicar a transformação apenas às strings identificadas como corrompidas (`grep` confirma antes/depois) |

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `grep` confirmar zero ocorrências de codificação corrompida nos dois ficheiros da Fase 2;
2. a primeira configuração de mensalidade de uma academia, feita a meio de um ano letivo já em curso, resolver corretamente o valor para os meses já decorridos desse ano letivo, sem regressão no comportamento de mudanças de preço já corretamente tratado;
3. `RUN_POSTGRES_INTEGRATION=1 go test ./... -run TestIntegration` passar por completo, sem nenhum `FAIL`, corrido de facto (não apenas presumido) contra PostgreSQL real, duas vezes seguidas a partir de uma base de dados limpa;
4. os 8 cenários de teste de integração da Seção 4 existirem e passarem;
5. nenhuma mudança de comportamento for introduzida nas partes já confirmadas corretas (Fase 1, autorização da Fase 2, exceção de login da Fase 3, valor fixado na aprovação da Fase 4).

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Correção final do módulo de pagamentos — Fases 1 a 4 (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
