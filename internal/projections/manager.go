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
	tx, err := m.client.DB().Begin()
	if err != nil {
		return fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = projection.HandleTx(tx, event); err != nil {
		return fmt.Errorf("HandleTx falhou: %w", err)
	}

	if _, err = tx.Exec(`
		INSERT INTO projection_checkpoints (projection_name, last_processed_event_id, last_processed_at, events_processed)
		VALUES ($1, $2, CURRENT_TIMESTAMP, 1)
		ON CONFLICT (projection_name) DO UPDATE SET
			last_processed_event_id = $2,
			last_processed_at       = CURRENT_TIMESTAMP,
			events_processed        = projection_checkpoints.events_processed + 1
	`, projection.Name(), event.ID); err != nil {
		return fmt.Errorf("erro ao gravar checkpoint transacional: %w", err)
	}

	return tx.Commit()
}

func (m *Manager) processEventWithRetry(name string, projection Projection, event db.Event) error {
	maxRetries := 3
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := projection.Handle(event)
		if err == nil {
			return nil
		}
		log.Printf("[WARN] %s: tentativa %d/%d falhou para evento %d: %v", name, attempt, maxRetries, event.ID, err)
		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 3
		}
	}
	return fmt.Errorf("falha após %d tentativas no evento %d", maxRetries, event.ID)
}

func (m *Manager) commitCheckpoint(projection Projection, eventID int64) error {
	return projection.UpdateCheckpoint(eventID)
}

// ============================================================================
// Rebuild
// ============================================================================

// RebuildProjection reconstrói uma projeção específica.
//
// FIX SEC-01: antes de processar qualquer evento, verifica a integridade do
// ledger para todos os aggregates relevantes. Se qualquer aggregate tiver
// cadeia de hashes comprometida, o rebuild é abortado com erro detalhado.
// Isso impede que dados adulterados sejam materializados nas projeções.
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
//
// FIX SEC-01: verifica integridade do ledger inteiro antes de iniciar qualquer
// rebuild. Se o ledger estiver comprometido, nenhuma projeção é reconstruída.
func (m *Manager) RebuildAllProjections() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// FIX SEC-01: verificação global antes de qualquer rebuild.
	// Um ledger adulterado não deve ser materializado em nenhuma projeção.
	log.Printf("[SECURITY] RebuildAll: verificando integridade do ledger antes de iniciar")
	if err := m.verifyFullLedgerIntegrity(); err != nil {
		return fmt.Errorf("rebuild abortado: integridade do ledger comprometida: %w", err)
	}
	log.Printf("[SECURITY] RebuildAll: ledger íntegro — iniciando reconstrução")

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
// FIX SEC-01: verifica a integridade do ledger para os aggregates que esta
// projeção consome antes de processar qualquer evento. Se a cadeia de hashes
// estiver comprometida, o rebuild é abortado — dados adulterados não são
// materializados.
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

	// FIX SEC-01: verificar integridade do ledger antes de processar eventos.
	// Cada projeção verifica os aggregates que ela consome (identificados pelo
	// aggregate_type que seus eventos pertencem).
	// RebuildAll já fez a verificação global; aqui verificamos novamente para
	// rebuilds individuais (chamados diretamente via handler HTTP).
	log.Printf("[SECURITY] %s: verificando integridade do ledger antes do rebuild", name)
	if err := m.verifyFullLedgerIntegrity(); err != nil {
		if resetErr := m.markRebuildFailed(name); resetErr != nil {
			log.Printf("[WARN] %s: erro ao resetar is_rebuilding após falha de integridade: %v", name, resetErr)
		}
		return fmt.Errorf("%s: rebuild abortado por integridade comprometida: %w", name, err)
	}
	log.Printf("[SECURITY] %s: ledger íntegro — prosseguindo com rebuild", name)

	// FIX SCHEMA-01: defer garante que is_rebuilding sempre volta para FALSE,
	// independente de sucesso ou falha do Rebuild(). Sem isso, uma falha
	// mantinha is_rebuilding=TRUE indefinidamente, tornando impossível
	// distinguir rebuild em andamento de rebuild que falhou.
	rebuildErr := projection.Rebuild()
	if rebuildErr != nil {
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

// verifyFullLedgerIntegrity verifica a integridade de todos os aggregates
// presentes no ledger. Retorna erro detalhado no primeiro aggregate comprometido.
//
// Esta função é chamada antes de qualquer rebuild para garantir que dados
// adulterados não sejam materializados nas projeções.
func (m *Manager) verifyFullLedgerIntegrity() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	aggregateIDs, err := m.eventStore.GetDistinctAggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("erro ao listar aggregates para verificação: %w", err)
	}

	if len(aggregateIDs) == 0 {
		log.Printf("[SECURITY] Ledger vazio — nenhuma verificação necessária")
		return nil
	}

	log.Printf("[SECURITY] Verificando integridade de %d aggregate(s)", len(aggregateIDs))

	compromised := 0
	for _, aggID := range aggregateIDs {
		valid, err := m.eventStore.VerifyLedgerIntegrity(ctx, aggID)
		if err != nil {
			// VerifyLedgerIntegrity retorna erro com detalhes quando !valid
			compromised++
			log.Printf("[SECURITY] ALERTA: aggregate %s comprometido: %v", aggID, err)
			// Retorna no primeiro comprometido — não faz sentido continuar rebuild
			return fmt.Errorf("aggregate %s: %w", aggID, err)
		}
		if !valid {
			compromised++
			return fmt.Errorf("aggregate %s: integridade inválida sem detalhes", aggID)
		}
	}

	log.Printf("[SECURITY] Verificação concluída: %d aggregate(s) íntegros", len(aggregateIDs))
	return nil
}

// ============================================================================
// Checkpoint helpers
// ============================================================================

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
			is_rebuilding     = FALSE,
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

// ============================================================================
// Event fetch
// ============================================================================

// getNewEvents busca eventos do ledger com id > fromID, limitado a batchSize.
//
// FIX MGR-02: contexto com timeout — impede bloqueio indefinido.
func (m *Manager) getNewEvents(fromID int64) ([]db.Event, error) {
	if fromID < 0 {
		fromID = 0
	}
	limit := db.ValidateLimit(m.batchSize)

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

// ============================================================================
// Status / introspection
// ============================================================================

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