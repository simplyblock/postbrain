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

func TestEnsureEmbeddingModelTable_CreatesAndIsIdempotent(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	modelID := uuid.New()

	tableName, err := db.EnsureEmbeddingModelTable(ctx, pool, modelID, 1536)
	if err != nil {
		t.Fatalf("EnsureEmbeddingModelTable first call: %v", err)
	}
	if tableName == "" {
		t.Fatal("EnsureEmbeddingModelTable returned empty table name")
	}

	if _, err := db.EnsureEmbeddingModelTable(ctx, pool, modelID, 1536); err != nil {
		t.Fatalf("EnsureEmbeddingModelTable second call: %v", err)
	}

	var exists bool
	err = pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)`, tableName,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("table existence query: %v", err)
	}
	if !exists {
		t.Fatalf("expected table %q to exist", tableName)
	}

	var typ string
	err = pool.QueryRow(ctx, `
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = $1 AND a.attname = 'embedding' AND a.attnum > 0 AND NOT a.attisdropped
	`, tableName).Scan(&typ)
	if err != nil {
		t.Fatalf("embedding column type query: %v", err)
	}
	if !strings.Contains(typ, "vector(1536)") {
		t.Fatalf("embedding column type = %q, want vector(1536)", typ)
	}
}

func TestEnsureEmbeddingModelTable_UsesHalfvecForHighDimensions(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()
	modelID := uuid.New()

	tableName, err := db.EnsureEmbeddingModelTable(ctx, pool, modelID, 2560)
	if err != nil {
		t.Fatalf("EnsureEmbeddingModelTable first call: %v", err)
	}
	if _, err := db.EnsureEmbeddingModelTable(ctx, pool, modelID, 2560); err != nil {
		t.Fatalf("EnsureEmbeddingModelTable second call: %v", err)
	}

	var typ string
	err = pool.QueryRow(ctx, `
		SELECT format_type(a.atttypid, a.atttypmod)
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		WHERE c.relname = $1 AND a.attname = 'embedding' AND a.attnum > 0 AND NOT a.attisdropped
	`, tableName).Scan(&typ)
	if err != nil {
		t.Fatalf("embedding column type query: %v", err)
	}
	if !strings.Contains(typ, "vector(2560)") {
		t.Fatalf("embedding column type = %q, want vector(2560)", typ)
	}

	var indexDef string
	err = pool.QueryRow(ctx, `
		SELECT indexdef
		FROM pg_indexes
		WHERE schemaname = 'public' AND tablename = $1 AND indexname = $2
	`, tableName, "embm_"+strings.ReplaceAll(modelID.String(), "-", "")[:16]+"_hnsw").Scan(&indexDef)
	if err != nil {
		t.Fatalf("hnsw index query: %v", err)
	}
	if !strings.Contains(indexDef, "halfvec_cosine_ops") {
		t.Fatalf("hnsw index definition = %q, want halfvec_cosine_ops", indexDef)
	}
	if !strings.Contains(indexDef, "embedding") || !strings.Contains(indexDef, "halfvec(2560)") {
		t.Fatalf("hnsw index definition = %q, want embedding cast to halfvec(2560)", indexDef)
	}
}
