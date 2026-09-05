package finance

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"errors"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
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
		CodigoAcademia:             academia,
		MetodoPagamento:            in.MetodoPagamento,
		Amount:                     valor.Float64,
		Description:                "Taxa de inscrição - serviço extra " + academia,
		MerchantTransactionID:      merchantID(),
		Telefone:                   in.Telefone,
		CodigoInscricaoServico:     in.SolicitacaoID,
		TipoLancamentoServicoExtra: "taxa_inscricao",
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

// DadosServicoExtraDaCobranca devolve a inscrição e o lançamento associado à cobrança.
func (s *Service) DadosServicoExtraDaCobranca(ctx context.Context, identifier string) (codigoInscricao, tipoLancamento string, mes, ano int, err error) {
	row, err := s.loadCharge(ctx, identifier)
	if err != nil {
		return "", "", 0, 0, err
	}
	codigoInscricao, _ = row.Payload["codigo_inscricao_servico"].(string)
	tipoLancamento, _ = row.Payload["tipo_lancamento_servico_extra"].(string)
	if m, ok := row.Payload["mes_referencia"].(float64); ok {
		mes = int(m)
	}
	if a, ok := row.Payload["ano_referencia"].(float64); ok {
		ano = int(a)
	}
	return strings.TrimSpace(codigoInscricao), tipoLancamento, mes, ano, nil
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

func (s *Service) recordServicoExtraObrigacao(ctx context.Context, solicitacaoID, event string, payload map[string]any, userID, userType, ip string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("autor do evento financeiro é obrigatório")
	}
	agg := aggregates.NewFinanceiroWithID(servicoExtraObrigacaoAggregateID(solicitacaoID))
	agg.Registrar(event, payload)
	if err := s.repository.WithContext(ctx).SaveWithAudit(agg, db.AuditContext{UserID: userID, UserType: userType, IP: ip}); err != nil {
		return err
	}
	return s.projection.ApplyLatestForAggregate(agg.ID)
}
func servicoExtraObrigacaoAggregateID(solicitacaoID string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("spuri:servico_extra_obrigacao:"+strings.ToLower(strings.TrimSpace(solicitacaoID))))
}
func (s *Service) estadoObrigacaoServicoExtra(ctx context.Context, solicitacaoID, tipo string, ano, mes int) (string, error) {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT tipo FROM financeiro_servico_extra_obrigacoes_eventos WHERE solicitacao_id=$1 AND tipo_lancamento=$2 AND ano IS NOT DISTINCT FROM NULLIF($3,0) AND mes IS NOT DISTINCT FROM NULLIF($4,0) ORDER BY ocorrido_em, id`, solicitacaoID, tipo, ano, mes)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var eventos []string
	for rows.Next() {
		var typ string
		if err := rows.Scan(&typ); err != nil {
			return "", err
		}
		eventos = append(eventos, typ)
	}
	return precedenciaEstado(eventos), rows.Err()
}

type ServicoExtraPendenciaView struct {
	TipoLancamento string  `json:"tipo_lancamento"`
	Ano            int     `json:"ano,omitempty"`
	Mes            int     `json:"mes,omitempty"`
	Estado         string  `json:"estado"`
	Valor          float64 `json:"valor"`
}

func (s *Service) PendenciasServicoExtra(ctx context.Context, solicitacaoID, tipoCobranca string, preco float64, vinculadaEm, fimPeriodo time.Time) ([]ServicoExtraPendenciaView, error) {
	if tipoCobranca == "unico" {
		estado, err := s.estadoObrigacaoServicoExtra(ctx, solicitacaoID, "preco_unico", 0, 0)
		if err != nil {
			return nil, err
		}
		return []ServicoExtraPendenciaView{{TipoLancamento: "preco_unico", Estado: estado, Valor: preco}}, nil
	}
	var out []ServicoExtraPendenciaView
	cursor := time.Date(vinculadaEm.Year(), vinculadaEm.Month(), 1, 0, 0, 0, 0, time.UTC)
	limite := time.Date(fimPeriodo.Year(), fimPeriodo.Month(), 1, 0, 0, 0, 0, time.UTC)
	for !cursor.After(limite) {
		estado, err := s.estadoObrigacaoServicoExtra(ctx, solicitacaoID, "mensalidade", cursor.Year(), int(cursor.Month()))
		if err != nil {
			return nil, err
		}
		out = append(out, ServicoExtraPendenciaView{TipoLancamento: "mensalidade", Ano: cursor.Year(), Mes: int(cursor.Month()), Estado: estado, Valor: preco})
		cursor = cursor.AddDate(0, 1, 0)
	}
	return out, nil
}

type ServicoExtraObrigacaoPagamentoInput struct {
	SolicitacaoID   string `json:"-"`
	TipoLancamento  string `json:"tipo_lancamento"`
	Ano             int    `json:"ano,omitempty"`
	Mes             int    `json:"mes,omitempty"`
	MetodoPagamento string `json:"metodo_pagamento"`
	Telefone        string `json:"telefone,omitempty"`
}

func (s *Service) IniciarPagamentoServicoExtraObrigacao(ctx context.Context, in ServicoExtraObrigacaoPagamentoInput, codigoAcademia string, preco float64, metodos []string, ip string) (QRCodeResult, error) {
	in.MetodoPagamento = strings.ToUpper(strings.TrimSpace(in.MetodoPagamento))
	if in.TipoLancamento != "mensalidade" && in.TipoLancamento != "preco_unico" {
		return QRCodeResult{}, errors.New("tipo_lancamento deve ser 'mensalidade' ou 'preco_unico'")
	}
	if in.TipoLancamento == "mensalidade" && (in.Mes < 1 || in.Mes > 12 || in.Ano < 2000) {
		return QRCodeResult{}, errors.New("ano e mes válidos são obrigatórios para mensalidade")
	}
	if !contains(metodos, in.MetodoPagamento) {
		return QRCodeResult{}, errors.New("método de pagamento não está habilitado para este serviço")
	}
	estado, err := s.estadoObrigacaoServicoExtra(ctx, in.SolicitacaoID, in.TipoLancamento, in.Ano, in.Mes)
	if err != nil {
		return QRCodeResult{}, err
	}
	if estado == EstadoPago {
		return QRCodeResult{}, errors.New("este lançamento já está pago")
	}
	if estado == EstadoAnulado {
		return QRCodeResult{}, errors.New("este lançamento foi anulado pela academia")
	}
	open, err := s.servicoExtraObrigacaoTemCobrancaAberta(ctx, in.SolicitacaoID, in.TipoLancamento, in.Ano, in.Mes)
	if err != nil {
		return QRCodeResult{}, err
	}
	if open {
		return QRCodeResult{}, errors.New("já existe cobrança em aberto para este lançamento")
	}
	var mr, ar *int
	desc := "Serviço extra " + codigoAcademia
	if in.TipoLancamento == "mensalidade" {
		mr, ar = &in.Mes, &in.Ano
		desc = fmt.Sprintf("Serviço extra %s - %02d/%d", codigoAcademia, in.Mes, in.Ano)
	}
	return s.gerarCobranca(ctx, gerarCobrancaInput{CodigoAcademia: codigoAcademia, MetodoPagamento: in.MetodoPagamento, Amount: preco, Description: desc, MerchantTransactionID: merchantID(), Telefone: in.Telefone, CodigoInscricaoServico: in.SolicitacaoID, TipoLancamentoServicoExtra: in.TipoLancamento, MesReferencia: mr, AnoReferencia: ar}, "solicitacao_servico_extra:"+in.SolicitacaoID, "solicitante", ip)
}
func (s *Service) servicoExtraObrigacaoTemCobrancaAberta(ctx context.Context, id, tipo string, ano, mes int) (bool, error) {
	var ok bool
	err := s.client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM financeiro_cobrancas WHERE payload->>'codigo_inscricao_servico'=$1 AND payload->>'tipo_lancamento_servico_extra'=$2 AND COALESCE((payload->>'ano_referencia')::int,0)=$3 AND COALESCE((payload->>'mes_referencia')::int,0)=$4 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`))`, id, tipo, ano, mes).Scan(&ok)
	return ok, err
}
func (s *Service) ConfirmarLancamentoServicoExtraPago(ctx context.Context, id, tipo string, ano, mes int, actor, actorType, ip string) error {
	payload := map[string]any{"solicitacao_id": id, "tipo_lancamento": tipo}
	if tipo == "mensalidade" {
		payload["ano"], payload["mes"] = ano, mes
	}
	return s.recordServicoExtraObrigacao(ctx, id, aggregates.ServicoExtraLancamentoPago, payload, actor, actorType, ip)
}
func (s *Service) AnularObrigacaoServicoExtra(ctx context.Context, id, tipo string, ano, mes int, motivo, actor, actorType, ip string) error {
	return s.alterarObrigacaoServicoExtra(ctx, id, tipo, ano, mes, motivo, actor, actorType, ip, true)
}
func (s *Service) ReativarObrigacaoServicoExtra(ctx context.Context, id, tipo string, ano, mes int, motivo, actor, actorType, ip string) error {
	return s.alterarObrigacaoServicoExtra(ctx, id, tipo, ano, mes, motivo, actor, actorType, ip, false)
}
func (s *Service) alterarObrigacaoServicoExtra(ctx context.Context, id, tipo string, ano, mes int, motivo, actor, actorType, ip string, anular bool) error {
	estado, err := s.estadoObrigacaoServicoExtra(ctx, id, tipo, ano, mes)
	if err != nil {
		return err
	}
	if anular && estado == EstadoPago {
		return errors.New("não é possível anular um lançamento já pago")
	}
	if !anular && estado != EstadoAnulado {
		return errors.New("só é possível reativar um lançamento anulado e não pago")
	}
	payload := map[string]any{"solicitacao_id": id, "tipo_lancamento": tipo, "motivo": strings.TrimSpace(motivo)}
	if tipo == "mensalidade" {
		payload["ano"], payload["mes"] = ano, mes
	}
	event := aggregates.ObrigacaoServicoExtraReativada
	if anular {
		event = aggregates.ObrigacaoServicoExtraAnulada
	}
	if err = s.recordServicoExtraObrigacao(ctx, id, event, payload, actor, actorType, ip); err != nil {
		return err
	}
	if anular {
		return s.servicoExtraObrigacaoCancelarCobrancasAbertas(ctx, id, tipo, ano, mes, actor, actorType, ip)
	}
	return nil
}
func (s *Service) servicoExtraObrigacaoCancelarCobrancasAbertas(ctx context.Context, id, tipo string, ano, mes int, actor, actorType, ip string) error {
	rows, err := s.client.DB().QueryContext(ctx, `SELECT id::text,codigo_academia FROM financeiro_cobrancas WHERE payload->>'codigo_inscricao_servico'=$1 AND payload->>'tipo_lancamento_servico_extra'=$2 AND COALESCE((payload->>'ano_referencia')::int,0)=$3 AND COALESCE((payload->>'mes_referencia')::int,0)=$4 AND lower(COALESCE(payload->>'status','')) NOT IN (`+chargeAbertaStatusExcluidos+`)`, id, tipo, ano, mes)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var charge, academia string
		if err = rows.Scan(&charge, &academia); err != nil {
			return err
		}
		if _, err = s.CancelCharge(ctx, ContextoAcademia, academia, charge, "obrigação de serviço extra anulada pela academia", actor, actorType, ip); err != nil {
			return err
		}
	}
	return rows.Err()
}
