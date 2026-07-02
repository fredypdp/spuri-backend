---
modificado: 2026-07-02 00:00
criado: 2026-07-02 00:00
---
# Atualizar regra de avaliação final com escopo por curso/ano e matérias aplicáveis por ano

## Objetivo

Implementar uma atualização profunda no modelo de **regra de avaliação final** para que a regra deixe de trabalhar com `anos_academicos` e `materias_aplicaveis` como listas simples globais e passe a declarar o escopo de aplicação por **curso**, **ano académico** e, quando aplicável, **matérias específicas daquele ano**.

A mudança deve atravessar todo o backend: contratos públicos, DTOs, validações, schemas, agregados, eventos, projeções, handlers, migrações, índices de unicidade, consultas, execução da avaliação final, testes e documentação.

A implementação precisa preservar o modelo atual de avaliação final automática por matéria, regras descendentes, pendências, auditoria e uso de `nivel`, mas deve substituir o modo de representar o escopo da regra e o filtro de matérias aplicáveis.

## Arquivos e áreas de referência obrigatória

Leia e atualize, no mínimo:

- `internal/handlers/avaliacao_final_regras.go`;
- `internal/handlers/avaliacao_final_handler.go`;
- `internal/handlers/avaliacao_final_regras_test.go`;
- `internal/handlers/avaliacao_final_formula_test.go`;
- `internal/projections/avaliacao_final_projection.go`;
- `internal/projections/avaliacao_final_projection_test.go`;
- `internal/domain/models.go`;
- `internal/domain/aggregates/estudante_avaliacao.go`;
- `internal/domain/aggregates/estudante_avaliacao_test.go`;
- migrações relacionadas a regras/avaliações finais, especialmente as migrações de unicidade e escopo;
- `docs/Spuri - Documentação.md`, seção **5.6 Avaliação Final de Ano Académico**;
- `docs/Spuri - API.md`, seção **15. Avaliações Finais**;
- tarefas anteriores sobre avaliação final, pendências, matérias aplicáveis e remoção de `materias_chave` da regra.

Não trate esta lista como exaustiva. Faça busca ampla por `anos_academicos`, `materias_aplicaveis`, `avaliacao_final_regras`, `RegraAvaliacaoFinal`, `tipo_ensino`, `nivel`, `curso_id`, `ano_academico`, `materia_id` e nomes equivalentes em camelCase.

## Novo contrato de `anos_academicos`

### Estrutura desejada

O campo `anos_academicos` da regra passa a ser uma lista de itens por curso:

```json
[
  {
    "curso_id": "id do curso desses anos acadêmicos",
    "anos_academicos": ["array de strings dos anos desse curso que serão submetidos a essa regra"]
  }
]
```

### Regras obrigatórias

- Cada item precisa ter `curso_id` válido, ativo e pertencente à academia autenticada.
- `anos_academicos` de cada item precisa ser array não vazio de strings válidas para o curso informado.
- Não pode haver dois itens com o mesmo `curso_id`.
- Não pode haver ano duplicado dentro de `anos_academicos` de um mesmo item.
- Cada ano enviado precisa pertencer ao curso correspondente.
- Para cursos médios, os anos devem existir em `projection_cursos.anos_academicos`.
- Para cursos superiores, os anos acadêmicos devem corresponder aos anos calculados a partir de `periodos`/semestres do curso.
- Para regra fundamental, decidir e documentar explicitamente se `curso_id` é obrigatório por compatibilidade com o novo modelo ou se haverá um identificador/curso sintético. A decisão precisa ser refletida em schemas, validações, documentação e testes. Não deixe comportamento implícito.
- A normalização deve ordenar/deduplicar apenas quando isso não esconder erro do cliente. Preferencialmente, duplicidade no payload deve ser erro de validação claro.

## Novo contrato de `materias_aplicaveis`

`materias_aplicaveis` deixa de ser lista simples de IDs e passa a ser lista de escopos por ano académico, com regras diferentes por nível.

### Fundamental

Estrutura:

```json
[
  {
    "ano_academico": "1_ano_fundamental",
    "materias": ["id_materia_1", "id_materia_2"]
  }
]
```

Regras:

- A unicidade do item depende somente de `ano_academico`.
- Não pode haver dois itens com o mesmo `ano_academico`.
- `ano_academico` precisa estar coberto pelo escopo `anos_academicos` da regra.
- Cada matéria precisa existir, estar ativa, pertencer à academia, ser do nível fundamental e conter o `ano_academico` informado em seu próprio escopo.
- Não pode haver matéria duplicada dentro do mesmo item.
- Matérias fora do ano informado devem ser rejeitadas.

### Médio

Estrutura:

```json
[
  {
    "curso_id": "id do curso",
    "ano_academico": "1_ano_medio",
    "materias": ["id_materia_1", "id_materia_2"]
  }
]
```

Regras:

- A unicidade do item depende do par `curso_id` + `ano_academico`.
- Não pode haver dois itens com o mesmo par `curso_id` + `ano_academico`.
- `curso_id` precisa existir, estar ativo, pertencer à academia e ser curso médio.
- `ano_academico` precisa pertencer ao curso médio informado.
- O par `curso_id` + `ano_academico` precisa estar coberto por `anos_academicos` da regra.
- Cada matéria precisa existir, estar ativa, pertencer à mesma academia, ser do nível médio, pertencer ao mesmo `curso_id` e ao mesmo `ano_academico`.
- Não pode haver matéria duplicada dentro do mesmo item.

### Superior

Estrutura:

```json
[
  {
    "curso_id": "id do curso",
    "ano_academico": "1_ano_superior",
    "materias": ["id_materia_1", "id_materia_2"]
  }
]
```

Regras:

- A unicidade do item depende do par `curso_id` + `ano_academico`.
- Não pode haver dois itens com o mesmo par `curso_id` + `ano_academico`.
- `curso_id` precisa existir, estar ativo, pertencer à academia e ser curso superior.
- `ano_academico` precisa ser um ano superior válido para o curso, derivado dos períodos/semestres do curso.
- O par `curso_id` + `ano_academico` precisa estar coberto por `anos_academicos` da regra.
- Cada matéria precisa existir, estar ativa, pertencer à mesma academia, ser do nível superior, pertencer ao mesmo `curso_id` e estar em um período/semestre que corresponda ao par `curso_id` + `ano_academico`.
- Não pode haver matéria duplicada dentro do mesmo item.
- A validação deve mapear corretamente ano superior para períodos. Exemplo: `1_ano_superior` corresponde aos períodos/semestres iniciais daquele curso conforme o modelo existente.

## Novo modelo de garantia de unicidade da regra

A unicidade de regras ativas precisa ser expandida. Não basta mais comparar `codigo_academia`, `nivel`, `type` e uma lista simples de anos.

### Regra geral

Não pode existir mais de uma regra ativa, para a mesma academia, `nivel` e `type`, cujo escopo se sobreponha em qualquer item de `anos_academicos`.

A comparação de sobreposição deve considerar cada par:

- `curso_id`;
- cada `ano_academico` dentro do item daquele curso.

Em outras palavras, duas regras ativas conflitantes existem quando compartilham pelo menos um mesmo `curso_id` + `ano_academico` para o mesmo `codigo_academia`, `nivel` e `type`.

### Regras descendentes

- Regras descendentes precisam preservar compatibilidade de escopo com a regra raiz/ascendente.
- Defina se a descendente deve ter exatamente o mesmo escopo da ascendente ou se pode ter subconjunto. A decisão precisa ser explícita, validada e documentada.
- Se permitir subconjunto, a descendente não pode declarar nenhum `curso_id` + `ano_academico` fora do escopo da regra ascendente.
- `materias_aplicaveis` de uma descendente precisa ser subconjunto coerente do escopo da própria descendente e, por consequência, do escopo da cadeia.
- A detecção de ciclos, regra órfã, nível incompatível e dependência inativa deve continuar funcionando.

### Persistência e índices

Avalie e implemente a estratégia mais segura para unicidade:

- coluna JSONB normalizada com validação transacional;
- tabela/projeção auxiliar de escopos de regra (`regra_id`, `curso_id`, `ano_academico`, `nivel`, `type`, status);
- índice único parcial para regras ativas;
- ou outra abordagem robusta e auditável.

A solução deve ser segura contra corrida concorrente. Não confie apenas em validação em memória no handler.

## Atualização profunda de schemas, handlers, aggregates e projeções

A mudança deve ser aplicada de ponta a ponta:

### Contratos e schemas

- Atualizar structs/DTOs de request e response.
- Atualizar validações JSON e mensagens de erro.
- Rejeitar formatos antigos de `anos_academicos` e `materias_aplicaveis`, salvo se houver estratégia explícita de migração/compatibilidade documentada.
- Garantir que exemplos e respostas exponham o novo formato.
- Atualizar snapshots de eventos, se existirem.

### Handlers

- Atualizar criação, listagem, detalhe, edição, ativação/inativação e execução de regra.
- Validar `curso_id`, `ano_academico`, matéria e nível no momento da criação da regra.
- Manter rejeição de campos legados incompatíveis.
- Garantir erros determinísticos, com `field`, `code` e mensagem clara.

### Aggregates e domínio

- Atualizar modelos de domínio da regra.
- Atualizar invariantes de criação e transição de status.
- Garantir que o cálculo da avaliação final use o novo escopo por curso/ano.
- Garantir que `materias_aplicaveis` filtre corretamente por ano e curso no momento da execução.
- Garantir que pendências e progressão continuem usando o escopo correto.

### Projeções e migrações

- Atualizar projeções de regras e avaliações finais para armazenar/expor os novos campos.
- Migrar dados existentes de forma segura ou documentar caminho de migração caso não seja possível inferir `curso_id` para dados antigos.
- Criar índices/constraints necessários para unicidade por `curso_id` + `ano_academico`.
- Garantir replay de eventos antigos ou criar migração compatível para snapshots antigos.

### Execução da avaliação final

- Fundamental deve selecionar matérias aplicáveis pelo `ano_academico` do estudante e pelo filtro `materias_aplicaveis` do item correspondente.
- Médio deve selecionar matérias pelo `curso_id` do estudante, `ano_academico` atual e filtro do par correspondente.
- Superior deve selecionar matérias pelo `curso_id`, semestre/período atual, ano académico derivado e filtro do par correspondente.
- Se `materias_aplicaveis` estiver ausente/vazio, a regra deve avaliar todas as matérias aplicáveis ao escopo.
- Se houver filtro, somente as matérias listadas no item do escopo correspondente devem ser recalculadas.

## Documentação obrigatória

Atualize profundamente:

1. `docs/Spuri - Documentação.md`, seção **5.6 Avaliação Final de Ano Académico**;
2. `docs/Spuri - API.md`, seção **15. Avaliações Finais**.

A documentação precisa explicar:

- o novo formato de `anos_academicos`;
- o novo formato de `materias_aplicaveis` por nível;
- a nova regra de unicidade baseada em `curso_id` + `ano_academico`;
- exemplos de criação de regra fundamental, média e superior;
- exemplos de regra descendente com filtro de matérias por ano;
- erros esperados para duplicidade de itens, matéria fora do curso/ano/período, curso inexistente/inativo, ano não coberto pela regra e conflito de regra ativa;
- impacto na execução da avaliação final por matéria;
- como o frontend deve montar payloads por nível.

Remova ou reescreva trechos que ainda afirmem que:

- `anos_academicos` é apenas array simples de strings;
- médio/superior nunca usam `anos_academicos` na regra;
- `materias_aplicaveis` é lista simples global de IDs;
- a unicidade da regra ignora curso/ano por item.

## Testes obrigatórios

Inclua testes unitários, de integração/handler e, quando possível, de projeção/migração para:

- criação de regra com novo `anos_academicos` válido;
- rejeição de `anos_academicos` no formato antigo;
- rejeição de item duplicado por `curso_id`;
- rejeição de ano duplicado dentro do mesmo item;
- rejeição de ano que não pertence ao curso;
- rejeição de curso inexistente, inativo, deletado ou de outra academia;
- conflito de unicidade por mesmo `curso_id` + `ano_academico`;
- ausência de conflito quando regras ativas têm cursos/anos disjuntos;
- `materias_aplicaveis` fundamental com duplicidade de `ano_academico`;
- `materias_aplicaveis` médio/superior com duplicidade de `curso_id` + `ano_academico`;
- rejeição de matéria fora do ano fundamental;
- rejeição de matéria média fora do curso/ano;
- rejeição de matéria superior fora do curso/período correspondente ao ano;
- execução sem `materias_aplicaveis`, avaliando todas as matérias do escopo;
- execução com `materias_aplicaveis`, avaliando apenas matérias listadas para o escopo correto;
- regra descendente com escopo incompatível;
- replay/projeção/migração de regras existentes, conforme estratégia adotada.

## Critérios de aceite

A tarefa só deve ser considerada concluída quando:

- o contrato público novo estiver implementado e documentado;
- formatos antigos forem rejeitados ou migrados por estratégia explícita e testada;
- a unicidade da regra ativa considerar `curso_id` + cada `ano_academico` de cada item;
- `materias_aplicaveis` validar unicidade e pertinência por nível;
- a execução da avaliação final respeitar os novos filtros;
- handlers, schemas, aggregates, projeções, eventos, migrações, índices e testes estiverem coerentes;
- as seções **5.6 Avaliação Final de Ano Académico** e **15. Avaliações Finais** estiverem atualizadas;
- não houver documentação conflitante com o novo modelo;
- a suíte de testes relevante estiver passando.
