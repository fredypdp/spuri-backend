// ============================================================================
// ARQUIVO: internal/handlers/admin_handlers.go
// DESCRIÇÃO: Handlers unificados para operações administrativas + logs debug
// ============================================================================

package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
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
	log.Printf("[DEBUG] LoginAdmin: Início")
	
	var req struct {
		Email string `json:"email" binding:"required"`
		Senha string `json:"senha" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] LoginAdmin: Erro no bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[DEBUG] LoginAdmin: Tentativa de login para email: %s", req.Email)

	// Buscar admin
	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(req.Email)
	if err != nil || admin == nil {
		log.Printf("[ERROR] LoginAdmin: Admin não encontrado ou erro: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
		return
	}

	log.Printf("[DEBUG] LoginAdmin: Admin encontrado - ID: %s, Nome: %s, Status: %s", admin.ID, admin.Nome, admin.Status)

	// Verificar status
	if admin.Status != "ativo" {
		log.Printf("[ERROR] LoginAdmin: Admin inativo")
		c.JSON(http.StatusForbidden, gin.H{"error": "administrador inativo"})
		return
	}

	// Verificar senha
	if err := bcrypt.CompareHashAndPassword([]byte(admin.SenhaHash), []byte(req.Senha)); err != nil {
		log.Printf("[ERROR] LoginAdmin: Senha incorreta")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "credenciais inválidas"})
		return
	}

	log.Printf("[DEBUG] LoginAdmin: Senha verificada com sucesso")

	// Gerar token
	token, err := middleware.GenerateToken(admin.ID, "admin")
	if err != nil {
		log.Printf("[ERROR] LoginAdmin: Erro ao gerar token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao gerar token"})
		return
	}

	log.Printf("[DEBUG] LoginAdmin: Token gerado com sucesso para %s", admin.Nome)

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
	log.Printf("[DEBUG] RegisterAdmin: Início - UserID criador: %s", userID)

	var req struct {
		Nome  string `json:"nome" binding:"required"`
		Email string `json:"email" binding:"required"`
		Senha string `json:"senha" binding:"required"`
		Role  string `json:"role" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] RegisterAdmin: Erro no bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "dados inválidos"})
		return
	}

	log.Printf("[DEBUG] RegisterAdmin: Criando admin - Nome: %s, Email: %s, Role: %s", req.Nome, req.Email, req.Role)

	// Validar role
	if req.Role != "fpp" && req.Role != "adm" && req.Role != "gerente" {
		log.Printf("[ERROR] RegisterAdmin: Role inválido: %s", req.Role)
		c.JSON(http.StatusBadRequest, gin.H{"error": "role deve ser 'fpp', 'adm' ou 'gerente'"})
		return
	}

	// Buscar admin que está criando
	adminProj := getAdminProjection(c)
	creatorAdmin, err := adminProj.GetByID(userID)
	if err != nil || creatorAdmin == nil {
		log.Printf("[ERROR] RegisterAdmin: Admin criador não encontrado: %v", err)
		c.JSON(http.StatusForbidden, gin.H{"error": "administrador não encontrado"})
		return
	}

	log.Printf("[DEBUG] RegisterAdmin: Admin criador encontrado - Nome: %s, Role: %s", creatorAdmin.Nome, creatorAdmin.Role)

	// Verificar permissão hierárquica
	repository := getRepository(c)
	creatorAgg, err := repository.Load(userID, "Admin")
	if err != nil {
		log.Printf("[ERROR] RegisterAdmin: Erro ao carregar agregado do criador: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao carregar administrador"})
		return
	}

	creator := creatorAgg.(*aggregates.Admin)
	if err := creator.ValidatePermission(req.Role); err != nil {
		log.Printf("[ERROR] RegisterAdmin: Permissão negada: %v", err)
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] RegisterAdmin: Permissão validada")

	// Verificar se email já existe
	existing, _ := adminProj.GetByEmail(req.Email)
	if existing != nil {
		log.Printf("[ERROR] RegisterAdmin: Email já existe: %s", req.Email)
		c.JSON(http.StatusConflict, gin.H{"error": "email já cadastrado"})
		return
	}

	// Hash da senha
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("[ERROR] RegisterAdmin: Erro ao fazer hash da senha: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao processar senha"})
		return
	}

	log.Printf("[DEBUG] RegisterAdmin: Hash de senha gerado")

	// Criar agregado Admin
	newAdmin := aggregates.NewAdmin()
	if err := newAdmin.Criar(
		req.Nome,
		req.Email,
		string(hashedPassword),
		req.Role,
		&userID,
	); err != nil {
		log.Printf("[ERROR] RegisterAdmin: Erro ao criar agregado: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] RegisterAdmin: Agregado criado - ID: %s", newAdmin.ID)

	// Salvar eventos
	if err := repository.Save(newAdmin); err != nil {
		log.Printf("[ERROR] RegisterAdmin: Erro ao salvar eventos: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao criar administrador"})
		return
	}

	log.Printf("[DEBUG] RegisterAdmin: Eventos salvos com sucesso")

	// Registrar ação
	creator.RegistrarAcao("admin_criado", map[string]interface{}{
		"novo_admin_id": newAdmin.ID.String(),
		"role":          req.Role,
		"email":         req.Email,
	})
	repository.Save(creator)

	log.Printf("[DEBUG] RegisterAdmin: Ação registrada no criador")

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

// ConsultarAdmin busca um admin por email
func ConsultarAdmin(c *gin.Context) {
	email := c.Param("email")
	log.Printf("[DEBUG] ConsultarAdmin: Buscando admin por email: %s", email)
	
	if email == "" {
		log.Printf("[ERROR] ConsultarAdmin: Email não fornecido")
		c.JSON(http.StatusBadRequest, gin.H{"error": "email não fornecido"})
		return
	}

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByEmail(email)
	if err != nil {
		log.Printf("[ERROR] ConsultarAdmin: Erro ao buscar admin: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar admin"})
		return
	}

	if admin == nil {
		log.Printf("[ERROR] ConsultarAdmin: Admin não encontrado")
		c.JSON(http.StatusNotFound, gin.H{"error": "admin não encontrado"})
		return
	}

	log.Printf("[DEBUG] ConsultarAdmin: Admin encontrado - ID: %s, Nome: %s", admin.ID, admin.Nome)

	c.JSON(http.StatusOK, gin.H{
		"id":         admin.ID,
		"nome":       admin.Nome,
		"email":      admin.Email,
		"role":       admin.Role,
		"status":     admin.Status,
		"created_at": admin.CreatedAt,
	})
}

// ============================================================================
// OPERAÇÕES ADMINISTRATIVAS - CONSULTAS
// ============================================================================

// ListarEstudantes lista estudantes baseado no tipo de usuário
func ListarEstudantes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	log.Printf("[DEBUG] ListarEstudantes: Início - UserID: %s, UserType: %s", userID, userType)

	type EstudanteSimples struct {
		ID              uuid.UUID `json:"id"`
		Nome            string    `json:"nome"`
		CodigoEstudante string    `json:"codigo_estudante"`
		BilheteID       *string   `json:"bilhete_identidade"`
		CodigoAcademia  *string   `json:"codigo_academia"`
		AnoEscolar      *string   `json:"ano_escolar"`
		AnoSuperior     *string   `json:"ano_superior"`
		StatusEscolar   string    `json:"status_escolar"`
		StatusSuperior  string    `json:"status_superior"`
		CreatedAt       string    `json:"created_at"`
		TotalNotas      int       `json:"total_notas"`
		TotalFaltas     int       `json:"total_faltas"`
		TotalInscricoes int       `json:"total_inscricoes"`
	}

	var estudantes []EstudanteSimples
	client := getDbClient(c)

	if userType == "academia" {
		log.Printf("[DEBUG] ListarEstudantes: Fluxo ACADEMIA")
		
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil {
			log.Printf("[ERROR] ListarEstudantes: Erro ao buscar academia: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar dados da academia"})
			return
		}
		if academiaDTO == nil {
			log.Printf("[ERROR] ListarEstudantes: Academia não encontrada - ID: %s", userID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar dados da academia"})
			return
		}

		log.Printf("[DEBUG] ListarEstudantes: Academia encontrada - Código: %s, Nome: %s", 
			academiaDTO.CodigoAcademia, academiaDTO.Nome)

		safeCodigoAcademia := db.SafeString(academiaDTO.CodigoAcademia)
		query := fmt.Sprintf(`
			SELECT 
				id, nome, codigo_estudante, bilhete_identidade, ano_superior,
				codigo_academia, ano_escolar, status_escolar, status_superior,
				created_at, total_notas, total_faltas, total_inscricoes
			FROM projection_estudantes
			WHERE codigo_academia = '%s'
			ORDER BY created_at DESC
		`, safeCodigoAcademia)

		log.Printf("[DEBUG] ListarEstudantes: Executando query academia")

		rows, err := client.DB().Query(query)
		if err != nil {
			log.Printf("[ERROR] ListarEstudantes: Erro na query: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar estudantes"})
			return
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var est EstudanteSimples
			err := rows.Scan(
				&est.ID, &est.Nome, &est.CodigoEstudante, &est.BilheteID, &est.AnoSuperior,
				&est.CodigoAcademia, &est.AnoEscolar, &est.StatusEscolar, &est.StatusSuperior,
				&est.CreatedAt, &est.TotalNotas, &est.TotalFaltas, &est.TotalInscricoes,
			)
			if err != nil {
				log.Printf("[ERROR] ListarEstudantes: Erro ao fazer scan: %v", err)
				continue
			}
			count++
			estudantes = append(estudantes, est)
		}

		if err := rows.Err(); err != nil {
			log.Printf("[ERROR] ListarEstudantes: Erro ao iterar rows: %v", err)
		}

		log.Printf("[DEBUG] ListarEstudantes: %d estudantes encontrados para academia", count)

		c.JSON(http.StatusOK, gin.H{
			"estudantes":      estudantes,
			"total":           len(estudantes),
			"tipo_usuario":    "academia",
			"codigo_academia": academiaDTO.CodigoAcademia,
			"nome_academia":   academiaDTO.Nome,
		})

	} else if userType == "admin" {
		log.Printf("[DEBUG] ListarEstudantes: Fluxo ADMIN")
		
		query := `
			SELECT 
				id, nome, codigo_estudante, bilhete_identidade,
				codigo_academia, ano_escolar, status_escolar, status_superior,
				created_at, total_notas, total_faltas, total_inscricoes
			FROM projection_estudantes
			ORDER BY created_at DESC
		`

		log.Printf("[DEBUG] ListarEstudantes: Executando query admin")

		rows, err := client.DB().Query(query)
		if err != nil {
			log.Printf("[ERROR] ListarEstudantes: Erro na query admin: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar estudantes"})
			return
		}
		defer rows.Close()

		count := 0
		for rows.Next() {
			var est EstudanteSimples
			err := rows.Scan(
				&est.ID, &est.Nome, &est.CodigoEstudante, &est.BilheteID,
				&est.CodigoAcademia, &est.AnoEscolar, &est.StatusEscolar, &est.StatusSuperior,
				&est.CreatedAt, &est.TotalNotas, &est.TotalFaltas, &est.TotalInscricoes,
			)
			if err != nil {
				log.Printf("[ERROR] ListarEstudantes: Erro ao fazer scan: %v", err)
				continue
			}
			count++
			estudantes = append(estudantes, est)
		}

		if err := rows.Err(); err != nil {
			log.Printf("[ERROR] ListarEstudantes: Erro ao iterar rows: %v", err)
		}

		log.Printf("[DEBUG] ListarEstudantes: %d estudantes encontrados para admin", count)

		c.JSON(http.StatusOK, gin.H{
			"estudantes":   estudantes,
			"total":        len(estudantes),
			"tipo_usuario": "admin",
		})

	} else {
		log.Printf("[ERROR] ListarEstudantes: Tipo de usuário inválido: %s", userType)
		c.JSON(http.StatusForbidden, gin.H{
			"error": "acesso negado: apenas academias e administradores",
		})
	}
}

// ============================================================================
// OPERAÇÕES ADMINISTRATIVAS - GERENCIAMENTO DE ACADEMIAS
// ============================================================================

// AtivarAcademia ativa uma academia (gerente, adm ou fpp)
func AtivarAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	codigoAcademia := c.Param("codigo")

	log.Printf("[DEBUG] AtivarAcademia: Início - UserID: %s, Código: %s", userID, codigoAcademia)

	// Verificar permissão do admin
	if err := verificarPermissaoAdmin(c, "gerente"); err != nil {
		log.Printf("[ERROR] AtivarAcademia: Permissão negada: %v", err)
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] AtivarAcademia: Permissão verificada")

	// Buscar academia pelo código
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil {
		log.Printf("[ERROR] AtivarAcademia: Erro ao buscar academia: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}
	if academiaDTO == nil {
		log.Printf("[ERROR] AtivarAcademia: Academia não encontrada: %s", codigoAcademia)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}
	
	log.Printf("[DEBUG] AtivarAcademia: Academia encontrada - ID: %s, Nome: %s, Status: %s", 
		academiaDTO.ID, academiaDTO.Nome, academiaDTO.Status)

	// Carregar agregado
	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		log.Printf("[ERROR] AtivarAcademia: Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	log.Printf("[DEBUG] AtivarAcademia: Agregado carregado")

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.Ativar(); err != nil {
		log.Printf("[ERROR] AtivarAcademia: Erro ao ativar: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] AtivarAcademia: Comando Ativar executado")

	if err := repository.Save(academia); err != nil {
		log.Printf("[ERROR] AtivarAcademia: Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao ativar academia"})
		return
	}

	log.Printf("[DEBUG] AtivarAcademia: Eventos salvos")

	// Registrar ação do admin
	registrarAcaoAdmin(c, userID, "academia_ativada", map[string]interface{}{
		"codigo_academia": codigoAcademia,
		"academia_id":     academiaDTO.ID.String(),
	})

	log.Printf("[DEBUG] AtivarAcademia: Sucesso - Academia %s ativada", codigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":         "academia ativada com sucesso",
		"codigo_academia": academia.CodigoAcademia,
		"nome":            academia.Nome,
	})
}

// DesativarAcademia desativa uma academia (gerente, adm ou fpp)
func DesativarAcademia(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	codigoAcademia := c.Param("codigo")

	log.Printf("[DEBUG] DesativarAcademia: Início - UserID: %s, Código: %s", userID, codigoAcademia)

	var req struct {
		Motivo string `json:"motivo" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] DesativarAcademia: Erro no bind JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "motivo é obrigatório"})
		return
	}
	
	log.Printf("[DEBUG] DesativarAcademia: Motivo: %s", req.Motivo)

	// Verificar permissão
	if err := verificarPermissaoAdmin(c, "gerente"); err != nil {
		log.Printf("[ERROR] DesativarAcademia: Permissão negada: %v", err)
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] DesativarAcademia: Permissão verificada")

	// Buscar academia pelo código
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil {
		log.Printf("[ERROR] DesativarAcademia: Erro ao buscar academia: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar academia"})
		return
	}
	if academiaDTO == nil {
		log.Printf("[ERROR] DesativarAcademia: Academia não encontrada: %s", codigoAcademia)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}
	
	log.Printf("[DEBUG] DesativarAcademia: Academia encontrada - ID: %s, Nome: %s, Status: %s", 
		academiaDTO.ID, academiaDTO.Nome, academiaDTO.Status)

	// Carregar agregado
	repository := getRepository(c)
	academiaAgg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		log.Printf("[ERROR] DesativarAcademia: Erro ao carregar agregado: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	log.Printf("[DEBUG] DesativarAcademia: Agregado carregado")

	academia := academiaAgg.(*aggregates.Academia)
	if err := academia.Desativar(req.Motivo); err != nil {
		log.Printf("[ERROR] DesativarAcademia: Erro ao desativar: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[DEBUG] DesativarAcademia: Comando Desativar executado")

	if err := repository.Save(academia); err != nil {
		log.Printf("[ERROR] DesativarAcademia: Erro ao salvar: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao desativar academia"})
		return
	}

	log.Printf("[DEBUG] DesativarAcademia: Eventos salvos")

	// Registrar ação
	registrarAcaoAdmin(c, userID, "academia_desativada", map[string]interface{}{
		"codigo_academia": codigoAcademia,
		"academia_id":     academiaDTO.ID.String(),
		"motivo":          req.Motivo,
	})

	log.Printf("[DEBUG] DesativarAcademia: Sucesso - Academia %s desativada", codigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":         "academia desativada com sucesso",
		"codigo_academia": academia.CodigoAcademia,
		"nome":            academia.Nome,
		"motivo":          req.Motivo,
	})
}

// ============================================================================
// OPERAÇÕES ADMINISTRATIVAS - PROJEÇÕES E SISTEMA
// ============================================================================

// RebuildProjection reconstrói uma projeção específica
func RebuildProjection(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	log.Printf("[DEBUG] RebuildProjection: UserID: %s", userID)
	
	// Verificar se é admin e buscar role
	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByID(userID)
	if err != nil || admin == nil {
		log.Printf("[ERROR] RebuildProjection: Admin não encontrado")
		c.JSON(http.StatusForbidden, gin.H{"error": "apenas administradores"})
		return
	}

	log.Printf("[DEBUG] RebuildProjection: Admin encontrado - Role: %s", admin.Role)

	if admin.Role != "fpp" && admin.Role != "adm" {
		log.Printf("[ERROR] RebuildProjection: Permissão insuficiente - Role: %s", admin.Role)
		c.JSON(http.StatusForbidden, gin.H{"error": "apenas FPP ou ADM podem reconstruir projeções"})
		return
	}

	projectionName := c.Param("name")
	if projectionName == "" {
		log.Printf("[ERROR] RebuildProjection: Nome não fornecido")
		c.JSON(http.StatusBadRequest, gin.H{"error": "nome da projeção não fornecido"})
		return
	}

	log.Printf("[DEBUG] RebuildProjection: Reconstruindo projeção: %s", projectionName)

	client := getDbClient(c)
	manager := projections.NewManager(client)

	// Registrar projeções
	manager.RegisterProjection("estudantes", projections.NewEstudanteProjection(client))
	manager.RegisterProjection("academias", projections.NewAcademiaProjection(client))
	manager.RegisterProjection("admins", projections.NewAdminProjection(client))
	manager.RegisterProjection("notas", projections.NewNotasProjection(client))
	manager.RegisterProjection("faltas", projections.NewFaltasProjection(client))
	manager.RegisterProjection("inscricoes", projections.NewInscricoesProjection(client))
	manager.RegisterProjection("cursos", projections.NewCursosProjection(client))
	manager.RegisterProjection("materias", projections.NewMateriasProjection(client))

	var err2 error
	if projectionName == "all" {
		log.Printf("[DEBUG] RebuildProjection: Reconstruindo TODAS as projeções")
		err2 = manager.RebuildAllProjections()
	} else {
		err2 = manager.RebuildProjection(projectionName)
	}

	if err2 != nil {
		log.Printf("[ERROR] RebuildProjection: Erro ao reconstruir %s: %v", projectionName, err2)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "erro ao reconstruir projeção",
			"details": err2.Error(),
		})
		return
	}

	log.Printf("[DEBUG] RebuildProjection: Sucesso - %s reconstruída", projectionName)

	c.JSON(http.StatusOK, gin.H{
		"message":    "projeção reconstruída com sucesso",
		"projection": projectionName,
	})
}

// GetProjectionStatus retorna o status de uma projeção
func GetProjectionStatus(c *gin.Context) {
	projectionName := c.Param("name")
	log.Printf("[DEBUG] GetProjectionStatus: Projeção: %s", projectionName)
	
	if projectionName == "" {
		log.Printf("[ERROR] GetProjectionStatus: Nome não fornecido")
		c.JSON(http.StatusBadRequest, gin.H{"error": "nome da projeção não fornecido"})
		return
	}

	client := getDbClient(c)
	
	var proj projections.Projection
	switch projectionName {
	case "estudantes":
		proj = projections.NewEstudanteProjection(client)
	case "academias":
		proj = projections.NewAcademiaProjection(client)
	case "admins":
		proj = projections.NewAdminProjection(client)
	case "notas":
		proj = projections.NewNotasProjection(client)
	case "faltas":
		proj = projections.NewFaltasProjection(client)
	case "inscricoes":
		proj = projections.NewInscricoesProjection(client)
	case "cursos":
		proj = projections.NewCursosProjection(client)
	case "materias":
		proj = projections.NewMateriasProjection(client)
	default:
		log.Printf("[ERROR] GetProjectionStatus: Projeção não encontrada: %s", projectionName)
		c.JSON(http.StatusNotFound, gin.H{"error": "projeção não encontrada"})
		return
	}

	lastEventID, err := proj.GetLastProcessedEventID()
	if err != nil {
		log.Printf("[ERROR] GetProjectionStatus: Erro ao obter lastEventID: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao obter status"})
		return
	}

	safeName := db.SafeString(projectionName)
	query := fmt.Sprintf(`
		SELECT last_processed_at, events_processed 
		FROM projection_checkpoints 
		WHERE projection_name = '%s'
	`, safeName)

	var lastProcessedAt *string
	var eventsProcessed int
	err = client.DB().QueryRow(query).Scan(&lastProcessedAt, &eventsProcessed)

	log.Printf("[DEBUG] GetProjectionStatus: LastEventID: %d, EventsProcessed: %d", lastEventID, eventsProcessed)

	status := gin.H{
		"projection_name":        projectionName,
		"last_processed_event":   lastEventID,
		"events_processed_total": eventsProcessed,
	}

	if err == nil && lastProcessedAt != nil {
		status["last_processed_at"] = *lastProcessedAt
	}

	c.JSON(http.StatusOK, status)
}

// GetAllProjectionsStatus retorna o status de todas as projeções
func GetAllProjectionsStatus(c *gin.Context) {
	log.Printf("[DEBUG] GetAllProjectionsStatus: Início")
	
	client := getDbClient(c)

	query := `
		SELECT projection_name, last_processed_event_id, last_processed_at, events_processed
		FROM projection_checkpoints
		ORDER BY projection_name
	`

	rows, err := client.DB().Query(query)
	if err != nil {
		log.Printf("[ERROR] GetAllProjectionsStatus: Erro na query: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar status"})
		return
	}
	defer rows.Close()

	var statuses []gin.H
	count := 0
	for rows.Next() {
		var name string
		var lastEventID int64
		var lastProcessedAt string
		var eventsProcessed int

		if err := rows.Scan(&name, &lastEventID, &lastProcessedAt, &eventsProcessed); err != nil {
			log.Printf("[ERROR] GetAllProjectionsStatus: Erro ao fazer scan: %v", err)
			continue
		}

		statuses = append(statuses, gin.H{
			"projection_name":      name,
			"last_processed_event": lastEventID,
			"last_processed_at":    lastProcessedAt,
			"events_processed":     eventsProcessed,
		})
		count++
	}

	log.Printf("[DEBUG] GetAllProjectionsStatus: %d projeções encontradas", count)

	c.JSON(http.StatusOK, gin.H{
		"projections": statuses,
		"total":       len(statuses),
	})
}

// GetLedgerStats retorna estatísticas do ledger
func GetLedgerStats(c *gin.Context) {
	log.Printf("[DEBUG] GetLedgerStats: Início")
	
	client := getDbClient(c)

	var totalEvents int64
	var firstEvent, lastEvent string

	query1 := `SELECT COUNT(*) FROM spuri_ledger`
	client.DB().QueryRow(query1).Scan(&totalEvents)
	log.Printf("[DEBUG] GetLedgerStats: Total de eventos: %d", totalEvents)

	query2 := `SELECT occurred_at FROM spuri_ledger ORDER BY id ASC LIMIT 1`
	client.DB().QueryRow(query2).Scan(&firstEvent)
	log.Printf("[DEBUG] GetLedgerStats: Primeiro evento: %s", firstEvent)

	query3 := `SELECT occurred_at FROM spuri_ledger ORDER BY id DESC LIMIT 1`
	client.DB().QueryRow(query3).Scan(&lastEvent)
	log.Printf("[DEBUG] GetLedgerStats: Último evento: %s", lastEvent)

	aggregateQuery := `
		SELECT aggregate_type, COUNT(*) as count 
		FROM spuri_ledger 
		GROUP BY aggregate_type 
		ORDER BY count DESC
	`
	rows, _ := client.DB().Query(aggregateQuery)
	defer rows.Close()

	aggregateStats := make(map[string]int64)
	for rows.Next() {
		var aggType string
		var count int64
		if err := rows.Scan(&aggType, &count); err == nil {
			aggregateStats[aggType] = count
		}
	}

	log.Printf("[DEBUG] GetLedgerStats: Estatísticas por agregado: %v", aggregateStats)

	c.JSON(http.StatusOK, gin.H{
		"total_events":        totalEvents,
		"first_event_at":      firstEvent,
		"last_event_at":       lastEvent,
		"events_by_aggregate": aggregateStats,
	})
}

// VerifyAllIntegrity verifica a integridade do ledger
func VerifyAllIntegrity(c *gin.Context) {
	log.Printf("[DEBUG] VerifyAllIntegrity: Início")
	
	client := getDbClient(c)

	query := `
		SELECT 
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE ledger_hash IS NOT NULL) as with_hash,
			COUNT(*) FILTER (WHERE previous_hash IS NOT NULL) as with_previous
		FROM spuri_ledger
	`

	var total, withHash, withPrevious int64
	err := client.DB().QueryRow(query).Scan(&total, &withHash, &withPrevious)
	if err != nil {
		log.Printf("[ERROR] VerifyAllIntegrity: Erro na query: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao verificar integridade"})
		return
	}

	integro := (total == withHash) && (total-1 == withPrevious)

	log.Printf("[DEBUG] VerifyAllIntegrity: Total: %d, WithHash: %d, WithPrevious: %d, Integro: %v", 
		total, withHash, withPrevious, integro)

	c.JSON(http.StatusOK, gin.H{
		"integro":              integro,
		"total_events":         total,
		"events_with_hash":     withHash,
		"events_with_previous": withPrevious,
		"message": func() string {
			if integro {
				return "✅ Ledger íntegro - todos os eventos possuem hash"
			}
			return "⚠️ Problemas de integridade detectados"
		}(),
	})
}

// ============================================================================
// HELPERS
// ============================================================================

// getAdminProjection retorna a projeção de admins
func getAdminProjection(c *gin.Context) *projections.AdminProjection {
	client := getDbClient(c)
	return projections.NewAdminProjection(client)
}

// verificarPermissaoAdmin verifica se o admin tem a permissão necessária
func verificarPermissaoAdmin(c *gin.Context, minRole string) error {
	userID, _ := middleware.GetUserID(c)

	log.Printf("[DEBUG] verificarPermissaoAdmin: UserID: %s, MinRole: %s", userID, minRole)

	adminProj := getAdminProjection(c)
	admin, err := adminProj.GetByID(userID)
	if err != nil {
		log.Printf("[ERROR] verificarPermissaoAdmin: Erro ao buscar admin: %v", err)
		return fmt.Errorf("administrador não encontrado")
	}
	if admin == nil {
		log.Printf("[ERROR] verificarPermissaoAdmin: Admin não encontrado")
		return fmt.Errorf("administrador não encontrado")
	}

	log.Printf("[DEBUG] verificarPermissaoAdmin: Admin encontrado - Nome: %s, Role: %s, Status: %s", 
		admin.Nome, admin.Role, admin.Status)

	if admin.Status != "ativo" {
		log.Printf("[ERROR] verificarPermissaoAdmin: Admin está inativo")
		return fmt.Errorf("administrador está inativo")
	}

	// Hierarquia
	hierarchy := map[string]int{
		"fpp":     3,
		"adm":     2,
		"gerente": 1,
	}

	currentLevel := hierarchy[admin.Role]
	requiredLevel := hierarchy[minRole]

	log.Printf("[DEBUG] verificarPermissaoAdmin: Hierarquia - Current: %d (%s), Required: %d (%s)", 
		currentLevel, admin.Role, requiredLevel, minRole)

	if currentLevel < requiredLevel {
		log.Printf("[ERROR] verificarPermissaoAdmin: Permissão insuficiente")
		return fmt.Errorf("permissão negada: requer role '%s' ou superior", minRole)
	}

	log.Printf("[DEBUG] verificarPermissaoAdmin: Permissão OK")
	return nil
}

// registrarAcaoAdmin registra uma ação administrativa
func registrarAcaoAdmin(c *gin.Context, adminID uuid.UUID, acao string, detalhes map[string]interface{}) {
	log.Printf("[DEBUG] registrarAcaoAdmin: AdminID: %s, Ação: %s", adminID, acao)
	
	repository := getRepository(c)

	adminAgg, err := repository.Load(adminID, "Admin")
	if err != nil {
		log.Printf("[ERROR] registrarAcaoAdmin: Erro ao carregar admin: %v", err)
		return
	}

	admin := adminAgg.(*aggregates.Admin)
	admin.RegistrarAcao(acao, detalhes)
	
	if err := repository.Save(admin); err != nil {
		log.Printf("[ERROR] registrarAcaoAdmin: Erro ao salvar ação: %v", err)
		return
	}
	
	log.Printf("[DEBUG] registrarAcaoAdmin: Ação registrada com sucesso")
}