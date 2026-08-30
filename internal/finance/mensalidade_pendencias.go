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
			chaveMes := v.CodigoEstudante + "|" + v.AnoLetivo + "|" + strconv.Itoa(ref.Month)
			estado := EstadoPendente
			var audit []uuid.UUID
			if info, ok := estados[chaveMes]; ok {
				estado, audit = info.Estado, info.Audit
			}
			if estado != EstadoPendente {
				continue
			}
			chaveCfg := v.CodigoAcademia + "|" + v.Nivel + "|" + v.AnoAcademico + "|" + optionalUUID(v.CursoID) + "|" + ref.Data.Format("2006-01")
			cfg, temCfg := cfgCache[chaveCfg]
			if !temCfg {
				if cfgNaoEncontrada[chaveCfg] {
					continue
				}
				cfg, err = s.resolveConfiguracaoEfetiva(ctx, v.CodigoAcademia, v.Nivel, v.AnoAcademico, v.CursoID, ref.Data, true)
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
