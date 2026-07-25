---
modificado: 2026-07-25 17:05
criado: 2026-07-14 19:07
---
# Análise de Integração AppyPay para o Serviço de Gestão Financeira do Spuri

## 0. Objetivo deste documento

O Spuri (plataforma de gestão académica) precisa de um serviço de gestão financeira para cobrar, através da AppyPay, as escolas suas clientes (ex.: assinatura mensal do serviço Spuri).

Este documento identifica, **com base única e exclusivamente na documentação da API AppyPay**, quais rotas, métodos e mecanismos da AppyPay são usados para esse fluxo de cobrança.

---

## 1. Ambientes e URLs base

|Serviço|Ambiente|Base URL|
|---|---|---|
|Autenticação (OAuth2 Token)|TEST|`https://login.microsoftonline.com/appypaydev.onmicrosoft.com/oauth2/token`|
|Autenticação (OAuth2 Token)|PROD|`https://login.microsoftonline.com/auth.appypay.co.ao/oauth2/token`|
|AppyPay-API (Payments Gateway — cobranças, mandatos, referências, documentos fiscais)|TEST|`https://gwy-api-tst.appypay.co.ao/{version}`|
|AppyPay-API (Payments Gateway)|PROD|`https://gwy-api.appypay.co.ao/{version}`|
|AppyPay-Web-API (backoffice/portal — payouts)|DEV (única URL documentada)|`https://app-appypay-webapi-dev-001.azurewebsites.net/api/{version}`|

`{version}` = `v2.0` (versão corrente e default; v1.1 e v1.2 são legadas).

---

## 2. Autenticação

- Fluxo: **OAuth2 Client Credentials Grant**, via Azure AD B2C.
- Token: `Bearer`, válido por **1 hora**. Recomenda-se cache/reuso local do token até próximo da expiração.
- Credenciais (`client_id` / `client_secret`) são geradas no Portal AppyPay ou solicitadas ao gestor comercial.

**Endpoint:**

```
GET {auth_base}/token
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
client_id={client_id}
client_secret={client_secret}
resource={resource_guid}
```

**Resposta 200:**

```json
{
  "token_type": "Bearer",
  "expires_in": "3599",
  "access_token": "eyJ..."
}
```

Todas as chamadas subsequentes usam `Authorization: Bearer <access_token>`.

---

## 3. Conceitos-chave do modelo de dados AppyPay

|Conceito|O que a documentação mostra|Fonte|
|---|---|---|
|**Merchant**|A entidade contratante junto à AppyPay (no nosso caso, o Spuri). Possui credenciais próprias (`client_id`/`client_secret`), conta(s) bancária(s) e definições fiscais.|Autenticação|
|**Application**|Um "application" **não é um sub-cliente/sub-loja**; é a integração de um **método de pagamento específico** (GPO, UMM, REF, eTPA, SDD…) dentro da conta do merchant, com o seu próprio `apiKey` e Webhook. `GET /applications` devolve uma lista de `applications`, cada uma com `paymentMethod`, `applicationKeys[].apiKey` e `webHookUrl`.|Get all applications|
|**`paymentMethod` usado em `POST /charges`**|Não é apenas "GPO"/"UMM"/"REF"; é uma string composta `{IDENTIFICADOR}_{apiKey}` (ex.: `GPO_53c70da3-1c88-4391-8b60-ab4757fbb044`), ou seja, o `apiKey` de uma _application_ específica.|Post a Charge|
|**SDD Creditor Account**|`GET /applications/{applicationId}/sdd-creditor-accounts/{creditorAccId}` — conta credora vinculada a uma _application_ ligada ao método SDD (Débito Direto), usada como conta de destino dos débitos diretos daquela application.|Get a creditor account for the specific application (beta)|
|**Payout**|Liquidação periódica feita pela AppyPay ao **merchant**, reunindo várias transações (`transactions[]`) de uma dada `application`/`paymentMethod` num período. Consultável via `GET /payouts` e `GET /payouts/{transferId}`.|Get a payout; Get all payouts|

---

## 4. Spuri cobra as Escolas (ex.: assinatura mensal da plataforma)

Neste modelo, o Spuri é o **merchant** (credor) e cada **escola** é o **pagador/devedor**. É o caso de uso "normal" de um gateway de pagamentos: um único merchant a cobrar os seus próprios clientes.

### 4.1 Métodos de pagamento aplicáveis

|Método|Adequação ao caso de uso|Observações da documentação|
|---|---|---|
|**REF** (Referência Multicaixa)|✅ Recomendado para cobrança pontual/mensal manual|Sem valor mínimo/máx. documentado além de "min 9–15 dígitos" no número; expira em 72h por default; suporta valor fixo ou livre (`minAmount`/`maxAmount`)|
|**GPO** (Gateway de Pagamentos Online / Multicaixa Express)|✅ Boa opção para pagamento online assistido pela escola (via app MCX)|Aprovação até 90s; valor mínimo 1 AOA|
|**UMM** (Unitel Money)|✅ Alternativa por telemóvel|Aprovação até 30s; valor mínimo 50 AOA; suporta Reversão e Reembolso|
|**SDD** (Débito Direto) — **ALPHA**|✅ **Recomendado especificamente para a cobrança recorrente da assinatura mensal**, por ser o único método com suporte nativo a mandato + débito periódico|Requer mandato ativo; débito com no mínimo 10 dias de antecedência da data solicitada; frequência configurável (`MNTH` = mensal)|
|**FTBAI** (Transferência interbancária, BETA)|⚠️ Não é "cobrança" (é transferência ativa do Spuri, não puxa dinheiro da escola) — não recomendado para este modelo|Só BAI↔BAI|

### 4.2 Fluxo recomendado A — Cobrança pontual/manual (REF ou GPO/UMM)

1. `GET {auth}/token` — obter token do Spuri.
2. `POST {api}/charges` — criar a cobrança, com `paymentMethod` = `REF_<apiKey da application REF do Spuri>` (ou `GPO_<apiKey>` / `UMM_<apiKey>`).
3. Se síncrono (`Accept: application/json`): aguardar resposta imediata (até 90s dependendo do método). Se assíncrono (`Accept: application/vnd.appypay.asyncapi+json`): receber `202`, e o resultado chega via **Webhook do merchant** configurado no Portal.
4. `GET {api}/charges/{id}` — confirmar o estado da transação (recomendado sempre, como segurança adicional, mesmo tendo recebido webhook).
5. Emitir opcionalmente o **documento fiscal** correspondente (fatura/recibo) via `POST {api}/fiscal-documents` (ver secção 4.5).

### 4.3 Fluxo recomendado B — Cobrança recorrente automática via Débito Direto (SDD)

Este é o mecanismo mais adequado para **débito automático mensal da assinatura da escola**, pois evita que o Spuri tenha de gerar manualmente uma nova cobrança todos os meses.

**Passo 1 — Criar o Mandato (contrato de autorização de débito):**

```
POST {api}/direct-debit/mandates      (multipart/form-data)
```

Campos relevantes: `applicationKey` (apiKey da application SDD do Spuri), `creditorAccId` (conta credora do Spuri — ver `GET /applications/{id}/sdd-creditor-accounts/{id}`), `currencyId=AOA`, `frequencyType=MNTH`, `sequenceTypeCode=RCUR`, `merchantReferenceNumber` (referência interna do Spuri para identificar a escola/contrato), `debitStartDate`, `debitDay`, `serviceDescription`, `maxAmount` (teto de segurança), `debitorName`, `debitorNIF`, `debitorIBAN` (dados da **escola**, que é o "debitor" neste contexto), `debitorTelephone`, `debitorEmail`, `debitorSignaturePlace`, `debitorSignatureDate`, `debitorSignature` (upload da assinatura, PNG/JPG ≤20KB).

**Passo 2 — Acompanhar o estado do mandato:**

- `GET {api}/direct-debit/mandates/{mandateId}` — consulta pontual.
- `GET {api}/direct-debit/mandates` — listagem (filtros por `MerchantReferenceNumber`, `mandateStatusId`).
- **Webhook não-transacional** (configurado no Portal) — notifica automaticamente a cada mudança de estado (`Requested → Pending → Active/Failed/Expired`).

**Passo 3 — Cobrar mensalmente usando o mandato ativo:**

```
POST {api}/charges
{
  "amount": <valor da assinatura>,
  "currency": "AOA",
  "merchantTransactionId": "<id único>",
  "paymentMethod": "SDD_<apiKey>",
  "paymentInfo": { "mandateId": "<id do mandato ativo>" }
}
```

> ⚠️ Regra documentada em "SDD Payments Error Handling": a data de débito da instrução deve ser **pelo menos 10 dias posterior à data do pedido**, e deve respeitar a janela do mandato (não anterior ao `debitStartDate`, não posterior ao `debitEndDate`). Isto significa que o Spuri **não pode debitar no mesmo dia** — o ciclo de cobrança mensal precisa de ser programado com essa antecedência mínima.

**Passo 4 — Cancelamento (se a escola sair da plataforma ou mudar de plano):**

```
POST {api}/mandates/{mandateId}/cancel
{ "reason": "CTCA", "description": "Cancelamento por rescisão de contrato" }
```

### 4.4 Consulta, reembolso e reversão

|Ação|Endpoint|Suporte por método|
|---|---|---|
|Consultar uma cobrança|`GET {api}/charges/{id}` (ou por `merchantTransactionId` via query)|Todos|
|Listar/pesquisar cobranças (com filtros de data/valor)|`GET {api}/charges`|Todos|
|Reembolsar (total ou parcial)|`POST {api}/refunds/{id}`|Apenas **GPO** e **UMM** (e SDD, com `reasonCode` obrigatório)|
|Reverter|`POST {api}/reverses/{id}`|Apenas **UMM**|
|Analytics agregada de cobranças (com filtros mais amplos)|`GET {api}/analytics/charges`|Todos|

### 4.5 Documentos fiscais (complementar, opcional mas recomendado)

A AppyPay oferece um serviço de **faturação eletrónica em conformidade com SAF-T(AO)/AGT**, útil para o Spuri emitir a fatura/recibo da assinatura mensal automaticamente após cada cobrança:

1. `POST {api}/fiscal-series` — criar série de numeração (necessária antes de emitir documentos).
2. `POST {api}/fiscal-documents` — criar o documento (ex.: `documentType=FR` — Fatura/Recibo), assíncrono (`202`), notificado por webhook não-transacional quando processado pela AGT.
3. `GET {api}/fiscal-documents/{id}` — obter o PDF/QR code do documento processado.
4. `GET {api}/fiscal-documents/taxpayer/{nif}` — validar o NIF da escola antes de emitir o documento.
5. `POST {api}/fiscal-documents/{id}/validate` — confirmar/rejeitar um documento (fluxo do adquirente, quando aplicável).

### 4.6 Webhooks necessários

|Tipo|Obrigatório quando|Conteúdo|
|---|---|---|
|Webhook transacional (Charges)|Sempre que se usar Widget; **sempre obrigatório para REF**; obrigatório para pedidos assíncronos de GPO/UMM/eTPA|Estado final da cobrança (`responseStatus`, `id`, `merchantTransactionId`, `amount`…)|
|Webhook não-transacional (SDD)|Recomendado para acompanhar mudanças de estado do mandato|`directDebit.mandateStatus`, `providerMandateId`, datas|
|Webhook não-transacional (Fiscal Documents)|Recomendado para saber quando o documento fica pronto|Estado do documento fiscal|

> Nota: a AppyPay avisa que o mesmo webhook pode ser **chamado mais de uma vez** para a mesma transação (ex.: comunicação instável com o provedor). O Spuri deve tratar o webhook de forma idempotente e, por segurança, sempre confirmar com `GET /charges/{id}` antes de dar a cobrança como definitivamente liquidada.

### 4.7 Tabela-resumo de endpoints

|Método HTTP|Endpoint|Finalidade|
|---|---|---|
|GET|`{auth}/token`|Obter token Bearer|
|POST|`{api}/charges`|Criar cobrança pontual (REF/GPO/UMM) ou disparar débito sob mandato ativo (SDD)|
|GET|`{api}/charges/{id}`|Consultar uma cobrança específica|
|GET|`{api}/charges`|Listar/pesquisar cobranças|
|GET|`{api}/analytics/charges`|Analytics/pesquisa avançada de cobranças|
|POST|`{api}/refunds/{id}`|Reembolsar (GPO, UMM, SDD)|
|POST|`{api}/reverses/{id}`|Reverter (UMM)|
|POST|`{api}/direct-debit/mandates`|Criar mandato de débito direto com a escola|
|GET|`{api}/direct-debit/mandates/{mandateId}`|Consultar mandato|
|GET|`{api}/direct-debit/mandates`|Listar mandatos|
|POST|`{api}/mandates/{mandateId}/cancel`|Cancelar mandato|
|GET|`{api}/applications/{applicationId}/sdd-creditor-accounts/{creditorAccId}`|Consultar conta credora SDD do Spuri|
|POST|`{api}/references`|Criar referência(s) de pagamento Multicaixa (REF)|
|GET|`{api}/references/{referenceNumber}`|Consultar referência|
|GET|`{api}/references`|Listar referências|
|GET|`{api}/analytics/references`|Analytics de referências|
|POST|`{api}/fiscal-series`|Criar série fiscal|
|GET|`{api}/fiscal-series`|Listar séries fiscais|
|POST|`{api}/fiscal-documents`|Emitir fatura/recibo da assinatura|
|GET|`{api}/fiscal-documents/{id}`|Obter documento fiscal processado|
|GET|`{api}/fiscal-documents`|Listar documentos fiscais|
|GET|`{api}/fiscal-documents/taxpayer/{nif}`|Validar NIF da escola|
|GET|`{api}/applications`|Listar as _applications_ (métodos de pagamento) do Spuri, com `apiKey`|
|GET|`{webapi}/payouts` , `{webapi}/payouts/{transferId}`|Consultar liquidações recebidas da AppyPay pelo Spuri|

---

## 5. Tratamento de erros e envelope de resposta

Todas as respostas seguem, de forma geral, um envelope com:

```json
{
  "id": "uuid",
  "responseStatus": {
    "successful": true,
    "status": "Success",      // Requested | Pending | Success | Failed | Cancelled | Expired
    "code": 100,
    "message": "...",
    "source": "GPO",           // APPY | REF | UMM | FTBAI | SDD | ...
    "sourceDetails": { "attempt": 1, "type": "...", "code": "...", "message": "..." }
  }
}
```

Códigos HTTP: `200` OK, `202` Accepted (processamento assíncrono), `400`/`401`/`403`/`404`/`405`/`415` erros de cliente, `500` erro de servidor.

Existe uma tabela extensa de **códigos de resposta internos** (ex.: `100` sucesso, `101` pedido aceite para processamento, `200-2xx` recusas específicas de GPO, `230-250` recusas específicas de UMM, `Error_01_xx`/`Error_02_xx`/`Error_03_xx` para SDD) que deve ser mapeada no backend do Spuri para mensagens de erro amigáveis ao utilizador final. Recomenda-se tratar isto centralizado num módulo de "tradução de erros AppyPay" no Spuri, dado o volume de códigos.

Mensagens de erro suportam **tradução PT/EN** via header `Accept-Language` ou query `culture`.

---

## 6. Apêndice — Referência completa dos endpoints identificados na documentação

|Método|Endpoint|Categoria|
|---|---|---|
|GET|`{auth}/token`|Autenticação|
|GET|`{api}/applications`|Applications|
|GET|`{api}/applications/{id}`|Applications (pontos de venda)|
|GET|`{api}/applications/{applicationId}/sdd-creditor-accounts/{creditorAccId}`|SDD|
|POST|`{api}/charges`|Cobranças|
|GET|`{api}/charges/{id}`|Cobranças|
|GET|`{api}/charges`|Cobranças|
|GET|`{api}/analytics/charges`|Cobranças (analytics)|
|POST|`{api}/refunds/{id}`|Cobranças (reembolso)|
|POST|`{api}/reverses/{id}`|Cobranças (reversão)|
|POST|`{api}/qr-codes`|GPO QR Code|
|POST|`{api}/direct-debit/mandates`|SDD (mandatos)|
|GET|`{api}/direct-debit/mandates/{mandateId}`|SDD (mandatos)|
|GET|`{api}/direct-debit/mandates`|SDD (mandatos)|
|POST|`{api}/mandates/{mandateId}/cancel`|SDD (mandatos)|
|POST|`{api}/references`|Referências|
|GET|`{api}/references/{referenceNumber}`|Referências|
|GET|`{api}/references`|Referências|
|GET|`{api}/analytics/references`|Referências (analytics)|
|POST|`{api}/mocks/referenceProcessing`|Referências (mock/teste)|
|GET|`{api}/accounts/{identifier}`|Contas (UMM)|
|POST|`{api}/fiscal-series`|Documentos fiscais|
|GET|`{api}/fiscal-series`|Documentos fiscais|
|POST|`{api}/fiscal-documents`|Documentos fiscais|
|GET|`{api}/fiscal-documents/{id}`|Documentos fiscais|
|GET|`{api}/fiscal-documents`|Documentos fiscais|
|GET|`{api}/fiscal-documents/taxpayer/{nif}`|Documentos fiscais|
|POST|`{api}/fiscal-documents/{id}/validate`|Documentos fiscais|
|GET|`{webapi}/payouts/{transferId}`|Payouts (Web-API)|
|GET|`{webapi}/payouts`|Payouts (Web-API)|

---

## 8. Resumo executivo

- A cobrança das escolas pela plataforma está **bem suportada** pela documentação da AppyPay fornecida: cobrança pontual via REF/GPO/UMM, e cobrança recorrente mensal via **Débito Direto (SDD)** com mandato + intents periódicos. Todos os endpoints necessários (cobrança, consulta, reembolso, mandatos, faturação) estão documentados com corpo de requisição/resposta claros.
- Resta uma única pergunta em aberto para a equipa AppyPay (secção 6): a maturidade do método SDD para uso em produção, dado que a documentação o lista como **ALPHA**.
