package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/utils"
)

// deletionEventTypes — os 3 tipos de evento que representam uma deleção
// lógica no sistema (Tarefa 73). Mantido em um único lugar para evitar
// divergência entre este handler e internal/db/safe_queries.go.
var deletionEventTypes = map[string]string{
	"academia":      "AcademiaDeletada",
	"administrador": "AdminDeletado",
	"estudante":     "EstudanteDeletado",
}

// deletionPayload — campos comuns aos 3 eventos de deleção (ver
// AcademiaDeletadaEvent/AdminDeletadoEvent/EstudanteDeletadoEvent). Academia
// também grava CodigoAcademia no payload; os outros dois não guardam o
// identificador legível no payload — por isso o enriquecimento abaixo
// consulta a projeção correspondente (que sobrevive à deleção lógica).
type deletionPayload struct {
	CodigoAcademia string    `json:"CodigoAcademia"`
	Motivo         string    `json:"Motivo"`
	DeletadoPor    uuid.UUID `json:"DeletadoPor"`
	DeletedAt      time.Time `json:"DeletedAt"`
}

// ListarAuditoriaDelecoes — Tarefa 73/2. Endpoint dedicado de auditoria: lista
// as deleções lógicas (Academia/Administrador/Estudante) diretamente do
// spuri_ledger, mais recentes primeiro. Este é o único lugar pensado para
// consultar entidades deletadas — as listagens gerais (ListarTodasAcademias,
// ListarEstudantes, ListarTodosAdmins) agora excluem status='deletado'.
//
// GET /dominis/auditoria/delecoes?tipo=academia|administrador|estudante&limit=&offset=
// tipo é opcional; sem ele, traz os 3 tipos juntos, ordenados por data.
func ListarAuditoriaDelecoes(c *gin.Context) {
	limit, offset := getPaginationParams(c)

	var eventTypes []string
	if tipo := c.Query("tipo"); tipo != "" {
		et, ok := deletionEventTypes[tipo]
		if !ok {
			utils.RespondWithValidationError(c, fmt.Errorf("tipo inválido: valores aceitos são academia, administrador ou estudante"))
			return
		}
		eventTypes = []string{et}
	} else {
		eventTypes = []string{
			deletionEventTypes["academia"],
			deletionEventTypes["administrador"],
			deletionEventTypes["estudante"],
		}
	}

	repository := getRepository(c)
	events, err := repository.GetEventsByTypes(eventTypes, limit, offset)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	adminProj := getAdminProjection(c)
	estudanteProj := getEstudanteProjection(c)

	resultado := make([]gin.H, 0, len(events))
	for _, event := range events {
		var payload deletionPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			// Não deixa um evento malformado quebrar a listagem inteira.
			continue
		}

		item := gin.H{
			"event_id":     event.EventID,
			"entidade_id":  event.AggregateID,
			"motivo":       payload.Motivo,
			"deletado_por": payload.DeletadoPor,
			"deletado_em":  payload.DeletedAt,
		}

		switch event.EventType {
		case "AcademiaDeletada":
			item["tipo_entidade"] = "academia"
			item["identificador"] = payload.CodigoAcademia
			if aca, err := academiaProj.GetByID(event.AggregateID); err == nil && aca != nil {
				item["nome"] = aca.Nome
			}
			if executor, err := adminProj.GetByID(payload.DeletadoPor); err == nil && executor != nil {
				item["deletado_por_nome"] = executor.Nome
				item["deletado_por_email"] = executor.Email
			}

		case "AdminDeletado":
			item["tipo_entidade"] = "administrador"
			if adm, err := adminProj.GetByID(event.AggregateID); err == nil && adm != nil {
				item["identificador"] = adm.Email
				item["nome"] = adm.Nome
				item["role"] = adm.Role
			}
			if executor, err := adminProj.GetByID(payload.DeletadoPor); err == nil && executor != nil {
				item["deletado_por_nome"] = executor.Nome
				item["deletado_por_email"] = executor.Email
			}

		case "EstudanteDeletado":
			item["tipo_entidade"] = "estudante"
			if est, err := estudanteProj.GetByID(event.AggregateID); err == nil && est != nil {
				item["identificador"] = est.CodigoEstudante
				item["nome"] = est.Nome
			}
			// Estudante é sempre autodeleção — deletado_por == entidade_id,
			// não há um "executor" terceiro a resolver aqui.
		}

		resultado = append(resultado, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"delecoes": resultado,
		"total":    len(resultado),
		"limit":    limit,
		"offset":   offset,
	})
}
