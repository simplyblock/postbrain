package auth

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/simplyblock/postbrain/internal/db"
)

func TestHashToken_Deterministic(t *testing.T) {
	input := "pb_abc123test"
	h1 := HashToken(input)
	h2 := HashToken(input)
	if h1 != h2 {
		t.Errorf("HashToken is not deterministic: %q != %q", h1, h2)
	}
}

func TestHashToken_EmptyDoesNotPanic(t *testing.T) {
	// Should not panic; just produce a valid hash of empty input.
	got := HashToken("")
	if got == "" {
		t.Error("HashToken(\"\") returned empty string")
	}
}

func TestGenerateToken_HasPbPrefix(t *testing.T) {
	raw, _, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if !strings.HasPrefix(raw, "pb_") {
		t.Errorf("raw token %q does not start with 'pb_'", raw)
	}
}

func TestGenerateToken_HashMatchesRaw(t *testing.T) {
	raw, hash, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	expected := HashToken(raw)
	if hash != expected {
		t.Errorf("hash mismatch: got %q want %q", hash, expected)
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	raw1, _, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	raw2, _, err := GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if raw1 == raw2 {
		t.Error("two calls to GenerateToken returned the same token")
	}
}

func TestEnforceScopeAccess_NilScopeIds(t *testing.T) {
	tok := &db.Token{ScopeIds: nil}
	if err := EnforceScopeAccess(tok, uuid.New()); err != nil {
		t.Errorf("nil ScopeIds should allow any scope, got: %v", err)
	}
}

func TestEnforceScopeAccess_ScopeIDPresent(t *testing.T) {
	id := uuid.New()
	tok := &db.Token{ScopeIds: []uuid.UUID{uuid.New(), id, uuid.New()}}
	if err := EnforceScopeAccess(tok, id); err != nil {
		t.Errorf("scope present in list should be allowed, got: %v", err)
	}
}

func TestEnforceScopeAccess_ScopeIDAbsent(t *testing.T) {
	tok := &db.Token{ScopeIds: []uuid.UUID{uuid.New(), uuid.New()}}
	if err := EnforceScopeAccess(tok, uuid.New()); err == nil {
		t.Error("scope absent from non-nil list should be rejected")
	}
}

func TestTokenStore_UpdateLastUsed_NilPool(t *testing.T) {
	ts := NewTokenStore(nil)
	// Must not panic when pool is nil.
	ts.UpdateLastUsed(nil, uuid.New())
}
