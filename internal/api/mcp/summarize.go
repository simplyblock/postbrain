package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/memory"
)

// handleSummarize consolidates memories for a scope/topic, or previews the plan.
func (s *Server) handleSummarize(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	report := s.progressReporter(ctx, req)

	args := req.GetArguments()

	scopeStr := argString(args, "scope")
	if scopeStr == "" {
		return mcpgo.NewToolResultError("summarize: 'scope' is required"), nil
	}

	topic := argString(args, "topic")
	dryRun := argBool(args, "dry_run")

	if s.pool == nil {
		return mcpgo.NewToolResultError("summarize: server not configured"), nil
	}

	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("summarize: invalid scope: %v", err)), nil
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("summarize: scope lookup: %v", err)), nil
	}
	if scope == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("summarize: scope '%s' not found", scopeStr)), nil
	}
	if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
		return scopeAuthzToolError(ctx, "summarize", scope.ID, err), nil
	}
	report(1, 3, "scope verified")

	consolidator := memory.NewConsolidator(s.pool, s.svc)
	clusters, err := consolidator.FindClusters(ctx, scope.ID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("summarize: find clusters: %v", err)), nil
	}
	report(2, 3, fmt.Sprintf("found %d clusters", len(clusters)))

	if dryRun {
		var wouldConsolidate []string
		var contentSnippets []string
		for _, cluster := range clusters {
			for _, m := range cluster {
				wouldConsolidate = append(wouldConsolidate, m.ID.String())
				if topic == "" || strings.Contains(strings.ToLower(m.Content), strings.ToLower(topic)) {
					contentSnippets = append(contentSnippets, m.Content)
				}
			}
		}
		proposed := "No clusters found"
		if len(contentSnippets) > 0 {
			proposed = fmt.Sprintf("Proposed summary of %d memories about %q", len(contentSnippets), topic)
		}
		out, _ := json.Marshal(map[string]any{
			"would_consolidate": wouldConsolidate,
			"proposed_summary":  proposed,
		})
		return mcpgo.NewToolResultText(string(out)), nil
	}

	// Run actual consolidation.
	// Simple summarizer: join all contents.
	// TODO(task-summarize): replace with LLM-based summarization once an LLM client is added.
	summarizer := func(_ context.Context, contents []string) (string, error) {
		return strings.Join(contents, "\n---\n"), nil
	}

	var totalConsolidated int
	var lastResultID uuid.UUID
	var lastSummary string

	for _, cluster := range clusters {
		if len(cluster) < 2 {
			continue
		}
		result, err := consolidator.MergeCluster(ctx, cluster, summarizer)
		if err != nil {
			continue // log and skip failing clusters; don't abort
		}
		totalConsolidated += len(cluster)
		lastResultID = result.ID
		lastSummary = result.Content
	}

	report(3, 3, "consolidation complete")
	out, _ := json.Marshal(map[string]any{
		"consolidated_count": totalConsolidated,
		"result_memory_id":   lastResultID.String(),
		"summary":            lastSummary,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}
