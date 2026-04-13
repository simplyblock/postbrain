//go:build integration

package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/db/compat"
	"github.com/simplyblock/postbrain/internal/testhelper"
)

func TestTokenStore_Lookup_NilToken(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ts := NewTokenStore(pool)
	ctx := context.Background()

	// Hash of a token that was never inserted.
	hash := HashToken("pb_doesnotexist")
	got, err := ts.Lookup(ctx, hash)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown token, got %+v", got)
	}
}

func TestTokenStore_Lookup_ValidToken(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ts := NewTokenStore(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "auth-lookup-"+uuid.New().String())
	raw, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = compat.CreateToken(ctx, pool, principal.ID, hash, "test-token", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	got, err := ts.Lookup(ctx, HashToken(raw))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil token for valid hash")
	}
	if got.PrincipalID != principal.ID {
		t.Errorf("PrincipalID = %v; want %v", got.PrincipalID, principal.ID)
	}
}

func TestTokenStore_Lookup_RevokedToken(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ts := NewTokenStore(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "auth-revoked-"+uuid.New().String())
	_, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	tok, err := compat.CreateToken(ctx, pool, principal.ID, hash, "revoke-test", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if err := compat.RevokeToken(ctx, pool, tok.ID); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	got, err := ts.Lookup(ctx, hash)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for revoked token, got %+v", got)
	}
}

func TestTokenStore_Lookup_ExpiredToken(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ts := NewTokenStore(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "auth-expired-"+uuid.New().String())
	_, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	_, err = compat.CreateToken(ctx, pool, principal.ID, hash, "expired-test", nil, nil, &past)
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}

	got, err := ts.Lookup(ctx, hash)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for expired token, got %+v", got)
	}
}

func TestTokenStore_Create_And_Revoke(t *testing.T) {
	pool := testhelper.NewTestPool(t)
	ts := NewTokenStore(pool)
	ctx := context.Background()

	principal := testhelper.CreateTestPrincipal(t, pool, "user", "auth-create-"+uuid.New().String())
	_, hash, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}

	tok, err := ts.Create(ctx, principal.ID, hash, "my-token", nil, nil, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if tok.ID == (uuid.UUID{}) {
		t.Error("expected non-zero token ID")
	}

	// Token should be retrievable.
	got, err := ts.Lookup(ctx, hash)
	if err != nil {
		t.Fatalf("Lookup after Create: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil token after Create")
	}

	// Revoke it.
	if err := ts.Revoke(ctx, tok.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	// Now it should be gone.
	got, err = ts.Lookup(ctx, hash)
	if err != nil {
		t.Fatalf("Lookup after Revoke: %v", err)
	}
	if got != nil {
		t.Error("expected nil after Revoke")
	}
}
