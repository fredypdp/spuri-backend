package handlers

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseMultiValueQueryParam_CombinadoRepetidoECSV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/notas?ano_letivo=2024_2025,2025_2026&ano_letivo=2025_2026&ano_letivo=2026_2027", nil)

	valores := parseMultiValueQueryParam(ctx, "ano_letivo")
	esperado := []string{"2024_2025", "2025_2026", "2026_2027"}
	if !reflect.DeepEqual(valores, esperado) {
		t.Fatalf("valores inesperados. esperado=%v recebido=%v", esperado, valores)
	}
}

func TestParseFiltrosRegistros_AcademiaBloqueiaCodigoAcademiaDeTerceiros(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/notas?codigo_academia=ACAD_001&codigo_academia=ACAD_999", nil)

	filtros, err := parseFiltrosRegistros(ctx, "academia", "ACAD_001")
	if err != nil {
		t.Fatalf("não esperava erro, recebeu: %v", err)
	}
	if !filtros.forbidden {
		t.Fatalf("esperava forbidden=true quando academia consulta outro código")
	}
}

func TestBuildWhereSQL_NotasComCategoriaEMultiplosValores(t *testing.T) {
	filtros := filtrosRegistros{
		anoLectivos:     []string{"2025_2026", "2026_2027"},
		codigosAcademia: []string{"ACAD_001", "ACAD_002"},
		categorias:      []string{"p1", "p2"},
	}

	where, _ := filtros.buildWhereSQL("n", true)
	if !strings.Contains(where, "n.categoria = ANY") {
		t.Fatalf("esperava filtro por categoria no SQL. where=%s", where)
	}
	if !strings.Contains(where, "n.ano_lectivo = ANY") {
		t.Fatalf("esperava filtro multi-valor de ano_letivo no SQL. where=%s", where)
	}
	if !strings.Contains(where, "n.codigo_academia = ANY") {
		t.Fatalf("esperava filtro multi-valor de codigo_academia no SQL. where=%s", where)
	}
}

func TestParseFiltrosRegistros_FiltraCorrigidos(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/notas?corrigido=true", nil)

	filtros, err := parseFiltrosRegistros(ctx, "admin", "")
	if err != nil || filtros.corrigido == nil || !*filtros.corrigido {
		t.Fatalf("filtro corrigido=true invalido: filtros=%+v err=%v", filtros, err)
	}
	where, _ := filtros.buildWhereSQL("n", true)
	if !strings.Contains(where, "n.corrigido_em IS NOT NULL") {
		t.Fatalf("esperava filtro por registros corrigidos. where=%s", where)
	}
}

func TestParseFiltrosRegistrosEstudante_DeveIgnorarCategoriaQuandoNaoSuportado(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/faltas-estudante/ABC1234?categoria=p1&periodo=1_semestre,2_semestre", nil)

	filtros, err := parseFiltrosRegistrosEstudante(ctx, false)
	if err != nil {
		t.Fatalf("não esperava erro, recebeu: %v", err)
	}
	if len(filtros.categorias) != 0 {
		t.Fatalf("esperava categorias vazias para filtros sem categoria, recebeu=%v", filtros.categorias)
	}
	if !reflect.DeepEqual(filtros.periodos, []string{"1_semestre", "2_semestre"}) {
		t.Fatalf("periodos inesperados: %v", filtros.periodos)
	}
}
