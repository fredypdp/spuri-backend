package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

func rejectDedicatedContactFields(c *gin.Context) bool {
	var raw map[string]json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("body inválido"))
		return true
	}
	for _, field := range []string{"email", "telefone"} {
		if _, ok := raw[field]; ok {
			rota := "/me/email"
			if field == "telefone" {
				rota = "/me/telefone"
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "VALIDATION_ERROR", "message": fmt.Sprintf("O campo '%s' não é aceito nesta rota. Use PUT %s para alterar o %s do usuário autenticado.", field, rota, field), "details": []gin.H{{"field": field, "code": "campo_nao_permitido", "message": fmt.Sprintf("Use PUT %s para alterar o %s.", rota, field)}}})
			return true
		}
	}
	b, _ := json.Marshal(raw)
	c.Request.Body = io.NopCloser(strings.NewReader(string(b)))
	return false
}

func rejectAcademiaDadosRestrictedFields(c *gin.Context) bool {
	var raw map[string]json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("body inválido"))
		return true
	}
	messages := map[string]string{
		"telefone":        "O campo 'telefone' não é aceito em PUT /academia/dados. Use PUT /me/telefone para alterar o telefone da academia pelo fluxo dedicado.",
		"email":           "O campo 'email' não é aceito em PUT /academia/dados. Use PUT /me/email para alterar o e-mail da academia pelo fluxo dedicado.",
		"anos_academicos": "O campo 'anos_academicos' não é aceito em PUT /academia/dados. Use POST/DELETE /academia/anos-academicos para adicionar ou remover anos acadêmicos.",
		"cursos":          "O campo 'cursos' não é aceito em PUT /academia/dados. Use as rotas /academia/curso para gerir cursos.",
		"type":            "O campo 'type' não é aceito em PUT /academia/dados. A alteração exige documento comprobativo pelo fluxo dedicado da tarefa 07 e está temporariamente indisponível por este caminho.",
		"nivel_escolar":   "O campo 'nivel_escolar' não é aceito em PUT /academia/dados. A alteração exige documento comprobativo pelo fluxo dedicado da tarefa 07 e está temporariamente indisponível por este caminho.",
		"nif":             "O campo 'nif' não é aceito em PUT /academia/dados. A alteração de NIF exige fluxo dedicado com validações próprias.",
	}
	for _, field := range []string{"telefone", "email", "anos_academicos", "cursos", "type", "nivel_escolar", "nif"} {
		if _, ok := raw[field]; ok {
			msg := messages[field]
			c.JSON(http.StatusBadRequest, gin.H{"error": "VALIDATION_ERROR", "message": msg, "details": []gin.H{{"field": field, "code": "campo_nao_permitido", "message": msg}}})
			return true
		}
	}
	b, _ := json.Marshal(raw)
	c.Request.Body = io.NopCloser(strings.NewReader(string(b)))
	return false
}

func AtualizarMeuEmail(c *gin.Context)    { atualizarMeuContato(c, "email") }
func AtualizarMeuTelefone(c *gin.Context) { atualizarMeuContato(c, "telefone") }

func atualizarMeuContato(c *gin.Context, campo string) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	var raw map[string]json.RawMessage
	if err := c.ShouldBindJSON(&raw); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("body inválido"))
		return
	}
	v, ok := raw[campo]
	if !ok || len(raw) != 1 {
		utils.RespondWithValidationError(c, fmt.Errorf("forneça somente o campo %s", campo))
		return
	}
	var valor string
	if err := json.Unmarshal(v, &valor); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("%s deve ser uma string", campo))
		return
	}
	valor = strings.TrimSpace(valor)
	if campo == "email" {
		if err := utils.ValidateEmail(valor); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	} else {
		if err := utils.ValidatePhoneStrictNational(valor); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}
	if campo == "email" && emailEmUsoPorOutro(c, valor, userID.String()) {
		utils.RespondWithConflictError(c, "email já cadastrado no sistema")
		return
	}
	repo := getRepository(c)
	aggType := map[string]string{"estudante": "Estudante", "academia": "Academia", "admin": "Admin"}[userType]
	if aggType == "" {
		utils.RespondWithForbiddenError(c, "tipo de usuário não suportado")
		return
	}
	agg, err := repo.Load(userID, aggType)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	p := &valor
	switch a := agg.(type) {
	case *aggregates.Estudante:
		if campo == "email" {
			err = a.AtualizarDadosPessoais(nil, p, nil, nil, nil, nil, nil)
		} else {
			err = a.AtualizarDadosPessoais(nil, nil, p, nil, nil, nil, nil)
		}
	case *aggregates.Academia:
		if campo == "email" {
			err = a.AtualizarDados(nil, nil, nil, nil, nil, nil, p, nil, nil, nil, nil)
		} else {
			err = a.AtualizarDados(nil, nil, nil, nil, nil, p, nil, nil, nil, nil, nil)
		}
	case *aggregates.Admin:
		if campo == "email" {
			err = a.AtualizarDados(nil, p, nil, userID)
		} else {
			err = a.AtualizarDados(nil, nil, p, userID)
		}
	}
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	if err := repo.SaveWithAudit(agg, db.AuditContext{UserID: userID.String(), UserType: userType, IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("%s atualizado com sucesso", campo)})
}

func emailEmUsoPorOutro(c *gin.Context, email, self string) bool {
	if e, _ := getEstudanteProjection(c).GetByEmail(email); e != nil && e.ID.String() != self {
		return true
	}
	if a, _ := getAcademiaProjection(c).GetByEmail(email); a != nil && a.ID.String() != self {
		return true
	}
	if a, _ := getAdminProjection(c).GetByEmail(email); a != nil && a.ID.String() != self {
		return true
	}
	return false
}
