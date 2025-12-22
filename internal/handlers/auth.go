package handlers

import (
	"fmt"
	"net/http"
	"spuri/internal/domain"
	"spuri/internal/middleware"
	"spuri/internal/store"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Login autentica um usuário (academia ou estudante)
func Login(c *gin.Context) {
	var req domain.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "dados inválidos"})
		return
	}

	// Validar tipo
	if req.Type != "estudante" && req.Type != "academia" {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "tipo deve ser 'estudante' ou 'academia'"})
		return
	}

	var userID uuid.UUID
	var userName string
	var senhaHash string

	if req.Type == "academia" {
		// Buscar academia
		academia, err := store.GetAcademiaByCodigoOrEmail(req.Usuario)
		if err != nil {
			c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar academia"})
			return
		}
		if academia == nil {
			c.JSON(http.StatusUnauthorized, domain.ErrorResponse{Error: "credenciais inválidas"})
			return
		}

		userID = academia.ID
		userName = academia.Nome
		senhaHash = academia.SenhaHash

	} else {
		// Buscar estudante
		estudante, err := store.GetEstudanteByBilhete(req.Usuario)
		if err != nil {
			c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao buscar estudante"})
			return
		}
		if estudante == nil {
			c.JSON(http.StatusUnauthorized, domain.ErrorResponse{Error: "credenciais inválidas"})
			return
		}

		userID = estudante.ID
		userName = estudante.Nome
		senhaHash = estudante.SenhaHash
	}

	// Verificar senha
	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.Senha)); err != nil {
		c.JSON(http.StatusUnauthorized, domain.ErrorResponse{Error: "credenciais inválidas"})
		return
	}

	// Gerar token
	token, err := middleware.GenerateToken(userID, req.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao gerar token"})
		return
	}

	c.JSON(http.StatusOK, domain.LoginResponse{
		Token: token,
		ID:    userID,
		Nome:  userName,
		Type:  req.Type,
	})
}

// RegisterAcademia registra uma nova academia
func RegisterAcademia(c *gin.Context) {
	var req domain.RegisterAcademiaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "dados inválidos"})
		return
	}

	// Validações
	if req.Type != "escola" && req.Type != "superior" {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "type deve ser 'escola' ou 'superior'"})
		return
	}

	if req.Type == "escola" && req.NivelEscolar == nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "nivel_escolar é obrigatório para escolas"})
		return
	}

	// Hash da senha
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao processar senha"})
		return
	}

	// Gerar código da academia
	codigo := generateCodigoAcademia(req.Endereco)

	// Criar academia
	academia := &domain.Academia{
		Type:           req.Type,
		Nome:           req.Nome,
		CodigoAcademia: codigo,
		SenhaHash:      string(hashedPassword),
		Endereco:       req.Endereco,
		NumeroTelefone: req.NumeroTelefone,
		Email:          req.Email,
		Website:        req.Website,
		NivelEscolar:   req.NivelEscolar,
		Status:         "ativo",
		Cursos:         req.Cursos,
	}

	if err := store.CreateAcademia(academia); err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: fmt.Sprintf("erro ao criar academia: %v", err)})
		return
	}

	// Criar evento
	metadata := domain.EventMetadata{
		ActorID:   academia.ID,
		ActorType: "Academia",
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	event, err := domain.NewEvent(
		academia.ID,
		domain.AggregateTypeAcademia,
		domain.EventTypeAcademiaCriada,
		map[string]interface{}{
			"nome":            academia.Nome,
			"type":            academia.Type,
			"codigo_academia": academia.CodigoAcademia,
		},
		metadata,
	)

	if err == nil {
		store.SaveEvent(event)
	}

	c.JSON(http.StatusCreated, domain.SuccessResponse{
		Message: "academia criada com sucesso",
		Data: map[string]interface{}{
			"id":              academia.ID,
			"codigo_academia": academia.CodigoAcademia,
		},
	})
}

// RegisterEstudante registra um novo estudante
func RegisterEstudante(c *gin.Context) {
	var req domain.RegisterEstudanteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "dados inválidos"})
		return
	}

	// Validações
	if req.BilheteIdentidade == nil && req.BilheteIdentidadeResp == nil {
		c.JSON(http.StatusBadRequest, domain.ErrorResponse{Error: "pelo menos um bilhete de identidade é obrigatório"})
		return
	}

	// Hash da senha
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: "erro ao processar senha"})
		return
	}

	// Criar estudante
	estudante := &domain.Estudante{
		Nome:                  req.Nome,
		SenhaHash:             string(hashedPassword),
		BilheteIdentidade:     req.BilheteIdentidade,
		BilheteIdentidadeResp: req.BilheteIdentidadeResp,
		AnoEscolar:            req.AnoEscolar,
		AnoSuperior:           req.AnoSuperior,
		CursoMedio:            req.CursoMedio,
		CursoSuperior:         req.CursoSuperior,
		StatusEscolar:         req.StatusEscolar,
		StatusSuperior:        req.StatusSuperior,
	}

	if err := store.CreateEstudante(estudante); err != nil {
		c.JSON(http.StatusInternalServerError, domain.ErrorResponse{Error: fmt.Sprintf("erro ao criar estudante: %v", err)})
		return
	}

	// Criar evento
	metadata := domain.EventMetadata{
		ActorID:   estudante.ID,
		ActorType: "Estudante",
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	event, err := domain.NewEvent(
		estudante.ID,
		domain.AggregateTypeEstudante,
		domain.EventTypeEstudanteCriado,
		map[string]interface{}{
			"nome": estudante.Nome,
		},
		metadata,
	)

	if err == nil {
		store.SaveEvent(event)
	}

	c.JSON(http.StatusCreated, domain.SuccessResponse{
		Message: "estudante criado com sucesso",
		Data: map[string]interface{}{
			"id": estudante.ID,
		},
	})
}

// generateCodigoAcademia gera um código único para a academia
func generateCodigoAcademia(endereco string) string {
	// Extrair província (última palavra geralmente)
	parts := strings.Split(strings.TrimSpace(endereco), ",")
	provincia := "UNK"
	
	if len(parts) > 0 {
		lastPart := strings.TrimSpace(parts[len(parts)-1])
		provincia = strings.ToUpper(lastPart[:3])
	}

	// Mapa de províncias
	provinciaMap := map[string]string{
		"LUA": "LUA", "LUAN": "LUA",
		"BEN": "BGO", "BENG": "BGU",
		"HUA": "HUA", "HUAM": "HUA",
		"CAB": "CAB", "CABI": "CAB",
	}

	if code, ok := provinciaMap[provincia]; ok {
		provincia = code
	}

	// Ano atual
	ano := time.Now().Year()

	// Número sequencial (simplificado - em produção usar contador no banco)
	numero := time.Now().UnixNano() % 10000

	return fmt.Sprintf("%s%d%d", provincia, ano, numero)
}