//go:build integration

package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/memory"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMemoryCreate_Created(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewDeterministicEmbeddingService()

	author := testhelper.CreateTestPrincipal(t, pool, "user", "alice")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme/api", nil, author.ID)

	store := memory.NewStore(pool, svc)
	result, err := store.Create(context.Background(), memory.CreateInput{
		Content:    "The payment service owns all Stripe webhook processing",
		MemoryType: "semantic",
		ScopeID:    scope.ID,
		AuthorID:   author.ID,
		Importance: 0.8,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if result.Action != "created" {
		t.Errorf("expected action=created, got %s", result.Action)
	}
	if result.MemoryID == uuid.Nil {
		t.Error("expected non-nil MemoryID")
	}
}

func TestMemoryCreate_NearDuplicateReturnsUpdated(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)

	// Use deterministic embedder: same content → same vector → cosine dist = 0
	svc := testhelper.NewDeterministicEmbeddingService()
	author := testhelper.CreateTestPrincipal(t, pool, "user", "bob")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme/api2", nil, author.ID)
	store := memory.NewStore(pool, svc)

	ctx := context.Background()
	input := memory.CreateInput{
		Content:    "Identical content for dedup test",
		MemoryType: "semantic",
		ScopeID:    scope.ID,
		AuthorID:   author.ID,
	}

	r1, err := store.Create(ctx, input)
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if r1.Action != "created" {
		t.Errorf("expected created, got %s", r1.Action)
	}

	r2, err := store.Create(ctx, input)
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if r2.Action != "updated" {
		t.Errorf("expected updated, got %s", r2.Action)
	}
	if r2.MemoryID != r1.MemoryID {
		t.Error("expected same memory ID on dedup")
	}
}

func TestMemoryCreate_WorkingDefaultTTL(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	author := testhelper.CreateTestPrincipal(t, pool, "user", "carol")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme/ttl", nil, author.ID)
	store := memory.NewStore(pool, svc)

	result, err := store.Create(context.Background(), memory.CreateInput{
		Content:    "Working memory with default TTL",
		MemoryType: "working",
		ScopeID:    scope.ID,
		AuthorID:   author.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Fetch the memory and verify expires_at is ~1 hour from now
	mem, err := db.GetMemory(context.Background(), pool, result.MemoryID)
	if err != nil {
		t.Fatalf("GetMemory: %v", err)
	}
	if mem.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set for working memory")
	}

	expectedExpiry := time.Now().Add(3600 * time.Second)
	diff := mem.ExpiresAt.Sub(expectedExpiry)
	if diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt %v not within 5s of expected %v", mem.ExpiresAt, expectedExpiry)
	}
}

func TestMemorySoftDelete_ExcludedFromRecall(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewDeterministicEmbeddingService()
	author := testhelper.CreateTestPrincipal(t, pool, "user", "dave")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme/del", nil, author.ID)
	store := memory.NewStore(pool, svc)
	ctx := context.Background()

	r, err := store.Create(ctx, memory.CreateInput{
		Content:    "Memory to be deleted",
		MemoryType: "semantic",
		ScopeID:    scope.ID,
		AuthorID:   author.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.SoftDelete(ctx, r.MemoryID); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	results, err := store.Recall(ctx, memory.RecallInput{
		Query:       "Memory to be deleted",
		ScopeID:     scope.ID,
		PrincipalID: author.ID,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	for _, res := range results {
		if res.Memory.ID == r.MemoryID {
			t.Error("soft-deleted memory should not appear in Recall results")
		}
	}
}

func TestMemoryHardDelete(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	testhelper.CreateTestEmbeddingModel(t, pool)
	svc := testhelper.NewMockEmbeddingService()
	author := testhelper.CreateTestPrincipal(t, pool, "user", "eve")
	scope := testhelper.CreateTestScope(t, pool, "project", "acme/hard", nil, author.ID)
	store := memory.NewStore(pool, svc)
	ctx := context.Background()

	r, err := store.Create(ctx, memory.CreateInput{
		Content:    "Hard delete target",
		MemoryType: "episodic",
		ScopeID:    scope.ID,
		AuthorID:   author.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := store.HardDelete(ctx, r.MemoryID); err != nil {
		t.Fatalf("HardDelete: %v", err)
	}

	mem, err := db.GetMemory(ctx, pool, r.MemoryID)
	if err == nil && mem != nil {
		t.Error("expected hard-deleted memory to be gone")
	}
}
