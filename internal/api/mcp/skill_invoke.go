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

// handleSkillInvoke looks up a skill by slug, substitutes params, and returns the expanded body.
func (s *Server) handleSkillInvoke(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	slug := argString(args, "slug")
	if slug == "" {
		return mcpgo.NewToolResultError("skill_invoke: 'slug' is required"), nil
	}
	scopeStr := argString(args, "scope")
	if scopeStr == "" {
		return mcpgo.NewToolResultError("skill_invoke: 'scope' is required"), nil
	}

	if s.pool == nil || s.sklStore == nil {
		return mcpgo.NewToolResultError("skill_invoke: server not configured"), nil
	}

	scopeID, errResult := s.resolveScope(ctx, "skill_invoke", scopeStr)
	if errResult != nil {
		return errResult, nil
	}

	skill, err := s.sklStore.GetBySlug(ctx, scopeID, slug)
	if err != nil || skill == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("skill_invoke: skill '%s' not found", slug)), nil
	}

	// Extract params map.
	params := map[string]any{}
	if v, ok := args["params"].(map[string]any); ok {
		params = v
	}

	body, err := skills.Invoke(skill, params)
	if err != nil {
		var ve *skills.ValidationError
		if ok := isValidationError(err, &ve); ok {
			return mcpgo.NewToolResultError(fmt.Sprintf("skill_invoke: validation: %v", err)), nil
		}
		return mcpgo.NewToolResultError(fmt.Sprintf("skill_invoke: %v", err)), nil
	}

	// Fire-and-forget invocation event; activates the skills_update_invocation_stats trigger.
	go func() {
		sessionID := uuid.Nil
		if sid, ok := args["session_id"].(string); ok && sid != "" {
			if parsed, err := uuid.Parse(sid); err == nil {
				sessionID = parsed
			}
		}
		payload, _ := json.Marshal(map[string]any{"skill_id": skill.ID.String()})
		_ = db.InsertEvent(context.Background(), s.pool, sessionID, scopeID, "skill_invoked", payload)
	}()

	out, _ := json.Marshal(map[string]any{
		"skill_id": skill.ID.String(),
		"slug":     skill.Slug,
		"body":     body,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}

// isValidationError checks if err is a *skills.ValidationError and sets the target pointer.
func isValidationError(err error, target **skills.ValidationError) bool {
	if ve, ok := err.(*skills.ValidationError); ok {
		*target = ve
		return true
	}
	return false
}
