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

func TestRegisterEmbeddingModel_CreatesTableAndPendingRows(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "owner-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "memory content")

	if _, err := pool.Exec(ctx, `
		INSERT INTO entities (scope_id, entity_type, name, canonical)
		VALUES ($1, 'component', 'Auth', 'auth')
	`, scope.ID); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO knowledge_artifacts (knowledge_type, owner_scope_id, author_id, visibility, status, title, content)
		VALUES ('semantic', $1, $2, 'project', 'published', 'Artifact', 'Artifact content')
	`, scope.ID, owner.ID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO skills (scope_id, author_id, slug, name, description, body)
		VALUES ($1, $2, $3, 'Skill', 'Skill desc', 'Skill body')
	`, scope.ID, owner.ID, "skill-"+uuid.NewString()); err != nil {
		t.Fatalf("insert skill: %v", err)
	}

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "text-model-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-large",
		Dimensions:    1536,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("RegisterEmbeddingModel: %v", err)
	}

	if model == nil {
		t.Fatal("RegisterEmbeddingModel returned nil model")
	}

	var tableName string
	var isReady bool
	err = pool.QueryRow(ctx, `
		SELECT table_name, is_ready FROM ai_models WHERE id = $1
	`, model.ID).Scan(&tableName, &isReady)
	if err != nil {
		t.Fatalf("load model metadata: %v", err)
	}
	if tableName == "" {
		t.Fatal("table_name was empty")
	}
	if !isReady {
		t.Fatal("is_ready = false, want true")
	}

	var counts = map[string]int{}
	rows, err := pool.Query(ctx, `
		SELECT object_type, COUNT(*)
		FROM embedding_index
		WHERE model_id = $1
		GROUP BY object_type
	`, model.ID)
	if err != nil {
		t.Fatalf("query embedding_index: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var objectType string
		var count int
		if err := rows.Scan(&objectType, &count); err != nil {
			t.Fatalf("scan count row: %v", err)
		}
		counts[objectType] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	for _, objectType := range []string{"memory", "entity", "knowledge_artifact", "skill"} {
		if got := counts[objectType]; got != 1 {
			t.Fatalf("pending rows for %s = %d, want 1", objectType, got)
		}
	}
}

func TestRegisterEmbeddingModel_ProviderConfig_DefaultAndOverride(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	defaultModel, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "text-model-default-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-large",
		Dimensions:    1536,
		ContentType:   "text",
		Activate:      false,
	})
	if err != nil {
		t.Fatalf("RegisterEmbeddingModel default provider_config: %v", err)
	}

	var providerConfig string
	if err := pool.QueryRow(ctx, `SELECT provider_config FROM ai_models WHERE id = $1`, defaultModel.ID).Scan(&providerConfig); err != nil {
		t.Fatalf("load default provider_config: %v", err)
	}
	if providerConfig != "default" {
		t.Fatalf("default provider_config = %q, want default", providerConfig)
	}

	customModel, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:           "text-model-custom-" + uuid.NewString(),
		Provider:       "openai",
		ServiceURL:     "https://api.openai.com/v1",
		ProviderModel:  "text-embedding-3-large",
		ProviderConfig: "openai-prod",
		Dimensions:     1536,
		ContentType:    "text",
		Activate:       false,
	})
	if err != nil {
		t.Fatalf("RegisterEmbeddingModel custom provider_config: %v", err)
	}

	if err := pool.QueryRow(ctx, `SELECT provider_config FROM ai_models WHERE id = $1`, customModel.ID).Scan(&providerConfig); err != nil {
		t.Fatalf("load custom provider_config: %v", err)
	}
	if providerConfig != "openai-prod" {
		t.Fatalf("custom provider_config = %q, want openai-prod", providerConfig)
	}
}

func TestRegisterEmbeddingModel_IdempotentOnSlug(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "owner-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "scope-"+uuid.NewString(), nil, owner.ID)
	testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "memory content")

	slug := "text-model-" + uuid.NewString()
	params := db.RegisterEmbeddingModelParams{
		Slug:          slug,
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-large",
		Dimensions:    1536,
		ContentType:   "text",
		Activate:      true,
	}

	first, err := db.RegisterEmbeddingModel(ctx, pool, params)
	if err != nil {
		t.Fatalf("first RegisterEmbeddingModel: %v", err)
	}
	second, err := db.RegisterEmbeddingModel(ctx, pool, params)
	if err != nil {
		t.Fatalf("second RegisterEmbeddingModel: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("model IDs differ: first=%s second=%s", first.ID, second.ID)
	}

	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM embedding_index WHERE model_id = $1 AND object_type = 'memory'
	`, first.ID).Scan(&count)
	if err != nil {
		t.Fatalf("count embedding_index rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("memory embedding_index rows = %d, want 1", count)
	}
}

func TestRegisterEmbeddingModel_RollsBackOnProvisionFailure(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	var modelID uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO ai_models (slug, provider, service_url, provider_model, dimensions, content_type, model_type, is_active, is_ready)
		VALUES ('conflict-model', 'openai', 'http://localhost:11434/v1', 'text-embedding-3-large', 8, 'text', 'embedding', false, true)
		RETURNING id
	`).Scan(&modelID)
	if err != nil {
		t.Fatalf("insert seed embedding model: %v", err)
	}
	if _, err := db.EnsureEmbeddingModelTable(ctx, pool, modelID, 8); err != nil {
		t.Fatalf("EnsureEmbeddingModelTable: %v", err)
	}

	_, err = db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "conflict-model",
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-large",
		Dimensions:    16,
		ContentType:   "text",
		Activate:      false,
	})
	if err == nil {
		t.Fatal("expected dimension mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "dimension") {
		t.Fatalf("expected dimension error, got: %v", err)
	}

	var dimensions int
	err = pool.QueryRow(ctx, `SELECT dimensions FROM ai_models WHERE id = $1`, modelID).Scan(&dimensions)
	if err != nil {
		t.Fatalf("load model dimensions: %v", err)
	}
	if dimensions != 8 {
		t.Fatalf("dimensions after rollback = %d, want 8", dimensions)
	}
}

func TestRegisterEmbeddingModel_GenerationModelTypeSkipsEmbeddingTableProvisioning(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "summary-model-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "https://api.openai.com/v1",
		ProviderModel: "gpt-4o-mini",
		Dimensions:    1536,
		ContentType:   "text",
		ModelType:     "generation",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("RegisterEmbeddingModel: %v", err)
	}

	var modelType string
	var tableName *string
	var isReady bool
	if err := pool.QueryRow(ctx, `
		SELECT model_type, table_name, is_ready
		FROM ai_models
		WHERE id = $1
	`, model.ID).Scan(&modelType, &tableName, &isReady); err != nil {
		t.Fatalf("load model metadata: %v", err)
	}
	if modelType != "generation" {
		t.Fatalf("model_type = %q, want generation", modelType)
	}
	if tableName != nil && *tableName != "" {
		t.Fatalf("table_name = %q, want empty for generation model", *tableName)
	}
	if !isReady {
		t.Fatal("is_ready = false, want true")
	}

	var idxRows int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM embedding_index
		WHERE model_id = $1
	`, model.ID).Scan(&idxRows); err != nil {
		t.Fatalf("count embedding_index rows: %v", err)
	}
	if idxRows != 0 {
		t.Fatalf("embedding_index rows = %d, want 0 for generation model", idxRows)
	}
}
