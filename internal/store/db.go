package store

import (
	"fmt"
	"log"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// DB é a instância global do banco de dados
var DB *sqlx.DB

// InitDB inicializa a conexão com o banco de dados
func InitDB() error {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "postgres")
	password := getEnv("DB_PASSWORD", "")
	dbname := getEnv("DB_NAME", "spuri")
	sslmode := getEnv("DB_SSLMODE", "disable")

	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode,
	)

	var err error
	DB, err = sqlx.Connect("postgres", connStr)
	if err != nil {
		return fmt.Errorf("erro ao conectar ao banco de dados: %w", err)
	}

	// Configurações de pool de conexões
	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)

	// Testa a conexão
	if err := DB.Ping(); err != nil {
		return fmt.Errorf("erro ao pingar o banco de dados: %w", err)
	}

	log.Println("✅ Conexão com PostgreSQL estabelecida com sucesso")
	return nil
}

// CloseDB fecha a conexão com o banco de dados
func CloseDB() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

// getEnv obtém uma variável de ambiente ou retorna um valor padrão
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}