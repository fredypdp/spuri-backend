---
criado: 11-09-2026
origem: Fredy + Claude (depuração completa do código, front-end e back-end)
status: feita
tipo: correção de segurança / privacidade (spuri-backend)
prioridade: ALTA — duas das três correções já vazam texto técnico para usuários finais não-administradores em produção agora
---

# Remover vazamento de mensagens técnicas em respostas de sucesso para usuários não-administradores

### Documento de execução para o Codex (localizado e pré-testado pelo Claude)

> **Este documento já contém tudo que é necessário. Você (Codex) não precisa investigar nada, nem
> decidir nada — apenas aplicar os três "localizar/substituir" da seção 3, exatamente como estão
> escritos, e depois rodar a seção 4.** Todas as três correções já foram escritas, revisadas e (onde
> possível no meu ambiente) validadas por mim antes de este documento existir. Se qualquer trecho de
> "localizar" abaixo não bater 100% com o que está no arquivo real (por exemplo, se o arquivo mudou
> entre a auditoria e a execução), pare e não improvise uma correção parecida — confira a seção 3.4
> ("se o texto não bater exatamente") antes de continuar.

## 0. Nota sobre o ambiente do Codex (leia antes de começar)

Sei que o seu ambiente bloqueia `apt` (403), não tem Docker nem `psql` — diferente do meu sandbox, onde
consigo instalar PostgreSQL de verdade. **Isso não importa para esta tarefa**: nenhuma das três
correções toca em SQL, migração, schema ou dado persistido — são só literais de string dentro de
respostas HTTP já existentes. Não há nada aqui que dependa de banco de dados.

A única coisa que o seu ambiente precisa e consegue fazer sozinho é compilar e rodar os testes normais
do Go (`go build`, `go vet`, `go test`) — isso não depende de rede nem de Postgres, só do módulo já
baixado localmente (o `go.sum` já existe no repositório). Trate a seção 4 como o verdadeiro portão de
qualidade: se `go build ./...` ou `go vet ./...` falhar depois das suas edições, algo foi digitado
errado — pare e confira contra o texto exato da seção 3 antes de seguir.

## 1. O que foi relatado e o que a auditoria encontrou

Fredy relatou que a página pública `/instituicoes/cadastrar` (frontend, repositório `spuripainel`)
mostra, ao final do cadastro de uma nova academia, uma mensagem contendo detalhe técnico de API, por
exemplo:

> "guarde o código da academia: ele é o seu identificador de login. você definiu sua própria senha no
> cadastro. alvará não enviado no cadastro. envie depois em POST /documentos/academias/LDA20268/alvara/upload."

Regra dele, que vale para todo o sistema: **nenhum usuário que não seja administrador deve receber
mensagens técnicas** (rota HTTP, verbo HTTP, nome de variável de ambiente, texto de erro interno, etc.).

Rastreei a origem: o frontend já foi corrigido hoje mesmo, antes desta tarefa, num commit separado (PR
"Hide technical API messages from non-admin users and simplify public signup message", já mesclada) —
esse commit parou de **exibir** o campo `aviso` na tela e adicionou sanitização automática para
mensagens de **erro** HTTP (`isTechnicalApiMessage` / `sanitizeApiMessageForCurrentUser` em
`spuripainel/src/lib/api/client.ts`). Isso não é parte desta tarefa — já está feito.

O problema é que a correção de frontend só resolve a **exibição**; o **backend** (este repositório,
`spuri-backend`) continua **construindo e enviando** o texto técnico dentro do corpo de respostas de
**sucesso** (200/201/202) — que nunca passam pela sanitização de erro do frontend, porque não são
erros. Fiz uma varredura em todo o `internal/handlers` procurando esse padrão (texto com verbo HTTP +
rota, tipo `POST /algo/coisa`) em qualquer campo de resposta, cruzando cada ocorrência com o
middleware da rota (`RequireFPP`, `RequireAdmin`, `RequireAcademia`, `RequireEstudante`, rota pública,
etc.) para separar o que é seguro (só chega a administradores) do que não é. Achei **três** ocorrências
que precisam de correção, descritas na seção 3. Todo o resto que usa um padrão parecido já está seguro
hoje — está listado, com o motivo, na seção 5 ("Fora de escopo"), para que você não mexa nisso por
engano.

## 2. Resumo executivo

| # | Arquivo | Rota afetada | Quem recebe hoje | Severidade |
| - | --- | --- | --- | --- |
| 1 | `internal/handlers/academia_handlers.go` | `POST /academia/cadastro` (pública, sem autenticação) | **Qualquer visitante anônimo** que se autocadastra como academia | CRÍTICA — é o bug relatado |
| 2 | `internal/handlers/estudante_handlers.go` | `POST /academia/estudante/register` (autenticada, role `academia`) | Qualquer **academia** autenticada | CRÍTICA — achado novo na auditoria, já ativo hoje, e ainda pior (interpola erro Go bruto) |
| 3 | `internal/handlers/async_batch_handlers.go` | `POST /academia/*/async` (autenticada, role `academia`) e as rotas equivalentes de admin | Qualquer **academia** ou admin autenticado | Baixa — hoje nenhum componente do frontend exibe esse campo; é correção preventiva |

Nas três, a mudança é **só o texto** de um campo de string dentro de uma resposta que já existe — nenhum
campo é adicionado, removido ou renomeado, nenhuma assinatura de função muda, nenhum novo import é
necessário.

## 3. Localizar/substituir

### 3.1 Correção 1 (CRÍTICA) — `RegisterAcademiaPublica`

**Arquivo:** `internal/handlers/academia_handlers.go`

**Contexto:** esta é a função por trás de `POST /academia/cadastro`, a rota **pública, sem
autenticação**, usada pela página `/instituicoes/cadastrar` do frontend. É exatamente o bug relatado
por Fredy. A versão atual expõe o verbo HTTP e a rota interna de upload de documento
(`POST /documentos/academias/<codigo>/alvara/upload`) dentro do campo `aviso` da resposta 201.

**Localizar (texto exato):**

```go
	aviso := "guarde o código da academia: ele é o seu identificador de login. você definiu sua própria senha no cadastro."
	if !temAlvara {
		aviso += fmt.Sprintf(
			" alvará não enviado no cadastro. envie depois em POST /documentos/academias/%s/alvara/upload.",
			codigoAcademia,
		)
	}
```

**Substituir por:**

```go
	aviso := "guarde o código da academia: ele é o seu identificador de login. você definiu sua própria senha no cadastro."
	if !temAlvara {
		aviso += " alvará não enviado no cadastro. você poderá enviá-lo mais tarde, pelo painel, após a ativação da conta."
	}
```

Por que este texto novo e não outro: confirmei que existe, de fato, uma tela no painel
(`spuripainel/src/app/(painel)/configuracoes/AlvaraSettingsCard.tsx`) onde a academia consegue enviar o
alvará depois da ativação — então a frase é factualmente correta, só sem citar o verbo HTTP e a rota
interna. Não removi o campo `aviso` nem a informação em si (a academia continua sabendo que o alvará não
foi enviado e que pode enviar depois) — só o detalhe de implementação que não deveria estar aí.

### 3.2 Correção 2 (CRÍTICA) — vínculo de estudante à turma

**Arquivo:** `internal/handlers/estudante_handlers.go`

**Contexto:** este trecho fica dentro do fluxo de cadastro de estudante pela academia
(`POST /academia/estudante/register`, autenticada, role `academia` — **não-admin**). Quando o estudante
é criado com sucesso mas o vínculo automático à turma falha, o campo `turma_aviso` da resposta 201 hoje
expõe **dois** problemas ao mesmo tempo: o verbo HTTP + rota (`POST /academia/turma/<codigo>/estudante`)
e o texto bruto do erro Go (`%v`, que pode conter detalhe interno dependendo da causa da falha).
Confirmei que o frontend exibe esse campo diretamente na tela para a academia, em
`spuripainel/src/app/(painel)/estudantes/cadastrar/CadastroSingularForm.tsx`.

**Localizar (texto exato):**

```go
		if err := vincularEstudanteATurma(c, academia, codigoEstudante, req.AnoEscolar, req.AnoEscolarMedio, req.AnoSuperior, req.CursoMedioID, req.CursoSuperiorID, codigoTurma, true, academiaID); err != nil {
			aviso := fmt.Sprintf("não foi possível vincular à turma '%s': %v. Use POST /academia/turma/%s/estudante para tentar novamente.", codigoTurma, err, codigoTurma)
			log.Printf("[WARN] falha ao vincular estudante recém-criado à turma: codigo_estudante=%s codigo_turma=%s erro=%v", codigoEstudante, codigoTurma, err)
			data["turma_vinculada"] = false
			data["turma_aviso"] = aviso
		} else {
```

**Substituir por:**

```go
		if err := vincularEstudanteATurma(c, academia, codigoEstudante, req.AnoEscolar, req.AnoEscolarMedio, req.AnoSuperior, req.CursoMedioID, req.CursoSuperiorID, codigoTurma, true, academiaID); err != nil {
			log.Printf("[WARN] falha ao vincular estudante recém-criado à turma: codigo_estudante=%s codigo_turma=%s erro=%v", codigoEstudante, codigoTurma, err)
			data["turma_vinculada"] = false
			data["turma_aviso"] = fmt.Sprintf("o estudante foi cadastrado com sucesso, mas não foi possível vinculá-lo automaticamente à turma '%s'. Vincule manualmente na tela de Turmas.", codigoTurma)
		} else {
```

Note que:
- O `log.Printf` de depuração **não muda** — continua registrando `err` por completo no log do
  servidor (isso é interno, nunca chega ao cliente, e continua útil para você mesmo/Fredy investigarem
  depois).
- `err` continua sendo usado (no `log.Printf`), então não sobra variável não utilizada.
- `codigoTurma` não é informação técnica — é um código de turma que a própria academia escolheu/criou;
  manter esse dado na mensagem continua sendo útil e não é o problema aqui.
- "Vincule manualmente na tela de Turmas" é factualmente correto — confirmei que
  `spuripainel/src/components/paineis/TurmasPainel.tsx` tem uma seção "Estudantes sem turma" de onde dá
  para vincular manualmente.

### 3.3 Correção 3 (preventiva, baixa prioridade) — mensagem de job em lote

**Arquivo:** `internal/handlers/async_batch_handlers.go`

**Contexto:** `publishAndReturnEnqueuedJob` é a função compartilhada que responde toda vez que um job
assíncrono é criado — tanto em rotas de admin quanto em rotas de academia (por exemplo,
`POST /academia/faltas-aluno/async` e `POST /academia/turma/estudante/async`, ambas role `academia`,
não-admin). Hoje nenhuma tela do frontend lê o campo `message` desta resposta (o frontend usa só
`job_id` para fazer o polling — confirmei em `spuripainel/src/lib/api/job-service.ts` e nos formulários
que chamam essas rotas), mas o texto ainda é enviado no corpo da resposta 202 para qualquer academia
autenticada, e instrui a chamar rotas HTTP diretamente. Corrijo por precaução/consistência, não porque
haja um vazamento visível hoje.

**Localizar (texto exato):**

```go
	c.JSON(http.StatusAccepted, gin.H{
		"message":     "job criado com sucesso — use GET /jobs/:id ou GET /jobs/stream para acompanhar o progresso",
		"job_id":      j.ID,
```

**Substituir por:**

```go
	c.JSON(http.StatusAccepted, gin.H{
		"message":     "job criado com sucesso. acompanhe o progresso pelo painel.",
		"job_id":      j.ID,
```

Os campos `poll_url` e `sse_url`, logo abaixo dessas linhas, **não mudam** — são metadados de URL
usados programaticamente (não são "mensagem" em linguagem natural) e não fazem parte do que Fredy pediu
para remover.

### 3.4 Se o texto de "localizar" não bater exatamente

Se, ao abrir um dos três arquivos, o trecho não bater byte a byte com o que está escrito acima (por
exemplo, uma linha a mais/a menos de espaço, ou o código ao redor mudou desde a auditoria), **não
invente uma adaptação**: procure a mesma função pelo nome (`RegisterAcademiaPublica`,
`RegisterEstudantePorAcademia` — a função que contém o bloco `if codigoTurma != ""` — e
`publishAndReturnEnqueuedJob`) e aplique a mesma mudança de *intenção* (remover o verbo HTTP + rota da
string, e no caso 2 também remover o `%v` do erro Go), preservando a indentação com tabs que já existe
no arquivo. Se não conseguir ter certeza, pare e não aplique — é melhor deixar para o Fredy revisar do
que arriscar quebrar a função.

## 4. Comandos de verificação que você (Codex) deve rodar

```bash
go build ./...
go vet ./...
go test ./...
```

Nenhum destes deveria falhar. Nenhuma das três correções altera assinatura de função, tipo, import ou
qualquer coisa que testes existentes dependam — são só literais de string dentro de handlers já
testados. Se `go test ./...` mostrar alguma falha, confirme primeiro se ela já existia **antes** das
suas mudanças (rode `git stash` e repita o comando para comparar) — corrigir falhas pré-existentes não
relacionadas a esta tarefa não é seu trabalho aqui.

Depois do build, rode estas três checagens de texto para confirmar que a remoção foi completa:

```bash
grep -n "POST /documentos/academias" internal/handlers/academia_handlers.go
grep -n "Use POST /academia/turma" internal/handlers/estudante_handlers.go
grep -n "use GET /jobs" internal/handlers/async_batch_handlers.go
```

As três devem retornar **vazio**.

## 5. O que eu (Claude) já validei, e o que fica para você

Não tenho Go instalado no meu sandbox (é uma limitação do meu ambiente, não do seu — é o oposto da sua
limitação de Postgres/Docker), então não consegui rodar `go build`/`go vet`/`go test` eu mesmo. O que eu
fiz, para reduzir ao máximo o risco de erro antes de chegar a você:

- Apliquei as três mudanças exatas acima numa cópia local do repositório e conferi visualmente, linha a
  linha, que chaves/parênteses continuam balanceados, que a indentação com tabs bate com o resto do
  arquivo, e que nenhuma variável (`err`, `codigoTurma`, `fmt`) fica sem uso depois da edição.
- Confirmei, com `git show HEAD:<arquivo>`, que o texto de "localizar" de cada uma das três correções
  bate exatamente (byte a byte) com o que está hoje na branch `main` dos dois repositórios.
- Auditei manualmente todos os outros lugares do backend com um padrão parecido (string com verbo HTTP
  + rota, ou campo `"aviso"`) e classifiquei cada um contra o middleware da rota correspondente — o
  resultado dessa auditoria está na seção 6.
- Confirmei, no repositório `spuripainel`, que os campos `turma_aviso` e `poll_url`/`sse_url`/`message`
  do job são lidos exatamente do jeito que descrevo nas seções 3.2 e 3.3 (nenhuma suposição — li o
  código real dos componentes citados).

Ou seja: **a seção 4 (`go build`/`go vet`/`go test`) é a única validação de compilação real que falta —
é inteiramente sua.** Eu fiz tudo o que dava para fazer sem o toolchain do Go.

## 6. Fora de escopo — não altere nada disto

Auditei o backend inteiro atrás do mesmo padrão (verbo HTTP + rota, ou campo `"aviso"`, em qualquer
resposta) e encontrei outros lugares que **já estão seguros hoje**, por motivos diferentes. Não toque em
nenhum deles nesta tarefa:

- **`internal/handlers/academia_handlers.go`, linha ~228** (dentro de `RegisterAcademia`, a rota
  `POST /dominis/academia/cadastro`): mesmo padrão de texto técnico, mas a rota exige
  `middleware.RequireFPP()` — só admin FPP autenticado chega aqui. Permitido pela regra do Fredy
  (administrador pode receber mensagem técnica).
- **`internal/handlers/nivel_escolar_handlers.go`, `internal/handlers/materia_disciplinar_handlers.go`,
  `internal/handlers/solicitacao_edicao_dado_estudante_handlers.go`,
  `internal/handlers/bilhete_identidade_sem_academia_handlers.go`, e o trecho de
  `internal/handlers/notas_handlers.go` que chama `resolverPeriodosValidos`/`resolverAnoLetivoAcademia`**:
  todos usam `utils.RespondWithValidationError(...)`, que gera o envelope padrão de erro
  (`{error, message, request_id, details}`, com `request_id`). Esse envelope já é interceptado e
  sanitizado no frontend (`sanitizeApiErrorEnvelopeForCurrentUser`, já mesclado hoje) para qualquer
  usuário que não seja admin — já está seguro.
- **`internal/handlers/contact_handlers.go`** (linhas ~25 e ~51-57): responde com `c.JSON` direto,
  faltando o campo `request_id` no corpo. Isso faz o envelope **não bater** com o formato esperado pelo
  frontend, então hoje a mensagem cai para um texto genérico (`statusText`) mesmo para administradores —
  não é um vazamento (é, na verdade, uma inconsistência não relacionada, que perde informação útil para
  admin). Fora do escopo desta tarefa; não é o mesmo tipo de problema.
- **`internal/handlers/bootstrap_handler.go`** (`POST /bootstrap`): confirmei, com busca em todo o
  `spuripainel`, que **nenhuma tela do frontend chama essa rota** — é usada manualmente (curl/script) só
  na primeira configuração do sistema, para criar o primeiro admin FPP. Não há superfície de usuário
  final aqui.
- **`internal/handlers/sistema_handler.go`** (rebuild de projeção): rota `admin.POST(... RequireFPP())`.
  Admin FPP only.
- **`internal/handlers/admin_handlers.go`, linha ~389** (`"aviso": "email_nao_enviado"`): não é texto
  técnico (não cita rota/verbo HTTP/infra) e a rota é `admin.POST("/register", RequireFPP())`. Nenhum
  dos dois motivos de exclusão se aplica, mas incluí aqui para deixar registrado que já vi essa linha e
  decidi que não precisa de correção.
- **`internal/handlers/academia_handlers.go`, linha ~1148** (`c.JSON(http.StatusNotFound, gin.H{"error": ...})`):
  não usa `RespondWith*`, então o corpo não tem `message`/`request_id` — mesma situação do
  `contact_handlers.go` acima (cai para texto genérico, não vaza, mas é inconsistente). Fora do escopo.

Não abra uma segunda frente corrigindo essas inconsistências de envelope — são um problema diferente
(perda de mensagem útil para admin), não vazamento, e não foi isso que Fredy pediu.

## 7. Checklist de aceite

- [ ] As três correções da seção 3 foram aplicadas exatamente como especificado.
- [ ] Nenhum outro arquivo, além dos três citados, foi tocado.
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `go test ./...` sem falhas novas (comparado ao estado antes desta tarefa).
- [ ] As três checagens de `grep` da seção 4 retornam vazio.
- [ ] Nenhum dos itens da seção 6 ("Fora de escopo") foi alterado.

## 8. Procedimento de conclusão

1. Aplique as seções 3.1, 3.2 e 3.3, na ordem.
2. Rode os comandos da seção 4 e resolva qualquer erro de compilação antes de seguir — não deveria
   haver nenhum, já que este código foi revisado linha a linha antes de chegar a este documento.
3. Confira o checklist da seção 7.
4. Não faça commit/push nem abra pull request — deixe as alterações prontas no working tree para o
   Fredy revisar.
5. Quando Fredy confirmar que está tudo certo, este documento deve ser movido de
   `docs/Lista de Tarefas/` para `docs/Tarefas feitas/`, renomeado para
   `96 - Remover vazamento de mensagens técnicas em respostas de sucesso para usuários não-administradores.md`
   (seguindo a numeração sequencial já usada pelos arquivos 90-95 dessa pasta), com o front matter
   atualizado para `status: feita` — mas isso só depois da confirmação, não como parte da execução
   automática.
