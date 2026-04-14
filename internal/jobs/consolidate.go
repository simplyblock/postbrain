package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/providers"
)

// maxClustersPerScope is the maximum number of clusters the background job
// will merge per scope per run. This bounds the number of LLM summarization
// calls to at most maxClustersPerScope × (number of scopes) per invocation.
// Unmerged clusters are picked up on the next scheduled run.
const maxClustersPerScope = 10

// ConsolidateJob runs LLM-assisted memory consolidation for all scopes.
type ConsolidateJob struct {
	pool      *pgxpool.Pool
	svc       *providers.EmbeddingService
	summarize func(ctx context.Context, contents []string) (string, error)
}

// NewConsolidateJob creates a new ConsolidateJob. If summarize is nil and svc
// is non-nil, the configured LLM summarizer is used. If both are nil, the
// fallback joins contents with a separator (no external API call).
func NewConsolidateJob(pool *pgxpool.Pool, svc *providers.EmbeddingService, summarize func(ctx context.Context, contents []string) (string, error)) *ConsolidateJob {
	if summarize == nil {
		if svc != nil {
			summarize = llmSummarizer(svc)
		} else {
			summarize = defaultSummarizer
		}
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
		consolidator.MaxClusters = maxClustersPerScope
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

// llmSummarizer returns a summarizer that calls svc.Summarize on the joined
// content, falling back to the plain join if the LLM call fails.
func llmSummarizer(svc *providers.EmbeddingService) func(context.Context, []string) (string, error) {
	return func(ctx context.Context, contents []string) (string, error) {
		joined := strings.Join(contents, "\n\n")
		summary, err := svc.Summarize(ctx, joined)
		if err != nil {
			return joined, nil
		}
		return summary, nil
	}
}

// defaultSummarizer is the fallback summarizer used when no LLM is available.
// It concatenates contents with a separator — no external API call required.
func defaultSummarizer(_ context.Context, contents []string) (string, error) {
	return strings.Join(contents, "\n---\n"), nil
}
