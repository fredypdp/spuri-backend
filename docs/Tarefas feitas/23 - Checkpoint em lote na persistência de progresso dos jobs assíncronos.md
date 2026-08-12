---
criado: 2026-08-10 00:00
origem: Continuação da investigação de otimização de processos pesados (Tarefas 21/22) — conversa com Claude, a pedido explícito de avaliar todos os processos assíncronos (lançamento de notas em lote, cadastro de estudantes em lote, etc.)
status: a fazer
---

# 23 — Checkpoint em lote na persistência de progresso dos jobs assíncronos (a fazer)

## Prompt recomendado para executar a atualização

Implemente exatamente a mudança descrita na seção "Escopo obrigatório": em `internal/jobs/store.go`, substituir `AppendResult` por dois métodos — `AppendResultInMemory` (atualiza contadores e o array `Results` só em memória, sem tocar o banco) e `FlushResults` (persiste o estado atual em memória para o banco, idêntico ao que `persist` já faz hoje); em `internal/jobs/worker.go`, adaptar o laço em `process()` para chamar `AppendResultInMemory` a cada item e `FlushResults` só a cada `flushBatchSize` itens OU a cada `flushInterval` decorrido (o que vier primeiro), sempre incluindo um flush incondicional no último item do lote. NÃO altere `publishProgress`, `w.notifier.Publish`, o mecanismo de SSE, nem a lógica de resumo (`idx < (j.DoneItems + j.FailItems)`) — o objetivo é reduzir a frequência de escrita no banco sem mudar nenhum outro comportamento observável. NÃO altere nenhum handler de negócio (`RegistrarNota`, `RegisterEstudantePorAcademiaJobItem`, etc.) nem qualquer aggregate — esta tarefa mexe exclusivamente na infraestrutura de acompanhamento de progresso do worker, compartilhada pelos 25 tipos de job hoje registrados. Ao final, rode `gofmt`, `go build ./...`, `go vet ./...` e `go test ./...`, e confirme cada item da seção "Critérios de aceite" antes de reportar a tarefa como concluída.

## Contexto

Esta tarefa nasce da mesma investigação de compute do NeonDB das Tarefas 19-22, mas olhando para uma categoria de operação com **frequência de uso muito maior**: os 25 tipos de job assíncrono do sistema (`internal/jobs/job.go`), incluindo lançamento de notas em lote, cadastro de estudantes em lote, criação de turmas/matérias/cursos em lote, etc. — operações do dia a dia de uma academia, não administrativas/raras como um rebuild de projeção.

### Diagnóstico confirmado no código

Em `internal/jobs/worker.go`, `process()` processa um job item a item, chamando `w.store.AppendResult(j.ID, result)` **depois de cada item**, para todos os 25 tipos de job igualmente (a lógica é genérica, no worker — não em cada handler). Em `internal/jobs/store.go`, `AppendResult`:

```go
func (s *Store) AppendResult(id uuid.UUID, item ItemResult) error {
	...
	j.Results = append(j.Results, item)
	...
	return s.persist(j) // grava json.Marshal(j.Results) INTEIRO de volta no banco
}
```

`persist` faz um `INSERT ... ON CONFLICT DO UPDATE` gravando o campo `results` **completo** (todo o histórico de itens já processados, serializado em JSON) a cada chamada — não só o item novo. Isso significa que um lote de N itens não faz N escritas de tamanho constante: faz escritas de tamanho **crescente** (1, 2, 3, ..., N resultados), totalizando ao longo do job algo da ordem de N²/2 "unidades" de dados transferidas, em vez de N se cada escrita fosse independente do tamanho do histórico. Confirmado: `AppendResult` tem um único call site em todo o repositório (`worker.go:212`), então a mudança é isolada e de baixo risco de regressão em outros fluxos.

### Por que isto é diferente — e mais simples — do que a otimização das Tarefas 21/22

Ao contrário do cache de existência (Tarefa 22), que precisou de uma garantia estrutural específica (ordem de rebuild) para ser seguro, esta otimização **não depende de nenhuma regra de negócio** — é puramente sobre a frequência com que o progresso de um job é gravado no banco, algo inteiramente interno ao worker e ao job store. Nenhum handler, nenhuma projeção e nenhum aggregate precisa mudar.

### O motivo de existir hoje, e o trade-off desta mudança

O comentário no código já documenta a intenção original: "Persistir todo item para retomada resiliente após restart/crash." Hoje, se o processo cair no meio de um lote, `DoneItems + FailItems` no banco reflete exatamente o progresso real até o último item processado — o resumo (`idx < (j.DoneItems + j.FailItems)`) nunca reprocessa um item que já tinha sido concluído.

Fazer checkpoint a cada K itens (em vez de a cada 1) introduz uma janela: se o processo cair entre dois checkpoints, até K-1 itens já processados (mas ainda não persistidos) seriam reprocessados na retomada. **Isto só é seguro se reprocessar um item já bem-sucedido não corrompe nada** — e isso foi verificado, não assumido: em `internal/domain/aggregates/estudante_notas.go`, `RegistrarNota` tem uma proteção explícita e documentada contra isso:

```go
// FIX NOTA-AGG-01: detectar duplicata via estado do aggregate.
// Evita double-submit e a violação de unique constraint 23505 na projeção.
chave := chaveNota(codigoAcademia, anoLectivo, periodo, materiaDisciplinarID, tipo, categoria)
if e.NotasRegistradasPorChave != nil && e.NotasRegistradasPorChave[chave] {
	return fmt.Errorf("nota já registrada para periodo '%s', ...")
}
```

Ou seja: reprocessar um item de `registrar_nota_batch` que já tinha sido concluído com sucesso não cria uma nota duplicada — a segunda tentativa é rejeitada pelo próprio aggregate, e o item aparece como "falha" no resultado do job (mensagem "nota já registrada..."), o que é enganoso na aparência (o dado está correto, só o relatório do lote mostraria uma falha para um item que na verdade tinha sucedido antes do crash) mas não é uma corrupção de dados.

**Isto não foi verificado para os outros 24 tipos de job.** Não há evidência de que TODOS tenham a mesma proteção de duplicata no aggregate correspondente — é plausível que a maioria tenha (checagens de estado antes de aplicar uma mudança são comuns no padrão de Event Sourcing já usado no projeto), mas não é uma garantia comprovada como foi feito para notas. Por isso esta tarefa é desenhada para manter a janela de exposição pequena (poucos itens, poucos segundos) em vez de assumir segurança universal — ver "Critérios de aceite" e "Checks manuais" para a verificação empírica recomendada antes de considerar a tarefa validada em produção.

## Objetivo

Reduzir o número de escritas no banco durante o processamento de um job em lote, de "uma por item" para "uma a cada K itens ou T segundos, o que vier primeiro, mais sempre no último item", sem:

- Mudar o acompanhamento de progresso em tempo real (SSE) — que já é só em memória e não depende desta escrita.
- Mudar a lógica de resumo após crash/restart além de aceitar a janela de exposição já descrita e mantida deliberadamente pequena.
- Tocar em nenhum handler de negócio, aggregate ou regra de validação.

## Escopo obrigatório

### 1. `internal/jobs/store.go` — substituir `AppendResult` por `AppendResultInMemory` + `FlushResults`

#### Correção esperada

Remover `AppendResult` e adicionar em seu lugar:

```go
// AppendResultInMemory adiciona o resultado de um item ao estado em memória
// do job (contadores DoneItems/FailItems e o array Results), sem persistir no
// banco. Usado pelo worker para acumular vários itens entre checkpoints — ver
// FlushResults. Thread-safe.
func (s *Store) AppendResultInMemory(id uuid.UUID, item ItemResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.cache[id]
	if !ok {
		return fmt.Errorf("store.AppendResultInMemory: job %s não encontrado no cache", id)
	}

	j.Results = append(j.Results, item)
	if item.Sucesso {
		j.DoneItems++
	} else {
		j.FailItems++
	}
	return nil
}

// FlushResults persiste no banco o estado atual (em memória) do job — usado
// como checkpoint periódico durante o processamento de um lote, e sempre no
// último item (sucesso ou falha), para garantir que o resultado final de um
// job nunca fique só em memória. Thread-safe.
func (s *Store) FlushResults(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.cache[id]
	if !ok {
		return fmt.Errorf("store.FlushResults: job %s não encontrado no cache", id)
	}
	return s.persist(j)
}
```

#### Cuidados

- Não alterar `persist`, `Get`, `GetByUser`, `ListActive`, `UpdateStatus`, `Cleanup`, `loadFromDB`, `scanJobs`.
- Manter o mesmo padrão de lock já usado em `AppendResult` hoje (`s.mu.Lock()`/`defer s.mu.Unlock()` em cada método público; `persist` continua assumindo que o lock já está adquirido, sem lock próprio).
- Confirmar, depois da remoção, que não sobra nenhum call site de `AppendResult`: `rg -n "\.AppendResult\(" internal/`.

---

### 2. `internal/jobs/worker.go` — checkpoint em lote no laço de `process()`

#### Correção esperada

Adicionar duas constantes perto do topo do arquivo (junto das já existentes `minInterval`/`maxInterval` da Tarefa 21, ou em um bloco `const` novo):

```go
const (
	// resultsFlushBatchSize: quantos itens processar antes de persistir o
	// progresso no banco. Mantido pequeno de propósito — ver "Riscos e
	// mitigação" no documento da Tarefa 23 sobre a janela de reprocessamento
	// em caso de crash entre checkpoints.
	resultsFlushBatchSize = 10
	// resultsFlushInterval: teto de tempo entre persistências, mesmo que
	// resultsFlushBatchSize ainda não tenha sido atingido — evita que um lote
	// pequeno ou um item individualmente lento demore demais para ter
	// qualquer progresso durável.
	resultsFlushInterval = 5 * time.Second
)
```

Substituir o laço atual em `process()`:

```go
	for idx, rawItem := range rawItems {
		if idx < (j.DoneItems + j.FailItems) {
			continue // retoma do ponto salvo
		}
		result := w.processItem(h, j, idx, rawItem)
		if err := w.store.AppendResult(j.ID, result); err != nil {
			log.Printf("[worker] WARN: AppendResult idx=%d job=%s: %v", idx, j.ID, err)
			if db.IsTransientConnectionError(err) {
				log.Printf("[worker] WARN: job %s pausado por indisponibilidade transitória do banco; progresso persistido será retomado", j.ID)
				return
			}
		}
		if latest, err := w.store.Get(j.ID); err == nil && latest != nil {
			j = latest
		}
		w.publishProgress(j, EventJobProgress)
	}
```

por:

```go
	lastFlush := time.Now()
	itemsSinceFlush := 0

	for idx, rawItem := range rawItems {
		if idx < (j.DoneItems + j.FailItems) {
			continue // retoma do ponto salvo
		}
		result := w.processItem(h, j, idx, rawItem)
		if err := w.store.AppendResultInMemory(j.ID, result); err != nil {
			log.Printf("[worker] WARN: AppendResultInMemory idx=%d job=%s: %v", idx, j.ID, err)
		}
		itemsSinceFlush++

		isLastItem := idx == len(rawItems)-1
		shouldFlush := itemsSinceFlush >= resultsFlushBatchSize ||
			time.Since(lastFlush) >= resultsFlushInterval ||
			isLastItem

		if shouldFlush {
			if err := w.store.FlushResults(j.ID); err != nil {
				log.Printf("[worker] WARN: FlushResults idx=%d job=%s: %v", idx, j.ID, err)
				if db.IsTransientConnectionError(err) {
					log.Printf("[worker] WARN: job %s pausado por indisponibilidade transitória do banco; progresso persistido será retomado", j.ID)
					return
				}
			}
			lastFlush = time.Now()
			itemsSinceFlush = 0
		}

		if latest, err := w.store.Get(j.ID); err == nil && latest != nil {
			j = latest
		}
		w.publishProgress(j, EventJobProgress)
	}
```

#### Cuidados

- `isLastItem` deve garantir que o **último item do lote sempre dispara um flush**, mesmo que `itemsSinceFlush < resultsFlushBatchSize` e o tempo desde o último flush seja menor que `resultsFlushInterval` — o resultado final de um job nunca deve ficar só em memória.
- A checagem de `db.IsTransientConnectionError` e o `return` antecipado (pausa o job para retomada posterior via `sweepPending`, já corrigido na Tarefa 21) devem continuar existindo, só que agora associados ao `FlushResults`, não ao `AppendResultInMemory` (que não toca o banco e portanto não pode falhar por indisponibilidade transitória).
- Não alterar `w.publishProgress`, `w.notifier.Publish`, nem a ordem em que `publishProgress` é chamado em relação ao resto do laço — deve continuar rodando a cada item, independente do checkpoint.
- Não alterar o restante de `process()` (verificação de `IsDone`, `UpdateStatus` inicial/final, `buildFailureReason`, `markInFlight`/`unmarkInFlight`).
- Confirmar que o `import "time"` já existe em `worker.go` (deve existir, dado o uso em `sweepPending`/`cleanupLoop` da Tarefa 21).

## Fora de escopo

Não fazer nesta tarefa:

- Alterar qualquer handler de job (`RegistrarNota`, `RegisterEstudantePorAcademiaJobItem`, `CriarTurma`, etc.) ou qualquer aggregate.
- Investigar ou corrigir a busca redundante de entidades "constantes durante o lote" (ex.: a mesma academia/matéria sendo buscada em cada item de um lote de notas) — identificado na mesma conversa que originou esta tarefa, mas tratado como uma categoria de risco diferente (mexe em lógica de negócio, precisa de análise caso a caso por handler) e decidido explicitamente como fora desta tarefa.
- Verificar/corrigir a proteção de duplicata (idempotência) nos outros 24 tipos de job além de `registrar_nota_batch` — ver "Checks manuais" para uma verificação amostral recomendada, não uma auditoria completa de todos os aggregates nesta tarefa.
- Tornar `resultsFlushBatchSize`/`resultsFlushInterval` configuráveis por variável de ambiente — valores fixos são suficientes para esta fase; pode virar ajuste futuro se necessário.
- Alterar `processItem`, `setupCtx`, `RegisterHandler`, `markInFlight`/`unmarkInFlight`, `sweepPending`, `cleanupLoop` (estas duas últimas já corrigidas na Tarefa 21).
- Alterar o schema de `async_jobs` ou qualquer migration.

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual.
2. Rodar `rg -n "\.AppendResult\(" internal/` e confirmar o único call site (`worker.go`), que será substituído.
3. Editar `internal/jobs/store.go`: remover `AppendResult`, adicionar `AppendResultInMemory` e `FlushResults`.
4. Rodar `gofmt` e `go build ./internal/jobs/...`.
5. Editar `internal/jobs/worker.go`: adicionar as constantes, substituir o laço de `process()` conforme especificado.
6. Rodar `gofmt` e `go build ./...`.
7. Rodar `go vet ./...` e `go test ./...`.
8. Revisar o diff completo e confirmar que nenhum arquivo fora de `internal/jobs/store.go` e `internal/jobs/worker.go` foi alterado.

## Critérios de aceite

- [ ] `internal/jobs/store.go`: `AppendResult` não existe mais; `AppendResultInMemory` atualiza `Results`/`DoneItems`/`FailItems` sem chamar `persist`; `FlushResults` chama `persist` com o estado atual do cache.
- [ ] `internal/jobs/worker.go`: o laço em `process()` chama `AppendResultInMemory` a cada item e `FlushResults` apenas quando `itemsSinceFlush >= resultsFlushBatchSize`, `time.Since(lastFlush) >= resultsFlushInterval`, ou `isLastItem`.
- [ ] O último item de qualquer lote sempre resulta em um `FlushResults` bem-sucedido (ou no `return` antecipado por erro transitório, preservando o comportamento de pausa/retomada).
- [ ] `publishProgress` continua sendo chamado a cada item, sem mudança de frequência ou payload.
- [ ] A detecção de erro transitório (`db.IsTransientConnectionError`) e a pausa antecipada do job continuam funcionando, agora associadas a `FlushResults`.
- [ ] Nenhum handler em `internal/handlers/` foi alterado.
- [ ] Nenhum arquivo em `internal/domain/aggregates/` foi alterado.
- [ ] Nenhum outro arquivo além de `internal/jobs/store.go` e `internal/jobs/worker.go` foi alterado.
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `go test ./...` passa, ou qualquer falha é documentada com causa comprovadamente não relacionada a esta tarefa.

## Checks manuais recomendados após deploy em staging

### 1. Confirmar redução de escritas e tempo total de um lote

1. Rodar um `registrar_nota_batch` ou `register_estudante_batch` com pelo menos 30-50 itens em staging.
2. Confirmar nos logs que `FlushResults` aparece a cada ~10 itens (ou a cada ~5s), não a cada item.
3. Comparar o tempo total do job com uma execução anterior a esta tarefa, se houver um baseline disponível.

### 2. Confirmar que o acompanhamento em tempo real continua idêntico

1. Abrir a conexão SSE (`/jobs/stream`) enquanto um lote roda.
2. Confirmar que o progresso reportado (`DoneItems`, `FailItems`, `Progress()`) avança item a item, sem "saltos" perceptíveis nem atraso — já que `publishProgress` continua rodando a cada item, independente do checkpoint.

### 3. Verificação amostral de segurança de reprocessamento (recomendado antes de considerar a tarefa validada em produção)

Escolher pelo menos 2-3 tipos de job além de `registrar_nota_batch` (ex.: `register_estudante_batch`, `adicionar_estudante_batch`, `criar_turma_batch`) e, em staging:

1. Rodar um lote pequeno (3-5 itens).
2. Interromper o processo do backend manualmente **entre dois checkpoints** (ex.: matar o processo logo depois do item 2 de um lote de 5, antes do `FlushResults` do item 5/último).
3. Reiniciar o backend e deixar o job retomar via `sweepPending`.
4. Confirmar que os itens reprocessados **não duplicam dados** — o resultado esperado é que o item reprocessado falhe de forma clara (ex.: "já existe"/"já registrado") ou seja idempotente por natureza, nunca que crie uma segunda entidade/evento para a mesma ação.
5. Documentar o resultado desta verificação (mesmo que informalmente, num comentário do PR ou numa nota em `docs/Debbugs/`) — isso substitui a auditoria completa dos 25 aggregates, que está fora do escopo desta tarefa.

## Riscos e mitigação

### Risco: crash do processo entre dois checkpoints reprocessa até `resultsFlushBatchSize - 1` itens já concluídos

- Mitigado parcialmente por design: janela limitada a no máximo 10 itens ou 5 segundos de trabalho, o que for menor — não é um valor arbitrariamente grande.
- Confirmado seguro para `registrar_nota_batch` especificamente: `FIX NOTA-AGG-01` no aggregate `Estudante` rejeita duplicatas via estado (`NotasRegistradasPorChave`), então reprocessar um item já bem-sucedido falha de forma limpa (nota não é duplicada), embora apareça como "falha" no relatório do lote para um item que na verdade já tinha sucedido.
- Não verificado universalmente para os outros 24 tipos de job — ver "Checks manuais" item 3 para verificação amostral recomendada antes de considerar esta mudança totalmente validada em produção. Se a verificação amostral encontrar um tipo de job onde o reprocessamento causa duplicação real (não apenas uma falha cosmética), a mitigação é reduzir `resultsFlushBatchSize` para esse padrão especificamente ser tratado à parte, ou investigar adicionar uma proteção de idempotência no aggregate correspondente — fora do escopo desta tarefa, mas registrar como achado se ocorrer.

### Risco: crash exatamente no último item, antes do flush final incondicional completar

- Mesmo cenário do risco acima, sem diferença adicional — o item que estava em voo é tratado da mesma forma que qualquer item dentro da janela de checkpoint.

### Risco: `resultsFlushInterval` baseado em `time.Since` pode não disparar exatamente no tempo esperado se um item individual demorar muito mais que 5 segundos

- Não é um problema: o flush acontece **depois** que o item retorna (não há necessidade de um timer concorrente) — se um item demorar 20 segundos, o flush anterior já tinha decorrido há mais de `resultsFlushInterval`, então o `shouldFlush` dispara assim que esse item termina, exatamente como esperado.

## Observações finais

Esta tarefa é a peça "segura e universal" identificada na investigação sobre processos assíncronos — aplica-se aos 25 tipos de job sem exceção, sem tocar em nenhuma lógica de negócio. A busca redundante de entidades "constantes durante o lote" (ex.: mesma academia/matéria repetida em cada item) é uma otimização real, mas de categoria diferente — depende de decidir, handler a handler, quais campos são seguros para tratar como "congelados" durante a execução de um lote sem enfraquecer nenhuma validação de negócio (ex.: uma matéria podendo ser desativada no meio de um lote de notas). Isso fica para uma investigação e decisão à parte, deliberadamente fora do escopo desta tarefa.
