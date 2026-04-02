package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simplyblock/postbrain/internal/db"
)

var ErrInvalidRedirectURI = errors.New("oauth: invalid redirect_uri")

type clientQueries interface {
	RegisterClient(ctx context.Context, arg db.RegisterClientParams) (*db.OauthClient, error)
	LookupClient(ctx context.Context, clientID string) (*db.OauthClient, error)
	RevokeClient(ctx context.Context, id uuid.UUID) error
}

// RegisterRequest contains OAuth client registration details.
type RegisterRequest struct {
	Name         string
	RedirectURIs []string
	GrantTypes   []string
	Scopes       []string
	IsPublic     bool
	Meta         []byte
}

// OAuthClient is the domain model for oauth_clients rows.
type OAuthClient struct {
	ID               uuid.UUID
	ClientID         string
	ClientSecretHash *string
	Name             string
	RedirectURIs     []string
	GrantTypes       []string
	Scopes           []string
	IsPublic         bool
	Meta             []byte
}

// ClientStore persists and validates OAuth clients.
type ClientStore struct {
	q clientQueries
}

// NewClientStore constructs a ClientStore backed by sqlc queries.
func NewClientStore(pool *pgxpool.Pool) *ClientStore {
	return &ClientStore{q: db.New(pool)}
}

// Register creates a new OAuth client. For confidential clients, a secret is generated
// and only its hash is persisted.
func (s *ClientStore) Register(ctx context.Context, req RegisterRequest) (*OAuthClient, string, error) {
	clientID, err := generateOpaque("pb_client_")
	if err != nil {
		return nil, "", err
	}

	var rawSecret string
	var secretHash *string
	if !req.IsPublic {
		rawSecret, err = generateOpaque("pb_secret_")
		if err != nil {
			return nil, "", err
		}
		hash := hashSHA256Hex(rawSecret)
		secretHash = &hash
	}

	row, err := s.q.RegisterClient(ctx, db.RegisterClientParams{
		ClientID:         clientID,
		ClientSecretHash: secretHash,
		Name:             req.Name,
		RedirectUris:     req.RedirectURIs,
		GrantTypes:       req.GrantTypes,
		Scopes:           req.Scopes,
		IsPublic:         req.IsPublic,
		Meta:             req.Meta,
	})
	if err != nil {
		return nil, "", err
	}
	return toOAuthClient(row), rawSecret, nil
}

// LookupByClientID finds a non-revoked client by client_id.
// Returns nil, nil if the client is not found or revoked.
func (s *ClientStore) LookupByClientID(ctx context.Context, clientID string) (*OAuthClient, error) {
	row, err := s.q.LookupClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return toOAuthClient(row), nil
}

// ValidateRedirectURI enforces an exact redirect URI match against the registered list.
func (s *ClientStore) ValidateRedirectURI(client *OAuthClient, redirectURI string) error {
	for _, registered := range client.RedirectURIs {
		if registered == redirectURI {
			return nil
		}
	}
	return ErrInvalidRedirectURI
}

// Revoke marks a client as revoked.
func (s *ClientStore) Revoke(ctx context.Context, id uuid.UUID) error {
	return s.q.RevokeClient(ctx, id)
}

func toOAuthClient(row *db.OauthClient) *OAuthClient {
	if row == nil {
		return nil
	}
	return &OAuthClient{
		ID:               row.ID,
		ClientID:         row.ClientID,
		ClientSecretHash: row.ClientSecretHash,
		Name:             row.Name,
		RedirectURIs:     row.RedirectUris,
		GrantTypes:       row.GrantTypes,
		Scopes:           row.Scopes,
		IsPublic:         row.IsPublic,
		Meta:             row.Meta,
	}
}

func generateOpaque(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate opaque value: %w", err)
	}
	return prefix + hex.EncodeToString(b), nil
}
