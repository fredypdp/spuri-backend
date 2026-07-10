# Depurar infraestrutura para migração NeonDB com scale-to-zero (feito)

## Contexto

Este debug nasce da comparação entre o briefing `docs/Spuri_-_Briefing_para_Varredura_NeonDB.md` e o relatório técnico `docs/Relatorio_Tecnico_Infraestrutura_Spuri_NeonDB.md`.

O objetivo não é migrar o banco imediatamente. O objetivo é corrigir primeiro os pontos do backend que mantêm o PostgreSQL acordado sem necessidade, criam picos artificiais de conexão ou desperdiçam queries enquanto o sistema está ocioso. Sem isso, a migração para NeonDB tende a perder a principal vantagem econômica do serverless: pagar compute apenas quando houver consumo real.

## Problema principal

Hoje existem rotinas de background e políticas de conexão que podem manter o NeonDB ativo mesmo sem usuários:

1. `internal/db/client.go` executa `PingContext` a cada 10 segundos via `startHealthCheck()`.
2. `internal/projections/manager.go` consulta o ledger a cada 1 segundo para cada projeção registrada, mesmo quando não há eventos novos.
3. `internal/jobs/worker.go` consulta jobs ativos a cada 30 segundos, mesmo quando não há jobs em execução.
4. `internal/db/client.go` contém wrappers `QueryWithRetry`, `QueryRowWithRetry` e `ExecWithRetry` que reciclam o pool de conexões com `SetMaxIdleConns(0)` em caso de erro.
5. `DefaultConfig()` usa `MaxConnections=25` por instância, valor arriscado para múltiplas réplicas e desnecessário quando a aplicação usa o pooler do Neon.

Esses pontos reduzem ou anulam a chance de o banco ficar 5 minutos sem atividade e entrar em scale-to-zero.

## Objetivo do debug

Entregar uma correção segura, incremental e testável para que o Spuri:

- pare de fazer ping permanente no banco;
- mantenha healthcheck apenas sob demanda;
- use pooling mais conservador para Neon;
- reduza polling ocioso com backoff exponencial/adaptativo;
- preserve a semântica de Event Sourcing/CQRS;
- preserve a consistência eventual das projeções;
- preserve o processamento assíncrono de jobs;
- minimize custo de compute e conexões no NeonDB.

## Escopo obrigatório

### 1. Remover healthcheck permanente em `internal/db/client.go`

#### Diagnóstico

`startHealthCheck()` cria um ticker de 10 segundos e executa `c.db.PingContext(ctx)` continuamente. Em NeonDB serverless, esse padrão mantém atividade constante no banco e impede períodos longos de ociosidade.

#### Correção esperada

Remover completamente:

- campos `healthTicker *time.Ticker` e `stopHealthCheck chan bool` da struct `Client`;
- chamada `client.startHealthCheck()` em `NewClient()`;
- método `startHealthCheck()` inteiro;
- fechamento de `stopHealthCheck` em `Close()`;
- logs que afirmem que o healthcheck automático está ativo.

Manter:

- `Health() error`, pois é healthcheck sob demanda;
- `Close()` fechando apenas o `sqlx.DB`;
- `setUTF8Encoding()`.

#### Resultado desejado

A aplicação só deve tocar o banco para healthcheck quando uma rota/monitor externo chamar explicitamente o healthcheck. Não deve existir goroutine interna fazendo ping periódico.

#### Cuidados

- Não envolver imports em `try/catch` ou padrões equivalentes.
- Após remover os campos/método, revisar imports para remover `time` apenas se ele não for mais usado por outras partes do arquivo. Neste arquivo, `time` ainda será usado por `ConnMaxLifetime`, `Health()` e possivelmente configuração.
- Validar que `Close()` continua idempotente o suficiente para o uso atual.

---

### 2. Remover wrappers manuais de retry em `internal/db/client.go`

#### Diagnóstico

Os métodos `QueryWithRetry`, `QueryRowWithRetry` e `ExecWithRetry` tentam tratar falhas de conexão manualmente. O problema é que eles:

- forçam reciclagem do pool com `SetMaxIdleConns(0)`;
- podem causar picos de reconexão em falhas transitórias;
- duplicam comportamento já tratado pelo pacote `database/sql`;
- no caso de `QueryRowWithRetry`, executam a query uma vez para testar `Scan` e depois executam novamente para devolver um `Row` fresco.

Esse último ponto é perigoso porque uma query não deve ser executada duas vezes apenas para testar conexão. Mesmo em `SELECT`, pode haver funções voláteis, locks, leituras não repetíveis ou semântica inesperada.

#### Correção esperada

Remover completamente:

- `QueryWithRetry`;
- `QueryRowWithRetry`;
- `ExecWithRetry`;
- `isConnectionError`;
- `containsIgnoreCase`.

#### Pré-validação obrigatória

Antes de remover, confirmar com `rg` se esses métodos são usados em outros arquivos:

```bash
rg -n "QueryWithRetry|QueryRowWithRetry|ExecWithRetry|isConnectionError|containsIgnoreCase" .
```

Se aparecer uso fora de `internal/db/client.go`, substituir pelo método nativo equivalente:

- `client.DB().Query(...)`;
- `client.DB().QueryRow(...)`;
- `client.DB().Exec(...)`;
- ou as variantes `QueryContext`, `QueryRowContext`, `ExecContext` quando houver contexto disponível.

#### Resultado desejado

O pool deve ser controlado por `database/sql`, sem reciclagem manual agressiva em cada falha.

---

### 3. Ajustar pooling padrão para uso eficiente com Neon

#### Diagnóstico

`DefaultConfig()` usa `MaxConnections=25` tanto com `DATABASE_URL` quanto com variáveis individuais. Em uma implantação com múltiplas instâncias, o número máximo teórico de conexões cresce como:

```text
conexões máximas = número de instâncias × MaxConnections
```

Com Neon, a recomendação operacional para aplicação web serverless é usar a connection string do pooler quando possível e manter o pool local conservador.

#### Correção esperada

No bloco `if dbURL := os.Getenv("DATABASE_URL"); dbURL != ""`, alterar:

```go
MaxConnections: 25,
```

para:

```go
MaxConnections: 10,
```

Manter `MaxIdleConns=5` e `ConnMaxLifetime=5 * time.Minute` inicialmente, salvo se testes/medição mostrarem necessidade de ajuste adicional.

#### Importante

Não reduzir necessariamente o valor do bloco sem `DATABASE_URL` nesta tarefa. O foco é o ambiente de deploy que receberá a connection string do Neon. Se depois for decidido padronizar ambos os cenários, criar tarefa separada ou justificar explicitamente no PR.

#### Resultado desejado

Cada instância do backend passa a abrir no máximo 10 conexões locais quando configurada por `DATABASE_URL`, reduzindo risco de esgotar conexões no Neon e evitando overprovisioning de pool.

---

### 4. Trocar polling fixo do Projection Manager por backoff em `internal/projections/manager.go`

#### Diagnóstico

`NewManager()` define `pollInterval: 1 * time.Second`, e `StartProcessing()` usa `time.NewTicker(m.pollInterval)` para chamar `processNewEvents()` continuamente. Com várias projeções registradas, o sistema faz consultas frequentes ao checkpoint e ao ledger mesmo sem eventos novos.

#### Correção esperada

Alterar apenas o necessário para backoff adaptativo:

1. Mudar `processNewEvents()` de:

```go
func (m *Manager) processNewEvents() error
```

para:

```go
func (m *Manager) processNewEvents() (bool, error)
```

O `bool` deve indicar se pelo menos uma projeção encontrou/processou eventos.

2. Mudar `processProjection(...)` de:

```go
func (m *Manager) processProjection(name string, projection Projection) error
```

para:

```go
func (m *Manager) processProjection(name string, projection Projection) (bool, error)
```

O retorno deve ser:

- `false, nil` quando `len(events) == 0`;
- `true, nil` quando havia eventos, mesmo que um evento tenha falhado e ficado para retry no próximo ciclo;
- `false, err` em falhas para obter checkpoint ou buscar eventos, se não foi possível saber/processar eventos.

3. Substituir o ticker fixo em `StartProcessing()` por loop com sleep variável:

```text
intervalo inicial: 1s
se processou eventos: volta para 1s
se não processou eventos: dobra até o máximo de 30s
se ocorrer erro: aplicar backoff também, sem encerrar o manager
se ctx for cancelado: sair imediatamente
```

#### Pseudocódigo sugerido

```go
func (m *Manager) StartProcessing() {
    log.Println("[DEBUG] Iniciando processamento de projeções")

    currentInterval := m.pollInterval
    maxInterval := 30 * time.Second

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

        if processed {
            currentInterval = m.pollInterval
        } else if currentInterval < maxInterval {
            currentInterval *= 2
            if currentInterval > maxInterval {
                currentInterval = maxInterval
            }
        }

        timer := time.NewTimer(currentInterval)
        select {
        case <-m.ctx.Done():
            timer.Stop()
            log.Println("[DEBUG] Parando processamento")
            return
        case <-timer.C:
        }
    }
}
```

#### Atenção ao primeiro ciclo

O pseudocódigo acima processa imediatamente ao iniciar e dorme depois. Isso é aceitável e até melhor para reduzir latência de bootstrap. Se preferir manter o primeiro processamento após 1 segundo, documentar a escolha. O critério principal é não manter polling fixo indefinidamente em idle.

#### Cuidados críticos

- Não alterar a lógica de `processEventWithRetry`.
- Não alterar a lógica de `processEventTransactional`.
- Não alterar rebuilds.
- Não alterar integridade do ledger.
- Não avançar checkpoint quando evento falha.
- Não introduzir paralelismo novo entre projeções nesta tarefa.
- Manter logs suficientes para diagnosticar quando o intervalo entra em backoff.

#### Resultado desejado

Quando não houver eventos, o manager reduz consultas de aproximadamente 1 ciclo por segundo para no máximo 1 ciclo a cada 30 segundos. Quando houver eventos, volta rapidamente para 1 segundo.

---

### 5. Trocar varredura fixa de jobs por backoff em `internal/jobs/worker.go`

#### Diagnóstico

`sweepPending()` usa `time.NewTicker(30 * time.Second)` e chama `w.store.ListActive(500)` continuamente. Em períodos sem jobs, essa query só serve para manter atividade no banco.

#### Correção esperada

Substituir o ticker fixo por intervalo adaptativo:

- intervalo inicial/mínimo: 30 segundos;
- se `ListActive(500)` retornar um ou mais jobs: enfileirar e voltar para 30 segundos;
- se retornar zero jobs: dobrar intervalo até o máximo de 5 minutos;
- em erro: aplicar backoff também, até 5 minutos;
- cancelar imediatamente em `ctx.Done()` ou `w.stopCh`.

#### Pseudocódigo sugerido

```go
func (w *Worker) sweepPending(ctx context.Context) {
    minInterval := 30 * time.Second
    maxInterval := 5 * time.Minute
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

A função auxiliar pode ser local ao arquivo, por exemplo:

```go
func nextBackoff(current, max time.Duration) time.Duration {
    next := current * 2
    if next > max {
        return max
    }
    return next
}
```

Se já existir função similar no pacote, reutilizar para evitar duplicação.

#### Cuidados críticos

- Não alterar `process()`.
- Não alterar `processItem()`.
- Não alterar handlers.
- Não alterar `markInFlight`/`unmarkInFlight`.
- Não alterar `cleanupLoop()` nesta tarefa; seu intervalo de 1 hora é aceitável.
- Garantir que jobs recém-criados continuam sendo processados por `Enqueue()` imediatamente; o backoff afeta apenas recuperação periódica de jobs que ficaram no banco.

#### Resultado desejado

Em ociosidade, a varredura de recuperação deixa de consultar o banco a cada 30 segundos e passa gradualmente para até 5 minutos, permitindo janelas reais de inatividade.

## Fora de escopo

Não fazer nesta tarefa:

- migrar dados para NeonDB;
- alterar migrations;
- trocar driver Postgres;
- introduzir Redis, fila externa ou `LISTEN/NOTIFY`;
- reescrever o Projection Manager para consumidor único;
- ativar rate limiting;
- otimizar queries de listagem;
- alterar regras de negócio;
- alterar modelos de domínio;
- alterar contratos de API.

Esses pontos podem virar tarefas futuras, mas este debug deve permanecer focado nos bloqueadores críticos de scale-to-zero e consumo mínimo.

## Plano de execução recomendado

1. Criar branch de correção a partir do estado atual.
2. Rodar `rg` para confirmar que os wrappers `*WithRetry` não são usados fora de `client.go`.
3. Corrigir `client.go` primeiro, pois é a mudança de menor risco e maior impacto no scale-to-zero.
4. Rodar `gofmt` no arquivo alterado.
5. Corrigir `manager.go` com backoff e retornos booleanos.
6. Rodar `gofmt` e `go test ./internal/projections/...` se houver testes aplicáveis.
7. Corrigir `worker.go` com backoff adaptativo.
8. Rodar `gofmt` e `go test ./internal/jobs/...`.
9. Rodar `go test ./...`.
10. Revisar logs para garantir que não ficaram mensagens dizendo que healthcheck automático está ativo.

## Critérios de aceite

A tarefa só deve ser considerada concluída quando todos os itens abaixo forem verdadeiros.

Status da depuração em 2026-07-10: implementação validada no código e testes automatizados executados com sucesso.

- [x] `internal/db/client.go` não contém `startHealthCheck`.
- [x] `internal/db/client.go` não contém `healthTicker`.
- [x] `internal/db/client.go` não contém `stopHealthCheck`.
- [x] `internal/db/client.go` mantém `Health() error` sob demanda.
- [x] `internal/db/client.go` não contém `QueryWithRetry`, `QueryRowWithRetry` ou `ExecWithRetry`.
- [x] `internal/db/client.go` não contém `isConnectionError` nem `containsIgnoreCase`.
- [x] `DefaultConfig()` usa `MaxConnections: 10` no caminho com `DATABASE_URL`.
- [x] `StartProcessing()` não usa ticker fixo de 1 segundo para sempre.
- [x] `processNewEvents()` retorna se houve eventos.
- [x] `processProjection()` retorna se havia eventos para aquela projeção.
- [x] O Projection Manager usa backoff até 30 segundos quando não há eventos.
- [x] O Projection Manager volta para 1 segundo quando encontra eventos.
- [x] `sweepPending()` não usa ticker fixo de 30 segundos para sempre.
- [x] `sweepPending()` usa backoff até 5 minutos quando não há jobs ativos.
- [x] `sweepPending()` volta para 30 segundos quando encontra jobs ativos.
- [x] `cleanupLoop()` permanece inalterado, salvo ajuste de formatação inevitável.
- [x] `go test ./...` passa ou qualquer falha é documentada com causa não relacionada.

## Checks manuais recomendados após deploy em staging

Executar estes checks antes da migração real para NeonDB:

### Verificar ausência de atividade periódica agressiva

1. Subir backend em staging apontando para banco de teste.
2. Não enviar requisições por pelo menos 10 minutos.
3. Observar logs do backend:
   - não deve haver ping de banco a cada 10 segundos;
   - o Projection Manager deve indicar backoff crescente ou reduzir logs de ciclo;
   - worker deve indicar varreduras cada vez mais espaçadas quando sem jobs.
4. Observar métricas do banco:
   - conexões não devem crescer em idle;
   - queries por segundo devem cair significativamente;
   - compute deve conseguir ficar ocioso quando não houver healthcheck externo tocando banco.

### Verificar retomada após evento novo

1. Enviar uma operação simples que gere evento no ledger.
2. Confirmar que a projeção é atualizada.
3. Confirmar que o Projection Manager reduz intervalo para 1 segundo após detectar eventos.
4. Confirmar que não há atraso funcional relevante para o usuário.

### Verificar jobs assíncronos

1. Criar um job assíncrono pequeno.
2. Confirmar que `Enqueue()` processa imediatamente.
3. Simular reinício do servidor com job pendente no banco.
4. Confirmar que `sweepPending()` recupera o job no próximo ciclo.
5. Confirmar que, sem jobs, o intervalo cresce até o teto.

## Riscos e mitigação

### Risco: projeções demorarem mais para processar evento após longo idle

- Com backoff máximo de 30 segundos, o pior caso de detecção por polling pode chegar a 30 segundos se o evento não acionar nenhum outro mecanismo.
- Mitigação: manter teto em 30 segundos, não 5 minutos, para projeções.
- Futuro: substituir polling por `LISTEN/NOTIFY` ou fila externa.

### Risco: jobs pendentes demorarem até 5 minutos para recuperação

- Esse atraso só afeta jobs que não entraram na fila em memória ou ficaram pendentes após reinício/fila cheia.
- Jobs criados durante operação normal devem continuar sendo enfileirados imediatamente por `Enqueue()`.
- Mitigação: manter mínimo de 30 segundos quando há atividade.

### Risco: healthcheck externo ainda manter banco acordado

- Mesmo removendo `startHealthCheck()`, uma rota de health externa pode chamar `Health()` e tocar o banco em intervalo curto.
- Mitigação: revisar configuração do Render/monitoramento para usar healthcheck leve sem banco ou intervalo compatível com scale-to-zero.
- Se necessário, criar endpoint separado: `liveness` sem banco e `readiness` com banco.

### Risco: menos conexões afetarem throughput em pico

- Reduzir `MaxConnections` para 10 no caminho com `DATABASE_URL` pode reduzir paralelismo local por instância.
- Mitigação: usar pooler do Neon e medir latência em staging com carga representativa.
- Se necessário, ajustar por variável de ambiente em tarefa futura, em vez de voltar para 25 fixo.

## Métricas para comparar antes/depois

Coletar em staging ou produção controlada:

- queries por segundo em idle;
- conexões abertas em idle;
- conexões em uso durante pico;
- tempo médio para projeção processar evento recém-criado;
- tempo para processar job assíncrono pequeno;
- compute ativo no Neon durante 30 minutos sem usuários;
- custo estimado diário com e sem loops agressivos.

## Observações finais

Esta correção deve ser feita antes de qualquer migração real para NeonDB. Migrar primeiro e corrigir depois pode gerar custo desnecessário, dificultar diagnóstico e mascarar a economia esperada do serverless.

A prioridade é reduzir consumo em idle sem mudar regras de negócio. Depois desta tarefa, ainda será importante criar tarefas separadas para rate limiting real, medição de storage, paginação de endpoints pesados e estratégia operacional de rebuilds.
