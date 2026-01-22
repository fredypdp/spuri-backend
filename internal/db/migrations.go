package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func (c *Client) RunMigrations() error {
	log.Println("🔍 Verificando migrations...")

	exists, err := c.tableExists("spuri_ledger")
	if err != nil {
		return fmt.Errorf("erro ao verificar tabelas: %w", err)
	}

	if exists {
		log.Println("✅ Migrations já aplicadas - Tabelas existem")
		c.logStats()
		return nil
	}

	log.Println("🆕 Primeira execução - Aplicando migrations...")

	if err := c.runMigrationFile("migrations/001_complete_schema.sql"); err != nil {
		return fmt.Errorf("erro na migration 1: %w", err)
	}

	log.Println("🎉 Todas as migrations aplicadas com sucesso!")
	return nil
}

// ✅ SAFE: String validada
func (c *Client) tableExists(tableName string) (bool, error) {
	// Validar nome da tabela
	if err := ValidateTableName(tableName); err != nil {
		return false, err
	}

	safeName := SafeString(tableName)

	query := fmt.Sprintf(`
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		AND table_name = '%s'`, safeName)

	var count int
	
	err := c.db.QueryRow(query).Scan(&count)

	if err == sql.ErrNoRows {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (c *Client) runMigrationFile(filename string) error {
	log.Printf("📄 Aplicando: %s", filename)

	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo: %w", err)
	}

	ctx := context.Background()
	_, err = c.db.ExecContext(ctx, string(content))
	if err != nil {
		return fmt.Errorf("erro ao executar SQL: %w", err)
	}

	log.Printf("✅ Migration aplicada: %s", filepath.Base(filename))
	return nil
}

// ✅ SAFE: Sem parâmetros
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