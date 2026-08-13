package middleware

import "testing"

func TestRotaFinanceiraRestritaNaoAbreDemaisRotasProtegidas(t *testing.T) {
	for _, path := range []string{"/financeiro/mensalidades/pagamento", "/financeiro/mensalidades/estudante/EST001"} {
		if !rotaFinanceiraRestrita(path) {
			t.Fatalf("rota financeira permitida foi recusada: %s", path)
		}
	}
	for _, path := range []string{"/meu-perfil", "/notas", "/financeiro/appypay/cobrancas"} {
		if rotaFinanceiraRestrita(path) {
			t.Fatalf("rota não financeira foi aberta pela sessão restrita: %s", path)
		}
	}
}
