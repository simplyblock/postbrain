//go:build integration

package social

import (
	"context"
	"errors"
	"testing"

	"github.com/simplyblock/postbrain/internal/db"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestIdentityStore_FindOrCreate_NewIdentity_CreatesPrincipalAndIdentity(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	store := NewIdentityStore(pool)
	ctx := context.Background()

	info := &UserInfo{
		ProviderID:  "gh-1001",
		Email:       "new-social-user@example.com",
		DisplayName: "New Social User",
		AvatarURL:   "https://cdn.example/new.png",
		RawProfile:  []byte(`{"id":"gh-1001"}`),
	}

	principal, err := store.FindOrCreate(ctx, "github", info)
	if err != nil {
		t.Fatalf("FindOrCreate: %v", err)
	}
	if principal == nil {
		t.Fatal("principal is nil")
	}
	if principal.Kind != "user" {
		t.Fatalf("principal kind = %q, want user", principal.Kind)
	}

	var linkedPrincipalID string
	err = pool.QueryRow(ctx, `
		SELECT principal_id::text FROM social_identities
		WHERE provider = 'github' AND provider_id = 'gh-1001'
	`).Scan(&linkedPrincipalID)
	if err != nil {
		t.Fatalf("query social identity: %v", err)
	}
	if principal.ID.String() != linkedPrincipalID {
		t.Fatalf("linked principal_id = %s, want %s", linkedPrincipalID, principal.ID)
	}
}

func TestIdentityStore_FindOrCreate_ExistingIdentity_UpdatesProfile_ReturnsSamePrincipal(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	store := NewIdentityStore(pool)
	ctx := context.Background()

	first, err := store.FindOrCreate(ctx, "github", &UserInfo{
		ProviderID:  "gh-1002",
		Email:       "first@example.com",
		DisplayName: "First Name",
		AvatarURL:   "https://cdn.example/first.png",
		RawProfile:  []byte(`{"id":"gh-1002","v":1}`),
	})
	if err != nil {
		t.Fatalf("FindOrCreate first: %v", err)
	}

	second, err := store.FindOrCreate(ctx, "github", &UserInfo{
		ProviderID:  "gh-1002",
		Email:       "updated@example.com",
		DisplayName: "Updated Name",
		AvatarURL:   "https://cdn.example/updated.png",
		RawProfile:  []byte(`{"id":"gh-1002","v":2}`),
	})
	if err != nil {
		t.Fatalf("FindOrCreate second: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("principal IDs differ: first=%s second=%s", first.ID, second.ID)
	}

	var email, displayName, avatar string
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(email::text,''), COALESCE(display_name,''), COALESCE(avatar_url,'')
		FROM social_identities
		WHERE provider = 'github' AND provider_id = 'gh-1002'
	`).Scan(&email, &displayName, &avatar)
	if err != nil {
		t.Fatalf("query social identity after update: %v", err)
	}
	if email != "updated@example.com" || displayName != "Updated Name" || avatar != "https://cdn.example/updated.png" {
		t.Fatalf("updated social identity mismatch: email=%q display_name=%q avatar=%q", email, displayName, avatar)
	}
}

func TestIdentityStore_FindOrCreate_SlugCollision_AppendsProviderID(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	store := NewIdentityStore(pool)
	ctx := context.Background()

	q := db.New(pool)
	_, err := q.CreatePrincipal(ctx, db.CreatePrincipalParams{
		Kind:        "user",
		Slug:        "collision@example.com",
		DisplayName: "Existing User",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("seed existing principal: %v", err)
	}

	principal, err := store.FindOrCreate(ctx, "github", &UserInfo{
		ProviderID:  "gh-collision-1",
		Email:       "collision@example.com",
		DisplayName: "Colliding User",
		AvatarURL:   "https://cdn.example/collision.png",
		RawProfile:  []byte(`{"id":"gh-collision-1"}`),
	})
	if err != nil {
		t.Fatalf("FindOrCreate collision: %v", err)
	}
	if principal.Slug != "collision@example.com-gh-collision-1" {
		t.Fatalf("principal slug = %q, want collision@example.com-gh-collision-1", principal.Slug)
	}

	var linkedPrincipalID string
	err = pool.QueryRow(ctx, `
		SELECT principal_id::text FROM social_identities
		WHERE provider = 'github' AND provider_id = 'gh-collision-1'
	`).Scan(&linkedPrincipalID)
	if err != nil {
		t.Fatalf("query social identity: %v", err)
	}
	if principal.ID.String() != linkedPrincipalID {
		t.Fatalf("linked principal_id = %s, want %s", linkedPrincipalID, principal.ID)
	}
}

func TestIdentityStore_FindOrCreateWithPolicy_AutoCreateDisabled_PreprovisionedEmailLinksPrincipal(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	store := NewIdentityStore(pool)
	ctx := context.Background()

	q := db.New(pool)
	existing, err := q.CreatePrincipal(ctx, db.CreatePrincipalParams{
		Kind:        "user",
		Slug:        "provisioned@example.com",
		DisplayName: "Provisioned User",
		Meta:        []byte(`{}`),
	})
	if err != nil {
		t.Fatalf("seed existing principal: %v", err)
	}

	principal, err := store.FindOrCreateWithPolicy(ctx, "google", &UserInfo{
		ProviderID:    "google-123",
		Email:         "provisioned@example.com",
		EmailVerified: true,
		DisplayName:   "Provisioned User",
		RawProfile:    []byte(`{"sub":"google-123"}`),
	}, IdentityPolicy{AutoCreateUsers: false})
	if err != nil {
		t.Fatalf("FindOrCreateWithPolicy: %v", err)
	}
	if principal.ID != existing.ID {
		t.Fatalf("principal ID = %s, want %s", principal.ID, existing.ID)
	}
}

func TestIdentityStore_FindOrCreateWithPolicy_AutoCreateDisabled_UnprovisionedEmailReturnsError(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	store := NewIdentityStore(pool)
	ctx := context.Background()

	_, err := store.FindOrCreateWithPolicy(ctx, "google", &UserInfo{
		ProviderID:    "google-456",
		Email:         "missing@example.com",
		EmailVerified: true,
		DisplayName:   "Missing User",
		RawProfile:    []byte(`{"sub":"google-456"}`),
	}, IdentityPolicy{AutoCreateUsers: false})
	if !errors.Is(err, ErrPrincipalNotProvisioned) {
		t.Fatalf("expected ErrPrincipalNotProvisioned, got %v", err)
	}
}
