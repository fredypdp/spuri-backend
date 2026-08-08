---
criado: 2026-08-08 00:00
origem: Depuração de logs de produção (Render) + code review em github.com/fredypdp/spuri-backend — conversa com Claude
status: feito
---

# 19 — Eliminar polling residual que impede o NeonDB de entrar em idle (feito)

## Prompt recomendado para executar a atualização

Implemente exatamente as quatro mudanças descritas na seção "Escopo obrigatório" deste documento: (1) um mecanismo de "wake" orientado a evento no `Manager` de projeções, (2) um hook de escrita no pacote `db` que dispara esse wake após cada gravação confirmada no ledger, (3) a ligação desse hook em `cmd/server/main.go`, e (4) o aumento do teto de backoff do `sweepPending` em `internal/jobs/worker.go`. Não introduza `LISTEN/NOTIFY` do PostgreSQL, filas externas (Redis, RabbitMQ, etc.), nem qualquer dependência nova — o mecanismo deve ser inteiramente in-process, usando apenas `channel` do Go. Não altere regras de negócio, agregados, projeções individuais, handlers HTTP, `cleanupLoop`, nem a configuração de pool de conexões (`MaxConnections`, `MaxIdleConns`, `ConnMaxIdleTime`), pois já foram ajustados numa tarefa anterior (`docs/Tarefas feitas/Depurar infraestrutura para migracao NeonDB com scale-to-zero.md`). Ao final, rode `gofmt`, `go build ./...`, `go vet ./...` e `go test ./...`, e confirme cada item da seção "Critérios de aceite" antes de reportar a tarefa como concluída.

## Contexto

Esta tarefa é a **continuação direta** de `docs/Tarefas feitas/Depurar infraestrutura para migracao NeonDB com scale-to-zero.md` (concluída em 2026-07-10). Aquela tarefa já resolveu os pontos mais graves:

- removeu o `startHealthCheck()` que fazia `PingContext` a cada 10 segundos;
- removeu os wrappers manuais `QueryWithRetry` / `QueryRowWithRetry` / `ExecWithRetry` que reciclavam o pool agressivamente;
- reduziu `MaxConnections` para um valor conservador;
- trocou o polling fixo de 1 segundo do Projection Manager por backoff exponencial com teto de **30 segundos**;
- trocou o polling fixo de 30 segundos do `sweepPending` (worker de jobs) por backoff exponencial com teto de **5 minutos**.

Essa tarefa anterior deixou explicitamente registrado, na seção "Fora de escopo", que `LISTEN/NOTIFY`, fila externa e reescrever o Projection Manager para consumidor único ficariam para uma tarefa futura — e, na seção "Riscos e mitigação", anotou que "Futuro: substituir polling por `LISTEN/NOTIFY` ou fila externa" seria necessário para reduzir ainda mais o teto de 30 segundos.

## Problema principal (evidência dos logs de produção)

Logs do Render de 2026-08-08, cobrindo ~15 minutos sem nenhuma requisição de usuário, mostram consultas ao NeonDB **a cada 30 segundos, indefinidamente**:

```
[DEBUG] LastID: 2798 para projection: solicitacoes_matricula
[DEBUG] LastID: 2798 para projection: financeiro
[DEBUG] Projection Manager backoff: processados=false próximo_intervalo=2s
...
[DEBUG] Projection Manager backoff: processados=false próximo_intervalo=30s
```
seguido de repetições de `LastID` a cada ~30s pelo resto da janela observada, e:
```
[worker] varredura de jobs ativos — ativos=0 fila=0 próximo_intervalo=5m0s
```
a cada 5 minutos.

Ambos os comportamentos são exatamente o que a tarefa anterior implementou (30s e 5min são os tetos definidos por design naquela tarefa, não um bug). O problema é que **o teto de 30 segundos ainda é curto demais** perante o auto-suspend do NeonDB: confirmado com a documentação oficial da Neon, o plano **Free** suspende o compute após **5 minutos fixos** de inatividade, valor esse que não pode ser alterado nesse plano ([neon.com/docs/introduction/scale-to-zero](https://neon.com/docs/introduction/scale-to-zero)). Com uma consulta garantida a cada 30 segundos, esse período de inatividade nunca se completa — o compute nunca chega a acumular 5 minutos "limpos" e nunca suspende.

Adicionalmente, confirmei por leitura de código que **11 das 13 projeções registradas** (`AcademiaProjection`, `AdminProjection`, `CursosProjection`, etc.) implementam a mesma consulta de checkpoint (`SELECT last_processed_event_id FROM projection_checkpoints ...`) sem passar pelo helper `BaseProjection.GetLastProcessedEventIDByName` — por isso não aparecem nos logs (só `FinanceiroProjection` e `SolicitacaoMatriculaProjection` usam o helper que tem `log.Printf`). Isso não é um bug funcional, mas significa que o volume real de queries por ciclo é maior do que os logs sugerem (até 13 queries a cada ciclo do Manager, não 2).

## Objetivo

Reduzir a atividade de fundo no NeonDB ao mínimo possível **sem perder responsividade real**, através de um mecanismo "acorda quando há trabalho, dorme quando não há" ao invés de um temporizador fixo curto:

- Quando uma escrita real acontece no ledger (`spuri_ledger`), o Projection Manager deve processar o novo evento quase imediatamente — como acontece hoje, sem regressão de UX.
- Quando **não** há escritas, o Manager não deve tocar o banco a cada 30 segundos — deve poder ficar em silêncio por um período bem mais longo (a "rede de segurança"), permitindo que o NeonDB realmente complete uma janela de inatividade e suspenda o compute.
- O worker de jobs assíncronos já é, na prática, orientado a evento (`Enqueue()` é chamado diretamente na criação do job, em `internal/handlers/sistema_handler.go`, `internal/handlers/job_handlers.go` e `internal/handlers/async_batch_handlers.go`) — `sweepPending()` é apenas uma rede de segurança para recuperação após reinício ou fila cheia, então seu teto pode subir com segurança.
- Preservar 100% a semântica de Event Sourcing/CQRS, a consistência eventual das projeções e o processamento assíncrono de jobs.

## Escopo obrigatório

### 1. Adicionar mecanismo de "wake" ao Projection Manager — `internal/projections/manager.go`

#### Diagnóstico

`Manager.StartProcessing()` só tem duas formas de decidir quando rodar o próximo ciclo: o timer de backoff (`timer.C`) ou o cancelamento do contexto (`m.ctx.Done()`). Não existe forma de "acordar" o loop antes do timer disparar.

#### Correção esperada

Adicionar um canal de sinalização à struct `Manager`:

```go
type Manager struct {
	client       *db.Client
	eventStore   *db.EventStore
	projections  map[string]Projection
	ctx          context.Context
	cancel       context.CancelFunc
	pollInterval time.Duration
	batchSize    int
	mu           sync.Mutex
	rebuildMu    sync.Mutex
	rebuilding   string
	wakeCh       chan struct{}
}
```

Inicializar em `NewManager`:

```go
func NewManager(client *db.Client) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		client:       client,
		eventStore:   db.NewEventStore(client),
		projections:  make(map[string]Projection),
		ctx:          ctx,
		cancel:       cancel,
		pollInterval: 1 * time.Second,
		batchSize:    100,
		wakeCh:       make(chan struct{}, 1),
	}
}
```

Adicionar o método público `Wake`, não bloqueante:

```go
// Wake sinaliza ao Manager que há uma escrita nova no ledger para processar
// imediatamente, sem esperar o próximo ciclo de backoff. É seguro chamar de
// qualquer goroutine. Se já houver um sinal pendente (Manager ainda não
// consumiu o anterior), a chamada é descartada — não há necessidade de
// empilhar sinais, pois o próximo ciclo já processa tudo que estiver pendente
// no ledger, não apenas o evento que motivou a chamada.
func (m *Manager) Wake() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}
```

Alterar `StartProcessing` para: (a) escutar `wakeCh` além do timer, e (b) usar um teto de backoff muito maior, já que o wake cobre o caso comum:

```go
func (m *Manager) StartProcessing() {
	log.Println("[DEBUG] Iniciando processamento de projeções")

	currentInterval := m.pollInterval
	// maxInterval agora é uma rede de segurança, não o mecanismo principal de
	// detecção de eventos novos — isso é papel do wakeCh, acionado por
	// db.SetLedgerWriteHook após cada escrita confirmada no ledger. O NeonDB
	// free tier suspende o compute após 5 minutos fixos de inatividade (não
	// configurável nesse plano); 20 minutos dá margem de 4× para garantir uma
	// janela real e completa de inatividade entre um ciclo e outro.
	maxInterval := 20 * time.Minute

	for {
		select {
		case <-m.ctx.Done():
			log.Println("[DEBUG] Parando processamento")
			return
		default:
		}

		processed, err := m.processNewEvents()
		if err != nil {
			log.Printf("[ERROR] Erro ao processar eventos: %v", err)
		}

		previousInterval := currentInterval
		if processed {
			currentInterval = m.pollInterval
		} else {
			currentInterval = projectionBackoff(currentInterval, maxInterval)
		}
		if currentInterval != previousInterval {
			log.Printf("[DEBUG] Projection Manager backoff: processados=%t próximo_intervalo=%s", processed, currentInterval)
		}

		timer := time.NewTimer(currentInterval)
		select {
		case <-m.ctx.Done():
			timer.Stop()
			log.Println("[DEBUG] Parando processamento")
			return
		case <-timer.C:
		case <-m.wakeCh:
			timer.Stop()
			currentInterval = m.pollInterval
			log.Println("[DEBUG] Projection Manager acordado por escrita no ledger")
		}
	}
}
```

#### Cuidados

- Não alterar `processNewEvents`, `processProjection`, `processEventWithRetry`, `commitCheckpoint`, `RebuildProjection`, `RebuildAllProjections`, `executeRebuild`, `waitForDatabaseReadiness` nem qualquer outra função do arquivo.
- Não alterar `projectionBackoff` (a função auxiliar de backoff exponencial já existente) — apenas o valor de `maxInterval` passado a ela dentro de `StartProcessing`.
- O valor `20 * time.Minute` é um ponto de partida. Se o auto-suspend configurado no console do NeonDB for diferente de 5 minutos, ajustar proporcionalmente (ver seção "Checks manuais").

---

### 2. Adicionar hook de escrita no pacote `db`

#### Diagnóstico

O pacote `projections` já importa `db`, então não é possível o pacote `db` importar `projections` de volta (ciclo de import). Para o `AggregateRepository.Save`/`SaveWithAudit` conseguir "avisar" o `Manager` sem esse ciclo, o pacote `db` deve expor um callback genérico (`func()`), configurado uma única vez a partir de `main.go`.

#### Pré-validação obrigatória

Antes de implementar, confirmar que `appendDirect` (em `internal/db/event_store.go`) continua sem chamadores fora do próprio pacote — é o único outro caminho de escrita no ledger além de `AggregateRepository.Save`/`SaveWithAudit`, e não deve ser necessário tocá-lo:

```bash
rg -n "appendDirect\b" internal/ cmd/
```

O resultado esperado é que só apareçam ocorrências dentro de `internal/db/event_store.go`. Se aparecer uso fora do pacote `db`, reportar antes de continuar — pode indicar que existe outro caminho de escrita no ledger que também precisa dispersar o wake, fora do escopo original desta tarefa.

#### Correção esperada

Criar um novo arquivo `internal/db/ledger_hook.go`:

```go
package db

// ledgerWriteHook, quando definido, é chamado de forma não bloqueante toda vez
// que uma escrita no ledger é confirmada com sucesso via AggregateRepository.Save
// ou SaveWithAudit. Existe para acordar o Projection Manager sem que o pacote
// db precise importar o pacote projections (evitaria import cycle, já que
// projections importa db). Definido uma única vez em cmd/server/main.go,
// antes de router.Run(), a partir de projManager.Wake.
var ledgerWriteHook func()

// SetLedgerWriteHook registra o callback chamado após cada escrita confirmada
// no ledger. Passar nil desativa o callback. Deve ser chamado uma única vez
// durante a inicialização do servidor — não é seguro chamar concorrentemente
// com escritas em andamento.
func SetLedgerWriteHook(fn func()) {
	ledgerWriteHook = fn
}

// notifyLedgerWritten dispara o hook, se existir. Não bloqueia o caminho de
// escrita: o hook (Manager.Wake) já é não bloqueante por construção.
func notifyLedgerWritten() {
	if ledgerWriteHook != nil {
		ledgerWriteHook()
	}
}
```

Em `internal/db/repository.go`, chamar `notifyLedgerWritten()` **depois** de `tx.Commit()` ter sucesso, em `Save`:

```go
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("erro ao confirmar transação: %w", err)
	}

	aggregate.ClearUncommittedEvents()
	notifyLedgerWritten()
	return nil
}
```

E, de forma idêntica, em `SaveWithAudit`:

```go
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("erro ao confirmar transação: %w", err)
	}

	aggregate.ClearUncommittedEvents()
	notifyLedgerWritten()
	return nil
}
```

#### Cuidados

- Não chamar `notifyLedgerWritten()` antes de `tx.Commit()` retornar sem erro — só uma escrita realmente confirmada deve acordar o Manager.
- Não chamar dentro de `AppendTx` nem `appendDirect` (esses rodam dentro de transações potencialmente ainda não confirmadas).
- Não alterar `Load`, `getAggregateVersionTx`, `SaveSnapshot`, `dbEvent`, `dbEventWithAudit` nem qualquer outra função de `repository.go`.
- Não alterar `event_store.go` além de confirmar a pré-validação acima — nenhuma mudança de código é esperada nesse arquivo.

---

### 3. Ligar o hook em `cmd/server/main.go`

#### Correção esperada

Em `initProjections()`, logo após o bloco de `RegisterProjection`, ligar o hook antes (ou imediatamente ao lado) de iniciar o processamento:

```go
	projManager.RegisterProjection("financeiro", projections.NewFinanceiroProjection(dbClient))

	db.SetLedgerWriteHook(projManager.Wake)

	go projManager.StartProcessing()
	return nil
}
```

#### Cuidados

- Não mover a ligação para depois de `router.Run()` — deve estar concluída antes do servidor aceitar tráfego.
- Não alterar `initDB`, `initStorage`, `initJobs`, `setupRouter` nem qualquer outra função de `main.go`.
- `db` já está importado em `main.go`; não é necessário adicionar import novo.

---

### 4. Aumentar o teto de backoff do worker de jobs — `internal/jobs/worker.go`

#### Diagnóstico

`sweepPending()` já é, na prática, apenas uma rede de segurança: jobs criados durante operação normal são despachados imediatamente por chamadas diretas a `w.Enqueue(j)` logo após `store.Enqueue(...)`, em três handlers (`sistema_handler.go`, `job_handlers.go`, `async_batch_handlers.go`). `sweepPending` só importa para recuperar jobs que ficaram "pending" no banco sem entrar na fila em memória — reinício do servidor ou fila cheia. Não há razão funcional para manter o teto atual de 5 minutos tão baixo.

#### Correção esperada

Alterar apenas a constante `maxInterval` dentro de `sweepPending`:

```go
func (w *Worker) sweepPending(ctx context.Context) {
	minInterval := 30 * time.Second
	maxInterval := 30 * time.Minute
	currentInterval := minInterval
	...
```

Nada mais nessa função muda.

#### Cuidados

- Não alterar `minInterval` (mantém 30 segundos quando há jobs ativos).
- Não alterar `nextBackoff`, `loop`, `process`, `Enqueue`, `markInFlight`, `unmarkInFlight`, `cleanupLoop`.
- Não introduzir nenhum mecanismo de wake aqui — não é necessário, pois o despacho real já é imediato via `Enqueue()`.

## Fora de escopo

Não fazer nesta tarefa:

- Implementar `LISTEN/NOTIFY` nativo do PostgreSQL (isso exigiria manter uma conexão dedicada de `LISTEN` aberta, o que por si só pode impedir o NeonDB de suspender — avaliar separadamente no futuro, com medição, se ainda fizer sentido depois desta tarefa).
- Introduzir Redis, fila externa, ou qualquer dependência nova.
- Alterar `cleanupLoop()` (`internal/jobs/worker.go`) — teto de 1 hora continua aceitável.
- Alterar `MaxConnections`, `MaxIdleConns`, `ConnMaxIdleTime` em `internal/db/client.go` (já ajustados na tarefa anterior).
- Alterar o endpoint `/health` ou a lógica de `dbClient.Health()`.
- Preparar o `wakeCh` para múltiplas instâncias/réplicas do backend (o mecanismo é in-process; ver "Riscos e mitigação").
- Unificar as 11 projeções que reimplementam `GetLastProcessedEventID` manualmente ao invés de usar `BaseProjection.GetLastProcessedEventIDByName` — é só uma inconsistência de logging, não afeta o consumo de compute, e pode virar uma tarefa de limpeza separada.
- Alterar regras de negócio, agregados, projeções individuais, handlers HTTP ou contratos de API.
- Ativar rate limiting, otimizar queries de listagem, adicionar métricas/observabilidade novas.

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual.
2. Rodar a pré-validação `rg -n "appendDirect\b" internal/ cmd/` e confirmar que não há chamadores externos ao pacote `db`.
3. Criar `internal/db/ledger_hook.go` conforme especificado.
4. Editar `internal/db/repository.go`: adicionar `notifyLedgerWritten()` em `Save` e `SaveWithAudit`, logo após `tx.Commit()` bem-sucedido.
5. Rodar `gofmt` e `go build ./internal/db/...`.
6. Editar `internal/projections/manager.go`: campo `wakeCh`, inicialização em `NewManager`, método `Wake`, e o `select` adicional em `StartProcessing` com o novo `maxInterval`.
7. Rodar `gofmt` e `go build ./internal/projections/...`.
8. Editar `cmd/server/main.go`: adicionar `db.SetLedgerWriteHook(projManager.Wake)` em `initProjections()`.
9. Editar `internal/jobs/worker.go`: alterar apenas o valor de `maxInterval` em `sweepPending`.
10. Rodar `gofmt` em todos os arquivos alterados.
11. Rodar `go build ./...`, `go vet ./...`, `go test ./...`.
12. Revisar o diff completo e confirmar que nenhum arquivo fora dos quatro listados no escopo foi alterado.

## Critérios de aceite

A tarefa só deve ser considerada concluída quando todos os itens abaixo forem verdadeiros:

- [ ] `internal/db/ledger_hook.go` existe, com `SetLedgerWriteHook`, `notifyLedgerWritten` e a variável `ledgerWriteHook`.
- [ ] `internal/db/repository.go`: `Save` chama `notifyLedgerWritten()` imediatamente após `tx.Commit()` bem-sucedido.
- [ ] `internal/db/repository.go`: `SaveWithAudit` chama `notifyLedgerWritten()` imediatamente após `tx.Commit()` bem-sucedido.
- [ ] `internal/projections/manager.go`: struct `Manager` contém o campo `wakeCh chan struct{}`.
- [ ] `internal/projections/manager.go`: `NewManager` inicializa `wakeCh: make(chan struct{}, 1)`.
- [ ] `internal/projections/manager.go`: existe o método `func (m *Manager) Wake()`, não bloqueante (usa `select`/`default`).
- [ ] `internal/projections/manager.go`: `StartProcessing` tem `maxInterval` maior que `30 * time.Second` (ex.: `20 * time.Minute`) e escuta `m.wakeCh` no `select` junto de `timer.C` e `m.ctx.Done()`.
- [ ] `cmd/server/main.go`: `initProjections()` chama `db.SetLedgerWriteHook(projManager.Wake)`.
- [ ] `internal/jobs/worker.go`: `sweepPending` tem `maxInterval := 30 * time.Minute` (ou valor equivalente, maior que os 5 minutos anteriores), mantendo `minInterval := 30 * time.Second`.
- [ ] Nenhum outro arquivo além de `internal/db/ledger_hook.go`, `internal/db/repository.go`, `internal/projections/manager.go`, `cmd/server/main.go` e `internal/jobs/worker.go` foi alterado.
- [ ] `rg -n "appendDirect\b" internal/ cmd/` continua mostrando apenas ocorrências dentro de `internal/db/event_store.go`.
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `go test ./...` passa, ou qualquer falha é documentada com causa comprovadamente não relacionada a esta tarefa.

## Checks manuais recomendados após deploy em staging

### 1. Auto-suspensão do NeonDB — valor confirmado, não é necessário verificar

Confirmado junto à documentação oficial da Neon: no plano **Free**, o auto-suspend do compute é **fixo em 5 minutos de inatividade e não pode ser alterado** (nem via console, nem via `neon branches create --suspend-timeout`, que só tem efeito em planos pagos). Fonte: [neon.com/docs/introduction/scale-to-zero](https://neon.com/docs/introduction/scale-to-zero) e [neon.com/docs/manage/computes#edit-a-compute](https://neon.com/docs/manage/computes#edit-a-compute).

Isso não é uma mudança de código nem uma verificação pendente — é uma constante conhecida do ambiente. Os valores de `maxInterval` definidos nesta tarefa (20 minutos no Projection Manager, 30 minutos no worker de jobs) já foram escolhidos com margem de 4× e 6× acima desse valor fixo, especificamente para garantir que uma janela completa e "limpa" de 5 minutos sempre ocorra entre um ciclo de rede de segurança e outro, permitindo a suspensão real do compute. Não é necessário nenhum ajuste adicional nesses valores por causa deste item — apenas confirmar, na validação da seção seguinte, que a suspensão de fato acontece dentro da janela observada.

Se o projeto migrar para um plano pago da Neon no futuro, este item deve ser revisitado: em planos pagos o auto-suspend é configurável (de 1 minuto até "always on"), o que pode justificar recalibrar `maxInterval` — mas isso fica fora do escopo desta tarefa.

### 2. Confirmar ausência de atividade agressiva em idle real

1. Subir o backend em staging apontando para um banco de teste.
2. Não enviar nenhuma requisição por pelo menos 25 minutos.
3. Observar logs do backend:
   - não deve haver `[DEBUG] LastID` nem `Projection Manager backoff` em intervalos menores que o novo `maxInterval`;
   - não deve haver `[worker] varredura de jobs ativos` em intervalos menores que 30 minutos.
4. Observar métricas do NeonDB (painel do projeto):
   - o compute deve aparecer como suspenso ("idle"/"suspended") em algum ponto dentro dessa janela de 25 minutos;
   - se isso não acontecer, revisar se algum monitor externo (UptimeRobot, health check do Render configurado para `/health`, etc.) está batendo no banco em paralelo — fora do escopo desta tarefa, mas precisa ser identificado.

### 3. Confirmar retomada quase imediata após escrita real

1. Com o backend ainda em idle (nenhuma atividade há vários minutos), disparar uma operação simples que grave um evento no ledger (ex.: criar ou atualizar um registro via API).
2. Confirmar no log a linha `[DEBUG] Projection Manager acordado por escrita no ledger`.
3. Confirmar que a projeção correspondente é atualizada sem atraso perceptível ao usuário (equivalente ao comportamento atual, não deve haver regressão).

### 4. Confirmar que jobs assíncronos continuam imediatos

1. Criar um job assíncrono pequeno via qualquer endpoint que use `store.Enqueue` + `w.Enqueue`.
2. Confirmar que ele é processado imediatamente (via `Enqueue` direto, não via `sweepPending`).
3. Opcional: simular reinício do servidor com um job "pending" no banco e confirmar que `sweepPending` ainda o recupera dentro do novo teto de 30 minutos.

## Riscos e mitigação

### Risco: qualquer futuro caminho de escrita no ledger que não passe por `AggregateRepository.Save`/`SaveWithAudit` não vai disparar o wake

- Hoje isso não acontece (`appendDirect` não tem chamadores externos ao pacote `db`, conforme pré-validação).
- Mitigação: o comentário em `ledger_hook.go` e nesta tarefa deixam isso documentado. Se uma tarefa futura criar um novo caminho de escrita, deve chamar `notifyLedgerWritten()` também, ou passar pelo repositório existente.
- Rede de segurança: mesmo que isso aconteça sem ser notado, o `maxInterval` de 20 minutos garante que o evento será processado eventualmente, só que mais devagar — não há perda de dados, apenas atraso.

### Risco: múltiplas instâncias/réplicas do backend no futuro

- `wakeCh` é in-process: só acorda o `Manager` da mesma instância que fez a escrita. Se o Spuri passar a rodar com múltiplas réplicas, uma escrita numa instância não acorda o Manager de outra instância — cada uma dependeria do seu próprio `maxInterval` como rede de segurança.
- Mitigação: aceitável para o cenário atual (instância única no Render). Se houver decisão de escalar horizontalmente, tratar como tarefa própria (ex.: `LISTEN/NOTIFY` real do Postgres, que já foi intencionalmente deixado fora do escopo aqui).

### Risco: `maxInterval` de 20 minutos parecer "alto demais" numa primeira leitura

- Isso é intencional: o objetivo desta tarefa é justamente parar de depender do timer como mecanismo principal. Com o wake ativo, o timer só existe como rede de segurança para casos excepcionais (ver risco acima) — não é o caminho normal de detecção de eventos. O valor de 20 minutos foi calibrado com margem de 4× sobre os 5 minutos fixos de auto-suspend do NeonDB free tier (confirmado, não configurável nesse plano — [neon.com/docs/introduction/scale-to-zero](https://neon.com/docs/introduction/scale-to-zero)), não é um número arbitrário.

### Risco: monitor externo de uptime continuar batendo no `/health` em intervalo curto

- Fora do escopo desta tarefa (não é mudança de código), mas invalida o resultado esperado se não for verificado. Ver "Checks manuais", item 2.

## Observações finais

Esta tarefa assume que a tarefa anterior (`Depurar infraestrutura para migracao NeonDB com scale-to-zero.md`) já está em produção — os tetos de 30 segundos e 5 minutos mencionados no diagnóstico são o resultado esperado e correto daquela tarefa, não um bug novo. O que muda aqui é a estratégia: sair de "polling curto para tudo" para "wake imediato quando há trabalho real + polling raro como rede de segurança", o que deve permitir que o NeonDB efetivamente entre em modo idle no free tier sem sacrificar a responsividade percebida pelo usuário.
