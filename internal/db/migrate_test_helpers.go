//go:build integration

package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrateForTest applies migrations with pg_cron, pg_partman, and pg_prewarm
// calls stripped, and with vector dimensions reduced to 4 for speed.
// Used in integration tests where the container does not have these extensions.
func MigrateForTest(ctx context.Context, pool *pgxpool.Pool) error {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("migrate_test: read migrations dir: %w", err)
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrate_test: acquire connection: %w", err)
	}
	defer conn.Release()

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		data, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("migrate_test: read %s: %w", e.Name(), err)
		}
		sql := stripTestUnsupported(string(data))
		if _, err := conn.Exec(ctx, sql); err != nil {
			return fmt.Errorf("migrate_test: execute %s: %w", e.Name(), err)
		}
	}
	return nil
}

// stripTestUnsupported removes SQL statements that reference extensions or
// functions not available in the pgvector/pgvector:pg18 test image, and
// downsizes vector dimensions to 4 for fast test embeddings.
//
// The function works statement-by-statement (splitting on ";") so that
// multi-line calls like SELECT partman.create_parent(...) are removed in full.
// Comments within a statement are excluded from keyword matching so that
// file-header comments referencing banned extension names do not accidentally
// suppress real DDL statements.
func stripTestUnsupported(sql string) string {
	skipKeywords := []string{
		"pg_cron",
		"pg_partman",
		"cron.schedule",
		"partman.",
		"create_partition", // pg_partman function not prefixed with schema name
		"part_config",      // pg_partman internal table
		"pg_prewarm",
		"shared_preload_libraries",
	}

	statements := strings.Split(sql, ";")
	kept := make([]string, 0, len(statements))
	for _, stmt := range statements {
		// Strip single-line SQL comments before keyword-checking so that
		// comment lines mentioning banned names do not drop live DDL.
		nonComment := removeLineComments(stmt)
		lower := strings.ToLower(nonComment)
		keep := true
		for _, kw := range skipKeywords {
			if strings.Contains(lower, kw) {
				keep = false
				break
			}
		}
		if keep {
			kept = append(kept, stmt)
		}
	}
	result := strings.Join(kept, ";")

	// Substitute the schema placeholder so migration SQL is valid.
	result = strings.ReplaceAll(result, schemaPlaceholder, "public")

	// Downsize vector dimensions so 4-dim test embeddings fit the columns.
	result = strings.ReplaceAll(result, "vector(1536)", "vector(4)")
	result = strings.ReplaceAll(result, "vector(1024)", "vector(4)")
	return result
}

// removeLineComments strips -- style single-line comments from a SQL fragment.
func removeLineComments(sql string) string {
	lines := strings.Split(sql, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		// Remove everything from -- onwards (simple approach; does not handle
		// -- inside string literals, but that is not an issue for our migrations).
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
