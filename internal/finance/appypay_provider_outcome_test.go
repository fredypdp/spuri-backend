package finance

import "testing"

func TestExtractProviderOutcomePostShapeGPOCancelledCodes(t *testing.T) {
	cases := []struct {
		nome          string
		code          float64
		wantCategoria string
	}{
		{"saldo insuficiente", 209, "saldo_insuficiente"},
		{"timeout do processador", 210, "tempo_esgotado"},
		{"timeout da transação", 211, "tempo_esgotado"},
		{"recusado pelo cliente", 231, "recusado_pelo_cliente"},
		{"recusado pelo processador", 200, "recusado_pelo_processador"},
		{"recusado pelo emissor", 201, "recusado_pelo_emissor"},
	}
	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			response := map[string]any{"id": "provider-1", "responseStatus": map[string]any{"successful": false, "status": "Failed", "code": tc.code, "message": "mensagem de teste", "source": "GPO"}}
			outcome := extractProviderOutcome(response)
			if outcome.Status != "Cancelled" {
				t.Fatalf("Status = %q, queria Cancelled (código %v)", outcome.Status, tc.code)
			}
			if outcome.Categoria != tc.wantCategoria {
				t.Fatalf("Categoria = %q, queria %q", outcome.Categoria, tc.wantCategoria)
			}
			if !outcome.HasCode || outcome.Code != int(tc.code) {
				t.Fatalf("Code = %d (HasCode=%t), queria %v", outcome.Code, outcome.HasCode, tc.code)
			}
			if outcome.Message != "mensagem de teste" || outcome.Source != "GPO" {
				t.Fatalf("Message/Source não preservados: %q/%q", outcome.Message, outcome.Source)
			}
			if got := normalizeChargeStatus(outcome.Status); !isTerminalChargeStatus(got) {
				t.Fatalf("normalizeChargeStatus(%q) = %q não é reconhecido como terminal", outcome.Status, got)
			}
		})
	}
}

func TestExtractProviderOutcomeRefExpired(t *testing.T) {
	response := map[string]any{"id": "provider-ref-1", "responseStatus": map[string]any{"successful": false, "status": "Failed", "code": float64(245), "message": "The payment has expired", "source": "REF"}}
	outcome := extractProviderOutcome(response)
	if outcome.Status != "Expired" {
		t.Fatalf("Status = %q, queria Expired", outcome.Status)
	}
	if outcome.Categoria != "referencia_expirada" {
		t.Fatalf("Categoria = %q, queria referencia_expirada", outcome.Categoria)
	}
}

func TestExtractProviderOutcomeGetChargeShape(t *testing.T) {
	response := map[string]any{"payment": map[string]any{"id": "provider-get-1", "status": "Failed", "transactionEvents": []any{map[string]any{"id": float64(1), "responseStatus": map[string]any{"successful": false, "status": "Failed", "code": float64(231), "message": "Payment not authorized by the customer", "source": "UMM"}}}}}
	outcome := extractProviderOutcome(response)
	if outcome.Status != "Cancelled" {
		t.Fatalf("Status = %q, queria Cancelled", outcome.Status)
	}
	if outcome.Categoria != "recusado_pelo_cliente" {
		t.Fatalf("Categoria = %q, queria recusado_pelo_cliente", outcome.Categoria)
	}
	if outcome.Code != 231 {
		t.Fatalf("Code = %d, queria 231", outcome.Code)
	}
	if id := responseID(response); id != "provider-get-1" {
		t.Fatalf("responseID(payment aninhado) = %q, queria provider-get-1", id)
	}
}

func TestExtractProviderOutcomeGetChargeMultipleEventsUsesLast(t *testing.T) {
	response := map[string]any{"payment": map[string]any{"id": "provider-get-2", "status": "Failed", "transactionEvents": []any{map[string]any{"responseStatus": map[string]any{"status": "Failed", "code": float64(210)}}, map[string]any{"responseStatus": map[string]any{"status": "Failed", "code": float64(231)}}}}}
	outcome := extractProviderOutcome(response)
	if outcome.Code != 231 {
		t.Fatalf("Code = %d, queria 231 (último elemento de transactionEvents)", outcome.Code)
	}
	if outcome.Categoria != "recusado_pelo_cliente" {
		t.Fatalf("Categoria = %q, queria recusado_pelo_cliente", outcome.Categoria)
	}
}

func TestExtractProviderOutcomeCodigoDesconhecidoNuncaFicaSemClassificacao(t *testing.T) {
	response := map[string]any{"responseStatus": map[string]any{"successful": false, "status": "", "code": float64(999999), "message": "algo que a Spuri nunca viu antes", "source": "GPO"}}
	outcome := extractProviderOutcome(response)
	if outcome.Status != "Failed" {
		t.Fatalf("código desconhecido sem status literal: Status = %q, queria Failed (nunca vazio nem Success)", outcome.Status)
	}
	if outcome.Categoria != "desconhecido" {
		t.Fatalf("Categoria = %q, queria desconhecido", outcome.Categoria)
	}
	if outcome.Message != "algo que a Spuri nunca viu antes" {
		t.Fatalf("mensagem crua não preservada para um código desconhecido: %q", outcome.Message)
	}
}

func TestExtractProviderOutcomeCodigoStringENaoFloat(t *testing.T) {
	response := map[string]any{"responseStatus": map[string]any{"status": "Failed", "code": "245"}}
	outcome := extractProviderOutcome(response)
	if !outcome.HasCode || outcome.Code != 245 {
		t.Fatalf("code como string não foi convertido: HasCode=%t Code=%d", outcome.HasCode, outcome.Code)
	}
	if outcome.Status != "Expired" {
		t.Fatalf("Status = %q, queria Expired", outcome.Status)
	}
}

func TestExtractProviderOutcomeSemNenhumaInformacao(t *testing.T) {
	outcome := extractProviderOutcome(map[string]any{"id": "sem-informacao"})
	if outcome.Status != "" || outcome.HasCode {
		t.Fatalf("esperava outcome vazio, obteve %+v", outcome)
	}
}

func TestAppyPayCodeOutcomesConsistency(t *testing.T) {
	estadosValidos := map[string]bool{"Success": true, "Pending": true, "Cancelled": true, "Expired": true, "Failed": true}
	for code, info := range appyPayCodeOutcomes {
		if !estadosValidos[info.Estado] {
			t.Errorf("código %d tem Estado %q fora do vocabulário da AppyPay", code, info.Estado)
		}
		if (info.Estado == "Cancelled" || info.Estado == "Expired") && info.Categoria == "" {
			t.Errorf("código %d (%s) deveria ter Categoria preenchida", code, info.Estado)
		}
	}
	mustBe := map[int]string{100: "Success", 101: "Pending", 209: "Cancelled", 210: "Cancelled", 211: "Cancelled", 231: "Cancelled", 245: "Expired"}
	for code, want := range mustBe {
		if got := appyPayCodeOutcomes[code].Estado; got != want {
			t.Errorf("appyPayCodeOutcomes[%d].Estado = %q, queria %q", code, got, want)
		}
	}
}
