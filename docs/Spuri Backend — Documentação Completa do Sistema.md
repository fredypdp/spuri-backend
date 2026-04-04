# Spuri Backend — Guia Rápido para Humanos e IA

Este documento explica **o que existe**, **como funciona** e **como integrar** com o Spuri Backend.
Objetivo: permitir reconstrução/refatoração rápida do sistema e integração clara com o frontend.

---

## 1) TL;DR (visão de 60 segundos)

- API REST em Go com **Event Sourcing + CQRS**.
- Toda escrita passa pelo ledger imutável `spuri_ledger`.
- Leitura sempre vem de projeções (`projection_*`).
- Endpoints `/batch` processam item por item usando handlers normais (sem lógica paralela “secreta”).
- `POST /login` é unificado para admin, academia e estudante.
- Frontend deve tratar sucesso parcial em batch (`207 Multi-Status`).

---

## 2) Mapa mental da arquitetura

### Escrita (comando)

```text
HTTP -> Handler -> Aggregate (valida + gera evento)
     -> Repository.SaveWithAudit -> spuri_ledger
     -> Projection Handler -> projection_*
```

### Leitura (consulta)

```text
HTTP -> Handler -> projection_* (query SQL) -> resposta
```

### Regra de ouro

**Nunca mutar projeção direto** para representar regra de negócio.
A fonte da verdade é sempre o evento no ledger.

---

## 3) Estrutura de pastas (para onboarding rápido)

- `cmd/server/`: boot da API, registro de rotas e jobs.
- `internal/domain/aggregates/`: regras de negócio, comandos, `RaiseEvent`, `Apply`.
- `internal/db/`: event store, save/load de aggregates, segurança de eventos.
- `internal/projections/`: materialização para leitura (`projection_*`).
- `internal/handlers/`: endpoints HTTP, parsing/validação de input, resposta JSON.
- `internal/middleware/`: autenticação/autorização, status ativo, role mínima.
- `migrations/`: schema, funções SQL, índices e constraints.

---

## 4) Entidades principais e responsabilidades

- **Admin**: gestão administrativa, ativa/desativa academias, gerencia roles.
- **Academia**: instituição que cria cursos, matérias, turmas e registra vida escolar.
- **Estudante**: dados pessoais/acadêmicos, notas, faltas, avaliações finais.
- **Curso / Matéria / Turma**: estrutura acadêmica da academia.
- **TelefoneExtra**: telefone adicional com verificação e restrição de unicidade.

### Regras essenciais que quebram integração se ignoradas

- Academia nasce `inativo`; só admin ativa.
- Ano letivo por academia deve ser definido antes de registrar nota/falta/avaliação.
- Formatos canônicos importam (ex.: `1_ano_fundamental`, `1_ano_medio`, `YYYY_YYYY`).
- Códigos (estudante/academia) e idempotência são protegidos por regras do domínio + banco.

---

## 5) Ledger e replay (ponto crítico para refatoração)

`spuri_ledger` é imutável (sem UPDATE/DELETE). Cada evento possui:
- `aggregate_id`, `aggregate_type`, `event_type`, `event_version`
- `payload`, `metadata` (auditoria)
- hash atual e hash anterior (cadeia de integridade)

### Impacto prático

- Se adicionar novo evento, precisa:
  1. emitir no aggregate,
  2. aplicar no `Apply`,
  3. aceitar no whitelist de eventos,
  4. projetar no read model.

Se faltar qualquer etapa, rebuild/load quebra ou dado “some” na leitura.

---

## 6) Batch: contrato oficial para frontend

### Comportamento

- Cada item do array vira uma execução do handler normal equivalente.
- Não é transação global do lote.
- Pode haver sucesso parcial.

### Envelope de resposta (padrão)

```json
{
  "total": 3,
  "sucesso": 2,
  "falhas": 1,
  "items": [
    { "index": 0, "sucesso": true, "dados": {} },
    { "index": 1, "sucesso": false, "erro": "mensagem" }
  ]
}
```

### HTTP status esperado

- `200`: todos OK
- `207`: parcial
- `422`: todos falharam
- `400`: body inválido/array vazio/limite excedido

### Regra frontend

Sempre analisar `items[]` (não confiar apenas no status HTTP).

---

## 7) Guia de integração frontend (checklist)

## 7.1 Autenticação

1. Fazer `POST /login` com credenciais.
2. Guardar JWT.
3. Enviar `Authorization: Bearer <token>` nas rotas protegidas.

## 7.2 Fluxo recomendado para academia recém-criada

1. Admin cria academia (`/dominis/academia/register` ou `/batch`).
2. Admin ativa academia (`/dominis/academia/:codigo/ativar` ou `/batch`).
3. Academia faz login.
4. Academia define ano letivo (`POST /academia/ano-letivo`).
5. Só então registrar estudantes/notas/faltas/avaliações.

## 7.3 Tratamento de erro

- Exibir erro por item em batch.
- Em `422`, oferecer reenvio apenas dos itens falhos.
- Em `401/403`, redirecionar para login/permissão.
- Em `400`, validar payload local antes de reenviar.

## 7.4 Idempotência e deduplicação

- Evitar reenvio cego de lote completo após timeout.
- Reenviar só falhos quando houver `items[]`.
- Manter chave local (ex.: hash do item) para evitar clique duplo.

---

## 8) Rotas mais usadas (resumo)

### Admin
- `POST /dominis/academia/register`
- `POST /dominis/academia/register/batch`
- `PUT /dominis/academia/:codigo/ativar`
- `PUT /dominis/academia/ativar/batch`

### Academia
- `POST /academia/ano-letivo`
- `POST /academia/estudante/register` (+ `/batch`)
- `POST /academia/notas-aluno` (+ `/batch`)
- `POST /academia/faltas-aluno` (+ `/batch`)
- `POST /academia/avaliacao-final` (+ `/batch`)

### Consulta compartilhada
- `GET /academias`
- `GET /estudantes`
- `GET /notas-estudante/:codigo`
- `GET /faltas-estudante/:codigo`

---

## 9) Estratégia de refatoração segura (para IA/humano)

Quando mudar comportamento de domínio:

1. Ajustar aggregate (comando + evento + apply).
2. Ajustar whitelist de eventos no DB layer.
3. Ajustar projection handler correspondente.
4. Validar endpoint HTTP e contrato JSON.
5. Validar batch equivalente.
6. Rodar testes e simular rebuild da projeção afetada.

### Anti-padrões proibidos

- Adicionar regra de negócio em projeção.
- Alterar evento histórico em vez de criar novo evento.
- “Corrigir dado” com UPDATE manual em projeção como solução definitiva.

---

## 10) Exemplo de cliente batch resiliente (pseudo)

```ts
const res = await api.post('/academia/notas-aluno/batch', items)
if (res.status === 200) return done
if (res.status === 207 || res.status === 422) {
  const failed = res.data.items.filter(i => !i.sucesso)
  log(failed)
  retryOnly(failed.map(f => items[f.index]))
}
```

---

## 11) Glossário curto

- **Aggregate**: objeto de domínio que aplica regras e emite eventos.
- **Event Sourcing**: persistir mudanças como eventos imutáveis.
- **CQRS**: separar escrita (comando) e leitura (query/projeção).
- **Projection**: visão de leitura materializada a partir de eventos.
- **Rebuild**: reconstrução de projeções via replay do ledger.

---

## 12) Resultado esperado com este guia

Com este documento, time de produto/frontend/engenharia consegue:
- entender o ciclo completo de dado (escrita/leitura),
- integrar batch sem inconsistência,
- refatorar com baixo risco de quebrar replay,
- acelerar onboarding de humano e IA no código.