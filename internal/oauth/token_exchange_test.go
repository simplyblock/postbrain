package oauth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db"
)

type fakeTokenStore struct {
	lastPrincipalID uuid.UUID
	lastHash        string
	lastName        string
	lastPermissions []string
	lastExpiresAt   *time.Time
}

func (f *fakeTokenStore) Create(_ context.Context, principalID uuid.UUID, hash, name string, _ []uuid.UUID, permissions []string, expiresAt *time.Time) (*db.Token, error) {
	f.lastPrincipalID = principalID
	f.lastHash = hash
	f.lastName = name
	f.lastPermissions = permissions
	f.lastExpiresAt = expiresAt
	return &db.Token{
		ID:          uuid.New(),
		PrincipalID: principalID,
		TokenHash:   hash,
		Name:        name,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
	}, nil
}

func TestIssue_TranslatesScopes_ToPermissions(t *testing.T) {
	fake := &fakeTokenStore{}
	issuer := &Issuer{
		tokenStore: fake,
		generateToken: func() (string, string, error) {
			return "pb_test_raw", "hash", nil
		},
		now: func() time.Time {
			return time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
		},
	}

	_, err := issuer.Issue(context.Background(), uuid.New(), []string{ScopeMemoriesRead, ScopeKnowledgeWrite}, 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(fake.lastPermissions) != 2 || fake.lastPermissions[0] != ScopeMemoriesRead || fake.lastPermissions[1] != ScopeKnowledgeWrite {
		t.Fatalf("permissions = %v, want scopes mapped 1:1", fake.lastPermissions)
	}
}

func TestIssue_CreatesTokenWithCorrectPrincipal(t *testing.T) {
	fake := &fakeTokenStore{}
	issuer := &Issuer{
		tokenStore: fake,
		generateToken: func() (string, string, error) {
			return "pb_test_raw", "hash", nil
		},
		now: time.Now,
	}
	principalID := uuid.New()

	_, err := issuer.Issue(context.Background(), principalID, []string{ScopeMemoriesRead}, 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if fake.lastPrincipalID != principalID {
		t.Fatalf("principal_id = %s, want %s", fake.lastPrincipalID, principalID)
	}
}

func TestIssue_ZeroTTL_CreatesNonExpiringToken(t *testing.T) {
	fake := &fakeTokenStore{}
	issuer := &Issuer{
		tokenStore: fake,
		generateToken: func() (string, string, error) {
			return "pb_test_raw", "hash", nil
		},
		now: time.Now,
	}

	_, err := issuer.Issue(context.Background(), uuid.New(), []string{ScopeMemoriesRead}, 0)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if fake.lastExpiresAt != nil {
		t.Fatalf("expires_at = %v, want nil for zero ttl", fake.lastExpiresAt)
	}
}

func TestIssue_NonZeroTTL_SetsExpiresAt(t *testing.T) {
	fake := &fakeTokenStore{}
	baseNow := time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC)
	issuer := &Issuer{
		tokenStore: fake,
		generateToken: func() (string, string, error) {
			return "pb_test_raw", "hash", nil
		},
		now: func() time.Time { return baseNow },
	}

	_, err := issuer.Issue(context.Background(), uuid.New(), []string{ScopeMemoriesRead}, 30*time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if fake.lastExpiresAt == nil {
		t.Fatal("expires_at is nil, want non-nil for non-zero ttl")
	}
	want := baseNow.Add(30 * time.Minute)
	if !fake.lastExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", *fake.lastExpiresAt, want)
	}
}
