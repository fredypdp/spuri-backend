package finance

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestMensalidadePagamentoViewIncludesQRCodeArr reproduz o Problema 2
// documentado em
// "docs/Lista de Tarefas/Problemas de Backend - Modulo de Pagamentos.md":
// antes da correção, Charge era ChargeResult (sem QRCodeArr), então o campo
// nunca sobrevivia à serialização JSON da resposta de pagamento de
// mensalidade, mesmo quando a AppyPay devolvia o QR Code corretamente.
func TestMensalidadePagamentoViewIncludesQRCodeArr(t *testing.T) {
	view := MensalidadePagamentoView{
		Charge: QRCodeResult{
			ChargeResult: ChargeResult{ID: uuid.New(), MerchantTransactionID: "M1", Status: "criada"},
			QRCodeArr:    "base64-qr-mensalidade",
		},
		Meses: []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 3}},
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	cobranca, ok := decoded["cobranca"].(map[string]any)
	if !ok {
		t.Fatalf("campo cobranca ausente ou com formato inesperado: %s", raw)
	}
	if cobranca["qrCodeArr"] != "base64-qr-mensalidade" {
		t.Fatalf("qrCodeArr não chegou na resposta de pagamento de mensalidade: %s", raw)
	}
}

// TestMatriculaPagamentoViewIncludesQRCodeArr é o equivalente de
// TestMensalidadePagamentoViewIncludesQRCodeArr para o fluxo de matrícula.
func TestMatriculaPagamentoViewIncludesQRCodeArr(t *testing.T) {
	view := MatriculaPagamentoView{
		Charge: QRCodeResult{
			ChargeResult: ChargeResult{ID: uuid.New(), MerchantTransactionID: "M2", Status: "criada"},
			QRCodeArr:    "base64-qr-matricula",
		},
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	cobranca, ok := decoded["cobranca"].(map[string]any)
	if !ok {
		t.Fatalf("campo cobranca ausente ou com formato inesperado: %s", raw)
	}
	if cobranca["qrCodeArr"] != "base64-qr-matricula" {
		t.Fatalf("qrCodeArr não chegou na resposta de pagamento de matrícula: %s", raw)
	}
}

// TestMensalidadePagamentoViewOmiteQRCodeArrParaOutrosMetodos garante que a
// correção não introduz o campo qrCodeArr (mesmo vazio) para métodos que não
// são GPO_QR — QRCodeResult tem omitempty em QRCodeArr especificamente para
// isso.
func TestMensalidadePagamentoViewOmiteQRCodeArrParaOutrosMetodos(t *testing.T) {
	view := MensalidadePagamentoView{
		Charge: QRCodeResult{ChargeResult: ChargeResult{ID: uuid.New(), MerchantTransactionID: "M3", Status: "criada"}},
		Meses:  []MensalidadeSelecaoMes{{AnoLetivo: "2026_2027", Mes: 3}},
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	cobranca, ok := decoded["cobranca"].(map[string]any)
	if !ok {
		t.Fatalf("campo cobranca ausente ou com formato inesperado: %s", raw)
	}
	if _, present := cobranca["qrCodeArr"]; present {
		t.Fatalf("qrCodeArr não deveria aparecer no JSON quando vazio (omitempty): %s", raw)
	}
}
