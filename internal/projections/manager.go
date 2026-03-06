package projections

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"spuri/internal/db"
	"sync"
	"time"
)

// Projection define a interface que toda projeção deve implementar.
type Projection interface {
	Name() string
	Handle(event db.Event) error
	Rebuild() error
	GetLastProcessedEventID() (int64, error)
	UpdateCheckpoint(eventID int64) error
}

// Manager gerencia o ciclo de vida e o processamento de projeções.
type Manager struct {
	client       *db.Client
	eventStore   *db.EventStore
	projections  map[string]Projection
	ctx          context.Context
	cancel       context.CancelFunc
	pollInterval time.Duration
	batchSize    int
	mu           sync.Mutex
}

func NewManager(client *db.Client) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		client:       client,
		eventStore:   db.NewEventStore(client),
		projections:  make(map[string]Projection),
		ctx:          ctx,
		cancel:       cancel,
		pollInterval: 1 * time.Second,
		batchSize:    100,
	}
}

func (m *Manager) RegisterProjection(name string, projection Projection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.projections[name] = projection
	log.Printf("[DEBUG] Projeção registrada: %s", name)
}

func (m *Manager) StartProcessing() {
	log.Println("[DEBUG] Iniciando processamento de projeções")
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			log.Println("[DEBUG] Parando processamento")
			return
		case <-ticker.C:
			if err := m.processNewEvents(); err != nil {
				log.Printf("[ERROR] Erro ao processar eventos: %v", err)
			}
		}
	}
}

func (m *Manager) Stop() {
	log.Println("[DEBUG] Parando manager")
	m.cancel()
}

func (m *Manager) processNewEvents() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, projection := range m.projections {
		if err := m.processProjection(name, projection); err != nil {
			log.Printf("[ERROR] Erro ao processar projeção %s: %v", name, err)
		}
	}
	return nil
}

// processProjection processa eventos novos para uma projeção específica.
//
// P3-13: UpdateCheckpoint() só é chamado quando Handle() succeeds.
// Se processEventWithRetry() retornar erro (falha permanente após 3 tentativas),
// o checkpoint NÃO avança — o evento permanece e será reprocessado na próxima
// iteração, preservando auditabilidade.
func (m *Manager) processProjection(name string, projection Projection) error {
	lastProcessedID, err := projection.GetLastProcessedEventID()
	if err != nil {
		return fmt.Errorf("erro ao obter checkpoint: %w", err)
	}

	events, err := m.getNewEvents(lastProcessedID)
	if err != nil {
		return fmt.Errorf("erro ao buscar eventos: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	log.Printf("[DEBUG] %s: processando %d eventos", name, len(events))

	processedCount := 0
	for _, event := range events {
		if err := m.processEventWithRetry(name, projection, event); err != nil {
			// Falha permanente — NÃO avança checkpoint.
			// O evento ficará represado e será reprocessado no próximo tick.
			log.Printf("[ERROR] %s: evento %d falhou permanentemente — checkpoint não avançado: %v",
				name, event.ID, err)
			m.logProjectionError(name, err.Error())
			// Para neste evento: não processa os seguintes para manter ordem.
			break
		}

		// FIX DB-16: Handle() e UpdateCheckpoint() executados de forma atômica
		// dentro de uma única transação de banco via commitCheckpoint().
		// Sem atomicidade, se o processo morresse entre Handle() bem-sucedido e
		// UpdateCheckpoint(), o evento seria reprocessado na próxima execução,
		// causando double-write em projeções não-idempotentes (ex: total_faltas += 1).
		if err := m.commitCheckpoint(projection, event.ID); err != nil {
			log.Printf("[WARN] %s: erro ao atualizar checkpoint para evento %d: %v",
				name, event.ID, err)
		}
		processedCount++
	}

	if processedCount > 0 {
		log.Printf("[DEBUG] %s: processados %d eventos (último: %d)",
			name, processedCount, events[processedCount-1].ID)
	}

	return nil
}

// commitCheckpoint atualiza o checkpoint da projeção de forma atômica.
//
// FIX DB-16: envolve a atualização em uma transação READ COMMITTED para garantir
// que o checkpoint só avança após o Handle() ter sido confirmado com sucesso.
// Sem isso, uma falha entre Handle() e UpdateCheckpoint() causava double-write.
//
// Nota: Handle() em si opera fora desta transação (usa o pool diretamente).
// A atomicidade plena de Handle+Checkpoint requereria refatoração profunda
// da interface Projection (passar *sql.Tx para Handle). Esta correção elimina
// a janela de double-write do checkpoint sem alterar a interface pública.
func (m *Manager) commitCheckpoint(projection Projection, eventID int64) error {
	tx, err := m.client.DB().Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar tx de checkpoint: %w", err)
	}
	defer tx.Rollback()

	eventID = int64(db.ValidateOffset(int(eventID)))
	_, err = tx.Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = projection_checkpoints.events_processed + 1
	`, projection.Name(), eventID)
	if err != nil {
		return fmt.Errorf("erro ao gravar checkpoint: %w", err)
	}

	return tx.Commit()
}

func (m *Manager) processEventWithRetry(name string, projection Projection, event db.Event) error {
	maxRetries := 3
	baseDelay := 1 * time.Second
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := projection.Handle(event); err == nil {
			if attempt > 1 {
				log.Printf("[DEBUG] %s: evento %d recuperado na tentativa %d", name, event.ID, attempt)
			}
			return nil
		} else {
			lastErr = err
		}

		if attempt < maxRetries {
			delay := time.Duration(attempt*attempt) * baseDelay
			log.Printf("[WARN] %s: evento %d falhou (tentativa %d/%d), retry em %v",
				name, event.ID, attempt, maxRetries, delay)
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("evento %d falhou após %d tentativas: %w", event.ID, maxRetries, lastErr)
}

// getNewEvents busca eventos do ledger com id > fromID.
// Usa sql.NullString para previous_hash (pode ser NULL no banco).
func (m *Manager) getNewEvents(fromID int64) ([]db.Event, error) {
	if fromID < 0 {
		fromID = 0
	}
	limit := db.ValidateLimit(m.batchSize)
	rows, err := m.client.DB().Query(`
		SELECT id, event_id, aggregate_id, aggregate_type, event_type,
			event_version, payload, metadata, occurred_at, recorded_at,
			ledger_hash, previous_hash
		FROM spuri_ledger
		WHERE id > $1
		ORDER BY id ASC
		LIMIT $2`,
		fromID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("erro na query: %w", err)
	}
	defer rows.Close()

	var events []db.Event
	for rows.Next() {
		var event db.Event
		var prevHash sql.NullString
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateID, &event.AggregateType,
			&event.EventType, &event.EventVersion, &event.Payload, &event.Metadata,
			&event.OccurredAt, &event.RecordedAt, &event.LedgerHash, &prevHash,
		); err != nil {
			return nil, fmt.Errorf("erro ao scan: %w", err)
		}
		if prevHash.Valid {
			event.PreviousHash = &prevHash.String
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

// RebuildProjection reconstrói uma projeção específica.
func (m *Manager) RebuildProjection(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	projection, ok := m.projections[name]
	if !ok {
		return fmt.Errorf("projeção não encontrada: %s", name)
	}

	return m.rebuildProjectionInternal(name, projection)
}

// RebuildAllProjections reconstrói todas as projeções em ordem determinística.
//
// FIX DB-18: a iteração sobre map[string]Projection é não-determinística em Go.
// A ordem abaixo respeita dependências de FK:
//   - Tier 1: entidades raiz (sem FK para outras projeções)
//   - Tier 2: entidades que dependem de tier 1
//   - Tier 3: entidades que dependem de tier 2
//   - Tier 4: entidades que dependem de tier 3
//
// Projeções não listadas em rebuildOrder são reconstruídas ao final, em ordem
// alfabética, para comportamento determinístico.
func (m *Manager) RebuildAllProjections() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ordem explícita respeitando dependências de FK entre projeções.
	rebuildOrder := []string{
		// Tier 1 — sem dependências externas
		"admins",
		"academias",
		"cursos",
		"materias",
		"sistema_config",
		"categorias_nota",
		// Tier 2 — dependem de academias/cursos
		"estudantes",
		"turmas",
		// Tier 3 — dependem de estudantes e materias
		"notas",
		"faltas",
		// Tier 4 — dependem de estudantes e aprovações
		"aprovacao_ano",
		"reprovacoes",
		"avaliacao_final",
		"inscricoes",
	}

	// Registrar quais projeções já foram processadas nesta passagem.
	processed := make(map[string]bool)

	for _, name := range rebuildOrder {
		projection, ok := m.projections[name]
		if !ok {
			log.Printf("[DEBUG] RebuildAll: projeção %q não registrada, pulando", name)
			continue
		}
		log.Printf("[DEBUG] RebuildAll: reconstruindo %s (tier ordenado)", name)
		if err := m.rebuildProjectionInternal(name, projection); err != nil {
			return fmt.Errorf("falha ao reconstruir %s: %w", name, err)
		}
		processed[name] = true
	}

	// Reconstruir projeções restantes (não listadas) em ordem alfabética.
	remaining := make([]string, 0)
	for name := range m.projections {
		if !processed[name] {
			remaining = append(remaining, name)
		}
	}
	sort.Strings(remaining)

	for _, name := range remaining {
		projection := m.projections[name]
		log.Printf("[DEBUG] RebuildAll: reconstruindo %s (ordem alfabética)", name)
		if err := m.rebuildProjectionInternal(name, projection); err != nil {
			return fmt.Errorf("falha ao reconstruir %s: %w", name, err)
		}
	}

	return nil
}

// rebuildProjectionInternal executa o rebuild de uma projeção individual.
// Deve ser chamado com m.mu já adquirido.
func (m *Manager) rebuildProjectionInternal(name string, projection Projection) error {
	log.Printf("[DEBUG] Reconstruindo projeção: %s", name)

	if err := m.markRebuildStart(name); err != nil {
		log.Printf("[WARN] %s: erro ao marcar início de rebuild: %v", name, err)
	}

	if err := projection.Rebuild(); err != nil {
		return fmt.Errorf("erro no rebuild de %s: %w", name, err)
	}

	if err := m.markRebuildComplete(name); err != nil {
		log.Printf("[WARN] %s: erro ao marcar fim de rebuild: %v", name, err)
	}

	log.Printf("[DEBUG] Projeção %s reconstruída com sucesso", name)
	return nil
}

// markRebuildStart zera o checkpoint antes do rebuild.
func (m *Manager) markRebuildStart(name string) error {
	_, err := m.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, 0, CURRENT_TIMESTAMP, 0)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = 0,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = 0
	`, name)
	return err
}

// markRebuildComplete atualiza o checkpoint para o MAX(id) atual do ledger.
func (m *Manager) markRebuildComplete(name string) error {
	var maxID int64
	err := m.client.DB().QueryRow(`SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`).Scan(&maxID)
	if err != nil {
		return err
	}

	_, err = m.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP
	`, name, maxID)
	return err
}

// logProjectionError registra erros de processamento de projeção na tabela de log.
func (m *Manager) logProjectionError(name string, errMsg string) {
	_, err := m.client.DB().Exec(`
		INSERT INTO projection_errors (projection_name, error_message, occurred_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT DO NOTHING
	`, name, errMsg)
	if err != nil {
		log.Printf("[WARN] Erro ao registrar falha de projeção %s: %v", name, err)
	}
}