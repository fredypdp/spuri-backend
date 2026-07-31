---
modificado: 2026-07-31 15:40
criado: 2026-07-31 15:40
---
# Correções executadas — Verificação das correções AppyPay 1

## Origem

Este documento registra as correções aplicadas a partir da depuração `docs/Debbugs/Depuração — Verificação das correções 'AppyPay 1'.md`.

## Correções aplicadas

### 1. Idempotência de cobrança reservada antes da chamada ao provider

**Problema:** `GerarCobrancaFinanceiraBase` liberava o mutex antes de chamar `Provider.CriarCobranca`, mas só preenchia `s.idem` e `s.charges` após o retorno do provider. Duas requisições concorrentes com a mesma `referencia_externa` podiam atravessar a janela e enviar duas cobranças ao provider.

**Correção:** a cobrança agora é registrada em memória e a chave idempotente é reservada em `s.idem` imediatamente após o evento `CobrancaFinanceiraCriada`, ainda sob `s.mu`, antes de liberar o lock para a chamada externa. Assim, uma segunda requisição concorrente encontra a chave já reservada e retorna a mesma cobrança lógica sem acionar o provider novamente.

**Arquivo alterado:** `internal/finance/financeiro.go`.

### 2. Academia não pode reatribuir contexto/dono ao atualizar credencial

**Problema:** `AtualizarCredencial` autorizava a academia com base na credencial antiga, mas aceitava `contexto_tipo` e `codigo_academia` do corpo da requisição para construir e persistir a nova versão. Uma academia poderia reescrever a própria credencial para outro tenant ou para escopo `spuri`.

**Correção:** quando `autorTipo == "academia"`, a nova versão preserva obrigatoriamente `ContextoTipo` e `CodigoAcademia` da credencial anterior, ignorando qualquer tentativa de alteração desses campos no payload. FPP/ADMIN continuam podendo atualizar estes campos conforme a permissão administrativa já existente.

**Arquivo alterado:** `internal/finance/financeiro.go`.

## Testes adicionados

- `TestGerarCobrancaConcorrenteReservaIdempotenciaAntesDoProvider`: executa duas goroutines com a mesma `referencia_externa` contra um provider lento e valida que ambas recebem o mesmo ID de cobrança e que o provider é chamado apenas uma vez.
- `TestAcademiaNaoReatribuiContextoAoAtualizarCredencial`: valida que uma academia dona da credencial não consegue alterar `codigo_academia`/`contexto_tipo` por meio do payload de atualização.

**Arquivo alterado:** `internal/finance/financeiro_test.go`.

## Validação executada

```bash
go test ./internal/finance/... -race -v
```

Resultado: todos os testes do pacote financeiro passaram com `-race`.
