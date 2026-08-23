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
