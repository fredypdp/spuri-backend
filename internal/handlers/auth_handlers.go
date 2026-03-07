package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// LoginRequest é o corpo do único endpoint de autenticação do sistema.
//
// Campos:
//   - usuario: email (admin), código ou email (academia), código (estudante)
//   - senha:   senha em texto plano — comparada via bcrypt
//   - type:    "admin" | "academia" | "estudante"
type LoginRequest struct {
	Usuario string `json:"usuario" binding:"required"`
	Senha   string `json:"senha"   binding:"required"`
	Type    string `json:"type"    binding:"required"`
}

// Login é o único endpoint de autenticação do sistema.
// Atende todos os tipos de usuário: admin, academia e estudante.
//
// Segurança:
//   - Timing-safe: bcrypt é SEMPRE executado, mesmo quando o usuário não existe,
//     igualando o tempo de resposta a "senha errada" e prevenindo user enumeration.
//   - Status verificado ANTES de emitir o JWT — conta inativa nunca recebe token.
//   - Resposta genérica: não indica qual campo (usuário ou senha) estava errado.
//   - SenhaHash NUNCA incluída na resposta.
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	switch req.Type {
	case "admin", "academia", "estudante":
		// tipos válidos — prosseguir
	default:
		utils.RespondWithValidationError(c, fmt.Errorf("type inválido: use 'admin', 'academia' ou 'estudante'"))
		return
	}

	// dummyHash: hash bcrypt válido usado quando o usuário não existe, para que
	// CompareHashAndPassword leve o mesmo tempo que no caso de senha errada,
	// prevenindo timing attack e user enumeration.
	const dummyHash = "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012345"

	var (
		userID     uuid.UUID
		userName   string
		senhaHash  string
		identifier string // email (admin) ou código (academia/estudante) — retornado no response
		userStatus string
		userRole   string // preenchido apenas para admin
		userFound  bool
	)

	switch req.Type {

	// ── Admin ──────────────────────────────────────────────────────────────
	// usuario = email do administrador
	case "admin":
		adminProj := getAdminProjection(c)
		if adminProj == nil {
			return // getAdminProjection → getDbClient já abortou com 500
		}
		admin, err := adminProj.GetByEmailForLogin(req.Usuario)
		if err != nil {
			log.Printf("❌ [Login/admin] Erro ao buscar admin '%s': %v", req.Usuario, err)
			utils.RespondWithInternalError(c, err)
			return
		}
		if admin != nil {
			userID = admin.ID
			userName = admin.Nome
			senhaHash = admin.SenhaHash
			identifier = admin.Email
			userStatus = admin.Status
			userRole = admin.Role
			userFound = true
		}

	// ── Academia ───────────────────────────────────────────────────────────
	// usuario = código ou email da academia
	case "academia":
		academiaProj := getAcademiaProjection(c)
		if academiaProj == nil {
			return
		}
		academia, err := academiaProj.GetByCodigoOrEmail(req.Usuario)
		if err != nil {
			log.Printf("❌ [Login/academia] Erro ao buscar academia '%s': %v", req.Usuario, err)
			utils.RespondWithInternalError(c, err)
			return
		}
		if academia != nil {
			userID = academia.ID
			userName = academia.Nome
			senhaHash = academia.SenhaHash
			identifier = academia.CodigoAcademia
			userStatus = academia.Status
			userFound = true
		}

	// ── Estudante ──────────────────────────────────────────────────────────
	// usuario = código do estudante
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		if estudanteProj == nil {
			return
		}
		// GetAuthByCodigo retorna EstudanteAuthDTO que contém o Hash mas tem
		// todos os campos com json:"-" — nunca serializado em respostas HTTP.
		// O EstudanteDTO público (GetByCodigo) não expõe SenhaHash.
		estudanteAuth, err := estudanteProj.GetAuthByCodigo(req.Usuario)
		if err != nil {
			log.Printf("❌ [Login/estudante] Erro ao buscar estudante '%s': %v", req.Usuario, err)
			utils.RespondWithInternalError(c, err)
			return
		}
		if estudanteAuth != nil {
			userID = estudanteAuth.ID
			userName = estudanteAuth.Nome
			senhaHash = estudanteAuth.Hash
			identifier = estudanteAuth.Codigo
			userStatus = estudanteAuth.Status
			userFound = true
		}
	}

	// ── Comparação bcrypt (timing-safe) ────────────────────────────────────
	// Sempre executada — mesmo quando userFound=false — para que o tempo de
	// resposta seja idêntico entre "usuário não existe" e "senha errada".
	hashToCompare := senhaHash
	if !userFound {
		hashToCompare = dummyHash
	}
	bcryptErr := bcrypt.CompareHashAndPassword([]byte(hashToCompare), []byte(req.Senha))

	// Resposta deliberadamente genérica: não revela qual campo estava errado.
	if !userFound || bcryptErr != nil {
		log.Printf("[INFO] [Login] Falha — type: %s, usuario: %s, found: %v", req.Type, req.Usuario, userFound)
		utils.RespondWithUnauthorizedError(c)
		return
	}

	// ── Verificação de status ANTES de emitir o JWT ────────────────────────
	if userStatus != "ativo" {
		log.Printf("[INFO] [Login] Conta inativa — type: %s, identifier: %s, status: %s",
			req.Type, identifier, userStatus)
		var msg string
		switch req.Type {
		case "admin":
			msg = "conta inativa. Entre em contato com o suporte."
		case "academia":
			msg = "academia inativa. Entre em contato com o administrador."
		case "estudante":
			msg = "conta inativa. Entre em contato com sua academia."
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": msg})
		return
	}

	// ── Emissão do JWT ─────────────────────────────────────────────────────
	token, err := middleware.GenerateToken(userID, req.Type)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [Login] Autenticação bem-sucedida: %s (%s)", userName, req.Type)

	// ── Resposta — SenhaHash NUNCA incluída ────────────────────────────────
	resp := gin.H{
		"token": token,
		"nome":  userName,
		"type":  req.Type,
	}
	if req.Type == "admin" {
		resp["email"] = identifier
		resp["role"] = userRole
	} else {
		resp["codigo"] = identifier
	}

	c.JSON(http.StatusOK, resp)
}