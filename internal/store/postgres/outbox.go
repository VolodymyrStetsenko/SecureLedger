package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/VolodymyrStetsenko/secureledger/internal/risk"
)

var _ risk.Outbox = (*Store)(nil)

func (s *Store) ClaimRiskEvents(ctx context.Context, limit int, now time.Time) ([]risk.Delivery, error) {
	limit = normaliseOutboxLimit(limit)
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("begin risk claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		WITH candidates AS (
			SELECT id
			FROM risk_events
			WHERE attempts < 10
			  AND (
				(status IN ('pending', 'failed') AND available_at <= $1)
				OR (status = 'processing' AND locked_at < $1 - interval '1 minute')
			  )
			ORDER BY available_at, created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		)
		UPDATE risk_events event
		SET status = 'processing', attempts = event.attempts + 1,
		    locked_at = $1, last_error = NULL
		FROM candidates
		WHERE event.id = candidates.id
		RETURNING event.id, event.event_type, event.severity, event.transaction_id,
		          event.reason, event.created_at, event.attempts`, now.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("claim risk events: %w", err)
	}
	defer rows.Close()

	deliveries := make([]risk.Delivery, 0, limit)
	for rows.Next() {
		var delivery risk.Delivery
		if err := rows.Scan(
			&delivery.Event.ID,
			&delivery.Event.Type,
			&delivery.Event.Severity,
			&delivery.Event.TransferID,
			&delivery.Event.Reason,
			&delivery.Event.CreatedAt,
			&delivery.Attempts,
		); err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit risk claim: %w", err)
	}
	return deliveries, nil
}

func (s *Store) MarkRiskEventPublished(ctx context.Context, id string, publishedAt time.Time) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE risk_events
		SET status = 'published', published_at = $2, locked_at = NULL, last_error = NULL
		WHERE id = $1 AND status = 'processing'`, id, publishedAt.UTC())
	if err != nil {
		return fmt.Errorf("mark risk event published: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("risk event %s is not claimed", id)
	}
	return nil
}

func (s *Store) MarkRiskEventFailed(ctx context.Context, id, reason string, retryAt time.Time) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE risk_events
		SET status = 'failed', available_at = $2, locked_at = NULL,
		    published_at = NULL, last_error = $3
		WHERE id = $1 AND status = 'processing'`, id, retryAt.UTC(), reason)
	if err != nil {
		return fmt.Errorf("mark risk event failed: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("risk event %s is not claimed", id)
	}
	return nil
}

func normaliseOutboxLimit(limit int) int {
	if limit <= 0 {
		return 32
	}
	if limit > 100 {
		return 100
	}
	return limit
}
