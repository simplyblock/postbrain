package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/graph"
)

func (s *Server) handleGraphQuery(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
	args := req.GetArguments()

	cypher := argString(args, "cypher")
	if strings.TrimSpace(cypher) == "" {
		return mcpgo.NewToolResultError("graph_query: 'cypher' is required"), nil
	}
	scopeStr := argString(args, "scope")
	if strings.TrimSpace(scopeStr) == "" {
		return mcpgo.NewToolResultError("graph_query: 'scope' is required"), nil
	}
	if s.pool == nil {
		return mcpgo.NewToolResultError("graph_query: server not configured (no database connection)"), nil
	}

	scopeID, errResult := s.resolveScope(ctx, "graph_query", scopeStr)
	if errResult != nil {
		return errResult, nil
	}

	rows, err := graph.RunCypherQuery(ctx, s.pool, scopeID, cypher)
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
