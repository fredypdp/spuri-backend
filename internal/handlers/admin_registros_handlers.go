package handlers

import (
	"log"
	"net/http"
	"spuri/internal/db"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
)

// requireAdminType é um guard inline de defesa em profundidade.
// Verifica que o usuário autenticado é do tipo "admin", independentemente
// do middleware de rota já aplicado. Retorna true se o acesso foi bloqueado.
func requireAdminType(c *gin.Context) bool {
	userType, _ := middleware.GetUserType(c)
	if userType != "admin" {
		utils.RespondWithForbiddenError(c, "acesso restrito a administradores")
		return true
	}
	return false
}

// ListarTodosRegistros lista notas e faltas de todos os estudantes.
//
// FIX H4-ADR-01: erros de rows.Scan agora são logados (não silenciados).
// FIX H4-ADR-02: rows.Err() verificado após cada loop de iteração.
// FIX H4-ADR-04: erros de QueryRow para contagens agora são logados.
func ListarTodosRegistros(c *gin.Context) {
	// H4-16: defesa em profundidade — verificação explícita de tipo de usuário.
	if requireAdminType(c) {
		return
	}

	client := getDbClient(c)

	// FIX HDL-01: usa db.ValidateLimit e db.ValidateOffset em vez de valores
	// brutos de fmt.Sscanf sem validação. Isso garante:
	//   - limit=0 retorna o default (50) em vez de LIMIT 0
	//   - limit=9999999 é truncado para 1000 (maxLimit)
	//   - offset negativo é tratado como 0
	limit, offset := getPaginationParams(c)
	limit = db.ValidateLimit(limit)
	offset = db.ValidateOffset(offset)

	tipoFiltro := c.Query("tipo")
	response := gin.H{}

	if tipoFiltro == "" || tipoFiltro == "notas" {
		// FIX HDL-02: prepared statement com $1/$2.
		queryNotas := `
			SELECT
				n.id, n.codigo_estudante, e.nome as estudante_nome,
				n.codigo_academia, a.nome as academia_nome, n.ano_lectivo, n.periodo,
				n.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
				n.nota, n.observacao, n.registered_at, n.event_id, n.version
			FROM projection_notas n
			LEFT JOIN projection_estudantes e ON n.codigo_estudante = e.codigo_estudante
			LEFT JOIN projection_academias a ON n.codigo_academia = a.codigo_academia
			LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id
			ORDER BY n.registered_at DESC
			LIMIT $1 OFFSET $2
		`

		type NotaCompleta struct {
			ID                   string  `json:"id"`
			CodigoEstudante      string  `json:"codigo_estudante"`
			EstudanteNome        string  `json:"estudante_nome"`
			CodigoAcademia       string  `json:"codigo_academia"`
			AcademiaNome         string  `json:"academia_nome"`
			AnoLectivo           string  `json:"ano_lectivo"`
			Periodo              string  `json:"periodo"`
			MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
			MateriaNome          string  `json:"materia_nome"`
			Nota                 float64 `json:"nota"`
			Observacao           *string `json:"observacao,omitempty"`
			RegisteredAt         string  `json:"registered_at"`
			EventID              string  `json:"event_id"`
			Version              int     `json:"version"`
		}

		rows, err := client.DB().Query(queryNotas, limit, offset)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		var notas []NotaCompleta
		for rows.Next() {
			var nota NotaCompleta
			// FIX H4-ADR-01: erro de Scan logado em vez de silenciado.
			if err := rows.Scan(
				&nota.ID, &nota.CodigoEstudante, &nota.EstudanteNome,
				&nota.CodigoAcademia, &nota.AcademiaNome, &nota.AnoLectivo, &nota.Periodo,
				&nota.MateriaDisciplinarID, &nota.MateriaNome,
				&nota.Nota, &nota.Observacao, &nota.RegisteredAt, &nota.EventID, &nota.Version,
			); err != nil {
				log.Printf("[WARN] ListarTodosRegistros/notas: erro ao ler linha: %v", err)
				continue
			}
			notas = append(notas, nota)
		}

		// FIX H4-ADR-02: verificar rows.Err() após iteração.
		if err := rows.Err(); err != nil {
			log.Printf("[ERROR] ListarTodosRegistros/notas: erro durante iteração: %v", err)
			utils.RespondWithInternalError(c, err)
			return
		}

		var totalNotas int
		// FIX H4-ADR-04: erro de QueryRow para contagem logado.
		if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_notas`).Scan(&totalNotas); err != nil {
			log.Printf("[WARN] ListarTodosRegistros: erro ao contar notas: %v", err)
		}

		response["notas"] = notas
		response["total_notas"] = len(notas)
		response["total_notas_geral"] = totalNotas
	}

	if tipoFiltro == "" || tipoFiltro == "faltas" {
		// FIX HDL-02: prepared statement com $1/$2.
		queryFaltas := `
			SELECT
				f.id, f.codigo_estudante, e.nome as estudante_nome,
				f.codigo_academia, a.nome as academia_nome, f.ano_lectivo,
				f.data, f.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
				f.quantidade, f.observacao, f.registered_at, f.event_id, f.version
			FROM projection_faltas f
			LEFT JOIN projection_estudantes e ON f.codigo_estudante = e.codigo_estudante
			LEFT JOIN projection_academias a ON f.codigo_academia = a.codigo_academia
			LEFT JOIN projection_materias m ON f.materia_disciplinar_id = m.id
			ORDER BY f.registered_at DESC
			LIMIT $1 OFFSET $2
		`

		type FaltaCompleta struct {
			ID                   string  `json:"id"`
			CodigoEstudante      string  `json:"codigo_estudante"`
			EstudanteNome        string  `json:"estudante_nome"`
			CodigoAcademia       string  `json:"codigo_academia"`
			AcademiaNome         string  `json:"academia_nome"`
			AnoLectivo           string  `json:"ano_lectivo"`
			Data                 string  `json:"data"`
			MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
			MateriaNome          string  `json:"materia_nome"`
			Quantidade           int     `json:"quantidade"`
			Observacao           *string `json:"observacao,omitempty"`
			RegisteredAt         string  `json:"registered_at"`
			EventID              string  `json:"event_id"`
			Version              int     `json:"version"`
		}

		rows, err := client.DB().Query(queryFaltas, limit, offset)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		var faltas []FaltaCompleta
		for rows.Next() {
			var falta FaltaCompleta
			// FIX H4-ADR-01: erro de Scan logado em vez de silenciado.
			if err := rows.Scan(
				&falta.ID, &falta.CodigoEstudante, &falta.EstudanteNome,
				&falta.CodigoAcademia, &falta.AcademiaNome, &falta.AnoLectivo,
				&falta.Data, &falta.MateriaDisciplinarID, &falta.MateriaNome,
				&falta.Quantidade, &falta.Observacao, &falta.RegisteredAt, &falta.EventID, &falta.Version,
			); err != nil {
				log.Printf("[WARN] ListarTodosRegistros/faltas: erro ao ler linha: %v", err)
				continue
			}
			faltas = append(faltas, falta)
		}

		// FIX H4-ADR-02: verificar rows.Err() após iteração.
		if err := rows.Err(); err != nil {
			log.Printf("[ERROR] ListarTodosRegistros/faltas: erro durante iteração: %v", err)
			utils.RespondWithInternalError(c, err)
			return
		}

		var totalFaltas int
		// FIX H4-ADR-04: erro de QueryRow para contagem logado.
		if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_faltas`).Scan(&totalFaltas); err != nil {
			log.Printf("[WARN] ListarTodosRegistros: erro ao contar faltas: %v", err)
		}

		response["faltas"] = faltas
		response["total_faltas"] = len(faltas)
		response["total_faltas_geral"] = totalFaltas
	}

	var stats struct {
		TotalEstudantes int
		TotalAcademias  int
		TotalNotas      int
		TotalFaltas     int
	}

	queryStats := `
		SELECT
			(SELECT COUNT(*) FROM projection_estudantes) as total_estudantes,
			(SELECT COUNT(*) FROM projection_academias) as total_academias,
			(SELECT COUNT(*) FROM projection_notas) as total_notas,
			(SELECT COUNT(*) FROM projection_faltas) as total_faltas
	`
	// FIX H4-ADR-04: erro de QueryRow para estatísticas logado.
	if err := client.DB().QueryRow(queryStats).Scan(
		&stats.TotalEstudantes,
		&stats.TotalAcademias,
		&stats.TotalNotas,
		&stats.TotalFaltas,
	); err != nil {
		log.Printf("[WARN] ListarTodosRegistros: erro ao carregar estatísticas: %v", err)
	}

	response["estatisticas"] = stats
	response["limit"] = limit
	response["offset"] = offset
	response["filtro_tipo"] = tipoFiltro

	c.JSON(http.StatusOK, response)
}

// ListarRegistrosPorEstudante lista notas e faltas de um estudante específico.
//
// FIX H4-ADR-01: erros de rows.Scan agora são logados.
// FIX H4-ADR-02: rows.Err() verificado após cada loop.
// FIX H4-ADR-03: codigo_estudante validado contra vazio antes de qualquer query.
func ListarRegistrosPorEstudante(c *gin.Context) {
	// H4-16: defesa em profundidade — verificação explícita de tipo de usuário.
	if requireAdminType(c) {
		return
	}

	codigoEstudante := c.Param("codigo")
	if codigoEstudante == "" {
		utils.RespondWithValidationError(c, nil)
		return
	}

	client := getDbClient(c)

	// FIX HDL-01/02: prepared statements com $1/$2/$3 e limites validados.
	limit := db.ValidateLimit(100)
	offset := db.ValidateOffset(0)

	type NotaEstudante struct {
		ID                   string  `json:"id"`
		CodigoAcademia       string  `json:"codigo_academia"`
		AnoLectivo           string  `json:"ano_lectivo"`
		Periodo              string  `json:"periodo"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
		MateriaNome          string  `json:"materia_nome"`
		Nota                 float64 `json:"nota"`
		Observacao           *string `json:"observacao,omitempty"`
		RegisteredAt         string  `json:"registered_at"`
	}

	rowsNotas, err := client.DB().Query(`
		SELECT n.id, n.codigo_academia, n.ano_lectivo, n.periodo,
			n.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			n.nota, n.observacao, n.registered_at
		FROM projection_notas n
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id
		WHERE n.codigo_estudante = $1
		ORDER BY n.registered_at DESC
		LIMIT $2 OFFSET $3
	`, codigoEstudante, limit, offset)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsNotas.Close()

	var notas []NotaEstudante
	for rowsNotas.Next() {
		var nota NotaEstudante
		// FIX H4-ADR-01: erro de Scan logado.
		if err := rowsNotas.Scan(
			&nota.ID, &nota.CodigoAcademia, &nota.AnoLectivo, &nota.Periodo,
			&nota.MateriaDisciplinarID, &nota.MateriaNome,
			&nota.Nota, &nota.Observacao, &nota.RegisteredAt,
		); err != nil {
			log.Printf("[WARN] ListarRegistrosPorEstudante/notas: erro ao ler linha: %v", err)
			continue
		}
		notas = append(notas, nota)
	}

	// FIX H4-ADR-02: verificar rows.Err() após iteração de notas.
	if err := rowsNotas.Err(); err != nil {
		log.Printf("[ERROR] ListarRegistrosPorEstudante/notas: erro durante iteração: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	type FaltaEstudante struct {
		ID                   string  `json:"id"`
		CodigoAcademia       string  `json:"codigo_academia"`
		AnoLectivo           string  `json:"ano_lectivo"`
		Data                 string  `json:"data"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
		MateriaNome          string  `json:"materia_nome"`
		Quantidade           int     `json:"quantidade"`
		Observacao           *string `json:"observacao,omitempty"`
		RegisteredAt         string  `json:"registered_at"`
	}

	rowsFaltas, err := client.DB().Query(`
		SELECT f.id, f.codigo_academia, f.ano_lectivo, f.data,
			f.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			f.quantidade, f.observacao, f.registered_at
		FROM projection_faltas f
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id = m.id
		WHERE f.codigo_estudante = $1
		ORDER BY f.registered_at DESC
		LIMIT $2 OFFSET $3
	`, codigoEstudante, limit, offset)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsFaltas.Close()

	var faltas []FaltaEstudante
	for rowsFaltas.Next() {
		var falta FaltaEstudante
		// FIX H4-ADR-01: erro de Scan logado.
		if err := rowsFaltas.Scan(
			&falta.ID, &falta.CodigoAcademia, &falta.AnoLectivo, &falta.Data,
			&falta.MateriaDisciplinarID, &falta.MateriaNome,
			&falta.Quantidade, &falta.Observacao, &falta.RegisteredAt,
		); err != nil {
			log.Printf("[WARN] ListarRegistrosPorEstudante/faltas: erro ao ler linha: %v", err)
			continue
		}
		faltas = append(faltas, falta)
	}

	// FIX H4-ADR-02: verificar rows.Err() após iteração de faltas.
	if err := rowsFaltas.Err(); err != nil {
		log.Printf("[ERROR] ListarRegistrosPorEstudante/faltas: erro durante iteração: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"codigo_estudante": codigoEstudante,
		"notas":            notas,
		"faltas":           faltas,
		"total_notas":      len(notas),
		"total_faltas":     len(faltas),
	})
}