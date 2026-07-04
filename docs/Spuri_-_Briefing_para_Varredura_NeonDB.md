---
finalidade: prompt para varredura de código-fonte do Spuri
objetivo: levantar dados reais do backend para modelar custo de migração/operação no NeonDB
---

# Briefing: Varredura de Código do Spuri para Modelagem de Custos no NeonDB

## Como usar este documento

Este arquivo deve ser enviado como prompt para uma IA com acesso ao código-fonte real do
backend do Spuri (Go + PostgreSQL, Event Sourcing/CQRS). O objetivo da varredura é gerar um
**relatório técnico** que responda, com base no código real (não em suposições), às perguntas
listadas na Seção 2. Esse relatório será usado para calcular previsão de custos no NeonDB
(billing por CU-hora de compute + GB-mês de storage + PITR + branches).

A IA que for fazer a varredura deve:

1. Ler este documento por completo antes de iniciar.
2. Inspecionar o código-fonte (handlers, aggregates, repository/event store, projection
   manager, jobs assíncronos, middlewares, configuração de conexão com o banco).
3. Responder **cada item da Seção 2** com base em evidência concreta do código (citando
   arquivo/função quando possível), não em estimativas genéricas.
4. Quando não for possível responder com certeza a partir do código (ex.: depende de
   variável de ambiente não versionada, ou de comportamento em runtime), marcar
   explicitamente como `NÃO DETERMINÁVEL PELO CÓDIGO — requer medição em produção/staging`.
5. Entregar o resultado como um novo arquivo `.md` estruturado seguindo o índice da Seção 3.

---

## 1. Contexto do sistema (para a IA que for ler o código)

O Spuri é um sistema de gestão acadêmica para Angola, construído em **Go**, usando
**PostgreSQL** como único banco de dados, com arquitetura **Event Sourcing + CQRS**:

- Toda escrita é primeiro um evento imutável gravado em `spuri_ledger` (append-only, trigger
  bloqueia UPDATE/DELETE/TRUNCATE).
- Leituras acontecem via **projeções** (tabelas read-model: `projection_estudantes`,
  `projection_academias`, `projection_notas`, `projection_faltas`, `projection_turmas`,
  `projection_avaliacao_final`, `projection_categorias_nota`, `projection_admins`,
  `projection_cursos`, `projection_materias`, `projection_telefones_extra`,
  `projection_sistema_config`, `projection_anos_letivos_configuracoes`,
  `projection_anos_letivos_academia_finalizacoes`, `projection_solicitacoes_matricula`, etc.).
- Um **Projection Manager** roda em background com polling de ~1 segundo, consumindo novos
  eventos do ledger e atualizando as projeções.
- Existe um sistema de **Jobs Assíncronos** (pool de 4 workers) para operações em lote
  (`/academia/notas-aluno/async`, `/academia/faltas-aluno/async`, etc.), com acompanhamento
  via polling (`GET /jobs/:id`) e via **Server-Sent Events** (`GET /jobs/stream`), que envia
  heartbeat periódico (`: ping`).
- O sistema atende três tipos de usuário (admin, academia, estudante) e tem rotas públicas
  com autenticação opcional (`GET /academias`, `GET /consultar-academia/:codigo`,
  `GET /academia/cursos`).
- Hospedagem atual do backend Go: Render. Decisão em avaliação: migrar o PostgreSQL para
  NeonDB (serverless, billing por CU-hora + storage, com scale-to-zero após inatividade).

O motivo de cada pergunta da Seção 2 está ligado a uma variável específica do billing do
Neon. Por isso a varredura precisa ser literal e baseada em código, não em resumo da
documentação de negócio (`Spuri - Documentação.md` e `Spuri - API.md` já documentam as
regras de negócio — **não repita essas regras no relatório final**, foque apenas no que
afeta infraestrutura/custo).

---

## 2. Perguntas que o relatório final precisa responder

### 2.1 Padrão de conexão com o banco

- Qual driver/ORM é usado para conectar ao Postgres (`pgx`, `database/sql` + `lib/pq`,
  `gorm`, etc.)?
- Existe connection pooling configurado no lado da aplicação (`pgxpool`, `sql.DB` com
  `SetMaxOpenConns`/`SetMaxIdleConns`/`SetConnMaxLifetime`)? Quais são os valores
  configurados hoje (hardcoded ou via env var)?
- Quantas instâncias do backend Go rodam simultaneamente em produção hoje (réplicas/processos
  no Render)? Cada instância abre seu próprio pool de conexões?
- O código já assume um pooler externo tipo PgBouncer, ou conecta direto ao Postgres?
- Existe alguma rotina que abre e fecha conexões com muita frequência (ex.: nova conexão por
  request em vez de reaproveitar o pool)?

> **Por que importa**: Neon cobra por CU-hora de compute ativo. Conexões "zumbi" mantidas
> abertas sem necessidade impedem o scale-to-zero de funcionar, porque o Postgres nunca fica
> realmente ocioso enquanto há conexão TCP ativa segurando uma transação ou idle-in-transaction.

---

### 2.2 Processos que rodam continuamente (polling/background)

- **Projection Manager**: qual é o intervalo de polling real no código (confirmar se é
  literalmente 1 segundo)? Esse polling executa uma query no banco **mesmo quando não há
  eventos novos** (ex.: `SELECT ... WHERE id > checkpoint`), ou só consulta quando algum
  gatilho indica que há trabalho pendente?
- Esse polling roda **mesmo fora do horário de uso** (madrugada, fins de semana), ou existe
  algum mecanismo de pausa quando não há atividade?
- **Jobs assíncronos**: os 4 workers do pool ficam fazendo polling na tabela `async_jobs`
  continuamente (busy-wait) ou usam algum mecanismo de notificação (ex.: `LISTEN/NOTIFY` do
  Postgres, canal Go, fila externa)?
- **SSE (`GET /jobs/stream`)**: o heartbeat `: ping` é enviado pelo servidor Go sem tocar o
  banco, ou cada heartbeat dispara uma query? Qual o intervalo do heartbeat?
- Existe algum cron job, scheduler, ou rotina `time.Tick`/`time.Sleep` no código que roda em
  loop infinito e toca o banco em intervalo fixo, independente de haver requisição de
  usuário?
- Existe algum healthcheck (do Render, de monitoramento, ou interno) que faz `SELECT 1` ou
  similar no banco em intervalo curto (ex.: a cada 10-30s)?

> **Por que importa**: este é o ponto mais crítico para o cálculo do Neon. Se qualquer
> processo acima gera tráfego no banco em intervalo curto e constante, **o banco nunca atinge
> os 5 minutos de inatividade necessários para o scale-to-zero**, e o billing do Neon passa a
> se comportar como "quase always-on", eliminando a vantagem de custo esperada.

---

### 2.3 Volume e padrão de escrita (ledger)

- Qual o tamanho médio real (em bytes) de uma linha da tabela `spuri_ledger`? Levantar isso
  a partir do schema real (tipos de coluna: `payload JSONB`, `metadata JSONB`,
  `ledger_hash`, `previous_hash`, etc.) e, se possível, de uma amostra de dados existentes
  (`pg_column_size` ou `SELECT pg_total_relation_size('spuri_ledger')` dividido pelo número
  de linhas, se houver dados de teste/staging).
- Qual o tamanho médio de uma linha em cada projeção principal (`projection_notas`,
  `projection_faltas`, `projection_estudantes`)?
- Quantos eventos são gravados por operação de negócio típica? (ex.: confirmar no código se
  `POST /academia/notas-aluno` gera 1 evento só, ou também eventos adicionais quando dispara
  avaliação final automática).
- Existem índices nas tabelas de ledger e projeções? Listar quantos e quais colunas, para
  estimar overhead de storage real (índices ocupam espaço adicional, cobrado igual a dados).
- O ledger tem alguma estratégia de particionamento (por mês, por academia, por ano letivo)
  já implementada ou planejada no código/migrations?
- Qual o tamanho atual do banco em produção hoje (se houver acesso a
  `SELECT pg_database_size(...)`), para calibrar a estimativa de crescimento?

> **Por que importa**: storage no Neon é cobrado a $0,35/GB-mês (mais $0,20/GB-mês de
> histórico de PITR). Como o ledger nunca encolhe (é append-only), o tamanho real por evento
> determina diretamente a trajetória de custo de storage no longo prazo, que é cumulativo e
> nunca diminui.

---

### 2.4 Padrão de leitura/concorrência nos picos

- Quais endpoints fazem `SELECT` mais pesados (joins múltiplos, agregações, paginação sem
  índice adequado)? Atenção especial a `GET /notas`, `GET /faltas`, `GET /estudantes` (que
  têm muitos filtros opcionais combináveis) e `GET /eventos-estudante/:codigo` (ledger
  completo de um estudante).
- Existe paginação obrigatória nessas rotas, ou é possível pedir grandes volumes de uma vez
  (ex.: `limit` sem teto real aplicado no código, mesmo que a documentação diga que há
  teto)?
- Os endpoints batch assíncronos (`/async`, limites de 200 a 2000 itens por request,
  conforme a tabela da documentação da API) processam os itens em transações individuais ou
  em lote (uma transação por N itens)? Isso afeta quantas conexões/quanto tempo de compute
  cada job consome.
- Existe rate limiting de fato implementado no código hoje, ou (conforme nota na
  documentação) os middlewares de rate limit estão todos como `c.Next()` sem verificação
  real? Confirmar isso é crítico: sem rate limit real, um pico de matrícula com muitos
  clientes simultâneos pode gerar muito mais CU-hora do que o esperado.
- Existe cache de aplicação (Redis, in-memory, etc.) para reduzir leituras repetidas no
  Postgres, ou toda leitura vai direto ao banco?

> **Por que importa**: o custo de compute do Neon escala com CU-hora consumido, e CU sobe
> conforme CPU/memória/concorrência de query sobe. Sem rate limit nem cache, os picos sazonais
> (matrícula, provas, mensalidade) podem demandar autoscale muito mais agressivo do que uma
> estimativa linear "por escola" sugere.

---

### 2.5 Rebuilds e operações pesadas

- Quanto tempo leva, na prática (ou em estimativa pelo código), um rebuild completo de
  projeção (`POST /dominis/projections/rebuild/:name`) para a maior tabela (provavelmente
  `notas` ou `faltas`)? Isso é relevante porque um rebuild percorre o ledger inteiro e gera
  um pico de compute sustentado.
- Os rebuilds fazem `SELECT *` de todos os eventos de uma vez (carregando tudo em memória) ou
  processam em batches/streaming?
- Existe alguma rotina de manutenção (VACUUM manual, REINDEX, ANALYZE) configurada ou
  recomendada na infraestrutura atual?

> **Por que importa**: rebuilds são operações de alto consumo de compute que podem acontecer
> fora de hora prevista (ex.: correção de bug em produção), e precisam ser contabilizadas
> separadamente dos picos sazonais "normais" no modelo de custo.

---

### 2.6 Ambiente atual (Render) como linha de base

- Qual o plano/instância atual do Postgres no Render (tipo, RAM, CPU, storage alocado)?
- Qual o uso médio de CPU/RAM/conexões reportado pelo Render hoje (se houver métricas
  históricas disponíveis)?
- Quantas réplicas do serviço web Go estão rodando hoje no Render?
- Existe staging/ambiente de teste separado da produção? Em qual banco ele roda hoje?

> **Por que importa**: isso dá um ponto de comparação real (não estimado) para calibrar as
> projeções de CU-hora necessárias no Neon, em vez de partir de estimativas genéricas de
> "horas de pico por escola".

---

### 2.7 Particularidades de billing que dependem de comportamento de código

- O sistema já implementa algum mecanismo de "wake-up" deliberado do banco (ex.: warm-up
  antes de horário de pico esperado)? Isso afetaria diretamente o billing de compute.
- Existe replicação de leitura, banco secundário, ou qualquer estratégia multi-banco hoje?
- O upload de documentos de matrícula (Google Drive) tem algum acoplamento com o Postgres
  além de salvar `path`/`file_url`/`download_url` na projeção? (Confirmar que uploads de
  arquivo não geram carga adicional no Postgres além do registro de metadados.)

---

## 3. Estrutura esperada do relatório final (.md) a ser gerado

O relatório gerado pela varredura deve seguir esta estrutura:

```markdown
# Relatório Técnico — Infraestrutura do Spuri para Modelagem de Custo NeonDB

## 1. Resumo executivo
(3-5 bullets com os achados mais críticos para o cálculo de custo)

## 2. Conexões e pooling
(respostas da seção 2.1, com trechos de código citados)

## 3. Processos contínuos / background
(respostas da seção 2.2 — esta seção deve ter destaque visual, é a mais crítica)

## 4. Volume de dados (ledger e projeções)
(respostas da seção 2.3, com tamanhos médios reais ou estimados a partir do schema)

## 5. Padrões de leitura e concorrência
(respostas da seção 2.4)

## 6. Operações pesadas (rebuilds)
(respostas da seção 2.5)

## 7. Linha de base atual (Render)
(respostas da seção 2.6)

## 8. Itens não determináveis pelo código
(lista explícita de tudo que precisa de medição em runtime/produção, não inferível
estaticamente)

## 9. Recomendações antes de migrar para Neon
(based on findings — ex.: "ajustar polling do Projection Manager para X antes de migrar")
```

---

## 4. Regras para quem for fazer a varredura

- **Não invente números.** Se o código não permite determinar um valor com certeza, marcar
  como não determinável e sugerir como medir (ex.: "rodar `EXPLAIN ANALYZE` em produção",
  "instrumentar com `pg_stat_statements`").
- **Cite arquivo e função/linha** sempre que possível, para que decisões de custo possam ser
  auditadas depois.
- **Não copie as regras de negócio** dos arquivos `Spuri - API.md` e
  `Spuri - Documentação.md` — este relatório é sobre infraestrutura/custo, não sobre regras
  de domínio (matrícula, notas, avaliação final etc.), que já estão documentadas em outro
  lugar.
- **Separe claramente "fato verificado no código" de "inferência razoável"** — use marcação
  explícita tipo `[CONFIRMADO NO CÓDIGO]` vs `[INFERIDO]` em cada resposta.
- Se o código mudar entre o momento desta varredura e o momento da decisão final de migração,
  o relatório deve ser re-executado — ele é um snapshot, não uma verdade permanente.
