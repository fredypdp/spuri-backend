package handlers

import (
	"testing"

	"spuri/internal/domain/aggregates"
)

func TestSafeDocumentFilename(t *testing.T) {
	cases := map[string]string{
		"declaracao":       "declaracao.pdf",
		"BI/Estudante":     "bi_estudante.pdf",
		"documento.pdf":    "documento.pdf",
		"":                 "documento.pdf",
		"../segredo\".pdf": ".._segredo_.pdf",
	}
	for input, want := range cases {
		if got := safeDocumentFilename(input); got != want {
			t.Fatalf("safeDocumentFilename(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAcademiaDocumentoDownloadURL(t *testing.T) {
	got := academiaDocumentoDownloadURL("ACA001", "alvara")
	want := "/documentos/academias/ACA001/alvara/download"
	if got != want {
		t.Fatalf("academiaDocumentoDownloadURL() = %q, want %q", got, want)
	}
}

func TestScopedDocumentoDownloadURLs(t *testing.T) {
	cases := map[string]string{
		estudanteDocumentoProprioDownloadURL("bi_estudante"):                 "/estudante/documentos/bi_estudante/download",
		academiaDocumentoProprioDownloadURL("alvara"):                        "/academia/documentos/academia/alvara/download",
		documentosComDownloadAcademia("ACA001")["alvara"].DownloadURL:        "/documentos/academias/ACA001/alvara/download",
		documentosComDownloadAcademiaPropria("ACA001")["alvara"].DownloadURL: "/academia/documentos/academia/alvara/download",
		academiaEstudanteDocumentoDownloadURL("EST001", "bi_estudante"):      "/academia/documentos/estudantes/EST001/bi_estudante/download",
		academiaSolicitacaoDocumentoDownloadURL("SOL001", "bi_estudante"):    "/academia/documentos/solicitacoes-matricula/SOL001/bi_estudante/download",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("download URL = %q, want %q", got, want)
		}
	}
}

func TestDocumentoConsultaHelpersSemprePreenchemDownloadURLDaRota(t *testing.T) {
	documentos := map[string]aggregates.DocumentoMatricula{
		"bi_estudante": {Path: "old/path.pdf", DownloadURL: "https://storage.example/old.pdf"},
	}

	cases := map[string]map[string]aggregates.DocumentoMatricula{
		"consulta global da academia":      documentosComDownloadAcademia("ACA001"),
		"consulta própria da academia":     documentosComDownloadAcademiaPropria("ACA001"),
		"consulta global do estudante":     documentosComDownloadEstudante("EST001", documentos),
		"consulta própria do estudante":    documentosComDownloadEstudanteProprio(documentos),
		"consulta da academia/estudante":   documentosComDownloadEstudanteAcademia("EST001", documentos),
		"consulta global de solicitação":   documentosComDownloadSolicitacao("SOL001", documentos),
		"consulta da academia/solicitação": documentosComDownloadSolicitacaoAcademia("SOL001", documentos),
	}

	wants := map[string]string{
		"consulta global da academia":      "/documentos/academias/ACA001/alvara/download",
		"consulta própria da academia":     "/academia/documentos/academia/alvara/download",
		"consulta global do estudante":     "/documentos/estudantes/EST001/bi_estudante/download",
		"consulta própria do estudante":    "/estudante/documentos/bi_estudante/download",
		"consulta da academia/estudante":   "/academia/documentos/estudantes/EST001/bi_estudante/download",
		"consulta global de solicitação":   "/documentos/solicitacoes-matricula/SOL001/bi_estudante/download",
		"consulta da academia/solicitação": "/academia/documentos/solicitacoes-matricula/SOL001/bi_estudante/download",
	}

	for name, gotDocs := range cases {
		campo := "bi_estudante"
		if name == "consulta global da academia" || name == "consulta própria da academia" {
			campo = "alvara"
		}
		got := gotDocs[campo].DownloadURL
		if got != wants[name] {
			t.Fatalf("%s: download_url = %q, want %q", name, got, wants[name])
		}
	}
}

func TestDocumentoEstudantePorCampoEscopoResolveChaveAcademicaNormalizada(t *testing.T) {
	documentos := map[string]aggregates.DocumentoMatricula{
		"medio.2_ano_medio.declaracao_2_ano_medio": {
			Tipo:         "declaracao_2_ano_medio",
			Nivel:        "medio",
			AnoAcademico: "2_ano_medio",
			Path:         "docs/medio/2.pdf",
		},
		"medio.3_ano_medio.declaracao_3_ano_medio": {
			Tipo:         "declaracao_3_ano_medio",
			Nivel:        "medio",
			AnoAcademico: "3_ano_medio",
			Path:         "docs/medio/3.pdf",
		},
	}

	doc, ok := documentoEstudantePorCampoEscopo(documentos, "declaracao_3_ano_medio", "medio", "3_ano_medio")
	if !ok {
		t.Fatalf("documento acadêmico normalizado não encontrado por tipo+escopo")
	}
	if doc.Path != "docs/medio/3.pdf" {
		t.Fatalf("path = %q, want docs/medio/3.pdf", doc.Path)
	}
}

func TestDocumentoEstudantePorCampoEscopoNaoConfundeAnos(t *testing.T) {
	documentos := map[string]aggregates.DocumentoMatricula{
		"medio.2_ano_medio.declaracao": {
			Tipo:         "declaracao",
			Nivel:        "medio",
			AnoAcademico: "2_ano_medio",
			Path:         "docs/medio/2.pdf",
		},
		"medio.3_ano_medio.declaracao": {
			Tipo:         "declaracao",
			Nivel:        "medio",
			AnoAcademico: "3_ano_medio",
			Path:         "docs/medio/3.pdf",
		},
	}

	doc, ok := documentoEstudantePorCampoEscopo(documentos, "declaracao", "medio", "3_ano_medio")
	if !ok {
		t.Fatalf("documento acadêmico não encontrado pelo escopo")
	}
	if doc.Path != "docs/medio/3.pdf" {
		t.Fatalf("path = %q, want docs/medio/3.pdf", doc.Path)
	}
}
