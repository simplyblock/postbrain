package mcp

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/knowledge"
	"github.com/simplyblock/postbrain/internal/memory"
)

func (s *Server) registerContext() {
	s.mcpServer.AddTool(mcpgo.NewTool("context",
		mcpgo.WithReadOnlyHintAnnotation(true),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
		mcpgo.WithDescription("Retrieve a context bundle for the current scope and query"),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("query", mcpgo.Description("What you are about to work on")),
		mcpgo.WithNumber("max_tokens", mcpgo.Description("Token budget for context (default: 4000)")),
	), withToolMetrics("context", withToolPermission("memories:read", s.handleContext)))
}

// handleContext retrieves a structured context bundle for a new session.
func (s *Server) handleContext(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	scopeStr := argString(args, "scope")
	if scopeStr == "" {
		return mcpgo.NewToolResultError("context: 'scope' is required"), nil
	}

	query := argString(args, "query")
	maxTokens := argIntOrDefault(args, "max_tokens", 4000)

	if s.pool == nil {
		return mcpgo.NewToolResultError("context: server not configured"), nil
	}

	scopeID, errResult := s.resolveScope(ctx, "context", scopeStr)
	if errResult != nil {
		return errResult, nil
	}

	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)
	authorizedScopeIDs, err := s.effectiveScopeIDsForRequest(ctx)
	if err != nil {
		return mcpgo.NewToolResultError("context: scope authorization failed"), nil
	}

	type contextBlock struct {
		Layer                string `json:"layer"`
		Type                 string `json:"type,omitempty"`
		ArtifactKind         string `json:"artifact_kind,omitempty"`
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
			ScopeID: scopeID,
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
					ArtifactKind:         a.Artifact.ArtifactKind,
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
			Query:              query,
			ScopeID:            scopeID,
			PrincipalID:        principalID,
			AuthorizedScopeIDs: authorizedScopeIDs,
			SearchMode:         "hybrid",
			Limit:              50,
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
