---
modificado: 2026-07-30 23:11
criado: 2026-07-30 23:11
---
# Depuração — Módulo base de gestão financeira com AppyPay

## Objetivo da auditoria

Confirmar se a tarefa `docs/Tarefas feitas/15 - Modulo base de gestao financeira com AppyPay.md` está corretamente implementada no código atual (`internal/finance/financeiro.go`, `internal/domain/aggregates/financeiro.go`, `internal/projections/financeiro_projection.go`, `internal/handlers/financeiro_handlers.go`, `cmd/server/main.go`, `internal/db/safe_queries.go`, migrations `097`/`098`), seguindo o mesmo padrão dos demais debugs do repositório.

Existe um debug anterior (`docs/Debbugs/Depurar modulo base de gestao financeira com AppyPay.md`) que deu o módulo como correto, com apenas o ajuste da moeda `AOA`. Essa auditoria estava incompleta: os testes existentes só exercitam o mesmo processo em memória e nunca testam nem o _replay_ real via `FinanceiroProjection`, nem o papel `estudante`. Por isso vários problemas reais passaram despercebidos.

## Resultado geral

A base está **parcialmente implementada**: contextos `spuri`/`academia`, modalidade de pagamento, idempotência de cobrança, criptografia de segredos e whitelist de eventos existem e funcionam. Porém há **uma falha de autorização real e explorável**, e **três problemas arquiteturais** que comprometem exatamente a garantia central da tarefa 15/16 ("ledger como fonte de verdade auditável e reconstruível").

---

## Achados críticos

### 1. 🔴 CRÍTICO — Estudante contorna o isolamento por academia em 3 funções de credenciais

`ListarCredenciais`, `ObterCredencial` e `TestarCredencial` (em `internal/finance/financeiro.go`) usam o padrão **"permitir por padrão, negar só se for `academia` com mismatch"**:

```go
if autorTipo == "academia" && (c.ContextoTipo != ContextoAcademia || c.CodigoAcademia != codAcad) {
    continue // ou return erro
}
```

Isso significa que **qualquer papel diferente de `"academia"`** — em particular `"estudante"` — **passa direto pelo filtro**, porque a condição nunca é verdadeira para ele. Resultado:

- `GET /financeiro/appypay/credenciais` — um estudante autenticado lista **todas** as credenciais (spuri e de todas as academias), mascaradas mas com `client_id`, `ambiente`, URLs, `applicationId` e `webhook` visíveis.
- `GET /financeiro/appypay/credenciais/:id` — um estudante consulta **qualquer** credencial por UUID.
- `POST /financeiro/appypay/credenciais/:id/testar` — um estudante **executa** o teste de qualquer credencial e **grava** o evento `CredenciaisAppyPayValidadas` no ledger com o próprio `autorID`/`autor_tipo="estudante"` — ou seja, não é só vazamento de leitura, é escrita de evento por um ator sem permissão nenhuma sobre o módulo financeiro.

As rotas estão no grupo `protected` de `cmd/server/main.go`, que só exige `middleware.AuthMiddleware()` (qualquer tipo de usuário autenticado), sem `RequireAcademiaOuAdmin()` nem equivalente.

Por comparação, `AtualizarCredencial`, `AlterarStatusCredencial`, `AlterarModalidade` e `CriarCredencial` usam o padrão correto ("negar por padrão, permitir só se…"), então não sofrem do problema — o bug está isolado a essas 3 funções.

**Por que não foi pego antes:** o único teste de isolamento (`TestIsolamentoAcademiasEIdempotencia`) só testa `academia` vs `academia`, nunca `estudante`.

**Correção sugerida:**

```go
if autorTipo != "fpp" && autorTipo != "admin" &&
   !(autorTipo == "academia" && c.ContextoTipo == ContextoAcademia && c.CodigoAcademia == codAcad) {
    // negar / continue
}
```

Aplicar o mesmo padrão nas três funções, e reforçar com `middleware.RequireAcademiaOuAdmin()` no grupo `/financeiro` do router como defesa em profundidade.

---

### 2. 🟠 ALTO — Escritor duplo (`Service` + `FinanceiroProjection`) degrada o histórico auditável em produção

Existem **dois** caminhos independentes escrevendo nas mesmas tabelas `financeiro_*`:

1. **Síncrono**: métodos do `Service` (`CriarCredencial`, `AlterarStatusCredencial`, etc.) chamam `s.projectCredencial(ctx, c)` com o objeto `c` completo, incluindo `Historico` (autor, motivo, timestamp de cada ação).
2. **Assíncrono**: `internal/projections/financeiro_projection.go` (`FinanceiroProjection`, registrada em `initProjections()` e processada continuamente por `projManager.StartProcessing()`) reconstrói o mesmo registro **só a partir do `payload` do evento do ledger** e faz `INSERT ... ON CONFLICT DO UPDATE SET payload=EXCLUDED.payload`, **sobrescrevendo a linha inteira**.

O problema: a reconstrução do caminho 2 **nunca popula `Historico`** (nem para credenciais, nem para modalidade com `AutorID`/`AutorTipo`/`Motivo`). Como o `projManager` roda em loop contínuo processando todo evento novo do ledger, **pouco depois de qualquer escrita financeira, o registro rico (com histórico completo) é sobrescrito por uma versão empobrecida** — isso acontece em operação normal, não só em rebuild manual.

Isso viola diretamente o critério de aceite da tarefa 16: _"projeções financeiras forem reconstruíveis por replay"_ — elas são reconstruíveis, mas para um estado **pior** do que o escrito originalmente, o que é o oposto do que CQRS deveria garantir (convergência para o mesmo estado completo).

**Correção sugerida:** eliminar um dos dois escritores. O caminho recomendado é manter só `FinanceiroProjection` (a via "oficial" registrada no Projection Manager, usada pelo rebuild administrativo) e fazer com que ela leia também `event.Metadata` (que já contém `user_id`/`user_type`/`origem`) para reconstruir `Historico` corretamente — e então remover as escritas diretas em `Service.project*`, deixando o `Service` apenas ler do banco/cache quando precisar responder a uma requisição.

---

### 3. 🟠 ALTO — `motivo` nunca é gravado no payload dos eventos (sistemático)

Em todas as operações que recebem `motivo` como parâmetro, o valor é atribuído só ao `EventoFinanceiro` em memória, mas **nunca** entra no `payload` passado a `s.record(...)`:

|Função|Payload gravado no ledger|`motivo` incluído?|
|---|---|---|
|`AlterarModalidade`|`{"escopo":..., "codigo_academia":..., "ativa":...}`|❌|
|`CancelarCobrancaFinanceiraBase`|`chargePayload(c)`|❌|
|`AlterarStatusCredencial`|`credentialPayload(c)`|❌|
|`ReembolsarCobrancaFinanceiraBase`|`{"cobranca_id":..., "valor":...}`|❌|
|`ReverterCobrancaFinanceiraBase`|`{"cobranca_id":...}`|❌|

Como `Historico[i].Motivo` só existe no objeto local retornado na mesma chamada, ele **não sobrevive a um rebuild/replay** — nem pelo caminho em memória (`internal/finance/financeiro.go:applyLedgerProjection`), nem pelo caminho oficial (`internal/projections/financeiro_projection.go`). Isso contraria explicitamente o requisito da tarefa 15 (seção 7.1): _"toda alteração financeira relevante deve ter evento, autor, contexto, **motivo quando aplicável** e timestamp"_ — o motivo não é recuperável do ledger, só da memória do processo que fez a chamada.

**Correção sugerida:** incluir `"motivo": motivo` no `map[string]any` do payload em cada uma dessas chamadas de `s.record(...)`, e ajustar `credentialPayload`/`chargePayload` (ou os literais inline) para sempre carregar o campo quando aplicável.

---

### 4. 🟡 MÉDIO-ALTO — Cofre de segredos (`financeiro_segredos_appypay`) existe na migration mas nunca é usado

A migration `098_financeiro_event_sourcing.sql` cria `financeiro_segredos_appypay` como o "armazenamento operacional de segredos cifrados" recomendado pela tarefa 16 (seção 4.3, "Opção recomendada"). Na prática:

- `CredencialAppyPay` e `Application` não têm tags `json:"-"` em `ClientSecretEncrypted`, `WebhookSecretEncrypted`, `APIKeyEncrypted`;
- `projectCredencial` faz `json.Marshal(c)` do objeto **completo** (com o ciphertext incluído) e grava direto em `financeiro_credenciais_appypay.payload`;
- `financeiro_segredos_appypay` fica **órfã**, sem nenhum `INSERT`/`SELECT` em todo o código.

Isso contradiz diretamente a própria `Documentação.md`, que afirma: _"financeiro_credenciais_appypay: [...] segredos/ciphertexts pertencem ao armazenamento operacional controlado"_ — na implementação real, o ciphertext está **dentro** da tabela de metadados, não separado dela. A API continua mascarando corretamente na resposta HTTP (não há vazamento via endpoint), mas a separação de responsabilidade (permitir uma role de leitura menos privilegiada na projeção pública sem acesso a ciphertext) descrita no desenho de segurança não existe de fato.

**Correção sugerida:** mover a escrita/leitura do ciphertext para `financeiro_segredos_appypay` (com `credential_id`, `secret_type`, `application_id`, `ciphertext`, `key_id`) e manter em `financeiro_credenciais_appypay` só as máscaras.

---

### 5. 🟡 MÉDIO — Não existe função de `decrypt`, nem cliente HTTP real da AppyPay

`encrypt()` existe e é usada em toda parte, mas **não há `decrypt()` em lugar nenhum** do módulo. Isso significa que, mesmo com um `Provider` real implementado no futuro, hoje **não há como recuperar o `client_secret` em claro** para autenticar de fato contra `{auth_base_url}/token`. O único `Provider` existente é `FakeProvider`, que nunca faz chamada HTTP real, nunca monta o `paymentMethod` no formato `{IDENTIFICADOR}_{apiKey}` exigido pela AppyPay, e sempre responde sucesso.

Isso é consistente com o escopo da tarefa 15 (que pede só as _funções base_, não a integração final), mas a seção "Mapeamento AppyPay" da `Documentação.md` dá a entender um nível de prontidão maior do que existe ("A base prepara o uso de OAuth2..."). Vale deixar isso explícito: **hoje o módulo é 100% funcional como esqueleto de domínio/auditoria, mas não fala com a AppyPay de verdade**, e falta o `decrypt()` como pré-requisito bloqueante para quando isso for implementado.

---

### 6. 🟡 MÉDIO — Reembolso, reversão e reconciliação são só o "pedido inicial"

Os eventos `ReembolsoFinanceiroStatusAtualizado`, `ReversaoFinanceiraStatusAtualizado`, `DivergenciaFinanceiraDetectada` e `DivergenciaFinanceiraReconciliada` estão na whitelist (`safe_queries.go`) mas **nunca são emitidos** por nenhuma função. `ReembolsarCobrancaFinanceiraBase`/`ReverterCobrancaFinanceiraBase` só gravam o "solicitado" e não têm função-irmã de conclusão. `ReconciliarFinanceiroBase` grava um único evento `ReconciliacaoFinanceiraExecutada` sem nenhuma lógica de comparação (porque, como no achado 5, não há provider real para comparar contra). Isso é coerente com "não implementar cobrança específica de negócio ainda", mas o _fluxo completo_ de reembolso/reversão descrito na seção 5.5/5.6 da tarefa não está pronto — só a primeira metade.

---

### 7. 🟢 BAIXO-MÉDIO — Validação "estudante pertence à academia" é opcional, não estrutural

Em `GerarCobrancaFinanceiraBase`, a checagem de isolamento depende de `in.Metadata["codigo_academia_estudante"]` ser passado pelo chamador:

```go
if in.PagadorTipo == "estudante" && in.Metadata["codigo_academia_estudante"] != "" && in.Metadata["codigo_academia_estudante"] != in.CodigoAcademia {
    return CobrancaFinanceira{}, errors.New("estudante não pertence à academia")
}
```

Se o `metadata` vier vazio, a validação simplesmente não roda — não há consulta real a `projection_estudantes`. Isso é aceitável como função "base" (o handler de negócio futuro deveria fazer a consulta e popular o metadata), mas contraria a leitura literal do critério de aceite #8 da tarefa 15 ("tentativa de cobrar estudante não vinculado à academia é rejeitada"), que hoje só é garantido se o chamador cooperar.

---

## O que está correto (validado)

- Contextos `spuri`/`academia` separados, com `activeCred` filtrando corretamente por `codigo_academia` para o contexto academia.
- `CriarCredencial`, `AtualizarCredencial`, `AlterarStatusCredencial`, `AlterarModalidade`, `CriarCredencial` têm RBAC corretamente estruturado (negar por padrão).
- Criptografia AES-GCM com nonce aleatório, mascaramento (`****cret`) nas respostas.
- `APPYPAY_API_BASE_URL` vem sempre do ambiente, nunca do payload.
- Moeda sempre normalizada para `AOA` (correção já aplicada no debug anterior).
- Idempotência de cobrança por `contexto:academia:referencia_externa` funcionando e testada.
- Webhook duplicado corretamente ignorado por `event_id`; liquidação só ocorre após `SincronizarStatusCobrancaFinanceiraBase`, nunca diretamente pelo payload do webhook.
- `autor_id` obrigatório em todas as funções que geram evento (`validarAutorID`), com teste dedicado.
- Nenhum endpoint HTTP transacional exposto — rotas em `main.go` são só de configuração/controle, conforme escopo.
- Ledger append-only reforçado (migrations 029/091) segue protegendo `spuri_ledger` mesmo para eventos financeiros.
- `Documentação.md` cobre entidades, endpoints e funções internas do módulo de forma geral (com a ressalva do achado 4).

---

## Checklist da tarefa 15 revisado

- [x] Contextos `spuri`/`academia` separados
- [x] Credenciais cadastráveis, testáveis, ativáveis/desativáveis, com segredos criptografados
- [ ] ⚠️ Segredos armazenados no cofre dedicado (estão embutidos na projeção pública)
- [x] Modalidade global/específica/spuri controláveis só por FPP/ADMIN
- [x] Academia só cobra com modalidade global+específica+credencial ativas
- [ ] ⚠️ Isolamento estudante↔academia é opcional (depende do chamador)
- [x] Funções base de cobrança, consulta, sincronização, cancelamento implementadas
- [ ] ❌ Reembolso/reversão/reconciliação: só "solicitado", sem conclusão real
- [x] Webhook idempotente e sem liquidação sem confirmação
- [ ] ❌ Histórico financeiro reconstruível por replay com fidelidade (perde `Historico`/`motivo`)
- [x] Testes cobrindo credenciais, isolamento academia×academia, modalidade, idempotência, webhook
- [ ] ❌ Testes cobrindo papel `estudante`, reembolso, reversão e concorrência
- [x] `Documentação.md` atualizada (com imprecisão pontual sobre onde os segredos ficam)

---

## Recomendação de ordem de correção

1. **Corrigir a falha de autorização** (achado 1) — é o único item com exploração direta via rota já publicada.
2. **Unificar o escritor de projeção** (achado 2) e propagar `Historico`/`motivo` a partir de `event.Metadata`/`event.Payload` (achados 2+3 juntos, mesma correção estrutural).
3. **Mover ciphertext para `financeiro_segredos_appypay`** (achado 4) e alinhar `Documentação.md`.
4. Adicionar testes para papel `estudante` contra as rotas financeiras, reembolso, reversão e atualização concorrente de status (fecha os itens 3/12/13/14 da seção 8 da tarefa 15).
5. Deixar explícito em `Documentação.md` que a integração HTTP real com a AppyPay (incluindo `decrypt()` e o `Provider` real) ainda não existe — hoje é só o esqueleto de domínio/auditoria.

## Comandos de validação sugeridos

- `go test ./internal/finance/... -run TestIsolamento -v` (depois de adicionar caso `estudante`)
- `go test ./internal/projections/... -run Financeiro -v` (não existe hoje — recomendo criar)
- Inspeção manual: `SELECT payload->'ClientSecretEncrypted' FROM financeiro_credenciais_appypay;` para confirmar o achado 4 no ambiente real.