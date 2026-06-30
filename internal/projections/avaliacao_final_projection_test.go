package projections

import (
	"encoding/json"
	"testing"
)

func TestParseAvaliacaoFinalPayloadSnakeCase(t *testing.T) {
	raw := json.RawMessage(`{
		"codigo_estudante":"EST1234",
		"codigo_academia":"ACA_01",
		"ano_lectivo":"2025_2026",
		"nivel":"fundamental",
		"nivel_ano_academico_atual":"2_ano_fundamental",
		"proximo_ano_academico":"3_ano_fundamental",
		"aprovado":true,
		"observacao":"ok"
	}`)

	payload, err := parseAvaliacaoFinalPayload(raw)
	if err != nil {
		t.Fatalf("parseAvaliacaoFinalPayload retornou erro: %v", err)
	}

	if payload.CodigoEstudante != "EST1234" {
		t.Fatalf("CodigoEstudante inválido: %q", payload.CodigoEstudante)
	}
	if payload.CodigoAcademia != "ACA_01" {
		t.Fatalf("CodigoAcademia inválido: %q", payload.CodigoAcademia)
	}
	if payload.AnoLectivo != "2025_2026" {
		t.Fatalf("AnoLectivo inválido: %q", payload.AnoLectivo)
	}
	if payload.TipoEnsino != "fundamental" {
		t.Fatalf("TipoEnsino inválido: %q", payload.TipoEnsino)
	}
	if payload.AnoAcademicoAtual != "2_ano_fundamental" {
		t.Fatalf("AnoAcademicoAtual inválido: %q", payload.AnoAcademicoAtual)
	}
	if payload.ProximoAnoAcademico == nil || *payload.ProximoAnoAcademico != "3_ano_fundamental" {
		t.Fatalf("ProximoAnoAcademico inválido: %#v", payload.ProximoAnoAcademico)
	}
	if !payload.Aprovado {
		t.Fatal("Aprovado deveria ser true")
	}
	if payload.Observacao == nil || *payload.Observacao != "ok" {
		t.Fatalf("Observacao inválida: %#v", payload.Observacao)
	}
}

func TestParseAvaliacaoFinalPayloadLegacyPascalCase(t *testing.T) {
	raw := json.RawMessage(`{
		"CodigoEstudante":"EST9876",
		"CodigoAcademia":"ACA_02",
		"AnoLectivo":"2024_2025",
		"TipoEnsino":"medio",
		"AnoAcademicoAtual":"1_ano_medio",
		"Aprovado":false
	}`)

	payload, err := parseAvaliacaoFinalPayload(raw)
	if err != nil {
		t.Fatalf("parseAvaliacaoFinalPayload retornou erro: %v", err)
	}

	if payload.CodigoEstudante != "EST9876" || payload.CodigoAcademia != "ACA_02" {
		t.Fatalf("payload legado não parseado corretamente: %#v", payload)
	}
	if payload.AnoLectivo != "2024_2025" || payload.TipoEnsino != "medio" {
		t.Fatalf("campos legado inválidos: %#v", payload)
	}
	if payload.AnoAcademicoAtual != "1_ano_medio" {
		t.Fatalf("AnoAcademicoAtual legado inválido: %q", payload.AnoAcademicoAtual)
	}
}
