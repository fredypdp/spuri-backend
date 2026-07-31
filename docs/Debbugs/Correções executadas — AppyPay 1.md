---
modificado: 2026-07-31 00:00
criado: 2026-07-31 00:00
---
# Correções executadas — módulo financeiro AppyPay 1

## Escopo

Este documento registra as correções implementadas a partir da depuração `docs/Debbugs/Depuração — Verificação das correções do módulo financeiro AppyPay 1.md`.

## Correções aplicadas

### 1. Chave de criptografia validada pela variável de ambiente correta

- `encrypt()` e `decrypt()` passaram a consultar `ENV=production`, que é a convenção usada pelo restante do backend, em vez de `GO_ENV=production`.
- A validação foi centralizada em `ValidateEncryptionConfig()` para permitir falha antecipada no boot.
- `initDB()` chama `finance.ValidateEncryptionConfig()` antes de conectar e migrar o banco, impedindo que a aplicação inicialize em produção sem `FINANCE_ENCRYPTION_KEY`.
- `buildCredential()` deixou de ignorar erros de criptografia e agora propaga a falha para o chamador.

### 2. Escritor duplo das projeções financeiras removido do fluxo síncrono do Service

- O `Service` deixou de gravar diretamente nas tabelas públicas de projeção `financeiro_credenciais_appypay`, `financeiro_cobrancas` e `financeiro_modalidade_pagamento` durante operações HTTP síncronas.
- A escrita canônica dessas tabelas fica concentrada no `FinanceiroProjection`/`projManager`, que processa o ledger.
- O `Service` continua atualizando seu cache em memória para manter respostas síncronas consistentes no processo atual.
- Segredos cifrados continuam sendo gravados pelo `Service` em `financeiro_segredos_appypay`, porque eles não trafegam no payload público do ledger e não pertencem à projeção pública.

### 3. Mutex global liberado durante chamada externa ao provider

- `GerarCobrancaFinanceiraBase()` agora copia e valida o estado necessário sob lock, registra o evento inicial e libera `s.mu` antes de chamar `Provider.CriarCobranca()`.
- Após a resposta do provider, o método reacquire o lock apenas para registrar o evento final e atualizar o cache de cobranças/idempotência.
- Isso evita que uma chamada lenta ao provider bloqueie globalmente operações financeiras administrativas, incluindo o kill-switch da modalidade de pagamento.

### 4. `codigo_academia` populado no contexto Gin para usuários academia

- `AuthMiddleware` agora consulta `codigo_academia` junto com `status` para `user_type=academia`.
- Quando encontrado, o valor é salvo em `c.Set("codigo_academia", ...)`, destravando handlers financeiros que dependem de `c.GetString("codigo_academia")` para autorizar autoatendimento de academia.

## Validação executada

- `go test ./internal/finance/... ./internal/middleware/... ./cmd/server/...`
