package aggregates

import (
	"testing"
	"time"
)

func TestSolicitacaoEdicaoDadoEstudanteTerminal(t *testing.T) {
	s := NewSolicitacaoEdicaoDadoEstudante()
	if err := s.Criar("SOL123", "EST001", "ACA001", CampoEdicaoNome, "Nome Antigo", "Nome Novo", "tmp/doc.pdf", "", "EST001"); err != nil {
		t.Fatalf("criar solicitação: %v", err)
	}
	if s.Status != StatusSolicitacaoPendente {
		t.Fatalf("status inicial = %s", s.Status)
	}
	if err := s.Aprovar("ACA001"); err != nil {
		t.Fatalf("aprovar solicitação: %v", err)
	}
	if s.Status != StatusSolicitacaoAprovada {
		t.Fatalf("status aprovado = %s", s.Status)
	}
	if err := s.Reprovar("ACA001", "documento inválido"); err == nil {
		t.Fatalf("solicitação decidida não deve aceitar nova decisão")
	}
}

func TestEventosDedicadosAlteramApenasCampoSolicitado(t *testing.T) {
	e := NewEstudante()
	oldBI := "123456789LA01"
	e.BilheteIdentidade = &oldBI
	e.DataNascimento = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := e.AlterarNomePorSolicitacao("Novo Nome", "SOL123", "ACA001"); err != nil {
		t.Fatalf("alterar nome por solicitação: %v", err)
	}
	if e.Nome != "Novo Nome" {
		t.Fatalf("nome não alterado: %s", e.Nome)
	}
	if e.BilheteIdentidade == nil || *e.BilheteIdentidade != oldBI {
		t.Fatalf("BI não deveria mudar")
	}
}
