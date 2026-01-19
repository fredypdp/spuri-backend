// ============================================================================
// ARQUIVO: internal/handlers/auth_handlers.go
// ATUALIZADO: Usar codigo_estudante em vez de bilhete para login
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/services"
	"spuri/internal/utils" // 🔥 NOVO: Import do gerador de código
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// LoginRequest representa uma requisição de login
type LoginRequest struct {
	Usuario string `json:"usuario" binding:"required"` // codigo_estudante para estudantes, codigo_academia para academias
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

// 🔥 ATUALIZADO: Login usa codigo_estudante para estudantes
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
		// Buscar academia na projeção (código ou email)
		log.Printf("🔍 [LOGIN] Buscando academia com código/email: %s", req.Usuario)

		academia, err := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if err != nil {
			log.Printf("❌ [LOGIN] Erro ao buscar academia: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
			return
		}
		if academia == nil {
			log.Printf("❌ [LOGIN] Academia não encontrada na projeção")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
			return
		}

		log.Printf("✅ [LOGIN] Academia encontrada: %s (ID: %s)", academia.Nome, academia.ID)

		userID = academia.ID
		userName = academia.Nome
		senhaHash = academia.SenhaHash

	} else {
		// 🔥 ESTUDANTE - Buscar por CÓDIGO em vez de BILHETE
		log.Printf("🔍 [LOGIN] Buscando estudante com código: %s", req.Usuario)

		estudante, err := estudanteProj.GetByCodigo(req.Usuario)
		if err != nil {
			log.Printf("❌ [LOGIN] Erro ao buscar estudante: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar estudante"})
			return
		}
		if estudante == nil {
			log.Printf("❌ [LOGIN] Estudante não encontrado na projeção")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
			return
		}

		log.Printf("✅ [LOGIN] Estudante encontrado: %s (Código: %s)", estudante.Nome, estudante.CodigoEstudante)

		userID = estudante.ID
		userName = estudante.Nome
		senhaHash = estudante.SenhaHash
	}

	// Verificar senha
	log.Printf("🔐 [LOGIN] Verificando senha...")
	if senhaHash == "" {
		log.Printf("❌ [LOGIN] SenhaHash vazio na projeção!")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(senhaHash), []byte(req.Senha)); err != nil {
		log.Printf("❌ [LOGIN] Senha incorreta: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
		return
	}

	log.Printf("✅ [LOGIN] Senha correta!")

	// Gerar token
	log.Printf("🎫 [LOGIN] Gerando token JWT...")
	token, err := middleware.GenerateToken(userID, req.Type)
	if err != nil {
		log.Printf("❌ [LOGIN] Erro ao gerar token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao gerar token"})
		return
	}

	// 🔥 Preparar resposta com código
	var codigo string
	if req.Type == "academia" {
		// Buscar novamente para pegar o código
		academiaDTO, _ := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if academiaDTO != nil {
			codigo = academiaDTO.CodigoAcademia
		}
	} else {
		// Buscar novamente para pegar o código
		estudanteDTO, _ := estudanteProj.GetByCodigo(req.Usuario)
		if estudanteDTO != nil {
			codigo = estudanteDTO.CodigoEstudante
		}
	}

	log.Printf("✅ [LOGIN] Login bem-sucedido para: %s (%s) - Código: %s", userName, req.Type, codigo)
	c.JSON(http.StatusOK, gin.H{
		"token":  token,
		"codigo": codigo, // 🔥 Retorna codigo_estudante ou codigo_academia
		"nome":   userName,
		"type":   req.Type,
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

	// 🔥 ATUALIZADO: Validar nivel_escolar para escolas
	if req.Type == "escola" {
		if req.NivelEscolar == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "nivel_escolar é obrigatório para escolas"})
			return
		}
		
		// Validar valores permitidos
		validNiveis := map[string]bool{
			"fundamental": true,
			"medio":       true,
			"misto":       true,
		}
		
		if !validNiveis[*req.NivelEscolar] {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "nivel_escolar deve ser 'fundamental', 'medio' ou 'misto'",
			})
			return
		}
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
	log.Printf("🔐 [REGISTER] Gerando hash da senha...")
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("❌ [REGISTER] Erro ao gerar hash: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}
	log.Printf("✅ [REGISTER] Hash gerado: %s", string(hashedPassword[:20])+"...")

	// Criar agregado Academia
	log.Printf("🗃️ [REGISTER] Criando agregado Academia...")
	repository := getRepository(c)
	academia := aggregates.NewAcademia()

	// Executar comando Criar
	log.Printf("⚡ [REGISTER] Executando comando Criar...")
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
		log.Printf("❌ [REGISTER] Erro ao executar comando: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos (Event Sourcing)
	log.Printf("💾 [REGISTER] Salvando eventos no Banco de dados...")
	if err := repository.Save(academia); err != nil {
		log.Printf("❌ [REGISTER] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("erro ao criar academia: %v", err)})
		return
	}

	log.Printf("✅ [REGISTER] Academia criada com sucesso! ID: %s, Código: %s", academia.ID, academia.CodigoAcademia)

	c.JSON(http.StatusCreated, gin.H{
		"message": "academia criada com sucesso",
		"data": gin.H{
			"id":              academia.ID,
			"codigo_academia": academia.CodigoAcademia,
		},
	})
}

// RegisterEstudanteRequest representa uma requisição de registro de estudante
// Atualizar struct:
type RegisterEstudanteRequest struct {
	Senha                 string  `json:"senha" binding:"required"`
	Nome                  string  `json:"nome" binding:"required"`
	Email                 *string `json:"email"`              // 🔥 NOVO
	Telefone              *string `json:"telefone"`           // 🔥 NOVO
	BilheteIdentidade     *string `json:"bilhete_identidade"`
	BilheteIdentidadeResp *string `json:"bilhete_identidade_responsavel"`
	AnoEscolar            *string `json:"ano_escolar"`
	AnoSuperior           *string `json:"ano_superior"`
	CursoMedio            *string `json:"curso_medio"`
	CursoSuperior         *string `json:"curso_superior"`
	StatusEscolar         *string `json:"status_escolar"`
	StatusSuperior        *string `json:"status_superior"`
}

// Atualizar função RegisterEstudante:
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

	// Gerar código único
	client := getDbClient(c)
	codigoEstudante, err := utils.GenerateUniqueCodigoEstudante(client.DB())
	if err != nil {
		log.Printf("❌ [REGISTER] Erro ao gerar código: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao gerar código de estudante"})
		return
	}

	log.Printf("🎫 [REGISTER] Código gerado: %s", codigoEstudante)

	// Hash da senha
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("❌ [REGISTER] Erro ao gerar hash: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	// Criar agregado Estudante
	repository := getRepository(c)
	estudante := aggregates.NewEstudante()

	// 🔥 EXECUTAR comando Criar com email e telefone
	if err := estudante.Criar(
		req.Nome,
		codigoEstudante,
		string(hashedPassword),
		req.Email,              // 🔥 NOVO
		req.Telefone,           // 🔥 NOVO
		req.BilheteIdentidade,
		req.BilheteIdentidadeResp,
		req.AnoEscolar,
		req.AnoSuperior,
		req.CursoMedio,
		req.CursoSuperior,
		req.StatusEscolar,
		req.StatusSuperior,
	); err != nil {
		log.Printf("❌ [REGISTER] Erro ao executar comando: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	if err := repository.Save(estudante); err != nil {
		log.Printf("❌ [REGISTER] Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("erro ao criar estudante: %v", err)})
		return
	}

	// 🔥 Enviar email de verificação se email fornecido
	if req.Email != nil && *req.Email != "" {
		emailSvc := services.NewEmailService(client.DB())
		if err := emailSvc.SendVerificationEmail(estudante.ID, "estudante", *req.Email, req.Nome); err != nil {
			log.Printf("⚠️ [REGISTER] Erro ao enviar email de verificação: %v", err)
			// Não falhar registro por causa do email
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
		response["email_verificacao"] = "Email de verificação enviado. Verifique sua caixa de entrada."
	}

	c.JSON(http.StatusCreated, response)
}

// Helper functions

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
