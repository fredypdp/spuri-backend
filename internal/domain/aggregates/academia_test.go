// ============================================================================
// ARQUIVO: internal/domain/aggregates/academia_test.go
// Testes para agregado Academia
// ============================================================================

package aggregates

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAcademia_Criar(t *testing.T) {
	t.Run("should create escola with nivel_escolar", func(t *testing.T) {
		academia := NewAcademia()
		nivel := "medio"

		err := academia.Criar(
			"escola",
			"Escola Teste",
			"LUA20241234",
			"hash",
			"LUA",
			"Rua A, 123",
			nil, nil, nil,
			&nivel,
			[]string{"Matemática", "Português"},
		)

		assert.NoError(t, err)
		assert.Equal(t, "escola", academia.Type)
		assert.Equal(t, "inativo", academia.Status)
		assert.Equal(t, &nivel, academia.NivelEscolar)
	})

	t.Run("should fail escola without nivel_escolar", func(t *testing.T) {
		academia := NewAcademia()

		err := academia.Criar(
			"escola",
			"Escola Teste",
			"LUA20241234",
			"hash",
			"LUA",
			"Rua A, 123",
			nil, nil, nil,
			nil,
			[]string{},
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nivel_escolar é obrigatório")
	})

	t.Run("should fail with invalid nivel_escolar", func(t *testing.T) {
		academia := NewAcademia()
		nivel := "invalido"

		err := academia.Criar(
			"escola",
			"Escola Teste",
			"LUA20241234",
			"hash",
			"LUA",
			"Rua A, 123",
			nil, nil, nil,
			&nivel,
			[]string{},
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "fundamental", "medio", "misto")
	})

	t.Run("should create universidade without nivel_escolar", func(t *testing.T) {
		academia := NewAcademia()

		err := academia.Criar(
			"superior",
			"Universidade Teste",
			"LUA20241234",
			"hash",
			"LUA",
			"Rua A, 123",
			nil, nil, nil,
			nil,
			[]string{"Engenharia", "Medicina"},
		)

		assert.NoError(t, err)
		assert.Equal(t, "superior", academia.Type)
		assert.Nil(t, academia.NivelEscolar)
	})

	t.Run("should fail with invalid type", func(t *testing.T) {
		academia := NewAcademia()

		err := academia.Criar(
			"invalido",
			"Academia",
			"LUA20241234",
			"hash",
			"LUA",
			"Rua A",
			nil, nil, nil, nil, []string{},
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tipo deve ser")
	})
}

func TestAcademia_Ativar_Desativar(t *testing.T) {
	t.Run("should activate inactive academia", func(t *testing.T) {
		academia := criarAcademiaBase()
		assert.Equal(t, "inativo", academia.Status)

		err := academia.Ativar()
		assert.NoError(t, err)
		assert.Equal(t, "ativo", academia.Status)
	})

	t.Run("should fail to activate already active", func(t *testing.T) {
		academia := criarAcademiaBase()
		academia.Ativar()
		academia.ClearUncommittedEvents()

		err := academia.Ativar()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "já está ativa")
	})

	t.Run("should deactivate active academia", func(t *testing.T) {
		academia := criarAcademiaBase()
		academia.Ativar()
		academia.ClearUncommittedEvents()

		err := academia.Desativar("Teste")
		assert.NoError(t, err)
		assert.Equal(t, "inativo", academia.Status)
	})
}

func TestAcademia_AprovarInscricao(t *testing.T) {
	t.Run("should approve when active", func(t *testing.T) {
		academia := criarAcademiaBase()
		academia.Ativar()
		academia.ClearUncommittedEvents()

		estudanteID := uuid.New()
		inscricaoID := uuid.New()

		err := academia.AprovarInscricao(
			estudanteID,
			inscricaoID,
			"escola",
			"2024",
			nil,
		)

		assert.NoError(t, err)
	})

	t.Run("should fail when inactive", func(t *testing.T) {
		academia := criarAcademiaBase()

		err := academia.AprovarInscricao(
			uuid.New(),
			uuid.New(),
			"escola",
			"2024",
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inativa")
	})
}

func TestAcademia_AtualizarDados(t *testing.T) {
	t.Run("should update nome", func(t *testing.T) {
		academia := criarAcademiaBase()
		novoNome := "Escola Atualizada"

		err := academia.AtualizarDados(
			&novoNome,
			nil, nil, nil, nil, nil, nil, nil,
		)

		assert.NoError(t, err)
	})

	t.Run("should fail with empty nome", func(t *testing.T) {
		academia := criarAcademiaBase()
		vazio := ""

		err := academia.AtualizarDados(
			&vazio,
			nil, nil, nil, nil, nil, nil, nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "vazio")
	})

	t.Run("should fail without fields", func(t *testing.T) {
		academia := criarAcademiaBase()

		err := academia.AtualizarDados(
			nil, nil, nil, nil, nil, nil, nil, nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nenhum campo")
	})
}

func criarAcademiaBase() *Academia {
	a := NewAcademia()
	nivel := "medio"
	a.Criar(
		"escola",
		"Escola Teste",
		"LUA20241234",
		"hash",
		"LUA",
		"Rua A, 123",
		nil, nil, nil,
		&nivel,
		[]string{},
	)
	a.ClearUncommittedEvents()
	return a
}