package handlers

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"spuri/internal/domain/aggregates"
	"spuri/internal/projections"
)

const (
	categoriaNotaPAP = "nota_pap"
)

type categoriaNotaEscolarFixa struct {
	Codigo string `json:"codigo"`
	Nome   string `json:"nome"`
}

var categoriasEscolaresRegulares = []categoriaNotaEscolarFixa{
	{Codigo: "nota_professor", Nome: "Nota do professor/Avaliação contínua"},
	{Codigo: "prova_trimestral", Nome: "Prova trimestral"},
}

var categoriasEscolaresComExames = []categoriaNotaEscolarFixa{
	{Codigo: "nota_professor", Nome: "Nota do professor/Avaliação contínua"},
	{Codigo: "prova_trimestral", Nome: "Prova trimestral"},
	{Codigo: "exame_final", Nome: "Exame final"},
	{Codigo: "exame_recurso", Nome: "Exame de recurso"},
}

var categoriasEscolaresPAP = []categoriaNotaEscolarFixa{
	{Codigo: categoriaNotaPAP, Nome: "Prova de Aptidão Profissional"},
}

func isAnoFundamental(ano string) bool {
	return strings.HasSuffix(strings.TrimSpace(ano), "_ano_fundamental")
}
func isAnoMedio(ano string) bool { return strings.HasSuffix(strings.TrimSpace(ano), "_ano_medio") }

func categoriasEscolaresFixasParaAno(anoAcademico string, modeloCursoMedio string) []categoriaNotaEscolarFixa {
	ano := strings.TrimSpace(anoAcademico)
	if ano == "4_ano_medio" && strings.TrimSpace(modeloCursoMedio) == aggregates.ModeloCursoMedioTecnico {
		return categoriasEscolaresPAP
	}
	switch ano {
	case "6_ano_fundamental", "9_ano_fundamental", "3_ano_medio":
		return categoriasEscolaresComExames
	default:
		if isAnoFundamental(ano) || isAnoMedio(ano) {
			return categoriasEscolaresRegulares
		}
		return nil
	}
}

func codigosCategoriasEscolaresFixasParaAno(anoAcademico, modeloCursoMedio string) []string {
	cats := categoriasEscolaresFixasParaAno(anoAcademico, modeloCursoMedio)
	out := make([]string, 0, len(cats))
	for _, cat := range cats {
		out = append(out, cat.Codigo)
	}
	return out
}

func validarEscalaNotaPorAnoAcademico(anoAcademico string, nota float64) error {
	max := 20.0
	if strings.Contains(anoAcademico, "_ano_fundamental") {
		switch anoAcademico {
		case "1_ano_fundamental", "2_ano_fundamental", "3_ano_fundamental", "4_ano_fundamental", "5_ano_fundamental", "6_ano_fundamental":
			max = 10
		}
	}
	if nota < 0 || nota > max {
		return fmt.Errorf("nota %.2f fora da escala permitida para %s: use valores entre 0 e %.0f", nota, anoAcademico, max)
	}
	return nil
}

func modeloCursoMedioDaMateria(c *gin.Context, materiaDTO *projections.MateriaDTO) string {
	if materiaDTO == nil || materiaDTO.CursoID == nil {
		return ""
	}
	cursoDTO, err := getCursosProjection(c).GetByID(*materiaDTO.CursoID)
	if err != nil || cursoDTO == nil || cursoDTO.Type != "medio" {
		return ""
	}
	return cursoDTO.Modelo
}

func anexarCategoriasEscolaresFixas(categorias []projections.CategoriaNotaDTO, academia *projections.AcademiaDTO) []interface{} {
	out := make([]interface{}, 0, len(categorias)+8)
	for _, cat := range categorias {
		out = append(out, cat)
	}
	if academia == nil || academia.Nivel != "escola" {
		return out
	}
	anos := academia.AnosAcademicos
	for _, ano := range anos {
		for _, cat := range categoriasEscolaresFixasParaAno(ano, "") {
			out = append(out, map[string]interface{}{
				"codigo_academia": academia.CodigoAcademia,
				"codigo":          cat.Codigo,
				"nome":            cat.Nome,
				"anos_academicos": []string{ano},
				"source":          "system",
				"fixed":           true,
				"readonly":        true,
				"status":          "ativo",
			})
		}
	}
	return out
}
