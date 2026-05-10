package db

import (
	"fmt"
	"regexp"
	"strings"
)

var validEventTypes = map[string]bool{
	"SchemaCreated": true,
	// ── Estudante ────────────────────────────────────────────────────────────
	"EstudanteCriadoComVinculo":  true,
	"DadosPessoaisAtualizados":   true,
	"DadosAcademicosAtualizados": true,
	"SenhaAlterada":              true,
	"CursoAlterado":              true,
	// ── Status Escolar ───────────────────────────────────────────────────────
	"StatusEscolarFundamentalAtualizado": true,
	"StatusEscolarMedioAtualizado":       true,
	"StatusSuperiorAtualizado":           true,
	// ── Email Estudante ──────────────────────────────────────────────────────
	"EmailVerificadoEstudante": true,
	// ── Avaliação Final ───────────────────────────────────────────────────────
	"AvaliacaoFinalEscolar":      true,
	"AvaliacaoFinalSuperior":     true,

	// ── Academia ─────────────────────────────────────────────────────────────
	"AcademiaCriada":            true,
	"AcademiaAtivada":           true,
	"AcademiaDesativada":        true,
	"AcademiaDadosAtualizados":  true,
	"CursosAtualizados":         true,
	"AcademiaSenhaAlterada":     true,
	"CategoriaNotaAdicionada":   true,
	"AnoLetivoAcademiaDefinido": true,
	// ── Email Academia / Admin (compartilhado) ───────────────────────────────
	"EmailVerificado": true,
	// ── Admin ────────────────────────────────────────────────────────────────
	"AdminCriado":           true,
	"AdminAtivado":          true,
	"AdminDesativado":       true,
	"AdminDadosAtualizados": true,
	"AdminSenhaAlterada":    true,
	"AcaoAdminRegistrada":   true,
	"AdminRoleAtualizado":   true,
	// ── Notas e Faltas ────────────────────────────────────────────────────────
	"NotaAtualizada":    true,
	"NotasRegistradas":  true,
	"NotaDeletada":      true,
	"FaltasRegistradas": true,
	"FaltaRegistrada":   true,
	"FaltaAtualizada":   true,
	"FaltaDeletada":     true,
	// ── Turma ─────────────────────────────────────────────────────────────────
	"TurmaCriada":               true,
	"TurmaAtivada":              true,
	"TurmaDesativada":           true,
	"TurmaDadosAtualizados":     true,
	"TurmaDeletada":             true,
	"TurmaEncerrada":            true,
	"EstudanteAdicionadoATurma": true,
	"EstudanteRemovidoDaTurma":  true,
	// ── Curso ─────────────────────────────────────────────────────────────────
	"CursoCriado":           true,
	"CursoAtivado":          true,
	"CursoDesativado":       true,
	"CursoDadosAtualizados": true,
	"CursoDeletado":         true,
	// ── MateriaDisciplinar ────────────────────────────────────────────────────
	"MateriaCriada":           true,
	"MateriaAtivada":          true,
	"MateriaDesativada":       true,
	"MateriaDadosAtualizados": true,
	"MateriaPeriodoDefinido":  true,
	"MateriaDeletada":         true,
	// ── TelefoneExtra ──────────────────────────────────────────────────────────
	"TelefoneExtraAdicionado": true,
	"TelefoneExtraVerificado": true,
}

// validAggregateTypes é o mapa canônico de aggregate types permitidos no ledger.
var validAggregateTypes = map[string]bool{
	"Estudante":          true,
	"Academia":           true,
	"Admin":              true,
	"Curso":              true,
	"MateriaDisciplinar": true,
	"Turma":              true,
	"TelefoneExtra":      true,
	"System":             true,
}

// ValidateEventType verifica se o tipo de evento é permitido.
func ValidateEventType(eventType string) error {
	if !validEventTypes[eventType] {
		return fmt.Errorf("tipo de evento inválido: %s", eventType)
	}
	return nil
}

// ValidateAggregateType verifica se o tipo de aggregate é permitido.
func ValidateAggregateType(aggregateType string) error {
	if !validAggregateTypes[aggregateType] {
		return fmt.Errorf("tipo de aggregate inválido: %s", aggregateType)
	}
	return nil
}

// RegisteredEventTypes retorna a lista de event types atualmente na whitelist.
func RegisteredEventTypes() []string {
	types := make([]string, 0, len(validEventTypes))
	for k := range validEventTypes {
		types = append(types, k)
	}
	return types
}

// RegisteredAggregateTypes retorna a lista de aggregate types atualmente na whitelist.
func RegisteredAggregateTypes() []string {
	types := make([]string, 0, len(validAggregateTypes))
	for k := range validAggregateTypes {
		types = append(types, k)
	}
	return types
}

// ValidateOffset valida e sanitiza um offset para paginação.
func ValidateOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

// ValidateLimit valida e sanitiza um limit para paginação.
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

var safeIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// SafeString verifica se uma string é um identificador SQL seguro.
func SafeString(s string) bool {
	return safeIdentifierRegex.MatchString(s) && !strings.Contains(s, "--")
}

// ValidateTableName verifica se um nome de tabela é seguro para uso em queries dinâmicas.
func ValidateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("nome de tabela não pode ser vazio")
	}
	if !safeIdentifierRegex.MatchString(name) {
		return fmt.Errorf("nome de tabela inválido: %q (use apenas letras, números e underscores)", name)
	}
	return nil
}
