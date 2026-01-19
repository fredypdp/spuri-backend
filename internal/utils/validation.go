// ============================================================================
// ARQUIVO: internal/utils/validation.go (NOVO)
// Validação de anos acadêmicos conforme padrão TypeScript
// ============================================================================

package utils

import "fmt"

// AnoEscolar - Anos válidos para ensino fundamental e médio
var AnosEscolares = []string{
	// Fundamental
	"primeiro_fundamental", "segundo_fundamental", "terceiro_fundamental",
	"quarto_fundamental", "quinto_fundamental", "sexto_fundamental",
	"setimo_fundamental", "oitavo_fundamental", "nono_fundamental",
	// Médio
	"primeiro_medio", "segundo_medio", "terceiro_medio", "quarto_medio",
}

// AnoSuperior - Anos válidos para ensino superior
var AnosSuperiores = []string{
	"primeiro_superior", "segundo_superior", "terceiro_superior",
	"quarto_superior", "quinto_superior",
}

// AnosFundamental - Apenas anos do fundamental
var AnosFundamental = []string{
	"primeiro_fundamental", "segundo_fundamental", "terceiro_fundamental",
	"quarto_fundamental", "quinto_fundamental", "sexto_fundamental",
	"setimo_fundamental", "oitavo_fundamental", "nono_fundamental",
}

// AnosMedio - Apenas anos do médio
var AnosMedio = []string{
	"primeiro_medio", "segundo_medio", "terceiro_medio", "quarto_medio",
}

// ValidateAnoEscolar valida se o ano escolar é válido
func ValidateAnoEscolar(ano string) error {
	for _, valid := range AnosEscolares {
		if ano == valid {
			return nil
		}
	}
	return fmt.Errorf("ano escolar inválido: %s", ano)
}

// ValidateAnoSuperior valida se o ano superior é válido
func ValidateAnoSuperior(ano string) error {
	for _, valid := range AnosSuperiores {
		if ano == valid {
			return nil
		}
	}
	return fmt.Errorf("ano superior inválido: %s", ano)
}

// ValidateAnosFundamental valida array de anos fundamentais
func ValidateAnosFundamental(anos []string) error {
	for _, ano := range anos {
		found := false
		for _, valid := range AnosFundamental {
			if ano == valid {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("ano fundamental inválido: %s", ano)
		}
	}
	return nil
}

// ValidateAnosMedio valida array de anos médio
func ValidateAnosMedio(anos []string) error {
	for _, ano := range anos {
		found := false
		for _, valid := range AnosMedio {
			if ano == valid {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("ano médio inválido: %s", ano)
		}
	}
	return nil
}

// ValidateAnosSuperiores valida array de anos superior
func ValidateAnosSuperiores(anos []string) error {
	for _, ano := range anos {
		found := false
		for _, valid := range AnosSuperiores {
			if ano == valid {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("ano superior inválido: %s", ano)
		}
	}
	return nil
}

// ValidateNivelCurso valida nível de curso baseado no tipo
func ValidateNivelCurso(tipo string, nivel []string) error {
	if len(nivel) == 0 {
		return fmt.Errorf("nível não pode estar vazio")
	}

	switch tipo {
	case "medio":
		return ValidateAnosMedio(nivel)
	case "superior":
		return ValidateAnosSuperiores(nivel)
	default:
		return fmt.Errorf("tipo de curso inválido: %s", tipo)
	}
}

// IsAnoFundamental verifica se é ano fundamental
func IsAnoFundamental(ano string) bool {
	for _, valid := range AnosFundamental {
		if ano == valid {
			return true
		}
	}
	return false
}

// IsAnoMedio verifica se é ano médio
func IsAnoMedio(ano string) bool {
	for _, valid := range AnosMedio {
		if ano == valid {
			return true
		}
	}
	return false
}

// IsAnoSuperior verifica se é ano superior
func IsAnoSuperior(ano string) bool {
	for _, valid := range AnosSuperiores {
		if ano == valid {
			return true
		}
	}
	return false
}