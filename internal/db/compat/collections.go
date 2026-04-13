package compat

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// CreateCollection inserts a new knowledge collection.
func CreateCollection(ctx context.Context, pool *pgxpool.Pool, c *db.KnowledgeCollection) (*db.KnowledgeCollection, error) {
	if c.Meta == nil {
		c.Meta = []byte("{}")
	}
	q := db.New(pool)
	result, err := q.CreateCollection(ctx, db.CreateCollectionParams{
		ScopeID:     c.ScopeID,
		OwnerID:     c.OwnerID,
		Slug:        c.Slug,
		Name:        c.Name,
		Description: c.Description,
		Visibility:  c.Visibility,
		Meta:        c.Meta,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create collection: %w", err)
	}
	return result, nil
}

// GetCollection retrieves a collection by ID. Returns nil, nil if not found.
func GetCollection(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*db.KnowledgeCollection, error) {
	q := db.New(pool)
	c, err := q.GetCollection(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get collection: %w", err)
	}
	return c, nil
}

// GetCollectionBySlug retrieves a collection by scope + slug. Returns nil, nil if not found.
func GetCollectionBySlug(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID, slug string) (*db.KnowledgeCollection, error) {
	q := db.New(pool)
	c, err := q.GetCollectionBySlug(ctx, db.GetCollectionBySlugParams{
		ScopeID: scopeID,
		Slug:    slug,
	})
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: get collection by slug: %w", err)
	}
	return c, nil
}

// ListCollections returns all collections for a scope.
func ListCollections(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]*db.KnowledgeCollection, error) {
	q := db.New(pool)
	cs, err := q.ListCollections(ctx, scopeID)
	if err != nil {
		return nil, err
	}
	return cs, nil
}

// ListAllCollections returns all collections across all scopes, ordered by name.
func ListAllCollections(ctx context.Context, pool *pgxpool.Pool) ([]*db.KnowledgeCollection, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, scope_id, owner_id, slug, name, description, visibility, meta, created_at, updated_at
		 FROM knowledge_collections ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("db: list all collections: %w", err)
	}
	defer rows.Close()
	var cs []*db.KnowledgeCollection
	for rows.Next() {
		var c db.KnowledgeCollection
		if err := rows.Scan(&c.ID, &c.ScopeID, &c.OwnerID, &c.Slug, &c.Name, &c.Description,
			&c.Visibility, &c.Meta, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("db: list all collections scan: %w", err)
		}
		cs = append(cs, &c)
	}
	return cs, rows.Err()
}

// AddCollectionItem inserts a knowledge_collection_items row.
func AddCollectionItem(ctx context.Context, pool *pgxpool.Pool, collectionID, artifactID, addedBy uuid.UUID) error {
	q := db.New(pool)
	return q.AddCollectionItem(ctx, db.AddCollectionItemParams{
		CollectionID: collectionID,
		ArtifactID:   artifactID,
		AddedBy:      addedBy,
	})
}

// RemoveCollectionItem deletes a knowledge_collection_items row.
func RemoveCollectionItem(ctx context.Context, pool *pgxpool.Pool, collectionID, artifactID uuid.UUID) error {
	q := db.New(pool)
	return q.RemoveCollectionItem(ctx, db.RemoveCollectionItemParams{
		CollectionID: collectionID,
		ArtifactID:   artifactID,
	})
}

// ListCollectionItems returns the artifacts in a collection.
func ListCollectionItems(ctx context.Context, pool *pgxpool.Pool, collectionID uuid.UUID) ([]*db.KnowledgeArtifact, error) {
	q := db.New(pool)
	as, err := q.ListCollectionItems(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	out := make([]*db.KnowledgeArtifact, len(as))
	for i, r := range as {
		out[i] = knowledgeArtifactFromListCollectionItemsRow(r)
	}
	return out, nil
}
