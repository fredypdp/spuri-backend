package handlers

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
	"spuri/internal/storage"
)

// extraDocFieldPrefix é o prefixo de campo multipart usado pelo frontend
// para enviar um arquivo de documento extra: "documento_extra_<id do
// catálogo>" (ex.: "documento_extra_3f1e2c4a-...").
const extraDocFieldPrefix = "documento_extra_"

// documentoExtraUpload pareia o arquivo enviado com a definição de catálogo
// (DocumentoExtraDTO) que ele pretende satisfazer.
type documentoExtraUpload struct {
	Catalog projections.DocumentoExtraDTO
	File    uploadedPDF
}

// readAndValidateDocumentoExtra valida tamanho (mesmo limite de 10MB de
// MaxPDFUploadBytes, reaproveitado — "o limite de tamanho mantém-se o
// mesmo") e assinatura do arquivo, de acordo com o `tipo` configurado pela
// academia para este documento extra ("pdf" ou "jpg"). Mesma técnica de
// validação (Content-Type + extensão + magic bytes) já usada por
// readAndValidatePDF para os documentos fixos.
func readAndValidateDocumentoExtra(rotulo string, fh *multipart.FileHeader, tipoEsperado string) (uploadedPDF, error) {
	if fh.Size > MaxPDFUploadBytes {
		return uploadedPDF{}, fmt.Errorf("%s deve ter no máximo 10MB", rotulo)
	}
	ct := fh.Header.Get("Content-Type")
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	switch tipoEsperado {
	case "pdf":
		if !strings.EqualFold(ct, "application/pdf") || ext != ".pdf" {
			return uploadedPDF{}, fmt.Errorf("%s deve ser um arquivo PDF", rotulo)
		}
	case "jpg":
		if !(strings.EqualFold(ct, "image/jpeg") || strings.EqualFold(ct, "image/jpg")) || (ext != ".jpg" && ext != ".jpeg") {
			return uploadedPDF{}, fmt.Errorf("%s deve ser um arquivo JPG", rotulo)
		}
	default:
		return uploadedPDF{}, fmt.Errorf("tipo de documento extra desconhecido para %s", rotulo)
	}
	file, err := fh.Open()
	if err != nil {
		return uploadedPDF{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxPDFUploadBytes+1))
	if err != nil {
		return uploadedPDF{}, err
	}
	if int64(len(data)) > MaxPDFUploadBytes {
		return uploadedPDF{}, fmt.Errorf("%s deve ter no máximo 10MB", rotulo)
	}
	switch tipoEsperado {
	case "pdf":
		if len(data) < 4 || string(data[:4]) != "%PDF" {
			return uploadedPDF{}, fmt.Errorf("%s não possui assinatura PDF válida", rotulo)
		}
	case "jpg":
		if len(data) < 3 || data[0] != 0xFF || data[1] != 0xD8 || data[2] != 0xFF {
			return uploadedPDF{}, fmt.Errorf("%s não possui assinatura JPEG válida", rotulo)
		}
	}
	return uploadedPDF{field: rotulo, data: data, size: int64(len(data))}, nil
}

// parseDocumentosExtra varre o multipart form já parseado (c.Request.ParseMultipartForm
// deve ter sido chamado antes) por campos "documento_extra_<id>", validando
// cada um contra `catalogo` — as definições ATIVAS desta academia para o
// ano_academico do estudante sendo cadastrado/matriculado
// (projections.DocumentoExtraProjection.GetAtivosPorAnoAcademico).
//
// Allowlist estrita: qualquer campo "documento_extra_*" que não corresponda
// a um id presente em `catalogo` é rejeitado — mesma filosofia de
// validarCamposArquivoMatricula para os documentos fixos, evitando que um
// cliente envie arquivos para ids de outra academia ou de outro ano.
func parseDocumentosExtra(form *multipart.Form, catalogo []projections.DocumentoExtraDTO) (map[string]documentoExtraUpload, error) {
	out := map[string]documentoExtraUpload{}
	if form == nil {
		return out, nil
	}
	porID := make(map[string]projections.DocumentoExtraDTO, len(catalogo))
	for _, d := range catalogo {
		porID[d.ID.String()] = d
	}
	for field, headers := range form.File {
		if !strings.HasPrefix(field, extraDocFieldPrefix) {
			continue
		}
		if len(headers) == 0 {
			continue
		}
		catalogID := strings.TrimPrefix(field, extraDocFieldPrefix)
		cat, ok := porID[catalogID]
		if !ok {
			return nil, fmt.Errorf("documento extra não aplicável a este cadastro: %s", catalogID)
		}
		pdf, err := readAndValidateDocumentoExtra(cat.Rotulo, headers[0], cat.Tipo)
		if err != nil {
			return nil, err
		}
		out[catalogID] = documentoExtraUpload{Catalog: cat, File: pdf}
	}
	return out, nil
}

// validarObrigatoriedadeDocumentosExtra garante que todo documento extra
// com obrigatorio=true em `catalogo` foi efetivamente enviado em `enviados`.
// Assim como a validação dos documentos fixos (aggregates.ValidarDocumentosMatricula),
// esta checagem deve ser pulada quando a academia registra o estudante em
// modo "pendente de documentos".
func validarObrigatoriedadeDocumentosExtra(catalogo []projections.DocumentoExtraDTO, enviados map[string]documentoExtraUpload) error {
	for _, cat := range catalogo {
		if !cat.Obrigatorio {
			continue
		}
		if _, ok := enviados[cat.ID.String()]; !ok {
			return fmt.Errorf("documento obrigatório ausente: %s", cat.Rotulo)
		}
	}
	return nil
}

// storagePathDocumentoExtra monta o caminho de armazenamento no formato
// pedido para esta tarefa: ".../documento_extra/{id do documento}/...".
// {id do documento} aqui é o id da DEFINIÇÃO no catálogo (DocumentoExtra.ID)
// — não o id do arquivo em si, que vira o nome do arquivo
// (documento_id.{ext}, mesmo padrão dos documentos fixos).
func storagePathDocumentoExtra(baseDir, catalogID, tipo string) (string, string) {
	documentoID := uuid.NewString()
	return documentoID, fmt.Sprintf("%s/documento_extra/%s/%s.%s", baseDir, catalogID, documentoID, tipo)
}

// armazenarDocumentosExtra envia cada arquivo validado ao storage provider e
// devolve o mapa pronto para ser mesclado no `documentos` do estudante, sob
// a chave "documento_extra.<id do catálogo>". `downloadURL` monta a URL de
// download definitiva para cada campo — reaproveita as MESMAS rotas
// genéricas de download já existentes para os documentos fixos
// (estudanteDocumentoDownloadURL / solicitacaoDocumentoDownloadURL etc.),
// já que elas resolvem "campo" por lookup direto na chave do mapa
// Documentos (ver documentoEstudantePorCampoEscopo) — chaves com ponto,
// como a nossa, já são um padrão existente (ex.: "nivel.ano.campo" para
// declarações), então nenhuma rota nova precisou ser criada.
func armazenarDocumentosExtra(provider storage.StorageProvider, dir string, enviados map[string]documentoExtraUpload, downloadURL func(campo string) string) (map[string]aggregates.DocumentoMatricula, error) {
	out := map[string]aggregates.DocumentoMatricula{}
	for catalogID, up := range enviados {
		documentoID, storagePath := storagePathDocumentoExtra(dir, catalogID, up.Catalog.Tipo)
		stored, err := provider.Upload(storagePath, bytes.NewReader(up.File.data), up.File.size)
		if err != nil {
			return nil, err
		}
		campo := "documento_extra." + catalogID
		out[campo] = aggregates.DocumentoMatricula{
			DocumentoID:      documentoID,
			Tipo:             up.Catalog.Tipo,
			Nivel:            up.Catalog.Nivel,
			AnoAcademico:     up.Catalog.AnoAcademico,
			DocumentoExtraID: catalogID,
			Path:             stored.Path,
			FileURL:          stored.FileURL,
			DownloadURL:      downloadURL(campo),
		}
	}
	return out, nil
}

// resolverAnoAcademicoParaDocumentosExtra escolhe, entre os três campos de
// ano possíveis de uma matrícula/cadastro, qual está preenchido — mesma
// regra de exclusividade mútua já validada em aggregates.ValidarDocumentosMatricula
// (um estudante pertence a exatamente um nível por vez).
func resolverAnoAcademicoParaDocumentosExtra(anoFundamental, anoMedio, anoSuperior *string) string {
	if anoFundamental != nil && strings.TrimSpace(*anoFundamental) != "" {
		return strings.TrimSpace(*anoFundamental)
	}
	if anoMedio != nil && strings.TrimSpace(*anoMedio) != "" {
		return strings.TrimSpace(*anoMedio)
	}
	if anoSuperior != nil && strings.TrimSpace(*anoSuperior) != "" {
		return strings.TrimSpace(*anoSuperior)
	}
	return ""
}
