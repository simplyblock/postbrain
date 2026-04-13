//go:build integration

package db_test

import (
	"context"
	"fmt"
	"testing"

	pgvector "github.com/pgvector/pgvector-go"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestUpsertEntity_DualWritesToEmbeddingRepository(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "entity-dual-user")
	scope := testhelper.CreateTestScope(t, pool, "project", "entity/dual", nil, owner.ID)

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "entity-dual-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-small",
		Dimensions:    4,
		ContentType:   "text",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	vec := pgvector.NewVector([]float32{0.1, 0.2, 0.3, 0.4})
	entity, err := compat.UpsertEntity(ctx, pool, &db.Entity{
		ScopeID:          scope.ID,
		EntityType:       "component",
		Name:             "Auth",
		Canonical:        "auth",
		Embedding:        &vec,
		EmbeddingModelID: &model.ID,
	})
	if err != nil {
		t.Fatalf("UpsertEntity: %v", err)
	}

	var status string
	err = pool.QueryRow(ctx, `
		SELECT status FROM embedding_index
		WHERE object_type = 'entity' AND object_id = $1 AND model_id = $2
	`, entity.ID, model.ID).Scan(&status)
	if err != nil {
		t.Fatalf("select embedding_index row: %v", err)
	}
	if status != "ready" {
		t.Fatalf("embedding_index status = %q, want ready", status)
	}

	var tableName string
	err = pool.QueryRow(ctx, `SELECT table_name FROM ai_models WHERE id=$1`, model.ID).Scan(&tableName)
	if err != nil {
		t.Fatalf("select model table name: %v", err)
	}
	var exists bool
	err = pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM %s WHERE object_type='entity' AND object_id=$1
		)
	`, tableName), entity.ID).Scan(&exists)
	if err != nil {
		t.Fatalf("select model table row: %v", err)
	}
	if !exists {
		t.Fatal("expected dual-write row in model table")
	}
}
