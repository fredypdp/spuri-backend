package aggregates

import (
	"github.com/google/uuid"
	"testing"
)

func TestServicoExtraValidation(t *testing.T) {
	id := uuid.New()
	s := NewServicoExtra()
	if e := s.Criar("A", "Transporte", "", "", false, 0, "", nil, false, 0, nil, nil, nil, false, "", nil, id); e != nil {
		t.Fatal(e)
	}
	if !s.Ativo {
		t.Fatal("should start active")
	}
	gratuito := NewServicoExtra()
	if e := gratuito.Criar("A", "x", "", "", false, 1, "", nil, false, 0, nil, nil, nil, false, "", nil, id); e != nil {
		t.Fatal(e)
	}
	if gratuito.Preco != 0 || gratuito.TipoCobranca != "" {
		t.Fatal("free service billing fields were not cleared")
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", true, 1, "anual", []string{"GPO"}, false, 0, nil, nil, nil, false, "", nil, id); e == nil {
		t.Fatal("invalid billing type accepted")
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, true, 1, []string{"gpo"}, nil, nil, false, "", nil, id); e != nil {
		t.Fatal(e)
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", []string{"PIX"}, false, 0, nil, nil, nil, false, "", nil, id); e == nil {
		t.Fatal("invalid method accepted")
	}
}
func TestServicoExtraDeactivate(t *testing.T) {
	s := NewServicoExtra()
	if e := s.Criar("A", "x", "", "", true, 1, "mensal", []string{"GPO"}, false, 0, nil, nil, nil, false, "", nil, uuid.New()); e != nil {
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
	if err := s.Criar("A", "x", "", "", true, 1, "mensal", []string{"GPO"}, false, 0, nil, nil, nil, false, "", nil, uuid.New()); err != nil {
		t.Fatal(err)
	}
	pago := false
	if err := s.Atualizar(nil, nil, nil, &pago, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, uuid.New()); err != nil {
		t.Fatal(err)
	}
	if s.Pago || s.Preco != 0 || s.TipoCobranca != "" || len(s.MetodosPagamento) != 0 {
		t.Fatalf("campos de cobrança não foram zerados: %+v", s)
	}
}

func TestServicoExtraGratuitoComTaxaInscricaoValida(t *testing.T) {
	s := NewServicoExtra()
	if err := s.Criar("A", "x", "", "", false, 0, "", nil, true, 1, []string{"GPO"}, nil, nil, false, "", nil, uuid.New()); err != nil {
		t.Fatalf("serviço gratuito com taxa válida foi rejeitado: %v", err)
	}
}

func TestServicoExtraCursosDisponiveisValidation(t *testing.T) {
	id := uuid.New()
	cursoID := uuid.New().String()
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil, []string{cursoID + "|2_ano_medio"}, false, "", nil, id); e != nil {
		t.Fatalf("entrada válida rejeitada: %v", e)
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, []string{"6_ano_fundamental"}, []string{cursoID + "|3_ano_superior"}, false, "", nil, id); e != nil {
		t.Fatalf("combinação fundamental + curso rejeitada: %v", e)
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil, []string{cursoID + "-2_ano_medio"}, false, "", nil, id); e == nil {
		t.Fatal("formato sem separador '|' foi aceito")
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil, []string{"nao-e-uuid|2_ano_medio"}, false, "", nil, id); e == nil {
		t.Fatal("curso_id inválido foi aceito")
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, nil, []string{cursoID + "|6_ano_fundamental"}, false, "", nil, id); e == nil {
		t.Fatal("ano fundamental escopado a curso foi aceito")
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, false, 0, nil, []string{"2_ano_medio"}, nil, false, "", nil, id); e == nil {
		t.Fatal("ano médio solto em anos_academicos_disponiveis foi aceito (suporte legado deveria ter sido removido)")
	}
}
