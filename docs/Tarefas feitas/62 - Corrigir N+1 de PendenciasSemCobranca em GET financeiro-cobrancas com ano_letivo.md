---
criado: 2026-08-23
origem: docs/Debbugs/Depurar lentidao de varios minutos em GET financeiro-cobrancas com ano_letivo.md
status: concluido
tipo: correcao_critica_performance
concluido: 2026-08-23
---

# Corrigir N+1 de `PendenciasSemCobranca` em `GET /financeiro/cobrancas` com `ano_letivo`

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Você (Codex) está rodando num ambiente que **bloqueia `apt` (403 Forbidden)** e **não tem Docker nem `psql`**. Isso significa que você **não consegue subir um PostgreSQL real** para validar esta correção com dados reais.

Isso já foi feito por outra IA (Claude/Anthropic), num sandbox com acesso root e `apt` liberado, que:

1. Clonou o repositório, instalou **PostgreSQL 16.15** e **Go 1.24.4** (via `apt-get`, sem Docker).
2. Aplicou as 116 migrations reais do repositório sem erro.
3. Implementou esta correção, mediu o problema **antes** (37 consultas SQL por estudante, confirmado por log real do Postgres; tempo crescendo linearmente até 1,96s com 300 estudantes) e **depois** (6 consultas totais, independentes do número de estudantes; 11,2ms com 300 estudantes — ~173× mais rápido).
4. Rodou a suíte completa de testes de integração (`go test ./internal/finance/...`) com `RUN_POSTGRES_INTEGRATION=1` contra o Postgres real: todos os testes relacionados a esta área passam, incluindo um teste novo de regressão para um caso de borda (estudante em duas turmas no mesmo ano letivo).
5. Confirmou que as únicas falhas remanescentes na suíte (9 testes) são pré-existentes, não relacionadas a esta correção, e falham igualmente no código original (variável de ambiente `FINANCE_ENCRYPTION_KEY` ausente neste sandbox de validação).

**O que isso significa na prática para você:**

- **Não tente instalar Postgres, Docker ou rodar migrations.** Você não precisa disso para executar esta tarefa.
- **Trate a causa raiz e a correção abaixo como já confirmadas e validadas** — não são hipóteses para re-verificar do zero.
- Toda a correção é entregue como **conteúdo completo de arquivo** (não como patches parciais): você deve **substituir o conteúdo inteiro** dos dois arquivos existentes pelo conteúdo exato fornecido nas seções 1 e 2, e **criar** o arquivo novo da seção 1 com o conteúdo exato fornecido. Isso elimina qualquer risco de aplicar um patch parcial no lugar errado.
- Sua validação (seção "Checklist de validação") usa **apenas** `go build`, `go vet`, `gofmt` e `go test ./...` — nenhum desses comandos precisa de banco de dados. Os testes de integração (arquivos `*_integration_test.go`, incluindo os desta área) já são projetados para **pular automaticamente** (`t.Skip(...)`) quando a variável `RUN_POSTGRES_INTEGRATION` não estiver definida como `"1"` — ou seja, `go test ./...` no seu ambiente vai *mostrar esses testes como pulados*, nunca como falha, e isso é o comportamento esperado e correto.

---

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento, nesta ordem, sem alterar nenhuma decisão de design. Todas as decisões já foram tomadas e validadas (causa raiz confirmada, correção implementada e testada com PostgreSQL 16 e Go 1.24 reais, incluindo medição de performance antes/depois). Sua tarefa é puramente mecânica: (1) substituir o conteúdo inteiro de `internal/finance/mensalidade_pendencias.go` pelo conteúdo exato da seção 2; (2) criar o arquivo novo `internal/finance/mensalidade_pendencias_batch.go` com o conteúdo exato da seção 3; (3) substituir o conteúdo inteiro de `internal/finance/mensalidade_pendencias_integration_test.go` pelo conteúdo exato da seção 4; (4) rodar cada item da seção "Checklist de validação" e reportar o resultado de cada um; (5) seguir o "Procedimento de conclusão". Não toque em nenhum arquivo ou lógica fora do escopo listado na seção "Fora de escopo". Não é necessário PostgreSQL, Docker nem `psql` para nenhuma parte desta tarefa.

---

## 2. Contexto

`GET /financeiro/cobrancas` (handler `ListarCobrancasAppyPay`) chama `FinanceiroService.PendenciasSemCobranca` sempre que `turma_id`, `curso_id`, `ano_academico` **ou** `ano_letivo` é informado — independentemente do filtro `estado`. A página `/financas/pagamentos` do frontend sempre envia `ano_letivo` + `mes` juntos (por design, documentado no próprio código do frontend), então `PendenciasSemCobranca` roda em **toda** consulta feita a partir dessa página.

`PendenciasSemCobranca` resolvia o escopo de estudantes via `escopoMensalidadeEstudantes` (rápido, poucos milissegundos mesmo em escala) e depois chamava `ListMensalidades` **uma vez por estudante** do escopo. `ListMensalidades` foi desenhada para o caso de 1 estudante e dispara, internamente, **~37 consultas SQL sequenciais** por chamada (confirmado por log real do PostgreSQL). O comentário original do código presumia que o escopo era sempre pequeno (uma turma), mas `ano_letivo` sozinho casa com **todos os estudantes da academia inteira** naquele ano — não com uma turma — o que torna o custo N×37 proibitivo (medido: 1,96 segundos só em localhost com 300 estudantes; em produção, com latência de rede real até o banco, isso se traduz nos "vários minutos sem resposta" relatados).

Ver `docs/Debbugs/Depurar lentidao de varios minutos em GET financeiro-cobrancas com ano_letivo.md` para a análise completa, evidência quantitativa e raciocínio da correção.

**Resumo da correção:** `PendenciasSemCobranca` deixa de chamar `ListMensalidades`/`vinculosMensalidade` por estudante. Em vez disso:
- reaproveita os vínculos que `escopoMensalidadeEstudantes` **já resolveu** (elimina a re-consulta redundante de vínculo por estudante);
- memoiza, dentro da mesma chamada, os resultados de `mesInicioEfetivo` e `resolveConfiguracao` (que nunca dependem do estudante, só da combinação academia/nível/ano/curso/mês) — chamando essas **mesmas funções, sem nenhuma alteração nelas**, só evitando repetir a mesma consulta para estudantes que compartilham a mesma turma/configuração;
- substitui o laço `estadoObrigacao` (1 consulta por estudante por mês) por uma nova função `estadosObrigacaoBatch` (1 única consulta para todos os estudantes do escopo de uma vez).

Nenhuma regra de negócio foi duplicada ou reescrita: `mesInicioEfetivo`, `resolveConfiguracao`, `precedenciaEstado`, `ListMensalidades`, `vinculosMensalidade`, `estadoObrigacao`, `escopoMensalidadeEstudantes` continuam **exatamente como estavam**, sem nenhuma linha alterada — são reaproveitadas ou memoizadas, nunca reimplementadas.

Um caso de borda foi identificado e corrigido: `escopoMensalidadeEstudantes` deduplica por `(turma_id, academia, ano_letivo, nivel, ano_academico, curso_id, estudante)` — **incluindo `turma_id`** — enquanto a versão antiga (via `vinculosMensalidade`) deduplicava sem `turma_id`. Um estudante que aparece em duas turmas diferentes para a mesma combinação de ano/nível/curso (ex.: transferência no meio do ano letivo histórico) geraria pendências **duplicadas** se os vínculos crus não fossem deduplicados antes de processá-los. A correção deduplica explicitamente por essa mesma chave (sem `turma_id`) antes de expandir os meses. Um teste de regressão permanente cobre este cenário (seção 4).

---

## 3. Resumo executivo

| # | Arquivo | Tipo de mudança |
|---|---|---|
| 1 | `internal/finance/mensalidade_pendencias.go` | **Substituir conteúdo inteiro do arquivo.** Só a função `PendenciasSemCobranca` e seu comentário mudam de comportamento; as demais funções do arquivo (`escopoMensalidadeEstudantes`, `cobrancasExistentesMensalidade`, `chargeIDsEscopoMensalidade`, `PendenciasSemCobrancaEstudante`) estão **byte-a-byte idênticas** ao original — a substituição do arquivo inteiro é só para eliminar qualquer risco de erro de patch parcial |
| 2 | `internal/finance/mensalidade_pendencias_batch.go` | **Arquivo novo.** Contém apenas `estadosObrigacaoBatch` (a versão em lote de `estadoObrigacao`) |
| 3 | `internal/finance/mensalidade_pendencias_integration_test.go` | **Substituir conteúdo inteiro do arquivo.** Todos os testes existentes continuam **byte-a-byte idênticos**; só foi acrescentado 1 teste novo ao final |

Nenhum arquivo é removido. Nenhum outro arquivo do repositório (incluindo `internal/handlers/financeiro_handlers.go`, `internal/finance/mensalidade.go` e `internal/finance/appypay.go`) precisa de qualquer alteração — a assinatura pública de `PendenciasSemCobranca` (`func (s *Service) PendenciasSemCobranca(ctx context.Context, academia string, turmaID, cursoID *uuid.UUID, anoAcademico, anoLetivo string, mes *int) ([]MensalidadeMesView, error)`) permanece **idêntica**, então nenhum chamador precisa mudar.

---

## 4. Validação atômica ANTES de aplicar (pré-flight — rode e confira antes de tocar em qualquer arquivo)

Estes nomes são introduzidos **pela primeira vez** por esta tarefa. Confirme que nenhum já existe no repositório (evita redeclaração):

```bash
grep -rn "estadosObrigacaoBatch\|obrigacaoEstadoBatch" --include="*.go" .
```

**Resultado esperado: vazio (nenhuma ocorrência).** Se qualquer coisa aparecer, **pare** e reporte antes de prosseguir — significa que o repositório já mudou desde que esta tarefa foi escrita, e a tarefa precisa ser revisada antes de ser aplicada.

Confirme também que o arquivo novo ainda não existe:

```bash
ls internal/finance/mensalidade_pendencias_batch.go
```

**Resultado esperado:** `No such file or directory` (o arquivo ainda não existe). Se já existir, **pare** e reporte.

---

## 5. `internal/finance/mensalidade_pendencias.go` — substituir conteúdo inteiro

Apague todo o conteúdo atual do arquivo `internal/finance/mensalidade_pendencias.go` e substitua exatamente pelo conteúdo abaixo (do `package finance` até o fechamento da última função):

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

// PendenciasSemCobranca lista os meses de mensalidade em estado "pendente"
// que NUNCA tiveram nenhuma tentativa de cobrança registrada, para o
// conjunto de estudantes definido pelo escopo obrigatório informado (ver
// escopoMensalidadeEstudantes). É esta lista que resolve o problema de a
// academia não enxergar, em nenhuma consulta, a dívida de um estudante que
// ainda não gerou (nem tentou gerar) nenhuma cobrança — hoje só o próprio
// estudante vê isso, via GET /financeiro/mensalidades/estudante/:codigo.
//
// ATENÇÃO — histórico de performance (ver docs/Debbugs/ e docs/Lista de
// Tarefas/ da tarefa "GET /financeiro/cobrancas — lentidão de vários
// minutos com ano_letivo"): esta função já chamou ListMensalidades (que
// dispara ~37 consultas SQL sequenciais por estudante) uma vez por
// estudante do escopo, presumindo que o escopo era sempre pequeno (uma
// turma, um curso, um ano acadêmico OU um ano letivo). Essa premissa não se
// sustenta para ano_letivo sozinho — o filtro que o frontend usa em
// /financas/pagamentos, junto de mes — porque ano_letivo casa com TODOS os
// estudantes da ACADEMIA INTEIRA naquele ano, não com uma turma. Numa
// academia de porte médio isso já significava milhares de idas ao banco em
// série dentro de uma única requisição HTTP.
//
// A implementação atual NÃO chama ListMensalidades nem vinculosMensalidade
// por estudante: os vínculos já vêm, para todo o escopo de uma vez, de
// escopoMensalidadeEstudantes (uma única consulta que já precisava rodar
// para resolver o escopo). O que ainda depende de I/O é tratado assim:
//   - mesInicioEfetivo e resolveConfiguracao (chamadas sem alteração,
//     mesmo comportamento e mesma assinatura de sempre) dependem só de
//     (academia, ano_letivo, nivel) e de (academia, nivel, ano_academico,
//     curso_id, mês) respectivamente — nunca do estudante. São memoizadas
//     nesta chamada: uma única consulta por combinação distinta, e não
//     mais uma consulta por estudante.
//   - estadoObrigacao (que É por estudante) foi convertida, só para este
//     caminho multi-estudante, em estadosObrigacaoBatch
//     (mensalidade_pendencias_batch.go): uma única consulta para todos os
//     estudantes do escopo, em vez de uma consulta por (estudante, mês).
//     estadoObrigacao em si continua existindo, inalterada, para o
//     caminho por estudante (ListMensalidades / PendenciasSemCobrancaEstudante).
//
// Um mesmo estudante pode aparecer em escopoMensalidadeEstudantes mais de
// uma vez com o MESMO (ano_letivo, nivel, ano_academico, curso_id) — só
// diferindo por turma_id (ex.: transferência de turma no meio do ano
// letivo histórico) — porque aquela função inclui turma_id na
// deduplicação. Para não listar o mesmo mês duas vezes, os vínculos são
// deduplicados aqui com a MESMA chave que vinculosMensalidade já usa (sem
// turma_id) antes de processá-los.
//
// mes (tarefa 60) restringe adicionalmente o resultado a um único mês de
// calendário (1-12) — mesmo raciocínio de chargeIDsEscopoMensalidade: só
// refina um escopo já resolvido pelos outros filtros, nunca os substitui.
// É aplicado o quanto antes (antes mesmo de resolver a configuração do
// mês) para evitar trabalho descartado quando o chamador já sabe que só
// quer um mês — o caso comum vindo do frontend.
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

	vinculosVistos := map[string]bool{}
	vinculosUnicos := make([]mensalidadeEscopoVinculo, 0, len(vinculos))
	estudantesSet := map[string]bool{}
	anosLetivosSet := map[string]bool{}
	for _, v := range vinculos {
		chaveVinculo := v.CodigoEstudante + "|" + v.CodigoAcademia + "|" + v.AnoLetivo + "|" + v.Nivel + "|" + v.AnoAcademico + "|" + optionalUUID(v.CursoID)
		if vinculosVistos[chaveVinculo] {
			continue
		}
		vinculosVistos[chaveVinculo] = true
		vinculosUnicos = append(vinculosUnicos, v)
		estudantesSet[v.CodigoEstudante] = true
		anosLetivosSet[v.AnoLetivo] = true
	}
	estudantes := make([]string, 0, len(estudantesSet))
	for e := range estudantesSet {
		estudantes = append(estudantes, e)
	}
	anosLetivos := make([]string, 0, len(anosLetivosSet))
	for a := range anosLetivosSet {
		anosLetivos = append(anosLetivos, a)
	}

	existentes, err := s.cobrancasExistentesMensalidade(ctx, academia, estudantes)
	if err != nil {
		return nil, err
	}
	estados, err := s.estadosObrigacaoBatch(ctx, academia, anosLetivos, estudantes)
	if err != nil {
		return nil, err
	}

	inicioCache := map[string]int{}
	cfgCache := map[string]MensalidadeConfiguracaoView{}
	cfgNaoEncontrada := map[string]bool{}

	out := []MensalidadeMesView{}
	for _, v := range vinculosUnicos {
		chaveInicio := v.CodigoAcademia + "|" + v.AnoLetivo + "|" + v.Nivel
		inicio, temInicio := inicioCache[chaveInicio]
		if !temInicio {
			inicio, err = s.mesInicioEfetivo(ctx, v.CodigoAcademia, v.AnoLetivo, v.Nivel)
			if err != nil {
				return nil, err
			}
			inicioCache[chaveInicio] = inicio
		}
		natural := mesNaturalInicioAnoLetivo(v.Nivel)
		inicioPos := posicaoNoAnoLetivo(inicio, natural)
		for _, ref := range mesesAnoLetivo(v.AnoLetivo, v.Nivel) {
			if posicaoNoAnoLetivo(ref.Month, natural) < inicioPos {
				continue
			}
			if mes != nil && ref.Month != *mes {
				continue
			}
			chaveCfg := v.CodigoAcademia + "|" + v.Nivel + "|" + v.AnoAcademico + "|" + optionalUUID(v.CursoID) + "|" + ref.Data.Format("2006-01")
			cfg, temCfg := cfgCache[chaveCfg]
			if !temCfg {
				if cfgNaoEncontrada[chaveCfg] {
					continue
				}
				cfg, err = s.resolveConfiguracao(ctx, v.CodigoAcademia, v.Nivel, v.AnoAcademico, v.CursoID, ref.Data)
				if errors.Is(err, ErrNotFound) {
					cfgNaoEncontrada[chaveCfg] = true
					continue
				}
				if err != nil {
					return nil, err
				}
				cfgCache[chaveCfg] = cfg
			}
			if posicaoNoAnoLetivo(ref.Month, natural) > posicaoNoAnoLetivo(cfg.MesFimCobranca, natural) {
				continue
			}
			chaveMes := v.CodigoEstudante + "|" + v.AnoLetivo + "|" + strconv.Itoa(ref.Month)
			estado := EstadoPendente
			var audit []uuid.UUID
			if info, ok := estados[chaveMes]; ok {
				estado = info.Estado
				audit = info.Audit
			}
			if estado != EstadoPendente {
				continue
			}
			if existentes[chaveMes] {
				continue
			}
			out = append(out, MensalidadeMesView{
				CodigoEstudante:  v.CodigoEstudante,
				CodigoAcademia:   v.CodigoAcademia,
				AnoLetivo:        v.AnoLetivo,
				Mes:              ref.Month,
				DataReferencia:   ref.Data,
				Nivel:            v.Nivel,
				AnoAcademico:     v.AnoAcademico,
				CursoID:          v.CursoID,
				Valor:            cfg.Valor,
				MesFimCobranca:   cfg.MesFimCobranca,
				Estado:           estado,
				EventosAuditoria: audit,
			})
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

**Atenção — não redeclara nada:** este arquivo usa `mesInicioEfetivo`, `resolveConfiguracao`, `mesNaturalInicioAnoLetivo`, `posicaoNoAnoLetivo`, `mesesAnoLetivo`, `optionalUUID`, `ErrNotFound`, `MensalidadeConfiguracaoView`, `MensalidadeMesView`, `EstadoPendente` — todos já definidos em `internal/finance/mensalidade.go`/`internal/finance/appypay.go` e **inalterados** por esta tarefa. Só `estadosObrigacaoBatch` é novo, e vem do arquivo da seção 3.

---

## 6. `internal/finance/mensalidade_pendencias_batch.go` — criar arquivo novo

Crie o arquivo `internal/finance/mensalidade_pendencias_batch.go` (ele não existe ainda) com exatamente este conteúdo:

```go
package finance

// Este arquivo contém APENAS a consulta em lote de estados de obrigação de
// mensalidade (financeiro_mensalidade_obrigacoes_eventos) para muitos
// estudantes de uma vez. É usada exclusivamente por PendenciasSemCobranca
// (mensalidade_pendencias.go) para eliminar o padrão N+1 que causava a
// lentidão de vários minutos em GET /financeiro/cobrancas quando ano_letivo
// era informado sem turma_id/curso_id/ano_academico — ver
// docs/Debbugs/ e docs/Lista de Tarefas/ da tarefa correspondente.
//
// Não duplica a regra de precedência: reaproveita precedenciaEstado
// (mensalidade.go), a mesma função usada por estadoObrigacao (que continua
// existindo, inalterada, para o caminho por estudante em ListMensalidades).

import (
	"context"
	"strconv"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// obrigacaoEstadoBatch é o resultado, para UM par (codigo_estudante, mes),
// da mesma regra de precedência aplicada por estadoObrigacao — só que
// resolvida para muitos estudantes de uma vez, a partir de uma única
// consulta ao banco, em vez de uma consulta por (estudante, mes).
type obrigacaoEstadoBatch struct {
	Estado string
	Audit  []uuid.UUID
}

// estadosObrigacaoBatch é a versão em lote de estadoObrigacao: em vez de uma
// consulta por (estudante, mes), busca TODOS os eventos de obrigação de
// TODOS os estudantes informados (restrito aos ano_letivo informados) em UMA
// única consulta, e aplica precedenciaEstado (inalterada) a cada grupo
// (estudante, ano_letivo, mes) em memória.
//
// A chave do mapa devolvido é "codigo_estudante|ano_letivo|mes" (mes como
// string via strconv.Itoa) — o mesmo formato de chave já usado por
// cobrancasExistentesMensalidade, para que o chamador possa reaproveitar a
// mesma chave nas duas consultas.
//
// Um par (estudante, mes) ausente do mapa devolvido nunca teve nenhum
// evento de obrigação registrado — o chamador deve tratar essa ausência
// exatamente como estadoObrigacao trata zero linhas: estado "pendente" e
// auditoria vazia (o mesmo que precedenciaEstado(nil) devolve).
func (s *Service) estadosObrigacaoBatch(ctx context.Context, academia string, anosLetivos, estudantes []string) (map[string]obrigacaoEstadoBatch, error) {
	out := map[string]obrigacaoEstadoBatch{}
	if len(anosLetivos) == 0 || len(estudantes) == 0 {
		return out, nil
	}
	rows, err := s.client.DB().QueryContext(ctx, `SELECT codigo_estudante, ano_letivo, mes, event_id, tipo
		FROM financeiro_mensalidade_obrigacoes_eventos
		WHERE codigo_academia=$1 AND ano_letivo = ANY($2) AND codigo_estudante = ANY($3)
		ORDER BY codigo_estudante, ano_letivo, mes, ocorrido_em, event_id`,
		academia, pq.Array(anosLetivos), pq.Array(estudantes))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type acumulado struct {
		eventos []string
		audit   []uuid.UUID
	}
	acumulador := map[string]*acumulado{}
	ordem := make([]string, 0)

	for rows.Next() {
		var estudante, anoLetivo, tipo string
		var mesEvento int
		var eventID uuid.UUID
		if err := rows.Scan(&estudante, &anoLetivo, &mesEvento, &eventID, &tipo); err != nil {
			return nil, err
		}
		chave := estudante + "|" + anoLetivo + "|" + strconv.Itoa(mesEvento)
		acc, ok := acumulador[chave]
		if !ok {
			acc = &acumulado{}
			acumulador[chave] = acc
			ordem = append(ordem, chave)
		}
		acc.eventos = append(acc.eventos, tipo)
		acc.audit = append(acc.audit, eventID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, chave := range ordem {
		acc := acumulador[chave]
		out[chave] = obrigacaoEstadoBatch{Estado: precedenciaEstado(acc.eventos), Audit: acc.audit}
	}
	return out, nil
}
```

**Atenção — colunas da tabela:** `financeiro_mensalidade_obrigacoes_eventos` já tem as colunas `codigo_estudante`, `codigo_academia`, `ano_letivo`, `mes`, `event_id`, `tipo`, `ocorrido_em` — todas usadas por `estadoObrigacao` (em `mensalidade.go`, inalterada) hoje. Nenhuma migration nova é necessária.

**Atenção — `precedenciaEstado`:** esta função já existe em `internal/finance/mensalidade.go` e não é alterada por esta tarefa. `estadosObrigacaoBatch` apenas a chama, uma vez por grupo, com a mesma lista ordenada de tipos de evento que `estadoObrigacao` já monta hoje (por isso a `ORDER BY ... ocorrido_em, event_id` é obrigatória — sem ela, a ordem dos eventos dentro do grupo mudaria e `precedenciaEstado` poderia produzir um resultado diferente).

---

## 7. `internal/finance/mensalidade_pendencias_integration_test.go` — substituir conteúdo inteiro

Apague todo o conteúdo atual do arquivo e substitua exatamente pelo conteúdo abaixo. **Todos os testes já existentes permanecem idênticos** — a única mudança é o teste novo ao final (`TestIntegrationPendenciasSemCobrancaNaoDuplicaEstudanteEmDuasTurmasMesmoAno`):

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

	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
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
	if _, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "", nil); err == nil {
		t.Fatal("esperava erro de validação sem nenhum filtro de escopo")
	}
	if _, err := service.PendenciasSemCobranca(ctx, "", nil, nil, "", "2026_2027", nil); err == nil {
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

	semFiltro, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "", nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if semFiltro.Total != 2 {
		t.Fatalf("esperava 2 cobranças sem filtro de escopo, obteve %d", semFiltro.Total)
	}

	comFiltroAno, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "7_ano_fundamental", "", nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comFiltroAno.Total != 2 {
		t.Fatalf("as duas turmas são 7_ano_fundamental (mesmo ano_academico); esperava 2, obteve %d", comFiltroAno.Total)
	}

	comFiltroAnoLetivoInexistente, err := service.ListCobrancas(ctx, ContextoAcademia, academia, nil, nil, nil, nil, "", "2099_2100", nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if comFiltroAnoLetivoInexistente.Total != 0 {
		t.Fatalf("ano_letivo inexistente deveria devolver 0 cobranças, obteve %d", comFiltroAnoLetivoInexistente.Total)
	}
}

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

// TestIntegrationPendenciasSemCobrancaNaoDuplicaEstudanteEmDuasTurmasMesmoAno
// cobre um caso de borda da correção de performance de PendenciasSemCobranca
// (tarefa "GET /financeiro/cobrancas — lentidão de vários minutos com
// ano_letivo"): escopoMensalidadeEstudantes inclui turma_id na
// deduplicação (SELECT DISTINCT ... turma_id, ...), diferente de
// vinculosMensalidade (que dedupe por academia+ano_letivo+nivel+
// ano_academico+curso_id, SEM turma_id). Um estudante que aparece em DUAS
// turmas diferentes para a MESMA combinação (ex.: transferência de turma no
// meio do ano letivo histórico) produz duas linhas distintas em
// escopoMensalidadeEstudantes — PendenciasSemCobranca precisa deduplicar
// essas linhas antes de expandir os meses, ou listaria cada mês pendente
// duas vezes para esse estudante.
func TestIntegrationPendenciasSemCobrancaNaoDuplicaEstudanteEmDuasTurmasMesmoAno(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "6_ano_fundamental", nil, 15000, 7, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	seedMensalidadeTurma(t, client, academia, "T-DUP-A", "2020_2021", "ESTDUP01", nil)
	seedMensalidadeTurma(t, client, academia, "T-DUP-B", "2020_2021", "ESTDUP01", nil)

	mesSetembro := 9
	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2020_2021", &mesSetembro)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range res {
		if m.CodigoEstudante == "ESTDUP01" && m.Mes == 9 {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("esperava exatamente 1 pendência para ESTDUP01/setembro (estudante em 2 turmas do mesmo ano), obteve %d: %#v", count, res)
	}

	semMes, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2020_2021", nil)
	if err != nil {
		t.Fatal(err)
	}
	porMes := map[int]int{}
	for _, m := range semMes {
		if m.CodigoEstudante == "ESTDUP01" {
			porMes[m.Mes]++
		}
	}
	if len(porMes) == 0 {
		t.Fatal("esperava pendências para ESTDUP01 no ano letivo inteiro")
	}
	for mes, qtd := range porMes {
		if qtd != 1 {
			t.Fatalf("mês %d apareceu %d vezes para ESTDUP01 (esperava exatamente 1)", mes, qtd)
		}
	}
}
```

---

## 8. Fora de escopo (não altere)

- `internal/finance/mensalidade.go` inteiro (`ListMensalidades`, `vinculosMensalidade`, `mesInicioEfetivo`, `resolveConfiguracao`, `estadoObrigacao`, `precedenciaEstado`, `mesNaturalInicioAnoLetivo`, `posicaoNoAnoLetivo`, `mesesAnoLetivo`, `optionalUUID`, e qualquer outra função) — nenhuma linha muda.
- `internal/finance/appypay.go` inteiro, incluindo `ListCobrancas` — nenhuma linha muda.
- `internal/handlers/financeiro_handlers.go` — a assinatura de `PendenciasSemCobranca` não mudou, então o handler não precisa de nenhuma alteração.
- A redundância de `escopoMensalidadeEstudantes` rodar duas vezes por requisição (uma em `ListCobrancas`, outra em `PendenciasSemCobranca`) — documentada na seção 5 do documento de depuração como achado secundário, **deliberadamente fora do escopo** desta tarefa (o custo medido é de poucos milissegundos, desprezível perto do ganho principal; corrigi-la aumentaria a superfície de mudança em código compartilhado sem necessidade).
- Qualquer arquivo do repositório `spuripainel` (frontend) — não é necessária nenhuma alteração de frontend; o padrão de chamada (`ano_letivo` + `mes`) já está correto por design.
- Qualquer refatoração adicional, renomeação de variáveis, ou "melhoria" não explicitamente pedida neste documento.
- Não crie nenhum mecanismo de cache persistente/global, fila, worker assíncrono ou paginação server-side adicional para este endpoint — a correção pedida é apenas a eliminação do N+1 dentro de `PendenciasSemCobranca`, exatamente como especificado nas seções 5 e 6.

---

## 9. Checklist de validação (Codex deve executar e reportar o resultado de cada item)

Nenhum destes comandos requer PostgreSQL, Docker ou `psql`:

1. `grep -rn "estadosObrigacaoBatch\|obrigacaoEstadoBatch" --include="*.go" .` **antes** de aplicar qualquer mudança — deve retornar vazio (seção 4). Repita **depois** de aplicar — agora deve aparecer só nas duas ocorrências esperadas (definição em `mensalidade_pendencias_batch.go`, uso em `mensalidade_pendencias.go`).
2. `go build ./...` — deve terminar sem erros (nenhum "redeclared", nenhum "undefined", nenhum "imported and not used").
3. `go vet ./...` — deve terminar sem erros.
4. `gofmt -l internal/finance/mensalidade_pendencias.go internal/finance/mensalidade_pendencias_batch.go internal/finance/mensalidade_pendencias_integration_test.go` — deve retornar vazio (nenhum arquivo fora do padrão de formatação).
5. `go test ./...` — deve rodar sem falhas. Os testes `TestIntegration*` do pacote `internal/finance` vão aparecer como `SKIP` (não como `FAIL`) se `RUN_POSTGRES_INTEGRATION` não estiver definida — isso é esperado e correto no seu ambiente.
6. `git diff --stat` — deve mostrar alterações **apenas** em `internal/finance/mensalidade_pendencias.go`, `internal/finance/mensalidade_pendencias_integration_test.go` (modificados) e `internal/finance/mensalidade_pendencias_batch.go` (novo) — mais os documentos de conclusão da seção seguinte.

Se qualquer um desses itens falhar, **não prossiga** — reporte o erro exato ao invés de tentar corrigir com uma solução diferente da especificada neste documento.

---

## 10. Critérios de aceite

- [ ] `internal/finance/mensalidade_pendencias.go` substituído exatamente pelo conteúdo da seção 5.
- [ ] `internal/finance/mensalidade_pendencias_batch.go` criado exatamente com o conteúdo da seção 6.
- [ ] `internal/finance/mensalidade_pendencias_integration_test.go` substituído exatamente pelo conteúdo da seção 7.
- [ ] Todos os 6 itens do checklist de validação (seção 9) executados e reportados com sucesso.
- [ ] Nenhum arquivo fora do escopo desta tarefa foi alterado (seção 8).
- [ ] `git diff --stat` mostra alterações apenas nos 3 arquivos listados no resumo executivo (seção 3), mais os documentos de conclusão.

---

## 11. Procedimento de conclusão

1. Após todos os critérios de aceite acima estarem satisfeitos, mover este arquivo de `docs/Lista de Tarefas/` para `docs/Tarefas feitas/`, atualizando o frontmatter: `status: concluido` e adicionando `concluido: <data de hoje>`.
2. Atualizar o frontmatter de `docs/Debbugs/Depurar lentidao de varios minutos em GET financeiro-cobrancas com ano_letivo.md`, campo `status`, de `causa_raiz_confirmada_correcao_pronta_para_execucao` para `corrigido_via_tarefa_N1_pendencias_sem_cobranca` (substitua pelo nome real do arquivo desta tarefa após movido).
3. Criar um commit único contendo todas as alterações, com mensagem: `Corrigir N+1 de PendenciasSemCobranca em GET /financeiro/cobrancas com ano_letivo`.
4. Reportar a Fredy: resultado de cada item do checklist de validação, e a lista de arquivos alterados (`git diff --stat` do commit). **Não é necessário** nenhuma validação adicional com PostgreSQL real — a Claude já validou esta correção especificamente com PostgreSQL 16 e Go 1.24 reais, incluindo medição de performance antes/depois (37 queries/estudante → 6 queries totais; 1,96s → 11,2ms com 300 estudantes) e a suíte completa de testes de integração. Esta tarefa é de execução mecânica de um resultado já testado e aprovado.

**Nenhuma etapa deste procedimento remove ou altera qualquer código relacionado à inscrição de estudantes em academias** — todas as alterações estão contidas ao módulo financeiro de mensalidades (`internal/finance/mensalidade_pendencias*.go`), sem tocar em matrícula, cadastro, turmas ou vínculo de estudante à academia.
