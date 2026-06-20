package projections

import (
	"testing"

	"spuri/internal/db"

	"github.com/google/uuid"
)

func TestCategoriaNotaProjectionIDUsesEventID(t *testing.T) {
	aggregateID := uuid.New()
	eventID := uuid.New()

	got := categoriaNotaProjectionID(db.Event{
		AggregateID: aggregateID,
		EventID:     eventID,
	})

	if got != eventID {
		t.Fatalf("categoriaNotaProjectionID() = %s, want event id %s", got, eventID)
	}
	if got == aggregateID {
		t.Fatalf("categoriaNotaProjectionID() reused aggregate id %s, causing primary-key collisions between categories", aggregateID)
	}
}
