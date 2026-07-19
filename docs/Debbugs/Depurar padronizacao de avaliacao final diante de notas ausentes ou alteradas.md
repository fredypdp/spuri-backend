---
modificado: 2026-07-19 00:00
criado: 2026-07-19 00:00
---

# Depurar padronização de avaliação final diante de notas ausentes ou alteradas

## Escopo auditado

Auditoria crítica da tarefa concluída em `docs/Tarefas feitas/02 - Padronizar avaliação final diante de notas ausentes ou alteradas.md`, com foco em confirmar se o backend passou a tratar notas exigidas e ausentes como `0` no momento do gatilho de avaliação final, sem criar mecanismos de edição de notas e sem divergência silenciosa entre o modelo escolar fixo e o modelo configurável do Superior.

Arquivos e pontos verificados:

- `internal/handlers/avaliacao_final_handler.go`
- `internal/handlers/avaliacao_final_regras.go`
- `internal/handlers/avaliacao_final_formula_test.go`
- `internal/domain/aggregates/estudante_avaliacao.go`
- `internal/projections/avaliacao_final_projection.go`
- `Documentação.md`

## Resultado da auditoria

A implementação está correta para a regra principal: quando uma avaliação automática é disparada por uma nota-gatilho de uma matéria, as demais referências exigidas pela fórmula para essa mesma matéria são preenchidas com `0` antes do cálculo.

Não foi necessário alterar código de produção nesta depuração. A tarefa já estava implementada de forma compatível com os critérios de aceite.

## Evidências técnicas

### 1. Substituição por zero no ponto comum de cálculo

O cálculo por matéria usa `calcularResultadoMateriasAvaliacaoFinal`, que é chamado por `executarRegraAvaliacaoFinalAutomatica` para regras escolares fixas e regras configuráveis do Superior. Isso mantém o comportamento em um ponto comum, sem bifurcação de regra de negócio entre níveis.

Dentro desse fluxo, quando existe `overlay` da nota recém-lançada para a matéria avaliada, `substituirNotasAusentesPorZero` é executado antes de `calcularFormulaAvaliacao`. Assim, referências exigidas pela fórmula e ainda sem nota registrada passam a ter uma entrada `0` no mapa de notas usado no cálculo.

### 2. Escopo restrito à matéria que recebeu o gatilho

A função ignora matérias diferentes da matéria do `overlay` quando a avaliação é disparada automaticamente por uma nota específica. Com isso, matérias do mesmo estudante que ainda não receberam a própria nota-gatilho não são avaliadas nem forçadas com zeros.

### 3. Suporte ao Superior com período inferido

Para `nivel="superior"`, a fórmula configurável continua sem período explícito no contrato público. Durante a execução, o backend preenche o período da fórmula com o período da matéria avaliada antes de buscar notas, substituir ausências por zero e calcular a nota final.

### 4. Auditoria no snapshot

O aggregate de avaliação final inclui `ResultadoMateriaAvaliacaoFinal.NotasSubstituidasZero`, serializado como `notas_substituidas_zero`. Esse campo é populado por matéria e persiste no payload do evento/projeção junto de `materia_id`, `nota_final`, `aprovado`, `type`, `formula_snapshot`, `regra_avaliacao_final_id` e `pendencia_permitida`.

### 5. Regras descendentes e recurso

A mesma função de cálculo por matéria é usada para regras descendentes. O fluxo filtra o `exame_recurso` para matérias reprovadas anteriormente e rejeita recurso para matéria já aprovada, preservando o escopo da etapa descendente. Quando uma nota de recurso dispara a regra, ausências exigidas pela fórmula dessa etapa também são substituídas por zero apenas para a matéria disparada.

### 6. Testes existentes

Os testes unitários de fórmula cobrem:

- cálculo normal com todas as notas presentes;
- substituição de múltiplas notas ausentes por zero;
- ausência de substituição quando todas as notas exigidas existem;
- Superior com período inferido antes da substituição por zero;
- comportamento base de `calcularFormulaAvaliacao`, que ainda retorna erro de `nota ausente` quando a substituição não é solicitada.

### 7. Documentação viva

`Documentação.md` registra a mudança de comportamento, o escopo por matéria, o campo `notas_substituidas_zero` no snapshot, os gatilhos escolares fixos e a regra futura para reavaliação após eventual correção/edição de notas.

A documentação também deixa explícito que notas continuam imutáveis na versão atual e que a regra de reavaliação futura é uma decisão de produto, não um comportamento implementado agora.

## Checagens executadas

- `rg` para localizar `nota_despertadora`, substituição por zero, referências a nota ausente, fórmula e documentação relacionada.
- Inspeção manual dos handlers de avaliação final, parser/cálculo de fórmula, aggregate de evento e projeção.
- Tentativa de executar `go test ./...`.

## Limitação encontrada nos testes

`go test ./...` não concluiu porque o pacote `github.com/t3rm1n4l/go-mega` está ausente de `go.mod`, apesar de haver entradas em `go.sum`, e o ambiente bloqueou a resolução do módulo via proxy e via acesso direto ao GitHub com HTTP 403. Essa falha é externa à tarefa auditada e impede a compilação completa do repositório neste ambiente.

## Conclusão

A atualização documentada na tarefa foi implementada corretamente. Nenhum ajuste adicional de código foi necessário nesta depuração.
