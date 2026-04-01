//go:build integration

package jobs

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestReembedJob_RunText_NoActiveModel(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	// No embedding_model is inserted → activeModelID returns nil → RunText returns nil.
	j := NewReembedJob(pool, svc, 1)
	if err := j.RunText(ctx); err != nil {
		t.Fatalf("RunText with no active model: %v", err)
	}
}

func TestReembedJob_RunText_ReembedsMismatchedMemory(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	// Insert active text embedding model.
	activeID := testhelper.CreateTestEmbeddingModel(t, pool)

	// Create a memory whose embedding_model_id is NULL (mismatched).
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-text-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-text-scope", nil, principal.ID)
	mem := testhelper.CreateTestMemory(t, pool, scope.ID, principal.ID, "reembed text content")

	// Verify embedding_model_id starts NULL.
	var beforeModelID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT embedding_model_id FROM memories WHERE id = $1`, mem.ID,
	).Scan(&beforeModelID); err != nil {
		t.Fatalf("scan before RunText: %v", err)
	}
	if beforeModelID != nil {
		t.Fatalf("expected embedding_model_id=NULL before reembed, got %v", beforeModelID)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunText(ctx); err != nil {
		t.Fatalf("RunText: %v", err)
	}

	var afterModelID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT embedding_model_id FROM memories WHERE id = $1`, mem.ID,
	).Scan(&afterModelID); err != nil {
		t.Fatalf("scan after RunText: %v", err)
	}
	if afterModelID == nil {
		t.Fatal("expected embedding_model_id to be set after RunText")
	}
	if *afterModelID != activeID {
		t.Errorf("embedding_model_id = %v; want %v", *afterModelID, activeID)
	}
}

func TestReembedJob_RunCode_ReembedsMismatchedCodeMemory(t *testing.T) {
	t.Parallel()
	pool := testhelper.NewTestPool(t)
	svc := testhelper.NewMockEmbeddingService()
	ctx := context.Background()

	// Insert active code embedding model.
	var codeModelID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO embedding_models (slug, dimensions, content_type, is_active)
		 VALUES ('test/code-model', 4, 'code', true)
		 RETURNING id`,
	).Scan(&codeModelID); err != nil {
		t.Fatalf("create code embedding model: %v", err)
	}

	// Create a memory with content_kind='code' and no code embedding model set.
	principal := testhelper.CreateTestPrincipal(t, pool, "user", "reembed-code-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "reembed-code-scope", nil, principal.ID)

	var memID uuid.UUID
	if err := pool.QueryRow(ctx,
		`INSERT INTO memories (memory_type, scope_id, author_id, content, content_kind, is_active)
		 VALUES ('semantic', $1, $2, 'fn main() {}', 'code', true)
		 RETURNING id`,
		scope.ID, principal.ID,
	).Scan(&memID); err != nil {
		t.Fatalf("insert code memory: %v", err)
	}

	j := NewReembedJob(pool, svc, 64)
	if err := j.RunCode(ctx); err != nil {
		t.Fatalf("RunCode: %v", err)
	}

	var afterModelID *uuid.UUID
	if err := pool.QueryRow(ctx,
		`SELECT embedding_code_model_id FROM memories WHERE id = $1`, memID,
	).Scan(&afterModelID); err != nil {
		t.Fatalf("scan after RunCode: %v", err)
	}
	if afterModelID == nil {
		t.Fatal("expected embedding_code_model_id to be set after RunCode")
	}
	if *afterModelID != codeModelID {
		t.Errorf("embedding_code_model_id = %v; want %v", *afterModelID, codeModelID)
	}
}
