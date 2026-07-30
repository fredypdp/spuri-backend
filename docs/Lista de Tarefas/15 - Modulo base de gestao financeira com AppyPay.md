---
criado: 2026-07-26 00:00
origem: docs/Parceiros e integrações/AppyPay - Análise de Integração para o Serviço de Gestão Financeira do Spuri.md
status: pendente
---

# Implementar módulo base de gestão financeira com AppyPay para Spuri e academias (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento criando um **módulo base de gestão financeira** integrado à AppyPay, capaz de atender dois contextos financeiros independentes: (1) o **Spuri** cobrando instituições integradas/academias pelo uso da plataforma; e (2) cada **academia** cobrando os seus próprios estudantes em seu contexto institucional. O módulo deve cadastrar, validar, armazenar com criptografia e usar credenciais AppyPay por contexto financeiro, expor endpoints apenas para configuração e controlo do sistema financeiro, mantendo cobrança/consulta/cancelamento/reembolso/webhook/reconciliação como funções/handlers Model internos genéricos e reutilizáveis, e garantir auditabilidade, idempotência, rastreabilidade, imutabilidade do histórico financeiro, atomicidade, consistência, controlo de concorrência, autorização adequada, isolamento por instituição e observabilidade. Ao final, atualize testes, documentação técnica, `Documentação.md`, OpenAPI/Swagger quando existir e qualquer documentação afetada. Não criar funcionalidades específicas de negócio como cobrar propina, matrícula, mensalidade ou assinatura por nome específico; essas regras devem ser tarefas futuras que reutilizam as funções base deste módulo.

## Contexto

O documento de análise da integração AppyPay descreve inicialmente o caso em que o Spuri é o **merchant** que cobra escolas/academias, usando OAuth2 Client Credentials, `POST /charges`, consultas por `GET /charges/{id}`/`GET /charges`, reembolsos, reversões, mandatos SDD, referências, documentos fiscais, payouts e webhooks. A mesma base técnica deve ser expandida para permitir que cada academia também seja um contexto financeiro próprio e possa cobrar seus estudantes por meio das suas próprias credenciais AppyPay.

Neste desenho, a plataforma passa a gerir dois níveis de cobrança:

1. **Contexto Spuri** — credenciais e aplicações AppyPay controladas pela plataforma, usadas para cobrar academias integradas.
2. **Contexto Academia** — credenciais e aplicações AppyPay cadastradas por cada academia, usadas apenas para cobranças daquela academia contra seus estudantes.

A palavra `AppPay` usada em alguns requisitos deve ser tratada como referência à **AppyPay**, mantendo a nomenclatura oficial da documentação técnica.

O módulo deve ser construído como infraestrutura financeira genérica. Cobranças específicas de negócio — por exemplo propina, matrícula, mensalidade escolar, certificado, taxa administrativa ou assinatura do Spuri — devem ser implementadas depois como camadas de domínio que chamam as funções base aqui definidas.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Integração | AppyPay como provider inicial do módulo financeiro | Base preparada com Models/handlers internos para cobrança, consulta, cancelamento, reembolso, webhook e reconciliação, sem endpoints transacionais nesta tarefa |
| Contextos financeiros | `spuri` e `academia` | Spuri cobra academias; academias cobram estudantes usando credenciais próprias |
| Credenciais de academia | Cadastro e gestão segura por academia | Cada academia usa seu próprio `client_id`, `client_secret`, `resource`, `apiKey`/applications e webhooks |
| Ativação de pagamentos | Controle global e por academia | FPP/ADMIN podem ativar/desativar a modalidade de pagamento de forma geral ou específica |
| Escopo funcional | Endpoints de configuração/controlo + funções Model base genéricas | Nenhum endpoint transacional ou cobrança específica de negócio é implementado nesta tarefa |
| Segurança | Criptografia, RBAC, webhooks seguros, idempotência e isolamento | Dados sensíveis protegidos e transações rastreáveis sem mistura entre instituições |
| Histórico financeiro | Event sourcing/ledger imutável + projeções consultáveis | Auditoria completa e reconstrução segura do estado financeiro |

---

# 1. Objetivo

Criar o módulo financeiro base do Spuri com suporte à AppyPay para:

1. permitir que o **Spuri** gere e acompanhe cobranças contra academias/instituições integradas;
2. permitir que **academias** cadastrem suas credenciais AppyPay e gerem cobranças contra estudantes no seu próprio contexto;
3. expor por HTTP apenas operações de configuração e controlo do sistema financeiro, mantendo operações financeiras transacionais como funções/handlers Model genéricos e reutilizáveis, sem acoplar a implementação a tipos específicos de cobrança;
4. preservar histórico financeiro imutável, auditável, idempotente e isolado por instituição;
5. permitir que FPP/ADMIN controlem se pagamentos estão ativos globalmente e/ou para uma academia específica.

# 2. Modelo de contextos financeiros

## 2.1 Contexto `spuri`

O contexto `spuri` representa a plataforma como recebedora. Ele deve suportar cobranças feitas pelo Spuri contra academias integradas, usando credenciais AppyPay administradas pela plataforma.

Regras obrigatórias:

1. somente FPP/ADMIN podem criar, atualizar, ativar, desativar ou testar credenciais do contexto `spuri`;
2. cobranças do contexto `spuri` devem identificar a academia devedora, mas nunca usar credenciais da academia;
3. todos os eventos e projeções devem deixar claro que o beneficiário financeiro é a plataforma.

## 2.2 Contexto `academia`

O contexto `academia` representa uma academia como recebedora. Ele deve suportar cobranças feitas por uma academia contra estudantes vinculados à própria academia, usando credenciais AppyPay cadastradas por essa academia ou por um administrador autorizado.

Regras obrigatórias:

1. cada academia só pode consultar e usar suas próprias credenciais e transações;
2. uma cobrança de estudante deve validar que o estudante pertence à academia do contexto autenticado;
3. nenhuma credencial de uma academia pode ser usada para cobrar estudante de outra academia;
4. FPP/ADMIN podem gerir ou bloquear a configuração de uma academia para suporte operacional e compliance.

# 3. Credenciais AppyPay

## 3.1 Dados suportados

Implementar entidade/configuração de credenciais AppyPay por contexto financeiro, contemplando pelo menos:

| Campo | Obrigatório | Descrição |
| --- | --- | --- |
| `id` | Sim | Identificador interno da configuração |
| `contexto_tipo` | Sim | `spuri` ou `academia` |
| `codigo_academia` | Condicional | Obrigatório quando `contexto_tipo=academia` |
| `ambiente` | Sim | `test` ou `prod` |
| `auth_base_url` | Sim | URL base de autenticação AppyPay/Azure AD |
| `api_base_url` | Sim | URL base da AppyPay-API |
| `webapi_base_url` | Não | URL base da AppyPay-Web-API quando houver uso de payouts/backoffice |
| `client_id` | Sim | Credencial OAuth2 Client Credentials |
| `client_secret` | Sim | Segredo OAuth2, sempre criptografado em repouso |
| `resource` | Sim | Resource GUID usado para emissão de token |
| `applications` | Sim | Lista de métodos/aplicações AppyPay configuradas, com `paymentMethod`, `applicationId`, `apiKey`, webhook e metadados necessários |
| `webhook_secret`/`webhook_token` | Não | Segredo local para validação adicional de webhook, quando aplicável |
| `status` | Sim | `ativo`, `inativo`, `pendente_validacao`, `erro_validacao` |
| `created_at`/`updated_at`/`version` | Sim | Metadados de auditoria e controlo de concorrência |

Se a AppyPay exigir outros dados de autenticação ou uso da API além dos listados, a implementação deve incluí-los como campos explícitos, documentados e protegidos como dados sensíveis quando aplicável.

## 3.2 Segurança das credenciais

1. `client_secret`, API keys sensíveis, tokens, segredos de webhook e qualquer material equivalente devem ser criptografados em repouso.
2. Respostas de API nunca devem expor segredos em claro; usar mascaramento (`****1234`) ou apenas metadados.
3. Logs, métricas, eventos de domínio e mensagens de erro não podem conter segredos.
4. Atualizações de credenciais devem gravar evento auditável sem persistir valores sensíveis em claro no payload público do evento.
5. Deve existir rotação de credenciais por substituição controlada, mantendo histórico auditável de quem alterou e quando, mas sem permitir recuperar segredos antigos em claro.

## 3.3 Endpoints de gestão de credenciais

Criar endpoints equivalentes, respeitando RBAC e isolamento:

1. `POST /financeiro/appypay/credenciais` — criar credenciais do contexto `spuri` ou de uma academia, permitido apenas para FPP/ADMIN;
2. `PUT /financeiro/appypay/credenciais/:id` — atualizar/rotacionar credenciais, permitido para FPP/ADMIN e, no contexto `academia`, para administrador autenticado da própria academia quando a política permitir;
3. `GET /financeiro/appypay/credenciais` — listar metadados sem segredos, filtrados por contexto e permissões;
4. `GET /financeiro/appypay/credenciais/:id` — consultar metadados sem segredos;
5. `POST /financeiro/appypay/credenciais/:id/testar` — validar autenticação, token e applications configuradas sem criar cobrança real;
6. `POST /financeiro/appypay/credenciais/:id/ativar` e `POST /financeiro/appypay/credenciais/:id/desativar` — alterar status com evento auditável.

# 4. Ativação/desativação da modalidade de pagamento

A modalidade de pagamento das academias deve poder ser ativada ou desativada em dois níveis:

1. **Global** — controla se academias, de modo geral, podem usar pagamentos próprios na plataforma. Apenas FPP/ADMIN podem alterar.
2. **Específico por academia** — controla se uma academia específica pode usar seu contexto financeiro próprio. Apenas FPP/ADMIN podem alterar; a academia pode consultar o seu estado, mas não pode forçar ativação se estiver bloqueada pela plataforma.

Regras obrigatórias:

1. se a modalidade global estiver desativada, nenhuma academia pode gerar cobranças próprias, mesmo que sua configuração específica esteja ativa;
2. se a modalidade global estiver ativa, uma academia só pode gerar cobranças próprias se sua modalidade específica e suas credenciais estiverem ativas e válidas;
3. o contexto `spuri` para cobrar academias deve ter chave de ativação separada, para não ser bloqueado acidentalmente pela desativação da modalidade de cobrança das academias;
4. toda alteração de ativação/desativação deve registrar motivo, autor, data/hora e escopo (`global` ou `academia`).

# 5. Funções base do módulo financeiro

Implementar e documentar apenas funções/handlers Model genéricos/base para as capacidades transacionais abaixo. Nesta tarefa, **não criar endpoints HTTP transacionais** para cobrança, consulta de cobrança, sincronização de status, cancelamento, reembolso, reversão, webhooks transacionais ou reconciliação operacional. Os endpoints permitidos nesta tarefa são apenas os de configuração e controlo do próprio sistema de gestão financeira, como credenciais, ativação/desativação, validação de configuração, estado operacional e parâmetros administrativos. As funções Model devem ser suficientemente versáteis para serem reutilizadas futuramente por handlers específicos de domínio, por exemplo geração de cobrança de propina, matrícula, mensalidade, certificado ou assinatura, sem duplicar integração AppyPay nem regras de idempotência/auditoria.

## 5.1 Gerar cobrança

Criar função/handler Model base para gerar cobrança, sem endpoint HTTP direto nesta tarefa. Exemplo conceitual de assinatura interna: `gerarCobrancaFinanceiraBase(...)`, aceitando:

| Campo | Descrição |
| --- | --- |
| `contexto_tipo` | `spuri` ou `academia` |
| `codigo_academia` | Academia do contexto, obrigatória para contexto `academia`; academia devedora para contexto `spuri` quando aplicável |
| `pagador_tipo` | `academia`, `estudante` ou outro tipo genérico futuro |
| `pagador_id` | Identificador interno do pagador |
| `valor` | Valor monetário em AOA |
| `moeda` | Inicialmente `AOA` |
| `metodo_pagamento` | Método AppyPay base: `REF`, `GPO`, `UMM`, `SDD` etc., conforme credenciais/applications ativas |
| `descricao` | Descrição genérica da cobrança, sem criar tipo específico de negócio |
| `referencia_externa` | Referência idempotente do chamador/domínio que originou a cobrança |
| `metadata` | Metadados não sensíveis para rastreabilidade |

Regras obrigatórias:

1. gerar `merchantTransactionId` único e idempotente por contexto financeiro;
2. mapear `metodo_pagamento` para a `application`/`apiKey` correta, formando o `paymentMethod` esperado pela AppyPay;
3. persistir primeiro uma intenção/evento interno de cobrança antes de chamar a AppyPay, ou usar estratégia transacional equivalente que permita reconciliação se houver falha após a chamada externa;
4. tratar `200` e `202` conforme o fluxo síncrono/assíncrono da AppyPay;
5. nunca marcar cobrança como definitivamente paga apenas pela criação da cobrança; confirmação final deve vir de consulta segura, webhook validado ou reconciliação.

## 5.2 Consultar cobrança

Criar função/handler Model base para consultar cobrança interna e, quando necessário, sincronizar com `GET /charges/{id}` ou `GET /charges` da AppyPay, sem endpoint HTTP direto nesta tarefa.

A consulta deve retornar:

1. status interno normalizado;
2. status bruto AppyPay (`responseStatus`) preservado para auditoria;
3. identificadores internos e externos (`id`, `merchantTransactionId`, provider charge id);
4. histórico de mudanças de estado;
5. contexto financeiro e pagador, respeitando permissões.

## 5.3 Consultar estado/status de transação

Criar função/handler Model explícito de atualização de status, por exemplo `sincronizarStatusCobrancaFinanceiraBase(...)`, para consultar a AppyPay e gravar evento interno quando houver mudança, sem endpoint HTTP direto nesta tarefa.

Regras obrigatórias:

1. operação idempotente;
2. controlo de concorrência para evitar duas sincronizações simultâneas gravando estados conflitantes;
3. preservação do histórico anterior; status financeiro não deve ser sobrescrito sem evento.

## 5.4 Cancelar cobrança

Criar função/handler Model base para cancelar cobrança quando o método/provider suportar cancelamento ou quando o cancelamento for apenas interno antes de envio/processamento, sem endpoint HTTP direto nesta tarefa.

Regras obrigatórias:

1. validar se o status atual permite cancelamento;
2. registrar motivo e autor;
3. quando houver chamada externa, persistir resposta bruta e status normalizado;
4. não apagar cobrança cancelada.

## 5.5 Reembolsar cobrança

Criar função/handler Model base para reembolso, usando `POST /refunds/{id}` quando aplicável, sem endpoint HTTP direto nesta tarefa.

Regras obrigatórias:

1. validar suporte do método AppyPay (ex.: GPO, UMM e SDD conforme documentação);
2. suportar reembolso total ou parcial quando o provider permitir;
3. impedir reembolso acima do valor liquidado líquido;
4. registrar evento financeiro próprio para solicitação, aceite, falha e conclusão do reembolso;
5. manter vínculo entre cobrança original e reembolso.

## 5.6 Reverter cobrança, quando aplicável

Criar função base para reversão quando o método suportar, como UMM via `POST /reverses/{id}`.

A reversão deve ser modelada separadamente de reembolso, pois possui semântica e disponibilidade diferentes no provider.

## 5.7 Webhooks transacionais e não transacionais

Criar handlers Model/serviços internos para processar payloads de webhook AppyPay de cobranças, mandatos, documentos fiscais e outros eventos aplicáveis. Nesta tarefa, não expor endpoints públicos de webhook transacional; a rota pública, quando criada futuramente, deverá ser apenas um adaptador fino que valida a entrada e chama estes handlers base.

Regras obrigatórias:

1. validar origem/autenticidade com os mecanismos disponíveis e com segredo local quando configurado;
2. processar de forma idempotente, pois a AppyPay pode reenviar o mesmo evento;
3. gravar payload bruto de forma segura para auditoria, removendo/mascarando dados sensíveis quando necessário;
4. confirmar transações críticas com `GET /charges/{id}` antes de marcar como liquidadas definitivamente;
5. isolar o webhook por contexto financeiro, identificando corretamente se o evento pertence ao Spuri ou a uma academia.

## 5.8 Reconciliação e observabilidade

Implementar função/handler Model ou processo interno de reconciliação que consulte cobranças, referências, analytics e payouts disponíveis na AppyPay para detectar divergências entre provider e estado interno. Nesta tarefa, qualquer endpoint HTTP para disparar reconciliação fica fora de escopo; se houver controlo administrativo, deve limitar-se a configuração/estado operacional.

Regras obrigatórias:

1. gerar alertas/métricas para cobranças presas em `pendente`, webhooks duplicados, webhooks inválidos, divergência de valor/status e falhas de autenticação;
2. registrar eventos de reconciliação sem alterar histórico passado;
3. permitir reprocessamento seguro sem duplicar efeitos financeiros;
4. expor logs estruturados com correlation id, contexto financeiro, academia e identificadores externos, sem segredos.

# 6. Modelo de dados e eventos esperados

A implementação deve seguir o padrão de event sourcing/projeções já usado no backend, evitando atualização direta e silenciosa de estado financeiro.

Entidades/projeções mínimas esperadas:

1. `ConfiguracaoFinanceira` ou equivalente para ativação global/específica;
2. `CredencialAppyPay` ou equivalente para metadados e segredos criptografados;
3. `CobrancaFinanceira` para cobranças genéricas;
4. `TransacaoFinanceira` ou histórico de estados externos;
5. `ReembolsoFinanceiro` e `ReversaoFinanceira`, se aplicável;
6. `WebhookFinanceiroRecebido` para idempotência e auditoria;
7. `ReconciliacaoFinanceira` para divergências e ajustes controlados.

Eventos mínimos esperados:

1. `CredenciaisAppyPayCadastradas`;
2. `CredenciaisAppyPayAtualizadas`;
3. `CredenciaisAppyPayValidadas`;
4. `CredenciaisAppyPayAtivadas`/`CredenciaisAppyPayDesativadas`;
5. `ModalidadePagamentoGlobalAlterada`;
6. `ModalidadePagamentoAcademiaAlterada`;
7. `CobrancaFinanceiraCriada`;
8. `CobrancaFinanceiraEnviadaAoProvider`;
9. `CobrancaFinanceiraStatusAtualizado`;
10. `CobrancaFinanceiraCancelada`;
11. `ReembolsoFinanceiroSolicitado`/`ReembolsoFinanceiroStatusAtualizado`;
12. `ReversaoFinanceiraSolicitada`/`ReversaoFinanceiraStatusAtualizado`;
13. `WebhookFinanceiroRecebido`/`WebhookFinanceiroIgnoradoComoDuplicado`;
14. `DivergenciaFinanceiraDetectada`/`DivergenciaFinanceiraReconciliada`.

# 7. Requisitos de qualidade e segurança

A tarefa só deve ser implementada se contemplar explicitamente:

1. **auditabilidade** — toda alteração financeira relevante deve ter evento, autor, contexto, motivo quando aplicável e timestamp;
2. **idempotência** — criação de cobranças, processamento de webhooks, reembolsos, reversões e reconciliação não podem duplicar efeitos;
3. **rastreabilidade** — cada cobrança deve ligar domínio interno, pagador, contexto financeiro, `merchantTransactionId` e identificadores AppyPay;
4. **imutabilidade do histórico financeiro** — eventos financeiros não podem ser apagados ou reescritos para “corrigir” estado;
5. **atomicidade e consistência** — falhas entre gravação interna e chamada externa devem ser recuperáveis por reconciliação;
6. **controlo de concorrência** — evitar cobranças duplicadas e atualizações concorrentes de status/reembolso;
7. **autenticação e autorização** — FPP/ADMIN controlam ativação e credenciais sensíveis; academias só atuam no próprio contexto;
8. **criptografia de dados sensíveis** — segredos sempre criptografados em repouso e mascarados em saída/logs;
9. **webhooks seguros** — validação, idempotência, confirmação ativa no provider para liquidação e armazenamento seguro;
10. **isolamento por instituição** — consultas, cobranças e credenciais de uma academia nunca vazam para outra;
11. **observabilidade** — métricas, logs estruturados, alertas e correlation ids;
12. **reconciliação** — jobs ou rotas administrativas para comparar estado interno e AppyPay.

# 8. Testes obrigatórios

Criar testes automatizados cobrindo pelo menos:

1. FPP/ADMIN cadastram credenciais AppyPay do contexto `spuri` com segredos criptografados;
2. academia cadastrando/atualizando suas credenciais quando permitido não consegue ver segredos em claro na resposta;
3. academia A não consegue listar, consultar ou usar credenciais/cobranças da academia B;
4. modalidade global desativada impede cobranças próprias das academias;
5. modalidade específica desativada impede cobranças da academia bloqueada;
6. contexto `spuri` consegue gerar cobrança contra academia sem usar credenciais da academia;
7. contexto `academia` consegue gerar cobrança genérica contra estudante vinculado;
8. tentativa de cobrar estudante não vinculado à academia é rejeitada;
9. criação idempotente com a mesma `referencia_externa` não duplica cobrança;
10. webhook duplicado não duplica evento de liquidação;
11. webhook que indica sucesso só marca cobrança como liquidada após confirmação segura por consulta ao provider, usando mock/fake do provider nos testes;
12. reembolso respeita valor máximo e suporte do método;
13. atualização concorrente de status não gera histórico inconsistente;
14. logs/respostas de erro não contêm `client_secret`, `apiKey` sensível, token ou segredo de webhook.

# 9. Atualização obrigatória da documentação

Atualizar `Documentação.md` e documentação técnica relacionada com:

1. entidades/configurações financeiras criadas;
2. endpoints de credenciais e ativação/controlo permitidos, e funções/handlers Model internos para cobranças, consulta, sincronização, cancelamento, reembolso, reversão, webhooks e reconciliação;
3. estados normalizados de cobrança, reembolso e reversão;
4. matriz de permissões FPP/ADMIN/academia;
5. política de criptografia, mascaramento e rotação de credenciais;
6. garantias de idempotência e chaves idempotentes usadas;
7. mapeamento básico dos endpoints AppyPay usados;
8. aviso explícito de que propina, matrícula, mensalidade e outras cobranças específicas de negócio ficam fora desta tarefa.

# Fora de escopo

- Implementar cobranças específicas como `cobrar propina`, `cobrar matrícula`, `cobrar mensalidade`, `cobrar certificado`, `cobrar assinatura` ou qualquer outro tipo de negócio nomeado.
- Criar endpoints HTTP transacionais de cobrança, consulta de cobrança, sincronização de status, cancelamento, reembolso, reversão, webhook público transacional ou reconciliação manual; nesta tarefa esses fluxos devem existir apenas como funções/handlers Model internos reutilizáveis.
- Criar planos comerciais, contratos, tabelas de preços, descontos, multas, juros ou calendários financeiros específicos.
- Criar UI/frontend de gestão financeira.
- Implementar outro provider de pagamento além da AppyPay nesta tarefa.
- Automatizar faturação fiscal completa como requisito obrigatório; documentos fiscais podem ser preparados como integração base/opcional, mas fluxos fiscais específicos devem ter tarefa própria se necessário.
- Resolver a maturidade produtiva do SDD listado como ALPHA; se SDD for usado, documentar a limitação e permitir desativação por configuração.

# Critérios de aceite

1. existe um módulo financeiro base com contextos `spuri` e `academia` claramente separados;
2. credenciais AppyPay podem ser cadastradas, validadas, ativadas, desativadas e rotacionadas com criptografia e sem exposição de segredos;
3. FPP/ADMIN conseguem ativar/desativar a modalidade de pagamento globalmente e por academia;
4. academias só conseguem usar cobranças próprias quando a modalidade global, modalidade específica e credenciais estiverem ativas;
5. o Spuri consegue criar cobranças genéricas contra academias usando o contexto financeiro da plataforma;
6. academias conseguem criar cobranças genéricas contra estudantes vinculados usando seu próprio contexto financeiro;
7. funções/handlers Model base de gerar cobrança, consultar cobrança, consultar/sincronizar status, cancelar, reembolsar, reverter, processar webhook e reconciliar quando aplicável estão implementadas e documentadas sem endpoints HTTP transacionais diretos;
8. webhooks são idempotentes, seguros e não marcam liquidação definitiva sem confirmação quando exigido;
9. histórico financeiro é imutável e auditável via eventos/projeções;
10. testes obrigatórios da seção 8 passam;
11. `Documentação.md` e documentação técnica afetada estão atualizadas;
12. o PR explica como as funções específicas futuras de propina, matrícula e mensalidade deverão reutilizar as funções base sem duplicar integração AppyPay.

## Procedimento de conclusão

Ao finalizar esta tarefa:

1. atualizar o título interno para `# Implementar módulo base de gestão financeira com AppyPay para Spuri e academias (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
