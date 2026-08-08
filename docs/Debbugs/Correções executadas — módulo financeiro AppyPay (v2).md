---
criado: 2026-08-08
origem: docs/Debbugs/Depuração — Verificação da implementação do módulo financeiro AppyPay (v2).md
---

# Correções executadas — módulo financeiro AppyPay (v2)

## Itens resolvidos

- **5.1 — Idempotência de cobranças:** a migration `102_financeiro_cobrancas_idempotencia.sql` cria uma reserva atómica por `merchantTransactionId`. `CreateCharge` reserva a chave antes de gravar qualquer evento; retentativas devolvem o resultado persistido e requisições simultâneas, ainda sem resultado consultável, retornam conflito. A reserva é removida se não for possível persistir o evento inicial.
- **5.2 — Documentação:** `resource` agora é apresentado como UUID, o exemplo de QR usa `SINGLE`, e os exemplos de `merchantTransactionId` obedecem à validação alfanumérica de no máximo 15 caracteres. A tabela de erros foi alinhada a `400`, `404`, `409`, `503` e `500` reais.
- **5.3 — Método de pagamento:** identificadores GPO/REF configurados são comparados com `strings.EqualFold`.
- **5.4 — Consultas:** uma consulta só grava `CobrancaAppyPayConsultada` quando status, identificador do provider ou resposta sanitizada tiverem mudado.
- **5.5/5.6 — Cifra:** o arranque chama `finance.ValidateEncryptionConfig`; a chave deve ser Base64 de 32 bytes ou possuir pelo menos 32 caracteres.
- **5.7 (cobertura unitária possível sem Postgres):** foram adicionados testes para robustez da chave e comparação case-insensitive dos métodos. A idempotência de webhook e isolamento de academias permanecem cobertos pela implementação, mas requerem uma suíte de integração com Postgres para teste automatizado fim a fim.

## Verificação

- `go test ./internal/finance -count=1`: passou.
- `go vet ./internal/finance`: passou.
- `go build ./...; go vet ./...; go test ./...` foi iniciado antes das alterações, mas excedeu o limite de 60 segundos do ambiente sem emitir diagnóstico. A validação completa e os testes de integração com Postgres devem ser executados em CI ou num ambiente com a base disponível.
