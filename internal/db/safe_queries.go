package db

import (
	"fmt"
	"regexp"
	"strings"
)

// validEventTypes é o mapa canônico de event types permitidos no ledger.
// Exportado indiretamente via RegisteredEventTypes() para uso nos testes.
//
// MANUTENÇÃO: ao adicionar um novo evento no domínio, inclua-o aqui E no
// slice allDomainEventTypes em internal/db/whitelist_test.go.
// O teste TestWhitelistEventTypes falhará se o evento não estiver aqui,
// e TestWhitelistNoOrphans falhará se houver entrada sem correspondência no domínio.
var validEventTypes = map[string]bool{
	// ── Bootstrap ────────────────────────────────────────────────────────────
	// FIX DB-08: SchemaCreated é o evento gerado pela migration 001 (bootstrap).
	// Sem esta entrada, replay ou verificação de integridade rejeitaria o evento.
	"SchemaCreated": true,

	// ── Estudante ────────────────────────────────────────────────────────────
	"EstudanteCriado":            true,
	"EstudanteCriadoComVinculo":  true,
	"DadosPessoaisAtualizados":   true,
	"DadosAcademicosAtualizados": true,
	"SenhaAlterada":              true,
	"CursoAlterado":              true,

	// ── Status Escolar ───────────────────────────────────────────────────────
	// "StatusEscolarAtualizado" REMOVIDO — evento fantasma (FIX-WL2 Etapa 1).
	"StatusEscolarFundamentalAtualizado": true,
	"StatusEscolarMedioAtualizado":       true,
	"StatusSuperiorAtualizado":           true,

	// ── Email Estudante ──────────────────────────────────────────────────────
	// Nome distinto de "EmailVerificado" (Admin/Academia) para evitar ambiguidade.
	"EmailVerificadoEstudante": true,

	// ── Inscrição ────────────────────────────────────────────────────────────
	"EstudanteInscrito":   true,
	"InscricaoAprovada":   true,
	"InscricaoReprovada":  true,

	// ── Aprovação e Avaliação ─────────────────────────────────────────────────
	"AprovacaoAnoRegistrada":     true,
	"AvaliacaoFinalAnoAcademico": true,

	// ── Academia ─────────────────────────────────────────────────────────────
	"AcademiaCriada":           true,
	"AcademiaAtivada":          true,
	"AcademiaDesativada":       true,
	"AcademiaDadosAtualizados": true,
	"CursosAtualizados":        true,
	"AcademiaSenhaAlterada":    true,
	"CategoriaNotaAdicionada":  true,

	// ── Email Academia / Admin (compartilhado) ───────────────────────────────
	"EmailVerificado": true,

	// ── Admin ────────────────────────────────────────────────────────────────
	"AdminCriado":           true,
	"AdminAtivado":          true,
	"AdminDesativado":       true,
	"AdminDadosAtualizados": true,
	"AdminSenhaAlterada":    true,
	// FIX-WL-01: era "AdminAcaoRegistrada" — nome correto é "AcaoAdminRegistrada"
	"AcaoAdminRegistrada": true,
	// FIX-WL-02: ausente — aggregate Admin emite "AdminRoleAtualizado"
	"AdminRoleAtualizado": true,

	// ── Notas e Faltas ────────────────────────────────────────────────────────
	"NotaAtualizada": true,
	// FIX-WL-03: "NotasAtualizadas" era fantasma — nome correto é "NotasRegistradas"
	"NotasRegistradas":  true,
	"FaltasRegistradas": true,
	"FaltaRegistrada":   true,

	// ── Turma ─────────────────────────────────────────────────────────────────
	"TurmaCriada":           true,
	"TurmaAtivada":          true,
	"TurmaDesativada":       true,
	"TurmaDadosAtualizados": true,
	"TurmaDeletada":         true,
	"TurmaEncerrada":        true,
	// FIX-WL-07: era "EstudanteAdicionadoTurma" — nome correto emitido pelo aggregate
	"EstudanteAdicionadoATurma": true,
	"EstudanteAdicionadoNaTurma": true, // alias — aceitar ambos durante migração de nomes
	// FIX-WL-08: era "EstudanteRemovidoTurma" — nome correto emitido pelo aggregate
	"EstudanteRemovidoDaTurma": true,

	// ── Curso ─────────────────────────────────────────────────────────────────
	"CursoCriado":           true,
	"CursoAtivado":          true,
	"CursoDesativado":       true,
	"CursoDadosAtualizados": true,
	// FIX-WL-06: ausente — aggregate Curso emite "CursoDeletado"
	"CursoDeletado": true,

	// ── MateriaDisciplinar ────────────────────────────────────────────────────
	"MateriaCriada":           true,
	"MateriaAtivada":          true,
	"MateriaDesativada":       true,
	"MateriaDadosAtualizados": true,
	// FIX-WL-04: ausente — aggregate MateriaDisciplinar emite "MateriaPeriodoDefinido"
	"MateriaPeriodoDefinido": true,
	// FIX-WL-05: ausente — aggregate MateriaDisciplinar emite "MateriaDeletada"
	"MateriaDeletada": true,

	// ── SistemaConfig ─────────────────────────────────────────────────────────
	"AnoLetivoDefinido": true,
}

// validAggregateTypes é o mapa canônico de aggregate types permitidos no ledger.
// Exportado indiretamente via RegisteredAggregateTypes() para uso nos testes.
//
// MANUTENÇÃO: ao adicionar um novo aggregate, inclua-o aqui E no slice
// aggregateTypes em internal/db/whitelist_test.go.
var validAggregateTypes = map[string]bool{
	"Estudante":          true,
	"Academia":           true,
	"Admin":              true,
	"Curso":              true,
	"MateriaDisciplinar": true,
	"SistemaConfig":      true,
	"Turma":              true,
	// FIX DB-08: "System" é o aggregate_type usado pelo evento de bootstrap da migration 001.
	// Sem esta entrada, replay ou verificação de integridade rejeitaria o evento SchemaCreated.
	"System": true,
}

// ValidateEventType verifica se o tipo de evento é permitido.
// TODOS os eventos emitidos pelos aggregates devem estar listados aqui.
// Um evento ausente é silenciosamente rejeitado por EventStore.AppendTx(),
// fazendo com que o Save retorne erro e o evento nunca chegue ao ledger.
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
// Usado por TestWhitelistNoOrphans para detectar entradas obsoletas.
func RegisteredEventTypes() []string {
	types := make([]string, 0, len(validEventTypes))
	for k := range validEventTypes {
		types = append(types, k)
	}
	return types
}

// RegisteredAggregateTypes retorna a lista de aggregate types atualmente na whitelist.
// Usado por TestWhitelistNoOrphans para detectar entradas obsoletas.
func RegisteredAggregateTypes() []string {
	types := make([]string, 0, len(validAggregateTypes))
	for k := range validAggregateTypes {
		types = append(types, k)
	}
	return types
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