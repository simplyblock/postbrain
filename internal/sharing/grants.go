// Package sharing provides grant-based access control for memories and
// knowledge artifacts across scope boundaries.
package sharing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

	var result Grant
	err := s.pool.QueryRow(ctx,
		`INSERT INTO sharing_grants (memory_id, artifact_id, grantee_scope_id, granted_by, can_reshare, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, memory_id, artifact_id, grantee_scope_id, granted_by, can_reshare, expires_at, created_at`,
		g.MemoryID, g.ArtifactID, g.GranteeScopeID, g.GrantedBy, g.CanReshare, g.ExpiresAt,
	).Scan(
		&result.ID, &result.MemoryID, &result.ArtifactID,
		&result.GranteeScopeID, &result.GrantedBy, &result.CanReshare,
		&result.ExpiresAt, &result.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("sharing: create grant: %w", err)
	}
	return &result, nil
}

// Revoke deletes a sharing grant by ID.
func (s *Store) Revoke(ctx context.Context, grantID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`DELETE FROM sharing_grants WHERE id=$1`, grantID,
	)
	if err != nil {
		return fmt.Errorf("sharing: revoke grant: %w", err)
	}
	return nil
}

// List returns sharing grants visible to a grantee scope, paginated.
func (s *Store) List(ctx context.Context, granteeScopeID uuid.UUID, limit, offset int) ([]*Grant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, memory_id, artifact_id, grantee_scope_id, granted_by, can_reshare, expires_at, created_at
		 FROM sharing_grants WHERE grantee_scope_id=$1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		granteeScopeID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("sharing: list grants: %w", err)
	}
	defer rows.Close()

	var grants []*Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(
			&g.ID, &g.MemoryID, &g.ArtifactID,
			&g.GranteeScopeID, &g.GrantedBy, &g.CanReshare,
			&g.ExpiresAt, &g.CreatedAt,
		); err != nil {
			return nil, err
		}
		grants = append(grants, &g)
	}
	return grants, rows.Err()
}

// IsMemoryAccessible checks whether memoryID has been granted to requesterScopeID.
func (s *Store) IsMemoryAccessible(ctx context.Context, memoryID, requesterScopeID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM sharing_grants
		    WHERE memory_id = $1
		      AND grantee_scope_id = $2
		      AND (expires_at IS NULL OR expires_at > now())
		)`,
		memoryID, requesterScopeID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sharing: is memory accessible: %w", err)
	}
	return exists, nil
}

// IsArtifactAccessible checks whether artifactID has been granted to requesterScopeID.
func (s *Store) IsArtifactAccessible(ctx context.Context, artifactID, requesterScopeID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (
		    SELECT 1 FROM sharing_grants
		    WHERE artifact_id = $1
		      AND grantee_scope_id = $2
		      AND (expires_at IS NULL OR expires_at > now())
		)`,
		artifactID, requesterScopeID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sharing: is artifact accessible: %w", err)
	}
	return exists, nil
}
