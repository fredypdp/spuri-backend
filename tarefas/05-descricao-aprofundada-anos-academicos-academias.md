---
criado: 2026-06-21 20:30
origem: "tarefas/Lista de tarefas.md#5"
status: pronto_para_implementacao
---

# Tarefa 5 — Permitir às academias adicionar/remover anos acadêmicos com validações avançadas

## Prompt recomendado para executar a atualização

Implemente no backend a capacidade de academias gerenciarem seus anos acadêmicos habilitados, permitindo adicionar, editar metadados e remover/desativar anos acadêmicos dentro do próprio escopo. A implementação deve cobrir explicitamente academias de ensino fundamental, ensino médio, ensino superior e academias mistas que oferecem fundamental + médio. Também deve aplicar validações de segurança avançadas para impedir que uma academia altere anos de outra, crie anos incompatíveis com seu nível/tipo, remova ou edite anos com dados dependentes ativos de forma destrutiva, burle regras globais definidas pela plataforma ou cause efeitos retroativos em ledger, histórico de estudantes, histórico acadêmico, turmas, matérias, notas, faltas, eventos e processos antigos. A remoção deve ser lógica/desativação, preservando histórico e bloqueando a operação quando houver estudantes ainda em atividade no ano acadêmico manipulado.

## Contexto do problema

O sistema já possui conceitos de anos acadêmicos vinculados a academias, cursos, matérias, turmas e regras de avaliação. Porém, a gestão desses anos pode estar centralizada ou rígida demais. A tarefa pede que a própria academia consiga ajustar seus anos acadêmicos, desde que o backend garanta segurança e consistência.

Exemplo de configuração:

```json
{
  "academia_id": "uuid-da-academia",
  "anos_academicos": [1, 2, 3, 4, 5, 6, 7, 8, 9],
  "nivel": "escolar",
  "atualizado_por": "uuid-do-usuario"
}
```

Para superior, o modelo pode representar anos, períodos ou semestres conforme o padrão existente:

```json
{
  "academia_id": "uuid-da-academia",
  "anos_academicos": [1, 2, 3, 4],
  "nivel": "superior"
}
```


## Escopo por tipo de academia

A implementação deve tratar os anos acadêmicos conforme o tipo/nível real da academia, sem assumir que todas usam a mesma escala.

### Ensino fundamental

- Representar anos do ensino fundamental conforme o domínio adotado pela plataforma, normalmente `1` a `9`.
- Permitir que a academia habilite apenas anos compatíveis com sua configuração oficial e com regras globais do produto.
- Impedir a adição de anos exclusivos do ensino médio ou superior em academias configuradas somente como fundamental.
- Validar nomes/descrições como `1º ano`, `2º ano` etc. apenas como metadados; a regra de negócio deve depender do código/ano canônico.

### Ensino médio

- Representar séries/anos do ensino médio conforme o domínio adotado, normalmente `10` a `12` quando o sistema usa continuidade do 1º ao 12º ano, ou `1` a `3` quando existe um tipo separado para médio.
- A escolha entre `10-12` e `1-3` deve seguir o padrão já existente no backend e ser documentada no helper de validação.
- Impedir que uma academia configurada somente como médio habilite anos do fundamental, salvo se ela estiver marcada explicitamente como academia mista.

### Ensino fundamental + médio (academia mista)

- Suportar academias que oferecem os dois níveis no mesmo cadastro/configuração.
- Permitir o conjunto combinado de anos do fundamental e do médio, respeitando a representação canônica do domínio. Exemplos possíveis:
  - `1..12`, se o domínio usa sequência contínua;
  - `fundamental: 1..9` e `medio: 1..3`, se o domínio separa o nível pelo campo `type`.
- Exigir que o backend diferencie o nível do ano por campo canônico (`type`, `nivel`, faixa ou configuração global), e não apenas por descrição textual.
- Impedir colisões ambíguas quando fundamental e médio usam números iguais. Por exemplo, se `1` pode significar `1º ano fundamental` ou `1º ano médio`, o registro deve carregar `type`/`nivel` obrigatório e a chave lógica deve considerar essa dimensão.
- Garantir que desativar ou editar um ano do fundamental não afete o ano equivalente do médio, e vice-versa, quando o modelo usa numeração sobreposta.

### Ensino superior

- Representar anos, períodos, semestres ou módulos conforme o padrão já existente para cursos superiores.
- Validar compatibilidade com a duração dos cursos ativos da academia e com as regras globais do produto.
- Não permitir que alterações em anos/períodos superiores invalidem estudantes, matrículas, disciplinas, notas, faltas, histórico curricular, equivalências, pré-requisitos, mensalidades/ledger ou eventos acadêmicos já lançados.
- Quando a duração variar por curso, a validação deve considerar curso, matriz curricular ou configuração equivalente antes de aceitar o ano/período.

## Objetivos funcionais

### 1. Criar endpoints para listar, adicionar, editar metadados e remover anos acadêmicos

Sugestão de API:

```http
GET /academia/anos-academicos
POST /academia/anos-academicos
PATCH /academia/anos-academicos/:ano_academico
DELETE /academia/anos-academicos/:ano_academico
```

Body para adicionar:

```json
{
  "ano_academico": 10,
  "descricao": "10º ano",
  "type": "medio"
}
```

Resposta sugerida:

```json
{
  "message": "ano acadêmico adicionado com sucesso",
  "ano_academico": 10,
  "ativo": true
}
```

Para remoção/desativação:

```http
DELETE /academia/anos-academicos/10
```

Resposta sugerida:

```json
{
  "message": "ano acadêmico desativado com sucesso",
  "ano_academico": 10,
  "ativo": false
}
```

### 2. Usar remoção lógica em vez de exclusão destrutiva

A remoção deve desativar o ano acadêmico para novos cadastros, mantendo dados históricos. Não implementar exclusão física para dados que já participaram de qualquer processo acadêmico, financeiro, de ledger ou de auditoria.

Regras:

- Não apagar eventos históricos.
- Não apagar notas, faltas, turmas, estudantes ou matérias existentes.
- Marcar o ano como inativo/desabilitado na projeção/configuração da academia.
- Impedir novos vínculos ao ano desativado.
- Permitir consulta histórica de dados já existentes.
- Não reprocessar, recalcular, apagar, substituir ou reescrever lançamentos de ledger associados a estudantes, turmas, mensalidades, propinas, cobranças ou pagamentos históricos.
- Não alterar o histórico acadêmico dos estudantes que já passaram pelo ano, ainda que o ano seja desativado para novos cadastros.

### 3. Validar compatibilidade com nível/tipo da academia e modalidade mista

O backend deve validar se o ano acadêmico solicitado faz sentido para a academia.

Sugestões de regras:

- Ensino fundamental/escolar: aceitar apenas anos dentro do intervalo configurado pela plataforma ou pelo domínio escolar.
- Ensino médio: aceitar anos correspondentes ao médio, se o projeto separar médio de fundamental.
- Ensino fundamental + médio: aceitar o conjunto combinado apenas quando a academia estiver configurada como mista, mantendo distinção segura entre níveis quando houver numeração sobreposta.
- Superior: aceitar anos/períodos conforme duração dos cursos ativos da academia.
- Academia não pode criar ano acadêmico incompatível com seu `nivel`, `type`, modalidade, cursos ou matriz curricular.
- Academia não pode alterar valores globais da plataforma para outras academias.

Se as regras exatas variarem, implementar helper configurável e documentar as premissas.

### 4. Bloquear remoção quando houver dependências críticas ativas

Antes de desativar um ano acadêmico, verificar dependências.

Dependências prováveis:

- estudantes atualmente matriculados, ativos, transferidos em processamento ou com qualquer vínculo aberto no ano;
- turmas ativas vinculadas ao ano;
- matérias ativas obrigatórias para o ano;
- cursos ativos que exigem o ano;
- categorias de notas/regras de avaliação vinculadas;
- notas/faltas/sumários do ano letivo atual.

Política recomendada:

- Se houver dependências ativas no ano letivo corrente, bloquear com `409 Conflict` e mensagem clara.
- Se houver qualquer estudante ainda em atividade no ano acadêmico manipulado, bloquear remoção/desativação e qualquer edição que torne o vínculo inválido.
- Se houver processos antigos concluídos ou histórico consolidado, permitir apenas desativação prospectiva, preservando histórico sem reescrita.
- Opcionalmente oferecer parâmetro `force=false` inicialmente, mas não implementar força destrutiva sem necessidade.

Mensagem sugerida:

```text
não é possível desativar o ano acadêmico 10: existem 2 turmas ativas e 35 estudantes vinculados
```

### 5. Registrar alterações em eventos auditáveis

Eventos sugeridos:

- `AnoAcademicoAcademiaAdicionado`;
- `AnoAcademicoAcademiaDesativado`;
- `AnoAcademicoAcademiaReativado`, se necessário.

Payload sugerido:

```json
{
  "event_type": "AnoAcademicoAcademiaAdicionado",
  "academia_id": "uuid-da-academia",
  "ano_academico": 10,
  "descricao": "10º ano",
  "type": "medio",
  "alterado_por": "uuid-do-usuario",
  "alterado_em": "2026-06-21T20:30:00Z"
}
```

## Áreas prováveis de alteração

Use esta lista como guia inicial; confirme no código antes de editar.

- Handlers e rotas de academia:
  - `internal/handlers/academia_handlers.go`
  - arquivos de registro de rotas.
- Aggregate/eventos de academia:
  - `internal/domain/aggregates/academia.go`.
- Projeções:
  - `internal/projections/academia_projection.go`
  - projeções relacionadas a cursos, turmas, matérias, estudantes e categorias de nota.
- Migrações/schema:
  - `migrations/`.
- Documentação:
  - `docs/Spuri - API.md`.

## Modelo de dados sugerido

### Projeção de anos acadêmicos por academia

Se já existir estrutura similar, adaptar sem duplicar. Caso contrário:

```sql
CREATE TABLE projection_academia_anos_academicos (
  academia_id UUID NOT NULL,
  ano_academico INTEGER NOT NULL,
  descricao TEXT,
  type TEXT,
  ativo BOOLEAN NOT NULL DEFAULT TRUE,
  criado_por UUID,
  criado_em TIMESTAMPTZ NOT NULL,
  atualizado_por UUID,
  atualizado_em TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (academia_id, ano_academico)
);
```

Índices recomendados:

```sql
CREATE INDEX idx_academia_anos_academicos_ativos ON projection_academia_anos_academicos (academia_id, ativo);
```

### Compatibilidade com configuração existente

Se a academia já possuir um campo/lista como `anos_academicos`, decidir entre:

- migrar para tabela normalizada;
- manter lista e adicionar metadados de ativo/inativo;
- usar tabela nova apenas como projeção derivada.

Preferir o padrão que melhor se encaixe no event sourcing e nos rebuilds existentes.

## Validações obrigatórias

### Autorização

- A academia só pode alterar os próprios anos acadêmicos.
- `academia_id` deve vir do token/sessão, nunca do payload.
- Admin FPP pode consultar e eventualmente corrigir via endpoint administrativo separado, se já existir padrão para isso.

### Entrada

- `ano_academico` deve ser inteiro positivo.
- Não aceitar zero, negativo, texto ou valor fora do intervalo permitido.
- Não criar duplicado ativo.
- Se existir inativo, permitir reativar ou retornar mensagem orientando endpoint de reativação; escolher comportamento e documentar.
- `type`, quando informado, deve ser compatível com valores aceitos no domínio.

### Dependências

- Bloquear desativação se existirem turmas ativas no ano.
- Bloquear desativação se existirem estudantes ativos/matriculados no ano, matrículas em andamento, transferências pendentes, rematrículas abertas ou qualquer vínculo estudantil não encerrado.
- Bloquear desativação se existirem matérias ativas obrigatórias no ano.
- Bloquear desativação se existirem regras/categorias ativas que tornariam operações inconsistentes.
- Garantir que novos cadastros de estudantes, turmas, matérias, sumários e faltas não usem anos desativados.
- Garantir que edições de metadados do ano acadêmico não quebrem referências históricas nem alterem semântica de registros já persistidos.

### Segurança contra manipulação

- Não permitir que o frontend envie listas completas substituindo tudo sem validação item a item.
- Evitar operações “replace all” que removam anos silenciosamente.
- Registrar usuário, data e motivo/descrição quando disponível.
- Garantir comportamento consistente em rebuild de projeções.
- Não emitir eventos que reescrevam ou removam eventos antigos do ledger, de estudantes ou de processos acadêmicos já concluídos.
- Tratar operações como comandos prospectivos: `add`, `update_metadata`, `disable` e, se necessário, `reactivate`, nunca como substituição destrutiva de estado global.
- Rodar as validações de dependências dentro de transação/lock apropriado para evitar corrida entre desativação e criação de estudante/turma/matrícula.
- Garantir idempotência dos comandos/eventos para evitar duplicidade ou inconsistência em retentativas.

## Estratégia de implementação sugerida

1. Mapear como os anos acadêmicos da academia são armazenados hoje.
2. Mapear todos os pontos que validam `ano_academico` em estudantes, cursos, matérias, turmas, notas, faltas, categorias, avaliações, cobranças/ledger e processos acadêmicos antigos.
3. Criar helpers para validar ano acadêmico por academia, nível, modalidade mista e curso/matriz curricular quando aplicável.
4. Criar evento(s) e migration/projeção para adição/desativação.
5. Implementar endpoint de listagem.
6. Implementar endpoint de adição com validação de duplicidade e compatibilidade.
7. Implementar endpoint de edição segura de metadados com validação de impacto em estudantes ativos e histórico.
8. Implementar endpoint de desativação com checagem de dependências ativas, estudantes ainda em atividade e impactos em ledger/histórico.
9. Integrar a validação de “ano acadêmico ativo” nos fluxos de criação/atualização dependentes.
10. Atualizar documentação da API.
11. Adicionar testes de handlers, projection rebuild e dependências.

## Cenários de teste mínimos

### Adição

- Academia adiciona ano acadêmico válido com sucesso.
- Academia tenta adicionar ano duplicado ativo e recebe erro controlado.
- Academia tenta adicionar ano incompatível com seu nível/modalidade e recebe `400`.
- Academia mista adiciona ano de fundamental e ano de médio com sucesso, preservando a distinção entre níveis quando houver numeração sobreposta.
- Academia somente fundamental tenta adicionar ano exclusivo do médio e recebe `400`.
- Academia somente médio tenta adicionar ano exclusivo do fundamental e recebe `400`.
- Academia superior tenta adicionar período incompatível com duração de curso/matriz ativa e recebe `400` ou `409`, conforme a causa.
- Academia tenta enviar `academia_id` de outra academia e o backend ignora/rejeita.
- Usuário não autorizado recebe `403`.

### Desativação

- Desativar ano sem dependências ativas funciona.
- Desativar ano com turmas ativas retorna `409` com detalhes.
- Desativar ano com estudantes ativos, matriculados ou com vínculo/processo aberto retorna `409` com detalhes.
- Desativar ano com matérias/categorias ativas retorna `409`.
- Dados históricos continuam consultáveis após desativação.
- Desativação não altera lançamentos de ledger, notas, faltas, turmas antigas, matrículas encerradas ou histórico acadêmico consolidado.
- Edição de descrição/type/nivel é bloqueada quando tornaria inválidos estudantes ativos ou registros históricos referenciados.

### Integração com outros fluxos

- Criar turma em ano desativado deve falhar.
- Criar estudante/matrícula em ano desativado deve falhar.
- Criar matéria/categoria de nota em ano desativado deve falhar.
- Criar sumário/falta em ano desativado deve falhar quando aplicável.
- Rebuild de projeções preserva o estado ativo/inativo corretamente.

## Critérios de aceite

- Academias conseguem listar, adicionar, editar metadados seguros e desativar seus anos acadêmicos.
- Operações são autorizadas pelo contexto autenticado e não por campos enviados pelo cliente.
- Remoção é lógica e preserva histórico.
- Desativação ou edição incompatível com dependências ativas, estudantes ainda em atividade ou processos abertos é bloqueada com erro claro.
- Fluxos dependentes respeitam apenas anos acadêmicos ativos para novos dados, sem afetar dados antigos já consolidados.
- Eventos/projeções são compatíveis com rebuild.
- Documentação e testes cobrem casos de sucesso, autorização e conflitos.

## Observações importantes

- Esta tarefa é sensível porque anos acadêmicos são usados por várias entidades e por históricos financeiros/acadêmicos; evite mudanças destrutivas.
- Prefira comandos pequenos (`add`, `disable`, `reactivate`) a substituição completa da lista.
- Se já houver migrações recentes sobre anos acadêmicos, reutilize o padrão existente.
- Considere dependência futura com sumários/aulas da Tarefa 4.
