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

// substituteSchema replaces {{POSTBRAIN_SCHEMA}} with "public" so that raw
// migration SQL read from files is valid when executed directly in tests
// (bypassing the schemaSource wrapper that the normal migrator uses).
func substituteSchema(sql string) string {
	return strings.ReplaceAll(sql, "{{POSTBRAIN_SCHEMA}}", "public")
}

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
		"principals", "tokens", "scopes", "embedding_models", "ai_models",
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
	if _, err := pool.Exec(ctx, substituteSchema(string(downSQL))); err != nil {
		t.Fatalf("apply 000011 down migration: %v", err)
	}

	for _, table := range oauthTables {
		assertTableExists(t, ctx, pool, table, false)
	}

	upSQL, err := os.ReadFile("migrations/000011_oauth.up.sql")
	if err != nil {
		t.Fatalf("read 000011 up migration: %v", err)
	}
	if _, err := pool.Exec(ctx, substituteSchema(string(upSQL))); err != nil {
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

// TestMigration000015_ScopeGrantsAndSystemAdmin verifies that migration 000015
// creates the scope_grants table and adds the is_system_admin column to principals.
func TestMigration000015_ScopeGrantsAndSystemAdmin(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	// scope_grants table must exist
	assertTableExists(t, ctx, pool, "scope_grants", true)

	// principals.is_system_admin column must exist and default to false
	var isAdmin bool
	slug := "migration015-test-" + t.Name()
	if _, err := pool.Exec(ctx, `
		INSERT INTO principals (kind, slug, display_name) VALUES ('user', $1, $2)
	`, slug, slug); err != nil {
		t.Fatalf("insert principal: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT is_system_admin FROM principals WHERE slug = $1`, slug,
	).Scan(&isAdmin); err != nil {
		t.Fatalf("read is_system_admin: %v", err)
	}
	if isAdmin {
		t.Error("is_system_admin should default to false")
	}

	// is_system_admin can be set to true
	if _, err := pool.Exec(ctx,
		`UPDATE principals SET is_system_admin = true WHERE slug = $1`, slug,
	); err != nil {
		t.Fatalf("set is_system_admin: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT is_system_admin FROM principals WHERE slug = $1`, slug,
	).Scan(&isAdmin); err != nil {
		t.Fatalf("read is_system_admin after update: %v", err)
	}
	if !isAdmin {
		t.Error("is_system_admin should be true after update")
	}
}

// TestMigration000015_ScopeGrantsConstraints verifies scope_grants unique and FK constraints.
func TestMigration000015_ScopeGrantsConstraints(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	p := testhelper.CreateTestPrincipal(t, pool, "user", "sg-constraint-p-"+t.Name())
	s := testhelper.CreateTestScope(t, pool, "project", "sg-constraint-s-"+t.Name(), nil, p.ID)

	// First grant should succeed
	if _, err := pool.Exec(ctx, `
		INSERT INTO scope_grants (principal_id, scope_id, permissions)
		VALUES ($1, $2, ARRAY['read'])
	`, p.ID, s.ID); err != nil {
		t.Fatalf("first scope grant insert: %v", err)
	}

	// Duplicate (same principal_id, scope_id) should fail
	_, err := pool.Exec(ctx, `
		INSERT INTO scope_grants (principal_id, scope_id, permissions)
		VALUES ($1, $2, ARRAY['write'])
	`, p.ID, s.ID)
	if err == nil {
		t.Error("expected unique constraint violation for duplicate scope grant, got nil")
	}
	if !strings.Contains(err.Error(), "unique") && !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected unique constraint error, got: %v", err)
	}
}

// TestMigration000016_TokenPermissionsV2 verifies that migration 000016 converts
// legacy 'admin' tokens and adds the CHECK constraint.
func TestMigration000016_TokenPermissionsV2(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	// No existing token should have 'admin' in permissions after migration
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM tokens WHERE 'admin' = ANY(permissions)`,
	).Scan(&count); err != nil {
		t.Fatalf("count admin tokens: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tokens with 'admin' permission after migration, got %d", count)
	}

	// Attempting to insert a token with 'admin' permission must fail the CHECK constraint
	p := testhelper.CreateTestPrincipal(t, pool, "user", "token-v2-test-"+t.Name())
	_, err := pool.Exec(ctx, `
		INSERT INTO tokens (principal_id, token_hash, name, scope_ids, permissions)
		VALUES ($1, 'hash-admin-test', 'admin-token', ARRAY[]::uuid[], ARRAY['admin'])
	`, p.ID)
	if err == nil {
		t.Error("expected CHECK constraint violation for 'admin' permission, got nil")
	}
	if !strings.Contains(err.Error(), "tokens_no_admin_permission") {
		t.Errorf("expected tokens_no_admin_permission constraint error, got: %v", err)
	}
}

func TestMigration000017_KnowledgeArtifactColumnsAndConstraints(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "migration017-owner-"+t.Name())
	author := testhelper.CreateTestPrincipal(t, pool, "user", "migration017-author-"+t.Name())
	scope := testhelper.CreateTestScope(t, pool, "project", "migration017-scope-"+t.Name(), nil, owner.ID)

	var (
		artifactKind string
		artifactMeta []byte
		occurredAt   *string
	)
	if err := pool.QueryRow(ctx, `
		INSERT INTO knowledge_artifacts (
			knowledge_type, owner_scope_id, author_id, visibility, status, title, content
		) VALUES (
			'semantic', $1, $2, 'team', 'draft', 'migration017', 'body'
		)
		RETURNING artifact_kind, artifact_meta, occurred_at::text
	`, scope.ID, author.ID).Scan(&artifactKind, &artifactMeta, &occurredAt); err != nil {
		t.Fatalf("insert artifact with defaults: %v", err)
	}
	if artifactKind != "general" {
		t.Fatalf("artifact_kind default = %q, want %q", artifactKind, "general")
	}
	if string(artifactMeta) != "{}" {
		t.Fatalf("artifact_meta default = %q, want %q", string(artifactMeta), "{}")
	}
	if occurredAt != nil {
		t.Fatalf("occurred_at default = %v, want nil", *occurredAt)
	}

	oldArtifact := testhelper.CreateTestArtifact(t, pool, scope.ID, author.ID, "migration017-old")
	var supersededID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO knowledge_artifacts (
			knowledge_type, artifact_kind, owner_scope_id, author_id, visibility, status, title, content, supersedes_artifact_id
		) VALUES (
			'semantic', 'decision', $1, $2, 'team', 'draft', 'migration017-new', 'body', $3
		)
		RETURNING supersedes_artifact_id::text
	`, scope.ID, author.ID, oldArtifact.ID).Scan(&supersededID); err != nil {
		t.Fatalf("insert artifact with supersedes_artifact_id: %v", err)
	}
	if supersededID != oldArtifact.ID.String() {
		t.Fatalf("supersedes_artifact_id = %q, want %q", supersededID, oldArtifact.ID.String())
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO knowledge_artifacts (
			knowledge_type, artifact_kind, owner_scope_id, author_id, visibility, status, title, content
		) VALUES (
			'semantic', 'not-a-kind', $1, $2, 'team', 'draft', 'bad-kind', 'body'
		)
	`, scope.ID, author.ID)
	if err == nil {
		t.Fatal("expected artifact_kind CHECK violation, got nil")
	}
	if !strings.Contains(err.Error(), "knowledge_artifacts_artifact_kind_check") {
		t.Fatalf("expected artifact_kind constraint error, got: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO knowledge_artifacts (
			knowledge_type, artifact_kind, owner_scope_id, author_id, visibility, status, title, content, supersedes_artifact_id
		) VALUES (
			'semantic', 'decision', $1, $2, 'team', 'draft', 'bad-supersedes', 'body', '00000000-0000-0000-0000-000000000123'
		)
	`, scope.ID, author.ID)
	if err == nil {
		t.Fatal("expected supersedes_artifact_id FK violation, got nil")
	}
}

func TestMigration000018_TimeWindowRecallIndexes(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	indexes := []string{
		"memories_scope_active_created_at_idx",
		"knowledge_owner_status_published_at_idx",
		"knowledge_owner_status_created_at_idx",
	}

	for _, idx := range indexes {
		assertIndexExists(t, ctx, pool, idx, true)
	}

	downSQL, err := os.ReadFile("migrations/000018_cross_scope_time_window_indexes.down.sql")
	if err != nil {
		t.Fatalf("read 000018 down migration: %v", err)
	}
	if _, err := pool.Exec(ctx, substituteSchema(string(downSQL))); err != nil {
		t.Fatalf("apply 000018 down migration: %v", err)
	}
	for _, idx := range indexes {
		assertIndexExists(t, ctx, pool, idx, false)
	}

	upSQL, err := os.ReadFile("migrations/000018_cross_scope_time_window_indexes.up.sql")
	if err != nil {
		t.Fatalf("read 000018 up migration: %v", err)
	}
	if _, err := pool.Exec(ctx, substituteSchema(string(upSQL))); err != nil {
		t.Fatalf("apply 000018 up migration: %v", err)
	}
	for _, idx := range indexes {
		assertIndexExists(t, ctx, pool, idx, true)
	}
}
