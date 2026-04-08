//go:build integration

package testhelper

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
)

// NewTestPool starts a pgvector/pgvector:pg18 container, runs test migrations,
// and returns a ready connection pool. The container and pool are cleaned up
// when t.Cleanup runs.
func NewTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return NewTestPoolWithImage(t, "pgvector/pgvector:pg18")
}

// NewTestPoolWithImage starts a PostgreSQL container from the provided image,
// runs test migrations, and returns a ready connection pool. Additional
// testcontainers customizers are forwarded to tcpostgres.Run.
func NewTestPoolWithImage(t *testing.T, image string, customizers ...testcontainers.ContainerCustomizer) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	runCustomizers := make([]testcontainers.ContainerCustomizer, 0, len(customizers)+4)
	runCustomizers = append(runCustomizers, customizers...)
	runCustomizers = append(runCustomizers,
		tcpostgres.WithDatabase("postbrain_test"),
		tcpostgres.WithUsername("postbrain"),
		tcpostgres.WithPassword("postbrain"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)

	ctr, err := tcpostgres.Run(ctx, image, runCustomizers...)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	cfg := &config.DatabaseConfig{
		URL:            connStr,
		MaxOpen:        5,
		MaxIdle:        2,
		ConnectTimeout: 10 * time.Second,
	}
	pool, err := db.NewPool(ctx, cfg)
	if err != nil {
		t.Fatalf("create pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := db.MigrateForTest(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return pool
}
