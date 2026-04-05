//go:build integration

package mcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMCP_PermissionAuthz_ReadVsWrite(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-perm-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "mcp-perm-scope-"+uuid.NewString(), nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	srv := mcpapi.NewServer(pool, svc, cfg).MCPServer()

	ctxRead := context.WithValue(ctx, auth.ContextKeyPrincipalID, principal.ID)
	ctxRead = context.WithValue(ctxRead, auth.ContextKeyToken, &db.Token{
		PrincipalID: principal.ID,
		ScopeIds:    []uuid.UUID{scope.ID},
		Permissions: []string{"read"},
	})
	ctxWrite := context.WithValue(ctx, auth.ContextKeyPrincipalID, principal.ID)
	ctxWrite = context.WithValue(ctxWrite, auth.ContextKeyToken, &db.Token{
		PrincipalID: principal.ID,
		ScopeIds:    []uuid.UUID{scope.ID},
		Permissions: []string{"write"},
	})

	recallTool := srv.GetTool("recall")
	if recallTool == nil {
		t.Fatal("recall tool not registered")
	}
	rememberTool := srv.GetTool("remember")
	if rememberTool == nil {
		t.Fatal("remember tool not registered")
	}

	t.Run("read permission can use read tool", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "recall"
		req.Params.Arguments = map[string]any{
			"query": "permission check",
			"scope": "project:" + scope.ExternalID,
		}
		result, err := recallTool.Handler(ctxRead, req)
		if err != nil {
			t.Fatalf("recall handler error: %v", err)
		}
		if result == nil || result.IsError {
			t.Fatalf("recall expected success, got %+v", result)
		}
	})

	t.Run("read permission cannot use write tool", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "remember"
		req.Params.Arguments = map[string]any{
			"content":     "permission check write",
			"scope":       "project:" + scope.ExternalID,
			"memory_type": "semantic",
		}
		result, err := rememberTool.Handler(ctxRead, req)
		if err != nil {
			t.Fatalf("remember handler error: %v", err)
		}
		if result == nil || !result.IsError {
			t.Fatalf("remember expected error, got %+v", result)
		}
		msg := firstToolText(result)
		if !strings.Contains(msg, "forbidden: insufficient permissions") {
			t.Fatalf("remember msg = %q, want insufficient permissions", msg)
		}
	})

	t.Run("write permission can use write tool", func(t *testing.T) {
		req := mcpgo.CallToolRequest{}
		req.Params.Name = "remember"
		req.Params.Arguments = map[string]any{
			"content":     "permission check write allowed",
			"scope":       "project:" + scope.ExternalID,
			"memory_type": "semantic",
		}
		result, err := rememberTool.Handler(ctxWrite, req)
		if err != nil {
			t.Fatalf("remember handler error: %v", err)
		}
		if result == nil || result.IsError {
			t.Fatalf("remember expected success, got %+v", result)
		}
	})
}
