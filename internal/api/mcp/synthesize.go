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
)

// handleSynthesizeTopic synthesises multiple published knowledge artifacts into
// a single topic digest artifact.
func (s *Server) handleSynthesizeTopic(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	var token mcpgo.ProgressToken
	if req.Params.Meta != nil {
		token = req.Params.Meta.ProgressToken
	}
	report := func(progress, total float64, msg string) {
		if token == nil || s.mcpServer == nil {
			return
		}
		_ = s.mcpServer.SendNotificationToClient(ctx, "notifications/progress", map[string]any{
			"progressToken": token,
			"progress":      progress,
			"total":         total,
			"message":       msg,
		})
	}

	args := req.GetArguments()

	scopeStr, ok := args["scope"].(string)
	if !ok || scopeStr == "" {
		return mcpgo.NewToolResultError("synthesize_topic: 'scope' is required"), nil
	}

	// Parse source_ids array.
	rawIDs, ok := args["source_ids"].([]any)
	if !ok || len(rawIDs) < 2 {
		return mcpgo.NewToolResultError("synthesize_topic: 'source_ids' must contain at least 2 artifact IDs"), nil
	}
	sourceIDs := make([]uuid.UUID, 0, len(rawIDs))
	for _, v := range rawIDs {
		s, ok := v.(string)
		if !ok {
			return mcpgo.NewToolResultError("synthesize_topic: each source_id must be a string UUID"), nil
		}
		id, err := uuid.Parse(s)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("synthesize_topic: invalid source_id %q: %v", s, err)), nil
		}
		sourceIDs = append(sourceIDs, id)
	}

	title := ""
	if v, ok := args["title"].(string); ok {
		title = v
	}

	autoReview := false
	if v, ok := args["auto_review"].(bool); ok {
		autoReview = v
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("synthesize_topic: server not configured"), nil
	}

	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("synthesize_topic: invalid scope: %v", err)), nil
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("synthesize_topic: scope lookup: %v", err)), nil
	}
	if scope == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("synthesize_topic: scope '%s' not found", scopeStr)), nil
	}
	if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
		return scopeAuthzToolError(ctx, "synthesize_topic", scope.ID, err), nil
	}
	report(1, 2, "scope verified")

	authorID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	report(2, 2, fmt.Sprintf("synthesising %d artifacts", len(sourceIDs)))
	synth := knowledge.NewSynthesiser(s.pool, s.svc)
	artifact, err := synth.Create(ctx, knowledge.SynthesisInput{
		ScopeID:    scope.ID,
		AuthorID:   authorID,
		SourceIDs:  sourceIDs,
		Title:      title,
		AutoReview: autoReview,
	})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("synthesize_topic: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]any{
		"artifact_id":  artifact.ID.String(),
		"title":        artifact.Title,
		"status":       artifact.Status,
		"source_count": len(sourceIDs),
	})
	return mcpgo.NewToolResultText(string(out)), nil
}
