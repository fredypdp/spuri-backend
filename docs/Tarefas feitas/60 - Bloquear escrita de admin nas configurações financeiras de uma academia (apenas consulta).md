---
criado: 2026-08-22
origem: Claude (orquestrador) — a pedido de Fredy Luís, Fundador e CEO da Spuri
status: concluída
tarefa: Bloquear escrita (criar/atualizar/remover/rotacionar) de admin nas configurações financeiras de uma academia no módulo de finança; admin passa a poder apenas consultar. Confirmado que o isolamento academia-vs-academia já está correto (sem ação necessária nessa parte).
---

# Bloquear escrita de admin nas configurações financeiras de uma academia (apenas consulta)

> **Papel do Codex nesta tarefa:** aplicar um patch já pronto, testado e validado três
> vezes (incluindo contra o `main` mais recente do GitHub no momento da entrega). **Não é
> necessário investigar, planejar, redesenhar ou decidir nada** — toda a investigação, o
> desenho da correção e a validação (inclusive contra PostgreSQL 16 real, o que o
> ambiente do Codex não consegue fazer) já foram feitos pelo Claude (orquestrador). O
> Codex só precisa aplicar o patch, conferir que bateu certo, rodar o que o seu ambiente
> permitir (seção 8) e commitar.

---

## 0. Resumo executivo

| Pergunta do Fredy | Resposta, com evidência real |
|---|---|
| Um admin consegue remover ou manipular (criar/atualizar/remover/rotacionar) as configurações financeiras de uma academia, quando deveria só poder consultar? | **Sim, isso estava acontecendo.** Confirmado e reproduzido contra PostgreSQL real: um admin com papel `fpp` conseguia criar, atualizar, remover e rotacionar credenciais AppyPay, configurações de mensalidade, configurações de matrícula e o mês de início de cobrança de **qualquer** academia. **Corrigido nesta tarefa.** |
| Uma academia consegue consultar ou manipular as configurações financeiras de outra academia? | **Não. Já estava corretamente bloqueado** (confirmado com teste real contra Postgres, inclusive contra um possível ataque via `id` de credencial). **Nenhuma ação necessária nessa parte** — não mexi em nada relacionado a isso. |

**Arquivos alterados:** 2 (ambos em `internal/handlers/`, backend Go).
**Arquivos a remover:** nenhum.
**Migração de banco de dados:** nenhuma (mudança é 100% de autorização em código Go, não toca em schema nem em eventos do ledger).
**Risco de regressão:** validado como nulo — suíte completa (`go test ./...`, todos os pacotes) passa 100%, antes e depois do patch, incluindo os testes que já cobriam admin/academia neste módulo.

---

## 1. O problema encontrado (com evidência, não suposição)

### 1.1 Onde estava o problema

Duas funções de autorização, ambas em `internal/handlers/`, decidem se um ator (academia
ou admin) pode agir sobre um "contexto financeiro" (uma academia específica, ou o
contexto global "spuri"):

- `authorizeFinanceScope` (`financeiro_handlers.go`) — usada pelas rotas de **credenciais
  AppyPay** (configurar, atualizar, remover, listar) e pelas rotas de **cobrança/QR
  Code**.
- `authorizeMensalidadeAcademia` (`mensalidade_handlers.go`) — usada pelas rotas de
  **configuração de mensalidade, matrícula e mês de início de cobrança** (configurar,
  listar, remover).

Antes desta correção, quando o ator era um admin com permissão `fpp`, as duas funções
autorizavam **qualquer** operação (leitura ou escrita) sobre **qualquer** academia
informada na requisição — sem nenhuma distinção entre "consultar" e "criar/atualizar/
remover". Ou seja: bastava o admin ter a permissão `fpp` e informar o `codigo_academia`
que quisesse no corpo da requisição.

### 1.2 Prova concreta (reproduzida contra PostgreSQL 16 real)

Escrevi um teste de prova de conceito que usa os handlers HTTP reais (não mock) e
confirma o efeito **direto no banco de dados** (contagem de linhas antes/depois de cada
chamada, não só o código HTTP de resposta). Com um admin `fpp` autenticado e uma academia
que **não é dele**, todas as operações abaixo tiveram sucesso (deveriam ter sido
bloqueadas com `403 Forbidden`):

1. `POST /financeiro/appypay/credenciais` — admin **criou** credenciais AppyPay da
   academia alheia (linhas no banco: `0 → 1`).
2. `POST /financeiro/mensalidades/configuracoes` — admin **criou** configuração de
   mensalidade da academia alheia (`0 → 1`).
3. `DELETE /financeiro/mensalidades/configuracoes` — admin **removeu** essa mesma
   configuração (`1 → 0`).
4. `POST /financeiro/matriculas/configuracoes` — admin **criou** configuração de
   matrícula da academia alheia (`0 → 1`).
5. `DELETE /financeiro/appypay/credenciais` — admin **removeu** as credenciais AppyPay da
   academia alheia (`1 → 0`).

Este teste foi usado só para diagnóstico (rodado no meu sandbox, contra Postgres real) e
**não faz parte da entrega** — foi apagado depois de confirmar o problema. Não precisa
ser recriado nem versionado.

### 1.3 Confirmação de que o isolamento academia-vs-academia já estava correto

Escrevi um segundo teste (mesmo estilo: handlers reais, academia atacante autenticada
tentando mexer nos dados de outra academia) cobrindo:

- `POST /financeiro/appypay/credenciais` de uma academia em nome de outra → **403**,
  nenhuma linha criada.
- `GET /financeiro/appypay/credenciais` de uma academia consultando outra → **403**.
- `POST /financeiro/mensalidades/configuracoes` de uma academia em nome de outra → **403**,
  nenhuma linha criada.
- `GET /financeiro/mensalidades/configuracoes` de uma academia consultando outra → **403**.

Todos os quatro já bloqueavam corretamente **antes** desta correção, e continuam
bloqueando depois (o patch não muda nenhum comportamento do ator `"academia"`). **Não há
nenhuma ação de código para esta parte.**

---

## 2. Desenho da correção (por que ficou assim, para quem for debugar depois)

A correção adiciona um parâmetro `write bool` às duas funções de autorização e a uma
função auxiliar que ambas usam indiretamente (`credentialScopeAuthorized`). A regra de
negócio é:

- **Ator `"academia"`**: comportamento **inalterado**. Só pode agir sobre o próprio
  contexto (`codigo_academia` é sempre forçado para o valor do próprio token,
  independentemente de `write`).
- **Ator `"admin"` com permissão `fpp`**:
  - `write=false` (consulta/listagem) → **continua liberado** para qualquer academia,
    exatamente como antes. É isso que corresponde a "admin só pode consultar".
  - `write=true` (criar, atualizar, remover, rotacionar) → **agora bloqueado** quando o
    contexto é o de uma academia específica (`contexto_tipo == "academia"`). A escrita do
    admin continua permitida **apenas** para o contexto `"spuri"` (as configurações do
    próprio Spuri — que não pertencem a nenhuma academia, então não se enquadram em "as
    configurações de uma academia").

Cada chamador (cada handler HTTP) passa `true` ou `false` de acordo com a natureza da
própria operação:

| Handler | Operação | `write` |
|---|---|---|
| `ConfigurarCredencialAppyPay` | criar credencial | `true` |
| `RemoverCredencialAppyPay` | remover credencial | `true` |
| `AtualizarCredencialAppyPay` | atualizar credencial | `true` |
| `ListarCredenciaisAppyPay` | listar/consultar | `false` |
| `ConsultarSegredoWebhookAppyPay` | consultar segredo | `false` |
| `RotacionarSegredoWebhookAppyPay` | rotacionar segredo (gera novo, invalida o antigo) | `true` |
| `ConfigurarMensalidade` | criar configuração | `true` |
| `ListarConfiguracoesMensalidade` | listar/consultar | `false` |
| `RemoverConfiguracaoMensalidade` | remover configuração | `true` |
| `ConfigurarMatricula` | criar configuração | `true` |
| `ListarConfiguracoesMatricula` | listar/consultar | `false` |
| `RemoverConfiguracaoMatricula` | remover configuração | `true` |
| `DefinirMesInicioCobranca` | definir (escrita) | `true` |
| `RemoverMesInicioCobranca` | remover (escrita) | `true` |

**Deliberadamente fora do escopo desta correção** (por não serem "configurações" de uma
academia, e sim operações transacionais pontuais de cobrança — o pedido do Fredy foi
especificamente sobre *configurações*):

| Handler | Por quê fica de fora | `write` passado |
|---|---|---|
| `CriarCobrancaAppyPay` | Emitir uma cobrança não é uma configuração — comportamento do admin preservado exatamente como estava. | `false` |
| `GerarQRCodeAppyPay` | Idem — operação sobre uma cobrança já existente. | `false` |
| `ConsultarCobrancaAppyPay` | Já era consulta; comportamento inalterado. | `false` |
| `ListarCobrancasAppyPay` | Já era consulta; comportamento inalterado. | `false` |

Note que estes quatro handlers **já usavam** `authorizeFinanceScope` antes desta
correção — a mudança de assinatura da função exige que **todo** chamador passe o novo
parâmetro, mas o valor escolhido para estes quatro (`false`) preserva **exatamente** o
comportamento que já existia. Isto está refletido no patch anexo (arquivo
`60_bloquear_escrita_admin_configuracoes_financeiras.patch`) e é a razão de o diff mexer
nessas 4 linhas mesmo sem mudar o comportamento delas — é só a assinatura da função que
mudou, não a regra de autorização aplicada a elas.

`CancelarCobrancaAppyPay` **não usa** `authorizeFinanceScope` (tem a própria lógica,
específica, que já restringe o admin a cancelar somente cobranças do contexto "spuri",
nunca de uma academia) — não é tocado por este patch.

As rotas de anular/reativar obrigações de mensalidade (`AnularObrigacoesMensalidade`/
`ReativarObrigacoesMensalidade`) também não usam nenhuma das duas funções alteradas — já
bloqueiam qualquer admin por completo (não são "configurações", são decisões de isenção
por estudante, e o código já as trata como exclusivas da própria academia). Não tocados
por este patch.

---

## 3. Arquivos afetados

**Nenhum arquivo precisa ser removido ou criado.** Apenas 2 arquivos existentes são
modificados:

1. `internal/handlers/financeiro_handlers.go`
2. `internal/handlers/mensalidade_handlers.go`

Nenhuma migração de banco, nenhum evento novo, nenhuma rota nova, nenhuma dependência
nova. A mudança é inteiramente local a essas duas funções de autorização e aos pontos
onde já eram chamadas.

---

## 4. Como aplicar — patch anexo (método principal)

O arquivo **`60_bloquear_escrita_admin_configuracoes_financeiras.patch`**, localizado em
`docs/Lista de Tarefas/`, contém o diff exato e completo das duas mudanças. Já foi
validado com `git apply --check` a partir de um clone limpo do `main` mais recente do
GitHub (checado no momento da entrega desta tarefa) — deve aplicar sem conflito.

```bash
git apply docs/Lista\ de\ Tarefas/60_bloquear_escrita_admin_configuracoes_financeiras.patch
```

Se por algum motivo o `main` tiver avançado o suficiente desde a entrega para o patch não
aplicar limpo (`error: patch does not apply`), **não tente adivinhar ou recriar a lógica**
— nesse caso, use o método alternativo da seção 5 (arquivos completos), que sempre
funciona independentemente do estado do `main`, porque substitui o arquivo inteiro.

---

## 5. Método alternativo — arquivos completos (fallback, se o patch não aplicar)

Se o `git apply` da seção 4 falhar por qualquer motivo, os dois arquivos completos e já
corrigidos estão anexos a este documento:

- `financeiro_handlers.go` (arquivo completo, 583 linhas)
- `mensalidade_handlers.go` (arquivo completo, 382 linhas)

Basta **substituir integralmente** os arquivos correspondentes em
`internal/handlers/financeiro_handlers.go` e `internal/handlers/mensalidade_handlers.go`
pelo conteúdo desses dois arquivos anexos, e seguir para a seção 8 (passo a passo).

**Atenção:** use este método alternativo **somente** se o patch da seção 4 falhar. Se o
`main` tiver avançado desde a entrega desta tarefa com mudanças nesses mesmos dois
arquivos (por exemplo, novos handlers ou novos endpoints), substituir o arquivo inteiro
por estas versões **descartaria essas mudanças novas**. Nesse cenário específico, pare e
sinalize para revisão manual em vez de substituir o arquivo às cegas — não tente mesclar
manualmente.

---

## 6. Validação já realizada por mim (evidência, o Codex pode confiar sem reproduzir)

Toda a validação abaixo foi feita em ambiente próprio com PostgreSQL 16 real e Go
1.24.12 (a mesma versão do `go.mod`), em três rodadas independentes:

1. **Antes do patch**, suíte completa (`go test ./...`, todos os pacotes) — 100% passa
   (baseline limpo, nenhum problema pré-existente).
2. **Prova da vulnerabilidade** (seção 1.2) — reproduzida com sucesso contra Postgres
   real, com verificação direta nas tabelas do banco (não só código HTTP).
3. **Prova do isolamento academia-vs-academia** (seção 1.3) — confirmado que já
   funcionava corretamente.
4. **Depois do patch**, no mesmo ambiente:
   - `go build ./...` — limpo.
   - `go vet ./...` — limpo.
   - `gofmt -l .` — limpo (nenhum arquivo listado).
   - Suíte completa (`go test ./...`, todos os pacotes, incluindo `internal/finance` e
     `internal/handlers`) — **100% passa, nenhuma regressão**, incluindo os testes que já
     existiam cobrindo comportamento de admin neste módulo
     (`TestIntegrationFinanceRejectsNonFPPAdmins`,
     `TestIntegrationFinanceFPPAdminCannotCancelAcademyCharge`,
     `TestIntegrationFPPAdminNaoPodeAnularOuReativarMensalidade`,
     `TestIntegrationHandlersRemocaoFinanceiraRespeitamEscopoDaAcademia`, entre outros).
   - Os dois testes de prova de conceito da seção 1 foram re-executados: o que antes
     confirmava a falha (admin escrevendo em academia alheia) **agora falha ao tentar
     confirmar a falha** — ou seja, a escrita é corretamente bloqueada (`403` em todas as
     5 operações) — e o teste de isolamento academia-vs-academia continua passando sem
     nenhuma mudança de comportamento.
5. **Validação final, independente, contra o `main` real e atual do GitHub** (feita
   depois de tudo acima, para garantir que nada no repositório avançou de forma
   incompatível durante a investigação): clonei `spuri-backend` do zero novamente,
   confirmei que o patch aplica limpo (`git apply --check`), apliquei de fato
   (`git apply`), e repeti build + vet + gofmt + suíte completa — **100% limpo, 100% dos
   testes passam**, sem nenhuma alteração adicional necessária.

## 7. Análise arquivo por arquivo (funcionalidade confirmada, sem erros)

### 7.1 `internal/handlers/financeiro_handlers.go`
- **Compila** (`go build`): sim.
- **`go vet`**: limpo.
- **`gofmt`**: limpo (formatação padrão Go aplicada).
- **Redeclarações/undefined**: nenhuma — `write bool` é o único parâmetro novo, adicionado
  de forma consistente em `authorizeFinanceScope` e `credentialScopeAuthorized`, e todas
  as 11 chamadas dessas duas funções no arquivo (e em todo o repositório — confirmado via
  busca global) foram atualizadas para a nova assinatura. Não sobra nenhuma chamada com a
  assinatura antiga.
- **Lógica isolada**: a mudança é inteiramente sobre autorização; nenhuma lógica de
  domínio (cobrança, credencial, ledger) foi tocada.
- **Testes de integração do próprio módulo** (`internal/finance`, que consome as funções
  indiretamente via `Service`, e os testes de `internal/handlers` que exercitam este
  arquivo diretamente): 100% passam.

### 7.2 `internal/handlers/mensalidade_handlers.go`
- **Compila** (`go build`): sim.
- **`go vet`**: limpo.
- **`gofmt`**: limpo.
- **Redeclarações/undefined**: nenhuma — `write bool` adicionado de forma consistente em
  `authorizeMensalidadeAcademia`; todas as 8 chamadas no arquivo foram atualizadas (2 como
  `false` para as rotas de listagem, 6 como `true` para as rotas de escrita). Confirmado
  via busca global que não sobra nenhuma chamada com a assinatura antiga em todo o
  repositório.
- **Lógica isolada**: idem — só autorização, nenhuma lógica de negócio (cálculo de
  mensalidade, resolução histórica de preço, event sourcing) foi tocada.
- **Testes de integração**: 100% passam, incluindo o teste que já cobria
  `TestIntegrationFPPAdminNaoPodeAnularOuReativarMensalidade` (rota não afetada por este
  patch, mas confirmando que a suíte inteira do arquivo continua saudável).

**Conclusão da análise:** os dois arquivos, depois do patch, compilam sem erro, passam em
`go vet` e `gofmt` sem nenhum apontamento, e toda a suíte de testes de integração (contra
PostgreSQL real) que os exercita passa a 100%, sem nenhuma regressão em nenhum outro
módulo do repositório.

---

## 8. Passo a passo exato para o Codex

1. No repositório `spuri-backend`, na raiz, aplicar o patch:
   ```bash
   git apply docs/Lista\ de\ Tarefas/60_bloquear_escrita_admin_configuracoes_financeiras.patch
   ```
   Se falhar, ver seção 5 (método alternativo com arquivos completos) — e se mesmo assim
   houver dúvida sobre conflito com mudanças novas nesses arquivos, parar e reportar em
   vez de decidir sozinho.

2. Conferir que só os 2 arquivos da seção 3 aparecem como modificados:
   ```bash
   git status
   ```

3. Rodar formatação e vet (não precisam de banco de dados, funcionam no ambiente do
   Codex mesmo sem `apt`/Docker/`psql`):
   ```bash
   gofmt -l .        # não deve listar nenhum arquivo
   go vet ./...       # ver seção 9 se o ambiente não conseguir baixar dependências
   ```

4. **Não tentar rodar os testes de integração**
   (`RUN_POSTGRES_INTEGRATION=1 go test ./...`) — o ambiente do Codex não tem PostgreSQL
   nem Docker. Essa parte **já foi validada de verdade por mim**, três vezes, incluindo a
   partir de um clone limpo do `main` atual com este mesmo patch aplicado (build limpo +
   suíte completa `go test ./...` de todos os pacotes, sem nenhuma falha). Ver seção 9
   para o detalhe de por que isso é confiável mesmo sem o Codex conseguir reproduzir.

5. Commitar com uma mensagem descrevendo a correção, por exemplo:
   ```
   fix(financeiro): bloqueia escrita de admin nas configuracoes de uma academia

   - authorizeFinanceScope e authorizeMensalidadeAcademia ganham parametro
     write bool: admin com permissao fpp continua podendo consultar (write=false)
     qualquer academia, mas nunca mais pode criar/atualizar/remover/rotacionar
     (write=true) as configuracoes financeiras de uma academia especifica
   - Escrita do admin continua permitida apenas no contexto "spuri" (nao
     pertence a nenhuma academia) e para a propria academia sobre si mesma
   - CriarCobrancaAppyPay, GerarQRCodeAppyPay, ConsultarCobrancaAppyPay e
     ListarCobrancasAppyPay preservam o comportamento anterior (write=false),
     por nao serem "configuracoes" e sim operacoes de cobranca
   - Isolamento academia-vs-academia confirmado como ja correto, sem alteracao
   - Validado com go build, go vet, gofmt e suite completa de testes de
     integracao contra PostgreSQL 16 real (100% passa, zero regressao)
   ```

---

## 9. Sobre a limitação real de testes no ambiente do Codex

O ambiente do Codex bloqueia `apt` (403 Forbidden) e não tem Docker nem `psql`. Isso
significa que o Codex **não consegue** rodar os testes de integração (`RUN_POSTGRES_
INTEGRATION=1 go test ./...`) nem aplicar migrations contra um banco real — mas esta
tarefa **não adiciona nem altera nenhuma migração**, então isso nem chega a ser relevante
aqui além da suíte de testes em si.

Para esta tarefa especificamente:
- **Positivo confirmado**: a parte que depende de banco de dados real (toda a suíte
  `go test ./...`, incluindo os testes de integração do módulo financeiro) foi validada
  por mim, três vezes, a mais recente delas a partir de um clone limpo do `main` atual do
  GitHub com o patch já aplicado. **100% dos testes passam.**
- **O que o Codex PODE e DEVE fazer**: aplicar o patch (ou os arquivos completos, se
  necessário), conferir com `gofmt`/`go vet` (funcionam sem banco), revisar que os
  arquivos batem com a lista da seção 3, e commitar.
- **O que o Codex NÃO precisa fazer**: tentar montar um Postgres, tentar rodar os testes
  de integração, ou questionar/replanejar o desenho da correção — isso tudo já foi
  decidido e comprovado nesta tarefa.
- Se `go build ./...` ou `go vet ./...` falhar no ambiente do Codex por causa de
  `golang.org/x/net`, `golang.org/x/crypto`, `google.golang.org/protobuf` ou
  `gopkg.in/yaml.v3` não resolverem (proxy de módulos Go bloqueado, mesmo tipo de
  restrição de rede do `apt`), isso é uma limitação de rede do ambiente, não um problema
  do patch — o `go.mod`/`go.sum` **não foram alterados** por este patch (propositalmente,
  para não gerar esse tipo de conflito), então qualquer projeto que já compilava antes do
  patch no ambiente do Codex deve continuar compilando depois. Nesse caso, revisar o
  código estaticamente (leitura) é suficiente; não é necessário forçar a compilação.

---

## 10. Critérios de aceitação

- [ ] `git status` mostra somente `internal/handlers/financeiro_handlers.go` e
      `internal/handlers/mensalidade_handlers.go` como modificados.
- [ ] `gofmt -l .` não lista nenhum arquivo.
- [ ] `go vet ./...` não reporta nenhum erro (ou, se o ambiente não conseguir baixar
      dependências por restrição de rede, isso é registrado como limitação ambiental, não
      como falha do patch — ver seção 9).
- [ ] Nenhum arquivo novo criado, nenhum arquivo removido.
- [ ] Nenhuma migração nova, nenhum evento novo, nenhuma rota nova.
- [ ] Commit feito com a mensagem descrita na seção 8, passo 5 (ou equivalente).
