package utils

import (
	"fmt"
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
	
	return fmt.Sprintf("%c%c%c%04d", letra1, letra2, letra3, numero)
}

// ✅ SAFE: String validada e escapada
func GenerateUniqueCodigoEstudante(db *sqlx.DB) (string, error) {
	maxAttempts := 100
	
	for i := 0; i < maxAttempts; i++ {
		codigo := GenerateCodigoEstudante()
		
		// Validar formato antes de usar
		if !ValidateCodigoEstudante(codigo) {
			continue
		}
		
		// Escapar string - apenas caracteres seguros (letras + números)
		// Não precisa de escape adicional pois só contém A-Z e 0-9
		query := fmt.Sprintf(`
			SELECT EXISTS(
				SELECT 1 FROM projection_estudantes 
				WHERE codigo_estudante = '%s'
			)
		`, codigo)
		
		var exists bool
		err := db.QueryRow(query).Scan(&exists)
		
		if err != nil {
			return "", fmt.Errorf("erro ao verificar código: %w", err)
		}
		
		if !exists {
			return codigo, nil
		}
	}
	
	return "", fmt.Errorf("não foi possível gerar código único após %d tentativas", maxAttempts)
}

func ValidateCodigoEstudante(codigo string) bool {
	if len(codigo) != 7 {
		return false
	}
	
	// Primeiros 3 caracteres devem ser letras maiúsculas
	for i := 0; i < 3; i++ {
		c := codigo[i]
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	
	// Últimos 4 caracteres devem ser números
	for i := 3; i < 7; i++ {
		c := codigo[i]
		if c < '0' || c > '9' {
			return false
		}
	}
	
	return true
}