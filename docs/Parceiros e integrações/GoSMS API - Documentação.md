# GoSMS API — Documentação

> Fonte: ficheiro OpenAPI fornecido (`GoSMS API - openapi.json`), cruzado com a doc oficial em https://developer.go-sms.co.ao/
> GoSMS é um gateway de envio de SMS em Angola. A API expõe três grupos de recursos: **Mensagens** (envio e consulta de SMS), **Remetentes** (Sender IDs) e **Créditos** (saldo e histórico de carregamentos).

---

## Índice

1. [Introdução](#1-introdução)
2. [Autenticação](#2-autenticação)
3. [Convenções da API](#3-convenções-da-api)
4. [Nota sobre os corpos de resposta](#4-nota-sobre-os-corpos-de-resposta)
5. [Mensagens](#5-mensagens)
6. [Remetentes](#6-remetentes)
7. [Créditos](#7-créditos)
8. [O que ainda falta capturar](#8-o-que-ainda-falta-capturar)

---

## 1. Introdução

**Base URL:** `https://api.go-sms.co.ao`

Todos os paths abaixo são relativos a esta base — ex: `/v1/messages` significa `https://api.go-sms.co.ao/v1/messages`.

---

## 2. Autenticação

Todos os endpoints exigem o header `Authorization`, com um token obtido no Portal GoSMS:

```http
Authorization: Token a9eb6ea6-5777-4848-a9ed-8cbffc74a503
```

O spec não indica o endereço do Portal GoSMS nem o fluxo de obtenção/rotação do token — apenas que o token "está disponível no Portal GoSMS". Isso deve ser confirmado com o fornecedor ou com quem já tenha acesso à conta.

---

## 3. Convenções da API

- **Content-Type:** obrigatório `application/json` em todos os pedidos `POST` (`Enviar Mensagem`, `Criar Remetente`).
- **Paginação:** os endpoints de listagem aceitam um parâmetro de query opcional `page`. O spec não documenta o tamanho de página, nem a estrutura do envelope de paginação (total de páginas, total de registos, etc.) — isto também precisa de ser confirmado com uma chamada real (ver secção 8).
- **Datas:**
  - `schedule` (em `Enviar Mensagem`) usa o formato `yyyyMMddHHmmss` — note que o spec marca este campo como `"format": "date-time"`, mas o exemplo (`20231015182000`) e a descrição deixam claro que **não é** ISO 8601, é este formato compacto próprio.
  - `start` / `end` (em `Listar Mensagens Intervalo de datas`) usam o formato `YYYYMMDD` (ex: `20231011`).

---

## 4. Nota sobre os corpos de resposta

O spec OpenAPI fornecido não inclui o corpo (schema/exemplo) de nenhuma resposta — apenas o código de estado HTTP e uma descrição textual. Confirmei isto com duas fontes independentes:

- A doc oficial gerada pelo próprio fornecedor em https://developer.go-sms.co.ao/ tem exatamente a mesma lacuna: para cada endpoint mostra o código (`200`, `201`, `204`, `4XX`) e a descrição, mas nenhum exemplo de JSON de resposta.
- Não encontrei nenhuma fonte pública (blog, repositório, coleção Postman) com uma resposta real capturada da GoSMS Angola.

**Atualização (29/08/2026):** a maioria dos corpos de resposta `200` foi entretanto capturada com chamadas `GET` reais, feitas com um token de conta válido — estão preenchidos nas secções 5–7 abaixo, com a nota **✅ capturado em 29/08/2026**. Continuam por confirmar os corpos de:
- `201` de `POST /v1/messages` e `POST /v1/senders/` (ações de escrita, não testadas de propósito — a primeira consome crédito e envia SMS real, e como a conta tinha crédito limitado optou-se por não gastar em testes; a segunda cria um remetente real na conta).
- `200` de `GET /v1/messages/recipient` e `GET /v1/messages/one` — precisam de um `phone_number`/`message_id` reais, que só existem depois de a conta ter enviado pelo menos uma SMS.
- `200` de `GET /v1/messages/date` — chamada feita mas a resposta não chegou a ser partilhada.

**Atualização (06/09/2026):** foi enviada uma SMS de teste real (`POST /v1/messages`, para `972475676`), o que permitiu capturar quase tudo o que faltava — ver secções 5.1, 5.2, 5.4 e 5.6. Duas coisas importantes que isto revelou:

1. **A API usa nomes de campo inconsistentes para o mesmo ID — e isso troca até os parâmetros de query.** O ID do envio em si (o "lote") aparece como `id` na resposta de `POST /v1/messages` e de `GET /v1/messages`, mas como `parent_id` na resposta de `GET /v1/messages/recipient`. Já o ID de cada destinatário dentro desse envio aparece sempre como `message_id` (dentro do array `recipients`, e também no `GET /v1/messages/recipient`). **Confirmado:** em `GET /v1/messages/one`, o parâmetro `message_id` espera na verdade o ID do destinatário, e `id` espera o ID do envio — invertido face à leitura literal das descrições do spec (ver secção 5.7).
2. **Existe um segundo formato de erro**, diferente do capturado antes:
   ```json
   {
     "errors": [
       { "message": "mensagem inexistente" }
     ]
   }
   ```
   Ou seja, `errors` aqui é um **array** de objetos só com `message` (sem `code`), enquanto o erro de token inválido tinha `errors` como **objeto único** com `message` e `code`. A API não é consistente na forma do erro entre endpoints — quem for tratar erros no código de integração deve preparar-se para os dois formatos (e possivelmente outros ainda não vistos), não assumir uma forma fixa.

Isto é uma amostra real, mas apenas dos dois casos observados (token inválido/malformado, e "mensagem inexistente" em `GET /v1/messages/one`) — não está confirmado se o `4XX` de `POST /v1/messages` (ex: número inválido, remetente não aprovado, saldo insuficiente) usa um dos dois envelopes acima ou uma terceira estrutura.

Duas exceções onde não há nada em falta, porque `204 No Content` nunca tem corpo por definição do próprio protocolo HTTP:
- `DELETE /v1/messages/{message_id}`
- `DELETE /v1/senders/{sender_id}`

Ver a secção 8 para como completar o que ainda falta.

---

## 5. Mensagens

### 5.1 Enviar Mensagem

**`POST` `/v1/messages`**

Permite enviar uma SMS para um ou mais números de telefone.

**Headers:**

| Header          | Obrigatório | Descrição                                                             |
|-----------------|:-----------:|-------------------------------------------------------------------------|
| `Authorization` | ✅          | Token de autorização disponível no Portal GoSMS.                       |
| `Content-Type`  | ✅          | `application/json`.                                                    |

**Body:**

| Campo      | Tipo     | Obrigatório | Descrição                                                                                      |
|------------|----------|:-----------:|----------------------------------------------------------------------------------------------------|
| `message`  | `string` | ✅          | Texto da mensagem.                                                                              |
| `from`     | `string` | ✅          | Remetente da mensagem — deve estar previamente criado e aprovado (ver [Criar Remetente](#61-criar-remetente)). |
| `to`       | `string` | ✅          | Número(s) destinatário(s) da mensagem.                                                          |
| `schedule` | `string` | ❌          | Data/hora para envio agendado, no formato `yyyyMMddHHmmss`.                                     |

```json
{
  "message": "Mensagem de teste.",
  "from": "MINHALOJA",
  "to": "921939411",
  "schedule": "20231015182000"
}
```

**Exemplo cURL:**

```bash
curl -X POST https://api.go-sms.co.ao/v1/messages \
  -H "Authorization: Token a9eb6ea6-5777-4848-a9ed-8cbffc74a503" \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Mensagem de teste.",
    "from": "MINHALOJA",
    "to": "921939411"
  }'
```

**Respostas:**

| Código | Descrição (do spec)                                                          | Corpo             |
|--------|--------------------------------------------------------------------------------|--------------------|
| `201`  | Mensagem enviada com sucesso                                                   | ✅ capturado em 06/09/2026 |
| `4XX`  | Houve um problema no pedido. Haverá uma descrição clara da causa do erro.     | 🔶 não documentado — dois formatos de erro diferentes já foram vistos noutros endpoints (ver secção 4), mas nenhum especificamente para este |

```json
{
  "id": "4cffa53b-4f05-4334-b866-455b7ccdc15e",
  "content": "Spuri Reen",
  "cost": "8.8",
  "size": 10,
  "gsm": true,
  "parts": 1,
  "from": "SPURI",
  "created_at": "2026-09-06T21:06:18.907+01:00",
  "recipients": [
    {
      "phone_number": "972475676",
      "message_status": "PENDING",
      "message_id": "5b7e0e2d-a7a3-40b5-aa39-0dddd31fa8e0",
      "group_name": "Sem grupo de envio"
    }
  ],
  "total_recipients": 1,
  "recipients_pending": 1,
  "recipients_delivered": 0,
  "recipients_refused": 0,
  "total_pdus": 1
}
```

> `id`: ID do envio (o "lote"), usado depois em `GET /v1/messages/{message_id}` da rota de apagar. `cost`: string decimal, custo do envio (não confirmado se é em Kwanzas ou noutra unidade). `size`: nº de caracteres da mensagem. `gsm`: `true` quando a mensagem cabe na codificação GSM-7 (sem acentos/emojis fora do alfabeto suportado); `false` deve ativar UCS-2 e reduzir os caracteres por parte. `parts`: nº de segmentos SMS (mensagens longas usam mais de 1). `recipients[].message_status` observado: `PENDING` (logo após o envio) e, mais tarde, `DELIVERED` (ver 5.2) — provavelmente também existe `REFUSED`, dado o contador `recipients_refused`. `recipients[].message_id`: ID do destinatário **dentro deste envio** — não confundir com o `id` do envio em si (ver nota na secção 4 sobre a inconsistência de nomes). `group_name`: `"Sem grupo de envio"` por omissão quando o envio não usa grupos de destinatários. `total_pdus`: unidades de protocolo, aqui igual a `parts × total_recipients`.

### 5.2 Listar Mensagens

**`GET` `/v1/messages`**

**Headers:** `Authorization` (✅)

**Query params:**

| Parâmetro | Tipo      | Obrigatório | Descrição                                                              |
|-----------|-----------|:-----------:|----------------------------------------------------------------------------|
| `page`    | `integer` | ❌          | Página específica dos resultados, quando a listagem é paginada.       |

**Respostas:**

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ✅ capturado em 06/09/2026 |

```json
{
  "messages": [
    {
      "id": "4cffa53b-4f05-4334-b866-455b7ccdc15e",
      "content": "Spuri Reen",
      "cost": "8.8",
      "size": 10,
      "gsm": true,
      "parts": 1,
      "from": "SPURI",
      "created_at": "2026-09-06T21:06:18.907+01:00",
      "recipients": [
        {
          "phone_number": "972475676",
          "message_status": "DELIVERED",
          "message_id": "5b7e0e2d-a7a3-40b5-aa39-0dddd31fa8e0",
          "group_name": "Sem grupo de envio"
        }
      ],
      "total_recipients": 1,
      "recipients_pending": 0,
      "recipients_delivered": 1,
      "recipients_refused": 0,
      "total_pdus": 1
    }
  ],
  "pagination": {
    "total_itens": 1,
    "page": 0,
    "items_per_page": 20,
    "last_page": 0
  }
}
```

> Cada item de `messages` tem exatamente a forma do corpo devolvido por `POST /v1/messages` (secção 5.1) — mesmos campos. Aqui dá para ver a transição de estado: o mesmo envio que em 5.1 tinha `message_status: "PENDING"` e `recipients_pending: 1` já aparece como `DELIVERED`/`recipients_delivered: 1` minutos depois. `page` é indexado a partir de `0`. Este envelope de paginação (`total_itens`, `page`, `items_per_page`, `last_page`) é o mesmo usado nos outros endpoints de listagem — confirmado igual em [Listar Destinatários](#54-listar-destinatários).

### 5.3 Apagar Mensagem

**`DELETE` `/v1/messages/{message_id}`**

Apagar o registo de envio de uma SMS.

**Headers:** `Authorization` (✅)

**Path params:**

| Parâmetro    | Tipo     | Descrição      |
|---------------|----------|--------------------|
| `message_id` | `string` | ID da mensagem.   |

**Respostas:**

| Código | Descrição (do spec)          | Corpo                                  |
|--------|----------------------------------|---------------------------------------------|
| `204`  | Pedido Realizado com sucesso     | Nenhum (`204 No Content` não tem corpo) |

### 5.4 Listar Destinatários

**`GET` `/v1/messages/recipients`**

Listar todos os números que já foram alvo de uma SMS através da conta.

**Headers:** `Authorization` (✅)

**Query params:**

| Parâmetro | Tipo      | Obrigatório | Descrição                                    |
|-----------|-----------|:-----------:|--------------------------------------------------|
| `page`    | `integer` | ❌          | Página específica dos resultados.               |

**Respostas:**

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ✅ capturado em 06/09/2026 |

```json
{
  "recipients": [
    { "phone_number": "972475676" }
  ],
  "pagination": {
    "total_itens": 1,
    "page": 0,
    "items_per_page": 20,
    "last_page": 0
  }
}
```

> Ao contrário do array `recipients` dentro de uma mensagem (secção 5.1/5.2), aqui cada item é só `{"phone_number": ...}` — faz sentido, dado que esta rota lista números únicos já contactados, não envios individuais. Mesmo envelope de paginação de [Listar Mensagens](#52-listar-mensagens).

### 5.5 Listar Mensagens por Intervalo de Datas

**`GET` `/v1/messages/date`**

Listar todas as mensagens enviadas dentro de um intervalo de datas.

**Headers:** `Authorization` (✅)

**Query params:**

| Parâmetro | Tipo      | Obrigatório | Descrição                              |
|-----------|-----------|:-----------:|---------------------------------------------|
| `start`   | `integer` | ✅          | Data de início, no formato `YYYYMMDD`.      |
| `end`     | `integer` | ✅          | Data de fim, no formato `YYYYMMDD`.         |
| `page`    | `integer` | ❌          | Página específica dos resultados.           |

**Respostas:**

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ⚠️ capturado, mas com um resultado estranho — ver nota |

```json
{
  "messages": [],
  "pagination": {
    "total_itens": 0,
    "page": 0,
    "items_per_page": 20,
    "last_page": 0
  }
}
```

> **Anomalia:** esta chamada (`start=20250101&end=20261231`) foi feita minutos **depois** do envio de 06/09/2026 (secção 5.1), e mesmo assim devolveu `messages` vazio, apesar de a mensagem enviada estar claramente dentro do intervalo pedido — enquanto `GET /v1/messages` (sem filtro de data) já mostrava a mensagem. Não está confirmado se é atraso de indexação neste endpoint específico, uma diferença na forma como `start`/`end` são interpretados (fuso horário? o formato `YYYYMMDD` pode não incluir o próprio dia?), ou um bug do lado da GoSMS. Vale testar de novo mais tarde, e/ou com um intervalo mais estreito à volta de `20260906`.

### 5.6 Listar Mensagens de um Destinatário

**`GET` `/v1/messages/recipient`**

**Headers:** `Authorization` (✅)

**Query params:**

| Parâmetro      | Tipo      | Obrigatório | Descrição                              |
|-----------------|-----------|:-----------:|---------------------------------------------|
| `phone_number` | `integer` | não indicado no spec | Número de telefone do destinatário. |
| `page`         | `integer` | ❌          | Página específica dos resultados.           |

**Respostas:**

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ✅ capturado em 06/09/2026 |

```json
{
  "messages": [
    {
      "content": "Spuri Reen",
      "message_status": "DELIVERED",
      "message_id": "5b7e0e2d-a7a3-40b5-aa39-0dddd31fa8e0",
      "parent_id": "4cffa53b-4f05-4334-b866-455b7ccdc15e",
      "created_at": "2026-09-06T21:06:18.907+01:00"
    }
  ],
  "pagination": {
    "total_itens": 1,
    "page": 0,
    "items_per_page": 20,
    "last_page": 0
  }
}
```

> `message_id` aqui é o ID do **destinatário dentro do envio** (o mesmo valor que aparece como `recipients[].message_id` em `POST /v1/messages`/`GET /v1/messages`), e `parent_id` é o ID do **envio em si** (o mesmo valor que aparece como `id` nesses outros endpoints). Ou seja, para este endpoint específico, o "ID do envio" muda de nome de `id` para `parent_id` — mais um caso da inconsistência de nomes descrita na secção 4.

### 5.7 Mostrar Mensagem por ID

**`GET` `/v1/messages/one`**

**Headers:** `Authorization` (✅)

**Query params:**

| Parâmetro    | Tipo     | Descrição (do spec)                                                    | Uso real confirmado                                    |
|---------------|----------|--------------------------------------------------------------------------|----------------------------------------------------------|
| `message_id` | `string` | ID da mensagem.                                                         | ID do **destinatário dentro do envio** (não do envio em si) |
| `id`         | `string` | ID do número de destino da SMS no contexto de uma mensagem.            | ID do **envio** (o mesmo `id`/`parent_id` visto noutros endpoints) |

**Respostas:**

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ✅ capturado em 06/09/2026 |

**Confirmado:** ao contrário do que a descrição do spec sugere, o parâmetro `message_id` espera o ID do **destinatário dentro do envio** (o que a API chama `message_id` no array `recipients`), e o parâmetro `id` espera o ID do **envio em si** (o que a API chama `id`/`parent_id` noutros endpoints) — invertido face à leitura literal de "`message_id`: ID da Mensagem" / "`id`: ID do número de destino". Usar assim:

```bash
curl -s "https://api.go-sms.co.ao/v1/messages/one?message_id=5b7e0e2d-a7a3-40b5-aa39-0dddd31fa8e0&id=4cffa53b-4f05-4334-b866-455b7ccdc15e" \
  -H "Authorization: Token <TOKEN_REAL>"
```

```json
{
  "id": "4cffa53b-4f05-4334-b866-455b7ccdc15e",
  "content": "Spuri Reen",
  "cost": "8.8",
  "size": 10,
  "gsm": true,
  "parts": 1,
  "from": "SPURI",
  "created_at": "2026-09-06T21:06:18.907+01:00",
  "recipients": [
    {
      "phone_number": "972475676",
      "message_status": "DELIVERED",
      "message_id": "5b7e0e2d-a7a3-40b5-aa39-0dddd31fa8e0",
      "group_name": "Sem grupo de envio"
    }
  ],
  "total_recipients": 1,
  "recipients_pending": 0,
  "recipients_delivered": 1,
  "recipients_refused": 0,
  "total_pdus": 1
}
```

> Corpo idêntico ao de um item de `GET /v1/messages` (secção 5.2) — mesma forma, só que devolve uma única mensagem específica em vez de uma lista.

---

## 6. Remetentes

### 6.1 Criar Remetente

**`POST` `/v1/senders/`**

**Headers:**

| Header          | Obrigatório | Descrição            |
|-----------------|:-----------:|---------------------------|
| `Authorization` | ✅          | Token do Portal GoSMS.   |
| `Content-Type`  | ✅          | `application/json`.       |

**Body:**

| Campo  | Tipo     | Descrição                     |
|--------|----------|------------------------------------|
| `name` | `string` | Nome do remetente da SMS.         |

```json
{
  "name": "LOJAHEBER"
}
```

**Respostas:**

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `201`  | Pedido Realizado com sucesso     | 🔶 não documentado — não testado de propósito (criaria um remetente real na conta) |

### 6.2 Listar Remetentes

**`GET` `/v1/senders`** — **Headers:** `Authorization` (✅)

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ✅ capturado em 29/08/2026 |

```json
{
  "total": 1,
  "remetentes": [
    {
      "id": "1fbc8c16-7a75-4c6a-8a3a-850b41713fdc",
      "name": "SPURI",
      "status": "APPROVED",
      "created_at": "2026-08-17T23:42:03.522+01:00"
    }
  ]
}
```

> `status` observado: `APPROVED`. Outros valores prováveis (não confirmados): `PENDING`, e possivelmente algo como `REJECTED` — o spec só descreve os grupos "Aprovados" e "Pendentes de Aprovação", nada sobre rejeição. `created_at` vem em ISO 8601 com offset (`+01:00`, hora de Angola/WAT — na prática sempre `+01:00`, já que Angola não observa horário de verão).

### 6.3 Listar Remetentes Aprovados

**`GET` `/v1/senders/approved`** — **Headers:** `Authorization` (✅)

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ✅ capturado em 29/08/2026 |

```json
[
  {
    "id": "1fbc8c16-7a75-4c6a-8a3a-850b41713fdc",
    "name": "SPURI",
    "status": "APPROVED",
    "created_at": "2026-08-17T23:42:03.522+01:00"
  }
]
```

> Ao contrário de `GET /v1/senders`, este vem como **array direto**, sem envelope `{"total": ..., "remetentes": [...]}`. Mesma forma de objeto remetente.

### 6.4 Listar Remetentes Pendentes de Aprovação

**`GET` `/v1/senders/pending`** — **Headers:** `Authorization` (✅)

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ✅ capturado em 29/08/2026 (vazio) |

```json
[]
```

> Array direto (mesmo padrão de `/v1/senders/approved`), sem remetentes pendentes nesta conta no momento do teste — a forma do objeto dentro do array, quando não vazio, ainda não está confirmada (por analogia deve ser igual à de `/v1/senders/approved`, com `status: "PENDING"`).

### 6.5 Apagar Remetente

**`DELETE` `/v1/senders/{sender_id}`**

**Headers:** `Authorization` (✅)

**Path params:**

| Parâmetro   | Tipo     | Descrição       |
|--------------|----------|---------------------|
| `sender_id` | `string` | ID do Remetente.   |

**Respostas:**

| Código | Descrição (do spec)          | Corpo                                  |
|--------|----------------------------------|---------------------------------------------|
| `204`  | Pedido Realizado com sucesso     | Nenhum (`204 No Content` não tem corpo) |

---

## 7. Créditos

### 7.1 Mostrar Saldo/Créditos de SMS

**`GET` `/v1/credits`** — **Headers:** `Authorization` (✅)

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ✅ capturado em 29/08/2026 |

```json
{
  "available_sms": 10,
  "postpaid": false
}
```

> `available_sms`: número de SMS restantes no saldo pré-pago. `postpaid`: `false` indica conta pré-paga (com `available_sms` a fazer sentido); numa conta `postpaid: true` não está confirmado se `available_sms` continua presente ou com outro significado.

### 7.2 Histórico de Carregamentos

**`GET` `/v1/recharges`** — **Headers:** `Authorization` (✅)

| Código | Descrição (do spec)          | Corpo             |
|--------|----------------------------------|--------------------|
| `200`  | Pedido Realizado com sucesso     | ✅ capturado em 29/08/2026 (vazio) |

```json
[]
```

> Array direto, sem carregamentos ainda nesta conta — a forma de cada item (valor, data, método de pagamento, etc.) ainda não está confirmada.

---

## 8. O que ainda falta capturar

1. **`4XX` de `POST /v1/messages`** — só temos exemplos de erro de outros endpoints (secção 4), nenhum de um envio malsucedido em si (ex: número inválido, remetente não aprovado, saldo insuficiente). Não é essencial e não vale gastar crédito só para provocar isto de propósito.
2. **`POST /v1/senders/`** (`201`) — endpoint de escrita, cria um remetente real na conta; continua não testado de propósito.
3. **Anomalia da secção 5.5** — `GET /v1/messages/date` devolveu vazio para um intervalo que devia incluir a mensagem enviada; vale confirmar mais tarde se foi só atraso de indexação.
4. Itens menores já assinalados nas suas secções: forma dos itens (quando não vazios) de `/v1/senders/pending` e `/v1/recharges`, que dependem de eventos que ainda não aconteceram nesta conta (um remetente pendente de aprovação, um carregamento de crédito).

Nenhum destes é bloqueante — são todos casos de borda ou ações de escrita que consomem recursos reais da conta.
