---
criado: 2026-08-14 00:00
origem: Depuração pós-implementação da Tarefa 34 (docs/Tarefas feitas/34 - Adicionar periodo as faltas.md) — conversa com Claude, o orquestrador; execução por Codex
status: pendente
prioridade: média — documentação incorreta induz o consumidor da API a erro; ausência de testes é dívida de qualidade sem sintoma em produção ainda
repositorio: fredypdp/spuri-backend (branch main)
---

# 37 — Corrigir pendências da Tarefa 34 (nota de contrato duplicada e testes de periodo em faltas ausentes)

## Prompt recomendado para executar a atualização

Esta tarefa corrige duas pendências encontradas numa depuração pós-implementação da Tarefa 34 (`docs/Tarefas feitas/34 - Adicionar periodo as faltas.md`, commit `cf0f2be`). O código de domínio, handlers, projeção e migração da Tarefa 34 foi auditado linha a linha e está correto — **não altere nenhum arquivo em `internal/domain/aggregates/`, `internal/handlers/faltas_handlers.go`, `internal/handlers/registros_handlers.go`, `internal/projections/faltas_projection.go` ou `migrations/107_periodo_faltas.sql`** como parte desta tarefa; eles não precisam de nenhuma mudança. Implemente exatamente os dois itens da seção "Escopo obrigatório" abaixo: (A) remover a nota de contrato duplicada indevidamente em 11 seções não relacionadas de `Documentação da API.md`, mantendo-a apenas onde pertence; (B) implementar os 13 testes obrigatórios que a especificação original da Tarefa 34 (seção 5) exigia e que não foram criados. Ao final, rode `gofmt`, `go build ./...`, `go vet ./...` e `go test ./...`, e confirme cada item de "Critérios de aceite".

## Contexto

A Tarefa 34 implementou o campo `periodo` em `Falta`, espelhando `Nota`. A implementação de código (domínio, handler, projeção, migração `107_periodo_faltas.sql`) foi auditada e está correta e completa. Duas coisas, porém, ficaram pendentes:

### Problema A — nota de contrato "vazou" para 11 endpoints não relacionados

O commit `cf0f2be` adicionou a seguinte frase em `Documentação da API.md`:

> **Nota de contrato:** esta é uma mudança breaking; `POST /academia/faltas-aluno` e `POST /academia/faltas-aluno/async` rejeitam itens sem `periodo`.

Essa frase aparece **12 vezes** no arquivo. Apenas **1 ocorrência** está no lugar certo (na seção `POST /academia/faltas-aluno/async`). As outras **11** foram inseridas por engano em seções de endpoints completamente alheios a faltas — aparentemente por uma inserção automatizada baseada em padrão de texto (ex.: "logo após todo bloco de `Response 202: job assíncrono`"), sem verificar em qual seção estava sendo inserida. As seções afetadas indevidamente, na ordem em que aparecem no arquivo, são:

1. `PUT /academia/materia/ativar/async`
2. `PUT /academia/materia/desativar/async`
3. `PUT /academia/materia/dados/async`
4. `DELETE /academia/materia/async`
5. `POST /academia/turma/async`
6. `POST /academia/turma/estudante/async`
7. `PUT /academia/turma/ativar/async`
8. `PUT /academia/turma/desativar/async`
9. `PUT /academia/turma/dados/async`
10. `DELETE /academia/turma/async`
11. `DELETE /academia/turma/estudante/async`

Isso é uma documentação enganosa: um integrador lendo a seção de matérias ou turmas em lote encontra uma nota alertando sobre um campo `periodo` obrigatório em faltas, que nada tem a ver com o endpoint que está lendo.

### Problema B — testes obrigatórios da seção 5 da Tarefa 34 não foram implementados

O commit `cf0f2be` só ajustou a **assinatura** de chamadas já existentes em dois arquivos de teste para continuarem compilando após a mudança de assinatura de `RegistrarFalta`/`CorrigirFalta` (`internal/domain/aggregates/estudante_registros_correcao_test.go` e `cmd/server/notas_faltas_correcao_integration_test.go`). Nenhum teste novo foi criado. A especificação original da Tarefa 34, seção "5. Testes obrigatórios", listava 13 cenários — nenhum deles tem cobertura dedicada hoje. Isso viola o critério de aceite nº 6 da Tarefa 34 ("os testes da seção 5 estiverem implementados e passando, incluindo o rebuild de projeção").

## Objetivo

1. Documentação: a nota de contrato sobre `periodo` obrigatório em faltas deve existir **apenas** na seção `POST /academia/faltas-aluno/async`.
2. Testes: os 13 cenários da seção 5 da Tarefa 34 devem estar implementados e passando.

## Escopo obrigatório

### A. `Documentação da API.md`

Remover a linha (e a linha em branco imediatamente associada a ela, mantendo o espaçamento igual ao resto do documento) `**Nota de contrato:** esta é uma mudança breaking; \`POST /academia/faltas-aluno\` e \`POST /academia/faltas-aluno/async\` rejeitam itens sem \`periodo\`.` das 11 seções listadas no Contexto acima. **Não remover** a ocorrência que está corretamente posicionada na seção `POST /academia/faltas-aluno/async` (a mais próxima do final do arquivo, junto ao exemplo de request em array de faltas com `periodo`).

Confirme ao final que `grep -n "Nota de contrato" "Documentação da API.md"` retorna exatamente **uma** ocorrência.

### B. Testes da seção 5 da Tarefa 34

Implemente os 13 testes a seguir. Reaproveite os padrões já usados no repositório para os equivalentes de `Nota` e para faltas onde já existirem, para manter consistência de estilo:

- Testes de agregado (sem banco de dados) seguem o padrão de `internal/domain/aggregates/estudante_registros_correcao_test.go` (função helper `estudanteParaRegistro()` já existe nesse arquivo/pacote).
- Testes HTTP + banco real seguem o padrão de `cmd/server/notas_faltas_correcao_integration_test.go`, incluindo o guard `if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" { t.Skip(...) }` e o fixture `setupRegistrosCorrecaoIntegration` (reaproveite ou estenda esse fixture em vez de duplicar a criação de academia/estudante/matéria).

Lista de testes obrigatórios (numeração igual à da especificação original, para rastreabilidade):

1. Registrar falta tipo escolar com `periodo="1_trimestre"` — sucesso.
2. Registrar falta tipo escolar com `periodo` fora do conjunto fixo (ex.: `"4_trimestre"`) — `400`.
3. Registrar falta tipo escolar sem `periodo` — `400`, mensagem listando o campo como obrigatório.
4. Registrar falta tipo superior com `periodo` igual ao período fixo já configurado na matéria — sucesso.
5. Registrar falta tipo superior com `periodo` diferente do período fixo da matéria — `400`, mesma mensagem/padrão usado em `RegistrarNota` (`"periodo '%s' invalido para a materia '%s'. Periodo definido: '%s'"`).
6. `PATCH /academia/faltas-aluno/:id` enviando `periodo` no corpo — rejeitado (campo não suportado nesta rota; ver `rejeitarCamposLegadosSumarioFaltas(c, "periodo")` em `CorrigirFalta`).
7. `PATCH /academia/faltas-aluno/:id` sem enviar `periodo` — sucesso, `periodo` do registro permanece o do lançamento original.
8. `GET /faltas` com filtro `?periodo=1_trimestre` — retorna somente faltas cujo **próprio registro** tem esse período, não mais inferido pela matéria.
9. `GET /faltas-estudante/:codigo?periodo=2_semestre` — mesmo comportamento acima, para o endpoint por estudante.
10. Rebuild da projeção `faltas` (`FaltasProjection.Rebuild()`) preserva `periodo` corretamente a partir do ledger.
11. Duas faltas do mesmo estudante/matéria/data (as quatro colunas que já compunham `uq_falta_unica` antes desta mudança) mas com `periodo` diferente — como `periodo` passa a integrar a constraint, ambos os registros são aceitos; documentar esse comportamento de borda no próprio teste.
12. Migração `107_periodo_faltas.sql` aplicada sobre uma base com faltas pré-existentes sem `periodo` — confirmar que o backfill determinístico (matérias tipo superior) funciona e que a migração aborta com erro claro se restarem linhas sem período determinável, em vez de aplicar `NOT NULL` silenciosamente sobre dado incompleto.
13. Teste de regressão confirmando que `FaltasProjection.GetByPeriodo` (a função de intervalo de datas, não renomeada) continua funcionando sem alteração de comportamento, apenas retornando `periodo` a mais em cada `FaltaDTO`.

## Fora de escopo

- Qualquer alteração em `internal/domain/aggregates/estudante_falta.go`, `internal/handlers/faltas_handlers.go`, `internal/handlers/registros_handlers.go`, `internal/projections/faltas_projection.go` ou `migrations/107_periodo_faltas.sql` — o código já está correto; esta tarefa é só documentação + testes.
- Qualquer alteração em `Documentação da API.md` fora da remoção das 11 ocorrências indevidas listadas na seção A.
- Renomear ou alterar a assinatura de `FaltasProjection.GetByPeriodo` — permanece fora de escopo, como já determinado pela Tarefa 34 original.
- Investigar a causa raiz de como a duplicação da nota de contrato ocorreu (não é necessário para a correção; só remover o resultado).

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual.
2. Aplicar a correção A em `Documentação da API.md` e confirmar com `grep -n "Nota de contrato" "Documentação da API.md"` que sobra exatamente uma ocorrência, na seção correta.
3. Implementar os testes da seção B, distribuindo entre testes de agregado (sem banco) e testes de integração HTTP (com banco), conforme a natureza de cada cenário.
4. Rodar `gofmt -l .` (sem saída) e `go build ./...`.
5. Rodar `go vet ./...`.
6. Rodar `go test ./...` e, para os testes de integração que exigem banco, `SPURI_RUN_DB_INTEGRITY_TESTS=1 go test ./... -run <padrão relevante>` contra uma base PostgreSQL isolada de testes.
7. Revisar o diff completo e confirmar que só os arquivos de documentação e de teste foram alterados/criados.

## Critérios de aceite

- [ ] `Documentação da API.md` contém exatamente uma ocorrência da nota de contrato sobre `periodo` obrigatório em faltas, na seção `POST /academia/faltas-aluno/async`.
- [ ] As 11 seções listadas no Contexto (Problema A) não contêm mais essa nota.
- [ ] Os 13 testes da seção B estão implementados, com nomes de teste claros o suficiente para identificar qual cenário da lista cada um cobre.
- [ ] Nenhum arquivo de código de produção (`internal/...` fora de `_test.go`, `migrations/...`) foi alterado.
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `go test ./...` passa; testes de integração que dependem de `SPURI_RUN_DB_INTEGRITY_TESTS=1` foram executados pelo menos uma vez contra uma base real e confirmados passando (documentar no PR que isso foi feito, já que esses testes são pulados por padrão).

## Procedimento de conclusão

Ao finalizar a implementação:

1. Atualizar o título interno desta tarefa para `# 37 — Corrigir pendências da Tarefa 34 (nota de contrato duplicada e testes de periodo em faltas ausentes) (feito)`;
2. Alterar o front matter para `status: feito`;
3. Mover este arquivo para `docs/Tarefas feitas/`.
