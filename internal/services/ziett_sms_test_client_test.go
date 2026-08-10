package services

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFormatAngolanMobileE164(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"national", "923456789", "+244923456789"},
		{"leading zero", "0923456789", "+244923456789"},
		{"plus country", "+244923456789", "+244923456789"},
		{"country", "244923456789", "+244923456789"},
		{"separators", "(+244) 923-456-789", "+244923456789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatAngolanMobileE164(tt.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestFormatAngolanMobileE164Invalid(t *testing.T) {
	for _, in := range []string{"", "123456789", "92345678", "9234567890", "92345A789"} {
		if _, err := FormatAngolanMobileE164(in); err == nil {
			t.Fatalf("expected error for %q", in)
		}
	}
}

func TestZiettClientSendSMSSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-API-KEY") != "zk_test_key" {
			t.Fatalf("missing api key")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["channel_type"] != "SMS" || body["target_e164"] != "+244923456789" {
			t.Fatalf("unexpected body %#v", body)
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message_id":"msg_123"}`))
	}))
	defer server.Close()

	client := NewZiettClientForTest("zk_test_key", server.URL, server.Client())
	got, err := client.SendSMS(context.Background(), ZiettSendMessageRequest{RemitterID: "550e8400-e29b-41d4-a716-446655440000", TargetE164: "923456789", Content: "teste"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MessageID != "msg_123" || got.TargetE164 != "+244923456789" {
		t.Fatalf("unexpected result %#v", got)
	}
}

func TestZiettClientSendSMSErrorPreservesFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"AUTH_INVALID_API_KEY","message":"invalid","status":401,"trace_id":"trace-1","service":"core"}`))
	}))
	defer server.Close()

	client := NewZiettClientForTest("zk_test_key", server.URL, server.Client())
	_, err := client.SendSMS(context.Background(), ZiettSendMessageRequest{RemitterID: "550e8400-e29b-41d4-a716-446655440000", TargetE164: "923456789", Content: "teste"})
	apiErr, ok := err.(*ZiettAPIError)
	if !ok {
		t.Fatalf("expected ZiettAPIError, got %T", err)
	}
	if apiErr.Code != "AUTH_INVALID_API_KEY" || apiErr.TraceID != "trace-1" || apiErr.Status != 401 {
		t.Fatalf("unexpected error %#v", apiErr)
	}
}
