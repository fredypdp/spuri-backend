package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

func strPtr(s string) *string { return &s }

func TestEstudanteAtualizarDadosPessoaisResetaVerificacoesApenasQuandoValorMuda(t *testing.T) {
	estudante := NewEstudante()
	estudante.Status = "ativo"
	estudante.Email = strPtr("old@example.com")
	estudante.Telefone = strPtr("923456789")
	estudante.TelefoneEncarregado = strPtr("923456780")
	estudante.EmailVerificado = true
	estudante.TelefoneVerificado = true
	estudante.TelefoneEncarregadoVerificado = true

	mesmoEmail := "old@example.com"
	mesmoTelefone := "923456789"
	mesmoTelefoneEnc := "923456780"
	if err := estudante.AtualizarDadosPessoais(nil, &mesmoEmail, &mesmoTelefone, &mesmoTelefoneEnc, nil, nil, nil); err != nil {
		t.Fatalf("AtualizarDadosPessoais() com os mesmos contatos retornou erro: %v", err)
	}
	if !estudante.EmailVerificado || !estudante.TelefoneVerificado || !estudante.TelefoneEncarregadoVerificado {
		t.Fatalf("reenvio dos mesmos contatos não deve resetar flags: email=%v telefone=%v encarregado=%v", estudante.EmailVerificado, estudante.TelefoneVerificado, estudante.TelefoneEncarregadoVerificado)
	}

	novoEmail := "new@example.com"
	novoTelefone := "923456781"
	novoTelefoneEnc := "923456782"
	if err := estudante.AtualizarDadosPessoais(nil, &novoEmail, &novoTelefone, &novoTelefoneEnc, nil, nil, nil); err != nil {
		t.Fatalf("AtualizarDadosPessoais() com novos contatos retornou erro: %v", err)
	}
	if estudante.EmailVerificado || estudante.TelefoneVerificado || estudante.TelefoneEncarregadoVerificado {
		t.Fatalf("alteração de contatos deve resetar flags: email=%v telefone=%v encarregado=%v", estudante.EmailVerificado, estudante.TelefoneVerificado, estudante.TelefoneEncarregadoVerificado)
	}
}

func TestAdminAtualizarDadosResetaVerificacoesApenasQuandoValorMuda(t *testing.T) {
	admin := NewAdmin()
	admin.Status = "ativo"
	admin.Email = "old@example.com"
	admin.Telefone = strPtr("923456789")
	admin.EmailVerificado = true
	admin.TelefoneVerificado = true

	mesmoEmail := "old@example.com"
	mesmoTelefone := "923456789"
	if err := admin.AtualizarDados(nil, &mesmoEmail, &mesmoTelefone, uuid.New()); err != nil {
		t.Fatalf("AtualizarDados() com os mesmos contatos retornou erro: %v", err)
	}
	if !admin.EmailVerificado || !admin.TelefoneVerificado {
		t.Fatalf("reenvio dos mesmos contatos não deve resetar flags: email=%v telefone=%v", admin.EmailVerificado, admin.TelefoneVerificado)
	}

	novoEmail := "new@example.com"
	novoTelefone := "923456780"
	if err := admin.AtualizarDados(nil, &novoEmail, &novoTelefone, uuid.New()); err != nil {
		t.Fatalf("AtualizarDados() com novos contatos retornou erro: %v", err)
	}
	if admin.EmailVerificado || admin.TelefoneVerificado {
		t.Fatalf("alteração de contatos deve resetar flags: email=%v telefone=%v", admin.EmailVerificado, admin.TelefoneVerificado)
	}
}

func TestAdminValidatePermissionAplicaHierarquiaEstrita(t *testing.T) {
	fpp := NewAdmin()
	fpp.Status = "ativo"
	fpp.Role = "fpp"
	if err := fpp.ValidatePermission("gerente"); err != nil {
		t.Fatalf("fpp deve poder gerir gerente: %v", err)
	}

	gerente := NewAdmin()
	gerente.Status = "ativo"
	gerente.Role = "gerente"
	if err := gerente.ValidatePermission("fpp"); err == nil {
		t.Fatalf("gerente não deve gerir fpp")
	}
	if err := gerente.ValidatePermission("adm"); err == nil {
		t.Fatalf("gerente não deve gerir adm")
	}
	if err := gerente.ValidatePermission("gerente"); err == nil {
		t.Fatalf("gerente não deve gerir outro gerente pela hierarquia estrita")
	}
}

func TestEstudanteAtualizarDadosPessoaisRejeitaDataNascimento(t *testing.T) {
	estudante := NewEstudante()
	estudante.Status = "ativo"
	estudante.Telefone = strPtr("923456789")

	dataNascimento := estudante.DataNascimento.AddDate(-20, 0, 0)
	if err := estudante.AtualizarDadosPessoais(nil, nil, nil, nil, nil, nil, &dataNascimento); err == nil {
		t.Fatalf("esperava rejeição da edição de data_nascimento")
	}
}
