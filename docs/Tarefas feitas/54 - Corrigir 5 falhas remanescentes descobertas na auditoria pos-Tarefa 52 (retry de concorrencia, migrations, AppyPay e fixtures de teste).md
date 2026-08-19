---
status: feito
---

# Tarefa 54 — Corrigir 5 falhas remanescentes descobertas na auditoria pós-Tarefa 52 (retry de concorrência do event store, migrations, AppyPay e fixtures de teste)

## Prompt recomendado para Codex

```
Execute EXATAMENTE as instruções da Tarefa 53 (docs/Lista de Tarefas/53 - ...md), na ordem em
que aparecem: Seção 1 (AppyPay), Seção 2 (migrations), Seção 3 (nome com dígito), Seção 4
(determinismo de rebuild), Seção 5 (retry de concorrência do SaveWithAudit — a mais importante,
é a única com impacto em produção). Todos os 5 diffs já estão prontos para copiar, já foram
aplicados e validados por completo pela Claude: build, vet, e a suíte de testes inteira rodada
3 vezes seguidas do zero (banco limpo a cada vez) contra PostgreSQL 16 real, 100% verde nas 3,
mais o teste de concorrência da Seção 5 rodado isoladamente 20 vezes seguidas (também 100%
verde) porque é um teste de corrida entre goroutines e uma passada só não prova nada. NÃO
invente nada além do que está escrito e NÃO toque em go.mod nem go.sum. Depois de aplicar tudo,
rode exatamente os comandos da seção "Comandos que você deve rodar" e cole a saída completa de
cada um. Você NÃO tem PostgreSQL disponível no seu ambiente — isso é esperado, os testes de
integração serão pulados (skip) automaticamente, não é falha sua. NÃO tente instalar Postgres,
Docker ou usar apt. NÃO marque esta tarefa como concluída nem mova o arquivo para "Tarefas
feitas" — isso é feito pelo Fredy (com a Claude), que já validou tudo isto de ponta a ponta
antes de te passar esta tarefa. Ao final, apenas reporte a saída dos comandos e pare.
```

## Contexto e diagnóstico

### Como isto foi descoberto

Depois da Tarefa 52 (CORS/PATCH), pedi à Claude para investigar 3 falhas de teste que ela tinha
notado como "pré-existentes e sem relação com CORS". Ao investigar a fundo (rodando a suíte
contra PostgreSQL real, banco limpo, repetidas vezes), a lista real acabou sendo maior e
diferente do que parecia inicialmente — e, no meio do processo, a Claude percebeu que estava
trabalhando havia várias horas em cima de uma cópia desatualizada do repositório, sem perceber
que o repositório real já tinha avançado (a própria Tarefa 52 já tinha sido aplicada e fechada
nesse meio tempo). Depois de resincronizar 100% com o estado atual do GitHub e comparar
arquivo por arquivo, ficou claro que **a maior parte do que a Claude tinha encontrado já estava
corrigida** — de forma mais completa do que ela mesma tinha feito — em `turma_vinculo_estudante_integration_test.go`
e `notas_faltas_correcao_integration_test.go` (ano letivo, conteúdo do PDF de upload, telefone e
BI do encarregado, gênero `M`/`F` → `masculino`/`feminino`). Ótima notícia: nenhum trabalho
necessário aí.

O que sobrou, depois de rodar a suíte inteira do zero contra o código real atual (banco limpo,
uma única execução limpa), foram exatamente **5 problemas independentes**, descritos abaixo. Os 4
primeiros são bugs de teste, sem nenhum impacto em produção. **O quinto é diferente: é um bug
real, que pode causar erro 500 em produção sob concorrência real** — foi encontrado por acaso ao
depurar o teste de concorrência, não porque alguém suspeitava dele.

### Seção 1 — `APPYPAY_RESOURCE` ausente em 3 testes (não 2 como se pensava)

Três funções de teste chamam `IniciarPagamentoMensalidades`/`IniciarPagamentoMatricula` sem
`t.Setenv("APPYPAY_RESOURCE", ...)`, ao contrário dos demais testes do pacote `internal/finance`.
Isso faz essas 3 falharem com `APPYPAY_RESOURCE não configurada` — só que de forma **dependente da
ordem de execução dos outros testes do pacote** (às vezes um teste anterior "empresta"
acidentalmente a variável de ambiente por muito pouco tempo, então o sintoma pode aparecer ou
não dependendo de quais outros testes rodam antes). Isolando cada teste, a falha é 100%
reprodutível. Nenhum impacto em produção — é só configuração ausente no próprio teste.

### Seção 2 — Migrations não encontradas em 2 testes de `internal/db`

`TestLedgerAppendOnlyTriggersAndIntegrityIfDatabaseAvailable` e
`TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable` chamam `client.RunMigrations()`,
que lê o diretório `migrations` com um caminho relativo. Como o Go sempre roda o binário de teste
com o diretório de trabalho igual ao diretório do pacote (`internal/db`), esse caminho relativo
nunca resolve (só existe `migrations/` na raiz do repositório). Os testes de integração
equivalentes em `cmd/server` já fazem `os.Chdir("../..")` antes de rodar migrations — esses dois
testes em `internal/db` nunca tinham essa chamada. Nenhum impacto em produção.

### Seção 3 — Nome com dígito rejeitado em `TestTurmaVinculo10...`

O teste usa `fmt.Sprintf("Aluno concorrente %d", i)`, gerando nomes como "Aluno concorrente 0" —
que a validação atual de nome (só letras, acentos, espaços e apóstrofos) rejeita com 400. Só
precisa trocar o número por texto. Nenhum impacto em produção.

### Seção 4 — Comparação por ponteiro no teste de determinismo de rebuild

`TestIntegrationRebuildNotasEFaltasMantemRegistrosCorrigidos` compara duas structs
`snapshotRegistrosCorrecao` com `!=`. Essa struct tem dois campos ponteiro (`NotaAnterior
*float64`, `FaltaAnterior *int`), preenchidos via `sql.Scan` a cada chamada — que sempre aloca
um ponteiro novo, mesmo quando o valor lido do banco é idêntico. Resultado: `primeiro != segundo`
dá sempre `true` (ponteiros diferentes), então o teste **nunca poderia passar**, mesmo que o
rebuild fosse perfeitamente determinístico (e é — o valor real gravado no banco é sempre o
mesmo). É um bug de comparação no teste, não uma falha real de determinismo. Nenhum impacto em
produção.

### Seção 5 — `SaveWithAudit` não retenta sob concorrência real alta (BUG REAL, com impacto em produção)

Este é diferente dos outros 4. `TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable`
faz 8 goroutines gravarem eventos concorrentemente no **mesmo aggregate** ao mesmo tempo — e
falhava de forma **reprodutível mesmo com banco 100% limpo** (não é flakiness de teste) com:

```
pq: duplicate key value violates unique constraint "spuri_ledger_aggregate_id_event_version_key"
```

`SaveWithAudit` já tinha lógica de retry, mas só para `SQLSTATE 40001`
(`serialization_failure`) — o erro clássico que o PostgreSQL usa sob isolamento `SERIALIZABLE`
quando duas transações concorrentes leem `MAX(event_version)` e depois tentam inserir a próxima
versão. Só que, sob concorrência **alta** (mais de duas transações disputando a mesma linha ao
mesmo tempo, não só duas), o PostgreSQL às vezes só detecta o conflito no momento da checagem do
índice único, e retorna `SQLSTATE 23505` (`unique_violation`) em vez de `40001` — e
`SaveWithAudit` tratava isso como **falha definitiva, sem retentar**, mesmo sendo exatamente o
mesmo tipo de conflito de concorrência que já era retentado quando vinha como `40001`.

**Isso é real, não é um cenário artificial de teste**: sempre que duas ou mais requisições
tentam gravar eventos no mesmo aggregate quase ao mesmo tempo (ex.: duas ações quase simultâneas
sobre o mesmo estudante — uma correção manual e um job assíncrono processando o mesmo registro, ou
duas abas do painel administrativo agindo sobre o mesmo aluno), a que perder a corrida pode
receber um 500 aleatório e definitivo em vez de uma gravação bem-sucedida após um retry
transparente de poucos milissegundos.

Investigando mais fundo, também ficou claro que o **número de tentativas (3) não era suficiente**
para concorrência real de 8 gravações simultâneas — mesmo depois de reconhecer o `23505` como
retentável, o teste ainda falhava ~60% das vezes (rodei 10 vezes seguidas: só 3 passaram). O
motivo: com N gravações verdadeiramente simultâneas, o pior caso realista pode exigir até N-1
retentativas para o "azarado" da fila conseguir gravar, e o backoff era **fixo, sem variação
aleatória** — o que faz duas goroutines que colidiram no mesmo instante acordarem de novo nos
mesmos intervalos e colidirem outra vez, em cadeia. A correção da Seção 5 resolve os dois
problemas juntos: reconhece `23505` nesta constraint específica como retentável, sobe o limite de
tentativas de 3 para 10, e adiciona uma variação aleatória (jitter) ao backoff. Validado com 20
execuções seguidas do teste de concorrência: **20/20 passaram**.

---

## Seção 1 — `APPYPAY_RESOURCE` ausente (3 localizações)

### Arquivo `internal/finance/financeiro_ledger_integrity_test.go`

```go
// ANTES:
func TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

// DEPOIS:
func TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()
```

### Arquivo `internal/finance/qrcode_regression_integration_test.go` (2 funções)

```go
// ANTES:
func TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

// DEPOIS:
func TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()
```

```go
// ANTES:
func TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

// DEPOIS:
func TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr(t *testing.T) {
	t.Setenv("APPYPAY_RESOURCE", "integration-resource")
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()
```

(São duas funções distintas no mesmo arquivo — cada uma recebe sua própria linha
`t.Setenv`, logo após a assinatura `func`.)

---

## Seção 2 — Migrations não encontradas (2 arquivos)

### Arquivo `internal/db/event_store_integrity_test.go`

```go
// ANTES:
func TestLedgerAppendOnlyTriggersAndIntegrityIfDatabaseAvailable(t *testing.T) {
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}

	client, err := NewClient(DefaultConfig())

// DEPOIS:
func TestLedgerAppendOnlyTriggersAndIntegrityIfDatabaseAvailable(t *testing.T) {
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}
	prevDir, _ := os.Getwd()
	_ = os.Chdir("../..")
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	client, err := NewClient(DefaultConfig())
```

### Arquivo `internal/db/repository_concurrency_test.go`

```go
// ANTES:
func TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable(t *testing.T) {
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}
	client, err := NewClient(DefaultConfig())

// DEPOIS:
func TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable(t *testing.T) {
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}
	prevDir, _ := os.Getwd()
	_ = os.Chdir("../..")
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	client, err := NewClient(DefaultConfig())
```

---

## Seção 3 — Nome com dígito em `TestTurmaVinculo10...`

Arquivo `cmd/server/turma_vinculo_estudante_integration_test.go`:

```go
// ANTES:
	for i := range codes {
		c := camposCadastro(fmt.Sprintf("Aluno concorrente %d", i))

// DEPOIS:
	for i := range codes {
		c := camposCadastro(fmt.Sprintf("Aluno concorrente %s", []string{"um", "dois"}[i]))
```

---

## Seção 4 — Determinismo de rebuild (comparação por ponteiro)

Arquivo `cmd/server/notas_faltas_correcao_integration_test.go`.

Primeiro, troque a comparação:

```go
// ANTES:
	segundo := snapshotRegistrosCorrigidos(t, fx.client, fx.notaID, fx.faltaID)
	if primeiro != segundo {
		t.Fatalf("rebuild não foi determinístico: primeiro=%+v segundo=%+v", primeiro, segundo)
	}

// DEPOIS:
	segundo := snapshotRegistrosCorrigidos(t, fx.client, fx.notaID, fx.faltaID)
	if !primeiro.Equal(segundo) {
		t.Fatalf("rebuild não foi determinístico: primeiro=%s segundo=%s", primeiro.String(), segundo.String())
	}
```

Depois, adicione os métodos `Equal` e `String` logo após a definição da struct
`snapshotRegistrosCorrecao` (que continua exatamente igual, não mude os campos dela):

```go
// ANTES:
type snapshotRegistrosCorrecao struct {
	Nota          float64
	NotaAnterior  *float64
	Falta         int
	FaltaAnterior *int
}

func snapshotRegistrosCorrigidos(t *testing.T, client *db.Client, notaID uuid.UUID, faltaID string) snapshotRegistrosCorrecao {

// DEPOIS:
type snapshotRegistrosCorrecao struct {
	Nota          float64
	NotaAnterior  *float64
	Falta         int
	FaltaAnterior *int
}

// Equal compara os valores apontados por NotaAnterior/FaltaAnterior, não os
// ponteiros em si. snapshotRegistrosCorrigidos aloca ponteiros novos a cada
// chamada (via sql.Scan), então comparar a struct com "!=" sempre dá
// diferente mesmo quando os valores lidos do banco são idênticos.
func (s snapshotRegistrosCorrecao) Equal(other snapshotRegistrosCorrecao) bool {
	if s.Nota != other.Nota || s.Falta != other.Falta {
		return false
	}
	if (s.NotaAnterior == nil) != (other.NotaAnterior == nil) {
		return false
	}
	if s.NotaAnterior != nil && *s.NotaAnterior != *other.NotaAnterior {
		return false
	}
	if (s.FaltaAnterior == nil) != (other.FaltaAnterior == nil) {
		return false
	}
	if s.FaltaAnterior != nil && *s.FaltaAnterior != *other.FaltaAnterior {
		return false
	}
	return true
}

func (s snapshotRegistrosCorrecao) String() string {
	notaAnterior, faltaAnterior := "nil", "nil"
	if s.NotaAnterior != nil {
		notaAnterior = fmt.Sprintf("%v", *s.NotaAnterior)
	}
	if s.FaltaAnterior != nil {
		faltaAnterior = fmt.Sprintf("%v", *s.FaltaAnterior)
	}
	return fmt.Sprintf("{Nota:%v NotaAnterior:%s Falta:%v FaltaAnterior:%s}", s.Nota, notaAnterior, s.Falta, faltaAnterior)
}

func snapshotRegistrosCorrigidos(t *testing.T, client *db.Client, notaID uuid.UUID, faltaID string) snapshotRegistrosCorrecao {
```

(`fmt` já está importado neste arquivo — não precisa adicionar import novo.)

---

## Seção 5 — Retry de concorrência do `SaveWithAudit` (a correção com impacto real em produção)

### Arquivo `internal/utils/errors.go`

Adicione esta função nova ao final do arquivo (depois de `IsSerializationFailure`, que continua
exatamente igual):

```go
// IsAggregateVersionConflict identifies a duplicate-key violation on
// spuri_ledger's UNIQUE(aggregate_id, event_version) constraint.
//
// Sob isolamento SERIALIZABLE, uma corrida entre duas transações que leem
// COALESCE(MAX(event_version)) e depois inserem a próxima versão do mesmo
// aggregate É o exemplo clássico de conflito que o SSI do Postgres deveria
// sinalizar como 40001 (serialization_failure). Na prática, com concorrência
// alta (mais de duas transações disputando a mesma linha), o Postgres às
// vezes detecta o conflito apenas no momento da checagem do índice único,
// retornando 23505 (unique_violation) em vez de 40001 — e SaveWithAudit
// tratava isso como falha definitiva, sem retentar, mesmo sendo exatamente o
// mesmo tipo de conflito de concorrência que IsSerializationFailure já
// retenta. Só o nome exato desta constraint é aceito propositalmente: um
// 23505 em qualquer outra constraint (ex.: código de estudante duplicado) é
// uma violação de regra de negócio real e não deve ser retentado.
func IsAggregateVersionConflict(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "23505" && pqErr.Constraint == "spuri_ledger_aggregate_id_event_version_key"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlstate 23505") && strings.Contains(msg, "spuri_ledger_aggregate_id_event_version_key")
}
```

(`pq`, `errors` e `strings` já estão importados neste arquivo — é o mesmo padrão de
`IsSerializationFailure`, logo acima.)

### Arquivo `internal/db/repository.go`

Primeiro, adicione `"math/rand"` aos imports:

```go
// ANTES:
import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)

// DEPOIS:
import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)
```

Depois, substitua o corpo de `SaveWithAudit` inteiro:

```go
// ANTES:
func (r *AggregateRepository) SaveWithAudit(aggregate aggregates.Aggregate, audit AuditContext) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = r.saveWithAuditOnce(aggregate, audit)
		if err == nil || !utils.IsSerializationFailure(err) {
			return err
		}
		// The serializable transaction was rolled back and no event was cleared,
		// so persistence can safely be retried with a short bounded backoff.
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}
	return err
}

// DEPOIS:
func (r *AggregateRepository) SaveWithAudit(aggregate aggregates.Aggregate, audit AuditContext) error {
	// maxAttempts=10: com N gravações verdadeiramente simultâneas no mesmo
	// aggregate, o pior caso realista pode exigir até N-1 retentativas antes
	// de vencer a corrida. 3 tentativas bastavam para 2-3 gravações
	// concorrentes, mas eram insuficientes sob contenção maior (ver
	// repository_concurrency_test.go, workers=8) — o writer mais azarado
	// esgotava as tentativas e retornava erro definitivo em vez de
	// eventualmente conseguir gravar.
	const maxAttempts = 10
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		err = r.saveWithAuditOnce(aggregate, audit)
		if err == nil || !(utils.IsSerializationFailure(err) || utils.IsAggregateVersionConflict(err)) {
			return err
		}
		// The serializable transaction was rolled back and no event was cleared,
		// so persistence can safely be retried with a short bounded backoff.
		// IsAggregateVersionConflict cobre o caso em que o Postgres reporta a
		// mesma corrida de concorrência como 23505 (unique_violation) em vez de
		// 40001 (serialization_failure) — ver comentário na função.
		//
		// O jitter aleatório evita que writers que colidiram no mesmo instante
		// acordem nos mesmos intervalos e colidam de novo em lockstep — um
		// backoff puramente linear e determinístico não resolve isso sozinho.
		backoff := time.Duration(15*(attempt+1)) * time.Millisecond
		jitter := time.Duration(rand.Intn(15*(attempt+1)+1)) * time.Millisecond
		time.Sleep(backoff + jitter)
	}
	return err
}
```

Nenhuma outra função de `repository.go` muda — só o corpo de `SaveWithAudit` e o import novo.

---

## Comandos que você deve rodar (sem Postgres — Codex)

Nesta ordem, colando a saída completa de cada um:

```bash
go build ./...
go vet ./...
go test ./internal/finance/... ./internal/db/... ./internal/utils/... ./cmd/server/... -run 'TestIntegrationPagamentoMensalidadeConfirmadoPelaAppyPayMarcaComoPago|TestIntegrationPagamentoMensalidadeGPOQRDevolveQRCodeArr|TestIntegrationPagamentoMatriculaGPOQRDevolveQRCodeArr|TestTurmaVinculo10ConflitoOtimistaNoVinculoTemRetryOuFalhaLimpaSemCorromperTurma|TestIntegrationRebuildNotasEFaltasMantemRegistrosCorrigidos' -v
go test ./...
```

O terceiro comando roda especificamente os testes que estas 5 seções tocam (a maioria vai
aparecer como `SKIP` no seu ambiente, por falta de Postgres — isso é esperado). O quarto roda a
suíte inteira. **Resultado esperado:** `go build` e `go vet` sem nenhuma saída; `go test ./...`
sem nenhuma falha nova além de possíveis `skip`s por falta de Postgres.

---

## O que você NÃO deve fazer

- Não altere `go.mod` nem `go.sum`.
- Não tente instalar PostgreSQL, Docker ou usar `apt`.
- Não defina `SPURI_RUN_DB_INTEGRITY_TESTS=1` nem `RUN_POSTGRES_INTEGRITY_TESTS=1`.
- Não toque em nenhum outro arquivo além dos 7 listados nas Seções 1 a 5.
- Não toque no repositório `spuripainel` (front-end) — nenhuma destas 5 correções tem
  qualquer relação com o front-end.
- Não marque esta tarefa como concluída nem mova este arquivo para "Tarefas feitas".

## Commit sugerido

```
fix(db): retentar SaveWithAudit sob conflito de unicidade de versão (23505)

Sob concorrência real (N>2 gravações simultâneas no mesmo aggregate), o
Postgres às vezes reporta o conflito de concorrência como 23505 na
constraint UNIQUE(aggregate_id, event_version) em vez de 40001
(serialization_failure). SaveWithAudit só reconhecia 40001 como
retentável, retornando 500 definitivo nesses casos. Adiciona
IsAggregateVersionConflict (reconhece especificamente essa constraint,
sem mascarar outras violações de unicidade), sobe o limite de
tentativas de 3 para 10 e adiciona jitter ao backoff para evitar
colisões em lockstep entre writers concorrentes.

fix(tests): corrigir 4 falhas de teste sem impacto em produção

- APPYPAY_RESOURCE ausente em 3 testes de pagamento (dependia da ordem
  de execução dos demais testes do pacote).
- Path relativo de migrations não resolvido em 2 testes de
  internal/db (faltava os.Chdir("../..")).
- Nome com dígito rejeitado pela validação atual em
  TestTurmaVinculo10.
- Comparação por ponteiro (em vez de valor) no teste de determinismo
  de rebuild de notas/faltas, que fazia o teste falhar sempre mesmo
  com dados idênticos.
```
