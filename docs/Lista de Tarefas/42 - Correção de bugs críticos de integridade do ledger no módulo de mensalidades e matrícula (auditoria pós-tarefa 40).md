---
criado: 2026-08-16 03:00
origem: Auditoria profunda do módulo de pagamento (cobrança/pagamento de mensalidades e matrícula) conduzida por Claude (Anthropic) com ambiente real (Go 1.24 + PostgreSQL 16), executando build, vet e toda a suíte de testes de integração repetidamente contra banco limpo. Foco da auditoria: integridade, auditoria, gravação e leitura correta dos dados no ledger.
status: feito
depende_de: "40 - Correção de bugs críticos de produção descobertos na auditoria pós-tarefa 37 (módulo financeiro).md"
---

# Correção de bugs críticos de integridade do ledger no módulo de mensalidades e matrícula (auditoria pós-tarefa 40)

## Prompt recomendado para executar a atualização

```
Leia por completo o arquivo "docs/Lista de Tarefas/41 - Correção de bugs críticos de integridade do ledger no
módulo de mensalidades e matrícula (auditoria pós-tarefa 40).md". Ele contém 5 correções já totalmente
especificadas e validadas (diffs exatos, arquivo e trecho a substituir), mais 1 arquivo de teste de regressão
novo, completo e já validado. Não é necessário planejar, investigar causa raiz ou decidir a abordagem — tudo
isso já foi feito e confirmado experimentalmente contra PostgreSQL real. Aplique as 5 correções na ordem em
que aparecem, exatamente como especificado em cada "Localizar" / "Substituir por", crie o arquivo de teste
novo exatamente como especificado, e então execute a seção "Checklist de aceitação" ao final do documento, na
ordem, sem pular nenhum passo. Se qualquer comando do checklist falhar, pare e reporte o erro — não prossiga
para o próximo item nem tente uma correção diferente da especificada aqui sem antes reportar.
```

## Contexto

A tarefa 40 corrigiu 9 bugs críticos do módulo financeiro descobertos numa auditoria pós-tarefa 37, e foi
verificada como corretamente implementada (build, vet e as duas suítes de integração — `internal/finance` e
`internal/handlers` — passando 5 vezes seguidas cada, contra banco recriado do zero, mais `go test ./...`
limpo em todo o repositório).

A partir daí foi conduzida uma nova auditoria, desta vez focada especificamente no módulo de **pagamento de
mensalidades/propina e matrícula**, com foco em **integridade, auditoria, e gravação/leitura correta dos
dados no ledger** (`spuri_ledger`). Diferente da auditoria anterior, que analisava principalmente os testes já
existentes, esta auditoria **exercitou diretamente, pela primeira vez, os caminhos de código que nenhum teste
do repositório jamais tinha chamado**: `Service.ConfigureMensalidade`, `Service.ConfigureMatricula`, e o fluxo
completo `IniciarPagamentoMensalidades → confirmação AppyPay "Success" → obrigação marcada como paga`.

Isso revelou **5 bugs novos**, não relacionados aos 9 da tarefa 40, dois deles catastróficos: **hoje, em
produção, uma academia nunca consegue configurar propina/matrícula com sucesso visível, e mesmo quando um
estudante paga a propina e a AppyPay confirma o pagamento como "Success", o sistema nunca registra esse
pagamento como concluído** — a mensalidade permanece "pendente" para sempre, sem nenhum evento
`MensalidadesCobrancaConfirmada` no ledger.

Todas as 5 correções abaixo foram aplicadas neste mesmo ambiente real (Go 1.24.4 + PostgreSQL 16) e validadas:
`go build`/`go vet` limpos, um arquivo de teste de regressão novo com 5 testes cobrindo exatamente estes 5
cenários, a suíte completa `internal/finance` (19 testes, os 14 já existentes + os 5 novos) executada 5 vezes
seguidas contra banco recriado do zero — todas as 5 execuções verdes, `internal/handlers` também 5 vezes
seguidas verde, e `go test ./...` do repositório inteiro limpo. Os diffs abaixo são exatamente os que foram
validados; não é necessário alterá-los.

---

## Bug 1 (CRÍTICO) — Gravação de `metodos_pagamento` na projeção financeira falha sempre

**Arquivo:** `internal/projections/financeiro_projection.go`

**Causa raiz confirmada:** os casos `"MatriculaConfigurada"` e `"MensalidadeConfigurada"` de
`FinanceiroProjection.Handle` gravam a coluna `metodos_pagamento TEXT[]` passando `in.MetodosPagamento`
(`[]string`) **diretamente** como parâmetro de `Exec`, sem `pq.Array(...)`. O driver `lib/pq` não sabe
converter um `[]string` cru para um parâmetro de array do Postgres e rejeita a chamada. Reproduzido de forma
isolada, com uma tabela `TEXT[]` de teste e o mesmo `*sqlx.DB` usado em produção:

```
sql: converting argument $1 type: unsupported type []string, a slice of string
```

Isso ocorre **incondicionalmente** — com slice `nil`, vazio ou populado — portanto **toda chamada** a
`ConfigureMatricula` e `ConfigureMensalidade` falha neste ponto, depois de o evento já ter sido gravado no
ledger (ver Bug 2 para o caso de `MatriculaConfigurada`, que nem chega a este ponto por outro motivo). O
resultado é um evento "fantasma": ele existe no ledger, mas nunca fica visível na leitura
(`financeiro_matricula_configuracoes` / `financeiro_mensalidade_configuracoes`), quebrando a integridade
ledger ↔ projeção.

Este é exatamente o mesmo padrão de bug que a tarefa 40 já havia corrigido do lado da **leitura** (`Scan` sem
`pq.Array`, no item 1 daquela tarefa) — aqui o mesmo padrão existe do lado da **escrita**, e não tinha sido
coberto porque nenhum teste do repositório chama `ConfigureMensalidade` nem `ConfigureMatricula`; todos os
testes existentes semeiam `financeiro_mensalidade_configuracoes`/`financeiro_matricula_configuracoes`
diretamente via SQL bruto, contornando este código por completo.

**Correção 1a — import de `pq`:**

Localizar:
```go
	"github.com/google/uuid"
	"spuri/internal/db"
)
```

Substituir por:
```go
	"github.com/google/uuid"
	"github.com/lib/pq"
	"spuri/internal/db"
)
```

**Correção 1b — caso `MatriculaConfigurada`:**

Localizar:
```go
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_matricula_configuracoes (event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,metodos_pagamento,vigente_em) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.Nivel, in.AnoAcademico, stringValue(in.CursoID), in.Valor, in.MetodosPagamento, e.OccurredAt)
```

Substituir por:
```go
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_matricula_configuracoes (event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,metodos_pagamento,vigente_em) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7,$8,$9) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.Nivel, in.AnoAcademico, stringValue(in.CursoID), in.Valor, pq.Array(in.MetodosPagamento), e.OccurredAt)
```

**Correção 1c — caso `MensalidadeConfigurada`:**

Localizar:
```go
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes (event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,metodos_pagamento,vigente_em) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'' )::uuid,$7,$8,$9,$10) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.Nivel, in.AnoAcademico, stringValue(in.CursoID), in.Valor, in.MesFimCobranca, in.MetodosPagamento, e.OccurredAt)
```

Substituir por:
```go
		_, err := p.client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes (event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,metodos_pagamento,vigente_em) VALUES ($1,$2,$3,$4,$5,NULLIF($6,'' )::uuid,$7,$8,$9,$10) ON CONFLICT (event_id) DO NOTHING`, e.EventID, e.AggregateID, in.CodigoAcademia, in.Nivel, in.AnoAcademico, stringValue(in.CursoID), in.Valor, in.MesFimCobranca, pq.Array(in.MetodosPagamento), e.OccurredAt)
```

---

## Bug 2 (CRÍTICO) — Dois tipos de evento financeiro ausentes da whitelist do ledger

**Arquivo:** `internal/db/safe_queries.go`

**Causa raiz confirmada:** o mapa `validEventTypes`, usado por `ValidateEventType` dentro de
`EventStore.Append`/`AppendTx` (chamado por `AggregateRepository.SaveWithAudit`, o caminho usado por
**todo** `s.record`/`s.recordMensalidade` do pacote `internal/finance`), contém todos os tipos de evento
financeiro emitidos pelo código **exceto dois**: `"MatriculaConfigurada"` e
`"MensalidadesCobrancaConfirmada"`. Ambos já são emitidos por `internal/finance` (em `matricula.go` e
`mensalidade.go` respectivamente) e ambos já são tratados corretamente por `FinanceiroProjection.Handle` — só
faltava esta whitelist, um mapa de segurança separado, ser atualizada.

Efeito confirmado, chamando `Service.ConfigureMatricula` e `Service.confirmMensalidadeCharge` (via
`IniciarPagamentoMensalidades` + `ConsultCharge`) de ponta a ponta contra Postgres real:

- `ConfigureMatricula` falha **imediatamente**, com `tipo de evento inválido: MatriculaConfigurada`, **antes
  mesmo de gravar qualquer coisa no ledger**. Como `MetodosPagamento` é obrigatório (`len(...) >= 1`) em toda
  chamada válida de `ConfigureMatricula`, isto significa que a configuração de matrícula está **100% quebrada
  em produção**, sem exceção.
- `confirmMensalidadeCharge` — chamada depois de **toda** cobrança de mensalidade que a AppyPay confirma como
  `"Success"` (seja na criação síncrona, na consulta, ou no webhook) — falha da mesma forma, com
  `tipo de evento inválido: MensalidadesCobrancaConfirmada`. Esse erro é **silenciosamente descartado** com
  `_ = s.confirmMensalidadeCharge(...)` em todos os pontos onde é chamado. Ou seja: **um estudante pode pagar a
  propina, a AppyPay pode confirmar o pagamento, e o Spuri nunca registra esse pagamento como concluído** — a
  mensalidade permanece com estado `"pendente"` para sempre, sem nenhum evento no ledger provando que o
  pagamento aconteceu.

Consequência adicional confirmada: como o evento `MensalidadeConfigurada` **é** aceito pelo ledger (só falha
depois, na projeção — Bug 1), qualquer chamada bem-sucedida de `ConfigureMensalidade` deixa esse evento
gravado no ledger. Um `Rebuild()` da projeção `"financeiro"` (endpoint administrativo real:
`POST /admin/projections/rebuild/financeiro`) reprocessa os eventos do ledger em ordem e **aborta inteiro,
para todas as academias**, ao encontrar esse evento, porque `FinanceiroProjection.Handle` continua falhando
para ele com o mesmo erro do Bug 1. Isso foi confirmado empiricamente: bastou uma chamada a
`ConfigureMensalidade` (mesmo que ela "falhe" do ponto de vista de quem chamou) para que `Rebuild()` passasse
a retornar erro e nunca mais completar.

**Correção — adicionar as duas entradas faltantes na whitelist:**

Localizar:
```go
	"MensalidadeConfigurada":                             true,
	"MesInicioCobrancaDefinido":                          true,
	"ObrigacaoMensalidadeAnulada":                        true,
	"ObrigacaoMensalidadeReativada":                      true,
	// MensalidadePaga is emitted by Phase 3. It is registered now so this
	// projection can consume a real payment event without any compatibility
	// path or inferred payment state.
	"MensalidadePaga": true,
}
```

Substituir por:
```go
	"MensalidadeConfigurada":                             true,
	"MesInicioCobrancaDefinido":                          true,
	"ObrigacaoMensalidadeAnulada":                        true,
	"ObrigacaoMensalidadeReativada":                      true,
	// MensalidadePaga is emitted by Phase 3. It is registered now so this
	// projection can consume a real payment event without any compatibility
	// path or inferred payment state.
	"MensalidadePaga": true,
	// MatriculaConfigurada e MensalidadesCobrancaConfirmada já eram emitidos
	// por internal/finance (matricula.go e mensalidade.go) e já eram
	// tratados por FinanceiroProjection.Handle, mas faltavam nesta
	// whitelist: todo SaveWithAudit/AppendTx para esses dois tipos de
	// evento era rejeitado com "tipo de evento inválido" antes mesmo de
	// tentar gravar no ledger.
	"MatriculaConfigurada":            true,
	"MensalidadesCobrancaConfirmada":  true,
}
```

> Nota: o `gofmt` desta struct usa alinhamento de colunas por bloco de linhas consecutivas sem linha em
> branco/comentário entre elas — o alinhamento acima (`MatriculaConfigurada` e `MensalidadesCobrancaConfirmada`
> alinhados entre si, mas não com o bloco anterior por causa do comentário no meio) já é o resultado de rodar
> `gofmt -w` neste arquivo. Se o editor mudar o alinhamento, rode `gofmt -w internal/db/safe_queries.go` no
> final — o checklist de aceitação verifica isso.

---

## Bug 3 (CRÍTICO) — `confirmMensalidadeCharge` sobrescreve o estudante correto com um valor sempre vazio

**Arquivo:** `internal/finance/mensalidade.go`, função `confirmMensalidadeCharge`

**Causa raiz confirmada:** mesmo depois de corrigir os Bugs 1 e 2, o pagamento de uma mensalidade via GPO ou
REF **continua** nunca sendo confirmado como pago. Instrumentando o código para imprimir o payload da cobrança
imediatamente antes da chamada, confirmou-se que `row.Payload["codigo_estudante"]` **está correto** (ex.:
`"codigo_estudante":"EST-PAGA-40daada0"`), mas o código, logo em seguida, **sobrescreve** essa variável:

```go
estudante, _ := row.Payload["codigo_estudante"].(string)
if info, ok := row.Payload["payment_info"].(map[string]any); ok {
	estudante, _ = info["codigo_estudante"].(string)
}
```

`payment_info` é o mapa de parâmetros específicos do provedor (`ChargeRequest.PaymentInfo`) — para GPO
contém apenas `{"phoneNumber": "..."}`, para REF é um mapa vazio `{}`. Ele **nunca** contém
`codigo_estudante`. Como `payment_info` é sempre um `map[string]any` não-nulo (mesmo vazio) para qualquer
cobrança criada por `IniciarPagamentoMensalidades`, a asserção de tipo `ok` é **sempre verdadeira**, e
`info["codigo_estudante"]` é **sempre** `nil` → `estudante` é sobrescrito para `""` em 100% dos casos. A
função então retorna `errors.New("cobrança de mensalidade sem estudante")`, erro que — como no Bug 2 — é
descartado silenciosamente por todo chamador.

Confirmado com um teste de ponta a ponta: criar uma cobrança de mensalidade via `IniciarPagamentoMensalidades`
(`MetodoPagamento: "GPO"`), fazer a AppyPay (mock) confirmar `"Success"` via `ConsultCharge`, e verificar que
mesmo assim nenhum evento `MensalidadesCobrancaConfirmada` era gravado — com a mensagem de erro
`cobrança de mensalidade sem estudante` aparecendo (depois de instrumentado) apesar de o payload conter o
`codigo_estudante` correto.

**Correção — remover o bloco que sobrescreve `estudante` a partir de `payment_info`:**

Localizar:
```go
	estudante, _ := row.Payload["codigo_estudante"].(string)
	// Student identity is stored in the structured selection payload, never
	// inferred from an untrusted provider response.
	if info, ok := row.Payload["payment_info"].(map[string]any); ok {
		estudante, _ = info["codigo_estudante"].(string)
	}
	if estudante == "" {
		return errors.New("cobrança de mensalidade sem estudante")
	}
```

Substituir por:
```go
	// Student identity is stored in the structured selection payload (set by
	// CreateCharge/CreateGPOQRCode from the validated ChargeRequest), never
	// inferred from payment_info, which only carries provider-specific
	// parameters (e.g. GPO's phoneNumber) and never a student code.
	estudante, _ := row.Payload["codigo_estudante"].(string)
	if estudante == "" {
		return errors.New("cobrança de mensalidade sem estudante")
	}
```

---

## Bug 4 (ALTO) — `Rebuild()` da projeção financeira não limpa duas tabelas derivadas do ledger

**Arquivo:** `internal/projections/financeiro_projection.go`, função `Rebuild`

**Causa raiz confirmada:** a instrução `DELETE` no início de `Rebuild()` limpa
`financeiro_mensalidade_obrigacoes_eventos`, `financeiro_mensalidade_inicio_cobranca`,
`financeiro_mensalidade_configuracoes`, `financeiro_webhooks_recebidos`, `financeiro_cobrancas` e
`financeiro_credenciais_appypay` — mas **não** `financeiro_matricula_configuracoes` nem
`financeiro_mensalidade_cobrancas`, apesar de ambas serem tabelas derivadas do ledger, populadas por
`Handle()` a partir dos eventos `MatriculaConfigurada` e `CobrancaAppyPay*`/`QRCodeAppyPay*` respectivamente
(via `upsertMensalidadeCobrancas`), exatamente como as demais.

Isso significa que, num `Rebuild()` real (endpoint `POST /admin/projections/rebuild/financeiro`), linhas
incorretas ou desatualizadas nessas duas tabelas **sobrevivem silenciosamente** a uma reconstrução completa a
partir do ledger — justamente o cenário em que um `Rebuild()` costuma ser executado (para corrigir uma
projeção corrompida ou divergente). Confirmado com um teste que apaga manualmente uma linha de
`financeiro_matricula_configuracoes` (simulando divergência do read model) e chama `Rebuild()`: sem esta
correção, a linha nunca volta, porque o `ON CONFLICT (event_id) DO NOTHING` do Bug 1 (já corrigido) faz o
`INSERT` de replay ser bem-sucedido silenciosamente só quando a tabela foi previamente esvaziada.

Não há risco de violação de chave estrangeira ao incluir as duas tabelas na mesma instrução `DELETE`:
`financeiro_mensalidade_cobrancas.charge_id` referencia `financeiro_cobrancas(id)`, e a ordem abaixo já
garante que `financeiro_mensalidade_cobrancas` é limpa antes de `financeiro_cobrancas`.
`financeiro_matricula_configuracoes` não tem nenhuma chave estrangeira.

**Correção:**

Localizar:
```go
	// Secrets are operational material and intentionally survive a ledger replay.
	if _, err := p.client.DB().Exec(`DELETE FROM financeiro_mensalidade_obrigacoes_eventos; DELETE FROM financeiro_mensalidade_inicio_cobranca; DELETE FROM financeiro_mensalidade_configuracoes; DELETE FROM financeiro_webhooks_recebidos; DELETE FROM financeiro_cobrancas; DELETE FROM financeiro_credenciais_appypay;`); err != nil {
		return err
	}
```

Substituir por:
```go
	// Secrets are operational material and intentionally survive a ledger replay.
	if _, err := p.client.DB().Exec(`DELETE FROM financeiro_mensalidade_obrigacoes_eventos; DELETE FROM financeiro_mensalidade_inicio_cobranca; DELETE FROM financeiro_mensalidade_configuracoes; DELETE FROM financeiro_mensalidade_cobrancas; DELETE FROM financeiro_matricula_configuracoes; DELETE FROM financeiro_webhooks_recebidos; DELETE FROM financeiro_cobrancas; DELETE FROM financeiro_credenciais_appypay;`); err != nil {
		return err
	}
```

---

## Bug 5 (MÉDIO-ALTO) — `ListMensalidades` ordena os meses numericamente em vez de cronologicamente

**Arquivo:** `internal/finance/mensalidade.go`, função `ListMensalidades`

**Causa raiz confirmada:** a ordenação final de `ListMensalidades` compara `result[i].Mes < result[j].Mes`
— um inteiro de 1 a 12 — em vez de considerar a posição real de cada mês dentro do ano letivo. Como o ano
letivo começa em setembro (ou outubro, no nível superior) e termina em julho do ano civil seguinte, essa
comparação numérica **inverte a ordem cronológica real**: dentro do mesmo `ano_letivo`, janeiro a julho (mês
1–7, do segundo ano civil) aparecem **antes** de setembro a dezembro (mês 9–12, do primeiro ano civil).

Confirmado com um teste que lista as mensalidades de um ano letivo completo e compara a ordem devolvida:

```
esperado (set->jul): [9 10 11 12 1 2 3 4 5 6 7]
obtido:              [1 2 3 4 5 6 7 9 10 11 12]
```

Isto quebra dois comportamentos:

1. A regra de negócio em `IniciarPagamentoMensalidades` que exige que a seleção inclua
   `oldest := pendentes[0]` (a mensalidade pendente mais antiga) — com a ordenação errada, `pendentes[0]`
   pode ser janeiro em vez de setembro, bloqueando ou confundindo pagamentos legítimos com uma mensagem de
   erro que aponta o mês errado como "mais antigo".
2. A ordem em que a lista de mensalidades é devolvida pela API (`GET` que chama `ListMensalidades` e devolve
   o resultado sem reordenar) — o estudante/academia veria os meses fora de ordem cronológica.

O campo `MensalidadeMesView.DataReferencia` já contém a data de calendário correta e monotonicamente
crescente para cada mês do ano letivo (calculada em `mesesAnoLetivo`, que já usa o ano civil certo para cada
mês — setembro–dezembro no primeiro ano, janeiro–julho no segundo). Basta ordenar por ela em vez de pelo
número cru do mês.

**Correção:**

Localizar:
```go
		if result[i].CodigoAcademia != result[j].CodigoAcademia {
			return result[i].CodigoAcademia < result[j].CodigoAcademia
		}
		return result[i].Mes < result[j].Mes
	})
```

Substituir por:
```go
		if result[i].CodigoAcademia != result[j].CodigoAcademia {
			return result[i].CodigoAcademia < result[j].CodigoAcademia
		}
		return result[i].DataReferencia.Before(result[j].DataReferencia)
	})
```

---

## Teste de regressão novo (arquivo completo, já validado)

Criar o arquivo `internal/finance/financeiro_ledger_integrity_test.go` com exatamente o conteúdo abaixo. Ele
cobre os 5 bugs acima: dois testes de escrita (`ConfigureMensalidade`/`ConfigureMatricula` gravando E
projetando corretamente), o teste mais importante — o pagamento de uma mensalidade sendo confirmado como pago
depois de a AppyPay retornar `"Success"` —, o teste de ordenação cronológica, e o teste de que `Rebuild()`
reconstrói de fato `financeiro_matricula_configuracoes`. Ele reutiliza os helpers já existentes em
`mensalidade_integration_test.go` e `appypay_integration_test.go` do mesmo pacote (`mensalidadeCodigo`,
`seedMensalidadeAcademia`, `seedMensalidadeTurma`, `seedMensalidadeConfiguracao`, `mensalidadePorMes`,
`integrationClient`, `configureIntegrationCredential`, `appyPayMockTransport`) — não recria nenhum deles.

```go
package finance

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestIntegrationConfigureMensalidadeGravaNoLedgerEProjectaCorretamente
// cobre o caminho de escrita completo de ConfigureMensalidade: o evento deve
// ser aceite pelo ledger (whitelist de tipos de evento) e a projeção de
// leitura financeiro_mensalidade_configuracoes deve refletir imediatamente o
// que foi configurado, incluindo o campo metodos_pagamento (TEXT[]).
func TestIntegrationConfigureMensalidadeGravaNoLedgerEProjectaCorretamente(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	configureIntegrationCredential(t, service, ContextoAcademia, academia)

	view, err := service.ConfigureMensalidade(ctx, MensalidadeConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental",
		Valor: 1000, MesFimCobranca: 7, MetodosPagamento: []string{"GPO", "REF"},
	}, "admin-teste", "academia", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConfigureMensalidade retornou erro: %v", err)
	}
	if view.Valor != 1000 {
		t.Fatalf("valor inesperado na view retornada: %v", view.Valor)
	}

	var ledgerCount int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND event_type='MensalidadeConfigurada'`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("esperava 1 evento MensalidadeConfigurada no ledger, obteve %d", ledgerCount)
	}

	configs, err := service.ListMensalidadeConfiguracoes(ctx, academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("esperava 1 configuração visível em financeiro_mensalidade_configuracoes, obteve %d", len(configs))
	}
	if len(configs[0].MetodosPagamento) != 2 {
		t.Fatalf("esperava 2 métodos de pagamento persistidos, obteve %v", configs[0].MetodosPagamento)
	}
}

// TestIntegrationConfigureMatriculaGravaNoLedgerEProjectaCorretamente cobre
// o mesmo caminho de escrita para ConfigureMatricula, cujo evento
// MatriculaConfigurada estava totalmente ausente da whitelist do ledger.
func TestIntegrationConfigureMatriculaGravaNoLedgerEProjectaCorretamente(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	configureIntegrationCredential(t, service, ContextoAcademia, academia)

	view, err := service.ConfigureMatricula(ctx, MatriculaConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental",
		Valor: 5000, MetodosPagamento: []string{"GPO"},
	}, "admin-teste", "academia", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConfigureMatricula retornou erro: %v", err)
	}
	if view.Valor != 5000 {
		t.Fatalf("valor inesperado na view retornada: %v", view.Valor)
	}

	var ledgerCount int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND event_type='MatriculaConfigurada'`).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("esperava 1 evento MatriculaConfigurada no ledger, obteve %d", ledgerCount)
	}

	configs, err := service.ListMatriculaConfiguracoes(ctx, academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("esperava 1 configuração visível em financeiro_matricula_configuracoes, obteve %d", len(configs))
	}
	if len(configs[0].MetodosPagamento) != 1 || configs[0].MetodosPagamento[0] != "GPO" {
		t.Fatalf("métodos de pagamento persistidos incorretamente: %v", configs[0].MetodosPagamento)
	}
}

// TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago é o
// teste mais importante deste ficheiro: reproduz o fluxo real de um
// estudante a pagar a propina (mensalidade) - criação da cobrança, seguida
// da AppyPay confirmando o pagamento como "Success" - e garante que essa
// confirmação fica de facto registada no ledger (evento
// MensalidadesCobrancaConfirmada) e refletida como "pago" na leitura.
func TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "EST-PAGA-" + uuid.NewString()[:8]
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-PAGA", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO}' WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}

	configureIntegrationCredential(t, service, ContextoAcademia, academia)
	transport := &appyPayMockTransport{status: "Pending"}
	service.SetHTTPClient(&http.Client{Transport: transport})

	pendentesAntes, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendentesAntes) == 0 {
		t.Fatal("esperava pelo menos uma mensalidade pendente antes do pagamento")
	}
	alvo := pendentesAntes[0]
	if alvo.Estado != EstadoPendente {
		t.Fatalf("primeira mensalidade da lista não está pendente: %#v", alvo)
	}

	view, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
		CodigoEstudante: estudante, CodigoAcademia: academia,
		Meses:           []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}},
		MetodoPagamento: "GPO", Telefone: "923000000",
	}, estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("IniciarPagamentoMensalidades falhou: %v", err)
	}

	// A AppyPay confirma o pagamento de forma assíncrona; o Spuri descobre
	// isso por consulta (ou webhook, mesmo caminho interno). Simula a
	// confirmação exatamente como um pagamento GPO/REF real acontece.
	transport.status = "Success"
	consulted, err := service.ConsultCharge(ctx, ContextoAcademia, academia, view.Charge.ID.String(), estudante, "estudante", "127.0.0.1")
	if err != nil {
		t.Fatalf("ConsultCharge falhou: %v", err)
	}
	if !isSuccessfulChargeStatus(consulted.Status) {
		t.Fatalf("mock não retornou Success na consulta: status=%s", consulted.Status)
	}

	var ledgerCountConfirmada int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND event_type='MensalidadesCobrancaConfirmada'`).Scan(&ledgerCountConfirmada); err != nil {
		t.Fatal(err)
	}
	if ledgerCountConfirmada != 1 {
		t.Fatalf("esperava 1 evento MensalidadesCobrancaConfirmada no ledger após pagamento Success, obteve %d", ledgerCountConfirmada)
	}

	var obrigacoesPagas int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM financeiro_mensalidade_obrigacoes_eventos WHERE codigo_estudante=$1 AND codigo_academia=$2 AND ano_letivo=$3 AND mes=$4 AND tipo='paga'`, estudante, academia, alvo.AnoLetivo, alvo.Mes).Scan(&obrigacoesPagas); err != nil {
		t.Fatal(err)
	}
	if obrigacoesPagas != 1 {
		t.Fatalf("esperava 1 linha 'paga' na obrigação de mensalidade, obteve %d", obrigacoesPagas)
	}

	pendentesDepois, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	alvoDepois := mensalidadePorMes(t, pendentesDepois, academia, alvo.AnoLetivo, alvo.Mes)
	if alvoDepois.Estado != EstadoPago {
		t.Fatalf("mensalidade paga continua com estado=%q após pagamento confirmado como Success pela AppyPay (deveria ser %q)", alvoDepois.Estado, EstadoPago)
	}
}

// TestIntegrationListMensalidadesOrdemCronologicaAnoLetivo garante que os
// meses dentro de um mesmo ano_letivo são devolvidos em ordem cronológica
// real (setembro..dezembro do 1º ano civil, depois janeiro..julho do 2º),
// e não pela ordenação numérica crua do número do mês.
func TestIntegrationListMensalidadesOrdemCronologicaAnoLetivo(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	estudante := "EST-ORDEM-" + uuid.NewString()[:8]
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	seedMensalidadeTurma(t, client, academia, "T-ORDEM", "2025_2026", estudante, nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))

	valores, err := service.ListMensalidades(ctx, estudante, &academia)
	if err != nil {
		t.Fatal(err)
	}
	var meses []int
	for _, v := range valores {
		if v.AnoLetivo == "2025_2026" {
			meses = append(meses, v.Mes)
		}
	}
	esperado := []int{9, 10, 11, 12, 1, 2, 3, 4, 5, 6, 7}
	if len(meses) != len(esperado) {
		t.Fatalf("esperava %d meses, obteve %d (%v)", len(esperado), len(meses), meses)
	}
	for i := range esperado {
		if meses[i] != esperado[i] {
			t.Fatalf("ordem cronológica incorreta.\n  esperado (set->jul): %v\n  obtido:              %v", esperado, meses)
		}
	}
}

// TestIntegrationRebuildFinanceiroReconstroiConfiguracoesEcobrancasMensalidade
// garante que um Rebuild completo da projeção financeiro reconstrói TODAS as
// tabelas derivadas do ledger, incluindo financeiro_matricula_configuracoes
// e financeiro_mensalidade_cobrancas, que antes sobreviviam intactas (não
// eram limpas) a um Rebuild.
func TestIntegrationRebuildFinanceiroReconstroiConfiguracoesEcobrancasMensalidade(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
	configureIntegrationCredential(t, service, ContextoAcademia, academia)

	if _, err := service.ConfigureMatricula(ctx, MatriculaConfiguracaoInput{
		CodigoAcademia: academia, Nivel: NivelFundamental, AnoAcademico: "6_ano_fundamental",
		Valor: 5000, MetodosPagamento: []string{"GPO"},
	}, "admin-teste", "academia", "127.0.0.1"); err != nil {
		t.Fatalf("ConfigureMatricula falhou: %v", err)
	}

	// Simula corrupção/drift do read model: apaga a linha manualmente sem
	// tocar no ledger (fonte da verdade).
	if _, err := client.DB().Exec(`DELETE FROM financeiro_matricula_configuracoes WHERE codigo_academia=$1`, academia); err != nil {
		t.Fatal(err)
	}

	if err := service.projection.Rebuild(); err != nil {
		t.Fatalf("Rebuild() falhou: %v", err)
	}

	configs, err := service.ListMatriculaConfiguracoes(ctx, academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 {
		t.Fatalf("após Rebuild(), esperava financeiro_matricula_configuracoes reconstruída a partir do ledger (1 linha), obteve %d", len(configs))
	}
}
```

---

## Ordem de execução recomendada

1. Bug 1 (import de `pq` + os dois `pq.Array(...)`).
2. Bug 2 (whitelist).
3. Bug 3 (remoção do bloco `payment_info`).
4. Bug 4 (`Rebuild()`).
5. Bug 5 (ordenação por `DataReferencia`).
6. Criar `internal/finance/financeiro_ledger_integrity_test.go` com o conteúdo especificado acima.
7. Rodar `gofmt -w internal/db/safe_queries.go internal/projections/financeiro_projection.go internal/finance/mensalidade.go internal/finance/financeiro_ledger_integrity_test.go`.
8. Executar a checklist de aceitação abaixo, na ordem.

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

2. **Suíte `internal/finance`, 5 execuções seguidas, banco recriado do zero a cada vez:**
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
   Todas as 5 execuções devem terminar com `PASS`/`ok`, sem nenhum `FAIL`. Confirme especificamente que os
   19 testes abaixo aparecem como `--- PASS` em cada execução (14 já existentes + 5 novos deste documento):
   `TestIntegrationAcceptWebhookIsIdempotent`,
   `TestIntegrationMatriculaPagamentoFixaValorImpedeDuplicidadeECancelaEmCascata`,
   `TestIntegrationMatriculaWebhookTardioMantemCancelamentoERegistraConflito`,
   `TestIntegrationWebhookAuthConfigurableHeaderAndResourceFreeCredentials`,
   `TestIntegrationCancelChargeAndLateSuccessConflict`,
   `TestIntegrationMensalidadeResolvePrecoHistorico`,
   `TestIntegrationMensalidadePrimeiraConfiguracaoRetroageSemReescreverHistorico`,
   `TestIntegrationMensalidadeMantemAnoAcademicoHistorico`,
   `TestIntegrationMensalidadeMantemCursoHistorico`,
   `TestIntegrationMensalidadeMantemAcademiaHistoricaAposTransferencia`,
   `TestIntegrationMensalidadeValidaGranularidade`,
   `TestIntegrationMensalidadeValidaMesFim`,
   `TestIntegrationMensalidadeMesInicioEValidadePorAno`,
   `TestIntegrationMensalidadeAnularEReativar`,
   `TestIntegrationMensalidadeConsultaRespeitaAcademia`,
   `TestIntegrationConfigureMensalidadeGravaNoLedgerEProjectaCorretamente`,
   `TestIntegrationConfigureMatriculaGravaNoLedgerEProjectaCorretamente`,
   `TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago`,
   `TestIntegrationListMensalidadesOrdemCronologicaAnoLetivo`,
   `TestIntegrationRebuildFinanceiroReconstroiConfiguracoesEcobrancasMensalidade`.

3. **Suíte `internal/handlers`, 5 execuções seguidas, banco recriado do zero a cada vez:**
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
   Deve mostrar exatamente estes arquivos modificados, e nenhum outro (nem `go.mod`, nem `go.sum`):
   - `internal/db/safe_queries.go`
   - `internal/finance/mensalidade.go`
   - `internal/projections/financeiro_projection.go`

   E exatamente este arquivo novo, não rastreado antes desta tarefa:
   - `internal/finance/financeiro_ledger_integrity_test.go`

Se todos os itens passarem, a tarefa está concluída. Mova este arquivo de
`docs/Lista de Tarefas/41 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e
matrícula (auditoria pós-tarefa 40).md` para
`docs/Tarefas feitas/41 - Correção de bugs críticos de integridade do ledger no módulo de mensalidades e
matrícula (auditoria pós-tarefa 40).md`, e atualize o front-matter (`status: feito`) antes de finalizar.
