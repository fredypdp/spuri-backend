package handlers

import "testing"

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
		estudanteDocumentoProprioDownloadURL("bi_estudante"):              "/estudante/documentos/bi_estudante/download",
		academiaDocumentoProprioDownloadURL("alvara"):                     "/academia/documentos/academia/alvara/download",
		academiaEstudanteDocumentoDownloadURL("EST001", "bi_estudante"):   "/academia/documentos/estudantes/EST001/bi_estudante/download",
		academiaSolicitacaoDocumentoDownloadURL("SOL001", "bi_estudante"): "/academia/documentos/solicitacoes-matricula/SOL001/bi_estudante/download",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("download URL = %q, want %q", got, want)
		}
	}
}
