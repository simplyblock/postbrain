//go:build integration

package testhelper

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/principals"
)

// CreateTestPrincipal inserts a principal and returns it.
func CreateTestPrincipal(t *testing.T, pool *pgxpool.Pool, kind, slug string) *db.Principal {
	t.Helper()
	p, err := db.CreatePrincipal(context.Background(), pool, kind, slug, slug, []byte("{}"))
	if err != nil {
		t.Fatalf("create principal %s: %v", slug, err)
	}
	return p
}

// CreateTestScope inserts a scope and returns it.
func CreateTestScope(t *testing.T, pool *pgxpool.Pool, kind, externalID string, parentID *uuid.UUID, principalID uuid.UUID) *db.Scope {
	t.Helper()
	s, err := db.CreateScope(context.Background(), pool, kind, externalID, externalID, parentID, principalID, []byte("{}"))
	if err != nil {
		t.Fatalf("create scope %s: %v", externalID, err)
	}
	return s
}

// CreateTestMemory inserts a minimal memory record and returns it.
func CreateTestMemory(t *testing.T, pool *pgxpool.Pool, scopeID, authorID uuid.UUID, content string) *db.Memory {
	t.Helper()
	m, err := db.CreateMemory(context.Background(), pool, &db.Memory{
		MemoryType: "semantic",
		ScopeID:    scopeID,
		AuthorID:   authorID,
		Content:    content,
	})
	if err != nil {
		t.Fatalf("create memory: %v", err)
	}
	return m
}

// CreateTestArtifact inserts a minimal published knowledge artifact and returns it.
// CreateTestEmbeddingModel must be called before this in the same test.
func CreateTestArtifact(t *testing.T, pool *pgxpool.Pool, scopeID, authorID uuid.UUID, title string) *db.KnowledgeArtifact {
	t.Helper()
	a, err := db.CreateArtifact(context.Background(), pool, &db.KnowledgeArtifact{
		KnowledgeType: "semantic",
		OwnerScopeID:  scopeID,
		AuthorID:      authorID,
		Visibility:    "project",
		Status:        "published",
		Title:         title,
		Content:       title,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	return a
}

// CreateTestEmbeddingModel inserts an active text embedding model (4 dims for speed).
func CreateTestEmbeddingModel(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO embedding_models (slug, dimensions, content_type, is_active)
		VALUES ('test/model', 4, 'text', true)
		RETURNING id
	`).Scan(&id)
	if err != nil {
		t.Fatalf("create embedding model: %v", err)
	}
	return id
}

// ScopeAuthzGraph is a reusable principal/scope fixture for API authz tests.
// Chain: user -> team -> company, plus unrelated principal/scope.
type ScopeAuthzGraph struct {
	UserPrincipal      *db.Principal
	TeamPrincipal      *db.Principal
	CompanyPrincipal   *db.Principal
	UnrelatedPrincipal *db.Principal

	UserScope      *db.Scope
	TeamScope      *db.Scope
	CompanyScope   *db.Scope
	UnrelatedScope *db.Scope
}

// CreateScopeAuthzGraph creates a reusable multi-hop authorization fixture.
// The role is used for both memberships in the chain: user->team and team->company.
func CreateScopeAuthzGraph(t *testing.T, pool *pgxpool.Pool, prefix, role string) *ScopeAuthzGraph {
	t.Helper()
	ctx := context.Background()

	graph := &ScopeAuthzGraph{
		UserPrincipal:      CreateTestPrincipal(t, pool, "user", prefix+"-user-"+uuid.New().String()),
		TeamPrincipal:      CreateTestPrincipal(t, pool, "team", prefix+"-team-"+uuid.New().String()),
		CompanyPrincipal:   CreateTestPrincipal(t, pool, "company", prefix+"-company-"+uuid.New().String()),
		UnrelatedPrincipal: CreateTestPrincipal(t, pool, "team", prefix+"-unrelated-"+uuid.New().String()),
	}

	graph.CompanyScope = CreateTestScope(t, pool, "project", prefix+"-company-scope-"+uuid.New().String(), nil, graph.CompanyPrincipal.ID)
	graph.TeamScope = CreateTestScope(t, pool, "project", prefix+"-team-scope-"+uuid.New().String(), &graph.CompanyScope.ID, graph.TeamPrincipal.ID)
	graph.UserScope = CreateTestScope(t, pool, "project", prefix+"-user-scope-"+uuid.New().String(), &graph.TeamScope.ID, graph.UserPrincipal.ID)
	graph.UnrelatedScope = CreateTestScope(t, pool, "project", prefix+"-unrelated-scope-"+uuid.New().String(), nil, graph.UnrelatedPrincipal.ID)

	ms := principals.NewMembershipStore(pool)
	if err := ms.AddMembership(ctx, graph.UserPrincipal.ID, graph.TeamPrincipal.ID, role, nil); err != nil {
		t.Fatalf("add membership user->team: %v", err)
	}
	if err := ms.AddMembership(ctx, graph.TeamPrincipal.ID, graph.CompanyPrincipal.ID, role, nil); err != nil {
		t.Fatalf("add membership team->company: %v", err)
	}

	return graph
}
