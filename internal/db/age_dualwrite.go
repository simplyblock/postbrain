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
	cypher := buildEntityUpsertCypher(entity)
	if _, err := pool.Exec(ctx, buildAGECypherSQLLiteral(cypher)); err != nil {
		return fmt.Errorf("sync entity to age: %w", err)
	}
	return nil
}

func syncRelationToAGE(ctx context.Context, pool *pgxpool.Pool, rel *Relation) error {
	cypher := buildRelationUpsertCypher(rel)
	if _, err := pool.Exec(ctx, buildAGECypherSQLLiteral(cypher)); err != nil {
		return fmt.Errorf("sync relation to age: %w", err)
	}
	return nil
}

func buildEntityUpsertCypher(entity *Entity) string {
	return fmt.Sprintf(`
MERGE (e:Entity {id: '%s'})
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
}

func buildRelationUpsertCypher(rel *Relation) string {
	return fmt.Sprintf(`
MATCH (a:Entity), (b:Entity)
WHERE a.id = '%s'
  AND b.id = '%s'
MERGE (a)-[r:RELATION {predicate: '%s'}]->(b)
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
