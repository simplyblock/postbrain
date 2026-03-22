package knowledge

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

var validVisibilities = map[string]struct{}{
	"private":    {},
	"project":    {},
	"team":       {},
	"department": {},
	"company":    {},
}

// CollectionStore provides CRUD operations for knowledge collections.
type CollectionStore struct {
	pool *pgxpool.Pool
}

// NewCollectionStore creates a new CollectionStore backed by the given pool.
func NewCollectionStore(pool *pgxpool.Pool) *CollectionStore {
	return &CollectionStore{pool: pool}
}

// Create creates a new knowledge collection. Visibility must be one of
// private|project|team|department|company.
func (c *CollectionStore) Create(ctx context.Context, scopeID, ownerID uuid.UUID, slug, name, visibility string, description *string) (*db.KnowledgeCollection, error) {
	if _, ok := validVisibilities[visibility]; !ok {
		return nil, fmt.Errorf("knowledge: invalid visibility: %s", visibility)
	}
	coll := &db.KnowledgeCollection{
		ScopeID:     scopeID,
		OwnerID:     ownerID,
		Slug:        slug,
		Name:        name,
		Description: description,
		Visibility:  visibility,
	}
	result, err := db.CreateCollection(ctx, c.pool, coll)
	if err != nil {
		return nil, fmt.Errorf("knowledge: create collection: %w", err)
	}
	return result, nil
}

// GetByID retrieves a collection by ID. Returns nil, nil if not found.
func (c *CollectionStore) GetByID(ctx context.Context, id uuid.UUID) (*db.KnowledgeCollection, error) {
	result, err := db.GetCollection(ctx, c.pool, id)
	if err != nil {
		return nil, fmt.Errorf("knowledge: get collection: %w", err)
	}
	return result, nil
}

// GetBySlug retrieves a collection by scope + slug. Returns nil, nil if not found.
func (c *CollectionStore) GetBySlug(ctx context.Context, scopeID uuid.UUID, slug string) (*db.KnowledgeCollection, error) {
	result, err := db.GetCollectionBySlug(ctx, c.pool, scopeID, slug)
	if err != nil {
		return nil, fmt.Errorf("knowledge: get collection by slug: %w", err)
	}
	return result, nil
}

// List returns all collections for a given scope.
func (c *CollectionStore) List(ctx context.Context, scopeID uuid.UUID) ([]*db.KnowledgeCollection, error) {
	results, err := db.ListCollections(ctx, c.pool, scopeID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: list collections: %w", err)
	}
	return results, nil
}

// AddItem adds an artifact to a collection.
func (c *CollectionStore) AddItem(ctx context.Context, collectionID, artifactID, addedBy uuid.UUID) error {
	if err := db.AddCollectionItem(ctx, c.pool, collectionID, artifactID, addedBy); err != nil {
		return fmt.Errorf("knowledge: add collection item: %w", err)
	}
	return nil
}

// RemoveItem removes an artifact from a collection.
func (c *CollectionStore) RemoveItem(ctx context.Context, collectionID, artifactID uuid.UUID) error {
	if err := db.RemoveCollectionItem(ctx, c.pool, collectionID, artifactID); err != nil {
		return fmt.Errorf("knowledge: remove collection item: %w", err)
	}
	return nil
}

// ListItems returns the artifacts in a collection ordered by position.
func (c *CollectionStore) ListItems(ctx context.Context, collectionID uuid.UUID) ([]*db.KnowledgeArtifact, error) {
	results, err := db.ListCollectionItems(ctx, c.pool, collectionID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: list collection items: %w", err)
	}
	return results, nil
}
