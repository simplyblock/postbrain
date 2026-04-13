package compat

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/db"
)

// CreateToken inserts a new token record.
func CreateToken(ctx context.Context, pool *pgxpool.Pool, principalID uuid.UUID, tokenHash, name string, scopeIDs []uuid.UUID, permissions []string, expiresAt *time.Time) (*db.Token, error) {
	if len(permissions) == 0 {
		permissions = []string{"read", "write", "edit", "delete"}
	}
	q := db.New(pool)
	t, err := q.CreateToken(ctx, db.CreateTokenParams{
		PrincipalID: principalID,
		TokenHash:   tokenHash,
		Name:        name,
		ScopeIds:    scopeIDs,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("db: create token: %w", err)
	}
	return t, nil
}

// LookupToken finds a token by hash. Returns nil, nil if not found.
func LookupToken(ctx context.Context, pool *pgxpool.Pool, tokenHash string) (*db.Token, error) {
	q := db.New(pool)
	t, err := q.LookupToken(ctx, tokenHash)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("db: lookup token: %w", err)
	}
	return t, nil
}

// RevokeToken soft-revokes a token.
func RevokeToken(ctx context.Context, pool *pgxpool.Pool, tokenID uuid.UUID) error {
	q := db.New(pool)
	return q.RevokeToken(ctx, tokenID)
}

// UpdateTokenScopes updates scope_ids for a token owned by principalID.
// Returns true when a token row was updated.
func UpdateTokenScopes(ctx context.Context, pool *pgxpool.Pool, tokenID, principalID uuid.UUID, scopeIDs []uuid.UUID) (bool, error) {
	ct, err := pool.Exec(ctx,
		`UPDATE tokens
		 SET scope_ids = $1
		 WHERE id = $2
		   AND principal_id = $3
		   AND revoked_at IS NULL`,
		scopeIDs, tokenID, principalID,
	)
	if err != nil {
		return false, fmt.Errorf("db: update token scopes: %w", err)
	}
	return ct.RowsAffected() == 1, nil
}

// UpdateTokenLastUsed sets last_used_at = now().
func UpdateTokenLastUsed(ctx context.Context, pool *pgxpool.Pool, tokenID uuid.UUID) error {
	q := db.New(pool)
	return q.UpdateTokenLastUsed(ctx, tokenID)
}

// ListTokens returns all tokens, optionally filtered to a single principal.
// Pass nil to list all tokens across all principals.
func ListTokens(ctx context.Context, pool *pgxpool.Pool, principalID *uuid.UUID) ([]*db.Token, error) {
	q := db.New(pool)
	if principalID == nil {
		return q.ListAllTokens(ctx)
	}
	return q.ListTokensByPrincipal(ctx, *principalID)
}
