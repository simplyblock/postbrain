//go:build integration

package jobs

import (
	"context"
	"errors"
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
		fmt.Sprintf("SELECT count(*) FROM cypher('postbrain', $$ MATCH (n:Entity) WHERE n.id = '%s' RETURN n $$) AS (result agtype)", subjectID.String()),
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

func TestAGEBackfillJob_Run_EnsuresOverlayBeforeDetectAndBackfills(t *testing.T) {
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

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "age-backfill-owner-self-heal")
	scope := testhelper.CreateTestScope(t, pool, "project", "age-backfill-scope-self-heal", nil, owner.ID)

	subjectID := uuid.New()
	objectID := uuid.New()

	if _, err := pool.Exec(ctx, `
		INSERT INTO entities (id, scope_id, entity_type, name, canonical, meta)
		VALUES ($1, $2, 'function', 'RefreshSession', 'auth.RefreshSession', '{}'::jsonb)
	`, subjectID, scope.ID); err != nil {
		t.Fatalf("insert subject entity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO entities (id, scope_id, entity_type, name, canonical, meta)
		VALUES ($1, $2, 'function', 'ValidateSession', 'auth.ValidateSession', '{}'::jsonb)
	`, objectID, scope.ID); err != nil {
		t.Fatalf("insert object entity: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO relations (scope_id, subject_id, predicate, object_id, confidence)
		VALUES ($1, $2, 'depends_on', $3, 0.90)
	`, scope.ID, subjectID, objectID); err != nil {
		t.Fatalf("insert relation: %v", err)
	}

	j := NewAGEBackfillJob(pool, 10)
	if err := j.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var ageInstalled bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='age')").Scan(&ageInstalled); err != nil {
		t.Fatalf("detect age extension: %v", err)
	}
	if !ageInstalled {
		t.Fatalf("AGE extension should be installed by backfill run")
	}

	var edgeCount int
	if err := pool.QueryRow(ctx,
		fmt.Sprintf(
			"SELECT count(*) FROM cypher('postbrain', $$ MATCH (a:Entity)-[r:RELATION]->(b:Entity) WHERE a.id = '%s' AND b.id = '%s' AND r.predicate = 'depends_on' RETURN r $$) AS (result agtype)",
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

func TestAGEBackfillJob_Run_MaxOneConnection_DoesNotSelfDeadlock(t *testing.T) {
	basePool := testhelper.NewTestPool(t)
	ctx := context.Background()

	connCfg := basePool.Config().ConnConfig
	dbURL := "postgres://" + connCfg.User + ":" + connCfg.Password + "@" + connCfg.Host + ":" + strconv.Itoa(int(connCfg.Port)) + "/" + connCfg.Database + "?sslmode=disable"
	smallPool, err := db.NewPool(ctx, &config.DatabaseConfig{
		URL:            dbURL,
		MaxOpen:        1,
		MaxIdle:        1,
		ConnectTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("create small pool: %v", err)
	}
	defer smallPool.Close()

	j := NewAGEBackfillJob(smallPool, 10)
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = j.Run(runCtx)
	if err == nil {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("Run should not deadlock with max-open=1 pool: %v", err)
	}
}
