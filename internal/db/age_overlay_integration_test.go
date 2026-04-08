//go:build integration

package db_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestEnsureAGEOverlay_IdempotentAndBestEffort(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ctx := context.Background()

	if err := db.EnsureAGEOverlay(ctx, pool); err != nil {
		t.Fatalf("EnsureAGEOverlay first call: %v", err)
	}
	if err := db.EnsureAGEOverlay(ctx, pool); err != nil {
		t.Fatalf("EnsureAGEOverlay second call: %v", err)
	}

	var ageInstalled bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='age')").Scan(&ageInstalled); err != nil {
		t.Fatalf("query age extension availability: %v", err)
	}
	if !ageInstalled {
		// Test image may not ship AGE. The contract is graceful no-op.
		return
	}

	var graphExists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM ag_catalog.ag_graph WHERE name='postbrain')").Scan(&graphExists); err != nil {
		t.Fatalf("query postbrain age graph: %v", err)
	}
	if !graphExists {
		t.Fatalf("expected AGE graph %q to exist after EnsureAGEOverlay", "postbrain")
	}
}

func TestEnsureAGEOverlay_AGEImage_ActivatesExtensionAndGraph(t *testing.T) {
	ageImage := strings.TrimSpace(os.Getenv("POSTBRAIN_TEST_AGE_IMAGE"))
	if ageImage == "" {
		t.Skip("set POSTBRAIN_TEST_AGE_IMAGE to run strict AGE activation coverage")
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

	var ageInstalled bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='age')").Scan(&ageInstalled); err != nil {
		t.Fatalf("query age extension availability: %v", err)
	}
	if !ageInstalled {
		t.Fatal("expected AGE extension to be installed in AGE-enabled image")
	}

	var graphExists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM ag_catalog.ag_graph WHERE name='postbrain')").Scan(&graphExists); err != nil {
		t.Fatalf("query postbrain age graph: %v", err)
	}
	if !graphExists {
		t.Fatalf("expected AGE graph %q to exist after EnsureAGEOverlay", "postbrain")
	}
}

func TestEnsureAGEOverlay_GrantsAGESchemaUsage_ForRestrictedRole(t *testing.T) {
	ageImage := strings.TrimSpace(os.Getenv("POSTBRAIN_TEST_AGE_IMAGE"))
	if ageImage == "" {
		t.Skip("set POSTBRAIN_TEST_AGE_IMAGE to run strict AGE activation coverage")
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

	if _, err := pool.Exec(ctx, `CREATE ROLE pb_app LOGIN PASSWORD 'pb_app'`); err != nil {
		t.Fatalf("create app role: %v", err)
	}
	if _, err := pool.Exec(ctx, `GRANT CONNECT ON DATABASE postbrain_test TO pb_app`); err != nil {
		t.Fatalf("grant connect: %v", err)
	}
	if _, err := pool.Exec(ctx, `GRANT USAGE ON SCHEMA public TO pb_app`); err != nil {
		t.Fatalf("grant public schema usage: %v", err)
	}
	if _, err := pool.Exec(ctx, `GRANT SELECT, INSERT, UPDATE ON TABLE entities TO pb_app`); err != nil {
		t.Fatalf("grant entities DML: %v", err)
	}

	owner := testhelper.CreateTestPrincipal(t, pool, "user", "age-grants-owner")
	scope := testhelper.CreateTestScope(t, pool, "project", "age-grants-scope", nil, owner.ID)

	cfg := pool.Config().ConnConfig
	appURL := "postgres://pb_app:pb_app@" + cfg.Host + ":" + strconv.Itoa(int(cfg.Port)) + "/" + cfg.Database + "?sslmode=disable"
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

	if _, err := db.UpsertEntity(ctx, appPool, &db.Entity{
		ScopeID:    scope.ID,
		EntityType: "concept",
		Name:       "age grants test",
		Canonical:  "age-grants-test",
	}); err != nil {
		t.Fatalf("UpsertEntity as restricted role: %v", err)
	}
}
