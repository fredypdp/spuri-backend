---
criado: 2026-08-14 22:46
origem: depuração pós-implementação da tarefa 33 (Fases 1-4 do módulo de pagamentos), feita pelo Spuri (Claude como orquestrador/auditor, verificação com Go 1.24 + PostgreSQL real, `go test ./internal/finance/... -run TestIntegration` corrido de facto contra banco real, várias vezes, em isolamento)
status: feito
depende_de: nenhuma (a tarefa 33 já foi implementada; esta tarefa corrige regressões e lacunas que a tarefa 33 não resolveu ou resolveu apenas parcialmente)
---

# Correção pós-implementação da tarefa 33 (feito)

## Prompt recomendado para executar a atualização

A tarefa 33 foi implementada e duas das suas correções (Secção 1 — codificação UTF-8, e Secção 2 — fallback de `resolveConfiguracao`) estão corretas por leitura e por teste direto. Mas uma auditoria com ambiente real (Go 1.24 + PostgreSQL local, não apenas leitura de código) revelou que a suite de testes de integração do módulo financeiro **continua a falhar em massa** — 11 de 15 testes em `internal/finance` — e que pelo menos dois desses problemas são **bugs de produção graves**, não apenas problemas de teste. Corrija pela ordem das secções abaixo — a Secção 3 (bug de meses do ano letivo) é a mais grave e deve ser a prioridade máxima, porque afeta todo estudante, em toda academia, sempre.

**Importante:** confirme cada correção corrndo de facto `go test ./internal/finance/... -run TestIntegration -v` contra um PostgreSQL real (`RUN_POSTGRES_INTEGRATION=1`), **usando uma base de dados recém-criada a cada execução completa da suite** (`DROP DATABASE`/`CREATE DATABASE` antes de correr, ou equivalente). Esta auditoria confirmou que os testes de integração deste módulo não são isolados entre si nem entre execuções — dados de uma execução ficam na base de dados e podem mascarar ou causar falhas espúrias na próxima. Corra a suite completa **duas vezes seguidas sobre a mesma base de dados sem recriá-la**, para confirmar que as correções da Secção 4 tornam a suite verdadeiramente repetível.

## Contexto (metodologia e achados da auditoria)

Foi montado um ambiente de verificação real: `go build ./...` e `go vet ./...` completos (sem erros), e os testes de integração foram corridos repetidamente contra PostgreSQL local, incluindo em execuções isoladas (`-run <TesteEspecífico>`) com bases de dados recém-criadas, para eliminar qualquer ambiguidade sobre a causa de cada falha. Cada bug abaixo foi confirmado com evidência direta (consulta SQL manual reproduzindo o valor exato gravado, ou instrumentação temporária do código com `fmt.Printf`, removida antes de concluir a auditoria).

### O que está confirmado correto (não mexer)

- **Secção 1 da tarefa 33** (codificação UTF-8): confirmado por `grep -c` — zero ocorrências em `internal/finance/mensalidade.go` e `internal/handlers/mensalidade_handlers.go`.
- **Secção 2 da tarefa 33** (`resolveConfiguracao`, linha ~493 de `mensalidade.go`): a consulta SQL com fallback (`ORDER BY CASE WHEN vigente_em <= $5 THEN 0 ELSE 1 END, ...`) está correta e foi confirmada por consulta manual direta ao PostgreSQL a devolver a configuração mais antiga disponível quando não há nenhuma anterior à data de referência. Este bug específico (o que a tarefa 33 foi desenhada para resolver) está mesmo resolvido — o que falha agora é um bug **diferente e anterior**, nunca antes detectado porque a suite nunca tinha corrido de facto até ao fim.

### Problemas confirmados

**1. 🔴 CRÍTICO — coluna `status` de `projection_solicitacoes_matricula` é `VARCHAR(20)`, mas a tarefa 33 (Fase 4) introduziu um valor de status com 37 caracteres. Isto quebra em produção, não só em teste.**

A migration `067_solicitacoes_matricula_mega.sql` define `status VARCHAR(20) NOT NULL CHECK (...)`. A migration `106_financeiro_matricula.sql` (da tarefa 33) alarga corretamente a `CHECK CONSTRAINT` para aceitar os novos valores `'cancelada'` e `'aprovada_pendente_pagamento_matricula'`, mas **nunca alarga o tipo da coluna em si**. `'aprovada_pendente_pagamento_matricula'` tem 37 caracteres — excede o limite de 20 do `VARCHAR`. Confirmado diretamente no PostgreSQL:

```
column_name | data_type          | character_maximum_length
status      | character varying  | 20
```

Isto não é um problema de teste: `internal/projections/solicitacao_matricula_projection.go:178` executa exatamente `UPDATE projection_solicitacoes_matricula SET status='aprovada_pendente_pagamento_matricula', ...` — este é o código de produção que aplica o evento de aprovação com taxa de matrícula. **Toda vez que uma solicitação de matrícula com taxa é aprovada em produção, a projeção falha com `pq: value too long for type character varying(20)`**, deixando o ledger com o evento gravado (a fonte da verdade) mas a projeção de leitura sem o refletir — um estado inconsistente entre o ledger e a projeção, exatamente o tipo de falha que o design event-sourced deste sistema deveria evitar.

**2. 🔴 CRÍTICO — `ListMensalidades` descarta todos os meses de Setembro a Dezembro sempre que `mes_fim_cobranca` é 6 ou 7 (ou seja, sempre — é o único valor permitido). Nenhum estudante, em nenhuma academia, jamais vê uma mensalidade de Set/Out/Nov/Dez.**

Em `internal/finance/mensalidade.go`, dentro de `ListMensalidades`:

```go
if ref.Month > cfg.MesFimCobranca {
    continue
}
```

`cfg.MesFimCobranca` só pode ser `6` ou `7` (validado em `validateConfiguracaoMensalidade`, linha ~364: `if in.MesFimCobranca != 6 && in.MesFimCobranca != 7`). Os meses de referência de Setembro a Dezembro do ano letivo (a primeira metade, gerados por `mesesAnoLetivo`) têm `ref.Month` igual a `9, 10, 11, 12`. Como `9, 10, 11, 12` são sempre numericamente maiores do que `6` ou `7`, esta condição é **sempre verdadeira** para esses meses, e eles são sempre descartados (`continue`) — independentemente de haver ou não configuração de mensalidade válida para eles.

O bug é a comparação direta de números de mês de calendário sem considerar que o ano letivo "dá a volta" ao virar do ano: os meses 9-12 são o **início** do período de cobrança (não podem estar "depois do fim"), e só os meses 1-7 do ano civil seguinte é que devem de facto ser comparados com `mes_fim_cobranca`. Confirmado por instrumentação direta do código (removida depois): para um vínculo válido com configuração correta, `resolveConfiguracao` devolve `cfg.Valor=1000, MesFimCobranca=7` sem erro para Setembro — mas a linha seguinte descarta o resultado antes de chegar a `estadoObrigacao` e a `result = append(...)`.

Isto explica, sozinho, a totalidade das falhas "mensalidade .../9 não encontrada" em `TestIntegrationMensalidadeResolvePrecoHistorico`, `...PrimeiraConfiguracaoRetroageSemReescreverHistorico`, `...MantemAnoAcademicoHistorico`, `...MantemCursoHistorico`, `...MantemAcademiaHistoricaAposTransferencia`, `...ConsultaRespeitaAcademia`, e a falha "mês fora do período de mensalidade configurado" em `TestIntegrationMensalidadeAnularEReativar` (que chama `mesDevido` → `ListMensalidades` internamente para o mês 9).

**3. 🔴 CRÍTICO — mesma classe de bug em `mesInicioEfetivo`: um `mes_inicio` legítimo entre Janeiro e Julho é rejeitado como "inconsistente" por comparação numérica direta com o mês natural (9 ou 10).**

Em `internal/finance/mensalidade.go`, dentro de `mesInicioEfetivo`:

```go
if mes < natural {
    return 0, errors.New("configuração de mes_inicio inconsistente")
}
```

Um `mes_inicio` guardado como `1` (Janeiro) para representar "esta academia só começou a cobrar mensalidade a partir de Janeiro daquele ano letivo" é um valor perfeitamente válido e posterior a Setembro **em termos de ano letivo** — mas `1 < 9` é verdadeiro em termos de número de mês de calendário, disparando incorretamente o erro "configuração de mes_inicio inconsistente". Isto derruba `TestIntegrationMensalidadeMesInicioEValidadePorAno`, que depende exatamente deste cenário (uma academia com um `mes_inicio` de Janeiro guardado para um ano letivo anterior, coexistindo com o ano letivo atual).

Este bug e o da Secção 2 acima têm a mesma causa raiz: comparação de números de mês de calendário sem primeiro converter para uma posição relativa ao ano letivo (onde Setembro é a 1ª posição, Outubro a 2ª, ..., Dezembro a 4ª, Janeiro a 5ª, ..., Julho a 11ª). Corrija as duas com a mesma função auxiliar, para garantir que ambas usam exatamente a mesma noção de ordem.

**4. 🟠 ALTO (só testes, mas quebra a suite de Fase 1/3/4 sempre que mais de uma cobrança é cancelada/consultada no mesmo teste) — `appyPayMockTransport.RoundTrip`, ramo `GET`, continua a devolver o `id` literal `"provider-charge"` em vez de um valor único.**

A tarefa 33 (Secção 3.3) corrigiu os ramos `POST` (criação de cobrança) e `qr-codes` do mock em `internal/finance/appypay_integration_test.go` para usarem `t.providerID(kind)` (um contador atómico por instância do mock), mas **esqueceu o ramo `GET`** (linha 30-31), usado para consultar o estado atual de uma cobrança junto do AppyPay — chamado internamente por `CancelCharge` (antes de cancelar, para confirmar que a cobrança não foi paga) e por `ConsultCharge`:

```go
case req.Method == http.MethodGet:
    body = `{"id":"provider-charge","status":"` + t.status + `"}`
```

Como a projeção financeira (`internal/projections/financeiro_projection.go:182`) faz `INSERT ... ON CONFLICT (id) DO UPDATE SET provider_charge_id=COALESCE(EXCLUDED.provider_charge_id, financeiro_cobrancas.provider_charge_id)`, qualquer resposta de consulta (`GET`) do mock sobrescreve o `provider_charge_id` da cobrança consultada com o literal fixo `"provider-charge"`. Confirmado experimentalmente numa execução isolada de `TestIntegrationCancelChargeAndLateSuccessConflict` contra base de dados recém-criada: depois do primeiro cancelamento (da cobrança `spuri`), a linha correspondente em `financeiro_cobrancas` já mostra `provider_charge_id = 'provider-charge'` (não `'provider-charge-1'`, que era o valor original da criação). Ao cancelar a segunda cobrança (da academia `CANCELACA1`) no mesmo teste, o mesmo mecanismo tenta gravar o mesmo literal `'provider-charge'` para uma linha diferente, e colide com a restrição `ux_financeiro_cobrancas_provider_id`.

Isto não é um bug de produção (o AppyPay real devolveria sempre o mesmo ID, correto, do lado da cobrança consultada, não um literal fixo), mas impede a suite de correr até ao fim, mascarando a validação de outras partes da Fase 1.

**5. 🟡 MÉDIO — a suite de integração não é isolada entre testes nem entre execuções, o que por si só já causa falhas espúrias adicionais (para além do problema 4).**

Nenhum teste de integração deste pacote limpa dados entre si nem usa transação com `ROLLBACK`; todos escrevem diretamente na mesma base de dados persistente através de `client.DB().Exec`. Isto combinado com o problema 4 causa colisões adicionais quando a suite completa corre mais de uma vez sobre a mesma base sem recriá-la (confirmado: a mesma suite, corrida uma segunda vez sem recriar a base, falhou num teste diferente e mais cedo do que na primeira execução, por colisão com dados residuais da execução anterior). Mesmo depois de corrigido o problema 4, isto continuará a ser uma fragilidade estrutural da suite — mas corrigir isso de forma completa (isolamento total por teste) é maior do que o âmbito desta tarefa; o objetivo mínimo aqui é apenas garantir que a suite é repetível quando corrida contra a mesma base de dados sem ser recriada entre execuções, o que a correção da Secção 4 já garante na prática (nenhuma colisão de `provider_charge_id` deverá voltar a acontecer depois de resolvido o ramo `GET` do mock).

**6. 🟡 MÉDIO — a Secção 4 da tarefa 33 (testes de integração da Fase 4) ficou incompleta: falta cobertura do fluxo HTTP completo de efetivação de vínculo via webhook.**

Existem testes de integração para: duplicidade/cascata de cancelamento de cobrança de matrícula, webhook tardio mantendo cancelamento, e busca pública com dois campos. Mas **nenhum teste chama `ReceberWebhookAppyPay` (o handler HTTP real, em `internal/handlers/financeiro_handlers.go:231`) para confirmar a cadeia completa**: webhook autenticado → `AcceptWebhook` → `CodigoSolicitacaoDaCobranca` → `efetivarVinculoMatriculaPaga` (`internal/handlers/solicitacao_matricula_handlers.go:501`) → `Estudante.CriarComVinculo`. Os testes de integração existentes chamam `service.AcceptWebhook` diretamente (camada de serviço), nunca passando pelo handler HTTP, que é o único ponto do sistema onde a efetivação do vínculo de facto acontece. Isto significa que a criação real do `Estudante` a partir do pagamento da matrícula nunca foi testada com banco de dados real.

---

# 1. Corrigir a coluna `status` de `projection_solicitacoes_matricula`

## Objetivo

Alargar a coluna `status` para caber `'aprovada_pendente_pagamento_matricula'` (37 caracteres) e qualquer valor futuro razoável, sem quebrar a `CHECK CONSTRAINT` já existente.

## Escopo obrigatório

### 1.1 Nova migration

Crie uma nova migration (próximo número sequencial disponível em `migrations/`) que faça:

```sql
ALTER TABLE projection_solicitacoes_matricula
    ALTER COLUMN status TYPE VARCHAR(50);
```

Use `VARCHAR(50)` (não um valor mais apertado) para dar margem a valores futuros sem repetir este problema. Não remova nem recrie a `CHECK CONSTRAINT` existente — ela já está correta desde a migration 106.

### 1.2 Testes obrigatórios

1. `go build ./...` continua a compilar sem erros;
2. Rode as migrations do zero (`client.RunMigrations()` contra uma base nova) e confirme, por consulta a `information_schema.columns`, que `status` de `projection_solicitacoes_matricula` tem `character_maximum_length = 50`;
3. `TestIntegrationMatriculaPagamentoFixaValorImpedeDuplicidadeECancelaEmCascata` e `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito` (em `internal/finance/appypay_integration_test.go`) devem passar de `seedMatriculaPendente` sem erro `value too long`.

## Fora de escopo

Não alterar `bilhete_identidade`, `bilhete_identidade_encarregado`, `codigo_estudante_gerado` ou qualquer outra coluna — já foram confirmadas com largura suficiente para os dados reais.

---

# 2. Corrigir a comparação de meses do ano letivo (bug duplo: `ListMensalidades` e `mesInicioEfetivo`)

## Objetivo

Fazer com que qualquer comparação entre números de mês dentro do módulo de mensalidade use a posição do mês **relativa ao ano letivo** (Setembro = 1ª posição, ..., Dezembro = 4ª, Janeiro = 5ª, ..., Julho = 11ª — ou 2ª a 10ª quando o nível é `superior`, que começa em Outubro), nunca o número de mês de calendário bruto.

## Escopo obrigatório

### 2.1 Criar uma função auxiliar de posição relativa ao ano letivo

Em `internal/finance/mensalidade.go`, adicione uma função como:

```go
// posicaoNoAnoLetivo devolve a posição ordinal do mês dentro do ano letivo
// (1 = primeiro mês cobrável, crescente), para que meses de calendário de
// anos civis diferentes dentro do mesmo ano letivo sejam comparáveis.
// natural é o mês de calendário em que o ano letivo começa (9 para os
// níveis fundamental/médio, 10 para superior).
func posicaoNoAnoLetivo(mes, natural int) int {
    if mes >= natural {
        return mes - natural + 1
    }
    return mes + (12 - natural) + 1
}
```

Confirme o comportamento com os casos usados nos testes existentes: `posicaoNoAnoLetivo(9, 9) == 1`, `posicaoNoAnoLetivo(12, 9) == 4`, `posicaoNoAnoLetivo(1, 9) == 5`, `posicaoNoAnoLetivo(7, 9) == 11`.

### 2.2 Corrigir `ListMensalidades`

Substitua:

```go
if ref.Month > cfg.MesFimCobranca {
    continue
}
```

por uma comparação que use `posicaoNoAnoLetivo` tanto para `ref.Month` como para `cfg.MesFimCobranca`, usando o mesmo `natural` já calculado em `mesInicioEfetivo` para este vínculo (ou recalculado localmente a partir de `v.Nivel`, do mesmo modo que `mesInicioEfetivo` já faz). Ou seja, o mês de referência só deve ser descartado quando a sua posição relativa ao ano letivo for **maior** do que a posição relativa de `cfg.MesFimCobranca`.

### 2.3 Corrigir `mesInicioEfetivo`

Substitua:

```go
if mes < natural {
    return 0, errors.New("configuração de mes_inicio inconsistente")
}
```

pela mesma lógica de posição relativa: um `mes_inicio` só é inconsistente se a sua posição relativa ao ano letivo for **anterior** à posição do próprio `natural` (que é sempre posição 1) — ou seja, na prática, isto só pode falhar se `mes` não for um mês de calendário válido (já coberto por `mesValido`) ou se, por alguma razão futura, o `natural` mudar de definição. Reveja se esta validação ainda faz sentido depois da correção, ou se deve ser removida/simplificada — o objetivo é que qualquer mês entre `natural` e o mês imediatamente anterior a `natural` no ano seguinte (ou seja, qualquer mês de calendário válido) seja aceite como `mes_inicio`, desde que não seja posterior ao `mes_fim_cobranca` já configurado (essa segunda validação, em `validateMesInicioCobranca`, já está correta e não precisa de alteração).

### 2.4 Reveja se `validateConfiguracaoMensalidade` ou outro ponto do ficheiro tem o mesmo problema

Procure por outras comparações diretas de `mes`/`Month`/`MesFimCobranca`/`MesInicio` em `internal/finance/mensalidade.go` (e em `internal/handlers/mensalidade_handlers.go`, se aplicável) que possam sofrer do mesmo problema de comparação numérica sem conversão para posição relativa ao ano letivo. Corrija qualquer ocorrência encontrada com a mesma função auxiliar.

## Testes obrigatórios

1. `go build ./...` e `go vet ./...` sem erros;
2. `go test ./internal/finance/... -run TestIntegrationMensalidade -v` contra PostgreSQL real: todos os testes de `TestIntegrationMensalidade*` devem passar, incluindo `TestIntegrationMensalidadeResolvePrecoHistorico`, `TestIntegrationMensalidadePrimeiraConfiguracaoRetroageSemReescreverHistorico`, `TestIntegrationMensalidadeMesInicioEValidadePorAno`, `TestIntegrationMensalidadeAnularEReativar`, `TestIntegrationMensalidadeConsultaRespeitaAcademia`, e quaisquer outros no mesmo padrão;
3. Confirme manualmente (por exemplo com um teste temporário ou consulta direta) que, para uma academia com `mes_fim_cobranca=7`, `ListMensalidades` devolve os 11 meses esperados (Set-Dez do primeiro ano civil, Jan-Jul do segundo), não apenas os de Jan-Jul.

## Fora de escopo

Não alterar `mesesAnoLetivo` (a geração da lista de meses candidatos já está correta) nem `resolveConfiguracao` (já corrigido na tarefa 33, Secção 2).

---

# 3. Corrigir o mock `appyPayMockTransport` (ramo `GET`)

## Objetivo

Fazer o ramo `GET` do mock devolver um `id` de cobrança consistente com o valor já atribuído à cobrança sendo consultada, em vez de um literal fixo, para que consultas/cancelamentos de múltiplas cobranças no mesmo teste não colidam entre si.

## Escopo obrigatório

### 3.1 Corrigir `RoundTrip`

Em `internal/finance/appypay_integration_test.go`, o ramo `GET` deve devolver um `id` derivado do próprio pedido (por exemplo, extraído do caminho da URL, que normalmente contém o ID/merchant transaction ID da cobrança sendo consultada), em vez do literal `"provider-charge"`. Se não houver forma simples de extrair o ID original do pedido `GET` (dependendo de como `appypay.go` monta a URL de consulta), alternativamente ajuste o mock para nunca reatribuir `provider_charge_id` via `GET` — ou seja, mantenha `id` vazio/omitido na resposta do `GET` quando isso não quebrar a lógica de negócio que lê apenas o campo `status` dessa resposta (confirme em `appypay.go` se o `id` da resposta de consulta é de facto usado para atualizar `provider_charge_id`, ou se poderia/deveria ser ignorado nesse caso — corrija o lado que fizer mais sentido depois de ler `appypay.go`).

### 3.2 Testes obrigatórios

1. `go test ./internal/finance/... -run TestIntegrationCancelChargeAndLateSuccessConflict -v` contra uma base de dados **recém-criada** deve passar sem nenhum erro `ux_financeiro_cobrancas_provider_id`;
2. Corra a suite completa (`go test ./internal/finance/... -run TestIntegration -v`) **duas vezes seguidas sem recriar a base de dados entre as execuções** — a segunda execução deve ter exatamente as mesmas passagens/falhas que a primeira relativamente a este problema (nenhuma colisão nova de `provider_charge_id`).

## Fora de escopo

Não alterar `internal/finance/appypay.go` a menos que a investigação do passo 3.1 confirme que o problema também existe do lado do código de produção (não deveria, mas confirme).

---

# 4. Adicionar teste de integração para o fluxo HTTP completo de efetivação de vínculo via webhook

## Objetivo

Cobrir o caminho que nenhum teste atual cobre: `POST` no endpoint real do webhook AppyPay (`ReceberWebhookAppyPay`) → autenticação → `AcceptWebhook` → `efetivarVinculoMatriculaPaga` → criação real do `Estudante`.

## Escopo obrigatório

### 4.1 Novo teste de integração

Em `internal/handlers/` (o pacote correto para testar o handler HTTP, seguindo o padrão já usado em `financeiro_handlers_integration_test.go`), adicione um teste que:

1. Configure uma credencial AppyPay de academia (via `service.ConfigureCredential`, como já é feito nos testes existentes);
2. Semeie uma solicitação de matrícula já aprovada com pagamento pendente (estado `aprovada_pendente_pagamento_matricula`, reaproveitando o padrão de `seedMatriculaPendente` de `internal/finance/appypay_integration_test.go`, ou criando um equivalente neste pacote se houver problema de import cíclico);
3. Inicie o pagamento da matrícula (`IniciarPagamentoMatricula`) para obter uma cobrança em aberto;
4. Monte um pedido HTTP real (`httptest.NewRequest`/`httptest.NewRecorder`, ou o padrão já usado nos testes de handlers deste projeto) simulando o webhook do AppyPay a confirmar sucesso do pagamento, com a assinatura/cabeçalho de autenticação corretos para a credencial configurada;
5. Envie este pedido através do `gin.Engine`/router real (não chame `AcceptWebhook` diretamente) para `ReceberWebhookAppyPay`;
6. Confirme, por consulta direta à base de dados, que: (a) o status HTTP da resposta é 200; (b) a solicitação de matrícula muda de estado para o estado final esperado; (c) um `Estudante` foi de facto criado com o vínculo correto à academia e à turma/curso esperados; (d) a operação é idempotente — reenviar o mesmo webhook (mesmo `event_id`) não duplica o `Estudante` nem gera um segundo evento de efetivação.

### 4.2 Testes obrigatórios

1. `go build ./...` sem erros;
2. O novo teste passa contra PostgreSQL real;
3. Rode a suite completa de `internal/handlers/...` e `internal/finance/...` uma vez mais para confirmar que nada foi quebrado.

## Fora de escopo

Não é necessário testar o cenário de aprovação da solicitação (sem taxa de matrícula, fluxo direto) nem duplicar testes já existentes de duplicidade/cancelamento em cascata — esses já estão cobertos.

---

# Checklist de aceitação (todas as secções)

- [ ] `go build ./...` e `go vet ./...` sem erros
- [ ] Nova migration aplicada e `status` de `projection_solicitacoes_matricula` confirmado com `character_maximum_length = 50`
- [ ] `posicaoNoAnoLetivo` (ou equivalente) criada e usada em `ListMensalidades` e `mesInicioEfetivo`
- [ ] Todos os testes `TestIntegrationMensalidade*` em `internal/finance` passam contra PostgreSQL real
- [ ] Todos os testes `TestIntegration*` em `internal/finance/appypay_integration_test.go` passam contra PostgreSQL real, incluindo `TestIntegrationCancelChargeAndLateSuccessConflict`
- [ ] A suite completa de `internal/finance/...` corre duas vezes seguidas sobre a mesma base de dados (sem recriá-la) sem novas falhas de colisão
- [ ] Novo teste de integração HTTP do webhook → efetivação de vínculo → criação de `Estudante` adicionado e a passar
- [ ] Nenhum teste anteriormente confirmado a passar (tarefa 33, Secções 1 e 2) foi quebrado

# Verificações manuais

1. Correr `go test ./... -run TestIntegration -v` (não só `./internal/finance/...`) contra PostgreSQL real, para confirmar que nenhum outro pacote foi afetado pela alteração de `status` para `VARCHAR(50)` ou pela nova migration.
2. Inspecionar manualmente, por consulta SQL, uma execução completa do fluxo de matrícula com taxa (aprovação → pagamento → webhook) para confirmar visualmente que a projeção e o ledger ficam consistentes entre si.

# Riscos e mitigações

- **Risco:** alargar `status` para `VARCHAR(50)` pode mascarar, no futuro, um valor de status ainda maior sem que ninguém note até o `CHECK CONSTRAINT` disparar de forma menos óbvia do que o limite de largura. **Mitigação:** o `CHECK CONSTRAINT` já existente continua a ser a validação de facto; a largura de 50 é apenas margem de segurança, não a validação principal.
- **Risco:** a correção da Secção 2 (comparação de meses) é o núcleo do módulo de mensalidades — qualquer erro aqui tem impacto amplo. **Mitigação:** os testes de integração já existentes (`TestIntegrationMensalidade*`) cobrem exatamente os casos de fronteira necessários (mês natural, mês de fim, retroatividade histórica); não avance para a Secção 3 sem primeiro confirmar que 100% desses testes passam.
- **Risco:** a correção do mock (Secção 3) pode não refletir fielmente o comportamento real do AppyPay em produção. **Mitigação:** o objetivo é apenas destravar a suite de testes; revalidar contra a documentação/comportamento real do AppyPay não faz parte desta tarefa, mas deve ficar registado como pendência caso a Secção 3.1 revele uma ambiguidade genuína sobre o que `appypay.go` faz com o `id` de uma resposta de consulta.
