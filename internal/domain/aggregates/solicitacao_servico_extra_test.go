package aggregates

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func novaSolicitacaoServicoExtra(t *testing.T) *SolicitacaoServicoExtra {
	t.Helper()
	s := NewSolicitacaoServicoExtra()
	if err := s.Criar(uuid.New(), "ACA001", "EST001", "", ""); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSolicitacaoServicoExtraCriar(t *testing.T) {
	if got := novaSolicitacaoServicoExtra(t).Status; got != StatusInscricaoPendente {
		t.Fatalf("status = %q, esperado %q", got, StatusInscricaoPendente)
	}
}

func TestSolicitacaoServicoExtraAprovarSemTaxaVincula(t *testing.T) {
	s := novaSolicitacaoServicoExtra(t)
	if err := s.Aprovar(false, 0, nil, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusInscricaoVinculada || s.VinculadaEm.IsZero() {
		t.Fatalf("aprovação sem taxa não vinculou corretamente: status=%q vinculada_em=%v", s.Status, s.VinculadaEm)
	}
	if err := s.Aprovar(false, 0, nil, uuid.New()); err == nil {
		t.Fatal("segunda aprovação foi aceita")
	}
}

func TestSolicitacaoServicoExtraAprovarComTaxaAguardaPagamento(t *testing.T) {
	s := novaSolicitacaoServicoExtra(t)
	if err := s.Aprovar(true, 100, []string{"GPO"}, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusInscricaoAprovadaPendentePagamentoTaxa || !s.VinculadaEm.IsZero() {
		t.Fatalf("estado inesperado: status=%q vinculada_em=%v", s.Status, s.VinculadaEm)
	}
	if err := novaSolicitacaoServicoExtra(t).Aprovar(true, 0, []string{"GPO"}, uuid.New()); err == nil {
		t.Fatal("taxa sem valor foi aceita")
	}
}

func TestSolicitacaoServicoExtraReprovar(t *testing.T) {
	s := novaSolicitacaoServicoExtra(t)
	if err := s.Reprovar("", uuid.New()); err == nil {
		t.Fatal("reprovação sem motivo foi aceita")
	}
	if err := s.Reprovar("documento ilegível", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusInscricaoReprovada {
		t.Fatalf("status = %q", s.Status)
	}
	if err := novaSolicitacaoVinculada(t).Reprovar("motivo", uuid.New()); err == nil {
		t.Fatal("reprovação de solicitação vinculada foi aceita")
	}
}

func TestSolicitacaoServicoExtraVincularAposPagamento(t *testing.T) {
	if err := novaSolicitacaoServicoExtra(t).VincularAposPagamento(); err == nil {
		t.Fatal("vínculo sem aprovação pendente foi aceito")
	}
	s := novaSolicitacaoServicoExtra(t)
	if err := s.Aprovar(true, 50, []string{"REF"}, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := s.VincularAposPagamento(); err != nil {
		t.Fatal(err)
	}
	vinculadaEm := s.VinculadaEm
	if s.Status != StatusInscricaoVinculada || vinculadaEm.IsZero() {
		t.Fatalf("vínculo não efetivado: status=%q data=%v", s.Status, vinculadaEm)
	}
	time.Sleep(time.Millisecond)
	if err := s.VincularAposPagamento(); err == nil {
		t.Fatal("segundo vínculo foi aceito")
	}
	if !s.VinculadaEm.Equal(vinculadaEm) {
		t.Fatal("VinculadaEm mudou após segunda tentativa")
	}
}

func TestSolicitacaoServicoExtraCancelar(t *testing.T) {
	pendente := novaSolicitacaoServicoExtra(t)
	if err := pendente.CancelarAntesDaVinculacao("motivo", "estudante"); err == nil {
		t.Fatal("cancelamento antes do vínculo em pendente foi aceito")
	}
	if err := pendente.Cancelar("motivo", "estudante"); err == nil {
		t.Fatal("cancelamento em pendente foi aceito")
	}
	s := novaSolicitacaoServicoExtra(t)
	if err := s.Aprovar(true, 50, []string{"GPO"}, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelarAntesDaVinculacao("desistiu", "outro"); err == nil {
		t.Fatal("cancelada_por inválido foi aceito antes do vínculo")
	}
	if err := s.CancelarAntesDaVinculacao("desistiu", "estudante"); err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusInscricaoCanceladaAntesDaVinculacao {
		t.Fatalf("status = %q", s.Status)
	}
	vinculada := novaSolicitacaoVinculada(t)
	if err := vinculada.Cancelar("motivo", "outro"); err == nil {
		t.Fatal("cancelada_por inválido foi aceito após vínculo")
	}
	if err := vinculada.Cancelar("motivo", "academia"); err != nil {
		t.Fatal(err)
	}
	if vinculada.Status != StatusInscricaoCancelada {
		t.Fatalf("status = %q", vinculada.Status)
	}
}

func novaSolicitacaoVinculada(t *testing.T) *SolicitacaoServicoExtra {
	t.Helper()
	s := novaSolicitacaoServicoExtra(t)
	if err := s.Aprovar(false, 0, nil, uuid.New()); err != nil {
		t.Fatal(err)
	}
	return s
}
