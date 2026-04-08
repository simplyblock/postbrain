package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ageAvailable(ctx context.Context, pool *pgxpool.Pool) bool {
	if pool == nil {
		return false
	}
	var installed bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='age')").Scan(&installed); err != nil {
		return false
	}
	return installed
}

func syncEntityToAGEIfAvailable(ctx context.Context, pool *pgxpool.Pool, entity *Entity) error {
	if entity == nil || !ageAvailable(ctx, pool) {
		return nil
	}
	return syncEntityToAGE(ctx, pool, entity)
}

func syncRelationToAGEIfAvailable(ctx context.Context, pool *pgxpool.Pool, rel *Relation) error {
	if rel == nil || !ageAvailable(ctx, pool) {
		return nil
	}
	return syncRelationToAGE(ctx, pool, rel)
}

func syncEntityToAGE(ctx context.Context, pool *pgxpool.Pool, entity *Entity) error {
	updateCypher := fmt.Sprintf(`
MATCH (e:Entity)
WHERE e.id = '%s'
SET e.scope_id = '%s',
    e.entity_type = '%s',
    e.name = '%s',
    e.canonical = '%s'
RETURN e
`,
		entity.ID.String(),
		entity.ScopeID.String(),
		escapeCypherString(entity.EntityType),
		escapeCypherString(entity.Name),
		escapeCypherString(entity.Canonical),
	)
	updated, err := ageCypherHasRows(ctx, pool, updateCypher)
	if err != nil {
		return fmt.Errorf("sync entity to age: %w", err)
	}
	if updated {
		return nil
	}

	createCypher := fmt.Sprintf(`
CREATE (e:Entity)
SET e.id = '%s',
    e.scope_id = '%s',
    e.entity_type = '%s',
    e.name = '%s',
    e.canonical = '%s'
RETURN e
`,
		entity.ID.String(),
		entity.ScopeID.String(),
		escapeCypherString(entity.EntityType),
		escapeCypherString(entity.Name),
		escapeCypherString(entity.Canonical),
	)
	if _, err := pool.Exec(ctx, buildAGECypherSQLLiteral(createCypher)); err != nil {
		return fmt.Errorf("sync entity to age: %w", err)
	}
	return nil
}

func syncRelationToAGE(ctx context.Context, pool *pgxpool.Pool, rel *Relation) error {
	updateCypher := fmt.Sprintf(`
MATCH (a:Entity)-[r:RELATION]->(b:Entity)
WHERE a.id = '%s'
  AND b.id = '%s'
  AND r.predicate = '%s'
SET r.confidence = %s,
    r.scope_id = '%s'
RETURN r
`,
		rel.SubjectID.String(),
		rel.ObjectID.String(),
		escapeCypherString(rel.Predicate),
		strconv.FormatFloat(rel.Confidence, 'f', -1, 64),
		rel.ScopeID.String(),
	)
	updated, err := ageCypherHasRows(ctx, pool, updateCypher)
	if err != nil {
		return fmt.Errorf("sync relation to age: %w", err)
	}
	if updated {
		return nil
	}

	createCypher := fmt.Sprintf(`
MATCH (a:Entity), (b:Entity)
WHERE a.id = '%s'
  AND b.id = '%s'
CREATE (a)-[r:RELATION]->(b)
SET r.predicate = '%s',
    r.confidence = %s,
    r.scope_id = '%s'
RETURN r
`,
		rel.SubjectID.String(),
		rel.ObjectID.String(),
		escapeCypherString(rel.Predicate),
		strconv.FormatFloat(rel.Confidence, 'f', -1, 64),
		rel.ScopeID.String(),
	)
	if _, err := pool.Exec(ctx, buildAGECypherSQLLiteral(createCypher)); err != nil {
		return fmt.Errorf("sync relation to age: %w", err)
	}
	return nil
}

func ageCypherHasRows(ctx context.Context, pool *pgxpool.Pool, cypher string) (bool, error) {
	rows, err := pool.Query(ctx, buildAGECypherSQLLiteral(cypher))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

func buildAGECypherSQLLiteral(cypher string) string {
	tag := "postbrain"
	delim := "$" + tag + "$"
	for strings.Contains(cypher, delim) {
		tag += "_x"
		delim = "$" + tag + "$"
	}
	return fmt.Sprintf(
		"SELECT * FROM ag_catalog.cypher('postbrain', %s%s%s) AS (result ag_catalog.agtype)",
		delim,
		cypher,
		delim,
	)
}

func escapeCypherString(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}
