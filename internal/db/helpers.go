// ============================================================================
// ARQUIVO: internal/db/helpers.go (NOVO)
// Funções auxiliares para trabalhar com queries diretas
// ============================================================================

package db

import (
	"strings"
)

// EscapeString escapa strings para uso seguro em SQL
// Substitui aspas simples por duas aspas simples (padrão PostgreSQL)
func EscapeString(s string) string {
	// Substituir aspas simples
	s = strings.ReplaceAll(s, "'", "''")
	// Substituir barras invertidas
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return s
}

// QuoteString coloca aspas simples ao redor de uma string escapada
func QuoteString(s string) string {
	return "'" + EscapeString(s) + "'"
}

// FormatUUID formata UUID para string SQL
func FormatUUID(s string) string {
	return "'" + s + "'"
}

// FormatTimestamp formata timestamp para PostgreSQL
func FormatTimestamp(t string) string {
	return "'" + t + "'"
}