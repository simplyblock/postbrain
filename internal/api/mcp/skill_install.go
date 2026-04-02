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

// handleSkillInstall materialises a skill into the agent command directory.
func (s *Server) handleSkillInstall(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	if s.pool == nil || s.sklStore == nil {
		return mcpgo.NewToolResultError("skill_install: server not configured"), nil
	}

	agentType := "claude-code"
	if v, ok := args["agent_type"].(string); ok && v != "" {
		agentType = v
	}

	workdir := "."
	if v, ok := args["workdir"].(string); ok && v != "" {
		workdir = v
	}

	// Get skill by ID or slug.
	var skill *db.Skill
	if idStr, ok := args["skill_id"].(string); ok && idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("skill_install: invalid skill_id: %v", err)), nil
		}
		sk, err := s.sklStore.GetByID(ctx, id)
		if err != nil || sk == nil {
			return mcpgo.NewToolResultError("skill_install: skill not found"), nil
		}
		skill = sk
	} else if slug, ok := args["slug"].(string); ok && slug != "" {
		scopeStr, _ := args["scope"].(string)
		var scopeID uuid.UUID
		if scopeStr != "" {
			kind, externalID, err := parseScopeString(scopeStr)
			if err != nil {
				return mcpgo.NewToolResultError(fmt.Sprintf("skill_install: invalid scope: %v", err)), nil
			}
			scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
			if err != nil || scope == nil {
				return mcpgo.NewToolResultError("skill_install: scope not found"), nil
			}
			if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
				return scopeAuthzToolError(err), nil
			}
			scopeID = scope.ID
		}
		sk, err := s.sklStore.GetBySlug(ctx, scopeID, slug)
		if err != nil || sk == nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("skill_install: skill '%s' not found", slug)), nil
		}
		skill = sk
	} else {
		return mcpgo.NewToolResultError("skill_install: 'skill_id' or 'slug' is required"), nil
	}

	path, err := skills.Install(skill, agentType, workdir)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("skill_install: install failed: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]any{
		"path": path,
		"slug": skill.Slug,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}
