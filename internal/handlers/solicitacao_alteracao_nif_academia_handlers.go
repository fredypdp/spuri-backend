package handlers

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"
)

// ============================================================================
// Academia: criar e listar as próprias solicitações de alteração de NIF
// ============================================================================

// CriarSolicitacaoAlteracaoNIFAcademiaHandler cria um pedido de alteração de
// NIF para a academia autenticada. Nada muda em projection_academias aqui —
// apenas o pedido é gravado (ledger + projeção de solicitações) com status
// "pendente". A alteração real só acontece se um Admin (role "adm" ou "fpp")
// aprovar, via DecidirSolicitacaoAlteracaoNIFAcademiaHandler(true).
func CriarSolicitacaoAlteracaoNIFAcademiaHandler(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	academia, err := getAcademiaProjection(c).GetByID(academiaID)
	if err != nil || academia == nil {
		utils.RespondWithForbiddenError(c, "academia inválida")
		return
	}

	var req struct {
		NovoNIF string `json:"novo_nif"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	novoNif := strings.TrimSpace(req.NovoNIF)
	if novoNif == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_nif é obrigatório"))
		return
	}
	if err := utils.ValidateNIF(novoNif); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if strings.EqualFold(strings.TrimSpace(academia.NIF), novoNif) {
		utils.RespondWithValidationError(c, fmt.Errorf("novo_nif deve ser diferente do nif atual"))
		return
	}

	guardKey := db.CanonicalGuardKey(academia.CodigoAcademia)
	guard, err := db.NewUniqueOperationGuard(getDbClient(c)).WithContext(c.Request.Context()).Reserve(
		"solicitacao_alteracao_nif_academia:pendente",
		guardKey,
		db.UniqueGuardOptions{UserID: academiaID.String(), UserType: "academia"},
	)
	if errors.Is(err, db.ErrUniqueOperationInProgress) {
		log.Printf("⚠️ [UniqueGuard] conflito scope=solicitacao_alteracao_nif_academia:pendente key_hash=%s user=%s", db.MaskGuardKey(guardKey), academiaID.String())
		utils.RespondWithConflictError(c, "já existe solicitação de alteração de NIF pendente ou em criação para esta academia")
		return
	}
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	guardConsumed := false
	defer func() {
		if !guardConsumed {
			_ = guard.Release()
		}
	}()

	pend, err := getSolicitacaoAlteracaoNIFAcademiaProjection(c).ExistePendente(academia.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if pend {
		utils.RespondWithConflictError(c, "já existe solicitação de alteração de NIF pendente para esta academia")
		return
	}

	codigo, err := generateUniqueCodigoSolicitacaoAlteracaoNIF(getDbClient(c))
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	agg := aggregates.NewSolicitacaoAlteracaoNIFAcademia()
	if err := agg.Criar(codigo, academia.CodigoAcademia, academia.NIF, novoNif, academia.CodigoAcademia); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	audit := db.AuditContext{UserID: academiaID.String(), UserType: "academia", IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if err := guard.Consume(agg.GetID()); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	guardConsumed = true

	c.JSON(http.StatusCreated, gin.H{
		"message":            "solicitação de alteração de NIF criada com sucesso",
		"codigo_solicitacao": codigo,
		"nif_atual":          academia.NIF,
		"nif_solicitado":     novoNif,
		"status":             aggregates.StatusSolicitacaoPendente,
	})
}

// ListarSolicitacoesNIFAcademia lista as solicitações de alteração de NIF da
// própria academia autenticada (qualquer status).
func ListarSolicitacoesNIFAcademia(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	academia, err := getAcademiaProjection(c).GetByID(academiaID)
	if err != nil || academia == nil {
		utils.RespondWithForbiddenError(c, "academia inválida")
		return
	}
	listarSolicitacoesAlteracaoNIF(c, c.Query("status"), academia.CodigoAcademia)
}

// ============================================================================
// Admin: listar e decidir solicitações de alteração de NIF
// ============================================================================

// ListarSolicitacoesNIFAdmin lista solicitações de alteração de NIF de
// qualquer academia. Visível a qualquer admin autenticado (role "gerente" ou
// superior); apenas a decisão (aprovar/reprovar) exige role "adm" ou "fpp"
// (ver middleware.RequireAdm() nas rotas correspondentes em main.go).
func ListarSolicitacoesNIFAdmin(c *gin.Context) {
	listarSolicitacoesAlteracaoNIF(c, c.Query("status"), c.Query("codigo_academia"))
}

func listarSolicitacoesAlteracaoNIF(c *gin.Context, status, codigoAcademia string) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	itens, err := getSolicitacaoAlteracaoNIFAcademiaProjection(c).List(status, codigoAcademia, limit, offset)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"solicitacoes": itens, "limit": limit, "offset": offset, "total": len(itens)})
}

// DecidirSolicitacaoAlteracaoNIFAcademiaHandler aprova ou reprova uma
// solicitação pendente de alteração de NIF. Protegido por
// middleware.RequireAdm() na rota (role "adm" ou "fpp" — hierarquia
// fpp=3 >= adm=2 já cobre "ADM ou FPP").
//
//   - Aprovado: Academia.AlterarNIFPorSolicitacao é chamado ANTES de marcar a
//     solicitação como aprovada — só altera o dado se a solicitação puder ser
//     salva; se a alteração da Academia falhar, a solicitação continua
//     pendente.
//   - Reprovado: nenhum dado da Academia é tocado; apenas a solicitação muda
//     de status.
func DecidirSolicitacaoAlteracaoNIFAcademiaHandler(aprovar bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		adminID, _ := middleware.GetUserID(c)
		admin, err := getAdminProjection(c).GetByID(adminID)
		if err != nil || admin == nil {
			utils.RespondWithForbiddenError(c, "administrador inválido")
			return
		}

		sol, err := getSolicitacaoAlteracaoNIFAcademiaProjection(c).GetByCodigo(strings.TrimSpace(c.Param("codigo")))
		if err == sql.ErrNoRows {
			utils.RespondWithNotFoundError(c, "solicitação")
			return
		}
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if sol.Status != aggregates.StatusSolicitacaoPendente {
			utils.RespondWithConflictError(c, "solicitação já decidida")
			return
		}

		loaded, err := getRepository(c).WithContext(c.Request.Context()).Load(sol.ID, "SolicitacaoAlteracaoNIFAcademia")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		agg := loaded.(*aggregates.SolicitacaoAlteracaoNIFAcademia)

		if aprovar {
			if err := aplicarAlteracaoNIFAprovada(c, sol, adminID.String()); err != nil {
				return
			}
			err = agg.Aprovar(adminID.String())
		} else {
			var req struct {
				MotivoReprovacao string `json:"motivo_reprovacao"`
			}
			_ = c.ShouldBindJSON(&req)
			err = agg.Reprovar(adminID.String(), req.MotivoReprovacao)
		}
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}

		audit := db.AuditContext{UserID: adminID.String(), UserType: "admin", IP: c.ClientIP()}
		if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		_ = db.NewUniqueOperationGuard(getDbClient(c)).WithContext(c.Request.Context()).ReleaseKey(
			"solicitacao_alteracao_nif_academia:pendente", db.CanonicalGuardKey(sol.CodigoAcademia))

		acao := "reprovar_solicitacao_alteracao_nif_academia"
		novoStatus := aggregates.StatusSolicitacaoReprovada
		if aprovar {
			acao = "aprovar_solicitacao_alteracao_nif_academia"
			novoStatus = aggregates.StatusSolicitacaoAprovada
		}
		registrarAcaoAdmin(c, adminID, acao, map[string]interface{}{
			"codigo_solicitacao": sol.CodigoSolicitacao,
			"codigo_academia":    sol.CodigoAcademia,
			"nif_atual":          sol.NIFAtual,
			"nif_solicitado":     sol.NIFSolicitado,
		})

		c.JSON(http.StatusOK, gin.H{
			"message":            "solicitação decidida com sucesso",
			"codigo_solicitacao": sol.CodigoSolicitacao,
			"status":             novoStatus,
		})
	}
}

// aplicarAlteracaoNIFAprovada altera o NIF da Academia dona da solicitação.
// Chamado apenas no caminho de aprovação, antes de marcar a solicitação como
// aprovada — se isto falhar, a solicitação permanece pendente e nenhum dado
// muda (o handler retorna sem persistir a decisão).
func aplicarAlteracaoNIFAprovada(c *gin.Context, sol *projections.SolicitacaoAlteracaoNIFAcademiaDTO, decididoPor string) error {
	academia, err := getAcademiaProjection(c).GetByCodigo(sol.CodigoAcademia)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return fmt.Errorf("academia não encontrada")
	}
	loaded, err := getRepository(c).WithContext(c.Request.Context()).Load(academia.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return err
	}
	agg := loaded.(*aggregates.Academia)
	if err := agg.AlterarNIFPorSolicitacao(sol.NIFSolicitado, sol.CodigoSolicitacao, decididoPor); err != nil {
		utils.RespondWithValidationError(c, err)
		return err
	}
	audit := db.AuditContext{UserID: decididoPor, UserType: "admin", IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(agg, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return err
	}
	return nil
}

func generateUniqueCodigoSolicitacaoAlteracaoNIF(client *db.Client) (string, error) {
	for i := 0; i < 20; i++ {
		code, err := generateUniqueCodigoSolicitacao(client)
		if err != nil {
			return "", err
		}
		var exists bool
		if err := client.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM projection_solicitacoes_alteracao_nif_academia WHERE codigo_solicitacao=$1)`, code).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", fmt.Errorf("não foi possível gerar codigo_solicitacao único")
}
