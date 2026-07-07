package handlers

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

func regrasAvaliacaoFinalEscolaresFixas(c *gin.Context, codigoAcademia, tipoEnsino, anoAcademico string, categoria *string, cursoID *string) []regraAvaliacaoFinalDTO {
	if tipoEnsino != "fundamental" && tipoEnsino != "medio" {
		return nil
	}
	var regras []regraAvaliacaoFinalDTO
	modeloCursoMedio := modeloCursoMedioPorID(c, cursoID)
	if r := regraAvaliacaoFinalEscolarFixa(codigoAcademia, tipoEnsino, anoAcademico, "normal", cursoID, modeloCursoMedio); r != nil {
		regras = append(regras, *r)
	}
	if r := regraAvaliacaoFinalEscolarFixa(codigoAcademia, tipoEnsino, anoAcademico, "exame_recurso", cursoID, modeloCursoMedio); r != nil {
		regras = append(regras, *r)
	}
	if categoria != nil && strings.TrimSpace(*categoria) != "" && len(regras) > 0 {
		categoriaFiltro := strings.TrimSpace(*categoria)
		filtradas := make([]regraAvaliacaoFinalDTO, 0, len(regras))
		for _, regra := range regras {
			if regra.NotaDespertadora != nil && strings.TrimSpace(*regra.NotaDespertadora) == categoriaFiltro {
				filtradas = append(filtradas, regra)
			}
		}
		return filtradas
	}
	return regras
}

func regraAvaliacaoFinalEscolarFixa(codigoAcademia, tipoEnsino, anoAcademico, typ string, cursoID *string, modeloCursoMedio string) *regraAvaliacaoFinalDTO {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		typ = "normal"
	}
	if tipoEnsino != "fundamental" && tipoEnsino != "medio" {
		return nil
	}
	if tipoEnsino == "fundamental" && !isAnoFundamental(anoAcademico) {
		return nil
	}
	if tipoEnsino == "medio" && !isAnoMedio(anoAcademico) {
		return nil
	}
	anoEscopo := anoAcademico
	if tipoEnsino == "medio" && cursoID != nil && strings.TrimSpace(*cursoID) != "" {
		anoEscopo = strings.TrimSpace(*cursoID) + "|" + anoAcademico
	}
	base := regraAvaliacaoFinalDTO{
		ID:                 uuid.NewSHA1(uuid.NameSpaceOID, []byte("spuri:regra-escolar:"+tipoEnsino+":"+anoEscopo+":"+typ)),
		CodigoAcademia:     codigoAcademia,
		Type:               typ,
		Nivel:              tipoEnsino,
		AnosAcademicos:     []string{anoEscopo},
		Status:             "ativo",
		Version:            1,
		Source:             "system",
		Fixed:              true,
		Readonly:           true,
		MateriasAplicaveis: nil,
	}
	regular := "(((([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre])/2)+(([nota_professor,2_trimestre]+[prova_trimestral,2_trimestre])/2)+(([nota_professor,3_trimestre]+[prova_trimestral,3_trimestre])/2))/3)"
	comExame := "(((([nota_professor,1_trimestre]+[prova_trimestral,1_trimestre])/2)+(([nota_professor,2_trimestre]+[prova_trimestral,2_trimestre])/2)+(([nota_professor,3_trimestre]+[exame_final,3_trimestre])/2))/3)"
	switch anoAcademico {
	case "1_ano_fundamental", "2_ano_fundamental", "3_ano_fundamental", "4_ano_fundamental", "5_ano_fundamental":
		if typ != "normal" {
			return nil
		}
		base.Nome = "Avaliação final"
		base.NotaMinimaAprovacao = 5
		base.CategoriasEnvolvidas = []string{"nota_professor", "prova_trimestral"}
		base.Formula = regular
		v := "prova_trimestral"
		base.NotaDespertadora = &v
	case "7_ano_fundamental", "8_ano_fundamental", "1_ano_medio", "2_ano_medio":
		if typ != "normal" {
			return nil
		}
		base.Nome = "Avaliação final"
		base.NotaMinimaAprovacao = 10
		base.CategoriasEnvolvidas = []string{"nota_professor", "prova_trimestral"}
		base.Formula = regular
		v := "prova_trimestral"
		base.NotaDespertadora = &v
	case "6_ano_fundamental", "9_ano_fundamental", "3_ano_medio":
		if typ == "normal" {
			base.Nome = "Avaliação final"
			if anoAcademico == "6_ano_fundamental" {
				base.NotaMinimaAprovacao = 5
			} else {
				base.NotaMinimaAprovacao = 10
			}
			base.CategoriasEnvolvidas = []string{"nota_professor", "prova_trimestral", "exame_final"}
			base.Formula = comExame
			v := "exame_final"
			base.NotaDespertadora = &v
		} else if typ == "exame_recurso" {
			base.Nome = "Exame de recurso"
			if anoAcademico == "6_ano_fundamental" {
				base.NotaMinimaAprovacao = 5
			} else {
				base.NotaMinimaAprovacao = 10
			}
			base.CategoriasEnvolvidas = []string{"exame_recurso"}
			base.Formula = "[exame_recurso,3_trimestre]"
			dep := "normal"
			base.AplicaSeReprovadoEmType = &dep
			v := "exame_recurso"
			base.NotaDespertadora = &v
		} else {
			return nil
		}
	case "4_ano_medio":
		if tipoEnsino != "medio" || typ != "normal" || strings.TrimSpace(modeloCursoMedio) != aggregates.ModeloCursoMedioTecnico {
			return nil
		}
		base.Nome = "Prova de Aptidão Profissional"
		base.NotaMinimaAprovacao = 10
		base.CategoriasEnvolvidas = []string{categoriaNotaPAP}
		base.Formula = "[nota_pap,3_trimestre]"
		v := categoriaNotaPAP
		base.NotaDespertadora = &v
	default:
		return nil
	}
	return &base
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

func modeloCursoMedioPorID(c *gin.Context, cursoID *string) string {
	if c == nil || cursoID == nil || strings.TrimSpace(*cursoID) == "" {
		return ""
	}
	id, err := uuid.Parse(strings.TrimSpace(*cursoID))
	if err != nil {
		return ""
	}
	cursoDTO, err := getCursosProjection(c).GetByID(id)
	if err != nil || cursoDTO == nil || cursoDTO.Type != "medio" {
		return ""
	}
	return cursoDTO.Modelo
}

func categoriasEscolaresFixasDaAcademia(c *gin.Context, academia *projections.AcademiaDTO) []interface{} {
	if academia == nil || academia.Nivel != "escola" {
		return nil
	}
	type item struct{ ano, modelo string }
	vistos := map[string]bool{}
	itens := []item{}
	add := func(ano, modelo string) {
		ano = strings.TrimSpace(ano)
		modelo = strings.TrimSpace(modelo)
		if ano == "" {
			return
		}
		key := ano + "|" + modelo
		if !vistos[key] {
			vistos[key] = true
			itens = append(itens, item{ano: ano, modelo: modelo})
		}
	}
	for _, ano := range academia.AnosAcademicos {
		add(ano, "")
	}
	if c != nil {
		if cursos, err := getCursosProjection(c).GetByAcademia(academia.CodigoAcademia); err == nil {
			for _, curso := range cursos {
				if curso.Type != "medio" || curso.Status != "ativo" {
					continue
				}
				for _, ano := range curso.AnosAcademicos {
					add(ano, curso.Modelo)
				}
			}
		}
	}
	out := []interface{}{}
	for _, it := range itens {
		for _, cat := range categoriasEscolaresFixasParaAno(it.ano, it.modelo) {
			out = append(out, map[string]interface{}{
				"codigo_academia": academia.CodigoAcademia,
				"codigo":          cat.Codigo,
				"nome":            cat.Nome,
				"anos_academicos": []string{it.ano},
				"source":          "system",
				"fixed":           true,
				"readonly":        true,
				"status":          "ativo",
			})
		}
	}
	return out
}

func anexarCategoriasEscolaresFixas(categorias []projections.CategoriaNotaDTO, academia *projections.AcademiaDTO) []interface{} {
	out := make([]interface{}, 0, len(categorias)+8)
	for _, cat := range categorias {
		out = append(out, cat)
	}
	return append(out, categoriasEscolaresFixasDaAcademia(nil, academia)...)
}
