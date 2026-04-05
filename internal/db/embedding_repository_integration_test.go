//go:build integration

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func sparseVec(dims int, idx int, value float32) []float32 {
	v := make([]float32, dims)
	if idx >= 0 && idx < dims {
		v[idx] = value
	}
	return v
}

func TestEmbeddingRepository_UpsertAndGet(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "owner-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	memory := testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "hello")

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "repo-model-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-small",
		Dimensions:    3,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	repo := db.NewEmbeddingRepository(pool)
	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "memory",
		ObjectID:   memory.ID,
		ScopeID:    scope.ID,
		ModelID:    model.ID,
		Embedding:  []float32{1, 2, 3},
	}); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	got, err := repo.GetEmbedding(ctx, model.ID, "memory", memory.ID)
	if err != nil {
		t.Fatalf("GetEmbedding: %v", err)
	}
	if len(got) != 3 || got[0] != 1 {
		t.Fatalf("embedding = %#v, want [1 2 3]", got)
	}
}

func TestEmbeddingRepository_QuerySimilar_HighDimensionHalfvecPath(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "owner-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	memoryA := testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "hello a")
	memoryB := testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "hello b")

	const dims = 2048
	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "repo-model-high-dim-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-large",
		Dimensions:    dims,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	repo := db.NewEmbeddingRepository(pool)
	vecA := sparseVec(dims, 0, 1.0)
	vecB := sparseVec(dims, 0, -1.0)

	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "memory",
		ObjectID:   memoryA.ID,
		ScopeID:    scope.ID,
		ModelID:    model.ID,
		Embedding:  vecA,
	}); err != nil {
		t.Fatalf("UpsertEmbedding A: %v", err)
	}
	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "memory",
		ObjectID:   memoryB.ID,
		ScopeID:    scope.ID,
		ModelID:    model.ID,
		Embedding:  vecB,
	}); err != nil {
		t.Fatalf("UpsertEmbedding B: %v", err)
	}

	hits, err := repo.QuerySimilar(ctx, db.EmbeddingQuery{
		ModelID:    model.ID,
		ObjectType: "memory",
		Embedding:  vecA,
		Limit:      2,
		Scope: &db.ScopeFilter{
			ScopePath: scope.Path,
		},
	})
	if err != nil {
		t.Fatalf("QuerySimilar: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits len = %d, want 2", len(hits))
	}
	if hits[0].ObjectID != memoryA.ID {
		t.Fatalf("first hit object id = %s, want %s", hits[0].ObjectID, memoryA.ID)
	}
}

func TestEmbeddingRepository_UpsertEmbeddingDimensionMismatch(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "owner-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	memory := testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "hello")

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "repo-model-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-small",
		Dimensions:    3,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	repo := db.NewEmbeddingRepository(pool)
	err = repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "memory",
		ObjectID:   memory.ID,
		ScopeID:    scope.ID,
		ModelID:    model.ID,
		Embedding:  []float32{1, 2},
	})
	if err == nil {
		t.Fatal("expected dimension mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "dimension") {
		t.Fatalf("expected dimension error, got: %v", err)
	}
}

func TestEmbeddingRepository_UpsertEmbedding_ModelNotReady(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "owner-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	memory := testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "hello")

	modelID := uuid.New()
	if _, err := pool.Exec(ctx, `
		INSERT INTO embedding_models (id, slug, provider, service_url, provider_model, dimensions, content_type, is_active, is_ready)
		VALUES ($1, $2, 'openai', 'http://localhost:11434/v1', 'text-embedding-3-small', 3, 'text', true, false)
	`, modelID, "repo-model-not-ready-"+uuid.NewString()); err != nil {
		t.Fatalf("insert model: %v", err)
	}

	repo := db.NewEmbeddingRepository(pool)
	err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "memory",
		ObjectID:   memory.ID,
		ScopeID:    scope.ID,
		ModelID:    modelID,
		Embedding:  []float32{1, 2, 3},
	})
	if err == nil {
		t.Fatal("expected model not ready error, got nil")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected not ready error, got: %v", err)
	}
}

func TestEmbeddingRepository_QuerySimilar_MemoryScopeFilter(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "owner-"+uuid.NewString())
	scopeA := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	memoryA := testhelper.CreateTestMemory(t, pool, scopeA.ID, owner.ID, "hello a")
	memoryB := testhelper.CreateTestMemory(t, pool, scopeB.ID, owner.ID, "hello b")

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "repo-model-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-small",
		Dimensions:    3,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	repo := db.NewEmbeddingRepository(pool)
	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "memory",
		ObjectID:   memoryA.ID,
		ScopeID:    scopeA.ID,
		ModelID:    model.ID,
		Embedding:  []float32{1, 0, 0},
	}); err != nil {
		t.Fatalf("UpsertEmbedding A: %v", err)
	}
	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "memory",
		ObjectID:   memoryB.ID,
		ScopeID:    scopeB.ID,
		ModelID:    model.ID,
		Embedding:  []float32{1, 0, 0},
	}); err != nil {
		t.Fatalf("UpsertEmbedding B: %v", err)
	}

	hits, err := repo.QuerySimilar(ctx, db.EmbeddingQuery{
		ModelID:    model.ID,
		ObjectType: "memory",
		Embedding:  []float32{1, 0, 0},
		Limit:      10,
		Scope: &db.ScopeFilter{
			ScopePath: scopeA.Path,
		},
	})
	if err != nil {
		t.Fatalf("QuerySimilar: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits len = %d, want 1", len(hits))
	}
	if hits[0].ObjectID != memoryA.ID {
		t.Fatalf("first hit object id = %s, want %s", hits[0].ObjectID, memoryA.ID)
	}
}

func TestEmbeddingRepository_QuerySimilar_KnowledgeScopeFilter(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "owner-"+uuid.NewString())
	scopeA := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	var artifactA, artifactB uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO knowledge_artifacts (knowledge_type, owner_scope_id, author_id, visibility, status, title, content)
		VALUES ('semantic', $1, $2, 'project', 'published', 'A', 'content a')
		RETURNING id
	`, scopeA.ID, owner.ID).Scan(&artifactA); err != nil {
		t.Fatalf("insert artifact A: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO knowledge_artifacts (knowledge_type, owner_scope_id, author_id, visibility, status, title, content)
		VALUES ('semantic', $1, $2, 'project', 'published', 'B', 'content b')
		RETURNING id
	`, scopeB.ID, owner.ID).Scan(&artifactB); err != nil {
		t.Fatalf("insert artifact B: %v", err)
	}

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "repo-model-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-small",
		Dimensions:    3,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	repo := db.NewEmbeddingRepository(pool)
	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "knowledge_artifact",
		ObjectID:   artifactA,
		ScopeID:    scopeA.ID,
		ModelID:    model.ID,
		Embedding:  []float32{1, 0, 0},
	}); err != nil {
		t.Fatalf("UpsertEmbedding A: %v", err)
	}
	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "knowledge_artifact",
		ObjectID:   artifactB,
		ScopeID:    scopeB.ID,
		ModelID:    model.ID,
		Embedding:  []float32{1, 0, 0},
	}); err != nil {
		t.Fatalf("UpsertEmbedding B: %v", err)
	}

	hits, err := repo.QuerySimilar(ctx, db.EmbeddingQuery{
		ModelID:    model.ID,
		ObjectType: "knowledge_artifact",
		Embedding:  []float32{1, 0, 0},
		Limit:      10,
		Scope: &db.ScopeFilter{
			ScopePath: scopeA.Path,
		},
	})
	if err != nil {
		t.Fatalf("QuerySimilar: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits len = %d, want 1", len(hits))
	}
	if hits[0].ObjectID != artifactA {
		t.Fatalf("first hit object id = %s, want %s", hits[0].ObjectID, artifactA)
	}
}

func TestEmbeddingRepository_QuerySimilar_SkillScopeFilter(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "owner-"+uuid.NewString())
	scopeA := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	scopeB := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	var skillA, skillB uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO skills (scope_id, author_id, slug, name, description, body)
		VALUES ($1, $2, $3, 'A', 'desc a', 'body a')
		RETURNING id
	`, scopeA.ID, owner.ID, "skill-a-"+uuid.NewString()).Scan(&skillA); err != nil {
		t.Fatalf("insert skill A: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO skills (scope_id, author_id, slug, name, description, body)
		VALUES ($1, $2, $3, 'B', 'desc b', 'body b')
		RETURNING id
	`, scopeB.ID, owner.ID, "skill-b-"+uuid.NewString()).Scan(&skillB); err != nil {
		t.Fatalf("insert skill B: %v", err)
	}

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "repo-model-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-small",
		Dimensions:    3,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	repo := db.NewEmbeddingRepository(pool)
	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "skill",
		ObjectID:   skillA,
		ScopeID:    scopeA.ID,
		ModelID:    model.ID,
		Embedding:  []float32{1, 0, 0},
	}); err != nil {
		t.Fatalf("UpsertEmbedding A: %v", err)
	}
	if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
		ObjectType: "skill",
		ObjectID:   skillB,
		ScopeID:    scopeB.ID,
		ModelID:    model.ID,
		Embedding:  []float32{1, 0, 0},
	}); err != nil {
		t.Fatalf("UpsertEmbedding B: %v", err)
	}

	hits, err := repo.QuerySimilar(ctx, db.EmbeddingQuery{
		ModelID:    model.ID,
		ObjectType: "skill",
		Embedding:  []float32{1, 0, 0},
		Limit:      10,
		Scope: &db.ScopeFilter{
			ScopePath: scopeA.Path,
		},
	})
	if err != nil {
		t.Fatalf("QuerySimilar: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits len = %d, want 1", len(hits))
	}
	if hits[0].ObjectID != skillA {
		t.Fatalf("first hit object id = %s, want %s", hits[0].ObjectID, skillA)
	}
}
