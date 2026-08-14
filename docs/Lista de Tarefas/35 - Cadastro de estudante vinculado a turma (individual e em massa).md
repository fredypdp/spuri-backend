---
criado: 2026-08-13 00:00
atualizado: 2026-08-14 00:00
origem: Solicitação direta do dono do produto (orquestrado via Claude, execução via Codex)
status: feito
repositorio: fredypdp/spuri-backend (branch main)
---

# Cadastro de estudante já vinculado a uma turma (feito)

## Prompt recomendado para executar a atualização

Implemente o cadastro de estudante (individual e em massa) já vinculado a uma turma no momento do registro, reaproveitando integralmente a validação de compatibilidade estudante↔turma e o comando `AdicionarEstudanteNoAnoLectivo` já existentes no agregado `Turma`. A turma deve ser validada **antes** de qualquer efeito colateral (antes de criar o estudante, fazer upload de documentos ou gravar o evento de criação do estudante), para que um `codigo_turma` inválido nunca produza um estudante órfão de validação. A vinculação em si só pode acontecer **depois** que o estudante já existe e já foi persistido com sucesso (`repository.SaveWithAudit`) — nunca antes, e nunca como pré-condição para a criação do estudante. Isso é obrigatório porque a projeção de estudantes é eventualmente consistente e leva um tempo considerável para refletir um cadastro recém-feito: **a etapa de vinculação não pode depender de reconsultar a projeção do estudante recém-criado** (`estudanteProj.GetByCodigo`) logo em seguida, sob risco de encontrar `nil`/`404` mesmo com o cadastro já efetivado com sucesso. Use, em vez disso, os dados que já estão em memória na própria requisição de cadastro (o mesmo `ano_escolar`/`curso`/etc. que acabou de ser usado para criar o estudante) para alimentar a validação de compatibilidade e a vinculação — sem depender da leitura de nenhuma projeção do lado do estudante. Reaproveite a lógica de `AdicionarEstudanteATurma` (`internal/handlers/turmas_handler.go`) extraindo-a para uma função interna compartilhada em vez de duplicar código. Trate a falha rara da etapa de vinculação pós-criação como degradação graciosa (o estudante já registrado não pode ser desfeito num sistema de event sourcing), devolvendo aviso explícito na resposta em vez de erro genérico ou falha silenciosa. Propague o campo pelos dois fluxos reais de cadastro que existem hoje — individual (`POST /academia/estudante/register`) e em massa (`POST /academia/estudante/register/async`, sempre assíncrono via fila de jobs, limite de 100 itens) —, sem criar caminhos de código divergentes entre eles. Ao final, atualize testes, `Documentação da API.md` e qualquer outra documentação afetada. Não criar aliases, wrappers de compatibilidade nem fallback silencioso para o comportamento antigo (cadastro sem turma continua funcionando normalmente quando `codigo_turma` não é informado, pois o campo é opcional).

## Contexto

Hoje, vincular um estudante a uma turma é sempre uma segunda operação manual, feita depois do cadastro:

1. `POST /academia/estudante/register` (individual, JSON ou multipart) cria o estudante — sem qualquer noção de turma. Internamente, converge para `registerEstudantePorAcademiaComRequestModo` (`internal/handlers/estudante_handlers.go`).
2. `POST /academia/estudante/register/async` (em massa, **sempre assíncrono**, limite de 100 itens — `validarTamanhoBatch(len(items), 100)`) recebe o lote, enfileira um job (`jobs.JobTypeRegisterEstudanteBatch`) e responde imediatamente; o worker processa os itens depois, um a um, chamando `RegisterEstudantePorAcademiaJobItem` (`internal/handlers/job_item_handlers.go`), que por sua vez chama exatamente a mesma `registerEstudantePorAcademiaComRequestModo`. **Não existe** uma rota `POST /academia/estudante/register/batch`, nem um caminho síncrono separado para lote — o único caminho de lote é o assíncrono via `/register/async`. (Existe uma função `processarCadastroEstudanteBatch` em `internal/handlers/batch_handlers.go`, mas ela não é chamada por nenhuma rota nem pelo worker de jobs — é código morto; não usá-la como referência de fluxo real.)
3. Só depois, `POST /academia/turma/:codigo/estudante` (individual) ou `POST /academia/turma/estudante/async` + `AdicionarEstudanteATurmaJobItem` (em massa) vinculam o estudante já existente a uma turma.

Isso significa que, entre os dois passos, existe uma janela em que o estudante está cadastrado mas não pertence a nenhuma turma, exige uma segunda chamada manual da academia (fácil de esquecer, principalmente em cadastro em massa de dezenas/centenas de estudantes) e não há nenhuma garantia de que o cadastro em massa realmente termine com todos os estudantes em suas turmas corretas.

**Ponto crítico de arquitetura — consistência eventual da projeção de estudantes**: `AdicionarEstudanteATurma` hoje localiza o estudante via `estudanteProj.GetByCodigo(req.CodigoEstudante)`, lendo o modelo de leitura (projeção), que é reconstruído de forma assíncrona a partir do ledger de eventos e **leva um tempo considerável** para refletir um cadastro recém-feito. Isso é seguro na rota manual porque, na prática, ela é chamada bem depois do cadastro (o estudante já teve tempo de aparecer na projeção). Mas **não pode** ser reaproveitado da mesma forma logo após criar o estudante nesta tarefa: se a vinculação, executada imediatamente após `SaveWithAudit` do estudante, tentar reconsultar `estudanteProj.GetByCodigo`, é bem provável que ainda não encontre o registro — não por erro de negócio, mas por atraso de propagação da projeção. A solução é não depender da projeção nesse ponto: os únicos dados que `AdicionarEstudanteATurma` de fato extrai do `EstudanteDTO` são `CodigoAcademia`, `AnoEscolar`, `AnoEscolarMedio`, `AnoSuperior`, `CursoMedioID` e `CursoSuperiorID` — e **todos eles já estão disponíveis, no próprio processo, a partir do `CadastroEstudanteAcademiaRequest` que acabou de ser usado para criar o estudante**. Portanto, a etapa de vinculação pós-criação deve usar esses valores em memória diretamente, sem nenhuma leitura de projeção do lado do estudante.

O agregado `Turma` (`internal/domain/aggregates/turma.go`) já expõe `AdicionarEstudanteNoAnoLectivo(codigoEstudante, anoLectivo string, adicionadoPor uuid.UUID) error`, que:

- rejeita duplicidade do mesmo estudante na mesma turma;
- emite o evento `EstudanteAdicionadoATurma`, consumido por `internal/projections/turmas_projection.go`.

O handler `AdicionarEstudanteATurma` (`internal/handlers/turmas_handler.go`, a partir da linha ~532) já concentra toda a validação necessária:

- turma pertence à academia autenticada e existe (`turmasProj.GetByCodigoTurma`);
- estudante não pertence a nenhuma outra turma ativa (`turmasProj.ListByEstudante` + verificação de status `!= "deletado"`);
- compatibilidade estudante↔turma via `validarCompatibilidadeEstudanteTurma` (nível, curso, ano escolar/médio/superior);
- resolução do ano letivo ativo via `resolverAnoLetivoAcademia`;
- carregamento do agregado via `repository.Load(turmaDTO.ID, "Turma")` e persistência via `repository.SaveWithAudit`.

Note que **nenhuma dessas consultas depende da projeção de estudantes** — todas são sobre a projeção de turmas (`turmasProj`) e sobre o agregado `Turma`, que não sofrem do mesmo atraso relevante para um estudante recém-criado. O único ponto que hoje depende da projeção de estudantes é exatamente a leitura de `estudanteDTO` no início do handler — e é justamente essa leitura que deve ser eliminada do novo caminho (pós-criação), substituída pelos dados já disponíveis em memória.

Não existe hoje limite de capacidade de turma no agregado `Turma` (não há campo de vagas máximas), então a única invariante de negócio relevante além da compatibilidade já validada é "um estudante pertence a no máximo uma turma por vez" — que, para um estudante **recém-criado**, é sempre trivialmente verdadeira (um estudante que acabou de nascer no sistema não pode já pertencer a nenhuma turma), então essa checagem específica pode ser dispensada com segurança no novo caminho, evitando uma consulta desnecessária à projeção de turmas por estudante.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Novo campo | `codigo_turma` (opcional) em `CadastroEstudanteAcademiaRequest` | Disponível simultaneamente em cadastro individual e em massa (que reaproveita a mesma struct via `cadastroEstudanteJSONItem.toCadastroRequest()`) |
| Ordem de validação da turma | Validada **antes** de criar o estudante (existência, pertencimento à academia, status ativo, compatibilidade a partir dos dados do request) | `codigo_turma` inválido nunca chega a criar estudante nem fazer upload de documento |
| Ordem de vinculação | Vinculação só acontece **depois** que o estudante já foi criado e persistido com sucesso | Nenhuma vinculação é tentada antes de o cadastro estar garantido |
| Fonte de dados da vinculação | Campos do `CadastroEstudanteAcademiaRequest` já em memória — **nunca** uma releitura de `estudanteProj.GetByCodigo` logo após a criação | Imune ao atraso de propagação da projeção de estudantes |
| Reaproveitamento de lógica | Extrai a lógica de `AdicionarEstudanteATurma` para uma função interna compartilhada, parametrizada por valores primitivos (não por `*EstudanteDTO`) | Nenhuma duplicação de regra de compatibilidade/duplicidade; a rota manual continua funcionando como hoje |
| Falha rara pós-criação | Estudante permanece criado; resposta sinaliza `turma_vinculada: false` com motivo e orientação de retry via a rota manual | Nenhuma tentativa de "desfazer" um evento já gravado no ledger |
| Fluxos reais de cadastro | Apenas dois: individual (`POST /academia/estudante/register`) e em massa assíncrono (`POST /academia/estudante/register/async`, limite 100, sempre via job) | Nenhum caminho de código divergente entre eles — ambos convergem para `registerEstudantePorAcademiaComRequestModo` |
| Migração de schema | Nenhuma | O vínculo estudante↔turma já é modelado inteiramente via evento `EstudanteAdicionadoATurma` na projeção de turmas; não há nova coluna a criar |

---

# 1. Validação da turma antes de qualquer efeito colateral

## Objetivo

Garantir que, quando `codigo_turma` for informado, toda a validação de existência/pertencimento/status/compatibilidade da turma aconteça **antes** de gerar código de estudante, fazer upload de documentos, gravar o evento de criação do estudante ou reservar qualquer guarda de unicidade.

## Escopo obrigatório

### 1.1 Local da pré-validação

Em `registerEstudantePorAcademiaComRequestModo` (`internal/handlers/estudante_handlers.go`), logo após a academia ser carregada e antes de `validarCursosMatriculaCommon`/upload de documentos, quando `req.CodigoTurma` (após `strings.TrimSpace`) não for vazio:

1. Carregar a turma via `turmasProj.GetByCodigoTurma(codigoTurma, academia.CodigoAcademia)`; se não existir, `404`. Esta é uma leitura da projeção de **turmas**, não da de estudantes — não sofre do mesmo atraso, pois a turma já existia antes desta requisição começar.
2. Validar que a turma pertence à mesma academia autenticada (já implícito na consulta acima, mas a mensagem de erro deve deixar claro que a turma não pertence à academia, não apenas "não encontrada", seguindo o padrão já usado em `AdicionarEstudanteATurma`).
3. Validar `turmaDTO.Status == "ativo"`; rejeitar com erro de validação claro se a turma estiver `inativo` ou `deletado` — hoje `AdicionarEstudanteATurma` não faz essa checagem explícita de status; adicione-a em ambos os lugares (pré-validação do cadastro e no helper compartilhado da seção 2), pois vincular um estudante recém-criado a uma turma inativa não faz sentido de negócio.
4. Validar compatibilidade estudante↔turma via `validarCompatibilidadeEstudanteTurma`, usando os dados do **request** (`req.AnoEscolar`, `req.AnoEscolarMedio`, `req.AnoSuperior`, `req.CursoMedioID`, `req.CursoSuperiorID`) — o estudante ainda não existe como agregado neste ponto, mas a função já aceita esses valores diretamente (não depende de um `EstudanteDTO` carregado), então a validação é possível sem ordem invertida e sem depender da projeção de estudantes.

Qualquer falha nesta etapa deve interromper o cadastro imediatamente, com o mesmo tipo de resposta de erro (`RespondWithValidationError`/`RespondWithNotFoundError`/`RespondWithForbiddenError`) já usado no restante do arquivo, e **sem** nenhum efeito colateral (nada de storage, nada de agregado criado).

### 1.2 Reaproveitamento entre os dois fluxos reais

`registerEstudantePorAcademiaComRequestModo` é chamada tanto pelo cadastro individual (`RegisterEstudantePorAcademia` → `registerEstudantePorAcademiaMultipart`/JSON) quanto pelo item de job do cadastro em massa (`RegisterEstudantePorAcademiaJobItem`). Como a pré-validação da seção 1.1 vive dentro dessa função compartilhada, nenhuma alteração adicional é necessária nos dois pontos de entrada além de garantir que `codigo_turma` seja lido corretamente de cada payload (ver seção 3): o campo de formulário/JSON no cadastro individual, e o campo `codigo_turma` em cada item de `cadastroEstudanteJSONItem` no cadastro em massa.

---

# 2. Vinculação do estudante à turma após a criação

## Objetivo

Depois que o estudante for criado com sucesso (`estudante.CriarComVinculo`/`CriarComVinculoPendenteDocumentos` + `repository.SaveWithAudit`), vincular imediatamente o novo `codigo_estudante` à turma já validada na seção 1 — **usando exclusivamente dados já disponíveis em memória**, sem reconsultar a projeção de estudantes.

## Escopo obrigatório

### 2.1 Extrair função interna compartilhada, parametrizada por valores primitivos

Refatore `AdicionarEstudanteATurma` (`internal/handlers/turmas_handler.go`) para extrair a parte que vai da checagem de duplicidade de turma (`turmasProj.ListByEstudante`) até `repository.SaveWithAudit` para uma função interna que **não recebe `*projections.EstudanteDTO`**, e sim os campos primitivos que de fato são usados (exatamente os que `validarCompatibilidadeEstudanteTurma` consome):

```go
func vincularEstudanteATurma(
    c *gin.Context,
    academiaDTO *projections.AcademiaDTO,
    codigoEstudante string,
    anoEscolar, anoEscolarMedio, anoSuperior string,
    cursoMedioID, cursoSuperiorID string,
    codigoTurma string,
    ignorarChecagemDuplicidade bool, // true quando o estudante acabou de ser criado
    atuadoPor uuid.UUID,
) error
```

- `AdicionarEstudanteATurma` (rota manual, inalterada em comportamento) passa a extrair esses campos de `estudanteDTO` (lido da projeção, como já faz hoje) e chama a função com `ignorarChecagemDuplicidade = false`.
- O novo caminho de cadastro (seção 2.2) chama a mesma função passando os campos diretamente de `req` (`CadastroEstudanteAcademiaRequest`) e `ignorarChecagemDuplicidade = true`, pulando a consulta `turmasProj.ListByEstudante` (desnecessária para um estudante recém-criado, conforme justificado no Contexto).

Isso evita duas implementações divergentes da mesma regra de negócio e deixa explícito, na assinatura da função, que ela nunca depende da projeção de estudantes.

### 2.2 Chamada após a criação do estudante

Em `registerEstudantePorAcademiaComRequestModo`, imediatamente após `repository.SaveWithAudit(estudante, audit)` ter sucesso (e depois do guard de BI ser consumido, para não deixar reserva presa em caso de erro na vinculação):

- se `req.CodigoTurma` estiver vazio, seguir o fluxo atual sem nenhuma mudança;
- se estiver preenchido, chamar `vincularEstudanteATurma` passando `codigoEstudante` (o código recém-gerado para o novo estudante) e os campos `req.AnoEscolar`, `req.AnoEscolarMedio`, `req.AnoSuperior`, `req.CursoMedioID`, `req.CursoSuperiorID` — **nunca** uma releitura via `getEstudanteProjection(c).GetByCodigo(codigoEstudante)`. O estudante já foi criado com sucesso nesse ponto; a vinculação é uma operação exclusivamente sobre o agregado `Turma`, que não precisa "ver" o estudante na projeção para vinculá-lo — só precisa do código dele e dos atributos usados na validação de compatibilidade, ambos já em mãos.

### 2.3 Degradação graciosa em caso de falha

Como o evento de criação do estudante já foi gravado no ledger (imutável), uma falha em `vincularEstudanteATurma` **não pode** desfazer o cadastro. Trate como sucesso parcial:

- responder `201 Created` normalmente (o estudante foi criado com sucesso);
- incluir no corpo da resposta `codigo_turma`, `turma_vinculada: false` e `turma_aviso` com o motivo textual da falha (ex.: turma desativada entre a validação e a criação, conflito de versão otimista no agregado `Turma`, etc.);
- orientar explicitamente no `turma_aviso` que a academia pode tentar novamente via `POST /academia/turma/:codigo/estudante`;
- logar o evento com `log.Printf` no mesmo padrão já usado no arquivo, incluindo `codigo_estudante` e `codigo_turma`, para permitir auditoria operacional.

Isso não deve virar uma nova propriedade persistente no agregado `Estudante` (não é o mesmo conceito de `status: "pendente_documentos"`); é puramente uma resposta HTTP informativa — o estado real de vínculo com turma sempre vive na projeção de turmas, que pode ser consultada a qualquer momento por `GET /turmas-estudante/:codigo`.

### 2.4 Conflito de concorrência otimista

Como `vincularEstudanteATurma` faz `repository.Load` seguido de `repository.SaveWithAudit` sobre o agregado `Turma`, um conflito de versão otimista é possível se outra requisição alterar a mesma turma entre o carregamento e a gravação. Implemente **uma única tentativa de retry automático** (recarregar o agregado e tentar de novo) antes de desistir e cair no caminho da seção 2.3 — o mesmo espírito de resiliência a corridas já discutido na tarefa `docs/Tarefas feitas/14 - Bloquear duplicidade concorrente antes da projeção.md`, mas sem reintroduzir mutex em memória nem qualquer mecanismo dependente de uma única instância do processo.

---

# 3. Contrato de API

## 3.1 `POST /academia/estudante/register` (individual — JSON ou multipart)

Novo campo opcional: `codigo_turma`.

```json
{
  "nome": "Maria Silva",
  "ano_escolar_fundamental": "6_ano",
  "codigo_turma": "TURMA-A"
}
```

No multipart, o mesmo campo como campo de formulário: `codigo_turma=TURMA-A`.

## 3.2 Cadastro em massa — `POST /academia/estudante/register/async` (sempre assíncrono, limite de 100 itens)

Adicionar `CodigoTurma string \`json:"codigo_turma"\`` a `cadastroEstudanteJSONItem` (`internal/handlers/job_item_handlers.go`) e propagar para `CadastroEstudanteAcademiaRequest` dentro de `toCadastroRequest()`. Como cada item do lote é processado por um job independente (`RegisterEstudantePorAcademiaJobItem`, um item por execução), o comportamento de validação/vinculação/degradação graciosa das seções 1 e 2 se aplica individualmente a cada estudante do lote, sem afetar os demais itens.

```json
{
  "com_arquivo": false,
  "estudantes": [
    {
      "nome": "Maria Silva",
      "genero": "feminino",
      "data_nascimento": "2012-04-10",
      "ano_escolar_fundamental": "6_ano",
      "codigo_turma": "TURMA-A"
    }
  ]
}
```

## 3.3 Resposta — sucesso completo

```json
{
  "message": "estudante registrado com sucesso",
  "data": {
    "id": "uuid-estudante",
    "codigo_estudante": "EST-2026-0001",
    "codigo_academia": "ACAD001",
    "status": "ativo",
    "codigo_turma": "TURMA-A",
    "turma_vinculada": true
  }
}
```

## 3.4 Resposta — estudante criado, vinculação falhou

```json
{
  "message": "estudante registrado com sucesso",
  "data": {
    "id": "uuid-estudante",
    "codigo_estudante": "EST-2026-0001",
    "codigo_academia": "ACAD001",
    "status": "ativo",
    "codigo_turma": "TURMA-A",
    "turma_vinculada": false,
    "turma_aviso": "não foi possível vincular à turma 'TURMA-A': <motivo>. Use POST /academia/turma/TURMA-A/estudante para tentar novamente."
  }
}
```

Quando `codigo_turma` não for informado, a resposta permanece exatamente como é hoje (sem os campos `codigo_turma`/`turma_vinculada`), preservando compatibilidade para quem não usa o novo campo. No cadastro em massa (job assíncrono), o mesmo formato de `data` aparece no resultado individual de cada item, no local onde o job já expõe o resultado por item hoje.

---

# 4. Testes obrigatórios

1. cadastro individual sem `codigo_turma` — comportamento idêntico ao atual (regressão);
2. cadastro individual com `codigo_turma` válido e compatível — estudante criado e vinculado, `turma_vinculada: true`;
3. cadastro individual com `codigo_turma` inexistente — `404`, nenhum estudante criado;
4. cadastro individual com `codigo_turma` de outra academia — erro de autorização, nenhum estudante criado;
5. cadastro individual com `codigo_turma` de turma inativa/deletada — erro de validação, nenhum estudante criado;
6. cadastro individual com `codigo_turma` incompatível (nível/curso/ano) — erro de validação com a mesma mensagem de `validarCompatibilidadeEstudanteTurma`, nenhum estudante criado;
7. teste explícito comprovando que a etapa de vinculação **não** dispara nenhuma leitura à projeção de estudantes (ex.: via mock/spy no `estudanteProj` que falha o teste se `GetByCodigo` for chamado durante o fluxo de vinculação pós-criação) — este é o teste que protege contra regressão do problema de atraso de propagação relatado;
8. cadastro em massa (`POST /academia/estudante/register/async`) com mistura de itens com e sem `codigo_turma` — cada item processado independentemente pelo worker, sem afetar os demais, e cada resultado individual reflete `turma_vinculada`;
9. simulação de falha da etapa de vinculação após o estudante já criado (ex.: turma desativada concorrentemente entre a pré-validação e a vinculação) — estudante permanece criado, resposta traz `turma_vinculada: false` e `turma_aviso`, e o estudante pode ser vinculado manualmente depois via `POST /academia/turma/:codigo/estudante`;
10. conflito de concorrência otimista no agregado `Turma` durante a vinculação — uma tentativa de retry automático resolve o caso comum; se persistir, cai no comportamento do item 9;
11. teste garantindo que `AdicionarEstudanteATurma` (rota manual já existente) continua funcionando sem regressão após a extração da função compartilhada, incluindo a checagem de duplicidade de turma que só se aplica a esse caminho.

---

# 5. Documentação obrigatória

Atualizar `Documentação da API.md`:

- seção **8.1 Cadastro de Estudante** e **`POST /academia/estudante/register`**: novo campo `codigo_turma` (opcional), novos campos de resposta `codigo_turma`/`turma_vinculada`/`turma_aviso`;
- seção referente a **`POST /academia/estudante/register/async`**: novo campo `codigo_turma` no item do array, e nota explícita de que a vinculação (quando solicitada) acontece de forma independente por item, sem depender de ordem de processamento entre estudantes do mesmo lote;
- seção **12. Turmas**, nota explícita em `POST /academia/turma/:codigo/estudante` esclarecendo que esta rota continua existindo para vincular/revincular estudantes já cadastrados, e que o cadastro agora também aceita vinculação direta via `codigo_turma`.

---

# Fora de escopo

- Criar limite de capacidade (vagas máximas) de turma — não existe hoje no agregado `Turma` e não é parte desta tarefa.
- Migrar retroativamente estudantes já cadastrados sem turma.
- Alterar ou remover a rota manual `POST /academia/turma/:codigo/estudante` — ela continua sendo o caminho para vincular estudantes já existentes ou para corrigir o caso de degradação graciosa da seção 2.3.
- Alterar a regra de "um estudante pertence a no máximo uma turma por vez".
- Ressuscitar ou reaproveitar `processarCadastroEstudanteBatch` — é código morto hoje e não deve virar parte do fluxo real como efeito colateral desta tarefa.
- Qualquer mudança no fluxo de solicitação de matrícula (diferente do cadastro direto pela academia).

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `codigo_turma` for aceito, opcionalmente, tanto no cadastro individual quanto no cadastro em massa, através de um único ponto de extensão compartilhado (`registerEstudantePorAcademiaComRequestModo`);
2. toda validação de turma (existência, pertencimento à academia, status ativo, compatibilidade) acontecer antes de qualquer efeito colateral do cadastro;
3. a etapa de vinculação pós-criação usar exclusivamente dados já disponíveis em memória (do `CadastroEstudanteAcademiaRequest`), sem nenhuma releitura da projeção de estudantes — comprovado pelo teste da seção 4, item 7;
4. a vinculação reaproveitar a mesma lógica de negócio de `AdicionarEstudanteATurma`, sem duplicação de regra;
5. uma falha rara na vinculação pós-criação nunca resultar em erro genérico nem em perda de rastreabilidade — sempre resposta clara com `turma_vinculada: false` e orientação de retry;
6. os testes da seção 4 estiverem implementados e passando;
7. `Documentação da API.md` estiver atualizada conforme a seção 5;
8. nenhuma migração de banco de dados desnecessária for introduzida (o vínculo continua modelado inteiramente por eventos).

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Cadastro de estudante já vinculado a uma turma (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
