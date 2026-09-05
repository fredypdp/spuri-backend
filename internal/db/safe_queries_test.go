package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ============================================================================
// Descoberta estática de event types / aggregate types via AST
// ============================================================================
//
// Contexto (Tarefa: correção do bug "tipo de evento inválido:
// AcademiaNIFAlteradoPorSolicitacao"): os dois testes que existiam aqui antes
// (TestValidateEventTypeAcceptsEventsDiscoveredInCode e
// TestValidateAggregateTypeAcceptsAggregatesDiscoveredInCode) eram, eles
// próprios, listas mantidas manualmente — exatamente a mesma classe de fonte
// de verdade duplicada que causou o bug: um event type novo em
// internal/domain/aggregates (ou internal/finance) só chegava à whitelist em
// validEventTypes/validAggregateTypes (safe_queries.go) se alguém se
// lembrasse de atualizar os DOIS lugares. Isso já falhou, no mínimo, para
// NotaCorrigida/FaltaCorrigida, MatriculaConfigurada/MensalidadesCobrancaConfirmada,
// o trio SolicitacaoAlteracaoNIFAcademia* e, mais recentemente,
// AcademiaNIFAlteradoPorSolicitacao, SolicitacaoMatriculaValorPendenteAtualizado
// e o aggregate/eventos de Sumario — todos descobertos e corrigidos juntos
// nesta mesma tarefa.
//
// Em vez de outra lista manual, os testes abaixo fazem a própria varredura:
// parseiam o código-fonte de internal/domain/aggregates e internal/finance em
// tempo de teste e extraem todo event type / aggregate type que o código
// realmente usa, cobrindo os padrões observados no repositório:
//
//  1. BaseEvent{EventType: "Literal", ...} — a maioria dos eventos.
//  2. const Nome = "Nome" (auto-referente) em internal/domain/aggregates —
//     convenção usada por financeiro.go para os eventos emitidos via
//     Financeiro.Registrar(aggregates.Nome, ...).
//  3. func (x *Tipo) GetType() string { return "Literal" } — os aggregate
//     types.
//  4. s.record(ctx, id, "Literal", ...) / s.recordMensalidade(ctx, key,
//     "Literal", ...) e `eventType := "Literal"` — o padrão usado em
//     internal/finance/appypay.go e mensalidade.go para eventos financeiros
//     que não passam por uma const auto-referente.
//
// Qualquer event type ou aggregate type descoberto que NÃO estiver em
// validEventTypes/validAggregateTypes faz este teste falhar, com o nome
// exato do tipo ausente — o mesmo erro que hoje só aparece como um 500 em
// produção ("tipo de evento inválido: X") passa a quebrar o `go test` antes
// do merge.
//
// Isto não substitui o cuidado de registrar o novo event/aggregate type ao
// criá-lo — é a rede de segurança para quando esse cuidado falhar de novo.

// aggregatesSourceDir e financeSourceDir localizam os pacotes-fonte a partir
// do diretório deste pacote (internal/db), que é o diretório de trabalho do
// `go test`.
const (
	aggregatesSourceDir = "../domain/aggregates"
	financeSourceDir    = "../finance"
)

// discoverEventAndAggregateTypes varre os pacotes-fonte em dirs e retorna
// todo event type e aggregate type que o código realmente usa, pelos quatro
// padrões descritos acima. O padrão 4 (record/recordMensalidade, eventType
// local) é específico de internal/finance, mas é seguro aplicar aos dois
// diretórios: internal/domain/aggregates não contém nenhuma chamada
// .record(/.recordMensalidade( nem variável local "eventType".
func discoverEventAndAggregateTypes(t *testing.T, dirs ...string) (eventTypes, aggregateTypes map[string]bool) {
	t.Helper()
	eventTypes = map[string]bool{}
	aggregateTypes = map[string]bool{}
	fset := token.NewFileSet()

	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatalf("glob %s: %v", dir, err)
		}
		if len(files) == 0 {
			t.Fatalf("nenhum arquivo .go encontrado em %s — caminho relativo mudou?", dir)
		}

		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", f, err)
			}

			for _, decl := range file.Decls {
				// Padrão 2: const Nome = "Nome" (auto-referente).
				if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.CONST {
					for _, spec := range gd.Specs {
						vs, ok := spec.(*ast.ValueSpec)
						if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
							continue
						}
						lit, ok := vs.Values[0].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						if val, err := strconv.Unquote(lit.Value); err == nil && val == vs.Names[0].Name {
							eventTypes[val] = true
						}
					}
				}
				// Padrão 3: func (x *T) GetType() string { return "Literal" }.
				if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv != nil && fd.Name.Name == "GetType" && fd.Body != nil {
					for _, stmt := range fd.Body.List {
						ret, ok := stmt.(*ast.ReturnStmt)
						if !ok || len(ret.Results) != 1 {
							continue
						}
						lit, ok := ret.Results[0].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						if val, err := strconv.Unquote(lit.Value); err == nil {
							aggregateTypes[val] = true
						}
					}
				}
			}

			ast.Inspect(file, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.KeyValueExpr:
					// Padrão 1: BaseEvent{EventType: "Literal", ...}.
					key, ok := x.Key.(*ast.Ident)
					if !ok || key.Name != "EventType" {
						return true
					}
					lit, ok := x.Value.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return true
					}
					if val, err := strconv.Unquote(lit.Value); err == nil {
						eventTypes[val] = true
					}
				case *ast.AssignStmt:
					// Padrão 4a: eventType := "Literal" (appypay.go).
					for i, lhs := range x.Lhs {
						id, ok := lhs.(*ast.Ident)
						if !ok || id.Name != "eventType" || i >= len(x.Rhs) {
							continue
						}
						lit, ok := x.Rhs[i].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						if val, err := strconv.Unquote(lit.Value); err == nil {
							eventTypes[val] = true
						}
					}
				case *ast.CallExpr:
					// Padrão 4b: s.record(ctx, id, "Literal", ...) /
					// s.recordMensalidade(ctx, key, "Literal", ...).
					sel, ok := x.Fun.(*ast.SelectorExpr)
					if !ok || (sel.Sel.Name != "record" && sel.Sel.Name != "recordMensalidade") || len(x.Args) < 3 {
						return true
					}
					lit, ok := x.Args[2].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						return true
					}
					if val, err := strconv.Unquote(lit.Value); err == nil {
						eventTypes[val] = true
					}
				}
				return true
			})
		}
	}
	return eventTypes, aggregateTypes
}

// TestValidateEventTypeAcceptsEventsDiscoveredInCode varre
// internal/domain/aggregates e internal/finance e falha se algum event type
// realmente emitido pelo código não estiver em validEventTypes — o cenário
// exato do bug corrigido nesta tarefa (AcademiaNIFAlteradoPorSolicitacao e
// outros; ver comentário no topo do arquivo).
func TestValidateEventTypeAcceptsEventsDiscoveredInCode(t *testing.T) {
	eventTypes, _ := discoverEventAndAggregateTypes(t, aggregatesSourceDir, financeSourceDir)
	// SchemaCreated é gerado pela infraestrutura de bootstrap do ledger, não
	// por um aggregate — não aparece em nenhum dos padrões varridos acima,
	// então é adicionado manualmente aqui.
	eventTypes["SchemaCreated"] = true

	if len(eventTypes) < 50 {
		t.Fatalf("apenas %d event types descobertos — a varredura AST provavelmente está quebrada (esperado 90+)", len(eventTypes))
	}

	for eventType := range eventTypes {
		eventType := eventType
		t.Run(eventType, func(t *testing.T) {
			if err := ValidateEventType(eventType); err != nil {
				t.Errorf("%q é emitido pelo código mas rejeitado por ValidateEventType — adicione-o a validEventTypes em safe_queries.go: %v", eventType, err)
			}
		})
	}
}

// TestValidateAggregateTypeAcceptsAggregatesDiscoveredInCode é o equivalente
// acima para aggregate types (GetType()).
func TestValidateAggregateTypeAcceptsAggregatesDiscoveredInCode(t *testing.T) {
	_, aggregateTypes := discoverEventAndAggregateTypes(t, aggregatesSourceDir, financeSourceDir)
	// System é usado pela infraestrutura de bootstrap do ledger (aggregate
	// virtual do evento SchemaCreated), não por um GetType() real.
	aggregateTypes["System"] = true

	if len(aggregateTypes) < 8 {
		t.Fatalf("apenas %d aggregate types descobertos — a varredura AST provavelmente está quebrada (esperado 11+)", len(aggregateTypes))
	}

	for aggregateType := range aggregateTypes {
		aggregateType := aggregateType
		t.Run(aggregateType, func(t *testing.T) {
			if err := ValidateAggregateType(aggregateType); err != nil {
				t.Errorf("%q é usado pelo código mas rejeitado por ValidateAggregateType — adicione-o a validAggregateTypes em safe_queries.go: %v", aggregateType, err)
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

func TestValidateAggregateTypeAcceptsFinanceiro(t *testing.T) {
	t.Parallel()

	if err := ValidateAggregateType("Finance" + "iro"); err != nil {
		t.Fatalf("ValidateAggregateType(\"Finance\" + \"iro\") retornou %v, want nil", err)
	}
}
