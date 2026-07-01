---
modificado: 2026-06-30 21:09
criado: 2026-06-30 21:09
---
# Resumo da implementação — Corrigir regra de avaliação final automática por matéria e pendências

## Objetivo do documento

Este documento resume, de forma auditável e fácil de revisar, o que foi implementado, alterado ou removido para atender à tarefa [[Corrigir regra de avaliacao final automatica por materia e pendencias]].

A implementação atual reforça o contrato público de regras de avaliação final baseado em `nivel`, prepara a persistência para resultados por matéria e pendências, adiciona campos de pendência em matérias e documenta o comportamento esperado na API e na documentação funcional do Spuri.

## Arquivos principais envolvidos

- `internal/handlers/avaliacao_final_regras.go`
- `internal/handlers/avaliacao_final_handler.go`
- `internal/handlers/avaliacao_final_formula_test.go`
- `internal/domain/aggregates/estudante_avaliacao.go`
- `internal/domain/aggregates/materia_disciplinar.go`
- `internal/projections/avaliacao_final_projection.go`
- `internal/projections/materias_projection.go`
- `migrations/077_materia_pendencia_permitida.sql`
- `migrations/078_materia_pendencia_nivel_conclusao.sql`
- `migrations/079_avaliacao_final_nivel_materias_pendentes.sql`
- `docs/Spuri - API.md`
- `docs/Spuri - Documentação.md`

## Implementado

### 1. Contrato de regras com `nivel`

- O contrato de regra de avaliação final passou a usar `nivel` como campo público da regra.
- O campo legado `tipo_ensino` é rejeitado explicitamente no `POST /academia/avaliacao-final/regras`.
- O `PUT /academia/avaliacao-final/regras/:id` também rejeita `tipo_ensino`, evitando que o contrato legado volte por edição.
- O backend preenche ou valida `nivel` conforme a academia autenticada:
  - academia superior força `nivel='superior'`;
  - academia escolar fundamental força ou valida `nivel='fundamental'`;
  - academia escolar média força ou valida `nivel='medio'`;
  - academia mista exige que o payload informe `fundamental` ou `medio`.

### 2. Validações por nível na configuração da regra

- `anos_academicos` é obrigatório apenas para regras `fundamental`.
- `anos_academicos` é rejeitado para regras `medio` e `superior`.
- `limite_materias_pendentes` é rejeitado para `fundamental`.
- `limite_materias_pendentes` é obrigatório e não pode ser negativo para `medio` e `superior`.
- `materias_chave` é obrigatório para regra raiz de `nivel='medio'`.
- Payloads de edição não podem alterar `nivel` nem `anos_academicos`; para mudar o escopo, a regra deve ser recriada.

### 3. Fórmula declarativa e validação por nível

- A fórmula continua usando o parser próprio do backend, sem `eval` e sem execução de código vindo do payload.
- Para `fundamental` e `medio`, cada referência de nota deve informar período explicitamente, no formato `[categoria,periodo]`.
- Para `superior`, cada referência deve ser enviada sem período, no formato `[categoria]`; o período é inferido pelo backend no momento da execução.
- Fórmulas de `superior` com período explícito agora falham com erro claro.
- A criação e a edição de regras usam a validação de fórmula por `nivel`.

### 4. Preparação de escopo por matérias e pendências

- A migração `079_avaliacao_final_nivel_materias_pendentes.sql` renomeia `tipo_ensino` para `nivel` na tabela de regras de avaliação final.
- Foram adicionados campos para suportar:
  - `materias_chave`;
  - `materias_aplicaveis`;
  - `limite_materias_pendentes`;
  - `resultados_materias_snapshot` em regras;
  - `resultados_materias` em avaliações finais;
  - `aprovado_com_pendencia`;
  - `pendencias_geradas`.
- Foi criada a tabela `projection_materias_pendentes` para armazenar pendências abertas e históricas por estudante, matéria, academia, curso, nível, ano letivo e escopo acadêmico.
- Foi criado índice único parcial para impedir pendência aberta duplicada no mesmo escopo.

### 5. Campos de pendência em matérias

- `pendencia_permitida` foi adicionado para matérias de `medio` e `superior`.
- `pendencia_nivel_conclusao` foi adicionado para controlar bloqueio de progressão/conclusão quando a matéria pendente está em etapa de conclusão.
- A criação e atualização de matérias rejeitam uso desses campos em matérias `fundamental`.
- A projeção de matérias persiste e expõe esses campos.

### 6. Execução automática e idempotência

- A avaliação automática por lançamento de nota consulta a cadeia de regras aplicável ao estudante, nível e ano/período atual.
- Regras descendentes só são executadas quando a regra dependida foi reprovada.
- A verificação de avaliação já existente considera estudante, academia, ano letivo, nível, escopo acadêmico e `type` da regra.
- Para `superior`, o período atual é resolvido pelo backend e usado para preencher a fórmula no momento do cálculo.

### 7. Testes adicionados ou reforçados

- Foram adicionados testes para garantir que fórmula superior sem período explícito é válida.
- Foram adicionados testes para garantir que fórmula superior com período explícito é rejeitada.
- Foram adicionados testes para garantir que fórmulas de `fundamental` e `medio` exigem período explícito.
- A suíte completa foi executada com sucesso após os ajustes.

## Mudado

### 1. Nome público da regra

- Antes: o contrato de regra usava `tipo_ensino`.
- Agora: o contrato de regra usa `nivel`.
- A API de regras rejeita `tipo_ensino` em criação e edição.

### 2. Escopo de `anos_academicos`

- Antes: o escopo podia ser tratado de forma genérica para diferentes níveis.
- Agora: `anos_academicos` pertence apenas a regras fundamentais.
- Médio e superior resolvem escopo pelo estudante, curso e período/ano aplicável, não por `anos_academicos` no payload público da regra.

### 3. Fórmula superior

- Antes: fórmulas podiam ser validadas como se sempre tivessem período explícito.
- Agora: `superior` não deve declarar período na fórmula; o período é inferido da matéria/escopo avaliado.

### 4. Edição de regras

- Antes: a edição validava principalmente nome, nota mínima e fórmula.
- Agora: a edição também rejeita campos legados ou ambíguos (`tipo_ensino`, `nivel`, `anos_academicos`) e valida a fórmula conforme o nível persistido da regra.

### 5. Documentação

- A documentação da API e a documentação funcional foram atualizadas para descrever:
  - `nivel` no lugar de `tipo_ensino`;
  - regras por nível;
  - fórmula superior com período inferido;
  - campos `materias_chave`, `materias_aplicaveis` e `limite_materias_pendentes`;
  - pendências de matérias;
  - bloqueio por `pendencia_nivel_conclusao`;
  - regularização posterior de pendências.

## Removido ou bloqueado

### 1. Uso público de `tipo_ensino` nas regras de avaliação final

- `tipo_ensino` não é aceito no payload de criação de regra.
- `tipo_ensino` não é aceito no payload de edição de regra.
- Não foi mantido alias, fallback ou tradução automática para `tipo_ensino` no contrato de regras.

### 2. `anos_academicos` fora do fundamental

- `anos_academicos` é rejeitado para regras de `medio` e `superior`.
- A edição também bloqueia alteração de `anos_academicos` para evitar mudanças silenciosas de escopo.

### 3. Período explícito em fórmula superior

- Fórmulas superiores no formato `[categoria,periodo]` são rejeitadas.
- O formato aceito para superior é `[categoria]`, com preenchimento automático do período no cálculo.

### 4. Pendências no fundamental

- Matérias fundamentais não podem usar `pendencia_permitida`.
- Matérias fundamentais não podem usar `pendencia_nivel_conclusao`.
- Regras fundamentais não aceitam `limite_materias_pendentes`.

## Pontos de atenção técnica

- A tabela `projection_avaliacao_final` ainda mantém colunas históricas relacionadas a `tipo_ensino` em algumas projeções e fluxos antigos de avaliação final do estudante. A mudança de nomenclatura exigida pela tarefa foi aplicada ao contrato de regras de avaliação final, enquanto avaliações já existentes continuam usando o vocabulário interno/histórico onde o modelo agregado ainda depende dele.
- A estrutura persistente para resultados por matéria e matérias pendentes foi criada, mas qualquer evolução futura deve garantir que todos os fluxos de progressão/conclusão consultem pendências abertas do curso atual antes de avançar ou finalizar ciclo.
- As validações de subconjunto de `materias_aplicaveis` em regras descendentes e de `materias_chave` por curso/ano devem continuar sendo mantidas próximas das consultas de matéria/curso para evitar configuração fora do escopo.

## Comandos de verificação executados

```bash
go test ./internal/domain/aggregates -count=1
```

```bash
go test ./internal/handlers -run 'TestFormula' -count=1
```

```bash
timeout 120s go test ./...
```
