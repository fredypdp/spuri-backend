---
criado: 2026-08-26
origem: conversa com Fredy (Claude como orquestrador, Codex como executor)
status: pendente
tipo: correcao_de_bug_e_redesenho_de_contrato
depende_de: docs/Tarefas feitas/64 - Unificar cobrancas e pendencias_sem_cobranca numa unica lista paginada.md
gera_dependencia_para: tarefa companion no repositório spuripainel (frontend) — ver seção 0
---

# Novo modelo de estados de cobrança (`aguardando_pagamento`), correção do filtro `estado`/`tipo` na lista unificada de pagamentos, e suporte completo aos estados documentados pela AppyPay

## 0. Leia isto primeiro — sobre o seu ambiente (Codex) e sobre a tarefa companion do frontend

Você não tem `apt`, Docker nem `psql`. Não precisa disso aqui. Claude já validou esta correção inteira com **PostgreSQL 16 real e Go 1.24 real**, num sandbox próprio: aplicou cada mudança, recriou o banco do zero várias vezes, e rodou a suíte de testes inteira (`go build ./...`, `go vet ./...`, `go test ./...`) repetidamente até tudo ficar verde — incluindo os testes novos escritos especificamente para esta tarefa. Chegou a reverter deliberadamente a correção do bug de filtro só para confirmar que o teste novo falha sem ela (e falha exatamente com o sintoma que Fredy relatou), e revalidar depois de restaurar a correção. A seção 9 (Evidência de validação) tem os comandos exatos e a saída real de cada rodada.

**Sua validação usa só `go build ./...`, `go vet ./...`, `gofmt -l` e `go test ./...`** (os testes de integração pulam automaticamente sem `RUN_POSTGRES_INTEGRATION`, isso é esperado e não é um problema seu — mas como Claude já rodou tudo com PostgreSQL real, você não precisa se preocupar em validar o comportamento de tempo de execução, só que o código compila e os testes que não precisam de banco continuam passando).

**Existe uma tarefa companion no repositório `spuripainel` (frontend)** que precisa ser aplicada para o contrato da API ficar coerente dos dois lados — ela remove o campo `pendencia_sem_cobranca` do tipo `PagamentoResumo` e troca o rótulo/valor do filtro de estado "Pendente"/`Pending` por "Aguardando pagamento"/`aguardando_pagamento`. **Diferente da tarefa 64/65 (onde a ordem de deploy entre backend e frontend não importava), desta vez a ordem importa**: esta tarefa (backend) remove o campo `pendencia_sem_cobranca` da resposta JSON — um frontend não atualizado que ainda dependesse desse campo pararia de funcionar corretamente assim que este backend for implantado. Aplique e implante as duas tarefas juntas (mesmo commit/PR não é necessário, mas o deploy do backend não deve ficar em produção sozinho por muito tempo sem o frontend acompanhando). Avise Fredy explicitamente disso ao final (ver seção 12).

---

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento, nesta ordem. Todas as decisões de desenho já foram tomadas e validadas por Claude (diagnóstico completo com evidência de código, implementação testada com PostgreSQL 16 e Go 1.24 reais, incluindo reverter deliberadamente a correção do bug de filtro para confirmar que o teste de regressão a pega). Sua tarefa é mecânica: (1) aplicar os 5 diffs cirúrgicos em `internal/finance/mensalidade.go`, `internal/finance/matricula.go`, `internal/finance/appypay.go`, `internal/finance/pagamentos_unificado.go` e `internal/handlers/financeiro_handlers.go` descritos na seção 4; (2) aplicar os 7 diffs cirúrgicos nos arquivos de teste descritos na seção 5; (3) atualizar `Documentação da API.md` conforme os diffs da seção 6; (4) rodar cada item da seção "Checklist de validação" (seção 8) e reportar o resultado; (5) seguir o "Procedimento de conclusão" (seção 12). Não toque em nenhum arquivo ou lógica fora do escopo listado na seção 7 ("Fora de escopo"). Não é necessário PostgreSQL, Docker nem `psql` — os diffs já foram validados com esses três reais por Claude.

---

## 2. Contexto e diagnóstico

Fredy relatou três problemas relacionados, depois de confirmar que a tarefa 64 (unificação de `cobrancas` + `pendencias_sem_cobranca` numa lista só, `pagamentos`) está em produção e funcionando como planejado:

### 2.1. Modelo de estados confuso: "pendente" usado para dois significados diferentes

Hoje, quando a AppyPay devolve um estado ainda não resolvido para uma cobrança real recém-criada (`"Pending"`, ou `"Requested"` em alguns fluxos), o Spuri grava e devolve esse valor **verbatim**, em inglês, misturado com estados locais em português (`"solicitada"`, `"criada"`, `"falhada"`, `"cancelada"`). Ao mesmo tempo, uma pendência **sintética** (um mês de mensalidade que nunca teve nenhuma cobrança gerada) tem `status: "pendente"`, em português. Nenhuma cobrança real jamais usa literalmente `"pendente"` — mas o parecido visual entre "Pending" (cobrança real, tentada) e "pendente" (nenhuma cobrança, nunca tentada) é justamente a confusão que motivou a tarefa 64 a introduzir o campo booleano `pendencia_sem_cobranca` para desambiguar.

Fredy propôs um modelo mais simples: renomear o estado "cobrança gerada, ainda sem resolução do provedor" para `"aguardando_pagamento"`, e reservar `"pendente"` exclusivamente para quando **nenhuma** cobrança foi gerada nem está em andamento. Como esses dois casos passam a ter valores de `status` completamente distintos e nunca ambíguos entre si, **o campo `pendencia_sem_cobranca` se torna redundante** — é exatamente a suspeita que Fredy levantou, e Claude confirma: com o novo modelo, `status` sozinho já diz tudo que `pendencia_sem_cobranca` dizia, então o campo foi removido (ver seção 3.4).

### 2.2. Bug de filtro confirmado e reproduzido: `estado`/`tipo` ignorados pelas pendências sintéticas

Fredy relatou que, na página `/financas/pagamentos` do frontend, a requisição

```
GET /financeiro/cobrancas?contexto_tipo=academia&codigo_academia=LDA20263&estado=Failed&tipo=mensalidade&ano_letivo=2020_2021&mes=3&limit=30&offset=0
```

devolveu "todos os pagamentos" em vez de só os `Failed`.

**Causa raiz confirmada em código** (`internal/handlers/financeiro_handlers.go`, ambos os handlers `ListarCobrancasAppyPay` e `ConsultarCobrancasEstudante`): a computação das pendências sintéticas (`FinanceiroService.PendenciasSemCobranca`/`PendenciasSemCobrancaEstudante`) nunca olhava para os filtros `estado`/`tipo` antes de misturá-las com as cobranças reais em `ListarPagamentosUnificado`. Como toda pendência sintética tem sempre `status: "pendente"` e `origem: "mensalidade"`, e o filtro `estado` só era aplicado dentro do `WHERE` SQL de `ListCobrancas` (que só enxerga cobranças reais em `financeiro_cobrancas` — nunca as pendências, que são computadas à parte a partir de `MensalidadeMesView`), pedir `estado=Failed` filtrava corretamente as cobranças reais, mas **nunca impedia as pendências sintéticas de entrarem na resposta mesmo assim** — e com `ano_letivo`/`mes` informados (que ativam a computação de pendências), essas pendências podem facilmente ser a maioria (ou totalidade) da página, dando a impressão de "todos os pagamentos" em vez de só os `Failed`.

Claude reproduziu esse exato cenário num teste de integração novo (`TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas`, seção 5) — confirmou empiricamente que, **sem** a correção, o teste falha (a resposta filtrada por `estado=Failed` continua trazendo as pendências sintéticas do mês, com `total_geral` maior que zero mesmo sem nenhuma cobrança `Failed` existir); **com** a correção, o teste passa (resposta vazia, exatamente o comportamento correto).

### 2.3. Suporte incompleto aos estados documentados pela AppyPay — três bugs adicionais encontrados

A `AppyPay Documentação.md` já espelhada no repositório (`docs/Parceiros e integrações/`) documenta seis estados possíveis para uma cobrança: `Requested`, `Pending`, `Success`, `Failed`, `Cancelled`, `Expired`. Investigando cada ponto do código que classifica um status como terminal ou não, Claude encontrou **três bugs concretos de suporte incompleto**, todos além do escopo do que Fredy pediu explicitamente mas diretamente implicados pelo pedido de "garanta que o sistema tem suporte a todos os estados":

1. **`isTerminalChargeStatus` (`internal/finance/appypay.go`) só reconhecia `"cancelada"`, `"falhada"` e `"Success"`** — uma cobrança com o estado bruto `"Failed"`, `"Cancelled"` ou `"Expired"` (devolvidos pela própria AppyPay) nunca era tratada como terminal. Consequência real: `CancelCharge` não bloqueava uma segunda tentativa de cancelamento sobre uma cobrança que já tinha `"Failed"` genuíno, podendo sobrescrever esse status com `"cancelada"` e perder a razão real da falha.

2. **O mesmo gap, duplicado em SQL puro, em QUATRO funções**: `mensalidadeTemCobrancaAberta`, `cancelOpenMensalidadeCharges` (`mensalidade.go`) e `matriculaTemCobrancaAberta`, `CancelarCobrancaMatriculaAberta` (`matricula.go`) — todas usavam uma lista `NOT IN (...)` incompleta para decidir se uma cobrança está "em aberto" (bloqueando uma nova tentativa de pagamento do mesmo mês/matrícula). **Bug de produção confirmado**: um estudante cuja cobrança de mensalidade recebeu `status: "Failed"` da AppyPay ficava **permanentemente bloqueado** de tentar pagar de novo aquele mês — a cobrança "Failed" nunca era reconhecida como resolvida, então `mensalidadeTemCobrancaAberta` continuava dizendo que havia uma cobrança em aberto, para sempre. Claude reproduziu isso também num teste de integração novo (`TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa`, seção 5) — confirma que uma segunda tentativa de pagamento é aceita corretamente depois da correção.

3. **`AcceptWebhook` só refletia a cobrança em caso de sucesso** — um webhook da AppyPay avisando que uma referência REF expirou (`"Expired"`) ou que um GPO foi recusado (`"Failed"`) era gravado só como auditoria (`WebhookAppyPayRecebido`), mas nunca atualizava `financeiro_cobrancas`. A cobrança ficava presa em `aguardando_pagamento` até alguém consultá-la manualmente (`GET /financeiro/appypay/cobrancas/:id`), mesmo a AppyPay já tendo avisado proativamente que ela tinha terminado. Corrigido generalizando `AcceptWebhook` para refletir qualquer estado reportado — não só sucesso — com uma guarda para nunca sobrescrever uma cobrança que já chegou a um estado terminal.

---
## 3. Desenho da solução e decisões de projeto

### 3.1. `EstadoCobrancaAguardandoPagamento = "aguardando_pagamento"` — o novo estado canônico

Constante nova em `internal/finance/mensalidade.go`, ao lado de `EstadoPendente`. Representa "esta cobrança foi gerada/tentada junto à AppyPay, mas ainda não foi resolvida" — cobre, de forma unificada:

- Os estados locais que o Spuri já gravava antes de qualquer resposta do provedor: `"solicitada"` (gravado antes da chamada HTTP), `"criada"` (fallback usado quando a AppyPay responde 2xx sem nenhum campo de status no corpo).
- Os estados brutos que a própria AppyPay documenta para essa mesma fase: `"Requested"`, `"Pending"`.

Deliberadamente **distinto** de `EstadoPendente` (`"pendente"`), que continua reservado exclusivamente para uma pendência sintética (nenhuma cobrança gerada nem tentada) — uma cobrança real nunca usa `"pendente"` como seu status, nem antes nem depois desta tarefa. É essa garantia (já verdadeira antes desta tarefa, e que continua verdadeira depois) que permite ao `status` sozinho dizer se existe ou não uma cobrança real por trás de um item — sem precisar de nenhum campo booleano adicional (ver 3.4).

**Por que não renomear `Success`/`Failed`/`Cancelled`/`Expired` também?** Fredy só pediu a mudança para o caso "pending derivado da AppyPay depois de um pagamento ter sido feito" — os outros quatro estados já são inequívocos (nenhum deles jamais colide com `"pendente"`) e renomeá-los sem necessidade aumentaria o escopo e o risco sem nenhum ganho de clareza correspondente. Eles continuam exatamente como a AppyPay os documenta.

### 3.2. `normalizeChargeStatus` — tradução única, idempotente, aplicada em todo ponto de leitura E escrita

Função nova em `appypay.go` que traduz qualquer um dos cinco valores acima (`"solicitada"`, `"criada"`, `"Requested"`, `"Pending"`, e o próprio `"aguardando_pagamento"`) para `EstadoCobrancaAguardandoPagamento`, e devolve qualquer outro valor inalterado. Entrada vazia devolve vazia — quem decide o fallback apropriado é o chamador, porque o fallback correto depende do contexto (ver 3.3).

**Por que a normalização acontece nos dois lados (escrita E leitura), e não só na escrita:** o ledger (`spuri_ledger`) é append-only — eventos já gravados antes desta tarefa continuam existindo com os valores antigos (`"Pending"`, `"Requested"`, `"solicitada"`, `"criada"`) para sempre; não há como "corrigir" retroativamente um evento já gravado. Se a normalização só acontecesse na escrita (só cobranças novas, criadas depois do deploy, ganhassem o novo nome), uma cobrança antiga ainda não resolvida no momento do deploy ficaria mostrando o valor antigo indefinidamente até a próxima escrita naquele registro (próxima consulta ou webhook) — inconsistente com o que Fredy pediu ("passe a ser **gravado e lido** como aguardando pagamento"). Por isso `normalizeChargeStatus` também é aplicada em todo ponto de **leitura**: `loadCharge` (que cobre `consultCharge`, `CancelCharge`, `AcceptWebhook`, e os retornos idempotentes de charge/QR Code já existente) e `scanCobrancaResumo` (usada por `ListCobrancas`/`ListCobrancasEstudante`, a base da lista unificada de pagamentos). Por ser idempotente, aplicar a mesma função tanto sobre um valor bruto recém-chegado da AppyPay quanto sobre um valor já gravado (histórico ou já canônico) é sempre seguro.

### 3.3. `estadosCobrancaEquivalentes` — o filtro `estado=aguardando_pagamento` também precisa encontrar cobranças antigas

A normalização de leitura da seção 3.2 resolve a **exibição**, mas não o **filtro SQL**: `ListCobrancas`/`ListCobrancasEstudante` filtram com `payload->>'status' = ANY($1)`, uma comparação direta contra o valor bruto gravado no banco. Sem tratamento adicional, filtrar por `estado=aguardando_pagamento` só encontraria cobranças criadas **depois** desta tarefa (que já gravam o valor canônico) — escondendo qualquer cobrança mais antiga que ainda esteja nesse estado, uma inconsistência com o que a mesma listagem mostra sem filtro nenhum (que já normaliza na leitura).

`estadosCobrancaEquivalentes` expande, antes de montar a cláusula SQL, um filtro `estado=aguardando_pagamento` para o conjunto completo `["aguardando_pagamento", "Pending", "Requested", "solicitada", "criada"]`. Qualquer outro valor de filtro passa inalterado. Claude validou essa equivalência com um teste de integração que insere uma cobrança diretamente no banco com o status bruto histórico `"criada"` (simulando uma cobrança anterior a esta tarefa) e confirma que ela aparece corretamente como `"aguardando_pagamento"` na leitura **e** é encontrada pelo filtro `estado=aguardando_pagamento` (seção 5, `TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado` estendido).

### 3.4. Remoção de `PendenciaSemCobranca`/`pendencia_sem_cobranca`

Confirmando a suspeita de Fredy: com o novo modelo, `status` sozinho já desambigua os dois casos que o campo booleano existia para resolver — `status == "pendente"` sempre e só significa "pendência sintética, nenhuma cobrança gerada"; qualquer outro valor sempre significa "cobrança real". O campo foi removido de `PagamentoResumo` (`internal/finance/pagamentos_unificado.go`) e de todo lugar que o lia (dois handlers, `Documentação da API.md`, e a tarefa companion remove o equivalente no frontend). `PagamentoResumo` continua existindo como tipo (hoje idêntico a `CobrancaResumo`, ver comentário atualizado no código) — mantendo a camada de composição caso volte a precisar crescer no futuro, sem forçar uma renomeação maior agora.

### 3.5. `DeveIncluirPendenciasSemCobranca(estados, origens []string) bool` — a correção do bug de filtro

Função nova em `pagamentos_unificado.go`, chamada pelos dois handlers antes de computar pendências sintéticas: se `estados` foi informado e não contém `"pendente"`, ou se `origens` foi informado e não contém `"mensalidade"`, a computação é pulada inteiramente (nem chama `PendenciasSemCobranca`/`PendenciasSemCobrancaEstudante`) — economizando também a consulta desnecessária ao banco. Sem nenhum filtro informado em qualquer um dos dois argumentos, o comportamento continua exatamente como antes (pendências sempre incluídas quando o escopo de turma/curso/ano permite computá-las).

### 3.6. Estados terminais completos, e `AcceptWebhook` generalizado

`isTerminalChargeStatus` passa a reconhecer, além de `"cancelada"`/`"falhada"`/`Success` (case-insensitive), também `Failed`, `Cancelled` e `Expired` (case-insensitive) — os quatro estados terminais que a própria AppyPay documenta. A mesma lista, em minúsculas, vira a constante compartilhada `chargeAbertaStatusExcluidos` em `mensalidade.go`, usada pelas quatro consultas SQL "está em aberto" (`mensalidadeTemCobrancaAberta`, `cancelOpenMensalidadeCharges`, `matriculaTemCobrancaAberta`, `CancelarCobrancaMatriculaAberta`) — essas não podem chamar a função Go diretamente (comparam direto na query SQL), então precisam da mesma lista mantida manualmente em sincronia; o comentário no código deixa isso explícito para quem tocar nessas listas no futuro.

`AcceptWebhook` (`appypay.go`) generalizado: antes só atualizava a cobrança quando o webhook reportava sucesso; agora reflete qualquer status reportado (via `normalizeChargeStatus`), com uma guarda — só atualiza quando o novo status é sucesso, **ou** quando a cobrança ainda não está num estado terminal. Isso evita que um webhook atrasado e não-bem-sucedido sobrescreva uma cobrança que já foi resolvida por outro caminho (ex.: já paga, ou já cancelada localmente) — o tratamento de conflito pós-cancelamento já existente (sucesso tardio depois de cancelamento local) continua funcionando exatamente como antes, sem nenhuma mudança de comportamento nesse caso específico.

---
## 4. Diffs exatos — arquivos de código (5 arquivos)

Cada bloco abaixo é um diff unificado real, gerado por Claude comparando um clone limpo de `main` com o estado já validado no seu próprio sandbox (PostgreSQL 16 + Go 1.24 reais). Aplique cada hunk (`@@ ... @@`) exatamente: remova as linhas que começam com `-`, adicione as linhas que começam com `+`, mantendo as linhas de contexto (sem prefixo) como estão. Não altere nada fora dos hunks mostrados.

```diff
==========================================
FILE: internal/finance/mensalidade.go
==========================================
--- a/internal/finance/mensalidade.go
+++ b/internal/finance/mensalidade.go
@@ -23,12 +23,33 @@
 	NivelMedio       = "medio"
 	NivelSuperior    = "superior"
 
 	EstadoPendente = "pendente"
 	EstadoPago     = "pago"
 	EstadoAnulado  = "anulado"
+
+	// EstadoCobrancaAguardandoPagamento é o estado canônico de uma cobrança
+	// REAL (financeiro_cobrancas) que já foi gerada/tentada junto à AppyPay
+	// mas ainda não foi resolvida — ver normalizeChargeStatus em appypay.go
+	// para a tradução completa (cobre os estados locais intermediários
+	// "solicitada"/"criada" e os estados brutos "Requested"/"Pending" que a
+	// própria AppyPay devolve nesta fase, ambos documentados em
+	// docs/Parceiros e integrações/AppyPay Documentação.md).
+	//
+	// Deliberadamente DISTINTO de EstadoPendente ("pendente"): EstadoPendente
+	// é reservado exclusivamente para uma OBRIGAÇÃO de mensalidade (ou uma
+	// pendência sintética em PagamentoResumo, ver pagamentos_unificado.go)
+	// que NUNCA teve nenhuma cobrança gerada nem tentada — uma cobrança real
+	// nunca usa "pendente" como seu status; assim que qualquer cobrança é
+	// gerada (mesmo antes de qualquer resposta do provedor), o status passa
+	// a ser EstadoCobrancaAguardandoPagamento até resolver para um estado
+	// terminal (pago, falhado, cancelado ou expirado). Essa separação é o
+	// que permite ao status, sozinho, dizer se existe ou não uma cobrança
+	// real por trás de um item da lista unificada de pagamentos — sem
+	// precisar de nenhum campo booleano adicional.
+	EstadoCobrancaAguardandoPagamento = "aguardando_pagamento"
 )
 
 type MensalidadeConfiguracaoInput struct {
 	CodigoAcademia   string   `json:"codigo_academia"`
 	Nivel            string   `json:"nivel"`
 	AnoAcademico     string   `json:"ano_academico"`
@@ -792,26 +813,44 @@
 }
 
 func (s *Service) MetodosPagamentoMensalidade(ctx context.Context, academia string) ([]string, error) {
 	return s.metodosPagamentoMensalidade(ctx, academia)
 }
 
+// chargeAbertaStatusExcluidos é a lista (em minúsculas) de todo status
+// TERMINAL que uma cobrança real pode ter — usada para excluir cobranças
+// "em aberto" nas consultas SQL diretas abaixo e em matriculaTemCobrancaAberta/
+// CancelarCobrancaMatriculaAberta (matricula.go). Precisa ficar em sincronia
+// manual com isTerminalChargeStatus (appypay.go): as duas listam exatamente
+// os mesmos estados terminais, mas isTerminalChargeStatus não pode ser
+// chamada aqui porque estas são consultas SQL, não Go, sobre linhas que
+// ainda não foram carregadas em memória. Cobre tanto os estados locais
+// ("cancelada", "falhada") quanto os quatro estados terminais que a própria
+// AppyPay documenta e devolve verbatim (Success, Failed, Cancelled, Expired
+// — ver docs/Parceiros e integrações/AppyPay Documentação.md): antes desta
+// correção, uma cobrança com status bruto "Failed"/"Cancelled"/"Expired" da
+// AppyPay nunca entrava nesta lista e por isso ficava "presa" como em
+// aberto para sempre — bloqueando indefinidamente uma nova tentativa de
+// pagamento do mesmo mês/matrícula mesmo depois de a cobrança anterior já
+// ter definitivamente falhado no provedor.
+const chargeAbertaStatusExcluidos = `'success','cancelada','falhada','failed','cancelled','expired'`
+
 func (s *Service) mensalidadeTemCobrancaAberta(ctx context.Context, estudante, academia, ano string, mes int) (bool, error) {
 	var exists bool
 	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS (
 		SELECT 1 FROM financeiro_mensalidade_cobrancas m JOIN financeiro_cobrancas c ON c.id=m.charge_id
 		WHERE m.codigo_estudante=$1 AND m.codigo_academia=$2 AND m.ano_letivo=$3 AND m.mes=$4
-		AND lower(COALESCE(c.payload->>'status','')) NOT IN ('success','cancelada','falhada')
+		AND lower(COALESCE(c.payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)
 	)`, estudante, academia, ano, mes).Scan(&exists)
 	return exists, err
 }
 
 func (s *Service) cancelOpenMensalidadeCharges(ctx context.Context, estudante, academia, ano string, mes int, actorID, actorType, ip string) error {
 	rows, err := s.client.DB().QueryContext(ctx, `SELECT c.id::text FROM financeiro_mensalidade_cobrancas m JOIN financeiro_cobrancas c ON c.id=m.charge_id
 		WHERE m.codigo_estudante=$1 AND m.codigo_academia=$2 AND m.ano_letivo=$3 AND m.mes=$4
-		AND lower(COALESCE(c.payload->>'status','')) NOT IN ('success','cancelada','falhada')`, estudante, academia, ano, mes)
+		AND lower(COALESCE(c.payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)`, estudante, academia, ano, mes)
 	if err != nil {
 		return err
 	}
 	defer rows.Close()
 	for rows.Next() {
 		var id string
==========================================
FILE: internal/finance/matricula.go
==========================================
--- a/internal/finance/matricula.go
+++ b/internal/finance/matricula.go
@@ -230,13 +230,13 @@
 		return MatriculaPagamentoView{}, err
 	}
 	return MatriculaPagamentoView{Charge: QRCodeResult{ChargeResult: charge}}, nil
 }
 func (s *Service) matriculaTemCobrancaAberta(ctx context.Context, codigo string) (bool, error) {
 	var ok bool
-	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_cobrancas WHERE payload->>'codigo_solicitacao'=$1 AND COALESCE(payload->>'status','') NOT IN ('Success','success','cancelada','falhada'))`, codigo).Scan(&ok)
+	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_cobrancas WHERE payload->>'codigo_solicitacao'=$1 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`))`, codigo).Scan(&ok)
 	return ok, err
 }
 
 // CodigoSolicitacaoDaCobranca identifies an enrollment charge without exposing
 // applicant details to AppyPay-facing code.
 func (s *Service) CodigoSolicitacaoDaCobranca(ctx context.Context, identifier string) (string, error) {
@@ -245,13 +245,13 @@
 		return "", err
 	}
 	codigo, _ := row.Payload["codigo_solicitacao"].(string)
 	return strings.TrimSpace(codigo), nil
 }
 func (s *Service) CancelarCobrancaMatriculaAberta(ctx context.Context, codigo, motivo, actorID, actorType, ip string) error {
-	rows, err := s.client.DB().QueryContext(ctx, `SELECT id::text,codigo_academia FROM financeiro_cobrancas WHERE payload->>'codigo_solicitacao'=$1 AND COALESCE(payload->>'status','') NOT IN ('Success','success','cancelada','falhada')`, codigo)
+	rows, err := s.client.DB().QueryContext(ctx, `SELECT id::text,codigo_academia FROM financeiro_cobrancas WHERE payload->>'codigo_solicitacao'=$1 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)`, codigo)
 	if err != nil {
 		return err
 	}
 	defer rows.Close()
 	for rows.Next() {
 		var id, academia string
==========================================
FILE: internal/finance/appypay.go
==========================================
--- a/internal/finance/appypay.go
+++ b/internal/finance/appypay.go
@@ -464,27 +464,30 @@
 		}
 		if errors.Is(err, ErrNotFound) {
 			return ChargeResult{}, ErrConflict
 		}
 		return ChargeResult{}, err
 	}
-	payload := chargePayload(id, in, "", "solicitada", nil)
+	payload := chargePayload(id, in, "", EstadoCobrancaAguardandoPagamento, nil)
 	if err = s.record(ctx, id, "CobrancaAppyPaySolicitada", payload, actorID, actorType, ip); err != nil {
 		_ = s.releaseChargeReservation(ctx, in.MerchantTransactionID, id)
 		return ChargeResult{}, err
 	}
 	providerBody := map[string]any{"amount": in.Amount, "currency": in.Currency, "description": in.Description, "merchantTransactionId": in.MerchantTransactionID, "paymentMethod": method, "paymentInfo": in.PaymentInfo, "options": in.Options, "notify": in.Notify}
 	response, err := s.callJSON(ctx, credential, http.MethodPost, "/charges", providerBody, in.Async)
 	if err != nil {
 		_ = s.record(ctx, id, "CobrancaAppyPayFalhou", chargePayload(id, in, "", "falhada", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
 		return ChargeResult{ID: id, MerchantTransactionID: in.MerchantTransactionID, Status: "falhada"}, err
 	}
 	providerID := responseID(response)
-	status := responseStatus(response)
+	status := normalizeChargeStatus(responseStatus(response))
 	if status == "" {
-		status = "criada"
+		// A AppyPay respondeu 2xx sem nenhum campo de status no corpo — a
+		// cobrança foi aceita mas ainda não temos nenhuma informação sobre
+		// sua resolução, exatamente o significado de aguardando pagamento.
+		status = EstadoCobrancaAguardandoPagamento
 	}
 	if err = s.record(ctx, id, "CobrancaAppyPayCriada", chargePayload(id, in, providerID, status, response), actorID, actorType, ip); err != nil {
 		return ChargeResult{}, err
 	}
 	result := ChargeResult{ID: id, ProviderChargeID: providerID, MerchantTransactionID: in.MerchantTransactionID, Status: status, Response: response}
 	if isSuccessfulChargeStatus(status) {
@@ -540,25 +543,27 @@
 	if typ == "MULTIPLE" {
 		body["minAmount"] = *in.MinAmount
 		body["maxTransactions"] = *in.MaxTransactions
 		body["startDate"] = in.StartDate
 		body["endDate"] = in.EndDate
 	}
-	if err = s.record(ctx, id, "QRCodeAppyPaySolicitado", qrCodePayload(id, in, typ, "", "solicitada", nil), actorID, actorType, ip); err != nil {
+	if err = s.record(ctx, id, "QRCodeAppyPaySolicitado", qrCodePayload(id, in, typ, "", EstadoCobrancaAguardandoPagamento, nil), actorID, actorType, ip); err != nil {
 		_ = s.releaseChargeReservation(ctx, in.MerchantTransactionID, id)
 		return QRCodeResult{}, err
 	}
 	response, err := s.callJSON(ctx, cred, http.MethodPost, "/qr-codes", body, false)
 	if err != nil {
 		_ = s.record(ctx, id, "QRCodeAppyPayFalhou", qrCodePayload(id, in, typ, "", "falhada", map[string]any{"error": "provider_request_failed"}), actorID, actorType, ip)
 		return QRCodeResult{}, err
 	}
 	providerID := responseID(response)
-	status := responseStatus(response)
+	status := normalizeChargeStatus(responseStatus(response))
 	if status == "" {
-		status = "criada"
+		// Mesmo raciocínio de CreateCharge: 2xx sem status = aceito, ainda
+		// sem resolução conhecida.
+		status = EstadoCobrancaAguardandoPagamento
 	}
 	payload := qrCodePayload(id, in, typ, providerID, status, response)
 	if err = s.record(ctx, id, "QRCodeAppyPayGerado", payload, actorID, actorType, ip); err != nil {
 		return QRCodeResult{}, err
 	}
 	qr, _ := response["qrCodeArr"].(string)
@@ -659,13 +664,13 @@
 		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
 		args = append(args, academia)
 		i++
 	}
 	if len(estados) > 0 {
 		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
-		args = append(args, pq.Array(estados))
+		args = append(args, pq.Array(estadosCobrancaEquivalentes(estados)))
 		i++
 	}
 	if len(origens) > 0 {
 		clause, err := origensClause(origens)
 		if err != nil {
 			return nil, err
@@ -703,12 +708,41 @@
 	if err := rows.Err(); err != nil {
 		return nil, err
 	}
 	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
 }
 
+// estadosCobrancaEquivalentes expande os valores de filtro "estado"
+// informados pelo chamador para o conjunto completo de valores brutos que
+// scanCobrancaResumo/normalizeChargeStatus tratam como equivalentes, antes
+// de montar a cláusula SQL "payload->>'status' = ANY($n)" em ListCobrancas/
+// ListCobrancasEstudante. Necessário porque esse filtro SQL compara o valor
+// BRUTO gravado no ledger — e cobranças criadas antes desta tarefa ainda
+// têm, no payload de eventos já gravados (o ledger é append-only, ver
+// spuri_ledger), os valores antigos "solicitada"/"criada"/"Requested"/
+// "Pending" em vez do estado canônico EstadoCobrancaAguardandoPagamento.
+// Sem esta expansão, filtrar por estado=aguardando_pagamento encontraria só
+// as cobranças criadas DEPOIS do deploy desta tarefa, escondendo qualquer
+// cobrança antiga que ainda esteja nesse estado — inconsistente com o que
+// scanCobrancaResumo mostra ao ler a mesma linha (que já normaliza na
+// leitura). Qualquer outro valor de filtro (Success, Failed, Cancelled,
+// Expired, ou qualquer string não reconhecida, incluindo EstadoPendente)
+// passa inalterado — só "aguardando_pagamento" tem essa equivalência
+// histórica com valores brutos diferentes de si mesmo.
+func estadosCobrancaEquivalentes(estados []string) []string {
+	out := make([]string, 0, len(estados))
+	for _, estado := range estados {
+		if strings.EqualFold(strings.TrimSpace(estado), EstadoCobrancaAguardandoPagamento) {
+			out = append(out, EstadoCobrancaAguardandoPagamento, "Pending", "Requested", "solicitada", "criada")
+			continue
+		}
+		out = append(out, estado)
+	}
+	return out
+}
+
 // origensClause monta a cláusula SQL "AND (...)" que filtra
 // financeiro_cobrancas pelo tipo de cobrança derivado do payload
 // (mensalidade, matrícula ou avulsa) — a mesma derivação usada por
 // scanCobrancaResumo. Devolve "" (sem filtro) quando origens está vazio.
 // Extraída durante a tarefa 49 para ser compartilhada por ListCobrancas e
 // ListCobrancasEstudante e nunca divergir entre as duas.
@@ -747,13 +781,14 @@
 		return CobrancaResumo{}, err
 	}
 	var payload map[string]any
 	if err := json.Unmarshal(rawPayload, &payload); err != nil {
 		return CobrancaResumo{}, err
 	}
-	dto.Status, _ = payload["status"].(string)
+	rawStatus, _ := payload["status"].(string)
+	dto.Status = normalizeChargeStatus(rawStatus)
 	dto.Valor, _ = payload["amount"].(float64)
 	dto.Moeda, _ = payload["currency"].(string)
 	dto.Descricao, _ = payload["description"].(string)
 	dto.MetodoPagamento, _ = payload["payment_method"].(string)
 	if qrType, ok := payload["qr_code_type"].(string); ok && qrType != "" {
 		dto.MetodoPagamento = "GPO_QR"
@@ -806,13 +841,13 @@
 		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
 		args = append(args, *somenteAcademia)
 		i++
 	}
 	if len(estados) > 0 {
 		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
-		args = append(args, pq.Array(estados))
+		args = append(args, pq.Array(estadosCobrancaEquivalentes(estados)))
 		i++
 	}
 	if len(origens) > 0 {
 		clause, err := origensClause(origens)
 		if err != nil {
 			return nil, err
@@ -869,16 +904,79 @@
 }
 
 func isSuccessfulChargeStatus(status string) bool {
 	return strings.EqualFold(strings.TrimSpace(status), "Success")
 }
 
+// isTerminalChargeStatus reporta se uma cobrança real chegou a um estado do
+// qual nunca mais sai sozinha (não há mais nada que a AppyPay ou o Spuri
+// vão fazer para resolvê-la). Cobre os dois estados locais ("cancelada" —
+// cancelamento feito pelo Spuri; "falhada" — a própria chamada HTTP para a
+// AppyPay falhou, sem chegar a existir uma cobrança do lado do provedor) e
+// os quatro estados terminais documentados pela própria AppyPay e devolvidos
+// verbatim (ver docs/Parceiros e integrações/AppyPay Documentação.md):
+// Success (paga), Failed (recusada pelo processador), Cancelled (cancelada
+// do lado da AppyPay) e Expired (referência REF expirou sem pagamento).
+//
+// Antes desta correção só "cancelada"/"falhada"/Success eram reconhecidos:
+// uma cobrança devolvida pela AppyPay como Failed/Cancelled/Expired não era
+// terminal aos olhos desta função, o que tinha dois efeitos colaterais
+// reais — (1) CancelCharge não bloqueava uma segunda tentativa de
+// cancelamento sobre uma cobrança já resolvida, podendo sobrescrever um
+// status Failed genuíno com "cancelada", perdendo a razão real da falha; e
+// (2) o SQL equivalente (chargeAbertaStatusExcluidos, mensalidade.go)
+// tratava essa cobrança como "em aberto" para sempre. Precisa ficar
+// manualmente sincronizada com chargeAbertaStatusExcluidos.
 func isTerminalChargeStatus(status string) bool {
-	return strings.EqualFold(strings.TrimSpace(status), "cancelada") ||
-		strings.EqualFold(strings.TrimSpace(status), "falhada") ||
-		isSuccessfulChargeStatus(status)
+	trimmed := strings.TrimSpace(status)
+	switch {
+	case strings.EqualFold(trimmed, "cancelada"),
+		strings.EqualFold(trimmed, "falhada"),
+		strings.EqualFold(trimmed, "Failed"),
+		strings.EqualFold(trimmed, "Cancelled"),
+		strings.EqualFold(trimmed, "Expired"):
+		return true
+	default:
+		return isSuccessfulChargeStatus(trimmed)
+	}
+}
+
+// normalizeChargeStatus traduz o vocabulário histórico/bruto de status de
+// uma cobrança real para o estado canônico único
+// EstadoCobrancaAguardandoPagamento (mensalidade.go), sempre que o valor de
+// entrada representar "cobrança gerada/tentada junto à AppyPay, ainda sem
+// resolução": os estados locais intermediários que o Spuri gravava antes
+// desta tarefa ("solicitada", gravado antes de qualquer chamada ao
+// provedor; "criada", o fallback usado quando o provedor responde 2xx sem
+// nenhum campo de status) e os estados brutos que a própria AppyPay
+// documenta para esta mesma fase ("Requested" e "Pending" — ver docs/
+// Parceiros e integrações/AppyPay Documentação.md). Qualquer outro valor
+// (Success, Failed, Cancelled, Expired, ou o próprio
+// EstadoCobrancaAguardandoPagamento) é devolvido inalterado — a função é
+// idempotente e pode ser chamada tanto sobre um valor bruto recém-recebido
+// da AppyPay quanto sobre um valor já gravado (histórico ou canônico).
+//
+// Entrada vazia é devolvida vazia: normalizeChargeStatus nunca decide um
+// fallback por conta própria, porque o fallback correto depende do
+// contexto de quem chama (ex.: CreateCharge trata "" como uma cobrança
+// nova ainda sem informação = aguardando pagamento; já consultCharge trata
+// "" como "o provedor não devolveu nada desta vez, mantém o status
+// anterior").
+func normalizeChargeStatus(raw string) string {
+	trimmed := strings.TrimSpace(raw)
+	switch {
+	case trimmed == "":
+		return ""
+	case strings.EqualFold(trimmed, "Pending"),
+		strings.EqualFold(trimmed, "Requested"),
+		strings.EqualFold(trimmed, "solicitada"),
+		strings.EqualFold(trimmed, "criada"):
+		return EstadoCobrancaAguardandoPagamento
+	default:
+		return trimmed
+	}
 }
 
 // consultCharge is shared by normal consultation and cancellation's mandatory
 // pre-check. A late Success after local cancellation is preserved as a
 // reconciliation conflict and never changes the local cancelled status.
 func (s *Service) consultCharge(ctx context.Context, row chargeRow, actorID, actorType, ip string) (ChargeResult, error) {
@@ -891,14 +989,17 @@
 		path = "/charges?merchantTransactionId=" + url.QueryEscape(row.Merchant)
 	}
 	response, err := s.callJSON(ctx, cred, http.MethodGet, path, nil, false)
 	if err != nil {
 		return ChargeResult{}, err
 	}
-	status := responseStatus(response)
+	status := normalizeChargeStatus(responseStatus(response))
 	if status == "" {
+		// AppyPay não devolveu nenhum campo de status desta vez — mantém o
+		// status anterior (row.Status já vem normalizado por loadCharge) em
+		// vez de assumir um novo estado.
 		status = row.Status
 	}
 	previousResponse := row.Payload["response"]
 	payload := make(map[string]any, len(row.Payload)+3)
 	for key, value := range row.Payload {
 		payload[key] = value
@@ -1223,31 +1324,49 @@
 	}
 	data := map[string]any{"event_id": eventID, "metodo": metodo, "credential_id": owner.CredentialID.String(), "contexto_tipo": owner.ContextoTipo, "codigo_academia": owner.CodigoAcademia, "payload": sanitize(payload)}
 	if err = s.record(ctx, uuid.New(), "WebhookAppyPayRecebido", data, "appypay:webhook", "sistema", "webhook"); err != nil {
 		_, _ = s.client.DB().ExecContext(ctx, `DELETE FROM financeiro_webhooks_recebidos WHERE event_id=$1`, eventID)
 		return false, err
 	}
-	if isSuccessfulChargeStatus(responseStatus(payload)) {
-		if charge, loadErr := s.loadCharge(ctx, eventID); loadErr == nil && charge.Contexto == owner.ContextoTipo && charge.Academia == owner.CodigoAcademia {
+	// Reflete no read model qualquer estado que o webhook reporte — sucesso
+	// (Success) ou qualquer um dos outros três estados terminais que a
+	// própria AppyPay documenta (Failed, Cancelled, Expired). Antes desta
+	// correção só um webhook de sucesso atualizava a cobrança: um webhook
+	// avisando que uma referência REF expirou ou que um GPO foi recusado
+	// era gravado em WebhookAppyPayRecebido (acima) mas nunca refletia em
+	// financeiro_cobrancas, deixando a cobrança "presa" em
+	// aguardando_pagamento até alguém consultá-la manualmente.
+	if raw := responseStatus(payload); raw != "" {
+		normalized := normalizeChargeStatus(raw)
+		success := isSuccessfulChargeStatus(normalized)
+		if charge, loadErr := s.loadCharge(ctx, eventID); loadErr == nil && charge.Contexto == owner.ContextoTipo && charge.Academia == owner.CodigoAcademia &&
+			// Um webhook atrasado e não-bem-sucedido nunca sobrescreve uma
+			// cobrança que já chegou a um estado terminal (ex.: já paga, já
+			// cancelada) — só um sucesso tem tratamento de conflito próprio
+			// (abaixo) que pode correr por cima de um estado terminal local.
+			(success || !isTerminalChargeStatus(charge.Status)) {
 			updated := make(map[string]any, len(charge.Payload)+3)
 			for k, v := range charge.Payload {
 				updated[k] = v
 			}
-			updated["status"] = "Success"
+			updated["status"] = normalized
+			if success {
+				updated["status"] = "Success"
+			}
 			updated["provider_charge_id"] = first(responseID(payload), charge.ProviderID)
 			updated["response"] = sanitize(payload)
 			eventType := "CobrancaAppyPayConsultada"
-			if strings.EqualFold(charge.Status, "cancelada") {
+			if success && strings.EqualFold(charge.Status, "cancelada") {
 				// A provider may still settle a REF/GPO/QR after Spuri's local
 				// cancellation. Preserve cancellation and leave an explicit audit
 				// conflict for FPP reconciliation.
 				updated["status"] = "cancelada"
 				updated["provider_status"] = "Success"
 				eventType = "CobrancaAppyPayConflitoPosCancelamento"
 			}
-			if s.record(ctx, charge.ID, eventType, updated, "appypay:webhook", "sistema", "webhook") == nil && eventType == "CobrancaAppyPayConsultada" {
+			if s.record(ctx, charge.ID, eventType, updated, "appypay:webhook", "sistema", "webhook") == nil && success && eventType == "CobrancaAppyPayConsultada" {
 				_ = s.confirmMensalidadeCharge(ctx, charge.ID, "appypay:webhook", "sistema", "webhook")
 			}
 		}
 	}
 	return true, nil
 }
@@ -1265,13 +1384,14 @@
 	if err != nil {
 		return r, fmt.Errorf("%w: cobrança não encontrada", ErrNotFound)
 	}
 	if err = json.Unmarshal(raw, &r.Payload); err != nil {
 		return r, err
 	}
-	r.Status, _ = r.Payload["status"].(string)
+	rawStatus, _ := r.Payload["status"].(string)
+	r.Status = normalizeChargeStatus(rawStatus)
 	return r, nil
 }
 
 func (s *Service) reserveCharge(ctx context.Context, merchant string, chargeID uuid.UUID) (bool, error) {
 	res, err := s.client.DB().ExecContext(ctx, `INSERT INTO financeiro_cobrancas_reservas (merchant_transaction_id,charge_id) VALUES ($1,$2) ON CONFLICT (merchant_transaction_id) DO NOTHING`, merchant, chargeID)
 	if err != nil {
==========================================
FILE: internal/finance/pagamentos_unificado.go
==========================================
--- a/internal/finance/pagamentos_unificado.go
+++ b/internal/finance/pagamentos_unificado.go
@@ -101,30 +101,37 @@
 // nenhum, só precisa ser constante entre chamadas para que o mesmo
 // (academia, estudante, ano_letivo, mes) sempre produza o mesmo id
 // sintético em pendenciaParaPagamentoResumo. Gerado uma vez com uuid.New()
 // e fixado aqui; não tem nenhum significado além de ser constante.
 var pendenciaNamespace = uuid.MustParse("c8ede658-7791-4abf-a329-164fba114d8f")
 
-// PagamentoResumo é CobrancaResumo mais um único campo adicional,
-// PendenciaSemCobranca — ver ListarPagamentosUnificado para o porquê da
-// unificação. Quando PendenciaSemCobranca é true, o item foi sintetizado a
-// partir de uma pendência de mensalidade sem NENHUMA cobrança criada (nem
-// tentada) — não existe uma linha real em financeiro_cobrancas por trás
-// dele. Quando é false, é uma cobrança real, com todos os campos vindos de
-// financeiro_cobrancas exatamente como sempre foi.
-//
-// Status == "pendente" pode vir de QUALQUER um dos dois casos: uma
-// cobrança real cujo status ainda não foi resolvido pelo provedor
-// (PendenciaSemCobranca=false — o pagamento foi tentado e a AppyPay
-// retornou um estado não-terminal), ou uma pendência sintética
-// (PendenciaSemCobranca=true — não tem cobrança gerada). O campo
-// PendenciaSemCobranca é o que desambigua os dois — sem ele, os dois casos
-// seriam indistinguíveis só pelo status.
+// PagamentoResumo é a unidade da lista unificada de pagamentos — ver
+// ListarPagamentosUnificado para o porquê da unificação. Hoje é idêntico a
+// CobrancaResumo (o embedding existe para permitir voltar a crescer com
+// campos exclusivos da composição, sem repetir todos os campos de
+// CobrancaResumo, se algum dia precisar).
+//
+// Existem dois casos possíveis por trás de cada item, e Status sozinho já
+// diz qual é, sem precisar de nenhum campo booleano adicional:
+//   - Status == EstadoPendente ("pendente"): pendência sintética — NENHUMA
+//     cobrança foi gerada nem tentada para este mês; não existe uma linha
+//     real em financeiro_cobrancas por trás dele (ver
+//     pendenciaParaPagamentoResumo).
+//   - Qualquer outro Status (incluindo
+//     EstadoCobrancaAguardandoPagamento, "aguardando_pagamento" —
+//     mensalidade.go): cobrança real, com todos os campos vindos de
+//     financeiro_cobrancas exatamente como sempre foi. Uma cobrança real
+//     NUNCA tem Status == EstadoPendente — assim que uma cobrança é
+//     gerada/tentada (mesmo antes de qualquer resposta do provedor), seu
+//     status passa a ser EstadoCobrancaAguardandoPagamento, nunca
+//     "pendente" (ver normalizeChargeStatus, appypay.go). É essa garantia
+//     que torna o campo PendenciaSemCobranca (existente antes desta
+//     tarefa) redundante: os dois casos já eram, e continuam sendo,
+//     completamente distinguíveis só pelo Status.
 type PagamentoResumo struct {
 	CobrancaResumo
-	PendenciaSemCobranca bool `json:"pendencia_sem_cobranca"`
 }
 
 // PagamentoListResult é o resultado paginado de ListarPagamentosUnificado —
 // mesmo papel de CobrancaListResult, para a lista já unificada. Total é o
 // total geral (pendências + cobranças reais) que casa com os filtros
 // aplicados — o mesmo significado que CobrancaListResult.Total sempre
@@ -161,16 +168,42 @@
 			Valor:           m.Valor,
 			Moeda:           "AOA",
 			Descricao:       fmt.Sprintf("Propinas %s: 1 mensalidade(s) — pendência sem cobrança gerada", m.CodigoAcademia),
 			CodigoEstudante: m.CodigoEstudante,
 			Mensalidades:    []MensalidadeSelecaoMes{{AnoLetivo: m.AnoLetivo, Mes: m.Mes}},
 		},
-		PendenciaSemCobranca: true,
 	}
 }
 
+// DeveIncluirPendenciasSemCobranca decide se a computação de pendências sem
+// cobrança (PendenciasSemCobranca/PendenciasSemCobrancaEstudante) deve
+// acontecer antes de montar a lista unificada — ver ListarPagamentosUnificado.
+// Uma pendência sem cobrança é SEMPRE origem "mensalidade" e SEMPRE
+// Status == EstadoPendente ("pendente"): se o chamador filtrou estado ou
+// tipo (origem) de um jeito que exclua explicitamente "pendente" ou
+// "mensalidade" respectivamente, incluir pendências no resultado
+// desrespeitaria o filtro pedido.
+//
+// Corrige um bug real: antes desta função existir, os dois handlers
+// (ListarCobrancasAppyPay, ConsultarCobrancasEstudante) computavam
+// pendências sem nunca olhar para estados/origens — filtrar por
+// estado=Failed continuava devolvendo todas as pendências sintéticas do
+// escopo pedido (todas "pendente", nunca "Failed"), porque nada impedia
+// isso. Sem nenhum filtro informado em qualquer um dos dois argumentos
+// (slice vazia), pendências continuam incluídas — mesmo comportamento de
+// sempre.
+func DeveIncluirPendenciasSemCobranca(estados, origens []string) bool {
+	if len(estados) > 0 && !contains(estados, EstadoPendente) {
+		return false
+	}
+	if len(origens) > 0 && !contains(origens, "mensalidade") {
+		return false
+	}
+	return true
+}
+
 // ListarPagamentosUnificado combina, numa única lista paginada, as
 // cobranças reais (buscarCobrancas) com as pendências de mensalidade sem
 // nenhuma cobrança (pendencias, já totalmente resolvidas pelo chamador via
 // PendenciasSemCobranca ou PendenciasSemCobrancaEstudante — nenhuma das
 // duas muda). pendencias pode ser nil/vazio (ex.: nenhum filtro de escopo
 // informado em GET /financeiro/cobrancas) — nesse caso o resultado é
@@ -243,13 +276,13 @@
 	res, err := buscarCobrancas(buscaLimit, offsetCobrancas)
 	if err != nil {
 		return nil, err
 	}
 	if limiteCobrancas > 0 {
 		for _, c := range res.Cobrancas {
-			itens = append(itens, PagamentoResumo{CobrancaResumo: c, PendenciaSemCobranca: false})
+			itens = append(itens, PagamentoResumo{CobrancaResumo: c})
 		}
 	}
 
 	return &PagamentoListResult{
 		Pagamentos: itens,
 		Total:      totalPendencias + res.Total,
==========================================
FILE: internal/handlers/financeiro_handlers.go
==========================================
--- a/internal/handlers/financeiro_handlers.go
+++ b/internal/handlers/financeiro_handlers.go
@@ -415,14 +415,21 @@
 	origens := c.QueryArray("tipo")
 	// pendências sem cobrança só são computadas quando pelo menos um dos
 	// quatro filtros de escopo (turma_id, curso_id, ano_academico,
 	// ano_letivo) é informado junto de codigo_academia — sem isso, a
 	// varredura seria sobre a academia inteira sem limite. mes (tarefa 60)
 	// só refina esse escopo, nunca o substitui. Ver finance.PendenciasSemCobranca.
+	//
+	// Além do escopo, uma pendência sintética só entra na lista se ela não
+	// for excluída pelos filtros estado/tipo — ver
+	// finance.DeveIncluirPendenciasSemCobranca para o porquê: toda
+	// pendência sintética é sempre estado="pendente" e tipo="mensalidade",
+	// então pedir estado=Failed (por exemplo) deve excluí-las, não trazê-las
+	// de qualquer forma.
 	var pendencias []finance.MensalidadeMesView
-	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
+	if (turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "") && finance.DeveIncluirPendenciasSemCobranca(estados, origens) {
 		pendencias, err = FinanceiroService.PendenciasSemCobranca(c.Request.Context(), academia, turmaID, cursoID, anoAcademico, anoLetivo, mes)
 		if err != nil {
 			financeError(c, err)
 			return
 		}
 		pendencias, err = FinanceiroService.FiltrarPendenciasComCobrancaRealVinculada(c.Request.Context(), pendencias)
@@ -502,25 +509,32 @@
 	anoLetivo := c.Query("ano_letivo")
 	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
 	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
 	estados := c.QueryArray("estado")
 	origens := c.QueryArray("tipo")
 	// pendências sem cobrança são sempre calculadas aqui (sem exigir nenhum
-	// filtro extra): esta consulta já está inerentemente delimitada a UM
-	// estudante, então não há o mesmo risco de varredura sem limite que
-	// existe em ListarCobrancasAppyPay. Ver
+	// filtro extra de escopo): esta consulta já está inerentemente
+	// delimitada a UM estudante, então não há o mesmo risco de varredura
+	// sem limite que existe em ListarCobrancasAppyPay. Ver
 	// finance.PendenciasSemCobrancaEstudante.
-	pendencias, err := FinanceiroService.PendenciasSemCobrancaEstudante(c.Request.Context(), codigo, somenteAcademia)
-	if err != nil {
-		financeError(c, err)
-		return
-	}
-	pendencias, err = FinanceiroService.FiltrarPendenciasComCobrancaRealVinculada(c.Request.Context(), pendencias)
-	if err != nil {
-		financeError(c, err)
-		return
+	//
+	// Mesmo assim, um filtro de estado/tipo que exclua explicitamente
+	// "pendente"/"mensalidade" deve excluir as pendências sintéticas do
+	// resultado — ver finance.DeveIncluirPendenciasSemCobranca.
+	var pendencias []finance.MensalidadeMesView
+	if finance.DeveIncluirPendenciasSemCobranca(estados, origens) {
+		pendencias, err = FinanceiroService.PendenciasSemCobrancaEstudante(c.Request.Context(), codigo, somenteAcademia)
+		if err != nil {
+			financeError(c, err)
+			return
+		}
+		pendencias, err = FinanceiroService.FiltrarPendenciasComCobrancaRealVinculada(c.Request.Context(), pendencias)
+		if err != nil {
+			financeError(c, err)
+			return
+		}
 	}
 	// mes não é exposto como parâmetro de query nesta rota ainda (só em
 	// GET /financeiro/cobrancas, tarefa 60) — passamos nil para manter o
 	// comportamento anterior inalterado aqui.
 	res, err := finance.ListarPagamentosUnificado(pendencias, func(limitCobrancas, offsetCobrancas int) (*finance.CobrancaListResult, error) {
 		return FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, estados, origens, turmaID, cursoID, anoAcademico, anoLetivo, nil, limitCobrancas, offsetCobrancas)
```

---

## 5. Diffs exatos — arquivos de teste (7 arquivos)

Mesmo formato da seção 4. Estes diffs cobrem: (a) ajuste mecânico dos testes existentes que quebrariam com a remoção de `PendenciaSemCobranca` (troca por uma checagem equivalente em `Status == EstadoPendente` — o stub de teste usado nesses arquivos nunca preenche `Status` para uma cobrança real, então a equivalência é exata); (b) os testes novos escritos para esta tarefa (`TestNormalizeChargeStatus`, `TestEstadosCobrancaEquivalentes`, extensão de `TestCancelChargeAuthorizationAndTerminalStatuses`, `TestDeveIncluirPendenciasSemCobranca`, `TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa`, `TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso`, `TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas`, e a extensão de `TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado` para cobrir a equivalência histórica da seção 3.3).

```diff
==========================================
FILE: internal/finance/appypay_test.go
==========================================
--- a/internal/finance/appypay_test.go
+++ b/internal/finance/appypay_test.go
@@ -70,17 +70,108 @@
 	if canCancelCharge(academy, "", "admin") {
 		t.Fatal("admin não pode cancelar cobrança de academia")
 	}
 	if !canCancelCharge(academy, "ACA1", "academia") || canCancelCharge(academy, "ACA2", "academia") {
 		t.Fatal("isolamento de cancelamento por academia inválido")
 	}
-	for _, status := range []string{"cancelada", "FALHADA", "Success", "SUCCESS"} {
+	// Antes desta tarefa só "cancelada"/"falhada"/"Success" eram
+	// reconhecidos como terminal — "Failed", "Cancelled" e "Expired"
+	// (documentados pela própria AppyPay) ficavam de fora, o que permitia
+	// re-cancelar uma cobrança já resolvida e mantinha uma "mensalidade
+	// aberta" presa para sempre (ver TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa).
+	for _, status := range []string{"cancelada", "FALHADA", "Success", "SUCCESS", "Failed", "FAILED", "Cancelled", "CANCELLED", "Expired", "EXPIRED"} {
 		if !isTerminalChargeStatus(status) {
 			t.Fatalf("estado terminal %q não foi reconhecido", status)
 		}
 	}
+	// aguardando_pagamento (e os valores brutos/locais que normalizeChargeStatus
+	// traduz para ele) é o único estado não-terminal possível para uma
+	// cobrança real — nunca deve ser tratado como terminal.
+	for _, status := range []string{EstadoCobrancaAguardandoPagamento, "Pending", "Requested", "solicitada", "criada", ""} {
+		if isTerminalChargeStatus(status) {
+			t.Fatalf("estado %q não deveria ser terminal", status)
+		}
+	}
+}
+
+// TestNormalizeChargeStatus cobre a tradução central do novo modelo de
+// estados: os valores locais intermediários ("solicitada", "criada") e os
+// valores brutos que a própria AppyPay documenta para "cobrança gerada,
+// ainda sem resolução" ("Requested", "Pending") devem virar o estado
+// canônico único EstadoCobrancaAguardandoPagamento — em qualquer
+// combinação de maiúsculas/minúsculas, já que a AppyPay não garante uma
+// caixa fixa. Qualquer outro valor (terminal ou já canônico) deve passar
+// inalterado — a função é idempotente. Entrada vazia continua vazia: quem
+// decide o fallback é o chamador (CreateCharge/CreateGPOQRCode tratam ""
+// como aguardando_pagamento; consultCharge tem preferido preservar o
+// status anterior).
+func TestNormalizeChargeStatus(t *testing.T) {
+	awaiting := map[string]bool{
+		"Pending": true, "pending": true, "PENDING": true,
+		"Requested": true, "requested": true,
+		"solicitada": true, "SOLICITADA": true,
+		"criada": true, "CRIADA": true,
+		EstadoCobrancaAguardandoPagamento: true,
+	}
+	for raw := range awaiting {
+		if got := normalizeChargeStatus(raw); got != EstadoCobrancaAguardandoPagamento {
+			t.Fatalf("normalizeChargeStatus(%q) = %q, esperava %q", raw, got, EstadoCobrancaAguardandoPagamento)
+		}
+	}
+	passthrough := []string{"Success", "Failed", "Cancelled", "Expired", "falhada", "cancelada", "algo-desconhecido"}
+	for _, raw := range passthrough {
+		if got := normalizeChargeStatus(raw); got != raw {
+			t.Fatalf("normalizeChargeStatus(%q) deveria devolver o valor inalterado, obteve %q", raw, got)
+		}
+	}
+	if got := normalizeChargeStatus(""); got != "" {
+		t.Fatalf("normalizeChargeStatus(\"\") deveria devolver \"\", obteve %q", got)
+	}
+	// Idempotência: aplicar duas vezes sobre o próprio resultado não muda
+	// nada — importante porque scanCobrancaResumo/loadCharge normalizam
+	// tanto valores brutos históricos quanto valores já canônicos.
+	for _, raw := range append(passthrough, EstadoCobrancaAguardandoPagamento) {
+		once := normalizeChargeStatus(raw)
+		twice := normalizeChargeStatus(once)
+		if once != twice {
+			t.Fatalf("normalizeChargeStatus não é idempotente para %q: 1a chamada=%q, 2a chamada=%q", raw, once, twice)
+		}
+	}
+}
+
+// TestEstadosCobrancaEquivalentes cobre a expansão do filtro estado=
+// aguardando_pagamento para o conjunto de valores brutos históricos
+// equivalentes (ver ListCobrancas/ListCobrancasEstudante) — sem essa
+// expansão, filtrar por esse novo estado canônico não encontraria nenhuma
+// cobrança criada antes desta tarefa (ainda gravada como "Pending",
+// "Requested", "solicitada" ou "criada" no payload do ledger, imutável).
+func TestEstadosCobrancaEquivalentes(t *testing.T) {
+	got := estadosCobrancaEquivalentes([]string{"aguardando_pagamento"})
+	esperado := map[string]bool{"aguardando_pagamento": true, "Pending": true, "Requested": true, "solicitada": true, "criada": true}
+	if len(got) != len(esperado) {
+		t.Fatalf("esperava %d valores equivalentes, obteve %d: %#v", len(esperado), len(got), got)
+	}
+	for _, v := range got {
+		if !esperado[v] {
+			t.Fatalf("valor inesperado na expansão: %q (lista completa: %#v)", v, got)
+		}
+	}
+	// Qualquer outro estado passa inalterado — não tem equivalência
+	// histórica com outros valores brutos.
+	for _, outros := range [][]string{{"Success"}, {"Failed"}, {"Cancelled"}, {"Expired"}, {"pendente"}} {
+		out := estadosCobrancaEquivalentes(outros)
+		if len(out) != 1 || out[0] != outros[0] {
+			t.Fatalf("esperava %v inalterado, obteve %v", outros, out)
+		}
+	}
+	// Uma lista com múltiplos estados só expande o que casa com
+	// aguardando_pagamento, preservando os demais.
+	misto := estadosCobrancaEquivalentes([]string{"Success", "aguardando_pagamento"})
+	if len(misto) != 6 {
+		t.Fatalf("esperava 6 valores (1 Success + 5 da expansão), obteve %d: %#v", len(misto), misto)
+	}
 }
 
 func TestEncryptionRoundTripAndNoFallbackKey(t *testing.T) {
 	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
 	ciphertext, err := encrypt("segredo AppyPay")
 	if err != nil {
==========================================
FILE: internal/finance/pagamentos_unificado_test.go
==========================================
--- a/internal/finance/pagamentos_unificado_test.go
+++ b/internal/finance/pagamentos_unificado_test.go
@@ -35,12 +35,46 @@
 	for i := range out {
 		out[i] = MensalidadeMesView{CodigoEstudante: idxToMTID(i), CodigoAcademia: "ACA1", AnoLetivo: "2026_2027", Mes: (i % 12) + 1}
 	}
 	return out
 }
 
+// TestDeveIncluirPendenciasSemCobranca cobre a decisão que corrige o bug
+// relatado em produção: filtrar GET /financeiro/cobrancas por
+// estado=Failed continuava devolvendo pendências sintéticas (sempre
+// status="pendente"), porque nada olhava para o filtro antes de computá-
+// -las. Uma pendência sintética é sempre origem="mensalidade" e
+// status="pendente" — qualquer filtro que exclua explicitamente um dos
+// dois deve excluir as pendências do resultado.
+func TestDeveIncluirPendenciasSemCobranca(t *testing.T) {
+	casos := []struct {
+		nome            string
+		estados, origens []string
+		esperado        bool
+	}{
+		{"sem nenhum filtro", nil, nil, true},
+		{"estado=pendente (o próprio valor das pendências)", []string{EstadoPendente}, nil, true},
+		{"estado=Failed exclui pendências — bug relatado", []string{"Failed"}, nil, false},
+		{"estado=Failed,pendente inclui (um dos valores casa)", []string{"Failed", EstadoPendente}, nil, true},
+		{"tipo=mensalidade inclui (o próprio valor das pendências)", nil, []string{"mensalidade"}, true},
+		{"tipo=matricula exclui pendências", nil, []string{"matricula"}, false},
+		{"tipo=matricula,mensalidade inclui (um dos valores casa)", nil, []string{"matricula", "mensalidade"}, true},
+		{"estado e tipo compatíveis inclui", []string{EstadoPendente}, []string{"mensalidade"}, true},
+		{"estado compatível mas tipo incompatível exclui", []string{EstadoPendente}, []string{"matricula"}, false},
+		{"estado incompatível mas tipo compatível exclui", []string{"Failed"}, []string{"mensalidade"}, false},
+	}
+	for _, c := range casos {
+		t.Run(c.nome, func(t *testing.T) {
+			got := DeveIncluirPendenciasSemCobranca(c.estados, c.origens)
+			if got != c.esperado {
+				t.Fatalf("DeveIncluirPendenciasSemCobranca(%v, %v) = %v, esperava %v", c.estados, c.origens, got, c.esperado)
+			}
+		})
+	}
+}
+
 // TestListarPagamentosUnificadoSemPendencias cobre o caso mais comum hoje
 // (nenhum filtro de escopo em GET /financeiro/cobrancas): sem pendências,
 // o resultado deve ser um passthrough exato das cobranças reais, com o
 // limit/offset originais intactos — nenhuma mudança de comportamento em
 // relação a antes desta unificação.
 func TestListarPagamentosUnificadoSemPendencias(t *testing.T) {
@@ -59,14 +93,14 @@
 		t.Fatalf("esperava a página começar em COB idx 60, começou em %s", res.Pagamentos[0].MerchantTransactionID)
 	}
 	if res.Total != 120 {
 		t.Fatalf("esperava total=120, obteve %d", res.Total)
 	}
 	for _, p := range res.Pagamentos {
-		if p.PendenciaSemCobranca {
-			t.Fatalf("nenhum item deveria ter PendenciaSemCobranca=true: %#v", p)
+		if p.Status == EstadoPendente {
+			t.Fatalf("nenhum item deveria ter Status=%q (pendência sintética): %#v", EstadoPendente, p)
 		}
 	}
 }
 
 // TestListarPagamentosUnificadoPaginaSoComPendencias cobre a página em que
 // as pendências sozinhas já preenchem o limit inteiro — buscarCobrancas
@@ -83,14 +117,14 @@
 		t.Fatalf("esperava 1 chamada a buscarCobrancas (só para o total), obteve %d", chamadas)
 	}
 	if len(res.Pagamentos) != 30 {
 		t.Fatalf("esperava 30 itens (todos de pendências), obteve %d", len(res.Pagamentos))
 	}
 	for i, p := range res.Pagamentos {
-		if !p.PendenciaSemCobranca {
-			t.Fatalf("item %d deveria ser uma pendência sintética: %#v", i, p)
+		if p.Status != EstadoPendente {
+			t.Fatalf("item %d deveria ser uma pendência sintética (Status=%q): %#v", i, EstadoPendente, p)
 		}
 	}
 	if res.Total != 60 {
 		t.Fatalf("esperava total=60 (50 pendências + 10 cobranças), obteve %d", res.Total)
 	}
 }
@@ -109,18 +143,18 @@
 		t.Fatalf("esperava 1 chamada a buscarCobrancas, obteve %d", chamadas)
 	}
 	if len(res.Pagamentos) != 30 {
 		t.Fatalf("esperava 30 itens (25 pendências + 5 cobranças), obteve %d", len(res.Pagamentos))
 	}
 	for i := 0; i < 25; i++ {
-		if !res.Pagamentos[i].PendenciaSemCobranca {
-			t.Fatalf("item %d deveria ser pendência", i)
+		if res.Pagamentos[i].Status != EstadoPendente {
+			t.Fatalf("item %d deveria ser pendência (Status=%q)", i, EstadoPendente)
 		}
 	}
 	for i := 25; i < 30; i++ {
-		if res.Pagamentos[i].PendenciaSemCobranca {
+		if res.Pagamentos[i].Status == EstadoPendente {
 			t.Fatalf("item %d deveria ser cobrança real", i)
 		}
 	}
 	// As 5 cobranças reais na página mista devem ser exatamente as 5
 	// primeiras (offset=0 do lado das cobranças, porque as pendências
 	// não consomem nenhum offset delas).
@@ -150,13 +184,13 @@
 		t.Fatal(err)
 	}
 	if len(res.Pagamentos) != 30 {
 		t.Fatalf("esperava 30 itens, obteve %d", len(res.Pagamentos))
 	}
 	for _, p := range res.Pagamentos {
-		if p.PendenciaSemCobranca {
+		if p.Status == EstadoPendente {
 			t.Fatalf("nenhum item desta página deveria ser pendência: %#v", p)
 		}
 	}
 	if res.Pagamentos[0].MerchantTransactionID != idxToMTID(5) {
 		t.Fatalf("esperava continuar em cobranca idx 5 (sem pular nem repetir), obteve %s", res.Pagamentos[0].MerchantTransactionID)
 	}
@@ -179,26 +213,26 @@
 		t.Fatal(err)
 	}
 	if len(pagina1.Pagamentos) != 30 {
 		t.Fatalf("pagina1: esperava 30 itens, obteve %d", len(pagina1.Pagamentos))
 	}
 	for _, p := range pagina1.Pagamentos {
-		if !p.PendenciaSemCobranca {
-			t.Fatalf("pagina1: todos os itens deveriam ser pendências: %#v", p)
+		if p.Status != EstadoPendente {
+			t.Fatalf("pagina1: todos os itens deveriam ser pendências (Status=%q): %#v", EstadoPendente, p)
 		}
 	}
 
 	pagina2, err := ListarPagamentosUnificado(pendencias, cobrancaListStub(50, &chamadas), 30, 30)
 	if err != nil {
 		t.Fatal(err)
 	}
 	if len(pagina2.Pagamentos) != 30 {
 		t.Fatalf("pagina2: esperava 30 itens, obteve %d", len(pagina2.Pagamentos))
 	}
 	for _, p := range pagina2.Pagamentos {
-		if p.PendenciaSemCobranca {
+		if p.Status == EstadoPendente {
 			t.Fatalf("pagina2: nenhum item deveria ser pendência: %#v", p)
 		}
 	}
 	if pagina2.Pagamentos[0].MerchantTransactionID != idxToMTID(0) {
 		t.Fatalf("pagina2: esperava comecar na cobranca idx 0, obteve %s", pagina2.Pagamentos[0].MerchantTransactionID)
 	}
@@ -231,15 +265,12 @@
 	m := MensalidadeMesView{CodigoEstudante: "EST001", CodigoAcademia: "ACA1", AnoLetivo: "2026_2027", Mes: 9, Valor: 15000}
 	a := pendenciaParaPagamentoResumo(m)
 	b := pendenciaParaPagamentoResumo(m)
 	if a.ID != b.ID {
 		t.Fatalf("a mesma pendência produziu ids diferentes entre chamadas: %s vs %s", a.ID, b.ID)
 	}
-	if !a.PendenciaSemCobranca {
-		t.Fatal("esperava PendenciaSemCobranca=true")
-	}
 	if a.Status != EstadoPendente {
 		t.Fatalf("esperava status=%q, obteve %q", EstadoPendente, a.Status)
 	}
 	if a.AtualizadoEm != nil {
 		t.Fatalf("esperava AtualizadoEm nil para uma pendência sintética, obteve %v", a.AtualizadoEm)
 	}
==========================================
FILE: internal/finance/pagamentos_unificado_integration_test.go
==========================================
--- a/internal/finance/pagamentos_unificado_integration_test.go
+++ b/internal/finance/pagamentos_unificado_integration_test.go
@@ -163,17 +163,14 @@
 	}
 
 	var pendentesSinteticas, cobrancasReais int
 	estudantesVistos := map[string]bool{}
 	for _, p := range res.Pagamentos {
 		estudantesVistos[p.CodigoEstudante] = true
-		if p.PendenciaSemCobranca {
+		if p.Status == EstadoPendente {
 			pendentesSinteticas++
-			if p.Status != EstadoPendente {
-				t.Fatalf("pendência sintética deveria ter status=%q, obteve %q", EstadoPendente, p.Status)
-			}
 			if p.AtualizadoEm != nil {
 				t.Fatalf("pendência sintética deveria ter AtualizadoEm nil, obteve %v", p.AtualizadoEm)
 			}
 		} else {
 			cobrancasReais++
 			if p.CodigoEstudante != "ESTFLXFL" {
@@ -199,14 +196,14 @@
 	if estudantesVistos["ESTVINC1"] {
 		t.Fatal("estudante inesperado na lista")
 	}
 
 	// Ordem: pendências primeiro, cobranças reais depois.
 	for i := 0; i < 3; i++ {
-		if !res.Pagamentos[i].PendenciaSemCobranca {
+		if res.Pagamentos[i].Status != EstadoPendente {
 			t.Fatalf("item %d deveria ser pendência (pendências vêm primeiro)", i)
 		}
 	}
-	if res.Pagamentos[3].PendenciaSemCobranca {
+	if res.Pagamentos[3].Status == EstadoPendente {
 		t.Fatal("item 3 deveria ser a cobrança real (por último)")
 	}
 }
==========================================
FILE: internal/finance/mensalidade_integration_test.go
==========================================
--- a/internal/finance/mensalidade_integration_test.go
+++ b/internal/finance/mensalidade_integration_test.go
@@ -1,10 +1,11 @@
 package finance
 
 import (
 	"context"
+	"net/http"
 	"strings"
 	"testing"
 	"time"
 
 	"github.com/google/uuid"
 	"spuri/internal/db"
@@ -311,12 +312,102 @@
 	}
 	if err := service.ReativarObrigacoesMensalidade(context.Background(), in, uuid.NewString(), "academia", "127.0.0.1"); err == nil {
 		t.Fatal("reativacao de mensalidade paga foi aceite")
 	}
 }
 
+// TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa
+// cobre um bug real de produção: mensalidadeTemCobrancaAberta (usada por
+// IniciarPagamentoMensalidades para bloquear uma segunda tentativa
+// enquanto já existe uma cobrança "em aberto" para o mesmo mês) só
+// reconhecia os estados terminais locais ("cancelada", "falhada") e
+// "Success" — uma cobrança com o estado bruto "Failed" devolvido pela
+// própria AppyPay (recusa no processador, ver docs/Parceiros e
+// integrações/AppyPay Documentação.md) nunca entrava nessa lista e por
+// isso ficava "presa" como em aberto para sempre, bloqueando
+// indefinidamente qualquer nova tentativa de pagamento do mesmo mês —
+// mesmo a cobrança anterior já tendo definitivamente falhado no provedor.
+func TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa(t *testing.T) {
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	client := integrationClient(t)
+	service := NewService(client)
+	ctx := context.Background()
+
+	academia := mensalidadeCodigo()
+	estudante := "EST-RETRY-" + uuid.NewString()[:8]
+	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2025_2026")
+	seedMensalidadeTurma(t, client, academia, "T-RETRY", "2025_2026", estudante, nil)
+	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 1000, 7, time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC))
+	if _, err := client.DB().Exec(`UPDATE financeiro_mensalidade_configuracoes SET metodos_pagamento='{GPO}' WHERE codigo_academia=$1`, academia); err != nil {
+		t.Fatal(err)
+	}
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+	transport := &appyPayMockTransport{status: "Pending"}
+	service.SetHTTPClient(&http.Client{Transport: transport})
+
+	pendentes, err := service.ListMensalidades(ctx, estudante, &academia)
+	if err != nil {
+		t.Fatal(err)
+	}
+	if len(pendentes) == 0 {
+		t.Fatal("esperava pelo menos uma mensalidade pendente")
+	}
+	alvo := pendentes[0]
+	meses := []MensalidadeSelecaoMes{{AnoLetivo: alvo.AnoLetivo, Mes: alvo.Mes}}
+
+	// 1a tentativa: cria a cobrança (POST /charges do mock sempre devolve
+	// "Pending" — vira EstadoCobrancaAguardandoPagamento após a
+	// normalização desta tarefa).
+	primeira, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
+		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
+		MetodoPagamento: "GPO", Telefone: "923000000",
+	}, estudante, "estudante", "127.0.0.1")
+	if err != nil {
+		t.Fatalf("1a tentativa de pagamento falhou: %v", err)
+	}
+	if primeira.Charge.Status != EstadoCobrancaAguardandoPagamento {
+		t.Fatalf("esperava status=%q logo após criar a cobrança, obteve %q", EstadoCobrancaAguardandoPagamento, primeira.Charge.Status)
+	}
+
+	// Uma 2a tentativa imediata (sem a AppyPay ter resolvido a 1a) deve
+	// continuar bloqueada — comportamento que já existia antes desta
+	// tarefa e continua correto: a cobrança está aberta de verdade.
+	if _, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
+		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
+		MetodoPagamento: "GPO", Telefone: "923000000",
+	}, estudante, "estudante", "127.0.0.1"); err == nil {
+		t.Fatal("esperava bloqueio de 2a tentativa enquanto a 1a cobrança ainda está aguardando pagamento")
+	}
+
+	// A AppyPay resolve a 1a cobrança como Failed (recusada no
+	// processador) — o Spuri descobre isso numa consulta, exatamente como
+	// aconteceria via webhook ou verificação manual.
+	transport.status = "Failed"
+	consultada, err := service.ConsultCharge(ctx, ContextoAcademia, academia, primeira.Charge.ID.String(), estudante, "estudante", "127.0.0.1")
+	if err != nil {
+		t.Fatalf("ConsultCharge falhou: %v", err)
+	}
+	if consultada.Status != "Failed" {
+		t.Fatalf("esperava status=Failed após a consulta, obteve %q", consultada.Status)
+	}
+
+	// A 2a tentativa agora deve ser aceite: a cobrança anterior já está
+	// definitivamente resolvida (Failed), não "em aberto" — este é
+	// exatamente o bug corrigido nesta tarefa.
+	segunda, err := service.IniciarPagamentoMensalidades(ctx, MensalidadePagamentoInput{
+		CodigoEstudante: estudante, CodigoAcademia: academia, Meses: meses,
+		MetodoPagamento: "GPO", Telefone: "923000000",
+	}, estudante, "estudante", "127.0.0.1")
+	if err != nil {
+		t.Fatalf("esperava que a 2a tentativa fosse aceite após a 1a cobrança ter falhado no provedor, obteve erro: %v", err)
+	}
+	if segunda.Charge.ID == primeira.Charge.ID {
+		t.Fatal("a 2a tentativa deveria ter criado uma cobrança nova, não reutilizado a 1a")
+	}
+}
+
 func TestIntegrationMensalidadeConsultaRespeitaAcademia(t *testing.T) {
 	client := integrationClient(t)
 	primeira, segunda := mensalidadeCodigo(), mensalidadeCodigo()
 	seedMensalidadeAcademia(t, client, primeira, "private", "fundamental", "2025_2026")
 	seedMensalidadeAcademia(t, client, segunda, "private", "fundamental", "2025_2026")
 	seedMensalidadeTurma(t, client, primeira, "T-CONS-1", "2025_2026", "EST-CONS", nil)
==========================================
FILE: internal/finance/appypay_integration_test.go
==========================================
--- a/internal/finance/appypay_integration_test.go
+++ b/internal/finance/appypay_integration_test.go
@@ -204,12 +204,76 @@
 	}
 	if conflitos != 1 {
 		t.Fatalf("conflitos pós-cancelamento = %d, queria 1", conflitos)
 	}
 }
 
+// TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso cobre a
+// generalização de AcceptWebhook feita nesta tarefa: antes, só um webhook
+// de sucesso atualizava financeiro_cobrancas — um webhook avisando que uma
+// referência REF expirou (ou que um GPO foi recusado) era gravado em
+// WebhookAppyPayRecebido mas nunca refletia na cobrança, que ficava presa
+// em aguardando_pagamento até alguém consultá-la manualmente. Cobre
+// também a guarda que impede um segundo webhook terminal de sobrescrever
+// um estado terminal já registrado.
+func TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso(t *testing.T) {
+	client := integrationClient(t)
+	t.Setenv("ENV", "test")
+	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
+	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
+	service := NewService(client)
+	service.httpClient = &http.Client{Transport: &appyPayMockTransport{status: "Pending"}}
+	academia := "MAT" + uuid.NewString()[:8]
+	configureIntegrationCredential(t, service, ContextoAcademia, academia)
+	codigo := seedMatriculaPendente(t, client, academia, 900)
+	charge, err := service.IniciarPagamentoMatricula(context.Background(), MatriculaPagamentoInput{CodigoSolicitacao: codigo, MetodoPagamento: "REF"}, "127.0.0.1")
+	if err != nil {
+		t.Fatal(err)
+	}
+	if charge.Charge.Status != EstadoCobrancaAguardandoPagamento {
+		t.Fatalf("esperava status=%q logo após criar a cobrança, obteve %q", EstadoCobrancaAguardandoPagamento, charge.Charge.Status)
+	}
+
+	owner := WebhookOwner{CredentialID: uuid.New(), ContextoTipo: ContextoAcademia, CodigoAcademia: academia}
+	accepted, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ProviderChargeID, owner, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Expired"})
+	if err != nil || !accepted {
+		t.Fatalf("webhook Expired = accepted %t, err %v", accepted, err)
+	}
+	statusAtual := func() string {
+		var status string
+		if err := client.DB().QueryRow(`SELECT payload->>'status' FROM financeiro_cobrancas WHERE id=$1`, charge.Charge.ID).Scan(&status); err != nil {
+			t.Fatal(err)
+		}
+		return status
+	}
+	if got := statusAtual(); got != "Expired" {
+		t.Fatalf("esperava status=Expired refletido na cobrança após o webhook, obteve %q", got)
+	}
+
+	// Um segundo webhook tardio (ex.: reentrega), com um estado terminal
+	// DIFERENTE, não deve sobrescrever o Expired já registrado — só um
+	// eventID diferente (aqui o id interno da cobrança, que loadCharge
+	// também reconhece) passa pela deduplicação de
+	// financeiro_webhooks_recebidos para realmente exercer a guarda.
+	accepted2, err := service.AcceptWebhook(context.Background(), "REF", charge.Charge.ID.String(), owner, map[string]any{"id": charge.Charge.ProviderChargeID, "status": "Failed"})
+	if err != nil || !accepted2 {
+		t.Fatalf("segundo webhook = accepted %t, err %v", accepted2, err)
+	}
+	if got := statusAtual(); got != "Expired" {
+		t.Fatalf("um segundo webhook terminal não deveria sobrescrever Expired, obteve %q", got)
+	}
+
+	var confirmacoes int
+	if err = client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_type='Financeiro' AND aggregate_id=$1 AND event_type='MensalidadesCobrancaConfirmada'`, charge.Charge.ID).Scan(&confirmacoes); err != nil {
+		t.Fatal(err)
+	}
+	if confirmacoes != 0 {
+		t.Fatalf("um webhook não-sucesso nunca deveria confirmar pagamento, obteve %d confirmações", confirmacoes)
+	}
+}
+
 func TestIntegrationWebhookSecretGeneratedOnceGlobalHeaderAndRotation(t *testing.T) {
 	client := integrationClient(t)
 	service := NewService(client)
 	ctx := context.Background()
 	t.Setenv("ENV", "test")
 	t.Setenv("FINANCE_ENCRYPTION_KEY", "test-only-secret-material-at-least-32")
==========================================
FILE: internal/handlers/financeiro_cobrancas_handlers_test.go
==========================================
--- a/internal/handlers/financeiro_cobrancas_handlers_test.go
+++ b/internal/handlers/financeiro_cobrancas_handlers_test.go
@@ -72,12 +72,42 @@
 	if err := json.Unmarshal(all.Body.Bytes(), &body); err != nil {
 		t.Fatal(err)
 	}
 	if body.TotalGeral != 3 {
 		t.Fatalf("academia A deveria ver 3 cobranças próprias, viu %d: %s", body.TotalGeral, all.Body.String())
 	}
+	// A cobrança "criada" foi inserida diretamente no banco (bypassando o
+	// Service), simulando uma cobrança criada ANTES desta tarefa — o
+	// status bruto histórico "criada" nunca deveria voltar ao chamador
+	// como está: scanCobrancaResumo normaliza a leitura para o estado
+	// canônico único aguardando_pagamento, mesmo para uma linha que nunca
+	// passou pelo novo código de escrita.
+	var achouCriadaComoAguardando bool
+	for _, p := range body.Pagamentos {
+		if p.Status == "criada" {
+			t.Fatalf("status bruto histórico \"criada\" vazou para a API sem normalizar: %#v", p)
+		}
+		if p.Status == finance.EstadoCobrancaAguardandoPagamento && p.Origem == "avulsa" {
+			achouCriadaComoAguardando = true
+		}
+	}
+	if !achouCriadaComoAguardando {
+		t.Fatalf("esperava a cobrança \"criada\" normalizada para %q: %s", finance.EstadoCobrancaAguardandoPagamento, all.Body.String())
+	}
+
+	// Filtrar por estado=aguardando_pagamento (o novo nome canônico) deve
+	// encontrar essa MESMA cobrança histórica, mesmo o valor gravado no
+	// banco ainda sendo o bruto "criada" — é a expansão de
+	// estadosCobrancaEquivalentes que garante essa equivalência no SQL.
+	porNovoEstado := call(academiaA, "estado=aguardando_pagamento")
+	if err := json.Unmarshal(porNovoEstado.Body.Bytes(), &body); err != nil {
+		t.Fatal(err)
+	}
+	if body.TotalGeral != 1 || len(body.Pagamentos) != 1 || body.Pagamentos[0].Status != finance.EstadoCobrancaAguardandoPagamento {
+		t.Fatalf("filtro por estado=aguardando_pagamento deveria encontrar a cobrança histórica \"criada\": %s", porNovoEstado.Body.String())
+	}
 
 	filtrada := call(academiaA, "estado=Success")
 	if err := json.Unmarshal(filtrada.Body.Bytes(), &body); err != nil {
 		t.Fatal(err)
 	}
 	if body.TotalGeral != 1 || len(body.Pagamentos) != 1 || body.Pagamentos[0].Origem != "mensalidade" {
==========================================
FILE: internal/handlers/financeiro_pendencias_handlers_test.go
==========================================
--- a/internal/handlers/financeiro_pendencias_handlers_test.go
+++ b/internal/handlers/financeiro_pendencias_handlers_test.go
@@ -51,20 +51,21 @@
 		VALUES ($1,$2,$3,'fundamental',$4,NULL,$5,7,'2026-01-01')`,
 		uuid.New(), uuid.New(), academia, anoAcademico, valor); err != nil {
 		t.Fatal(err)
 	}
 }
 
-// TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciaSemCobranca
+// TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciaSintetica
 // cobre, no nível HTTP, o problema original da tarefa 58 (um estudante que
 // nunca tentou nenhuma cobrança de mensalidade é invisível para a academia
 // em GET /financeiro/cobrancas a menos que ela informe um filtro de
 // escopo), já na forma unificada desta tarefa: quando ano_letivo é
 // informado, a pendência aparece dentro de "pagamentos", com
-// pendencia_sem_cobranca=true — não mais num array separado.
-func TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciaSemCobranca(t *testing.T) {
+// status="pendente" — o único sinal, desde esta tarefa, de que não existe
+// nenhuma cobrança real por trás do item.
+func TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciaSintetica(t *testing.T) {
 	gin.SetMode(gin.TestMode)
 	client := integrationFinanceClient(t)
 	academia := "PND" + strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
 	estudante := "ESTPND1"
 	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-PND", "2026_2027", "7_ano_fundamental", estudante)
 	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)
@@ -102,13 +103,13 @@
 	if len(bodySemEscopo.Pagamentos) != 0 || bodySemEscopo.TotalGeral != 0 {
 		t.Fatalf("sem filtro de escopo, esperava lista vazia (nenhuma cobrança real, pendências não computadas): %s", semEscopo.Body.String())
 	}
 
 	// Com ano_letivo: o estudante nunca tentou nenhuma cobrança, então
 	// TODOS os meses pendentes dele devem vir em "pagamentos", com
-	// pendencia_sem_cobranca=true.
+	// status="pendente".
 	comEscopo := call("ano_letivo=2026_2027")
 	if comEscopo.Code != http.StatusOK {
 		t.Fatalf("com escopo = %d: %s", comEscopo.Code, comEscopo.Body.String())
 	}
 	var body struct {
 		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
@@ -117,33 +118,124 @@
 		t.Fatal(err)
 	}
 	if len(body.Pagamentos) == 0 {
 		t.Fatalf("esperava pagamentos não vazio: %s", comEscopo.Body.String())
 	}
 	for _, p := range body.Pagamentos {
-		if !p.PendenciaSemCobranca {
-			t.Fatalf("esperava só pendências sintéticas nesta academia (nenhuma cobrança real criada): %#v", p)
+		if p.Status != finance.EstadoPendente {
+			t.Fatalf("esperava só pendências sintéticas (status=%q) nesta academia (nenhuma cobrança real criada): %#v", finance.EstadoPendente, p)
 		}
 		if p.CodigoEstudante != estudante {
 			t.Fatalf("pendência de outro estudante inesperada: %#v", p)
 		}
-		if p.Status != finance.EstadoPendente {
-			t.Fatalf("esperava status pendente, obteve %q", p.Status)
-		}
 		if p.AtualizadoEm != nil {
 			t.Fatalf("pendência sintética não deveria ter atualizado_em: %#v", p)
 		}
 	}
 }
 
-// TestIntegrationConsultarCobrancasEstudanteIncluiPendenciaSemCobranca
-// cobre, no nível HTTP, a versão por estudante (sempre calculada, sem
-// exigir filtro de escopo): a própria academia, consultando o histórico de
-// UM estudante específico, já enxerga dentro de "pagamentos" os meses que
-// ele deve e nunca tentou pagar, marcados com pendencia_sem_cobranca=true.
-func TestIntegrationConsultarCobrancasEstudanteIncluiPendenciaSemCobranca(t *testing.T) {
+// TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas
+// reproduz, no nível HTTP, o bug relatado em produção: uma academia
+// consultou GET /financeiro/cobrancas?...&estado=Failed&tipo=mensalidade&
+// ano_letivo=...&mes=... e recebeu de volta todos os pagamentos do mês
+// (não só os Failed), porque a computação de pendências sintéticas nunca
+// olhava para o filtro de estado antes desta tarefa — toda pendência é
+// sempre status="pendente", nunca "Failed", mas entrava na lista do mesmo
+// jeito. Com o mesmo escopo usado no relato original (ano_letivo e mes),
+// nenhum estudante tentou nenhuma cobrança ainda: o resultado filtrado por
+// estado=Failed deve vir vazio, e não "todos os pagamentos".
+func TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas(t *testing.T) {
+	gin.SetMode(gin.TestMode)
+	client := integrationFinanceClient(t)
+	academia := "BUG" + strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
+	estudante := "ESTBUG1"
+	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-BUG", "2026_2027", "7_ano_fundamental", estudante)
+	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)
+
+	previousService := FinanceiroService
+	FinanceiroService = finance.NewService(client)
+	t.Cleanup(func() { FinanceiroService = previousService })
+
+	call := func(query string) *httptest.ResponseRecorder {
+		recorder := httptest.NewRecorder()
+		ctx, _ := gin.CreateTestContext(recorder)
+		ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?"+query, nil)
+		ctx.Set("dbClient", client)
+		ctx.Set("user_id", uuid.New())
+		ctx.Set("user_type", "academia")
+		ctx.Set("codigo_academia", academia)
+		ListarCobrancasAppyPay(ctx)
+		return recorder
+	}
+
+	// Confirma primeiro que, sem o filtro de estado, a pendência sintética
+	// aparece normalmente (mesma cobertura do teste anterior) — isolando
+	// que a mudança de comportamento vem exclusivamente do filtro estado.
+	semFiltro := call("ano_letivo=2026_2027&mes=3")
+	var bodySemFiltro struct {
+		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
+		TotalGeral int                       `json:"total_geral"`
+	}
+	if err := json.Unmarshal(semFiltro.Body.Bytes(), &bodySemFiltro); err != nil {
+		t.Fatal(err)
+	}
+	if bodySemFiltro.TotalGeral == 0 {
+		t.Fatalf("sem filtro de estado, esperava ver a pendência sintética de março: %s", semFiltro.Body.String())
+	}
+
+	// Reproduz exatamente a query relatada (adaptada ao escopo deste
+	// teste): estado=Failed nunca deveria trazer pendências sintéticas,
+	// já que nenhuma delas tem esse status.
+	comFiltro := call("estado=Failed&tipo=mensalidade&ano_letivo=2026_2027&mes=3&limit=30&offset=0")
+	if comFiltro.Code != http.StatusOK {
+		t.Fatalf("com filtro estado=Failed = %d: %s", comFiltro.Code, comFiltro.Body.String())
+	}
+	var bodyComFiltro struct {
+		Pagamentos []finance.PagamentoResumo `json:"pagamentos"`
+		TotalGeral int                       `json:"total_geral"`
+	}
+	if err := json.Unmarshal(comFiltro.Body.Bytes(), &bodyComFiltro); err != nil {
+		t.Fatal(err)
+	}
+	if bodyComFiltro.TotalGeral != 0 || len(bodyComFiltro.Pagamentos) != 0 {
+		t.Fatalf("estado=Failed não deveria trazer nenhuma pendência sintética (nenhuma cobrança real existe nesta academia): %s", comFiltro.Body.String())
+	}
+
+	// tipo=matricula (excluindo mensalidade) também deve excluir as
+	// pendências, já que toda pendência sintética é sempre mensalidade.
+	comTipoMatricula := call("tipo=matricula&ano_letivo=2026_2027&mes=3")
+	var bodyTipoMatricula struct {
+		TotalGeral int `json:"total_geral"`
+	}
+	if err := json.Unmarshal(comTipoMatricula.Body.Bytes(), &bodyTipoMatricula); err != nil {
+		t.Fatal(err)
+	}
+	if bodyTipoMatricula.TotalGeral != 0 {
+		t.Fatalf("tipo=matricula não deveria trazer pendências sintéticas de mensalidade: %s", comTipoMatricula.Body.String())
+	}
+
+	// E, de volta, estado=pendente (o próprio valor das pendências)
+	// continua trazendo-as normalmente.
+	comEstadoPendente := call("estado=pendente&ano_letivo=2026_2027&mes=3")
+	var bodyEstadoPendente struct {
+		TotalGeral int `json:"total_geral"`
+	}
+	if err := json.Unmarshal(comEstadoPendente.Body.Bytes(), &bodyEstadoPendente); err != nil {
+		t.Fatal(err)
+	}
+	if bodyEstadoPendente.TotalGeral == 0 {
+		t.Fatalf("estado=pendente deveria continuar trazendo a pendência sintética: %s", comEstadoPendente.Body.String())
+	}
+}
+
+// TestIntegrationConsultarCobrancasEstudanteIncluiPendenciaSintetica cobre,
+// no nível HTTP, a versão por estudante (sempre calculada, sem exigir
+// filtro de escopo): a própria academia, consultando o histórico de UM
+// estudante específico, já enxerga dentro de "pagamentos" os meses que ele
+// deve e nunca tentou pagar, marcados com status="pendente".
+func TestIntegrationConsultarCobrancasEstudanteIncluiPendenciaSintetica(t *testing.T) {
 	gin.SetMode(gin.TestMode)
 	client := integrationFinanceClient(t)
 	academia := "PNDE" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
 	estudante := "ESTPND2"
 	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-PNDE", "2026_2027", "7_ano_fundamental", estudante)
 	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)
@@ -171,13 +263,13 @@
 	}
 	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
 		t.Fatal(err)
 	}
 	achouPendencia := false
 	for _, p := range body.Pagamentos {
-		if p.PendenciaSemCobranca {
+		if p.Status == finance.EstadoPendente {
 			achouPendencia = true
 		}
 	}
 	if !achouPendencia {
 		t.Fatalf("esperava ao menos 1 pendência sintética em pagamentos: %s", recorder.Body.String())
 	}
```

---

## 6. Diffs exatos — `Documentação da API.md`

Atualiza as seções 19.6 (nota sobre webhook não tocada — só 19.10 recebeu a nota nova, 19.11 já herda por dizer "aplica as mesmas regras" de 19.10), 19.7, 19.8, 19.9 e 19.10 para refletir o novo modelo de estados, a remoção de `pendencia_sem_cobranca`, e o suporte completo aos estados terminais.

```diff
--- "a/Documenta\303\247\303\243o da API.md"
+++ "backend/Documenta\303\247\303\243o da API.md"
@@ -7684,24 +7684,24 @@
 - O `:id` pode ser o identificador retornado pelo provider ou o `merchantTransactionId` informado na criação.
 - A consulta grava evento financeiro apenas quando o estado, identificador do provider ou resposta relevante mudar; consultas sem mudança não poluem o ledger.
 - Se a consulta detectar `Success` da AppyPay depois de a cobrança ter sido cancelada localmente, grava `CobrancaAppyPayConflitoPosCancelamento` e preserva o status local `cancelada`, para reconciliação manual por admin FPP.
 
 #### 19.7 GET /financeiro/cobrancas
 
-**Escopo da rota:** lista, numa única lista paginada, os pagamentos (mensalidade, matrícula ou avulsa) do contexto autorizado — visão de academia/admin sobre pagamentos recebidos e pendentes. Quando filtrada por turma, curso, ano acadêmico ou ano letivo, a lista também inclui as pendências de mensalidade que ainda não foram pagas e não têm nenhuma cobrança real vinculada — ver `pendencia_sem_cobranca` abaixo. Para o estudante consultar o próprio histórico de pagamentos, use 19.8.
+**Escopo da rota:** lista, numa única lista paginada, os pagamentos (mensalidade, matrícula ou avulsa) do contexto autorizado — visão de academia/admin sobre pagamentos recebidos e pendentes. Quando filtrada por turma, curso, ano acadêmico ou ano letivo, a lista também inclui as pendências de mensalidade que ainda não foram pagas e não têm nenhuma cobrança real vinculada — ver as regras de negócio abaixo sobre `status: "pendente"`. Para o estudante consultar o próprio histórico de pagamentos, use 19.8.
 
 **Proteção:** autenticado + academia do próprio contexto ou admin FPP. Estudantes recebem `403` nesta rota.
 
 **Query params:**
 
 | Campo | Tipo | Obrigatório | Descrição |
 |---|---|---|---|
 | `contexto_tipo` | string | Não | Contexto financeiro consultado. Para academia autenticada é forçado para `academia`. |
 | `codigo_academia` | string | Não | Academia dona das cobranças. Para academia autenticada é forçado para o código do token. |
-| `estado` | string, repetível | Não | Filtra pelo texto exato (case-sensitive) persistido em `status` de uma cobrança real — mistura estados internos (`solicitada`, `criada`, `cancelada`, `falhada`) e estados crus da AppyPay (`Success`, `Pending`, `Failed`, etc). Repita o parâmetro para casar mais de um valor (`?estado=Success&estado=Pending`). **Não filtra os itens sintéticos** (`pendencia_sem_cobranca: true`) — esses sempre têm `status: "pendente"` e sempre aparecem, independente deste filtro (ver regras de negócio). |
-| `tipo` | string, repetível | Não | Filtra por origem: `matricula`, `mensalidade` ou `avulsa`. Mesma ressalva de `estado`: não filtra os itens sintéticos, que são sempre `origem: "mensalidade"`. |
+| `estado` | string, repetível | Não | Filtra pelo estado de uma cobrança real. Aceita o texto exato persistido em `status` — estados internos (`cancelada`, `falhada`) e estados crus da AppyPay (`Success`, `Failed`, `Cancelled`, `Expired`) — mais o estado canônico `aguardando_pagamento` (cobrança gerada/tentada, ainda sem resolução do provedor; casa também com cobranças antigas gravadas antes desta forma existir). Repita o parâmetro para casar mais de um valor (`?estado=Success&estado=aguardando_pagamento`). Também filtra os itens sintéticos: como toda pendência sintética tem sempre `status: "pendente"`, um filtro de `estado` que não inclua `"pendente"` exclui as pendências do resultado (ver regras de negócio). |
+| `tipo` | string, repetível | Não | Filtra por origem: `matricula`, `mensalidade` ou `avulsa`. Mesma lógica de `estado`: como toda pendência sintética é sempre `origem: "mensalidade"`, um filtro de `tipo` que não inclua `"mensalidade"` exclui as pendências do resultado. |
 | `turma_id` | UUID | Não | Restringe a pagamentos de mensalidade vinculados a esta turma. Só afeta cobranças reais de origem `mensalidade` — ver regras de negócio. |
 | `curso_id` | UUID | Não | Restringe a pagamentos de mensalidade vinculados a este curso. Mesma ressalva de `turma_id`. |
 | `ano_academico` | string | Não | Restringe a pagamentos de mensalidade deste ano/classe (ex.: `7_ano_fundamental`). Mesma ressalva de `turma_id`. |
 | `ano_letivo` | string | Não | Restringe a pagamentos de mensalidade deste ano letivo (ex.: `2026_2027`). Mesma ressalva de `turma_id`. |
 | `mes` | inteiro (1-12) | Não | Restringe a um mês de calendário específico. Só tem efeito quando combinado com pelo menos um dos quatro filtros acima; sozinho, é ignorado silenciosamente (não delimita o suficiente — um mês de calendário pode abranger estudantes de vários anos letivos diferentes). `400` se fora do intervalo 1-12. |
 | `limit` | inteiro | Não | Itens por página. Padrão 50, mínimo 1, máximo 1000. |
@@ -7718,17 +7718,32 @@
       "status": "pendente",
       "valor": 15000.00,
       "moeda": "AOA",
       "descricao": "Propinas ACA001: 1 mensalidade(s) — pendência sem cobrança gerada",
       "codigo_estudante": "EST0002",
       "mensalidades": [{"ano_letivo": "2025_2026", "mes": 9}],
-      "pendencia_sem_cobranca": true,
       "id": "b6f2e6b1-3f1a-5e9c-8f2a-1a2b3c4d5e6f",
       "contexto_tipo": "academia"
     },
     {
+      "id": "7c1a9e2d-...",
+      "provider_charge_id": "APPYPAY-987655",
+      "merchant_transaction_id": "P2608LDA000002",
+      "contexto_tipo": "academia",
+      "codigo_academia": "ACA001",
+      "origem": "mensalidade",
+      "status": "aguardando_pagamento",
+      "valor": 1000.00,
+      "moeda": "AOA",
+      "descricao": "Mensalidade novembro/2025",
+      "metodo_pagamento": "GPO",
+      "codigo_estudante": "EST0003",
+      "mensalidades": [{"ano_letivo": "2025_2026", "mes": 11}],
+      "atualizado_em": "2026-08-25T09:10:00Z"
+    },
+    {
       "id": "4d2bbf53-c8c0-4c9a-a3f4-5a0f0cf988d1",
       "provider_charge_id": "APPYPAY-987654",
       "merchant_transaction_id": "P2608LDA000001",
       "contexto_tipo": "academia",
       "codigo_academia": "ACA001",
       "origem": "mensalidade",
@@ -7736,46 +7751,47 @@
       "valor": 1000.00,
       "moeda": "AOA",
       "descricao": "Mensalidade outubro/2025",
       "metodo_pagamento": "GPO",
       "codigo_estudante": "EST0001",
       "mensalidades": [{"ano_letivo": "2025_2026", "mes": 10}],
-      "atualizado_em": "2026-08-08T12:30:00Z",
-      "pendencia_sem_cobranca": false
+      "atualizado_em": "2026-08-08T12:30:00Z"
     }
   ],
-  "total": 2,
-  "total_geral": 2,
+  "total": 3,
+  "total_geral": 3,
   "limit": 50,
   "offset": 0
 }
 ```
 
 **Regras de negócio:**
 
-- Cada item de `pagamentos` tem o mesmo formato — o mesmo objeto que antes era só uma "cobrança" ganhou o campo `pendencia_sem_cobranca`, e todo item o traz, dos dois tipos:
-  - `pendencia_sem_cobranca: false` — um pagamento real, com todos os campos vindos de uma cobrança de fato criada (`id` real, `atualizado_em` presente, e opcionalmente `provider_charge_id`/`merchant_transaction_id`/`metodo_pagamento` quando fizer sentido). Não devolve `payment_info`, `response` (resposta crua da AppyPay) nem `qrCodeArr`; para o detalhe completo de uma cobrança específica, use 19.6.
-  - `pendencia_sem_cobranca: true` — um mês de mensalidade que ainda não foi pago (nem anulado) e não tem **nenhuma** cobrança real vinculada (nem sequer uma tentativa falhada) — sintetizado a partir da mesma computação de 19.17 (`MensalidadeMesView`). `id` é determinístico (hash estável de academia+estudante+ano_letivo+mês — a mesma pendência sempre tem o mesmo `id` entre chamadas, útil como key de lista no cliente), `status` é sempre `"pendente"`, `atualizado_em` é sempre ausente (não existe nenhuma atividade real para reportar), e `metodo_pagamento`/`provider_charge_id`/`merchant_transaction_id` também ficam ausentes.
-- **`status: "pendente"` pode vir de dois casos diferentes**, e é `pendencia_sem_cobranca` que desambigua qual: (a) uma cobrança real cujo status ainda não foi resolvido pelo provedor (`pendencia_sem_cobranca: false` — a cobrança foi de fato tentada e a AppyPay devolveu um estado não-terminal); ou (b) uma pendência sintética (`pendencia_sem_cobranca: true` — não existe nenhuma cobrança para este mês).
-- Um mês com uma cobrança real **falhada** aparece como item real (`status: "falhada"`, `pendencia_sem_cobranca: false`) — **não** gera também um item sintético duplicado para o mesmo mês, mesmo continuando a valer como "ainda não pago" internamente (ver 19.17 e a tarefa que corrigiu esse critério). A cobrança real, com seu histórico verdadeiro, já é a representação desse mês na lista.
+- Cada item de `pagamentos` tem o mesmo formato, dos dois tipos possíveis — `status` sozinho já diz qual é:
+  - Qualquer `status` diferente de `"pendente"` (incluindo `"aguardando_pagamento"`) — um pagamento real, com todos os campos vindos de uma cobrança de fato criada (`id` real, `atualizado_em` presente, e opcionalmente `provider_charge_id`/`merchant_transaction_id`/`metodo_pagamento` quando fizer sentido). Não devolve `payment_info`, `response` (resposta crua da AppyPay) nem `qrCodeArr`; para o detalhe completo de uma cobrança específica, use 19.6.
+  - `status: "pendente"` — um mês de mensalidade que ainda não foi pago (nem anulado) e não tem **nenhuma** cobrança real vinculada (nem sequer uma tentativa falhada) — sintetizado a partir da mesma computação de 19.17 (`MensalidadeMesView`). `id` é determinístico (hash estável de academia+estudante+ano_letivo+mês — a mesma pendência sempre tem o mesmo `id` entre chamadas, útil como key de lista no cliente), `atualizado_em` é sempre ausente (não existe nenhuma atividade real para reportar), e `metodo_pagamento`/`provider_charge_id`/`merchant_transaction_id` também ficam ausentes.
+- **`status` sozinho já diz se existe uma cobrança real por trás do item** — `"pendente"` é exclusivo das pendências sintéticas; uma cobrança real nunca usa esse valor. Assim que uma cobrança é gerada/tentada (mesmo antes de qualquer resposta do provedor), seu status passa a ser `"aguardando_pagamento"` — o estado canônico que substitui os antigos `"solicitada"`/`"criada"` (gravados localmente antes de qualquer resposta da AppyPay) e os estados crus `"Requested"`/`"Pending"` que a própria AppyPay documenta para essa mesma fase (cobrança gerada, ainda sem resolução). Cobranças criadas antes desta forma existir, se ainda não resolvidas, continuam sendo lidas e filtradas como `"aguardando_pagamento"` também (equivalência histórica, não é preciso reprocessar nada).
+- Estados terminais de uma cobrança real (não mudam mais sozinhos): `"Success"` (paga), `"Failed"` (recusada no processador da AppyPay), `"Cancelled"` (cancelada do lado da AppyPay), `"Expired"` (referência REF expirada sem pagamento), `"falhada"` (a chamada à AppyPay falhou, sem chegar a existir cobrança do lado do provedor) e `"cancelada"` (cancelamento feito pelo Spuri, 19.9).
+- Um mês com uma cobrança real **falhada** aparece como item real (`status: "falhada"` ou `"Failed"`) — **não** gera também um item sintético duplicado para o mesmo mês, mesmo continuando a valer como "ainda não pago" internamente (ver 19.17 e a tarefa que corrigiu esse critério). A cobrança real, com seu histórico verdadeiro, já é a representação desse mês na lista.
 - `origem` é derivada do payload persistido para itens reais, nunca gravada separadamente: `matricula` quando a cobrança tem `codigo_solicitacao`, `mensalidade` quando tem `codigo_estudante` (e não tem `codigo_solicitacao`), `avulsa` nos demais casos. Itens sintéticos são sempre `origem: "mensalidade"`.
-- **Ordenação:** itens sintéticos (`pendencia_sem_cobranca: true`) sempre vêm primeiro — representam ação pendente ("isto ainda precisa de uma cobrança"). Depois vêm os itens reais, por `updated_at DESC` (atividade mais recente primeiro). A paginação (`limit`/`offset`) percorre essa ordem combinada como uma lista única.
+- **Ordenação:** itens sintéticos (`status: "pendente"`) sempre vêm primeiro — representam ação pendente ("isto ainda precisa de uma cobrança"). Depois vêm os itens reais, por `updated_at DESC` (atividade mais recente primeiro). A paginação (`limit`/`offset`) percorre essa ordem combinada como uma lista única.
 - `total` é o número de itens nesta página; `total_geral` é o total real (pendências sintéticas + cobranças reais) que casa com os filtros aplicados.
 - `turma_id`, `curso_id`, `ano_academico` e `ano_letivo` só têm efeito sobre cobranças reais de origem `mensalidade`: usar qualquer um deles exclui automaticamente cobranças de `matricula` e `avulsa` do resultado, porque essas duas origens não têm um vínculo de turma/ano letivo resolvível (a cobrança de matrícula antecede a atribuição de turma do estudante). Pendências sintéticas só existem para `mensalidade`, então também dependem de pelo menos um desses quatro filtros — sem nenhum, nenhum item sintético é computado nem aparece na lista (evita varrer a academia inteira sem limite a cada chamada).
+- **Itens sintéticos respeitam `estado` e `tipo`:** como toda pendência sintética é sempre `status: "pendente"` e `origem: "mensalidade"`, informar `estado` sem incluir `"pendente"`, ou `tipo` sem incluir `"mensalidade"`, exclui as pendências do resultado — mesmo dentro do escopo de turma/curso/ano onde elas normalmente apareceriam. Sem nenhum desses dois filtros, pendências continuam incluídas normalmente.
 
 #### 19.8 GET /financeiro/cobrancas/estudante/:codigo
 
-**Escopo da rota:** lista, numa única lista paginada, TODOS os pagamentos que um estudante já teve — cobranças reais, em qualquer estado, academia ou origem (incluindo a cobrança da matrícula original, mesmo que ela tenha sido paga antes de o estudante existir como tal), e pendências de mensalidade que ainda não têm nenhuma cobrança real vinculada. Mesmo formato de resposta de 19.7 — ver `pendencia_sem_cobranca` ali. É a visão do próprio estudante sobre o seu histórico de pagamentos. Para a visão de academia/admin sobre cobranças recebidas, use 19.7.
+**Escopo da rota:** lista, numa única lista paginada, TODOS os pagamentos que um estudante já teve — cobranças reais, em qualquer estado, academia ou origem (incluindo a cobrança da matrícula original, mesmo que ela tenha sido paga antes de o estudante existir como tal), e pendências de mensalidade que ainda não têm nenhuma cobrança real vinculada. Mesmo formato de resposta de 19.7 — ver as regras de negócio ali sobre `status: "pendente"`. É a visão do próprio estudante sobre o seu histórico de pagamentos. Para a visão de academia/admin sobre cobranças recebidas, use 19.7.
 
 **Proteção:** o próprio estudante (`:codigo` deve ser o código do token), academia à qual o estudante pertence ou pertenceu (mesmo vínculo histórico de `GET /financeiro/mensalidades/estudante/:codigo`), ou admin FPP.
 
 **Query params:**
 
 | Campo | Tipo | Obrigatório | Descrição |
 |---|---|---|---|
-| `estado` | string, repetível | Não | Mesmo filtro de 19.7. Sem filtro, devolve todos os estados. |
+| `estado` | string, repetível | Não | Mesmo filtro de 19.7 (inclusive a equivalência histórica de `aguardando_pagamento` e o efeito sobre itens sintéticos). Sem filtro, devolve todos os estados. |
 | `tipo` | string, repetível | Não | Mesmo filtro de 19.7: `matricula`, `mensalidade` ou `avulsa`. |
 | `turma_id` | UUID | Não | Mesmo filtro de 19.7. Só tem efeito quando quem consulta é a academia (isto é, quando o contexto de uma única academia já está resolvido) — ver regras de negócio. |
 | `curso_id` | UUID | Não | Mesma ressalva de `turma_id`. |
 | `ano_academico` | string | Não | Mesma ressalva de `turma_id`. |
 | `ano_letivo` | string | Não | Mesma ressalva de `turma_id`. |
 | `limit` | inteiro | Não | Itens por página. Padrão 50, mínimo 1, máximo 1000. |
@@ -7786,13 +7802,13 @@
 **Regras de negócio:**
 
 - Diferente de 19.7, esta consulta não aceita `contexto_tipo` nem `codigo_academia`: um estudante pode ter mensalidades e matrícula em mais de uma academia (histórico), e o histórico mostra tudo — exceto quando quem consulta é uma academia, caso em que o resultado é restrito às cobranças feitas a essa academia especificamente (uma academia nunca vê pagamentos que o estudante fez a outra academia, mesmo com vínculo histórico com as duas).
 - A cobrança de matrícula é resolvida pelo vínculo `codigo_estudante_gerado`, já gravado em `projection_solicitacoes_matricula` quando a solicitação é aprovada — o payload da cobrança de matrícula em si nunca grava `codigo_estudante`, porque a cobrança é anterior ao registo do estudante.
 - Sem filtro de `estado`, a listagem inclui cobranças reais pendentes, falhadas e canceladas, não só as pagas — intencional: o objetivo é o estudante conseguir ver tudo que já teve, não só os pagamentos concluídos.
 - `turma_id`, `curso_id`, `ano_academico` e `ano_letivo` seguem a mesma restrição de 19.7 (só afetam cobranças reais de origem `mensalidade`), mas só têm efeito quando quem consulta é uma academia (o resultado já está então restrito a uma única academia); quando quem consulta é o próprio estudante ou admin FPP sem restringir a academia, esses quatro filtros são ignorados.
-- **Diferença chave em relação a 19.7:** os itens sintéticos (`pendencia_sem_cobranca: true`) aqui são **sempre** calculados, sem exigir nenhum dos quatro filtros de escopo — porque esta consulta já está inerentemente limitada a um único estudante, então não há o mesmo risco de varredura sem limite que existe em 19.7 (que precisa de pelo menos um filtro para computar pendências).
+- **Diferença chave em relação a 19.7:** os itens sintéticos (`status: "pendente"`) aqui são **sempre** calculados, sem exigir nenhum dos quatro filtros de escopo — porque esta consulta já está inerentemente limitada a um único estudante, então não há o mesmo risco de varredura sem limite que existe em 19.7 (que precisa de pelo menos um filtro para computar pendências). Mesmo assim, um `estado`/`tipo` que exclua `"pendente"`/`"mensalidade"` continua excluindo-os, igual a 19.7.
 - **Não aceita** o parâmetro `mes` — só disponível em 19.7 por enquanto.
 
 **Erros comuns:** `404` estudante inexistente, `403` estudante tentando ver outro código, academia sem vínculo ou admin sem `fpp`.
 
 
 #### 19.9 POST /financeiro/appypay/cobrancas/:id/cancelar
@@ -7813,13 +7829,13 @@
 
 **Response 200:** a mesma estrutura da consulta, com `status: "cancelada"`.
 
 **Regras de negócio:**
 
 - Antes de registrar `CobrancaAppyPayCancelada`, o backend reconsulta a AppyPay. Se o estado mais recente já for `Success`, não cancela nem grava evento de cancelamento.
-- Cobranças `cancelada`, `falhada` ou `Success` não podem ser canceladas novamente ou reabertas. Para cobrar de novo, crie uma nova cobrança com outro `merchantTransactionId`.
+- Cobranças em qualquer estado terminal — `cancelada`, `falhada`, `Success`, `Failed`, `Cancelled` ou `Expired` — não podem ser canceladas novamente ou reabertas. Para cobrar de novo, crie uma nova cobrança com outro `merchantTransactionId`.
 - O evento `CobrancaAppyPayCancelada` é interno ao ledger Spuri. Uma referência/QR já emitido pode continuar tecnicamente pagável fora da plataforma até expirar; sucesso tardio gera `CobrancaAppyPayConflitoPosCancelamento`, sem alterar o status local cancelado.
 
 #### 19.10 POST /webhooks/appypay/gpo
 
 **Escopo da rota:** entrada pública para notificações AppyPay do método GPO.
 
@@ -7835,15 +7851,13 @@
   "paidAt": "2026-08-08T12:30:00Z"
 }
 ```
 
 **Response 200:** corpo vazio.
 
-**Regras de negócio:** evento sem autenticação válida retorna `401`; JSON inválido ou sem identificador retorna `400`; reentregas com o mesmo identificador respondem `200` e são idempotentes.
-
-#### 19.11 POST /webhooks/appypay/ref
+**Regras de negócio:** evento sem autenticação válida retorna `401`; JSON inválido ou sem identificador retorna `400`; reentregas com o mesmo identificador respondem `200` e são idempotentes. O `status` reportado é refletido na cobrança correspondente sempre que ela ainda não estiver num estado terminal — não só em sucesso: um webhook de `Failed`, `Cancelled` ou `Expired` também atualiza `status` (ver 19.7), evitando que a cobrança fique presa em `aguardando_pagamento` até uma consulta manual (19.6).
 
 **Escopo da rota:** entrada pública para notificações AppyPay do método REF.
 
 **Proteção:** igual ao webhook GPO: autenticação pelo segredo de webhook no único cabeçalho HTTP fixo da plataforma (`webhook_header_name`, sempre `X-Spuri-Webhook-Secret`).
 
 **Request JSON:**
```

---

## 7. Fora de escopo (não altere)

- `internal/db/` inteiro — Claude verificou: não existe nenhum `CHECK` constraint sobre a coluna `payload`/`status` em `financeiro_cobrancas` (ver `migrations/101_financeiro_appypay_base_v2.sql`), nem nenhum registro de tipo de evento que precise mudar (`ValidateEventType`/whitelists em `internal/db/`) — esta tarefa não precisa de nenhuma migration nova.
- `internal/domain/aggregates/financeiro.go` — verificado: o agregado é stateless (não faz replay nem valida transições de estado), nada a mudar.
- `internal/projections/financeiro_projection.go` — verificado: a projeção é um `ON CONFLICT DO UPDATE SET payload=EXCLUDED.payload` puro, não interpreta nem transforma `status` de forma nenhuma; toda a normalização acontece na camada de serviço (`internal/finance/`), antes do evento ser gravado ou depois de ser lido — a projeção nunca precisa saber do novo vocabulário.
- `internal/domain/models.go` — nenhuma struct ali é relacionada a cobranças/pagamentos.
- Qualquer função de `internal/finance/mensalidade_pendencias.go` e `internal/finance/mensalidade_pendencias_batch.go` (tarefas 62/63) — `PendenciasSemCobranca`, `PendenciasSemCobrancaEstudante`, `escopoMensalidadeEstudantes`, `estadosObrigacaoBatch`, `chargeIDsEscopoMensalidade` — nenhuma muda. Elas continuam devolvendo `MensalidadeMesView` com `Estado: EstadoPendente` para obrigações não pagas, exatamente como antes — o novo estado `EstadoCobrancaAguardandoPagamento` é exclusivo de **cobranças** (`CobrancaResumo`/`financeiro_cobrancas`), nunca de **obrigações** (`MensalidadeMesView`/`financeiro_mensalidade_configuracoes`), que são conceitos diferentes e não devem ser confundidos.
- `ListMensalidades`, `estadoObrigacao`, `precedenciaEstado` (`mensalidade.go`) — a lógica de estado de uma **obrigação** de mensalidade (`pendente`/`pago`/`anulado`) não muda; só a lógica de estado de uma **cobrança** real muda.
- `ListCobrancas`/`ListCobrancasEstudante`/`scanCobrancaResumo`/`CobrancaListResult` além das mudanças explícitas mostradas nos diffs da seção 4 — não altere mais nada nessas funções.
- Qualquer handler além de `ListarCobrancasAppyPay` e `ConsultarCobrancasEstudante`.
- Não crie nenhuma migration de banco — confirmado que não é necessária.
- Não crie nenhum novo tipo de evento no ledger — esta tarefa reaproveita os tipos de evento já existentes (`CobrancaAppyPaySolicitada`, `CobrancaAppyPayCriada`, `CobrancaAppyPayFalhou`, `CobrancaAppyPayConsultada`, `CobrancaAppyPayCancelada`, `CobrancaAppyPayConflitoPosCancelamento`, `QRCodeAppyPaySolicitado`, `QRCodeAppyPayFalhou`, `WebhookAppyPayRecebido`), só muda o VALOR de `status` dentro do payload desses eventos.
- **Não renomeie `Success`/`Failed`/`Cancelled`/`Expired`** para português — ver justificativa na seção 3.1. Só o estado "aguardando" muda de nome.
- **Não aplique a tarefa companion do frontend** (repositório `spuripainel`) como parte desta tarefa — são repositórios e PRs separados (ver seção 0 sobre a ordem de deploy).
- A rota nova `DELETE /dominios/academia/:codigo`, adicionada ao repositório recentemente, não tem nenhuma relação com este módulo financeiro — não precisa de nenhuma atenção nesta tarefa.

---
## 8. Checklist de validação (Codex deve executar e reportar o resultado de cada item)

Nenhum destes comandos requer PostgreSQL, Docker ou `psql`:

1. `grep -rn "pendencia_sem_cobranca\|PendenciaSemCobranca" --include="*.go" internal/` — deve devolver vazio (o campo foi completamente removido, inclusive da `Documentação da API.md`).
2. `grep -n "EstadoCobrancaAguardandoPagamento" internal/finance/mensalidade.go internal/finance/appypay.go | wc -l` — deve ser maior que 1 (a constante é declarada em `mensalidade.go` e usada em vários pontos de `appypay.go`).
3. `grep -n "chargeAbertaStatusExcluidos" internal/finance/mensalidade.go internal/finance/matricula.go` — deve mostrar a declaração da constante em `mensalidade.go` e 4 usos no total entre os dois arquivos (2 em cada).
4. `go build ./...` — sem erros.
5. `go vet ./...` — sem erros.
6. `gofmt -l internal/finance/mensalidade.go internal/finance/matricula.go internal/finance/appypay.go internal/finance/pagamentos_unificado.go internal/handlers/financeiro_handlers.go internal/finance/appypay_test.go internal/finance/pagamentos_unificado_test.go internal/finance/pagamentos_unificado_integration_test.go internal/finance/mensalidade_integration_test.go internal/finance/appypay_integration_test.go internal/handlers/financeiro_cobrancas_handlers_test.go internal/handlers/financeiro_pendencias_handlers_test.go` — vazio (nenhum arquivo mal formatado).
7. `go test ./...` — sem falhas (os testes de integração aparecem como `SKIP` sem `RUN_POSTGRES_INTEGRATION`, isso é esperado — Claude já validou o comportamento de tempo de execução completo desses testes com PostgreSQL real, ver seção 9).
8. `git diff --stat` — alterações apenas nos 5 arquivos de código da seção 4, nos 7 arquivos de teste da seção 5, e em `Documentação da API.md`, mais os documentos de conclusão desta tarefa.

Se qualquer item falhar, não prossiga — reporte o erro exato.

---

## 9. Evidência de validação (já executada por Claude — PostgreSQL 16 + Go 1.24 reais)

Claude validou esta tarefa inteira, do zero, num sandbox com PostgreSQL 16 e Go 1.24 reais instalados (não apenas lida a partir do código-fonte). Resumo das rodadas mais relevantes — os comandos e resultados abaixo são reais, não hipotéticos:

**Baseline (antes de qualquer mudança desta tarefa), suíte completa, banco recriado do zero:**
```
$ go build ./... && go vet ./... && go test ./... -count=1
BUILD_OK
VET_OK
ok  	spuri/cmd/server
ok  	spuri/internal/db
ok  	spuri/internal/domain/aggregates
ok  	spuri/internal/finance      2.346s
ok  	spuri/internal/handlers     0.874s
ok  	spuri/internal/middleware
ok  	spuri/internal/projections
ok  	spuri/internal/services
ok  	spuri/internal/storage
ok  	spuri/internal/utils
```

**Depois de todas as mudanças desta tarefa aplicadas, suíte completa, banco recriado do zero (mesmo comando, mesma limpeza de banco entre rodadas):**
```
$ go build ./... && go vet ./... && go test ./... -count=1
BUILD_OK
VET_OK
ok  	spuri/cmd/server	0.017s
ok  	spuri/internal/db	0.009s
ok  	spuri/internal/domain/aggregates	0.012s
ok  	spuri/internal/finance	2.285s
ok  	spuri/internal/handlers	0.829s
ok  	spuri/internal/middleware	0.007s
ok  	spuri/internal/projections	0.009s
ok  	spuri/internal/services	0.011s
ok  	spuri/internal/storage	0.004s
ok  	spuri/internal/utils	0.008s
```

**Nota sobre `-count=1` e banco fresco:** alguns testes pré-existentes da suíte (não relacionados a esta tarefa — ex. `TestIntegrationConfigureMensalidadeGravaNoLedgerEProjectaCorretamente`) fazem uma contagem global de eventos no ledger (`SELECT COUNT(*) FROM spuri_ledger WHERE event_type=...`) sem escopar por academia — só funcionam corretamente contra um banco recriado do zero para aquela rodada específica, e rodando a suíte inteira uma única vez (não em execuções repetidas acumulando dados no mesmo banco, nem com o cache de teste do Go reaproveitando um resultado antigo). Isso já era assim **antes** desta tarefa (não é algo que esta tarefa introduziu) — Claude só precisou descobrir isso na prática (recriando o banco entre rodadas, usando `-count=1`) para conseguir uma leitura confiável. Mencionando aqui só para o caso de você (Codex) rodar `go test ./...` mais de uma vez sobre o mesmo banco ao validar — não deveria acontecer já que você não roda os testes de integração sem `RUN_POSTGRES_INTEGRATION`, mas caso rode, este é o motivo se aparecer uma falha nesse padrão específico.

**Prova de que o teste de regressão do bug de filtro (seção 2.2) realmente pega o bug** — Claude reverteu deliberadamente só a guarda `finance.DeveIncluirPendenciasSemCobranca` em `ListarCobrancasAppyPay` (voltando ao `if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {` sem a checagem adicional) e rodou o teste novo:
```
$ go test ./internal/handlers/... -run 'TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas' -v -count=1
--- FAIL: TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas (0.03s)
    financeiro_pendencias_handlers_test.go:...: estado=Failed não deveria trazer nenhuma pendência sintética (nenhuma cobrança real existe nesta academia): {"pagamentos":[...3 itens com status "pendente"...],"total":3,"total_geral":3,...}
FAIL
```
Exatamente o sintoma relatado por Fredy ("recebi todos os pagamentos"). Depois de restaurar a correção:
```
$ go test ./internal/handlers/... -run 'TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas' -v -count=1
--- PASS: TestIntegrationListarCobrancasAppyPayFiltroEstadoExcluiPendenciasSinteticas (0.04s)
PASS
```

**Prova de que o bug de "cobrança Failed trava o mês para sempre" (seção 2.3, item 2) está corrigido:**
```
$ go test ./internal/finance/... -run 'TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa' -v -count=1
--- PASS: TestIntegrationMensalidadeComCobrancaFalhadaNaAppyPayPermiteNovaTentativa (0.09s)
PASS
```
(o teste cria uma cobrança, consulta a AppyPay simulada devolvendo `Failed`, confirma que uma segunda tentativa de pagamento do mesmo mês — que antes desta tarefa seria bloqueada — agora é aceita e cria uma cobrança nova e distinta da primeira.)

**Prova de que `AcceptWebhook` agora reflete estados não-sucesso (seção 2.3, item 3):**
```
$ go test ./internal/finance/... -run 'TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso' -v -count=1
--- PASS: TestIntegrationAcceptWebhookReflecteEstadoNaoSucesso (0.26s)
PASS
```

**Testes unitários novos (sem necessidade de banco), todos verdes:**
```
$ go test ./internal/finance/... -run 'TestNormalizeChargeStatus|TestEstadosCobrancaEquivalentes|TestCancelChargeAuthorizationAndTerminalStatuses|TestDeveIncluirPendenciasSemCobranca' -v -count=1
--- PASS: TestCancelChargeAuthorizationAndTerminalStatuses
--- PASS: TestNormalizeChargeStatus
--- PASS: TestEstadosCobrancaEquivalentes
--- PASS: TestDeveIncluirPendenciasSemCobranca (+ 10 subtestes)
PASS
```

**Ambiente usado para esta validação** (para referência, caso surja alguma dúvida sobre reprodutibilidade): Go 1.24.4 (`golang-1.24-go` via apt), PostgreSQL 16.15 (Ubuntu), variáveis `DATABASE_URL=postgres://spuri:spuri@localhost:5432/spuri?sslmode=disable`, `RUN_POSTGRES_INTEGRATION=1`, `APPYPAY_RESOURCE=test-resource`, `FINANCE_ENCRYPTION_KEY=test-only-secret-material-at-least-32`. Nenhuma dessas variáveis nem essa infraestrutura precisa existir no seu ambiente (Codex) — só documentado aqui para rastreabilidade de como a validação foi feita.

---

## 10. Critérios de aceite

- [ ] Os 5 diffs da seção 4 aplicados exatamente.
- [ ] Os 7 diffs da seção 5 aplicados exatamente.
- [ ] Os diffs da seção 6 aplicados exatamente em `Documentação da API.md`.
- [ ] `grep -rn "pendencia_sem_cobranca\|PendenciaSemCobranca" --include="*.go" internal/` devolve vazio.
- [ ] Todos os 8 itens do checklist de validação (seção 8) executados e reportados com sucesso.
- [ ] Nenhum arquivo fora do escopo desta tarefa foi alterado (seção 7).

---
## 11. Procedimento de conclusão

1. Mover este arquivo para `docs/Tarefas feitas/`, com `status: concluido` e `concluido: <data de hoje>` no frontmatter (numeração 66, a próxima disponível no momento em que este documento foi escrito — a tarefa 65, no repositório `spuripainel`, já ocupa esse número lá; os dois repositórios têm sequências de numeração independentes).
2. Um commit único, mensagem: `Novo modelo de estados de cobranca (aguardando_pagamento) e correcao do filtro estado/tipo na lista unificada de pagamentos`.
3. Reportar a Fredy: resultado de cada item do checklist e `git diff --stat` do commit. Nenhuma validação adicional com PostgreSQL real é necessária — já foi feita (seção 9).
4. **Avisar explicitamente a Fredy** que existe uma tarefa companion no repositório `spuripainel` (frontend) que precisa ser aplicada junto — ela remove o campo `pendencia_sem_cobranca` do lado do frontend e atualiza o filtro de estado da UI para o novo vocabulário. Diferente de tarefas anteriores (64/65), desta vez a ordem de deploy importa: este backend remove um campo da resposta JSON que o frontend hoje depende para funcionar corretamente — não deixar este backend em produção sozinho por muito tempo sem o frontend acompanhando (ver seção 0).

**Nenhuma etapa deste procedimento remove ou altera qualquer código relacionado à inscrição de estudantes em academias, matrícula, cadastro, turmas ou vínculo de estudante à academia** — todas as alterações estão contidas ao módulo financeiro de cobranças/pagamentos (`internal/finance/mensalidade.go`, `internal/finance/matricula.go`, `internal/finance/appypay.go`, `internal/finance/pagamentos_unificado.go` e seus testes, `internal/handlers/financeiro_handlers.go` e seus testes) e à documentação da API.
