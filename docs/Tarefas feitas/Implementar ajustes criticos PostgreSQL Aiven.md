---
criado: 2026-07-12 00:00
origem: docs/ajustes-criticos-postgres-aiven.md
status: feito
---

# Implementar ajustes críticos PostgreSQL Aiven (feito)

## Prompt recomendado para executar a atualização

Implemente os ajustes críticos descritos em `docs/ajustes-criticos-postgres-aiven.md` para tornar o backend seguro para PostgreSQL hospedado na Aiven com teto físico de 20 conexões simultâneas e sem PgBouncer/pooling gerenciado. Localize no código real onde o pool PostgreSQL é inicializado, como os workers assíncronos acessam o banco, como erros transitórios de conexão são tratados e onde existem health checks. Garanta limites explícitos e conservadores de conexão, compartilhamento do mesmo pool entre handlers e workers, tratamento resiliente de falhas transitórias com `503 SERVICE_UNAVAILABLE` no envelope padrão, health check com timeout curto e testes/documentação cobrindo o comportamento.

## Contexto

O ambiente PostgreSQL da Aiven utilizado pelo projeto possui limite absoluto de 20 conexões simultâneas e não conta com PgBouncer ou pooling gerenciado. Em Go, um `*sql.DB` sem limites explícitos pode abrir conexões conforme a demanda, e múltiplos pools independentes podem ultrapassar rapidamente o teto do plano, principalmente quando handlers HTTP e workers assíncronos processam lotes grandes em paralelo.

Além disso, o banco pode ficar temporariamente indisponível por falhas transitórias de rede ou comportamento operacional do provedor. O backend não deve derrubar o processo nem tratar indisponibilidade temporária como erro fatal quando for possível responder de forma controlada, registrar o evento e permitir retry seguro.

Esta tarefa transforma o documento de diagnóstico `docs/ajustes-criticos-postgres-aiven.md` em uma tarefa de implementação rastreável, testável e alinhada ao padrão das demais tarefas do repositório.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Pool PostgreSQL | Limitar explicitamente conexões abertas, ociosas e tempos de vida | Não ultrapassar o teto físico de 20 conexões da Aiven |
| Workers assíncronos | Usar o mesmo pool compartilhado da aplicação | Evitar pools duplicados e estouro de conexões sob lote |
| Falhas transitórias | Tratar indisponibilidade de conexão sem derrubar o processo | Retornar `503 SERVICE_UNAVAILABLE` no envelope padrão quando aplicável |
| Jobs assíncronos | Preservar retomada por progresso já processado | Perda temporária de conexão não deve duplicar ou perder itens indevidamente |
| Health check | Usar `PingContext` com timeout curto e sob demanda | Detectar indisponibilidade sem travar requests ou iniciar crash desnecessário |
| Documentação e testes | Atualizar contratos operacionais e cobrir cenários críticos | Mudança auditável para backend, QA e operação |

---

# 1. Limitar explicitamente o pool PostgreSQL para Aiven

## Objetivo

Garantir que a aplicação nunca tente consumir todas as 20 conexões disponíveis no plano Aiven, deixando folga para administração, monitoramento, migrations pontuais e conexões emergenciais via `psql`.

## Escopo obrigatório

1. Localizar a inicialização real do pool PostgreSQL, seja ela baseada em `*sql.DB`, `sqlx.DB`, `pgxpool.Pool` ou wrapper interno.
2. Confirmar quais limites já existem hoje e se eles são carregados por configuração, variáveis de ambiente ou valores padrão.
3. Ajustar os limites padrão para Aiven conforme o diagnóstico:

```go
db.SetMaxOpenConns(15)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
db.SetConnMaxIdleTime(2 * time.Minute)
```

4. Se o projeto usa `pgxpool.Pool`, aplicar configuração equivalente de máximo de conexões, mínimo/ociosas e tempo de vida, preservando a intenção de limite físico.
5. Evitar valores padrão maiores que 15 conexões abertas por instância quando o alvo for Aiven sem PgBouncer.
6. Documentar claramente como alterar os limites por ambiente, se o backend já suporta configuração externa.

## Regras técnicas

- Não criar um segundo pool para contornar o limite.
- Não usar valor “ilimitado” como fallback silencioso.
- Não assumir que o provedor fará pooling externo.
- Se houver múltiplas réplicas da aplicação, considerar que o consumo máximo total é a soma dos pools de cada réplica.

## Testes esperados

Adicionar ou ajustar testes de configuração cobrindo:

1. valores padrão seguros para Aiven;
2. leitura de variáveis de ambiente quando existirem;
3. rejeição ou normalização de configuração acima do limite permitido, se o projeto possuir validação de config;
4. preservação de compatibilidade com testes locais.

---

# 2. Garantir pool único compartilhado por handlers e workers

## Objetivo

Confirmar e corrigir o acesso dos workers assíncronos ao banco para que todos usem a mesma instância de pool compartilhado pela aplicação.

## Escopo obrigatório

1. Auditar inicialização dos handlers HTTP, services, projections, repositories e workers assíncronos.
2. Identificar qualquer chamada que abra nova conexão PostgreSQL, crie novo `*sql.DB`, novo `sqlx.DB`, novo `pgxpool.Pool` ou novo client de banco fora do bootstrap principal.
3. Remover pools paralelos usados por workers.
4. Injetar a dependência compartilhada já existente no bootstrap da aplicação.
5. Confirmar que o pool de 4 goroutines dos jobs assíncronos não multiplica pools nem conexões máximas.
6. Garantir que testes e modo local continuem criando apenas os recursos necessários para o processo de teste.

## Resultado desejado

Sob carga, handlers HTTP e workers devem disputar o mesmo limite de conexões definido no pool central. O total de conexões simultâneas por instância deve permanecer previsível e abaixo do teto físico da Aiven.

## Validação obrigatória

Executar buscas no código para confirmar pontos de criação de conexão, por exemplo:

```bash
rg -n "sql.Open|sqlx.Open|sqlx.Connect|pgxpool.New|pgx.Connect|NewClient|DATABASE_URL|POSTGRES|DB_" .
```

Toda ocorrência que inicialize banco deve ser classificada como uma das opções abaixo na entrega:

1. bootstrap principal mantido;
2. teste isolado aceitável;
3. migration/CLI fora do runtime HTTP;
4. duplicação removida;
5. falso positivo justificado.

---

# 3. Implementar tratamento seguro para falhas transitórias de conexão

## Objetivo

Evitar que falhas temporárias de rede, indisponibilidade momentânea do banco ou reconexão do provedor derrubem o processo ou retornem erro fora do padrão da API.

## Escopo obrigatório

1. Auditar como a abertura inicial de conexão é feita hoje.
2. Auditar como queries críticas propagam erros de conexão.
3. Identificar se já existe retry, backoff, circuit breaker, middleware de erro ou envelope padrão de erro.
4. Implementar retry com backoff exponencial apenas onde for seguro:
   - abertura inicial de conexão;
   - health checks;
   - operações idempotentes ou explicitamente seguras;
   - pontos de infraestrutura em que não haja risco de duplicar comandos de negócio.
5. Não aplicar retry automático em erros de validação de negócio, autorização, conflito, payload inválido ou comandos não idempotentes sem garantia de segurança.
6. Mapear falhas transitórias de conexão para `503 SERVICE_UNAVAILABLE` no envelope padrão de erro da aplicação.
7. Garantir logs úteis sem vazar credenciais, DSN completo, senha ou dados sensíveis.

## Regras para classificação de erro transitório

A implementação deve tratar como candidatos a indisponibilidade transitória, conforme o driver usado:

1. erro de conexão recusada;
2. timeout de conexão;
3. conexão resetada/fechada inesperadamente;
4. servidor indisponível;
5. falhas temporárias de DNS/rede;
6. erros equivalentes específicos de `pq`, `pgx`, `database/sql` ou `sqlx`.

A lista final deve ser validada no código real e coberta por testes unitários sempre que possível.

## Resultado desejado

Quando o banco estiver temporariamente indisponível durante uma request, a API deve responder com status HTTP `503` e corpo no padrão de erro vigente. O processo não deve encerrar por panic ou `log.Fatal` em falhas recuperáveis durante o runtime.

---

# 4. Preservar retomada segura dos jobs assíncronos

## Objetivo

Garantir que perda de conexão no meio do processamento assíncrono não cause perda silenciosa, duplicação indevida ou bloqueio permanente de lote.

## Escopo obrigatório

1. Auditar o fluxo de jobs assíncronos e a lógica de progresso baseada em itens concluídos e itens falhados.
2. Confirmar se a retomada por `done_items + fail_items`, ou estrutura equivalente do código atual, cobre perda de conexão no meio do processamento.
3. Quando ocorrer erro transitório de banco:
   - registrar falha controlada;
   - preservar progresso já persistido;
   - permitir retomada ou reprocessamento seguro dos itens pendentes;
   - não marcar lote inteiro como sucesso;
   - não perder detalhes necessários para diagnóstico.
4. Garantir que workers respeitem contexto/cancelamento e não mantenham conexões presas após erro.

## Testes esperados

Adicionar ou ajustar testes cobrindo:

1. worker usando pool injetado compartilhado;
2. erro transitório de conexão durante processamento;
3. preservação do progresso já concluído;
4. retomada de itens pendentes sem duplicar itens já concluídos, quando a semântica do job exigir isso;
5. classificação correta de falha transitória versus erro de negócio do item.

---

# 5. Ajustar health check com timeout curto e comportamento não fatal

## Objetivo

Garantir que a verificação ativa de conexão ao banco seja rápida, controlada e não derrube o serviço em indisponibilidade transitória.

## Escopo obrigatório

1. Localizar rotas, handlers, métodos ou goroutines de health check existentes.
2. Substituir ou ajustar ping de banco para usar contexto com timeout curto:

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()
if err := db.PingContext(ctx); err != nil {
    // tratar como indisponibilidade transitória, não crash
}
```

3. Se já existir timeout configurável, garantir que o padrão seja equivalente a 3 segundos ou outro valor explicitamente justificado.
4. Health check chamado por rota/monitor deve sinalizar indisponibilidade do banco sem panic e sem finalizar o processo.
5. Não criar ping permanente que mantenha o banco artificialmente ativo, salvo se houver exigência operacional explícita e documentada.

## Resultado desejado

O health check deve informar indisponibilidade temporária de forma observável e previsível, permitindo que orquestradores ou monitores reajam sem transformar falha transitória em crash desnecessário.

---

# 6. Atualizar documentação operacional

## Objetivo

Registrar no material ativo do projeto as premissas operacionais para PostgreSQL Aiven.

## Escopo obrigatório

Atualizar, conforme aplicável:

1. documentação técnica do backend;
2. documentação de deploy/configuração;
3. exemplos de `.env` ou variáveis de ambiente;
4. documentação de jobs assíncronos, se existir;
5. qualquer guia que mencione PostgreSQL, NeonDB, Aiven, pooling, health check ou migrations.

A documentação deve explicar:

1. limite físico de 20 conexões no plano Aiven atual;
2. razão do padrão `MaxOpenConns=15` e `MaxIdleConns=5`;
3. necessidade de usar pool único compartilhado;
4. comportamento esperado em falhas transitórias;
5. status HTTP esperado para indisponibilidade do banco;
6. cuidados para migrations e acesso administrativo não consumirem toda a reserva.

---

# 7. Validações finais obrigatórias

## Comandos mínimos

Executar, no mínimo:

```bash
go test ./...
rg -n "sql.Open|sqlx.Open|sqlx.Connect|pgxpool.New|pgx.Connect|DATABASE_URL|POSTGRES|DB_" .
rg -n "SetMaxOpenConns|SetMaxIdleConns|SetConnMaxLifetime|SetConnMaxIdleTime|MaxConns|PingContext|SERVICE_UNAVAILABLE|503" .
```

Se algum comando não puder ser executado por limitação de ambiente, justificar claramente na entrega.

## Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. o pool PostgreSQL tiver limites explícitos e seguros para Aiven;
2. handlers e workers assíncronos compartilharem o mesmo pool no runtime;
3. não houver criação acidental de pools paralelos no caminho principal da aplicação;
4. falhas transitórias de banco forem tratadas sem derrubar o processo durante runtime;
5. indisponibilidade transitória em requests retornar `503 SERVICE_UNAVAILABLE` no envelope padrão;
6. health check usar timeout curto e comportamento não fatal;
7. jobs assíncronos preservarem progresso e permitirem retomada segura após falha de conexão;
8. testes automatizados cobrirem configuração, erro transitório e workers quando aplicável;
9. documentação operacional estiver atualizada;
10. a implementação tiver sido validada com buscas de criação de conexão e uso de health check.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Implementar ajustes críticos PostgreSQL Aiven (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/Implementar ajustes criticos PostgreSQL Aiven.md`;
4. manter `docs/ajustes-criticos-postgres-aiven.md` como documento de origem histórica ou removê-lo apenas se houver decisão explícita de produto/arquitetura.
