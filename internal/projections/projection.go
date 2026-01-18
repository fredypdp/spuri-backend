// ============================================================================
// ARQUIVO 1: internal/projections/projection.go
// ============================================================================

package projections

import "spuri/internal/db"

// Projection interface para todas as projeções
type Projection interface {
	// Name retorna o nome da projeção
	Name() string
	
	// Handle processa um evento
	Handle(event db.Event) error
	
	// Rebuild reconstrói a projeção do zero
	Rebuild() error
	
	// GetLastProcessedEventID retorna o último evento processado
	GetLastProcessedEventID() (int64, error)
	
	// UpdateCheckpoint atualiza o checkpoint
	UpdateCheckpoint(eventID int64) error
}