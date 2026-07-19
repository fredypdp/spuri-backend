---
modificado: 2026-07-19 00:00
criado: 2026-07-19 00:00
---
# Depurar padronização de erros no cadastro de academia em `/dominis/academia/register`

## Objetivo

Garantir que a rota administrativa `POST /dominis/academia/register` não regrida para o formato legado de erro e continue usando o envelope padronizado adotado pelo backend:

```json
{
  "error": "VALIDATION_ERROR | UNAUTHORIZED | FORBIDDEN | NOT_FOUND | CONFLICT | RATE_LIMIT | INTERNAL_ERROR | ERROR",
  "message": "mensagem de erro para o cliente",
  "request_id": "uuid",
  "details": []
}
```

## Debug executado

- A rota foi verificada no roteador principal para confirmar que permanece registrada dentro do grupo `/dominis` e protegida por autenticação/admin/FPP.
- Os fluxos iniciais de falha foram cobertos com testes de regressão:
  - requisição sem `Authorization`, validando `UNAUTHORIZED`, `message` e `request_id`;
  - corpo JSON inválido no binder do cadastro de academia, validando `VALIDATION_ERROR`, `message` e `request_id`;
  - multipart inválido no binder do cadastro de academia, validando `VALIDATION_ERROR`, `message` e `request_id`.

## Resultado

O debug adicionou testes automatizados para garantir que os erros da rota e do parser específico do cadastro de academia sigam o padrão global de `utils.RespondWithError`/`utils.RespondWithValidationError`, em vez de respostas legadas contendo apenas `error` textual.

## Observação de execução

A execução de `go test ./cmd/server ./internal/handlers` foi bloqueada antes dos testes por uma dependência já ausente no módulo (`github.com/t3rm1n4l/go-mega` importada por `internal/storage/storage.go`). A falha não veio dos novos testes, mas da resolução de dependências do pacote.
