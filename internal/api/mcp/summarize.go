package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/memory"
)

func (s *Server) registerSummarize() {
	s.mcpServer.AddTool(mcpgo.NewTool("summarize",
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithOpenWorldHintAnnotation(true),
		mcpgo.WithDescription("Consolidate memories into a higher-level semantic memory"),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("topic", mcpgo.Description("Topic to cluster and summarize")),
		mcpgo.WithBoolean("dry_run", mcpgo.Description("If true, preview without writing (default: false)")),
	), withToolMetrics("summarize", withToolPermission("memories:write", s.handleSummarize)))
}

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

	scopeID, errResult := s.resolveScope(ctx, "summarize", scopeStr)
	if errResult != nil {
		return errResult, nil
	}
	report(1, 3, "scope verified")

	consolidator := memory.NewConsolidator(s.pool, s.svc)
	clusters, err := consolidator.FindClusters(ctx, scopeID)
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

	// Run actual consolidation using the configured summarization model.
	summarizer := func(ctx context.Context, contents []string) (string, error) {
		joined := strings.Join(contents, "\n\n")
		if s.svc == nil {
			return joined, nil
		}
		summary, err := s.svc.Summarize(ctx, joined)
		if err != nil {
			return joined, nil // fall back to joined text rather than failing the cluster
		}
		return summary, nil
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
