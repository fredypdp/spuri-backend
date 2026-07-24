package aggregates

import (
	"strings"
	"testing"
)

const bcryptHashTesteAdmin = "$2a$10$123456789012345678901uWHzxGWxH1YBjSLnxc0xcr3qz1G4DKmW"

func TestAdminCriarExigeEmailETelefoneParaQualquerRole(t *testing.T) {
	roles := []string{"fpp", "adm", "gerente"}
	telefone := "923456789"

	for _, role := range roles {
		t.Run(role+"_sem_email", func(t *testing.T) {
			admin := NewAdmin()
			if err := admin.Criar("Admin", "", &telefone, bcryptHashTesteAdmin, role, nil); err == nil {
				t.Fatalf("Criar() deveria rejeitar admin %s sem email", role)
			}
		})

		t.Run(role+"_sem_telefone", func(t *testing.T) {
			admin := NewAdmin()
			if err := admin.Criar("Admin", "admin@example.com", nil, bcryptHashTesteAdmin, role, nil); err == nil {
				t.Fatalf("Criar() deveria rejeitar admin %s sem telefone", role)
			}
		})
	}
}

func TestAdminCriarPersisteTelefoneNoEventoEEstado(t *testing.T) {
	telefone := "923456789"
	admin := NewAdmin()

	if err := admin.Criar("Admin", "admin@example.com", &telefone, bcryptHashTesteAdmin, "gerente", nil); err != nil {
		t.Fatalf("Criar() erro inesperado: %v", err)
	}

	if admin.Telefone == nil || *admin.Telefone != telefone {
		t.Fatalf("telefone do estado = %v, esperado %s", admin.Telefone, telefone)
	}

	events := admin.GetUncommittedEvents()
	if len(events) != 1 {
		t.Fatalf("eventos gerados = %d, esperado 1", len(events))
	}
	event, ok := events[0].(*AdminCriadoEvent)
	if !ok {
		t.Fatalf("evento gerado tem tipo %T, esperado *AdminCriadoEvent", events[0])
	}
	if event.Telefone == nil || *event.Telefone != telefone {
		t.Fatalf("telefone do evento = %v, esperado %s", event.Telefone, telefone)
	}
}

func TestAdminCriarValidaFormatoTelefone(t *testing.T) {
	telefone := "123"
	admin := NewAdmin()

	err := admin.Criar("Admin", "admin@example.com", &telefone, bcryptHashTesteAdmin, "gerente", nil)
	if err == nil || !strings.Contains(err.Error(), "formato de telefone inválido") {
		t.Fatalf("Criar() erro = %v, esperado formato de telefone inválido", err)
	}
}
