---
criado: 2026-08-27
origem: conversa com Fredy (Claude como orquestrador — depuração e implementação com PostgreSQL 16 real em sandbox, Codex como executor)
status: feito
tipo: melhoria
concluido: 2026-08-27
depende_de: nenhuma (aplica sobre o main atual, commit 0966309, que já inclui as tarefas 598e142 e 70)
---

# Expor quais meses continuam em dívida numa cobrança de mensalidade falhada (MensalidadesEmAberto) (feito)

## 0. Leia isto primeiro — sobre esta tarefa e sobre o seu ambiente (Codex)

Claude já implementou, testou e validou esta correção inteira com PostgreSQL 16 real, sobre o `main` atual (commit `0966309`, que já inclui a correção do filtro `estado=Failed` e a tarefa 70 de extração real de status/motivo da AppyPay): `go build ./...`, `go vet ./...`, `gofmt -l .` (limpo) e `go test ./...` — **suíte inteira do repositório**, com banco de dados recriado do zero antes de cada execução — todos passando, sem nenhuma asserção pré-existente alterada.

O seu ambiente (Codex) não tem `psql`/Docker/Postgres e bloqueia `apt` — por isso, a validação com banco de dados real já foi feita por Claude e está descrita na seção 5. A sua tarefa é aplicar o diff da seção 3 e o novo arquivo de teste da seção 3.2, depois rodar o que conseguir do checklist da seção 5.1.

Se o seu ambiente tiver acesso a `proxy.golang.org` (diferente do sandbox de Claude, que precisou de `replace` temporários em `go.mod` só para compilar offline — **não aplicar nenhum `replace` no `go.mod` real**, isso foi só uma limitação pontual do sandbox de Claude e não faz parte desta correção), o build deve funcionar normalmente sem nenhum passo extra.

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento, nesta ordem, sobre o `main` atual do repositório. Todas as decisões de desenho já foram tomadas e validadas por Claude (implementação testada com `go build`, `go vet`, `gofmt` e a suíte inteira `go test ./...` contra PostgreSQL 16 real). Sua tarefa é mecânica: (1) aplicar o diff da seção 3.1, usando `git apply` quando possível ou replicando manualmente o antes/depois quando o diff não aplicar por causa de drift de linha; (2) criar o arquivo novo da seção 3.2 exatamente como está; (3) rodar cada item do checklist da seção 5.1 e reportar o resultado; (4) seguir o "Procedimento de conclusão" (seção 7). Não toque em nenhum arquivo fora do escopo listado na seção 6 ("Fora de escopo").

---

## 2. Contexto — o problema relatado por Fredy

Fredy reportou, com uma captura de tela real do painel: numa consulta de cobranças filtrada por academia (`GET /financeiro/cobrancas?contexto_tipo=academia&codigo_academia=LDA20263&...`), um estudante (SJS1125) com uma cobrança de mensalidade que falhou aparecia só com status `Failed` — sem nenhuma indicação de que ele ainda devia aquele mês. Do lado do próprio estudante, a mesma dívida aparecia claramente, com a opção de pagar.

### 2.1 Causa raiz

Isto **não é um bug de deduplicação** — `FiltrarPendenciasComCobrancaRealVinculada` (tarefa 64) já funciona corretamente e simetricamente nos dois papéis (academia e estudante): quando um mês já tem uma cobrança real vinculada (mesmo que falhada), a pendência sintética duplicada desse mês é removida de propósito da listagem unificada, para não repetir a mesma linha duas vezes. Isso foi verificado com um teste de integração real reproduzindo o cenário de Fredy (2 alunos nunca tentaram pagar + 1 aluno com cobrança falhada, `mes=9`, mesma academia): dos dois lados, o mês falhado aparece exatamente uma vez.

O problema real é um **efeito colateral não intencional dessa deduplicação**: como a pendência sintética (que sempre carrega `Status == "pendente"`) é removida, e a cobrança real que fica no lugar dela mostra o status da própria tentativa (`Failed`/`Cancelled`/`Expired`), **nada na resposta da API dizia se a falta de pagamento daquele mês continuava em aberto**. O campo `Status` de uma cobrança é sobre o que aconteceu com *aquela tentativa específica*, não sobre o estado atual da obrigação (`financeiro_mensalidade_obrigacoes_eventos`) — e são coisas que podem divergir (ex.: a academia pode ter anulado a mensalidade depois; ou, mais comum, o estudante pode ter feito uma segunda tentativa bem-sucedida depois, tornando o mês pago apesar da primeira cobrança continuar `Failed` no histórico).

O próprio Spuri já sabe, com precisão, se um mês continua devido — é exatamente a regra de precedência que `estadoObrigacao`/`PendenciasSemCobranca` usam desde a tarefa 63 ("uma tentativa falhada nunca tira um mês do estado pendente"). Só que essa informação nunca era exposta junto da cobrança real na listagem.

### 2.2 Por que ninguém percebeu antes

Os testes existentes de `ListCobrancas`/`ListCobrancasEstudante` e da deduplicação (`TestIntegrationFiltrarPendenciasComCobrancaRealVinculadaRemoveApenasOsVinculados`, `TestIntegrationListarCobrancasHandlerFluxoUnificado`) verificam que a *lista* está correta (sem duplicatas, contagens certas) — nenhum deles verifica o *conteúdo* de uma cobrança `Failed` isoladamente para checar se ela comunica que a dívida persiste. O gap só aparece quando se pergunta, isoladamente, "essa cobrança Failed — o estudante ainda deve isso?", que foi exatamente a pergunta de Fredy.

---

## 3. Diffs a aplicar

### 3.1 `internal/finance/appypay.go` e `internal/finance/pagamentos_unificado.go`

Aplicar via `git apply` (colar em um arquivo `.patch` e rodar `git apply nome.patch` a partir da raiz do repositório; se falhar por drift de linha, replicar manualmente cada trecho `-`/`+`):

```diff
diff --git a/internal/finance/appypay.go b/internal/finance/appypay.go
index eb1d359..ce32393 100644
--- a/internal/finance/appypay.go
+++ b/internal/finance/appypay.go
@@ -212,6 +212,22 @@ type CobrancaResumo struct {
 	Valor            float64 `json:"valor"`
 	Moeda            string  `json:"moeda,omitempty"`
 	Descricao        string  `json:"descricao,omitempty"`
+	// MensalidadesEmAberto é o subconjunto de Mensalidades cujo estado da
+	// obrigação (financeiro_mensalidade_obrigacoes_eventos, mesma regra de
+	// precedenciaEstado usada por estadoObrigacao/PendenciasSemCobranca)
+	// ainda é EstadoPendente no momento da consulta. Só populado por
+	// PreencherMensalidadesEmAberto (pagamentos_unificado.go), chamada
+	// automaticamente ao final de ListCobrancas/ListCobrancasEstudante,
+	// para Origem == "mensalidade" com Status terminal de falha
+	// (Failed/Cancelled/Expired/falhada/cancelada — nunca para Success nem
+	// para aguardando_pagamento, que já é auto-explicativo). Existe porque
+	// FiltrarPendenciasComCobrancaRealVinculada (tarefa 64) remove de
+	// propósito a pendência sintética duplicada do mesmo mês da listagem
+	// unificada — sem este campo, uma cobrança Failed não informava, por
+	// si só, se a dívida daquele mês continuava em aberto (relatado por
+	// Fredy: a academia via "Failed" sem nenhum sinal de que o estudante
+	// ainda devia).
+	MensalidadesEmAberto []MensalidadeSelecaoMes `json:"mensalidades_em_aberto,omitempty"`
 	// MetodoPagamento reflete "GPO_QR" (não apenas "GPO") quando a cobrança
 	// tem qr_code_type no payload — CreateGPOQRCode grava payment_method
 	// como "GPO" internamente, então sem este ajuste a origem QR ficaria
@@ -743,6 +759,9 @@ func (s *Service) ListCobrancas(ctx context.Context, contexto, academia string,
 	if err := rows.Err(); err != nil {
 		return nil, err
 	}
+	if err := s.PreencherMensalidadesEmAberto(ctx, out); err != nil {
+		return nil, err
+	}
 	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
 }
 
@@ -953,6 +972,9 @@ func (s *Service) ListCobrancasEstudante(ctx context.Context, codigoEstudante st
 	if err := rows.Err(); err != nil {
 		return nil, err
 	}
+	if err := s.PreencherMensalidadesEmAberto(ctx, out); err != nil {
+		return nil, err
+	}
 	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
 }
 
diff --git a/internal/finance/pagamentos_unificado.go b/internal/finance/pagamentos_unificado.go
index 71188f5..77f0f5b 100644
--- a/internal/finance/pagamentos_unificado.go
+++ b/internal/finance/pagamentos_unificado.go
@@ -191,6 +191,88 @@ func pendenciaParaPagamentoResumo(m MensalidadeMesView) PagamentoResumo {
 // isso. Sem nenhum filtro informado em qualquer um dos dois argumentos
 // (slice vazia), pendências continuam incluídas — mesmo comportamento de
 // sempre.
+// PreencherMensalidadesEmAberto enriquece, em lote (uma consulta por
+// academia envolvida, nunca N+1), cada CobrancaResumo de Origem ==
+// "mensalidade" cujo Status seja um estado terminal de falha
+// (Failed/Cancelled/Expired/falhada/cancelada) com
+// CobrancaResumo.MensalidadesEmAberto — o subconjunto de Mensalidades cujo
+// estado de obrigação ainda é EstadoPendente, pela mesma regra de
+// precedência que estadoObrigacao/PendenciasSemCobranca usam
+// (precedenciaEstado, via estadosObrigacaoBatch).
+//
+// Cobranças Success nunca são tocadas (isSuccessfulChargeStatus) nem
+// cobranças ainda aguardando_pagamento (não é terminal): nesses dois casos
+// o próprio Status já é auto-explicativo, e consultar a obrigação seria
+// trabalho redundante. Cobranças de outra Origem (matricula/avulsa) ou sem
+// CodigoEstudante/CodigoAcademia/Mensalidades também são ignoradas — não
+// têm obrigação de mensalidade para consultar.
+//
+// Motivação: FiltrarPendenciasComCobrancaRealVinculada (tarefa 64) remove
+// de propósito a pendência sintética duplicada do mesmo mês da listagem
+// unificada quando já existe uma cobrança real vinculada — dedup correto
+// para não repetir a mesma linha duas vezes. O efeito colateral não
+// intencional: uma cobrança Failed, sozinha, não informava se o mês
+// continuava em aberto ou se tinha sido resolvido de outra forma (ex.:
+// anulado pela academia) — quem consultasse GET /cobrancas via a academia
+// via "Failed" sem nenhum sinal de que a mensalidade ainda estava por
+// pagar, embora o próprio estudante, ao consultar pendências, continuasse
+// vendo a opção de pagar normalmente (Estado nunca sai de pendente só por
+// causa de uma tentativa falhada — ver Tarefa 63). Este campo fecha essa
+// lacuna sem desfazer a deduplicação.
+//
+// Chamada automaticamente ao final de ListCobrancas e ListCobrancasEstudante
+// (appypay.go), sobre a página já carregada — nunca refaz nem pagina a
+// consulta principal de financeiro_cobrancas.
+func (s *Service) PreencherMensalidadesEmAberto(ctx context.Context, cobrancas []CobrancaResumo) error {
+	porAcademia := map[string][]int{}
+	for idx := range cobrancas {
+		c := &cobrancas[idx]
+		if c.Origem != "mensalidade" || c.CodigoAcademia == "" || c.CodigoEstudante == "" || len(c.Mensalidades) == 0 {
+			continue
+		}
+		if isSuccessfulChargeStatus(c.Status) || !isTerminalChargeStatus(c.Status) {
+			continue
+		}
+		porAcademia[c.CodigoAcademia] = append(porAcademia[c.CodigoAcademia], idx)
+	}
+	for academia, indices := range porAcademia {
+		anosSet := map[string]bool{}
+		estudantesSet := map[string]bool{}
+		for _, idx := range indices {
+			estudantesSet[cobrancas[idx].CodigoEstudante] = true
+			for _, m := range cobrancas[idx].Mensalidades {
+				anosSet[m.AnoLetivo] = true
+			}
+		}
+		anos := make([]string, 0, len(anosSet))
+		for a := range anosSet {
+			anos = append(anos, a)
+		}
+		estudantes := make([]string, 0, len(estudantesSet))
+		for e := range estudantesSet {
+			estudantes = append(estudantes, e)
+		}
+		estados, err := s.estadosObrigacaoBatch(ctx, academia, anos, estudantes)
+		if err != nil {
+			return err
+		}
+		for _, idx := range indices {
+			c := &cobrancas[idx]
+			for _, m := range c.Mensalidades {
+				chave := c.CodigoEstudante + "|" + m.AnoLetivo + "|" + strconv.Itoa(m.Mes)
+				estado := EstadoPendente
+				if b, ok := estados[chave]; ok {
+					estado = b.Estado
+				}
+				if estado == EstadoPendente {
+					c.MensalidadesEmAberto = append(c.MensalidadesEmAberto, m)
+				}
+			}
+		}
+	}
+	return nil
+}
+
 func DeveIncluirPendenciasSemCobranca(estados, origens []string) bool {
 	if len(estados) > 0 && !contains(estados, EstadoPendente) {
 		return false
```

### 3.2 Novo arquivo `internal/finance/mensalidades_em_aberto_integration_test.go`

Criar este arquivo com o conteúdo exato abaixo:

```go
package finance

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// TestIntegrationMensalidadesEmAbertoAcademiaVeDividaMesmoAposCobrancaFalhada
// reproduz o relato de Fredy: uma cobrança de mensalidade falha (a AppyPay
// responde e recusa — código 246/"erro_interno_provedor", cenário real
// coberto pela tarefa 70), e do lado da academia (GET /financeiro/cobrancas
// contexto_tipo=academia) a linha aparecia só com status "Failed", sem
// nenhum sinal de que o estudante ainda devia aquele mês — mesmo a
// pendência sintética correspondente tendo sido removida de propósito pela
// deduplicação (FiltrarPendenciasComCobrancaRealVinculada, tarefa 64).
//
// Cobre três momentos:
//  1. Logo após a falha: MensalidadesEmAberto deve conter o mês (ainda
//     pendente).
//  2. Depois de uma nova tentativa bem-sucedida para o MESMO mês: a cobrança
//     ANTIGA (falhada) consultada de novo deve deixar de mostrar o mês em
//     MensalidadesEmAberto — prova que o campo reflete o estado atual da
//     obrigação, não um "carimbo" fixo da hora em que a cobrança falhou.
func TestIntegrationMensalidadesEmAbertoAcademiaVeDividaMesmoAposCobrancaFalhada(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "SJS-" + mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-EMABERTO", "2025_2026", estudante, nil)
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

	// 1a tentativa: cria a cobrança (POST /charges do mock sempre devolve
	// "Pending" — vira aguardando_pagamento). Depois a AppyPay resolve como
	// Failed (saldo insuficiente) e o Spuri descobre isso numa consulta —
	// mesmo padrão de TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa.
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
	acha := func(res *CobrancaListResult) *CobrancaResumo {
		for i := range res.Cobrancas {
			c := &res.Cobrancas[i]
			if c.CodigoEstudante == estudante && c.Origem == "mensalidade" {
				return c
			}
		}
		return nil
	}

	resAcademia, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", alvo.AnoLetivo, &mesAlvo, 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	cobranca := acha(resAcademia)
	if cobranca == nil {
		t.Fatal("esperava encontrar a cobrança falhada do lado da academia")
	}
	if cobranca.Status != "Failed" {
		t.Fatalf("Status = %q, queria Failed", cobranca.Status)
	}
	if len(cobranca.MensalidadesEmAberto) != 1 || cobranca.MensalidadesEmAberto[0].Mes != mesAlvo {
		t.Fatalf("BUG: cobrança Failed não sinalizou o mês %d como em aberto — MensalidadesEmAberto=%+v (a academia veria 'Failed' sem saber que o estudante ainda deve)", mesAlvo, cobranca.MensalidadesEmAberto)
	}
	t.Logf("OK: cobrança Failed sinaliza corretamente MensalidadesEmAberto=%+v", cobranca.MensalidadesEmAberto)

	// Do lado do estudante, a mesma cobrança deve mostrar exatamente a
	// mesma coisa (simetria entre os dois papéis).
	resEstudante, err := service.ListCobrancasEstudante(ctx, estudante, &academia, nil, nil, nil, nil, "", alvo.AnoLetivo, nil, 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	cobrancaEst := acha(resEstudante)
	if cobrancaEst == nil || len(cobrancaEst.MensalidadesEmAberto) != 1 {
		t.Fatalf("BUG: lado do estudante não sinalizou o mesmo mês em aberto: %+v", cobrancaEst)
	}

	// 2a tentativa, agora bem-sucedida, para o MESMO mês (mesmo padrão:
	// cria como Pending, depois o Spuri descobre Success numa consulta).
	transport.status, transport.code, transport.message = "Pending", 0, ""
	segunda, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
		MetodoPagamento: "GPO", Telefone: "923000000",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("2a tentativa deveria ser aceite (a 1a já está Failed, não bloqueia): %v", err)
	}
	transport.status = "Success"
	consultadaSegunda, err := service.ConsultCharge(ctx, ContextoAcademia, academia, segunda.Charge.ID.String(), estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConsultCharge da 2a tentativa falhou: %v", err)
	}
	if consultadaSegunda.Status != "Success" {
		t.Fatalf("2a tentativa Status = %q, queria Success", consultadaSegunda.Status)
	}

	// Reconsultando a cobrança ANTIGA (falhada): o mês já foi pago por
	// outra cobrança, então MensalidadesEmAberto deve esvaziar — o campo
	// reflete o estado ATUAL da obrigação, não um carimbo da hora da falha.
	resAcademiaDepois, err := service.ListCobrancas(ctx, ContextoAcademia, academia, []string{"Failed"}, nil, nil, nil, "", alvo.AnoLetivo, &mesAlvo, 30, 0)
	if err != nil {
		t.Fatal(err)
	}
	var antiga *CobrancaResumo
	for i := range resAcademiaDepois.Cobrancas {
		if resAcademiaDepois.Cobrancas[i].ID == cobranca.ID {
			antiga = &resAcademiaDepois.Cobrancas[i]
		}
	}
	if antiga == nil {
		t.Fatal("esperava reencontrar a cobrança falhada antiga filtrando por estado=Failed")
	}
	if len(antiga.MensalidadesEmAberto) != 0 {
		t.Fatalf("BUG: cobrança falhada antiga ainda mostra o mês em aberto depois de pago por outra cobrança: %+v", antiga.MensalidadesEmAberto)
	}
	t.Logf("OK: após pagamento bem-sucedido em nova tentativa, a cobrança antiga deixa de sinalizar o mês como em aberto")
}
```

---

## 4. Resumo do que cada mudança faz (para revisão, não para re-decidir nada)

1. **`CobrancaResumo`** ganha um campo novo, aditivo (`json:"mensalidades_em_aberto,omitempty"`): `MensalidadesEmAberto []MensalidadeSelecaoMes`. Nenhum campo existente muda de nome, tipo ou posição.
2. Nova função `(s *Service) PreencherMensalidadesEmAberto(ctx, cobrancas []CobrancaResumo) error` em `pagamentos_unificado.go`: para cada cobrança de `Origem == "mensalidade"` com `Status` num estado terminal de falha (`Failed`/`Cancelled`/`Expired`/`falhada`/`cancelada` — nunca `Success` nem `aguardando_pagamento`), consulta em lote (`estadosObrigacaoBatch`, já existente desde a tarefa 60/63, uma consulta por academia envolvida na página — nunca N+1) o estado atual da obrigação de cada mês listado na cobrança, e preenche `MensalidadesEmAberto` só com os meses que **ainda** estão `EstadoPendente`.
3. `ListCobrancas` e `ListCobrancasEstudante` (`appypay.go`) passam a chamar `PreencherMensalidadesEmAberto` automaticamente, sobre a página já carregada, antes de devolver o resultado — nenhum chamador (nenhum handler HTTP) precisa lembrar de um passo extra.
4. Cobranças de `matricula`/`avulsa`, ou de mensalidade mas `Success`/`aguardando_pagamento`, nunca fazem a consulta extra (`continue` cedo) — sem custo de performance fora do caso relevante.
5. Novo teste de integração `TestIntegrationMensalidadesEmAbertoAcademiaVeDividaMesmoAposCobrancaFalhada`: reproduz o relato de Fredy de ponta a ponta (cobrança falha de verdade via o mock da AppyPay, não um valor seedado manualmente) e confirma três coisas: (a) a cobrança falhada sinaliza corretamente o mês como em aberto; (b) o mesmo vale simetricamente do lado do estudante; (c) depois de uma nova tentativa bem-sucedida para o mesmo mês, a cobrança **antiga** (ainda `Failed` no seu próprio histórico) deixa de sinalizar o mês como em aberto — prova que o campo reflete o estado *atual* da obrigação, não um carimbo fixo da hora da falha.

---

## 5. Validação já executada por Claude (com PostgreSQL 16 real, sobre o main atual — commit 0966309)

```
$ go build ./...                    # sem erros
$ go vet ./...                      # sem avisos
$ gofmt -l .                        # sem arquivos com formatação incorreta
$ go test ./... -count=1            # com banco de dados recriado do zero antes da execução

ok  	spuri/cmd/server
ok  	spuri/internal/db
?   	spuri/internal/domain	[no test files]
ok  	spuri/internal/domain/aggregates
ok  	spuri/internal/finance      (inclui o teste novo desta tarefa)
ok  	spuri/internal/handlers
?   	spuri/internal/jobs	[no test files]
ok  	spuri/internal/middleware
?   	spuri/internal/monitoring	[no test files]
ok  	spuri/internal/projections
ok  	spuri/internal/services
ok  	spuri/internal/storage
ok  	spuri/internal/utils
```

Nenhum teste pré-existente precisou de asserção diferente — a mudança é puramente aditiva (um campo novo `omitempty`, preenchido só num subconjunto específico de casos).

### 5.1 O que o Codex deve rodar

```
go build ./...
go vet ./...
gofmt -l .
```

Os três devem terminar sem erro/aviso algum e `gofmt -l .` não deve listar nenhum arquivo.

Se o ambiente do Codex tiver acesso a PostgreSQL (verificar antes de assumir que não tem), rodar também:

```
RUN_POSTGRES_INTEGRATION=1 DB_HOST=... DB_PORT=... DB_USER=... DB_PASSWORD=... DB_NAME=... DB_SSLMODE=disable \
FINANCE_ENCRYPTION_KEY=<qualquer string de 32+ bytes> APPYPAY_RESOURCE=<qualquer uuid> \
go test ./... -count=1
```

Se não houver Postgres disponível, pular esta etapa e reportar isso claramente no resultado final — não é motivo para bloquear a aplicação do diff, já que Claude já validou esta parte.

---

## 6. Fora de escopo

- Qualquer mudança em `FiltrarPendenciasComCobrancaRealVinculada`/deduplicação (tarefa 64) — verificado com teste real que já funciona corretamente e simetricamente entre academia e estudante; não é tocada aqui.
- Qualquer mudança relacionada à extração de status/motivo real da AppyPay (`extractProviderOutcome`, `appyPayCodeOutcomes`) — isso é a tarefa 70, já mesclada, e não é tocada aqui.
- Qualquer mudança no frontend (`spuripainel`) — o campo novo é aditivo/`omitempty`; um frontend que não o leia continua funcionando exatamente como antes. Exibir `mensalidades_em_aberto` na tabela de cobranças (ex.: um selo "ainda em dívida" ao lado do status `Failed`) é uma tarefa separada, de frontend, fora do escopo deste documento.
- Granularidade por-mês em cobranças `aguardando_pagamento` ou `Success`: não populamos `MensalidadesEmAberto` nesses casos de propósito (o próprio `Status` já é auto-explicativo nesses dois casos, e consultar a obrigação seria trabalho redundante).

## 7. Critérios de aceite

1. O diff da seção 3.1 aplicado exatamente como descrito, sobre o `main` atual.
2. O arquivo novo da seção 3.2 criado exatamente como está.
3. `go build ./...`, `go vet ./...` e `gofmt -l .` limpos.
4. Se Postgres estiver disponível no ambiente do Codex: `go test ./... -count=1` inteiro passando, com banco limpo, incluindo o novo `TestIntegrationMensalidadesEmAbertoAcademiaVeDividaMesmoAposCobrancaFalhada`.
5. Nenhum arquivo fora da lista da seção 3 alterado.

### Procedimento de conclusão

Ao finalizar:

1. Atualizar o título interno deste documento para `# Expor quais meses continuam em dívida numa cobrança de mensalidade falhada (MensalidadesEmAberto) (feito)`;
2. Confirmar que o front matter já diz `status: feito` e `concluido: 2026-08-27` (ou atualizar a data se aplicado em outro dia);
3. Mover este arquivo para `docs/Tarefas feitas/`, se ainda não estiver lá.
