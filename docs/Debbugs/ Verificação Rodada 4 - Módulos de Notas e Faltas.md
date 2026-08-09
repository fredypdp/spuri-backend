# Relatório de Verificação — Rodada 4 — Módulos de Notas e Faltas (spuri-backend)

**Repositório verificado:** `fredypdp/spuri-backend` (branch `main`, snapshot mais recente — posterior à quarta rodada de correções, que tratava os 7 pendentes de `verificacao-final-notas-faltas.md`: testes de concorrência, testes HTTP e2e, teste de recálculo de avaliação final, teste de `ValidateJWTConfig`, teste de rebuild ponta a ponta, atualização da `Documentação da API.md`, e limpeza cosmética).
**Metodologia:** clone completo novo, leitura direta de cada arquivo de teste e de documentação tocado por esta rodada, sem presumir nada a partir de nomes de arquivo/função.

---

## 1. Sumário executivo

| Pendência da rodada anterior | Status agora |
|---|---|
| Teste de concorrência real (`PROD-02`) | ✅ Resolvido |
| Teste HTTP de ponta a ponta para `CorrigirNota`/`CorrigirFalta` | ❌ Não resolvido |
| Teste de recálculo de avaliação final após correção | ❌ Não resolvido |
| Teste de `ValidateJWTConfig` | ✅ Resolvido |
| Teste de rebuild ponta a ponta para notas/faltas | ❌ Não resolvido |
| Atualização de `Documentação da API.md` | ✅ Resolvido (completo e bem feito) |
| Limpeza cosmética (rota duplicada + teste do ramo academia) | ⚠️ Parcial (rota duplicada removida; teste do ramo academia ainda falta) |

**Achado novo desta rodada:** um teste existente (`TestNotasAndFaltasExposeOnlyCreateAndReadRoutes`, em `cmd/server/main_test.go`) ficou **desatualizado e enganoso** — seu próprio nome e sua lista de rotas "removidas" continuam afirmando que notas/faltas só têm rotas de criação e leitura, o que não é mais verdade desde que `PATCH /academia/notas-aluno/:id` e `PATCH /academia/faltas-aluno/:id` foram implementados. Ver seção 3.

**Veredito resumido:** avanço real e concreto nesta rodada — 4 dos 7 itens foram resolvidos com qualidade, incluindo os dois mais importantes do ponto de vista de prova de segurança (`ValidateJWTConfig`) e de robustez sob carga (concorrência real). A documentação da API está agora completa e correta. **Mas o módulo ainda não está pronto para produção**, porque falta exatamente a camada de teste que prova, de ponta a ponta, que os dois endpoints novos (`PATCH` de correção) funcionam como documentado — motivo obrigatório, posse, recálculo de avaliação final — e essa lacuna é agravada por um teste existente que hoje **afirma o oposto do que o sistema faz**.

---

## 2. Verificação item a item

### Teste de concorrência real — ✅ RESOLVIDO

**Evidência:** novo arquivo `internal/db/repository_concurrency_test.go`, `TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable`.

Sobe 8 goroutines chamando `SaveWithAudit` simultaneamente para o **mesmo** `aggregate_id`, cada uma com um evento `NotasRegistradas` sintético (chave de concorrência distinta por goroutine), usando o mesmo padrão `SPURI_RUN_DB_INTEGRITY_TESTS=1` já estabelecido no repositório. Confirma que **todas as 8** terminam sem erro e que **os 8 eventos** ficam persistidos no ledger (`SELECT COUNT(*) ... = workers`) — prova direta, contra um Postgres real, de que o retry em `40001`/`40P01` (`PROD-02`) absorve a corrida sem perder gravação nem devolver erro ao chamador.

**Observação (não é falha):** o teste usa um aggregate sintético mínimo (`concurrentEstudanteAggregate`) em vez do `Estudante` real, isolando deliberadamente a camada de repositório/banco da lógica de domínio de notas/faltas — abordagem correta, já que o mecanismo de retry é genérico do repositório, não específico de notas. Isso prova `PROD-02` de forma sólida; não substitui, porém, um teste e2e que exercite `RegistrarNota`/`RegistrarFaltas` concorrentes via HTTP (esse continua sendo um teste diferente, útil por outro motivo — ver seção 2.2).

---

### Teste de `ValidateJWTConfig` — ✅ RESOLVIDO

**Evidência:** novo arquivo `internal/middleware/auth_config_test.go`, `TestValidateJWTConfig`, com 6 casos: `development`/`test` aceitam segredo efêmero; `production` sem segredo falha; `ENV` vazio falha (fail-safe); `" Production "` (case/espaço variados) também falha (confirma a normalização funcionando no sentido correto — continua exigindo segredo mesmo com variação de escrita); `production` com segredo configurado passa.

Cobre exatamente os casos que a rodada anterior apontou como faltantes, incluindo o caso mais importante para `SEC-01` (variação de `ENV` não deve abrir uma brecha).

---

### Atualização de `Documentação da API.md` — ✅ RESOLVIDO

**Evidência:** conferido linha a linha contra o código atual.

1. `### \`PATCH /academia/notas-aluno/:id\`` (linha 5115) — documentado com proteção, path params, request de exemplo, regras (`motivo` obrigatório, escala de nota, limite de `observacao`), response de sucesso e a lista completa de erros (400/403/404).
2. `### \`PATCH /academia/faltas-aluno/:id\`` (linha 5298) — mesmo padrão, incluindo o teto de `quantidade` (1–100).
3. `### \`GET /eventos/:event_id\`` (linha 5038) — documentado com a regra de autorização por posse e o detalhe importante de devolver 404 (não 403) para não revelar a existência do evento a quem não tem posse.
4. `### \`GET /eventos-estudante/:codigo\`` — ganhou seção própria (antes só era citado pelo nome), com autorização e exemplo de resposta.
5. Os quatro exemplos de resposta (`GET /notas`, `GET /notas-estudante/:codigo`, `GET /faltas`, `GET /faltas-estudante/:codigo`) agora incluem `registrado_por`, `valor_anterior`, `motivo_correcao`, `corrigido_por`, `corrigido_em` — confirmado por 4 ocorrências de `registrado_por` no arquivo, uma por endpoint.
6. O filtro `?corrigido=true|false` está documentado na lista de query params de `GET /notas` e `GET /faltas`.

Não restam gaps de documentação para este módulo.

---

### Limpeza cosmética — ⚠️ PARCIALMENTE RESOLVIDA

- ✅ A rota duplicada `admin.GET("/eventos/:event_id", ...)` foi removida de `cmd/server/main.go` — só resta `protected.GET("/eventos/:event_id", handlers.GetEventoAuditoria)`.
- ❌ O teste do ramo `academia` de `podeAuditarEstudante` continua ausente — `internal/handlers/auditoria_autorizacao_test.go` só tem `TestPodeAuditarEstudanteRestringeEstudanteAoProprioAggregate` e `TestPodeAuditarEstudantePermiteAdmin`, sem um terceiro caso para academia. Baixa prioridade, mantido como pendência menor.

---

### Teste HTTP de ponta a ponta para `CorrigirNota`/`CorrigirFalta` — ❌ NÃO RESOLVIDO

Busca por qualquer teste que exercite as rotas reais `PATCH /academia/notas-aluno/:id` ou `PATCH /academia/faltas-aluno/:id` via requisição HTTP (`httptest`) não encontra nada. `CorrigirNota`/`CorrigirFalta` só são referenciados em `internal/domain/aggregates/estudante_registros_correcao_test.go` — o mesmo arquivo, com os mesmos 4 testes já existentes na rodada anterior (`TestCorrigirNotaPreservaEventoOriginal`, `TestCorrigirFaltaExigeMotivo`, `TestRegistrarECorrigirFaltaRespeitamTetoDoAggregate`, `TestRegistrarNotaRespeitaTetoDoAggregate`) — todos no nível do aggregate, nenhum no nível HTTP.

Isso significa que continuam sem prova automatizada, no caminho HTTP completo (`decodeStrictJSON` → resolução de `academiaDTO`/nota por ID → checagem de posse → chamada ao aggregate → `SaveWithAudit`): 403 ao tentar corrigir nota/falta de outra academia, 400 com `motivo` ausente, 404 com `id` inexistente, 400 ao exceder o teto.

---

### Teste de recálculo de avaliação final após correção — ❌ NÃO RESOLVIDO

Nenhum teste novo relaciona `CorrigirNota` a `tentarAvaliacoesFinaisAutomaticas`. Os testes de avaliação final existentes (`avaliacao_final_regras_test.go`, `avaliacao_final_formula_test.go`, `avaliacao_final_projection_test.go`) continuam sendo só testes unitários de parsing/validação de fórmula e regra — nenhum integra registro+correção de nota com o cálculo de avaliação final. Continua sendo o ponto de maior risco de negócio sem prova automatizada.

---

### Teste de rebuild ponta a ponta para notas/faltas — ❌ NÃO RESOLVIDO

`internal/projections/manager_rebuild_test.go` continua com os mesmos 4 testes de antes (`TestRebuildLockRejectsConcurrentRebuildAndReleasesAfterFailure`, `TestRebuildProjectionUnknownNameReturnsControlledErrorAndReleasesLock`, `TestDefaultRebuildOrderCoversAllRegisteredProjections`, `TestOrderedRebuildProjectionNamesUsesDependencyOrderBeforeFallback`) — todos sobre o mecanismo de lock/ordem de rebuild, nenhum sobre o conteúdo de dados após um rebuild real de `projection_notas`/`projection_faltas` com registros corrigidos.

---

## 3. Achado novo — MÉDIO — Teste existente afirma uma invariante que não é mais verdadeira

**Arquivo:** `cmd/server/main_test.go`, `TestNotasAndFaltasExposeOnlyCreateAndReadRoutes` (linhas ~242–290).

**Problema:** este teste já existia antes de `PROD-01` ser implementado (ou foi escrito durante a implementação e nunca atualizado) e continua com o nome e o propósito original: comprovar que notas e faltas "expõem apenas rotas de criação e leitura". Isso deixou de ser verdade a partir da segunda rodada de correções, que introduziu `PATCH /academia/notas-aluno/:id` e `PATCH /academia/faltas-aluno/:id`.

O teste **ainda passa** hoje, mas por um motivo que mascara o problema em vez de expô-lo: a lista `removed` (rotas que devem devolver 404) testa `PATCH /academia/notas-aluno` **sem** o `:id` — um caminho que nunca existiu e nunca vai bater com a rota real `/academia/notas-aluno/:id`. A lista também testa caminhos inventados que nunca foram implementados (`/academia/atualizar-nota`, `/academia/nota/:id` com `DELETE`, etc.) — nenhum deles é a rota que de fato foi criada. Ou seja, o teste dá a impressão de que "provou" que não existe rota de edição, mas na prática **nunca chegou perto de testar a rota real que existe**.

**Impacto:** é um risco de manutenção, não de segurança — mas é exatamente o tipo de risco que motivou o cuidado deste projeto com comentários desatualizados (`UNIQ-02`, na rodada 2): um teste com esse nome, passando em verde, pode levar qualquer pessoa (ou o próprio Codex, numa tarefa futura) a acreditar que a política "notas e faltas são somente criação e leitura" ainda está em vigor e é reforçada por teste — quando na verdade ela foi deliberadamente revista.

**Correção recomendada:**
1. Renomear o teste para algo como `TestNotasAndFaltasExposeApenasRotasSuportadas` (ou dividir em dois: um confirmando que create/read/correção existem, outro confirmando que métodos/caminhos inválidos continuam 404).
2. Adicionar a lista `registered` os casos que hoje faltam: `PATCH /academia/notas-aluno/:id` e `PATCH /academia/faltas-aluno/:id` com um UUID válido (esperando **não** 404 — mesmo que a resposta final seja 401/400 por falta de autenticação/corpo, o importante é confirmar que a rota está registrada).
3. Manter a lista `removed` como está (ela continua útil para os métodos/caminhos que de fato não devem existir), só deixando claro no comentário do teste que ela não cobre `:id` dinâmico.

---

## 4. Plano de trabalho restante (ordem de prioridade)

1. **Corrigir/renomear `TestNotasAndFaltasExposeOnlyCreateAndReadRoutes`** (seção 3) — rápido, e remove uma fonte ativa de desinformação no próprio repositório antes de escrever os testes novos.
2. **Teste HTTP de ponta a ponta para `CorrigirNota`/`CorrigirFalta`**, usando o padrão `SPURI_RUN_DB_INTEGRITY_TESTS=1` já validado nesta rodada por `repository_concurrency_test.go` (subir client real, rodar migrations, montar o router de verdade via `setupRouter()` como em `main_test.go`, autenticar como academia de teste). Cobrir: 403 (outra academia), 400 (motivo ausente), 404 (id inexistente), 400 (teto excedido), 200 no caminho feliz com `registrado_por`/`corrigido_por` na resposta.
3. **Teste de recálculo de avaliação final após correção** — no mesmo teste de integração acima ou em um dedicado: registrar notas suficientes para disparar uma avaliação final automática, corrigir uma delas, confirmar que a avaliação final reflete o valor novo.
4. **Teste de rebuild ponta a ponta para notas/faltas** — registrar + corrigir nota/falta, rodar `Rebuild("notas")`/`Rebuild("faltas")`, comparar estado da tabela antes/depois.
5. **Teste do ramo `academia` em `podeAuditarEstudante`** — baixa prioridade, mas trivial de fechar junto com o restante.
6. **Opcional:** remover o arquivo vazio `bash.exe.stackdump`, commitado por acidente na raiz do repositório (0 bytes, resquício de crash do Git Bash no Windows) — cosmético, sem qualquer risco, mas não deveria estar versionado.

---

## 5. Veredito final sobre prontidão para produção

**Ainda não — mas o que falta agora é estritamente teste, não código nem documentação.** As quatro rodadas de correção, somadas, resolveram integralmente todos os achados de segurança, gravação de dados, unicidade e rastreabilidade identificados desde o primeiro relatório, e a documentação da API está completa e correta. O que resta é provar, com testes de ponta a ponta, que os dois endpoints de correção — a peça central de todo este ciclo de correções — funcionam exatamente como a documentação (agora correta) descreve, e que a avaliação final é recalculada como o desenho promete. Recomendo tratar os itens 1–3 da seção 4 como o último bloqueador antes do lançamento; os itens 4–6 podem ser resolvidos em paralelo ou logo em seguida, sem risco de bloquear o lançamento.

---

## 6. Checklist de validação (atualizado, 4ª rodada)

- [x] Segredo JWT fail-safe, com teste automatizado cobrindo os casos de falha e sucesso.
- [x] Retry em conflito de serialização provado com teste de concorrência real contra Postgres.
- [x] Correção de nota/falta implementada como evento compensatório, sem apagar o original.
- [x] Teto de nota/quantidade validado dentro do aggregate, em registro e correção.
- [x] `registrado_por` e campos de correção presentes em todos os endpoints de leitura.
- [x] Estudante/academia auditam os próprios eventos; admin audita todos; 404 para não revelar existência de evento de terceiros.
- [x] `Documentação da API.md` atualizada com as duas rotas `PATCH`, os dois endpoints de auditoria e os campos/filtro novos nas listagens.
- [ ] **Teste HTTP de ponta a ponta para `CorrigirNota`/`CorrigirFalta` — pendente.**
- [ ] **Teste de recálculo de avaliação final após correção — pendente.**
- [ ] **Teste de rebuild ponta a ponta para notas/faltas — pendente.**
- [ ] **`TestNotasAndFaltasExposeOnlyCreateAndReadRoutes` corrigido/renomeado para refletir a realidade atual — pendente.**
- [ ] Teste do ramo `academia` em `podeAuditarEstudante` — pendente (baixa prioridade).
- [ ] `go vet ./...` e `go test ./...` (incluindo `SPURI_RUN_DB_INTEGRITY_TESTS=1` contra banco isolado) — não executados neste ambiente; rodar antes do deploy.
