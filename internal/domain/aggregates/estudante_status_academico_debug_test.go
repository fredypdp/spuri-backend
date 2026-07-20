package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

func TestDebugDesvinculacaoDefineStatusInativoEPreservaHistorico(t *testing.T) {
	codigoAcademia := "ACA_01"
	atorID := uuid.New()
	ano := "6_ano_fundamental"

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "ativo"
	estudante.StatusEscolarFundamental = "em_andamento"
	estudante.AnoEscolar = &ano

	if err := estudante.DesvincularDaAcademia(codigoAcademia, "transferência solicitada", atorID); err != nil {
		t.Fatalf("DesvincularDaAcademia retornou erro: %v", err)
	}

	if estudante.Status != "inativo" {
		t.Fatalf("Status = %q, want inativo", estudante.Status)
	}
	if estudante.AnoEscolar == nil || *estudante.AnoEscolar != ano {
		t.Fatalf("AnoEscolar = %v, want histórico preservado em %q", estudante.AnoEscolar, ano)
	}
	last := estudante.UncommittedEvents[len(estudante.UncommittedEvents)-1]
	event, ok := last.(*EstudanteDesvinculadoDaAcademiaEvent)
	if !ok {
		t.Fatalf("último evento = %T, want *EstudanteDesvinculadoDaAcademiaEvent", last)
	}
	if event.Nivel != "fundamental:6_ano_fundamental" {
		t.Fatalf("Nivel = %q, want snapshot acadêmico atual", event.Nivel)
	}
}

func TestDebugInterromperPercursoExigeExatamenteUmaEtapaEmAndamento(t *testing.T) {
	atorID := uuid.New()
	estudante := NewEstudante()
	estudante.StatusEscolarFundamental = "em_andamento"
	estudante.StatusEscolarMedio = "em_andamento"

	if err := estudante.InterromperPercursoAcademico("mudança de cidade", atorID); err == nil {
		t.Fatal("InterromperPercursoAcademico com duas etapas em andamento retornou nil, want erro")
	}

	estudante.StatusEscolarMedio = "inativo"
	if err := estudante.InterromperPercursoAcademico("mudança de cidade", atorID); err != nil {
		t.Fatalf("InterromperPercursoAcademico retornou erro inesperado: %v", err)
	}
	if estudante.StatusEscolarFundamental != "inativo" {
		t.Fatalf("StatusEscolarFundamental = %q, want inativo", estudante.StatusEscolarFundamental)
	}
	if got := estudante.UncommittedEvents[len(estudante.UncommittedEvents)-1].GetEventType(); got != "FundamentalInterrompido" {
		t.Fatalf("evento gerado = %q, want FundamentalInterrompido", got)
	}
}

func TestDebugReintegracaoExigeStatusInativo(t *testing.T) {
	codigoAcademia := "ACA_01"
	estudante := NewEstudante()
	estudante.Status = "ativo"

	if err := estudante.Reintegrar(codigoAcademia, "fundamental", nil, nil, nil, nil, uuid.New()); err == nil {
		t.Fatal("Reintegrar com estudante ativo retornou nil, want erro")
	}
}
