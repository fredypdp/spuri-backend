---
criado: 2026-06-21 20:30
origem: tarefas/Lista de tarefas.md#2
status: pronto_para_implementacao
modificado: 2026-07-01 20:08
---

# Tarefa 2 — Permitir que academias finalizem o ano letivo e bloquear retrocessos globais (feito)

## Prompt recomendado para executar a atualização

Implemente no backend a possibilidade de cada academia declarar que um determinado `ano_letivo` foi finalizado. Essa finalização deve ser persistida de forma auditável e usada pela plataforma para impedir que o admin FPP defina globalmente um ano letivo anterior ao último ano letivo finalizado por todas as academias aplicáveis. Quando todas as academias tiverem finalizado um ano letivo específico, a plataforma só pode definir como ano letivo global o ano seguinte ou posterior, nunca o mesmo ano finalizado nem qualquer ano anterior.

## Contexto do problema

A plataforma possui configuração global de ano letivo e também regras por academia. Se a plataforma configurar incorretamente um ano letivo antigo, academias que já encerraram o ciclo podem ser afetadas. A nova regra cria uma proteção operacional: a conclusão declarada pelas academias vira um limite mínimo para avanço global.

Exemplo:

```json
{
  "academia_id": "uuid-da-academia",
  "type": "escolar",
  "ano_letivo": "2025_2026",
  "finalizado": true,
  "finalizado_em": "2026-07-31T20:30:00Z"
}
```

Se todas as academias relevantes finalizarem `2025_2026` para `type = escolar`, então a plataforma não pode voltar o ano letivo escolar global para `2025_2026`, `2024_2025` ou qualquer valor anterior. O mínimo permitido passa a ser `2026_2027`.

## Objetivos funcionais

### 1. Criar comando/endpoint para academia finalizar ano letivo

Adicionar endpoint autenticado para academias declararem a finalização do ano letivo atual ou de um ano letivo explicitamente informado.

Sugestão de API:

```http
POST /academia/anos-letivos/finalizar
```

Body sugerido:

```json
{
  "type": "escolar",
  "ano_letivo": "2025_2026",
  "observacao": "Ano letivo encerrado após fechamento de notas e faltas."
}
```

Resposta sugerida:

```json
{
  "message": "ano letivo finalizado com sucesso",
  "academia_id": "uuid-da-academia",
  "type": "escolar",
  "ano_letivo": "2025_2026",
  "finalizado": true
}
```

Regras:

- A academia só pode finalizar anos letivos do próprio escopo.
- O backend deve identificar a academia pelo token/sessão, não por `academia_id` enviado pelo cliente.
- O `type` deve ser normalizado para os valores canônicos do projeto, preferencialmente `escolar` e `superior`.
- O `ano_letivo` deve seguir o formato `YYYY_YYYY`, com o segundo ano igual ao primeiro + 1.
- Se o ano letivo já estiver finalizado, retornar sucesso idempotente ou erro controlado `409`; escolher uma abordagem e documentar.

### 2. Registrar finalização de forma auditável/event-sourced

Criar evento de domínio para registrar a finalização, por exemplo:

```json
{
  "event_type": "AnoLetivoAcademiaFinalizado",
  "academia_id": "uuid-da-academia",
  "type": "escolar",
  "ano_letivo": "2025_2026",
  "finalizado_por": "uuid-do-usuario",
  "finalizado_em": "2026-06-21T20:30:00Z",
  "observacao": "texto opcional"
}
```

O evento deve alimentar projeções que permitam consultas rápidas por:

- academia;
- tipo (`escolar`/`superior`);
- ano letivo;
- status finalizado;
- data de finalização.

### 3. Bloquear retrocesso global quando todas as academias finalizarem

Ao admin FPP definir o ano letivo global ou o ano letivo seguinte, validar contra o maior ano letivo que foi finalizado por todas as academias aplicáveis.

Regra principal:

- Para cada `type`, calcular o último `ano_letivo` que todas as academias ativas e aplicáveis a esse tipo já finalizaram.
- Se existir esse marco, o menor novo ano letivo global permitido deve ser o ano imediatamente seguinte.
- Bloquear qualquer valor igual ou anterior ao marco finalizado por todas.

Exemplo:

```text
Todas as academias escolar finalizaram 2025_2026.
Admin tenta definir global escolar = 2025_2026 → bloquear.
Admin tenta definir global escolar = 2024_2025 → bloquear.
Admin tenta definir global escolar = 2026_2027 → permitir.
```

Mensagem sugerida:

```text
não é possível definir o ano letivo escolar para 2025_2026: todas as academias já finalizaram 2025_2026; o mínimo permitido é 2026_2027
```

### 4. Determinar academias aplicáveis por tipo

A regra “todas as academias” deve considerar somente academias que fazem sentido para o `type` validado.

Sugestão:

- `type = escolar`: academias com nível/tipo escolar, médio ou equivalente.
- `type = superior`: academias com nível superior.
- Academias inativas/desativadas não devem bloquear avanço, salvo se o domínio exigir o contrário.
- Academias criadas depois do ano letivo finalizado não devem necessariamente invalidar um marco passado; definir regra explícita antes de implementar.

Se a modelagem atual não distinguir perfeitamente as academias por tipo, criar helper de elegibilidade e testes para documentar o comportamento.

### 5. Consultar status de finalização

Adicionar endpoint de leitura para academia e/ou admin acompanhar finalizações.

Sugestões:

```http
GET /academia/anos-letivos/finalizacoes
GET /admin/academias/anos-letivos/finalizacoes?type=escolar&ano_letivo=2025_2026
GET /admin/sistema/anos-letivos/finalizacao-limites
```

A resposta administrativa pode incluir:

```json
{
  "type": "escolar",
  "ano_letivo_finalizado_por_todas": "2025_2026",
  "minimo_global_permitido": "2026_2027",
  "academias_total": 12,
  "academias_finalizadas": 12
}
```

## Áreas prováveis de alteração

Use esta lista como guia inicial; confirme no código antes de editar.

- Rotas e handlers de academia/admin:
  - `internal/handlers/academia_handlers.go`
  - `internal/handlers/admin_handlers.go`
  - arquivos onde as rotas de ano letivo são registradas.
- Aggregate/eventos de academia ou sistema:
  - `internal/domain/aggregates/academia.go`
  - diretórios de eventos/event store usados pelo projeto.
- Projeções:
  - `internal/projections/academia_projection.go`
  - nova projeção para finalizações de ano letivo, se necessário.
- Migrações/schema:
  - `migrations/`.
- Documentação:
  - `docs/Spuri - API.md`.

## Modelo de dados sugerido

### Tabela/projeção de finalizações

```sql
CREATE TABLE projection_anos_letivos_academia_finalizacoes (
  academia_id UUID NOT NULL,
  type TEXT NOT NULL,
  ano_letivo TEXT NOT NULL,
  finalizado BOOLEAN NOT NULL DEFAULT TRUE,
  finalizado_por UUID,
  finalizado_em TIMESTAMPTZ NOT NULL,
  observacao TEXT,
  PRIMARY KEY (academia_id, type, ano_letivo)
);
```

Ajuste nomes/colunas ao padrão atual do repositório.

### Helper de comparação de ano letivo

Criar função pura para comparar anos letivos:

- `ParseAnoLetivo("2025_2026")` → `{Inicio: 2025, Fim: 2026}`;
- `ProximoAnoLetivo("2025_2026")` → `"2026_2027"`;
- `CompareAnoLetivo(a, b)` → `-1`, `0`, `1`.

Evite comparação textual simples sem validação prévia.

## Validações obrigatórias

### Autorização

- Somente usuário autenticado da academia pode finalizar o ano letivo da própria academia.
- Admin FPP pode consultar finalizações globais.
- Cliente não pode enviar `academia_id` para finalizar em nome de outra academia.

### Integridade

- Não aceitar `ano_letivo` inválido (`2025`, `2025-2026`, `2025_2027`, texto vazio).
- Não aceitar `type` desconhecido.
- Não duplicar finalização para mesma academia + tipo + ano letivo.
- Não permitir finalizar ano letivo futuro muito distante, salvo se houver justificativa de domínio.
- Opcionalmente bloquear finalização se ainda existirem pendências críticas, como notas/faltas não fechadas; se não implementar agora, deixar ponto de extensão claro.

### Segurança operacional

- O bloqueio de retrocesso deve rodar no backend em todos os fluxos que alteram o ano letivo global, incluindo handlers síncronos, jobs e comandos administrativos existentes.
- A validação deve ser transacional o suficiente para evitar race condition entre finalizações e mudança global.

## Estratégia de implementação sugerida

1. Mapear todos os lugares que definem ano letivo global ou avançam para o ano seguinte.
2. Criar helpers de validação/comparação de `ano_letivo` e normalização de `type`.
3. Criar evento de domínio de finalização e projection/migração correspondente.
4. Implementar handler da academia para finalizar ano letivo.
5. Implementar query que calcula o maior ano letivo finalizado por todas as academias aplicáveis.
6. Integrar a validação nos comandos administrativos de definição de ano letivo global.
7. Criar endpoints de consulta para admin e academia.
8. Atualizar documentação da API.
9. Adicionar testes unitários, testes de handler e testes de projeção.

## Cenários de teste mínimos

### Finalização por academia

- Academia finaliza `2025_2026` com sucesso.
- Repetir a mesma finalização é idempotente ou retorna erro controlado documentado.
- Academia A não consegue finalizar em nome da academia B.
- `type` inválido retorna `400`.
- `ano_letivo = "2025_2027"` retorna `400`.

### Cálculo de marco global

- Com 3 academias ativas, se apenas 2 finalizarem `2025_2026`, não há bloqueio por “todas”.
- Quando as 3 finalizarem `2025_2026`, o mínimo permitido vira `2026_2027`.
- Se todas finalizarem `2026_2027`, o mínimo permitido vira `2027_2028`.
- Academias inativas não entram na contagem, conforme regra definida.

### Bloqueio administrativo

- Admin FPP tenta definir global para ano anterior ao marco finalizado → `400` ou `409`.
- Admin FPP tenta definir global para o mesmo ano já finalizado por todas → `400` ou `409`.
- Admin FPP define global para o próximo ano → sucesso.
- O mesmo bloqueio funciona em endpoint de “ano letivo seguinte”.

## Critérios de aceite

- Academias conseguem declarar finalização de ano letivo por tipo.
- A finalização é persistida e reconstruída corretamente via projeção/event sourcing.
- O sistema calcula o maior ano letivo finalizado por todas as academias aplicáveis.
- Alterações globais de ano letivo respeitam o mínimo permitido.
- Existem endpoints/documentação para finalizar e consultar status.
- Testes cobrem autorização, validação, cálculo do marco e bloqueio de retrocesso.

## Observações importantes

- Esta tarefa depende conceitualmente da normalização de `type` entre `escolar` e `superior` descrita na Tarefa 1.
- Não confie em `academia_id` enviado no payload para ações de academia.
- Mantenha a implementação compatível com event sourcing e rebuilds de projeção.
- Se existirem academias híbridas, documente como elas contam para cada `type`.
