package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Event representa um evento no Event Store
type Event struct {
	ID            int64           `json:"-" db:"id"`
	EventID       uuid.UUID       `json:"event_id" db:"event_id"`
	AggregateID   uuid.UUID       `json:"aggregate_id" db:"aggregate_id"`
	AggregateType string          `json:"aggregate_type" db:"aggregate_type"`
	EventType     string          `json:"event_type" db:"event_type"`
	Payload       json.RawMessage `json:"payload" db:"payload"`
	Metadata      json.RawMessage `json:"metadata" db:"metadata"`
	OccurredAt    time.Time       `json:"occurred_at" db:"occurred_at"`
	Version       int             `json:"version" db:"version"`
}

// Tipos de eventos
const (
	EventTypeNotasRegistradas         = "NotasRegistradas"
	EventTypeFaltasRegistradas        = "FaltasRegistradas"
	EventTypeEstudanteInscrito        = "EstudanteInscrito"
	EventTypeInscricaoAprovada        = "InscricaoAprovada"
	EventTypeInscricaoReprovada       = "InscricaoReprovada"
	EventTypeAcademiaCriada           = "AcademiaCriada"
	EventTypeEstudanteCriado          = "EstudanteCriado"
	EventTypeVinculoAtualizado        = "VinculoAtualizado"
)

// Tipos de agregados
const (
	AggregateTypeEstudante = "Estudante"
	AggregateTypeAcademia  = "Academia"
)

// NotasRegistradasPayload representa o payload do evento NotasRegistradas
type NotasRegistradasPayload struct {
	IDAcademia  uuid.UUID `json:"id_academia"`
	AnoLectivo  string    `json:"ano_lectivo"`
	Periodo     string    `json:"periodo"`
	Materias    []Materia `json:"materias"`
}

// FaltasRegistradasPayload representa o payload do evento FaltasRegistradas
type FaltasRegistradasPayload struct {
	IDAcademia  uuid.UUID       `json:"id_academia"`
	AnoLectivo  string          `json:"ano_lectivo"`
	Periodo     string          `json:"periodo"`
	Materias    []MateriaFaltas `json:"materias"`
}

// EstudanteInscritoPayload representa o payload do evento EstudanteInscrito
type EstudanteInscritoPayload struct {
	AcademiaID   uuid.UUID `json:"academia_id"`
	Tipo         string    `json:"tipo"`
	AnoInscricao string    `json:"ano_inscricao"`
	Curso        *string   `json:"curso,omitempty"`
}

// InscricaoAprovadaPayload representa o payload de inscrição aprovada
type InscricaoAprovadaPayload struct {
	InscricaoID  uuid.UUID  `json:"inscricao_id"`
	AcademiaID   uuid.UUID  `json:"academia_id"`
	Tipo         string     `json:"tipo"`
	AnoInscricao string     `json:"ano_inscricao"`
	Curso        *string    `json:"curso,omitempty"`
}

// EventMetadata representa metadados do evento
type EventMetadata struct {
	ActorID   uuid.UUID `json:"actor_id"`
	ActorType string    `json:"actor_type"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
}

// NewEvent cria um novo evento
func NewEvent(aggregateID uuid.UUID, aggregateType, eventType string, payload interface{}, metadata EventMetadata) (*Event, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	return &Event{
		EventID:       uuid.New(),
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		EventType:     eventType,
		Payload:       payloadJSON,
		Metadata:      metadataJSON,
		OccurredAt:    time.Now(),
		Version:       1,
	}, nil
}