package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/services"
	"spuri/internal/utils"
)

type ziettSMSTestRequest struct {
	RemitterID string `json:"remitter_id"`
	TargetE164 string `json:"target_e164"`
	Content    string `json:"content"`
}

func EnviarMensagemTesteZiettSMS(c *gin.Context) {
	var req ziettSMSTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "JSON inválido ou payload malformado.", err)
		return
	}

	if strings.TrimSpace(req.RemitterID) == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "remitter_id é obrigatório e deve ser um UUID válido.", errors.New("remitter_id obrigatório"))
		return
	}
	if _, err := uuid.Parse(strings.TrimSpace(req.RemitterID)); err != nil {
		utils.RespondWithError(c, http.StatusBadRequest, "remitter_id deve ser um UUID válido.", err)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		utils.RespondWithError(c, http.StatusBadRequest, "content é obrigatório e não pode estar vazio.", errors.New("content obrigatório"))
		return
	}
	if len([]rune(req.Content)) > 1600 {
		utils.RespondWithError(c, http.StatusBadRequest, "content deve ter no máximo 1600 caracteres.", errors.New("content acima do limite"))
		return
	}

	client, err := services.NewZiettClientFromEnv()
	if err != nil {
		utils.RespondWithError(c, http.StatusServiceUnavailable, "ZIETT_API_KEY não configurada. Configure a variável de ambiente para testar o envio via Ziett.", err)
		return
	}

	result, err := client.SendSMS(c.Request.Context(), services.ZiettSendMessageRequest{RemitterID: strings.TrimSpace(req.RemitterID), TargetE164: req.TargetE164, Content: req.Content})
	if err != nil {
		var apiErr *services.ZiettAPIError
		if errors.As(err, &apiErr) {
			status := apiErr.Status
			if status < 400 || status > 599 {
				status = http.StatusBadGateway
			}
			utils.RespondWithErrorData(c, status, "A Ziett rejeitou o envio da mensagem de teste.", err, gin.H{"ziett_code": apiErr.Code, "ziett_trace_id": apiErr.TraceID, "ziett_status": apiErr.Status, "ziett_message": apiErr.Message, "ziett_service": apiErr.Service, "ziett_fields": apiErr.Fields})
			return
		}
		var netErr *services.ZiettNetworkError
		if errors.As(err, &netErr) {
			utils.RespondWithError(c, http.StatusInternalServerError, "Falha ao contactar a Ziett. Tente novamente mais tarde.", err)
			return
		}
		utils.RespondWithError(c, http.StatusBadRequest, err.Error(), err)
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "mensagem de teste enviada à Ziett com sucesso", "message_id": result.MessageID, "target_e164": result.TargetE164, "channel_type": services.ZiettSMSChannelType})
}
