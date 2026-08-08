---
criado: 2026-08-08
origem: verificação das correcções aplicadas em `docs/Debbugs/Correções executadas — módulo financeiro AppyPay (v2).md`, commit `c6cbded`, sobre `docs/Debbugs/Depuração — Verificação da implementação do módulo financeiro AppyPay (v2).md`.
---

# Depuração — Verificação das correcções (v3) do módulo financeiro AppyPay

## 1. Método

Comparei `git diff 063ceb6..c6cbded` linha a linha contra cada um dos 8 itens do documento anterior (`Depuração ... (v2).md`), sem confiar apenas no auto-relatório em `Correções executadas ... (v2).md`. Corri `gofmt -l` nos ficheiros alterados (0 erros de sintaxe/parsing — é o único nível de verificação estática que consigo fazer neste sandbox, ver secção 3). Confirmei que os novos identificadores (`finance.ErrNotFound`, `utils.RespondWithNotFoundError`, etc.) existem de facto no repositório com a assinatura usada.

## 2. Resultado por item

| # | Item (v2) | Estado | Nota |
|---|---|---|---|
| 5.1 | Idempotência de `CreateCharge` | ✅ Corrigido, mas ⚠️ **ficou incompleto** — ver secção 3 | Reserva atómica (`financeiro_cobrancas_reservas`) antes de qualquer evento; retentativa devolve o resultado original; libera a reserva se o evento inicial falhar. Isolamento entre contextos também tratado (`existingChargeResult` recusa devolver o resultado de outra academia). **Mas `CreateGPOQRCode` não recebeu a mesma correcção** — ver abaixo. |
| 5.2 | Documentação errada (`resource`, `qrCodeType`, tabela de erros) | ✅ Corrigido | `resource` agora é o UUID correcto; `qrCodeType` usa `SINGLE`; a tabela de erros já reflecte `400/404/409/503/500` reais; até corrigiram um erro que eu não tinha apontado (o exemplo de REF tinha `paymentInfo.phoneNumber`, que não pertence a REF). |
| 5.3 | `paymentMethod` sensível a maiúsculas/minúsculas | ✅ Corrigido | `strings.EqualFold` nos dois lados da comparação; teste novo cobre o caso. |
| 5.4 | `ConsultCharge` grava evento em toda consulta | ✅ Corrigido | Só grava quando `status`, `provider_charge_id` ou `response` (comparado via `sameJSON`) mudarem. |
| 5.5 | `FINANCE_ENCRYPTION_KEY` só validada no primeiro uso | ✅ Corrigido | `finance.ValidateEncryptionConfig()` chamada em `initDB()`; falha o arranque cedo. |
| 5.6 | Sem comprimento mínimo da chave | ✅ Corrigido | Exige Base64 de 32 bytes ou string de ≥ 32 caracteres; teste novo cobre isso. |
| 5.7 | Cobertura de testes fina | 🟡 Parcial | Dois testes novos (robustez da chave, comparação case-insensitive). **Continuam sem teste** os três itens mais importantes que eu tinha pedido: idempotência de webhook ao reprocessar o mesmo `event_id`, isolamento entre academias, e rejeição de admin `gerente`/`adm` nas rotas financeiras. O próprio relatório deles admite isto e diz que precisam de uma suíte de integração com Postgres — correcto, mas continua em aberto. |
| 5.8 | `appypay.go` num único ficheiro de 700+ linhas | ⏸️ Não feito | Esperado — item de baixa prioridade, não bloqueante, fica para depois. |

## 3. 🟠 Problema novo encontrado: a correcção de idempotência não cobre `CreateGPOQRCode`

O item 5.1 só foi corrigido em `Service.CreateCharge`. `Service.CreateGPOQRCode` (`internal/finance/appypay.go`, geração de QR Code GPO) **continua exactamente com o problema original**: chama `s.callJSON(...)` (pedido real à AppyPay) **antes** de qualquer verificação de reserva/idempotência, e só tenta gravar o evento (`QRCodeAppyPayGerado`) depois. Como os QR Codes são gravados na mesma tabela `financeiro_cobrancas` com a mesma constraint `UNIQUE` em `merchant_transaction_id`, uma retentativa com o mesmo `merchantTransactionId`:

1. Chama a AppyPay pela segunda vez de verdade (nada impede, ao contrário do que agora acontece em `CreateCharge`).
2. Só falha depois, na escrita da projecção — deixando outro evento órfão no ledger, exactamente o problema que a correcção do item 5.1 resolveu para cobranças normais mas não para QR Code.

A própria documentação (secção 19 do `Documentação da API.md`) agora promete idempotência por `merchantTransactionId` de forma genérica ("O mesmo `merchantTransactionId` devolve o resultado já persistido...") sem distinguir `/cobrancas` de `/qr-codes` — ou seja, o comportamento documentado hoje **não é verdade** para o endpoint de QR Code.

**Correcção recomendada:** aplicar o mesmo padrão (`reserveCharge`/`existingChargeResult`/`releaseChargeReservation`, ou uma variante para `QRCodeResult`) em `CreateGPOQRCode`, antes de chamar `s.callJSON(ctx, cred, http.MethodPost, "/qr-codes", ...)`.

## 4. ⚠️ Ainda não confirmado: build/testes completos do repositório

Nem eu neste ambiente (sandbox sem acesso ao toolchain `go1.24.12` exigido pelo `go.mod`) nem quem aplicou as correcções (o relatório deles diz literalmente que `go build ./...; go vet ./...; go test ./...` "excedeu o limite de 60 segundos do ambiente sem emitir diagnóstico") confirmámos que o repositório **inteiro** compila e todos os testes (não só os de `internal/finance`) continuam a passar depois destas alterações. Só foi confirmado `go test ./internal/finance` e `go vet ./internal/finance` isoladamente. `gofmt -l` nos ficheiros alterados não apanhou nenhum erro de sintaxe, o que é um bom sinal mas não substitui uma compilação completa (não apanha erros de tipos, imports não usados/em falta fora do que já verifiquei manualmente, nem falhas de integração com o resto do sistema).

**Isto tem de ser corrido até ao fim, num ambiente com o toolchain certo e tempo suficiente, antes de qualquer deploy para produção:**

```
go build ./...
go vet ./...
go test ./...
```

## 5. Veredicto: ainda não está pronto para produção

Muito perto — a maioria das correcções (5/8 itens) está bem resolvida, com verificação directa no código (não só no relatório de quem corrigiu). Mas falta:

1. **Bloqueante:** aplicar a mesma correcção de idempotência (5.1) a `CreateGPOQRCode` (secção 3 deste documento) — hoje há uma inconsistência real entre o que a API promete (idempotência genérica) e o que o código faz (só cobranças normais são idempotentes, QR Code não).
2. **Bloqueante:** confirmar `go build ./... && go vet ./... && go test ./...` até ao fim, sem timeout, num ambiente com o toolchain correcto — ninguém confirmou isto ainda para o repositório completo.
3. **Fortemente recomendado antes de produção** (não necessariamente bloqueante para um primeiro ambiente de testes/staging): os três testes de integração em falta do item 5.7 (idempotência de webhook, isolamento entre academias, RBAC negativo `gerente`/`adm`) — o próprio autor da correcção reconhece que faltam.
4. Opcional, sem urgência: dividir `appypay.go` por sub-domínio (5.8).

## 6. Checklist para o codex

- [ ] Aplicar a `CreateGPOQRCode` a mesma reserva de idempotência já usada em `CreateCharge` (secção 3).
- [ ] Correr `go build ./... && go vet ./... && go test ./...` até ao fim (não só `./internal/finance`) e resolver o que aparecer.
- [ ] Adicionar os três testes de integração em falta (webhook idempotente ao reprocessar `event_id`, isolamento entre academias, rejeição de `gerente`/`adm`) — pode precisar de um Postgres de teste no CI, conforme já apontado pelo próprio autor da correcção anterior.
- [ ] Só depois destes três itens, considerar o módulo pronto para produção.
