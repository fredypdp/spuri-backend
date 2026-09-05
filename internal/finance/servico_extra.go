package finance

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/lib/pq"
)

type TaxaInscricaoServicoExtraPagamentoInput struct {
	SolicitacaoID   string `json:"-"` // preenchido pelo handler a partir do path/identidade, nunca do body do cliente
	MetodoPagamento string `json:"metodo_pagamento"`
	Telefone        string `json:"telefone,omitempty"`
}

type TaxaInscricaoServicoExtraPagamentoView struct {
	Charge QRCodeResult `json:"cobranca"`
}

// IniciarPagamentoTaxaInscricaoServicoExtra inicia a cobrança da taxa de
// inscrição de uma SolicitacaoServicoExtra já aprovada e aguardando
// pagamento. Mirror direto de IniciarPagamentoMatricula — mesma validação de
// estado, mesma checagem de cobrança aberta duplicada, mesmo uso de
// gerarCobranca.
func (s *Service) IniciarPagamentoTaxaInscricaoServicoExtra(ctx context.Context, in TaxaInscricaoServicoExtraPagamentoInput, ip string) (TaxaInscricaoServicoExtraPagamentoView, error) {
	in.SolicitacaoID = strings.TrimSpace(in.SolicitacaoID)
	in.MetodoPagamento = strings.ToUpper(strings.TrimSpace(in.MetodoPagamento))
	if in.SolicitacaoID == "" || in.MetodoPagamento == "" {
		return TaxaInscricaoServicoExtraPagamentoView{}, errors.New("solicitação e método de pagamento são obrigatórios")
	}
	var academia, status string
	var valor sql.NullFloat64
	var metodos []string
	err := s.client.DB().QueryRowContext(ctx, `SELECT codigo_academia,status,valor_taxa_inscricao::float8,metodos_pagamento_taxa_inscricao FROM projection_solicitacoes_servico_extra WHERE id=$1::uuid`, in.SolicitacaoID).
		Scan(&academia, &status, &valor, pq.Array(&metodos))
	if err != nil || status != "aprovada_pendente_pagamento_taxa_inscricao" || !valor.Valid {
		return TaxaInscricaoServicoExtraPagamentoView{}, errors.New("solicitação não disponível para pagamento de taxa de inscrição")
	}
	if !contains(metodos, in.MetodoPagamento) {
		return TaxaInscricaoServicoExtraPagamentoView{}, errors.New("método de pagamento não está habilitado para esta taxa de inscrição")
	}
	open, err := s.servicoExtraTemCobrancaAberta(ctx, in.SolicitacaoID)
	if err != nil {
		return TaxaInscricaoServicoExtraPagamentoView{}, err
	}
	if open {
		return TaxaInscricaoServicoExtraPagamentoView{}, errors.New("solicitação já possui cobrança de taxa de inscrição em aberto")
	}
	result, err := s.gerarCobranca(ctx, gerarCobrancaInput{
		CodigoAcademia:         academia,
		MetodoPagamento:        in.MetodoPagamento,
		Amount:                 valor.Float64,
		Description:            "Taxa de inscrição - serviço extra " + academia,
		MerchantTransactionID:  merchantID(),
		Telefone:               in.Telefone,
		CodigoInscricaoServico: in.SolicitacaoID,
	}, "solicitacao_servico_extra:"+in.SolicitacaoID, "solicitante", ip)
	if err != nil {
		return TaxaInscricaoServicoExtraPagamentoView{}, err
	}
	return TaxaInscricaoServicoExtraPagamentoView{Charge: result}, nil
}

func (s *Service) servicoExtraTemCobrancaAberta(ctx context.Context, solicitacaoID string) (bool, error) {
	var ok bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_cobrancas WHERE payload->>'codigo_inscricao_servico'=$1 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`))`, solicitacaoID).Scan(&ok)
	return ok, err
}

// CodigoInscricaoServicoExtraDaCobranca identifica a solicitação de serviço
// extra associada a uma cobrança, sem expor detalhes do estudante ao código
// que fala com a AppyPay. Mirror de CodigoSolicitacaoDaCobranca.
func (s *Service) CodigoInscricaoServicoExtraDaCobranca(ctx context.Context, identifier string) (string, error) {
	row, err := s.loadCharge(ctx, identifier)
	if err != nil {
		return "", err
	}
	codigo, _ := row.Payload["codigo_inscricao_servico"].(string)
	return strings.TrimSpace(codigo), nil
}

// CancelarCobrancaTaxaInscricaoServicoAberta cancela qualquer cobrança em
// aberto da taxa de inscrição antes de a solicitação ser cancelada
// (CancelarAntesDaVinculacao). Mirror de CancelarCobrancaMatriculaAberta.
func (s *Service) CancelarCobrancaTaxaInscricaoServicoAberta(ctx context.Context, solicitacaoID, motivo, actorID, actorType, ip string) error {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id::text,codigo_academia FROM financeiro_cobrancas WHERE payload->>'codigo_inscricao_servico'=$1 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)`, solicitacaoID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, academia string
		if err := rows.Scan(&id, &academia); err != nil {
			return err
		}
		if _, err := s.CancelCharge(ctx, ContextoAcademia, academia, id, motivo, actorID, actorType, ip); err != nil {
			return err
		}
	}
	return rows.Err()
}
