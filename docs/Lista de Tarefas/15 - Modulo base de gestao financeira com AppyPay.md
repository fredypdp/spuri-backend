---

⚠️ **Nota desta revisão:** esta versão foi reescrita a partir de: (1) contacto directo com o suporte comercial da AppyPay (email trocado pelo Fredy, CEO do Spuri, em 04/08/2026), (2) a documentação oficial consolidada em `docs/Parceiros e integrações/AppyPay Documentação.md` (secção nova "Escopo do Módulo Financeiro Base — Spuri × AppyPay (Fase 1)"), e (3) instruções directas do Fredy sobre o que incluir/excluir. **Não tive acesso ao conteúdo da versão anterior desta tarefa**, nem a `docs/Debbugs/Depuração — Rollback total do módulo financeiro AppyPay.md` nem a `docs/Tarefas feitas/17 - Remover completamente o modulo financeiro AppyPay (rollback total).md` — ambos vieram vazios neste ambiente. Antes de iniciar a implementação, **quem for executar esta tarefa deve ler primeiro esses dois documentos**, se disponíveis, para não repetir a causa-raiz do rollback anterior. Se este documento contradisser algo aprendido nesse rollback, a lição aprendida do rollback prevalece e este documento deve ser ajustado.

---

# 15 — Módulo base de gestão financeira com AppyPay

## 1. Objectivo

Construir a camada **base** de integração do Spuri com o gateway de pagamentos AppyPay: autenticação, criação/consulta de cobranças, e recepção de webhooks. Esta camada é a **única** parte do sistema que fala directamente com a API da AppyPay.

Casos de uso concretos de cobrança (ex.: cobrança de propina, cobrança de matrícula, etc.) **não fazem parte desta tarefa**. Serão implementados em tarefas futuras como módulos finos que apenas preenchem os parâmetros de negócio (valor, descrição, método escolhido, dados do pagador) e chamam a função base descrita aqui. Por isso a função base de criação de cobrança tem de ser **genérica e suportar todos os parâmetros possíveis de uma cobrança AppyPay**, não apenas os que um caso de uso específico usa hoje.

## 2. Escopo (o que construir nesta tarefa)

1. **Autenticação AppyPay** (client credentials grant), com renovação/cache de token, para dois níveis de credenciais:
   - Credenciais da **Academia** (cada instituição de ensino integrada ao Spuri tem a sua própria conta/credenciais na AppyPay).
   - Credenciais do **Spuri** (conta própria da plataforma, para cobranças que não pertencem a uma academia específica, caso existam).
2. **Armazenamento de credenciais no banco de dados**, cifradas, por academia e para o Spuri (ver secção 6).
3. **Função base de criação de cobrança** (`POST /charges` e `POST /qr-codes`), robusta o suficiente para suportar qualquer combinação válida dos parâmetros documentados pela AppyPay (não apenas os campos usados pelos métodos abaixo).
4. **Função base de consulta de cobrança** (`GET /charges/{id}`).
5. **Base de recepção de Webhooks** (endpoint HTTP exposto pelo Spuri, autenticação do lado do Spuri, idempotência, persistência do evento recebido).
6. **Selecção automática de ambiente** (TEST/PROD) da API AppyPay a partir da variável de ambiente que já indica o ambiente de execução do Spuri.

### Métodos de pagamento suportados nesta fase

Apenas: **GPO** (cobrança directa MCX App), **GPO QR Code**, **REF** (referência, gerada pelo comerciante ou pelo gateway).

### Explicitamente fora de escopo nesta tarefa

- UMM, FTBAI, eTPA, SDD (mandatos de débito directo) — não implementar.
- Documentos fiscais (Fiscal Documents) — não implementar.
- Funds-Transfers — não implementar.
- **5.5 Reembolsar cobrança** (`Post a charge refund`) — **removido do escopo**. Não implementar reembolsos nesta fase.
- **5.6 Reverter cobrança** (`Post a charge reverse`) — **removido do escopo**. Não implementar reversões nesta fase.
- **5.8 Reconciliação e observabilidade** (dashboards, relatórios de conciliação, jobs de reconciliação em lote) — **removido do escopo**. A única "reconciliação" desta fase é a dupla verificação pontual via `GET /charges/{id}` já descrita na função base de consulta (secção 5.3), não um subsistema de observabilidade.
- Widget de checkout embutido (frontend) — mencionado na documentação oficial mas não é necessário para esta tarefa, que é backend. Pode ser avaliado em tarefa futura separada.
- Qualquer cobrança de caso de uso específico (propinas, matrículas, etc.) — fica para tarefas futuras que consumirão esta base.

## 3. Onde consultar a documentação da AppyPay

Todo o trabalho técnico de integração deve ser validado contra `docs/Parceiros e integrações/AppyPay Documentação.md`. Secções relevantes dentro desse ficheiro:

| O que implementar | Secção a consultar |
| --- | --- |
| Visão geral de autenticação | `Authentication and Authorization` |
| Obter token (endpoint, grant, expiração) | `Get a token`, `AppyPay-Authentication` (URLs TEST/PROD) |
| URL base da API de charges/qr-codes | `AppyPay-API` (URLs TEST/PROD) |
| Criar cobrança GPO/REF | `Post a Charge`, `Charges` |
| Criar QR Code GPO | `Post a GPO QR Code` |
| Consultar cobrança | `Get a charge` |
| Métodos de pagamento suportados e limites (valor mínimo, tempo de aprovação, etc.) | `Supported Payment Methods` |
| Webhooks recebidos do lado do Spuri | `Merchant Webhooks` |
| Códigos de erro e como reagir | `Errors` |
| **Resumo específico do Spuri (comece por aqui)** | `Escopo do Módulo Financeiro Base — Spuri × AppyPay (Fase 1)` (secção adicionada ao final do documento) |

A secção "Escopo do Módulo Financeiro Base — Spuri × AppyPay (Fase 1)" já traz as URLs de TEST/PROD confirmadas, exemplos reais de corpo de pedido para GPO/REF (incluindo os números de telefone de simulação de cenários de teste do GPO) e as regras de segurança de credenciais — usar como referência rápida em vez de procurar pelo documento inteiro.

## 4. Arquitectura — como isto encaixa no padrão existente do Spuri

O Spuri usa Event Sourcing/CQRS em todo o sistema (ver `internal/db/event_store.go`, `internal/domain/aggregates/`, `internal/projections/`). O módulo financeiro deve seguir o mesmo padrão:

- **Eventos** de mudanças financeiras (ex.: credenciais configuradas, cobrança criada, resultado de cobrança recebido via webhook/consulta) são gravados no `spuri_ledger` com `aggregate_type = "Financeiro"`, tal como qualquer outro agregado do sistema.
- **Projeções** (`financeiro_*`) são read models reconstruíveis a partir do ledger, seguindo o mesmo mecanismo de rebuild já usado pelos outros módulos (`internal/projections/manager.go`).
- **Nunca** incluir segredos (`client_secret`, tokens de acesso, `webhook_secret`, API keys) nos payloads de eventos nem em respostas públicas da API — apenas metadados não sensíveis e máscaras (ex.: últimos 4 caracteres). Os segredos em si vivem cifrados numa tabela operacional, não no ledger de eventos.
- Antes de desenhar as tabelas/eventos, procurar no repositório se ainda existem vestígios do módulo financeiro anterior (migrações, agregados, projeções) que não tenham sido completamente removidos no rollback (tarefa 17) — reaproveitar nomes/convenções onde fizer sentido, mas não assumir que o schema anterior ainda é válido sem o revalidar.

## 5. Funcionalidades a implementar

### 5.1 Autenticação e gestão de credenciais (Spuri + Academia)

- Fluxo **OAuth2 client credentials grant** contra o endpoint de token da AppyPay.
- Parâmetros do pedido de token: `grant_type=client_credentials`, `client_id`, `client_secret`, `resource` (UUID fornecido pela AppyPay — confirmar se é o mesmo valor para todas as contas ou específico por conta antes de assumir um valor fixo).
- Cache do token em memória/processo com expiração (token válido por 1 hora); renovar antes de expirar em vez de pedir um token novo a cada chamada.
- Estrutura de credenciais a armazenar, por entidade (Academia ou Spuri):
  - `client_id`, `client_secret` (cifrado em repouso).
  - `resource` (UUID da AppyPay).
  - Identificadores de método de pagamento contratados por essa conta (ex.: `GPO_{uuid}`, `REF_{uuid}` — o UUID é gerado no portal AppyPay dessa conta específica).
  - Ambiente ao qual essas credenciais pertencem (TEST/PROD) — uma academia pode ter credenciais de teste e, mais tarde, credenciais de produção distintas.
- Endpoint(s) administrativo(s) para uma academia (ou o Spuri, no caso das suas próprias credenciais) configurar/actualizar estas credenciais. Apenas quem administra a academia deve poder ver/editar — nunca expor `client_secret` de volta numa resposta de leitura (mostrar mascarado).

### 5.2 Selecção de ambiente (TEST/PROD)

- Ler a variável de ambiente que já indica se o Spuri está a correr em desenvolvimento/teste ou produção.
- Se desenvolvimento/teste → usar as URLs TEST (token e API base) descritas na secção "Escopo..." do documento AppyPay.
- Se produção → usar as URLs PROD.
- Centralizar esta escolha numa única função/config (não espalhar `if` de ambiente pelo código) para que o resto do módulo apenas peça "a URL de token" / "a URL base da API" sem saber qual ambiente está activo.

### 5.3 Função base de criação de cobrança

- Uma função/serviço genérico (ex.: `CriarCobranca`) que aceite **todos** os campos documentados no corpo de `POST /charges`: `amount`, `currency`, `description`, `merchantTransactionId`, `paymentMethod`, `paymentInfo` (objecto livre — o conteúdo varia por método), `options` (até 2 chaves customizadas), `notify` (`name`, `telephone`, `email`, `smsNotification`, `emailNotification`).
- Deve receber também qual academia (ou o Spuri) está a efectuar a cobrança, para escolher as credenciais/token correctos.
- Deve suportar explicitamente os três métodos em escopo, validando o mínimo necessário por método antes de chamar a AppyPay (ex.: GPO exige `phoneNumber` em `paymentInfo`; REF aceita `paymentInfo` opcional — se omitido, o gateway gera a referência).
- Não deve conter nenhuma lógica de negócio específica (ex.: "isto é uma propina") — quem chama esta função é que decide o que preencher. Esta função apenas conhece o vocabulário da AppyPay.
- Suportar tanto o modo síncrono (`Accept: application/json`, resposta imediata) como o assíncrono (`Accept: application/vnd.appypay.asyncapi+json`, resposta 202 + resultado por webhook) — decidir o modo por parâmetro de chamada, não fixo.
- `merchantTransactionId` deve ser gerado de forma a ser único e respeitar o padrão exigido pela AppyPay (alfanumérico, até 15 caracteres).
- Gravar evento de "cobrança criada/solicitada" no ledger antes/depois da chamada (conforme o padrão de garantias de atomicidade já usado no resto do sistema), guardando a resposta da AppyPay (id da transacção, estado inicial).

### 5.4 Geração de QR Code GPO

- Função dedicada para `POST /qr-codes`, reaproveitando a mesma autenticação/credenciais da secção 5.1.
- Suportar `qrCodeType` `SINGLE` (por defeito) e `MULTIPLE` (exige `minAmount`, `maxTransactions`, `startDate`, `endDate`).
- A resposta inclui a imagem do QR Code em base64 (`qrCodeArr`) — decidir e documentar como/onde esta imagem é entregue ao chamador (não é preciso desenhar a UI de apresentação nesta tarefa, apenas devolver o dado de forma utilizável).

### 5.5 ~~Reembolsar cobrança~~ — removido do escopo

Não implementar nesta fase. Ver secção 2.

### 5.6 ~~Reverter cobrança~~ — removido do escopo

Não implementar nesta fase. Ver secção 2.

### 5.7 Função base de consulta de cobrança

- Função/serviço genérico (ex.: `ConsultarCobranca`) que chama `GET /charges/{id}` (ou por `merchantTransactionId`), usando as credenciais/token da mesma academia/Spuri que criou a cobrança.
- Deve ser utilizável tanto por uma rotina de dupla verificação pontual (chamada manual ou pelo próprio webhook antes de aplicar efeitos) como, futuramente, por consumidores que só têm o `merchantTransactionId`.
- Actualizar a projecção financeira com o estado retornado.

### 5.8 ~~Reconciliação e observabilidade~~ — removido do escopo

Não implementar dashboards, relatórios de conciliação ou jobs de reconciliação em lote nesta fase. A única verificação de consistência prevista aqui é a chamada pontual a `GET /charges/{id}` já coberta na secção 5.7.

### 5.9 Base de Webhooks

- Expor um endpoint HTTP público (`POST`) dedicado a receber notificações transaccionais da AppyPay.
- Autenticação do lado do Spuri conforme suportado pela AppyPay: Basic Auth ou API Key (decidir e documentar qual será usado; guardar o segredo correspondente cifrado, associado à academia/Spuri dono do webhook).
- Responder **HTTP 200** assim que o payload for aceite/enfileirado — não fazer processamento pesado de forma síncrona dentro do handler do webhook.
- **Idempotência obrigatória:** o mesmo `id`/`merchantTransactionId` pode chegar mais de uma vez (reenvios por falha de comunicação; para REF, uma notificação por tentativa de comunicação com o provedor). Processar de forma que reaplicar o mesmo evento não duplique efeitos.
- Persistir o payload recebido como evento no ledger (sem segredos, se algum vier embutido) antes/como parte da actualização de estado da cobrança correspondente.
- Esta tarefa cobre apenas a **recepção, autenticação, idempotência e persistência** do webhook. Reagir ao resultado de uma cobrança específica de negócio (ex.: marcar uma propina como paga) é responsabilidade das tarefas futuras que criaram essa cobrança, não desta base.

## 6. Persistência e segurança

- Seguir Event Sourcing/CQRS (secção 4): eventos no `spuri_ledger` com `aggregate_type = "Financeiro"`; tabelas `financeiro_*` como projeções/read models e armazenamento operacional cifrado das credenciais.
- **Nunca** persistir ou expor em respostas públicas: `client_secret`, `webhook_secret`/API key do webhook, tokens de acesso (access token, mesmo que de curta duração), ou o conteúdo bruto de credenciais. Nos eventos e em qualquer log/auditoria, usar apenas máscaras (ex.: últimos 4 caracteres) e metadados não sensíveis (quem alterou, quando).
- Cifragem em repouso das credenciais na tabela operacional correspondente; decidir o mecanismo de cifragem seguindo o que já existe no projecto (verificar se já há um padrão de cifragem usado noutro lugar do sistema antes de introduzir um novo).
- Rebuild de projeções financeiras deve seguir o mesmo mecanismo dos outros módulos (`internal/projections/manager.go`), reaplicando os eventos financeiros em ordem determinística sem apagar o ledger.

## 7. Critérios de aceitação

1. É possível configurar credenciais AppyPay (client_id/client_secret/resource/UUIDs de método) para uma academia e, separadamente, para o Spuri, cifradas em repouso, nunca devolvidas em texto puro numa leitura.
2. O sistema obtém e reutiliza um token válido, escolhendo automaticamente a URL de token TEST ou PROD conforme o ambiente de execução, sem valores fixos no código.
3. Existe uma função base de criação de cobrança que aceita todos os parâmetros documentados pela AppyPay e consegue criar, com sucesso em ambiente de teste, uma cobrança GPO e uma cobrança REF (com e sem `paymentInfo`, cobrindo referência gerada pelo comerciante e pelo gateway), usando os exemplos e números de simulação confirmados na secção "Escopo..." do documento AppyPay.
4. Existe uma função base para gerar um QR Code GPO em ambiente de teste.
5. Existe uma função base de consulta de cobrança por id/`merchantTransactionId`.
6. Existe um endpoint de webhook público, protegido (Basic Auth ou API Key), que responde 200, é idempotente a reenvios do mesmo evento, e persiste o payload recebido como evento financeiro.
7. Nenhum segredo (client_secret, tokens, api key/segredo de webhook) aparece em payload de evento, log ou resposta de API.
8. Reembolso, reversão, e qualquer subsistema de reconciliação/observabilidade **não** foram implementados nesta tarefa.

## 8. Notas para quem for executar (qwen/codex)

- Este documento é orientação de requisitos, não uma especificação de código linha-a-linha. Seguir os padrões e nomes já usados no resto do repositório (agregados em `internal/domain/aggregates/`, projeções em `internal/projections/`, handlers em `internal/handlers/`, migrações numeradas sequencialmente em `migrations/`) em vez de inventar uma convenção nova.
- Validar cada endpoint contra a documentação oficial (`docs/Parceiros e integrações/AppyPay Documentação.md`) antes de dar a funcionalidade por concluída — os exemplos de corpo de pedido/resposta citados nas secções acima vieram directamente do suporte da AppyPay e devem bater certo com o que for implementado.
- Se `docs/Debbugs/Depuração — Rollback total do módulo financeiro AppyPay.md` ou `docs/Tarefas feitas/17 - Remover completamente o modulo financeiro AppyPay (rollback total).md` estiverem disponíveis no repositório, leia-os **antes** de desenhar o schema/eventos — eles explicam por que a tentativa anterior deste mesmo módulo foi completamente revertida (migração 099), e essa causa-raiz não está reflectida neste documento.
