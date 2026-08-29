---
criado: 2026-08-28
atualizado: 2026-08-29
status: pronto_para_execucao
tipo: feature_nova_delecao_auditavel_event_sourcing
patch: tarefa74_delecao_auditavel.patch
---

# Tarefa 74 — Mecanismo de deleção auditável (event sourcing) para Academia, Administrador e Estudante

> Esta é a v2 deste documento. A v1 cobria só as 3 regras de negócio originais; Fredy pediu 3 adições depois de revisar: (1) as listagens gerais não devem mais retornar entidades deletadas, com um endpoint dedicado de auditoria para quem precisa vê-las; (2) e-mail/BI/telefone de admin e estudante devem ficar reutilizáveis depois que a conta é deletada; (3) uma tarefa separada para o frontend (`spuripainel`) refletir tudo isso. O patch já contempla as 3 adições — se você já aplicou o patch da v1, descarte-o e use o novo.

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Você não tem `apt`, Docker nem `psql` neste ambiente. **Isso é esperado e você não precisa deles para esta tarefa.**

Claude (orquestrador) já fez todo o trabalho que dependia de PostgreSQL real: aplicou as 111 migrations existentes numa instância PostgreSQL 16 real (mesma ordem lexicográfica que `internal/db/migrations.go` usa), escreveu as três migrations novas (112, 113, 114), aplicou-as contra esse banco, e rodou baterias de teste reais (dados inseridos de verdade, queries reais, updates reais) cobrindo os três tipos de usuário, o reuso de dados únicos e a query de auditoria. Todos os testes passaram. Detalhes na seção 8.

**Claude não conseguiu compilar o projeto Go** no próprio sandbox desta vez — não por limitação igual à sua, mas porque a rede do sandbox de Claude bloqueia `golang.org/x/*`, `gopkg.in/*` e `google.golang.org/*` (dependências transitivas de `gin`/`bcrypt`, usadas em `internal/handlers` e `internal/utils`). Claude validou a sintaxe de **todos** os arquivos alterados com `gofmt -l`/`gofmt -w` (formatação correta — mas isso não verifica tipos nem resolve imports). **`go build ./...`, `go vet ./...` e `go test ./...` ainda não foram executados por ninguém para este patch.** É o primeiro passo obrigatório do seu checklist (seção 9).

Se `go build` apontar algum erro, é quase certamente algo mecânico pequeno — todo o código novo espelha padrões já existentes e testados no repositório. Corrija o que for trivial e reporte o que não for.

---

## 1. Prompt recomendado para executar esta tarefa

> Execute esta tarefa. Todas as decisões de design já foram tomadas. Um patch pronto está em `tarefa73_delecao_auditavel.patch` — aplique com `git apply tarefa73_delecao_auditavel.patch` na raiz do repositório `spuri-backend`. Depois, rode o checklist da seção 9 deste documento, na ordem, e reporte o resultado de cada item. Se `go build`/`go vet`/`go test` encontrarem algo, corrija se for um erro mecânico óbvio; se não for óbvio, pare e reporte o erro exato antes de tentar mais nada. Não toque em nenhum arquivo fora dos 16 listados na seção 3. Depois do checklist verde, siga o procedimento de conclusão da seção 12.

---

## 2. Contexto e regras de negócio

Fredy pediu um mecanismo de deleção auditável, respeitando os conceitos de event sourcing já usados no projeto, para três tipos de usuário, mais três exigências transversais adicionadas numa segunda rodada:

**Regras originais:**
1. **Academia** — só pode ser deletada quando **nenhum estudante estiver vinculado a ela**, e só por um admin **FPP**. Histórico (notas, faltas, dados de estudantes) é mantido.
2. **Administrador** — hierarquia **FPP → ADM → Gerente** (Gerente não deleta ninguém); mesmo cargo nunca deleta outro do mesmo cargo.
3. **Estudante** — autodeleção, só se **não vinculado a nenhuma academia** no momento. Histórico mantido.

**Adições desta v2:**
4. **Consultas não retornam o que foi deletado.** Listagens de estudantes, academias e administradores excluem `status = 'deletado'` por padrão.
5. **Endpoint dedicado de auditoria de deleções.** Quem precisa ver o que foi deletado (quando, por quem, por quê) usa um endpoint separado — não as listagens gerais.
6. **Dados exclusivos ficam reutilizáveis após deleção.** E-mail, bilhete de identidade e telefone de um admin/estudante deletado deixam de "reservar" o valor — outro cadastro pode reutilizá-lo, porque não há mais ninguém vinculado a eles.

**Achado central (repetido da v1, ainda vale):** o mecanismo de soft delete + evento no `spuri_ledger` (append-only, hash-chained) já existia para Academia (migrations 110/111) antes desta tarefa. Esta tarefa completa a regra de negócio que faltava em Academia e replica o padrão, já comprovado, para Administrador e Estudante — e agora também estende esse MESMO padrão (já usado pelo NIF/e-mail de Academia) para os campos únicos de Admin/Estudante.

---

## 3. Resumo executivo — o que muda

| # | Arquivo | Tipo | O que muda |
|---|---|---|---|
| 1 | `migrations/112_admin_delecao_event_sourcing.sql` | novo | `status` de admin aceita `'deletado'`; `deleted_at`/`deletado_por` |
| 2 | `migrations/113_estudante_delecao_event_sourcing.sql` | novo | idem para estudante |
| 3 | `migrations/114_liberar_dados_unicos_apos_delecao.sql` | **novo (v2)** | converte UNIQUE incondicional de e-mail (admin) e telefone/BI (admin+estudante) em índices parciais que excluem `deletado` |
| 4 | `internal/domain/aggregates/admin.go` | modificado | campos de auditoria; evento `AdminDeletado`; `Deletar()`; guarda contra reativar admin deletado |
| 5 | `internal/domain/aggregates/estudante.go` | modificado | idem para estudante; evento `EstudanteDeletado`; `Deletar()` |
| 6 | `internal/domain/aggregates/academia.go` | modificado | correção de bug: `AtivarComAutor()`/`Desativar()` não impediam reativar academia deletada |
| 7 | `internal/projections/admin_projection.go` | modificado | `handleAdminDeletado`; **`GetAll`/`GetByEmail` agora excluem `deletado`** |
| 8 | `internal/projections/estudante_projection.go` | modificado | `handleEstudanteDeletado`; `CountVinculadosAtivos`; **`GetAll`/`GetByEmail`/`GetByAcademia`/`GetByBilheteIdentidadePrincipal(+ExcludingID)` agora excluem `deletado`** |
| 9 | `internal/db/safe_queries.go` | modificado | whitelist: `AdminDeletado`, `EstudanteDeletado` |
| 10 | `internal/db/event_store.go` | **modificado (v2)** | novo `GetEventsByTypes` (múltiplos tipos + paginação, usado pela auditoria) |
| 11 | `internal/db/repository.go` | **modificado (v2)** | wrapper `GetEventsByTypes` |
| 12 | `internal/handlers/academia_handlers.go` | modificado | `DeletarAcademia` checa estudantes vinculados; **`ListarTodasAcademias` exclui `deletado` no caso default** |
| 13 | `internal/handlers/admin_handlers.go` | modificado | novo handler `DeletarAdmin` |
| 14 | `internal/handlers/estudante_handlers.go` | modificado | novo handler `DeletarContaEstudante`; **`ListarEstudantes` exclui `deletado` incondicionalmente** |
| 15 | `internal/handlers/auditoria_delecoes_handler.go` | **novo (v2)** | handler `ListarAuditoriaDelecoes` — o endpoint dedicado do item 5 |
| 16 | `cmd/server/main.go` | modificado | 3 rotas novas |

**16 arquivos, patch de ~1080 linhas.** Nenhum arquivo fora desta lista deve ser tocado.

Novos endpoints (⚠️ o grupo admin é montado em **`/dominis`**, não `/admin` — `router.Group("/dominis")` em `main.go`; a v1 deste documento tinha esse prefixo errado):

| Método | Rota | Quem pode chamar | O que faz |
|---|---|---|---|
| `DELETE` | `/dominis/academia/:codigo` *(já existia, corrigido)* | FPP | bloqueia se houver estudante vinculado |
| `DELETE` | `/dominis/admin/:id` *(novo)* | ADM ou FPP | hierarquia via `ValidatePermission` |
| `DELETE` | `/estudante/conta` *(novo)* | o próprio estudante | bloqueia se `status != 'inativo'` |
| `GET` | `/dominis/auditoria/delecoes?tipo=&limit=&offset=` *(novo)* | ADM ou FPP | lista deleções dos 3 tipos, mais recentes primeiro |

---

## 4. Achados da investigação (leia antes de aplicar)

### 4.1 A "pegadinha" do `codigo_academia` do estudante

`projection_estudantes.codigo_academia` nunca é limpo quando o estudante se desvincula — só `status` muda para `'inativo'`. "Vinculado" é sempre `status IN ('ativo','pendente_documentos')`, nunca `codigo_academia IS NULL`. Testado ao vivo (seção 8).

### 4.2 A hierarquia de Admin já existe e já está em produção

`Admin.ValidatePermission(targetRole)` já implementa a regra pedida (nega quando `myLevel <= targetLevel`, com `fpp=3, adm=2, gerente=1`), já usada por `AtivarAdmin`/`DesativarAdmin`. `DeletarAdmin` reaproveita sem alterar — zero risco de regressão. Tabela-verdade conferida matematicamente contra os 9 casos — bate 100% com a regra pedida.

### 4.3 Bugs pré-existentes corrigidos nesta tarefa

1. `DeletarAcademia` não verificava estudantes vinculados. Corrigido com `EstudanteProjection.CountVinculadosAtivos`.
2. `Academia.AtivarComAutor()`/`Desativar()` "ressuscitavam" academias deletadas (só checavam `== "ativo"`/`"inativo"`, nunca `== "deletado"`). Guarda explícita adicionada; a mesma guarda foi aplicada desde o início em `Admin.Ativar()`/`Desativar()` para não repetir o bug.

### 4.4 O whitelist de eventos (`safe_queries.go`) é fácil de esquecer

`AppendTx` rejeita com 500 qualquer `event_type` fora do whitelist. `AdminDeletado`/`EstudanteDeletado` já adicionados no patch.

### 4.5 (v2) Onde exatamente as listagens vazavam entidades deletadas

- `ListarTodasAcademias`: o caso **default** (sem `?status=` explícito — o uso mais comum) não tinha filtro nenhum. Os casos explícitos (`?status=ativo`, etc.) já excluíam `deletado` implicitamente, por serem igualdade exata.
- `ListarEstudantes`: o filtro `?status=` já rejeitava `"deletado"` como valor inválido — mas **sem nenhum filtro** (uso comum), a query não tinha WHERE nenhum, retornando tudo. Corrigido com uma exclusão incondicional (`e.status <> 'deletado'`) adicionada uma única vez, que protege tanto o branch de academia quanto o de admin visualizando "todos os estudantes".
- `AdminProjection.GetAll`: idem, sem filtro algum. Usado por `ListarTodosAdmins` **e** pela checagem de bootstrap (`bootstrap_handler.go`) — conferido que excluir `deletado` não quebra o bootstrap; ao contrário, corrige um caso de borda (se o único FPP existente for deletado, o sistema volta a permitir bootstrap, em vez de ficar permanentemente travado).
- `EstudanteProjection.GetByAcademia`: usado por `documento_download_handlers.go` para listar estudantes de uma academia — tinha o mesmo problema, corrigido pelo mesmo motivo.
- Pré-checagens de "e-mail/BI já em uso" (`AdminProjection.GetByEmail`, `EstudanteProjection.GetByEmail`, `GetByBilheteIdentidadePrincipal(ExcludingID)`) também foram corrigidas — sem isso, mesmo com a migration 114 liberando o índice do banco, a APLICAÇÃO continuaria recusando o cadastro com "e-mail já em uso" antes mesmo de chegar ao banco. `AcademiaProjection.GetByNIF` já fazia isso corretamente desde a migration 111 — foi usado como referência/prova do padrão certo.

### 4.6 (v2) O que foi deliberadamente NÃO liberado, e por quê

- `projection_academias.codigo_academia` — é alvo de FK de `projection_cursos`/`projection_materias`. Não pode ser liberado (já era assim antes desta tarefa).
- `projection_estudantes.codigo_estudante` — **decisão desta tarefa**: não é FK declarada, mas `projection_notas`/`projection_faltas` guardam `codigo_estudante` como chave natural sem FK. Se fosse liberado, um novo estudante poderia herdar o mesmo código de um estudante deletado, e consultas de notas/faltas por código passariam a misturar o histórico acadêmico de duas pessoas diferentes — um risco de integridade/legal, não um simples "campo exclusivo". Testado ao vivo que continua bloqueado mesmo após deleção (seção 8).
- `telefone_encarregado` do estudante (telefone do encarregado de educação) **foi** liberado, por inferência: uma vez que o estudante é deletado, o vínculo que "reservava" o telefone do encarregado por causa dele deixa de existir — por exemplo, um encarregado que teve um filho com conta deletada deve poder cadastrar outro filho usando o mesmo contato. Fredy não pediu isso explicitamente (falou em dados "dele", do estudante); sinalizando aqui para veto se não for a leitura correta.

---

## 5. Como aplicar

Na raiz do repositório (`spuri-backend`):

```bash
git apply tarefa73_delecao_auditavel.patch
```

Testado por Claude: `git apply --check` limpo contra um clone independente e recente de `main`, `git apply` aplicado com sucesso, e os 16 arquivos conferidos (inclusive os 4 novos, byte-a-byte). Se não aplicar de primeira num checkout muito divergente, use `git apply --reject` e reporte os `.rej` antes de resolver manualmente.

---

## 6. Detalhamento por arquivo

*(Itens 6.1–6.12 abaixo cobrem as regras originais; 6.13–6.16 são as adições da v2.)*

### 6.1–6.2 Migrations 112/113

Espelham a migration 110 (academia): `status` aceita `'deletado'`, colunas `deleted_at`/`deletado_por`, índice parcial. Nomes de constraint conferidos contra o catálogo real do Postgres.

### 6.3 `internal/domain/aggregates/admin.go`

Campos `DeletedAt`/`DeletadoPor`; case `"AdminDeletado"` no `Apply()`; guarda `status == "deletado"` em `Ativar()`/`Desativar()`; método `Deletar(motivo, deletadoPor)` (não valida hierarquia — isso é do chamador, via `ValidatePermission`); `applyAdminDeletado`; struct `AdminDeletadoEvent`.

### 6.4 `internal/domain/aggregates/estudante.go`

Idem, mais a checagem `e.Status == "inativo"` dentro de `Deletar()` (a regra de "não vinculado").

### 6.5 `internal/domain/aggregates/academia.go`

Guarda `status == "deletado"` em `AtivarComAutor()`/`Desativar()` — correção de bug (4.3).

### 6.6 `internal/projections/admin_projection.go`

`handleAdminDeletado` (idempotente via `WHERE ... AND status <> 'deletado'`). **v2:** `GetAll` ganhou `WHERE status <> 'deletado'`; `GetByEmail` ganhou `AND status <> 'deletado'` (isso também afeta login — um admin deletado não consegue mais autenticar por e-mail, o que é o comportamento correto e reforça, em outra camada, o que `RequireAdminRole` já impunha).

### 6.7 `internal/projections/estudante_projection.go`

`handleEstudanteDeletado`; `CountVinculadosAtivos(codigoAcademia)` — a query central da regra de Academia. **v2:** `GetAll`, `GetByEmail`, `GetByAcademia`, `GetByBilheteIdentidadePrincipal` e `...ExcludingID` todos ganharam `AND/WHERE status <> 'deletado'`.

### 6.8 `internal/db/safe_queries.go`

`AdminDeletado`/`EstudanteDeletado` no whitelist.

### 6.9 (v2) `internal/db/event_store.go` e `internal/db/repository.go`

Novo `EventStore.GetEventsByTypes(ctx, eventTypes []string, limit, offset int) ([]Event, error)` — como o já existente `GetEventsByType` (singular), mas aceita vários tipos numa query (`event_type = ANY($1)`) e suporta `OFFSET`. Wrapper equivalente em `AggregateRepository`. Usado só pelo endpoint de auditoria.

### 6.10 `internal/handlers/academia_handlers.go`

`DeletarAcademia` chama `CountVinculadosAtivos` antes de deletar; `409 Conflict` se houver vinculados. **v2:** `ListarTodasAcademias`, caso default, ganhou `WHERE pa.status <> 'deletado'` (na contagem e na query).

### 6.11 `internal/handlers/admin_handlers.go`

`DeletarAdmin`, espelhando `DesativarAdmin`: carrega admin-alvo e executor, `executor.ValidatePermission(target.Role)`, bloqueia autodeleção, `target.Deletar(...)`, `SaveWithAudit`, `registrarAcaoAdmin`.

### 6.12 `internal/handlers/estudante_handlers.go`

`DeletarContaEstudante`: pega o próprio `userID` via `middleware.GetUserID`, exige `motivo`, chama `estudante.Deletar(motivo, userID)`. **v2:** `ListarEstudantes` ganhou `conditions = append(conditions, "e.status <> 'deletado'")` uma única vez, antes da montagem do WHERE — protege os dois branches (academia vendo seus estudantes; admin vendo todos) com uma linha só.

### 6.13 (v2) `internal/handlers/auditoria_delecoes_handler.go` — arquivo novo

`ListarAuditoriaDelecoes`: aceita `?tipo=academia|administrador|estudante` (opcional) e paginação. Chama `GetEventsByTypes` para os 3 tipos de evento de deleção, e enriquece cada um com dados legíveis (nome, identificador) consultando a projeção correspondente **pelo ID** (que continua resolvendo entidades deletadas — só as funções de listagem/busca por valor foram filtradas, `GetByID` continua igual em Academia/Admin/Estudante). Para Academia e Admin, também resolve o nome/e-mail de quem executou a deleção; para Estudante, `deletado_por` é sempre o próprio (autodeleção), então não há um segundo nome a resolver.

### 6.14 (v2) `migrations/114_liberar_dados_unicos_apos_delecao.sql` — arquivo novo

Ver seção 4.5/4.6. Converte 7 constraints/índices UNIQUE em índices parciais que excluem `status = 'deletado'`:
- `projection_admins.email` (era `UNIQUE` de tabela — virou índice parcial `uq_admin_email_ativo`)
- `idx_bootstrap_fpp_unique` (bootstrap FPP)
- `idx_telefone_verificado_admin`
- `uq_estudante_bilhete_identidade_normalizado`
- `idx_telefone_verificado_estudante`
- `idx_telefone_resp_verificado_estudante`
- `idx_estudante_telefone_encarregado_unico`

`codigo_estudante` **não** foi tocado (ver 4.6).

### 6.15–6.16 `cmd/server/main.go`

3 rotas: `DELETE /dominis/admin/:id`, `DELETE /estudante/conta`, `GET /dominis/auditoria/delecoes`.

---

## 7. O que NÃO está neste patch (e por quê)

- **UI do `spuripainel`.** Coberto por um documento separado, entregue junto com este (ver Tarefa 1 do repositório `spuripainel`). Backend e frontend são repositórios/deploys distintos — não faz sentido misturar num único patch.
- **Liberar `codigo_estudante`.** Ver 4.6 — risco de integridade de dados, decisão deliberada.

---

## 8. Testes que Claude já rodou (PostgreSQL 16 real)

Banco de teste criado do zero, 111 migrations existentes + 112/113/114 aplicadas na mesma ordem que `internal/db/migrations.go` usa. Todos os testes abaixo rodaram dentro de transações com `ROLLBACK` (não deixaram lixo no banco).

**Regras de negócio (12 cenários, todos ✅ — repetido da v1):** contagem de estudantes vinculados (conta `ativo`/`pendente_documentos`, ignora `inativo` mesmo com `codigo_academia` preenchido), academia sem estudantes, `UPDATE` idempotente de deleção de admin e estudante, `CHECK` aceitando `'deletado'`, autodeleção de estudante, `INSERT` no ledger com hash chain, `UPDATE`/`DELETE` no ledger corretamente bloqueados pelos triggers de imutabilidade mesmo para os eventos novos.

**Reuso de dados únicos (v2, novos, todos ✅):**

| # | Cenário | Esperado | Resultado |
|---|---|---|---|
| 13 | Cadastrar 2º admin com e-mail já em uso por admin **ativo** | rejeitado (`unique_violation`) | ✅ rejeitado |
| 14 | Deletar o 1º admin, depois cadastrar novo admin com o **mesmo e-mail** | permitido | ✅ permitido |
| 15 | Mesmo teste com bilhete de identidade de estudante | permitido após deleção | ✅ permitido |
| 16 | Tentar reaproveitar `codigo_estudante` de um estudante deletado | **continua bloqueado** | ✅ bloqueado (comportamento pretendido) |

**Endpoint de auditoria (v2, novos, todos ✅):**

| # | Cenário | Esperado | Resultado |
|---|---|---|---|
| 17 | Query com os 3 tipos de evento de deleção juntos, ordenados por data | mais recente primeiro; eventos de outro tipo (ex: `AcademiaAtivada`) não aparecem | ✅ |
| 18 | Paginação (`limit=1 offset=1`) | traz exatamente o 2º item | ✅ |
| 19 | Filtro por um único tipo (`?tipo=academia`) | só `AcademiaDeletada` | ✅ |

---

## 9. Checklist de validação do Codex

1. `git apply tarefa73_delecao_auditavel.patch`.
2. `gofmt -l .` — vazio (já conferido por Claude para os 13 arquivos `.go`, confirme no seu checkout).
3. `go build ./...` — **primeira verificação real de compilação; Claude não conseguiu rodar isto (seção 0).**
4. `go vet ./...`.
5. `go test ./...` — testes de integração Postgres devem pular sem `RUN_POSTGRES_INTEGRATION`/`SPURI_RUN_DB_INTEGRITY_TESTS` (esperado).
6. Revisão rápida: nenhum `DELETE FROM` novo em `projection_academias`/`projection_admins`/`projection_estudantes` — toda deleção é `UPDATE ... SET status = 'deletado'`.
7. Revisão rápida: confirme que `internal/handlers/auditoria_delecoes_handler.go` não importa nada não utilizado (Claude removeu um import de `middleware` que sobrou de uma primeira tentativa — confirme que não voltou).

---

## 10. Critérios de aceite

- [ ] Os 16 arquivos da seção 3 aplicados, nenhum outro tocado.
- [ ] `go build ./...`, `go vet ./...`, `gofmt -l .` limpos; `go test ./...` verde.
- [ ] `DeletarAcademia` bloqueia com `409` quando há estudante vinculado.
- [ ] `DeletarAdmin` respeita a hierarquia (4.2).
- [ ] `DeletarContaEstudante` só funciona com `status == 'inativo'`.
- [ ] `ListarTodasAcademias`/`ListarEstudantes`/`ListarTodosAdmins` (sem filtro explícito) não retornam nada com `status = 'deletado'`.
- [ ] `GET /dominis/auditoria/delecoes` retorna os 3 tipos de deleção, mais recente primeiro, com paginação e filtro por `?tipo=` funcionando.
- [ ] Cadastrar um novo admin/estudante com e-mail/BI/telefone de uma conta **deletada** funciona; com uma conta **ativa**, continua rejeitando.
- [ ] Nenhuma linha `DELETE FROM` nova nas 3 tabelas de projeção.

---

## 11. Fora do escopo (não implementar sem Fredy pedir)

- Frontend — documento separado.
- Liberar `codigo_estudante` para reuso (4.6).

---

## 12. Procedimento de conclusão

1. Checklist da seção 9 100% verde → mover este arquivo para `docs/Tarefas feitas/`, `status: concluido` + `concluido: <data>` no frontmatter, mantendo o número 73.
2. Commit único, mensagem sugerida: `delecao: mecanismo auditavel (event sourcing) para academia/admin/estudante + filtra listagens + libera dados unicos + endpoint de auditoria`.
3. Reportar a Fredy: resultado de cada item do checklist, `git diff --stat`, e qualquer ajuste manual que `go build` tenha exigido (informação nova que Claude não tinha — seção 0).
