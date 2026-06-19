---
modificado: 2026-06-20 0:12
criado: 2026-06-20 0:12
---
# Tarefa: substituir a DSL JSON de avaliação final por uma fórmula textual segura

## Contexto

O modelo atual de regras de avaliação final usa uma DSL em árvore JSON no campo `formula`, com operações como `sum_periods`, `category_total`, `add` e `div`. Embora seja seguro por não executar código arbitrário, ele é verboso e difícil de elaborar por academias ou por interfaces administrativas simples.

A proposta é trocar esse modelo por uma fórmula textual controlada, mais parecida com uma expressão matemática, em que cada operando referencia explicitamente uma categoria de nota e um período. Exemplo conceitual:

```text
[nota_escola,1_trimestre]+[nota_professor,1_trimestre]+[nota_exame_final,3_trimestre]/3
```

A sintaxe final deve ser definida de forma mais rígida do que o exemplo acima para evitar ambiguidades e preservar segurança.

## Objetivo

Criar um novo modelo de elaboração de fórmulas para avaliação final que seja mais simples de entender, digitar, revisar e explicar na documentação, mantendo validação forte no backend e sem permitir execução de código arbitrário.

## Decisão técnica proposta

Implementar a fórmula como uma string declarativa, validada e interpretada por parser próprio do backend, sem `eval`, sem execução dinâmica e sem bibliotecas que executem expressões arbitrárias.

A string deve aceitar apenas:

- Referências de nota no formato definido pelo backend, por exemplo `categoria@periodo` ou `[categoria,periodo]`.
- Operadores matemáticos permitidos: `+`, `-`, `*`, `/`.
- Parênteses para agrupamento, se forem necessários para clareza.
- Números constantes positivos quando forem úteis para médias, pesos ou divisores.
- Espaços opcionais, ignorados pelo parser.

A gramática deve rejeitar qualquer caractere ou estrutura fora da lista permitida.

## Requisitos de segurança

- Não usar `eval`, JavaScript, SQL dinâmico, templates executáveis ou qualquer forma de execução de código fornecido pelo usuário.
- Criar lexer/parser próprio ou usar biblioteca de parsing que apenas gere AST, sem avaliar código externo.
- Converter a fórmula em AST interna tipada antes de salvar ou antes de executar.
- Validar todas as categorias referenciadas contra `categorias_envolvidas`.
- Validar todos os períodos referenciados com as regras já existentes de período/ano/semestre.
- Impedir divisão por zero em constantes e durante execução.
- Definir precisão/normalização dos cálculos para manter comportamento determinístico.
- Garantir que a avaliação continue usando apenas notas do estudante, academia e ano letivo corretos, não deletadas.
- Manter snapshot auditável da fórmula textual usada no momento da avaliação final.
- Impedir duplicatas
- Garantir o uso de categorias que a acadmeia possui

## Comportamento esperado

1. A academia cadastra uma regra de avaliação final enviando `formula` como string.
2. O backend valida sintaxe, operadores, categorias, períodos, constantes e limites.
3. O backend persiste a fórmula normalizada e/ou a AST serializada de forma auditável.
4. Ao registrar notas, o backend interpreta a fórmula validada contra as notas disponíveis.
5. Se faltar nota para qualquer referência `categoria/período`, a avaliação aguarda novo lançamento, como já acontece hoje.
6. O resultado calculado continua sendo comparado com `nota_minima_aprovacao`.
7. A cadeia `normal`, `recurso`, `especial` etc. continua funcionando pelo campo `aplica_se_reprovado_em_type`.

## Remoção obrigatória do modelo antigo

O modelo antigo de fórmula em árvore JSON deve ser totalmente apagado do código, sem deixar resquícios funcionais ou documentação antiga como alternativa.

Remover coisas como completamente:

- Structs e funções específicas da DSL JSON atual.
- Validações das operações `sum_periods`, `category_total`, `add` e `div`.
- Exemplos da DSL JSON nas documentações.
- Textos que indiquem suporte ao modelo antigo.
- Testes antigos que validem a DSL JSON, substituindo-os por testes da fórmula textual.

Não manter compatibilidade temporária de dados legado pois não existem.

## Atualização de banco e versionamento

- Avaliar se o campo `formula JSONB` deve virar `formula TEXT` ou se será criado um novo campo com migração controlada.
- Se houver alteração de tipo/estrutura, criar migração nova e segura.
- Atualizar snapshots de avaliação final para armazenarem a fórmula textual usada.
- Incrementar a versão das regras novas conforme o padrão do projeto.
- Documentar claramente o impacto de versão do modelo da fórmula.

## Atualização obrigatória das documentações

Depois de atualizar completamente o código, atualizar as duas documentações do projeto:

1. `docs/Spuri - API.md`
2. `docs/Spuri - Documentação.md`

As duas documentações devem ser bem didáticas e conter:

- Sintaxe oficial da fórmula textual.
- Lista de operadores permitidos.
- Regras de precedência e uso de parênteses.
- Como referenciar categoria e período.
- Como calcular médias simples.
- Como calcular fórmulas com pesos.
- Como o backend trata notas ausentes.
- Exemplos válidos e inválidos.
- Explicação de segurança: a fórmula é interpretada por parser controlado, não executada como código.
- Campo de versão/documentação do novo modelo.
- Remoção explícita da documentação do modelo antigo em árvore JSON.

## Exemplos didáticos sugeridos

### Média de três trimestres

```text
([nota_escola,1_trimestre]+[nota_escola,2_trimestre]+[nota_escola,3_trimestre])/3
```

### Média ponderada

```text
([nota_escola,1_trimestre]*0.3)+([nota_escola,2_trimestre]*0.3)+([nota_exame_final,3_trimestre]*0.4)
```

### Soma de componentes do mesmo período

```text
[nota_escola,1_trimestre]+[nota_professor,1_trimestre]
```

## Critérios de aceite

- O endpoint de criação de regra aceita somente o novo formato textual.
- O formato JSON antigo deixa de ser aceito.
- O cálculo automático de avaliação final usa apenas o novo parser.
- As validações impedem caracteres, tokens e categorias/períodos inválidos.
- Divisão por zero é bloqueada.
- Fórmulas grandes demais são rejeitadas.
- Testes cobrem fórmulas válidas, inválidas, notas ausentes, precedência, parênteses, pesos e divisão por zero.
- As duas documentações são atualizadas com linguagem didática e exemplos completos.
- Não ficam referências ao modelo antigo como opção suportada.
