package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrAGEUnavailable indicates Apache AGE is not available in the active DB.
var ErrAGEUnavailable = errors.New("graph: age unavailable")

// DetectAGE returns true when Apache AGE is installed and visible.
func DetectAGE(ctx context.Context, pool *pgxpool.Pool) bool {
	if pool == nil {
		return false
	}
	var installed bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='age')").Scan(&installed); err != nil {
		return false
	}
	return installed
}

// RunCypherQuery executes a scoped Cypher query against the AGE graph.
func RunCypherQuery(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, cypher string) ([]map[string]any, error) {
	if pool == nil {
		return nil, fmt.Errorf("graph: nil pool")
	}
	if !DetectAGE(ctx, pool) {
		return nil, ErrAGEUnavailable
	}

	scopedCypher := buildScopedCypher(scopeID, cypher)
	rows, err := pool.Query(ctx, "SELECT * FROM cypher('postbrain', $1) AS (result agtype)", scopedCypher)
	if err != nil {
		return nil, fmt.Errorf("graph: run cypher query: %w", err)
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("graph: read cypher row values: %w", err)
		}
		rowMap := make(map[string]any, len(values))
		for i, fd := range rows.FieldDescriptions() {
			rowMap[string(fd.Name)] = normalizeCypherValue(values[i])
		}
		out = append(out, rowMap)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph: iterate cypher rows: %w", err)
	}
	return out, nil
}

func buildScopedCypher(scopeID uuid.UUID, cypher string) string {
	trimmed := strings.TrimSpace(cypher)
	if trimmed == "" {
		trimmed = "RETURN n"
	}
	return fmt.Sprintf(
		"MATCH (n:Entity {scope_id: '%s'})\nWITH n\n%s",
		scopeID.String(),
		trimmed,
	)
}

func normalizeCypherValue(v any) any {
	switch t := v.(type) {
	case []byte:
		return string(t)
	default:
		return t
	}
}
