package db

import (
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"

	"spuri/internal/domain/aggregates"
)

// concurrentEstudanteAggregate is deliberately minimal: it exercises the
// repository's SERIALIZABLE SaveWithAudit path without adding projection or
// handler concerns to this database integration test.
type concurrentEstudanteAggregate struct{ aggregates.BaseAggregate }

func (a *concurrentEstudanteAggregate) GetType() string                    { return "Estudante" }
func (a *concurrentEstudanteAggregate) Apply(aggregates.DomainEvent) error { return nil }

func newConcurrentEstudanteAggregate(id uuid.UUID, key int) *concurrentEstudanteAggregate {
	a := &concurrentEstudanteAggregate{BaseAggregate: aggregates.BaseAggregate{ID: id}}
	a.RaiseEvent(&aggregates.BaseEvent{
		EventType:   "NotasRegistradas",
		AggregateID: id,
		Payload:     map[string]any{"concurrency_key": key},
	})
	return a
}

func TestSaveWithAuditRetriesSerializableConflictsIfDatabaseAvailable(t *testing.T) {
	if os.Getenv("SPURI_RUN_DB_INTEGRITY_TESTS") != "1" {
		t.Skip("set SPURI_RUN_DB_INTEGRITY_TESTS=1 with an isolated PostgreSQL database to run")
	}
	client, err := NewClient(DefaultConfig())
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer client.Close()
	if err := client.RunMigrations(); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	const workers = 8
	aggregateID := uuid.New()
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(key int) {
			defer wg.Done()
			<-start
			repo := NewAggregateRepository(client)
			err := repo.SaveWithAudit(newConcurrentEstudanteAggregate(aggregateID, key), AuditContext{UserID: uuid.NewString(), UserType: "academia", IP: "127.0.0.1"})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent SaveWithAudit failed: %v", err)
		}
	}

	var count int
	if err := client.DB().QueryRow(`SELECT COUNT(*) FROM spuri_ledger WHERE aggregate_id = $1`, aggregateID).Scan(&count); err != nil {
		t.Fatalf("count concurrent events: %v", err)
	}
	if count != workers {
		t.Fatalf("persisted events = %d, want %d", count, workers)
	}

}
