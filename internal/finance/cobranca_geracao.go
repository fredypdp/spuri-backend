package finance

import (
	"context"
	"strings"
)

// gerarCobrancaInput agrupa os parâmetros necessários para emitir uma nova
// cobrança em nome de uma academia (contexto sempre ContextoAcademia), a
// partir de uma obrigação já validada por quem chama (mensalidade ou taxa
// de matrícula). CodigoEstudante, CodigoSolicitacao e Mensalidades são
// metadados de auditoria opcionais: preencha apenas o(s) que fizer(em)
// sentido para a origem da cobrança (CodigoEstudante para mensalidade,
// CodigoSolicitacao para matrícula) e deixe os demais no valor zero —
// ChargeRequest e QRCodeRequest já tratam esses campos como omitempty e
// eles nunca são enviados à AppyPay (ver comentário de ChargeRequest em
// appypay.go).
type gerarCobrancaInput struct {
	CodigoAcademia         string
	MetodoPagamento        string // "REF", "GPO" ou "GPO_QR" — já normalizado (TrimSpace+ToUpper) pelo chamador
	Amount                 float64
	Description            string
	MerchantTransactionID  string
	Telefone               string // usado apenas quando MetodoPagamento == "GPO"; ignorado nos demais casos
	CodigoEstudante        string
	CodigoSolicitacao      string
	CodigoInscricaoServico string
	Mensalidades           []MensalidadeSelecaoMes
}

// gerarCobranca é a única função do módulo financeiro que decide, a partir
// de um método de pagamento aceite pelo sistema (REF, GPO ou GPO_QR), qual
// das duas funções que efetivamente falam com a AppyPay chamar —
// CreateGPOQRCode (GPO_QR) ou CreateCharge (REF e GPO) — e monta o
// paymentInfo.phoneNumber exigido pela AppyPay quando o método é GPO.
//
// É reutilizada tanto por IniciarPagamentoMatricula (matricula.go) quanto
// por IniciarPagamentoMensalidades (mensalidade.go): as duas únicas
// funções do sistema que iniciam uma cobrança nova a partir de uma
// obrigação (matrícula ou mensalidade) e de um método de pagamento
// escolhido pelo pagador. Nenhum outro lugar do módulo deve reimplementar
// esta decisão — ver o comentário de pacote no topo de appypay.go
// ("Package finance is the only package allowed to call AppyPay's HTTP
// API").
//
// Endpoints administrativos que criam uma cobrança "avulsa" com controlo
// total sobre o payload (CriarCobrancaAppyPay, GerarQRCodeAppyPay em
// internal/handlers/financeiro_handlers.go) continuam, deliberadamente,
// chamando CreateCharge/CreateGPOQRCode diretamente: não partem de uma
// obrigação nem de um método simples REF/GPO/GPO_QR escolhido pelo
// pagador, então esta função não se aplica a eles.
//
// O retorno é sempre QRCodeResult (mesmo para REF/GPO, com QRCodeArr
// vazio), para que o chamador nunca precise tratar dois tipos de retorno
// diferentes — o mesmo motivo pelo qual MatriculaPagamentoView.Charge e
// MensalidadePagamentoView.Charge já são declarados como QRCodeResult.
func (s *Service) gerarCobranca(ctx context.Context, in gerarCobrancaInput, actorID, actorType, ip string) (QRCodeResult, error) {
	if in.MetodoPagamento == "GPO_QR" {
		qr, err := s.CreateGPOQRCode(ctx, QRCodeRequest{
			ContextoTipo:           ContextoAcademia,
			CodigoAcademia:         in.CodigoAcademia,
			CodigoEstudante:        in.CodigoEstudante,
			CodigoSolicitacao:      in.CodigoSolicitacao,
			CodigoInscricaoServico: in.CodigoInscricaoServico,
			Amount:                 in.Amount,
			Currency:               "AOA",
			Description:            in.Description,
			MerchantTransactionID:  in.MerchantTransactionID,
			Mensalidades:           in.Mensalidades,
		}, actorID, actorType, ip)
		if err != nil {
			return QRCodeResult{}, err
		}
		return qr, nil
	}
	info := map[string]any{}
	if in.MetodoPagamento == "GPO" {
		info["phoneNumber"] = strings.TrimSpace(in.Telefone)
	}
	charge, err := s.CreateCharge(ctx, ChargeRequest{
		ContextoTipo:           ContextoAcademia,
		CodigoAcademia:         in.CodigoAcademia,
		CodigoEstudante:        in.CodigoEstudante,
		CodigoSolicitacao:      in.CodigoSolicitacao,
		CodigoInscricaoServico: in.CodigoInscricaoServico,
		Mensalidades:           in.Mensalidades,
		Amount:                 in.Amount,
		Currency:               "AOA",
		Description:            in.Description,
		MerchantTransactionID:  in.MerchantTransactionID,
		PaymentMethod:          in.MetodoPagamento,
		PaymentInfo:            info,
	}, actorID, actorType, ip)
	if err != nil {
		return QRCodeResult{}, err
	}
	return QRCodeResult{ChargeResult: charge}, nil
}
