---
modificado: 2026-06-18 20:30
criado: 2026-06-18 20:30
---
# Adaptar avaliação final automática do ensino superior para progressão semestral

## Contexto

O modelo correto para o ensino superior no Spuri deve tratar o **semestre** como a unidade letiva de progressão. Diferente do fundamental e do médio, que avançam por ano acadêmico, o superior possui períodos semestrais (`1_semestre`, `2_semestre`, ..., `N_semestre`) configurados no curso e utilizados nas matérias e notas.

Historicamente, a transição do estudante superior era orientada por `semestre_atual`:

- o estudante superior possuía `semestre_atual` como posição sequencial no curso;
- o `ano_superior` era derivado do semestre atual pela fórmula `ceil(semestre_atual / 2)`;
- a avaliação final superior recebia/operava sobre o semestre atual, por exemplo `nivel_ano_academico_atual = "1_semestre"`;
- se aprovado, o backend avançava `semestre_atual += 1` até o último semestre do curso;
- se aprovado no último semestre, `status_superior` era marcado como `finalizado`;
- se reprovado, o estudante permanecia no mesmo semestre;
- a aprovação/reprovação era registrada no histórico da avaliação final.

Com a avaliação final automática atual, o fluxo foi generalizado para usar `ano_academico_atual` e `curso.anos_academicos`. Para o superior, isso faz a avaliação avançar diretamente de `1_ano_superior` para `2_ano_superior`, tratando os semestres apenas como `periodos` de notas/fórmulas. Essa abordagem simplifica a progressão, mas não representa corretamente o funcionamento acadêmico semestral do ensino superior.

## Problema a resolver

A avaliação final automática do ensino superior precisa deixar de decidir a progressão diretamente pelo `ano_superior` e passar a decidir a progressão pelo **semestre atual do estudante**.

O ano superior deve continuar existindo, mas como dado derivado/compatível para consultas, turmas, relatórios e agrupamentos acadêmicos. A fonte de progressão no superior deve ser o `semestre_atual`.

## Por que esse é o modelo correto para o ensino superior

1. **Disciplinas superiores são semestrais**: matérias do tipo `superior` já exigem `periodo` (`1_semestre`, `2_semestre`, etc.) e o período precisa existir no curso.
2. **Notas superiores são vinculadas ao semestre da matéria**: uma nota superior só pode ser registrada no mesmo `periodo` da matéria.
3. **Cursos superiores têm semestres dinâmicos**: o curso define `periodos`, e esses períodos são a sequência natural para progressão acadêmica.
4. **O ano superior é uma agregação do semestre**: em cursos semestrais, dois semestres normalmente compõem um ano acadêmico; portanto `ano_superior` deve refletir o semestre, não comandar a progressão.
5. **Evita avanço prematuro de ano**: se o estudante conclui apenas o `1_semestre`, ele deve avançar para `2_semestre`, não para `2_ano_superior`.
6. **Mantém compatibilidade com fórmulas**: a avaliação final automática já usa `sum_periods`, categorias e notas por período. No superior, o período avaliado deve ser o semestre atual.

## Objetivo da atualização

Adaptar o código atual de avaliação final automática para que, quando `tipo_ensino = "superior"`, a regra aplicável, as notas carregadas, a decisão de aprovação/reprovação e a transição do estudante sejam baseadas no semestre atual.

O fluxo esperado é:

```text
Curso superior com 8 semestres

1_semestre aprovado → semestre_atual = 2, ano_superior = 1_ano_superior
2_semestre aprovado → semestre_atual = 3, ano_superior = 2_ano_superior
3_semestre aprovado → semestre_atual = 4, ano_superior = 2_ano_superior
...
8_semestre aprovado → status_superior = finalizado

Qualquer semestre reprovado → permanece no mesmo semestre e mesmo ano derivado
```

## Regras funcionais esperadas

### 1. Identificação do semestre atual

Para estudantes do superior, o backend deve usar `semestre_atual` como nível corrente da avaliação final.

- Se `semestre_atual` estiver ausente ou inválido, a avaliação final superior deve ser bloqueada.
- O semestre acadêmico corrente deve ser convertido para string de período no formato `[n]_semestre`, por exemplo `semestre_atual = 3` → `3_semestre`.
- O período `[n]_semestre` precisa existir em `curso.periodos`.

### 2. Regra de avaliação final superior por semestre

As regras de avaliação final do superior devem poder ser aplicadas por semestre/período.

Opções possíveis de modelagem:

- criar/usar campo específico como `periodos` na regra de avaliação final; ou
- permitir que `anos_academicos` de regras superiores aceite valores semestrais (`1_semestre`, `2_semestre`, etc.) apenas para `tipo_ensino = "superior"`; ou
- manter `anos_academicos` para compatibilidade, mas resolver internamente a regra superior pelo semestre atual.

A escolha técnica deve evitar ambiguidade entre ano superior (`1_ano_superior`) e período semestral (`1_semestre`).

### 3. Cálculo da nota final no superior

Para `tipo_ensino = "superior"`, a fórmula deve considerar as notas do semestre avaliado.

- `sum_periods` deve exigir, no mínimo, o `periodo` correspondente ao semestre atual.
- A fórmula pode continuar usando categorias (`nota_pp1`, `nota_pp2`, `nota_exame`, categorias adicionais etc.).
- Se faltar nota obrigatória de matéria/categoria/período exigido, a avaliação deve aguardar novos lançamentos ou retornar erro, conforme o caminho de execução.
- Não deve haver aprovação manual por `observacao`.

### 4. Transição em caso de aprovação

Se aprovado em semestre que não é o último período do curso:

1. incrementar `semestre_atual` para o próximo semestre sequencial;
2. recalcular `ano_superior = ceil(semestre_atual / 2)`;
3. persistir o novo `semestre_atual` e o novo `ano_superior` no aggregate/projeção;
4. registrar no evento de avaliação final o semestre avaliado e o próximo semestre calculado.

Exemplos:

| Semestre avaliado | Resultado | Novo `semestre_atual` | Novo `ano_superior` |
|---|---|---:|---|
| `1_semestre` | aprovado | `2` | `1_ano_superior` |
| `2_semestre` | aprovado | `3` | `2_ano_superior` |
| `3_semestre` | aprovado | `4` | `2_ano_superior` |
| `4_semestre` | aprovado | `5` | `3_ano_superior` |

### 5. Transição em caso de aprovação no último semestre

Se o estudante for aprovado no último semestre configurado no curso:

- não há próximo semestre;
- `status_superior` deve passar para `finalizado`;
- `semestre_atual` pode permanecer no último semestre aprovado para histórico/consulta;
- `ano_superior` deve permanecer coerente com o último semestre.

### 6. Reprovação

Se reprovado:

- `semestre_atual` não muda;
- `ano_superior` não muda;
- `status_superior` não muda;
- a avaliação final reprovada deve ser registrada com o semestre avaliado;
- regras dependentes (`recurso`, `especial`, etc.) podem ser executadas somente se configuradas para aquele semestre e dependentes da reprovação anterior.

### 7. Idempotência e unicidade

A unicidade da avaliação final superior deve considerar o semestre avaliado.

Um estudante não pode ter duas avaliações finais do mesmo `type` no mesmo:

- `codigo_academia`;
- `ano_lectivo`;
- `tipo_ensino = superior`;
- semestre/período avaliado (`1_semestre`, `2_semestre`, etc.);
- `type` de avaliação (`normal`, `recurso`, etc.).

O sistema deve impedir que uma reprovação/aprovação de `1_semestre` bloqueie indevidamente a avaliação de `2_semestre` no mesmo ano letivo.

### 8. Eventos e projeções

Os eventos/projeções de avaliação final superior devem carregar dados suficientes para rebuild determinístico:

- semestre avaliado (`semestre_atual` e/ou `periodo_avaliado`);
- próximo semestre calculado, se aprovado e não finalizado;
- `ano_superior` antes/depois, se necessário para compatibilidade;
- `nota_final`;
- `nota_minima_aprovacao`;
- `type`;
- `regra_avaliacao_final_id`;
- `formula_snapshot`;
- `aplica_se_reprovado_em_type`.

A projeção de estudantes deve atualizar `semestre_atual` e `ano_superior` quando a avaliação superior aprovar com próximo semestre.

## Impactos esperados no código atual

### Avaliação final

Adaptar a resolução de nível atual para superior:

- hoje usa `ano_superior`/`ano_academico_atual` para calcular o próximo nível;
- deve usar `semestre_atual` convertido em `[n]_semestre`.

Adaptar o cálculo de próximo nível:

- manter `calcularProximoAnoCurso` para médio;
- criar cálculo específico para superior, por exemplo `calcularProximoSemestreCurso`;
- validar o semestre atual contra `curso.periodos`;
- retornar próximo semestre e ano superior derivado.

### Aggregate `Estudante`

Adaptar o evento de avaliação final superior para conseguir atualizar:

- `SemestreAtual`;
- `AnoSuperior` derivado;
- `StatusSuperior` quando finalizado.

### Projeção de estudantes

Adaptar o handler da projeção para, em avaliação superior aprovada:

- atualizar `semestre_atual` quando houver próximo semestre;
- recalcular/persistir `ano_superior`;
- finalizar `status_superior` no último semestre.

### Projeção de avaliação final

Adicionar ou reutilizar campos para identificar claramente o semestre avaliado. Se o campo atual `ano_academico_atual` for mantido, para superior ele deve armazenar o período semestral avaliado (`1_semestre`, `2_semestre`, etc.) ou deve ser criado um campo mais explícito, como `periodo_avaliado`.

### Regras de avaliação final

Revisar o schema de regras para suportar aplicação por semestre no superior sem quebrar fundamental/médio.

Sugestão:

- `tipo_ensino = fundamental|medio` continua usando `anos_academicos`;
- `tipo_ensino = superior` usa `periodos`/`semestres`;
- validar unicidade por escopo correto.

### Rotas e contratos

Revisar payloads e documentação para deixar claro que:

- fundamental/médio avaliam por ano acadêmico;
- superior avalia por semestre;
- `ano_superior` é derivado de `semestre_atual`;
- cliente não deve enviar `aprovado` nem `proximo_ano_academico`;
- cliente não deve manipular diretamente a transição de semestre.

## Critérios de aceite

1. Estudante superior em `semestre_atual = 1`, aprovado na avaliação final do `1_semestre`, passa para `semestre_atual = 2` e permanece em `1_ano_superior`.
2. Estudante superior em `semestre_atual = 2`, aprovado na avaliação final do `2_semestre`, passa para `semestre_atual = 3` e muda para `2_ano_superior`.
3. Estudante superior reprovado permanece no mesmo `semestre_atual` e no mesmo `ano_superior`.
4. Estudante aprovado no último semestre do curso tem `status_superior = finalizado`.
5. Avaliação final superior não registra duplicidade para o mesmo estudante, ano letivo, semestre e `type`.
6. Avaliação final de `2_semestre` não é bloqueada por já existir avaliação final de `1_semestre`.
7. Fórmulas superiores usam notas do semestre/período avaliado.
8. Regras dependentes (`recurso`, `especial`) funcionam por semestre e dependem de reprovação anterior no mesmo semestre.
9. Rebuild das projeções reproduz corretamente `semestre_atual`, `ano_superior`, avaliações e status finalizado.
10. Documentação/API explicam que superior progride por semestre e que `ano_superior = ceil(semestre_atual / 2)`.

## Observações de migração

- Verificar dados existentes com `status_superior = em_andamento` e `semestre_atual` nulo.
- Definir estratégia de backfill para `semestre_atual` a partir de `ano_superior`, quando possível.
- Avaliar compatibilidade de avaliações finais superiores já registradas por `ano_superior`; elas podem precisar ser preservadas como legado ou migradas para o semestre correspondente.
- Garantir que cursos superiores possuam `periodos` coerentes com a quantidade de anos acadêmicos.

## Resultado esperado da adaptação

Após a atualização, o ensino superior passa a ter um fluxo coerente com sua natureza semestral:

```text
matérias/notas por semestre
       ↓
avaliação final automática do semestre
       ↓
aprovação: próximo semestre
reprovação: mesmo semestre
último semestre aprovado: superior finalizado
```

O ano superior deixa de ser a unidade principal da transição e passa a ser uma informação derivada/agrupadora, mantendo compatibilidade com relatórios, turmas e filtros existentes.
