package utils

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/jmoiron/sqlx"
)

var (
	letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	random  = rand.New(rand.NewSource(time.Now().UnixNano()))
)

func GenerateCodigoEstudante() string {
	letra1 := letters[random.Intn(len(letters))]
	letra2 := letters[random.Intn(len(letters))]
	letra3 := letters[random.Intn(len(letters))]
	numero := random.Intn(10000)
	
	codigo := fmt.Sprintf("%c%c%c%04d", letra1, letra2, letra3, numero)
	log.Printf("🎲 [GenerateCodigoEstudante] Código gerado: %s", codigo)
	
	return codigo
}

func GenerateUniqueCodigoEstudante(db *sqlx.DB) (string, error) {
	log.Printf("🔄 [GenerateUniqueCodigoEstudante] Iniciando geração de código único")
	
	maxAttempts := 100
	
	for i := 0; i < maxAttempts; i++ {
		codigo := GenerateCodigoEstudante()
		
		log.Printf("🔍 [GenerateUniqueCodigoEstudante] Tentativa %d/%d - Código: %s", i+1, maxAttempts, codigo)
		
		if !ValidateCodigoEstudante(codigo) {
			log.Printf("⚠️ [GenerateUniqueCodigoEstudante] Código inválido gerado: %s", codigo)
			continue
		}
		
		// ✅ SEM PREPARED STATEMENT - usar interpolação direta
		query := fmt.Sprintf(`
			SELECT EXISTS(
				SELECT 1 FROM projection_estudantes 
				WHERE codigo_estudante = '%s'
			)
		`, codigo)
		
		log.Printf("📝 [GenerateUniqueCodigoEstudante] Query: %s", query)
		
		var exists bool
		err := db.QueryRow(query).Scan(&exists)
		
		if err != nil {
			log.Printf("❌ [GenerateUniqueCodigoEstudante] Erro ao verificar código: %v", err)
			return "", fmt.Errorf("erro ao verificar código: %w", err)
		}
		
		log.Printf("🔎 [GenerateUniqueCodigoEstudante] Código %s - Existe: %v", codigo, exists)
		
		if !exists {
			log.Printf("✅ [GenerateUniqueCodigoEstudante] Código único gerado com sucesso: %s", codigo)
			return codigo, nil
		}
		
		log.Printf("🔁 [GenerateUniqueCodigoEstudante] Código %s já existe, tentando novamente...", codigo)
	}
	
	log.Printf("❌ [GenerateUniqueCodigoEstudante] Falha após %d tentativas", maxAttempts)
	return "", fmt.Errorf("não foi possível gerar código único após %d tentativas", maxAttempts)
}

func ValidateCodigoEstudante(codigo string) bool {
	log.Printf("🔍 [ValidateCodigoEstudante] Validando código: %s", codigo)
	
	if len(codigo) != 7 {
		log.Printf("❌ [ValidateCodigoEstudante] Tamanho inválido: %d (esperado: 7)", len(codigo))
		return false
	}
	
	// Validar 3 primeiras letras
	for i := 0; i < 3; i++ {
		c := codigo[i]
		if c < 'A' || c > 'Z' {
			log.Printf("❌ [ValidateCodigoEstudante] Caractere inválido na posição %d: %c (esperado: A-Z)", i, c)
			return false
		}
	}
	
	// Validar 4 últimos números
	for i := 3; i < 7; i++ {
		c := codigo[i]
		if c < '0' || c > '9' {
			log.Printf("❌ [ValidateCodigoEstudante] Caractere inválido na posição %d: %c (esperado: 0-9)", i, c)
			return false
		}
	}
	
	log.Printf("✅ [ValidateCodigoEstudante] Código válido: %s", codigo)
	return true
}