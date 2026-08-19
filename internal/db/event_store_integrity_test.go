package db

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateEventTypeRejectsWhitelistBypassVariants(t *testing.T) {
	invalid := []string{
		"EventoInexistente",
		"estudantecriadocomvinculo",
		" EstudanteCriadoComVinculo",
		"EstudanteCriadoComVinculo ",
		"xEstudanteCriadoComVinculo",
		"EstudanteCriadoComVinculoDROP",
	}
	for _, eventType := range invalid {
		if err := ValidateEventType(eventType); err == nil {
			t.Fatalf("ValidateEventType(%q) = nil, want error", eventType)
		}
	}
}

func TestLedgerAppendOnlyTriggersAndIntegrityIfDatabaseAvailable(t *testing.T) {
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}

	prevDir, _ := os.Getwd()
	_ = os.Chdir("../..")
	t.Cleanup(func() { _ = os.Chdir(prevDir) })

	client, err := NewClient(DefaultConfig())
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer client.Close()
	if err := client.RunMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	ctx := context.Background()
	aggregateID := uuid.New()
	eventStore := NewEventStore(client)

	for version := 1; version <= 2; version++ {
		payload, _ := json.Marshal(map[string]any{"version": version})
		event := &Event{
			EventID:       uuid.New(),
			AggregateID:   aggregateID,
			AggregateType: "System",
			EventType:     "SchemaCreated",
			EventVersion:  version,
			Payload:       payload,
			Metadata:      json.RawMessage(`{"test":true}`),
			OccurredAt:    time.Now().UTC(),
		}
		if err := eventStore.appendDirect(ctx, event); err != nil {
			t.Fatalf("append event %d: %v", version, err)
		}
	}

	if ok, err := eventStore.VerifyLedgerIntegrity(ctx, aggregateID); err != nil || !ok {
		t.Fatalf("VerifyLedgerIntegrity before tamper = %v, %v; want true, nil", ok, err)
	}

	if _, err := client.DB().ExecContext(ctx, `UPDATE spuri_ledger SET payload = jsonb_set(payload, '{tampered}', 'true') WHERE aggregate_id = $1`, aggregateID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("UPDATE error = %v, want append-only rejection", err)
	}
	if _, err := client.DB().ExecContext(ctx, `DELETE FROM spuri_ledger WHERE aggregate_id = $1`, aggregateID); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("DELETE error = %v, want append-only rejection", err)
	}

	payload, _ := json.Marshal(map[string]any{"version": 3})
	event := &Event{EventID: uuid.New(), AggregateID: aggregateID, AggregateType: "System", EventType: "SchemaCreated", EventVersion: 3, Payload: payload, Metadata: json.RawMessage(`{}`), OccurredAt: time.Now().UTC()}
	if err := eventStore.appendDirect(ctx, event); err != nil {
		t.Fatalf("append after rejected tamper: %v", err)
	}
}
