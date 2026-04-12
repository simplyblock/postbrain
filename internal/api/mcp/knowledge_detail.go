package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/db"
)

// handleKnowledgeDetail returns the full content of a knowledge artifact by ID.
// Use this after a recall result indicates full_content_available=true.
func (s *Server) handleKnowledgeDetail(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()
	idStr := argString(args, "artifact_id")
	if idStr == "" {
		return mcpgo.NewToolResultError("knowledge_detail: artifact_id is required"), nil
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("knowledge_detail: invalid artifact_id: %v", err)), nil
	}

	if s.knwStore == nil {
		return mcpgo.NewToolResultError("knowledge_detail: server not configured (no database connection)"), nil
	}

	artifact, err := s.knwStore.GetByID(ctx, id)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("knowledge_detail: %v", err)), nil
	}
	if artifact == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("knowledge_detail: artifact %s not found", idStr)), nil
	}

	go func() { _ = db.IncrementArtifactAccess(context.Background(), s.pool, artifact.ID) }()

	payload, _ := json.Marshal(map[string]any{
		"id":             artifact.ID.String(),
		"title":          artifact.Title,
		"content":        artifact.Content,
		"knowledge_type": artifact.KnowledgeType,
		"status":         artifact.Status,
		"visibility":     artifact.Visibility,
		"version":        artifact.Version,
	})
	return mcpgo.NewToolResultText(string(payload)), nil
}
