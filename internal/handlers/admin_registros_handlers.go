package handlers

import (
	"fmt"
	"net/http"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
)

func ListarTodosRegistros(c *gin.Context) {
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
		queryNotas := fmt.Sprintf(`
			SELECT 
				n.id, n.estudante_id, e.codigo_estudante, e.nome as estudante_nome,
				n.codigo_academia, a.nome as academia_nome, n.ano_lectivo, n.periodo,
				n.materias, n.registered_at, n.event_id, n.version
			FROM projection_notas n
			LEFT JOIN projection_estudantes e ON n.estudante_id = e.id
			LEFT JOIN projection_academias a ON n.codigo_academia = a.codigo_academia
			ORDER BY n.registered_at DESC
			LIMIT %d OFFSET %d
		`, limit, offset)

		type NotaCompleta struct {
			ID              string      `json:"id"`
			EstudanteID     string      `json:"estudante_id"`
			CodigoEstudante string      `json:"codigo_estudante"`
			EstudanteNome   string      `json:"estudante_nome"`
			CodigoAcademia  string      `json:"codigo_academia"`
			AcademiaNome    string      `json:"academia_nome"`
			AnoLectivo      string      `json:"ano_lectivo"`
			Periodo         string      `json:"periodo"`
			Materias        interface{} `json:"materias"`
			RegisteredAt    string      `json:"registered_at"`
			EventID         string      `json:"event_id"`
			Version         int         `json:"version"`
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
			if err := rows.Scan(&nota.ID, &nota.EstudanteID, &nota.CodigoEstudante, &nota.EstudanteNome,
				&nota.CodigoAcademia, &nota.AcademiaNome, &nota.AnoLectivo, &nota.Periodo,
				&nota.Materias, &nota.RegisteredAt, &nota.EventID, &nota.Version); err == nil {
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
				f.id, f.estudante_id, e.codigo_estudante, e.nome as estudante_nome,
				f.codigo_academia, a.nome as academia_nome, f.ano_lectivo, f.periodo,
				f.materias, f.registered_at, f.event_id, f.version
			FROM projection_faltas f
			LEFT JOIN projection_estudantes e ON f.estudante_id = e.id
			LEFT JOIN projection_academias a ON f.codigo_academia = a.codigo_academia
			ORDER BY f.registered_at DESC
			LIMIT %d OFFSET %d
		`, limit, offset)

		type FaltaCompleta struct {
			ID              string      `json:"id"`
			EstudanteID     string      `json:"estudante_id"`
			CodigoEstudante string      `json:"codigo_estudante"`
			EstudanteNome   string      `json:"estudante_nome"`
			CodigoAcademia  string      `json:"codigo_academia"`
			AcademiaNome    string      `json:"academia_nome"`
			AnoLectivo      string      `json:"ano_lectivo"`
			Periodo         string      `json:"periodo"`
			Materias        interface{} `json:"materias"`
			RegisteredAt    string      `json:"registered_at"`
			EventID         string      `json:"event_id"`
			Version         int         `json:"version"`
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
			if err := rows.Scan(&falta.ID, &falta.EstudanteID, &falta.CodigoEstudante, &falta.EstudanteNome,
				&falta.CodigoAcademia, &falta.AcademiaNome, &falta.AnoLectivo, &falta.Periodo,
				&falta.Materias, &falta.RegisteredAt, &falta.EventID, &falta.Version); err == nil {
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
	codigoEstudante := c.Param("codigo")
	client := getDbClient(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		utils.RespondWithNotFoundError(c, "estudante")
		return
	}

	queryNotas := fmt.Sprintf(`
		SELECT 
			n.id, n.codigo_academia, a.nome as academia_nome,
			n.ano_lectivo, n.periodo, n.materias, n.registered_at
		FROM projection_notas n
		LEFT JOIN projection_academias a ON n.codigo_academia = a.codigo_academia
		WHERE n.estudante_id = '%s'
		ORDER BY n.registered_at DESC
	`, estudante.ID)

	type NotaSimples struct {
		ID             string      `json:"id"`
		CodigoAcademia string      `json:"codigo_academia"`
		AcademiaNome   string      `json:"academia_nome"`
		AnoLectivo     string      `json:"ano_lectivo"`
		Periodo        string      `json:"periodo"`
		Materias       interface{} `json:"materias"`
		RegisteredAt   string      `json:"registered_at"`
	}

	rowsNotas, err := client.DB().Query(queryNotas)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsNotas.Close()

	var notas []NotaSimples
	for rowsNotas.Next() {
		var nota NotaSimples
		if err := rowsNotas.Scan(&nota.ID, &nota.CodigoAcademia, &nota.AcademiaNome,
			&nota.AnoLectivo, &nota.Periodo, &nota.Materias, &nota.RegisteredAt); err == nil {
			notas = append(notas, nota)
		}
	}

	queryFaltas := fmt.Sprintf(`
		SELECT 
			f.id, f.codigo_academia, a.nome as academia_nome,
			f.ano_lectivo, f.periodo, f.materias, f.registered_at
		FROM projection_faltas f
		LEFT JOIN projection_academias a ON f.codigo_academia = a.codigo_academia
		WHERE f.estudante_id = '%s'
		ORDER BY f.registered_at DESC
	`, estudante.ID)

	type FaltaSimples struct {
		ID             string      `json:"id"`
		CodigoAcademia string      `json:"codigo_academia"`
		AcademiaNome   string      `json:"academia_nome"`
		AnoLectivo     string      `json:"ano_lectivo"`
		Periodo        string      `json:"periodo"`
		Materias       interface{} `json:"materias"`
		RegisteredAt   string      `json:"registered_at"`
	}

	rowsFaltas, err := client.DB().Query(queryFaltas)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsFaltas.Close()

	var faltas []FaltaSimples
	for rowsFaltas.Next() {
		var falta FaltaSimples
		if err := rowsFaltas.Scan(&falta.ID, &falta.CodigoAcademia, &falta.AcademiaNome,
			&falta.AnoLectivo, &falta.Periodo, &falta.Materias, &falta.RegisteredAt); err == nil {
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
	codigoAcademia := c.Param("codigo")
	client := getDbClient(c)

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		utils.RespondWithNotFoundError(c, "academia")
		return
	}

	queryNotas := fmt.Sprintf(`
		SELECT 
			n.id, n.estudante_id, e.codigo_estudante, e.nome as estudante_nome,
			n.ano_lectivo, n.periodo, n.materias, n.registered_at
		FROM projection_notas n
		LEFT JOIN projection_estudantes e ON n.estudante_id = e.id
		WHERE n.codigo_academia = '%s'
		ORDER BY n.registered_at DESC
	`, codigoAcademia)

	type NotaPorAcademia struct {
		ID              string      `json:"id"`
		EstudanteID     string      `json:"estudante_id"`
		CodigoEstudante string      `json:"codigo_estudante"`
		EstudanteNome   string      `json:"estudante_nome"`
		AnoLectivo      string      `json:"ano_lectivo"`
		Periodo         string      `json:"periodo"`
		Materias        interface{} `json:"materias"`
		RegisteredAt    string      `json:"registered_at"`
	}

	rowsNotas, err := client.DB().Query(queryNotas)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsNotas.Close()

	var notas []NotaPorAcademia
	for rowsNotas.Next() {
		var nota NotaPorAcademia
		if err := rowsNotas.Scan(&nota.ID, &nota.EstudanteID, &nota.CodigoEstudante, &nota.EstudanteNome,
			&nota.AnoLectivo, &nota.Periodo, &nota.Materias, &nota.RegisteredAt); err == nil {
			notas = append(notas, nota)
		}
	}

	queryFaltas := fmt.Sprintf(`
		SELECT 
			f.id, f.estudante_id, e.codigo_estudante, e.nome as estudante_nome,
			f.ano_lectivo, f.periodo, f.materias, f.registered_at
		FROM projection_faltas f
		LEFT JOIN projection_estudantes e ON f.estudante_id = e.id
		WHERE f.codigo_academia = '%s'
		ORDER BY f.registered_at DESC
	`, codigoAcademia)

	type FaltaPorAcademia struct {
		ID              string      `json:"id"`
		EstudanteID     string      `json:"estudante_id"`
		CodigoEstudante string      `json:"codigo_estudante"`
		EstudanteNome   string      `json:"estudante_nome"`
		AnoLectivo      string      `json:"ano_lectivo"`
		Periodo         string      `json:"periodo"`
		Materias        interface{} `json:"materias"`
		RegisteredAt    string      `json:"registered_at"`
	}

	rowsFaltas, err := client.DB().Query(queryFaltas)
	if err != nil {
		utils.RespondWithInternalError(c, err)
		return
	}
	defer rowsFaltas.Close()

	var faltas []FaltaPorAcademia
	for rowsFaltas.Next() {
		var falta FaltaPorAcademia
		if err := rowsFaltas.Scan(&falta.ID, &falta.EstudanteID, &falta.CodigoEstudante, &falta.EstudanteNome,
			&falta.AnoLectivo, &falta.Periodo, &falta.Materias, &falta.RegisteredAt); err == nil {
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