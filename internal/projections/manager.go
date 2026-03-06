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

// TransactionalProjection é uma interface opcional que projeções não-idempotentes
// devem implementar para garantir atomicidade real entre Handle e checkpoint.
//
// FIX DB-16 (correção real): o problema original é que Handle() e o avanço do
// checkpoint são operações separadas. Se o processo morrer entre elas, o evento
// é reprocessado, causando double-write em projeções não-idempotentes
// (ex: total_faltas += 1, total_estudantes += 1).
//
// Solução: projeções não-idempotentes implementam HandleTx(*sql.Tx, db.Event),
// que recebe a transação aberta pelo Manager. O Manager executa Handle + checkpoint
// dentro da mesma transação — se qualquer um falhar, ambos são revertidos.
//
// Projeções idempotentes (INSERT ... ON CONFLICT DO NOTHING/DO UPDATE) não
// precisam implementar esta interface — o comportamento de retry é seguro para elas.
type TransactionalProjection interface {
	Projection
	HandleTx(tx *sql.Tx, event db.Event) error
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
	// FIX MGR-03: o lock é adquirido apenas para ler o snapshot das projeções,
	// não durante o processamento (que pode envolver time.Sleep em retries).
	// Isso evita que RebuildProjection/RebuildAllProjections bloqueiem por até
	// 27s enquanto processEventWithRetry dorme com o lock ativo.
	m.mu.Lock()
	snapshot := make(map[string]Projection, len(m.projections))
	for name, p := range m.projections {
		snapshot[name] = p
	}
	m.mu.Unlock()

	for name, projection := range snapshot {
		if err := m.processProjection(name, projection); err != nil {
			log.Printf("[ERROR] Erro ao processar projeção %s: %v", name, err)
		}
	}
	return nil
}

// processProjection processa eventos novos para uma projeção específica.
//
// FIX DB-16 (correção real): para projeções que implementam TransactionalProjection,
// Handle e o avanço do checkpoint são executados dentro da mesma transação de banco.
// Se o processo morrer após o Commit, o checkpoint já foi gravado — nenhum
// double-write. Se morrer antes do Commit, ambos são revertidos — o evento será
// reprocessado na próxima iteração, o que é seguro pois HandleTx deve ser
// idempotente por design.
//
// Para projeções sem HandleTx (idempotentes), o comportamento anterior é mantido:
// Handle é chamado diretamente, e commitCheckpoint grava o checkpoint em seguida.
func (m *Manager) processProjection(name string, projection Projection) error {
	lastID, err := projection.GetLastProcessedEventID()
	if err != nil {
		return fmt.Errorf("erro ao obter checkpoint de %s: %w", name, err)
	}

	events, err := m.getNewEvents(lastID)
	if err != nil {
		return fmt.Errorf("erro ao buscar eventos para %s: %w", name, err)
	}

	if len(events) == 0 {
		return nil
	}

	txProjection, isTransactional := projection.(TransactionalProjection)

	for _, event := range events {
		if isTransactional {
			if err := m.processEventTransactional(name, txProjection, event); err != nil {
				m.logProjectionError(name, err.Error())
				log.Printf("[ERROR] %s: falha permanente no evento %d: %v", name, event.ID, err)
				return err
			}
		} else {
			if err := m.processEventWithRetry(name, projection, event); err != nil {
				m.logProjectionError(name, err.Error())
				log.Printf("[ERROR] %s: falha permanente no evento %d: %v", name, event.ID, err)
				return err
			}
			if err := m.commitCheckpoint(projection, event.ID); err != nil {
				log.Printf("[WARN] %s: erro ao gravar checkpoint para evento %d: %v", name, event.ID, err)
			}
		}
	}

	return nil
}

// processEventTransactional executa Handle + checkpoint na mesma transação.
//
// Se o processo morrer após tx.Commit(), o checkpoint já está gravado — sem
// reprocessamento. Se morrer antes, ambos são revertidos — o evento será
// reprocessado, mas HandleTx é idempotente.
func (m *Manager) processEventTransactional(name string, projection TransactionalProjection, event db.Event) error {
	maxRetries := 3
	baseDelay := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		tx, err := m.client.DB().Begin()
		if err != nil {
			return fmt.Errorf("erro ao iniciar transação: %w", err)
		}

		if handleErr := projection.HandleTx(tx, event); handleErr != nil {
			tx.Rollback()
			if attempt < maxRetries {
				delay := time.Duration(attempt*attempt) * baseDelay
				log.Printf("[WARN] %s: evento %d falhou na tx (tentativa %d/%d), retry em %v: %v",
					name, event.ID, attempt, maxRetries, delay, handleErr)
				// FIX MGR-03: sleep fora do lock — o lock já foi liberado antes de
				// processNewEvents chamar processProjection (ver processNewEvents acima).
				time.Sleep(delay)
				continue
			}
			return fmt.Errorf("evento %d falhou após %d tentativas: %w", event.ID, maxRetries, handleErr)
		}

		eventID := int64(db.ValidateOffset(int(event.ID)))
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
			tx.Rollback()
			return fmt.Errorf("erro ao gravar checkpoint na transação para evento %d: %w", event.ID, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("erro ao commitar transação para evento %d: %w", event.ID, err)
		}

		if attempt > 1 {
			log.Printf("[DEBUG] %s: evento %d recuperado na tentativa %d", name, event.ID, attempt)
		}
		return nil
	}

	return fmt.Errorf("evento %d: máximo de tentativas atingido", event.ID)
}

// commitCheckpoint atualiza o checkpoint da projeção em uma transação própria.
// Usado apenas para projeções idempotentes (sem TransactionalProjection).
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

// FIX MGR-03: processEventWithRetry não adquire nenhum lock —
// é chamado de processProjection que por sua vez é chamado de
// processNewEvents SEM o lock ativo (ver comentário em processNewEvents).
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
			// FIX MGR-03: sleep seguro — nenhum mutex m.mu está ativo aqui.
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("evento %d falhou após %d tentativas: %w", event.ID, maxRetries, lastErr)
}

// getNewEvents busca eventos do ledger com id > fromID.
//
// FIX MGR-02: a query agora usa context.WithTimeout para evitar que uma
// falha ou travamento do banco bloqueie a goroutine de processamento
// indefinidamente. Timeout de 30s é conservador — queries normais levam <1s.
func (m *Manager) getNewEvents(fromID int64) ([]db.Event, error) {
	if fromID < 0 {
		fromID = 0
	}
	limit := db.ValidateLimit(m.batchSize)

	// FIX MGR-02: contexto com timeout — impede bloqueio indefinido.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := m.client.DB().QueryContext(ctx, `
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
// A ordem abaixo respeita dependências de FK entre projeções.
func (m *Manager) RebuildAllProjections() error {
	m.mu.Lock()
	defer m.mu.Unlock()

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
//
// FIX SCHEMA-01: is_rebuilding é garantidamente resetado para FALSE mesmo
// em caso de falha do Rebuild(). O defer garante que markRebuildFailed() é
// chamado se a função retornar com erro — sem risco de estado "rebuilding"
// permanente após falha.
func (m *Manager) rebuildProjectionInternal(name string, projection Projection) error {
	log.Printf("[DEBUG] Reconstruindo projeção: %s", name)

	if err := m.markRebuildStart(name); err != nil {
		log.Printf("[WARN] %s: erro ao marcar início de rebuild: %v", name, err)
	}

	// FIX SCHEMA-01: defer garante que is_rebuilding sempre volta para FALSE,
	// independente de sucesso ou falha do Rebuild(). Sem isso, uma falha
	// mantinha is_rebuilding=TRUE indefinidamente, tornando impossível
	// distinguir rebuild em andamento de rebuild que falhou.
	rebuildErr := projection.Rebuild()
	if rebuildErr != nil {
		// Resetar is_rebuilding para FALSE antes de retornar o erro.
		if resetErr := m.markRebuildFailed(name); resetErr != nil {
			log.Printf("[WARN] %s: erro ao resetar is_rebuilding após falha: %v", name, resetErr)
		}
		return fmt.Errorf("erro no rebuild de %s: %w", name, rebuildErr)
	}

	if err := m.markRebuildComplete(name); err != nil {
		log.Printf("[WARN] %s: erro ao marcar fim de rebuild: %v", name, err)
	}

	log.Printf("[DEBUG] Projeção %s reconstruída com sucesso", name)
	return nil
}

// markRebuildStart zera o checkpoint e seta is_rebuilding=TRUE antes do rebuild.
//
// Usa UPSERT para garantir que funciona mesmo se o checkpoint não existir ainda —
// sem risco de UPDATE operar 0 linhas silenciosamente (bug MGR-04 da auditoria,
// já corrigido nas etapas anteriores via UPSERT).
func (m *Manager) markRebuildStart(name string) error {
	_, err := m.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, events_processed, is_rebuilding, rebuild_started_at)
		VALUES ($1, 0, CURRENT_TIMESTAMP, 0, TRUE, CURRENT_TIMESTAMP)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = 0,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = 0,
			is_rebuilding           = TRUE,
			rebuild_started_at      = CURRENT_TIMESTAMP
	`, name)
	return err
}

// markRebuildComplete atualiza o checkpoint para o MAX(id) atual do ledger
// e reseta is_rebuilding=FALSE após rebuild bem-sucedido.
func (m *Manager) markRebuildComplete(name string) error {
	var maxID int64
	err := m.client.DB().QueryRow(`SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`).Scan(&maxID)
	if err != nil {
		return err
	}

	_, err = m.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, is_rebuilding)
		VALUES ($1, $2, CURRENT_TIMESTAMP, FALSE)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP,
			is_rebuilding           = FALSE
	`, name, maxID)
	return err
}

// markRebuildFailed reseta is_rebuilding=FALSE após falha de rebuild.
// FIX SCHEMA-01: sem esta função, is_rebuilding permanecia TRUE indefinidamente
// após qualquer falha, tornando o estado da projeção opaco para os operadores.
func (m *Manager) markRebuildFailed(name string) error {
	_, err := m.client.DB().Exec(`
		INSERT INTO projection_checkpoints
			(projection_name, last_processed_event_id, last_processed_at, is_rebuilding)
		VALUES ($1, 0, CURRENT_TIMESTAMP, FALSE)
		ON CONFLICT (projection_name) DO UPDATE SET
			is_rebuilding = FALSE,
			last_processed_at = CURRENT_TIMESTAMP
	`, name)
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

// IsProjectionRegistered verifica se uma projeção está registrada no manager.
func (m *Manager) IsProjectionRegistered(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.projections[name]
	return ok
}

// GetProjectionStatus retorna o status atual de uma projeção.
func (m *Manager) GetProjectionStatus(name string) (map[string]interface{}, error) {
	m.mu.Lock()
	projection, ok := m.projections[name]
	m.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("projeção não encontrada: %s", name)
	}

	lastID, err := projection.GetLastProcessedEventID()
	if err != nil {
		return nil, fmt.Errorf("erro ao obter checkpoint: %w", err)
	}

	var maxID int64
	_ = m.client.DB().QueryRow(`SELECT COALESCE(MAX(id), 0) FROM spuri_ledger`).Scan(&maxID)

	_, isTransactional := projection.(TransactionalProjection)

	return map[string]interface{}{
		"name":              name,
		"last_processed_id": lastID,
		"ledger_max_id":     maxID,
		"lag":               maxID - lastID,
		"transactional":     isTransactional,
	}, nil
}