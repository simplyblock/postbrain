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

func (s *Server) registerPublish() {
	s.mcpServer.AddTool(mcpgo.NewTool("publish",
		mcpgo.WithReadOnlyHintAnnotation(false),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithOpenWorldHintAnnotation(false),
		mcpgo.WithDescription("Create or update a knowledge artifact"),
		mcpgo.WithString("title", mcpgo.Required(), mcpgo.Description("Artifact title")),
		mcpgo.WithString("content", mcpgo.Required(), mcpgo.Description("Artifact content")),
		mcpgo.WithString("knowledge_type", mcpgo.Required(), mcpgo.Description("semantic|episodic|procedural|reference")),
		mcpgo.WithString("artifact_kind", mcpgo.Description("general|decision|meeting_note|retrospective|spec|design_doc|research (default: general)")),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Owner scope as kind:external_id")),
		mcpgo.WithString("visibility", mcpgo.Description("private|project|team|department|company (default: team)")),
		mcpgo.WithString("summary", mcpgo.Description("Short summary")),
		mcpgo.WithBoolean("auto_review", mcpgo.Description("Move directly to in_review (default: false)")),
		mcpgo.WithString("collection_slug", mcpgo.Description("Add to this collection slug after creation")),
	), withToolMetrics("publish", withToolPermission("knowledge:write", s.handlePublish)))
}

// handlePublish creates a new knowledge artifact.
func (s *Server) handlePublish(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	title := argString(args, "title")
	if title == "" {
		return mcpgo.NewToolResultError("publish: 'title' is required"), nil
	}
	content := argString(args, "content")
	if content == "" {
		return mcpgo.NewToolResultError("publish: 'content' is required"), nil
	}
	knowledgeType := argString(args, "knowledge_type")
	if knowledgeType == "" {
		return mcpgo.NewToolResultError("publish: 'knowledge_type' is required"), nil
	}
	scopeStr := argString(args, "scope")
	if scopeStr == "" {
		return mcpgo.NewToolResultError("publish: 'scope' is required"), nil
	}

	visibility := argString(args, "visibility")
	if visibility == "" {
		visibility = "team"
	}

	var summary *string
	if v := argString(args, "summary"); v != "" {
		summary = &v
	}

	autoReview := argBool(args, "auto_review")
	artifactKind := argString(args, "artifact_kind")
	normalizedArtifactKind, err := knowledge.NormalizeArtifactKind(artifactKind)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("publish: invalid artifact_kind: %v", err)), nil
	}

	collectionSlug := argString(args, "collection_slug")

	if s.pool == nil {
		return mcpgo.NewToolResultError("publish: server not configured"), nil
	}

	scopeID, errResult := s.resolveScope(ctx, "publish", scopeStr)
	if errResult != nil {
		return errResult, nil
	}

	authorID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	artifact, err := s.knwStore.Create(ctx, knowledge.CreateInput{
		KnowledgeType: knowledgeType,
		ArtifactKind:  normalizedArtifactKind,
		OwnerScopeID:  scopeID,
		AuthorID:      authorID,
		Visibility:    visibility,
		Title:         title,
		Content:       content,
		Summary:       summary,
		AutoReview:    autoReview,
	})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("publish: create artifact: %v", err)), nil
	}

	// Optionally add to collection.
	if collectionSlug != "" && s.knwColl != nil {
		coll, err := s.knwColl.GetBySlug(ctx, scopeID, collectionSlug)
		if err == nil && coll != nil {
			_ = s.knwColl.AddItem(ctx, coll.ID, artifact.ID, authorID)
		}
	}

	out, _ := json.Marshal(map[string]any{
		"artifact_id": artifact.ID.String(),
		"status":      artifact.Status,
		"version":     artifact.Version,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}
