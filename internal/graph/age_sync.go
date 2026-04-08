package graph

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// SyncEntityToAGE upserts a relational entity into the AGE overlay graph.
func SyncEntityToAGE(ctx context.Context, pool *pgxpool.Pool, entity *db.Entity) error {
	if pool == nil {
		return fmt.Errorf("graph: nil pool")
	}
	if entity == nil {
		return fmt.Errorf("graph: nil entity")
	}
	if !DetectAGE(ctx, pool) {
		return ErrAGEUnavailable
	}

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
		return fmt.Errorf("graph: sync entity to age: %w", err)
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
	if _, err := pool.Exec(ctx, buildAGECypherSQL(createCypher)); err != nil {
		return fmt.Errorf("graph: sync entity to age: %w", err)
	}
	return nil
}

// SyncRelationToAGE upserts a relational edge into the AGE overlay graph.
func SyncRelationToAGE(ctx context.Context, pool *pgxpool.Pool, rel *db.Relation) error {
	if pool == nil {
		return fmt.Errorf("graph: nil pool")
	}
	if rel == nil {
		return fmt.Errorf("graph: nil relation")
	}
	if !DetectAGE(ctx, pool) {
		return ErrAGEUnavailable
	}

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
		return fmt.Errorf("graph: sync relation to age: %w", err)
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
	if _, err := pool.Exec(ctx, buildAGECypherSQL(createCypher)); err != nil {
		return fmt.Errorf("graph: sync relation to age: %w", err)
	}
	return nil
}

func ageCypherHasRows(ctx context.Context, pool *pgxpool.Pool, cypher string) (bool, error) {
	rows, err := pool.Query(ctx, buildAGECypherSQL(cypher))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), rows.Err()
}

func escapeCypherString(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}
