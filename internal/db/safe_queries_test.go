package db

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestValidateEventTypeAcceptsEventsDiscoveredInCode(t *testing.T) {
	t.Parallel()

	eventTypes := []string{
		"SchemaCreated",
		"EstudanteCriadoComVinculo",
		"DadosPessoaisAtualizados",
		"DadosAcademicosAtualizados",
		"SenhaAlterada",
		"CursoAlterado",
		"MatriculaFundamentalEfetivada",
		"FundamentalRetomado",
		"FundamentalInterrompido",
		"EquivalenciaFundamentalReconhecida",
		"MatriculaMedioEfetivada",
		"MedioRetomado",
		"MedioInterrompido",
		"EquivalenciaMedioReconhecida",
		"MatriculaSuperiorEfetivada",
		"MatriculaSuperiorReativada",
		"IngressoSuperiorPorEquivalenciaRegistrado",
		"SuperiorTrancado",
		"SuperiorAbandonado",
		"EstudanteDesvinculadoDaAcademia",
		"EstudanteReintegrado",
		"EmailVerificadoEstudante",
		"AvaliacaoFinalEscolar",
		"AvaliacaoFinalSuperior",
		"AcademiaCriada",
		"AcademiaAtivada",
		"AcademiaDesativada",
		"AcademiaDadosAtualizados",
		"CursosAtualizados",
		"AcademiaSenhaAlterada",
		"CategoriaNotaAdicionada",
		"CategoriaNotaRemovida",
		"AnoLetivoAcademiaDefinido",
		"AnoLetivoAcademiaFinalizado",
		"AcademiaDocumentosObrigatoriosAtualizados",
		"EmailVerificado",
		"AdminCriado",
		"AdminAtivado",
		"AdminDesativado",
		"AdminDadosAtualizados",
		"AdminSenhaAlterada",
		"AcaoAdminRegistrada",
		"AdminRoleAtualizado",
		"NotaAtualizada",
		"NotasRegistradas",
		"NotaDeletada",
		"FaltasRegistradas",
		"FaltaRegistrada",
		"FaltaAtualizada",
		"FaltaDeletada",
		"TurmaCriada",
		"TurmaAtivada",
		"TurmaDesativada",
		"TurmaDadosAtualizados",
		"TurmaDeletada",
		"TurmaEncerrada",
		"EstudanteAdicionadoATurma",
		"EstudanteRemovidoDaTurma",
		"CursoCriado",
		"CursoAtivado",
		"CursoDesativado",
		"CursoDadosAtualizados",
		"CursoDeletado",
		"MateriaCriada",
		"MateriaAtivada",
		"MateriaDesativada",
		"MateriaDadosAtualizados",
		"MateriaPeriodoDefinido",
		"MateriaDeletada",
		"SolicitacaoMatriculaCriada",
		"SolicitacaoMatriculaAprovada",
		"SolicitacaoMatriculaReprovada",
	}

	for _, eventType := range eventTypes {
		eventType := eventType
		t.Run(eventType, func(t *testing.T) {
			t.Parallel()
			if err := ValidateEventType(eventType); err != nil {
				t.Fatalf("ValidateEventType(%q) unexpected error: %v", eventType, err)
			}
		})
	}
}

func TestValidateAggregateTypeAcceptsAggregatesDiscoveredInCode(t *testing.T) {
	t.Parallel()

	aggregateTypes := []string{
		"Estudante",
		"Academia",
		"Admin",
		"Curso",
		"MateriaDisciplinar",
		"Turma",
		"SolicitacaoMatricula",
		"System",
	}

	for _, aggregateType := range aggregateTypes {
		aggregateType := aggregateType
		t.Run(aggregateType, func(t *testing.T) {
			t.Parallel()
			if err := ValidateAggregateType(aggregateType); err != nil {
				t.Fatalf("ValidateAggregateType(%q) unexpected error: %v", aggregateType, err)
			}
		})
	}
}

func TestEventWhitelistCoversAllEventTypesDiscoveredInCode(t *testing.T) {
	t.Parallel()

	discovered, err := discoverEventTypesInCode()
	if err != nil {
		t.Fatalf("discover event types in code: %v", err)
	}

	missing := make([]string, 0)
	for eventType := range discovered {
		if err := ValidateEventType(eventType); err != nil {
			missing = append(missing, eventType)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Fatalf("event types discovered in code but missing from whitelist: %s", strings.Join(missing, ", "))
	}
}

func discoverEventTypesInCode() (map[string][]string, error) {
	eventTypes := make(map[string][]string)
	root := filepath.Join("..")

	eventTypePatterns := []*regexp.Regexp{
		regexp.MustCompile(`EventType:\s*"([A-Za-z0-9_]+)"`),
		regexp.MustCompile(`case\s+"([A-Za-z0-9_]+)"\s*:`),
		regexp.MustCompile(`"([A-Za-z0-9_]+)"\s*:\s*[^,\n]+handle[A-Za-z0-9_]*`),
		regexp.MustCompile(`GetEventType\(\)\s*==\s*"([A-Za-z0-9_]+)"`),
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if !isEventSourceFile(path) {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(content)

		for _, pattern := range eventTypePatterns {
			for _, match := range pattern.FindAllStringSubmatch(text, -1) {
				eventType := match[1]
				if !isDomainEventType(eventType) {
					continue
				}
				eventTypes[eventType] = append(eventTypes[eventType], filepath.ToSlash(path))
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return eventTypes, nil
}

func isEventSourceFile(path string) bool {
	normalized := filepath.ToSlash(path)
	return strings.Contains(normalized, "/domain/aggregates/") ||
		strings.Contains(normalized, "/projections/")
}

func isDomainEventType(value string) bool {
	if value == "" || strings.ToLower(value[:1]) == value[:1] {
		return false
	}
	if validAggregateTypes[value] {
		return false
	}

	return true
}
