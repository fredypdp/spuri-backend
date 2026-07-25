package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var ErrUniqueOperationInProgress = errors.New("unique operation in progress")

type UniqueOperationGuard struct {
	client *Client
	ctx    context.Context
}

type UniqueGuardReservation struct {
	ID    uuid.UUID
	Scope string
	Key   string
	guard *UniqueOperationGuard
}

type UniqueGuardOptions struct {
	AggregateType string
	AggregateID   *uuid.UUID
	UserID        string
	UserType      string
	Metadata      map[string]interface{}
	TTL           time.Duration
}

func NewUniqueOperationGuard(client *Client) *UniqueOperationGuard {
	return &UniqueOperationGuard{client: client, ctx: context.Background()}
}

func (g *UniqueOperationGuard) WithContext(ctx context.Context) *UniqueOperationGuard {
	if ctx == nil {
		ctx = context.Background()
	}
	return &UniqueOperationGuard{client: g.client, ctx: ctx}
}

func CanonicalGuardKey(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		normalized = append(normalized, strings.ToLower(strings.TrimSpace(part)))
	}
	return strings.Join(normalized, ":")
}

func MaskGuardKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:16]
}

func (g *UniqueOperationGuard) Reserve(scope, key string, opts UniqueGuardOptions) (*UniqueGuardReservation, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	key = strings.ToLower(strings.TrimSpace(key))
	if scope == "" || key == "" {
		return nil, fmt.Errorf("scope e key da guarda são obrigatórios")
	}
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	metadata, err := json.Marshal(opts.Metadata)
	if err != nil {
		return nil, fmt.Errorf("metadata inválido da guarda: %w", err)
	}

	tx, err := g.client.BeginTx(g.ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(g.ctx, `UPDATE unique_operation_guards SET status='expired', updated_at=NOW(), released_at=NOW() WHERE status='reserved' AND expires_at < NOW()`); err != nil {
		return nil, err
	}

	id := uuid.New()
	_, err = tx.ExecContext(g.ctx, `INSERT INTO unique_operation_guards (id, scope, key, aggregate_type, aggregate_id, user_id, user_type, metadata, status, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'reserved',NOW()+($9 * INTERVAL '1 second'))`, id, scope, key, nullString(opts.AggregateType), opts.AggregateID, nullString(opts.UserID), nullString(opts.UserType), metadata, int(ttl.Seconds()))
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return nil, ErrUniqueOperationInProgress
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &UniqueGuardReservation{ID: id, Scope: scope, Key: key, guard: g}, nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: strings.TrimSpace(value) != ""}
}

func (r *UniqueGuardReservation) Consume(aggregateID uuid.UUID) error {
	if r == nil {
		return nil
	}
	_, err := r.guard.client.DB().ExecContext(r.guard.ctx, `UPDATE unique_operation_guards SET status='consumed', aggregate_id=COALESCE(NULLIF($2, '00000000-0000-0000-0000-000000000000')::uuid, aggregate_id), consumed_at=NOW(), updated_at=NOW() WHERE id=$1 AND status='reserved'`, r.ID, aggregateID.String())
	return err
}

func (r *UniqueGuardReservation) Release() error {
	if r == nil {
		return nil
	}
	_, err := r.guard.client.DB().ExecContext(r.guard.ctx, `UPDATE unique_operation_guards SET status='released', released_at=NOW(), updated_at=NOW() WHERE id=$1 AND status IN ('reserved','consumed')`, r.ID)
	return err
}

func (g *UniqueOperationGuard) ReleaseKey(scope, key string) error {
	scope = strings.ToLower(strings.TrimSpace(scope))
	key = strings.ToLower(strings.TrimSpace(key))
	if scope == "" || key == "" {
		return fmt.Errorf("scope e key da guarda são obrigatórios")
	}
	_, err := g.client.DB().ExecContext(g.ctx, `UPDATE unique_operation_guards SET status='released', released_at=NOW(), updated_at=NOW() WHERE scope=$1 AND key=$2 AND status IN ('reserved','consumed')`, scope, key)
	return err
}
