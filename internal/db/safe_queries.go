package db

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidateEventType verifica se o tipo de evento é permitido.
// TODOS os eventos emitidos pelos aggregates devem estar listados aqui.
// Um evento ausente é silenciosamente rejeitado por EventStore.AppendTx(),
// fazendo com que o Save retorne erro e o evento nunca chegue ao ledger.
func ValidateEventType(eventType string) error {
	validTypes := map[string]bool{
		// ── Estudante ────────────────────────────────────────────────────────
		"EstudanteCriado":            true,
		"EstudanteCriadoComVinculo":  true,
		"DadosPessoaisAtualizados":   true,
		"DadosAcademicosAtualizados": true,
		"SenhaAlterada":              true,
		"CursoAlterado":              true,

		// ── Status Escolar ───────────────────────────────────────────────────
		// "StatusEscolarAtualizado" REMOVIDO — evento fantasma (FIX-WL2 Etapa 1).
		"StatusEscolarFundamentalAtualizado": true,
		"StatusEscolarMedioAtualizado":       true,
		"StatusSuperiorAtualizado":           true,

		// ── Email Estudante ──────────────────────────────────────────────────
		// Nome distinto de "EmailVerificado" (Admin/Academia) para evitar ambiguidade.
		"EmailVerificadoEstudante": true,

		// ── Academia ─────────────────────────────────────────────────────────
		"AcademiaCriada":           true,
		"AcademiaAtivada":          true,
		"AcademiaDesativada":       true,
		"AcademiaDadosAtualizados": true,
		"CursosAtualizados":        true,
		"EmailVerificado":          true,
		"AcademiaSenhaAlterada":    true,

		// ── Admin ────────────────────────────────────────────────────────────
		"AdminCriado":           true,
		"AdminAtivado":          true,
		"AdminDesativado":       true,
		"AdminDadosAtualizados": true,
		"AdminSenhaAlterada":    true,
		"AcaoAdminRegistrada":   true,
		"AdminRoleAtualizado":   true,

		// ── Aprovação e Avaliação ─────────────────────────────────────────────
		"AprovacaoAnoRegistrada":     true,
		"AvaliacaoFinalAnoAcademico": true,

		// ── Notas e Faltas ────────────────────────────────────────────────────
		"NotaAtualizada":    true,
		"NotasRegistradas":  true,
		"FaltasRegistradas": true,

		// ── Turma ─────────────────────────────────────────────────────────────
		"TurmaCriada":              true,
		"TurmaAtivada":             true,
		"TurmaDesativada":          true,
		"TurmaDadosAtualizados":    true,
		"TurmaDeletada":            true,
		"EstudanteAdicionadoATurma": true,
		"EstudanteRemovidoDaTurma":  true,

		// ── Curso ─────────────────────────────────────────────────────────────
		"CursoCriado":           true,
		"CursoAtivado":          true,
		"CursoDesativado":       true,
		"CursoDadosAtualizados": true,
		"CursoDeletado":         true,

		// ── MateriaDisciplinar ────────────────────────────────────────────────
		"MateriaCriada":           true,
		"MateriaAtivada":          true,
		"MateriaDesativada":       true,
		"MateriaDadosAtualizados": true,
		"MateriaPeriodoDefinido":  true,
		"MateriaDeletada":         true,

		// ── SistemaConfig ─────────────────────────────────────────────────────
		"AnoLetivoDefinido": true,

		// ── Categorias de Nota ────────────────────────────────────────────────
		"CategoriaNotaAdicionada": true,

		// ── Sistema (evento interno de migration — DB-08 FIX) ─────────────────
		// O evento SchemaCreated é inserido diretamente pela migration 001 no ledger.
		// Está na whitelist para que replay/testes não rejeitem o evento ao chamar AppendTx.
		"SchemaCreated": true,
	}

	if !validTypes[eventType] {
		return fmt.Errorf("tipo de evento inválido: %s", eventType)
	}
	return nil
}

// ValidateAggregateType verifica se o tipo de aggregate é permitido.
//
// DB-08 FIX: "System" adicionado para cobrir o evento SchemaCreated inserido
// pela migration 001 com aggregate_type = 'System'. Sem isso, qualquer
// reprocessamento ou validação de integridade desse evento retornaria falso
// negativo ao chamar ValidateAggregateType.
//
// DB-09 NOTA: "Aprovacao", "AvaliacaoFinal" e "Reprovacao" não existem como
// aggregates próprios — esses eventos são emitidos pelo aggregate "Estudante".
// Se no futuro esses eventos forem movidos para aggregates dedicados, adicionar
// os tipos correspondentes aqui imediatamente.
func ValidateAggregateType(aggregateType string) error {
	validTypes := map[string]bool{
		"Estudante":          true,
		"Academia":           true,
		"Admin":              true,
		"Curso":              true,
		"MateriaDisciplinar": true,
		"SistemaConfig":      true,
		"Turma":              true,
		// DB-08 FIX: aggregate virtual usado pela migration 001 para o evento SchemaCreated.
		"System": true,
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
func ValidateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("nome de tabela não pode ser vazio")
	}
	if !safeIdentifierRegex.MatchString(name) {
		return fmt.Errorf("nome de tabela inválido: %q (use apenas letras, números e underscores)", name)
	}
	return nil
}