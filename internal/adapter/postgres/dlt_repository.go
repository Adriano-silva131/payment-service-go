package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adriano-linux/payment-service-go/internal/domain"
)

type DltRepository struct {
	pool *pgxpool.Pool
}

func NewDltRepository(pool *pgxpool.Pool) *DltRepository {
	return &DltRepository{pool: pool}
}

func (r *DltRepository) ExistsPendingByTopicAndKey(ctx context.Context, topic, key string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM dlt_messages
			WHERE original_topic = $1 AND message_key = $2 AND status = 'PENDING'
		)
	`, topic, key).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking pending dlt message: %w", err)
	}
	return exists, nil
}

func (r *DltRepository) Insert(ctx context.Context, msg *domain.DltMessage) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO dlt_messages (id, original_topic, message_key, event_type, payload, error_message, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT DO NOTHING
	`, msg.ID, msg.OriginalTopic, msg.MessageKey, msg.EventType, msg.Payload, msg.ErrorMessage, msg.Status, msg.CreatedAt)
	if err != nil {
		return fmt.Errorf("inserting dlt message: %w", err)
	}
	return nil
}

// ProcessPendingLocked runs fn for up to `limit` PENDING messages, holding a
// SELECT ... FOR UPDATE SKIP LOCKED row lock for each one until fn returns and (on
// success) the REPROCESSED update commits — this mirrors the old Java
// DltReprocessingService's @Lock(PESSIMISTIC_WRITE) behavior, including holding the lock
// across the outbound Kafka republish call, so concurrent instances never double-process
// the same message. Safe for a single low-volume reprocessing job; not a general-purpose
// pattern for high-throughput outbound calls.
func (r *DltRepository) ProcessPendingLocked(ctx context.Context, limit int, fn func(ctx context.Context, msg *domain.DltMessage) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning dlt reprocessing transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op if already committed

	rows, err := tx.Query(ctx, `
		SELECT id, original_topic, message_key, event_type, payload, error_message, status, created_at, reprocessed_at
		FROM dlt_messages
		WHERE status = 'PENDING'
		ORDER BY created_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return fmt.Errorf("querying pending dlt messages: %w", err)
	}

	var messages []*domain.DltMessage
	for rows.Next() {
		msg, err := scanDltMessage(rows)
		if err != nil {
			rows.Close()
			return err
		}
		messages = append(messages, msg)
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return fmt.Errorf("iterating pending dlt messages: %w", rowsErr)
	}

	for _, msg := range messages {
		if err := fn(ctx, msg); err != nil {
			continue // leave PENDING, next tick retries
		}
		if _, err := tx.Exec(ctx, `
			UPDATE dlt_messages SET status = 'REPROCESSED', reprocessed_at = now() WHERE id = $1
		`, msg.ID); err != nil {
			return fmt.Errorf("marking dlt message %s reprocessed: %w", msg.ID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing dlt reprocessing transaction: %w", err)
	}
	return nil
}

func scanDltMessage(row pgx.Row) (*domain.DltMessage, error) {
	var msg domain.DltMessage
	err := row.Scan(
		&msg.ID, &msg.OriginalTopic, &msg.MessageKey, &msg.EventType,
		&msg.Payload, &msg.ErrorMessage, &msg.Status, &msg.CreatedAt, &msg.ReprocessedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scanning dlt message row: %w", err)
	}
	return &msg, nil
}
