---
criado: 2026-09-12
origem: Claude (orquestrador) — pré-testado com PostgreSQL real e suíte de testes completa
status: implementado e testado — aplicar o patch e validar no seu ambiente
patch_anexo: senha-minima-e-documentos-extra.patch (mesma pasta)
---

# Senha mínima de 8 caracteres + documentos extra no cadastro/matrícula do estudante

## Leia isto primeiro

Esta tarefa **já foi implementada e testada de ponta a ponta** contra um
PostgreSQL real (migration aplicada, constraints testadas com INSERT reais,
event sourcing testado via ledger → rebuild da projeção → queries) e contra a
suíte de testes completa do projeto (`go test ./...` com
`SPURI_RUN_DB_INTEGRITY_TESTS=1`, banco limpo, **0 falhas**, nenhum teste
pré-existente quebrado).

O patch `senha-minima-e-documentos-extra.patch`, nesta mesma pasta, contém
TODAS as mudanças de código. **Seu trabalho não é reimplementar nem
replanejar nada** — é:

1. Aplicar o patch (`git apply "docs/Lista de Tarefas/senha-minima-e-documentos-extra.patch"`
   a partir da raiz do repositório).
2. Rodar `go build ./...` e `go vet ./...` no seu ambiente (aqui já rodou limpo).
3. Rodar `go test ./...` no seu ambiente. Se seu ambiente tiver acesso normal
   ao proxy de módulos Go (`proxy.golang.org`) — o que é o padrão, diferente
   do bloqueio de `apt`/Docker/psql — isso deve funcionar sem nenhum
   workaround. Os testes que dependem de PostgreSQL real
   (`SPURI_RUN_DB_INTEGRITY_TESTS=1`) não vão rodar no seu ambiente (sem
   `psql`/Docker) — isso é esperado e já foi validado aqui, não precisa
   tentar contornar.
4. Fazer os itens da seção "O que falta fazer" abaixo (documentação da API e
   as duas decisões marcadas como "fora de escopo").
5. Mover este arquivo (e o `.patch`) de `docs/Lista de Tarefas/` para
   `docs/Tarefas feitas/` ao concluir, conforme o padrão já usado neste
   repositório.

Se `git apply` reclamar de conflito, é sinal de que o `main` mudou desde que
este documento foi gerado — nesse caso, aplique as mudanças manualmente
lendo o `.patch` como referência exata (ele já está correto e testado), não
como sugestão a adaptar livremente.

---

## O que foi pedido

1. Senha com no mínimo 8 caracteres (era 6).
2. A academia pode exigir/oferecer outros documentos no cadastro/matrícula
   do estudante, além dos fixos do sistema (BI, cédula, declaração,
   certificados). O limite de tamanho é o mesmo já usado (10MB). A academia
   define, por documento: obrigatoriedade e tipo (PDF ou JPG).
3. Ideia de esquema dada na tarefa original: `documentos_extra: [{id: 1,
   rotulo, tipo, obrigatorio, nivel}]`.
4. Armazenamento em `.../documento_extra/{id do documento}/...`.

## Decisões tomadas (e por quê) — divergências do esquema sugerido

O pedido chamava isso de "uma ideia" de esquema, não uma especificação
fechada. Duas decisões importantes divergem do exemplo literal, deliberada e
documentadamente:

- **`id` é UUID, não inteiro sequencial.** Todo o resto do sistema
  (categorias de serviço, serviços extra, cursos, turmas, ...) usa UUID como
  identificador. Um contador sequencial exigiria coordenação adicional
  (lock/sequence) sem trazer nenhum benefício aqui, e quebraria o padrão que
  todo o resto do código já segue.
- **O campo é `ano_academico` (não `nivel`) para a exigência em si, com
  `nivel` derivado automaticamente.** No restante do sistema, "nivel"
  significa especificamente `fundamental | medio | superior`, e
  "ano_academico" é o valor específico (`6_ano_fundamental`,
  `2_ano_medio`, `1_ano_superior`) — são dois conceitos diferentes e já
  usados assim em toda parte (ver `aggregates.DocumentoMatricula`,
  `aggregates.NivelDoAnoAcademico`). Segui esse padrão: a academia informa
  `ano_academico` (formato `N_ano_fundamental|N_ano_medio|N_ano_superior`),
  e `nivel` é **derivado automaticamente** no backend via
  `NivelDoAnoAcademico()` — nunca aceito solto no payload.
  **Implicação prática**: uma exigência vale para UM ano específico. Se a
  academia quiser exigir o mesmo documento em vários anos de um nível (ex.:
  todo o fundamental), ela cria uma definição por ano. Isso não foi
  automatizado nesta tarefa — ver "Fora de escopo".

## O que foi implementado

### 1. Senha mínima de 8 caracteres

- `internal/utils/validation.go` — `ValidateSenha`: 6 → 8.
- `internal/utils/errors.go` — mensagem espelho (usada como fallback ao
  traduzir erros de banco/validação que mencionam "senha") também
  atualizada para "8 caracteres".
- **Gap encontrado e corrigido**: `ResetarSenha` (recuperação de senha por
  e-mail, `internal/handlers/auth_email_handlers.go`) **não chamava**
  `utils.ValidateSenha` antes desta correção — era o único dos três fluxos
  de definição de senha (troca própria, cadastro de academia, recuperação)
  sem essa validação. Corrigido chamando `ValidateSenha` **antes** de
  consumir o token de recuperação (que é de uso único), para que uma senha
  curta não queime o link por engano.
- Não mexi no bootstrap do admin FPP (`bootstrap_handler.go`) — é uma rota
  operacional de uso único pela equipe técnica, não um cadastro de usuário
  final, e hoje não valida senha de nenhum tamanho. Deixei como estava por
  não fazer parte do que foi pedido; se quiserem endurecer isso também, é
  tarefa separada.

### 2-4. Catálogo de documentos extra

Implementado como um **aggregate próprio** (`DocumentoExtra`), event-sourced,
seguindo exatamente o mesmo padrão já usado por `CategoriaServico`
(`internal/domain/aggregates/categoria_servico.go` — veja esse arquivo se
quiser comparar lado a lado). Não usei o padrão alternativo mais simples de
"campo JSONB solto na Academia" (usado por `anos_academicos`,
`documentos_obrigatorios`) porque cada documento extra precisa de identidade
própria (o `{id}` é usado no path de storage) — um aggregate dedicado com
sua própria tabela de projeção é o encaixe mais natural e correto aqui.

**Migration** `migrations/126_documentos_extra.sql`:
- Tabela `projection_documentos_extra` (id UUID PK, codigo_academia, rotulo,
  tipo, obrigatorio, nivel, ano_academico, ativo, criado_por, timestamps,
  version, last_event_id).
- `CHECK (tipo IN ('pdf','jpg'))`, `CHECK (nivel IN ('fundamental','medio','superior'))`.
- Índice único **case-insensitive** por `(codigo_academia, ano_academico,
  lower(rotulo))` filtrado a `ativo = true` — impede duas exigências
  ativas com o mesmo rótulo no mesmo ano acadêmico da mesma academia.
- Testada com INSERT reais neste ambiente: tipo inválido rejeitado, rótulo
  duplicado (mesmo com case diferente) rejeitado, inserção válida aceita.

**Aggregate** `internal/domain/aggregates/documento_extra.go`:
- `Criar / Atualizar / Desativar / Reativar` — desativar é sempre remoção
  lógica (preserva histórico), igual ao resto do sistema.
- `Atualizar` é **prospectivo**: mudar `tipo` de uma definição não invalida
  nem re-exige arquivos já enviados por estudantes com o tipo antigo — só
  vale para os próximos cadastros/matrículas. Isso é intencional, documentado
  no código, e consistente com como o resto do sistema trata edições de
  catálogo.
- Validação de `ano_academico` reaproveita `utils.ValidateAnoFundamental /
  ValidateAnoMedio / ValidateAnoSuperior` (as mesmas usadas pelos
  documentos fixos) — não inventei uma validação paralela.

**Achado importante do pré-teste** (documentado com comentário extenso no
código, em `validarRotuloAnoDocumentoExtraDisponivel`): a unicidade de
rótulo é garantida pelo **índice da projeção**, não pelo ledger de eventos.
Sem uma pré-checagem no handler ANTES de gerar o evento, um segundo evento
com rótulo duplicado é aceito no ledger e **trava o checkpoint da projeção
`documentos_extra` permanentemente para TODAS as academias** ao tentar
reconstruir/processar esse evento. Reproduzi esse travamento neste ambiente
antes de adicionar a pré-checagem, e confirmei que ela resolve o problema.
Isso é o mesmo cuidado que `CategoriaServico` já toma
(`validarNomeCategoriaServicoDisponivel`) — se for tocar em código
parecido no futuro, este é o padrão a seguir.

**Projeção** `internal/projections/documento_extra_projection.go` — CRUD +
`GetByAcademia(codigo, ativosOnly)` + `GetAtivosPorAnoAcademico(codigo,
anoAcademico)` (usada na validação de obrigatoriedade no cadastro/matrícula).

**Handlers** `internal/handlers/documento_extra_handlers.go` — rotas
registradas em `cmd/server/main.go`:

| Método | Rota | Descrição |
|---|---|---|
| GET | `/academia/documentos-extra` | Lista o catálogo da academia autenticada. `?ativos=true` filtra só ativos. |
| POST | `/academia/documentos-extra` | Cria uma definição. Body: `{rotulo, tipo, obrigatorio, ano_academico}`. |
| PUT | `/academia/documentos-extra/:id` | Edita uma definição (mesmo body). |
| PUT | `/academia/documentos-extra/:id/desativar` | Remoção lógica. |
| PUT | `/academia/documentos-extra/:id/reativar` | Reativa (sujeito à mesma checagem de rótulo único). |

Resposta de cada item: `{id, codigo_academia, rotulo, tipo, obrigatorio,
nivel, ano_academico, ativo, created_at, updated_at}`.

**Upload e validação** `internal/handlers/documento_extra_upload.go`:
- Convenção de campo multipart: **`documento_extra_<id do catálogo>`**
  (ex.: `documento_extra_3f1e2c4a-...`).
- `readAndValidateDocumentoExtra`: mesmo limite de 10MB
  (`MaxPDFUploadBytes`, reaproveitado — "o limite de tamanho mantém-se o
  mesmo"), valida Content-Type + extensão + **assinatura binária real**
  (`%PDF` para pdf; `FF D8 FF` para jpg) de acordo com o `tipo` configurado
  para aquele documento específico — mesma técnica já usada em
  `readAndValidatePDF` para os documentos fixos.
- `parseDocumentosExtra`: allowlist estrita — só aceita campos
  `documento_extra_*` cujo id exista no catálogo **ativo** desta academia
  **para o ano_academico do estudante sendo cadastrado/matriculado**.
  Qualquer outro é rejeitado (mesma filosofia de
  `validarCamposArquivoMatricula` para os documentos fixos).
- `validarObrigatoriedadeDocumentosExtra`: bloqueia o cadastro/matrícula se
  algum documento com `obrigatorio=true` do catálogo aplicável não foi
  enviado.
- `storagePathDocumentoExtra`: monta exatamente
  `{baseDir}/documento_extra/{id do catálogo}/{documento_id}.{pdf|jpg}` —
  como pedido no item 4.
- Metadados do arquivo enviado ficam em
  `projection_estudantes.documentos`, chave `"documento_extra.<id do
  catálogo>"` (reaproveita a estrutura `DocumentoMatricula` já existente,
  com o novo campo `DocumentoExtraID` para rastrear de qual definição do
  catálogo aquele arquivo veio).

**Integrado em três pontos** (os três fluxos que hoje lidam com documentos
de estudante):
- `POST /academia/estudante/register` (cadastro direto) — pulado por
  completo quando a academia registra em modo "pendente de documentos"
  (mesmo tratamento dos documentos fixos).
- `POST /solicitacao-matricula` (matrícula pública) — sempre obrigatório
  quando configurado, já que este fluxo não tem modo "pendente".
- `POST /academia/estudante/:codigo/documentos` (completar documentos de um
  estudante pendente) — reconhece documentos extra já enviados
  anteriormente ao validar o que ainda falta.

### Download dos documentos extra

Inicialmente eu tinha deixado isso fora de escopo — recebi feedback de que
é essencial (sem isso, o documento fica órfão: enviado e armazenado, mas
sem forma de ser consultado, o que ficaria fora do padrão do resto do
sistema). Corrigido. Acontece que **nenhuma rota nova precisou ser
criada**: as rotas de download genéricas que já existem para os documentos
fixos (`streamDocumentoEstudante`, `streamDocumentoSolicitacaoMatricula`,
usadas por `DownloadDocumentoEstudanteAcademia`,
`DownloadMeuDocumentoEstudante`, `DownloadDocumentoSolicitacaoMatriculaAcademia`,
etc.) resolvem o documento por **lookup direto na chave do mapa
`Documentos`** via `documentoEstudantePorCampoEscopo` — e chaves com ponto
(como a nossa, `"documento_extra.<id>"`) já eram um padrão existente ali
(usado para declarações escopadas por nível+ano). Ou seja, essas rotas já
funcionavam para documento_extra assim que o campo certo fosse preenchido.

Duas coisas realmente faltavam, e foram as que corrigi:

1. **`DownloadURL` não estava sendo preenchido.** `armazenarDocumentosExtra`
   agora recebe uma função `downloadURL(campo string) string` (mesmo padrão
   já usado por `documentosComDownload` para os outros tipos de documento) e
   preenche `DownloadURL` corretamente — reaproveitando
   `estudanteDocumentoDownloadURL` (cadastro direto e completar pendentes)
   ou `solicitacaoDocumentoDownloadURL` (matrícula pública), exatamente as
   mesmas funções já usadas pelos documentos fixos nesses mesmos três
   pontos de integração.
2. **`streamDocumento` assumia PDF sempre** (Content-Type
   `application/pdf` e extensão `.pdf` hardcoded). Como documento extra
   pode ser JPG, isso serviria um JPG com o Content-Type errado. Corrigido
   para escolher `image/jpeg`/`.jpg` quando `doc.Tipo == "jpg"`, e manter
   `application/pdf`/`.pdf` em qualquer outro caso — o que preserva 100%
   do comportamento atual para os documentos fixos (cujo `Tipo` é sempre um
   nome de campo/categoria, nunca literalmente `"jpg"`).

Com isso, os documentos extra aparecem automaticamente nas rotas de
listagem existentes (`GET /estudante/documentos`, `GET
/academia/documentos-inventario` e afins — as que já iteram sobre todo o
mapa `Documentos` de um estudante/solicitação) com uma `download_url`
correta, e essa URL funciona de verdade.

## Testes automatizados incluídos no patch

- `internal/domain/aggregates/documento_extra_test.go` — ciclo de vida
  completo do aggregate (validações de rotulo/tipo/ano_academico, derivação
  de nível, atualizar, desativar/reativar e suas rejeições).
- Suíte completa (`go test ./...`, banco limpo, `SPURI_RUN_DB_INTEGRITY_TESTS=1`)
  rodada e aprovada neste ambiente após todas as mudanças — nenhum teste
  pré-existente quebrou.

## Fora de escopo (não implementado nesta tarefa — decisão consciente)

- **"Aplicar a um nível inteiro" (todos os anos de uma vez).** Cada
  definição vale para um `ano_academico` específico, conforme decisão
  documentada acima. Se quiserem essa automação, é uma tarefa separada.
- **Revalidação de obrigatoriedade no momento da aprovação de uma
  solicitação de matrícula.** A obrigatoriedade é checada quando a
  solicitação é CRIADA (`POST /solicitacao-matricula`). No momento da
  aprovação (`AprovarSolicitacaoMatricula`), o sistema já revalida os
  documentos FIXOS (`validateDocumentosMatricula`) mas não os extra — não
  estendi essa revalidação porque o catálogo e os arquivos não mudam entre
  a criação e a aprovação da solicitação nesse fluxo. Se um dia existir uma
  forma de editar uma solicitação pendente antes da aprovação, vale
  reconsiderar isso.
- **Documentação da API** (`Documentação da API.md`) — arquivo grande
  demais para eu reescrever seção por seção com segurança neste patch. Ver
  próxima seção.

## O que falta fazer (seu trabalho nesta tarefa)

1. Aplicar o patch e confirmar build/vet/test no seu ambiente (seção "Leia
   isto primeiro").
2. Adicionar ao `Documentação da API.md` (backend) uma seção nova para
   `/academia/documentos-extra` (as 5 rotas da tabela acima, com exemplos de
   request/response) e documentar o novo campo multipart
   `documento_extra_<id>` nas seções já existentes de
   `/academia/estudante/register` e `/solicitacao-matricula`. Não
   reescreva o resto do arquivo — só adicione.
3. Conferir se existe alguma rotina de seed/dados de exemplo
   (`scripts/seed` ou similar) que lista campos de documento — se existir,
   pode fazer sentido adicionar um exemplo de documento extra lá, mas só se
   já for um padrão existente no repositório (não crie um sistema de seed
   novo só por causa disso).
4. Depois de validar tudo, mover este `.md` e o `.patch` para
   `docs/Tarefas feitas/`.

## Critérios de aceite

- [ ] `git apply` do patch aplica sem conflito (ou as mudanças foram
      replicadas manualmente a partir dele).
- [ ] `go build ./...` e `go vet ./...` limpos.
- [ ] `go test ./...` sem falhas novas (os testes de integração com
      Postgres real não rodam no seu ambiente — esperado).
- [ ] Seção nova sobre documentos extra adicionada em `Documentação da
      API.md` (mencionando também que eles aparecem nas rotas de download
      e listagem já existentes, com a mesma URL pattern dos documentos
      fixos).
- [ ] Nenhuma mudança fora do que está descrito aqui e no patch.
