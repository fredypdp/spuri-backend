package genesisdb

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

// Client representa o cliente GenesisDB
type Client struct {
	db     *sqlx.DB
	config *Config
	ctx    context.Context
}

// Config configuração do GenesisDB
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
	DatabaseURL     string // ← NOVO: Para Railway
}

// NewClient cria uma nova instância do cliente GenesisDB
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	var connStr string
	
	// Se DATABASE_URL existe (Railway/Heroku), usar diretamente
	if config.DatabaseURL != "" {
		connStr = config.DatabaseURL
		log.Printf("🔗 Usando DATABASE_URL para conexão")
	} else {
		connStr = fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			config.Host, config.Port, config.User, config.Password, 
			config.DBName, config.SSLMode,
		)
	}

	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao conectar ao GenesisDB: %w", err)
	}

	// Configurar pool de conexões
	db.SetMaxOpenConns(config.MaxConnections)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)

	// Testar conexão
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("erro ao pingar GenesisDB: %w", err)
	}

	client := &Client{
		db:     db,
		config: config,
		ctx:    context.Background(),
	}

	// Log de conexão
	if config.DatabaseURL != "" {
		log.Printf("✅ GenesisDB conectado via DATABASE_URL")
	} else {
		log.Printf("✅ GenesisDB conectado: %s@%s:%s/%s", 
			config.User, config.Host, config.Port, config.DBName)
	}

	return client, nil
}

// DefaultConfig retorna configuração padrão do GenesisDB
func DefaultConfig() *Config {
	// PRIORIDADE 1: DATABASE_URL (Railway, Heroku, etc)
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		log.Println("📊 Detectado DATABASE_URL - usando configuração Railway/Heroku")
		return &Config{
			DatabaseURL:     dbURL,
			SSLMode:         "require", // Railway/Heroku usam SSL
			MaxConnections:  25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		}
	}

	// PRIORIDADE 2: Variáveis individuais (desenvolvimento local)
	log.Println("📊 Usando variáveis de ambiente individuais")
	return &Config{
		Host:            getEnv("GENESISDB_HOST", "localhost"),
		Port:            getEnv("GENESISDB_PORT", "5432"),
		User:            getEnv("GENESISDB_USER", "genesis"),
		Password:        getEnv("GENESISDB_PASSWORD", "genesis123"),
		DBName:          getEnv("GENESISDB_NAME", "spuri_genesis"),
		SSLMode:         getEnv("GENESISDB_SSLMODE", "disable"),
		MaxConnections:  25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// Close fecha a conexão com o GenesisDB
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
📊 GenesisDB Stats:
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