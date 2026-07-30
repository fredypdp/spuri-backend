---
criado: 2026-07-30 00:00
origem: Revisão arquitetural do módulo financeiro AppyPay
status: pendente
---

# Refatorar módulo financeiro para Event Sourcing/CQRS completo (pendente)

## Prompt recomendado para executar a atualização

Refatore o módulo financeiro base (`internal/finance`) para que credenciais AppyPay, configurações de modalidade, cobranças, webhooks, reconciliações, reembolsos e reversões sigam completamente o padrão de Event Sourcing/CQRS usado no restante do backend. O `spuri_ledger` deve ser a fonte de verdade auditável e imutável para todas as mudanças de estado financeiro, usando aggregate type `Financeiro` e eventos financeiros já permitidos/novos quando necessário. As tabelas `financeiro_*` devem ser tratadas como projeções/read models ou armazenamento operacional de segredos cifrados, nunca como histórico primário substitutivo do ledger. A implementação deve proteger material sensível, não gravar segredos em claro em eventos, respostas, logs ou métricas, manter idempotência e isolamento por contexto (`spuri`/`academia`), suportar rebuild seguro das projeções a partir do ledger, atualizar testes, migrations, documentação técnica, `Documentação.md`, OpenAPI/Swagger quando existir e qualquer documentação afetada. Não criar aliases, wrappers de compatibilidade, fallbacks temporários ou caminhos paralelos que contornem o ledger.

## Contexto

O módulo financeiro base com AppyPay foi criado para gerir credenciais, modalidade de pagamento e operações financeiras genéricas. A implementação atual persiste o estado diretamente em tabelas `financeiro_*` com `payload JSONB` e mantém um `Historico []EventoFinanceiro` embutido nesses payloads. Isso fornece persistência operacional, mas não equivale ao padrão de Event Sourcing/CQRS do restante do sistema, porque as alterações não passam pelo `spuri_ledger`, não recebem versão/hash chain do ledger e não podem ser reconstruídas pelo mecanismo canônico de replay.

O backend já possui infraestrutura de ledger imutável (`spuri_ledger`), repositório de aggregates, validação de aggregate/event types e eventos financeiros cadastrados na whitelist. Portanto, esta tarefa corrige a lacuna arquitetural: o módulo financeiro deve deixar de tratar as tabelas próprias como fonte primária de verdade e passar a registrar toda mudança de estado financeiro como evento no ledger.

A refatoração deve ser feita de modo profissional e seguro, pois o domínio financeiro envolve credenciais, integração com provedor externo, cobranças, webhooks, status monetários e dados sensíveis. O objetivo não é apenas "gravar mais uma cópia" no ledger: o objetivo é tornar o ledger a fonte de verdade, mantendo projeções consistentes, reconstruíveis, idempotentes e sem exposição de segredos.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Fonte de verdade | `spuri_ledger` | Toda mudança financeira relevante é registrada como evento imutável, versionado e auditável |
| Aggregate | `Financeiro` | Novo aggregate/entidades financeiras compatíveis com o repositório de aggregates existente |
| Projeções | Tabelas `financeiro_*` | Usadas para consulta rápida, idempotência e operação, reconstruíveis por replay |
| Segredos | Nunca em claro no ledger | Eventos usam metadados não sensíveis/mascarados; material secreto fica cifrado em armazenamento controlado |
| Credenciais | Eventadas | Cadastro, atualização/rotação, validação, ativação e desativação passam pelo ledger |
| Modalidade | Eventada | Alterações globais, do contexto Spuri e por academia passam pelo ledger |
| Cobranças | Eventadas | Criação, envio ao provider, status, cancelamento, reembolso, reversão e reconciliação passam pelo ledger |
| Webhooks | Eventados e idempotentes | Recebimento e duplicidade ficam auditáveis sem permitir dupla aplicação |
| Rebuild | Obrigatório | Projeções financeiras devem ser reconstruíveis a partir do ledger |
| Segurança | Obrigatória | Sem segredos em logs, respostas, erros, métricas ou payload público de evento |

---

# 1. Objetivo

Refatorar o módulo financeiro para alinhar completamente a persistência e auditoria ao padrão de Event Sourcing/CQRS do backend, garantindo que:

1. o `spuri_ledger` seja a fonte de verdade de todas as mudanças de estado financeiro;
2. as tabelas `financeiro_*` sejam projeções/read models ou armazenamento operacional especializado, não histórico primário substitutivo;
3. credenciais, configurações, cobranças e webhooks sejam reconstruíveis por replay;
4. segredos financeiros sejam protegidos com criptografia forte e nunca persistidos em claro no ledger;
5. cada evento carregue autoria, contexto, versão, data/hora, escopo e metadados suficientes para auditoria;
6. o módulo permaneça genérico, sem introduzir tipos específicos de negócio como propina, matrícula, mensalidade ou assinatura nesta tarefa.

# 2. Diagnóstico da lacuna atual

## 2.1 Persistência direta em tabelas financeiras

Hoje, funções como criação/atualização de credencial e alteração de modalidade serializam o estado inteiro em JSON e fazem `INSERT ... ON CONFLICT ... DO UPDATE` em tabelas financeiras. Essa abordagem atualiza o estado final, mas não cria eventos canônicos no ledger.

## 2.2 Histórico embutido não substitui ledger

O campo `Historico []EventoFinanceiro` dentro dos payloads financeiros deve ser considerado apenas um detalhe transitório da implementação atual. Ele não substitui o ledger porque:

1. fica dentro de uma projeção mutável;
2. não participa da hash chain do `spuri_ledger`;
3. não usa `aggregate_id`, `aggregate_type`, `event_type` e `event_version` canônicos;
4. não é consumido pelo fluxo padrão de rebuild do sistema;
5. pode ser sobrescrito junto com o payload da entidade.

## 2.3 Whitelist financeira já indica o desenho correto

O aggregate type `Financeiro` e vários eventos financeiros já estão previstos na whitelist do sistema. A implementação deve usar essa infraestrutura em vez de manter um caminho paralelo.

---

# 3. Arquitetura obrigatória

## 3.1 Aggregate financeiro

Criar um aggregate financeiro compatível com `internal/domain/aggregates.Aggregate`, usando `aggregate_type = "Financeiro"`.

O desenho pode usar:

1. um aggregate financeiro por contexto (`spuri` ou `academia:{codigo_academia}`); ou
2. um aggregate financeiro por entidade operacional (`credencial`, `cobranca`, `modalidade`), desde que a escolha preserve consistência, versionamento e replay simples.

A decisão deve ser registrada no PR e na documentação técnica. Não usar UUID aleatório desconectado da entidade quando isso dificultar consultas, idempotência ou rebuild.

## 3.2 Eventos financeiros como fonte de verdade

Toda operação que altera estado financeiro deve gerar evento no ledger antes de atualizar a projeção correspondente, dentro de uma estratégia transacional segura.

Eventos mínimos obrigatórios:

### Credenciais AppyPay

- `CredenciaisAppyPayCadastradas`
- `CredenciaisAppyPayAtualizadas`
- `CredenciaisAppyPayValidadas`
- `CredenciaisAppyPayAtivadas`
- `CredenciaisAppyPayDesativadas`

### Modalidade de pagamento

- `ModalidadePagamentoGlobalAlterada`
- `ModalidadePagamentoAcademiaAlterada`
- Adicionar evento específico para contexto Spuri se o evento atual não distinguir esse escopo de forma inequívoca, por exemplo `ModalidadePagamentoSpuriAlterada`.

### Cobranças

- `CobrancaFinanceiraCriada`
- `CobrancaFinanceiraEnviadaAoProvider`
- `CobrancaFinanceiraStatusAtualizado`
- `CobrancaFinanceiraCancelada`

### Reembolsos e reversões

- `ReembolsoFinanceiroSolicitado`
- `ReembolsoFinanceiroStatusAtualizado`
- `ReversaoFinanceiraSolicitada`
- `ReversaoFinanceiraStatusAtualizado`

### Webhooks e reconciliação

- `WebhookFinanceiroRecebido`
- `WebhookFinanceiroIgnoradoComoDuplicado`
- `DivergenciaFinanceiraDetectada`
- `DivergenciaFinanceiraReconciliada`
- Adicionar `ReconciliacaoFinanceiraExecutada` à whitelist se a implementação mantiver esse evento.

## 3.3 Projeções financeiras

As tabelas existentes podem ser mantidas, mas devem passar a ter papel explícito de projeção/read model:

| Tabela | Novo papel esperado |
| --- | --- |
| `financeiro_credenciais_appypay` | Projeção atual das credenciais/metadados visíveis e ponte para segredos cifrados |
| `financeiro_modalidade_pagamento` | Projeção singleton da configuração ativa de modalidade |
| `financeiro_cobrancas` | Projeção atual das cobranças e idempotência por referência externa |
| `financeiro_webhooks_recebidos` | Projeção/índice operacional para idempotência de webhooks |

A implementação deve documentar quais colunas/tabelas são projeções reconstruíveis e quais, se houver, são armazenamento operacional não reconstruível por conter material secreto cifrado.

## 3.4 Rebuild/replay

Adicionar mecanismo de replay/rebuild das projeções financeiras a partir do `spuri_ledger`, seguindo o padrão já existente para projeções do sistema.

O rebuild deve:

1. limpar ou reconstruir apenas projeções financeiras, sem apagar o ledger;
2. aplicar eventos em ordem determinística por `aggregate_id`/`event_version` ou pela regra canônica do event store;
3. ser idempotente;
4. rejeitar ou registrar erro controlado para evento financeiro desconhecido;
5. nunca tentar recuperar segredo em claro a partir de evento público;
6. preservar referências a segredos cifrados conforme o desenho escolhido.

---

# 4. Segurança de credenciais e segredos

## 4.1 Regra principal

Segredos financeiros nunca podem aparecer em claro em:

1. payload do `spuri_ledger`;
2. payload de projeções consultáveis por API;
3. logs;
4. métricas;
5. erros retornados ao cliente;
6. documentação gerada com exemplos reais;
7. eventos de auditoria.

São considerados segredos, no mínimo:

- `client_secret`;
- `webhook_secret`/`webhook_token`;
- `applications[].apiKey`;
- tokens OAuth/cache de token;
- qualquer chave de assinatura de webhook;
- credenciais futuras equivalentes.

## 4.2 Payload seguro de evento

Eventos de credenciais devem conter apenas dados não sensíveis ou mascarados, como:

| Campo | Permitido no ledger? | Observação |
| --- | --- | --- |
| `credential_id` | Sim | UUID interno |
| `contexto_tipo` | Sim | `spuri` ou `academia` |
| `codigo_academia` | Sim | Obrigatório para contexto academia |
| `ambiente` | Sim | `test` ou `prod` |
| `auth_base_url` | Sim | Validar para não conter credencial embutida |
| `api_base_url` | Sim | Deve vir de configuração de ambiente |
| `webapi_base_url` | Sim | Validar para não conter credencial embutida |
| `client_id` | Sim | Não é tratado como segredo primário, mas deve ser evitado em logs desnecessários |
| `resource` | Sim | Não sensível por si só |
| `client_secret` | Não | Nunca em claro |
| `client_secret_mask` | Sim | Ex.: `****1234` |
| `client_secret_encrypted` | Preferencialmente não | Ver seção 4.3 |
| `webhook_secret` | Não | Nunca em claro |
| `apiKey` | Não | Nunca em claro |
| `apiKey_mask` | Sim | Ex.: `****abcd` |
| `applications[].applicationId` | Sim | Se não for tratado como segredo pelo provider |
| `applications[].paymentMethod` | Sim | Necessário para operação/projeção |

## 4.3 Armazenamento de material secreto

Implementar uma das estratégias abaixo e documentar a decisão:

### Opção recomendada — cofre/tabela operacional de segredos cifrados

Criar ou adaptar armazenamento especializado para segredos cifrados, separado do payload público de evento, com no mínimo:

- `credential_id`;
- `secret_version`;
- `secret_type` (`client_secret`, `webhook_secret`, `application_api_key`, etc.);
- `application_id`, quando aplicável;
- `ciphertext`;
- `key_id` ou identificador de versão da chave;
- `algorithm`;
- `created_at`;
- `rotated_at`/`revoked_at`, quando aplicável.

O ledger deve registrar que a versão do segredo foi criada/rotacionada, mas não precisa conter o ciphertext se isso contrariar a política de retenção.

### Opção alternativa — ciphertext no ledger

Só usar esta opção se a equipe decidir que a reconstituição integral por ledger é mais importante que a capacidade operacional de reduzir exposição histórica. Nesse caso:

1. nunca gravar segredo em claro;
2. gravar `ciphertext`, `algorithm` e `key_id`;
3. prever rotação de chave;
4. documentar que versões antigas cifradas permanecerão no ledger append-only;
5. garantir que endpoints e logs nunca retornem ciphertext salvo quando estritamente necessário a componente interna autorizada.

## 4.4 Criptografia

Revisar a criptografia atual para produção:

1. `FINANCE_ENCRYPTION_KEY` deve ser obrigatório em produção;
2. a derivação de chave deve usar material com entropia suficiente;
3. registrar `key_id` para suportar rotação;
4. usar AES-GCM ou alternativa aprovada pela equipe;
5. incluir testes que provem que valores iguais geram ciphertexts diferentes por nonce aleatório;
6. incluir testes que provem que nenhum segredo em claro é persistido ou retornado.

---

# 5. Fluxo de escrita obrigatório

## 5.1 Cadastro de credencial

Ao cadastrar credencial:

1. validar RBAC;
2. validar contexto (`spuri`/`academia`) e isolamento por academia;
3. validar campos obrigatórios;
4. normalizar `api_base_url` a partir da variável de ambiente, não do payload do usuário;
5. cifrar segredos ou gravá-los no cofre operacional;
6. emitir `CredenciaisAppyPayCadastradas` no ledger com payload seguro;
7. atualizar projeção `financeiro_credenciais_appypay` a partir do evento;
8. retornar apenas metadados e máscaras.

## 5.2 Atualização/rotação de credencial

Ao atualizar credencial:

1. validar permissões (`fpp`/`admin` ou academia dona quando permitido);
2. tratar atualização como rotação completa ou parcial conforme decisão documentada;
3. preservar histórico via novo evento, não por sobrescrita silenciosa;
4. incrementar versão via ledger;
5. invalidar/cachear tokens de forma segura se existirem;
6. retornar apenas metadados e máscaras.

## 5.3 Ativação/desativação e validação

Ativar, desativar e validar credenciais deve:

1. exigir `autor_id` não vazio;
2. registrar evento no ledger;
3. guardar `motivo` quando aplicável;
4. alterar a projeção apenas após evento persistido;
5. não apagar segredos antigos sem evento de rotação/revogação correspondente.

## 5.4 Modalidade de pagamento

Alterações de modalidade devem gerar eventos distintos por escopo:

1. global de academias;
2. contexto Spuri;
3. academia específica.

Evitar evento genérico ambíguo que dificulte replay ou auditoria. Se a nomenclatura atual não for suficiente, criar eventos novos e adicioná-los à whitelist.

---

# 6. Cobranças, webhooks e reconciliação

## 6.1 Cobrança idempotente

A criação de cobrança deve manter idempotência por contexto financeiro e `referencia_externa`, mas a idempotência deve ser refletida em projeção reconstruível a partir do ledger.

Fluxo obrigatório:

1. validar modalidade ativa;
2. validar credencial/application ativa;
3. calcular chave idempotente canônica;
4. se já existir cobrança projetada para a chave, retornar a cobrança existente;
5. se não existir, emitir `CobrancaFinanceiraCriada` antes da chamada externa ou aplicar estratégia transacional/outbox documentada;
6. chamar provider;
7. emitir `CobrancaFinanceiraEnviadaAoProvider` ou evento de falha controlado, conforme o resultado;
8. atualizar projeção de cobrança por replay/aplicador.

## 6.2 Chamadas externas e outbox

Como chamadas HTTP externas não participam da transação do banco, a implementação deve escolher uma estratégia segura:

1. evento de intenção antes da chamada + reconciliação posterior; ou
2. outbox transacional para envio assíncrono ao provider; ou
3. outra estratégia documentada que preserve auditabilidade e recuperação.

Não permitir estado em que uma cobrança foi enviada ao provider mas não deixou rastro auditável no ledger.

## 6.3 Webhooks

O processamento de webhook deve:

1. validar assinatura/segredo quando configurado;
2. registrar `WebhookFinanceiroRecebido` no ledger;
3. usar projeção/índice idempotente para bloquear duplicidade;
4. registrar `WebhookFinanceiroIgnoradoComoDuplicado` quando aplicável, se a decisão for auditar duplicatas;
5. nunca liquidar cobrança apenas por payload do webhook sem consulta/validação segura quando a regra do provider exigir confirmação;
6. emitir evento de status somente após confirmação segura.

## 6.4 Reconciliação

A reconciliação deve:

1. emitir evento auditável com `autor_id` ou autor técnico configurado;
2. detectar divergências entre projeção interna e provider;
3. emitir `DivergenciaFinanceiraDetectada` e `DivergenciaFinanceiraReconciliada` quando aplicável;
4. ser idempotente;
5. nunca mascarar divergência apenas atualizando a projeção diretamente.

---

# 7. Migração dos dados existentes

## 7.1 Estratégia obrigatória

Criar migration/rotina controlada para converter dados existentes nas tabelas `financeiro_*` em eventos do ledger sem perder estado operacional.

A migração deve:

1. identificar credenciais, modalidade, cobranças e webhooks já persistidos;
2. emitir eventos compatíveis com o estado atual;
3. preservar IDs existentes sempre que possível;
4. não descriptografar nem expor segredos;
5. registrar apenas máscaras/referências seguras no ledger;
6. criar ou popular armazenamento de segredos cifrados se a estratégia recomendada for adotada;
7. ser idempotente, podendo ser reexecutada sem duplicar eventos;
8. registrar relatório operacional de itens migrados e ignorados.

## 7.2 Dados sem autoria histórica

Se registros antigos não tiverem autoria completa, usar autor técnico explícito, por exemplo:

```text
system:migracao_financeiro_event_sourcing
```

Essa decisão deve aparecer no payload/metadata do evento de migração e na documentação.

## 7.3 Compatibilidade pós-migração

Depois da migração:

1. novas escritas não podem usar persistência direta como fonte primária;
2. código antigo que atualize `financeiro_*` diretamente deve ser removido;
3. testes devem falhar se uma operação financeira alterar projeção sem evento correspondente no ledger.

---

# 8. Autorização, auditoria e isolamento

## 8.1 Autoria obrigatória

Toda função financeira que emite evento deve exigir `autor_id` e `autor_tipo` adequados. Operações técnicas devem usar autor técnico explícito e documentado.

## 8.2 Metadata de auditoria

Usar `SaveWithAudit` ou mecanismo equivalente para preencher metadata com:

- `user_id`;
- `user_type`;
- IP quando disponível;
- origem da operação (`http`, `webhook`, `job`, `migration`, `reconciliation`);
- correlation/request ID quando existir.

## 8.3 Isolamento por contexto

Garantir que:

1. credencial de uma academia nunca seja listada/usada por outra;
2. cobrança de contexto `academia` sempre valide `codigo_academia`;
3. cobrança de contexto `spuri` nunca use credenciais da academia;
4. projeções e consultas respeitem RBAC;
5. eventos tenham contexto suficiente para auditoria multi-instituição.

---

# 9. Observabilidade e tratamento de erros

## 9.1 Logs seguros

Logs financeiros devem conter IDs, contexto, status e códigos de erro, mas nunca segredos, tokens, API keys, ciphertext desnecessário, payload completo de provider ou dados pessoais sensíveis.

## 9.2 Erros sanitizados

Mensagens retornadas ao cliente devem ser úteis operacionalmente, mas não podem revelar:

1. segredo ausente/parcial;
2. token;
3. resposta bruta contendo credenciais;
4. detalhes internos de criptografia;
5. payload completo do provider.

## 9.3 Métricas

Se métricas forem adicionadas, usar labels de baixa cardinalidade e sem dados sensíveis, por exemplo:

- `contexto_tipo`;
- `metodo_pagamento`;
- `status_normalizado`;
- `provider`;
- `resultado`.

---

# 10. Testes obrigatórios

## 10.1 Credenciais

1. criar credencial grava evento `CredenciaisAppyPayCadastradas` no ledger e atualiza projeção;
2. atualizar credencial grava `CredenciaisAppyPayAtualizadas` com versão nova;
3. ativar/desativar grava eventos correspondentes;
4. usuário sem permissão não grava evento nem altera projeção;
5. academia não acessa credencial de outra academia;
6. segredo em claro nunca aparece no ledger, projeção pública, resposta, log capturável ou erro;
7. máscaras são retornadas corretamente;
8. `APPYPAY_API_BASE_URL` continua vindo do ambiente, não do payload.

## 10.2 Criptografia e segredos

1. `FINANCE_ENCRYPTION_KEY` ausente em produção causa erro de configuração;
2. dois segredos iguais geram ciphertexts diferentes por nonce aleatório;
3. rotação de segredo cria nova versão/referência auditável;
4. segredo antigo não é retornado em claro;
5. tentativa de logar payload sensível é bloqueada/sanitizada quando houver mecanismo de sanitização.

## 10.3 Modalidade

1. alteração global de modalidade grava evento e atualiza projeção;
2. alteração do contexto Spuri grava evento inequívoco;
3. alteração de academia específica grava evento com `codigo_academia`;
4. replay reconstrói exatamente o estado final da modalidade;
5. usuário sem permissão não grava evento nem altera projeção.

## 10.4 Cobranças

1. criação de cobrança grava evento de intenção/criação;
2. envio ao provider grava evento de envio/status inicial;
3. falha do provider deixa evento auditável e estado reconciliável;
4. idempotência por `contexto + codigo_academia + referencia_externa` retorna a cobrança existente;
5. replay reconstrói cobrança e índice idempotente;
6. cobrança de estudante de outra academia é rejeitada sem evento de sucesso;
7. moeda é normalizada para `AOA`.

## 10.5 Webhooks e reconciliação

1. webhook válido grava evento de recebimento;
2. webhook duplicado não aplica status duas vezes;
3. duplicidade é auditada conforme decisão da implementação;
4. webhook inválido não altera cobrança;
5. reconciliação detecta divergência e grava evento;
6. replay após webhooks/reconciliação reproduz o estado final.

## 10.6 Rebuild e integridade

1. apagar projeções financeiras e executar rebuild recria o estado a partir do ledger;
2. rebuild é idempotente;
3. evento financeiro desconhecido falha de forma controlada;
4. cadeia de hash do ledger continua válida após operações financeiras;
5. nenhum teste depende de mapa em memória como fonte persistente;
6. concorrência em criação/atualização não gera versões duplicadas nem projeções divergentes.

---

# 11. Documentação obrigatória

Atualizar:

1. `Documentação.md`, explicando que o ledger é a fonte de verdade do módulo financeiro;
2. documentação das rotas financeiras;
3. documentação de variáveis de ambiente financeiras;
4. documentação de segurança/criptografia e rotação de credenciais;
5. OpenAPI/Swagger quando existir;
6. notas de migração dos dados financeiros existentes;
7. instruções de rebuild das projeções financeiras;
8. lista de eventos financeiros e payloads seguros.

A documentação deve deixar claro:

- quais dados vão para o ledger;
- quais dados ficam apenas em armazenamento operacional cifrado;
- como consultar credenciais sem expor segredos;
- como reconstruir projeções;
- como auditar uma alteração financeira.

---

# 12. Fora de escopo

Não implementar nesta tarefa:

1. cobrança específica de propina, matrícula, mensalidade, certificado, assinatura ou qualquer regra de negócio não genérica;
2. novo provider de pagamento além da AppyPay;
3. painel administrativo frontend;
4. alteração de política comercial, comissão, split ou payout não prevista no módulo base;
5. remoção de criptografia existente sem substituição equivalente ou superior;
6. mecanismo temporário que permita gravar financeiro fora do ledger depois da migração.

---

# 13. Critérios de aceite

A tarefa só pode ser considerada concluída quando:

1. toda mudança de estado financeiro relevante gerar evento no `spuri_ledger`;
2. não existir caminho de escrita financeira que atualize projeções como fonte primária sem evento correspondente;
3. projeções financeiras forem reconstruíveis por replay;
4. credenciais e segredos nunca forem gravados em claro no ledger, respostas, logs ou métricas;
5. RBAC e isolamento por academia forem preservados;
6. idempotência de cobranças e webhooks sobreviver a restart e rebuild;
7. migração de dados existentes for idempotente e documentada;
8. testes automatizados cobrirem credenciais, modalidade, cobranças, webhooks, reconciliação, criptografia, autorização, rebuild e concorrência;
9. `Documentação.md` e documentação de API estiverem atualizadas;
10. nenhum alias, fallback temporário ou caminho legado de escrita direta permanecer ativo.

---

# 14. Procedimento de conclusão

Ao concluir a implementação:

1. executar todos os testes unitários e de integração relevantes;
2. executar verificação de integridade/rebuild do ledger e das projeções financeiras;
3. revisar logs para confirmar ausência de segredos;
4. atualizar `Documentação.md` e OpenAPI/Swagger quando existir;
5. mover este arquivo para `docs/Tarefas feitas/`;
6. atualizar `docs/Lista de Tarefas/00 - Índice e ordem de implementação.md`, removendo ou marcando esta tarefa como feita;
7. registrar no PR a estratégia adotada para armazenamento de segredos e para chamadas externas/outbox.
