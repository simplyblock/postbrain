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

// handlePromote nominates a memory for elevation into a knowledge artifact.
func (s *Server) handlePromote(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	memIDStr, ok := args["memory_id"].(string)
	if !ok || memIDStr == "" {
		return mcpgo.NewToolResultError("promote: 'memory_id' is required"), nil
	}
	memID, err := uuid.Parse(memIDStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("promote: invalid memory_id: %v", err)), nil
	}

	targetScopeStr, ok := args["target_scope"].(string)
	if !ok || targetScopeStr == "" {
		return mcpgo.NewToolResultError("promote: 'target_scope' is required"), nil
	}

	targetVisibility, ok := args["target_visibility"].(string)
	if !ok || targetVisibility == "" {
		return mcpgo.NewToolResultError("promote: 'target_visibility' is required"), nil
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("promote: server not configured"), nil
	}

	kind, externalID, err := parseScopeString(targetScopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("promote: invalid target_scope: %v", err)), nil
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("promote: scope lookup: %v", err)), nil
	}
	if scope == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("promote: scope '%s' not found", targetScopeStr)), nil
	}

	requesterID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	var proposedTitle *string
	if v, ok := args["proposed_title"].(string); ok && v != "" {
		proposedTitle = &v
	}

	// Optionally resolve collection by slug.
	var collectionID *uuid.UUID
	if collSlug, ok := args["collection_slug"].(string); ok && collSlug != "" && s.knwColl != nil {
		coll, err := s.knwColl.GetBySlug(ctx, scope.ID, collSlug)
		if err == nil && coll != nil {
			collectionID = &coll.ID
		}
	}

	promotionReq, err := s.knwProm.CreateRequest(ctx, knowledge.PromoteInput{
		MemoryID:             memID,
		RequestedBy:          requesterID,
		TargetScopeID:        scope.ID,
		TargetVisibility:     targetVisibility,
		ProposedTitle:        proposedTitle,
		ProposedCollectionID: collectionID,
	})
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("promote: create request: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]any{
		"promotion_request_id": promotionReq.ID.String(),
		"status":               promotionReq.Status,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}
