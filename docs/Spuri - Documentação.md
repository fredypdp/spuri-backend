---
modificado: 26-06-2026 00:00
criado: 05-04-2026 13:01
---
Versão atual: 2.0.0
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
11. [[#11. Solicitação de Matrícula e Armazenamento]]

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

**Natureza da academia (`type`):**

| Type      | Significado |
| --------- | ----------- |
| `public`  | Instituição pública |
| `private` | Instituição privada |

**Ano Letivo**: o ano letivo ativo é definido e mantido por academia. Sem ano letivo ativo na academia, nenhum registro de nota, falta ou avaliação é permitido. O sistema também mantém `anos_letivos_lista` (array de objetos) com histórico sem duplicação por `ano_letivo`.

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


### Categorias de nota (código vs rótulo)

- `codigo`: identificador técnico único por academia (sem espaços). Exemplo de categoria personalizada: `prova_profesor`.
- `nome`: rótulo descritivo exibido ao usuário (pode conter espaços). Exemplo de categoria personalizada: `Prova do professor`.
- Códigos fixos e rótulos padrão:
  - `nota_escola` -> `Nota da escola`
  - `nota_professor` -> `Nota do professor`
  - `nota_pp1` -> `Prova Parcelar 1`
  - `nota_pp2` -> `Prova Parcelar 2`
  - `nota_exame` -> `Exame Final`

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
| bilhete_identidade_responsavel | Texto                              | Obrigatório caso bilhete_identidade esteja vazio; não pode ser igual a `bilhete_identidade` |

> Regra de documentos: quando ambos forem informados, `bilhete_identidade` e `bilhete_identidade_responsavel` são comparados sem espaços nas extremidades e sem diferenciar maiúsculas/minúsculas; valores iguais são rejeitados no cadastro direto, atualização de dados pessoais e solicitação de matrícula.

**Progressão escolar**: o estudante tem três trajetórias paralelas e independentes:

```
Status Fundamental: inativo → em_andamento → finalizado
Status Médio:       inativo → em_andamento → finalizado
Status Superior:    inativo → em_andamento → finalizado
```

**Regra de status**: o status geral do estudante pode ser `inativo`, `ativo` ou `arquivado`; `finalizado` não é status geral. Os status escolares (`status_escolar_fundamental`, `status_escolar_medio`, `status_superior`) continuam aceitando `inativo`, `em_andamento` e `finalizado`, mas não são definidos por endpoints diretos: eles mudam por eventos reais como matrícula, interrupção, trancamento, avaliação final aprovada, equivalência/conclusão externa reconhecida, desvinculação e reintegração.

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
|`MatriculaFundamentalEfetivada`|Fundamental iniciado ou retomado|
|`FundamentalInterrompido`|Fundamental interrompido sem conclusão|
|`MatriculaMedioEfetivada`|Médio iniciado ou retomado|
|`MedioInterrompido`|Médio interrompido sem conclusão|
|`MatriculaSuperiorEfetivada`|Superior iniciado|
|`SuperiorTrancado`|Superior trancado|
|`EstudanteDesvinculadoDaAcademia`|Estudante saiu da academia e foi arquivado|
|`EstudanteReintegrado`|Estudante arquivado voltou para a academia|
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

O código de turma deve ser **único dentro da academia**. Antes da validação de unicidade, `codigo_turma` é normalizado: espaços antes/depois são descartados, somente espaços internos entre textos viram `_` e caracteres especiais diferentes de `_` são rejeitados.

**Estudantes na turma**: guardados como lista de `CodigoEstudante` (strings). Um estudante pode estar em múltiplas turmas simultaneamente.

**Integridade em atualização**: ao atualizar `nivel` e/ou `curso_id` da turma, o sistema valida os estudantes já vinculados. Se algum ficar incompatível, a alteração é rejeitada para evitar estado inconsistente.

**Deleção**: a turma deve estar inativa e sem estudantes vinculados.

**Remoção automática**: removida para avaliações finais escolares; nesses casos o efeito agora é progressão/retenção de turma (ver seção de avaliação final).

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

1. Admin envia dados da academia (nivel, type, nome, província, endereço, nível escolar, etc.)
2. Sistema gera o código único consultando o ledger (ex: `LDA20261`)
3. Sistema gera a senha padrão = código da academia (ex: `LDA20261`)
4. Academia é criada com **status `inativo`**
5. Um admin com role `adm` ou `fpp` deve ativar manualmente

**Regras de validação:**

- `nivel` deve ser `escola` ou `superior`
- `type` é obrigatório (campo textual) e deve ser `public` ou `private`
- Para `nivel=escola` com nível escolar `fundamental` ou `misto`: `anos_academicos` é obrigatório (formato `[1-9]_ano_fundamental`)
- Para `nivel=escola` com nível escolar `medio`: `anos_academicos` não deve ser informado
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
- `ano_escolar_fundamental` deve seguir o formato canônico para o tipo de ensino
- Se informar `curso_medio_id`, o curso deve existir e ser do tipo `medio`
- Se informar `curso_superior_id`, o curso deve existir e ser do tipo `superior`
- Status inicial padrão para fundamental: `em_andamento`
- Status inicial padrão para médio e superior: `em_andamento`

---

### 5.3 Configuração do Ano Letivo

**Quem faz**:

- **Admin FPP** define o **ano letivo oficial global por tipo** via `POST /admin/definir-ano-letivo-geral`, informando `type=escolar` ou `type=superior` e o `ano_letivo` desejado no formato `YYYY_YYYY`, apenas enquanto não houver academia ativa daquele tipo com ano letivo definido
- **Academia** define o seu primeiro ano letivo ativo via `POST /academia/definir-ano-letivo`, sempre alinhado ao global do tipo inferido pelo próprio cadastro (`escola` → `escolar`, `superior` → `superior`); depois avança automaticamente ao finalizar o ano letivo

Antes de registar qualquer nota, falta ou avaliação, a academia deve definir o ano letivo ativo.

Os anos letivos oficiais globais são persistidos em `projection_sistema_config` com as chaves `ano_letivo_atual_escolar` e `ano_letivo_atual_superior`; essa projeção deve existir antes da chamada administrativa.

Além do valor atual, o sistema mantém `anos_letivos_lista` em `projection_sistema_config` como histórico global (sem duplicar `ano_letivo`). Esse histórico pode ser consultado por qualquer usuário autenticado na rota `GET /anos-letivos-lista?type=...` e o valor atual em `GET /ano-letivo?type=...`.

**Formato obrigatório**: `YYYY_YYYY` onde o segundo ano é exatamente o primeiro + 1 (ex: `2025_2026`)

**Tipo**: `escolar` ou `superior`; `escola` não é aceito como alias para tipo de ano letivo.

A academia só pode definir diretamente quando ainda não possui ano letivo. Depois disso, a evolução acontece pela finalização realizada pelas academias, que calcula o próximo período a partir do ano final do anterior. O ano letivo ativo é resolvido automaticamente em todos os novos registos de nota, falta e avaliação.

O período real aceito para faltas não é salvo como datas fixas em cada ano. O Admin FPP mantém uma configuração global por tipo em `projection_anos_letivos_configuracoes` (`type` + `periodo`). O backend combina essa configuração com o `ano_letivo` ativo da academia para calcular o intervalo: com `ano_letivo=2025_2026` e `periodo=10_07`, o início é `2025-10-01` e o fim é `2026-07-31`.

**Regra de alinhamento obrigatório**: se a academia tentar definir um ano letivo diferente do ano oficial global do seu tipo definido pelo admin FPP, a operação deve ser rejeitada com erro de negócio.

Sempre que o ano letivo for atualizado, ele é adicionado em `anos_letivos_lista` apenas se ainda não existir para aquela academia. Se já existir, o backend ignora a duplicação.

---

### 5.4 Registro de Notas

**Quem faz**: Academia (status ativo, com ano letivo configurado)

**Processo:**

1. Academia envia: código do estudante, período, matéria, tipo, categoria, valor
2. Sistema valida o ano letivo ativo da academia
3. Sistema verifica que estudante pertence à academia
4. Sistema verifica que matéria pertence à academia
5. Sistema infere o `ano_academico`:
    - Se estudante tem `ano_escolar_fundamental` preenchido (fundamental) → usa esse valor **somente se** esse ano existir em `anos_academicos` da matéria
    - Se não existir, o registro é bloqueado com erro de validação (incompatibilidade estudante × matéria)
    - Caso contrário → usa `anos_academicos[0]` da matéria
6. Sistema valida se a `categoria` está configurada na academia com `anos_academicos` contendo o `ano_academico` inferido; sem anos definidos ou sem correspondência, o registro é bloqueado
7. Sistema verifica idempotência (chave: `codigoAcademia_anoLectivo_periodo_materiaID_tipo_categoria`)
8. Se não for duplicata, emite `NotasRegistradas` no ledger do estudante

**Tipos de nota:**

|Tipo|Academia|Categorias fixas|Períodos|
|---|---|---|---|
|`escolar`|`escola`|`nota_escola`, `nota_professor`|`1_trimestre`, `2_trimestre`, `3_trimestre`|
|`superior`|`superior`|`nota_pp1`, `nota_pp2`, `nota_exame`|Semestres do curso|

Academias podem criar **categorias adicionais** personalizadas e também configurar as categorias fixas/obrigatórias. Toda categoria de nota possui `anos_academicos`; apenas os anos presentes nessa lista aceitam registros. Se a categoria não tiver anos definidos, nenhuma nota pode ser registrada nela. O `codigo` da categoria é normalizado antes de persistir: espaços antes/depois são descartados, somente espaços internos entre textos viram `_`, letras maiúsculas viram minúsculas e caracteres especiais diferentes de `_` são rejeitados.

**Valor**: entre 0 ou mais (validado no aggregate)

**Correção de nota**: `observacao` é **obrigatória** (justificativa da correção)

**Deleção de nota**: `motivo` é **obrigatório**; soft delete (permanece no ledger)

**Proteção contra duplicatas**: o aggregate mantém um mapa em memória (`NotasRegistradasPorChave`). Se a mesma combinação de academia/ano/período/matéria/tipo/categoria já existir, o comando é rejeitado com erro de negócio claro antes de tocar o banco.

---

### 5.5 Registro de Faltas

**Quem faz**: Academia (status ativo, com ano letivo configurado)

**Processo:**

1. Academia envia: código do estudante, data, matéria, quantidade
2. Sistema valida ano letivo ativo
3. Sistema verifica pertencimento do estudante e da matéria à academia
4. Sistema infere o `ano_academico` (mesma lógica das notas)
5. Sistema verifica idempotência (chave: `data+codigo_estudante+materia_disciplinar_id`; no aggregate ela é resolvida no contexto de academia + ano letivo)

**Quantidade**: deve ser positiva (≥ 1)

**Data**: formato `AAAA-MM-DD` (date-only, sem componente de hora)

**Regra de registro**: faltas mantêm unicidade por combinação de `data + codigo_estudante + materia_disciplinar_id` (equivalente à unicidade técnica em projeção: estudante + academia + data + matéria).

**Quantidade por registro**: não possui teto máximo (apenas deve ser `>= 1`).

**Correção de falta**: `observacao` é **obrigatória** (justificativa da correção).

**Deleção de falta**: `motivo` é **obrigatório**; soft delete

---

### 5.6 Avaliação Final de Ano Académico

**Quem faz**: Academia (status ativo, com ano letivo configurado)

A avaliação final é o mecanismo auditável que decide aprovação, reprovação, progressão de nível e finalização de ciclo. A decisão **não é manual**: a academia configura regras de avaliação final, e o backend calcula a `nota_final` por fórmula, compara com a `nota_minima_aprovacao` da regra e registra o resultado por evento.

**Conceitos principais:**

- `type` identifica publicamente a etapa da avaliação final configurada na regra (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.). Ele não é enviado pelo cliente para executar a avaliação: na execução automática, o backend descobre o `type` percorrendo a cadeia de regras aplicável, da raiz até as dependentes.
- `tipo_ensino` da avaliação é sempre inferido do estudante: `superior` tem prioridade quando há curso/ano/status superior; depois `medio`; caso contrário, `fundamental`.
- `nivel_ano_academico_atual` precisa ser o nível atual real do estudante e precisa ser válido para o tipo de ensino inferido.
- `proximo_ano_academico` é sempre calculado pelo backend. O cliente não pode enviá-lo.
- `aprovado` também é sempre calculado pelo backend e não é aceito como decisão manual.

**Configuração de regras:**

- Cada regra pertence à academia autenticada e contém `type`, `nome`, `descricao`, `tipo_ensino`, `anos_academicos`, `nota_minima_aprovacao`, `categorias_envolvidas`, `formula`, `aplica_se_reprovado_em_type`, `status` e `version`.
- `type` é obrigatório na criação; o cliente deve enviar explicitamente a etapa pública (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.). O backend aceita apenas letras, números, espaços e `_`, descarta espaços antes/depois, converte apenas espaços internos entre textos para `_` antes de persistir e rejeita outros caracteres.
- `type`, `nome`, `tipo_ensino`, `anos_academicos`, `formula` e `nota_minima_aprovacao > 0` são obrigatórios na criação; `descricao` é opcional. `categorias_envolvidas` não precisa ser enviado: o backend extrai automaticamente as categorias referenciadas na `formula`. Se o cliente enviar esse campo, ele deve bater exatamente com as categorias da fórmula, sem sobras nem omissões. Exemplos de `nome`: `Avaliação final`, `Avaliação final (com exame)` e `Avaliação final (com recurso)`.
- `tipo_ensino` deve ser exatamente `fundamental`, `medio` ou `superior`.
- Não pode haver dois registros ativos com o mesmo `codigo_academia`, `tipo_ensino`, `type` e ano acadêmico sobreposto. Assim, regras do mesmo `type` podem coexistir para anos diferentes, mas não para o mesmo ano. Ao criar ou editar uma regra, o `type` pode ser igual ao de uma regra inativa; nesse caso, a regra inativa fica impedida de ser ativada enquanto existir regra ativa conflitante.
- Para cada academia, tipo de ensino e ano acadêmico, deve existir no máximo uma regra raiz ativa. Regra raiz é a regra sem `aplica_se_reprovado_em_type`.
- `aplica_se_reprovado_em_type` é opcional apenas para a regra raiz; em regra dependente (`avaliacao_final_com_recurso`, por exemplo), passa pela mesma normalização de `type`, deve apontar para um `type` ativo existente na mesma academia e tipo de ensino, não pode apontar para o próprio `type`, não pode criar ciclo de dependências e deve usar exatamente os mesmos `anos_academicos` da regra raiz da cadeia. Uma regra dependente inativa não pode ser ativada enquanto a regra da qual ela depende estiver inativa.
- A cadeia aplicável a um estudante precisa ter exatamente uma raiz; regras dependentes só participam quando apontam para outro `type` dentro da mesma cadeia aplicável ao ano acadêmico.

**Fórmula textual (`formula_textual_v1`):**

A regra usa `formula` como string declarativa. O backend valida e interpreta a expressão com parser próprio; não há `eval`, JavaScript, SQL dinâmico, templates executáveis nem execução de código do usuário. O modelo antigo em árvore JSON foi removido e não é alternativa suportada.

Sintaxe oficial:

- Referência de nota: `[categoria,periodo]`, por exemplo `[nota_escola,1_trimestre]`.
- Operadores permitidos: `+`, `-`, `*`, `/`.
- Precedência: multiplicação/divisão antes de soma/subtração. Parênteses podem agrupar partes da fórmula.
- Constantes numéricas positivas ou zero podem ser usadas para médias, pesos e divisores, com decimal por ponto (`0.4`).
- Espaços são ignorados. Qualquer caractere fora dessa gramática é rejeitado.
- O backend extrai de `formula` as `categorias_envolvidas`; todas as categorias referenciadas precisam estar ativas/configuradas para a academia nos anos acadêmicos da regra. Se `categorias_envolvidas` for enviado manualmente, ele precisa corresponder exatamente à extração da fórmula.
- Todos os períodos são validados pelas regras existentes (`1_trimestre`, `2_trimestre`, `3_trimestre`, `[n]_semestre` etc.).
- Divisão por zero é bloqueada tanto na validação quanto durante o cálculo.
- Fórmulas grandes demais são rejeitadas.

Exemplos válidos:

```text
([nota_escola,1_trimestre]+[nota_escola,2_trimestre]+[nota_escola,3_trimestre])/3
([nota_escola,1_trimestre]*0.3)+([nota_escola,2_trimestre]*0.3)+([nota_exame_final,3_trimestre]*0.4)
[nota_escola,1_trimestre]+[nota_professor,1_trimestre]
```

Exemplos inválidos:

```text
{ "op": "..." }
[nota_escola]
[nota_escola,1_trimestre]/0
eval([nota_escola,1_trimestre])
[nota_inexistente,1_trimestre]
```

Quando a fórmula referencia uma nota ainda ausente, a avaliação não é fechada naquele momento; ela aguarda novo lançamento.

**Processo automático ao registrar notas:**

Não há rota pública registrada para execução manual de avaliação final. A antiga rota manual de avaliação final não faz parte do contrato exposto; a avaliação final é acionada automaticamente pelo registro de notas. O cliente configura regras e lança notas, e o backend decide quando a avaliação pode ser calculada.

1. Ao registrar uma nota, o backend valida a academia autenticada, o ano letivo, o estudante, a matéria/categoria/período e o pertencimento à academia.
2. O backend infere o `tipo_ensino` do estudante e identifica o ano acadêmico atual real.
3. O backend busca todas as regras ativas aplicáveis à academia, ao tipo de ensino e ao ano acadêmico.
4. A cadeia precisa ter exatamente uma regra raiz, isto é, uma regra sem `aplica_se_reprovado_em_type`.
5. A execução começa na raiz e não na categoria da nota recém-registrada. O `type` executado vem da regra encontrada.
6. Para regras dependentes, o backend segue `aplica_se_reprovado_em_type` e só executa a regra se o estudante foi reprovado no `type` pré-requisito, seja no mesmo processamento, seja em avaliação já persistida.
7. Se um pré-requisito aprovou, a dependente é encerrada sem execução; se o pré-requisito ainda não existe, a dependente aguarda.
8. O backend impede duplicidade para o mesmo estudante, academia, ano letivo, tipo de ensino, ano acadêmico e `type`.
9. O backend carrega notas do ano letivo atual apenas nas categorias envolvidas na regra, calcula a fórmula e obtém `nota_final`.
10. Se faltar nota exigida pela fórmula, a regra é ignorada naquele momento e será tentada novamente em novos lançamentos de nota.
11. O backend define `aprovado = nota_final >= nota_minima_aprovacao`.
12. O backend calcula `proximo_ano_academico`: quando reprovado, permanece no nível; quando aprovado, avança para o próximo nível ou retorna `null` no último nível do ciclo.
13. O evento registra `type`, `nota_final`, `nota_minima_aprovacao`, `regra_avaliacao_final_id`, `formula_snapshot`, `aplica_se_reprovado_em_type`, turma atual e turmas removidas.

**Como a fórmula considera períodos:**

- Cada referência `[categoria,periodo]` exige nota naquele par exato de categoria e período.
- Para médias simples, some explicitamente os períodos necessários e divida pela quantidade desejada.
- Para pesos, multiplique cada referência ou grupo por sua constante, por exemplo `[nota_escola,1_trimestre]*0.3`.

**Persistência, auditoria e versionamento:**

- O aggregate emite `AvaliacaoFinalEscolar` para fundamental/médio e `AvaliacaoFinalSuperior` para superior.
- A projeção `projection_avaliacao_final` grava os dados calculados e usa unicidade por `codigo_estudante`, `codigo_academia`, `ano_lectivo`, `tipo_ensino`, `ano_academico_atual` e `type`.
- A avaliação salva o snapshot da regra usada (`formula_snapshot`) e o identificador `regra_avaliacao_final_id`; alterações futuras na regra não alteram avaliações já registradas.
- O campo `version` da avaliação na projeção acompanha a versão do evento do aggregate.
- O campo `version` da regra começa em `1` na criação da regra e aumenta a cada edição ou inativação/deleção lógica.

**Cálculo da nota final e decisão de aprovação/reprovação:**

- A decisão final é sempre `aprovado = nota_final >= nota_minima_aprovacao`; se a nota calculada for menor que a mínima, o resultado é reprovação.
- `nota_final` não é uma média fixa do sistema. Ela é o resultado da `formula` configurada pela academia para a regra ativa daquele `tipo_ensino`, `ano_academico` e `type`.
- A fórmula lê somente notas do `ano_lectivo` atual, da mesma `codigo_academia`, do mesmo `codigo_estudante`, não deletadas (`deleted_at IS NULL`) e pertencentes às `categorias_envolvidas`.
- Se a fórmula exigir uma categoria/período que ainda não possui nota, o cálculo não é fechado. Na execução automática por lançamento de nota, a regra fica aguardando novos lançamentos; na execução manual interna/legada, o backend devolve erro de validação.
- Notas corrigidas ou deletadas não reabrem automaticamente uma avaliação final já registrada. A avaliação final é um evento auditável e idempotente por ano letivo, nível e `type`; ajustes posteriores exigem fluxo operacional próprio/rebuild conforme administração do sistema.

**Funcionamento escolar (Fundamental e Médio):**

- `fundamental` usa sequência fixa de níveis: `1_ano_fundamental` até `9_ano_fundamental`.
- `medio` usa a sequência `anos_academicos` do curso médio vinculado ao estudante; por isso o estudante precisa ter curso médio existente, ativo e com `anos_academicos` configurados.
- O backend valida que `nivel_ano_academico_atual` é exatamente o nível atualmente armazenado no estudante (`ano_escolar` para fundamental ou `ano_escolar_medio` para médio). Se o payload indicar outro nível, a avaliação é bloqueada.
- Se reprovado, `proximo_ano_academico` fica `null`, o estudante permanece no mesmo nível, os status de ciclo não mudam e ele não é removido das turmas atuais.
- Se aprovado e ainda existe próximo nível, `proximo_ano_academico` recebe o próximo item da sequência e o aggregate atualiza `ano_escolar` ou `ano_escolar_medio`.
- Se aprovado no último nível do ciclo, `proximo_ano_academico` fica `null` e o aggregate marca `status_escolar_fundamental` ou `status_escolar_medio` como `finalizado`.
- Para eventos escolares aprovados com turmas removidas, a projeção de turmas remove o estudante das turmas atuais, registra histórico no ano letivo e tenta adicioná-lo a uma turma ativa do próximo nível.
- A seleção de turma destino prioriza compatibilidade com `turno` e `curso_id` da turma de origem; se não houver compatível, usa qualquer turma ativa do próximo nível na mesma academia.
- Se não existir turma destino válida para aprovado com próximo nível, a projeção de turmas falha para impedir estado parcial.

**Funcionamento no Ensino Superior:**

- `superior` usa `semestre_atual` como unidade corrente; o backend o converte para `[n]_semestre`, valida o período em `curso.periodos` e deriva `ano_superior = ceil(semestre_atual / 2)`.
- O estudante precisa ter curso superior vinculado; o curso precisa existir, estar `ativo` e possuir `periodos` com o semestre atual.
- O backend valida que o nível avaliado no superior é o período derivado de `semestre_atual`, não o `ano_superior`; por exemplo, `semestre_atual = 2` avalia `2_semestre`.
- A aprovação no superior avança para o próximo semestre configurado no curso; no último semestre, marca `status_superior = finalizado`.
- A reprovação no superior mantém o estudante no mesmo `semestre_atual`, no mesmo `ano_superior` derivado e não altera `status_superior`.
- Avaliação superior não altera turmas automaticamente; vínculos com turmas do superior são geridos pelas regras próprias de turmas/matrícula.

**Transição para ano seguinte e semestres:**

- A transição do estudante sempre usa o **nível acadêmico atual** e o **próximo nível acadêmico** calculado pelo backend. Para Fundamental, a sequência é fixa; para Médio, a sequência vem de `curso.anos_academicos`; para Superior, a sequência vem de `curso.periodos`.
- Em aprovação com próximo nível, o evento grava `proximo_ano_academico` para Fundamental/Médio; no Superior, grava `proximo_semestre_atual` e o `ano_superior` derivado após a progressão.
- Em aprovação sem próximo nível, significa conclusão do último nível configurado: o campo de ano não avança e o status do ciclo passa para `finalizado`.
- Em reprovação, `proximo_ano_academico` fica `null`; isto significa retenção no mesmo nível, não conclusão.
- No Superior, a avaliação final progride por semestre. O campo persistido `semestre_atual` é a fonte de verdade, e `ano_superior` é compatibilidade derivada pela fórmula `ceil(semestre_atual / 2)`.
- Os semestres do Superior (`1_semestre`, `2_semestre`, etc.) são simultaneamente `periodos` de curso/matéria/nota e o escopo da regra de avaliação final superior. Regras superiores usam valores semestrais em `anos_academicos` por compatibilidade com o schema.
- Aprovação em semestre intermediário incrementa `semestre_atual` e recalcula `ano_superior`; aprovação no último semestre marca `status_superior = finalizado`; reprovação mantém semestre, ano superior e status.
- Eventos `AvaliacaoFinalSuperior` carregam o semestre avaliado, próximo semestre calculado e `ano_superior` antes/depois, permitindo rebuild determinístico das projeções de estudantes e avaliações.

**Cadeias de avaliação final (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso` etc.):**

- Para cada academia, tipo de ensino e ano acadêmico deve haver exatamente uma regra raiz aplicável, ou seja, uma regra ativa sem `aplica_se_reprovado_em_type`.
- Regras dependentes precisam ter o mesmo escopo de `anos_academicos` da raiz e só executam se o estudante foi reprovado no `type` indicado em `aplica_se_reprovado_em_type`. Exemplo: `avaliacao_final_com_recurso` pode depender de reprovação em `avaliacao_final`; `avaliacao_final_com_exame` pode depender de reprovação em `avaliacao_final_com_recurso`.
- Se a regra anterior aprovar, as dependentes são encerradas sem execução, porque não há reprovação a recuperar.
- Cada `type` tem idempotência própria: o estudante pode ter uma avaliação `avaliacao_final` e, se reprovado, uma avaliação `avaliacao_final_com_recurso`, mas não duas avaliações `avaliacao_final` para o mesmo ano letivo, nível e tipo de ensino.

**Consultas:**

- `GET /avaliacoes` → todos os registos, com filtros por `tipo_ensino`, `ano_letivo`, `ano_academico_atual`, `codigo_turma`, `codigo_academia` e `type`.
- `GET /aprovacoes` → apenas aprovados (`aprovado = TRUE`) com os mesmos filtros.
- `GET /reprovacoes` → apenas reprovados (`aprovado = FALSE`) com os mesmos filtros.
- `GET /academia/avaliacao-final/regras` → lista regras da academia autenticada.
- `PUT /academia/avaliacao-final/regras/:id` → edita apenas `nome`, `descricao`, `nota_minima_aprovacao` e `formula`; as categorias são recalculadas pela fórmula.
- `DELETE /academia/avaliacao-final/regras/:id` → inativa a regra e suas dependentes em cadeia. A deleção é lógica, não física, para preservar histórico e snapshots de avaliações já registradas. Após a inativação, dependentes não podem ser reativadas sem o pré-requisito ativo, e regras inativas com `type` conflitante com regra ativa permanecem bloqueadas para ativação.

**Escopo por academia:** quando o usuário autenticado é academia, o backend força `codigo_academia` para a academia autenticada; não é permitido consultar dados de outra academia.

**Dependência entre filtros:** para consultas admin, o filtro `codigo_turma` exige também `codigo_academia` para garantir resolução correta da turma no contexto da academia.

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

**Remoção automática**: para avaliações finais escolares, removida. Agora há progressão de turma na aprovação e permanência na turma na reprovação.

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
| Admin FPP define diretamente, por tipo, o ano letivo oficial global via `POST /admin/definir-ano-letivo-geral`; depois é atualizado automaticamente quando todas as academias ativas do mesmo tipo estiverem alinhadas | É a referência obrigatória para todo o sistema          |
| Academia só pode definir diretamente uma vez e sempre igual ao oficial global; avanços acontecem pela finalização do ano letivo | Divergência é bloqueada com erro de negócio             |
| Sem ano letivo na academia, notas/faltas/avaliações são bloqueadas | Pré-condição para registos académicos               |
| Histórico `anos_letivos_lista` não duplica `ano_letivo`       | Atualizações repetidas do mesmo ano são ignoradas       |
| Histórico global `anos_letivos_lista` (admin) não duplica `ano_letivo` | Lista global de anos letivos mantém unicidade por ano |
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
| Ano do estudante deve pertencer à matéria   | Se `ano_escolar_fundamental` não estiver em `anos_academicos`, bloqueia   |
| Observação obrigatória na correção          | Justificativa da alteração                                    |
| Motivo obrigatório na deleção               | Para auditoria                                                |
| Duplicata bloqueada no aggregate            | Mesma combinação ano/período/matéria/tipo/categoria rejeitada |
| Nota deletada não pode ser re-registada     | Mapa de chaves não remove entradas deletadas                  |

### 6.4 Regras de Faltas

| Regra                                            | Detalhe                                                                                 |
| ------------------------------------------------ | --------------------------------------------------------------------------------------- |
| Quantidade deve ser 1 ou mais                    | Validação no handler e no aggregate                                                     |
| Data no formato date (`AAAA-MM-DD`)              | Campo date-only em faltas (sem hora)                                                    |
| Ano do estudante deve pertencer à matéria        | Se `ano_escolar_fundamental` do estudante não existir em `anos_academicos` da matéria, bloqueia    |
| Observação obrigatória na correção               | Justificativa da alteração em `PUT /academia/atualizar-falta`                           |
| Motivo obrigatório na deleção                    | Para auditoria no ledger e na projeção                                                  |
| Duplicata bloqueada                              | Mesma combinação `data + codigo_estudante + materia_disciplinar_id` é rejeitada         |

### 6.5 Regras de Avaliação Final

| Regra                                       | Detalhe                                    |
| ------------------------------------------- | ------------------------------------------ |
| Aprovação exige notas presentes na fórmula | Cada referência `[categoria,periodo]` exige nota daquele par; se faltar, a avaliação aguarda novo lançamento |
| Observação não faz override de nota | `observacao` é apenas metadado; aprovação/reprovação vem de `nota_final >= nota_minima_aprovacao` |
| Fundamental usa sequência fixa 1..9 | `1_ano_fundamental` até `9_ano_fundamental` |
| Médio usa sequência do curso | Avança conforme `anos_academicos` do curso médio ativo vinculado |
| Tipo de ensino é inferido no backend | Não deve ser enviado no payload da avaliação final |
| Superior progride por semestre | `semestre_atual` define o período avaliado (`[n]_semestre`), aprovações intermediárias incrementam o semestre e `ano_superior = ceil(semestre_atual / 2)` |
| Reprovação não altera o ano/status | Mantém nível atual e não finaliza ciclo |
| Aprovação no último nível finaliza ciclo | Define o status do ciclo correspondente como `finalizado` |
| Uma avaliação por type/ano letivo/nível | Idempotência via aggregate e projeção |
| Ativação de regra dependente | Uma regra dependente inativa só pode ser ativada se o `type` apontado em `aplica_se_reprovado_em_type` estiver ativo |
| Reativação com `type` conflitante | Regra inativa não pode ser ativada enquanto houver regra ativa com o mesmo `type`, tipo de ensino e ano acadêmico sobreposto |
| Aprovação escolar move para turma do próximo ano; reprovação escolar mantém na turma | Automático na projeção de turmas para fundamental/médio |
| Avaliação superior não altera turmas | Turmas do superior são geridas separadamente |

### 6.6 Regras de Turma

| Regra                                                                        | Detalhe                                                                                                                                                                                                                                                                                    |
| ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Código único por academia                                                    | Não pode repetir dentro da mesma academia                                                                                                                                                                                                                                                  |
| Turno: `manha`, `tarde` ou `noite`                                           | Valores fixos                                                                                                                                                                                                                                                                              |
| Edição restrita à academia dona da turma                                     |                                                                                                                                                                                                                                                                                            |
| Edição aceita apenas `nivel`, `curso_id` e `turno`                           |                                                                                                                                                                                                                                                                                            |
| Mudança de nível/curso enquanto a turma tem estudantes exige compatibilidade | Se a turma já tiver estudantes vinculados, qualquer alteração de `nivel` e/ou `curso_id` dispara revalidação de compatibilidade antes de persistir. Onde os estudantes devem ter o ano acadêmico igual à esse novo nível, ou o curso_medio_id ou curso_superior_id igual à esse novo curso |
| Deleção exige inatividade                                                    | Desativar antes de deletar                                                                                                                                                                                                                                                                 |
| Deleção exige sem estudantes                                                 | Remover todos os estudantes primeiro                                                                                                                                                                                                                                                       |

### 6.7 Regras de Matéria Disciplinar

| Regra                                                                                                | Detalhe                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Edição restrita à academia dona da matéria                                                           |                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Anos acadêmicos deve ser compatível aos anos acadêmicos da academia ou do curso                      | Ao criar ou editar anos_academicos ele deve ser compatível com os anos acadêmicos da academia (para matéria do tipo fundamental), ou com os anos acadêmicos do curso (matéria do tipo medio ou superior)                                                                                                                                                                                                                                                              |
| Tipo compativel com o nivel da academia                                                              | - Quando a academia é do nível escola e `NivelEscolar` = "fundamental", o tipo será `fundamental`.<br>- Quando a academia é do nível escola e `NivelEscolar` = "medio", o tipo será `medio`.<br>- Quando a academia é do nível superior o tipo será `superior`.<br><br>MateriaType será preenchido automaticamente, apenas quando a academia é do nível escola e `NivelEscolar` = "misto", a academia terá que enviar o tipo definindo se é `fundamental` ou `medio`. |
| Período só pode ser definido para matéria do tipo `superior`. E deve ser compatível com o seu curso. | Matérias `fundamental` e `medio` não aceitam definição de período. E o período da matéria do tipo superior deve ser compatível com um dos períodos do seu curso                                                                                                                                                                                                                                                                                                       |
| Quando a matéria é do tipo `superior` período não pode ser vazio                                     |                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| Deleção exige inatividade                                                                            | Matéria com status `ativo` é rejeitada; é obrigatório desativar antes de deletar                                                                                                                                                                                                                                                                                                                                                                                      |

### 6.8 Regras de Curso

| Regra                                                           | Detalhe                                                                                                                                                                                    |
| --------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Tipo imutável após criação e compatível com o nível da academia | Preenchido automaticamente no back end. Quando a academia é do nível escola e `NivelEscolar` = "medio", o tipo será `medio`. Quando a academia é do nível superior o tipo será `superior`. |
| Curso superior exige períodos                                   | Ao menos um semestre                                                                                                                                                                       |
| Curso superior limita semestres por ano                         | `total_semestres >= total_anos` e `total_semestres <= total_anos * 2`                                                                                                                     |
| Curso médio não deve ter períodos                               | Trimestres são fixos do sistema                                                                                                                                                            |
| Edição de anos acadêmicos bloqueia remoções em uso              | Não é permitido remover de `anos_academicos` um ano com estudantes ativos matriculados no curso                                                                                            |
| Edição de períodos do superior bloqueia remoções em uso          | Não é permitido remover de `periodos` um semestre com estudantes ativos no curso superior usando o `semestre_atual` correspondente                                                         |
| Deleção exige inatividade                                       | Desativar primeiro                                                                                                                                                                         |
| Deleção exige sem estudantes matriculados                       | Verificação antes de deletar                                                                                                                                                               |
| Matérias ativas bloqueiam deleção                               | Desativar todas as matérias antes                                                                                                                                                          |
| Cascata na deleção                                              | Matérias e turmas inativas e sem estudantes são deletadas automaticamente                                                                                                                  |

### 6.9 Regras de Admin

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

### Consulta pública de academias

As rotas `GET /academias` e `GET /consultar-academia/:codigo` são públicas com autenticação opcional. Usuários não autenticados podem consultar a lista de academias ou uma academia específica pelo código, mas a resposta expõe somente os campos públicos: `nivel`, `type`, `nome`, `codigo_academia`, `provincia`, `endereco`, `nivel_escolar` e `anos_academicos`. Para escolas fundamentais ou mistas, `anos_academicos` informa os anos acadêmicos ofertados sem exigir sessão.

As rotas `GET /academia/cursos?codigo_academia=...` e `GET /academia/curso/:id` também são públicas com autenticação opcional para consulta dos cursos e dos anos desses cursos em escolas do médio e academias do nível superior. Academias autenticadas continuam consultando os próprios cursos sem informar `codigo_academia`; admins autenticados continuam informando `codigo_academia` na listagem.

Quando a requisição envia `Authorization: Bearer <jwt_token>` válido, a API preserva o contrato autenticado anterior, retornando também campos operacionais para usuários autenticados e campos administrativos adicionais para admins. Tokens enviados em formato inválido, expirados ou pertencentes a contas inativas devem ser rejeitados com `401`.

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

### 10.1 Arquivamento de Estudante

O sistema possui o status geral `arquivado` para estudantes que saíram da academia, mas cujos registos históricos devem ser mantidos. A academia não define esse status diretamente: usa `POST /academia/estudante/:codigo/desvincular`, que registra `EstudanteDesvinculadoDaAcademia`. Para retorno do estudante, usa `POST /academia/estudante/:codigo/revincular`, que registra `EstudanteReintegrado` e reativa o vínculo.

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


---

## 11. Solicitação de Matrícula e Armazenamento

### Entidade `SolicitacaoMatricula`

A entidade representa o pedido feito pelo estudante para se matricular numa academia. Ela possui código único de 11 caracteres (`codigo_solicitacao`), dados pessoais/académicos, mapa de documentos enviados, status (`pendente`, `aprovada`, `reprovada`) e campos de decisão (`codigo_estudante_gerado`, `motivo_reprovacao`, `aprovada_por`, `reprovada_por`). Cada documento enviado é salvo como objeto com `path`, `file_url` e `download_url`, permitindo que as rotas GET retornem tanto o caminho interno quanto as URLs do arquivo e de download.

Eventos do ledger:

- `SolicitacaoMatriculaCriada`
- `SolicitacaoMatriculaAprovada`
- `SolicitacaoMatriculaReprovada`

### Processo de negócio

1. O estudante envia `POST /solicitacao-matricula` com formulário multipart e PDFs.
2. O backend valida bilhete de identidade do responsável, cédula do estudante quando necessário, data de nascimento, academia ativa, assinatura/extensão PDF, limite máximo de 5MB por ficheiro e as regras automáticas de declaração/certificados.
3. Os documentos são enviados ao storage em `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/`.
4. Para cada PDF, o storage devolve o caminho interno, a URL do arquivo (`file_url`) e a URL de download (`download_url`); esses dados são gravados no evento de criação e na projeção.
5. O aggregate `SolicitacaoMatricula` valida que `bilhete_identidade` e `bilhete_identidade_responsavel`, quando ambos informados, não sejam iguais.
6. O aggregate `SolicitacaoMatricula` grava o evento de criação.
7. A academia lista/consulta solicitações e aprova ou reprova.
8. Na aprovação, o sistema reutiliza o aggregate `Estudante` e emite `EstudanteCriadoComVinculo`.
9. Na reprovação, grava o evento de reprovação e remove o diretório dos documentos.

### Regras de negócio

- O bilhete de identidade do responsável é obrigatório para toda academia escolar e de nível superior.
- A cédula do estudante é obrigatória quando o bilhete de identidade do estudante não for enviado.
- Certificado do 6.º ano fundamental só é aplicável para matrículas do 7.º ao 9.º ano fundamental.
- Certificado do 9.º ano fundamental só é aplicável para matrículas do ensino médio.
- Certificado do ensino médio só é aplicável para matrículas do ensino superior.
- A declaração escolar é obrigatória quando o certificado aplicável não for enviado ou quando não houver certificado aplicável ao ano académico informado.
- Arquivos devem ser PDFs (`Content-Type`, extensão e assinatura `%PDF`).
- Apenas a academia dona pode aprovar/reprovar.
- Solicitação decidida não volta para pendente.
- Rebuild inclui `solicitacoes_matricula` após as projeções principais.

### Armazenamento de arquivos (Google Drive)

O backend usa a biblioteca oficial `google.golang.org/api/drive/v3` integrada com `golang.org/x/oauth2/google` para autenticar-se como service account. A autenticação é feita a partir do ficheiro JSON da service account (`GOOGLE_DRIVE_CREDENTIALS_PATH` ou `GOOGLE_DRIVE_CREDENTIALS_JSON` em base64), com renovação automática de tokens OAuth — sem necessidade de gestão manual de tokens.

Configuração de produção:

- `GOOGLE_DRIVE_CREDENTIALS_PATH`: caminho para o ficheiro JSON da service account.
- `GOOGLE_DRIVE_CREDENTIALS_JSON`: alternativa em base64 para ambientes sem disco persistente.
- `GOOGLE_DRIVE_ROOT_FOLDER_ID`: ID da pasta raiz no Drive partilhada com a service account.

Remover: `GOOGLE_DRIVE_ACCESS_TOKEN` (deixa de existir).

Configuração local/teste:

- `GOOGLE_DRIVE_QUOTA_LOCAL_ESTIMATE=true`: habilita o provider local sem chamar Google Drive.
- `GOOGLE_DRIVE_LOCAL_ROOT`: diretório local usado para simular o Drive (padrão `data/google_drive_storage`).

Os documentos de matrícula continuam sendo gravados em `{codigo_academia}/matriculas/matricula_{codigo_solicitacao}/`. `EnsureDir` cria a hierarquia de pastas no Drive verificando cada nível antes de criá-lo, `Upload` envia PDFs para a pasta pai resolvida e retorna metadados do arquivo armazenado. No Google Drive, esses metadados vêm de `webViewLink` (`file_url`) e `webContentLink` (`download_url`), além do `path` interno; no provider local de teste, as URLs usam `file://`. `Delete` remove arquivos ou diretórios resolvendo o caminho dentro da pasta raiz configurada. `GetQuota` não tenta mais estimar consumo fora da pasta raiz compartilhada/gerenciada: ele lista recursivamente apenas essa pasta, define `total_bytes`/`used_bytes` como a soma real dos arquivos existentes nela, preenche `academias`/`managed_bytes` com arquivos dentro dos diretórios de academia e preenche `outside_academias_bytes` com arquivos que estão na raiz ou fora de diretórios de academia. `unmanaged_bytes` fica reservado para compatibilidade e não representa mais consumo externo à pasta raiz. Falhas de configuração retornam mensagens operacionais explícitas para ausência de credenciais, ausência de `GOOGLE_DRIVE_ROOT_FOLDER_ID`, credencial inválida e quota indisponível sem credenciais/estimativa local.

### Permissões

|Ação|Quem pode|
|---|---|
|Criar solicitação|Público|
|Listar/consultar solicitações da academia|Academia dona|
|Aprovar/reprovar|Academia dona|
|Listar todas|Admin|
|Configurar documentos obrigatórios|Academia dona|
|Consultar quota de storage|Admin|

---

## 12. Anos letivos escolar/superior

O ano letivo global e o ano letivo da academia passam a separar o identificador evolutivo (`YYYY_YYYY`) das configurações estáveis por tipo:

- `escolar`: fundamental e médio. O alias legado `escola` não é mais aceito para `type` de ano letivo.
- `superior`: ensino superior.

Cada tipo possui um único `periodo` configurado por Admin FPP no formato `MM_MM`. Esse período não é recriado a cada virada de ano; ele é combinado com o `ano_letivo` ativo para calcular o intervalo real aceito nas operações de faltas.

As academias podem declarar a finalização de um ano letivo por tipo. Essa ação é registrada no ledger por meio do evento `AnoLetivoAcademiaFinalizado` e projetada em `projection_anos_letivos_academia_finalizacoes`, mantendo a informação auditável por academia, tipo, ano, usuário, data e observação. A finalização, que também define automaticamente o ano letivo seguinte da academia, só é aceita na janela operacional entre o mês de fim do período letivo configurado para o tipo e o mês imediatamente anterior ao mês de início do próximo período: em termos de validação, o mês atual precisa ser `>=` ao mês final de `periodo` e `<` ao mês inicial de `periodo`. Exemplo: com `periodo=10_07`, a academia pode finalizar em julho, agosto ou setembro (meses 07, 08 e 09); de outubro a junho a operação é bloqueada, porque o ano letivo ainda está em curso ou o próximo período já começou. Quando todas as academias ativas do mesmo tipo ficam alinhadas no mesmo ano letivo após esses avanços, a plataforma atualiza automaticamente esse valor como o ano letivo global daquele tipo. Escolas nunca bloqueiam nem avançam o calendário global do superior, e o superior nunca bloqueia nem avança o calendário global escolar.
