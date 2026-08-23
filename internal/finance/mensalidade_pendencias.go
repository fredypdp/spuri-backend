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
