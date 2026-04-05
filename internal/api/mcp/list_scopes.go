package mcp

import (
	"context"
	"encoding/json"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/db"
)

// handleListScopes returns all scopes writable by the calling principal,
// further restricted by token scope_ids when present.
func (s *Server) handleListScopes(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.pool == nil {
		return mcpgo.NewToolResultError("list_scopes: server not configured (no database connection)"), nil
	}

	authorizedScopeIDs, err := s.authorizedScopeIDsForRequest(ctx)
	if err != nil {
		return mcpgo.NewToolResultError("list_scopes: " + err.Error()), nil
	}
	scopes, err := db.GetScopesByIDs(ctx, s.pool, authorizedScopeIDs)
	if err != nil {
		return mcpgo.NewToolResultError("list_scopes: " + err.Error()), nil
	}

	type scopeJSON struct {
		ID         string `json:"id"`
		Kind       string `json:"kind"`
		ExternalID string `json:"external_id"`
		Name       string `json:"name"`
		Scope      string `json:"scope"` // convenience: "kind:external_id"
	}

	out := make([]scopeJSON, 0, len(scopes))
	for _, s := range scopes {
		out = append(out, scopeJSON{
			ID:         s.ID.String(),
			Kind:       s.Kind,
			ExternalID: s.ExternalID,
			Name:       s.Name,
			Scope:      s.Kind + ":" + s.ExternalID,
		})
	}

	payload, _ := json.Marshal(map[string]any{"scopes": out})
	return mcpgo.NewToolResultText(string(payload)), nil
}
