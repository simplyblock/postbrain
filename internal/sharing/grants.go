// Package sharing provides grant-based access control for memories and
// knowledge artifacts across scope boundaries.
package sharing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// Grant represents a sharing permission record.
type Grant struct {
	ID             uuid.UUID
	MemoryID       *uuid.UUID
	ArtifactID     *uuid.UUID
	GranteeScopeID uuid.UUID
	GrantedBy      uuid.UUID
	CanReshare     bool
	ExpiresAt      *time.Time
	CreatedAt      time.Time
}

// ErrInvalidGrant is returned when exactly one of MemoryID or ArtifactID is not set.
var ErrInvalidGrant = errors.New("sharing: exactly one of memory_id or artifact_id must be set")

// Store provides grant CRUD operations backed by PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a new Store backed by the given pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// Create validates and inserts a sharing grant.
// Exactly one of g.MemoryID or g.ArtifactID must be non-nil.
func (s *Store) Create(ctx context.Context, g *Grant) (*Grant, error) {
	if (g.MemoryID == nil) == (g.ArtifactID == nil) {
		return nil, ErrInvalidGrant
	}

	row, err := db.New(s.pool).CreateSharingGrant(ctx, db.CreateSharingGrantParams{
		MemoryID:       g.MemoryID,
		ArtifactID:     g.ArtifactID,
		GranteeScopeID: g.GranteeScopeID,
		GrantedBy:      g.GrantedBy,
		CanReshare:     g.CanReshare,
		ExpiresAt:      g.ExpiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("sharing: create grant: %w", err)
	}
	return grantFromDB(row), nil
}

// GetByID retrieves a sharing grant by its ID. Returns nil, nil if not found.
func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*Grant, error) {
	g, err := db.New(s.pool).GetSharingGrant(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("sharing: get grant: %w", err)
	}
	return grantFromDB(g), nil
}

// Revoke deletes a sharing grant by ID.
func (s *Store) Revoke(ctx context.Context, grantID uuid.UUID) error {
	if err := db.New(s.pool).RevokeSharingGrant(ctx, grantID); err != nil {
		return fmt.Errorf("sharing: revoke grant: %w", err)
	}
	return nil
}

// List returns sharing grants visible to a grantee scope, paginated.
func (s *Store) List(ctx context.Context, granteeScopeID uuid.UUID, limit, offset int) ([]*Grant, error) {
	rows, err := db.New(s.pool).ListSharingGrants(ctx, db.ListSharingGrantsParams{
		GranteeScopeID: granteeScopeID,
		Limit:          int32(limit),
		Offset:         int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("sharing: list grants: %w", err)
	}
	grants := make([]*Grant, len(rows))
	for i, r := range rows {
		grants[i] = grantFromDB(r)
	}
	return grants, nil
}

// IsMemoryAccessible checks whether memoryID has been granted to requesterScopeID.
func (s *Store) IsMemoryAccessible(ctx context.Context, memoryID, requesterScopeID uuid.UUID) (bool, error) {
	exists, err := db.New(s.pool).IsMemoryGranted(ctx, db.IsMemoryGrantedParams{
		MemoryID:       &memoryID,
		GranteeScopeID: requesterScopeID,
	})
	if err != nil {
		return false, fmt.Errorf("sharing: is memory accessible: %w", err)
	}
	return exists, nil
}

// IsArtifactAccessible checks whether artifactID has been granted to requesterScopeID.
func (s *Store) IsArtifactAccessible(ctx context.Context, artifactID, requesterScopeID uuid.UUID) (bool, error) {
	exists, err := db.New(s.pool).IsArtifactGranted(ctx, db.IsArtifactGrantedParams{
		ArtifactID:     &artifactID,
		GranteeScopeID: requesterScopeID,
	})
	if err != nil {
		return false, fmt.Errorf("sharing: is artifact accessible: %w", err)
	}
	return exists, nil
}

func grantFromDB(g *db.SharingGrant) *Grant {
	return &Grant{
		ID:             g.ID,
		MemoryID:       g.MemoryID,
		ArtifactID:     g.ArtifactID,
		GranteeScopeID: g.GranteeScopeID,
		GrantedBy:      g.GrantedBy,
		CanReshare:     g.CanReshare,
		ExpiresAt:      g.ExpiresAt,
		CreatedAt:      g.CreatedAt,
	}
}
