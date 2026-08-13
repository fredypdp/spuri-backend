package finance

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMensalidadeAnoLetivoEMesesRespeitamPeriodosFixos(t *testing.T) {
	if !anoLetivoValido("2026_2027") || anoLetivoValido("2026_2028") {
		t.Fatal("validaÃ§Ã£o de ano letivo invÃ¡lida")
	}
	escolar := mesesAnoLetivo("2026_2027", NivelFundamental)
	if len(escolar) != 11 || escolar[0].Month != 9 || escolar[0].Data != time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC) || escolar[len(escolar)-1].Month != 7 {
		t.Fatalf("meses escolares invÃ¡lidos: %#v", escolar)
	}
	superior := mesesAnoLetivo("2026_2027", NivelSuperior)
	if len(superior) != 10 || superior[0].Month != 10 || superior[len(superior)-1].Month != 7 {
		t.Fatalf("meses superiores invÃ¡lidos: %#v", superior)
	}
}

func TestMensalidadeAggregateIDIsStableAndScopedByAcademy(t *testing.T) {
	a := mensalidadeAggregateID("ACA001")
	if a == uuid.Nil || a != mensalidadeAggregateID("aca001") {
		t.Fatal("aggregate mensalidade deve ser determinÃ­stico por academia")
	}
	if a == mensalidadeAggregateID("ACA002") {
		t.Fatal("academias distintas nÃ£o podem compartilhar stream financeiro de mensalidade")
	}
}

func TestMensalidadeStatePrecedence(t *testing.T) {
	// A precedÃªncia efetiva preserva um pagamento real mesmo diante de um
	// evento histÃ³rico de anulaÃ§Ã£o e permite reativaÃ§Ã£o somente do anulado.
	state := precedenciaEstado([]string{"anulada", "reativada", "paga", "anulada"})
	if state != EstadoPago {
		t.Fatalf("estado final = %s, want pago", state)
	}
}
