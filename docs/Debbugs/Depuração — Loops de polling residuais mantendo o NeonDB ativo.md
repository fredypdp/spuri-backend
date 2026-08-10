# Depuração — Loops de polling residuais mantendo o NeonDB ativo além dos 5 minutos de inatividade

> Continuação da Tarefa 20 (Auditoria e eliminação de requisições desnecessárias ao NeonDB). A Fase 1 daquela tarefa endereçou duplicação de queries dentro do ciclo de uma requisição HTTP; esta depuração endereça um problema estrutural diferente e mais impactante: **loops de background com poll ativo (não puramente orientados a evento), que mantêm o compute acordado minutos depois de qualquer atividade real ter cessado.**

## Contexto / evidência observada

Painel Monitoring → System Operations do NeonDB (consultado em 10/ago/2026) mostra ciclos start/suspend com duração ativa consistentemente de **15 a 25 minutos**, muito acima dos 5 minutos de inatividade configurados para suspensão automática (fixo no plano Free, não configurável):

| Start compute | Suspend compute | Duração ativa |
|---|---|---|
| Aug 8, 10:29 pm | Aug 8, 10:52 pm | 23 min |
| Aug 9, 12:43 am | Aug 9, 12:58 am | 15 min |
| Aug 10, 1:40 am | Aug 10, 2:05 am | 25 min |
| Aug 10, 6:08 pm | Aug 10, 6:23 pm | 15 min |

Isso indica que, entre o início e o fim de cada janela ativa, algo continua a consultar o banco em intervalos menores que 5 minutos — mesmo sem tráfego de usuário contínuo.

## Método

Clonagem do repositório (`fredypdp/spuri-backend`, branch `main`, commit mais recente) e varredura por padrões de atividade periódica em todo o código Go e nas migrations:

```bash
grep -rn "time.NewTicker\|time.Tick(\|time.Sleep\|cron\.\|robfig" --include="*.go" . | grep -v "_test.go"
grep -rn "^\s*go [A-Za-z]" --include="*.go" . | grep -v "_test.go"
grep -rln "cron\|schedule\|pg_cron" migrations/
```

Cada goroutine de background identificada foi lida integralmente e seu comportamento de intervalo/reset foi mapeado matematicamente contra o limiar de suspensão de 5 minutos do NeonDB.

## Achados

### 🔴 Achado A — `projections/manager.go`, `StartProcessing`: backoff reinicia para 1s a cada escrita real no ledger

**Local:** `internal/projections/manager.go:119-167`

O loop principal do Projection Manager processa eventos e, quando não há nada novo, dobra seu intervalo de poll (`projectionBackoff`, linha 264) até um teto de 20 minutos — mas **qualquer escrita confirmada no ledger** (via `db.SetLedgerWriteHook(projManager.Wake)`, registrado em `cmd/server/main.go:141`) reinicia o intervalo para `pollInterval = 1 * time.Second` (linha 93).

Sequência de intervalos a partir de um reset: 1s, 2s, 4s, 8s, 16s, 32s, 64s, 128s, 256s — soma cumulativa = 511s. **Todos esses intervalos individuais são menores que 300s (5 min)**, então o NeonDB nunca chega a ter uma janela de inatividade suficiente para suspender durante essa rampa. Só o próximo intervalo (512s) ultrapassa o limiar.

**Consequência prática:** qualquer operação de escrita (nota lançada, falta registrada, matrícula, login) reinicia essa rampa. Uma sessão de uso real com múltiplas escritas espaçadas em poucos minutos (ex.: lançamento de notas de uma turma) mantém o reset acontecendo repetidamente, e o sistema só começa a "desacelerar" de fato depois da última escrita — **adicionando cerca de 8,5 minutos de cauda de atividade após o usuário parar de usar o sistema.**

Isto explica a maior parte da duração observada nas janelas do print: sessão real + ~8,5min de cauda ≈ 15–20min.

### 🔴 Achado B — `jobs/worker.go`, `sweepPending`: mesmo padrão, piso de 30s reiniciado por qualquer job ativo

**Local:** `internal/jobs/worker.go:108-144`

Mesma estrutura do Achado A: intervalo mínimo de 30s, teto de 30 minutos, mas reinicia para 30s **sempre que `w.store.ListActive(500)` retornar pelo menos um job "pending" ou "processing"** (linha 136-137).

Sequência a partir de um reset: 30s, 60s, 120s, 240s — soma cumulativa = 450s, todos abaixo do limiar de 5 min. **~7,5 minutos de consultas contínuas** depois do último job ativo desaparecer.

Isso se soma ao Achado A quando qualquer operação em lote (matrícula em lote, lançamento de notas em lote) passa pela fila de jobs assíncronos — o que é comum, já que o sistema usa esse pipeline propositalmente para esse tipo de operação.

### 🟠 Achado C — `jobs/worker.go`, `cleanupLoop`: ticker incondicional de 1 hora, sem relação com atividade real

**Local:** `internal/jobs/worker.go:154-167`, executando `internal/jobs/store.go:271` (`DELETE FROM async_jobs WHERE status IN ('done','failed') AND completed_at < $1`)

Este é um `time.Ticker` fixo, sem backoff e sem gatilho de evento — dispara a cada hora, para sempre, **mesmo que não haja absolutamente nada para limpar**. Sozinho, isso garante no mínimo 24 despertares do compute por dia, todos os dias (incluindo madrugadas e fins de semana sem uso real), com uma query de escrita (`DELETE`) que reinicia o cronômetro de suspensão a cada disparo.

Diferente dos Achados A e B, este não depende de atividade do usuário — é polling puro por definição, o oposto do modelo evento→gatilho que a Tarefa 20 estabeleceu como objetivo. Impacto isolado é menor que A e B em minutos por ocorrência, mas acontece 24×/dia garantidamente, inclusive em períodos de zero uso.

## Hipóteses descartadas (com evidência)

| Candidato | Motivo da descartada |
|---|---|
| Heartbeat SSE (`job_handlers.go:196`, `StreamJobs`) | O ticker de 20s só escreve `: ping` no `http.ResponseWriter`; não há chamada ao banco dentro do case `<-heartbeat.C`. Só consulta o banco (`store.IsHiddenFromSSE`) quando chega um evento real. |
| Polling do bootstrap (`bootstrap_handler.go:163`) | Uso único — só roda na criação do primeiro admin do sistema, máximo 50 tentativas × 200ms = 10s, e o lock é liberado antes do polling (FIX E4-LN-02 já documentado no próprio código). |
| Retries de e-mail (`email_service.go`) e Mega storage (`storage.go:215`) | Só disparam em caminho de erro (falha de envio/upload), não são periódicos nem rodam em background por padrão. |
| `internal/monitoring/health.go` (`HealthChecker`, `CheckAll`) | Não tem nenhuma chamada em todo o repositório — código morto, não afeta nada em runtime. |
| `processEventWithRetry` (`manager.go:311`) | Só dispara quando o processamento de um evento específico falha; no máximo 3 tentativas com backoff de até ~1,5s total. Não é um loop de fundo. |

## Comandos de validação usados

```bash
# Confirmar as 4 goroutines reais de background do processo
grep -rn "^\s*go [A-Za-z]" --include="*.go" . | grep -v "_test.go"
# → cmd/server/main.go:143  go projManager.StartProcessing()
# → internal/jobs/worker.go:84  go w.loop(ctx)          [consumidor puro de canal, não é poll]
# → internal/jobs/worker.go:89  go w.sweepPending(ctx)   [Achado B]
# → internal/jobs/worker.go:92  go w.cleanupLoop(ctx)    [Achado C]
# → internal/storage/storage.go:215  go func() { ... }() [wrapper de timeout do Mega, não periódico]

# Confirmar que .Wake() só é acionado pelo hook de escrita no ledger
grep -rn "Wake(\|SetLedgerWriteHook" --include="*.go" . | grep -v "_test.go"
```

## Recomendação (direção, não implementação)

Os três achados compartilham a mesma causa-raiz conceitual: **usar exponential backoff como mecanismo primário de "voltar a ficar quieto"**, em vez de ficar quieto imediatamente e confiar inteiramente no gatilho de evento (`wakeCh` / equivalente) para acordar. O backoff foi pensado como rede de segurança (correto), mas o piso de reinício (1s e 30s) é agressivo demais para um ambiente onde cada segundo de atividade tem custo direto no orçamento de 100h/mês.

Direção de correção a validar antes de virar tarefa para o Codex:
- **Achado A e B:** considerar não resetar para o piso mínimo a cada evento, e sim para um valor já acima de 5 minutos quando uma escrita ocorre — perdendo pouca responsividade (a projeção/job ainda processa quase instantaneamente via `wakeCh`/`Enqueue`, que já são might imediatos; o poll de fallback só existe para pegar o que esse caminho rápido perder). Alternativa: manter o piso baixo apenas pela primeira consulta pós-wake, e já saltar para um intervalo seguro (>5min) na sequência, já que o wake em si já tratou o evento que motivou o reset.
- **Achado C:** substituir o ticker incondicional por um gatilho oportunista (ex.: contabilizar jobs concluídos e só limpar quando passar de um limiar, aproveitando uma consulta que já ia acontecer por outro motivo) ou reduzir a frequência para 1×/dia, já que jobs só são elegíveis para limpeza depois de 24h parados.

A tarefa formal para o Codex, com escopo obrigatório e critérios de aceitação, deve ser desenhada depois de confirmar essa direção — os números exatos de piso/teto têm implicação direta na responsividade percebida do sistema (ex.: quanto tempo um usuário espera para ver uma nota lançada refletida na tela) e merecem uma decisão explícita, não só a correção mecânica.
