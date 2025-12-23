package handlers

import (
	"fmt"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/genesisdb"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// LoginRequest representa uma requisição de login
type LoginRequest struct {
	Usuario string `json:"usuario" binding:"required"`
	Senha   string `json:"senha" binding:"required"`
	Type    string `json:"type" binding:"required"`
}

// LoginResponse representa a resposta de login
type LoginResponse struct {
	Token string    `json:"token"`
	ID    uuid.UUID `json:"id"`
	Nome  string    `json:"nome"`
	Type  string    `json:"type"`
}

// Login autentica um usuário (academia ou estudante)
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Validar tipo
	if req.Type != "estudante" && req.Type != "academia" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tipo deve ser 'estudante' ou 'academia'"})
		return
	}

	// Obter projeções do contexto
	estudanteProj := getEstudanteProjection(c)
	academiaProj := getAcademiaProjection(c)

	var userID uuid.UUID
	var userName string
	var senhaHash string

	if req.Type == "academia" {
		// Buscar academia na projeção
		academia, err := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
			return
		}
		if academia == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
			return
		}

		userID = academia.ID
		userName = academia.Nome
		senhaHash = academia.SenhaHash

	} else {
		// Buscar estudante na projeção
		estudante, err := estudanteProj.GetByBilhete(req.Usuario)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar estudante"})
			return
		}
		if estudante == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
			return
		}

		userID = estudante.ID
		userName = estudante.Nome
		senhaHash = "" // TODO: adicionar senha_hash na projeção
	}

	// Verificar senha
	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.Senha)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
		return
	}

	// Gerar token
	token, err := middleware.GenerateToken(userID, req.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao gerar token"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token: token,
		ID:    userID,
		Nome:  userName,
		Type:  req.Type,
	})
}

// RegisterAcademiaRequest representa uma requisição de registro de academia
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

// RegisterAcademia registra uma nova academia usando Event Sourcing
func RegisterAcademia(c *gin.Context) {
	var req RegisterAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Validações
	if req.Type != "escola" && req.Type != "superior" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type deve ser 'escola' ou 'superior'"})
		return
	}

	if req.Type == "escola" && req.NivelEscolar == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nivel_escolar é obrigatório para escolas"})
		return
	}

	// Validar província
	codigoProvincia, err := validarProvincia(req.Provincia)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Gerar código da academia
	codigo := generateCodigoAcademia(codigoProvincia)

	// Hash da senha
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	// Criar agregado Academia
	repository := getRepository(c)
	academia := aggregates.NewAcademia()

	// Executar comando Criar - usando hashedPassword aqui
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos (Event Sourcing)
	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("erro ao criar academia: %v", err)})
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

// RegisterEstudanteRequest representa uma requisição de registro de estudante
type RegisterEstudanteRequest struct {
	Senha                 string  `json:"senha" binding:"required"`
	Nome                  string  `json:"nome" binding:"required"`
	BilheteIdentidade     *string `json:"bilhete_identidade"`
	BilheteIdentidadeResp *string `json:"bilhete_identidade_responsavel"`
	AnoEscolar            *string `json:"ano_escolar"`
	AnoSuperior           *string `json:"ano_superior"`
	CursoMedio            *string `json:"curso_medio"`
	CursoSuperior         *string `json:"curso_superior"`
	StatusEscolar         *string `json:"status_escolar"`
	StatusSuperior        *string `json:"status_superior"`
}

// RegisterEstudante registra um novo estudante usando Event Sourcing
func RegisterEstudante(c *gin.Context) {
	var req RegisterEstudanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Validações
	if req.BilheteIdentidade == nil && req.BilheteIdentidadeResp == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pelo menos um bilhete de identidade é obrigatório"})
		return
	}

	// Hash da senha
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	// Criar agregado Estudante
	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	// Executar comando Criar
	if err := estudante.Criar(
		req.Nome,
		req.BilheteIdentidade,
		req.BilheteIdentidadeResp,
		req.AnoEscolar,
		req.AnoSuperior,
		req.CursoMedio,
		req.CursoSuperior,
		req.StatusEscolar,
		req.StatusSuperior,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos (Event Sourcing)
	if err := repository.Save(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("erro ao criar estudante: %v", err)})
		return
	}

	// TODO: Armazenar hashedPassword no agregado/evento
	_ = hashedPassword // Temporariamente ignorar até implementar

	c.JSON(http.StatusCreated, gin.H{
		"message": "estudante criado com sucesso",
		"data": gin.H{
			"id": estudante.ID,
		},
	})
}

// Helper functions

func getRepository(c *gin.Context) *genesisdb.AggregateRepository {
	repo, _ := c.Get("repository")
	return repo.(*genesisdb.AggregateRepository)
}

func getEstudanteProjection(c *gin.Context) *projections.EstudanteProjection {
	client := getGenesisClient(c)
	return projections.NewEstudanteProjection(client)
}

func getAcademiaProjection(c *gin.Context) *projections.AcademiaProjection {
	client := getGenesisClient(c)
	return projections.NewAcademiaProjection(client)
}

func getGenesisClient(c *gin.Context) *genesisdb.Client {
	// Criar client temporário (idealmente injetar diretamente)
	config := genesisdb.DefaultConfig()
	client, _ := genesisdb.NewClient(config)
	return client
}

// validarProvincia valida e retorna o código da província
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

// generateCodigoAcademia gera um código único para a academia
func generateCodigoAcademia(codigoProvincia string) string {
	ano := time.Now().Year()
	numero := time.Now().UnixNano() % 10000
	return fmt.Sprintf("%s%d%d", codigoProvincia, ano, numero)
}