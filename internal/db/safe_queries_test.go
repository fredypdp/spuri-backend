package db

import "testing"

func TestValidateEventTypeAcceptsEventsDiscoveredInCode(t *testing.T) {
	t.Parallel()

	eventTypes := []string{
		"SchemaCreated",
		"EstudanteCriadoComVinculo",
		"DadosPessoaisAtualizados",
		"DadosAcademicosAtualizados",
		"SenhaAlterada",
		"CursoAlterado",
		"FundamentalRetomado",
		"FundamentalInterrompido",
		"EquivalenciaFundamentalReconhecida",
		"MedioRetomado",
		"MedioInterrompido",
		"EquivalenciaMedioReconhecida",
		"MatriculaSuperiorReativada",
		"IngressoSuperiorPorEquivalenciaRegistrado",
		"SuperiorInterrompido",
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
		"NotasRegistradas",
		"FaltasRegistradas",
		"FaltaRegistrada",
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
		"SolicitacaoMatriculaCancelada",
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

func TestValidateEventTypeRejectsRemovedStudentProgressionEvents(t *testing.T) {
	t.Parallel()

	removed := []string{
		"MatriculaFundamentalEfetivada",
		"MatriculaMedioEfetivada",
		"MatriculaSuperiorEfetivada",
		"SuperiorTrancado",
	}

	for _, eventType := range removed {
		eventType := eventType
		t.Run(eventType, func(t *testing.T) {
			t.Parallel()
			if err := ValidateEventType(eventType); err == nil {
				t.Fatalf("ValidateEventType(%q) retornou nil, want erro", eventType)
			}
		})
	}
}

func TestValidateEventTypeRejectsRemovedPaymentEvents(t *testing.T) {
	t.Parallel()

	removed := []string{
		"Credenciais" + "Appy" + "PayCadastradas",
		"Credenciais" + "Appy" + "PayAtualizadas",
		"Credenciais" + "Appy" + "PayValidadas",
		"Credenciais" + "Appy" + "PayAtivadas",
		"Credenciais" + "Appy" + "PayDesativadas",
		"ModalidadePagamentoGlobalAlterada",
		"ModalidadePagamentoSpuriAlterada",
		"ModalidadePagamentoAcademiaAlterada",
		"CobrancaFinanceiraCriada",
		"CobrancaFinanceiraEnviadaAoProvider",
		"CobrancaFinanceiraStatusAtualizado",
		"CobrancaFinanceiraCancelada",
		"ReembolsoFinanceiroSolicitado",
		"ReembolsoFinanceiroStatusAtualizado",
		"ReversaoFinanceiraSolicitada",
		"ReversaoFinanceiraStatusAtualizado",
		"WebhookFinanceiroRecebido",
		"WebhookFinanceiroIgnoradoComoDuplicado",
		"DivergenciaFinanceiraDetectada",
		"DivergenciaFinanceiraReconciliada",
		"ReconciliacaoFinanceiraExecutada",
	}

	for _, eventType := range removed {
		eventType := eventType
		t.Run(eventType, func(t *testing.T) {
			t.Parallel()
			if err := ValidateEventType(eventType); err == nil {
				t.Fatalf("ValidateEventType(%q) retornou nil, want erro", eventType)
			}
		})
	}
}

func TestValidateAggregateTypeRejectsRemovedPaymentAggregate(t *testing.T) {
	t.Parallel()

	if err := ValidateAggregateType("Finance" + "iro"); err == nil {
		t.Fatal("ValidateAggregateType(\"Finance\" + \"iro\") retornou nil, want erro")
	}
}
