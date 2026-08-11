package jobs

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Store persiste e recupera jobs.
// Usa memória como cache quente e PostgreSQL como storage durável.
type Store struct {
	db            *sqlx.DB
	mu            sync.RWMutex
	cache         map[uuid.UUID]*Job
	hiddenSSEJobs map[uuid.UUID]map[uuid.UUID]struct{}
}

func NewStore(db *sqlx.DB) *Store {
	s := &Store{
		db:            db,
		cache:         make(map[uuid.UUID]*Job),
		hiddenSSEJobs: make(map[uuid.UUID]map[uuid.UUID]struct{}),
	}
	return s
}

// Enqueue cria e persiste um novo job com status "pending".
func (s *Store) Enqueue(jobType JobType, userID uuid.UUID, userType string, payload json.RawMessage, totalItems int) (*Job, error) {
	now := time.Now()
	j := &Job{
		ID:         uuid.New(),
		Type:       jobType,
		Status:     StatusPending,
		UserID:     userID,
		UserType:   userType,
		Payload:    payload,
		Results:    []ItemResult{},
		TotalItems: totalItems,
		CreatedAt:  now,
	}

	if err := s.persist(j); err != nil {
		return nil, fmt.Errorf("store.Enqueue: persist: %w", err)
	}

	s.mu.Lock()
	s.cache[j.ID] = j
	s.mu.Unlock()

	log.Printf("[jobs] enqueued %s id=%s items=%d", jobType, j.ID, totalItems)
	return j, nil
}

// Get retorna um job por ID. Busca no cache primeiro, depois no banco.
func (s *Store) Get(id uuid.UUID) (*Job, error) {
	s.mu.RLock()
	j, ok := s.cache[id]
	s.mu.RUnlock()
	if ok {
		return j, nil
	}
	return s.loadFromDB(id)
}

// GetByUser retorna todos os jobs de um usuário (últimos 100, mais recentes primeiro).
func (s *Store) GetByUser(userID uuid.UUID) ([]*Job, error) {
	rows, err := s.db.Query(`
		SELECT id, type, status, user_id, user_type,
		       payload, results, total_items, done_items, fail_items,
		       error, created_at, started_at, completed_at
		FROM async_jobs
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 100
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("store.GetByUser: %w", err)
	}
	defer rows.Close()
	return scanJobs(rows)
}

// ListActive retorna jobs pendentes/em processamento para recuperação.
func (s *Store) ListActive(limit int) ([]*Job, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.Query(`
		SELECT id, type, status, user_id, user_type,
		       payload, results, total_items, done_items, fail_items,
		       error, created_at, started_at, completed_at
		FROM async_jobs
		WHERE status IN ('pending','processing')
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("store.ListActive: %w", err)
	}
	defer rows.Close()

	jobList, err := scanJobs(rows)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	for _, j := range jobList {
		s.cache[j.ID] = j
	}
	s.mu.Unlock()
	return jobList, nil
}

// UpdateStatus atualiza o status de um job (thread-safe).
func (s *Store) UpdateStatus(id uuid.UUID, status Status, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.cache[id]
	if !ok {
		var err error
		j, err = s.loadFromDB(id)
		if err != nil {
			return err
		}
	}

	j.Status = status
	j.Error = errMsg

	now := time.Now()
	switch status {
	case StatusProcessing:
		j.StartedAt = &now
	case StatusDone, StatusFailed:
		j.CompletedAt = &now
	}

	return s.persist(j)
}

// AppendResultInMemory adiciona o resultado de um item ao estado em memória
// do job (contadores DoneItems/FailItems e o array Results), sem persistir no
// banco. Usado pelo worker para acumular vários itens entre checkpoints — ver
// FlushResults. Thread-safe.
func (s *Store) AppendResultInMemory(id uuid.UUID, item ItemResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.cache[id]
	if !ok {
		return fmt.Errorf("store.AppendResultInMemory: job %s não encontrado no cache", id)
	}

	j.Results = append(j.Results, item)
	if item.Sucesso {
		j.DoneItems++
	} else {
		j.FailItems++
	}
	return nil
}

// FlushResults persiste no banco o estado atual (em memória) do job — usado
// como checkpoint periódico durante o processamento de um lote, e sempre no
// último item (sucesso ou falha), para garantir que o resultado final de um
// job nunca fique só em memória. Thread-safe.
func (s *Store) FlushResults(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	j, ok := s.cache[id]
	if !ok {
		return fmt.Errorf("store.FlushResults: job %s não encontrado no cache", id)
	}
	return s.persist(j)
}

// persist grava o job no banco (deve ser chamado com o lock já adquirido quando necessário).
func (s *Store) persist(j *Job) error {
	resultsJSON, err := json.Marshal(j.Results)
	if err != nil {
		return fmt.Errorf("persist: marshal results: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT INTO async_jobs (
			id, type, status, user_id, user_type,
			payload, results, total_items, done_items, fail_items,
			error, created_at, started_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (id) DO UPDATE SET
			status       = EXCLUDED.status,
			results      = EXCLUDED.results,
			done_items   = EXCLUDED.done_items,
			fail_items   = EXCLUDED.fail_items,
			error        = EXCLUDED.error,
			started_at   = EXCLUDED.started_at,
			completed_at = EXCLUDED.completed_at
	`,
		j.ID, string(j.Type), string(j.Status), j.UserID, j.UserType,
		j.Payload, resultsJSON, j.TotalItems, j.DoneItems, j.FailItems,
		j.Error, j.CreatedAt, j.StartedAt, j.CompletedAt,
	)
	return err
}

func (s *Store) loadFromDB(id uuid.UUID) (*Job, error) {
	rows, err := s.db.Query(`
		SELECT id, type, status, user_id, user_type,
		       payload, results, total_items, done_items, fail_items,
		       error, created_at, started_at, completed_at
		FROM async_jobs WHERE id = $1
	`, id)
	if err != nil {
		return nil, fmt.Errorf("store.loadFromDB: %w", err)
	}
	defer rows.Close()

	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("job %s não encontrado", id)
	}

	j := jobs[0]
	s.cache[j.ID] = j
	return j, nil
}

func scanJobs(rows *sql.Rows) ([]*Job, error) {
	var jobs []*Job
	for rows.Next() {
		var j Job
		var jobType, status string
		var resultsJSON []byte
		var errStr sql.NullString

		if err := rows.Scan(
			&j.ID, &jobType, &status, &j.UserID, &j.UserType,
			&j.Payload, &resultsJSON, &j.TotalItems, &j.DoneItems, &j.FailItems,
			&errStr, &j.CreatedAt, &j.StartedAt, &j.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scanJobs: %w", err)
		}

		j.Type = JobType(jobType)
		j.Status = Status(status)
		if errStr.Valid {
			j.Error = errStr.String
		}
		if len(resultsJSON) > 0 {
			_ = json.Unmarshal(resultsJSON, &j.Results)
		}
		if j.Results == nil {
			j.Results = []ItemResult{}
		}

		jobs = append(jobs, &j)
	}
	return jobs, rows.Err()
}

// Cleanup remove jobs concluídos com mais de 24h (chamado periodicamente).
func (s *Store) Cleanup() {
	cutoff := time.Now().Add(-24 * time.Hour)
	result, err := s.db.Exec(`
		DELETE FROM async_jobs
		WHERE status IN ('done', 'failed') AND completed_at < $1
	`, cutoff)
	if err != nil {
		log.Printf("[jobs] cleanup error: %v", err)
		return
	}

	n, _ := result.RowsAffected()
	if n > 0 {
		log.Printf("[jobs] cleanup: %d jobs antigos removidos", n)
	}

	// Remover do cache também
	s.mu.Lock()
	for id, j := range s.cache {
		if j.IsDone() && j.CompletedAt != nil && j.CompletedAt.Before(cutoff) {
			delete(s.cache, id)
		}
	}
	s.mu.Unlock()
}

// HideFromSSE marca um job como oculto no stream SSE para um usuário.
func (s *Store) HideFromSSE(userID, jobID uuid.UUID) error {
	_, err := s.db.Exec(`
		INSERT INTO async_job_sse_hidden (user_id, job_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, job_id) DO NOTHING
	`, userID, jobID)
	if err != nil {
		return fmt.Errorf("store.HideFromSSE: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.hiddenSSEJobs[userID]; !ok {
		s.hiddenSSEJobs[userID] = make(map[uuid.UUID]struct{})
	}
	s.hiddenSSEJobs[userID][jobID] = struct{}{}
	return nil
}

// UnhideFromSSE remove a marca de ocultação do stream SSE para um usuário.
func (s *Store) UnhideFromSSE(userID, jobID uuid.UUID) error {
	_, err := s.db.Exec(`
		DELETE FROM async_job_sse_hidden
		WHERE user_id = $1 AND job_id = $2
	`, userID, jobID)
	if err != nil {
		return fmt.Errorf("store.UnhideFromSSE: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if hidden, ok := s.hiddenSSEJobs[userID]; ok {
		delete(hidden, jobID)
		if len(hidden) == 0 {
			delete(s.hiddenSSEJobs, userID)
		}
	}
	return nil
}

// IsHiddenFromSSE indica se um job deve ser filtrado no stream SSE do usuário.
func (s *Store) IsHiddenFromSSE(userID, jobID uuid.UUID) (bool, error) {
	s.mu.RLock()
	if hidden, ok := s.hiddenSSEJobs[userID]; ok {
		_, exists := hidden[jobID]
		s.mu.RUnlock()
		return exists, nil
	}
	s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT job_id
		FROM async_job_sse_hidden
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return false, fmt.Errorf("store.IsHiddenFromSSE: %w", err)
	}
	defer rows.Close()

	loaded := make(map[uuid.UUID]struct{})
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return false, fmt.Errorf("store.IsHiddenFromSSE scan: %w", err)
		}
		loaded[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("store.IsHiddenFromSSE rows: %w", err)
	}

	s.mu.Lock()
	s.hiddenSSEJobs[userID] = loaded
	s.mu.Unlock()

	_, exists := loaded[jobID]
	return exists, nil
}
