---
criado: 2026-08-16 15:30
origem: Auditoria profunda do módulo de pagamento (cobrança/pagamento de mensalidades e matrícula) conduzida por Claude (Anthropic) com ambiente real (Go 1.24.4 + PostgreSQL 16), executando build, vet e as suítes de integração repetidamente contra banco limpo. Foco da auditoria: integridade, auditoria, gravação e leitura correta dos dados no ledger. Conduzida como continuação direta da auditoria que originou a tarefa 42, desta vez com a tarefa 42 já verificada como corretamente implementada (checklist de aceitação completo re-executado e confirmado, 5x cada suíte, contra banco recriado do zero).
status: pendente
depende_de: "42 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e matrícula (auditoria pós-tarefa 40).md"
---

# Correção de bug crítico de confirmação de matrícula via consulta de cobrança AppyPay (auditoria pós-tarefa 42)

## Prompt recomendado para executar a atualização

```
Leia por completo o arquivo "docs/Lista de Tarefas/44 - Correção de bug crítico de confirmação de matrícula
via consulta de cobrança AppyPay (auditoria pós-tarefa 42).md". Ele contém 1 correção já totalmente
especificada e validada (diff exato, arquivo e trecho a substituir), mais 1 arquivo de teste de regressão
novo, completo e já validado, que reproduz o bug antes da correção e passa depois dela. Não é necessário
planejar, investigar causa raiz ou decidir a abordagem — tudo isso já foi feito e confirmado
experimentalmente contra PostgreSQL real. Aplique a correção exatamente como especificado em "Localizar" /
"Substituir por", crie o arquivo de teste novo exatamente como especificado, e então execute a seção
"Checklist de aceitação" ao final do documento, na ordem, sem pular nenhum passo. Se qualquer comando do
checklist falhar, pare e reporte o erro — não prossiga para o próximo item nem tente uma correção diferente
da especificada aqui sem antes reportar.
```

## Contexto

A tarefa 42 corrigiu 5 bugs críticos de integridade do ledger no fluxo de **mensalidades**, e foi
**re-verificada nesta auditoria como corretamente implementada**: os 5 diffs batem, campo a campo, com o
código atualmente em `main` (commit `94c57b0`); o arquivo de teste novo
`internal/finance/financeiro_ledger_integrity_test.go` é idêntico, byte a byte, ao especificado no documento;
e o checklist de aceitação completo foi re-executado do zero neste ambiente (Go 1.24.4 + PostgreSQL 16) —
`go build`/`go vet` limpos, `internal/finance` com 20 testes passando 5 execuções seguidas contra banco
recriado, `internal/handlers` 5 execuções seguidas, e `go test ./...` do repositório inteiro limpo. **Nenhuma
correção é necessária na tarefa 42.**

A auditoria então avançou para o segundo objetivo: revisar o módulo de **pagamento**, desta vez olhando
especificamente para o fluxo de **matrícula** (taxa de matrícula/inscrição), que a tarefa 42 não cobriu (ela
tratou apenas de mensalidade/propina). O mesmo método da tarefa 42 foi aplicado: em vez de reler os testes já
existentes, foram exercitados diretamente os caminhos de código que nenhum teste do repositório jamais tinha
chamado para matrícula — em particular, o caminho de **consulta/polling de status de cobrança**
(`GET /financeiro/appypay/cobrancas/:id`, `Service.ConsultCharge`), que já era coberto por teste para
mensalidade (é exatamente o que `TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago`, da
tarefa 42, exercita), mas nunca tinha sido exercitado para matrícula.

Isso revelou **1 bug novo, catastrófico**: quando o pagamento da matrícula é confirmado pela AppyPay através
da consulta de status — o caminho normal de reconciliação para pagamentos GPO/REF, que **nunca** retornam
"success" de forma síncrona na criação da cobrança (GPO exige aprovação por push no telemóvel do pagador; REF
exige que o pagamento seja feito depois, numa referência bancária/multicaixa) — **o sistema nunca efetiva a
matrícula**: o estudante nunca é criado e a solicitação nunca sai de
`aprovada_pendente_pagamento_matricula`, mesmo com a cobrança já mostrando `status: "Success"` no read model.
Isto é estruturalmente o mesmo tipo de falha que a tarefa 42 corrigiu para mensalidades (Bug 2/3 daquele
documento: um pagamento confirmado pelo provedor mas nunca refletido no lado do Spuri), só que desta vez no
fluxo de matrícula, e alcançável pelo caminho de consulta em vez do de mensalidade recorrente.

A correção abaixo foi aplicada neste mesmo ambiente real (Go 1.24.4 + PostgreSQL 16) e validada: `go
build`/`go vet` limpos, um teste de regressão novo que falha antes da correção (reproduzindo o bug) e passa
depois dela, a suíte `internal/handlers` completa (72 testes, incluindo o novo) executada 5 vezes seguidas
contra banco recriado do zero — todas as 5 verdes —, a suíte `internal/finance` também 5 vezes seguidas
verde, e `go test ./...` do repositório inteiro limpo. O diff abaixo é exatamente o que foi validado; não é
necessário alterá-lo.

---

## Bug 1 (CRÍTICO) — Confirmação de pagamento de matrícula via consulta de cobrança nunca efetiva o vínculo

**Arquivo:** `internal/handlers/financeiro_handlers.go`

**Causa raiz confirmada:** o handler `ConsultarCobrancaAppyPay` (rota
`GET /financeiro/appypay/cobrancas/:id`, protegida por `RequireAcademiaOuAdmin()`, o mesmo endpoint genérico
usado para consultar **qualquer** cobrança, seja de mensalidade ou de matrícula) chama
`FinanceiroService.ConsultCharge` e devolve o resultado ao cliente, sem mais nenhuma ação.

`ConsultCharge` (dentro do pacote `finance`) já chama internamente `confirmMensalidadeCharge` quando a
consulta revela `status: "Success"` — mas essa função é, por desenho, um no-op silencioso para qualquer
cobrança cujo payload não contenha a chave `"mensalidades"` (ver `mensalidadesDoPayload`, que devolve slice
vazio nesse caso, fazendo `confirmMensalidadeCharge` retornar `nil` sem gravar nada). **Toda cobrança de
matrícula** tem esse formato de payload (contém `codigo_solicitacao`, não `mensalidades`), então
`confirmMensalidadeCharge` sempre no-opa para elas — isto é correto e intencional, `confirmMensalidadeCharge`
não deveria mesmo mexer em matrícula.

O problema é que **nada mais** cobre esse caso. A efetivação do vínculo de matrícula
(`efetivarVinculoMatriculaPaga`, que cria o `Estudante` e transiciona a `SolicitacaoMatricula` para
`aprovada`) só é chamada em dois lugares do código inteiro:

1. `internal/handlers/solicitacao_matricula_handlers.go`, dentro do handler `IniciarPagamentoMatricula`,
   quando a resposta **síncrona** da criação da cobrança já vem com `status == "success"`. Na prática, isto
   não acontece para GPO (assíncrono, depende de aprovação por push) nem para REF (assíncrono, depende de
   pagamento posterior numa referência bancária) — os dois únicos métodos de pagamento de matrícula
   suportados pelo sistema (ver `MetodoPagamento` aceito em `MatriculaPagamentoInput`).
2. `internal/handlers/financeiro_handlers.go`, dentro de `ReceberWebhookAppyPay`, quando o webhook da AppyPay
   entrega um evento de sucesso.

`ConsultarCobrancaAppyPay` **não é nenhum destes dois lugares**. Se a confirmação do pagamento chega através
da consulta — o fluxo natural que a própria equipe da academia usaria no painel para reconciliar um
pagamento REF ("o candidato diz que já pagou na referência, deixa eu conferir o status") — a AppyPay confirma
o pagamento, o Spuri grava `status: "Success"` em `financeiro_cobrancas` e devolve isso na resposta da
consulta, mas **o estudante nunca é criado e a solicitação fica presa para sempre** em
`aprovada_pendente_pagamento_matricula`. Não há nenhum job em background, cron ou rotina de reconciliação que
cubra esse caso (confirmado por busca em todo o repositório: `efetivarVinculoMatriculaPaga` só é chamada
nesses dois lugares).

**Efeito observado (reproduzido com teste de ponta a ponta contra Postgres real, ver `Bug 1 confirmado` no
histórico de execução deste documento):** cobrança de matrícula REF criada com `status: "Pending"` (como
acontece sempre na prática); status alterado para `"Success"` no provedor mock; consulta via
`ConsultarCobrancaAppyPay` devolve `200 OK` com `status: "Success"` — mas
`projection_solicitacoes_matricula.status` continua `"aprovada_pendente_pagamento_matricula"` e nenhuma linha
é criada em `projection_estudantes`.

**Correção:** replicar, dentro de `ConsultarCobrancaAppyPay`, exatamente o mesmo padrão já usado em
`ReceberWebhookAppyPay` — se a consulta revela `status: "success"`, resolver o código da solicitação a partir
do identificador da cobrança (`FinanceiroService.CodigoSolicitacaoDaCobranca`, já existente e já usado pelo
próprio `ReceberWebhookAppyPay`) e chamar `efetivarVinculoMatriculaPaga`. Para cobranças de mensalidade,
`CodigoSolicitacaoDaCobranca` não encontra nenhuma solicitação correspondente (devolve string vazia ou erro,
dependendo da implementação — em qualquer um dos dois casos a chamada a `efetivarVinculoMatriculaPaga` é
pulada), então a mudança não afeta em nada o fluxo de mensalidade — só fecha a lacuna do fluxo de matrícula.
`efetivarVinculoMatriculaPaga` já é idempotente e segura contra reentrada/redelivery (ver o comentário que já
existe acima da própria função: "efetivarVinculoMatriculaPaga is safe on webhook redelivery: the terminal
aggregate transition is checked before an Estudante can be created again"), então não há risco de duplicar o
estudante se a consulta for chamada várias vezes, ou se o webhook e a consulta chegarem em qualquer ordem.

**Localizar** (dentro de `func ConsultarCobrancaAppyPay(c *gin.Context)`, é o trecho final da função):

```go
	out, err := FinanceiroService.ConsultCharge(c.Request.Context(), contexto, academia, c.Param("id"), id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}
```

**Substituir por:**

```go
	out, err := FinanceiroService.ConsultCharge(c.Request.Context(), contexto, academia, c.Param("id"), id.String(), t, c.ClientIP())
	if err != nil {
		financeError(c, err)
		return
	}
	// A cobrança pode ser de matrícula: diferente de uma cobrança de
	// mensalidade (confirmada via confirmMensalidadeCharge dentro do
	// próprio ConsultCharge), a efetivação do vínculo de matrícula
	// (criação do estudante e transição da solicitação) não faz parte do
	// pacote financeiro e precisa ser acionada aqui, exatamente como já é
	// feito em ReceberWebhookAppyPay e na criação síncrona da cobrança em
	// IniciarPagamentoMatricula. Sem isto, uma cobrança de matrícula que só
	// é confirmada pela AppyPay quando alguém consulta o status (fluxo
	// normal para GPO/REF, que nunca retornam "success" na criação) nunca
	// efetiva a matrícula.
	if strings.EqualFold(strings.TrimSpace(out.Status), "success") {
		if codigo, err := FinanceiroService.CodigoSolicitacaoDaCobranca(c.Request.Context(), c.Param("id")); err == nil && codigo != "" {
			if err := efetivarVinculoMatriculaPaga(c, codigo); err != nil {
				utils.RespondWithInternalError(c, err)
				return
			}
		}
	}
	c.JSON(http.StatusOK, out)
}
```

Não é necessário adicionar nenhum import: `strings` já está importado em
`internal/handlers/financeiro_handlers.go`, e `utils`, `finance` e `efetivarVinculoMatriculaPaga` (esta
última definida no mesmo pacote, em `solicitacao_matricula_handlers.go`) já estão disponíveis.

---

## Teste de regressão novo

Criar o arquivo `internal/handlers/financeiro_matricula_consulta_test.go` com exatamente este conteúdo. Ele
reutiliza os helpers de seed já existentes no pacote (`integrationFinanceClient`,
`seedAcademiaParaMatriculaWebhook`, `seedSolicitacaoMatriculaPendenteComLedger`, já usados por
`TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula`). Antes da correção acima, este teste falha
(`status da solicitação = "aprovada_pendente_pagamento_matricula", esperado "aprovada"...`); depois da
correção, passa.

```go
package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
	"spuri/internal/projections"
)

type matriculaConsultaMockTransport struct{ status string }

func (t *matriculaConsultaMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body := `{"id":"provider-charge-consulta","status":"Pending"}`
	switch {
	case strings.Contains(req.URL.Path, "/oauth2/token"):
		body = `{"access_token":"test-token","expires_in":3600}`
	case req.Method == http.MethodGet:
		body = `{"id":"provider-charge-consulta","status":"` + t.status + `"}`
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// TestIntegrationConsultarCobrancaAppyPayNaoEfetivaMatriculaAposSuccess
// reproduz o fluxo de polling: a academia (ou o candidato) consulta o status
// de uma cobrança de matrícula GPO/REF através do endpoint genérico
// GET /financeiro/appypay/cobrancas/:id (o mesmo endpoint e o mesmo caminho
// de código, finance.Service.ConsultCharge, usados pelo fluxo de
// mensalidades e exercitados pelo teste
// TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago da
// tarefa 42). A cobrança é criada como "Pending" (comportamento real de
// GPO/REF: a criação nunca retorna "Success" de forma síncrona) e só depois
// passa a "Success" na consulta. Diferente do webhook (que já efetiva o
// vínculo corretamente, ver
// TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula), este
// caminho nunca chama efetivarVinculoMatriculaPaga: confirmMensalidadeCharge
// é chamado incondicionalmente a partir de ConsultCharge, mas é um no-op
// silencioso para qualquer cobrança cujo payload não contenha
// "mensalidades" (como é o caso de toda cobrança de matrícula). O
// resultado: a cobrança fica "Success" no read model, mas o estudante nunca
// é criado e a solicitação nunca sai de
// "aprovada_pendente_pagamento_matricula".
func TestIntegrationConsultarCobrancaAppyPayNaoEfetivaMatriculaAposSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	t.Setenv("ENV", "test")
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")

	academia := "WC" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	seedAcademiaParaMatriculaWebhook(t, client, academia)
	codigo, codigoEstudante := seedSolicitacaoMatriculaPendenteComLedger(t, client, academia, 750)

	transport := &matriculaConsultaMockTransport{status: "Pending"}
	service := finance.NewService(client)
	service.SetHTTPClient(&http.Client{Transport: transport})
	if _, err := service.ConfigureCredential(context.Background(), nil, finance.CredentialInput{
		ContextoTipo: finance.ContextoAcademia, CodigoAcademia: academia,
		ClientID: "integration-client", ClientSecret: "integration-secret",
		GPOPaymentMethod: "GPO_INTEGRATION", REFPaymentMethod: "REF_INTEGRATION",
		WebhookSecret: "webhook-secret-" + codigo,
	}, "integration-test", "sistema", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}

	charge, err := service.IniciarPagamentoMatricula(context.Background(), finance.MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMatricula falhou: %v", err)
	}
	if strings.EqualFold(charge.Charge.Status, "success") {
		t.Fatalf("cobrança REF retornou success na criação, cenário não reproduz o fluxo real (deveria ser Pending): %q", charge.Charge.Status)
	}

	previousService := FinanceiroService
	FinanceiroService = service
	t.Cleanup(func() { FinanceiroService = previousService })

	// AppyPay confirma o pagamento de forma assíncrona (pagamento na
	// referência bancária). A academia/candidato descobre isso consultando o
	// status da cobrança pelo endpoint genérico, exatamente como o teste da
	// tarefa 42 fez para mensalidades via ConsultCharge.
	transport.status = "Success"
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/appypay/cobrancas/"+charge.Charge.ID.String(), nil)
	ctx.Params = gin.Params{{Key: "id", Value: charge.Charge.ID.String()}}
	ctx.Set("dbClient", client)
	ctx.Set("repository", db.NewAggregateRepository(client))
	ctx.Set("user_id", uuid.New())
	ctx.Set("user_type", "academia")
	ctx.Set("codigo_academia", academia)

	ConsultarCobrancaAppyPay(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ConsultarCobrancaAppyPay retornou %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(strings.ToLower(recorder.Body.String()), "success") {
		t.Fatalf("consulta não refletiu o status Success da AppyPay: %s", recorder.Body.String())
	}
	if err := projections.NewSolicitacaoMatriculaProjection(client).Rebuild(); err != nil {
		t.Fatal(err)
	}
	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatal(err)
	}

	var status string
	if err := client.DB().QueryRow(`SELECT status FROM projection_solicitacoes_matricula WHERE codigo_solicitacao=$1`, codigo).Scan(&status); err != nil {
		t.Fatal(err)
	}
	var estudantes int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_estudantes WHERE codigo_estudante=$1`, codigoEstudante).Scan(&estudantes); err != nil {
		t.Fatal(err)
	}

	// A AppyPay confirmou o pagamento (a consulta acima devolveu "Success" e
	// isso já está persistido em financeiro_cobrancas): a solicitação deve
	// estar "aprovada" e o estudante deve existir, exatamente como acontece
	// quando a confirmação chega por webhook
	// (TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula).
	if status != "aprovada" {
		t.Fatalf("status da solicitação = %q, esperado \"aprovada\" após a consulta confirmar o pagamento", status)
	}
	if estudantes != 1 {
		t.Fatalf("estudantes criados = %d, esperado 1 após a consulta confirmar o pagamento", estudantes)
	}
}
```

Observação sobre `ctx.Set("repository", db.NewAggregateRepository(client))`: diferente de
`ConsultCharge` (que só grava no ledger e não lê agregados), `efetivarVinculoMatriculaPaga` usa
`getRepository(c)` para carregar o agregado `SolicitacaoMatricula` e depois `SaveWithAudit` o `Estudante`
novo — por isso o teste precisa colocar um repositório no contexto do Gin, exatamente como os demais testes
de handlers que exercitam esse caminho (ver `seedSolicitacaoMatriculaPendenteComLedger` e o teste do
webhook). Sem isso o teste entra em pânico com nil pointer, não é uma falha do código de produção — em
produção o middleware já popula `"repository"` no contexto de toda requisição.

Observação sobre os dois `Rebuild()` antes das leituras finais: `SaveWithAudit` só grava no ledger e notifica
o `Manager` de projeções (`notifyLedgerWritten`), que processa de forma assíncrona em background — no
servidor real esse `Manager` está sempre rodando (`StartProcessing`, iniciado em `cmd/server/main.go`), mas
o teste não sobe esse processo de fundo, então precisa forçar a reprojeção manualmente antes de consultar as
tabelas de leitura, exatamente como já faz
`TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula`.

---

## Ordem de execução recomendada

1. Aplicar a correção do Bug 1 em `internal/handlers/financeiro_handlers.go`, exatamente como especificado
   acima.
2. Criar `internal/handlers/financeiro_matricula_consulta_test.go` com o conteúdo especificado acima.
3. Rodar `gofmt -w internal/handlers/financeiro_handlers.go internal/handlers/financeiro_matricula_consulta_test.go`.
4. Executar a checklist de aceitação abaixo, na ordem.

---

## Checklist de aceitação

Execute cada item na ordem. Se qualquer um falhar, pare e reporte — não prossiga nem tente uma correção
diferente da especificada acima sem antes reportar o erro exato.

1. **Build e vet limpos:**
   ```
   go build ./...
   go vet ./...
   ```
   Ambos devem terminar sem nenhuma saída de erro.

2. **Suíte `internal/handlers`, 5 execuções seguidas, banco recriado do zero a cada vez:**
   ```
   for i in 1 2 3 4 5; do
     psql -c "DROP DATABASE IF EXISTS spuri_test;" -U postgres
     psql -c "CREATE DATABASE spuri_test;" -U postgres
     RUN_POSTGRES_INTEGRATION=1 DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
       DB_NAME=spuri_test DB_SSLMODE=disable APPYPAY_RESOURCE=integration-resource \
       FINANCE_ENCRYPTION_KEY=01234567890123456789012345678901 ENV=test \
       go test -count=1 ./internal/handlers/... -run TestIntegration -v
   done
   ```
   Todas as 5 execuções devem terminar com `PASS`/`ok`, sem nenhum `FAIL`. Confirme especificamente que
   `TestIntegrationConsultarCobrancaAppyPayNaoEfetivaMatriculaAposSuccess` aparece como `--- PASS` em cada
   execução, e que `TestIntegrationReceberWebhookAppyPayEfetivaVinculoMatricula` continua passando (a
   correção não deve alterar o comportamento do caminho de webhook).

3. **Suíte `internal/finance`, 5 execuções seguidas, banco recriado do zero a cada vez (garantir que nada no
   fluxo de mensalidade foi afetado, já que `ConsultCharge` é compartilhado entre mensalidade e matrícula):**
   ```
   for i in 1 2 3 4 5; do
     psql -c "DROP DATABASE IF EXISTS spuri_test;" -U postgres
     psql -c "CREATE DATABASE spuri_test;" -U postgres
     RUN_POSTGRES_INTEGRATION=1 DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
       DB_NAME=spuri_test DB_SSLMODE=disable APPYPAY_RESOURCE=integration-resource \
       FINANCE_ENCRYPTION_KEY=01234567890123456789012345678901 ENV=test \
       go test -count=1 ./internal/finance/... -run TestIntegration -v
   done
   ```
   Todas as 5 execuções devem terminar com `PASS`/`ok`, sem nenhum `FAIL`.

4. **Suíte completa do repositório (sem a flag de integração):**
   ```
   go test ./...
   ```
   Todos os pacotes devem terminar `ok`.

5. **Diff final — confirmar que apenas os arquivos esperados foram alterados:**
   ```
   git diff --stat
   git status --short
   ```
   Deve mostrar exatamente este arquivo modificado, e nenhum outro (nem `go.mod`, nem `go.sum`):
   - `internal/handlers/financeiro_handlers.go`

   E exatamente este arquivo novo, não rastreado antes desta tarefa:
   - `internal/handlers/financeiro_matricula_consulta_test.go`

Se todos os itens passarem, a tarefa está concluída. Mova este arquivo de
`docs/Lista de Tarefas/43 - Correção de bug crítico de confirmação de matrícula via consulta de cobrança
AppyPay (auditoria pós-tarefa 42).md` para
`docs/Tarefas feitas/43 - Correção de bug crítico de confirmação de matrícula via consulta de cobrança
AppyPay (auditoria pós-tarefa 42).md`, e atualize o front-matter (`status: feito`) antes de finalizar.

Aproveite também para mover
`docs/Lista de Tarefas/42 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e
matrícula (auditoria pós-tarefa 40).md` para
`docs/Tarefas feitas/42 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e
matrícula (auditoria pós-tarefa 40).md`, atualizando também o seu front-matter (`status: feito`): esta
auditoria confirmou, re-executando o checklist de aceitação inteiro daquele documento do zero, que a tarefa
42 foi corretamente implementada e nenhuma correção adicional é necessária nela — ela só não tinha sido
movida porque a sessão anterior do Codex parou antes de completar o checklist (ambiente sem `psql`/PostgreSQL
disponível) e, corretamente, não marcou a tarefa como concluída sem essa confirmação.
