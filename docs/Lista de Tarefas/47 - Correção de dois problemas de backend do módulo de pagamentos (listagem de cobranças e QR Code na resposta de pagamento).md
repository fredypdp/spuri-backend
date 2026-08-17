---
criado: 2026-08-16
origem: "docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md" (levantamento feito durante a preparação da tarefa de frontend do módulo de pagamentos). Esta tarefa foi orquestrada por Claude (Anthropic) a partir da leitura completa do código-fonte relevante de spuri-backend (branch main): cmd/server/main.go, internal/handlers/financeiro_handlers.go, internal/handlers/solicitacao_matricula_handlers.go, internal/finance/appypay.go, internal/finance/mensalidade.go, internal/finance/matricula.go, internal/projections/financeiro_projection.go, internal/domain/aggregates/financeiro.go, internal/domain/models.go, migrations/097 a 108, e os arquivos de teste existentes do módulo financeiro.
status: pendente
depende_de: nenhuma
---

# Correção de dois problemas de backend do módulo de pagamentos (listagem de cobranças e QR Code na resposta de pagamento)

## Prompt recomendado para executar a tarefa

```
Leia por completo o arquivo "docs/Lista de Tarefas/46 - Correção de dois problemas de backend do módulo de
pagamentos (listagem de cobranças e QR Code na resposta de pagamento).md". Ele contém 2 correções já
totalmente especificadas (diff exato, arquivo e trecho a substituir, com justificativa) e 4 arquivos de teste
novos, completos, prontos para criar. Todas as decisões de design (nomes de rota, nomes de campos, formato de
resposta, onde colocar cada função) já foram tomadas e estão na seção "Decisões de design já tomadas" — não é
necessário planejar, investigar causa raiz ou decidir abordagem nenhuma. Aplique as correções exatamente como
especificado em cada "Localizar" / "Substituir por", crie os 4 arquivos de teste exatamente como
especificados, e então execute a seção "Checklist de aceitação" ao final do documento, na ordem, sem pular
nenhum passo. Este documento foi produzido por leitura cuidadosa do código-fonte real (todo trecho citado foi
lido diretamente do repositório), mas as correções NÃO foram compiladas nem testadas antes de escrever este
documento (ambiente de orquestração sem acesso ao proxy de módulos Go). Ou seja, a execução do checklist de
aceitação abaixo é a primeira compilação/execução real deste diff. Se qualquer comando do checklist falhar,
pare e reporte o erro exato — não prossiga para o próximo item nem invente uma correção diferente da
especificada aqui sem antes reportar.
```

## Contexto

O documento `docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md` identificou dois problemas
reais de backend, confirmados por leitura direta do código-fonte, que bloqueavam partes da tarefa de frontend
do módulo de pagamentos:

1. **Problema 1** — não existe nenhuma rota nem consulta capaz de listar cobranças/pagamentos de uma
   academia (ou de todas, para o admin). Só é possível consultar uma cobrança pontual já sabendo o
   identificador dela, ou mensalidades de um estudante já sabendo o código dele.
2. **Problema 2** — quando o método de pagamento escolhido é `GPO_QR`, o backend gera o QR Code
   corretamente, mas o campo `qrCodeArr` (o conteúdo do QR Code) é descartado antes de chegar na resposta de
   `POST /financeiro/mensalidades/pagamento` e de `POST /solicitacoes-matricula/:codigo/pagamento-matricula`
   — as duas únicas respostas que o pagador final (estudante ou candidato) efetivamente recebe.

Este documento contém a correção completa dos dois problemas: código exato a alterar, testes de regressão
novos e checklist de validação. Nenhuma migration nova é necessária — os dois problemas são resolvidos
inteiramente na camada de aplicação (`internal/finance`, `internal/handlers`, `cmd/server/main.go`), usando
tabelas e colunas que já existem hoje (`financeiro_cobrancas`, populada desde a migration 101).

---

## Escopo desta tarefa (e o que NÃO fazer)

**Arquivos a modificar:**
- `internal/finance/appypay.go`
- `internal/finance/mensalidade.go`
- `internal/finance/matricula.go`
- `internal/handlers/financeiro_handlers.go`
- `cmd/server/main.go`

**Arquivos novos a criar:**
- `internal/finance/qrcode_pagamento_view_test.go`
- `internal/finance/qrcode_regression_integration_test.go`
- `internal/finance/cobrancas_integration_test.go`
- `internal/handlers/financeiro_cobrancas_handlers_test.go`

**Fora do escopo — não alterar:**
- Nenhuma migration (`migrations/`). A tabela `financeiro_cobrancas` já tem todas as colunas necessárias.
- Nenhum aggregate (`internal/domain/aggregates/`). Os dois problemas são de leitura/resposta, não de escrita
  de eventos — nenhum evento novo é registrado no ledger.
- `internal/domain/models.go` — não tem relação com o módulo financeiro.
- `internal/projections/financeiro_projection.go` — apesar de o documento de origem sugerir colocar a nova
  consulta ali, esta tarefa decide colocá-la em `internal/finance/appypay.go` em vez disso. O motivo está
  documentado na seção "Decisões de design já tomadas" abaixo — não é uma omissão, é uma decisão deliberada.
- Qualquer outro módulo (estudantes, turmas, avaliação final, etc.) — os dois problemas são inteiramente
  contidos no módulo financeiro.
- `go.mod` / `go.sum` — `github.com/lib/pq` já é dependência direta (`go.mod` linha 14); nenhuma dependência
  nova é necessária.

---

## Decisões de design já tomadas

Estas decisões resolvem todos os pontos que os dois problemas originais deixaram em aberto ("a decidir por
quem for corrigir"). Não é necessário reabri-las.

1. **Problema 2 — abordagem 1 (trocar o tipo do campo), não a abordagem 2.** O documento de origem propunha
   duas abordagens. Esta tarefa usa a abordagem 1: trocar o tipo de `Charge` em `MensalidadePagamentoView` e
   `MatriculaPagamentoView` de `ChargeResult` para `QRCodeResult` — é a correção mais simples, mais local, e
   resolve o problema na origem (o campo `QRCodeArr` já existe e já é preenchido corretamente por
   `CreateGPOQRCode`; só não sobrevive à atribuição `Charge: qr.ChargeResult`, que descarta a struct externa
   e fica só com a interna). `QRCodeResult` embute `ChargeResult`, então todo código existente que acessa
   `out.Charge.Status`, `out.Charge.ID`, `out.Charge.ProviderChargeID` etc. continua compilando sem alteração
   (campo promovido pelo embedding do Go) — confirmado lendo os três pontos do repositório que acessam esses
   campos (`internal/handlers/solicitacao_matricula_handlers.go` linha 458, e os testes de integração já
   existentes).

2. **Problema 1 — a nova consulta fica em `internal/finance/appypay.go`, não em
   `internal/projections/financeiro_projection.go`.** O documento de origem sugeria colocar a consulta na
   projeção. Lendo o código, isso contraria a convenção já estabelecida pelo próprio módulo financeiro:
   `FinanceiroProjection` tem o comentário de documentação `"FinanceiroProjection is the sole writer of
   financial read models"` (arquivo `internal/projections/financeiro_projection.go`, linha 49) e, de fato,
   **só** tem métodos de escrita (`Handle`, `Rebuild`, `ApplyNow`, etc.) — nenhuma leitura. Toda leitura do
   módulo financeiro já vive em `internal/finance/*.go` como método de `Service`, consultando o banco
   diretamente via `s.client.DB()` — é exatamente assim que `ListCredentials` (mesmo arquivo, já existente)
   funciona hoje para a tabela `financeiro_credenciais_appypay`. A nova função `ListCobrancas` segue esse
   mesmo padrão já estabelecido, para a tabela `financeiro_cobrancas`.

3. **Rota nova: `GET /financeiro/cobrancas`** (não `/financeiro/appypay/cobrancas`, que colidiria
   conceitualmente com o grupo específico de rotas AppyPay de baixo nível). Fica dentro do grupo `financeiro`
   já protegido por `middleware.RequireAcademiaOuAdmin()`, ao lado de `/mensalidades/*` e `/matriculas/*` —
   é exatamente o path sugerido no documento de origem.

4. **Uma única rota unificada, com campo `origem` indicando a proveniência**, em vez de duas rotas separadas
   (mensalidade vs. matrícula) — o documento de origem deixou as duas opções em aberto, dizendo que o
   frontend se adapta a qualquer uma. Uma rota unificada é mais simples de implementar e manter, e o
   documento de frontend original queria justamente "todos os pagamentos, em todos os estados" num único
   lugar. `origem` é `"matricula"` (cobrança com `codigo_solicitacao`), `"mensalidade"` (cobrança com
   `codigo_estudante` e sem `codigo_solicitacao`) ou `"avulsa"` (cobrança sem nenhum dos dois — criada
   diretamente via `POST /financeiro/appypay/cobrancas` ou `/appypay/qr-codes` sem vínculo, ex.: um cobrança
   manual da academia que não é nem mensalidade nem matrícula).

5. **A listagem devolve um resumo, não o detalhe completo.** O novo endpoint não inclui `payment_info`,
   `response` (resposta crua da AppyPay) nem `qrCodeArr` — só o suficiente para montar uma tabela/lista
   navegável (id, status, valor, moeda, descrição, método, origem, vínculo). O detalhe completo de uma
   cobrança específica continua vindo de `GET /financeiro/appypay/cobrancas/:id`, exatamente como a tarefa de
   frontend original já previa ("permitindo a abertura de subtelas para ver com mais detalhes cada
   cobrança/pagamento"). Isso também evita responses desnecessariamente grandes numa lista paginada.

6. **Nenhuma migration nova.** A listagem usa apenas a tabela `financeiro_cobrancas` já existente (colunas
   `id`, `provider_charge_id`, `merchant_transaction_id`, `contexto_tipo`, `codigo_academia`, `payload`
   JSONB, `updated_at`) e filtra/extrai dados do `payload` via operadores JSONB do Postgres (`->>`). Não há
   coluna `created_at` separada nesta tabela (só `updated_at`, atualizada a cada mudança de status) —
   a ordenação da listagem é por `updated_at DESC` (cobranças com atividade mais recente primeiro). Isto é
   suficiente para o caso de uso e evita alterar o schema.

7. **Filtro de estado (`estado` na query string) é um match exato, case-sensitive, sobre o texto cru
   persistido em `payload->>'status'`.** Esse campo já mistura, hoje, estados internos do Spuri
   (`"solicitada"`, `"criada"`, `"cancelada"`, `"falhada"`) com estados crus devolvidos pela AppyPay
   (`"Success"`, `"Pending"`, `"Failed"`, etc. — a capitalização exata depende do que a AppyPay devolve) —
   é o mesmo texto que já aparece hoje no campo `status` de `GET /financeiro/appypay/cobrancas/:id`. Esta
   tarefa **não** normaliza esse campo (nem em maiúsculas/minúsculas, nem para um enum fixo), pelo mesmo
   motivo que o próprio código já trata isso caso a caso (`isSuccessfulChargeStatus`,
   `isTerminalChargeStatus`, ambas com comparação case-insensitive ad-hoc, sem nunca alterar o dado
   persistido). Normalizar о campo estaria fora do escopo desta correção e poderia mascarar o texto real
   devolvido pelo provedor. Quem consumir esta rota deve tratar os valores de estado exatamente como já trata
   os valores de status de `GET /financeiro/appypay/cobrancas/:id` hoje.

8. **Nomes de query string:** `contexto_tipo`, `codigo_academia` (mesmos nomes já usados em
   `ListarCredenciaisAppyPay`/`ConsultarCobrancaAppyPay`), `estado` (repetível, nome sugerido pelo próprio
   documento de origem), `tipo` (repetível, valores `matricula`/`mensalidade`/`avulsa`), `limit`, `offset`
   (mesmos nomes e mesmos limites — default 50, mínimo 1, máximo 1000 para `limit`; default 0, mínimo 0,
   máximo 1_000_000 para `offset` — já usados em `listSolicitacoes`, via a função `parseBoundedInt` já
   existente em `internal/handlers/solicitacao_matricula_handlers.go`, reaproveitada sem alteração).

9. **Formato da resposta:** `{"cobrancas": [...], "total": N, "total_geral": M, "limit": L, "offset": O}` —
   mesmo formato já usado por `listSolicitacoes` (`GET /solicitacoes-matricula`), com `total` = itens nesta
   página e `total_geral` = total real que casa com os filtros.

10. **Autorização:** exatamente `authorizeFinanceScope`, a mesma função já usada por
    `ConsultarCobrancaAppyPay` e `ListarCredenciaisAppyPay`. Uma academia só vê as próprias cobranças
    (contexto e academia são sobrescritos para os dela, ignorando o que vier na query string); um admin
    precisa da permissão `"fpp"` e pode consultar qualquer `contexto_tipo`/`codigo_academia` via query string
    (inclusive vazio, o que lista tudo, sem filtro de academia — mesmo comportamento que
    `ListarCredenciaisAppyPay` já tem hoje).

---

## Tarefa A — Problema 2: QR Code não chega à resposta de pagamento

### A.1 — `internal/finance/mensalidade.go`

**Causa raiz confirmada:** `MensalidadePagamentoView.Charge` é declarado como `ChargeResult` (sem o campo
`QRCodeArr`). No branch de `GPO_QR`, o código já tem o QR Code em mãos (`qr`, do tipo `QRCodeResult`, com
`QRCodeArr` preenchido pela AppyPay), mas descarta a struct externa ao escrever `Charge: qr.ChargeResult` —
isso acessa só a sub-struct embutida, e `QRCodeArr` nunca chega a existir dentro de `Charge`.

**Localizar:**

```go
type MensalidadePagamentoView struct {
	Charge ChargeResult            `json:"cobranca"`
	Meses  []MensalidadeSelecaoMes `json:"meses"`
}
```

**Substituir por:**

```go
type MensalidadePagamentoView struct {
	// Charge é QRCodeResult (não ChargeResult) para que o campo QRCodeArr (o
	// conteúdo do QR Code) chegue nesta resposta quando
	// metodo_pagamento = "GPO_QR". Para qualquer outro método, QRCodeArr
	// fica vazio e é omitido do JSON (omitempty). Ver Problema 2 em
	// docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md.
	Charge QRCodeResult            `json:"cobranca"`
	Meses  []MensalidadeSelecaoMes `json:"meses"`
}
```

**Localizar** (dentro de `IniciarPagamentoMensalidades`):

```go
	if in.MetodoPagamento == "GPO_QR" {
		qr, err := s.CreateGPOQRCode(ctx, QRCodeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: in.CodigoAcademia, CodigoEstudante: in.CodigoEstudante, Amount: total, Currency: "AOA", Description: description, MerchantTransactionID: merchant, Mensalidades: in.Meses}, actorID, actorType, ip)
		if err != nil {
			return MensalidadePagamentoView{}, err
		}
		return MensalidadePagamentoView{Charge: qr.ChargeResult, Meses: in.Meses}, nil
	}
	info := map[string]any{}
	if in.MetodoPagamento == "GPO" {
		info["phoneNumber"] = strings.TrimSpace(in.Telefone)
	}
	charge, err := s.CreateCharge(ctx, ChargeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: in.CodigoAcademia, CodigoEstudante: in.CodigoEstudante, Mensalidades: in.Meses, Amount: total, Currency: "AOA", Description: description, MerchantTransactionID: merchant, PaymentMethod: in.MetodoPagamento, PaymentInfo: info}, actorID, actorType, ip)
	if err != nil {
		return MensalidadePagamentoView{}, err
	}
	return MensalidadePagamentoView{Charge: charge, Meses: in.Meses}, nil
```

**Substituir por:**

```go
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
```

Nenhum import novo é necessário neste arquivo.

### A.2 — `internal/finance/matricula.go`

Mesma causa raiz, mesmo padrão, no fluxo de matrícula.

**Localizar:**

```go
type MatriculaPagamentoView struct {
	Charge ChargeResult `json:"cobranca"`
}
```

**Substituir por:**

```go
type MatriculaPagamentoView struct {
	// Charge é QRCodeResult pelo mesmo motivo documentado em
	// MensalidadePagamentoView (internal/finance/mensalidade.go).
	Charge QRCodeResult `json:"cobranca"`
}
```

**Localizar** (dentro de `IniciarPagamentoMatricula`):

```go
	desc := "Taxa de matrícula " + academia
	if in.MetodoPagamento == "GPO_QR" {
		qr, err := s.CreateGPOQRCode(ctx, QRCodeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: academia, CodigoSolicitacao: in.CodigoSolicitacao, Amount: valor.Float64, Currency: "AOA", Description: desc, MerchantTransactionID: merchantID()}, "solicitacao:"+in.CodigoSolicitacao, "solicitante", ip)
		if err != nil {
			return MatriculaPagamentoView{}, err
		}
		return MatriculaPagamentoView{Charge: qr.ChargeResult}, nil
	}
	info := map[string]any{}
	if in.MetodoPagamento == "GPO" {
		info["phoneNumber"] = strings.TrimSpace(in.Telefone)
	}
	charge, err := s.CreateCharge(ctx, ChargeRequest{ContextoTipo: ContextoAcademia, CodigoAcademia: academia, CodigoSolicitacao: in.CodigoSolicitacao, Amount: valor.Float64, Currency: "AOA", Description: desc, MerchantTransactionID: merchantID(), PaymentMethod: in.MetodoPagamento, PaymentInfo: info}, "solicitacao:"+in.CodigoSolicitacao, "solicitante", ip)
	if err != nil {
		return MatriculaPagamentoView{}, err
	}
	return MatriculaPagamentoView{Charge: charge}, nil
```

**Substituir por:**

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
```

Nenhum import novo é necessário neste arquivo.

### A.3 — Teste novo (unitário, sem banco): `internal/finance/qrcode_pagamento_view_test.go`

Prova a correção no nível mais direto possível: serializa as duas views para JSON e confirma que
`qrCodeArr` chega no campo `cobranca`. Não precisa de Postgres, roda em qualquer `go test ./internal/finance/...`.

Criar o arquivo com exatamente este conteúdo:

```go
package finance

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestMensalidadePagamentoViewIncludesQRCodeArr reproduz o Problema 2
// documentado em
// "docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md":
// antes da correção, Charge era ChargeResult (sem QRCodeArr), então o campo
// nunca sobrevivia à serialização JSON da resposta de pagamento de
// mensalidade, mesmo quando a AppyPay devolvia o QR Code corretamente.
func TestMensalidadePagamentoViewIncludesQRCodeArr(t *testing.T) {
	view := MensalidadePagamentoView{
		Charge: QRCodeResult{
			ChargeResult: ChargeResult{ID: uuid.New(), MerchantTransactionID: "M1", Status: "criada"},
			QRCodeArr:    "base64-qr-mensalidade",
		},
		Meses: []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 3}},
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	cobranca, ok := decoded["cobranca"].(map[string]any)
	if !ok {
		t.Fatalf("campo cobranca ausente ou com formato inesperado: %s", raw)
	}
	if cobranca["qrCodeArr"] != "base64-qr-mensalidade" {
		t.Fatalf("qrCodeArr não chegou na resposta de pagamento de mensalidade: %s", raw)
	}
}

// TestMatriculaPagamentoViewIncludesQRCodeArr é o equivalente de
// TestMensalidadePagamentoViewIncludesQRCodeArr para o fluxo de matrícula.
func TestMatriculaPagamentoViewIncludesQRCodeArr(t *testing.T) {
	view := MatriculaPagamentoView{
		Charge: QRCodeResult{
			ChargeResult: ChargeResult{ID: uuid.New(), MerchantTransactionID: "M2", Status: "criada"},
			QRCodeArr:    "base64-qr-matricula",
		},
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	cobranca, ok := decoded["cobranca"].(map[string]any)
	if !ok {
		t.Fatalf("campo cobranca ausente ou com formato inesperado: %s", raw)
	}
	if cobranca["qrCodeArr"] != "base64-qr-matricula" {
		t.Fatalf("qrCodeArr não chegou na resposta de pagamento de matrícula: %s", raw)
	}
}

// TestMensalidadePagamentoViewOmiteQRCodeArrParaOutrosMetodos garante que a
// correção não introduz o campo qrCodeArr (mesmo vazio) para métodos que não
// são GPO_QR — QRCodeResult tem omitempty em QRCodeArr especificamente para
// isso.
func TestMensalidadePagamentoViewOmiteQRCodeArrParaOutrosMetodos(t *testing.T) {
	view := MensalidadePagamentoView{
		Charge: QRCodeResult{ChargeResult: ChargeResult{ID: uuid.New(), MerchantTransactionID: "M3", Status: "criada"}},
		Meses:  []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 3}},
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	cobranca, ok := decoded["cobranca"].(map[string]any)
	if !ok {
		t.Fatalf("campo cobranca ausente ou com formato inesperado: %s", raw)
	}
	if _, present := cobranca["qrCodeArr"]; present {
		t.Fatalf("qrCodeArr não deveria aparecer no JSON quando vazio (omitempty): %s", raw)
	}
}
```

### A.4 — Teste novo (integração, ponta a ponta, com Postgres): `internal/finance/qrcode_regression_integration_test.go`

Prova o problema real de ponta a ponta: `IniciarPagamentoMensalidades`/`IniciarPagamentoMatricula` com
`metodo_pagamento = "GPO_QR"`, contra a AppyPay simulada (mock transport já existente,
`appyPayMockTransport`, que já devolve `qrCodeArr` para `POST /qr-codes` — ver
`internal/finance/appypay_integration_test.go` linhas 33-34), confirmando que `QRCodeArr` chega não-vazio na
`view` devolvida pelo `Service`. Reaproveita os helpers já existentes do pacote
(`integrationClient`, `configureIntegrationCredential`, `appyPayMockTransport`, `mensalidadeCodigo`,
`seedMensalidadeAcademia`, `seedMensalidadeTurma`, `seedMensalidadeConfiguracao`, `seedMatriculaPendente`,
`NivelFundamental`) — nenhum helper novo é necessário.

Criar o arquivo com exatamente este conteúdo:

```go
package finance

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr reproduz o
// Problema 2 documentado em
// "docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md" de
// ponta a ponta: antes da correção, esta chamada devolvia
// view.Charge.QRCodeArr == "" mesmo com a AppyPay (simulada aqui pelo mock
// transport) devolvendo qrCodeArr normalmente.
func TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "EST-QR-" + uuid.NewString()[:8]
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-QR", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO_QR}' WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}

	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	service.SetHTTPClient(&http.Client{Transport: &appyPayMockTransport{status: "Pending"}})

	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendentes) == 0 {
		t.Fatal("esperava pelo menos uma mensalidade pendente")
	}
	alvo := pendentes[0]

	view, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia,
		Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
		MetodoPagamento: "GPO_QR",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
	}
	if view.Charge.QRCodeArr == "" {
		t.Fatalf("qrCodeArr não chegou na resposta de pagamento de mensalidade GPO_QR: %#v", view.Charge)
	}
}

// TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr é o equivalente de
// TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr para o fluxo de
// matrícula.
func TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	codigo := seedMatriculaPendente(t, client, academia, 750)
	if _, err := client.DB().Exec(`UPDATE projection_solicitacoes_matricula SET metodos_pagamento_matricula='{GPO_QR}' WHERE codigo_solicitacao=$1`, codigo); err != nil {
		t.Fatal(err)
	}

	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	service.SetHTTPClient(&http.Client{Transport: &appyPayMockTransport{status: "Pending"}})

	view, err := service.IniciarPagamentoMatricula(ctx, MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "GPO_QR"}, "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMatricula falhou: %v", err)
	}
	if view.Charge.QRCodeArr == "" {
		t.Fatalf("qrCodeArr não chegou na resposta de pagamento de matrícula GPO_QR: %#v", view.Charge)
	}
}
```

---

## Tarefa B — Problema 1: endpoint de listagem de cobranças/pagamentos

### B.1 — `internal/finance/appypay.go`: import novo

**Localizar** (bloco de imports, começo do arquivo):

```go
import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)
```

**Substituir por:**

```go
import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)
```

(`github.com/lib/pq` já é dependência direta do módulo — `go.mod` linha 14 — nenhuma alteração em `go.mod`/`go.sum` é necessária.)

### B.2 — `internal/finance/appypay.go`: tipos novos (DTO da listagem)

**Localizar:**

```go
type QRCodeResult struct {
	ChargeResult
	QRCodeArr string `json:"qrCodeArr,omitempty"`
}
type tokenEntry struct {
```

**Substituir por:**

```go
type QRCodeResult struct {
	ChargeResult
	QRCodeArr string `json:"qrCodeArr,omitempty"`
}

// CobrancaResumo é o resumo de uma cobrança devolvido pela listagem
// GET /financeiro/cobrancas. Deliberadamente não inclui payment_info,
// response nem qrCodeArr — esses detalhes completos continuam disponíveis
// apenas em GET /financeiro/appypay/cobrancas/:id, para quem já sabe o
// identificador da cobrança. Ver Problema 1 em
// docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md.
type CobrancaResumo struct {
	ID                    uuid.UUID `json:"id"`
	ProviderChargeID      string    `json:"provider_charge_id,omitempty"`
	MerchantTransactionID string    `json:"merchant_transaction_id"`
	ContextoTipo          string    `json:"contexto_tipo"`
	CodigoAcademia        string    `json:"codigo_academia,omitempty"`
	// Origem é derivada do payload da cobrança, nunca persistida
	// separadamente: "matricula" quando há codigo_solicitacao,
	// "mensalidade" quando há codigo_estudante (e não há
	// codigo_solicitacao), "avulsa" nos demais casos (cobrança criada
	// diretamente via POST /financeiro/appypay/cobrancas ou /appypay/qr-codes
	// sem vínculo a mensalidade nem matrícula).
	Origem string  `json:"origem"`
	Status string  `json:"status"`
	Valor  float64 `json:"valor"`
	Moeda  string  `json:"moeda,omitempty"`
	Descricao string `json:"descricao,omitempty"`
	// MetodoPagamento reflete "GPO_QR" (não apenas "GPO") quando a cobrança
	// tem qr_code_type no payload — CreateGPOQRCode grava payment_method
	// como "GPO" internamente, então sem este ajuste a origem QR ficaria
	// indistinguível de um GPO comum nesta listagem.
	MetodoPagamento   string                   `json:"metodo_pagamento,omitempty"`
	CodigoEstudante   string                   `json:"codigo_estudante,omitempty"`
	CodigoSolicitacao string                   `json:"codigo_solicitacao,omitempty"`
	Mensalidades      []MensalidadeSelecaoMes  `json:"mensalidades,omitempty"`
	AtualizadoEm      time.Time                `json:"atualizado_em"`
}

// CobrancaListResult é o resultado paginado de ListCobrancas.
type CobrancaListResult struct {
	Cobrancas []CobrancaResumo `json:"cobrancas"`
	Total     int              `json:"total"`
}

type tokenEntry struct {
```

(Não se preocupe com o alinhamento das colunas dos campos da struct acima — o passo `gofmt -w` no checklist
final corrige isso automaticamente.)

### B.3 — `internal/finance/appypay.go`: método `ListCobrancas`

**Localizar** (fim de `CancelCharge`, logo antes de `canCancelCharge`):

```go
	return ChargeResult{ID: row.ID, ProviderChargeID: current.ProviderChargeID, MerchantTransactionID: row.Merchant, Status: "cancelada", Response: current.Response}, nil
}

func canCancelCharge(row chargeRow, academia, actorType string) bool {
```

**Substituir por:**

```go
	return ChargeResult{ID: row.ID, ProviderChargeID: current.ProviderChargeID, MerchantTransactionID: row.Merchant, Status: "cancelada", Response: current.Response}, nil
}

// ListCobrancas lista cobranças AppyPay (mensalidade, matrícula ou avulsa)
// filtrando por contexto/academia, estado (status) e origem, com paginação.
// contexto e academia vazios não restringem a consulta — o mesmo padrão de
// filtro opcional já usado em ListCredentials. estados e origens vazios
// também não restringem. limit/offset devem já vir validados (bounded) por
// quem chama (ver handlers.ListarCobrancasAppyPay).
//
// O filtro de estado é um match exato (case-sensitive) sobre o texto cru
// persistido em payload->>'status' — o mesmo texto devolvido no campo
// "status" de GET /financeiro/appypay/cobrancas/:id. Esse campo mistura
// estados internos do Spuri ("solicitada", "criada", "cancelada", "falhada")
// com estados crus devolvidos pela AppyPay ("Success", "Pending", "Failed",
// etc.) — este método deliberadamente não normaliza nada, pelo mesmo motivo
// que isSuccessfulChargeStatus/isTerminalChargeStatus já fazem sua própria
// comparação case-insensitive em vez de normalizar o dado persistido.
func (s *Service) ListCobrancas(ctx context.Context, contexto, academia string, estados, origens []string, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	where := "WHERE 1=1"
	args := []any{}
	i := 1
	if contexto != "" {
		where += fmt.Sprintf(" AND contexto_tipo=$%d", i)
		args = append(args, contexto)
		i++
	}
	if academia != "" {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, academia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estados))
		i++
	}
	if len(origens) > 0 {
		clauses := make([]string, 0, len(origens))
		for _, origem := range origens {
			switch origem {
			case "matricula":
				clauses = append(clauses, "COALESCE(payload->>'codigo_solicitacao','') <> ''")
			case "mensalidade":
				clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_estudante','') <> '')")
			case "avulsa":
				clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_estudante','') = '')")
			default:
				return nil, fmt.Errorf("tipo de cobrança inválido: %s", origem)
			}
		}
		where += " AND (" + strings.Join(clauses, " OR ") + ")"
	}
	var total int
	if err := s.client.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM financeiro_cobrancas "+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`SELECT id, COALESCE(provider_charge_id,''), merchant_transaction_id, contexto_tipo, COALESCE(codigo_academia,''), payload, updated_at FROM financeiro_cobrancas %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, where, i, i+1)
	args = append(args, limit, offset)
	rows, err := s.client.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CobrancaResumo{}
	for rows.Next() {
		var dto CobrancaResumo
		var rawPayload []byte
		if err := rows.Scan(&dto.ID, &dto.ProviderChargeID, &dto.MerchantTransactionID, &dto.ContextoTipo, &dto.CodigoAcademia, &rawPayload, &dto.AtualizadoEm); err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := json.Unmarshal(rawPayload, &payload); err != nil {
			return nil, err
		}
		dto.Status, _ = payload["status"].(string)
		dto.Valor, _ = payload["amount"].(float64)
		dto.Moeda, _ = payload["currency"].(string)
		dto.Descricao, _ = payload["description"].(string)
		dto.MetodoPagamento, _ = payload["payment_method"].(string)
		if qrType, ok := payload["qr_code_type"].(string); ok && qrType != "" {
			dto.MetodoPagamento = "GPO_QR"
		}
		dto.CodigoEstudante, _ = payload["codigo_estudante"].(string)
		dto.CodigoSolicitacao, _ = payload["codigo_solicitacao"].(string)
		switch {
		case dto.CodigoSolicitacao != "":
			dto.Origem = "matricula"
		case dto.CodigoEstudante != "":
			dto.Origem = "mensalidade"
		default:
			dto.Origem = "avulsa"
		}
		if mesesRaw, ok := payload["mensalidades"]; ok && mesesRaw != nil {
			if b, err := json.Marshal(mesesRaw); err == nil {
				var meses []MensalidadeSelecaoMes
				if json.Unmarshal(b, &meses) == nil {
					dto.Mensalidades = meses
				}
			}
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
}

func canCancelCharge(row chargeRow, academia, actorType string) bool {
```

### B.4 — `internal/handlers/financeiro_handlers.go`: handler novo

**Localizar** (fim de `ConsultarCobrancaAppyPay`, logo antes do comentário de `CancelarCobrancaAppyPay`):

```go
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

// CancelarCobrancaAppyPay intentionally does not use authorizeFinanceScope:
```

**Substituir por:**

```go
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

// ListarCobrancasAppyPay lista cobranças (mensalidade, matrícula ou avulsa)
// do contexto autorizado, com filtros opcionais por estado e origem e
// paginação — resolve o Problema 1 documentado em
// docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md.
// Mesma autorização de ConsultarCobrancaAppyPay/ListarCredenciaisAppyPay:
// uma academia só vê as próprias cobranças; um admin precisa da permissão
// "fpp" e pode consultar qualquer contexto/academia via query string.
func ListarCobrancasAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := FinanceiroService.ListCobrancas(c.Request.Context(), contexto, academia, c.QueryArray("estado"), c.QueryArray("tipo"), limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset})
}

// CancelarCobrancaAppyPay intentionally does not use authorizeFinanceScope:
```

Nenhum import novo é necessário neste arquivo (`parseBoundedInt` já está definida no mesmo pacote, em
`internal/handlers/solicitacao_matricula_handlers.go`).

### B.5 — `cmd/server/main.go`: rota nova

**Localizar:**

```go
			financeiro.GET("/appypay/cobrancas/:id", handlers.ConsultarCobrancaAppyPay)
			financeiro.POST("/appypay/cobrancas/:id/cancelar", handlers.CancelarCobrancaAppyPay)
```

**Substituir por:**

```go
			financeiro.GET("/appypay/cobrancas/:id", handlers.ConsultarCobrancaAppyPay)
			financeiro.POST("/appypay/cobrancas/:id/cancelar", handlers.CancelarCobrancaAppyPay)
			financeiro.GET("/cobrancas", handlers.ListarCobrancasAppyPay)
```

### B.6 — Teste novo (integração, nível `Service`): `internal/finance/cobrancas_integration_test.go`

Prova `ListCobrancas` diretamente: isolamento por academia, filtro por estado, filtro por origem/tipo e
paginação. Insere linhas diretamente em `financeiro_cobrancas` (mesmo padrão já usado em
`internal/handlers/financeiro_handlers_integration_test.go`, função
`TestIntegrationFinanceRejectsAcademyChargeOutsideScope`) — mais simples e mais direto do que reproduzir o
fluxo completo de criação de cobrança para este teste específico, que é sobre a consulta, não sobre a
escrita.

Criar o arquivo com exatamente este conteúdo:

```go
package finance

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestIntegrationListCobrancasFiltraOrigemEstadoEIsolaPorAcademia cobre o
// Problema 1 documentado em
// "docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md":
// até esta correção não existia nenhuma consulta capaz de listar cobranças
// por academia, com ou sem filtro de estado.
func TestIntegrationListCobrancasFiltraOrigemEstadoEIsolaPorAcademia(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academiaA := mensalidadeCodigo()
	academiaB := mensalidadeCodigo()

	insert := func(academia, status, estudante, solicitacao string) {
		payload := map[string]any{"status": status, "amount": 500.0, "currency": "AOA", "description": "teste", "payment_method": "REF"}
		if estudante != "" {
			payload["codigo_estudante"] = estudante
		}
		if solicitacao != "" {
			payload["codigo_solicitacao"] = solicitacao
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), integrationMerchant("LST"), academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert(academiaA, "criada", "", "")            // avulsa
	insert(academiaA, "Success", "EST-LST-1", "")  // mensalidade paga
	insert(academiaA, "cancelada", "", "SOL-LST-1") // matrícula cancelada
	insert(academiaB, "criada", "", "")             // outra academia, não deve aparecer para A

	todas, err := service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if todas.Total != 3 {
		t.Fatalf("esperava 3 cobranças da academia A, obteve %d", todas.Total)
	}

	pagas, err := service.ListCobrancas(ctx, ContextoAcademia, academiaA, []string{"Success"}, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if pagas.Total != 1 || len(pagas.Cobrancas) != 1 || pagas.Cobrancas[0].Origem != "mensalidade" {
		t.Fatalf("filtro por estado=Success deveria devolver só a cobrança de mensalidade: %#v", pagas)
	}

	matriculas, err := service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, []string{"matricula"}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if matriculas.Total != 1 || matriculas.Cobrancas[0].CodigoSolicitacao != "SOL-LST-1" {
		t.Fatalf("filtro por tipo=matricula incorreto: %#v", matriculas)
	}

	pagina, err := service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(pagina.Cobrancas) != 1 || pagina.Total != 3 {
		t.Fatalf("paginação incorreta: len=%d total=%d", len(pagina.Cobrancas), pagina.Total)
	}

	outraAcademia, err := service.ListCobrancas(ctx, ContextoAcademia, academiaB, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if outraAcademia.Total != 1 {
		t.Fatalf("academia B deveria ver só a própria cobrança, obteve %d", outraAcademia.Total)
	}
}
```

### B.7 — Teste novo (integração, nível handler HTTP): `internal/handlers/financeiro_cobrancas_handlers_test.go`

Prova a rota `ListarCobrancasAppyPay` como um cliente HTTP a veria: escopo por academia (isolamento entre
academias, mesmo padrão de `TestIntegrationFinanceRejectsAcademyChargeOutsideScope`) e rejeição de admin sem
permissão `"fpp"` (mesmo padrão de `TestIntegrationFinanceRejectsNonFPPAdmins`) — ambos já existentes em
`internal/handlers/financeiro_handlers_integration_test.go`, reaproveitando o helper `integrationFinanceClient`
já definido lá.

Criar o arquivo com exatamente este conteúdo:

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
)

// TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado cobre o
// Problema 1 no nível HTTP: uma academia só vê as próprias cobranças, e os
// filtros de estado/tipo funcionam através da rota real.
func TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academiaA := "LSTA" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
	academiaB := "LSTB" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]

	insert := func(academia, status, origemCampo, origemValor string) {
		payload := map[string]any{"status": status, "amount": 250.0, "currency": "AOA", "description": "teste", "payment_method": "REF"}
		if origemCampo != "" {
			payload[origemCampo] = origemValor
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		merchant := "LST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
			uuid.New(), merchant, academia, raw); err != nil {
			t.Fatal(err)
		}
	}
	insert(academiaA, "criada", "", "")
	insert(academiaA, "Success", "codigo_estudante", "EST-LST-1")
	insert(academiaA, "cancelada", "codigo_solicitacao", "SOL-LST-1")
	insert(academiaB, "criada", "", "")

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	call := func(academia, query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?"+query, nil)
		ctx.Set("dbClient", client)
		ctx.Set("user_id", uuid.New())
		ctx.Set("user_type", "academia")
		ctx.Set("codigo_academia", academia)
		ListarCobrancasAppyPay(ctx)
		return recorder
	}

	var body struct {
		Cobrancas []struct {
			Origem string `json:"origem"`
			Status string `json:"status"`
		} `json:"cobrancas"`
		TotalGeral int `json:"total_geral"`
	}

	all := call(academiaA, "")
	if all.Code != http.StatusOK {
		t.Fatalf("listagem sem filtro = %d: %s", all.Code, all.Body.String())
	}
	if err := json.Unmarshal(all.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 3 {
		t.Fatalf("academia A deveria ver 3 cobranças próprias, viu %d: %s", body.TotalGeral, all.Body.String())
	}

	filtrada := call(academiaA, "estado=Success")
	if err := json.Unmarshal(filtrada.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 1 || len(body.Cobrancas) != 1 || body.Cobrancas[0].Origem != "mensalidade" {
		t.Fatalf("filtro por estado=Success deveria devolver só a cobrança de mensalidade paga: %s", filtrada.Body.String())
	}

	outraAcademia := call(academiaB, "")
	if err := json.Unmarshal(outraAcademia.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TotalGeral != 1 {
		t.Fatalf("academia B deveria ver só a própria cobrança, viu %d", body.TotalGeral)
	}
}

// TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP garante
// que a nova rota usa a mesma autorização das demais rotas de /financeiro:
// um admin sem a permissão "fpp" não pode listar cobranças.
func TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	adminID := uuid.New()
	if _, err := client.DB().Exec(`INSERT INTO projection_admins (id,nome,email,senha_hash,role,status) VALUES ($1,'gerente-lst',$2,'hash','gerente','ativo')`, adminID, "gerente-lst-"+uuid.NewString()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas", nil)
	ctx.Set("dbClient", client)
	ctx.Set("user_id", adminID)
	ctx.Set("user_type", "admin")

	ListarCobrancasAppyPay(ctx)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("admin sem permissão fpp recebeu %d, queria 403: %s", recorder.Code, recorder.Body.String())
	}
}
```

---

## Ordem de execução recomendada

1. Aplicar A.1 (`internal/finance/mensalidade.go`) e A.2 (`internal/finance/matricula.go`).
2. Criar A.3 (`internal/finance/qrcode_pagamento_view_test.go`) e A.4
   (`internal/finance/qrcode_regression_integration_test.go`).
3. Aplicar B.1, B.2 e B.3, todos em `internal/finance/appypay.go`, na ordem (import, tipos, método).
4. Aplicar B.4 (`internal/handlers/financeiro_handlers.go`) e B.5 (`cmd/server/main.go`).
5. Criar B.6 (`internal/finance/cobrancas_integration_test.go`) e B.7
   (`internal/handlers/financeiro_cobrancas_handlers_test.go`).
6. Rodar:
   ```
   gofmt -w internal/finance/appypay.go internal/finance/mensalidade.go internal/finance/matricula.go \
     internal/finance/qrcode_pagamento_view_test.go internal/finance/qrcode_regression_integration_test.go \
     internal/finance/cobrancas_integration_test.go internal/handlers/financeiro_handlers.go \
     internal/handlers/financeiro_cobrancas_handlers_test.go cmd/server/main.go
   ```
7. Executar a checklist de aceitação abaixo, na ordem.

---

## Checklist de aceitação

Execute cada item na ordem. Se qualquer um falhar, pare e reporte o erro exato — não prossiga nem tente uma
correção diferente da especificada acima sem antes reportar.

1. **Build e vet limpos:**
   ```
   go build ./...
   go vet ./...
   ```
   Ambos devem terminar sem nenhuma saída de erro.

2. **`gofmt` não encontra nada pendente:**
   ```
   gofmt -l .
   ```
   Deve devolver vazio (nenhum arquivo listado). Se listar algo, rode `gofmt -w` no(s) arquivo(s) listado(s)
   e confirme de novo.

3. **Suíte `internal/finance` (unitária + integração), 5 execuções seguidas, banco recriado do zero a cada
   vez:**
   ```
   for i in 1 2 3 4 5; do
     psql -c "DROP DATABASE IF EXISTS spuri_test;" -U postgres
     psql -c "CREATE DATABASE spuri_test;" -U postgres
     RUN_POSTGRES_INTEGRATION=1 DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
       DB_NAME=spuri_test DB_SSLMODE=disable APPYPAY_RESOURCE=integration-resource \
       FINANCE_ENCRYPTION_KEY=01234567890123456789012345678901 ENV=test \
       go test -count=1 ./internal/finance/... -v
   done
   ```
   Todas as 5 execuções devem terminar com `PASS`/`ok`, sem nenhum `FAIL`. Confirme especificamente que
   aparecem como `--- PASS` em cada execução:
   - `TestMensalidadePagamentoViewIncludesQRCodeArr`
   - `TestMatriculaPagamentoViewIncludesQRCodeArr`
   - `TestMensalidadePagamentoViewOmiteQRCodeArrParaOutrosMetodos`
   - `TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr`
   - `TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr`
   - `TestIntegrationListCobrancasFiltraOrigemEstadoEIsolaPorAcademia`

   E que os testes já existentes relacionados continuam passando (garantindo que nada foi quebrado), em
   especial `TestQRCodeIdempotencyPayloadAndPersistedResult`,
   `TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago`,
   `TestIntegrationMatriculaPagamentoFixaValorImpedeDuplicidadeECancelaEmCascata` e
   `TestIntegrationConfigureMensalidadeGravaNoLedgerEProjectaCorretamente`.

4. **Suíte `internal/handlers`, 5 execuções seguidas, banco recriado do zero a cada vez:**
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
   aparecem como `--- PASS` em cada execução:
   - `TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado`
   - `TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP`

   E que os testes já existentes de escopo financeiro continuam passando, em especial
   `TestIntegrationFinanceRejectsAcademyChargeOutsideScope`, `TestIntegrationFinanceRejectsNonFPPAdmins`,
   `TestIntegrationFinanceFPPAdminCannotCancelAcademyCharge` e
   `TestIntegrationConsultarCobrancaAppyPayNaoEfetivaMatriculaAposSuccess`.

5. **Suíte completa do repositório:**
   ```
   RUN_POSTGRES_INTEGRATION=1 DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
     DB_NAME=spuri_test DB_SSLMODE=disable APPYPAY_RESOURCE=integration-resource \
     FINANCE_ENCRYPTION_KEY=01234567890123456789012345678901 ENV=test \
     go test ./...
   ```
   Todos os pacotes devem terminar `ok`.

6. **Diff final — confirmar que apenas os arquivos esperados foram alterados:**
   ```
   git diff --stat
   git status --short
   ```
   Deve mostrar exatamente estes arquivos modificados, e nenhum outro (nem `go.mod`, nem `go.sum`):
   - `internal/finance/appypay.go`
   - `internal/finance/mensalidade.go`
   - `internal/finance/matricula.go`
   - `internal/handlers/financeiro_handlers.go`
   - `cmd/server/main.go`

   E exatamente estes arquivos novos, não rastreados antes desta tarefa:
   - `internal/finance/qrcode_pagamento_view_test.go`
   - `internal/finance/qrcode_regression_integration_test.go`
   - `internal/finance/cobrancas_integration_test.go`
   - `internal/handlers/financeiro_cobrancas_handlers_test.go`

Se todos os itens passarem, a tarefa está concluída. Mova este arquivo de
`docs/Lista de Tarefas/46 - Correção de dois problemas de backend do módulo de pagamentos (listagem de
cobranças e QR Code na resposta de pagamento).md` para
`docs/Tarefas feitas/46 - Correção de dois problemas de backend do módulo de pagamentos (listagem de
cobranças e QR Code na resposta de pagamento).md`, atualize o front-matter (`status: feito`), e também
atualize `docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md` (front-matter `status: feito
— ver tarefa 46`) para refletir que os dois problemas já foram corrigidos no backend — isso desbloqueia o
retrabalho da tela `/financas/pagamentos` do frontend, que hoje está contornando os dois problemas
(listagem substituída por três buscas pontuais, e `GPO_QR` escondido do pagador final) exatamente como
descrito naquele documento.
