# Relatório de Verificação Final — Módulos de Notas e Faltas (spuri-backend)

**Repositório verificado:** `fredypdp/spuri-backend` (branch `main`, snapshot mais recente — posterior à segunda rodada de correções, que tratava os pendentes de `verificacao-correcoes-notas-faltas.md`: `TRACE-03`, `UNIQ-02`, `NOVO-01`, `NOVO-02`, `NOVO-03` e cobertura de testes).
**Objetivo:** terceira rodada de verificação. Confere item a item se os 5 pendentes da rodada anterior foram resolvidos, e reavalia a prontidão geral para produção considerando as duas rodadas anteriores como já fechadas.
**Metodologia:** novo clone completo do repositório público, leitura direta do código atual (aggregates, handlers, projections, rotas, testes) linha a linha, sem presumir nada a partir de nomes de função — cada "✅ Resolvido" foi confirmado lendo a implementação e, quando existia, o teste correspondente.

---

## 1. Sumário executivo

| Pendência da rodada anterior | Status agora |
|---|---|
| `UNIQ-02` — comentário incorreto em `estudante_falta.go` | ✅ Resolvido |
| `TRACE-03` — auditoria de eventos restrita a admin | ✅ Resolvido |
| `NOVO-01` — `ListarNotas`/`ListarFaltas` sem campos de auditoria/correção | ✅ Resolvido (e com filtro `?corrigido=` como bônus) |
| `NOVO-02` — `CorrigirFalta` sem teto de quantidade no aggregate | ✅ Resolvido |
| `NOVO-03` — mesma lacuna também em `RegistrarFalta` | ✅ Resolvido |
| Cobertura de teste (concorrência, HTTP e2e, avaliação final recalculada, `ValidateJWTConfig`, rebuild ponta a ponta) | ⚠️ Parcialmente resolvido |
| Documentação da API (`Documentação da API.md`) | ❌ Não atualizada |

**Veredito resumido:** todos os 5 itens de código pendentes da rodada anterior foram corretamente implementados — não há mais nenhum item de segurança, gravação, unicidade ou rastreabilidade em aberto no código em si. Restam dois tipos de pendência, nenhum deles de lógica: (1) **cobertura de testes automatizados**, detalhada na seção 2.7; e (2) **a documentação da API não foi atualizada** para refletir a superfície nova criada pelas três rodadas de correção — dois endpoints `PATCH` inteiros, um endpoint de auditoria, e novos campos/filtro em quatro endpoints já documentados —, detalhada na seção 2.8.

---

## 2. Verificação item a item

### UNIQ-02 — ✅ RESOLVIDO — Comentário de `applyFaltasRegistradas`

**Evidência:** `internal/domain/aggregates/estudante_falta.go`, comentário atual:

```go
// applyFaltasRegistradas mantém e.FaltasRegistradasPorChave para que
// RegistrarFalta detecte duplicatas por estudante, academia, ano letivo, data
// e matéria antes de emitir o evento. A projeção reforça a mesma unicidade pela
// constraint uq_falta_unica; ver migration 053_restaurar_unicidade_faltas.sql.
func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
```

Agora reflete corretamente o comportamento real do código e do schema. Sem risco residual de regressão por má leitura do comentário.

---

### TRACE-03 — ✅ RESOLVIDO — Auditoria de eventos para estudante/academia

**Evidência:** `internal/handlers/estudante_handlers.go`.

Foi criada a função `podeAuditarEstudante(c, estudante)` que resolve corretamente os três papéis:
- `admin` → acesso irrestrito;
- `estudante` → apenas o próprio (`userID == estudante.ID`);
- `academia` → apenas se o estudante pertence à academia autenticada (`estudante.CodigoAcademia == academia.CodigoAcademia`).

`GetEventosEstudante` (`GET /eventos-estudante/:codigo`) e `GetEventoAuditoria` (`GET /eventos/:event_id`) agora usam essa checagem em vez do bloqueio antigo "apenas admin". `GetEventoAuditoria` corretamente devolve **404** (não 403) quando o solicitante não tem posse — exatamente o comportamento certo para não revelar a existência de um evento a quem não deveria nem saber que ele existe.

Ambas as rotas foram movidas para o grupo `protected` (qualquer autenticado, com a checagem de posse feita dentro do handler), o que é o padrão correto.

**Teste:** `internal/handlers/auditoria_autorizacao_test.go` — `TestPodeAuditarEstudanteRestringeEstudanteAoProprioAggregate` e `TestPodeAuditarEstudantePermiteAdmin` cobrem a lógica central de `podeAuditarEstudante`.

**Pendência residual, baixa prioridade:**
1. Não há teste cobrindo o ramo `academia` de `podeAuditarEstudante` (só `estudante` e `admin` foram testados diretamente).
2. Não há teste HTTP (via `httptest`, batendo na rota de verdade) confirmando que `GetEventoAuditoria` devolve 404 — só a função interna foi testada isoladamente.
3. `admin.GET("/eventos/:event_id", handlers.GetEventoAuditoria)` (`cmd/server/main.go`, dentro do grupo `/admin`) ficou registrada em duplicidade com `protected.GET("/eventos/:event_id", handlers.GetEventoAuditoria)` — a rota sob `/admin` é redundante agora (o handler já cobre admin via `podeAuditarEstudante`) e pode ser removida para reduzir superfície de rota. Cosmético, sem impacto funcional ou de segurança.

---

### NOVO-01 — ✅ RESOLVIDO — Campos de auditoria/correção ausentes em `ListarNotas`/`ListarFaltas`

**Evidência:** `internal/handlers/registros_handlers.go` — `NotaRegistroResponse`/`FaltaRegistroResponse` agora incluem `registrado_por`, `valor_anterior`, `motivo_correcao`, `corrigido_por`, `corrigido_em`, todos como ponteiro com `omitempty` (correto, já que a maioria dos registros nunca foi corrigida). O `SELECT` SQL de `ListarNotas`/`ListarFaltas` foi atualizado para trazer essas colunas.

**Bônus, não pedido explicitamente mas bem-vindo:** foi adicionado um filtro `?corrigido=true|false` (`parseFiltrosRegistros`, linha ~313), validado com `strconv.ParseBool` e aplicado via `corrigido_em IS NOT NULL`/`IS NULL` — exatamente o tipo de filtro útil para uma tela de auditoria administrativa ("mostrar só o que já foi corrigido"). Implementação segura (sem concatenação de string vinda do usuário na query).

---

### NOVO-02 / NOVO-03 — ✅ RESOLVIDO — Teto de quantidade de falta ausente no aggregate

**Evidência:** `internal/domain/aggregates/estudante_falta.go`.

```go
const MaxQuantidadeFaltasPadrao = 100

func (e *Estudante) RegistrarFalta(..., maxQuantidade int) error {
    ...
    if quantidade <= 0 || quantidade > maxQuantidade {
        return fmt.Errorf("quantidade deve estar entre 1 e %d", maxQuantidade)
    }
    ...
}

func (e *Estudante) CorrigirFalta(..., maxQuantidade int) error {
    ...
}
```

Tanto `RegistrarFalta` quanto `CorrigirFalta` agora recebem `maxQuantidade` como parâmetro e o validam dentro do domínio — exatamente o mesmo padrão já usado com `maxNota` em `RegistrarNota`/`CorrigirNota`. Os handlers (`faltas_handlers.go`) passam `aggregates.MaxQuantidadeFaltasPadrao` (100) nas duas chamadas. Agora nota e falta estão simétricas: a invariante de teto vive no domínio nos dois módulos, não só na borda HTTP.

**Teste:** `TestRegistrarECorrigirFaltaRespeitamTetoDoAggregate` (`estudante_registros_correcao_test.go`) confirma que tanto o registro quanto a correção acima do teto são rejeitados diretamente no aggregate, sem depender do handler.

---

### Cobertura de testes — ⚠️ PARCIALMENTE RESOLVIDO

O que foi adicionado desde a rodada anterior:
- `TestRegistrarECorrigirFaltaRespeitamTetoDoAggregate` (cobre `NOVO-02`/`NOVO-03`).
- `internal/handlers/auditoria_autorizacao_test.go` (cobre parcialmente `TRACE-03`, ver ressalvas acima).
- `internal/db/event_store_integrity_test.go` — `TestLedgerAppendOnlyTriggersAndIntegrityIfDatabaseAvailable`: um teste de integração real (conecta a um Postgres de verdade, roda migrations, tenta `UPDATE`/`DELETE` no ledger e confirma que os triggers de append-only rejeitam) e `TestValidateEventTypeRejectsWhitelistBypassVariants`. É um padrão novo e bom — gated pela variável `SPURI_RUN_DB_INTEGRITY_TESTS=1`, para não travar `go test ./...` comum sem banco disponível, mas rodável em CI com banco isolado.

O que **ainda não existe**, e continua sendo a lacuna mais importante deste módulo:

1. **Nenhum teste de concorrência real.** Não há, em nenhum arquivo do repositório, uso de `go func()`/`sync.WaitGroup` disparando `RegistrarNota`/`RegistrarFalta`/`SaveWithAudit` simultaneamente para o mesmo estudante. Isso significa que a proteção implementada em `PROD-02` (retry em `40001`) está correta **pela leitura do código**, mas **nunca foi exercitada de fato** contra um Postgres real sob concorrência. Dado que o padrão `SPURI_RUN_DB_INTEGRITY_TESTS` já existe e já resolve o problema de "precisa de banco" de forma limpa, esse é o teste mais fácil e mais valioso a acrescentar agora.
2. **Nenhum teste HTTP de ponta a ponta** para `RegistrarNota`, `CorrigirNota`, `RegistrarFaltas` ou `CorrigirFalta` — ou seja, nenhum teste que monta um `httptest.NewRecorder()` + `gin.Context` reais, chama o handler como uma requisição HTTP de verdade, e verifica o `status code`. Tudo o que existe hoje testa funções auxiliares (`inferirAnoAcademicoParaNota`, `podeAuditarEstudante`) ou o aggregate diretamente (pulando o handler, o binding JSON, o `decodeStrictJSON`, a resolução de `academiaDTO`/`estudanteDTO`, etc.). Isso deixa sem prova automatizada, por exemplo: uma academia tentando corrigir a nota de um aluno de outra academia recebe mesmo 403 pelo caminho HTTP completo; um `motivo` vazio no `PATCH` realmente vira 400 antes de chegar ao aggregate; um `id` inexistente vira 404.
3. **Nenhum teste confirmando o recálculo de avaliação final após correção de nota** — o comportamento mais arriscado de todo o desenho de `PROD-01` (uma avaliação final já calculada com o valor errado deve refletir o valor corrigido) continua sem nenhuma prova automatizada, mesmo já tendo passado por duas rodadas de correção.
4. **Nenhum teste para `ValidateJWTConfig`** — continua sem confirmação automatizada de que o processo falha ao subir com `ENV` variado e `JWT_SECRET` vazio.
5. **Nenhum teste de rebuild ponta a ponta** especificamente para `projection_notas`/`projection_faltas` (registrar → corrigir → `Rebuild()` → comparar estado) usando o novo padrão `SPURI_RUN_DB_INTEGRITY_TESTS`. O teste que existe hoje (`event_store_integrity_test.go`) valida a integridade genérica do ledger, não o comportamento específico de `handleNotaCorrigida`/`handleFaltaCorrigida` durante um rebuild.

---

### Documentação da API — ❌ NÃO ATUALIZADA

**Arquivo:** `Documentação da API.md` (raiz do repositório).

Conferido linha a linha contra o código atual: a documentação **não foi tocada** nas três rodadas de correção, apesar de o código ter ganhado dois endpoints novos, um endpoint reaberto para novos papéis, e cinco campos novos em quatro respostas já documentadas. Especificamente:

1. **`PATCH /academia/notas-aluno/:id` (`CorrigirNota`) — não documentado.** Não existe nenhuma seção `### \`PATCH /academia/notas-aluno/:id\`` no arquivo. Quem for integrar com a API hoje não tem como descobrir que esse endpoint existe, muito menos seu contrato (`motivo` obrigatório, o que `nota`/`observacao` aceitam, os códigos de erro 403/404/400).
2. **`PATCH /academia/faltas-aluno/:id` (`CorrigirFalta`) — não documentado.** Mesmo problema do item anterior.
3. **`GET /eventos/:event_id` (`GetEventoAuditoria`) — não documentado.** Só aparece citado de passagem, pelo nome, dentro do parágrafo-resumo da linha ~6858 ("autenticadas globais: (...) `/eventos-estudante/:codigo`, (...)") — e nem isso: `/eventos/:event_id` **não aparece nem nesse resumo**. Não há seção dedicada com path params, autorização (`podeAuditarEstudante`) ou exemplo de resposta.
4. **`GET /eventos-estudante/:codigo` (`GetEventosEstudante`) — citado só pelo nome, sem seção dedicada.** Aparece uma única vez, dentro da mesma lista-resumo da linha ~6858, sem descrição de autorização, query params ou schema de resposta — diferente de todos os outros endpoints de notas/faltas, que têm seção própria com exemplo de JSON (ex. `### \`GET /notas-estudante/:codigo\`` na linha 5108). Antes da correção deste item era só admin, e a documentação também não dizia isso — ou seja, o gap já existia, e continua existindo agora que a regra de autorização mudou (estudante/academia auditam o próprio, admin audita tudo).
5. **`GET /notas` e `GET /notas-estudante/:codigo` — schema de resposta desatualizado.** O JSON de exemplo documentado (linhas 5063–5093 e 5108–5121) não inclui `registrado_por`, `valor_anterior`, `motivo_correcao`, `corrigido_por` nem `corrigido_em` — todos já presentes na resposta real desde a migration 103. A lista de **query params** de `GET /notas` também não menciona o filtro `?corrigido=true|false` adicionado nesta rodada.
6. **`GET /faltas` e `GET /faltas-estudante/:codigo` — mesmo problema do item 5**, com os mesmos cinco campos ausentes do exemplo de resposta (linhas 5202–5232 e 5246–5259) e o mesmo filtro `?corrigido=` ausente da lista de query params de `GET /faltas`.

**Impacto:** isto não é um risco de segurança nem de gravação de dados — o código está correto independentemente da documentação. Mas é um risco real de **produção e de rastreabilidade operacional**: qualquer time de frontend, QA ou integração externa que use `Documentação da API.md` como fonte de verdade (prática comum e, pelo tamanho e detalhamento deste arquivo, claramente a intenção do projeto) vai simplesmente **não saber que a funcionalidade de correção de notas/faltas existe**, e vai continuar achando — como a documentação ainda afirma implicitamente ao não mencionar o contrário — que notas e faltas são somente-criação. Isso é agravado pelo fato de essa mesma documentação já ter descrito explicitamente, em algum momento anterior (ver o próprio histórico deste módulo), o modelo "somente criação e leitura" como uma decisão deliberada — ou seja, um leitor da documentação atual não tem nenhum sinal de que essa decisão foi revista.

**Correção recomendada:**
1. Adicionar duas novas seções `### \`PATCH /academia/notas-aluno/:id\`` e `### \`PATCH /academia/faltas-aluno/:id\``, no mesmo formato e nível de detalhe das seções vizinhas (`POST /academia/notas-aluno`, linha 5016; `POST /academia/faltas-aluno`, linha 5162) — proteção, path params, body esperado (`nota`/`quantidade`, `observacao`, `motivo` obrigatório), respostas de sucesso e de erro (400 motivo ausente, 403 posse, 404 id inexistente, 400 teto excedido), e uma nota explícita de que a correção **não apaga** o lançamento original (preservado no ledger) e que pode disparar recálculo de avaliação final.
2. Adicionar uma seção `### \`GET /eventos/:event_id\`` e transformar a menção solta de `GET /eventos-estudante/:codigo` em seção própria, documentando a regra de autorização por posse (`podeAuditarEstudante`) e o formato de resposta (payload do evento + `metadata` com `user_id`/`user_type`/`ip`).
3. Atualizar os quatro exemplos de resposta JSON (`GET /notas`, `GET /notas-estudante/:codigo`, `GET /faltas`, `GET /faltas-estudante/:codigo`) para incluir `registrado_por`, `valor_anterior`, `motivo_correcao`, `corrigido_por`, `corrigido_em` (como campos opcionais/nulos quando não houve correção, coerente com o `omitempty` do código).
4. Adicionar `corrigido` à lista de query params documentada de `GET /notas` e `GET /faltas`.
5. Revisar o restante do arquivo em busca de qualquer frase remanescente que ainda descreva notas/faltas como "somente criação e leitura" ou equivalente, e atualizar para refletir o novo modelo de correção via evento compensatório.

---

## 3. O que falta para o Codex fazer (ordem de prioridade)

Como todos os itens de **código de produção** já estão corrigidos, o que resta é fechar a lacuna de prova por teste — recomendo tratar isso como o único bloqueador restante antes de liberar para produção, dado que envolve justamente os dois pontos de maior risco residual do sistema (concorrência de gravação e autorização dos novos endpoints de correção).

1. **Teste de concorrência para `PROD-02`** (maior prioridade). Usando o padrão `SPURI_RUN_DB_INTEGRITY_TESTS=1` já existente em `internal/db/event_store_integrity_test.go` como referência: criar um teste (pode ficar em `internal/db/repository_concurrency_test.go` ou similar) que sobe N goroutines chamando `SaveWithAudit` para o **mesmo** `aggregate_id` de estudante com eventos de nota/falta de chaves de negócio diferentes (sem conflito de regra), e confirma que todas terminam com sucesso — provando que o retry de `40001` realmente absorve a corrida sem perder gravação nem devolver 500 ao chamador.
2. **Testes HTTP de ponta a ponta para `CorrigirNota`/`CorrigirFalta`** (e, se ainda não existirem de forma completa, para `RegistrarNota`/`RegistrarFaltas`): usar o padrão de teste HTTP já usado em outros pacotes do projeto (`httptest` + `gin.CreateTestContext`, olhar `financeiro_handlers_integration_test.go` ou `unique_operation_guard_integration_test.go` como referência de estilo, já que ambos parecem já exercitar handlers via HTTP real). Cobrir no mínimo: 403 ao tentar corrigir nota/falta de outra academia; 400 com `motivo` vazio; 404 com `id` inexistente; 400 ao exceder o teto (nota e quantidade); 201/200 no caminho feliz confirmando `registrado_por`/`corrigido_por` na resposta.
3. **Teste de recálculo de avaliação final após correção** (T24 do primeiro relatório) — o cenário de maior risco de negócio: registrar notas suficientes para disparar uma avaliação final automática, corrigir uma delas depois, e confirmar que a avaliação final final reflete o valor novo, não o antigo.
4. **Teste de `ValidateJWTConfig`** — subir com `ENV` ausente/variado e `JWT_SECRET` vazio, confirmar erro; subir com `ENV=development` e `JWT_SECRET` vazio, confirmar que não falha.
5. **Teste de rebuild ponta a ponta para notas/faltas**, no mesmo padrão gated por `SPURI_RUN_DB_INTEGRITY_TESTS`: registrar nota/falta, corrigir, rodar `Rebuild()` da projeção correspondente, e comparar se o estado final da tabela bate com o estado antes do rebuild (mesmo `nota`/`quantidade` atual, mesmo `valor_anterior`, mesmo `corrigido_por`).
6. **Atualizar `Documentação da API.md`** com os 5 pontos detalhados na seção "Documentação da API" acima: as duas novas rotas `PATCH`, a rota `GET /eventos/:event_id` e a seção própria para `GET /eventos-estudante/:codigo`, os campos novos nos quatro exemplos de resposta de notas/faltas, e o filtro `?corrigido=`. Isto não bloqueia o funcionamento do sistema, mas bloqueia qualquer time (frontend, QA, integração externa) que dependa dessa documentação para saber que a funcionalidade de correção existe — por isso deve ser tratado como parte do mesmo pacote de trabalho, não como um "depois".
7. **Limpeza cosmética, opcional:** remover a rota duplicada `admin.GET("/eventos/:event_id", ...)` em `cmd/server/main.go`, já redundante com a versão em `protected`; adicionar um teste para o ramo `academia` de `podeAuditarEstudante`.

Nenhum desses itens exige mudança de comportamento do sistema — são só testes confirmando o que já foi implementado. Por isso, diferente das duas rodadas anteriores, esta pode ser tratada como uma tarefa de menor risco/complexidade para o Codex, mesmo sendo o bloqueador restante.

---

## 4. Veredito final sobre prontidão para produção

**Do ponto de vista de código: pronto.** As três rodadas de depuração, no total, resolveram integralmente os problemas de segurança (`SEC-01/02/03`), de gravação de dados (`PROD-01` a `PROD-07`), de unicidade (`UNIQ-01/02/03/04`) e de rastreabilidade (`TRACE-01/02/03`) identificados originalmente, incluindo os efeitos colaterais que apareceram no meio do caminho (`NOVO-01/02/03`). Não restam achados de código em aberto neste módulo.

**Do ponto de vista de garantia de qualidade: ainda não.** Falta a prova automatizada de que a proteção contra concorrência (`PROD-02`) funciona sob carga real, e falta cobertura HTTP de ponta a ponta para os dois novos endpoints de correção — que são justamente os pontos de maior superfície nova introduzida por este ciclo de correções (uma rota de escrita nova por módulo, mais a lógica de autorização de auditoria). Recomendo fechar os itens 1–3 da seção 3 (concorrência, HTTP e2e, recálculo de avaliação final) como último passo antes do lançamento em produção — são testes rápidos de escrever dado que toda a infraestrutura necessária (padrão `SPURI_RUN_DB_INTEGRITY_TESTS`, helpers de aggregate, handlers já finalizados) já existe no repositório.

**Do ponto de vista de documentação: também ainda não.** `Documentação da API.md` não foi atualizada em nenhuma das três rodadas e hoje descreve uma API defasada — sem os dois endpoints `PATCH` de correção, sem o endpoint de auditoria de eventos devidamente documentado, e com o schema de resposta de `GET /notas`/`GET /faltas` (e suas variantes por estudante) desatualizado. Isto não é um risco de segurança ou de dado, mas é um bloqueador de lançamento na prática: um time de frontend/QA/integração que siga a documentação hoje simplesmente não descobre que a correção de notas/faltas existe. Deve ser corrigido junto com o item 6 da seção 3, no mesmo pacote de trabalho que fecha os testes pendentes.

---

## 5. Checklist de validação (atualizado, 3ª rodada)

- [x] `ENV`/`JWT_SECRET` fail-safe implementado e usado em `main.go`.
- [x] Logs sem dado sensível em nível não-debug.
- [x] `decodeStrictJSON` em todos os endpoints de escrita de notas/faltas (registro e correção).
- [x] Correção de nota/falta implementada como evento compensatório, sem apagar o original, com recálculo de avaliação final na mesma transação.
- [x] Retry em conflito de serialização (`40001`/`40P01`) implementado em `SaveWithAudit`.
- [x] Teto de nota e de quantidade de falta validados dentro do aggregate, em registro e em correção.
- [x] Checagem de matrícula `em_andamento` antes de registrar nota/falta.
- [x] `registrado_por` e campos de correção presentes em **todos** os endpoints de leitura de notas/faltas (`GetNotasEstudante`, `GetFaltasEstudante`, `ListarNotas`, `ListarFaltas`).
- [x] Comentário de `applyFaltasRegistradas` corrigido.
- [x] Estudante e academia conseguem auditar os próprios eventos; admin audita todos; 404 (não 403) para não revelar existência de evento de terceiros.
- [ ] **Teste de concorrência real provando `PROD-02` — pendente.**
- [ ] **Teste HTTP de ponta a ponta para `CorrigirNota`/`CorrigirFalta` — pendente.**
- [ ] **Teste de recálculo de avaliação final após correção — pendente.**
- [ ] Teste de `ValidateJWTConfig` — pendente.
- [ ] Teste de rebuild ponta a ponta para notas/faltas — pendente.
- [ ] **`Documentação da API.md` atualizada com `PATCH /academia/notas-aluno/:id`, `PATCH /academia/faltas-aluno/:id`, `GET /eventos/:event_id`, seção própria para `GET /eventos-estudante/:codigo`, campos novos nos exemplos de resposta de notas/faltas e o filtro `?corrigido=` — pendente.**
- [ ] `go vet ./...` e `go test ./...` — não executados neste ambiente (sem toolchain Go disponível); rodar antes do deploy, incluindo `SPURI_RUN_DB_INTEGRITY_TESTS=1` contra um banco isolado de teste.
