package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
)

func (s *Server) registerEndorse() {
	s.mcpServer.AddTool(mcpgo.NewTool("endorse",
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithOpenWorldHintAnnotation(false),
		mcpgo.WithDescription("Endorse a knowledge artifact or skill"),
		mcpgo.WithString("artifact_id", mcpgo.Required(), mcpgo.Description("UUID of the artifact or skill to endorse")),
		mcpgo.WithString("note", mcpgo.Description("Optional endorsement note")),
	), withToolMetrics("endorse", withAnyToolPermission([]authz.Permission{"knowledge:write", "skills:write"}, s.handleEndorse)))
}

// handleEndorse endorses a knowledge artifact or skill.
func (s *Server) handleEndorse(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	artifactIDStr := argString(args, "artifact_id")
	if artifactIDStr == "" {
		return mcpgo.NewToolResultError("endorse: 'artifact_id' is required"), nil
	}

	artifactID, err := uuid.Parse(artifactIDStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("endorse: invalid artifact_id: %v", err)), nil
	}

	var note *string
	if v := argString(args, "note"); v != "" {
		note = &v
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("endorse: server not configured"), nil
	}

	endorserID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	// Try knowledge lifecycle first.
	if s.knwLife != nil {
		result, err := s.knwLife.Endorse(ctx, artifactID, endorserID, note)
		if err == nil {
			out, _ := json.Marshal(map[string]any{
				"artifact_id":       artifactIDStr,
				"endorsement_count": result.EndorsementCount,
				"status":            result.Status,
				"auto_published":    result.AutoPublished,
			})
			return mcpgo.NewToolResultText(string(out)), nil
		}
		// If artifact not found in knowledge, fall through to skills.
	}

	// Try skill lifecycle.
	if s.sklLife != nil {
		result, err := s.sklLife.Endorse(ctx, artifactID, endorserID, note)
		if err == nil {
			out, _ := json.Marshal(map[string]any{
				"artifact_id":       artifactIDStr,
				"endorsement_count": result.EndorsementCount,
				"status":            result.Status,
				"auto_published":    result.AutoPublished,
			})
			return mcpgo.NewToolResultText(string(out)), nil
		}
		return mcpgo.NewToolResultError(fmt.Sprintf("endorse: skill endorse failed: %v", err)), nil
	}

	return mcpgo.NewToolResultError("endorse: artifact not found in knowledge or skills"), nil
}
