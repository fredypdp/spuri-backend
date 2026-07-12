package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
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
	ConnMaxIdleTime time.Duration
	HealthTimeout   time.Duration
	DatabaseURL     string
}

func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	var connStr string

	if config.DatabaseURL != "" {
		connStr = withClientEncoding(config.DatabaseURL)
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

	applyPoolLimits(db, config)

	if err := pingWithRetry(context.Background(), db, config.HealthTimeout); err != nil {
		_ = db.Close()
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
		return normalizeConfig(&Config{
			DatabaseURL:     dbURL,
			SSLMode:         "require",
			MaxConnections:  getEnvInt("DB_MAX_OPEN_CONNS", 15),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDurationSeconds("DB_CONN_MAX_LIFETIME_SECONDS", 5*time.Minute),
			ConnMaxIdleTime: getEnvDurationSeconds("DB_CONN_MAX_IDLE_TIME_SECONDS", 2*time.Minute),
			HealthTimeout:   getEnvDurationSeconds("DB_HEALTH_TIMEOUT_SECONDS", 3*time.Second),
		})
	}

	log.Println("📊 Usando variáveis individuais")
	return normalizeConfig(&Config{
		Host:            getEnv("DB_HOST", "localhost"),
		Port:            getEnv("DB_PORT", "5432"),
		User:            getEnv("DB_USER", "fredy"),
		Password:        getEnv("DB_PASSWORD", "fredy123"),
		DBName:          getEnv("DB_NAME", "spuri_db"),
		SSLMode:         getEnv("DB_SSLMODE", "disable"),
		MaxConnections:  getEnvInt("DB_MAX_OPEN_CONNS", 15),
		MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
		ConnMaxLifetime: getEnvDurationSeconds("DB_CONN_MAX_LIFETIME_SECONDS", 5*time.Minute),
		ConnMaxIdleTime: getEnvDurationSeconds("DB_CONN_MAX_IDLE_TIME_SECONDS", 2*time.Minute),
		HealthTimeout:   getEnvDurationSeconds("DB_HEALTH_TIMEOUT_SECONDS", 3*time.Second),
	})
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
	ctx, cancel := context.WithTimeout(c.ctx, c.config.HealthTimeout)
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

func normalizeConfig(config *Config) *Config {
	if config.MaxConnections <= 0 || config.MaxConnections > 15 {
		config.MaxConnections = 15
	}
	if config.MaxIdleConns <= 0 || config.MaxIdleConns > 5 {
		config.MaxIdleConns = 5
	}
	if config.MaxIdleConns > config.MaxConnections {
		config.MaxIdleConns = config.MaxConnections
	}
	if config.ConnMaxLifetime <= 0 {
		config.ConnMaxLifetime = 5 * time.Minute
	}
	if config.ConnMaxIdleTime <= 0 {
		config.ConnMaxIdleTime = 2 * time.Minute
	}
	if config.HealthTimeout <= 0 {
		config.HealthTimeout = 3 * time.Second
	}
	return config
}

func applyPoolLimits(db *sqlx.DB, config *Config) {
	config = normalizeConfig(config)
	db.SetMaxOpenConns(config.MaxConnections)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)
}

func pingWithRetry(ctx context.Context, db *sqlx.DB, timeout time.Duration) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, timeout)
		lastErr = db.PingContext(pingCtx)
		cancel()
		if lastErr == nil {
			return nil
		}
		if !IsTransientConnectionError(lastErr) {
			return lastErr
		}
		time.Sleep(time.Duration(math.Pow(2, float64(attempt))) * 100 * time.Millisecond)
	}
	return lastErr
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("⚠️ valor inválido para %s, usando padrão seguro: %v", key, err)
		return defaultValue
	}
	return parsed
}

func getEnvDurationSeconds(key string, defaultValue time.Duration) time.Duration {
	seconds := getEnvInt(key, int(defaultValue.Seconds()))
	return time.Duration(seconds) * time.Second
}

func withClientEncoding(databaseURL string) string {
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}
	return databaseURL + separator + "client_encoding=UTF8"
}
