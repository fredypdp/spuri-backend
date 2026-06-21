---
criado: 2026-06-21 20:30
origem: "tarefas/Lista de tarefas.md#4"
status: pronto_para_implementacao
---

# Tarefa 4 — Adicionar sumários/aulas e vincular opcionalmente às faltas

## Prompt recomendado para executar a atualização

Implemente no backend o cadastro de sumários/aulas por academia, com validações de escopo e segurança. Um sumário deve representar uma aula/conteúdo ministrado e conter dados como título, período avaliativo (`trimestre` ou `semestre`), ano acadêmico, nível (`escolar`/`superior`), tipo acadêmico (`medio`/`superior`, se aplicável), curso e semestre quando necessários. Em seguida, permita que registros de faltas referenciem opcionalmente um sumário por `sumario_id` e gravem também `sumario_titulo` como snapshot para leitura histórica. O backend deve preencher/inferir campos sensíveis a partir do contexto da academia e validar que curso, ano acadêmico, matéria, turma e semestre pertencem ao escopo correto da academia requisitante.

## Contexto do problema

As faltas atualmente registram presença/ausência, mas não carregam uma referência estruturada à aula ou conteúdo correspondente. A criação de sumários/aulas permite:

- vincular faltas a uma aula específica;
- contar sumários por trimestre ou semestre;
- gerar estatísticas futuras por ano acadêmico, curso, matéria, turma e professor;
- manter histórico do título da aula mesmo se o sumário for editado depois.

Exemplo de sumário desejado:

```json
{
  "sumario_titulo": "Introdução às equações do 2º grau",
  "periodo": "trimestre",
  "ano_academico": 9,
  "nivel": "escolar",
  "type": "medio",
  "curso_id": "uuid-do-curso",
  "semestre": null
}
```

Exemplo para superior:

```json
{
  "sumario_titulo": "Aula 3 — Normalização de bases de dados",
  "periodo": "semestre",
  "ano_academico": 1,
  "nivel": "superior",
  "type": "superior",
  "curso_id": "uuid-do-curso",
  "semestre": 2
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
  "periodo": "trimestre",
  "ano_letivo": "2025_2026",
  "ano_academico": 9,
  "nivel": "escolar",
  "type": "medio",
  "curso_id": "uuid opcional/conforme nível",
  "materia_id": "uuid opcional/recomendado",
  "turma_id": "uuid opcional/recomendado",
  "semestre": null,
  "data_aula": "2026-03-15",
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
GET /academia/sumarios?ano_letivo=2025_2026&periodo=trimestre&ano_academico=9&curso_id=...&materia_id=...&turma_id=...
```

Resposta resumida:

```json
{
  "items": [
    {
      "id": "uuid-do-sumario",
      "sumario_titulo": "Introdução às equações do 2º grau",
      "periodo": "trimestre",
      "ano_letivo": "2025_2026",
      "ano_academico": 9,
      "nivel": "escolar",
      "type": "medio",
      "curso_id": "uuid-do-curso",
      "semestre": null,
      "data_aula": "2026-03-15"
    }
  ]
}
```

### 3. Preencher e proteger campos de escopo

Campos como `academia_id` e `nivel` não devem ser confiados diretamente do payload.

Regras:

- `academia_id` vem sempre do token/sessão.
- `nivel` deve ser inferido a partir da academia ou do curso/matéria/turma selecionado.
- `type` deve estar coerente com o nível e com o curso/ano acadêmico.
- `curso_id`, se informado, deve pertencer à academia autenticada.
- `materia_id`, se informado, deve pertencer à academia e ser compatível com curso/ano/período.
- `turma_id`, se informado, deve pertencer à academia e ser compatível com curso/ano/período.
- `semestre` só deve ser aceito quando o contexto for superior.
- Para ensino escolar/médio, `semestre` deve ser nulo ou rejeitado.
- Para superior, `periodo` normalmente deve ser `semestre`.
- Para escolar/médio, `periodo` normalmente deve aceitar `trimestre`; se semestres também forem permitidos em algum contexto, documentar claramente.

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
- Não permitir vincular falta a sumário incompatível com estudante, matéria, turma, curso, ano acadêmico, data da falta ou ano letivo.

### 5. Criar contagens trimestrais/semestrais

Adicionar endpoint ou agregação em listagem para contagem de sumários por período.

Sugestão:

```http
GET /academia/sumarios/estatisticas?ano_letivo=2025_2026&ano_academico=9&periodo=trimestre
```

Resposta sugerida:

```json
{
  "ano_letivo": "2025_2026",
  "ano_academico": 9,
  "periodo": "trimestre",
  "total_sumarios": 18,
  "por_materia": [
    {
      "materia_id": "uuid-materia",
      "materia_nome": "Matemática",
      "total_sumarios": 6
    }
  ]
}
```

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
  - procurar agregados de faltas, matérias, turmas e academia em `internal/domain/aggregates/`.
- Migrações:
  - `migrations/`.
- Rotas:
  - arquivos onde endpoints `/academia/*` são registrados.
- Documentação:
  - `docs/Spuri - API.md`.

## Modelo de dados sugerido

### Projeção de sumários

```sql
CREATE TABLE projection_sumarios_aulas (
  id UUID PRIMARY KEY,
  academia_id UUID NOT NULL,
  sumario_titulo TEXT NOT NULL,
  descricao TEXT,
  periodo TEXT NOT NULL,
  ano_letivo TEXT NOT NULL,
  ano_academico INTEGER NOT NULL,
  nivel TEXT NOT NULL,
  type TEXT,
  curso_id UUID,
  materia_id UUID,
  turma_id UUID,
  semestre INTEGER,
  data_aula DATE,
  ativo BOOLEAN NOT NULL DEFAULT TRUE,
  criado_por UUID,
  criado_em TIMESTAMPTZ NOT NULL,
  atualizado_em TIMESTAMPTZ NOT NULL
);
```

Índices recomendados:

```sql
CREATE INDEX idx_sumarios_academia_ano_periodo ON projection_sumarios_aulas (academia_id, ano_letivo, periodo);
CREATE INDEX idx_sumarios_contexto ON projection_sumarios_aulas (academia_id, ano_academico, curso_id, materia_id, turma_id);
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
- `periodo` restrito a valores permitidos (`trimestre`, `semestre`, e outros se o domínio já usar).
- `ano_letivo` válido (`YYYY_YYYY`).
- `ano_academico` deve existir/estar habilitado para a academia.
- `curso_id`, `materia_id` e `turma_id` devem pertencer à academia.
- `semestre` deve ser coerente com nível superior e dentro dos limites do curso.
- `data_aula`, se informada, deve respeitar o ano letivo e período configurado.

### Validação de vínculo com falta

- `sumario_id` deve existir e estar ativo.
- Sumário deve pertencer à mesma academia da falta.
- Sumário deve ser compatível com o estudante e a matéria da falta.
- Sumário deve estar no mesmo ano letivo e período esperado da falta.
- `sumario_titulo` não deve ser aceito do cliente como fonte de verdade.

### Autorização

- Apenas usuários autorizados da academia podem criar/editar/remover sumários.
- Professores, se existirem no domínio, só podem criar sumários para turmas/matérias às quais estejam vinculados.
- Admin FPP pode consultar para suporte, mas não deve alterar em nome da academia salvo regra explícita.

## Estratégia de implementação sugerida

1. Mapear modelos atuais de faltas, matérias, turmas, cursos e anos acadêmicos.
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

- Criar sumário escolar com `periodo = trimestre` e dados válidos.
- Criar sumário superior com `periodo = semestre` e `semestre = 2`.
- Rejeitar sumário com curso de outra academia.
- Rejeitar sumário superior sem semestre quando o curso exigir semestre.
- Rejeitar sumário escolar com semestre preenchido, se a regra do domínio não permitir.
- Atualizar título de sumário sem quebrar faltas já vinculadas.
- Remover/desativar sumário preservando histórico de faltas.

### Faltas com sumário

- Criar falta sem `sumario_id` continua funcionando.
- Criar falta com `sumario_id` válido grava `sumario_id` e snapshot `sumario_titulo`.
- Cliente tenta enviar `sumario_titulo` divergente; backend ignora ou rejeita e usa o título real.
- Rejeitar vínculo com sumário de outra academia.
- Rejeitar vínculo com sumário incompatível com matéria/turma/ano acadêmico.
- Atualizar falta trocando sumário atualiza o snapshot.

### Estatísticas

- Contagem por trimestre retorna total correto.
- Contagem por semestre retorna total correto para superior.
- Filtros por matéria/curso/turma não vazam dados de outra academia.

## Critérios de aceite

- Existe cadastro de sumários/aulas com persistência auditável.
- Sumários possuem validação forte de escopo acadêmico.
- Faltas aceitam `sumario_id` opcional e gravam `sumario_titulo` como snapshot.
- Não há possibilidade de vincular faltas a sumários fora do escopo da academia.
- Existem contagens básicas por período, ano acadêmico e filtros relevantes.
- Documentação e testes cobrem criação, atualização, vínculo com faltas e estatísticas.

## Observações importantes

- Não permitir que o frontend defina campos sensíveis como `academia_id` e `nivel` sem validação/inferência server-side.
- Preferir soft delete para sumários, porque faltas antigas podem depender deles.
- Esta tarefa se beneficia das regras de ano letivo/período da Tarefa 1.
- O snapshot `sumario_titulo` na falta é importante para histórico e auditoria.
