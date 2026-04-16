---
modificado: 16-04-2026 16:00
criado: 05-04-2026 13:01
---
Versão atual: 1.1.4
## Índice

1. [[#1. Visão Geral]]
2. [[#2. Arquitetura]]
3. [[#3. Fluxo de Dados]]
4. [[#4. Entidades do Sistema]]
5. [[#5. Processos de Negócio]]
6. [[#6. Regras de Negócio]]
7. [[#7. Sistema de Permissões]]
8. [[#8. Segurança e Autenticação]]
9. [[#9. Operações em Lote]]
10. [[#10. Recomendações de Melhoria]]

---

## 1. Visão Geral

O Spuri é um sistema de gestão académica para Angola. Permite que **instituições de ensino** (escolas e universidades) gerenciem estudantes, notas, faltas, avaliações finais, turmas e matérias.

O sistema suporta três tipos de ensino:

| Tipo            | Ciclo                    |
| --------------- | ------------------------ |
| **Fundamental** | 1.º ao 9.º ano           |
| **Médio**       | 1.º ao 3.º ano (ou mais) |
| **Superior**    | 1.º ao N.º semestre      |

### Atores do Sistema

| Ator              | Papel                                                              |
| ----------------- | ------------------------------------------------------------------ |
| **Admin FPP**     | Administrador máximo. Cria academias, outros admins, faz rebuilds. |
| **Admin ADM**     | Ativa/desativa academias e admins de nível inferior.               |
| **Admin Gerente** | Consultas e ações básicas administrativas.                         |
| **Academia**      | Gere estudantes, notas, faltas, cursos, matérias e turmas.         |
| **Estudante**     | Visualiza os próprios dados académicos.                            |

---

## 2. Arquitetura

### 2.1 Padrão Event Sourcing + CQRS

O sistema implementa **Event Sourcing com CQRS (Command Query Responsibility Segregation)**. Isto significa:

- **Toda mutação de estado** é registada primeiro no ledger (`spuri_ledger`) como um evento imutável.
- As **projeções de leitura** são tabelas derivadas, reconstruídas a partir dos eventos.
- Nenhum `UPDATE` ou `DELETE` direto acontece nas tabelas de projeção sem passar pelo ledger.

#### Por que Event Sourcing?

- **Auditoria completa**: cada ação fica registada permanentemente com quem fez, quando e o que mudou.
- **Reconstrução**: qualquer projeção pode ser recalculada do zero replaying os eventos.
- **Integridade**: uma hash chain garante que nenhum evento foi alterado ou removido.

### 2.2 Componentes Principais

```
┌─────────────────────────────────────────────────────┐
│                   HTTP (Gin Router)                  │
│              Middleware: Auth                        │
└─────────────────────┬───────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────┐
│                    Handlers                          │
│         (validação, orquestração, resposta)          │
└──────────┬──────────────────────┬───────────────────┘
           │                      │
┌──────────▼──────┐    ┌──────────▼──────────────────┐
│   Aggregates    │    │       Projections             │
│  (lógica de     │    │   (leitura rápida do         │
│   negócio)      │    │    banco, read models)       │
└──────────┬──────┘    └─────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────────┐
│             Repository / Event Store                 │
│        (SaveWithAudit → spuri_ledger)                │
└──────────┬──────────────────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────────┐
│               spuri_ledger (imutável)                │
│           hash chain → integridade garantida         │
└──────────┬──────────────────────────────────────────┘
           │
┌──────────▼──────────────────────────────────────────┐
│          Projection Manager (background)             │
│     Escuta novos eventos → atualiza projeções        │
└─────────────────────────────────────────────────────┘
```

### 2.3 O Ledger (`spuri_ledger`)

O ledger é a **fonte de verdade absoluta** do sistema. Característica fundamental: é **imutável**. Triggers no PostgreSQL bloqueiam qualquer `UPDATE`, `DELETE` ou `TRUNCATE` na tabela.

Cada evento no ledger contém:

|Campo|Descrição|
|---|---|
|`id`|Sequência global (BIGSERIAL)|
|`event_id`|UUID único do evento|
|`aggregate_id`|UUID do objeto que sofreu a ação|
|`aggregate_type`|Tipo do objeto (ex: `Estudante`, `Academia`)|
|`event_type`|O que aconteceu (ex: `EstudanteCriadoComVinculo`)|
|`event_version`|Versão do aggregate naquele momento|
|`payload`|Dados completos do evento em JSON|
|`metadata`|Contexto de auditoria: user_id, IP, tipo do usuário|
|`ledger_hash`|Hash SHA256 encadeado com o evento anterior|
|`previous_hash`|Hash do evento anterior (garante a cadeia)|

A hash chain permite verificar a integridade do ledger: se qualquer evento for alterado, toda a cadeia subsequente se torna inválida.

### 2.4 Aggregates

Aggregates são os objetos de domínio que encapsulam as **regras de negócio**. Eles:

1. Recebem um **comando** (ex: `Criar`, `Ativar`, `RegistrarNota`)
2. Validam as regras de negócio
3. **Emitem eventos** (`RaiseEvent`)
4. Aplicam os eventos ao próprio estado (`Apply`)
5. O repositório salva os eventos no ledger

**Regra de ouro**: os aggregates nunca falam com o banco de dados diretamente. Toda comunicação com o mundo externo é via eventos.

### 2.5 Projeções (Read Models)

Projeções são tabelas PostgreSQL otimizadas para leitura. São reconstruídas pelo **Projection Manager** que roda em background, consumindo novos eventos do ledger a cada segundo.

|Projeção|Dados|
|---|---|
|`projection_estudantes`|Dados actuais de todos os estudantes|
|`projection_academias`|Dados actuais de todas as academias|
|`projection_admins`|Dados actuais de todos os admins|
|`projection_cursos`|Cursos por academia|
|`projection_materias`|Matérias por academia|
|`projection_turmas`|Turmas com lista atual de estudantes e histórico por ano letivo|
|`projection_notas`|Notas registadas|
|`projection_faltas`|Faltas registadas|
|`projection_avaliacao_final`|Avaliações finais de ano|
|`projection_categorias_nota`|Categorias personalizadas de nota|
|`projection_telefones_extra`|Telefones extras de qualquer usuário|

Se uma projeção ficar corrompida ou inconsistente, basta executar um **Rebuild** que a reconstrói do zero a partir do ledger.

---

## 3. Fluxo de Dados

### 3.1 Fluxo de Escrita (Comandos)

```
1. HTTP Request chega ao handler
2. Handler valida o input básico
3. Handler carrega o aggregate do ledger (Load)
4. Handler chama o comando no aggregate (ex: estudante.RegistrarNota(...))
5. Aggregate valida as regras de negócio
6. Aggregate emite o evento (RaiseEvent) e aplica ao próprio estado
7. Repository.SaveWithAudit salva os eventos uncommitted no ledger
8. Projection Manager (em background) detecta o novo evento e atualiza a projeção
9. Handler responde ao cliente
```

### 3.2 Fluxo de Leitura (Queries)

```
1. HTTP Request chega ao handler
2. Handler consulta diretamente a projeção (tabela de leitura)
3. Handler responde ao cliente com os dados
```

As leituras são simples queries SQL nas projeções. Não há reconstrução de aggregates para leituras.

### 3.3 Reconstrução de um Aggregate (Load)

Quando um aggregate precisa de ser carregado para executar um comando:

```
1. Busca todos os eventos do aggregate_id no ledger (ORDER BY event_version ASC)
2. Cria uma instância vazia do aggregate (factory)
3. Injeta o ID real no aggregate (SetID)
4. Aplica cada evento em sequência (Apply)
5. O aggregate está no estado atual, pronto para receber um novo comando
```

### 3.4 Projection Manager

O manager roda em background com polling de 1 segundo:

```
A cada 1 segundo:
  Para cada projeção registada:
    1. Busca o último evento processado (checkpoint)
    2. Busca novos eventos no ledger após esse checkpoint
    3. Processa cada evento com até 3 tentativas (backoff exponencial)
    4. Se um evento falhar após 3 tentativas, o checkpoint para nesse evento
       (será retentado no próximo ciclo, 1s depois)
    5. Atualiza o checkpoint com o ID do último evento processado com sucesso
```

**Comportamento especial**: se um evento de estudante chegar antes que a academia correspondente esteja na projeção (race condition), o handler retorna um erro temporário e o manager retenta automaticamente. A academia será projetada primeiro, e na próxima tentativa o estudante será inserido com sucesso.

---

## 4. Entidades do Sistema

### 4.1 Admin

Administradores do sistema com hierarquia de roles.

**Hierarquia (ordem de poder):**

```
fpp (3) > adm (2) > gerente (1)
```

Um admin só pode gerenciar admins de role **estritamente inferior** ao seu.

**Estados possíveis:** `ativo` / `inativo`

**Eventos:**

|Evento|Quando ocorre|
|---|---|
|`AdminCriado`|Novo admin cadastrado|
|`EmailVerificado`|Email confirmado via token|
|`AdminAtivado`|Admin reativado|
|`AdminDesativado`|Admin suspenso|
|`AcaoAdminRegistrada`|Qualquer ação administrativa registada|
|`AdminDadosAtualizados`|Nome ou email alterados|
|`AdminRoleAtualizado`|Role modificado (somente por FPP)|
|`AdminSenhaAlterada`|Senha alterada|

**Observação sobre senhas**: o hash bcrypt da senha é gravado no payload dos eventos `AdminCriado` e `AdminSenhaAlterada`. Isso é necessário para que um Rebuild restaure a senha correta. O hash bcrypt não é a senha em texto plano.

---

### 4.2 Academia

Representa uma instituição de ensino. Pode ser uma **escola** (ensino fundamental/médio) ou uma **universidade/superior**.

**Código único**: gerado automaticamente no formato `{PROVINCIA}{ANO}{SEQUENCIAL}`, consultando o ledger para garantir unicidade mesmo em cadastros simultâneos. Exemplo: `LDA20261`.

**Tipos:**

| Tipo       | Nível Escolar | Características                                                              |
| ---------- | ------------- | ---------------------------------------------------------------------------- |
| `escola`   | `fundamental` | Tem anos_academicos (1º a 9º ano)                                            |
| `escola`   | `medio`       | Sem anos_academicos fixos, eles são definidos nos seus cursos                |
| `escola`   | `misto`       | Tem anos_academicos para fundamental                                         |
| `superior` | —             | Sem nível escolar; tem cursos superiores, eles são definidos nos seus cursos |

**Ano Letivo**: cada academia define o seu próprio ano letivo ativo (ex: `2025_2026`). Sem ano letivo, nenhum registro de nota, falta ou avaliação é permitido.

**Estados possíveis:** `ativo` / `inativo`

**Eventos:**

|Evento|Quando ocorre|
|---|---|
|`AcademiaCriada`|Nova academia registada pelo admin|
|`AcademiaAtivada`|Academia ativada pelo admin|
|`AcademiaDesativada`|Academia desativada (com motivo obrigatório)|
|`AcademiaDadosAtualizados`|Dados cadastrais alterados|
|`CursosAtualizados`|Lista de cursos alterada|
|`EmailVerificado`|Email confirmado|
|`AcademiaSenhaAlterada`|Senha alterada|
|`CategoriaNotaAdicionada`|Categoria de nota personalizada criada|
|`AnoLetivoAcademiaDefinido`|Ano letivo ativo definido ou atualizado|

---

### 4.3 Estudante

Representa um aluno vinculado a uma academia.

**Código único**: gerado no formato `AAA1234` (3 letras maiúsculas + 4 dígitos). Verificado contra o ledger E a projeção para evitar colisões.
**Criação**: apenas pela academia (não existe auto-cadastro). O estudante é criado já vinculado a uma academia.

**Campos obrigatórios no cadastro:**

| Campo                          | Valores                            | Detalhe                                                      |
| ------------------------------ | ---------------------------------- | ------------------------------------------------------------ |
| `nome`                         | Texto                              |                                                              |
| `genero`                       | `masculino` / `feminino`           |                                                              |
| `data_nascimento`              | Data ISO, deve ser anterior a hoje |                                                              |
| bilhete_identidade             | Texto                              | Obrigatório caso bilhete_identidade_responsavel esteja vazio |
| bilhete_identidade_responsavel | Texto                              | Obrigatório caso bilhete_identidade esteja vazio             |

**Progressão escolar**: o estudante tem três trajetórias paralelas e independentes:

```
Status Fundamental: inativo → em_andamento → finalizado
Status Médio:       inativo → em_andamento → finalizado
Status Superior:    inativo → em_andamento → finalizado
```

**Regra de progressão**: o status superior só pode avançar para `em_andamento` ou `finalizado` se o fundamental e o médio estiverem `finalizado` ou `inativo`. O status médio só pode avançar para `em_andamento` ou `finalizado` se o fundamental estiver `finalizado` ou `inativo`.

**Formato dos anos académicos:**

|Ciclo|Formato|Exemplos|
|---|---|---|
|Fundamental|`[1-9]_ano_fundamental`|`1_ano_fundamental`, `9_ano_fundamental`|
|Médio|`[n]_ano_medio`|`1_ano_medio`, `3_ano_medio`|
|Superior|`[n]_ano_superior`|`1_ano_superior`, `5_ano_superior`|

**Senha padrão**: o código do estudante é a senha inicial (ex: estudante `ABC1234` tem senha `ABC1234`).

**Eventos:**

|Evento|Quando ocorre|
|---|---|
|`EstudanteCriadoComVinculo`|Cadastro pela academia|
|`DadosPessoaisAtualizados`|Nome, email, bilhete, data de nascimento alterados|
|`DadosAcademicosAtualizados`|Ano escolar, curso alterados|
|`SenhaAlterada`|Senha alterada|
|`CursoAlterado`|Curso médio ou superior alterado|
|`EmailVerificadoEstudante`|Email confirmado|
|`StatusEscolarFundamentalAtualizado`|Status fundamental alterado|
|`StatusEscolarMedioAtualizado`|Status médio alterado|
|`StatusSuperiorAtualizado`|Status superior alterado|
|`AvaliacaoFinalAnoAcademico`|Avaliação final registada|
|`NotasRegistradas`|Nota registada|
|`NotaAtualizada`|Nota corrigida|
|`NotaDeletada`|Nota removida (soft delete)|
|`FaltasRegistradas`|Faltas registadas|
|`FaltaAtualizada`|Falta corrigida|
|`FaltaDeletada`|Falta removida|

---

### 4.4 Curso

Representa um curso oferecido por uma academia (médio ou superior). O **tipo é imutável após criação**.

**Tipos:**

| Tipo       | Períodos                                        | Anos                       |
| ---------- | ----------------------------------------------- | -------------------------- |
| `medio`    | Trimestres fixos do sistema (não configuráveis) | Formato `[n]_ano_medio`    |
| `superior` | Semestres configurados pela academia            | Formato `[n]_ano_superior` |

**Formato dos semestres**: `[n]_semestre` onde n ≥ 1 (ex: `1_semestre`, `2_semestre`).

Os trimestres (`1_trimestre`, `2_trimestre`, `3_trimestre`) são **fixos do sistema** e nunca configurados no curso. São os períodos padrão para notas do tipo escolar.

**Estados:** `ativo` / `inativo` / `deletado`

Para deletar um curso, ele deve estar **inativo** e sem estudantes matriculados. A deleção em cascata remove matérias inativas e turmas inativas vinculadas.

**Eventos:** `CursoCriado`, `CursoAtivado`, `CursoDesativado`, `CursoDadosAtualizados`, `CursoDeletado`

---

### 4.5 Matéria Disciplinar

Representa uma disciplina vinculada a uma academia e tipo de ensino.

**Tipos:**

|Tipo|Criada com status|Requer|
|---|---|---|
|`fundamental`|`ativo`|Ano(s) no formato `[1-9]_ano_fundamental`|
|`medio`|`ativo`|Exatamente 1 ano no formato `[n]_ano_medio`; curso_id obrigatório|
|`superior`|**`inativo`**|Exatamente 1 ano no formato `[n]_ano_superior`; curso_id obrigatório|

**Matérias superiores nascem inativas**: exigem que um período seja definido antes de poder ser ativadas (`PUT /academia/materia/:id/periodo`).

**Estados:** `ativo` / `inativo` / `deletado`

**Eventos:** `MateriaCriada`, `MateriaAtivada`, `MateriaDesativada`, `MateriaDadosAtualizados`, `MateriaPeriodoDefinido`, `MateriaDeletada`

---

### 4.6 Turma

Agrupa estudantes num contexto de nível, turno e curso.

**Campos obrigatórios**: `codigo_turma`, `nivel`, `turno` (`manha`/`tarde`/`noite`)

O código de turma deve ser **único dentro da academia**.

**Estudantes na turma**: guardados como lista de `CodigoEstudante` (strings). Um estudante pode estar em múltiplas turmas simultaneamente.

**Deleção**: a turma deve estar inativa e sem estudantes vinculados.

**Remoção automática**: quando uma avaliação final é registada, o estudante é automaticamente removido de todas as turmas da academia.

**Estados:** `ativo` / `inativo` / `deletado`

**Eventos:** `TurmaCriada`, `TurmaAtivada`, `TurmaDesativada`, `TurmaDadosAtualizados`, `TurmaDeletada`, `EstudanteAdicionadoATurma`, `EstudanteRemovidoDaTurma`

---

### 4.7 Telefone Extra

Permite que qualquer usuário (estudante, academia ou admin) registe números de telefone adicionais.

**Normalização**: o número é normalizado antes de salvar (remove espaços, hífens, parênteses; mantém `+` inicial).

**Formato aceito após normalização**: `+?[0-9]{7,15}`

**Estados de verificação**:

- Não verificado: qualquer usuário pode cadastrar o mesmo número
- Verificado: apenas um usuário pode ter aquele número verificado (índice único parcial)

**Eventos:** `TelefoneExtraAdicionado`, `TelefoneExtraVerificado`

---

## 5. Processos de Negócio

### 5.1 Cadastro de Academia

**Quem faz**: Admin (FPP)

1. Admin envia dados da academia (tipo, nome, província, endereço, nível escolar, etc.)
2. Sistema gera o código único consultando o ledger (ex: `LDA20261`)
3. Sistema gera a senha padrão = código da academia (ex: `LDA20261`)
4. Academia é criada com **status `inativo`**
5. Um admin com role `adm` ou `fpp` deve ativar manualmente

**Regras de validação:**

- `type` deve ser `escola` ou `superior`
- Para `escola` com nível `fundamental` ou `misto`: `anos_academicos` é obrigatório (formato `[1-9]_ano_fundamental`)
- Para `escola` com nível `medio`: `anos_academicos` não deve ser informado
- Província deve ser um código válido de Angola (21 províncias):
	- `{ nome: 'BENGO', codigo: 'BGO' },
	  { nome: 'BENGUELA', codigo: 'BGU' },
	  { nome: 'BIE', codigo: 'BIE' },
	  { nome: 'CABINDA', codigo: 'CAB' },
	  { nome: 'CUANDO CUBANGO', codigo: 'CND' },
	  { nome: 'CUANZA NORTE', codigo: 'CNO' },
	  { nome: 'CUANZA SUL', codigo: 'CUS' },
	  { nome: 'CUBANGO', codigo: 'CBG' },
	  { nome: 'CUNENE', codigo: 'CNN' },
	  { nome: 'HUAMBO', codigo: 'HUA' },
	  { nome: 'HUILA', codigo: 'HUI' },
	  { nome: 'ICOLO E BENGO',  codigo: 'IBG' },
	  { nome: 'LUANDA', codigo: 'LUA' },
	  { nome: 'LUNDA NORTE', codigo: 'LNO' },
	  { nome: 'LUNDA SUL', codigo: 'LSU' },
	  { nome: 'MALANJE', codigo: 'MAL' },
	  { nome: 'MOXICO', codigo: 'MOX' },
	  { nome: 'MOXICO LESTE', codigo: 'MXL' },
	  { nome: 'NAMIBE', codigo: 'NAM' },
	  { nome: 'UIGE', codigo: 'UIG' },
	  { nome: 'ZAIRE', codigo: 'ZAI' },
	];`

---

### 5.2 Cadastro de Estudante

**Quem faz**: Academia (status ativo)

1. Academia envia os dados do estudante
2. Sistema gera código único (`AAA1234`), verificando ledger e projeção
3. Senha padrão = código do estudante (ex: `ABC1234`)
4. Estudante criado com **status `ativo`** e vinculado à academia
5. Dados académicos (ano escolar, status, curso) são configurados na criação

**Regras de validação:**

- `genero` obrigatório: `masculino` ou `feminino`
- `data_nascimento` obrigatório: deve ser anterior à data atual
- `ano_escolar` deve seguir o formato canônico para o tipo de ensino
- Se informar `curso_medio_id`, o curso deve existir e ser do tipo `medio`
- Se informar `curso_superior_id`, o curso deve existir e ser do tipo `superior`
- Status inicial padrão para fundamental: `em_andamento`
- Status inicial padrão para médio e superior: `em_andamento`

---

### 5.3 Configuração do Ano Letivo

**Quem faz**: Academia (para si própria)

Antes de registar qualquer nota, falta ou avaliação, a academia deve definir o ano letivo ativo.

**Formato obrigatório**: `YYYY_YYYY` onde o segundo ano é exatamente o primeiro + 1 (ex: `2025_2026`)

**Tipo**: `escola` ou `superior`

Pode ser chamado múltiplas vezes — cada chamada substitui o valor anterior. O ano letivo ativo é resolvido automaticamente em todos os novos registos de nota, falta e avaliação

---

### 5.4 Registro de Notas

**Quem faz**: Academia (status ativo, com ano letivo configurado)

**Processo:**

1. Academia envia: código do estudante, período, matéria, tipo, categoria, valor
2. Sistema valida o ano letivo ativo da academia
3. Sistema verifica que estudante pertence à academia
4. Sistema verifica que matéria pertence à academia
5. Sistema infere o `ano_academico`:
    - Se estudante tem `ano_escolar` preenchido (fundamental) → usa esse valor
    - Caso contrário → usa `anos_academicos[0]` da matéria
6. Sistema verifica idempotência (chave: `anoLectivo_periodo_materiaID_tipo_categoria`)
7. Se não for duplicata, emite `NotasRegistradas` no ledger do estudante

**Tipos de nota:**

|Tipo|Academia|Categorias fixas|Períodos|
|---|---|---|---|
|`escolar`|`escola`|`nota_escola`, `nota_professor`|`1_trimestre`, `2_trimestre`, `3_trimestre`|
|`superior`|`superior`|`nota_pp1`, `nota_pp2`, `nota_exame`|Semestres do curso|

Academias podem criar **categorias adicionais** personalizadas, disponíveis para qualquer tipo de nota.

**Valor**: entre 0 ou mais (validado no aggregate)

**Correção de nota**: `observacao` é **obrigatória** (justificativa da correção)

**Deleção de nota**: `motivo` é **obrigatório**; soft delete (permanece no ledger)

**Proteção contra duplicatas**: o aggregate mantém um mapa em memória (`NotasRegistradasPorChave`). Se a mesma combinação de ano/período/matéria/tipo/categoria já existir, o comando é rejeitado com erro de negócio claro antes de tocar o banco.

---

### 5.5 Registro de Faltas

**Quem faz**: Academia (status ativo, com ano letivo configurado)

**Processo:**

1. Academia envia: código do estudante, data, matéria, quantidade
2. Sistema valida ano letivo ativo
3. Sistema verifica pertencimento do estudante e da matéria à academia
4. Sistema infere o `ano_academico` (mesma lógica das notas)
5. Sistema verifica idempotência (chave: `anoLectivo_data_materiaID`)

**Quantidade**: deve ser positiva (≥ 1)

**Data**: formato `AAAA-MM-DD`

**Regra de registro**: faltas mantêm unicidade por combinação de ano letivo + data + matéria.

**Quantidade por registro**: não possui teto máximo (apenas deve ser `>= 1`).

**Deleção de falta**: `motivo` é **obrigatório**; soft delete

---

### 5.6 Avaliação Final de Ano Académico

**Quem faz**: Academia (status ativo, com ano letivo configurado)

Este é o **único mecanismo de transição de ano** no sistema. Registar a avaliação final é o que faz o estudante avançar (ou não) de ano.

**Processo:**

1. Academia envia: código do estudante, tipo de ensino, nível atual, próximo nível (se aprovado), flag de aprovação
2. Sistema valida o ano letivo ativo
3. Sistema valida pertencimento do estudante à academia
4. Sistema verifica idempotência (chave: `tipoEnsino_anoLectivo_anoAcademicoAtual`)

**Validação de notas antes da aprovação:**

- Para `fundamental`: todas as matérias do ano devem ter `nota_escola` nos 3 trimestres
- Para `medio`: todas as matérias do curso/ano devem ter `nota_escola` nos 3 trimestres
- Para `superior`: todas as matérias do curso/período devem ter `nota_exame`
- Se notas estiverem faltando, a aprovação é bloqueada — a menos que `observacao` seja fornecida (override manual)
- Se `aprovado = false`, a validação de notas é ignorada

**Efeitos da aprovação:**

- Com `proximo_ano_academico` preenchido → avança o estudante para o próximo nível
- Com `proximo_ano_academico = null` (último ano do ciclo) → marca o status como `finalizado`

**Efeitos da reprovação:**

- Nenhuma alteração de ano ou status; apenas registado no histórico

**Efeitos automáticos (em ambos os casos):**

- O estudante é **removido de todas as turmas** da academia

**Atomicidade na projeção**: a lista de turmas a remover é incluída no payload do evento `AvaliacaoFinalAnoAcademico`. A projeção de turmas processa este mesmo evento e remove o estudante de todas as turmas listadas no mesmo ciclo de projeção, evitando inconsistência por falha parcial entre avaliação final e remoção.

**Consultas:**

- `GET /avaliacoes` → todos os registos
- `GET /aprovacoes` → apenas aprovados (`aprovado = TRUE`)
- `GET /reprovacoes` → apenas reprovados (`aprovado = FALSE`)

---

### 5.7 Gestão de Turmas

**Quem faz**: Academia

**Ciclo de vida de uma turma:**

```
Criada (ativo) → Desativada (inativo) → Deletada (deletado)
              ↑_________|
         (pode ativar novamente)
```

**Para deletar**: a turma deve estar inativa e sem estudantes vinculados.

**Adição de estudantes**: o estudante deve pertencer à academia. Apenas estudantes do superior podem estar em múltiplas turmas simultaneamente.

**Remoção automática**: ao registar avaliação final, o estudante é removido de todas as turmas via processamento do evento `AvaliacaoFinalAnoAcademico` na projeção de turmas.

**Histórico por ano letivo**: cada turma mantém `historico_estudantes_ano_letivo` (mapa `ano_letivo -> [codigo_estudante]`) com os estudantes que já fizeram parte dela em cada ano letivo.

---

### 5.8 Gestão de Cursos e Matérias

**Ciclo de vida do curso:**

```
Criado → Ativado → Desativado → Deletado
       ↑__________|
```

Para deletar um curso:

1. Deve estar inativo
2. Não pode ter estudantes matriculados
3. Não pode ter matérias ativas (desativar todas primeiro)
4. Matérias inativas são deletadas em cascata automaticamente
5. Turmas inativas vinculadas são deletadas em cascata automaticamente

**Ciclo de vida da matéria superior:**

```
Criada (INATIVO) → Período Definido → Ativada (ATIVO) → Desativada → Deletada
```

Matérias fundamental e médio são criadas já **ativas**.

**Validação de período**: o período da matéria deve existir na lista de períodos do curso vinculado.

---

### 5.9 Verificação de Integridade do Ledger

O sistema suporta verificação da cadeia de hashes do ledger para qualquer estudante:

```
GET /verificar-integridade/:codigo
```

A função SQL `verify_hash_chain` verifica se todos os hashes encadeados são válidos. Se qualquer evento foi adulterado, a verificação retorna `integro = false` indicando a versão onde a cadeia foi quebrada.

---

### 5.10 Rebuild de Projeções

Admins com role `fpp` podem reconstruir projeções:

```
POST /dominis/projections/rebuild/:name
```

Para evitar timeout em rebuilds longos (ex.: projeções com alto volume de eventos no ledger), use a versão assíncrona:

```
POST /dominis/projections/rebuild/:name/async
```

Esse endpoint retorna `202 Accepted` com `job_id`, `poll_url` e `sse_url`; o cliente pode acompanhar em `GET /jobs/:id` e/ou receber eventos em `GET /jobs/stream`.

**Concorrência de rebuild**: o manager permite apenas **1 rebuild por vez** (lock global). Se outro rebuild já estiver em execução, o endpoint síncrono retorna `409 Conflict`.

**Antes de reconstruir**, o sistema verifica a integridade completa do ledger. Se qualquer aggregate estiver com hash chain inválida, o rebuild é abortado.

**Ordem de rebuild recomendada** (respeita dependências):

1. `admins`
2. `academias`
3. `cursos`, `materias`, `categorias_nota`, `telefones_extra`
4. `estudantes`, `turmas`
5. `notas`, `faltas`
6. `avaliacao_final`

---

### 5.11 Sistema de Jobs Assíncronos

Para operações em lote com muitos itens, o sistema oferece endpoints `/async` que criam um **job em background**:

```
POST /academia/notas-aluno/async  →  { "job_id": "uuid", "status": "pending", "poll_url": "/jobs/:id", "sse_url": "/jobs/stream" }
GET  /jobs/:id                    →  { "status": "done", "progress": 100, ... }
```

**Pool de workers**: 4 goroutines paralelas processam itens em background.

**Polling recomendado**: intervalo de 1.5s a 8s com backoff exponencial.

**Status do job**: `pending` → `processing` → `done` / `failed`

Se qualquer item falhar, o job fica como `failed` (não `done`), permitindo que o cliente identifique e retente apenas os itens com falha.

**Persistência resiliente de payload e progresso**:

- Cada item processado é persistido imediatamente em `async_jobs.results` (sem janela de 10 itens).
- O `payload` bruto do job é preservado integralmente em `async_jobs.payload`.
- Cada resultado individual inclui também o `payload` do item, permitindo replay exato dos itens que falharam.
- Em reinício/crash, o worker varre jobs `pending`/`processing` e retoma do ponto salvo (`done_items + fail_items`), evitando perda silenciosa de itens.
- O enqueue dos endpoints `/async` valida e conta itens no body bruto (sem dupla serialização do array), reduzindo consumo de CPU/memória e janelas de timeout em lotes grandes (ex.: notas/faltas).

**Erro sempre explícito em jobs assíncronos**:

- Jobs com falha parcial agora finalizam com `status=failed` e `error` contendo o motivo com amostras de itens.
- Rebuild de projeções retorna o erro real no body HTTP, não apenas log interno.
- Rebuild assíncrono de projeções usa o mesmo pipeline de integridade do rebuild síncrono, mas sem manter a conexão HTTP aberta por minutos.

---

## 6. Regras de Negócio

### 6.1 Regras de Academia

| Regra                                                         | Detalhe                                                |
| ------------------------------------------------------------- | ------------------------------------------------------ |
| Academias nascem inativas                                     | Apenas um admin pode ativar                            |
| Escola com nível fundamental/misto deve ter `anos_academicos` | Sem anos, o cadastro é rejeitado                       |
| Escola com nível médio não deve ter `anos_academicos`         | Anos são do curso, não da academia                     |
| Apenas academias ativas podem operar                          | Middleware valida status em cada request               |
| Cada academia define o seu próprio ano letivo                 | Sem ano letivo, notas/faltas/avaliações são bloqueadas |
| Senha padrão = código da academia                             | Deve ser alterada após o primeiro login                |
| Desativação exige motivo                                      | Registado no ledger e na projeção para auditoria       |

### 6.2 Regras de Estudante

| Regra                                                         | Detalhe                                 |
| ------------------------------------------------------------- | --------------------------------------- |
| Apenas academia pode cadastrar estudantes                     | Não existe auto-cadastro                |
| `genero` e `data_nascimento` são obrigatórios                 | Não podem ser omitidos                  |
| `data_nascimento` deve ser anterior a hoje                    | Validação no aggregate                  |
| Senha padrão = código do estudante                            | Ex: `ABC1234` acede com senha `ABC1234` |
| Status superior exige fundamntal e médio finalizados/inativos | Progressão lógica do ensino             |
| Deleção de nota/falta exige motivo                            | Para auditoria                          |

### 6.3 Regras de Notas

| Regra                                       | Detalhe                                                       |
| ------------------------------------------- | ------------------------------------------------------------- |
| Nota deve ser 0 ou mais                     | Validação no aggregate, não apenas no handler                 |
| Academia escola só registra notas `escolar` | Academia superior só registra `superior`                      |
| Observação obrigatória na correção          | Justificativa da alteração                                    |
| Motivo obrigatório na deleção               | Para auditoria                                                |
| Duplicata bloqueada no aggregate            | Mesma combinação ano/período/matéria/tipo/categoria rejeitada |
| Nota deletada não pode ser re-registada     | Mapa de chaves não remove entradas deletadas                  |

### 6.4 Regras de Avaliação Final

| Regra                                       | Detalhe                                    |
| ------------------------------------------- | ------------------------------------------ |
| Aprovação exige notas presentes             | Verificação automática antes de aprovar    |
| Observação permite override                 | Aprovação forçada mesmo sem todas as notas |
| Próximo ano obrigatório se não for o último | Exceto quando o ciclo termina              |
| Reprovação não altera o ano/status          | Apenas registado no histórico              |
| Uma avaliação por tipo/ano letivo/nível     | Idempotência via mapa no aggregate         |
| Aprovação ou reprovação remove das turmas   | Automaticamente ao registar                |

### 6.5 Regras de Turma

| Regra                                                | Detalhe                                   |
| ---------------------------------------------------- | ----------------------------------------- |
| Código único por academia                            | Não pode repetir dentro da mesma academia |
| Turno: `manha`, `tarde` ou `noite`                   | Valores fixos                             |
| Deleção exige inatividade                            | Desativar antes de deletar                |
| Deleção exige sem estudantes                         | Remover todos os estudantes primeiro      |
| Estudante do superior pode estar em múltiplas turmas | Sem restrição de exclusividade            |

### 6.6 Regras de Curso

| Regra                                     | Detalhe                                                  |
| ----------------------------------------- | -------------------------------------------------------- |
| Tipo imutável após criação                | `medio` nunca vira `superior`                            |
| Curso superior exige períodos             | Ao menos um semestre                                     |
| Curso médio não deve ter períodos         | Trimestres são fixos do sistema                          |
| Deleção exige inatividade                 | Desativar primeiro                                       |
| Deleção exige sem estudantes matriculados | Verificação antes de deletar                             |
| Matérias ativas bloqueiam deleção         | Desativar todas as matérias antes                        |
| Cascata na deleção                        | Matérias e turmas inativas são deletadas automaticamente |

### 6.7 Regras de Admin

| Regra                                    | Detalhe                                                                                            |
| ---------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Hierarquia estrita                       | Só pode gerenciar roles estritamente inferiores                                                    |
| Email verificado obrigatório para operar | Sem verificação, acesso ao painel é bloqueado                                                      |
| Apenas FPP altera roles                  | Regra de negócio deliberada                                                                        |
| Não pode desativar a si próprio          | Prevenção de bloqueio acidental                                                                    |
| Bootstrap único                          | Primeiro FPP criado via endpoint especial com advisory lock                                        |
| Senha gerada automaticamente             | Senha segura gerada com `crypto/rand` e enviada por email (apenas depois do e-mail ser verificado) |

---

## 7. Sistema de Permissões

### 7.1 Middleware de Autenticação

Toda rota protegida passa por `AuthMiddleware` que:

1. Valida o JWT (assinatura e expiração)
2. Consulta a projeção e verifica se o usuário está **ativo**
3. Rejeita com 401 se o usuário estiver inativo, mesmo com JWT válido
4. Injeta `user_id` e `user_type` no contexto Gin

**Degradação graciosa**: se o banco estiver indisponível durante a verificação de status, o middleware permite a passagem (logando um aviso). Evita bloquear todo o tráfego por instabilidade pontual do banco.

### 7.2 Matriz de Permissões

| Endpoint                  | Estudante | Academia | Gerente | ADM | FPP |
| ------------------------- | --------- | -------- | ------- | --- | --- |
| Login                     | ✅         | ✅        | ✅       | ✅   | ✅   |
| Meu Perfil                | ✅         | ✅        | ✅       | ✅   | ✅   |
| Alterar Senha             | ✅         | ✅        | ✅       | ✅   | ✅   |
| Minhas Notas/Faltas       | ✅         | —        | —       | —   | —   |
| Registrar Nota/Falta      | —         | ✅        | —       | —   | —   |
| Cadastrar Estudante       | —         | ✅        | —       | —   | —   |
| Gerenciar Cursos/Matérias | —         | ✅        | —       | —   | —   |
| Ver Todos os Estudantes   | —         | Próprios | ✅       | ✅   | ✅   |
| Registrar Academia        | —         | —        | —       | —   | ✅   |
| Ativar/Desativar Academia | —         | —        | —       | ✅   | ✅   |
| Criar Admin               | —         | —        | —       | —   | ✅   |
| Ativar/Desativar Admin    | —         | —        | —       | ✅   | ✅   |
| Alterar Role Admin        | —         | —        | —       | —   | ✅   |
| Rebuild de Projeção       | —         | —        | —       | —   | ✅   |

### 7.3 Login Unificado

Existe apenas **um endpoint de login** (`POST /login`). O tipo do usuário é inferido automaticamente por busca em cascata:

```
1. Busca em projection_admins (por email)
2. Busca em projection_academias (por código ou email)
3. Busca em projection_estudantes (por código ou email)
```

**Anti-timing attack**: o hash bcrypt é **sempre** comparado, mesmo quando o usuário não existe (usa um hash dummy). Isso torna o tempo de resposta idêntico para "usuário não encontrado" e "senha errada", impedindo user enumeration.

**Bloqueio por email não verificado**: academias e estudantes que tentam login com email (não com código) e o email ainda não foi verificado são bloqueados com mensagem clara.

Retornar mensagem de aviso quando não foi encontrado o usuário
Retornar mensagem de aviso quando a senha for incorreta

---

## 8. Segurança e Autenticação

### 8.1 JWT

- Algoritmo: HS256
- Expiração configurável via `JWT_EXPIRY_HOURS` (padrão: 24h)
- Secret configurável via `JWT_SECRET` (obrigatório em produção)
- Payload: `user_id` (UUID) e `user_type` (string)

### 8.2 Senhas

- Algoritmo: bcrypt com custo padrão
- Senhas de admins: geradas com `crypto/rand` (segurança criptográfica), nunca hardcoded
- Senhas de academia/estudante: código da entidade como senha inicial
- Todas as alterações de senha passam pelo ledger (evento)

### 8.3 Hash Chain do Ledger

Cada evento no ledger tem um hash SHA256 que inclui o hash do evento anterior:

```
hash(evento_N) = SHA256(conteúdo_N + hash(evento_N-1))
```

Qualquer adulteração de um evento invalida toda a cadeia a partir daquele ponto, tornando a adulteração detectável.

### 8.4 Whitelist de Eventos

Apenas eventos previamente autorizados podem ser gravados no ledger (`safe_queries.go`). Qualquer evento desconhecido é rejeitado antes de chegar ao banco.

### 8.5 Segurança das Queries

- Todas as queries SQL usam prepared statements com placeholders (`$1`, `$2`, ...)
- Nomes de tabelas interpolados dinamicamente só ocorrem em um switch fechado com valores constantes (sem input do usuário)
- Inputs validados e sanitizados antes de qualquer operação

---

## 9. Operações em Lote

### 9.1 Batch Assíncrono

Endpoints `/async` criam um job e retornam imediatamente. O processamento ocorre em background.

**Limites:** até 1000-2000 itens (dependendo do endpoint).

**Formato de payload:** array JSON (não objeto com `items`).

```json
[
  {"...": "payload do endpoint síncrono equivalente"}
]
```

**Fluxo:**

```
POST /academia/notas-aluno/async  →  202 Accepted + { job_id }
(polling)
GET  /jobs/:id                    →  { status, progress, done_items, fail_items }
(quando status = "done" ou "failed")
GET  /jobs/:id?results=true       →  { ... resultados por item ... }
```

**Gestão de stream SSE por academia:**

- `DELETE /jobs/:id/sse`: remove/oculta um job específico do stream `GET /jobs/stream` da própria academia.

**Retry parcial de falhas:**

- `POST /jobs/:id/retry-failed`: cria um novo job com o mesmo tipo do original, reenviando apenas os itens com falha (`sucesso=false`).
- Isso evita reprocessar itens que já deram certo.

**Cobertura adicional em 1.0.9 (SSE padronizado em todas as respostas `/async` + novidades prévias):**
- Academia: `PUT /academia/dados/async`, `POST /academia/categorias-nota/async`, `DELETE /academia/categorias-nota/async`.
- Cursos: `PUT /academia/curso/ativar|desativar|dados/async`, `DELETE /academia/curso/async`.
- Matérias: `PUT /academia/materia/ativar|desativar|periodo|dados/async`, `DELETE /academia/materia/async`.
- Turmas: `PUT /academia/turma/ativar|desativar|dados/async`, `DELETE /academia/turma/async`, `DELETE /academia/turma/estudante/async`.
- Admin: `PUT /dominis/admin/ativar/async`, `PUT /dominis/admin/desativar/async`.
- Consulta única por papel: `GET /turmas-estudante/:codigo` (estudante: próprio, academia: da sua academia, admin: qualquer estudante).

---

## 10. Recomendações de Melhoria

### 10.1 Soft Delete de Estudante

**Problema atual**: não existe um mecanismo de "arquivar" um estudante que saiu da escola sem precisar deletar os dados históricos.

**Recomendação**: adicionar um status `arquivado` para estudantes que saíram da academia mas cujos registos históricos devem ser mantidos.

### 10.2 Validação de Data de Falta

**Problema atual**: não existe validação de que a data de uma falta está dentro do ano letivo ativo.

**Recomendação**: validar no handler que a data da falta pertence ao período do ano letivo ativo da academia.

### 10.3 Nota Deletada Bloqueia Re-registro

**Problema atual**: uma nota deletada não pode ser re-registada com a mesma combinação de chave. Isso pode ser inconveniente se a deleção foi um erro.

**Recomendação**: avaliar se este comportamento é desejável. Se não, remover a chave do mapa `NotasRegistradasPorChave` no `applyNotaDeletada`.

### 10.4 Auditoria de Acessos de Leitura

**Problema atual**: apenas mutações são registadas no ledger. Não há registo de quem consultou os dados de um estudante.

**Recomendação**: para dados sensíveis, considerar um log de auditoria de leituras separado (não no ledger, mas numa tabela de auditoria).

### 10.5 Rate Limiting

**Problema atual**: o rate limiting está desativado em todos os endpoints (todos os middlewares de rate limit retornam `c.Next()` sem verificar nada).

**Recomendação**: ativar rate limiting real com `golang.org/x/time/rate` ou similar, especialmente nos endpoints de login, email e bootstrap.
