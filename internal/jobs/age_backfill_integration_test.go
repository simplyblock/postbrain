//go:build integration

package jobs

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

func TestAGEBackfillJob_Run_BackfillsExistingRelationalGraph(t *testing.T) {
	ageImage := strings.TrimSpace(os.Getenv("POSTBRAIN_TEST_AGE_IMAGE"))
	if ageImage == "" {
		t.Skip("set POSTBRAIN_TEST_AGE_IMAGE to run strict AGE backfill coverage")
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

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "age-backfill-owner")
	scope := testhelper.CreateTestScope(t, pool, "project", "age-backfill-scope", nil, owner.ID)

	subjectID := uuid.New()
	objectID := uuid.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO entities (id, scope_id, entity_type, name, canonical, meta)
		VALUES ($1, $2, 'function', 'Authenticate', 'auth.Authenticate', '{}'::jsonb)
	`, subjectID, scope.ID); err != nil {
		t.Fatalf("insert subject entity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entities (id, scope_id, entity_type, name, canonical, meta)
		VALUES ($1, $2, 'function', 'IssueJWT', 'auth.IssueJWT', '{}'::jsonb)
	`, objectID, scope.ID); err != nil {
		t.Fatalf("insert object entity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO relations (scope_id, subject_id, predicate, object_id, confidence)
		VALUES ($1, $2, 'depends_on', $3, 0.95)
	`, scope.ID, subjectID, objectID); err != nil {
		t.Fatalf("insert relation: %v", err)
	}

	j := NewAGEBackfillJob(pool, 10)
	if err := j.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var nodeCount int
	if err := pool.QueryRow(ctx,
		fmt.Sprintf("SELECT count(*) FROM cypher('postbrain', $$ MATCH (n:Entity {id: '%s'}) RETURN n $$) AS (result agtype)", subjectID.String()),
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
			subjectID.String(),
			objectID.String(),
		),
	).Scan(&edgeCount); err != nil {
		t.Fatalf("query AGE relation: %v", err)
	}
	if edgeCount != 1 {
		t.Fatalf("AGE relation count = %d, want 1", edgeCount)
	}
}
