package utils

import (
	"fmt"
	"log"
	"math/rand"

	"github.com/jmoiron/sqlx"
)

// Nota: rand.IntN (global) é goroutine-safe desde Go 1.20 — não usar rand.New() compartilhado.

func GenerateCodigoEstudante() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	letra1 := letters[rand.Intn(len(letters))]
	letra2 := letters[rand.Intn(len(letters))]
	letra3 := letters[rand.Intn(len(letters))]
	numero := rand.Intn(10000)

	codigo := fmt.Sprintf("%c%c%c%04d", letra1, letra2, letra3, numero)
	log.Printf("🎲 [GenerateCodigoEstudante] Código gerado: %s", codigo)

	return codigo
}

// GenerateUniqueCodigoEstudante gera um código de estudante único verificando
// contra a base de dados.
//
// ⚠️  IMPORTANTE PARA O CALLER: esta função reduz a chance de colisão, mas a
// janela entre o SELECT EXISTS aqui e o INSERT no caller ainda existe em
// cenários de altíssima concorrência. O UNIQUE constraint em
// projection_estudantes.codigo_estudante é a guarda definitiva depois do evento,
// e codigo_estudante_reservas fecha a janela de concorrência antes do INSERT.
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

		// Verifica unicidade na projeção, no ledger de eventos e na tabela de
		// reservas. A reserva é gravada antes do retorno para fechar a janela de
		// corrida entre geração do código e persistência do evento pelo caller.
		const query = `
			WITH codigo_livre AS (
				SELECT NOT EXISTS (
					SELECT 1 FROM projection_estudantes WHERE codigo_estudante = $1
					UNION ALL
					SELECT 1 FROM spuri_ledger
					WHERE aggregate_type = 'Estudante'
					  AND event_type = 'EstudanteCriadoComVinculo'
					  AND payload->>'CodigoEstudante' = $1
					UNION ALL
					SELECT 1 FROM codigo_estudante_reservas WHERE codigo_estudante = $1
				) AS livre
			), reserva AS (
				INSERT INTO codigo_estudante_reservas (codigo_estudante)
				SELECT $1 FROM codigo_livre WHERE livre
				ON CONFLICT DO NOTHING
				RETURNING codigo_estudante
			)
			SELECT EXISTS (SELECT 1 FROM reserva)
		`

		var reserved bool
		err := db.QueryRow(query, codigo).Scan(&reserved)

		if err != nil {
			log.Printf("❌ [GenerateUniqueCodigoEstudante] Erro ao reservar código: %v", err)
			return "", fmt.Errorf("erro ao reservar código: %w", err)
		}

		log.Printf("🔎 [GenerateUniqueCodigoEstudante] Código %s - Reservado: %v", codigo, reserved)

		if reserved {
			log.Printf("✅ [GenerateUniqueCodigoEstudante] Código único reservado com sucesso: %s", codigo)
			return codigo, nil
		}

		log.Printf("🔁 [GenerateUniqueCodigoEstudante] Código %s já existe ou já está reservado, tentando novamente...", codigo)
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
