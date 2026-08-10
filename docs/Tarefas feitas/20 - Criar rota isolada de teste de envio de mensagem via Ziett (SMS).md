---
criado: 2026-08-10 00:00
origem: solicitação do usuário
status: feito
---

# Criar rota isolada de teste de envio de mensagem via Ziett (SMS) (feito)

## Prompt recomendado para executar a atualização

Implemente uma rota isolada, autenticada, que envie uma mensagem de teste através da API da Ziett (`docs/Parceiros e integrações/Ziett API - Documentação.md`, endpoint `POST /messages`), com `channel_type` sempre fixo em `"SMS"`. Esta rota é exclusivamente um utilitário de teste de conectividade com a API externa da Ziett: **não deve, em nenhuma hipótese, ser integrada ao Event Sourcing/CQRS, ao ledger de auditoria (`spuri_ledger`), a nenhuma projeção, migration ou aggregate existente.** Toda a lógica deve viver isolada em arquivos novos e dedicados — nunca misturada com lógica de outro domínio (financeiro, matrícula, estudante, academia, etc.) em um mesmo arquivo — e nenhum arquivo relacionado ao fluxo de inscrição/matrícula de estudante numa academia deve ser alterado ou removido para esta tarefa. Ao final, valide com `go build ./...`, `go vet ./...` e `gofmt -l` antes de considerar a tarefa concluída, e atualize `Documentação da API.md` e `.env.example` conforme especificado abaixo.

## Contexto

A Ziett é uma CPaaS (Communications Platform as a Service) usada para envio de SMS/WhatsApp, com documentação completa já extraída para `docs/Parceiros e integrações/Ziett API - Documentação.md`. O objetivo desta tarefa não é integrar SMS ao sistema de notificações do Spuri como um todo (isso é trabalho futuro, referenciado no roadmap de notificações por SMS), mas apenas criar uma rota isolada e mínima que permita **testar** se o backend consegue autenticar e enviar uma mensagem real (ou de teste, dependendo da API Key usada) através da Ziett, validando a integração ponta a ponta antes de qualquer trabalho de integração mais profunda.

Por ser um endpoint de teste que dispara SMS real (com custo, quando usada uma API Key `zk_live_`) através de um serviço de terceiros, a rota deve ficar restrita a administradores de mais alto nível e completamente desacoplada de qualquer efeito colateral no domínio do Spuri (nenhum evento, nenhuma projeção, nenhuma linha em `spuri_ledger`).

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Nova rota | `POST /integracoes/ziett/mensagens/teste` | Rota isolada, fora de qualquer grupo de domínio existente |
| Autenticação | `AuthMiddleware()` + `RequireFPP()` | Apenas admins do maior nível hierárquico podem disparar SMS de teste com custo real |
| Variável de ambiente | `ZIETT_API_KEY` | Chave lida via `os.Getenv`, nunca hardcoded, nunca logada em texto plano |
| Isolamento de domínio | Nenhuma dependência de `internal/db`, `internal/domain`, `internal/projections`, `internal/finance` | Sem gravação de evento, sem leitura/escrita em `spuri_ledger`, sem projeção afetada |
| `channel_type` | Sempre `"SMS"`, fixo no backend | Payload do cliente não pode alterar este valor |
| `target_e164` | Recebido apenas como número nacional angolano de 9 dígitos, sem `"0"` inicial e sem `"+244"` | Backend monta o E.164 completo (`+244XXXXXXXXX`) antes de enviar à Ziett |

---

# 1. Variável de ambiente para a API Key

## Objetivo

Permitir configurar a API Key da Ziett (header `X-API-KEY` exigido pela Ziett) sem hardcode no código-fonte.

## Regra de negócio

1. Criar a variável de ambiente `ZIETT_API_KEY`, lida via `os.Getenv("ZIETT_API_KEY")` no momento em que o cliente da Ziett for construído (não em tempo de `init()` global — leitura tardia evita problemas de ordem de carregamento do `.env`).
2. Se `ZIETT_API_KEY` estiver vazia, a rota deve responder `503 Service Unavailable` com mensagem clara orientando a configurar a variável, sem tentar chamar a API da Ziett.
3. Adicionar a variável a `.env.example`, em uma nova seção dedicada (ex.: `# Ziett (CPaaS de SMS) — rota isolada de teste de integração`), com comentário explicando os prefixos `zk_test_` (sem custo/sem envio real) e `zk_live_` (produção, com custo), e citando explicitamente que essa variável só é usada por esta rota isolada.
4. Não reutilizar nem renomear nenhuma variável de ambiente já existente (`EMAILJS_*`, `MEGA_*`, `FINANCE_ENCRYPTION_KEY`, etc.) — `ZIETT_API_KEY` é uma variável nova e exclusiva desta integração.

---

# 2. Cliente HTTP isolado para a API da Ziett

## Objetivo

Criar um cliente HTTP dedicado, num arquivo novo e isolado, responsável apenas por montar e enviar a requisição `POST /messages` à Ziett e interpretar a resposta.

## Escopo obrigatório

### 2.1 Localização e isolamento

- Criar o cliente em um arquivo novo, dedicado exclusivamente à Ziett (ex.: dentro de `internal/services/`, mas em arquivo próprio, sem misturar com `email_service.go` ou `matricula_validation.go`).
- O cliente não pode importar `internal/db`, `internal/domain/aggregates`, `internal/projections`, `internal/finance` nem qualquer pacote que dependa de banco de dados. Deve depender apenas da biblioteca padrão (`net/http`, `encoding/json`, etc.) e, no máximo, de utilitários genéricos já existentes que não pertençam ao domínio de estudante/matrícula/academia.
- Base URL fixa conforme a documentação da Ziett (seção "4. Convenções da API"): `https://api.ziett.co/c/v1`. Não é necessário torná-la configurável por variável de ambiente — apenas a API Key varia por ambiente.

### 2.2 Formatação de `target_e164`

O payload da rota recebe `target_e164` **sem** o dígito `"0"` inicial e **sem** `"+244"` — apenas o número nacional angolano de 9 dígitos (ex.: `"923456789"`). O cliente/serviço deve conter uma função dedicada de formatação que:

1. Remova espaços, hífens e parênteses.
2. Rejeite valores vazios com erro de validação claro.
3. De forma defensiva (robustez contra payload malformado, ainda que o contrato oficial não inclua esses prefixos), remova um prefixo `"+244"` ou `"244"` recebido por engano, e um único `"0"` inicial recebido por engano, antes da validação final.
4. Valide que o resultado final tenha exatamente 9 dígitos numéricos, iniciados em `"9"` (padrão de rede móvel angolana). Caso contrário, retornar erro de validação explicando o formato esperado, com exemplo (`"923456789"`).
5. Retorne o número final já no formato E.164 completo: `"+244" + 9 dígitos`.

Esta função de formatação deve ser local a este módulo isolado — **não reutilizar nem modificar** `internal/utils/validation.go` (que contém `NormalizePhone`/`ValidatePhoneStrictNational`, usados pelos fluxos de estudante/academia). Mantenha a rota 100% isolada, sem dependência cruzada com esse arquivo.

### 2.3 Envio da mensagem

Implementar uma função que envie `POST {base_url}/messages` com:

- Header `Content-Type: application/json`.
- Header `X-API-KEY: {valor de ZIETT_API_KEY}`.
- Corpo JSON exatamente com os campos exigidos pela Ziett (seção "5. Endpoint: Send Message" da documentação):

```json
{
  "remitter_id": "uuid do Sender ID, recebido do payload da nossa rota",
  "channel_type": "SMS",
  "target_e164": "+244XXXXXXXXX (já formatado pela função da seção 2.2)",
  "content": "corpo da mensagem, recebido do payload da nossa rota"
}
```

`channel_type` deve ser fixado em `"SMS"` diretamente no código do cliente — o payload recebido pela nossa rota isolada não deve conter nem sobrescrever este campo.

### 2.4 Tratamento de resposta

- **`202 Accepted`**: sucesso. Extrair `message_id` do corpo da resposta e retornar ao chamador.
- **Qualquer outro status**: interpretar o corpo como o objeto de erro padrão da Ziett (`code`, `message`, `status`, `trace_id`, `timestamp`, `service`, `fields` opcional — seção "4. Convenções da API — Erros"), e propagar esses dados ao chamador (não apenas uma mensagem genérica) para facilitar o diagnóstico do teste de conectividade.
- Timeout de requisição HTTP razoável (ex.: 15 segundos) — a Ziett responde de forma síncrona a este endpoint (`202 Accepted` imediato, não é fila longa).
- Nunca logar a API Key em texto plano nos logs do servidor.

---

# 3. Handler e contrato da rota

## Objetivo

Expor a rota HTTP isolada que recebe o payload do cliente, valida, aciona o cliente Ziett da seção 2, e devolve o resultado.

## Escopo obrigatório

### 3.1 Localização

Criar o handler em um arquivo novo e isolado em `internal/handlers/` (ex.: dedicado exclusivamente a esta rota), sem misturar com nenhum handler de domínio existente (`financeiro_handlers.go`, `estudante_handlers.go`, `academia_handlers.go`, etc.).

### 3.2 Payload aceito (`POST /integracoes/ziett/mensagens/teste`)

| Campo | Tipo | Obrigatório | Validação |
| --- | --- | --- | --- |
| `remitter_id` | `string` (UUID) | Sim | Deve ser um UUID válido (qualquer versão) |
| `target_e164` | `string` | Sim | Número nacional angolano de 9 dígitos, sem `"0"` inicial e sem `"+244"` (ver seção 2.2 para a regra completa de formatação) |
| `content` | `string` | Sim | Não vazio; máximo 1600 caracteres (limite de SMS multi-parte da Ziett) |

`channel_type` **não** é um campo do payload de entrada desta rota — é sempre fixado como `"SMS"` internamente, conforme seção 2.3.

### 3.3 Validações e erros

- Payload inválido (JSON malformado, campos ausentes, `remitter_id` não-UUID) → `400 Bad Request`, seguindo o envelope de erro padrão já usado no restante da API (`{error, message, request_id, details?}`, ver `internal/utils/errors.go`).
- `target_e164` fora do formato esperado (ver seção 2.2) → `400 Bad Request` com mensagem explicando o formato exigido.
- `ZIETT_API_KEY` não configurada → `503 Service Unavailable`.
- Erro retornado pela própria Ziett (seção 2.4) → repassar o `status` HTTP da Ziett quando fizer sentido (ex.: `401`, `402`, `422`, `429`), incluindo `code` e `trace_id` da Ziett no corpo de erro para facilitar o diagnóstico do teste.
- Falha de rede/timeout ao contactar a Ziett → `500 Internal Server Error` com mensagem genérica (sem vazar detalhes internos de rede).

### 3.4 Resposta de sucesso

`202 Accepted`, contendo no mínimo: mensagem de confirmação, `message_id` retornado pela Ziett, o `target_e164` final já formatado (para conferência), e `channel_type: "SMS"`.

### 3.5 Autenticação e autorização

A rota deve exigir autenticação (`AuthMiddleware()`) e nível de admin FPP (`RequireFPP()`), pelo mesmo motivo de outras operações administrativas sensíveis já restritas a este nível (ex.: `POST /dominis/projections/rebuild/:name`): esta rota dispara SMS real, com custo, através de uma API de terceiros.

Se, na prática operacional da equipa, esse nível de restrição se mostrar excessivo para uso rotineiro de teste, isso deve ser decidido e ajustado explicitamente pela Fredy — não simplificar ou remover a autenticação por conta própria durante a implementação.

---

# 4. Registro da rota

## Escopo obrigatório

- Registrar a rota em `cmd/server/main.go`, dentro de `setupRouter()`, em um grupo novo e próprio (ex.: `router.Group("/integracoes")`), separado de todos os grupos de domínio existentes (`/academia`, `/estudante`, `/financeiro`, `/dominis`, etc.).
- Não adicionar esta rota a nenhum grupo já existente, mesmo que compartilhe o mesmo middleware de autenticação/autorização de outro grupo.
- Nenhuma outra linha de `main.go` deve ser alterada além da inserção deste novo grupo/rota.

---

# 5. Atualização obrigatória da documentação

1. Atualizar `Documentação da API.md`: criar uma nova seção no índice e no corpo do documento (ex.: seção 21, após "20. Armazenamento") intitulada algo como "Integrações Externas / Ziett (Teste)", documentando: método, rota, autenticação exigida, contrato completo do payload de entrada, contrato da resposta de sucesso, e os possíveis erros (incluindo os erros repassados da própria Ziett), seguindo o mesmo padrão de detalhamento já usado na seção "19. Financeiro / AppyPay".
2. Atualizar `.env.example` conforme especificado na seção 1 desta tarefa.
3. Não é necessário atualizar OpenAPI/Swagger caso o projeto não possua um arquivo desse tipo já mantido; verificar se existe antes de decidir.

---

# 6. Testes obrigatórios

1. `target_e164` válido (`"923456789"`) é corretamente formatado para `"+244923456789"` antes do envio.
2. `target_e164` com prefixo `"0"` recebido por engano (`"0923456789"`) ainda é aceito de forma defensiva e formatado corretamente (ver seção 2.2, item 3).
3. `target_e164` com `"+244"` ou `"244"` recebido por engano ainda é aceito de forma defensiva e formatado corretamente.
4. `target_e164` inválido (menos/mais de 9 dígitos, não iniciado em `"9"`, com letras) é rejeitado com `400` e mensagem clara.
5. `remitter_id` ausente ou não-UUID é rejeitado com `400`.
6. `content` ausente, vazio ou acima de 1600 caracteres é rejeitado com `400`.
7. Requisição sem `ZIETT_API_KEY` configurada retorna `503`, sem tentar chamar a Ziett.
8. Requisição sem autenticação retorna `401`; requisição autenticada mas sem nível FPP retorna `403`.
9. Resposta `202` da Ziett (mockada) é corretamente repassada ao chamador com `message_id`.
10. Resposta de erro da Ziett (mockada, ex.: `401`, `422`) é corretamente repassada ao chamador com `code` e `trace_id` da Ziett preservados.
11. Confirmar, por leitura de código/testes, que nenhuma chamada desta rota grava evento no ledger, nem lê/escreve em `internal/db`, `internal/domain` ou `internal/projections`.

---

# Fora de escopo

- Integração desta rota ao sistema de notificações SMS mais amplo do Spuri (aggregator com Sender ID alfanumérico "Spuri", arquitetura já referenciada no roadmap de notificações) — isso é trabalho futuro, distinto e não coberto por esta tarefa.
- Persistência histórica das mensagens enviadas por este endpoint (não criar tabela, projeção ou log estruturado dedicado além do log padrão de requisição/erro já usado no resto da API).
- Suporte a `channel_type` diferente de `"SMS"` (WhatsApp, Telegram, Push) — a Ziett já documenta esses canais como "em integração" ou "planeado"; esta tarefa cobre apenas SMS.
- Implementação de `Retrieve Message`, `List Messages`, `Send Batch Campaign` ou `Retrieve Campaign` — outros endpoints documentados na Ziett, mas fora do escopo desta rota isolada de teste.
- Qualquer alteração em arquivos do fluxo de inscrição/matrícula de estudante numa academia (`estudante_handlers.go`, `solicitacao_matricula_handlers.go`, agregados de `Estudante`/`SolicitacaoMatricula`, projeções correspondentes, etc.) — esta tarefa não deve tocar nenhum desses arquivos.
- Suporte a `save_contact` (upsert de contacto na Ziett) documentado no corpo opcional do `POST /messages` da Ziett — não faz parte do payload desta rota de teste.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `POST /integracoes/ziett/mensagens/teste` existir, isolada em grupo próprio, exigindo `AuthMiddleware()` + `RequireFPP()`;
2. a variável `ZIETT_API_KEY` estiver documentada em `.env.example` e lida via `os.Getenv`, sem hardcode;
3. o cliente HTTP da Ziett e o handler estiverem em arquivos novos e isolados, sem importar `internal/db`, `internal/domain`, `internal/projections` ou `internal/finance`, e sem depender de `internal/utils/validation.go`;
4. `channel_type` estiver sempre fixo em `"SMS"` no backend, independentemente do que o payload de entrada contiver;
5. `target_e164` for aceito no formato descrito (sem `"0"` inicial, sem `"+244"`) e corretamente convertido para E.164 antes do envio à Ziett, com o tratamento defensivo da seção 2.2;
6. os erros da Ziett (autenticação, saldo, validação, rate limit) forem repassados de forma legível ao chamador, incluindo `code` e `trace_id`;
7. `Documentação da API.md` e `.env.example` estiverem atualizados conforme a seção 5;
8. todos os cenários de teste da seção 6 estiverem cobertos por testes automatizados;
9. `go build ./...`, `go vet ./...` e `gofmt -l` rodarem limpos, sem erros nem arquivos mal formatados;
10. nenhum arquivo relacionado ao fluxo de inscrição/matrícula de estudante numa academia tiver sido alterado ou removido.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Criar rota isolada de teste de envio de mensagem via Ziett (SMS) (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
