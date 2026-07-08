package projections

import (
	"encoding/json"
	"testing"
)

func TestMateriaDTOMarshalJSONOmitePendenciaParaMateriaEscolar(t *testing.T) {
	payload, err := json.Marshal(MateriaDTO{Type: "medio", PendenciaPermitida: true})
	if err != nil {
		t.Fatalf("erro inesperado ao serializar matéria: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("erro inesperado ao desserializar matéria: %v", err)
	}
	if _, ok := got["pendencia_permitida"]; ok {
		t.Fatal("não esperava pendencia_permitida em resposta de matéria escolar")
	}
	if _, ok := got["pendencia_nivel_conclusao"]; ok {
		t.Fatal("não esperava pendencia_nivel_conclusao em resposta de matéria escolar")
	}
}

func TestMateriaDTOMarshalJSONExpoePendenciaParaMateriaSuperior(t *testing.T) {
	nivel := "2_semestre"
	payload, err := json.Marshal(MateriaDTO{Type: "superior", PendenciaPermitida: true, PendenciaNivelConclusao: &nivel})
	if err != nil {
		t.Fatalf("erro inesperado ao serializar matéria: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("erro inesperado ao desserializar matéria: %v", err)
	}
	if got["pendencia_permitida"] != true {
		t.Fatalf("esperava pendencia_permitida=true, obtido %#v", got["pendencia_permitida"])
	}
	if got["pendencia_nivel_conclusao"] != nivel {
		t.Fatalf("esperava pendencia_nivel_conclusao=%q, obtido %#v", nivel, got["pendencia_nivel_conclusao"])
	}
}
