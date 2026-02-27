package utils

import (
	"fmt"
	"html"
	"log"
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
	log.Printf("📧 [ValidateEmail] Validando email: %s", email)
	
	email = strings.TrimSpace(email)
	
	if email == "" {
		log.Printf("❌ [ValidateEmail] Email vazio")
		return fmt.Errorf("email não pode estar vazio")
	}
	
	if len(email) > 255 {
		log.Printf("❌ [ValidateEmail] Email muito longo: %d caracteres", len(email))
		return fmt.Errorf("email muito longo (máximo 255 caracteres)")
	}
	
	if !emailRegex.MatchString(email) {
		log.Printf("❌ [ValidateEmail] Formato inválido: %s", email)
		return fmt.Errorf("formato de email inválido")
	}
	
	if sqlCharsRegex.MatchString(email) {
		log.Printf("❌ [ValidateEmail] Caracteres SQL detectados")
		return fmt.Errorf("email contém caracteres inválidos")
	}
	
	log.Printf("✅ [ValidateEmail] Email válido")
	return nil
}

func ValidatePhone(phone string) error {
	if phone == "" {
		log.Printf("⏭️ [ValidatePhone] Telefone vazio (opcional)")
		return nil
	}
	
	log.Printf("📱 [ValidatePhone] Validando telefone: %s", phone)
	
	phone = strings.ReplaceAll(phone, " ", "")
	phone = strings.ReplaceAll(phone, "-", "")
	
	if !phoneRegex.MatchString(phone) {
		log.Printf("❌ [ValidatePhone] Formato inválido: %s", phone)
		return fmt.Errorf("formato de telefone inválido")
	}
	
	log.Printf("✅ [ValidatePhone] Telefone válido")
	return nil
}

func ValidateString(value, fieldName string, minLen, maxLen int, required bool) error {
	log.Printf("📝 [ValidateString] Campo: %s, Min: %d, Max: %d, Required: %v", 
		fieldName, minLen, maxLen, required)
	
	value = strings.TrimSpace(value)
	
	if required && value == "" {
		log.Printf("❌ [ValidateString] Campo obrigatório vazio: %s", fieldName)
		return fmt.Errorf("%s é obrigatório", fieldName)
	}

	if !required && value == "" {
		log.Printf("⏭️ [ValidateString] Campo opcional vazio: %s", fieldName)
		return nil
	}
	
	if value != "" && sqlCharsRegex.MatchString(value) {
		log.Printf("❌ [ValidateString] Caracteres SQL detectados em: %s", fieldName)
		return fmt.Errorf("%s contém caracteres não permitidos", fieldName)
	}
	
	length := utf8.RuneCountInString(value)
	
	if length < minLen {
		log.Printf("❌ [ValidateString] %s muito curto: %d (mín: %d)", fieldName, length, minLen)
		return fmt.Errorf("%s deve ter no mínimo %d caracteres", fieldName, minLen)
	}
	
	if length > maxLen {
		log.Printf("❌ [ValidateString] %s muito longo: %d (máx: %d)", fieldName, length, maxLen)
		return fmt.Errorf("%s deve ter no máximo %d caracteres", fieldName, maxLen)
	}
	
	log.Printf("✅ [ValidateString] %s válido - Length: %d", fieldName, length)
	return nil
}

func SanitizeHTML(value string) string {
	sanitized := html.EscapeString(strings.TrimSpace(value))
	log.Printf("🧹 [SanitizeHTML] Original length: %d, Sanitized length: %d", 
		len(value), len(sanitized))
	return sanitized
}

func ValidateNota(nota float64) error {
	log.Printf("📊 [ValidateNota] Validando nota: %.2f", nota)
	
	if nota < 0 || nota > 20 {
		log.Printf("❌ [ValidateNota] Nota fora do intervalo: %.2f", nota)
		return fmt.Errorf("nota deve estar entre 0 e 20")
	}
	
	log.Printf("✅ [ValidateNota] Nota válida: %.2f", nota)
	return nil
}

func ValidateQuantidade(quantidade int, fieldName string) error {
	log.Printf("🔢 [ValidateQuantidade] Campo: %s, Valor: %d", fieldName, quantidade)
	
	if quantidade <= 0 {
		log.Printf("❌ [ValidateQuantidade] %s <= 0: %d", fieldName, quantidade)
		return fmt.Errorf("%s deve ser maior que zero", fieldName)
	}
	if quantidade > 1000 {
		log.Printf("❌ [ValidateQuantidade] %s > 1000: %d", fieldName, quantidade)
		return fmt.Errorf("%s excede limite permitido", fieldName)
	}
	
	log.Printf("✅ [ValidateQuantidade] %s válido: %d", fieldName, quantidade)
	return nil
}

func ValidateSenha(senha string) error {
	log.Printf("🔐 [ValidateSenha] Validando senha - Length: %d", len(senha))
	
	if len(senha) < 6 {
		log.Printf("❌ [ValidateSenha] Senha muito curta: %d caracteres", len(senha))
		return fmt.Errorf("senha deve ter no mínimo 6 caracteres")
	}
	
	if len(senha) > 128 {
		log.Printf("❌ [ValidateSenha] Senha muito longa: %d caracteres", len(senha))
		return fmt.Errorf("senha muito longa (máximo 128 caracteres)")
	}
	
	log.Printf("✅ [ValidateSenha] Senha válida")
	return nil
}

func ValidateBilhete(bilhete string) error {
	if bilhete == "" {
		log.Printf("⏭️ [ValidateBilhete] Bilhete vazio (opcional)")
		return nil
	}
	
	log.Printf("🆔 [ValidateBilhete] Validando bilhete: %s", bilhete)
	
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
	
	log.Printf("📊 [ValidateBilhete] Dígitos: %d, Letras: %d, Total: %d", 
		numDigits, numLetters, len(bilhete))
	
	// Deve ter exatamente 12 números e 2 letras
	if numDigits != 12 || numLetters != 2 {
		log.Printf("❌ [ValidateBilhete] Composição inválida - Esperado: 12 dígitos + 2 letras")
		return fmt.Errorf("bilhete de identidade deve conter exatamente 12 números e 2 letras")
	}
	
	// Verifica comprimento total (deve ser 14 caracteres)
	if len(bilhete) != 14 {
		log.Printf("❌ [ValidateBilhete] Tamanho inválido: %d (esperado: 14)", len(bilhete))
		return fmt.Errorf("bilhete de identidade deve ter 14 caracteres")
	}
	
	log.Printf("✅ [ValidateBilhete] Bilhete válido")
	return nil
}

func ValidateNome(nome string) error {
	log.Printf("👤 [ValidateNome] Validando nome: %s", nome)
	return ValidateString(nome, "nome", 2, 255, true)
}

func ValidateEndereco(endereco string) error {
	log.Printf("🏠 [ValidateEndereco] Validando endereço")
	return ValidateString(endereco, "endereço", 5, 500, true)
}

func ValidateObservacao(obs *string) error {
	if obs == nil || *obs == "" {
		log.Printf("⏭️ [ValidateObservacao] Observação vazia (opcional)")
		return nil
	}
	
	length := utf8.RuneCountInString(*obs)
	log.Printf("📝 [ValidateObservacao] Validando observação - Length: %d", length)
	
	if length > 1000 {
		log.Printf("❌ [ValidateObservacao] Observação muito longa: %d caracteres", length)
		return fmt.Errorf("observação muito longa (máximo 1000 caracteres)")
	}
	
	log.Printf("✅ [ValidateObservacao] Observação válida")
	return nil
}

func ValidateProvincia(provincia string) error {
	log.Printf("🗺️ [ValidateProvincia] Validando província: %s", provincia)
	
	validProvincias := map[string]bool{
		"BGO": true, "BGU": true, "BIE": true, "CAB": true,
		"CND": true, "CNO": true, "CUS": true, "CBG": true,
		"CNN": true, "HUA": true, "HUI": true, "IBG": true,
		"LUA": true, "LNO": true, "LSU": true, "MAL": true,
		"MOX": true, "MXL": true, "NAM": true, "UIG": true,
		"ZAI": true,
	}
	
	provinciaUpper := strings.ToUpper(provincia)
	
	if !validProvincias[provinciaUpper] {
		log.Printf("❌ [ValidateProvincia] Província inválida: %s", provincia)
		return fmt.Errorf("província inválida")
	}
	
	log.Printf("✅ [ValidateProvincia] Província válida: %s", provinciaUpper)
	return nil
}

func ValidateURL(url string) error {
	if url == "" {
		log.Printf("⏭️ [ValidateURL] URL vazia (opcional)")
		return nil
	}
	
	log.Printf("🔗 [ValidateURL] Validando URL: %s", url)
	
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		log.Printf("❌ [ValidateURL] URL sem protocolo HTTP/HTTPS")
		return fmt.Errorf("URL deve começar com http:// ou https://")
	}
	
	if len(url) > 500 {
		log.Printf("❌ [ValidateURL] URL muito longa: %d caracteres", len(url))
		return fmt.Errorf("URL muito longa")
	}
	
	if sqlCharsRegex.MatchString(url) {
		log.Printf("❌ [ValidateURL] Caracteres SQL detectados na URL")
		return fmt.Errorf("URL contém caracteres não permitidos")
	}
	
	log.Printf("✅ [ValidateURL] URL válida")
	return nil
}

func ValidatePeriodo(periodo string) error {
	log.Printf("📅 [ValidatePeriodo] Validando período: %s", periodo)
	
	validPeriodos := map[string]bool{
		"1_trimestre": true,
		"2_trimestre": true,
		"3_trimestre": true,
		"1_semestre":  true,
		"2_semestre":  true,
	}
	
	if !validPeriodos[periodo] {
		log.Printf("❌ [ValidatePeriodo] Período inválido: %s", periodo)
		log.Printf("📋 [ValidatePeriodo] Períodos válidos: 1_trimestre, 2_trimestre, 3_trimestre, 1_semestre, 2_semestre")
		return fmt.Errorf("período inválido")
	}
	
	log.Printf("✅ [ValidatePeriodo] Período válido: %s", periodo)
	return nil
}

func ValidateRole(role string) error {
	log.Printf("👔 [ValidateRole] Validando role: %s", role)
	
	validRoles := map[string]bool{
		"fpp":     true,
		"adm":     true,
		"gerente": true,
	}
	
	if !validRoles[role] {
		log.Printf("❌ [ValidateRole] Role inválido: %s", role)
		log.Printf("📋 [ValidateRole] Roles válidos: fpp, adm, gerente")
		return fmt.Errorf("role inválido (deve ser: fpp, adm ou gerente)")
	}
	
	log.Printf("✅ [ValidateRole] Role válido: %s", role)
	return nil
}

func SanitizeAndValidateString(value, fieldName string, minLen, maxLen int, required bool) (string, error) {
	log.Printf("🧹 [SanitizeAndValidateString] Processando campo: %s", fieldName)
	
	sanitized := SanitizeHTML(strings.TrimSpace(value))
	
	if err := ValidateString(sanitized, fieldName, minLen, maxLen, required); err != nil {
		log.Printf("❌ [SanitizeAndValidateString] Validação falhou para %s: %v", fieldName, err)
		return "", err
	}
	
	log.Printf("✅ [SanitizeAndValidateString] %s processado com sucesso", fieldName)
	return sanitized, nil
}

// ValidateAnosFundamental valida que todos os itens são anos fundamentais válidos
// (primeiro_fundamental … nono_fundamental).
func ValidateAnosFundamental(anos []string) error {
	validos := map[string]bool{
		"primeiro_fundamental": true,
		"segundo_fundamental":  true,
		"terceiro_fundamental": true,
		"quarto_fundamental":   true,
		"quinto_fundamental":   true,
		"sexto_fundamental":    true,
		"setimo_fundamental":   true,
		"oitavo_fundamental":   true,
		"nono_fundamental":     true,
	}

	seen := make(map[string]bool, len(anos))
	for i, ano := range anos {
		trimmed := strings.TrimSpace(ano)
		if trimmed == "" {
			return fmt.Errorf("ano na posição %d não pode ser vazio", i)
		}
		if seen[trimmed] {
			return fmt.Errorf("ano duplicado: '%s'", trimmed)
		}
		seen[trimmed] = true
		if !validos[trimmed] {
			return fmt.Errorf(
				"ano '%s' inválido para o ensino fundamental. "+
					"Valores aceitos: primeiro_fundamental até nono_fundamental", trimmed)
		}
	}

	return nil
}

// ValidateAnosCurso valida os anos_academicos de um curso de médio ou superior.
//
// Para médio e superior, os anos são livres (definidos pela academia);
// apenas garantimos que a lista não está vazia e não há duplicatas/vazios.
func ValidateAnosCurso(tipo string, anos []string) error {
	if tipo != "medio" && tipo != "superior" {
		return fmt.Errorf("tipo deve ser 'medio' ou 'superior'; para fundamental use ValidateAnosFundamental")
	}

	if len(anos) == 0 {
		return fmt.Errorf("o curso deve ter pelo menos um ano definido em anos_academicos")
	}

	seen := make(map[string]bool, len(anos))
	for i, n := range anos {
		trimmed := strings.TrimSpace(n)
		if trimmed == "" {
			return fmt.Errorf("ano na posição %d não pode ser vazio", i)
		}
		if seen[trimmed] {
			return fmt.Errorf("ano duplicado em anos_academicos: '%s'", trimmed)
		}
		seen[trimmed] = true
	}

	return nil
}

// ValidateNivelCurso é um alias deprecado de ValidateAnosCurso para manter
// compatibilidade durante a migração. Remover após refatorar todos os callers.
//
// Deprecated: use ValidateAnosCurso.
func ValidateNivelCurso(tipo string, nivel []string) error {
	return ValidateAnosCurso(tipo, nivel)
}