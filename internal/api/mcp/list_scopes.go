package mcp

import (
	"context"
	"encoding/json"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

// handleListScopes returns all scopes accessible to the calling token.
// Tokens with a nil scope_ids list have access to all scopes; tokens with an
// explicit list are restricted to those scopes.
func (s *Server) handleListScopes(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	if s.pool == nil {
		return mcpgo.NewToolResultError("list_scopes: server not configured (no database connection)"), nil
	}

	token, _ := ctx.Value(auth.ContextKeyToken).(*db.Token)

	var scopes []*db.Scope
	var err error

	if token == nil || token.ScopeIds == nil {
		// No restriction — return all scopes.
		q := db.New(s.pool)
		rows, qErr := q.ListScopes(ctx, db.ListScopesParams{Limit: 1000, Offset: 0})
		if qErr != nil {
			return mcpgo.NewToolResultError("list_scopes: " + qErr.Error()), nil
		}
		for _, r := range rows {
			scopes = append(scopes, &db.Scope{
				ID:          r.ID,
				Kind:        r.Kind,
				ExternalID:  r.ExternalID,
				Name:        r.Name,
				ParentID:    r.ParentID,
				PrincipalID: r.PrincipalID,
				Path:        r.Path,
				Meta:        r.Meta,
				CreatedAt:   r.CreatedAt,
			})
		}
	} else {
		scopes, err = db.GetScopesByIDs(ctx, s.pool, token.ScopeIds)
		if err != nil {
			return mcpgo.NewToolResultError("list_scopes: " + err.Error()), nil
		}
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
