package scopeauth

import (
	"errors"
	"testing"

	"github.com/google/uuid"

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
