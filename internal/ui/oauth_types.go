package ui

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/oauth"
	"github.com/simplyblock/postbrain/internal/social"
)

type oauthStateStore interface {
	Issue(ctx context.Context, kind string, payload map[string]any, ttl time.Duration) (string, error)
	Peek(ctx context.Context, rawState string) (*oauth.StateRecord, error)
	Consume(ctx context.Context, rawState string) (*oauth.StateRecord, error)
}

type oauthCodeStore interface {
	Issue(ctx context.Context, req oauth.IssueCodeRequest) (string, error)
}

type oauthIssuer interface {
	Issue(ctx context.Context, principalID uuid.UUID, scopes []string, ttl time.Duration) (string, error)
}

type oauthClientLookup interface {
	LookupByClientID(ctx context.Context, clientID string) (*oauth.OAuthClient, error)
}

type socialIdentityStore interface {
	FindOrCreateWithPolicy(ctx context.Context, provider string, info *social.UserInfo, policy social.IdentityPolicy) (*db.Principal, error)
}
