---
modificado: 2026-07-07 00:00
criado: 2026-07-07 00:00
---
# Depurar implementação de padrão avaliativo fixo para escolas

Tarefa: [[Implementar padrao avaliativo fixo para escolas]]

## Objetivo da auditoria

Fazer uma auditoria crítica, completa e arquivo por arquivo da implementação da tarefa:

`docs/Tarefas feitas/Implementar padrao avaliativo fixo para escolas.md`

A auditoria deve confirmar se a implementação foi feita corretamente, completamente e **à risca**. Caso qualquer parte esteja incompleta, inconsistente, parcialmente implementada, sem validação, sem teste, sem documentação, com comportamento silencioso incorreto ou divergente do contrato esperado, esta tarefa exige **ajustar e terminar a implementação**.

Esta funcionalidade é crítica porque separa definitivamente o padrão escolar fixo do modelo superior configurável. O backend não pode permitir que escolas, academias mistas ou administradores criem, editem, removam ou sobrescrevam categorias e regras escolares oficiais.

## Resultado da depuração

A depuração confirmou que a implementação principal existe e cobre:

- catálogo escolar fixo por ano acadêmico;
- regras escolares fixas de avaliação final regular, exame de recurso e PAP;
- bloqueio de criação/remoção de categorias escolares por endpoint configurável;
- restrição das regras configuráveis de avaliação final ao ensino superior;
- validação de escala de nota por ano acadêmico;
- documentação funcional, documentação de API e manual de configuração inicial refletindo o novo padrão.

Durante a auditoria foi encontrado e corrigido um problema na filtragem das regras escolares fixas por categoria despertadora: quando o filtro era `exame_recurso`, a função comparava apenas a regra raiz e retornava vazio, ocultando a regra descendente fixa. A correção agora filtra todas as regras fixas e retorna a regra cuja `nota_despertadora` corresponde à categoria informada. Mas ainda garanta que isso realmente foi corrigido.

Também foi reforçada a geração determinística do ID das regras escolares médias para incluir o escopo completo (`curso_id|ano_academico`) quando houver curso, evitando colisão entre regras fixas do mesmo ano médio em cursos distintos.

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. modelo fixo de categorias escolares por ano acadêmico;
2. regras fixas escolares geradas pelo sistema;
3. validação de escala de nota por ano acadêmico;
4. bloqueios de criação, edição e remoção de categorias escolares;
5. bloqueios de criação, edição e remoção de regras escolares configuráveis;
6. cálculo automático de avaliação final escolar por gatilho;
7. regra descendente fixa `exame_recurso`;
8. regra especial `nota_pap` para `4_ano_medio` técnico;
9. persistência, snapshots, projeções e idempotência da avaliação final;
10. compatibilidade com escolas mistas;
11. exclusividade de matérias dependentes/pendências para Superior;
12. documentação em `docs/Spuri - Documentação.md`;
13. documentação em `docs/Spuri - API.md`;
14. manual `docs/Manual de Configuração Inicial da Academia.md`;
15. testes unitários e regressivos do padrão fixo.

## Checklist obrigatório de validação

### 1. Categorias escolares fixas

Confirmar que o backend fornece automaticamente, como `source="system"`, `fixed=true` e `readonly=true`, as categorias abaixo:

| Anos acadêmicos | Categorias obrigatórias |
| --- | --- |
| `1_ano_fundamental` a `5_ano_fundamental`, `7_ano_fundamental`, `8_ano_fundamental`, `1_ano_medio`, `2_ano_medio` | `nota_professor`, `prova_trimestral` |
| `6_ano_fundamental`, `9_ano_fundamental`, `3_ano_medio` | `nota_professor`, `prova_trimestral`, `exame_final`, `exame_recurso` |
| `4_ano_medio` técnico | `nota_pap` |

Validar que:

- escolas não conseguem criar categorias via `POST /academia/categorias-nota`;
- escolas não conseguem remover categorias via `DELETE /academia/categorias-nota/:codigo`;
- o Superior continua podendo configurar categorias próprias;
- o lançamento de notas escolares usa somente o catálogo fixo aplicável ao ano e modelo do curso;
- `4_ano_medio` técnico rejeita categorias trimestrais convencionais.

### 2. Regras escolares fixas

Confirmar que as regras escolares são geradas pelo sistema e não dependem de registros administráveis pela academia.

Validar especificamente:

- `1_ano_fundamental` a `5_ano_fundamental`: média trimestral com aprovação por `nota_final >= 5`;
- `7_ano_fundamental`, `8_ano_fundamental`, `1_ano_medio`, `2_ano_medio`: média trimestral com aprovação por `nota_final >= 10`;
- `6_ano_fundamental`: avaliação final com `exame_final` e aprovação por `nota_final >= 5`;
- `9_ano_fundamental` e `3_ano_medio`: avaliação final com `exame_final` e aprovação por `nota_final >= 10`;
- `exame_recurso`: disponível somente após reprovação anterior na matéria;
- `4_ano_medio` técnico: aprovação por `nota_pap >= 10`.

### 3. Gatilhos automáticos

Confirmar que a avaliação final escolar é tentada automaticamente após lançamento/atualização de nota e que dados incompletos deixam a avaliação pendente, sem aprovação/reprovação definitiva indevida.

Gatilhos esperados:

- `prova_trimestral` no fluxo regular;
- `exame_final` nos anos com exame;
- `exame_recurso` somente para matérias reprovadas na avaliação final anterior;
- `nota_pap` no `4_ano_medio` técnico.

### 4. Escala de notas

Confirmar que o backend rejeita notas fora das escalas oficiais:

| Anos acadêmicos | Escala |
| --- | --- |
| `1_ano_fundamental` a `6_ano_fundamental` | `0` a `10` |
| `7_ano_fundamental` a `9_ano_fundamental` | `0` a `20` |
| `1_ano_medio` a `4_ano_medio` | `0` a `20` |
| Superior | `0` a `20` |

A validação deve aceitar valores decimais dentro da escala e rejeitar negativos ou valores acima do máximo.

### 5. Documentação e manual

Confirmar que as seguintes documentações explicam profundamente a atualização:

- `docs/Spuri - Documentação.md` deve descrever a separação entre escola fixa e Superior configurável, categorias, escalas, gatilhos e regras por ano;
- `docs/Spuri - API.md` deve documentar contratos dos endpoints de categorias, notas e avaliação final, incluindo erros esperados para escolas;
- `docs/Manual de Configuração Inicial da Academia.md` deve orientar escolas a não criarem categorias/regras e orientar Superior a continuar configurando explicitamente.

## Correções aplicadas nesta depuração

1. A filtragem por categoria em regras escolares fixas agora percorre todas as regras aplicáveis e retorna a regra cuja `nota_despertadora` corresponde ao filtro, cobrindo corretamente `exame_recurso`.
2. O ID determinístico da regra escolar fixa passou a usar o escopo completo, incluindo `curso_id|ano_academico` para Médio quando houver curso, reduzindo risco de colisão em escolas com múltiplos cursos médios.
3. Foram adicionados testes unitários cobrindo categorias fixas, regras fixas, filtro de regra descendente por categoria e escala de notas.

## Critério de aceite

A depuração só pode ser considerada concluída quando:

- todos os itens do checklist acima forem verificados;
- eventuais bugs encontrados forem corrigidos;
- houver testes automatizados para as regras puras do padrão escolar fixo;
- a documentação e o manual refletirem a mudança sem orientar escolas a configurarem categorias ou regras escolares;
- a suíte relevante de testes passar.
