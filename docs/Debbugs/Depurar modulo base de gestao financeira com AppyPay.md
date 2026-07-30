---
modificado: 2026-07-30 00:00
criado: 2026-07-30 00:00
---
# Depurar módulo base de gestão financeira com AppyPay

Tarefa: [[15 - Modulo base de gestao financeira com AppyPay]]

## Objetivo da auditoria

Auditar a implementação documentada em `docs/Tarefas feitas/15 - Modulo base de gestao financeira com AppyPay.md`, seguindo o padrão de depuração das demais tarefas concluídas, para confirmar se o módulo financeiro base com AppyPay foi implementado corretamente e completar ajustes faltantes.

## Resultado da depuração

A auditoria confirmou que o backend já possui:

- módulo `internal/finance` com contextos `spuri` e `academia`;
- cadastro, atualização, listagem, consulta, teste, ativação e desativação de credenciais AppyPay;
- criptografia em repouso de `client_secret`, `apiKey` e segredo de webhook, com mascaramento nas respostas;
- controlo de modalidade global, por academia e separado para o contexto Spuri;
- funções internas genéricas para cobrança, consulta, sincronização de status, cancelamento, reembolso, reversão, webhook e reconciliação;
- rotas HTTP apenas para configuração/controlo, sem endpoints transacionais públicos de cobrança;
- documentação em `Documentação.md` para entidades, endpoints, funções internas, permissões, idempotência e mapeamento AppyPay.

## Correção aplicada

Foi identificado um ponto divergente da regra operacional extra: embora a documentação dissesse que `AOA` era a moeda padrão, a função interna de cobrança preservava moedas diferentes quando o chamador informava outro valor. Como a moeda padrão é `AOA` e nunca deve mudar, a geração de cobranças foi ajustada para normalizar sempre `Moeda` para `AOA`, independentemente do input.

Também foi adicionado teste automatizado garantindo que uma cobrança solicitada com moeda diferente continua registrada como `AOA`.

## Checklist validado

- [x] Credenciais sensíveis não aparecem em claro nas respostas.
- [x] Segredos são armazenados cifrados no serviço financeiro.
- [x] Academia não consegue consultar credenciais de outra academia.
- [x] Idempotência por referência externa evita duplicidade de cobrança.
- [x] Modalidade global desativada bloqueia cobranças próprias das academias.
- [x] Contexto Spuri permanece independente da modalidade global das academias.
- [x] Webhook duplicado é ignorado.
- [x] Liquidação por webhook depende de sincronização/consulta ao provider.
- [x] Moeda financeira é sempre `AOA` e não muda por payload interno.
- [x] Todos os eventos emitidos pelo módulo financeiro carregam `AutorID`, e operações eventadas rejeitam autor vazio.
- [x] Configurações, credenciais, cobranças e webhooks do módulo financeiro são persistidos em PostgreSQL por tabelas próprias, com cache em memória apenas em runtime.

## Comandos de validação

- `go test ./internal/finance`
- `go test ./...`
