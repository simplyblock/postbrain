package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/knowledge"
)

func (s *Server) registerSynthesizeTopic() {
	s.mcpServer.AddTool(mcpgo.NewTool("synthesize_topic",
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithOpenWorldHintAnnotation(true),
		mcpgo.WithDescription("Synthesise multiple published knowledge artifacts into a single topic digest artifact"),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Owner scope as kind:external_id")),
		mcpgo.WithArray("source_ids", mcpgo.Required(), mcpgo.Description("UUIDs of the source artifacts to synthesise (minimum 2, all must be published non-digest artifacts)")),
		mcpgo.WithString("title", mcpgo.Description("Digest title; inferred from sources if omitted")),
		mcpgo.WithBoolean("auto_review", mcpgo.Description("Move directly to in_review (default: false)")),
	), withToolMetrics("synthesize_topic", withToolPermission("knowledge:write", s.handleSynthesizeTopic)))
}

// handleSynthesizeTopic synthesises multiple published knowledge artifacts into
// a single topic digest artifact.
func (s *Server) handleSynthesizeTopic(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	report := s.progressReporter(ctx, req)

	args := req.GetArguments()

	scopeStr := argString(args, "scope")
	if scopeStr == "" {
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

	title := argString(args, "title")
	autoReview := argBool(args, "auto_review")

	if s.pool == nil {
		return mcpgo.NewToolResultError("synthesize_topic: server not configured"), nil
	}

	scopeID, errResult := s.resolveScope(ctx, "synthesize_topic", scopeStr)
	if errResult != nil {
		return errResult, nil
	}
	report(1, 2, "scope verified")

	authorID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	report(2, 2, fmt.Sprintf("synthesising %d artifacts", len(sourceIDs)))
	synth := knowledge.NewSynthesiser(s.pool, s.svc)
	artifact, err := synth.Create(ctx, knowledge.SynthesisInput{
		ScopeID:    scopeID,
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
