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
		"projection_estudantes":        true,
		"projection_academias":         true,
		"projection_admins":            true,
		"projection_notas":             true,
		"projection_faltas":            true,
		"projection_inscricoes":        true,
		"projection_cursos":            true,
		"projection_materias":          true,
		"spuri_ledger":                 true,
		"auth_tokens":                  true,
		"projection_checkpoints":       true,
		"projection_turmas":            true,
		"projection_reprovacoes":       true,
		"projection_aprovacao_ano":     true,
		"projection_categorias_nota":   true,
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

// SafeString escapa string para SQL (último recurso — queries com $1,$2 são preferíveis)
func SafeString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// ValidateStatus valida status (whitelist)
func ValidateStatus(status string) error {
	validStatuses := map[string]bool{
		"ativo":        true,
		"inativo":      true,
		"deletado":     true,
		"espera":       true,
		"aprovado":     true,
		"reprovado":    true,
		"em_andamento": true,
		"finalizado":   true,
	}

	if !validStatuses[status] {
		return fmt.Errorf("status inválido: %s", status)
	}
	return nil
}

func ValidateEventType(eventType string) error {
	validTypes := map[string]bool{
		// Estudante
		"EstudanteCriado":                    true,
		"EstudanteCriadoComVinculo":          true,
		"NotasRegistradas":                   true,
		"NotaAtualizada":                     true,
		"FaltasRegistradas":                  true,
		"EstudanteInscrito":                  true,
		"InscricaoAprovada":                  true,
		"InscricaoReprovada":                 true,
		"EstudanteVinculado":                 true,
		// REMOVIDO: "StatusEscolarAtualizado" — substituído pelos dois abaixo na migration 008
		"StatusEscolarFundamentalAtualizado": true,
		"StatusEscolarMedioAtualizado":       true,
		"StatusSuperiorAtualizado":           true,
		"DadosPessoaisAtualizados":           true,
		"DadosAcademicosAtualizados":         true,
		"AprovacaoAnoRegistrada":             true,
		"AvaliacaoFinalAnoAcademico":         true,
		"CursoAlterado":                      true,
		"EmailVerificado":                    true,

		// Academia
		"AcademiaCriada":           true,
		"AcademiaAtivada":          true,
		"AcademiaDesativada":       true,
		"AcademiaDadosAtualizados": true,
		"CursosAtualizados":        true,
		"AnoLetivoDefinido":        true,
		"CategoriaNotaAdicionada":  true,

		// Admin
		"AdminCriado":          true,
		"AdminAtivado":         true,
		"AdminDesativado":      true,
		"AcaoAdminRegistrada":  true,
		"AdminDadosAtualizados": true,
		"AdminRoleAtualizado":  true,

		// Curso
		"CursoCriado":          true,
		"CursoAtivado":         true,
		"CursoDesativado":      true,
		"CursoDadosAtualizados": true,
		"CursoDeletado":        true,

		// MateriaDisciplinar
		"MateriaCriada":           true,
		"MateriaAtivada":          true,
		"MateriaDesativada":       true,
		"MateriaDadosAtualizados": true,
		"MateriaPeriodoDefinido":  true,
		"MateriaDeletada":         true,

		// Turma
		"TurmaCriada":               true,
		"TurmaAtivada":              true,
		"TurmaDesativada":           true,
		"TurmaDadosAtualizados":     true,
		"EstudanteAdicionadoATurma": true,
		"EstudanteRemovidoDaTurma":  true,
		"TurmaDeletada":             true,
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