# Limitar paginação de endpoints GET pesados (feito)

## Contexto

Os endpoints de listagem que podem crescer para centenas de milhares de registros não devem permitir páginas grandes nem comportamento sem paginação efetiva. Consultas sem teto rígido podem varrer tabelas grandes no banco de dados e consumir CUs desnecessários.

## Regra implementada

- O limite global sanitizado por `db.ValidateLimit` passa a ter teto fixo de **100 registros por página**.
- Quando o cliente omite `limit`, o backend usa o padrão de **50 registros por página**.
- Quando o cliente envia `limit` acima de 100, o backend reduz para **100**, sem exceção.
- `offset` continua aceitando valores não negativos e é normalizado para `0` quando inválido.

## Rotas diretamente protegidas

- `GET /notas`
  - Usa `LIMIT/OFFSET` e retorna no máximo 100 notas por página.
- `GET /faltas`
  - Usa `LIMIT/OFFSET` e retorna no máximo 100 faltas por página.
- `GET /estudantes`
  - Agora aplica `LIMIT/OFFSET` para administradores e academias.
  - A resposta inclui `limit` e `offset` para deixar a paginação explícita.
- `GET /academias`
  - Deixou de usar o retorno ampliado de até 1.000 registros quando `limit` era omitido.
  - Agora também respeita o padrão global: 50 por página por omissão e teto de 100.

## Outras rotas GET impactadas pelo teto global

Toda rota que usa `getPaginationParams` em conjunto com `db.ValidateLimit` passa a respeitar o teto de 100. Isso inclui listagens administrativas de registros e demais handlers que adotam o helper global de paginação.

## Observação para novas rotas

Novas rotas GET que possam listar muitas linhas devem usar `getPaginationParams` + `db.ValidateLimit`, ou uma validação equivalente com teto máximo de 100, antes de montar a query SQL. Não criar exceções com limite maior sem uma tarefa específica de produto e avaliação de custo operacional.
