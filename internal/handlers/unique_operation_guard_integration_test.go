package handlers

import (
	"os"
	"strings"
	"testing"
)

func TestSolicitacaoEdicaoHandlersReserveAndReleasePendingGuard(t *testing.T) {
	t.Parallel()

	source := readHandlerSource(t, "internal/handlers/solicitacao_edicao_dado_estudante_handlers.go")
	mustContain(t, source, `Reserve(`)
	mustContain(t, source, `"solicitacao_edicao_dado_estudante:pendente"`)
	mustContain(t, source, `db.CanonicalGuardKey(est.CodigoEstudante, campo)`)
	mustContain(t, source, `errors.Is(err, db.ErrUniqueOperationInProgress)`)
	mustContain(t, source, `guard.Release()`)
	mustContain(t, source, `guard.Consume(agg.GetID())`)
	mustContain(t, source, `ReleaseKey("solicitacao_edicao_dado_estudante:pendente", db.CanonicalGuardKey(sol.CodigoEstudante, sol.Campo))`)
}

func TestStatusAcademicoHandlersReserveAndReleasePendingGuard(t *testing.T) {
	t.Parallel()

	source := readHandlerSource(t, "internal/handlers/academia_status_escolar_handlers.go")
	mustContain(t, source, `Reserve(`)
	mustContain(t, source, `"solicitacao_status_academico:pendente"`)
	mustContain(t, source, `db.CanonicalGuardKey(estudanteDTO.CodigoEstudante, academiaCodigo, tipo)`)
	mustContain(t, source, `errors.Is(err, db.ErrUniqueOperationInProgress)`)
	mustContain(t, source, `guard.Release()`)
	mustContain(t, source, `guard.Consume(uuid.Nil)`)
	mustContain(t, source, `ReleaseKey("solicitacao_status_academico:pendente", db.CanonicalGuardKey(codigoEst, codigoAcademia, tipo))`)
}

func TestStudentCreationHandlersReserveBIUniquenessGuard(t *testing.T) {
	t.Parallel()

	direct := readHandlerSource(t, "internal/handlers/estudante_handlers.go")
	mustContain(t, direct, `"estudante:bilhete_identidade"`)
	mustContain(t, direct, `db.CanonicalGuardKey(*bilhetePtr)`)
	mustContain(t, direct, `errors.Is(err, db.ErrUniqueOperationInProgress)`)
	mustContain(t, direct, `biGuard.Release()`)
	mustContain(t, direct, `biGuard.Consume(estudante.GetID())`)

	matricula := readHandlerSource(t, "internal/handlers/solicitacao_matricula_handlers.go")
	mustContain(t, matricula, `"estudante:bilhete_identidade"`)
	mustContain(t, matricula, `db.CanonicalGuardKey(*agg.BilheteIdentidade)`)
	mustContain(t, matricula, `errors.Is(err, db.ErrUniqueOperationInProgress)`)
	mustContain(t, matricula, `biGuard.Release()`)
	mustContain(t, matricula, `biGuard.Consume(est.GetID())`)
}

func readHandlerSource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile("../../" + path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	return string(data)
}

func mustContain(t *testing.T, source, want string) {
	t.Helper()
	if !strings.Contains(source, want) {
		t.Fatalf("source does not contain %q", want)
	}
}
