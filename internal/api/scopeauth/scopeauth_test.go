package scopeauth

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/simplyblock/postbrain/internal/auth"
	"github.com/simplyblock/postbrain/internal/db"
)

func TestAuthorizeRequestedScope(t *testing.T) {
	t.Parallel()

	requested := uuid.New()
	other := uuid.New()

	tests := []struct {
		name             string
		token            *db.Token
		effectiveScopeID []uuid.UUID
		wantErr          error
	}{
		{
			name: "token unrestricted and requested in effective scopes",
			token: &db.Token{
				ScopeIds: nil,
			},
			effectiveScopeID: []uuid.UUID{requested},
		},
		{
			name: "token restricted includes requested and effective includes requested",
			token: &db.Token{
				ScopeIds: []uuid.UUID{requested},
			},
			effectiveScopeID: []uuid.UUID{requested},
		},
		{
			name: "token restricted excludes requested",
			token: &db.Token{
				ScopeIds: []uuid.UUID{other},
			},
			effectiveScopeID: []uuid.UUID{requested},
			wantErr:          ErrTokenScopeDenied,
		},
		{
			name: "effective scopes exclude requested",
			token: &db.Token{
				ScopeIds: nil,
			},
			effectiveScopeID: []uuid.UUID{other},
			wantErr:          ErrPrincipalScopeDenied,
		},
		{
			name: "empty effective scopes denies",
			token: &db.Token{
				ScopeIds: nil,
			},
			effectiveScopeID: nil,
			wantErr:          ErrPrincipalScopeDenied,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := AuthorizeRequestedScope(tc.token, requested, tc.effectiveScopeID)
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

type fakeEffectiveScopeResolver struct {
	ids   []uuid.UUID
	err   error
	calls int
}

func (f *fakeEffectiveScopeResolver) EffectiveScopeIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	f.calls++
	return f.ids, f.err
}

func TestAuthorizeContextScope(t *testing.T) {
	t.Parallel()

	principalID := uuid.New()
	requested := uuid.New()
	other := uuid.New()
	baseCtx := context.Background()

	withToken := func(ctx context.Context, token *db.Token) context.Context {
		return context.WithValue(ctx, auth.ContextKeyToken, token)
	}
	withPrincipal := func(ctx context.Context, pid uuid.UUID) context.Context {
		return context.WithValue(ctx, auth.ContextKeyPrincipalID, pid)
	}

	tests := []struct {
		name     string
		ctx      context.Context
		resolver EffectiveScopeResolver
		wantErr  error
	}{
		{
			name: "authorized",
			ctx: withPrincipal(
				withToken(baseCtx, &db.Token{ScopeIds: []uuid.UUID{requested}}),
				principalID,
			),
			resolver: &fakeEffectiveScopeResolver{ids: []uuid.UUID{requested}},
		},
		{
			name: "missing token",
			ctx: withPrincipal(
				baseCtx,
				principalID,
			),
			resolver: &fakeEffectiveScopeResolver{ids: []uuid.UUID{requested}},
			wantErr:  ErrMissingToken,
		},
		{
			name: "missing principal",
			ctx: withToken(
				baseCtx,
				&db.Token{ScopeIds: []uuid.UUID{requested}},
			),
			resolver: &fakeEffectiveScopeResolver{ids: []uuid.UUID{requested}},
			wantErr:  ErrMissingPrincipal,
		},
		{
			name: "token denies scope",
			ctx: withPrincipal(
				withToken(baseCtx, &db.Token{ScopeIds: []uuid.UUID{other}}),
				principalID,
			),
			resolver: &fakeEffectiveScopeResolver{ids: []uuid.UUID{requested}},
			wantErr:  ErrTokenScopeDenied,
		},
		{
			name: "principal effective scopes deny",
			ctx: withPrincipal(
				withToken(baseCtx, &db.Token{ScopeIds: []uuid.UUID{requested}}),
				principalID,
			),
			resolver: &fakeEffectiveScopeResolver{ids: []uuid.UUID{other}},
			wantErr:  ErrPrincipalScopeDenied,
		},
		{
			name: "resolver unavailable",
			ctx: withPrincipal(
				withToken(baseCtx, &db.Token{ScopeIds: []uuid.UUID{requested}}),
				principalID,
			),
			resolver: nil,
			wantErr:  ErrScopeResolverUnavailable,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := AuthorizeContextScope(tc.ctx, tc.resolver, requested)
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

func TestAuthorizeContextScope_UsesCachedEffectiveScopes(t *testing.T) {
	t.Parallel()

	principalID := uuid.New()
	requested := uuid.New()

	resolver := &fakeEffectiveScopeResolver{ids: []uuid.UUID{}}
	ctx := context.Background()
	ctx = context.WithValue(ctx, auth.ContextKeyToken, &db.Token{ScopeIds: []uuid.UUID{requested}})
	ctx = context.WithValue(ctx, auth.ContextKeyPrincipalID, principalID)
	ctx = WithEffectiveScopeIDs(ctx, []uuid.UUID{requested})

	if err := AuthorizeContextScope(ctx, resolver, requested); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0 when cache is present", resolver.calls)
	}
}
