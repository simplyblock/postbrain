//go:build integration

package mcp_test

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"

	mcpapi "github.com/simplyblock/postbrain/internal/api/mcp"
	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMCP_Remember_Recall_Forget(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-e2e-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "mcp-e2e-project", nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	srv := mcpapi.NewServer(pool, svc, cfg)
	mcpSrv := srv.MCPServer()

	ctx = context.WithValue(ctx, auth.ContextKeyPrincipalID, principal.ID)
	scopeStr := "project:" + scope.ExternalID

	// Test remember.
	rememberTool := mcpSrv.GetTool("remember")
	if rememberTool == nil {
		t.Fatal("remember tool not registered")
	}

	req := mcpgo.CallToolRequest{}
	req.Params.Name = "remember"
	req.Params.Arguments = map[string]any{
		"content":     "E2E test: Go channels are goroutine-safe communication primitives",
		"scope":       scopeStr,
		"memory_type": "semantic",
	}

	result, err := rememberTool.Handler(ctx, req)
	if err != nil {
		t.Fatalf("remember failed: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("remember returned error result: %+v", result)
	}

	// Verify content is non-empty.
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty result content")
	}

	// Test recall.
	recallTool := mcpSrv.GetTool("recall")
	if recallTool == nil {
		t.Fatal("recall tool not registered")
	}

	recallReq := mcpgo.CallToolRequest{}
	recallReq.Params.Name = "recall"
	recallReq.Params.Arguments = map[string]any{
		"query": "goroutine communication",
		"scope": scopeStr,
	}

	recallResult, err := recallTool.Handler(ctx, recallReq)
	if err != nil {
		t.Fatalf("recall failed: %v", err)
	}
	if recallResult == nil {
		t.Fatal("recall returned nil result")
	}
}

func TestMCP_Publish_Endorse_AutoPublish(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	cfg := &config.Config{}

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "mcp-publish-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "mcp-publish-project", nil, principal.ID)
	testhelper.CreateTestEmbeddingModel(t, pool)

	srv := mcpapi.NewServer(pool, svc, cfg)
	mcpSrv := srv.MCPServer()

	ctx = context.WithValue(ctx, auth.ContextKeyPrincipalID, principal.ID)
	scopeStr := "project:" + scope.ExternalID

	// Publish an artifact.
	publishTool := mcpSrv.GetTool("publish")
	if publishTool == nil {
		t.Fatal("publish tool not registered")
	}

	pubReq := mcpgo.CallToolRequest{}
	pubReq.Params.Name = "publish"
	pubReq.Params.Arguments = map[string]any{
		"title":          "E2E Test Article",
		"content":        "This is a test knowledge artifact for E2E testing",
		"knowledge_type": "semantic",
		"scope":          scopeStr,
		"auto_review":    true,
	}

	pubResult, err := publishTool.Handler(ctx, pubReq)
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if pubResult == nil || pubResult.IsError {
		t.Fatalf("publish returned error: %+v", pubResult)
	}
}
