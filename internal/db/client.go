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

// Client representa o cliente Banco de dados
type Client struct {
	db     *sqlx.DB
	config *Config
	ctx    context.Context
}

// Config configuração do Banco de dados
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

// NewClient cria uma nova instância do cliente Banco de dados
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	var connStr string
	
	// Se DATABASE_URL existe (Railway/Heroku), usar diretamente
	if config.DatabaseURL != "" {
		// ✅ ADICIONAR prefer_simple_protocol=true para desabilitar prepared statements
		if !containsParam(config.DatabaseURL, "client_encoding") {
			connStr = config.DatabaseURL + "?client_encoding=UTF8&prefer_simple_protocol=true"
		} else {
			connStr = config.DatabaseURL + "&prefer_simple_protocol=true"
		}
		log.Printf("🔗 Usando DATABASE_URL para conexão (UTF-8, sem prepared statements)")
	} else {
		// ✅ ADICIONAR prefer_simple_protocol=true
		connStr = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s client_encoding=UTF8 prefer_simple_protocol=true",
			config.Host, config.Port, config.User, config.Password, 
			config.DBName, config.SSLMode,
		)
	}

	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao Banco de dados: %w", err)
	}

	// Configurar pool de conexões
	db.SetMaxOpenConns(config.MaxConnections)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)

	// Testar conexão
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("erro ao pingar Banco de dados: %w", err)
	}

	client := &Client{
		db:     db,
		config: config,
		ctx:    context.Background(),
	}

	// 🔒 UTF-8: Forçar encoding UTF-8 na sessão
	if err := client.setUTF8Encoding(); err != nil {
		log.Printf("⚠️ Aviso: não foi possível configurar UTF-8: %v", err)
	}

	// Log de conexão
	if config.DatabaseURL != "" {
		log.Printf("✅ Banco de dados conectado via DATABASE_URL (UTF-8, sem prepared statements)")
	} else {
		log.Printf("✅ Banco de dados conectado: %s@%s:%s/%s (UTF-8, sem prepared statements)", 
			config.User, config.Host, config.Port, config.DBName)
	}

	return client, nil
}

// 🔒 NOVO: Configurar encoding UTF-8 na sessão
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

// 🔒 NOVO: Verificar se connection string já tem parâmetro
func containsParam(connStr, param string) bool {
	return len(connStr) > 0 && (
		connStr[len(connStr)-1] == '?' ||
		connStr[len(connStr)-1] == '&') &&
		connStr[len(connStr)-len(param):] == param
}

// DefaultConfig retorna configuração padrão do Banco de dados
func DefaultConfig() *Config {
	// PRIORIDADE 1: DATABASE_URL (Railway, Heroku, etc)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		log.Println("📊 Detectado DATABASE_URL - usando configuração Railway/Heroku")
		return &Config{
			DatabaseURL:     dbURL,
			SSLMode:         "require",
			MaxConnections:  25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		}
	}

	// PRIORIDADE 2: Variáveis individuais (desenvolvimento local)
	log.Println("📊 Usando variáveis de ambiente individuais")
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

// Close fecha a conexão com o Banco de dados
func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

// DB retorna a instância do banco
func (c *Client) DB() *sqlx.DB {
	return c.db
}

// Context retorna o contexto
func (c *Client) Context() context.Context {
	return c.ctx
}

// Config retorna a configuração
func (c *Client) Config() *Config {
	return c.config
}

// Health verifica a saúde da conexão
func (c *Client) Health() error {
	ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
	defer cancel()

	return c.db.PingContext(ctx)
}

// BeginTx inicia uma transação
func (c *Client) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return c.db.BeginTxx(ctx, &sql.TxOptions{
		Isolation: sql.LevelSerializable,
	})
}

// getEnv obtém variável de ambiente com valor padrão
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// Stats retorna estatísticas da conexão
func (c *Client) Stats() sql.DBStats {
	return c.db.Stats()
}

// LogStats imprime estatísticas da conexão
func (c *Client) LogStats() {
	stats := c.Stats()
	log.Printf(`
📊 Banco de dados Stats:
  - Conexões abertas: %d
  - Conexões em uso: %d
  - Conexões ociosas: %d
  - Aguardando conexão: %d
  - Max abertas permitidas: %d
`,
		stats.OpenConnections,
		stats.InUse,
		stats.Idle,
		stats.WaitCount,
		stats.MaxOpenConnections,
	)
}