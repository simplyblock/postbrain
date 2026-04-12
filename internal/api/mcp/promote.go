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

// handlePromote nominates a memory for elevation into a knowledge artifact.
func (s *Server) handlePromote(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	memIDStr := argString(args, "memory_id")
	if memIDStr == "" {
		return mcpgo.NewToolResultError("promote: 'memory_id' is required"), nil
	}
	memID, err := uuid.Parse(memIDStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("promote: invalid memory_id: %v", err)), nil
	}

	targetScopeStr := argString(args, "target_scope")
	if targetScopeStr == "" {
		return mcpgo.NewToolResultError("promote: 'target_scope' is required"), nil
	}

	targetVisibility := argString(args, "target_visibility")
	if targetVisibility == "" {
		return mcpgo.NewToolResultError("promote: 'target_visibility' is required"), nil
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("promote: server not configured"), nil
	}

	scopeID, errResult := s.resolveScope(ctx, "promote", targetScopeStr)
	if errResult != nil {
		return errResult, nil
	}

	requesterID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	var proposedTitle *string
	if v := argString(args, "proposed_title"); v != "" {
		proposedTitle = &v
	}

	// Optionally resolve collection by slug.
	var collectionID *uuid.UUID
	if collSlug := argString(args, "collection_slug"); collSlug != "" && s.knwColl != nil {
		coll, err := s.knwColl.GetBySlug(ctx, scopeID, collSlug)
		if err == nil && coll != nil {
			collectionID = &coll.ID
		}
	}

	promotionReq, err := s.knwProm.CreateRequest(ctx, knowledge.PromoteInput{
		MemoryID:             memID,
		RequestedBy:          requesterID,
		TargetScopeID:        scopeID,
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
