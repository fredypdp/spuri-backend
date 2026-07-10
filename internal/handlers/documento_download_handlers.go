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

func solicitacaoDocumentoDownloadURL(codigoSolicitacao, campo string) string {
	return fmt.Sprintf("/documentos/solicitacoes-matricula/%s/%s/download", codigoSolicitacao, campo)
}

func academiaDocumentoDownloadURL(codigoAcademia, campo string) string {
	return fmt.Sprintf("/documentos/academias/%s/%s/download", codigoAcademia, campo)
}

// DownloadDocumentoEstudante streams a student document from the configured
// storage provider. The route is intentionally backend-owned so the front end
// does not need direct Mega credentials, links, or internal node IDs.
func DownloadDocumentoEstudante(c *gin.Context) {
	codigoEstudante := strings.TrimSpace(c.Param("codigo"))
	campo := strings.TrimSpace(c.Param("campo"))

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
	codigoSolicitacao := strings.TrimSpace(c.Param("codigo"))
	campo := strings.TrimSpace(c.Param("campo"))

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
	codigoAcademia := strings.TrimSpace(c.Param("codigo"))
	campo := strings.TrimSpace(c.Param("campo"))
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
