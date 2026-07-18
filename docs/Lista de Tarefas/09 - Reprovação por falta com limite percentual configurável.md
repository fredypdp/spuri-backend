---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Implementar reprovação por falta com limite percentual configurável (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento adicionando um limite percentual de faltas configurável por academia e uma reprovação automática por excesso de faltas, calculada a partir do número de aulas de cada matéria e do número de faltas do estudante nessa matéria. **Esta tarefa depende da funcionalidade de sumários/aulas** (item "[[Adicionar sumários e vincular opcionalmente às faltas]]" de `Lista de tarefas.md`, que já possui tarefa própria fora deste documento) e não deve ser iniciada antes que essa funcionalidade esteja implementada, pois o número de aulas por matéria é o dado que ainda não existe no sistema. Ao final, atualize testes, documentação técnica e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou fallbacks temporários.

## Contexto

`Documentação.md` (seção 14) confirma que o sistema **não possui** o recurso de sumários/aulas: "O recurso de sumários/aulas foi removido do contrato público da API. Não há endpoints para criar, listar, consultar, atualizar ou remover sumários, e faltas não aceitam nem retornam vínculo com sumário." Sem esse recurso, o sistema conhece o número de **faltas** de um estudante numa matéria (`FaltaDTO.quantidade`), mas não conhece o número **total de aulas** dadas naquela matéria — dado indispensável para calcular uma percentagem de faltas.

`Lista de tarefas.md` já reconhece essa dependência explicitamente: "Mecanismo de Reprovação por falta: será útil quando existir a criação de sumários/aulas para cada matéria". Por isso, esta tarefa é registrada como **bloqueada** até que a funcionalidade de sumários/aulas exista, mas já é especificada com detalhe suficiente para ser implementada assim que o pré-requisito estiver pronto, evitando retrabalho de design nesse momento.

O item também identifica uma lacuna adicional que não depende de sumários: hoje não existe nenhuma configuração de limite de faltas na academia. `Lista de tarefas.md` pede que esse limite seja expresso em percentagem.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Pré-requisito | Sumários/aulas por matéria (tarefa já existente, fora deste documento) | Esta tarefa só pode começar depois desse pré-requisito estar concluído |
| Novo campo de configuração | `limite_faltas_percentual` na academia | Percentual máximo de faltas aceito antes da reprovação automática |
| Cálculo | `(faltas da matéria / total de aulas da matéria) * 100` | Comparado contra o limite configurado |
| Consequência do excesso | Reprovação automática por falta naquela matéria | Independente do resultado por nota da avaliação final |
| Auditoria | Novo evento distinto de avaliação final por nota | Rastreabilidade separada entre reprovação por nota e reprovação por falta |

---

# 1. Adicionar configuração de limite percentual de faltas

## Objetivo

Permitir que a academia configure o percentual máximo de faltas aceito antes de um estudante ser considerado reprovado por falta numa matéria.

## Regra de negócio

Adicionar à entidade `Academia` o campo `limite_faltas_percentual` (numérico, `0` a `100`), representando o percentual máximo de aulas que um estudante pode faltar em uma matéria antes de ser reprovado automaticamente por falta naquele ano letivo/ano acadêmico.

## Escopo obrigatório

### 1.1 Configuração

- o campo deve ser editável pela academia por meio de rota apropriada (a definir no momento da implementação: pode ser uma rota dedicada ou incorporada a `PUT /academia/dados`, desde que não reabra nenhuma das brechas fechadas pela tarefa "Reforçar validações na edição de dados cadastrais dos usuários");
- deve haver validação de faixa (`0` a `100`, inclusive);
- ausência de configuração deve ser tratada de forma explícita e documentada: recomenda-se que, sem configuração explícita da academia, a reprovação automática por falta **não** seja aplicada (em vez de assumir um valor padrão arbitrário), até que a academia defina conscientemente o limite desejado.

### 1.2 Granularidade

Nesta primeira versão, o limite é único por academia (não por nível de ensino, curso ou matéria), conforme a redação original do pedido ("quantos por cento são aceitos na academia"). Se a equipe decidir, durante a implementação, que o limite deveria variar por nível de ensino, essa decisão deve ser registrada explicitamente no PR como extensão consciente do escopo original, não como suposição silenciosa.

---

# 2. Calcular percentual de faltas e disparar reprovação automática

## Objetivo

Comparar o percentual de faltas do estudante em cada matéria contra o limite configurado pela academia, e reprovar automaticamente quando o limite for ultrapassado.

## Regra de negócio

Depende diretamente da funcionalidade de sumários/aulas para obter o total de aulas por matéria/ano letivo. Assim que esse dado existir:

1. para cada matéria do estudante no ano letivo/ano acadêmico corrente, calcular `percentual_faltas = (soma de faltas do estudante na matéria / total de aulas da matéria no ano letivo) * 100`;
2. se `percentual_faltas > limite_faltas_percentual` da academia, marcar o estudante como reprovado por falta naquela matéria, **independentemente** do resultado calculado pela avaliação final por nota;
3. essa reprovação deve ser considerada no resultado geral do estudante na matéria da mesma forma que uma reprovação por nota seria considerada, integrando-se às regras já existentes de regra descendente/pendência (Superior) e de aprovação direta (escolar), sem duplicar decisão para a mesma matéria.

## Escopo obrigatório

### 2.1 Gatilho de cálculo

Definir, no momento da implementação (quando sumários/aulas existir), se o cálculo de percentual de faltas deve ser reavaliado a cada nova falta registrada, a cada nova aula registrada, ou apenas no momento em que a avaliação final por nota for calculada para aquela matéria. Recomenda-se recalcular a cada registro de falta ou de aula, para que a academia veja o risco de reprovação por falta antes do fechamento da avaliação final, mas a decisão final deve ser registrada explicitamente no PR.

### 2.2 Evento auditável distinto

Registrar a reprovação por falta com um evento/`type` claramente distinto de uma reprovação por nota (ex.: um campo `motivo_reprovacao_tipo = "falta"` no evento de avaliação final, ou um evento próprio), permitindo diferenciar, em consultas administrativas, quantos estudantes foram reprovados por nota e quantos por excesso de faltas.

### 2.3 Interação com avaliação final por nota

Se um estudante já atingiu a nota mínima de aprovação mas ultrapassou o limite de faltas, o resultado final deve ser reprovação — a nota não anula o efeito da falta. O snapshot da avaliação final deve deixar explícito que a reprovação ocorreu por falta, mesmo com nota suficiente, para evitar confusão administrativa.

### 2.4 Testes obrigatórios (a implementar junto com sumários/aulas)

1. estudante com percentual de faltas abaixo do limite configurado e nota suficiente: aprovado normalmente;
2. estudante com percentual de faltas acima do limite configurado e nota suficiente: reprovado por falta, com evento indicando o motivo;
3. estudante com percentual de faltas acima do limite e nota insuficiente: reprovado (evento pode registrar ambos os motivos, se o modelo permitir);
4. academia sem `limite_faltas_percentual` configurado: nenhuma reprovação automática por falta ocorre;
5. cálculo do percentual usando exatamente o total de aulas e o total de faltas da matéria no escopo correto (ano letivo/ano acadêmico), sem misturar matérias diferentes.

---

# 3. Atualização obrigatória da documentação

Atualizar `Documentação.md` com:

- o novo campo `limite_faltas_percentual` na entidade `Academia`;
- a regra de cálculo de percentual de faltas;
- a relação de dependência explícita com a funcionalidade de sumários/aulas;
- a forma como a reprovação por falta se integra ao resultado da avaliação final.

---

# Fora de escopo

- Implementar a funcionalidade de sumários/aulas em si (tarefa já existente e não duplicada aqui).
- Definir limite de faltas por nível de ensino, curso ou matéria, salvo decisão explícita registrada durante a implementação.
- Criar mecanismo de justificativa de falta (atestado médico, etc.) que neutralize o cálculo percentual; isso pode ser tratado em tarefa futura separada, se necessário.
- Alterar a imutabilidade de faltas já estabelecida (`docs/Tarefas feitas/Remover edicao exclusao faltas notas e validar periodo letivo.md`).

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. a funcionalidade de sumários/aulas já estiver implementada e disponível como pré-requisito;
2. `limite_faltas_percentual` existir como campo configurável da academia, validado entre `0` e `100`;
3. o cálculo de percentual de faltas por matéria estiver correto e testado;
4. a reprovação automática por falta ocorrer de forma independente do resultado por nota, quando o limite for ultrapassado;
5. o evento/snapshot distinguir claramente reprovação por nota de reprovação por falta;
6. `Documentação.md` estar atualizada;
7. testes automatizados cobrirem os cenários da seção 2.4;
8. o PR explicar claramente a integração com a avaliação final já existente.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Implementar reprovação por falta com limite percentual configurável (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
