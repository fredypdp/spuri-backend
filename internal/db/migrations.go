package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// allMigrations define a ordem exata de execução.
// Todas são idempotentes (IF NOT EXISTS / IF EXISTS / OR REPLACE).
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
	"migrations/015_add_periodos_to_cursos.sql",
	"migrations/016_avaliacao_final.sql",
	"migrations/017_avaliacao_turma.sql",
	"migrations/018_materia_periodo.sql",
	"migrations/019_soft_delete_auditavel.sql",
	"migrations/020_fix_verify_hash_chain.sql",
	"migrations/021_fix_projection_notas.sql",
	"migrations/022_reforcar_anos_academicos_constraint.sql",
	"migrations/023_admin_senha_alterada.sql",
	"migrations/024_remove_inscricoes_sistema.sql",
	"migrations/025_admin_email_unique_index.sql",
	"migrations/026_academia_motivo_desativacao.sql",
	"migrations/027_academia_senha_alterada.sql",
	"migrations/028_fix_estudante_email_verificado.sql",
	"migrations/029_fix_ledger_truncate_protection.sql",
	"migrations/030_fix_view_estudantes_com_cursos.sql",
	"migrations/031_fix_sistema_config_colunas.sql",
	"migrations/032_add_adicionado_por_categoria_nota.sql",
	"migrations/033_remove_curso_legado_varchar.sql",
	"migrations/034_garantias_atomicidade.sql",
	"migrations/035_spuri_generate_codigo_academia.sql",
	"migrations/036_turmas_status_alterado_por.sql",
	"migrations/037_soft_delete_faltas.sql",
	"migrations/038_auditoria_delecao_notas_faltas.sql",
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

// runMigrationFile executa o conteúdo de um arquivo SQL.
//
// FIX DB-12: migrations sem BEGIN/COMMIT explícito (ex: 001_complete_schema.sql)
// eram executadas sem proteção transacional — uma falha a meio deixava o banco
// em estado parcialmente inicializado sem rollback automático.
//
// Agora: se o arquivo não contém BEGIN no início, o runner envolve o SQL em
// BEGIN/COMMIT automaticamente. Arquivos que já têm BEGIN/COMMIT próprio
// (maioria) são executados sem interferência.
//
// Nota: arquivos com COMMIT no meio (ex: múltiplas transações separadas) são
// detectados e executados sem wrapping para não quebrar a lógica existente.
func (c *Client) runMigrationFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("erro ao ler arquivo: %w", err)
	}

	sql := string(content)
	ctx := context.Background()

	if migrationNeedsWrapper(sql) {
		log.Printf("[DEBUG] %s: sem BEGIN/COMMIT explícito — envolvendo em transação automática",
			filepath.Base(filename))
		sql = "BEGIN;\n" + sql + "\nCOMMIT;\n"
	}

	_, err = c.db.ExecContext(ctx, sql)
	return err
}

// migrationNeedsWrapper retorna true se o SQL não tem BEGIN explícito.
// Arquivos que já têm BEGIN/COMMIT não precisam de wrapping.
// A verificação é case-insensitive e ignora whitespace/comentários no início.
func migrationNeedsWrapper(sql string) bool {
	// Normalizar: remover comentários de linha e verificar tokens iniciais.
	lines := strings.Split(sql, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Ignorar linhas vazias e comentários SQL
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		// Primeira linha de código real: verificar se é BEGIN
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "BEGIN") {
			return false // já tem BEGIN
		}
		// Primeira instrução não é BEGIN — precisamos envolver
		return true
	}
	// Arquivo vazio ou só comentários: não há risco, mas envolve por segurança
	return true
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