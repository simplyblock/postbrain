package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
)

// handleContext retrieves a structured context bundle for a new session.
func (s *Server) handleContext(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	scopeStr, ok := args["scope"].(string)
	if !ok || scopeStr == "" {
		return mcpgo.NewToolResultError("context: 'scope' is required"), nil
	}

	query := ""
	if v, ok := args["query"].(string); ok {
		query = v
	}

	maxTokens := 4000
	if v, ok := args["max_tokens"].(float64); ok && v > 0 {
		maxTokens = int(v)
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("context: server not configured"), nil
	}

	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("context: invalid scope: %v", err)), nil
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("context: scope lookup: %v", err)), nil
	}
	if scope == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("context: scope '%s' not found", scopeStr)), nil
	}
	if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
		return scopeAuthzToolError(err), nil
	}

	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	type contextBlock struct {
		Layer                string `json:"layer"`
		Type                 string `json:"type,omitempty"`
		Title                string `json:"title,omitempty"`
		Content              string `json:"content"`
		ID                   string `json:"id,omitempty"`
		FullContentAvailable bool   `json:"full_content_available,omitempty"`
	}

	var blocks []contextBlock
	totalTokens := 0

	estimateTokens := func(text string) int { return len(text) / 4 }

	// Knowledge first (published artifacts relevant to query).
	if s.knwStore != nil {
		arts, err := s.knwStore.Recall(ctx, s.pool, knowledge.RecallInput{
			Query:   query,
			ScopeID: scope.ID,
			Limit:   50,
		})
		if err == nil {
			for _, a := range arts {
				content := a.Artifact.Content
				fullContentAvailable := false
				// Prefer summary if available.
				if a.Artifact.Summary != nil && *a.Artifact.Summary != "" {
					content = *a.Artifact.Summary
					fullContentAvailable = true
				}
				tokens := estimateTokens(content)
				if totalTokens+tokens > maxTokens {
					// Budget exhausted: include a one-line stub so the agent
					// knows the artifact exists and can fetch it via knowledge_detail.
					if a.Artifact.Summary != nil && *a.Artifact.Summary != "" {
						content = *a.Artifact.Summary
					} else {
						// Extractive: first sentence / 120 chars.
						content = extractLead(a.Artifact.Content, 120)
					}
					fullContentAvailable = true
					tokens = estimateTokens(content)
				}
				blocks = append(blocks, contextBlock{
					Layer:                "knowledge",
					Type:                 a.Artifact.KnowledgeType,
					Title:                a.Artifact.Title,
					Content:              content,
					ID:                   a.Artifact.ID.String(),
					FullContentAvailable: fullContentAvailable,
				})
				totalTokens += tokens
			}
		}
	}

	// Memories (relevant to query).
	if s.memStore != nil {
		mems, err := s.memStore.Recall(ctx, memory.RecallInput{
			Query:       query,
			ScopeID:     scope.ID,
			PrincipalID: principalID,
			SearchMode:  "hybrid",
			Limit:       50,
		})
		if err == nil {
			for _, m := range mems {
				content := m.Memory.Content
				fullContentAvailable := false
				tokens := estimateTokens(content)
				if totalTokens+tokens > maxTokens {
					// Budget exhausted: include a lead stub so the agent
					// knows the memory exists.
					content = extractLead(m.Memory.Content, 120)
					fullContentAvailable = true
					tokens = estimateTokens(content)
				}
				blocks = append(blocks, contextBlock{
					Layer:                "memory",
					Type:                 m.Memory.MemoryType,
					Content:              content,
					ID:                   m.Memory.ID.String(),
					FullContentAvailable: fullContentAvailable,
				})
				totalTokens += tokens
			}
		}
	}

	payload, _ := json.Marshal(map[string]any{
		"context_blocks": blocks,
		"total_tokens":   totalTokens,
	})
	return mcpgo.NewToolResultText(string(payload)), nil
}

// extractLead returns the first sentence of text, capped at maxChars.
// Used to produce a stub when the token budget is exhausted.
func extractLead(text string, maxChars int) string {
	for i, ch := range text {
		if ch == '.' || ch == '\n' {
			if i+1 <= maxChars {
				return text[:i+1]
			}
			break
		}
	}
	if len(text) <= maxChars {
		return text
	}
	return text[:maxChars] + "…"
}
