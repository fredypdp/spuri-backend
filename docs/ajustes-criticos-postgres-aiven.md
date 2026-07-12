# Ajustes Críticos no Backend — Integração PostgreSQL (Aiven)

> Sem acesso ao código-fonte (`db/`, `aggregates/`, `handlers/`, `models.go` não
> enviados), os pontos abaixo indicam **o quê** ajustar; o **onde** exato no código
> precisa ser localizado e confirmado por quem tem acesso ao repositório.

---

## 1. Pool de conexões — limite físico de 20 conexões

O plano Aiven usado tem **20 conexões simultâneas** como teto absoluto e **sem
PgBouncer/pooling gerenciado**. Se o `*sql.DB` não tiver limites explícitos, o driver
Go assume "ilimitado" e o serviço vai recusar conexão sob carga.

**Ajustar na inicialização do pool** (provavelmente em `db/` ou `main.go`):

```go
db.SetMaxOpenConns(15)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
db.SetConnMaxIdleTime(2 * time.Minute)
```

Deixar ~5 conexões de folga fora do pool da aplicação (para admin/monitoramento
pontual via `psql`).

---

## 2. Workers assíncronos (pool de 4 goroutines) devem usar o mesmo `*sql.DB`

Se cada goroutine do pool de jobs assíncronos abrir sua própria conexão/pool
separado do pool principal usado pelos handlers HTTP, o total de conexões
simultâneas pode ultrapassar 20 facilmente sob lote grande (até 1000-2000 itens).

**Verificar e corrigir:** os workers devem obter conexões do **mesmo** `*sql.DB`
compartilhado pelo resto da aplicação, nunca instanciar um pool próprio.

---

## 3. Retry/backoff para falhas transitórias de conexão

O plano gratuito desliga automaticamente por inatividade. O código atual
provavelmente trata falha de conexão como erro fatal — isso precisa mudar:

- Envolver a abertura inicial de conexão e queries críticas em retry com backoff
  exponencial (não aplicar retry em erros de validação de negócio, só em erros de
  rede/conexão).
- Garantir que uma falha transitória de conexão retorne `503 SERVICE_UNAVAILABLE`
  no envelope padrão de erro, em vez de derrubar o processo.
- No worker de jobs assíncronos: confirmar que a lógica de retomada por
  `done_items + fail_items` (já prevista na documentação) também cobre o caso de
  perda de conexão no meio do processamento, não só crash do processo.

---

## 4. Health check com timeout curto

Adicionar (ou ajustar, se já existir) uma verificação ativa de conexão:

```go
ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
defer cancel()
if err := db.PingContext(ctx); err != nil {
    // tratar como indisponibilidade transitória, não crash
}
```

Necessário especialmente por causa do auto-shutdown por inatividade — o primeiro
ping após período ocioso pode demorar mais que o normal.

---

## 5. Pontos a confirmar no código antes de aplicar o acima

- Onde o `*sql.DB`/`pgxpool.Pool` é inicializado hoje e quais limites já existem
- Se os workers assíncronos já compartilham esse pool ou abrem conexão própria
- Se existe alguma camada de retry/circuit breaker já implementada
- Ferramenta e local das migrations (schema ainda não existe no banco novo)
