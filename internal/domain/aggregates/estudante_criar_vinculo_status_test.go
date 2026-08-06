package aggregates

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCriarComVinculoDerivaStatusPorNivelFundamental(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()
	anoFundamental := "5_ano_fundamental"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"João Silva",
		"EST001",
		"senha_hash",
		nil,
		nil,
		nil,
		nil,
		nil,
		"masculino",
		mustParseTime("2010-05-15"),
		&anoFundamental,
		nil,
		nil,
		nil,
		nil,
		&academiaID,
		codigoAcademia,
	)
	if err != nil {
		t.Fatalf("CriarComVinculo retornou erro: %v", err)
	}

	if estudante.StatusEscolarFundamental != "em_andamento" {
		t.Errorf("StatusEscolarFundamental = %q, want \"em_andamento\"", estudante.StatusEscolarFundamental)
	}
	if estudante.StatusEscolarMedio != "inativo" {
		t.Errorf("StatusEscolarMedio = %q, want \"inativo\"", estudante.StatusEscolarMedio)
	}
	if estudante.StatusSuperior != "inativo" {
		t.Errorf("StatusSuperior = %q, want \"inativo\"", estudante.StatusSuperior)
	}
}

func TestCriarComVinculoDerivaStatusPorNivelMedio(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()
	anoMedio := "2_ano_medio"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Maria Santos",
		"EST002",
		"senha_hash",
		nil,
		nil,
		nil,
		nil,
		nil,
		"feminino",
		mustParseTime("2008-03-20"),
		nil,
		&anoMedio,
		nil,
		nil,
		nil,
		&academiaID,
		codigoAcademia,
	)
	if err != nil {
		t.Fatalf("CriarComVinculo retornou erro: %v", err)
	}

	if estudante.StatusEscolarFundamental != "finalizado" {
		t.Errorf("StatusEscolarFundamental = %q, want \"finalizado\"", estudante.StatusEscolarFundamental)
	}
	if estudante.StatusEscolarMedio != "em_andamento" {
		t.Errorf("StatusEscolarMedio = %q, want \"em_andamento\"", estudante.StatusEscolarMedio)
	}
	if estudante.StatusSuperior != "inativo" {
		t.Errorf("StatusSuperior = %q, want \"inativo\"", estudante.StatusSuperior)
	}
}

func TestCriarComVinculoDerivaStatusPorNivelSuperior(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()
	anoSuperior := "3_ano_superior"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Pedro Oliveira",
		"EST003",
		"senha_hash",
		nil,
		nil,
		nil,
		nil,
		nil,
		"masculino",
		mustParseTime("2002-07-10"),
		nil,
		nil,
		&anoSuperior,
		nil,
		nil,
		&academiaID,
		codigoAcademia,
	)
	if err != nil {
		t.Fatalf("CriarComVinculo retornou erro: %v", err)
	}

	if estudante.StatusEscolarFundamental != "finalizado" {
		t.Errorf("StatusEscolarFundamental = %q, want \"finalizado\"", estudante.StatusEscolarFundamental)
	}
	if estudante.StatusEscolarMedio != "finalizado" {
		t.Errorf("StatusEscolarMedio = %q, want \"finalizado\"", estudante.StatusEscolarMedio)
	}
	if estudante.StatusSuperior != "em_andamento" {
		t.Errorf("StatusSuperior = %q, want \"em_andamento\"", estudante.StatusSuperior)
	}
}

func TestCriarComVinculoRejeitaMaisDeUmNivelInformado(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()
	anoFundamental := "5_ano_fundamental"
	anoMedio := "1_ano_medio"

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Teste Erro",
		"EST004",
		"senha_hash",
		nil,
		nil,
		nil,
		nil,
		nil,
		"masculino",
		mustParseTime("2010-01-01"),
		&anoFundamental,
		&anoMedio,
		nil,
		nil,
		nil,
		&academiaID,
		codigoAcademia,
	)
	if err == nil {
		t.Fatal("CriarComVinculo deveria retornar erro ao informar múltiplos níveis, mas retornou nil")
	}
	if !strings.Contains(err.Error(), "apenas um nível acadêmico pode ser informado") {
		t.Errorf("Erro = %q, deve conter 'apenas um nível acadêmico pode ser informado'", err.Error())
	}
}

func TestInterromperPercursoAcademicoInterrompeNivelCorretoAposCadastroMedio(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()
	anoMedio := "2_ano_medio"
	atorID := uuid.New()

	estudante := NewEstudante()
	err := estudante.CriarComVinculo(
		"Carlos Mendes",
		"EST005",
		"senha_hash",
		nil,
		nil,
		nil,
		nil,
		nil,
		"masculino",
		mustParseTime("2008-11-25"),
		nil,
		&anoMedio,
		nil,
		nil,
		nil,
		&academiaID,
		codigoAcademia,
	)
	if err != nil {
		t.Fatalf("CriarComVinculo retornou erro: %v", err)
	}

	// Verificar status antes da interrupção
	if estudante.StatusEscolarMedio != "em_andamento" {
		t.Fatalf("StatusEscolarMedio = %q, want \"em_andamento\" antes da interrupção", estudante.StatusEscolarMedio)
	}

	// Interromper percurso acadêmico
	err = estudante.InterromperPercursoAcademico("motivo de teste", atorID)
	if err != nil {
		t.Fatalf("InterromperPercursoAcademico retornou erro: %v", err)
	}

	// Verificar que o último evento é MedioInterrompidoEvent (não FundamentalInterrompidoEvent)
	eventos := estudante.UncommittedEvents
	if len(eventos) == 0 {
		t.Fatal("Nenhum evento foi emitido após InterromperPercursoAcademico")
	}

	lastEvent := eventos[len(eventos)-1]
	eventType := lastEvent.GetEventType()
	if eventType != "MedioInterrompido" {
		t.Errorf("Último evento = %q, want \"MedioInterrompido\"", eventType)
	}
}

// mustParseTime converte uma string YYYY-MM-DD para time.Time em UTC.
// Usado apenas em testes.
func mustParseTime(date string) time.Time {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	return t
}
