package sharing_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/sharing"
)

// callCreate calls s.Create and returns (nil, ErrInvalidGrant) for validation
// failures or any non-nil error for other failures (including nil-pool panics
// recovered here so tests do not abort).
func callCreate(s *sharing.Store, g *sharing.Grant) (result *sharing.Grant, err error) {
	defer func() {
		if r := recover(); r != nil {
			// Nil pool caused a panic — validation passed (no ErrInvalidGrant returned).
			result = nil
			err = nil
		}
	}()
	return s.Create(nil, g) //nolint:staticcheck
}

func TestCreate_BothSet_ErrInvalidGrant(t *testing.T) {
	s := sharing.NewStore(nil)
	memID := uuid.New()
	artID := uuid.New()
	g := &sharing.Grant{
		MemoryID:       &memID,
		ArtifactID:     &artID,
		GranteeScopeID: uuid.New(),
		GrantedBy:      uuid.New(),
	}
	_, err := callCreate(s, g)
	if err != sharing.ErrInvalidGrant {
		t.Fatalf("expected ErrInvalidGrant, got %v", err)
	}
}

func TestCreate_NeitherSet_ErrInvalidGrant(t *testing.T) {
	s := sharing.NewStore(nil)
	g := &sharing.Grant{
		GranteeScopeID: uuid.New(),
		GrantedBy:      uuid.New(),
	}
	_, err := callCreate(s, g)
	if err != sharing.ErrInvalidGrant {
		t.Fatalf("expected ErrInvalidGrant, got %v", err)
	}
}

func TestCreate_OnlyMemoryID_ValidationPasses(t *testing.T) {
	// Validation should pass; nil pool causes a panic that we recover from.
	s := sharing.NewStore(nil)
	memID := uuid.New()
	g := &sharing.Grant{
		MemoryID:       &memID,
		GranteeScopeID: uuid.New(),
		GrantedBy:      uuid.New(),
	}
	_, err := callCreate(s, g)
	if err == sharing.ErrInvalidGrant {
		t.Fatalf("expected validation to pass, got ErrInvalidGrant")
	}
}

func TestCreate_OnlyArtifactID_ValidationPasses(t *testing.T) {
	s := sharing.NewStore(nil)
	artID := uuid.New()
	g := &sharing.Grant{
		ArtifactID:     &artID,
		GranteeScopeID: uuid.New(),
		GrantedBy:      uuid.New(),
		ExpiresAt:      func() *time.Time { t := time.Now().Add(time.Hour); return &t }(),
	}
	_, err := callCreate(s, g)
	if err == sharing.ErrInvalidGrant {
		t.Fatalf("expected validation to pass, got ErrInvalidGrant")
	}
}
