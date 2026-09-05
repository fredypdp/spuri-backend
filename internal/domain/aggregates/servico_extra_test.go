package aggregates

import (
	"github.com/google/uuid"
	"testing"
)

func TestServicoExtraValidation(t *testing.T) {
	id := uuid.New()
	s := NewServicoExtra()
	if e := s.Criar("A", "Transporte", "", "", false, 0, "", nil, false, 0, nil, nil, false, "", nil, id); e != nil {
		t.Fatal(e)
	}
	if !s.Ativo {
		t.Fatal("should start active")
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 1, "", nil, false, 0, nil, nil, false, "", nil, id); e == nil {
		t.Fatal("free service with price accepted")
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", true, 1, "anual", []string{"GPO"}, false, 0, nil, nil, false, "", nil, id); e == nil {
		t.Fatal("invalid billing type accepted")
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", nil, true, 1, []string{"gpo"}, nil, false, "", nil, id); e != nil {
		t.Fatal(e)
	}
	if e := NewServicoExtra().Criar("A", "x", "", "", false, 0, "", []string{"PIX"}, false, 0, nil, nil, false, "", nil, id); e == nil {
		t.Fatal("invalid method accepted")
	}
}
func TestServicoExtraDeactivate(t *testing.T) {
	s := NewServicoExtra()
	if e := s.Criar("A", "x", "", "", true, 1, "mensal", []string{"GPO"}, false, 0, nil, nil, false, "", nil, uuid.New()); e != nil {
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
