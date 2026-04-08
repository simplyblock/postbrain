package db

import (
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBuildMigratorDSN_ForcesPublicMigrationsTable(t *testing.T) {
	poolCfg, err := pgxpool.ParseConfig("postgres://user:pass@localhost:5432/postgres?sslmode=disable")
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	dsn, err := buildMigratorDSN(poolCfg.ConnConfig)
	if err != nil {
		t.Fatalf("buildMigratorDSN: %v", err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("url.Parse(dsn): %v", err)
	}

	if got := u.Query().Get("x-migrations-table"); got != `"public"."schema_migrations"` {
		t.Fatalf("x-migrations-table = %q, want %q", got, `"public"."schema_migrations"`)
	}
	if got := u.Query().Get("x-migrations-table-quoted"); got != "1" {
		t.Fatalf("x-migrations-table-quoted = %q, want %q", got, "1")
	}
	if got := u.Query().Get("search_path"); got != "" {
		t.Fatalf("search_path startup param must not be set in migrator DSN: %q", got)
	}
}
