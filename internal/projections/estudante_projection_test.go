package projections

import (
	"encoding/json"
	"testing"
)

func TestParseAvaliacaoFinalPayloadEstudanteSnakeCase(t *testing.T) {
	raw := json.RawMessage(`{"tipo_ensino":"fundamental","proximo_ano_academico":"2_ano_fundamental","aprovado":true}`)

	payload, err := parseAvaliacaoFinalPayloadEstudante(raw)
	if err != nil {
		t.Fatalf("parseAvaliacaoFinalPayload retornou erro: %v", err)
	}
	if payload.TipoEnsino != "fundamental" {
		t.Fatalf("TipoEnsino = %q; esperado fundamental", payload.TipoEnsino)
	}
	if payload.ProximoAnoAcademico == nil || *payload.ProximoAnoAcademico != "2_ano_fundamental" {
		t.Fatalf("ProximoAnoAcademico inesperado: %#v", payload.ProximoAnoAcademico)
	}
	if !payload.Aprovado {
		t.Fatalf("Aprovado = false; esperado true")
	}
}

func TestParseAvaliacaoFinalPayloadEstudanteLegacyPascalCase(t *testing.T) {
	raw := json.RawMessage(`{"TipoEnsino":"medio","ProximoAnoAcademico":"2_ano_medio","Aprovado":true}`)

	payload, err := parseAvaliacaoFinalPayloadEstudante(raw)
	if err != nil {
		t.Fatalf("parseAvaliacaoFinalPayload retornou erro: %v", err)
	}
	if payload.TipoEnsino != "medio" {
		t.Fatalf("TipoEnsino = %q; esperado medio", payload.TipoEnsino)
	}
	if payload.ProximoAnoAcademico == nil || *payload.ProximoAnoAcademico != "2_ano_medio" {
		t.Fatalf("ProximoAnoAcademico inesperado: %#v", payload.ProximoAnoAcademico)
	}
	if !payload.Aprovado {
		t.Fatalf("Aprovado = false; esperado true")
	}
}
