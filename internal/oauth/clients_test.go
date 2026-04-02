package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/simplyblock/postbrain/internal/db"
)

type fakeClientQueries struct {
	clientsByID map[string]*db.OauthClient
	revokedIDs  map[uuid.UUID]bool
}

func (f *fakeClientQueries) RegisterClient(_ context.Context, arg db.RegisterClientParams) (*db.OauthClient, error) {
	row := &db.OauthClient{
		ID:               uuid.New(),
		ClientID:         arg.ClientID,
		ClientSecretHash: arg.ClientSecretHash,
		Name:             arg.Name,
		RedirectUris:     arg.RedirectUris,
		GrantTypes:       arg.GrantTypes,
		Scopes:           arg.Scopes,
		IsPublic:         arg.IsPublic,
		Meta:             arg.Meta,
		CreatedAt:        time.Now(),
	}
	f.clientsByID[row.ClientID] = row
	return row, nil
}

func (f *fakeClientQueries) LookupClient(_ context.Context, clientID string) (*db.OauthClient, error) {
	row, ok := f.clientsByID[clientID]
	if !ok {
		return nil, pgx.ErrNoRows
	}
	if f.revokedIDs[row.ID] {
		return nil, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeClientQueries) RevokeClient(_ context.Context, id uuid.UUID) error {
	f.revokedIDs[id] = true
	return nil
}

func newTestClientStore() *ClientStore {
	return &ClientStore{
		q: &fakeClientQueries{
			clientsByID: map[string]*db.OauthClient{},
			revokedIDs:  map[uuid.UUID]bool{},
		},
	}
}

func TestClientStore_Register_PublicClient_NoSecret(t *testing.T) {
	store := newTestClientStore()
	ctx := context.Background()

	client, rawSecret, err := store.Register(ctx, RegisterRequest{
		Name:         "Public MCP Client",
		RedirectURIs: []string{"http://localhost/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"memories:read"},
		IsPublic:     true,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if rawSecret != "" {
		t.Fatalf("raw secret for public client = %q, want empty", rawSecret)
	}
	if client.ClientSecretHash != nil {
		t.Fatalf("client secret hash for public client = %v, want nil", *client.ClientSecretHash)
	}
}

func TestClientStore_Register_ConfidentialClient_HashesSecret(t *testing.T) {
	store := newTestClientStore()
	ctx := context.Background()

	client, rawSecret, err := store.Register(ctx, RegisterRequest{
		Name:         "Confidential MCP Client",
		RedirectURIs: []string{"http://localhost/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"memories:read"},
		IsPublic:     false,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if rawSecret == "" {
		t.Fatal("raw secret is empty for confidential client")
	}
	if client.ClientSecretHash == nil {
		t.Fatal("client secret hash is nil for confidential client")
	}
	if *client.ClientSecretHash == rawSecret {
		t.Fatal("client secret appears to be stored in plaintext")
	}
	sum := sha256.Sum256([]byte(rawSecret))
	want := hex.EncodeToString(sum[:])
	if *client.ClientSecretHash != want {
		t.Fatalf("client secret hash = %q, want %q", *client.ClientSecretHash, want)
	}
}

func TestClientStore_LookupByClientID_Found(t *testing.T) {
	store := newTestClientStore()
	ctx := context.Background()

	registered, _, err := store.Register(ctx, RegisterRequest{
		Name:         "Lookup Client",
		RedirectURIs: []string{"http://localhost/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"memories:read"},
		IsPublic:     true,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	found, err := store.LookupByClientID(ctx, registered.ClientID)
	if err != nil {
		t.Fatalf("LookupByClientID: %v", err)
	}
	if found == nil || found.ClientID != registered.ClientID {
		t.Fatalf("LookupByClientID returned %+v, want client_id=%q", found, registered.ClientID)
	}
}

func TestClientStore_LookupByClientID_Revoked_ReturnsNil(t *testing.T) {
	store := newTestClientStore()
	ctx := context.Background()

	registered, _, err := store.Register(ctx, RegisterRequest{
		Name:         "Revoked Client",
		RedirectURIs: []string{"http://localhost/callback"},
		GrantTypes:   []string{"authorization_code"},
		Scopes:       []string{"memories:read"},
		IsPublic:     true,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := store.Revoke(ctx, registered.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	found, err := store.LookupByClientID(ctx, registered.ClientID)
	if err != nil {
		t.Fatalf("LookupByClientID revoked: %v", err)
	}
	if found != nil {
		t.Fatalf("LookupByClientID revoked returned %+v, want nil", found)
	}
}

func TestClientStore_ValidateRedirectURI_ExactMatch_OK(t *testing.T) {
	store := newTestClientStore()
	client := &OAuthClient{
		RedirectURIs: []string{
			"http://localhost/callback",
			"https://app.example.com/oauth/callback",
		},
	}
	if err := store.ValidateRedirectURI(client, "https://app.example.com/oauth/callback"); err != nil {
		t.Fatalf("ValidateRedirectURI: %v", err)
	}
}

func TestClientStore_ValidateRedirectURI_NoMatch_ReturnsError(t *testing.T) {
	store := newTestClientStore()
	client := &OAuthClient{
		RedirectURIs: []string{"https://app.example.com/oauth/callback"},
	}
	if err := store.ValidateRedirectURI(client, "https://app.example.com/oauth/callback?next=/ui"); !errors.Is(err, ErrInvalidRedirectURI) {
		t.Fatalf("ValidateRedirectURI error = %v, want ErrInvalidRedirectURI", err)
	}
}
