---
modificado: 28-06-2026 17:10
criado: 05-04-2026 13:01
---
Versão atual: 2.0.8
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
| `superior` | Total de semestres informado como número na API; backend deriva `1_semestre` até `N_semestre` | Calculados pelo backend no formato `[n]_ano_superior` |

Para cursos superiores, a criação recebe `periodos` como número inteiro positivo (quantidade total de semestres) e não aceita `anos_academicos` no payload. A rota cadastral de edição `PUT /academia/curso/:id/dados` não aceita manipular `periodos`, `semestres` nem `anos_academicos`; ela fica restrita a dados cadastrais. O backend persiste os semestres sequenciais no formato `[n]_semestre` e calcula os anos acadêmicos com `ceil(periodos / 2)`. Ex.: `periodos = 3` deriva `periodos = ["1_semestre", "2_semestre", "3_semestre"]` e `anos_academicos = ["1_ano_superior", "2_ano_superior"]`.

Na criação de cursos médios, `POST /academia/curso` aplica a mesma proteção de sequência de anos médios usada por `POST /academia/anos-academicos`: `anos_academicos` deve começar em `1_ano_medio`, seguir em ordem crescente, não pular posições e não repetir anos. Assim, cargas como `["2_ano_medio"]`, `["1_ano_medio", "3_ano_medio"]` ou `["2_ano_medio", "1_ano_medio"]` são rejeitadas antes da criação.

**Formato dos semestres persistidos**: `[n]_semestre` onde n ≥ 1 (ex: `1_semestre`, `2_semestre`).

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
|`superior`|**`inativo`**|Exatamente 1 ano no formato `[n]_ano_superior`; curso_id e periodo obrigatórios|

**Matérias superiores nascem inativas**: exigem que `periodo` seja enviado no `POST /academia/materia`, válido dentro dos períodos do curso vinculado. O período não pode mais ser editado depois da criação, e a rota `PUT /academia/materia/:id/periodo` foi removida.

**Estados:** `ativo` / `inativo` / `deletado`

**Eventos:** `MateriaCriada`, `MateriaAtivada`, `MateriaDesativada`, `MateriaDadosAtualizados`, `MateriaPeriodoDefinido`, `MateriaDeletada`. Para matérias superiores novas, `MateriaPeriodoDefinido` é emitido junto da criação para registrar o período inicial obrigatório.

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

1. Academia envia os dados do estudante em `multipart/form-data`, com ou sem anexos.
2. Sistema mantém obrigatórias as validações cadastrais e acadêmicas, mas não bloqueia o cadastro direto pela ausência de PDFs.
3. Sistema valida que todos os arquivos enviados são PDF, respeitam o limite de 5MB e possuem assinatura `%PDF`.
4. Sistema gera código único (`AAA1234`), verificando ledger e projeção.
5. Quando enviados, os documentos são enviados ao storage definitivo em `{codigo_academia}/estudantes/{codigo_estudante}/documentos/`.
6. Senha padrão = código do estudante (ex: `ABC1234`).
7. Estudante é criado com **status `ativo`**, vinculado à academia e com o mapa `documentos` gravado no evento `EstudanteCriadoComVinculo` e na projeção.
8. Se qualquer validação ou persistência falhar após upload parcial, o diretório de documentos do estudante é removido para evitar ficheiros órfãos.

**Regras de validação:**

- `genero` obrigatório: `masculino` ou `feminino`
- `data_nascimento` obrigatório: deve ser anterior à data atual
- JSON puro não é aceito no cadastro direto; o fluxo deve usar `multipart/form-data`, mesmo quando nenhum anexo for enviado
- `bilhete_identidade_responsavel` continua obrigatório para estudantes escolares/fundamental/médio; o PDF `bi_responsavel` é opcional no cadastro direto
- `bilhete_identidade` e `bilhete_identidade_responsavel`, quando ambos informados, não podem ser iguais após normalização
- `bi_estudante` e `cedula_estudante` são opcionais no cadastro direto; quando enviados, precisam ser PDFs válidos
- o BI do responsável não pode coincidir com o BI principal de outro estudante escolar/fundamental/médio, mas pode repetir como BI de responsável de irmãos/outros estudantes
- Certificados acadêmicos e `declaracao` são opcionais no cadastro direto; quando enviados, precisam ser PDFs válidos
- `ano_escolar_fundamental` deve seguir o formato canônico para o tipo de ensino
- Se informar `curso_medio_id`, o curso deve existir, estar ativo, pertencer à academia e ser do tipo `medio`
- Se informar `curso_superior_id`, o curso deve existir, estar ativo, pertencer à academia e ser do tipo `superior`
- Status inicial padrão para fundamental: `em_andamento`
- Status inicial padrão para médio e superior: `inativo` até eventos específicos de matrícula/curso

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

**Quem faz**: Academia ativa, com ano letivo configurado, por meio da configuração de regras e do lançamento de notas. A academia **não envia manualmente** a nota final calculada nem decide aprovação/reprovação no payload de execução.

A avaliação final é automática, auditável e orientada por regras. Ela é disparada pelo fluxo de lançamento de notas quando o backend identifica que existem regras ativas e notas suficientes para calcular a etapa aplicável. O modelo atual **não é uma média global única do estudante**: o backend calcula uma `nota_final` independente para cada matéria disciplinar aplicável, registra resultados por matéria e deriva a decisão geral do conjunto de resultados, da cadeia de regras e, apenas para Médio/Superior, das regras de pendência.

#### 5.6.1 Conceitos funcionais

| Conceito | Significado funcional |
|---|---|
| Regra raiz | Regra ativa sem `aplica_se_reprovado_em_type`. É a primeira etapa da cadeia para uma academia, `nivel` e escopo. Deve existir no máximo uma raiz ativa por escopo aplicável. |
| Regra descendente | Regra ativa com `aplica_se_reprovado_em_type`, executada somente depois de reprovação no `type` indicado. Modela recuperação, exame, recurso ou outra nova chance. |
| `type` | Nome público da etapa (`avaliacao_final`, `avaliacao_final_com_exame`, `avaliacao_final_com_recurso`, etc.). É configurado na regra, não enviado para executar avaliação. |
| `nivel` | Campo público de escopo da regra: `fundamental`, `medio` ou `superior`. O contrato novo de regras não aceita `tipo_ensino`. |
| Fórmula | Expressão textual declarativa validada por parser próprio. Calcula a nota final de uma matéria usando notas existentes. Não há `eval`, script, template executável nem código dinâmico. |
| `nota_minima_aprovacao` | Nota mínima para aprovar cada matéria avaliada na etapa. |
| Matérias avaliadas | Matérias ativas da academia que pertencem ao nível/curso/ano/período do estudante e, se houver, ao filtro `materias_aplicaveis` da regra. |
| `materias_chave` | Lista obrigatória na regra raiz do Médio. Na implementação atual, reprovação em matéria não-chave na raiz não impede aprovação direta; reprovação em matéria-chave impede e pode acionar cadeia/pendência. |
| `materias_aplicaveis` | Lista opcional de matérias que restringe a execução da regra. É especialmente útil em descendentes para recalcular apenas matérias de recuperação/exame/recurso. |
| `limite_materias_pendentes` | Inteiro obrigatório e não negativo para Médio/Superior. Define quantas reprovações finais podem virar pendência. Não existe no Fundamental. |
| `pendencia_permitida` | Campo da matéria de Médio/Superior. Somente matérias com esse campo verdadeiro podem gerar aprovação com pendência. |
| `pendencia_nivel_conclusao` | Campo da matéria de Médio/Superior que indica o nível/semestre de conclusão usado para bloqueio funcional de progressão/conclusão quando há pendência aberta. |
| Matéria pendente | Registro persistente em `projection_materias_pendentes`, criado quando uma avaliação de Médio/Superior aprova com pendência. Mantém histórico aberto/baixado por estudante, matéria, curso, ano letivo e escopo. |

A avaliação registrada é idempotente no escopo suportado: o sistema evita gravar duas avaliações com o mesmo estudante, academia, ano letivo, nível interno da avaliação, ano/período acadêmico atual e `type`. Eventos e projeções preservam snapshots de fórmula, regra, matérias e pendências suficientes para auditoria.

#### 5.6.2 Montagem e criação de regras de avaliação final

A academia monta uma cadeia declarando uma regra raiz e, opcionalmente, regras descendentes. O endpoint de criação usa `nivel` como campo público; `tipo_ensino` é legado e é rejeitado em criação/edição de regras.

**Preenchimento/validação de `nivel` pela academia autenticada:**

| Academia autenticada | Comportamento |
|---|---|
| Superior | O backend força `nivel="superior"`. Se o payload informar outro nível, falha. |
| Escola fundamental | O backend aceita omissão ou `nivel="fundamental"`; outro nível falha. |
| Escola média | O backend aceita omissão ou `nivel="medio"`; outro nível falha. |
| Escola mista | O payload deve informar explicitamente `fundamental` ou `medio`. Escola mista não cria regra `superior`. |

**Campos por nível:**

| Campo | Fundamental | Médio | Superior |
|---|---:|---:|---:|
| `nivel` | `fundamental` | `medio` | `superior` |
| `anos_academicos` | Obrigatório e não vazio; deve conter anos fundamentais válidos | Rejeitado | Rejeitado |
| `materias_chave` | Rejeitado | Obrigatório na regra raiz; opcional/dispensável em descendentes | Não é campo funcional obrigatório |
| `materias_aplicaveis` | Opcional para limitar matérias | Opcional para limitar matérias | Opcional para limitar matérias |
| `limite_materias_pendentes` | Rejeitado | Obrigatório, `>= 0` | Obrigatório, `>= 0` |
| `aplica_se_reprovado_em_type` | Ausente na raiz; presente em descendente | Ausente na raiz; presente em descendente | Ausente na raiz; presente em descendente |

**Regras de cadeia e unicidade:**

- `type`, `nome`, `nivel`, `formula` e `nota_minima_aprovacao > 0` são obrigatórios na criação; `descricao` é opcional.
- `type` aceita letras, números, espaços e `_`; espaços internos são normalizados para `_`.
- `categorias_envolvidas` é extraído da `formula`; se enviado, deve bater exatamente com as categorias extraídas.
- Não pode haver duas regras ativas com o mesmo `codigo_academia`, `nivel`, `type` e escopo fundamental sobreposto. Médio/Superior não usam `anos_academicos` na regra.
- Deve haver no máximo uma raiz ativa por academia, `nivel` e escopo. Uma cadeia sem raiz ou com múltiplas raízes aplicáveis não é executável de forma determinística.
- Descendentes devem apontar para regra ativa existente no mesmo `nivel`, não podem apontar para si mesmas, não podem criar ciclo e, no Fundamental, devem usar exatamente os mesmos `anos_academicos` da raiz.
- Inativar uma regra inativa também suas dependentes diretas/indiretas para preservar a consistência da cadeia.
- Na edição, não é permitido alterar `type`, `nivel`, `anos_academicos`, dependência, `materias_chave`, `materias_aplicaveis`, limite, status ou version; para mudar escopo/cadeia, cria-se nova regra.

**Exemplos funcionais de configuração:**

| Cenário | Configuração típica |
|---|---|
| Raiz do Fundamental | `nivel=fundamental`, `anos_academicos=["6_ano_fundamental"]`, fórmula com trimestres explícitos, sem limite de pendência. |
| Recuperação do Fundamental | `nivel=fundamental`, mesmo `anos_academicos` da raiz, `aplica_se_reprovado_em_type="avaliacao_final"`, fórmula da recuperação/exame. |
| Raiz do Médio | `nivel=medio`, sem `anos_academicos`, `materias_chave` com matérias obrigatórias do curso/ano, `limite_materias_pendentes` definido. |
| Descendente do Médio | `nivel=medio`, `aplica_se_reprovado_em_type` apontando para a raiz, `materias_aplicaveis` com as matérias que podem ser recalculadas no exame. |
| Raiz do Superior | `nivel=superior`, sem `anos_academicos`, fórmula com referências `[categoria]`, `limite_materias_pendentes` definido. |
| Superior com pendência | Mesma regra superior, matérias com `pendencia_permitida=true` e, quando aplicável, `pendencia_nivel_conclusao` indicando o semestre-limite. |

#### 5.6.3 Fórmulas por nível

A regra usa `formula` como texto declarativo. O parser valida referências, operadores, parênteses, constantes, categorias e períodos antes de persistir ou calcular. Erro de fórmula é erro de validação, não falha operacional inesperada.

| Nível | Formato da referência | Exemplo válido | Exemplo inválido |
|---|---|---|---|
| Fundamental | `[categoria,periodo]` | `([nota_escola,1_trimestre]+[nota_escola,2_trimestre]+[nota_escola,3_trimestre])/3` | `[nota_escola]` |
| Médio | `[categoria,periodo]` | `[nota_escola,1_trimestre]*0.4+[nota_exame,3_trimestre]*0.6` | `[nota_exame]` |
| Superior | `[categoria]`; período inferido pela matéria/semestre avaliado | `([nota_pp1]+[nota_pp2])/2` | `[nota_pp1,1_semestre]` |

No Superior, o backend preenche o período no momento do cálculo usando a matéria/escopo avaliado (`periodo` da matéria e `semestre_atual` do estudante). Assim, a mesma regra superior pode calcular as matérias do semestre atual sem expor período explícito no payload da regra.

Se a fórmula exigir nota que ainda não existe para determinada matéria, categoria e período, aquela execução não fecha a avaliação naquele momento; o lançamento de novas notas tentará novamente. A fórmula sempre lê notas do ano letivo atual, da mesma academia, do mesmo estudante, da matéria avaliada e de categorias extraídas da própria fórmula.

#### 5.6.4 Execução automática por lançamento de notas

1. A academia registra/atualiza nota; o backend valida ano letivo, estudante, matéria, categoria, período e pertencimento.
2. O backend infere o nível acadêmico do estudante para execução: Superior tem prioridade quando há vínculo/status superior; depois Médio; caso contrário Fundamental.
3. Para Superior, o backend transforma `semestre_atual` em `[n]_semestre` e valida esse período contra o curso.
4. O backend busca regras ativas aplicáveis à academia, ao `nivel` e ao escopo acadêmico atual.
5. A cadeia começa na raiz. Descendentes só são consideradas se a etapa anterior reprovou.
6. Para cada regra executável, o backend resolve as matérias aplicáveis e calcula `nota_final` individual por matéria.
7. O resultado de cada matéria compara `nota_final` com `nota_minima_aprovacao`.
8. A decisão geral é derivada dos resultados por matéria e, no Médio/Superior, das condições de pendência.
9. O evento grava snapshots: regra, `type`, fórmula usada, nota mínima, resultados por matéria, pendências geradas, turma atual/turmas removidas quando aplicável e dados de progressão.
10. Se a avaliação já existir no escopo idempotente, o backend não duplica o registro.

#### 5.6.5 Fundamental

- O escopo é `ano_escolar_fundamental` atual do estudante (`1_ano_fundamental` a `9_ano_fundamental`).
- Regras fundamentais são as únicas que usam `anos_academicos`.
- O backend avalia cada matéria fundamental ativa aplicável ao ano do estudante, respeitando `materias_aplicaveis` se configurado.
- Cada matéria recebe `nota_final` própria; aprovação direta exige que todas as matérias avaliadas atinjam a mínima.
- Uma ou mais matérias abaixo da mínima reprovam a etapa e podem acionar regra descendente por matéria reprovada.
- Fundamental não permite aprovação com pendência: regra fundamental não tem `limite_materias_pendentes` e matérias fundamentais não aceitam `pendencia_permitida`/`pendencia_nivel_conclusao`.
- Aprovado em ano intermediário progride para o próximo ano fundamental. Se a academia não oferta o próximo ano, o evento registra o motivo `academia_sem_oferta_do_proximo_ano_academico_fundamental`, mantém o ciclo em andamento e não adiciona turma automaticamente.
- Aprovado no `9_ano_fundamental` finaliza o ciclo fundamental. Reprovado permanece no mesmo ano.

#### 5.6.6 Médio

- O escopo é o `ano_escolar_medio` atual do estudante, validado contra o curso médio ativo vinculado.
- O backend avalia matérias médias ativas do curso e ano atual; `materias_aplicaveis` restringe a lista quando informado.
- A regra raiz de Médio exige `materias_chave`. Na implementação atual, na raiz, reprovação em matéria-chave impede aprovação direta; reprovação apenas em matéria não-chave não torna a decisão geral reprovada nessa etapa, embora o resultado por matéria continue registrado.
- Descendentes podem recalcular matérias reprovadas e/ou limitadas por `materias_aplicaveis`; se uma matéria reprovada não estiver na lista da descendente, ela não é recalculada por aquela regra.
- Depois da última etapa aplicável, reprovações podem virar pendência somente se: o total de reprovações for menor ou igual a `limite_materias_pendentes` e todas as matérias reprovadas tiverem `pendencia_permitida=true`.
- Se essas condições forem satisfeitas, o evento é aprovado com `aprovado_com_pendencia=true` e gera registros em `projection_materias_pendentes`.
- Se o limite for ultrapassado, ou se alguma matéria reprovada não permitir pendência, o estudante reprova totalmente e nenhuma pendência nova é criada.
- Pendências de curso anterior permanecem históricas e não devem bloquear o curso atual; o curso salvo na pendência faz parte da decisão funcional de bloqueio.
- `pendencia_nivel_conclusao` representa o ano-limite para bloquear conclusão/progressão quando há pendência aberta do curso atual.

Cenários típicos do Médio:

| Cenário | Resultado funcional |
|---|---|
| Todas as matérias-chave aprovadas | Aprovação direta na raiz, com progressão ou conclusão conforme ano do curso. |
| Matéria não-chave reprovada na raiz | Resultado por matéria fica reprovado, mas a decisão geral da raiz pode permanecer aprovada conforme comportamento atual. |
| Matéria-chave reprovada e aprovada em descendente | A cadeia registra a nova etapa e a aprovação da descendente permite progressão/conclusão. |
| Matéria reprovada sem descendente aplicável | Decide reprovação total ou aprovação com pendência conforme limite e permissão da matéria. |
| Uma pendência dentro do limite | Aprovação com pendência; pendência aberta é criada. |
| Pendências acima do limite | Reprovação total; não cria pendências. |
| Matéria com `pendencia_permitida=false` | Reprovação total, mesmo dentro do limite numérico. |

#### 5.6.7 Superior

- O escopo é o curso superior ativo e o `semestre_atual` do estudante, convertido para `1_semestre`, `2_semestre`, etc.
- O backend avalia matérias superiores ativas do curso cujo `periodo` corresponde ao semestre atual.
- Fórmulas superiores não declaram período; o período é preenchido automaticamente para cada matéria avaliada.
- Aprovação direta exige todas as matérias avaliadas com nota final maior ou igual à mínima.
- Reprovação em matéria aciona descendentes aplicáveis; descendentes também trabalham por matéria e podem ser restringidas por `materias_aplicaveis`.
- Após esgotar a cadeia, o estudante pode aprovar com pendência se o total de reprovações couber em `limite_materias_pendentes` e todas as matérias reprovadas permitirem pendência.
- Reprovação por limite excedido ou matéria sem pendência permitida mantém o estudante no mesmo `semestre_atual` e não altera o status superior.
- Aprovação em semestre intermediário incrementa `semestre_atual` e recalcula `ano_superior`; aprovação no último semestre finaliza o ciclo superior.
- Pendência de curso anterior permanece histórica e não bloqueia o curso atual.

#### 5.6.8 Regras descendentes por matéria

Regra descendente é qualquer regra com `aplica_se_reprovado_em_type`. Ela representa uma etapa posterior da cadeia e só roda quando a etapa ascendente indicada reprovou. A descendente herda a lógica por matéria: calcula notas para matérias aplicáveis, compara cada resultado com a mínima e grava `type`/regra/fórmula usados naquela etapa.

Pontos importantes:

- A descendente não é uma média global do estudante; ela recalcula matérias no escopo da regra.
- `materias_aplicaveis` funciona como filtro: matéria fora da lista não é recalculada naquela etapa.
- A cadeia termina quando não há descendente ativa aplicável, quando a etapa anterior aprovou ou quando faltam notas para calcular a próxima etapa.
- Ao final da última etapa reprovada, Médio/Superior avaliam se a reprovação vira pendência; Fundamental sempre permanece reprovado.
- Exemplo: raiz `avaliacao_final` reprova Matemática e Física; descendente `avaliacao_final_com_exame` com `materias_aplicaveis=[Matemática]` recalcula somente Matemática. Física continua com o resultado anterior para a decisão final/pendência.

#### 5.6.9 Resultados por matéria, eventos, projeções e auditoria

Cada avaliação final gravada deve ser explicada pelos itens de `resultados_materias`, não por média global única. Cada item contém, no mínimo, `materia_id`, `nota_final`, `aprovado`, `type`, `formula_snapshot`, `regra_avaliacao_final_id` e `pendencia_permitida`. A projeção também mantém `nota_final` agregada como média dos itens calculados para compatibilidade/consulta resumida, mas a decisão funcional é por matéria.

Eventos `AvaliacaoFinalEscolar` e `AvaliacaoFinalSuperior` preservam snapshots de regra, fórmula, notas calculadas, progressão e pendências geradas. Alterações posteriores de regra, matéria ou nota não reescrevem silenciosamente decisões já registradas; ajustes exigem fluxo operacional próprio/rebuild controlado.

#### 5.6.10 Pendências de matérias

Pendências existem apenas para Médio e Superior. Elas são consideradas depois de reprovação na cadeia aplicável e só são criadas quando a decisão final é aprovação com pendência. Se o estudante reprova totalmente, nenhuma nova pendência é criada.

A pendência carrega funcionalmente: estudante, matéria, academia, curso, `nivel`, ano letivo, escopo acadêmico (`ano_escolar_medio` ou `periodo_superior`), regra/evento de origem, status `pendente`, dados de origem/snapshot e timestamps. Há proteção contra duplicidade aberta no mesmo estudante, matéria, curso, nível, ano letivo e escopo. A estrutura também possui campos de baixa (`baixada_por_event_id`, `updated_at`) para histórico, mas a documentação funcional reconhece uma limitação atual: **não há rota pública consolidada de regularização/baixa de pendência exposta nesta documentação de API**. Portanto, o sistema já persiste e consulta a base de pendências abertas/históricas, mas a regularização operacional precisa ser implementada ou conduzida por fluxo administrativo/evento específico antes de ser tratada como rotina pública.

#### 5.6.11 Bloqueio por `pendencia_nivel_conclusao` e regularização

`pendencia_nivel_conclusao` pertence à matéria e deve ser usado para identificar pendências bloqueantes do curso atual. Funcionalmente:

- No Médio, pendência aberta cujo limite coincide com ano de conclusão bloqueia conclusão automática até baixa.
- No Superior, pendência aberta cujo limite coincide com semestre/período conclusivo bloqueia conclusão automática até baixa.
- Aprovação com pendência pode permitir progressão intermediária, mas não deve permitir conclusão com pendência bloqueante do curso atual.
- Pendências não bloqueantes permitem progressão conforme regra de avaliação, desde que pertençam a escopo anterior e dentro do limite funcional definido.
- Pendências de curso anterior são históricas e não bloqueiam o curso atual.
- Regularização de pendência é diferente de avaliação final normal: deve avaliar a matéria pendente, registrar evento próprio auditável, baixar a pendência se aprovada e manter aberta se reprovada. Como limitação atual, esse fluxo ainda não está exposto como endpoint público completo; ao ser implementado, deve reutilizar os dados de origem da pendência e retomar progressão/conclusão quando não restarem pendências relevantes abertas.

#### 5.6.12 Cenários de erro e validação

Devem falhar com erro de validação ou bloqueio funcional:

- Payload de regra com `tipo_ensino`; use `nivel`.
- Academia mista criando regra sem `nivel` ou tentando criar regra `superior`.
- Academia não mista criando regra de nível incompatível com sua configuração.
- `anos_academicos` ausente em regra fundamental ou presente em Médio/Superior.
- `limite_materias_pendentes` presente no Fundamental, ausente em Médio/Superior ou negativo.
- `materias_chave` em Fundamental ou regra raiz de Médio sem `materias_chave`.
- `materias_chave`/`materias_aplicaveis` fora do escopo do curso/ano/período aplicável devem ser tratadas como configuração inválida ou ineficaz operacionalmente; QA deve validar esse cenário contra a base de matérias da academia.
- Descendente órfã, descendente que aponta para si mesma, ciclo de dependências ou escopo fundamental diferente da raiz.
- Fórmula Fundamental/Médio sem período explícito (`[categoria]`).
- Fórmula Superior com período explícito (`[categoria,periodo]`).
- Fórmula com categoria inexistente, período inválido, divisão por zero, caracteres fora da gramática ou categorias enviadas que não batem com a fórmula.
- Tentativa de criar pendência em matéria fundamental.
- Tentativa de criar duplicidade de pendência aberta no mesmo escopo.
- Tentativa de concluir/progredir em desacordo com pendência bloqueante do curso atual.

#### 5.6.13 Consultas

- `GET /avaliacoes` → registros de avaliação final, com filtros por `nivel`, ano letivo, ano/período acadêmico atual, turma, academia e `type`. O filtro legado `tipo_ensino` é rejeitado no handler atual.
- `GET /aprovacoes` → apenas aprovados (`aprovado = TRUE`) com os mesmos filtros.
- `GET /reprovacoes` → reprovações definitivas; reprovações intermediárias com descendente ativa posterior não aparecem como definitivas até a cadeia terminar.
- `GET /academia/avaliacao-final/regras` → lista regras da academia autenticada.
- `PUT /academia/avaliacao-final/regras/:id` → edita apenas campos seguros de apresentação/cálculo (`nome`, `descricao`, `nota_minima_aprovacao`, `formula`).
- `DELETE /academia/avaliacao-final/regras/:id` → inativa a regra e suas dependentes em cascata.

**Escopo por academia:** usuário autenticado como academia só consulta/gerencia dados da própria academia. Admin pode consultar de forma ampla com filtros.

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
3. `cursos`, `materias`, `categorias_nota`
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
| Sem vínculo de sumário                           | Faltas são independentes e não aceitam `sumario_id` ou `sumario_titulo` |

### 6.4.1 Remoção de Sumários/Aulas

O sistema não possui mais a entidade sumário/aula. As faltas devem ser lançadas e consultadas sem `sumario_id`, `sumario_titulo` ou qualquer vínculo equivalente.

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
| Reativação com `type` conflitante | Regra inativa não pode ser ativada enquanto houver regra ativa com o mesmo `type`, `nivel` e escopo acadêmico sobreposto |
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

O sistema possui o status geral `arquivado` para estudantes que saíram da academia, mas cujos registos históricos devem ser mantidos. A academia não define esse status diretamente: usa `POST /academia/estudante/:codigo/desvincular`, que registra `EstudanteDesvinculadoDaAcademia`. Para retorno do estudante, usa `POST /academia/estudante/:codigo/revincular`, que registra `EstudanteReintegrado` e reativa o vínculo. A revinculação não aceita ano/semestre definido pelo cliente; o backend encontra a posição em que o estudante parou a partir do histórico. Ela diferencia retorno ao mesmo curso de mudança real de curso: no mesmo curso o aggregate mantém a posição acadêmica anterior (`ano_escolar_fundamental`, `ano_escolar_medio`, `semestre_atual` e `ano_superior`); ao mudar de curso médio reinicia em `1_ano_medio`; ao mudar de curso superior reinicia em `1_semestre`/`1_ano_superior`. Trancamento, interrupção, desvinculação e reativação alteram vínculo/status operacional, mas não zeram progressão nem removem histórico acadêmico, financeiro ou de auditoria.

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
6. Para estudantes escolares/fundamental/médio, o handler e o aggregate exigem `bilhete_identidade_responsavel`, `bi_responsavel`, `bi_estudante` quando houver BI próprio, `cedula_estudante` quando não houver BI próprio, e certificado aplicável ou `declaracao`.
7. Antes de criar ou aprovar a solicitação escolar, o handler confirma que o BI do responsável não pertence como BI principal a outro estudante escolar/fundamental/médio já existente.
8. O aggregate `SolicitacaoMatricula` grava o evento de criação.
9. A academia lista/consulta solicitações e aprova ou reprova.
10. Na aprovação, o sistema reutiliza o aggregate `Estudante`, revalida os documentos e conflitos atuais, e emite `EstudanteCriadoComVinculo`.
11. Na reprovação, grava o evento de reprovação e remove o diretório dos documentos.

### Regras de negócio

- O bilhete de identidade do responsável é obrigatório para estudantes escolares/fundamental/médio.
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

---

## Atualização 2.0.7 — Gestão segura de anos acadêmicos por academias

Academias agora podem consultar, adicionar e desabilitar escopos acadêmicos habilitados via `/academia/anos-academicos`, sem substituição em massa. O contrato mantém a separação entre **ano acadêmico/período** e **ano letivo/calendário**.

- **Fundamental/misto**: a lista ativa continua na academia (`projection_academias.anos_academicos`) e aceita somente códigos canônicos `[1-9]_ano_fundamental`.
- **Médio**: a lista ativa pertence ao curso médio (`projection_cursos.anos_academicos`) e requer `curso_id`, evitando colisão com anos do fundamental em escolas mistas.
- **Superior**: a academia informa somente `periodos` numérico no curso superior; semestres (`[n]_semestre`) e anos superiores (`[n]_ano_superior`) seguem derivados pelo backend.
- **Segurança**: cada alteração valida propriedade da academia, compatibilidade entre `type` e nível/curso, e bloqueia reduções que afetariam estudantes ativos no ano ou semestre removido.
- **Preservação histórica**: remoções são lógicas/prospectivas; eventos, ledger, histórico acadêmico, turmas, matérias, notas, faltas, avaliações finais já registrados não são apagados nem reprocessados.
- **Contratos explícitos na API**: a documentação da API detalha `GET`, `POST` e `DELETE /academia/anos-academicos` com funcionamento, permissões, payloads por `type`, respostas de sucesso e erros esperados.
- **Leitura por admin**: admins usam `GET /academia/anos-academicos?codigo_academia=...`; as rotas de escrita permanecem exclusivas para academias autenticadas e ativas.
- **Erros acionáveis**: as respostas de erro dessas rotas usam o envelope padrão com `details[]` contendo `field`, `code` e `message`, inclusive em conflitos `409` causados por estudantes ativos, para indicar exatamente o campo problemático, o motivo e a correção esperada.

## Atualização 2.0.8 — Progressão fundamental sem oferta do próximo ano

A avaliação final do fundamental agora diferencia conclusão real do ciclo e ausência operacional de oferta na academia. Quando o estudante é aprovado em um ano anterior ao `9_ano_fundamental`, o backend calcula o próximo ano global canônico e verifica se ele existe em `projection_academias.anos_academicos` da academia atual. Se não existir, o estudante avança para esse próximo ano global, permanece com `status_escolar_fundamental = "em_andamento"`, continua vinculado à mesma academia, não recebe turma automática e o evento registra `motivo_progressao = "academia_sem_oferta_do_proximo_ano_academico_fundamental"`. A aprovação no `9_ano_fundamental` continua finalizando o fundamental normalmente.
