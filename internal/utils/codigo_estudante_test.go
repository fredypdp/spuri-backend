// ============================================================================
// ARQUIVO: internal/utils/codigo_estudante_test.go
// Testes para geração de código de estudante
// ============================================================================

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateCodigoEstudante(t *testing.T) {
	t.Run("should generate codigo with correct format", func(t *testing.T) {
		codigo := GenerateCodigoEstudante()

		assert.Equal(t, 7, len(codigo), "Código deve ter 7 caracteres")
		
		// Verificar primeiros 3 caracteres são letras
		for i := 0; i < 3; i++ {
			assert.True(t, codigo[i] >= 'A' && codigo[i] <= 'Z', 
				"Primeiros 3 chars devem ser letras maiúsculas")
		}
		
		// Verificar últimos 4 caracteres são números
		for i := 3; i < 7; i++ {
			assert.True(t, codigo[i] >= '0' && codigo[i] <= '9', 
				"Últimos 4 chars devem ser números")
		}
	})

	t.Run("should generate different codes", func(t *testing.T) {
		codes := make(map[string]bool)
		
		// Gerar 100 códigos e verificar unicidade
		for i := 0; i < 100; i++ {
			codigo := GenerateCodigoEstudante()
			codes[codigo] = true
		}
		
		// Com espaço de 26^3 * 10000, é muito improvável ter duplicatas em 100 tentativas
		assert.GreaterOrEqual(t, len(codes), 95, 
			"Deve gerar códigos diferentes na maioria das vezes")
	})

	t.Run("should always match validation", func(t *testing.T) {
		for i := 0; i < 50; i++ {
			codigo := GenerateCodigoEstudante()
			assert.True(t, ValidateCodigoEstudante(codigo), 
				"Código gerado deve sempre passar na validação")
		}
	})
}

func TestValidateCodigoEstudante(t *testing.T) {
	tests := []struct {
		name    string
		codigo  string
		valid   bool
	}{
		{"valid format", "ABC1234", true},
		{"valid with zeros", "XYZ0000", true},
		{"valid random", "KAF7392", true},
		{"too short", "AB123", false},
		{"too long", "ABCD1234", false},
		{"lowercase letters", "abc1234", false},
		{"letters in numbers", "ABC12X4", false},
		{"numbers in letters", "A1C1234", false},
		{"all letters", "ABCDEFG", false},
		{"all numbers", "1234567", false},
		{"empty string", "", false},
		{"special chars", "AB@1234", false},
		{"with spaces", "ABC 1234", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateCodigoEstudante(tt.codigo)
			assert.Equal(t, tt.valid, result, 
				"Validação de '%s' deveria retornar %v", tt.codigo, tt.valid)
		})
	}
}

func BenchmarkGenerateCodigoEstudante(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateCodigoEstudante()
	}
}

func BenchmarkValidateCodigoEstudante(b *testing.B) {
	codigo := "ABC1234"
	for i := 0; i < b.N; i++ {
		ValidateCodigoEstudante(codigo)
	}
}