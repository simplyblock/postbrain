// Package auth provides token generation, hashing, lookup, and HTTP middleware
// for bearer token authentication in Postbrain.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

// HashToken returns the hex-encoded SHA-256 digest of raw.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// GenerateToken generates a new random bearer token.
// The raw token format is "pb_" followed by 64 hex characters (32 random bytes).
// Returns the raw token, its SHA-256 hash, and any error from the PRNG.
func GenerateToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: generate token: %w", err)
	}
	raw = "pb_" + hex.EncodeToString(b)
	hash = HashToken(raw)
	return raw, hash, nil
}

// TokenStore provides token lifecycle operations backed by a pgx pool.
type TokenStore struct {
	pool *pgxpool.Pool
}

// NewTokenStore creates a new TokenStore backed by the given connection pool.
func NewTokenStore(pool *pgxpool.Pool) *TokenStore {
	return &TokenStore{pool: pool}
}

// Lookup finds a valid (non-revoked, non-expired) token by its hash.
// Returns nil, nil if the token is not found.
func (ts *TokenStore) Lookup(ctx context.Context, hash string) (*db.Token, error) {
	t, err := db.LookupToken(ctx, ts.pool, hash)
	if err != nil {
		return nil, fmt.Errorf("auth: lookup: %w", err)
	}
	if t == nil {
		return nil, nil
	}
	// Check revocation.
	if t.RevokedAt != nil {
		return nil, nil
	}
	// Check expiry.
	if !t.ExpiresAt.IsZero() && t.ExpiresAt.Before(time.Now()) {
		return nil, nil
	}
	return t, nil
}

// Create creates a new token record. The caller provides the hash (not the raw value).
func (ts *TokenStore) Create(ctx context.Context, principalID uuid.UUID, hash, name string, scopeIDs []uuid.UUID, permissions []string, expiresAt *time.Time) (*db.Token, error) {
	t, err := db.CreateToken(ctx, ts.pool, principalID, hash, name, scopeIDs, permissions, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("auth: create token: %w", err)
	}
	return t, nil
}

// Revoke soft-revokes a token by setting revoked_at.
func (ts *TokenStore) Revoke(ctx context.Context, tokenID uuid.UUID) error {
	if err := db.RevokeToken(ctx, ts.pool, tokenID); err != nil {
		return fmt.Errorf("auth: revoke token: %w", err)
	}
	return nil
}

// UpdateLastUsed updates last_used_at asynchronously (fire-and-forget goroutine).
// Does not block; errors are logged via slog.
// If pool is nil, this is a no-op (used in tests).
func (ts *TokenStore) UpdateLastUsed(pool *pgxpool.Pool, tokenID uuid.UUID) {
	if pool == nil {
		return
	}
	go func() {
		if err := db.UpdateTokenLastUsed(context.Background(), pool, tokenID); err != nil {
			slog.Error("auth: update last used", "token_id", tokenID, "error", err)
		}
	}()
}

// EnforceScopeAccess returns an error if the token's ScopeIDs is non-nil and
// does not contain requestedScopeID. A nil ScopeIDs list grants access to all scopes.
func EnforceScopeAccess(token *db.Token, requestedScopeID uuid.UUID) error {
	if token.ScopeIds == nil {
		return nil
	}
	for _, id := range token.ScopeIds {
		if id == requestedScopeID {
			return nil
		}
	}
	return errors.New("auth: token does not have access to requested scope")
}
