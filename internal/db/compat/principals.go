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

// CreatePrincipal inserts a new principal row and returns the created record.
func CreatePrincipal(ctx context.Context, pool *pgxpool.Pool, kind, slug, displayName string, meta []byte) (*db.Principal, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	q := db.New(pool)
	p, err := q.CreatePrincipal(ctx, db.CreatePrincipalParams{
		Kind:        kind,
		Slug:        slug,
		DisplayName: displayName,
		Meta:        meta,
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// GetPrincipalByID looks up a principal by its UUID. Returns nil, nil if not found.
func GetPrincipalByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*db.Principal, error) {
	q := db.New(pool)
	p, err := q.GetPrincipalByID(ctx, id)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// GetPrincipalBySlug looks up a principal by slug. Returns nil, nil if not found.
func GetPrincipalBySlug(ctx context.Context, pool *pgxpool.Pool, slug string) (*db.Principal, error) {
	q := db.New(pool)
	p, err := q.GetPrincipalBySlug(ctx, slug)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// ListPrincipals returns principals ordered by creation time.
func ListPrincipals(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]*db.Principal, error) {
	if limit < 0 || limit > math.MaxInt32 {
		return nil, fmt.Errorf("db: invalid limit: %d", limit)
	}
	if offset < 0 || offset > math.MaxInt32 {
		return nil, fmt.Errorf("db: invalid offset: %d", offset)
	}
	q := db.New(pool)
	ps, err := q.ListPrincipals(ctx, db.ListPrincipalsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("db: list principals: %w", err)
	}
	return ps, nil
}

// CreateMembership inserts a membership record.
// grantedBy may be nil; the column is nullable in the schema.
func CreateMembership(ctx context.Context, pool *pgxpool.Pool, memberID, parentID uuid.UUID, role string, grantedBy *uuid.UUID) (*db.Membership, error) {
	const q = `
INSERT INTO principal_memberships (member_id, parent_id, role, granted_by)
VALUES ($1, $2, $3, NULLIF($4, '00000000-0000-0000-0000-000000000000'::uuid))
RETURNING member_id, parent_id, role, granted_by, created_at`
	var gb uuid.UUID
	if grantedBy != nil {
		gb = *grantedBy
	}
	row := pool.QueryRow(ctx, q, memberID, parentID, role, gb)
	var m db.Membership
	if err := row.Scan(&m.MemberID, &m.ParentID, &m.Role, &m.GrantedBy, &m.CreatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

// DeleteMembership removes a direct membership.
func DeleteMembership(ctx context.Context, pool *pgxpool.Pool, memberID, parentID uuid.UUID) error {
	q := db.New(pool)
	return q.DeleteMembership(ctx, db.DeleteMembershipParams{
		MemberID: memberID,
		ParentID: parentID,
	})
}

// GetMemberships returns direct parent memberships for a principal.
func GetMemberships(ctx context.Context, pool *pgxpool.Pool, memberID uuid.UUID) ([]*db.Membership, error) {
	q := db.New(pool)
	return q.GetMemberships(ctx, memberID)
}

// GetAllParentIDs returns all ancestor principal IDs via recursive CTE.
func GetAllParentIDs(ctx context.Context, pool *pgxpool.Pool, memberID uuid.UUID) ([]uuid.UUID, error) {
	q := db.New(pool)
	return q.GetAllParentIDs(ctx, memberID)
}

// ListAllMemberships returns all memberships with member and parent display names.
func ListAllMemberships(ctx context.Context, pool *pgxpool.Pool) ([]*db.MembershipRow, error) {
	const query = `
SELECT pm.member_id, mp.slug, mp.display_name,
       pm.parent_id, pp.slug, pp.display_name,
       pm.role, pm.created_at
FROM principal_memberships pm
JOIN principals mp ON mp.id = pm.member_id
JOIN principals pp ON pp.id = pm.parent_id
ORDER BY pm.created_at DESC`
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("db: list all memberships: %w", err)
	}
	defer rows.Close()
	var items []*db.MembershipRow
	for rows.Next() {
		var r db.MembershipRow
		if err := rows.Scan(&r.MemberID, &r.MemberSlug, &r.MemberDisplayName,
			&r.ParentID, &r.ParentSlug, &r.ParentDisplayName,
			&r.Role, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("db: list all memberships scan: %w", err)
		}
		items = append(items, &r)
	}
	return items, rows.Err()
}
