package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/simplyblock/postbrain/internal/authz"
	"github.com/simplyblock/postbrain/internal/db"
)

// fakeTokenLookup implements tokenLookup for testing.
type fakeTokenLookup struct {
	token *db.Token
	err   error
}

func (f *fakeTokenLookup) Lookup(_ context.Context, _ string) (*db.Token, error) {
	return f.token, f.err
}

func (f *fakeTokenLookup) UpdateLastUsed(_ *pgxpool.Pool, _ uuid.UUID) {
	// no-op in tests
}

// okHandler is a simple downstream handler that responds 200.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// Verify context values are set.
	pid := r.Context().Value(ContextKeyPrincipalID)
	if pid == nil {
		http.Error(w, "missing principal_id in context", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
})

func TestMiddleware_MissingAuthHeader(t *testing.T) {
	store := &fakeTokenLookup{}
	mw := bearerTokenMiddlewareWithStore(store, nil)
	handler := mw(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}

func TestMiddleware_NonBearerScheme(t *testing.T) {
	store := &fakeTokenLookup{}
	mw := bearerTokenMiddlewareWithStore(store, nil)
	handler := mw(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}

func TestMiddleware_ValidToken(t *testing.T) {
	principalID := uuid.New()
	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: principalID,
		Permissions: []string{"memories:read"},
	}
	store := &fakeTokenLookup{token: tok}
	mw := bearerTokenMiddlewareWithStore(store, nil)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := r.Context().Value(ContextKeyPrincipalID)
		if pid == nil {
			t.Error("principal_id not in context")
		}
		// ContextKeyPermissions must now store authz.PermissionSet.
		perms, ok := r.Context().Value(ContextKeyPermissions).(authz.PermissionSet)
		if !ok {
			t.Error("ContextKeyPermissions must be authz.PermissionSet")
		}
		if !perms.Contains("memories:read") {
			t.Errorf("expected memories:read in PermissionSet, got %v", perms.Permissions())
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer pb_validtoken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
}

func TestMiddleware_TokenPermissions_ParsedAsPermissionSet(t *testing.T) {
	// Verify that bare shorthands are expanded and stored as authz.PermissionSet.
	principalID := uuid.New()
	tok := &db.Token{
		ID:          uuid.New(),
		PrincipalID: principalID,
		Permissions: []string{"read"}, // shorthand → all :read permissions
	}
	store := &fakeTokenLookup{token: tok}
	mw := bearerTokenMiddlewareWithStore(store, nil)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		perms, ok := r.Context().Value(ContextKeyPermissions).(authz.PermissionSet)
		if !ok {
			t.Error("ContextKeyPermissions must be authz.PermissionSet")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// "read" shorthand should expand to memories:read, knowledge:read, etc.
		for _, p := range []authz.Permission{"memories:read", "knowledge:read", "scopes:read"} {
			if !perms.Contains(p) {
				t.Errorf("expected %s in expanded PermissionSet", p)
			}
		}
		// Should NOT contain any :write permissions.
		if perms.Contains("memories:write") {
			t.Error("memories:write should not be present for read-only token")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer pb_validtoken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d; want 200", w.Code)
	}
}

func TestMiddleware_UnknownToken(t *testing.T) {
	// Lookup returns nil, nil (token not found).
	store := &fakeTokenLookup{token: nil, err: nil}
	mw := bearerTokenMiddlewareWithStore(store, nil)
	handler := mw(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer pb_unknowntoken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}

func TestMiddleware_BearerPrefixEmptyToken(t *testing.T) {
	// "Bearer " with nothing after the space — rawToken == "".
	store := &fakeTokenLookup{}
	mw := bearerTokenMiddlewareWithStore(store, nil)
	handler := mw(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}

func TestMiddleware_LookupError_Returns401(t *testing.T) {
	// Lookup returns an error — should still 401, not 500.
	store := &fakeTokenLookup{err: errors.New("db down")}
	mw := bearerTokenMiddlewareWithStore(store, nil)
	handler := mw(okHandler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer pb_sometoken")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d; want 401", w.Code)
	}
}
