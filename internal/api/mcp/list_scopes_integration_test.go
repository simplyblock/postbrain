//go:build integration

package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMCP_ListScopes_RestrictedToEffectiveWritableScopes(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principalA := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-list-scope-a-"+uuid.NewString())
	principalB := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-list-scope-b-"+uuid.NewString())
	scopeA1 := testhelper.CreateTestScope(t, pool, "project", "mcp-list-a1-"+uuid.NewString(), nil, principalA.ID)
	scopeA2 := testhelper.CreateTestScope(t, pool, "project", "mcp-list-a2-"+uuid.NewString(), nil, principalA.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "mcp-list-b-"+uuid.NewString(), nil, principalB.ID)

	srv := mcpapi.NewServer(pool, svc, cfg)
	tool := srv.MCPServer().GetTool("list_scopes")
	if tool == nil {
		t.Fatal("list_scopes tool not registered")
	}

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "list_scopes"
	req.Params.Arguments = map[string]any{}

	ctx = context.WithValue(ctx, auth.ContextKeyPrincipalID, principalA.ID)
	ctx = context.WithValue(ctx, auth.ContextKeyToken, &db.Token{
		PrincipalID: principalA.ID,
		ScopeIds:    nil,
		Permissions: []string{"scopes:read"},
	})

	result, err := tool.Handler(ctx, req)
	if err != nil {
		t.Fatalf("list_scopes failed: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("list_scopes returned error result: %+v", result)
	}
	if len(result.Content) == 0 {
		t.Fatal("list_scopes returned empty content")
	}
	text, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}

	var payload struct {
		Scopes []struct {
			ID string `json:"id"`
		} `json:"scopes"`
	}
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("unmarshal list_scopes payload: %v", err)
	}

	got := map[string]bool{}
	for _, s := range payload.Scopes {
		got[s.ID] = true
	}
	if !got[scopeA1.ID.String()] || !got[scopeA2.ID.String()] {
		t.Fatalf("expected principalA writable scopes in output, got=%v", got)
	}
	if got[scopeB.ID.String()] {
		t.Fatalf("did not expect principalB scope %s in output", scopeB.ID)
	}
}
