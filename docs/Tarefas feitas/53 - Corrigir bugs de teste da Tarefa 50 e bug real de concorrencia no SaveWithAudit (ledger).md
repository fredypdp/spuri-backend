---
status: feito
---

# Tarefa 53 — Corrigir bugs de teste da Tarefa 50 e bug real de concorrência em `SaveWithAudit` (ledger)

## Prompt recomendado para Codex

```
Execute EXATAMENTE as instruções desta tarefa, na ordem em que aparecem: Seção 1 (2 correções em
cmd/server/turma_vinculo_estudante_integration_test.go), Seção 2 (1 correção em
cmd/server/notas_faltas_correcao_integration_test.go), Seção 3 (chdir em 2 arquivos de
internal/db) e Seção 4 (2 correções de produção — internal/utils/errors.go e
internal/db/repository.go). NÃO invente nada além do que está escrito — todo código já está
pronto para copiar, exatamente como vai ficar depois de aplicado (blocos "DEPOIS"). NÃO toque em
go.mod nem go.sum. Depois de aplicar tudo, rode exatamente os comandos da seção "Comandos que você
deve rodar" e cole a saída completa de cada um. Você NÃO tem PostgreSQL disponível no seu ambiente
(isso é esperado e está documentado abaixo) — TODOS os testes tocados por esta tarefa exigem
SPURI_RUN_DB_INTEGRITY_TESTS=1 com PostgreSQL real, então vão aparecer como "skip" no seu ambiente,
e isso é normal, não é uma falha sua; `go build` e `go vet` passando limpos é a validação que você
consegue fazer. NÃO tente instalar Postgres, Docker ou usar apt — nada disso vai funcionar no seu
ambiente. NÃO marque esta tarefa como concluída nem mova o arquivo para "Tarefas feitas" — quem faz
essa confirmação final é o Fredy (com a Claude), que já validou esta correção inteira, incluindo os
testes de integração com PostgreSQL real e um teste de concorrência real com 8 escritores
simultâneos, antes de te passar esta tarefa. Ao final, apenas reporte a saída dos comandos e pare.
```

## Contexto e diagnóstico

### De onde isso veio

Depois que o Codex implementou a Tarefa 50 (`docs/Lista de Tarefas/50 - ...md` — correção do bug
de produção nota-falta/turma-vínculo), a Claude auditou o resultado num sandbox com **PostgreSQL 16
e Go 1.24 reais** (não a suíte "pulada" que o Codex reportou, que só rodou com
`SPURI_RUN_DB_INTEGRITY_TESTS` ausente). Rodando de verdade, apareceram 3 bugs nos testes escritos
pela própria Tarefa 50 — nenhum deles no código de produção da Tarefa 50, mas o suficiente para
bloquear a suíte obrigatória dela (Seções 1, 2 e 3 abaixo).

Investigando a causa raiz de uma falha adicional já conhecida e documentada como "fora de escopo"
na Tarefa 52 (a falha de `internal/db` por `chdir` ausente), a Claude corrigiu esse bug também — e,
ao corrigi-lo, descobriu que ele estava mascarando um **bug real de produção**: o mecanismo de
retry de `SaveWithAudit` (usado em toda escrita no ledger de eventos) nunca conseguia se recuperar
de uma corrida de escrita concorrente real contra o mesmo aggregate, porque classificava o erro
errado como não-retryable. Isso é a Seção 4 abaixo — a mais importante desta tarefa.

Tudo que segue já foi aplicado, testado e revalidado do zero pela Claude, com banco recriado a cada
rodada, incluindo `-count=3` e execuções repetidas para garantir que não é coincidência.

---

## Seção 1 — Dois bugs de fixture em `TestTurmaVinculo`

Arquivo: `cmd/server/turma_vinculo_estudante_integration_test.go`

### 1.1 — `TestTurmaVinculo10`: nome de fixture com dígito

**Causa raiz:** o validador de nome de estudante rejeita qualquer caractere que não seja letra,
acento, espaço ou apóstrofo. O fixture gerava `"Aluno concorrente 0"`, `"Aluno concorrente 1"` —
com dígito — e a criação do estudante falhava com `400`, derrubando o teste sempre.

```go
// ANTES:
		c := camposCadastro(fmt.Sprintf("Aluno concorrente %d", i))

// DEPOIS:
		c := camposCadastro(fmt.Sprintf("Aluno concorrente %s", string(rune('A'+i))))
```

### 1.2 — `TestTurmaVinculo09`: contagem por nome sem escopo de academia (flaky sob `-count>1`)

**Causa raiz:** `estudanteCount` contava estudantes por `nome` sem filtrar por academia. Como o
fixture usa um nome fixo (`"Aluno job aviso"`) em toda execução, rodar o teste mais de uma vez no
mesmo banco (exatamente o que `-count=3` faz, exigido pela Tarefa 50) acumula matches de execuções
anteriores — na 2ª rodada o count vira 2, na 3ª vira 3, e a asserção `!= 1` passa a falhar. Isso não
é sobre o código de produção: é o teste contando errado.

```go
// ANTES:
func estudanteCount(t *testing.T, fx *turmaVinculoFixture, nome string) int {
	t.Helper()
	var n int
	if err := fx.client.DB().QueryRow(`SELECT count(*) FROM projection_estudantes WHERE nome=$1`, nome).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// DEPOIS:
func estudanteCount(t *testing.T, fx *turmaVinculoFixture, nome string) int {
	t.Helper()
	return estudanteCountPorAcademia(t, fx, nome, fx.academia.CodigoAcademia)
}

// estudanteCountPorAcademia escopa a contagem por codigo_academia. Contar só
// por nome (sem escopo) acumula matches de execuções anteriores no mesmo
// banco — por exemplo, com nomes fixos de fixture repetidos entre rodadas de
// `-count=N` — e produz falsos negativos em testes que esperam count==1.
func estudanteCountPorAcademia(t *testing.T, fx *turmaVinculoFixture, nome, codigoAcademia string) int {
	t.Helper()
	var n int
	if err := fx.client.DB().QueryRow(`SELECT count(*) FROM projection_estudantes WHERE nome=$1 AND codigo_academia=$2`, nome, codigoAcademia).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
```

E, dentro do corpo de `TestTurmaVinculo09FalhaPosCriacaoGeraTurmaAvisoSemAbortarCadastro` (a
academia usada nesse teste específico é `academiaSemAnoLetivo`, não `fx.academia`):

```go
// ANTES:
	_ = projections.NewEstudanteProjection(fx.client).Rebuild()
	if estudanteCount(t, fx, item["nome"]) != 1 {
		t.Fatal("estudante deveria ter sido criado apesar da falha de vínculo")
	}
}

// DEPOIS:
	_ = projections.NewEstudanteProjection(fx.client).Rebuild()
	if estudanteCountPorAcademia(t, fx, item["nome"], academiaSemAnoLetivo.CodigoAcademia) != 1 {
		t.Fatal("estudante deveria ter sido criado apesar da falha de vínculo")
	}
}
```

**Validado pela Claude:** `TestTurmaVinculo -count=3 -v` com banco recriado antes: **33/33 passam**
(11 testes × 3 rodadas), sem nenhuma flakiness em 3 execuções completas seguidas.

---

## Seção 2 — Assert quebrado em `TestIntegrationRebuildNotasEFaltasMantemRegistrosCorrigidos`

Arquivo: `cmd/server/notas_faltas_correcao_integration_test.go`

**Causa raiz — a mais sutil das três, e a mais importante:** este é o teste que prova o fix
central da Tarefa 50 (Seção 0.2: IDs determinísticos de projeção sobrevivendo a um rebuild). Ele
comparava dois snapshots com `primeiro != segundo`, onde a struct `snapshotRegistrosCorrecao` tem
campos `*float64`/`*int` (`NotaAnterior`, `FaltaAnterior`). Em Go, comparar uma struct com `!=`
quando ela contém campos de ponteiro compara o **endereço do ponteiro**, não o valor apontado. Como
cada leitura via `Scan()` aloca ponteiros novos, essa comparação falhava **sempre** — mesmo quando
os dois snapshots tinham valores idênticos. Resultado prático: a Tarefa 50 nunca teve, de fato, uma
prova automatizada rodando de que seu bug de produção mais grave estava corrigido.

```go
// ANTES:
	segundo := snapshotRegistrosCorrigidos(t, fx.client, fx.notaID, fx.faltaID)
	if primeiro != segundo {
		t.Fatalf("rebuild não foi determinístico: primeiro=%+v segundo=%+v", primeiro, segundo)
	}

// DEPOIS:
	segundo := snapshotRegistrosCorrigidos(t, fx.client, fx.notaID, fx.faltaID)
	if !snapshotsIguais(primeiro, segundo) {
		t.Fatalf("rebuild não foi determinístico: primeiro=%s segundo=%s", formatSnapshot(primeiro), formatSnapshot(segundo))
	}
```

E adicione estas duas funções nova logo antes de `func snapshotRegistrosCorrigidos(...)`:

```go
// snapshotsIguais compara dois snapshots por valor. snapshotRegistrosCorrecao
// tem campos *float64/*int (NotaAnterior/FaltaAnterior); comparar a struct
// diretamente com `!=` compara endereço de ponteiro, não o valor apontado, e
// por isso falha sempre — mesmo entre dois snapshots com valores idênticos —
// já que cada leitura via Scan() aloca ponteiros novos.
func snapshotsIguais(a, b snapshotRegistrosCorrecao) bool {
	if a.Nota != b.Nota || a.Falta != b.Falta {
		return false
	}
	if (a.NotaAnterior == nil) != (b.NotaAnterior == nil) {
		return false
	}
	if a.NotaAnterior != nil && *a.NotaAnterior != *b.NotaAnterior {
		return false
	}
	if (a.FaltaAnterior == nil) != (b.FaltaAnterior == nil) {
		return false
	}
	if a.FaltaAnterior != nil && *a.FaltaAnterior != *b.FaltaAnterior {
		return false
	}
	return true
}

func formatSnapshot(s snapshotRegistrosCorrecao) string {
	notaAnterior := "nil"
	if s.NotaAnterior != nil {
		notaAnterior = fmt.Sprintf("%v", *s.NotaAnterior)
	}
	faltaAnterior := "nil"
	if s.FaltaAnterior != nil {
		faltaAnterior = fmt.Sprintf("%v", *s.FaltaAnterior)
	}
	return fmt.Sprintf("{Nota:%v NotaAnterior:%s Falta:%v FaltaAnterior:%s}", s.Nota, notaAnterior, s.Falta, faltaAnterior)
}
```

**Validado pela Claude:** com esta correção, `TestIntegrationRebuildNotasEFaltasMantemRegistrosCorrigidos`
passa de verdade contra Postgres real — junto com o resto da suíte de notas/faltas, **14/14**.

---

## Seção 3 — `chdir` ausente em `internal/db` (bug pré-existente, já citado na Tarefa 52)

A Tarefa 52 já documentou esta falha como "pré-existente e fora de escopo" — esta tarefa é onde ela
é efetivamente corrigida.

**Causa raiz:** ao contrário dos testes de integração em `cmd/server` (que fazem
`os.Chdir("../..")` antes de rodar migrations), estes dois testes em `internal/db` chamam
`client.RunMigrations()` sem antes mudar o diretório de trabalho para a raiz do repo. Como
`RunMigrations()` lê o caminho relativo `"migrations"`, e o pacote roda a partir de `internal/db/`,
a migration nunca é encontrada: `erro ao ler diretório de migrations 'migrations': no such file or
directory`.

### 3.1 — `internal/db/event_store_integrity_test.go`

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

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDir) })

	client, err := NewClient(DefaultConfig())
```

### 3.2 — `internal/db/repository_concurrency_test.go`

```go
// ANTES:
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}
	client, err := NewClient(DefaultConfig())

// DEPOIS:
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDir) })

	client, err := NewClient(DefaultConfig())
```

Aplicando só a Seção 3.1, `TestLedgerAppendOnlyTriggersAndIntegrityIfDatabaseAvailable` já passa
sozinho. `TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable` (3.2) continua falhando
mesmo depois do `chdir` — só que agora falha por um motivo completamente diferente e muito mais
importante: é aí que o bug real de produção da Seção 4 apareceu.

---

## Seção 4 — Bug real de concorrência em `SaveWithAudit` (produção, não só teste)

**Este é o achado mais importante desta tarefa.** Depois de corrigir o `chdir` (Seção 3.2), o teste
de concorrência (`internal/db/repository_concurrency_test.go`, 8 escritores simultâneos gravando no
mesmo aggregate) passou a rodar de verdade contra Postgres — e falhou de forma **100% reprodutível**
(8/8 execuções), sempre com o mesmo erro:

```
concurrent SaveWithAudit failed: erro ao salvar evento: erro ao adicionar evento na transação:
pq: duplicate key value violates unique constraint "spuri_ledger_aggregate_id_event_version_key"
```

### Causa raiz

`SaveWithAudit` (`internal/db/repository.go`) já tinha lógica de retry para conflitos de escrita
concorrente — mas só reconhecia como "retryable" os códigos Postgres `40001`
(`serialization_failure`) e `40P01` (`deadlock_detected`), via `utils.IsSerializationFailure`.

O que acontece de fato quando dois escritores concorrentes leem a mesma versão corrente do mesmo
aggregate e tentam inserir o próximo `event_version` ao mesmo tempo: sob isolamento
`SERIALIZABLE`, o mecanismo de detecção preventiva do Postgres (SSI) **nem sempre** aborta a
segunda transação a tempo com `40001` — quando a primeira já comitou antes da segunda tentar o
`INSERT`, a segunda simplesmente esbarra na constraint física de unicidade
(`spuri_ledger_aggregate_id_event_version_key`) e recebe `23505` (`unique_violation`) em vez de
`40001`. É o mesmo conflito de concorrência, só que manifestado de outra forma — e
`IsSerializationFailure` não reconhecia essa forma, então `SaveWithAudit` desistia na primeira
tentativa em vez de tentar de novo com a versão correta.

**Isso não é um problema só do teste.** Qualquer escrita concorrente real no mesmo aggregate em
produção (duas requisições da API tentando alterar o mesmo estudante/turma/nota ao mesmo tempo, por
exemplo) pode sofrer o mesmo destino: um 500 definitivo em vez do retry transparente que
`SaveWithAudit` foi desenhado para fazer.

### 4.1 — `internal/utils/errors.go`

```go
// ANTES:
func IsSerializationFailure(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "40001" || string(pqErr.Code) == "40P01"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlstate 40001") || strings.Contains(msg, "serialization_failure") || strings.Contains(msg, "sqlstate 40p01") || strings.Contains(msg, "deadlock detected")
}

// DEPOIS:
func IsSerializationFailure(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if string(pqErr.Code) == "40001" || string(pqErr.Code) == "40P01" {
			return true
		}
		// 23505 (unique_violation) na constraint de unicidade de versão do
		// ledger (aggregate_id, event_version) é o mesmo conflito de escrita
		// concorrente que 40001 detecta preventivamente: duas transações leem
		// a mesma versão corrente do aggregate e tentam inserir o mesmo
		// próximo event_version. Sob SERIALIZABLE, o SSI do Postgres nem
		// sempre aborta a segunda transação antes do INSERT físico — quando a
		// primeira já comitou, a segunda esbarra na constraint de unicidade
		// em vez de receber 40001. Tratar como retryable aqui é seguro porque
		// esta função tem um único call site (SaveWithAudit), cujo próprio
		// propósito é resolver exatamente esta corrida relendo a versão.
		if string(pqErr.Code) == "23505" && pqErr.Constraint == "spuri_ledger_aggregate_id_event_version_key" {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "sqlstate 40001") || strings.Contains(msg, "serialization_failure") || strings.Contains(msg, "sqlstate 40p01") || strings.Contains(msg, "deadlock detected")
}
```

### 4.2 — `internal/db/repository.go`: budget de retry insuficiente

Corrigir só a classificação acima **não bastou sozinha**. A Claude testou isoladamente:

| Tentativas (`attempt < N`) | Classificação 23505 corrigida? | Resultado (8 escritores concorrentes) |
|---|---|---|
| 3 (valor original) | Não | 8/8 execuções falham (bug original) |
| 3 | Sim | 8/8 execuções falham — 3 tentativas não é budget suficiente |
| 5 | Sim | 3/8 execuções falham — ainda flaky |
| 8 | Sim | **10/10 execuções passam** |
| 10 | Sim | 6/6 execuções passam |
| 10 | Não | 6/6 execuções falham — confirma que a classificação é a correção essencial, não o budget sozinho |

Ou seja: as duas mudanças são necessárias juntas. `attempt < 8` foi escolhido porque, no pior caso
com N escritores concorrentes no mesmo aggregate, um escritor "azarado" pode precisar de até N-1
retries para conseguir a versão correta — 8 casa com o próprio teste de concorrência (8
escritores), e foi validado com 10/10 execuções reais sem falha.

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
	var err error
	// Budget de 8 tentativas: sob concorrência real (N escritores simultâneos
	// no mesmo aggregate), o pior caso para um escritor "azarado" é precisar
	// de até N-1 retries para conseguir a versão correta. Validado
	// empiricamente com 8 escritores concorrentes (10/10 execuções reais
	// contra Postgres sem falha; 5 tentativas falhava em ~3/8 execuções). Ver
	// Tarefa 53, Seção 4.
	for attempt := 0; attempt < 8; attempt++ {
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
```

**Validado pela Claude:** `TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable`
passa em 10/10 execuções reais e independentes (banco recriado a cada rodada) com as duas mudanças
juntas.

### Se você (Fredy) preferir um número diferente de tentativas

O valor 8 é uma recomendação com evidência, não uma verdade absoluta — é uma decisão de quanto
budget de retry (e, portanto, de latência no pior caso: até `20+40+...+160ms ≈ 720ms` de backoff
acumulado) vale a pena pagar para tolerar N escritores concorrentes no mesmo aggregate. Se
academias/estudantes específicos puderem sofrer mais de 8 escritas simultâneas no mesmo aggregate
em produção, considere um valor maior; a tabela acima dá a base empírica para essa decisão.

---

## Comandos que você deve rodar (sem Postgres — Codex)

```bash
go build ./...
go vet ./...
go test ./cmd/server/... -run 'TestTurmaVinculo' -v
go test ./cmd/server/... -run 'TestFaltasPeriodo|TestNotasFaltas|TestHTTPIntegrationCorrigir|TestIntegrationRebuild' -v
go test ./internal/db/... -v
go test ./...
```

Todos os testes tocados por esta tarefa são condicionados a `SPURI_RUN_DB_INTEGRITY_TESTS=1`, então
no seu ambiente eles vão aparecer como `--- SKIP` — isso é esperado e não deve ser tratado como
falha. **Resultado esperado:** `go build` e `go vet` sem nenhuma saída (sucesso silencioso); todo o
resto com `ok`/`SKIP`, sem nenhum `FAIL`. Um `FAIL` de compilação (não de banco) nesses comandos
indica que algum bloco "DEPOIS" foi colado errado — revise contra o texto desta tarefa.

## O que você NÃO deve fazer

- Não altere `go.mod` nem `go.sum`.
- Não tente instalar PostgreSQL, Docker ou usar `apt` — não vai funcionar no seu ambiente.
- Não defina `SPURI_RUN_DB_INTEGRITY_TESTS=1` — sem Postgres real, isso só vai travar tentando
  conectar.
- Não mude o valor `8` da Seção 4.2 por conta própria — se achar que devia ser outro número, escreva
  isso como observação no seu relatório final, mas não decida sozinho (ver tabela empírica acima).
- Não toque em nenhum outro arquivo além dos listados nas Seções 1 a 4.
- Não marque esta tarefa como concluída nem mova este arquivo para "Tarefas feitas".

## Commit sugerido

```
fix(tests,ledger): corrigir 3 bugs de teste da Tarefa 50 e bug real de
concorrência em SaveWithAudit

- TestTurmaVinculo10: fixture com dígito no nome violava validação
- TestTurmaVinculo09: contagem de estudante sem escopo de academia,
  acumulava entre execuções repetidas (-count>1)
- TestIntegrationRebuildNotasEFaltasMantemRegistrosCorrigidos: comparação
  de struct com campos de ponteiro via != comparava endereço, não valor
  — teste nunca provava de fato o fix de IDs determinísticos da Tarefa 50
- internal/db: chdir ausente antes de RunMigrations() em 2 testes
  (bug pré-existente, já citado na Tarefa 52 como fora de escopo)
- SaveWithAudit: retry não reconhecia 23505 (unique_violation) na
  constraint de versão do ledger como o mesmo conflito de concorrência
  que 40001 — sob SERIALIZABLE, o SSI do Postgres nem sempre antecipa o
  conflito como 40001 antes do INSERT físico. Retry budget insuficiente
  (3) também corrigido para 8, validado com 8 escritores concorrentes.
```
