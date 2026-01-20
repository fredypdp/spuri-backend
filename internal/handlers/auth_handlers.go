package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/services"
	"spuri/internal/utils"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Usuario string `json:"usuario" binding:"required"`
	Senha   string `json:"senha" binding:"required"`
	Type    string `json:"type" binding:"required"`
}

type LoginResponse struct {
	Token  string    `json:"token"`
	ID     uuid.UUID `json:"id"`
	Nome   string    `json:"nome"`
	Type   string    `json:"type"`
}

func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Type != "estudante" && req.Type != "academia" {
		utils.RespondWithValidationError(c, fmt.Errorf("tipo deve ser 'estudante' ou 'academia'"))
		return
	}

	estudanteProj := getEstudanteProjection(c)
	academiaProj := getAcademiaProjection(c)

	var userID uuid.UUID
	var userName string
	var senhaHash string

	if req.Type == "academia" {
		academia, err := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if err != nil {
			log.Printf("❌ Erro ao buscar academia: %v", err)
			utils.RespondWithInternalError(c, err)
			return
		}
		if academia == nil {
			utils.RespondWithUnauthorizedError(c)
			return
		}

		userID = academia.ID
		userName = academia.Nome
		senhaHash = academia.SenhaHash

	} else {
		estudante, err := estudanteProj.GetByCodigo(req.Usuario)
		if err != nil {
			log.Printf("❌ Erro ao buscar estudante: %v", err)
			utils.RespondWithInternalError(c, err)
			return
		}
		if estudante == nil {
			utils.RespondWithUnauthorizedError(c)
			return
		}

		userID = estudante.ID
		userName = estudante.Nome
		senhaHash = estudante.SenhaHash
	}

	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.Senha)); err != nil {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	token, err := middleware.GenerateToken(userID, req.Type)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	var codigo string
	if req.Type == "academia" {
		academiaDTO, _ := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if academiaDTO != nil {
			codigo = academiaDTO.CodigoAcademia
		}
	} else {
		estudanteDTO, _ := estudanteProj.GetByCodigo(req.Usuario)
		if estudanteDTO != nil {
			codigo = estudanteDTO.CodigoEstudante
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"token":  token,
		"codigo": codigo,
		"nome":   userName,
		"type":   req.Type,
	})
}

type RegisterAcademiaRequest struct {
	Type           string   `json:"type" binding:"required"`
	Senha          string   `json:"senha" binding:"required"`
	Nome           string   `json:"nome" binding:"required"`
	Provincia      string   `json:"provincia" binding:"required"`
	Endereco       string   `json:"endereco" binding:"required"`
	NumeroTelefone *string  `json:"numero_telefone"`
	Email          *string  `json:"email"`
	Website        *string  `json:"website"`
	NivelEscolar   *string  `json:"nivel_escolar"`
	Cursos         []string `json:"cursos"`
}

func RegisterAcademia(c *gin.Context) {
	var req RegisterAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ✅ VALIDAÇÕES
	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateSenha(req.Senha); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateEndereco(req.Endereco); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Website != nil {
		if err := utils.ValidateURL(*req.Website); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Type != "escola" && req.Type != "superior" {
		utils.RespondWithValidationError(c, fmt.Errorf("type deve ser 'escola' ou 'superior'"))
		return
	}

	if req.Type == "escola" {
		if req.NivelEscolar == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_escolar é obrigatório para escolas"))
			return
		}
		
		validNiveis := map[string]bool{
			"fundamental": true,
			"medio":       true,
			"misto":       true,
		}
		
		if !validNiveis[*req.NivelEscolar] {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_escolar deve ser 'fundamental', 'medio' ou 'misto'"))
			return
		}
	}

	codigoProvincia, err := validarProvincia(req.Provincia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	codigo := generateCodigoAcademia(codigoProvincia)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	repository := getRepository(c)
	academia := aggregates.NewAcademia()

	if err := academia.Criar(
		req.Type,
		req.Nome,
		codigo,
		string(hashedPassword),
		codigoProvincia,
		req.Endereco,
		req.NumeroTelefone,
		req.Email,
		req.Website,
		req.NivelEscolar,
		req.Cursos,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(academia); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "academia criada com sucesso",
		"data": gin.H{
			"id":              academia.ID,
			"codigo_academia": academia.CodigoAcademia,
		},
	})
}

type RegisterEstudanteRequest struct {
	Senha                 string  `json:"senha" binding:"required"`
	Nome                  string  `json:"nome" binding:"required"`
	Email                 *string `json:"email"`
	Telefone              *string `json:"telefone"`
	BilheteIdentidade     *string `json:"bilhete_identidade"`
	BilheteIdentidadeResp *string `json:"bilhete_identidade_responsavel"`
	AnoEscolar            *string `json:"ano_escolar"`
	AnoSuperior           *string `json:"ano_superior"`
	CursoMedio            *string `json:"curso_medio"`
	CursoSuperior         *string `json:"curso_superior"`
	StatusEscolar         *string `json:"status_escolar"`
	StatusSuperior        *string `json:"status_superior"`
}

func RegisterEstudante(c *gin.Context) {
	var req RegisterEstudanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ✅ VALIDAÇÕES
	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateSenha(req.Senha); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Telefone != nil {
		if err := utils.ValidatePhone(*req.Telefone); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if err := utils.ValidateBilhete(utils.SafeDeref(req.BilheteIdentidade)); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateBilhete(utils.SafeDeref(req.BilheteIdentidadeResp)); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.BilheteIdentidade == nil && req.BilheteIdentidadeResp == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("pelo menos um bilhete de identidade é obrigatório"))
		return
	}

	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	if err := estudante.Criar(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		req.Email,
		req.Telefone,
		req.BilheteIdentidade,
		req.BilheteIdentidadeResp,
		req.AnoEscolar,
		req.AnoSuperior,
		req.CursoMedio,
		req.CursoSuperior,
		req.StatusEscolar,
		req.StatusSuperior,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	if req.Email != nil && *req.Email != "" {
		emailSvc := services.NewEmailService(client.DB())
		if err := emailSvc.SendVerificationEmail(estudante.ID, "estudante", *req.Email, req.Nome); err != nil {
			log.Printf("⚠️ Erro ao enviar email: %v", err)
		}
	}

	response := gin.H{
		"message": "estudante criado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
		},
	}

	if req.Email != nil && *req.Email != "" {
		response["email_verificacao"] = "Email de verificação enviado"
	}

	c.JSON(http.StatusCreated, response)
}

func validarProvincia(provincia string) (string, error) {
	provinciaInput := strings.ToUpper(strings.TrimSpace(provincia))

	provinciaMap := map[string]string{
		"BENGO": "BGO", "BGO": "BGO",
		"BENGUELA": "BGU", "BGU": "BGU",
		"BIE": "BIE", "BIÉ": "BIE",
		"CABINDA": "CAB", "CAB": "CAB",
		"CUANDO": "CND", "CND": "CND", "CUANDO CUBANGO": "CND",
		"CUANZA NORTE": "CNO", "CNO": "CNO", "KWANZA NORTE": "CNO",
		"CUANZA SUL": "CUS", "CUS": "CUS", "KWANZA SUL": "CUS",
		"CUBANGO": "CBG", "CBG": "CBG",
		"CUNENE": "CNN", "CNN": "CNN",
		"HUAMBO": "HUA", "HUA": "HUA",
		"HUILA": "HUI", "HUÍLA": "HUI", "HUI": "HUI",
		"ICOLO E BENGO": "IBG", "IBG": "IBG",
		"LUANDA": "LUA", "LUA": "LUA",
		"LUNDA NORTE": "LNO", "LNO": "LNO",
		"LUNDA SUL": "LSU", "LSU": "LSU",
		"MALANJE": "MAL", "MAL": "MAL",
		"MOXICO": "MOX", "MOX": "MOX",
		"MOXICO LESTE": "MXL", "MXL": "MXL",
		"NAMIBE": "NAM", "NAM": "NAM",
		"UIGE": "UIG", "UÍGE": "UIG", "UIG": "UIG",
		"ZAIRE": "ZAI", "ZAI": "ZAI",
	}

	if code, ok := provinciaMap[provinciaInput]; ok {
		return code, nil
	}

	return "", fmt.Errorf("província inválida: '%s'", provincia)
}

func generateCodigoAcademia(codigoProvincia string) string {
	ano := time.Now().Year()
	numero := time.Now().UnixNano() % 10000
	return fmt.Sprintf("%s%d%d", codigoProvincia, ano, numero)
}