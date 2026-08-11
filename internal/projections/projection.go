package projections

import (
	"context"
	"database/sql"
	"log"
	"spuri/internal/db"
	"sync"

	"github.com/google/uuid"
)

type BaseProjection struct {
	client *db.Client
	ctx    context.Context
}

func NewBaseProjection(client *db.Client) *BaseProjection {
	return &BaseProjection{
		client: client,
		ctx:    context.Background(),
	}
}

func (bp *BaseProjection) GetLastProcessedEventIDByName(name string) (int64, error) {
	var lastID int64
	err := bp.client.DB().QueryRow(
		`SELECT last_processed_event_id FROM projection_checkpoints WHERE projection_name = $1`,
		name,
	).Scan(&lastID)
	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] Nenhum checkpoint encontrado para: %s", name)
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	log.Printf("[DEBUG] LastID: %d para projection: %s", lastID, name)
	return lastID, nil
}

func (bp *BaseProjection) UpdateCheckpointByName(name string, eventID int64) error {
	if eventID < 0 {
		eventID = 0
	}
	_, err := bp.client.DB().Exec(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at = CURRENT_TIMESTAMP,
			events_processed = projection_checkpoints.events_processed + 1
	`, name, eventID)
	if err != nil {
		log.Printf("[ERROR] Erro ao atualizar checkpoint para %s: %v", name, err)
	}
	return err
}

// ============================================================================
// Helpers internos
// ============================================================================

func nullOrUUID(u *uuid.UUID) interface{} {
	if u == nil {
		return nil
	}
	return u.String()
}

func nullOrString(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

// ============================================================================
// Cache de existência para uso exclusivo durante Rebuild()
// ============================================================================

// ExistenceCache acelera checagens repetidas de "esta chave já existe numa
// tabela de outra projeção" durante o REBUILD de uma projeção, evitando uma
// consulta ao banco por evento. NÃO deve ser usado no caminho de
// processamento ao vivo (Handle chamado a partir do processamento normal do
// Manager) — lá a verificação direta ao banco continua obrigatória, porque
// projeções diferentes processam de forma independente e a entidade
// pré-requisito pode legitimamente ainda não existir (condição de corrida
// documentada, por exemplo, como FIX BUG #2 em estudante_projection.go).
//
// Uso seguro durante rebuild: a ordem de reconstrução (defaultRebuildOrder,
// em manager.go) garante que projeções pré-requisito já estão totalmente
// reconstruídas antes de uma projeção dependente começar, e um rebuild de
// uma única projeção via RebuildProjection não altera as demais. Um snapshot
// inicial das chaves válidas é, portanto, seguro na esmagadora maioria dos
// casos. Para não depender inteiramente dessa suposição — por exemplo, se
// uma escrita ao vivo legítima criar uma nova academia exatamente durante a
// janela do rebuild — toda consulta que não encontra a chave no snapshot cai
// automaticamente para a mesma checagem direta ao banco usada hoje. O
// comportamento observável nunca muda; só o número de consultas no caso
// comum (chave já existente) é que cai.
//
// Cada instância deve viver apenas durante uma única chamada a Rebuild() —
// nunca deve ser guardada como campo de uma struct de projeção. Isso evita
// introduzir estado mutável compartilhado entre a goroutine do rebuild e a
// goroutine de processamento ao vivo, que podem rodar concorrentemente sobre
// a mesma instância de projeção.
type ExistenceCache struct {
	mu       sync.Mutex
	known    map[string]struct{}
	fallback func(key string) (bool, error)
}

// NewExistenceCache cria um cache pré-populado com as chaves de seed (ex.:
// todos os codigo_academia já presentes em projection_academias no início do
// rebuild) e uma função de fallback — deve ser exatamente a mesma checagem
// direta ao banco já usada hoje (ex.: o método academiaExists existente),
// para qualquer chave que não estiver no seed.
func NewExistenceCache(seed []string, fallback func(key string) (bool, error)) *ExistenceCache {
	known := make(map[string]struct{}, len(seed))
	for _, k := range seed {
		known[k] = struct{}{}
	}
	return &ExistenceCache{known: known, fallback: fallback}
}

// Exists devolve exatamente o mesmo resultado que a checagem direta ao banco
// devolveria, resolvendo a partir do snapshot em memória sempre que possível.
func (c *ExistenceCache) Exists(key string) (bool, error) {
	c.mu.Lock()
	_, ok := c.known[key]
	c.mu.Unlock()
	if ok {
		return true, nil
	}

	exists, err := c.fallback(key)
	if err != nil {
		return false, err
	}
	if exists {
		c.mu.Lock()
		c.known[key] = struct{}{}
		c.mu.Unlock()
	}
	return exists, nil
}
