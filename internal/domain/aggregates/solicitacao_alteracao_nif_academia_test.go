package aggregates

import "testing"

func TestSolicitacaoAlteracaoNIFAcademiaTerminal(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("SOLNIF0001", "ACA001", "1234567890", "9876543210", "ACA001"); err != nil {
		t.Fatalf("criar solicitação: %v", err)
	}
	if s.Status != StatusSolicitacaoPendente {
		t.Fatalf("status inicial = %s", s.Status)
	}
	if err := s.Aprovar("ADMIN001"); err != nil {
		t.Fatalf("aprovar solicitação: %v", err)
	}
	if s.Status != StatusSolicitacaoAprovada {
		t.Fatalf("status aprovado = %s", s.Status)
	}
	if err := s.Reprovar("ADMIN001", "motivo qualquer"); err == nil {
		t.Fatalf("solicitação decidida não deve aceitar nova decisão")
	}
}

func TestSolicitacaoAlteracaoNIFAcademiaReprovarExigeMotivo(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("SOLNIF0002", "ACA001", "1234567890", "9876543210", "ACA001"); err != nil {
		t.Fatalf("criar solicitação: %v", err)
	}
	if err := s.Reprovar("ADMIN001", ""); err == nil {
		t.Fatalf("reprovar sem motivo deveria falhar")
	}
	if s.Status != StatusSolicitacaoPendente {
		t.Fatalf("status não deveria mudar após reprovação inválida: %s", s.Status)
	}
}

func TestSolicitacaoAlteracaoNIFAcademiaCriarRejeitaNIFIgual(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("SOLNIF0003", "ACA001", "1234567890", "1234567890", "ACA001"); err == nil {
		t.Fatalf("nif_solicitado igual ao nif_atual deveria ser rejeitado")
	}
}

func TestSolicitacaoAlteracaoNIFAcademiaCriarRejeitaNIFInvalido(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("SOLNIF0004", "ACA001", "1234567890", "abc", "ACA001"); err == nil {
		t.Fatalf("nif_solicitado com formato inválido deveria ser rejeitado")
	}
}

func TestSolicitacaoAlteracaoNIFAcademiaCriarRejeitaCamposObrigatorios(t *testing.T) {
	s := NewSolicitacaoAlteracaoNIFAcademia()
	if err := s.Criar("", "ACA001", "1234567890", "9876543210", "ACA001"); err == nil {
		t.Fatalf("codigo vazio deveria ser rejeitado")
	}
	if err := s.Criar("SOLNIF0005", "", "1234567890", "9876543210", "ACA001"); err == nil {
		t.Fatalf("codigo_academia vazio deveria ser rejeitado")
	}
	if err := s.Criar("SOLNIF0006", "ACA001", "1234567890", "9876543210", ""); err == nil {
		t.Fatalf("solicitado_por vazio deveria ser rejeitado")
	}
}

func TestAcademiaAlterarNIFPorSolicitacao(t *testing.T) {
	a := NewAcademia()
	a.NIF = "1234567890"
	if err := a.AlterarNIFPorSolicitacao("9876543210", "SOLNIF0001", "ADMIN001"); err != nil {
		t.Fatalf("alterar nif por solicitação: %v", err)
	}
	if a.NIF != "9876543210" {
		t.Fatalf("nif não alterado: %s", a.NIF)
	}
}

func TestAcademiaAlterarNIFPorSolicitacaoRejeitaFormatoInvalido(t *testing.T) {
	a := NewAcademia()
	a.NIF = "1234567890"
	if err := a.AlterarNIFPorSolicitacao("abc", "SOLNIF0001", "ADMIN001"); err == nil {
		t.Fatalf("nif inválido deveria ser rejeitado")
	}
	if a.NIF != "1234567890" {
		t.Fatalf("nif não deveria mudar após rejeição: %s", a.NIF)
	}
}
