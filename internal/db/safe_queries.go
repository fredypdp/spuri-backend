// ============================================================================
// ARQUIVO: internal/db/safe_queries.go
//
// Funções de validação e sanitização para queries SQL.
// TODAS as queries dinâmicas devem usar estas funções.
//
// CORREÇÕES APLICADAS:
//   FIX-ERR1 — ValidateLimit volta a receber 1 argumento (estava quebrando
//               callers em event_store.go, manager.go). Defaults embutidos:
//               defaultLimit=50, maxLimit=1000.
//   FIX-ERR2 — ValidateTableName adicionada (era usada em migrations.go
//               mas não estava definida aqui, causando undefined error).
//   FIX-WL1  — EmailVerificadoEstudante adicionado à whitelist de eventos.
//   FIX-WL2  — StatusEscolarAtualizado REMOVIDO da whitelist (evento fantasma).
// ============================================================================

package db

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidateEventType verifica se o tipo de evento é permitido.
func ValidateEventType(eventType string) error {
	validTypes := map[string]bool{
		// ── Estudante ────────────────────────────────────────────────────────
		"EstudanteCriado":            true,
		"EstudanteCriadoComVinculo":  true,
		"DadosPessoaisAtualizados":   true,
		"DadosAcademicosAtualizados": true,
		"SenhaAlterada":              true,
		"CursoAlterado":              true,

		// ── Status Escolar ────────────────────────────────────────────────────
		// "StatusEscolarAtualizado" REMOVIDO — evento fantasma (FIX-WL2).
		"StatusEscolarFundamentalAtualizado": true,
		"StatusEscolarMedioAtualizado":       true,
		"StatusSuperiorAtualizado":           true,

		// ── Email Estudante ───────────────────────────────────────────────────
		// FIX-WL1: nome distinto para não colidir com "EmailVerificado" de Admin/Academia.
		"EmailVerificadoEstudante": true,

		// ── Academia ─────────────────────────────────────────────────────────
		"AcademiaCriada":           true,
		"AcademiaAtivada":          true,
		"AcademiaDesativada":       true,
		"AcademiaDadosAtualizados": true,
		"CursosAtualizados":        true,
		"EmailVerificado":          true,
		"AcademiaSenhaAlterada":    true,

		// ── Admin ─────────────────────────────────────────────────────────────
		"AdminCriado":           true,
		"AdminAtivado":          true,
		"AdminDesativado":       true,
		"AdminDadosAtualizados": true,
		"AdminSenhaAlterada":    true,
		"AdminAcaoRegistrada":   true,

		// ── Aprovação e Avaliação ─────────────────────────────────────────────
		"AprovacaoAnoRegistrada":     true,
		"AvaliacaoFinalAnoAcademico": true,

		// ── Notas e Faltas ────────────────────────────────────────────────────
		"NotaAtualizada":    true,
		"NotasAtualizadas":  true,
		"FaltasRegistradas": true,

		// ── Turma ─────────────────────────────────────────────────────────────
		"TurmaCriada":              true,
		"TurmaAtivada":             true,
		"TurmaDesativada":          true,
		"EstudanteAdicionadoTurma": true,
		"EstudanteRemovidoTurma":   true,
		"TurmaDadosAtualizados":    true,
		"TurmaDeletada":            true,

		// ── Curso ─────────────────────────────────────────────────────────────
		"CursoCriado":           true,
		"CursoAtivado":          true,
		"CursoDesativado":       true,
		"CursoDadosAtualizados": true,

		// ── MateriaDisciplinar ────────────────────────────────────────────────
		"MateriaCriada":           true,
		"MateriaAtivada":          true,
		"MateriaDesativada":       true,
		"MateriaDadosAtualizados": true,

		// ── SistemaConfig ─────────────────────────────────────────────────────
		"AnoLetivoDefinido": true,

		// ── Categorias de Nota ────────────────────────────────────────────────
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
// FIX-ERR1: 1 argumento — compatível com event_store.go, manager.go e qualquer outro caller.
// Default = 50; máximo = 1000.
func ValidateLimit(limit int) int {
	const defaultLimit = 50
	const maxLimit = 1000
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// safeIdentifierRegex valida identificadores SQL seguros.
var safeIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// SafeString verifica se uma string é um identificador SQL seguro.
// Use APENAS para nomes de tabelas/colunas — nunca para valores (use $1..$N).
func SafeString(s string) bool {
	return safeIdentifierRegex.MatchString(s) && !strings.Contains(s, "--")
}

// ValidateTableName verifica se um nome de tabela é seguro para uso em queries dinâmicas.
// FIX-ERR2: era usada em migrations.go mas não estava definida, causando "undefined: ValidateTableName".
func ValidateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("nome de tabela não pode ser vazio")
	}
	if !safeIdentifierRegex.MatchString(name) {
		return fmt.Errorf("nome de tabela inválido: %q (use apenas letras, números e underscores)", name)
	}
	return nil
}