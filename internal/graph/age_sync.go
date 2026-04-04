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

	cypher := fmt.Sprintf(`
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

	if _, err := pool.Exec(ctx, "SELECT * FROM cypher('postbrain', $1) AS (result agtype)", cypher); err != nil {
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

	cypher := fmt.Sprintf(`
MATCH (a:Entity {id: '%s'}), (b:Entity {id: '%s'})
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

	if _, err := pool.Exec(ctx, "SELECT * FROM cypher('postbrain', $1) AS (result agtype)", cypher); err != nil {
		return fmt.Errorf("graph: sync relation to age: %w", err)
	}
	return nil
}

func escapeCypherString(s string) string {
	return strings.ReplaceAll(s, "'", "\\'")
}
