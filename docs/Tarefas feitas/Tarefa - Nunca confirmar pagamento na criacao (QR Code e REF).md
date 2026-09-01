---
criado: 2026-09-01
origem: Investigação solicitada por Fredy — "o que garante que REF/QR code também funcionam, se o teste real só cobriu GPO com telefone?"
status: feito
---

# Tarefa — Nunca confirmar pagamento (matrícula/mensalidade) a partir da resposta síncrona de CRIAÇÃO de cobrança/QR Code

## Prompt recomendado para o Codex

> Substitua `internal/finance/appypay.go` e `internal/finance/appypay_integration_test.go`
> pelos arquivos completos anexados a esta tarefa (já corrigidos e validados
> — não é preciso escrever nenhum código novo, só aplicar os arquivos).
> Depois, rode `gofmt -l .`, `go vet ./...`, `go build ./...` e `go test
> ./...` (incluindo os testes de integração com `RUN_POSTGRES_INTEGRATION=1`
> contra um PostgreSQL real) e reporte os resultados. Confirme com `git diff
> --stat` que só esses 2 arquivos mudaram. Siga o "Procedimento de
> conclusão" no fim deste documento.

## Contexto

Numa auditoria de conformidade AppyPay, foi identificado e corrigido que
**gerar um QR Code GPO para pagamento de matrícula ou mensalidade efetivava
o pagamento imediatamente** — antes de qualquer pessoa realmente pagar
(detalhes na seção 1). Ao investigar se o mesmo tipo de problema afeta REF
(pagamento por referência/multibanco), foi identificado um risco
estruturalmente idêntico, tratado preventivamente (seção 2) mesmo sem
confirmação empírica de que a AppyPay hoje devolve esse cenário — o custo de
bloquear é zero e o custo de não bloquear, se acontecer, é o mesmo das duas
seções: matrícula ou mensalidade dada como paga sem dinheiro nenhum ter
mudado de mãos.

`GPO_QR` e `REF` são métodos de pagamento validados e suportados tanto para
matrícula (`internal/finance/matricula.go:167`) quanto para mensalidade
(`internal/finance/mensalidade.go:454`) — não são casos hipotéticos, são
opções de configuração reais que qualquer academia pode ativar.

## 1. QR Code (`CreateGPOQRCode`) — causa raiz confirmada empiricamente

A resposta real e documentada da AppyPay para "Post a GPO QR Code"
(`docs/Parceiros e integrações/AppyPay Documentação.md`) é:

```json
{
  "id": "50625c7f-894b-410b-9ee8-6ef958a534d9",
  "qrCodeArr": "iVBORw0KGgo...",
  "responseStatus": {
    "successful": true, "status": "Active", "code": 103,
    "message": "QR code successfully created.", "source": "GPO"
  }
}
```

`"status": "Active"` já deixa claro que o QR foi **criado**, não **pago**.
Mas `appyPayCodeOutcomes[103]` mapeava o código 103 para `{"Success", ""}`
— o bracket que a própria documentação da AppyPay usa para esse código, só
que ali "Success" quer dizer "a operação de criar o QR teve sucesso", não
"o pagamento foi recebido".

**Cadeia do bug:** `CreateGPOQRCode` devolvia `Status: "Success"` →
`isSuccessfulChargeStatus` → `true` → `IniciarPagamentoMatricula`
(`internal/handlers/solicitacao_matricula_handlers.go:461`) chamava
`efetivarVinculoMatriculaPaga` — cria a conta do estudante e grava o
evento **irreversível** `SolicitacaoMatriculaVinculada` no `spuri_ledger`
(append-only, encadeado por hash). O mesmo aconteceria para mensalidade via
`confirmMensalidadeCharge`, chamado de dentro do próprio `CreateGPOQRCode`.

**Validação empírica feita nesta investigação:** reproduzi o fluxo completo
com o servidor HTTP real do Spuri, Postgres real, e a resposta documentada
acima como resposta simulada da AppyPay. Confirmei no `spuri_ledger` real
que os eventos `EstudanteCriadoComVinculo` e `SolicitacaoMatriculaVinculada`
eram gravados permanentemente sem nenhum pagamento ter acontecido — e que,
com a correção, eles deixam de ser gravados (`status: "aguardando_pagamento"`).

## 2. Referência (REF, `CreateCharge`) — risco tratado preventivamente

`docs/.../AppyPay Documentação.md` ("Supported Payment Methods" >
"Validations and Limitations") documenta REF como o **único** método com
**"Webhook: Always required"** — diferente de GPO/UMM/eTPA ("Required for
async request"). Isto significa que, por definição, o pagamento de uma
referência **nunca** acontece na chamada de criação — o cliente paga depois,
num multibanco/balcão, fora da API. A confirmação só pode legitimamente
chegar depois, por webhook ou consulta.

A tabela de códigos da AppyPay lista o código 100 ("Thank you! The payment
has been successfully registered") com "Applies To" incluindo **REF**, ao
lado de UMM/GPO/FTBAI — métodos onde 100 pode sim significar pagamento
concluído na hora. Não há nada na documentação garantindo que a AppyPay
nunca devolve esse código (ou qualquer outro classificado como "Success")
na resposta síncrona de **criação** de uma referência.

**Por precaução, e pela mesma razão da correção da seção 1:** `CreateCharge`
agora nunca aceita uma classificação "Success" vinda da chamada de criação
quando o método é REF, custe o que custar — o pior cenário de aceitar por
engano é idêntico ao do QR code, e o custo de bloquear é zero, porque REF
genuíno sempre confirma depois mesmo assim (por webhook, que já está
implementado e com double-check antes de qualquer efeito irreversível —
ver `AcceptWebhook`, tarefa 79). Isto **não** foi confirmado com uma
chamada real à AppyPay retornando esse cenário (diferente da seção 1, que
foi confirmada via reprodução exata da resposta documentada) — é uma
correção preventiva, bem fundamentada na documentação, não a correção de
um bug observado ao vivo.

## Fora de escopo

- `consultCharge`, `AcceptWebhook` — não tocados; continuam podendo
  classificar REF/GPO como "Success" quando a confirmação chega de verdade
  (por webhook ou consulta), que é o único jeito legítimo para REF.
- UMM, GPO por telefone, FTBAI — não tocados. Já validados em separado (ver
  `docs/Debbugs/Auditoria de conformidade AppyPay (autenticação e geração de
  cobrança).md` e o adendo de 2026-08-31, com chamada real bem-sucedida à
  AppyPay).
- `mensalidade.go`, `matricula.go` — não tocados. As duas correções ficam
  nos pontos únicos e compartilhados (`CreateGPOQRCode`, `CreateCharge`),
  que já protegem os dois casos automaticamente.
- `go.mod`/`go.sum` — não tocados.

## Arquivos alterados

Só 2, ambos anexados **completos e já validados** a esta tarefa — não é
necessário escrever nada, só substituir:

- `internal/finance/appypay.go` — os 2 blocos de correção (QR e REF),
  cada um dentro da função onde o problema existe, comentados no local.
- `internal/finance/appypay_integration_test.go` — os 2 testes de
  regressão permanentes correspondentes:
  `TestIntegrationCreateGPOQRCodeCriacaoNuncaEClassificadaComoPagamentoSucedido`
  e
  `TestIntegrationCreateChargeREFNuncaEClassificadaComoPagamentoSucedidoNaCriacao`.

## Critérios de aceitação

- [ ] `gofmt -l .` sem saída.
- [ ] `go vet ./...` sem erros.
- [ ] `go build ./...` sem erros.
- [ ] `go test ./...` (unitários) passa, todos os pacotes.
- [ ] `go test ./...` com `RUN_POSTGRES_INTEGRATION=1` contra PostgreSQL
      real passa, incluindo os 2 novos testes de regressão.
- [ ] `git diff --stat` mostra exatamente 2 arquivos alterados:
      `internal/finance/appypay.go` e
      `internal/finance/appypay_integration_test.go`. Nenhum outro arquivo.

## Procedimento de conclusão

1. Substituir os 2 arquivos pelos anexados.
2. Rodar as validações da seção "Critérios de aceitação" e reportar os
   resultados (colar a saída real de `go test ./...`, não só "passou").
3. Mover este arquivo de `docs/Lista de Tarefas/` para
   `docs/Tarefas feitas/`.
