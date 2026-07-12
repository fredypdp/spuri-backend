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
