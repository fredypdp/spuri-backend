package aggregates

import (
	"github.com/google/uuid"
	"testing"
)

func TestSumarioCriarValidacoes(t *testing.T) {
	cases := []struct {
		nivel, tipo string
		curso       *uuid.UUID
		ok          bool
	}{{"fundamental", TipoEscolar, nil, true}, {"medio", TipoEscolar, nil, false}, {"superior", TipoSuperior, nil, false}, {"fundamental", TipoEscolar, ptrUUID(), false}, {"superior", TipoSuperior, ptrUUID(), true}}
	for _, tt := range cases {
		s := NewSumario()
		err := s.Criar("Aula válida", nil, "ACA", tt.tipo, tt.nivel, "1_trimestre", "7_ano_fundamental", tt.curso, uuid.New(), uuid.New())
		if (err == nil) != tt.ok {
			t.Errorf("%+v: %v", tt, err)
		}
	}
	s := NewSumario()
	if err := s.Criar("Aula válida", nil, "ACA", TipoEscolar, "fundamental", "", "7_ano_fundamental", nil, uuid.New(), uuid.New()); err == nil {
		t.Fatal("expected empty periodo error")
	}
}
func ptrUUID() *uuid.UUID { x := uuid.New(); return &x }
