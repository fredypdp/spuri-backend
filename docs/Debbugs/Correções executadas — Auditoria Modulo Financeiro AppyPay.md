---
modificado: 2026-07-31 18:05
criado: 2026-07-31 18:05
origem: Auditoria e Plano de Melhoria — Modulo Financeiro AppyPay
---

# Correções executadas — Auditoria do Módulo Financeiro AppyPay

## Resumo

Foram corrigidos os bloqueios de segurança e consistência apontados na auditoria do módulo financeiro, com foco nas rotas e nas proteções internas do serviço:

- Operações sensíveis de credenciais e kill-switch financeiro agora exigem role `fpp` no roteamento HTTP.
- O handler financeiro passa a propagar o `admin_role` real para o serviço, em vez de enviar apenas `user_type=admin`.
- A lógica de negócio do serviço deixou de aceitar qualquer `admin` para criação, ativação/desativação e alteração de modalidade; essas ações agora exigem explicitamente `fpp`.
- As respostas de erro do handler financeiro foram migradas para o envelope padrão da API (`error`, `message`, `request_id`, `details`).
- A sanitização de payloads gravados no ledger passou a ser recursiva, cobrindo mapas aninhados e metadata de cobranças.
- O `SQLLedger` passou a validar `aggregate_type` e `event_type` pela whitelist central antes de inserir eventos no ledger.
- A cobrança enviada ao provider agora atualiza o estado em memória antes da tentativa de gravação do segundo evento no ledger, evitando que uma falha de auditoria deixe a cobrança presa em `pendente`.
- Foi removido um lock duplicado em `TestarCredencial`, prevenindo deadlock ao registrar validações de credencial.

## Itens tratados da auditoria

### RBAC financeiro crítico

A correção foi aplicada em duas camadas:

1. **Roteamento HTTP:** criação de credencial, ativação, desativação e alteração de modalidade foram protegidas por `RequireFPP()`.
2. **Serviço:** `CriarCredencial`, `AlterarStatusCredencial` e `AlterarModalidade` agora rejeitam qualquer autor que não seja `fpp`.

Leituras continuam disponíveis para admins e academias conforme isolamento existente. Atualização de credencial continua permitindo autoatendimento da academia proprietária, mantendo o contexto original da credencial.

### Envelope de erro padrão

Os erros de `internal/handlers/financeiro_handlers.go` não retornam mais `gin.H{"error": err.Error()}` diretamente. As respostas agora usam `utils.RespondWithError`, preservando o contrato documentado da API e evitando exposição direta de mensagens internas como contrato público.

### Whitelist do ledger no SQLLedger

`SQLLedger.AppendFinanceEvent` agora chama `db.ValidateAggregateType("Financeiro")` e `db.ValidateEventType(event.EventType)` antes do `INSERT` manual. Isso elimina o bypass direto da whitelist central para esse escritor alternativo.

### Sanitização recursiva

`sanitizeMap` passou a chamar `sanitizeValue`, que percorre:

- `map[string]any`
- `map[string]string`
- `[]map[string]any`
- `[]any`

Com isso, chaves sensíveis dentro de metadata aninhada são redigidas antes de entrarem em payloads auditáveis.

### Cobrança enviada ao provider

Depois da resposta do provider, o serviço atualiza `s.charges` e `s.idem` antes de registrar `CobrancaFinanceiraEnviadaAoProvider`. Se a gravação do evento falhar, a função retorna a cobrança com o estado real observado (`enviada_provider` ou `falhada`) junto do erro, em vez de deixar o cache compartilhado preso no estado anterior.

## Pontos que permanecem para evolução futura

Alguns itens estruturais da auditoria exigem mudanças maiores de arquitetura e permanecem recomendados para uma etapa posterior:

- Mover idempotência de cobrança para reserva durável em banco antes da chamada ao provider.
- Deduplicar webhooks exclusivamente via banco em cenários multi-instância.
- Implementar assinatura/HMAC de webhooks contra segredo local da credencial.
- Implementar rotação real de chaves (`key_id`).
- Concluir os fluxos de reembolso, reversão e reconciliação com provider real.
- Avaliar remoção completa de `SQLLedger` e do rebuild legado do `Service` após estabilizar o caminho canônico por `FinanceiroProjection`.

## Verificação executada

- `go test ./internal/finance` passou com sucesso após a atualização dos testes para refletirem a exigência explícita de `fpp` nas operações sensíveis.
