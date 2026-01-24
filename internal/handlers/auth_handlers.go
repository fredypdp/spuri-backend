package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
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
	log.Printf("[DEBUG] Login: Início")
	
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] Login: Erro no bind JSON: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	log.Printf("[DEBUG] Login: Usuário: %s, Tipo: %s", req.Usuario, req.Type)

	if req.Type != "estudante" && req.Type != "academia" {
		log.Printf("[ERROR] Login: Tipo inválido: %s", req.Type)
		utils.RespondWithValidationError(c, fmt.Errorf("tipo deve ser 'estudante' ou 'academia'"))
		return
	}

	estudanteProj := getEstudanteProjection(c)
	academiaProj := getAcademiaProjection(c)

	var userID uuid.UUID
	var userName string
	var senhaHash string

	if req.Type == "academia" {
		log.Printf("[DEBUG] Login: Buscando academia")
		
		academia, err := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if err != nil {
			log.Printf("[ERROR] Login: Erro ao buscar academia: %v", err)
			utils.RespondWithInternalError(c, err)
			return
		}
		if academia == nil {
			log.Printf("[ERROR] Login: Academia não encontrada")
			utils.RespondWithUnauthorizedError(c)
			return
		}

		log.Printf("[DEBUG] Login: Academia encontrada - ID: %s, Nome: %s", academia.ID, academia.Nome)

		userID = academia.ID
		userName = academia.Nome
		senhaHash = academia.SenhaHash

	} else {
		log.Printf("[DEBUG] Login: Buscando estudante")
		
		estudante, err := estudanteProj.GetByCodigo(req.Usuario)
		if err != nil {
			log.Printf("[ERROR] Login: Erro ao buscar estudante: %v", err)
			utils.RespondWithInternalError(c, err)
			return
		}
		if estudante == nil {
			log.Printf("[ERROR] Login: Estudante não encontrado")
			utils.RespondWithUnauthorizedError(c)
			return
		}

		log.Printf("[DEBUG] Login: Estudante encontrado - ID: %s, Nome: %s", estudante.ID, estudante.Nome)

		userID = estudante.ID
		userName = estudante.Nome
		senhaHash = estudante.SenhaHash
	}

	log.Printf("[DEBUG] Login: Verificando senha")

	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.Senha)); err != nil {
		log.Printf("[ERROR] Login: Senha incorreta")
		utils.RespondWithUnauthorizedError(c)
		return
	}

	log.Printf("[DEBUG] Login: Gerando token")

	token, err := middleware.GenerateToken(userID, req.Type)
	if err != nil {
		log.Printf("[ERROR] Login: Erro ao gerar token: %v", err)
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

	log.Printf("[DEBUG] Login: Sucesso - UserID: %s, Nome: %s", userID, userName)

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
	log.Printf("[DEBUG] RegisterAcademia: Início")
	
	var req RegisterAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] RegisterAcademia: Erro no bind JSON: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterAcademia: Nome: %s, Tipo: %s, Província: %s", req.Nome, req.Type, req.Provincia)

	if err := utils.ValidateNome(req.Nome); err != nil {
		log.Printf("[ERROR] RegisterAcademia: Nome inválido: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateSenha(req.Senha); err != nil {
		log.Printf("[ERROR] RegisterAcademia: Senha inválida: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateEndereco(req.Endereco); err != nil {
		log.Printf("[ERROR] RegisterAcademia: Endereço inválido: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			log.Printf("[ERROR] RegisterAcademia: Email inválido: %v", err)
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Website != nil {
		if err := utils.ValidateURL(*req.Website); err != nil {
			log.Printf("[ERROR] RegisterAcademia: Website inválido: %v", err)
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Type != "escola" && req.Type != "superior" {
		log.Printf("[ERROR] RegisterAcademia: Tipo inválido: %s", req.Type)
		utils.RespondWithValidationError(c, fmt.Errorf("type deve ser 'escola' ou 'superior'"))
		return
	}

	if req.Type == "escola" {
		if req.NivelEscolar == nil {
			log.Printf("[ERROR] RegisterAcademia: Nível escolar obrigatório para escolas")
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_escolar é obrigatório para escolas"))
			return
		}
		
		validNiveis := map[string]bool{
			"fundamental": true,
			"medio":       true,
			"misto":       true,
		}
		
		if !validNiveis[*req.NivelEscolar] {
			log.Printf("[ERROR] RegisterAcademia: Nível escolar inválido: %s", *req.NivelEscolar)
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_escolar deve ser 'fundamental', 'medio' ou 'misto'"))
			return
		}
	}

	log.Printf("[DEBUG] RegisterAcademia: Validando província")

	codigoProvincia, err := validarProvincia(req.Provincia)
	if err != nil {
		log.Printf("[ERROR] RegisterAcademia: Província inválida: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterAcademia: Código província: %s", codigoProvincia)

	codigo := generateCodigoAcademia(codigoProvincia)
	log.Printf("[DEBUG] RegisterAcademia: Código gerado: %s", codigo)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[ERROR] RegisterAcademia: Erro ao gerar hash: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterAcademia: Criando agregado")

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
		log.Printf("[ERROR] RegisterAcademia: Erro ao criar agregado: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterAcademia: Salvando eventos")

	if err := repository.Save(academia); err != nil {
		log.Printf("[ERROR] RegisterAcademia: Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterAcademia: Sucesso - ID: %s, Código: %s", academia.ID, academia.CodigoAcademia)

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
	log.Printf("[DEBUG] RegisterEstudante: Início")
	
	var req RegisterEstudanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] RegisterEstudante: Erro no bind JSON: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterEstudante: Nome: %s", req.Nome)

	if err := utils.ValidateNome(req.Nome); err != nil {
		log.Printf("[ERROR] RegisterEstudante: Nome inválido: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateSenha(req.Senha); err != nil {
		log.Printf("[ERROR] RegisterEstudante: Senha inválida: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != nil {
		if err := utils.ValidateEmail(*req.Email); err != nil {
			log.Printf("[ERROR] RegisterEstudante: Email inválido: %v", err)
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Telefone != nil {
		if err := utils.ValidatePhone(*req.Telefone); err != nil {
			log.Printf("[ERROR] RegisterEstudante: Telefone inválido: %v", err)
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if err := utils.ValidateBilhete(utils.SafeDeref(req.BilheteIdentidade)); err != nil {
		log.Printf("[ERROR] RegisterEstudante: Bilhete principal inválido: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateBilhete(utils.SafeDeref(req.BilheteIdentidadeResp)); err != nil {
		log.Printf("[ERROR] RegisterEstudante: Bilhete responsável inválido: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.BilheteIdentidade == nil && req.BilheteIdentidadeResp == nil {
		log.Printf("[ERROR] RegisterEstudante: Nenhum bilhete fornecido")
		utils.RespondWithValidationError(c, fmt.Errorf("pelo menos um bilhete de identidade é obrigatório"))
		return
	}

	// Verificar se o bilhete_identidade já existe
	if req.BilheteIdentidade != nil && *req.BilheteIdentidade != "" {
		log.Printf("[DEBUG] RegisterEstudante: Verificando bilhete existente: %s", *req.BilheteIdentidade)
		
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(*req.BilheteIdentidade)
		if err != nil {
			log.Printf("[ERROR] RegisterEstudante: Erro ao verificar bilhete: %v", err)
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil {
			log.Printf("[ERROR] RegisterEstudante: Bilhete já cadastrado")
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado"))
			return
		}
	}

	log.Printf("[DEBUG] RegisterEstudante: Gerando código único")

	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		log.Printf("[ERROR] RegisterEstudante: Erro ao gerar código: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterEstudante: Código gerado: %s", codigoEstudante)

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[ERROR] RegisterEstudante: Erro ao gerar hash: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterEstudante: Criando agregado")

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
		log.Printf("[ERROR] RegisterEstudante: Erro ao criar agregado: %v", err)
		utils.RespondWithValidationError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterEstudante: Salvando eventos")

	if err := repository.Save(estudante); err != nil {
		log.Printf("[ERROR] RegisterEstudante: Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("[DEBUG] RegisterEstudante: Sucesso - ID: %s, Código: %s", estudante.ID, codigoEstudante)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante criado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
		},
	})
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