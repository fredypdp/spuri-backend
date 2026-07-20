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

	if err := estudante.InterromperSuperior("interrupção", atorID); err != nil {
		t.Fatalf("InterromperSuperior retornou erro: %v", err)
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
	estudante.Status = "inativo"
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
	estudante.Status = "inativo"
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
	estudante.Status = "inativo"
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

	if err := estudante.InterromperSuperior("interrupção", atorID); err != nil {
		t.Fatalf("InterromperSuperior retornou erro: %v", err)
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

func TestApplyEventosRetomadaPreservamHistoricoSemMudancaCurso(t *testing.T) {
	cursoMedioID := uuid.New()
	cursoSuperiorID := uuid.New()
	anoFundamental := "5_ano_fundamental"
	anoMedio := "2_ano_medio"
	anoSuperior := "2_ano_superior"
	semestre := 4

	estudante := NewEstudante()
	estudante.AnoEscolar = &anoFundamental
	estudante.AnoEscolarMedio = &anoMedio
	estudante.CursoMedioID = &cursoMedioID
	estudante.AnoSuperior = &anoSuperior
	estudante.SemestreAtual = &semestre
	estudante.CursoSuperiorID = &cursoSuperiorID

	if err := estudante.Apply(&FundamentalRetomadoEvent{
		BaseEvent:  BaseEvent{EventType: "FundamentalRetomado", AggregateID: estudante.ID},
		AnoEscolar: "1_ano_fundamental",
	}); err != nil {
		t.Fatalf("Apply(FundamentalRetomado) retornou erro: %v", err)
	}
	if err := estudante.Apply(&MedioRetomadoEvent{
		BaseEvent:  BaseEvent{EventType: "MedioRetomado", AggregateID: estudante.ID},
		CursoID:    cursoMedioID,
		AnoEscolar: "1_ano_medio",
	}); err != nil {
		t.Fatalf("Apply(MedioRetomado) retornou erro: %v", err)
	}
	if err := estudante.Apply(&MatriculaSuperiorReativadaEvent{
		BaseEvent:     BaseEvent{EventType: "MatriculaSuperiorReativada", AggregateID: estudante.ID},
		CursoID:       cursoSuperiorID,
		AnoSuperior:   "1_ano_superior",
		SemestreAtual: 1,
	}); err != nil {
		t.Fatalf("Apply(MatriculaSuperiorReativada) retornou erro: %v", err)
	}

	if estudante.AnoEscolar == nil || *estudante.AnoEscolar != "5_ano_fundamental" {
		t.Fatalf("AnoEscolar = %v, want 5_ano_fundamental", estudante.AnoEscolar)
	}
	if estudante.AnoEscolarMedio == nil || *estudante.AnoEscolarMedio != "2_ano_medio" {
		t.Fatalf("AnoEscolarMedio = %v, want 2_ano_medio", estudante.AnoEscolarMedio)
	}
	if estudante.SemestreAtual == nil || *estudante.SemestreAtual != 4 {
		t.Fatalf("SemestreAtual = %v, want 4", estudante.SemestreAtual)
	}
	if estudante.AnoSuperior == nil || *estudante.AnoSuperior != "2_ano_superior" {
		t.Fatalf("AnoSuperior = %v, want 2_ano_superior", estudante.AnoSuperior)
	}
}

func TestReintegrarMedioSemCursoInformadoPreservaCursoEAnoAnterior(t *testing.T) {
	codigoAcademia := "ACA_01"
	cursoID := uuid.New()
	atorID := uuid.New()
	ano := "2_ano_medio"

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "inativo"
	estudante.StatusEscolarMedio = "inativo"
	estudante.CursoMedioID = &cursoID
	estudante.AnoEscolarMedio = &ano

	if err := estudante.Reintegrar(codigoAcademia, "medio", nil, nil, nil, nil, atorID); err != nil {
		t.Fatalf("Reintegrar retornou erro: %v", err)
	}

	if estudante.CursoMedioID == nil || *estudante.CursoMedioID != cursoID {
		t.Fatalf("CursoMedioID = %v, want %s", estudante.CursoMedioID, cursoID)
	}
	if estudante.AnoEscolarMedio == nil || *estudante.AnoEscolarMedio != "2_ano_medio" {
		t.Fatalf("AnoEscolarMedio = %v, want 2_ano_medio", estudante.AnoEscolarMedio)
	}
}

func TestReintegrarSuperiorSemCursoInformadoPreservaCursoSemestreEAnoAnterior(t *testing.T) {
	codigoAcademia := "ACA_01"
	cursoID := uuid.New()
	atorID := uuid.New()
	ano := "2_ano_superior"
	semestre := 4

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "inativo"
	estudante.StatusSuperior = "inativo"
	estudante.CursoSuperiorID = &cursoID
	estudante.AnoSuperior = &ano
	estudante.SemestreAtual = &semestre

	if err := estudante.Reintegrar(codigoAcademia, "superior", nil, nil, nil, nil, atorID); err != nil {
		t.Fatalf("Reintegrar retornou erro: %v", err)
	}

	if estudante.CursoSuperiorID == nil || *estudante.CursoSuperiorID != cursoID {
		t.Fatalf("CursoSuperiorID = %v, want %s", estudante.CursoSuperiorID, cursoID)
	}
	if estudante.SemestreAtual == nil || *estudante.SemestreAtual != 4 {
		t.Fatalf("SemestreAtual = %v, want 4", estudante.SemestreAtual)
	}
	if estudante.AnoSuperior == nil || *estudante.AnoSuperior != "2_ano_superior" {
		t.Fatalf("AnoSuperior = %v, want 2_ano_superior", estudante.AnoSuperior)
	}
}
