package utils

import (
	"errors"
	"testing"
)

func TestSafeErrorMessage_NotaDuplicadaNaoViraPeriodoInvalido(t *testing.T) {
	err := errors.New("nota já registrada para periodo '1_trimestre', materia 'abc', tipo 'escolar', categoria 'nota_escola' no ano letivo '2025_2026'")
	got := SafeErrorMessage(err)

	want := "Nota já registrada para o mesmo ano/período/matéria/tipo/categoria"
	if got != want {
		t.Fatalf("mensagem inesperada.\nwant=%q\ngot=%q", want, got)
	}
}

func TestSafeErrorMessage_PeriodoInvalido(t *testing.T) {
	err := errors.New("periodo '9_trimestre' inválido para este contexto. Aceitos: [1_trimestre 2_trimestre 3_trimestre]")
	got := SafeErrorMessage(err)

	want := "Período inválido"
	if got != want {
		t.Fatalf("mensagem inesperada.\nwant=%q\ngot=%q", want, got)
	}
}

