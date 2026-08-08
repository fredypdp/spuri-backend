---
criado: 2026-08-08
origem: verificação das correcções aplicadas em `docs/Debbugs/Correções executadas — módulo financeiro AppyPay (v3).md`, commit `78ea0b1`, sobre `docs/Debbugs/Depuração — Verificação das correções (v3) do módulo financeiro AppyPay.md`.
---

# Depuração — Verificação das correcções (v4) do módulo financeiro AppyPay

## 1. Método

`git diff c6cbded..78ea0b1` linha a linha, focado nos 3 itens bloqueantes/recomendados do documento v3. Confirmei os novos identificadores no código (não só no relatório de quem corrigiu): `qrCodePayload`, `existingQRCodeResult`, `qrCodeResultFromRow`, as entradas novas em `validEventTypes` e no `switch` da `FinanceiroProjection`. `gofmt -l` nos ficheiros alterados novamente sem erros. Reconfirmei que `AcceptWebhook` (idempotência de webhook) e `authorizeFinanceScope`/`financeAdminAllowed` (isolamento entre academias e RBAC `fpp`) não foram tocados desde as rondas anteriores — continuam como verifiquei e confirmei correctos nos documentos v2/v3.

## 2. Resultado por item do documento v3

| # | Item | Estado |
|---|---|---|
| 1 | Idempotência em `CreateGPOQRCode` (o problema novo encontrado na v3) | ✅ **Corrigido.** Mesmo protocolo de `CreateCharge`: reserva (`financeiro_cobrancas_reservas`) antes de qualquer evento → grava `QRCodeAppyPaySolicitado` → chama a AppyPay → grava `QRCodeAppyPayGerado`/`QRCodeAppyPayFalhou` → retentativa devolve o QR já persistido; requisição concorrente ainda sem resultado devolve `409`; isolamento entre academias tratado em `qrCodeResultFromRow` (recusa devolver QR de outro contexto, `ErrConflict`); protegido também contra colisão de `merchantTransactionId` entre uma cobrança normal e um QR Code (o `qr_code_type` no payload é usado para distinguir). Eventos novos correctamente na whitelist do ledger (`internal/db/safe_queries.go`) e na projecção (`internal/projections/financeiro_projection.go`). Documentação da secção 19 actualizada para deixar isto explícito, e os exemplos de `merchantTransactionId` agora respeitam mesmo o limite alfanumérico de 15 caracteres. |
| 2 | `go build ./... && go vet ./... && go test ./...` completo, sem timeout | ✅ Reportado como concluído com sucesso pelo autor da correcção. **Não consegui confirmar de forma 100% independente neste sandbox** (mesma limitação de toolchain das rondas anteriores) — mas não há nada no diff que sugira quebra (imports coerentes, `gofmt -l` limpo, assinaturas de funções batem com o resto do código). |
| 3 | Testes de integração (webhook idempotente, isolamento entre academias, RBAC negativo `gerente`/`adm`) | 🟡 **Ainda não adicionados**, admitido explicitamente pelo autor da correcção ("este ambiente não possui uma configuração de banco de teste disponível"). Foi acrescentado, em troca, um teste unitário puro para a nova lógica de QR Code (`TestQRCodeIdempotencyPayloadAndPersistedResult`), que cobre o isolamento entre academias e a whitelist de eventos ao nível de função, mas não substitui um teste de integração ponta-a-ponta contra os handlers HTTP e uma base de dados real. |

## 3. Veredicto: o módulo já pode ir para produção, com uma ressalva

Todos os problemas que envolviam risco real de dinheiro/duplicação de cobrança, RBAC ou exposição de segredos entre academias foram encontrados, corrigidos e **verificados directamente no código** ao longo de 3 rondas (não apenas no relatório de quem implementou):

- Idempotência real (reserva-antes-de-chamar-a-AppyPay) para cobranças **e** QR Codes, com devolução do resultado original em vez de erro genérico.
- Isolamento entre academias e entre Spuri/academia em todas as operações (credenciais, cobranças, QR Codes, consulta).
- RBAC restrito a `fpp` (nenhum admin `gerente`/`adm` toca no módulo).
- Nenhum segredo (`client_secret`, tokens, API key de webhook) em claro em eventos, projecções ou respostas.
- Webhook idempotente por `event_id`, com reserva atómica em base de dados.
- Cifra de credenciais validada no arranque, com comprimento mínimo de chave.
- Documentação (secção 19 da `Documentação da API.md`) agora bate com o comportamento real do código, incluindo os códigos HTTP (`400/404/409/503/500`).

O que falta é **rede de segurança para regressões futuras**, não uma falha activa hoje: os três testes de integração (webhook reprocessado, isolamento entre academias via handlers HTTP, RBAC negativo) continuam por escrever, por falta de uma base Postgres de teste disponível no ambiente de quem corrigiu. As propriedades que esses testes verificariam **já foram confirmadas manualmente por mim, no código, em três rondas diferentes** — não é um risco desconhecido, é um risco de "alguém editar este código daqui a 3 meses e quebrar isto sem ninguém notar".

**Recomendação:** pode avançar para produção. Antes ou logo a seguir ao lançamento, montar uma base Postgres de teste no CI (mesmo que só para este módulo) e escrever esses três testes — não para "aprovar" o que já está correcto, mas para garantir que continua correcto depois da próxima alteração.

## 4. Checklist final (não bloqueante para produção, mas para fechar o ciclo)

- [ ] Confirmar `go build ./... && go vet ./... && go test ./...` de forma independente em CI (ex.: GitHub Actions, que já deve ter o toolchain certo) antes do deploy, como última confirmação formal.
- [ ] Configurar uma base Postgres de teste no CI e adicionar os três testes de integração pendentes (webhook reprocessado, isolamento entre academias, RBAC negativo `gerente`/`adm`).
- [ ] (Sem urgência, item 5.8 desde a v2) Dividir `internal/finance/appypay.go` por sub-domínio quando houver disponibilidade.
