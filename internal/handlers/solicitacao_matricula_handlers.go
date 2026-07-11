package handlers

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/services"
	"spuri/internal/storage"
	"spuri/internal/utils"
)

const MaxPDFUploadBytes int64 = 10 << 20

var solicitacaoDocFields = []string{"bi_estudante", "bi_responsavel", "cedula_estudante", "declaracao", "certificado_6_ano_fundamental", "certificado_9_ano_fundamental", "certificado_ensino_medio"}

var solicitacaoDocFieldSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(solicitacaoDocFields))
	for _, field := range solicitacaoDocFields {
		set[field] = struct{}{}
	}
	return set
}()

type uploadedPDF struct {
	field string
	data  []byte
	size  int64
}

func isMatriculaEscolar(anoFund, anoMedio *string) bool {
	return anoFund != nil && strings.TrimSpace(*anoFund) != "" || anoMedio != nil && strings.TrimSpace(*anoMedio) != ""
}

func stringPtrIfNotBlank(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}

func validateDocumentosMatricula(bi *string, biResp *string, anoFund, anoMedio, anoSuperior *string, documentos map[string]aggregates.DocumentoMatricula, contexto string) error {
	return aggregates.ValidarDocumentosMatricula(bi, biResp, anoFund, anoMedio, anoSuperior, documentos)
}

func validateBIResponsavelNaoConflitaComEscolar(c *gin.Context, biResp *string, excludeID *uuid.UUID) error {
	if biResp == nil || strings.TrimSpace(*biResp) == "" {
		return nil
	}
	var existente *projections.EstudanteDTO
	var err error
	if excludeID != nil {
		existente, err = getEstudanteProjection(c).GetEscolarByBilheteIdentidadePrincipalExcludingID(*biResp, *excludeID)
	} else {
		existente, err = getEstudanteProjection(c).GetEscolarByBilheteIdentidadePrincipal(*biResp)
	}
	if err != nil {
		return err
	}
	if existente != nil {
		return fmt.Errorf("bilhete_identidade_responsavel não pode coincidir com o bilhete_identidade principal de outro estudante escolar")
	}
	return nil
}

func CriarSolicitacaoMatricula(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("multipart/form-data inválido"))
		return
	}
	if err := validarCamposArquivoMatricula(c.Request.MultipartForm); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	get := func(k string) string { return strings.TrimSpace(c.PostForm(k)) }
	codigoAcademia, nome, genero, dataNascRaw := get("codigo_academia"), get("nome"), get("genero"), get("data_nascimento")
	if codigoAcademia == "" || nome == "" || genero == "" || dataNascRaw == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia, nome, genero e data_nascimento são obrigatórios"))
		return
	}
	dataNasc, err := time.Parse("2006-01-02", dataNascRaw)
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("data_nascimento deve ser YYYY-MM-DD anterior à data atual"))
		return
	}
	academia, err := getAcademiaProjection(c).GetByCodigo(codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academia == nil || academia.Status != "ativo" {
		utils.RespondWithError(c, http.StatusForbidden, "academia inativa ou não encontrada", nil)
		return
	}

	cursoMedioID, cursoSuperiorID, err := validarCursosMatriculaCommon(c, codigoAcademia, get("curso_medio_id"), get("curso_superior_id"))
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	bi, biResp := get("bilhete_identidade"), get("bilhete_identidade_responsavel")

	year := firstNonEmpty(get("ano_escolar_fundamental"), get("ano_escolar_medio"), get("ano_superior"))
	_, err = certificateFieldForMatricula(c, academia, year)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	files := map[string]uploadedPDF{}
	for _, field := range solicitacaoDocFields {
		if fh, err := c.FormFile(field); err == nil {
			pdf, err := readAndValidatePDF(field, fh)
			if err != nil {
				utils.RespondWithValidationError(c, err)
				return
			}
			files[field] = pdf
		}
	}
	documentosParaValidacao := documentosMatriculaParaValidacao(files, get("declaracao_ano_academico"))
	biPtr, biRespPtr := stringPtrIfNotBlank(bi), stringPtrIfNotBlank(biResp)
	anoFundPtr := stringPtrIfNotBlank(get("ano_escolar_fundamental"))
	anoMedioPtr := stringPtrIfNotBlank(get("ano_escolar_medio"))
	anoSupPtr := stringPtrIfNotBlank(get("ano_superior"))
	validado, err := services.ValidateMatriculaCommon(services.MatriculaCommonInput{
		Contexto: services.MatriculaContextSolicitacao,
		Nome:     nome, Genero: genero, DataNascimento: dataNasc,
		Email: stringPtrIfNotBlank(get("email")), TelefoneEstudante: stringPtrIfNotBlank(get("telefone")), TelefoneResponsavel: stringPtrIfNotBlank(get("telefone_responsavel")),
		BilheteIdentidade: biPtr, BilheteIdentidadeResponsavel: biRespPtr,
		AnoEscolarFundamental: anoFundPtr, AnoEscolarMedio: anoMedioPtr, AnoSuperior: anoSupPtr,
		Documentos: documentosParaValidacao,
	})
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	biPtr, biRespPtr = validado.BilheteIdentidade, validado.BilheteIdentidadeResponsavel
	anoFundPtr, anoMedioPtr, anoSupPtr = validado.AnoEscolarFundamental, validado.AnoEscolarMedio, validado.AnoSuperior
	if isMatriculaEscolar(anoFundPtr, anoMedioPtr) {
		if err := validateBIResponsavelNaoConflitaComEscolar(c, biRespPtr, nil); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	codigo, err := generateUniqueCodigoSolicitacao(getDbClient(c))
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	provider := getStorageProvider(c)
	if provider == nil {
		p, _ := storage.NewStorageProvider()
		provider = p
	}
	dir := fmt.Sprintf("%s/matriculas/matricula_%s", codigoAcademia, codigo)
	if err := provider.EnsureDir(dir); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	documentos := map[string]aggregates.DocumentoMatricula{}
	for field, f := range files {
		remote := fmt.Sprintf("%s/%s_%s.pdf", dir, field, codigo)
		stored, err := provider.Upload(remote, bytes.NewReader(f.data), f.size)
		if err != nil {
			_ = provider.Delete(dir)
			utils.RespondWithInternalError(c, fmt.Errorf("falha no upload dos documentos: %w", err))
			return
		}
		documentos[field] = aggregates.DocumentoMatricula{Path: stored.Path, FileURL: stored.FileURL, DownloadURL: solicitacaoDocumentoDownloadURL(codigo, field)}
		if field == "declaracao" {
			doc := documentos[field]
			doc.AnoAcademico = get("declaracao_ano_academico")
			documentos[field] = doc
		}
	}

	emailPtr := validado.Email
	telPtr := validado.TelefoneEstudante
	sol := aggregates.NewSolicitacaoMatricula()
	if err := sol.Criar(codigo, codigoAcademia, nome, genero, dataNasc, emailPtr, telPtr, validado.TelefoneResponsavel, biPtr, biRespPtr, anoFundPtr, anoMedioPtr, cursoMedioID, anoSupPtr, cursoSuperiorID, documentos); err != nil {
		_ = provider.Delete(dir)
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).WithContext(c.Request.Context()).Save(sol); err != nil {
		_ = provider.Delete(dir)
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "solicitação de matrícula criada com sucesso", "codigo_solicitacao": codigo, "codigo_academia": codigoAcademia, "status": "pendente"})
}

func ListarSolicitacoesMatriculaAcademia(c *gin.Context) {
	academia, ok := currentAcademiaDTO(c)
	if !ok {
		return
	}
	listSolicitacoes(c, []string{academia.CodigoAcademia})
}
func ListarSolicitacoesMatriculaAdmin(c *gin.Context) {
	listSolicitacoes(c, c.QueryArray("codigo_academia"))
}
func listSolicitacoes(c *gin.Context, codigos []string) {
	limit := parseBoundedInt(c.Query("limit"), 50, 1, 1000)
	offset := parseBoundedInt(c.Query("offset"), 0, 0, 1_000_000)
	res, err := getSolicitacaoMatriculaProjection(c).List(c.QueryArray("status"), codigos, limit, offset)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"solicitacoes": res.Solicitacoes, "total": res.Total, "limit": limit, "offset": offset})
}
func GetSolicitacaoMatriculaAcademia(c *gin.Context) {
	academia, ok := currentAcademiaDTO(c)
	if !ok {
		return
	}
	sol, err := getSolicitacaoMatriculaProjection(c).GetByCodigo(c.Param("codigo"))
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if sol == nil {
		utils.RespondWithError(c, http.StatusNotFound, "solicitação não encontrada", nil)
		return
	}
	if sol.CodigoAcademia != academia.CodigoAcademia {
		utils.RespondWithError(c, http.StatusForbidden, "solicitação não pertence à academia autenticada", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"solicitacao": sol})
}

func AprovarSolicitacaoMatricula(c *gin.Context) {
	academia, ok := currentAcademiaDTO(c)
	if !ok {
		return
	}
	solDTO, agg, ok := loadSolicitacaoByCodigo(c, c.Param("codigo"))
	if !ok {
		return
	}
	if solDTO.CodigoAcademia != academia.CodigoAcademia {
		utils.RespondWithError(c, http.StatusForbidden, "solicitação não pertence à academia autenticada", nil)
		return
	}
	if agg.Status != aggregates.StatusSolicitacaoPendente {
		utils.RespondWithError(c, http.StatusConflict, "solicitação já foi aprovada ou reprovada", nil)
		return
	}
	if isMatriculaEscolar(agg.AnoEscolarFundamental, agg.AnoEscolarMedio) || (agg.AnoSuperior != nil && strings.TrimSpace(*agg.AnoSuperior) != "") {
		if err := validateDocumentosMatricula(agg.BilheteIdentidade, agg.BilheteIdentidadeResponsavel, agg.AnoEscolarFundamental, agg.AnoEscolarMedio, agg.AnoSuperior, agg.Documentos, "aprovação da solicitação"); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		if err := validateBIResponsavelNaoConflitaComEscolar(c, agg.BilheteIdentidadeResponsavel, nil); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}
	if agg.BilheteIdentidade != nil && strings.TrimSpace(*agg.BilheteIdentidade) != "" {
		existente, err := getEstudanteProjection(c).GetByBilheteIdentidadePrincipal(*agg.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado"))
			return
		}
	}
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(getDbClient(c).DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(services.GetDefaultPassword("estudante", codigoEstudante)), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	est := aggregates.NewEstudante()
	if err := est.CriarComVinculo(agg.Nome, codigoEstudante, string(hash), agg.Email, utils.NormalizePhonePtr(agg.Telefone), utils.NormalizePhonePtr(agg.TelefoneResponsavel), agg.BilheteIdentidade, agg.BilheteIdentidadeResponsavel, agg.Genero, agg.DataNascimento, agg.AnoEscolarFundamental, agg.AnoEscolarMedio, agg.AnoSuperior, agg.CursoMedioID, agg.CursoSuperiorID, &academia.ID, academia.CodigoAcademia, agg.Documentos); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	repo := getRepository(c).WithContext(c.Request.Context())
	audit := db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: c.ClientIP()}
	if err := repo.SaveWithAudit(est, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if err := agg.Aprovar(academia.ID, codigoEstudante); err != nil {
		utils.RespondWithError(c, http.StatusConflict, err.Error(), nil)
		return
	}
	if err := repo.SaveWithAudit(agg, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "solicitação aprovada e estudante registado com sucesso", "codigo_solicitacao": agg.CodigoSolicitacao, "codigo_estudante_gerado": codigoEstudante})
}

func ReprovarSolicitacaoMatricula(c *gin.Context) {
	academia, ok := currentAcademiaDTO(c)
	if !ok {
		return
	}
	_, agg, ok := loadSolicitacaoByCodigo(c, c.Param("codigo"))
	if !ok {
		return
	}
	if agg.CodigoAcademia != academia.CodigoAcademia {
		utils.RespondWithError(c, http.StatusForbidden, "solicitação não pertence à academia autenticada", nil)
		return
	}
	var req struct {
		Motivo string `json:"motivo_reprovacao"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Motivo) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("motivo_reprovacao é obrigatório"))
		return
	}
	if err := agg.Reprovar(academia.ID, req.Motivo); err != nil {
		utils.RespondWithError(c, http.StatusConflict, err.Error(), nil)
		return
	}
	if err := getRepository(c).WithContext(c.Request.Context()).SaveWithAudit(agg, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if p := getStorageProvider(c); p != nil {
		if err := p.Delete(fmt.Sprintf("%s/matriculas/matricula_%s", agg.CodigoAcademia, agg.CodigoSolicitacao)); err != nil {
			log.Printf("[WARN] falha ao deletar documentos da solicitação %s: %v", agg.CodigoSolicitacao, err)
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "solicitação reprovada com sucesso", "codigo_solicitacao": agg.CodigoSolicitacao})
}

func GetStorageQuota(c *gin.Context) {
	p := getStorageProvider(c)
	if p == nil {
		var err error
		p, err = storage.NewStorageProvider()
		if err != nil {
			utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
			return
		}
	}
	q, err := p.GetQuota()
	if err != nil {
		utils.RespondWithError(c, http.StatusServiceUnavailable, err.Error(), err)
		return
	}
	academias := make([]gin.H, 0, len(q.Academias))
	for _, a := range q.Academias {
		academias = append(academias, gin.H{"codigo_academia": a.CodigoAcademia, "used_bytes": a.UsedBytes, "used_human": storage.HumanBytes(a.UsedBytes)})
	}
	files := make([]gin.H, 0, len(q.AccountFiles))
	for _, f := range q.AccountFiles {
		files = append(files, gin.H{"path": f.Path, "name": f.Name, "size_bytes": f.SizeBytes, "size_human": storage.HumanBytes(f.SizeBytes), "managed": f.Managed})
	}
	folders := make([]gin.H, 0, len(q.AccountFolders))
	for _, f := range q.AccountFolders {
		folders = append(folders, gin.H{"path": f.Path, "name": f.Name, "size_bytes": f.SizeBytes, "size_human": storage.HumanBytes(f.SizeBytes), "managed": f.Managed})
	}
	c.JSON(http.StatusOK, gin.H{"provider": p.ProviderName(), "total_bytes": q.TotalBytes, "used_bytes": q.UsedBytes, "available_bytes": q.AvailableBytes, "managed_bytes": q.ManagedBytes, "outside_academias_bytes": q.OutsideAcademiasBytes, "unmanaged_bytes": q.UnmanagedBytes, "total_human": storage.HumanBytes(q.TotalBytes), "used_human": storage.HumanBytes(q.UsedBytes), "available_human": storage.HumanBytes(q.AvailableBytes), "managed_human": storage.HumanBytes(q.ManagedBytes), "outside_academias_human": storage.HumanBytes(q.OutsideAcademiasBytes), "unmanaged_human": storage.HumanBytes(q.UnmanagedBytes), "academias": academias, "account_files": files, "account_folders": folders})
}

func currentAcademiaDTO(c *gin.Context) (*projections.AcademiaDTO, bool) {
	id, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return nil, false
	}
	a, err := getAcademiaProjection(c).GetByID(id)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return nil, false
	}
	if a == nil {
		utils.RespondWithError(c, http.StatusNotFound, "academia não encontrada", nil)
		return nil, false
	}
	return a, true
}

func loadSolicitacaoByCodigo(c *gin.Context, codigo string) (*projections.SolicitacaoMatriculaDTO, *aggregates.SolicitacaoMatricula, bool) {
	dto, err := getSolicitacaoMatriculaProjection(c).GetByCodigo(codigo)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return nil, nil, false
	}
	if dto == nil {
		utils.RespondWithError(c, http.StatusNotFound, "solicitação não encontrada", nil)
		return nil, nil, false
	}
	loaded, err := getRepository(c).WithContext(c.Request.Context()).Load(dto.ID, "SolicitacaoMatricula")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return nil, nil, false
	}
	agg, ok := loaded.(*aggregates.SolicitacaoMatricula)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("aggregate inválido"))
		return nil, nil, false
	}
	return dto, agg, true
}

func readAndValidatePDF(field string, fh *multipart.FileHeader) (uploadedPDF, error) {
	if fh.Size > MaxPDFUploadBytes {
		return uploadedPDF{}, fmt.Errorf("%s deve ter no máximo 10MB", field)
	}
	if !strings.EqualFold(fh.Header.Get("Content-Type"), "application/pdf") {
		return uploadedPDF{}, fmt.Errorf("%s deve ser PDF", field)
	}
	if strings.ToLower(filepath.Ext(fh.Filename)) != ".pdf" {
		return uploadedPDF{}, fmt.Errorf("%s deve ter extensão .pdf", field)
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
		return uploadedPDF{}, fmt.Errorf("%s deve ter no máximo 10MB", field)
	}
	if len(data) < 4 || string(data[:4]) != "%PDF" {
		return uploadedPDF{}, fmt.Errorf("%s não possui assinatura PDF válida", field)
	}
	return uploadedPDF{field: field, data: data, size: int64(len(data))}, nil
}
func generateUniqueCodigoSolicitacao(client *db.Client) (string, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 11)
	for attempts := 0; attempts < 20; attempts++ {
		r := make([]byte, 11)
		if _, err := rand.Read(r); err != nil {
			return "", err
		}
		for i := range r {
			b[i] = alphabet[int(r[i])%len(alphabet)]
		}
		code := string(b)
		var exists bool
		if err := client.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM projection_solicitacoes_matricula WHERE codigo_solicitacao=$1)`, code).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("não foi possível gerar codigo_solicitacao único")
}
func parseOptionalCurso(c *gin.Context, raw, tipo, codigoAcademia string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("curso_%s_id inválido", tipo)
	}
	curso, err := getCursosProjection(c).GetByID(id)
	if err != nil {
		return nil, err
	}
	if curso == nil || curso.Type != tipo || curso.CodigoAcademia != codigoAcademia || curso.Status != "ativo" {
		return nil, fmt.Errorf("curso_%s_id inválido para a academia", tipo)
	}
	return &id, nil
}

func validarCursosMatriculaCommon(c *gin.Context, codigoAcademia, cursoMedioIDRaw, cursoSuperiorIDRaw string) (*uuid.UUID, *uuid.UUID, error) {
	cursoMedioID, err := parseOptionalCurso(c, cursoMedioIDRaw, "medio", codigoAcademia)
	if err != nil {
		return nil, nil, err
	}
	cursoSuperiorID, err := parseOptionalCurso(c, cursoSuperiorIDRaw, "superior", codigoAcademia)
	if err != nil {
		return nil, nil, err
	}
	return cursoMedioID, cursoSuperiorID, nil
}

func validarCamposArquivoMatricula(form *multipart.Form) error {
	if form == nil {
		return nil
	}
	for field := range form.File {
		if _, ok := solicitacaoDocFieldSet[field]; !ok {
			return fmt.Errorf("campo de arquivo não suportado para matrícula: %s", field)
		}
	}
	return nil
}

func documentosMatriculaParaValidacao(files map[string]uploadedPDF, declaracaoAnoAcademico string) map[string]aggregates.DocumentoMatricula {
	documentos := map[string]aggregates.DocumentoMatricula{}
	for field := range files {
		documento := aggregates.DocumentoMatricula{Path: field + ".pdf"}
		if field == "declaracao" {
			documento.AnoAcademico = declaracaoAnoAcademico
		}
		documentos[field] = documento
	}
	return documentos
}
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
func parseBoundedInt(raw string, def, min, max int) int {
	if raw == "" {
		return def
	}
	var v int
	if _, err := fmt.Sscanf(raw, "%d", &v); err != nil {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func certificateFieldForMatricula(c *gin.Context, academia *projections.AcademiaDTO, year string) (string, error) {
	if year == "" {
		return "", fmt.Errorf("informe o ano académico da matrícula")
	}
	if strings.HasSuffix(year, "_ano_fundamental") {
		if academia.Nivel != "escola" {
			return "", fmt.Errorf("o ano fundamental informado não pertence a uma academia de nível superior")
		}
		if year == "7_ano_fundamental" {
			return "certificado_6_ano_fundamental", nil
		}
		return "", nil
	}
	if strings.HasSuffix(year, "_ano_medio") {
		if err := ensureActiveCourseYear(c, academia.CodigoAcademia, "medio", year); err != nil {
			return "", err
		}
		if year == "1_ano_medio" {
			return "certificado_9_ano_fundamental", nil
		}
		return "", nil
	}
	if strings.HasSuffix(year, "_ano_superior") {
		if err := ensureActiveCourseYear(c, academia.CodigoAcademia, "superior", year); err != nil {
			return "", err
		}
		if year == "1_ano_superior" {
			return "certificado_ensino_medio", nil
		}
		return "", nil
	}
	return "", fmt.Errorf("ano académico inválido para matrícula: %s", year)
}

func ensureActiveCourseYear(c *gin.Context, codigoAcademia, tipo, year string) error {
	cursos, err := getCursosProjection(c).GetByAcademia(codigoAcademia)
	if err != nil {
		return err
	}
	for _, curso := range cursos {
		if curso.Status != "ativo" || curso.Type != tipo {
			continue
		}
		for _, ano := range curso.AnosAcademicos {
			if ano == year {
				return nil
			}
		}
	}
	if tipo == "medio" {
		return fmt.Errorf("o ano do ensino médio informado não está ativo para esta academia")
	}
	return fmt.Errorf("o ano do ensino superior informado não está ativo para esta academia")
}

func documentLabel(field string) string {
	switch field {
	case "certificado_6_ano_fundamental":
		return "certificado do 6.º ano fundamental"
	case "certificado_9_ano_fundamental":
		return "certificado do 9.º ano fundamental"
	case "certificado_ensino_medio":
		return "certificado do ensino médio"
	default:
		return field
	}
}
