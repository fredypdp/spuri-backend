package db

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestDefaultConfigUsesAivenSafePoolLimits(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	t.Setenv("DB_MAX_IDLE_CONNS", "")

	cfg := DefaultConfig()

	if cfg.MaxConnections != 15 {
		t.Fatalf("MaxConnections = %d, want 15", cfg.MaxConnections)
	}
	if cfg.MaxIdleConns != 5 {
		t.Fatalf("MaxIdleConns = %d, want 5", cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime != 5*time.Minute {
		t.Fatalf("ConnMaxLifetime = %s, want 5m", cfg.ConnMaxLifetime)
	}
	if cfg.ConnMaxIdleTime != 2*time.Minute {
		t.Fatalf("ConnMaxIdleTime = %s, want 2m", cfg.ConnMaxIdleTime)
	}
	if cfg.HealthTimeout != 3*time.Second {
		t.Fatalf("HealthTimeout = %s, want 3s", cfg.HealthTimeout)
	}
}

func TestDefaultConfigNormalizesUnsafePoolEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@example.com/db")
	t.Setenv("DB_MAX_OPEN_CONNS", "50")
	t.Setenv("DB_MAX_IDLE_CONNS", "50")

	cfg := DefaultConfig()

	if cfg.MaxConnections != 15 {
		t.Fatalf("MaxConnections = %d, want normalized 15", cfg.MaxConnections)
	}
	if cfg.MaxIdleConns != 5 {
		t.Fatalf("MaxIdleConns = %d, want normalized 5", cfg.MaxIdleConns)
	}
}

func TestDefaultConfigReadsSafePoolEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_MAX_OPEN_CONNS", "12")
	t.Setenv("DB_MAX_IDLE_CONNS", "4")
	t.Setenv("DB_CONN_MAX_IDLE_TIME_SECONDS", "90")
	t.Setenv("DB_HEALTH_TIMEOUT_SECONDS", "2")

	cfg := DefaultConfig()

	if cfg.MaxConnections != 12 || cfg.MaxIdleConns != 4 {
		t.Fatalf("pool limits = open %d idle %d, want 12/4", cfg.MaxConnections, cfg.MaxIdleConns)
	}
	if cfg.ConnMaxIdleTime != 90*time.Second {
		t.Fatalf("ConnMaxIdleTime = %s, want 90s", cfg.ConnMaxIdleTime)
	}
	if cfg.HealthTimeout != 2*time.Second {
		t.Fatalf("HealthTimeout = %s, want 2s", cfg.HealthTimeout)
	}
}

func TestDefaultConfigUsesSingleConnectionForNeonPooler(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@ep-cool-name-123456-pooler.us-east-2.aws.neon.tech/neondb?sslmode=require")
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	t.Setenv("DB_MAX_IDLE_CONNS", "")
	t.Setenv("DB_FORCE_SINGLE_CONN", "")

	cfg := DefaultConfig()

	if cfg.MaxConnections != 1 {
		t.Fatalf("MaxConnections = %d, want 1 for Neon pooled connection", cfg.MaxConnections)
	}
	if cfg.MaxIdleConns != 1 {
		t.Fatalf("MaxIdleConns = %d, want 1 for Neon pooled connection", cfg.MaxIdleConns)
	}
}

func TestDefaultConfigAllowsNeonPoolerOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@ep-cool-name-123456-pooler.us-east-2.aws.neon.tech/neondb?sslmode=require")
	t.Setenv("DB_MAX_OPEN_CONNS", "7")
	t.Setenv("DB_MAX_IDLE_CONNS", "3")
	t.Setenv("DB_FORCE_SINGLE_CONN", "")

	cfg := DefaultConfig()

	if cfg.MaxConnections != 7 || cfg.MaxIdleConns != 3 {
		t.Fatalf("pool limits = open %d idle %d, want explicit 7/3", cfg.MaxConnections, cfg.MaxIdleConns)
	}
}

func TestDefaultConfigCanForceSingleConnection(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@db.example.com:5432/app")
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	t.Setenv("DB_MAX_IDLE_CONNS", "")
	t.Setenv("DB_FORCE_SINGLE_CONN", "true")

	cfg := DefaultConfig()

	if cfg.MaxConnections != 1 || cfg.MaxIdleConns != 1 {
		t.Fatalf("pool limits = open %d idle %d, want forced 1/1", cfg.MaxConnections, cfg.MaxIdleConns)
	}
}

func TestWithClientEncodingDoesNotAppendStartupParamForNeonPooler(t *testing.T) {
	databaseURL := "postgres://user:pass@ep-cool-name-123456-pooler.us-east-2.aws.neon.tech/neondb?sslmode=require"

	got := withClientEncoding(databaseURL)

	if got != databaseURL {
		t.Fatalf("withClientEncoding() = %q, want original pooled URL", got)
	}
}

func TestWithClientEncodingAppendsStartupParamForDirectConnection(t *testing.T) {
	databaseURL := "postgres://user:pass@ep-cool-name-123456.us-east-2.aws.neon.tech/neondb?sslmode=require"

	got := withClientEncoding(databaseURL)
	want := databaseURL + "&client_encoding=UTF8"

	if got != want {
		t.Fatalf("withClientEncoding() = %q, want %q", got, want)
	}
}

func TestConnectionStringForDriverRewritesNeonPoolerToDirectHost(t *testing.T) {
	t.Setenv("DB_ALLOW_POOLER", "")
	databaseURL := "postgres://user:pass@ep-cool-name-123456-pooler.us-east-2.aws.neon.tech/neondb?sslmode=require"

	got := connectionStringForDriver(databaseURL)
	want := "postgres://user:pass@ep-cool-name-123456.us-east-2.aws.neon.tech/neondb?sslmode=require&client_encoding=UTF8"

	if got != want {
		t.Fatalf("connectionStringForDriver() = %q, want %q", got, want)
	}
}

func TestConnectionStringForDriverCanKeepExplicitlyAllowedPooler(t *testing.T) {
	t.Setenv("DB_ALLOW_POOLER", "true")
	databaseURL := "postgres://user:pass@ep-cool-name-123456-pooler.us-east-2.aws.neon.tech/neondb?sslmode=require"

	got := connectionStringForDriver(databaseURL)

	if got != databaseURL {
		t.Fatalf("connectionStringForDriver() = %q, want original pooled URL", got)
	}
}

func TestDefaultConfigReadsDatabaseAuthFromEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_HOST", "db.example.com")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_USER", "app_user")
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("DB_NAME", "app_db")
	t.Setenv("DB_SSLMODE", "require")

	cfg := DefaultConfig()

	if cfg.Host != "db.example.com" {
		t.Fatalf("Host = %q, want db.example.com", cfg.Host)
	}
	if cfg.Port != "6543" {
		t.Fatalf("Port = %q, want 6543", cfg.Port)
	}
	if cfg.User != "app_user" {
		t.Fatalf("User = %q, want app_user", cfg.User)
	}
	if cfg.Password != "secret" {
		t.Fatalf("Password = %q, want secret", cfg.Password)
	}
	if cfg.DBName != "app_db" {
		t.Fatalf("DBName = %q, want app_db", cfg.DBName)
	}
	if cfg.SSLMode != "require" {
		t.Fatalf("SSLMode = %q, want require", cfg.SSLMode)
	}
}

func TestNormalizeConfigDoesNotHardcodeDatabaseAuthDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_USER", "")
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "")

	cfg := normalizeConfig(&Config{})

	if cfg.Host != "" || cfg.User != "" || cfg.Password != "" || cfg.DBName != "" {
		t.Fatalf("database auth config = host %q user %q password %q dbname %q, want empty values from unset env", cfg.Host, cfg.User, cfg.Password, cfg.DBName)
	}
}

func TestIsTransientConnectionError(t *testing.T) {
	cases := []error{
		errors.New("dial tcp 10.0.0.1:5432: connect: connection refused"),
		errors.New("pq: the database system is starting up"),
		errors.New("read tcp: i/o timeout"),
	}
	for _, err := range cases {
		if !IsTransientConnectionError(err) {
			t.Fatalf("expected transient error for %q", err.Error())
		}
	}
	if IsTransientConnectionError(errors.New("violates unique constraint")) {
		t.Fatal("business constraint error must not be transient")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestValidateLimitCapsLargePagesAtOneHundred(t *testing.T) {
	cases := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "default for zero", limit: 0, want: 50},
		{name: "keeps valid value", limit: 75, want: 75},
		{name: "caps over max", limit: 1000, want: 100},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidateLimit(tc.limit); got != tc.want {
				t.Fatalf("ValidateLimit(%d) = %d, want %d", tc.limit, got, tc.want)
			}
		})
	}
}
