---
criado: 2026-08-12 00:00
origem: Depuração da implementação da Tarefa 23 (docs/Debbugs/) — conversa com Claude, o orquestrador; execução por Codex
status: a fazer
prioridade: alta — regressão de integridade de dados, não apenas de desempenho
---

# 25 — Corrigir sobrescrita indevida do cache de jobs por ListActive (regressão introduzida pela Tarefa 23)

## Prompt recomendado para executar a atualização

Implemente exatamente a mudança descrita na seção "Escopo obrigatório": em `internal/jobs/store.go`, `ListActive` não deve mais sobrescrever uma entrada já existente em `s.cache` — só deve preencher o cache para jobs que ainda não estão lá. NÃO altere `internal/jobs/worker.go`, `sweepPending`, `Enqueue`, `markInFlight`, `AppendResultInMemory`, `FlushResults`, nem nenhuma outra função de `store.go` além de `ListActive`. Ao final, rode `gofmt`, `go build ./...`, `go vet ./...` e `go test ./...`, e confirme cada item de "Critérios de aceite".

## Contexto

Esta tarefa corrige uma regressão de integridade de dados encontrada numa depuração da Tarefa 23 (checkpoint em lote na persistência de progresso de jobs). A Tarefa 23 foi implementada corretamente em relação ao que foi especificado — o problema é uma interação com um comportamento pré-existente de `ListActive` que a análise de risco da Tarefa 23 não cobriu.

### Mecanismo do problema

`internal/jobs/store.go`, `ListActive` (chamada por `sweepPending` a cada ~30 segundos sempre que existe algum job "pending"/"processing"):

```go
s.mu.Lock()
for _, j := range jobList {
	s.cache[j.ID] = j   // sobrescreve incondicionalmente, mesmo se já existir
}
s.mu.Unlock()
```

Isto sobrescreve a entrada em `s.cache` para **qualquer** job "processing" encontrado no banco — inclusive um job sendo processado neste exato momento por uma goroutine `process()` ativa (`internal/jobs/worker.go`).

Antes da Tarefa 23, isso era inofensivo: cada item era persistido imediatamente (`AppendResult` antigo), então a versão carregada do banco por `ListActive` estava sempre praticamente sincronizada com a versão em memória.

Depois da Tarefa 23, a memória fica deliberadamente à frente do banco entre checkpoints (até 10 itens ou 5 segundos — ver Tarefa 23). Se `ListActive` rodar dentro dessa janela, ele carrega do banco uma versão desatualizada do job (refletindo só o último checkpoint) e substitui o ponteiro em `s.cache[j.ID]` por ela.

`AppendResultInMemory` e o `Get` chamado a cada item no laço de `process()` (`internal/jobs/worker.go`, dentro do `for idx, rawItem := range rawItems`) buscam `s.cache[j.ID]` de novo a cada chamada — não guardam uma referência fixa. Assim que `process()` chama `Get` depois da sobrescrita, sua variável local `j` passa a apontar para o objeto desatualizado; o objeto antigo (com os resultados dos itens acumulados desde o último checkpoint) fica sem nenhuma referência e é descartado. Os itens seguintes do lote são gravados em cima dessa base incompleta.

**Resultado:** para qualquer job em lote que leve mais de ~30 segundos para rodar, o `Results` final pode ficar com itens faltando, e `DoneItems`/`FailItems` subcontados — sem nenhum erro, crash ou log de aviso. Isto é diferente do risco já aceito na Tarefa 23 (reprocessamento de itens após um crash, mitigado por proteção de idempotência) — aqui não há crash nem reprocessamento, é perda silenciosa de progresso já correto, em execução normal.

### Por que a correção é segura

O propósito documentado de `ListActive` (comentário já existente: "retorna jobs pendentes/em processamento para recuperação") e de `sweepPending` (comentário já existente: "recupera jobs 'pending' que não entraram na fila (ex: reinício do servidor, fila cheia)") é **descobrir jobs que ainda não estão sendo acompanhados** — um cenário de cache frio. Para um job que já está em `s.cache` (ou seja, já está sendo ativamente rastreado por alguma goroutine), não há nenhum motivo legítimo para `ListActive` substituir essa entrada: dentro de uma única instância do processo, o cache em memória é sempre a cópia mais atual — o banco é só a cópia durável de apoio, nunca a fonte mais recente para um job já em cache. `Enqueue`, chamado logo depois em `sweepPending`, já protege contra reprocessamento duplicado via `markInFlight` — mas essa proteção age depois que o cache já foi corrompido pela sobrescrita. Bloquear a sobrescrita na origem resolve o problema por completo, sem precisar de nenhuma outra mudança.

## Objetivo

Fazer `ListActive` preencher o cache apenas para jobs que ainda não estão presentes nele, preservando seu comportamento de descoberta para jobs realmente desconhecidos (reinício do servidor, fila cheia) sem nunca substituir uma entrada já rastreada ativamente.

## Escopo obrigatório

### `internal/jobs/store.go` — `ListActive`

#### Correção esperada

```go
// ListActive retorna jobs pendentes/em processamento para recuperação.
func (s *Store) ListActive(limit int) ([]*Job, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(`
		SELECT id, type, status, user_id, user_type,
		       payload, results, total_items, done_items, fail_items,
		       error, created_at, started_at, completed_at
		FROM async_jobs
		WHERE status IN ('pending','processing')
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("store.ListActive: %w", err)
	}
	defer rows.Close()

	jobList, err := scanJobs(rows)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	for _, j := range jobList {
		// Não sobrescrever uma entrada já existente: se o job já está no
		// cache, alguma goroutine pode estar processando-o ativamente com
		// progresso em memória mais atual que o último checkpoint no banco
		// (ver Tarefa 23). Sobrescrever aqui descartaria esse progresso.
		// ListActive só deve preencher o cache para jobs realmente
		// desconhecidos (ex.: reinício do servidor, fila cheia) — o cenário
		// para o qual esta função foi originalmente escrita.
		if _, exists := s.cache[j.ID]; !exists {
			s.cache[j.ID] = j
		}
	}
	s.mu.Unlock()
	return jobList, nil
}
```

A única mudança é envolver `s.cache[j.ID] = j` com `if _, exists := s.cache[j.ID]; !exists { ... }`.

#### Cuidados

- Não alterar a query SQL, o valor de `limit`, nem o valor de retorno (`jobList` continua sendo retornado por completo, incluindo os jobs que já estavam em cache — o `sweepPending` continua recebendo a lista completa para decidir se chama `Enqueue`; só o que muda é se essa entrada é ou não escrita de volta no cache).
- Não alterar `sweepPending`, `Enqueue`, `markInFlight` — a proteção contra reprocessamento duplicado que eles já fazem continua exatamente igual, e continua sendo necessária (esta correção resolve a corrupção de dado; `markInFlight` continua sendo o que evita duas goroutines processando o mesmo job ao mesmo tempo).
- Não alterar `Get`, `loadFromDB`, `GetByUser`, `UpdateStatus`, `AppendResultInMemory`, `FlushResults`, `persist`, `Cleanup`.

## Fora de escopo

Não fazer nesta tarefa:

- Alterar `internal/jobs/worker.go` de nenhuma forma.
- Investigar ou corrigir `loadFromDB` (`internal/jobs/store.go`), que escreve em `s.cache` sem adquirir `s.mu` quando chamado a partir de `Get()` (que já liberou seu `RLock` antes de chamar `loadFromDB`) — é uma condição de corrida de escrita em mapa sem sincronização, pré-existente e não relacionada à Tarefa 23. Real, mas de categoria e prioridade diferentes (mais difícil de disparar na prática, já que exige duas goroutines chamando `Get`/`loadFromDB` para o mesmo job ao mesmo tempo com o cache frio). Considerar como tarefa própria futura.
- Qualquer consideração de múltiplas instâncias do backend rodando simultaneamente (fora do escopo atual do projeto, conforme já registrado nas notas do projeto sobre escalonamento horizontal).

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual.
2. Editar `internal/jobs/store.go`: aplicar a mudança em `ListActive` conforme especificado.
3. Rodar `gofmt` e `go build ./internal/jobs/...`.
4. Rodar `go build ./...`, `go vet ./...`, `go test ./...`.
5. Revisar o diff completo e confirmar que só `internal/jobs/store.go` foi alterado, e só dentro de `ListActive`.

## Critérios de aceite

- [ ] `ListActive` só grava em `s.cache[j.ID]` quando a chave ainda não existe no mapa.
- [ ] `ListActive` continua retornando a lista completa de jobs ativos (`jobList`), independente de terem sido escritos no cache ou não.
- [ ] Nenhuma outra função em `internal/jobs/store.go` foi alterada.
- [ ] Nenhum arquivo além de `internal/jobs/store.go` foi alterado.
- [ ] `go build ./...` sem erros.
- [ ] `go vet ./...` sem erros.
- [ ] `go test ./...` passa, ou qualquer falha é documentada com causa comprovadamente não relacionada a esta tarefa.

## Checks manuais recomendados após deploy em staging

1. Rodar um job em lote com volume suficiente para levar mais de 30 segundos (ex.: 60+ itens de `registrar_nota_batch` ou `register_estudante_batch`).
2. Confirmar nos logs que `sweepPending` roda (`[worker] varredura de jobs ativos...`) pelo menos uma vez durante a execução do lote.
3. Ao final, comparar `TotalItems` com `len(Results)` do job (via `GetByUser`/endpoint de consulta de job) — devem ser iguais, e `DoneItems + FailItems` deve bater com `TotalItems`.
4. Repetir o teste 2-3 vezes para aumentar a chance de o timing coincidir com uma janela de checkpoint pendente (o bug é dependente de timing — nem toda execução necessariamente o expõe).

## Observações finais

Esta é uma correção de uma regressão introduzida pela interação entre a Tarefa 23 e um comportamento pré-existente de `ListActive` que não fazia parte do escopo analisado na especificação original da Tarefa 23 — a falha é de análise de risco no momento de especificar a Tarefa 23, não de execução. A Tarefa 23 em si (a lógica de checkpoint em `worker.go` e a divisão `AppendResultInMemory`/`FlushResults` em `store.go`) está implementada corretamente e não precisa de nenhuma mudança.
