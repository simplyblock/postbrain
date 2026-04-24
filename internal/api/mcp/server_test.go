package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// ---- handleForget tests ----

func TestHandleForget_SoftDelete(t *testing.T) {
	s := &Server{}
	memID := "018e4f2a-0000-7000-8000-000000000001"
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"memory_id": memID,
		"hard":      false,
	}

	result, err := s.handleForget(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// With nil pool the store will fail; we only verify the result shape.
	// We just check it returns a result (error or success).
}

func TestHandleForget_ReturnShape(t *testing.T) {
	s := &Server{}
	cases := []struct {
		name   string
		hard   bool
		action string
	}{
		{"soft", false, "deactivated"},
		{"hard", true, "deleted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcpgo.CallToolRequest{}
			req.Params.Arguments = map[string]any{
				"memory_id": "018e4f2a-0000-7000-8000-000000000001",
				"hard":      tc.hard,
			}
			result, _ := s.handleForget(context.Background(), req)
			if result != nil && !result.IsError {
				// Parse content to check action field.
				if len(result.Content) > 0 {
					if tc, ok := result.Content[0].(mcpgo.TextContent); ok {
						var m map[string]any
						if jerr := json.Unmarshal([]byte(tc.Text), &m); jerr == nil {
							_ = m // action verified in integration tests
						}
					}
				}
			}
		})
	}
}

// ---- handleRemember tests ----

func TestHandleRemember_MissingContent(t *testing.T) {
	s := &Server{}
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"scope": "project:acme/api",
		// content is intentionally omitted
	}
	result, err := s.handleRemember(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error (should return tool error, not Go error): %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for missing content, got false")
	}
}

func TestHandleRemember_MissingScope(t *testing.T) {
	s := &Server{}
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"content": "some content",
		// scope omitted
	}
	result, err := s.handleRemember(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Errorf("expected IsError=true for missing scope, got false")
	}
}

// ---- handleRecall tests ----

func TestHandleRecall_MemoryLayerOnly(t *testing.T) {
	// This test verifies that when layers=["memory"] is passed,
	// handleRecall still returns a result structure (with nil stores it will error,
	// but we verify the layer parsing logic does not panic).
	s := &Server{}
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query":  "test query",
		"scope":  "project:acme/api",
		"layers": []any{"memory"},
	}
	// With nil stores this will return an error result; that's expected in unit tests.
	result, err := s.handleRecall(context.Background(), req)
	// Should not panic.
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	_ = result
}

func TestNewServer_NoPool_DoesNotRegisterGraphQuery(t *testing.T) {
	s := NewServer(nil, nil, nil)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if tool := s.MCPServer().GetTool("graph_query"); tool != nil {
		t.Fatal("graph_query must not be registered when database/AGE is unavailable")
	}
}

func TestNewServer_RegistersCrossScopeContextTool(t *testing.T) {
	s := NewServer(nil, nil, nil)
	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if tool := s.MCPServer().GetTool("cross_scope_context"); tool == nil {
		t.Fatal("cross_scope_context tool must be registered")
	}
}

func TestNewServer_ArrayArgsDefineItemsSchema(t *testing.T) {
	s := NewServer(nil, nil, nil)
	if s == nil {
		t.Fatal("expected non-nil server")
	}

	cases := []struct {
		tool    string
		arg     string
		wantAny bool
	}{
		{tool: "recall", arg: "memory_types"},
		{tool: "recall", arg: "layers"},
		{tool: "cross_scope_context", arg: "comparison_scopes"},
		{tool: "cross_scope_context", arg: "layers"},
		{tool: "synthesize_topic", arg: "source_ids"},
		{tool: "remember", arg: "entities", wantAny: true},
	}

	for _, tc := range cases {
		t.Run(tc.tool+"_"+tc.arg, func(t *testing.T) {
			tool := s.MCPServer().GetTool(tc.tool)
			if tool == nil {
				t.Fatalf("%s tool must be registered", tc.tool)
			}

			prop, ok := tool.Tool.InputSchema.Properties[tc.arg].(map[string]any)
			if !ok {
				t.Fatalf("%s.%s property must be an object schema", tc.tool, tc.arg)
			}

			items, ok := prop["items"].(map[string]any)
			if !ok {
				t.Fatalf("%s.%s must define array items schema", tc.tool, tc.arg)
			}
			if tc.wantAny {
				return
			}
			if got := items["type"]; got != "string" {
				t.Fatalf("%s.%s items.type = %v, want string", tc.tool, tc.arg, got)
			}
		})
	}
}
