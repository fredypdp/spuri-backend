package db_test

// whitelist_test.go — FIX DB-09
//
// PROBLEMA:
//   A whitelist em safe_queries.go (ValidateEventType / ValidateAggregateType)
//   não tem verificação em compile-time. Um aggregate novo ou um evento novo
//   adicionado ao domínio é silenciosamente rejeitado na primeira chamada a
//   AppendTx(), causando erro em produção sem aviso em desenvolvimento.
//
// SOLUÇÃO:
//   Testes que verificam, para cada aggregate registrado na factory, que:
//     1. O aggregate_type está na whitelist de ValidateAggregateType.
//     2. Cada evento emitido pelo aggregate está na whitelist de ValidateEventType.
//
//   Executar:
//     go test ./internal/db/... -run TestWhitelist -v
//
// MANUTENÇÃO:
//   Ao adicionar novo aggregate: inclua em aggregateTypes (abaixo) E em
//   validAggregateTypes (safe_queries.go).
//
//   Ao adicionar novo evento: inclua em allDomainEventTypes (abaixo) E em
//   validEventTypes (safe_queries.go).
//
//   O teste vai FALHAR até que ambos os locais sejam atualizados. Isso é intencional.

import (
	"testing"

	"spuri/internal/db"
)

// aggregateTypes espelha os cases em DefaultAggregateFactory.Create().
// MANUTENÇÃO: ao adicionar novo aggregate, inclua aqui E em safe_queries.go.
var aggregateTypes = []string{
	"Estudante",
	"Academia",
	"Admin",
	"Curso",
	"MateriaDisciplinar",
	"SistemaConfig",
	"Turma",
	"System", // aggregate virtual do evento de bootstrap (migration 001)
}

// allDomainEventTypes espelha os event types emitidos por todos os aggregates.
// MANUTENÇÃO: ao adicionar novo evento, inclua aqui E em safe_queries.go.
var allDomainEventTypes = []string{
	// ── Bootstrap ────────────────────────────────────────────────────────
	"SchemaCreated",

	// ── Estudante ────────────────────────────────────────────────────────
	"EstudanteCriado",
	"EstudanteCriadoComVinculo",
	"DadosPessoaisAtualizados",
	"DadosAcademicosAtualizados",
	"SenhaAlterada",
	"CursoAlterado",

	// ── Status Escolar ───────────────────────────────────────────────────
	"StatusEscolarFundamentalAtualizado",
	"StatusEscolarMedioAtualizado",
	"StatusSuperiorAtualizado",

	// ── Email Estudante ──────────────────────────────────────────────────
	"EmailVerificadoEstudante",

	// ── Inscrição ────────────────────────────────────────────────────────
	"EstudanteInscrito",
	"InscricaoAprovada",
	"InscricaoReprovada",

	// ── Aprovação / Avaliação ────────────────────────────────────────────
	"AprovacaoAnoRegistrada",
	"AvaliacaoFinalAnoAcademico",

	// ── Turma ────────────────────────────────────────────────────────────
	"TurmaCriada",
	"TurmaAtivada",
	"TurmaDesativada",
	"TurmaDadosAtualizados",
	"TurmaDeletada",
	"TurmaEncerrada",
	"EstudanteAdicionadoATurma",
	"EstudanteAdicionadoNaTurma",
	"EstudanteRemovidoDaTurma",

	// ── Notas / Faltas ───────────────────────────────────────────────────
	"NotasRegistradas",
	"NotaAtualizada",
	"FaltasRegistradas",
	"FaltaRegistrada",

	// ── Academia ─────────────────────────────────────────────────────────
	"AcademiaCriada",
	"AcademiaAtivada",
	"AcademiaDesativada",
	"AcademiaDadosAtualizados",
	"CursosAtualizados",
	"AcademiaSenhaAlterada",
	"CategoriaNotaAdicionada",

	// ── Email compartilhado (Academia + Admin) ───────────────────────────
	"EmailVerificado",

	// ── Admin ────────────────────────────────────────────────────────────
	"AdminCriado",
	"AdminAtivado",
	"AdminDesativado",
	"AcaoAdminRegistrada",
	"AdminDadosAtualizados",
	"AdminRoleAtualizado",
	"AdminSenhaAlterada",

	// ── Curso ────────────────────────────────────────────────────────────
	"CursoCriado",
	"CursoAtivado",
	"CursoDesativado",
	"CursoDadosAtualizados",
	"CursoDeletado",

	// ── MateriaDisciplinar ───────────────────────────────────────────────
	"MateriaCriada",
	"MateriaAtivada",
	"MateriaDesativada",
	"MateriaDadosAtualizados",
	"MateriaPeriodoDefinido",
	"MateriaDeletada",

	// ── SistemaConfig ────────────────────────────────────────────────────
	"AnoLetivoDefinido",
}

// TestWhitelistAggregateTypes verifica que cada aggregate type do domínio
// está registrado em ValidateAggregateType (safe_queries.go).
func TestWhitelistAggregateTypes(t *testing.T) {
	for _, aggType := range aggregateTypes {
		aggType := aggType
		t.Run(aggType, func(t *testing.T) {
			if err := db.ValidateAggregateType(aggType); err != nil {
				t.Errorf(
					"aggregate type %q não está na whitelist — adicione-o em internal/db/safe_queries.go: %v",
					aggType, err,
				)
			}
		})
	}
}

// TestWhitelistEventTypes verifica que cada tipo de evento do domínio
// está registrado em ValidateEventType (safe_queries.go).
func TestWhitelistEventTypes(t *testing.T) {
	for _, eventType := range allDomainEventTypes {
		eventType := eventType
		t.Run(eventType, func(t *testing.T) {
			if err := db.ValidateEventType(eventType); err != nil {
				t.Errorf(
					"event type %q não está na whitelist — adicione-o em internal/db/safe_queries.go: %v",
					eventType, err,
				)
			}
		})
	}
}

// TestWhitelistNoOrphans verifica que não há entradas na whitelist sem
// correspondência no domínio — detecta dead entries acumuladas ao longo do tempo.
func TestWhitelistNoOrphans(t *testing.T) {
	knownAggTypes := make(map[string]bool, len(aggregateTypes))
	for _, a := range aggregateTypes {
		knownAggTypes[a] = true
	}

	knownEventTypes := make(map[string]bool, len(allDomainEventTypes))
	for _, e := range allDomainEventTypes {
		knownEventTypes[e] = true
	}

	for _, registered := range db.RegisteredAggregateTypes() {
		if !knownAggTypes[registered] {
			t.Errorf(
				"aggregate type %q está na whitelist mas não encontrado no domínio — "+
					"remova de safe_queries.go ou adicione ao aggregateTypes do teste",
				registered,
			)
		}
	}

	for _, registered := range db.RegisteredEventTypes() {
		if !knownEventTypes[registered] {
			t.Errorf(
				"event type %q está na whitelist mas não encontrado no domínio — "+
					"remova de safe_queries.go ou adicione ao allDomainEventTypes do teste",
				registered,
			)
		}
	}
}
