package compat

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// UpsertEntity inserts or updates an entity.
func UpsertEntity(ctx context.Context, pool *pgxpool.Pool, e *db.Entity) (*db.Entity, error) {
	if e.Meta == nil {
		e.Meta = []byte("{}")
	}
	q := db.New(pool)
	result, err := q.UpsertEntity(ctx, db.UpsertEntityParams{
		ScopeID:          e.ScopeID,
		EntityType:       e.EntityType,
		Name:             e.Name,
		Canonical:        e.Canonical,
		Meta:             e.Meta,
		Embedding:        e.Embedding,
		EmbeddingModelID: e.EmbeddingModelID,
	})
	if err != nil {
		return nil, fmt.Errorf("db: upsert entity: %w", err)
	}
	if err := db.SyncEntityToAGEIfAvailable(ctx, pool, result); err != nil {
		_ = db.BestEffortAGEDualWriteError("entity", err)
	}

	if result.EmbeddingModelID != nil && result.Embedding != nil {
		repo := db.NewEmbeddingRepository(pool)
		if err := repo.UpsertEmbedding(ctx, db.UpsertEmbeddingInput{
			ObjectType: "entity",
			ObjectID:   result.ID,
			ScopeID:    result.ScopeID,
			ModelID:    *result.EmbeddingModelID,
			Embedding:  result.Embedding.Slice(),
		}); err != nil {
			return nil, fmt.Errorf("db: upsert entity dual-write: %w", err)
		}
	}

	return result, nil
}

// GetEntityByCanonical looks up an entity by scope, type, and canonical name.
func GetEntityByCanonical(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, entityType, canonical string) (*db.Entity, error) {
	q := db.New(pool)
	e, err := q.GetEntityByCanonical(ctx, db.GetEntityByCanonicalParams{
		ScopeID:    scopeID,
		EntityType: entityType,
		Canonical:  canonical,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get entity by canonical: %w", err)
	}
	return e, nil
}

// GetEntityByID retrieves an entity by its UUID. Returns nil, nil if not found.
func GetEntityByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*db.Entity, error) {
	q := db.New(pool)
	e, err := q.GetEntityByID(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get entity by id: %w", err)
	}
	return e, nil
}

// LinkMemoryToEntity inserts a memory_entities row.
func LinkMemoryToEntity(ctx context.Context, pool *pgxpool.Pool, memoryID, entityID uuid.UUID, role string) error {
	q := db.New(pool)
	var rolePtr *string
	if role != "" {
		rolePtr = &role
	}
	err := q.LinkMemoryToEntity(ctx, db.LinkMemoryToEntityParams{
		MemoryID: memoryID,
		EntityID: entityID,
		Role:     rolePtr,
	})
	if err != nil {
		return fmt.Errorf("db: link memory to entity: %w", err)
	}
	return nil
}

// UpsertRelation inserts or updates a relation.
func UpsertRelation(ctx context.Context, pool *pgxpool.Pool, r *db.Relation) (*db.Relation, error) {
	q := db.New(pool)
	result, err := q.UpsertRelation(ctx, db.UpsertRelationParams{
		ScopeID:        r.ScopeID,
		SubjectID:      r.SubjectID,
		Predicate:      r.Predicate,
		ObjectID:       r.ObjectID,
		Confidence:     r.Confidence,
		SourceMemory:   r.SourceMemory,
		SourceArtifact: r.SourceArtifact,
		SourceFile:     r.SourceFile,
	})
	if err != nil {
		return nil, fmt.Errorf("db: upsert relation: %w", err)
	}
	rel := &db.Relation{
		ID:             result.ID,
		ScopeID:        result.ScopeID,
		SubjectID:      result.SubjectID,
		Predicate:      result.Predicate,
		ObjectID:       result.ObjectID,
		Confidence:     result.Confidence,
		SourceMemory:   result.SourceMemory,
		SourceArtifact: result.SourceArtifact,
		SourceFile:     result.SourceFile,
		CreatedAt:      result.CreatedAt,
	}
	if err := db.SyncRelationToAGEIfAvailable(ctx, pool, rel); err != nil {
		_ = db.BestEffortAGEDualWriteError("relation", err)
	}
	return rel, nil
}

// DeleteRelationsBySourceFile removes all relations from a scope that were extracted from a given file.
func DeleteRelationsBySourceFile(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, sourceFile string) error {
	q := db.New(pool)
	return q.DeleteRelationsBySourceFile(ctx, db.DeleteRelationsBySourceFileParams{
		ScopeID:    scopeID,
		SourceFile: &sourceFile,
	})
}

// ListRelationsForEntity returns relations for an entity, optionally filtered by predicate.
func ListRelationsForEntity(ctx context.Context, pool *pgxpool.Pool, entityID uuid.UUID, predicate string) ([]*db.Relation, error) {
	q := db.New(pool)
	if predicate != "" {
		rows, err := q.ListRelationsForEntityByPredicate(ctx, db.ListRelationsForEntityByPredicateParams{
			SubjectID: entityID,
			Predicate: predicate,
		})
		if err != nil {
			return nil, fmt.Errorf("db: list relations for entity by predicate: %w", err)
		}
		out := make([]*db.Relation, len(rows))
		for i, r := range rows {
			out[i] = &db.Relation{
				ID:             r.ID,
				ScopeID:        r.ScopeID,
				SubjectID:      r.SubjectID,
				Predicate:      r.Predicate,
				ObjectID:       r.ObjectID,
				Confidence:     r.Confidence,
				SourceMemory:   r.SourceMemory,
				SourceArtifact: r.SourceArtifact,
				CreatedAt:      r.CreatedAt,
			}
		}
		return out, nil
	}
	rows, err := q.ListRelationsForEntity(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("db: list relations for entity: %w", err)
	}
	out := make([]*db.Relation, len(rows))
	for i, r := range rows {
		out[i] = &db.Relation{
			ID:             r.ID,
			ScopeID:        r.ScopeID,
			SubjectID:      r.SubjectID,
			Predicate:      r.Predicate,
			ObjectID:       r.ObjectID,
			Confidence:     r.Confidence,
			SourceMemory:   r.SourceMemory,
			SourceArtifact: r.SourceArtifact,
			CreatedAt:      r.CreatedAt,
		}
	}
	return out, nil
}

// ListEntitiesByCanonical returns all entities in a scope that share a canonical
// but have a different entity_type than excludeType.
func ListEntitiesByCanonical(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, canonical, excludeType string) ([]*db.Entity, error) {
	q := db.New(pool)
	es, err := q.ListEntitiesByCanonical(ctx, db.ListEntitiesByCanonicalParams{
		ScopeID:    scopeID,
		Canonical:  canonical,
		EntityType: excludeType,
	})
	if err != nil {
		return nil, fmt.Errorf("db: list entities by canonical: %w", err)
	}
	return es, nil
}

// ListEntitiesByScope returns entities in a scope.
func ListEntitiesByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, entityType string, limit, offset int) ([]*db.Entity, error) {
	if limit < 0 || limit > math.MaxInt32 {
		return nil, fmt.Errorf("sharing: invalid limit: %d", limit)
	}
	if offset < 0 || offset > math.MaxInt32 {
		return nil, fmt.Errorf("sharing: invalid offset: %d", offset)
	}
	q := db.New(pool)
	es, err := q.ListEntitiesByScope(ctx, db.ListEntitiesByScopeParams{
		ScopeID: scopeID,
		Column2: entityType,
		Limit:   int32(limit),
		Offset:  int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list entities by scope: %w", err)
	}
	return es, nil
}

// FindEntitiesBySuffix returns entities in a scope whose canonical name equals
// suffix or ends with ".suffix", "::suffix", or "#suffix".
func FindEntitiesBySuffix(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, suffix string) ([]*db.Entity, error) {
	q := db.New(pool)
	es, err := q.FindEntitiesBySuffix(ctx, db.FindEntitiesBySuffixParams{
		ScopeID:   scopeID,
		Canonical: suffix,
	})
	if err != nil {
		return nil, fmt.Errorf("db: find entities by suffix: %w", err)
	}
	return es, nil
}

// ListOutgoingRelations returns relations where the entity is the subject,
// optionally filtered by predicate (empty string = all predicates).
func ListOutgoingRelations(ctx context.Context, pool *pgxpool.Pool, scopeID, entityID uuid.UUID, predicate string) ([]*db.Relation, error) {
	q := db.New(pool)
	rows, err := q.ListOutgoingRelations(ctx, db.ListOutgoingRelationsParams{
		ScopeID:   scopeID,
		SubjectID: entityID,
		Column3:   predicate,
	})
	if err != nil {
		return nil, fmt.Errorf("db: list outgoing relations: %w", err)
	}
	out := make([]*db.Relation, len(rows))
	for i, r := range rows {
		out[i] = &db.Relation{
			ID:             r.ID,
			ScopeID:        r.ScopeID,
			SubjectID:      r.SubjectID,
			Predicate:      r.Predicate,
			ObjectID:       r.ObjectID,
			Confidence:     r.Confidence,
			SourceMemory:   r.SourceMemory,
			SourceArtifact: r.SourceArtifact,
			SourceFile:     r.SourceFile,
			CreatedAt:      r.CreatedAt,
		}
	}
	return out, nil
}

// ListIncomingRelations returns relations where the entity is the object,
// optionally filtered by predicate (empty string = all predicates).
func ListIncomingRelations(ctx context.Context, pool *pgxpool.Pool, scopeID, entityID uuid.UUID, predicate string) ([]*db.Relation, error) {
	q := db.New(pool)
	rows, err := q.ListIncomingRelations(ctx, db.ListIncomingRelationsParams{
		ScopeID:  scopeID,
		ObjectID: entityID,
		Column3:  predicate,
	})
	if err != nil {
		return nil, fmt.Errorf("db: list incoming relations: %w", err)
	}
	out := make([]*db.Relation, len(rows))
	for i, r := range rows {
		out[i] = &db.Relation{
			ID:             r.ID,
			ScopeID:        r.ScopeID,
			SubjectID:      r.SubjectID,
			Predicate:      r.Predicate,
			ObjectID:       r.ObjectID,
			Confidence:     r.Confidence,
			SourceMemory:   r.SourceMemory,
			SourceArtifact: r.SourceArtifact,
			SourceFile:     r.SourceFile,
			CreatedAt:      r.CreatedAt,
		}
	}
	return out, nil
}

// ListMemoriesForEntity returns active memories linked to a given entity.
func ListMemoriesForEntity(ctx context.Context, pool *pgxpool.Pool, entityID uuid.UUID, limit int) ([]*db.Memory, error) {
	q := db.New(pool)
	rows, err := q.ListMemoriesForEntity(ctx, db.ListMemoriesForEntityParams{
		EntityID: entityID,
		Limit:    int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list memories for entity: %w", err)
	}
	out := make([]*db.Memory, len(rows))
	for i, r := range rows {
		out[i] = memoryFromListMemoriesForEntityRow(r)
	}
	return out, nil
}

// ListRelationsByScope returns all relations in a scope.
func ListRelationsByScope(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]*db.Relation, error) {
	q := db.New(pool)
	rs, err := q.ListRelationsByScope(ctx, scopeID)
	if err != nil {
		return nil, fmt.Errorf("db: list relations by scope: %w", err)
	}
	out := make([]*db.Relation, len(rs))
	for i, r := range rs {
		out[i] = &db.Relation{
			ID:             r.ID,
			ScopeID:        r.ScopeID,
			SubjectID:      r.SubjectID,
			Predicate:      r.Predicate,
			ObjectID:       r.ObjectID,
			Confidence:     r.Confidence,
			SourceMemory:   r.SourceMemory,
			SourceArtifact: r.SourceArtifact,
			CreatedAt:      r.CreatedAt,
		}
	}
	return out, nil
}
