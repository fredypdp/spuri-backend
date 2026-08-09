# Relatório de Verificação — Correções dos Módulos de Notas e Faltas (spuri-backend)

**Repositório verificado:** `fredypdp/spuri-backend` (branch `main`, snapshot de 08/08/2026 — commit atual, posterior à implementação das correções descritas em `docs/Debbugs/Depuração notas faltas.md`)
**Objetivo deste documento:** confirmar, item a item, se cada achado do relatório anterior foi de fato corrigido no código (não apenas mencionado), identificar o que ficou pendente ou foi implementado parcialmente, e dar um veredito sobre prontidão para produção.
**Metodologia:** novo clone completo do repositório público, leitura linha a linha de todos os arquivos tocados pela correção (aggregates, handlers, projections, migrations, `main.go`, testes) e cruzamento com o relatório original. Cada "✅ Resolvido" abaixo foi confirmado lendo o código atual, não apenas a existência de um nome de função.

**Como usar este documento (orientação para o Codex):** a seção 2 é o veredito por item (mantendo os IDs do relatório anterior). A seção 3 traz os achados novos (coisas que só apareceram por causa da própria correção). A seção 4 é a lista de trabalho restante, em ordem de prioridade, para o Codex executar antes de liberar o módulo para produção. A seção 5 é o veredito final.

---

## 1. Sumário executivo

| Status | Quantidade | IDs |
|---|---|---|
| ✅ Resolvido (confirmado por leitura de código) | 14 | SEC-01, SEC-02, SEC-03, PROD-01, PROD-02, PROD-03, PROD-04, PROD-05, PROD-06, PROD-07, TRACE-01, TRACE-02, UNIQ-01 (sem regressão), UNIQ-03 |
| ⚠️ Parcialmente resolvido | 1 | TRACE-03 |
| ❌ Não resolvido | 1 | UNIQ-02 |
| 🆕 Achado novo (efeito colateral da correção) | 3 | NOVO-01, NOVO-02, NOVO-03 |
| ⚠️ Cobertura de teste insuficiente (transversal) | — | ver seção 3.4 |

**Veredito resumido:** a correção foi feita com qualidade real — não é superficial. Os dois itens críticos do relatório anterior (`SEC-01` e `PROD-01`) foram bem resolvidos, com um desenho de evento compensatório correto para notas/faltas que preserva a imutabilidade do ledger. Ainda assim, **o módulo não está pronto para produção** por três motivos concretos: (1) um comentário que pode induzir uma regressão futura continua incorreto no código; (2) o endpoint de auditoria ficou restrito a admin quando a própria especificação editada pedia acesso de estudante/academia aos próprios eventos; (3) a cobertura de testes automatizados é muito inferior ao que a mudança exige — em especial não há nenhum teste de concorrência real, que é justamente o cenário que `PROD-02` foi criado para proteger.

---

## 2. Veredito por item (relatório anterior)

### SEC-01 — ✅ RESOLVIDO — Segredo JWT

**Evidência:** `internal/middleware/auth.go` (linhas 30–55) e `cmd/server/main.go` (linhas 43–51).

A implementação é sólida e vai além do mínimo pedido:
- `env` normalizado com `strings.ToLower(strings.TrimSpace(...))`.
- Quando `JWT_SECRET` está vazio, `init()` não usa mais um segredo fixo público — gera **32 bytes aleatórios via `crypto/rand`** e só loga um aviso.
- Foi criada `ValidateJWTConfig()`, chamada em `main.go` logo no início de `main()` (linha 49), **antes** de conectar ao banco ou subir o servidor: retorna erro se `JWT_SECRET` estiver vazio e `ENV` não for `"development"` nem `"test"` — exatamente a lógica *fail-safe* (allow-list) recomendada, invertendo a lógica antiga.
- `main.go` chama `log.Fatalf` se `ValidateJWTConfig()` falhar (linha 50), garantindo que o processo nunca chega a escutar uma porta com configuração insegura.

**Pendência residual (baixa):** não existe teste automatizado cobrindo esse comportamento (item 4 da correção recomendada original). Ver seção 3.4.

---

### SEC-02 — ✅ RESOLVIDO — Logs sem controle de nível

**Evidência:** `internal/utils/logging.go` (novo arquivo) e uso em `internal/handlers/notas_handlers.go`, `internal/domain/aggregates/aggregate.go`, `internal/projections/notas_projection.go`, `internal/projections/faltas_projection.go`.

```go
func Debugf(format string, args ...interface{}) {
    if strings.EqualFold(strings.TrimSpace(os.Getenv("LOG_LEVEL")), "debug") {
        log.Printf(format, args...)
    }
}
```

Todos os `log.Printf("[DEBUG] ...")` antigos foram trocados por `utils.Debugf(...)`, incluindo dentro de `aggregate.go` (`RaiseEvent`, `ClearUncommittedEvents`, `NewAggregate`, `LoadFromHistory`). O log de `[nota-debug]` que antes imprimia `nota=%.2f` teve o valor da nota **removido** do formato — confirmado comparando o `log.Printf` remanescente (linha 240 de `notas_handlers.go`, nível info, sem gate) que só imprime `estudante`, `tipo`, `categoria`, `materia`, nunca o valor numérico da nota nem a observação.

---

### SEC-03 — ✅ RESOLVIDO — JSON permissivo a campos desconhecidos

**Evidência:** `internal/handlers/notas_handlers.go` linha 36 (`RegistrarNota`), linha 269 (`CorrigirNota`); `internal/handlers/faltas_handlers.go` linha 60 (`RegistrarFaltas`), linha 198 (`CorrigirFalta`) — todos agora chamam `decodeStrictJSON(c, &req)` em vez de `c.ShouldBindJSON`.

O caminho assíncrono (`/notas-aluno/async`, `/faltas-aluno/async`) reaproveita o mesmo `handlers.RegistrarNota`/`RegistrarFaltas` por item (via `worker.processItem`), então herda a mesma validação estrita sem precisar de duplicação — confirmado em `internal/jobs/worker.go`.

---

### PROD-01 — ✅ RESOLVIDO — Ausência de via de correção (evento compensatório)

Esta era a correção de maior esforço e foi implementada de forma consistente com o padrão de event sourcing do resto do sistema. Evidência cruzada em 5 camadas:

1. **Aggregate** (`internal/domain/aggregates/estudante_notas.go` linhas 210–230, `estudante_falta.go` linhas 111–131): novos métodos `CorrigirNota`/`CorrigirFalta`. Ambos exigem `motivo` não vazio, validam que a chave de negócio original existe em `NotasRegistradasPorChave`/`FaltasRegistradasPorChave` antes de aceitar a correção, e **nunca apagam nem alteram o evento original** — apenas levantam um novo evento (`NotaCorrigida`/`FaltaCorrigida`).
2. **Eventos**: `NotaCorrigidaEvent`/`FaltaCorrigidaEvent` carregam `Motivo`, `CorrigidoPor`, `CorrigidoEm`, o ID da nota/falta original e o novo valor — payload completo para auditoria, dentro do próprio ledger imutável.
3. **Projeção** (`notas_projection.go` linhas 177–197, `faltas_projection.go` linhas 168–188): `handleNotaCorrigida`/`handleFaltaCorrigida` fazem `UPDATE` na linha existente, com `valor_anterior=nota` (captura o valor pré-update corretamente, por semântica padrão do `UPDATE` em SQL) e gravam `motivo_correcao`, `corrigido_por`, `corrigido_em`. Se a linha original não existir, retornam erro (`rows != 1`), evitando correção "no vazio".
4. **Handler HTTP** (`notas_handlers.go` linhas 256–328, `faltas_handlers.go` linhas 186–253): busca a nota/falta pelo **ID** (chave primária da projeção), deriva todos os campos de chave de negócio (`periodo`, `materia`, `tipo`, `categoria`/`data`) **a partir do registro encontrado** — não do corpo da requisição — o que elimina qualquer risco de o cliente enviar um `id` de um registro e valores de negócio de outro. Confirma posse (`nota.CodigoAcademia == academia.CodigoAcademia`, 403 caso contrário — proteção IDOR).
5. **Reprocessamento de avaliação final**: `CorrigirNota` chama `tentarAvaliacoesFinaisAutomaticas(...)` com o valor corrigido logo depois de `estudante.CorrigirNota(...)` e antes do `SaveWithAudit` — ou seja, o recálculo de avaliação final entra na **mesma transação** da correção, atomicamente. Isso resolve exatamente o ponto 5 da correção recomendada original.

**Rotas:** `PATCH /academia/notas-aluno/:id` e `PATCH /academia/faltas-aluno/:id`, restritas ao grupo `academia` (com `RequireAcademia()` + `ValidarStatusAcademia()`) — sem acesso de admin, conforme o texto já editado da especificação committada.

**Migration:** `migrations/103_notas_faltas_correcao_auditoria.sql` adiciona `registrado_por`, `valor_anterior`, `motivo_correcao`, `corrigido_por`, `corrigido_em` a `projection_notas` e `projection_faltas`, com índices parciais em `corrigido_em`.

**Teste existente:** `internal/domain/aggregates/estudante_registros_correcao_test.go` — `TestCorrigirNotaPreservaEventoOriginal` confirma que a correção soma um evento `NotaCorrigida` sem remover os anteriores; `TestCorrigirFaltaExigeMotivo` confirma a exigência de motivo.

**Nenhum problema de corretude encontrado nesta parte.** Único ponto de atenção é de cobertura de teste (histórico de múltiplas correções sucessivas, teste HTTP de ponta a ponta) — ver seção 3.4.

---

### PROD-02 — ✅ RESOLVIDO — Falta de retry em conflito de serialização (40001)

**Evidência:** `internal/utils/errors.go` linhas 375–386 (`IsSerializationFailure`, reconhece `pqErr.Code == "40001"` e `"40P01"`, com fallback por string para drivers/wrappers que não expõem `*pq.Error`); `internal/db/repository.go` linhas 177–189 (`SaveWithAudit`):

```go
func (r *AggregateRepository) SaveWithAudit(aggregate aggregates.Aggregate, audit AuditContext) error {
    var err error
    for attempt := 0; attempt < 3; attempt++ {
        err = r.saveWithAuditOnce(aggregate, audit)
        if err == nil || !utils.IsSerializationFailure(err) {
            return err
        }
        time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
    }
    return err
}
```

3 tentativas, backoff de 20/40/60ms, e a versão do aggregate é relida dentro de cada nova transação (`getAggregateVersionTx`), então o retry é seguro mesmo que outra transação concorrente tenha avançado a versão nesse meio tempo. `RegistrarNota`, `RegistrarFaltas`, `CorrigirNota` e `CorrigirFalta` usam todos `SaveWithAudit` — cobertos.

**Pendência residual (baixa, fora do escopo direto de notas/faltas):** o método `Save` (sem audit, ainda usado por outros módulos do sistema) **não** tem o mesmo retry. Não bloqueia a entrega deste módulo, mas vale registrar para consistência do repositório como um todo.

**Pendência de teste (relevante):** não há teste de concorrência real disparando duas gravações simultâneas para confirmar que o retry funciona sob carga — ver seção 3.4, é a lacuna de teste mais importante deste relatório.

---

### PROD-03 — ✅ RESOLVIDO — Teto de nota só no handler

**Evidência:** `internal/domain/aggregates/estudante_notas.go` linha 162 (`RegistrarNota` recebe `maxNota float64` como parâmetro) e linha 176 (`if nota < 0 || nota > maxNota { return err }`); mesmo padrão em `CorrigirNota` linhas 210 e 220–222. O handler (`internal/handlers/modelo_avaliativo_escolar.go` linhas 192–209) manteve a validação amigável de mensagem (`validarEscalaNotaPorAnoAcademico`) **e** passa o mesmo teto (via a função compartilhada `limiteNotaPorAnoAcademico`) para o aggregate — ou seja, agora há dupla validação (UX no handler + invariante no domínio), com uma única fonte de verdade para o valor do teto.

**Teste confirmando:** `TestRegistrarNotaRespeitaTetoDoAggregate` (em `estudante_registros_correcao_test.go`) chama `RegistrarNota` diretamente no aggregate (sem passar pelo handler) com nota 10.01 e teto 10, e confirma que é rejeitado — prova que a invariante agora vive mesmo sem o handler.

---

### PROD-04 — ✅ RESOLVIDO — Sem checagem de status de matrícula

**Evidência:** `internal/handlers/notas_handlers.go` linha 337 (`validarMatriculaEmAndamento`), chamada na linha 94 de `RegistrarNota` e na linha 95 de `faltas_handlers.go` (`RegistrarFaltas`). A função resolve o nível de ensino do estudante (`fundamental`/`medio`/`superior`) e rejeita se o campo de status correspondente (`StatusEscolarFundamental`/`StatusEscolarMedio`/`StatusSuperior`) não for `"em_andamento"`.

**Observação de design (não é bug):** `CorrigirNota`/`CorrigirFalta` **não** chamam essa validação — o que é o comportamento correto: corrigir um lançamento histórico não deveria ser bloqueado só porque o estudante já concluiu o nível depois daquele lançamento.

---

### PROD-05 — ✅ RESOLVIDO — Sem teto de sanidade em quantidade/observação

**Evidência:** `internal/handlers/faltas_handlers.go` linha 103 (`if req.Quantidade > 100 { ... }`) em `RegistrarFaltas`, e linha 202 (`if req.Quantidade > 100 || motivo vazio`) em `CorrigirFalta`. `internal/handlers/notas_handlers.go` linhas 330–335 (`validarObservacao`, limite de 2000 caracteres), chamada tanto em `RegistrarNota` quanto em `RegistrarFaltas`/`CorrigirNota`/`CorrigirFalta`.

**Pendência residual (baixa):** diferente de `CorrigirNota` (que recebe `maxNota` como parâmetro do aggregate), `CorrigirFalta` no aggregate **não** recebe um teto de quantidade — o limite de 100 existe **apenas no handler** (`faltas_handlers.go` linha 202), não dentro de `Estudante.CorrigirFalta`. Isso é o mesmo tipo de gap que `PROD-03` corrigiu para notas, mas não foi replicado por completo para a correção de faltas. Baixo risco hoje (só há um caminho de entrada), mas é uma inconsistência que vale fechar — ver `NOVO-03`.

---

### PROD-06 — ✅ RESOLVIDO — Handlers de lote síncronos mortos

**Evidência:** busca por `^func RegistrarNotaBatch\b` e `^func RegistrarFaltasBatch\b` em `internal/handlers/batch_handlers.go` não retorna mais nenhuma definição — as funções foram removidas. As referências remanescentes a `jobs.JobTypeRegistrarNotaBatch`/`JobTypeRegistrarFaltasBatch` em `async_batch_handlers.go` e `main.go` são o tipo de job do caminho **assíncrono**, que continua existindo corretamente (não é o código morto que foi sinalizado).

---

### PROD-07 — ✅ RESOLVIDO — Structs legadas em `models.go`

**Evidência:** `internal/domain/models.go` agora tem 105 linhas (era maior) e os únicos `type` declarados são `Academia`, `Estudante`, `Curso`, `LoginRequest`, `LoginResponse`, `RegisterAcademiaRequest`, `RegisterEstudanteRequest`, `ErrorResponse`, `SuccessResponse`. `RegistroNotas`, `RegistroFaltas`, `Materia`, `MateriaFaltas`, `RegistrarNotasRequest`, `RegistrarFaltasRequest` não existem mais no arquivo.

---

### UNIQ-01 — ✅ Sem regressão (cadeia de unicidade de notas continua coerente)

**Evidência:** o `ON CONFLICT` de `handleNotasRegistradas` (`notas_projection.go` linha 158) continua usando exatamente as mesmas 7 colunas da constraint `uq_nota_unica`. O próprio comentário no código (linhas 124–132) documenta que essa é a "FIX PROJ-NOTA-03" — ou seja, o time já identificou e corrigiu esse desalinhamento em algum commit anterior a este ciclo, confirmando que era mesmo um ponto histórico frágil como o relatório anterior alertava.

**Pendência:** continua sem um teste de regressão automatizado que registre notas e rode `Rebuild()` contra um banco real para garantir que isso nunca quebre de novo silenciosamente (recomendação original da seção 5, teste T16). Ver seção 3.4.

---

### UNIQ-02 — ❌ NÃO RESOLVIDO — Comentário incorreto sobre unicidade de faltas

**Evidência:** `internal/domain/aggregates/estudante_falta.go`, linhas 137–138, **inalterado**:

```go
// applyFaltasRegistradas — aggregate não mantém estado derivado para faltas.
// A projeção persiste cada registro sem restrição de unicidade por data/matéria.
func (e *Estudante) applyFaltasRegistradas(event DomainEvent) error {
```

Este comentário continua **exatamente igual** ao que foi sinalizado como incorreto no relatório anterior, e continua contradizendo o código das linhas seguintes (que mantém `e.FaltasRegistradasPorChave` e é usado por `RegistrarFalta` para bloquear duplicata) e o schema real (`uq_falta_unica`, restaurada desde a migration 053 e agora reforçada por `ON CONFLICT ON CONSTRAINT uq_falta_unica` em `handleFaltasRegistradasTx`).

Este era o item #4 do plano de execução do relatório anterior — classificado como "médio, trivial" e com a recomendação explícita de ser corrigido **antes** de qualquer nova alteração no aggregate de faltas por outra pessoa/IA. Apesar disso, mesmo com `estudante_falta.go` tendo sido editado nesta mesma rodada de correções (para adicionar `CorrigirFalta`), o comentário não foi tocado.

**Correção recomendada (igual à anterior, ainda válida):**

```go
// applyFaltasRegistradas mantém e.FaltasRegistradasPorChave para permitir que
// RegistrarFalta detecte duplicata por (estudante, academia, ano_lectivo, data,
// materia) antes de emitir o evento. A projeção reforça a mesma unicidade via
// constraint uq_falta_unica (codigo_estudante, codigo_academia, data,
// materia_disciplinar_id) — ver migration 053_restaurar_unicidade_faltas.sql.
```

---

### UNIQ-03 — ✅ RESOLVIDO — `ON CONFLICT DO NOTHING` sem alvo explícito

**Evidência:** `internal/projections/faltas_projection.go` linha 156:

```sql
ON CONFLICT ON CONSTRAINT uq_falta_unica DO NOTHING
```

Exatamente a correção recomendada, e agora simétrica ao padrão já usado em `notas_projection.go`.

---

### UNIQ-04 — Sem alteração (informativo, sem ação obrigatória)

Não houve mudança na chave de unicidade de falta (`estudante+academia+data+matéria`), o que é esperado — esse item era uma observação de modelagem para confirmar com produto, não um defeito. Como agora existe `CorrigirFalta`, a "segunda aula do mesmo dia" descrita no relatório anterior tem, na prática, um caminho claro: usar a correção para ajustar a `quantidade` em vez de criar um segundo registro. Vale só **documentar isso explicitamente** na documentação da API para quem for lançar faltas.

---

### TRACE-01 — ✅ RESOLVIDO — `registrado_por` de nota

**Evidência:** `migrations/103_notas_faltas_correcao_auditoria.sql` adiciona a coluna; `handleNotasRegistradas` (`notas_projection.go` linha 156) insere `payload.RegistradoPor`; `NotaDTO` (linha 216) expõe `registrado_por`; `GetByEstudante`, `GetByAcademia` e `GetNotaByID` (a nova função usada por `CorrigirNota` para IDOR/consistência) todos fazem `SELECT` incluindo a coluna.

**Ver `NOVO-01` na seção 3** — o campo não chegou a **todos** os endpoints de leitura de notas.

---

### TRACE-02 — ✅ RESOLVIDO — `RegistradoPor` de falta

**Evidência:** `FaltasRegistradasEvent` (`estudante_falta.go` linha 29) ganhou o campo `RegistradoPor uuid.UUID`; `RegistrarFalta` (handler) já passava `userID`; a projeção persiste e `FaltaDTO` expõe o campo — simétrico ao que foi feito para notas, na mesma migration 103.

**Ver `NOVO-01`** — mesma ressalva de `ListarFaltas` não expor o campo.

---

### TRACE-03 — ⚠️ PARCIALMENTE RESOLVIDO — Endpoint de auditoria de eventos

**Evidência:** `cmd/server/main.go` linha 494: `admin.GET("/eventos/:event_id", handlers.GetEventoAuditoria)`; `internal/handlers/estudante_handlers.go` linhas 617–664 (`GetEventosEstudante`, `GetEventoAuditoria`).

O que **foi** feito: um endpoint admin funcional que devolve o evento completo do ledger (payload + `metadata`, que contém `user_id`/`user_type`/`ip`) dado um `event_id`, e um segundo endpoint que lista o histórico completo de eventos de um estudante. Ambos usam corretamente `AggregateRepository.GetEventByID`/`GetEventHistory`, que já existiam — exatamente como a correção recomendada descrevia.

**O que não foi feito:** a especificação **já editada e committada** pelo próprio time (texto atual do `TRACE-03` no documento anexado nesta conversa) pede explicitamente:

> "adicionar uma rota para que **estudantes, academias possam auditar os eventos envolvendo eles**, e para administradores poderem auditar os eventos de todos"

O que existe hoje é só a segunda metade. `GetEventosEstudante` está registrada no grupo `protected` (linha 297), que aceita qualquer usuário autenticado — mas o **próprio handler** faz:

```go
userType, _ := middleware.GetUserType(c)
if userType != "admin" {
    utils.RespondWithForbiddenError(c, "Apenas administradores podem consultar eventos.")
    return
}
```

Ou seja: um estudante autenticado tentando ver o próprio histórico de eventos, ou uma academia tentando auditar os lançamentos feitos para seus próprios alunos, recebe `403` hoje. `GetEventoAuditoria` (consulta por `event_id` isolado) está registrada só no grupo `admin`, então nem chega a ser tecnicamente alcançável por estudante/academia.

**Correção recomendada:**
1. Em `GetEventosEstudante`, trocar o bloqueio `userType != "admin"` por uma checagem de posse, no mesmo padrão já usado em `VerificarIntegridade`/`GetNotasEstudante`:
   - `admin` → acesso irrestrito;
   - `estudante` → só se `userID == estudante.ID`;
   - `academia` → só se `estudante.CodigoAcademia == academiaDTO.CodigoAcademia`;
   - qualquer outro caso → 403.
2. Criar (ou reabrir) uma rota equivalente a `GetEventoAuditoria` fora do grupo `admin` (ex. `protected.GET("/eventos/:event_id", ...)`), com a mesma checagem de posse acima — mas cuidado: como o endpoint recebe só um `event_id` isolado (sem saber a priori a quem pertence), o handler precisa primeiro carregar o evento, extrair `AggregateID`, resolver o estudante correspondente, e só então aplicar a checagem de posse — só depois devolver o payload completo. Rejeitar com 404 (não 403, para não vazar a existência do evento) se o solicitante não tiver posse e não for admin.
3. Cobrir com teste: estudante A não consegue ver evento de estudante B; academia X não consegue ver evento de aluno de academia Y; admin vê qualquer um.

---

## 3. Achados novos (efeitos colaterais da própria correção)

### NOVO-01 — MÉDIO — `ListarNotas`/`ListarFaltas` não expõem os novos campos de auditoria/correção

**Arquivo:** `internal/handlers/registros_handlers.go`, `NotaRegistroResponse` (linhas 18–36) e `FaltaRegistroResponse` (linhas 38–54) — a consulta SQL de `ListarNotas`/`ListarFaltas` (`GET /notas`, `GET /faltas`, usados por admin e por academia para listar tudo com filtros) **não inclui** `registrado_por`, `valor_anterior`, `motivo_correcao`, `corrigido_por`, `corrigido_em`, mesmo essas colunas já existindo na tabela desde a migration 103.

**Impacto:** existe hoje uma inconsistência entre os dois caminhos de leitura de notas/faltas: `GET /notas-estudante/:codigo` e `GET /faltas-estudante/:codigo` (que usam `NotaDTO`/`FaltaDTO` da projeção diretamente) **já mostram** quem registrou e se houve correção; `GET /notas` e `GET /faltas` (que usam os `struct` locais de `registros_handlers.go`, com SQL própria) **não mostram**. Um admin auditando pela listagem geral (o caso de uso mais natural para auditoria — "quero ver todos os lançamentos corrigidos recentemente") não teria como fazer isso hoje sem trocar de endpoint.

**Correção recomendada:** adicionar as mesmas 5 colunas ao `SELECT` de `ListarNotas`/`ListarFaltas` em `registros_handlers.go` e aos structs `NotaRegistroResponse`/`FaltaRegistroResponse`, no mesmo padrão (`omitempty` para os campos de correção, que são opcionais). Aproveitar para adicionar um filtro opcional `?corrigido=true` que restrinja a listagem a registros que já sofreram correção — natural para uma tela de auditoria.

---

### NOVO-02 — BAIXO — `CorrigirFalta` no aggregate não recebe teto de quantidade como parâmetro

**Arquivo:** `internal/domain/aggregates/estudante_falta.go`, `CorrigirFalta` (linhas 111–131) — só valida `novaQuantidade <= 0`, sem limite superior. O teto de 100 (mesmo valor usado em `RegistrarFalta`) só é aplicado no handler (`faltas_handlers.go` linha 202), antes de chamar o aggregate.

**Impacto:** baixo hoje (só existe um caminho de entrada para `CorrigirFalta`, e ele já valida), mas é a mesma classe de problema que motivou `PROD-03` — a invariante "quantidade razoável" deveria viver no domínio, não só na borda HTTP, para não depender de todo caminho de entrada futuro lembrar de repetir a checagem.

**Correção recomendada:** adicionar um parâmetro `maxQuantidade int` a `CorrigirFalta` (e, já de caminho, ao próprio `RegistrarFalta`, que hoje só valida `quantidade <= 0` sem teto superior dentro do aggregate — o teto de 100 em `RegistrarFaltas` também só existe no handler, não no aggregate), espelhando exatamente o padrão já usado com sucesso em `RegistrarNota`/`CorrigirNota` e `maxNota`.

---

### NOVO-03 — BAIXO — Consolidar `PROD-05` também dentro do aggregate de faltas

Decorrência direta de `NOVO-02`: como nem `RegistrarFalta` nem `CorrigirFalta` recebem um teto de quantidade no aggregate, o `PROD-05` ficou resolvido de forma assimétrica entre notas (teto no domínio) e faltas (teto só no handler). Resolver junto com `NOVO-02`.

---

## 4. Cobertura de testes — situação atual vs. necessária

A matriz de 25 cenários (T1–T25) proposta no relatório anterior **não foi implementada** como tal. O que existe hoje, especificamente sobre as mudanças desta rodada:

| Teste existente | Cobre |
|---|---|
| `TestCorrigirNotaPreservaEventoOriginal` | Evento de correção não apaga o original (nível aggregate, em memória) |
| `TestCorrigirFaltaExigeMotivo` | Motivo obrigatório na correção de falta (nível aggregate) |
| `TestRegistrarNotaRespeitaTetoDoAggregate` | Teto de nota agora vive no aggregate (nível aggregate) |

Isso cobre bem a lógica pura do aggregate, mas **nenhum teste novo** exercita:
- **Concorrência real** — o cenário exato que `PROD-02` foi criado para resolver (T8/T9 do relatório anterior). Sem isso, a garantia de que o retry de `40001` realmente funciona sob carga concorrente **não está provada**, só é plausível pela leitura do código.
- **Rebuild ponta a ponta contra banco real** após registrar + corrigir notas/faltas (T16/T20), confirmando que `handleNotaCorrigida`/`handleFaltaCorrigida` também funcionam corretamente quando disparados via `Rebuild()` (não só via `Handle()` direto no fluxo síncrono).
- **HTTP de ponta a ponta** para `CorrigirNota`/`CorrigirFalta`: 404 quando o `id` não existe, 403 quando pertence a outra academia, 400 quando falta `motivo`, 400 quando a nova nota extrapola o teto.
- **Recalculo de avaliação final após correção** (T24) — a parte mais arriscada de `PROD-01` do ponto de vista de corretude de negócio (uma avaliação final já calculada com o valor errado precisa refletir o valor corrigido), e é justamente a parte com lógica mais complexa (`tentarAvaliacoesFinaisAutomaticas` reaproveitando regras em cadeia). Hoje isso não tem nenhuma cobertura automatizada.
- **Verificação de matrícula** (`validarMatriculaEmAndamento`, T15) bloqueando lançamento para estudante `finalizado`/`inativo`.
- **Janela de data de falta** (T17) e **IDOR** (T13/T14, e os novos endpoints `PATCH`).
- **`ValidateJWTConfig`** (processo falha com `ENV` variado e `JWT_SECRET` vazio).

**Recomendação:** antes de liberar para produção, o Codex deve, no mínimo, implementar os testes de concorrência (o de maior risco) e os testes HTTP de `CorrigirNota`/`CorrigirFalta` com foco em IDOR e recálculo de avaliação final. Os demais (rebuild ponta a ponta, `ValidateJWTConfig`) podem ser feitos em paralelo, mas não deveriam ser adiados para depois do lançamento.

---

## 5. Plano de trabalho restante (ordem de prioridade)

1. **`TRACE-03`** — abrir acesso de estudante/academia aos próprios eventos, com checagem de posse. É o único item que ainda deixa uma promessa da especificação sem cumprir.
2. **Testes de concorrência para `PROD-02`** — validar sob carga real que o retry de `40001` funciona; é a parte do sistema com menor cobertura de prova hoje, apesar de crítica para o pico de lançamento de notas.
3. **Teste HTTP de ponta a ponta para `CorrigirNota`/`CorrigirFalta`** — IDOR, motivo ausente, id inexistente, teto de nota, e principalmente o recálculo de avaliação final após correção (T24).
4. **`UNIQ-02`** — corrigir o comentário incorreto em `estudante_falta.go`. Trivial, sem risco, deveria ter sido feito junto com o resto.
5. **`NOVO-01`** — adicionar os campos de auditoria/correção a `ListarNotas`/`ListarFaltas`.
6. **`NOVO-02` / `NOVO-03`** — levar o teto de quantidade de falta para dentro do aggregate (`RegistrarFalta` e `CorrigirFalta`), igual já foi feito para nota.
7. **Teste de `ValidateJWTConfig`** e **teste de rebuild ponta a ponta** — podem ser feitos em paralelo com os itens acima.

---

## 6. Veredito final sobre prontidão para produção

**Ainda não.** A qualidade do trabalho feito é alta — os dois riscos mais graves do relatório anterior (segredo JWT fraco e impossibilidade de corrigir lançamentos) foram resolvidos com um desenho correto, não com atalhos. Mas "pronto para produção" para um sistema de notas/faltas usado por escolas e universidades exige mais do que "o código parece correto na leitura": exige prova de que o caminho crítico (lançamento e correção sob concorrência real, com recálculo de avaliação final) funciona, e isso hoje não está testado. Some a isso um item de rastreabilidade prometido e não entregue (`TRACE-03`) e um comentário incorreto que continua sendo uma armadilha para a próxima pessoa (ou IA) que mexer em `estudante_falta.go`.

Recomendo tratar os itens 1–3 da seção 5 como bloqueadores de lançamento, e os itens 4–7 como aceitáveis para corrigir logo em seguida, já em produção, sem risco relevante.

---

## 7. Checklist de validação (atualizado)

- [x] Subir a aplicação com `ENV` vazio/ausente e `JWT_SECRET` vazio → processo falha antes de escutar a porta.
- [x] `ON CONFLICT` de notas e faltas usa exatamente as colunas/constraint corretas.
- [x] Nota lançada errada → corrigida via `PATCH /academia/notas-aluno/:id` → valor atual correto, `valor_anterior`/`motivo_correcao`/`corrigido_por`/`corrigido_em` preenchidos, ledger original preservado.
- [x] `GET /notas-estudante/:codigo` e `GET /faltas-estudante/:codigo` retornam `registrado_por`.
- [ ] `GET /notas` e `GET /faltas` (listagem geral) também retornam `registrado_por` e campos de correção — **pendente (`NOVO-01`)**.
- [x] Logs em produção não imprimem valor de nota/observação em nível info.
- [ ] Duas notas/faltas concorrentes para o mesmo estudante, chaves diferentes → ambas gravadas, sem 500 espúrio — **implementado no código, sem teste automatizado provando (pendente)**.
- [ ] Estudante/academia conseguem consultar os próprios eventos de auditoria — **pendente (`TRACE-03`)**.
- [ ] Comentário de `applyFaltasRegistradas` corrigido — **pendente (`UNIQ-02`)**.
- [ ] Avaliação final recalculada automaticamente após correção de nota, coberto por teste — **implementado no código, sem teste automatizado provando (pendente)**.
- [ ] `go vet ./...` e `go test ./...` sem falhas em todo o repositório — não executado neste ambiente (sem acesso a banco/toolchain completo); recomenda-se rodar antes do deploy.
