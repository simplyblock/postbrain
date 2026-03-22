package jobs

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PromotionNotifyJob notifies reviewers of pending promotion requests.
// TODO(task-jobs): implement webhook/email notification. Currently just logs pending requests.
type PromotionNotifyJob struct {
	pool *pgxpool.Pool
}

// NewPromotionNotifyJob creates a new PromotionNotifyJob.
func NewPromotionNotifyJob(pool *pgxpool.Pool) *PromotionNotifyJob {
	return &PromotionNotifyJob{pool: pool}
}

// Run checks for pending promotion requests and logs them.
func (j *PromotionNotifyJob) Run(ctx context.Context) error {
	// TODO(task-jobs): fetch pending promotion_requests older than 24h and notify reviewer
	slog.Info("promotion notify job: checking pending promotion requests")
	return nil
}
