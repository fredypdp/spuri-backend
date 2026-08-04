package handlers

import (
	"os"
	"strings"
	"testing"
)

func TestRegisterAcademiaPassesContactFieldsToAggregate(t *testing.T) {
	source, err := os.ReadFile("academia_handlers.go")
	if err != nil {
		t.Fatalf("read academia handler source: %v", err)
	}

	const expectedCall = "\t\treq.Endereco,\n\t\treq.Telefone,\n\t\treq.Email,\n\t\treq.Website,"
	if !strings.Contains(string(source), expectedCall) {
		t.Fatal("RegisterAcademia must pass req.Telefone and req.Email to Academia.Criar")
	}
}
