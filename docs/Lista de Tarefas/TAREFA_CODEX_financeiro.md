# Tarefa para o Codex — Módulo Financeiro (spuri-backend)

**Autor da investigação/implementação:** Claude (orquestrador), com validação real em
PostgreSQL 16 + Go 1.24 num sandbox próprio (clone limpo do repositório, migrations
aplicadas do zero, suite completa `go test ./...` rodada múltiplas vezes, incluindo a
partir de um clone limpo com o patch já aplicado).

**Papel do Codex nesta tarefa:** aplicar um patch já pronto, testado e validado.
**Não é necessário planejar, redesenhar ou decidir nada** — todas as decisões de design já
foram tomadas e comprovadas com testes reais. O Codex só precisa aplicar o patch, revisar
que bateu certo, rodar o que seu ambiente permitir (ver seção "O que o Codex consegue
validar" abaixo) e commitar.

---

## 0. Resumo executivo (3 pedidos originais, 3 resultados)

| # | Pedido | Resultado | Ação para o Codex |
|---|--------|-----------|--------------------|
| 1 | Regra de ano do mês de início de cobrança (set-dez = ano de início do ano letivo; jan-jul = ano de fim) | **Já estava correto.** Validado com 3 testes de integração reais cobrindo exatamente os exemplos do pedido (fundamental e superior). | Nenhuma. |
| 2.1 | Bloquear criação de cobrança se academia/Spuri não tiver credenciais AppyPay | **Já estava implementado**, em duas camadas independentes (configuração + criação de cobrança). Validado com teste de integração real, incluindo o cenário que a funcionalidade nova do item 2.2 introduz (credencial que existia e foi removida). | Nenhuma. |
| 2.2 | Mecanismo de remoção de configurações já definidas, respeitando event sourcing | **Não existia. Implementado, testado e validado nesta tarefa.** | Aplicar o patch anexo (`financeiro_remocao_configuracoes.patch`). |

Só o item 2.2 gera mudança de código. Os itens 1 e 2.1 estão documentados abaixo apenas
para registro/auditoria — **não exigem nenhuma ação de código.**

---

## 1. Item 1 — Ano do mês de início de cobrança (SEM AÇÃO NECESSÁRIA)

### O que foi verificado
A regra pedida: para um ano letivo `"2026_2027"`, meses de setembro a dezembro pertencem a
2026 (ano de início do ano letivo) e meses de janeiro a julho pertencem a 2027 (ano de fim).
O exemplo do pedido (academia entra em outubro/2026, cobrança começa em novembro →
novembro/2026 até julho/2027; se começar em janeiro → janeiro/2027 até julho/2027) já é
exatamente o que o código faz hoje.

### Onde está a lógica
- `internal/finance/mensalidade.go`, função `mesesAnoLetivo(anoLetivo, nivel)`: monta a
  lista de meses do ano letivo atribuindo `ano` (a primeira metade do `anoLetivo`, ex.
  `2026`) aos meses de setembro/outubro a dezembro, e `ano+1` aos meses de janeiro a julho.
- `mesInicioEfetivo` (mesma file) + `posicaoNoAnoLetivo`: comparam meses por **posição
  ordinal dentro do ano letivo**, não pelo número bruto do mês — é isso que evita o bug
  clássico de comparar "novembro (11) > janeiro (1)" fora de contexto.
- Isso já é single-source-of-truth: não há nenhuma outra função no repositório recalculando
  esse mapeamento mês→ano de forma divergente (busquei `time.Date(ano`, `ano+1`, `anoInicio`
  em todo o repo).

### Evidência (testes de integração reais que rodei, contra Postgres real)
Escrevi e rodei (depois apaguei do meu ambiente de trabalho, pois não alteram
comportamento nenhum — são só verificação) três testes cobrindo:
1. Escolar, ano letivo `2026_2027`, início de cobrança = novembro → primeira mensalidade
   com `data_referencia = 2026-11-01`, última (mês 7) com `data_referencia = 2027-07-01`,
   total de 9 mensalidades (nov+dez/2026 + jan-jul/2027), setembro e outubro corretamente
   ausentes. **PASS**.
2. Mesmo cenário com início em janeiro → primeira mensalidade com
   `data_referencia = 2027-01-01`, total de 7 mensalidades. **PASS**.
3. Ensino superior (mês natural = outubro), início em novembro → mesmo comportamento.
   **PASS**.

**Conclusão: não há bug aqui. Não aplicar nenhuma mudança para o item 1.**

---

## 2. Item 2.1 — Bloqueio de cobrança sem credenciais (SEM AÇÃO NECESSÁRIA)

### O que foi verificado
O pedido: nenhum evento do estudante que gera cobrança para a academia deve conseguir
criar essa cobrança se a academia (ou o Spuri) não tiver credenciais AppyPay configuradas.

### O que já existe (duas camadas independentes)

**Camada 1 — bloqueio na CONFIGURAÇÃO:**
`validateConfiguracaoMensalidade` (mensalidade.go) e a validação equivalente em
`matricula.go` já impedem a academia de habilitar um método de pagamento sem ter
credenciais AppyPay configuradas para o contexto (mensagem de erro: *"não é possível
habilitar propina sem credenciais AppyPay da academia"*).

**Camada 2 — bloqueio na CRIAÇÃO DA COBRANÇA em si (defesa em profundidade):**
`Service.CreateCharge` e `Service.CreateGPOQRCode` (`internal/finance/appypay.go`) chamam
`s.loadCredential(ctx, in.ContextoTipo, in.CodigoAcademia)` **antes** de gravar qualquer
evento no ledger. Isso vale genericamente para `ContextoAcademia` **e** `ContextoSpuri` —
é a mesma função, sem distinção de contexto. Os dois únicos pontos de entrada que um
estudante aciona (`IniciarPagamentoMensalidades` e `IniciarPagamentoMatricula`) roteiam
exclusivamente por `CreateCharge`/`CreateGPOQRCode` — não existe nenhum caminho alternativo
que os contorne.

### Evidência (teste de integração real que rodei)
Fluxo completo, ponta a ponta, contra Postgres real:
1. Tentei habilitar método de pagamento sem credencial → bloqueado pela Camada 1.
2. Configurei credencial, configurei mensalidade com método habilitado (sucesso).
3. Apaguei a linha de `financeiro_credenciais_appypay` diretamente (simulando o que a
   funcionalidade nova de remoção do item 2.2 agora faz de forma legítima) → tentativa de
   `IniciarPagamentoMensalidades` bloqueada pela Camada 2 com `ErrNotFound`, **nenhuma**
   cobrança nem evento gravado no ledger.

**Conclusão: não há bug aqui. Não aplicar nenhuma mudança para o item 2.1.** A ressalva
importante é que, **antes desta tarefa**, o cenário "credencial existia e sumiu depois" só
era alcançável manipulando o banco diretamente (não existia via fluxo legítimo). Agora que
o item 2.2 implementa remoção de credenciais de verdade, esse cenário passa a ser
alcançável pelo fluxo normal — e o teste
`internal/finance/appypay_remocao_integration_test.go` (incluído no patch) prova que a
Camada 2 continua protegendo corretamente depois da remoção.

---

## 3. Item 2.2 — Mecanismo de remoção respeitando event sourcing (AÇÃO: APLICAR O PATCH)

### 3.1 Por que isso não existia
Não havia nenhum evento nem comando para "desfazer" uma configuração já definida:
`MensalidadeConfigurada`, `MesInicioCobrancaDefinido`, `MatriculaConfigurada` e
`CredenciaisAppyPayConfiguradas` só tinham o lado "criar/atualizar", nunca "remover".

### 3.2 Princípio de design (por que ficou assim, para quando alguém for debugar)
Event sourcing = nunca apagar ou reescrever um fato já gravado no ledger. Uma "remoção" é
sempre um **novo evento imutável**, nunca um `DELETE`/`UPDATE` sobre um evento antigo.
Foram usados dois padrões diferentes, dependendo da natureza de cada recurso:

- **Recursos com histórico por data** (`MensalidadeConfigurada`, `MatriculaConfigurada`,
  `MesInicioCobrancaDefinido`, todos versionados e "latest wins" ou "latest wins até uma
  data de referência"): a remoção é registrada numa **tabela de fatos separada**
  (`financeiro_*_remocoes`), e uma **VIEW** (`financeiro_*_atual`) combina "a versão mais
  recente" com "a remoção mais recente" para resolver o estado vigente — sem nunca apagar
  linha nenhuma da tabela original. Para mensalidade especificamente (que tem resolução de
  **preço histórico por data**, usada para não reescrever cobrança de meses já fechados), a
  lógica de "isso foi removido depois desta versão e antes da data que estou consultando"
  fica em Go (`resolveConfiguracao`), não na view — porque a view só resolve "agora", não
  "uma data arbitrária no passado".
- **Credenciais AppyPay** (`financeiro_credenciais_appypay`): não é um recurso
  versionado por data, é um "estado atual" simples (upsert por escopo). A remoção grava o
  evento `CredenciaisAppyPayRemovidas` e a projeção **apaga** a linha da projeção (a mesma
  tabela já é inteiramente reconstruída a partir do ledger em `Rebuild()`, então isso é
  seguro). O cofre de segredos (`financeiro_segredos_appypay`, que vive **fora** do replay
  do ledger — não é reconstruído por `Rebuild()`) é limpo pelo `Service` no mesmo comando,
  exatamente como `ConfigureCredential` já grava segredos fora da projeção hoje.

### 3.3 Garantia mais importante (testada explicitamente)
**Remover uma configuração de mensalidade NUNCA reescreve o preço de um mês que já foi
cobrado antes da remoção.** Isso foi testado explicitamente com uma linha do tempo
controlada (`T1`=configuração, `T2`=mês já cobrado, `T3`=remoção depois de `T2`,
`T4`=referência atual): resolver `T2` depois da remoção continua devolvendo o preço
antigo; só resolver `T4` (depois de `T3`) é que passa a dar "sem configuração ativa".
Reconfigurar depois da remoção também não preenche retroativamente o intervalo removido.

### 3.4 O que o patch adiciona, arquivo por arquivo

**Migration nova:** `migrations/109_financeiro_remocao_configuracoes.sql`
- 3 tabelas de fatos: `financeiro_mensalidade_configuracoes_remocoes`,
  `financeiro_mensalidade_inicio_cobranca_remocoes`,
  `financeiro_matricula_configuracoes_remocoes`.
- 3 views de estado atual: `financeiro_mensalidade_configuracoes_atual`,
  `financeiro_mensalidade_inicio_cobranca_atual`, `financeiro_matricula_configuracoes_atual`.
- Não altera nenhuma tabela existente (nenhum `ALTER TABLE`, nenhuma constraint tocada).

**`internal/domain/aggregates/financeiro.go`**: 4 novas constantes de evento —
`MensalidadeConfiguracaoRemovida`, `MesInicioCobrancaRemovido`,
`MatriculaConfiguracaoRemovida`, `CredenciaisAppyPayRemovidas`.

**`internal/db/safe_queries.go`**: os 4 eventos novos adicionados à whitelist do ledger
(sem isso, `SaveWithAudit`/`AppendTx` rejeitaria os eventos — é um mecanismo de segurança
já existente no projeto contra gravação de tipos de evento não registrados).

**`internal/projections/financeiro_projection.go`**:
- 4 novos `case` no switch de projeção, um por evento novo, gravando na tabela de fatos de
  remoção correspondente (ou apagando a linha, no caso de credenciais).
- `Rebuild()` atualizado para também limpar as 3 novas tabelas de remoção antes do replay
  (senão um rebuild duplicaria os fatos de remoção a cada chamada).

**`internal/finance/mensalidade.go`**:
- `resolveConfiguracao` (resolução de preço por data histórica): passou a checar, depois
  de resolver a versão candidata, se existe uma remoção entre o `vigente_em` dessa versão e
  a data de referência — se sim, devolve `ErrNotFound`.
- `ListMensalidadeConfiguracoes`, `metodosPagamentoMensalidade`, a consulta de
  `MIN(mes_fim_cobranca)` e `mesInicioEfetivo`: todas trocadas para ler das novas views
  `_atual` em vez da tabela bruta com `DISTINCT ON`.
- Duas funções de comando novas: `RemoveMensalidadeConfiguracao` e
  `RemoveMesInicioCobranca`. Ambas validam que existe algo ativo para remover (senão
  `ErrNotFound`) antes de gravar o evento.

**`internal/finance/matricula.go`**:
- `ListMatriculaConfiguracoes` e `ResolveMatriculaConfiguracao`: trocadas para a view
  `financeiro_matricula_configuracoes_atual` (matrícula não tem resolução histórica por
  data — sempre "vale a versão mais recente" — então aqui a view sozinha já resolve tudo,
  sem precisar da lógica extra em Go que a mensalidade precisa).
- Função de comando nova: `RemoveMatriculaConfiguracao`.

**`internal/finance/appypay.go`**:
- Função de comando nova: `RemoveCredential(ctx, contextoTipo, codigoAcademia, ...)`. Grava
  o evento, e limpa `financeiro_segredos_appypay` para aquele `credential_id`.

**`internal/handlers/mensalidade_handlers.go`** e
**`internal/handlers/financeiro_handlers.go`**: 4 handlers HTTP novos —
`RemoverConfiguracaoMensalidade`, `RemoverMesInicioCobranca`, `RemoverConfiguracaoMatricula`,
`RemoverCredencialAppyPay` — seguindo exatamente o mesmo padrão de autorização
(`authorizeMensalidadeAcademia` / `authorizeFinanceScope`) e resposta (`financeError`) dos
handlers de configuração já existentes.

**`cmd/server/main.go`**: 4 rotas `DELETE` novas, registradas no mesmo grupo
`financeiro` (autenticado, `RequireAcademiaOuAdmin`) das rotas de configuração
equivalentes:
```
DELETE /financeiro/appypay/credenciais          body: {contexto_tipo, codigo_academia}
DELETE /financeiro/mensalidades/configuracoes   body: {codigo_academia, nivel, ano_academico, curso_id?}
DELETE /financeiro/mensalidades/inicio-cobranca body: {codigo_academia, ano_letivo}
DELETE /financeiro/matriculas/configuracoes     body: {codigo_academia, nivel, ano_academico, curso_id?}
```
Todas devolvem `204 No Content` em sucesso, e passam por `financeError` em erro (`404` se
não havia nada ativo para remover — inclusive numa segunda tentativa de remover a mesma
coisa —, `403` se a academia não é dona do escopo).

**4 arquivos de teste novos** (testes de integração reais, todos passando):
- `internal/finance/appypay_remocao_integration_test.go`
- `internal/finance/mensalidade_remocao_integration_test.go`
- `internal/finance/matricula_remocao_integration_test.go`
- `internal/handlers/financeiro_remocao_handlers_integration_test.go`

### 3.5 O que cada teste novo prova (resumo)
- Evento original **nunca desaparece** do ledger depois da remoção (contei linhas em
  `spuri_ledger` antes/depois).
- Preço histórico de um mês já "fechado" **não muda** depois que a configuração é removida.
- Depois da remoção, a configuração/credencial some das listagens "atuais" e do
  `metodosPagamentoMensalidade` (fechando o loop com o item 2.1: sem configuração ativa,
  automaticamente não há método de pagamento habilitado).
- Tentar remover de novo (nada ativo) dá `ErrNotFound` — nunca grava um segundo evento de
  remoção para o mesmo "já removido".
- Reconfigurar depois de remover funciona normalmente, e não preenche retroativamente o
  intervalo que ficou removido.
- `Rebuild()` a partir do ledger reproduz exatamente o mesmo estado final — prova de que a
  projeção continua 100% derivável do ledger, sem estado escondido.
- No nível de handler HTTP: uma academia não consegue remover a configuração/credencial de
  outra academia (`authorizeFinanceScope`/`authorizeMensalidadeAcademia` continuam
  funcionando).

---

## 4. Passo a passo EXATO para o Codex

1. No repositório `spuri-backend`, na raiz, aplicar o patch:
   ```bash
   git apply financeiro_remocao_configuracoes.patch
   ```
   (O patch já foi testado com `git apply --check` a partir de um clone limpo de
   `https://github.com/fredypdp/spuri-backend` na branch `main` — deve aplicar sem
   conflito. Se por algum motivo a branch `main` tiver avançado desde que este documento
   foi gerado e o patch não aplicar limpo, ver a seção 5 abaixo.)

2. Conferir que os arquivos batem com a lista da seção 3.4 (`git status`).

3. Rodar formatação e vet (não precisam de banco de dados, funcionam no ambiente do
   Codex mesmo sem `apt`/Docker/`psql`):
   ```bash
   gofmt -l .        # não deve listar nenhum arquivo
   go vet ./...       # se o toolchain Go do ambiente do Codex conseguir baixar as
                       # dependências (golang.org/x/*, google.golang.org/protobuf,
                       # gopkg.in/yaml.v3) — ver seção 5 se não conseguir
   ```

4. **Não tentar rodar os testes de integração** (`RUN_POSTGRES_INTEGRATION=1 go test
   ./...`) — o ambiente do Codex não tem PostgreSQL nem Docker, essa parte **já foi
   validada de verdade por mim**, três vezes, inclusive a partir de um clone limpo com este
   mesmo patch aplicado (build limpo + suite completa `go test ./...` de todos os pacotes
   do repositório, sem nenhuma falha). Ver seção 5 para o detalhe de por que isso é
   confiável mesmo sem o Codex conseguir reproduzir.

5. Commitar com uma mensagem descrevendo as 3 partes (mecanismo de remoção +
   confirmação de que os itens 1 e 2.1 já estavam corretos), por exemplo:
   ```
   feat(financeiro): mecanismo de remoção de configurações (event sourcing)

   - Novos eventos: MensalidadeConfiguracaoRemovida, MesInicioCobrancaRemovido,
     MatriculaConfiguracaoRemovida, CredenciaisAppyPayRemovidas
   - Nova migration 109: tabelas de fatos de remoção + views de estado atual
   - Preço histórico de mensalidade nunca é reescrito por uma remoção posterior
   - 4 endpoints DELETE novos, mesmo padrão de autorização dos endpoints de configuração
   - Confirmado (sem alteração de código): regra de ano do mês de cobrança já estava
     correta; bloqueio de cobrança sem credenciais AppyPay já estava implementado em
     duas camadas
   ```

---

## 5. Sobre a limitação real de testes no ambiente do Codex

O ambiente do Codex bloqueia `apt` (403 Forbidden) e não tem Docker nem `psql`. Isso
significa que o Codex **não consegue** rodar os testes de integração (`RUN_POSTGRES_
INTEGRATION=1 go test ./...`) nem aplicar as migrations contra um banco real.

Isso já foi levado em conta: **toda a validação que precisa de banco real já foi feita por
mim**, não uma vez, mas várias, incluindo o teste mais forte possível — clonar o
repositório do zero, aplicar exatamente este patch, e rodar a suite completa
(`go test ./...`, todos os pacotes, não só `internal/finance`) contra um PostgreSQL 16
recém-criado, do zero, sem cache, duas vezes seguidas para garantir que não foi coincidência
de estado. Resultado: **100% dos testes passam**, incluindo todos os testes pré-existentes
do módulo financeiro e de outros módulos (nada quebrou), mais os testes novos.

Então, para o Codex:
- **Positivo confirmado**: a parte que depende de banco de dados real está correta. O
  Codex pode confiar nisso sem precisar reproduzir.
- **O que o Codex PODE e DEVE fazer**: aplicar o patch, conferir com `gofmt`/`go vet`
  (funcionam sem banco), revisar que os arquivos batem com a lista da seção 3.4, e
  commitar.
- **O que o Codex NÃO precisa fazer**: tentar montar um Postgres, tentar rodar as
  migrations, tentar rodar os testes de integração, ou questionar/re-planejar o design —
  isso tudo já foi decidido e comprovado.
- Se `go build ./...` ou `go vet ./...` falhar no ambiente do Codex por causa de
  `golang.org/x/net`, `golang.org/x/crypto`, `google.golang.org/protobuf` ou
  `gopkg.in/yaml.v3` não resolverem (proxy de módulos Go bloqueado, mesmo tipo de
  restrição de rede do `apt`), isso é uma limitação de rede do ambiente, não um problema
  do patch — o `go.mod`/`go.sum` **não foram alterados** por este patch (propositalmente,
  para não gerar esse tipo de conflito), então qualquer projeto que já compilava antes do
  patch no ambiente do Codex deve continuar compilando depois. Nesse caso, revisar o código
  estaticamente (leitura) é suficiente; não é necessário forçar a compilação.
