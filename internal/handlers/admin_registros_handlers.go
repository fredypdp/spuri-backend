package handlers

import (
	"fmt"
	"net/http"
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

func ListarTodosRegistros(c *gin.Context) {
	// H4-16: defesa em profundidade — verificação explícita de tipo de usuário.
	if requireAdminType(c) {
		return
	}

	client := getDbClient(c)

	limit := 100
	offset := 0

	if limitParam := c.Query("limit"); limitParam != "" {
		fmt.Sscanf(limitParam, "%d", &limit)
	}
	if offsetParam := c.Query("offset"); offsetParam != "" {
		fmt.Sscanf(offsetParam, "%d", &offset)
	}

	tipoFiltro := c.Query("tipo")
	response := gin.H{}

	if tipoFiltro == "" || tipoFiltro == "notas" {
		// ✅ Sem parâmetros externos nesta query — paginação usa ints validados
		queryNotas := fmt.Sprintf(`
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
			LIMIT %d OFFSET %d
		`, limit, offset)

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

		rows, err := client.DB().Query(queryNotas)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		var notas []NotaCompleta
		for rows.Next() {
			var nota NotaCompleta
			if err := rows.Scan(
				&nota.ID, &nota.CodigoEstudante, &nota.EstudanteNome,
				&nota.CodigoAcademia, &nota.AcademiaNome, &nota.AnoLectivo, &nota.Periodo,
				&nota.MateriaDisciplinarID, &nota.MateriaNome,
				&nota.Nota, &nota.Observacao, &nota.RegisteredAt, &nota.EventID, &nota.Version,
			); err == nil {
				notas = append(notas, nota)
			}
		}

		var totalNotas int
		client.DB().QueryRow(`SELECT COUNT(*) FROM projection_notas`).Scan(&totalNotas)

		response["notas"] = notas
		response["total_notas"] = len(notas)
		response["total_notas_geral"] = totalNotas
	}

	if tipoFiltro == "" || tipoFiltro == "faltas" {
		queryFaltas := fmt.Sprintf(`
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
			LIMIT %d OFFSET %d
		`, limit, offset)

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

		rows, err := client.DB().Query(queryFaltas)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		defer rows.Close()

		var faltas []FaltaCompleta
		for rows.Next() {
			var falta FaltaCompleta
			if err := rows.Scan(
				&falta.ID, &falta.CodigoEstudante, &falta.EstudanteNome,
				&falta.CodigoAcademia, &falta.AcademiaNome, &falta.AnoLectivo,
				&falta.Data, &falta.MateriaDisciplinarID, &falta.MateriaNome,
				&falta.Quantidade, &falta.Observacao, &falta.RegisteredAt, &falta.EventID, &falta.Version,
			); err == nil {
				faltas = append(faltas, falta)
			}
		}

		var totalFaltas int
		client.DB().QueryRow(`SELECT COUNT(*) FROM projection_faltas`).Scan(&totalFaltas)

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
	client.DB().QueryRow(queryStats).Scan(
		&stats.TotalEstudantes,
		&stats.TotalAcademias,
		&stats.TotalNotas,
		&stats.TotalFaltas,
	)

	response["estatisticas"] = stats
	response["limit"] = limit
	response["offset"] = offset
	response["filtro_tipo"] = tipoFiltro

	c.JSON(http.StatusOK, response)
}

func ListarRegistrosPorEstudante(c *gin.Context) {
	// H4-16: defesa em profundidade — verificação explícita de tipo de usuário.
	if requireAdminType(c) {
		return
	}

	codigoEstudante := c.Param("codigo")
	client := getDbClient(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	// ✅ Prepared statement — codigoEstudante é parâmetro $1
	rowsNotas, err := client.DB().Query(`
		SELECT
			n.id, n.codigo_academia, a.nome as academia_nome,
			n.ano_lectivo, n.periodo,
			n.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			n.nota, n.observacao, n.registered_at
		FROM projection_notas n
		LEFT JOIN projection_academias a ON n.codigo_academia = a.codigo_academia
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id
		WHERE n.codigo_estudante = $1
		ORDER BY n.registered_at DESC
	`, estudante.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsNotas.Close()

	type NotaSimples struct {
		ID                   string  `json:"id"`
		CodigoAcademia       string  `json:"codigo_academia"`
		AcademiaNome         string  `json:"academia_nome"`
		AnoLectivo           string  `json:"ano_lectivo"`
		Periodo              string  `json:"periodo"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
		MateriaNome          string  `json:"materia_nome"`
		Nota                 float64 `json:"nota"`
		Observacao           *string `json:"observacao,omitempty"`
		RegisteredAt         string  `json:"registered_at"`
	}

	var notas []NotaSimples
	for rowsNotas.Next() {
		var nota NotaSimples
		if err := rowsNotas.Scan(
			&nota.ID, &nota.CodigoAcademia, &nota.AcademiaNome,
			&nota.AnoLectivo, &nota.Periodo,
			&nota.MateriaDisciplinarID, &nota.MateriaNome,
			&nota.Nota, &nota.Observacao, &nota.RegisteredAt,
		); err == nil {
			notas = append(notas, nota)
		}
	}

	// ✅ Prepared statement — codigoEstudante é parâmetro $1
	rowsFaltas, err := client.DB().Query(`
		SELECT
			f.id, f.codigo_academia, a.nome as academia_nome,
			f.ano_lectivo, f.data,
			f.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			f.quantidade, f.observacao, f.registered_at
		FROM projection_faltas f
		LEFT JOIN projection_academias a ON f.codigo_academia = a.codigo_academia
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id = m.id
		WHERE f.codigo_estudante = $1
		ORDER BY f.registered_at DESC
	`, estudante.CodigoEstudante)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsFaltas.Close()

	type FaltaSimples struct {
		ID                   string  `json:"id"`
		CodigoAcademia       string  `json:"codigo_academia"`
		AcademiaNome         string  `json:"academia_nome"`
		AnoLectivo           string  `json:"ano_lectivo"`
		Data                 string  `json:"data"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
		MateriaNome          string  `json:"materia_nome"`
		Quantidade           int     `json:"quantidade"`
		Observacao           *string `json:"observacao,omitempty"`
		RegisteredAt         string  `json:"registered_at"`
	}

	var faltas []FaltaSimples
	for rowsFaltas.Next() {
		var falta FaltaSimples
		if err := rowsFaltas.Scan(
			&falta.ID, &falta.CodigoAcademia, &falta.AcademiaNome,
			&falta.AnoLectivo, &falta.Data,
			&falta.MateriaDisciplinarID, &falta.MateriaNome,
			&falta.Quantidade, &falta.Observacao, &falta.RegisteredAt,
		); err == nil {
			faltas = append(faltas, falta)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"estudante": gin.H{
			"codigo": estudante.CodigoEstudante,
			"nome":   estudante.Nome,
			"id":     estudante.ID,
		},
		"notas":        notas,
		"total_notas":  len(notas),
		"faltas":       faltas,
		"total_faltas": len(faltas),
	})
}

func ListarRegistrosPorAcademia(c *gin.Context) {
	// H4-16: defesa em profundidade — verificação explícita de tipo de usuário.
	if requireAdminType(c) {
		return
	}

	codigoAcademia := c.Param("codigo")
	client := getDbClient(c)

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	// ✅ Prepared statement — codigoAcademia é parâmetro $1
	rowsNotas, err := client.DB().Query(`
		SELECT
			n.id, n.codigo_estudante, e.nome as estudante_nome,
			n.ano_lectivo, n.periodo,
			n.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			n.nota, n.observacao, n.registered_at
		FROM projection_notas n
		LEFT JOIN projection_estudantes e ON n.codigo_estudante = e.codigo_estudante
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id
		WHERE n.codigo_academia = $1
		ORDER BY n.registered_at DESC
	`, codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsNotas.Close()

	type NotaPorAcademia struct {
		ID                   string  `json:"id"`
		CodigoEstudante      string  `json:"codigo_estudante"`
		EstudanteNome        string  `json:"estudante_nome"`
		AnoLectivo           string  `json:"ano_lectivo"`
		Periodo              string  `json:"periodo"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
		MateriaNome          string  `json:"materia_nome"`
		Nota                 float64 `json:"nota"`
		Observacao           *string `json:"observacao,omitempty"`
		RegisteredAt         string  `json:"registered_at"`
	}

	var notas []NotaPorAcademia
	for rowsNotas.Next() {
		var nota NotaPorAcademia
		if err := rowsNotas.Scan(
			&nota.ID, &nota.CodigoEstudante, &nota.EstudanteNome,
			&nota.AnoLectivo, &nota.Periodo,
			&nota.MateriaDisciplinarID, &nota.MateriaNome,
			&nota.Nota, &nota.Observacao, &nota.RegisteredAt,
		); err == nil {
			notas = append(notas, nota)
		}
	}

	// ✅ Prepared statement — codigoAcademia é parâmetro $1
	rowsFaltas, err := client.DB().Query(`
		SELECT
			f.id, f.codigo_estudante, e.nome as estudante_nome,
			f.ano_lectivo, f.data,
			f.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			f.quantidade, f.observacao, f.registered_at
		FROM projection_faltas f
		LEFT JOIN projection_estudantes e ON f.codigo_estudante = e.codigo_estudante
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id = m.id
		WHERE f.codigo_academia = $1
		ORDER BY f.registered_at DESC
	`, codigoAcademia)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsFaltas.Close()

	type FaltaPorAcademia struct {
		ID                   string  `json:"id"`
		CodigoEstudante      string  `json:"codigo_estudante"`
		EstudanteNome        string  `json:"estudante_nome"`
		AnoLectivo           string  `json:"ano_lectivo"`
		Data                 string  `json:"data"`
		MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
		MateriaNome          string  `json:"materia_nome"`
		Quantidade           int     `json:"quantidade"`
		Observacao           *string `json:"observacao,omitempty"`
		RegisteredAt         string  `json:"registered_at"`
	}

	var faltas []FaltaPorAcademia
	for rowsFaltas.Next() {
		var falta FaltaPorAcademia
		if err := rowsFaltas.Scan(
			&falta.ID, &falta.CodigoEstudante, &falta.EstudanteNome,
			&falta.AnoLectivo, &falta.Data,
			&falta.MateriaDisciplinarID, &falta.MateriaNome,
			&falta.Quantidade, &falta.Observacao, &falta.RegisteredAt,
		); err == nil {
			faltas = append(faltas, falta)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"academia": gin.H{
			"codigo": academia.CodigoAcademia,
			"nome":   academia.Nome,
			"id":     academia.ID,
		},
		"notas":        notas,
		"total_notas":  len(notas),
		"faltas":       faltas,
		"total_faltas": len(faltas),
	})
}