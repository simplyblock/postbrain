package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/retrieval"
)

func (s *Server) registerRecall() {
	s.mcpServer.AddTool(mcpgo.NewTool("recall",
		mcpgo.WithReadOnlyHintAnnotation(true),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
		mcpgo.WithDescription("Retrieve memories and knowledge relevant to a query"),
		mcpgo.WithString("query", mcpgo.Required(), mcpgo.Description("Semantic search query")),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithArray("memory_types",
			mcpgo.WithStringEnumItems([]string{"semantic", "episodic", "procedural", "working"}),
			mcpgo.Description("Filter by memory type: semantic|episodic|procedural|working"),
		),
		mcpgo.WithArray("layers",
			mcpgo.WithStringEnumItems([]string{"memory", "knowledge", "skill"}),
			mcpgo.Description("Layers to query: memory|knowledge|skill (default: all)"),
		),
		mcpgo.WithString("agent_type", mcpgo.Description("Filter skills by agent compatibility")),
		mcpgo.WithNumber("limit", mcpgo.Description("Max results (default: 10)")),
		mcpgo.WithNumber("min_score", mcpgo.Description("Min combined score 0–1 (default: 0.0)")),
		mcpgo.WithString("search_mode", mcpgo.Description("text|code|hybrid (default: hybrid)")),
		mcpgo.WithNumber("graph_depth", mcpgo.Description("Graph traversal depth for code results: 0=off, 1=direct neighbours (default: 1)")),
	), withToolMetrics("recall", withToolPermission("memories:read", s.handleRecall)))
}

const defaultRecallGraphDepth = 1
const defaultCrossScopeGraphDepth = 0

func parseGraphDepthWithDefault(args map[string]any, defaultDepth int) int {
	graphDepth := defaultDepth
	if v, ok := args["graph_depth"].(float64); ok {
		graphDepth = int(v)
		if graphDepth < 0 {
			graphDepth = 0
		}
	}
	if graphDepth > 2 {
		graphDepth = 2 // cap to avoid fanout explosion
	}
	return graphDepth
}

func parseGraphDepth(args map[string]any) int {
	return parseGraphDepthWithDefault(args, defaultRecallGraphDepth)
}

// handleRecall retrieves memories and knowledge relevant to a query.
func (s *Server) handleRecall(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query := argString(args, "query")
	if query == "" {
		return mcpgo.NewToolResultError("recall: 'query' is required"), nil
	}
	scopeStr := argString(args, "scope")
	limit := argIntOrDefault(args, "limit", 10)
	minScore := argFloat64OrDefault(args, "min_score", 0.0)
	searchMode := argString(args, "search_mode")
	if searchMode == "" {
		searchMode = "hybrid"
	}
	agentType := argString(args, "agent_type")
	graphDepth := parseGraphDepth(args)
	memoryTypes := argStringSlice(args, "memory_types")

	// Parse layers (default: all three).
	activeLayers := map[retrieval.Layer]bool{
		retrieval.LayerMemory:    true,
		retrieval.LayerKnowledge: true,
		retrieval.LayerSkill:     true,
	}
	if v, ok := args["layers"].([]any); ok && len(v) > 0 {
		activeLayers = map[retrieval.Layer]bool{}
		for _, l := range v {
			if ls, ok := l.(string); ok {
				activeLayers[retrieval.Layer(ls)] = true
			}
		}
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("recall: server not configured (no database connection)"), nil
	}

	// Resolve scope.
	var scopeID uuid.UUID
	if scopeStr != "" {
		kind, externalID, err := parseScopeString(scopeStr)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: invalid scope: %v", err)), nil
		}
		scope, err := compat.GetScopeByExternalID(ctx, s.pool, kind, externalID)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: scope lookup: %v", err)), nil
		}
		if scope == nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("recall: scope '%s' not found", scopeStr)), nil
		}
		if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
			return scopeAuthzToolError(ctx, "recall", scope.ID, err), nil
		}
		scopeID = scope.ID
	}

	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	authorizedScopeIDs, err := s.effectiveScopeIDsForRequest(ctx)
	if err != nil {
		return mcpgo.NewToolResultError("recall: scope authorization failed"), nil
	}

	merged, err := retrieval.OrchestrateRecall(ctx, retrieval.OrchestrateDeps{
		Pool:     s.pool,
		MemStore: s.memStore,
		KnwStore: s.knwStore,
		SklStore: s.sklStore,
		Svc:      s.svc,
	}, retrieval.OrchestrateInput{
		Query:              query,
		ScopeID:            scopeID,
		PrincipalID:        principalID,
		AuthorizedScopeIDs: authorizedScopeIDs,
		MemoryTypes:        memoryTypes,
		SearchMode:         searchMode,
		AgentType:          agentType,
		Limit:              limit,
		MinScore:           minScore,
		GraphDepth:         graphDepth,
		ActiveLayers:       activeLayers,
	})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("recall: %v", err)), nil
	}

	type resultJSON struct {
		Layer                string   `json:"layer"`
		ID                   string   `json:"id"`
		Score                float64  `json:"score"`
		Content              string   `json:"content,omitempty"`
		Title                string   `json:"title,omitempty"`
		MemoryType           string   `json:"memory_type,omitempty"`
		KnowledgeType        string   `json:"knowledge_type,omitempty"`
		ArtifactKind         string   `json:"artifact_kind,omitempty"`
		SourceRef            string   `json:"source_ref,omitempty"`
		Visibility           string   `json:"visibility,omitempty"`
		Status               string   `json:"status,omitempty"`
		Endorsements         int      `json:"endorsements,omitempty"`
		FullContentAvailable bool     `json:"full_content_available,omitempty"`
		Slug                 string   `json:"slug,omitempty"`
		Name                 string   `json:"name,omitempty"`
		Description          string   `json:"description,omitempty"`
		AgentTypes           []string `json:"agent_types,omitempty"`
		InvocationCount      int      `json:"invocation_count,omitempty"`
		Installed            bool     `json:"installed,omitempty"`
		GraphContext         bool     `json:"graph_context,omitempty"`
	}

	out := make([]resultJSON, 0, len(merged))
	for _, r := range merged {
		out = append(out, resultJSON{
			Layer:                string(r.Layer),
			ID:                   r.ID.String(),
			Score:                r.Score,
			Content:              r.Content,
			Title:                r.Title,
			MemoryType:           r.MemoryType,
			KnowledgeType:        r.KnowledgeType,
			ArtifactKind:         r.ArtifactKind,
			SourceRef:            r.SourceRef,
			Visibility:           r.Visibility,
			Status:               r.Status,
			Endorsements:         r.Endorsements,
			FullContentAvailable: r.FullContentAvailable,
			Slug:                 r.Slug,
			Name:                 r.Name,
			Description:          r.Description,
			AgentTypes:           r.AgentTypes,
			InvocationCount:      r.InvocationCount,
			Installed:            r.Installed,
			GraphContext:         r.GraphContext,
		})
	}

	payload, _ := json.Marshal(map[string]any{"results": out})
	return mcpgo.NewToolResultText(string(payload)), nil
}
