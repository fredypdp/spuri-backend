// ============================================================================
// ARQUIVO: internal/db/repository_test.go
// Testes de integração para Repository
// ============================================================================

package db

import (
	"os"
	"testing"

	"spuri/internal/domain/aggregates"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*Client, func()) {
	// Usar variáveis de teste ou criar DB temporário
	config := &Config{
		Host:     getEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:     getEnvOrDefault("TEST_DB_PORT", "5432"),
		User:     getEnvOrDefault("TEST_DB_USER", "postgres"),
		Password: getEnvOrDefault("TEST_DB_PASSWORD", "postgres"),
		DBName:   getEnvOrDefault("TEST_DB_NAME", "spuri_test"),
		SSLMode:  "disable",
	}

	client, err := NewClient(config)
	require.NoError(t, err, "Falha ao conectar ao banco de teste")

	cleanup := func() {
		// Limpar dados de teste
		client.DB().Exec("TRUNCATE TABLE spuri_ledger CASCADE")
		client.Close()
	}

	return client, cleanup
}

func TestRepository_SaveAndLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Pulando teste de integração")
	}

	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewAggregateRepository(client)

	t.Run("should save and load estudante", func(t *testing.T) {
		// Criar estudante
		estudante := aggregates.NewEstudante()
		bilhete := "123456789LA"
		
		err := estudante.Criar(
			"João Teste",
			"TST1234",
			"hash",
			nil, nil, &bilhete, nil,
			nil, nil, nil, nil, nil, nil,
		)
		require.NoError(t, err)

		// Salvar
		err = repo.Save(estudante)
		require.NoError(t, err)

		// Carregar
		loaded, err := repo.Load(estudante.ID, "Estudante")
		require.NoError(t, err)
		
		loadedEstudante := loaded.(*aggregates.Estudante)
		assert.Equal(t, estudante.Nome, loadedEstudante.Nome)
		assert.Equal(t, estudante.CodigoEstudante, loadedEstudante.CodigoEstudante)
	})

	t.Run("should maintain event version", func(t *testing.T) {
		estudante := aggregates.NewEstudante()
		bilhete := "987654321LA"
		
		err := estudante.Criar(
			"Maria Teste",
			"TST5678",
			"hash",
			nil, nil, &bilhete, nil,
			nil, nil, nil, nil, nil, nil,
		)
		require.NoError(t, err)

		// Salvar primeira vez
		err = repo.Save(estudante)
		require.NoError(t, err)

		// Adicionar novo evento
		estudante.ClearUncommittedEvents()
		status := "finalizado"
		estudante.AtualizarStatusEscolar(status)

		// Salvar segunda vez
		err = repo.Save(estudante)
		require.NoError(t, err)

		// Carregar e verificar
		loaded, err := repo.Load(estudante.ID, "Estudante")
		require.NoError(t, err)
		
		assert.Equal(t, 2, loaded.GetVersion())
	})
}

func TestRepository_VerifyIntegrity(t *testing.T) {
	if testing.Short() {
		t.Skip("Pulando teste de integração")
	}

	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewAggregateRepository(client)

	t.Run("should verify hash chain integrity", func(t *testing.T) {
		estudante := aggregates.NewEstudante()
		bilhete := "111222333LA"
		
		err := estudante.Criar(
			"Pedro Teste",
			"TST9999",
			"hash",
			nil, nil, &bilhete, nil,
			nil, nil, nil, nil, nil, nil,
		)
		require.NoError(t, err)

		err = repo.Save(estudante)
		require.NoError(t, err)

		// Verificar integridade
		valid, err := repo.VerifyIntegrity(estudante.ID)
		require.NoError(t, err)
		assert.True(t, valid, "Hash chain deve estar íntegro")
	})
}

func TestRepository_Exists(t *testing.T) {
	if testing.Short() {
		t.Skip("Pulando teste de integração")
	}

	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewAggregateRepository(client)

	t.Run("should return true for existing aggregate", func(t *testing.T) {
		estudante := aggregates.NewEstudante()
		bilhete := "444555666LA"
		
		err := estudante.Criar(
			"Ana Teste",
			"TST7777",
			"hash",
			nil, nil, &bilhete, nil,
			nil, nil, nil, nil, nil, nil,
		)
		require.NoError(t, err)

		err = repo.Save(estudante)
		require.NoError(t, err)

		exists, err := repo.Exists(estudante.ID)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("should return false for non-existing aggregate", func(t *testing.T) {
		randomID := uuid.New()

		exists, err := repo.Exists(randomID)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}