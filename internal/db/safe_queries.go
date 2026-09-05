package db

import (
	"fmt"
	"regexp"
	"strings"
)

var validEventTypes = map[string]bool{
	"SchemaCreated":                                     true,
	"SolicitacaoServicoExtraCriada":                     true,
	"SolicitacaoServicoExtraAprovadaPendentePagamento":  true,
	"SolicitacaoServicoExtraVinculada":                  true,
	"SolicitacaoServicoExtraReprovada":                  true,
	"SolicitacaoServicoExtraCanceladaAntesDaVinculacao": true,
	"SolicitacaoServicoExtraCancelada":                  true,
	// ── Estudante ────────────────────────────────────────────────────────────
	"EstudanteCriadoComVinculo":      true,
	"EstudanteDocumentosCompletados": true,
	"DadosPessoaisAtualizados":       true,
	"DadosAcademicosAtualizados":     true,
	"SenhaAlterada":                  true,
	"CursoAlterado":                  true,
	// ── Acontecimentos de vínculo e trajetória escolar ───────────────────────
	"FundamentalRetomado":                       true,
	"FundamentalInterrompido":                   true,
	"EquivalenciaFundamentalReconhecida":        true,
	"MedioRetomado":                             true,
	"MedioInterrompido":                         true,
	"EquivalenciaMedioReconhecida":              true,
	"MatriculaSuperiorReativada":                true,
	"IngressoSuperiorPorEquivalenciaRegistrado": true,
	"SuperiorInterrompido":                      true,
	"SuperiorAbandonado":                        true,
	"EstudanteDesvinculadoDaAcademia":           true,
	"EstudanteReintegrado":                      true,
	"EstudanteDeletado":                         true, // Tarefa 73
	// ── Email Estudante ──────────────────────────────────────────────────────
	"EmailVerificadoEstudante": true,
	// ── Avaliação Final ───────────────────────────────────────────────────────
	"AvaliacaoFinalEscolar":  true,
	"AvaliacaoFinalSuperior": true,

	// ── Academia ─────────────────────────────────────────────────────────────
	"AcademiaCriada":                            true,
	"AcademiaAtivada":                           true,
	"AcademiaDesativada":                        true,
	"AcademiaDeletada":                          true,
	"AcademiaDadosAtualizados":                  true,
	"CursosAtualizados":                         true,
	"AcademiaSenhaAlterada":                     true,
	"CategoriaNotaAdicionada":                   true,
	"CategoriaNotaRemovida":                     true,
	"AnoLetivoAcademiaDefinido":                 true,
	"AnoLetivoAcademiaFinalizado":               true,
	"AcademiaDocumentosObrigatoriosAtualizados": true,
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
	"AdminDeletado":         true, // Tarefa 73
	// ── Notas e Faltas ────────────────────────────────────────────────────────
	"NotasRegistradas":  true,
	"FaltasRegistradas": true,
	"FaltaRegistrada":   true,
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
	"CursoCriado":            true,
	"CursoAtivado":           true,
	"CursoDesativado":        true,
	"CursoDadosAtualizados":  true,
	"CursoDeletado":          true,
	"ServicoExtraCriado":     true,
	"ServicoExtraAtualizado": true,
	"ServicoExtraDesativado": true,
	"ServicoExtraReativado":  true,
	// ── MateriaDisciplinar ────────────────────────────────────────────────────
	"MateriaCriada":           true,
	"MateriaAtivada":          true,
	"MateriaDesativada":       true,
	"MateriaDadosAtualizados": true,
	"MateriaPeriodoDefinido":  true,
	"MateriaDeletada":         true,
	// ── Solicitação de Matrícula ───────────────────────────────────────────────
	"SolicitacaoMatriculaCriada":                         true,
	"SolicitacaoMatriculaAprovada":                       true,
	"SolicitacaoMatriculaAprovadaPendentePagamento":      true,
	"SolicitacaoMatriculaReprovada":                      true,
	"SolicitacaoMatriculaCancelada":                      true,
	"SolicitacaoMatriculaVinculada":                      true,
	"SolicitacaoEdicaoDadoEstudanteCriada":               true,
	"SolicitacaoEdicaoDadoEstudanteAprovada":             true,
	"SolicitacaoEdicaoDadoEstudanteReprovada":            true,
	"NomeEstudanteAlteradoPorSolicitacao":                true,
	"BilheteIdentidadeEstudanteAlteradoPorSolicitacao":   true,
	"BilheteIdentidadeEncarregadoAlteradoPorSolicitacao": true,
	"DataNascimentoEstudanteAlteradaPorSolicitacao":      true,
	"TelefoneEncarregadoAlterado":                        true,
	"CredenciaisAppyPayConfiguradas":                     true,
	"CredenciaisAppyPayRemovidas":                        true,
	"SegredoWebhookAppyPayRotacionado":                   true,
	"CobrancaAppyPaySolicitada":                          true,
	"CobrancaAppyPayCriada":                              true,
	"CobrancaAppyPayFalhou":                              true,
	"CobrancaAppyPayConsultada":                          true,
	"CobrancaAppyPayCancelada":                           true,
	"CobrancaAppyPayConflitoPosCancelamento":             true,
	"QRCodeAppyPaySolicitado":                            true,
	"QRCodeAppyPayGerado":                                true,
	"QRCodeAppyPayFalhou":                                true,
	"WebhookAppyPayRecebido":                             true,
	"MensalidadeConfigurada":                             true,
	"MensalidadeConfiguracaoRemovida":                    true,
	"MesInicioCobrancaDefinido":                          true,
	"MesInicioCobrancaRemovido":                          true,
	"ObrigacaoMensalidadeAnulada":                        true,
	"ObrigacaoMensalidadeReativada":                      true,
	// MensalidadePaga is emitted by Phase 3. It is registered now so this
	// projection can consume a real payment event without any compatibility
	// path or inferred payment state.
	"MensalidadePaga": true,
	// MatriculaConfigurada e MensalidadesCobrancaConfirmada já eram emitidos
	// por internal/finance (matricula.go e mensalidade.go) e já eram
	// tratados por FinanceiroProjection.Handle, mas faltavam nesta
	// whitelist: todo SaveWithAudit/AppendTx para esses dois tipos de
	// evento era rejeitado com "tipo de evento inválido" antes mesmo de
	// tentar gravar no ledger.
	"MatriculaConfigurada":           true,
	"MatriculaConfiguracaoRemovida":  true,
	"MensalidadesCobrancaConfirmada": true,
	// NotaCorrigida e FaltaCorrigida são emitidos por
	// Estudante.CorrigirNota/CorrigirFalta (estudante_notas.go /
	// estudante_falta.go) desde a Tarefa 33/35, mas nunca constavam nesta
	// whitelist: todo SaveWithAudit para uma correção de nota ou falta era
	// rejeitado com "tipo de evento inválido" antes de tentar gravar no
	// ledger, fazendo a rota PATCH /academia/notas-aluno/{id} e
	// /academia/faltas-aluno/{id} retornar 500 sempre.
	"NotaCorrigida":  true,
	"FaltaCorrigida": true,
	// ── Solicitação de Alteração de NIF da Academia (Tarefas 81-82) ────────────
	// SolicitacaoAlteracaoNIFAcademiaCriada/Aprovada/Reprovada são emitidos por
	// aggregates.SolicitacaoAlteracaoNIFAcademia (solicitacao_alteracao_nif_academia.go)
	// desde a Tarefa 81, mas ficaram de fora desta whitelist quando a feature foi
	// implementada: todo AppendTx/SaveWithAudit para essas solicitações era
	// rejeitado com "tipo de evento inválido" antes de tentar gravar no ledger,
	// fazendo POST /academia/solicitacoes-nif e PUT
	// /admin/solicitacoes-nif-academia/{codigo}/aprovar|reprovar retornarem 500
	// sempre (mesma classe de bug já vista com NotaCorrigida/FaltaCorrigida acima).
	"SolicitacaoAlteracaoNIFAcademiaCriada":    true,
	"SolicitacaoAlteracaoNIFAcademiaAprovada":  true,
	"SolicitacaoAlteracaoNIFAcademiaReprovada": true,
}

// validAggregateTypes é o mapa canônico de aggregate types permitidos no ledger.
var validAggregateTypes = map[string]bool{
	"SolicitacaoServicoExtra":        true,
	"Estudante":                      true,
	"Academia":                       true,
	"Admin":                          true,
	"Curso":                          true,
	"ServicoExtra":                   true,
	"MateriaDisciplinar":             true,
	"Turma":                          true,
	"SolicitacaoMatricula":           true,
	"SolicitacaoEdicaoDadoEstudante": true,
	"System":                         true,
	"Financeiro":                     true,
	// SolicitacaoAlteracaoNIFAcademia (Tarefas 81-82): ver comentário
	// equivalente em validEventTypes acima sobre o mesmo bug de registro.
	"SolicitacaoAlteracaoNIFAcademia": true,
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
	const maxLimit = 100
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
