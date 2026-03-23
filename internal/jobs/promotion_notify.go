package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PromotionNotifyJob notifies reviewers of pending promotion requests.
// Webhook/email delivery is a future extension; the job currently logs stale
// requests so operators can act on them.
type PromotionNotifyJob struct {
	pool *pgxpool.Pool
}

// NewPromotionNotifyJob creates a new PromotionNotifyJob.
func NewPromotionNotifyJob(pool *pgxpool.Pool) *PromotionNotifyJob {
	return &PromotionNotifyJob{pool: pool}
}

type pendingPromotion struct {
	id            uuid.UUID
	memoryID      uuid.UUID
	targetScopeID uuid.UUID
	createdAt     time.Time
}

// Run checks for pending promotion requests older than 24 hours and logs them.
func (j *PromotionNotifyJob) Run(ctx context.Context) error {
	rows, err := j.pool.Query(ctx,
		`SELECT id, memory_id, target_scope_id, created_at
		 FROM promotion_requests
		 WHERE status = 'pending' AND created_at < now() - interval '24 hours'
		 ORDER BY created_at`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var pending []pendingPromotion
	for rows.Next() {
		var p pendingPromotion
		if err := rows.Scan(&p.id, &p.memoryID, &p.targetScopeID, &p.createdAt); err != nil {
			return err
		}
		pending = append(pending, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(pending) == 0 {
		slog.Info("promotion notify: no stale pending requests")
		return nil
	}

	for _, p := range pending {
		age := time.Since(p.createdAt).Round(time.Minute)
		slog.Warn("promotion notify: stale pending request",
			"id", p.id,
			"memory_id", p.memoryID,
			"target_scope_id", p.targetScopeID,
			"age", age,
		)
	}
	slog.Info("promotion notify: stale requests logged", "count", len(pending))
	return nil
}
