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

	log.Printf("Login bem-sucedido: %s (%s)", userName, req.Type)

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

	log.Printf("Academia criada: %s - %s", codigo, req.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message": "academia criada com sucesso",
		"data": gin.H{
			"id":              academia.ID,
			"codigo_academia": academia.CodigoAcademia,
		},
	})
}

type RegisterEstudanteRequest struct {
	Senha                 string     `json:"senha" binding:"required"`
	Nome                  string     `json:"nome" binding:"required"`
	Email                 *string    `json:"email"`
	Telefone              *string    `json:"telefone"`
	BilheteIdentidade     *string    `json:"bilhete_identidade"`
	BilheteIdentidadeResp *string    `json:"bilhete_identidade_responsavel"`
	AnoEscolar            *string    `json:"ano_escolar"`
	AnoSuperior           *string    `json:"ano_superior"`
	CursoMedioID          *uuid.UUID `json:"curso_medio_id"`
	CursoSuperiorID       *uuid.UUID `json:"curso_superior_id"`
	StatusEscolar         *string    `json:"status_escolar"`
	StatusSuperior        *string    `json:"status_superior"`
}

func RegisterEstudante(c *gin.Context) {
	var req RegisterEstudanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

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
	
	if req.BilheteIdentidade != nil && req.BilheteIdentidadeResp != nil {
		if *req.BilheteIdentidade == *req.BilheteIdentidadeResp {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade do estudante e bilhete do responsável não podem ser iguais"))
			return
		}
	}

	if err := utils.ValidateBilhete(utils.SafeDeref(req.BilheteIdentidadeResp)); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.BilheteIdentidade == nil && req.BilheteIdentidadeResp == nil {
		utils.RespondWithValidationError(c, fmt.Errorf("pelo menos um bilhete de identidade é obrigatório"))
		return
	}

	if req.BilheteIdentidade != nil && *req.BilheteIdentidade != "" {
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(*req.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado"))
			return
		}
	}

	if req.CursoMedioID != nil && *req.CursoMedioID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id não encontrado"))
			return
		}
		if curso.Type != "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id deve ser do tipo 'medio'"))
			return
		}
	}

	if req.CursoSuperiorID != nil && *req.CursoSuperiorID != uuid.Nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoSuperiorID)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id não encontrado"))
			return
		}
		if curso.Type != "superior" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id deve ser do tipo 'superior'"))
			return
		}
	}

	if req.CursoMedioID != nil && *req.CursoMedioID != uuid.Nil && req.AnoEscolar != nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoMedioID)
		if curso != nil {
			// Verificar se o ano está no curso
			anoValido := false
			for _, nivelCurso := range curso.Nivel {
				if nivelCurso == *req.AnoEscolar {
					anoValido = true
					break
				}
			}
			if !anoValido {
				utils.RespondWithValidationError(c, fmt.Errorf("ano_escolar '%s' não existe no curso selecionado", *req.AnoEscolar))
				return
			}
		}
	}

	if req.CursoSuperiorID != nil && *req.CursoSuperiorID != uuid.Nil && req.AnoSuperior != nil {
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(*req.CursoSuperiorID)
		if curso != nil {
			// Verificar se o ano está no curso
			anoValido := false
			for _, nivelCurso := range curso.Nivel {
				if nivelCurso == *req.AnoSuperior {
					anoValido = true
					break
				}
			}
			if !anoValido {
				utils.RespondWithValidationError(c, fmt.Errorf("ano_superior '%s' não existe no curso selecionado", *req.AnoSuperior))
				return
			}
		}
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
		req.CursoMedioID,
		req.CursoSuperiorID,
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

	log.Printf("Estudante criado: %s - %s", codigoEstudante, req.Nome)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante criado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
		},
	})
}

// CadastroEstudanteAcademiaRequest - payload para cadastro por academia
type CadastroEstudanteAcademiaRequest struct {
	Nome                  string `json:"nome" binding:"required"`
	Email                 string `json:"email"`
	Telefone              string `json:"telefone"`
	BilheteIdentidade     string `json:"bilhete_identidade"`
	BilheteResponsavel    string `json:"bilhete_identidade_responsavel"`
	AnoEscolar            string `json:"ano_escolar"`
	AnoSuperior           string `json:"ano_superior"`
	CursoMedioID          string `json:"curso_medio_id"`
	CursoSuperiorID       string `json:"curso_superior_id"`
	StatusEscolar         string `json:"status_escolar"`
	StatusSuperior        string `json:"status_superior"`
}

// RegisterEstudantePorAcademia - Academia cadastra estudante já vinculado
func RegisterEstudantePorAcademia(c *gin.Context) {
	var req CadastroEstudanteAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := utils.ValidateNome(req.Nome); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if req.Email != "" {
		if err := utils.ValidateEmail(req.Email); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.Telefone != "" {
		if err := utils.ValidatePhone(req.Telefone); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.BilheteIdentidade != "" {
		if err := utils.ValidateBilhete(req.BilheteIdentidade); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.BilheteIdentidade != "" && req.BilheteResponsavel != "" {
		if req.BilheteIdentidade == req.BilheteResponsavel {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade do estudante e bilhete do responsável não podem ser iguais"))
			return
		}
	}

	if req.BilheteResponsavel != "" {
		if err := utils.ValidateBilhete(req.BilheteResponsavel); err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
	}

	if req.BilheteIdentidade == "" && req.BilheteResponsavel == "" {
		utils.RespondWithValidationError(c, fmt.Errorf("pelo menos um bilhete de identidade é obrigatório"))
		return
	}

	if req.BilheteIdentidade != "" {
		estudanteProj := getEstudanteProjection(c)
		existente, err := estudanteProj.GetByBilheteIdentidadePrincipal(req.BilheteIdentidade)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if existente != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("bilhete de identidade já cadastrado"))
			return
		}
	}

	var emailPtr, telefonePtr, bilhetePtr, bilheteRespPtr *string
	var anoEscolarPtr, anoSuperiorPtr *string
	var statusEscolarPtr, statusSuperiorPtr *string

	if req.Email != "" {
		emailPtr = &req.Email
	}
	if req.Telefone != "" {
		telefonePtr = &req.Telefone
	}
	if req.BilheteIdentidade != "" {
		bilhetePtr = &req.BilheteIdentidade
	}
	if req.BilheteResponsavel != "" {
		bilheteRespPtr = &req.BilheteResponsavel
	}
	if req.AnoEscolar != "" {
		anoEscolarPtr = &req.AnoEscolar
	}
	if req.AnoSuperior != "" {
		anoSuperiorPtr = &req.AnoSuperior
	}
	if req.StatusEscolar != "" {
		statusEscolarPtr = &req.StatusEscolar
	} else {
		defaultStatus := "em_andamento"
		statusEscolarPtr = &defaultStatus
	}
	if req.StatusSuperior != "" {
		statusSuperiorPtr = &req.StatusSuperior
	}

	var cursoMedioUUID *uuid.UUID
	var cursoSuperiorUUID *uuid.UUID

	if req.CursoMedioID != "" {
		parsed, err := uuid.Parse(req.CursoMedioID)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id inválido"))
			return
		}
		cursoMedioUUID = &parsed

		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id não encontrado"))
			return
		}
		if curso.Type != "medio" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_medio_id deve ser do tipo 'medio'"))
			return
		}
	}

	if req.CursoSuperiorID != "" {
		parsed, err := uuid.Parse(req.CursoSuperiorID)
		if err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id inválido"))
			return
		}
		cursoSuperiorUUID = &parsed

		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id não encontrado"))
			return
		}
		if curso.Type != "superior" {
			utils.RespondWithValidationError(c, fmt.Errorf("curso_superior_id deve ser do tipo 'superior'"))
			return
		}
	}

	if req.CursoMedioID != "" && req.AnoEscolar != "" {
		parsed, _ := uuid.Parse(req.CursoMedioID)
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso != nil {
			anoValido := false
			for _, nivelCurso := range curso.Nivel {
				if nivelCurso == req.AnoEscolar {
					anoValido = true
					break
				}
			}
			if !anoValido {
				utils.RespondWithValidationError(c, fmt.Errorf("ano_escolar '%s' não existe no curso selecionado", req.AnoEscolar))
				return
			}
		}
	}

	if req.CursoSuperiorID != "" && req.AnoSuperior != "" {
		parsed, _ := uuid.Parse(req.CursoSuperiorID)
		cursosProj := getCursosProjection(c)
		curso, _ := cursosProj.GetByID(parsed)
		if curso != nil {
			anoValido := false
			for _, nivelCurso := range curso.Nivel {
				if nivelCurso == req.AnoSuperior {
					anoValido = true
					break
				}
			}
			if !anoValido {
				utils.RespondWithValidationError(c, fmt.Errorf("ano_superior '%s' não existe no curso selecionado", req.AnoSuperior))
				return
			}
		}
	}

	academiaID, ok := middleware.GetUserID(c)
	if !ok {
		utils.RespondWithUnauthorizedError(c)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByID(academiaID)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	if academia == nil {
		utils.RespondWithError(c, http.StatusNotFound, "academia não encontrada", nil)
		return
	}

	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte("spuri123"), bcrypt.DefaultCost)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	if err := estudante.CriarComVinculo(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		emailPtr,
		telefonePtr,
		bilhetePtr,
		bilheteRespPtr,
		anoEscolarPtr,
		anoSuperiorPtr,
		cursoMedioUUID,
		cursoSuperiorUUID,
		statusEscolarPtr,
		statusSuperiorPtr,
		academia.CodigoAcademia,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := repository.Save(estudante); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("Estudante cadastrado: %s (academia: %s)", codigoEstudante, academia.CodigoAcademia)

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante cadastrado e vinculado com sucesso",
		"data": gin.H{
			"id":               estudante.ID,
			"codigo_estudante": codigoEstudante,
			"codigo_academia":  academia.CodigoAcademia,
			"status":           "ativo",
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