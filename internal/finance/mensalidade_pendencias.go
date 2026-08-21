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
