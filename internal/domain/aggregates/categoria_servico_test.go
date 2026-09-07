package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

func TestCategoriaServicoLifecycle(t *testing.T) {
	c := NewCategoriaServico()
	if err := c.Criar("ACA", "", uuid.New()); err == nil {
		t.Fatal("empty name accepted")
	}
	if err := c.Criar("ACA", " Transporte ", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if !c.Ativo || c.Nome != "Transporte" {
		t.Fatalf("unexpected category: %+v", c)
	}
	if err := c.Renomear("", uuid.New()); err == nil {
		t.Fatal("empty rename accepted")
	}
	if err := c.Renomear("Dança", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := c.Desativar(uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := c.Desativar(uuid.New()); err == nil {
		t.Fatal("duplicate deactivation accepted")
	}
	if err := c.Reativar(uuid.New()); err != nil {
		t.Fatal(err)
	}
	if err := c.Reativar(uuid.New()); err == nil {
		t.Fatal("duplicate reactivation accepted")
	}
}
