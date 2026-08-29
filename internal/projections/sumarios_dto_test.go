package projections

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSumarioEFaltaDTOJSONSumarioFields(t *testing.T) {
	b, _ := json.Marshal(SumarioDTO{})
	if strings.Contains(string(b), "curso_id") {
		t.Fatal("nil optional field serialized")
	}
	id, title := "id", "titulo"
	b, _ = json.Marshal(FaltaDTO{SumarioID: &id, SumarioTitulo: &title})
	if !strings.Contains(string(b), `"sumario_id":"id"`) || !strings.Contains(string(b), `"sumario_titulo":"titulo"`) {
		t.Fatalf("fields missing: %s", b)
	}
}
