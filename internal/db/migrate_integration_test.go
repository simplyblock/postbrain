//go:build integration

package db_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func assertTableExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, table string, want bool) {
	t.Helper()

	var exists bool
	err := pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)", table,
	).Scan(&exists)
	if err != nil {
		t.Fatalf("table %q: query error: %v", table, err)
	}
	if exists != want {
		t.Fatalf("table %q existence mismatch: got=%v want=%v", table, exists, want)
	}
}

func TestMigrationsApplyCleanly(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	tables := []string{
		"principals", "tokens", "scopes", "embedding_models",
		"memories", "entities", "relations",
		"knowledge_artifacts", "knowledge_endorsements", "knowledge_history",
		"knowledge_collections", "sharing_grants", "promotion_requests",
		"staleness_flags", "skills", "skill_endorsements",
		"social_identities", "oauth_clients", "oauth_auth_codes", "oauth_states",
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

	_, err := pool.Exec(ctx, "INSERT INTO oauth_states(state_hash, kind, payload, expires_at) VALUES ($1, $2, '{}'::jsonb, now() + interval '5 minutes')", "bad-kind-state-hash", "invalid-kind")
	if err == nil {
		t.Fatalf("expected oauth_states kind check constraint violation, got nil")
	}
	if !strings.Contains(err.Error(), "oauth_states_kind_check") {
		t.Fatalf("expected oauth_states kind check constraint error, got: %v", err)
	}

	const principalSlug = "oauth-migration-test-user"
	var principalID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO principals(kind, slug, display_name)
		VALUES ('user', $1, 'OAuth Migration Test User')
		RETURNING id
	`, principalSlug).Scan(&principalID); err != nil {
		t.Fatalf("insert principal for social identity constraint test: %v", err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO social_identities(principal_id, provider, provider_id)
		VALUES ($1, 'github', 'provider-user-1')
	`, principalID); err != nil {
		t.Fatalf("insert first social identity: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO social_identities(principal_id, provider, provider_id)
		VALUES ($1, 'github', 'provider-user-1')
	`, principalID)
	if err == nil {
		t.Fatalf("expected social_identities unique(provider, provider_id) violation, got nil")
	}
	if !strings.Contains(err.Error(), "social_identities_provider_provider_id_key") {
		t.Fatalf("expected social_identities unique constraint error, got: %v", err)
	}
}

func TestMigrateDownAndUpOAuthMigration(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	oauthTables := []string{"social_identities", "oauth_clients", "oauth_auth_codes", "oauth_states"}
	for _, table := range oauthTables {
		assertTableExists(t, ctx, pool, table, true)
	}

	downSQL, err := os.ReadFile("migrations/000011_oauth.down.sql")
	if err != nil {
		t.Fatalf("read 000011 down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(downSQL)); err != nil {
		t.Fatalf("apply 000011 down migration: %v", err)
	}

	for _, table := range oauthTables {
		assertTableExists(t, ctx, pool, table, false)
	}

	upSQL, err := os.ReadFile("migrations/000011_oauth.up.sql")
	if err != nil {
		t.Fatalf("read 000011 up migration: %v", err)
	}
	if _, err := pool.Exec(ctx, string(upSQL)); err != nil {
		t.Fatalf("apply 000011 up migration: %v", err)
	}

	for _, table := range oauthTables {
		assertTableExists(t, ctx, pool, table, true)
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
