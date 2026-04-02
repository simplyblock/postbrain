package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/skills"
)

// handleSkillSearch searches for skills by semantic similarity.
func (s *Server) handleSkillSearch(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return mcpgo.NewToolResultError("skill_search: 'query' is required"), nil
	}

	if s.pool == nil || s.sklStore == nil || s.svc == nil {
		return mcpgo.NewToolResultError("skill_search: server not configured"), nil
	}

	limit := 10
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	agentType := ""
	if v, ok := args["agent_type"].(string); ok {
		agentType = v
	}

	// Resolve scope if provided.
	var scopeIDs []uuid.UUID
	if scopeStr, ok := args["scope"].(string); ok && scopeStr != "" {
		kind, externalID, err := parseScopeString(scopeStr)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("skill_search: invalid scope: %v", err)), nil
		}
		scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
		if err != nil || scope == nil {
			return mcpgo.NewToolResultError("skill_search: scope not found"), nil
		}
		if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
			return scopeAuthzToolError(ctx, "skill_search", scope.ID, err), nil
		}
		scopeIDs = []uuid.UUID{scope.ID}
	}

	var installedFilter *bool
	if v, ok := args["installed"].(bool); ok {
		installedFilter = &v
	}

	results, err := s.sklStore.Recall(ctx, s.svc, skills.RecallInput{
		Query:     query,
		ScopeIDs:  scopeIDs,
		AgentType: agentType,
		Limit:     limit,
		Installed: installedFilter,
	})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("skill_search: recall: %v", err)), nil
	}

	type skillJSON struct {
		ID              string   `json:"id"`
		Slug            string   `json:"slug"`
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		Score           float64  `json:"score"`
		AgentTypes      []string `json:"agent_types"`
		Visibility      string   `json:"visibility"`
		Status          string   `json:"status"`
		InvocationCount int      `json:"invocation_count"`
		Installed       bool     `json:"installed"`
		Layer           string   `json:"layer"`
	}

	var out []skillJSON
	for _, r := range results {
		out = append(out, skillJSON{
			ID:              r.Skill.ID.String(),
			Slug:            r.Skill.Slug,
			Name:            r.Skill.Name,
			Description:     r.Skill.Description,
			Score:           r.Score,
			AgentTypes:      r.Skill.AgentTypes,
			Visibility:      r.Skill.Visibility,
			Status:          r.Skill.Status,
			InvocationCount: int(r.Skill.InvocationCount),
			Installed:       r.Installed,
			Layer:           "skill",
		})
	}

	payload, _ := json.Marshal(map[string]any{"results": out})
	return mcpgo.NewToolResultText(string(payload)), nil
}
