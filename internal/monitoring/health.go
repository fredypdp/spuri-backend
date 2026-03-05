package monitoring

import (
	"context"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
)

type HealthStatus string

const (
	StatusHealthy   HealthStatus = "healthy"
	StatusDegraded  HealthStatus = "degraded"
	StatusUnhealthy HealthStatus = "unhealthy"
)

type ComponentHealth struct {
	Status    HealthStatus      `json:"status"`
	Message   string            `json:"message,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Details   map[string]string `json:"details,omitempty"`
}

type SystemHealth struct {
	Status     HealthStatus               `json:"status"`
	Timestamp  time.Time                  `json:"timestamp"`
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentHealth `json:"components"`
	mu         sync.RWMutex
}

type HealthChecker struct {
	db        *sqlx.DB
	startTime time.Time
	health    *SystemHealth
}

func NewHealthChecker(db *sqlx.DB) *HealthChecker {
	return &HealthChecker{
		db:        db,
		startTime: time.Now(),
		health: &SystemHealth{
			Status:     StatusHealthy,
			Components: make(map[string]ComponentHealth),
		},
	}
}

// CheckAll verifica todos os componentes
func (hc *HealthChecker) CheckAll() *SystemHealth {
	hc.health.mu.Lock()
	defer hc.health.mu.Unlock()

	hc.health.Timestamp = time.Now()
	hc.health.Uptime = time.Since(hc.startTime).String()

	// Verificar banco de dados
	hc.checkDatabase()

	// Determinar status geral
	hc.health.Status = hc.calculateOverallStatus()

	return hc.health
}

// checkDatabase verifica conexão e performance do banco
func (hc *HealthChecker) checkDatabase() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	err := hc.db.PingContext(ctx)
	latency := time.Since(start)

	if err != nil {
		hc.health.Components["database"] = ComponentHealth{
			Status:    StatusUnhealthy,
			Message:   "Database connection failed",
			Timestamp: time.Now(),
			Details: map[string]string{
				"error":   err.Error(),
				"latency": latency.String(),
			},
		}
		return
	}

	// Verificar latência
	status := StatusHealthy
	message := "Database is healthy"

	if latency > 1*time.Second {
		status = StatusDegraded
		message = "Database latency is high"
	}

	// Verificar pool de conexões
	stats := hc.db.Stats()
	details := map[string]string{
		"latency":          latency.String(),
		"open_connections": string(rune(stats.OpenConnections)),
		"in_use":           string(rune(stats.InUse)),
		"idle":             string(rune(stats.Idle)),
		"wait_count":       string(rune(stats.WaitCount)),
	}

	// Alertar se pool saturado
	if stats.WaitCount > 100 {
		status = StatusDegraded
		message = "Database pool is saturated"
	}

	hc.health.Components["database"] = ComponentHealth{
		Status:    status,
		Message:   message,
		Timestamp: time.Now(),
		Details:   details,
	}
}

// calculateOverallStatus determina status geral baseado nos componentes
func (hc *HealthChecker) calculateOverallStatus() HealthStatus {
	hasUnhealthy := false
	hasDegraded := false

	for _, component := range hc.health.Components {
		switch component.Status {
		case StatusUnhealthy:
			hasUnhealthy = true
		case StatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return StatusUnhealthy
	}
	if hasDegraded {
		return StatusDegraded
	}
	return StatusHealthy
}

// IsHealthy retorna se o sistema está saudável
func (hc *HealthChecker) IsHealthy() bool {
	hc.health.mu.RLock()
	defer hc.health.mu.RUnlock()
	return hc.health.Status == StatusHealthy
}