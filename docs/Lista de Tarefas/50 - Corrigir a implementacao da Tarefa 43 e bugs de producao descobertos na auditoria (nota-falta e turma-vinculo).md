---
status: a fazer
---

# Tarefa 50 — Corrigir a implementação da Tarefa 43 e bugs de produção descobertos na auditoria pós-implementação

## Prompt recomendado para Codex

```
Execute EXATAMENTE as instruções da Tarefa 44 (docs/Lista de Tarefas/44 - ...md), na ordem em
que aparecem: primeiro a Seção 0 (bugs de produção), depois a Seção 1 e a Seção 2 (arquivos de
teste). Cada bloco de código já está pronto para copiar; não invente nada além do que está
escrito. Depois de aplicar tudo, rode exatamente os comandos da seção "Testes obrigatórios" com
SPURI_RUN_DB_INTEGRITY_TESTS=1 contra um PostgreSQL real e cole a saída completa. NÃO marque a
tarefa como concluída, nem mova o arquivo para "Tarefas feitas", se qualquer teste falhar. Para
a Seção 3, siga as instruções de investigação (não são fixes prontos) e documente o que
encontrar antes de decidir o que fazer.
```

## Contexto e diagnóstico

Esta tarefa nasceu de uma auditoria pós-implementação da Tarefa 43. A implementação da Tarefa 43
(commit `009f16b` da Codex) tinha a estrutura certa (arquivos no lugar certo, package `main` em
`cmd/server`, stub antigo removido), mas **nenhum dos 24 testes novos/afetados (11 de
`turma_vinculo_estudante_integration_test.go` + 13 novos de `notas_faltas_correcao_integration_test.go`
+ 2 testes originais da Tarefa 38 que passam a ser exercidos pela mesma fixture) tinha sido
executado contra um PostgreSQL real** — a Codex reportou que o ambiente dela não tinha Postgres
acessível. Eu instalei Go 1.22 (via apt) e PostgreSQL 16 (via apt) num sandbox, contornei as
restrições de rede do proxy de módulos Go com `replace` directives temporárias apontando para
espelhos no GitHub (só para viabilizar o build local — **não faça isso no repositório real**, lá
o `go.mod`/`go.sum` já devem resolver normalmente), e rodei os testes de verdade, iterando
build → rodar → diagnosticar → corrigir → rodar de novo dezenas de vezes até verificar cada
correção abaixo.

**Resultado da auditoria**: a implementação da Tarefa 43, como está no commit `009f16b`, falha
quase inteiramente contra Postgres real. Encontrei 10 causas-raiz distintas nos testes novos, e
**dois bugs de produção graves e pré-existentes** (não introduzidos pela Tarefa 43, mas expostos
por ela) que fazem a funcionalidade de "correção de nota/falta" — o assunto central das Tarefas
33 a 39 — nunca ter funcionado de verdade em produção. Depois de aplicar todas as correções
abaixo, o estado é:

- `turma_vinculo_estudante_integration_test.go`: **11/11 testes passam**, verificado com
  múltiplas execuções (inclusive `-count=3` para checar flakiness na concorrência).
- `notas_faltas_correcao_integration_test.go`: **13/15 testes passam**. Os 2 restantes
  (`TestFaltasPeriodo15HistoricaSemPeriodoListavelECorrigivel` e
  `TestHTTPIntegrationCorrigirNotaRecalculaAvaliacaoFinal`) precisam de uma decisão de design
  antes de corrigir — ver Seção 3. **Não invente uma correção para esses dois sem seguir a
  investigação pedida na Seção 3.**

As correções abaixo estão na ordem em que devem ser aplicadas. A Seção 0 é a mais importante:
sem ela, habilitar a correção de notas/faltas em produção (que hoje está completamente quebrada)
pode piorar as coisas se aplicada pela metade.

---

## Seção 0 — CRÍTICO: bugs de produção (não são bugs de teste, precisam ir juntos)

### 0.1 — `NotaCorrigida` e `FaltaCorrigida` nunca estiveram na whitelist de eventos do ledger

**Evidência**: `Estudante.CorrigirNota` (`internal/domain/aggregates/estudante_notas.go:227`) e
`Estudante.CorrigirFalta` (`internal/domain/aggregates/estudante_falta.go:151`) levantam eventos
`"NotaCorrigida"` e `"FaltaCorrigida"` respectivamente. Mas `internal/db/safe_queries.go`, que
define a whitelist `validEventTypes` usada por `ValidateEventType` (chamada por todo
`SaveWithAudit`/`AppendTx` antes de gravar no ledger), **nunca incluiu esses dois tipos**. Isso
significa que **toda tentativa de corrigir uma nota ou falta pelas rotas
`PATCH /academia/notas-aluno/{id}` e `PATCH /academia/faltas-aluno/{id}` sempre retornou
500 Internal Server Error**, desde que essas rotas foram implementadas (Tarefas 33/35). Confirmei
isso rodando os dois testes originais da Tarefa 38
(`TestHTTPIntegrationCorrigirNotaEFalta`, `TestHTTPIntegrationCorrigirNotaRecalculaAvaliacaoFinal`)
contra Postgres real pela primeira vez: ambos falhavam com
`"tipo de evento inválido: NotaCorrigida"`.

**Verifique primeiro no ambiente de produção real** (consulta somente leitura, sem side effects)
se alguma correção já foi tentada e gerou dado inconsistente:

```sql
SELECT count(*) FROM spuri_ledger WHERE event_type IN ('NotaCorrigida', 'FaltaCorrigida');
```

Esperado: `0`. Se não for `0`, pare e avise o Fredy antes de continuar — significa que o
comportamento pode ser diferente do que documentei aqui (por exemplo, se a whitelist em
produção já foi corrigida por outro caminho).

**Correção** — em `internal/db/safe_queries.go`, localize o fim do mapa `validEventTypes`
(a chave `"MensalidadesCobrancaConfirmada"` é a penúltima entrada antes do `}` de fechamento) e
aplique:

```go
// ANTES (final do mapa validEventTypes):
	"MatriculaConfigurada":           true,
	"MensalidadesCobrancaConfirmada": true,
}

// DEPOIS:
	"MatriculaConfigurada":           true,
	"MensalidadesCobrancaConfirmada": true,
	// NotaCorrigida e FaltaCorrigida são emitidos por
	// Estudante.CorrigirNota/CorrigirFalta (estudante_notas.go /
	// estudante_falta.go) desde a Tarefa 33/35, mas nunca constavam nesta
	// whitelist: todo SaveWithAudit para uma correção de nota ou falta era
	// rejeitado com "tipo de evento inválido" antes de tentar gravar no
	// ledger, fazendo a rota PATCH /academia/notas-aluno/{id} e
	// /academia/faltas-aluno/{id} retornar 500 sempre.
	"NotaCorrigida":  true,
	"FaltaCorrigida": true,
}
```

### 0.2 — IDs não determinísticos em `projection_notas`/`projection_faltas` quebram o replay depois de QUALQUER correção

**Esta é a parte mais grave da auditoria.** Depois de aplicar só a correção 0.1 acima (sem esta),
o comportamento piora: a primeira correção de nota/falta funciona, mas **qualquer rebuild
completo subsequente da projeção `notas` ou `faltas` passa a falhar permanentemente**, com erros
como:

```
handleNotaCorrigida: nota original f520b797-... não encontrada
handleFaltaCorrigida: falta original ... não encontrada
```

**Causa raiz**: `handleNotasRegistradas` (`internal/projections/notas_projection.go`) e
`handleFaltasRegistradasTx` (`internal/projections/faltas_projection.go`) inserem linhas em
`projection_notas`/`projection_faltas` **sem especificar a coluna `id`**, deixando o Postgres
gerar um UUID aleatório via `DEFAULT` a cada `INSERT`. O evento `NotaCorrigida`/`FaltaCorrigida`,
por sua vez, grava permanentemente no payload do ledger (imutável) uma referência a esse `id`
aleatório (`NotaAnteriorID`/`FaltaAnteriorID`), capturado no momento da correção.

Isso funciona enquanto a projeção nunca é reconstruída do zero. Mas assim que
`NotasProjection.Rebuild()`/`FaltasProjection.Rebuild()` roda de novo (o que acontece
rotineiramente neste sistema — após um cold-start do NeonDB, uma manutenção administrativa, ou
qualquer rebuild geral via `Manager.RebuildAllProjections()`), o evento `NotasRegistradas`
original é reprocessado primeiro e a linha ganha um **novo** `id` aleatório (diferente do
anterior). Quando o evento `NotaCorrigida` é reprocessado em seguida, ele procura
`WHERE id = <id antigo>` — que não existe mais — e o `UPDATE` afeta 0 linhas, o que o handler
trata como erro fatal (`rows != 1`), **abortando o rebuild inteiro da projeção**.

Comprovei isso na prática: com a correção 0.1 isolada, rodar toda a suíte de testes de
notas/faltas em sequência (cada teste faz seu próprio setup, que inclui um rebuild completo da
projeção) fazia **15 de 15 testes falharem**, porque o primeiro teste a corrigir uma nota
"envenenava" todos os rebuilds seguintes na mesma execução.

**Correção**: os IDs de `projection_notas`/`projection_faltas` devem ser determinísticos,
derivados da própria chave natural do registro (a mesma tupla já usada nas constraints
`ON CONFLICT`), usando `uuid.NewSHA1`, que já é o padrão estabelecido neste repositório (veja
`internal/handlers/modelo_avaliativo_escolar.go:67` e `internal/finance/mensalidade.go:482`).
Isso garante que o mesmo registro lógico sempre recebe o mesmo `id`, não importa quantas vezes a
projeção seja reconstruída — exatamente a propriedade de idempotência que este sistema já exige
de toda projeção reconstruível.

Em `internal/projections/notas_projection.go`, função `handleNotasRegistradas`:

```go
// ANTES:
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleNotasRegistradas: parse error: %w", err)
	}

	result, err := p.client.DB().Exec(`
		INSERT INTO projection_notas (
			codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			periodo, materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, registrado_por, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
		DO NOTHING
	`,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Periodo, payload.MateriaDisciplinarID, payload.Tipo, payload.Categoria,
		payload.Nota, payload.Observacao,
		payload.RegisteredAt, payload.RegistradoPor, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleNotasRegistradas: exec error: %w", err)
	}

// DEPOIS:
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("handleNotasRegistradas: parse error: %w", err)
	}

	notaID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("spuri:nota:"+payload.CodigoEstudante+":"+payload.CodigoAcademia+":"+payload.AnoLectivo+":"+payload.Periodo+":"+payload.MateriaDisciplinarID+":"+payload.Tipo+":"+payload.Categoria))
	result, err := p.client.DB().Exec(`
		INSERT INTO projection_notas (
			id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			periodo, materia_disciplinar_id, tipo, categoria, nota, observacao,
			registered_at, registrado_por, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (codigo_estudante, codigo_academia, ano_lectivo, periodo, materia_disciplinar_id, tipo, categoria)
		DO NOTHING
	`,
		notaID, payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Periodo, payload.MateriaDisciplinarID, payload.Tipo, payload.Categoria,
		payload.Nota, payload.Observacao,
		payload.RegisteredAt, payload.RegistradoPor, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleNotasRegistradas: exec error: %w", err)
	}
```

`payload.Periodo` já é usado como parte da chave `ON CONFLICT`, então usá-lo também no hash é
seguro (mesma granularidade). `uuid` já está importado neste arquivo.

Em `internal/projections/faltas_projection.go`, função `handleFaltasRegistradasTx`:

```go
// ANTES:
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error FaltasRegistradas: %w", err)
	}

	_, err := tx.Exec(`
		INSERT INTO projection_faltas (
			codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			periodo, data, materia_disciplinar_id, quantidade, observacao,
			registered_at, registrado_por, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT ON CONSTRAINT uq_falta_unica DO NOTHING
	`,
		payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Periodo, payload.Data.UTC(), payload.MateriaDisciplinarID, payload.Quantidade, payload.Observacao,
		payload.RegisteredAt.UTC(), payload.RegistradoPor, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleFaltasRegistradasTx: exec error: %w", err)
	}
	return nil
}

// DEPOIS:
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse error FaltasRegistradas: %w", err)
	}

	faltaID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("spuri:falta:"+payload.CodigoEstudante+":"+payload.CodigoAcademia+":"+payload.Data.UTC().Format("2006-01-02")+":"+payload.MateriaDisciplinarID+":"+payload.Periodo))
	_, err := tx.Exec(`
		INSERT INTO projection_faltas (
			id, codigo_estudante, codigo_academia, ano_lectivo, ano_academico,
			periodo, data, materia_disciplinar_id, quantidade, observacao,
			registered_at, registrado_por, event_id, version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT ON CONSTRAINT uq_falta_unica DO NOTHING
	`,
		faltaID, payload.CodigoEstudante, payload.CodigoAcademia, payload.AnoLectivo, payload.AnoAcademico,
		payload.Periodo, payload.Data.UTC(), payload.MateriaDisciplinarID, payload.Quantidade, payload.Observacao,
		payload.RegisteredAt.UTC(), payload.RegistradoPor, event.EventID, event.EventVersion,
	)
	if err != nil {
		return fmt.Errorf("handleFaltasRegistradasTx: exec error: %w", err)
	}
	return nil
}
```

`faltas_projection.go` **não** importa `github.com/google/uuid` ainda — adicione ao bloco de
`import`:

```go
// ANTES:
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/db"
	"spuri/internal/utils"
	"time"
)

// DEPOIS:
import (
	"database/sql"
	"encoding/json"
	"fmt"
	"spuri/internal/db"
	"spuri/internal/utils"
	"time"

	"github.com/google/uuid"
)
```

**Não existe FK de nenhuma outra tabela para `projection_notas.id`/`projection_faltas.id`**
(verificado com `grep -rn "REFERENCES projection_notas\|REFERENCES projection_faltas"
migrations/*.sql`, sem resultados), então esta mudança não exige migração de schema nem afeta
outras tabelas. Como a Seção 0.1 confirma que nenhum `NotaCorrigida`/`FaltaCorrigida` jamais foi
gravado com sucesso em produção (a whitelist sempre rejeitou antes), **não há dado histórico para
migrar** — mas confirme isso rodando a query de verificação da Seção 0.1 antes de aplicar, por
segurança.

Depois de 0.1 + 0.2 juntas, rodei a suíte completa de notas/faltas (15 testes, cada um fazendo
seu próprio setup+rebuild) e o "envenenamento" entre testes desapareceu — os testes voltam a
falhar (ou passar) apenas pelos próprios motivos específicos de cada um, não mais em cascata.

---

## Seção 1 — `cmd/server/turma_vinculo_estudante_integration_test.go`

Depois de todas as correções desta seção, os 11 testes (`TestTurmaVinculo01` a `11`) passam
consistentemente (testado com `-count=3`).

### 1.1 — Import de `net/textproto` e `github.com/google/uuid`

```go
// ANTES:
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"

// DEPOIS:
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/db"
```

### 1.2 — `t.Setenv("ENV", "test")` ausente — causa um panic real (nil pointer)

**Diagnóstico**: sem isso, `getStorageProvider(c)` no handler de cadastro de estudante
(`internal/handlers/estudante_handlers.go`, por volta da linha 189) não encontra um provider no
contexto (só `main.go` injeta o `storageProvider` global via `initStorage()`, que o fixture de
teste nunca chama) e cai no fallback `p, _ := storage.NewStorageProvider()` — que tenta construir
um cliente Mega real (sem credenciais no ambiente de teste), retorna erro, esse erro é
descartado, e `provider` fica `nil`. A chamada seguinte `provider.EnsureDir(dir)` gera um panic
de nil pointer que derruba o processo de teste inteiro (`panic: runtime error: invalid memory
address or nil pointer dereference`, goroutine crash em
`estudante_handlers.go:189`).

`internal/storage/storage.go` já tem um fallback local para isso: `useLocalMegaFallback()`
retorna `true` quando `ENV=test`, fazendo `NewMegaProvider()` devolver um provider local (sem
tentar rede), sem erro. Esse é o padrão já usado por outros testes de integração deste repo
(`t.Setenv("ENV", "test")` aparece em `internal/handlers/financeiro_matricula_consulta_test.go`,
`internal/handlers/financeiro_handlers_integration_test.go`,
`internal/finance/appypay_integration_test.go`, `internal/storage/storage_test.go`) — só faltava
em `turma_vinculo_estudante_integration_test.go`.

```go
// ANTES (dentro de setupTurmaVinculoIntegration, logo depois do t.Skip):
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}
	prev, _ := os.Getwd()

// DEPOIS:
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}
	t.Setenv("ENV", "test")
	prev, _ := os.Getwd()
```

### 1.3 — `criarAcademiaCorrecao` nunca chama `DefinirAnoLetivo`

**Diagnóstico**: `vincularEstudanteATurma` (handler) chama internamente
`resolverAnoLetivoAcademia(academiaDTO.AnoLetivo, ...)`, que retorna erro se `AnoLetivo` estiver
vazio. `Academia.Criar(...)` **não** aceita `anoLetivo` como parâmetro — é preciso chamar
`Academia.DefinirAnoLetivo(...)` à parte. `criarAcademiaCorrecao` (função de fixture
compartilhada com a Tarefa 38, em `notas_faltas_correcao_integration_test.go`) nunca faz essa
chamada, então **toda tentativa de vincular um estudante recém-criado a uma turma falhava**
com `"a academia '...' não possui um ano letivo ativo"` — mascarando a validação real que cada
cenário pretendia testar. Não existe nenhum teste HTTP anterior a este exercitando esse caminho
(`vincularEstudanteATurma` via HTTP), então isso nunca tinha aparecido antes.

```go
// ANTES (dentro de setupTurmaVinculoIntegration):
	seq := time.Now().UnixNano()
	academia := criarAcademiaCorrecao(t, repository, seq, "V")
	outra := criarAcademiaCorrecao(t, repository, seq, "W")
	if err := projections.NewAcademiaProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild academias: %v", err)
	}

// DEPOIS:
	seq := time.Now().UnixNano()
	academia := criarAcademiaCorrecao(t, repository, seq, "V")
	outra := criarAcademiaCorrecao(t, repository, seq, "W")
	if err := academia.DefinirAnoLetivo("2025_2026", "escolar", academia.ID); err != nil {
		t.Fatalf("definir ano letivo academia: %v", err)
	}
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar ano letivo academia: %v", err)
	}
	if err := outra.DefinirAnoLetivo("2025_2026", "escolar", outra.ID); err != nil {
		t.Fatalf("definir ano letivo outra academia: %v", err)
	}
	if err := repository.SaveWithAudit(outra, db.AuditContext{UserID: outra.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar ano letivo outra academia: %v", err)
	}
	if err := projections.NewAcademiaProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild academias: %v", err)
	}
```

### 1.4 — `montarMultipartCadastroEstudante` usa `CreateFormFile`, que gera `Content-Type: application/octet-stream`

**Diagnóstico**: `readAndValidatePDF` (`internal/handlers/solicitacao_matricula_handlers.go:698`)
exige `Content-Type: application/pdf` no part do multipart. `multipart.Writer.CreateFormFile` do
Go **hardcoda** `Content-Type: application/octet-stream` — não há como mudar isso usando
`CreateFormFile`. Isso fazia **todo teste que chama `postCadastro` com `comArquivos=true`** (a
maioria) falhar com `"bi_encarregado deve ser PDF"`, mesmo com conteúdo `%PDF-1.4` válido. O
padrão correto já existe no repositório: `multipartFileHeader` em
`internal/handlers/solicitacao_matricula_handlers_test.go` usa `CreatePart` com um
`textproto.MIMEHeader` explícito.

```go
// ANTES (dentro de montarMultipartCadastroEstudante):
	if comArquivos {
		for _, f := range []string{"bi_encarregado", "cedula_estudante"} {
			part, err := w.CreateFormFile(f, f+".pdf")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte("%PDF-1.4\n%%EOF")); err != nil {
				t.Fatal(err)
			}
		}
	}

// DEPOIS:
	if comArquivos {
		for _, f := range []string{"bi_encarregado", "cedula_estudante"} {
			header := make(textproto.MIMEHeader)
			header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, f, f+".pdf"))
			header.Set("Content-Type", "application/pdf")
			part, err := w.CreatePart(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte("%PDF-1.4\n%%EOF")); err != nil {
				t.Fatal(err)
			}
		}
	}
```

### 1.5 — `telefone_encarregado` hardcoded e idêntico em todo teste

**Diagnóstico**: `camposCadastro` usava sempre `"912345678"` para todo estudante criado por
qualquer teste. Isso colide com a constraint real de unicidade de telefone assim que mais de um
estudante com o mesmo telefone existe no banco e uma projeção é reconstruída do zero (o que
alguns cenários exigem — ver 1.7). A Tarefa 43 original já orientava reaproveitar um gerador de
dígitos aleatórios; adicione este helper local (o mesmo padrão de
`internal/handlers/financeiro_handlers_integration_test.go:49`, que está noutro pacote e não pode
ser importado diretamente):

```go
// ANTES:
func camposCadastro(nome string) map[string]string {
	return map[string]string{"nome": nome, "genero": "masculino", "data_nascimento": "2014-01-01", "ano_escolar_fundamental": "1_ano_fundamental", "telefone_encarregado": "912345678", "bilhete_identidade_encarregado": "BI" + strings.ReplaceAll(nome, " ", "")}
}

// DEPOIS:
func geraDigitosTurmaVinculo(n int) string {
	digitos := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, uuid.NewString())
	for len(digitos) < n {
		digitos += "0"
	}
	return digitos[:n]
}
func camposCadastro(nome string) map[string]string {
	return map[string]string{"nome": nome, "genero": "masculino", "data_nascimento": "2014-01-01", "ano_escolar_fundamental": "1_ano_fundamental", "telefone_encarregado": "9" + geraDigitosTurmaVinculo(8), "bilhete_identidade_encarregado": "BI" + strings.ReplaceAll(nome, " ", "")}
}
```

### 1.6 — `TestTurmaVinculo08` e `TestTurmaVinculo10` embutiam um dígito no nome do aluno via `fmt.Sprintf("... %d", i)`

**Diagnóstico**: a validação de nome de estudante exige "apenas letras, acentos, espaços e
apóstrofos" — um nome como `"Aluno job 0"` (com dígito) é rejeitado com 400 antes mesmo de
chegar à lógica que o cenário pretendia testar.

### 1.7 — `TestTurmaVinculo08`: a suposição original do cenário (turma inexistente → 2xx com aviso) está errada; o comportamento real é 404

**Diagnóstico**: rodando contra Postgres real, uma requisição de cadastro (individual OU em
lote/job) com `codigo_turma` inexistente é rejeitada com **404 antes de criar o estudante** — a
mesma pré-checagem usada no cadastro individual (`turmaDTO == nil` →
`RespondWithNotFoundError`) roda também no item de job, e ela acontece **antes** de o estudante
ser persistido. Não existe nenhum caminho de "aviso suave" para um código de turma inexistente —
esse mecanismo (`turma_vinculada=false` + `turma_aviso`) só existe para uma turma que passa nessa
pré-checagem mas falha depois, durante `vincularEstudanteATurma` propriamente dito (ver 1.8). A
suposição original do documento da Tarefa 43 nesse ponto estava errada; meça sempre o
comportamento real, não assuma.

O teste também não conferia os campos `turma_vinculada`/`turma_aviso` da resposta — apenas o
código HTTP — o que teria mascarado até uma regressão real nesse comportamento.

Substitua **`TestTurmaVinculo08CadastroEmMassaTrataItensComESemCodigoTurmaDeFormaIndependente`
inteira** e **`TestTurmaVinculo09FalhaPosCriacaoGeraTurmaAvisoSemAbortarCadastro` inteira**
(ver 1.8 para o motivo da reescrita de `09`) pelo bloco abaixo, cobrindo também os itens 1.5 e
1.6 já embutidos:

```go
// ANTES (as duas funções, na íntegra):
func TestTurmaVinculo08CadastroEmMassaTrataItensComESemCodigoTurmaDeFormaIndependente(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	for i, cod := range []string{"", fx.turmaAtiva.CodigoTurma, "TURMA_INEXISTENTE"} {
		item := camposCadastro(fmt.Sprintf("Aluno job %d", i))
		item["codigo_turma"] = cod
		w := jobCadastro(t, fx, item)
		if w.Code < 200 || w.Code > 299 {
			t.Fatalf("item %d status=%d %s", i, w.Code, w.Body.String())
		}
	}
}
func TestTurmaVinculo09FalhaPosCriacaoGeraTurmaAvisoSemAbortarCadastro(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	item := camposCadastro("Aluno job aviso")
	item["codigo_turma"] = fx.turmaInativa.CodigoTurma
	w := jobCadastro(t, fx, item)
	if w.Code < 200 || w.Code > 299 || !strings.Contains(w.Body.String(), "turma_aviso") {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
}

// DEPOIS (as duas funções, na íntegra):
func TestTurmaVinculo08CadastroEmMassaTrataItensComESemCodigoTurmaDeFormaIndependente(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)

	item0 := camposCadastro("Aluno job Um")
	w0 := jobCadastro(t, fx, item0)
	if w0.Code < 200 || w0.Code > 299 {
		t.Fatalf("item sem turma: status=%d %s", w0.Code, w0.Body.String())
	}
	if v, ok := dataMap(decodeMap(t, w0.Body.Bytes()))["turma_vinculada"]; ok {
		t.Fatalf("item sem turma não deveria ter turma_vinculada no payload: %v", v)
	}

	item1 := camposCadastro("Aluno job Dois")
	item1["codigo_turma"] = fx.turmaAtiva.CodigoTurma
	w1 := jobCadastro(t, fx, item1)
	if w1.Code < 200 || w1.Code > 299 || dataMap(decodeMap(t, w1.Body.Bytes()))["turma_vinculada"] != true {
		t.Fatalf("item com turma válida: status=%d %s", w1.Code, w1.Body.String())
	}

	item2 := camposCadastro("Aluno job Tres")
	item2["codigo_turma"] = "TURMA_INEXISTENTE"
	w2 := jobCadastro(t, fx, item2)
	if w2.Code != 404 {
		t.Fatalf("item com turma inexistente deveria falhar com 404: status=%d %s", w2.Code, w2.Body.String())
	}
	if estudanteCount(t, fx, item2["nome"]) != 0 {
		t.Fatal("item com turma inexistente não deveria criar estudante")
	}
}
func TestTurmaVinculo09FalhaPosCriacaoGeraTurmaAvisoSemAbortarCadastro(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupTurmaVinculoIntegration(t)
	// A turma existe, está ativa e é compatível — passa na pré-checagem de
	// registerEstudantePorAcademiaComRequestModo. Mas a academia usada aqui
	// nunca teve DefinirAnoLetivo chamado, então vincularEstudanteATurma
	// falha depois de o estudante já ter sido persistido (resolverAnoLetivoAcademia
	// erro), gerando turma_vinculada=false + turma_aviso sem abortar o cadastro.
	academiaSemAnoLetivo := criarAcademiaCorrecao(t, fx.repository, time.Now().UnixNano(), "Z")
	if err := projections.NewAcademiaProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild academias: %v", err)
	}
	turma := aggregates.NewTurma()
	if err := turma.Criar("9A", academiaSemAnoLetivo.CodigoAcademia, "1_ano_fundamental", nil, "manha", academiaSemAnoLetivo.ID); err != nil {
		t.Fatalf("criar turma sem ano letivo: %v", err)
	}
	if err := fx.repository.SaveWithAudit(turma, db.AuditContext{UserID: academiaSemAnoLetivo.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar turma sem ano letivo: %v", err)
	}
	if err := projections.NewTurmasProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild turmas: %v", err)
	}
	tokenSemAnoLetivo, err := middleware.GenerateToken(academiaSemAnoLetivo.ID, "academia")
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	item := camposCadastro("Aluno job aviso")
	item["codigo_turma"] = "9A"
	b, _ := json.Marshal(item)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/jobs/item", bytes.NewReader(b))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", academiaSemAnoLetivo.ID)
	c.Set("user_type", "academia")
	c.Set("dbClient", fx.client)
	c.Set("repository", fx.repository)
	c.Set("projManager", projManager)
	_ = tokenSemAnoLetivo
	handlers.RegisterEstudantePorAcademiaJobItem(c)

	if w.Code < 200 || w.Code > 299 || !strings.Contains(w.Body.String(), "turma_aviso") {
		t.Fatalf("status=%d %s", w.Code, w.Body.String())
	}
	_ = projections.NewEstudanteProjection(fx.client).Rebuild()
	if estudanteCount(t, fx, item["nome"]) != 1 {
		t.Fatal("estudante deveria ter sido criado apesar da falha de vínculo")
	}
}
```

### 1.8 — `TestTurmaVinculo09` original usava `fx.turmaInativa`, que é rejeitada na pré-checagem (não gera aviso)

Já coberto pela reescrita em 1.7: uma turma inativa é bloqueada na mesma pré-checagem que bloqueia
turma inexistente (retorna 400 antes de criar o estudante) — isso é exatamente o que
`TestTurmaVinculo05` já testa. O único jeito real de a turma passar na pré-checagem e falhar
depois, durante `vincularEstudanteATurma`, é uma falha que a pré-checagem **não replica** — e
`resolverAnoLetivoAcademia` (chamado só dentro de `vincularEstudanteATurma`, nunca na
pré-checagem) é exatamente esse gap. Por isso a reescrita usa uma academia sem `ano_letivo`
definido, com uma turma válida e ativa, em vez de reaproveitar `fx.turmaInativa`.

### 1.9 — Rebuilds explícitos de projeção ausentes antes de operações que dependem delas

**Diagnóstico**: o fixture não roda `go projManager.StartProcessing()` (não há worker assíncrono
de projeção rodando durante os testes) — toda leitura via projeção só reflete o ledger depois de
um `Rebuild()` explícito. As rotas `AdicionarEstudanteATurma` (rota manual) e a checagem de
duplicidade dentro de `vincularEstudanteATurma` leem a partir de
`projections.EstudanteProjection`/`TurmasProjection`, não do agregado. Sem um rebuild explícito
logo após criar/vincular um estudante, chamadas HTTP subsequentes no mesmo teste recebem
`"estudante não encontrado"` mesmo o estudante tendo sido criado com sucesso (o `postCadastro`
anterior retornou 201). Isso afeta `TestTurmaVinculo10` (concorrência) e
`TestTurmaVinculo11` (rota manual).

Em `TestTurmaVinculo10ConflitoOtimistaNoVinculoTemRetryOuFalhaLimpaSemCorromperTurma`, logo
depois do loop que cria os dois estudantes de teste e antes do `var wg sync.WaitGroup`:

```go
// ANTES:
		codes[i] = codigoEstudante(decodeMap(t, w.Body.Bytes()))
	}
	var wg sync.WaitGroup

// DEPOIS:
		codes[i] = codigoEstudante(decodeMap(t, w.Body.Bytes()))
	}
	if err := projections.NewEstudanteProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild estudantes: %v", err)
	}
	var wg sync.WaitGroup
```

Em `TestTurmaVinculo11AdicionarEstudanteATurmaRotaManualPreservaStatusERegraDuplicidade`, logo
depois de extrair `cod` da resposta de cadastro, e novamente entre a primeira chamada bem-sucedida
(`call(fx.turmaAtiva.CodigoTurma)`) e a checagem de duplicidade (a checagem de duplicidade lê a
projeção `turmas` para saber a quais turmas o estudante já pertence, então precisa ver o vínculo
da primeira chamada):

```go
// ANTES:
	cod := codigoEstudante(decodeMap(t, w.Body.Bytes()))
	call := func(turma string) *httptest.ResponseRecorder {
		...
	}
	if rr := call(fx.turmaAtiva.CodigoTurma); rr.Code != 200 {
		t.Fatalf("ativo: %d %s", rr.Code, rr.Body.String())
	}
	if rr := call("TURMA_INEXISTENTE"); rr.Code != 404 {

// DEPOIS:
	cod := codigoEstudante(decodeMap(t, w.Body.Bytes()))
	if err := projections.NewEstudanteProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild estudantes: %v", err)
	}
	call := func(turma string) *httptest.ResponseRecorder {
		...
	}
	if rr := call(fx.turmaAtiva.CodigoTurma); rr.Code != 200 {
		t.Fatalf("ativo: %d %s", rr.Code, rr.Body.String())
	}
	if err := projections.NewTurmasProjection(fx.client).Rebuild(); err != nil {
		t.Fatalf("rebuild turmas: %v", err)
	}
	if rr := call("TURMA_INEXISTENTE"); rr.Code != 404 {
```

(mantenha o corpo da função `call := func(turma string) ...` exatamente como está — só adicione
as duas chamadas de `Rebuild()` nos pontos indicados.)

### 1.10 — Melhoria recomendada (não bloqueante): `TestTurmaVinculo10` não confere as respostas HTTP das goroutines nem implementa o fallback de retry descrito na Tarefa 43 original

O teste dispara as duas requisições concorrentes e descarta a resposta de cada uma
(`fx.router.ServeHTTP(w, req)` sem checar `w.Code`), confiando só no estado final da projeção
`turmas`. Isso já é suficiente para pegar regressões funcionais (o teste falha corretamente se
os vínculos se perderem), mas não implementa o fallback específico pedido no documento original
da Tarefa 43 ("se colidir duas vezes seguidas e falhar, aceite desde que o erro seja limpo, e
repita a chamada de forma síncrona para confirmar que teria sucesso"). Isto **não bloqueia** a
conclusão desta tarefa — é uma melhoria de robustez de teste que pode ficar para depois — mas
registre no corpo do PR que essa parte específica do escopo original não foi implementada.

---

## Seção 2 — `cmd/server/notas_faltas_correcao_integration_test.go`

Aplique a Seção 0 (produção) antes desta seção — vários destes testes dependem dela para passar.
Depois de tudo, 13 dos 15 testes passam (os 2 restantes estão na Seção 3).

### 2.1 — `genero` passado como `"M"`/`"F"` em vez de `"masculino"`/`"feminino"`

**Diagnóstico**: `Estudante.CriarComVinculo`/`CriarComVinculoComDocumentosOpcionais` validam
`genero` contra exatamente `"masculino"` ou `"feminino"`
(`internal/domain/aggregates/estudante.go:466`,
`internal/domain/aggregates/solicitacao_matricula.go:120`). O fixture `setupRegistrosCorrecaoIntegration`
— usado tanto pelos 2 testes originais da Tarefa 38 quanto pelos 13 novos da Tarefa 43 — sempre
usou `"M"`/`"F"`, o que faz a criação do estudante falhar já no primeiro `t.Fatalf("criar
estudante: %v", err)` do setup, **impedindo absolutamente todos os 15 testes deste arquivo de
sequer começar**. Isso nunca foi percebido porque, como a própria Codex reportou, nenhum destes
testes tinha sido rodado contra Postgres real antes desta auditoria.

### 2.2 — `telefone_encarregado`/`bilhete_identidade_encarregado` do estudante ausentes

**Diagnóstico**: depois de corrigir 2.1, o próximo erro é
`"telefone_encarregado é obrigatório para estudante escolar"` e depois
`"bi_encarregado é obrigatório quando bilhete_identidade_encarregado é informado"` — a validação
de documentos de matrícula (`ValidarDocumentosMatricula`,
`internal/domain/aggregates/solicitacao_matricula.go`) roda dentro de `CriarComVinculo`
(`exigirDocumentosEscolares=true`). Como este fixture não está testando fluxo de matrícula/
documentos, é mais simples e mais robusto trocar para
`CriarComVinculoComDocumentosOpcionais` (que pula essa validação — `exigirDocumentosEscolares=false`),
igual a outros fixtures deste repositório, fornecendo apenas telefone/BI do encarregado (que
continuam sendo exigidos por `ValidarTelefonesMatricula`/`ValidarBilhetesMatricula`,
que rodam incondicionalmente).

### 2.3 — Matéria/curso "superior" inseridos via SQL bruto direto em `projection_cursos`/`projection_materias`, com nome de coluna errado e sem sobreviver a rebuilds

**Diagnóstico, parte 1 (coluna errada)**: o `INSERT` bruto usava a coluna `nivel` em
`projection_cursos`. Essa coluna foi renomeada para `anos_academicos` na migration
`011_cursos_nivel_to_anos_academicos.sql`. O erro real é
`pq: column "nivel" of relation "projection_cursos" does not exist`.

**Diagnóstico, parte 2 (não sobrevive a rebuilds — mais grave)**: mesmo corrigindo o nome da
coluna, inserir `curso`/`matéria` via SQL bruto os torna **não rastreáveis pelo ledger**.
`CursosProjection.Rebuild()` (`internal/projections/cursos_projection.go:76`) faz
`TRUNCATE TABLE projection_cursos CASCADE` — que, por causa das foreign keys
(`projection_materias.curso_id`, `projection_estudantes.curso_medio_id`/`curso_superior_id`,
todas apontando de volta a `projection_cursos`/`projection_materias`), **arrasta em cascata**
qualquer linha ligada a elas, inclusive as de `projection_estudantes`. Como toda linha inserida
via SQL bruto não tem evento correspondente no ledger, ela nunca é recriada por nenhum rebuild —
ela simplesmente desaparece. Comprovei isso com uma reprodução mínima (2 testes rodando em
sequência no mesmo processo/banco): o segundo teste, ao rodar seu próprio
`academiaProjection.Rebuild()` de rotina, apaga silenciosamente o estudante e a matéria do
**primeiro** teste, quebrando o rebuild de `notas` do primeiro com uma violação de foreign key.

A correção correta é criar `curso` e `matéria` pelos próprios agregados (`Curso.Criar`,
`MateriaDisciplinar.Criar` + `DefinirPeriodo`), gravando no ledger como tudo mais neste sistema, e
registrar as projeções `cursos`/`materias` no `projManager` deste fixture (que hoje só registra
`notas`/`faltas`).

**Diagnóstico, parte 3 (ordem de rebuild)**: depois de mover para agregados, é essencial
reconstruir `cursos`/`materias` **antes** de `estudantes` — a ordem documentada em
`internal/projections/manager.go`, variável `defaultRebuildOrder`, é exatamente
`..., academias, cursos, materias, categorias_nota, estudantes, turmas, ...`. O motivo é o mesmo
`TRUNCATE ... CASCADE`: se `estudantes` for reconstruída primeiro e só depois `cursos`/`materias`
forem reconstruídas, o `TRUNCATE CASCADE` de `cursos` apaga os estudantes que acabaram de ser
projetados.

Aplique o bloco completo abaixo, que substitui a parte do `setupRegistrosCorrecaoIntegration`
que vai desde o registro das projeções no `projManager` até a criação dos dois estudantes de
teste (mantenha tudo antes e depois deste trecho como está):

```go
// ANTES:
	projManager = projections.NewManager(client)
	projManager.RegisterProjection("notas", projections.NewNotasProjection(client))
	projManager.RegisterProjection("faltas", projections.NewFaltasProjection(client))
	t.Cleanup(func() {
		dbClient, repository, projManager = oldDBClient, oldRepository, oldProjManager
	})

	sequence := time.Now().UnixNano()
	academia := criarAcademiaCorrecao(t, repository, sequence, "A")
	outraAcademia := criarAcademiaCorrecao(t, repository, sequence, "B")
	academiaProjection := projections.NewAcademiaProjection(client)
	if err := academiaProjection.Rebuild(); err != nil {
		t.Fatalf("rebuild academias: %v", err)
	}

	ano := "1_ano_fundamental"
	codigoAluno := fmt.Sprintf("%07d", sequence%10_000_000)
	estudante := aggregates.NewEstudante()
	if err := estudante.CriarComVinculo("Aluno de integração", codigoAluno, "hash", nil, nil, nil, nil, nil, "M", time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC), &ano, nil, nil, nil, nil, &academia.ID, academia.CodigoAcademia); err != nil {
		t.Fatalf("criar estudante: %v", err)
	}
	if err := repository.SaveWithAudit(estudante, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar estudante: %v", err)
	}
	anoSuperior := "1_ano_superior"
	codigoAlunoSuperior := fmt.Sprintf("%07d", (sequence+1)%10_000_000)
	estudanteSuperior := aggregates.NewEstudante()
	if err := estudanteSuperior.CriarComVinculo("Aluno superior de integração", codigoAlunoSuperior, "hash", nil, nil, nil, nil, nil, "F", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), nil, nil, &anoSuperior, nil, nil, &academia.ID, academia.CodigoAcademia); err != nil {
		t.Fatalf("criar estudante superior: %v", err)
	}
	if err := repository.SaveWithAudit(estudanteSuperior, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar estudante superior: %v", err)
	}

	cursoSuperiorID := uuid.New()
	if _, err := client.DB().Exec(`
		INSERT INTO projection_cursos (id, nome, type, nivel, periodos, codigo_academia, status, created_at)
		VALUES ($1, 'Curso Superior integração', 'superior', '[]'::jsonb, '["1_semestre","2_semestre"]'::jsonb, $2, 'ativo', CURRENT_TIMESTAMP)
	`, cursoSuperiorID, academia.CodigoAcademia); err != nil {
		t.Fatalf("inserir curso superior: %v", err)
	}
	materiaSuperiorID := uuid.New()
	if _, err := client.DB().Exec(`
		INSERT INTO projection_materias (id, nome, type, codigo_academia, curso_id, periodo, anos_academicos, status, created_at)
		VALUES ($1, 'Cálculo I integração', 'superior', $2, $3, '1_semestre', '["1_ano_superior"]'::jsonb, 'ativo', CURRENT_TIMESTAMP)
	`, materiaSuperiorID, academia.CodigoAcademia, cursoSuperiorID); err != nil {
		t.Fatalf("inserir materia superior: %v", err)
	}

	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild estudantes: %v", err)
	}

	materiaID := uuid.New()
	if _, err := client.DB().Exec(`
		INSERT INTO projection_materias (id, nome, type, codigo_academia, anos_academicos, status, created_at)
		VALUES ($1, 'Matemática integração', 'fundamental', $2, '["1_ano_fundamental"]'::jsonb, 'ativo', CURRENT_TIMESTAMP)
	`, materiaID, academia.CodigoAcademia); err != nil {
		t.Fatalf("inserir matéria: %v", err)
	}

// DEPOIS:
	projManager = projections.NewManager(client)
	projManager.RegisterProjection("notas", projections.NewNotasProjection(client))
	projManager.RegisterProjection("faltas", projections.NewFaltasProjection(client))
	projManager.RegisterProjection("cursos", projections.NewCursosProjection(client))
	projManager.RegisterProjection("materias", projections.NewMateriasProjection(client))
	t.Cleanup(func() {
		dbClient, repository, projManager = oldDBClient, oldRepository, oldProjManager
	})

	sequence := time.Now().UnixNano()
	academia := criarAcademiaCorrecao(t, repository, sequence, "A")
	outraAcademia := criarAcademiaCorrecao(t, repository, sequence, "B")
	if err := academia.DefinirAnoLetivo("2025_2026", "escolar", academia.ID); err != nil {
		t.Fatalf("definir ano letivo academia: %v", err)
	}
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar ano letivo academia: %v", err)
	}
	if err := outraAcademia.DefinirAnoLetivo("2025_2026", "escolar", outraAcademia.ID); err != nil {
		t.Fatalf("definir ano letivo outra academia: %v", err)
	}
	if err := repository.SaveWithAudit(outraAcademia, db.AuditContext{UserID: outraAcademia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar ano letivo outra academia: %v", err)
	}
	academiaProjection := projections.NewAcademiaProjection(client)
	if err := academiaProjection.Rebuild(); err != nil {
		t.Fatalf("rebuild academias: %v", err)
	}

	ano := "1_ano_fundamental"
	anoSuperior := "1_ano_superior"

	cursoSuperior := aggregates.NewCurso()
	if err := cursoSuperior.Criar("Curso Superior integração", "superior", "", []string{"1_ano_superior"}, []string{"1_semestre", "2_semestre"}, academia.CodigoAcademia); err != nil {
		t.Fatalf("criar curso superior: %v", err)
	}
	if err := repository.SaveWithAudit(cursoSuperior, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar curso superior: %v", err)
	}
	cursoSuperiorID := cursoSuperior.ID
	materiaSuperior := aggregates.NewMateriaDisciplinar()
	if err := materiaSuperior.Criar("Cálculo I integração", "superior", []string{"1_ano_superior"}, academia.CodigoAcademia, &cursoSuperiorID, nil, nil, academia.ID); err != nil {
		t.Fatalf("criar materia superior: %v", err)
	}
	if err := materiaSuperior.DefinirPeriodo("1_semestre", academia.ID); err != nil {
		t.Fatalf("definir periodo materia superior: %v", err)
	}
	if err := repository.SaveWithAudit(materiaSuperior, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar materia superior: %v", err)
	}
	materiaSuperiorID := materiaSuperior.ID

	materia := aggregates.NewMateriaDisciplinar()
	if err := materia.Criar("Matemática integração", "fundamental", []string{"1_ano_fundamental"}, academia.CodigoAcademia, nil, nil, nil, academia.ID); err != nil {
		t.Fatalf("criar materia: %v", err)
	}
	if err := repository.SaveWithAudit(materia, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar materia: %v", err)
	}
	materiaID := materia.ID

	// IMPORTANTE: cursos e materias devem ser reconstruídos ANTES de
	// estudantes, replicando defaultRebuildOrder em manager.go. Ambas as
	// projeções fazem TRUNCATE ... CASCADE nas suas tabelas, o que arrasta
	// projection_estudantes (FK curso_medio_id/curso_superior_id) — se
	// chamadas depois do rebuild de estudantes, apagam silenciosamente os
	// estudantes já projetados.
	if err := projections.NewCursosProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild cursos: %v", err)
	}
	if err := projections.NewMateriasProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild materias: %v", err)
	}

	codigoAluno := fmt.Sprintf("%07d", sequence%10_000_000)
	telefoneEncarregado := fmt.Sprintf("9%08d", sequence%100_000_000)
	biEncarregado := fmt.Sprintf("BI%09d", sequence%1_000_000_000)
	estudante := aggregates.NewEstudante()
	if err := estudante.CriarComVinculoComDocumentosOpcionais("Aluno de integração", codigoAluno, "hash", nil, nil, &telefoneEncarregado, nil, &biEncarregado, "masculino", time.Date(2014, 1, 1, 0, 0, 0, 0, time.UTC), &ano, nil, nil, nil, nil, &academia.ID, academia.CodigoAcademia); err != nil {
		t.Fatalf("criar estudante: %v", err)
	}
	if err := repository.SaveWithAudit(estudante, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar estudante: %v", err)
	}
	codigoAlunoSuperior := fmt.Sprintf("%07d", (sequence+1)%10_000_000)
	telefoneSuperior := fmt.Sprintf("9%08d", (sequence+1)%100_000_000)
	biSuperior := fmt.Sprintf("BI%09d", (sequence+2)%1_000_000_000)
	estudanteSuperior := aggregates.NewEstudante()
	if err := estudanteSuperior.CriarComVinculoComDocumentosOpcionais("Aluno superior de integração", codigoAlunoSuperior, "hash", nil, &telefoneSuperior, nil, &biSuperior, nil, "feminino", time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), nil, nil, &anoSuperior, nil, nil, &academia.ID, academia.CodigoAcademia); err != nil {
		t.Fatalf("criar estudante superior: %v", err)
	}
	if err := repository.SaveWithAudit(estudanteSuperior, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: "127.0.0.1"}); err != nil {
		t.Fatalf("salvar estudante superior: %v", err)
	}

	if err := projections.NewEstudanteProjection(client).Rebuild(); err != nil {
		t.Fatalf("rebuild estudantes: %v", err)
	}
```

Depois deste bloco, o código original continua com
`if err := estudante.RegistrarNota(...)` — mantenha exatamente como está (não duplique o rebuild
de estudantes que já existia ali; ele foi movido para dentro do bloco acima).

**Atenção**: depois desta mudança, `uuid.New()` deixa de ser necessário para `cursoSuperiorID`/
`materiaSuperiorID`/`materiaID` neste trecho (eles vêm de `.ID` dos agregados), mas o import
`github.com/google/uuid` continua sendo usado em outras partes do arquivo (`uuid.NewString()` em
`TestFaltasPeriodo12`, `var notaID uuid.UUID`, etc.) — não remova o import.

### 2.4 — `TestFaltasPeriodo10RebuildPreservaPeriodo`: `fx.faltaID` fica obsoleto depois de um segundo `Rebuild()`

**Diagnóstico**: depois da correção 0.2 (IDs determinísticos), este problema específico
desaparece — mas só se 0.2 for aplicada. Documentando por completude e porque a asserção antiga
tinha outro problema: ela fazia `dto, err := ...GetByID(fx.faltaID); if err != nil ||
dto.Periodo != ...` — se `GetByID` retornar `(nil, nil)` (sem erro, mas sem linha encontrada), a
segunda parte da condição faz dereference de `dto` nil e gera um **panic** que derruba todo o
processo de teste (não é só uma falha do teste, é um crash do binário `go test` inteiro — foi
assim que descobri isto, o restante da suíte parou de rodar). Ajustado para buscar pela chave
natural (`codigo_estudante`) em vez de depender de um id capturado antes do rebuild:

```go
// ANTES:
func TestFaltasPeriodo10RebuildPreservaPeriodo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	if err := projections.NewFaltasProjection(fx.client).Rebuild(); err != nil {
		t.Fatal(err)
	}
	dto, err := projections.NewFaltasProjection(fx.client).GetByID(fx.faltaID)
	if err != nil || dto.Periodo != "1_trimestre" {
		t.Fatalf("periodo=%q err=%v", dto.Periodo, err)
	}
}

// DEPOIS:
func TestFaltasPeriodo10RebuildPreservaPeriodo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fx := setupRegistrosCorrecaoIntegration(t)
	if err := projections.NewFaltasProjection(fx.client).Rebuild(); err != nil {
		t.Fatal(err)
	}
	// O id de projection_faltas é gerado por DEFAULT no INSERT (não é
	// determinístico a partir do evento), então fx.faltaID fica obsoleto
	// após este segundo Rebuild(). Buscamos pela chave natural (codigo do
	// estudante) para confirmar que o período sobrevive à reconstrução.
	faltas, err := projections.NewFaltasProjection(fx.client).GetByEstudante(fx.codigoAluno)
	if err != nil {
		t.Fatal(err)
	}
	var achou bool
	for _, f := range faltas {
		if f.Periodo == "1_trimestre" {
			achou = true
			break
		}
	}
	if !achou {
		t.Fatalf("periodo '1_trimestre' não sobreviveu ao rebuild: %+v", faltas)
	}
}
```

### 2.5 — `TestFaltasPeriodo05`: asserção conferia o texto de log interno, não a resposta HTTP real

**Diagnóstico**: o handler loga a mensagem detalhada (`periodo '2_semestre' invalido para a
materia 'Cálculo I integração'. Periodo definido: '1_semestre'`) mas devolve ao cliente uma
mensagem pública genérica (`{"error":"VALIDATION_ERROR","message":"Período inválido",...}`). O
teste conferia a mensagem detalhada (só existe no log do servidor) contra o corpo da resposta
HTTP, o que nunca bate.

```go
// ANTES (dentro de TestFaltasPeriodo05SuperiorComPeriodoDiferenteDaMateriaRetorna400):
	w := registrarFaltaCorrecao(t, fx, fx.codigoAlunoSuperior, "2_semestre", fx.materiaSuperiorID, "2026-02-14")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "periodo '2_semestre' invalido") {

// DEPOIS:
	w := registrarFaltaCorrecao(t, fx, fx.codigoAlunoSuperior, "2_semestre", fx.materiaSuperiorID, "2026-02-14")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Período inválido") {
```

(o resto da função continua igual.)

### 2.6 — `TestFaltasPeriodo12`: strings literais `'x'`/`'y'` inseridas numa coluna `UUID`

**Diagnóstico**: `event_id` é `UUID` no schema; o teste inseria os literais `'x'`/`'y'`
diretamente, causando `pq: invalid input syntax for type uuid: "x"`.

```go
// ANTES (dentro de TestFaltasPeriodo12BackfillMigracaoPreservaHistoricaSemPeriodo):
	id1, id2 := uuid.NewString(), uuid.NewString()
	_, err := fx.client.DB().Exec(`INSERT INTO projection_faltas (id,codigo_estudante,codigo_academia,ano_lectivo,ano_academico,periodo,data,materia_disciplinar_id,quantidade,registered_at,event_id,version) VALUES ($1,$2,$3,'2026','1_ano_superior',NULL,'2026-02-18',$4,1,CURRENT_TIMESTAMP,'x',1),($5,$2,$3,'2026','1_ano_fundamental',NULL,'2026-02-19',$6,1,CURRENT_TIMESTAMP,'y',1)`, id1, fx.codigoAlunoSuperior, fx.academia.CodigoAcademia, fx.materiaSuperiorID.String(), id2, fx.materiaID.String())

// DEPOIS:
	id1, id2 := uuid.NewString(), uuid.NewString()
	eventID1, eventID2 := uuid.NewString(), uuid.NewString()
	_, err := fx.client.DB().Exec(`INSERT INTO projection_faltas (id,codigo_estudante,codigo_academia,ano_lectivo,ano_academico,periodo,data,materia_disciplinar_id,quantidade,registered_at,event_id,version) VALUES ($1,$2,$3,'2026','1_ano_superior',NULL,'2026-02-18',$4,1,CURRENT_TIMESTAMP,$7,1),($5,$2,$3,'2026','1_ano_fundamental',NULL,'2026-02-19',$6,1,CURRENT_TIMESTAMP,$8,1)`, id1, fx.codigoAlunoSuperior, fx.academia.CodigoAcademia, fx.materiaSuperiorID.String(), id2, fx.materiaID.String(), eventID1, eventID2)
```

(o resto da função continua igual.)

### 2.7 — `assertRespostaContemAuditoriaCorrecao`: tipo de decodificação incompatível com a resposta real da API

**Diagnóstico**: a função decodificava o corpo inteiro como `map[string][]map[string]any`,
assumindo que TODOS os campos do JSON de nível superior são arrays. Mas a resposta real de
`GET /notas` e `GET /faltas` é um envelope com campos numéricos também
(`{"limit":50,"notas":[...],"offset":0,"total":1,"total_geral":1}`), então o decode falhava com
`"cannot unmarshal number into Go value of type []map[string]interface {}"` assim que tentava
decodificar `"limit":50` como se fosse um array.

```go
// ANTES:
func assertRespostaContemAuditoriaCorrecao(t *testing.T, body []byte, campo, academiaID string) {
	t.Helper()
	var response map[string][]map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decodificar resposta de %s: %v; body=%s", campo, err, body)
	}
	for _, registro := range response[campo] {
		if registro["corrigido_por"] == academiaID && registro["registrado_por"] == academiaID && registro["motivo_correcao"] != nil {
			return
		}
	}
	t.Fatalf("resposta de %s não expôs campos de auditoria da correção: %s", campo, body)
}

// DEPOIS:
func assertRespostaContemAuditoriaCorrecao(t *testing.T, body []byte, campo, academiaID string) {
	t.Helper()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decodificar resposta de %s: %v; body=%s", campo, err, body)
	}
	var registros []map[string]any
	if err := json.Unmarshal(envelope[campo], &registros); err != nil {
		t.Fatalf("decodificar campo %s da resposta: %v; body=%s", campo, err, body)
	}
	for _, registro := range registros {
		if registro["corrigido_por"] == academiaID && registro["registrado_por"] == academiaID && registro["motivo_correcao"] != nil {
			return
		}
	}
	t.Fatalf("resposta de %s não expôs campos de auditoria da correção: %s", campo, body)
}
```

---

## Seção 3 — Itens que exigem investigação (NÃO aplicar um fix sem entender primeiro)

Estes dois testes continuam falhando depois de tudo acima, por motivos que envolvem uma decisão
de design/comportamento, não um bug óbvio de teste. **Não invente uma correção só para fazer o
teste passar** — investigue qual dos dois lados (teste ou funcionalidade) está errado e
documente a conclusão antes de mexer em código de produção.

### 3.1 — `TestFaltasPeriodo15HistoricaSemPeriodoListavelECorrigivel`

O teste força `periodo=NULL` diretamente na projeção (`UPDATE projection_faltas SET
periodo=NULL WHERE id=$1`) para simular uma falta "histórica" (registrada antes da Tarefa 34,
quando o campo `periodo` não existia), depois tenta corrigi-la via PATCH. A correção falha com
`"falta original não encontrada para correção"`.

Causa: `Estudante.CorrigirFalta` (`internal/domain/aggregates/estudante_falta.go:122`) não
consulta a projeção — ele mantém um mapa em memória (`FaltasRegistradasPorChave`), reconstruído a
partir dos **eventos do próprio agregado** no ledger. O evento `FaltasRegistradas` original desta
falta específica (criada por `estudante.RegistrarFalta(..., "1_trimestre", ...)` no próprio setup
do fixture) **já tem** `Periodo="1_trimestre"` gravado no ledger — só a projeção foi
artificialmente zerada pelo `UPDATE` do teste. Isso é o oposto do cenário real que a Tarefa 34
precisava tratar (ledger antigo sem período, projeção nova com período retroativo via migration);
aqui é ledger moderno com período, projeção artificialmente sem período. O fallback de
compatibilidade que já existe no código (linhas 137-147 do mesmo arquivo, comentado
explicitamente para esse propósito) só cobre a direção antiga, não esta.

**O que investigar**: decida entre (a) reescrever o setup deste teste para simular uma falta
histórica de verdade — ou seja, um evento `FaltasRegistradas` no ledger que **realmente não
tenha** período (não é possível hoje via `estudante.RegistrarFalta`, que sempre recebe um
`periodo` nos testes atuais; verifique se existe algum caminho no agregado que permita isso, ou
se será preciso inserir o evento manualmente no ledger com um payload sem o campo `Periodo`,
imitando um evento pré-Tarefa-34 de verdade) — ou (b) se o requisito real é que uma correção deva
funcionar mesmo quando só a *projeção* está sem período (por exemplo, depois de uma falha de
migration parcial), então `CorrigirFalta` precisa de um fallback adicional para esse caso, e isso
é uma mudança de comportamento de produção que merece atenção própria do Fredy antes de
implementar.

### 3.2 — `TestHTTPIntegrationCorrigirNotaRecalculaAvaliacaoFinal`

O teste registra 5 notas, corrige uma delas via PATCH, e espera encontrar um novo evento
`AvaliacaoFinalEscolar` no ledger (`spuri_ledger WHERE aggregate_id=... AND
event_type='AvaliacaoFinalEscolar'`) com a nota final recalculada (`22/3 ≈ 7.3333`). O resultado
real é `0.0000` (ou seja, ou nenhum evento novo é encontrado e a query pega um resultado vazio/
zerado, ou o evento encontrado não reflete a correção).

Investigação já feita: nem `Estudante.RegistrarNota`
(`internal/domain/aggregates/estudante_notas.go`) nem `Estudante.CorrigirNota` (mesmo arquivo)
fazem qualquer referência a `AvaliacaoFinal` — confirmado com
`grep -n "AvaliacaoFinal" internal/domain/aggregates/estudante_notas.go`, zero resultados. O
único lugar que levanta o evento `AvaliacaoFinalEscolar` é uma função em
`internal/domain/aggregates/estudante_avaliacao.go` que tem uma guarda de idempotência
explícita (`"avaliação final já registrada para o nível '%s' no ano letivo '%s'"`), sugerindo que
essa é uma ação deliberada e única (provavelmente disparada por uma rota separada de
"finalizar ano letivo" ou "avaliar estudante"), não algo que acontece automaticamente a cada nota
registrada ou corrigida.

**O que investigar**: (a) existe uma rota/handler separado que dispara essa avaliação final
(procure por chamadas ao método que levanta `AvaliacaoFinalEscolar` em
`internal/handlers/*.go`) — se sim, o teste provavelmente está incompleto: falta chamar essa
rota explicitamente depois de registrar as 5 notas e de novo depois da correção, comparando os
dois resultados; ou (b) recalcular a avaliação final automaticamente ao corrigir uma nota é uma
funcionalidade que deveria existir mas nunca foi implementada — nesse caso é um gap de produto
que talvez mereça sua própria tarefa, não um encaixe apressado aqui. De qualquer forma, isto não
é um bug óbvio de string ou de fixture como os outros — é preciso entender o fluxo de "avaliação
final" (rotas em `internal/handlers/avaliacao_final_regras.go` e afins) antes de decidir.

---

## Escopo obrigatório

1. Aplicar a Seção 0 (0.1 e 0.2) — os dois bugs de produção.
2. Aplicar a Seção 1 completa em `cmd/server/turma_vinculo_estudante_integration_test.go`.
3. Aplicar a Seção 2 completa (2.1 a 2.7) em `cmd/server/notas_faltas_correcao_integration_test.go`.
4. Investigar (não corrigir às cegas) os dois itens da Seção 3 e documentar a conclusão no PR —
   se a investigação apontar para uma correção clara e pequena, aplicá-la; se apontar para uma
   decisão de produto/arquitetura maior, deixar os dois testes com `t.Skip("ver Tarefa 44, Seção
   3.x — requer decisão de design, ver <link do PR/issue>")` e abrir a discussão com o Fredy em
   vez de inventar um comportamento nas costas dele.

## Fora do escopo

- Implementar o fallback de retry síncrono completo descrito no item 1.10 (melhoria de robustez
  de teste, não bloqueante).
- Qualquer mudança em `internal/storage/` além de já usar o fallback local existente via
  `ENV=test` (não mude a lógica de `storage.NewStorageProvider()` nem `useLocalMegaFallback()`).
- Qualquer migração de dado em produção além da query de verificação somente-leitura da
  Seção 0.1. Se essa query retornar algo diferente de `0`, pare e avise o Fredy antes de
  prosseguir — não decida sozinho como migrar dado de correção pré-existente.
- Implementar a funcionalidade de "recalcular avaliação final ao corrigir nota" — a Seção 3.2
  pede investigação, não implementação, a menos que a investigação mostre que é um fix trivial
  de uma linha (por exemplo, o teste só estar chamando a rota errada).

## Testes obrigatórios

Rodar tudo isto contra um PostgreSQL real e isolado (recrie o banco entre execuções se for
reusar o mesmo container, para não herdar estado de execuções anteriores):

```bash
go build ./...
go vet ./...

export DB_HOST=<host> DB_PORT=<port> DB_USER=<user> DB_PASSWORD=<senha> DB_NAME=<banco_isolado>
export SPURI_RUN_DB_INTEGRITY_TESTS=1

go test ./cmd/server/... -run 'TestTurmaVinculo' -v
go test ./cmd/server/... -run 'TestTurmaVinculo' -count=3 -v   # checar flakiness na concorrência (teste 10)
go test ./cmd/server/... -run 'TestFaltasPeriodo|TestNotasFaltas|TestHTTPIntegrationCorrigir' -v

go test ./...   # suíte completa, garantir que nada mais quebrou
```

Cole a saída completa (não só o resumo `--- PASS`/`--- FAIL`) no PR/relato final. Se qualquer
teste falhar, **não marque esta tarefa como concluída nem mova o arquivo para
`docs/Tarefas feitas/`**.

## Critérios de aceitação

- [ ] `go build ./...` e `go vet ./...` sem erros.
- [ ] Query de verificação da Seção 0.1 rodada em produção e resultado documentado no PR antes
      de aplicar a Seção 0.2.
- [ ] `internal/db/safe_queries.go`: `NotaCorrigida` e `FaltaCorrigida` na whitelist.
- [ ] `internal/projections/notas_projection.go` e `faltas_projection.go`: IDs determinísticos
      via `uuid.NewSHA1`, confirmado rodando a suíte de notas/faltas em sequência (não isolada)
      sem erros de "não encontrada" em rebuilds subsequentes.
- [ ] `TestTurmaVinculo01` a `11`: 11/11 passam, inclusive com `-count=3`.
- [ ] `TestFaltasPeriodo02` a `13` (exceto `15`) e `TestNotasFaltas*`: todos passam.
- [ ] `TestHTTPIntegrationCorrigirNotaEFalta`: passa.
- [ ] `TestFaltasPeriodo15` e `TestHTTPIntegrationCorrigirNotaRecalculaAvaliacaoFinal`: ou
      corrigidos com a causa raiz entendida e documentada, ou deixados com `t.Skip` explicando o
      motivo e apontando para uma decisão pendente do Fredy — nunca "ajustados" só para
      silenciar a falha sem entender o porquê.
- [ ] `go test ./...` completo sem novas regressões em outras suítes.
- [ ] Nenhuma mudança fora do escopo listado acima.

## Verificações manuais

- Depois de aplicar a Seção 0, teste manualmente (ou via Postman/curl) uma correção real de nota
  num ambiente de staging/dev antes de ir para produção, confirmando que a resposta é 200 e que
  um rebuild subsequente da projeção `notas` (`RebuildProjection("notas")` ou equivalente) não
  quebra.
- Confirme que o `defaultRebuildOrder` em `internal/projections/manager.go` continua consistente
  com a nova dependência `cursos`/`materias` → `estudantes` (ele já está correto; só not mexer
  nele).

## Risco e mitigação

- **Risco alto, mitigado**: a Seção 0.2 muda como IDs são gerados para linhas novas de
  `projection_notas`/`projection_faltas`. Isso não afeta linhas já existentes (elas mantêm seus
  IDs aleatórios antigos) até a próxima vez que a projeção for reconstruída do zero — nesse
  momento, TODAS as linhas (novas e antigas) passam a ter IDs determinísticos. Como confirmado
  na Seção 0.1, não há nenhum `NotaCorrigida`/`FaltaCorrigida` gravado em produção até hoje, então
  não há referência antiga que quebre. Se essa premissa mudar (a query da Seção 0.1 retornar
  algo diferente de zero), pare e reavalie antes de prosseguir.
- **Risco médio**: mover a criação de curso/matéria de SQL bruto para agregados (Seção 2.3) muda
  o número de eventos no ledger de teste e pode interagir com outros testes que dependem de
  contagem exata de eventos processados nos logs `[DEBUG] Rebuild concluído: N eventos
  processados` — isso é só log, não deve quebrar nenhuma asserção, mas vale conferir se algum
  teste (fora do escopo desta tarefa) faz assert sobre esse número.
- **Risco baixo**: todas as mudanças da Seção 1 e 2 (exceto 0.1/0.2) são só em arquivos
  `_test.go`, sem nenhum impacto em produção.
