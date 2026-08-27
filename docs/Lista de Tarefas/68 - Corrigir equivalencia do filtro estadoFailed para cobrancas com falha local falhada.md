---
criado: 2026-08-27
origem: conversa com Fredy (Claude como orquestrador, Codex como executor)
status: pronto_para_execucao
tipo: correcao_de_bug
depende_de: docs/Tarefas feitas/66 - Novo modelo de estados de cobranca e correcao do filtro estadotipo na lista unificada.md
gera_dependencia_para: tarefa companion no repositório spuripainel (frontend) — ver seção 0
---

# Corrigir equivalência do filtro `estado=Failed` para cobranças com falha local (`falhada`)

## 0. Leia isto primeiro — sobre o seu ambiente (Codex) e sobre a tarefa companion do frontend

Você não tem `apt`, Docker nem `psql`. Não precisa disso aqui. Claude já validou esta correção inteira com **PostgreSQL 16 real e Go 1.24 real**, num sandbox próprio: aplicou cada mudança, recriou o banco do zero várias vezes, e rodou a suíte de testes inteira (`go build ./...`, `go vet ./...`, `go test ./...`, e `go test ./... -race` nos dois pacotes tocados) repetidamente até tudo ficar verde — incluindo os 5 testes novos/alterados escritos especificamente para esta tarefa. A seção 9 (Evidência de validação) tem os comandos exatos e a saída real de cada rodada.

**Sua validação usa só `go build ./...`, `go vet ./...`, `gofmt -l` e `go test ./...`** (os testes de integração pulam automaticamente sem `RUN_POSTGRES_INTEGRATION`, isso é esperado e não é um problema seu — mas como Claude já rodou tudo com PostgreSQL real, você não precisa se preocupar em validar o comportamento de tempo de execução, só que o código compila e os testes que não precisam de banco continuam passando).

**Esta tarefa nasceu de dois problemas que Fredy relatou na mesma conversa, mas só o segundo é backend:**

1. **Frontend:** em `/financas/pagamentos` e `/pagamentos`, não havia como filtrar só os pagamentos com `status: "pendente"` (pendências sintéticas, sem nenhuma cobrança gerada) — só existia a opção "Aguardando pagamento" (`aguardando_pagamento`, cobrança real já gerada mas sem resolução). O backend já suportava perfeitamente `estado=pendente` desde a tarefa 66 (ver `DeveIncluirPendenciasSemCobranca` em `internal/finance/pagamentos_unificado.go`) — faltava só a opção no dropdown do frontend. **Esta tarefa (backend) não toca em nada relacionado a isso.** A correção é a tarefa companion no repositório `spuripainel` (tarefa 68 lá), resumida na seção 0.1 abaixo para contexto — diagnosticada, com o diff pronto e já validado por Claude (`tsc --noEmit` e `eslint` limpos), mas ainda não aplicada ao repositório real: quem executa esse diff mecanicamente é a instância de Codex trabalhando na tarefa 68 do `spuripainel`, não você — não é responsabilidade sua reaplicá-la aqui.
2. **Backend (esta tarefa):** `GET /financeiro/cobrancas/estudante/:codigo?estado=Failed` devolvia uma lista vazia mesmo o estudante tendo cobranças reais com falha, reproduzido também para `GET /financeiro/cobrancas` (academia/admin). Diagnóstico completo na seção 2.

### 0.1. Resumo da tarefa companion do frontend (spuripainel, tarefa 68) — diagnosticada e validada por Claude, não sua responsabilidade

Em `src/components/paineis/financeiroShared.tsx`, a constante `ESTADO_PAGAMENTO_OPCOES` (compartilhada por `FinanceiroPagamentosPainel.tsx` e `EstudantePagamentosPainel.tsx`) ganha a opção `{ value: "pendente", label: "Pendente (sem cobrança gerada)" }`. Nenhuma outra mudança é necessária: `StatusBadge`, `CobrancasTable` e `SubtelaDetalheCobranca` já tratam corretamente `status === "pendente"` desde a tarefa 67 (companion da 66) — a lista unificada já mistura pendências sintéticas com cobranças reais por padrão, só faltava a opção de filtro explícita. `npx tsc --noEmit` e `npx eslint` (escopado aos 3 arquivos que importam a constante) limpos, sem nenhum erro novo introduzido — validado por Claude no mesmo sandbox, mas o diff ainda precisa ser aplicado ao repositório `spuripainel` por uma instância de Codex própria (documento separado, tarefa 68 desse repositório, não é este documento nem esta tarefa). Documento completo: `src/docs/68 - Adicionar opcao Pendente ao filtro de estado da lista unificada de pagamentos.md` no repositório `spuripainel`.

---

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento, nesta ordem. Todas as decisões de desenho já foram tomadas e validadas por Claude (diagnóstico completo com evidência de código, implementação testada com PostgreSQL 16 e Go 1.24 reais). Sua tarefa é mecânica: (1) aplicar os 2 diffs cirúrgicos em `internal/finance/appypay.go` e `internal/finance/pagamentos_unificado.go` descritos na seção 4; (2) aplicar os 5 diffs cirúrgicos nos arquivos de teste descritos na seção 5; (3) atualizar `Documentação da API.md` conforme os diffs da seção 6; (4) rodar cada item da seção "Checklist de validação" (seção 8) e reportar o resultado; (5) seguir o "Procedimento de conclusão" (seção 11). Não toque em nenhum arquivo ou lógica fora do escopo listado na seção 7 ("Fora de escopo"). Não é necessário PostgreSQL, Docker nem `psql` — os diffs já foram validados com esses dois reais por Claude.

---

## 2. Contexto e diagnóstico

### 2.1. O relato de Fredy

> Ao consultar apenas as cobranças que deram erro no pagamento (falha) como estudante, não retornou nada sendo que ao consultar tudo é retornado pagamentos falhos. Esse erro acontece pra admin e academia também.
> `/financeiro/cobrancas/estudante/SJS1125?estado=Failed&limit=30&offset=0`

Fredy também anexou o resultado de uma consulta sem filtro para o mesmo estudante, mostrando itens com `"status": "falhada"` (minúsculo, em português) misturados com itens `"status": "pendente"` — confirmando que existem cobranças reais nesse estado, e que o valor bruto persistido para elas é literalmente `"falhada"`, não `"Failed"`.

### 2.2. Causa raiz confirmada em código

Duas origens distintas para "a cobrança falhou", gravadas como dois valores brutos diferentes no payload persistido em `financeiro_cobrancas` (`internal/finance/appypay.go`, antes desta tarefa):

- **`"Failed"`** — a AppyPay processou a cobrança e o **processador** a recusou. É o valor que a própria AppyPay devolve no corpo da resposta (`responseStatus(response)`), documentado em `docs/Parceiros e integrações/AppyPay Documentação.md`.
- **`"falhada"`** — a própria **chamada HTTP** do Spuri à AppyPay falhou (erro de rede, timeout, ou a AppyPay respondeu um HTTP não-2xx — ver `callJSON`), então nunca chegou a existir cobrança nenhuma do lado do provedor. Gravado explicitamente em dois pontos, ambos no branch `if err != nil` logo após a chamada:
  - `CreateCharge` (linha ~478): evento `CobrancaAppyPayFalhou`.
  - `CreateGPOQRCode` (linha ~555): evento `QRCodeAppyPayFalhou`.

O filtro SQL em `ListCobrancas`/`ListCobrancasEstudante` (`internal/finance/appypay.go`) compara o valor do parâmetro `estado` diretamente contra o valor BRUTO persistido (`payload->>'status' = ANY($n)`), passando antes pela função `estadosCobrancaEquivalentes`, que — antes desta tarefa — só tinha uma equivalência especial para `aguardando_pagamento` (mapeando para os valores históricos `Pending`/`Requested`/`solicitada`/`criada`, ver tarefa 66). Para qualquer outro valor, incluindo `"Failed"`, a função devolvia o valor inalterado — então `estado=Failed` só casava com a string literal `"Failed"`, nunca com `"falhada"`. Resultado: toda cobrança gravada como `"falhada"` fica invisível para esse filtro, em `ListCobrancas` (academia/admin) e `ListCobrancasEstudante` (estudante) igualmente — exatamente o sintoma relatado, nos dois papéis.

Confirmado por leitura direta do código (`estadosCobrancaEquivalentes`, antes desta tarefa):

```go
func estadosCobrancaEquivalentes(estados []string) []string {
	out := make([]string, 0, len(estados))
	for _, estado := range estados {
		if strings.EqualFold(strings.TrimSpace(estado), EstadoCobrancaAguardandoPagamento) {
			out = append(out, EstadoCobrancaAguardandoPagamento, "Pending", "Requested", "solicitada", "criada")
			continue
		}
		out = append(out, estado)
	}
	return out
}
```

### 2.3. Por que isso não foi pego na tarefa 66

A tarefa 66 generalizou o suporte aos quatro estados terminais documentados pela AppyPay (`Success`, `Failed`, `Cancelled`, `Expired`) e corrigiu a equivalência histórica só para `aguardando_pagamento` — mas não tocou em `"falhada"`, que já existia antes da 66 e continuou sendo gravado normalmente pelo branch de erro de rede/HTTP de `CreateCharge`/`CreateGPOQRCode`, sem que ninguém tivesse ainda testado especificamente o filtro `estado=Failed` contra uma cobrança gravada assim.


---

## 3. Desenho da solução e decisões de projeto

**Decisão de Fredy, explícita na conversa: "daqui pra frente suportar apenas Failed/failed".** Duas leituras possíveis do bug foram consideradas:

- (A) Tratar `"Failed"` e `"falhada"` como dois valores permanentemente distintos, só ensinando o *filtro* a enxergar os dois (`estadosCobrancaEquivalentes` expande `Failed` → `{Failed, falhada}`), sem tocar em nada mais — os dois nomes continuariam aparecendo nas respostas da API para sempre, dependendo de qual foi a causa da falha.
- (B) Tratar `"falhada"` exatamente como a tarefa 66 já trata os valores brutos antigos de `aguardando_pagamento` (`Pending`/`Requested`/`solicitada`/`criada`): um valor **histórico**, que só existe em cobranças já persistidas antes desta tarefa (o ledger é append-only, imutável) — novas cobranças passam a usar só o vocabulário canônico, e a leitura de linhas antigas é normalizada de volta para esse vocabulário, então nenhum chamador (frontend, integração, suporte) nunca mais vê `"falhada"` em uma resposta.

Fredy escolheu (B). Isso significa três mudanças, não uma:

1. **Escrita:** `CreateCharge` e `CreateGPOQRCode` passam a gravar `"Failed"` diretamente (não mais `"falhada"`) quando a própria chamada HTTP à AppyPay falha. O campo `error: "provider_request_failed"` no payload do evento continua existindo — quem precisar distinguir "processador recusou" de "Spuri nem conseguiu tentar" ainda consegue, só não é mais pelo campo `status`.
2. **Leitura:** `normalizeChargeStatus` (chamada em todo ponto de leitura — `scanCobrancaResumo`, a listagem unificada, `consultCharge`) passa a traduzir `"falhada"` → `"Failed"`, exatamente no mesmo padrão que já traduz `"criada"`/`"solicitada"`/`"Pending"`/`"Requested"` → `"aguardando_pagamento"`. Cobranças antigas continuam existindo com o valor bruto `"falhada"` na coluna `payload`, mas nunca mais chegam assim a um chamador.
3. **Filtro:** `estadosCobrancaEquivalentes` ganha o mesmo tipo de equivalência histórica que já existe para `aguardando_pagamento`: filtrar por `estado=Failed` expande para casar tanto `Failed` quanto `falhada` na cláusula SQL, porque essa cláusula compara o valor BRUTO (não passa por `normalizeChargeStatus`). Sem essa expansão, cobranças antigas gravadas como `falhada` nunca apareceriam ao filtrar por `Failed`, mesmo já aparecendo normalizadas como `"Failed"` numa consulta sem filtro — inconsistência pior que a original. A direção é só uma (filtrar por `falhada` diretamente continua estrito): o frontend só expõe `Failed` como opção (`ESTADO_PAGAMENTO_OPCOES` em `financeiroShared.tsx`), nunca `falhada`.

As três mudanças moram todas em `internal/finance/appypay.go` — nenhuma outra função ou arquivo depende do valor `"falhada"` de um jeito que essa tradução quebre (ver seção 7, "Fora de escopo", para os lugares que foram checados e não precisam mudar).

---

## 4. Diffs exatos — arquivos de código (2 arquivos)

Cada bloco abaixo é um diff unificado real, gerado por Claude comparando um clone limpo de `main` (já com a tarefa 67, "Unificar geração de cobrança REF-GPO-GPOQR num único ponto", mesclada) com o estado já validado no seu próprio sandbox (PostgreSQL 16 + Go 1.24 reais). Aplique cada hunk (`@@ ... @@`) exatamente: remova as linhas que começam com `-`, adicione as linhas que começam com `+`, mantendo as linhas de contexto (sem prefixo) como estão. Não altere nada fora dos hunks mostrados.

```diff
diff --git a/internal/finance/appypay.go b/internal/finance/appypay.go
index 39048d8..2777985 100644
--- a/internal/finance/appypay.go
+++ b/internal/finance/appypay.go
@@ -475,8 +475,8 @@ func (s *Service) CreateCharge(ctx context.Context, in ChargeRequest, actorID, a
 	providerBody := map[string]any{"amount": in.Amount, "currency": in.Currency, "description": in.Description, "merchantTransactionId": in.MerchantTransactionID, "paymentMethod": method, "paymentInfo": in.PaymentInfo, "options": in.Options, "notify": in.Notify}
 	response, err := s.callJSON(ctx, credential, http.MethodPost, "/charges", providerBody, in.Async)
 	if err != nil {
-		_ = s.record(ctx, id, "CobrancaAppyPayFalhou", chargePayload(id, in, "", "falhada", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
-		return ChargeResult{ID: id, MerchantTransactionID: in.MerchantTransactionID, Status: "falhada"}, err
+		_ = s.record(ctx, id, "CobrancaAppyPayFalhou", chargePayload(id, in, "", "Failed", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
+		return ChargeResult{ID: id, MerchantTransactionID: in.MerchantTransactionID, Status: "Failed"}, err
 	}
 	providerID := responseID(response)
 	status := normalizeChargeStatus(responseStatus(response))
@@ -552,7 +552,7 @@ func (s *Service) CreateGPOQRCode(ctx context.Context, in QRCodeRequest, actorID
 	}
 	response, err := s.callJSON(ctx, cred, http.MethodPost, "/qr-codes", body, false)
 	if err != nil {
-		_ = s.record(ctx, id, "QRCodeAppyPayFalhou", qrCodePayload(id, in, typ, "", "falhada", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
+		_ = s.record(ctx, id, "QRCodeAppyPayFalhou", qrCodePayload(id, in, typ, "", "Failed", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
 		return QRCodeResult{}, err
 	}
 	providerID := responseID(response)
@@ -724,18 +724,40 @@ func (s *Service) ListCobrancas(ctx context.Context, contexto, academia string,
 // as cobranças criadas DEPOIS do deploy desta tarefa, escondendo qualquer
 // cobrança antiga que ainda esteja nesse estado — inconsistente com o que
 // scanCobrancaResumo mostra ao ler a mesma linha (que já normaliza na
-// leitura). Qualquer outro valor de filtro (Success, Failed, Cancelled,
-// Expired, ou qualquer string não reconhecida, incluindo EstadoPendente)
-// passa inalterado — só "aguardando_pagamento" tem essa equivalência
-// histórica com valores brutos diferentes de si mesmo.
+// leitura).
+//
+// A mesma lacuna existia para "Failed" (bug relatado por Fredy: GET
+// .../estudante/:codigo?estado=Failed devolvia vazio mesmo havendo
+// cobranças falhadas do estudante, reproduzido também para academia/admin
+// — ver tarefa 69). "Failed" é o valor cru que a própria AppyPay devolve
+// quando o PROCESSADOR recusa a cobrança (docs/Parceiros e integrações/
+// AppyPay Documentação.md). Antes da tarefa 69, CreateCharge/
+// CreateGPOQRCode gravavam um valor local diferente, "falhada", quando a
+// própria chamada HTTP à AppyPay falhava — nunca chegando a existir uma
+// cobrança do lado do provedor, então a AppyPay nunca teve chance de
+// devolver "Failed". Por decisão de Fredy, esta tarefa unifica os dois:
+// daqui pra frente CreateCharge/CreateGPOQRCode gravam "Failed"
+// diretamente nesse caso — "falhada" só continua a existir como o valor
+// BRUTO de cobranças criadas antes do deploy desta tarefa (o ledger é
+// append-only, imutável), exatamente a mesma situação de
+// aguardando_pagamento/Pending/Requested/solicitada/criada acima, só que
+// para o estado terminal Failed em vez do estado aguardando_pagamento.
+//
+// Qualquer outro valor de filtro (Success, Cancelled, Expired, ou qualquer
+// string não reconhecida, incluindo EstadoPendente) passa inalterado — só
+// "aguardando_pagamento" e "Failed" têm equivalência com valores brutos
+// históricos diferentes de si mesmos.
 func estadosCobrancaEquivalentes(estados []string) []string {
 	out := make([]string, 0, len(estados))
 	for _, estado := range estados {
-		if strings.EqualFold(strings.TrimSpace(estado), EstadoCobrancaAguardandoPagamento) {
+		switch {
+		case strings.EqualFold(strings.TrimSpace(estado), EstadoCobrancaAguardandoPagamento):
 			out = append(out, EstadoCobrancaAguardandoPagamento, "Pending", "Requested", "solicitada", "criada")
-			continue
+		case strings.EqualFold(strings.TrimSpace(estado), "Failed"):
+			out = append(out, "Failed", "falhada")
+		default:
+			out = append(out, estado)
 		}
-		out = append(out, estado)
 	}
 	return out
 }
@@ -941,17 +963,29 @@ func isTerminalChargeStatus(status string) bool {
 }
 
 // normalizeChargeStatus traduz o vocabulário histórico/bruto de status de
-// uma cobrança real para o estado canônico único
-// EstadoCobrancaAguardandoPagamento (mensalidade.go), sempre que o valor de
-// entrada representar "cobrança gerada/tentada junto à AppyPay, ainda sem
-// resolução": os estados locais intermediários que o Spuri gravava antes
-// desta tarefa ("solicitada", gravado antes de qualquer chamada ao
-// provedor; "criada", o fallback usado quando o provedor responde 2xx sem
-// nenhum campo de status) e os estados brutos que a própria AppyPay
-// documenta para esta mesma fase ("Requested" e "Pending" — ver docs/
-// Parceiros e integrações/AppyPay Documentação.md). Qualquer outro valor
-// (Success, Failed, Cancelled, Expired, ou o próprio
-// EstadoCobrancaAguardandoPagamento) é devolvido inalterado — a função é
+// uma cobrança real para os estados canônicos únicos que a API expõe,
+// sempre que o valor de entrada tiver um equivalente canônico:
+//
+//   - EstadoCobrancaAguardandoPagamento (mensalidade.go): os estados locais
+//     intermediários que o Spuri gravava antes da tarefa 66 ("solicitada",
+//     gravado antes de qualquer chamada ao provedor; "criada", o fallback
+//     usado quando o provedor responde 2xx sem nenhum campo de status) e os
+//     estados brutos que a própria AppyPay documenta para esta mesma fase
+//     ("Requested" e "Pending" — ver docs/Parceiros e integrações/AppyPay
+//     Documentação.md).
+//   - "Failed": desde a tarefa 69, também "falhada" — o valor local que
+//     CreateCharge/CreateGPOQRCode gravavam antes desta tarefa quando a
+//     própria chamada HTTP à AppyPay falhava (nunca chegando a existir uma
+//     cobrança do lado do provedor, então a AppyPay nunca teve chance de
+//     devolver "Failed" — ver estadosCobrancaEquivalentes, que resolve o
+//     mesmo problema do lado do filtro SQL). Daqui pra frente
+//     CreateCharge/CreateGPOQRCode já gravam "Failed" diretamente nesse caso;
+//     "falhada" só volta a aparecer como o valor BRUTO de uma cobrança
+//     criada antes do deploy desta tarefa — e mesmo assim nunca chega ao
+//     chamador, porque normalizeChargeStatus a traduz aqui na leitura.
+//
+// Qualquer outro valor (Success, Cancelled, Expired, os dois canônicos
+// acima, ou uma string não reconhecida) é devolvido inalterado — a função é
 // idempotente e pode ser chamada tanto sobre um valor bruto recém-recebido
 // da AppyPay quanto sobre um valor já gravado (histórico ou canônico).
 //
@@ -971,6 +1005,8 @@ func normalizeChargeStatus(raw string) string {
 		strings.EqualFold(trimmed, "solicitada"),
 		strings.EqualFold(trimmed, "criada"):
 		return EstadoCobrancaAguardandoPagamento
+	case strings.EqualFold(trimmed, "falhada"):
+		return "Failed"
 	default:
 		return trimmed
 	}
```

```diff
diff --git a/internal/finance/pagamentos_unificado.go b/internal/finance/pagamentos_unificado.go
index d0b7815..71188f5 100644
--- a/internal/finance/pagamentos_unificado.go
+++ b/internal/finance/pagamentos_unificado.go
@@ -34,7 +34,7 @@ import (
 // continua válida: um mês com cobrança falhada continua contando como
 // pendente. O que esta função resolve é só evitar que ele apareça DUAS
 // vezes na lista final — uma vez como a cobrança real (com seu status
-// verdadeiro, ex. "falhada") e outra vez como uma pendência sintética
+// verdadeiro, ex. "Failed") e outra vez como uma pendência sintética
 // redundante para o mesmo mês.
 func (s *Service) mesesComCobrancaRealVinculada(ctx context.Context, academia string, estudantes []string) (map[string]bool, error) {
 	out := map[string]bool{}
```

---

## 5. Diffs exatos — arquivos de teste (5 arquivos)

Mesma convenção da seção 4: diffs unificados reais, já validados rodando de verdade (seção 9).

```diff
diff --git a/internal/finance/appypay_test.go b/internal/finance/appypay_test.go
index 31af3c1..8ad8193 100644
--- a/internal/finance/appypay_test.go
+++ b/internal/finance/appypay_test.go
@@ -97,13 +97,15 @@ func TestCancelChargeAuthorizationAndTerminalStatuses(t *testing.T) {
 // estados: os valores locais intermediários ("solicitada", "criada") e os
 // valores brutos que a própria AppyPay documenta para "cobrança gerada,
 // ainda sem resolução" ("Requested", "Pending") devem virar o estado
-// canônico único EstadoCobrancaAguardandoPagamento — em qualquer
-// combinação de maiúsculas/minúsculas, já que a AppyPay não garante uma
-// caixa fixa. Qualquer outro valor (terminal ou já canônico) deve passar
-// inalterado — a função é idempotente. Entrada vazia continua vazia: quem
-// decide o fallback é o chamador (CreateCharge/CreateGPOQRCode tratam ""
-// como aguardando_pagamento; consultCharge tem preferido preservar o
-// status anterior).
+// canônico único EstadoCobrancaAguardandoPagamento; e, desde a tarefa 69,
+// "falhada" (o valor local que CreateCharge/CreateGPOQRCode gravavam antes
+// dessa tarefa quando a própria chamada HTTP à AppyPay falhava) deve virar
+// "Failed" — em qualquer combinação de maiúsculas/minúsculas, já que nem a
+// AppyPay nem o código local garantem uma caixa fixa. Qualquer outro valor
+// (terminal ou já canônico) deve passar inalterado — a função é
+// idempotente. Entrada vazia continua vazia: quem decide o fallback é o
+// chamador (CreateCharge/CreateGPOQRCode tratam "" como aguardando_pagamento;
+// consultCharge tem preferido preservar o status anterior).
 func TestNormalizeChargeStatus(t *testing.T) {
 	awaiting := map[string]bool{
 		"Pending": true, "pending": true, "PENDING": true,
@@ -117,7 +119,19 @@ func TestNormalizeChargeStatus(t *testing.T) {
 			t.Fatalf("normalizeChargeStatus(%q) = %q, esperava %q", raw, got, EstadoCobrancaAguardandoPagamento)
 		}
 	}
-	passthrough := []string{"Success", "Failed", "Cancelled", "Expired", "falhada", "cancelada", "algo-desconhecido"}
+	// "falhada" (tarefa 69) — valor local histórico, nunca mais gravado por
+	// CreateCharge/CreateGPOQRCode a partir desta tarefa, mas que ainda
+	// pode aparecer no payload bruto de cobranças criadas antes do deploy
+	// (ledger append-only, imutável) — deve normalizar para "Failed", o
+	// mesmo valor que a AppyPay usa quando o processador recusa a
+	// cobrança, para a API nunca expor os dois nomes distintos a nenhum
+	// chamador.
+	for _, raw := range []string{"falhada", "FALHADA", "Falhada"} {
+		if got := normalizeChargeStatus(raw); got != "Failed" {
+			t.Fatalf("normalizeChargeStatus(%q) = %q, esperava %q", raw, got, "Failed")
+		}
+	}
+	passthrough := []string{"Success", "Failed", "Cancelled", "Expired", "cancelada", "algo-desconhecido"}
 	for _, raw := range passthrough {
 		if got := normalizeChargeStatus(raw); got != raw {
 			t.Fatalf("normalizeChargeStatus(%q) deveria devolver o valor inalterado, obteve %q", raw, got)
@@ -129,7 +143,7 @@ func TestNormalizeChargeStatus(t *testing.T) {
 	// Idempotência: aplicar duas vezes sobre o próprio resultado não muda
 	// nada — importante porque scanCobrancaResumo/loadCharge normalizam
 	// tanto valores brutos históricos quanto valores já canônicos.
-	for _, raw := range append(passthrough, EstadoCobrancaAguardandoPagamento) {
+	for _, raw := range append(passthrough, EstadoCobrancaAguardandoPagamento, "falhada") {
 		once := normalizeChargeStatus(raw)
 		twice := normalizeChargeStatus(once)
 		if once != twice {
@@ -143,7 +157,18 @@ func TestNormalizeChargeStatus(t *testing.T) {
 // equivalentes (ver ListCobrancas/ListCobrancasEstudante) — sem essa
 // expansão, filtrar por esse novo estado canônico não encontraria nenhuma
 // cobrança criada antes desta tarefa (ainda gravada como "Pending",
-// "Requested", "solicitada" ou "criada" no payload do ledger, imutável).
+// "Requested", "solicitada" ou "criada" no payload do ledger, imutável). E,
+// desde a tarefa 69, a expansão irmã de estado=Failed para também incluir
+// "falhada" (o valor local que CreateCharge/CreateGPOQRCode gravavam antes
+// desta tarefa quando a própria chamada HTTP à AppyPay falhava, nunca
+// chegando a existir uma cobrança do lado do provedor) — sem ela, filtrar
+// por Failed nunca encontrava cobranças criadas antes desta tarefa, mesmo
+// elas sendo, do ponto de vista de quem filtra, tão "falhadas" quanto uma
+// recusada pelo processador. Daqui pra frente as duas funções já gravam
+// "Failed" diretamente (ver normalizeChargeStatus, que também traduz
+// "falhada" para "Failed" na leitura) — esta expansão nunca deixa de ser
+// necessária, porque o ledger é append-only e "falhada" continua existindo
+// no payload bruto de cobranças antigas para sempre.
 func TestEstadosCobrancaEquivalentes(t *testing.T) {
 	got := estadosCobrancaEquivalentes([]string{"aguardando_pagamento"})
 	esperado := map[string]bool{"aguardando_pagamento": true, "Pending": true, "Requested": true, "solicitada": true, "criada": true}
@@ -155,20 +180,41 @@ func TestEstadosCobrancaEquivalentes(t *testing.T) {
 			t.Fatalf("valor inesperado na expansão: %q (lista completa: %#v)", v, got)
 		}
 	}
-	// Qualquer outro estado passa inalterado — não tem equivalência
-	// histórica com outros valores brutos.
-	for _, outros := range [][]string{{"Success"}, {"Failed"}, {"Cancelled"}, {"Expired"}, {"pendente"}} {
+	// estado=Failed expande para também casar com "falhada" (tarefa 69).
+	gotFailed := estadosCobrancaEquivalentes([]string{"Failed"})
+	esperadoFailed := map[string]bool{"Failed": true, "falhada": true}
+	if len(gotFailed) != len(esperadoFailed) {
+		t.Fatalf("esperava %d valores equivalentes para Failed, obteve %d: %#v", len(esperadoFailed), len(gotFailed), gotFailed)
+	}
+	for _, v := range gotFailed {
+		if !esperadoFailed[v] {
+			t.Fatalf("valor inesperado na expansão de Failed: %q (lista completa: %#v)", v, gotFailed)
+		}
+	}
+	// O inverso não é verdadeiro: filtrar por "falhada" diretamente
+	// continua estrito, sem casar com "Failed" — só o valor canônico
+	// exposto ao chamador (o que o frontend manda: ver
+	// ESTADO_PAGAMENTO_OPCOES em financeiroShared.tsx, que só tem a opção
+	// "Failed") expande para os valores brutos históricos equivalentes;
+	// "falhada" sozinho só faria sentido numa consulta manual direto no
+	// ledger. Qualquer outro estado também passa inalterado — não tem
+	// equivalência com outros valores brutos.
+	for _, outros := range [][]string{{"Success"}, {"falhada"}, {"Cancelled"}, {"Expired"}, {"pendente"}} {
 		out := estadosCobrancaEquivalentes(outros)
 		if len(out) != 1 || out[0] != outros[0] {
 			t.Fatalf("esperava %v inalterado, obteve %v", outros, out)
 		}
 	}
 	// Uma lista com múltiplos estados só expande o que casa com
-	// aguardando_pagamento, preservando os demais.
+	// aguardando_pagamento ou Failed, preservando os demais.
 	misto := estadosCobrancaEquivalentes([]string{"Success", "aguardando_pagamento"})
 	if len(misto) != 6 {
 		t.Fatalf("esperava 6 valores (1 Success + 5 da expansão), obteve %d: %#v", len(misto), misto)
 	}
+	mistoFailed := estadosCobrancaEquivalentes([]string{"Cancelled", "Failed"})
+	if len(mistoFailed) != 3 {
+		t.Fatalf("esperava 3 valores (1 Cancelled + 2 da expansão de Failed), obteve %d: %#v", len(mistoFailed), mistoFailed)
+	}
 }
 
 func TestEncryptionRoundTripAndNoFallbackKey(t *testing.T) {
```

```diff
diff --git a/internal/finance/appypay_integration_test.go b/internal/finance/appypay_integration_test.go
index 6fa5873..dce5ea2 100644
--- a/internal/finance/appypay_integration_test.go
+++ b/internal/finance/appypay_integration_test.go
@@ -271,6 +271,99 @@ func TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso(t *testing.T) {
 	}
 }
 
+// failingProviderTransport simula uma falha na própria chamada HTTP à
+// AppyPay (timeout, DNS, conexão recusada — qualquer erro que nunca chega
+// a produzir uma resposta do provedor), distinta de um 2xx com
+// status="Failed" no corpo (essa a AppyPay processou; esta aqui o Spuri
+// nem conseguiu enviar). Usado por
+// TestIntegrationCreateChargeECreateGPOQRCodeFalhaLocalGravaFailed (tarefa
+// 69) para provar que CreateCharge/CreateGPOQRCode gravam e devolvem
+// "Failed" (não mais "falhada") quando isso acontece.
+type failingProviderTransport struct{}
+
+func (t *failingProviderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
+	if strings.Contains(req.URL.Path, "/oauth2/token") {
+		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"test-token","expires_in":3600}`)), Request: req}, nil
+	}
+	return nil, errors.New("conexão recusada (simulado)")
+}
+
+// TestIntegrationCreateChargeECreateGPOQRCodeFalhaLocalGravaFailed
+// reproduz a causa raiz do bug relatado por Fredy (tarefa 69): antes desta
+// tarefa, quando a própria chamada HTTP à AppyPay falhava (sem chegar a
+// existir uma cobrança do lado do provedor), CreateCharge e CreateGPOQRCode
+// gravavam o valor local "falhada" — diferente do valor "Failed" que a
+// AppyPay usa quando o PROCESSADOR recusa a cobrança — e filtrar
+// estado=Failed nunca encontrava essas cobranças (ver
+// TestEstadosCobrancaEquivalentes em appypay_test.go, e os testes de
+// integração em internal/handlers/financeiro_cobrancas_handlers_test.go e
+// financeiro_cobrancas_estudante_handlers_test.go, que cobrem o mesmo
+// cenário pelo lado HTTP). Daqui pra frente as duas funções gravam
+// "Failed" diretamente: este teste confirma isso tanto no valor devolvido
+// por CreateCharge quanto no payload persistido por ambas — CreateGPOQRCode
+// não devolve o status no result de erro (só gerarCobranca/o chamador HTTP
+// consomem esse retorno), então o caminho do QR code só é verificável via
+// o payload persistido, por isso o MerchantTransactionID é fixado
+// explicitamente (para poder localizar a linha depois, já que o ID gerado
+// internamente não é devolvido em caso de erro).
+func TestIntegrationCreateChargeECreateGPOQRCodeFalhaLocalGravaFailed(t *testing.T) {
+	client := integrationClient(t)
+	t.Setenv("ENV", "test")
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
+	service := NewService(client)
+	service.SetHTTPClient(&http.Client{Transport: &failingProviderTransport{}})
+	academia := "FAL" + uuid.NewString()[:8]
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+
+	chargeResult, err := service.CreateCharge(context.Background(), ChargeRequest{
+		ContextoTipo: ContextoAcademia, CodigoAcademia: academia,
+		Amount: 500, Currency: "AOA", Description: "teste falha local", PaymentMethod: "REF",
+	}, "actor", "academia", "127.0.0.1")
+	if err == nil {
+		t.Fatal("esperava erro na chamada com transporte falho")
+	}
+	if chargeResult.Status != "Failed" {
+		t.Fatalf("CreateCharge com falha local devolveu Status=%q, queria \"Failed\"", chargeResult.Status)
+	}
+	var persistedCharge string
+	if err = client.DB().QueryRow(`SELECT payload->>'status' FROM financeiro_cobrancas WHERE id=$1`, chargeResult.ID).Scan(&persistedCharge); err != nil {
+		t.Fatal(err)
+	}
+	if persistedCharge != "Failed" {
+		t.Fatalf("payload persistido de CreateCharge com falha local = %q, queria \"Failed\"", persistedCharge)
+	}
+
+	qrMerchant := integrationMerchant("QRFAL")
+	_, err = service.CreateGPOQRCode(context.Background(), QRCodeRequest{
+		ContextoTipo: ContextoAcademia, CodigoAcademia: academia,
+		Amount: 500, Currency: "AOA", Description: "teste falha local QR",
+		MerchantTransactionID: qrMerchant,
+	}, "actor", "academia", "127.0.0.1")
+	if err == nil {
+		t.Fatal("esperava erro na chamada de QR code com transporte falho")
+	}
+	var persistedQR string
+	if err = client.DB().QueryRow(`SELECT payload->>'status' FROM financeiro_cobrancas WHERE merchant_transaction_id=$1`, qrMerchant).Scan(&persistedQR); err != nil {
+		t.Fatal(err)
+	}
+	if persistedQR != "Failed" {
+		t.Fatalf("payload persistido de CreateGPOQRCode com falha local = %q, queria \"Failed\"", persistedQR)
+	}
+
+	// Consequência prática: as duas já aparecem sob estado=Failed sem
+	// precisar da equivalência com "falhada" — a expansão em
+	// estadosCobrancaEquivalentes só é necessária para cobranças criadas
+	// antes desta tarefa.
+	result, err := service.ListCobrancas(context.Background(), ContextoAcademia, academia, []string{"Failed"}, nil, nil, nil, "", "", nil, 30, 0)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if result.Total != 2 {
+		t.Fatalf("ListCobrancas estado=Failed = %d cobranças, queria 2", result.Total)
+	}
+}
+
 func TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation(t *testing.T) {
 	client := integrationClient(t)
 	service := NewService(client)
```

```diff
diff --git a/internal/finance/pagamentos_unificado_integration_test.go b/internal/finance/pagamentos_unificado_integration_test.go
index c243ec0..86f06c8 100644
--- a/internal/finance/pagamentos_unificado_integration_test.go
+++ b/internal/finance/pagamentos_unificado_integration_test.go
@@ -176,8 +176,13 @@ func TestIntegrationListarCobrancasHandlerFluxoUnificado(t *testing.T) {
 			if p.CodigoEstudante != "ESTFLXFL" {
 				t.Fatalf("única cobrança real esperada era de ESTFLXFL, veio de %s", p.CodigoEstudante)
 			}
-			if p.Status != "falhada" {
-				t.Fatalf("esperava status=falhada na cobrança real, obteve %q", p.Status)
+			// Desde a tarefa 69, o valor bruto histórico "falhada" (usado
+			// aqui só como fixture — simula uma cobrança criada antes
+			// dessa tarefa) normaliza para "Failed" na leitura, junto com
+			// o valor que a própria AppyPay usa — ver
+			// normalizeChargeStatus em appypay.go.
+			if p.Status != "Failed" {
+				t.Fatalf("esperava status=Failed (falhada normalizado) na cobrança real, obteve %q", p.Status)
 			}
 			if p.AtualizadoEm == nil {
 				t.Fatal("cobrança real deveria ter AtualizadoEm preenchido")
```

```diff
diff --git a/internal/handlers/financeiro_cobrancas_estudante_handlers_test.go b/internal/handlers/financeiro_cobrancas_estudante_handlers_test.go
index d80e5ff..9430906 100644
--- a/internal/handlers/financeiro_cobrancas_estudante_handlers_test.go
+++ b/internal/handlers/financeiro_cobrancas_estudante_handlers_test.go
@@ -79,6 +79,91 @@ func TestIntegrationConsultarCobrancasEstudanteEstudanteVeTodosOsEstados(t *test
 	}
 }
 
+// TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal
+// reproduz, no nível HTTP, o bug relatado por Fredy: GET
+// /financeiro/cobrancas/estudante/:codigo?estado=Failed devolvia uma lista
+// vazia mesmo o estudante tendo cobranças reais falhadas — porque essas
+// cobranças foram gravadas com o valor local "falhada" (a própria chamada
+// HTTP à AppyPay falhou, nunca chegando a existir cobrança do lado do
+// provedor), e o filtro SQL antes desta tarefa só reconhecia o valor
+// "Failed" (recusa do processador). As duas são "falhas" do ponto de vista
+// de quem consulta — ver estadosCobrancaEquivalentes (tarefa 69). A linha
+// inserida com "falhada" simula uma cobrança criada antes do deploy desta
+// tarefa (ledger imutável); CreateCharge/CreateGPOQRCode já não gravam
+// mais esse valor daqui pra frente (ver
+// TestIntegrationCreateChargeECreateGPOQRCodeFalhaLocalGravaFailed em
+// appypay_integration_test.go), mas a linha antiga tem que continuar
+// aparecendo — e aparecendo já como "Failed", nunca como "falhada", já
+// que normalizeChargeStatus normaliza isso na leitura. Reproduzido também
+// para academia/admin em
+// TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal
+// (financeiro_cobrancas_handlers_test.go), já que os dois handlers
+// compartilham a mesma função de expansão do filtro.
+func TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal(t *testing.T) {
+	gin.SetMode(gin.TestMode)
+	client := integrationFinanceClient(t)
+	academia := "COBEST" + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
+	codigoEstudante := "ESTCOB4"
+	estudanteID := seedEstudanteParaCobrancas(t, client, codigoEstudante, academia)
+
+	insert := func(status string) {
+		payload := map[string]any{"status": status, "amount": 300.0, "currency": "AOA", "description": "teste", "payment_method": "GPO", "codigo_estudante": codigoEstudante}
+		raw, err := json.Marshal(payload)
+		if err != nil {
+			t.Fatal(err)
+		}
+		merchant := "COB" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
+		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
+			uuid.New(), merchant, academia, raw); err != nil {
+			t.Fatal(err)
+		}
+	}
+	// "falhada": valor bruto histórico — simula uma cobrança criada antes
+	// da tarefa 69 (a própria chamada HTTP à AppyPay falhou na época).
+	// "Failed": a AppyPay processou e o processador recusou (também o que
+	// CreateCharge/CreateGPOQRCode gravam diretamente para o caso acima,
+	// daqui pra frente).
+	// "Success": controle — nunca deve aparecer no filtro estado=Failed.
+	insert("falhada")
+	insert("Failed")
+	insert("Success")
+
+	previousService := FinanceiroService
+	FinanceiroService = finance.NewService(client)
+	t.Cleanup(func() { FinanceiroService = previousService })
+
+	recorder := httptest.NewRecorder()
+	ctx, _ := gin.CreateTestContext(recorder)
+	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+codigoEstudante+"?estado=Failed&limit=30&offset=0", nil)
+	ctx.Params = gin.Params{{Key: "codigo", Value: codigoEstudante}}
+	ctx.Set("dbClient", client)
+	ctx.Set("user_id", estudanteID)
+	ctx.Set("user_type", "estudante")
+
+	ConsultarCobrancasEstudante(ctx)
+	if recorder.Code != http.StatusOK {
+		t.Fatalf("estudante filtrando estado=Failed = %d: %s", recorder.Code, recorder.Body.String())
+	}
+	var bodyFailed struct {
+		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
+		TotalGeral int                       `json:"total_geral"`
+	}
+	if err := json.Unmarshal(recorder.Body.Bytes(), &bodyFailed); err != nil {
+		t.Fatal(err)
+	}
+	if bodyFailed.TotalGeral != 2 {
+		t.Fatalf("estado=Failed deveria trazer as 2 cobranças falhadas (gravada como \"Failed\" e a histórica gravada como \"falhada\"), obteve %d: %s", bodyFailed.TotalGeral, recorder.Body.String())
+	}
+	for _, p := range bodyFailed.Pagamentos {
+		// normalizeChargeStatus traduz "falhada" para "Failed" na leitura
+		// — nenhum chamador deveria ver "falhada" na resposta, nem a
+		// linha histórica inserida diretamente acima.
+		if p.Status != "Failed" {
+			t.Fatalf("esperava só status=\"Failed\" no resultado (normalizado), obteve %q: %s", p.Status, recorder.Body.String())
+		}
+	}
+}
+
 // TestIntegrationConsultarCobrancasEstudanteRejeitaOutroEstudante garante
 // que um estudante não consegue consultar o histórico de outro, mesmo
 // sabendo o código dele.
```

```diff
diff --git a/internal/handlers/financeiro_cobrancas_handlers_test.go b/internal/handlers/financeiro_cobrancas_handlers_test.go
index e59e7a8..90129ff 100644
--- a/internal/handlers/financeiro_cobrancas_handlers_test.go
+++ b/internal/handlers/financeiro_cobrancas_handlers_test.go
@@ -123,6 +123,84 @@ func TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado(t *testing.T) {
 	}
 }
 
+// TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal
+// reproduz, no nível HTTP, a mesma causa raiz de
+// TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal
+// (financeiro_cobrancas_estudante_handlers_test.go) mas pelo lado de
+// academia/admin: Fredy relatou que o mesmo erro (estado=Failed devolvendo
+// vazio mesmo havendo cobranças falhadas) também acontece em GET
+// /financeiro/cobrancas — ver estadosCobrancaEquivalentes (tarefa 69).
+// TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal
+// reproduz, no nível HTTP, a mesma causa raiz de
+// TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal
+// (financeiro_cobrancas_estudante_handlers_test.go) mas pelo lado de
+// academia/admin: Fredy relatou que o mesmo erro (estado=Failed devolvendo
+// vazio mesmo havendo cobranças falhadas) também acontece em GET
+// /financeiro/cobrancas — ver estadosCobrancaEquivalentes (tarefa 69). A
+// linha inserida com "falhada" simula uma cobrança criada antes do deploy
+// desta tarefa (ledger imutável) — CreateCharge/CreateGPOQRCode já não
+// gravam mais esse valor daqui pra frente.
+func TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal(t *testing.T) {
+	gin.SetMode(gin.TestMode)
+	client := integrationFinanceClient(t)
+	academia := "LSTF" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
+
+	insert := func(status string) {
+		payload := map[string]any{"status": status, "amount": 250.0, "currency": "AOA", "description": "teste", "payment_method": "GPO"}
+		raw, err := json.Marshal(payload)
+		if err != nil {
+			t.Fatal(err)
+		}
+		merchant := "LSF" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
+		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
+			uuid.New(), merchant, academia, raw); err != nil {
+			t.Fatal(err)
+		}
+	}
+	// "falhada": valor bruto histórico — simula uma cobrança criada antes
+	// da tarefa 69. "Failed": o que a AppyPay grava quando o processador
+	// recusa (também o que CreateCharge/CreateGPOQRCode gravam
+	// diretamente para o caso acima, daqui pra frente). "Success":
+	// controle — nunca deve aparecer no filtro estado=Failed.
+	insert("falhada")
+	insert("Failed")
+	insert("Success")
+
+	previousService := FinanceiroService
+	FinanceiroService = finance.NewService(client)
+	t.Cleanup(func() { FinanceiroService = previousService })
+
+	recorder := httptest.NewRecorder()
+	ctx, _ := gin.CreateTestContext(recorder)
+	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?estado=Failed", nil)
+	ctx.Set("dbClient", client)
+	ctx.Set("user_id", uuid.New())
+	ctx.Set("user_type", "academia")
+	ctx.Set("codigo_academia", academia)
+	ListarCobrancasAppyPay(ctx)
+
+	if recorder.Code != http.StatusOK {
+		t.Fatalf("academia filtrando estado=Failed = %d: %s", recorder.Code, recorder.Body.String())
+	}
+	var bodyFailed struct {
+		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
+		TotalGeral int                       `json:"total_geral"`
+	}
+	if err := json.Unmarshal(recorder.Body.Bytes(), &bodyFailed); err != nil {
+		t.Fatal(err)
+	}
+	if bodyFailed.TotalGeral != 2 {
+		t.Fatalf("estado=Failed deveria trazer as 2 cobranças falhadas (gravada como \"Failed\" e a histórica gravada como \"falhada\"), obteve %d: %s", bodyFailed.TotalGeral, recorder.Body.String())
+	}
+	for _, p := range bodyFailed.Pagamentos {
+		// normalizeChargeStatus traduz "falhada" para "Failed" na leitura
+		// — nenhum chamador deveria ver "falhada" na resposta.
+		if p.Status != "Failed" {
+			t.Fatalf("esperava só status=\"Failed\" no resultado (normalizado), obteve %q: %s", p.Status, recorder.Body.String())
+		}
+	}
+}
+
 // TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP garante
 // que a nova rota usa a mesma autorização das demais rotas de /financeiro:
 // um admin sem a permissão "fpp" não pode listar cobranças.
```

---

## 6. Diffs exatos — `Documentação da API.md`

```diff
diff --git "a/Documenta\303\247\303\243o da API.md" "b/Documenta\303\247\303\243o da API.md"
index 694ebbe..0786143 100644
--- "a/Documenta\303\247\303\243o da API.md"	
+++ "b/Documenta\303\247\303\243o da API.md"	
@@ -7602,7 +7602,7 @@ Neste caso, `currency` assume `AOA` e a AppyPay gera os dados de referência seg
 - `amount` deve ser positivo e ter no máximo duas casas decimais; `currency`, `description` e `paymentMethod` devem ser coerentes com o método configurado na credencial.
 - `merchantTransactionId` é a referência externa recomendada para idempotência e posterior consulta.
 - O mesmo `merchantTransactionId` devolve o resultado já persistido e não cria uma nova cobrança. Enquanto a primeira requisição ainda estiver sendo processada, a repetição recebe `409` e pode ser tentada novamente.
-- A cobrança é registrada no ledger como solicitação e, conforme resposta da AppyPay, como criada ou falhada.
+- A cobrança é registrada no ledger como solicitação e, conforme resposta da AppyPay, como criada ou `Failed` (se a própria chamada HTTP à AppyPay falhar, sem chegar a existir cobrança do lado do provedor).
 
 #### 19.5 POST /financeiro/appypay/qr-codes
 
@@ -7742,7 +7742,7 @@ Para `MULTIPLE`, os quatro campos adicionais são obrigatórios. Seus valores e
 |---|---|---|---|
 | `contexto_tipo` | string | Não | Contexto financeiro consultado. Para academia autenticada é forçado para `academia`. |
 | `codigo_academia` | string | Não | Academia dona das cobranças. Para academia autenticada é forçado para o código do token. |
-| `estado` | string, repetível | Não | Filtra pelo estado de uma cobrança real. Aceita o texto exato persistido em `status` — estados internos (`cancelada`, `falhada`) e estados crus da AppyPay (`Success`, `Failed`, `Cancelled`, `Expired`) — mais o estado canônico `aguardando_pagamento` (cobrança gerada/tentada, ainda sem resolução do provedor; casa também com cobranças antigas gravadas antes desta forma existir). Repita o parâmetro para casar mais de um valor (`?estado=Success&estado=aguardando_pagamento`). Também filtra os itens sintéticos: como toda pendência sintética tem sempre `status: "pendente"`, um filtro de `estado` que não inclua `"pendente"` exclui as pendências do resultado (ver regras de negócio). |
+| `estado` | string, repetível | Não | Filtra pelo estado de uma cobrança real. Aceita o texto exato persistido em `status` — estados internos (`cancelada`) e estados crus da AppyPay (`Success`, `Failed`, `Cancelled`, `Expired`) — mais dois estados canônicos com equivalência histórica: `aguardando_pagamento` (cobrança gerada/tentada, ainda sem resolução do provedor; casa também com cobranças antigas gravadas antes desta forma existir) e `Failed` (casa também com `falhada`, o valor bruto que cobranças antigas usavam quando a própria chamada HTTP à AppyPay falhava, sem chegar a existir cobrança do lado do provedor — daqui pra frente gravado já como `Failed`; ver 19.4). Repita o parâmetro para casar mais de um valor (`?estado=Success&estado=aguardando_pagamento`). Também filtra os itens sintéticos: como toda pendência sintética tem sempre `status: "pendente"`, um filtro de `estado` que não inclua `"pendente"` exclui as pendências do resultado (ver regras de negócio). |
 | `tipo` | string, repetível | Não | Filtra por origem: `matricula`, `mensalidade` ou `avulsa`. Mesma lógica de `estado`: como toda pendência sintética é sempre `origem: "mensalidade"`, um filtro de `tipo` que não inclua `"mensalidade"` exclui as pendências do resultado. |
 | `turma_id` | UUID | Não | Restringe a pagamentos de mensalidade vinculados a esta turma. Só afeta cobranças reais de origem `mensalidade` — ver regras de negócio. |
 | `curso_id` | UUID | Não | Restringe a pagamentos de mensalidade vinculados a este curso. Mesma ressalva de `turma_id`. |
@@ -7814,9 +7814,9 @@ Para `MULTIPLE`, os quatro campos adicionais são obrigatórios. Seus valores e
 - Cada item de `pagamentos` tem o mesmo formato, dos dois tipos possíveis — `status` sozinho já diz qual é:
   - Qualquer `status` diferente de `"pendente"` (incluindo `"aguardando_pagamento"`) — um pagamento real, com todos os campos vindos de uma cobrança de fato criada (`id` real, `atualizado_em` presente, e opcionalmente `provider_charge_id`/`merchant_transaction_id`/`metodo_pagamento` quando fizer sentido). Não devolve `payment_info`, `response` (resposta crua da AppyPay) nem `qrCodeArr`; para o detalhe completo de uma cobrança específica, use 19.6.
   - `status: "pendente"` — um mês de mensalidade que ainda não foi pago (nem anulado) e não tem **nenhuma** cobrança real vinculada (nem sequer uma tentativa falhada) — sintetizado a partir da mesma computação de 19.17 (`MensalidadeMesView`). `id` é determinístico (hash estável de academia+estudante+ano_letivo+mês — a mesma pendência sempre tem o mesmo `id` entre chamadas, útil como key de lista no cliente), `atualizado_em` é sempre ausente (não existe nenhuma atividade real para reportar), e `metodo_pagamento`/`provider_charge_id`/`merchant_transaction_id` também ficam ausentes.
-- **`status` sozinho já diz se existe uma cobrança real por trás do item** — `"pendente"` é exclusivo das pendências sintéticas; uma cobrança real nunca usa esse valor. Assim que uma cobrança é gerada/tentada (mesmo antes de qualquer resposta do provedor), seu status passa a ser `"aguardando_pagamento"` — o estado canônico que substitui os antigos `"solicitada"`/`"criada"` (gravados localmente antes de qualquer resposta da AppyPay) e os estados crus `"Requested"`/`"Pending"` que a própria AppyPay documenta para essa mesma fase (cobrança gerada, ainda sem resolução). Cobranças criadas antes desta forma existir, se ainda não resolvidas, continuam sendo lidas e filtradas como `"aguardando_pagamento"` também (equivalência histórica, não é preciso reprocessar nada).
-- Estados terminais de uma cobrança real (não mudam mais sozinhos): `"Success"` (paga), `"Failed"` (recusada no processador da AppyPay), `"Cancelled"` (cancelada do lado da AppyPay), `"Expired"` (referência REF expirada sem pagamento), `"falhada"` (a chamada à AppyPay falhou, sem chegar a existir cobrança do lado do provedor) e `"cancelada"` (cancelamento feito pelo Spuri, 19.9).
-- Um mês com uma cobrança real **falhada** aparece como item real (`status: "falhada"` ou `"Failed"`) — **não** gera também um item sintético duplicado para o mesmo mês, mesmo continuando a valer como "ainda não pago" internamente (ver 19.17 e a tarefa que corrigiu esse critério). A cobrança real, com seu histórico verdadeiro, já é a representação desse mês na lista.
+- **`status` sozinho já diz se existe uma cobrança real por trás do item** — `"pendente"` é exclusivo das pendências sintéticas; uma cobrança real nunca usa esse valor. Assim que uma cobrança é gerada/tentada (mesmo antes de qualquer resposta do provedor), seu status passa a ser `"aguardando_pagamento"` — o estado canônico que substitui os antigos `"solicitada"`/`"criada"` (gravados localmente antes de qualquer resposta da AppyPay) e os estados crus `"Requested"`/`"Pending"` que a própria AppyPay documenta para essa mesma fase (cobrança gerada, ainda sem resolução). Cobranças criadas antes desta forma existir, se ainda não resolvidas, continuam sendo lidas e filtradas como `"aguardando_pagamento"` também (equivalência histórica, não é preciso reprocessar nada). Mesmo raciocínio para `"Failed"`: cobranças criadas antes de existir esse valor único, quando a própria chamada HTTP à AppyPay falhava (sem chegar a existir cobrança do lado do provedor), foram gravadas com o valor local `"falhada"` — continuam sendo lidas e filtradas como `"Failed"` (idem, equivalência histórica).
+- Estados terminais de uma cobrança real (não mudam mais sozinhos): `"Success"` (paga), `"Failed"` (recusada no processador da AppyPay, ou — para cobranças antigas — a própria chamada HTTP à AppyPay falhou sem chegar a existir cobrança do lado do provedor), `"Cancelled"` (cancelada do lado da AppyPay), `"Expired"` (referência REF expirada sem pagamento) e `"cancelada"` (cancelamento feito pelo Spuri, 19.9).
+- Um mês com uma cobrança real **falhada** aparece como item real (`status: "Failed"`) — **não** gera também um item sintético duplicado para o mesmo mês, mesmo continuando a valer como "ainda não pago" internamente (ver 19.17 e a tarefa que corrigiu esse critério). A cobrança real, com seu histórico verdadeiro, já é a representação desse mês na lista.
 - `origem` é derivada do payload persistido para itens reais, nunca gravada separadamente: `matricula` quando a cobrança tem `codigo_solicitacao`, `mensalidade` quando tem `codigo_estudante` (e não tem `codigo_solicitacao`), `avulsa` nos demais casos. Itens sintéticos são sempre `origem: "mensalidade"`.
 - **Ordenação:** itens sintéticos (`status: "pendente"`) sempre vêm primeiro — representam ação pendente ("isto ainda precisa de uma cobrança"). Depois vêm os itens reais, por `updated_at DESC` (atividade mais recente primeiro). A paginação (`limit`/`offset`) percorre essa ordem combinada como uma lista única.
 - `total` é o número de itens nesta página; `total_geral` é o total real (pendências sintéticas + cobranças reais) que casa com os filtros aplicados.
@@ -7833,7 +7833,7 @@ Para `MULTIPLE`, os quatro campos adicionais são obrigatórios. Seus valores e
 
 | Campo | Tipo | Obrigatório | Descrição |
 |---|---|---|---|
-| `estado` | string, repetível | Não | Mesmo filtro de 19.7 (inclusive a equivalência histórica de `aguardando_pagamento` e o efeito sobre itens sintéticos). Sem filtro, devolve todos os estados. |
+| `estado` | string, repetível | Não | Mesmo filtro de 19.7 (inclusive a equivalência histórica de `aguardando_pagamento` e de `Failed`, e o efeito sobre itens sintéticos). Sem filtro, devolve todos os estados. |
 | `tipo` | string, repetível | Não | Mesmo filtro de 19.7: `matricula`, `mensalidade` ou `avulsa`. |
 | `turma_id` | UUID | Não | Mesmo filtro de 19.7. Só tem efeito quando quem consulta é a academia (isto é, quando o contexto de uma única academia já está resolvido) — ver regras de negócio. |
 | `curso_id` | UUID | Não | Mesma ressalva de `turma_id`. |
```

---

## 7. Fora de escopo (não altere)

Lugares que foram checados especificamente e **não** precisam mudar, para que Codex não amplie o escopo por conta própria:

- **`isTerminalChargeStatus`** (`internal/finance/appypay.go`) — já reconhece `"falhada"` (junto com `"Failed"`) como estado terminal. Continua reconhecendo os dois: precisa, porque cobranças antigas gravadas como `"falhada"` (ledger imutável) continuam existindo e continuam sendo terminais. Não remover `"falhada"` dessa lista.
- **`chargeAbertaStatusExcluidos`** (`internal/finance/mensalidade.go`, criada na tarefa 66) — mesma razão: já inclui `'falhada'` na lista de estados que tiram uma mensalidade/matrícula do estado "em aberto". Continua precisando, pelo mesmo motivo (cobranças antigas).
- **`internal/finance/mensalidade_pendencias_integration_test.go`** (linhas 84 e 223, `seedFinanceiroCobrancaMensalidade(..., "falhada", ...)`) — usa `"falhada"` só como fixture genérica para exercitar a lógica de contagem de pendências (`PendenciasSemCobranca`), não afirma nada sobre o valor de `status` na resposta da API. Não depende de `normalizeChargeStatus` nem de `estadosCobrancaEquivalentes`. Não precisa de nenhuma alteração — já foi checado rodando a suíte inteira (seção 9) e continua verde sem tocar neste arquivo.
- **`internal/finance/cobranca_geracao.go`** (`gerarCobranca`, criada na tarefa 67) — chama `CreateCharge`/`CreateGPOQRCode` e repassa o erro/resultado delas diretamente, sem gravar nenhum evento próprio nem inspecionar o campo `status`. A correção desta tarefa (nos dois pontos de escrita dentro de `CreateCharge`/`CreateGPOQRCode`) já cobre totalmente o caminho unificado de `gerarCobranca` — não precisa de nenhuma mudança nesse arquivo.
- **Frontend (`spuripainel`)** — `ESTADO_PAGAMENTO_OPCOES` já expõe só `"Failed"` como opção (nunca `"falhada"`), desde antes desta tarefa. Nenhuma mudança de frontend é necessária por causa desta tarefa backend (a mudança de frontend relacionada é a tarefa 68, sobre o filtro `"pendente"" — ver seção 0.1, assunto completamente diferente).
- **`docs/Debbugs/`** — nenhum documento nessa pasta menciona `"falhada"`/`estadosCobrancaEquivalentes`; não há nada para atualizar lá.

---

## 8. Checklist de validação (Codex deve executar e reportar o resultado de cada item)

Nenhum destes comandos requer PostgreSQL, Docker ou `psql`:

1. `grep -n '"falhada"' internal/finance/appypay.go` — deve aparecer só dentro de comentários e dentro do `case` de `normalizeChargeStatus`/`estadosCobrancaEquivalentes` (a string literal usada para RECONHECER o valor histórico) — **nunca** mais como o valor sendo gravado nos dois `s.record(...)` de `CreateCharge`/`CreateGPOQRCode` (que devem mostrar `"Failed"`).
2. `grep -n 'case strings.EqualFold(strings.TrimSpace(estado), "Failed")' internal/finance/appypay.go` — deve aparecer exatamente uma vez, dentro de `estadosCobrancaEquivalentes`.
3. `grep -n 'case strings.EqualFold(trimmed, "falhada")' internal/finance/appypay.go` — deve aparecer exatamente uma vez, dentro de `normalizeChargeStatus`.
4. `go build ./...` — sem erros.
5. `go vet ./...` — sem erros.
6. `gofmt -l internal/finance/appypay.go internal/finance/pagamentos_unificado.go internal/finance/appypay_test.go internal/finance/appypay_integration_test.go internal/finance/pagamentos_unificado_integration_test.go internal/handlers/financeiro_cobrancas_handlers_test.go internal/handlers/financeiro_cobrancas_estudante_handlers_test.go` — vazio (nenhum arquivo mal formatado).
7. `go test ./...` — sem falhas (os testes de integração aparecem como `SKIP` sem `RUN_POSTGRES_INTEGRATION`, isso é esperado — Claude já validou o comportamento de tempo de execução completo desses testes com PostgreSQL real, ver seção 9).
8. `git diff --stat` — alterações apenas nos 2 arquivos de código da seção 4, nos 5 arquivos de teste da seção 5, e em `Documentação da API.md`, mais os documentos de conclusão desta tarefa.

Se qualquer item falhar, não prossiga — reporte o erro exato.

---

## 9. Evidência de validação (já executada por Claude — PostgreSQL 16 + Go 1.24 reais)

Claude validou esta tarefa inteira, do zero, num sandbox com PostgreSQL 16.15 e Go 1.24.4 reais instalados (`golang-1.24-go` via `apt`) — não apenas lida a partir do código-fonte. Nota de reprodutibilidade: como o módulo proxy padrão do Go (`proxy.golang.org`) não estava acessível nesse sandbox específico, as dependências foram baixadas com `GOPROXY=direct` (resolução direta via VCS/GitHub) usando um bloco `replace` temporário no `go.mod` só para os módulos de import-path "vanity" (`golang.org/x/*`, `google.golang.org/protobuf`, `gopkg.in/yaml.v3`) — **esse bloco nunca fez parte do diff entregue** (seção 4), foi removido antes de gerar os diffs finais; a cópia usada para gerar os diffs da seção 4 nunca teve esse bloco.

**Build, vet e gofmt, banco recriado do zero:**
```
$ go build ./...
BUILD_OK
$ go vet ./...
VET_OK
$ gofmt -l internal/finance/appypay.go internal/finance/pagamentos_unificado.go internal/finance/appypay_test.go internal/finance/appypay_integration_test.go internal/finance/pagamentos_unificado_integration_test.go internal/handlers/financeiro_cobrancas_handlers_test.go internal/handlers/financeiro_cobrancas_estudante_handlers_test.go
(vazio)
```

**Suíte completa, banco recriado do zero, `-count=1`:**
```
$ go test ./... -count=1
ok  	spuri/cmd/server	0.018s
ok  	spuri/internal/db	0.010s
ok  	spuri/internal/domain/aggregates	0.011s
ok  	spuri/internal/finance	2.464s
ok  	spuri/internal/handlers	0.844s
ok  	spuri/internal/middleware	0.006s
ok  	spuri/internal/projections	0.007s
ok  	spuri/internal/services	0.008s
ok  	spuri/internal/storage	0.005s
ok  	spuri/internal/utils	0.007s
```

**Mesma suíte, `-race`, banco recriado do zero (só os dois pacotes tocados):**
```
$ go test ./internal/finance/... ./internal/handlers/... -count=1 -race
ok  	spuri/internal/finance	4.847s
ok  	spuri/internal/handlers	5.567s
```

**Prova de que o teste de regressão realmente pega o bug relatado por Fredy** — Claude reverteu deliberadamente só o `case` novo de `"Failed"` dentro de `estadosCobrancaEquivalentes` (voltando ao comportamento de antes desta tarefa — sem tocar na escrita nem em `normalizeChargeStatus`) e rodou os dois testes de regressão HTTP:
```
$ go test ./internal/handlers/... -run 'TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal|TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal' -v -count=1
    financeiro_cobrancas_estudante_handlers_test.go:155: estado=Failed deveria trazer as 2 cobranças falhadas (gravada como "Failed" e a histórica gravada como "falhada"), obteve 1: {...,"pagamentos":[{...,"status":"Failed",...}],"total":1,"total_geral":1}
--- FAIL: TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal (0.43s)
    financeiro_cobrancas_handlers_test.go:193: estado=Failed deveria trazer as 2 cobranças falhadas (gravada como "Failed" e a histórica gravada como "falhada"), obteve 1: {...,"pagamentos":[{...,"status":"Failed",...}],"total":1,"total_geral":1}
--- FAIL: TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal (0.06s)
FAIL
```
A cobrança gravada com o valor histórico `"falhada"` some do resultado (`total: 1` em vez de `2`) — exatamente o sintoma relatado por Fredy: filtrar por `estado=Failed` não traz cobranças que realmente falharam. Depois de restaurar a correção:
```
$ go test ./internal/handlers/... -run 'TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal|TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal' -v -count=1
--- PASS: TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal (0.42s)
--- PASS: TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal (0.03s)
ok  	spuri/internal/handlers	0.467s
```

**Os 6 testes novos/alterados desta tarefa, individualmente, todos verdes:**
```
$ go test ./internal/finance/... ./internal/handlers/... -run 'TestNormalizeChargeStatus|TestEstadosCobrancaEquivalentes|TestIntegrationCreateChargeECreateGPOQRCodeFalhaLocalGravaFailed|TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal|TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal|TestIntegrationListarCobrancasHandlerFluxoUnificado' -v -count=1
--- PASS: TestIntegrationCreateChargeECreateGPOQRCodeFalhaLocalGravaFailed (0.43s)
--- PASS: TestNormalizeChargeStatus (0.00s)
--- PASS: TestEstadosCobrancaEquivalentes (0.00s)
--- PASS: TestIntegrationListarCobrancasHandlerFluxoUnificado (0.03s)
ok  	spuri/internal/finance	0.478s
--- PASS: TestIntegrationConsultarCobrancasEstudanteFiltroEstadoFailedIncluiFalhadaLocal (0.02s)
--- PASS: TestIntegrationListarCobrancasAppyPayFiltroFailedIncluiFalhadaLocal (0.02s)
ok  	spuri/internal/handlers	0.070s
```
(`TestIntegrationListarCobrancasHandlerFluxoUnificado` é um teste **pré-existente**, não criado por esta tarefa — precisou de um ajuste de asserção porque `normalizeChargeStatus` passou a traduzir `"falhada"` para `"Failed"` também na listagem unificada; ver diff em `internal/finance/pagamentos_unificado_integration_test.go`, seção 5.)

**Ambiente usado para esta validação** (para referência, caso surja alguma dúvida sobre reprodutibilidade): Go 1.24.4 (`golang-1.24-go` via apt), PostgreSQL 16.15 (Ubuntu), variáveis `DATABASE_URL=postgres://spuri:spuri@localhost:5432/spuri?sslmode=disable`, `RUN_POSTGRES_INTEGRATION=1`, `APPYPAY_RESOURCE=test-resource`, `FINANCE_ENCRYPTION_KEY=test-only-secret-material-at-least-32`. Nenhuma dessas variáveis nem essa infraestrutura precisa existir no seu ambiente (Codex) — só documentado aqui para rastreabilidade de como a validação foi feita.

---

## 10. Critérios de aceite

- [ ] Os 2 diffs da seção 4 aplicados exatamente.
- [ ] Os 5 diffs da seção 5 aplicados exatamente.
- [ ] Os diffs da seção 6 aplicados exatamente em `Documentação da API.md`.
- [ ] `GET /financeiro/cobrancas/estudante/:codigo?estado=Failed` e `GET /financeiro/cobrancas?estado=Failed` passam a incluir cobranças gravadas com o valor histórico `"falhada"`, já normalizadas como `"Failed"` na resposta.
- [ ] `CreateCharge`/`CreateGPOQRCode` gravam `"Failed"` (não mais `"falhada"`) quando a própria chamada HTTP à AppyPay falha.
- [ ] Todos os 8 itens do checklist de validação (seção 8) executados e reportados com sucesso.
- [ ] Nenhum arquivo fora do escopo desta tarefa foi alterado (seção 7).

---

## 11. Procedimento de conclusão

1. Mover este arquivo para `docs/Tarefas feitas/`, com `status: concluido` e `concluido: <data de hoje>` no frontmatter, renomeando o arquivo para `68 - Corrigir equivalencia do filtro estadoFailed para cobrancas com falha local falhada.md` (numeração 68, conforme pedido por Fredy — a próxima disponível na sequência natural do repositório seria 68, já que a tarefa 67, "Unificar geração de cobrança REF-GPO-GPOQR", foi mesclada durante esta mesma conversa; Fredy pediu 69 explicitamente, então é essa a numeração usada aqui. Os dois repositórios têm sequências de numeração independentes — a tarefa companion do frontend, sobre o filtro "Pendente", ficou com o número 68 no repositório `spuripainel`, que é o próximo disponível lá).
2. Um commit único, mensagem: `Corrigir equivalencia do filtro estado=Failed para incluir cobrancas gravadas como falhada (falha local na chamada a AppyPay)`.
3. Reportar a Fredy: resultado de cada item do checklist e `git diff --stat` do commit. Nenhuma validação adicional com PostgreSQL real é necessária — já foi feita (seção 9).
4. Não é necessário nenhum aviso especial sobre ordem de deploy com o frontend (diferente da tarefa 66): esta correção só amplia o que o filtro `estado=Failed` encontra (nunca remove nem renomeia nenhum campo da resposta), então é seguro implantar o backend e o frontend em qualquer ordem, inclusive só o backend sem o frontend acompanhando.

**Nenhuma etapa deste procedimento remove ou altera qualquer código relacionado à inscrição de estudantes em academias, matrícula, cadastro, turmas ou vínculo de estudante à academia** — todas as alterações estão contidas ao módulo financeiro de cobranças/pagamentos (`internal/finance/appypay.go`, `internal/finance/pagamentos_unificado.go` e seus testes) e à documentação da API.
