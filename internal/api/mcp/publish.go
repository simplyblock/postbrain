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

// handlePublish creates a new knowledge artifact.
func (s *Server) handlePublish(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	title, ok := args["title"].(string)
	if !ok || title == "" {
		return mcpgo.NewToolResultError("publish: 'title' is required"), nil
	}
	content, ok := args["content"].(string)
	if !ok || content == "" {
		return mcpgo.NewToolResultError("publish: 'content' is required"), nil
	}
	knowledgeType, ok := args["knowledge_type"].(string)
	if !ok || knowledgeType == "" {
		return mcpgo.NewToolResultError("publish: 'knowledge_type' is required"), nil
	}
	scopeStr, ok := args["scope"].(string)
	if !ok || scopeStr == "" {
		return mcpgo.NewToolResultError("publish: 'scope' is required"), nil
	}

	visibility := "team"
	if v, ok := args["visibility"].(string); ok && v != "" {
		visibility = v
	}

	var summary *string
	if v, ok := args["summary"].(string); ok && v != "" {
		summary = &v
	}

	autoReview := false
	if v, ok := args["auto_review"].(bool); ok {
		autoReview = v
	}

	collectionSlug := ""
	if v, ok := args["collection_slug"].(string); ok {
		collectionSlug = v
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("publish: server not configured"), nil
	}

	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("publish: invalid scope: %v", err)), nil
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("publish: scope lookup: %v", err)), nil
	}
	if scope == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("publish: scope '%s' not found", scopeStr)), nil
	}
	if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
		return scopeAuthzToolError(err), nil
	}

	authorID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	artifact, err := s.knwStore.Create(ctx, knowledge.CreateInput{
		KnowledgeType: knowledgeType,
		OwnerScopeID:  scope.ID,
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
		coll, err := s.knwColl.GetBySlug(ctx, scope.ID, collectionSlug)
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
