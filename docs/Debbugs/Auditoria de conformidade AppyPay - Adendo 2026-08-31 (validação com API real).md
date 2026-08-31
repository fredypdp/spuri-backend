---
criado: 2026-08-31
origem: Adendo à "Auditoria de conformidade AppyPay (autenticação e geração de cobrança).md"
status: concluído — nenhuma correção de código necessária
---

# Adendo — Validação com chamadas reais à API da AppyPay

## Contexto

A auditoria original (`docs/Debbugs/Auditoria de conformidade AppyPay
(autenticação e geração de cobrança).md`) foi feita por leitura de código e
testes com mocks. Este adendo documenta uma segunda rodada, pedida
explicitamente por Fredy, com **chamadas reais** à API da AppyPay
(ambiente de teste, `gwy-api-tst.appypay.co.ao`) usando credenciais reais, e
com o servidor HTTP real do Spuri (Postgres real, todas as camadas —
routing, binding, handler, service).

O sandbox onde a análise foi feita não alcança `login.microsoftonline.com`
nem `gwy-api-tst.appypay.co.ao` (rede restrita a uma allowlist fixa). Por
isso, o "Teste 1" (chamada direta à AppyPay) foi executado por Fredy, fora
do sandbox, com uma ferramenta (`appypay_teste1.ps1`/`.exe`) construída e
validada aqui antes da entrega. O resultado real (`appypay_teste1_resultado.json`,
gerado em `2026-08-31T11:40:06Z`) foi devolvido e analisado nesta sessão.

## Teste 1 — chamada direta à API real da AppyPay

### 1.1 Autenticação — POST

**Resultado: sucesso total.** `POST
https://login.microsoftonline.com/appypaydev.onmicrosoft.com/oauth2/token`
com corpo `application/x-www-form-urlencoded` (`grant_type`, `client_id`,
`client_secret`, `resource`) devolveu HTTP 200 com um Bearer token JWT
válido. O corpo da resposta bate **campo a campo** com o exemplo
documentado em "Get a token": `token_type`, `expires_in` (string, `"3599"`),
`ext_expires_in`, `expires_on`, `not_before`, `resource`, `access_token`.

Isto confirma, com uma chamada real (não um exemplo da documentação), que o
método POST usado por `internal/finance/appypay.go` está correto.

### 1.2 Autenticação — GET (teste de confirmação da divergência já registrada)

**Resultado: o próprio cliente HTTP recusou a chamada antes de sair para a
rede** — erro `"Não é possível enviar um conteúdo com este tipo de
verbo."`. Isto não é uma resposta da AppyPay/Microsoft: é uma restrição do
lado do cliente (o stack HTTP do .NET/Windows recusa enviar corpo com
verbo GET, por design).

**Conclusão definitiva:** o exemplo `curl --request GET` da documentação da
AppyPay não é executável por um cliente HTTP padrão com um corpo
`application/x-www-form-urlencoded`. Isto fecha, com evidência prática (não
apenas teórica), a divergência já registrada na auditoria original — o
código do Spuri usar POST está certo, e continuará assim.

### 1.3 Criação de cobrança GPO real (`POST /v2.0/charges`)

**Resultado: aceite pela AppyPay (HTTP 200).** O corpo enviado:

```json
{
  "amount": 50,
  "currency": "AOA",
  "description": "Teste de auditoria Spuri",
  "merchantTransactionId": "TC7B22FF06535E1",
  "paymentMethod": "GPO_E217DDB6-FC4C-44E9-BC23-DD86930EA943",
  "paymentInfo": { "phoneNumber": "931417623" }
}
```

— está exatamente no formato que `CreateCharge` monta (sem `options`/`notify`
vazios, `merchantTransactionId` alfanumérico de 15 caracteres aceite sem
reclamação).

A cobrança em si foi **recusada pela AppyPay/Multicaixa Express**, por
motivo de negócio, não de formato:

```json
{
  "id": "fc3bba31-c767-481c-9eb8-5d8c9c84f018",
  "responseStatus": {
    "successful": false,
    "status": "Failed",
    "code": 200,
    "message": "O seu pagamento foi recusado pelo sistema Multicaixa Express...",
    "source": "GPO",
    "sourceDetails": {
      "type": "EPMS_PROCESSOR",
      "code": "EPMS_907",
      "message": "Recusado pelo Processador. Número de Telemóvel não Activo no MCX Express..."
    }
  }
}
```

O número de telefone de teste (`931417623`) não está activo no Multicaixa
Express — comportamento esperado para um número que não foi
propositalmente preparado para o teste, não um erro do Spuri nem da
ferramenta de teste.

**Achado empírico importante:** o código de resposta é `200`, mas o campo
literal `"status"` devolvido é `"Failed"`. Segundo a tabela de códigos da
documentação da AppyPay (seção "Reason Description"), o código `200`
corresponde a `[Cancelled]`, não a `"Failed"`. Ou seja: **a própria resposta
ao vivo da AppyPay não bate 100% com a categorização que a própria
documentação da AppyPay descreve para esse código.**

Isto confirma, com dados reais, que a decisão de projeto já tomada pelo
Spuri — usar `appyPayCodeOutcomes` (baseado no `code` numérico) como fonte
de verdade em vez do campo `"status"` livre — não é só teoricamente mais
robusta, é **empiricamente necessária**: nem a própria AppyPay é
consistente entre o texto livre que devolve e a tabela que documenta. Como
`isTerminalChargeStatus` trata `"Failed"` e `"Cancelled"` da mesma forma
(ambos terminais, nenhum dos dois sucesso), isto **não tem nenhum impacto
funcional** — é só uma nota de precisão para o registro.

### 1.4 Consulta da cobrança criada (`GET /v2.0/charges/{id}`)

**Resultado: sucesso (HTTP 200).** Devolveu o envelope `{"payment": {...}}`
com `status`, `paymentMethod`, `transactionEvents` (incluindo o
`responseStatus` do evento de recusa) — exatamente a forma que
`extractProviderOutcome`/`liveChargeStatus` já sabem ler (caso `payment`,
tratado antes do caso `responseStatus` solto).

**Achado a acompanhar (não é um bug de código):** o campo
`applicationFeeAmount` veio como `50` (igual ao valor total da cobrança).
Em **todos** os exemplos da documentação da AppyPay, `applicationFeeAmount`
aparece como `0`. O Spuri não lê nem depende deste campo em nenhum lugar do
código hoje — não há nenhum bug ou impacto atual — mas isto é diretamente
relevante à tarefa já prevista "AppyPay aggregator model validation"
(confirmar `applicationFeeAmount = 0`, payout structure). Como esta
transação específica **falhou**, não dá para saber se `applicationFeeAmount`
volta a `0` numa transação bem-sucedida ou se reflete uma configuração real
da conta merchant de teste. **Recomendação:** repetir o Teste 1 com um
número de telefone realmente activo no Multicaixa Express (para a cobrança
chegar a `Success`) e verificar `applicationFeeAmount` nesse cenário; se
continuar a refletir o valor total, vale confirmar com o gestor comercial
da AppyPay antes de finalizar o modelo de agregador.

## Teste 2 — através da API do próprio Spuri (como um cliente externo)

Reconfirmado nesta sessão com o servidor HTTP real do Spuri (Gin real,
Postgres real, 124 migrations aplicadas), simulando exatamente o payload
**real** de recusa devolvido pela AppyPay no Teste 1 (seção 1.3 acima) no
lugar da AppyPay real (a única parte substituída — inevitável, dado o
bloqueio de rede do sandbox — foi a última milha de rede; toda a lógica de
construção de requisição e interpretação de resposta é código real e
inalterado do Spuri).

Fluxo testado: `POST /solicitacao-matricula/{codigo}/pagamento-matricula`
(pagamento de taxa de matrícula — inscrição do estudante numa academia),
sem nenhum cabeçalho de autenticação (rota pública, como já é hoje).

**Confirmado:**
- O corpo que o Spuri envia à AppyPay é byte a byte o mesmo formato validado
  no Teste 1 (`amount`, `currency`, `description`, `merchantTransactionId`,
  `paymentMethod`, `paymentInfo.phoneNumber`, sem `options`/`notify`).
- Ao receber o payload real de recusa, o Spuri classifica corretamente como
  `status: "Cancelled"`, preenche `codigo_provedor: 200`,
  `mensagem_provedor` (a mensagem real em português da AppyPay),
  `fonte_provedor: "GPO"`.
- O handler devolve HTTP 201 com esses detalhes completos para quem chamou
  a API (o frontend pode mostrar a mensagem real da AppyPay ao encarregado
  de educação).
- `efetivarVinculoMatriculaPaga` **não** é chamado (`status != "success"`) —
  nenhuma matrícula é efetivada indevidamente numa cobrança recusada.

## Conclusão geral

Nenhuma correção de código é necessária em autenticação ou geração de
cobrança AppyPay. As correções já entregues em tarefas anteriores (67, 68,
70, 77, 79) seguem corretas e agora estão confirmadas não só por leitura de
código e testes com mocks, mas por **duas chamadas reais e sucedidas** à
API de teste da AppyPay (autenticação e criação de cobrança) e por uma
consulta real bem-sucedida.

Suite completa re-executada nesta sessão antes de qualquer alteração
temporária ser revertida: `gofmt -l .` limpo, `go vet ./...` limpo, `go
build ./...` OK, `go test ./...` (unitários + integração com PostgreSQL
real) — **todos os pacotes passam**, incluindo um teste adicional desta
sessão que reproduz o payload real de recusa (removido do repositório após
validação, por ser scaffolding de teste, não código de produção).

Repositório devolvido bit a bit idêntico ao original (`go.mod`/`go.sum`
revertidos, nenhum arquivo de teste temporário deixado para trás — `git
status` limpo).

## Único item para acompanhamento (não é tarefa de código)

Verificar `applicationFeeAmount` numa transação GPO **bem-sucedida** (com
um número de telefone activo no Multicaixa Express) e, se continuar
diferente de `0`, confirmar o modelo de taxa com a AppyPay antes de fechar
a validação do modelo de agregador — ver seção 1.4 acima.
