package handlers

import (
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
//   - usuario: qualquer identificador único do usuário —
//     admin     → e-mail
//     academia  → código de academia ou e-mail
//     estudante → código de estudante ou e-mail (e-mail deve estar verificado)
//   - senha: senha em texto plano — comparada via bcrypt
//
// O tipo do usuário é inferido automaticamente pela busca em cascata
// (admin → academia → estudante), eliminando a necessidade de o cliente
// informar o campo "type" explicitamente.
type LoginRequest struct {
	Usuario string `json:"usuario" binding:"required"`
	Senha   string `json:"senha"   binding:"required"`
}

// Login é o único endpoint de autenticação do sistema.
// Atende todos os tipos de usuário: admin, academia e estudante.
//
// Segurança:
//   - Timing-safe: bcrypt é SEMPRE executado, mesmo quando o usuário não existe,
//     igualando o tempo de resposta a "senha errada" e prevenindo user enumeration.
//   - Busca em cascata: admin → academia → estudante. O primeiro resultado válido
//     é usado; os demais não são consultados.
//   - Status verificado ANTES de emitir o JWT — conta inativa nunca recebe token.
//   - Resposta genérica: não indica qual campo (usuário ou senha) estava errado.
//   - SenhaHash NUNCA incluída na resposta.
func Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
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
		identifier string // e-mail (admin) ou código (academia/estudante) — retornado no response
		userStatus string
		userType   string // inferido pela busca — "admin" | "academia" | "estudante"
		userRole   string // preenchido apenas para admin
		userFound  bool
	)

	// ── Busca em cascata: admin → academia → estudante ─────────────────────
	//
	// Ordem de prioridade deliberada:
	//   1. Admin   — identificador exclusivo: e-mail (não colide com academia/estudante)
	//   2. Academia — identificador: código ou e-mail
	//   3. Estudante — identificador: código, e-mail ou telefone
	//
	// A busca para assim que o primeiro resultado é encontrado.

	// ── 1. Admin ───────────────────────────────────────────────────────────
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
		userType = "admin"
		userFound = true
	}

	// ── 2. Academia ────────────────────────────────────────────────────────
	if !userFound {
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
			// Bloquear login via e-mail se ainda não verificado.
			// Login por código de academia (ex: AC1234) é sempre permitido —
			// o código é atribuído pelo sistema e não requer verificação adicional.
			if academia.Email != nil && *academia.Email == req.Usuario && !academia.EmailVerificado {
				log.Printf("[INFO] [Login] Academia tentou login com e-mail não verificado: %s", req.Usuario)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "e-mail não verificado. Verifique sua caixa de entrada e confirme seu e-mail antes de fazer login.",
				})
				return
			}

			userID = academia.ID
			userName = academia.Nome
			senhaHash = academia.SenhaHash
			identifier = academia.CodigoAcademia
			userStatus = academia.Status
			userType = "academia"
			userFound = true
		}
	}

	// ── 3. Estudante ───────────────────────────────────────────────────────
	// GetAuthByIdentificador aceita código ou e-mail.
	// Telefone removido — verificação de telefone ainda não existe no sistema.
	if !userFound {
		estudanteProj := getEstudanteProjection(c)
		if estudanteProj == nil {
			return
		}
		estudanteAuth, err := estudanteProj.GetAuthByIdentificador(req.Usuario)
		if err != nil {
			log.Printf("❌ [Login/estudante] Erro ao buscar estudante '%s': %v", req.Usuario, err)
			utils.RespondWithInternalError(c, err)
			return
		}
		if estudanteAuth != nil {
			// Bloquear login via e-mail se ainda não verificado.
			// Login por código de estudante (ex: ABC1234) é sempre permitido —
			// o código é atribuído pelo sistema e não requer verificação adicional.
			if estudanteAuth.Email != nil && *estudanteAuth.Email == req.Usuario && !estudanteAuth.EmailVerificado {
				log.Printf("[INFO] [Login] Estudante tentou login com e-mail não verificado: %s", req.Usuario)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "e-mail não verificado. Verifique sua caixa de entrada e confirme seu e-mail antes de fazer login.",
				})
				return
			}

			userID = estudanteAuth.ID
			userName = estudanteAuth.Nome
			senhaHash = estudanteAuth.Hash
			identifier = estudanteAuth.Codigo
			userStatus = estudanteAuth.Status
			userType = "estudante"
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

	// Resposta deliberadamente genérica: não revela qual campo estava errado
	// nem qual tipo de usuário foi (ou não) encontrado.
	if !userFound {
		log.Printf("[INFO] [Login] Usuário não encontrado: %s", req.Usuario)
		utils.RespondWithError(c, http.StatusUnauthorized, "usuário não encontrado", nil)
		return
	}
	if bcryptErr != nil {
		log.Printf("[INFO] [Login] Senha incorreta para usuário: %s", req.Usuario)
		utils.RespondWithError(c, http.StatusUnauthorized, "senha incorreta", nil)
		return
	}

	// ── Verificação de status ANTES de emitir o JWT ────────────────────────
	// Estudantes podem autenticar com qualquer status diferente de "inativo"
	// (ex.: finalizado/regularização), enquanto admins e academias continuam
	// exigindo status exatamente "ativo".
	statusBloqueado := userStatus != "ativo"
	if userType == "estudante" {
		statusBloqueado = userStatus == "inativo"
	}
	if statusBloqueado {
		log.Printf("[INFO] [Login] Conta bloqueada por status — type: %s, identifier: %s, status: %s",
			userType, identifier, userStatus)
		var msg string
		switch userType {
		case "admin":
			msg = "conta inativa. Entre em contato com o suporte."
		case "academia":
			msg = "academia inativa. Entre em contato com o administrador."
		case "estudante":
			msg = "conta inativa. Entre em contato com sua academia."
		}
		utils.RespondWithError(c, http.StatusUnauthorized, msg, nil)
		return
	}

	// ── Emissão do JWT ─────────────────────────────────────────────────────
	token, err := middleware.GenerateToken(userID, userType)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [Login] Autenticação bem-sucedida: %s (%s)", userName, userType)

	// ── Resposta — SenhaHash NUNCA incluída ────────────────────────────────
	resp := gin.H{
		"token": token,
		"nome":  userName,
		"type":  userType,
	}
	if userType == "admin" {
		resp["email"] = identifier
		resp["role"] = userRole
	} else {
		resp["codigo"] = identifier
	}

	c.JSON(http.StatusOK, resp)
}

// Logout encerra a sessão no cliente.
//
// Como a API usa JWT stateless sem blacklist/revogação server-side,
// o logout efetivo consiste em o cliente descartar o token.
// Este endpoint existe para padronizar o fluxo no frontend e confirmar
// ao cliente que a operação de logout foi solicitada com sucesso.
func Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "logout realizado com sucesso",
	})
}
