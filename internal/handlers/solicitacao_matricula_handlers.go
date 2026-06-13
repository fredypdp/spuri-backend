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

var solicitacaoDocFields = []string{"bi_estudante", "bi_responsavel", "cedula", "declaracao", "certificado_6_ano_fundamental", "certificado_9_ano_fundamental", "certificado_ensino_medio"}

type uploadedPDF struct {
	field string
	data  []byte
	size  int64
}

func CriarSolicitacaoMatricula(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("multipart/form-data inválido"))
		return
	}
	get := func(k string) string { return strings.TrimSpace(c.PostForm(k)) }
	codigoAcademia, nome, genero, dataNascRaw := get("codigo_academia"), get("nome"), get("genero"), get("data_nascimento")
	if codigoAcademia == "" || nome == "" || genero == "" || dataNascRaw == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia, nome, genero e data_nascimento são obrigatórios"))
		return
	}
	if genero != "masculino" && genero != "feminino" {
		utils.RespondWithValidationError(c, fmt.Errorf("genero deve ser 'masculino' ou 'feminino'"))
		return
	}
	dataNasc, err := time.Parse("2006-01-02", dataNascRaw)
	if err != nil || !dataNasc.Before(time.Now().UTC().Truncate(24*time.Hour)) {
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

	bi, biResp := get("bilhete_identidade"), get("bilhete_identidade_responsavel")
	if bi == "" && biResp == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("bilhete_identidade ou bilhete_identidade_responsavel é obrigatório"))
		return
	}
	if email := get("email"); email != "" {
		if err := utils.ValidateEmail(email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	year := firstNonEmpty(get("ano_escolar_fundamental"), get("ano_escolar_medio"), get("ano_superior"))
	requiredDecl := containsString(academia.DocumentosObrigatorios["declaracao"], year)
	requiredCertField := requiredCertificateField(academia.DocumentosObrigatorios, year)
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
	if _, ok := files["bi_estudante"]; !ok {
		if _, ok := files["bi_responsavel"]; !ok {
			utils.RespondWithValidationError(c, fmt.Errorf("documento bi_estudante ou bi_responsavel é obrigatório"))
			return
		}
	}
	if _, hasStudentBI := files["bi_estudante"]; !hasStudentBI {
		if _, ok := files["cedula"]; !ok {
			utils.RespondWithValidationError(c, fmt.Errorf("cedula é obrigatória quando apenas bi_responsavel é enviado"))
			return
		}
	}
	if requiredDecl {
		if _, ok := files["declaracao"]; !ok {
			utils.RespondWithValidationError(c, fmt.Errorf("declaracao é obrigatória para o ano académico informado"))
			return
		}
	}
	if requiredCertField != "" {
		if _, ok := files[requiredCertField]; !ok {
			utils.RespondWithValidationError(c, fmt.Errorf("%s é obrigatório para o ano académico informado", requiredCertField))
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
		p, _ := storage.NewMegaProvider()
		provider = p
	}
	dir := fmt.Sprintf("%s/matriculas/matricula_%s", codigoAcademia, codigo)
	if err := provider.EnsureDir(dir); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	documentos := map[string]string{}
	for field, f := range files {
		remote := fmt.Sprintf("%s/%s_%s.pdf", dir, field, codigo)
		if err := provider.Upload(remote, bytes.NewReader(f.data), f.size); err != nil {
			_ = provider.Delete(dir)
			utils.RespondWithInternalError(c, fmt.Errorf("falha no upload dos documentos: %w", err))
			return
		}
		documentos[field] = remote
	}

	var emailPtr, telPtr, biPtr, biRespPtr, anoFundPtr, anoMedioPtr, anoSupPtr *string
	setPtr := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	emailPtr = setPtr(get("email"))
	telPtr = setPtr(get("telefone"))
	biPtr = setPtr(bi)
	biRespPtr = setPtr(biResp)
	anoFundPtr = setPtr(get("ano_escolar_fundamental"))
	anoMedioPtr = setPtr(get("ano_escolar_medio"))
	anoSupPtr = setPtr(get("ano_superior"))
	cursoMedioID, err := parseOptionalCurso(c, get("curso_medio_id"), "medio", codigoAcademia)
	if err != nil {
		_ = provider.Delete(dir)
		utils.RespondWithValidationError(c, err)
		return
	}
	cursoSuperiorID, err := parseOptionalCurso(c, get("curso_superior_id"), "superior", codigoAcademia)
	if err != nil {
		_ = provider.Delete(dir)
		utils.RespondWithValidationError(c, err)
		return
	}
	sol := aggregates.NewSolicitacaoMatricula()
	if err := sol.Criar(codigo, codigoAcademia, nome, genero, dataNasc, emailPtr, telPtr, biPtr, biRespPtr, anoFundPtr, anoMedioPtr, cursoMedioID, anoSupPtr, cursoSuperiorID, documentos); err != nil {
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
	if err := est.CriarComVinculo(agg.Nome, codigoEstudante, string(hash), agg.Email, agg.Telefone, agg.BilheteIdentidade, agg.BilheteIdentidadeResponsavel, agg.Genero, agg.DataNascimento, agg.AnoEscolarFundamental, agg.AnoEscolarMedio, agg.AnoSuperior, agg.CursoMedioID, agg.CursoSuperiorID, &academia.ID, academia.CodigoAcademia); err != nil {
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

func AtualizarDocumentosObrigatorios(c *gin.Context) {
	academia, ok := currentAcademiaDTO(c)
	if !ok {
		return
	}
	var req map[string][]string
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("payload inválido"))
		return
	}
	docs := defaultDocumentosObrigatoriosMap()
	for key := range docs {
		docs[key] = academia.DocumentosObrigatorios[key]
		if docs[key] == nil {
			docs[key] = []string{}
		}
	}
	for key, v := range req {
		if _, ok := docs[key]; !ok {
			utils.RespondWithValidationError(c, fmt.Errorf("documento obrigatório %q não é suportado", key))
			return
		}
		docs[key] = v
	}
	if err := validarAnosDocumentosObrigatorios(c, academia.CodigoAcademia, docs); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	loaded, err := getRepository(c).WithContext(c.Request.Context()).Load(academia.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	agg := loaded.(*aggregates.Academia)
	if err := agg.AtualizarDocumentosObrigatorios(aggregates.DocumentosObrigatorios{Declaracao: docs["declaracao"], Certificado6AnoFundamental: docs["certificado_6_ano_fundamental"], Certificado9AnoFundamental: docs["certificado_9_ano_fundamental"], CertificadoEnsinoMedio: docs["certificado_ensino_medio"]}); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).WithContext(c.Request.Context()).SaveWithAudit(agg, db.AuditContext{UserID: academia.ID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "configuração de documentos obrigatórios atualizada com sucesso", "documentos_obrigatorios": docs})
}
func GetDocumentosObrigatorios(c *gin.Context) {
	userType, _ := middleware.GetUserType(c)
	var codigo string
	if userType == "academia" {
		a, ok := currentAcademiaDTO(c)
		if !ok {
			return
		}
		codigo = a.CodigoAcademia
	} else {
		codigo = strings.TrimSpace(c.Query("codigo_academia"))
		if codigo == "" {
			utils.RespondWithError(c, http.StatusNotFound, "academia não encontrada", nil)
			return
		}
	}
	a, err := getAcademiaProjection(c).GetByCodigo(codigo)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if a == nil {
		utils.RespondWithError(c, http.StatusNotFound, "academia não encontrada", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{"codigo_academia": a.CodigoAcademia, "documentos_obrigatorios": a.DocumentosObrigatorios})
}
func GetStorageQuota(c *gin.Context) {
	p := getStorageProvider(c)
	if p == nil {
		var err error
		p, err = storage.NewMegaProvider()
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
	c.JSON(http.StatusOK, gin.H{"provider": "mega", "total_bytes": q.TotalBytes, "used_bytes": q.UsedBytes, "available_bytes": q.AvailableBytes, "managed_bytes": q.ManagedBytes, "unmanaged_bytes": q.UnmanagedBytes, "total_human": storage.HumanBytes(q.TotalBytes), "used_human": storage.HumanBytes(q.UsedBytes), "available_human": storage.HumanBytes(q.AvailableBytes), "managed_human": storage.HumanBytes(q.ManagedBytes), "unmanaged_human": storage.HumanBytes(q.UnmanagedBytes), "academias": academias, "account_files": files})
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
	data, err := io.ReadAll(file)
	if err != nil {
		return uploadedPDF{}, err
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
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
func containsString(values []string, target string) bool {
	if target == "" {
		return false
	}
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
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
func defaultDocumentosObrigatoriosMap() map[string][]string {
	return map[string][]string{
		"declaracao":                    {},
		"certificado_6_ano_fundamental": {},
		"certificado_9_ano_fundamental": {},
		"certificado_ensino_medio":      {},
	}
}

func requiredCertificateField(docs map[string][]string, year string) string {
	for _, field := range []string{"certificado_6_ano_fundamental", "certificado_9_ano_fundamental", "certificado_ensino_medio"} {
		if containsString(docs[field], year) {
			return field
		}
	}
	return ""
}

func validarAnosDocumentosObrigatorios(c *gin.Context, codigoAcademia string, docs map[string][]string) error {
	cursos, err := getCursosProjection(c).GetByAcademia(codigoAcademia)
	if err != nil {
		return err
	}
	academia, err := getAcademiaProjection(c).GetByCodigo(codigoAcademia)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, a := range academia.AnosAcademicos {
		allowed[a] = true
	}
	for _, curso := range cursos {
		if curso.Status != "ativo" {
			continue
		}
		for _, a := range curso.AnosAcademicos {
			allowed[a] = true
		}
	}
	for key, anos := range docs {
		for _, ano := range anos {
			if !allowed[ano] {
				return fmt.Errorf("ano académico %q em %s não pertence à academia", ano, key)
			}
		}
	}
	return nil
}
