package handlers

import (
	"strings"
	"testing"
)

// TestRegisterAcademiaPublicaNotificaAdminsRegistroAcademia garante que:
//   - o autocadastro público (RegisterAcademiaPublica, POST /academia/cadastro)
//     chama notificarAdminsSobreNovaAcademiaPendente logo após persistir a
//     academia com sucesso;
//   - o cadastro feito por um admin fpp (RegisterAcademia,
//     POST /dominis/academia/cadastro) NÃO chama essa função — o aviso é só
//     para o autocadastro público; quem cadastrou pela rota admin já é, ele
//     mesmo, um admin com permissão de ativação e já sabe da academia que
//     acabou de criar.
func TestRegisterAcademiaPublicaNotificaAdminsRegistroAcademia(t *testing.T) {
	source := readHandlerSource(t, "internal/handlers/academia_handlers.go")

	mustContain(t, source, "func notificarAdminsSobreNovaAcademiaPendente(")
	mustContain(t, source, "GetAdminsParaNotificarNovaAcademia()")
	mustContain(t, source, "SendAcademiaCadastradaEmail(")

	publicaBody := extractFuncBody(t, source, "func RegisterAcademiaPublica(")
	if !strings.Contains(publicaBody, "notificarAdminsSobreNovaAcademiaPendente(") {
		t.Fatal("RegisterAcademiaPublica deveria chamar notificarAdminsSobreNovaAcademiaPendente após o cadastro")
	}

	adminBody := extractFuncBody(t, source, "func RegisterAcademia(")
	if strings.Contains(adminBody, "notificarAdminsSobreNovaAcademiaPendente(") {
		t.Fatal("RegisterAcademia (cadastro pelo admin fpp) não deveria chamar notificarAdminsSobreNovaAcademiaPendente — o aviso é só para o autocadastro público")
	}
}

// extractFuncBody devolve o texto da função (do "func Nome(" até a próxima
// declaração de função de nível superior), para checar chamadas feitas
// especificamente dentro dela e não em qualquer lugar do arquivo.
func extractFuncBody(t *testing.T, source, funcSignaturePrefix string) string {
	t.Helper()
	start := strings.Index(source, funcSignaturePrefix)
	if start == -1 {
		t.Fatalf("declaração %q não encontrada", funcSignaturePrefix)
	}
	rest := source[start+len(funcSignaturePrefix):]
	end := strings.Index(rest, "\nfunc ")
	if end == -1 {
		t.Fatalf("não foi possível delimitar o fim da função %q", funcSignaturePrefix)
	}
	return rest[:end]
}
