// ============================================================================
// ARQUIVO: internal/domain/aggregates/aggregate_test.go
// Testes para agregados base
// ============================================================================

package aggregates

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestBaseAggregate(t *testing.T) {
	t.Run("should initialize with zero version", func(t *testing.T) {
		agg := &BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		}

		assert.Equal(t, 0, agg.GetVersion())
		assert.Equal(t, 0, len(agg.GetUncommittedEvents()))
	})

	t.Run("should raise event and increment version", func(t *testing.T) {
		agg := &BaseAggregate{
			ID:                uuid.New(),
			Version:           0,
			UncommittedEvents: []DomainEvent{},
		}

		event := &BaseEvent{
			EventType:   "TestEvent",
			AggregateID: agg.ID,
		}

		agg.RaiseEvent(event)

		assert.Equal(t, 1, agg.GetVersion())
		assert.Equal(t, 1, len(agg.GetUncommittedEvents()))
	})

	t.Run("should clear uncommitted events", func(t *testing.T) {
		agg := &BaseAggregate{
			ID:                uuid.New(),
			Version:           1,
			UncommittedEvents: []DomainEvent{&BaseEvent{}},
		}

		agg.ClearUncommittedEvents()

		assert.Equal(t, 0, len(agg.GetUncommittedEvents()))
		assert.Equal(t, 1, agg.GetVersion()) // Version não muda
	})
}

func TestDefaultAggregateFactory(t *testing.T) {
	factory := &DefaultAggregateFactory{}

	tests := []struct {
		name          string
		aggregateType string
		expectError   bool
	}{
		{"should create Estudante", "Estudante", false},
		{"should create Academia", "Academia", false},
		{"should create Admin", "Admin", false},
		{"should create Curso", "Curso", false},
		{"should create MateriaDisciplinar", "MateriaDisciplinar", false},
		{"should fail for unknown type", "Unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agg, err := factory.Create(tt.aggregateType)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, agg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, agg)
				assert.Equal(t, tt.aggregateType, agg.GetType())
			}
		})
	}
}