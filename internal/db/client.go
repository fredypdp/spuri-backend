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
	db              *sqlx.DB
	config          *Config
	ctx             context.Context
	healthTicker    *time.Ticker
	stopHealthCheck chan bool
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

	sqlDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir BD: %w", err)
	}

	db := sqlx.NewDb(sqlDB, "postgres")

	// 🔥 CONFIGURAÇÕES OTIMIZADAS PARA CONEXÕES DE LONGA DURAÇÃO
	db.SetMaxOpenConns(config.MaxConnections)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	
	// ✅ NOVO: Define idle timeout para fechar conexões inativas
	db.SetConnMaxIdleTime(30 * time.Second)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("erro ao pingar BD: %w", err)
	}

	client := &Client{
		db:              db,
		config:          config,
		ctx:             context.Background(),
		stopHealthCheck: make(chan bool),
	}

	if err := client.setUTF8Encoding(); err != nil {
		log.Printf("⚠️ Aviso: não foi possível configurar UTF-8: %v", err)
	}

	// ✅ NOVO: Iniciar health check automático
	client.startHealthCheck()

	log.Printf("✅ BD conectado (UTF-8) com health check ativo")

	return client, nil
}

// ✅ NOVO: Health check periódico para reciclar conexões mortas
func (c *Client) startHealthCheck() {
	c.healthTicker = time.NewTicker(10 * time.Second)
	
	go func() {
		for {
			select {
			case <-c.healthTicker.C:
				ctx, cancel := context.WithTimeout(c.ctx, 2*time.Second)
				if err := c.db.PingContext(ctx); err != nil {
					log.Printf("⚠️ [HealthCheck] Conexão perdida, tentando reconectar: %v", err)
					
					// Força fechamento de conexões idle quebradas
					c.db.SetMaxIdleConns(0)
					time.Sleep(100 * time.Millisecond)
					c.db.SetMaxIdleConns(c.config.MaxIdleConns)
				}
				cancel()
				
			case <-c.stopHealthCheck:
				c.healthTicker.Stop()
				return
			}
		}
	}()
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
	// ✅ NOVO: Parar health check antes de fechar
	if c.stopHealthCheck != nil {
		close(c.stopHealthCheck)
	}
	
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

// ✅ NOVO: Executar query com retry automático
func (c *Client) QueryWithRetry(query string, args ...interface{}) (*sql.Rows, error) {
	maxRetries := 3
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		rows, err := c.db.Query(query, args...)
		if err == nil {
			return rows, nil
		}
		
		lastErr = err
		
		// Se for erro de conexão, tentar novamente
		if isConnectionError(err) {
			log.Printf("⚠️ [QueryWithRetry] Tentativa %d/%d falhou: %v", attempt, maxRetries, err)
			
			if attempt < maxRetries {
				// Forçar reciclagem de conexões
				c.db.SetMaxIdleConns(0)
				time.Sleep(time.Duration(attempt*100) * time.Millisecond)
				c.db.SetMaxIdleConns(c.config.MaxIdleConns)
			}
			continue
		}
		
		// Se não for erro de conexão, retornar imediatamente
		return nil, err
	}
	
	return nil, fmt.Errorf("falha após %d tentativas: %w", maxRetries, lastErr)
}

// ✅ NOVO: Executar QueryRow com retry
func (c *Client) QueryRowWithRetry(query string, args ...interface{}) *sql.Row {
	maxRetries := 3
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		row := c.db.QueryRow(query, args...)
		
		// Testar se a conexão está ok fazendo um Scan em variável dummy
		var test interface{}
		err := row.Scan(&test)
		
		if err == nil || err == sql.ErrNoRows {
			// Refazer a query para retornar row fresco
			return c.db.QueryRow(query, args...)
		}
		
		if isConnectionError(err) && attempt < maxRetries {
			log.Printf("⚠️ [QueryRowWithRetry] Tentativa %d/%d falhou: %v", attempt, maxRetries, err)
			
			c.db.SetMaxIdleConns(0)
			time.Sleep(time.Duration(attempt*100) * time.Millisecond)
			c.db.SetMaxIdleConns(c.config.MaxIdleConns)
			continue
		}
		
		break
	}
	
	return c.db.QueryRow(query, args...)
}

// ✅ NOVO: Executar Exec com retry
func (c *Client) ExecWithRetry(query string, args ...interface{}) (sql.Result, error) {
	maxRetries := 3
	var lastErr error
	
	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := c.db.Exec(query, args...)
		if err == nil {
			return result, nil
		}
		
		lastErr = err
		
		if isConnectionError(err) {
			log.Printf("⚠️ [ExecWithRetry] Tentativa %d/%d falhou: %v", attempt, maxRetries, err)
			
			if attempt < maxRetries {
				c.db.SetMaxIdleConns(0)
				time.Sleep(time.Duration(attempt*100) * time.Millisecond)
				c.db.SetMaxIdleConns(c.config.MaxIdleConns)
			}
			continue
		}
		
		return nil, err
	}
	
	return nil, fmt.Errorf("falha após %d tentativas: %w", maxRetries, lastErr)
}

// ✅ NOVO: Detectar erros de conexão
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	
	errMsg := err.Error()
	connectionErrors := []string{
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"connection timed out",
		"driver: bad connection",
		"invalid connection",
		"the database system is starting up",
	}
	
	for _, connErr := range connectionErrors {
		if containsIgnoreCase(errMsg, connErr) {
			return true
		}
	}
	
	return false
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    len(s) > len(substr) && 
		    (s[:len(substr)] == substr || 
		     containsIgnoreCase(s[1:], substr)))
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
