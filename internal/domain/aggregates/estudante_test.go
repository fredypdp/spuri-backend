// ============================================================================
// ARQUIVO: internal/domain/aggregates/estudante_test.go
// Testes para agregado Estudante
// ============================================================================

package aggregates

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestEstudante_Criar(t *testing.T) {
	t.Run("should create estudante successfully", func(t *testing.T) {
		estudante := NewEstudante()
		bilhete := "123456789LA"
		status := "inativo"

		err := estudante.Criar(
			"João Silva",
			"ABC1234",
			"hashed_password",
			nil, nil,
			&bilhete, nil,
			nil, nil, nil, nil,
			&status, nil,
		)

		assert.NoError(t, err)
		assert.Equal(t, "João Silva", estudante.Nome)
		assert.Equal(t, "ABC1234", estudante.CodigoEstudante)
		assert.Equal(t, "inativo", estudante.Status)
		assert.Equal(t, 1, len(estudante.GetUncommittedEvents()))
	})

	t.Run("should fail without required fields", func(t *testing.T) {
		estudante := NewEstudante()

		err := estudante.Criar("", "", "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "obrigatório")
	})

	t.Run("should fail without bilhetes", func(t *testing.T) {
		estudante := NewEstudante()

		err := estudante.Criar(
			"João Silva", "ABC1234", "hash",
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "bilhete")
	})

	t.Run("should validate status transitions", func(t *testing.T) {
		estudante := NewEstudante()
		bilhete := "123456789LA"
		statusEscolar := "finalizado"
		statusSuperior := "em_andamento"

		err := estudante.Criar(
			"João", "ABC1234", "hash",
			nil, nil, &bilhete, nil,
			nil, nil, nil, nil,
			&statusEscolar, &statusSuperior,
		)

		assert.NoError(t, err)
	})
}

func TestEstudante_RegistrarNota(t *testing.T) {
	estudante := criarEstudanteBase()
	academia := "ACA2024"
	estudante.CodigoAcademia = &academia

	t.Run("should register nota successfully", func(t *testing.T) {
		materiaID := uuid.New()
		err := estudante.RegistrarNota(
			academia,
			"2024",
			"1_trimestre",
			materiaID,
			15.5,
			nil,
		)

		assert.NoError(t, err)
	})

	t.Run("should fail with invalid academia", func(t *testing.T) {
		materiaID := uuid.New()
		err := estudante.RegistrarNota(
			"OUTRA",
			"2024",
			"1_trimestre",
			materiaID,
			15.5,
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "não pertence")
	})

	t.Run("should fail with invalid nota", func(t *testing.T) {
		materiaID := uuid.New()
		err := estudante.RegistrarNota(
			academia,
			"2024",
			"1_trimestre",
			materiaID,
			25.0,
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "entre 0 e 20")
	})

	t.Run("should fail with invalid periodo", func(t *testing.T) {
		materiaID := uuid.New()
		err := estudante.RegistrarNota(
			academia,
			"2024",
			"trimestre_invalido",
			materiaID,
			15.0,
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "período inválido")
	})
}

func TestEstudante_RegistrarFalta(t *testing.T) {
	estudante := criarEstudanteBase()
	academia := "ACA2024"
	estudante.CodigoAcademia = &academia

	t.Run("should register falta successfully", func(t *testing.T) {
		materiaID := uuid.New()
		err := estudante.RegistrarFalta(
			academia,
			"2024",
			time.Now(),
			materiaID,
			1,
			nil,
		)

		assert.NoError(t, err)
	})

	t.Run("should fail with invalid quantidade", func(t *testing.T) {
		materiaID := uuid.New()
		err := estudante.RegistrarFalta(
			academia,
			"2024",
			time.Now(),
			materiaID,
			0,
			nil,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "maior que zero")
	})
}

func TestEstudante_VincularAcademia(t *testing.T) {
	estudante := criarEstudanteBase()
	inscricaoID := uuid.New()

	// Adicionar inscrição aprovada
	estudante.Inscricoes = []Inscricao{
		{
			ID:             inscricaoID,
			CodigoAcademia: "ACA2024",
			Status:         "aprovado",
			StatusUsado:    false,
		},
	}

	t.Run("should vincular successfully", func(t *testing.T) {
		err := estudante.VincularAcademia(inscricaoID)
		assert.NoError(t, err)
		assert.Equal(t, "ativo", estudante.Status)
	})

	t.Run("should fail if already used", func(t *testing.T) {
		estudante2 := criarEstudanteBase()
		estudante2.Inscricoes = []Inscricao{
			{
				ID:          inscricaoID,
				Status:      "aprovado",
				StatusUsado: true,
			},
		}

		err := estudante2.VincularAcademia(inscricaoID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "utilizada")
	})

	t.Run("should fail if not approved", func(t *testing.T) {
		estudante3 := criarEstudanteBase()
		pendingID := uuid.New()
		estudante3.Inscricoes = []Inscricao{
			{
				ID:     pendingID,
				Status: "espera",
			},
		}

		err := estudante3.VincularAcademia(pendingID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "não foi aprovada")
	})
}

func criarEstudanteBase() *Estudante {
	e := NewEstudante()
	bilhete := "123456789LA"
	e.Criar(
		"João Silva",
		"ABC1234",
		"hash",
		nil, nil, &bilhete, nil,
		nil, nil, nil, nil, nil, nil,
	)
	e.ClearUncommittedEvents()
	return e
}