package handlers

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"
)

func getSolicitacaoEdicaoDadoEstudanteProjection(c *gin.Context) *projections.SolicitacaoEdicaoDadoEstudanteProjection {
	return projections.NewSolicitacaoEdicaoDadoEstudanteProjection(getDbClient(c))
}

func CriarSolicitacaoEdicaoDadoEstudanteHandler(campo string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := middleware.GetUserID(c)
		est, err := getEstudanteProjection(c).GetByID(userID)
		if err != nil || est == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		if est.CodigoAcademia == nil || strings.TrimSpace(*est.CodigoAcademia) == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("estudante sem academia vinculada"))
			return
		}
		if err := c.Request.ParseMultipartForm(MaxPDFUploadBytes + 1024); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("multipart/form-data inválido"))
			return
		}
		novo := strings.TrimSpace(c.PostForm("novo_valor"))
		valorAtual, valorSolicitado, err := validarValorSolicitadoEdicao(c, est, campo, novo)
		if err != nil {
			return
		}
		pend, err := getSolicitacaoEdicaoDadoEstudanteProjection(c).ExistePendente(est.CodigoEstudante, campo)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if pend {
			utils.RespondWithConflictError(c, "já existe solicitação pendente para este campo")
			return
		}
		fh, err := c.FormFile("documento")
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("documento PDF é obrigatório"))
			return
		}
		pdf, err := readAndValidatePDF("documento", fh)
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		codigo, err := generateUniqueCodigoSolicitacaoEdicao(getDbClient(c))
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		path := fmt.Sprintf("%s/estudantes/%s/edicoes_dados_pendentes/%s_%s.pdf", *est.CodigoAcademia, est.CodigoEstudante, campo, codigo)
		provider := getStorageProvider(c)
		if provider == nil {
			utils.RespondWithInternalError(c, fmt.Errorf("storage não configurado"))
			return
		}
		stored, err := provider.Upload(path, bytes.NewReader(pdf.data), pdf.size)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		agg := aggregates.NewSolicitacaoEdicaoDadoEstudante()
		if err := agg.Criar(codigo, est.CodigoEstudante, *est.CodigoAcademia, campo, valorAtual, valorSolicitado, stored.Path, stored.FileURL, est.CodigoEstudante); err != nil {
			_ = provider.Delete(stored.Path)
			utils.RespondWithValidationError(c, err)
			return
		}
		audit := db.AuditContext{UserID: userID.String(), UserType: "estudante", IP: c.ClientIP()}
		if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
			_ = provider.Delete(stored.Path)
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"message": "solicitação criada com sucesso", "codigo_solicitacao": codigo, "campo": campo, "status": aggregates.StatusSolicitacaoPendente})
	}
}

func DecidirSolicitacaoEdicaoDadoEstudanteHandler(campo string, aprovar bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		academiaID, _ := middleware.GetUserID(c)
		academia, err := getAcademiaProjection(c).GetByID(academiaID)
		if err != nil || academia == nil {
			utils.RespondWithForbiddenError(c, "academia inválida")
			return
		}
		sol, err := getSolicitacaoEdicaoDadoEstudanteProjection(c).GetByCodigo(strings.TrimSpace(c.Param("codigo")))
		if err == sql.ErrNoRows {
			utils.RespondWithNotFoundError(c, "solicitação")
			return
		}
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if sol.CodigoAcademia != academia.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "solicitação pertence a outra academia")
			return
		}
		if sol.Campo != campo {
			utils.RespondWithValidationError(c, fmt.Errorf("solicitação não pertence ao campo desta rota"))
			return
		}
		if sol.Status != aggregates.StatusSolicitacaoPendente {
			utils.RespondWithConflictError(c, "solicitação já decidida")
			return
		}
		loaded, err := getRepository(c).WithContext(c.Request.Context()).Load(sol.ID, "SolicitacaoEdicaoDadoEstudante")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		agg := loaded.(*aggregates.SolicitacaoEdicaoDadoEstudante)
		if aprovar {
			if err := aplicarEdicaoAprovada(c, sol, academiaID.String()); err != nil {
				return
			}
			err = agg.Aprovar(academiaID.String())
		} else {
			var req struct {
				MotivoReprovacao string `json:"motivo_reprovacao"`
			}
			_ = c.ShouldBindJSON(&req)
			err = agg.Reprovar(academiaID.String(), req.MotivoReprovacao)
		}
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		audit := db.AuditContext{UserID: academiaID.String(), UserType: "academia", IP: c.ClientIP()}
		if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if p := getStorageProvider(c); p != nil {
			if err := p.Delete(sol.DocumentoTemporarioPath); err != nil {
				log.Printf("[WARN] falha ao deletar documento temporário da solicitação %s: %v", sol.CodigoSolicitacao, err)
			}
		}
		c.JSON(http.StatusOK, gin.H{"message": "solicitação decidida com sucesso", "codigo_solicitacao": sol.CodigoSolicitacao, "status": map[bool]string{true: aggregates.StatusSolicitacaoAprovada, false: aggregates.StatusSolicitacaoReprovada}[aprovar]})
	}
}

func aplicarEdicaoAprovada(c *gin.Context, sol *projections.SolicitacaoEdicaoDadoEstudanteDTO, decididoPor string) error {
	est, err := getEstudanteProjection(c).GetByCodigo(sol.CodigoEstudante)
	if err != nil || est == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return fmt.Errorf("estudante não encontrado")
	}
	if _, _, err := validarValorSolicitadoEdicao(c, est, sol.Campo, sol.ValorSolicitado); err != nil {
		return err
	}
	loaded, err := getRepository(c).WithContext(c.Request.Context()).Load(est.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return err
	}
	agg := loaded.(*aggregates.Estudante)
	switch sol.Campo {
	case aggregates.CampoEdicaoNome:
		err = agg.AlterarNomePorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor)
	case aggregates.CampoEdicaoBI:
		err = agg.AlterarBilheteIdentidadePorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor)
	case aggregates.CampoEdicaoBIEncarregado:
		err = agg.AlterarBilheteIdentidadeEncarregadoPorSolicitacao(sol.ValorSolicitado, sol.CodigoSolicitacao, decididoPor)
	case aggregates.CampoEdicaoDataNascimento:
		dt, _ := time.Parse("2006-01-02", sol.ValorSolicitado)
		err = agg.AlterarDataNascimentoPorSolicitacao(dt, sol.CodigoSolicitacao, decididoPor)
	}
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return err
	}
	audit := db.AuditContext{UserID: decididoPor, UserType: "academia", IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return err
	}
	return nil
}

func AtualizarTelefoneEncarregado(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	var raw map[string]string
	if err := c.ShouldBindJSON(&raw); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	for k := range raw {
		if k != "telefone_encarregado" {
			utils.RespondWithValidationError(c, fmt.Errorf("campo não permitido: %s", k))
			return
		}
	}
	tel := strings.TrimSpace(raw["telefone_encarregado"])
	if err := utils.ValidatePhoneStrictNational(tel); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	loaded, err := getRepository(c).Load(userID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	agg := loaded.(*aggregates.Estudante)
	if err := agg.AlterarTelefoneEncarregado(tel); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).SaveWithAudit(agg, db.AuditContext{UserID: userID.String(), UserType: "estudante", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "telefone do encarregado atualizado com sucesso"})
}

func ListarSolicitacoesEdicaoEstudante(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	est, err := getEstudanteProjection(c).GetByID(userID)
	if err != nil || est == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	listarSolicitacoesEdicao(c, "", "", est.CodigoEstudante)
}
func ListarSolicitacoesEdicaoAcademia(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	ac, err := getAcademiaProjection(c).GetByID(academiaID)
	if err != nil || ac == nil {
		utils.RespondWithForbiddenError(c, "academia inválida")
		return
	}
	listarSolicitacoesEdicao(c, ac.CodigoAcademia, c.Query("codigo_estudante"), "")
}
func listarSolicitacoesEdicao(c *gin.Context, codigoAcademia, codigoEstudanteForcado, codigoEstudanteProprio string) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	codigoEstudante := codigoEstudanteForcado
	if codigoEstudanteProprio != "" {
		codigoEstudante = codigoEstudanteProprio
	}
	itens, err := getSolicitacaoEdicaoDadoEstudanteProjection(c).List(c.Query("status"), c.Query("campo"), codigoEstudante, codigoAcademia, limit, offset)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	academiaURL := codigoAcademia != ""
	for i := range itens {
		itens[i].Documento = documentoSolicitacaoEdicao(itens[i], academiaURL)
	}
	c.JSON(http.StatusOK, gin.H{"solicitacoes": itens, "limit": limit, "offset": offset, "total": len(itens)})
}

func validarValorSolicitadoEdicao(c *gin.Context, est *projections.EstudanteDTO, campo, novo string) (string, string, error) {
	if novo == "" {
		err := fmt.Errorf("novo_valor é obrigatório")
		utils.RespondWithValidationError(c, err)
		return "", "", err
	}
	atual := ""
	switch campo {
	case aggregates.CampoEdicaoNome:
		atual = est.Nome
		if len([]rune(novo)) < 2 || len([]rune(novo)) > 150 {
			err := fmt.Errorf("nome inválido")
			utils.RespondWithValidationError(c, err)
			return "", "", err
		}
	case aggregates.CampoEdicaoBI:
		if est.BilheteIdentidade != nil {
			atual = *est.BilheteIdentidade
		}
		if err := utils.ValidateBilhete(novo); err != nil {
			utils.RespondWithValidationError(c, err)
			return "", "", err
		}
		if exists, err := getEstudanteProjection(c).BilheteIdentidadeExists(novo, est.CodigoEstudante); err != nil {
			utils.RespondWithInternalError(c, err)
			return "", "", err
		} else if exists {
			err := fmt.Errorf("bilhete_identidade já usado por outro estudante")
			utils.RespondWithValidationError(c, err)
			return "", "", err
		}
	case aggregates.CampoEdicaoBIEncarregado:
		if est.BilheteIdentidadeResp != nil {
			atual = *est.BilheteIdentidadeResp
		}
		if err := utils.ValidateBilhete(novo); err != nil {
			utils.RespondWithValidationError(c, err)
			return "", "", err
		}
	case aggregates.CampoEdicaoDataNascimento:
		atual = est.DataNascimento.Format("2006-01-02")
		dt, err := time.Parse("2006-01-02", novo)
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return "", "", err
		}
		if err := aggregates.ValidarDataNascimentoPublic(dt); err != nil {
			utils.RespondWithValidationError(c, err)
			return "", "", err
		}
		novo = dt.Format("2006-01-02")
	default:
		err := fmt.Errorf("campo inválido")
		utils.RespondWithValidationError(c, err)
		return "", "", err
	}
	if strings.EqualFold(strings.TrimSpace(atual), strings.TrimSpace(novo)) {
		err := fmt.Errorf("novo_valor deve ser diferente do valor atual")
		utils.RespondWithValidationError(c, err)
		return "", "", err
	}
	return atual, novo, nil
}

func generateUniqueCodigoSolicitacaoEdicao(client *db.Client) (string, error) {
	for i := 0; i < 20; i++ {
		code, err := generateUniqueCodigoSolicitacao(client)
		if err != nil {
			return "", err
		}
		var exists bool
		if err := client.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM projection_solicitacoes_edicao_dados_estudante WHERE codigo_solicitacao=$1)`, code).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("não foi possível gerar codigo_solicitacao único")
}
