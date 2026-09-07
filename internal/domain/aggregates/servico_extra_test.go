package aggregates

import (
	"fmt"

	"github.com/google/uuid"
	"testing"
)

func TestServicoExtraValidation(t *testing.T) {
	id := uuid.New()
	s := NewServicoExtra()
	if e := s.Criar("A", "Transporte", "", nil, false, 0, "", nil, false, 0, nil, nil, nil, false, "", nil, id); e != nil {
		t.Fatal(e)
	}
	if !s.Ativo {
		t.Fatal("should start active")
	}
	gratuito := NewServicoExtra()
	if e := gratuito.Criar("A", "x", "", nil, false, 1, "", nil, false, 0, nil, nil, nil, false, "", nil, id); e != nil {
		t.Fatal(e)
	}
	if gratuito.Preco != 0 || gratuito.TipoCobranca != "" {
		t.Fatal("free service billing fields were not cleared")
	}
	if e := NewServicoExtra().Criar("A", "x", "", nil, true, 1, "anual", []string{"GPO"}, false, 0, nil, nil, nil, false, "", nil, id); e == nil {
		t.Fatal("invalid billing type accepted")
	}
	if e := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, true, 1, []string{"gpo"}, nil, nil, false, "", nil, id); e != nil {
		t.Fatal(e)
	}
	if e := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", []string{"PIX"}, false, 0, nil, nil, nil, false, "", nil, id); e == nil {
		t.Fatal("invalid method accepted")
	}
}
func TestServicoExtraDeactivate(t *testing.T) {
	s := NewServicoExtra()
	if e := s.Criar("A", "x", "", nil, true, 1, "mensal", []string{"GPO"}, false, 0, nil, nil, nil, false, "", nil, uuid.New()); e != nil {
		t.Fatal(e)
	}
	if e := s.Desativar(uuid.New()); e != nil {
		t.Fatal(e)
	}
	if e := s.Desativar(uuid.New()); e == nil {
		t.Fatal("second deactivate accepted")
	}
	if e := s.Reativar(uuid.New()); e != nil {
		t.Fatal(e)
	}
	if e := s.Reativar(uuid.New()); e == nil {
		t.Fatal("second reactivate accepted")
	}
}

func TestServicoExtraAtualizarDesligarPagoZeraCampos(t *testing.T) {
	s := NewServicoExtra()
	if err := s.Criar("A", "x", "", nil, true, 1, "mensal", []string{"GPO"}, false, 0, nil, nil, nil, false, "", nil, uuid.New()); err != nil {
		t.Fatal(err)
	}
	pago := false
	if err := s.Atualizar(nil, nil, nil, false, &pago, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if s.Pago || s.Preco != 0 || s.TipoCobranca != "" || len(s.MetodosPagamento) != 0 {
		t.Fatalf("campos de cobrança não foram zerados: %+v", s)
	}
}

func TestServicoExtraGratuitoComTaxaInscricaoValida(t *testing.T) {
	s := NewServicoExtra()
	if err := s.Criar("A", "x", "", nil, false, 0, "", nil, true, 1, []string{"GPO"}, nil, nil, false, "", nil, uuid.New()); err != nil {
		t.Fatalf("serviço gratuito com taxa válida foi rejeitado: %v", err)
	}
}

func TestServicoExtraCursosDisponiveisValidation(t *testing.T) {
	id := uuid.New()
	cursoID := uuid.New().String()
	if e := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, nil, []string{cursoID + "|2_ano_medio"}, false, "", nil, id); e != nil {
		t.Fatalf("entrada válida rejeitada: %v", e)
	}
	if e := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, []string{"6_ano_fundamental"}, []string{cursoID + "|3_ano_superior"}, false, "", nil, id); e != nil {
		t.Fatalf("combinação fundamental + curso rejeitada: %v", e)
	}
	if e := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, nil, []string{cursoID + "-2_ano_medio"}, false, "", nil, id); e == nil {
		t.Fatal("formato sem separador '|' foi aceito")
	}
	if e := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, nil, []string{"nao-e-uuid|2_ano_medio"}, false, "", nil, id); e == nil {
		t.Fatal("curso_id inválido foi aceito")
	}
	if e := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, nil, []string{cursoID + "|6_ano_fundamental"}, false, "", nil, id); e == nil {
		t.Fatal("ano fundamental escopado a curso foi aceito")
	}
	if e := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, []string{"2_ano_medio"}, nil, false, "", nil, id); e == nil {
		t.Fatal("ano médio solto em anos_academicos_disponiveis foi aceito (suporte legado deveria ter sido removido)")
	}
}

func TestServicoExtraDetalhesPersonalizadosTipados(t *testing.T) {
	transporte := map[string]DetalhePersonalizado{
		"rota":              {Rotulo: "Rota", Valor: "Centro", Tipo: "texto"},
		"ponto_de_embarque": {Rotulo: "Ponto de embarque", Valor: "Escola", Tipo: "texto"},
		"horario_de_saida":  {Rotulo: "Horário de saída", Valor: "06:30", Tipo: "hora"},
	}
	natacao := map[string]DetalhePersonalizado{
		"piscina":              {Rotulo: "Piscina", Valor: "Olímpica", Tipo: "texto"},
		"exige_saber_nadar":    {Rotulo: "Exige saber nadar", Valor: true, Tipo: "booleano"},
		"equipamento_incluido": {Rotulo: "Equipamento incluído", Valor: []interface{}{"touca", "óculos"}, Tipo: "lista_texto"},
	}
	for _, detalhes := range []map[string]DetalhePersonalizado{transporte, natacao} {
		if err := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, nil, nil, false, "", detalhes, uuid.New()); err != nil {
			t.Fatal(err)
		}
	}
	invalidos := []DetalhePersonalizado{{Rotulo: "x", Valor: 1.0, Tipo: "texto"}, {Rotulo: "x", Valor: "dez", Tipo: "numero"}, {Rotulo: "x", Valor: "sim", Tipo: "booleano"}, {Rotulo: "x", Valor: "01/01/2026", Tipo: "data"}, {Rotulo: "x", Valor: "25:00", Tipo: "hora"}, {Rotulo: "x", Valor: []interface{}{1.0}, Tipo: "lista_texto"}}
	for _, d := range invalidos {
		if err := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, nil, nil, false, "", map[string]DetalhePersonalizado{"campo": d}, uuid.New()); err == nil {
			t.Fatalf("invalid detail accepted: %+v", d)
		}
	}
	many := map[string]DetalhePersonalizado{}
	for i := 0; i < 31; i++ {
		many[fmt.Sprintf("campo_%d", i)] = DetalhePersonalizado{Rotulo: "x", Valor: "x", Tipo: "texto"}
	}
	if err := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, nil, nil, false, "", many, uuid.New()); err == nil {
		t.Fatal("more than 30 details accepted")
	}
	if err := NewServicoExtra().Criar("A", "x", "", nil, false, 0, "", nil, false, 0, nil, nil, nil, false, "", map[string]DetalhePersonalizado{"Campo invalido": {Rotulo: "x", Valor: "x", Tipo: "texto"}}, uuid.New()); err == nil {
		t.Fatal("invalid key accepted")
	}
}

func TestServicoExtraCategoriaServicoID(t *testing.T) {
	categoria := uuid.New()
	s := NewServicoExtra()
	if err := s.Criar("A", "x", "", &categoria, false, 0, "", nil, false, 0, nil, nil, nil, false, "", nil, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if s.CategoriaServicoID == nil || *s.CategoriaServicoID != categoria {
		t.Fatal("category ID not persisted")
	}
}
