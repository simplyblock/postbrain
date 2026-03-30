package graph

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// Neighbour is a resolved graph neighbour: the related entity plus the edge metadata.
type Neighbour struct {
	Entity     *db.Entity
	Predicate  string
	Direction  string  // "outgoing" (subject→object) or "incoming" (object←subject)
	Confidence float64
	SourceFile *string
}

// TraversalResult is returned by all traversal functions.
type TraversalResult struct {
	Entity     *db.Entity
	Neighbours []Neighbour
}

// ResolveSymbol looks up an entity in scopeID by exact canonical name, falling
// back to heuristic suffix matching. Returns nil if nothing matches.
func ResolveSymbol(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, symbol string) (*db.Entity, error) {
	// Try each code entity type with an exact canonical match first.
	for _, etype := range []string{"function", "method", "type", "struct", "interface",
		"class", "module", "package", "variable", "file"} {
		e, err := db.GetEntityByCanonical(ctx, pool, scopeID, etype, symbol)
		if err == nil && e != nil {
			return e, nil
		}
	}
	// Heuristic suffix fallback.
	candidates, err := db.FindEntitiesBySuffix(ctx, pool, scopeID, symbol)
	if err != nil {
		return nil, fmt.Errorf("graph: resolve symbol: %w", err)
	}
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	return nil, nil
}

// Callers returns entities that call the named symbol (incoming `calls` edges).
func Callers(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, symbol string) (*TraversalResult, error) {
	entity, err := ResolveSymbol(ctx, pool, scopeID, symbol)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}
	rels, err := db.ListIncomingRelations(ctx, pool, scopeID, entity.ID, "calls")
	if err != nil {
		return nil, fmt.Errorf("graph: callers: %w", err)
	}
	return buildResult(ctx, pool, entity, rels, "incoming")
}

// Callees returns entities called by the named symbol (outgoing `calls` edges).
func Callees(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, symbol string) (*TraversalResult, error) {
	entity, err := ResolveSymbol(ctx, pool, scopeID, symbol)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}
	rels, err := db.ListOutgoingRelations(ctx, pool, scopeID, entity.ID, "calls")
	if err != nil {
		return nil, fmt.Errorf("graph: callees: %w", err)
	}
	return buildResult(ctx, pool, entity, rels, "outgoing")
}

// Dependencies returns what the named symbol imports or depends on
// (outgoing `imports`, `uses`, `extends`, `implements` edges).
func Dependencies(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, symbol string) (*TraversalResult, error) {
	entity, err := ResolveSymbol(ctx, pool, scopeID, symbol)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}

	var all []*db.Relation
	for _, pred := range []string{"imports", "uses", "extends", "implements", "defines"} {
		rels, err := db.ListOutgoingRelations(ctx, pool, scopeID, entity.ID, pred)
		if err != nil {
			return nil, fmt.Errorf("graph: dependencies: %w", err)
		}
		all = append(all, rels...)
	}
	return buildResult(ctx, pool, entity, all, "outgoing")
}

// Dependents returns entities that depend on the named symbol
// (all incoming edges: calls, uses, imports, extends, implements).
func Dependents(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, symbol string) (*TraversalResult, error) {
	entity, err := ResolveSymbol(ctx, pool, scopeID, symbol)
	if err != nil {
		return nil, err
	}
	if entity == nil {
		return nil, nil
	}
	rels, err := db.ListIncomingRelations(ctx, pool, scopeID, entity.ID, "")
	if err != nil {
		return nil, fmt.Errorf("graph: dependents: %w", err)
	}
	return buildResult(ctx, pool, entity, rels, "incoming")
}

// NeighboursForEntity fetches all direct neighbours of a known entity (both directions).
// Used by graph-augmented recall.
func NeighboursForEntity(ctx context.Context, pool *pgxpool.Pool, scopeID, entityID uuid.UUID) ([]Neighbour, error) {
	entity, err := db.GetEntityByID(ctx, pool, entityID)
	if err != nil || entity == nil {
		return nil, err
	}
	outgoing, err := db.ListOutgoingRelations(ctx, pool, scopeID, entityID, "")
	if err != nil {
		return nil, fmt.Errorf("graph: neighbours outgoing: %w", err)
	}
	incoming, err := db.ListIncomingRelations(ctx, pool, scopeID, entityID, "")
	if err != nil {
		return nil, fmt.Errorf("graph: neighbours incoming: %w", err)
	}

	result, err := buildResult(ctx, pool, entity, append(outgoing, incoming...), "mixed")
	if err != nil || result == nil {
		return nil, err
	}
	return result.Neighbours, nil
}

// buildResult hydrates the neighbour entities for a set of relations.
func buildResult(ctx context.Context, pool *pgxpool.Pool, entity *db.Entity, rels []*db.Relation, direction string) (*TraversalResult, error) {
	res := &TraversalResult{Entity: entity}
	for _, r := range rels {
		var neighbourID uuid.UUID
		dir := direction
		if direction == "outgoing" {
			neighbourID = r.ObjectID
		} else if direction == "incoming" {
			neighbourID = r.SubjectID
		} else {
			// mixed: determine direction from relation
			if r.SubjectID == entity.ID {
				neighbourID = r.ObjectID
				dir = "outgoing"
			} else {
				neighbourID = r.SubjectID
				dir = "incoming"
			}
		}

		neighbour, err := db.GetEntityByID(ctx, pool, neighbourID)
		if err != nil || neighbour == nil {
			continue
		}
		res.Neighbours = append(res.Neighbours, Neighbour{
			Entity:     neighbour,
			Predicate:  r.Predicate,
			Direction:  dir,
			Confidence: r.Confidence,
			SourceFile: r.SourceFile,
		})
	}
	return res, nil
}
