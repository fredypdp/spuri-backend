package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type estudanteBusinessContext struct {
	AcademiaID     uuid.UUID
	CodigoAcademia string
	Estudante      *aggregates.Estudante
}

func carregarEstudanteDaAcademia(c *gin.Context) (*estudanteBusinessContext, bool) {
	codigoEstudante := c.Param("codigo")
	academiaID, _ := middleware.GetUserID(c)
	academia, err := getAcademiaProjection(c).GetByID(academiaID)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return nil, false
	}
	estudanteDTO, err := getEstudanteProjection(c).GetByCodigo(codigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return nil, false
	}
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academia.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return nil, false
	}
	estudanteAgg, err := getRepository(c).Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return nil, false
	}
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("tipo de aggregate inesperado"))
		return nil, false
	}
	return &estudanteBusinessContext{academiaID, academia.CodigoAcademia, estudante}, true
}
func salvarEventoEstudante(c *gin.Context, ctx *estudanteBusinessContext) bool {
	audit := db.AuditContext{UserID: ctx.AcademiaID.String(), UserType: "academia", IP: c.ClientIP()}
	if err := getRepository(c).SaveWithAudit(ctx.Estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return false
	}
	return true
}
func parseCursoID(c *gin.Context, raw string, tipo string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		utils.RespondWithValidationError(c, fmt.Errorf("curso_id inválido"))
		return uuid.Nil, false
	}
	curso, _ := getCursosProjection(c).GetByID(id)
	if curso == nil || curso.Type != tipo || curso.Status != "ativo" {
		utils.RespondWithValidationError(c, fmt.Errorf("curso_id deve existir, estar ativo e ser do tipo '%s'", tipo))
		return uuid.Nil, false
	}
	return id, true
}

type solicitacaoStatusReq struct {
	Motivo          string `json:"motivo" binding:"required"`
	TipoEnsino      string `json:"tipo_ensino"`
	CursoMedioID    string `json:"curso_medio_id"`
	CursoSuperiorID string `json:"curso_superior_id"`
}
type decisaoReq struct {
	SolicitacaoID    string `json:"solicitacao_id" binding:"required"`
	Observacao       string `json:"observacao_academia"`
	MotivoReprovacao string `json:"motivo_reprovacao"`
}

func CriarSolicitacaoStatusAcademicoHandler(tipo string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req solicitacaoStatusReq
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Motivo) == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("motivo é obrigatório"))
			return
		}
		userID, _ := middleware.GetUserID(c)
		estudanteDTO, err := getEstudanteProjection(c).GetByID(userID)
		if err != nil || estudanteDTO == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		academiaCodigo := c.Param("codigo_academia")
		if academiaCodigo == "" && estudanteDTO.CodigoAcademia != nil {
			academiaCodigo = *estudanteDTO.CodigoAcademia
		}
		if academiaCodigo == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia é obrigatório"))
			return
		}
		client := getDbClient(c)
		if client == nil {
			return
		}
		guardKey := db.CanonicalGuardKey(estudanteDTO.CodigoEstudante, academiaCodigo, tipo)
		guard, err := db.NewUniqueOperationGuard(client).WithContext(c.Request.Context()).Reserve(
			"solicitacao_status_academico:pendente",
			guardKey,
			db.UniqueGuardOptions{UserID: userID.String(), UserType: "estudante", Metadata: map[string]interface{}{"tipo": tipo}},
		)
		if errors.Is(err, db.ErrUniqueOperationInProgress) {
			utils.RespondWithConflictError(c, "já existe solicitação pendente ou em criação para este estudante nesta academia")
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

		var exists bool
		if err := client.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM projection_solicitacoes_status_academico WHERE codigo_estudante=$1 AND codigo_academia=$2 AND tipo=$3 AND status='pendente')`, estudanteDTO.CodigoEstudante, academiaCodigo, tipo).Scan(&exists); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if exists {
			utils.RespondWithConflictError(c, "já existe solicitação pendente para este estudante nesta academia")
			return
		}
		id := "SSA" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
		_, err = client.DB().Exec(`INSERT INTO projection_solicitacoes_status_academico (id,codigo_solicitacao,codigo_estudante,codigo_academia,tipo,status,motivo,tipo_ensino,curso_medio_id,curso_superior_id,solicitada_por,created_at,updated_at) VALUES ($1,$2,$3,$4,$5,'pendente',$6,$7,$8,$9,$10,$11,$11)`, uuid.New(), id, estudanteDTO.CodigoEstudante, academiaCodigo, tipo, strings.TrimSpace(req.Motivo), req.TipoEnsino, nullableUUID(req.CursoMedioID), nullableUUID(req.CursoSuperiorID), userID, time.Now().UTC())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if err := guard.Consume(uuid.Nil); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		guardConsumed = true
		c.JSON(http.StatusCreated, gin.H{"message": "solicitação criada com sucesso", "codigo_solicitacao": id, "status": "pendente"})
	}
}
func nullableUUID(raw string) interface{} {
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil
	}
	return id
}

func ListarMinhasSolicitacoesStatusAcademicoHandler(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	estudanteDTO, err := getEstudanteProjection(c).GetByID(userID)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	listarSolicitacoesStatusAcademico(c, "codigo_estudante=$1", estudanteDTO.CodigoEstudante)
}

func ListarSolicitacoesStatusAcademicoHandler(c *gin.Context) {
	userType, _ := c.Get("user_type")
	if userType == "admin" {
		codigoAcademia := strings.TrimSpace(c.Query("codigo_academia"))
		if codigoAcademia == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("codigo_academia é obrigatório para administradores"))
			return
		}
		listarSolicitacoesStatusAcademico(c, "codigo_academia=$1", codigoAcademia)
		return
	}

	academiaID, _ := middleware.GetUserID(c)
	academia, _ := getAcademiaProjection(c).GetByID(academiaID)
	if academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	listarSolicitacoesStatusAcademico(c, "codigo_academia=$1", academia.CodigoAcademia)
}

func listarSolicitacoesStatusAcademico(c *gin.Context, where string, arg interface{}) {
	query := fmt.Sprintf(`SELECT codigo_solicitacao,codigo_estudante,codigo_academia,tipo,status,motivo,tipo_ensino,motivo_reprovacao,observacao_academia,created_at,updated_at,decidida_at FROM projection_solicitacoes_status_academico WHERE %s ORDER BY created_at DESC`, where)
	rows, err := getDbClient(c).DB().Query(query, arg)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var cod, est, acad, tipo, status, motivo string
		var te, motivoReprovacao, observacaoAcademia *string
		var cr, up time.Time
		var decididaAt *time.Time
		if err := rows.Scan(&cod, &est, &acad, &tipo, &status, &motivo, &te, &motivoReprovacao, &observacaoAcademia, &cr, &up, &decididaAt); err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		out = append(out, gin.H{"codigo_solicitacao": cod, "codigo_estudante": est, "codigo_academia": acad, "tipo": tipo, "status": status, "motivo": motivo, "tipo_ensino": te, "motivo_reprovacao": motivoReprovacao, "observacao_academia": observacaoAcademia, "created_at": cr, "updated_at": up, "decidida_at": decididaAt})
	}
	if err := rows.Err(); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"solicitacoes": out, "total": len(out)})
}

func AprovarSolicitacaoStatusAcademicoHandler(tipo string) gin.HandlerFunc {
	return func(c *gin.Context) { decidirSolicitacaoStatus(c, tipo, true) }
}
func ReprovarSolicitacaoStatusAcademicoHandler(tipo string) gin.HandlerFunc {
	return func(c *gin.Context) { decidirSolicitacaoStatus(c, tipo, false) }
}
func decidirSolicitacaoStatus(c *gin.Context, tipo string, aprovar bool) {
	var req decisaoReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.SolicitacaoID) == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("solicitacao_id é obrigatório"))
		return
	}
	academiaID, _ := middleware.GetUserID(c)
	academia, _ := getAcademiaProjection(c).GetByID(academiaID)
	if academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}
	var codigoEst, codigoAcademia, status, motivo, tipoEnsino string
	var cursoMedio, cursoSuperior *uuid.UUID
	err := getDbClient(c).DB().QueryRow(`SELECT codigo_estudante,codigo_academia,status,motivo,COALESCE(tipo_ensino,''),curso_medio_id,curso_superior_id FROM projection_solicitacoes_status_academico WHERE codigo_solicitacao=$1 AND tipo=$2`, req.SolicitacaoID, tipo).Scan(&codigoEst, &codigoAcademia, &status, &motivo, &tipoEnsino, &cursoMedio, &cursoSuperior)
	if err != nil {
		utils.RespondWithNotFoundError(c, "solicitação")
		return
	}
	if codigoAcademia != academia.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "solicitação não pertence a esta academia")
		return
	}
	if codigoParam := strings.TrimSpace(c.Param("codigo")); codigoParam != "" && codigoParam != codigoEst {
		utils.RespondWithValidationError(c, fmt.Errorf("solicitacao_id não pertence ao estudante informado na rota"))
		return
	}
	if status != "pendente" {
		utils.RespondWithValidationError(c, fmt.Errorf("solicitação já decidida"))
		return
	}
	if !aprovar {
		if strings.TrimSpace(req.MotivoReprovacao) == "" {
			utils.RespondWithValidationError(c, fmt.Errorf("motivo_reprovacao é obrigatório"))
			return
		}
		_, err = getDbClient(c).DB().Exec(`UPDATE projection_solicitacoes_status_academico SET status='reprovada',motivo_reprovacao=$1,decidida_por=$2,decidida_at=$3,updated_at=$3 WHERE codigo_solicitacao=$4`, req.MotivoReprovacao, academiaID, time.Now().UTC(), req.SolicitacaoID)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		_ = db.NewUniqueOperationGuard(getDbClient(c)).WithContext(c.Request.Context()).ReleaseKey("solicitacao_status_academico:pendente", db.CanonicalGuardKey(codigoEst, codigoAcademia, tipo))
		c.JSON(http.StatusOK, gin.H{"message": "solicitação reprovada"})
		return
	}
	c.Params = append(c.Params, gin.Param{Key: "codigo", Value: codigoEst})
	ctx, ok := carregarEstudanteDaAcademia(c)
	if !ok {
		return
	}
	var e error
	switch tipo {
	case "interrupcao":
		e = ctx.Estudante.InterromperPercursoAcademico(motivo, academiaID)
	case "desvinculacao":
		e = ctx.Estudante.DesvincularDaAcademia(codigoAcademia, motivo, academiaID)
	case "revinculacao":
		e = ctx.Estudante.Reintegrar(codigoAcademia, tipoEnsino, nil, nil, cursoMedio, cursoSuperior, academiaID)
	}
	if e != nil {
		utils.RespondWithValidationError(c, e)
		return
	}
	anotarSolicitacaoNoUltimoEvento(ctx.Estudante, req.SolicitacaoID)
	if !salvarEventoEstudante(c, ctx) {
		return
	}
	_, err = getDbClient(c).DB().Exec(`UPDATE projection_solicitacoes_status_academico SET status='aprovada',observacao_academia=$1,decidida_por=$2,decidida_at=$3,updated_at=$3 WHERE codigo_solicitacao=$4`, req.Observacao, academiaID, time.Now().UTC(), req.SolicitacaoID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	_ = db.NewUniqueOperationGuard(getDbClient(c)).WithContext(c.Request.Context()).ReleaseKey("solicitacao_status_academico:pendente", db.CanonicalGuardKey(codigoEst, codigoAcademia, tipo))
	c.JSON(http.StatusOK, gin.H{"message": "solicitação aprovada", "codigo_solicitacao": req.SolicitacaoID})
}

func anotarSolicitacaoNoUltimoEvento(estudante *aggregates.Estudante, solicitacaoID string) {
	if len(estudante.UncommittedEvents) == 0 {
		return
	}
	switch ev := estudante.UncommittedEvents[len(estudante.UncommittedEvents)-1].(type) {
	case *aggregates.FundamentalInterrompidoEvent:
		ev.SolicitacaoID = solicitacaoID
	case *aggregates.MedioInterrompidoEvent:
		ev.SolicitacaoID = solicitacaoID
	case *aggregates.SuperiorInterrompidoEvent:
		ev.SolicitacaoID = solicitacaoID
	case *aggregates.EstudanteDesvinculadoDaAcademiaEvent:
		ev.SolicitacaoID = solicitacaoID
	case *aggregates.EstudanteReintegradoEvent:
		ev.SolicitacaoID = solicitacaoID
	}
}
