package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/graph"
)

func (s *Server) handleGraphQuery(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	cypher, ok := args["cypher"].(string)
	if !ok || strings.TrimSpace(cypher) == "" {
		return mcpgo.NewToolResultError("graph_query: 'cypher' is required"), nil
	}
	scopeStr, ok := args["scope"].(string)
	if !ok || strings.TrimSpace(scopeStr) == "" {
		return mcpgo.NewToolResultError("graph_query: 'scope' is required"), nil
	}
	if s.pool == nil {
		return mcpgo.NewToolResultError("graph_query: server not configured (no database connection)"), nil
	}

	kind, externalID, err := parseScopeString(scopeStr)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("graph_query: invalid scope: %v", err)), nil
	}
	scope, err := db.GetScopeByExternalID(ctx, s.pool, kind, externalID)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("graph_query: scope lookup failed: %v", err)), nil
	}
	if scope == nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("graph_query: scope '%s' not found", scopeStr)), nil
	}
	if err := s.authorizeRequestedScope(ctx, scope.ID); err != nil {
		return scopeAuthzToolError(ctx, "graph_query", scope.ID, err), nil
	}

	rows, err := graph.RunCypherQuery(ctx, s.pool, scope.ID, cypher)
	if err != nil {
		if err == graph.ErrAGEUnavailable {
			return mcpgo.NewToolResultError("graph_query: AGE unavailable"), nil
		}
		return mcpgo.NewToolResultError(fmt.Sprintf("graph_query: query failed: %v", err)), nil
	}

	out := map[string]any{
		"rows": rows,
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return mcpgo.NewToolResultError(fmt.Sprintf("graph_query: marshal output: %v", err)), nil
	}
	return mcpgo.NewToolResultText(string(payload)), nil
}
