---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Padronizar avaliação final diante de notas ausentes ou alteradas (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que, quando a categoria configurada como `nota_despertadora` (ou o gatilho fixo escolar equivalente: `prova_trimestral` do 3º trimestre, `exame_final` ou `nota_pap`) for lançada para uma matéria, mas ainda faltar alguma outra nota exigida pela fórmula **para essa mesma matéria**, o backend passe a considerar `0` (zero) para a nota ausente em vez de deixar a avaliação aguardando indefinidamente. Documente também, sem implementar código de edição de notas, a regra de negócio que deve ser seguida quando um mecanismo de correção/edição de notas existir no futuro. Ao final, atualize testes, documentação técnica e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou comportamento silenciosamente divergente entre o modelo escolar fixo e o modelo configurável do Superior.

## Contexto

`Documentação.md` (seção 16.1.3, "Execução automática por lançamento de notas") descreve o comportamento atual: "Se a fórmula exigir nota que ainda não existe para determinada matéria, categoria e período, aquela execução não fecha a avaliação naquele momento; o lançamento de novas notas tentará novamente." A mesma limitação é reafirmada na seção 16.1.5 para o modelo escolar fixo: "a avaliação final só fecha quando existirem as notas exigidas de professor e prova trimestral nos três trimestres de cada matéria avaliada."

Esse comportamento cria um risco operacional real: se o professor lançar a nota que serve de `nota_despertadora` (tipicamente a última peça de dado esperada do ano, como `exame_final` ou `prova_trimestral` do 3º trimestre) mas esquecer de lançar uma nota anterior da mesma matéria (por exemplo, `nota_professor` do 1º trimestre), a avaliação daquela matéria fica pendente **indefinidamente**, sem erro visível para a academia, até que alguém perceba a lacuna e lance a nota faltante manualmente. Como o gatilho já disparou, não há nenhum novo evento de nota que force uma nova tentativa de cálculo para essa matéria specificamente — a avaliação simplesmente nunca se completa.

`Lista de tarefas.md` propõe resolver isso considerando `0` para a nota ausente no momento em que o gatilho dispara. Esse mesmo item de tarefas também levanta uma pergunta relacionada, mas não diretamente implementável hoje: como proceder quando notas forem alteradas depois de uma avaliação final já registrada. Atualmente as notas são imutáveis por definição de produto — `Documentação.md` (seção 13.1) afirma explicitamente que "notas só podem ser criadas e consultadas" e que "não existe endpoint público, administrativo, batch ou assíncrono para editar, eliminar, restaurar ou ocultar notas por soft delete" (ver também `docs/Tarefas feitas/Remover edicao exclusao faltas notas e validar periodo letivo.md`). Como não existe hoje nenhum caminho para alterar uma nota já registrada, a parte de "recálculo após alteração" não pode ser implementada em código nesta tarefa; ela deve ser registrada como decisão de produto para orientar quem, no futuro, eventualmente construir um mecanismo de correção de notas.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Nota ausente ao disparar o gatilho | Considerar `0` para a nota ausente da mesma matéria | Avaliação final da matéria é concluída no momento do gatilho, em vez de ficar pendente para sempre |
| Escopo do "zero" | Aplicado apenas à matéria que já possui a nota-gatilho, para as demais notas exigidas dessa mesma matéria | Matérias sem nenhuma nota lançada continuam aguardando seu próprio gatilho, sem serem forçadas |
| Abrangência | Aplicado tanto ao modelo escolar fixo quanto ao modelo configurável do Superior | Nenhuma divergência de comportamento entre os dois modelos |
| Auditoria | Registrar no snapshot quais notas foram substituídas por zero | Rastreabilidade completa de quando o zero foi aplicado |
| Notas alteradas após avaliação final | Registrar regra de negócio para o futuro, sem implementar edição de notas agora | Documentação preparada para quando um mecanismo de correção de notas existir |

---

# 1. Tratar nota ausente como zero no momento do gatilho

## Objetivo

Garantir que, ao disparar o gatilho de avaliação final para uma matéria, o backend não deixe a avaliação daquela matéria pendente indefinidamente por causa de uma nota ausente de outra categoria/período da mesma matéria.

## Regra de negócio

Quando o backend identificar que a categoria da nota recém-lançada corresponde à `nota_despertadora` da regra raiz aplicável (ou ao gatilho fixo escolar equivalente: `prova_trimestral` do 3º trimestre nos anos sem exame, `exame_final` nos anos com exame, `nota_pap` no `4_ano_medio` técnico, ou `exame_recurso` na etapa de recurso), o backend deve:

1. resolver a fórmula normalmente para a matéria cujo gatilho disparou;
2. para cada referência `[categoria,periodo]` (ou `[categoria]` no Superior, com período inferido) exigida pela fórmula **dessa mesma matéria** que ainda não tiver nota lançada, usar o valor `0` no lugar da nota ausente;
3. calcular `nota_final` normalmente com os valores disponíveis e os zeros substituídos;
4. comparar `nota_final` com `nota_minima_aprovacao` e prosseguir com o fluxo de aprovação/reprovação/regra descendente já existente, sem alteração de comportamento a partir daí.

## Escopo obrigatório

### 1.1 Escopo do zero por matéria, não por estudante inteiro

O comportamento de considerar zero se aplica **apenas à matéria que já recebeu a nota-gatilho**. Matérias do mesmo estudante que ainda não possuem nenhuma nota lançada (incluindo a própria categoria-gatilho) **não** devem ser forçadas a calcular com zero; elas continuam aguardando o próprio lançamento de nota, como já ocorre hoje. Isso evita reprovar matérias que simplesmente ainda não começaram a ser avaliadas no período.

### 1.2 Aplicação ao modelo escolar fixo e ao modelo configurável do Superior

A mudança deve ser implementada no ponto de execução comum usado tanto pelas regras fixas escolares (fundamental e médio) quanto pelas regras configuráveis do Superior, para evitar divergência de comportamento entre os dois modelos. Se hoje esses dois fluxos possuem implementações separadas, a correção deve alcançar ambos de forma equivalente, preferencialmente por meio de um único ponto de resolução de notas por fórmula reutilizado pelos dois.

### 1.3 Regras descendentes e recurso

A mesma regra de zero se aplica à execução de regras descendentes (ex.: `exame_recurso`), sempre restrita às matérias e ao escopo aplicável daquela etapa. Uma matéria não reprovada na etapa anterior não deve ser afetada pela substituição por zero de uma etapa descendente que não se aplica a ela.

### 1.4 Auditoria da substituição por zero

O snapshot/evento da avaliação final deve registrar, para cada matéria avaliada, quais referências `[categoria,periodo]` (ou `[categoria]`) foram efetivamente lançadas pelo professor e quais foram substituídas por zero por ausência de nota no momento do gatilho. Essa informação deve estar disponível para consulta administrativa (ex.: no detalhe da avaliação final), permitindo que a academia identifique rapidamente lançamentos esquecidos.

### 1.5 Testes obrigatórios

Adicionar ou ajustar testes cobrindo:

1. gatilho dispara com todas as notas exigidas presentes: comportamento inalterado;
2. gatilho dispara com uma nota exigida ausente na mesma matéria: avaliação é concluída usando zero para a nota ausente;
3. gatilho dispara com múltiplas notas exigidas ausentes na mesma matéria: todas são tratadas como zero;
4. matéria sem nenhuma nota lançada (incluindo a categoria-gatilho) não é avaliada nem forçada por esta regra;
5. comportamento equivalente para o modelo escolar fixo (fundamental e médio) e para o modelo configurável do Superior;
6. regra descendente (`exame_recurso`) aplicando zero apenas dentro do próprio escopo, sem afetar matérias fora dele;
7. snapshot da avaliação final registrando quais notas foram substituídas por zero.

---

# 2. Registrar a regra de negócio para notas alteradas após avaliação final

## Objetivo

Deixar documentada, de forma explícita e auditável, a decisão de produto sobre como recalcular uma avaliação final quando um mecanismo de correção/edição de notas existir no futuro — sem implementar esse mecanismo agora, já que notas continuam imutáveis por definição vigente do produto.

## Regra de negócio a documentar (sem implementação de código nesta tarefa)

Quando uma funcionalidade de correção/edição de notas for eventualmente criada, o comportamento esperado é:

1. verificar se o ano letivo atualmente ativo da academia é o mesmo ano letivo da nota corrigida e da avaliação final já registrada para aquele estudante/matéria/escopo;
2. se for o mesmo ano letivo, refazer o cálculo da avaliação final daquela matéria usando a nota corrigida;
3. o resultado do novo cálculo deve ser registrado como um **novo evento** distinto do evento de avaliação final original, identificado claramente como reavaliação (ex.: um `type` ou campo próprio indicando que se trata de uma reavaliação decorrente de correção de nota), preservando o evento original para auditoria;
4. se o ano letivo ativo da academia já tiver avançado (não for mais o mesmo ano letivo da nota/avaliação original), a reavaliação automática não deve ocorrer; o tratamento nesse cenário fica em aberto para definição junto com a própria funcionalidade de edição de notas, quando ela for especificada.

## Escopo obrigatório

### 2.1 Registrar a decisão em documentação viva

Adicionar esta regra à seção de avaliação final de `Documentação.md`, deixando explícito que:

- notas são imutáveis na versão atual do sistema;
- esta regra é uma decisão de produto para orientar uma futura funcionalidade de correção de notas, e não descreve comportamento já implementado;
- qualquer implementação futura de edição de notas deve reutilizar esta regra em vez de inventar um comportamento novo sem revisão.

### 2.2 Não implementar edição de notas nesta tarefa

Esta tarefa não deve criar nenhum endpoint, comando, evento ou caminho de código que permita editar, corrigir ou substituir uma nota já registrada. O único código a ser alterado por esta tarefa é o descrito na seção 1 (substituição de nota ausente por zero no momento do gatilho).

---

# Fora de escopo

- Implementar qualquer mecanismo de edição, correção ou exclusão de notas já registradas.
- Alterar a proteção de duplicidade de notas (`NotasRegistradasPorChave`).
- Alterar a escala de notas por ano acadêmico.
- Alterar as regras de aprovação/reprovação, pendência ou regras descendentes além do necessário para tratar a nota ausente como zero.
- Criar aliases, wrappers de compatibilidade ou comportamento condicional que mantenha o "aguardar indefinidamente" como opção configurável.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. o gatilho da avaliação final, ao disparar, considerar `0` para qualquer nota ausente exigida pela fórmula da mesma matéria;
2. matérias sem nenhuma nota lançada não forem afetadas por esta regra;
3. o comportamento for idêntico entre o modelo escolar fixo e o modelo configurável do Superior;
4. o snapshot da avaliação final registrar quais notas foram substituídas por zero;
5. a regra de negócio para notas alteradas após avaliação final estiver documentada em `Documentação.md` como decisão de produto para implementação futura, sem código de edição de notas sendo criado nesta tarefa;
6. testes automatizados cobrirem os cenários da seção 1.5;
7. o PR explicar claramente a mudança de comportamento e o impacto em avaliações que hoje ficam pendentes por notas ausentes.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Padronizar avaliação final diante de notas ausentes ou alteradas (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
