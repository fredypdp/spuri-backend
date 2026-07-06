package projections

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestCursoDTOMarshalModeloExclusivoCursoMedio(t *testing.T) {
	medio := CursoDTO{
		ID:             uuid.New(),
		Nome:           "Ciências Físicas e Biológicas",
		Type:           "medio",
		Modelo:         "liceu",
		AnosAcademicos: []string{"1_ano_medio"},
		Periodos:       []string{},
		CodigoAcademia: "ACA_01",
		Status:         "ativo",
	}
	medioJSON, err := json.Marshal(medio)
	if err != nil {
		t.Fatalf("json.Marshal(medio) erro inesperado: %v", err)
	}
	var medioMap map[string]any
	if err := json.Unmarshal(medioJSON, &medioMap); err != nil {
		t.Fatalf("json.Unmarshal(medio) erro inesperado: %v", err)
	}
	if got := medioMap["modelo"]; got != "liceu" {
		t.Fatalf("modelo de curso médio = %v, want liceu", got)
	}

	superior := CursoDTO{
		ID:             uuid.New(),
		Nome:           "Engenharia Informática",
		Type:           "superior",
		AnosAcademicos: []string{"1_ano_superior"},
		Periodos:       []string{"1_semestre", "2_semestre"},
		CodigoAcademia: "ACA_01",
		Status:         "ativo",
	}
	superiorJSON, err := json.Marshal(superior)
	if err != nil {
		t.Fatalf("json.Marshal(superior) erro inesperado: %v", err)
	}
	var superiorMap map[string]any
	if err := json.Unmarshal(superiorJSON, &superiorMap); err != nil {
		t.Fatalf("json.Unmarshal(superior) erro inesperado: %v", err)
	}
	if _, ok := superiorMap["modelo"]; ok {
		t.Fatalf("curso superior não deve expor modelo: %s", superiorJSON)
	}
}

func TestModeloCursoParaProjecaoBackfillEventoLegadoMedio(t *testing.T) {
	if got := modeloCursoParaProjecao("medio", ""); got != "liceu" {
		t.Fatalf("modelo legado médio = %q, want liceu", got)
	}
	if got := modeloCursoParaProjecao("superior", ""); got != "" {
		t.Fatalf("modelo legado superior = %q, want vazio", got)
	}
}
