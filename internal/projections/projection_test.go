// ============================================================================
// ARQUIVO: internal/projections/projection_test.go
// Testes básicos para projeções
// ============================================================================

package projections

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"spuri/internal/db"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock projection para testes
type MockProjection struct {
	name           string
	handleCalled   bool
	rebuildCalled  bool
	lastEventID    int64
	handleError    error
	rebuildError   error
}

func (m *MockProjection) Name() string {
	return m.name
}

func (m *MockProjection) Handle(event db.Event) error {
	m.handleCalled = true
	return m.handleError
}

func (m *MockProjection) Rebuild() error {
	m.rebuildCalled = true
	return m.rebuildError
}

func (m *MockProjection) GetLastProcessedEventID() (int64, error) {
	return m.lastEventID, nil
}

func (m *MockProjection) UpdateCheckpoint(eventID int64) error {
	m.lastEventID = eventID
	return nil
}

func TestProjectionInterface(t *testing.T) {
	t.Run("mock projection implements interface", func(t *testing.T) {
		var _ Projection = &MockProjection{}
	})

	t.Run("should call Handle", func(t *testing.T) {
		mock := &MockProjection{name: "test"}
		
		payload := map[string]string{"test": "data"}
		payloadBytes, _ := json.Marshal(payload)
		
		event := db.Event{
			ID:            1,
			EventID:       uuid.New(),
			AggregateID:   uuid.New(),
			AggregateType: "Test",
			EventType:     "TestEvent",
			EventVersion:  1,
			Payload:       payloadBytes,
			OccurredAt:    time.Now(),
		}

		err := mock.Handle(event)

		assert.NoError(t, err)
		assert.True(t, mock.handleCalled)
	})

	t.Run("should call Rebuild", func(t *testing.T) {
		mock := &MockProjection{name: "test"}

		err := mock.Rebuild()

		assert.NoError(t, err)
		assert.True(t, mock.rebuildCalled)
	})

	t.Run("should track checkpoint", func(t *testing.T) {
		mock := &MockProjection{name: "test"}

		err := mock.UpdateCheckpoint(100)
		require.NoError(t, err)

		lastID, err := mock.GetLastProcessedEventID()
		require.NoError(t, err)
		assert.Equal(t, int64(100), lastID)
	})
}

func TestEventToProjection(t *testing.T) {
	t.Run("should unmarshal payload correctly", func(t *testing.T) {
		payload := map[string]interface{}{
			"Nome":  "Test",
			"Codigo": "ABC123",
		}
		payloadBytes, _ := json.Marshal(payload)

		event := db.Event{
			ID:            1,
			EventID:       uuid.New(),
			AggregateID:   uuid.New(),
			AggregateType: "Estudante",
			EventType:     "EstudanteCriado",
			EventVersion:  1,
			Payload:       payloadBytes,
			OccurredAt:    time.Now(),
		}

		var unmarshaled map[string]interface{}
		err := json.Unmarshal(event.Payload, &unmarshaled)

		assert.NoError(t, err)
		assert.Equal(t, "Test", unmarshaled["Nome"])
		assert.Equal(t, "ABC123", unmarshaled["Codigo"])
	})

	t.Run("should handle nested payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"Estudante": map[string]interface{}{
				"Nome":   "Test",
				"Codigo": "ABC123",
			},
			"Academia": map[string]interface{}{
				"Nome": "Test School",
			},
		}
		payloadBytes, _ := json.Marshal(payload)

		event := db.Event{
			Payload: payloadBytes,
		}

		var unmarshaled map[string]interface{}
		err := json.Unmarshal(event.Payload, &unmarshaled)

		assert.NoError(t, err)
		
		estudante := unmarshaled["Estudante"].(map[string]interface{})
		assert.Equal(t, "Test", estudante["Nome"])
		
		academia := unmarshaled["Academia"].(map[string]interface{})
		assert.Equal(t, "Test School", academia["Nome"])
	})
}

func TestProjectionErrorHandling(t *testing.T) {
	t.Run("should propagate handle error", func(t *testing.T) {
		mock := &MockProjection{
			name:        "test",
			handleError: assert.AnError,
		}

		err := mock.Handle(db.Event{})

		assert.Error(t, err)
	})

	t.Run("should propagate rebuild error", func(t *testing.T) {
		mock := &MockProjection{
			name:         "test",
			rebuildError: assert.AnError,
		}

		err := mock.Rebuild()

		assert.Error(t, err)
	})
}

func TestProjectionConcurrency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrency test")
	}

	t.Run("should handle concurrent updates", func(t *testing.T) {
		mock := &MockProjection{name: "test"}
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		done := make(chan bool, 10)
		
		for i := 0; i < 10; i++ {
			go func(id int) {
				event := db.Event{
					ID:            int64(id),
					EventID:       uuid.New(),
					AggregateID:   uuid.New(),
					EventType:     "TestEvent",
					EventVersion:  1,
					Payload:       json.RawMessage("{}"),
					OccurredAt:    time.Now(),
				}
				
				mock.Handle(event)
				mock.UpdateCheckpoint(int64(id))
				
				select {
				case done <- true:
				case <-ctx.Done():
				}
			}(i)
		}

		// Aguardar conclusão
		for i := 0; i < 10; i++ {
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("Timeout waiting for concurrent updates")
			}
		}

		assert.True(t, mock.handleCalled)
		assert.True(t, mock.lastEventID >= 0)
	})
}