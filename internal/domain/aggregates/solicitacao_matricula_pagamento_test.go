package aggregates

import "testing"

func TestSolicitacaoMatriculaPagamentoPendenteFixaValorECancela(t *testing.T) {
	s := NewSolicitacaoMatricula()
	s.Status = StatusSolicitacaoAprovada
	s.CodigoSolicitacao = "SOL-PAGAMENTO"
	s.CodigoAcademia = "ACA-1"
	if err := s.MarcarPendentePagamentoMatricula(1250.50, []string{"REF", "GPO"}); err != nil {
		t.Fatalf("marcar pendente: %v", err)
	}
	events := s.GetUncommittedEvents()
	if got := events[len(events)-1].GetEventType(); got != "SolicitacaoMatriculaAprovadaPendentePagamento" {
		t.Fatalf("evento = %q", got)
	}
	if err := s.Apply(events[len(events)-1]); err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusSolicitacaoAprovadaPendentePagamentoMatricula || s.ValorMatricula == nil || *s.ValorMatricula != 1250.50 {
		t.Fatalf("estado pendente não foi aplicado: %#v", s)
	}
	s.CodigoEstudanteGerado = ptr("EST-RESERVADO")
	if err := s.CancelarPendentePagamentoMatricula("desistência"); err != nil {
		t.Fatal(err)
	}
	events = s.GetUncommittedEvents()
	if err := s.Apply(events[len(events)-1]); err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusSolicitacaoCancelada || s.CodigoEstudanteGerado != nil {
		t.Fatalf("cancelamento não liberou o código: %#v", s)
	}
}

func ptr(v string) *string { return &v }
