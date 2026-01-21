// ============================================================================
// ARQUIVO: internal/domain/aggregates/curso_test.go
// Testes para agregado Curso
// ============================================================================

package aggregates

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurso_Criar(t *testing.T) {
	t.Run("should create curso medio successfully", func(t *testing.T) {
		curso := NewCurso()
		nivel := []string{"primeiro_medio", "segundo_medio"}

		err := curso.Criar(
			"Informática",
			"medio",
			nivel,
			"ACA2024",
		)

		assert.NoError(t, err)
		assert.Equal(t, "Informática", curso.Nome)
		assert.Equal(t, "medio", curso.Type)
		assert.Equal(t, "ativo", curso.Status)
	})

	t.Run("should create curso superior successfully", func(t *testing.T) {
		curso := NewCurso()
		nivel := []string{"primeiro_ano", "segundo_ano", "terceiro_ano"}

		err := curso.Criar(
			"Engenharia",
			"superior",
			nivel,
			"UNI2024",
		)

		assert.NoError(t, err)
		assert.Equal(t, "superior", curso.Type)
	})

	t.Run("should fail with invalid type", func(t *testing.T) {
		curso := NewCurso()
		nivel := []string{"primeiro_ano"}

		err := curso.Criar(
			"Curso Teste",
			"invalido",
			nivel,
			"ACA2024",
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tipo deve ser")
	})

	t.Run("should fail without nivel", func(t *testing.T) {
		curso := NewCurso()

		err := curso.Criar(
			"Curso Teste",
			"medio",
			[]string{},
			"ACA2024",
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nível é obrigatório")
	})

	t.Run("should fail with invalid nivel medio", func(t *testing.T) {
		curso := NewCurso()
		nivel := []string{"primeiro_ano"}

		err := curso.Criar(
			"Curso Teste",
			"medio",
			nivel,
			"ACA2024",
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inválido")
	})

	t.Run("should fail with invalid nivel superior", func(t *testing.T) {
		curso := NewCurso()
		nivel := []string{"primeiro_medio"}

		err := curso.Criar(
			"Curso Teste",
			"superior",
			nivel,
			"UNI2024",
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inválido")
	})

	t.Run("should fail without required fields", func(t *testing.T) {
		curso := NewCurso()

		err := curso.Criar("", "medio", []string{"primeiro_medio"}, "")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "obrigatório")
	})
}

func TestCurso_Ativar_Desativar(t *testing.T) {
	t.Run("should activate curso", func(t *testing.T) {
		curso := criarCursoBase()
		curso.Status = "inativo"
		curso.ClearUncommittedEvents()

		err := curso.Ativar()
		assert.NoError(t, err)
		assert.Equal(t, "ativo", curso.Status)
	})

	t.Run("should fail to activate already active", func(t *testing.T) {
		curso := criarCursoBase()

		err := curso.Ativar()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "já está ativo")
	})

	t.Run("should deactivate curso", func(t *testing.T) {
		curso := criarCursoBase()

		err := curso.Desativar()
		assert.NoError(t, err)
		assert.Equal(t, "inativo", curso.Status)
	})

	t.Run("should fail to deactivate already inactive", func(t *testing.T) {
		curso := criarCursoBase()
		curso.Desativar()
		curso.ClearUncommittedEvents()

		err := curso.Desativar()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "já está inativo")
	})
}

func TestCurso_AtualizarDados(t *testing.T) {
	t.Run("should update nome", func(t *testing.T) {
		curso := criarCursoBase()
		novoNome := "Informática Avançada"

		err := curso.AtualizarDados(&novoNome, nil, nil)
		assert.NoError(t, err)
	})

	t.Run("should update type and nivel", func(t *testing.T) {
		curso := criarCursoBase()
		novoType := "superior"
		novoNivel := []string{"primeiro_ano", "segundo_ano"}

		err := curso.AtualizarDados(nil, &novoType, novoNivel)
		assert.NoError(t, err)
	})

	t.Run("should fail with empty nome", func(t *testing.T) {
		curso := criarCursoBase()
		vazio := ""

		err := curso.AtualizarDados(&vazio, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "vazio")
	})

	t.Run("should fail without fields", func(t *testing.T) {
		curso := criarCursoBase()

		err := curso.AtualizarDados(nil, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nenhum campo")
	})

	t.Run("should fail when inactive", func(t *testing.T) {
		curso := criarCursoBase()
		curso.Desativar()
		curso.ClearUncommittedEvents()

		novoNome := "Teste"
		err := curso.AtualizarDados(&novoNome, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inativo")
	})

	t.Run("should validate nivel when updating type", func(t *testing.T) {
		curso := criarCursoBase()
		novoType := "medio"
		nivelInvalido := []string{"primeiro_ano"}

		err := curso.AtualizarDados(nil, &novoType, nivelInvalido)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inválido")
	})
}

func criarCursoBase() *Curso {
	c := NewCurso()
	nivel := []string{"primeiro_medio", "segundo_medio"}
	c.Criar("Informática", "medio", nivel, "ACA2024")
	c.ClearUncommittedEvents()
	return c
}