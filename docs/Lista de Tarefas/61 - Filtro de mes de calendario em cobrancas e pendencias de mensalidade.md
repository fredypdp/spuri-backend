---
criado: 2026-08-22 00:00
origem: Pedido do usuário (Spuri), orquestrado por Claude (Anthropic) em sandbox com PostgreSQL 16 e Go 1.24 reais
status: pendente
prioridade: alta
depende_de: "59 - Pendencias de mensalidade sem cobranca e filtros de escopo academico em cobrancas.md"
---

# Filtro de mês de calendário em cobranças e pendências de mensalidade

## 0. Leia isto primeiro — sobre o seu ambiente (Codex) e o motivo desta tarefa

Mesma situação da tarefa 59: você (Codex) não tem `apt`, Docker nem `psql` disponíveis, então não consegue
validar isto com um Postgres real. Isso já foi feito por Claude, no mesmo sandbox usado na tarefa 59
(PostgreSQL 16 + Go 1.24 via `apt-get`), com o código já **compilado, testado e validado** contra dados
reais antes de este documento ser escrito. Trate o desenho abaixo como já corrigido, não como algo a
reavaliar — sua tarefa é aplicar exatamente o que está escrito aqui.

**Motivo desta tarefa:** o time está construindo, no frontend (`spuripainel`), uma tela de drill-down
"ano letivo → mês → lista de pagamentos" (ver o outro documento de tarefa entregue ao `spuripainel` no
mesmo lote). Sem um filtro de mês no backend, esse último passo ("mês") teria que buscar o ano letivo
inteiro (até 11 meses de cobranças) e filtrar no cliente — quebrando a paginação de verdade e sendo
desnecessariamente pesado. Esta tarefa fecha essa lacuna com uma extensão pequena e cirúrgica do que a
tarefa 59 já implementou, reaproveitando exatamente o mesmo padrão.

**O que você PODE e DEVE rodar no seu ambiente**: `go build ./...`, `go vet ./...`, `gofmt -l .` (deve vir
vazio). **Não rode `go test ./...`** esperando resultado — os testes de integração pulam (`SKIP`)
automaticamente sem `RUN_POSTGRES_INTEGRATION=1` e Postgres real, que você não tem.

---

## 1. Prompt recomendado para executar esta tarefa

Aplique exatamente os diffs descritos na seção 3 deste documento, na ordem da seção 4, sem alterar o
desenho (nomes, assinaturas, mensagens). Depois de aplicar, rode `gofmt -w` nos arquivos tocados, confirme
`go build ./...`, `go vet ./...` e `gofmt -l .` limpos, confirme com `git status --short` que só os arquivos
listados na seção 5 foram alterados (nenhum arquivo novo nesta tarefa), e gere a documentação de tarefa
concluída em `docs/Tarefas feitas/`, movendo este arquivo para lá com `status: feito`.

---

## 2. O que muda

Um parâmetro novo, `mes` (inteiro 1-12), **refina** os filtros de escopo que a tarefa 59 já introduziu —
nunca os substitui. Ele só tem efeito quando combinado com pelo menos um de `turma_id`, `curso_id`,
`ano_academico` ou `ano_letivo` (a mesma exigência de escopo obrigatório da tarefa 59 continua valendo,
inalterada); `mes` sozinho não delimita o suficiente, porque um mês de calendário pode abranger estudantes
de vários anos letivos diferentes.

Afetado:
- `GET /financeiro/cobrancas` — novo parâmetro de query `mes`, opcional, valida 1-12 (400 se fora do
  intervalo). Filtra tanto `cobrancas` quanto `pendencias_sem_cobranca`.
- `Service.ListCobrancas`, `Service.ListCobrancasEstudante`, `Service.chargeIDsEscopoMensalidade`,
  `Service.PendenciasSemCobranca` — todas ganham um parâmetro `mes *int` a mais na assinatura.

**Não afetado** (decisão de design deliberada, não esquecimento): `GET /financeiro/cobrancas/estudante/:codigo`
não expõe `mes` como parâmetro de query — a assinatura Go de `ListCobrancasEstudante` precisa aceitar o
parâmetro (porque compartilha `chargeIDsEscopoMensalidade` com `ListCobrancas`), mas o handler
`ConsultarCobrancasEstudante` sempre passa `nil` para ele. Motivo: essa rota não tem, hoje, nenhum consumidor
que precise filtrar por mês, e expor o parâmetro só em parte do fluxo (cobranças filtradas por mês, mas
pendências não, já que `PendenciasSemCobrancaEstudante` não foi tocada nesta tarefa) criaria um
comportamento parcial e confuso. Se um consumidor futuro precisar disso, é uma tarefa separada.

---

## 3. Diffs exatos

### 3.1 — `internal/finance/mensalidade_pendencias.go`

**Localizar:**

```go
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
```

**Substituir por:**

```go
// mes (tarefa 60) filtra adicionalmente por um mês específico de calendário
// (1-12) dentro do escopo já resolvido — não substitui os filtros de
// turma/curso/ano_academico/ano_letivo, apenas os refina, porque um mês
// sozinho não delimita o suficiente (poderia abranger vários anos letivos
// de vários estudantes).
func (s *Service) chargeIDsEscopoMensalidade(ctx context.Context, academia string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, mes *int) ([]string, error) {
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
	q := `SELECT DISTINCT charge_id, codigo_estudante, ano_letivo FROM financeiro_mensalidade_cobrancas WHERE codigo_academia=$1 AND codigo_estudante = ANY($2)`
	args := []any{academia, pq.Array(estudantes)}
	if mes != nil {
		q += " AND mes=$3"
		args = append(args, *mes)
	}
	rows, err := s.client.DB().QueryContext(ctx, q, args...)
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
```

---

**Localizar** (a assinatura de `PendenciasSemCobranca` e o início do seu corpo, até o loop de meses):

```go
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
```

**Substituir por:**

```go
// mes (tarefa 60) restringe adicionalmente o resultado a um único mês de
// calendário (1-12) — mesmo raciocínio de chargeIDsEscopoMensalidade: só
// refina um escopo já resolvido pelos outros filtros, nunca os substitui.
func (s *Service) PendenciasSemCobranca(ctx context.Context, academia string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, mes *int) ([]MensalidadeMesView, error) {
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
			if mes != nil && m.Mes != *mes {
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
```

(o resto do corpo — fechamento do loop, `sort.Slice(...)`, `return out, nil` — permanece exatamente igual,
não mude mais nada nele.)

---

### 3.2 — `internal/finance/appypay.go`

**Localizar:**

```go
func (s *Service) ListCobrancas(ctx context.Context, contexto, academia string, estados, origens []string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, limit, offset int) (*CobrancaListResult, error) {
```

**Substituir por:**

```go
// mes (tarefa 60) restringe adicionalmente a um mês de calendário (1-12)
// dentro do escopo — ver chargeIDsEscopoMensalidade. Só tem efeito quando
// combinado com pelo menos um dos quatro filtros de escopo acima; sozinho,
// não delimita o suficiente (haveria estudantes de vários anos letivos
// diferentes com cobranças naquele mesmo mês de calendário).
func (s *Service) ListCobrancas(ctx context.Context, contexto, academia string, estados, origens []string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, mes *int, limit, offset int) (*CobrancaListResult, error) {
```

---

**Localizar** (dentro do corpo de `ListCobrancas`, o bloco que resolve o escopo — este texto é único no
arquivo, não se confunde com o bloco equivalente de `ListCobrancasEstudante` porque a chamada usa `academia`
diretamente, não `academiaEscopo`):

```go
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		chargeIDs, err := s.chargeIDsEscopoMensalidade(ctx, academia, turmaID, cursoID, anoAcademico, anoLetivo)
		if err != nil {
			return nil, err
		}
		where += fmt.Sprintf(" AND id = ANY($%d::uuid[])", i)
		args = append(args, pq.Array(chargeIDs))
		i++
	}
```

**Substituir por:**

```go
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		chargeIDs, err := s.chargeIDsEscopoMensalidade(ctx, academia, turmaID, cursoID, anoAcademico, anoLetivo, mes)
		if err != nil {
			return nil, err
		}
		where += fmt.Sprintf(" AND id = ANY($%d::uuid[])", i)
		args = append(args, pq.Array(chargeIDs))
		i++
	}
```

---

**Localizar:**

```go
func (s *Service) ListCobrancasEstudante(ctx context.Context, codigoEstudante string, somenteAcademia *string, estados, origens []string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, limit, offset int) (*CobrancaListResult, error) {
```

**Substituir por:**

```go
// mes (tarefa 60) tem o mesmo efeito de ListCobrancas: só delimita a mais
// junto de um dos quatro filtros acima. Nenhum endpoint HTTP expõe este
// parâmetro para ListCobrancasEstudante ainda — a assinatura só ganhou o
// parâmetro para poder compartilhar chargeIDsEscopoMensalidade com
// ListCobrancas sem duplicar código; ver tarefa 60 para o contexto completo.
func (s *Service) ListCobrancasEstudante(ctx context.Context, codigoEstudante string, somenteAcademia *string, estados, origens []string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, mes *int, limit, offset int) (*CobrancaListResult, error) {
```

---

**Localizar** (dentro do corpo de `ListCobrancasEstudante`, distinguível do bloco equivalente de
`ListCobrancas` pela variável `academiaEscopo`):

```go
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
```

**Substituir por:**

```go
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		academiaEscopo := ""
		if somenteAcademia != nil {
			academiaEscopo = *somenteAcademia
		}
		chargeIDs, err := s.chargeIDsEscopoMensalidade(ctx, academiaEscopo, turmaID, cursoID, anoAcademico, anoLetivo, mes)
		if err != nil {
			return nil, err
		}
		where += fmt.Sprintf(" AND id = ANY($%d::uuid[])", i)
		args = append(args, pq.Array(chargeIDs))
		i++
	}
```

---

### 3.3 — `internal/handlers/financeiro_handlers.go`

**Localizar** (o bloco de import):

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

**Substituir por:**

```go
import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/finance"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)
```

(só adiciona `"strconv"`, nada mais muda neste bloco.)

---

**Localizar** (a função `ListarCobrancasAppyPay` inteira, como ficou depois da tarefa 59):

```go
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

**Substituir por:**

```go
// parseOptionalMesQuery lê um parâmetro de query opcional como mês de
// calendário (1-12). Devolve nil quando o parâmetro não foi informado, e
// erro quando foi informado mas não é um inteiro entre 1 e 12.
func parseOptionalMesQuery(c *gin.Context, param string) (*int, error) {
	raw := strings.TrimSpace(c.Query(param))
	if raw == "" {
		return nil, nil
	}
	mes, err := strconv.Atoi(raw)
	if err != nil || mes < 1 || mes > 12 {
		return nil, fmt.Errorf("%s deve ser um mês entre 1 e 12", param)
	}
	return &mes, nil
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
	mes, err := parseOptionalMesQuery(c, "mes")
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := FinanceiroService.ListCobrancas(c.Request.Context(), contexto, academia, c.QueryArray("estado"), c.QueryArray("tipo"), turmaID, cursoID, anoAcademico, anoLetivo, mes, limit, offset)
	if err != nil {
		financeError(c, err)
		return
	}
	body := gin.H{"cobrancas": res.Cobrancas, "total": len(res.Cobrancas), "total_geral": res.Total, "limit": limit, "offset": offset}
	// pendencias_sem_cobranca só é computado quando pelo menos um dos
	// quatro filtros de escopo (turma_id, curso_id, ano_academico,
	// ano_letivo) é informado junto de codigo_academia — sem isso, a
	// varredura seria sobre a academia inteira sem limite. mes (tarefa 60)
	// só refina esse escopo, nunca o substitui. Ver finance.PendenciasSemCobranca.
	if turmaID != nil || cursoID != nil || anoAcademico != "" || anoLetivo != "" {
		pendencias, err := FinanceiroService.PendenciasSemCobranca(c.Request.Context(), academia, turmaID, cursoID, anoAcademico, anoLetivo, mes)
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

**Localizar** (dentro de `ConsultarCobrancasEstudante`, a linha que chama `ListCobrancasEstudante`):

```go
	res, err := FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, c.QueryArray("estado"), c.QueryArray("tipo"), turmaID, cursoID, anoAcademico, anoLetivo, limit, offset)
```

**Substituir por:**

```go
	// mes não é exposto como parâmetro de query nesta rota ainda (só em
	// GET /financeiro/cobrancas, tarefa 60) — passamos nil para manter o
	// comportamento anterior inalterado aqui.
	res, err := FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, c.QueryArray("estado"), c.QueryArray("tipo"), turmaID, cursoID, anoAcademico, anoLetivo, nil, limit, offset)
```

---

### 3.4 — Testes existentes: só assinatura de chamada muda

Todo call site de `ListCobrancas`, `ListCobrancasEstudante` e `PendenciasSemCobranca` nos arquivos de teste
abaixo precisa do argumento `mes` a mais (sempre `nil`, já que nenhum desses testes testa o filtro `mes`
especificamente — o teste do filtro `mes` é novo, ver seção 3.5).

**`internal/finance/cobrancas_integration_test.go`** — toda chamada `.ListCobrancas(`\<args\>`, "", "",
<limit>, <offset>)` vira `.ListCobrancas(`\<args\>`, "", "", nil, <limit>, <offset>)`. Concretamente:

| Antes | Depois |
|---|---|
| `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, nil, nil, "", "", 50, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, nil, nil, "", "", nil, 50, 0)` |
| `service.ListCobrancas(ctx, ContextoAcademia, academiaA, []string{"Success"}, nil, nil, nil, "", "", 50, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaA, []string{"Success"}, nil, nil, nil, "", "", nil, 50, 0)` |
| `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, []string{"matricula"}, nil, nil, "", "", 50, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, []string{"matricula"}, nil, nil, "", "", nil, 50, 0)` |
| `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, nil, nil, "", "", 1, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaA, nil, nil, nil, nil, "", "", nil, 1, 0)` |
| `service.ListCobrancas(ctx, ContextoAcademia, academiaB, nil, nil, nil, nil, "", "", 50, 0)` | `service.ListCobrancas(ctx, ContextoAcademia, academiaB, nil, nil, nil, nil, "", "", nil, 50, 0)` |

**`internal/finance/cobrancas_estudante_integration_test.go`** — mesmo padrão para
`.ListCobrancasEstudante(`:

| Antes | Depois |
|---|---|
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, nil, nil, nil, "", "", 50, 0)` | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, nil, nil, nil, "", "", nil, 50, 0)` |
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, []string{"Success"}, nil, nil, nil, "", "", 50, 0)` | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, []string{"Success"}, nil, nil, nil, "", "", nil, 50, 0)` |
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"mensalidade"}, nil, nil, "", "", 50, 0)` | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"mensalidade"}, nil, nil, "", "", nil, 50, 0)` |
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"matricula"}, nil, nil, "", "", 50, 0)` | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"matricula"}, nil, nil, "", "", nil, 50, 0)` |
| `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"invalido"}, nil, nil, "", "", 50, 0)` (dentro de `if _, err := ...; err == nil`) | `service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"invalido"}, nil, nil, "", "", nil, 50, 0)` |
| `service.ListCobrancasEstudante(ctx, estudante, nil, nil, nil, nil, nil, "", "", 50, 0)` | `service.ListCobrancasEstudante(ctx, estudante, nil, nil, nil, nil, nil, "", "", nil, 50, 0)` |
| `service.ListCobrancasEstudante(ctx, estudante, &academiaA, nil, nil, nil, nil, "", "", 50, 0)` | `service.ListCobrancasEstudante(ctx, estudante, &academiaA, nil, nil, nil, nil, "", "", nil, 50, 0)` |

**`internal/finance/mensalidade_pendencias_integration_test.go`** — toda chamada a `.ListCobrancas(` segue a
mesma regra da tabela acima (mesmos 4 call sites já existentes nesse arquivo, incluindo os que usam
`"7_ano_fundamental"` ou `"2099_2100"` como argumento de `anoAcademico`/`anoLetivo` — o `nil` de `mes` entra
sempre imediatamente antes de `<limit>, <offset>`). Toda chamada a `.PendenciasSemCobranca(`\<args\>`)` vira
`.PendenciasSemCobranca(`\<args\>`, nil)` — são 3 call sites já existentes nesse arquivo.

---

### 3.5 — Testes novos (conteúdo completo a acrescentar)

Acrescente ao **final** de `internal/finance/mensalidade_pendencias_integration_test.go` (depois do último
`}` do arquivo, que hoje fecha `TestIntegrationListCobrancasFiltraPorEscopoMensalidade`):

```go

// TestIntegrationListCobrancasFiltraPorMes cobre a tarefa 60: mes restringe
// ainda mais um escopo já delimitado por ano_letivo (ou outro dos quatro
// filtros) a um único mês de calendário — necessário para o fluxo de
// drill-down do frontend (ano letivo -> mês -> lista) paginar corretamente
// sem precisar buscar o ano letivo inteiro para filtrar no cliente.
func TestIntegrationListCobrancasFiltraPorMes(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-MES-A", "2026_2027", "ESTMS01", nil)

	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTMS01", "Success", "2026_2027", 9, 15000)
	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTMS01", "Success", "2026_2027", 10, 15000)

	mesNove := 9
	comMes, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2026_2027", &mesNove, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comMes.Total != 1 {
		t.Fatalf("esperava 1 cobrança filtrando por mes=9, obteve %d", comMes.Total)
	}

	mesDez := 12
	comMesSemCobranca, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2026_2027", &mesDez, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comMesSemCobranca.Total != 0 {
		t.Fatalf("dezembro não tem cobrança nenhuma; esperava 0, obteve %d", comMesSemCobranca.Total)
	}

	semMes, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2026_2027", nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if semMes.Total != 2 {
		t.Fatalf("sem filtro de mes, esperava as 2 cobranças (setembro e outubro), obteve %d", semMes.Total)
	}
}

// TestIntegrationPendenciasSemCobrancaFiltraPorMes cobre o mesmo filtro
// aplicado a PendenciasSemCobranca — o passo final do drill-down do
// frontend precisa das pendências de UM mês específico, não do ano letivo
// inteiro.
func TestIntegrationPendenciasSemCobrancaFiltraPorMes(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-MESP-A", "2026_2027", "ESTMP01", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	mesSetembro := 9
	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", &mesSetembro)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("esperava exatamente 1 pendência (setembro), obteve %d: %#v", len(res), res)
	}
	if res[0].Mes != 9 {
		t.Fatalf("esperava mes=9, obteve %d", res[0].Mes)
	}

	semMes, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(semMes) <= 1 {
		t.Fatalf("sem filtro de mes, esperava mais de 1 pendência (todo o ano letivo), obteve %d", len(semMes))
	}
}
```

Acrescente ao **final** de `internal/handlers/financeiro_pendencias_handlers_test.go`:

```go

// TestIntegrationListarCobrancasAppyPayFiltraPorMes cobre, no nível HTTP, o
// filtro mes (tarefa 60): combinado com ano_letivo, restringe tanto
// cobrancas quanto pendencias_sem_cobranca a um único mês de calendário —
// é este par de parâmetros que o passo final do drill-down do frontend usa.
func TestIntegrationListarCobrancasAppyPayFiltraPorMes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := integrationFinanceClient(t)
	academia := "MES" + strings.ReplaceAll(uuid.NewString(), "-", "")[:7]
	estudante := "ESTHMS1"
	seedAcademiaEscolarPrivadaComTurma(t, client, academia, "T-HTTP-MES", "2026_2027", "7_ano_fundamental", estudante)
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

	comMesInvalido := call("ano_letivo=2026_2027&mes=13")
	if comMesInvalido.Code != http.StatusBadRequest {
		t.Fatalf("mes=13 deveria ser rejeitado com 400, obteve %d: %s", comMesInvalido.Code, comMesInvalido.Body.String())
	}

	comMesSetembro := call("ano_letivo=2026_2027&mes=9")
	if comMesSetembro.Code != http.StatusOK {
		t.Fatalf("mes=9 = %d: %s", comMesSetembro.Code, comMesSetembro.Body.String())
	}
	var body struct {
		PendenciasSemCobranca []finance.MensalidadeMesView `json:"pendencias_sem_cobranca"`
	}
	if err := json.Unmarshal(comMesSetembro.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.PendenciasSemCobranca) != 1 {
		t.Fatalf("esperava exatamente 1 pendência filtrando por mes=9, obteve %d: %s", len(body.PendenciasSemCobranca), comMesSetembro.Body.String())
	}
	if body.PendenciasSemCobranca[0].Mes != 9 {
		t.Fatalf("esperava mes=9, obteve %d", body.PendenciasSemCobranca[0].Mes)
	}
}
```

`seedFinanceiroCobrancaMensalidade`, `mensalidadeCodigo`, `seedMensalidadeAcademia`, `seedMensalidadeTurma`,
`seedMensalidadeConfiguracao`, `integrationClient`, `NivelFundamental`, `seedAcademiaEscolarPrivadaComTurma`,
`seedMensalidadeConfigParaHTTP`, `integrationFinanceClient` já existem (introduzidos pela tarefa 59) — não
redefina nada disso.

---

## 4. Ordem de execução recomendada

1. `internal/finance/mensalidade_pendencias.go` — os dois diffs da seção 3.1, na ordem em que aparecem no
   arquivo.
2. `internal/finance/appypay.go` — os quatro diffs da seção 3.2, na ordem em que aparecem no arquivo.
3. `internal/handlers/financeiro_handlers.go` — os três diffs da seção 3.3, na ordem em que aparecem no
   arquivo.
4. As substituições mecânicas da seção 3.4 nos três arquivos de teste existentes.
5. Os dois blocos de teste novo da seção 3.5, acrescentados ao final dos respectivos arquivos.
6. `gofmt -w` nos 5 arquivos tocados, depois a checklist da seção 6.

---

## 5. Checklist de aceitação

1. **Build e vet limpos:** `go build ./...` e `go vet ./...` sem nenhuma saída de erro.
2. **`gofmt -l .`** devolve vazio.
3. **Não rode `go test ./...`** esperando resultado (ver seção 0).
4. **Diff final** — `git status --short` deve mostrar exatamente estes arquivos modificados (`M`), e
   nenhum arquivo novo:
   - `internal/finance/appypay.go`
   - `internal/finance/mensalidade_pendencias.go`
   - `internal/finance/cobrancas_integration_test.go`
   - `internal/finance/cobrancas_estudante_integration_test.go`
   - `internal/finance/mensalidade_pendencias_integration_test.go`
   - `internal/handlers/financeiro_handlers.go`
   - `internal/handlers/financeiro_pendencias_handlers_test.go`

---

## 6. Evidência de validação (já executada por Claude, sandbox com Postgres 16 real)

```
$ go build ./...        # limpo
$ go vet ./...           # limpo
$ gofmt -l .              # vazio

$ RUN_POSTGRES_INTEGRATION=1 DATABASE_URL=postgres://postgres:postgres@localhost:5432/spuri_test_mes2 \
  FINANCE_ENCRYPTION_KEY=12345678901234567890123456789012 go test ./... -count=1

ok      spuri/cmd/server
ok      spuri/internal/db
ok      spuri/internal/domain/aggregates
ok      spuri/internal/finance          (todos os testes, incluindo os 2 novos desta tarefa — PASS)
ok      spuri/internal/handlers          (incluindo o 1 novo desta tarefa — PASS)
ok      spuri/internal/middleware
ok      spuri/internal/projections
ok      spuri/internal/services
ok      spuri/internal/storage
ok      spuri/internal/utils
```

Rodado 2x seguidas contra bancos recriados do zero, sem flakiness. Testes novos confirmados `PASS`
individualmente:

- `TestIntegrationListCobrancasFiltraPorMes`
- `TestIntegrationPendenciasSemCobrancaFiltraPorMes`
- `TestIntegrationListarCobrancasAppyPayFiltraPorMes`

---

## 7. Ao terminar

Mova este arquivo de `docs/Lista de Tarefas/60 - Filtro de mes de calendario em cobrancas e pendencias de mensalidade.md`
para `docs/Tarefas feitas/60 - Filtro de mes de calendario em cobrancas e pendencias de mensalidade.md`,
atualizando o front-matter para `status: feito`.
