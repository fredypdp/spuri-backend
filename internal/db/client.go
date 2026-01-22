package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type Client struct {
	db     *sqlx.DB
	config *Config
	ctx    context.Context
}

type Config struct {
	Host            string
	Port            string
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxConnections  int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	DatabaseURL     string
}

func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	var connStr string
	
	if config.DatabaseURL != "" {
		connStr = config.DatabaseURL + "?client_encoding=UTF8"
		log.Printf("🔗 Usando DATABASE_URL (UTF-8)")
	} else {
		connStr = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s client_encoding=UTF8",
			config.Host, config.Port, config.User, config.Password, 
			config.DBName, config.SSLMode,
		)
	}

	// ✅ CORRIGIDO: Adicionar operador :=
	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao BD: %w", err)
	}

	db.SetMaxOpenConns(config.MaxConnections)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("erro ao pingar BD: %w", err)
	}

	client := &Client{
		db:     db,
		config: config,
		ctx:    context.Background(),
	}

	if err := client.setUTF8Encoding(); err != nil {
		log.Printf("⚠️ Aviso: não foi possível configurar UTF-8: %v", err)
	}

	log.Printf("✅ BD conectado (UTF-8)")

	return client, nil
}

func (c *Client) setUTF8Encoding() error {
	queries := []string{
		"SET client_encoding = 'UTF8'",
		"SET standard_conforming_strings = on",
	}

	for _, query := range queries {
		if _, err := c.db.Exec(query); err != nil {
			return err
		}
	}

	return nil
}

func DefaultConfig() *Config {
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		log.Println("📊 Detectado DATABASE_URL")
		return &Config{
			DatabaseURL:     dbURL,
			SSLMode:         "require",
			MaxConnections:  25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		}
	}

	log.Println("📊 Usando variáveis individuais")
	return &Config{
		Host:            getEnv("DB_HOST", "localhost"),
		Port:            getEnv("DB_PORT", "5432"),
		User:            getEnv("DB_USER", "fredy"),
		Password:        getEnv("DB_PASSWORD", "fredy123"),
		DBName:          getEnv("DB_NAME", "spuri_db"),
		SSLMode:         getEnv("DB_SSLMODE", "disable"),
		MaxConnections:  25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

func (c *Client) DB() *sqlx.DB {
	return c.db
}

func (c *Client) Context() context.Context {
	return c.ctx
}

func (c *Client) Config() *Config {
	return c.config
}

func (c *Client) Health() error {
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
	defer cancel()

	return c.db.PingContext(ctx)
}

func (c *Client) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return c.db.BeginTxx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func (c *Client) Stats() sql.DBStats {
	return c.db.Stats()
}

func (c *Client) LogStats() {
	stats := c.Stats()
	log.Printf(`
📊 BD Stats:
  - Conexões abertas: %d
  - Conexões em uso: %d
  - Conexões ociosas: %d
`,
		stats.OpenConnections,
		stats.InUse,
		stats.Idle,
	)
}