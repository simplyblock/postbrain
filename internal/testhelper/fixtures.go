//go:build integration

package testhelper

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
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
