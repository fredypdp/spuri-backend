package db

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// ValidateUUID garante que string é UUID válido
func ValidateUUID(id string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("UUID inválido")
	}
	return parsed, nil
}

// ValidateTableName valida nome de tabela (whitelist)
func ValidateTableName(table string) error {
	validTables := map[string]bool{
		"projection_estudantes":      true,
		"projection_academias":       true,
		"projection_admins":          true,
		"projection_notas":           true,
		"projection_faltas":          true,
		"projection_inscricoes":      true,
		"projection_cursos":          true,
		"projection_materias":        true,
		"spuri_ledger":               true,
		"auth_tokens":                true,
		"projection_checkpoints":     true,
		"projection_turmas":          true,
		"projection_categorias_nota": true,
	}

	if !validTables[table] {
		return fmt.Errorf("tabela inválida: %s", table)
	}
	return nil
}

// ValidateColumnName valida nome de coluna (apenas alfanumérico e _)
func ValidateColumnName(column string) error {
	re := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	if !re.MatchString(column) {
		return fmt.Errorf("nome de coluna inválido: %s", column)
	}
	return nil
}

// SafeString escapa string para SQL (último recurso — preferir prepared statements)
func SafeString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// ValidateStatus valida status (whitelist)
func ValidateStatus(status string) error {
	validStatuses := map[string]bool{
		"ativo":     true,
		"inativo":   true,
		"espera":    true,
		"aprovado":  true,
		"reprovado": true,
	}

	if !validStatuses[status] {
		return fmt.Errorf("status inválido: %s", status)
	}
	return nil
}

// ValidateEventType valida tipo de evento (whitelist).
// CORRIGIDO: "AdminSenhaAlterada" adicionado — necessário para que o evento
// seja gravado no spuri_ledger via EventStore.Append().
func ValidateEventType(eventType string) error {
	validTypes := map[string]bool{
		// Estudante
		"EstudanteCriado":                     true,
		"EstudanteCriadoComVinculo":           true,
		"DadosPessoaisAtualizados":            true,
		"DadosAcademicosAtualizados":          true,
		"StatusEscolarAtualizado":             true,
		"StatusEscolarFundamentalAtualizado":  true,
		"StatusEscolarMedioAtualizado":        true,
		"StatusSuperiorAtualizado":            true,
		"CursoAlterado":                       true,
		"EstudanteVinculado":                  true,
		"AprovacaoAnoRegistrada":              true,
		// Academia
		"AcademiaCriada":         true,
		"AcademiaAtivada":        true,
		"AcademiaDesativada":     true,
		"AcademiaDadosAtualizados": true,
		"CursosAtualizados":      true,
		"AnoLetivoDefinido":      true,
		// Admin
		"AdminCriado":          true,
		"AdminAtivado":         true,
		"AdminDesativado":      true,
		"AcaoAdminRegistrada":  true,
		"AdminDadosAtualizados": true,
		"AdminRoleAtualizado":  true,
		"EmailVerificado":      true,
		// CORRIGIDO: novo evento de troca de senha via event sourcing
		"AdminSenhaAlterada": true,
		// Inscrições
		"EstudanteInscrito":   true,
		"InscricaoAprovada":   true,
		"InscricaoReprovada":  true,
		// Notas e Faltas
		"NotasRegistradas":  true,
		"NotasAtualizadas":  true,
		"FaltasRegistradas": true,
		// Cursos
		"CursoCriado":          true,
		"CursoAtivado":         true,
		"CursoDesativado":      true,
		"CursoDadosAtualizados": true,
		"CursoDeletado":        true,
		// Matérias
		"MateriaCriada":          true,
		"MateriaAtivada":         true,
		"MateriaDesativada":      true,
		"MateriaDadosAtualizados": true,
		"MateriaPeriodoDefinido": true,
		"MateriaDeletada":        true,
		// Turmas
		"TurmaCriada":              true,
		"TurmaAtivada":             true,
		"TurmaDesativada":          true,
		"TurmaDadosAtualizados":    true,
		"EstudanteAdicionadoATurma": true,
		"EstudanteRemovidoDaTurma": true,
		"TurmaDeletada":            true,
		// Avaliação e Categorias
		"AvaliacaoFinalRegistrada": true,
		"CategoriaNotaAdicionada": true,
	}

	if !validTypes[eventType] {
		return fmt.Errorf("tipo de evento inválido: %s", eventType)
	}
	return nil
}

// ValidateAggregateType valida tipo de agregado (whitelist)
func ValidateAggregateType(aggType string) error {
	validTypes := map[string]bool{
		"Estudante":          true,
		"Academia":           true,
		"Admin":              true,
		"Curso":              true,
		"MateriaDisciplinar": true,
		"SistemaConfig":      true,
		"Turma":              true,
	}

	if !validTypes[aggType] {
		return fmt.Errorf("tipo de agregado inválido: %s", aggType)
	}
	return nil
}

// ValidateLimit valida limite de paginação
func ValidateLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

// ValidateOffset valida offset de paginação
func ValidateOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}