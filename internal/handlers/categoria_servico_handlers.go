package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/utils"
)

type categoriaServicoPayload struct {
	Nome string `json:"nome"`
}

func bindCategoriaServicoPayload(c *gin.Context, r *categoriaServicoPayload) error {
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(r); err != nil {
		return fmt.Errorf("dados invalidos")
	}
	return nil
}
func categoriaServicoToJSON(cat *aggregates.CategoriaServico) gin.H {
	return gin.H{"id": cat.GetID(), "codigo_academia": cat.CodigoAcademia, "nome": cat.Nome, "ativo": cat.Ativo, "created_at": cat.CreatedAt, "updated_at": cat.UpdatedAt}
}
func validarCategoriaServico(c *gin.Context, codigoAcademia string, categoriaID *uuid.UUID) error {
	if categoriaID == nil {
		return nil
	}
	cat, err := getCategoriasServicoProjection(c).GetByID(*categoriaID)
	if err != nil {
		return fmt.Errorf("erro ao verificar categoria: %v", err)
	}
	if cat == nil {
		return fmt.Errorf("categoria de serviço não encontrada")
	}
	if cat.CodigoAcademia != codigoAcademia {
		return fmt.Errorf("categoria de serviço não pertence a esta academia")
	}
	if !cat.Ativo {
		return fmt.Errorf("categoria de serviço está inativa")
	}
	return nil
}

// validarNomeCategoriaServicoDisponivel verifica, ANTES de gerar o evento
// (CategoriaServicoCriada/Renomeada/Reativada), se já existe outra categoria
// ATIVA com o mesmo nome (ignorando maiúsculas/minúsculas) nesta academia.
//
// Esta pré-checagem existe porque a unicidade de nome só é garantida por um
// índice único parcial na PROJEÇÃO (ux_categorias_servico_nome_ativo em
// projection_categorias_servico), não no event store. SaveWithAudit só grava
// o evento no ledger — a projeção é atualizada depois, de forma assíncrona,
// pelo Projection Manager. Sem esta pré-checagem, dois eventos com nomes
// colidentes são aceitos normalmente pelo ledger (o handler HTTP responde
// sucesso para ambos), mas quando o Manager tenta aplicar o SEGUNDO evento na
// projeção, o INSERT/UPDATE viola o índice único. Esse erro não é
// transitório, então processEventWithRetry (internal/projections/manager.go)
// esgota as 3 tentativas e o checkpoint da projeção "categorias_servico" para
// de avançar PERMANENTEMENTE nesse evento — bloqueando toda criação, edição,
// ativação e desativação de categoria de serviço, em QUALQUER academia, até
// intervenção manual. Um rebuild da projeção não resolve sozinho, pois
// replaya os mesmos eventos na mesma ordem e falha exatamente no mesmo ponto.
func validarNomeCategoriaServicoDisponivel(c *gin.Context, codigoAcademia, nome string, excluirID *uuid.UUID) error {
	nome = strings.TrimSpace(nome)
	cats, err := getCategoriasServicoProjection(c).GetByAcademia(codigoAcademia, true)
	if err != nil {
		return fmt.Errorf("erro ao verificar nome de categoria: %v", err)
	}
	for _, existente := range cats {
		if excluirID != nil && existente.ID == *excluirID {
			continue
		}
		if strings.EqualFold(existente.Nome, nome) {
			return fmt.Errorf("já existe uma categoria de serviço ativa com este nome nesta academia")
		}
	}
	return nil
}

func CriarCategoriaServico(c *gin.Context) {
	var r categoriaServicoPayload
	if err := bindCategoriaServicoPayload(c, &r); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	codigo, id, ok := academy(c)
	if !ok {
		return
	}
	if err := validarNomeCategoriaServicoDisponivel(c, codigo, r.Nome, nil); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	cat := aggregates.NewCategoriaServico()
	if err := cat.Criar(codigo, r.Nome, id); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).SaveWithAudit(cat, db.AuditContext{UserID: id.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "categoria de serviço criada com sucesso", "data": categoriaServicoToJSON(cat)})
}
func loadCategoriaServico(c *gin.Context) (*aggregates.CategoriaServico, uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("ID de categoria de serviço inválido"))
		return nil, uuid.Nil, false
	}
	codigo, user, ok := academy(c)
	if !ok {
		return nil, user, false
	}
	x, err := getRepository(c).Load(id, "CategoriaServico")
	if err != nil {
		utils.RespondWithNotFoundError(c, "categoria de serviço")
		return nil, user, false
	}
	cat, ok := x.(*aggregates.CategoriaServico)
	if !ok || cat.CodigoAcademia != codigo {
		utils.RespondWithForbiddenError(c, "categoria de serviço não pertence a esta academia")
		return nil, user, false
	}
	return cat, user, true
}
func AtualizarCategoriaServico(c *gin.Context) {
	var r categoriaServicoPayload
	if err := bindCategoriaServicoPayload(c, &r); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	cat, id, ok := loadCategoriaServico(c)
	if !ok {
		return
	}
	catID := cat.GetID()
	if err := validarNomeCategoriaServicoDisponivel(c, cat.CodigoAcademia, r.Nome, &catID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := cat.Renomear(r.Nome, id); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := getRepository(c).SaveWithAudit(cat, db.AuditContext{UserID: id.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "categoria de serviço atualizada com sucesso", "data": categoriaServicoToJSON(cat)})
}
func toggleCategoriaServico(c *gin.Context, ativar bool) {
	cat, id, ok := loadCategoriaServico(c)
	if !ok {
		return
	}
	var err error
	if ativar {
		catID := cat.GetID()
		if err = validarNomeCategoriaServicoDisponivel(c, cat.CodigoAcademia, cat.Nome, &catID); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		err = cat.Reativar(id)
	} else {
		err = cat.Desativar(id)
	}
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err = getRepository(c).SaveWithAudit(cat, db.AuditContext{UserID: id.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": categoriaServicoToJSON(cat)})
}
func DesativarCategoriaServico(c *gin.Context) { toggleCategoriaServico(c, false) }
func ReativarCategoriaServico(c *gin.Context)  { toggleCategoriaServico(c, true) }
func ListarCategoriasServico(c *gin.Context) {
	codigo, _, ok := academy(c)
	if !ok {
		return
	}
	ativosOnly := strings.EqualFold(c.Query("ativos"), "true")
	cats, err := getCategoriasServicoProjection(c).GetByAcademia(codigo, ativosOnly)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"categorias_servico": cats, "total": len(cats)})
}
