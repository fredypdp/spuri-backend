package aggregates

import (
	"strings"
	"testing"
	"time"
)

func TestEstudanteCriarComVinculoRejeitaBilhetesIguais(t *testing.T) {
	bi := " 001LA001 "
	tel := "923000000"
	biResp := "001la001"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Aluno Teste",
		"ABC1234",
		strings.Repeat("a", 60),
		nil,
		&tel,
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
	tel := "923000000"
	estudante.Telefone = &tel

	biResp := " 001la001 "
	err := estudante.AtualizarDadosPessoais(nil, nil, nil, nil, nil, &biResp, nil)
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

func TestEstudanteCriarComVinculoRejeitaDocumentoEscolarSemReferencia(t *testing.T) {
	bi := "001LA002"
	biResp := "001LA003"
	tel := "923000000"
	anoFundamental := "1_ano_fundamental"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Aluno Teste",
		"ABC1235",
		strings.Repeat("a", 60),
		nil,
		&tel,
		nil,
		&bi,
		&biResp,
		"masculino",
		time.Now().AddDate(-10, 0, 0),
		&anoFundamental,
		nil,
		nil,
		nil,
		nil,
		nil,
		"ACA2026",
		map[string]DocumentoMatricula{
			"bi_responsavel":                {Path: "bi-responsavel.pdf"},
			"bi_estudante":                  {Path: "bi-estudante.pdf"},
			"certificado_6_ano_fundamental": {},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "certificado aplicável ou declaracao") {
		t.Fatalf("esperava erro de documento académico sem referência, recebeu %v", err)
	}
}

func TestEstudanteCriarComVinculoAceitaDocumentoEscolarComDownloadURL(t *testing.T) {
	bi := "001LA004"
	biResp := "001LA005"
	tel := "923000000"
	anoFundamental := "1_ano_fundamental"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Aluno Teste",
		"ABC1236",
		strings.Repeat("a", 60),
		nil,
		&tel,
		nil,
		&bi,
		&biResp,
		"feminino",
		time.Now().AddDate(-10, 0, 0),
		&anoFundamental,
		nil,
		nil,
		nil,
		nil,
		nil,
		"ACA2026",
		map[string]DocumentoMatricula{
			"bi_responsavel": {DownloadURL: "https://example.com/bi-responsavel.pdf"},
			"bi_estudante":   {FileURL: "https://example.com/bi-estudante.pdf"},
			"declaracao":     {Path: "declaracao.pdf"},
		},
	)
	if err != nil {
		t.Fatalf("não esperava erro com documentos válidos, recebeu %v", err)
	}
}
