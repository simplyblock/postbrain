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

// handleSessionBegin creates a new session for the given scope and returns its ID.
// The returned session_id should be passed to skill_invoke and other tools that
// emit events, so that all activity within a logical agent session is correlated.
func (s *Server) handleSessionBegin(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	scopeStr, ok := args["scope"].(string)
	if !ok || scopeStr == "" {
		return mcpgo.NewToolResultError("session_begin: 'scope' is required"), nil
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("session_begin: server not configured (no database connection)"), nil
	}

	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("session_begin: invalid scope: %v", err)), nil
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("session_begin: scope lookup: %v", err)), nil
	}
	if scope == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("session_begin: scope '%s' not found", scopeStr)), nil
	}

	principalID, _ := ctx.Value(auth.ContextKeyPrincipalID).(uuid.UUID)

	session, err := db.CreateSession(ctx, s.pool, scope.ID, principalID, nil)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("session_begin: %v", err)), nil
	}

	payload, _ := json.Marshal(map[string]any{
		"session_id": session.ID.String(),
		"scope":      scopeStr,
		"started_at": session.StartedAt,
	})
	return mcpgo.NewToolResultText(string(payload)), nil
}

// handleSessionEnd closes an open session by ID.
func (s *Server) handleSessionEnd(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	sessionIDStr, ok := args["session_id"].(string)
	if !ok || sessionIDStr == "" {
		return mcpgo.NewToolResultError("session_end: 'session_id' is required"), nil
	}
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("session_end: invalid session_id: %v", err)), nil
	}

	if s.pool == nil {
		return mcpgo.NewToolResultError("session_end: server not configured (no database connection)"), nil
	}

	session, err := db.EndSession(ctx, s.pool, sessionID, nil)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("session_end: %v", err)), nil
	}

	payload, _ := json.Marshal(map[string]any{
		"session_id": session.ID.String(),
		"ended_at":   session.EndedAt,
	})
	return mcpgo.NewToolResultText(string(payload)), nil
}
