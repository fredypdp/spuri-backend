---
criado: 2026-07-24 00:00
modificado: 2026-07-24 00:00
---
# Depurar separação de edição de email e telefone em rotas dedicadas

Tarefa auditada: `docs/Tarefas feitas/12 - Separar edicao de email e telefone em rotas dedicadas.md`.

## Objetivo da auditoria

Confirmar se a implementação feita para separar a edição de `email` e `telefone` em rotas próprias estava efetiva no código, sem aliases de compatibilidade nas rotas genéricas, e completar lacunas objetivas de cobertura automatizada caso fossem encontradas.

## Verificações executadas

```bash
rg "me/email|me/telefone|dados-pessoais|academia/dados|admin/.*/dados|telefone_verificado|email_verificado|telefone" -n internal cmd
rg "rejectDedicatedContactFields|AtualizarMeuEmail|AtualizarMeuTelefone|Contact" -n internal/handlers cmd/server/main_test.go
rg "AtualizarMeuEmail|/me/email|telefone deve conter|campo_nao_permitido|dedicad" -n internal cmd Documentação.md docs/Debbugs
```

## Resultado da auditoria

A implementação principal já estava presente e aderente aos critérios funcionais da tarefa:

| Critério auditado | Resultado |
| --- | --- |
| Rotas dedicadas `PUT /me/email` e `PUT /me/telefone` | Registradas no grupo autenticado e apontando para handlers específicos. |
| Identificação por token | Os handlers usam `middleware.GetUserID` e `middleware.GetUserType`, sem aceitar `tipo_usuario`, `role`, `academia_id`, `estudante_id` ou `admin_id` no payload para escolher o alvo. |
| Cobertura de entidades | O handler central carrega os aggregates `Estudante`, `Academia` ou `Admin` conforme o tipo autenticado. |
| Validação de email | Usa o validador de email existente antes da persistência. |
| Validação estrita de telefone | Usa `ValidatePhoneStrictNational`, que exige exatamente 9 dígitos nacionais e rejeita DDI/formatação antes de qualquer normalização permissiva. |
| Rotas genéricas | `PUT /academia/dados`, `PUT /estudante/dados-pessoais` e `PUT /dominis/admin/:id/dados` chamam `rejectDedicatedContactFields` antes de fazer bind ou carregar aggregate. |
| Sem mutação parcial | A rejeição de `email`/`telefone` ocorre antes da execução dos comandos de atualização das rotas genéricas. |
| Reset de flags | Continua delegado aos eventos/aggregates já existentes, que marcam alteração real e as projeções resetam `email_verificado`/`telefone_verificado` somente quando os flags de alteração indicam mudança real. |
| Documentação principal | `Documentação.md` descreve as rotas dedicadas e avisa que rotas genéricas rejeitam `email`/`telefone`. |

## Lacuna encontrada

Não foi encontrada falha funcional que exigisse alterar a regra de negócio. A lacuna objetiva estava na cobertura de debug automatizada: não havia testes unitários dedicados para garantir que:

1. as rotas genéricas rejeitam explicitamente `email` e orientam `PUT /me/email`;
2. as rotas genéricas rejeitam explicitamente `telefone` e orientam `PUT /me/telefone`;
3. a validação estrita de telefone rejeita os exemplos obrigatórios com DDI, espaços, hífens, parênteses, letras e valor vazio.

## Correções aplicadas

1. Adicionado `internal/handlers/contact_handlers_test.go` cobrindo a rejeição explícita e atômica de campos de contato nas rotas genéricas.
2. Adicionado `internal/utils/phone_strict_test.go` cobrindo a regra de telefone nacional de 9 dígitos e os formatos proibidos listados na tarefa.

## Validação final

```bash
go test ./...
```

A suíte completa passou após a inclusão dos testes de debug, confirmando que a atualização documentada permanece implementada corretamente e agora tem cobertura automatizada para os pontos auditados.
