// Package jobs provides background job scheduling and execution for Postbrain.
// It uses robfig/cron for in-process scheduling and supplements the pg_cron
// jobs that run directly inside PostgreSQL.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/embedding"
	"github.com/simplyblock/postbrain/internal/metrics"
)

// Scheduler manages the lifecycle of all background jobs.
type Scheduler struct {
	cron *cron.Cron
	pool *pgxpool.Pool
	svc  *embedding.EmbeddingService
	cfg  *config.JobsConfig
}

// NewScheduler creates a new Scheduler with the given database pool,
// embedding service, and jobs configuration.
func NewScheduler(pool *pgxpool.Pool, svc *embedding.EmbeddingService, cfg *config.JobsConfig) *Scheduler {
	return &Scheduler{
		cron: cron.New(),
		pool: pool,
		svc:  svc,
		cfg:  cfg,
	}
}

// Register adds all enabled jobs to the cron scheduler.
// Each job is wrapped in a recovery wrapper that logs panics as slog.Error.
func (s *Scheduler) Register() error {
	if s.cfg.ConsolidationEnabled {
		job := NewConsolidateJob(s.pool, s.svc, nil)
		if _, err := s.cron.AddFunc("@every 6h", safeRun("consolidation", func() {
			if err := job.Run(context.Background()); err != nil {
				slog.Error("consolidation job failed", "error", err)
			}
		})); err != nil {
			return err
		}
	}

	if s.cfg.ReembedEnabled {
		job := NewReembedJob(s.pool, s.svc, 0)
		if _, err := s.cron.AddFunc("@every 1h", safeRun("reembed", func() {
			ctx := context.Background()
			if err := job.RunText(ctx); err != nil {
				slog.Error("reembed text job failed", "error", err)
			}
			if err := job.RunCode(ctx); err != nil {
				slog.Error("reembed code job failed", "error", err)
			}
		})); err != nil {
			return err
		}
	}

	if s.cfg.ContradictionEnabled {
		job := NewContradictionJob(s.pool, s.svc, nil)
		if _, err := s.cron.AddFunc("@weekly", safeRun("contradiction", func() {
			if err := job.Run(context.Background()); err != nil {
				slog.Error("contradiction job failed", "error", err)
			}
		})); err != nil {
			return err
		}
	}

	if s.cfg.AgeCheckEnabled {
		slog.Info("low_access_age staleness detection is handled by pg_cron job detect-stale-knowledge-age")
	}

	if s.cfg.BackfillSummariesEnabled {
		job := NewBackfillSummariesJob(s.pool, s.svc, 0)
		if _, err := s.cron.AddFunc("@every 24h", safeRun("backfill_summaries", func() {
			if err := job.Run(context.Background()); err != nil {
				slog.Error("backfill summaries job failed", "error", err)
			}
		})); err != nil {
			return err
		}
	}

	if s.cfg.ChunkBackfillEnabled {
		job := NewChunkBackfillJob(s.pool, s.svc, 0)
		if _, err := s.cron.AddFunc("@every 24h", safeRun("chunk_backfill", func() {
			ctx := context.Background()
			if err := job.RunMemories(ctx); err != nil {
				slog.Error("chunk backfill memories job failed", "error", err)
			}
			if err := job.RunArtifacts(ctx); err != nil {
				slog.Error("chunk backfill artifacts job failed", "error", err)
			}
		})); err != nil {
			return err
		}
	}

	return nil
}

// Start starts the cron scheduler (non-blocking).
func (s *Scheduler) Start() {
	s.cron.Start()
}

// Stop gracefully stops the scheduler and waits for running jobs to complete.
func (s *Scheduler) Stop(ctx context.Context) {
	stopCtx := s.cron.Stop()
	select {
	case <-stopCtx.Done():
	case <-ctx.Done():
	}
}

// safeRun wraps a job function with panic recovery and structured logging.
// If the wrapped function panics, the panic is caught and logged as slog.Error
// rather than propagating to the cron scheduler's goroutine.
func safeRun(name string, fn func()) func() {
	return func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("job panicked", "job", name, "panic", r)
			}
		}()
		slog.Info("job started", "job", name)
		start := time.Now()
		defer func() {
			metrics.JobDuration.WithLabelValues(name).Observe(time.Since(start).Seconds())
		}()
		fn()
		slog.Info("job finished", "job", name, "duration", time.Since(start))
	}
}
