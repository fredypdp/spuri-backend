package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

func TestReintegrarSuperiorMesmoCursoPreservaSemestreAtual(t *testing.T) {
	codigoAcademia := "ACA_01"
	cursoID := uuid.New()
	atorID := uuid.New()
	ano := "2_ano_superior"
	semestre := 4

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "ativo"
	estudante.StatusSuperior = "em_andamento"
	estudante.CursoSuperiorID = &cursoID
	estudante.AnoSuperior = &ano
	estudante.SemestreAtual = &semestre

	if err := estudante.TrancarSuperior("trancamento", atorID); err != nil {
		t.Fatalf("TrancarSuperior retornou erro: %v", err)
	}
	if err := estudante.DesvincularDaAcademia(codigoAcademia, "transferência", atorID); err != nil {
		t.Fatalf("DesvincularDaAcademia retornou erro: %v", err)
	}
	if err := estudante.Reintegrar(codigoAcademia, "superior", nil, nil, nil, &cursoID, atorID); err != nil {
		t.Fatalf("Reintegrar retornou erro: %v", err)
	}

	if estudante.SemestreAtual == nil || *estudante.SemestreAtual != 4 {
		t.Fatalf("SemestreAtual = %v, want 4", estudante.SemestreAtual)
	}
	if estudante.AnoSuperior == nil || *estudante.AnoSuperior != "2_ano_superior" {
		t.Fatalf("AnoSuperior = %v, want 2_ano_superior", estudante.AnoSuperior)
	}
}

func TestReintegrarSuperiorCursoDiferenteReiniciaNovoCurso(t *testing.T) {
	codigoAcademia := "ACA_01"
	cursoAntigoID := uuid.New()
	cursoNovoID := uuid.New()
	atorID := uuid.New()
	ano := "2_ano_superior"
	semestre := 4

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "arquivado"
	estudante.StatusSuperior = "inativo"
	estudante.CursoSuperiorID = &cursoAntigoID
	estudante.AnoSuperior = &ano
	estudante.SemestreAtual = &semestre

	if err := estudante.Reintegrar(codigoAcademia, "superior", nil, nil, nil, &cursoNovoID, atorID); err != nil {
		t.Fatalf("Reintegrar retornou erro: %v", err)
	}

	if estudante.SemestreAtual == nil || *estudante.SemestreAtual != 1 {
		t.Fatalf("SemestreAtual = %v, want 1", estudante.SemestreAtual)
	}
	if estudante.AnoSuperior == nil || *estudante.AnoSuperior != "1_ano_superior" {
		t.Fatalf("AnoSuperior = %v, want 1_ano_superior", estudante.AnoSuperior)
	}
}

func TestReintegrarMedioMesmoCursoPreservaAnoEscolar(t *testing.T) {
	codigoAcademia := "ACA_01"
	cursoID := uuid.New()
	atorID := uuid.New()
	ano := "2_ano_medio"
	anoSolicitado := "1_ano_medio"

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "ativo"
	estudante.StatusEscolarMedio = "em_andamento"
	estudante.CursoMedioID = &cursoID
	estudante.AnoEscolarMedio = &ano

	if err := estudante.InterromperMedio("interrupção", atorID); err != nil {
		t.Fatalf("InterromperMedio retornou erro: %v", err)
	}
	if err := estudante.DesvincularDaAcademia(codigoAcademia, "transferência", atorID); err != nil {
		t.Fatalf("DesvincularDaAcademia retornou erro: %v", err)
	}
	if err := estudante.Reintegrar(codigoAcademia, "medio", nil, &anoSolicitado, &cursoID, nil, atorID); err != nil {
		t.Fatalf("Reintegrar retornou erro: %v", err)
	}

	if estudante.AnoEscolarMedio == nil || *estudante.AnoEscolarMedio != "2_ano_medio" {
		t.Fatalf("AnoEscolarMedio = %v, want 2_ano_medio", estudante.AnoEscolarMedio)
	}
}

func TestReintegrarMedioCursoDiferenteReiniciaNovoCurso(t *testing.T) {
	codigoAcademia := "ACA_01"
	cursoAntigoID := uuid.New()
	cursoNovoID := uuid.New()
	atorID := uuid.New()
	ano := "2_ano_medio"

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "arquivado"
	estudante.StatusEscolarMedio = "inativo"
	estudante.CursoMedioID = &cursoAntigoID
	estudante.AnoEscolarMedio = &ano

	if err := estudante.Reintegrar(codigoAcademia, "medio", nil, nil, &cursoNovoID, nil, atorID); err != nil {
		t.Fatalf("Reintegrar retornou erro: %v", err)
	}

	if estudante.AnoEscolarMedio == nil || *estudante.AnoEscolarMedio != "1_ano_medio" {
		t.Fatalf("AnoEscolarMedio = %v, want 1_ano_medio", estudante.AnoEscolarMedio)
	}
}

func TestReintegrarFundamentalSemAnoPreservaAnoAnterior(t *testing.T) {
	codigoAcademia := "ACA_01"
	atorID := uuid.New()
	ano := "5_ano_fundamental"

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "arquivado"
	estudante.StatusEscolarFundamental = "inativo"
	estudante.AnoEscolar = &ano

	if err := estudante.Reintegrar(codigoAcademia, "fundamental", nil, nil, nil, nil, atorID); err != nil {
		t.Fatalf("Reintegrar retornou erro: %v", err)
	}

	if estudante.AnoEscolar == nil || *estudante.AnoEscolar != "5_ano_fundamental" {
		t.Fatalf("AnoEscolar = %v, want 5_ano_fundamental", estudante.AnoEscolar)
	}
}

func TestReplayEventosPreservaEstadoFinalDaReintegracao(t *testing.T) {
	codigoAcademia := "ACA_01"
	cursoID := uuid.New()
	atorID := uuid.New()
	ano := "2_ano_superior"
	semestre := 4

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "ativo"
	estudante.StatusSuperior = "em_andamento"
	estudante.CursoSuperiorID = &cursoID
	estudante.AnoSuperior = &ano
	estudante.SemestreAtual = &semestre

	if err := estudante.TrancarSuperior("trancamento", atorID); err != nil {
		t.Fatalf("TrancarSuperior retornou erro: %v", err)
	}
	if err := estudante.DesvincularDaAcademia(codigoAcademia, "transferência", atorID); err != nil {
		t.Fatalf("DesvincularDaAcademia retornou erro: %v", err)
	}
	if err := estudante.Reintegrar(codigoAcademia, "superior", nil, nil, nil, &cursoID, atorID); err != nil {
		t.Fatalf("Reintegrar retornou erro: %v", err)
	}

	replay := NewEstudante()
	replay.CodigoEstudante = "EST1234"
	replay.CodigoAcademia = &codigoAcademia
	replay.Status = "ativo"
	replay.StatusSuperior = "em_andamento"
	replay.CursoSuperiorID = &cursoID
	replay.AnoSuperior = &ano
	replay.SemestreAtual = &semestre
	for _, event := range estudante.UncommittedEvents {
		if err := replay.Apply(event); err != nil {
			t.Fatalf("replay Apply(%s) retornou erro: %v", event.GetEventType(), err)
		}
	}

	if replay.Status != estudante.Status || replay.StatusSuperior != estudante.StatusSuperior {
		t.Fatalf("status replay = (%q, %q), want (%q, %q)", replay.Status, replay.StatusSuperior, estudante.Status, estudante.StatusSuperior)
	}
	if replay.SemestreAtual == nil || estudante.SemestreAtual == nil || *replay.SemestreAtual != *estudante.SemestreAtual {
		t.Fatalf("SemestreAtual replay = %v, want %v", replay.SemestreAtual, estudante.SemestreAtual)
	}
	if replay.AnoSuperior == nil || estudante.AnoSuperior == nil || *replay.AnoSuperior != *estudante.AnoSuperior {
		t.Fatalf("AnoSuperior replay = %v, want %v", replay.AnoSuperior, estudante.AnoSuperior)
	}
}
