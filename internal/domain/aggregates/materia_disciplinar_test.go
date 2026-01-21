// ============================================================================
// ARQUIVO: internal/domain/aggregates/materia_disciplinar_test.go
// Testes para agregado MateriaDisciplinar
// ============================================================================

package aggregates

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestMateriaDisciplinar_Criar(t *testing.T) {
	t.Run("should create materia fundamental successfully", func(t *testing.T) {
		materia := NewMateriaDisciplinar()
		nivel := []string{"primeiro_fundamental", "segundo_fundamental"}

		err := materia.Criar(
			"Matemática",
			"fundamental",
			nivel,
			"ESC2024",
			nil,
		)

		assert.NoError(t, err)
		assert.Equal(t, "Matemática", materia.Nome)
		assert.Equal(t, "fundamental", materia.Type)
		assert.Equal(t, "ativo", materia.Status)
		assert.Nil(t, materia.CursoID)
	})

	t.Run("should create materia medio with curso", func(t *testing.T) {
		materia := NewMateriaDisciplinar()
		cursoID := uuid.New()

		err := materia.Criar(
			"Física",
			"medio",
			nil,
			"ESC2024",
			&cursoID,
		)

		assert.NoError(t, err)
		assert.Equal(t, "medio", materia.Type)
		assert.NotNil(t, materia.CursoID)
	})

	t.Run("should create materia superior with curso", func(t *testing.T) {
		materia := NewMateriaDisciplinar()
		cursoID := uuid.New()

		err := materia.Criar(
			"Cálculo I",
			"superior",
			nil,
			"UNI2024",
			&cursoID,
		)

		assert.NoError(t, err)
		assert.Equal(t, "superior", materia.Type)
	})

	t.Run("should fail fundamental with curso_id", func(t *testing.T) {
		materia := NewMateriaDisciplinar()
		cursoID := uuid.New()
		nivel := []string{"primeiro_fundamental"}

		err := materia.Criar(
			"Português",
			"fundamental",
			nivel,
			"ESC2024",
			&cursoID,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "não podem ter curso")
	})

	t.Run("should fail medio without curso_id", func(t *testing.T) {
		materia := NewMateriaDisciplinar()

		err := materia.Criar(
			"Química",
			"medio",
			nil,
			"ESC2024",
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "devem ter curso")
	})

	t.Run("should fail superior without curso_id", func(t *testing.T) {
		materia := NewMateriaDisciplinar()

		err := materia.Criar(
			"Álgebra",
			"superior",
			nil,
			"UNI2024",
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "devem ter curso")
	})

	t.Run("should fail fundamental without nivel", func(t *testing.T) {
		materia := NewMateriaDisciplinar()

		err := materia.Criar(
			"História",
			"fundamental",
			[]string{},
			"ESC2024",
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nível definido")
	})

	t.Run("should fail with invalid type", func(t *testing.T) {
		materia := NewMateriaDisciplinar()

		err := materia.Criar(
			"Matéria",
			"invalido",
			nil,
			"ESC2024",
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tipo deve ser")
	})

	t.Run("should fail with invalid nivel fundamental", func(t *testing.T) {
		materia := NewMateriaDisciplinar()
		nivel := []string{"primeiro_medio"}

		err := materia.Criar(
			"Geografia",
			"fundamental",
			nivel,
			"ESC2024",
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inválido")
	})

	t.Run("should fail without required fields", func(t *testing.T) {
		materia := NewMateriaDisciplinar()

		err := materia.Criar("", "fundamental", nil, "", nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "obrigatório")
	})
}

func TestMateriaDisciplinar_Ativar_Desativar(t *testing.T) {
	t.Run("should activate materia", func(t *testing.T) {
		materia := criarMateriaBase()
		materia.Status = "inativo"
		materia.ClearUncommittedEvents()

		err := materia.Ativar()
		assert.NoError(t, err)
		assert.Equal(t, "ativo", materia.Status)
	})

	t.Run("should fail to activate already active", func(t *testing.T) {
		materia := criarMateriaBase()

		err := materia.Ativar()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "já está ativa")
	})

	t.Run("should deactivate materia", func(t *testing.T) {
		materia := criarMateriaBase()

		err := materia.Desativar()
		assert.NoError(t, err)
		assert.Equal(t, "inativo", materia.Status)
	})

	t.Run("should fail to deactivate already inactive", func(t *testing.T) {
		materia := criarMateriaBase()
		materia.Desativar()
		materia.ClearUncommittedEvents()

		err := materia.Desativar()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "já está inativa")
	})
}

func TestMateriaDisciplinar_AtualizarDados(t *testing.T) {
	t.Run("should update nome", func(t *testing.T) {
		materia := criarMateriaBase()
		novoNome := "Matemática Avançada"

		err := materia.AtualizarDados(&novoNome, nil)
		assert.NoError(t, err)
	})

	t.Run("should update type", func(t *testing.T) {
		materia := criarMateriaBase()
		novoType := "medio"

		err := materia.AtualizarDados(nil, &novoType)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sem curso")
	})

	t.Run("should fail with empty nome", func(t *testing.T) {
		materia := criarMateriaBase()
		vazio := ""

		err := materia.AtualizarDados(&vazio, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "vazio")
	})

	t.Run("should fail without fields", func(t *testing.T) {
		materia := criarMateriaBase()

		err := materia.AtualizarDados(nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nenhum campo")
	})

	t.Run("should fail when inactive", func(t *testing.T) {
		materia := criarMateriaBase()
		materia.Desativar()
		materia.ClearUncommittedEvents()

		novoNome := "Teste"
		err := materia.AtualizarDados(&novoNome, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inativa")
	})

	t.Run("should fail changing fundamental to medio without curso", func(t *testing.T) {
		materia := criarMateriaBase()
		novoType := "medio"

		err := materia.AtualizarDados(nil, &novoType)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sem curso")
	})
}

func criarMateriaBase() *MateriaDisciplinar {
	m := NewMateriaDisciplinar()
	nivel := []string{"primeiro_fundamental", "segundo_fundamental"}
	m.Criar("Matemática", "fundamental", nivel, "ESC2024", nil)
	m.ClearUncommittedEvents()
	return m
}