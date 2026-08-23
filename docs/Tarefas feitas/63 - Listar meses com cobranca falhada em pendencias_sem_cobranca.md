---
criado: 2026-08-23
origem: docs/Debbugs/Depurar pendencias_sem_cobranca esconde meses com cobranca falhada.md
status: concluido
tipo: correcao_regra_de_negocio
concluido: 2026-08-23
depende_de: docs/Tarefas feitas/62 - Corrigir N+1 de PendenciasSemCobranca em GET financeiro-cobrancas com ano_letivo.md
---

# `pendências_sem_cobranca` deve listar TODO mês não pago, não só os nunca tentados

## 0. Leia isto primeiro — sobre o seu ambiente (Codex)

Mesma situação da tarefa 62: você não tem `apt`, Docker nem `psql`. Não precisa disso aqui. Claude já validou esta correção inteira com PostgreSQL 16 e Go 1.24 reais, incluindo aplicar estes 3 arquivos sobre um clone **novo e limpo** de `main` (que já tem a tarefa 62) e rodar a suíte de testes do zero — 100% verde. Sua validação usa só `go build`, `go vet`, `gofmt` e `go test ./...` (os testes de integração pulam automaticamente sem `RUN_POSTGRES_INTEGRATION`, isso é esperado).

**Esta tarefa pressupõe que a tarefa 62 já foi aplicada** (ela já está em `docs/Tarefas feitas/` no momento em que este documento foi escrito). Os três arquivos abaixo são o conteúdo **completo e final**, já incorporando a tarefa 62 — se por algum motivo a 62 ainda não tiver sido aplicada, aplicar esta tarefa por si só já entrega as duas correções juntas (não precisa aplicar a 62 antes).

---

## 1. Prompt recomendado para executar esta correção

> Execute exatamente as alterações descritas neste documento. Todas as decisões já foram tomadas e validadas (causa raiz confirmada, correção implementada e testada com PostgreSQL 16 e Go 1.24 reais). Sua tarefa é mecânica: (1) substituir o conteúdo inteiro de `internal/finance/mensalidade_pendencias.go` pelo conteúdo exato da seção 3; (2) substituir o conteúdo inteiro de `internal/finance/mensalidade_pendencias_integration_test.go` pelo conteúdo exato da seção 4; (3) substituir o conteúdo inteiro de `internal/finance/mensalidade_pendencias_batch.go` pelo conteúdo exato da seção 5 (só um comentário mudou — a função em si é idêntica); (4) rodar cada item da seção "Checklist de validação" e reportar o resultado; (5) seguir o "Procedimento de conclusão". Não toque em nenhum outro arquivo. Não é necessário PostgreSQL, Docker nem `psql`.

---

## 2. Contexto

Depois da tarefa 62, Fredy testou o endpoint e reparou que `pendencias_sem_cobranca` vinha vazio para um mês (setembro) que continuava sem pagamento — o estudante tinha tentado pagar (uma cobrança GPO_QR cobrindo janeiro e setembro juntos) mas a tentativa **falhou**. `PendenciasSemCobranca` excluía esse mês porque, além do critério correto (`Estado != EstadoPendente`, vindo dos eventos de obrigação — pago/anulado), havia um segundo critério mais amplo: qualquer linha em `financeiro_mensalidade_cobrancas` (uma tabela de vínculo escrita a **cada evento do ciclo de vida** de uma tentativa — solicitada, criada, falhou, cancelada, etc., não só quando dá certo) também excluía o mês.

Decisão de produto confirmada por Fredy: `pendências_sem_cobranca` deve listar **todo** mês que ainda não foi pago (nem anulado) — tentativa falhada ou nenhuma tentativa, tanto faz. Ver `docs/Debbugs/Depurar pendencias_sem_cobranca esconde meses com cobranca falhada.md` para a análise completa e a reprodução com dados reais.

**Resumo da correção:** remove o segundo critério (a função `cobrancasExistentesMensalidade` inteira, e as duas chamadas a ela) de `PendenciasSemCobranca` e `PendenciasSemCobrancaEstudante`. O critério correto (`Estado != EstadoPendente`, vindo de `financeiro_mensalidade_obrigacoes_eventos` via `estadoObrigacao`/`estadosObrigacaoBatch`, ambas com comportamento inalterado) já estava certo e continua sendo o único critério de exclusão. `financeiro_mensalidade_cobrancas` continua existindo e sendo escrita normalmente — só deixa de ser consultada por estas duas funções; ela permanece a fonte de `chargeIDsEscopoMensalidade`, para um propósito diferente (vincular cobranças existentes ao escopo na listagem normal), que não muda. O terceiro arquivo (`mensalidade_pendencias_batch.go`) só perde um comentário que citava a função removida como referência de formato de chave — nenhum código muda ali.

---

## 3. `internal/finance/mensalidade_pendencias.go` — substituir conteúdo inteiro

Apague todo o conteúdo atual do arquivo e substitua exatamente pelo conteúdo abaixo:

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
// (nunca marcados como pagos nem anulados) para o conjunto de estudantes
// definido pelo escopo obrigatório informado (ver
// escopoMensalidadeEstudantes). É esta lista que resolve o problema de a
// academia não enxergar, em nenhuma consulta, a dívida de um estudante que
// ainda não pagou — hoje só o próprio estudante vê isso, via
// GET /financeiro/mensalidades/estudante/:codigo.
//
// ATENÇÃO — histórico do critério de exclusão (ver docs/Debbugs/ e
// docs/Lista de Tarefas/ da tarefa "GET /financeiro/cobrancas —
// pendências_sem_cobranca some meses com cobrança falhada"): esta função já
// excluiu, além dos meses com Estado != EstadoPendente (pago/anulado, a
// única fonte correta, vinda de financeiro_mensalidade_obrigacoes_eventos),
// qualquer mês que já tivesse QUALQUER linha em
// financeiro_mensalidade_cobrancas — uma tabela de vínculo escrita a cada
// evento do CICLO DE VIDA de uma cobrança (solicitada, criada, consultada,
// falhou, cancelada, QR gerado, QR falhou — ver upsertMensalidadeCobrancas
// em internal/projections/financeiro_projection.go), não só quando ela é
// paga. Isso escondia de "pendências sem cobrança" qualquer mês cuja única
// tentativa tivesse FALHADO (ex.: GPO_QR expirado, cartão recusado): o mês
// continuava por pagar, mas desaparecia de toda visão agregada da
// academia — só reaparecia se o estudante estivesse entre os poucos meses
// exibidos na listagem normal de cobranças (e, numa cobrança que agrupa
// vários meses numa única tentativa, um mês "escondido" nem sempre é óbvio
// de identificar ali). A decisão de produto (Fredy, 2026-08-23) foi listar
// tudo que ainda não foi pago, tentativa falhada ou não — o critério de
// exclusão passou a ser exclusivamente Estado != EstadoPendente.
// financeiro_mensalidade_cobrancas continua existindo e sendo escrita
// normalmente; só deixou de ser consultada por esta função (e por
// PendenciasSemCobrancaEstudante, o mesmo caminho para um único
// estudante) — ela permanece a fonte usada por chargeIDsEscopoMensalidade
// para vincular cobranças de mensalidade ao escopo na listagem normal
// (ListCobrancas), o que é um propósito diferente e não muda.
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
// pagou, sem exigir nenhum filtro extra do chamador.
//
// Até 2026-08-23 também excluía qualquer mês que já tivesse alguma
// tentativa de cobrança registrada (mesmo falhada) — ver o comentário
// histórico em PendenciasSemCobranca, que documenta por que esse critério
// foi removido em favor de Estado != EstadoPendente sozinho (a fonte
// correta, vinda dos eventos de obrigação já computados por
// ListMensalidades). ListMensalidades já devolve Estado corretamente
// calculado por mês; esta função só precisa filtrar por ele.
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
	return pendentes, nil
}
```

---

## 4. `internal/finance/mensalidade_pendencias_integration_test.go` — substituir conteúdo inteiro

Apague todo o conteúdo atual do arquivo e substitua exatamente pelo conteúdo abaixo. O teste `TestIntegrationPendenciasSemCobrancaExcluiQuandoJaExisteTentativa` (que verificava o comportamento ANTIGO, agora incorreto) foi removido e substituído por `TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma`, com a expectativa invertida (correta). Três testes novos protegem os critérios de exclusão que continuam válidos (pago, anulado/reativado, e o mesmo no caminho por estudante único). Todos os demais testes do arquivo (escopo obrigatório, filtro por mês, deduplicação de turma — da tarefa 62 — e os testes de `ListCobrancas`) continuam idênticos:

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
// registrada para o mês informado. Usada pelos testes de ListCobrancas
// (que filtram a listagem normal de cobranças por escopo/mês — inalterado
// por esta tarefa) e, nos testes de PendenciasSemCobranca, para comprovar
// que uma tentativa (mesmo com status "falhada") sozinha NÃO tira mais um
// mês de pendências_sem_cobranca — ver
// TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma.
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

// TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma
// cobre duas coisas juntas porque são a mesma regra de negócio vista de dois
// ângulos: um estudante que deve uma mensalidade continua aparecendo em
// pendências_sem_cobranca enquanto ela não for efetivamente PAGA (nem
// anulada) — não importa se ele nunca tentou nenhuma cobrança, ou se já
// tentou e a tentativa FALHOU.
//
// ESTPN01 nunca tentou nenhuma cobrança. ESTPN02 já tem uma cobrança
// FALHADA para setembro. Até 2026-08-23 este era o caso que
// PendenciasSemCobranca excluía (por engano — ver o comentário histórico em
// PendenciasSemCobranca): setembro do ESTPN02 desaparecia de toda visão
// agregada da academia mesmo continuando por pagar. Decisão de produto
// (Fredy, 2026-08-23): os dois devem aparecer igualmente — só uma cobrança
// bem-sucedida (ou uma anulação) tira um mês de pendências_sem_cobranca.
func TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PND-A", "2026_2027", "ESTPN01", nil)
	seedMensalidadeTurma(t, client, academia, "T-PND-B", "2026_2027", "ESTPN02", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	// ESTPN02 já tem uma tentativa de cobrança FALHADA para setembro — mas
	// ainda deve aparecer em pendências_sem_cobranca, porque continua sem
	// pagar.
	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTPN02", "falhada", "2026_2027", 9, 15000)

	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}

	achouEst1Setembro, achouEst2Setembro := false, false
	for _, m := range res {
		if m.CodigoEstudante == "ESTPN01" && m.Mes == 9 {
			achouEst1Setembro = true
			if m.Estado != EstadoPendente {
				t.Fatalf("ESTPN01/setembro: esperava estado pendente, obteve %q", m.Estado)
			}
		}
		if m.CodigoEstudante == "ESTPN02" && m.Mes == 9 {
			achouEst2Setembro = true
			if m.Estado != EstadoPendente {
				t.Fatalf("ESTPN02/setembro: esperava estado pendente, obteve %q", m.Estado)
			}
		}
	}
	if !achouEst1Setembro {
		t.Fatalf("ESTPN01/setembro nunca teve nenhuma cobrança; deveria aparecer em pendências_sem_cobranca. resultado: %#v", res)
	}
	if !achouEst2Setembro {
		t.Fatalf("ESTPN02/setembro já tentou (falhou) mas continua sem pagar; deveria aparecer em pendências_sem_cobranca mesmo assim. resultado: %#v", res)
	}
}

// TestIntegrationPendenciasSemCobrancaExcluiMesesPagos cobre o lado oposto
// do teste acima: um mês com um evento "paga" registrado (a fonte correta e
// única de exclusão, vinda de financeiro_mensalidade_obrigacoes_eventos) NÃO
// deve aparecer em pendências_sem_cobranca — mesmo que o mesmo estudante
// tenha outros meses pendentes.
func TestIntegrationPendenciasSemCobrancaExcluiMesesPagos(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PAGO-A", "2026_2027", "ESTPAGO1", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	if _, err := client.DB().Exec(`INSERT INTO financeiro_mensalidade_obrigacoes_eventos (event_id,aggregate_id,codigo_estudante,codigo_academia,ano_letivo,mes,tipo,ocorrido_em) VALUES ($1,$2,'ESTPAGO1',$3,'2026_2027',9,'paga',CURRENT_TIMESTAMP)`,
		uuid.New(), uuid.New(), academia); err != nil {
		t.Fatal(err)
	}

	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res {
		if m.CodigoEstudante == "ESTPAGO1" && m.Mes == 9 {
			t.Fatalf("ESTPAGO1/setembro já foi PAGO; não deveria aparecer em pendências_sem_cobranca: %#v", m)
		}
	}
	outrosMeses := 0
	for _, m := range res {
		if m.CodigoEstudante == "ESTPAGO1" {
			outrosMeses++
		}
	}
	if outrosMeses == 0 {
		t.Fatal("ESTPAGO1 deveria continuar com outros meses pendentes além de setembro (que já foi pago)")
	}
}

// TestIntegrationPendenciasSemCobrancaExcluiMesesAnuladosEIncluiReativados
// cobre o outro caso de exclusão legítima (Estado == EstadoAnulado) e
// confirma que reativar volta a listar o mês — usando
// AnularObrigacoesMensalidade/ReativarObrigacoesMensalidade (o caminho de
// comando real, não INSERT direto), porque este teste também serve de
// regressão para essas duas operações continuarem consistentes com
// PendenciasSemCobranca depois da mudança de critério de exclusão.
func TestIntegrationPendenciasSemCobrancaExcluiMesesAnuladosEIncluiReativados(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-ANUL-A", "2026_2027", "ESTANUL1", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	in := ObrigacaoMensalidadeInput{CodigoEstudante: "ESTANUL1", CodigoAcademia: academia, AnoLetivo: "2026_2027", Meses: []int{9}}
	if err := service.AnularObrigacoesMensalidade(ctx, in, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}

	res, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res {
		if m.CodigoEstudante == "ESTANUL1" && m.Mes == 9 {
			t.Fatalf("ESTANUL1/setembro foi ANULADO; não deveria aparecer em pendências_sem_cobranca: %#v", m)
		}
	}

	if err := service.ReativarObrigacoesMensalidade(ctx, in, uuid.NewString(), "academia", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	resReativado, err := service.PendenciasSemCobranca(ctx, academia, nil, nil, "", "2026_2027", nil)
	if err != nil {
		t.Fatal(err)
	}
	achouReativado := false
	for _, m := range resReativado {
		if m.CodigoEstudante == "ESTANUL1" && m.Mes == 9 {
			achouReativado = true
			if m.Estado != EstadoPendente {
				t.Fatalf("esperava estado pendente após reativação, obteve %q", m.Estado)
			}
		}
	}
	if !achouReativado {
		t.Fatal("ESTANUL1/setembro foi reativado; deveria voltar a aparecer em pendências_sem_cobranca")
	}
}

// TestIntegrationPendenciasSemCobrancaEstudanteIncluiMesComTentativaFalhada
// cobre a mesma mudança de critério (ver
// TestIntegrationPendenciasSemCobrancaIncluiMesesComTentativaFalhadaOuSemNenhuma),
// só que no caminho por estudante único (PendenciasSemCobrancaEstudante,
// usada por ConsultarCobrancasEstudante) — os dois caminhos precisam
// continuar consistentes entre si.
func TestIntegrationPendenciasSemCobrancaEstudanteIncluiMesComTentativaFalhada(t *testing.T) {
	client := integrationClient(t)
	service := NewService(client)
	ctx := context.Background()

	academia := mensalidadeCodigo()
	seedMensalidadeAcademia(t, client, academia, "private", "fundamental", "2026_2027")
	seedMensalidadeTurma(t, client, academia, "T-PNDE-B", "2026_2027", "ESTPNE04", nil)
	seedMensalidadeConfiguracao(t, client, academia, NivelFundamental, "7_ano_fundamental", nil, 15000, 7, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	seedFinanceiroCobrancaMensalidade(t, client, academia, "ESTPNE04", "falhada", "2026_2027", 9, 15000)

	res, err := service.PendenciasSemCobrancaEstudante(ctx, "ESTPNE04", &academia)
	if err != nil {
		t.Fatal(err)
	}
	achouSetembro := false
	for _, m := range res {
		if m.Mes == 9 {
			achouSetembro = true
		}
	}
	if !achouSetembro {
		t.Fatalf("ESTPNE04/setembro já tentou (falhou) mas continua sem pagar; deveria aparecer em pendências_sem_cobranca. resultado: %#v", res)
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

## 5. `internal/finance/mensalidade_pendencias_batch.go` — substituir conteúdo inteiro

Apague todo o conteúdo atual do arquivo e substitua exatamente pelo conteúdo abaixo. **Único ponto que muda:** o comentário de `estadosObrigacaoBatch` não cita mais `cobrancasExistentesMensalidade` (função removida por esta tarefa) como referência de formato de chave. A função `estadosObrigacaoBatch` em si, seu comportamento e sua assinatura são **idênticos** aos da tarefa 62 — nenhuma lógica muda neste arquivo:

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
// string via strconv.Itoa).
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

---

## 6. Fora de escopo (não altere)

- Qualquer outra função de `internal/finance/mensalidade_pendencias.go` além das listadas acima: `escopoMensalidadeEstudantes`, `chargeIDsEscopoMensalidade`, `mensalidadeEscopoVinculo` — nenhuma muda.
- `internal/finance/mensalidade.go` inteiro (`ListMensalidades`, `estadoObrigacao`, `precedenciaEstado`, `AnularObrigacoesMensalidade`, `ReativarObrigacoesMensalidade`, etc.) — não muda.
- `internal/finance/appypay.go` inteiro (`ListCobrancas`) — não muda. `financeiro_mensalidade_cobrancas` continua sendo escrita normalmente pela projeção (`internal/projections/financeiro_projection.go`) — não há nenhuma mudança de schema ou de escrita, só deixa de ser LIDA por `PendenciasSemCobranca`/`PendenciasSemCobrancaEstudante`.
- `internal/handlers/financeiro_handlers.go` — as assinaturas de `PendenciasSemCobranca` e `PendenciasSemCobrancaEstudante` não mudaram, nenhuma alteração necessária no handler.
- Qualquer arquivo do repositório `spuripainel` (frontend) — nenhuma alteração de frontend é necessária.
- Não invente nenhum critério adicional (ex.: distinguir cobrança "em andamento" de "falhada definitivamente") — a decisão de produto foi explícita: listar tudo que não está pago nem anulado, sem meio-termo.

---

## 7. Checklist de validação (Codex deve executar e reportar o resultado de cada item)

Nenhum destes comandos requer PostgreSQL, Docker ou `psql`:

1. `grep -rn "cobrancasExistentesMensalidade" --include="*.go" .` — deve retornar **vazio** depois de aplicar a mudança (a função, as duas chamadas, e o comentário que a citava foram todos removidos; nenhuma outra parte do repositório a usa).
2. `go build ./...` — sem erros.
3. `go vet ./...` — sem erros.
4. `gofmt -l internal/finance/mensalidade_pendencias.go internal/finance/mensalidade_pendencias_integration_test.go internal/finance/mensalidade_pendencias_batch.go` — vazio.
5. `go test ./...` — sem falhas (testes de integração aparecem como `SKIP`, não `FAIL`, sem `RUN_POSTGRES_INTEGRATION` — esperado).
6. `git diff --stat` — alterações apenas nos 3 arquivos das seções 3, 4 e 5, mais os documentos de conclusão.

Se qualquer item falhar, não prossiga — reporte o erro exato.

---

## 8. Critérios de aceite

- [ ] `internal/finance/mensalidade_pendencias.go` substituído exatamente pelo conteúdo da seção 3.
- [ ] `internal/finance/mensalidade_pendencias_integration_test.go` substituído exatamente pelo conteúdo da seção 4.
- [ ] `internal/finance/mensalidade_pendencias_batch.go` substituído exatamente pelo conteúdo da seção 5.
- [ ] Todos os 6 itens do checklist executados e reportados com sucesso.
- [ ] Nenhum arquivo fora do escopo desta tarefa foi alterado (seção 6).

---

## 9. Procedimento de conclusão

1. Mover este arquivo para `docs/Tarefas feitas/`, com `status: concluido` e `concluido: <data de hoje>` no frontmatter (seguindo a numeração já usada no diretório — o próximo número disponível no momento em que este documento foi escrito é 63).
2. Atualizar `docs/Debbugs/Depurar pendencias_sem_cobranca esconde meses com cobranca falhada.md`, campo `status`, para `corrigido_via_63_...` (nome real do arquivo desta tarefa após movido).
3. Um commit único, mensagem: `pendencias_sem_cobranca lista meses com tentativa falhada, nao so os nunca tentados`.
4. Reportar a Fredy: resultado de cada item do checklist e `git diff --stat` do commit. Nenhuma validação adicional com PostgreSQL real é necessária — já foi feita.

**Nenhuma etapa remove ou altera qualquer código relacionado à inscrição de estudantes em academias** — as alterações estão contidas ao módulo financeiro de mensalidades (`internal/finance/mensalidade_pendencias*.go`), sem tocar em matrícula, cadastro, turmas ou vínculo de estudante à academia.
