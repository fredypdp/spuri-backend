package aggregates

import (
	"strings"
	"testing"
	"time"
)

func TestEstudanteCriarComVinculoRejeitaBilhetesIguais(t *testing.T) {
	bi := " 001LA001 "
	biResp := "001la001"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Aluno Teste",
		"ABC1234",
		strings.Repeat("a", 60),
		nil,
		nil,
		&bi,
		&biResp,
		"masculino",
		time.Now().AddDate(-10, 0, 0),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"ACA2026",
	)
	if err == nil || !strings.Contains(err.Error(), "não podem ser iguais") {
		t.Fatalf("esperava erro de bilhetes iguais, recebeu %v", err)
	}
}

func TestEstudanteAtualizarDadosPessoaisRejeitaBilhetesEfetivosIguais(t *testing.T) {
	bi := "001LA001"
	estudante := NewEstudante()
	estudante.BilheteIdentidade = &bi

	biResp := " 001la001 "
	err := estudante.AtualizarDadosPessoais(nil, nil, nil, nil, &biResp, nil)
	if err == nil || !strings.Contains(err.Error(), "não podem ser iguais") {
		t.Fatalf("esperava erro de bilhetes iguais na atualização, recebeu %v", err)
	}
}

func TestSolicitacaoMatriculaCriarRejeitaBilhetesIguais(t *testing.T) {
	bi := "001LA001"
	biResp := " 001la001 "
	solicitacao := NewSolicitacaoMatricula()

	err := solicitacao.Criar(
		"SOL2026001",
		"ACA2026",
		"Aluno Teste",
		"feminino",
		time.Now().AddDate(-8, 0, 0),
		nil,
		nil,
		&bi,
		&biResp,
		nil,
		nil,
		nil,
		nil,
		nil,
		map[string]DocumentoMatricula{"bi_responsavel": {Path: "bi.pdf"}},
	)
	if err == nil || !strings.Contains(err.Error(), "não podem ser iguais") {
		t.Fatalf("esperava erro de bilhetes iguais na solicitação, recebeu %v", err)
	}
}
