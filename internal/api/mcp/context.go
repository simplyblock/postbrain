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

	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	type contextBlock struct {
		Layer   string `json:"layer"`
		Type    string `json:"type,omitempty"`
		Title   string `json:"title,omitempty"`
		Content string `json:"content"`
	}

	var blocks []contextBlock
	totalTokens := 0

	estimateTokens := func(text string) int { return len(text) / 4 }

	// Knowledge first (published artifacts relevant to query).
	if s.knwStore != nil {
		arts, err := s.knwStore.Recall(ctx, s.pool, knowledge.RecallInput{
			Query:    query,
			ScopeIDs: []uuid.UUID{scope.ID},
			Limit:    50,
		})
		if err == nil {
			for _, a := range arts {
				tokens := estimateTokens(a.Artifact.Content)
				if totalTokens+tokens > maxTokens {
					continue // skip if exceeds budget
				}
				blocks = append(blocks, contextBlock{
					Layer:   "knowledge",
					Type:    a.Artifact.KnowledgeType,
					Title:   a.Artifact.Title,
					Content: a.Artifact.Content,
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
				tokens := estimateTokens(m.Memory.Content)
				if totalTokens+tokens > maxTokens {
					continue
				}
				blocks = append(blocks, contextBlock{
					Layer:   "memory",
					Type:    m.Memory.MemoryType,
					Content: m.Memory.Content,
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
