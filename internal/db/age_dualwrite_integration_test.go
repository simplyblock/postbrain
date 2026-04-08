//go:build integration

package db_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestUpsertEntityAndRelation_DualWriteToAGE_WhenAvailable(t *testing.T) {
	ageImage := strings.TrimSpace(os.Getenv("POSTBRAIN_TEST_AGE_IMAGE"))
	if ageImage == "" {
		t.Skip("set POSTBRAIN_TEST_AGE_IMAGE to run strict AGE dual-write coverage")
	}

	pool := testhelper.NewTestPoolWithImage(
		t,
		ageImage,
		testcontainers.WithCmd(
			"postgres",
			"-c", "shared_preload_libraries=age,pg_cron,pg_partman_bgw",
			"-c", "cron.database_name=postbrain_test",
			"-c", "pg_partman_bgw.dbname=postbrain_test",
		),
	)
	ctx := context.Background()
	if err := db.EnsureAGEOverlay(ctx, pool); err != nil {
		t.Fatalf("EnsureAGEOverlay: %v", err)
	}

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "age-dualwrite-owner")
	scope := testhelper.CreateTestScope(t, pool, "project", "age-dualwrite-scope", nil, owner.ID)

	subject, err := db.UpsertEntity(ctx, pool, &db.Entity{
		ScopeID:    scope.ID,
		EntityType: "function",
		Name:       "Authenticate",
		Canonical:  "auth.Authenticate",
	})
	if err != nil {
		t.Fatalf("UpsertEntity(subject): %v", err)
	}
	object, err := db.UpsertEntity(ctx, pool, &db.Entity{
		ScopeID:    scope.ID,
		EntityType: "function",
		Name:       "IssueJWT",
		Canonical:  "auth.IssueJWT",
	})
	if err != nil {
		t.Fatalf("UpsertEntity(object): %v", err)
	}

	if _, err := db.UpsertRelation(ctx, pool, &db.Relation{
		ScopeID:    scope.ID,
		SubjectID:  subject.ID,
		ObjectID:   object.ID,
		Predicate:  "depends_on",
		Confidence: 0.9,
	}); err != nil {
		t.Fatalf("UpsertRelation: %v", err)
	}

	var nodeCount int
	if err := pool.QueryRow(ctx,
		fmt.Sprintf("SELECT count(*) FROM cypher('postbrain', $$ MATCH (n:Entity) WHERE n.id = '%s' RETURN n $$) AS (result agtype)", subject.ID.String()),
	).Scan(&nodeCount); err != nil {
		t.Fatalf("query AGE node: %v", err)
	}
	if nodeCount != 1 {
		t.Fatalf("AGE subject node count = %d, want 1", nodeCount)
	}

	var edgeCount int
	if err := pool.QueryRow(ctx,
		fmt.Sprintf(
			"SELECT count(*) FROM cypher('postbrain', $$ MATCH (a:Entity)-[r:RELATION]->(b:Entity) WHERE a.id = '%s' AND b.id = '%s' AND r.predicate = 'depends_on' RETURN r $$) AS (result agtype)",
			subject.ID.String(),
			object.ID.String(),
		),
	).Scan(&edgeCount); err != nil {
		t.Fatalf("query AGE relation: %v", err)
	}
	if edgeCount != 1 {
		t.Fatalf("AGE relation count = %d, want 1", edgeCount)
	}
}

func TestUpsertEntity_DoesNotFailWhenAGEUnavailable(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "age-unavailable-owner-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "age-unavailable-scope-"+uuid.NewString(), nil, owner.ID)

	if _, err := db.UpsertEntity(ctx, pool, &db.Entity{
		ScopeID:    scope.ID,
		EntityType: "component",
		Name:       "Auth",
		Canonical:  "auth",
	}); err != nil {
		t.Fatalf("UpsertEntity without AGE should still succeed: %v", err)
	}
}

func TestUpsertEntityAndRelation_DoNotFailWhenAGEDualWriteErrors(t *testing.T) {
	ageImage := strings.TrimSpace(os.Getenv("POSTBRAIN_TEST_AGE_IMAGE"))
	if ageImage == "" {
		t.Skip("set POSTBRAIN_TEST_AGE_IMAGE to run strict AGE dual-write coverage")
	}

	pool := testhelper.NewTestPoolWithImage(
		t,
		ageImage,
		testcontainers.WithCmd(
			"postgres",
			"-c", "shared_preload_libraries=age,pg_cron,pg_partman_bgw",
			"-c", "cron.database_name=postbrain_test",
			"-c", "pg_partman_bgw.dbname=postbrain_test",
		),
	)
	ctx := context.Background()
	if err := db.EnsureAGEOverlay(ctx, pool); err != nil {
		t.Fatalf("EnsureAGEOverlay: %v", err)
	}

	// Force AGE dual-write failures for non-owner roles while keeping relational writes possible.
	if _, err := pool.Exec(ctx, `REVOKE USAGE ON SCHEMA ag_catalog FROM PUBLIC`); err != nil {
		t.Fatalf("revoke ag_catalog usage: %v", err)
	}
	if _, err := pool.Exec(ctx, `REVOKE EXECUTE ON ALL FUNCTIONS IN SCHEMA ag_catalog FROM PUBLIC`); err != nil {
		t.Fatalf("revoke ag_catalog function execute: %v", err)
	}

	if _, err := pool.Exec(ctx, `CREATE ROLE pb_age_broken LOGIN PASSWORD 'pb_age_broken'`); err != nil {
		t.Fatalf("create app role: %v", err)
	}
	if _, err := pool.Exec(ctx, `GRANT CONNECT ON DATABASE postbrain_test TO pb_age_broken`); err != nil {
		t.Fatalf("grant connect: %v", err)
	}
	if _, err := pool.Exec(ctx, `GRANT USAGE ON SCHEMA public TO pb_age_broken`); err != nil {
		t.Fatalf("grant public usage: %v", err)
	}
	if _, err := pool.Exec(ctx, `GRANT SELECT, INSERT, UPDATE ON TABLE entities TO pb_age_broken`); err != nil {
		t.Fatalf("grant entities dml: %v", err)
	}
	if _, err := pool.Exec(ctx, `GRANT SELECT, INSERT, UPDATE ON TABLE relations TO pb_age_broken`); err != nil {
		t.Fatalf("grant relations dml: %v", err)
	}

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "age-broken-owner-"+uuid.NewString())
	scope := testhelper.CreateTestScope(t, pool, "project", "age-broken-scope-"+uuid.NewString(), nil, owner.ID)

	cfg := pool.Config().ConnConfig
	appURL := "postgres://pb_age_broken:pb_age_broken@" + cfg.Host + ":" + strconv.Itoa(int(cfg.Port)) + "/" + cfg.Database + "?sslmode=disable"
	appPool, err := db.NewPool(ctx, &config.DatabaseConfig{
		URL:            appURL,
		MaxOpen:        2,
		MaxIdle:        1,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("create app pool: %v", err)
	}
	defer appPool.Close()

	subject, err := db.UpsertEntity(ctx, appPool, &db.Entity{
		ScopeID:    scope.ID,
		EntityType: "function",
		Name:       "Authenticate",
		Canonical:  "auth.Authenticate",
	})
	if err != nil {
		t.Fatalf("UpsertEntity(subject) should succeed even when AGE dual-write fails: %v", err)
	}
	object, err := db.UpsertEntity(ctx, appPool, &db.Entity{
		ScopeID:    scope.ID,
		EntityType: "function",
		Name:       "IssueJWT",
		Canonical:  "auth.IssueJWT",
	})
	if err != nil {
		t.Fatalf("UpsertEntity(object) should succeed even when AGE dual-write fails: %v", err)
	}

	if _, err := db.UpsertRelation(ctx, appPool, &db.Relation{
		ScopeID:    scope.ID,
		SubjectID:  subject.ID,
		ObjectID:   object.ID,
		Predicate:  "depends_on",
		Confidence: 0.8,
	}); err != nil {
		t.Fatalf("UpsertRelation should succeed even when AGE dual-write fails: %v", err)
	}

	relationalEntity, err := db.GetEntityByCanonical(ctx, appPool, scope.ID, "function", "auth.Authenticate")
	if err != nil {
		t.Fatalf("GetEntityByCanonical: %v", err)
	}
	if relationalEntity == nil {
		t.Fatal("expected relational entity to be persisted")
	}
	rels, err := db.ListRelationsForEntity(ctx, appPool, subject.ID, "depends_on")
	if err != nil {
		t.Fatalf("ListRelationsForEntity: %v", err)
	}
	if len(rels) != 1 {
		t.Fatalf("expected one relational relation, got %d", len(rels))
	}
}
