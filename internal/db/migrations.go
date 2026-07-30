package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

const (
	migrationsDir                   = "migrations"
	migrationsAdvisoryLockKey int64 = 20260730011743
)

// loadMigrations lê o diretório de migrations e retorna os caminhos
// ordenados por nome de arquivo (ordem numérica 001_, 002_, ...).
// Apenas arquivos .sql são incluídos.
func loadMigrations(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler diretório de migrations '%s': %w", dir, err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}

	// os.ReadDir já retorna em ordem alfabética na maioria dos sistemas,
	// mas ordenamos explicitamente para garantir consistência em qualquer OS.
	sort.Strings(paths)

	return paths, nil
}

func (c *Client) RunMigrations() error {
	log.Println("🔍 Verificando migrations...")

	ctx := context.Background()
	conn, unlock, err := c.acquireMigrationsLock(ctx)
	if err != nil {
		return fmt.Errorf("erro ao obter lock de migrations: %w", err)
	}
	defer unlock()

	if err := c.ensureMigrationsTable(ctx, conn); err != nil {
		return fmt.Errorf("erro ao criar tabela schema_migrations: %w", err)
	}

	migrations, err := loadMigrations(migrationsDir)
	if err != nil {
		return fmt.Errorf("erro ao carregar migrations: %w", err)
	}

	if len(migrations) == 0 {
		log.Printf("⚠️  Nenhuma migration encontrada em '%s'", migrationsDir)
		return nil
	}

	log.Printf("📂 %d migration(s) encontrada(s) em '%s'", len(migrations), migrationsDir)

	applied := 0
	for _, path := range migrations {
		name := filepath.Base(path)

		done, err := c.isMigrationApplied(ctx, conn, name)
		if err != nil {
			return fmt.Errorf("erro ao verificar %s: %w", name, err)
		}
		if done {
			log.Printf("✅ Já aplicada: %s", name)
			continue
		}

		log.Printf("📄 Aplicando: %s", name)
		if err := c.runMigrationFile(ctx, conn, path); err != nil {
			return fmt.Errorf("erro na migration %s: %w", name, err)
		}

		if err := c.markMigrationApplied(ctx, conn, name); err != nil {
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
func (c *Client) ensureMigrationsTable(ctx context.Context, conn *sqlx.Conn) error {
	_, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename   VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

func (c *Client) acquireMigrationsLock(ctx context.Context) (*sqlx.Conn, func(), error) {
	conn, err := c.db.Connx(ctx)
	if err != nil {
		return nil, nil, err
	}

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationsAdvisoryLockKey); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	log.Println("🔒 Lock de migrations obtido")
	return conn, func() {
		if _, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationsAdvisoryLockKey); err != nil {
			log.Printf("⚠️ Erro ao liberar lock de migrations: %v", err)
		}
		if err := conn.Close(); err != nil {
			log.Printf("⚠️ Erro ao fechar conexão do lock de migrations: %v", err)
		}
	}, nil
}

func (c *Client) isMigrationApplied(ctx context.Context, conn *sqlx.Conn, filename string) (bool, error) {
	var exists bool
	err := conn.QueryRowxContext(
		ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, filename,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		log.Printf("⚠️ Consulta de migration %s não retornou linha; tratando como não aplicada", filename)
		return false, nil
	}
	return exists, err
}

func (c *Client) markMigrationApplied(ctx context.Context, conn *sqlx.Conn, filename string) error {
	_, err := conn.ExecContext(ctx,
		`INSERT INTO schema_migrations (filename) VALUES ($1) ON CONFLICT DO NOTHING`, filename,
	)
	return err
}

// runMigrationFile executa o conteúdo de um arquivo SQL.
//
// FIX DB-12: migrations sem BEGIN/COMMIT explícito são envolvidas em transação
// automática. Arquivos que já têm BEGIN/COMMIT próprio são executados sem
// interferência.
func (c *Client) runMigrationFile(ctx context.Context, conn *sqlx.Conn, filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo: %w", err)
	}

	sql := string(content)

	if migrationNeedsWrapper(sql) {
		log.Printf("[DEBUG] %s: sem BEGIN/COMMIT explícito — envolvendo em transação automática",
			filepath.Base(filename))
		sql = "BEGIN;\n" + sql + "\nCOMMIT;\n"
	}

	_, err = conn.ExecContext(ctx, sql)
	return err
}

// migrationNeedsWrapper retorna true se o SQL não tem BEGIN explícito.
// Ignora linhas vazias, comentários de linha (--) e comentários de bloco (/* */).
func migrationNeedsWrapper(content string) bool {
	lines := strings.Split(content, "\n")
	inBlockComment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "/*") {
			inBlockComment = true
		}
		if inBlockComment {
			if strings.Contains(trimmed, "*/") {
				inBlockComment = false
			}
			continue
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		// Primeira instrução real encontrada
		return !strings.HasPrefix(strings.ToUpper(trimmed), "BEGIN")
	}

	return true
}

func (c *Client) tableExists(tableName string) (bool, error) {
	if err := ValidateTableName(tableName); err != nil {
		return false, err
	}
	query := `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = $1`
	var count int
	err := c.db.QueryRow(query, tableName).Scan(&count)
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
