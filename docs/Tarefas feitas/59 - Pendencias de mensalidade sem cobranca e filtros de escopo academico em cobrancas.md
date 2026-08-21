---
criado: 2026-08-21 00:00
origem: Pedido do usuário (Spuri), orquestrado por Claude (Anthropic) em sandbox com PostgreSQL 16 e Go 1.24 reais
status: feito
prioridade: alta
---

# Pendências de mensalidade sem cobrança + filtros de escopo acadêmico em cobranças (feito)

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Você (Codex) está rodando num ambiente que **bloqueia `apt` (403 Forbidden)** e **não tem Docker nem `psql`**.
Você **não consegue** subir um PostgreSQL real para validar isto do zero.

Isso já foi feito por outra IA (Claude/Anthropic), num sandbox com `apt` liberado, que:

1. Clonou o repositório, instalou PostgreSQL 16 e Go 1.24 via `apt-get` (sem Docker).
2. Rodou as 116 migrations reais (`internal/db/migrations.go`) contra esse Postgres.
3. Implementou a correção inteira descrita abaixo, **compilou** (`go build ./...`, `go vet ./...`,
   `gofmt -l .`) e **executou de fato** toda a lógica nova contra dados reais — inclusive escrevendo
   estudantes/turmas/mensalidades/cobranças de teste e inspecionando os resultados linha a linha antes de
   fixar o desenho final.
4. Escreveu 6 testes de integração novos (4 no pacote `finance`, 2 no pacote `handlers`, via HTTP real) que
   cobrem exatamente os cenários do pedido original, incluindo o caso citado pelo usuário ("estudante deve
   janeiro, nunca gerou cobrança, a academia não vê").
5. Rodou a **suíte inteira do repositório** (`go test ./...`, todos os pacotes) contra bancos recriados do
   zero, múltiplas vezes seguidas — **0 falhas, 0 regressões**, incluindo os testes já existentes que
   exercitam `ListCobrancas`/`ListCobrancasEstudante`.

**O que isso significa na prática para você:**

- **Não tente instalar Postgres, Docker ou rodar migrations.** Você não precisa disso para aplicar esta
  correção — ela já foi validada ponta a ponta.
- **Trate o desenho abaixo (diffs exatos + arquivos novos com conteúdo completo) como já corrigido e
  testado**, não como uma proposta a reavaliar. Sua tarefa é **aplicar exatamente o que está escrito aqui**,
  não redesenhar, não "melhorar", não simplificar.
- **O que você PODE e DEVE rodar no seu ambiente**: `go build ./...`, `go vet ./...`, `gofmt -l .` (deve
  vir vazio). Esses três já são suficientes para confirmar que a aplicação dos diffs foi mecânica e
  correta, porque toda a lógica nova é orientada a banco de dados (SQL) e já foi validada com Postgres real
  por Claude — o valor do seu `go build`/`go vet` aqui é garantir que você não introduziu nenhum erro de
  digitação ao copiar os blocos de código.
- **Não rode `go test ./...`** esperando que os testes novos passem — eles são testes de integração que
  exigem `RUN_POSTGRES_INTEGRATION=1` e um Postgres real, que você não tem. Eles vão dar `SKIP`
  automaticamente (ver `integrationClient`/`integrationFinanceClient` em cada pacote) — isso é esperado e
  correto, não é uma falha.
- Ao final, gere um `docs/Tarefas feitas/...md` seguindo o padrão já usado neste repositório (veja exemplos
  em `docs/Tarefas feitas/`, em especial a tarefa 47, que tem o mesmo formato de "Problema 1 / Problema 2"
  neste mesmo módulo financeiro).

---

## 1. Prompt recomendado para executar esta tarefa

Aplique exatamente os diffs e arquivos novos descritos nas seções 5 e 6 deste documento, na ordem da seção
7, sem alterar o desenho (nomes de função, assinatura, nomes de campo JSON, mensagens de erro). Depois de
aplicar, rode `gofmt -w` nos arquivos tocados, confirme `go build ./...`, `go vet ./...` e `gofmt -l .`
limpos, confirme com `git status --short` que só os arquivos listados na seção 8 foram tocados/criados, e
gere a documentação de tarefa concluída em `docs/Tarefas feitas/`, movendo este arquivo para lá com
`status: feito`.

---

## 2. Contexto — os dois problemas

### Problema 1 — pendências de mensalidade invisíveis para a academia

O módulo financeiro tem dois modelos de dados paralelos:

- **Obrigação de mensalidade** (`financeiro_mensalidade_configuracoes` + eventos de
  anulação/reativação/pagamento em `financeiro_mensalidade_obrigacoes_eventos`): é inteiramente **derivada**
  — `Service.ListMensalidades` calcula, para um estudante, todo mês entre o início da cobrança e o fim do
  ano letivo, e classifica cada mês como `pendente`, `pago` ou `anulado`, **sem que nenhuma cobrança precise
  ter sido criada**. É o que `GET /financeiro/mensalidades/estudante/:codigo` já expõe hoje corretamente —
  por isso o próprio estudante já vê a própria dívida.
- **Cobrança** (`financeiro_cobrancas`, projetada via AppyPay): só existe uma linha aqui **depois** que uma
  cobrança foi criada (ou uma tentativa de criação foi feita). É o que `GET /financeiro/cobrancas` (academia
  / admin) e `GET /financeiro/cobrancas/estudante/:codigo` expõem.

O resultado, exatamente como descrito pelo usuário: um estudante que deve janeiro mas **nunca executou o
evento que gera a cobrança** (nem tentou) não aparece em nenhuma consulta de "pagamentos" que a academia
usa — porque essas consultas são todas orientadas a `financeiro_cobrancas`, que não tem nenhuma linha para
esse mês. O estudante enxerga a própria dívida (via `/financeiro/mensalidades/estudante/:codigo`); a
academia, não.

### Problema 2 — falta de filtros de escopo acadêmico em cobranças

`ListCobrancas`/`ListCobrancasEstudante` hoje só filtram por `contexto_tipo`, `codigo_academia`, `estado` e
`tipo` (origem). Não há como pedir "só as cobranças da turma X", "só do ano letivo 2026_2027", "só do 7º ano
fundamental" ou "só do curso Y" — algo essencial para uma academia com múltiplas turmas/anos/cursos
acompanhar cobranças por recorte acadêmico.

### Por que as duas coisas viraram uma tarefa só

A saída natural para o Problema 1 (uma consulta de "pendências sem cobrança" abrangendo vários estudantes ao
mesmo tempo) exige, por segurança de performance, um **filtro de escopo obrigatório** — do contrário cada
chamada varreria a academia inteira. Os mesmos quatro filtros que resolvem o Problema 2
(`turma_id`/`curso_id`/`ano_academico`/`ano_letivo`) são reaproveitados como esse escopo obrigatório. As duas
correções compartilham a mesma consulta SQL de resolução de vínculo estudante↔turma↔ano, então foram
implementadas juntas.

---

## 3. Escopo desta tarefa (e o que NÃO fazer)

**Fazer:**

- Adicionar `Service.PendenciasSemCobranca` (multi-estudante, escopo obrigatório) e
  `Service.PendenciasSemCobrancaEstudante` (um único estudante, sem escopo obrigatório) em
  `internal/finance`.
- Adicionar os filtros `turma_id`, `curso_id`, `ano_academico`, `ano_letivo` a `ListCobrancas` e
  `ListCobrancasEstudante`.
- Expor tudo isso via os dois endpoints HTTP **já existentes** — `GET /financeiro/cobrancas` e
  `GET /financeiro/cobrancas/estudante/:codigo` — sem criar rota nova nenhuma. `cmd/server/main.go` **não
  precisa de nenhuma alteração**.
- Adicionar um novo campo `pendencias_sem_cobranca` no corpo JSON de resposta desses dois endpoints (não
  mexe no formato de `cobrancas`, é aditivo).

**NÃO fazer:**

- Não mude o formato de `CobrancaResumo` nem tente "misturar" pendências dentro do array `cobrancas` como
  itens sintéticos com `id` nulo — isso quebraria a paginação (que é feita em SQL, `LIMIT`/`OFFSET`) e
  qualquer consumidor que assuma `id` sempre presente. `pendencias_sem_cobranca` é um array **separado**,
  não paginado, no mesmo corpo de resposta.
- Não crie os filtros `turma_id`/`curso_id`/`ano_academico`/`ano_letivo` como repetíveis (array); são
  valores únicos, como `codigo_academia` já é hoje.
- Não tente fazer esses quatro filtros funcionarem para cobranças de origem `matricula` ou `avulsa` — ver
  decisão de design #3 abaixo. É intencional que usar qualquer um deles restrinja o resultado a cobranças de
  `mensalidade`.
- Não toque em `internal/finance/mensalidade.go` nem em `internal/finance/matricula.go` — toda a lógica nova
  fica isolada num arquivo novo (`internal/finance/mensalidade_pendencias.go`), reaproveitando funções já
  existentes (`ListMensalidades`) sem alterá-las.

---

## 4. Decisões de design já tomadas

**#1 — Escopo obrigatório para a consulta multi-estudante, nenhum escopo exigido para a consulta
por-estudante.** `PendenciasSemCobranca` (usada por `GET /financeiro/cobrancas`, que pode abranger toda uma
academia) exige pelo menos um entre `turma_id`, `curso_id`, `ano_academico`, `ano_letivo` — sem isso, cada
chamada varreria todos os estudantes de uma academia a cada request. `PendenciasSemCobrancaEstudante` (usada
por `GET /financeiro/cobrancas/estudante/:codigo`) já está inerentemente limitada a um único estudante, então
não exige nada extra e é **sempre** calculada nesse endpoint.

**#2 — Reaproveitar `ListMensalidades` por estudante, não reimplementar a regra de negócio em SQL batch.**
A trajetória de um mês de mensalidade (`mesInicioEfetivo`, `resolveConfiguracao` versionado por data,
`precedenciaEstado` entre eventos de anulação/reativação/pagamento) é lógica de negócio já testada
extensivamente. Reimplementá-la como uma única query SQL agregada para múltiplos estudantes seria mais
"performático" na teoria, mas introduziria risco real de divergir sutilmente da regra já correta. Como o
escopo é sempre delimitado (decisão #1), o custo de uma chamada Go por estudante do escopo é aceitável. Duas
únicas consultas em lote (não por estudante) resolvem o resto: (a) quem são os estudantes do escopo, (b)
quais (estudante, ano_letivo, mês) já têm alguma cobrança.

**#3 — Os quatro novos filtros só têm efeito sobre cobranças de origem `mensalidade`.** A tabela
`financeiro_mensalidade_cobrancas` (que já existe, escrita a cada evento de cobrança de mensalidade — ver
`upsertMensalidadeCobrancas` em `internal/projections/financeiro_projection.go`) só tem linha para cobranças
de mensalidade; matrícula e avulsa nunca escrevem lá. Isso é usado deliberadamente como a fonte da
junção — usar qualquer um dos quatro filtros naturalmente exclui matrícula/avulsa do resultado. Matrícula não
tem um vínculo direto e não-ambíguo com turma/ano_letivo no momento da cobrança (a matrícula precede a
atribuição de turma), então estender esses filtros para ela ficaria semanticamente confuso; documentado aqui
como decisão deliberada, não esquecimento.

**#4 — "já teve tentativa de cobrança" ignora o resultado da tentativa.** Uma pendência só é considerada
"sem cobrança" se **nenhuma** linha existir em `financeiro_mensalidade_cobrancas` para aquele
(estudante, ano_letivo, mês) — independente de a cobrança ter sido bem-sucedida, ter falhado ou estar
pendente. Um mês com cobrança falhada **não** aparece em `pendencias_sem_cobranca`, porque já está visível
de outra forma (na listagem normal de `cobrancas`, com o estado real dela). Isso evita duplicar a mesma
informação em dois formatos diferentes na mesma resposta.

**#5 — `pendencias_sem_cobranca` é aditivo, não substitui nada.** Retrocompatível: qualquer consumidor atual
que ignore chaves desconhecidas no JSON continua funcionando sem nenhuma mudança.

---

## 5. Diffs exatos

### 5.1 — `internal/finance/appypay.go`

**Import**: `github.com/google/uuid` já está importado neste arquivo (usado em outras funções). Não precisa
adicionar nada.

**Localizar** (a assinatura e o corpo completo de `ListCobrancas`):

```go
func (s *Service) ListCobrancas(ctx context.Context, contexto, academia string, estados, origens []string, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	where := "WHERE 1=1"
	args := []any{}
	i := 1
	if contexto != "" {
		where += fmt.Sprintf(" AND contexto_tipo=$%d", i)
		args = append(args, contexto)
		i++
	}
	if academia != "" {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, academia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estados))
		i++
	}
	if len(origens) > 0 {
		clause, err := origensClause(origens)
		if err != nil {
			return nil, err
		}
		where += clause
	}
	var total int
```

**Substituir por:**

```go
// ListCobrancas lista cobranças de um contexto/academia, com filtros
// opcionais de estado e origem. turmaID, cursoID, anoAcademico e anoLetivo
// (introduzidos na tarefa 58) restringem adicionalmente o resultado às
// cobranças de mensalidade vinculadas a esse escopo (turma, curso, ano
// acadêmico ou ano letivo) — ver chargeIDsEscopoMensalidade. Como esse
// escopo só existe para cobranças de ORIGEM mensalidade, usar qualquer um
// desses quatro filtros exclui automaticamente cobranças de matrícula e
// avulsas do resultado; isso é intencional. Quando nenhum dos quatro é
// informado, o comportamento é idêntico ao anterior à tarefa 58.
func (s *Service) ListCobrancas(ctx context.Context, contexto, academia string, estados, origens []string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	where := "WHERE 1=1"
	args := []any{}
	i := 1
	if contexto != "" {
		where += fmt.Sprintf(" AND contexto_tipo=$%d", i)
		args = append(args, contexto)
		i++
	}
	if academia != "" {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, academia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estados))
		i++
	}
	if len(origens) > 0 {
		clause, err := origensClause(origens)
		if err != nil {
			return nil, err
		}
		where += clause
	}
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		chargeIDs, err := s.chargeIDsEscopoMensalidade(ctx, academia, turmaID, cursoID, anoAcademico, anoLetivo)
		if err != nil {
			return nil, err
		}
		where += fmt.Sprintf(" AND id = ANY($%d::uuid[])", i)
		args = append(args, pq.Array(chargeIDs))
		i++
	}
	var total int
```

(o resto do corpo da função — a partir de `if err := s.client.DB().QueryRowContext(...` até o fechamento —
permanece exatamente igual, não mude mais nada nela.)

---

**Localizar** (a assinatura e o corpo completo de `ListCobrancasEstudante`):

```go
func (s *Service) ListCobrancasEstudante(ctx context.Context, codigoEstudante string, somenteAcademia *string, estados, origens []string, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	if codigoEstudante == "" {
		return nil, errors.New("código do estudante é obrigatório")
	}
	where := `WHERE (payload->>'codigo_estudante' = $1 OR payload->>'codigo_solicitacao' IN (SELECT codigo_solicitacao FROM projection_solicitacoes_matricula WHERE codigo_estudante_gerado = $1))`
	args := []any{codigoEstudante}
	i := 2
	if somenteAcademia != nil {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, *somenteAcademia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estados))
		i++
	}
	if len(origens) > 0 {
		clause, err := origensClause(origens)
		if err != nil {
			return nil, err
		}
		where += clause
	}
	var total int
```

**Substituir por:**

```go
// turmaID, cursoID, anoAcademico e anoLetivo (tarefa 58) filtram
// adicionalmente por escopo de mensalidade, igual a ListCobrancas. Como o
// escopo exige codigo_academia (ver escopoMensalidadeEstudantes), esses
// quatro filtros só têm efeito quando somenteAcademia não é nil (chamada de
// uma academia) — quando o estudante ou um admin FPP consultam sem
// restringir a academia, informar qualquer um desses quatro filtros
// devolve erro de validação, porque não há uma única academia para resolver
// o escopo de turma/curso contra o histórico do estudante.
func (s *Service) ListCobrancasEstudante(ctx context.Context, codigoEstudante string, somenteAcademia *string, estados, origens []string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	if codigoEstudante == "" {
		return nil, errors.New("código do estudante é obrigatório")
	}
	where := `WHERE (payload->>'codigo_estudante' = $1 OR payload->>'codigo_solicitacao' IN (SELECT codigo_solicitacao FROM projection_solicitacoes_matricula WHERE codigo_estudante_gerado = $1))`
	args := []any{codigoEstudante}
	i := 2
	if somenteAcademia != nil {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, *somenteAcademia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estados))
		i++
	}
	if len(origens) > 0 {
		clause, err := origensClause(origens)
		if err != nil {
			return nil, err
		}
		where += clause
	}
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		academiaEscopo := ""
		if somenteAcademia != nil {
			academiaEscopo = *somenteAcademia
		}
		chargeIDs, err := s.chargeIDsEscopoMensalidade(ctx, academiaEscopo, turmaID, cursoID, anoAcademico, anoLetivo)
		if err != nil {
			return nil, err
		}
		where += fmt.Sprintf(" AND id = ANY($%d::uuid[])", i)
		args = append(args, pq.Array(chargeIDs))
		i++
	}
	var total int
```

(novamente, o resto do corpo — a partir de `if err := s.client.DB().QueryRowContext(...` — permanece igual.)

---

### 5.2 — `internal/handlers/financeiro_handlers.go`

**Localizar** (o bloco de import):

```go
import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)
```

**Substituir por:**

```go
import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)
```

(só adiciona `"fmt"`, nada mais muda neste bloco.)

---

**Localizar** (a função `ListarCobrancasAppyPay` inteira):

```go
func ListarCobrancasAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := FinanceiroService.ListCobrancas(c.Request.Context(), contexto, academia, c.QueryArray("estado"), c.QueryArray("tipo"), limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset})
}
```

**Substituir por:**

```go
// parseOptionalUUIDQuery lê um parâmetro de query opcional como UUID. Devolve
// nil quando o parâmetro não foi informado, e erro quando foi informado mas
// não é um UUID válido.
func parseOptionalUUIDQuery(c *gin.Context, param string) (*uuid.UUID, error) {
	raw := strings.TrimSpace(c.Query(param))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%s inválido", param)
	}
	return &id, nil
}

func ListarCobrancasAppyPay(c *gin.Context) {
	contexto := c.Query("contexto_tipo")
	academia := c.Query("codigo_academia")
	if !authorizeFinanceScope(c, &contexto, &academia) {
		utils.RespondWithForbiddenError(c, "sem permissão para este contexto financeiro")
		return
	}
	turmaID, err := parseOptionalUUIDQuery(c, "turma_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	cursoID, err := parseOptionalUUIDQuery(c, "curso_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	anoAcademico := c.Query("ano_academico")
	anoLetivo := c.Query("ano_letivo")
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := FinanceiroService.ListCobrancas(c.Request.Context(), contexto, academia, c.QueryArray("estado"), c.QueryArray("tipo"), turmaID, cursoID, anoAcademico, anoLetivo, limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	body := gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset}
	// pendencias_sem_cobranca só é computado quando pelo menos um dos
	// quatro filtros de escopo (turma_id, curso_id, ano_academico,
	// ano_letivo) é informado junto de codigo_academia — sem isso, a
	// varredura seria sobre a academia inteira sem limite. Ver
	// finance.PendenciasSemCobranca.
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		pendencias, err := FinanceiroService.PendenciasSemCobranca(c.Request.Context(), academia, turmaID, cursoID, anoAcademico, anoLetivo)
		if err != nil {
			financeError(c, err)
			return
		}
		body["pendencias_sem_cobranca"] = pendencias
	}
	c.JSON(http.StatusOK, body)
}
```

---

**Localizar** (a parte final de `ConsultarCobrancasEstudante`, a partir da checagem de `limit`/`offset`):

```go
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, c.QueryArray("estado"), c.QueryArray("tipo"), limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset})
}
```

**Atenção**: este trecho é o final da função `ConsultarCobrancasEstudante` (o que vem logo depois do
`switch typ { ... }` que resolve `somenteAcademia`). Note que ele é parecido, mas **não idêntico**, ao final
de `ListarCobrancasAppyPay` tratado no bloco anterior — a chamada aqui é a `ListCobrancasEstudante(...)`, não
`ListCobrancas(...)`, então o texto exato abaixo (já conferido por Claude, caractere por caractere, contra o
arquivo original) só tem **uma** ocorrência no arquivo; não há ambiguidade na busca.

**Substituir por:**

```go
	turmaID, err := parseOptionalUUIDQuery(c, "turma_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	cursoID, err := parseOptionalUUIDQuery(c, "curso_id")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	anoAcademico := c.Query("ano_academico")
	anoLetivo := c.Query("ano_letivo")
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, c.QueryArray("estado"), c.QueryArray("tipo"), turmaID, cursoID, anoAcademico, anoLetivo, limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	body := gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset}
	// pendencias_sem_cobranca é sempre calculado aqui (sem exigir nenhum
	// filtro extra): esta consulta já está inerentemente delimitada a UM
	// estudante, então não há o mesmo risco de varredura sem limite que
	// existe em ListarCobrancasAppyPay. Ver
	// finance.PendenciasSemCobrancaEstudante.
	pendencias, err := FinanceiroService.PendenciasSemCobrancaEstudante(c.Request.Context(), codigo, somenteAcademia)
	if err != nil {
		financeError(c, err)
		return
	}
	body["pendencias_sem_cobranca"] = pendencias
	c.JSON(http.StatusOK, body)
}
```

---

### 5.3 — Testes existentes: só assinatura de chamada muda

Os arquivos `internal/finance/cobrancas_integration_test.go` e
`internal/finance/cobrancas_estudante_integration_test.go` chamam `ListCobrancas`/`ListCobrancasEstudante`
várias vezes com a assinatura antiga. Como esses dois métodos agora têm 4 parâmetros novos
(`turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string`) inseridos **antes** de `limit, offset`, cada
chamada precisa dos 4 novos argumentos. Em todos os casos, use `nil, nil, "", ""` (nenhum filtro de escopo
extra) — nenhum desses testes testa os filtros novos, eles testam outra coisa e só precisam continuar
compilando com o comportamento idêntico ao de antes.

Faça a seguinte substituição **mecânica** (mesmo padrão em ambos os arquivos): toda ocorrência de
`.ListCobrancas(`\<argumentos\>`, <um número>, <um número>)` vira
`.ListCobrancas(`\<argumentos\>`, nil, nil, "", "", <um número>, <um número>)` — e o mesmo para
`.ListCobrancasEstudante(`. Concretamente, em `internal/finance/cobrancas_integration_test.go`:

| Antes | Depois |
|---|---|
| `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, 50, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, nil, nil, "", "", 50, 0)` |
| `service.ListCobrancas(ctx, ContextoAcademia, academiaA, []string{"Success"}, nil, 50, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaA, []string{"Success"}, nil, nil, nil, "", "", 50, 0)` |
| `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, []string{"matricula"}, 50, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, []string{"matricula"}, nil, nil, "", "", 50, 0)` |
| `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, 1, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, nil, nil, "", "", 1, 0)` |
| `service.ListCobrancas(ctx, ContextoAcademia, academiaB, nil, nil, 50, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaB, nil, nil, nil, nil, "", "", 50, 0)` |

E em `internal/finance/cobrancas_estudante_integration_test.go`:

| Antes | Depois |
|---|---|
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, nil, 50, 0)` | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, nil, nil, nil, "", "", 50, 0)` |
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, []string{"Success"}, nil, 50, 0)` | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, []string{"Success"}, nil, nil, nil, "", "", 50, 0)` |
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"mensalidade"}, 50, 0)` | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"mensalidade"}, nil, nil, "", "", 50, 0)` |
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"matricula"}, 50, 0)` | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"matricula"}, nil, nil, "", "", 50, 0)` |
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"invalido"}, 50, 0)` (dentro de um `if _, err := ...; err == nil`) | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"invalido"}, nil, nil, "", "", 50, 0)` |
| `service.ListCobrancasEstudante(ctx, estudante, nil, nil, nil, 50, 0)` | `service.ListCobrancasEstudante(ctx, estudante, nil, nil, nil, nil, nil, "", "", 50, 0)` |
| `service.ListCobrancasEstudante(ctx, estudante, &academiaA, nil, nil, 50, 0)` | `service.ListCobrancasEstudante(ctx, estudante, &academiaA, nil, nil, nil, nil, "", "", 50, 0)` |

Não existem outros call sites destas duas funções em nenhum outro lugar do repositório (já confirmado por
`grep -rn` em todo o código-fonte, fora os dois handlers já tratados na seção 5.2).

---

## 6. Arquivos novos (conteúdo completo)

### 6.1 — `internal/finance/mensalidade_pendencias.go`

```go
package finance

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// mensalidadeEscopoVinculo é uma linha do escopo multi-estudante resolvido
// por escopoMensalidadeEstudantes: um vínculo (estudante + turma + ano
// letivo) que casa com os filtros pedidos.
type mensalidadeEscopoVinculo struct {
	TurmaID         uuid.UUID
	CodigoAcademia  string
	AnoLetivo       string
	Nivel           string
	AnoAcademico    string
	CursoID         *uuid.UUID
	CodigoEstudante string
}

// escopoMensalidadeEstudantes enumera, para uma academia, todos os vínculos
// (estudante + turma + ano_letivo) que casam com os filtros opcionais
// informados (turmaID, cursoID, anoAcademico, anoLetivo). É a versão
// multi-estudante de vinculosMensalidade: o mesmo padrão de JOIN (turma
// atual via projection_turmas.estudantes + projection_academias.ano_letivo,
// e turmas históricas via historico_estudantes_ano_letivo), mas enumerando
// TODOS os estudantes que casam, em vez de checar a presença de um só.
//
// Pelo menos um filtro é obrigatório: sem nenhum, a consulta processaria a
// academia inteira (potencialmente milhares de estudantes) a cada chamada, o
// que essa função rejeita explicitamente com um erro de validação — ver
// PendenciasSemCobranca, a única chamadora hoje.
func (s *Service) escopoMensalidadeEstudantes(ctx context.Context, academia string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string) ([]mensalidadeEscopoVinculo, error) {
	if academia == "" {
		return nil, errors.New("codigo_academia é obrigatório para consultar pendências sem cobrança")
	}
	if turmaID == nil && cursoID == nil && anoAcademico == "" && anoLetivo == "" {
		return nil, errors.New("informe ao menos um filtro (turma_id, curso_id, ano_academico ou ano_letivo) para consultar pendências sem cobrança")
	}
	args := []any{academia}
	filter := ""
	i := 2
	if turmaID != nil {
		filter += fmt.Sprintf(" AND turma_id=$%d", i)
		args = append(args, *turmaID)
		i++
	}
	if cursoID != nil {
		filter += fmt.Sprintf(" AND curso_id=$%d", i)
		args = append(args, *cursoID)
		i++
	}
	if anoAcademico != "" {
		filter += fmt.Sprintf(" AND ano_academico=$%d", i)
		args = append(args, anoAcademico)
		i++
	}
	if anoLetivo != "" {
		filter += fmt.Sprintf(" AND ano_letivo=$%d", i)
		args = append(args, anoLetivo)
		i++
	}
	q := `WITH vinculos AS (
		SELECT t.id AS turma_id, t.codigo_academia, h.key AS ano_letivo, t.nivel AS ano_academico, t.curso_id,
		       COALESCE(c.type, CASE WHEN t.nivel LIKE '%_ano_fundamental' THEN 'fundamental' END) AS nivel,
		       est.value AS codigo_estudante
		FROM projection_turmas t
		CROSS JOIN LATERAL jsonb_each(t.historico_estudantes_ano_letivo) h
		CROSS JOIN LATERAL jsonb_array_elements_text(h.value) AS est(value)
		LEFT JOIN projection_cursos c ON c.id=t.curso_id JOIN projection_academias a ON a.codigo_academia=t.codigo_academia
		WHERE a.type='private' AND t.codigo_academia=$1
		UNION
		SELECT t.id, t.codigo_academia, a.ano_letivo, t.nivel, t.curso_id,
		       COALESCE(c.type, CASE WHEN t.nivel LIKE '%_ano_fundamental' THEN 'fundamental' END),
		       est.value
		FROM projection_turmas t
		CROSS JOIN LATERAL jsonb_array_elements_text(t.estudantes) AS est(value)
		LEFT JOIN projection_cursos c ON c.id=t.curso_id JOIN projection_academias a ON a.codigo_academia=t.codigo_academia
		WHERE a.type='private' AND a.ano_letivo IS NOT NULL AND t.codigo_academia=$1
	) SELECT DISTINCT turma_id, codigo_academia, ano_letivo, nivel, ano_academico, curso_id, codigo_estudante
	  FROM vinculos WHERE nivel IS NOT NULL AND codigo_estudante <> ''` + filter
	rows, err := s.client.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []mensalidadeEscopoVinculo
	for rows.Next() {
		var v mensalidadeEscopoVinculo
		var curso any
		if err := rows.Scan(&v.TurmaID, &v.CodigoAcademia, &v.AnoLetivo, &v.Nivel, &v.AnoAcademico, &curso, &v.CodigoEstudante); err != nil {
			return nil, err
		}
		if s, ok := curso.(string); ok && s != "" {
			id, err := uuid.Parse(s)
			if err != nil {
				return nil, err
			}
			v.CursoID = &id
		}
		if !anoLetivoValido(v.AnoLetivo) || !nivelValido(v.Nivel) {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// cobrancasExistentesMensalidade devolve o conjunto de (codigo_estudante,
// ano_letivo, mes) que JÁ tiveram alguma tentativa de cobrança de
// mensalidade registrada, qualquer que tenha sido o resultado (sucesso,
// falha, cancelada). financeiro_mensalidade_cobrancas é escrita a cada
// evento de cobrança de mensalidade (ver upsertMensalidadeCobrancas em
// internal/projections/financeiro_projection.go), então esta é a fonte
// definitiva para "existiu tentativa" — independente do estado atual da
// cobrança ou da obrigação.
func (s *Service) cobrancasExistentesMensalidade(ctx context.Context, academia string, estudantes []string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(estudantes) == 0 {
		return out, nil
	}
	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT codigo_estudante, ano_letivo, mes FROM financeiro_mensalidade_cobrancas WHERE codigo_academia=$1 AND codigo_estudante = ANY($2)`, academia, pq.Array(estudantes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var estudante, ano string
		var mes int
		if err := rows.Scan(&estudante, &ano, &mes); err != nil {
			return nil, err
		}
		out[estudante+"|"+ano+"|"+strconv.Itoa(mes)] = true
	}
	return out, rows.Err()
}

// chargeIDsEscopoMensalidade devolve os IDs de financeiro_cobrancas cujas
// mensalidades pertencem ao escopo pedido (turma/curso/ano_academico/
// ano_letivo), resolvido via o mesmo escopoMensalidadeEstudantes usado por
// PendenciasSemCobranca. Como financeiro_mensalidade_cobrancas só tem linha
// para cobranças de ORIGEM mensalidade (nunca matrícula ou avulsa — ver
// upsertMensalidadeCobrancas), este filtro naturalmente restringe o
// resultado a cobranças de mensalidade quando usado; é uma decisão de design
// deliberada, documentada na tarefa que introduziu este filtro.
// Devolve []string (representação textual dos UUIDs), não []uuid.UUID:
// mesma convenção já usada em internal/handlers/avaliacao_final_regras.go
// (uuidStrings) para parâmetros ANY($n::uuid[]) via pq.Array — pq.Array não
// suporta []uuid.UUID diretamente por reflection.
func (s *Service) chargeIDsEscopoMensalidade(ctx context.Context, academia string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string) ([]string, error) {
	vinculos, err := s.escopoMensalidadeEstudantes(ctx, academia, turmaID, cursoID, anoAcademico, anoLetivo)
	if err != nil {
		return nil, err
	}
	if len(vinculos) == 0 {
		return []string{}, nil
	}
	pares := map[string]bool{}
	estudantesSet := map[string]bool{}
	for _, v := range vinculos {
		pares[v.CodigoEstudante+"|"+v.AnoLetivo] = true
		estudantesSet[v.CodigoEstudante] = true
	}
	estudantes := make([]string, 0, len(estudantesSet))
	for e := range estudantesSet {
		estudantes = append(estudantes, e)
	}
	rows, err := s.client.DB().QueryContext(ctx, `SELECT DISTINCT charge_id, codigo_estudante, ano_letivo FROM financeiro_mensalidade_cobrancas WHERE codigo_academia=$1 AND codigo_estudante = ANY($2)`, academia, pq.Array(estudantes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id uuid.UUID
		var estudante, ano string
		if err := rows.Scan(&id, &estudante, &ano); err != nil {
			return nil, err
		}
		if pares[estudante+"|"+ano] {
			out = append(out, id.String())
		}
	}
	return out, rows.Err()
}

// PendenciasSemCobranca lista os meses de mensalidade em estado "pendente"
// que NUNCA tiveram nenhuma tentativa de cobrança registrada, para o
// conjunto de estudantes definido pelo escopo obrigatório informado (ver
// escopoMensalidadeEstudantes). É esta lista que resolve o problema de a
// academia não enxergar, em nenhuma consulta, a dívida de um estudante que
// ainda não gerou (nem tentou gerar) nenhuma cobrança — hoje só o próprio
// estudante vê isso, via GET /financeiro/mensalidades/estudante/:codigo.
//
// Reaproveita ListMensalidades (já testado, mesma função usada por
// ConsultarMensalidadesEstudante) uma vez por estudante do escopo, em vez de
// re-derivar mesInicioEfetivo/resolveConfiguracao/precedenciaEstado em SQL
// batch: o escopo obrigatório garante que o número de estudantes é sempre
// delimitado (uma turma, um ano acadêmico, um curso ou um ano letivo, nunca
// a academia inteira sem filtro), então o custo de N chamadas sequenciais é
// aceitável nesse volume.
func (s *Service) PendenciasSemCobranca(ctx context.Context, academia string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string) ([]MensalidadeMesView, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	vinculos, err := s.escopoMensalidadeEstudantes(ctx, academia, turmaID, cursoID, anoAcademico, anoLetivo)
	if err != nil {
		return nil, err
	}
	if len(vinculos) == 0 {
		return []MensalidadeMesView{}, nil
	}
	estudantesSet := map[string]bool{}
	vinculoValido := map[string]bool{}
	for _, v := range vinculos {
		estudantesSet[v.CodigoEstudante] = true
		vinculoValido[v.CodigoEstudante+"|"+v.AnoLetivo+"|"+v.AnoAcademico+"|"+optionalUUID(v.CursoID)] = true
	}
	estudantes := make([]string, 0, len(estudantesSet))
	for e := range estudantesSet {
		estudantes = append(estudantes, e)
	}

	existentes, err := s.cobrancasExistentesMensalidade(ctx, academia, estudantes)
	if err != nil {
		return nil, err
	}

	out := []MensalidadeMesView{}
	for _, estudante := range estudantes {
		meses, err := s.ListMensalidades(ctx, estudante, &academia)
		if err != nil {
			return nil, err
		}
		for _, m := range meses {
			if m.Estado != EstadoPendente {
				continue
			}
			chaveVinculo := m.CodigoEstudante + "|" + m.AnoLetivo + "|" + m.AnoAcademico + "|" + optionalUUID(m.CursoID)
			if !vinculoValido[chaveVinculo] {
				continue
			}
			chaveCobranca := m.CodigoEstudante + "|" + m.AnoLetivo + "|" + strconv.Itoa(m.Mes)
			if existentes[chaveCobranca] {
				continue
			}
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CodigoEstudante != out[j].CodigoEstudante {
			return out[i].CodigoEstudante < out[j].CodigoEstudante
		}
		return out[i].DataReferencia.Before(out[j].DataReferencia)
	})
	return out, nil
}

// PendenciasSemCobrancaEstudante é a versão de PendenciasSemCobranca
// delimitada a UM estudante — sempre segura de chamar sem exigir escopo
// adicional, porque já está inerentemente limitada a um único estudante.
// Usada por ConsultarCobrancasEstudante para que a consulta de pagamentos de
// um estudante específico traga também os meses que ele deve mas ainda não
// tentou pagar, sem exigir nenhum filtro extra do chamador.
func (s *Service) PendenciasSemCobrancaEstudante(ctx context.Context, codigoEstudante string, somenteAcademia *string) ([]MensalidadeMesView, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	if codigoEstudante == "" {
		return nil, errors.New("código do estudante é obrigatório")
	}
	meses, err := s.ListMensalidades(ctx, codigoEstudante, somenteAcademia)
	if err != nil {
		return nil, err
	}
	pendentes := make([]MensalidadeMesView, 0, len(meses))
	for _, m := range meses {
		if m.Estado == EstadoPendente {
			pendentes = append(pendentes, m)
		}
	}
	if len(pendentes) == 0 {
		return []MensalidadeMesView{}, nil
	}
	academiasSet := map[string]bool{}
	for _, m := range pendentes {
		academiasSet[m.CodigoAcademia] = true
	}
	existentes := map[string]bool{}
	for academia := range academiasSet {
		parcial, err := s.cobrancasExistentesMensalidade(ctx, academia, []string{codigoEstudante})
		if err != nil {
			return nil, err
		}
		for k := range parcial {
			existentes[k] = true
		}
	}
	out := []MensalidadeMesView{}
	for _, m := range pendentes {
		chave := m.CodigoEstudante + "|" + m.AnoLetivo + "|" + strconv.Itoa(m.Mes)
		if existentes[chave] {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}
```

Notas para quem for revisar este arquivo:

- `optionalUUID`, `anoLetivoValido`, `nivelValido`, `EstadoPendente`, `MensalidadeMesView`, `ListMensalidades`
  já existem em `internal/finance/mensalidade.go` — este arquivo novo só os reaproveita, não redefine nada.
- `uuidStrings` (mencionado no comentário) é de `internal/handlers/avaliacao_final_regras.go`, citado só
  como referência de convenção — não precisa (e não deve) ser importado aqui.

---

### 6.2 — `internal/finance/mensalidade_pendencias_integration_test.go`

```go
package finance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"spuri/internal/db"
)

// seedFinanceiroMensalidadeCobranca insere diretamente a linha de vínculo
// cobrança<->mês que, em produção, é escrita por
// upsertMensalidadeCobrancas (internal/projections/financeiro_projection.go)
// a cada evento de cobrança de mensalidade. Os testes de integração deste
// pacote não passam pelo pipeline de eventos/projeção completo, então
// simulamos aqui só a linha que PendenciasSemCobranca e
// chargeIDsEscopoMensalidade efetivamente leem.
func seedFinanceiroMensalidadeCobranca(t *testing.T, client *db.Client, chargeID uuid.UUID, estudante, academia, anoLetivo string, mes int) {
	t.Helper()
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_cobrancas (charge_id,codigo_estudante,codigo_academia,ano_letivo,mes) VALUES ($1,$2,$3,$4,$5)`,
		chargeID, estudante, academia, anoLetivo, mes); err != nil {
		t.Fatal(err)
	}
}

// seedFinanceiroCobrancaMensalidade insere uma cobrança de mensalidade
// (financeiro_cobrancas) e o vínculo correspondente em
// financeiro_mensalidade_cobrancas, simulando uma tentativa de cobrança já
// registrada para o mês informado — o caso que PendenciasSemCobranca deve
// EXCLUIR do resultado (a cobrança pode ter falhado; o que importa é que
// já existiu tentativa).
func seedFinanceiroCobrancaMensalidade(t *testing.T, client *db.Client, academia, estudante, status, anoLetivo string, mes int, valor float64) uuid.UUID {
	t.Helper()
	id := uuid.New()
	payload, err := json.Marshal(map[string]any{
		"status": status, "amount": valor, "currency": "AOA", "description": "mensalidade",
		"payment_method": "REF", "codigo_estudante": estudante,
		"mensalidades": []MensalidadeSelecaoMes{{AnoLetivo: anoLetivo, Mes: mes}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DB().Exec(`INSERT INTO financeiro_cobrancas (id,merchant_transaction_id,contexto_tipo,codigo_academia,payload) VALUES ($1,$2,'academia',$3,$4)`,
		id, integrationMerchant("PND"), academia, payload); err != nil {
		t.Fatal(err)
	}
	seedFinanceiroMensalidadeCobranca(t, client, id, estudante, academia, anoLetivo, mes)
	return id
}

// TestIntegrationPendenciasSemCobrancaExcluiQuandoJaExisteTentativa cobre o
// problema 1 da tarefa 58: um estudante que deve uma mensalidade mas nunca
// gerou (nem tentou gerar) nenhuma cobrança fica hoje totalmente invisível
// para a academia em qualquer consulta de pagamentos — só ele mesmo vê a
// própria dívida, via GET /financeiro/mensalidades/estudante/:codigo.
//
// ESTPN01 nunca tentou nenhuma cobrança: TODOS os seus meses pendentes
// devem aparecer em PendenciasSemCobranca. ESTPN02 já tem uma cobrança
// falhada para setembro: aquele mês específico NÃO deve aparecer (já está
// visível de outra forma, na listagem normal de cobranças), mas os demais
// meses dele, sim.
func TestIntegrationPendenciasSemCobrancaExcluiQuandoJaExisteTentativa(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PND-A", "2026_2027", "ESTPN01", nil)
	seedMensalidadeTurma(t, client, academia, "T-PND-B", "2026_2027", "ESTPN02", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// ESTPN02 já tem uma tentativa de cobrança (falhada) para setembro —
	// não deve aparecer como "pendência sem cobrança" para esse mês.
	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTPN02", "falhada", "2026_2027", 9, 15000)

	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027")
	if err != nil {
		t.Fatal(err)
	}

	achouEst1Setembro := false
	for _, m := range res {
		if m.CodigoEstudante == "ESTPN02" && m.Mes == 9 {
			t.Fatalf("ESTPN02/setembro já tem cobrança (falhada); não deveria aparecer como pendência sem cobrança: %#v", m)
		}
		if m.CodigoEstudante == "ESTPN01" && m.Mes == 9 {
			achouEst1Setembro = true
			if m.Estado != EstadoPendente {
				t.Fatalf("esperava estado pendente, obteve %q", m.Estado)
			}
		}
	}
	if !achouEst1Setembro {
		t.Fatalf("ESTPN01/setembro nunca teve nenhuma cobrança; deveria aparecer como pendência sem cobrança. resultado: %#v", res)
	}

	// ESTPN02 continua tendo os OUTROS meses (out..jul) como pendência
	// sem cobrança — só setembro está coberto pela tentativa já existente.
	outrosMesesEst2 := 0
	for _, m := range res {
		if m.CodigoEstudante == "ESTPN02" {
			outrosMesesEst2++
		}
	}
	if outrosMesesEst2 == 0 {
		t.Fatalf("ESTPN02 deveria ter outros meses pendentes sem cobrança além de setembro")
	}
}

// TestIntegrationPendenciasSemCobrancaExigeEscopo cobre a proteção contra
// varredura sem limite: sem nenhum filtro de escopo (turma_id, curso_id,
// ano_academico ou ano_letivo), PendenciasSemCobranca processaria a
// academia inteira a cada chamada. A função rejeita explicitamente essa
// chamada com erro de validação.
func TestIntegrationPendenciasSemCobrancaExigeEscopo(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	if _, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", ""); err == nil {
		t.Fatal("esperava erro de validação sem nenhum filtro de escopo")
	}
	if _, err := service.PendenciasSemCobranca(ctx, "", nil, nil, "", "2026_2027"); err == nil {
		t.Fatal("esperava erro de validação sem codigo_academia")
	}
}

// TestIntegrationPendenciasSemCobrancaEstudanteNaoExigeEscopo cobre a versão
// por estudante: como já está inerentemente limitada a UM estudante, não
// exige nenhum filtro extra — usada por ConsultarCobrancasEstudante.
func TestIntegrationPendenciasSemCobrancaEstudanteNaoExigeEscopo(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PNDE-A", "2026_2027", "ESTPN03", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	res, err := service.PendenciasSemCobrancaEstudante(ctx, "ESTPN03", &academia)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("esperava pendências sem cobrança para ESTPN03")
	}
	for _, m := range res {
		if m.CodigoEstudante != "ESTPN03" {
			t.Fatalf("resultado contém outro estudante: %#v", m)
		}
	}
}

// TestIntegrationListCobrancasFiltraPorEscopoMensalidade cobre o problema 2
// da tarefa 58: ListCobrancas passa a aceitar turma_id/curso_id/
// ano_academico/ano_letivo para restringir o resultado a cobranças de
// mensalidade vinculadas a esse escopo. Duas turmas da MESMA academia:
// filtrar por uma delas não deve trazer cobranças da outra.
func TestIntegrationListCobrancasFiltraPorEscopoMensalidade(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-FLT-A", "2026_2027", "ESTFL01", nil)
	seedMensalidadeTurma(t, client, academia, "T-FLT-B", "2026_2027", "ESTFL02", nil)

	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTFL01", "Success", "2026_2027", 9, 15000)
	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTFL02", "Success", "2026_2027", 9, 16000)

	semFiltro, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if semFiltro.Total != 2 {
		t.Fatalf("esperava 2 cobranças sem filtro de escopo, obteve %d", semFiltro.Total)
	}

	comFiltroAno, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "7_ano_fundamental", "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comFiltroAno.Total != 2 {
		t.Fatalf("as duas turmas são 7_ano_fundamental (mesmo ano_academico); esperava 2, obteve %d", comFiltroAno.Total)
	}

	comFiltroAnoLetivoInexistente, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2099_2100", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comFiltroAnoLetivoInexistente.Total != 0 {
		t.Fatalf("ano_letivo inexistente deveria devolver 0 cobranças, obteve %d", comFiltroAnoLetivoInexistente.Total)
	}
}
```

`mensalidadeCodigo`, `seedMensalidadeAcademia`, `seedMensalidadeTurma`, `seedMensalidadeConfiguracao`,
`integrationClient`, `integrationMerchant`, `NivelFundamental`, `EstadoPendente`, `ContextoAcademia` já
existem no pacote (`mensalidade_integration_test.go` e `appypay_integration_test.go`) — este arquivo não
redefine nada disso.

---

### 6.3 — `internal/handlers/financeiro_pendencias_handlers_test.go`

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/finance"
)

// seedAcademiaEscolarPrivadaComTurma cria a academia + turma mínimas
// necessárias para exercitar o escopo de mensalidade (turma_id, curso_id,
// ano_academico, ano_letivo) usado por PendenciasSemCobranca e pelos novos
// filtros de ListCobrancas/ListCobrancasEstudante — ver tarefa 58.
func seedAcademiaEscolarPrivadaComTurma(t *testing.T, client *db.Client, academia, codigoTurma, anoLetivo, anoAcademico, estudante string) {
	t.Helper()
	nif := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, uuid.NewString())
	if len(nif) < 10 {
		nif = nif + strings.Repeat("0", 10-len(nif))
	}
	nif = nif[:10]
	if _, err := client.DB().Exec(`INSERT INTO projection_academias
		(id,nivel,nome,nif,codigo_academia,senha_hash,provincia,endereco,nivel_escolar,status,cursos,anos_academicos,type,ano_letivo,created_at)
		VALUES ($1,'escola','Academia HTTP teste',$2,$3,'hash','LUA','endereco','fundamental','ativo','[]'::jsonb,$4::jsonb,'private',$5,CURRENT_TIMESTAMP)`,
		uuid.New(), nif, academia, `["`+anoAcademico+`"]`, anoLetivo); err != nil {
		t.Fatal(err)
	}
	historico := `{"` + anoLetivo + `":["` + estudante + `"]}`
	if _, err := client.DB().Exec(`INSERT INTO projection_turmas
		(id,codigo_turma,codigo_academia,nivel,curso_id,turno,estudantes,historico_estudantes_ano_letivo,status,created_at)
		VALUES ($1,$2,$3,$4,NULL,'manha','[]'::jsonb,$5::jsonb,'ativo',CURRENT_TIMESTAMP)`,
		uuid.New(), codigoTurma, academia, anoAcademico, historico); err != nil {
		t.Fatal(err)
	}
}

func seedMensalidadeConfigParaHTTP(t *testing.T, client *db.Client, academia, anoAcademico string, valor float64) {
	t.Helper()
	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_configuracoes
		(event_id,aggregate_id,codigo_academia,nivel,ano_academico,curso_id,valor,mes_fim_cobranca,vigente_em)
		VALUES ($1,$2,$3,'fundamental',$4,NULL,$5,7,'2026-01-01')`,
		uuid.New(), uuid.New(), academia, anoAcademico, valor); err != nil {
		t.Fatal(err)
	}
}

// TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciasSemCobranca
// cobre, no nível HTTP, o problema 1 da tarefa 58: um estudante que nunca
// tentou nenhuma cobrança de mensalidade é invisível para a academia em
// GET /financeiro/cobrancas — a menos que ela informe um filtro de escopo
// (aqui, ano_letivo), caso em que a resposta passa a incluir
// pendencias_sem_cobranca com os meses que faltam.
func TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciasSemCobranca(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "PND" + strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
	estudante := "ESTPND1"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-PND", "2026_2027", "7_ano_fundamental", estudante)
	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	call := func(query string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas?"+query, nil)
		ctx.Set("dbClient", client)
		ctx.Set("user_id", uuid.New())
		ctx.Set("user_type", "academia")
		ctx.Set("codigo_academia", academia)
		ListarCobrancasAppyPay(ctx)
		return recorder
	}

	// Sem filtro de escopo: nenhuma cobrança foi criada ainda, e
	// pendencias_sem_cobranca não é computado (evita varredura sem limite).
	semEscopo := call("")
	if semEscopo.Code != http.StatusOK {
		t.Fatalf("sem escopo = %d: %s", semEscopo.Code, semEscopo.Body.String())
	}
	var bodySemEscopo map[string]json.RawMessage
	if err := json.Unmarshal(semEscopo.Body.Bytes(), &bodySemEscopo); err != nil {
		t.Fatal(err)
	}
	if _, ok := bodySemEscopo["pendencias_sem_cobranca"]; ok {
		t.Fatalf("sem filtro de escopo, pendencias_sem_cobranca não deveria aparecer na resposta: %s", semEscopo.Body.String())
	}

	// Com ano_letivo: o estudante nunca tentou nenhuma cobrança, então
	// TODOS os meses pendentes dele devem vir em pendencias_sem_cobranca.
	comEscopo := call("ano_letivo=2026_2027")
	if comEscopo.Code != http.StatusOK {
		t.Fatalf("com escopo = %d: %s", comEscopo.Code, comEscopo.Body.String())
	}
	var body struct {
		Cobrancas             []any                        `json:"cobrancas"`
		PendenciasSemCobranca []finance.MensalidadeMesView `json:"pendencias_sem_cobranca"`
	}
	if err := json.Unmarshal(comEscopo.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.PendenciasSemCobranca) == 0 {
		t.Fatalf("esperava pendencias_sem_cobranca não vazio: %s", comEscopo.Body.String())
	}
	for _, p := range body.PendenciasSemCobranca {
		if p.CodigoEstudante != estudante {
			t.Fatalf("pendência de outro estudante inesperada: %#v", p)
		}
		if p.Estado != finance.EstadoPendente {
			t.Fatalf("esperava estado pendente, obteve %q", p.Estado)
		}
	}
}

// TestIntegrationConsultarCobrancasEstudanteIncluiPendenciasSemCobranca
// cobre, no nível HTTP, a versão por estudante (sempre calculada, sem
// exigir filtro de escopo): a própria academia, consultando o histórico de
// UM estudante específico, já enxerga os meses que ele deve e nunca tentou
// pagar — sem precisar de nenhum parâmetro extra.
func TestIntegrationConsultarCobrancasEstudanteIncluiPendenciasSemCobranca(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "PNDE" + strings.ReplaceAll(uuid.NewString(), "-", "")[:6]
	estudante := "ESTPND2"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-PNDE", "2026_2027", "7_ano_fundamental", estudante)
	seedMensalidadeConfigParaHTTP(t, client, academia, "7_ano_fundamental", 15000)
	seedEstudanteParaCobrancas(t, client, estudante, academia)

	previousService := FinanceiroService
	FinanceiroService = finance.NewService(client)
	t.Cleanup(func() { FinanceiroService = previousService })

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/financeiro/cobrancas/estudante/"+estudante, nil)
	ctx.Params = gin.Params{{Key: "codigo", Value: estudante}}
	ctx.Set("dbClient", client)
	ctx.Set("user_id", uuid.New())
	ctx.Set("user_type", "academia")
	ctx.Set("codigo_academia", academia)

	ConsultarCobrancasEstudante(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("academia consultando estudante vinculado = %d: %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		PendenciasSemCobranca []finance.MensalidadeMesView `json:"pendencias_sem_cobranca"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.PendenciasSemCobranca) == 0 {
		t.Fatalf("esperava pendencias_sem_cobranca não vazio: %s", recorder.Body.String())
	}
}
```

`integrationFinanceClient` e `seedEstudanteParaCobrancas` já existem no pacote
(`internal/handlers/financeiro_handlers_integration_test.go` e
`internal/handlers/financeiro_cobrancas_estudante_handlers_test.go`) — não redefina.

**Atenção a um detalhe de schema**: `projection_academias.nif` tem a constraint `check_academia_nif_10_digits`
— precisa ser exatamente 10 dígitos numéricos. `projection_estudantes.codigo_estudante` e
`financeiro_mensalidade_cobrancas.codigo_estudante`/turma `historico_estudantes_ano_letivo` usam códigos de
estudante de no máximo 7 caracteres (`VARCHAR(7)` em `projection_estudantes`) — por isso os códigos de teste
usados aqui (`ESTPND1`, `ESTPND2`, `ESTPN01` etc.) têm exatamente 7 caracteres. Isso já foi verificado
empiricamente por Claude (o teste falhou com strings mais longas antes de ajustar) — não troque esses
códigos por algo mais longo/descritivo.

---

## 7. Ordem de execução recomendada

1. Aplicar o diff da seção 5.1 em `internal/finance/appypay.go` (as duas funções, na ordem em que aparecem
   no arquivo).
2. Aplicar o diff da seção 5.2 em `internal/handlers/financeiro_handlers.go` (import, depois
   `ListarCobrancasAppyPay`, depois a parte final de `ConsultarCobrancasEstudante`).
3. Aplicar as substituições mecânicas da seção 5.3 nos dois arquivos de teste existentes.
4. Criar os três arquivos novos da seção 6, exatamente como estão.
5. Rodar:
   ```
   gofmt -w internal/finance/appypay.go internal/finance/mensalidade_pendencias.go \
     internal/finance/mensalidade_pendencias_integration_test.go \
     internal/finance/cobrancas_integration_test.go internal/finance/cobrancas_estudante_integration_test.go \
     internal/handlers/financeiro_handlers.go internal/handlers/financeiro_pendencias_handlers_test.go
   ```
6. Executar a checklist da seção 8, na ordem.

---

## 8. Checklist de aceitação

Execute cada item na ordem. Se qualquer um falhar, pare e reporte o erro exato — não prossiga nem tente uma
correção diferente da especificada acima sem antes reportar.

1. **Build e vet limpos:**
   ```
   go build ./...
   go vet ./...
   ```
   Ambos devem terminar sem nenhuma saída de erro.

2. **`gofmt` não encontra nada pendente:**
   ```
   gofmt -l .
   ```
   Deve devolver vazio (nenhum arquivo listado).

3. **NÃO rode `go test ./...` esperando resultado —** os testes novos (e vários dos já existentes no pacote
   `finance`/`handlers`) são de integração e vão pular (`SKIP`) automaticamente sem
   `RUN_POSTGRES_INTEGRATION=1` e um Postgres real, que você não tem neste ambiente. Se quiser confirmar que
   pelo menos compilam, `go test -run NADA_QUE_NAO_EXISTE ./...` compila todos os pacotes de teste sem
   executar nada — use isso só para confirmar que não há erro de compilação nos testes novos, não para
   validar comportamento.

4. **Diff final — confirmar que só os arquivos esperados foram alterados/criados:**
   ```
   git status --short
   ```
   Deve mostrar exatamente estes arquivos modificados (`M`), e nenhum outro (nem `go.mod`, nem `go.sum`,
   nem `cmd/server/main.go` — não há rota nova):
   - `internal/finance/appypay.go`
   - `internal/finance/cobrancas_integration_test.go`
   - `internal/finance/cobrancas_estudante_integration_test.go`
   - `internal/handlers/financeiro_handlers.go`

   E exatamente estes arquivos novos (`??`), não rastreados antes desta tarefa:
   - `internal/finance/mensalidade_pendencias.go`
   - `internal/finance/mensalidade_pendencias_integration_test.go`
   - `internal/handlers/financeiro_pendencias_handlers_test.go`

---

## 9. Evidência de validação (já executada por Claude, sandbox com Postgres 16 real)

Para referência — você não precisa reproduzir isto, só confiar que já foi feito:

```
$ go build ./...        # limpo
$ go vet ./...           # limpo
$ gofmt -l .              # vazio

$ RUN_POSTGRES_INTEGRATION=1 DATABASE_URL=postgres://postgres:postgres@localhost:5432/spuri_test_final \
  FINANCE_ENCRYPTION_KEY=12345678901234567890123456789012 go test ./... -count=1

ok      spuri/cmd/server
ok      spuri/internal/db
ok      spuri/internal/domain/aggregates
ok      spuri/internal/finance          (47 testes, incluindo os 4 novos desta tarefa — todos PASS)
ok      spuri/internal/handlers          (incluindo os 2 novos desta tarefa — todos PASS)
ok      spuri/internal/middleware
ok      spuri/internal/projections
ok      spuri/internal/services
ok      spuri/internal/storage
ok      spuri/internal/utils
```

Testes novos confirmados `PASS` individualmente, rodados 2x seguidas contra bancos recriados do zero
(sem flakiness):

- `TestIntegrationPendenciasSemCobrancaExcluiQuandoJaExisteTentativa`
- `TestIntegrationPendenciasSemCobrancaExigeEscopo`
- `TestIntegrationPendenciasSemCobrancaEstudanteNaoExigeEscopo`
- `TestIntegrationListCobrancasFiltraPorEscopoMensalidade`
- `TestIntegrationListarCobrancasAppyPayComEscopoRetornaPendenciasSemCobranca`
- `TestIntegrationConsultarCobrancasEstudanteIncluiPendenciasSemCobranca`

Testes já existentes que exercitam diretamente o código alterado, confirmados ainda `PASS` (zero
regressões):

- `TestIntegrationListCobrancasFiltraOrigemEstadoEIsolaPorAcademia`
- `TestIntegrationListCobrancasEstudanteIncluiMensalidadeEMatricula`
- `TestIntegrationListCobrancasEstudanteSomenteAcademiaIsolaOutraAcademia`
- `TestIntegrationListarCobrancasAppyPayFiltraPorEscopoEEstado`
- `TestIntegrationListarCobrancasAppyPayRejeitaAdminSemPermissaoFPP`
- `TestIntegrationConsultarCobrancasEstudanteEstudanteVeTodosOsEstados`
- `TestIntegrationConsultarCobrancasEstudanteRejeitaOutroEstudante`
- `TestIntegrationConsultarCobrancasEstudanteAcademiaSemVinculoEProibida`

---

## 10. Exemplo de uso (para referência, não para incluir no código)

```
GET /financeiro/cobrancas/estudante/EST0001
```
→ resposta agora inclui, além de `cobrancas` (inalterado):
```json
{
  "cobrancas": [ ... ],
  "total": 2,
  "total_geral": 2,
  "limit": 50,
  "offset": 0,
  "pendencias_sem_cobranca": [
    {
      "codigo_estudante": "EST0001",
      "codigo_academia": "TESTACAD",
      "ano_letivo": "2026_2027",
      "mes": 9,
      "data_referencia": "2026-09-01T00:00:00Z",
      "nivel": "fundamental",
      "ano_academico": "7_ano_fundamental",
      "valor": 15000,
      "mes_fim_cobranca": 7,
      "estado": "pendente",
      "eventos_auditoria": []
    }
  ]
}
```

```
GET /financeiro/cobrancas?codigo_academia=TESTACAD&ano_letivo=2026_2027&ano_academico=7_ano_fundamental
```
→ `cobrancas` restrito às cobranças de mensalidade do 7º ano fundamental de 2026_2027 (matrícula/avulsa
excluídas — ver decisão de design #3), e `pendencias_sem_cobranca` com todos os alunos desse recorte que
nunca tentaram pagar algum mês.

---

## 11. Ao terminar

Depois de passar por toda a checklist da seção 8, mova este arquivo de
`docs/Lista de Tarefas/58 - Pendencias de mensalidade sem cobranca e filtros de escopo academico em cobrancas.md`
para
`docs/Tarefas feitas/58 - Pendencias de mensalidade sem cobranca e filtros de escopo academico em cobrancas.md`,
atualizando o front-matter para `status: feito`, seguindo o padrão dos demais arquivos em
`docs/Tarefas feitas/` (em especial a tarefa 47, no mesmo módulo).

Isso desbloqueia, no frontend, uma tela de "pendências" para a academia acompanhar cobranças ainda não
geradas por turma/curso/ano — hoje impossível, porque a informação simplesmente não existe em nenhuma
resposta de API.
