// ============================================================================
// ARQUIVO: internal/handlers/admin_auth_handlers.go
// Handlers de autenticação e operações administrativas
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ============================================================================
// AUTENTICAÇÃO ADMIN
// ============================================================================

// LoginAdmin autentica um administrador
func LoginAdmin(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required"`
		Senha string `json:"senha" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Buscar admin
	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(req.Email)
	if err != nil || admin == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
		return
	}

	// Verificar status
	if admin.Status != "ativo" {
		c.JSON(http.StatusForbidden, gin.H{"error": "administrador inativo"})
		return
	}

	// Verificar senha
	if err := bcrypt.CompareHashAndPassword([]byte(admin.SenhaHash), []byte(req.Senha)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
		return
	}

	// Gerar token
	token, err := middleware.GenerateToken(admin.ID, "admin")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao gerar token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token": token,
		"nome":  admin.Nome,
		"role":  admin.Role,
		"type":  "admin",
	})
}

// RegisterAdmin cria um novo administrador
func RegisterAdmin(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		Nome  string `json:"nome" binding:"required"`
		Email string `json:"email" binding:"required"`
		Senha string `json:"senha" binding:"required"`
		Role  string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	// Validar role
	if req.Role != "fpp" && req.Role != "adm" && req.Role != "gerente" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role deve ser 'fpp', 'adm' ou 'gerente'"})
		return
	}

	// Buscar admin que está criando
	adminProj := getAdminProjection(c)
	creatorAdmin, err := adminProj.GetByID(userID)
	if err != nil || creatorAdmin == nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "administrador não encontrado"})
		return
	}

	// Verificar permissão hierárquica
	repository := getRepository(c)
	creatorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar administrador"})
		return
	}

	creator := creatorAgg.(*aggregates.Admin)
	if err := creator.ValidatePermission(req.Role); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	// Verificar se email já existe
	existing, _ := adminProj.GetByEmail(req.Email)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "email já cadastrado"})
		return
	}

	// Hash da senha
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	// Criar agregado Admin
	newAdmin := aggregates.NewAdmin()
	if err := newAdmin.Criar(
		req.Nome,
		req.Email,
		string(hashedPassword),
		req.Role,
		&userID,
	); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Salvar eventos
	if err := repository.Save(newAdmin); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao criar administrador"})
		return
	}

	// Registrar ação
	creator.RegistrarAcao("admin_criado", map[string]interface{}{
		"novo_admin_id": newAdmin.ID.String(),
		"role":          req.Role,
		"email":         req.Email,
	})
	repository.Save(creator)

	c.JSON(http.StatusCreated, gin.H{
		"message": "administrador criado com sucesso",
		"data": gin.H{
			"id":    newAdmin.ID,
			"nome":  newAdmin.Nome,
			"email": req.Email,
			"role":  newAdmin.Role,
		},
	})
}

// ============================================================================
// OPERAÇÕES ADMINISTRATIVAS - CONSULTAS
// ============================================================================

// ListarTodosEstudantes lista todos os estudantes (admin)
func ListarTodosEstudantes(c *gin.Context) {
	query := `
		SELECT 
			id, nome, codigo_estudante, bilhete_identidade,
			id_academia, ano_escolar, status_escolar,
			created_at, total_notas, total_faltas, total_inscricoes
		FROM projection_estudantes
		ORDER BY created_at DESC
	`

	type EstudanteSimples struct {
		ID               uuid.UUID  `db:"id" json:"id"`
		Nome             string     `db:"nome" json:"nome"`
		CodigoEstudante  string     `db:"codigo_estudante" json:"codigo_estudante"`
		BilheteID        *string    `db:"bilhete_identidade" json:"bilhete_identidade"`
		IDAcademia       *uuid.UUID `db:"id_academia" json:"id_academia"`
		AnoEscolar       *string    `db:"ano_escolar" json:"ano_escolar"`
		StatusEscolar    *string    `db:"status_escolar" json:"status_escolar"`
		CreatedAt        string     `db:"created_at" json:"created_at"`
		TotalNotas       int        `db:"total_notas" json:"total_notas"`
		TotalFaltas      int        `db:"total_faltas" json:"total_faltas"`
		TotalInscricoes  int        `db:"total_inscricoes" json:"total_inscricoes"`
	}

	var estudantes []EstudanteSimples
	client := getGenesisClient(c)
	if err := client.DB().Select(&estudantes, query); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar estudantes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"estudantes": estudantes,
		"total":      len(estudantes),
	})
}

// ListarTodasAcademias lista todas as academias (admin)
func ListarTodasAcademias(c *gin.Context) {
	query := `
		SELECT 
			id, nome, codigo_academia, type, provincia,
			status, nivel_escolar, created_at,
			total_estudantes, total_inscricoes_pendentes
		FROM projection_academias
		ORDER BY created_at DESC
	`

	type AcademiaSimples struct {
		ID                       uuid.UUID `db:"id" json:"id"`
		Nome                     string    `db:"nome" json:"nome"`
		CodigoAcademia           string    `db:"codigo_academia" json:"codigo_academia"`
		Type                     string    `db:"type" json:"type"`
		Provincia                string    `db:"provincia" json:"provincia"`
		Status                   string    `db:"status" json:"status"`
		NivelEscolar             *string   `db:"nivel_escolar" json:"nivel_escolar"`
		CreatedAt                string    `db:"created_at" json:"created_at"`
		TotalEstudantes          int       `db:"total_estudantes" json:"total_estudantes"`
		TotalInscricoesPendentes int       `db:"total_inscricoes_pendentes" json:"total_inscricoes_pendentes"`
	}

	var academias []AcademiaSimples
	client := getGenesisClient(c)
	if err := client.DB().Select(&academias, query); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academias"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"academias": academias,
		"total":     len(academias),
	})
}

// ============================================================================
// OPERAÇÕES ADMINISTRATIVAS - GERENCIAMENTO DE ACADEMIAS
// ============================================================================

// AtivarAcademia ativa uma academia (gerente, adm ou fpp)
func AtivarAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	academiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	// Verificar permissão do admin
	if err := verificarPermissaoAdmin(c, "gerente"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	// Carregar academia
	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaID, "Academia")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.Ativar(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao ativar academia"})
		return
	}

	// Registrar ação do admin
	registrarAcaoAdmin(c, userID, "academia_ativada", map[string]interface{}{
		"academia_id": academiaID.String(),
	})

	c.JSON(http.StatusOK, gin.H{
		"message":         "academia ativada com sucesso",
		"codigo_academia": academia.CodigoAcademia,
	})
}

// DesativarAcademia desativa uma academia (gerente, adm ou fpp)
func DesativarAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	academiaID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "motivo é obrigatório"})
		return
	}

	// Verificar permissão
	if err := verificarPermissaoAdmin(c, "gerente"); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	// Carregar academia
	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaID, "Academia")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.Desativar(req.Motivo); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := repository.Save(academia); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao desativar academia"})
		return
	}

	// Registrar ação
	registrarAcaoAdmin(c, userID, "academia_desativada", map[string]interface{}{
		"academia_id": academiaID.String(),
		"motivo":      req.Motivo,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":         "academia desativada com sucesso",
		"codigo_academia": academia.CodigoAcademia,
	})
}

// ============================================================================
// HELPERS
// ============================================================================

func getAdminProjection(c *gin.Context) *projections.AdminProjection {
	client := getGenesisClient(c)
	return projections.NewAdminProjection(client)
}

// verificarPermissaoAdmin verifica se o admin tem a permissão necessária
func verificarPermissaoAdmin(c *gin.Context, minRole string) error {
	userID, _ := middleware.GetUserID(c)

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByID(userID)
	if err != nil || admin == nil {
		return fmt.Errorf("administrador não encontrado")
	}

	if admin.Status != "ativo" {
		return fmt.Errorf("administrador está inativo")
	}

	// Hierarquia
	hierarchy := map[string]int{
		"fpp":     3,
		"adm":     2,
		"gerente": 1,
	}

	if hierarchy[admin.Role] < hierarchy[minRole] {
		return fmt.Errorf("permissão negada: requer role '%s' ou superior", minRole)
	}

	return nil
}

// registrarAcaoAdmin registra uma ação administrativa
func registrarAcaoAdmin(c *gin.Context, adminID uuid.UUID, acao string, detalhes map[string]interface{}) {
	repository := getRepository(c)
	
	adminAgg, err := repository.Load(adminID, "Admin")
	if err != nil {
		log.Printf("Erro ao carregar admin para registrar ação: %v", err)
		return
	}

	admin := adminAgg.(*aggregates.Admin)
	admin.RegistrarAcao(acao, detalhes)
	repository.Save(admin)
}