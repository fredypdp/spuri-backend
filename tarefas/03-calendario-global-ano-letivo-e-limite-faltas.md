# Admin FPP define início/fim do ano letivo escolar e superior; faltas devem respeitar intervalo

## Objetivo

Permitir que apenas Admin FPP defina o período oficial de cada ano letivo, separando calendário escolar e superior, e impedir que faltas sejam registradas fora dessas datas.

## Estado atual identificado

O sistema possui rota `POST /admin/sistema/ano-letivo` para definir `ano_letivo` global, mas o payload atual aceita apenas o valor `YYYY_YYYY`. A projeção `projection_sistema_config` já possui colunas `data_inicio` e `data_fim`, porém elas não são preenchidas pelo handler atual. A academia define seu ano letivo somente se coincidir com o global.

## Regra proposta

### Permissão

- Somente Admin FPP pode criar/alterar calendário oficial.
- Academias apenas aderem ao ano letivo oficial; não definem datas globais.

### Calendários separados

Deve existir calendário por par:

- `ano_letivo`: ex. `2025_2026`
- `tipo`: `escola` ou `superior`

Cada par deve ter:

- `data_inicio`
- `data_fim`
- `status`: `planejado`, `ativo`, `finalizado`, `cancelado`
- `definido_por`
- `definido_em`
- `observacao`

### Validações de datas

- `data_inicio < data_fim`.
- Datas em formato `YYYY-MM-DD`.
- O ano de `data_inicio` deve ser o primeiro ano do `YYYY_YYYY` ou exceção justificada.
- O ano de `data_fim` deve ser o segundo ano do `YYYY_YYYY` ou exceção justificada.
- Não permitir sobreposição de calendário ativo para mesmo `tipo`.
- Não permitir alterar datas de ano já finalizado sem processo administrativo especial e auditoria reforçada.

## Validação de faltas

Ao registrar ou atualizar falta:

1. Resolver `ano_letivo` da academia.
2. Resolver o tipo de calendário:
   - academia `escola` -> `escola`
   - academia `superior` -> `superior`
3. Buscar calendário oficial para `(ano_letivo, tipo)`.
4. Se inexistente ou sem datas, bloquear registro.
5. Validar `data >= data_inicio` e `data <= data_fim`.
6. Se fora do intervalo, retornar erro claro com o intervalo permitido.

## Endpoints sugeridos

### Criar/atualizar calendário

`POST /admin/sistema/calendarios-letivos`

```json
{
  "ano_letivo": "2025_2026",
  "tipo": "escola",
  "data_inicio": "2025-09-01",
  "data_fim": "2026-07-31",
  "observacao": "Calendário oficial escolar"
}
```

### Consultar calendário atual

`GET /admin/sistema/calendarios-letivos?ano_letivo=2025_2026&tipo=escola`

### Consulta pública autenticada para academia

`GET /academia/calendario-letivo`

Retorna o calendário efetivo da academia com base no ano letivo ativo dela.

## Fluxo operacional

1. Admin FPP cadastra ano letivo e calendários de escola/superior.
2. Academia define/ativa o ano letivo, obrigatoriamente igual ao global.
3. Academia registra faltas.
4. Backend valida a data da falta contra o calendário do tipo da academia.
5. Faltas fora do intervalo são bloqueadas antes de emitir evento.

## Impacto em banco/projeções

Preferir nova projeção `projection_calendarios_letivos` em vez de sobrecarregar uma única linha de `projection_sistema_config`, porque existem dois calendários para o mesmo ano.

Campos sugeridos:

- `id`
- `ano_letivo`
- `tipo`
- `data_inicio`
- `data_fim`
- `status`
- `observacao`
- `event_id`
- `version`
- `created_at`
- `updated_at`

Criar índice único `(ano_letivo, tipo)`.

## Testes recomendados

- Admin não FPP tenta definir calendário: 403.
- FPP cria calendário escola válido: 200/201.
- FPP cria `data_inicio >= data_fim`: 400.
- Academia registra falta antes de `data_inicio`: 400.
- Academia registra falta no primeiro dia: 201.
- Academia registra falta no último dia: 201.
- Academia registra falta depois de `data_fim`: 400.
- Atualizar falta para data fora do intervalo: 400.
