package oauth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

type tokenCreator interface {
	Create(ctx context.Context, principalID uuid.UUID, hash, name string, scopeIDs []uuid.UUID, permissions []string, expiresAt *time.Time) (*db.Token, error)
}

// Issuer exchanges an authorization code into a Postbrain bearer token.
type Issuer struct {
	tokenStore    tokenCreator
	generateToken func() (raw string, hash string, err error)
	now           func() time.Time
}

// NewIssuer constructs an Issuer backed by auth.TokenStore.
func NewIssuer(tokenStore *auth.TokenStore) *Issuer {
	return &Issuer{
		tokenStore:    tokenStore,
		generateToken: auth.GenerateToken,
		now:           time.Now,
	}
}

// Issue creates a token for the principal with permissions derived from scopes.
func (i *Issuer) Issue(ctx context.Context, principalID uuid.UUID, scopes []string, ttl time.Duration) (string, error) {
	raw, hash, err := i.generateToken()
	if err != nil {
		return "", err
	}

	permissions := ScopeToPermissions(scopes)
	var expiresAt *time.Time
	if ttl > 0 {
		t := i.now().Add(ttl)
		expiresAt = &t
	}

	if _, err := i.tokenStore.Create(ctx, principalID, hash, "oauth", nil, permissions, expiresAt); err != nil {
		return "", err
	}
	return raw, nil
}
