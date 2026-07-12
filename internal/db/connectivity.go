package db

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"strings"
)

// IsTransientConnectionError classifies database/network connectivity failures
// that should be exposed as temporary unavailability instead of business errors.
func IsTransientConnectionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, sql.ErrConnDone) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	msg := strings.ToLower(err.Error())
	patterns := []string{
		"connection refused", "connection reset", "connection closed", "broken pipe",
		"server closed the connection", "terminating connection", "database system is starting up",
		"database system is shutting down", "could not connect", "connection timed out",
		"timeout", "temporary failure in name resolution", "no such host", "network is unreachable",
		"i/o timeout", "sql: database is closed", "bad connection",
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
