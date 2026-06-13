package handlers

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"
)

func TestReadAndValidatePDFRejectsFilesLargerThanFiveMB(t *testing.T) {
	fh := multipartFileHeader(t, "documento", "documento.pdf", "application/pdf", append([]byte("%PDF"), bytes.Repeat([]byte("a"), int(maxSolicitacaoDocumentoBytes))...))

	_, err := readAndValidatePDF("documento", fh)
	if err == nil {
		t.Fatalf("esperava erro para documento maior que 5MB")
	}
	if !strings.Contains(err.Error(), "no máximo 5MB") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestReadAndValidatePDFAcceptsFilesUpToFiveMB(t *testing.T) {
	fh := multipartFileHeader(t, "documento", "documento.pdf", "application/pdf", append([]byte("%PDF"), bytes.Repeat([]byte("a"), int(maxSolicitacaoDocumentoBytes)-4)...))

	pdf, err := readAndValidatePDF("documento", fh)
	if err != nil {
		t.Fatalf("não esperava erro para documento com 5MB exatos: %v", err)
	}
	if pdf.size != maxSolicitacaoDocumentoBytes {
		t.Fatalf("tamanho inesperado: recebido=%d esperado=%d", pdf.size, maxSolicitacaoDocumentoBytes)
	}
}

func multipartFileHeader(t *testing.T, field, filename, contentType string, data []byte) *multipart.FileHeader {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("falha ao criar multipart part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("falha ao escrever multipart part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("falha ao fechar multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, "/", body)
	if err != nil {
		t.Fatalf("falha ao criar request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(maxSolicitacaoDocumentoBytes + 1024); err != nil {
		t.Fatalf("falha ao parsear multipart form: %v", err)
	}
	files := req.MultipartForm.File[field]
	if len(files) != 1 {
		t.Fatalf("esperava 1 arquivo, recebeu %d", len(files))
	}
	return files[0]
}
