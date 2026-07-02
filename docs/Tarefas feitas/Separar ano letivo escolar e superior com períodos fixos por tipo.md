---
criado: 2026-06-21 20:10
origem: tarefas/Lista de tarefas.md#1
status: pronto_para_implementacao
modificado: 2026-06-27 19:07
---

# Tarefa 1 — Separar ano letivo escolar e superior com períodos fixos por tipo (feito)

## Prompt recomendado para executar a atualização

Implemente no backend a separação do ano letivo global/da academia em dois tipos independentes: `escolar` e `superior`. Cada tipo deve ter um `periodo` configurável pelo admin FPP no formato `MM_MM` (ex.: `09_07`), representando o mês de início no ano inicial do `ano_letivo` e o mês de término no ano final do `ano_letivo`. O `ano_letivo` continua mudando ao longo do tempo (`2025_2026`, `2026_2027`, etc.), mas o `type` e o `periodo` são configurações estáveis do tipo e não devem ser recriados como se fossem o próprio ano letivo. Também valide que datas de faltas fiquem dentro do intervalo de datas calculado a partir do `ano_letivo` + `periodo` do tipo correto.

## Contexto do problema

Hoje o sistema trata o ano letivo como um valor único, com `tipo` aceitando valores como `escola`/`superior` em alguns pontos. A tarefa pede uma modelagem mais explícita:

- Deve existir uma configuração para cada tipo de ano letivo:
  - `type: "escolar"`
  - `type: "superior"`
- Para cada `type`, deve existir exatamente um `periodo` ativo/configurado.
- O `periodo` usa apenas meses (`1` a `12`) no formato textual `MM_MM` ou `M_M`, preferencialmente normalizado para dois dígitos (`09_07`) se o projeto aceitar essa mudança.
- O `ano_letivo` continua sendo histórico e evolutivo, por exemplo `2025_2026` → `2026_2027`.
- O sistema deve calcular as datas reais do intervalo letivo usando:
  - ano inicial = primeira parte de `ano_letivo`;
  - ano final = segunda parte de `ano_letivo`;
  - mês inicial = primeira parte de `periodo`;
  - mês final = segunda parte de `periodo`.

Exemplo:

```json
{
  "ano_letivo": "2025_2026",
  "type": "escolar",
  "periodo": "10_07"
}
```

Esse exemplo significa:

- início: outubro de 2025;
- fim: julho de 2026;
- as faltas do tipo `escolar` só podem ter data dentro desse intervalo.

## Objetivos funcionais

### 1. Separar os tipos `escolar` e `superior`

Substituir ou compatibilizar o uso atual de `tipo: "escola"` para a nomenclatura da tarefa:

- Novo valor canônico: `escolar`.
- Valor canônico já esperado: `superior`.
- Decidir se `escola` continuará aceito como alias temporário por compatibilidade. Se aceitar, normalizar internamente para `escolar` e documentar como legado.

Critérios:

- Não podem existir duas configurações de ano letivo com o mesmo `type`.
- O mesmo `type` não deve aparecer duplicado na estrutura persistida/projetada.
- O `type` de uma configuração pode ser editado apenas se a edição não criar duplicidade. Se for mais seguro, bloquear edição de `type` depois da criação e permitir apenas edição de `periodo`.

### 2. Configurar período fixo por tipo somente por admin FPP

Criar/ajustar endpoint(s) administrativos para permitir que apenas admin com role `fpp` defina o período dos tipos letivos.

Sugestão de API:

```http
PUT /admin/sistema/anos-letivos/configuracoes/:type
```

Body:

```json
{
  "periodo": "09_07"
}
```

Resposta:

```json
{
  "message": "configuração de ano letivo atualizada com sucesso",
  "type": "escolar",
  "periodo": "09_07"
}
```

Também criar/listar:

```http
GET /admin/sistema/anos-letivos/configuracoes
GET /anos-letivos/configuracoes
```

O endpoint público/autenticado de leitura pode retornar configurações úteis para academias e telas administrativas, mas a escrita deve ser exclusiva de admin FPP.

### 3. Manter evolução do `ano_letivo` independente do `periodo`

A evolução de `ano_letivo` deve continuar funcionando, mas agora deve preservar as configurações de `type` e `periodo`.

Exemplo desejado:

```json
[
  {
    "ano_letivo": "2025_2026",
    "type": "escolar",
    "periodo": "10_07"
  },
  {
    "ano_letivo": "2026_2027",
    "type": "escolar",
    "periodo": "10_07"
  }
]
```

O `periodo` é uma configuração do tipo, não um dado histórico que deva mudar automaticamente a cada virada de ano.

### 4. Validar datas de faltas dentro do período letivo

Ao registrar ou atualizar faltas, validar a data contra o intervalo real do ano letivo ativo e do tipo correspondente.

Regras:

- Se o tipo for `escolar`, usar a configuração `periodo` do tipo `escolar`.
- Se o tipo for `superior`, usar a configuração `periodo` do tipo `superior`.
- Para `ano_letivo = "2025_2026"` e `periodo = "10_07"`:
  - primeira data permitida: `2025-10-01`;
  - última data permitida: último dia de julho de 2026 (`2026-07-31`).
- Se a data da falta estiver antes do início ou depois do fim, retornar erro `400` com mensagem clara.
- Aplicar a mesma regra nos fluxos batch/async que reutilizam o registro de faltas.

Mensagem de erro sugerida:

```text
data da falta fora do período letivo escolar 2025_2026: permitido de 2025-10-01 até 2026-07-31
```

### 5. Determinar corretamente o tipo da falta

O backend deve inferir o tipo letivo da falta de forma confiável, evitando confiar em campo enviado pelo cliente quando for possível derivar do contexto.

Sugestões de inferência:

- Estudante/matéria de ensino fundamental ou médio → `escolar`.
- Estudante/matéria/curso superior → `superior`.
- Se a inferência for ambígua, retornar erro e exigir correção dos dados relacionados, em vez de aceitar uma data sem validação.

A validação deve ser feita no backend, não apenas na API/documentação.

## Áreas prováveis de alteração

Use esta lista como guia inicial; confirme no código antes de editar.

- Handlers de admin/ano letivo:
  - `internal/handlers/admin_handlers.go`
  - `internal/handlers/academia_handlers.go`
- Aggregate de academia e eventos:
  - `internal/domain/aggregates/academia.go`
- Registro/atualização de faltas:
  - `internal/handlers/faltas_handlers.go`
  - `internal/domain/aggregates/estudante_falta.go`
  - `internal/handlers/batch_handlers.go`
  - `internal/handlers/async_batch_handlers.go`
- Projeções e DTOs de academia/sistema:
  - `internal/projections/academia_projection.go`
  - `internal/projections/faltas_projection.go`
  - arquivos de migração/schema em `migrations/` ou diretórios equivalentes.
- Rotas:
  - procurar onde `DefinirAnoLetivoGlobalSistema`, `DefinirAnoLetivoSeguinte`, `DefinirAnoLetivoAcademia`, `RegistrarFaltas` e `AtualizarFalta` são registrados.
- Documentação:
  - `docs/Spuri - API.md`

## Modelo de dados sugerido

### Opção A — tabela/projeção de configuração global

Adicionar uma estrutura em `projection_sistema_config` ou nova tabela/projeção, por exemplo:

```json
{
  "anos_letivos_configuracoes": [
    {
      "type": "escolar",
      "periodo": "09_07",
      "updated_at": "2026-06-21T20:10:00Z",
      "updated_by": "uuid-admin-fpp"
    },
    {
      "type": "superior",
      "periodo": "02_12",
      "updated_at": "2026-06-21T20:10:00Z",
      "updated_by": "uuid-admin-fpp"
    }
  ]
}
```

### Opção B — campos explícitos por tipo

```sql
periodo_ano_letivo_escolar varchar(5)
periodo_ano_letivo_superior varchar(5)
```

A opção A é mais extensível, mas a opção B é mais simples. Preferir a opção que melhor encaixe no padrão atual do repositório.

## Validações obrigatórias

### Validação de `ano_letivo`

- Formato obrigatório: `YYYY_YYYY`.
- O segundo ano deve ser o primeiro + 1.
- Mensagem clara em caso de erro.

### Validação de `type`

- Valores canônicos: `escolar`, `superior`.
- Compatibilidade opcional: aceitar `escola` como alias de `escolar` somente na camada de entrada.
- Persistir sempre normalizado.

### Validação de `periodo`

- Formato: dois meses separados por `_`.
- Meses inteiros entre `1` e `12`.
- Não aceitar valores vazios, texto, `0`, `13`, negativos ou três partes.
- Normalizar para `MM_MM` se possível.
- Permitir períodos que atravessam o ano civil, como `09_07`.
- Para esta tarefa, o mês inicial pertence sempre ao ano inicial e o mês final pertence sempre ao ano final do `ano_letivo`.

### Validação de faltas

- Registro de falta (`POST /academia/faltas-aluno`) deve bloquear data fora do intervalo.
- Atualização de falta (`PUT /academia/atualizar-falta`) deve bloquear a nova data fora do intervalo.
- Batch e async devem herdar a validação e reportar erro por item.
- A validação deve considerar a data final efetiva em atualização: se `data` não for enviada, usar a data atual da falta; se matéria/estudante mudarem algum tipo inferido, validar com o novo contexto efetivo.

## Regras de autorização e segurança

- Apenas admin FPP pode criar/alterar configuração de período por tipo.
- Academias não podem alterar o `periodo` global.
- Academias continuam podendo definir/avançar seu ano letivo conforme a regra existente, mas usando tipos normalizados.
- O cliente não deve conseguir burlar a validação de tipo informando manualmente `type` no payload de falta.
- Projeções e eventos devem continuar auditáveis/self-contained, seguindo o estilo event sourcing atual do projeto.

## Estratégia de implementação sugerida

1. Mapear onde o ano letivo global e o ano letivo da academia são persistidos/projetados.
2. Criar helpers puros para:
   - normalizar `type`;
   - validar/normalizar `periodo`;
   - calcular intervalo `[inicio, fim]` a partir de `ano_letivo` + `periodo`;
   - validar uma data dentro do intervalo.
3. Adicionar testes unitários desses helpers antes de integrar nos handlers.
4. Adicionar persistência/projeção da configuração por tipo.
5. Criar endpoints administrativos de escrita e leitura.
6. Ajustar `DefinirAnoLetivoAcademia` e `DefinirAnoLetivoSeguinte` para usar `escolar`/`superior` e preservar compatibilidade com `escola` se necessário.
7. Integrar a validação nos handlers de faltas e nos fluxos batch/async.
8. Atualizar documentação da API.
9. Rodar testes existentes e adicionar testes específicos da tarefa.

## Cenários de teste mínimos

### Helpers de período

- `periodo = "10_07"`, `ano_letivo = "2025_2026"` → início `2025-10-01`, fim `2026-07-31`.
- `periodo = "1_12"` deve normalizar para `01_12` ou ser aceito consistentemente.
- `periodo = "00_07"` deve falhar.
- `periodo = "09_13"` deve falhar.
- `ano_letivo = "2025_2027"` deve falhar.

### Configuração de tipo

- Admin FPP configura `escolar` com `09_07` com sucesso.
- Admin não FPP recebe `403`.
- Academia recebe `403` ao tentar configurar período.
- Criar configuração duplicada para o mesmo `type` deve atualizar a existente ou retornar erro controlado, conforme decisão de implementação; nunca duplicar.

### Faltas

Com `ano_letivo = "2025_2026"` e `periodo escolar = "10_07"`:

- `2025-10-01` deve ser aceito.
- `2026-07-31` deve ser aceito.
- `2025-09-30` deve ser rejeitado.
- `2026-08-01` deve ser rejeitado.
- Atualizar uma falta para data fora do período deve ser rejeitado.
- Batch com um item inválido deve reportar erro nesse item sem mascarar a causa.

## Critérios de aceite

- Existem configurações independentes para `escolar` e `superior`, com no máximo uma configuração por tipo.
- `periodo` é validado, persistido e retornado pela API.
- Apenas admin FPP consegue alterar os períodos.
- O ano letivo continua evoluindo sem recriar/alterar automaticamente o `periodo`.
- Faltas fora do intervalo calculado são bloqueadas em criação, atualização, batch e async.
- A documentação da API reflete os novos campos e endpoints.
- Testes automatizados cobrem helpers, autorização/configuração e validação de faltas.

## Observações importantes

- O termo da tarefa é `type: escolar/superior`; evite manter `tipo: escola` como modelo interno principal. Se a compatibilidade for necessária, trate `escola` como alias legado.
- O mês inicial é sempre do ano inicial do `ano_letivo`; o mês final é sempre do ano final do `ano_letivo`.
- Não confiar em datas ou tipo enviados pelo frontend para aplicar segurança; o backend deve inferir/validar com dados projetados e agregados.
- Preservar o padrão de auditoria/event sourcing existente no projeto.
