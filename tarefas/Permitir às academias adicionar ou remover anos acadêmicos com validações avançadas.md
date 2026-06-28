---
criado: 2026-06-21 20:30
origem: tarefas/Lista de tarefas.md#5
status: pronto_para_implementacao
modificado: 2026-06-28 03:10
---

# Permitir às academias adicionar/remover anos acadêmicos com validações avançadas

## Prompt recomendado para executar a atualização

Implemente no backend a capacidade de academias gerenciarem seus anos acadêmicos habilitados, permitindo adicionar, editar metadados e remover/desativar anos acadêmicos dentro do próprio escopo. A implementação deve cobrir explicitamente academias de ensino fundamental, ensino médio, ensino superior e academias mistas que oferecem fundamental + médio. Também deve aplicar validações de segurança avançadas para impedir que uma academia altere anos de outra, crie anos incompatíveis com seu nível/tipo, remova ou edite anos com dados dependentes ativos de forma destrutiva, burle regras globais definidas pela plataforma ou cause efeitos retroativos em ledger, histórico de estudantes, histórico acadêmico, turmas, matérias, notas, faltas, eventos e processos antigos. A remoção deve ser lógica/desativação, preservando histórico e bloqueando a operação quando houver estudantes ainda em atividade no ano/período manipulado (`status_escolar_fundamental`, `status_escolar_medio` ou `status_superior` igual a `em_andamento`, além de vínculos/processos abertos).

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

Para superior, **não usar o exemplo antigo acima como contrato de entrada**. Depois da tarefa `Curso superior - periodos numericos e anos academicos calculados`, curso superior recebe apenas `periodos` numérico na API de cursos e o backend deriva semestres e anos superiores. Portanto, a gestão segura em academias superiores deve operar sobre a duração/semestres dos cursos, não sobre uma lista manual de anos acadêmicos da academia:

```json
{
  "curso_id": "uuid-do-curso-superior",
  "periodos": 8
}
```

Internamente o backend mantém compatibilidade com:

```json
{
  "periodos": ["1_semestre", "2_semestre", "3_semestre", "4_semestre", "5_semestre", "6_semestre", "7_semestre", "8_semestre"],
  "anos_academicos": ["1_ano_superior", "2_ano_superior", "3_ano_superior", "4_ano_superior"]
}
```


## Estado atual observado no código

A implementação deve partir destes contratos já presentes no backend, para evitar reintroduzir regras antigas:

- Academia escolar `fundamental` ou `misto` possui `anos_academicos` no cadastro da própria academia, validados no formato canônico `[1-9]_ano_fundamental`; academia escolar `medio` não deve definir `anos_academicos` na academia; academia `superior` também não usa `anos_academicos` na academia.
- Cursos só podem ser criados por academias de nível superior ou por escolas de nível escolar `medio`; escolas `fundamental`/`misto` não criam curso pelo fluxo atual de `/academia/cursos`.
- Curso médio mantém `anos_academicos` manual no formato `[n]_ano_medio`, validado pelo aggregate de curso.
- Curso superior não aceita `anos_academicos` no payload; recebe `periodos` como inteiro positivo, deriva `periodos` internos sequenciais (`1_semestre` até `N_semestre`) e deriva `anos_academicos` como `1_ano_superior` até `ceil(N/2)_ano_superior`.
- Na edição de curso, o backend já bloqueia remoção de anos/períodos usados por estudantes ativos: médio por `ano_escolar_medio` e superior por `semestre_atual`. A nova gestão deve reutilizar/ampliar essa proteção em vez de criar regra paralela menos precisa.
- No superior, `semestre_atual` é a fonte operacional de progressão e `ano_superior` é compatibilidade derivada; validações de desativação/redução devem olhar semestres removidos, não apenas anos superiores.
- Existem configurações de **ano letivo** (`projection_anos_letivos_configuracoes` e `projection_anos_letivos_academia_finalizacoes`) que são conceito diferente de **ano acadêmico/período do curso**. Esta tarefa não deve misturar calendário letivo (`2026_2027`) com ano acadêmico (`1_ano_fundamental`, `1_ano_medio`, `1_ano_superior`) ou semestre (`1_semestre`).

## Decisão de contrato atualizada

Para tornar a implementação segura depois da mudança dos cursos superiores:

1. **Fundamental/misto**: gerenciar anos acadêmicos ofertados pela academia em `projection_academias.anos_academicos`, sempre no formato canônico `[1-9]_ano_fundamental`.
2. **Médio**: gerenciar anos acadêmicos no escopo do curso médio, não como lista global da academia, porque a academia de nível médio atualmente não define `anos_academicos` na própria academia.
3. **Superior**: gerenciar semestres pelo campo `periodos` numérico do curso superior; `anos_academicos` superiores continuam derivados e não editáveis manualmente.
4. **Misto**: continua sendo escola com fundamental + médio; apenas o fundamental fica na academia. O médio deve continuar dependente de cursos médios e seus anos, para evitar colisão entre `1_ano_fundamental` e `1_ano_medio`.
5. A API nova pode expor uma visão unificada de “escopos acadêmicos habilitados”, mas internamente deve encaminhar comandos para o dono correto do dado: academia para fundamental, curso para médio/superior.

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

- Representar a gestão como alteração controlada da **quantidade de semestres (`periodos`) do curso superior**.
- Não aceitar adição/remoção manual de `anos_academicos` superiores em endpoint de academia; anos superiores devem ser sempre derivados por `ceil(periodos / 2)`.
- Validar compatibilidade com a duração dos cursos ativos da academia e com as regras globais do produto.
- Não permitir que alterações em semestres superiores invalidem estudantes, matrículas, disciplinas, notas, faltas, histórico curricular, equivalências, pré-requisitos, mensalidades/ledger ou eventos acadêmicos já lançados.
- Quando a duração variar por curso, a validação deve considerar `curso_id` obrigatório; não existe um único conjunto global seguro de períodos superiores por academia.
- Em reduções, bloquear se algum estudante ativo estiver com `semestre_atual` em semestre que seria removido; também bloquear se matérias, regras de avaliação final, notas, faltas ou sumários ativos referenciam os semestres removidos.
- Garantir que respostas e projeções preservem `ano_superior` apenas como campo derivado/compatibilidade, nunca como fonte manual de configuração.

## Objetivos funcionais

### 1. Criar endpoints para listar, adicionar, editar metadados e remover anos acadêmicos

Sugestão de API:

```http
GET /academia/anos-academicos
POST /academia/anos-academicos
PATCH /academia/anos-academicos/:ano_academico
DELETE /academia/anos-academicos/:ano_academico
```

Body para adicionar/habilitar fundamental na academia:

```json
{
  "ano_academico": "4_ano_fundamental",
  "descricao": "4º ano",
  "type": "fundamental"
}
```

Para médio/superior, exigir `curso_id` e direcionar para a configuração do curso. Médio altera `anos_academicos`; superior altera `periodos` numérico e deriva anos automaticamente:

```json
{
  "curso_id": "uuid-do-curso-superior",
  "periodos": 8,
  "type": "superior"
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

- `ano_academico` deve usar o formato canônico atual do domínio, não inteiro solto: `[1-9]_ano_fundamental`, `[n]_ano_medio` ou `[n]_ano_superior` apenas quando este último for campo derivado de leitura.
- Para superior, entrada de escrita deve ser `periodos` como número inteiro positivo no curso; rejeitar `anos_academicos` no payload.
- Não aceitar zero, negativo, texto livre, arrays antigos de `periodos` na API superior, valores fora do intervalo permitido ou sufixos incompatíveis com o tipo.
- Não criar duplicado ativo.
- Se existir inativo, permitir reativar ou retornar mensagem orientando endpoint de reativação; escolher comportamento e documentar.
- `type`, quando informado, deve ser compatível com valores aceitos no domínio e com o dono do dado (`fundamental` na academia, `medio`/`superior` no curso).

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
3. Criar/reutilizar helpers para validar ano acadêmico por academia, nível, modalidade mista e curso/matriz curricular quando aplicável; para superior, reutilizar a derivação `periodos -> semestres -> anos` já existente em cursos.
4. Criar evento(s) e migration/projeção para adição/desativação.
5. Implementar endpoint de listagem.
6. Implementar endpoint de adição com validação de duplicidade e compatibilidade.
7. Implementar endpoint de edição segura de metadados com validação de impacto em estudantes ativos e histórico.
8. Implementar endpoint de desativação com checagem de dependências ativas, estudantes ainda em atividade e impactos em ledger/histórico.
9. Integrar a validação de “ano acadêmico ativo” nos fluxos de criação/atualização dependentes.
10. Atualizar documentação da API.
11. Adicionar testes de handlers, projection rebuild e dependências, incluindo regressões que garantam que curso superior continua rejeitando `anos_academicos` manual e `periodos` array.

## Cenários de teste mínimos

### Adição

- Academia adiciona ano acadêmico válido com sucesso.
- Academia tenta adicionar ano duplicado ativo e recebe erro controlado.
- Academia tenta adicionar ano incompatível com seu nível/modalidade e recebe `400`.
- Academia mista adiciona ano de fundamental e ano de médio com sucesso, preservando a distinção entre níveis quando houver numeração sobreposta.
- Academia somente fundamental tenta adicionar ano exclusivo do médio e recebe `400`.
- Academia somente médio tenta adicionar ano exclusivo do fundamental e recebe `400`.
- Academia superior tenta enviar `anos_academicos` manual e recebe `400`.
- Academia superior aumenta `periodos` numérico de curso e o backend deriva novos semestres/anos.
- Academia superior tenta reduzir `periodos` removendo semestre usado por estudante ativo em `semestre_atual` e recebe `409`.
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
- Criar sumário/falta em ano/período desativado deve falhar quando aplicável; no superior, a validação deve comparar o período semestral da matéria/regra, não apenas `ano_superior`.
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
- Se já houver migrações recentes sobre anos acadêmicos ou anos letivos, reutilize o padrão existente, mas não confunda ano letivo/calendário com ano acadêmico/período curricular.
- Considere dependência futura com sumários/aulas da Tarefa 4.
