package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// callTool is a helper that builds a CallToolRequest from a plain map and
// invokes the given handler, returning the result.
func callTool(t *testing.T, args map[string]any, fn func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)) *mcpgo.CallToolResult {
	t.Helper()
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	result, err := fn(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected Go error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}
	return result
}

// assertToolError fails the test if result.IsError is not true.
func assertToolError(t *testing.T, result *mcpgo.CallToolResult) {
	t.Helper()
	if !result.IsError {
		t.Errorf("expected tool error (IsError=true), got IsError=false")
	}
}

// ── handleRecall ─────────────────────────────────────────────────────────────

func TestHandleRecall_MissingQuery_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"scope": "project:acme/api",
		// query intentionally omitted
	}, s.handleRecall)
	assertToolError(t, result)
}

func TestHandleRecall_EmptyQuery_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"query": "",
		"scope": "project:acme/api",
	}, s.handleRecall)
	assertToolError(t, result)
}

// ── handleRemember ────────────────────────────────────────────────────────────

// Note: TestHandleRemember_MissingContent and TestHandleRemember_MissingScope
// already exist in server_test.go.  The case below covers the empty-string
// variant to complement those tests.
func TestHandleRemember_EmptyContent_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"content": "", // present but empty
		"scope":   "project:acme/api",
	}, s.handleRemember)
	assertToolError(t, result)
}

// ── handleForget ──────────────────────────────────────────────────────────────

func TestHandleForget_InvalidUUID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"memory_id": "not-a-valid-uuid",
	}, s.handleForget)
	assertToolError(t, result)
}

func TestHandleForget_MissingMemoryID_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		// memory_id intentionally omitted
	}, s.handleForget)
	assertToolError(t, result)
}

// ── handlePublish ─────────────────────────────────────────────────────────────

func TestHandlePublish_MissingTitle_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"content":        "some content",
		"knowledge_type": "note",
		"scope":          "project:acme/api",
		// title intentionally omitted
	}, s.handlePublish)
	assertToolError(t, result)
}

func TestHandlePublish_MissingScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"title":          "My Doc",
		"content":        "some content",
		"knowledge_type": "note",
		// scope intentionally omitted
	}, s.handlePublish)
	assertToolError(t, result)
}

// ── handleSummarize ───────────────────────────────────────────────────────────

func TestHandleSummarize_MissingScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		// scope intentionally omitted
	}, s.handleSummarize)
	assertToolError(t, result)
}

func TestHandleSummarize_EmptyScope_ReturnsToolError(t *testing.T) {
	s := &Server{}
	result := callTool(t, map[string]any{
		"scope": "", // present but empty
	}, s.handleSummarize)
	assertToolError(t, result)
}
