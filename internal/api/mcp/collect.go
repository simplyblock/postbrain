package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

// handleCollect dispatches collection actions.
func (s *Server) handleCollect(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	action, ok := args["action"].(string)
	if !ok || action == "" {
		return mcpgo.NewToolResultError("collect: 'action' is required"), nil
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("collect: server not configured"), nil
	}

	callerID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	switch action {
	case "add_to_collection":
		return s.collectAddToCollection(ctx, args, callerID)
	case "create_collection":
		return s.collectCreate(ctx, args, callerID)
	case "list_collections":
		return s.collectList(ctx, args)
	default:
		return mcpgo.NewToolResultError(fmt.Sprintf("collect: unknown action '%s'", action)), nil
	}
}

func (s *Server) collectAddToCollection(ctx context.Context, args map[string]any, callerID uuid.UUID) (*mcpgo.CallToolResult, error) {
	artifactIDStr, ok := args["artifact_id"].(string)
	if !ok || artifactIDStr == "" {
		return mcpgo.NewToolResultError("collect: 'artifact_id' is required for add_to_collection"), nil
	}
	artifactID, err := uuid.Parse(artifactIDStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("collect: invalid artifact_id: %v", err)), nil
	}

	// Resolve collection by ID or slug.
	var collectionID uuid.UUID
	if idStr, ok := args["collection_id"].(string); ok && idStr != "" {
		collectionID, err = uuid.Parse(idStr)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("collect: invalid collection_id: %v", err)), nil
		}
	} else if slug, ok := args["collection_slug"].(string); ok && slug != "" {
		scopeStr, _ := args["scope"].(string)
		if scopeStr == "" {
			return mcpgo.NewToolResultError("collect: 'scope' is required when using 'collection_slug'"), nil
		}
		kind, externalID, err := parseScopeString(scopeStr)
		if err != nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("collect: invalid scope: %v", err)), nil
		}
		scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
		if err != nil || scope == nil {
			return mcpgo.NewToolResultError("collect: scope not found"), nil
		}
		coll, err := s.knwColl.GetBySlug(ctx, scope.ID, slug)
		if err != nil || coll == nil {
			return mcpgo.NewToolResultError(fmt.Sprintf("collect: collection '%s' not found", slug)), nil
		}
		collectionID = coll.ID
	} else {
		return mcpgo.NewToolResultError("collect: 'collection_id' or 'collection_slug' required"), nil
	}

	if err := s.knwColl.AddItem(ctx, collectionID, artifactID, callerID); err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("collect: add item: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]any{
		"collection_id": collectionID.String(),
		"artifact_id":   artifactIDStr,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}

func (s *Server) collectCreate(ctx context.Context, args map[string]any, callerID uuid.UUID) (*mcpgo.CallToolResult, error) {
	scopeStr, ok := args["scope"].(string)
	if !ok || scopeStr == "" {
		return mcpgo.NewToolResultError("collect: 'scope' is required for create_collection"), nil
	}
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return mcpgo.NewToolResultError("collect: 'name' is required for create_collection"), nil
	}
	slug, _ := args["collection_slug"].(string)
	if slug == "" {
		return mcpgo.NewToolResultError("collect: 'collection_slug' is required for create_collection"), nil
	}

	visibility := "team"
	if v, ok := args["visibility"].(string); ok && v != "" {
		visibility = v
	}

	var description *string
	if d, ok := args["description"].(string); ok && d != "" {
		description = &d
	}

	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("collect: invalid scope: %v", err)), nil
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil || scope == nil {
		return mcpgo.NewToolResultError("collect: scope not found"), nil
	}

	coll, err := s.knwColl.Create(ctx, scope.ID, callerID, slug, name, visibility, description)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("collect: create: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]any{
		"collection_id": coll.ID.String(),
		"slug":          coll.Slug,
		"name":          coll.Name,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}

func (s *Server) collectList(ctx context.Context, args map[string]any) (*mcpgo.CallToolResult, error) {
	scopeStr, ok := args["scope"].(string)
	if !ok || scopeStr == "" {
		return mcpgo.NewToolResultError("collect: 'scope' is required for list_collections"), nil
	}

	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("collect: invalid scope: %v", err)), nil
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil || scope == nil {
		return mcpgo.NewToolResultError("collect: scope not found"), nil
	}

	colls, err := s.knwColl.List(ctx, scope.ID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("collect: list: %v", err)), nil
	}

	type collJSON struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
		Name string `json:"name"`
	}
	var out []collJSON
	for _, c := range colls {
		out = append(out, collJSON{ID: c.ID.String(), Slug: c.Slug, Name: c.Name})
	}
	payload, _ := json.Marshal(map[string]any{"collections": out})
	return mcpgo.NewToolResultText(string(payload)), nil
}
