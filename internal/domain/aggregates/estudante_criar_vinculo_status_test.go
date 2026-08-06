package aggregates

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func criarEstudanteComAnoParaTeste(t *testing.T, anoFund, anoMedio, anoSup *string) *Estudante {
	t.Helper()

	bi := "001LA100"
	biResp := "001LA101"
	telefone := "923000000"
	telefoneResp := "924000000"
	docs := map[string]DocumentoMatricula{
		"bi_encarregado": {Path: "bi-encarregado.pdf"},
		"bi_estudante":   {Path: "bi-estudante.pdf"},
		"declaracao":     {Path: "declaracao.pdf", AnoAcademico: "9_ano_fundamental"},
	}
	if anoSup != nil {
		docs["certificado_medio"] = DocumentoMatricula{Path: "certificado-medio.pdf"}
		docs["declaracao"] = DocumentoMatricula{Path: "declaracao.pdf", AnoAcademico: "3_ano_medio"}
	}

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Aluno Teste",
		"EST1000",
		strings.Repeat("a", 60),
		nil,
		&telefone,
		&telefoneResp,
		&bi,
		&biResp,
		"masculino",
		time.Now().AddDate(-18, 0, 0),
		anoFund,
		anoMedio,
		anoSup,
		nil,
		nil,
		nil,
		"ACA2026",
		docs,
	)
	if err != nil {
		t.Fatalf("CriarComVinculo retornou erro: %v", err)
	}
	return estudante
}

func TestCriarComVinculoDerivaStatusPorNivelFundamental(t *testing.T) {
	ano := "1_ano_fundamental"
	estudante := criarEstudanteComAnoParaTeste(t, &ano, nil, nil)

	if estudante.StatusEscolarFundamental != "em_andamento" {
		t.Fatalf("StatusEscolarFundamental = %q, want em_andamento", estudante.StatusEscolarFundamental)
	}
	if estudante.StatusEscolarMedio != "inativo" {
		t.Fatalf("StatusEscolarMedio = %q, want inativo", estudante.StatusEscolarMedio)
	}
	if estudante.StatusSuperior != "inativo" {
		t.Fatalf("StatusSuperior = %q, want inativo", estudante.StatusSuperior)
	}
}

func TestCriarComVinculoDerivaStatusPorNivelMedio(t *testing.T) {
	ano := "1_ano_medio"
	estudante := criarEstudanteComAnoParaTeste(t, nil, &ano, nil)

	if estudante.StatusEscolarFundamental != "finalizado" {
		t.Fatalf("StatusEscolarFundamental = %q, want finalizado", estudante.StatusEscolarFundamental)
	}
	if estudante.StatusEscolarMedio != "em_andamento" {
		t.Fatalf("StatusEscolarMedio = %q, want em_andamento", estudante.StatusEscolarMedio)
	}
	if estudante.StatusSuperior != "inativo" {
		t.Fatalf("StatusSuperior = %q, want inativo", estudante.StatusSuperior)
	}
}

func TestCriarComVinculoDerivaStatusPorNivelSuperior(t *testing.T) {
	ano := "1_ano_superior"
	estudante := criarEstudanteComAnoParaTeste(t, nil, nil, &ano)

	if estudante.StatusEscolarFundamental != "finalizado" {
		t.Fatalf("StatusEscolarFundamental = %q, want finalizado", estudante.StatusEscolarFundamental)
	}
	if estudante.StatusEscolarMedio != "finalizado" {
		t.Fatalf("StatusEscolarMedio = %q, want finalizado", estudante.StatusEscolarMedio)
	}
	if estudante.StatusSuperior != "em_andamento" {
		t.Fatalf("StatusSuperior = %q, want em_andamento", estudante.StatusSuperior)
	}
}

func TestCriarComVinculoRejeitaMaisDeUmNivelInformado(t *testing.T) {
	anoFund := "1_ano_fundamental"
	anoMedio := "1_ano_medio"
	bi := "001LA102"
	biResp := "001LA103"
	telefone := "923000000"
	telefoneResp := "924000000"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo("Aluno Teste", "EST1001", strings.Repeat("a", 60), nil, &telefone, &telefoneResp, &bi, &biResp, "feminino", time.Now().AddDate(-18, 0, 0), &anoFund, &anoMedio, nil, nil, nil, nil, "ACA2026")
	if err == nil || !strings.Contains(err.Error(), "apenas um nível acadêmico") {
		t.Fatalf("esperava erro de múltiplos níveis acadêmicos, recebeu %v", err)
	}
}

func TestInterromperPercursoAcademicoInterrompeNivelCorretoAposCadastroMedio(t *testing.T) {
	ano := "1_ano_medio"
	estudante := criarEstudanteComAnoParaTeste(t, nil, &ano, nil)

	if err := estudante.InterromperPercursoAcademico("mudança de cidade", uuid.New()); err != nil {
		t.Fatalf("InterromperPercursoAcademico retornou erro: %v", err)
	}

	last := estudante.GetUncommittedEvents()[len(estudante.GetUncommittedEvents())-1]
	if _, ok := last.(*MedioInterrompidoEvent); !ok {
		t.Fatalf("último evento = %T, want *MedioInterrompidoEvent", last)
	}
}
