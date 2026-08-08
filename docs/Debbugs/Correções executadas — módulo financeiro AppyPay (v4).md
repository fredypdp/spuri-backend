---
criado: 2026-08-08
origem: docs/Debbugs/Depuração — Verificação das correções (v4) do módulo financeiro AppyPay.md
---

# Correções executadas — módulo financeiro AppyPay (v4)

## Rede de regressão no CI

Foi criada a pipeline `.github/workflows/ci.yml`. Ela instala Go 1.24.12, inicia PostgreSQL 16 descartável e executa `go build ./...`, `go vet ./...` e `go test ./...` com `RUN_POSTGRES_INTEGRATION=1`.

## Testes de integração adicionados

- `TestIntegrationAcceptWebhookIsIdempotent`: entrega o mesmo `event_id` duas vezes e confirma uma única reserva e um único evento no ledger.
- `TestIntegrationFinanceRejectsAcademyChargeOutsideScope`: confirma que uma academia não consulta cobrança de outra academia.
- `TestIntegrationFinanceRejectsNonFPPAdmins`: confirma que os roles `gerente` e `adm` recebem `403` nas rotas financeiras.

Os testes de integração são ignorados fora do CI, salvo quando `RUN_POSTGRES_INTEGRATION=1` e uma `DATABASE_URL` de teste forem fornecidas. Eles executam migrations antes dos cenários e usam dados com UUIDs exclusivos.
