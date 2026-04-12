package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// callResource is a test helper that builds a ReadResourceRequest and invokes
// the given resource handler, returning the contents slice and any error.
func callResource(t *testing.T, uri string, fn func(context.Context, mcpgo.ReadResourceRequest) ([]mcpgo.ResourceContents, error)) ([]mcpgo.ResourceContents, error) {
	t.Helper()
	req := mcpgo.ReadResourceRequest{}
	req.Params.URI = uri
	return fn(context.Background(), req)
}

// ── handleMemoryResource ──────────────────────────────────────────────────────

func TestHandleMemoryResource_NilPool_ReturnsError(t *testing.T) {
	s := &Server{}
	_, err := callResource(t, "postbrain://memory/00000000-0000-0000-0000-000000000001", s.handleMemoryResource)
	if err == nil {
		t.Fatal("expected error with nil pool, got nil")
	}
}

func TestHandleMemoryResource_InvalidUUID_ReturnsError(t *testing.T) {
	s := &Server{} // nil pool fires after UUID parse — but UUID parse fires first
	_, err := callResource(t, "postbrain://memory/not-a-uuid", s.handleMemoryResource)
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestHandleMemoryResource_MissingID_ReturnsError(t *testing.T) {
	s := &Server{}
	_, err := callResource(t, "postbrain://memory/", s.handleMemoryResource)
	if err == nil {
		t.Fatal("expected error for missing UUID, got nil")
	}
}

// ── handleKnowledgeResource ───────────────────────────────────────────────────

func TestHandleKnowledgeResource_NilPool_ReturnsError(t *testing.T) {
	s := &Server{}
	_, err := callResource(t, "postbrain://knowledge/00000000-0000-0000-0000-000000000001", s.handleKnowledgeResource)
	if err == nil {
		t.Fatal("expected error with nil pool, got nil")
	}
}

func TestHandleKnowledgeResource_InvalidUUID_ReturnsError(t *testing.T) {
	s := &Server{}
	_, err := callResource(t, "postbrain://knowledge/not-a-uuid", s.handleKnowledgeResource)
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestHandleKnowledgeResource_MissingID_ReturnsError(t *testing.T) {
	s := &Server{}
	_, err := callResource(t, "postbrain://knowledge/", s.handleKnowledgeResource)
	if err == nil {
		t.Fatal("expected error for missing UUID, got nil")
	}
}

// ── handleSessionResource ─────────────────────────────────────────────────────

func TestHandleSessionResource_NilPool_ReturnsError(t *testing.T) {
	s := &Server{}
	_, err := callResource(t, "postbrain://session/00000000-0000-0000-0000-000000000001", s.handleSessionResource)
	if err == nil {
		t.Fatal("expected error with nil pool, got nil")
	}
}

func TestHandleSessionResource_InvalidUUID_ReturnsError(t *testing.T) {
	s := &Server{}
	_, err := callResource(t, "postbrain://session/not-a-uuid", s.handleSessionResource)
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
}

func TestHandleSessionResource_MissingID_ReturnsError(t *testing.T) {
	s := &Server{}
	_, err := callResource(t, "postbrain://session/", s.handleSessionResource)
	if err == nil {
		t.Fatal("expected error for missing UUID, got nil")
	}
}
