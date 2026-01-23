package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListarTodosRegistros lista TODOS os registros de notas e faltas (apenas admin)
func ListarTodosRegistros(c *gin.Context) {
	client := getDbClient(c)

	limit := 100
	offset := 0

	if limitParam := c.Query("limit"); limitParam != "" {
		_, _ = fmt.Sscanf(limitParam, "%d", &limit)
	}
	if offsetParam := c.Query("offset"); offsetParam != "" {
		_, _ = fmt.Sscanf(offsetParam, "%d", &offset)
	}

	tipoFiltro := c.Query("tipo")
	response := gin.H{}

	// ========================================
	// BUSCAR NOTAS
	// ========================================
	if tipoFiltro == "" || tipoFiltro == "notas" {
		queryNotas := `
			SELECT 
				n.id,
				n.estudante_id,
				e.codigo_estudante,
				e.nome as estudante_nome,
				n.codigo_academia,
				a.nome as academia_nome,
				n.ano_lectivo,
				n.periodo,
				n.materias,
				n.registered_at,
				n.event_id,
				n.version
			FROM projection_notas n
			LEFT JOIN projection_estudantes e ON n.estudante_id = e.id
			LEFT JOIN projection_academias a ON n.codigo_academia = a.codigo_academia
			ORDER BY n.registered_at DESC
			LIMIT $1 OFFSET $2
		`

		type NotaCompleta struct {
			ID              string      `db:"id" json:"id"`
			EstudanteID     string      `db:"estudante_id" json:"estudante_id"`
			CodigoEstudante string      `db:"codigo_estudante" json:"codigo_estudante"`
			EstudanteNome   string      `db:"estudante_nome" json:"estudante_nome"`
			CodigoAcademia  string      `db:"codigo_academia" json:"codigo_academia"`
			AcademiaNome    string      `db:"academia_nome" json:"academia_nome"`
			AnoLectivo      string      `db:"ano_lectivo" json:"ano_lectivo"`
			Periodo         string      `db:"periodo" json:"periodo"`
			Materias        interface{} `db:"materias" json:"materias"`
			RegisteredAt    string      `db:"registered_at" json:"registered_at"`
			EventID         string      `db:"event_id" json:"event_id"`
			Version         int         `db:"version" json:"version"`
		}

		// ✅ CORRIGIDO: Query() + loop ao invés de Select()
		rows, err := client.DB().Query(queryNotas, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "erro ao buscar notas",
				"details": err.Error(),
			})
			return
		}
		defer rows.Close()

		var notas []NotaCompleta
		for rows.Next() {
			var nota NotaCompleta
			err := rows.Scan(
				&nota.ID, &nota.EstudanteID, &nota.CodigoEstudante, &nota.EstudanteNome,
				&nota.CodigoAcademia, &nota.AcademiaNome, &nota.AnoLectivo, &nota.Periodo,
				&nota.Materias, &nota.RegisteredAt, &nota.EventID, &nota.Version,
			)
			if err != nil {
				continue
			}
			notas = append(notas, nota)
		}

		// Contar total de notas
		var totalNotas int
		countQueryNotas := `SELECT COUNT(*) FROM projection_notas`
		client.DB().QueryRow(countQueryNotas).Scan(&totalNotas)

		response["notas"] = notas
		response["total_notas"] = len(notas)
		response["total_notas_geral"] = totalNotas
	}

	// ========================================
	// BUSCAR FALTAS
	// ========================================
	if tipoFiltro == "" || tipoFiltro == "faltas" {
		queryFaltas := `
			SELECT 
				f.id,
				f.estudante_id,
				e.codigo_estudante,
				e.nome as estudante_nome,
				f.codigo_academia,
				a.nome as academia_nome,
				f.ano_lectivo,
				f.periodo,
				f.materias,
				f.registered_at,
				f.event_id,
				f.version
			FROM projection_faltas f
			LEFT JOIN projection_estudantes e ON f.estudante_id = e.id
			LEFT JOIN projection_academias a ON f.codigo_academia = a.codigo_academia
			ORDER BY f.registered_at DESC
			LIMIT $1 OFFSET $2
		`

		type FaltaCompleta struct {
			ID              string      `db:"id" json:"id"`
			EstudanteID     string      `db:"estudante_id" json:"estudante_id"`
			CodigoEstudante string      `db:"codigo_estudante" json:"codigo_estudante"`
			EstudanteNome   string      `db:"estudante_nome" json:"estudante_nome"`
			CodigoAcademia  string      `db:"codigo_academia" json:"codigo_academia"`
			AcademiaNome    string      `db:"academia_nome" json:"academia_nome"`
			AnoLectivo      string      `db:"ano_lectivo" json:"ano_lectivo"`
			Periodo         string      `db:"periodo" json:"periodo"`
			Materias        interface{} `db:"materias" json:"materias"`
			RegisteredAt    string      `db:"registered_at" json:"registered_at"`
			EventID         string      `db:"event_id" json:"event_id"`
			Version         int         `db:"version" json:"version"`
		}

		// ✅ CORRIGIDO: Query() + loop ao invés de Select()
		rows, err := client.DB().Query(queryFaltas, limit, offset)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "erro ao buscar faltas",
				"details": err.Error(),
			})
			return
		}
		defer rows.Close()

		var faltas []FaltaCompleta
		for rows.Next() {
			var falta FaltaCompleta
			err := rows.Scan(
				&falta.ID, &falta.EstudanteID, &falta.CodigoEstudante, &falta.EstudanteNome,
				&falta.CodigoAcademia, &falta.AcademiaNome, &falta.AnoLectivo, &falta.Periodo,
				&falta.Materias, &falta.RegisteredAt, &falta.EventID, &falta.Version,
			)
			if err != nil {
				continue
			}
			faltas = append(faltas, falta)
		}

		// Contar total de faltas
		var totalFaltas int
		countQueryFaltas := `SELECT COUNT(*) FROM projection_faltas`
		client.DB().QueryRow(countQueryFaltas).Scan(&totalFaltas)

		response["faltas"] = faltas
		response["total_faltas"] = len(faltas)
		response["total_faltas_geral"] = totalFaltas
	}

	// ========================================
	// ESTATÍSTICAS GERAIS
	// ========================================
	var stats struct {
		TotalEstudantes int `db:"total_estudantes"`
		TotalAcademias  int `db:"total_academias"`
		TotalNotas      int `db:"total_notas"`
		TotalFaltas     int `db:"total_faltas"`
	}

	queryStats := `
		SELECT 
			(SELECT COUNT(*) FROM projection_estudantes) as total_estudantes,
			(SELECT COUNT(*) FROM projection_academias) as total_academias,
			(SELECT COUNT(*) FROM projection_notas) as total_notas,
			(SELECT COUNT(*) FROM projection_faltas) as total_faltas
	`
	
	// ✅ CORRIGIDO: QueryRow().Scan() ao invés de Get()
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

// ListarRegistrosPorEstudante lista todos os registros de um estudante específico (admin)
func ListarRegistrosPorEstudante(c *gin.Context) {
	codigoEstudante := c.Param("codigo")
	client := getDbClient(c)

	estudanteProj := getEstudanteProjection(c)
	estudante, err := estudanteProj.GetByCodigo(codigoEstudante)
	if err != nil || estudante == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "estudante não encontrado"})
		return
	}

	// Buscar todas as notas
	queryNotas := `
		SELECT 
			n.id,
			n.codigo_academia,
			a.nome as academia_nome,
			n.ano_lectivo,
			n.periodo,
			n.materias,
			n.registered_at
		FROM projection_notas n
		LEFT JOIN projection_academias a ON n.codigo_academia = a.codigo_academia
		WHERE n.estudante_id = $1
		ORDER BY n.registered_at DESC
	`

	type NotaSimples struct {
		ID             string      `db:"id" json:"id"`
		CodigoAcademia string      `db:"codigo_academia" json:"codigo_academia"`
		AcademiaNome   string      `db:"academia_nome" json:"academia_nome"`
		AnoLectivo     string      `db:"ano_lectivo" json:"ano_lectivo"`
		Periodo        string      `db:"periodo" json:"periodo"`
		Materias       interface{} `db:"materias" json:"materias"`
		RegisteredAt   string      `db:"registered_at" json:"registered_at"`
	}

	// ✅ CORRIGIDO: Query() + loop
	rowsNotas, err := client.DB().Query(queryNotas, estudante.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar notas"})
		return
	}
	defer rowsNotas.Close()

	var notas []NotaSimples
	for rowsNotas.Next() {
		var nota NotaSimples
		err := rowsNotas.Scan(
			&nota.ID, &nota.CodigoAcademia, &nota.AcademiaNome,
			&nota.AnoLectivo, &nota.Periodo, &nota.Materias, &nota.RegisteredAt,
		)
		if err != nil {
			continue
		}
		notas = append(notas, nota)
	}

	// Buscar todas as faltas
	queryFaltas := `
		SELECT 
			f.id,
			f.codigo_academia,
			a.nome as academia_nome,
			f.ano_lectivo,
			f.periodo,
			f.materias,
			f.registered_at
		FROM projection_faltas f
		LEFT JOIN projection_academias a ON f.codigo_academia = a.codigo_academia
		WHERE f.estudante_id = $1
		ORDER BY f.registered_at DESC
	`

	type FaltaSimples struct {
		ID             string      `db:"id" json:"id"`
		CodigoAcademia string      `db:"codigo_academia" json:"codigo_academia"`
		AcademiaNome   string      `db:"academia_nome" json:"academia_nome"`
		AnoLectivo     string      `db:"ano_lectivo" json:"ano_lectivo"`
		Periodo        string      `db:"periodo" json:"periodo"`
		Materias       interface{} `db:"materias" json:"materias"`
		RegisteredAt   string      `db:"registered_at" json:"registered_at"`
	}

	// ✅ CORRIGIDO: Query() + loop
	rowsFaltas, err := client.DB().Query(queryFaltas, estudante.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar faltas"})
		return
	}
	defer rowsFaltas.Close()

	var faltas []FaltaSimples
	for rowsFaltas.Next() {
		var falta FaltaSimples
		err := rowsFaltas.Scan(
			&falta.ID, &falta.CodigoAcademia, &falta.AcademiaNome,
			&falta.AnoLectivo, &falta.Periodo, &falta.Materias, &falta.RegisteredAt,
		)
		if err != nil {
			continue
		}
		faltas = append(faltas, falta)
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

// ListarRegistrosPorAcademia lista todos os registros de uma academia (admin)
func ListarRegistrosPorAcademia(c *gin.Context) {
	codigoAcademia := c.Param("codigo")
	client := getDbClient(c)

	academiaProj := getAcademiaProjection(c)
	academia, err := academiaProj.GetByCodigo(codigoAcademia)
	if err != nil || academia == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "academia não encontrada"})
		return
	}

	// Buscar todas as notas desta academia
	queryNotas := `
		SELECT 
			n.id,
			n.estudante_id,
			e.codigo_estudante,
			e.nome as estudante_nome,
			n.ano_lectivo,
			n.periodo,
			n.materias,
			n.registered_at
		FROM projection_notas n
		LEFT JOIN projection_estudantes e ON n.estudante_id = e.id
		WHERE n.codigo_academia = $1
		ORDER BY n.registered_at DESC
	`

	type NotaPorAcademia struct {
		ID              string      `db:"id" json:"id"`
		EstudanteID     string      `db:"estudante_id" json:"estudante_id"`
		CodigoEstudante string      `db:"codigo_estudante" json:"codigo_estudante"`
		EstudanteNome   string      `db:"estudante_nome" json:"estudante_nome"`
		AnoLectivo      string      `db:"ano_lectivo" json:"ano_lectivo"`
		Periodo         string      `db:"periodo" json:"periodo"`
		Materias        interface{} `db:"materias" json:"materias"`
		RegisteredAt    string      `db:"registered_at" json:"registered_at"`
	}

	// ✅ CORRIGIDO: Query() + loop
	rowsNotas, err := client.DB().Query(queryNotas, codigoAcademia)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar notas"})
		return
	}
	defer rowsNotas.Close()

	var notas []NotaPorAcademia
	for rowsNotas.Next() {
		var nota NotaPorAcademia
		err := rowsNotas.Scan(
			&nota.ID, &nota.EstudanteID, &nota.CodigoEstudante, &nota.EstudanteNome,
			&nota.AnoLectivo, &nota.Periodo, &nota.Materias, &nota.RegisteredAt,
		)
		if err != nil {
			continue
		}
		notas = append(notas, nota)
	}

	// Buscar todas as faltas desta academia
	queryFaltas := `
		SELECT 
			f.id,
			f.estudante_id,
			e.codigo_estudante,
			e.nome as estudante_nome,
			f.ano_lectivo,
			f.periodo,
			f.materias,
			f.registered_at
		FROM projection_faltas f
		LEFT JOIN projection_estudantes e ON f.estudante_id = e.id
		WHERE f.codigo_academia = $1
		ORDER BY f.registered_at DESC
	`

	type FaltaPorAcademia struct {
		ID              string      `db:"id" json:"id"`
		EstudanteID     string      `db:"estudante_id" json:"estudante_id"`
		CodigoEstudante string      `db:"codigo_estudante" json:"codigo_estudante"`
		EstudanteNome   string      `db:"estudante_nome" json:"estudante_nome"`
		AnoLectivo      string      `db:"ano_lectivo" json:"ano_lectivo"`
		Periodo         string      `db:"periodo" json:"periodo"`
		Materias        interface{} `db:"materias" json:"materias"`
		RegisteredAt    string      `db:"registered_at" json:"registered_at"`
	}

	// ✅ CORRIGIDO: Query() + loop
	rowsFaltas, err := client.DB().Query(queryFaltas, codigoAcademia)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "erro ao buscar faltas"})
		return
	}
	defer rowsFaltas.Close()

	var faltas []FaltaPorAcademia
	for rowsFaltas.Next() {
		var falta FaltaPorAcademia
		err := rowsFaltas.Scan(
			&falta.ID, &falta.EstudanteID, &falta.CodigoEstudante, &falta.EstudanteNome,
			&falta.AnoLectivo, &falta.Periodo, &falta.Materias, &falta.RegisteredAt,
		)
		if err != nil {
			continue
		}
		faltas = append(faltas, falta)
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