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
	args := req.GetArguments()

	scopeStr, ok := args["scope"].(string)
	if !ok || scopeStr == "" {
		return mcpgo.NewToolResultError("summarize: 'scope' is required"), nil
	}

	topic := ""
	if v, ok := args["topic"].(string); ok {
		topic = v
	}

	dryRun := false
	if v, ok := args["dry_run"].(bool); ok {
		dryRun = v
	}

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
		return scopeAuthzToolError(err), nil
	}

	consolidator := memory.NewConsolidator(s.pool, s.svc)
	clusters, err := consolidator.FindClusters(ctx, scope.ID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("summarize: find clusters: %v", err)), nil
	}

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

	out, _ := json.Marshal(map[string]any{
		"consolidated_count": totalConsolidated,
		"result_memory_id":   lastResultID.String(),
		"summary":            lastSummary,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}
