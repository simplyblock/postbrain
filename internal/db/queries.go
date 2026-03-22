package db

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// scanPrincipal scans a row into a Principal struct.
func scanPrincipal(row pgx.Row) (*Principal, error) {
	var p Principal
	err := row.Scan(
		&p.ID,
		&p.Kind,
		&p.Slug,
		&p.DisplayName,
		&p.Meta,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePrincipal inserts a new principal row and returns the created record.
func CreatePrincipal(ctx context.Context, pool *pgxpool.Pool, kind, slug, displayName string, meta []byte) (*Principal, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	row := pool.QueryRow(ctx,
		`INSERT INTO principals (kind, slug, display_name, meta)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, kind, slug, display_name, meta, created_at, updated_at`,
		kind, slug, displayName, meta,
	)
	return scanPrincipal(row)
}

// GetPrincipalByID looks up a principal by its UUID.
func GetPrincipalByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Principal, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, kind, slug, display_name, meta, created_at, updated_at
		 FROM principals WHERE id = $1`,
		id,
	)
	p, err := scanPrincipal(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// GetPrincipalBySlug looks up a principal by its slug (case-insensitive).
func GetPrincipalBySlug(ctx context.Context, pool *pgxpool.Pool, slug string) (*Principal, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, kind, slug, display_name, meta, created_at, updated_at
		 FROM principals WHERE slug = $1`,
		slug,
	)
	p, err := scanPrincipal(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// CreateMembership inserts a membership record linking memberID to parentID.
func CreateMembership(ctx context.Context, pool *pgxpool.Pool, memberID, parentID uuid.UUID, role string, grantedBy *uuid.UUID) (*Membership, error) {
	var m Membership
	err := pool.QueryRow(ctx,
		`INSERT INTO principal_memberships (member_id, parent_id, role, granted_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING member_id, parent_id, role, granted_by, created_at`,
		memberID, parentID, role, grantedBy,
	).Scan(&m.MemberID, &m.ParentID, &m.Role, &m.GrantedBy, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DeleteMembership removes a direct membership between memberID and parentID.
func DeleteMembership(ctx context.Context, pool *pgxpool.Pool, memberID, parentID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`DELETE FROM principal_memberships WHERE member_id = $1 AND parent_id = $2`,
		memberID, parentID,
	)
	return err
}

// GetMemberships returns the direct parent memberships for a given principal.
func GetMemberships(ctx context.Context, pool *pgxpool.Pool, memberID uuid.UUID) ([]*Membership, error) {
	rows, err := pool.Query(ctx,
		`SELECT member_id, parent_id, role, granted_by, created_at
		 FROM principal_memberships WHERE member_id = $1`,
		memberID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memberships []*Membership
	for rows.Next() {
		var m Membership
		if err := rows.Scan(&m.MemberID, &m.ParentID, &m.Role, &m.GrantedBy, &m.CreatedAt); err != nil {
			return nil, err
		}
		memberships = append(memberships, &m)
	}
	return memberships, rows.Err()
}

// GetAllParentIDs returns all ancestor principal IDs for a given principal via recursive CTE.
// The result includes the principal itself.
func GetAllParentIDs(ctx context.Context, pool *pgxpool.Pool, memberID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx,
		`WITH RECURSIVE member_tree AS (
		     SELECT $1::uuid AS id
		     UNION ALL
		     SELECT pm.parent_id
		     FROM   principal_memberships pm
		     JOIN   member_tree mt ON pm.member_id = mt.id
		 )
		 SELECT id FROM member_tree`,
		memberID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CreateScope inserts a new scope row. The path is computed by the database trigger.
func CreateScope(ctx context.Context, pool *pgxpool.Pool, kind, externalID, name string, parentID *uuid.UUID, principalID uuid.UUID, meta []byte) (*Scope, error) {
	if meta == nil {
		meta = []byte("{}")
	}
	var s Scope
	err := pool.QueryRow(ctx,
		`INSERT INTO scopes (kind, external_id, name, parent_id, principal_id, meta, path)
		 VALUES ($1, $2, $3, $4, $5, $6, 'placeholder')
		 RETURNING id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at`,
		kind, externalID, name, parentID, principalID, meta,
	).Scan(&s.ID, &s.Kind, &s.ExternalID, &s.Name, &s.ParentID, &s.PrincipalID, &s.Path, &s.Meta, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetScopeByID retrieves a scope by its UUID.
func GetScopeByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (*Scope, error) {
	var s Scope
	err := pool.QueryRow(ctx,
		`SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at
		 FROM scopes WHERE id = $1`,
		id,
	).Scan(&s.ID, &s.Kind, &s.ExternalID, &s.Name, &s.ParentID, &s.PrincipalID, &s.Path, &s.Meta, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetScopeByExternalID retrieves a scope by its kind and external_id (case-insensitive).
func GetScopeByExternalID(ctx context.Context, pool *pgxpool.Pool, kind, externalID string) (*Scope, error) {
	var s Scope
	err := pool.QueryRow(ctx,
		`SELECT id, kind, external_id, name, parent_id, principal_id, path::text, meta, created_at
		 FROM scopes WHERE kind = $1 AND external_id = $2`,
		kind, externalID,
	).Scan(&s.ID, &s.Kind, &s.ExternalID, &s.Name, &s.ParentID, &s.PrincipalID, &s.Path, &s.Meta, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetAncestorScopeIDs returns all ancestor scope IDs of a given scope (including itself)
// using the ltree @> operator.
func GetAncestorScopeIDs(ctx context.Context, pool *pgxpool.Pool, scopeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := pool.Query(ctx,
		`SELECT s2.id FROM scopes s1
		 JOIN scopes s2 ON s2.path @> s1.path
		 WHERE s1.id = $1`,
		scopeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// scanToken scans a token row. scopeIDs is stored as a PostgreSQL UUID array.
func scanToken(row pgx.Row) (*Token, error) {
	var t Token
	var scopeIDs []uuid.UUID
	var permissions []string
	err := row.Scan(
		&t.ID,
		&t.PrincipalID,
		&t.TokenHash,
		&t.Name,
		&scopeIDs,
		&permissions,
		&t.ExpiresAt,
		&t.LastUsedAt,
		&t.CreatedAt,
		&t.RevokedAt,
	)
	if err != nil {
		return nil, err
	}
	t.ScopeIDs = scopeIDs
	t.Permissions = permissions
	return &t, nil
}

// CreateToken inserts a new token record. The raw token is never stored; only the hash.
func CreateToken(ctx context.Context, pool *pgxpool.Pool, principalID uuid.UUID, tokenHash, name string, scopeIDs []uuid.UUID, permissions []string, expiresAt *time.Time) (*Token, error) {
	if len(permissions) == 0 {
		permissions = []string{"read"}
	}

	row := pool.QueryRow(ctx,
		`INSERT INTO tokens (principal_id, token_hash, name, scope_ids, permissions, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, principal_id, token_hash, name, scope_ids, permissions, expires_at, last_used_at, created_at, revoked_at`,
		principalID, tokenHash, name, scopeIDs, permissions, expiresAt,
	)
	t, err := scanToken(row)
	if err != nil {
		return nil, fmt.Errorf("db: create token: %w", err)
	}
	return t, nil
}

// LookupToken finds a token by its hash. Returns nil, nil if not found.
// Does not filter by revoked_at or expires_at — callers must check those fields.
func LookupToken(ctx context.Context, pool *pgxpool.Pool, tokenHash string) (*Token, error) {
	row := pool.QueryRow(ctx,
		`SELECT id, principal_id, token_hash, name, scope_ids, permissions, expires_at, last_used_at, created_at, revoked_at
		 FROM tokens WHERE token_hash = $1`,
		tokenHash,
	)
	t, err := scanToken(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: lookup token: %w", err)
	}
	return t, nil
}

// RevokeToken soft-revokes a token by setting revoked_at = now().
func RevokeToken(ctx context.Context, pool *pgxpool.Pool, tokenID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`UPDATE tokens SET revoked_at = now() WHERE id = $1`,
		tokenID,
	)
	return err
}

// UpdateTokenLastUsed sets last_used_at = now() for a token.
func UpdateTokenLastUsed(ctx context.Context, pool *pgxpool.Pool, tokenID uuid.UUID) error {
	_, err := pool.Exec(ctx,
		`UPDATE tokens SET last_used_at = now() WHERE id = $1`,
		tokenID,
	)
	return err
}
