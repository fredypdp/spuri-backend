package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/db"
	"spuri/internal/domain/aggregates"
	"spuri/internal/middleware"
	"spuri/internal/utils"
)

type nivelEscolarRequest struct {
	NivelEscolar   string   `json:"nivel_escolar"`
	AnosAcademicos []string `json:"anos_academicos"`
}

func bindNivelEscolarRequest(c *gin.Context, req *nivelEscolarRequest) error {
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return newAnosValidationError("payload", "json_invalido", "O corpo da requisição deve ser um JSON válido. Verifique vírgulas, aspas, chaves e tipos dos campos antes de reenviar.")
	}
	for campo := range raw {
		switch campo {
		case "nivel_escolar", "anos_academicos":
		case "codigo_academia":
			return newAnosValidationError(campo, "campo_nao_permitido", "Não envie 'codigo_academia' em PUT /academia/nivel-escolar. A academia alterada é sempre a academia autenticada.")
		default:
			return newAnosValidationError(campo, "campo_nao_permitido", fmt.Sprintf("Campo não suportado em PUT /academia/nivel-escolar: %s", campo))
		}
	}
	if _, ok := raw["nivel_escolar"]; !ok {
		return newAnosValidationError("nivel_escolar", "campo_obrigatorio", "Informe o campo 'nivel_escolar' com o novo valor: 'fundamental', 'medio' ou 'misto'.")
	}
	data, _ := json.Marshal(raw)
	if err := json.Unmarshal(data, req); err != nil {
		return newAnosValidationError("payload", "json_invalido", "O corpo da requisição contém campos com tipos inválidos.")
	}
	return nil
}

// AtualizarNivelEscolarAcademia é o fluxo dedicado (Tarefa 07 revisada) para
// alterar nivel_escolar. PUT /academia/dados bloqueia esse campo
// permanentemente (ver rejectAcademiaDadosRestrictedFields) — esta é a única
// via de alteração. Antes de aplicar, valida que não há dependências ativas
// (estudantes, turmas, cursos, matérias, categorias de nota, regras de
// avaliação final e solicitações de matrícula pendentes) vinculadas ao
// domínio (fundamental/médio) que a academia está deixando de suportar.
func AtualizarNivelEscolarAcademia(c *gin.Context) {
	academiaDTO, ok := academiaAutenticada(c)
	if !ok {
		return
	}
	var req nivelEscolarRequest
	if err := bindNivelEscolarRequest(c, &req); err != nil {
		responderErroAnos(c, err)
		return
	}

	if academiaDTO.Nivel != "escola" {
		responderErroAnosValidacao(c, "nivel_escolar", "nivel_incompativel", fmt.Sprintf("Esta academia não pode ter nivel_escolar porque o nível cadastrado é nivel='%s'. Somente academias escolares (nivel='escola') têm nivel_escolar.", academiaDTO.Nivel))
		return
	}

	novo := strings.TrimSpace(strings.ToLower(req.NivelEscolar))
	if novo != "fundamental" && novo != "medio" && novo != "misto" {
		responderErroAnosValidacao(c, "nivel_escolar", "valor_invalido", fmt.Sprintf("O campo 'nivel_escolar' recebeu '%s', mas só aceita: 'fundamental', 'medio' ou 'misto'.", req.NivelEscolar))
		return
	}

	atual := stringPtrValue(academiaDTO.NivelEscolar)
	if atual == novo {
		responderErroAnosValidacao(c, "nivel_escolar", "sem_alteracao", fmt.Sprintf("Esta academia já está com nivel_escolar='%s'. Nenhuma alteração foi feita.", novo))
		return
	}

	perdeFundamental := atual != "medio" && novo == "medio"
	perdeMedio := atual != "fundamental" && novo == "fundamental"

	if perdeFundamental {
		deps, err := contarDependenciasNivelEscolar(c, academiaDTO.CodigoAcademia, "fundamental")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if deps.total() > 0 {
			responderErroAnos(c, conflictErrorWithDetail("nivel_escolar", "dependencias_ativas_vinculadas", fmt.Sprintf("Não é possível mudar nivel_escolar de '%s' para 'medio' porque existem dependências ativas vinculadas ao ensino fundamental: %s. Resolva essas dependências antes de mudar o nível escolar.", atual, deps.resumo())))
			return
		}
	}
	if perdeMedio {
		deps, err := contarDependenciasNivelEscolar(c, academiaDTO.CodigoAcademia, "medio")
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		if deps.total() > 0 {
			responderErroAnos(c, conflictErrorWithDetail("nivel_escolar", "dependencias_ativas_vinculadas", fmt.Sprintf("Não é possível mudar nivel_escolar de '%s' para 'fundamental' porque existem dependências ativas vinculadas ao ensino médio: %s. Resolva essas dependências antes de mudar o nível escolar.", atual, deps.resumo())))
			return
		}
	}

	// anos_academicos só é aceito neste payload quando a academia está saindo
	// de 'medio' (que nunca tem anos_academicos, ver validarAnosAcademicos)
	// para 'fundamental'/'misto' — precisa de um valor inicial. Nos demais
	// casos o campo é gerido por POST/DELETE /academia/anos-academicos, não
	// por esta rota (mesmo princípio de responsabilidade única já usado em
	// rejectAcademiaDadosRestrictedFields).
	var anosFinal []string
	if novo == "medio" {
		if len(req.AnosAcademicos) > 0 {
			responderErroAnosValidacao(c, "anos_academicos", "campo_nao_permitido", "nivel_escolar='medio' não aceita anos_academicos. Os anos atuais serão limpos automaticamente.")
			return
		}
		anosFinal = []string{}
	} else if atual == "medio" {
		if len(req.AnosAcademicos) == 0 {
			responderErroAnosValidacao(c, "anos_academicos", "campo_obrigatorio", fmt.Sprintf("Informe 'anos_academicos' ao mudar nivel_escolar de 'medio' para '%s'. Exemplo: [\"1_ano_fundamental\", \"2_ano_fundamental\"].", novo))
			return
		}
		validados, err := aggregates.ValidarAnosAcademicosParaNivelEscolar(novo, req.AnosAcademicos)
		if err != nil {
			responderErroAnosValidacao(c, "anos_academicos", "formato_invalido", err.Error())
			return
		}
		anosFinal = validados
	} else {
		if len(req.AnosAcademicos) > 0 {
			responderErroAnosValidacao(c, "anos_academicos", "campo_nao_permitido", "anos_academicos não é aceito nesta rota quando a academia já tinha anos cadastrados. Use POST/DELETE /academia/anos-academicos para ajustá-los.")
			return
		}
		anosFinal = nil // não altera os anos_academicos existentes
	}

	repository := getRepository(c)
	agg, err := repository.Load(academiaDTO.ID, "Academia")
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	academia := agg.(*aggregates.Academia)
	if err := academia.AtualizarDados(nil, nil, nil, nil, nil, nil, nil, nil, &novo, anosFinal, nil); err != nil {
		utils.RespondWithValidationError(c, err)
		return
	}
	userID, _ := middleware.GetUserID(c)
	if err := repository.SaveWithAudit(academia, db.AuditContext{UserID: userID.String(), UserType: "academia", IP: c.ClientIP()}); err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":         "nivel_escolar atualizado com sucesso",
		"nivel_escolar":   novo,
		"anos_academicos": academia.AnosAcademicos,
	})
}

type dependenciasNivelEscolar struct {
	EstudantesAtivos               int
	TurmasAtivas                   int
	CursosAtivos                   int
	MateriasAtivas                 int
	CategoriasNotaAtivas           int
	RegrasAvaliacaoFinalAtivas     int
	SolicitacoesMatriculaPendentes int
}

func (d dependenciasNivelEscolar) total() int {
	return d.EstudantesAtivos + d.TurmasAtivas + d.CursosAtivos + d.MateriasAtivas +
		d.CategoriasNotaAtivas + d.RegrasAvaliacaoFinalAtivas + d.SolicitacoesMatriculaPendentes
}

func (d dependenciasNivelEscolar) resumo() string {
	partes := []string{}
	if d.EstudantesAtivos > 0 {
		partes = append(partes, fmt.Sprintf("%d estudante(s) ativo(s)", d.EstudantesAtivos))
	}
	if d.TurmasAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d turma(s) ativa(s)", d.TurmasAtivas))
	}
	if d.CursosAtivos > 0 {
		partes = append(partes, fmt.Sprintf("%d curso(s) ativo(s)", d.CursosAtivos))
	}
	if d.MateriasAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d matéria(s) ativa(s)", d.MateriasAtivas))
	}
	if d.CategoriasNotaAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d categoria(s) de nota ativa(s)", d.CategoriasNotaAtivas))
	}
	if d.RegrasAvaliacaoFinalAtivas > 0 {
		partes = append(partes, fmt.Sprintf("%d regra(s) de avaliação final ativa(s)", d.RegrasAvaliacaoFinalAtivas))
	}
	if d.SolicitacoesMatriculaPendentes > 0 {
		partes = append(partes, fmt.Sprintf("%d solicitação(ões) de matrícula pendente(s)", d.SolicitacoesMatriculaPendentes))
	}
	return strings.Join(partes, ", ")
}

// contarDependenciasNivelEscolar conta dependências ativas vinculadas ao
// domínio "fundamental" ou "medio" de uma academia, para decidir se a troca
// de nivel_escolar que abandona esse domínio pode prosseguir. As queries
// abaixo foram testadas com dados reais semeados em Postgres antes desta
// tarefa ser escrita para o Codex — ver seção 8 do documento da tarefa.
func contarDependenciasNivelEscolar(c *gin.Context, codigoAcademia, dominio string) (dependenciasNivelEscolar, error) {
	deps := dependenciasNivelEscolar{}
	conn := getDbClient(c).DB()
	sufixoNivel := `%\_ano\_` + dominio

	statusEscolarCampo := "status_escolar_fundamental"
	if dominio == "medio" {
		statusEscolarCampo = "status_escolar_medio"
	}
	if err := conn.QueryRow(fmt.Sprintf(`
		SELECT COUNT(*) FROM projection_estudantes
		 WHERE codigo_academia = $1 AND status = 'ativo' AND %s = 'em_andamento'
	`, statusEscolarCampo), codigoAcademia).Scan(&deps.EstudantesAtivos); err != nil {
		return deps, err
	}

	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM projection_turmas
		 WHERE codigo_academia = $1 AND status = 'ativo' AND deleted_at IS NULL
		   AND nivel LIKE $2 ESCAPE '\'
	`, codigoAcademia, sufixoNivel).Scan(&deps.TurmasAtivas); err != nil {
		return deps, err
	}

	if dominio == "medio" {
		if err := conn.QueryRow(`
			SELECT COUNT(*) FROM projection_cursos
			 WHERE codigo_academia = $1 AND type = 'medio' AND status = 'ativo' AND deleted_at IS NULL
		`, codigoAcademia).Scan(&deps.CursosAtivos); err != nil {
			return deps, err
		}
	}

	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM projection_materias
		 WHERE codigo_academia = $1 AND type = $2 AND status = 'ativo' AND deleted_at IS NULL
	`, codigoAcademia, dominio).Scan(&deps.MateriasAtivas); err != nil {
		return deps, err
	}

	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM projection_categorias_nota
		 WHERE codigo_academia = $1 AND status = 'ativo'
		   AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(anos_academicos) AS ano WHERE ano LIKE $2 ESCAPE '\')
	`, codigoAcademia, sufixoNivel).Scan(&deps.CategoriasNotaAtivas); err != nil {
		return deps, err
	}

	if err := conn.QueryRow(`
		SELECT COUNT(*) FROM projection_regras_avaliacao_final
		 WHERE codigo_academia = $1 AND nivel = $2 AND status = 'ativo'
	`, codigoAcademia, dominio).Scan(&deps.RegrasAvaliacaoFinalAtivas); err != nil {
		return deps, err
	}

	if dominio == "fundamental" {
		if err := conn.QueryRow(`
			SELECT COUNT(*) FROM projection_solicitacoes_matricula
			 WHERE codigo_academia = $1 AND status = 'pendente' AND ano_escolar_fundamental IS NOT NULL
		`, codigoAcademia).Scan(&deps.SolicitacoesMatriculaPendentes); err != nil {
			return deps, err
		}
	} else {
		if err := conn.QueryRow(`
			SELECT COUNT(*) FROM projection_solicitacoes_matricula
			 WHERE codigo_academia = $1 AND status = 'pendente' AND (ano_escolar_medio IS NOT NULL OR curso_medio_id IS NOT NULL)
		`, codigoAcademia).Scan(&deps.SolicitacoesMatriculaPendentes); err != nil {
			return deps, err
		}
	}

	return deps, nil
}
