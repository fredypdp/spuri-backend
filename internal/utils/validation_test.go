// ============================================================================
// ARQUIVO: internal/utils/validation_test.go
// Testes para validações
// ============================================================================

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{"valid email", "user@example.com", false},
		{"valid with plus", "user+test@example.com", false},
		{"valid with subdomain", "user@mail.example.com", false},
		{"empty email", "", true},
		{"missing @", "userexample.com", true},
		{"missing domain", "user@", true},
		{"missing username", "@example.com", true},
		{"with spaces", "user @example.com", true},
		{"too long", string(make([]byte, 300)) + "@test.com", true},
		{"sql injection", "test'; DROP TABLE--@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePhone(t *testing.T) {
	tests := []struct {
		name    string
		phone   string
		wantErr bool
	}{
		{"valid Angola", "+244923456789", false},
		{"valid without plus", "244923456789", false},
		{"with spaces (normalized)", "244 923 456 789", false},
		{"with dashes (normalized)", "244-923-456-789", false},
		{"empty (optional)", "", false},
		{"too short", "12345", true},
		{"too long", "12345678901234567", true},
		{"with letters", "244abc456789", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePhone(tt.phone)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNota(t *testing.T) {
	tests := []struct {
		name    string
		nota    float64
		wantErr bool
	}{
		{"valid minimum", 0, false},
		{"valid maximum", 20, false},
		{"valid middle", 15.5, false},
		{"below minimum", -0.1, true},
		{"above maximum", 20.1, true},
		{"way too high", 100, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNota(tt.nota)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSenha(t *testing.T) {
	tests := []struct {
		name    string
		senha   string
		wantErr bool
	}{
		{"valid minimum", "123456", false},
		{"valid strong", "MySecureP@ssw0rd", false},
		{"too short", "12345", true},
		{"empty", "", true},
		{"too long", string(make([]byte, 200)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSenha(tt.senha)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePeriodo(t *testing.T) {
	tests := []struct {
		name    string
		periodo string
		wantErr bool
	}{
		{"valid 1_trimestre", "1_trimestre", false},
		{"valid 2_trimestre", "2_trimestre", false},
		{"valid 3_trimestre", "3_trimestre", false},
		{"valid 1_semestre", "1_semestre", false},
		{"valid 2_semestre", "2_semestre", false},
		{"invalid", "4_trimestre", true},
		{"invalid format", "trimestre_1", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePeriodo(tt.periodo)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateRole(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{"valid fpp", "fpp", false},
		{"valid adm", "adm", false},
		{"valid gerente", "gerente", false},
		{"invalid", "admin", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRole(tt.role)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSanitizeHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain text", "Hello World", "Hello World"},
		{"with script", "<script>alert('xss')</script>", "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;"},
		{"with quotes", "Test\"'test", "Test&#34;&#39;test"},
		{"with spaces", "  trimmed  ", "trimmed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateString(t *testing.T) {
	t.Run("should validate required field", func(t *testing.T) {
		err := ValidateString("", "nome", 2, 100, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "obrigatório")
	})

	t.Run("should allow empty optional field", func(t *testing.T) {
		err := ValidateString("", "opcional", 2, 100, false)
		assert.NoError(t, err)
	})

	t.Run("should check minimum length", func(t *testing.T) {
		err := ValidateString("a", "nome", 2, 100, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "mínimo")
	})

	t.Run("should check maximum length", func(t *testing.T) {
		long := string(make([]byte, 200))
		err := ValidateString(long, "nome", 2, 100, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "máximo")
	})

	t.Run("should reject SQL injection", func(t *testing.T) {
		err := ValidateString("test'; DROP TABLE--", "nome", 2, 100, true)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "não permitidos")
	})
}

func TestValidateAnosFundamental(t *testing.T) {
	tests := []struct {
		name    string
		anos    []string
		wantErr bool
	}{
		{
			"valid anos",
			[]string{"primeiro_fundamental", "segundo_fundamental"},
			false,
		},
		{
			"invalid ano",
			[]string{"primeiro_medio"},
			true,
		},
		{
			"mixed valid and invalid",
			[]string{"primeiro_fundamental", "invalid"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAnosFundamental(tt.anos)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateNivelCurso(t *testing.T) {
	tests := []struct {
		name    string
		tipo    string
		nivel   []string
		wantErr bool
	}{
		{
			"valid medio",
			"medio",
			[]string{"primeiro_medio", "segundo_medio"},
			false,
		},
		{
			"valid superior",
			"superior",
			[]string{"primeiro_ano", "segundo_ano"},
			false,
		},
		{
			"invalid medio",
			"medio",
			[]string{"primeiro_ano"},
			true,
		},
		{
			"invalid superior",
			"superior",
			[]string{"primeiro_medio"},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNivelCurso(tt.tipo, tt.nivel)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}