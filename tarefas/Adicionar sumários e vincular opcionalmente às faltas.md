---
criado: 2026-06-21 20:30
origem: tarefas/Lista de tarefas.md#4
status: pronto_para_implementacao
modificado: 2026-06-28 2:02
---

# Adicionar sumários/aulas e vincular opcionalmente às faltas (feito)

## Prompt recomendado para executar a atualização

Implemente no backend o cadastro de sumários/aulas por academia, com validações de escopo e segurança. Um sumário deve representar uma aula/conteúdo ministrado e conter dados como título, período avaliativo (`trimestre` ou `semestre`), ano acadêmico, nível (`escolar`/`superior`), tipo acadêmico (`medio`/`superior`, se aplicável), curso e semestre quando necessários. Em seguida, permita que registros de faltas referenciem opcionalmente um sumário por `sumario_id` e gravem também `sumario_titulo` como snapshot para leitura histórica. O backend deve preencher/inferir campos sensíveis a partir do contexto da academia e validar que curso, ano acadêmico, matéria e semestre pertencem ao escopo correto da academia requisitante.

## Contexto do problema

As faltas atualmente registram presença/ausência, mas não carregam uma referência estruturada à aula ou conteúdo correspondente. A criação de sumários/aulas permite:

- vincular faltas a uma aula específica;
- contar sumários por trimestre ou semestre;
- gerar estatísticas futuras por ano acadêmico, curso, matéria e professor;
- manter histórico do título da aula mesmo se o sumário for editado depois.

Exemplo de sumário desejado:

```json
{
  "sumario_titulo": "Introdução às equações do 2º grau",
  "periodo": "[1-3]_trimestre/[N]_semestre",
  "ano_academico": 9,
  "nivel": "escolar",
  "type": "medio",
  "curso_id": "uuid-do-curso",
}
```

Exemplo para superior:

```json
{
  "sumario_titulo": "Aula 3 — Normalização de bases de dados",
  "periodo": "[1-3]_trimestre/[N]_semestre",
  "ano_academico": 1,
  "nivel": "superior",
  "type": "superior",
  "curso_id": "uuid-do-curso",
}
```

## Objetivos funcionais

### 1. Criar entidade/evento de sumário/aula

Adicionar suporte a CRUD ou comandos event-sourced para sumários.

Campos recomendados:

```json
{
  "id": "uuid-do-sumario",
  "academia_id": "uuid-da-academia",
  "sumario_titulo": "texto obrigatório",
  "descricao": "texto opcional mais detalhado",
  "periodo": "1_trimestre",
  "ano_academico": 9,
  "nivel": "escolar",
  "type": "medio",
  "curso_id": "uuid",
  "materia_id": "uuid",
  "criado_por": "uuid-do-usuario",
  "criado_em": "2026-06-21T20:30:00Z",
  "atualizado_em": "2026-06-21T20:30:00Z"
}
```

Eventos sugeridos:

- `SumarioAulaCriado`;
- `SumarioAulaAtualizado`;
- `SumarioAulaRemovido` ou `SumarioAulaDesativado`.

Preferir remoção lógica/soft delete para preservar vínculos históricos com faltas.

### 2. Criar endpoints de cadastro e consulta

Sugestão de API:

```http
POST /academia/sumarios
GET /academia/sumarios
GET /academia/sumarios/:id
PUT /academia/sumarios/:id
DELETE /academia/sumarios/:id
```

Filtros recomendados em listagem:

```http
GET /academia/sumarios?periodo=trimestre&ano_academico=9&curso_id=...&materia_id=...
```

Resposta resumida:

```json
{
  "items": [
    {
      "id": "uuid-do-sumario",
      "sumario_titulo": "Introdução às equações do 2º grau",
      "periodo": "1_trimestre",
      "ano_academico": 9,
      "nivel": "escolar",
      "type": "medio",
      "curso_id": "uuid-do-curso",
    }
  ]
}
```

### 3. Preencher e proteger campos de escopo

Campos como `academia_id` e `nivel` não devem ser confiados diretamente do payload.

Regras:

- `academia_id` vem sempre do token/sessão.
- `nivel` deve ser inferido a partir da academia ou do curso/matéria selecionado.
- `type` deve estar coerente com o nível e com o curso/ano acadêmico.
- `curso_id`, obrigatório para médio e superior, se informado, deve pertencer à academia autenticada.
- `materia_id`, obrigatório, deve pertencer à academia e ser compatível com curso/ano/período.
- `periodo` quando o contexto for superior deve ser coerente com o período da matéria.
- Para ensino escolar/médio, `semestre` deve ser nulo ou rejeitado.
- Para superior, `periodo` normalmente deve ser `semestre`.
- Para escolar/médio, `periodo` normalmente deve aceitar `trimestre`.

### 4. Vincular faltas a sumários de forma opcional

Adicionar campos opcionais aos fluxos de criação/atualização de faltas:

```json
{
  "sumario_id": "uuid-do-sumario",
  "sumario_titulo": "snapshot do título no momento do vínculo"
}
```

Regras:

- O cliente deve enviar apenas `sumario_id`; o backend deve buscar o título e preencher `sumario_titulo` automaticamente.
- `sumario_titulo` na falta deve ser snapshot histórico para preservar leitura se o sumário for renomeado depois.
- Se `sumario_id` for removido/omitido, a falta deve continuar válida sem sumário.
- Ao atualizar uma falta e trocar `sumario_id`, atualizar também o snapshot `sumario_titulo`.
- Não permitir vincular falta a sumário de outra academia.
- Não permitir vincular falta a sumário incompatível com estudante, matéria, curso, ano acadêmico.

## Áreas prováveis de alteração

Use esta lista como guia inicial; confirme no código antes de editar.

- Handlers de faltas:
  - `internal/handlers/faltas_handlers.go`
  - `internal/handlers/batch_handlers.go`
  - `internal/handlers/async_batch_handlers.go`
- Projeções de faltas e novas projeções de sumários:
  - `internal/projections/faltas_projection.go`
  - novo arquivo `internal/projections/sumarios_projection.go`, se fizer sentido.
- Aggregates/eventos:
  - procurar agregados de faltas, matérias e academia em `internal/domain/aggregates/`.
- Migrações:
  - `migrations/`.
- Rotas:
  - arquivos onde endpoints `/academia/*` são registrados.
- Atualizar as documentações:
  - `docs/Spuri - API.md`.
  - `docs/Spuri - Documentação.md`.

## Modelo de dados sugerido

### Projeção de sumários

```sql
CREATE TABLE projection_sumarios_aulas (
  id UUID PRIMARY KEY,
  academia_id UUID NOT NULL,
  sumario_titulo TEXT NOT NULL,
  descricao TEXT,
  periodo TEXT NOT NULL,
  ano_academico INTEGER NOT NULL,
  nivel TEXT NOT NULL,
  type TEXT,
  curso_id UUID,
  materia_id UUID,
  criado_por UUID,
  criado_em TIMESTAMPTZ NOT NULL,
  atualizado_em TIMESTAMPTZ NOT NULL
);
```

Índices recomendados:

```sql
CREATE INDEX idx_sumarios_academia_ano_periodo ON projection_sumarios_aulas (academia_id, periodo);
CREATE INDEX idx_sumarios_contexto ON projection_sumarios_aulas (academia_id, ano_academico, curso_id, materia_id);
```

### Campos em faltas

Adicionar à projeção/evento de faltas:

```sql
ALTER TABLE projection_faltas ADD COLUMN sumario_id UUID NULL;
ALTER TABLE projection_faltas ADD COLUMN sumario_titulo TEXT NULL;
```

Ajuste nomes reais conforme a estrutura existente.

## Validações obrigatórias

### Validação de sumário

- `sumario_titulo` obrigatório, com tamanho mínimo e máximo razoável.
- `periodo` restrito a valores permitidos (`trimestre`, `semestre`).
- `ano_academico` deve existir/estar habilitado para a academia.
- `curso_id` e `materia_id` devem pertencer à academia.

### Validação de vínculo com falta

- `sumario_id` deve existir.
- Sumário deve pertencer à mesma academia da falta.
- Sumário deve ser compatível com o estudante e a matéria da falta.
- `sumario_titulo` não deve ser aceito do cliente como fonte de verdade.

### Autorização

- Admin FPP pode consultar para suporte, mas não deve alterar em nome da academia.

## Estratégia de implementação sugerida

1. Mapear modelos atuais de faltas, matérias, cursos e anos acadêmicos.
2. Criar helpers de validação de contexto acadêmico reutilizáveis.
3. Criar migration e projeção para sumários/aulas.
4. Criar eventos/aggregate ou comandos seguindo o padrão event sourcing do projeto.
5. Implementar endpoints CRUD de sumários.
6. Integrar `sumario_id` opcional nos fluxos de criação/atualização de faltas.
7. Garantir snapshot automático de `sumario_titulo` na falta.
8. Criar endpoint de estatísticas/contagem.
9. Atualizar documentação da API e exemplos.
10. Adicionar testes unitários, de handler, de projection e de batch/async.

## Cenários de teste mínimos

### Sumários

- Criar sumário escolar com `periodo = 1_trimestre` e dados válidos.
- Criar sumário superior com `periodo = 1_semestre`.
- Rejeitar sumário com curso de outra academia.
- Rejeitar sumário superior sem semestre quando o curso exigir semestre, ou com `[N]_semestre` inadequado aos semestres do curso.
- Rejeitar sumário escolar com `[N]_semestre` preenchido.
- Atualizar título de sumário sem quebrar faltas já vinculadas.
- Remover sumário preservando histórico de faltas.

### Faltas com sumário

- Criar falta sem `sumario_id` continua funcionando.
- Criar falta com `sumario_id` válido grava `sumario_id` e snapshot `sumario_titulo`.
- Cliente tenta enviar `sumario_titulo` divergente; backend ignora ou rejeita e usa o título real.
- Rejeitar vínculo com sumário de outra academia.
- Rejeitar vínculo com sumário incompatível com matéria/ano acadêmico.
- Atualizar falta trocando sumário atualiza o snapshot.

## Critérios de aceite

- Existe cadastro de sumários/aulas com persistência auditável.
- Sumários possuem validação forte de escopo acadêmico.
- Faltas aceitam `sumario_id` opcional e gravam `sumario_titulo` como snapshot.
- Não há possibilidade de vincular faltas a sumários fora do escopo da academia.
- Documentação e testes cobrem criação, atualização, vínculo com faltas.
