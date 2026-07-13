package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/storage"
	"spuri/internal/utils"
)

func estudanteDocumentoDownloadURL(codigoEstudante, campo string) string {
	return fmt.Sprintf("/documentos/estudantes/%s/%s/download", codigoEstudante, campo)
}

func estudanteDocumentoProprioDownloadURL(campo string) string {
	return fmt.Sprintf("/estudante/documentos/%s/download", campo)
}

func solicitacaoDocumentoDownloadURL(codigoSolicitacao, campo string) string {
	return fmt.Sprintf("/documentos/solicitacoes-matricula/%s/%s/download", codigoSolicitacao, campo)
}

func academiaSolicitacaoDocumentoDownloadURL(codigoSolicitacao, campo string) string {
	return fmt.Sprintf("/academia/documentos/solicitacoes-matricula/%s/%s/download", codigoSolicitacao, campo)
}

func academiaDocumentoDownloadURL(codigoAcademia, campo string) string {
	return fmt.Sprintf("/documentos/academias/%s/%s/download", codigoAcademia, campo)
}

func academiaDocumentoProprioDownloadURL(campo string) string {
	return fmt.Sprintf("/academia/documentos/academia/%s/download", campo)
}

func academiaEstudanteDocumentoDownloadURL(codigoEstudante, campo string) string {
	return fmt.Sprintf("/academia/documentos/estudantes/%s/%s/download", codigoEstudante, campo)
}

// ListarMeusDocumentosEstudante returns the authenticated student's own
// document metadata without requiring the student to know/use the generic
// administrative lookup routes.
func ListarMeusDocumentosEstudante(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondWithForbiddenError(c, "estudante não autenticado")
		return
	}

	estudante, err := getEstudanteProjection(c).GetByID(userID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": estudante.CodigoEstudante,
		"documentos":       documentosComDownloadEstudanteProprio(estudante.Documentos),
	})
}

// ListarDocumentosAcademia returns the authenticated academy's document
// inventory: its own formal document, documents of students linked to it, and
// documents attached to its enrollment requests.
func ListarDocumentosAcademia(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	if userType != "academia" && userType != "admin" {
		utils.RespondWithForbiddenError(c, "apenas academias e administradores podem consultar documentos da academia")
		return
	}

	codigoAcademia := strings.TrimSpace(c.Query("codigo_academia"))
	if userType == "academia" {
		userID, _ := middleware.GetUserID(c)
		academia, err := getAcademiaProjection(c).GetByID(userID)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if academia == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		codigoAcademia = academia.CodigoAcademia
	} else if codigoAcademia == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia é obrigatório para administradores"))
		return
	}

	academia, err := getAcademiaProjection(c).GetByCodigo(codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	estudantes, err := getEstudanteProjection(c).GetByAcademia(codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	limit := parseBoundedInt(c.Query("limit"), 1000, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	solicitacoes, err := getSolicitacaoMatriculaProjection(c).List(nil, []string{codigoAcademia}, limit, offset)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	estudantesDocs := make([]gin.H, 0, len(estudantes))
	for _, estudante := range estudantes {
		estudantesDocs = append(estudantesDocs, gin.H{
			"codigo_estudante": estudante.CodigoEstudante,
			"nome":             estudante.Nome,
			"status":           estudante.Status,
			"documentos":       documentosComDownloadEstudanteAcademia(estudante.CodigoEstudante, estudante.Documentos),
		})
	}

	solicitacoesDocs := make([]gin.H, 0, len(solicitacoes.Solicitacoes))
	for _, sol := range solicitacoes.Solicitacoes {
		solicitacoesDocs = append(solicitacoesDocs, gin.H{
			"codigo_solicitacao": sol.CodigoSolicitacao,
			"nome":               sol.Nome,
			"status":             sol.Status,
			"documentos":         documentosComDownloadSolicitacaoAcademia(sol.CodigoSolicitacao, sol.Documentos),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_academia": academia.CodigoAcademia,
		"documentos": gin.H{
			"academia": gin.H{
				"codigo_academia": academia.CodigoAcademia,
				"nome":            academia.Nome,
				"documentos": gin.H{
					"alvara": aggregates.DocumentoMatricula{
						Path:        fmt.Sprintf("%s/Documentação formal/alvara_%s.pdf", academia.CodigoAcademia, academia.CodigoAcademia),
						FileURL:     fmt.Sprintf("%s/Documentação formal/alvara_%s.pdf", academia.CodigoAcademia, academia.CodigoAcademia),
						DownloadURL: academiaDocumentoProprioDownloadURL("alvara"),
					},
				},
			},
			"estudantes":             estudantesDocs,
			"solicitacoes_matricula": solicitacoesDocs,
			"total_estudantes":       len(estudantesDocs),
			"total_solicitacoes":     solicitacoes.Total,
			"limit_solicitacoes":     limit,
			"offset_solicitacoes":    offset,
		},
	})
}

// DownloadMeuDocumentoEstudante streams a document from the authenticated
// student's own scope.
func DownloadMeuDocumentoEstudante(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondWithForbiddenError(c, "estudante não autenticado")
		return
	}
	estudante, err := getEstudanteProjection(c).GetByID(userID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	streamDocumentoEstudante(c, estudante.CodigoEstudante, strings.TrimSpace(c.Param("campo")))
}

// DownloadDocumentoAcademiaPropria streams the authenticated academy's own
// formal document. Admins may use codigo_academia as a query parameter.
func DownloadDocumentoAcademiaPropria(c *gin.Context) {
	codigoAcademia := strings.TrimSpace(c.Query("codigo_academia"))
	userType, _ := middleware.GetUserType(c)
	if userType == "academia" {
		userID, _ := middleware.GetUserID(c)
		academia, err := getAcademiaProjection(c).GetByID(userID)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if academia == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		codigoAcademia = academia.CodigoAcademia
	} else if codigoAcademia == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia é obrigatório para administradores"))
		return
	}
	streamDocumentoAcademiaPorCodigo(c, codigoAcademia, strings.TrimSpace(c.Param("campo")))
}

func DownloadDocumentoEstudanteAcademia(c *gin.Context) {
	streamDocumentoEstudante(c, strings.TrimSpace(c.Param("codigo")), strings.TrimSpace(c.Param("campo")))
}

func DownloadDocumentoSolicitacaoMatriculaAcademia(c *gin.Context) {
	streamDocumentoSolicitacaoMatricula(c, strings.TrimSpace(c.Param("codigo")), strings.TrimSpace(c.Param("campo")))
}

// DownloadDocumentoEstudante streams a student document from the configured
// storage provider. The route is intentionally backend-owned so the front end
// does not need direct Mega credentials, links, or internal node IDs.
func DownloadDocumentoEstudante(c *gin.Context) {
	streamDocumentoEstudante(c, strings.TrimSpace(c.Param("codigo")), strings.TrimSpace(c.Param("campo")))
}

func streamDocumentoEstudante(c *gin.Context, codigoEstudante, campo string) {
	estudante, err := getEstudanteProjection(c).GetByCodigo(codigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	if !canAccessEstudanteDocument(c, estudante.ID.String(), estudante.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para baixar documento deste estudante")
		return
	}
	doc, ok := estudante.Documentos[campo]
	if !ok || strings.TrimSpace(doc.Path) == "" {
		utils.RespondWithNotFoundError(c, "documento")
		return
	}
	streamDocumento(c, campo, doc)
}

// DownloadDocumentoSolicitacaoMatricula streams an enrollment-request document
// for academy/admin users authorized to inspect that request.
func DownloadDocumentoSolicitacaoMatricula(c *gin.Context) {
	streamDocumentoSolicitacaoMatricula(c, strings.TrimSpace(c.Param("codigo")), strings.TrimSpace(c.Param("campo")))
}

func streamDocumentoSolicitacaoMatricula(c *gin.Context, codigoSolicitacao, campo string) {
	sol, err := getSolicitacaoMatriculaProjection(c).GetByCodigo(codigoSolicitacao)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if sol == nil {
		utils.RespondWithNotFoundError(c, "solicitação")
		return
	}
	if !canAccessSolicitacaoDocument(c, sol.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para baixar documento desta solicitação")
		return
	}
	doc, ok := sol.Documentos[campo]
	if !ok || strings.TrimSpace(doc.Path) == "" {
		utils.RespondWithNotFoundError(c, "documento")
		return
	}
	streamDocumento(c, campo, doc)
}

// DownloadDocumentoAcademia streams formal academy documents, such as the
// alvará uploaded at registration, through the backend-owned storage boundary.
func DownloadDocumentoAcademia(c *gin.Context) {
	streamDocumentoAcademiaPorCodigo(c, strings.TrimSpace(c.Param("codigo")), strings.TrimSpace(c.Param("campo")))
}

func streamDocumentoAcademiaPorCodigo(c *gin.Context, codigoAcademia, campo string) {
	if strings.ToLower(campo) != "alvara" {
		utils.RespondWithNotFoundError(c, "documento")
		return
	}

	academia, err := getAcademiaProjection(c).GetByCodigo(codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	if !canAccessAcademiaDocument(c, academia.CodigoAcademia) {
		utils.RespondWithForbiddenError(c, "sem permissão para baixar documento desta academia")
		return
	}

	path := fmt.Sprintf("%s/Documentação formal/alvara_%s.pdf", academia.CodigoAcademia, academia.CodigoAcademia)
	streamDocumento(c, campo, aggregates.DocumentoMatricula{
		Path:        path,
		FileURL:     path,
		DownloadURL: academiaDocumentoDownloadURL(academia.CodigoAcademia, campo),
	})
}

func canAccessEstudanteDocument(c *gin.Context, estudanteID string, codigoAcademia *string) bool {
	userType, _ := middleware.GetUserType(c)
	userID, _ := middleware.GetUserID(c)
	switch userType {
	case "admin":
		return true
	case "estudante":
		return userID.String() == estudanteID
	case "academia":
		if codigoAcademia == nil {
			return false
		}
		academia, _ := getAcademiaProjection(c).GetByID(userID)
		return academia != nil && academia.CodigoAcademia == *codigoAcademia
	default:
		return false
	}
}

func canAccessAcademiaDocument(c *gin.Context, codigoAcademia string) bool {
	userType, _ := middleware.GetUserType(c)
	if userType == "admin" {
		return true
	}
	if userType != "academia" {
		return false
	}
	userID, _ := middleware.GetUserID(c)
	academia, _ := getAcademiaProjection(c).GetByID(userID)
	return academia != nil && academia.CodigoAcademia == codigoAcademia
}

func canAccessSolicitacaoDocument(c *gin.Context, codigoAcademia string) bool {
	userType, _ := middleware.GetUserType(c)
	if userType == "admin" {
		return true
	}
	if userType != "academia" {
		return false
	}
	userID, _ := middleware.GetUserID(c)
	academia, _ := getAcademiaProjection(c).GetByID(userID)
	return academia != nil && academia.CodigoAcademia == codigoAcademia
}

func streamDocumento(c *gin.Context, campo string, doc aggregates.DocumentoMatricula) {
	provider := getStorageProvider(c)
	if provider == nil {
		var err error
		provider, err = storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
	}
	r, err := provider.Read(doc.Path)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			utils.RespondWithNotFoundError(c, "documento")
			return
		}
		utils.RespondWithError(c, http.StatusServiceUnavailable, "falha ao ler documento no storage", err)
		return
	}
	defer r.Close()

	filename := safeDocumentFilename(campo)
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, filename))
	c.Header("Cache-Control", "private, max-age=300")
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, r)
}

func safeDocumentFilename(campo string) string {
	campo = strings.TrimSpace(strings.ToLower(campo))
	campo = strings.NewReplacer("/", "_", "\\", "_", "\x00", "_", "\"", "_", "'", "_").Replace(campo)
	if campo == "" || campo == "." || campo == ".." {
		campo = "documento"
	}
	if !strings.HasSuffix(campo, ".pdf") {
		campo += ".pdf"
	}
	return campo
}

func documentosComDownloadEstudante(codigoEstudante string, documentos map[string]aggregates.DocumentoMatricula) map[string]aggregates.DocumentoMatricula {
	return documentosComDownload(documentos, func(campo string) string {
		return estudanteDocumentoDownloadURL(codigoEstudante, campo)
	})
}

func documentosComDownloadEstudanteProprio(documentos map[string]aggregates.DocumentoMatricula) map[string]aggregates.DocumentoMatricula {
	return documentosComDownload(documentos, func(campo string) string {
		return estudanteDocumentoProprioDownloadURL(campo)
	})
}

func documentosComDownloadEstudanteAcademia(codigoEstudante string, documentos map[string]aggregates.DocumentoMatricula) map[string]aggregates.DocumentoMatricula {
	return documentosComDownload(documentos, func(campo string) string {
		return academiaEstudanteDocumentoDownloadURL(codigoEstudante, campo)
	})
}

func documentosComDownloadSolicitacao(codigoSolicitacao string, documentos map[string]aggregates.DocumentoMatricula) map[string]aggregates.DocumentoMatricula {
	return documentosComDownload(documentos, func(campo string) string {
		return solicitacaoDocumentoDownloadURL(codigoSolicitacao, campo)
	})
}

func documentosComDownloadSolicitacaoAcademia(codigoSolicitacao string, documentos map[string]aggregates.DocumentoMatricula) map[string]aggregates.DocumentoMatricula {
	return documentosComDownload(documentos, func(campo string) string {
		return academiaSolicitacaoDocumentoDownloadURL(codigoSolicitacao, campo)
	})
}

func documentosComDownload(documentos map[string]aggregates.DocumentoMatricula, downloadURL func(string) string) map[string]aggregates.DocumentoMatricula {
	out := make(map[string]aggregates.DocumentoMatricula, len(documentos))
	for campo, doc := range documentos {
		doc.DownloadURL = downloadURL(campo)
		out[campo] = doc
	}
	return out
}
