# Handoff — Redesign da autenticação de webhook AppyPay (secret gerado pelo servidor)

Este documento resume o que estamos fazendo, por quê, o que já foi decidido e implementado, e onde a conversa parou — para retomar numa conversa nova sem perder contexto.

## Repositórios envolvidos

- Backend: https://github.com/fredypdp/spuri-backend (trabalho em andamento, descrito abaixo)
- Frontend: https://github.com/fredypdp/spuripainel (ainda **não iniciado** para esta versão do redesenho — ver seção "Frontend" no fim)

## Contexto — como chegamos aqui

A autenticação de webhook AppyPay já passou por duas rodadas de mudança antes desta:

1. **Tarefa 24** (`docs/Tarefas feitas/24 - Cabeçalho de webhook configurável e resource AppyPay via variável de ambiente.md`, concluída e depurada): tornou o nome do cabeçalho de webhook configurável por credencial (`webhook_header_name`, padrão `X-API-Key`) e moveu `resource` para a variável de ambiente `APPYPAY_RESOURCE`.
2. **Tarefa 30** (`docs/Tarefas feitas/30 - Simplificar autenticação de webhook AppyPay para um único método.md`, concluída e depurada, commit `8ed624f`): eliminou o modo "Basic Auth" (usuário/senha), deixando só o modo "API Key" — mas o nome do cabeçalho **continuava configurável por credencial** e o **segredo continuava sendo digitado pelo usuário** no formulário.

Ao explicar a diferença entre os métodos para o Fredy, ele decidiu ir mais longe: já que a AppyPay só oferece um único par nome/valor de cabeçalho no painel deles, **não há necessidade nenhuma de o nome do cabeçalho variar por credencial** — pode ser um valor fixo, único, para toda a plataforma. E, aproveitando essa simplificação, ele decidiu também **tirar do usuário a responsabilidade de criar o segredo**: o backend deve gerar automaticamente um segredo aleatório quando a credencial é cadastrada, e o usuário só precisa copiá-lo para colar no painel da AppyPay.

## O que foi pedido nesta rodada (mensagem literal do Fredy, resumida)

1. **Nome do cabeçalho único globalmente** — o mesmo valor para toda a plataforma (todas as academias e o Spuri, em qualquer ambiente), não mais configurável por credencial.
2. **Segredo gerado pelo servidor**, não mais digitado pelo usuário — gerado automaticamente quando o usuário registra o cadastro de credenciais.
3. **Formato do segredo**: alfanumérico, 15 caracteres.
4. **Controle de acesso**: o dono do contexto (a própria academia, ou um admin com permissão financeira/"fpp") é o único que pode **consultar** o segredo atual e **requisitar a alteração** (rotação) dele.
5. **No frontend**, depois desta mudança, o usuário só precisa copiar o segredo exibido e colar no painel da AppyPay — não digita mais nada relacionado a autenticação de webhook, exceto colar o resultado lá.
6. Orquestrar **primeiro a tarefa do backend**; o frontend vem depois, numa tarefa separada.

## Decisão de desenho adotada (já implementada no sandbox, ver seção seguinte)

Em vez de manter um campo "tipo" de autenticação (não há mais escolha — só existe um método), o modelo ficou:

- **`WebhookHeaderName`** — nova constante Go **exportada**, fixa, em `internal/finance/appypay.go`:
  ```go
  const WebhookHeaderName = "X-Spuri-Webhook-Secret"
  ```
  (nome escolhido por mim, prefixado com o nome do produto, seguindo convenção comum de cabeçalhos customizados tipo `X-GitHub-*`/`X-Stripe-*`. Se o Fredy preferir outro nome, é só trocar essa constante — nada mais no código depende do valor literal.)

- **Geração do segredo** — nova função usando `crypto/rand` (seguro criptograficamente) + `math/big` para evitar víes de módulo, alfabeto alfanumérico completo (`A-Z a-z 0-9`), 15 caracteres:
  ```go
  const (
  	webhookSecretLength   = 15
  	webhookSecretAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
  )

  func generateWebhookSecret() (string, error) {
  	out := make([]byte, webhookSecretLength)
  	for i := range out {
  		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(webhookSecretAlphabet))))
  		if err != nil {
  			return "", fmt.Errorf("erro ao gerar segredo de webhook: %w", err)
  		}
  		out[i] = webhookSecretAlphabet[n.Int64()]
  	}
  	return string(out), nil
  }
  ```

- **`CredentialInput`** perdeu `WebhookSecret` e `WebhookHeaderName` inteiramente — não são mais aceitos no payload de `POST`/`PUT /financeiro/appypay/credenciais`. Ficou só: `ContextoTipo`, `CodigoAcademia`, `Ambiente`, `ClientID`, `ClientSecret`, `GPOPaymentMethod`, `REFPaymentMethod`.

- **`CredentialView.WebhookHeaderName`** continua existindo no JSON de resposta, mas agora é **sempre** a constante global (não vem mais do banco) — útil para o frontend não precisar hardcodar o valor.

- **`ConfigureCredential` muda de assinatura**: passa a retornar `(CredentialView, string, error)`. A `string` só vem preenchida **na primeira vez** que uma credencial ganha um segredo de webhook (checado via um novo helper `hasWebhookSecret(ctx, id)` que consulta `financeiro_segredos_appypay` sem decifrar nada). Numa atualização de uma credencial que já tem segredo, o segredo existente **não é tocado** e o segundo retorno vem vazio.

- **Três funções novas de serviço**:
  - `CredentialScope(ctx, id) (contexto, academia string, err error)` — resolve o contexto de uma credencial pelo ID, para autorização.
  - `WebhookSecret(ctx, id) (string, error)` — devolve o segredo atual em texto plano ("consultar").
  - `RotateWebhookSecret(ctx, id, userID, userType, ip) (string, error)` — gera um novo segredo, substitui o anterior, grava um evento no ledger (`SegredoWebhookAppyPayRotacionado`, adicionado à whitelist `validEventTypes` em `internal/db/safe_queries.go`, com um `case` novo — bem pequeno, só atualiza `updated_at` — no switch de projeção em `internal/projections/financeiro_projection.go`).

- **`AuthenticateWebhook` fica muito mais simples** — não recebe mais usuário/senha, só `headers http.Header`; extrai `headers.Get(WebhookHeaderName)` uma vez e compara contra o `webhook_secret` de cada credencial.

- **Dois endpoints novos**, registrados em `cmd/server/main.go` dentro do grupo `financeiro` já existente (mesma proteção `RequireAcademiaOuAdmin()`):
  - `GET /financeiro/appypay/credenciais/:id/webhook-secret` → handler `ConsultarSegredoWebhookAppyPay`
  - `POST /financeiro/appypay/credenciais/:id/webhook-secret/rotacionar` → handler `RotacionarSegredoWebhookAppyPay`
  
  Ambos resolvem o escopo da credencial via `CredentialScope` e autorizam via a função `authorizeFinanceScope` já existente (o mesmo mecanismo que já garante que uma academia só mexe nas próprias credenciais, e que um admin precisa da permissão "fpp") — não foi inventado nenhum mecanismo novo de autorização, só reaproveitado o que já existia.

- **`POST /financeiro/appypay/credenciais`** (criação) agora devolve um tipo de resposta novo, só para essa rota:
  ```go
  type CredencialAppyPayCriada struct {
  	finance.CredentialView
  	WebhookSecret string `json:"webhook_secret,omitempty"`
  }
  ```
  incluindo o segredo recém-gerado em texto plano — é a única vez que ele aparece "de graça" numa resposta, fora do `GET .../webhook-secret` dedicado.

## Arquivos alterados até agora (no ambiente de sandbox, ainda não escritos como tarefa formal para o Codex)

Todos em `spuri-backend`:
- `internal/finance/appypay.go` — mudanças descritas acima.
- `internal/handlers/financeiro_handlers.go` — dois handlers novos, dois handlers existentes ajustados, um tipo de resposta novo, um helper novo (`credentialScopeAuthorized`).
- `cmd/server/main.go` — duas rotas novas registradas.
- `internal/db/safe_queries.go` — novo tipo de evento na whitelist.
- `internal/projections/financeiro_projection.go` — novo `case` no switch de projeção.
- `internal/finance/appypay_test.go` — removido o teste do antigo `validHTTPHeaderName` (função não existe mais); adicionados `TestWebhookHeaderNameIsFixedGlobalConstant` e `TestGenerateWebhookSecretLengthAlphabetAndUniqueness`.
- `internal/finance/appypay_integration_test.go` — `configureIntegrationCredential` ajustado para a nova assinatura de 3 retornos; o teste antigo de auth de webhook foi substituído por `TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation`, cobrindo: segredo gerado só na criação, view sempre mostra a constante global, valor/cabeçalho errado não autenticam, atualização não regenera o segredo, `CredentialScope` resolve certo, `WebhookSecret()` devolve o atual, `RotateWebhookSecret` gera um novo diferente do anterior e invalida o antigo, evento gravado no ledger, requisição sem nenhum cabeçalho nunca autentica.

**Nenhum desses arquivos foi commitado nem viroutarefa formal ainda** — está tudo só num clone local de trabalho desta sessão, que não persiste para a próxima conversa. A próxima conversa precisa re-clonar os repositórios (a não ser que o Fredy já tenha aplicado algo manualmente) e, ou reconstruir essas mudanças a partir da especificação acima, ou (melhor) pedir para eu reimplementar do zero seguindo este documento como especificação — o que é rápido de refazer, já que o desenho está todo decidido.

## Validação já feita (parcial) e onde a sessão travou

Confirmado, com as mudanças acima aplicadas:
- `gofmt` limpo.
- `go build ./...` limpo no repositório inteiro.
- `go vet ./...` limpo no repositório inteiro.
- O teste novo `TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation`, rodado sozinho, passa.

**Problema em aberto, não resolvido:** ao rodar a suíte completa dos pacotes `internal/finance` e `internal/handlers` (não só o teste novo), aparecem várias falhas. Descobri, comparando com um clone limpo do `main` **sem nenhuma das minhas mudanças**, que **várias dessas falhas já existem hoje no `main`, independentemente deste trabalho**:

Falhas já presentes no `main` sem nenhuma mudança minha (banco novo, limpo):
- `internal/finance`: `TestIntegrationMatriculaPagamentoFixaValorImpedeDuplicidadeECancelaEmCascata`, `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito`, `TestIntegrationMensalidadeAnularEReativar`
- `internal/handlers`: `TestIntegrationBuscaPublicaMatriculaExigeDoisCamposENaoExibePagamento`, `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula`

Com as minhas mudanças aplicadas (mesmo método, banco novo, limpo), a lista de falhas em `internal/finance` fica maior — inclui as três de cima mais várias de Mensalidade (`TestIntegrationMensalidadeResolvePrecoHistorico`, `TestIntegrationMensalidadePrimeiraConfiguracaoRetroageSemReescreverHistorico`, `TestIntegrationMensalidadeMantemAnoAcademicoHistorico`, `TestIntegrationMensalidadeMantemCursoHistorico`, `TestIntegrationMensalidadeMantemAcademiaHistoricaAposTransferencia`, `TestIntegrationMensalidadeMesInicioEValidadePorAno`, `TestIntegrationMensalidadeConsultaRespeitaAcademia`) e `TestIntegrationCancelChargeAndLateSuccessConflict` — nenhuma delas relacionada a webhook/AppyPay diretamente. Em compensação, em `internal/handlers`, `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` (que falha no `main` puro) **passa** com as minhas mudanças — o que é um indício interessante: talvez esse teste já tenha sido escrito esperando o comportamento novo (cabeçalho global fixo + segredo gerado pelo servidor) e por isso falhe contra o comportamento antigo do `main`. Vale conferir o conteúdo desse teste no início da próxima sessão.

**Duas coisas precisam ser esclarecidas na próxima conversa, nesta ordem:**

1. **Investigar se as falhas extras de Mensalidade/CancelCharge são causadas de fato pelas minhas mudanças em `ConfigureCredential`/`AuthenticateWebhook`, ou se são só efeito de poluição/ordem de execução dos testes dentro do mesmo pacote** (vários testes desse pacote compartilham o mesmo banco sem isolamento entre si — já vimos esse padrão de problema antes, na depuração da tarefa 24). Rodar os testes de Mensalidade isoladamente (`-run TestIntegrationMensalidade`) tanto no `main` puro quanto com as mudanças, num banco limpo a cada vez, deve esclarecer isso rapidamente.
2. **Independente da causa, o `main` já tem pelo menos 5 testes de integração falhando hoje**, sem relação com AppyPay. Isso é uma informação nova para o Fredy, separada desta tarefa — vale avisá-lo diretamente, porque pode ser sintoma de alguma regressão de um merge recente (o commit mais recente antes de eu clonar era `a4b25e4`, um merge de "tornar senha obrigatória no registro", que a princípio não tem nada a ver com matrícula/mensalidade, mas é o suspeito mais óbvio por ser a mudança mais recente).

## Próximos passos, em ordem

1. Reimplementar (ou confirmar que ainda existe, caso o Fredy tenha salvo algo) o código descrito na seção "Decisão de desenho adotada" em `spuri-backend`.
2. Isolar a causa das falhas extras de Mensalidade/CancelCharge (comparação `main` vs. mudanças, testes isolados, banco limpo a cada rodada — não reaproveitar banco entre execuções).
3. Avisar o Fredy sobre as 5 falhas pré-existentes no `main`, independente do resultado do passo 2.
4. Só depois de 2 e 3 resolvidos/esclarecidos, escrever o documento de tarefa formal para o Codex (`docs/Lista de Tarefas/NN - ...md`, seguindo o mesmo formato das tarefas 24/30: prompt recomendado, contexto, resumo executivo, seções numeradas com código exato, fora de escopo, critérios de aceite, nota de validação, procedimento de conclusão).
5. Depois do backend concluído e depurado, orquestrar a tarefa equivalente do frontend (`spuripainel`).

## Frontend — estado (ainda não iniciado para este redesenho)

Existem dois documentos de tarefa **desatualizados** no repositório `spuripainel` (`src/docs/Atualizar cadastro de credenciais AppyPay no frontend...md`, se ainda não executados) que descrevem a versão **anterior** do formulário (com `webhook_header_name` configurável e `webhook_secret` digitado pelo usuário). Eles **não devem ser executados** — ficaram obsoletos com esta mudança. Quando o backend estiver fechado, a tarefa do frontend precisa ser reescrita do zero para reflect:
- Formulário de credenciais não pede mais nada relacionado a webhook (nem nome de cabeçalho, nem segredo) — isso tudo é automático.
- Uma tela/seção nova para exibir o segredo atual (via `GET .../webhook-secret`) com botão de copiar, e uma ação de rotacionar (via `POST .../webhook-secret/rotacionar`) com confirmação antes (rotacionar invalida o segredo antigo imediatamente).
- Mostrar em algum lugar visível o nome fixo do cabeçalho (`X-Spuri-Webhook-Secret`, vindo de `CredentialView.webhook_header_name` — não hardcodar, usar o valor que a API devolve) para o usuário saber o que digitar no painel da AppyPay.
