package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/providers"
	"github.com/simplyblock/postbrain/internal/memory"
)

// ConsolidateJob runs LLM-assisted memory consolidation for all scopes.
type ConsolidateJob struct {
	pool      *pgxpool.Pool
	svc       *providers.EmbeddingService
	summarize func(ctx context.Context, contents []string) (string, error)
}

// NewConsolidateJob creates a new ConsolidateJob. If summarize is nil, a default
// summarizer that concatenates contents with a separator is used.
func NewConsolidateJob(pool *pgxpool.Pool, svc *providers.EmbeddingService, summarize func(ctx context.Context, contents []string) (string, error)) *ConsolidateJob {
	if summarize == nil {
		summarize = defaultSummarizer
	}
	return &ConsolidateJob{
		pool:      pool,
		svc:       svc,
		summarize: summarize,
	}
}

// Run finds and merges near-duplicate memory clusters across all active scopes.
func (j *ConsolidateJob) Run(ctx context.Context) error {
	// Fetch all distinct scope IDs that have consolidation candidates.
	scopeIDs, err := db.New(j.pool).GetScopesWithConsolidationCandidates(ctx)
	if err != nil {
		return fmt.Errorf("consolidate: fetch scopes: %w", err)
	}

	totalMerged := 0
	for _, scopeID := range scopeIDs {
		consolidator := memory.NewConsolidator(j.pool, j.svc)
		clusters, err := consolidator.FindClusters(ctx, scopeID)
		if err != nil {
			slog.Error("consolidate: find clusters failed", "scope_id", scopeID, "error", err)
			continue
		}
		for _, cluster := range clusters {
			if _, err := consolidator.MergeCluster(ctx, cluster, j.summarize); err != nil {
				slog.Error("consolidate: merge cluster failed", "scope_id", scopeID, "error", err)
				continue
			}
			totalMerged++
		}
	}

	slog.Info("consolidate: complete", "scopes_processed", len(scopeIDs), "clusters_merged", totalMerged)
	return nil
}

// defaultSummarizer is the fallback summarizer used when no LLM summarizer is
// provided. It concatenates contents with a separator — no external API call required.
func defaultSummarizer(_ context.Context, contents []string) (string, error) {
	return strings.Join(contents, "\n---\n"), nil
}
