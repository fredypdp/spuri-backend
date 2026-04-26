package handlers

import (
	"fmt"
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CriarTurma cria uma nova turma para a academia autenticada.
// Rota: POST /academia/turmas
func CriarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoTurma string     `json:"codigo_turma" binding:"required"`
		Nivel       string     `json:"nivel"        binding:"required"`
		CursoID     *uuid.UUID `json:"curso_id"`
		Turno       string     `json:"turno"        binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	existing, _ := turmasProj.GetByCodigoTurma(req.CodigoTurma, academiaDTO.CodigoAcademia)
	if existing != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("já existe uma turma com este código nesta academia"))
		return
	}

	turma := aggregates.NewTurma()
	if err := turma.Criar(
		req.CodigoTurma,
		academiaDTO.CodigoAcademia,
		req.Nivel,
		req.CursoID,
		req.Turno,
		academiaID,
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	repository := getRepository(c)
	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		log.Printf("❌ [CriarTurma] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [CriarTurma] %s criada na academia %s", req.CodigoTurma, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusCreated, gin.H{
		"message":      "turma criada com sucesso",
		"id":           turma.ID,
		"codigo_turma": req.CodigoTurma,
	})
}

// ListarTurmasAcademia lista todas as turmas da academia autenticada.
// Rota: GET /academia/turmas
func ListarTurmasAcademia(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmas, err := turmasProj.ListByAcademia(academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"turmas": turmas})
}

// GetTurmasEstudante retorna as turmas de um estudante com autorização por papel.
// Rota única: GET /turmas-estudante/:codigo
//   - estudante: apenas o próprio código
//   - academia: apenas estudantes da sua academia
//   - admin: qualquer estudante
func GetTurmasEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	turmasProj := getTurmasProjection(c)

	var turmas []*projections.TurmaDTO
	switch userType {
	case "estudante":
		if userID != estudante.ID {
			utils.RespondWithForbiddenError(c, "você só pode visualizar suas próprias turmas")
			return
		}
		turmas, err = turmasProj.ListByEstudante(codigoEstudante, estudante.CodigoAcademia)
	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, errAcademia := academiaProj.GetByID(userID)
		if errAcademia != nil || academiaDTO == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return
		}
		if estudante.CodigoAcademia == nil || *estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
			return
		}
		turmas, err = turmasProj.ListByEstudante(codigoEstudante, &academiaDTO.CodigoAcademia)
	case "admin":
		turmas, err = turmasProj.ListByEstudante(codigoEstudante, nil)
	default:
		utils.RespondWithForbiddenError(c, "tipo de usuário não autorizado")
		return
	}

	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"turmas":           turmas,
		"total":            len(turmas),
	})
}

// GetTurma retorna uma turma pelo código.
// Rota: GET /academia/turmas/:codigo
func GetTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turma, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turma == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	c.JSON(http.StatusOK, turma)
}

// AtivarTurma ativa uma turma inativa da academia.
// Rota: PUT /academia/turmas/:codigo/ativar
func AtivarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	if err := turma.Ativar(academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		log.Printf("❌ [AtivarTurma] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [AtivarTurma] %s ativada pela academia %s", codigoTurma, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":      "turma ativada com sucesso",
		"codigo_turma": codigoTurma,
	})
}

// DesativarTurma desativa uma turma ativa da academia.
// Rota: PUT /academia/turmas/:codigo/desativar
func DesativarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	if err := turma.Desativar(academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		log.Printf("❌ [DesativarTurma] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [DesativarTurma] %s desativada pela academia %s", codigoTurma, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":      "turma desativada com sucesso",
		"codigo_turma": codigoTurma,
	})
}

// validarCompatibilidadeEstudanteTurma verifica se o estudante pode ser vinculado
// à turma com base no nível e curso.
func validarCompatibilidadeEstudanteTurma(
	_ interface{}, // reservado para compatibilidade futura
	_ interface{}, // reservado para compatibilidade futura
	academiaAnosAcademicos []string,
	nivel string,
	turmaCursoID *uuid.UUID,
	anoEscolar *string,
	anoEscolarMedio *string,
	anoSuperior *string,
	cursoMedioID *string,
	cursoSuperiorID *string,
) error {
	tipoEnsino := inferirTipoEnsinoPorNivel(nivel)

	switch tipoEnsino {
	case "fundamental":
		nivelValido := false
		for _, ano := range academiaAnosAcademicos {
			if ano == nivel {
				nivelValido = true
				break
			}
		}
		if !nivelValido {
			return fmt.Errorf(
				"o nível da turma '%s' não está configurado nos anos académicos desta academia",
				nivel,
			)
		}
		if anoEscolar == nil || *anoEscolar == "" {
			return fmt.Errorf(
				"estudante não possui ano escolar fundamental definido — configure o ano escolar antes de vincular à turma '%s'",
				nivel,
			)
		}
		if *anoEscolar != nivel {
			return fmt.Errorf(
				"estudante está no %s mas a turma é do nível %s",
				*anoEscolar, nivel,
			)
		}

	case "medio":
		if anoEscolarMedio == nil || *anoEscolarMedio == "" {
			return fmt.Errorf(
				"estudante não possui ano escolar médio definido — configure antes de vincular à turma '%s'",
				nivel,
			)
		}
		if *anoEscolarMedio != nivel {
			return fmt.Errorf(
				"estudante está no %s mas a turma é do nível %s",
				*anoEscolarMedio, nivel,
			)
		}
		if turmaCursoID == nil {
			return fmt.Errorf(
				"turma de nível médio '%s' não possui curso vinculado",
				nivel,
			)
		}
		if cursoMedioID == nil || *cursoMedioID == "" {
			return fmt.Errorf(
				"estudante não possui curso médio definido — configure antes de vincular à turma '%s'",
				nivel,
			)
		}
		if *cursoMedioID != turmaCursoID.String() {
			return fmt.Errorf(
				"o curso médio do estudante não corresponde ao curso da turma '%s'",
				nivel,
			)
		}

	case "superior":
		if anoSuperior == nil || *anoSuperior == "" {
			return fmt.Errorf(
				"estudante não possui ano superior definido — configure antes de vincular à turma '%s'",
				nivel,
			)
		}
		if *anoSuperior != nivel {
			return fmt.Errorf(
				"estudante está no %s mas a turma é do nível %s",
				*anoSuperior, nivel,
			)
		}
		if turmaCursoID == nil {
			return fmt.Errorf(
				"turma de nível superior '%s' não possui curso vinculado",
				nivel,
			)
		}
		if cursoSuperiorID == nil || *cursoSuperiorID == "" {
			return fmt.Errorf(
				"estudante não possui curso superior definido — configure antes de vincular à turma '%s'",
				nivel,
			)
		}
		if *cursoSuperiorID != turmaCursoID.String() {
			return fmt.Errorf(
				"o curso superior do estudante não corresponde ao curso da turma '%s'",
				nivel,
			)
		}

	default:
		return fmt.Errorf(
			"não foi possível determinar o tipo de ensino para o nível '%s' (use formato como '3_ano_fundamental', '1_ano_medio', '2_ano_superior')",
			nivel,
		)
	}

	return nil
}

type estudanteCompatibilidadeDTO struct {
	Codigo          string
	AnoEscolar      *string
	AnoEscolarMedio *string
	AnoSuperior     *string
	CursoMedioID    *string
	CursoSuperiorID *string
}

func validarCompatibilidadeTurmaComEstudantes(
	academiaAnosAcademicos []string,
	nivel string,
	cursoID *uuid.UUID,
	estudantes []estudanteCompatibilidadeDTO,
) error {
	for _, est := range estudantes {
		if err := validarCompatibilidadeEstudanteTurma(
			nil, nil,
			academiaAnosAcademicos,
			nivel,
			cursoID,
			est.AnoEscolar,
			est.AnoEscolarMedio,
			est.AnoSuperior,
			est.CursoMedioID,
			est.CursoSuperiorID,
		); err != nil {
			return fmt.Errorf(
				"estudante '%s' ficaria incompatível com os novos dados (%w)",
				est.Codigo,
				err,
			)
		}
	}
	return nil
}

// inferirTipoEnsinoPorNivel determina o tipo de ensino com base no formato do campo nivel da turma.
// Retorna "fundamental", "medio", "superior" ou "desconhecido".
func inferirTipoEnsinoPorNivel(nivel string) string {
	if len(nivel) > 16 && nivel[len(nivel)-16:] == "_ano_fundamental" {
		return "fundamental"
	}
	if len(nivel) > 9 && nivel[len(nivel)-9:] == "_ano_medio" {
		return "medio"
	}
	if len(nivel) > 12 && nivel[len(nivel)-12:] == "_ano_superior" {
		return "superior"
	}
	// Fallback para sufixos mais curtos
	for _, suf := range []string{"_ano_fundamental", "_ano_medio", "_ano_superior"} {
		if len(nivel) >= len(suf) && nivel[len(nivel)-len(suf):] == suf {
			switch suf {
			case "_ano_fundamental":
				return "fundamental"
			case "_ano_medio":
				return "medio"
			case "_ano_superior":
				return "superior"
			}
		}
	}
	return "desconhecido"
}

// AdicionarEstudanteATurma adiciona um estudante à turma.
// Rota: POST /academia/turmas/:codigo/estudantes
//
// Validações de compatibilidade adicionadas:
//   - Fundamental: o ano_escolar do estudante deve corresponder ao nivel da turma
//   - Médio: o ano_escolar_medio e o curso_medio_id do estudante devem
//     corresponder ao nivel e curso_id da turma
//   - Superior: o ano_superior e o curso_superior_id do estudante devem
//     corresponder ao nivel e curso_id da turma
func AdicionarEstudanteATurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	var req struct {
		CodigoEstudante string `json:"codigo_estudante" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	// Verifica se o estudante pertence à academia
	estudanteProj := getEstudanteProjection(c)
	estudanteDTO, err := estudanteProj.GetByCodigo(req.CodigoEstudante)
	if err != nil || estudanteDTO == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}
	if estudanteDTO.CodigoAcademia == nil || *estudanteDTO.CodigoAcademia != academiaDTO.CodigoAcademia {
		utils.RespondWithForbiddenError(c, "estudante não pertence a esta academia")
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	// ── Validação de compatibilidade estudante ↔ turma ────────────────────
	if err := validarCompatibilidadeEstudanteTurma(
		nil, nil,
		academiaDTO.AnosAcademicos,
		turmaDTO.Nivel,
		turmaDTO.CursoID,
		estudanteDTO.AnoEscolar,
		estudanteDTO.AnoEscolarMedio,
		estudanteDTO.AnoSuperior,
		estudanteDTO.CursoMedioID,
		estudanteDTO.CursoSuperiorID,
	); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("estudante incompatível com esta turma: %w", err))
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	anoLectivo, err := resolverAnoLetivoAcademia(academiaDTO.AnoLetivo, academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := turma.AdicionarEstudanteNoAnoLectivo(req.CodigoEstudante, anoLectivo, academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "estudante adicionado à turma com sucesso",
		"codigo_turma":     codigoTurma,
		"codigo_estudante": req.CodigoEstudante,
	})
}

// RemoverEstudanteDaTurma remove um estudante da turma.
// Rota: DELETE /academia/turmas/:codigo/estudantes/:codigoEstudante
func RemoverEstudanteDaTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")
	codigoEstudante := c.Param("codigoEstudante")

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	anoLectivo, err := resolverAnoLetivoAcademia(academiaDTO.AnoLetivo, academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if err := turma.RemoverEstudanteNoAnoLectivo(codigoEstudante, anoLectivo, academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":          "estudante removido da turma com sucesso",
		"codigo_turma":     codigoTurma,
		"codigo_estudante": codigoEstudante,
	})
}

// AtualizarTurma atualiza dados da turma.
// Rota: PUT /academia/turmas/:codigo
func AtualizarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	var req struct {
		Nivel   *string    `json:"nivel"`
		CursoID *uuid.UUID `json:"curso_id"`
		Turno   *string    `json:"turno"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	// Se houver alteração de nível e/ou curso, valida todos os estudantes já
	// vinculados antes de persistir a mudança.
	nivelEfetivo := turmaDTO.Nivel
	if req.Nivel != nil {
		nivelEfetivo = *req.Nivel
	}
	cursoIDEfetivo := turmaDTO.CursoID
	if req.CursoID != nil {
		cursoIDEfetivo = req.CursoID
	}

	if (req.Nivel != nil || req.CursoID != nil) && len(turmaDTO.Estudantes) > 0 {
		estudanteProj := getEstudanteProjection(c)
		estudantesParaValidar := make([]estudanteCompatibilidadeDTO, 0, len(turmaDTO.Estudantes))

		for _, codigoEstudante := range turmaDTO.Estudantes {
			estudanteDTO, err := estudanteProj.GetByCodigo(codigoEstudante)
			if err != nil {
				utils.RespondWithInternalError(c, err)
				return
			}
			if estudanteDTO == nil {
				utils.RespondWithValidationError(c, fmt.Errorf(
					"não é possível atualizar a turma: estudante '%s' não foi encontrado para validação de compatibilidade",
					codigoEstudante,
				))
				return
			}

			estudantesParaValidar = append(estudantesParaValidar, estudanteCompatibilidadeDTO{
				Codigo:          codigoEstudante,
				AnoEscolar:      estudanteDTO.AnoEscolar,
				AnoEscolarMedio: estudanteDTO.AnoEscolarMedio,
				AnoSuperior:     estudanteDTO.AnoSuperior,
				CursoMedioID:    estudanteDTO.CursoMedioID,
				CursoSuperiorID: estudanteDTO.CursoSuperiorID,
			})
		}

		if err := validarCompatibilidadeTurmaComEstudantes(
			academiaDTO.AnosAcademicos,
			nivelEfetivo,
			cursoIDEfetivo,
			estudantesParaValidar,
		); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("não é possível atualizar turma: %w", err))
			return
		}
	}

	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	if err := turma.AtualizarDados(req.Nivel, req.CursoID, req.Turno, academiaID); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "turma atualizada com sucesso"})
}

// DeletarTurma remove logicamente uma turma da academia.
// Rota: DELETE /academia/turmas/:codigo
func DeletarTurma(c *gin.Context) {
	academiaID, _ := middleware.GetUserID(c)
	codigoTurma := c.Param("codigo")

	var req struct {
		Motivo string `json:"motivo"`
	}
	_ = c.ShouldBindJSON(&req)

	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(academiaID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turmasProj := getTurmasProjection(c)
	turmaDTO, err := turmasProj.GetByCodigoTurma(codigoTurma, academiaDTO.CodigoAcademia)
	if err != nil || turmaDTO == nil {
		utils.RespondWithNotFoundError(c, "turma")
		return
	}

	repository := getRepository(c)
	agg, err := repository.Load(turmaDTO.ID, "Turma")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	turma, ok := agg.(*aggregates.Turma)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado"))
		return
	}

	if err := turma.Deletar(academiaID, req.Motivo); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   academiaID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(turma, audit); err != nil {
		log.Printf("❌ [DeletarTurma] Erro ao salvar: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	log.Printf("✅ [DeletarTurma] %s deletada pela academia %s", codigoTurma, academiaDTO.CodigoAcademia)

	c.JSON(http.StatusOK, gin.H{
		"message":      "turma deletada com sucesso",
		"codigo_turma": codigoTurma,
		"auditavel":    true,
	})
}

// AtualizarDadosTurma é alias de AtualizarTurma — mantém compatibilidade com main.go.
func AtualizarDadosTurma(c *gin.Context) {
	AtualizarTurma(c)
}
