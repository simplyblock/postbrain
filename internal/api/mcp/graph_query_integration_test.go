//go:build integration

package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/graph"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMCP_GraphQuery_AGEAwareBehavior(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-graph-query-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "mcp-graph-query-scope", nil, principal.ID)
	ctx = withAuthContext(ctx, principal.ID, scope.ID)

	if err := db.EnsureAGEOverlay(ctx, pool); err != nil {
		t.Fatalf("EnsureAGEOverlay: %v", err)
	}
	ageAvailable := graph.DetectAGE(ctx, pool)
	if ageAvailable {
		if err := graph.SyncEntityToAGE(ctx, pool, &db.Entity{
			ID:         scope.ID, // deterministic test id
			ScopeID:    scope.ID,
			EntityType: "file",
			Name:       "auth.go",
			Canonical:  "src/auth.go",
		}); err != nil {
			t.Fatalf("SyncEntityToAGE: %v", err)
		}
	}

	srv := mcpapi.NewServer(pool, svc, cfg).MCPServer()
	tool := srv.GetTool("graph_query")
	if tool == nil {
		t.Fatal("graph_query tool not registered")
	}

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "graph_query"
	req.Params.Arguments = map[string]any{
		"scope":  "project:" + scope.ExternalID,
		"cypher": "RETURN n",
	}
	result, err := tool.Handler(ctx, req)
	if err != nil {
		t.Fatalf("graph_query handler error: %v", err)
	}
	if result == nil {
		t.Fatal("graph_query returned nil result")
	}

	text := result.Content[0].(mcpgo.TextContent).Text
	if !ageAvailable {
		if !result.IsError {
			t.Fatalf("expected graph_query error when AGE unavailable, got success: %s", text)
		}
		if !strings.Contains(text, "AGE unavailable") {
			t.Fatalf("expected AGE unavailable error, got: %s", text)
		}
		return
	}

	if result.IsError {
		t.Fatalf("graph_query returned error with AGE available: %s", text)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("graph_query payload JSON: %v", err)
	}
	rows, ok := payload["rows"].([]any)
	if !ok || len(rows) == 0 {
		t.Fatalf("expected non-empty rows, got: %#v", payload["rows"])
	}
}
