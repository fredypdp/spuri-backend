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

O campo `anos_academicos` passa a suportar **dois formatos públicos**, de acordo com o `nivel` da regra:

1. para `fundamental`, mantém a estrutura anterior, como array simples de strings;
2. para `medio`, usa a nova estrutura por curso;
3. para `superior`, continua não devendo ser enviado, salvo se outra mudança explícita for aprovada em tarefa separada.

### Fundamental

Para regra `nivel="fundamental"`, `anos_academicos` deve continuar sendo array de strings com os anos fundamentais da academia que serão submetidos à regra:

```json
["1_ano_fundamental", "2_ano_fundamental"]
```

Regras:

- O array é obrigatório e não pode ser vazio para regras fundamentais.
- Cada item precisa ser um ano fundamental válido.
- Cada ano enviado precisa pertencer aos anos fundamentais ofertados pela academia autenticada.
- Não pode haver ano duplicado.
- Não deve existir `curso_id` no escopo fundamental.
- O backend deve rejeitar a nova estrutura por curso quando `nivel="fundamental"`, porque fundamental preserva o contrato antigo.

### Médio

Para regra `nivel="medio"`, `anos_academicos` deve usar a nova lista de itens por curso:

```json
[
  {
    "curso_id": "id do curso médio desses anos acadêmicos",
    "anos_academicos": ["1_ano_medio", "2_ano_medio"]
  }
]
```

Regras:

- Cada item precisa ter `curso_id` válido, ativo, não deletado, pertencente à academia autenticada e de curso médio.
- `anos_academicos` de cada item precisa ser array não vazio de strings válidas para o curso médio informado.
- Não pode haver dois itens com o mesmo `curso_id`.
- Não pode haver ano duplicado dentro de `anos_academicos` de um mesmo item.
- Cada ano enviado precisa pertencer a `projection_cursos.anos_academicos` do curso médio correspondente.
- O backend deve rejeitar o formato antigo de array simples quando `nivel="medio"`.

### Superior

- Regras `nivel="superior"` não devem aceitar `anos_academicos` no payload público nesta mudança.
- O escopo superior continua sendo resolvido por curso/período/semestre do estudante e das matérias durante a execução.
- Se no futuro o superior também passar a declarar `anos_academicos` na regra, isso deve ser especificado em tarefa própria para evitar ambiguidade com o modelo semestral existente.

### Normalização

A normalização deve ordenar/deduplicar apenas quando isso não esconder erro do cliente. Preferencialmente, duplicidade no payload deve ser erro de validação claro.

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
- Como `anos_academicos` não é enviado em regra superior nesta mudança, o par `curso_id` + `ano_academico` precisa ser validado contra o curso superior e contra o período/semestre das matérias, não contra o campo `anos_academicos` da regra.
- Cada matéria precisa existir, estar ativa, pertencer à mesma academia, ser do nível superior, pertencer ao mesmo `curso_id` e estar em um período/semestre que corresponda ao par `curso_id` + `ano_academico`.
- Não pode haver matéria duplicada dentro do mesmo item.
- A validação deve mapear corretamente ano superior para períodos. Exemplo: `1_ano_superior` corresponde aos períodos/semestres iniciais daquele curso conforme o modelo existente.

## Novo modelo de garantia de unicidade da regra

A unicidade de regras ativas precisa ser expandida. Não basta mais comparar `codigo_academia`, `nivel`, `type` e uma lista simples de anos.

### Regra geral

Não pode existir mais de uma regra ativa, para a mesma academia, `nivel` e `type`, cujo escopo se sobreponha em `anos_academicos`, respeitando o formato específico de cada nível.

A comparação de sobreposição deve considerar:

- para `fundamental`: cada `ano_academico` string do array simples;
- para `medio`: cada par `curso_id` + `ano_academico` dentro dos itens por curso;
- para `superior`: o modelo atual sem `anos_academicos` deve permanecer coerente com a unicidade já existente ou ser explicitamente revisado se houver alteração futura.

Em outras palavras, duas regras fundamentais ativas conflitantes existem quando compartilham pelo menos um mesmo ano fundamental para o mesmo `codigo_academia`, `nivel` e `type`. Duas regras médias ativas conflitantes existem quando compartilham pelo menos um mesmo `curso_id` + `ano_academico` para o mesmo `codigo_academia`, `nivel` e `type`.

### Regras descendentes

- Regras descendentes precisam preservar compatibilidade de escopo com a regra raiz/ascendente.
- Defina se a descendente deve ter exatamente o mesmo escopo da ascendente ou se pode ter subconjunto. A decisão precisa ser explícita, validada e documentada.
- Se permitir subconjunto, a descendente não pode declarar nenhum ano fundamental fora do array simples da regra ascendente nem nenhum `curso_id` + `ano_academico` médio fora do escopo da regra ascendente.
- `materias_aplicaveis` de uma descendente precisa ser subconjunto coerente do escopo da própria descendente e, por consequência, do escopo da cadeia.
- A detecção de ciclos, regra órfã, nível incompatível e dependência inativa deve continuar funcionando.

### Persistência e índices

Avalie e implemente a estratégia mais segura para unicidade:

- coluna JSONB normalizada com validação transacional;
- tabela/projeção auxiliar de escopos de regra (`regra_id`, `curso_id` nullable para fundamental, `ano_academico`, `nivel`, `type`, status);
- índice único parcial para regras ativas;
- ou outra abordagem robusta e auditável.

A solução deve ser segura contra corrida concorrente. Não confie apenas em validação em memória no handler.

## Atualização profunda de schemas, handlers, aggregates e projeções

A mudança deve ser aplicada de ponta a ponta:

### Contratos e schemas

- Atualizar structs/DTOs de request e response.
- Atualizar validações JSON e mensagens de erro.
- Rejeitar formatos incompatíveis por nível: fundamental deve aceitar apenas o array simples preservado, médio deve aceitar apenas a nova estrutura por curso, superior deve rejeitar `anos_academicos`, e `materias_aplicaveis` deve seguir o novo formato por nível.
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
- Garantir que o cálculo da avaliação final use o escopo correto: ano simples no fundamental, curso/ano no médio e curso/período derivado no superior.
- Garantir que `materias_aplicaveis` filtre corretamente por ano e curso no momento da execução.
- Garantir que pendências e progressão continuem usando o escopo correto.

### Projeções e migrações

- Atualizar projeções de regras e avaliações finais para armazenar/expor os novos campos.
- Migrar dados existentes de forma segura: regras fundamentais antigas já devem permanecer como array simples; para regras médias antigas, documentar caminho de migração caso não seja possível inferir `curso_id`.
- Criar índices/constraints necessários para unicidade por ano fundamental e por `curso_id` + `ano_academico` no médio.
- Garantir replay de eventos antigos ou criar migração compatível para snapshots antigos.

### Execução da avaliação final

- Fundamental deve selecionar matérias aplicáveis pelo `ano_academico` do estudante, validando o ano contra o array simples `anos_academicos`, e pelo filtro `materias_aplicaveis` do item correspondente.
- Médio deve selecionar matérias pelo `curso_id` do estudante, `ano_academico` atual e filtro do par correspondente.
- Superior deve selecionar matérias pelo `curso_id`, semestre/período atual, ano académico derivado e filtro do par correspondente.
- Se `materias_aplicaveis` estiver ausente/vazio, a regra deve avaliar todas as matérias aplicáveis ao escopo.
- Se houver filtro, somente as matérias listadas no item do escopo correspondente devem ser recalculadas.

## Documentação obrigatória

Atualize profundamente:

1. `docs/Spuri - Documentação.md`, seção **5.6 Avaliação Final de Ano Académico**;
2. `docs/Spuri - API.md`, seção **15. Avaliações Finais**.

A documentação precisa explicar:

- o formato de `anos_academicos` por nível: array simples preservado no fundamental, nova lista por curso no médio e rejeição no superior;
- o novo formato de `materias_aplicaveis` por nível;
- a nova regra de unicidade: ano simples no fundamental e `curso_id` + `ano_academico` no médio;
- exemplos de criação de regra fundamental com array simples, regra média com escopo por curso e regra superior sem `anos_academicos`;
- exemplos de regra descendente com filtro de matérias por ano;
- erros esperados para duplicidade de itens, matéria fora do curso/ano/período, curso inexistente/inativo, ano não coberto pela regra e conflito de regra ativa;
- impacto na execução da avaliação final por matéria;
- como o frontend deve montar payloads por nível.

Remova ou reescreva trechos que ainda afirmem que:

- `anos_academicos` é sempre array simples de strings para todos os níveis;
- médio nunca usa `anos_academicos` na regra;
- superior aceita `anos_academicos` nesta mudança;
- `materias_aplicaveis` é lista simples global de IDs;
- a unicidade da regra ignora curso/ano por item.

## Testes obrigatórios

Inclua testes unitários, de integração/handler e, quando possível, de projeção/migração para:

- criação de regra fundamental com `anos_academicos` no formato antigo preservado;
- criação de regra média com novo `anos_academicos` válido por curso;
- rejeição de `anos_academicos` fundamental no formato por curso;
- rejeição de `anos_academicos` médio no formato antigo de array simples;
- rejeição de `anos_academicos` em regra superior;
- rejeição de item médio duplicado por `curso_id`;
- rejeição de ano duplicado dentro do mesmo item;
- rejeição de ano que não pertence ao curso;
- rejeição de curso inexistente, inativo, deletado ou de outra academia;
- conflito de unicidade fundamental por mesmo ano acadêmico;
- conflito de unicidade média por mesmo `curso_id` + `ano_academico`;
- ausência de conflito quando regras ativas fundamentais têm anos disjuntos;
- ausência de conflito quando regras ativas médias têm cursos/anos disjuntos;
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
- formatos incompatíveis por nível forem rejeitados ou migrados por estratégia explícita e testada, preservando o array simples do fundamental;
- a unicidade da regra ativa considerar cada ano fundamental no formato simples e cada `curso_id` + `ano_academico` no médio;
- `materias_aplicaveis` validar unicidade e pertinência por nível;
- a execução da avaliação final respeitar os novos filtros;
- handlers, schemas, aggregates, projeções, eventos, migrações, índices e testes estiverem coerentes;
- as seções **5.6 Avaliação Final de Ano Académico** e **15. Avaliações Finais** estiverem atualizadas;
- não houver documentação conflitante com o novo modelo;
- a suíte de testes relevante estiver passando.
