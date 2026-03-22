package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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
		Permissions: []string{"read"},
	}
	store := &fakeTokenLookup{token: tok}
	mw := bearerTokenMiddlewareWithStore(store, nil)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pid := r.Context().Value(ContextKeyPrincipalID)
		if pid == nil {
			t.Error("principal_id not in context")
		}
		perms := r.Context().Value(ContextKeyPermissions)
		if perms == nil {
			t.Error("permissions not in context")
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
