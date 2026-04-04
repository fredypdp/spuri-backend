---
modificado: 04-04-2026 02:10
criado: 07-03-2026 00:12
---
## 1. Visão Geral da Arquitetura

O Spuri Backend é uma API REST em **Go** que usa **Event Sourcing + CQRS**.

**Regra central e inviolável:** toda mutação de estado passa obrigatoriamente pelo ledger (`spuri_ledger`) antes de atualizar qualquer projeção. Não existe `UPDATE` ou `INSERT` direto em tabelas de projeção que bypasse o ledger.

**Fluxo de escrita:**

```
HTTP Request → Handler → Aggregate (comando) → RaiseEvent → Apply (muta estado em memória)
    → Repository.SaveWithAudit → Ledger (spuri_ledger) → Projection Handler → Tabela de projeção
```

**Fluxo de leitura:**

```
HTTP Request → Handler → Projection (consulta direta no banco) → Resposta
```

**Componentes principais:**

- `internal/domain/aggregates/` — lógica de negócio, validações, eventos
- `internal/db/` — repositório, event store, hash chain, whitelist de eventos
- `internal/projections/` — projeções de leitura (read models)
- `internal/handlers/` — endpoints HTTP (Gin)
- `internal/handlers/batch_handlers.go` — endpoints de operação em lote
- `internal/handlers/batch_context.go` — utilitário de contexto sintético para batch
- `internal/middleware/` — autenticação JWT, autorização por role/tipo
- `migrations/` — SQL de schema e dados iniciais

---

## 2. Entidades e Aggregates

### 2.1 Admin

**Responsabilidade:** Gerenciar administradores do sistema. Único aggregate que pode ativar academias.

**Campos do aggregate:**

|Campo|Tipo|Descrição|
|---|---|---|
|`ID`|`uuid.UUID`|Identificador único|
|`Nome`|`string`|Nome completo|
|`Email`|`string`|Email único (regex validado)|
|`SenhaHash`|`string`|Hash bcrypt (mínimo 60 chars)|
|`Role`|`string`|`fpp` / `adm` / `gerente`|
|`Status`|`string`|`ativo` / `inativo`|
|`EmailVerificado`|`bool`|Se o email foi verificado|
|`CreatedBy`|`*uuid.UUID`|`nil` para o primeiro admin (bootstrap)|
|`CreatedAt`|`time.Time`|Data de criação|
|`ActivatedBy`|`uuid.UUID`|Quem ativou|
|`ActivatedAt`|`time.Time`|Quando foi ativado|
|`DeactivatedBy`|`uuid.UUID`|Quem desativou|
|`DeactivatedAt`|`time.Time`|Quando foi desativado|
|`TotalAcoesRealizadas`|`int`|Contador de ações registradas|

**Hierarquia de roles (ordem de poder):**

```
fpp (3) > adm (2) > gerente (1)
```

Um admin só pode gerenciar admins de role **estritamente inferior** ao seu. A hierarquia é mantida via mapa `adminHierarchy` com valores inteiros — nunca comparação de string direta.

**Regras de negócio:**

- O primeiro admin (`fpp`) só pode ser criado via `POST /bootstrap`, com advisory lock PostgreSQL para evitar race condition. Após existir qualquer admin, esse endpoint retorna 403.
- Apenas `fpp` pode alterar roles de outros admins.
- Um admin inativo não pode executar comandos.
- Email deve ser único e com formato válido.
- Senha deve ser bcrypt (mínimo 60 caracteres no hash).
- O hash bcrypt da senha é gravado no payload dos eventos `AdminCriado` e `AdminSenhaAlterada` no ledger. Ver decisão de design na seção 5.

**Eventos emitidos:**

|Evento|Quando|
|---|---|
|`AdminCriado`|Novo admin cadastrado|
|`EmailVerificado`|Email verificado via token|
|`AdminAtivado`|Admin ativado por outro admin|
|`AdminDesativado`|Admin desativado|
|`AcaoAdminRegistrada`|Ação administrativa registrada|
|`AdminDadosAtualizados`|Nome ou email alterados|
|`AdminRoleAtualizado`|Role alterado (somente por fpp)|
|`AdminSenhaAlterada`|Senha alterada|

---

### 2.2 Academia

**Responsabilidade:** Representa uma instituição de ensino que gerencia estudantes, cursos, matérias, turmas, notas e faltas.

**Campos do aggregate:**

|Campo|Tipo|Descrição|
|---|---|---|
|`ID`|`uuid.UUID`|Identificador único|
|`Type`|`string`|`escola` / `superior`|
|`Nome`|`string`|Nome da instituição|
|`CodigoAcademia`|`string`|Código único gerado no formato `{PROV}{ANO}{SEQ}` (ex: `LDA20261`)|
|`SenhaHash`|`string`|Hash bcrypt|
|`Provincia`|`string`|Código de 3 letras da província angolana (ex: `LDA`, `BGU`)|
|`Endereco`|`string`|Endereço físico|
|`NumeroTelefone`|`*string`|Telefone (opcional)|
|`Email`|`*string`|Email (opcional)|
|`EmailVerificado`|`bool`|Se o email foi verificado|
|`Website`|`*string`|Website (opcional)|
|`NivelEscolar`|`*string`|`fundamental` / `medio` / `misto` — obrigatório para `type=escola`, `nil` para `type=superior`|
|`AnosAcademicos`|`[]string`|Anos do ensino fundamental oferecidos (obrigatório quando `nivel_escolar=fundamental` ou `misto`)|
|`Status`|`string`|`ativo` / `inativo`|
|`Cursos`|`[]string`|Lista de nomes de cursos|
|`CategoriasNota`|`[]string`|Categorias de nota adicionadas pela academia (estado em memória para dedup)|
|`AtivadoPor`|`uuid.UUID`|Admin que ativou|
|`AtivadoEm`|`time.Time`|Data de ativação|
|`DesativadoPor`|`uuid.UUID`|Admin que desativou|
|`DesativadoEm`|`time.Time`|Data de desativação|
|`AnoLetivo`|`*string`|Ano letivo ativo da academia (ex: `2025_2026`). `nil` = não configurado|
|`TipoAnoLetivo`|`*string`|Tipo do ano letivo: `escola` ou `superior`|
|`AnoLetivoAtivadoEm`|`*time.Time`|Data/hora da última definição do ano letivo|
|`AnoLetivoAtivadoPor`|`*uuid.UUID`|UUID da academia que definiu o ano letivo|

**Código da academia:**

O código é gerado pela função SQL `spuri_generate_codigo_academia` (migration 035/045) no formato `{PROV}{ANO}{SEQ}`. A função consulta o `spuri_ledger` (não a projeção) para garantir unicidade mesmo em cadastros simultâneos. Exemplo: `LDA20261`, `LDA20262`, `BGU20261`.

**Regras de negócio:**

- Academias são criadas com `status = inativo`. Só um Admin pode ativar.
- `nivel_escolar = medio` não deve ter `anos_academicos`.
- `nivel_escolar = fundamental` ou `misto` deve ter `anos_academicos` validados no formato `[1-9]_ano_fundamental`.
- Apenas academias com `status = ativo` podem operar (middleware `ValidarStatusAcademia`).
- `CategoriasNota` no aggregate é necessário para detectar duplicatas sem depender da projeção.
- Categorias de nota podem ser adicionadas por academias de **qualquer tipo** (`escola` ou `superior`).
- **O ano letivo é definido por cada academia individualmente** via `POST /academia/ano-letivo`. Sem ano letivo ativo (`AnoLetivo == nil`), nenhum registro de nota, falta ou avaliação final é permitido — o handler bloqueia com 400 antes de qualquer processamento via `resolverAnoLetivoAcademia`.
- Província é armazenada como código de 3 letras; o handler converte o nome completo para código antes de passar ao aggregate.

**Eventos emitidos:**

|Evento|Quando|
|---|---|
|`AcademiaCriada`|Nova academia cadastrada|
|`AcademiaAtivada`|Ativada por admin|
|`AcademiaDesativada`|Desativada por admin|
|`AcademiaDadosAtualizados`|Dados cadastrais atualizados|
|`CursosAtualizados`|Lista de cursos alterada|
|`EmailVerificado`|Email verificado via token|
|`AcademiaSenhaAlterada`|Senha alterada via event sourcing|
|`CategoriaNotaAdicionada`|Nova categoria de nota adicionada|
|`AnoLetivoAcademiaDefinido`|Ano letivo ativo definido ou atualizado pela academia|

---

### 2.3 Estudante

**Responsabilidade:** Representa um estudante com seus dados pessoais, acadêmicos e status de ensino.

**Campos do aggregate:**

|Campo|Tipo|Descrição|
|---|---|---|
|`ID`|`uuid.UUID`|Identificador único|
|`Nome`|`string`|Nome completo|
|`CodigoEstudante`|`string`|Código único no formato `AAA1234` (3 letras + 4 dígitos)|
|`SenhaHash`|`string`|Hash bcrypt|
|`Email`|`*string`|Email (opcional)|
|`Telefone`|`*string`|Telefone (opcional)|
|`BilheteIdentidade`|`*string`|BI do próprio estudante|
|`BilheteIdentidadeResp`|`*string`|BI do responsável|
|`Genero`|`string`|`masculino` / `feminino` — **obrigatório no cadastro**|
|`DataNascimento`|`time.Time`|Data de nascimento — **obrigatória no cadastro**; deve ser anterior à data atual|
|`CodigoAcademia`|`*string`|Código da academia vinculada|
|`Status`|`string`|`inativo` / `ativo` / `finalizado`|
|`StatusEscolarFundamental`|`string`|`inativo` / `em_andamento` / `finalizado`|
|`StatusEscolarMedio`|`string`|`inativo` / `em_andamento` / `finalizado`|
|`StatusSuperior`|`string`|`inativo` / `em_andamento` / `finalizado`|
|`AnoEscolar`|`*string`|Ano atual no fundamental|
|`AnoEscolarMedio`|`*string`|Ano atual no médio|
|`AnoSuperior`|`*string`|Ano atual no superior|
|`CursoMedioID`|`*uuid.UUID`|Curso de médio vinculado|
|`CursoSuperiorID`|`*uuid.UUID`|Curso de superior vinculado|
|`EmailVerificado`|`bool`|Se o email foi verificado|
|`AvaliacoesPorAno`|`map[string]bool`|Mapa de idempotência para avaliações finais. Chave: `"<tipoEnsino>_<anoLectivo>_<anoAcademicoAtual>"`|
|`NotasRegistradasPorChave`|`map[string]bool`|Mapa de idempotência para notas|
|`FaltasRegistradasPorChave`|`map[string]bool`|Mapa de idempotência para faltas|

**Modo de criação:**

Existe apenas **um modo de criação**: `CriarComVinculo` — cadastro direto pela academia, já vinculando ao `CodigoAcademia`. O auto-cadastro público foi removido (migration 024).

**Formato dos anos (canônico):**

|Ciclo|Formato|Exemplos|
|---|---|---|
|Fundamental|`[1-9]_ano_fundamental`|`1_ano_fundamental`, `9_ano_fundamental`|
|Médio|`[n]_ano_medio`|`1_ano_medio`, `2_ano_medio`|
|Superior|`[n]_ano_superior`|`1_ano_superior`, `4_ano_superior`|

A validação é feita pelas funções `ValidateAnoFundamental`, `ValidateAnoMedio`, `ValidateAnoSuperior` em `internal/utils/validation.go`.

**Regras de negócio — Status de ensino:**

A ordem de progressão é: `Fundamental → Médio → Superior`.

- `StatusSuperior` só pode avançar para `em_andamento` ou `finalizado` se `StatusEscolarFundamental` e `StatusEscolarMedio` estiverem `finalizado` ou `inativo`.
- Somente a academia pode alterar o status escolar do estudante.
- Os três status representam trajetórias paralelas — não consolidados num único campo.

**Eventos emitidos:**

|Evento|Quando|
|---|---|
|`EstudanteCriadoComVinculo`|Cadastro pela academia|
|`DadosPessoaisAtualizados`|Dados pessoais alterados|
|`DadosAcademicosAtualizados`|Dados acadêmicos alterados|
|`SenhaAlterada`|Senha alterada|
|`CursoAlterado`|Curso médio ou superior alterado|
|`EmailVerificadoEstudante`|Email verificado|
|`StatusEscolarFundamentalAtualizado`|Status fundamental alterado|
|`StatusEscolarMedioAtualizado`|Status médio alterado|
|`StatusSuperiorAtualizado`|Status superior alterado|
|`AvaliacaoFinalAnoAcademico`|Avaliação final de ano acadêmico registrada (aprovação ou reprovação)|
|`NotasRegistradas`|Nota registrada|
|`NotaAtualizada`|Nota corrigida|
|`NotaDeletada`|Nota removida (soft delete)|
|`FaltasRegistradas`|Faltas registradas|
|`FaltaAtualizada`|Falta corrigida|
|`FaltaDeletada`|Falta removida (soft delete)|

---

### 2.4 Curso

**Responsabilidade:** Representa um curso de médio ou superior oferecido por uma academia.

**Campos do aggregate:**

|Campo|Tipo|Descrição|
|---|---|---|
|`ID`|`uuid.UUID`|Identificador único|
|`Nome`|`string`|Nome do curso|
|`Type`|`string`|`medio` / `superior` — **imutável após criação**|
|`AnosAcademicos`|`[]string`|Anos do curso definidos pela academia (formato canônico por tipo)|
|`Periodos`|`[]string`|Períodos letivos — obrigatório para `superior`, vazio para `medio`|
|`CodigoAcademia`|`string`|Academia proprietária|
|`Status`|`string`|`ativo` / `inativo` / `deletado`|
|`CreatedAt`|`time.Time`|Data de criação|
|`DeletedAt`|`*time.Time`|Data de deleção (soft delete)|

**Formato dos anos do curso:**

- `type=medio`: cada item no formato `[n]_ano_medio` (ex: `1_ano_medio`, `2_ano_medio`, `3_ano_medio`)
- `type=superior`: cada item no formato `[n]_ano_superior` (ex: `1_ano_superior`, `2_ano_superior`)

**Formato dos períodos (apenas `superior`):**

Cada item deve seguir `[n]_semestre` onde n é inteiro ≥ 1 (ex: `1_semestre`, `2_semestre`). Trimestres (`1_trimestre`, `2_trimestre`, `3_trimestre`) são períodos **fixos do sistema** para notas do tipo `escolar` — não são configurados no curso.

**Regras de negócio:**

- O `Type` é **imutável**. Uma vez criado como `medio`, nunca vira `superior`.
- Cursos `superior` devem ter `Periodos` configurados (obrigatório na criação).
- Cursos `medio` não devem ter `Periodos` (os trimestres são fixos do sistema).
- Para deletar, o curso deve estar `inativo` (não `ativo`).
- A validação de estudantes matriculados antes de deletar é feita no handler (requer acesso à projeção).
- Ao deletar um curso, matérias inativas vinculadas são deletadas em cascata (cada uma emite `MateriaDeletada`); turmas inativas vinculadas também são deletadas.

**Eventos emitidos:**

|Evento|Quando|
|---|---|
|`CursoCriado`|Novo curso criado|
|`CursoAtivado`|Curso ativado|
|`CursoDesativado`|Curso desativado|
|`CursoDadosAtualizados`|Nome, anos ou períodos alterados|
|`CursoDeletado`|Curso removido (soft delete)|

---

### 2.5 MateriaDisciplinar

**Responsabilidade:** Representa uma disciplina/matéria vinculada a uma academia, tipo de ensino e curso (se médio/superior).

**Campos do aggregate:**

|Campo|Tipo|Descrição|
|---|---|---|
|`ID`|`uuid.UUID`|Identificador único|
|`Nome`|`string`|Nome da matéria|
|`Type`|`string`|`fundamental` / `medio` / `superior`|
|`AnosAcademicos`|`[]string`|Anos a que pertence (formato canônico por tipo)|
|`Periodo`|`string`|Período letivo (apenas para `superior`; vazio nos demais)|
|`CodigoAcademia`|`string`|Academia proprietária|
|`CursoID`|`*uuid.UUID`|`nil` para fundamental; FK para médio/superior|
|`Status`|`string`|`ativo` / `inativo` / `deletado`|
|`CreatedAt`|`time.Time`|Data de criação|

**Regras de negócio — anos_academicos:**

- `fundamental`: 1 a 9 itens no formato `[1-9]_ano_fundamental`.
- `medio` ou `superior`: exatamente 1 item no formato correspondente.

**Regras de negócio — Status inicial:**

- Matérias `superior` nascem com `status = inativo`. Exigem `DefinirPeriodo` antes de poderem ser ativadas.
- Matérias `fundamental` e `medio` nascem com `status = ativo`.

**Regras de negócio — Período:**

- Apenas matérias `superior` têm período definido.
- O período deve pertencer à lista de `Periodos` do curso vinculado (validado no handler).
- Sem período definido, a matéria `superior` não pode ser ativada.

**Eventos emitidos:**

|Evento|Quando|
|---|---|
|`MateriaCriada`|Nova matéria criada|
|`MateriaAtivada`|Matéria ativada|
|`MateriaDesativada`|Matéria desativada|
|`MateriaDadosAtualizados`|Dados atualizados|
|`MateriaPeriodoDefinido`|Período definido/alterado|
|`MateriaDeletada`|Matéria removida|

---

### 2.6 Turma

**Responsabilidade:** Representa uma turma dentro de uma academia, contendo um conjunto de estudantes, com nível, turno e curso associado.

**Campos do aggregate:**

|Campo|Tipo|Descrição|
|---|---|---|
|`ID`|`uuid.UUID`|Identificador único|
|`CodigoTurma`|`string`|Código único dentro da academia|
|`CodigoAcademia`|`string`|Academia proprietária|
|`Nivel`|`string`|Ano escolar/superior da turma|
|`CursoID`|`*uuid.UUID`|FK para curso (apenas médio/superior)|
|`Turno`|`string`|`manha` / `tarde` / `noite`|
|`Estudantes`|`[]string`|Códigos dos estudantes da turma (`CodigoEstudante`)|
|`Status`|`string`|`ativo` / `inativo` / `deletado`|
|`StatusAlteradoPor`|`uuid.UUID`|Academia que fez a última ativação/desativação|
|`StatusAlteradoEm`|`time.Time`|Timestamp da última mudança de status|

> `Estudantes` é um array de `CodigoEstudante` (string) — não é FK para `projection_estudantes`.

**Regras de negócio:**

- Um estudante pode estar em mais de uma turma.
- Para deletar, a turma deve estar `inativa` e sem estudantes vinculados.
- Ao registrar avaliação final de um estudante, ele é automaticamente removido de todas as turmas da academia (evento `EstudanteRemovidoDaTurma` por turma).

**Eventos emitidos:**

|Evento|Quando|
|---|---|
|`TurmaCriada`|Nova turma criada|
|`TurmaAtivada`|Turma ativada|
|`TurmaDesativada`|Turma desativada|
|`TurmaDadosAtualizados`|Dados alterados|
|`TurmaDeletada`|Turma removida|
|`EstudanteAdicionadoATurma`|Estudante adicionado|
|`EstudanteRemovidoDaTurma`|Estudante removido (emitido automaticamente na avaliação final)|

---

### 2.7 TelefoneExtra

**Responsabilidade:** Representa um número de telefone adicional de qualquer tipo de usuário (estudante, academia ou admin).

**Campos do aggregate:**

|Campo|Tipo|Descrição|
|---|---|---|
|`ID`|`uuid.UUID`|Identificador único|
|`IDUser`|`uuid.UUID`|UUID do usuário dono do telefone|
|`TipoUser`|`string`|`estudante` / `academia` / `admin`|
|`NumeroTelefone`|`string`|Número normalizado (sem espaços, hífens ou parênteses)|
|`Verificado`|`bool`|Se o usuário confirmou a posse do número|
|`RegisteredAt`|`time.Time`|Data de registro|

**Normalização do número:**

O número é normalizado antes de ser persistido: espaços, hífens e parênteses são removidos. O `+` inicial é preservado. O formato aceito após normalização é `+?[0-9]{7,15}`.

**Regras de negócio:**

- O mesmo número pode ser cadastrado por múltiplos usuários enquanto nenhum deles o verificou.
- Quando um usuário verifica o número, os demais não podem verificá-lo (garantido por índice único parcial `WHERE verificado = TRUE` na tabela).
- Se um número já está verificado por qualquer usuário, nenhum outro pode cadastrá-lo (verificado pelo handler antes de emitir o evento).
- Um usuário não pode cadastrar o mesmo número duas vezes (constraint `UNIQUE (id_user, tipo_user, numero_telefone)` na tabela).
- Apenas o próprio dono pode verificar o telefone — validado no aggregate via `IDUser`.

**Eventos emitidos:**

|Evento|Quando|
|---|---|
|`TelefoneExtraAdicionado`|Número de telefone extra cadastrado|
|`TelefoneExtraVerificado`|Número de telefone verificado pelo dono|

---

## 3. Processos de Dados

### 3.1 Notas

**Quem registra:** Academia **Rota:** `POST /academia/notas-aluno`

**Campos de uma nota:**

|Campo|Tipo|Descrição|
|---|---|---|
|`CodigoEstudante`|`string`|Código do estudante|
|`CodigoAcademia`|`string`|Academia que registrou|
|`AnoLectivo`|`string`|Ano letivo ativo da academia — **não enviado no request**, resolvido de `AcademiaDTO.AnoLetivo`|
|`AnoAcademico`|`string`|Ano acadêmico inferido pelo handler|
|`Periodo`|`string`|Período letivo|
|`MateriaDisciplinarID`|`uuid.UUID`|Matéria associada|
|`Tipo`|`string`|`escolar` / `superior`|
|`Categoria`|`string`|Categoria da nota|
|`Nota`|`float64`|Valor de 0 a 20|
|`Observacao`|`*string`|Observação (opcional no registro, **obrigatória** na correção)|
|`RegistradoPor`|`uuid.UUID`|ID do usuário que registrou (auditoria)|

**Categorias de nota por tipo:**

- `escolar`: `nota_escola`, `nota_professor` (fixas) + categorias adicionais cadastradas pela academia
- `superior`: `nota_pp1`, `nota_pp2`, `nota_exame` (fixas) + categorias adicionais cadastradas pela academia

**Inferência do `AnoAcademico`:**

- Estudante com `AnoEscolar` preenchido (fundamental) → usa `AnoEscolar` do estudante
- Caso contrário (médio/superior) → usa `AnosAcademicos[0]` da matéria

**Validações:**

- Nota deve ser entre 0 e 20 — validação no aggregate, não apenas no handler.
- O estudante deve pertencer à academia.
- Período deve ser válido para o tipo de curso.
- Duplicata detectada via `NotasRegistradasPorChave` no aggregate (sem depender da projeção).
- Para atualizar uma nota (`PUT /academia/atualizar-nota`), `observacao` é **obrigatória**.
- Para deletar uma nota (`DELETE /academia/nota/:id`), `motivo` é **obrigatório**.
- Academia do tipo `escola` só pode registrar notas do tipo `escolar`; `superior` só do tipo `superior`.

**Acesso:**

- Academia e Admin: `GET /notas-estudante/:codigo`
- Estudante (próprias): `GET /estudante/minhas-notas`

---

### 3.2 Faltas

**Quem registra:** Academia **Rota:** `POST /academia/faltas-aluno`

**Campos de uma falta:**

|Campo|Tipo|Descrição|
|---|---|---|
|`CodigoEstudante`|`string`|Código do estudante|
|`CodigoAcademia`|`string`|Academia que registrou|
|`AnoLectivo`|`string`|Ano letivo ativo da academia — **não enviado no request**|
|`AnoAcademico`|`string`|Ano acadêmico inferido pelo handler|
|`Data`|`time.Time`|Data da falta (formato `AAAA-MM-DD` no request)|
|`MateriaDisciplinarID`|`uuid.UUID`|Matéria da falta|
|`Quantidade`|`int`|Número de aulas de falta (deve ser positivo)|
|`Observacao`|`*string`|Observação (opcional)|

**Validações:**

- `Quantidade` deve ser positivo — validado no aggregate.
- A matéria deve existir e pertencer à academia.
- Duplicata detectada via `FaltasRegistradasPorChave` no aggregate.
- Para deletar uma falta (`DELETE /academia/falta/:id`), `motivo` é **obrigatório**.

**Acesso:**

- Academia e Admin: `GET /faltas-estudante/:codigo`
- Estudante (próprias): `GET /estudante/minhas-faltas`

---

### 3.3 Avaliação Final de Ano Acadêmico (`AvaliacaoFinalAnoAcademico`)

**Quem registra:** Academia **Rota:** `POST /academia/avaliacao-final`

Este é o **único mecanismo** de avaliação de ano. Registra se o estudante foi aprovado ou reprovado ao final de um ano acadêmico e desencadeia ações automáticas.

**Campos do request:**

|Campo|Tipo|Descrição|
|---|---|---|
|`codigo_estudante`|`string`|Código do estudante|
|`tipo_ensino`|`string`|`fundamental` / `medio` / `superior`|
|`nivel_ano_academico_atual`|`string`|Ano acadêmico atual (formato canônico por tipo)|
|`proximo_ano_academico`|`*string`|Próximo ano (obrigatório se aprovado, exceto último ano do ciclo)|
|`aprovado`|`bool`|Se foi aprovado|
|`observacao`|`*string`|Override da validação de notas — forçar aprovação mesmo com notas ausentes|

> `AnoLectivo` não é enviado no request — é resolvido automaticamente de `AcademiaDTO.AnoLetivo`.

**Idempotência no aggregate:**

O mapa `AvaliacoesPorAno` (chave: `"<tipoEnsino>_<anoLectivo>_<anoAcademicoAtual>"`) impede que a mesma avaliação seja registrada duas vezes. O guard é aplicado **antes** de emitir o evento, retornando erro de negócio claro ao invés de violação de constraint no banco.

**Processo de validação antes de registrar (`validarNotasParaAprovacao`):**

1. Para `fundamental`: verificar se todas as matérias do ano têm `nota_escola` nos 3 trimestres.
2. Para `medio`: verificar se todas as matérias do curso/ano têm `nota_escola` nos períodos do curso.
3. Para `superior`: verificar se todas as matérias do curso/período têm `nota_exame`.
4. Se notas estiverem faltando, retorna erro — a menos que `observacao` seja fornecida para forçar.

> A validação de notas é ignorada quando `aprovado = false`.

**Efeitos no estado do aggregate ao aplicar o evento:**

- `aprovado=true` e `proximo_ano_academico` preenchido → avança `AnoEscolar`/`AnoEscolarMedio`/`AnoSuperior` para o próximo nível.
- `aprovado=true` e `proximo_ano_academico=null` (último ano do ciclo) → seta `StatusEscolarFundamental`/`StatusEscolarMedio`/`StatusSuperior` = `"finalizado"`.
- `aprovado=false` → sem alteração de ano/status; apenas registra no mapa de idempotência.

**Ações automáticas ao registrar avaliação final:**

- O estudante é **removido de todas as turmas** da academia (evento `EstudanteRemovidoDaTurma` por turma).
- A remoção de turmas **não é atômica** com a avaliação — são salvamentos separados no ledger. Em caso de falha parcial, o rebuild restaura o estado correto.

**Consulta de aprovações e reprovações:**

A projeção `projection_avaliacao_final` armazena todos os registros com o campo `aprovado`. Os endpoints filtram por esse campo:

- `GET /aprovacoes` → `aprovado = TRUE`
- `GET /reprovacoes` → `aprovado = FALSE`
- `GET /avaliacoes` → todos os registros

---

### 3.4 Ano Letivo por Academia

**Quem define:** Academia **Rotas:** `POST /academia/ano-letivo` / `GET /academia/ano-letivo`

Cada academia é responsável por definir e manter o seu próprio ano letivo ativo.

**Campos do request (`POST /academia/ano-letivo`):**

|Campo|Tipo|Descrição|
|---|---|---|
|`ano_letivo`|`string`|Formato obrigatório: `YYYY_YYYY` (ex: `2025_2026`)|
|`tipo`|`string`|`escola` ou `superior`|

**Regras de negócio:**

- O formato `YYYY_YYYY` é validado no aggregate — o segundo ano deve ser exatamente o primeiro + 1.
- O `tipo` deve ser `escola` ou `superior`.
- Pode ser chamado múltiplas vezes — cada chamada substitui o valor anterior.
- **Bloqueio:** qualquer tentativa de registrar nota, falta ou avaliação final de uma academia sem ano letivo ativo retorna 400.
- O evento `AnoLetivoAcademiaDefinido` é gravado no aggregate `Academia` no ledger.
- O rebuild da projeção `academias` restaura automaticamente o ano letivo de cada academia.

---

### 3.5 Telefone Extra

**Quem cadastra:** Qualquer usuário autenticado **Rota:** `POST /adicionar-telefone-extra`

Permite que qualquer tipo de usuário (estudante, academia ou admin) adicione um número de telefone extra à sua conta.

**Campos do request:**

|Campo|Tipo|Descrição|
|---|---|---|
|`numero_telefone`|`string`|Número de telefone (normalizado automaticamente)|

**Fluxo:**

1. O handler normaliza o número (remove espaços, hífens, parênteses).
2. Verifica na projeção se o número já está verificado por outro usuário → 409 se sim.
3. Verifica se o usuário já cadastrou este número → 409 se sim.
4. Cria aggregate `TelefoneExtra`, executa `Adicionar`, salva no ledger.

---

## 4. Tabelas de Projeção (Read Models)

As projeções são reconstruídas via `Rebuild()` replaying todos os eventos do ledger. O estado final deve ser idêntico ao gerado evento a evento.

O `Rebuild()` segue sempre o padrão:

1. `TRUNCATE TABLE projection_x CASCADE` (ou `DELETE FROM` para projeções com FKs sensíveis) — sem estado residual.
2. Replay de todos os eventos do ledger para o `aggregate_type` correspondente, em ordem `ORDER BY id ASC`.
3. Reset e atualização do checkpoint em `projection_checkpoints` via `markRebuildComplete`.

### 4.1 `projection_estudantes`

Campos principais: `id`, `codigo_estudante`, `nome`, `email`, `telefone`, `bilhete_identidade`, `bilhete_identidade_responsavel`, `genero`, `data_nascimento`, `codigo_academia`, `status`, `status_escolar_fundamental`, `status_escolar_medio`, `status_superior`, `ano_escolar`, `ano_escolar_medio`, `ano_superior`, `curso_medio_id`, `curso_superior_id`, `email_verificado`, `total_notas`, `total_faltas`, `version`.

> `data_nascimento DATE` adicionado na migration 043. Constraint: deve ser anterior à `CURRENT_DATE`.

### 4.2 `projection_academias`

Campos principais (atualizado): `id`, `type`, `nome`, `codigo_academia`, `senha_hash`, `provincia`, `endereco`, `numero_telefone`, `email`, `email_verificado`, `website`, `nivel_escolar`, `anos_academicos`, `cursos`, `status`, `motivo_desativacao`, `created_at`, `updated_at`, `last_event_id`, `total_estudantes`, `ano_letivo`, `tipo_ano_letivo`, `ano_letivo_ativado_em`, `ano_letivo_ativado_por`, `version`.

> Nota: nem todos os campos acima são expostos nas respostas HTTP. Ex.: `senha_hash` e `last_event_id` ficam restritos ao backend/projeções.

### 4.3 `projection_admins`

Campos principais: `id`, `nome`, `email`, `role`, `status`, `email_verificado`, `created_by`, `total_acoes_realizadas`.

Constraint especial: apenas **1 admin FPP** com `created_by IS NULL` (bootstrap único), garantido por índice único parcial.

### 4.4 `projection_cursos`

Campos principais: `id`, `nome`, `type`, `anos_academicos` (JSONB), `periodos` (JSONB, nullable — null para `medio`), `codigo_academia`, `status`, `deleted_at`.

### 4.5 `projection_materias`

Campos principais: `id`, `nome`, `type`, `anos_academicos` (JSONB), `periodo`, `codigo_academia`, `curso_id`, `status`, `deleted_at`.

### 4.6 `projection_turmas`

Campos principais: `id`, `codigo_turma`, `codigo_academia`, `nivel`, `curso_id`, `turno`, `estudantes` (JSON array de strings), `status`, `status_alterado_por`, `status_alterado_em`, `deleted_at`.

### 4.7 `projection_notas`

Campos principais: `id`, `codigo_estudante`, `codigo_academia`, `ano_lectivo`, `ano_academico`, `periodo`, `materia_disciplinar_id`, `tipo`, `categoria`, `nota`, `observacao`, `registered_at`, `deleted_at`, `deletado_por`, `motivo_exclusao`.

Constraint de unicidade: `UNIQUE (codigo_estudante, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)` — nome: `uq_nota_unica`.

### 4.8 `projection_faltas`

Campos principais: `id`, `codigo_estudante`, `codigo_academia`, `ano_lectivo`, `ano_academico`, `data`, `materia_disciplinar_id`, `quantidade`, `observacao`, `registered_at`, `deleted_at`, `deletado_por`, `motivo_exclusao`.

### 4.9 `projection_avaliacao_final`

**Única tabela de avaliação de ano.** Cobre tanto aprovações quanto reprovações via campo `aprovado`.

Campos principais: `id`, `event_id`, `codigo_estudante`, `codigo_academia`, `ano_lectivo`, `tipo_ensino`, `ano_academico_atual`, `proximo_ano_academico`, `codigo_turma`, `aprovado`, `observacao`, `registered_at`, `version`.

Constraint de unicidade: `UNIQUE (codigo_estudante, codigo_academia, ano_lectivo, tipo_ensino)` — o evento mais recente vence via `ON CONFLICT DO UPDATE`.

> **Nota:** o campo `codigo_turma` registra a turma em que o estudante estava no momento da avaliação, mas **não faz parte do DTO de leitura** (`AvaliacaoFinalDTO`) — existe apenas na tabela para auditoria.

**Filtros dos endpoints:**

- `GET /avaliacoes` → todos os registros
- `GET /aprovacoes` → `aprovado = TRUE`
- `GET /reprovacoes` → `aprovado = FALSE`

### 4.10 `projection_categorias_nota`

Campos principais: `id`, `codigo_academia`, `nome`, `descricao`, `adicionado_por`, `status`, `created_at`, `version`.

### 4.11 `projection_telefones_extra`

Campos principais: `id`, `id_user`, `tipo_user`, `numero_telefone`, `verificado`, `registered_at`, `updated_at`, `event_id`, `version`.

Constraints:

- `UNIQUE (id_user, tipo_user, numero_telefone)` — um usuário não pode cadastrar o mesmo número duas vezes.
- Índice único parcial `WHERE verificado = TRUE` em `numero_telefone` — no máximo um registro verificado por número.

### 4.12 Outras projeções

- `projection_inscricoes` — **depreciada** (migration 024), mantida para dados históricos

> **Removidas na migration 046:** `projection_aprovacao_ano` e `projection_reprovacoes`. Toda a informação de aprovação/reprovação de ano está em `projection_avaliacao_final`.

---

## 5. Ledger e Hash Chain (`spuri_ledger`)

O ledger é **imutável**. Nenhum `UPDATE` ou `DELETE` é permitido. Triggers PostgreSQL bloqueiam UPDATE, DELETE e TRUNCATE (`prevent_update_ledger`, `prevent_delete_ledger`, `prevent_truncate_ledger`).

**Campos do ledger:**

|Campo|Tipo|Descrição|
|---|---|---|
|`id`|`BIGSERIAL`|Sequência global|
|`event_id`|`UUID`|UUID único do evento|
|`aggregate_id`|`UUID`|Identificador do aggregate|
|`aggregate_type`|`string`|Tipo do aggregate (ex: `Estudante`)|
|`event_type`|`string`|Tipo do evento (ex: `AvaliacaoFinalAnoAcademico`)|
|`event_version`|`int`|Versão do aggregate no momento do evento|
|`payload`|`JSONB`|Dados do evento|
|`metadata`|`JSONB`|Contexto de auditoria (user_id, user_type, IP)|
|`occurred_at`|`TIMESTAMP`|Quando o evento ocorreu no domínio|
|`recorded_at`|`TIMESTAMP`|Quando foi gravado no ledger|
|`ledger_hash`|`string`|Hash SHA256 deste evento + hash anterior|
|`previous_hash`|`*string`|Hash do evento anterior (chain)|

**Constraint:** `UNIQUE(aggregate_id, event_version)` — não é possível gravar dois eventos com a mesma versão para o mesmo aggregate.

**Hash chain:** o trigger `auto_generate_ledger_hash` usa o hash do evento anterior via `ORDER BY event_version DESC`.

**Whitelist de eventos (`safe_queries.go`):**

A camada `internal/db/safe_queries.go` mantém uma whitelist de todos os `event_type` válidos. Nenhum evento não listado pode ser gravado no ledger. A whitelist deve estar sempre sincronizada com todos os eventos que os aggregates emitem — qualquer novo evento requer atualização desta lista.

**Whitelist atual de event types:**

```
SchemaCreated
EstudanteCriadoComVinculo, DadosPessoaisAtualizados, DadosAcademicosAtualizados
SenhaAlterada, CursoAlterado, EmailVerificadoEstudante
StatusEscolarFundamentalAtualizado, StatusEscolarMedioAtualizado, StatusSuperiorAtualizado
AvaliacaoFinalAnoAcademico
NotasRegistradas, NotaAtualizada, NotaDeletada
FaltasRegistradas, FaltaRegistrada, FaltaAtualizada, FaltaDeletada
AcademiaCriada, AcademiaAtivada, AcademiaDesativada, AcademiaDadosAtualizados
CursosAtualizados, AcademiaSenhaAlterada, CategoriaNotaAdicionada
AnoLetivoAcademiaDefinido, EmailVerificado
AdminCriado, AdminAtivado, AdminDesativado, AdminDadosAtualizados
AdminSenhaAlterada, AcaoAdminRegistrada, AdminRoleAtualizado
TurmaCriada, TurmaAtivada, TurmaDesativada, TurmaDadosAtualizados
TurmaDeletada, TurmaEncerrada
EstudanteAdicionadoATurma, EstudanteRemovidoDaTurma
CursoCriado, CursoAtivado, CursoDesativado, CursoDadosAtualizados, CursoDeletado
MateriaCriada, MateriaAtivada, MateriaDesativada
MateriaDadosAtualizados, MateriaPeriodoDefinido, MateriaDeletada
TelefoneExtraAdicionado, TelefoneExtraVerificado
```

> **Removidos da whitelist:** `AprovacaoAnoRegistrada` — evento depreciado.

**Aggregate types válidos:** `Estudante`, `Academia`, `Admin`, `Curso`, `MateriaDisciplinar`, `Turma`, `TelefoneExtra`, `System`.

**Reconstrução do aggregate (`Load`):**

O `Load()` reconstrói o aggregate replaying eventos em ordem `ORDER BY event_version ASC`. O UUID real é injetado via `SetID` antes de qualquer `Apply`. Erros de `Apply()` durante o `Load` são sempre propagados.

**Decisão de design — `SenhaHash` no payload do ledger:**

Os eventos de criação e alteração de senha incluem o hash bcrypt no payload gravado no ledger. Isso é necessário para que um `Rebuild()` restaure a senha correta. O hash bcrypt não é a senha em texto plano. Controles de acesso ao banco são a mitigação correspondente.

---

## 6. Sistema de Permissões

### Roles de usuário e acesso:

|Role|Pode fazer|
|---|---|
|`fpp` (admin)|Tudo: ativar academias, criar admins, rebuild de projeções|
|`adm` (admin)|Gerenciar admins de role inferior, consultar dados|
|`gerente` (admin)|Consultas, ações básicas|
|`academia`|Gerenciar seus estudantes, notas, faltas, turmas, cursos, matérias|
|`estudante`|Ver próprio perfil, notas, faltas, avaliações; atualizar dados pessoais|

### Middlewares de proteção:

|Middleware|Função|
|---|---|
|`AuthMiddleware`|Verifica JWT + status ativo do usuário em qualquer rota protegida|
|`RequireEstudante`|Garante que o token é de estudante|
|`RequireAcademia`|Garante que o token é de academia|
|`ValidarStatusAcademia`|Bloqueia academias inativas (verifica projeção)|
|`RequireAdmin`|Garante que o token é de admin com role ≥ `gerente` e email verificado|
|`RequireAdminRole(minRole)`|Garante role mínimo e email verificado do admin|
|`RequireAcademiaOuAdmin`|Permite academia ou qualquer admin|

**Regras invioláveis de middleware:**

- `c.Abort()` sempre chamado antes de retornar erro.
- `AuthMiddleware` verifica o status do usuário no banco para todos os tipos (`estudante`, `academia`, `admin`) — conta inativa bloqueia mesmo com JWT válido.
- `AuditContext` com `UserID`, `UserType` e `IP: c.ClientIP()` em todo `SaveWithAudit`.

### Hierarquia de role (Admin):

```go
adminHierarchy = map[string]int{
    "gerente": 1,
    "adm":     2,
    "fpp":     3,
}
```

`ValidatePermission` verifica nível **estritamente superior** (não igual) ao do alvo.

### Login unificado:

Existe um único endpoint de login: `POST /login`. O tipo do usuário é inferido automaticamente pela busca em cascata: `admin → academia → estudante`. O campo `type` **não é enviado no request**. A comparação bcrypt é sempre executada (mesmo quando o usuário não existe) para evitar timing attacks e user enumeration.

---

## 7. Fluxo de Email e Autenticação

**Verificação de email:**

- `POST /email/verificar-email/:token` — verifica email via token (Admin, Academia e Estudante)
- Token de verificação tem validade de 24h e tipo `verificacao_email`

**Recuperação de senha:**

- `POST /email/recuperar-senha/solicitar` — solicita token de recuperação (1h de validade)
- `POST /email/recuperar-senha/:token` — define nova senha via token

**Geração de tokens para frontend:**

- `POST /email/gerar-token/verificacao` — gera token e retorna ao frontend (sem enviar email)
- `POST /email/gerar-token/recuperacao` — gera token de recuperação e retorna ao frontend

**Todas as operações de senha passam por event sourcing** — cada mudança gera um evento no ledger (`AdminSenhaAlterada`, `AcademiaSenhaAlterada`, `SenhaAlterada`).

**Rate limit:** as rotas de email têm rate limiting próprio por IP e por identificador de usuário para evitar spam.

---

## 8. Operações em Lote (Batch)

Os endpoints `/batch` permitem que uma academia (ou admin) envie múltiplos itens em uma única requisição HTTP, reduzindo round-trips e latência de rede em operações em massa.

### Semântica e atomicidade

**Não há atomicidade entre itens.** Cada item é processado sequencialmente como uma operação independente no ledger. Se o item 3 falhar, os itens 1 e 2 já foram gravados — exatamente como aconteceria chamando a rota individual N vezes. O cliente deve tratar falhas parciais re-enviando apenas os itens com `sucesso=false`.

### Formato da resposta

Todos os endpoints batch retornam o mesmo envelope:

```json
{
  "total": 3,
  "sucesso": 2,
  "falhas": 1,
  "items": [
    { "index": 0, "sucesso": true,  "dados": { ... } },
    { "index": 1, "sucesso": true,  "dados": { ... } },
    { "index": 2, "sucesso": false, "erro": "estudante não pertence a esta academia" }
  ]
}
```

### Códigos HTTP de retorno

|Situação|HTTP|
|---|---|
|Todos os itens com sucesso|`200 OK`|
|Sucesso parcial (pelo menos 1 falhou)|`207 Multi-Status`|
|Todos os itens falharam|`422 Unprocessable Entity`|
|Body inválido / array vazio / excede limite|`400 Bad Request`|

### Implementação

Os batch handlers residem em `internal/handlers/batch_handlers.go` e utilizam `newFakeContext` (em `batch_context.go`) para chamar os handlers individuais existentes de forma sintética, sem round-trip de rede real. Os handlers originais **não foram modificados**.

### Endpoints batch disponíveis

#### Academia

|Método|Rota|Handler individual equivalente|Limite|
|---|---|---|---|
|`POST`|`/academia/estudante/register/batch`|`RegisterEstudantePorAcademia`|100|
|`POST`|`/academia/notas-aluno/batch`|`RegistrarNota`|200|
|`PUT`|`/academia/atualizar-nota/batch`|`AtualizarNota`|200|
|`DELETE`|`/academia/nota/batch`|`DeletarNota`|200|
|`POST`|`/academia/faltas-aluno/batch`|`RegistrarFaltas`|200|
|`PUT`|`/academia/atualizar-falta/batch`|`AtualizarFalta`|200|
|`DELETE`|`/academia/falta/batch`|`DeletarFalta`|200|
|`POST`|`/academia/avaliacao-final/batch`|`RegistrarAvaliacaoFinal`|100|
|`PUT`|`/academia/estudante/status-escolar/batch`|Consolida os 3 endpoints de status|100|
|`POST`|`/academia/curso/batch`|`CriarCurso`|50|
|`PUT`|`/academia/curso/ativar/batch`|`AtivarCurso`|50|
|`PUT`|`/academia/curso/desativar/batch`|`DesativarCurso`|50|
|`DELETE`|`/academia/curso/batch`|`DeletarCurso`|50|
|`POST`|`/academia/materia/batch`|`CriarMateria`|100|
|`PUT`|`/academia/materia/ativar/batch`|`AtivarMateria`|100|
|`PUT`|`/academia/materia/desativar/batch`|`DesativarMateria`|100|
|`DELETE`|`/academia/materia/batch`|`DeletarMateria`|100|
|`POST`|`/academia/turma/batch`|`CriarTurma`|50|
|`PUT`|`/academia/turma/ativar/batch`|`AtivarTurma`|50|
|`PUT`|`/academia/turma/desativar/batch`|`DesativarTurma`|50|
|`DELETE`|`/academia/turma/batch`|`DeletarTurma`|50|
|`POST`|`/academia/turma/estudante/batch`|`AdicionarEstudanteATurma`|100|
|`DELETE`|`/academia/turma/estudante/batch`|`RemoverEstudanteDaTurma`|100|

#### Admin

|Método|Rota|Handler individual equivalente|Limite|Proteção adicional|
|---|---|---|---|---|
|`POST`|`/dominis/academia/register/batch`|`RegisterAcademia`|50|—|
|`PUT`|`/dominis/academia/ativar/batch`|`AtivarAcademia`|50|`RequireAdm`|
|`PUT`|`/dominis/academia/desativar/batch`|`DesativarAcademia`|50|`RequireAdm`|

### Formatos de body por endpoint

**`/academia/estudante/register/batch`** — array de objetos `CadastroEstudanteAcademiaRequest`

**`/academia/notas-aluno/batch`:**

```json
[
  { "codigo_estudante": "ABC1234", "periodo": "1_trimestre",
    "materia_disciplinar_id": "uuid", "tipo": "escolar",
    "categoria": "nota_escola", "nota": 15.5 }
]
```

**`/academia/nota/batch` (DELETE):**

```json
[{ "id": "uuid-da-nota", "motivo": "erro de lançamento" }]
```

**`/academia/faltas-aluno/batch`:**

```json
[
  { "codigo_estudante": "ABC1234", "data": "2026-03-15",
    "materia_disciplinar_id": "uuid", "quantidade": 2 }
]
```

**`/academia/falta/batch` (DELETE):**

```json
[{ "id": "uuid-da-falta", "motivo": "erro de lançamento" }]
```

**`/academia/avaliacao-final/batch`:**

```json
[
  { "codigo_estudante": "ABC1234", "tipo_ensino": "fundamental",
    "nivel_ano_academico_atual": "3_ano_fundamental",
    "proximo_ano_academico": "4_ano_fundamental", "aprovado": true }
]
```

**`/academia/estudante/status-escolar/batch`** — consolida os 3 endpoints individuais:

```json
[
  { "codigo_estudante": "ABC1234", "tipo": "fundamental", "novo_status": "em_andamento" },
  { "codigo_estudante": "DEF5678", "tipo": "superior",    "novo_status": "finalizado" }
]
```

**Endpoints de ativar/desativar por ID** (`/curso`, `/materia`):

```json
[{ "id": "uuid" }, { "id": "uuid" }]
```

**Endpoints de ativar/desativar por código** (`/turma`, `/academia`):

```json
[{ "codigo": "T1" }, { "codigo": "T2" }]
```

**`/academia/turma/estudante/batch`** (POST e DELETE):

```json
[
  { "codigo_turma": "T1", "codigo_estudante": "ABC1234" },
  { "codigo_turma": "T2", "codigo_estudante": "DEF5678" }
]
```

**`/dominis/academia/desativar/batch`:**

```json
[{ "codigo": "LDA20261", "motivo": "encerramento de actividade" }]
```

---

## 9. Principais Endpoints HTTP

### Login e Bootstrap

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`POST`|`/login`|Pública|Login unificado (admin, academia ou estudante)|
|`POST`|`/bootstrap`|Pública (advisory lock)|Cria o primeiro admin `fpp`|

### Admin

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`POST`|`/dominis/register`|Auth + `gerente`|Cria novo admin|
|`PUT`|`/dominis/admin/:id/ativar`|Auth + `adm`|Ativa admin|
|`PUT`|`/dominis/admin/:id/desativar`|Auth + `adm`|Desativa admin|
|`PUT`|`/dominis/admin/:id/role`|Auth + `fpp`|Altera role|
|`PUT`|`/dominis/admin/:id/dados`|Auth + `gerente`|Atualiza nome/email|
|`GET`|`/dominis/admin-lista`|Auth + `gerente`|Lista admins|
|`GET`|`/dominis/consultar-admin/:email`|Auth + `gerente`|Busca admin por email|
|`GET`|`/dominis/metrics`|Auth + `gerente`|Métricas do sistema|
|`POST`|`/dominis/projections/rebuild/:name`|Auth + `fpp`|Rebuild de projeção|

### Academia (gerenciamento por admin)

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`POST`|`/dominis/academia/register`|Auth + `gerente`|Cria academia|
|`POST`|`/dominis/academia/register/batch`|Auth + `gerente`|Cria múltiplas academias|
|`PUT`|`/dominis/academia/:codigo/ativar`|Auth + `adm`|Ativa academia|
|`PUT`|`/dominis/academia/ativar/batch`|Auth + `adm`|Ativa múltiplas academias|
|`PUT`|`/dominis/academia/:codigo/desativar`|Auth + `adm`|Desativa academia|
|`PUT`|`/dominis/academia/desativar/batch`|Auth + `adm`|Desativa múltiplas academias|

### Academia (operações próprias)

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`PUT`|`/academia/dados`|Auth academia + ativa|Atualiza dados cadastrais|
|`POST`|`/academia/ano-letivo`|Auth academia + ativa|Define ou atualiza o ano letivo ativo|
|`GET`|`/academia/ano-letivo`|Auth academia + ativa|Consulta o ano letivo ativo|
|`POST`|`/academia/estudante/register`|Auth academia + ativa|Cadastra estudante com vínculo|
|`POST`|`/academia/estudante/register/batch`|Auth academia + ativa|Cadastra múltiplos estudantes|
|`POST`|`/academia/notas-aluno`|Auth academia + ativa|Registra nota|
|`POST`|`/academia/notas-aluno/batch`|Auth academia + ativa|Registra múltiplas notas|
|`PUT`|`/academia/atualizar-nota`|Auth academia + ativa|Corrige nota|
|`PUT`|`/academia/atualizar-nota/batch`|Auth academia + ativa|Corrige múltiplas notas|
|`DELETE`|`/academia/nota/:id`|Auth academia + ativa|Deleta nota (soft delete)|
|`DELETE`|`/academia/nota/batch`|Auth academia + ativa|Deleta múltiplas notas|
|`POST`|`/academia/faltas-aluno`|Auth academia + ativa|Registra falta|
|`POST`|`/academia/faltas-aluno/batch`|Auth academia + ativa|Registra múltiplas faltas|
|`PUT`|`/academia/atualizar-falta`|Auth academia + ativa|Corrige falta|
|`PUT`|`/academia/atualizar-falta/batch`|Auth academia + ativa|Corrige múltiplas faltas|
|`DELETE`|`/academia/falta/:id`|Auth academia + ativa|Deleta falta (soft delete)|
|`DELETE`|`/academia/falta/batch`|Auth academia + ativa|Deleta múltiplas faltas|
|`POST`|`/academia/avaliacao-final`|Auth academia + ativa|Registra avaliação final de ano|
|`POST`|`/academia/avaliacao-final/batch`|Auth academia + ativa|Registra múltiplas avaliações finais|
|`POST`|`/academia/categorias-nota`|Auth academia + ativa|Cria categoria de nota|
|`GET`|`/academia/categorias-nota`|Auth academia + ativa|Lista categorias de nota|
|`PUT`|`/academia/estudante/:codigo/status-escolar-fundamental`|Auth academia + ativa|Atualiza status fundamental|
|`PUT`|`/academia/estudante/:codigo/status-escolar-medio`|Auth academia + ativa|Atualiza status médio|
|`PUT`|`/academia/estudante/:codigo/status-superior`|Auth academia + ativa|Atualiza status superior|
|`PUT`|`/academia/estudante/status-escolar/batch`|Auth academia + ativa|Atualiza status de múltiplos estudantes|

### Cursos (academia)

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`POST`|`/academia/curso`|Auth academia + ativa|Cria curso|
|`POST`|`/academia/curso/batch`|Auth academia + ativa|Cria múltiplos cursos|
|`GET`|`/academia/cursos`|Auth academia + ativa|Lista cursos|
|`GET`|`/academia/curso/:id`|Auth academia + ativa|Consulta curso por ID|
|`PUT`|`/academia/curso/:id/ativar`|Auth academia + ativa|Ativa curso|
|`PUT`|`/academia/curso/ativar/batch`|Auth academia + ativa|Ativa múltiplos cursos|
|`PUT`|`/academia/curso/:id/desativar`|Auth academia + ativa|Desativa curso|
|`PUT`|`/academia/curso/desativar/batch`|Auth academia + ativa|Desativa múltiplos cursos|
|`PUT`|`/academia/curso/:id/dados`|Auth academia + ativa|Atualiza dados do curso|
|`DELETE`|`/academia/curso/:id`|Auth academia + ativa|Deleta curso (soft delete)|
|`DELETE`|`/academia/curso/batch`|Auth academia + ativa|Deleta múltiplos cursos|

### Matérias (academia)

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`POST`|`/academia/materia`|Auth academia + ativa|Cria matéria|
|`POST`|`/academia/materia/batch`|Auth academia + ativa|Cria múltiplas matérias|
|`GET`|`/academia/materias`|Auth academia + ativa|Lista matérias|
|`GET`|`/academia/materia/:id`|Auth academia + ativa|Consulta matéria por ID|
|`PUT`|`/academia/materia/:id/ativar`|Auth academia + ativa|Ativa matéria|
|`PUT`|`/academia/materia/ativar/batch`|Auth academia + ativa|Ativa múltiplas matérias|
|`PUT`|`/academia/materia/:id/desativar`|Auth academia + ativa|Desativa matéria|
|`PUT`|`/academia/materia/desativar/batch`|Auth academia + ativa|Desativa múltiplas matérias|
|`PUT`|`/academia/materia/:id/periodo`|Auth academia + ativa|Define período (apenas `superior`)|
|`PUT`|`/academia/materia/:id/dados`|Auth academia + ativa|Atualiza nome|
|`DELETE`|`/academia/materia/:id`|Auth academia + ativa|Deleta matéria|
|`DELETE`|`/academia/materia/batch`|Auth academia + ativa|Deleta múltiplas matérias|

### Turmas (academia)

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`POST`|`/academia/turma`|Auth academia + ativa|Cria turma|
|`POST`|`/academia/turma/batch`|Auth academia + ativa|Cria múltiplas turmas|
|`GET`|`/academia/turmas`|Auth academia + ativa|Lista turmas|
|`GET`|`/academia/turma/:codigo`|Auth academia + ativa|Consulta turma por código|
|`PUT`|`/academia/turma/:codigo/ativar`|Auth academia + ativa|Ativa turma|
|`PUT`|`/academia/turma/ativar/batch`|Auth academia + ativa|Ativa múltiplas turmas|
|`PUT`|`/academia/turma/:codigo/desativar`|Auth academia + ativa|Desativa turma|
|`PUT`|`/academia/turma/desativar/batch`|Auth academia + ativa|Desativa múltiplas turmas|
|`PUT`|`/academia/turma/:codigo/dados`|Auth academia + ativa|Atualiza dados da turma|
|`DELETE`|`/academia/turma/:codigo`|Auth academia + ativa|Deleta turma|
|`DELETE`|`/academia/turma/batch`|Auth academia + ativa|Deleta múltiplas turmas|
|`POST`|`/academia/turma/:codigo/estudante`|Auth academia + ativa|Adiciona estudante à turma|
|`POST`|`/academia/turma/estudante/batch`|Auth academia + ativa|Adiciona múltiplos estudantes a turmas|
|`DELETE`|`/academia/turma/:codigo/estudantes/:codigo_estudante`|Auth academia + ativa|Remove estudante da turma|
|`DELETE`|`/academia/turma/estudante/batch`|Auth academia + ativa|Remove múltiplos estudantes de turmas|

### Estudante (operações próprias)

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`PUT`|`/estudante/dados-pessoais`|Auth estudante|Atualiza dados pessoais|
|`GET`|`/estudante/minhas-notas`|Auth estudante|Consulta próprias notas|
|`GET`|`/estudante/minhas-faltas`|Auth estudante|Consulta próprias faltas|
|`GET`|`/estudante/minhas-avaliacoes`|Auth estudante|Consulta avaliações finais|

### Consultas e Sistema (rotas compartilhadas)

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`PUT`|`/alterar-senha`|Auth (qualquer tipo)|Altera própria senha|
|`GET`|`/meu-perfil`|Auth (qualquer tipo)|Consulta próprio perfil|
|`GET`|`/academias`|Auth (qualquer tipo)|Lista academias (com paginação e filtro por status)|
|`GET`|`/consultar-academia/:codigo`|Auth (qualquer tipo)|Consulta academia por código|
|`GET`|`/estudantes`|Auth academia ou admin|Lista estudantes|
|`GET`|`/consultar-estudante/:codigo`|Auth academia ou admin|Consulta estudante por código|
|`GET`|`/notas-estudante/:codigo`|Auth academia ou admin|Consulta notas do estudante|
|`GET`|`/faltas-estudante/:codigo`|Auth academia ou admin|Consulta faltas do estudante|
|`GET`|`/avaliacoes-estudante/:codigo`|Auth academia ou admin|Consulta avaliações do estudante|
|`GET`|`/avaliacoes`|Auth (qualquer tipo)|Lista avaliações (todos os registros)|
|`GET`|`/aprovacoes`|Auth (qualquer tipo)|Lista aprovações (`aprovado=TRUE`)|
|`GET`|`/reprovacoes`|Auth (qualquer tipo)|Lista reprovações (`aprovado=FALSE`)|
|`GET`|`/verificar-integridade/:codigo`|Auth (qualquer tipo)|Verifica hash chain do ledger|
|`GET`|`/eventos-estudante/:codigo`|Auth admin|Lista eventos do ledger de um estudante|
|`POST`|`/adicionar-telefone-extra`|Auth (qualquer tipo)|Adiciona telefone extra ao utilizador autenticado|
|`GET`|`/dominis/registros`|Auth admin|Lista todos os registros (notas/faltas)|
|`GET`|`/dominis/registros/:codigo`|Auth admin|Lista registros por estudante|

#### Guia de consumo (cliente) — rotas compartilhadas

Esta secção detalha **como consumir cada rota** do bloco “Consultas e Sistema” no frontend/client API.

##### `PUT /alterar-senha`
- **Auth:** obrigatório (qualquer tipo).
- **Body:** dados de senha atual + nova senha.
- **Resposta esperada:** confirmação de alteração (`message`) ou erro de validação/autorização.

##### `GET /meu-perfil`
- **Auth:** obrigatório.
- **Uso:** recuperar dados do usuário logado para preencher telas de perfil.
- **Resposta esperada:** objeto com os dados do usuário autenticado (campos variam por tipo: `admin`, `academia`, `estudante`).

##### `GET /academias`
- **Auth:** obrigatório.
- **Uso:** listagem para dropdowns, filtros e páginas de gestão.
- **Query params suportados:**

- `limit` (opcional): quantidade máxima de itens.
  - Sem `limit`, o endpoint usa **1000** por padrão.
  - Valores inválidos/fora do intervalo passam por sanitização (`db.ValidateLimit`), com teto global de **1000**.
- `offset` (opcional): deslocamento para paginação (default `0`).
- `status` (opcional): `ativo` ou `inativo`. Se omitido, retorna ambos.

- **Resposta base (`200 OK`):**

```json
{
  "academias": [
    {
      "id": "uuid",
      "type": "escola|superior",
      "nome": "string",
      "codigo_academia": "string",
      "provincia": "LDA",
      "endereco": "string",
      "numero_telefone": "string|null",
      "website": "string|null",
      "nivel_escolar": "fundamental|medio|misto|null",
      "status": "ativo|inativo",
      "cursos": ["string"],
      "email_verificado": false,
      "created_at": "RFC3339",
      "updated_at": "RFC3339|null"
    }
  ],
  "total": 1,
  "limit": 1000,
  "offset": 0
}
```

- **Campos adicionais quando o usuário autenticado é `admin`:**

- `email`
- `total_estudantes`
- `version`

##### `GET /consultar-academia/:codigo`
- **Auth:** obrigatório.
- **Path param:** `codigo` (código da academia, ex.: `LDA20261`).
- **Uso:** tela de detalhes de academia.
- **Resposta esperada:** objeto único da academia; `404` se não existir.

##### `GET /estudantes`
- **Auth:** obrigatório + perfil `academia` ou `admin`.
- **Uso:** listagem de estudantes com paginação/filtros no client.
- **Resposta esperada:** array paginado de estudantes da academia (ou escopo permitido ao admin).

##### `GET /consultar-estudante/:codigo`
- **Auth:** obrigatório + regras de ownership.
- **Path param:** `codigo` do estudante.
- **Uso:** página de perfil/detalhes do estudante.
- **Resposta esperada:** objeto do estudante; `403/404` conforme permissão/existência.

##### `GET /notas-estudante/:codigo`
- **Auth:** obrigatório + `academia`/`admin`.
- **Path param:** `codigo` do estudante.
- **Uso:** histórico de notas para boletim/analytics.
- **Resposta esperada:** lista de notas do estudante.

##### `GET /faltas-estudante/:codigo`
- **Auth:** obrigatório + `academia`/`admin`.
- **Path param:** `codigo` do estudante.
- **Uso:** acompanhamento de assiduidade.
- **Resposta esperada:** lista de faltas do estudante.

##### `GET /avaliacoes-estudante/:codigo`
- **Auth:** obrigatório + `academia`/`admin`.
- **Path param:** `codigo` do estudante.
- **Uso:** consolidado de avaliação final por estudante.
- **Resposta esperada:** lista de avaliações finais.

##### `GET /avaliacoes`
- **Auth:** obrigatório.
- **Uso:** visão global de avaliações finais.
- **Resposta esperada:** lista de avaliações (respeitando escopo de acesso).

##### `GET /aprovacoes`
- **Auth:** obrigatório.
- **Uso:** listar apenas avaliações finais com `aprovado=true`.
- **Resposta esperada:** lista filtrada de aprovações.

##### `GET /reprovacoes`
- **Auth:** obrigatório.
- **Uso:** listar apenas avaliações finais com `aprovado=false`.
- **Resposta esperada:** lista filtrada de reprovações.

##### `GET /verificar-integridade/:codigo`
- **Auth:** obrigatório.
- **Path param:** `codigo` do estudante.
- **Uso:** auditoria da cadeia de hashes (ledger) de um estudante.
- **Resposta esperada:** `{ codigo_estudante, nome, integro, message }`.

##### `GET /eventos-estudante/:codigo`
- **Auth:** obrigatório + `admin`.
- **Path param:** `codigo` do estudante.
- **Uso:** trilha de auditoria (event sourcing) para suporte/forense.
- **Resposta esperada:** stream/lista de eventos do estudante.

##### `POST /adicionar-telefone-extra`
- **Auth:** obrigatório.
- **Body:** telefone extra e metadados necessários.
- **Uso:** adicionar contacto alternativo do usuário autenticado.
- **Resposta esperada:** confirmação de criação e dados do telefone registrado.

##### `GET /dominis/registros`
- **Auth:** obrigatório + `admin`.
- **Uso:** consulta administrativa centralizada de registros (notas/faltas), para painéis de operação.
- **Paginação:** suporta `limit/offset` sanitizados no backend.

##### `GET /dominis/registros/:codigo`
- **Auth:** obrigatório + `admin`.
- **Path param:** `codigo` do estudante.
- **Uso:** investigação/relatório de registros de um estudante específico.
- **Resposta esperada:** coleção de registros associados ao código informado.

### Email

|Método|Rota|Proteção|Descrição|
|---|---|---|---|
|`POST`|`/email/verificar-email/:token`|Pública|Verifica email via token|
|`POST`|`/email/verificar-email/solicitar`|Auth (qualquer tipo)|Solicita envio do email de verificação|
|`POST`|`/email/recuperar-senha/solicitar`|Pública|Solicita token de recuperação de senha|
|`POST`|`/email/recuperar-senha/:token`|Pública|Define nova senha via token|
|`POST`|`/email/gerar-token/verificacao`|Auth (qualquer tipo)|Gera token de verificação (retorna ao frontend)|
|`POST`|`/email/gerar-token/recuperacao`|Pública|Gera token de recuperação (retorna ao frontend)|


### 9.1 Contrato global de integração (válido para todas as rotas)

Para evitar ambiguidade no cliente, **todas as rotas** seguem estes contratos transversais:

- **Content-Type:** `application/json` (exceto `GET`/`DELETE` sem body).
- **Auth bearer:** `Authorization: Bearer <jwt>` em todas as rotas protegidas.
- **Timezone:** datas no payload sempre em UTC quando houver timestamp completo (`RFC3339`).
- **Request ID:** toda resposta pode incluir `X-Request-ID`; o cliente deve persistir esse valor para suporte.
- **Envelope de erro padrão:**

```json
{ "error": "mensagem clara para integração" }
```

- **Faixas de status HTTP esperadas:**
  - `200/201/204` sucesso.
  - `400` erro de validação de entrada.
  - `401` token ausente/inválido/expirado.
  - `403` sem permissão ou entidade inativa.
  - `404` recurso não encontrado.
  - `409` conflito (duplicidade/estado incompatível).
  - `422` lote totalmente inválido.
  - `500` erro interno não esperado.

### 9.2 Matriz detalhada de rotas (integração cliente)

> Regra prática para o cliente: trate **4xx** como erro funcional corrigível (dados/permissão) e **5xx** como erro transitório com retry exponencial.

|Método|Rota|Auth|Path params|Query params|Body (resumo)|Sucesso|Erros de integração mais comuns|Notas de implementação cliente|
|---|---|---|---|---|---|---|---|---|
|POST|`/login`|Não|—|—|`identificador` + `senha`|`200` + token JWT|`400`, `401`|Salvar token + tipo de usuário retornado para controlar navegação.|
|POST|`/bootstrap`|Não|—|—|dados do 1º admin FPP|`201`|`403` se já existir FPP|Rota one-shot; no cliente, esconder após bootstrap concluído.|
|POST|`/email/verificar-email/:token`|Não|`token`|—|vazio|`200`|`400/404/410` token inválido/expirado|Fluxo ideal: abrir deep-link direto do email.|
|POST|`/email/verificar-email/solicitar`|Sim|—|—|identificador do usuário|`200`|`401/429`|Implementar cooldown no frontend para evitar spam.|
|POST|`/email/recuperar-senha/solicitar`|Não|—|—|email/código|`200`|`400/404/429`|Sempre mostrar mensagem neutra para evitar enumeração.|
|POST|`/email/recuperar-senha/:token`|Não|`token`|—|nova senha|`200`|`400/410`|Forçar política de senha no formulário antes do submit.|
|POST|`/email/gerar-token/recuperacao`|Não|—|—|identificador|`200`|`400/429`|Usado quando frontend envia email por conta própria.|
|POST|`/email/gerar-token/verificacao`|Sim|—|—|vazio|`200`|`401/429`|Exigir usuário logado e não verificado.|
|GET|`/jobs`|Sim|—|`limit`,`offset`,`status`|—|`200`|`401`|Polling para filas assíncronas com intervalo progressivo.|
|GET|`/jobs/:id`|Sim|`id`|—|—|`200`|`401/404`|Parar polling quando status final (`completed`/`failed`).|
|PUT|`/alterar-senha`|Sim|—|—|senha atual + nova|`200`|`400/401`|Após sucesso, forçar re-login por segurança.|
|GET|`/meu-perfil`|Sim|—|—|—|`200`|`401`|DTO varia por tipo (`admin`,`academia`,`estudante`).|
|GET|`/academias`|Sim|—|`limit`,`offset`,`status`|—|`200`|`401/400` query inválida|Paginar em cursor/offset; respeitar teto de `limit`.|
|GET|`/consultar-academia/:codigo`|Sim|`codigo`|—|—|`200`|`401/404`|Usar cache local por `codigo`.|
|GET|`/consultar-estudante/:codigo`|Sim (`academia/admin`)|`codigo`|—|—|`200`|`401/403/404`|Checar ownership no fluxo de academia.|
|GET|`/estudantes`|Sim (`academia/admin`)|—|`limit`,`offset`,`busca`*|—|`200`|`401/403`|Aplicar debounce em busca textual.|
|GET|`/notas-estudante/:codigo`|Sim (`academia/admin`)|`codigo`|filtros opcionais*|—|`200`|`401/403/404`|Ordenar por período/data no cliente.|
|GET|`/faltas-estudante/:codigo`|Sim (`academia/admin`)|`codigo`|filtros opcionais*|—|`200`|`401/403/404`|Somar `quantidade` por período para dashboards.|
|GET|`/avaliacoes-estudante/:codigo`|Sim (`academia/admin`)|`codigo`|—|—|`200`|`401/403/404`|Exibir trilha histórica por ano letivo.|
|GET|`/avaliacoes`|Sim|—|`limit`,`offset`,`tipo_ensino`*|—|`200`|`401`|Tela global; para estudante tende a retornar escopo próprio.|
|GET|`/aprovacoes`|Sim|—|`limit`,`offset`|—|`200`|`401`|Filtro server-side `aprovado=true`.|
|GET|`/reprovacoes`|Sim|—|`limit`,`offset`|—|`200`|`401`|Filtro server-side `aprovado=false`.|
|GET|`/verificar-integridade/:codigo`|Sim|`codigo`|—|—|`200`|`401/404`|Use para auditoria; não para fluxo transacional.|
|GET|`/eventos-estudante/:codigo`|Sim (`admin`)|`codigo`|`limit`,`offset`|—|`200`|`401/403/404`|Pode gerar payload grande; pagine sempre.|
|POST|`/adicionar-telefone-extra`|Sim|—|—|`numero_telefone`|`201`|`400/409`|Normalizar localmente melhora UX, mas backend também normaliza.|
|PUT|`/estudante/dados-pessoais`|Sim (`estudante`)|—|—|dados pessoais|`200`|`400/401`|Atualização parcial; enviar somente campos alterados quando possível.|
|GET|`/estudante/minhas-notas`|Sim (`estudante`)|—|filtros opcionais*|—|`200`|`401`|Renderizar por ano/período.|
|GET|`/estudante/minhas-faltas`|Sim (`estudante`)|—|filtros opcionais*|—|`200`|`401`|Exibir consolidado e itens detalhados.|
|GET|`/estudante/minhas-avaliacoes`|Sim (`estudante`)|—|—|—|`200`|`401`|Exibir histórico com status aprovado/reprovado.|
|PUT|`/academia/dados`|Sim (`academia ativa`)|—|—|dados cadastrais|`200`|`400/401/403`|Se mudar email, pode exigir nova verificação.|
|POST|`/academia/ano-letivo`|Sim (`academia ativa`)|—|—|`ano_letivo`,`tipo`|`200`|`400/403`|Pré-requisito para notas/faltas/avaliações.|
|GET|`/academia/ano-letivo`|Sim (`academia ativa`)|—|—|—|`200`|`401/403/404`|Ao iniciar app da academia, buscar este endpoint primeiro.|
|POST|`/academia/estudante/register`|Sim (`academia ativa`)|—|—|cadastro completo do estudante|`201`|`400/409`|Persistir `codigo_estudante` retornado para operações futuras.|
|POST|`/academia/notas-aluno`|Sim (`academia ativa`)|—|—|dados da nota|`201`|`400/403/409`|Sem ano letivo ativo retorna `400`.|
|PUT|`/academia/atualizar-nota`|Sim (`academia ativa`)|—|—|`id`,`nota`,`observacao`|`200`|`400/404`|`observacao` obrigatória na correção.|
|DELETE|`/academia/nota/:id`|Sim (`academia ativa`)|`id`|—|`motivo`|`200`|`400/404`|Soft delete: item some das listas ativas.|
|POST|`/academia/faltas-aluno`|Sim (`academia ativa`)|—|—|dados da falta|`201`|`400/403/409`|`quantidade` deve ser positiva.|
|PUT|`/academia/atualizar-falta`|Sim (`academia ativa`)|—|—|`id`,`quantidade`,`observacao`*|`200`|`400/404`|Usar confirmação antes de editar histórico.|
|DELETE|`/academia/falta/:id`|Sim (`academia ativa`)|`id`|—|`motivo`|`200`|`400/404`|`motivo` obrigatório.|
|POST|`/academia/avaliacao-final`|Sim (`academia ativa`)|—|—|dados de avaliação final|`201`|`400/403/409`|Remove estudante das turmas automaticamente após registrar.|
|POST|`/academia/categorias-nota`|Sim (`academia ativa`)|—|—|`nome`,`descricao`*|`201`|`400/409`|Sincronizar catálogo no cliente após criação.|
|GET|`/academia/categorias-nota`|Sim (`academia ativa`)|—|—|—|`200`|`401/403`|Carregar no formulário de lançamento de notas.|
|PUT|`/academia/estudante/:codigo/status-escolar-fundamental`|Sim (`academia ativa`)|`codigo`|—|`novo_status`|`200`|`400/404/409`|Respeitar progressão do ciclo.|
|PUT|`/academia/estudante/:codigo/status-escolar-medio`|Sim (`academia ativa`)|`codigo`|—|`novo_status`|`200`|`400/404/409`|Validar dependência de status fundamental.|
|PUT|`/academia/estudante/:codigo/status-superior`|Sim (`academia ativa`)|`codigo`|—|`novo_status`|`200`|`400/404/409`|Somente após ciclos anteriores compatíveis.|
|POST|`/academia/curso`|Sim (`academia ativa`)|—|—|`nome`,`type`,`anos`,`periodos`*|`201`|`400/409`|`type` é imutável após criação.|
|GET|`/academia/cursos`|Sim (`academia ativa`)|—|filtros opcionais*|—|`200`|`401/403`|Fonte principal para combos de matéria/turma.|
|GET|`/academia/curso/:id`|Sim (`academia ativa`)|`id`|—|—|`200`|`404`|Validar UUID no cliente evita round-trip.|
|PUT|`/academia/curso/:id/ativar`|Sim (`academia ativa`)|`id`|—|vazio|`200`|`404/409`|Só cursos inativos podem ativar.|
|PUT|`/academia/curso/:id/desativar`|Sim (`academia ativa`)|`id`|—|vazio|`200`|`404/409`|Pré-requisito para deleção.|
|PUT|`/academia/curso/:id/dados`|Sim (`academia ativa`)|`id`|—|campos editáveis|`200`|`400/404`|Não tentar alterar `type`.|
|DELETE|`/academia/curso/:id`|Sim (`academia ativa`)|`id`|—|vazio|`200`|`404/409`|Falha se curso ativo ou com vínculos impeditivos.|
|POST|`/academia/materia`|Sim (`academia ativa`)|—|—|dados da matéria|`201`|`400/409`|Para `superior`, período pode ser definido depois.|
|GET|`/academia/materias`|Sim (`academia ativa`)|—|filtros opcionais*|—|`200`|`401/403`|Use filtro por curso/tipo para formularios.|
|GET|`/academia/materia/:id`|Sim (`academia ativa`)|`id`|—|—|`200`|`404`|Detalhe para edição.|
|PUT|`/academia/materia/:id/ativar`|Sim (`academia ativa`)|`id`|—|vazio|`200`|`404/409`|Matéria superior sem período retorna conflito.|
|PUT|`/academia/materia/:id/desativar`|Sim (`academia ativa`)|`id`|—|vazio|`200`|`404/409`|Necessário para deleção.|
|PUT|`/academia/materia/:id/periodo`|Sim (`academia ativa`)|`id`|—|`periodo`|`200`|`400/404/409`|Somente `superior`; deve existir no curso.|
|PUT|`/academia/materia/:id/dados`|Sim (`academia ativa`)|`id`|—|campos editáveis|`200`|`400/404`|Atualização focada em metadados.|
|DELETE|`/academia/materia/:id`|Sim (`academia ativa`)|`id`|—|vazio|`200`|`404/409`|Soft delete.|
|POST|`/academia/turma`|Sim (`academia ativa`)|—|—|dados da turma|`201`|`400/409`|`codigo_turma` deve ser único na academia.|
|GET|`/academia/turmas`|Sim (`academia ativa`)|—|filtros opcionais*|—|`200`|`401/403`|Carregar para gestão de matrícula.|
|GET|`/academia/turma/:codigo`|Sim (`academia ativa`)|`codigo`|—|—|`200`|`404`|Código de turma é chave funcional.|
|PUT|`/academia/turma/:codigo/ativar`|Sim (`academia ativa`)|`codigo`|—|vazio|`200`|`404/409`|Respeitar estados permitidos.|
|PUT|`/academia/turma/:codigo/desativar`|Sim (`academia ativa`)|`codigo`|—|vazio|`200`|`404/409`|Para deletar, deve estar inativa.|
|PUT|`/academia/turma/:codigo/dados`|Sim (`academia ativa`)|`codigo`|—|campos editáveis|`200`|`400/404`|Evitar mudar código sem necessidade.|
|DELETE|`/academia/turma/:codigo`|Sim (`academia ativa`)|`codigo`|—|vazio|`200`|`404/409`|Falha se houver estudantes vinculados.|
|POST|`/academia/turma/:codigo/estudante`|Sim (`academia ativa`)|`codigo`|—|`codigo_estudante`|`200`|`400/404/409`|Permite estudante em múltiplas turmas.|
|DELETE|`/academia/turma/:codigo/estudantes/:codigo_estudante`|Sim (`academia ativa`)|`codigo`,`codigo_estudante`|—|vazio|`200`|`404`|Operação idempotente recomendada no cliente.|
|POST|`/dominis/register`|Sim (`admin`)|—|—|dados do admin|`201`|`400/403/409`|Role alvo deve ser inferior ao criador.|
|POST|`/dominis/academia/register`|Sim (`admin`)|—|—|dados da academia|`201`|`400/409`|Cria academia inicialmente inativa.|
|PUT|`/dominis/academia/:codigo/ativar`|Sim (`adm/fpp`)|`codigo`|—|vazio|`200`|`404/409`|Somente admin pode ativar academia.|
|PUT|`/dominis/academia/:codigo/desativar`|Sim (`adm/fpp`)|`codigo`|—|`motivo`*|`200`|`404/409`|Registrar motivo para trilha administrativa.|
|PUT|`/dominis/admin/:id/ativar`|Sim (`adm/fpp`)|`id`|—|vazio|`200`|`404/409`|Não permitir auto-lock em fluxos UI.|
|PUT|`/dominis/admin/:id/desativar`|Sim (`adm/fpp`)|`id`|—|vazio|`200`|`404/409`|Garantir pelo menos um admin operacional.|
|GET|`/dominis/admin-lista`|Sim (`admin`)|—|`limit`,`offset`|—|`200`|`401/403`|Tela de gestão administrativa.|
|GET|`/dominis/metrics`|Sim (`admin`)|—|—|—|`200`|`401/403`|Pode ser usado em dashboard.|
|POST|`/dominis/projections/rebuild/:name`|Sim (`fpp`)|`name`|—|vazio|`200`|`403/404/409`|Operação sensível; mostrar confirmação forte.|
|GET|`/dominis/consultar-admin/:email`|Sim (`admin`)|`email`|—|—|`200`|`404`|Encode URL do email corretamente.|
|GET|`/dominis/registros`|Sim (`admin`)|—|`limit`,`offset`,`tipo`*|—|`200`|`401/403`|Consulta central de notas/faltas.|
|GET|`/dominis/registros/:codigo`|Sim (`admin`)|`codigo`|`limit`,`offset`|—|`200`|`404`|Detalhamento por estudante.|
|PUT|`/dominis/admin/:id/role`|Sim (`fpp`)|`id`|—|`role`|`200`|`400/403/404`|Mudança de privilégio: auditar no cliente.|
|PUT|`/dominis/admin/:id/dados`|Sim (`admin`)|`id`|—|campos editáveis|`200`|`400/403/404`|Validação de email único.|

\* Parâmetros/fields opcionais dependem de versão do handler e contexto de uso.

### 9.3 Rotas batch e async (faltantes adicionadas)

Além das rotas síncronas já listadas, o backend possui contratos para operações em massa com duas modalidades:

1. **Batch síncrono** (`/batch`): responde na mesma requisição com `200`, `207` ou `422`.
2. **Batch assíncrono** (`/async`): cria job e retorna imediatamente; o cliente acompanha em `GET /jobs` e `GET /jobs/:id`.

#### 9.3.1 Endpoints `/async` disponíveis

##### Academia
- `POST /academia/estudante/register/async`
- `POST /academia/notas-aluno/async`
- `PUT /academia/atualizar-nota/async`
- `DELETE /academia/nota/async`
- `POST /academia/faltas-aluno/async`
- `PUT /academia/atualizar-falta/async`
- `DELETE /academia/falta/async`
- `POST /academia/avaliacao-final/async`
- `PUT /academia/estudante/status-escolar/async`
- `POST /academia/curso/async`
- `POST /academia/materia/async`
- `POST /academia/turma/async`
- `POST /academia/turma/estudante/async`

##### Admin
- `POST /dominis/academia/register/async`
- `PUT /dominis/academia/ativar/async`
- `PUT /dominis/academia/desativar/async`

#### 9.3.2 Contrato de resposta esperado para `/async`

```json
{
  "job_id": "uuid",
  "status": "queued",
  "message": "processamento agendado"
}
```

Fluxo recomendado no cliente:
- Criar job em `/async`.
- Fazer polling em `GET /jobs/:id` com backoff (ex.: 2s, 4s, 8s, máximo 20s).
- Encerrar polling quando `status` for final.
- Se `failed`, exibir erros por item retornados pelo job.

---

## 10. Regras de Design Invioláveis

Estas regras são obrigatórias em todo o código do sistema e não podem ser violadas em nenhuma circunstância:

**Event Sourcing:**

- Toda mutação de estado passa pelo ledger antes de atualizar qualquer projeção — sem exceção.
- `Apply()` sempre chamado após `RaiseEvent()` em todos os comandos do aggregate.
- `applyXxx()` sempre deserializa via `json.Marshal → json.Unmarshal` (nunca cast direto do payload) — garante que o rebuild funciona.
- Erros de `json.Unmarshal` sempre retornados com `fmt.Errorf("applyXxx: unmarshal error: %w", err)` — nunca silenciados.
- O switch `Apply()` de cada aggregate deve cobrir **todos** os eventos que ele emite — sem evento órfão.

**Ledger:**

- Nenhum `UPDATE` ou `DELETE` no `spuri_ledger` — é imutável.
- O schema do `spuri_ledger` nunca é alterado.
- `EventType` string nunca renomeado — quebra o ledger histórico e o rebuild.
- Campos de eventos nunca removidos — apenas adicionados como ponteiros (`*string`, `*uuid.UUID`) com valor nil-safe.

**Whitelist (`safe_queries.go`):**

- A whitelist deve estar sincronizada com todos os eventos que os aggregates emitem.
- Qualquer novo evento requer adição simultânea à whitelist antes de ser emitido em produção.
- Aggregate types igualmente controlados — nenhum novo tipo sem adição à whitelist.

**Segurança:**

- `senha_hash`, tokens e dados sensíveis nunca retornados no body de resposta HTTP.
- `AuditContext` (`UserID`, `UserType`, `IP: c.ClientIP()`) preenchido em todos os `SaveWithAudit`.
- Validações de autorização (pertence à academia, role correto) sempre antes do comando.
- Comparação bcrypt sempre executada independentemente de o usuário existir (anti-timing attack).

**Batch:**

- Endpoints batch nunca modificam a lógica dos handlers individuais.
- Ausência de atomicidade entre itens é proposital — documentar para o cliente.
- Limites de tamanho de array devem ser respeitados (veja seção 8).
- `newFakeContext` propaga exclusivamente as chaves listadas no arquivo `batch_context.go` — nunca copiar valores sensíveis além de `user_id`, `user_type`, `admin_role`, `dbClient`, `repository`, `projManager`, `request_id`.

**Qualidade:**

- Handler deve ser fino: extrair contexto, validar input, chamar aggregate, salvar, responder. Sem lógica de domínio.
- Aggregate sem imports de `gin`, sem chamadas HTTP, sem acesso a banco.
- Projeções sem lógica de negócio — apenas materialização de eventos.
- `c.Abort()` sempre chamado junto com `c.JSON` nos handlers de erro de middleware.

**Formatos canônicos imutáveis:**

- Código de estudante: `AAA1234` (3 letras maiúsculas + 4 dígitos)
- Código de academia: `{PROV}{ANO}{SEQ}` (ex: `LDA20261`)
- Ano fundamental: `[1-9]_ano_fundamental`
- Ano médio: `[n]_ano_medio`
- Ano superior: `[n]_ano_superior`
- Período semestral: `[n]_semestre`
- Períodos trimestrais (fixos do sistema): `1_trimestre`, `2_trimestre`, `3_trimestre`
- Ano letivo: `YYYY_YYYY` onde o segundo é exatamente o primeiro + 1
