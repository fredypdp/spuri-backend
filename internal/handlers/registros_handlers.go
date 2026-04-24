package handlers

import (
	"log"
	"net/http"

	"spuri/internal/db"
	"spuri/internal/middleware"
	"spuri/internal/utils"

	"github.com/gin-gonic/gin"
)

type NotaRegistroResponse struct {
	ID                   string  `json:"id"`
	CodigoEstudante      string  `json:"codigo_estudante"`
	EstudanteNome        string  `json:"estudante_nome"`
	CodigoAcademia       string  `json:"codigo_academia"`
	AcademiaNome         string  `json:"academia_nome"`
	AnoLectivo           string  `json:"ano_lectivo"`
	AnoAcademico         string  `json:"ano_academico"`
	Periodo              string  `json:"periodo"`
	MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
	MateriaNome          string  `json:"materia_nome"`
	Tipo                 string  `json:"tipo"`
	Categoria            string  `json:"categoria"`
	Nota                 float64 `json:"nota"`
	Observacao           *string `json:"observacao,omitempty"`
	RegisteredAt         string  `json:"registered_at"`
	EventID              string  `json:"event_id"`
	Version              int     `json:"version"`
}

type FaltaRegistroResponse struct {
	ID                   string  `json:"id"`
	CodigoEstudante      string  `json:"codigo_estudante"`
	EstudanteNome        string  `json:"estudante_nome"`
	CodigoAcademia       string  `json:"codigo_academia"`
	AcademiaNome         string  `json:"academia_nome"`
	AnoLectivo           string  `json:"ano_lectivo"`
	AnoAcademico         string  `json:"ano_academico"`
	Data                 string  `json:"data"`
	MateriaDisciplinarID string  `json:"materia_disciplinar_id"`
	MateriaNome          string  `json:"materia_nome"`
	Quantidade           int     `json:"quantidade"`
	Observacao           *string `json:"observacao,omitempty"`
	RegisteredAt         string  `json:"registered_at"`
	EventID              string  `json:"event_id"`
	Version              int     `json:"version"`
}

func resolverEscopoRegistros(c *gin.Context) (userType, codigoAcademia string, ok bool) {
	userID, _ := middleware.GetUserID(c)
	userType, _ = middleware.GetUserType(c)

	switch userType {
	case "admin":
		return userType, "", true
	case "academia":
		academiaProj := getAcademiaProjection(c)
		academiaDTO, err := academiaProj.GetByID(userID)
		if err != nil || academiaDTO == nil {
			utils.RespondWithNotFoundError(c, "academia")
			return "", "", false
		}
		return userType, academiaDTO.CodigoAcademia, true
	default:
		utils.RespondWithForbiddenError(c, "acesso permitido apenas para admin e academia")
		return "", "", false
	}
}

// ListarNotas retorna notas do sistema:
// - admin: todas as notas
// - academia: apenas notas vinculadas ao seu codigo_academia
func ListarNotas(c *gin.Context) {
	userType, codigoAcademia, ok := resolverEscopoRegistros(c)
	if !ok {
		return
	}

	client := getDbClient(c)
	limit, offset := getPaginationParams(c)
	limit = db.ValidateLimit(limit)
	offset = db.ValidateOffset(offset)

	query := `
		SELECT
			n.id, n.codigo_estudante, e.nome as estudante_nome,
			n.codigo_academia, a.nome as academia_nome, n.ano_lectivo, n.ano_academico, n.periodo,
			n.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			n.tipo, n.categoria, n.nota, n.observacao, n.registered_at, n.event_id, n.version
		FROM projection_notas n
		LEFT JOIN projection_estudantes e ON n.codigo_estudante = e.codigo_estudante
		LEFT JOIN projection_academias a ON n.codigo_academia = a.codigo_academia
		LEFT JOIN projection_materias m ON n.materia_disciplinar_id = m.id
	`

	var rowsErr error
	var rows interface {
		Close() error
		Next() bool
		Scan(...interface{}) error
		Err() error
	}

	if userType == "academia" {
		rowsTyped, err := client.DB().Query(query+` WHERE n.codigo_academia = $1 ORDER BY n.registered_at DESC LIMIT $2 OFFSET $3`, codigoAcademia, limit, offset)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		rows = rowsTyped
	} else {
		rowsTyped, err := client.DB().Query(query+` ORDER BY n.registered_at DESC LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		rows = rowsTyped
	}
	defer rows.Close()

	var notas []NotaRegistroResponse
	for rows.Next() {
		var nota NotaRegistroResponse
		if err := rows.Scan(
			&nota.ID, &nota.CodigoEstudante, &nota.EstudanteNome,
			&nota.CodigoAcademia, &nota.AcademiaNome, &nota.AnoLectivo, &nota.AnoAcademico, &nota.Periodo,
			&nota.MateriaDisciplinarID, &nota.MateriaNome,
			&nota.Tipo, &nota.Categoria, &nota.Nota, &nota.Observacao, &nota.RegisteredAt, &nota.EventID, &nota.Version,
		); err != nil {
			log.Printf("[WARN] ListarNotas: erro ao ler linha: %v", err)
			continue
		}
		notas = append(notas, nota)
	}

	if rowsErr = rows.Err(); rowsErr != nil {
		log.Printf("[ERROR] ListarNotas: erro durante iteração: %v", rowsErr)
		utils.RespondWithInternalError(c, rowsErr)
		return
	}

	var total int
	if userType == "academia" {
		if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_notas WHERE codigo_academia = $1`, codigoAcademia).Scan(&total); err != nil {
			log.Printf("[WARN] ListarNotas: erro ao contar notas da academia %s: %v", codigoAcademia, err)
		}
	} else {
		if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_notas`).Scan(&total); err != nil {
			log.Printf("[WARN] ListarNotas: erro ao contar notas: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"notas":       notas,
		"total":       len(notas),
		"total_geral": total,
		"limit":       limit,
		"offset":      offset,
	})
}

// ListarFaltas retorna faltas do sistema:
// - admin: todas as faltas
// - academia: apenas faltas vinculadas ao seu codigo_academia
func ListarFaltas(c *gin.Context) {
	userType, codigoAcademia, ok := resolverEscopoRegistros(c)
	if !ok {
		return
	}

	client := getDbClient(c)
	limit, offset := getPaginationParams(c)
	limit = db.ValidateLimit(limit)
	offset = db.ValidateOffset(offset)

	query := `
		SELECT
			f.id, f.codigo_estudante, e.nome as estudante_nome,
			f.codigo_academia, a.nome as academia_nome, f.ano_lectivo, f.ano_academico,
			f.data, f.materia_disciplinar_id, COALESCE(m.nome, '') as materia_nome,
			f.quantidade, f.observacao, f.registered_at, f.event_id, f.version
		FROM projection_faltas f
		LEFT JOIN projection_estudantes e ON f.codigo_estudante = e.codigo_estudante
		LEFT JOIN projection_academias a ON f.codigo_academia = a.codigo_academia
		LEFT JOIN projection_materias m ON f.materia_disciplinar_id = m.id
	`

	var rows interface {
		Close() error
		Next() bool
		Scan(...interface{}) error
		Err() error
	}
	if userType == "academia" {
		rowsTyped, err := client.DB().Query(query+` WHERE f.codigo_academia = $1 ORDER BY f.registered_at DESC LIMIT $2 OFFSET $3`, codigoAcademia, limit, offset)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		rows = rowsTyped
	} else {
		rowsTyped, err := client.DB().Query(query+` ORDER BY f.registered_at DESC LIMIT $1 OFFSET $2`, limit, offset)
		if err != nil {
			utils.RespondWithInternalError(c, err)
			return
		}
		rows = rowsTyped
	}
	defer rows.Close()

	var faltas []FaltaRegistroResponse
	for rows.Next() {
		var falta FaltaRegistroResponse
		if err := rows.Scan(
			&falta.ID, &falta.CodigoEstudante, &falta.EstudanteNome,
			&falta.CodigoAcademia, &falta.AcademiaNome, &falta.AnoLectivo, &falta.AnoAcademico,
			&falta.Data, &falta.MateriaDisciplinarID, &falta.MateriaNome,
			&falta.Quantidade, &falta.Observacao, &falta.RegisteredAt, &falta.EventID, &falta.Version,
		); err != nil {
			log.Printf("[WARN] ListarFaltas: erro ao ler linha: %v", err)
			continue
		}
		faltas = append(faltas, falta)
	}

	if err := rows.Err(); err != nil {
		log.Printf("[ERROR] ListarFaltas: erro durante iteração: %v", err)
		utils.RespondWithInternalError(c, err)
		return
	}

	var total int
	if userType == "academia" {
		if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_faltas WHERE codigo_academia = $1`, codigoAcademia).Scan(&total); err != nil {
			log.Printf("[WARN] ListarFaltas: erro ao contar faltas da academia %s: %v", codigoAcademia, err)
		}
	} else {
		if err := client.DB().QueryRow(`SELECT COUNT(*) FROM projection_faltas`).Scan(&total); err != nil {
			log.Printf("[WARN] ListarFaltas: erro ao contar faltas: %v", err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"faltas":      faltas,
		"total":       len(faltas),
		"total_geral": total,
		"limit":       limit,
		"offset":      offset,
	})
}
