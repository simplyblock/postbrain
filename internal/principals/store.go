// Package principals provides management of principals (agents, users, teams,
// departments, and companies) and their memberships within the Postbrain system.
package principals

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/db/compat"
)

// Store provides CRUD operations for principals.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new Store backed by the given connection pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create inserts a new principal and returns the created record.
func (s *Store) Create(ctx context.Context, kind, slug, displayName string, meta []byte) (*db.Principal, error) {
	p, err := compat.CreatePrincipal(ctx, s.pool, kind, slug, displayName, meta)
	if err != nil {
		return nil, fmt.Errorf("principals: create: %w", err)
	}
	return p, nil
}

// GetByID retrieves a principal by UUID. Returns nil, nil if not found.
func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*db.Principal, error) {
	p, err := compat.GetPrincipalByID(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("principals: get by id: %w", err)
	}
	return p, nil
}

// GetBySlug retrieves a principal by slug (case-insensitive). Returns nil, nil if not found.
func (s *Store) GetBySlug(ctx context.Context, slug string) (*db.Principal, error) {
	p, err := compat.GetPrincipalBySlug(ctx, s.pool, slug)
	if err != nil {
		return nil, fmt.Errorf("principals: get by slug: %w", err)
	}
	return p, nil
}

// Update updates the display_name and meta of a principal, returning the updated record.
func (s *Store) Update(ctx context.Context, id uuid.UUID, displayName string, meta []byte) (*db.Principal, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	p, err := db.New(s.pool).UpdatePrincipal(ctx, db.UpdatePrincipalParams{
		ID:          id,
		DisplayName: displayName,
		Meta:        meta,
	})
	if err != nil {
		return nil, fmt.Errorf("principals: update: %w", err)
	}
	return p, nil
}

// UpdateProfile updates slug and display_name of a principal, returning the updated record.
func (s *Store) UpdateProfile(ctx context.Context, id uuid.UUID, slug, displayName string) (*db.Principal, error) {
	p, err := db.New(s.pool).UpdatePrincipalProfile(ctx, db.UpdatePrincipalProfileParams{
		ID:          id,
		Slug:        slug,
		DisplayName: displayName,
	})
	if err != nil {
		return nil, fmt.Errorf("principals: update profile: %w", err)
	}
	return p, nil
}

// List returns principals ordered by creation time, with pagination.
func (s *Store) List(ctx context.Context, limit, offset int) ([]*db.Principal, error) {
	ps, err := compat.ListPrincipals(ctx, s.pool, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("principals: list: %w", err)
	}
	return ps, nil
}

// Delete removes a principal by UUID.
func (s *Store) Delete(ctx context.Context, id uuid.UUID) error {
	if err := db.New(s.pool).DeletePrincipal(ctx, id); err != nil {
		return fmt.Errorf("principals: delete: %w", err)
	}
	return nil
}
