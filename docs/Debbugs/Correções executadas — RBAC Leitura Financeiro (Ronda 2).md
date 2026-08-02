---
modificado: 2026-08-01
criado: 2026-08-01
origem: Depuração — Verificação das correções "Auditoria Módulo Financeiro AppyPay"
---

# Correções executadas — RBAC de leitura do módulo Financeiro (Ronda 2)

## Resumo

A auditoria de verificação (`Depuração — Verificação das correções "Auditoria
Módulo Financeiro AppyPay"`) apontou dois pontos em aberto. Depois de reexaminar
o código atual linha a linha antes de mexer em qualquer coisa:

| # | Item do critério de saída | Situação real encontrada |
|---|---|---|
| 1 | `AtualizarCredencial` deve exigir `fpp` real, não `"admin"` genérico | **Já estava correto** no código fornecido — nenhuma mudança feita em `internal/finance/financeiro.go` |
| 2 | `RequireAdmin()` aplicado às 3 rotas de leitura | **Confirmado em aberto** — corrigido nesta ronda, mas com uma abordagem diferente da sugerida (ver abaixo) |
| 3 | Teste que passe pelo caminho HTTP real | Adicionado um teste de registo/autenticação de rotas; teste completo de RBAC por papel fica documentado como próximo passo (precisa de banco de teste) |

## Item 1 — nenhuma mudança necessária

Reconferi `CriarCredencial`, `AlterarStatusCredencial`, `AlterarModalidade` e
`AtualizarCredencial` em `internal/finance/financeiro.go`. Todas já exigem
estritamente `autorTipo == "fpp"` para o ramo administrativo (a única exceção
prevista é a própria academia dona da credencial, em `AtualizarCredencial`,
que é comportamento documentado e intencional — autoatendimento). Não toquei
neste arquivo.

## Item 2 — por que não foi só "adicionar RequireAdmin()"

`RequireAdmin()` bloqueia qualquer requisição cujo `user_type` não seja
`"admin"`. As três rotas de leitura (e também o `PUT` de atualização) **também
precisam continuar aceitando academias** consultando/testando/atualizando as
próprias credenciais — isso está documentado em `Documentação.md` ("Como
configurar como academia") e coberto por teste
(`TestAcademiaNaoReatribuiContextoAoAtualizarCredencial`). Aplicar
`RequireAdmin()" diretamente quebraria esse fluxo.

A causa raiz real: `RequireAcademiaOuAdmin()` deixa passar admin e academia,
mas **nunca preenche `admin_role`** no contexto (só `RequireAdminRole`/
`RequireFPP`/`RequireAdm`/`RequireGerente` fazem isso). Sem `admin_role`, a
função `user()` em `internal/handlers/financeiro_handlers.go` devolve
`"admin"` genérico em vez de `"fpp"/"adm"/"gerente"` — e tanto
`podeAcessarCredencial` quanto `AtualizarCredencial` exigem exatamente esses
valores granulares. Consequência prática: **até um FPP legítimo chamando essas
rotas via HTTP recebia "sem permissão"**, não só os roles inferiores.

### Correção aplicada

Novo middleware `PopulateAdminRole()` em
`internal/middleware/admin_auth_middleware.go`:

- Se o usuário autenticado **não** é admin (é academia, por exemplo), não faz
  nada e segue — nunca bloqueia.
- Se é admin, busca `role`, `status` e `email_verificado` em
  `projection_admins` (mesma query já usada por `RequireAdminRole`) e, se o
  admin estiver ativo e verificado, preenche `admin_role` no contexto.
- Em qualquer falha (admin não encontrado, inativo, e-mail não verificado,
  erro de banco, `dbClient` ausente/nil) — **nunca aborta**; apenas segue sem
  preencher `admin_role`, deixando a decisão de autorização para o
  serviço/handler downstream, exatamente como já acontecia antes desta
  correção.

Em `cmd/server/main.go`, adicionei uma única linha ao grupo `/financeiro`:

```go
financeiro := protected.Group("/financeiro")
financeiro.Use(middleware.RequireAcademiaOuAdmin())
financeiro.Use(middleware.PopulateAdminRole())
```

Isso resolve, ao mesmo tempo:

- `GET /financeiro/appypay/credenciais` (listar) — volta a funcionar para
  qualquer admin (fpp/adm/gerente).
- `GET /financeiro/appypay/credenciais/:id` (obter) — idem.
- `POST /financeiro/appypay/credenciais/:id/testar` (testar) — idem.
- `PUT /financeiro/appypay/credenciais/:id` (atualizar) — FPP autenticado via
  HTTP volta a conseguir atualizar credenciais.

As rotas que já usavam `RequireFPP()` diretamente (`POST credenciais`,
`.../ativar`, `.../desativar`, `POST modalidade-pagamento`) **não foram
alteradas** — `RequireAdminRole`/`RequireFPP` já resolvem `admin_role`
sozinhas, de forma independente.

## O que NÃO foi alterado

- `internal/finance/financeiro.go` — sem mudanças (item 1 já correto).
- `internal/handlers/financeiro_handlers.go` — sem mudanças; a função `user()`
  já fazia a resolução correta assim que `admin_role` estivesse disponível.
- Qualquer outra rota fora do grupo `/financeiro` (ex.: `/estudantes`, que
  também usa `RequireAcademiaOuAdmin()`) — o novo middleware só foi aplicado
  ao grupo financeiro, para manter o raio de impacto da correção restrito ao
  problema identificado pela auditoria.
- Dívida técnica já assumida conscientemente pela ronda anterior (idempotência
  em memória, HMAC de webhook, rotação de chave, fluxo de reembolso/reversão,
  `SQLLedger` legado, isolamento opcional estudante-academia) — mantida como
  estava; não é exigida pelo critério de saída desta ronda e não há ainda
  `Provider` real nem handlers de domínio expondo esses caminhos.

## Teste adicionado

`cmd/server/main_test.go` ganhou `TestFinanceiroAppyPayRoutesRequireAuthentication`,
seguindo exatamente o padrão dos testes de rota já existentes no arquivo
(ex.: `TestDocumentoRoutesRequireAuthentication`): confirma que as 8 rotas do
grupo `/financeiro` continuam registadas (não 404) e continuam a exigir
autenticação (401 sem token).

**Isso não substitui integralmente o item 3** do critério de saída da
auditoria. Validar ponta-a-ponta que FPP/adm/gerente conseguem listar/ver/
testar credenciais, e que um `gerente` continua bloqueado de operações
sensíveis (como reescrever segredos de uma credencial `spuri` via `PUT`),
exige uma base de dados de teste real — para popular `projection_admins` com
um admin de cada role e gerar tokens reais via
`middleware.GenerateToken`. Isso não foi possível reproduzir neste ambiente de
revisão (sem acesso ao Postgres nem ao restante do código do projeto — apenas
aos arquivos fornecidos na conversa).

**Recomendação para fechar o item 3:** seguir o padrão já usado em
`internal/db/event_store_integrity_test.go`, guardando o teste atrás de uma
env var (ex.: `SPURI_RUN_DB_INTEGRITY_TESTS=1`), subindo o router com um
`dbClient` real, criando um admin de cada role via `AggregateRepository`,
gerando um JWT por role com `middleware.GenerateToken`, e chamando:

- `GET /financeiro/appypay/credenciais` com token `gerente` → esperar `200`.
- `PUT /financeiro/appypay/credenciais/:id` com token `gerente` sobre uma
  credencial `spuri` já ativa → esperar `403`.
- O mesmo `PUT` com token `fpp` → esperar `200`.

## Como aplicar

Substitua estes três arquivos no repositório pelos anexados:

- `cmd/server/main.go`
- `internal/middleware/admin_auth_middleware.go`
- `cmd/server/main_test.go`

Nenhum outro arquivo do projeto precisa mudar para esta correção. Depois de
aplicar, rode `go build ./...` e `go test ./...` no seu ambiente — não foi
possível compilar/testar aqui, pois este ambiente de revisão não tem acesso
ao repositório completo (só aos arquivos anexados na conversa) nem a um
toolchain Go configurado com todas as dependências do módulo.
