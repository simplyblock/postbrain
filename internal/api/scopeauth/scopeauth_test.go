package scopeauth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
)

// fakeResolver is a test implementation of authz.Resolver.
type fakeResolver struct {
	perms authz.PermissionSet
	err   error
}

func (f *fakeResolver) EffectivePermissions(_ context.Context, _, _ uuid.UUID) (authz.PermissionSet, error) {
	return f.perms, f.err
}

func (f *fakeResolver) HasPermission(_ context.Context, _, _ uuid.UUID, perm authz.Permission) (bool, error) {
	return f.perms.Contains(perm), f.err
}

// makeToken returns a minimal *db.Token with the given permissions set.
func makeToken(perms ...string) *db.Token {
	return &db.Token{
		ID:          uuid.New(),
		Permissions: perms,
	}
}

func TestAuthorizeContextScope(t *testing.T) {
	t.Parallel()

	const testPerm authz.Permission = "memories:write"
	requested := uuid.New()

	withToken := func(ctx context.Context, tok *db.Token) context.Context {
		return context.WithValue(ctx, auth.ContextKeyToken, tok)
	}
	withResolver := func(ctx context.Context, tr *authz.TokenResolver) context.Context {
		return context.WithValue(ctx, auth.ContextKeyTokenResolver, tr)
	}
	withPrincipal := func(ctx context.Context) context.Context {
		return context.WithValue(ctx, auth.ContextKeyPrincipalID, uuid.New())
	}

	tests := []struct {
		name    string
		ctx     context.Context
		wantErr error
	}{
		{
			name: "authorized: principal has permission, token unrestricted",
			ctx: func() context.Context {
				perms, _ := authz.NewPermissionSet([]string{"memories:write"})
				tr := authz.NewTokenResolver(&fakeResolver{perms: perms})
				ctx := withToken(context.Background(), makeToken("memories:write"))
				ctx = withPrincipal(ctx)
				return withResolver(ctx, tr)
			}(),
		},
		{
			name: "denied: principal lacks permission on scope",
			ctx: func() context.Context {
				// Principal only has memories:read; token also only read.
				perms, _ := authz.NewPermissionSet([]string{"memories:read"})
				tr := authz.NewTokenResolver(&fakeResolver{perms: perms})
				ctx := withToken(context.Background(), makeToken("memories:read"))
				ctx = withPrincipal(ctx)
				return withResolver(ctx, tr)
			}(),
			wantErr: ErrTokenScopeDenied,
		},
		{
			name:    "missing token returns ErrMissingToken",
			ctx:     withResolver(context.Background(), authz.NewTokenResolver(&fakeResolver{})),
			wantErr: ErrMissingToken,
		},
		{
			name:    "missing resolver returns ErrScopeResolverUnavailable",
			ctx:     withToken(context.Background(), makeToken("memories:write")),
			wantErr: ErrScopeResolverUnavailable,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := AuthorizeContextScope(tc.ctx, requested, testPerm)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %v, got nil", tc.wantErr)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}
}
