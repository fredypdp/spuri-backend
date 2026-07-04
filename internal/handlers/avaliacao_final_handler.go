package handlers

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/projections"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ============================================================================
// POST /academia/avaliacao-final
// ============================================================================

func RegistrarAvaliacaoFinal(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)

	var req struct {
		CodigoEstudante     string  `json:"codigo_estudante"          binding:"required"`
		AnoAcademicoAtual   string  `json:"nivel_ano_academico_atual" binding:"required"`
		ProximoAnoAcademico *string `json:"proximo_ano_academico,omitempty"`
		Type                string  `json:"type"`
		Observacao          *string `json:"observacao"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"campos obrigatórios: codigo_estudante, nivel_ano_academico_atual",
		))
		return
	}

	if req.ProximoAnoAcademico != nil {
		utils.RespondWithValidationError(c, fmt.Errorf("proximo_ano_academico é calculado automaticamente pelo backend e não deve ser enviado"))
		return
	}

	// ── Academia ──────────────────────────────────────────────────────────────
	academiaProj := getAcademiaProjection(c)
	academiaDTO, err := academiaProj.GetByID(userID)
	if err != nil || academiaDTO == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// Ano letivo obrigatório — bloqueia registro se a academia não tiver configurado
	anoLectivo, err := resolverAnoLetivoAcademia(academiaDTO.AnoLetivo, academiaDTO.CodigoAcademia)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Estudante ─────────────────────────────────────────────────────────────
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

	// FIX-COMPILE-02: EstudanteDTO armazena CursoMedioID e CursoSuperiorID como
	// *string (banco persiste UUID como texto). Converter para *uuid.UUID para
	// passar para validarNotasParaAprovacao e calcularProximoAnoCurso.
	var cursoMedioUUID, cursoSuperiorUUID *uuid.UUID
	if estudanteDTO.CursoMedioID != nil {
		if parsed, err := uuid.Parse(*estudanteDTO.CursoMedioID); err == nil {
			cursoMedioUUID = &parsed
		}
	}
	if estudanteDTO.CursoSuperiorID != nil {
		if parsed, err := uuid.Parse(*estudanteDTO.CursoSuperiorID); err == nil {
			cursoSuperiorUUID = &parsed
		}
	}

	tipoEnsino := inferirTipoEnsinoDoEstudante(estudanteDTO)
	switch tipoEnsino {
	case "fundamental":
		if err := utils.ValidateAnoFundamental(req.AnoAcademicoAtual); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_ano_academico_atual inválido: %w", err))
			return
		}
	case "medio":
		if err := utils.ValidateAnoMedio(req.AnoAcademicoAtual); err != nil {
			utils.RespondWithValidationError(c, fmt.Errorf("nivel_ano_academico_atual inválido: %w", err))
			return
		}
	case "superior":
		periodoAtual, err := periodoSuperiorAtual(c, estudanteDTO, cursoSuperiorUUID)
		if err != nil {
			utils.RespondWithValidationError(c, err)
			return
		}
		req.AnoAcademicoAtual = periodoAtual
	}

	avaliacaoProj := getAvaliacaoFinalProjection(c)
	if strings.TrimSpace(req.Type) == "" {
		req.Type = "normal"
	}
	regra, err := getRegraAvaliacaoFinal(c, academiaDTO.CodigoAcademia, tipoEnsino, req.AnoAcademicoAtual, req.Type)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	jaAvaliadoNoNivel, err := avaliacaoProj.ExistsByEstudanteAnoLetivoNivelType(
		req.CodigoEstudante,
		academiaDTO.CodigoAcademia,
		anoLectivo,
		tipoEnsino,
		req.AnoAcademicoAtual,
		req.Type,
	)
	if err != nil {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao verificar avaliação final existente no nível: %w", err))
		return
	}
	if jaAvaliadoNoNivel {
		utils.RespondWithValidationError(c, fmt.Errorf(
			"avaliação final já registrada para este estudante no nível %s do ano letivo %s",
			req.AnoAcademicoAtual,
			anoLectivo,
		))
		return
	}

	// O nível informado deve corresponder ao nível atual do estudante para evitar
	// finalizações indevidas quando um nível incorreto é enviado no payload.
	if err := validarNivelAtualDoEstudante(estudanteDTO, tipoEnsino, req.AnoAcademicoAtual); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	if regra.AplicaSeReprovadoEmType != nil {
		prev, err := avaliacaoProj.GetResultadoByType(req.CodigoEstudante, academiaDTO.CodigoAcademia, anoLectivo, tipoEnsino, req.AnoAcademicoAtual, *regra.AplicaSeReprovadoEmType)
		if err != nil || prev == nil || prev.Aprovado {
			utils.RespondWithValidationError(c, fmt.Errorf("avaliação '%s' exige reprovação anterior em '%s'", req.Type, *regra.AplicaSeReprovadoEmType))
			return
		}
	}
	formulaExecucao := regra.Formula
	if tipoEnsino == "superior" {
		formulaExecucao = preencherPeriodoFormulaSuperior(regra.Formula, req.AnoAcademicoAtual)
	}
	notasFormula, err := carregarNotasFormula(c, req.CodigoEstudante, academiaDTO.CodigoAcademia, anoLectivo, regra.CategoriasEnvolvidas)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	notaFinal, err := calcularFormulaAvaliacao(formulaExecucao, notasFormula)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	aprovado := notaFinal >= regra.NotaMinimaAprovacao
	materiasChaveResolvidas, err := resolverMateriasChaveAvaliacaoFinalMedio(c, tipoEnsino, estudanteDTO, req.AnoAcademicoAtual)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Cálculo do próximo nível (backend) ────────────────────────────────────
	var proximoAnoAcademico *string
	var semestreAvaliado, proximoSemestre *int
	var anoSuperiorAntes, anoSuperiorDepois *string
	var motivoProgressao *string
	switch tipoEnsino {
	case "fundamental":
		proximoAnoAcademico, err = calcularProximoAnoFundamental(req.AnoAcademicoAtual, aprovado)
		if err == nil {
			motivoProgressao = motivoProgressaoFundamentalSemOferta(aprovado, proximoAnoAcademico, academiaDTO.AnosAcademicos)
		}
	case "medio":
		proximoAnoAcademico, err = calcularProximoAnoCurso(c, cursoMedioUUID, req.AnoAcademicoAtual, aprovado)
	case "superior":
		proximoSemestre, anoSuperiorDepois, err = calcularProximoSemestreCurso(c, cursoSuperiorUUID, estudanteDTO.SemestreAtual, aprovado)
		semestreAvaliado = estudanteDTO.SemestreAtual
		anoSuperiorAntes = estudanteDTO.AnoSuperior
		proximoAnoAcademico = anoSuperiorDepois
	}
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	// ── Aggregate ─────────────────────────────────────────────────────────────
	repository := getRepository(c)
	estudanteAgg, err := repository.Load(estudanteDTO.ID, "Estudante")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	estudante, ok := estudanteAgg.(*aggregates.Estudante)
	if !ok {
		utils.RespondWithInternalError(c, fmt.Errorf("erro ao converter agregado estudante"))
		return
	}

	// Captura turmas atuais ANTES de registrar avaliação final.
	// A remoção será aplicada de forma determinística na projeção de turmas ao
	// processar este mesmo evento AvaliacaoFinalAnoAcademico.
	turmasAtuais := buscarTurmasDoEstudante(c, req.CodigoEstudante, academiaDTO.CodigoAcademia)
	var turmaAtual *string
	if len(turmasAtuais) > 0 {
		turmaAtual = &turmasAtuais[0]
	}

	if err := estudante.RegistrarAvaliacaoFinal(
		academiaDTO.CodigoAcademia,
		anoLectivo,
		tipoEnsino,
		req.AnoAcademicoAtual,
		proximoAnoAcademico,
		turmaAtual,
		turmasAtuais,
		aprovado,
		req.Observacao,
		req.Type,
		notaFinal,
		regra.NotaMinimaAprovacao,
		&regra.ID,
		formulaExecucao,
		regra.AplicaSeReprovadoEmType,
		cursoSnapshotAvaliacaoFinal(tipoEnsino, cursoMedioUUID, cursoSuperiorUUID),
		materiasChaveResolvidas,
		motivoProgressao,
		nil,
		false,
		nil,
		aggregates.AvaliacaoFinalSuperiorProgressao{
			SemestreAtualAvaliado: semestreAvaliado,
			ProximoSemestreAtual:  proximoSemestre,
			AnoSuperiorAntes:      anoSuperiorAntes,
			AnoSuperiorDepois:     anoSuperiorDepois,
		},
	); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	audit := db.AuditContext{
		UserID:   userID.String(),
		UserType: "academia",
		IP:       c.ClientIP(),
	}
	if err := repository.SaveWithAudit(estudante, audit); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	resultado := "reprovado"
	if aprovado {
		if proximoAnoAcademico != nil {
			resultado = fmt.Sprintf("aprovado → %s", *proximoAnoAcademico)
		} else {
			resultado = "aprovado (ciclo finalizado)"
		}
	}

	response := gin.H{
		"message":               "avaliação final registrada com sucesso",
		"nivel":                 tipoEnsino,
		"type":                  req.Type,
		"nota_final":            notaFinal,
		"nota_minima_aprovacao": regra.NotaMinimaAprovacao,
		"resultado":             resultado,
		"turmas_removidas":      turmasAtuais,
	}
	if motivoProgressao != nil {
		response["motivo_progressao"] = *motivoProgressao
		response["sem_oferta_do_proximo_ano_academico_na_academia"] = true
	}
	c.JSON(http.StatusCreated, response)
}

type notaFormulaOverlay struct {
	MateriaID string
	Categoria string
	Periodo   string
	Nota      float64
}

type resultadoAvaliacaoAutomatica struct {
	Aprovado bool
}

func tentarAvaliacoesFinaisAutomaticas(
	c *gin.Context,
	estudante *aggregates.Estudante,
	estudanteDTO *projections.EstudanteDTO,
	codigoAcademia string,
	anoLectivo string,
	tipoEnsino string,
	anoAcademicoAtual string,
	categoriaAlterada string,
	overlay *notaFormulaOverlay,
) ([]gin.H, error) {
	if tipoEnsino == "superior" {
		var cursoSuperiorUUID *uuid.UUID
		if estudanteDTO.CursoSuperiorID != nil {
			if parsed, err := uuid.Parse(*estudanteDTO.CursoSuperiorID); err == nil {
				cursoSuperiorUUID = &parsed
			}
		}
		periodoAtual, err := periodoSuperiorAtual(c, estudanteDTO, cursoSuperiorUUID)
		if err != nil {
			return nil, err
		}
		anoAcademicoAtual = periodoAtual
	}

	// O gatilho não escolhe a avaliação pela categoria da nota. Ele olha para a
	// cadeia completa aplicável ao estudante e começa pela única regra raiz — a
	// regra sem aplica_se_reprovado_em_type. Assim, quando a última nota necessária
	// de qualquer fórmula chega, o fluxo correto é descoberto pela configuração da
	// academia e não pela ordem/rota usada pelo cliente.
	regras, err := listarRegrasAvaliacaoFinalAplicaveis(c, codigoAcademia, tipoEnsino, anoAcademicoAtual, nil)
	if err != nil {
		return nil, err
	}
	if len(regras) == 0 {
		return nil, nil
	}
	if err := validarCadeiaAvaliacaoFinalAplicavel(regras, codigoAcademia, tipoEnsino, anoAcademicoAtual); err != nil {
		return nil, err
	}
	var raiz *regraAvaliacaoFinalDTO
	for i := range regras {
		if regras[i].AplicaSeReprovadoEmType == nil || strings.TrimSpace(*regras[i].AplicaSeReprovadoEmType) == "" {
			raiz = &regras[i]
			break
		}
	}
	if raiz == nil || raiz.NotaDespertadora == nil || strings.TrimSpace(*raiz.NotaDespertadora) == "" {
		return nil, nil
	}
	if strings.TrimSpace(categoriaAlterada) != strings.TrimSpace(*raiz.NotaDespertadora) {
		return nil, nil
	}

	avaliacaoProj := getAvaliacaoFinalProjection(c)
	resultados := make([]gin.H, 0, len(regras))
	resultadosPorType := map[string]resultadoAvaliacaoAutomatica{}
	processadas := map[string]bool{}

	for {
		avancou := false
		for _, regra := range regras {
			if processadas[regra.Type] {
				continue
			}
			podeExecutar, encerrar, err := regraPodeExecutarAutomaticamente(
				avaliacaoProj,
				estudanteDTO.CodigoEstudante,
				codigoAcademia,
				anoLectivo,
				tipoEnsino,
				anoAcademicoAtual,
				regra,
				resultadosPorType,
			)
			if err != nil {
				return resultados, err
			}
			if encerrar {
				processadas[regra.Type] = true
				avancou = true
				continue
			}
			if !podeExecutar {
				continue
			}

			resultado, registrado, err := executarRegraAvaliacaoFinalAutomatica(
				c,
				estudante,
				estudanteDTO,
				codigoAcademia,
				anoLectivo,
				tipoEnsino,
				anoAcademicoAtual,
				regra,
				overlay,
			)
			if err != nil {
				if strings.Contains(err.Error(), "nota ausente") {
					continue
				}
				return resultados, err
			}
			if !registrado {
				continue
			}

			resultados = append(resultados, resultado)
			if aprovado, ok := resultado["aprovado"].(bool); ok {
				resultadosPorType[regra.Type] = resultadoAvaliacaoAutomatica{Aprovado: aprovado}
			}
			processadas[regra.Type] = true
			avancou = true
		}
		if !avancou {
			break
		}
	}

	return resultados, nil
}

func regraPodeExecutarAutomaticamente(
	avaliacaoProj *projections.AvaliacaoFinalProjection,
	codigoEstudante string,
	codigoAcademia string,
	anoLectivo string,
	tipoEnsino string,
	anoAcademicoAtual string,
	regra regraAvaliacaoFinalDTO,
	resultadosPorType map[string]resultadoAvaliacaoAutomatica,
) (podeExecutar bool, encerrar bool, err error) {
	jaAvaliado, err := avaliacaoProj.ExistsByEstudanteAnoLetivoNivelType(
		codigoEstudante,
		codigoAcademia,
		anoLectivo,
		tipoEnsino,
		anoAcademicoAtual,
		regra.Type,
	)
	if err != nil {
		return false, false, fmt.Errorf("erro ao verificar avaliação final existente: %w", err)
	}
	if jaAvaliado {
		return false, true, nil
	}

	if regra.AplicaSeReprovadoEmType == nil || strings.TrimSpace(*regra.AplicaSeReprovadoEmType) == "" {
		return true, false, nil
	}

	dependencia := strings.TrimSpace(*regra.AplicaSeReprovadoEmType)
	if resultado, ok := resultadosPorType[dependencia]; ok {
		if resultado.Aprovado {
			return false, true, nil
		}
		return true, false, nil
	}

	prev, err := avaliacaoProj.GetResultadoByType(codigoEstudante, codigoAcademia, anoLectivo, tipoEnsino, anoAcademicoAtual, dependencia)
	if err != nil || prev == nil {
		return false, false, nil
	}
	if prev.Aprovado {
		return false, true, nil
	}
	return true, false, nil
}

func executarRegraAvaliacaoFinalAutomatica(
	c *gin.Context,
	estudante *aggregates.Estudante,
	estudanteDTO *projections.EstudanteDTO,
	codigoAcademia string,
	anoLectivo string,
	tipoEnsino string,
	anoAcademicoAtual string,
	regra regraAvaliacaoFinalDTO,
	overlay *notaFormulaOverlay,
) (gin.H, bool, error) {
	formulaExecucao := regra.Formula
	materiasAplicaveis, err := materiasAplicaveisAvaliacaoFinal(c, codigoAcademia, tipoEnsino, anoAcademicoAtual, regra, estudanteDTO)
	if err != nil {
		return nil, false, err
	}
	if len(materiasAplicaveis) == 0 {
		return nil, false, fmt.Errorf("nenhuma matéria aplicável encontrada para avaliação final")
	}
	materiasChaveResolvidas, err := resolverMateriasChaveAvaliacaoFinalMedio(c, tipoEnsino, estudanteDTO, anoAcademicoAtual)
	if err != nil {
		return nil, false, err
	}
	resultadosMaterias, notaFinal, aprovado, aprovadoComPendencia, pendenciasGeradas, err := calcularResultadoMateriasAvaliacaoFinal(
		c, estudanteDTO.CodigoEstudante, codigoAcademia, anoLectivo, tipoEnsino, anoAcademicoAtual, regra, materiasAplicaveis, materiasChaveResolvidas, overlay,
	)
	if err != nil {
		return nil, false, err
	}

	var cursoMedioUUID, cursoSuperiorUUID *uuid.UUID
	if estudanteDTO.CursoMedioID != nil {
		if parsed, err := uuid.Parse(*estudanteDTO.CursoMedioID); err == nil {
			cursoMedioUUID = &parsed
		}
	}
	if estudanteDTO.CursoSuperiorID != nil {
		if parsed, err := uuid.Parse(*estudanteDTO.CursoSuperiorID); err == nil {
			cursoSuperiorUUID = &parsed
		}
	}

	if err := validarNivelAtualDoEstudante(estudanteDTO, tipoEnsino, anoAcademicoAtual); err != nil {
		return nil, false, err
	}

	var proximoAnoAcademico *string
	var semestreAvaliado, proximoSemestre *int
	var anoSuperiorAntes, anoSuperiorDepois *string
	var motivoProgressao *string
	switch tipoEnsino {
	case "fundamental":
		proximoAnoAcademico, err = calcularProximoAnoFundamental(anoAcademicoAtual, aprovado)
		if err == nil {
			academiaDTO, academiaErr := getAcademiaProjection(c).GetByCodigo(codigoAcademia)
			if academiaErr != nil {
				err = academiaErr
			} else if academiaDTO != nil {
				motivoProgressao = motivoProgressaoFundamentalSemOferta(aprovado, proximoAnoAcademico, academiaDTO.AnosAcademicos)
			}
		}
	case "medio":
		proximoAnoAcademico, err = calcularProximoAnoCurso(c, cursoMedioUUID, anoAcademicoAtual, aprovado)
	case "superior":
		proximoSemestre, anoSuperiorDepois, err = calcularProximoSemestreCurso(c, cursoSuperiorUUID, estudanteDTO.SemestreAtual, aprovado)
		semestreAvaliado = estudanteDTO.SemestreAtual
		anoSuperiorAntes = estudanteDTO.AnoSuperior
		proximoAnoAcademico = anoSuperiorDepois
	}
	if err != nil {
		return nil, false, err
	}

	turmasAtuais := buscarTurmasDoEstudante(c, estudanteDTO.CodigoEstudante, codigoAcademia)
	var turmaAtual *string
	if len(turmasAtuais) > 0 {
		turmaAtual = &turmasAtuais[0]
	}

	if err := estudante.RegistrarAvaliacaoFinal(
		codigoAcademia,
		anoLectivo,
		tipoEnsino,
		anoAcademicoAtual,
		proximoAnoAcademico,
		turmaAtual,
		turmasAtuais,
		aprovado,
		nil,
		regra.Type,
		notaFinal,
		regra.NotaMinimaAprovacao,
		&regra.ID,
		formulaExecucao,
		regra.AplicaSeReprovadoEmType,
		cursoSnapshotAvaliacaoFinal(tipoEnsino, cursoMedioUUID, cursoSuperiorUUID),
		materiasChaveResolvidas,
		motivoProgressao,
		resultadosMaterias,
		aprovadoComPendencia,
		pendenciasGeradas,
		aggregates.AvaliacaoFinalSuperiorProgressao{
			SemestreAtualAvaliado: semestreAvaliado,
			ProximoSemestreAtual:  proximoSemestre,
			AnoSuperiorAntes:      anoSuperiorAntes,
			AnoSuperiorDepois:     anoSuperiorDepois,
		},
	); err != nil {
		if strings.Contains(err.Error(), "já registrada") {
			return nil, false, nil
		}
		return nil, false, err
	}

	resultado := gin.H{
		"type":                   regra.Type,
		"aprovado":               aprovado,
		"nota_final":             notaFinal,
		"nota_minima_aprovacao":  regra.NotaMinimaAprovacao,
		"proximo_ano_academico":  proximoAnoAcademico,
		"resultados_materias":    resultadosMaterias,
		"aprovado_com_pendencia": aprovadoComPendencia,
		"pendencias_geradas":     pendenciasGeradas,
		"materias_chave":         uuidStrings(materiasChaveResolvidas),
	}
	if motivoProgressao != nil {
		resultado["motivo_progressao"] = *motivoProgressao
		resultado["sem_oferta_do_proximo_ano_academico_na_academia"] = true
	}
	return resultado, true, nil
}

func cursoSnapshotAvaliacaoFinal(tipoEnsino string, cursoMedioUUID, cursoSuperiorUUID *uuid.UUID) *uuid.UUID {
	if tipoEnsino == "medio" {
		return cursoMedioUUID
	}
	if tipoEnsino == "superior" {
		return cursoSuperiorUUID
	}
	return nil
}

func resolverMateriasChaveAvaliacaoFinalMedio(c *gin.Context, tipoEnsino string, estudanteDTO *projections.EstudanteDTO, anoAcademicoAtual string) ([]uuid.UUID, error) {
	if tipoEnsino != "medio" {
		return nil, nil
	}
	if estudanteDTO.CursoMedioID == nil || *estudanteDTO.CursoMedioID == "" {
		return nil, fmt.Errorf("não foi possível resolver matérias-chave: estudante sem curso_medio_id")
	}
	cursoID, err := uuid.Parse(*estudanteDTO.CursoMedioID)
	if err != nil {
		return nil, fmt.Errorf("curso_medio_id inválido para resolver matérias-chave")
	}
	curso, err := getCursosProjection(c).GetByID(cursoID)
	if err != nil {
		return nil, err
	}
	if curso == nil || curso.Type != "medio" {
		return nil, fmt.Errorf("curso médio não encontrado para resolver matérias-chave")
	}
	for _, cfg := range curso.MateriasChave {
		if cfg.AnoAcademico == anoAcademicoAtual {
			if len(cfg.MateriasChave) == 0 {
				return nil, fmt.Errorf("curso médio %s não possui matérias-chave configuradas para o ano_academico %s", cursoID, anoAcademicoAtual)
			}
			return cfg.MateriasChave, nil
		}
	}
	return nil, fmt.Errorf("curso médio %s não possui configuração de materias_chave para o ano_academico %s", cursoID, anoAcademicoAtual)
}

func materiasAplicaveisAvaliacaoFinal(c *gin.Context, codigoAcademia, tipoEnsino, anoAcademicoAtual string, regra regraAvaliacaoFinalDTO, estudanteDTO *projections.EstudanteDTO) ([]projections.MateriaDTO, error) {
	todas, err := getMateriasProjection(c).GetByAcademia(codigoAcademia)
	if err != nil {
		return nil, err
	}
	idsFiltro := map[string]bool{}
	for _, id := range regra.MateriasAplicaveis {
		idsFiltro[id.String()] = true
	}
	var cursoID *string
	if tipoEnsino == "medio" {
		cursoID = estudanteDTO.CursoMedioID
	} else if tipoEnsino == "superior" {
		cursoID = estudanteDTO.CursoSuperiorID
	}
	out := []projections.MateriaDTO{}
	for _, m := range todas {
		if m.Status != "ativo" || m.Type != tipoEnsino {
			continue
		}
		if len(idsFiltro) > 0 && !idsFiltro[m.ID.String()] {
			continue
		}
		if tipoEnsino == "fundamental" {
			if containsString(m.AnosAcademicos, anoAcademicoAtual) {
				out = append(out, m)
			}
			continue
		}
		if cursoID == nil || m.CursoID == nil || m.CursoID.String() != *cursoID {
			continue
		}
		if tipoEnsino == "medio" && containsString(m.AnosAcademicos, anoAcademicoAtual) {
			out = append(out, m)
		}
		if tipoEnsino == "superior" && m.Periodo != nil && *m.Periodo == anoAcademicoAtual {
			out = append(out, m)
		}
	}
	return out, nil
}

func calcularResultadoMateriasAvaliacaoFinal(
	c *gin.Context,
	codigoEstudante, codigoAcademia, anoLectivo, tipoEnsino, anoAcademicoAtual string,
	regra regraAvaliacaoFinalDTO,
	materias []projections.MateriaDTO,
	materiasChaveResolvidas []uuid.UUID,
	overlay *notaFormulaOverlay,
) ([]aggregates.ResultadoMateriaAvaliacaoFinal, float64, bool, bool, []aggregates.MateriaPendenteGerada, error) {
	resultados := make([]aggregates.ResultadoMateriaAvaliacaoFinal, 0, len(materias))
	var soma float64
	reprovadasPendenciaveis := []projections.MateriaDTO{}
	for _, materia := range materias {
		formulaExecucao := regra.Formula
		if tipoEnsino == "superior" {
			if materia.Periodo == nil || *materia.Periodo == "" {
				return nil, 0, false, false, nil, fmt.Errorf("matéria superior %s sem período definido", materia.ID)
			}
			formulaExecucao = preencherPeriodoFormulaSuperior(regra.Formula, *materia.Periodo)
		}
		notasFormula, err := carregarNotasFormulaMateria(c, codigoEstudante, codigoAcademia, anoLectivo, materia.ID, regra.CategoriasEnvolvidas)
		if err != nil {
			return nil, 0, false, false, nil, err
		}
		if overlay != nil && overlay.MateriaID == materia.ID.String() {
			if notasFormula[overlay.Categoria] == nil {
				notasFormula[overlay.Categoria] = map[string][]float64{}
			}
			notasFormula[overlay.Categoria][overlay.Periodo] = append(notasFormula[overlay.Categoria][overlay.Periodo], overlay.Nota)
		}
		nota, err := calcularFormulaAvaliacao(formulaExecucao, notasFormula)
		if err != nil {
			return nil, 0, false, false, nil, fmt.Errorf("matéria %s: %w", materia.ID, err)
		}
		aprovada := nota >= regra.NotaMinimaAprovacao
		if !aprovada && materia.PendenciaPermitida {
			reprovadasPendenciaveis = append(reprovadasPendenciaveis, materia)
		}
		soma += nota
		resultados = append(resultados, aggregates.ResultadoMateriaAvaliacaoFinal{
			MateriaID:             materia.ID,
			NotaFinal:             nota,
			Aprovado:              aprovada,
			RegraAvaliacaoFinalID: &regra.ID,
			Type:                  regra.Type,
			FormulaSnapshot:       formulaExecucao,
			PendenciaPermitida:    materia.PendenciaPermitida,
		})
	}
	aprovado := true
	reprovadas := 0
	materiasChave := map[uuid.UUID]bool{}
	if tipoEnsino == "medio" && regra.AplicaSeReprovadoEmType == nil {
		for _, id := range materiasChaveResolvidas {
			materiasChave[id] = true
		}
	}
	for _, r := range resultados {
		if !r.Aprovado {
			reprovadas++
			if tipoEnsino != "medio" || regra.AplicaSeReprovadoEmType != nil || materiasChave[r.MateriaID] {
				aprovado = false
			}
		}
	}
	aprovadoComPendencia := false
	pendencias := []aggregates.MateriaPendenteGerada{}
	if !aprovado && tipoEnsino != "fundamental" && regra.LimiteMateriasPendentes != nil && reprovadas <= *regra.LimiteMateriasPendentes && reprovadas == len(reprovadasPendenciaveis) {
		aprovado = true
		aprovadoComPendencia = true
		for _, m := range reprovadasPendenciaveis {
			if m.CursoID != nil {
				pendencias = append(pendencias, aggregates.MateriaPendenteGerada{MateriaID: m.ID, CursoID: *m.CursoID, Nivel: tipoEnsino, Escopo: anoAcademicoAtual})
			}
		}
	}
	return resultados, soma / float64(len(resultados)), aprovado, aprovadoComPendencia, pendencias, nil
}

func inferirTipoEnsinoDoEstudante(estudante *projections.EstudanteDTO) string {
	if estudante == nil {
		return "fundamental"
	}
	if estudante.CursoSuperiorID != nil || estudante.AnoSuperior != nil || estudante.StatusSuperior == "em_andamento" {
		return "superior"
	}
	if estudante.CursoMedioID != nil || estudante.AnoEscolarMedio != nil || estudante.StatusEscolarMedio == "em_andamento" {
		return "medio"
	}
	return "fundamental"
}

// ============================================================================
// GET /avaliacoes
// Estudante → suas avaliações
// Academia  → todas da academia (?nivel=fundamental|medio|superior)
// Admin     → todas do sistema  (?nivel=...)
// ============================================================================

func ListarAvaliacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	avaliacaoProj := getAvaliacaoFinalProjection(c)
	filtros, err := parseFiltrosAvaliacaoFinal(c)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}

	switch userType {
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		avaliacoes, err := avaliacaoProj.GetByEstudante(estudante.CodigoEstudante)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		avaliacoes = filtrarAvaliacoesMemoria(avaliacoes, filtros)
		c.JSON(http.StatusOK, gin.H{"avaliacoes": avaliacoes, "total": len(avaliacoes)})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if filtros.CodigoAcademia != nil && *filtros.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "academia só pode consultar os próprios dados")
			return
		}
		filtros.CodigoAcademia = &academiaDTO.CodigoAcademia
		avaliacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, nil, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"avaliacoes": avaliacoes, "total": len(avaliacoes)})

	default: // admin
		if filtros.CodigoTurma != nil && filtros.CodigoAcademia == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("filtro codigo_turma exige codigo_academia para consultas admin"))
			return
		}
		avaliacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, nil, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"avaliacoes": avaliacoes, "total": len(avaliacoes)})
	}
}

// ============================================================================
// GET /avaliacoes-estudante/:codigo
// Apenas academia e admin (middleware.RequireAcademiaOuAdmin aplicado na rota).
// Academia → verifica se o estudante pertence à academia antes de retornar.
// Admin    → acesso irrestrito.
// ============================================================================

func GetAvaliacoesFinaisEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	if userType == "academia" {
		academiaProj := getAcademiaProjection(c)
		academiaDTO, _ := academiaProj.GetByID(userID)
		if estudante.CodigoAcademia == nil || academiaDTO == nil ||
			*estudante.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "Estudante não pertence a esta academia")
			return
		}
	}

	avaliacaoProj := getAvaliacaoFinalProjection(c)
	avaliacoes, err := avaliacaoProj.GetByEstudante(codigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"nome":             estudante.Nome,
		"avaliacoes":       avaliacoes,
		"total":            len(avaliacoes),
	})
}

// ============================================================================
// GET /aprovacoes
// Estudante → suas próprias aprovações
// Academia  → aprovações dos estudantes da academia
// Admin     → todas as aprovações do sistema
// ============================================================================

func ListarAprovacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	avaliacaoProj := getAvaliacaoFinalProjection(c)
	filtros, err := parseFiltrosAvaliacaoFinal(c)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	aprovado := true

	switch userType {
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		aprovacoes, err := avaliacaoProj.GetAprovacoesByEstudante(estudante.CodigoEstudante)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		aprovacoes = filtrarAvaliacoesMemoria(aprovacoes, filtros)
		c.JSON(http.StatusOK, gin.H{"aprovacoes": aprovacoes, "total": len(aprovacoes)})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if filtros.CodigoAcademia != nil && *filtros.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "academia só pode consultar os próprios dados")
			return
		}
		filtros.CodigoAcademia = &academiaDTO.CodigoAcademia
		aprovacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, &aprovado, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"aprovacoes": aprovacoes, "total": len(aprovacoes)})

	default: // admin
		if filtros.CodigoTurma != nil && filtros.CodigoAcademia == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("filtro codigo_turma exige codigo_academia para consultas admin"))
			return
		}
		aprovacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, &aprovado, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"aprovacoes": aprovacoes, "total": len(aprovacoes)})
	}
}

// ============================================================================
// GET /reprovacoes
// ============================================================================

func ListarReprovacoes(c *gin.Context) {
	userID, _ := middleware.GetUserID(c)
	userType, _ := middleware.GetUserType(c)
	avaliacaoProj := getAvaliacaoFinalProjection(c)
	filtros, err := parseFiltrosAvaliacaoFinal(c)
	if err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	aprovado := false

	switch userType {
	case "estudante":
		estudanteProj := getEstudanteProjection(c)
		estudante, err := estudanteProj.GetByID(userID)
		if err != nil || estudante == nil {
			utils.RespondWithNotFoundError(c, "estudante")
			return
		}
		reprovacoes, err := avaliacaoProj.GetReprovacoesByEstudante(estudante.CodigoEstudante)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		reprovacoes = filtrarAvaliacoesMemoria(reprovacoes, filtros)
		c.JSON(http.StatusOK, gin.H{"reprovacoes": reprovacoes, "total": len(reprovacoes)})

	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if filtros.CodigoAcademia != nil && *filtros.CodigoAcademia != academiaDTO.CodigoAcademia {
			utils.RespondWithForbiddenError(c, "academia só pode consultar os próprios dados")
			return
		}
		filtros.CodigoAcademia = &academiaDTO.CodigoAcademia
		reprovacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, &aprovado, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"reprovacoes": reprovacoes, "total": len(reprovacoes)})

	default: // admin
		if filtros.CodigoTurma != nil && filtros.CodigoAcademia == nil {
			utils.RespondWithValidationError(c, fmt.Errorf("filtro codigo_turma exige codigo_academia para consultas admin"))
			return
		}
		reprovacoes, err := avaliacaoProj.ListByFilters(filtros.CodigoAcademia, &aprovado, filtros.toProjectionFilters())
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"reprovacoes": reprovacoes, "total": len(reprovacoes)})
	}
}

type filtrosAvaliacaoFinal struct {
	Nivel             *string
	AnoLectivo        *string
	AnoAcademicoAtual *string
	CodigoTurma       *string
	CodigoAcademia    *string
	Type              *string
}

func parseFiltrosAvaliacaoFinal(c *gin.Context) (filtrosAvaliacaoFinal, error) {
	parse := func(name string) *string {
		value := strings.TrimSpace(c.Query(name))
		if value == "" {
			return nil
		}
		return &value
	}
	f := filtrosAvaliacaoFinal{
		Nivel:             parse("nivel"),
		AnoLectivo:        parse("ano_letivo"),
		AnoAcademicoAtual: parse("ano_academico_atual"),
		CodigoTurma:       parse("codigo_turma"),
		CodigoAcademia:    parse("codigo_academia"),
		Type:              parse("type"),
	}
	if parse("tipo_ensino") != nil {
		return f, fmt.Errorf("campo legado tipo_ensino não é aceito; use nivel")
	}
	if f.Nivel != nil {
		switch *f.Nivel {
		case "fundamental", "medio", "superior":
		default:
			return f, fmt.Errorf("nivel deve ser: fundamental, medio ou superior")
		}
	}
	return f, nil
}

func (f filtrosAvaliacaoFinal) toProjectionFilters() projections.AvaliacaoFinalFilters {
	return projections.AvaliacaoFinalFilters{
		TipoEnsino:        f.Nivel,
		AnoLectivo:        f.AnoLectivo,
		AnoAcademicoAtual: f.AnoAcademicoAtual,
		CodigoTurma:       f.CodigoTurma,
		Type:              f.Type,
	}
}

func filtrarAvaliacoesMemoria(in []projections.AvaliacaoFinalDTO, f filtrosAvaliacaoFinal) []projections.AvaliacaoFinalDTO {
	out := make([]projections.AvaliacaoFinalDTO, 0, len(in))
	for _, a := range in {
		if f.Nivel != nil && a.TipoEnsino != *f.Nivel {
			continue
		}
		if f.AnoLectivo != nil && a.AnoLectivo != *f.AnoLectivo {
			continue
		}
		if f.AnoAcademicoAtual != nil && a.AnoAcademicoAtual != *f.AnoAcademicoAtual {
			continue
		}
		if f.Type != nil && a.Type != *f.Type {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ============================================================================
// Helpers internos
// ============================================================================

// buscarTurmasDoEstudante retorna todas as turmas atuais do estudante.
func buscarTurmasDoEstudante(c *gin.Context, codigoEstudante, codigoAcademia string) []string {
	turmasProj := getTurmasProjection(c)
	turmas, err := turmasProj.ListByAcademia(codigoAcademia)
	if err != nil {
		log.Printf("[avaliacao-final] erro ao buscar turma atual do estudante %s: %v", codigoEstudante, err)
		return nil
	}
	result := make([]string, 0, 2)
	for _, turma := range turmas {
		for _, cod := range turma.Estudantes {
			if cod == codigoEstudante {
				result = append(result, turma.CodigoTurma)
				break
			}
		}
	}
	return result
}

// validarNotasParaAprovacao verifica se todas as notas obrigatórias estão presentes.
func validarNotasParaAprovacao(
	c *gin.Context,
	codigoEstudante string,
	anoLectivo string,
	tipoEnsino string,
	anoAcademicoAtual string,
	codigoAcademia string,
	cursoMedioID *uuid.UUID,
	cursoSuperiorID *uuid.UUID,
) error {
	materiasProj := getMateriasProjection(c)
	notasProj := getNotasProjection(c)

	todasMaterias, err := materiasProj.GetByAcademia(codigoAcademia)
	if err != nil {
		return fmt.Errorf("erro ao carregar matérias: %w", err)
	}

	var materiasFiltradas []projections.MateriaDTO
	var periodosEsperados []string
	var categoriaEsperada string

	switch tipoEnsino {
	case "fundamental":
		for _, m := range todasMaterias {
			if m.Type != "fundamental" {
				continue
			}
			for _, a := range m.AnosAcademicos {
				if a == anoAcademicoAtual {
					materiasFiltradas = append(materiasFiltradas, m)
					break
				}
			}
		}
		periodosEsperados = []string{"1_trimestre", "2_trimestre", "3_trimestre"}
		categoriaEsperada = "nota_escola"

	case "medio":
		if cursoMedioID == nil {
			return fmt.Errorf("estudante não possui curso médio vinculado")
		}
		cursosProj := getCursosProjection(c)
		cursoDTO, err := cursosProj.GetByID(*cursoMedioID)
		if err != nil || cursoDTO == nil {
			return fmt.Errorf("curso médio não encontrado")
		}
		if len(cursoDTO.Periodos) > 0 {
			periodosEsperados = cursoDTO.Periodos
		} else {
			periodosEsperados = []string{"1_trimestre", "2_trimestre", "3_trimestre"}
		}
		for _, m := range todasMaterias {
			if m.Type != "medio" || m.CursoID == nil || *m.CursoID != *cursoMedioID {
				continue
			}
			for _, a := range m.AnosAcademicos {
				if a == anoAcademicoAtual {
					materiasFiltradas = append(materiasFiltradas, m)
					break
				}
			}
		}
		categoriaEsperada = "nota_escola"

	case "superior":
		if cursoSuperiorID == nil {
			return fmt.Errorf("estudante não possui curso superior vinculado")
		}
		cursosProj := getCursosProjection(c)
		cursoDTO, err := cursosProj.GetByID(*cursoSuperiorID)
		if err != nil || cursoDTO == nil {
			return fmt.Errorf("curso superior não encontrado")
		}
		if len(cursoDTO.Periodos) == 0 {
			return fmt.Errorf("curso superior não possui períodos configurados")
		}
		for _, m := range todasMaterias {
			if m.Type != "superior" || m.CursoID == nil || *m.CursoID != *cursoSuperiorID {
				continue
			}
			if m.Periodo == nil || *m.Periodo == "" {
				log.Printf("[avaliacao-final] matéria '%s' sem periodo definido — ignorada na validação", m.Nome)
				continue
			}
			for _, a := range m.AnosAcademicos {
				if a == anoAcademicoAtual {
					materiasFiltradas = append(materiasFiltradas, m)
					break
				}
			}
		}
		categoriaEsperada = "nota_exame"
	}

	if len(materiasFiltradas) == 0 {
		return nil
	}

	todasNotas, err := notasProj.GetByEstudante(codigoEstudante)
	if err != nil {
		return fmt.Errorf("erro ao carregar notas: %w", err)
	}

	type notaKey struct{ materiaID, periodo, categoria string }
	notasExistentes := make(map[notaKey]bool)
	for _, n := range todasNotas {
		if n.AnoLectivo == anoLectivo && n.Categoria == categoriaEsperada {
			notasExistentes[notaKey{n.MateriaDisciplinarID, n.Periodo, n.Categoria}] = true
		}
	}

	var faltando []string
	if tipoEnsino == "superior" {
		for _, materia := range materiasFiltradas {
			periodo := *materia.Periodo
			key := notaKey{materia.ID.String(), periodo, categoriaEsperada}
			if !notasExistentes[key] {
				faltando = append(faltando, fmt.Sprintf("matéria '%s' — %s", materia.Nome, periodo))
			}
		}
	} else {
		for _, materia := range materiasFiltradas {
			for _, periodo := range periodosEsperados {
				key := notaKey{materia.ID.String(), periodo, categoriaEsperada}
				if !notasExistentes[key] {
					faltando = append(faltando, fmt.Sprintf("matéria '%s' — %s", materia.Nome, periodo))
				}
			}
		}
	}

	if len(faltando) > 0 {
		return fmt.Errorf(
			"notas de '%s' ausentes: %s. Preencha 'observacao' para forçar aprovação",
			categoriaEsperada,
			strings.Join(faltando, "; "),
		)
	}

	return nil
}

// calcularProximoAnoFundamental calcula o próximo ano na sequência fixa
// 1_ano_fundamental..9_ano_fundamental.
func calcularProximoAnoFundamental(
	nivelAtual string,
	aprovado bool,
) (*string, error) {
	sequenciaFundamental := []string{
		"1_ano_fundamental",
		"2_ano_fundamental",
		"3_ano_fundamental",
		"4_ano_fundamental",
		"5_ano_fundamental",
		"6_ano_fundamental",
		"7_ano_fundamental",
		"8_ano_fundamental",
		"9_ano_fundamental",
	}

	posAtual := -1
	for i, ano := range sequenciaFundamental {
		if ano == nivelAtual {
			posAtual = i
			break
		}
	}
	if posAtual == -1 {
		return nil, fmt.Errorf("nivel_atual '%s' não pertence à sequência fundamental (1_ano_fundamental..9_ano_fundamental)", nivelAtual)
	}

	if !aprovado {
		return nil, nil
	}

	if posAtual == len(sequenciaFundamental)-1 {
		return nil, nil
	}

	proximo := sequenciaFundamental[posAtual+1]
	return &proximo, nil
}

// calcularProximoAnoCurso calcula o próximo ano com base na sequência do curso (médio/superior).

const motivoAcademiaSemOfertaProximoAnoFundamental = "academia_sem_oferta_do_proximo_ano_academico_fundamental"

func motivoProgressaoFundamentalSemOferta(aprovado bool, proximoAnoAcademico *string, anosAcademicosAcademia []string) *string {
	if !aprovado || proximoAnoAcademico == nil {
		return nil
	}
	for _, ano := range anosAcademicosAcademia {
		if strings.TrimSpace(ano) == *proximoAnoAcademico {
			return nil
		}
	}
	motivo := motivoAcademiaSemOfertaProximoAnoFundamental
	return &motivo
}

func calcularProximoAnoCurso(
	c *gin.Context,
	cursoID *uuid.UUID,
	nivelAtual string,
	aprovado bool,
) (*string, error) {
	if cursoID == nil {
		return nil, fmt.Errorf("estudante não possui curso vinculado para este tipo de ensino")
	}

	cursosProj := getCursosProjection(c)
	curso, err := cursosProj.GetByID(*cursoID)
	if err != nil || curso == nil {
		return nil, fmt.Errorf("curso do estudante não encontrado")
	}

	if curso.Status != "ativo" {
		return nil, fmt.Errorf("curso do estudante está inativo")
	}

	if len(curso.AnosAcademicos) == 0 {
		return nil, fmt.Errorf("curso '%s' não possui anos_academicos definidos", curso.Nome)
	}

	posAtual := -1
	for i, n := range curso.AnosAcademicos {
		if n == nivelAtual {
			posAtual = i
			break
		}
	}
	if posAtual == -1 {
		return nil, fmt.Errorf("nivel_atual '%s' não pertence ao curso '%s'", nivelAtual, curso.Nome)
	}

	if !aprovado {
		return nil, nil
	}

	if posAtual == len(curso.AnosAcademicos)-1 {
		return nil, nil
	}

	proximo := curso.AnosAcademicos[posAtual+1]
	return &proximo, nil
}

func periodoSuperiorAtual(c *gin.Context, estudante *projections.EstudanteDTO, cursoID *uuid.UUID) (string, error) {
	if estudante == nil || estudante.SemestreAtual == nil || *estudante.SemestreAtual < 1 {
		return "", fmt.Errorf("estudante superior não possui semestre_atual válido")
	}
	periodo := fmt.Sprintf("%d_semestre", *estudante.SemestreAtual)
	if cursoID == nil {
		return "", fmt.Errorf("estudante não possui curso superior vinculado")
	}
	curso, err := getCursosProjection(c).GetByID(*cursoID)
	if err != nil || curso == nil {
		return "", fmt.Errorf("curso superior não encontrado")
	}
	if curso.Status != "ativo" {
		return "", fmt.Errorf("curso superior do estudante está inativo")
	}
	for _, p := range curso.Periodos {
		if p == periodo {
			return periodo, nil
		}
	}
	return "", fmt.Errorf("semestre_atual %d não existe nos periodos do curso superior '%s'", *estudante.SemestreAtual, curso.Nome)
}

func calcularAnoSuperiorPorSemestre(semestre int) string {
	ano := int(math.Ceil(float64(semestre) / 2.0))
	return fmt.Sprintf("%d_ano_superior", ano)
}

func calcularProximoSemestreCurso(c *gin.Context, cursoID *uuid.UUID, semestreAtual *int, aprovado bool) (*int, *string, error) {
	if semestreAtual == nil || *semestreAtual < 1 {
		return nil, nil, fmt.Errorf("estudante superior não possui semestre_atual válido")
	}
	if cursoID == nil {
		return nil, nil, fmt.Errorf("estudante não possui curso superior vinculado")
	}
	curso, err := getCursosProjection(c).GetByID(*cursoID)
	if err != nil || curso == nil {
		return nil, nil, fmt.Errorf("curso superior não encontrado")
	}
	periodoAtual := fmt.Sprintf("%d_semestre", *semestreAtual)
	pos := -1
	for i, p := range curso.Periodos {
		if p == periodoAtual {
			pos = i
			break
		}
	}
	if pos == -1 {
		return nil, nil, fmt.Errorf("semestre_atual %d não pertence ao curso '%s'", *semestreAtual, curso.Nome)
	}
	anoAtual := calcularAnoSuperiorPorSemestre(*semestreAtual)
	if !aprovado || pos == len(curso.Periodos)-1 {
		return nil, &anoAtual, nil
	}
	prox := *semestreAtual + 1
	anoDepois := calcularAnoSuperiorPorSemestre(prox)
	return &prox, &anoDepois, nil
}

func validarNivelAtualDoEstudante(estudante *projections.EstudanteDTO, tipoEnsino, nivelInformado string) error {
	if estudante == nil {
		return fmt.Errorf("estudante inválido")
	}

	var nivelAtual *string
	switch tipoEnsino {
	case "fundamental":
		nivelAtual = estudante.AnoEscolar
	case "medio":
		nivelAtual = estudante.AnoEscolarMedio
	case "superior":
		if estudante.SemestreAtual == nil || *estudante.SemestreAtual < 1 {
			return fmt.Errorf("estudante superior não possui semestre_atual válido")
		}
		esperado := fmt.Sprintf("%d_semestre", *estudante.SemestreAtual)
		if esperado != nivelInformado {
			return fmt.Errorf("nivel_ano_academico_atual incompatível: esperado semestre '%s', recebido '%s'", esperado, nivelInformado)
		}
		return nil
	}

	if nivelAtual == nil || strings.TrimSpace(*nivelAtual) == "" {
		return fmt.Errorf("estudante não possui nível acadêmico atual definido para '%s'", tipoEnsino)
	}
	if *nivelAtual != nivelInformado {
		return fmt.Errorf("nivel_ano_academico_atual incompatível: esperado '%s', recebido '%s'", *nivelAtual, nivelInformado)
	}
	return nil
}
