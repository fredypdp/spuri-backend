package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

func TestDocumentoExtraLifecycle(t *testing.T) {
	d := NewDocumentoExtra()

	// rotulo vazio
	if err := d.Criar("ACA", "", "pdf", true, "6_ano_fundamental", uuid.New()); err == nil {
		t.Fatal("rotulo vazio aceito")
	}
	// tipo invalido
	if err := d.Criar("ACA", "Foto 3x4", "png", true, "6_ano_fundamental", uuid.New()); err == nil {
		t.Fatal("tipo invalido aceito")
	}
	// ano_academico invalido
	if err := d.Criar("ACA", "Foto 3x4", "jpg", true, "99_ano_fundamental", uuid.New()); err == nil {
		t.Fatal("ano_academico invalido aceito")
	}
	// ano_academico em formato desconhecido
	if err := d.Criar("ACA", "Foto 3x4", "jpg", true, "fundamental", uuid.New()); err == nil {
		t.Fatal("ano_academico sem sufixo reconhecido aceito")
	}

	if err := d.Criar("ACA", " Foto 3x4 ", "JPG", true, "6_ano_fundamental", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if !d.Ativo || d.Rotulo != "Foto 3x4" || d.Tipo != "jpg" || d.Nivel != "fundamental" || d.AnoAcademico != "6_ano_fundamental" || !d.Obrigatorio {
		t.Fatalf("estado inesperado após Criar: %+v", d)
	}

	// nivel derivado corretamente para médio e superior também
	dMedio := NewDocumentoExtra()
	if err := dMedio.Criar("ACA", "Ficha médica", "pdf", false, "2_ano_medio", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if dMedio.Nivel != "medio" {
		t.Fatalf("nivel esperado 'medio', obtido %q", dMedio.Nivel)
	}
	dSuperior := NewDocumentoExtra()
	if err := dSuperior.Criar("ACA", "Comprovativo", "pdf", false, "1_ano_superior", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if dSuperior.Nivel != "superior" {
		t.Fatalf("nivel esperado 'superior', obtido %q", dSuperior.Nivel)
	}

	// Atualizar
	if err := d.Atualizar("", "pdf", false, "6_ano_fundamental", uuid.New()); err == nil {
		t.Fatal("rotulo vazio aceito em Atualizar")
	}
	if err := d.Atualizar("Foto tipo passe", "pdf", false, "7_ano_fundamental", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if d.Rotulo != "Foto tipo passe" || d.Tipo != "pdf" || d.Obrigatorio || d.AnoAcademico != "7_ano_fundamental" {
		t.Fatalf("estado inesperado após Atualizar: %+v", d)
	}

	// Desativar / Reativar
	if err := d.Desativar(uuid.New()); err != nil {
		t.Fatal(err)
	}
	if d.Ativo {
		t.Fatal("deveria estar inativo")
	}
	if err := d.Desativar(uuid.New()); err == nil {
		t.Fatal("dupla desativação aceita")
	}
	if err := d.Reativar(uuid.New()); err != nil {
		t.Fatal(err)
	}
	if !d.Ativo {
		t.Fatal("deveria estar ativo")
	}
	if err := d.Reativar(uuid.New()); err == nil {
		t.Fatal("dupla reativação aceita")
	}
}
