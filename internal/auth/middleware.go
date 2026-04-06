package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
)

// contextKey is the type for all context keys injected by the auth middleware.
type contextKey string

const (
	ContextKeyToken         contextKey = "pb_token"
	ContextKeyPrincipalID   contextKey = "pb_principal_id"
	ContextKeyPermissions   contextKey = "pb_permissions"
	ContextKeyTokenResolver contextKey = "pb_token_resolver"
)

// tokenLookup is the interface used by the middleware, allowing test doubles.
type tokenLookup interface {
	Lookup(ctx context.Context, hash string) (*db.Token, error)
	UpdateLastUsed(pool *pgxpool.Pool, tokenID uuid.UUID)
}

// unauthorized writes a 401 JSON response.
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

// BearerTokenMiddleware returns an http.Handler middleware that authenticates
// requests via a Bearer token in the Authorization header.
//
// On success it injects the token, principal ID, parsed authz.PermissionSet,
// and an authz.TokenResolver into the request context. On failure it responds
// with 401 JSON {"error":"unauthorized"}.
// UpdateLastUsed is called asynchronously (fire-and-forget) using pool.
// If pool is nil, UpdateLastUsed is still called but is a no-op (useful in tests).
func BearerTokenMiddleware(store *TokenStore, pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return bearerTokenMiddlewareWithStore(store, pool)
}

// bearerTokenMiddlewareWithStore is the testable inner implementation that
// accepts the tokenLookup interface instead of *TokenStore directly.
func bearerTokenMiddlewareWithStore(store tokenLookup, pool *pgxpool.Pool) func(http.Handler) http.Handler {
	var tokenResolver *authz.TokenResolver
	if pool != nil {
		tokenResolver = authz.NewTokenResolver(authz.NewDBResolver(pool))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				unauthorized(w)
				return
			}

			const prefix = "Bearer "
			if !strings.HasPrefix(authHeader, prefix) {
				unauthorized(w)
				return
			}

			rawToken := strings.TrimPrefix(authHeader, prefix)
			if rawToken == "" {
				unauthorized(w)
				return
			}

			hash := HashToken(rawToken)
			token, err := store.Lookup(r.Context(), hash)
			if err != nil || token == nil {
				unauthorized(w)
				return
			}

			// Fire-and-forget last-used update.
			store.UpdateLastUsed(pool, token.ID)

			// Parse token permissions via authz — expands shorthands and validates.
			// On parse error, fall back to an empty set (token is still authenticated
			// but will fail any permission check).
			perms, _ := authz.ParseTokenPermissions(token.Permissions)

			// Inject token metadata into context.
			ctx := r.Context()
			ctx = context.WithValue(ctx, ContextKeyToken, token)
			ctx = context.WithValue(ctx, ContextKeyPrincipalID, token.PrincipalID)
			ctx = context.WithValue(ctx, ContextKeyPermissions, perms)
			if tokenResolver != nil {
				ctx = context.WithValue(ctx, ContextKeyTokenResolver, tokenResolver)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
