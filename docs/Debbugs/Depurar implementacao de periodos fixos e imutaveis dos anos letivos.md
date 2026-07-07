---
modificado: 2026-07-07 00:00
criado: 2026-07-07 00:00
---
# Depurar implementação de períodos fixos e imutáveis dos anos letivos

Tarefa: [[Implementar periodos fixos e imutaveis dos anos letivos]]

## Objetivo da auditoria

Fazer uma auditoria profunda da implementação exigida em `docs/Lista de tarefas/Implementar periodos fixos e imutaveis dos anos letivos.md`, confirmando código, migrações, validações, endpoints, testes e documentação. A regra oficial é sistêmica e não configurável:

| Tipo | Período obrigatório |
| --- | --- |
| `escolar` | `09_07` |
| `superior` | `10_07` |

## Resultado da depuração

A auditoria encontrou uma implementação anterior ainda parcialmente configurável: `periodoConfigurado` lia `projection_anos_letivos_configuracoes`, o endpoint `PUT /admin/sistema/anos-letivos/configuracoes/:type` aceitava e persistia qualquer período válido em formato `MM_MM`, e a migration histórica inicializava `superior` com `02_12`. Isso violava o novo contrato, pois permitia transformar `escolar` em `10_07`, `superior` em valores arbitrários e validar faltas/finalizações por janelas mutáveis.

A correção aplicada garante que:

- a regra central `periodoFixoAnoLetivo(type)` resolve exclusivamente `escolar -> 09_07` e `superior -> 10_07`;
- payloads legados com `periodo` igual ao valor fixo são aceitos apenas como confirmação compatível;
- payloads divergentes são rejeitados por validação explícita;
- leituras e validações internas deixam de usar o valor persistido como fonte de verdade;
- listagens retornam o período derivado/fixo calculado pelo backend;
- migration corretiva força os dados legados para `09_07`/`10_07` e adiciona constraint contra regressão;
- testes cobrem resolução fixa, rejeição de divergência e intervalos reais para escolar e superior;
- a documentação passou a descrever `periodo` como fixo, imutável e não configurável.

## Checklist obrigatório validado

### 1. Regra centralizada

- Confirmado e corrigido em `internal/handlers/ano_letivo_helpers.go`.
- `periodoFixoAnoLetivo` é a única fonte de verdade para períodos de ano letivo.
- Tipos desconhecidos falham de forma explícita via `normalizarTipoAnoLetivo`.

### 2. Criação e atualização

- `POST /admin/sistema/definir-ano-letivo-geral` continua recebendo apenas `type` e `ano_letivo`; não há `periodo` no contrato.
- `POST /academia/definir-ano-letivo` continua recebendo apenas o ano letivo opcional; o tipo vem da academia e não há `periodo` configurável.
- `PUT /admin/sistema/anos-letivos/configuracoes/:type` deixou de gravar períodos arbitrários. Ele deriva o valor fixo pelo tipo e rejeita `periodo` divergente quando o campo legado vier no payload.

### 3. Persistência e migrações

- Adicionada migration corretiva para atualizar registros incompatíveis e criar constraint fixa em `projection_anos_letivos_configuracoes`.
- A migration também garante a existência das duas linhas oficiais: `escolar=09_07` e `superior=10_07`.

### 4. Validações dependentes de datas letivas

- `validarDataNoPeriodoLetivo` usa `periodoConfigurado`, que agora retorna o valor fixo derivado do tipo.
- `validarMesAtualPermiteFinalizacaoAnoLetivo` também usa o período fixo, impedindo que finalizações dependam de configuração mutável.
- `intervaloAnoLetivo` continua sendo helper puro para compor `ano_letivo` + `periodo`; os chamadores de negócio recebem o período fixo pelo tipo.

### 5. Documentação

- O manual de configuração inicial informa que `periodo` não deve ser enviado e que os valores são fixos por tipo.
- A documentação da tarefa anterior de separação de ano letivo foi atualizada com uma nota de contrato supersedente, deixando claro que a configuração livre foi substituída por período fixo.

## Testes mínimos exigidos

- Resolver período escolar como `09_07`.
- Resolver período superior como `10_07`.
- Rejeitar tipo desconhecido.
- Aceitar payload legado compatível (`escolar` com `9_7`, normalizado para `09_07`).
- Rejeitar `escolar` com `10_07`.
- Rejeitar `superior` com `09_07`.
- Calcular intervalo escolar de `2025_2026` como `2025-09-01` até `2026-07-31`.
- Calcular intervalo superior de `2025_2026` como `2025-10-01` até `2026-07-31`.

## Conclusão

A tarefa foi garantida e completada. O backend não trata mais o período do ano letivo como configuração editável; ele apenas expõe e persiste o valor fixo derivado da regra sistêmica para compatibilidade com projeções existentes.
