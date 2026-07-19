package projections

import (
	"strings"
	"testing"
)

func TestWhereConcorrentesPendentesPorBINaoFiltraAcademia(t *testing.T) {
	where := whereConcorrentesPendentesPorBI()

	if strings.Contains(where, "codigo_academia") {
		t.Fatalf("busca de concorrentes por BI não deve filtrar por academia: %s", where)
	}
	if !strings.Contains(where, "status = $1") {
		t.Fatalf("busca de concorrentes por BI deve restringir solicitações pendentes: %s", where)
	}
	if !strings.Contains(where, "codigo_solicitacao <> $2") {
		t.Fatalf("busca de concorrentes por BI deve excluir a solicitação aprovada: %s", where)
	}
	if !strings.Contains(where, "lower(btrim(bilhete_identidade)) = lower(btrim($3))") {
		t.Fatalf("busca de concorrentes por BI deve comparar BI normalizado: %s", where)
	}
}
