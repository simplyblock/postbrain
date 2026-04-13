package oauth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/config"
	"github.com/simplyblock/postbrain/internal/db"
)

type serverClientStore interface {
	LookupByClientID(ctx context.Context, clientID string) (*OAuthClient, error)
	ValidateRedirectURI(client *OAuthClient, redirectURI string) error
	Register(ctx context.Context, req RegisterRequest) (*OAuthClient, string, error)
}

type serverCodeStore interface {
	Consume(ctx context.Context, rawCode string) (*AuthCode, error)
	VerifyPKCE(code *AuthCode, verifier string) error
}

type serverStateStore interface {
	Issue(ctx context.Context, kind string, payload map[string]any, ttl time.Duration) (string, error)
}

type serverTokenStore interface {
	Lookup(ctx context.Context, hash string) (*db.Token, error)
	Revoke(ctx context.Context, tokenID uuid.UUID) error
}

type serverIssuer interface {
	Issue(ctx context.Context, principalID uuid.UUID, scopes []string, ttl time.Duration) (string, error)
}

// Server hosts OAuth authorization server routes.
type Server struct {
	clients serverClientStore
	codes   serverCodeStore
	states  serverStateStore
	issuer  serverIssuer
	tokens  serverTokenStore
	cfg     config.OAuthConfig
	limiter *registrationLimiter
}

// NewServer constructs an OAuth server.
func NewServer(clients serverClientStore, codes serverCodeStore, states serverStateStore, issuer serverIssuer, tokens serverTokenStore, cfg config.OAuthConfig) *Server {
	return &Server{
		clients: clients,
		codes:   codes,
		states:  states,
		issuer:  issuer,
		tokens:  tokens,
		cfg:     cfg,
		limiter: newRegistrationLimiter(),
	}
}
