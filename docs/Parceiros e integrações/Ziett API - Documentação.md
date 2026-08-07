# Ziett API — Documentação Completa

> Fonte: https://ziett.co/docs/introduction
> Ziett é uma Communications Platform as a Service (CPaaS) para envio de SMS, WhatsApp e notificações push, focada em Angola/África.

---

## Índice

1. [Introdução](#1-introdução)
2. [Autenticação](#2-autenticação)
3. [Rate Limits & Fiabilidade](#3-rate-limits--fiabilidade)
4. [Convenções da API](#4-convenções-da-api)
5. [Endpoint: Send Message](#5-endpoint-send-message)
6. [Endpoint: Retrieve Message](#6-endpoint-retrieve-message)
7. [Endpoint: List Messages](#7-endpoint-list-messages)
8. [Endpoint: Send Batch Campaign](#8-endpoint-send-batch-campaign)
9. [Endpoint: Retrieve Campaign](#9-endpoint-retrieve-campaign)

---

## 1. Introdução

Ziett é uma plataforma developer-first (CPaaS) para envio de mensagens em escala — de forma fiável, eficiente e multi-canal. Serve tanto para um OTP transacional único como para campanhas multi-canal para centenas de milhares de destinatários, através de uma única API unificada.

### O que se pode construir

- **Mensagens transacionais** — alertas em tempo real, OTPs, notificações de entrega e eventos de sistema via SMS ou WhatsApp, disparados diretamente pela aplicação.
- **Campanhas em massa** — upload de listas de contactos e disparo de campanhas de marketing/operacionais de alto volume, de forma assíncrona, com tracking completo de entrega.
- **Fallback omnichannel** — lógica de routing que tenta entrega pelo canal preferido (ex: Push Notification) e recua automaticamente para WhatsApp ou SMS em caso de falha.
- **WhatsApp baseado em templates** — envio de templates HSM (Highly Structured Message) aprovados pela Meta, para casos de uso utilitário, marketing e autenticação, com injeção de variáveis feita no servidor.

### Canais suportados

| Canal               | Estado           | Notas                                               |
|---------------------|------------------|------------------------------------------------------|
| SMS                 | Disponível       | Suporte completo para mensagens transacionais e em massa |
| WhatsApp (HSM)      | Em integração    | Mensagens baseadas em templates via Meta Cloud API   |
| Telegram            | Planeado         | —                                                     |
| Push Notifications  | Planeado         | —                                                     |

### Primeiros passos

1. Obter a **API Key** no [Ziett Dashboard](https://app.ziett.co), em *Organization → API Keys*.
2. Ler o guia de [Autenticação](#2-autenticação) para saber como incluir a chave em cada pedido e como a manter segura.
3. Fazer a primeira chamada — [enviar uma mensagem transacional](#5-endpoint-send-message) com um número de destino e o corpo da mensagem.

---

## 2. Autenticação

A API da Ziett usa **API Keys** para autenticação. Todos os pedidos devem incluir uma API Key válida nos headers. Não existem tokens de sessão, cookies, nem fluxos OAuth para integrações server-to-server.

### A tua API Key

As API Keys são geradas por **Organização** e geridas no [Ziett Dashboard](https://app.ziett.co), em **Organizations → API Keys**.

Cada chave é:

- **Única** — associada a uma única Organização.
- **Com escopo (scoped)** — carrega um conjunto de permissões (ex: `messages:send`, `campaigns:read`) que definem que operações pode executar.
- **Revogável** — pode ser rodada ou apagada a qualquer momento pelo Dashboard, sem afetar outras chaves.

### Fazer pedidos autenticados

Incluir a API Key no header HTTP `X-API-KEY` em cada pedido:

```http
POST /c/v1/messages HTTP/1.1
Host: api.ziett.co
Content-Type: application/json
X-API-KEY: zk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

### Ambientes da API Key

O prefixo da chave indica o ambiente:

| Prefixo    | Ambiente    | Descrição                                                                    |
|------------|-------------|-------------------------------------------------------------------------------|
| `zk_live_` | Produção    | Envia mensagens reais. Há cobrança.                                          |
| `zk_test_` | Teste       | Respostas simuladas apenas. Não são enviadas mensagens nem há cobrança.      |

> Usar sempre a chave de teste durante o desenvolvimento. Nem todos os canais suportam ambiente de teste.

### Escopos (scopes) da chave

Ao criar uma API Key, atribuem-se um ou mais **scopes** que definem exatamente que operações ela pode executar. Pedir uma operação fora dos scopes da chave resulta em erro `403 Forbidden`.

| Scope             | Operações permitidas                          |
|--------------------|------------------------------------------------|
| `messages:send`   | Enviar mensagens individuais                    |
| `messages:read`   | Consultar histórico e estado de entrega         |
| `campaigns:send`  | Criar e submeter batches de campanha            |
| `campaigns:read`  | Consultar estado e relatórios de campanha       |
| `contacts:write`  | Criar, atualizar e importar contactos           |
| `contacts:list`   | Listar e consultar registos de contactos        |
| `templates:list`  | Listar templates de WhatsApp aprovados          |

> **Princípio do menor privilégio:** criar API Keys dedicadas por serviço/microsserviço, atribuindo apenas os scopes que esse serviço precisa. Um serviço de notificações não precisa de `campaigns:send`.

### Boas práticas de segurança

- **Nunca expor a chave em código client-side.** Chaves em JavaScript frontend, apps mobile ou páginas públicas podem ser extraídas por qualquer pessoa. Todas as chamadas devem originar-se do backend.
- **Nunca fazer commit de chaves no controlo de versões.** Usar variáveis de ambiente ou um secrets manager (Cloud Secret Manager, AWS Secrets Manager, HashiCorp Vault, ou `.env` excluído via `.gitignore`).
- **Rodar as chaves regularmente.** Gerar uma nova chave no Dashboard e desativar a antiga sem downtime — recomenda-se rodar sempre que um membro da equipa com acesso saia da organização.
- **Usar chaves separadas por ambiente.** Nunca usar a chave de produção em staging/dev.

### Erros de autenticação

Se um pedido não puder ser autenticado, a API retorna `401 Unauthorized` ou `403 Forbidden`:

```json
{
  "code": "AUTH_INVALID_API_KEY",
  "message": "The provided API key is invalid or has been revoked.",
  "status": 401,
  "trace_id": "a1b2c3d4e5f644358a9e7011c123c831",
  "timestamp": "2025-07-14T10:30:00.000000+00:00",
  "service": "core"
}
```

---

## 3. Rate Limits & Fiabilidade

### Tiers de rate limit

| Endpoint                                  | Limite        | Janela                     |
|--------------------------------------------|---------------|------------------------------|
| `GET /*` (listagem e consulta)             | 300 pedidos   | por minuto, por API Key      |
| `POST /messages` (mensagem única)          | 120 pedidos   | por minuto, por API Key      |
| `POST /campaigns/batch` (envio em massa)   | 10 pedidos    | por minuto, por API Key      |
| `POST /contacts/import` (importação CSV)   | 5 pedidos     | por minuto, por API Key      |

> Os limites são aplicados atualmente **por API Key**. Está prevista migração para limites ao nível da organização.

### Headers de rate limit

Todas as respostas incluem headers para monitorizar o consumo em tempo real (nota: ainda em desenvolvimento, não ativos em todos os endpoints):

| Header                    | Descrição                                                         |
|----------------------------|---------------------------------------------------------------------|
| `X-RateLimit-Limit`       | Número máximo de pedidos permitidos na janela atual.               |
| `X-RateLimit-Remaining`   | Número de pedidos restantes até o limite reiniciar.                |
| `X-RateLimit-Reset`       | Timestamp Unix de quando a janela atual reinicia.                  |

```http
HTTP/1.1 200 OK
X-RateLimit-Limit: 120
X-RateLimit-Remaining: 87
X-RateLimit-Reset: 1752490260
```

### Tratamento de erros 429

Ao exceder um rate limit, a API retorna `429 Too Many Requests`:

```json
{
  "code": "RATE_LIMIT_EXCEEDED",
  "message": "You have exceeded the request limit for this endpoint. Please slow down.",
  "status": 429,
  "trace_id": "e901a2bc77f344358a9e7011c789d012",
  "timestamp": "2025-07-14T11:00:00.000000+00:00",
  "service": "core"
}
```

A resposta inclui também um header `Retry-After` a indicar quantos segundos esperar antes de retentar:

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 38
```

#### Exponential Backoff

Estratégia recomendada para `429` e erros `5xx` transitórios: **exponential backoff com jitter**, para evitar que múltiplos clientes retentem simultaneamente após o reset de uma janela.

```python
import time
import random
import httpx

def send_with_backoff(payload: dict, max_retries: int = 5):
    headers = {"X-API-KEY": "zk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx"}
    delay = 1.0  # delay inicial em segundos

    for attempt in range(max_retries):
        response = httpx.post(
            "https://api.ziett.co/c/v1/messages",
            json=payload,
            headers=headers,
        )

        if response.status_code not in (429, 500, 502, 503, 504):
            return response

        retry_after = response.headers.get("Retry-After")
        if retry_after:
            wait = float(retry_after)
        else:
            # Exponential backoff: 1s, 2s, 4s, 8s, 16s + jitter aleatório
            wait = delay * (2 ** attempt) + random.uniform(0, 1)

        print(f"Attempt {attempt + 1} failed ({response.status_code}). Retrying in {wait:.1f}s...")
        time.sleep(wait)

    raise Exception("Max retries exceeded.")
```

### Webhooks vs. Polling

A Ziett é uma **plataforma assíncrona por design**. A entrega de mensagens acontece através de redes de operadoras e da infraestrutura da Meta — não é instantânea.

- **Polling (não recomendado):** chamar repetidamente `GET /c/v1/campaigns/{id}` até o estado mudar para `COMPLETED` funciona, mas é ineficiente — consome o budget de rate limit, adiciona latência e não escala bem.
- **Webhooks (recomendado):** a Ziett envia eventos de entrega para o servidor em tempo real, assim que o estado muda. Regista-se um endpoint no Dashboard.
  > **Nota:** suporte a webhooks ainda está em desenvolvimento — esta secção será expandida com schemas de eventos completos, verificação de assinatura e instruções de configuração quando disponível.

### Boas práticas para fiabilidade

- **Responder rapidamente a webhooks** — o endpoint deve retornar `2xx` dentro de **5 segundos**. Se o processamento demorar mais, confirmar (ack) o webhook imediatamente e processar o evento de forma assíncrona numa fila em background.
- **Tratar eventos duplicados** — eventos de webhook podem ser entregues mais de uma vez devido a retries de rede. Usar `message_id` ou `campaign_id` como chave de idempotência na base de dados.
- **Guardar valores de `trace_id`** — registar o `trace_id` de cada resposta/erro da API; é a principal ferramenta de debug e a primeira coisa que o suporte da Ziett irá pedir.
- **Monitorizar o saldo de créditos** — configurar alertas de saldo baixo no Dashboard, para que os serviços nunca sejam interrompidos por falta de saldo num momento crítico.

---

## 4. Convenções da API

### Base URL

```text
https://api.ziett.co/c/v1
```

Todos os paths deste documento são relativos a esta base — ex: `/messages` significa `https://api.ziett.co/c/v1/messages`.

**Versionamento:** a versão atual é `v1`. Mudanças que quebrem compatibilidade serão lançadas sob um novo prefixo de versão (ex: `/c/v2`), com aviso prévio e janela de depreciação da versão anterior.

### Pedidos (Requests)

**Verbos HTTP:**

| Método   | Uso                                                                          |
|----------|-------------------------------------------------------------------------------|
| `GET`    | Consultar um recurso ou lista de recursos. Nunca modifica estado.            |
| `POST`   | Criar um novo recurso ou disparar uma operação.                              |
| `PATCH`  | Atualizar parcialmente um recurso existente. Apenas os campos enviados são modificados. |
| `DELETE` | Remover permanentemente um recurso.                                          |

**Content-Type:** todos os bodies devem ser enviados como JSON: `Content-Type: application/json`

**Timestamps e IDs:** todos os timestamps seguem **ISO 8601** em UTC (ex: `2025-07-14T10:30:00.000000+00:00`); todos os identificadores de recurso usam formato **UUID** (ex: `550e8400-e29b-41d4-a716-446655440000`).

### Respostas (Responses)

- **Síncrona — `200 OK`:** operações que completam imediatamente retornam `200 OK` com o recurso diretamente no body.
- **Assíncrona — `202 Accepted`:** operações que envolvem redes externas (operadoras, Meta) são enfileiradas e processadas em background. A API retorna `202 Accepted` imediatamente com um identificador para tracking. Guardar o identificador retornado e usá-lo para consultar o estado ou correlacionar com eventos de webhook — **não fazer polling em loop apertado**, desenhar em torno de callbacks orientados a eventos.
- **Criação de recurso — `201 Created`:** quando um recurso é criado de forma síncrona, a API retorna `201 Created` com a representação completa do novo objeto.

### Erros

A Ziett usa códigos de estado HTTP standard. Códigos `4xx` indicam problema no pedido; códigos `5xx` indicam problema do lado da Ziett (raros — se persistirem, verificar a [status page](https://status.ziett.co)).

**Objeto de erro** — toda resposta de erro segue a mesma estrutura JSON:

| Campo       | Tipo      | Descrição                                                                                          |
|-------------|-----------|-------------------------------------------------------------------------------------------------------|
| `code`      | `string`  | Identificador de erro legível por máquina (ex: `BILLING_INSUFFICIENT_FUNDS`). Usar para tratamento programático. |
| `message`   | `string`  | Descrição legível por humanos do que correu mal.                                                     |
| `status`    | `integer` | O código de estado HTTP.                                                                              |
| `trace_id`  | `string`  | Identificador único deste pedido. Incluir sempre ao contactar o suporte.                             |
| `timestamp` | `string`  | Timestamp ISO 8601 de quando o erro ocorreu.                                                          |
| `service`   | `string`  | O serviço interno da Ziett que gerou o erro (ex: `core`, `billing`).                                 |
| `fields`    | `object`  | *(Opcional)* Presente apenas em erros de validação. Mapeia nomes de campo para mensagens específicas. |

**Exemplo — Validação (422):**

```json
{
  "code": "VALIDATION_ERROR",
  "message": "One or more fields failed validation.",
  "status": 422,
  "trace_id": "f812a3bc91e044358a9e7011c456d901",
  "timestamp": "2025-07-14T10:30:00.000000+00:00",
  "service": "core",
  "fields": {
    "to": "Must be a valid E.164 formatted phone number (e.g., +2449XXXXXXXX).",
    "body": "This field is required and cannot be empty."
  }
}
```

**Códigos de erro comuns:**

| Código                        | Estado | Significado                                                                |
|--------------------------------|--------|-------------------------------------------------------------------------------|
| `AUTH_INVALID_API_KEY`        | 401    | API key ausente, malformada ou revogada.                                    |
| `AUTH_INVALID_SCOPE`          | 403    | A API key não tem o scope necessário para esta operação.                    |
| `RESOURCE_NOT_FOUND`          | 404    | O recurso não existe ou não pertence à tua organização.                     |
| `VALIDATION_ERROR`            | 422    | Um ou mais campos falharam validação. Ver o objeto `fields`.                |
| `BILLING_INSUFFICIENT_FUNDS`  | 402    | Saldo de créditos da conta abaixo do custo desta operação.                  |
| `RATE_LIMIT_EXCEEDED`         | 429    | Demasiados pedidos. Retentar com exponential backoff.                       |
| `INTERNAL_SERVER_ERROR`       | 500    | Erro inesperado do lado da Ziett. Retentar após um curto intervalo.        |

### Paginação

Todos os endpoints `GET` que retornam listas usam um envelope de paginação consistente.

**Parâmetros de query:**

| Parâmetro  | Tipo      | Default      | Máx   | Descrição                          |
|-------------|-----------|---------------|-------|---------------------------------------|
| `page`     | `integer` | `1`          | —     | Número da página a consultar.        |
| `size`     | `integer` | `30`         | `200` | Número de registos por página.       |
| `order_by` | `string`  | `created_at` | —     | Campo pelo qual ordenar os resultados. |
| `order`    | `string`  | `desc`       | —     | Direção da ordenação: `asc` ou `desc`. |

**Envelope de resposta:**

```json
{
  "total": 1250,
  "page": 2,
  "size": 50,
  "pages": 25,
  "entries": [
    { "id": "...", "to": "+2449XXXXXXXX", "status": "delivered" },
    { "id": "...", "to": "+2449XXXXXXXX", "status": "failed" }
  ]
}
```

| Campo     | Tipo      | Descrição                                              |
|------------|-----------|-----------------------------------------------------------|
| `total`   | `integer` | Total de registos correspondentes em todas as páginas.   |
| `page`    | `integer` | O número da página atual.                                 |
| `size`    | `integer` | Registos por página conforme pedido.                       |
| `pages`   | `integer` | Total de páginas disponíveis. Iterar até `page >= pages`. |
| `entries` | `array`   | Os registos da página atual.                               |

**Iterar todos os registos:**

```python
import httpx

headers = {"X-API-KEY": "zk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx"}
all_messages = []
page = 1

while True:
    response = httpx.get(
        "https://api.ziett.co/c/v1/messages",
        headers=headers,
        params={"page": page, "size": 200},
    )
    data = response.json()
    all_messages.extend(data["entries"])

    if page >= data["pages"]:
        break

    page += 1

print(f"Retrieved {len(all_messages)} messages.")
```

### Idempotência

Para pedidos `POST`, pode-se fornecer um header `Idempotency-Key` para retentar em segurança sem risco de processamento duplicado.

```http
POST /c/v1/messages HTTP/1.1
Idempotency-Key: my-unique-request-id-7f3a8bc
X-API-KEY: zk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

Se um pedido com a mesma chave for recebido dentro de uma **janela de 24 horas**, a Ziett retorna a resposta original sem reexecutar a operação. Especialmente útil quando um timeout de rede impede saber se um pedido anterior teve sucesso.

Usar um UUID ou combinação do ID interno do recurso com um timestamp como valor da chave — algo garantidamente único por operação lógica.

---

## 5. Endpoint: Send Message

**`POST` `api.ziett.co/c/v1/messages`**

Submete um pedido para enviar uma única mensagem transacional a um destinatário específico. Desenhado para notificações programáticas de alta fiabilidade e baixa latência — OTPs, alertas de entrega, eventos de conta e outras comunicações sensíveis ao tempo que precisam alcançar um único utilizador imediatamente.

Para enviar a audiências grandes, ver [Send Batch Campaign](#8-endpoint-send-batch-campaign).

### Como funciona

1. **Validação** — a API autentica a chave, verifica o scope `messages:send` e valida todos os campos.
2. **Routing** — o número do destinatário é analisado para identificar a rede da operadora e o país alvo.
3. **Pricing** — o custo para a rota e canal específicos é calculado de acordo com o plano.
4. **Verificação de saldo** — o sistema verifica se o saldo é suficiente; se não, o pedido é rejeitado com `402`.
5. **Enfileiramento** — a mensagem é colocada numa fila de entrega de alta prioridade e despachada para a operadora ou Cloud API da Meta.
6. **Resposta** — `202 Accepted` é retornado imediatamente com um `message_id` para tracking do ciclo de vida.

> `202 Accepted` — e não `200 OK` — é intencional. Significa que a mensagem foi **aceite e enfileirada**, ainda não entregue. Rastrear o estado final via webhook ou via [Retrieve Message](#6-endpoint-retrieve-message).

### Pedido

**Scope necessário:** `apikey:messages:send`

**Body** (`Content-Type: application/json`):

| Campo          | Tipo     | Obrigatório | Descrição                                                                                              |
|-----------------|----------|:-----------:|------------------------------------------------------------------------------------------------------------|
| `remitter_id`  | `uuid`   | ✅          | UUID do Sender ID configurado na organização. Determina o nome/número remetente exibido ao destinatário. |
| `channel_type` | `enum`   | ✅          | Canal de entrega. Valores aceites: `SMS`.                                                                 |
| `target_e164`  | `string` | ✅          | Número de telefone do destinatário em **formato E.164** (ex: `+244990090990`). Deve incluir o código do país. |
| `content`      | `string` | ✅          | Corpo da mensagem. Para SMS, máximo 1600 caracteres — mensagens multi-parte são concatenadas automaticamente. |
| `save_contact` | `object` | ❌          | Se fornecido, faz upsert do destinatário na lista de contactos. Ver [Save Contact Object](#save-contact-object). |

#### Save Contact Object

Quando incluído, o destinatário é criado/atualizado na lista de contactos no momento do envio — útil para construir a audiência organicamente a partir de tráfego transacional.

| Campo   | Tipo             | Obrigatório | Descrição                                                                    |
|---------|------------------|:-----------:|---------------------------------------------------------------------------------|
| `name`  | `string`         | ✅          | Nome completo do contacto.                                                     |
| `email` | `string`         | ❌          | Endereço de email. Deve ser um formato válido.                                |
| `tags`  | `array[string]`  | ❌          | Tags a atribuir (ex: `["Premium", "Angola"]`). Criadas automaticamente se não existirem. |

### Exemplo (Python)

```python
import httpx

response = httpx.post(
    "https://api.ziett.co/c/v1/messages",
    headers={
        "X-API-KEY": "zk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
        "Content-Type": "application/json",
    },
    json={
        "remitter_id": "019b4b56-e704-7c30-83a0-527df63c3e00",
        "channel_type": "SMS",
        "target_e164": "+244990090990",
        "content": "Your verification code is 482910. It expires in 5 minutes.",
    },
)

if response.status_code == 202:
    message_id = response.json()["message_id"]
    print(f"Message accepted. Tracking ID: {message_id}")
else:
    print(f"Error: {response.json()}")
```

### Respostas

**`202 Accepted` — Mensagem enfileirada**

A mensagem foi validada, a conta debitada, e a mensagem colocada na fila de entrega.

```json
{
  "message_id": "019b4b56-e704-7c30-83a0-527df63c3e00"
}
```

| Campo        | Tipo   | Descrição                                                                                    |
|---------------|--------|--------------------------------------------------------------------------------------------------|
| `message_id` | `uuid` | Identificador único desta mensagem. Guardar para consultar o estado de entrega ou correlacionar com eventos de webhook. |

**`401 Unauthorized` — API Key inválida**

```json
{
  "code": "AUTH_INVALID_API_KEY",
  "message": "The provided API key is invalid or has been revoked.",
  "status": 401,
  "trace_id": "a1b2c3d4e5f644358a9e7011c123c831",
  "timestamp": "2025-07-14T10:30:00.000000+00:00",
  "service": "core"
}
```

**`402 Payment Required` — Saldo insuficiente**

```json
{
  "code": "BILLING_INSUFFICIENT_FUNDS",
  "message": "Your account balance is too low to process this request. Please top up your credits.",
  "status": 402,
  "trace_id": "c109f78ae57744358a9e7011c123c831",
  "timestamp": "2025-07-14T10:30:00.000000+00:00",
  "service": "billing"
}
```

**`422 Unprocessable Entity` — Erro de validação**

```json
{
  "code": "VALIDATION_ERROR",
  "message": "One or more fields failed validation.",
  "status": 422,
  "trace_id": "f812a3bc91e044358a9e7011c456d901",
  "timestamp": "2025-07-14T10:30:00.000000+00:00",
  "service": "core",
  "fields": {
    "target_e164": "Must be a valid E.164 formatted phone number (e.g., +244990090990).",
    "channel_type": "Accepted values are: SMS, WHATSAPP."
  }
}
```

### Rate Limits

Limitado a **120 pedidos por minuto** por API Key.

### Endpoints relacionados

- [Retrieve Message](#6-endpoint-retrieve-message) — obter o estado de entrega de uma mensagem específica por `message_id`.
- [List Messages](#7-endpoint-list-messages) — obter histórico paginado de todas as mensagens enviadas pela organização.
- [Send Batch Campaign](#8-endpoint-send-batch-campaign) — enviar para uma audiência grande num único pedido.

---

## 6. Endpoint: Retrieve Message

**`GET` `api.ziett.co/c/v1/messages/{message_id}`**

Obtém os detalhes completos de execução e routing de uma mensagem específica pelo seu identificador único. Essencial para auditar comunicações individuais, inspecionar custos de transação, rastrear timestamps de entrega da operadora ou depurar falhas de entrega via códigos de erro de rede.

Para consultar múltiplos registos com filtros, ver [List Messages](#7-endpoint-list-messages).

### Pedido

**Scope necessário:** `apikey:messages:read`

**Path Parameters:**

| Parâmetro    | Tipo   | Descrição                                                                    |
|---------------|--------|----------------------------------------------------------------------------------|
| `message_id` | `uuid` | O identificador único UUID v7 de tracking atribuído à mensagem na ingestão.    |

### Exemplo (Python)

```python
import httpx

message_id = "019b4b56-e704-7c30-83a0-527df63c3e00"

response = httpx.get(
    f"https://api.ziett.co/c/v1/messages/{message_id}",
    headers={
        "X-API-KEY": "zk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    }
)

print(response.json())
```

### Respostas

**`200 OK` — Detalhes da mensagem obtidos**

```json
{
  "id": "019b4b56-e704-7c30-83a0-527df63c3e00",
  "organization_id": "019b4b56-ab10-7210-81a1-527df63c3111",
  "billing_account_id": "019b4b56-bc20-7315-92b2-627df63c3222",
  "campaign_id": "019b4b56-cd30-7420-a3c3-727df63c3333",
  "contact_id": "019b4b56-fa21-7210-91b1-127df63c3eef",
  "sms_remitter_id": "019b4b56-de40-7530-b4d4-827df63c3444",
  "sms_remitter": {
    "id": "019b4b56-de40-7530-b4d4-827df63c3444",
    "name": "InfoZiett"
  },
  "external_id": "client_ref_981242",
  "channel_type": "SMS",
  "channel_destination": "UNITEL",
  "provider_name": "UNITEL_DIRECT",
  "e164_format": "+244990090990",
  "content": "Your validation protocol code is 99201.",
  "trigger_source": "API",
  "cost": "15.000000",
  "is_refunded": false,
  "status": "DELIVERED",
  "error_code": null,
  "created_at": "2026-05-14T10:30:00.000000+00:00",
  "updated_at": "2026-05-14T10:30:04.000000+00:00"
}
```

**Campos da resposta:**

| Campo                  | Tipo               | Descrição                                                                                  |
|-------------------------|--------------------|-------------------------------------------------------------------------------------------------|
| `id`                   | `uuid`             | Identificador único UUID v7 de tracking desta mensagem.                                       |
| `organization_id`      | `uuid`             | ID da organização proprietária que disparou esta comunicação.                                 |
| `billing_account_id`   | `uuid`             | A conta de saldo específica debitada por esta transação.                                      |
| `campaign_id`          | `uuid` \| `null`   | Preenchido se este envio individual pertencer a uma execução de batch (campanha) pai.        |
| `contact_id`           | `uuid` \| `null`   | Token de referência apontando para o registo interno de CRM do destinatário.                  |
| `sms_remitter_id`      | `uuid` \| `null`   | O identificador exato do Sender ID aprovado usado na entrega.                                 |
| `sms_remitter`         | `object` \| `null` | Sub-objeto embutido com os parâmetros básicos de nome do Sender ID de SMS.                    |
| `external_id`          | `string` \| `null` | Referência de lookup opcional passada pelo sistema do cliente na ingestão.                    |
| `channel_type`         | `string`           | Classificação do formato/meio do canal da mensagem (`SMS`, `WHATSAPP`).                       |
| `channel_destination`  | `string`           | O mapa de routing do operador de rede de destino (ex: `UNITEL`, `AFRICELL_AO`).               |
| `provider_name`        | `string` \| `null` | Handler interno do gateway de integração parceiro usado para entregar o payload pela rede.    |
| `e164_format`          | `string`           | Número de destino do destinatário, normalizado segundo as regras do formato E.164.            |
| `content`              | `string`           | O conteúdo em texto despachado pela linha da operadora.                                       |
| `trigger_source`       | `string`           | Vetor de contexto de origem do pipeline de execução (`API_CLIENT`, `CUSTOMER_PORTAL`).       |
| `cost`                 | `string`           | Custo financeiro formatado em decimal, consumido do saldo da conta para processar a mensagem. |
| `is_refunded`          | `boolean`          | Indica se uma falha de entrega reverteu a transação de saldo de volta para a carteira.       |
| `status`               | `string`           | Estado real da rede em tempo real (`PENDING`, `SENT`, `DELIVERED`, `FAILED`, `UNDELIVERED`, `EXPIRED`, `REJECTED`, `CANCELLED`). |
| `error_code`           | `string` \| `null` | Identificador de erro categorizado indicando o motivo do bloqueio de entrega.                 |
| `created_at`           | `datetime`         | Timestamp de quando a transação foi recebida pela API.                                        |
| `updated_at`           | `datetime`         | Timestamp da última atualização de tracking ou transição de estado da operadora.              |

**`401 Unauthorized` — API Key inválida**

```json
{
  "code": "AUTH_INVALID_API_KEY",
  "message": "The provided API key is invalid or has been revoked.",
  "status": 401,
  "trace_id": "b2c3d4e5f644358a9e7011c123c831a1",
  "timestamp": "2026-05-14T10:30:00.000000+00:00",
  "service": "core"
}
```

**`404 Not Found` — Mensagem não encontrada**

```json
{
  "code": "NOT_FOUND",
  "message": "The requested message resource could not be found.",
  "status": 404,
  "trace_id": "c3d4e5f644358a9e7011c123c831a1b2",
  "timestamp": "2026-05-14T10:30:00.000000+00:00",
  "service": "core"
}
```

**`422 Unprocessable Entity` — Erro de validação**

```json
{
  "code": "VALIDATION_ERROR",
  "message": "One or more fields failed validation.",
  "status": 422,
  "trace_id": "f812a3bc91e044358a9e7011c456d901",
  "timestamp": "2026-05-14T10:30:00.000000+00:00",
  "service": "core",
  "fields": {
    "message_id": "Value must conform to a valid UUID v7 format rule pattern structure."
  }
}
```

### Endpoints relacionados

- [List Messages](#7-endpoint-list-messages) — navegar e auditar dados históricos com múltiplos filtros.
- [Send Message](#5-endpoint-send-message) — disparar uma nova transação de envio de alta prioridade.

---

## 7. Endpoint: List Messages

**`GET` `api.ziett.co/c/v1/messages`**

Obtém o histórico paginado de todas as mensagens enviadas pela organização. Fornece visibilidade profunda do tráfego transacional e de marketing, permitindo auditar estados de entrega, rastrear logs de routing e filtrar comunicações por múltiplas variáveis.

Para encontrar uma mensagem única instantaneamente já com o identificador de tracking, ver [Retrieve Message](#6-endpoint-retrieve-message).

### Pedido

**Scope necessário:** `apikey:messages:list`

**Query Parameters:**

| Parâmetro          | Tipo             | Default      | Descrição                                                                                                                 |
|----------------------|------------------|---------------|--------------------------------------------------------------------------------------------------------------------------------|
| `q`                 | `string`         | —            | Query de busca por palavra-chave. Corresponde parcialmente a números (`target_e164`) ou exatamente ao UUID da mensagem.       |
| `page`              | `integer`        | `1`          | Índice da página a consultar. Mínimo `1`.                                                                                     |
| `size`              | `integer`        | `30`         | Número de registos por página. Mínimo `1`, máximo `200`.                                                                       |
| `order`             | `enum`           | `desc`       | Direção da ordenação: `asc` (mais antigos primeiro), `desc` (mais recentes primeiro).                                        |
| `order_by`          | `enum`           | `created_at` | Atributo usado para ordenar. Valores: `created_at`, `updated_at`.                                                             |
| `status`            | `enum \| array`  | —            | Filtrar por estado(s) de entrega. Valores: `PENDING`, `SENT`, `DELIVERED`, `FAILED`, `UNDELIVERED`. Pode repetir o parâmetro para múltiplos estados (ex: `status=SENT&status=FAILED`). |
| `channel_type`      | `enum \| array`  | —            | Filtrar por canal de transporte. Valores: `SMS`, `WHATSAPP`. Pode ser repetido.                                              |
| `contact_id`        | `uuid \| array`  | —            | Filtrar histórico de um ou mais contactos específicos.                                                                        |
| `campaign_id`       | `uuid \| array`  | —            | Filtrar mensagens geradas por uma campanha (batch) específica.                                                               |
| `sms_remitter_id`   | `uuid \| array`  | —            | Filtrar pelo Sender ID (Remitter UUID) específico usado para despachar o tráfego.                                            |
| `created_at__ge`    | `datetime`       | —            | Limite inferior do intervalo de datas em **ISO 8601** (inclusivo). Ex: `2026-01-01T00:00:00Z`.                                |
| `created_at__le`    | `datetime`       | —            | Limite superior do intervalo de datas em **ISO 8601** (inclusivo). Ex: `2026-01-31T23:59:59Z`.                                |

### Exemplo (Python)

```python
import httpx

# Exemplo: obter a primeira página de mensagens SMS falhadas
response = httpx.get(
    "https://api.ziett.co/c/v1/messages",
    headers={
        "X-API-KEY": "zk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    },
    params={
        "page": 1,
        "size": 30,
        "status": ["FAILED", "UNDELIVERED"],
        "channel_type": "SMS",
    },
)

print(response.json())
```

### Respostas

**`200 OK` — Lista paginada obtida**

```json
{
  "entries": [
    {
      "id": "019b4b56-e704-7c30-83a0-527df63c3e00",
      "remitter_id": "019b4b56-e704-7c30-83a0-527df63c3e00",
      "campaign_id": null,
      "contact_id": "019b4b56-fa21-7210-91b1-127df63c3eef",
      "channel_type": "SMS",
      "channel_destination": "UNITEL",
      "target_e164": "+244990090990",
      "content": "Your verification code is 482910. It expires in 5 minutes.",
      "status": "DELIVERED",
      "error_code": null,
      "created_at": "2026-05-14T10:30:00.000000+00:00",
      "updated_at": "2026-05-14T10:30:04.000000+00:00"
    }
  ],
  "total": 1420,
  "page": 1,
  "size": 30,
  "pages": 48
}
```

**Campos do envelope:**

| Campo     | Tipo             | Descrição                                                                     |
|------------|-------------------|-----------------------------------------------------------------------------------|
| `entries` | `array[object]`  | Lista de detalhes de mensagens correspondentes aos critérios (ver abaixo).       |
| `total`   | `integer`        | Número absoluto de registos correspondentes aos filtros, globalmente.           |
| `page`    | `integer`        | O índice da página atual retornada.                                              |
| `size`    | `integer`        | O número de registos processados por página.                                     |
| `pages`   | `integer`        | Total de páginas acessíveis, calculado a partir de `total` e `size`.            |

**Objeto de detalhe da mensagem:**

| Campo                  | Tipo       | Descrição                                                                                    |
|--------------------------|------------|----------------------------------------------------------------------------------------------------|
| `id`                    | `uuid`     | Identificador único de tracking desta mensagem.                                                   |
| `remitter_id`           | `uuid`     | Assinatura de identidade/Sender ID usado para emitir a notificação.                              |
| `campaign_id`           | `uuid`     | `null` se não pertencer a uma campanha.                                                           |
| `contact_id`            | `uuid`     | `null` se não associado a um contacto.                                                            |
| `channel_type`          | `string`   | Canal de entrega (`SMS`, `WHATSAPP`).                                                             |
| `channel_destination`   | `string`   | Operador de rede/handler de entrega resolvido (ex: `UNITEL`, `AFRICELL_AO`).                     |
| `target_e164`           | `string`   | Número de destino internacionalizado formatado segundo E.164.                                    |
| `content`               | `string`   | O payload de conteúdo da mensagem despachado pela rede.                                          |
| `status`                | `string`   | Estado operacional final ou ativo (`PENDING`, `SENT`, `DELIVERED`, `FAILED`, `UNDELIVERED`).      |
| `error_code`            | `string`   | `null` se não houver erro.                                                                        |
| `created_at`            | `datetime` | Timestamp de quando o pedido de notificação foi ingerido e armazenado.                            |
| `updated_at`            | `datetime` | Timestamp da atualização de estado ou receção de entrega mais recente.                            |

**`401 Unauthorized` — API Key inválida**

```json
{
  "code": "AUTH_INVALID_API_KEY",
  "message": "The provided API key is invalid or has been revoked.",
  "status": 401,
  "trace_id": "a1b2c3d4e5f644358a9e7011c123c831",
  "timestamp": "2026-05-14T10:30:00.000000+00:00",
  "service": "core"
}
```

**`422 Unprocessable Entity` — Erro de validação**

```json
{
  "code": "VALIDATION_ERROR",
  "message": "One or more fields failed validation.",
  "status": 422,
  "trace_id": "f812a3bc91e044358a9e7011c456d901",
  "timestamp": "2026-05-14T10:30:00.000000+00:00",
  "service": "core",
  "fields": {
    "size": "Ensure this value is less than or equal to 200.",
    "created_at__ge": "Value must conform to a valid ISO 8601 date-time format structure."
  }
}
```

### Rate Limits

Limitado a **60 pedidos por minuto** por API Key.

---

## 8. Endpoint: Send Batch Campaign

**`POST` `api.ziett.co/c/v1/campaigns/batch`**

Inicia uma campanha de mensagens de alto throughput para múltiplos destinatários. Em vez de fazer centenas de pedidos HTTP individuais, este endpoint permite submeter um payload em batch. O sistema processa a lista, valida os números e enfileira para entrega de forma assíncrona.

Para enviar um único alerta sensível ao tempo (como um OTP), usar [Send Message](#5-endpoint-send-message).

### Como funciona

Como o processamento em batch lida com centenas de números simultaneamente, este endpoint opera inteiramente de forma assíncrona para evitar timeouts HTTP.

1. **Criação** — a API aceita imediatamente o payload e cria um registo de campanha com estado `PROCESSING`.
2. **Processamento** — workers em background fazem parse da lista de destinatários, formatam os números, calculam as rotas e debitam o custo total do saldo da conta de faturação da organização.
3. **Enfileiramento** — objetos de mensagem individuais são gerados e enviados para as filas de entrega específicas de cada provider.
4. **Envio** — quando todas as mensagens estiverem enfileiradas com sucesso, o estado da campanha transita para `SENDING`.
5. **Conclusão** — outro processo do sistema monitoriza a entrega das mensagens e marca a campanha como concluída (`COMPLETED`) quando todas as mensagens tiverem sido `SENT`.

> `202 Accepted` é retornado logo após o passo 1. Deve-se usar webhooks para ser notificado quando a campanha atingir o estado `COMPLETED`, ou acompanhar o progresso via [Retrieve Campaign](#9-endpoint-retrieve-campaign).

### Pedido

**Scope necessário:** `apikey:campaign:send`

**Body** (`Content-Type: application/json`):

| Campo             | Tipo             | Obrigatório | Descrição                                                                                                                                  |
|---------------------|------------------|:-----------:|--------------------------------------------------------------------------------------------------------------------------------------------|
| `remitter_id`      | `uuid`           | ✅          | O Sender ID aprovado usado para emitir a campanha. **Regra:** deve ser um UUID v7 válido.                                                 |
| `content`          | `string`         | ✅          | O conteúdo de texto da mensagem. Para SMS, mensagens longas são divididas automaticamente. **Regra:** mínimo `1`, máximo `10.000` caracteres. |
| `channel_type`     | `enum`           | ✅          | O canal de comunicação usado para entregar o batch. **Regra:** valor aceite é `SMS`.                                                       |
| `recipients`       | `array[string]`  | ✅          | Lista de números de telefone de destino. Aceita formatos locais ou E.164 internacional. **Regras:** array de `2` a `1000` itens; regex por item: `^\+?[\d\s\-\.]{7,30}$`. |
| `name`             | `string`         | ❌          | Nome interno amigável para identificar a campanha no dashboard. **Regra:** mínimo `1`, máximo `200` caracteres.                            |
| `country_alpha2`   | `string`         | ❌          | Código ISO 3166-1 alpha-2 especificando o país padrão para números locais sem prefixo internacional (ex: `AO`, `PT`). **Regra:** exatamente `2` caracteres, regex `^[A-Z]{2}$`. |

### Exemplo (Python)

```python
import httpx

response = httpx.post(
    "https://api.ziett.co/c/v1/campaigns/batch",
    headers={
        "X-API-KEY": "zk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
        "Content-Type": "application/json",
    },
    json={
        "name": "Black Friday Promo - Luanda",
        "remitter_id": "019b4b56-e704-7c30-83a0-527df63c3e00",
        "channel_type": "SMS",
        "country_alpha2": "AO",
        "content": "Flash Sale! Enjoy 50% off on all items this weekend only.",
        "recipients": [
            "+244990000001",
            "+244990000002",
            "990000003"  # será parseado usando country_alpha2 'AO'
        ]
    },
)

if response.status_code == 202:
    print("Batch accepted. Processing asynchronously.")
else:
    print(f"Error: {response.json()}")
```

### Respostas

**`202 Accepted` — Campanha enfileirada**

```json
{
  "message": "Campaign accepted and is pending processing.",
  "campaign_id": "019b4b56-e704-7c30-83a0-527df63c3e00"
}
```

*(Guardar o `campaign_id` retornado para consultar o estado depois ou correlacionar com webhooks recebidos.)*

**`401 Unauthorized` — API Key inválida**

```json
{
  "code": "AUTH_INVALID_API_KEY",
  "message": "The provided API key is invalid or has been revoked.",
  "status": 401,
  "trace_id": "a1b2c3d4e5f644358a9e7011c123c831",
  "timestamp": "2026-05-14T10:30:00.000000+00:00",
  "service": "core"
}
```

**`402 Payment Required` — Saldo insuficiente**

```json
{
  "code": "BILLING_INSUFFICIENT_FUNDS",
  "message": "Your account balance is too low to process this batch request. Please top up your credits.",
  "status": 402,
  "trace_id": "c109f78ae57744358a9e7011c123c831",
  "timestamp": "2026-05-14T10:30:00.000000+00:00",
  "service": "billing"
}
```

**`422 Unprocessable Entity` — Erro de validação**

```json
{
  "code": "VALIDATION_ERROR",
  "message": "One or more fields failed validation.",
  "status": 422,
  "trace_id": "f812a3bc91e044358a9e7011c456d901",
  "timestamp": "2026-05-14T10:30:00.000000+00:00",
  "service": "core",
  "fields": {
    "recipients": "Ensure this value has at most 1000 items.",
    "country_alpha2": "String should match pattern '^[A-Z]{2}$'."
  }
}
```

### Rate Limits

Devido às operações intensivas de base de dados necessárias para processar batches, este endpoint é estritamente limitado a **10 pedidos por minuto** por API Key. Garantir estratégias de backoff adequadas.

### Endpoints relacionados

- [Send Message](#5-endpoint-send-message) — disparar uma única mensagem transacional instantaneamente.
- [Retrieve Campaign](#9-endpoint-retrieve-campaign) — obter o estado de execução e analytics de uma campanha.

---

## 9. Endpoint: Retrieve Campaign

**`GET` `api.ziett.co/c/v1/campaigns/{campaign_id}`**

Obtém as especificações detalhadas de configuração, timestamps de estado de execução e registos de metadados de configuração para uma única instância de pipeline de campanha em batch. É o endpoint de diagnóstico fundamental para fazer polling seguro de estados de ingestão assíncrona ou obter resumos de dados de auditoria logo após concluir distribuições em massa.

### Pedido

**Scope necessário:** `apikey:campaign:read`

**Path Parameters:**

| Parâmetro      | Tipo   | Descrição                                                                        |
|-----------------|--------|--------------------------------------------------------------------------------------|
| `campaign_id`  | `uuid` | O token de tracking único atribuído ao recurso de pipeline da campanha em batch.   |

### Exemplo (Python)

```python
import httpx

campaign_id = "019b4b56-e704-7c30-83a0-527df63c3e00"

response = httpx.get(
    f"https://api.ziett.co/c/v1/campaigns/{campaign_id}",
    headers={
        "X-API-KEY": "zk_live_xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
    }
)

print(response.json())
```

### Respostas

**`200 OK` — Detalhes da campanha obtidos**

```json
{
  "id": "019b4b56-e704-7c30-83a0-527df63c3e00",
  "name": "Black Friday Promo - Luanda",
  "status": "COMPLETED",
  "organization_id": "019b4b56-ab10-7210-81a1-527df63c3111",
  "billing_account_id": "019b4b56-bc20-7315-92b2-627df63c3222",
  "trigger_source": "API",
  "file_metadata_id": null,
  "channel_type": "SMS",
  "sms_remitter_id": "019b4b56-de40-7530-b4d4-827df63c3444",
  "sms_remitter": {
    "id": "019b4b56-de40-7530-b4d4-827df63c3444",
    "name": "InfoZiett"
  },
  "content": "Flash Sale! Enjoy 50% off on all items this weekend only.",
  "total_estimated_recipients": 3,
  "scheduled_at": null,
  "processed_at": "2026-06-02T14:00:02.000000+00:00",
  "completed_at": "2026-06-02T14:02:15.000000+00:00",
  "error_code": null,
  "status_note": "All recipient segments parsed, compiled, and handed over to carrier routing lines successfully.",
  "data": {
    "reason_of_pause": "string"
  },
  "created_at": "2026-06-02T14:00:00.000000+00:00",
  "updated_at": "2026-06-02T14:02:15.000000+00:00"
}
```

**Campos da resposta:**

| Campo                          | Tipo                  | Descrição                                                                                                     |
|----------------------------------|-----------------------|--------------------------------------------------------------------------------------------------------------------|
| `id`                            | `uuid`                | UUID v7 de tracking do sistema, único para a campanha.                                                          |
| `name`                          | `string`               | Rótulo personalizado amigável identificando a execução do envio em batch.                                       |
| `status`                        | `enum`                 | Estado de progresso do workflow ativo. Valores: `DRAFT`, `SCHEDULED`, `PROCESSING`, `SENDING`, `COMPLETED`, `CANCELLED`, `FAILED`, `PAUSED`. |
| `organization_id`               | `uuid`                | Token de contexto de identidade da organização que hospeda este recurso.                                        |
| `billing_account_id`            | `uuid`                | O identificador de configuração de saldo (ledger) responsável por cobrir as deduções financeiras.               |
| `trigger_source`                | `enum`                 | Vetor de tracking de contexto de entrada do pipeline, indicando como o job foi submetido.                       |
| `file_metadata_id`              | `uuid` \| `null`      | Token de referência relacional indicando uma lista carregada via dashboard, se o job veio de um CSV importado.  |
| `channel_type`                  | `enum`                 | O meio/camada de sistema de comunicação usado para o envio. Valores: `SMS`.                                     |
| `sms_remitter_id`               | `uuid` \| `null`      | Token de routing do remetente alfanumérico/numérico configurado.                                                 |
| `sms_remitter`                  | `object` \| `null`    | Detalhes estruturados com tags de mascaramento alfanumérico do remetente ou configurações ativas.                |
| `content`                       | `string`               | O corpo de texto do template do payload despachado para todos os números alvo.                                  |
| `total_estimated_recipients`    | `integer`              | Métricas de contagem documentando o volume de variáveis de destinatários alvo ingeridas inicialmente.           |
| `scheduled_at`                  | `datetime` \| `null`  | Timestamp ISO definindo uma data futura de execução, se a entrega foi pausada para submissão automática.        |
| `processed_at`                  | `datetime` \| `null`  | Timestamp documentando quando as verificações de validação concluíram e os cálculos de saldo foram finalizados. |
| `completed_at`                  | `datetime` \| `null`  | Timestamp documentando quando os workers em background terminaram de enfileirar o último item do payload.       |
| `error_code`                    | `enum` \| `null`      | Código de falha de categoria global sinalizando problemas operacionais que possam ter interrompido o processamento. |
| `status_note`                   | `string` \| `null`    | Informação de log descritiva detalhando tarefas atuais, alertas de otimização ou avisos de execução.             |
| `data`                          | `object`               | Payload de tracking embutido com métricas em tempo real, detalhes de contabilidade e contagens de processamento de transações. |
| `created_at`                    | `datetime`             | Metadado de criação precisa indicando quando a estrutura do pedido original foi armazenada.                      |
| `updated_at`                    | `datetime` \| `null`  | Marcador de tracking documentando a última mudança de estado das propriedades internas da base de dados.        |

**`401 Unauthorized` — API Key inválida**

```json
{
  "code": "AUTH_INVALID_API_KEY",
  "message": "The provided API key is invalid or has been revoked.",
  "status": 401,
  "trace_id": "d4e5f644358a9e7011c123c831a1b2c3",
  "timestamp": "2026-06-02T14:00:00.000000+00:00",
  "service": "core"
}
```

**`404 Not Found` — Campanha não encontrada**

```json
{
  "code": "NOT_FOUND",
  "message": "The requested campaign resource tracking profile could not be found.",
  "status": 404,
  "trace_id": "e5f644358a9e7011c123c831a1b2c3d4",
  "timestamp": "2026-06-02T14:00:00.000000+00:00",
  "service": "core"
}
```

**`422 Unprocessable Entity` — Erro de validação**

```json
{
  "code": "VALIDATION_ERROR",
  "message": "One or more fields failed validation.",
  "status": 422,
  "trace_id": "f812a3bc91e044358a9e7011c456d901",
  "timestamp": "2026-06-02T14:00:00.000000+00:00",
  "service": "core",
  "fields": {
    "campaign_id": "Value must conform to a valid UUID v7 format rule pattern structure."
  }
}
```

### Endpoints relacionados

- [Send Batch Campaign](#8-endpoint-send-batch-campaign) — submeter um payload com até 1.000 linhas de destinatários.

---

## Notas Gerais

- **Base URL:** `https://api.ziett.co/c/v1`
- **Autenticação:** header `X-API-KEY`, chaves com prefixo `zk_live_` (produção) ou `zk_test_` (teste)
- **Idempotência:** header `Idempotency-Key`, janela de 24h
- **Webhooks:** ainda em desenvolvimento à data desta documentação — recomendado desenhar a integração já pensando em consumir eventos assíncronos assim que disponíveis
- **Suporte:** https://ziett.co/contact | Status page: https://status.ziett.co
