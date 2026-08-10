---
criado: 2026-08-10 00:00
origem: Depuração — Loops de polling residuais mantendo o NeonDB ativo.md (docs/Debbugs/) — conversa com Claude, a partir de evidência do painel Monitoring/System Operations do NeonDB
status: a fazer
---

# 21 — Eliminar rampa residual de backoff que mantém o NeonDB ativo entre 15 e 25 minutos (a fazer)

## Prompt recomendado para executar a atualização

Implemente exatamente as três mudanças descritas na seção "Escopo obrigatório" deste documento: (1) eliminar a rampa gradual de backoff em `internal/projections/manager.go`, substituindo-a por uma transição direta e imediata para o teto (`maxInterval`) assim que não houver mais trabalho a processar, removendo a função `projectionBackoff` que fica sem uso; (2) aplicar a mesma mudança em `internal/jobs/worker.go`, função `sweepPending`, removendo `nextBackoff`; (3) alterar o intervalo do `cleanupLoop` de 1 hora para 24 horas. Não altere `wakeCh`, `Wake()`, `Enqueue()`, `notifyLedgerWritten`, `SetLedgerWriteHook`, os valores de `pollInterval`/`minInterval` (o piso rápido usado enquanto há trabalho real sendo drenado), os valores de `maxInterval` de 20 minutos e 30 minutos já definidos, `MaxConnections`, `MaxIdleConns`, o endpoint `/health`, nem qualquer regra de negócio, agregado, projeção individual ou handler HTTP. Ao final, rode `gofmt`, `go build ./...`, `go vet ./...` e `go test ./...`, e confirme cada item da seção "Critérios de aceite" antes de reportar a tarefa como concluída.

## Contexto

Esta tarefa é a **continuação direta** de `docs/Tarefas feitas/19 - Eliminar polling residual que impede o NeonDB de entrar em idle.md` (concluída em 2026-08-08). Aquela tarefa implementou corretamente o mecanismo de wake orientado a evento (`wakeCh` no Projection Manager, acionado por `db.SetLedgerWriteHook` após cada escrita confirmada no ledger) e elevou os tetos de backoff (`maxInterval`) para 20 minutos no Projection Manager e 30 minutos no `sweepPending` do worker de jobs — ambos calibrados com margem sobre os 5 minutos fixos de auto-suspend do NeonDB free tier.

O que a Tarefa 19 não cobriu — porque na época não havia evidência disso — é que **o piso de reinício do backoff (não o teto) é o que na prática mantém o compute acordado por 15 a 25 minutos a cada janela de atividade real**, mesmo com o teto correto. Isso foi identificado em `docs/Debbugs/Depuração — Loops de polling residuais mantendo o NeonDB ativo.md`, a partir do painel Monitoring → System Operations do NeonDB, que mostrou ciclos start/suspend com durações de 15, 15, 23 e 25 minutos — muito acima do limiar de 5 minutos.

### Resumo do diagnóstico (detalhes completos no relatório de depuração)

- **Projection Manager** (`internal/projections/manager.go`, `StartProcessing`): toda escrita real no ledger reinicia `currentInterval` para `pollInterval` (1 segundo). Quando a escrita termina de ser processada e não há mais nada novo, o backoff **dobra gradualmente a partir de 1 segundo** (1s→2s→4s→8s→16s→32s→64s→128s→256s), e só o próximo intervalo (512s) finalmente ultrapassa os 5 minutos do NeonDB. Isso significa **~8,5 minutos de consultas contínuas ao banco depois da última escrita real**, antes que uma janela de inatividade completa possa ocorrer.
- **Worker de jobs** (`internal/jobs/worker.go`, `sweepPending`): mesmo padrão, piso de 30 segundos, reiniciado sempre que existir qualquer job "pending"/"processing". Rampa de 30s→60s→120s→240s soma **~7,5 minutos de consultas contínuas** depois do último job ativo desaparecer.
- **Worker de jobs** (`internal/jobs/worker.go`, `cleanupLoop`): ticker fixo de 1 hora, incondicional, sem relação com atividade real — garante ao menos 24 despertares/dia mesmo em dias sem nenhum uso.

Uma sessão real de uso (ex.: lançamento de notas de uma turma ao longo de alguns minutos, cada lançamento sendo uma escrita no ledger) reinicia a rampa do Projection Manager a cada escrita. Sessão real (poucos minutos) + cauda de desaceleração (~8,5min) explica com precisão as janelas de 15–25 minutos observadas no painel da Neon.

### Decisão de produto confirmada (define o escopo desta tarefa)

Ficou definido nesta conversa que, na fase atual (zero-budget, prioridade em poupança máxima de compute), **o usuário/cliente recebe atualização de processamento apenas quando pronto, não em tempo real via este canal de poll de segurança** — isso é aceitável porque a responsividade real a eventos já é garantida por um caminho completamente separado e não afetado por esta tarefa:

- Toda escrita no ledger já aciona `wakeCh`/`Manager.Wake()` de forma quase instantânea, independente do valor de `maxInterval` — o poll por timer é *apenas* a rede de segurança para o caso (hoje inexistente, ver Tarefa 19) de uma escrita não passar por esse caminho.
- Todo job assíncrono criado em operação normal já é despachado imediatamente via `w.Enqueue(j)`, chamado diretamente nos três handlers que criam jobs (`sistema_handler.go`, `job_handlers.go`, `async_batch_handlers.go`) — `sweepPending` também é *apenas* rede de segurança para recuperação após reinício do servidor ou fila cheia.

Ou seja: **esta tarefa não remove nem atrasa nenhum caminho de atualização real** — ela apenas torna a rede de segurança (que só existe para cobrir cenários excepcionais já raros) mais lenta para reagir nesses cenários excepcionais, em troca de eliminar por completo a rampa de minutos abaixo do limiar de suspensão do NeonDB.

## Objetivo

Eliminar a rampa gradual de backoff nos dois loops de fundo, substituindo-a por uma transição binária e imediata:

- **Enquanto há trabalho sendo processado** (evento novo no ledger / job ativo): manter o piso rápido atual (`pollInterval` = 1s / `minInterval` = 30s), sem nenhuma mudança — isso preserva a velocidade de drenagem de um backlog real (ex.: rebuild de projeção, importação em lote maior que `batchSize` = 100, recuperação de muitos jobs após reinício).
- **No exato momento em que não há mais trabalho** (evento processado e nada de novo apareceu / nenhum job ativo): pular diretamente para o teto (`maxInterval` = 20min / 30min), sem nenhum passo intermediário. Isso elimina inteiramente a janela de vários minutos em que o backoff ainda está "descendo" a partir de um valor baixo.
- Reduzir a frequência do `cleanupLoop` de 1 hora para 24 horas, já que jobs só se tornam elegíveis para limpeza depois de 24 horas concluídos (`cutoff := time.Now().Add(-24 * time.Hour)` em `internal/jobs/store.go`) — rodar de hora em hora não traz nenhum benefício funcional sobre rodar uma vez por dia.
- Remover as funções `projectionBackoff` e `nextBackoff`, que ficam sem nenhum ponto de chamada após esta mudança (confirmado: cada uma tem exatamente um call site hoje, ambos substituídos por esta tarefa).

## Escopo obrigatório

### 1. Projection Manager — `internal/projections/manager.go`

#### Diagnóstico

Em `StartProcessing` (linhas 119–167), o trecho:

```go
		previousInterval := currentInterval
		if processed {
			currentInterval = m.pollInterval
		} else {
			currentInterval = projectionBackoff(currentInterval, maxInterval)
		}
		if currentInterval != previousInterval {
			log.Printf("[DEBUG] Projection Manager backoff: processados=%t próximo_intervalo=%s", processed, currentInterval)
		}
```

faz o backoff dobrar gradualmente a partir de `m.pollInterval` (1 segundo) toda vez que `processed` passa de `true` para `false`, gastando ~8,5 minutos em consultas com intervalo menor que 5 minutos antes de alcançar um intervalo seguro.

#### Correção esperada

Substituir o bloco acima por:

```go
		previousInterval := currentInterval
		if processed {
			// Ainda há trabalho sendo drenado (ex.: backlog maior que
			// batchSize) — manter o piso rápido para não atrasar o
			// processamento de eventos reais em volume.
			currentInterval = m.pollInterval
		} else {
			// Não há mais nada novo para processar. Pular diretamente para
			// o teto em vez de uma rampa gradual: a responsividade a uma
			// PRÓXIMA escrita real continua garantida por wakeCh (ver
			// Manager.Wake, acionado por db.SetLedgerWriteHook), que não é
			// afetado por este valor. Este poll por timer é só a rede de
			// segurança para o caso (hoje sem ocorrência conhecida) de uma
			// escrita não passar por esse caminho — não precisa de uma
			// rampa gradual para cumprir esse papel.
			currentInterval = maxInterval
		}
		if currentInterval != previousInterval {
			log.Printf("[DEBUG] Projection Manager backoff: processados=%t próximo_intervalo=%s", processed, currentInterval)
		}
```

Remover a função `projectionBackoff` (linhas 264–270), que fica sem nenhum call site após esta mudança.

#### Cuidados

- Não alterar `maxInterval := 20 * time.Minute` nem o comentário que a acompanha (linhas 122–129) — o teto em si já está correto, calibrado pela Tarefa 19.
- Não alterar `m.pollInterval` (definido em `NewManager`, valor `1 * time.Second`) — continua sendo o piso usado enquanto há trabalho real.
- Não alterar o bloco `select` final (timer/wakeCh/ctx.Done) nem o reset de `currentInterval = m.pollInterval` dentro do case `<-m.wakeCh` (linha 163) — esse reset é o caminho de resposta imediata a uma escrita real e não deve mudar.
- Não alterar `processNewEvents`, `processProjection`, `processEventWithRetry`, `commitCheckpoint`, `RebuildProjection`, `RebuildAllProjections`, `executeRebuild`, `waitForDatabaseReadiness` nem qualquer outra função do arquivo.
- Confirmar, antes de remover `projectionBackoff`, que não há nenhum outro call site: `rg -n "projectionBackoff\b" internal/`.

---

### 2. Worker de jobs — `internal/jobs/worker.go`, função `sweepPending`

#### Diagnóstico

```go
func (w *Worker) sweepPending(ctx context.Context) {
	minInterval := 30 * time.Second
	maxInterval := 30 * time.Minute
	currentInterval := minInterval

	for {
		timer := time.NewTimer(currentInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-w.stopCh:
			timer.Stop()
			return
		case <-timer.C:
		}

		active, err := w.store.ListActive(500)
		if err != nil {
			log.Printf("[worker] WARN: erro na varredura de jobs ativos: %v", err)
			currentInterval = nextBackoff(currentInterval, maxInterval)
			continue
		}

		for _, j := range active {
			w.Enqueue(j)
		}

		if len(active) > 0 {
			currentInterval = minInterval
		} else {
			currentInterval = nextBackoff(currentInterval, maxInterval)
		}

		log.Printf("[worker] varredura de jobs ativos — ativos=%d fila=%d próximo_intervalo=%s", len(active), len(w.queue), currentInterval)
	}
}
```

Mesmo padrão do Achado 1: rampa gradual de 30s até 30min, com piso de 30 segundos reiniciado sempre que existir qualquer job ativo — ~7,5 minutos de consultas contínuas depois do último job ativo desaparecer.

#### Correção esperada

```go
func (w *Worker) sweepPending(ctx context.Context) {
	minInterval := 30 * time.Second
	maxInterval := 30 * time.Minute
	currentInterval := minInterval

	for {
		timer := time.NewTimer(currentInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-w.stopCh:
			timer.Stop()
			return
		case <-timer.C:
		}

		active, err := w.store.ListActive(500)
		if err != nil {
			// Erro na varredura em si não é motivo para reagir depressa: o
			// despacho real de jobs criados em operação normal já acontece
			// via Enqueue() direto, não depende deste loop. Pular para o
			// teto evita insistir no banco em caso de instabilidade
			// transitória.
			log.Printf("[worker] WARN: erro na varredura de jobs ativos: %v", err)
			currentInterval = maxInterval
			continue
		}

		for _, j := range active {
			w.Enqueue(j)
		}

		if len(active) > 0 {
			// Ainda há jobs pending/processing — manter o piso rápido para
			// recuperar rapidamente um backlog real (ex.: muitos jobs após
			// reinício do servidor).
			currentInterval = minInterval
		} else {
			// Nenhum job ativo: pular direto para o teto em vez de uma
			// rampa gradual. Jobs criados em operação normal continuam
			// imediatos via Enqueue() direto nos handlers — este loop é
			// só a rede de segurança para reinício/fila cheia.
			currentInterval = maxInterval
		}

		log.Printf("[worker] varredura de jobs ativos — ativos=%d fila=%d próximo_intervalo=%s", len(active), len(w.queue), currentInterval)
	}
}
```

Remover a função `nextBackoff` (fica sem nenhum call site após esta mudança).

#### Cuidados

- Não alterar `minInterval := 30 * time.Second` nem `maxInterval := 30 * time.Minute`.
- Não alterar `loop`, `process`, `Enqueue`, `markInFlight`, `unmarkInFlight`, `processItem`, `publishProgress`, `buildFailureReason`, `Start`.
- Confirmar, antes de remover `nextBackoff`, que não há nenhum outro call site: `rg -n "nextBackoff\b" internal/`.

---

### 3. Worker de jobs — `internal/jobs/worker.go`, função `cleanupLoop`

#### Diagnóstico

```go
func (w *Worker) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.store.Cleanup()
		}
	}
}
```

`store.Cleanup()` (em `internal/jobs/store.go`) só remove jobs concluídos há mais de 24 horas (`cutoff := time.Now().Add(-24 * time.Hour)`). Rodar essa varredura de hora em hora não muda o resultado — um job elegível para limpeza às 14h continua elegível às 15h, 16h etc. O ticker de 1 hora garante no mínimo 24 despertares do compute por dia, mesmo em dias sem nenhuma atividade real.

#### Correção esperada

```go
func (w *Worker) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.store.Cleanup()
		}
	}
}
```

#### Cuidados

- Não alterar `store.Cleanup()` em `internal/jobs/store.go` — a lógica de limpeza em si (cutoff de 24h) já está correta e não muda.
- Esta mudança significa que, num pior caso, um job concluído pode levar até 48h (não mais 25h) para ser efetivamente removido do banco — aceitável dado que a limpeza é só liberação de espaço, não afeta nenhuma funcionalidade visível ao usuário.

## Fora de escopo

Não fazer nesta tarefa:

- Alterar `wakeCh`, `Manager.Wake()`, `db.SetLedgerWriteHook`, `notifyLedgerWritten`, ou qualquer parte do mecanismo de wake orientado a evento — já está correto (Tarefa 19).
- Alterar `Enqueue()`, `markInFlight`/`unmarkInFlight`, ou os três handlers que chamam `w.Enqueue(j)` diretamente (`sistema_handler.go`, `job_handlers.go`, `async_batch_handlers.go`) — o despacho imediato de jobs já está correto.
- Alterar os valores de `pollInterval` (1s) e `minInterval` (30s) — continuam sendo o piso correto enquanto há trabalho real sendo drenado.
- Alterar os valores de `maxInterval` (20 minutos no Manager, 30 minutos no worker) — já calibrados pela Tarefa 19 com margem sobre o limiar fixo de 5 minutos do NeonDB free tier.
- Implementar `LISTEN/NOTIFY` nativo do PostgreSQL, Redis, fila externa, ou qualquer dependência nova.
- Alterar `MaxConnections`, `MaxIdleConns`, `ConnMaxIdleTime` em `internal/db/client.go`.
- Alterar o endpoint `/health` ou a lógica de `dbClient.Health()` — tratado por tarefa separada (Tarefa 20, auditoria de requisições duplicadas em middleware, ainda pendente).
- Implementar qualquer forma de notificação em tempo real ao usuário/cliente sobre progresso de processamento — decisão de produto confirmada nesta tarefa é que atualização "quando pronto" é suficiente nesta fase.
- Alterar regras de negócio, agregados, projeções individuais, handlers HTTP ou contratos de API.
- Adicionar métricas, observabilidade ou logging além do já existente nos trechos alterados.

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual.
2. Rodar `rg -n "projectionBackoff\b" internal/` e `rg -n "nextBackoff\b" internal/` e confirmar que cada função tem exatamente um call site (o que será substituído).
3. Editar `internal/projections/manager.go`: substituir o bloco `if processed {...} else {...}` em `StartProcessing` conforme especificado, e remover `projectionBackoff`.
4. Rodar `gofmt` e `go build ./internal/projections/...`.
5. Editar `internal/jobs/worker.go`: substituir o corpo de `sweepPending` conforme especificado (incluindo o caminho de erro), remover `nextBackoff`, e alterar `cleanupLoop` de `1 * time.Hour` para `24 * time.Hour`.
6. Rodar `gofmt` e `go build ./internal/jobs/...`.
7. Rodar `go build ./...`, `go vet ./...`, `go test ./...`.
8. Revisar o diff completo e confirmar que nenhum arquivo fora de `internal/projections/manager.go` e `internal/jobs/worker.go` foi alterado.

## Critérios de aceite

- [ ] `internal/projections/manager.go`: `StartProcessing` atribui `currentInterval = maxInterval` diretamente (sem chamada a função de backoff gradual) quando `processed == false`.
- [ ] `internal/projections/manager.go`: `currentInterval = m.pollInterval` quando `processed == true` permanece inalterado.
- [ ] `internal/projections/manager.go`: a função `projectionBackoff` não existe mais no arquivo.
- [ ] `internal/projections/manager.go`: o reset `currentInterval = m.pollInterval` dentro do case `<-m.wakeCh` permanece inalterado.
- [ ] `internal/jobs/worker.go`: `sweepPending` atribui `currentInterval = maxInterval` diretamente (sem chamada a função de backoff gradual) tanto no caminho de erro quanto quando `len(active) == 0`.
- [ ] `internal/jobs/worker.go`: `currentInterval = minInterval` quando `len(active) > 0` permanece inalterado.
- [ ] `internal/jobs/worker.go`: a função `nextBackoff` não existe mais no arquivo.
- [ ] `internal/jobs/worker.go`: `cleanupLoop` usa `time.NewTicker(24 * time.Hour)`.
- [ ] Nenhum outro arquivo além de `internal/projections/manager.go` e `internal/jobs/worker.go` foi alterado.
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `go test ./...` passa, ou qualquer falha é documentada com causa comprovadamente não relacionada a esta tarefa.

## Checks manuais recomendados após deploy em staging

### 1. Confirmar que a rampa desapareceu

1. Subir o backend em staging apontando para um banco de teste.
2. Disparar uma única escrita real (ex.: lançar uma nota).
3. Observar os logs: depois da linha `[DEBUG] Projection Manager acordado por escrita no ledger` e do processamento, a **próxima** linha de log de backoff deve mostrar `próximo_intervalo=20m0s` diretamente — não deve aparecer nenhuma sequência de `2s`, `4s`, `8s` etc.
4. Repetir o mesmo teste criando um job assíncrono simples e observar `[worker] varredura de jobs ativos` indo direto para `próximo_intervalo=30m0s` assim que o job concluir.

### 2. Confirmar suspensão real dentro de uma janela curta

1. Depois do teste acima, não enviar mais nenhuma requisição.
2. Observar o painel Monitoring → System Operations do NeonDB.
3. A duração da janela ativa (Start compute → Suspend compute) para esse ciclo deve cair para próximo do tempo da própria sessão de teste + 5 minutos — não mais 15-25 minutos.

### 3. Confirmar que backlog grande ainda drena rápido (sem regressão)

1. Simular um cenário com mais de 100 eventos pendentes de uma vez (ex.: rebuild de uma projeção, ou importação em lote grande).
2. Confirmar nos logs que `processados=true` se repete em sequência rápida (piso de 1s/30s) enquanto ainda há backlog, e só pula para o teto depois que o backlog é totalmente drenado.

## Riscos e mitigação

### Risco: uma escrita que, por algum motivo excepcional, não aciona `wakeCh`/`Enqueue` agora demora até 20-30 minutos para ser pega pela rede de segurança, em vez de alguns minutos

- Isso já era uma possibilidade teórica antes desta tarefa (o teto de 20-30 minutos já existia desde a Tarefa 19); esta tarefa só faz o sistema chegar a esse teto mais cedo em vez de gradualmente. O caminho principal (wakeCh/Enqueue) continua 100% inalterado e é o que cobre praticamente todos os casos reais, conforme validado na Tarefa 19 (`appendDirect` sem chamadores externos ao pacote `db`).
- Mitigação: aceito como parte da decisão de produto confirmada nesta tarefa — poupança máxima de compute nesta fase, com atualização "quando pronto" em vez de tempo real por este canal específico.

### Risco: caminho de erro do `sweepPending` pular direto para 30 minutos pode atrasar recuperação de uma instabilidade transitória real do banco

- Antes desta tarefa, um erro também levava ao mesmo backoff gradual usado pelo caminho "sem jobs ativos" — não era um retry rápido dedicado. O comportamento novo é mais lento nesse caminho específico, mas evita insistir no banco durante uma instabilidade, o que é preferível dado o objetivo desta tarefa.
- Mitigação: se isso se mostrar um problema real em produção, considerar um tratamento de erro dedicado (ex.: um retry único e rápido antes de recorrer ao teto) como tarefa futura separada — não necessário agora sem evidência de que isso ocorre.

## Observações finais

Esta tarefa assume que a Tarefa 19 já está em produção — os valores de `maxInterval` (20 minutos e 30 minutos) e o mecanismo de wake (`wakeCh`, `Enqueue`) são pré-requisitos corretos e não mudam aqui. O que muda é puramente a forma de transição para o estado ocioso: de uma rampa gradual que passa vários minutos abaixo do limiar de suspensão do NeonDB, para um salto direto e imediato ao teto já definido — sem introduzir nenhuma dependência nova, sem alterar a responsividade a eventos reais, e sem alterar nenhuma regra de negócio.
