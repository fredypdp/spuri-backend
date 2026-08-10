package monitoring

import (
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	// Contadores
	TotalRequests     atomic.Int64
	TotalErrors       atomic.Int64
	TotalAuthFailures atomic.Int64
	TotalRateLimits   atomic.Int64

	// Métricas por endpoint
	endpoints sync.Map // map[string]*EndpointMetrics

	// Janela de tempo
	windowStart time.Time
	mu          sync.RWMutex
}

type EndpointMetrics struct {
	Path         string
	Count        atomic.Int64
	Errors       atomic.Int64
	TotalLatency atomic.Int64 // microseconds
}

var globalMetrics = &Metrics{
	windowStart: time.Now(),
}

func GetMetrics() *Metrics {
	return globalMetrics
}

// RecordRequest registra uma requisição
func (m *Metrics) RecordRequest(path string, latency time.Duration, hasError bool) {
	m.TotalRequests.Add(1)

	if hasError {
		m.TotalErrors.Add(1)
	}

	// Endpoint específico
	key := path
	val, _ := m.endpoints.LoadOrStore(key, &EndpointMetrics{Path: path})
	em := val.(*EndpointMetrics)

	em.Count.Add(1)
	em.TotalLatency.Add(latency.Microseconds())

	if hasError {
		em.Errors.Add(1)
	}
}

// RecordAuthFailure registra falha de autenticação
func (m *Metrics) RecordAuthFailure() {
	m.TotalAuthFailures.Add(1)
}

// RecordRateLimit registra bloqueio por rate limit
func (m *Metrics) RecordRateLimit() {
	m.TotalRateLimits.Add(1)
}

// GetSnapshot retorna snapshot atual das métricas
func (m *Metrics) GetSnapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	uptime := time.Since(m.windowStart)

	endpoints := make([]map[string]interface{}, 0)
	m.endpoints.Range(func(key, value interface{}) bool {
		em := value.(*EndpointMetrics)
		count := em.Count.Load()

		avgLatency := int64(0)
		if count > 0 {
			avgLatency = em.TotalLatency.Load() / count
		}

		endpoints = append(endpoints, map[string]interface{}{
			"path":        em.Path,
			"requests":    count,
			"errors":      em.Errors.Load(),
			"avg_latency": avgLatency,
			"error_rate":  float64(em.Errors.Load()) / float64(count) * 100,
		})
		return true
	})

	totalReqs := m.TotalRequests.Load()
	requestsPerSecond := float64(0)
	if uptime.Seconds() > 0 {
		requestsPerSecond = float64(totalReqs) / uptime.Seconds()
	}

	return map[string]interface{}{
		"uptime_seconds":      uptime.Seconds(),
		"total_requests":      totalReqs,
		"total_errors":        m.TotalErrors.Load(),
		"total_auth_failures": m.TotalAuthFailures.Load(),
		"total_rate_limits":   m.TotalRateLimits.Load(),
		"requests_per_second": requestsPerSecond,
		"error_rate":          float64(m.TotalErrors.Load()) / float64(totalReqs) * 100,
		"endpoints":           endpoints,
	}
}

// Reset reseta as métricas
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.TotalRequests.Store(0)
	m.TotalErrors.Store(0)
	m.TotalAuthFailures.Store(0)
	m.TotalRateLimits.Store(0)
	m.endpoints = sync.Map{}
	m.windowStart = time.Now()
}
