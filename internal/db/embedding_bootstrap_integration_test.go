//go:build integration

package db_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func makeVec(n int) []float32 {
	v := make([]float32, n)
	for i := range v {
		v[i] = float32(i%10) / 10
	}
	return v
}

func TestBootstrapLegacyEmbeddingsForModel_TextCopiesAllObjectTypes(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "bootstrap-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "bootstrap-scope-"+uuid.NewString(), nil, owner.ID)
	memory := testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "memory")

	var entityID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO entities (scope_id, entity_type, name, canonical)
		VALUES ($1, 'component', 'Auth', 'auth')
		RETURNING id
	`, scope.ID).Scan(&entityID); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	var artifactID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO knowledge_artifacts (knowledge_type, owner_scope_id, author_id, visibility, status, title, content)
		VALUES ('semantic', $1, $2, 'project', 'published', 'A', 'content')
		RETURNING id
	`, scope.ID, owner.ID).Scan(&artifactID); err != nil {
		t.Fatalf("insert artifact: %v", err)
	}
	var skillID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO skills (scope_id, author_id, slug, name, description, body)
		VALUES ($1, $2, $3, 'S', 'desc', 'body')
		RETURNING id
	`, scope.ID, owner.ID, "skill-"+uuid.NewString()).Scan(&skillID); err != nil {
		t.Fatalf("insert skill: %v", err)
	}

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "bootstrap-text-" + uuid.NewString(),
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

	vecLit := db.ExportFloat32SliceToVector(makeVec(4))
	for _, q := range []string{
		`UPDATE memories SET embedding = $1::vector, embedding_model_id = $2 WHERE id = $3`,
		`UPDATE entities SET embedding = $1::vector, embedding_model_id = $2 WHERE id = $3`,
		`UPDATE knowledge_artifacts SET embedding = $1::vector, embedding_model_id = $2 WHERE id = $3`,
		`UPDATE skills SET embedding = $1::vector, embedding_model_id = $2 WHERE id = $3`,
	} {
		var id uuid.UUID
		switch q {
		case `UPDATE memories SET embedding = $1::vector, embedding_model_id = $2 WHERE id = $3`:
			id = memory.ID
		case `UPDATE entities SET embedding = $1::vector, embedding_model_id = $2 WHERE id = $3`:
			id = entityID
		case `UPDATE knowledge_artifacts SET embedding = $1::vector, embedding_model_id = $2 WHERE id = $3`:
			id = artifactID
		default:
			id = skillID
		}
		if _, err := pool.Exec(ctx, q, vecLit, model.ID, id); err != nil {
			t.Fatalf("seed legacy embedding (%s): %v", q, err)
		}
	}

	stats, err := db.BootstrapLegacyEmbeddingsForModel(ctx, pool, model.ID)
	if err != nil {
		t.Fatalf("BootstrapLegacyEmbeddingsForModel: %v", err)
	}
	if stats.UpsertedRows < 4 {
		t.Fatalf("UpsertedRows = %d, want >= 4", stats.UpsertedRows)
	}

	for _, tc := range []struct {
		objectType string
		id         uuid.UUID
	}{
		{"memory", memory.ID},
		{"entity", entityID},
		{"knowledge_artifact", artifactID},
		{"skill", skillID},
	} {
		var status string
		err := pool.QueryRow(ctx, `
			SELECT status FROM embedding_index
			WHERE object_type = $1 AND object_id = $2 AND model_id = $3
		`, tc.objectType, tc.id, model.ID).Scan(&status)
		if err != nil {
			t.Fatalf("embedding_index status (%s): %v", tc.objectType, err)
		}
		if status != "ready" {
			t.Fatalf("status (%s) = %q, want ready", tc.objectType, status)
		}
	}

	var tableName string
	if err := pool.QueryRow(ctx, `SELECT table_name FROM ai_models WHERE id=$1`, model.ID).Scan(&tableName); err != nil {
		t.Fatalf("load table_name: %v", err)
	}
	for _, tc := range []struct {
		objectType string
		id         uuid.UUID
	}{
		{"memory", memory.ID},
		{"entity", entityID},
		{"knowledge_artifact", artifactID},
		{"skill", skillID},
	} {
		var exists bool
		err := pool.QueryRow(ctx, fmt.Sprintf(`
			SELECT EXISTS(SELECT 1 FROM %s WHERE object_type = $1 AND object_id = $2)
		`, tableName), tc.objectType, tc.id).Scan(&exists)
		if err != nil {
			t.Fatalf("model table existence (%s): %v", tc.objectType, err)
		}
		if !exists {
			t.Fatalf("missing model table row for %s/%s", tc.objectType, tc.id)
		}
	}
}

func TestBootstrapLegacyEmbeddingsForModel_CodeUsesEmbeddingCode(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "bootstrap-code-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "bootstrap-code-scope-"+uuid.NewString(), nil, owner.ID)
	memory := testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "package main")

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "bootstrap-code-" + uuid.NewString(),
		Provider:      "openai",
		ServiceURL:    "http://localhost:11434/v1",
		ProviderModel: "text-embedding-3-small",
		Dimensions:    4,
		ContentType:   "code",
		Activate:      true,
	})
	if err != nil {
		t.Fatalf("register model: %v", err)
	}

	vecLit := db.ExportFloat32SliceToVector(makeVec(4))
	if _, err := pool.Exec(ctx, `
		UPDATE memories
		SET content_kind = 'code', embedding_code = $1::vector, embedding_code_model_id = $2
		WHERE id = $3
	`, vecLit, model.ID, memory.ID); err != nil {
		t.Fatalf("seed code embedding: %v", err)
	}

	stats, err := db.BootstrapLegacyEmbeddingsForModel(ctx, pool, model.ID)
	if err != nil {
		t.Fatalf("BootstrapLegacyEmbeddingsForModel: %v", err)
	}
	if stats.UpsertedRows < 1 {
		t.Fatalf("UpsertedRows = %d, want >= 1", stats.UpsertedRows)
	}

	var status string
	err = pool.QueryRow(ctx, `
		SELECT status FROM embedding_index
		WHERE object_type = 'memory' AND object_id = $1 AND model_id = $2
	`, memory.ID, model.ID).Scan(&status)
	if err != nil {
		t.Fatalf("embedding_index status: %v", err)
	}
	if status != "ready" {
		t.Fatalf("status = %q, want ready", status)
	}
}

func TestBootstrapLegacyEmbeddingsForModel_SecondRunSkipsReadyRows(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "bootstrap-resume-user-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "bootstrap-resume-scope-"+uuid.NewString(), nil, owner.ID)
	memory := testhelper.CreateTestMemory(t, pool, scope.ID, owner.ID, "hello")

	model, err := db.RegisterEmbeddingModel(ctx, pool, db.RegisterEmbeddingModelParams{
		Slug:          "bootstrap-resume-" + uuid.NewString(),
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

	vecLit := db.ExportFloat32SliceToVector(makeVec(4))
	if _, err := pool.Exec(ctx, `
		UPDATE memories SET embedding = $1::vector, embedding_model_id = $2 WHERE id = $3
	`, vecLit, model.ID, memory.ID); err != nil {
		t.Fatalf("seed legacy embedding: %v", err)
	}

	first, err := db.BootstrapLegacyEmbeddingsForModel(ctx, pool, model.ID)
	if err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	if first.UpsertedRows < 1 || first.IndexedRows < 1 {
		t.Fatalf("first stats = %+v, want at least 1 upsert/index", first)
	}

	second, err := db.BootstrapLegacyEmbeddingsForModel(ctx, pool, model.ID)
	if err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	if second.UpsertedRows != 0 {
		t.Fatalf("second UpsertedRows = %d, want 0", second.UpsertedRows)
	}
	if second.IndexedRows != 0 {
		t.Fatalf("second IndexedRows = %d, want 0", second.IndexedRows)
	}
}
