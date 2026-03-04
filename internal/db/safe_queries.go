// ============================================================================
// ARQUIVO: internal/db/safe_queries.go
//
// CORREÇÕES APLICADAS:
//   FIX-C3  — "EmailVerificadoEstudante" adicionado à whitelist (novo evento do estudante)
//   FIX-C11 — "StatusEscolarAtualizado" REMOVIDO — evento fantasma sem aggregate emitente
//   FIX-C11 — "AnoLetivoDefinido" já removido anteriormente (mantido comentário)
//   FIX-C11 — Whitelist agora espelha exatamente os eventos emitidos pelos aggregates
// ============================================================================

package db

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidateEventType verifica se o tipo de evento é permitido.
//
// ATENÇÃO: qualquer novo EventType emitido por um aggregate DEVE ser adicionado aqui.
// Se ausente, EventStore.Append() rejeita o evento — nada chega ao ledger.
func ValidateEventType(eventType string) error {
	validTypes := map[string]bool{
		// ── Estudante ───────────────────────────────────────────────────────
		"EstudanteCriado":                    true,
		"EstudanteCriadoComVinculo":          true,
		"DadosPessoaisAtualizados":           true,
		"DadosAcademicosAtualizados":         true,
		"StatusEscolarFundamentalAtualizado": true,
		"StatusEscolarMedioAtualizado":       true,
		"StatusSuperiorAtualizado":           true,
		"CursoAlterado":                      true,
		"AprovacaoAnoRegistrada":             true,
		"SenhaAlterada":                      true,
		// FIX-C3: evento de verificação de email do estudante via event sourcing
		"EmailVerificadoEstudante": true,
		// FIX-C11: "StatusEscolarAtualizado" REMOVIDO — evento fantasma:
		//   não existe aggregate que emite este evento, nem handler na projeção.
		//   Era entrada morta que indicava desorganização na whitelist.

		// ── Academia ────────────────────────────────────────────────────────
		"AcademiaCriada":           true,
		"AcademiaAtivada":          true,
		"AcademiaDesativada":       true,
		"AcademiaDadosAtualizados": true,
		"CursosAtualizados":        true,
		"AcademiaSenhaAlterada":    true,
		// FIX C11: "AnoLetivoDefinido" REMOVIDO — evento fantasma.

		// ── Admin ───────────────────────────────────────────────────────────
		"AdminCriado":         true,
		"AdminAtivado":        true,
		"AdminDesativado":     true,
		"AcaoAdminRegistrada": true,
		"AdminDadosAtualizados": true,
		"AdminRoleAtualizado":   true,
		// EmailVerificado é compartilhado entre Admin e Academia
		"EmailVerificado":  true,
		"AdminSenhaAlterada": true,

		// ── Notas e Faltas ───────────────────────────────────────────────────
		"NotasRegistradas": true,
		// "NotaAtualizada" é o evento real emitido pelo aggregate Estudante.
		// "NotasAtualizadas" (plural) mantido por compatibilidade com eventos históricos.
		"NotaAtualizada":  true,
		"NotasAtualizadas": true,
		"FaltasRegistradas": true,

		// ── Avaliação e Aprovação ────────────────────────────────────────────
		"AvaliacaoFinalAnoAcademico": true,

		// ── Turma ───────────────────────────────────────────────────────────
		"TurmaCriada":            true,
		"TurmaAtivada":           true,
		"TurmaDesativada":        true,
		"EstudanteAdicionadoTurma": true,
		"EstudanteRemovidoTurma":   true,
		"TurmaDadosAtualizados":  true,
		"TurmaDeletada":          true,

		// ── Curso ───────────────────────────────────────────────────────────
		"CursoCriado":         true,
		"CursoAtivado":        true,
		"CursoDesativado":     true,
		"CursoDadosAtualizados": true,

		// ── MateriaDisciplinar ───────────────────────────────────────────────
		"MateriaCriada":         true,
		"MateriaAtivada":        true,
		"MateriaDesativada":     true,
		"MateriaDadosAtualizados": true,

		// ── SistemaConfig ────────────────────────────────────────────────────
		"AnoLetivoDefinido": true,

		// ── Categorias de Nota ───────────────────────────────────────────────
		"CategoriaNotaAdicionada": true,
	}

	if !validTypes[eventType] {
		return fmt.Errorf("tipo de evento inválido: %s", eventType)
	}
	return nil
}

// ValidateAggregateType verifica se o tipo de aggregate é permitido.
func ValidateAggregateType(aggregateType string) error {
	validTypes := map[string]bool{
		"Estudante":          true,
		"Academia":           true,
		"Admin":              true,
		"Curso":              true,
		"MateriaDisciplinar": true,
		"SistemaConfig":      true,
		"Turma":              true,
	}

	if !validTypes[aggregateType] {
		return fmt.Errorf("tipo de aggregate inválido: %s", aggregateType)
	}
	return nil
}

// ValidateOffset valida e sanitiza um offset para paginação.
// Retorna 0 se o valor for negativo ou inválido.
func ValidateOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// ValidateLimit valida e sanitiza um limit para paginação.
// Retorna defaultLimit se o valor for inválido.
// Nunca retorna mais que maxLimit.
func ValidateLimit(limit, defaultLimit, maxLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// safeIdentifierRegex valida identificadores SQL seguros.
// Permite apenas letras, números e underscores.
var safeIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// SafeString verifica se uma string é um identificador SQL seguro.
// Use apenas para nomes de colunas ou tabelas (nunca para valores — use $1..$N).
func SafeString(s string) bool {
	return safeIdentifierRegex.MatchString(s) && !strings.Contains(s, "--")
}