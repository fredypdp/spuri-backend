# Período oficial do ano letivo escolar e superior

## Objetivo
Permitir que apenas o Admin FPP defina as datas oficiais de início e fim do ano letivo para ensino escolar e superior, garantindo que faltas, notas e avaliações fiquem dentro de um período válido.

## Regra principal
O sistema deve manter calendário oficial global por ano letivo e tipo de ensino. A data de qualquer falta deve estar dentro do intervalo oficial correspondente ao nível da academia e ao ano letivo ativo.

## Responsável
Somente Admin FPP pode criar, editar ou encerrar períodos oficiais globais.

## Configuração necessária
Para cada ano letivo:
- `ano_letivo`.
- `tipo_ensino`: `escolar` ou `superior`.
- `data_inicio`.
- `data_fim`.
- `status`: `rascunho`, `ativo`, `finalizado`, `cancelado`.
- `criado_por` e `alterado_por`.
- `justificativa` para edição após ativação.

## Regras de data
- `data_inicio` deve ser menor que `data_fim`.
- Períodos do mesmo tipo de ensino não devem se sobrepor.
- Um ano letivo ativo deve ter exatamente um período ativo por tipo de ensino, quando aplicável.
- Faltas só podem ser registradas se `data` estiver entre `data_inicio` e `data_fim`, inclusive.
- Data deve ser tratada como date-only, sem impacto de timezone.
- Faltas retroativas dentro do intervalo podem ser limitadas por configuração da academia.

## Fluxo operacional: definição do calendário
1. Admin FPP acessa painel de calendário global.
2. Seleciona ano letivo e tipo de ensino.
3. Informa data de início e fim.
4. Sistema valida conflitos e sobreposição.
5. Admin confirma.
6. Sistema grava evento `PeriodoAnoLetivoDefinido`.
7. Projeção global passa a disponibilizar o período para validações.

## Fluxo operacional: registro de falta
1. Academia envia falta com data.
2. Sistema identifica academia, nível e ano letivo ativo.
3. Sistema resolve tipo de ensino: escolar ou superior.
4. Sistema consulta período global ativo para aquele ano letivo e tipo.
5. Se não existir período, bloquear registro com erro claro.
6. Se data estiver fora do intervalo, bloquear registro.
7. Se data estiver dentro do intervalo, seguir validações atuais de matéria, estudante e idempotência.

## Mensagens de erro recomendadas
- `período oficial do ano letivo não definido para este tipo de ensino`.
- `data da falta está fora do período oficial do ano letivo`.
- `data_inicio deve ser anterior a data_fim`.
- `período informado sobrepõe outro ano letivo ativo`.

## Impactos em outras funcionalidades
Embora a demanda cite faltas, a mesma base deve ser reutilizada para:
- Notas.
- Avaliações finais.
- Matrículas/rematrículas por ano letivo.
- Fechamento do ano letivo.

## Critérios de aceite
- Apenas Admin FPP consegue definir datas oficiais.
- Academia não consegue registrar falta fora do período oficial.
- Sistema diferencia calendário escolar e superior.
- Edições em período ativo são auditadas.
- Faltas existentes fora do intervalo, em migração, devem ser relatadas para saneamento e não apagadas automaticamente.
