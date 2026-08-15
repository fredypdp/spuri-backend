package aggregates

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func estudanteParaRegistro() (*Estudante, string) {
	academia := "ACD001"
	e := NewEstudante()
	e.CodigoAcademia = &academia
	e.CodigoEstudante = "EST0001"
	return e, academia
}

func TestCorrigirNotaPreservaEventoOriginal(t *testing.T) {
	e, academia := estudanteParaRegistro()
	materia := uuid.New()
	usuario := uuid.New()
	if err := e.RegistrarNota(academia, "2026", "7_ano_fundamental", "1_trimestre", materia, TipoEscolar, "prova", 8, nil, []string{"prova"}, PeriodosEscolar, usuario, 20); err != nil {
		t.Fatal(err)
	}
	originais := len(e.GetUncommittedEvents())
	if err := e.CorrigirNota(uuid.New(), academia, "2026", "1_trimestre", materia, TipoEscolar, "prova", 9, nil, "erro de digitacao", usuario, 20); err != nil {
		t.Fatal(err)
	}
	eventos := e.GetUncommittedEvents()
	if len(eventos) != originais+1 || eventos[len(eventos)-1].GetEventType() != "NotaCorrigida" {
		t.Fatal("correcao deve acrescentar NotaCorrigida sem apagar o evento original")
	}
}

func TestCorrigirFaltaExigeMotivo(t *testing.T) {
	e, academia := estudanteParaRegistro()
	materia := uuid.New()
	data := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if err := e.RegistrarFalta(academia, "2026", "7_ano_fundamental", "1_trimestre", data, materia, 1, nil, uuid.New(), PeriodosEscolar, MaxQuantidadeFaltasPadrao); err != nil {
		t.Fatal(err)
	}
	if err := e.CorrigirFalta(uuid.New(), academia, "2026", "1_trimestre", data, materia, 2, nil, "", uuid.New(), MaxQuantidadeFaltasPadrao); err == nil {
		t.Fatal("correcao de falta sem motivo deveria falhar")
	}
}

func TestRegistrarECorrigirFaltaRespeitamTetoDoAggregate(t *testing.T) {
	e, academia := estudanteParaRegistro()
	materia := uuid.New()
	data := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if err := e.RegistrarFalta(academia, "2026", "7_ano_fundamental", "1_trimestre", data, materia, MaxQuantidadeFaltasPadrao+1, nil, uuid.New(), PeriodosEscolar, MaxQuantidadeFaltasPadrao); err == nil {
		t.Fatal("falta acima do teto deveria ser rejeitada pelo aggregate")
	}
	if err := e.RegistrarFalta(academia, "2026", "7_ano_fundamental", "1_trimestre", data, materia, 1, nil, uuid.New(), PeriodosEscolar, MaxQuantidadeFaltasPadrao); err != nil {
		t.Fatal(err)
	}
	if err := e.CorrigirFalta(uuid.New(), academia, "2026", "1_trimestre", data, materia, MaxQuantidadeFaltasPadrao+1, nil, "ajuste", uuid.New(), MaxQuantidadeFaltasPadrao); err == nil {
		t.Fatal("correcao acima do teto deveria ser rejeitada pelo aggregate")
	}
}

func TestRegistrarNotaRespeitaTetoDoAggregate(t *testing.T) {
	e, academia := estudanteParaRegistro()
	err := e.RegistrarNota(academia, "2026", "1_ano_fundamental", "1_trimestre", uuid.New(), TipoEscolar, "prova", 10.01, nil, []string{"prova"}, PeriodosEscolar, uuid.New(), 10)
	if err == nil {
		t.Fatal("nota acima do teto deveria ser rejeitada pelo aggregate")
	}
}

func TestRegistrarFaltaTipoEscolarComPeriodoValido(t *testing.T) {
	e, academia := estudanteParaRegistro()
	if err := e.RegistrarFalta(academia, "2026", "7_ano_fundamental", "1_trimestre", time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC), uuid.New(), 1, nil, uuid.New(), PeriodosEscolar, MaxQuantidadeFaltasPadrao); err != nil {
		t.Fatalf("RegistrarFalta escolar com periodo valido retornou erro: %v", err)
	}
}

func TestCorrigirFaltaAceitaChaveLegadaSemPeriodo(t *testing.T) {
	e, academia := estudanteParaRegistro()
	materia := uuid.New()
	data := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	evento := &FaltasRegistradasEvent{
		BaseEvent:            BaseEvent{EventType: "FaltasRegistradas", AggregateID: e.ID},
		CodigoEstudante:      e.CodigoEstudante,
		CodigoAcademia:       academia,
		AnoLectivo:           "2026",
		AnoAcademico:         "1_ano_superior",
		Periodo:              "",
		Data:                 data,
		MateriaDisciplinarID: materia,
		Quantidade:           1,
		RegisteredAt:         time.Now(),
		RegistradoPor:        uuid.New(),
	}
	if err := e.Apply(evento); err != nil {
		t.Fatalf("apply de falta legada: %v", err)
	}
	if err := e.CorrigirFalta(uuid.New(), academia, "2026", "1_semestre", data, materia, 2, nil, "ajuste de falta historica", uuid.New(), MaxQuantidadeFaltasPadrao); err != nil {
		t.Fatalf("CorrigirFalta deveria aceitar chave legada sem periodo: %v", err)
	}
}
