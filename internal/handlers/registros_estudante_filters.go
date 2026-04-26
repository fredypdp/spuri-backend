package handlers

import (
	"fmt"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"spuri/internal/projections"
)

type filtrosEstudante struct {
	anoLectivos           []string
	anoAcademicos         []string
	cursoIDs              []string
	periodos              []string
	materiasDisciplinares []string
	codigosAcademia       []string
	categorias            []string
}

func parseFiltrosRegistrosEstudante(c *gin.Context, includeCategoria bool) (filtrosEstudante, error) {
	f := filtrosEstudante{
		anoLectivos:           parseMultiValueQueryParam(c, "ano_letivo"),
		anoAcademicos:         parseMultiValueQueryParam(c, "ano_academico"),
		cursoIDs:              parseMultiValueQueryParam(c, "curso_id"),
		periodos:              parseMultiValueQueryParam(c, "periodo"),
		materiasDisciplinares: parseMultiValueQueryParam(c, "materia_disciplinar_id"),
		codigosAcademia:       parseMultiValueQueryParam(c, "codigo_academia"),
	}
	if includeCategoria {
		f.categorias = parseMultiValueQueryParam(c, "categoria")
	}

	for _, cursoID := range f.cursoIDs {
		if _, err := uuid.Parse(cursoID); err != nil {
			return f, fmt.Errorf("curso_id inválido")
		}
	}
	for _, materiaID := range f.materiasDisciplinares {
		if _, err := uuid.Parse(materiaID); err != nil {
			return f, fmt.Errorf("materia_disciplinar_id inválido")
		}
	}

	return f, nil
}

func matchesFiltroString(filtro []string, value string) bool {
	return len(filtro) == 0 || slices.Contains(filtro, value)
}

type materiaMeta struct {
	cursoID string
	periodo string
}

func getMateriaMeta(materiasProj *projections.MateriasProjection, cache map[string]materiaMeta, materiaID string) (materiaMeta, error) {
	if meta, ok := cache[materiaID]; ok {
		return meta, nil
	}

	id, err := uuid.Parse(materiaID)
	if err != nil {
		cache[materiaID] = materiaMeta{}
		return cache[materiaID], nil
	}

	m, err := materiasProj.GetByID(id)
	if err != nil || m == nil {
		cache[materiaID] = materiaMeta{}
		return cache[materiaID], err
	}

	meta := materiaMeta{}
	if m.CursoID != nil {
		meta.cursoID = m.CursoID.String()
	}
	if m.Periodo != nil {
		meta.periodo = *m.Periodo
	}
	cache[materiaID] = meta
	return meta, nil
}
