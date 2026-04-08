//go:build integration

package db_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"

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
