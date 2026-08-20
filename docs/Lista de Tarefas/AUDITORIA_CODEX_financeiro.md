# Tarefa para o Codex — Auditoria pós-implementação e correção de documentação (financeiro)

**Autor da auditoria:** Claude (orquestrador), com validação real em PostgreSQL 16 + Go 1.24
num sandbox próprio — clone limpo do `main` atual (pós-merge do PR #557), migrations
aplicadas do zero, suite completa `go test ./...` rodada duas vezes contra banco recém
criado, comparação byte-a-byte do código commitado contra o patch original.

**Papel do Codex nesta tarefa:** aplicar um patch já pronto, testado e validado — só
documentação, nenhuma mudança de código Go. **Não precisa de PostgreSQL, Docker nem
`psql` para validar nada aqui** (diferente da tarefa anterior): dá para conferir 100% no
próprio ambiente do Codex, só com `git apply` e uma leitura do markdown resultante.

---

## 0. Veredito da auditoria

| Frente auditada | Resultado |
|---|---|
| Código Go do mecanismo de remoção (commit `8e92c44`) | ✅ **Correto, sem nenhum bug.** Idêntico byte-a-byte ao patch original em todos os arquivos de produção e de teste. |
| Testes automatizados | ✅ **100% passam**, duas vezes, do zero, contra PostgreSQL real, a partir do estado atual de `main` (não uma cópia antiga). |
| Build / `gofmt` / `go vet` | ✅ Limpos. |
| Documentação da API (`Documentação da API.md`) | ❌ **Faltavam as 4 rotas `DELETE` novas.** Corrigido nesta auditoria — ver patch anexo. |
| Ciclo de vida do arquivo de tarefa (`docs/Lista de Tarefas/` → `docs/Tarefas feitas/`) | ❌ **Não tinha sido movido**, e o `.patch` de entrega ficou esquecido em `docs/`. Corrigido nesta auditoria — ver patch anexo. |

**Conclusão: o módulo está pronto para produção do ponto de vista de código.** Os dois
problemas encontrados são de documentação/organização interna, não afetam a lógica nem o
funcionamento do backend — exatamente o tipo de coisa que você pediu para não bloquear a
ida para produção, mas que também pediu para corrigir agora porque notou a falta. Este
documento cobre só essa correção.

---

## 1. Como a auditoria de código foi feita (resumo, para registro)

1. Cloneizei `https://github.com/fredypdp/spuri-backend` (branch `main`) do zero.
2. Confirmei que o commit `8e92c44 feat(financeiro): mecanismo de remoção de
   configurações` está mesclado em `main` (PR #557).
3. Comparei, arquivo por arquivo, o conteúdo de `8e92c44` contra o patch original que eu
   tinha entregue (`financeiro_remocao_configuracoes.patch`, da tarefa anterior):
   - Os 10 arquivos de produção (`cmd/server/main.go`, `internal/db/safe_queries.go`,
     `internal/domain/aggregates/financeiro.go`, `internal/finance/appypay.go`,
     `internal/finance/matricula.go`, `internal/finance/mensalidade.go`,
     `internal/handlers/financeiro_handlers.go`,
     `internal/handlers/mensalidade_handlers.go`,
     `internal/projections/financeiro_projection.go`,
     `migrations/109_financeiro_remocao_configuracoes.sql`) batem **byte a byte**, exceto
     dois trechos em `internal/finance/appypay.go` e
     `internal/handlers/financeiro_handlers.go` que são de uma tarefa **totalmente não
     relacionada** (filtro `tipo`/`origens` em `ListCobrancasEstudante`, mencionando
     "tarefa 49" no comentário) que chegou em `main` de forma independente — o Codex não
     alterou nada indevidamente, e as duas mudanças convivem sem conflito.
   - Os 4 arquivos de teste novos
     (`internal/finance/appypay_remocao_integration_test.go`,
     `internal/finance/mensalidade_remocao_integration_test.go`,
     `internal/finance/matricula_remocao_integration_test.go`,
     `internal/handlers/financeiro_remocao_handlers_integration_test.go`) batem **byte a
     byte**, sem nenhuma diferença.
4. Instalei PostgreSQL 16 e Go 1.24 no meu sandbox, recriei o banco do zero, rodei
   `go build ./...`, `gofmt -l .`, `go vet ./...` (limpos) e `go test ./...` (todos os
   pacotes do repositório, não só `internal/finance`) — **duas vezes seguidas, sem
   cache**, contra o estado **atual** de `main`. 100% dos testes passam, incluindo os 6
   testes novos de remoção (`TestIntegrationRemoveCredentialRespeitaEventSourcing`,
   `TestIntegrationRemoveMatriculaConfiguracaoFluxoDeComando`,
   `TestIntegrationResolveConfiguracaoComRemocaoPreservaHistoricoDePreco`,
   `TestIntegrationRemoveMensalidadeConfiguracaoFluxoDeComando`,
   `TestIntegrationRemoveMesInicioCobrancaVoltaAoPadraoNatural`,
   `TestIntegrationHandlersRemocaoFinanceiraRespeitamEscopoDaAcademia`).

**Não há nenhuma ação de código a executar.** O Codex não precisa mexer em nenhum arquivo
`.go` nem em migrations por causa desta auditoria.

---

## 2. O que estava faltando (e por que)

### 2.1 Documentação da API

`Documentação da API.md`, seção "19. Financeiro / AppyPay", documenta minuciosamente
19.1–19.22 (todas as rotas existentes antes desta tarefa), inclusive com uma tabela-resumo
logo no início da seção. As 4 rotas `DELETE` novas — que já estão em produção, registradas
em `cmd/server/main.go` e funcionando — não tinham entrada nem na tabela-resumo nem como
subseção dedicada. Isso quebra a regra que o próprio repositório estabelece em
`docs/Lista de Tarefas/00 - Índice e ordem de implementação.md`: *"Toda tarefa que altera
contrato público exige atualização de `Documentação.md`... seguindo o mesmo padrão das
tarefas já concluídas."*

### 2.2 Ciclo de vida do arquivo de tarefa

O mesmo índice também estabelece: *"Ao concluir cada tarefa, siga o \"Procedimento de
conclusão\"... e mova o documento para `docs/Tarefas feitas/`."* O arquivo
`TAREFA_CODEX_financeiro.md` (o documento de execução que eu tinha entregue) continuava em
`docs/Lista de Tarefas/`, sem frontmatter (`criado`/`origem`/`status`/`tarefa`) e sem ter
sido movido — apesar de a tarefa já estar 100% implementada e commitada. O arquivo
`financeiro_remocao_configuracoes.patch` (artefato temporário só para o `git apply`)
também ficou esquecido lá — nenhuma outra tarefa concluída no histórico do repositório
mantém um `.patch` avulso em `docs/`; o padrão é o conteúdo já estar permanentemente
preservado no próprio commit do Git.

---

## 3. O que o patch anexo faz (`financeiro_documentacao_correcao.patch`)

Só 3 mudanças, todas em arquivos de documentação — **nenhum arquivo `.go` é tocado**:

1. **`Documentação da API.md`**: adiciona 4 linhas na tabela-resumo da seção 19 (logo
   depois da linha de `GET /financeiro/matriculas/configuracoes`) e 4 subseções novas,
   `19.23` a `19.26`, uma para cada rota `DELETE`, seguindo exatamente o mesmo formato das
   subseções vizinhas (Escopo da rota / Proteção / Request JSON / Response / Regras de
   negócio / Erros comuns). O conteúdo já reflete com precisão o comportamento real do
   código (autorização, efeito no ledger, preservação de histórico, efeito nas rotas de
   listagem e nas rotas de pagamento do estudante).
2. **Move** `docs/Lista de Tarefas/TAREFA_CODEX_financeiro.md` para
   `docs/Tarefas feitas/56 - Mecanismo de remoção de configurações financeiras do módulo
   AppyPay (event sourcing).md`, com frontmatter (`status: concluída`) e uma "Nota de
   fechamento" no topo explicando o resultado real (commit, PR, resultado da auditoria),
   mantendo o texto original do documento de execução abaixo como registro histórico do
   raciocínio/design.
3. **Remove** `docs/Lista de Tarefas/financeiro_remocao_configuracoes.patch` (conteúdo já
   preservado para sempre no commit `8e92c44`).

---

## 4. Passo a passo EXATO para o Codex

1. Na raiz do repositório `spuri-backend` (branch atual, a mesma onde `8e92c44` já está
   commitado):
   ```bash
   git apply financeiro_documentacao_correcao.patch
   ```
   (Testado com `git apply --check` a partir de um clone limpo de `main` — aplica sem
   conflito. Se a `main` tiver avançado desde este documento e o patch não aplicar limpo,
   volte para mim antes de tentar resolver manualmente — não improvise a resolução.)

2. Conferir:
   ```bash
   git status --short
   ```
   Deve mostrar exatamente:
   - `M "Documentação da API.md"`
   - a remoção de `docs/Lista de Tarefas/TAREFA_CODEX_financeiro.md` e
     `docs/Lista de Tarefas/financeiro_remocao_configuracoes.patch`
   - a criação de `docs/Tarefas feitas/56 - Mecanismo de remoção de configurações
     financeiras do módulo AppyPay (event sourcing).md`

3. Não é necessário rodar `go build`/`go vet`/`gofmt` nem testes — nenhum arquivo `.go`
   foi alterado por este patch. Se quiser confirmar mesmo assim que nada de código mudou:
   ```bash
   git diff --cached --stat -- '*.go' '*.sql'   # deve vir vazio
   ```

4. Commitar, por exemplo:
   ```
   docs(financeiro): documentar rotas DELETE e arquivar tarefa concluída

   - Seção 19 de Documentação da API.md: subseções 19.23-19.26 + tabela-resumo
   - Move TAREFA_CODEX_financeiro.md para docs/Tarefas feitas/ (nº 56), com frontmatter
   - Remove o .patch de entrega, já preservado no commit 8e92c44
   - Auditoria pós-implementação (Claude, PostgreSQL 16 real): código do commit 8e92c44
     confirmado correto e idêntico ao patch original; suite completa 100% verde
   ```

5. Se o fluxo do time exigir PR (como da última vez): abrir o PR normalmente. Não é
   necessário nenhuma revisão adicional de conteúdo além de conferir que o `git status`
   bateu com o item 2 acima.

---

## 5. Por que isto é seguro de aplicar sem re-planejar nada

- O texto das 4 novas subseções da API já foi escrito por mim, palavra por palavra,
  espelhando o formato exato das 22 subseções vizinhas — não há decisão de estilo ou
  conteúdo a tomar.
- A auditoria de código (seção 1 deste documento) já provou, com PostgreSQL real, que o
  comportamento documentado (efeito no ledger, preservação de histórico, bloqueio de
  cobrança, reversão ao padrão natural) é exatamente o que o código faz — não é uma
  suposição.
- Mover o arquivo de tarefa e apagar o `.patch` são operações puramente organizacionais,
  sem risco técnico: o conteúdo integral de ambos já está preservado no histórico do Git
  (commit `8e92c44` para o código, e o próprio arquivo renomeado preserva o texto original
  da tarefa).
