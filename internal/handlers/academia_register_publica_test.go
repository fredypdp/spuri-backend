package handlers

import (
	"os"
	"strings"
	"testing"
)

func extractFunctionSource(t *testing.T, source, funcSignature string) string {
	t.Helper()
	start := strings.Index(source, funcSignature)
	if start == -1 {
		t.Fatalf("função não encontrada no arquivo: %s", funcSignature)
	}
	rest := source[start:]
	end := strings.Index(rest[1:], "\nfunc ")
	if end == -1 {
		return rest
	}
	return rest[:end+1]
}

func TestRegisterAcademiaPublicaPassesContactFieldsToAggregate(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademiaPublica(")

	const expectedCall = "\t\treq.Endereco,\n\t\treq.Telefone,\n\t\treq.Email,\n\t\treq.Website,"
	if !strings.Contains(fn, expectedCall) {
		t.Fatal("RegisterAcademiaPublica deve repassar req.Telefone e req.Email para Academia.Criar, igual ao RegisterAcademia")
	}
}

func TestRegisterAcademiaPublicaDoesNotRequireAdminAuth(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademiaPublica(")

	forbidden := []string{"getAdminProjection(", "middleware.GetUserID(", "executorAdmin"}
	for _, term := range forbidden {
		if strings.Contains(fn, term) {
			t.Fatalf("RegisterAcademiaPublica não deve depender de autenticação/admin — encontrado %q", term)
		}
	}
}

func TestRegisterAcademiaPublicaForcesNilCriadoPor(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademiaPublica(")

	if !strings.Contains(fn, "\t\treq.AnosAcademicos,\n\t\tnil,") {
		t.Fatal("RegisterAcademiaPublica deve chamar academia.Criar com criadoPor=nil (cadastro público sem admin executor)")
	}
}

func TestRegisterAcademiaPublicaRequiresCustomPassword(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademiaPublica(")

	mustContain := []string{
		`c.PostForm("senha")`,
		`fmt.Errorf("senha é obrigatória")`,
		"utils.ValidateSenha(senha)",
	}
	for _, term := range mustContain {
		if !strings.Contains(fn, term) {
			t.Fatalf("RegisterAcademiaPublica deve exigir senha customizada — não encontrado: %q", term)
		}
	}
	if strings.Contains(fn, `services.GetDefaultPassword("academia", codigoAcademia)`) {
		t.Fatal("RegisterAcademiaPublica não deve usar fallback de senha padrão; senha deve ser obrigatória")
	}
}

func TestRegisterAcademiaDoesNotAcceptCustomPassword(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}
	fn := extractFunctionSource(t, string(source), "func RegisterAcademia(")

	if strings.Contains(fn, `c.PostForm("senha")`) {
		t.Fatal("RegisterAcademia (fluxo admin) não deve aceitar senha customizada — comportamento deve continuar exclusivo do cadastro público")
	}
}

func TestAcademiaRegistoPublicoRouteIsPublic(t *testing.T) {
	source, err := os.ReadFile("../../cmd/server/main.go")
	if err != nil {
		t.Fatalf("read main.go source: %v", err)
	}

	var routeLine string
	for _, line := range strings.Split(string(source), "\n") {
		if strings.Contains(line, "/academia/registo-publico") {
			routeLine = line
			break
		}
	}
	if routeLine == "" {
		t.Fatal("rota POST /academia/registo-publico não encontrada em cmd/server/main.go")
	}
	if strings.Contains(routeLine, "middleware.") {
		t.Fatalf("rota /academia/registo-publico deve ser pública, sem middleware de autenticação: %q", routeLine)
	}
	if !strings.Contains(routeLine, "router.POST(") {
		t.Fatalf("rota /academia/registo-publico deve ser registrada diretamente em router.POST, fora de grupos autenticados: %q", routeLine)
	}
}
