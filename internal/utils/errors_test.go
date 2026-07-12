package utils

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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

func TestRespondWithInternalErrorMapsTransientDatabaseErrorTo503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	c.Request = req

	RespondWithInternalError(c, errors.New("dial tcp 127.0.0.1:5432: connect: connection refused"))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body.Error != "SERVICE_UNAVAILABLE" {
		t.Fatalf("error = %q, want SERVICE_UNAVAILABLE", body.Error)
	}
}
