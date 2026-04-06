package authz_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
)

type fakeResolver struct{}

func (fakeResolver) EffectivePermissions(context.Context, uuid.UUID, uuid.UUID) (authz.PermissionSet, error) {
	return authz.EmptyPermissionSet(), nil
}

func (fakeResolver) HasPermission(context.Context, uuid.UUID, uuid.UUID, authz.Permission) (bool, error) {
	return false, nil
}

// TestTokenResolver_ScopeRestriction_DoesNotPanicWithNonDBResolver asserts that
// TokenResolver behaves safely when constructed with a Resolver implementation
// other than *DBResolver.
func TestTokenResolver_ScopeRestriction_DoesNotPanicWithNonDBResolver(t *testing.T) {
	tr := authz.NewTokenResolver(fakeResolver{})
	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: uuid.New(),
		Permissions: []string{"read"},
		ScopeIds:    []uuid.UUID{uuid.New()},
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("EffectiveTokenPermissions panicked: %v", r)
		}
	}()

	_, _ = tr.EffectiveTokenPermissions(context.Background(), tok, uuid.New())
}
