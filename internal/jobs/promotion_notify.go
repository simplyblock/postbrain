package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
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

// Run checks for pending promotion requests older than 24 hours and logs them.
func (j *PromotionNotifyJob) Run(ctx context.Context) error {
	pending, err := db.New(j.pool).GetStalePromotionRequests(ctx)
	if err != nil {
		return err
	}

	if len(pending) == 0 {
		slog.Info("promotion notify: no stale pending requests")
		return nil
	}

	for _, p := range pending {
		age := time.Since(p.CreatedAt).Round(time.Minute)
		slog.Warn("promotion notify: stale pending request",
			"id", p.ID,
			"memory_id", p.MemoryID,
			"target_scope_id", p.TargetScopeID,
			"age", age,
		)
	}
	slog.Info("promotion notify: stale requests logged", "count", len(pending))
	return nil
}
