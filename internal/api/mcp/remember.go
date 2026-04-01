package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/memory"
)

// handleRemember stores a new memory or updates a near-duplicate.
func (s *Server) handleRemember(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	// Required: content
	content, ok := args["content"].(string)
	if !ok || content == "" {
		return mcpgo.NewToolResultError("remember: 'content' is required and must be a non-empty string"), nil
	}

	// Required: scope
	scopeStr, ok := args["scope"].(string)
	if !ok || scopeStr == "" {
		return mcpgo.NewToolResultError("remember: 'scope' is required"), nil
	}

	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("remember: invalid scope: %v", err)), nil
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("remember: server not configured (no database connection)"), nil
	}

	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("remember: scope lookup failed: %v", err)), nil
	}
	if scope == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("remember: scope '%s' not found", scopeStr)), nil
	}

	// Get principal from context.
	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	// Optional fields with defaults.
	memoryType := "semantic"
	if v, ok := args["memory_type"].(string); ok && v != "" {
		memoryType = v
	}

	importance := 0.5
	if v, ok := args["importance"].(float64); ok {
		importance = v
	}

	var sourceRef *string
	if v, ok := args["source_ref"].(string); ok && v != "" {
		sourceRef = &v
	}
	var summary *string
	if v, ok := args["summary"].(string); ok && v != "" {
		summary = &v
	}

	var entities []memory.EntityInput
	if v, ok := args["entities"].([]any); ok {
		for _, e := range v {
			switch val := e.(type) {
			case map[string]any:
				name, _ := val["name"].(string)
				typ, _ := val["type"].(string)
				if name != "" {
					entities = append(entities, memory.EntityInput{Name: name, Type: typ})
				}
			case string:
				if val != "" {
					entities = append(entities, memory.EntityInput{Name: val})
				}
			}
		}
	}

	var expiresIn *int
	if v, ok := args["expires_in"].(float64); ok {
		n := int(v)
		expiresIn = &n
	}

	input := memory.CreateInput{
		Content:    content,
		Summary:    summary,
		MemoryType: memoryType,
		ScopeID:    scope.ID,
		AuthorID:   principalID,
		Importance: importance,
		SourceRef:  sourceRef,
		Entities:   entities,
		ExpiresIn:  expiresIn,
	}

	result, err := s.memStore.Create(ctx, input)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("remember: store failed: %v", err)), nil
	}

	out, _ := json.Marshal(map[string]any{
		"memory_id": result.MemoryID.String(),
		"action":    result.Action,
	})
	return mcpgo.NewToolResultText(string(out)), nil
}
