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

func (s *Server) registerSkillInstall() {
	s.mcpServer.AddTool(mcpgo.NewTool("skill_install",
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithOpenWorldHintAnnotation(false),
		mcpgo.WithDescription("Materialise a skill into the agent command directory"),
		mcpgo.WithString("skill_id", mcpgo.Description("UUID of the skill to install")),
		mcpgo.WithString("slug", mcpgo.Description("Slug alternative to skill_id")),
		mcpgo.WithString("scope", mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("agent_type", mcpgo.Description("Target agent type")),
		mcpgo.WithString("workdir", mcpgo.Description("Working directory for installation")),
	), withToolMetrics("skill_install", withToolPermission("skills:read", s.handleSkillInstall)))
}

// handleSkillInstall materialises a skill into the agent command directory.
func (s *Server) handleSkillInstall(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	if s.pool == nil || s.sklStore == nil {
		return mcpgo.NewToolResultError("skill_install: server not configured"), nil
	}

	agentType := argString(args, "agent_type")
	if agentType == "" {
		agentType = "claude-code"
	}
	workdir := argString(args, "workdir")
	if workdir == "" {
		workdir = "."
	}

	// Get skill by ID or slug.
	var skill *db.Skill
	if idStr := argString(args, "skill_id"); idStr != "" {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("skill_install: invalid skill_id: %v", err)), nil
		}
		sk, err := s.sklStore.GetByID(ctx, id)
		if err != nil || sk == nil {
			return mcpgo.NewToolResultError("skill_install: skill not found"), nil
		}
		skill = sk
	} else if slug := argString(args, "slug"); slug != "" {
		scopeStr := argString(args, "scope")
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
				return scopeAuthzToolError(ctx, "skill_install", scope.ID, err), nil
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
