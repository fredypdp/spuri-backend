---
criado: 2026-08-26
origem: "Auditoria solicitada por Fredy Luís ao módulo financeiro (internal/finance), para confirmar que existe, para cada método de pagamento aceite pelo sistema (REF, GPO, GPO_QR), apenas uma função de geração de cobrança reutilizada por todo o sistema. Esta tarefa foi orquestrada por Claude (Anthropic) a partir da leitura completa do código-fonte de spuri-backend (branch main): internal/finance/appypay.go, internal/finance/matricula.go, internal/finance/mensalidade.go, internal/finance/pagamentos_unificado.go, internal/handlers/financeiro_handlers.go, e de uma busca exaustiva (grep) por todos os pontos do repositório que chamam CreateCharge e CreateGPOQRCode."
status: pendente
depende_de: nenhuma
---

# Unificar geração de cobrança (REF, GPO, GPO_QR) num único ponto para matrícula e mensalidade

## Prompt recomendado para executar a tarefa

```
Leia por completo o arquivo "docs/Lista de Tarefas/67 - Unificar geração de cobrança REF-GPO-GPOQR num único
ponto para matrícula e mensalidade.md". Ele contém 1 arquivo novo completo, pronto para criar
(internal/finance/cobranca_geracao.go), 1 arquivo de teste novo completo, pronto para criar
(internal/finance/cobranca_geracao_integration_test.go), e 2 alterações em arquivos existentes já
especificadas como diff exato ("Localizar" / "Substituir por"). Todas as decisões de design (nome da função,
nome do arquivo, assinatura, quais campos são obrigatórios vs. opcionais) já foram tomadas e estão na seção
"Decisões de design já tomadas" — não é necessário planejar, investigar causa raiz ou decidir abordagem
nenhuma. Aplique as duas alterações exatamente como especificado em cada "Localizar" / "Substituir por", crie
os 2 arquivos novos exatamente como especificados (copie o conteúdo literalmente, não retype à mão), e então
execute a seção "Checklist de aceitação" ao final do documento, na ordem, sem pular nenhum passo.

Diferente de tarefas anteriores deste projeto: desta vez o diff INTEIRO já foi compilado e testado pelo
orquestrador antes de escrever este documento — build (`go build ./...`), vet (`go vet ./...`), gofmt
(`gofmt -l .`) e a suíte de testes completa do repositório (`go test ./...`, incluindo os testes de integração
com PostgreSQL real de internal/finance e internal/handlers, e os 3 novos testes de integração criados por
esta tarefa) foram executados repetidamente (8 execuções da suíte de internal/finance e internal/handlers em
banco limpo, mais uma rodada com `-race`) e passaram 100% das vezes, sem nenhuma regressão. Você não tem
PostgreSQL nem Docker disponíveis no seu ambiente — isso é esperado e não é um bloqueio: a seção "Nota sobre
validação — o que já foi feito vs. o que você deve rodar" explica exatamente o que você precisa (e não precisa)
executar. Se qualquer comando do checklist que você conseguir rodar falhar, pare e reporte o erro exato — não
prossiga para o próximo item nem invente uma correção diferente da especificada aqui sem antes reportar.
```

## Contexto

Fredy pediu uma depuração do módulo financeiro para confirmar que, para cada método de pagamento aceite pelo
sistema (`REF`, `GPO`, `GPO_QR`), existe apenas uma função de geração de cobrança, reutilizada por todas as
outras funções do sistema — para facilitar depuração e refatoração futuras.

**O que a auditoria confirmou estar correto (nenhuma ação necessária):**

A camada que efetivamente fala com a AppyPay já está centralizada corretamente em
`internal/finance/appypay.go`:
- `CreateCharge` é a única função que faz `POST /charges` (cobre os métodos `REF` e `GPO`).
- `CreateGPOQRCode` é a única função que faz `POST /qr-codes` (cobre `GPO_QR`).

Confirmado por busca exaustiva no repositório: nenhum outro arquivo, em nenhum pacote, chama o cliente HTTP da
AppyPay diretamente para criar uma cobrança ou QR Code. Isso já está de acordo com o comentário de pacote no
topo de `appypay.go` ("Package finance is the only package allowed to call AppyPay's HTTP API").

**O problema real, confirmado por leitura direta do código-fonte:**

Um nível acima da chamada à AppyPay, existe a lógica que **decide** qual das duas funções chamar a partir do
método de pagamento escolhido pelo pagador (candidato ou estudante) — e essa lógica de decisão estava
**duplicada byte-a-byte** em dois lugares:

1. `IniciarPagamentoMatricula`, em `internal/finance/matricula.go` (linhas 216–232 antes desta correção).
2. `IniciarPagamentoMensalidades`, em `internal/finance/mensalidade.go` (linhas 347–365 antes desta correção).

Ambas as funções continham exatamente o mesmo bloco de decisão:

```go
if in.MetodoPagamento == "GPO_QR" {
    // chama CreateGPOQRCode, devolve QRCodeResult
}
info := map[string]any{}
if in.MetodoPagamento == "GPO" {
    info["phoneNumber"] = strings.TrimSpace(in.Telefone)
}
// chama CreateCharge, devolve QRCodeResult{ChargeResult: charge}
```

Isso é confirmado inclusive pelos próprios comentários já existentes no código (`MatriculaPagamentoView.Charge`
e `MensalidadePagamentoView.Charge` já se referenciam mutuamente como "declarados como `QRCodeResult` pelo
mesmo motivo"), evidenciando que os desenvolvedores já haviam notado a semelhança, mas nunca extraído a lógica
comum. Isso é exatamente o tipo de duplicação que este pedido de auditoria queria eliminar: se a AppyPay mudar
um requisito de um dos três métodos (por exemplo, um novo campo obrigatório para `GPO`), há dois lugares para
lembrar de atualizar, e um deles pode ser esquecido silenciosamente.

**A correção**: extrair esse bloco de decisão para uma única função nova, `gerarCobranca`, num arquivo
dedicado (`internal/finance/cobranca_geracao.go`), e fazer as duas funções (`IniciarPagamentoMatricula` e
`IniciarPagamentoMensalidades`) chamá-la, em vez de duplicar a decisão.

**O que deliberadamente NÃO muda** (e não deve ser tocado por esta tarefa): os endpoints administrativos
`CriarCobrancaAppyPay` e `GerarQRCodeAppyPay`, em `internal/handlers/financeiro_handlers.go`, continuam
chamando `CreateCharge`/`CreateGPOQRCode` diretamente. Eles criam uma cobrança "avulsa", com controlo total
sobre o payload (incluindo campos que `gerarCobranca` não expõe, como `Options`, `Notify`, `MinAmount`,
`MaxTransactions`), e não partem de uma obrigação (matrícula/mensalidade) nem de uma simples escolha de método
de pagamento pelo pagador. Não são o mesmo caso de uso, e fundi-los aumentaria o acoplamento sem necessidade.

---

## Escopo desta tarefa (e o que NÃO fazer)

**Arquivos a criar:**
- `internal/finance/cobranca_geracao.go` (conteúdo completo na Seção 1)
- `internal/finance/cobranca_geracao_integration_test.go` (conteúdo completo na Seção 2)

**Arquivos a alterar (diff exato, ver Seções 3 e 4):**
- `internal/finance/matricula.go`
- `internal/finance/mensalidade.go`

**Arquivos a remover:** nenhum. Esta tarefa não deixa código morto para trás — o bloco duplicado é
substituído, não apenas desativado.

**`go.mod` / `go.sum`: NÃO alterar.** Esta correção não introduz nenhuma dependência nova (usa apenas
pacotes já importados em `internal/finance`: `context`, `strings`, `encoding/json`, `io`, `net/http`, `sync`,
`testing`, `time`, `github.com/google/uuid`). Se o seu ambiente reportar necessidade de rodar `go mod tidy` ou
alterar `go.mod`/`go.sum` por qualquer motivo relacionado a esta tarefa, PARE e reporte — não é esperado e pode
indicar que algo foi copiado incorretamente.

**Não faça isto** (fora de escopo, mesmo que pareça relacionado):
- Não altere `internal/finance/appypay.go` (`CreateCharge`, `CreateGPOQRCode`, `ChargeRequest`,
  `QRCodeRequest`, `QRCodeResult`, `ChargeResult`). Estas funções e tipos já estão corretos e são usados,
  sem alteração, pela nova função `gerarCobranca`.
- Não altere `internal/handlers/financeiro_handlers.go`. Os endpoints administrativos `CriarCobrancaAppyPay`
  e `GerarQRCodeAppyPay` continuam chamando `CreateCharge`/`CreateGPOQRCode` diretamente — isso é intencional,
  ver "Contexto" acima.
- Não altere `internal/finance/pagamentos_unificado.go`, `internal/finance/mensalidade_pendencias.go`,
  `internal/finance/mensalidade_pendencias_batch.go` nem qualquer outro arquivo do módulo financeiro além dos
  quatro listados acima.
- Não altere nenhum teste pré-existente (`appypay_test.go`, `appypay_integration_test.go`,
  `mensalidade_integration_test.go`, `mensalidade_remocao_integration_test.go`,
  `qrcode_regression_integration_test.go`, `matricula_remocao_integration_test.go`, etc.). Todos eles já
  passam sem nenhuma modificação depois desta correção — se algum precisar de ajuste, isso significa que o
  diff foi aplicado incorretamente; pare e reporte, não edite o teste para "fazer passar".
- Não mude o comportamento observável de `IniciarPagamentoMatricula` nem de `IniciarPagamentoMensalidades`.
  Esta é uma refatoração pura (mesma entrada → mesma saída, mesmo request enviado à AppyPay); não é uma
  correção de bug.

---

## Decisões de design já tomadas

- **Nome da função compartilhada**: `gerarCobranca` (método não-exportado de `*Service`, minúsculo — nunca
  cruza fronteira de pacote nem é serializado em JSON, então não há motivo para exportar).
- **Nome do arquivo novo**: `internal/finance/cobranca_geracao.go`. Não foi colocado dentro de `appypay.go`
  (que já tem 1600+ linhas e é dedicado à integração bruta com a AppyPay) nem dentro de `matricula.go`/
  `mensalidade.go` (isso manteria a duplicação conceitual, só movida de lugar) — é uma terceira
  responsabilidade (decidir qual função de baixo nível chamar) que merece arquivo próprio, seguindo a regra
  de não misturar lógicas diferentes no mesmo arquivo.
- **Assinatura**: `func (s *Service) gerarCobranca(ctx context.Context, in gerarCobrancaInput, actorID,
  actorType, ip string) (QRCodeResult, error)`. Os três últimos parâmetros (`actorID, actorType, ip`) foram
  mantidos como parâmetros posicionais separados (em vez de dentro do struct `gerarCobrancaInput`) porque é
  assim que `CreateCharge` e `CreateGPOQRCode` já os recebem — manter a mesma forma evita uma camada extra de
  tradução sem motivo.
- **Struct de entrada**: `gerarCobrancaInput` (não-exportado, mesmo motivo do item acima). Contém todos os
  campos que hoje divergem entre matrícula e mensalidade (`CodigoEstudante` só existe no fluxo de mensalidade;
  `CodigoSolicitacao` só existe no fluxo de matrícula; `Mensalidades` só existe no fluxo de mensalidade) como
  campos simplesmente deixados no valor zero pelo chamador que não se aplica — o mesmo padrão que
  `ChargeRequest`/`QRCodeRequest` já usam com `omitempty`.
- **Retorno sempre `QRCodeResult`**: preserva a decisão de design já existente no código (documentada nos
  comentários de `MatriculaPagamentoView.Charge` e `MensalidadePagamentoView.Charge`) de que o campo de
  cobrança na resposta ao pagador é sempre `QRCodeResult`, mesmo para `REF`/`GPO` (com `QRCodeArr` vazio), para
  o front-end nunca precisar tratar dois formatos de resposta diferentes.
- **`merchantID()` chamado uma única vez por chamada**, antes de montar `gerarCobrancaInput`. Em
  `mensalidade.go` já era assim (`merchant := merchantID()`, reaproveitado nos dois ramos, que são mutuamente
  exclusivos). Em `matricula.go`, o código antigo chamava `merchantID()` duas vezes — uma vez inline em cada
  ramo — mas como os ramos são mutuamente exclusivos, isso já produzia exatamente um ID por chamada; a nova
  versão só torna essa equivalência explícita, sem mudar o comportamento observável.

---

## Seção 1 — Criar `internal/finance/cobranca_geracao.go`

Crie este arquivo com o seguinte conteúdo, exatamente como está (copie e cole; não digite à mão):

```go
package finance

import (
	"context"
	"strings"
)

// gerarCobrancaInput agrupa os parâmetros necessários para emitir uma nova
// cobrança em nome de uma academia (contexto sempre ContextoAcademia), a
// partir de uma obrigação já validada por quem chama (mensalidade ou taxa
// de matrícula). CodigoEstudante, CodigoSolicitacao e Mensalidades são
// metadados de auditoria opcionais: preencha apenas o(s) que fizer(em)
// sentido para a origem da cobrança (CodigoEstudante para mensalidade,
// CodigoSolicitacao para matrícula) e deixe os demais no valor zero —
// ChargeRequest e QRCodeRequest já tratam esses campos como omitempty e
// eles nunca são enviados à AppyPay (ver comentário de ChargeRequest em
// appypay.go).
type gerarCobrancaInput struct {
	CodigoAcademia        string
	MetodoPagamento       string // "REF", "GPO" ou "GPO_QR" — já normalizado (TrimSpace+ToUpper) pelo chamador
	Amount                float64
	Description           string
	MerchantTransactionID string
	Telefone              string // usado apenas quando MetodoPagamento == "GPO"; ignorado nos demais casos
	CodigoEstudante       string
	CodigoSolicitacao     string
	Mensalidades          []MensalidadeSelecaoMes
}

// gerarCobranca é a única função do módulo financeiro que decide, a partir
// de um método de pagamento aceite pelo sistema (REF, GPO ou GPO_QR), qual
// das duas funções que efetivamente falam com a AppyPay chamar —
// CreateGPOQRCode (GPO_QR) ou CreateCharge (REF e GPO) — e monta o
// paymentInfo.phoneNumber exigido pela AppyPay quando o método é GPO.
//
// É reutilizada tanto por IniciarPagamentoMatricula (matricula.go) quanto
// por IniciarPagamentoMensalidades (mensalidade.go): as duas únicas
// funções do sistema que iniciam uma cobrança nova a partir de uma
// obrigação (matrícula ou mensalidade) e de um método de pagamento
// escolhido pelo pagador. Nenhum outro lugar do módulo deve reimplementar
// esta decisão — ver o comentário de pacote no topo de appypay.go
// ("Package finance is the only package allowed to call AppyPay's HTTP
// API").
//
// Endpoints administrativos que criam uma cobrança "avulsa" com controlo
// total sobre o payload (CriarCobrancaAppyPay, GerarQRCodeAppyPay em
// internal/handlers/financeiro_handlers.go) continuam, deliberadamente,
// chamando CreateCharge/CreateGPOQRCode diretamente: não partem de uma
// obrigação nem de um método simples REF/GPO/GPO_QR escolhido pelo
// pagador, então esta função não se aplica a eles.
//
// O retorno é sempre QRCodeResult (mesmo para REF/GPO, com QRCodeArr
// vazio), para que o chamador nunca precise tratar dois tipos de retorno
// diferentes — o mesmo motivo pelo qual MatriculaPagamentoView.Charge e
// MensalidadePagamentoView.Charge já são declarados como QRCodeResult.
func (s *Service) gerarCobranca(ctx context.Context, in gerarCobrancaInput, actorID, actorType, ip string) (QRCodeResult, error) {
	if in.MetodoPagamento == "GPO_QR" {
		qr, err := s.CreateGPOQRCode(ctx, QRCodeRequest{
			ContextoTipo:          ContextoAcademia,
			CodigoAcademia:        in.CodigoAcademia,
			CodigoEstudante:       in.CodigoEstudante,
			CodigoSolicitacao:     in.CodigoSolicitacao,
			Amount:                in.Amount,
			Currency:              "AOA",
			Description:           in.Description,
			MerchantTransactionID: in.MerchantTransactionID,
			Mensalidades:          in.Mensalidades,
		}, actorID, actorType, ip)
		if err != nil {
			return QRCodeResult{}, err
		}
		return qr, nil
	}
	info := map[string]any{}
	if in.MetodoPagamento == "GPO" {
		info["phoneNumber"] = strings.TrimSpace(in.Telefone)
	}
	charge, err := s.CreateCharge(ctx, ChargeRequest{
		ContextoTipo:          ContextoAcademia,
		CodigoAcademia:        in.CodigoAcademia,
		CodigoEstudante:       in.CodigoEstudante,
		CodigoSolicitacao:     in.CodigoSolicitacao,
		Mensalidades:          in.Mensalidades,
		Amount:                in.Amount,
		Currency:              "AOA",
		Description:           in.Description,
		MerchantTransactionID: in.MerchantTransactionID,
		PaymentMethod:         in.MetodoPagamento,
		PaymentInfo:           info,
	}, actorID, actorType, ip)
	if err != nil {
		return QRCodeResult{}, err
	}
	return QRCodeResult{ChargeResult: charge}, nil
}
```

---

## Seção 2 — Criar `internal/finance/cobranca_geracao_integration_test.go`

Crie este arquivo com o seguinte conteúdo, exatamente como está (copie e cole; não digite à mão). Este teste
é novo (não existia antes desta tarefa) e fixa, com asserções concretas sobre o corpo da requisição enviada à
AppyPay (não apenas "não deu erro"), que:

1. O método `GPO` continua enviando `paymentInfo.phoneNumber` já sem espaços, tanto para mensalidade quanto
   para matrícula.
2. O método `REF` nunca envia `paymentInfo.phoneNumber`, nos dois fluxos.

```go
package finance

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// capturingAppyPayTransport é um mock isolado (não compartilhado com
// appyPayMockTransport de appypay_integration_test.go) que grava o corpo
// de cada requisição POST /charges ou /qr-codes enviada à AppyPay, para
// permitir inspecionar exatamente o payload que gerarCobranca monta —
// não apenas se a chamada teve sucesso.
type capturingAppyPayTransport struct {
	mu        sync.Mutex
	lastBody  map[string]any
	lastPath  string
	qrCodeArr string
}

func (t *capturingAppyPayTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.Contains(req.URL.Path, "/oauth2/token") {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"test-token","expires_in":3600}`)), Request: req}, nil
	}
	if req.Method == http.MethodPost {
		raw, _ := io.ReadAll(req.Body)
		var parsed map[string]any
		_ = json.Unmarshal(raw, &parsed)
		t.mu.Lock()
		t.lastBody = parsed
		t.lastPath = req.URL.Path
		t.mu.Unlock()

		if strings.HasSuffix(req.URL.Path, "/qr-codes") {
			body := `{"id":"provider-qr-` + uuid.NewString() + `","status":"Pending","qrCodeArr":"` + t.qrCodeArr + `"}`
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
		}
		body := `{"id":"provider-charge-` + uuid.NewString() + `","status":"Pending"}`
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	}
	// GET (consulta) não é usado por estes testes, mas devolve algo
	// coerente para não quebrar caso algum caminho inesperado o chame.
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"provider","status":"Pending"}`)), Request: req}, nil
}

func (t *capturingAppyPayTransport) paymentInfo() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	pi, _ := t.lastBody["paymentInfo"].(map[string]any)
	return pi
}

// TestIntegrationGerarCobrancaMensalidadeGPOEnviaPhoneNumberNormalizado
// prova que, após a extração de gerarCobranca (internal/finance/cobranca_geracao.go),
// o fluxo de mensalidade continua enviando paymentInfo.phoneNumber já sem
// espaços à AppyPay quando o método é GPO — exatamente como antes da
// refatoração.
func TestIntegrationGerarCobrancaMensalidadeGPOEnviaPhoneNumberNormalizado(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "EST-GPO-" + uuid.NewString()[:8]
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-GPO", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO}' WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}
	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	transport := &capturingAppyPayTransport{}
	service.SetHTTPClient(&http.Client{Transport: transport})

	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendentes) == 0 {
		t.Fatal("esperava pelo menos uma mensalidade pendente")
	}
	alvo := pendentes[0]

	_, err = service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia,
		Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
		MetodoPagamento: "GPO", Telefone: "  923000000  ",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
	}
	if transport.lastPath == "" || !strings.HasSuffix(transport.lastPath, "/charges") {
		t.Fatalf("esperava POST .../charges, obteve path=%q", transport.lastPath)
	}
	pi := transport.paymentInfo()
	if pi == nil || pi["phoneNumber"] != "923000000" {
		t.Fatalf("esperava paymentInfo.phoneNumber=923000000 (sem espaços), obteve %#v", pi)
	}
}

// TestIntegrationGerarCobrancaMatriculaGPOEnviaPhoneNumberNormalizado é o
// equivalente do teste acima para o fluxo de matrícula — prova que os dois
// únicos chamadores de gerarCobranca continuam se comportando de forma
// idêntica para o mesmo método de pagamento.
func TestIntegrationGerarCobrancaMatriculaGPOEnviaPhoneNumberNormalizado(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	codigo := seedMatriculaPendente(t, client, academia, 750)
	if _, err := client.DB().Exec(`UPDATE projection_solicitacoes_matricula SET metodos_pagamento_matricula='{GPO}' WHERE codigo_solicitacao=$1`, codigo); err != nil {
		t.Fatal(err)
	}
	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	transport := &capturingAppyPayTransport{}
	service.SetHTTPClient(&http.Client{Transport: transport})

	_, err := service.IniciarPagamentoMatricula(ctx, MatriculaPagamentoInput{
		CodigoSolicitacao: codigo, MetodoPagamento: "GPO", Telefone: "  912345678  ",
	}, "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMatricula falhou: %v", err)
	}
	if transport.lastPath == "" || !strings.HasSuffix(transport.lastPath, "/charges") {
		t.Fatalf("esperava POST .../charges, obteve path=%q", transport.lastPath)
	}
	pi := transport.paymentInfo()
	if pi == nil || pi["phoneNumber"] != "912345678" {
		t.Fatalf("esperava paymentInfo.phoneNumber=912345678 (sem espaços), obteve %#v", pi)
	}
}

// TestIntegrationGerarCobrancaREFNaoEnviaPhoneNumber prova que o método REF
// (matrícula e mensalidade) nunca envia paymentInfo.phoneNumber — o campo
// só existe para GPO. Cobre os dois chamadores de gerarCobranca no mesmo
// teste para deixar explícito que é o mesmo comportamento nos dois fluxos.
func TestIntegrationGerarCobrancaREFNaoEnviaPhoneNumber(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	t.Run("mensalidade", func(t *testing.T) {
		academia := mensalidadeCodigo()
		estudante := "EST-REF-" + uuid.NewString()[:8]
		seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
		seedMensalidadeTurma(t, client, academia, "T-REF", "2025_2026", estudante, nil)
		seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
		if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{REF}' WHERE codigo_academia=$1`, academia); err != nil {
			t.Fatal(err)
		}
		configureIntegrationCredential(t, service, ContextoAcademia, academia)
		transport := &capturingAppyPayTransport{}
		service.SetHTTPClient(&http.Client{Transport: transport})

		pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
		if err != nil {
			t.Fatal(err)
		}
		if len(pendentes) == 0 {
			t.Fatal("esperava pelo menos uma mensalidade pendente")
		}
		alvo := pendentes[0]

		_, err = service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
			CodigoEstudante: estudante, CodigoAcademia: academia,
			Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
			MetodoPagamento: "REF",
		}, estudante, "estudante", "127.0.0.1")
		if err != nil {
			t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
		}
		pi := transport.paymentInfo()
		if _, ok := pi["phoneNumber"]; ok {
			t.Fatalf("REF não deveria enviar phoneNumber, obteve paymentInfo=%#v", pi)
		}
	})

	t.Run("matricula", func(t *testing.T) {
		academia := mensalidadeCodigo()
		codigo := seedMatriculaPendente(t, client, academia, 750)
		configureIntegrationCredential(t, service, ContextoAcademia, academia)
		transport := &capturingAppyPayTransport{}
		service.SetHTTPClient(&http.Client{Transport: transport})

		_, err := service.IniciarPagamentoMatricula(ctx, MatriculaPagamentoInput{
			CodigoSolicitacao: codigo, MetodoPagamento: "REF",
		}, "127.0.0.1")
		if err != nil {
			t.Fatalf("IniciarPagamentoMatricula falhou: %v", err)
		}
		pi := transport.paymentInfo()
		if _, ok := pi["phoneNumber"]; ok {
			t.Fatalf("REF não deveria enviar phoneNumber, obteve paymentInfo=%#v", pi)
		}
	})
}
```

---

## Seção 3 — Alterar `internal/finance/matricula.go`

**Localizar** (bloco único e exato dentro de `IniciarPagamentoMatricula`):

```go
	desc := "Taxa de matrícula " + academia
	if in.MetodoPagamento == "GPO_QR" {
		qr, err := s.CreateGPOQRCode(ctx, QRCodeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: academia, CodigoSolicitacao: in.CodigoSolicitacao, Amount: valor.Float64, Currency: "AOA", Description: desc, MerchantTransactionID: merchantID()}, "solicitacao:"+in.CodigoSolicitacao, "solicitante", ip)
		if err != nil {
			return MatriculaPagamentoView{}, err
		}
		return MatriculaPagamentoView{Charge: qr}, nil
	}
	info := map[string]any{}
	if in.MetodoPagamento == "GPO" {
		info["phoneNumber"] = strings.TrimSpace(in.Telefone)
	}
	charge, err := s.CreateCharge(ctx, ChargeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: academia, CodigoSolicitacao: in.CodigoSolicitacao, Amount: valor.Float64, Currency: "AOA", Description: desc, MerchantTransactionID: merchantID(), PaymentMethod: in.MetodoPagamento, PaymentInfo: info}, "solicitacao:"+in.CodigoSolicitacao, "solicitante", ip)
	if err != nil {
		return MatriculaPagamentoView{}, err
	}
	return MatriculaPagamentoView{Charge: QRCodeResult{ChargeResult: charge}}, nil
}
```

**Substituir por:**

```go
	desc := "Taxa de matrícula " + academia
	result, err := s.gerarCobranca(ctx, gerarCobrancaInput{
		CodigoAcademia:        academia,
		MetodoPagamento:       in.MetodoPagamento,
		Amount:                valor.Float64,
		Description:           desc,
		MerchantTransactionID: merchantID(),
		Telefone:              in.Telefone,
		CodigoSolicitacao:     in.CodigoSolicitacao,
	}, "solicitacao:"+in.CodigoSolicitacao, "solicitante", ip)
	if err != nil {
		return MatriculaPagamentoView{}, err
	}
	return MatriculaPagamentoView{Charge: result}, nil
}
```

**Atenção ao import `strings` em `matricula.go`**: não remova o import `"strings"` do topo do arquivo. Ele
continua sendo usado por outras funções do mesmo arquivo (confirmado por grep antes desta tarefa) — a única
chamada `strings.TrimSpace` removida por este diff é a que estava dentro do bloco substituído acima, e ela
passa a viver dentro de `gerarCobranca` (`cobranca_geracao.go`), que tem seu próprio import de `"strings"`.

---

## Seção 4 — Alterar `internal/finance/mensalidade.go`

**Localizar** (bloco único e exato dentro de `IniciarPagamentoMensalidades`):

```go
	total = roundAmount(total)
	description, merchant := fmt.Sprintf("Propinas %s: %d mensalidade(s)", in.CodigoAcademia, len(in.Meses)), merchantID()
	if in.MetodoPagamento == "GPO_QR" {
		qr, err := s.CreateGPOQRCode(ctx, QRCodeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: in.CodigoAcademia, CodigoEstudante: in.CodigoEstudante, Amount: total, Currency: "AOA", Description: description, MerchantTransactionID: merchant, Mensalidades: in.Meses}, actorID, actorType, ip)
		if err != nil {
			return MensalidadePagamentoView{}, err
		}
		return MensalidadePagamentoView{Charge: qr, Meses: in.Meses}, nil
	}
	info := map[string]any{}
	if in.MetodoPagamento == "GPO" {
		info["phoneNumber"] = strings.TrimSpace(in.Telefone)
	}
	charge, err := s.CreateCharge(ctx, ChargeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: in.CodigoAcademia, CodigoEstudante: in.CodigoEstudante, Mensalidades: in.Meses, Amount: total, Currency: "AOA", Description: description, MerchantTransactionID: merchant, PaymentMethod: in.MetodoPagamento, PaymentInfo: info}, actorID, actorType, ip)
	if err != nil {
		return MensalidadePagamentoView{}, err
	}
	return MensalidadePagamentoView{Charge: QRCodeResult{ChargeResult: charge}, Meses: in.Meses}, nil
}
```

**Substituir por:**

```go
	total = roundAmount(total)
	description := fmt.Sprintf("Propinas %s: %d mensalidade(s)", in.CodigoAcademia, len(in.Meses))
	result, err := s.gerarCobranca(ctx, gerarCobrancaInput{
		CodigoAcademia:        in.CodigoAcademia,
		MetodoPagamento:       in.MetodoPagamento,
		Amount:                total,
		Description:           description,
		MerchantTransactionID: merchantID(),
		Telefone:              in.Telefone,
		CodigoEstudante:       in.CodigoEstudante,
		Mensalidades:          in.Meses,
	}, actorID, actorType, ip)
	if err != nil {
		return MensalidadePagamentoView{}, err
	}
	return MensalidadePagamentoView{Charge: result, Meses: in.Meses}, nil
}
```

**Atenção ao import `strings` em `mensalidade.go`**: mesma observação da Seção 3 — não remova o import,
continua em uso por outras funções do arquivo.

---

## Fora de escopo

- Qualquer mudança em `internal/finance/appypay.go`.
- Qualquer mudança em `internal/handlers/financeiro_handlers.go` (inclui os endpoints administrativos
  `CriarCobrancaAppyPay` e `GerarQRCodeAppyPay`, que continuam chamando `CreateCharge`/`CreateGPOQRCode`
  diretamente, de propósito).
- Qualquer mudança em `go.mod` ou `go.sum`.
- Qualquer mudança em testes pré-existentes.
- Suporte a um quarto método de pagamento, ou qualquer mudança na lista de métodos aceites (`REF`, `GPO`,
  `GPO_QR`) — isso é assunto de outra tarefa, se algum dia for necessário.
- Extrair também a lógica de `CriarCobrancaAppyPay`/`GerarQRCodeAppyPay` para usar `gerarCobranca` — não se
  aplica a eles (ver "Contexto").

---

## Nota sobre validação — o que já foi feito vs. o que você deve rodar

O ambiente do Codex bloqueia `apt` (403 Forbidden) e não tem Docker nem `psql`, então você **não consegue**
rodar os testes de integração deste módulo, que exigem PostgreSQL real (`RUN_POSTGRES_INTEGRATION=1`). Isso é
esperado e **não é um problema**: o orquestrador (ambiente com acesso a `apt`/PostgreSQL real) já fez essa
parte por você, com o diff exatamente como está especificado nas Seções 1 a 4:

- `go build ./...`, `go vet ./...` e `gofmt -l .` — limpos, sem nenhum erro ou aviso.
- `go test ./...` do repositório inteiro — todos os pacotes `ok`, nenhuma regressão.
- Suíte de `internal/finance` sozinha — executada **5 vezes seguidas em banco PostgreSQL limpo** (dropado e
  recriado antes de cada execução, migrations reaplicadas do zero pelo próprio runner de migrations da
  aplicação) — 5/5 `ok`.
- Suíte de `internal/handlers` sozinha — executada **3 vezes seguidas em banco limpo** — 3/3 `ok`.
- `go test -race ./internal/finance/...` — `ok`, sem nenhuma condição de corrida detectada.
- Os 3 testes novos desta tarefa (`TestIntegrationGerarCobrancaMensalidadeGPOEnviaPhoneNumberNormalizado`,
  `TestIntegrationGerarCobrancaMatriculaGPOEnviaPhoneNumberNormalizado`,
  `TestIntegrationGerarCobrancaREFNaoEnviaPhoneNumber`) foram executados isoladamente e passaram, além de
  passarem dentro das 5 execuções completas da suíte acima.
- Verificação estrutural por grep (prova de que a duplicação foi eliminada): depois da correção,
  `CreateCharge(` só aparece em `appypay.go` (definição), `cobranca_geracao.go` (única chamada de dentro de
  `gerarCobranca`) e `internal/handlers/financeiro_handlers.go` (endpoint administrativo, intencional).
  `CreateGPOQRCode(` segue o mesmo padrão. `gerarCobranca(` só é chamada de `matricula.go` e `mensalidade.go`
  — exatamente os dois lugares que motivaram esta tarefa.

**O que você (Codex) deve rodar**, a partir da raiz do repositório, depois de aplicar as Seções 1 a 4:

1. `go build ./...` — deve terminar sem nenhuma saída (sem erros).
2. `go vet ./...` — deve terminar sem nenhuma saída.
3. `gofmt -l .` — deve terminar sem nenhuma saída (nenhum arquivo precisa de reformatação). Se algum arquivo
   aparecer listado, rode `gofmt -w <arquivo>` nele antes de prosseguir.
4. `go test ./...` — todos os pacotes devem aparecer como `ok`. Sem `RUN_POSTGRES_INTEGRATION=1` definido, os
   testes de integração (incluindo os 3 novos desta tarefa) serão pulados automaticamente
   (`t.Skip("teste de integração requer RUN_POSTGRES_INTEGRATION=1 e PostgreSQL")`) — isso é o comportamento
   esperado e correto neste ambiente, não uma falha.
5. Rode a verificação estrutural por grep abaixo e confirme que a saída bate com o esperado:
   ```
   grep -rn "CreateCharge(" --include="*.go" . | grep -v "_test.go"
   grep -rn "CreateGPOQRCode(" --include="*.go" . | grep -v "_test.go"
   grep -rn "s\.gerarCobranca(" --include="*.go" . | grep -v "_test.go"
   ```
   Esperado: as duas primeiras buscas devem devolver exatamente 3 linhas cada (a definição em `appypay.go`, a
   chamada em `cobranca_geracao.go`, e a chamada em `financeiro_handlers.go`); a terceira deve devolver
   exatamente 2 linhas (`matricula.go` e `mensalidade.go`).

**Plano B** (se, por qualquer motivo, você tiver acesso a PostgreSQL neste ambiente — não esperado, mas caso
aconteça): pode rodar a suíte completa de integração para dupla confirmação, com:
```
export RUN_POSTGRES_INTEGRATION=1 DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
  DB_NAME=spuri_test DB_SSLMODE=disable APPYPAY_RESOURCE=integration-resource \
  FINANCE_ENCRYPTION_KEY=01234567890123456789012345678901 ENV=test
go test ./internal/finance/... ./internal/handlers/...
```
Isso é opcional — não é exigido para concluir esta tarefa, já que o orquestrador já validou esta parte.

---

## Checklist de aceitação

Execute nesta ordem exata. Pare e reporte no primeiro item que falhar.

1. [ ] Criar `internal/finance/cobranca_geracao.go` com o conteúdo exato da Seção 1.
2. [ ] Criar `internal/finance/cobranca_geracao_integration_test.go` com o conteúdo exato da Seção 2.
3. [ ] Aplicar o diff da Seção 3 em `internal/finance/matricula.go`.
4. [ ] Aplicar o diff da Seção 4 em `internal/finance/mensalidade.go`.
5. [ ] `go build ./...` — sem saída.
6. [ ] `go vet ./...` — sem saída.
7. [ ] `gofmt -l .` — sem saída.
8. [ ] `go test ./...` — todos os pacotes `ok` (integração pulada, ver nota acima).
9. [ ] Rodar as 3 buscas por grep da seção anterior e confirmar as contagens esperadas (3, 3, 2).
10. [ ] `git diff --stat` — confirmar que apenas estes 4 arquivos aparecem:
    `internal/finance/cobranca_geracao.go` (novo),
    `internal/finance/cobranca_geracao_integration_test.go` (novo),
    `internal/finance/matricula.go` (modificado),
    `internal/finance/mensalidade.go` (modificado). Nenhuma mudança em `go.mod`, `go.sum`, ou qualquer outro
    arquivo.
11. [ ] Commit com mensagem: `fix(finance): unificar geração de cobrança REF/GPO/GPO_QR num único ponto (gerarCobranca)`.

## Procedimento de conclusão

Depois que todos os itens do checklist passarem: mover este arquivo de
`docs/Lista de Tarefas/67 - Unificar geração de cobrança REF-GPO-GPOQR num único ponto para matrícula e mensalidade.md`
para `docs/Tarefas feitas/67 - Unificar geração de cobrança REF-GPO-GPOQR num único ponto para matrícula e mensalidade.md`,
atualizando o campo `status` no frontmatter de `pendente` para `feito`.
