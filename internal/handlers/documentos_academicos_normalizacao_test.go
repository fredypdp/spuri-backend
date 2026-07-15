package handlers

import (
	"strings"
	"testing"

	"spuri/internal/domain/aggregates"
)

func TestDocumentoMatriculaNormalizadoUsaChaveAcademicaPorNivelEAno(t *testing.T) {
	key, doc := documentoMatriculaNormalizado("declaracao", "3_ano_medio", "/download", "ACA/EST/documentos/medio/3_ano_medio/declaracao_3_ano_medio/doc.pdf", "file-url")

	if key != "medio.3_ano_medio.declaracao_3_ano_medio" {
		t.Fatalf("chave normalizada incorreta: %s", key)
	}
	if doc.Tipo != "declaracao_3_ano_medio" || doc.Nivel != "medio" || doc.AnoAcademico != "3_ano_medio" {
		t.Fatalf("metadados acadêmicos incorretos: %+v", doc)
	}
	legadoIndesejado := "declaracao_" + "ensino_medio"
	if strings.Contains(doc.Tipo, legadoIndesejado) || strings.Contains(key, legadoIndesejado) {
		t.Fatalf("não deve introduzir nomenclatura legada: key=%s doc=%+v", key, doc)
	}
}

func TestDocumentoMatriculaNormalizadoComBaseNaoPersisteDeclaracaoGenerica(t *testing.T) {
	key, doc := documentoMatriculaNormalizadoComBase("declaracao", "2_ano_fundamental", "", aggregates.DocumentoMatricula{Path: "manual.pdf"})

	if key != "fundamental.2_ano_fundamental.declaracao_2_ano_fundamental" {
		t.Fatalf("chave de documento informado incorreta: %s", key)
	}
	if doc.Tipo == "declaracao" || doc.Tipo != "declaracao_2_ano_fundamental" {
		t.Fatalf("declaração informada deve ser convertida para tipo explícito, recebeu %+v", doc)
	}
}

func TestStoragePathDocumentoEstudanteIncluiEscopoAcademico(t *testing.T) {
	storageTipo, path := storagePathDocumentoEstudante("ACA/estudantes/EST/documentos", "declaracao", "EST", "3_ano_medio")

	if storageTipo != "declaracao_3_ano_medio" {
		t.Fatalf("tipo de storage incorreto: %s", storageTipo)
	}
	if !strings.Contains(path, "/medio/3_ano_medio/declaracao_3_ano_medio/") {
		t.Fatalf("path não contém escopo acadêmico esperado: %s", path)
	}
}
