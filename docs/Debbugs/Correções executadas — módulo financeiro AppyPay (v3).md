---
criado: 2026-08-08
origem: docs/Debbugs/Depuração — Verificação das correções (v3) do módulo financeiro AppyPay.md
---

# Correções executadas — módulo financeiro AppyPay (v3)

## Correção bloqueante

`CreateGPOQRCode` agora segue o mesmo protocolo de idempotência de `CreateCharge`:

1. reserva `merchantTransactionId` antes de chamar a AppyPay;
2. grava `QRCodeAppyPaySolicitado`, permitindo que uma retentativa recupere o resultado persistido;
3. grava `QRCodeAppyPayGerado` ou `QRCodeAppyPayFalhou` conforme a resposta do gateway;
4. libera a reserva apenas se a gravação do evento inicial falhar;
5. devolve `409` para uma requisição concorrente cujo resultado ainda não esteja disponível, sem expor dados de outro contexto.

Os eventos novos foram acrescentados à whitelist do ledger e à projeção financeira. A documentação agora explicita que a idempotência também vale para QR Codes e corrige os identificadores de resposta que ainda não obedeciam à regra alfanumérica de no máximo 15 caracteres.

## Verificação

- `go build ./...`, `go vet ./...` e `go test ./...` foram concluídos com sucesso neste ambiente.
- `go test ./internal/finance -count=1` e `go vet ./internal/finance` também passaram após a alteração.
- Os testes de integração para webhook idempotente, isolamento entre academias e RBAC negativo continuam dependendo de uma base Postgres de teste; este ambiente não possui uma configuração de banco de teste disponível.
