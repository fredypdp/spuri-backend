package utils

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	phoneRegex    = regexp.MustCompile(`^\+?[0-9]{9,15}$`)
	sqlCharsRegex = regexp.MustCompile(`[';-]|--`)
)

func SafeDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)
	
	if email == "" {
		return fmt.Errorf("email não pode estar vazio")
	}
	
	if len(email) > 255 {
		return fmt.Errorf("email muito longo (máximo 255 caracteres)")
	}
	
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("formato de email inválido")
	}
	
	if sqlCharsRegex.MatchString(email) {
		return fmt.Errorf("email contém caracteres inválidos")
	}
	
	return nil
}

func ValidatePhone(phone string) error {
	if phone == "" {
		return nil
	}
	
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	
	if !phoneRegex.MatchString(phone) {
		return fmt.Errorf("formato de telefone inválido")
	}
	
	return nil
}

func ValidateString(value, fieldName string, minLen, maxLen int, required bool) error {
	value = strings.TrimSpace(value)
	
	if required && value == "" {
		return fmt.Errorf("%s é obrigatório", fieldName)
	}

	if !required && value == "" {
		return nil
	}
	
	if value != "" && sqlCharsRegex.MatchString(value) {
		return fmt.Errorf("%s contém caracteres não permitidos", fieldName)
	}
	
	length := utf8.RuneCountInString(value)
	
	if length < minLen {
		return fmt.Errorf("%s deve ter no mínimo %d caracteres", fieldName, minLen)
	}
	
	if length > maxLen {
		return fmt.Errorf("%s deve ter no máximo %d caracteres", fieldName, maxLen)
	}
	
	return nil
}

func SanitizeHTML(value string) string {
	return html.EscapeString(strings.TrimSpace(value))
}

func ValidateNota(nota float64) error {
	if nota < 0 || nota > 20 {
		return fmt.Errorf("nota deve estar entre 0 e 20")
	}
	return nil
}

func ValidateQuantidade(quantidade int, fieldName string) error {
	if quantidade <= 0 {
		return fmt.Errorf("%s deve ser maior que zero", fieldName)
	}
	if quantidade > 1000 {
		return fmt.Errorf("%s excede limite permitido", fieldName)
	}
	return nil
}

func ValidateSenha(senha string) error {
	if len(senha) < 6 {
		return fmt.Errorf("senha deve ter no mínimo 6 caracteres")
	}
	
	if len(senha) > 128 {
		return fmt.Errorf("senha muito longa (máximo 128 caracteres)")
	}
	
	return nil
}

func ValidateBilhete(bilhete string) error {
	if bilhete == "" {
		return nil
	}
	
	bilhete = strings.TrimSpace(bilhete)
	
	// Conta números e letras
	numDigits := 0
	numLetters := 0
	
	for _, char := range bilhete {
		if unicode.IsDigit(char) {
			numDigits++
		} else if unicode.IsLetter(char) {
			numLetters++
		}
	}
	
	// Deve ter exatamente 12 números e 2 letras
	if numDigits != 12 || numLetters != 2 {
		return fmt.Errorf("bilhete de identidade deve conter exatamente 12 números e 2 letras")
	}
	
	// Verifica comprimento total (deve ser 14 caracteres)
	if len(bilhete) != 14 {
		return fmt.Errorf("bilhete de identidade deve ter 14 caracteres")
	}
	
	return nil
}

func ValidateNome(nome string) error {
	return ValidateString(nome, "nome", 2, 255, true)
}

func ValidateEndereco(endereco string) error {
	return ValidateString(endereco, "endereço", 5, 500, true)
}

func ValidateObservacao(obs *string) error {
	if obs == nil || *obs == "" {
		return nil
	}
	
	if utf8.RuneCountInString(*obs) > 1000 {
		return fmt.Errorf("observação muito longa (máximo 1000 caracteres)")
	}
	
	return nil
}

func ValidateProvincia(provincia string) error {
	validProvincias := map[string]bool{
		"BGO": true, "BGU": true, "BIE": true, "CAB": true,
		"CND": true, "CNO": true, "CUS": true, "CBG": true,
		"CNN": true, "HUA": true, "HUI": true, "IBG": true,
		"LUA": true, "LNO": true, "LSU": true, "MAL": true,
		"MOX": true, "MXL": true, "NAM": true, "UIG": true,
		"ZAI": true,
	}
	
	if !validProvincias[strings.ToUpper(provincia)] {
		return fmt.Errorf("província inválida")
	}
	
	return nil
}

func ValidateURL(url string) error {
	if url == "" {
		return nil
	}
	
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("URL deve começar com http:// ou https://")
	}
	
	if len(url) > 500 {
		return fmt.Errorf("URL muito longa")
	}
	
	if sqlCharsRegex.MatchString(url) {
		return fmt.Errorf("URL contém caracteres não permitidos")
	}
	
	return nil
}

func ValidatePeriodo(periodo string) error {
	validPeriodos := map[string]bool{
		"1_trimestre": true,
		"2_trimestre": true,
		"3_trimestre": true,
		"1_semestre":  true,
		"2_semestre":  true,
	}
	
	if !validPeriodos[periodo] {
		return fmt.Errorf("período inválido")
	}
	
	return nil
}

func ValidateRole(role string) error {
	validRoles := map[string]bool{
		"fpp":     true,
		"adm":     true,
		"gerente": true,
	}
	
	if !validRoles[role] {
		return fmt.Errorf("role inválido (deve ser: fpp, adm ou gerente)")
	}
	
	return nil
}

func SanitizeAndValidateString(value, fieldName string, minLen, maxLen int, required bool) (string, error) {
	sanitized := SanitizeHTML(strings.TrimSpace(value))
	
	if err := ValidateString(sanitized, fieldName, minLen, maxLen, required); err != nil {
		return "", err
	}
	
	return sanitized, nil
}

func ValidateAnosFundamental(anos []string) error {
	validAnos := map[string]bool{
		"primeiro_fundamental": true, "segundo_fundamental": true,
		"terceiro_fundamental": true, "quarto_fundamental": true,
		"quinto_fundamental": true, "sexto_fundamental": true,
		"setimo_fundamental": true, "oitavo_fundamental": true,
		"nono_fundamental": true,
	}
	
	for _, ano := range anos {
		if !validAnos[ano] {
			return fmt.Errorf("ano fundamental inválido: %s", ano)
		}
	}
	
	return nil
}

func ValidateNivelCurso(tipo string, nivel []string) error {
	if tipo == "medio" {
		validNiveis := map[string]bool{
			"primeiro_medio": true,
			"segundo_medio":  true,
			"terceiro_medio": true,
		}
		
		for _, n := range nivel {
			if !validNiveis[n] {
				return fmt.Errorf("nível de ensino médio inválido: %s", n)
			}
		}
	} else if tipo == "superior" {
		validNiveis := map[string]bool{
			"primeiro_ano": true, "segundo_ano": true,
			"terceiro_ano": true, "quarto_ano": true,
			"quinto_ano": true, "sexto_ano": true,
		}
		
		for _, n := range nivel {
			if !validNiveis[n] {
				return fmt.Errorf("nível de ensino superior inválido: %s", n)
			}
		}
	}
	
	return nil
}