package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/retrieval"
)

const defaultRecallGraphDepth = 1

func parseGraphDepth(args map[string]any) int {
	graphDepth := defaultRecallGraphDepth
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

// handleRecall retrieves memories and knowledge relevant to a query.
func (s *Server) handleRecall(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query, _ := args["query"].(string)
	if query == "" {
		return mcpgo.NewToolResultError("recall: 'query' is required"), nil
	}
	scopeStr, _ := args["scope"].(string)

	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	minScore := 0.0
	if v, ok := args["min_score"].(float64); ok {
		minScore = v
	}

	searchMode := "hybrid"
	if v, ok := args["search_mode"].(string); ok && v != "" {
		searchMode = v
	}

	agentType := ""
	if v, ok := args["agent_type"].(string); ok {
		agentType = v
	}

	graphDepth := parseGraphDepth(args)

	var memoryTypes []string
	if v, ok := args["memory_types"].([]any); ok {
		for _, mt := range v {
			if ms, ok := mt.(string); ok {
				memoryTypes = append(memoryTypes, ms)
			}
		}
	}

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
		scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
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
