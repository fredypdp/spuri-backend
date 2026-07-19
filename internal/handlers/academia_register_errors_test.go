package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBindRegisterAcademiaRequestInvalidJSONUsesStandardErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/dominis/academia/register", strings.NewReader(`{"nivel":`))
	c.Request.Header.Set("Content-Type", "application/json")

	_, _, ok := bindRegisterAcademiaRequest(c)
	if ok {
		t.Fatalf("expected invalid JSON binding to fail")
	}
	assertHandlerStandardErrorEnvelope(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestBindRegisterAcademiaRequestInvalidMultipartUsesStandardErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/dominis/academia/register", strings.NewReader("not-a-multipart-body"))
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=spuri")

	_, _, ok := bindRegisterAcademiaRequest(c)
	if ok {
		t.Fatalf("expected invalid multipart binding to fail")
	}
	assertHandlerStandardErrorEnvelope(t, w, http.StatusBadRequest, "VALIDATION_ERROR")
}

func assertHandlerStandardErrorEnvelope(t *testing.T, w *httptest.ResponseRecorder, expectedStatus int, expectedError string) {
	t.Helper()

	if w.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d. body=%s", expectedStatus, w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON error envelope, got unparseable body %q: %v", w.Body.String(), err)
	}
	if body["error"] != expectedError {
		t.Fatalf("expected error=%q, got %v. body=%v", expectedError, body["error"], body)
	}
	if message, ok := body["message"].(string); !ok || message == "" {
		t.Fatalf("expected non-empty message in standard error envelope. body=%v", body)
	}
	if requestID, ok := body["request_id"].(string); !ok || requestID == "" {
		t.Fatalf("expected non-empty request_id in standard error envelope. body=%v", body)
	}
}
