# Relatório Técnico — Infraestrutura do Spuri para Modelagem de Custo NeonDB

## 1. Resumo executivo

- **Risco crítico para scale-to-zero:** a aplicação mantém atividade periódica no banco mesmo sem usuários: `db.Client.startHealthCheck()` executa `PingContext` a cada **10s**, e o `Projection Manager` consulta `spuri_ledger` a cada **1s** por projeção registrada. Isso tende a impedir os 5 minutos de inatividade exigidos para scale-to-zero no Neon.
- **Pool por instância:** o backend usa `database/sql` via `sqlx` com driver `lib/pq`, `MaxOpenConns=25`, `MaxIdleConns=5`, `ConnMaxLifetime=5m` e `ConnMaxIdleTime=30s`. Cada processo Go inicializa seu próprio pool; quantidade real de réplicas no Render não é determinável pelo código.
- **Jobs assíncronos não fazem busy-wait agressivo**, mas há varredura de recuperação de jobs ativos a cada **30s** e limpeza a cada **1h**. Workers consomem um canal Go em memória; não há `LISTEN/NOTIFY`.
- **Leituras potencialmente pesadas** existem em `/notas` e `/faltas`: fazem `LEFT JOIN` com estudantes, academias e matérias, aplicam filtros opcionais e executam uma query adicional de `COUNT(*)`. Há paginação com limite efetivo máximo de 200 nesse helper, mas outras funções de projeção expõem consultas sem paginação por academia/estudante.
- **Render como linha de base:** por informação fornecida no briefing de execução, o serviço atual usa plano gratuito do Render. Detalhes de CPU/RAM/storage, métricas históricas, número de réplicas e staging não estão versionados e requerem medição/consulta ao painel Render.

## 2. Conexões e pooling

### Driver/ORM usado

- O código usa `database/sql` com `github.com/lib/pq` como driver Postgres e encapsula o `*sql.DB` em `github.com/jmoiron/sqlx` (`sql.Open("postgres", connStr)` seguido de `sqlx.NewDb(sqlDB, "postgres")`). Evidência: `internal/db/client.go`, função `NewClient`.
- Não há ORM tipo GORM detectado na configuração principal de banco.

### Pooling configurado no lado da aplicação

Configuração atual em `internal/db/client.go`:

- `SetMaxOpenConns(config.MaxConnections)`.
- `SetMaxIdleConns(config.MaxIdleConns)`.
- `SetConnMaxLifetime(config.ConnMaxLifetime)`.
- `SetConnMaxIdleTime(30 * time.Second)` hardcoded.

Valores de `DefaultConfig()`:

- Com `DATABASE_URL`: `MaxConnections=25`, `MaxIdleConns=5`, `ConnMaxLifetime=5 * time.Minute`, `SSLMode=require`.
- Sem `DATABASE_URL`: os mesmos limites (`25/5/5m`) e variáveis individuais (`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`).

### Instâncias/réplicas no Render

- **NÃO DETERMINÁVEL PELO CÓDIGO — requer medição em produção/staging.** O código não versiona configuração Render (`render.yaml` não foi encontrado) nem quantidade de réplicas/processos.
- Cada instância/processo Go que executa `db.NewClient()` abre seu próprio pool; portanto, conexões máximas potenciais = `número de instâncias * 25` conexões abertas, além de até `5` idle por instância.

### Pooler externo / PgBouncer

- O código monta uma conexão direta via `DATABASE_URL` ou parâmetros individuais. Não há configuração explícita para PgBouncer/pooler externo no código.
- Se o `DATABASE_URL` apontar para um pooler externo, isso é transparente para a aplicação e **não determinável pelo código**.

### Abertura/fechamento frequente de conexões

- Não foi encontrado padrão de “nova conexão por request”. A aplicação cria um `dbClient` global em `cmd/server/main.go` e injeta esse cliente nos handlers.
- A rotina que mais impacta conexões ociosas é o healthcheck interno (`PingContext` a cada 10s), que mantém atividade constante no pool.

## 3. Processos contínuos / background

> **Seção crítica para Neon:** os loops abaixo geram tráfego periódico e podem manter o compute do Neon ativo indefinidamente.

### Projection Manager

- Intervalo real: **1 segundo**, hardcoded em `NewManager()` como `pollInterval: 1 * time.Second`.
- A cada tick, `StartProcessing()` chama `processNewEvents()`, que itera por todas as projeções registradas e chama `processProjection()`.
- Para cada projeção, o manager:
  1. lê o checkpoint (`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = $1`), implementado em cada projeção;
  2. consulta o ledger (`SELECT ... FROM spuri_ledger WHERE id > $1 ORDER BY id ASC LIMIT $2`) em `Manager.getNewEvents()`.
- Portanto, **sim: há query no banco mesmo quando não há eventos novos**; não existe gatilho/event notification antes da consulta.
- O manager é iniciado com `go projManager.StartProcessing()` em `cmd/server/main.go` e não há regra de pausa por horário, madrugada, fim de semana ou inatividade.
- Como há várias projeções registradas (`admins`, `academias`, `cursos`, `materias`, `categorias_nota`, `estudantes`, `turmas`, `notas`, `faltas`, `avaliacao_final`, `solicitacoes_matricula`), o custo de polling é multiplicado pelo número de projeções.

### Jobs assíncronos

- O pool é criado com **4 goroutines** em `initJobs()` (`jobs.NewWorker(..., 4)`).
- Os workers não fazem polling contínuo da tabela enquanto aguardam trabalho; eles bloqueiam em um canal Go (`case j := <-w.queue`).
- Há, porém, uma rotina de recuperação `sweepPending()` com ticker de **30s**, que consulta `async_jobs` por jobs `pending`/`processing` via `Store.ListActive(500)`.
- Há `cleanupLoop()` a cada **1h**, executando `DELETE FROM async_jobs WHERE status IN ('done', 'failed') AND completed_at < $1`.
- Não há `LISTEN/NOTIFY`; o mecanismo é canal Go em memória + varredura periódica no Postgres para recuperação.

### SSE (`GET /jobs/stream`)

- `StreamJobs()` usa `Notifier` em memória. O heartbeat é `time.NewTicker(20 * time.Second)` e escreve `: ping\n\n` no response.
- O heartbeat em si **não consulta o banco**.
- Atenção: quando chega um evento real de job, o handler chama `store.IsHiddenFromSSE(userID, ev.JobID)`. Se o cache em memória ainda não tiver a lista de jobs ocultos daquele usuário, essa função consulta `async_job_sse_hidden` no banco. Depois carrega em cache.

### Outros loops/tickers que tocam o banco

- `db.Client.startHealthCheck()` executa `PingContext` a cada **10s** permanentemente e recicla conexões idle em caso de erro. Esse é um risco direto para scale-to-zero.
- `HealthChecker.CheckAll()` também executa `PingContext`, mas é acionado sob demanda por rota/monitoramento; a frequência depende do healthcheck externo do Render/monitor e **não é determinável pelo código**.
- Rebuild/verificação de integridade tem loops de retry e sleeps, mas não roda continuamente; só durante operações de rebuild.

### Healthcheck do Render/monitoramento

- O código contém healthcheck interno com `PingContext`; se uma rota de health pública for chamada pelo Render em intervalo curto, ela também tocará o banco.
- **NÃO DETERMINÁVEL PELO CÓDIGO — requer verificar configuração do healthcheck do Render.**

## 4. Volume de dados (ledger e projeções)

### Tamanho médio real por linha

- **NÃO DETERMINÁVEL PELO CÓDIGO — requer medição em produção/staging.** O repositório não contém dump/amostra de dados nem acesso configurado ao banco de produção para executar `pg_column_size` ou `pg_total_relation_size`.
- Consultas recomendadas para medição:
  - `SELECT avg(pg_column_size(t)) FROM spuri_ledger t;`
  - `SELECT pg_total_relation_size('spuri_ledger') / NULLIF(reltuples,0) FROM pg_class WHERE relname='spuri_ledger';`
  - repetir para `projection_notas`, `projection_faltas`, `projection_estudantes`.

### Estimativa estrutural pelo schema

- `spuri_ledger` possui: `BIGSERIAL`, dois `UUID`, `VARCHAR(50)`, `VARCHAR(100)`, `INTEGER`, `JSONB payload`, `JSONB metadata`, dois timestamps e dois hashes `VARCHAR(64)`. O custo real será dominado por `payload` e `metadata`, além do overhead dos índices.
- `projection_estudantes` é uma linha relativamente larga: UUID, vários `VARCHAR`, flags booleanas, status, curso/ano, timestamps, `last_event_id` e contadores.
- `projection_notas` e `projection_faltas` são menores, mas têm múltiplos índices e constraints únicas; `observacao TEXT` pode variar muito.

### Eventos por operação típica

- O EventStore grava cada evento com um `INSERT INTO spuri_ledger` em `EventStore.Append()`/`AppendTx()`.
- Para notas/faltas, as projeções tratam apenas os eventos de criação `NotasRegistradas` e `FaltasRegistradas`; os recursos não possuem fluxos funcionais de edição ou exclusão.
- Operações que disparam avaliação final automática ou efeitos compostos não puderam ser contabilizadas com segurança apenas nesta varredura resumida; **NÃO DETERMINÁVEL PELO CÓDIGO sem mapear cada aggregate/handler de negócio individualmente e/ou medir em staging**.

### Índices relevantes

- `spuri_ledger`: primary key em `id`, unique em `event_id`, unique em `(aggregate_id,event_version)` e índices explícitos: `(aggregate_id,event_version)`, `event_type`, `occurred_at`, `aggregate_type`, GIN em `payload`.
- `projection_estudantes`: primary key, unique em `codigo_estudante` e índices explícitos em `codigo_estudante`, `email`, `codigo_academia`, `bilhete_identidade`, `bilhete_identidade_responsavel`, `status`, além de índices adicionais em migrations posteriores (`telefone`, data de nascimento, curso/status etc.).
- `projection_notas`: primary key, unique de negócio, índices por estudante, academia, matéria, período, `registered_at`, e migrations posteriores para tipo/categoria, soft-delete e lookup único.
- `projection_faltas`: primary key, unique de negócio, índices por estudante, academia, matéria, data, ano letivo, `registered_at`, e migrations posteriores para soft-delete, auditoria e `sumario_id`.

### Particionamento

- Não foi encontrado particionamento de `spuri_ledger` por mês, academia, ano letivo ou outro critério nas migrations. A tabela é append-only com triggers de imutabilidade.

### Tamanho atual do banco em produção

- **NÃO DETERMINÁVEL PELO CÓDIGO — requer acesso ao banco de produção/staging.**

## 5. Padrões de leitura e concorrência

### Endpoints com SELECTs mais pesados

- `GET /notas` (`ListarNotas`) faz `SELECT` em `projection_notas` com `LEFT JOIN` em `projection_estudantes`, `projection_academias` e `projection_materias`, filtros opcionais e paginação; também executa `COUNT(*)` adicional.
- `GET /faltas` (`ListarFaltas`) segue o mesmo padrão em `projection_faltas`, com joins e `COUNT(*)`.
- `GET /eventos-estudante/:codigo` busca o estudante por código e depois carrega todo o histórico do aggregate via repository; não há paginação no retorno dos eventos.
- Métodos de projeção como `NotasProjection.GetByAcademia`, `FaltasProjection.GetByAcademia`, `NotasProjection.GetByEstudante` e `FaltasProjection.GetByEstudante` retornam slices completos sem `LIMIT` interno.

### Paginação e limites

- `getPaginationParams()` defaulta para `limit=50` e só aceita `limit <= 200`. Em seguida `db.ValidateLimit()` permitiria até 1000, mas nesse fluxo o teto efetivo fica em 200 porque valores maiores não são aceitos pelo parser inicial.
- Algumas rotas/métodos por estudante ou por academia não aplicam paginação no método de projeção.

### Batch assíncrono

- O worker processa o payload item a item, chamando um handler Gin sintético para cada item e persistindo resultado parcial após cada item (`AppendResult()` atualiza a linha do job no Postgres).
- Não há transação única envolvendo todos os itens do batch. Cada item executa o fluxo normal do handler; cada resultado parcial gera persistência em `async_jobs`.
- Isso reduz risco de transações longas, mas aumenta número de writes no Postgres durante jobs grandes.

### Rate limiting

- Rate limiting está desativado: `GlobalRateLimit`, `LoginRateLimit`, `EmailRateLimit` e `RateLimitMiddleware` chamam apenas `c.Next()`.
- Consequência para Neon: picos de clientes simultâneos não são limitados pela aplicação.

### Cache

- Não foi encontrado Redis ou cache externo.
- Existem caches em memória pontuais: `jobs.Store.cache` e cache de ocultação SSE. Leituras de domínio/projeções vão majoritariamente direto ao Postgres.

## 6. Operações pesadas (rebuilds)

### Tempo prático de rebuild

- **NÃO DETERMINÁVEL PELO CÓDIGO — requer medição em staging/produção**, pois depende do volume do ledger e do tamanho de payloads/índices.

### Como rebuild processa eventos

- Rebuilds de projeções como `notas` e `faltas` fazem `TRUNCATE` da projeção e depois `SELECT ... FROM spuri_ledger WHERE event_type IN (...) ORDER BY id ASC`.
- O código usa `rows.Next()` para iterar, ou seja, não monta explicitamente todos os eventos em um slice Go antes de processar. Porém, a query não tem paginação/batching por `id`; o cursor/resultado fica associado a uma consulta longa.
- Antes do rebuild, `executeRebuild()` pode verificar integridade completa do ledger. Essa verificação lista todos os aggregate IDs e valida um por um, com timeout de 60s por aggregate e até 8 retries em falhas temporárias. Isso pode ser tão ou mais pesado que o rebuild em bancos grandes.
- O manager permite apenas um rebuild por vez com `beginRebuild()`/`endRebuild()`.

### Manutenção manual

- Não foram encontradas rotinas configuradas de `VACUUM`, `REINDEX` ou `ANALYZE` no código/migrations. Em PostgreSQL gerenciado, isso dependerá do provedor.

## 7. Linha de base atual (Render)

- Conforme nota fornecida na execução deste briefing, o ambiente atual usa **plano gratuito do Render**.
- Plano/instância atual do Postgres no Render (RAM/CPU/storage): **NÃO DETERMINÁVEL PELO CÓDIGO — requer painel Render**.
- Uso médio de CPU/RAM/conexões: **NÃO DETERMINÁVEL PELO CÓDIGO — requer métricas Render/produção**.
- Número de réplicas/processos do backend Go: **NÃO DETERMINÁVEL PELO CÓDIGO — requer painel Render**.
- Staging/ambiente separado e banco usado: **NÃO DETERMINÁVEL PELO CÓDIGO — requer configuração operacional fora do repositório**.

## 8. Itens não determináveis pelo código

- Quantidade real de réplicas/processos no Render.
- Se `DATABASE_URL` aponta para Postgres direto ou para pooler externo.
- Frequência real do healthcheck externo do Render/monitoramento.
- Plano exato do Postgres Render, CPU/RAM/storage e limites de conexão.
- Métricas históricas de CPU/RAM/conexões/latência.
- Tamanho atual do banco de produção e tamanho médio real das linhas/tabelas/índices.
- Volume real de eventos por operação em produção, especialmente operações compostas que podem disparar avaliação final/efeitos adicionais.
- Tempo real de rebuild da maior projeção.
- Existência e topologia de staging, se não estiver configurada por variáveis/infra fora do repo.

## 9. Recomendações antes de migrar para Neon

1. **Desativar ou tornar configurável o `startHealthCheck()` de 10s** em ambientes Neon serverless; use healthcheck sob demanda sem ping periódico contínuo, ou aumente muito o intervalo.
2. **Substituir polling de projeções a cada 1s** por mecanismo acionado por evento (`LISTEN/NOTIFY`, fila externa, ou backoff exponencial quando não há eventos). Alternativamente, pausar o Projection Manager fora do horário de operação se a consistência eventual permitir.
3. **Reduzir fan-out do Projection Manager:** hoje cada projeção consulta checkpoint e ledger independentemente; considerar um consumidor único do ledger que distribui eventos em memória para projeções.
4. **Reavaliar `sweepPending()` de jobs** para backoff adaptativo quando não houver jobs ativos; 30s ainda impede longos períodos de inatividade se o banco estiver tentando scale-to-zero.
5. **Ativar rate limiting real** nos middlewares antes de expor Neon autoscaling a picos não controlados.
6. **Medir storage real** com `pg_total_relation_size`, `pg_indexes_size` e `pg_column_size` em produção/staging antes de estimar custo de GB-mês + PITR.
7. **Medir rebuild em staging** com volume representativo e decidir janela operacional; rebuild deve ser tratado como evento de custo separado.
8. **Configurar pool/conexões para Neon:** revisar `MaxOpenConns=25` por réplica, `MaxIdleConns=5` e uso de pooler/connection string adequada para serverless.
9. **Documentar infraestrutura Render atual** (plano gratuito informado, número de réplicas, banco, healthcheck e métricas) para comparação justa com Neon.
