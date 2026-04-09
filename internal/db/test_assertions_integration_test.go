//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func assertIndexExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, indexName string, want bool) {
	t.Helper()
	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_indexes
			WHERE schemaname = 'public' AND indexname = $1
		)
	`, indexName).Scan(&exists)
	if err != nil {
		t.Fatalf("index %q: query error: %v", indexName, err)
	}
	if exists != want {
		t.Fatalf("index %q existence: got=%v want=%v", indexName, exists, want)
	}
}
