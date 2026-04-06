package jobs

import (
	"sync"

	"github.com/google/uuid"
)

// EventType identifica o tipo de evento enviado por SSE.
type EventType string

const (
	EventJobEnqueued EventType = "job_enqueued"
	EventJobProgress EventType = "job_progress"
	EventJobDone     EventType = "job_done"
	EventJobFailed   EventType = "job_failed"
)

// Event representa uma notificação emitida para o dono do job.
type Event struct {
	Type       EventType `json:"type"`
	JobID      uuid.UUID `json:"job_id"`
	JobType    JobType   `json:"job_type"`
	Status     Status    `json:"status"`
	Progress   int       `json:"progress"`
	DoneItems  int       `json:"done_items"`
	FailItems  int       `json:"fail_items"`
	TotalItems int       `json:"total_items"`
	Error      string    `json:"error,omitempty"`
}

// Notifier mantém assinaturas em memória para push de eventos de jobs.
type Notifier struct {
	mu          sync.RWMutex
	subscribers map[uuid.UUID]map[chan Event]struct{}
}

func NewNotifier() *Notifier {
	return &Notifier{
		subscribers: make(map[uuid.UUID]map[chan Event]struct{}),
	}
}

// Subscribe cria um canal de eventos para o userID.
func (n *Notifier) Subscribe(userID uuid.UUID) chan Event {
	n.mu.Lock()
	defer n.mu.Unlock()

	ch := make(chan Event, 32)
	if _, ok := n.subscribers[userID]; !ok {
		n.subscribers[userID] = make(map[chan Event]struct{})
	}
	n.subscribers[userID][ch] = struct{}{}
	return ch
}

// Unsubscribe remove e fecha o canal de eventos.
func (n *Notifier) Unsubscribe(userID uuid.UUID, ch chan Event) {
	n.mu.Lock()
	defer n.mu.Unlock()

	userSubs, ok := n.subscribers[userID]
	if !ok {
		return
	}
	if _, exists := userSubs[ch]; !exists {
		return
	}
	delete(userSubs, ch)
	close(ch)
	if len(userSubs) == 0 {
		delete(n.subscribers, userID)
	}
}

// Publish envia o evento sem bloquear processamento principal.
func (n *Notifier) Publish(userID uuid.UUID, event Event) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	userSubs, ok := n.subscribers[userID]
	if !ok {
		return
	}
	for ch := range userSubs {
		select {
		case ch <- event:
		default:
			// Canal cheio: descartamos para não bloquear worker.
		}
	}
}
