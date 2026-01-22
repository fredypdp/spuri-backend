package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// RunMigrations executa migrations automaticamente
func (c *Client) RunMigrations() error {
	log.Println("🔍 Verificando migrations...")

	// Verificar se tabela spuri_ledger existe
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

	// Aplicar migration 1
	if err := c.runMigrationFile("migrations/001_complete_schema.sql"); err != nil {
		return fmt.Errorf("erro na migration 1: %w", err)
	}

	log.Println("🎉 Todas as migrations aplicadas com sucesso!")
	return nil
}

func (c *Client) tableExists(tableName string) (bool, error) {
	query := `
		SELECT COUNT(*) 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		AND table_name = $1`

	var count int
	// QueryRow + Scan não retorna erro quando count = 0
	err := c.db.QueryRow(query, tableName).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func (c *Client) runMigrationFile(filename string) error {
	log.Printf("📝 Aplicando: %s", filename)

	// Ler arquivo
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo: %w", err)
	}

	// Executar SQL
	ctx := context.Background()
	_, err = c.db.ExecContext(ctx, string(content))
	if err != nil {
		return fmt.Errorf("erro ao executar SQL: %w", err)
	}

	log.Printf("✅ Migration aplicada: %s", filepath.Base(filename))
	return nil
}

func (c *Client) logStats() {
	ctx := context.Background()

	var eventCount, estudanteCount, academiaCount int64

	// QueryRow + Scan para evitar "no rows" error
	c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM spuri_ledger").Scan(&eventCount)
	c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM projection_estudantes").Scan(&estudanteCount)
	c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM projection_academias").Scan(&academiaCount)

	log.Println("\n📊 Estatísticas do Banco:")
	log.Printf("   Eventos no ledger: %d", eventCount)
	log.Printf("   Estudantes: %d", estudanteCount)
	log.Printf("   Academias: %d", academiaCount)
}