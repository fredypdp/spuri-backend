---
criado: 2026-08-28
origem: conversa com Fredy (Claude como orquestrador — depuração e implementação com PostgreSQL 16 real em sandbox, Codex como executor)
status: feito
tipo: melhoria
concluido: 2026-08-28
depende_de: nenhuma (aplica sobre o main atual, commit 5cdaa52, que já inclui a tarefa 72 — MensalidadesEmAberto)
---

# Voltar a mostrar a pendência sintética ao lado de uma cobrança de mensalidade falhada na lista unificada (feito)

## 0. Leia isto primeiro — sobre esta tarefa e sobre o seu ambiente (Codex)

Claude já implementou, testou e validou esta correção inteira com PostgreSQL 16 real, sobre o `main` atual (commit `5cdaa52`, que já inclui a tarefa 72 — `MensalidadesEmAberto`): `go build ./...`, `go vet ./...`, `gofmt -l .` (limpo) e `go test ./...` — **suíte inteira do repositório**, com banco de dados recriado do zero antes de cada execução — todos passando.

Esta tarefa **muda o comportamento de duas suítes de teste existentes** (`TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVinculados` → renomeado e reescrito, e `TestIntegrationListarCobrancasHandlerFluxoUnificado`, que teve suas asserções de contagem atualizadas) — isso é esperado e faz parte do diff da seção 3.1, não é uma regressão a investigar.

O seu ambiente (Codex) não tem `psql`/Docker/Postgres — a validação com banco de dados real já foi feita por Claude e está descrita na seção 5. Não é necessário nenhum passo extra de rede/vendoring (o `replace` temporário em `go.mod` usado no sandbox de Claude para compilar offline **não faz parte** desta correção e não deve ser aplicado).

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento, nesta ordem, sobre o `main` atual do repositório. Todas as decisões de desenho já foram tomadas e validadas por Claude (implementação testada com `go build`, `go vet`, `gofmt` e a suíte inteira `go test ./...` contra PostgreSQL 16 real, incluindo a atualização de dois testes existentes cujo comportamento esperado mudou de propósito). Sua tarefa é mecânica: (1) aplicar o diff da seção 3.1, usando `git apply` quando possível ou replicando manualmente o antes/depois quando o diff não aplicar por causa de drift de linha; (2) criar o arquivo novo da seção 3.2 exatamente como está; (3) rodar cada item do checklist da seção 5.1 e reportar o resultado; (4) seguir o "Procedimento de conclusão" (seção 7). Não toque em nenhum arquivo fora do escopo listado na seção 6 ("Fora de escopo").

---

## 2. Contexto — o problema relatado por Fredy

Depois da tarefa 72 (`MensalidadesEmAberto`), Fredy testou o resultado no painel (frontend) e reportou que o problema de fundo continuava: consultando a lista de cobranças **sem nenhum filtro**, o frontend só recebia a linha da cobrança `Failed` — a pendência sintética do mesmo mês (com o botão de pagar) continuava sem aparecer como item separado na tabela. A tarefa 72 resolveu o problema adicionando um *campo* (`mensalidades_em_aberto`) dentro da própria cobrança falhada, mas isso exige o frontend saber ler e exibir esse campo especificamente — o que ele não fazia. O pedido explícito de Fredy: **"o correto seria retornar os dois quando consultado tudo sem filtros: a cobrança pendente do estudante e as tentativas falhas que ele teve no pagamento, para serem listadas na tabela."** — ou seja, dois itens separados na lista, não um campo dentro de um único item.

### 2.1 Causa raiz

`mesesComCobrancaRealVinculada` (`pagamentos_unificado.go`, desde a tarefa 64) considerava **qualquer** cobrança real vinculada a um mês — mesmo já `Failed`/`Cancelled`/`Expired`/`falhada`/`cancelada` (estados terminais, que **não bloqueiam** uma nova tentativa de pagamento; ver `chargeAbertaStatusExcluidos` em `mensalidade.go`, usado por `mensalidadeTemCobrancaAberta`) — como motivo suficiente para esconder a pendência sintética daquele mês na lista unificada (`FiltrarPendenciasComCobrancaRealVinculada`). Isso fazia sentido para uma cobrança **em aberto** (`aguardando_pagamento`): não convém convidar a uma segunda tentativa enquanto a primeira ainda está em curso. Mas para uma cobrança já **falhada/terminal**, o mês volta a estar livre para uma nova tentativa — e escondia a pendência mesmo assim, sem necessidade.

### 2.2 Correção

`mesesComCobrancaRealVinculada` passa a considerar um mês "vinculado" (e por isso a esconder a pendência sintética) **só** quando existe uma cobrança real **em aberto** para ele — reutilizando a mesma condição `chargeAbertaStatusExcluidos` que já rege se uma nova tentativa de pagamento pode ser iniciada (`mensalidadeTemCobrancaAberta`, `mensalidade.go`). Uma cobrança falhada nunca mais esconde a pendência: as duas passam a aparecer como itens separados na lista unificada, exatamente como Fredy pediu. Uma cobrança `Success` nunca chegava a esta função de qualquer forma — o mês já sai de `EstadoPendente` assim que ela é registrada (tarefa 63), então nem entra como pendência bruta.

O campo `mensalidades_em_aberto` da tarefa 72 continua existindo (não foi revertido — aditivo, `omitempty`, não quebra nada), mas hoje é redundante na maioria dos casos, já que a pendência volta a aparecer como item próprio. Mantido por estabilidade de contrato de API e por continuar útil quando o chamador não pediu pendências sem cobrança.

---

## 3. Diffs a aplicar

### 3.1 `internal/finance/appypay.go`, `internal/finance/pagamentos_unificado.go` e `internal/finance/pagamentos_unificado_integration_test.go`

Aplicar via `git apply` (colar em um arquivo `.patch` e rodar `git apply nome.patch` a partir da raiz do repositório; se falhar por drift de linha, replicar manualmente cada trecho `-`/`+`):

```diff
diff --git a/internal/finance/appypay.go b/internal/finance/appypay.go
index ce32393..dc13801 100644
--- a/internal/finance/appypay.go
+++ b/internal/finance/appypay.go
@@ -220,13 +220,14 @@ type CobrancaResumo struct {
 	// automaticamente ao final de ListCobrancas/ListCobrancasEstudante,
 	// para Origem == "mensalidade" com Status terminal de falha
 	// (Failed/Cancelled/Expired/falhada/cancelada — nunca para Success nem
-	// para aguardando_pagamento, que já é auto-explicativo). Existe porque
-	// FiltrarPendenciasComCobrancaRealVinculada (tarefa 64) remove de
-	// propósito a pendência sintética duplicada do mesmo mês da listagem
-	// unificada — sem este campo, uma cobrança Failed não informava, por
-	// si só, se a dívida daquele mês continuava em aberto (relatado por
-	// Fredy: a academia via "Failed" sem nenhum sinal de que o estudante
-	// ainda devia).
+	// para aguardando_pagamento, que já é auto-explicativo). Desde a
+	// tarefa 73, a pendência sintética do mesmo mês também volta a
+	// aparecer como item separado na listagem unificada (só cobranças "em
+	// aberto" — aguardando_pagamento — continuam escondendo-a; ver
+	// mesesComCobrancaRealVinculada), então este campo hoje é redundante
+	// com aquele item na maioria dos casos — mantido por não quebrar
+	// contrato de API e por continuar útil quando o chamador não pediu a
+	// pendência sem cobrança (DeveIncluirPendenciasSemCobranca == false).
 	MensalidadesEmAberto []MensalidadeSelecaoMes `json:"mensalidades_em_aberto,omitempty"`
 	// MetodoPagamento reflete "GPO_QR" (não apenas "GPO") quando a cobrança
 	// tem qr_code_type no payload — CreateGPOQRCode grava payment_method
diff --git a/internal/finance/pagamentos_unificado.go b/internal/finance/pagamentos_unificado.go
index b71f61a..76cb430 100644
--- a/internal/finance/pagamentos_unificado.go
+++ b/internal/finance/pagamentos_unificado.go
@@ -22,26 +22,45 @@ import (
 )
 
 // mesesComCobrancaRealVinculada devolve o conjunto de (codigo_estudante,
-// ano_letivo, mes) que já têm PELO MENOS uma cobrança real vinculada em
-// financeiro_mensalidade_cobrancas — mesmo que ela tenha falhado. Mesma
-// consulta que a função cobrancasExistentesMensalidade fazia antes da
-// tarefa 63 (removida ali) — mas usada aqui para um propósito DIFERENTE:
-// deduplicação na composição da lista unificada (ver
-// filtrarPendenciasComCobrancaRealVinculada), não para decidir se um mês
+// ano_letivo, mes) que já têm PELO MENOS uma cobrança real "em aberto"
+// vinculada em financeiro_mensalidade_cobrancas — mesma regra de
+// chargeAbertaStatusExcluidos que mensalidadeTemCobrancaAberta usa para
+// decidir se uma nova tentativa de pagamento pode ser iniciada
+// (mensalidade.go). Antes da tarefa 73, QUALQUER cobrança vinculada (até
+// uma já Failed/Cancelled/Expired) contava, o que escondia a pendência
+// sintética mesmo quando o mês já podia ser tentado de novo — relatado por
+// Fredy: a academia via só a cobrança "Failed" na consulta sem filtros,
+// sem a pendência aparecer ao lado dela mostrando que o estudante ainda
+// podia pagar.
+//
+// Agora só uma cobrança "em aberto" (aguardando_pagamento — a única coisa
+// que de fato bloqueia mensalidadeTemCobrancaAberta) esconde a pendência:
+// enquanto há uma tentativa em curso, duplicar o convite para pagar de
+// novo seria confuso. Uma cobrança que já chegou a um estado terminal de
+// falha (Failed/Cancelled/Expired/falhada/cancelada) NÃO esconde mais a
+// pendência — as duas aparecem lado a lado na lista unificada: a
+// pendência (ainda pagável) e a(s) tentativa(s) falhada(s) (histórico).
+// Uma cobrança Success também não esconde nada aqui porque o mês já nem
+// chega a esta função como pendência: Estado deixa de ser EstadoPendente
+// assim que ela é registrada (ver PendenciasSemCobranca/tarefa 63), então
+// já é filtrado a montante.
+//
+// Mesma consulta que a função cobrancasExistentesMensalidade fazia antes
+// da tarefa 63 (removida ali) — mas usada aqui para um propósito
+// DIFERENTE: deduplicação na composição da lista unificada (ver
+// FiltrarPendenciasComCobrancaRealVinculada), não para decidir se um mês
 // está pago. Essa decisão continua sendo feita exclusivamente por
 // Estado != EstadoPendente, dentro de PendenciasSemCobranca/
-// PendenciasSemCobrancaEstudante — nenhuma das duas muda, e a tarefa 63
-// continua válida: um mês com cobrança falhada continua contando como
-// pendente. O que esta função resolve é só evitar que ele apareça DUAS
-// vezes na lista final — uma vez como a cobrança real (com seu status
-// verdadeiro, ex. "Failed") e outra vez como uma pendência sintética
-// redundante para o mesmo mês.
+// PendenciasSemCobrancaEstudante — nenhuma das duas muda.
 func (s *Service) mesesComCobrancaRealVinculada(ctx context.Context, academia string, estudantes []string) (map[string]bool, error) {
 	out := map[string]bool{}
 	if len(estudantes) == 0 {
 		return out, nil
 	}
-	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT codigo_estudante, ano_letivo, mes FROM financeiro_mensalidade_cobrancas WHERE codigo_academia=$1 AND codigo_estudante = ANY($2)`, academia, pq.Array(estudantes))
+	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT m.codigo_estudante, m.ano_letivo, m.mes
+		FROM financeiro_mensalidade_cobrancas m JOIN financeiro_cobrancas c ON c.id=m.charge_id
+		WHERE m.codigo_academia=$1 AND m.codigo_estudante = ANY($2)
+		AND lower(COALESCE(c.payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)`, academia, pq.Array(estudantes))
 	if err != nil {
 		return nil, err
 	}
diff --git a/internal/finance/pagamentos_unificado_integration_test.go b/internal/finance/pagamentos_unificado_integration_test.go
index 86f06c8..3d46edc 100644
--- a/internal/finance/pagamentos_unificado_integration_test.go
+++ b/internal/finance/pagamentos_unificado_integration_test.go
@@ -16,7 +16,7 @@ import (
 // significa que, ao montar a lista unificada, esse mês apareceria duas
 // vezes (uma como a cobrança real falhada, outra como pendência sintética
 // redundante) se não for filtrado antes.
-func TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVinculados(t *testing.T) {
+func TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaMantemFalhadaExcluiApenasAberta(t *testing.T) {
 	client := integrationClient(t)
 	service := NewService(client)
 	ctx := context.Background()
@@ -24,31 +24,42 @@ func TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVincu
 	academia := mensalidadeCodigo()
 	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
 	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
-	// ESTVINC1 tem uma cobrança falhada para setembro — continua contando
-	// como pendente (tarefa 63), mas não deve gerar pendência sintética
-	// duplicada para o mesmo mês.
+	// ESTVINC1 tem uma cobrança FALHADA para setembro — continua contando
+	// como pendente (tarefa 63) E, desde a tarefa 73, volta a aparecer
+	// como pendência sintética separada da cobrança real (o mês pode ser
+	// tentado de novo — mensalidadeTemCobrancaAberta não bloqueia
+	// falhada/Failed/cancelada/Cancelled/Expired).
 	seedMensalidadeTurma(t, client, academia, "T-VINC-A", "2026_2027", "ESTVINC1", nil)
 	// ESTVINC2 nunca teve nenhuma tentativa — continua aparecendo como
 	// pendência sintética normalmente.
 	seedMensalidadeTurma(t, client, academia, "T-VINC-B", "2026_2027", "ESTVINC2", nil)
-
-	chargeID := uuid.New()
-	payload, err := json.Marshal(map[string]any{
-		"status": "falhada", "amount": 15000, "currency": "AOA", "description": "Propinas: 1 mensalidade(s)",
-		"payment_method": "GPO_QR", "codigo_estudante": "ESTVINC1",
-		"mensalidades": []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 9}},
-	})
-	if err != nil {
-		t.Fatal(err)
-	}
-	if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
-		chargeID, integrationMerchant("VINC"), academia, payload); err != nil {
-		t.Fatal(err)
-	}
-	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_cobrancas (charge_id,codigo_estudante,codigo_academia,ano_letivo,mes) VALUES ($1,'ESTVINC1',$2,'2026_2027',9)`,
-		chargeID, academia); err != nil {
-		t.Fatal(err)
+	// ESTVINC3 tem uma cobrança ABERTA (aguardando_pagamento) para
+	// setembro — essa SIM deve continuar escondendo a pendência sintética,
+	// para não convidar a uma segunda tentativa enquanto a primeira ainda
+	// está em curso (mensalidadeTemCobrancaAberta bloqueia esse caso).
+	seedMensalidadeTurma(t, client, academia, "T-VINC-C", "2026_2027", "ESTVINC3", nil)
+
+	inserirCobranca := func(estudante, status string) {
+		chargeID := uuid.New()
+		payload, err := json.Marshal(map[string]any{
+			"status": status, "amount": 15000, "currency": "AOA", "description": "Propinas: 1 mensalidade(s)",
+			"payment_method": "GPO_QR", "codigo_estudante": estudante,
+			"mensalidades": []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 9}},
+		})
+		if err != nil {
+			t.Fatal(err)
+		}
+		if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
+			chargeID, integrationMerchant("VINC"), academia, payload); err != nil {
+			t.Fatal(err)
+		}
+		if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_cobrancas (charge_id,codigo_estudante,codigo_academia,ano_letivo,mes) VALUES ($1,$2,$3,'2026_2027',9)`,
+			chargeID, estudante, academia); err != nil {
+			t.Fatal(err)
+		}
 	}
+	inserirCobranca("ESTVINC1", "falhada")
+	inserirCobranca("ESTVINC3", "aguardando_pagamento")
 
 	mesSetembro := 9
 	pendencias, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", &mesSetembro)
@@ -69,20 +80,21 @@ func TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVincu
 	if err != nil {
 		t.Fatal(err)
 	}
+	presentes := map[string]bool{}
 	for _, p := range filtradas {
-		if p.CodigoEstudante == "ESTVINC1" && p.Mes == 9 {
-			t.Fatalf("ESTVINC1/setembro já tem cobrança real vinculada (falhada); não deveria sobrar em pendências após o filtro: %#v", p)
+		if p.Mes == 9 {
+			presentes[p.CodigoEstudante] = true
 		}
 	}
-	achouEst2 := false
-	for _, p := range filtradas {
-		if p.CodigoEstudante == "ESTVINC2" && p.Mes == 9 {
-			achouEst2 = true
-		}
+	if !presentes["ESTVINC1"] {
+		t.Fatal("BUG: ESTVINC1/setembro tem só uma cobrança FALHADA (mês retentável); a pendência sintética deveria continuar aparecendo ao lado dela, não desaparecer")
 	}
-	if !achouEst2 {
+	if !presentes["ESTVINC2"] {
 		t.Fatal("ESTVINC2/setembro nunca teve nenhuma cobrança; deveria continuar em pendências após o filtro")
 	}
+	if presentes["ESTVINC3"] {
+		t.Fatal("BUG: ESTVINC3/setembro tem uma cobrança ABERTA (aguardando_pagamento); a pendência sintética duplicada não deveria aparecer enquanto essa tentativa está em curso")
+	}
 }
 
 // TestIntegrationListarCobrancasHandlerFluxoUnificado reproduz, com
@@ -152,14 +164,16 @@ func TestIntegrationListarCobrancasHandlerFluxoUnificado(t *testing.T) {
 		t.Fatal(err)
 	}
 
-	// Total esperado para setembro: 3 pendências sintéticas (FLX01-03) +
-	// 1 cobrança real falhada (FLXFL) = 4. ESTFLXPG (pago) não entra em
+	// Total esperado para setembro: 4 pendências sintéticas (FLX01-03 +
+	// FLXFL — desde a tarefa 73, uma cobrança falhada NÃO esconde mais a
+	// pendência sintética do mesmo mês, porque o mês continua retentável)
+	// + 1 cobrança real falhada (FLXFL) = 5. ESTFLXPG (pago) não entra em
 	// nenhuma das duas fontes.
-	if res.Total != 4 {
-		t.Fatalf("esperava total=4 (3 pendências + 1 cobrança falhada), obteve %d: %#v", res.Total, res.Pagamentos)
+	if res.Total != 5 {
+		t.Fatalf("esperava total=5 (4 pendências + 1 cobrança falhada), obteve %d: %#v", res.Total, res.Pagamentos)
 	}
-	if len(res.Pagamentos) != 4 {
-		t.Fatalf("esperava 4 itens na página, obteve %d", len(res.Pagamentos))
+	if len(res.Pagamentos) != 5 {
+		t.Fatalf("esperava 5 itens na página, obteve %d", len(res.Pagamentos))
 	}
 
 	var pendentesSinteticas, cobrancasReais int
@@ -189,8 +203,8 @@ func TestIntegrationListarCobrancasHandlerFluxoUnificado(t *testing.T) {
 			}
 		}
 	}
-	if pendentesSinteticas != 3 {
-		t.Fatalf("esperava 3 pendências sintéticas, obteve %d", pendentesSinteticas)
+	if pendentesSinteticas != 4 {
+		t.Fatalf("esperava 4 pendências sintéticas (FLX01-03 + FLXFL), obteve %d", pendentesSinteticas)
 	}
 	if cobrancasReais != 1 {
 		t.Fatalf("esperava 1 cobrança real, obteve %d", cobrancasReais)
@@ -203,12 +217,12 @@ func TestIntegrationListarCobrancasHandlerFluxoUnificado(t *testing.T) {
 	}
 
 	// Ordem: pendências primeiro, cobranças reais depois.
-	for i := 0; i < 3; i++ {
+	for i := 0; i < 4; i++ {
 		if res.Pagamentos[i].Status != EstadoPendente {
 			t.Fatalf("item %d deveria ser pendência (pendências vêm primeiro)", i)
 		}
 	}
-	if res.Pagamentos[3].Status == EstadoPendente {
-		t.Fatal("item 3 deveria ser a cobrança real (por último)")
+	if res.Pagamentos[4].Status == EstadoPendente {
+		t.Fatal("item 4 deveria ser a cobrança real (por último)")
 	}
 }
```

### 3.2 Novo arquivo `internal/finance/lista_unificada_pendencia_e_falha_integration_test.go`

Criar este arquivo com o conteúdo exato abaixo:

```go
package finance

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestIntegrationListaUnificadaMostraPendenciaEFalhaJuntasSemFiltros reproduz
// o relato de Fredy após a tarefa 72: consultando GET /financeiro/cobrancas
// sem filtro de estado, para um estudante com uma tentativa de mensalidade
// falhada, o esperado é que a lista unificada devolva OS DOIS itens lado a
// lado — a pendência sintética daquele mês (ainda pagável, com Estado ==
// EstadoPendente) e a própria cobrança falhada (histórico da tentativa) —
// em vez de a pendência desaparecer só porque existe uma cobrança real
// vinculada a ela.
//
// Passa pelo fluxo real: cria a cobrança via IniciarPagamentoMensalidades
// (mock da AppyPay), deixa-a falhar de verdade via ConsultCharge, e só
// depois monta a lista unificada exatamente como ListarCobrancasAppyPay
// (handler HTTP) faz: PendenciasSemCobranca -> FiltrarPendenciasComCobrancaRealVinculada
// -> ListarPagamentosUnificado.
func TestIntegrationListaUnificadaMostraPendenciaEFalhaJuntasSemFiltros(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "SJS-" + mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-UNIF73", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 10000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO}' WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}
	configureIntegrationCredential(t, service, ContextoAcademia, academia)

	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendentes) == 0 {
		t.Fatal("esperava mensalidade pendente")
	}
	alvo := pendentes[0]
	meses := []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}}

	transport := &appyPayMockTransport{status: "Pending"}
	service.SetHTTPClient(&http.Client{Transport: transport})
	primeira, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
		MetodoPagamento: "GPO", Telefone: "923000000",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("1a tentativa deveria ser aceite: %v", err)
	}
	transport.status, transport.code, transport.message = "Failed", 246, "Internal provider error"
	if _, err := service.ConsultCharge(ctx, ContextoAcademia, academia, primeira.Charge.ID.String(), estudante, "estudante", "127.0.0.1"); err != nil {
		t.Fatalf("ConsultCharge falhou: %v", err)
	}

	mesAlvo := alvo.Mes

	// Exatamente o pipeline de ListarCobrancasAppyPay, sem filtro de
	// estado nem de origem (== "consultado tudo sem filtros").
	pendencias, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", alvo.AnoLetivo, &mesAlvo)
	if err != nil {
		t.Fatal(err)
	}
	pendencias, err = service.FiltrarPendenciasComCobrancaRealVinculada(ctx, pendencias)
	if err != nil {
		t.Fatal(err)
	}
	res, err := ListarPagamentosUnificado(pendencias, func(l, o int) (*CobrancaListResult, error) {
		return service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", alvo.AnoLetivo, &mesAlvo, 30, 0)
	}, 30, 0)
	if err != nil {
		t.Fatal(err)
	}

	var temPendente, temFalha bool
	for _, p := range res.Pagamentos {
		if p.CodigoEstudante != estudante {
			continue
		}
		if p.Status == EstadoPendente {
			temPendente = true
		}
		if p.Status == "Failed" {
			temFalha = true
		}
	}
	if !temPendente {
		t.Fatalf("BUG: a pendência sintética de %s/%d não apareceu na lista unificada sem filtros — a academia não veria a opção de o estudante ainda pagar: %#v", alvo.AnoLetivo, mesAlvo, res.Pagamentos)
	}
	if !temFalha {
		t.Fatalf("BUG: a cobrança falhada de %s/%d não apareceu na lista unificada sem filtros: %#v", alvo.AnoLetivo, mesAlvo, res.Pagamentos)
	}
	t.Logf("OK: pendência e cobrança falhada aparecem juntas na lista sem filtros, %d itens no total para o mês", res.Total)
}
```

---

## 4. Resumo do que cada mudança faz (para revisão, não para re-decidir nada)

1. **`mesesComCobrancaRealVinculada`** (`pagamentos_unificado.go`): a query SQL passa a fazer `JOIN financeiro_cobrancas` e filtrar `lower(payload->>'status') NOT IN (chargeAbertaStatusExcluidos)` — só cobranças "em aberto" (não terminal-falhadas, não bem-sucedidas) contam como vinculadas para efeito de esconder a pendência sintética. Comentário da função reescrito para explicar a nova regra e apontar para `mensalidadeTemCobrancaAberta` como fonte da mesma condição.
2. Comentário do campo `MensalidadesEmAberto` (`appypay.go`) atualizado para não afirmar mais que a pendência é sempre escondida — hoje só é escondida quando a cobrança vinculada está em aberto.
3. `TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVinculados` renomeado para `TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaMantemFalhadaExcluiApenasAberta` e reescrito: agora cobre três estudantes — um com cobrança falhada (pendência deve **continuar** aparecendo), um sem nenhuma tentativa (sempre apareceu), e um novo, com cobrança **aguardando_pagamento** (pendência deve **continuar escondida** — prova que o caso que a tarefa 64 queria proteger continua protegido).
4. `TestIntegrationListarCobrancasHandlerFluxoUnificado`: contagens atualizadas de 4→5 itens totais (a pendência de `ESTFLXFL`, antes escondida, agora soma-se às 3 pendências sintéticas puras, totalizando 4 pendências + 1 cobrança real = 5) e de "3 pendências, depois a real" para "4 pendências, depois a real".
5. Novo teste de integração `TestIntegrationListaUnificadaMostraPendenciaEFalhaJuntasSemFiltros`: reproduz o relato de Fredy de ponta a ponta pelo fluxo real (cobrança de mensalidade criada e depois descoberta como `Failed` via o mock da AppyPay, não uma fixture SQL manual), passando pelo pipeline exato de `ListarCobrancasAppyPay` (`PendenciasSemCobranca` → `FiltrarPendenciasComCobrancaRealVinculada` → `ListarPagamentosUnificado`) sem nenhum filtro de estado/origem, e confirma que os dois itens (pendência `Estado == "pendente"` e cobrança `Status == "Failed"`) aparecem juntos na resposta.

---

## 5. Validação já executada por Claude (com PostgreSQL 16 real, sobre o main atual — commit 5cdaa52)

```
$ go build ./...                    # sem erros
$ go vet ./...                      # sem avisos
$ gofmt -l .                        # sem arquivos com formatação incorreta
$ go test ./... -count=1            # com banco de dados recriado do zero antes da execução

ok  	spuri/cmd/server
ok  	spuri/internal/db
?   	spuri/internal/domain	[no test files]
ok  	spuri/internal/domain/aggregates
ok  	spuri/internal/finance      (inclui o teste novo desta tarefa e os dois atualizados)
ok  	spuri/internal/handlers
?   	spuri/internal/jobs	[no test files]
ok  	spuri/internal/middleware
?   	spuri/internal/monitoring	[no test files]
ok  	spuri/internal/projections
ok  	spuri/internal/services
ok  	spuri/internal/storage
ok  	spuri/internal/utils
```

### 5.1 O que o Codex deve rodar

```
go build ./...
go vet ./...
gofmt -l .
```

Os três devem terminar sem erro/aviso algum e `gofmt -l .` não deve listar nenhum arquivo.

Se o ambiente do Codex tiver acesso a PostgreSQL, rodar também:

```
RUN_POSTGRES_INTEGRATION=1 DB_HOST=... DB_PORT=... DB_USER=... DB_PASSWORD=... DB_NAME=... DB_SSLMODE=disable \
FINANCE_ENCRYPTION_KEY=<qualquer string de 32+ bytes> APPYPAY_RESOURCE=<qualquer uuid> \
go test ./... -count=1
```

Se não houver Postgres disponível, pular esta etapa e reportar isso claramente no resultado final — não é motivo para bloquear a aplicação do diff, já que Claude já validou esta parte.

---

## 6. Fora de escopo

- Frontend (`spuripainel`): já verificado que a tabela unificada (`financeiroShared.tsx`, componente `CobrancasTable`) apenas renderiza a lista de pagamentos vinda diretamente da resposta da API, sem nenhuma deduplicação ou filtro do lado do cliente — então esta correção, sendo só de backend, já é suficiente para os dois itens aparecerem na tabela sem nenhuma mudança de frontend.
- Reverter ou remover o campo `MensalidadesEmAberto`/`PreencherMensalidadesEmAberto` da tarefa 72 — continua existindo, aditivo, só teve seu comentário atualizado.
- Qualquer mudança em `PendenciasSemCobranca`/`PendenciasSemCobrancaEstudante`/`estadoObrigacao` (tarefa 63) — a decisão de o que conta como "pendente" continua exclusivamente ali; esta tarefa só mexe em qual pendência aparece na *listagem unificada*.
- Qualquer mudança na extração de status/motivo real da AppyPay (tarefa 70) — não tocada aqui.

## 7. Critérios de aceite

1. O diff da seção 3.1 aplicado exatamente como descrito, sobre o `main` atual.
2. O arquivo novo da seção 3.2 criado exatamente como está.
3. `go build ./...`, `go vet ./...` e `gofmt -l .` limpos.
4. Se Postgres estiver disponível no ambiente do Codex: `go test ./... -count=1` inteiro passando, com banco limpo, incluindo o novo `TestIntegrationListaUnificadaMostraPendenciaEFalhaJuntasSemFiltros` e os dois testes atualizados.
5. Nenhum arquivo fora da lista da seção 3 alterado.

### Procedimento de conclusão

Ao finalizar:

1. Atualizar o título interno deste documento para `# Voltar a mostrar a pendência sintética ao lado de uma cobrança de mensalidade falhada na lista unificada (feito)`;
2. Confirmar que o front matter já diz `status: feito` e `concluido: 2026-08-28` (ou atualizar a data se aplicado em outro dia);
3. Mover este arquivo para `docs/Tarefas feitas/`, se ainda não estiver lá.
