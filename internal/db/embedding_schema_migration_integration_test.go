//go:build integration

package db_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func assertColumnExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table, column string, want bool) {
	t.Helper()
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = $1 AND column_name = $2
		)`, table, column,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("column %q.%q: query error: %v", table, column, err)
	}
	if exists != want {
		t.Fatalf("column %q.%q existence: got=%v want=%v", table, column, exists, want)
	}
}

// TestEmbeddingIndexTableExists verifies the embedding_index table is created
// with the correct columns and primary key.
func TestEmbeddingIndexTableExists(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	assertTableExists(t, ctx, pool, "embedding_index", true)

	for _, col := range []string{
		"object_type", "object_id", "model_id",
		"status", "retry_count", "last_error",
		"created_at", "updated_at",
	} {
		assertColumnExists(t, ctx, pool, "embedding_index", col, true)
	}
}

// TestEmbeddingIndexStatusConstraint verifies the status CHECK constraint
// rejects values outside ('pending', 'ready', 'failed').
func TestEmbeddingIndexStatusConstraint(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	// Insert a valid ai_models row to satisfy the FK.
	var modelID string
	err := pool.QueryRow(ctx, `
		INSERT INTO ai_models (slug, provider, provider_model, service_url, dimensions, content_type, model_type, is_active, is_ready)
		VALUES ('test-model-status', 'ollama', 'nomic-embed-text', 'http://localhost:11434', 768, 'text', 'embedding', false, true)
		RETURNING id
	`).Scan(&modelID)
	if err != nil {
		t.Fatalf("insert embedding_model: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status)
		VALUES ('memory', uuidv7(), $1, 'invalid_status')
	`, modelID)
	if err == nil {
		t.Fatal("expected status check constraint violation, got nil")
	}
	if !strings.Contains(err.Error(), "embedding_index_status_check") {
		t.Fatalf("expected embedding_index_status_check error, got: %v", err)
	}
}

// TestEmbeddingIndexObjectTypeConstraint verifies the object_type CHECK constraint
// rejects values outside ('memory', 'entity', 'knowledge_artifact', 'skill').
func TestEmbeddingIndexObjectTypeConstraint(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	var modelID string
	err := pool.QueryRow(ctx, `
		INSERT INTO ai_models (slug, provider, provider_model, service_url, dimensions, content_type, model_type, is_active, is_ready)
		VALUES ('test-model-objtype', 'ollama', 'nomic-embed-text', 'http://localhost:11434', 768, 'text', 'embedding', false, true)
		RETURNING id
	`).Scan(&modelID)
	if err != nil {
		t.Fatalf("insert embedding_model: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id, status)
		VALUES ('unknown_type', uuidv7(), $1, 'pending')
	`, modelID)
	if err == nil {
		t.Fatal("expected object_type check constraint violation, got nil")
	}
	if !strings.Contains(err.Error(), "embedding_index_object_type_check") {
		t.Fatalf("expected embedding_index_object_type_check error, got: %v", err)
	}
}

// TestEmbeddingIndexDefaultStatus verifies that status defaults to 'pending'
// and retry_count defaults to 0.
func TestEmbeddingIndexDefaults(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	var modelID string
	err := pool.QueryRow(ctx, `
		INSERT INTO ai_models (slug, provider, provider_model, service_url, dimensions, content_type, model_type, is_active, is_ready)
		VALUES ('test-model-defaults', 'ollama', 'nomic-embed-text', 'http://localhost:11434', 768, 'text', 'embedding', false, true)
		RETURNING id
	`).Scan(&modelID)
	if err != nil {
		t.Fatalf("insert embedding_model: %v", err)
	}

	objID := "018f1e2a-3b4c-7d8e-9f0a-1b2c3d4e5f60"
	_, err = pool.Exec(ctx, `
		INSERT INTO embedding_index (object_type, object_id, model_id)
		VALUES ('memory', $1, $2)
	`, objID, modelID)
	if err != nil {
		t.Fatalf("insert embedding_index without status: %v", err)
	}

	var status string
	var retryCount int
	err = pool.QueryRow(ctx, `
		SELECT status, retry_count FROM embedding_index
		WHERE object_type = 'memory' AND object_id = $1 AND model_id = $2
	`, objID, modelID).Scan(&status, &retryCount)
	if err != nil {
		t.Fatalf("select defaults: %v", err)
	}
	if status != "pending" {
		t.Errorf("default status: got=%q want=%q", status, "pending")
	}
	if retryCount != 0 {
		t.Errorf("default retry_count: got=%d want=%d", retryCount, 0)
	}
}

// TestAIModelsNewColumns verifies the provider-related columns
// exist on ai_models.
func TestAIModelsNewColumns(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	for _, col := range []string{"provider", "service_url", "provider_model", "provider_config", "table_name", "is_ready"} {
		assertColumnExists(t, ctx, pool, "ai_models", col, true)
	}
}

// TestAIModelsSlugRetained verifies the slug column is retained
// as the unique operator-assigned identifier.
func TestAIModelsSlugRetained(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	assertColumnExists(t, ctx, pool, "ai_models", "slug", true)
}

// TestAIModelsActiveIndexesPreserved verifies the partial unique indexes
// enforcing one active model per content_type are still present.
func TestAIModelsActiveIndexesPreserved(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	assertIndexExists(t, ctx, pool, "ai_models_active_embedding_text_idx", true)
	assertIndexExists(t, ctx, pool, "ai_models_active_embedding_code_idx", true)
	assertIndexExists(t, ctx, pool, "ai_models_provider_config_idx", true)
}

// TestEmbeddingModelIDColumnsRetained verifies that the legacy embedding_model_id
// FK columns are retained during the compatibility phase.
func TestEmbeddingModelIDColumnsRetained(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	cases := []struct{ table, column string }{
		{"memories", "embedding_model_id"},
		{"memories", "embedding_code_model_id"},
		{"entities", "embedding_model_id"},
		{"knowledge_artifacts", "embedding_model_id"},
		{"skills", "embedding_model_id"},
	}
	for _, c := range cases {
		assertColumnExists(t, ctx, pool, c.table, c.column, true)
	}
}

// TestLegacyEmbeddingVectorColumnsRetained verifies that the legacy vector
// columns are still present during the transition period (dropped in Step 11).
func TestLegacyEmbeddingVectorColumnsRetained(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	cases := []struct{ table, column string }{
		{"memories", "embedding"},
		{"memories", "embedding_code"},
		{"entities", "embedding"},
		{"knowledge_artifacts", "embedding"},
		{"skills", "embedding"},
	}
	for _, c := range cases {
		assertColumnExists(t, ctx, pool, c.table, c.column, true)
	}
}
