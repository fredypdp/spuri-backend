// ============================================================================
// ARQUIVO: internal/db/safe_queries.go
//
// Funções de validação e sanitização para queries SQL.
// TODAS as queries dinâmicas devem usar estas funções.
//
// CORREÇÕES APLICADAS (Etapa 1 — pré-existentes):
//   FIX-ERR1 — ValidateLimit volta a receber 1 argumento.
//   FIX-ERR2 — ValidateTableName adicionada.
//   FIX-WL1  — EmailVerificadoEstudante adicionado.
//   FIX-WL2  — StatusEscolarAtualizado REMOVIDO (evento fantasma).
//
// CORREÇÕES APLICADAS (Etapa 2 — auditoria-etapa2-db.md):
//   FIX-WL-01 — "AdminAcaoRegistrada" corrigido para "AcaoAdminRegistrada"
//               (nome emitido pelo aggregate Admin).
//   FIX-WL-02 — "AdminRoleAtualizado" adicionado (estava ausente).
//   FIX-WL-03 — "NotasAtualizadas" (fantasma) removido; "NotasRegistradas"
//               adicionado (nome real emitido pelo aggregate Estudante).
//   FIX-WL-04 — "MateriaPeriodoDefinido" adicionado (estava ausente).
//   FIX-WL-05 — "MateriaDeletada" adicionado (estava ausente).
//   FIX-WL-06 — "CursoDeletado" adicionado (estava ausente).
//   FIX-WL-07 — "EstudanteAdicionadoTurma" corrigido para
//               "EstudanteAdicionadoATurma" (nome emitido pelo aggregate Turma).
//   FIX-WL-08 — "EstudanteRemovidoTurma" corrigido para
//               "EstudanteRemovidoDaTurma" (nome emitido pelo aggregate Turma).
// ============================================================================

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
		"EstudanteCriado":           true,
		"EstudanteCriadoComVinculo": true,
		"DadosPessoaisAtualizados":  true,
		"DadosAcademicosAtualizados": true,
		"SenhaAlterada":             true,
		"CursoAlterado":             true,

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
		// FIX-WL-01: era "AdminAcaoRegistrada" — nome correto é "AcaoAdminRegistrada"
		"AcaoAdminRegistrada": true,
		// FIX-WL-02: ausente — aggregate Admin emite "AdminRoleAtualizado"
		"AdminRoleAtualizado": true,

		// ── Aprovação e Avaliação ─────────────────────────────────────────────
		"AprovacaoAnoRegistrada":     true,
		"AvaliacaoFinalAnoAcademico": true,

		// ── Notas e Faltas ────────────────────────────────────────────────────
		"NotaAtualizada": true,
		// FIX-WL-03: "NotasAtualizadas" era fantasma — nome correto é "NotasRegistradas"
		"NotasRegistradas":  true,
		"FaltasRegistradas": true,

		// ── Turma ─────────────────────────────────────────────────────────────
		"TurmaCriada":      true,
		"TurmaAtivada":     true,
		"TurmaDesativada":  true,
		"TurmaDadosAtualizados": true,
		"TurmaDeletada":    true,
		// FIX-WL-07: era "EstudanteAdicionadoTurma" — nome correto emitido pelo aggregate
		"EstudanteAdicionadoATurma": true,
		// FIX-WL-08: era "EstudanteRemovidoTurma" — nome correto emitido pelo aggregate
		"EstudanteRemovidoDaTurma": true,

		// ── Curso ─────────────────────────────────────────────────────────────
		"CursoCriado":           true,
		"CursoAtivado":          true,
		"CursoDesativado":       true,
		"CursoDadosAtualizados": true,
		// FIX-WL-06: ausente — aggregate Curso emite "CursoDeletado"
		"CursoDeletado": true,

		// ── MateriaDisciplinar ────────────────────────────────────────────────
		"MateriaCriada":           true,
		"MateriaAtivada":          true,
		"MateriaDesativada":       true,
		"MateriaDadosAtualizados": true,
		// FIX-WL-04: ausente — aggregate MateriaDisciplinar emite "MateriaPeriodoDefinido"
		"MateriaPeriodoDefinido": true,
		// FIX-WL-05: ausente — aggregate MateriaDisciplinar emite "MateriaDeletada"
		"MateriaDeletada": true,

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
// FIX-ERR2: era usada em migrations.go mas não estava definida.
func ValidateTableName(name string) error {
	if name == "" {
		return fmt.Errorf("nome de tabela não pode ser vazio")
	}
	if !safeIdentifierRegex.MatchString(name) {
		return fmt.Errorf("nome de tabela inválido: %q (use apenas letras, números e underscores)", name)
	}
	return nil
}