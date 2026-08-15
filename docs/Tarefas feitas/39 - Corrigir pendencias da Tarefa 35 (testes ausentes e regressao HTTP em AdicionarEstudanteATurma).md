---
criado: 2026-08-15 00:00
origem: Depuração pós-implementação da Tarefa 35 (docs/Lista de Tarefas/35 - Cadastro de estudante vinculado a turma (individual e em massa).md, commits 9f8c73f e d11b1fe) — conversa com Claude, o orquestrador; auditoria feita clonando fredypdp/spuri-backend e lendo o código linha a linha
status: feito
prioridade: média-alta — o código funcional está correto (auditado e confirmado), mas há uma regressão de código HTTP não coberta por nenhum teste, e nenhum dos 11 testes obrigatórios da especificação original foi implementado
repositorio: fredypdp/spuri-backend (branch main)
---

# 38 — Corrigir pendências da Tarefa 35 (testes obrigatórios ausentes e regressão de código HTTP em AdicionarEstudanteATurma) (feito)

## Prompt recomendado para executar a atualização

Esta tarefa corrige duas pendências encontradas numa auditoria pós-implementação da Tarefa 35 (`docs/Lista de Tarefas/35 - Cadastro de estudante vinculado a turma (individual e em massa).md`). A lógica funcional da Tarefa 35 (ordem de validação da turma antes de efeitos colaterais, vinculação pós-criação sem reconsultar a projeção de estudante, degradação graciosa em `turma_aviso`, retry único em conflito de concorrência otimista, atualização de `Documentação da API.md`) foi auditada linha a linha e está correta — **não altere `internal/handlers/estudante_handlers.go`, `internal/handlers/job_item_handlers.go`, nem `Documentação da API.md`** como parte desta tarefa. Implemente exatamente os dois itens da seção "Escopo obrigatório": (A) corrigir a regressão de código HTTP em `AdicionarEstudanteATurma` (item cirúrgico, um único arquivo); (B) implementar os 11 testes obrigatórios que a especificação original da Tarefa 35 exigia e que não foram criados. Ao final, rode `gofmt`, `go build ./...`, `go vet ./...` e `go test ./...`, e confirme cada item de "Critérios de aceite".

## Contexto

A Tarefa 35 implementou vínculo de estudante a turma diretamente no cadastro (`POST /academia/estudante/register` e `/register/async`, síncrono e em massa), via um campo opcional `codigo_turma`. Dois commits implementaram isso de forma independente e foram mergeados (`9f8c73f`, depois `d11b1fe` — este último também relaxou a constraint de `periodo` em faltas, tratado no documento `37 - Corrigir pendencias da Tarefa 34...`). O resultado no `HEAD` atual foi auditado e a lógica de negócio está correta: a validação da turma (existência, pertencimento à academia, status `ativo`, compatibilidade de nível/curso com o estudante) acontece **antes** de qualquer upload de documento ou criação do agregado `Estudante`; a vinculação em si acontece **depois** de `SaveWithAudit` do estudante, com falhas capturadas em `turma_aviso` na resposta em vez de abortar o cadastro (degradação graciosa); há retry único em conflito de concorrência otimista ao adicionar o estudante à turma. `Documentação da API.md` já documenta `codigo_turma` nos três endpoints relevantes (`register`, `register/async`, e a nota em `POST /academia/turma/:codigo/estudante`).

Duas coisas, porém, ficaram pendentes:

### Problema A — regressão de código HTTP em `AdicionarEstudanteATurma` (rota manual)

A especificação original da Tarefa 35 pedia que a lógica de vinculação fosse extraída para uma função compartilhada (`vincularEstudanteATurma`, em `internal/handlers/turmas_handler.go`, linha 523), reaproveitada tanto pelo fluxo novo de cadastro quanto pela rota manual já existente `POST /academia/turma/:codigo/estudante` (`AdicionarEstudanteATurma`, linha 609) — e pedia explicitamente que a rota manual ficasse **"inalterada em comportamento"**.

Antes da extração, `AdicionarEstudanteATurma` tinha esta checagem própria para turma inexistente:

```go
turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
if err != nil || turmaDTO == nil {
    utils.RespondWithNotFoundError(c, "turma")
    return
}
```

Isso retornava **`404`**. Depois da extração, essa checagem foi movida para dentro de `vincularEstudanteATurma`, que agora retorna um `error` genérico:

```go
// dentro de vincularEstudanteATurma, linha ~538
if turmaDTO == nil {
    return fmt.Errorf("turma não encontrada ou não pertence a esta academia")
}
```

E `AdicionarEstudanteATurma` (linha ~637) trata **qualquer** erro vindo dessa função — turma inexistente, turma inativa, turma incompatível, estudante já vinculado a outra turma — da mesma forma:

```go
if err := vincularEstudanteATurma(c, academiaDTO, req.CodigoEstudante, ..., codigoTurma, false, academiaID); err != nil {
    utils.RespondWithValidationError(c, err) // sempre 400
    return
}
```

Ou seja: `POST /academia/turma/:codigo/estudante` com um `codigo_turma` inexistente **agora retorna `400` em vez de `404`**, contrariando a exigência explícita de que a rota manual permanecesse inalterada em comportamento. Isso pode quebrar clientes de API que dependem do `404` para distinguir "turma não existe" de "requisição malformada". Não há nenhum teste cobrindo `AdicionarEstudanteATurma` hoje (`grep` por essa função em arquivos `_test.go` no repositório não retorna nada), então ninguém pegou essa regressão.

O fluxo novo de cadastro (`registerEstudantePorAcademiaComRequestModo`, em `estudante_handlers.go`) **não** tem esse problema: ele faz sua própria checagem de existência da turma, com seu próprio `404`, **antes** de chamar `vincularEstudanteATurma` (que só é chamado depois da criação do estudante, com erros capturados em `turma_aviso`, nunca em código HTTP de erro). Portanto o Problema A afeta **apenas** a rota manual `POST /academia/turma/:codigo/estudante`.

### Problema B — nenhum dos 11 testes obrigatórios foi implementado

A especificação original da Tarefa 35 (seção 4, "Testes obrigatórios") listava 11 cenários. Hoje só existe um teste incidental (`TestCadastroEstudanteJSONItemToCadastroRequest`, em `internal/handlers/async_batch_handlers_test.go`) que verifica apenas que o campo `CodigoTurma` é propagado e tem espaços removidos ao converter um item de lote — não testa nenhum comportamento funcional do vínculo em si. Não existe nenhum teste de integração HTTP para o fluxo novo, nem nenhum teste que proteja `AdicionarEstudanteATurma` contra regressões (o que é como o Problema A passou despercebido).

## Objetivo

1. `POST /academia/turma/:codigo/estudante` volta a retornar `404` (não `400`) quando `codigo_turma` não existe ou não pertence à academia, exatamente como antes da Tarefa 35 — sem perder o reaproveitamento de `vincularEstudanteATurma` para o resto da lógica.
2. Os 11 cenários da seção 4 da Tarefa 35 estão implementados e passando.

## Escopo obrigatório

### A. Corrigir o código HTTP de `AdicionarEstudanteATurma`

Em `internal/handlers/turmas_handler.go`:

**1.** Logo abaixo dos imports do arquivo (linha ~14), adicione um sentinel error de pacote:

```go
var errTurmaNaoEncontradaParaVinculo = errors.New("turma não encontrada ou não pertence a esta academia")
```

(será necessário adicionar `"errors"` à lista de imports do arquivo, que hoje é `"fmt"`, `"log"`, `"net/http"`, `"spuri/internal/db"`, `"spuri/internal/domain/aggregates"`, `"spuri/internal/middleware"`, `"spuri/internal/projections"`, `"spuri/internal/utils"`, `"github.com/gin-gonic/gin"`, `"github.com/google/uuid"`).

**2.** Dentro de `vincularEstudanteATurma` (linha ~538), troque:

```go
if turmaDTO == nil {
    return fmt.Errorf("turma não encontrada ou não pertence a esta academia")
}
```

por:

```go
if turmaDTO == nil {
    return errTurmaNaoEncontradaParaVinculo
}
```

(a mensagem textual continua idêntica — isso preserva o texto que hoje aparece em `turma_aviso` no fluxo de cadastro, que não deve mudar).

**3.** Em `AdicionarEstudanteATurma` (linha ~637), troque:

```go
if err := vincularEstudanteATurma(c, academiaDTO, req.CodigoEstudante, derefString(estudanteDTO.AnoEscolar), derefString(estudanteDTO.AnoEscolarMedio), derefString(estudanteDTO.AnoSuperior), derefString(estudanteDTO.CursoMedioID), derefString(estudanteDTO.CursoSuperiorID), codigoTurma, false, academiaID); err != nil {
    utils.RespondWithValidationError(c, err)
    return
}
```

por:

```go
if err := vincularEstudanteATurma(c, academiaDTO, req.CodigoEstudante, derefString(estudanteDTO.AnoEscolar), derefString(estudanteDTO.AnoEscolarMedio), derefString(estudanteDTO.AnoSuperior), derefString(estudanteDTO.CursoMedioID), derefString(estudanteDTO.CursoSuperiorID), codigoTurma, false, academiaID); err != nil {
    if errors.Is(err, errTurmaNaoEncontradaParaVinculo) {
        utils.RespondWithNotFoundError(c, "turma")
    } else {
        utils.RespondWithValidationError(c, err)
    }
    return
}
```

Isso restaura o `404` original para turma inexistente na rota manual, mantém `400` para os demais erros (turma inativa, incompatibilidade, estudante já vinculado — que já eram `400` antes da Tarefa 35 e continuam sendo), e não muda em nada o comportamento do fluxo de cadastro (que trata o erro de forma totalmente separada, via `turma_aviso`, e não passa por este `if`).

Confirme com `go build ./...` que não sobrou nenhum uso do antigo `fmt.Errorf("turma não encontrada...")` inline, e que `fmt` continua sendo usado em outras partes do arquivo (não removê-lo dos imports).

### B. Testes da seção 4 da Tarefa 35 (11 testes, numeração original preservada)

Não existe hoje nenhum fixture de integração HTTP para turmas/vínculo de estudante. Crie um novo arquivo `internal/handlers/turma_vinculo_estudante_integration_test.go` (ou adicione a um arquivo de integração já existente no pacote `handlers`, se preferir manter tudo junto — mas não reaproveite `cmd/server/notas_faltas_correcao_integration_test.go`, que é do pacote `main` e cobre um domínio diferente).

Modele o fixture no padrão já usado por `setupRegistrosCorrecaoIntegration` (`cmd/server/notas_faltas_correcao_integration_test.go`, linha 41): guard `if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" { t.Skip(...) }`, troca de `dbClient`/`repository`/`projManager` globais por uma instância de teste com `client.RunMigrations()`, e `t.Cleanup` restaurando o estado anterior. Diferenças específicas necessárias para estes testes:

- Crie a academia via `aggregates.NewAcademia()` + `.Criar(...)` (ver `criarAcademiaCorrecao` como referência de como uma academia é montada e salva) e rebuild de `projections.NewAcademiaProjection(client)`.
- Crie ao menos uma turma via `aggregates.NewTurma()` + `.Criar(codigoTurma, codigoAcademia, nivel, cursoID, turno, criadoPor)` (`internal/domain/aggregates/turma.go`, linha 111) — note que turmas nascem com `Status = "ativo"` automaticamente; para o teste 5 (turma inativa) chame também `.Desativar(criadoPor)` antes de salvar. Rebuild de `projections.NewTurmasProjection(client)`.
- Use o router de produção via `setupRouter()` (mesma função usada pelo fixture de referência) e `middleware.GenerateToken(academiaID, "academia")` para o token de autenticação.
- Rotas relevantes (confirmadas em `cmd/server/main.go`): `POST /academia/estudante/register` (síncrono), `POST /academia/estudante/register/async` (em massa), `POST /academia/turma/:codigo/estudante` (rota manual).

Lista de testes obrigatórios:

1. **Cadastro individual sem `codigo_turma` — regressão.** `POST /academia/estudante/register` sem o campo `codigo_turma`. Confirme `201`/sucesso e que a resposta **não** contém `turma_vinculada`/`turma_aviso` (comportamento idêntico ao pré-Tarefa-35).

2. **Cadastro individual com `codigo_turma` válido — vinculado.** Mesmo endpoint, com `codigo_turma` de uma turma ativa e compatível com o estudante. Confirme `201`, resposta indicando vínculo bem-sucedido, e que uma consulta subsequente (`GET /academia/turma/:codigo/estudantes` ou equivalente já existente, ou diretamente via `projections.NewTurmasProjection(client).ListByEstudante(...)`) mostra o estudante de fato vinculado à turma.

3. **`codigo_turma` inexistente — `404`.** `POST /academia/estudante/register` com um `codigo_turma` que não existe. Confirme `404` e que **nenhum** estudante foi criado (consulte `projection_estudantes` ou tente buscar pelo código enviado — não deve existir). Isso comprova que a pré-validação acontece antes da criação, não depois.

4. **`codigo_turma` de outra academia — `404`.** Crie uma segunda academia com sua própria turma; tente cadastrar um estudante na primeira academia referenciando o `codigo_turma` da segunda. Confirme `404` (mesma mensagem/tratamento do teste 3 — turma "não pertence a esta academia").

5. **`codigo_turma` inativa — `400`.** Use a turma desativada criada no fixture (`.Desativar(...)`). Confirme `400`, e que nenhum estudante foi criado (mesma lógica do teste 3: a pré-validação deve abortar antes de qualquer criação).

6. **`codigo_turma` incompatível com o estudante (ex.: turma de nível médio, estudante com dados de fundamental) — `400`.** Confirme `400`, mensagem mencionando incompatibilidade, e nenhum estudante criado.

7. **Vinculação pós-criação não dispara leitura da projeção de estudantes.** Este é o teste mais importante para prevenir a classe de regressão descrita no Problema A. Como `getEstudanteProjection(c)` neste código não é injetável/mockável (constrói uma nova instância a partir do cliente de banco a cada chamada, sem ponto de injeção de spy), **não** tente usar mock — em vez disso, prove o comportamento de forma direta: cadastre um estudante com `codigo_turma` válido **sem** rodar `projections.NewEstudanteProjection(client).Rebuild()` depois — ou seja, deliberadamente deixe a projeção de estudantes desatualizada/atrasada em relação ao ledger, simulando o lag real de produção entre gravar o evento e a projeção assíncrona processá-lo. Confirme que o cadastro e o vínculo **mesmo assim têm sucesso** (a resposta indica vínculo bem-sucedido), provando que o passo de vinculação não depende de reconsultar `projection_estudantes` para o estudante recém-criado.

8. **Cadastro em massa (`/register/async`) com mistura de itens com e sem `codigo_turma`, incluindo pelo menos um `codigo_turma` inválido.** Monte um lote com 3+ itens: um sem `codigo_turma`, um com `codigo_turma` válido, um com `codigo_turma` inexistente. Rode o job (via `handlers.RegisterEstudantePorAcademiaJobItem`, chamado diretamente no teste com um item por vez, no mesmo padrão usado pelo worker de produção) e confirme que cada item é tratado de forma independente: o item sem turma e o item com turma válida são bem-sucedidos; o item com turma inválida falha (ou é criado com `turma_aviso`, conforme o comportamento já implementado para essa etapa pós-criação — confirme qual é o comportamento real antes de fixar o `assert`, já que a pré-validação da rota síncrona não se aplica da mesma forma dentro do processamento de item de job).

9. **Simulação de falha pós-criação — degradação graciosa.** Force uma falha na etapa de vinculação **depois** que o estudante já foi criado com sucesso (por exemplo, desative a turma entre o momento da pré-validação bem-sucedida e o momento da vinculação real — isso não é trivial de forçar via HTTP direto; uma alternativa mais simples e igualmente válida é usar o fluxo de item de job, que não tem pré-validação HTTP separada: cadastre passando `codigo_turma` de uma turma que existe mas está inativa). Confirme que **o estudante é criado com sucesso mesmo assim**, e que a resposta contém `turma_aviso` (ou o campo equivalente já implementado) relatando o erro de vinculação, em vez de abortar o cadastro inteiro.

10. **Conflito de concorrência otimista com retry.** Modele em `internal/db/repository_concurrency_test.go` (`TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable`) como referência de padrão para forçar conflito: dispare duas goroutines chamando `vincularEstudanteATurma` (ou, mais realisticamente, dois cadastros concorrentes com `codigo_turma` apontando para a mesma turma) simultaneamente contra a mesma turma. Confirme que ambas eventualmente têm sucesso (o retry único absorve o conflito de versão) ou, se ambas colidirem duas vezes seguidas (cenário raro), que a que falhar reporta erro de forma limpa sem corromper o estado da turma. Verifique ao final que `projection_turmas`/o agregado da turma reflete corretamente os dois estudantes adicionados, sem perda de nenhum dos dois.

11. **`AdicionarEstudanteATurma` (rota manual) continua funcionando sem regressão, incluindo a checagem de duplicidade que só se aplica a esse caminho.** Este teste também é a prova da correção do Problema A (seção A deste documento). Cubra especificamente: (a) vínculo bem-sucedido retorna `200`; (b) `codigo_turma` inexistente retorna **`404`** (não `400` — esta é a asserção que hoje falharia sem a correção do item A); (c) tentar vincular um estudante que já pertence a outra turma ativa retorna `400` com mensagem de duplicidade (a checagem de duplicidade só é pulada quando `ignorarChecagemDuplicidade=true`, que só acontece no fluxo de cadastro novo — confirme que a rota manual continua chamando com `false`, preservando essa checagem).

## Fora de escopo

- Qualquer alteração em `internal/handlers/estudante_handlers.go` — a lógica de pré-validação e o fluxo de cadastro já estão corretos.
- Qualquer alteração em `internal/handlers/job_item_handlers.go` — a propagação de `codigo_turma` através do processamento de item de job já está correta.
- Qualquer alteração em `Documentação da API.md` — já reflete corretamente `codigo_turma` nos três endpoints relevantes.
- Qualquer alteração em `internal/domain/aggregates/turma.go` — a lógica de `AdicionarEstudanteNoAnoLectivo` e a checagem de duplicidade já estão corretas.
- Alterar o texto da mensagem de erro de turma não encontrada — deve continuar sendo exatamente `"turma não encontrada ou não pertence a esta academia"`, já que esse texto também aparece em `turma_aviso` no fluxo de cadastro e não deve mudar.
- Qualquer mudança relacionada à Tarefa 34/faltas — isso é tratado em documento separado (`37 - Corrigir pendencias da Tarefa 34 (v2 final).md`).

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual de `main`.
2. Aplicar a correção A (sentinel error + `errors.Is`) em `internal/handlers/turmas_handler.go`.
3. Implementar o teste 11 primeiro (é o que comprova a correção A) e confirmar que falha antes da correção e passa depois — se estiver implementando a correção e o teste na mesma sessão, rode o teste contra o código antigo mentalmente/via `git stash` para validar que ele de fato pegaria a regressão.
4. Implementar o fixture de integração compartilhado e os testes 1 a 10 restantes.
5. Rodar `gofmt -l .` (sem saída) e `go build ./...`.
6. Rodar `go vet ./...`.
7. Rodar `go test ./...` e, para os testes de integração, `SPURI_RUN_DB_INTEGRITY_TESTS=1 go test ./... -run <padrão relevante>` contra uma base PostgreSQL isolada de testes.
8. Revisar o diff completo e confirmar que só `internal/handlers/turmas_handler.go` (código de produção) e arquivos `*_test.go` novos/alterados aparecem.

## Critérios de aceite

- [ ] `POST /academia/turma/:codigo/estudante` com `codigo_turma` inexistente retorna `404` (confirmado por teste).
- [ ] `POST /academia/turma/:codigo/estudante` com turma inativa, incompatível, ou estudante já vinculado a outra turma continua retornando `400` (sem regressão).
- [ ] Os 11 testes da seção B estão implementados, com nomes de teste claros o suficiente para identificar qual cenário da lista cada um cobre, e todos passam.
- [ ] Nenhum arquivo de código de produção foi alterado além de `internal/handlers/turmas_handler.go`.
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `go test ./...` passa; testes de integração que dependem de `SPURI_RUN_DB_INTEGRITY_TESTS=1` foram executados pelo menos uma vez contra uma base real e confirmados passando (documentar no PR).

## Procedimento de conclusão

Ao finalizar a implementação:

1. Atualizar o título interno deste arquivo para `# 38 — Corrigir pendências da Tarefa 35 (testes obrigatórios ausentes e regressão de código HTTP em AdicionarEstudanteATurma) (feito)`;
2. Alterar o front matter para `status: feito`;
3. Mover este arquivo para `docs/Tarefas feitas/`, e, se ainda não foi feito, mover também `docs/Lista de Tarefas/35 - Cadastro de estudante vinculado a turma (individual e em massa).md` para `docs/Tarefas feitas/` com `status: feito` (a implementação funcional em si já está completa — só as pendências desta tarefa 38 bloqueavam a conclusão formal da Tarefa 35).
