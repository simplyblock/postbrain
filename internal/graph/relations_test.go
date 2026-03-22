package graph_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/graph"
)

func TestLinkMemoryToEntity_InvalidRole(t *testing.T) {
	s := graph.NewStore(nil)
	// nil pool — we're only testing validation which happens before the DB call.
	err := s.LinkMemoryToEntity(nil, uuid.New(), uuid.New(), "invalid-role") //nolint:staticcheck
	if err != graph.ErrInvalidRole {
		t.Fatalf("expected ErrInvalidRole, got %v", err)
	}
}

func TestExtractEntitiesFromMemory_FileSourceRef(t *testing.T) {
	entities := graph.ExtractEntitiesFromMemory("some content", strptr("file:src/auth.go:42"))
	found := false
	for _, e := range entities {
		if e.EntityType == "file" && e.Canonical == "src/auth.go" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected file entity with canonical='src/auth.go', got %+v", entities)
	}
}

func TestExtractEntitiesFromMemory_PRPattern(t *testing.T) {
	entities := graph.ExtractEntitiesFromMemory("Fixed issue in pr:123 today", nil)
	found := false
	for _, e := range entities {
		if e.EntityType == "pr" && e.Canonical == "pr:123" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected pr entity with canonical='pr:123', got %+v", entities)
	}
}

func TestExtractEntitiesFromMemory_PascalCase(t *testing.T) {
	entities := graph.ExtractEntitiesFromMemory("The UserRepository class uses AuthService", nil)
	types := map[string]bool{}
	for _, e := range entities {
		types[e.Canonical] = true
	}
	if !types["UserRepository"] && !types["AuthService"] {
		t.Fatalf("expected concept entities for PascalCase words, got %+v", entities)
	}
}

func strptr(s string) *string { return &s }
