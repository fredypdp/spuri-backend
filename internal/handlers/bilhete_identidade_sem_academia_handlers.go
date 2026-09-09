package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

type bilheteIdentidadeSemAcademiaRequest struct {
	BilheteIdentidade string `json:"bilhete_identidade"`
}

// AtualizarBilheteIdentidadeSemAcademia é a via de autoatualização do BI do
// próprio estudante — só disponível quando ele NÃO está vinculado a nenhuma
// academia no momento (Status != "ativo" && Status != "pendente_documentos";
// NUNCA CodigoAcademia == nil, ver comentário em
// aggregates.Estudante.AlterarBilheteIdentidadeSemAcademia). Quando há
// academia vinculada, o único caminho é a solicitação com aprovação da
// academia: POST /estudante/solicitacoes-edicao/bilhete-identidade (ver
// CriarSolicitacaoEdicaoDadoEstudanteHandler, que exige Status IN ('ativo',
// 'pendente_documentos')). As duas rotas juntas cobrem os dois casos: com e
// sem academia vinculada no momento.
func AtualizarBilheteIdentidadeSemAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	est, err := getEstudanteProjection(c).GetByID(userID)
	if err != nil || est == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	// "Vinculado a uma academia" é Status IN ('ativo', 'pendente_documentos')
	// — NUNCA est.CodigoAcademia == nil: codigo_academia permanece preenchido
	// para sempre em cada estudante mesmo depois de desvinculado (ver
	// comentário em aggregates.Estudante.Deletar e
	// EstudanteProjection.CountVinculadosAtivos). Um estudante desvinculado
	// (Status = 'inativo') tem CodigoAcademia preenchido com a academia
	// ANTERIOR, mas não está mais vinculado a ela — é exatamente esse
	// estudante que esta rota atende.
	if est.Status == "ativo" || est.Status == "pendente_documentos" {
		utils.RespondWithValidationError(c, fmt.Errorf("você está vinculado a uma academia; use POST /estudante/solicitacoes-edicao/bilhete-identidade para solicitar a alteração com aprovação da academia"))
		return
	}

	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decErr := decoder.Decode(&raw); decErr != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("body inválido: envie um JSON com o campo bilhete_identidade"))
		return
	}
	if len(raw) != 1 {
		utils.RespondWithValidationError(c, fmt.Errorf("forneça somente o campo bilhete_identidade"))
		return
	}
	rawValor, ok := raw["bilhete_identidade"]
	if !ok {
		utils.RespondWithValidationError(c, fmt.Errorf("campo bilhete_identidade é obrigatório"))
		return
	}
	var req bilheteIdentidadeSemAcademiaRequest
	if jsonErr := json.Unmarshal(rawValor, &req.BilheteIdentidade); jsonErr != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("bilhete_identidade deve ser uma string"))
		return
	}

	// validarValorSolicitadoEdicao já cobre: obrigatoriedade, formato
	// (utils.ValidateBilhete), unicidade (BilheteIdentidadeExists) e "deve
	// ser diferente do valor atual" — mesma validação usada no fluxo de
	// solicitação, reaproveitada aqui para não divergir de regra.
	_, novoValidado, verr := validarValorSolicitadoEdicao(c, est, aggregates.CampoEdicaoBI, strings.TrimSpace(req.BilheteIdentidade))
	if verr != nil {
		return // validarValorSolicitadoEdicao já escreveu a resposta de erro
	}

	repository := getRepository(c)
	agg, loadErr := repository.Load(est.ID, "Estudante")
	if loadErr != nil {
		utils.RespondWithInternalError(c, loadErr)
		return
	}
	estudanteAgg := agg.(*aggregates.Estudante)
	if applyErr := estudanteAgg.AlterarBilheteIdentidadeSemAcademia(novoValidado); applyErr != nil {
		utils.RespondWithValidationError(c, applyErr)
		return
	}
	if saveErr := repository.SaveWithAudit(estudanteAgg, db.AuditContext{UserID: userID.String(), UserType: "estudante", IP: c.ClientIP()}); saveErr != nil {
		utils.RespondWithInternalError(c, saveErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            "bilhete de identidade atualizado com sucesso",
		"bilhete_identidade": novoValidado,
	})
}
