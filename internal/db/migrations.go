package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// allMigrations define a ordem exata de execução.
// Todas são idempotentes (IF NOT EXISTS / IF EXISTS).
var allMigrations = []string{
	"migrations/001_complete_schema.sql",
	"migrations/002_add_email_verificado_safe.sql",
	"migrations/003_add_aprovacao_ano.sql",
	"migrations/004_cursos_uuid.sql",
	"migrations/005_add_sistema_config.sql",
	"migrations/006_add_tipo_categoria_notas.sql",
	"migrations/007_add_turmas_genero.sql",
	"migrations/008_status_escolar_split_aprovacao.sql",
	"migrations/009_ano_escolar_medio_reprovacoes.sql",
	"migrations/010_academia_anos_academicos.sql",
	"migrations/011_cursos_nivel_to_anos_academicos.sql",
	"migrations/012_ano_academico.sql",
	"migrations/013_anos_academicos_materia.sql",
	"migrations/014_materias_nivel_to_anos_academicos.sql",
	"migrayions/015_add_periodos_to_cursos.sql",
}

func (c *Client) RunMigrations() error {
	log.Println("🔍 Verificando migrations...")

	if err := c.ensureMigrationsTable(); err != nil {
		return fmt.Errorf("erro ao criar tabela schema_migrations: %w", err)
	}

	applied := 0
	for _, path := range allMigrations {
		name := filepath.Base(path)

		done, err := c.isMigrationApplied(name)
		if err != nil {
			return fmt.Errorf("erro ao verificar %s: %w", name, err)
		}
		if done {
			log.Printf("✅ Já aplicada: %s", name)
			continue
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			log.Printf("⚠️  Arquivo não encontrado, pulando: %s", name)
			continue
		}

		log.Printf("📄 Aplicando: %s", name)
		if err := c.runMigrationFile(path); err != nil {
			return fmt.Errorf("erro na migration %s: %w", name, err)
		}

		if err := c.markMigrationApplied(name); err != nil {
			return fmt.Errorf("erro ao registrar %s: %w", name, err)
		}

		log.Printf("✅ Migration aplicada: %s", name)
		applied++
	}

	if applied == 0 {
		log.Println("✅ Todas as migrations já estavam aplicadas")
	} else {
		log.Printf("🎉 %d migration(s) aplicada(s) com sucesso!", applied)
	}

	c.logStats()
	return nil
}

// ensureMigrationsTable cria a tabela de controle se não existir.
func (c *Client) ensureMigrationsTable() error {
	_, err := c.db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (c *Client) isMigrationApplied(filename string) (bool, error) {
	var count int
	err := c.db.QueryRow(
		`SELECT COUNT(*) FROM schema_migrations WHERE filename = $1`, filename,
	).Scan(&count)
	return count > 0, err
}

func (c *Client) markMigrationApplied(filename string) error {
	_, err := c.db.Exec(
		`INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING`, filename,
	)
	return err
}

func (c *Client) runMigrationFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo: %w", err)
	}
	ctx := context.Background()
	_, err = c.db.ExecContext(ctx, string(content))
	return err
}

func (c *Client) tableExists(tableName string) (bool, error) {
	if err := ValidateTableName(tableName); err != nil {
		return false, err
	}
	safeName := SafeString(tableName)
	query := fmt.Sprintf(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = '%s'`, safeName)
	var count int
	err := c.db.QueryRow(query).Scan(&count)
	return count > 0, err
}

func (c *Client) logStats() {
	ctx := context.Background()
	var eventCount, estudanteCount, academiaCount int64
	c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM spuri_ledger").Scan(&eventCount)
	c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM projection_estudantes").Scan(&estudanteCount)
	c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM projection_academias").Scan(&academiaCount)
	log.Println("\n📊 Estatísticas do Banco:")
	log.Printf("   Eventos no ledger: %d", eventCount)
	log.Printf("   Estudantes: %d", estudanteCount)
	log.Printf("   Academias: %d", academiaCount)
}