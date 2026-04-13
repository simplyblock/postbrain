package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	"github.com/simplyblock/postbrain/internal/graph"
)

func (s *Server) registerGraphQuery() {
	if !s.ageEnabled {
		return
	}
	s.mcpServer.AddTool(mcpgo.NewTool("graph_query",
		mcpgo.WithReadOnlyHintAnnotation(true),
		mcpgo.WithDestructiveHintAnnotation(false),
		mcpgo.WithIdempotentHintAnnotation(true),
		mcpgo.WithOpenWorldHintAnnotation(false),
		mcpgo.WithDescription("Execute a scoped Cypher query against the AGE graph overlay"),
		mcpgo.WithString("scope", mcpgo.Required(), mcpgo.Description("Scope as kind:external_id")),
		mcpgo.WithString("cypher", mcpgo.Required(), mcpgo.Description("Cypher query body to execute")),
	), withToolMetrics("graph_query", withToolPermission("graph:read", s.handleGraphQuery)))
}

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
