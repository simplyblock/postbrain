// Package principals provides management of principals (agents, users, teams,
// departments, and companies) and their memberships within the Postbrain system.
package principals

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
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
	p, err := db.CreatePrincipal(ctx, s.pool, kind, slug, displayName, meta)
	if err != nil {
		return nil, fmt.Errorf("principals: create: %w", err)
	}
	return p, nil
}

// GetByID retrieves a principal by UUID. Returns nil, nil if not found.
func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*db.Principal, error) {
	p, err := db.GetPrincipalByID(ctx, s.pool, id)
	if err != nil {
		return nil, fmt.Errorf("principals: get by id: %w", err)
	}
	return p, nil
}

// GetBySlug retrieves a principal by slug (case-insensitive). Returns nil, nil if not found.
func (s *Store) GetBySlug(ctx context.Context, slug string) (*db.Principal, error) {
	p, err := db.GetPrincipalBySlug(ctx, s.pool, slug)
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
	var p db.Principal
	err := s.pool.QueryRow(ctx,
		`UPDATE principals SET display_name=$2, meta=$3, updated_at=now()
		 WHERE id=$1
		 RETURNING id, kind, slug, display_name, meta, created_at, updated_at`,
		id, displayName, meta,
	).Scan(&p.ID, &p.Kind, &p.Slug, &p.DisplayName, &p.Meta, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("principals: update: %w", err)
	}
	return &p, nil
}

// Delete removes a principal by UUID.
func (s *Store) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM principals WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("principals: delete: %w", err)
	}
	return nil
}
