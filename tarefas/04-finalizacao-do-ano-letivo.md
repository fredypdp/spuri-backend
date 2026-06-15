# Funcionalidade de finalização do ano letivo por academias

## Objetivo

Criar um mecanismo para declarar que uma academia finalizou determinado ano letivo. Quando todas as academias aplicáveis finalizarem um ano, a plataforma não deve permitir configurar ano letivo global anterior ou igual; o próximo ano global deve ser sempre o seguinte.

## Problema

Atualmente o Admin FPP pode substituir o ano letivo global por outro valor válido no formato `YYYY_YYYY`. Isso permite erro operacional: depois de academias terem trabalhado em `2025_2026`, alguém pode voltar para `2024_2025`, gerando inconsistências em notas, faltas, turmas e avaliações.

## Conceitos

- **Ano letivo ativo**: ano em que a academia está operando.
- **Ano letivo finalizado pela academia**: academia declara que encerrou lançamentos, avaliações e movimentações daquele ano.
- **Ano letivo consolidado pela plataforma**: todas as academias elegíveis finalizaram o mesmo ano; a plataforma avança o piso mínimo global.
- **Piso mínimo global**: menor ano letivo que o Admin FPP pode definir. Se `2025_2026` está consolidado, o próximo permitido é `2026_2027`.

## Regras de negócio

### Quem pode finalizar

- Academia pode finalizar apenas seu próprio ano letivo ativo.
- Admin FPP pode consultar progresso e, opcionalmente, forçar finalização com motivo administrativo.

### Pré-condições para academia finalizar

- Academia tem ano letivo ativo.
- Não há avaliações finais pendentes para estudantes em andamento, salvo configuração que permita finalização parcial com justificativa.
- Não há jobs assíncronos acadêmicos pendentes para a academia.
- Não há notas/faltas em lote ainda processando.
- Opcional: todos estudantes com turma/curso ativo tiveram avaliação final no ano.

### Efeitos da finalização

- Bloquear novos registros acadêmicos naquele ano: notas, faltas, avaliações finais, movimentações automáticas de turma.
- Permitir apenas consultas e relatórios.
- Correções pós-fecho exigem fluxo de reabertura com motivo e permissão elevada.

### Consolidação global

Quando todas as academias ativas e elegíveis finalizarem `YYYY_YYYY`:

- Marcar o ano como consolidado globalmente.
- Impedir `POST /admin/sistema/ano-letivo` com ano menor ou igual ao consolidado.
- Sugerir automaticamente o próximo ano `YYYY+1_YYYY+2`.

## Endpoints sugeridos

### Academia finaliza seu ano

`POST /academia/ano-letivo/finalizar`

```json
{
  "ano_letivo": "2025_2026",
  "observacao": "Ano letivo encerrado após avaliações finais"
}
```

### Admin consulta status global

`GET /admin/sistema/anos-letivos/finalizacoes?ano_letivo=2025_2026`

### Admin reabre ano de uma academia

`POST /admin/sistema/anos-letivos/reabrir-academia`

```json
{
  "codigo_academia": "ACAD001",
  "ano_letivo": "2025_2026",
  "motivo": "Correção administrativa autorizada"
}
```

## Eventos propostos

- `AnoLetivoAcademiaFinalizado`
- `AnoLetivoAcademiaReaberto`
- `AnoLetivoGlobalConsolidado`

## Projeções sugeridas

### `projection_anos_letivos_academia_status`

- `codigo_academia`
- `ano_letivo`
- `status`: `ativo`, `finalizado`, `reaberto`
- `finalizado_em`
- `finalizado_por`
- `observacao`

### `projection_anos_letivos_globais_status`

- `ano_letivo`
- `status`: `aberto`, `consolidado`
- `total_academias_elegiveis`
- `total_finalizadas`
- `consolidado_em`

## Fluxo operacional

1. Admin FPP define ano global `2025_2026`.
2. Academias aderem ao ano e operam normalmente.
3. Ao terminar, cada academia chama finalizar.
4. Backend valida pendências.
5. Evento de finalização é gravado.
6. Projeção atualiza status.
7. Quando a última academia finaliza, plataforma emite ou calcula consolidação global.
8. Tentativa de voltar para `2025_2026` ou anterior passa a ser bloqueada.

## Testes recomendados

- Academia finaliza sem ano ativo: 400.
- Academia finaliza ano diferente do ativo: 400.
- Academia finaliza com estudante pendente: 409.
- Após finalizada, registrar nota/falta no ano finalizado: 409.
- Todas academias finalizam: status global consolidado.
- Admin tenta definir ano anterior ao consolidado: 400.
- Admin define próximo ano: 200.
