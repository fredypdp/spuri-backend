package finance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const (
	ModoVigenciaAPartirDaAtualizacao = "a_partir_da_atualizacao"
	ModoVigenciaCobrancasPendentes   = "cobrancas_pendentes"
)

func modoVigenciaValido(v string) bool {
	return v == ModoVigenciaAPartirDaAtualizacao || v == ModoVigenciaCobrancasPendentes
}

func (s *Service) ultimaConfiguracaoMensalidade(ctx context.Context, academia, nivel, ano string, curso *uuid.UUID, agora time.Time) (view MensalidadeConfiguracaoView, removida bool, err error) {
	var cursoText sql.NullString
	err = s.client.DB().QueryRowContext(ctx, `SELECT curso_id,valor::float8,mes_fim_cobranca,metodos_pagamento,vigente_em,modo_vigencia
		FROM financeiro_mensalidade_configuracoes
		WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4
		ORDER BY sequencia DESC LIMIT 1`, academia, nivel, ano, nullableUUID(curso)).Scan(&cursoText, &view.Valor, &view.MesFimCobranca, pq.Array(&view.MetodosPagamento), &view.VigenteEm, &view.ModoVigencia)
	if err == sql.ErrNoRows {
		return MensalidadeConfiguracaoView{}, false, fmt.Errorf("%w: configuração de mensalidade", ErrNotFound)
	}
	if err != nil {
		return MensalidadeConfiguracaoView{}, false, err
	}
	view.CodigoAcademia, view.Nivel, view.AnoAcademico = academia, nivel, ano
	if cursoText.Valid {
		id, e := uuid.Parse(cursoText.String)
		if e != nil {
			return MensalidadeConfiguracaoView{}, false, e
		}
		view.CursoID = &id
	}
	var removidoEm time.Time
	errRem := s.client.DB().QueryRowContext(ctx, `SELECT removido_em FROM financeiro_mensalidade_configuracoes_remocoes
		WHERE codigo_academia=$1 AND nivel=$2 AND ano_academico=$3 AND curso_id IS NOT DISTINCT FROM $4
		AND removido_em >= $5 AND removido_em <= $6 ORDER BY removido_em DESC LIMIT 1`, academia, nivel, ano, nullableUUID(curso), view.VigenteEm, agora.UTC()).Scan(&removidoEm)
	if errRem == nil {
		return view, true, nil
	}
	if errRem != sql.ErrNoRows {
		return view, false, errRem
	}
	return view, false, nil
}

func (s *Service) resolveConfiguracaoEfetiva(ctx context.Context, academia, nivel, ano string, curso *uuid.UUID, referencia time.Time, pendente bool) (MensalidadeConfiguracaoView, error) {
	if pendente {
		ultima, removida, err := s.ultimaConfiguracaoMensalidade(ctx, academia, nivel, ano, curso, time.Now())
		if err != nil && !errors.Is(err, ErrNotFound) {
			return MensalidadeConfiguracaoView{}, err
		}
		if err == nil {
			if removida {
				return MensalidadeConfiguracaoView{}, fmt.Errorf("%w: configuração de mensalidade removida", ErrNotFound)
			}
			if ultima.ModoVigencia == ModoVigenciaCobrancasPendentes {
				return ultima, nil
			}
		}
	}
	return s.resolveConfiguracao(ctx, academia, nivel, ano, curso, referencia)
}
