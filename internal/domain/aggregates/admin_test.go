// ============================================================================
// ARQUIVO: internal/domain/aggregates/admin_test.go
// Testes para agregado Admin
// ============================================================================

package aggregates

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAdmin_Criar(t *testing.T) {
	t.Run("should create admin with valid role", func(t *testing.T) {
		admin := NewAdmin()

		err := admin.Criar(
			"Admin Teste",
			"admin@test.com",
			"hash",
			"gerente",
			nil,
		)

		assert.NoError(t, err)
		assert.Equal(t, "gerente", admin.Role)
		assert.Equal(t, "ativo", admin.Status)
	})

	t.Run("should fail with invalid role", func(t *testing.T) {
		admin := NewAdmin()

		err := admin.Criar(
			"Admin Teste",
			"admin@test.com",
			"hash",
			"invalido",
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "role deve ser")
	})

	t.Run("should fail without required fields", func(t *testing.T) {
		admin := NewAdmin()

		err := admin.Criar("", "", "", "fpp", nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "obrigatório")
	})
}

func TestAdmin_ValidatePermission(t *testing.T) {
	tests := []struct {
		name       string
		adminRole  string
		targetRole string
		shouldFail bool
	}{
		{"FPP can manage ADM", "fpp", "adm", false},
		{"FPP can manage GERENTE", "fpp", "gerente", false},
		{"ADM can manage GERENTE", "adm", "gerente", false},
		{"ADM cannot manage FPP", "adm", "fpp", true},
		{"GERENTE cannot manage ADM", "gerente", "adm", true},
		{"GERENTE cannot manage FPP", "gerente", "fpp", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admin := criarAdminBase(tt.adminRole)
			err := admin.ValidatePermission(tt.targetRole)

			if tt.shouldFail {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAdmin_AtualizarRole(t *testing.T) {
	t.Run("FPP can update role", func(t *testing.T) {
		admin := criarAdminBase("gerente")

		err := admin.AtualizarRole("adm", uuid.New(), "fpp")
		assert.NoError(t, err)
		assert.Equal(t, "adm", admin.Role)
	})

	t.Run("non-FPP cannot update role", func(t *testing.T) {
		admin := criarAdminBase("gerente")

		err := admin.AtualizarRole("adm", uuid.New(), "adm")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "apenas FPP")
	})

	t.Run("should fail with same role", func(t *testing.T) {
		admin := criarAdminBase("gerente")

		err := admin.AtualizarRole("gerente", uuid.New(), "fpp")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "já possui")
	})
}

func TestAdmin_Ativar_Desativar(t *testing.T) {
	t.Run("should deactivate active admin", func(t *testing.T) {
		admin := criarAdminBase("gerente")

		err := admin.Desativar(uuid.New(), "Teste")
		assert.NoError(t, err)
		assert.Equal(t, "inativo", admin.Status)
	})

	t.Run("should activate inactive admin", func(t *testing.T) {
		admin := criarAdminBase("gerente")
		admin.Desativar(uuid.New(), "Teste")
		admin.ClearUncommittedEvents()

		err := admin.Ativar(uuid.New())
		assert.NoError(t, err)
		assert.Equal(t, "ativo", admin.Status)
	})

	t.Run("inactive admin cannot perform actions", func(t *testing.T) {
		admin := criarAdminBase("gerente")
		admin.Desativar(uuid.New(), "Teste")
		admin.ClearUncommittedEvents()

		err := admin.ValidatePermission("gerente")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "inativo")
	})
}

func criarAdminBase(role string) *Admin {
	a := NewAdmin()
	a.Criar("Admin Teste", "admin@test.com", "hash", role, nil)
	a.ClearUncommittedEvents()
	return a
}