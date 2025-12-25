// ============================================================================
// ARQUIVO: internal/projections/projection.go
// Interface base para todas as projeções
// ============================================================================

package projections

import "spuri/internal/genesisdb"

// Projection interface para todas as projeções
type Projection interface {
	// Name retorna o nome da projeção
	Name() string
	
	// Handle processa um evento
	Handle(event genesisdb.Event) error
	
	// Rebuild reconstrói a projeção do zero
	Rebuild() error
	
	// GetLastProcessedEventID retorna o último evento processado
	GetLastProcessedEventID() (int64, error)
	
	// UpdateCheckpoint atualiza o checkpoint
	UpdateCheckpoint(eventID int64) error
}