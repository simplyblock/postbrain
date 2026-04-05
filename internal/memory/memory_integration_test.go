//go:build integration

package memory_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/embedding"
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

func TestMemoryCreate_DualWritesToEmbeddingRepository(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{
					"index":     0,
					"embedding": []float32{0.1, 0.2, 0.3, 0.4},
				},
			},
		})
	}))
	defer server.Close()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "mem-dual-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    server.URL + "/v1",
		ProviderModel: "text-embedding-3-small",
		Dimensions:    4,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	cfg := &config.EmbeddingConfig{
		Backend:        "openai",
		ServiceURL:     server.URL + "/v1",
		RequestTimeout: 5 * time.Second,
		BatchSize:      8,
		TextModel:      "unused-static-model",
	}
	svc, err := embedding.NewService(cfg)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if err := svc.EnableModelDrivenFactory(ctx, pool, cfg); err != nil {
		t.Fatalf("EnableModelDrivenFactory: %v", err)
	}

	author := testhelper.CreateTestPrincipal(t, pool, "user", "dual-write-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "dual/write", nil, author.ID)
	store := memory.NewStore(pool, svc)

	result, err := store.Create(ctx, memory.CreateInput{
		Content:    "Dual write check",
		MemoryType: "semantic",
		ScopeID:    scope.ID,
		AuthorID:   author.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var status string
	err = pool.QueryRow(ctx, `
		SELECT status FROM embedding_index
		WHERE object_type = 'memory' AND object_id = $1 AND model_id = $2
	`, result.MemoryID, model.ID).Scan(&status)
	if err != nil {
		t.Fatalf("select embedding_index row: %v", err)
	}
	if status != "ready" {
		t.Fatalf("embedding_index status = %q, want ready", status)
	}

	var exists bool
	var tableName string
	err = pool.QueryRow(ctx, `SELECT table_name FROM embedding_models WHERE id=$1`, model.ID).Scan(&tableName)
	if err != nil {
		t.Fatalf("select model table name: %v", err)
	}
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM %s WHERE object_type='memory' AND object_id=$1
		)
	`, tableName), result.MemoryID).Scan(&exists)
	if err != nil {
		t.Fatalf("select model table row: %v", err)
	}
	if !exists {
		t.Fatal("expected dual-write row in model table")
	}
}
