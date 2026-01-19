// ============================================================================
// ARQUIVO: internal/projections/helpers.go (NOVO)
// Funções auxiliares compartilhadas entre projeções
// ============================================================================

package projections

import "fmt"

// escapeString escapa strings para uso seguro em SQL
func escapeString(s string) string {
	result := ""
	for _, char := range s {
		if char == '\'' {
			result += "''"
		} else if char == '\\' {
			result += "\\\\"
		} else {
			result += string(char)
		}
	}
	return result
}

// formatNullableString formata string nullable para SQL
func formatNullableString(s *string) string {
	if s == nil {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", escapeString(*s))
}