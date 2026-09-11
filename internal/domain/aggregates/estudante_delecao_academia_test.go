package aggregates

import (
	"testing"

	"github.com/google/uuid"
)

// Tarefa 98 — Estudante.DeletarPorAcademia: a academia atualmente vinculada
// ao estudante pode deletar a conta dele diretamente (sem exigir
// desvinculação prévia, ao contrário de Deletar/autodeleção), desde que essa
// mesma academia seja a que está registrada em e.CodigoAcademia. A checagem
// de que essa academia também foi quem ORIGINALMENTE cadastrou o estudante
// (via histórico do ledger) é feita no handler, não neste método — ver
// estudante_delecao_academia_integration_test.go no pacote handlers.

func TestDeletarPorAcademiaOKQuandoAtivo(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "ativo"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademia, academiaID); err != nil {
		t.Fatalf("DeletarPorAcademia retornou erro inesperado: %v", err)
	}
	if estudante.Status != "deletado" {
		t.Fatalf("Status = %q, want %q", estudante.Status, "deletado")
	}
}

func TestDeletarPorAcademiaOKQuandoPendenteDocumentos(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "pendente_documentos"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademia, academiaID); err != nil {
		t.Fatalf("DeletarPorAcademia retornou erro inesperado: %v", err)
	}
	if estudante.Status != "deletado" {
		t.Fatalf("Status = %q, want %q", estudante.Status, "deletado")
	}
}

func TestDeletarPorAcademiaFalhaQuandoDesvinculado(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "inativo" // desvinculado

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademia, academiaID); err == nil {
		t.Fatal("esperava erro ao deletar estudante desvinculado via DeletarPorAcademia, mas não houve erro")
	}
	if estudante.Status != "inativo" {
		t.Fatalf("Status mudou apesar do erro: %q", estudante.Status)
	}
}

func TestDeletarPorAcademiaFalhaQuandoJaDeletado(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "deletado"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademia, academiaID); err == nil {
		t.Fatal("esperava erro ao deletar estudante já deletado, mas não houve erro")
	}
}

// TestDeletarPorAcademiaFalhaAcademiaDiferente cobre exatamente o caso que
// motivou a Tarefa 98: uma academia diferente da atualmente vinculada nunca
// pode deletar a conta do estudante através deste método, mesmo que informe
// o motivo corretamente. A checagem "foi essa academia que cadastrou o
// estudante originalmente" é uma camada ADICIONAL no handler (ver pacote
// handlers) — este teste cobre apenas o invariante que o aggregate consegue
// verificar sozinho (vínculo ATUAL).
func TestDeletarPorAcademiaFalhaAcademiaDiferente(t *testing.T) {
	codigoAcademiaVinculada := "ACA_01"
	codigoAcademiaSolicitante := "ACA_02"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademiaVinculada
	estudante.Status = "ativo"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", codigoAcademiaSolicitante, academiaID); err == nil {
		t.Fatal("esperava erro quando a academia solicitante não é a atualmente vinculada, mas não houve erro")
	}
	if estudante.Status != "ativo" {
		t.Fatalf("Status mudou apesar do erro: %q", estudante.Status)
	}
}

func TestDeletarPorAcademiaFalhaMotivoVazio(t *testing.T) {
	codigoAcademia := "ACA_01"
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = &codigoAcademia
	estudante.Status = "ativo"

	if err := estudante.DeletarPorAcademia("   ", codigoAcademia, academiaID); err == nil {
		t.Fatal("esperava erro com motivo vazio/em branco, mas não houve erro")
	}
}

func TestDeletarPorAcademiaFalhaSemVinculoNenhum(t *testing.T) {
	academiaID := uuid.New()

	estudante := NewEstudante()
	estudante.CodigoEstudante = "EST1234"
	estudante.CodigoAcademia = nil
	estudante.Status = "inativo"

	if err := estudante.DeletarPorAcademia("solicitado pela academia", "ACA_01", academiaID); err == nil {
		t.Fatal("esperava erro quando o estudante nunca teve academia, mas não houve erro")
	}
}

// TestDeletarAutoServicoContinuaExigindoDesvinculacao é um teste de
// regressão de baseline: confirma que Deletar (autodeleção) continua com o
// mesmo comportamento de antes da Tarefa 98 — só o próprio estudante,
// somente quando já desvinculado (Status == "inativo").
func TestDeletarAutoServicoContinuaExigindoDesvinculacao(t *testing.T) {
	proprioID := uuid.New()

	vinculado := NewEstudante()
	vinculado.CodigoEstudante = "EST1234"
	vinculado.Status = "ativo"
	if err := vinculado.Deletar("motivo", proprioID); err == nil {
		t.Fatal("esperava erro ao autodeletar estudante ainda vinculado, mas não houve erro")
	}

	desvinculado := NewEstudante()
	desvinculado.CodigoEstudante = "EST1234"
	desvinculado.Status = "inativo"
	if err := desvinculado.Deletar("motivo", proprioID); err != nil {
		t.Fatalf("Deletar (autodeleção) retornou erro inesperado: %v", err)
	}
	if desvinculado.Status != "deletado" {
		t.Fatalf("Status = %q, want %q", desvinculado.Status, "deletado")
	}
}
