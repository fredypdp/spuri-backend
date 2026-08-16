---
criado: 2026-08-16 00:00
origem: Auditoria profunda pós-implementação das Tarefas 38 e 39 (docs/Tarefas feitas/38 - Corrigir pendencias da Tarefa 34 (v2 final).md e docs/Tarefas feitas/39 - Corrigir pendencias da Tarefa 35 (testes ausentes e regressao HTTP em AdicionarEstudanteATurma).md), conduzida por Claude (Anthropic) clonando fredypdp/spuri-backend e lendo o código linha a linha (COALESCE nas 5+1 queries de faltas, fallback de chave legada em CorrigirFalta, sentinel error + errors.Is em AdicionarEstudanteATurma, migração 107, e todos os arquivos *_test.go relevantes).
status: feito
prioridade: alta — os 24 testes de regressão listados abaixo são a única prova de que duas correções críticas (leitura/correção de faltas históricas sem período; código HTTP de vínculo estudante↔turma) continuam funcionando; sem eles, qualquer regressão futura nesses dois pontos passa despercebida, exatamente como já aconteceu uma vez em cada um.
depende_de:
  - "docs/Tarefas feitas/38 - Corrigir pendencias da Tarefa 34 (v2 final).md"
  - "docs/Tarefas feitas/39 - Corrigir pendencias da Tarefa 35 (testes ausentes e regressao HTTP em AdicionarEstudanteATurma).md"
repositorio: fredypdp/spuri-backend (branch main)
---

# 43 — Implementar os testes de regressão ainda ausentes das Tarefas 38 e 39 (auditoria pós-implementação)

## Prompt recomendado para executar a atualização

Leia este documento por completo antes de escrever qualquer código. Ele é definitivo: todas as decisões de design,
localização de arquivos, nomes de campos de banco, mensagens de erro exatas e assinaturas de função já foram
verificadas linha a linha contra o `HEAD` atual do repositório. Você não precisa investigar nada, escolher entre
abordagens alternativas, nem reler o histórico do git ou os documentos das Tarefas 34/35/38/39 — tudo que é
necessário está reproduzido ou referenciado com precisão aqui.

**Resumo em uma frase:** o código de produção das Tarefas 38 e 39 está correto — não altere nenhum arquivo de
produção. O que falta é exclusivamente a implementação de 24 testes de regressão que já haviam sido especificados
e prometidos como concluídos, mas que na prática ficaram: 13 ausentes (Tarefa 38 — cenários 2 a 13 e 15) e 11
existentes apenas como stubs vazios com `t.Skip("TODO: implementar fixture HTTP completo da tarefa 39")` (Tarefa
39 — cenários 1 a 11).

Implemente exatamente os itens da seção "Escopo obrigatório" abaixo, na ordem Parte B → Parte A (B primeiro porque
corrige um problema estrutural — localização de pacote — que bloqueou toda a Tarefa 39 e cujo padrão correto você
vai reaproveitar depois na Parte A). Ao final, rode `gofmt -l .` (sem saída), `go build ./...`, `go vet ./...`,
`go test ./...`, e depois `SPURI_RUN_DB_INTEGRITY_TESTS=1 go test ./... -run <padrão relevante>` contra uma base
PostgreSQL de testes isolada para os testes de integração, e confirme cada item de "Critérios de aceite".

## Contexto — o que a auditoria encontrou

### O que está correto (não mexer)

- **Tarefa 38 / Problema C.1** (`COALESCE(f.periodo, '')` para não descartar silenciosamente faltas históricas com
  `periodo = NULL`): implementado corretamente nas 5 queries de `internal/projections/faltas_projection.go`
  (`GetByID`, `GetByEstudante`, `GetByAcademia`, `GetByPeriodo`, `GetAll` — todas usam `scanFaltas`/`scanFalta` com
  `COALESCE`) e na query de `ListarFaltas` em `internal/handlers/registros_handlers.go` (linha ~219).
- **Tarefa 38 / Problema C.2** (fallback de chave legada em `CorrigirFalta`, `internal/domain/aggregates/estudante_falta.go`):
  implementado, código idêntico ao especificado no documento original.
- **Tarefa 38 / Problema A** (remoção da nota de contrato duplicada de 11 seções de `Documentação da API.md`):
  implementado corretamente — restou exatamente 1 ocorrência, na seção `POST /academia/faltas-aluno/async`, onde
  deveria ficar.
- **Tarefa 39 / Problema A** (sentinel error `errTurmaNaoEncontradaParaVinculo` + `errors.Is` em
  `AdicionarEstudanteATurma`, `internal/handlers/turmas_handler.go`): implementado corretamente, código idêntico ao
  especificado, restaurando o `404` para `codigo_turma` inexistente na rota manual `POST /academia/turma/:codigo/estudante`.
- `migrations/107_periodo_faltas.sql` não foi tocado (correto — não deveria ser).

**Não altere nenhum desses pontos.** Todo o trabalho desta tarefa é em arquivos `*_test.go` novos ou existentes.

### O que falta

**Tarefa 38 — só 2 dos 15 testes obrigatórios existem.** `internal/domain/aggregates/estudante_registros_correcao_test.go`
tem `TestRegistrarFaltaTipoEscolarComPeriodoValido` (cenário 1) e `TestCorrigirFaltaAceitaChaveLegadaSemPeriodo`
(cenário 14). `cmd/server/notas_faltas_correcao_integration_test.go` continua com as mesmas 3 funções de teste que
já existiam antes da Tarefa 38 (nenhuma delas cobre os cenários novos). Os cenários 2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
12, 13 e 15 (13 testes) **não existem em lugar nenhum do repositório**.

**Tarefa 39 — os 11 testes existem apenas como stubs vazios que nunca executam a asserção que prometem.**
`internal/handlers/turma_vinculo_estudante_integration_test.go` foi criado com os 11 nomes de função corretos, mas
cada corpo de teste é apenas:

```go
func TestCadastroIndividualComCodigoTurmaValidoVincula(t *testing.T) {
	t.Parallel()
	requireTurmaVinculoIntegrationDB(t)
	t.Skip("TODO: implementar fixture HTTP completo da tarefa 39")
}
```

Ou seja: mesmo com `SPURI_RUN_DB_INTEGRITY_TESTS=1`, o teste sempre pula antes de qualquer requisição HTTP ou
asserção — incluindo o teste 11, que é justamente a prova de que a correção do Problema A (item já implementado
corretamente) funciona. `go test ./...` "passa" hoje só porque todos os 11 testes se auto-pulam, não porque
validam algo.

**Causa raiz provável do stub vazio (e correção estrutural obrigatória desta tarefa):** o documento original da
Tarefa 39 instruía, na mesma seção, duas coisas incompatíveis entre si:

1. Criar o arquivo de teste em `internal/handlers/turma_vinculo_estudante_integration_test.go` — ou seja, pacote
   `handlers`.
2. "Use o router de produção via `setupRouter()` (mesma função usada pelo fixture de referência)".

Isso é impossível como está escrito: `setupRouter()`, e as variáveis de pacote `dbClient`, `repository` e
`projManager` que ele fecha por clausura, são identificadores não exportados de `package main` em
`cmd/server/main.go`. Não é possível importá-los (nem sequer é possível importar um `package main` como
biblioteca) a partir de `package handlers`. O fixture de referência (`setupRegistrosCorrecaoIntegration`, usado
pela Tarefa 38) só funciona porque vive no próprio `cmd/server` (`package main`) e reatribui temporariamente essas
mesmas variáveis de pacote antes de chamar `setupRouter()`. Isso muito provavelmente é o motivo pelo qual a
implementação anterior parou em `t.Skip(...)` em vez de completar o fixture: a instrução, seguida ao pé da letra,
não compila.

A correção é simples e está detalhada na Parte B abaixo: mover o fixture para `cmd/server` (`package main`),
seguindo exatamente o mesmo padrão já usado e comprovado por `notas_faltas_correcao_integration_test.go`.

## Fora de escopo

- Qualquer alteração em `internal/projections/faltas_projection.go`, `internal/handlers/registros_handlers.go`,
  `internal/domain/aggregates/estudante_falta.go`, `internal/handlers/turmas_handler.go`, `Documentação da API.md`
  ou `migrations/*.sql`. Todos já estão corretos; esta tarefa é exclusivamente sobre testes.
- Qualquer alteração em `internal/handlers/estudante_handlers.go`, `internal/handlers/job_item_handlers.go`,
  `internal/domain/aggregates/turma.go` — a lógica funcional já foi auditada e está correta.
- Renomear, mover ou alterar a assinatura de qualquer função de produção existente.
- Investigar ou corrigir a causa raiz do porquê a implementação anterior deixou os testes como stub — não é
  necessário para esta correção, só a solução importa.

## Escopo obrigatório

### Parte B — Tarefa 39: substituir os 11 stubs por testes reais (fazer primeiro)

#### B.0 — Mover o fixture para o pacote correto

1. **Apague** `internal/handlers/turma_vinculo_estudante_integration_test.go` (o stub inteiro, incluindo
   `requireTurmaVinculoIntegrationDB` e qualquer outro helper nele).
2. **Crie** `cmd/server/turma_vinculo_estudante_integration_test.go`, `package main`, seguindo exatamente o padrão
   de `cmd/server/notas_faltas_correcao_integration_test.go` (guard de `SPURI_RUN_DB_INTEGRITY_TESTS`, troca de
   `dbClient`/`repository`/`projManager`, `client.RunMigrations()`, `t.Cleanup` restaurando o estado anterior,
   `setupRouter()` para o router real, `middleware.GenerateToken` para os tokens).

#### B.1 — Fixture compartilhada

Construa uma função `setupTurmaVinculoIntegration(t *testing.T) *turmaVinculoFixture` no mesmo espírito de
`setupRegistrosCorrecaoIntegration`. Reaproveite `criarAcademiaCorrecao` (já existe no pacote `main`, em
`notas_faltas_correcao_integration_test.go` — não duplique, apenas chame) para criar as academias. Adicione o que
falta:

- **Turma ativa e compatível:**
  ```go
  turma := aggregates.NewTurma()
  if err := turma.Criar("1A", academia.CodigoAcademia, "1_ano_fundamental", nil, "manha", academia.ID); err != nil {
      t.Fatalf("criar turma: %v", err)
  }
  if err := repository.SaveWithAudit(turma, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
      t.Fatalf("salvar turma: %v", err)
  }
  ```
  (turmas nascem com `Status = "ativo"` — ver `aggregates.NewTurma()`, `internal/domain/aggregates/turma.go`.)

- **Turma inativa** (para os cenários 5 e 9): crie uma segunda turma do mesmo jeito e, antes de salvar, chame
  `turma.Desativar(academia.ID)`.

- **Turma de outra academia** (para o cenário 4): use `outraAcademia` (já retornada por `criarAcademiaCorrecao`) e
  crie uma turma nela.

- Rebuild: `projections.NewTurmasProjection(client).Rebuild()` depois de criar as turmas.

- **Não** crie um estudante fixo no fixture como `setupRegistrosCorrecaoIntegration` faz — os testes 1 a 6 e 9
  precisam *cadastrar* o estudante via HTTP (é o próprio objeto do teste), então o estudante deve ser criado
  dentro de cada função de teste, não no setup compartilhado.

#### B.2 — Receita mínima para um cadastro individual válido via multipart

`POST /academia/estudante/register` (`RegisterEstudantePorAcademia`, `internal/handlers/estudante_handlers.go`)
exige `Content-Type: multipart/form-data` (rejeita JSON puro com `400`) e passa sempre por
`registerEstudantePorAcademiaComRequestModo(..., pendenteDocumentos=false)`, o que **não** pula a validação de
documentos (`ValidarDocumentosMatricula`, `internal/domain/aggregates/solicitacao_matricula.go`). Verificado linha
a linha, a combinação mínima de campos que passa nessa validação para um estudante do fundamental com
`ano_escolar_fundamental = "1_ano_fundamental"` (que dispensa comprovativo de ano anterior — ver
`validarComprovativoAcademico`, retorna `nil` direto para `"1_ano_fundamental"`) é:

- **Campos de texto obrigatórios:** `nome`, `genero` (`"masculino"` ou `"feminino"`), `data_nascimento`
  (`YYYY-MM-DD`, anterior a hoje), `ano_escolar_fundamental` = `"1_ano_fundamental"`, `telefone_encarregado`
  (formato válido — reaproveite o helper `geraDigitos`/padrão de telefone já usado em outros testes do pacote, ex.
  `"9" + geraDigitos(8)`), `bilhete_identidade_encarregado` (qualquer string não vazia, distinta do
  `bilhete_identidade` do estudante se este também for enviado — não envie `bilhete_identidade` do próprio
  estudante para simplificar).
- **Arquivos obrigatórios** (campos multipart tipo arquivo, ambos exigidos pela combinação acima — confirmado em
  `ValidarDocumentosMatricula`): `bi_encarregado` (exigido porque `bilhete_identidade_encarregado` foi informado) e
  `cedula_estudante` (exigido porque o `bilhete_identidade` do próprio estudante não foi informado).
- Cada arquivo precisa: `Content-Type: application/pdf`, nome terminando em `.pdf`, e conteúdo começando com a
  assinatura `%PDF` (ver `readAndValidatePDF`, `internal/handlers/solicitacao_matricula_handlers.go` linha ~693).
  Um conteúdo mínimo válido: `[]byte("%PDF-1.4\n%%EOF")`.
- Opcional: `codigo_turma` — é o campo sob teste.

Monte o `multipart.Writer` manualmente com `mime/multipart` (não existe helper pronto no pacote — os testes
existentes que tocam esse endpoint, como `internal/handlers/removed_fields_test.go`, só testam funções auxiliares
isoladas, não o fluxo HTTP completo). Um helper reutilizável nesse novo arquivo de teste, algo como
`montarMultipartCadastroEstudante(t *testing.T, campos map[string]string, comArquivos bool) (*bytes.Buffer, string)`
que devolve o corpo e o `Content-Type` (com boundary), evita repetir esse código em cada um dos ~7 testes que
precisam dele.

#### B.3 — Os 11 testes (numeração original preservada)

Implemente-os em `cmd/server/turma_vinculo_estudante_integration_test.go`. Todos usam `router.ServeHTTP` com
`httptest.NewRecorder()`/`httptest.NewRequest`, autenticação via header `Authorization: Bearer <token>` com o token
da academia gerado por `middleware.GenerateToken`.

1. **Cadastro individual sem `codigo_turma` — regressão.** `POST /academia/estudante/register`, multipart válido
   (ver B.2) sem o campo `codigo_turma`. Confirme status `201` e que o JSON de resposta (`data`) **não** contém as
   chaves `turma_vinculada` nem `turma_aviso`.

2. **Cadastro individual com `codigo_turma` válido — vinculado.** Mesmo request, adicionando `codigo_turma` da
   turma ativa e compatível do fixture. Confirme `201`, `data.turma_vinculada == true`, e confirme via
   `projections.NewTurmasProjection(client).GetByCodigoTurma(codigoTurma, academia.CodigoAcademia)` (depois de
   `Rebuild()`) que o `codigo_estudante` retornado no cadastro está presente em `TurmaDTO.Estudantes`.

3. **`codigo_turma` inexistente — `404`.** Mesmo request com um `codigo_turma` que não existe (ex.:
   `"TURMA_INEXISTENTE"`). Confirme `404` e, consultando `projections.NewEstudanteProjection(client)` pelo nome
   único usado no teste (ou contando linhas de `projection_estudantes` antes/depois do request), confirme que
   **nenhum** estudante foi criado.

4. **`codigo_turma` de outra academia — `404`.** Envie o `codigo_turma` da turma pertencente a `outraAcademia`
   usando o token da primeira academia. Confirme `404` e nenhum estudante criado, igual ao teste 3.

5. **`codigo_turma` inativa — `400`.** Use o `codigo_turma` da turma desativada do fixture. Confirme `400` e
   nenhum estudante criado.

6. **`codigo_turma` incompatível com o estudante — `400`.** Use a turma do fixture (nível `"1_ano_fundamental"`) mas
   envie `ano_escolar_fundamental = "2_ano_fundamental"` no cadastro (adicione um segundo ano válido aos
   `anos_academicos` da academia no fixture, ou crie a turma do fixture com nível diferente do que a academia
   aceita para o estudante — o mais simples é manter a turma em `"1_ano_fundamental"` e cadastrar o estudante com
   `"2_ano_fundamental"`, desde que `"2_ano_fundamental"` também esteja em `academia.AnosAcademicos`; ajuste
   `criarAcademiaCorrecao`/a criação da academia deste fixture para incluir ambos os anos se necessário — **não
   altere** `criarAcademiaCorrecao` em si, construa a academia deste teste diretamente com
   `aggregates.NewAcademia()` + `.Criar(...)` se precisar de uma lista de anos diferente). Confirme `400` e
   mensagem contendo `"incompatível"`, nenhum estudante criado.

7. **Vinculação pós-criação não depende de reconsultar a projeção de estudantes.** Cadastre um estudante com
   `codigo_turma` válido e **não** chame `projections.NewEstudanteProjection(client).Rebuild()` depois — deixe a
   projeção de estudantes deliberadamente desatualizada em relação ao ledger antes/durante a chamada. Confirme que
   o cadastro e o vínculo têm sucesso mesmo assim (`201`, `data.turma_vinculada == true`), provando que
   `vincularEstudanteATurma` não depende de reconsultar `projection_estudantes` para o estudante recém-criado (o
   código já usa os dados vindos do próprio request, não uma releitura da projeção — este teste apenas comprova
   isso).

8. **Cadastro em massa com mistura de itens.** Chame `handlers.RegisterEstudantePorAcademiaJobItem` diretamente,
   um item por vez, replicando o padrão usado pelo worker de produção (`internal/jobs/worker.go`, função
   `processItem`, linha ~337): monte um `*gin.Context` sintético com `gin.CreateTestContext`, `c.Request` apontando
   para um `io.NopCloser` com o JSON do item e `Content-Type: application/json`, e injete manualmente
   `c.Set("user_id", academia.ID)`, `c.Set("user_type", "academia")`, `c.Set("dbClient", client)`,
   `c.Set("repository", repository)`, `c.Set("projManager", projManager)` (mesmas chaves usadas por `setupCtx` em
   `cmd/server/main.go`, função `initJobs`, linha ~157 — reproduza-as, não importe `initJobs` que é privada).
   Diferente do endpoint síncrono, o item de job sem `arquivos` não passa por validação de documentos (ver
   `RegisterEstudantePorAcademiaJobItem`: quando `len(files) == 0`, chama
   `registerEstudantePorAcademiaComRequestModo(..., pendenteDocumentos=true)`, que passa
   `PularValidacaoDocumentos: true` para `ValidateMatriculaCommon`) — ou seja, para este teste **não é necessário**
   montar arquivos PDF, só os campos de texto (incluindo `telefone_encarregado`, ainda obrigatório). Monte 3 itens:
   um sem `codigo_turma`, um com `codigo_turma` válido, um com `codigo_turma` inexistente. Rode cada um e confirme
   que são tratados de forma independente: item 1 e item 2 têm sucesso (HTTP 2xx no `ResponseRecorder`); para o
   item 3, rode primeiro e observe o comportamento real (não adivinhe) — como não há pré-validação HTTP separada
   nesse caminho (a checagem de turma roda dentro de `vincularEstudanteATurma`, chamada depois de
   `SaveWithAudit` do estudante, com erro capturado em `turma_aviso`), o esperado é que o item 3 **também tenha
   sucesso** (2xx) mas com `data.turma_vinculada == false` e `data.turma_aviso` preenchido — confirme isso e fixe o
   `assert` no comportamento observado, documentando no comentário do teste por que difere do endpoint síncrono
   (que tem pré-validação e por isso retorna `404` para o mesmo caso).

9. **Degradação graciosa em falha pós-criação.** Usando o mesmo caminho de item de job do teste 8 (sem
   pré-validação HTTP), cadastre um item com `codigo_turma` apontando para a turma inativa do fixture. Confirme que
   o estudante é criado com sucesso (2xx) e que a resposta contém `turma_aviso` relatando o erro de vinculação
   (mencionando que a turma está inativa/não pode receber estudantes), em vez de abortar o cadastro inteiro.

10. **Conflito de concorrência otimista com retry.** Use `internal/db/repository_concurrency_test.go`
    (`TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable`) como referência de padrão para forçar
    conflito. Crie dois estudantes distintos de antemão (fora da race, para não confundir a asserção com falhas de
    cadastro) e dispare duas goroutines chamando `vincularEstudanteATurma` simultaneamente para a mesma turma, uma
    para cada estudante (`sync.WaitGroup` + canal de erro). Confirme que ambas eventualmente têm sucesso (o retry
    único absorve o conflito de versão) — se, num cenário raro, uma colidir duas vezes seguidas e falhar, o teste
    deve aceitar isso desde que o erro seja limpo (sem panic, sem corromper o agregado) e deve então repetir a
    chamada falha de forma síncrona para confirmar que ela teria sucesso em condições normais. Ao final, releia a
    turma via `projections.NewTurmasProjection(client).GetByCodigoTurma(...)` (após `Rebuild()`) e confirme que
    ambos os `codigo_estudante` estão presentes em `Estudantes`, sem perda de nenhum dos dois.

11. **`AdicionarEstudanteATurma` (rota manual) sem regressão — prova do item A já implementado.** `POST
    /academia/turma/:codigo/estudante` com corpo `{"codigo_estudante": "..."}`. Cubra três sub-casos no mesmo teste
    (ou em subtests com `t.Run`):
    - (a) Estudante existente (crie um sem `codigo_turma` no cadastro, ou diretamente via aggregate) e turma ativa
      compatível → `200`.
    - (b) `codigo_turma` inexistente no path → **`404`** (esta é a asserção que hoje falharia sem a correção do
      item A da Tarefa 39 — é a única forma de garantir que uma regressão futura nesse ponto específico seja
      detectada).
    - (c) Um estudante que já foi vinculado a outra turma ativa (use o resultado do sub-caso (a) ou vincule
      manualmente antes) tentando vincular a uma segunda turma ativa → `400` com mensagem mencionando duplicidade
      (`"já pertence à turma"`).

### Parte A — Tarefa 38: implementar os 13 testes ausentes (cenários 2–13 e 15)

Os testes de agregado (sem banco) vão em `internal/domain/aggregates/estudante_registros_correcao_test.go`,
reaproveitando `estudanteParaRegistro()` já existente no pacote. Os testes de integração HTTP + banco real vão em
`cmd/server/notas_faltas_correcao_integration_test.go`, reaproveitando e estendendo
`setupRegistrosCorrecaoIntegration` (guard `SPURI_RUN_DB_INTEGRITY_TESTS`, já existente — não duplique a criação de
academia/estudante/matéria fundamental que o fixture atual já monta).

#### A.1 — Extensão de fixture necessária para os cenários 4 e 5 (matéria tipo `superior`)

O fixture atual só tem uma matéria `fundamental` e um estudante com `AnoEscolar` preenchido. Os cenários 4 e 5
exigem uma matéria `superior` com período fixo — e, como `inferirAnoAcademicoFaltas` (`internal/handlers/notas_handlers.go`,
linha ~790) usa **apenas** `estudanteDTO.AnoEscolar`/`AnoEscolarMedio` (nunca `AnoSuperior`) para inferir
compatibilidade, reaproveitar o estudante fundamental existente do fixture faria esses dois testes falharem por um
motivo não relacionado a `periodo` (mismatch de ano acadêmico). Adicione ao fixture, sem alterar o que já existe:

```go
// Segundo estudante, sem ano fundamental/médio, para os testes de matéria "superior".
estudanteSuperior := aggregates.NewEstudante()
anoSuperior := "1_ano_superior"
codigoAlunoSuperior := fmt.Sprintf("%07d", (sequence+1)%10_000_000)
if err := estudanteSuperior.CriarComVinculo("Aluno superior de integração", codigoAlunoSuperior, "hash", nil, nil, nil, nil, nil, "F", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), nil, nil, &anoSuperior, nil, nil, &academia.ID, academia.CodigoAcademia); err != nil {
    t.Fatalf("criar estudante superior: %v", err)
}
if err := repository.SaveWithAudit(estudanteSuperior, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
    t.Fatalf("salvar estudante superior: %v", err)
}

cursoSuperiorID := uuid.New()
if _, err := client.DB().Exec(`
    INSERT INTO projection_cursos (id, nome, type, nivel, periodos, codigo_academia, status, created_at)
    VALUES ($1, 'Curso Superior integração', 'superior', '[]'::jsonb, '["1_semestre","2_semestre"]'::jsonb, $2, 'ativo', CURRENT_TIMESTAMP)
`, cursoSuperiorID, academia.CodigoAcademia); err != nil {
    t.Fatalf("inserir curso superior: %v", err)
}

materiaSuperiorID := uuid.New()
if _, err := client.DB().Exec(`
    INSERT INTO projection_materias (id, nome, type, codigo_academia, curso_id, periodo, anos_academicos, status, created_at)
    VALUES ($1, 'Cálculo I integração', 'superior', $2, $3, '1_semestre', '["1_ano_superior"]'::jsonb, 'ativo', CURRENT_TIMESTAMP)
`, materiaSuperiorID, academia.CodigoAcademia, cursoSuperiorID); err != nil {
    t.Fatalf("inserir materia superior: %v", err)
}
if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
    t.Fatalf("rebuild estudantes: %v", err)
}
```

Guarde `codigoAlunoSuperior` e `materiaSuperiorID` na struct `registrosCorrecaoFixture` (novos campos) para uso
pelos testes 4 e 5.

#### A.2 — Os 13 testes (numeração original preservada, cenário 1 e 14 já implementados e não devem ser duplicados)

2. **Registrar falta tipo escolar com `periodo` fora do conjunto fixo (ex.: `"4_trimestre"`) — `400`.** Teste HTTP:
   `POST /academia/faltas-aluno` com `periodo: "4_trimestre"` para a matéria fundamental do fixture. Confirme
   status `400`.

3. **Registrar falta tipo escolar sem `periodo` — `400`.** Teste HTTP: omita `periodo` no corpo. Confirme `400` e
   que a mensagem de erro menciona `periodo` — rode o teste primeiro para capturar a mensagem exata retornada por
   `decodeStrictJSON`/`binding:"required"` (`internal/handlers/faltas_handlers.go`, linha ~59) antes de fixar o
   `assert`, em vez de adivinhar a string.

4. **Registrar falta tipo superior com `periodo` igual ao período fixo da matéria — sucesso.** Usando o fixture de
   A.1: `POST /academia/faltas-aluno` com `codigo_estudante: codigoAlunoSuperior`, `materia_disciplinar_id:
   materiaSuperiorID`, `periodo: "1_semestre"`. Confirme `201`/sucesso.

5. **Registrar falta tipo superior com `periodo` diferente do período fixo da matéria — `400`.** Mesmo fixture,
   `periodo: "2_semestre"` (válido para o curso, mas diferente do `"1_semestre"` fixado na matéria). Confirme `400`
   e mensagem exata `"periodo '2_semestre' invalido para a materia 'Cálculo I integração'. Periodo definido:
   '1_semestre'"` (formato confirmado em `internal/handlers/faltas_handlers.go`, linha ~131).

6. **`PATCH /academia/faltas-aluno/:id` enviando `periodo` no corpo — rejeitado.** Registre uma falta normalmente
   (matéria fundamental do fixture), depois tente `PATCH .../faltas-aluno/:id` incluindo `"periodo": "2_trimestre"`
   no corpo. Confirme que é rejeitado — capture o status/mensagem reais retornados por
   `rejeitarCamposLegadosSumarioFaltas(c, "periodo")` (primeira linha de `CorrigirFalta`,
   `internal/handlers/faltas_handlers.go`, linha ~207) e fixe o `assert` no comportamento observado.

7. **`PATCH` sem `periodo` — sucesso, período do registro permanece o do lançamento original.** Registre uma falta
   com `periodo="1_trimestre"`, corrija quantidade/observação via `PATCH` sem tocar em `periodo`, confirme via
   `GET /faltas-estudante/:codigo` subsequente que o `periodo` do registro continua `"1_trimestre"`.

8. **`GET /faltas?periodo=1_trimestre` — retorna só faltas com esse período.** Registre duas faltas do mesmo
   estudante em matérias/dias diferentes com períodos diferentes (`1_trimestre` e `2_trimestre`), confirme que o
   filtro `?periodo=1_trimestre` retorna só a correta.

9. **`GET /faltas-estudante/:codigo?periodo=2_semestre` — mesmo comportamento, endpoint por estudante.** Análogo ao
   8, usando `GetFaltasEstudante`.

10. **`FaltasProjection.Rebuild()` preserva `periodo` corretamente.** Registre faltas com período (aproveite as já
    registradas em testes anteriores ou registre uma nova), chame `projections.NewFaltasProjection(client).Rebuild()`,
    confirme via `GetByID` ou query direta em `projection_faltas` que `periodo` continua correto após o rebuild.

11. **Duas faltas mesmo estudante/matéria/data com `periodo` diferente — ambas aceitas.** Registre a mesma falta
    (estudante, academia, data, matéria) duas vezes, uma com `periodo="1_trimestre"` e outra com
    `periodo="2_trimestre"`. Confirme que **ambas** são aceitas (`201` nas duas) — a constraint `uq_falta_unica`
    agora inclui `periodo` (ver `migrations/107_periodo_faltas.sql`). Documente no comentário do teste que esse é
    um comportamento de borda intencional desde a Tarefa 34.

12. **Migração 107 sobre base pré-existente sem `periodo`.** Não re-rode só a migração 107 isoladamente (o fixture
    já roda `client.RunMigrations()` completo, incluindo a 107, antes de qualquer dado existir). Em vez disso,
    replique o comportamento da migração diretamente contra dados inseridos deliberadamente no formato
    "pré-Tarefa-34" *depois* que o schema já existe: (a) insira uma falta em `projection_faltas` com `periodo =
    NULL` para uma matéria `type='superior'` cuja `projection_materias.periodo` esteja preenchida (reaproveite
    `materiaSuperiorID` de A.1); (b) insira outra falta com `periodo = NULL` para a matéria `fundamental` do
    fixture original (sem fonte determinística de período); (c) rode literalmente a mesma instrução `UPDATE` que
    a migração 107 usa para backfill (copie o bloco `UPDATE projection_faltas f SET periodo = m.periodo FROM
    projection_materias m WHERE ... AND m.type = 'superior' AND m.periodo IS NOT NULL AND f.periodo IS NULL` de
    `migrations/107_periodo_faltas.sql`) contra essas duas linhas; (d) confirme que a falta da matéria superior
    recebeu o backfill correto (`periodo = '1_semestre'`) e que a falta da matéria fundamental permanece com
    `periodo IS NULL` no banco, sem nenhum erro/abort; (e) confirme, usando `FaltasProjection.GetByEstudante`
    (já corrigido pelo item C.1 da Tarefa 38), que essa falta com `periodo NULL` no banco aparece na resposta
    normalmente com `Periodo == ""`, em vez de desaparecer.

13. **`FaltasProjection.GetByPeriodo` (intervalo de datas) continua funcionando, agora retornando `periodo`
    preenchido.** Chame `projections.NewFaltasProjection(client).GetByPeriodo(codigoEstudante, anoLectivo,
    dataInicio, dataFim)` diretamente (não é um teste HTTP), como já era chamado antes da Tarefa 34. Confirme que o
    filtro por intervalo de datas continua correto e que cada `FaltaDTO` retornado tem o campo `Periodo` preenchido
    corretamente.

15. **Integração HTTP + banco: falta histórica sem `periodo` é listável e corrigível ponta a ponta.** Estenda o
    fixture (ou crie uma variação local no mesmo arquivo de teste) para inserir diretamente no banco, imitando um
    dado pré-Tarefa-34: (a) insira uma falta em `projection_faltas` com `periodo = NULL`, associada a um
    `codigo_estudante`/`materia_disciplinar_id`/`data` reais do fixture; (b) grave também um evento
    `FaltasRegistradas` correspondente na tabela `spuri_ledger` — a forma mais simples é deixar `RegistrarFalta`
    gravar o evento normalmente e depois rodar
    `UPDATE spuri_ledger SET payload = payload - 'Periodo' WHERE event_type = 'FaltasRegistradas' AND aggregate_id = $1 AND ...`
    (operador `jsonb - text` do Postgres remove a chave `"Periodo"` do JSON, simulando um payload gravado antes da
    Tarefa 34 — confirme com uma query que a chave sumiu do `payload` antes de prosseguir) — e depois sobrescreva
    `periodo = NULL` na linha correspondente de `projection_faltas` (o ledger e a projeção não precisam ficar
    perfeitamente consistentes para este teste: o objetivo é só simular o estado "pré-migração" em ambos os
    lugares que o código lê). `AggregateRepository.Load` não valida a cadeia de hash do ledger na leitura
    (confirmado em `internal/db/repository.go`), então essa manipulação direta não quebra o replay. Depois: (i)
    confirme via `GET /faltas-estudante/:codigo` que essa falta **aparece** na resposta com `"periodo": ""` (prova
    de C.1); (ii) confirme via `PATCH /academia/faltas-aluno/:id` que a correção dessa falta **funciona** (`200`,
    prova de C.2 ponta a ponta).

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual de `main`.
2. Parte B.0: apagar o stub `internal/handlers/turma_vinculo_estudante_integration_test.go`; criar
   `cmd/server/turma_vinculo_estudante_integration_test.go`.
3. Parte B.1/B.2: implementar o fixture compartilhado e o helper de multipart.
4. Parte B.3: implementar o teste 11 primeiro (é a prova da correção A da Tarefa 39, já aplicada) e confirmar que
   ele de fato passa; implementar os testes 1 a 10 restantes.
5. Parte A.1: estender `setupRegistrosCorrecaoIntegration` com o segundo estudante/curso/matéria superior.
6. Parte A.2: implementar os 13 testes (2–13, 15) da Tarefa 38, distribuindo entre agregado e integração HTTP
   conforme cada um indica.
7. Rodar `gofmt -l .` (sem saída) e `go build ./...`.
8. Rodar `go vet ./...`.
9. Rodar `go test ./...` e, para os testes de integração,
   `SPURI_RUN_DB_INTEGRITY_TESTS=1 go test ./... -run <padrão relevante>` contra uma base PostgreSQL isolada de
   testes — confirme que **nenhum** teste novo aparece como `SKIP` nessa execução (os 24 devem de fato rodar e
   passar, não pular).
10. Revisar o diff completo: só devem aparecer arquivos `*_test.go` (novos ou alterados) e a remoção do arquivo
    stub de `internal/handlers/`. Nenhum arquivo de código de produção deve mudar.

## Critérios de aceite

- [ ] `internal/handlers/turma_vinculo_estudante_integration_test.go` não existe mais.
- [ ] `cmd/server/turma_vinculo_estudante_integration_test.go` existe, `package main`, e implementa os 11 testes da
      Parte B com nomes claros o suficiente para identificar qual cenário cada um cobre.
- [ ] Nenhum dos 11 testes de B contém `t.Skip` incondicional — o único `t.Skip` permitido é o guard de
      `SPURI_RUN_DB_INTEGRITY_TESTS != "1"`, idêntico ao padrão de `setupRegistrosCorrecaoIntegration`.
- [ ] O teste 11 de B cobre explicitamente os três sub-casos (a), (b) e (c), incluindo a asserção de `404` para
      `codigo_turma` inexistente na rota manual.
- [ ] Os 13 testes da Parte A (cenários 2–13 e 15) estão implementados, com nomes claros o suficiente para
      identificar qual cenário cada um cobre, e não duplicam os cenários 1 e 14 já existentes.
- [ ] Todos os 24 testes novos passam localmente com `SPURI_RUN_DB_INTEGRITY_TESTS=1` contra uma base PostgreSQL
      real (documentar no PR que essa execução foi feita, com o comando exato usado).
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `gofmt -l .` sem saída.
- [ ] `go test ./...` passa (sem `SPURI_RUN_DB_INTEGRITY_TESTS`, os 24 testes novos devem pular via guard, não
      falhar).
- [ ] Nenhum arquivo de código de produção (fora de `*_test.go`) foi alterado.

## Procedimento de conclusão

Ao finalizar a implementação:

1. Atualizar o front matter deste arquivo para `status: feito`.
2. Mover este arquivo para `docs/Tarefas feitas/`.
3. No PR, listar explicitamente o comando usado para rodar os testes de integração contra banco real e confirmar
   que os 24 testes novos passaram (não pularam) nessa execução.
