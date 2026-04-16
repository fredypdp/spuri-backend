package handlers

import (
	"encoding/json"
	"testing"

	"spuri/internal/jobs"

	"github.com/google/uuid"
)

func TestBuildRetryFailedPayload(t *testing.T) {
	t.Parallel()

	job := &jobs.Job{
		Payload: json.RawMessage(`[{"a":1},{"b":2},{"c":3}]`),
		Results: []jobs.ItemResult{
			{Index: 0, Sucesso: true, Payload: json.RawMessage(`{"a":1}`)},
			{Index: 1, Sucesso: false, Payload: json.RawMessage(`{"b":2}`), Erro: "erro 1"},
			{Index: 2, Sucesso: false, Payload: json.RawMessage(`{"c":3}`), Erro: "erro 2"},
		},
	}

	raw, err := buildRetryFailedPayload(job)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}

	var got []json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("payload retry inválido: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("esperava 2 itens falhados, obteve %d", len(got))
	}
	if string(got[0]) != `{"b":2}` || string(got[1]) != `{"c":3}` {
		t.Fatalf("itens de retry inesperados: %s", string(raw))
	}
}

func TestBuildRetryFailedPayloadFallbackByIndex(t *testing.T) {
	t.Parallel()

	job := &jobs.Job{
		Payload: json.RawMessage(`[{"a":1},{"b":2},{"c":3}]`),
		Results: []jobs.ItemResult{
			{Index: 0, Sucesso: true},
			{Index: 2, Sucesso: false, Erro: "erro no item 2"},
		},
	}

	raw, err := buildRetryFailedPayload(job)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}

	var got []json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("payload retry inválido: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("esperava 1 item falhado, obteve %d", len(got))
	}
	if string(got[0]) != `{"c":3}` {
		t.Fatalf("item de retry inesperado: %s", string(raw))
	}
}

func TestShouldListForAcademia(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		job    *jobs.Job
		expect bool
	}{
		{
			name:   "nil job",
			job:    nil,
			expect: false,
		},
		{
			name:   "pending",
			job:    &jobs.Job{Status: jobs.StatusPending},
			expect: true,
		},
		{
			name:   "processing",
			job:    &jobs.Job{Status: jobs.StatusProcessing},
			expect: true,
		},
		{
			name:   "failed",
			job:    &jobs.Job{Status: jobs.StatusFailed},
			expect: true,
		},
		{
			name:   "done",
			job:    &jobs.Job{Status: jobs.StatusDone},
			expect: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := shouldListForAcademia(tt.job)
			if got != tt.expect {
				t.Fatalf("esperava %v, obteve %v", tt.expect, got)
			}
		})
	}
}

func TestRetryJobNotFoundMessage(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("3811dcdf-5d11-41d3-bd0c-2abb87fd89ea")
	got := retryJobNotFoundMessage(id)
	want := "job 3811dcdf-5d11-41d3-bd0c-2abb87fd89ea não encontrado para retry-failed"
	if got != want {
		t.Fatalf("mensagem inesperada: %q", got)
	}
}
