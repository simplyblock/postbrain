//go:build integration

package db_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"

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
		fmt.Sprintf("SELECT count(*) FROM cypher('postbrain', $$ MATCH (n:Entity {id: '%s'}) RETURN n $$) AS (result agtype)", subject.ID.String()),
	).Scan(&nodeCount); err != nil {
		t.Fatalf("query AGE node: %v", err)
	}
	if nodeCount != 1 {
		t.Fatalf("AGE subject node count = %d, want 1", nodeCount)
	}

	var edgeCount int
	if err := pool.QueryRow(ctx,
		fmt.Sprintf(
			"SELECT count(*) FROM cypher('postbrain', $$ MATCH (a:Entity {id: '%s'})-[r:RELATION {predicate: 'depends_on'}]->(b:Entity {id: '%s'}) RETURN r $$) AS (result agtype)",
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
