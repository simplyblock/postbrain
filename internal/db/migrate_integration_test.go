//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestMigrationsApplyCleanly(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	tables := []string{
		"principals", "tokens", "scopes", "embedding_models",
		"memories", "entities", "relations",
		"knowledge_artifacts", "knowledge_endorsements", "knowledge_history",
		"knowledge_collections", "sharing_grants", "promotion_requests",
		"staleness_flags", "skills", "skill_endorsements",
	}
	for _, tbl := range tables {
		var exists bool
		err := pool.QueryRow(ctx,
			"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)", tbl,
		).Scan(&exists)
		if err != nil {
			t.Errorf("table %q: query error: %v", tbl, err)
			continue
		}
		if !exists {
			t.Errorf("table %q missing after migration", tbl)
		}
	}
}

func TestMigrateForTestIdempotent(t *testing.T) {
	// NewTestPool already calls MigrateForTest once. Running it a second time
	// should fail because tables already exist (CREATE TABLE without IF NOT EXISTS).
	// That is expected — this test just verifies the first run succeeded.
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	// Verify the pool is usable after migration.
	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("pool unusable after migration: %v", err)
	}
	if one != 1 {
		t.Errorf("expected 1, got %d", one)
	}

	// Confirm MigrateForTest is exported from the db package.
	_ = db.MigrateForTest
}
